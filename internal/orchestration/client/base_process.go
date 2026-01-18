// Package client provides common types and utilities for headless AI process management.
package client

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/zjrosen/perles/internal/log"
)

// ErrTimeout is returned when a process exceeds its configured timeout.
var ErrTimeout = fmt.Errorf("process timed out")

// ParseEventFunc parses a JSON line from stdout into an OutputEvent.
// Each provider implements this to handle their specific JSON format.
type ParseEventFunc func(line []byte) (OutputEvent, error)

// SessionExtractorFunc extracts the session reference from an event.
// Returns the session ID/thread ID, or empty string if not found.
// Called for every event (not just init) to support OpenCode's pattern.
type SessionExtractorFunc func(event OutputEvent, rawLine []byte) string

// OnInitEventFunc is called when an init event is received.
// Providers can use this for additional processing (e.g., Claude extracts mainModel).
// Optional - only set if provider needs extra init handling.
type OnInitEventFunc func(event OutputEvent, rawLine []byte)

// BaseProcessOption is a functional option for configuring BaseProcess.
type BaseProcessOption func(*BaseProcess)

// WithParseEventFunc sets the event parsing function.
func WithParseEventFunc(fn ParseEventFunc) BaseProcessOption {
	return func(bp *BaseProcess) {
		bp.parseEventFn = fn
	}
}

// WithSessionExtractor sets the session extraction function.
func WithSessionExtractor(fn SessionExtractorFunc) BaseProcessOption {
	return func(bp *BaseProcess) {
		bp.extractSessionFn = fn
	}
}

// WithOnInitEvent sets the init event callback.
func WithOnInitEvent(fn OnInitEventFunc) BaseProcessOption {
	return func(bp *BaseProcess) {
		bp.onInitEventFn = fn
	}
}

// WithStderrCapture enables stderr line capture for error messages.
func WithStderrCapture(capture bool) BaseProcessOption {
	return func(bp *BaseProcess) {
		bp.captureStderr = capture
	}
}

// WithProviderName sets the provider name for logging.
func WithProviderName(name string) BaseProcessOption {
	return func(bp *BaseProcess) {
		bp.providerName = name
	}
}

// BaseProcess provides common process lifecycle management for all providers.
// Providers embed this struct and configure behavior via functional options.
type BaseProcess struct {
	// Core process fields
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdout     io.ReadCloser
	stderr     io.ReadCloser
	sessionRef string // Unified name (was sessionID/threadID)
	workDir    string
	status     ProcessStatus
	events     chan OutputEvent
	errors     chan error
	cancelFunc context.CancelFunc
	ctx        context.Context
	mu         sync.RWMutex
	wg         sync.WaitGroup

	// Optional stderr capture (3/5 providers use this)
	stderrLines   []string
	captureStderr bool

	// Provider identification (for logging/errors)
	providerName string

	// Hook functions (set via functional options)
	parseEventFn     ParseEventFunc
	extractSessionFn SessionExtractorFunc
	onInitEventFn    OnInitEventFunc
}

// NewBaseProcess creates a new BaseProcess with the given configuration.
// The cmd must already have its pipes set up (stdout, stderr, and optionally stdin).
func NewBaseProcess(
	ctx context.Context,
	cancelFunc context.CancelFunc,
	cmd *exec.Cmd,
	stdout io.ReadCloser,
	stderr io.ReadCloser,
	workDir string,
	opts ...BaseProcessOption,
) *BaseProcess {
	bp := &BaseProcess{
		cmd:          cmd,
		stdout:       stdout,
		stderr:       stderr,
		workDir:      workDir,
		status:       StatusPending,
		events:       make(chan OutputEvent, 100),
		errors:       make(chan error, 10),
		cancelFunc:   cancelFunc,
		ctx:          ctx,
		providerName: "base", // default, should be overridden
	}

	// Apply functional options
	for _, opt := range opts {
		opt(bp)
	}

	return bp
}

// SetStdin sets the stdin writer for providers that need it (Amp, Codex).
func (bp *BaseProcess) SetStdin(stdin io.WriteCloser) {
	bp.stdin = stdin
}

// SetSessionRef sets the session reference. Thread-safe.
func (bp *BaseProcess) SetSessionRef(ref string) {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	bp.sessionRef = ref
}

// Events returns the channel of parsed output events.
// The channel is closed when the process completes.
func (bp *BaseProcess) Events() <-chan OutputEvent {
	return bp.events
}

// Errors returns the channel of process errors.
// Non-blocking; errors are dropped if the channel is full.
func (bp *BaseProcess) Errors() <-chan error {
	return bp.errors
}

// Status returns the current process status. Thread-safe.
func (bp *BaseProcess) Status() ProcessStatus {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	return bp.status
}

// IsRunning returns true if the process is actively running.
func (bp *BaseProcess) IsRunning() bool {
	return bp.Status() == StatusRunning
}

// WorkDir returns the working directory of the process.
func (bp *BaseProcess) WorkDir() string {
	return bp.workDir
}

// PID returns the OS process ID, or -1 if not running.
func (bp *BaseProcess) PID() int {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	if bp.cmd == nil || bp.cmd.Process == nil {
		return -1
	}
	return bp.cmd.Process.Pid
}

// SessionRef returns the session reference (session ID, thread ID, etc).
// May be empty until the init event is received. Thread-safe.
func (bp *BaseProcess) SessionRef() string {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	return bp.sessionRef
}

// Stdin returns the stdin writer, or nil if not configured.
func (bp *BaseProcess) Stdin() io.WriteCloser {
	return bp.stdin
}

// Context returns the process context.
func (bp *BaseProcess) Context() context.Context {
	return bp.ctx
}

// EventsWritable returns the writable events channel for internal use.
func (bp *BaseProcess) EventsWritable() chan OutputEvent {
	return bp.events
}

// ErrorsWritable returns the writable errors channel for internal use.
func (bp *BaseProcess) ErrorsWritable() chan error {
	return bp.errors
}

// Cmd returns the underlying exec.Cmd.
func (bp *BaseProcess) Cmd() *exec.Cmd {
	return bp.cmd
}

// StderrLines returns captured stderr lines. Thread-safe.
func (bp *BaseProcess) StderrLines() []string {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	result := make([]string, len(bp.stderrLines))
	copy(result, bp.stderrLines)
	return result
}

// AppendStderrLine appends a line to the stderr buffer. Thread-safe.
func (bp *BaseProcess) AppendStderrLine(line string) {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	bp.stderrLines = append(bp.stderrLines, line)
}

// CaptureStderr returns whether stderr capture is enabled.
func (bp *BaseProcess) CaptureStderr() bool {
	return bp.captureStderr
}

// ProviderName returns the provider name for logging.
func (bp *BaseProcess) ProviderName() string {
	return bp.providerName
}

// ParseEventFn returns the configured parse event function.
func (bp *BaseProcess) ParseEventFn() ParseEventFunc {
	return bp.parseEventFn
}

// ExtractSessionFn returns the configured session extractor function.
func (bp *BaseProcess) ExtractSessionFn() SessionExtractorFunc {
	return bp.extractSessionFn
}

// OnInitEventFn returns the configured init event callback.
func (bp *BaseProcess) OnInitEventFn() OnInitEventFunc {
	return bp.onInitEventFn
}

// WaitGroup returns a pointer to the internal WaitGroup for goroutine synchronization.
func (bp *BaseProcess) WaitGroup() *sync.WaitGroup {
	return &bp.wg
}

// Mutex returns a pointer to the internal mutex for thread-safe operations.
func (bp *BaseProcess) Mutex() *sync.RWMutex {
	return &bp.mu
}

// CancelFunc returns the cancel function for the process context.
func (bp *BaseProcess) CancelFunc() context.CancelFunc {
	return bp.cancelFunc
}

// SetStatus updates the process status. Thread-safe.
func (bp *BaseProcess) SetStatus(s ProcessStatus) {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	bp.status = s
}

// SendError attempts to send an error to the errors channel.
// If the channel is full, the error is logged but not sent to avoid blocking.
func (bp *BaseProcess) SendError(err error) {
	select {
	case bp.errors <- err:
		// Error sent successfully
	default:
		// Channel full, log the dropped error
		log.Debug(log.CatOrch, "error channel full, dropping error",
			"subsystem", bp.providerName, "error", err)
	}
}

// Cancel cancels the process. It sets the status to Cancelled before calling
// the cancelFunc to prevent race conditions with waitForCompletion.
// Cancel is a no-op if the process is already in a terminal status.
func (bp *BaseProcess) Cancel() error {
	bp.mu.Lock()
	if bp.status.IsTerminal() {
		bp.mu.Unlock()
		return nil
	}
	bp.status = StatusCancelled
	bp.mu.Unlock()
	bp.cancelFunc()
	return nil
}

// Wait blocks until all process goroutines complete.
func (bp *BaseProcess) Wait() error {
	bp.wg.Wait()
	return nil
}

// StartGoroutines launches the three goroutines that handle output parsing,
// stderr reading, and process completion. Call this after the process is started.
func (bp *BaseProcess) StartGoroutines() {
	bp.wg.Add(3)
	go bp.parseOutput()
	go bp.parseStderr()
	go bp.waitForCompletion()
}

// parseOutput reads stdout and parses stream-json events.
// It calls parseEventFn for each line and extractSessionFn for EVERY event
// to support OpenCode's pattern of capturing session ID from any event.
func (bp *BaseProcess) parseOutput() {
	defer bp.wg.Done()
	defer close(bp.events)

	scanner := bufio.NewScanner(bp.stdout)
	// Increase buffer size for large outputs (64KB initial, 1MB max)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		// If no parse function configured, skip parsing
		if bp.parseEventFn == nil {
			continue
		}

		event, err := bp.parseEventFn(line)
		if err != nil {
			log.Debug(log.CatOrch, "parse error",
				"subsystem", bp.providerName, "error", err, "line", string(line))
			continue
		}

		event.Raw = make([]byte, len(line))
		copy(event.Raw, line)
		event.Timestamp = time.Now()

		// Call extractSessionFn for EVERY event (not just init)
		// This supports OpenCode's pattern of capturing session ID from any event
		if bp.extractSessionFn != nil {
			if sessionRef := bp.extractSessionFn(event, line); sessionRef != "" {
				bp.mu.Lock()
				if bp.sessionRef == "" {
					bp.sessionRef = sessionRef
					log.Debug(log.CatOrch, "got session ref",
						"subsystem", bp.providerName, "sessionRef", sessionRef)
				}
				bp.mu.Unlock()
			}
		}

		// Call onInitEventFn for init events (e.g., Claude extracts mainModel)
		if event.Type == EventSystem && event.SubType == "init" {
			if bp.onInitEventFn != nil {
				bp.onInitEventFn(event, line)
			}
		}

		select {
		case bp.events <- event:
		case <-bp.ctx.Done():
			return
		}
	}

	if err := scanner.Err(); err != nil {
		log.Debug(log.CatOrch, "scanner error",
			"subsystem", bp.providerName, "error", err)
		bp.SendError(fmt.Errorf("stdout scanner error: %w", err))
	}
}

// parseStderr reads and logs stderr output.
// If captureStderr is enabled, lines are captured for inclusion in error messages.
func (bp *BaseProcess) parseStderr() {
	defer bp.wg.Done()

	scanner := bufio.NewScanner(bp.stderr)
	for scanner.Scan() {
		line := scanner.Text()
		log.Debug(log.CatOrch, "STDERR", "subsystem", bp.providerName, "line", line)

		if bp.captureStderr {
			bp.mu.Lock()
			bp.stderrLines = append(bp.stderrLines, line)
			bp.mu.Unlock()
		}
	}
	if err := scanner.Err(); err != nil {
		log.Debug(log.CatOrch, "stderr scanner error",
			"subsystem", bp.providerName, "error", err)
	}
}

// waitForCompletion waits for the process to exit and updates status.
// It closes the errors channel when done to signal completion to consumers.
func (bp *BaseProcess) waitForCompletion() {
	defer bp.wg.Done()
	defer close(bp.errors)

	err := bp.cmd.Wait()

	bp.mu.Lock()
	defer bp.mu.Unlock()

	// If already cancelled, don't override status
	if bp.status == StatusCancelled {
		log.Debug(log.CatOrch, "process was cancelled", "subsystem", bp.providerName)
		return
	}

	// Check for timeout using errors.Is()
	if errors.Is(bp.ctx.Err(), context.DeadlineExceeded) {
		bp.status = StatusFailed
		log.Debug(log.CatOrch, "process timed out", "subsystem", bp.providerName)
		bp.SendError(ErrTimeout)
		return
	}

	if err != nil {
		bp.status = StatusFailed
		// Include stderr output in error message if captured
		if bp.captureStderr && len(bp.stderrLines) > 0 {
			stderrMsg := strings.Join(bp.stderrLines, "\n")
			bp.SendError(fmt.Errorf("%s process failed: %s (exit: %w)", bp.providerName, stderrMsg, err))
		} else {
			bp.SendError(fmt.Errorf("%s process exited: %w", bp.providerName, err))
		}
	} else {
		bp.status = StatusCompleted
	}
}
