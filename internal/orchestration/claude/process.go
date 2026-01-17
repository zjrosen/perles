// Package claude provides a Go interface to headless Claude Code sessions.
package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
// Process implements client.HeadlessProcess.
type Process struct {
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdout     io.ReadCloser
	stderr     io.ReadCloser
	sessionID  string
	mainModel  string // Main model name from init event, used to extract correct usage from modelUsage
	workDir    string
	status     client.ProcessStatus
	events     chan client.OutputEvent
	errors     chan error
	cancelFunc context.CancelFunc
	ctx        context.Context
	mu         sync.RWMutex
	wg         sync.WaitGroup

	// stderrLines captures stderr output for inclusion in error messages.
	// Protected by mu.
	stderrLines []string
}

// ErrTimeout is returned when a Claude process exceeds its configured timeout.
var ErrTimeout = fmt.Errorf("claude process timed out")

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

	p := &Process{
		cmd:        cmd,
		stdin:      nil, // Not needed for --print mode
		stdout:     stdout,
		stderr:     stderr,
		sessionID:  cfg.SessionID,
		workDir:    cfg.WorkDir,
		status:     client.StatusPending,
		events:     make(chan client.OutputEvent, 100),
		errors:     make(chan error, 10),
		cancelFunc: cancel,
		ctx:        procCtx,
	}

	if err := cmd.Start(); err != nil {
		cancel()
		log.Debug(log.CatOrch, "Failed to start claude process", "subsystem", "claude", "error", err)
		return nil, fmt.Errorf("failed to start claude process: %w", err)
	}

	log.Debug(log.CatOrch, "Claude process started", "subsystem", "claude", "pid", cmd.Process.Pid)
	p.setStatus(client.StatusRunning)

	// Start output parser goroutines
	p.wg.Add(3)
	go p.parseOutput()
	go p.parseStderr()
	go p.waitForCompletion()

	return p, nil
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

// Events returns a channel that receives parsed output events.
func (p *Process) Events() <-chan client.OutputEvent {
	return p.events
}

// Errors returns a channel that receives errors.
func (p *Process) Errors() <-chan error {
	return p.errors
}

// SessionID returns the session ID (may be empty until init event is received).
func (p *Process) SessionID() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.sessionID
}

// Status returns the current process status.
func (p *Process) Status() client.ProcessStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.status
}

// IsRunning returns true if the process is currently running.
func (p *Process) IsRunning() bool {
	return p.Status() == client.StatusRunning
}

// WorkDir returns the working directory of the process.
func (p *Process) WorkDir() string {
	return p.workDir
}

// PID returns the process ID of the Claude process, or 0 if not running.
func (p *Process) PID() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Pid
	}
	return 0
}

// Cancel terminates the Claude process.
// The status is set before calling cancelFunc to prevent race with waitForCompletion.
func (p *Process) Cancel() error {
	p.mu.Lock()
	p.status = client.StatusCancelled
	p.mu.Unlock()
	p.cancelFunc()
	return nil
}

// Wait blocks until the process completes.
func (p *Process) Wait() error {
	p.wg.Wait()
	return nil
}

// SessionRef returns the session reference (session ID for Claude).
// This implements client.HeadlessProcess.
// May be empty until the init event is received.
func (p *Process) SessionRef() string {
	return p.SessionID()
}

// setStatus updates the process status thread-safely.
func (p *Process) setStatus(s client.ProcessStatus) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status = s
}

// sendError attempts to send an error to the errors channel.
// If the channel is full, the error is logged but not sent to avoid blocking.
func (p *Process) sendError(err error) {
	select {
	case p.errors <- err:
		// Error sent successfully
	default:
		// Channel full, log the dropped error
		log.Debug(log.CatOrch, "error channel full, dropping error", "subsystem", "claude", "error", err)
	}
}

// parseOutput reads stdout and parses stream-json events.
func (p *Process) parseOutput() {
	defer p.wg.Done()
	defer close(p.events)

	scanner := bufio.NewScanner(p.stdout)
	// Increase buffer size for large outputs
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		event, err := parseEvent(line)
		if err != nil {
			log.Debug(log.CatOrch, "parse error", "error", err, "line", string(line))
			continue
		}

		event.Raw = make([]byte, len(line))
		copy(event.Raw, line)
		event.Timestamp = time.Now()

		// Extract session ID and model from init event
		if event.Type == client.EventSystem && event.SubType == "init" {
			// Parse model from raw JSON since it's not in OutputEvent struct
			var initData struct {
				SessionID string `json:"session_id"`
				Model     string `json:"model"`
			}
			if err := json.Unmarshal(line, &initData); err == nil {
				p.mu.Lock()
				if initData.SessionID != "" {
					p.sessionID = initData.SessionID
				}
				if initData.Model != "" {
					p.mainModel = initData.Model
				}
				p.mu.Unlock()
				log.Debug(log.CatOrch, "session init", "sessionID", initData.SessionID, "model", initData.Model)
			}
		}

		// TODO this could be wrong but currently the EventResult doesnt' feel like its the correct token usage.
		// will need to revisit this to understand if this is a Claude bug or if we should be using the assistant event
		if event.Type == client.EventAssistant && event.Usage != nil {
			p.mu.RLock()
			sessionID := p.sessionID
			p.mu.RUnlock()

			log.Debug(log.CatOrch, "result context",
				"session", sessionID,
				"tokensUsed", event.Usage.TokensUsed,
				"pctUsed", float64(event.Usage.TokensUsed)/200000*100)
		}

		select {
		case p.events <- event:
		case <-p.ctx.Done():
			return
		}
	}

	if err := scanner.Err(); err != nil {
		log.Debug(log.CatOrch, "scanner error", "subsystem", "claude", "error", err)
		p.sendError(fmt.Errorf("stdout scanner error: %w", err))
	}
}

// parseStderr reads and logs stderr output, capturing lines for error messages.
func (p *Process) parseStderr() {
	defer p.wg.Done()

	scanner := bufio.NewScanner(p.stderr)
	for scanner.Scan() {
		line := scanner.Text()
		log.Debug(log.CatOrch, "STDERR", "subsystem", "claude", "line", line)

		// Capture stderr lines for inclusion in error messages
		p.mu.Lock()
		p.stderrLines = append(p.stderrLines, line)
		p.mu.Unlock()
	}
	if err := scanner.Err(); err != nil {
		log.Debug(log.CatOrch, "Stderr scanner error", "subsystem", "claude", "error", err)
	}
}

// waitForCompletion waits for the process to exit and updates status.
// It closes the errors channel when done to signal completion to consumers.
func (p *Process) waitForCompletion() {
	defer p.wg.Done()
	defer close(p.errors) // Signal that no more errors will be sent

	err := p.cmd.Wait()

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.status == client.StatusCancelled {
		log.Debug(log.CatOrch, "process was cancelled", "subsystem", "claude")
		return
	}

	if errors.Is(p.ctx.Err(), context.DeadlineExceeded) {
		p.status = client.StatusFailed
		log.Debug(log.CatOrch, "process timed out", "subsystem", "claude")
		p.sendError(ErrTimeout)
		return
	}

	if err != nil {
		p.status = client.StatusFailed
		if len(p.stderrLines) > 0 {
			stderrMsg := strings.Join(p.stderrLines, "\n")
			p.sendError(fmt.Errorf("claude process failed: %s (exit: %w)", stderrMsg, err))
		} else {
			p.sendError(fmt.Errorf("claude process exited: %w", err))
		}
	} else {
		p.status = client.StatusCompleted
	}
}

// Ensure Process implements client.HeadlessProcess at compile time.
var _ client.HeadlessProcess = (*Process)(nil)
