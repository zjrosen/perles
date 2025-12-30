package pool

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/zjrosen/perles/internal/orchestration/claude"
	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/metrics"
	"github.com/zjrosen/perles/internal/pubsub"
)

// WorkerStatus is an alias to events.WorkerStatus for backward compatibility.
// This allows existing code using pool.WorkerStatus to continue working.
type WorkerStatus = events.WorkerStatus

// Re-export status constants for backward compatibility.
const (
	WorkerReady   = events.WorkerReady
	WorkerWorking = events.WorkerWorking
	WorkerRetired = events.WorkerRetired
)

// Worker represents a single AI process in the worker pool.
// Workers are persistent and can be assigned multiple tasks over their lifetime.
type Worker struct {
	ID            string
	TaskID        string // Current task (empty if Ready)
	Process       client.HeadlessProcess
	SessionID     string
	Status        WorkerStatus
	Phase         events.WorkerPhase // Current workflow phase (idle, implementing, reviewing, etc.)
	StartedAt     time.Time          // When worker was spawned
	TaskStartedAt time.Time          // When current task was assigned
	Output        *OutputBuffer
	LastError     error
	metrics       *metrics.TokenMetrics // Current token metrics
	totalCostUSD  float64               // Accumulated cost across all turns
	mu            sync.RWMutex
}

// newWorker creates a new worker with the given ID.
// Worker starts in Working state as it will immediately process its initial prompt.
// Phase starts as Idle until a task is assigned.
func newWorker(id string, bufferCapacity int) *Worker {
	return &Worker{
		ID:        id,
		Status:    WorkerWorking,
		Phase:     events.PhaseIdle,
		StartedAt: time.Now(),
		Output:    NewOutputBuffer(bufferCapacity),
	}
}

// AssignTask assigns a task to a ready worker, transitioning to Working state.
func (w *Worker) AssignTask(taskID string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.Status != WorkerReady {
		return fmt.Errorf("worker %s is not ready (status: %s)", w.ID, w.Status)
	}

	w.TaskID = taskID
	w.Status = WorkerWorking
	w.TaskStartedAt = time.Now()
	w.Output.Clear() // Clear output buffer for new task
	return nil
}

// CompleteTask marks the current task as complete and returns to Ready state.
// Note: TaskID is preserved for display purposes as historical context.
func (w *Worker) CompleteTask() {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.Status = WorkerReady
	w.TaskStartedAt = time.Time{}
}

// Retire permanently shuts down the worker.
func (w *Worker) Retire() {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.Status = WorkerRetired
}

// GetStatus returns the current status thread-safely.
func (w *Worker) GetStatus() WorkerStatus {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.Status
}

// GetSessionID returns the session ID thread-safely.
func (w *Worker) GetSessionID() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.SessionID
}

// setSessionID updates the session ID thread-safely.
func (w *Worker) setSessionID(id string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.SessionID = id
}

// GetLastError returns the last error thread-safely.
func (w *Worker) GetLastError() error {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.LastError
}

// GetTaskID returns the current task ID thread-safely.
func (w *Worker) GetTaskID() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.TaskID
}

// GetPhase returns the current workflow phase thread-safely.
func (w *Worker) GetPhase() events.WorkerPhase {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.Phase
}

// SetPhase updates the workflow phase thread-safely.
func (w *Worker) SetPhase(phase events.WorkerPhase) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.Phase = phase
}

// SetTaskID updates the task ID thread-safely.
// This is used by the coordinator to set task ID (e.g., when assigning a reviewer).
func (w *Worker) SetTaskID(taskID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.TaskID = taskID
}

// GetStartedAt returns when the worker was spawned thread-safely.
func (w *Worker) GetStartedAt() time.Time {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.StartedAt
}

// setLastError updates the last error thread-safely.
func (w *Worker) setLastError(err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.LastError = err
}

// GetMetrics returns the current token metrics thread-safely.
func (w *Worker) GetMetrics() *metrics.TokenMetrics {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.metrics
}

// GetPID returns the PID of the worker's Claude process, or 0 if not running.
func (w *Worker) GetPID() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.Process != nil {
		return w.Process.PID()
	}
	return 0
}

// getContextWindow returns the context window size from the event or a default.
func (w *Worker) getContextWindow(event *client.OutputEvent) int {
	// Try to get from ModelUsage first (has ContextWindow field)
	for _, usage := range event.ModelUsage {
		if usage.ContextWindow > 0 {
			return usage.ContextWindow
		}
	}
	// Default context window for AI models
	return 200000
}

// Duration returns how long the worker has been alive.
func (w *Worker) Duration() time.Duration {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.StartedAt.IsZero() {
		return 0
	}
	return time.Since(w.StartedAt)
}

// Cancel stops the worker's Claude process and retires the worker.
func (w *Worker) Cancel() error {
	w.mu.Lock()
	proc := w.Process
	// Set status before unlock to prevent race with event loop
	w.Status = WorkerRetired
	w.TaskID = ""
	w.mu.Unlock()

	if proc != nil {
		return proc.Cancel()
	}
	return nil
}

// start begins processing events from the AI process and publishing them via the broker.
// It runs until the process completes or the context is cancelled.
// The worker starts in Working state (processing initial prompt) and transitions
// to Ready when AI completes its first turn.
func (w *Worker) start(ctx context.Context, proc client.HeadlessProcess, broker *pubsub.Broker[events.WorkerEvent]) {
	w.mu.Lock()
	w.Process = proc
	// Worker starts in Working state - AI is processing its initial prompt
	w.Status = WorkerWorking
	taskID := w.TaskID
	w.mu.Unlock()

	// Publish WorkerSpawned event (TUI uses this to initialize worker display)
	broker.Publish(pubsub.UpdatedEvent, events.WorkerEvent{
		WorkerID: w.ID,
		TaskID:   taskID,
		Type:     events.WorkerSpawned,
		Status:   WorkerWorking,
	})

	// Process events from AI
	w.processEvents(ctx, proc, broker)
}

// resume continues processing a worker that has been resumed via send_to_worker or assign_task.
// Sets worker to Working state while AI processes the message.
func (w *Worker) resume(ctx context.Context, proc client.HeadlessProcess, broker *pubsub.Broker[events.WorkerEvent]) {
	w.mu.Lock()
	w.Process = proc
	w.Status = WorkerWorking // Worker is now processing
	taskID := w.TaskID
	w.mu.Unlock()

	// Publish status change to Working
	broker.Publish(pubsub.UpdatedEvent, events.WorkerEvent{
		WorkerID: w.ID,
		TaskID:   taskID,
		Type:     events.WorkerStatusChange,
		Status:   WorkerWorking,
	})

	// Process events from AI
	w.processEvents(ctx, proc, broker)
}

// processEvents handles the event loop for an AI process.
// Shared by both start and resume to avoid duplication.
func (w *Worker) processEvents(ctx context.Context, proc client.HeadlessProcess, broker *pubsub.Broker[events.WorkerEvent]) {
	procEvents := proc.Events()
	procErrors := proc.Errors()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-procEvents:
			if !ok {
				// Events channel closed, process complete
				w.handleProcessComplete(proc, broker)
				return
			}
			w.handleOutputEvent(&event, broker)
		case err, ok := <-procErrors:
			if !ok {
				continue
			}
			w.handleError(err, broker)
		}
	}
}

// handleOutputEvent processes a single output event from the AI process.
func (w *Worker) handleOutputEvent(event *client.OutputEvent, broker *pubsub.Broker[events.WorkerEvent]) {
	// Extract session ID from init event
	if event.IsInit() && event.SessionID != "" {
		w.setSessionID(event.SessionID)
	}

	// Handle result events - may include errors (e.g., "Prompt is too long")
	if event.IsResult() {
		// Check for error results first (e.g., context window exceeded)
		if event.IsErrorResult {
			errMsg := event.GetErrorMessage()
			// Write error to output buffer so it appears in worker pane
			w.Output.Write("⚠️ Error: " + errMsg)
			// Also emit as error event for tracking
			w.handleError(fmt.Errorf("worker error: %s", errMsg), broker)
			return
		}

		// Extract token usage from successful result events
		if event.Usage != nil {
			// Build comprehensive TokenMetrics from event usage
			m := &metrics.TokenMetrics{
				InputTokens:              event.Usage.InputTokens,
				OutputTokens:             event.Usage.OutputTokens,
				CacheReadInputTokens:     event.Usage.CacheReadInputTokens,
				CacheCreationInputTokens: event.Usage.CacheCreationInputTokens,
				ContextTokens:            event.GetContextTokens(),
				ContextWindow:            w.getContextWindow(event),
				TurnCostUSD:              event.TotalCostUSD,
				LastUpdatedAt:            time.Now(),
			}

			// Accumulate total cost across turns
			w.mu.Lock()
			w.totalCostUSD += m.TurnCostUSD
			m.TotalCostUSD = w.totalCostUSD
			w.metrics = m
			w.mu.Unlock()

			// Emit token usage event with full metrics
			if m.ContextTokens > 0 {
				broker.Publish(pubsub.UpdatedEvent, events.WorkerEvent{
					Type:     events.WorkerTokenUsage,
					WorkerID: w.ID,
					TaskID:   w.TaskID,
					Metrics:  m,
				})
			}
		}
	}

	// Store text output in buffer and emit as output event
	if event.IsAssistant() && event.Message != nil {
		text := event.Message.GetText()
		if text != "" {
			w.Output.Write(text)
			broker.Publish(pubsub.UpdatedEvent, events.WorkerEvent{
				Type:     events.WorkerOutput,
				WorkerID: w.ID,
				TaskID:   w.TaskID,
				Output:   text,
				RawJSON:  event.Raw,
			})
		}

		// Also emit tool calls for visibility
		for i := range event.Message.Content {
			block := &event.Message.Content[i]
			if block.Type == "tool_use" && block.Name != "" {
				toolMsg := claude.FormatToolDisplay(block)
				broker.Publish(pubsub.UpdatedEvent, events.WorkerEvent{
					Type:     events.WorkerOutput,
					WorkerID: w.ID,
					TaskID:   w.TaskID,
					Output:   toolMsg,
				})
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
			w.Output.Write("[" + event.Tool.Name + "] " + output)
		}
	}
}

// handleError processes an error from the Claude process.
func (w *Worker) handleError(err error, broker *pubsub.Broker[events.WorkerEvent]) {
	w.setLastError(err)
	broker.Publish(pubsub.UpdatedEvent, events.WorkerEvent{
		WorkerID: w.ID,
		TaskID:   w.TaskID,
		Type:     events.WorkerError,
		Error:    err,
	})
}

// handleProcessComplete is called when the AI process finishes a turn.
// In the pool model, workers return to Ready state after completing a response.
// Workers are only Retired explicitly or on fatal errors.
func (w *Worker) handleProcessComplete(proc client.HeadlessProcess, broker *pubsub.Broker[events.WorkerEvent]) {
	// Wait for process to fully complete
	_ = proc.Wait()

	w.mu.Lock()
	// Don't change status if already retired
	if w.Status == WorkerRetired {
		w.mu.Unlock()
		return
	}

	// Determine final status based on process status
	var finalStatus WorkerStatus
	switch proc.Status() {
	case client.StatusCompleted:
		// Process completed normally - worker returns to Ready
		finalStatus = WorkerReady
		// TaskID is intentionally NOT cleared - coordinator manages task ID lifecycle
		// This allows TUI to display task context through phase transitions
		w.TaskStartedAt = time.Time{}
	case client.StatusCancelled:
		// Cancelled - retire the worker
		finalStatus = WorkerRetired
		w.TaskID = ""
	default:
		// Failed - retire the worker
		finalStatus = WorkerRetired
		w.TaskID = ""
	}

	w.Status = finalStatus
	currentStatus := w.Status
	taskID := w.TaskID
	w.mu.Unlock()

	broker.Publish(pubsub.UpdatedEvent, events.WorkerEvent{
		WorkerID: w.ID,
		TaskID:   taskID,
		Type:     events.WorkerStatusChange,
		Status:   currentStatus,
	})
}
