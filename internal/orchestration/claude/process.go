// Package claude provides a Go interface to headless Claude Code sessions.
package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"perles/internal/log"
	"perles/internal/orchestration/client"
)

// Log category for claude operations
const logCat = "claude"

// ProcessStatus is an alias to client.ProcessStatus for backward compatibility.
// Deprecated: Use client.ProcessStatus directly.
type ProcessStatus = client.ProcessStatus

// Status constants - aliases to client package for backward compatibility.
// Deprecated: Use client.Status* constants directly.
const (
	StatusPending   = client.StatusPending
	StatusRunning   = client.StatusRunning
	StatusCompleted = client.StatusCompleted
	StatusFailed    = client.StatusFailed
	StatusCancelled = client.StatusCancelled
)

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

// OutputEvent is an alias for client.OutputEvent for backward compatibility.
// Deprecated: Use client.OutputEvent directly.
//
// Event types and their fields:
//   - system (subtype: init): SessionRef, WorkDir
//   - assistant: Message (with Content blocks that may include tool_use)
//   - tool_result: Tool (with Name, Content/Output)
//   - result (subtype: success): TotalCostUSD, Usage, ModelUsage, DurationMs
//   - error: Error
type OutputEvent = client.OutputEvent

// Type aliases for backward compatibility.
// Deprecated: Use client package types directly.
type (
	ModelUsage     = client.ModelUsage
	MessageContent = client.MessageContent
	ContentBlock   = client.ContentBlock
	ToolContent    = client.ToolContent
	UsageInfo      = client.UsageInfo
	ErrorInfo      = client.ErrorInfo
)

// FormatToolDisplay returns a formatted string for displaying a tool call in the TUI.
// For Bash tools, it shows the description (or command if no description).
// For other tools, it shows just the tool name.
func FormatToolDisplay(b *ContentBlock) string {
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
	workDir    string
	status     client.ProcessStatus
	events     chan client.OutputEvent
	errors     chan error
	cancelFunc context.CancelFunc
	ctx        context.Context
	mu         sync.RWMutex
	wg         sync.WaitGroup
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

	args := buildArgs(cfg)
	log.Debug(logCat, "Spawning claude process", "args", strings.Join(args, " "), "workDir", cfg.WorkDir)

	// #nosec G204 -- args are built from Config struct, not user input
	cmd := exec.CommandContext(procCtx, "claude", args...)
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
		log.Debug(logCat, "Failed to start claude process", "error", err)
		return nil, fmt.Errorf("failed to start claude process: %w", err)
	}

	log.Debug(logCat, "Claude process started", "pid", cmd.Process.Pid)
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
		log.Debug(logCat, "Error channel full, dropping error", "error", err)
	}
}

// parseOutput reads stdout and parses stream-json events.
func (p *Process) parseOutput() {
	defer p.wg.Done()
	defer close(p.events)

	log.Debug(logCat, "Starting output parser")

	scanner := bufio.NewScanner(p.stdout)
	// Increase buffer size for large outputs
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	lineCount := 0
	for scanner.Scan() {
		line := scanner.Bytes()
		lineCount++

		if len(line) == 0 {
			continue
		}

		// Log raw JSON for debugging - this helps us see what Claude actually outputs
		log.Debug(logCat, "RAW_JSON", "lineNum", lineCount, "json", string(line))

		var event client.OutputEvent
		if err := json.Unmarshal(line, &event); err != nil {
			log.Debug(logCat, "Failed to parse JSON", "error", err, "line", string(line[:min(100, len(line))]))
			continue
		}

		log.Debug(logCat, "Parsed event", "type", event.Type, "subtype", event.SubType, "hasTool", event.Tool != nil, "hasMessage", event.Message != nil)

		// Log Usage data specifically for result events to debug token tracking
		if event.Type == client.EventResult {
			hasUsage := event.Usage != nil
			hasModelUsage := event.ModelUsage != nil
			log.Debug(logCat, "RESULT_EVENT_USAGE",
				"hasUsage", hasUsage,
				"hasModelUsage", hasModelUsage,
				"totalCostUSD", event.TotalCostUSD,
				"durationMs", event.DurationMs)
			if hasUsage {
				log.Debug(logCat, "USAGE_DETAILS",
					"inputTokens", event.Usage.InputTokens,
					"outputTokens", event.Usage.OutputTokens,
					"cacheReadInputTokens", event.Usage.CacheReadInputTokens,
					"cacheCreationInputTokens", event.Usage.CacheCreationInputTokens)
			}
			if hasModelUsage {
				for modelName, usage := range event.ModelUsage {
					log.Debug(logCat, "MODEL_USAGE_DETAILS",
						"model", modelName,
						"inputTokens", usage.InputTokens,
						"outputTokens", usage.OutputTokens,
						"cacheReadInputTokens", usage.CacheReadInputTokens,
						"cacheCreationInputTokens", usage.CacheCreationInputTokens,
						"contextWindow", usage.ContextWindow)
				}
			}
		}

		event.Raw = make([]byte, len(line))
		copy(event.Raw, line)
		event.Timestamp = time.Now()

		// Extract session ID from init event
		if event.Type == client.EventSystem && event.SubType == "init" && event.SessionID != "" {
			p.mu.Lock()
			p.sessionID = event.SessionID
			p.mu.Unlock()
			log.Debug(logCat, "Got session ID", "sessionID", event.SessionID)
		}

		select {
		case p.events <- event:
			log.Debug(logCat, "Sent event to channel", "type", event.Type)
		case <-p.ctx.Done():
			log.Debug(logCat, "Context done, stopping parser")
			return
		}
	}

	log.Debug(logCat, "Scanner finished", "totalLines", lineCount)

	if err := scanner.Err(); err != nil {
		log.Debug(logCat, "Scanner error", "error", err)
		p.sendError(fmt.Errorf("stdout scanner error: %w", err))
	}
}

// parseStderr reads and logs stderr output.
func (p *Process) parseStderr() {
	defer p.wg.Done()

	scanner := bufio.NewScanner(p.stderr)
	for scanner.Scan() {
		line := scanner.Text()
		log.Debug(logCat, "STDERR", "line", line)
	}
	if err := scanner.Err(); err != nil {
		log.Debug(logCat, "Stderr scanner error", "error", err)
	}
}

// waitForCompletion waits for the process to exit and updates status.
func (p *Process) waitForCompletion() {
	defer p.wg.Done()

	log.Debug(logCat, "Waiting for process to complete")
	err := p.cmd.Wait()
	log.Debug(logCat, "Process completed", "error", err)

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.status == client.StatusCancelled {
		// Already cancelled, don't override
		log.Debug(logCat, "Process was cancelled")
		return
	}

	// Check if this was a timeout
	if p.ctx.Err() == context.DeadlineExceeded {
		p.status = client.StatusFailed
		log.Debug(logCat, "Process timed out")
		p.sendError(ErrTimeout)
		return
	}

	if err != nil {
		p.status = client.StatusFailed
		log.Debug(logCat, "Process failed", "error", err)
		p.sendError(fmt.Errorf("claude process exited: %w", err))
	} else {
		p.status = client.StatusCompleted
		log.Debug(logCat, "Process completed successfully")
	}
}

// Ensure Process implements client.HeadlessProcess at compile time.
var _ client.HeadlessProcess = (*Process)(nil)
