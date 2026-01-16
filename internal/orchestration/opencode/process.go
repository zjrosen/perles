package opencode

import (
	"bufio"
	"context"
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

// Process represents a headless OpenCode CLI process.
// Process implements client.HeadlessProcess.
type Process struct {
	cmd        *exec.Cmd
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

	// stderrLines captures stderr output for inclusion in error messages.
	// Protected by mu.
	stderrLines []string
}

// ErrTimeout is returned when an OpenCode process exceeds its configured timeout.
var ErrTimeout = fmt.Errorf("opencode process timed out")

// ErrNotFound is returned when the opencode executable cannot be found.
var ErrNotFound = fmt.Errorf("opencode: executable not found - ensure 'opencode' is in PATH")

// findExecutable locates the opencode executable.
func findExecutable() (string, error) {
	// On Windows, executables need .exe extension
	execName := "opencode"
	if os.PathSeparator == '\\' {
		execName = "opencode.exe"
	}

	// Check ~/.local/bin/opencode first (common Go binary location)
	homeDir, err := os.UserHomeDir()
	if err == nil {
		localPath := filepath.Join(homeDir, ".local", "bin", execName)
		if _, err := os.Stat(localPath); err == nil {
			log.Debug(log.CatOrch, "Found opencode at local bin", "subsystem", "opencode", "path", localPath)
			return localPath, nil
		}
	}

	// Check /usr/local/bin/opencode
	localPath := "/usr/local/bin/opencode"
	if _, err := os.Stat(localPath); err == nil {
		log.Debug(log.CatOrch, "Found opencode at system bin", "subsystem", "opencode", "path", localPath)
		return localPath, nil
	}

	// Fall back to exec.LookPath
	path, err := exec.LookPath("opencode")
	if err == nil {
		log.Debug(log.CatOrch, "Found opencode via PATH", "subsystem", "opencode", "path", path)
		return path, nil
	}

	return "", ErrNotFound
}

// Spawn creates and starts a new headless OpenCode process.
// Context is used for cancellation and timeout control.
func Spawn(ctx context.Context, cfg Config) (*Process, error) {
	return spawnProcess(ctx, cfg, false)
}

// Resume continues an existing OpenCode session using --session flag.
func Resume(ctx context.Context, sessionID string, cfg Config) (*Process, error) {
	cfg.SessionID = sessionID
	return spawnProcess(ctx, cfg, true)
}

// spawnProcess is the internal implementation for both Spawn and Resume.
func spawnProcess(ctx context.Context, cfg Config, isResume bool) (*Process, error) {
	// Find the opencode executable
	execPath, err := findExecutable()
	if err != nil {
		return nil, err
	}

	var procCtx context.Context
	var cancel context.CancelFunc
	if cfg.Timeout > 0 {
		procCtx, cancel = context.WithTimeout(ctx, cfg.Timeout)
	} else {
		procCtx, cancel = context.WithCancel(ctx)
	}

	args := buildArgs(cfg, isResume)
	log.Debug(log.CatOrch, "Spawning opencode process", "subsystem", "opencode", "args", strings.Join(args, " "), "workDir", cfg.WorkDir)

	// #nosec G204 -- args are built from Config struct, not user input
	cmd := exec.CommandContext(procCtx, execPath, args...)
	cmd.Dir = cfg.WorkDir

	// Pass MCP config via OPENCODE_CONFIG_CONTENT env var for process isolation.
	// Each process gets its own MCP config without file conflicts.
	if cfg.MCPConfig != "" {
		cmd.Env = append(os.Environ(), "OPENCODE_CONFIG_CONTENT="+cfg.MCPConfig)
		log.Debug(log.CatOrch, "Setting OPENCODE_CONFIG_CONTENT", "subsystem", "opencode", "config", cfg.MCPConfig)
	}

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
		stdout:     stdout,
		stderr:     stderr,
		sessionID:  "", // Will be set from init event
		workDir:    cfg.WorkDir,
		status:     client.StatusPending,
		events:     make(chan client.OutputEvent, 100),
		errors:     make(chan error, 10),
		cancelFunc: cancel,
		ctx:        procCtx,
	}

	if err := cmd.Start(); err != nil {
		cancel()
		log.Debug(log.CatOrch, "Failed to start opencode process", "subsystem", "opencode", "error", err)
		return nil, fmt.Errorf("failed to start opencode process: %w", err)
	}

	log.Debug(log.CatOrch, "OpenCode process started", "subsystem", "opencode", "pid", cmd.Process.Pid)
	p.setStatus(client.StatusRunning)

	// Start output parser goroutines
	p.wg.Add(3)
	go p.parseOutput()
	go p.parseStderr()
	go p.waitForCompletion()

	return p, nil
}

// Events returns a channel that receives parsed output events.
func (p *Process) Events() <-chan client.OutputEvent {
	return p.events
}

// Errors returns a channel that receives errors.
func (p *Process) Errors() <-chan error {
	return p.errors
}

// SessionRef returns the session reference (session ID for OpenCode).
// May be empty until the init event is received.
func (p *Process) SessionRef() string {
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

// PID returns the process ID of the OpenCode process, or 0 if not running.
func (p *Process) PID() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Pid
	}
	return 0
}

// Cancel terminates the OpenCode process.
// The status is set before calling cancelFunc to prevent race with waitForCompletion.
func (p *Process) Cancel() error {
	p.mu.Lock()
	// Only set to cancelled if not already in a terminal state
	if !p.status.IsTerminal() {
		p.status = client.StatusCancelled
	}
	p.mu.Unlock()
	p.cancelFunc()
	return nil
}

// Wait blocks until the process completes.
func (p *Process) Wait() error {
	p.wg.Wait()
	return nil
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
		log.Debug(log.CatOrch, "Error channel full, dropping error", "subsystem", "opencode", "error", err)
	}
}

// parseOutput reads stdout and parses JSON events.
func (p *Process) parseOutput() {
	defer p.wg.Done()
	defer close(p.events)

	log.Debug(log.CatOrch, "Starting output parser", "subsystem", "opencode")

	scanner := bufio.NewScanner(p.stdout)
	// Increase buffer size for large outputs (64KB initial, 1MB max)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	lineCount := 0
	for scanner.Scan() {
		line := scanner.Bytes()
		lineCount++

		if len(line) == 0 {
			continue
		}

		// Log raw JSON for debugging
		log.Debug(log.CatOrch, "RAW_JSON", "subsystem", "opencode", "lineNum", lineCount, "json", string(line))

		event, err := parseEvent(line)
		if err != nil {
			log.Debug(log.CatOrch, "Failed to parse JSON", "subsystem", "opencode", "error", err, "line", string(line[:min(100, len(line))]))
			continue
		}

		log.Debug(log.CatOrch, "Parsed event", "subsystem", "opencode", "type", event.Type, "subtype", event.SubType, "hasTool", event.Tool != nil, "hasMessage", event.Message != nil)

		// Log Usage data for debugging token tracking
		if event.Type == client.EventResult || event.Usage != nil {
			log.Debug(log.CatOrch, "EVENT_USAGE",
				"subsystem", "opencode",
				"type", event.Type,
				"hasUsage", event.Usage != nil,
				"totalCostUSD", event.TotalCostUSD,
				"durationMs", event.DurationMs)
			if event.Usage != nil {
				log.Debug(log.CatOrch, "USAGE_DETAILS",
					"subsystem", "opencode",
					"tokensUsed", event.Usage.TokensUsed,
					"totalTokens", event.Usage.TotalTokens,
					"outputTokens", event.Usage.OutputTokens)
			}
		}

		event.Timestamp = time.Now()

		// Extract session_id from any event that has it (if we don't have one yet)
		// OpenCode doesn't emit a system/init event, so we capture from the first event with sessionID
		if event.SessionID != "" {
			p.mu.Lock()
			if p.sessionID == "" {
				p.sessionID = event.SessionID
				log.Debug(log.CatOrch, "Got session ID", "subsystem", "opencode", "sessionID", event.SessionID)
			}
			p.mu.Unlock()
		}

		select {
		case p.events <- event:
			log.Debug(log.CatOrch, "Sent event to channel", "subsystem", "opencode", "type", event.Type)
		case <-p.ctx.Done():
			log.Debug(log.CatOrch, "Context done, stopping parser", "subsystem", "opencode")
			return
		}
	}

	log.Debug(log.CatOrch, "Scanner finished", "subsystem", "opencode", "totalLines", lineCount)

	if err := scanner.Err(); err != nil {
		log.Debug(log.CatOrch, "Scanner error", "subsystem", "opencode", "error", err)
		p.sendError(fmt.Errorf("stdout scanner error: %w", err))
	}
}

// parseStderr reads and logs stderr output, capturing lines for error messages.
func (p *Process) parseStderr() {
	defer p.wg.Done()

	scanner := bufio.NewScanner(p.stderr)
	for scanner.Scan() {
		line := scanner.Text()
		log.Debug(log.CatOrch, "STDERR", "subsystem", "opencode", "line", line)

		// Capture stderr lines for inclusion in error messages
		p.mu.Lock()
		p.stderrLines = append(p.stderrLines, line)
		p.mu.Unlock()
	}
	if err := scanner.Err(); err != nil {
		log.Debug(log.CatOrch, "Stderr scanner error", "subsystem", "opencode", "error", err)
	}
}

// waitForCompletion waits for the process to exit and updates status.
// It closes the errors channel when done to signal completion to consumers.
func (p *Process) waitForCompletion() {
	defer p.wg.Done()
	defer close(p.errors) // Signal that no more errors will be sent

	log.Debug(log.CatOrch, "Waiting for process to complete", "subsystem", "opencode")
	err := p.cmd.Wait()
	log.Debug(log.CatOrch, "Process completed", "subsystem", "opencode", "error", err)

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.status == client.StatusCancelled {
		// Already cancelled, don't override
		log.Debug(log.CatOrch, "Process was cancelled", "subsystem", "opencode")
		return
	}

	// Check if this was a timeout
	if errors.Is(p.ctx.Err(), context.DeadlineExceeded) {
		p.status = client.StatusFailed
		log.Debug(log.CatOrch, "Process timed out", "subsystem", "opencode")
		p.sendError(ErrTimeout)
		return
	}

	if err != nil {
		p.status = client.StatusFailed
		log.Debug(log.CatOrch, "Process failed", "subsystem", "opencode", "error", err)
		// Include stderr output in error message if available
		if len(p.stderrLines) > 0 {
			stderrMsg := strings.Join(p.stderrLines, "\n")
			p.sendError(fmt.Errorf("opencode process failed: %s (exit: %w)", stderrMsg, err))
		} else {
			p.sendError(fmt.Errorf("opencode process exited: %w", err))
		}
	} else {
		p.status = client.StatusCompleted
		log.Debug(log.CatOrch, "Process completed successfully", "subsystem", "opencode")
	}
}

// Ensure Process implements client.HeadlessProcess at compile time.
var _ client.HeadlessProcess = (*Process)(nil)
