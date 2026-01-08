// Package processor provides the FIFO command processor for the v2 orchestration architecture.
// This file defines event types emitted by the processor for UI consumption.
package processor

import (
	"time"

	"github.com/zjrosen/perles/internal/orchestration/v2/command"
)

// CommandLogEvent is emitted after each command is processed for UI visibility.
// This event is consumed by the orchestration mode UI to display command activity
// in the command log pane.
type CommandLogEvent struct {
	// CommandID is the unique identifier of the processed command.
	CommandID string
	// CommandType indicates the type of command that was processed.
	CommandType command.CommandType
	// Source indicates where the command originated (mcp_tool, internal, callback, user).
	Source command.CommandSource
	// Success indicates whether the command executed successfully.
	Success bool
	// Error contains the error if the command failed (nil on success).
	Error error
	// Duration is how long the command took to execute.
	Duration time.Duration
	// Timestamp is when the command finished processing.
	Timestamp time.Time
	// TraceID is the distributed trace ID for correlation (empty if tracing disabled).
	TraceID string
}
