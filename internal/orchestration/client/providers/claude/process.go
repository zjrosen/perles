// Package claude provides a Go interface to headless Claude Code sessions.
package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/client"
)

// defaultKnownPaths defines the priority-ordered paths to check for the claude executable.
// These are checked before falling back to PATH lookup.
// Note: Unlike the original implementation, ExecutableFinder adds .exe suffix on Windows,
// fixing a bug where Claude previously didn't work on Windows.
var defaultKnownPaths = []string{
	"~/.claude/local/{name}",
	"~/.claude/{name}",
}

// Config holds configuration for spawning a Claude process.
type Config struct {
	WorkDir            string
	BeadsDir           string // Path to beads database directory for BEADS_DIR env var
	Prompt             string
	SessionID          string // For --resume
	Model              string // sonnet, opus, haiku
	AppendSystemPrompt string
	AllowedTools       []string
	DisallowedTools    []string
	SkipPermissions    bool
	Timeout            time.Duration
	MCPConfig          string            // JSON string for --mcp-config flag
	Env                map[string]string // Custom environment variables (supports ${VAR} expansion)
}

// FormatToolDisplay returns a formatted string for displaying a tool call in the TUI.
// For Bash tools, it shows the description (or command if no description).
// For other tools, it shows just the tool name.
func FormatToolDisplay(b *client.ContentBlock) string {
	if b.Type != "tool_use" || b.Name == "" {
		return ""
	}

	// For Bash tools, extract description or command from input
	if b.Name == "Bash" || b.Name == "bash" {
		var input struct {
			Description string `json:"description"`
			Command     string `json:"command"`
		}
		if err := json.Unmarshal(b.Input, &input); err == nil {
			if input.Description != "" {
				return fmt.Sprintf("🔧 %s: %s", b.Name, input.Description)
			}
			if input.Command != "" {
				// Truncate long commands
				cmd := input.Command
				if len(cmd) > 50 {
					cmd = cmd[:47] + "..."
				}
				return fmt.Sprintf("🔧 %s: %s", b.Name, cmd)
			}
		}
	}

	// For View/Read tools, show the file path
	if b.Name == "View" || b.Name == "view" || b.Name == "Read" || b.Name == "read" {
		var input struct {
			FilePath string `json:"file_path"`
		}
		if err := json.Unmarshal(b.Input, &input); err == nil && input.FilePath != "" {
			// Show just the filename for brevity
			path := input.FilePath
			if idx := strings.LastIndex(path, "/"); idx >= 0 {
				path = path[idx+1:]
			}
			return fmt.Sprintf("🔧 %s: %s", b.Name, path)
		}
	}

	// For Edit/Write tools, show the file path
	if b.Name == "Edit" || b.Name == "edit" || b.Name == "Write" || b.Name == "write" {
		var input struct {
			FilePath string `json:"file_path"`
		}
		if err := json.Unmarshal(b.Input, &input); err == nil && input.FilePath != "" {
			path := input.FilePath
			if idx := strings.LastIndex(path, "/"); idx >= 0 {
				path = path[idx+1:]
			}
			return fmt.Sprintf("🔧 %s: %s", b.Name, path)
		}
	}

	// For Grep/Glob tools, show the pattern
	if b.Name == "Grep" || b.Name == "grep" || b.Name == "Glob" || b.Name == "glob" {
		var input struct {
			Pattern string `json:"pattern"`
		}
		if err := json.Unmarshal(b.Input, &input); err == nil && input.Pattern != "" {
			pattern := input.Pattern
			if len(pattern) > 30 {
				pattern = pattern[:27] + "..."
			}
			return fmt.Sprintf("🔧 %s: %s", b.Name, pattern)
		}
	}

	// Default: just the tool name
	return fmt.Sprintf("🔧 %s", b.Name)
}

// CostInfo holds token usage and cost information.
// This is Claude-specific and not part of the client package.
type CostInfo struct {
	InputTokens  int `json:"input_tokens,omitempty"`
	OutputTokens int `json:"output_tokens,omitempty"`
}

// Process represents a headless Claude Code process.
// Process implements client.HeadlessProcess by embedding BaseProcess.
type Process struct {
	*client.BaseProcess

	// mainModel is Claude-specific: the model name from init event.
	// Used to extract correct usage from modelUsage.
	mainModel string
	mu        sync.RWMutex // protects mainModel
}

// ErrTimeout is returned when a Claude process exceeds its configured timeout.
var ErrTimeout = fmt.Errorf("claude process timed out")

// extractSession extracts the session ID from an init event.
func extractSession(event client.OutputEvent, rawLine []byte) string {
	if event.Type == client.EventSystem && event.SubType == "init" {
		var initData struct {
			SessionID string `json:"session_id"`
		}
		if err := json.Unmarshal(rawLine, &initData); err == nil && initData.SessionID != "" {
			return initData.SessionID
		}
	}
	return ""
}

// Spawn creates and starts a new headless Claude process.
// Context is used for cancellation and timeout control.
// Uses SpawnBuilder for clean process lifecycle management.
func Spawn(ctx context.Context, cfg Config) (*Process, error) {
	// Find the claude executable using ExecutableFinder
	claudePath, err := client.NewExecutableFinder("claude",
		client.WithKnownPaths(defaultKnownPaths...),
	).Find()
	if err != nil {
		return nil, err
	}

	args := buildArgs(cfg)

	// Build environment variables (BEADS_DIR if set)
	env := client.BuildEnvVars(client.Config{BeadsDir: cfg.BeadsDir})

	// Add custom env vars from config, expanding ${VAR} references
	for k, v := range cfg.Env {
		expanded := os.ExpandEnv(v)
		env = append(env, k+"="+expanded)
		// Log non-sensitive env vars (mask tokens/keys)
		logVal := expanded
		if strings.Contains(strings.ToLower(k), "token") ||
			strings.Contains(strings.ToLower(k), "key") ||
			strings.Contains(strings.ToLower(k), "secret") {
			logVal = "[REDACTED]"
		}
		log.Debug(log.CatOrch, "custom env var", "key", k, "value", logVal)
	}

	// Create Process wrapper FIRST (needed for OnInitEvent hook closure)
	p := &Process{}

	// Use SpawnBuilder with OnInitEvent hook for mainModel extraction
	base, err := client.NewSpawnBuilder(ctx).
		WithExecutable(claudePath, args).
		WithWorkDir(cfg.WorkDir).
		WithSessionRef(cfg.SessionID).
		WithTimeout(cfg.Timeout).
		WithParser(NewParser()).
		WithSessionExtractor(extractSession).
		WithOnInitEvent(p.extractMainModel).
		WithStderrCapture(true).
		WithProviderName("claude").
		WithEnv(env).
		Build()
	if err != nil {
		return nil, fmt.Errorf("claude: %w", err)
	}

	// Assign BaseProcess AFTER Build() completes (circular reference pattern)
	p.BaseProcess = base
	return p, nil
}

// extractMainModel extracts the main model name from the init event.
// This is called via the OnInitEvent hook.
func (p *Process) extractMainModel(event client.OutputEvent, rawLine []byte) {
	var initData struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(rawLine, &initData); err == nil && initData.Model != "" {
		p.mu.Lock()
		p.mainModel = initData.Model
		p.mu.Unlock()
		log.Debug(log.CatOrch, "extracted main model", "subsystem", "claude", "model", initData.Model)
	}
}

// MainModel returns the main model name from the init event.
func (p *Process) MainModel() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.mainModel
}

// ResumeWithConfig continues an existing Claude session with full configuration control.
// The SessionID in cfg is used if set, otherwise sessionID parameter is used.
func ResumeWithConfig(ctx context.Context, sessionID string, cfg Config) (*Process, error) {
	if cfg.SessionID == "" {
		cfg.SessionID = sessionID
	}
	return Spawn(ctx, cfg)
}

// buildArgs constructs the command line arguments for claude.
func buildArgs(cfg Config) []string {
	args := []string{
		"--print",
		"--output-format", "stream-json",
		"--verbose",
	}

	if cfg.SessionID != "" {
		args = append(args, "--resume", cfg.SessionID)
	}

	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}

	if cfg.SkipPermissions {
		args = append(args, "--dangerously-skip-permissions")
	}

	if cfg.AppendSystemPrompt != "" {
		args = append(args, "--append-system-prompt", cfg.AppendSystemPrompt)
	}

	for _, tool := range cfg.AllowedTools {
		args = append(args, "--allowed-tools", tool)
	}

	for _, tool := range cfg.DisallowedTools {
		args = append(args, "--disallowed-tools", tool)
	}

	if cfg.MCPConfig != "" {
		args = append(args, "--mcp-config", cfg.MCPConfig)
	}

	// Add -- separator and prompt as final argument
	// The -- ensures the prompt isn't consumed by preceding flags
	if cfg.Prompt != "" {
		args = append(args, "--", cfg.Prompt)
	}

	return args
}

// SessionID returns the session ID (may be empty until init event is received).
// This is a convenience method that wraps SessionRef for backwards compatibility.
func (p *Process) SessionID() string {
	return p.SessionRef()
}

// Ensure Process implements client.HeadlessProcess at compile time.
var _ client.HeadlessProcess = (*Process)(nil)
