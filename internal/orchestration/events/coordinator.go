package events

import "github.com/zjrosen/perles/internal/orchestration/metrics"

// CoordinatorEventType identifies the kind of coordinator event.
type CoordinatorEventType string

const (
	// CoordinatorChat is emitted when the coordinator outputs text.
	CoordinatorChat CoordinatorEventType = "chat"
	// CoordinatorStatusChange is emitted when the coordinator's status changes.
	CoordinatorStatusChange CoordinatorEventType = "status_change"
	// CoordinatorTokenUsage is emitted when token usage is updated.
	CoordinatorTokenUsage CoordinatorEventType = "token_usage"
	// CoordinatorError is emitted when the coordinator encounters an error.
	CoordinatorError CoordinatorEventType = "error"
	// CoordinatorReady is emitted when the coordinator becomes ready for input.
	CoordinatorReady CoordinatorEventType = "ready"
	// CoordinatorWorking is emitted when the coordinator starts processing.
	CoordinatorWorking CoordinatorEventType = "working"
)

// CoordinatorEvent represents an event from the coordinator process.
type CoordinatorEvent struct {
	// Type identifies the kind of event.
	Type CoordinatorEventType
	// Role identifies the source ("coordinator", "system").
	Role string
	// Content contains the message text for chat events.
	Content string
	// Status contains the new status for status change events.
	Status CoordinatorStatus
	// Metrics contains token usage and cost data for token usage events.
	Metrics *metrics.TokenMetrics
	// Error contains the error for error events.
	Error error
	// RawJSON contains the raw Claude API JSON response for session logging.
	// This is only populated for CoordinatorChat events.
	RawJSON []byte `json:"raw_json,omitempty"`
}

// CoordinatorStatus represents the coordinator's operational state.
type CoordinatorStatus string

const (
	// StatusReady means the coordinator is waiting for user input.
	StatusReady CoordinatorStatus = "ready"
	// StatusWorking means the coordinator is processing.
	StatusWorking CoordinatorStatus = "working"
	// StatusPaused means the coordinator is temporarily paused.
	StatusPaused CoordinatorStatus = "paused"
	// StatusStopped means the coordinator has stopped.
	StatusStopped CoordinatorStatus = "stopped"
)
