// Package shared provides common utilities shared between mode controllers.
package shared

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"text/template"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zjrosen/perles/internal/config"
	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/task"
)

// IssueContext provides template variables for user-defined actions.
// Fields are exported for text/template access.
type IssueContext struct {
	ID        string // Issue ID (e.g., "PROJ-123"), inserted as-is
	Title     string // Issue title (raw, not escaped - user handles quoting in command template)
	TitleText string // Issue title shell-escaped as a single-quoted word; do not wrap in extra quotes
}

// NewIssueContext creates an IssueContext from a beads Issue.
func NewIssueContext(issue *task.Issue) IssueContext {
	if issue == nil {
		return IssueContext{
			ID:        "",
			Title:     "",
			TitleText: shellQuote(""),
		}
	}

	return IssueContext{
		ID:        issue.ID,
		Title:     issue.TitleText,
		TitleText: shellQuote(issue.TitleText),
	}
}

// shellQuote returns s as a single POSIX shell word using single-quote escaping.
// The value is wrapped in single quotes and each embedded single quote is
// replaced with close-quote, escaped quote, reopen-quote:
//
//	'\''
//
// Nothing inside single quotes is interpreted by the shell, so $(...), backticks,
// variables, and newlines are passed through literally.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// renderCommand renders a command template with the given IssueContext.
// If the template contains no template markers ({{), it returns the command unchanged
// without parsing, providing a fast-path for simple commands.
// Template errors are wrapped with context for debugging.
func renderCommand(tmpl string, ctx IssueContext) (string, error) {
	// Fast-path: if no template markers, return as-is without parsing
	if !strings.Contains(tmpl, "{{") {
		return tmpl, nil
	}

	t, err := template.New("").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("invalid template: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("template execution failed: %w", err)
	}

	return buf.String(), nil
}

// ActionExecutedMsg is returned when a user action command is launched.
// Exported for cross-package use in kanban and search modes.
type ActionExecutedMsg struct {
	Name string // Action description for logging
	Err  error  // nil on success, error on failure to start
}

// ExecuteAction executes a user-defined action in fire-and-forget mode.
// It renders the command template, starts the command, and returns immediately
// without waiting for completion.
func ExecuteAction(action config.ActionConfig, issue *task.Issue, workDir string) tea.Cmd {
	return func() tea.Msg {
		// Create issue context and render the command template
		issueCtx := NewIssueContext(issue)
		rendered, err := renderCommand(action.Command, issueCtx)
		if err != nil {
			return ActionExecutedMsg{
				Name: action.Description,
				Err:  fmt.Errorf("template rendering failed: %w", err),
			}
		}

		log.Debug(log.CatUI, "Executing user action (fire-and-forget)",
			"action", action.Description,
			"command", rendered,
			"workDir", workDir)

		// #nosec G204 -- command is user-configured; {{.TitleText}} is shell-escaped for issue content
		cmd := exec.CommandContext(context.Background(), "sh", "-c", rendered)
		cmd.Dir = workDir

		// Start the command and return immediately (fire-and-forget)
		if err := cmd.Start(); err != nil {
			return ActionExecutedMsg{
				Name: action.Description,
				Err:  fmt.Errorf("failed to start command: %w", err),
			}
		}

		log.Debug(log.CatUI, "User action launched",
			"action", action.Description,
			"pid", cmd.Process.Pid)

		return ActionExecutedMsg{
			Name: action.Description,
			Err:  nil,
		}
	}
}

// MatchUserAction checks if a key message matches a configured user action.
// Returns the action config, action name, and whether a match was found.
// Uses NormalizeKey for consistent key comparison.
func MatchUserAction(msg tea.KeyMsg, actions map[string]config.ActionConfig) (config.ActionConfig, string, bool) {
	if actions == nil {
		return config.ActionConfig{}, "", false
	}

	keyStr := msg.String()
	normalizedKey := config.NormalizeKey(keyStr)

	for name, action := range actions {
		if config.NormalizeKey(action.Key) == normalizedKey {
			return action, name, true
		}
	}
	return config.ActionConfig{}, "", false
}
