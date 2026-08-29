package process

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/client/providers/claude"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/metrics"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
	"github.com/zjrosen/perles/internal/pubsub"
)

// DefaultOutputBufferCapacity is the default number of lines to keep in output buffer.
const DefaultOutputBufferCapacity = 100

// CommandSubmitter abstracts command submission to the FIFO processor.
// Process uses this to submit commands on state transitions (e.g., turn complete).
type CommandSubmitter interface {
	// Submit enqueues a command for processing.
	Submit(cmd command.Command)
}

// Process manages a single AI process and its event loop.
// It works identically for both coordinator and worker roles - the event loop,
// output buffering, metrics tracking, and turn completion are the same.
// Role-specific behavior is handled by the handlers, not by this struct.
type Process struct {
	// ID is the unique identifier (e.g., "coordinator", "worker-1").
	ID string
	// Role identifies whether this is coordinator or worker.
	Role repository.ProcessRole

	proc         client.HeadlessProcess
	output       *OutputBuffer
	cmdSubmitter CommandSubmitter
	eventBus     *pubsub.Broker[any]

	ctx       context.Context
	cancel    context.CancelFunc
	eventDone chan struct{} // Closed when eventLoop completes

	mu                   sync.RWMutex
	sessionID            string
	sessionIDAtTurnStart string // Session ID at start of current turn (for rollback on failure)
	metrics              *metrics.TokenMetrics
	cumulativeCostUSD    float64 // Running total cost across all turns
	taskID               string  // Worker-specific: current task ID
	isRetired            bool    // Whether this process has been retired
	lastError            error   // Last error received during this turn (for passing to command)
}

// New creates a Process. Call Start() to begin the event loop.
//
// Parameters:
//   - id: unique process identifier (e.g., "coordinator", "worker-1")
//   - role: RoleCoordinator or RoleWorker
//   - proc: the AI process to manage
//   - submitter: for submitting commands on state transitions
//   - eventBus: for publishing process events to subscribers
func New(id string, role repository.ProcessRole, proc client.HeadlessProcess, submitter CommandSubmitter, eventBus *pubsub.Broker[any]) *Process {
	ctx, cancel := context.WithCancel(context.Background())
	return &Process{
		ID:           id,
		Role:         role,
		proc:         proc,
		output:       NewOutputBuffer(DefaultOutputBufferCapacity),
		cmdSubmitter: submitter,
		eventBus:     eventBus,
		ctx:          ctx,
		cancel:       cancel,
		eventDone:    make(chan struct{}),
		metrics:      &metrics.TokenMetrics{},
	}
}

// NewDormant creates a Process without a live AI subprocess.
// Used for session restoration - the process holds the session ID and can be
// activated later via Resume() when a message needs to be delivered.
//
// The dormant process is in a "ready to resume" state:
// - No AI subprocess attached (proc is nil)
// - No event loop running
// - eventDone is pre-closed so Resume() won't block waiting for a prior event loop
//
// Parameters:
//   - id: unique process identifier (e.g., "coordinator", "worker-1")
//   - role: RoleCoordinator or RoleWorker
//   - sessionID: the AI session ID for resuming conversations (from saved session)
//   - submitter: for submitting commands on state transitions
//   - eventBus: for publishing process events to subscribers
func NewDormant(id string, role repository.ProcessRole, sessionID string, submitter CommandSubmitter, eventBus *pubsub.Broker[any]) *Process {
	ctx, cancel := context.WithCancel(context.Background())
	// Pre-close eventDone so Resume() won't block waiting for a prior event loop
	eventDone := make(chan struct{})
	close(eventDone)
	return &Process{
		ID:           id,
		Role:         role,
		proc:         nil, // No live subprocess yet
		sessionID:    sessionID,
		output:       NewOutputBuffer(DefaultOutputBufferCapacity),
		cmdSubmitter: submitter,
		eventBus:     eventBus,
		ctx:          ctx,
		cancel:       cancel,
		eventDone:    eventDone,
		metrics:      &metrics.TokenMetrics{},
	}
}

// Start launches the event loop goroutine.
// The goroutine processes AI events and submits a ProcessTurnCompleteCommand when done.
func (p *Process) Start() {
	go p.eventLoop()
}

// Stop cancels the event loop context and waits for it to finish.
// This is useful for graceful shutdown. Safe to call multiple times.
func (p *Process) Stop() {
	// Get proc reference under lock to terminate the AI subprocess
	p.mu.Lock()
	proc := p.proc
	p.mu.Unlock()

	// Kill the AI subprocess first - this closes its Events() channel
	if proc != nil {
		_ = proc.Cancel()
	}

	// Stop the event loop
	p.cancel()
	<-p.eventDone
}

// Resume attaches a new AI process and restarts the event loop.
// Called when delivering a queued message to a Ready process.
// Note: Status change to Working is handled by DeliverQueuedHandler BEFORE
// calling Resume, so Process doesn't need to emit that event.
func (p *Process) Resume(proc client.HeadlessProcess) {
	p.mu.Lock()
	// Cancel previous event loop if still running
	p.cancel()
	p.mu.Unlock()

	// Wait for previous event loop to complete
	<-p.eventDone

	p.mu.Lock()
	p.proc = proc
	// Create fresh context and done channel for the new event loop
	p.ctx, p.cancel = context.WithCancel(context.Background())
	p.eventDone = make(chan struct{})
	p.mu.Unlock()

	// Start new event loop goroutine
	go p.eventLoop()
}

// eventLoop processes AI events and publishes them to the event bus.
// This is identical for coordinator and workers - both follow the same pattern:
//   - Process output events and buffer them
//   - Extract session ID and metrics
//   - Submit ProcessTurnCompleteCommand when the AI turn finishes
func (p *Process) eventLoop() {
	defer close(p.eventDone)

	p.mu.Lock()
	proc := p.proc
	// Capture session ID at turn start for rollback on failure.
	// If Claude can't find the session, it emits init with a new session ID
	// then exits with error. We need to rollback to the previous valid session.
	p.sessionIDAtTurnStart = p.sessionID
	p.mu.Unlock()

	if proc == nil {
		return
	}

	procEvents := proc.Events()
	procErrors := proc.Errors()

	// Track when each channel closes. We must wait for BOTH to close
	// before calling handleProcessComplete to ensure all errors are processed.
	var eventsClosed, errorsClosed bool

	for !eventsClosed || !errorsClosed {
		select {
		case <-p.ctx.Done():
			return

		case event, ok := <-procEvents:
			if !ok {
				eventsClosed = true
				procEvents = nil // Prevent busy loop on closed channel
				continue
			}
			p.handleOutputEvent(&event)

		case err, ok := <-procErrors:
			if !ok {
				errorsClosed = true
				procErrors = nil // Prevent busy loop on closed channel
				continue
			}
			p.handleError(err)
		}
	}

	p.handleProcessComplete()
}

// handleOutputEvent processes a single output event from the AI process.
func (p *Process) handleOutputEvent(event *client.OutputEvent) {
	// Extract session ID from events.
	// Always update session ID when present - providers send the correct session ID
	// for the current turn. Failed resumes are handled by sessionIDAtTurnStart rollback
	// in handleProcessComplete.
	if event.SessionID != "" {
		p.setSessionID(event.SessionID)
	}

	if event.Usage != nil {
		// Build TokenMetrics from simplified event usage
		m := &metrics.TokenMetrics{
			TokensUsed:    event.Usage.TokensUsed,
			TotalTokens:   event.Usage.TotalTokens,
			OutputTokens:  event.Usage.OutputTokens,
			TurnCostUSD:   event.TotalCostUSD,
			LastUpdatedAt: time.Now(),
		}

		p.setMetrics(m)

		// Emit token usage event
		if m.TokensUsed > 0 {
			p.publishTokenUsageEvent(m)
		}
	}

	// Extract cost from result events when usage data is absent.
	// Cost data typically arrives in result events (TotalCostUSD > 0) while usage data
	// arrives in assistant events. Since result events often have event.Usage == nil,
	// we need to extract cost separately from the usage block above.
	// Only extract here if Usage was nil - otherwise cost was already processed above.
	if event.Type == client.EventResult && event.TotalCostUSD > 0 && event.Usage == nil {
		p.addTurnCost(event.TotalCostUSD)
		p.publishCostEvent(event.TotalCostUSD)
	}

	// Check for context exhaustion in assistant messages with error code
	// This catches the Claude-specific pattern where error is in message content
	if event.IsAssistant() && event.Error != nil && event.Error.IsContextExceeded() {
		p.output.Append("⚠️ Context Exhausted")
		p.handleInFlightError(&ContextExceededError{})
		return
	}

	// Handle error events (e.g., turn.failed, error from Codex, context exceeded from OpenCode)
	if event.Type == client.EventError {
		errMsg := event.GetErrorMessage()
		p.output.Append("⚠️ Error: " + errMsg)

		// Check if this is a context exceeded error
		if event.Error != nil && event.Error.IsContextExceeded() {
			p.output.Append("⚠️ Context Exhausted")
			p.handleInFlightError(&ContextExceededError{})
			return
		}

		p.handleInFlightError(fmt.Errorf("process error: %s", errMsg))
		return
	}

	// Handle result events - may include errors (e.g., "Prompt is too long")
	if event.IsResult() {
		// Check for error results first (e.g., context window exceeded)
		if event.IsErrorResult {
			errMsg := event.GetErrorMessage()
			// Write error to output buffer so it appears in process pane
			p.output.Append("⚠️ Error: " + errMsg)

			// Check if this is a context exceeded error (Amp sends this in result events)
			if event.Error != nil && event.Error.IsContextExceeded() {
				p.handleInFlightError(&ContextExceededError{})
				return
			}

			// Publish immediately for real-time TUI visibility
			p.handleInFlightError(fmt.Errorf("process error: %s", errMsg))
			return
		}
	}

	// Store text output in buffer and emit as output event
	if event.IsAssistant() && event.Message != nil {
		text := event.Message.GetText()
		if text != "" {
			p.output.Append(text)
			p.publishOutputEvent(text, event.Raw, event.Delta)
		}

		// Also emit tool calls for visibility
		for i := range event.Message.Content {
			block := &event.Message.Content[i]
			if block.Type == "tool_use" && block.Name != "" {
				toolMsg := claude.FormatToolDisplay(block)
				p.output.Append(toolMsg)
				p.publishOutputEvent(toolMsg, nil, false)
			}
		}
	}

	// Handle tool_use events (Codex emits these separately from assistant messages)
	if event.IsToolUse() && event.Message != nil {
		for i := range event.Message.Content {
			block := &event.Message.Content[i]
			if block.Type == "tool_use" && block.Name != "" {
				toolMsg := claude.FormatToolDisplay(block)
				p.output.Append(toolMsg)
				p.publishOutputEvent(toolMsg, nil, false)
			}
		}
	}

	// Store tool results in buffer
	if event.IsToolResult() && event.Tool != nil {
		output := event.Tool.GetOutput()
		if output != "" {
			// Truncate very long tool outputs
			if len(output) > 500 {
				output = output[:500] + "..."
			}
			p.output.Append("[" + event.Tool.Name + "] " + output)
		}
	}
}

// handleError processes an error from the AI process (typically exit errors).
// Stores the error for passing to ProcessTurnCompleteCommand.
// Does NOT publish ProcessError - the handler is the authoritative source
// of events and will emit ProcessError for startup failures.
// For in-flight errors that need immediate visibility, use handleInFlightError.
func (p *Process) handleError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.lastError != nil {
		return
	}
	p.lastError = err
}

// handleInFlightError processes an error that occurs during the turn (not at exit).
// These are published immediately for real-time TUI visibility (e.g., usage limits).
// Also stores the error for the handler to include in its ProcessError event.
//
// Note: Does NOT overwrite if we already have a ContextExceededError, since
// that's the most informative error for the handler.
func (p *Process) handleInFlightError(err error) {
	p.mu.Lock()
	// Don't overwrite a ContextExceededError with a generic error
	if _, ok := errors.AsType[*ContextExceededError](p.lastError); !ok {
		p.lastError = err
	}
	p.mu.Unlock()
	p.publishErrorEvent(err)
}

// handleProcessComplete is called when the AI process finishes a turn.
// It submits a ProcessTurnCompleteCommand for the handler to update repository.
func (p *Process) handleProcessComplete() {
	p.mu.RLock()
	proc := p.proc
	m := p.metrics
	sessionIDAtStart := p.sessionIDAtTurnStart
	lastErr := p.lastError
	p.mu.RUnlock()

	if proc == nil {
		return
	}

	// Wait for process to fully complete
	_ = proc.Wait()

	// Determine outcome based on process status AND in-flight errors.
	// A turn is only successful if the process exited cleanly AND no in-flight
	// errors were captured (e.g., API errors like "tool_use ids must be unique").
	var succeeded bool
	procStatus := proc.Status()

	switch procStatus {
	case client.StatusCompleted:
		// Process exited cleanly, but check for in-flight errors (API errors, etc.)
		succeeded = lastErr == nil
	default:
		succeeded = false
		// If process failed, the session ID we captured from init may be invalid.
		// This happens when Claude can't find the session to resume - it emits an init
		// with a new session ID, but then immediately exits with error.
		// Restore the session ID from turn start to avoid using the invalid new one.
		p.mu.Lock()
		if sessionIDAtStart != "" {
			// Restore the known-good session ID from before this turn
			p.sessionID = sessionIDAtStart
		} else {
			// No previous session - clear the invalid one from this failed turn
			p.sessionID = ""
		}
		p.mu.Unlock()
	}

	// Submit unified command - handler routes based on process ID
	// Pass lastError so handler can include it in ProcessError event for startup failures
	if p.cmdSubmitter != nil {
		p.cmdSubmitter.Submit(command.NewProcessTurnCompleteCommand(
			p.ID, succeeded, m, lastErr,
		))
	}
}

// publishOutputEvent publishes an output event to the event bus.
// delta indicates this is a streaming chunk that should be accumulated with previous output.
func (p *Process) publishOutputEvent(text string, rawJSON []byte, delta bool) {
	if p.eventBus == nil {
		return
	}

	// Use unified ProcessEvent for both coordinator and workers
	// Subscribers filter by Role field
	p.eventBus.Publish(pubsub.UpdatedEvent, events.NewProcessEvent(events.ProcessOutput, p.ID, p.Role).
		WithOutput(text).
		WithDelta(delta).
		WithTaskID(p.GetTaskID()).
		WithRawJSON(rawJSON))
}

// publishTokenUsageEvent publishes a token usage event.
func (p *Process) publishTokenUsageEvent(m *metrics.TokenMetrics) {
	if p.eventBus == nil {
		return
	}

	p.eventBus.Publish(pubsub.UpdatedEvent, events.NewProcessEvent(events.ProcessTokenUsage, p.ID, p.Role).
		WithTaskID(p.GetTaskID()).
		WithMetrics(m))
}

// publishErrorEvent publishes an error event.
func (p *Process) publishErrorEvent(err error) {
	if p.eventBus == nil {
		return
	}

	p.eventBus.Publish(pubsub.UpdatedEvent, events.NewProcessEvent(events.ProcessError, p.ID, p.Role).
		WithTaskID(p.GetTaskID()).
		WithError(err))
}

// addTurnCost adds the turn cost to the cumulative total thread-safely.
// This is called when extracting cost from result events, which have TotalCostUSD > 0
// but event.Usage == nil.
func (p *Process) addTurnCost(cost float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cumulativeCostUSD += cost
	log.Debug(log.CatOrch, "cost_extracted", "id", p.ID, "session", p.sessionID, "turn_cost", cost, "cumulative", p.cumulativeCostUSD)
}

// publishCostEvent publishes a cost-only metrics event.
// This is used when cost arrives in result events without token usage data.
// The event uses the existing ProcessTokenUsage type with only cost populated,
// allowing session to accumulate cost without requiring token data.
func (p *Process) publishCostEvent(turnCost float64) {
	if p.eventBus == nil {
		return
	}

	p.eventBus.Publish(pubsub.UpdatedEvent, events.NewProcessEvent(events.ProcessTokenUsage, p.ID, p.Role).
		WithTaskID(p.GetTaskID()).
		WithMetrics(&metrics.TokenMetrics{
			TurnCostUSD:   turnCost,
			TotalCostUSD:  turnCost, // Turn cost for this event (session accumulates)
			LastUpdatedAt: time.Now(),
		}))
}

// setSessionID updates the session ID thread-safely.
func (p *Process) setSessionID(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sessionID = id
}

// SessionID returns the session ID thread-safely.
func (p *Process) SessionID() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.sessionID
}

// setMetrics updates the metrics thread-safely and accumulates cumulative cost.
func (p *Process) setMetrics(m *metrics.TokenMetrics) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Accumulate turn cost into cumulative total
	p.cumulativeCostUSD += m.TurnCostUSD

	log.Debug(log.CatOrch, "tokens", "id", p.ID, "session", p.sessionID, "tokens", m.TokensUsed, "cost", m.TurnCostUSD)

	// Update metrics with cumulative totals
	m.CumulativeCostUSD = p.cumulativeCostUSD
	// IMPORTANT: Publish turn cost (not cumulative) to prevent double-accumulation.
	// The session accumulates turn costs into its own total via updateTokenUsage().
	// If we published cumulative here, session would add cumulative to its running total,
	// causing costs to inflate: Turn1=$0.01→Session=$0.01, Turn2=$0.02→cumulative=$0.03
	// →Session+=$0.03→$0.04 (wrong!). By publishing turn cost, session correctly sums.
	m.TotalCostUSD = m.TurnCostUSD
	p.metrics = m
}

// Metrics returns the current metrics snapshot thread-safely.
func (p *Process) Metrics() *metrics.TokenMetrics {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.metrics
}

// GetTaskID returns the current task ID thread-safely (worker-specific).
func (p *Process) GetTaskID() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.taskID
}

// SetTaskID updates the task ID thread-safely (worker-specific).
func (p *Process) SetTaskID(taskID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.taskID = taskID
}

// Output returns the output buffer.
func (p *Process) Output() *OutputBuffer {
	return p.output
}

// IsRunning returns true if the underlying process is running.
func (p *Process) IsRunning() bool {
	p.mu.RLock()
	proc := p.proc
	p.mu.RUnlock()

	if proc == nil {
		return false
	}
	return proc.IsRunning()
}

// WorkDir returns the working directory of the process.
func (p *Process) WorkDir() string {
	p.mu.RLock()
	proc := p.proc
	p.mu.RUnlock()

	if proc == nil {
		return ""
	}
	return proc.WorkDir()
}

// Cancel stops the underlying AI process.
func (p *Process) Cancel() error {
	p.mu.RLock()
	proc := p.proc
	p.mu.RUnlock()

	if proc != nil {
		return proc.Cancel()
	}
	return nil
}

// Wait blocks until the underlying process completes.
func (p *Process) Wait() error {
	p.mu.RLock()
	proc := p.proc
	p.mu.RUnlock()

	if proc != nil {
		return proc.Wait()
	}
	return nil
}

// SetRetired marks this process as retired.
func (p *Process) SetRetired(retired bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.isRetired = retired
}

// IsRetired returns true if this process has been retired.
func (p *Process) IsRetired() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.isRetired
}

// Events returns the underlying process's events channel for direct access.
// Use with caution - prefer using Process methods.
func (p *Process) Events() <-chan client.OutputEvent {
	p.mu.RLock()
	proc := p.proc
	p.mu.RUnlock()

	if proc == nil {
		// Return a closed channel
		ch := make(chan client.OutputEvent)
		close(ch)
		return ch
	}
	return proc.Events()
}

// Errors returns the underlying process's errors channel for direct access.
func (p *Process) Errors() <-chan error {
	p.mu.RLock()
	proc := p.proc
	p.mu.RUnlock()

	if proc == nil {
		// Return a closed channel
		ch := make(chan error)
		close(ch)
		return ch
	}
	return proc.Errors()
}

// Status returns the underlying process status.
func (p *Process) Status() client.ProcessStatus {
	p.mu.RLock()
	proc := p.proc
	p.mu.RUnlock()

	if proc == nil {
		return client.StatusPending
	}
	return proc.Status()
}

// PID returns the OS process ID of the underlying AI process.
// Returns 0 if the process is not running or not available.
func (p *Process) PID() int {
	p.mu.RLock()
	proc := p.proc
	p.mu.RUnlock()

	if proc == nil {
		return 0
	}
	return proc.PID()
}

type ContextExceededError struct{}

func (e *ContextExceededError) Error() string {
	return "context exceeded limit"
}
