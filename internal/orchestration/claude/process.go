// Package claude provides a Go interface to headless Claude Code sessions.
package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/client"
)

// findExecutable returns the path to the claude executable.
// It checks common installation locations before falling back to PATH lookup.
// This handles cases where claude is installed via pnpm/npm and uses an alias
// rather than being directly in PATH.
func findExecutable() (string, error) {
	// Common installation paths to check
	homeDir, err := os.UserHomeDir()
	if err == nil {
		knownPaths := []string{
			filepath.Join(homeDir, ".claude", "local", "claude"),
			filepath.Join(homeDir, ".claude", "claude"),
		}
		for _, path := range knownPaths {
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				log.Debug(log.CatOrch, "Found claude executable", "subsystem", "claude", "path", path)
				return path, nil
			}
		}
	}

	// Fall back to PATH lookup
	path, err := exec.LookPath("claude")
	if err != nil {
		return "", fmt.Errorf("claude executable not found in known locations or PATH: %w", err)
	}
	return path, nil
}

// Config holds configuration for spawning a Claude process.
type Config struct {
	WorkDir            string
	Prompt             string
	SessionID          string // For --resume
	Model              string // sonnet, opus, haiku
	AppendSystemPrompt string
	AllowedTools       []string
	DisallowedTools    []string
	SkipPermissions    bool
	Timeout            time.Duration
	MCPConfig          string // JSON string for --mcp-config flag
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
func Spawn(ctx context.Context, cfg Config) (*Process, error) {
	var procCtx context.Context
	var cancel context.CancelFunc
	if cfg.Timeout > 0 {
		procCtx, cancel = context.WithTimeout(ctx, cfg.Timeout)
	} else {
		procCtx, cancel = context.WithCancel(ctx)
	}

	claudePath, err := findExecutable()
	if err != nil {
		cancel()
		return nil, err
	}

	args := buildArgs(cfg)
	log.Debug(log.CatOrch, "Spawning claude process", "subsystem", "claude", "executable", claudePath, "args", strings.Join(args, " "), "workDir", cfg.WorkDir)

	// #nosec G204 -- args are built from Config struct, not user input
	cmd := exec.CommandContext(procCtx, claudePath, args...)
	cmd.Dir = cfg.WorkDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Create the Claude process with embedded BaseProcess
	p := &Process{}

	// Create BaseProcess with Claude-specific hooks
	bp := client.NewBaseProcess(
		procCtx,
		cancel,
		cmd,
		stdout,
		stderr,
		cfg.WorkDir,
		client.WithEventParser(NewParser()),
		client.WithSessionExtractor(extractSession),
		client.WithOnInitEvent(p.extractMainModel),
		client.WithStderrCapture(true),
		client.WithProviderName("claude"),
	)
	p.BaseProcess = bp

	// Set initial session ID if provided (for --resume)
	if cfg.SessionID != "" {
		bp.SetSessionRef(cfg.SessionID)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		log.Debug(log.CatOrch, "Failed to start claude process", "subsystem", "claude", "error", err)
		return nil, fmt.Errorf("failed to start claude process: %w", err)
	}

	log.Debug(log.CatOrch, "Claude process started", "subsystem", "claude", "pid", cmd.Process.Pid)
	bp.SetStatus(client.StatusRunning)

	// Start output parser goroutines
	bp.StartGoroutines()

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
