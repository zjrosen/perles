package amp

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"perles/internal/log"
	"perles/internal/orchestration/client"
)

// Log category for amp operations.
const logCat = "amp"

// Process represents a headless Amp process.
// Process implements client.HeadlessProcess.
type Process struct {
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdout     io.ReadCloser
	stderr     io.ReadCloser
	threadID   string
	workDir    string
	status     client.ProcessStatus
	events     chan client.OutputEvent
	errors     chan error
	cancelFunc context.CancelFunc
	ctx        context.Context
	mu         sync.RWMutex
	wg         sync.WaitGroup
}

// ErrTimeout is returned when an Amp process exceeds its configured timeout.
var ErrTimeout = fmt.Errorf("amp process timed out")

// Spawn creates and starts a new headless Amp process.
// Context is used for cancellation and timeout control.
func Spawn(ctx context.Context, cfg Config) (*Process, error) {
	return spawnProcess(ctx, cfg, false)
}

// Resume continues an existing Amp thread.
func Resume(ctx context.Context, threadID string, cfg Config) (*Process, error) {
	cfg.ThreadID = threadID
	return spawnProcess(ctx, cfg, true)
}

// spawnProcess is the internal implementation for both Spawn and Resume.
func spawnProcess(ctx context.Context, cfg Config, isResume bool) (*Process, error) {
	var procCtx context.Context
	var cancel context.CancelFunc
	if cfg.Timeout > 0 {
		procCtx, cancel = context.WithTimeout(ctx, cfg.Timeout)
	} else {
		procCtx, cancel = context.WithCancel(ctx)
	}

	args := buildArgs(cfg, isResume)
	log.Debug(logCat, "Spawning amp process", "args", strings.Join(args, " "), "workDir", cfg.WorkDir)

	// #nosec G204 -- args are built from Config struct, not user input
	cmd := exec.CommandContext(procCtx, "amp", args...)
	cmd.Dir = cfg.WorkDir

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
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
		stdin:      stdin,
		stdout:     stdout,
		stderr:     stderr,
		threadID:   cfg.ThreadID,
		workDir:    cfg.WorkDir,
		status:     client.StatusPending,
		events:     make(chan client.OutputEvent, 100),
		errors:     make(chan error, 10),
		cancelFunc: cancel,
		ctx:        procCtx,
	}

	if err := cmd.Start(); err != nil {
		cancel()
		log.Debug(logCat, "Failed to start amp process", "error", err)
		return nil, fmt.Errorf("failed to start amp process: %w", err)
	}

	log.Debug(logCat, "Amp process started", "pid", cmd.Process.Pid)
	p.setStatus(client.StatusRunning)

	// Write prompt to stdin if provided (Amp reads prompt from stdin in execute mode)
	if cfg.Prompt != "" {
		go func() {
			defer func() {
				if closeErr := stdin.Close(); closeErr != nil {
					log.Debug(logCat, "Failed to close stdin", "error", closeErr)
				}
			}()
			_, err := io.WriteString(stdin, cfg.Prompt)
			if err != nil {
				log.Debug(logCat, "Failed to write prompt to stdin", "error", err)
			}
		}()
	} else {
		if closeErr := stdin.Close(); closeErr != nil {
			log.Debug(logCat, "Failed to close stdin", "error", closeErr)
		}
	}

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

// SessionRef returns the thread ID (Amp's equivalent of session ID).
// May be empty until init event is received.
func (p *Process) SessionRef() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.threadID
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

// PID returns the process ID of the Amp process, or 0 if not running.
func (p *Process) PID() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Pid
	}
	return 0
}

// Cancel terminates the Amp process.
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

		// Log raw JSON for debugging
		log.Debug(logCat, "RAW_JSON", "lineNum", lineCount, "json", string(line))

		event, err := parseEvent(line)
		if err != nil {
			log.Debug(logCat, "Failed to parse JSON", "error", err, "line", string(line[:min(100, len(line))]))
			continue
		}

		log.Debug(logCat, "Parsed event", "type", event.Type, "subtype", event.SubType, "hasTool", event.Tool != nil, "hasMessage", event.Message != nil)

		// Log Usage data for debugging token tracking
		if event.Type == client.EventResult || event.Usage != nil {
			log.Debug(logCat, "EVENT_USAGE",
				"type", event.Type,
				"hasUsage", event.Usage != nil,
				"totalCostUSD", event.TotalCostUSD,
				"durationMs", event.DurationMs)
			if event.Usage != nil {
				log.Debug(logCat, "USAGE_DETAILS",
					"inputTokens", event.Usage.InputTokens,
					"outputTokens", event.Usage.OutputTokens,
					"cacheReadInputTokens", event.Usage.CacheReadInputTokens,
					"cacheCreationInputTokens", event.Usage.CacheCreationInputTokens)
			}
		}

		event.Timestamp = time.Now()

		// Extract thread ID from init event
		if event.Type == client.EventSystem && event.SubType == "init" && event.SessionID != "" {
			p.mu.Lock()
			p.threadID = event.SessionID
			p.mu.Unlock()
			log.Debug(logCat, "Got thread ID", "threadID", event.SessionID)
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
		p.sendError(fmt.Errorf("amp process exited: %w", err))
	} else {
		p.status = client.StatusCompleted
		log.Debug(logCat, "Process completed successfully")
	}
}

// Ensure Process implements client.HeadlessProcess at compile time.
var _ client.HeadlessProcess = (*Process)(nil)
