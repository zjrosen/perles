// Package handler provides command handlers for the v2 orchestration architecture.
// This file implements StopWorkerHandler for terminating worker processes.
package handler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/process"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
)

// GracefulStopTimeout is the maximum time to wait for graceful termination
// before escalating to forceful termination.
const GracefulStopTimeout = 5 * time.Second

// StopWorkerHandler handles CmdStopProcess commands.
// It implements tiered termination: graceful (Cancel + timeout) then forceful.
// Follows the RetireProcessHandler pattern for lifecycle management.
type StopWorkerHandler struct {
	processRepo repository.ProcessRepository
	taskRepo    repository.TaskRepository
	queueRepo   repository.QueueRepository
	registry    *process.ProcessRegistry
}

// NewStopWorkerHandler creates a new StopWorkerHandler.
func NewStopWorkerHandler(
	processRepo repository.ProcessRepository,
	taskRepo repository.TaskRepository,
	queueRepo repository.QueueRepository,
	registry *process.ProcessRegistry,
) *StopWorkerHandler {
	return &StopWorkerHandler{
		processRepo: processRepo,
		taskRepo:    taskRepo,
		queueRepo:   queueRepo,
		registry:    registry,
	}
}

// Handle processes a StopProcessCommand.
// It implements tiered termination with phase-aware warnings.
func (h *StopWorkerHandler) Handle(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
	stopCmd := cmd.(*command.StopProcessCommand)

	// Get process from repository
	proc, err := h.processRepo.Get(stopCmd.ProcessID)
	if err != nil {
		if errors.Is(err, repository.ErrProcessNotFound) {
			return nil, ErrProcessNotFound
		}
		return nil, fmt.Errorf("failed to get process: %w", err)
	}

	// Handle idempotency: if already stopped or retired, return success
	if proc.Status == repository.StatusStopped || proc.Status == repository.StatusRetired {
		return SuccessResult(&StopWorkerResult{
			ProcessID: proc.ID,
			WasNoOp:   true,
		}), nil
	}

	// Phase-aware warning: if worker is in Committing phase and force is not set,
	// return a warning without terminating
	if proc.Phase != nil && *proc.Phase == events.ProcessPhaseCommitting && !stopCmd.Force {
		return SuccessWithEvents(&StopWorkerResult{
			ProcessID:    proc.ID,
			PhaseWarning: true,
			WasNoOp:      true,
		}, events.ProcessEvent{
			Type:      events.ProcessOutput,
			ProcessID: proc.ID,
			Role:      proc.Role,
			Output:    "Warning: Worker is in Committing phase. Use --force to terminate during commit, or wait for commit to complete.",
			TaskID:    proc.TaskID,
		}), nil
	}

	// Get live process from registry
	var liveProcess *process.Process
	if h.registry != nil {
		liveProcess = h.registry.Get(stopCmd.ProcessID)
	}

	// If process not in registry, just update repository
	if liveProcess == nil {
		return h.updateRepositoryAndCleanup(proc)
	}

	// Attempt graceful or forceful stop based on Force flag
	if stopCmd.Force {
		return h.forceStop(proc, liveProcess)
	}
	return h.gracefulStop(ctx, proc, liveProcess)
}

// gracefulStop attempts to stop the worker gracefully with a timeout.
// If the process doesn't respond within GracefulStopTimeout, it escalates to force stop.
func (h *StopWorkerHandler) gracefulStop(ctx context.Context, proc *repository.Process, liveProcess *process.Process) (*command.CommandResult, error) {
	// Try graceful cancellation
	_ = liveProcess.Cancel()

	// Wait with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, GracefulStopTimeout)
	defer cancel()

	done := make(chan struct{})
	go func() {
		_ = liveProcess.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Graceful shutdown succeeded
		return h.finishStop(proc, liveProcess, true)
	case <-timeoutCtx.Done():
		// Timeout - escalate to forceful
		return h.forceStop(proc, liveProcess)
	}
}

// forceStop immediately terminates the worker forcefully.
// On Unix, this sends SIGKILL. On Windows, this calls TerminateProcess.
func (h *StopWorkerHandler) forceStop(proc *repository.Process, liveProcess *process.Process) (*command.CommandResult, error) {
	pid := liveProcess.PID()
	if pid > 0 {
		// Force kill the process (platform-specific implementation)
		_ = killProcess(pid)
	}

	return h.finishStop(proc, liveProcess, false)
}

// finishStop completes the stop operation by updating state and emitting events.
func (h *StopWorkerHandler) finishStop(proc *repository.Process, liveProcess *process.Process, graceful bool) (*command.CommandResult, error) {
	// Stop the event loop (SetRetired + Stop follows RetireProcessHandler pattern)
	liveProcess.SetRetired(true)
	liveProcess.Stop()

	// Update repository and cleanup task
	return h.updateRepositoryAndCleanupWithResult(proc, graceful)
}

// updateRepositoryAndCleanup updates the repository to retired status and cleans up task state.
// Used when process is not in live registry.
func (h *StopWorkerHandler) updateRepositoryAndCleanup(proc *repository.Process) (*command.CommandResult, error) {
	return h.updateRepositoryAndCleanupWithResult(proc, true)
}

// updateRepositoryAndCleanupWithResult updates repository and task state, then returns result.
func (h *StopWorkerHandler) updateRepositoryAndCleanupWithResult(proc *repository.Process, graceful bool) (*command.CommandResult, error) {
	// Clean up task assignment if worker had a task
	if proc.TaskID != "" && h.taskRepo != nil {
		task, err := h.taskRepo.Get(proc.TaskID)
		if err == nil && task != nil {
			// Reset task status based on current state
			// If the task was being implemented by this worker, reset to implementing
			// If it was being reviewed, the implementer keeps their status
			if task.Implementer == proc.ID {
				// Clear implementer since the worker is being stopped
				task.Implementer = ""
				task.Status = repository.TaskImplementing
				_ = h.taskRepo.Save(task)
			} else if task.Reviewer == proc.ID {
				// Clear reviewer since the worker is being stopped
				task.Reviewer = ""
				// Keep task in review status, waiting for new reviewer
				_ = h.taskRepo.Save(task)
			}
		}
	}

	// Drain any queued messages for this worker
	var drainedCount int
	if h.queueRepo != nil {
		queue := h.queueRepo.GetOrCreate(proc.ID)
		drainedEntries := queue.Drain()
		drainedCount = len(drainedEntries)
	}

	// Clear TaskID from process
	oldTaskID := proc.TaskID
	proc.TaskID = ""

	// Update process status to stopped (can be resumed later)
	proc.Status = repository.StatusStopped

	if err := h.processRepo.Save(proc); err != nil {
		return nil, fmt.Errorf("failed to save process: %w", err)
	}

	// Build events to emit
	var resultEvents []any

	// Emit ProcessStatusChange event
	statusEvent := events.ProcessEvent{
		Type:      events.ProcessStatusChange,
		ProcessID: proc.ID,
		Role:      proc.Role,
		Status:    events.ProcessStatusStopped,
		TaskID:    oldTaskID, // Include old task ID for reference
	}
	resultEvents = append(resultEvents, statusEvent)

	// If we drained messages, emit ProcessQueueChanged so TUI updates the queue badge
	if drainedCount > 0 {
		queueEvent := events.ProcessEvent{
			Type:       events.ProcessQueueChanged,
			ProcessID:  proc.ID,
			Role:       proc.Role,
			Status:     events.ProcessStatusStopped,
			QueueCount: 0, // Queue is now empty
		}
		resultEvents = append(resultEvents, queueEvent)
	}

	result := &StopWorkerResult{
		ProcessID:    proc.ID,
		Graceful:     graceful,
		DrainedCount: drainedCount,
		WasNoOp:      false,
	}

	return SuccessWithEvents(result, resultEvents...), nil
}

// StopWorkerResult contains the result of stopping a worker process.
type StopWorkerResult struct {
	ProcessID    string
	Graceful     bool // true if stopped gracefully, false if force-killed
	PhaseWarning bool // true if returned due to Committing phase warning
	DrainedCount int  // number of queued messages that were drained
	WasNoOp      bool // true if process was already retired
}
