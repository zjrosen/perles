package events

import "github.com/zjrosen/perles/internal/orchestration/metrics"

// WorkerStatus represents the current state of a worker in the pool.
// Note: This mirrors pool.WorkerStatus to avoid import cycle.
type WorkerStatus int

const (
	// WorkerReady means the worker is idle and waiting for a task assignment.
	WorkerReady WorkerStatus = iota
	// WorkerWorking means the worker is actively processing a task.
	WorkerWorking
	// WorkerRetired means the worker has been shut down (terminal state).
	WorkerRetired
)

// WorkerPhase represents what workflow step a worker is in.
// This is orthogonal to WorkerStatus - a worker can be "Working" in various phases.
type WorkerPhase string

const (
	// PhaseIdle means the worker is ready for assignment.
	PhaseIdle WorkerPhase = "idle"
	// PhaseImplementing means the worker is actively coding a task.
	PhaseImplementing WorkerPhase = "implementing"
	// PhaseAwaitingReview means implementation is done, waiting for a reviewer.
	PhaseAwaitingReview WorkerPhase = "awaiting_review"
	// PhaseReviewing means the worker is reviewing another worker's code.
	PhaseReviewing WorkerPhase = "reviewing"
	// PhaseAddressingFeedback means the worker is fixing issues from a review denial.
	PhaseAddressingFeedback WorkerPhase = "addressing_feedback"
	// PhaseCommitting means the worker is creating a git commit.
	PhaseCommitting WorkerPhase = "committing"
)

func (s WorkerStatus) String() string {
	switch s {
	case WorkerReady:
		return "ready"
	case WorkerWorking:
		return "working"
	case WorkerRetired:
		return "retired"
	default:
		return "unknown"
	}
}

// IsDone returns true if the worker is in a terminal state.
// Only Retired is terminal - Ready and Working workers can receive new tasks.
func (s WorkerStatus) IsDone() bool {
	return s == WorkerRetired
}

// IsActive returns true if the worker can receive tasks or is working.
func (s WorkerStatus) IsActive() bool {
	return s == WorkerReady || s == WorkerWorking
}

// WorkerEventType identifies the kind of worker event.
type WorkerEventType string

const (
	// WorkerSpawned is emitted when a new worker is created.
	WorkerSpawned WorkerEventType = "spawned"
	// WorkerOutput is emitted when a worker produces output.
	WorkerOutput WorkerEventType = "output"
	// WorkerStatusChange is emitted when a worker's status changes.
	WorkerStatusChange WorkerEventType = "status_change"
	// WorkerTokenUsage is emitted when a worker's token usage is updated.
	WorkerTokenUsage WorkerEventType = "token_usage"
	// WorkerIncoming is emitted when a worker receives a message.
	WorkerIncoming WorkerEventType = "incoming"
	// WorkerError is emitted when a worker encounters an error.
	WorkerError WorkerEventType = "error"
)

// WorkerEvent represents an event from a worker process.
type WorkerEvent struct {
	// Type identifies the kind of event.
	Type WorkerEventType
	// WorkerID identifies which worker emitted the event.
	WorkerID string
	// TaskID is the current task the worker is processing.
	TaskID string
	// Output contains the message text for output events.
	Output string
	// Status contains the worker's status for status change events.
	Status WorkerStatus
	// Phase contains the worker's workflow phase for status change events.
	Phase WorkerPhase
	// Metrics contains token usage and cost data for token usage events.
	Metrics *metrics.TokenMetrics
	// Message contains the incoming message text for incoming events.
	Message string
	// Error contains the error for error events.
	Error error
	// RawJSON contains the raw Claude API JSON response for session logging.
	// This is only populated for WorkerOutput events.
	RawJSON []byte `json:"raw_json,omitempty"`
}
