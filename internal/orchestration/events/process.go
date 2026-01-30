package events

import (
	"github.com/zjrosen/perles/internal/orchestration/metrics"
)

// ProcessEventType identifies the kind of process event.
type ProcessEventType string

const (
	// ProcessSpawned is emitted when a new process is created.
	ProcessSpawned ProcessEventType = "spawned"
	// ProcessOutput is emitted when a process produces output.
	ProcessOutput ProcessEventType = "output"
	// ProcessStatusChange is emitted when a process's status changes.
	ProcessStatusChange ProcessEventType = "status_change"
	// ProcessTokenUsage is emitted when a process's token usage is updated.
	ProcessTokenUsage ProcessEventType = "token_usage"
	// ProcessIncoming is emitted when a process receives a message.
	ProcessIncoming ProcessEventType = "incoming"
	// ProcessError is emitted when a process encounters an error.
	ProcessError ProcessEventType = "error"
	// ProcessQueueChanged is emitted when a process's message queue changes.
	ProcessQueueChanged ProcessEventType = "queue_changed"
	// ProcessReady is emitted when a process becomes ready for input.
	ProcessReady ProcessEventType = "ready"
	// ProcessWorking is emitted when a process starts processing.
	ProcessWorking ProcessEventType = "working"
	// ProcessWorkflowComplete is emitted when a workflow completes.
	ProcessWorkflowComplete ProcessEventType = "workflow_complete"
	// ProcessAutoRefreshRequired is emitted when a coordinator's context is exhausted
	// and an automatic refresh is needed. This event is for TUI notification only;
	// the actual refresh is triggered by the handler layer.
	ProcessAutoRefreshRequired ProcessEventType = "auto_refresh_required"
	// ProcessUserNotification is emitted when the coordinator requests user attention.
	// This is used for human checkpoints in DAG workflows (e.g., clarification review).
	ProcessUserNotification ProcessEventType = "user_notification"
)

// ProcessRole identifies what kind of process this is.
// This is the canonical definition - repository.ProcessRole is a type alias to this.
type ProcessRole string

const (
	// RoleCoordinator is the singleton orchestrating process.
	RoleCoordinator ProcessRole = "coordinator"
	// RoleWorker is a task-executing process.
	RoleWorker ProcessRole = "worker"
	// RoleObserver is a passive monitoring process that subscribes to all channels.
	RoleObserver ProcessRole = "observer"
)

// ProcessStatus represents the process's current operational state.
// This is the canonical definition - repository.ProcessStatus is a type alias to this.
type ProcessStatus string

const (
	// ProcessStatusPending means the process is created but not yet started.
	ProcessStatusPending ProcessStatus = "pending"
	// ProcessStatusStarting means the process is in the process of starting up.
	ProcessStatusStarting ProcessStatus = "starting"
	// ProcessStatusReady means the process is idle and waiting for input.
	ProcessStatusReady ProcessStatus = "ready"
	// ProcessStatusWorking means the process is actively processing a turn.
	ProcessStatusWorking ProcessStatus = "working"
	// ProcessStatusPaused means the process has been temporarily paused by user request.
	ProcessStatusPaused ProcessStatus = "paused"
	// ProcessStatusStopped means the process has been stopped by user request (can be resumed).
	ProcessStatusStopped ProcessStatus = "stopped"
	// ProcessStatusRetiring means the process is being replaced and will retire after handoff.
	// This is an intermediate state during coordinator replacement (spawn-before-retire pattern).
	// The process is still active and can complete its current work, but will transition to
	// Retired once the replacement is ready.
	ProcessStatusRetiring ProcessStatus = "retiring"
	// ProcessStatusRetired means the process has been gracefully shut down (terminal state).
	ProcessStatusRetired ProcessStatus = "retired"
	// ProcessStatusFailed means the process encountered an error (terminal state).
	ProcessStatusFailed ProcessStatus = "failed"
)

// IsTerminal returns true if this is a terminal status (Retired or Failed).
// Paused is NOT terminal - it can transition back to Ready or Working.
func (s ProcessStatus) IsTerminal() bool {
	return s == ProcessStatusRetired || s == ProcessStatusFailed
}

// ProcessEvent represents an event from any process (coordinator or worker).
// Subscribers filter by Role field to route events appropriately.
type ProcessEvent struct {
	// Type identifies the kind of event.
	Type ProcessEventType
	// ProcessID identifies which process emitted the event.
	ProcessID string
	// Role indicates whether this is from coordinator or worker.
	Role ProcessRole
	// Output contains the message text for output events.
	Output string
	// Delta indicates this is a streaming chunk that should be accumulated
	// with the previous message rather than displayed as a new message.
	Delta bool
	// Status contains the process's status for status change events.
	Status ProcessStatus
	// Phase contains the worker's workflow phase (nil for coordinator).
	Phase *ProcessPhase
	// TaskID is the current task (empty for coordinator).
	TaskID string
	// Metrics contains token usage and cost data.
	Metrics *metrics.TokenMetrics
	// Message contains incoming message text.
	Message string
	// Sender identifies who sent the message (for ProcessIncoming events).
	// Empty string means unknown/legacy, "user" means TUI, "coordinator" means coordinator process.
	Sender string
	// Error contains the error for error events.
	Error error
	// RawJSON contains raw API response for logging.
	RawJSON []byte `json:"raw_json,omitempty"`
	// QueueCount contains pending messages in queue.
	QueueCount int `json:"queue_count,omitempty"`
}

// IsCoordinator returns true if this event is from the coordinator.
func (e ProcessEvent) IsCoordinator() bool {
	return e.Role == RoleCoordinator
}

// IsWorker returns true if this event is from a worker.
func (e ProcessEvent) IsWorker() bool {
	return e.Role == RoleWorker
}

// IsObserver returns true if this event is from the observer.
func (e ProcessEvent) IsObserver() bool {
	return e.Role == RoleObserver
}

// ProcessPhase represents what workflow step a worker process is in.
// This is orthogonal to ProcessStatus - a process can be "Working" in various phases.
// Used in ProcessEvent.Phase to track workflow state for worker processes.
// For coordinator processes, Phase is always nil.
type ProcessPhase string

const (
	// ProcessPhaseIdle means the worker is ready for assignment.
	ProcessPhaseIdle ProcessPhase = "idle"
	// ProcessPhaseImplementing means the worker is actively coding a task.
	ProcessPhaseImplementing ProcessPhase = "implementing"
	// ProcessPhaseAwaitingReview means implementation is done, waiting for a reviewer.
	ProcessPhaseAwaitingReview ProcessPhase = "awaiting_review"
	// ProcessPhaseReviewing means the worker is reviewing another worker's code.
	ProcessPhaseReviewing ProcessPhase = "reviewing"
	// ProcessPhaseAddressingFeedback means the worker is fixing issues from a review denial.
	ProcessPhaseAddressingFeedback ProcessPhase = "addressing_feedback"
	// ProcessPhaseCommitting means the worker is creating a git commit.
	ProcessPhaseCommitting ProcessPhase = "committing"
)

// IsDone returns true if the process is in a terminal state (retired or failed).
func (s ProcessStatus) IsDone() bool {
	return s == ProcessStatusRetired || s == ProcessStatusFailed
}

// IsActive returns true if the process can receive tasks or is working.
func (s ProcessStatus) IsActive() bool {
	return s == ProcessStatusReady || s == ProcessStatusWorking
}
