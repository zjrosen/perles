// Package command provides the foundational types for the v2 orchestration architecture.
// This package defines the Command interface, CommandType constants, and BaseCommand
// struct that all v2 commands implement.
package command

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// Command represents an explicit intent entering the orchestration system.
// All commands must implement this interface to be processed by the FIFO processor.
type Command interface {
	// ID returns unique command identifier for tracing/correlation
	ID() string
	// Type returns the command type for routing to handlers
	Type() CommandType
	// Validate checks command preconditions before execution
	Validate() error
	// Priority returns execution priority (0=normal, 1=urgent)
	Priority() int
	// CreatedAt returns when command was created
	CreatedAt() time.Time
}

// CommandType identifies the kind of command for handler routing.
type CommandType string

const (
	// Task Assignment Commands

	// CmdAssignTask assigns a bd task to an implementer.
	CmdAssignTask CommandType = "assign_task"
	// CmdAssignReview assigns a reviewer to an implemented task.
	CmdAssignReview CommandType = "assign_review"
	// CmdApproveCommit approves implementation and triggers commit phase.
	CmdApproveCommit CommandType = "approve_commit"
	// CmdAssignReviewFeedback sends review feedback to an implementer after denial.
	CmdAssignReviewFeedback CommandType = "assign_review_feedback"

	// Message Routing Commands

	// CmdBroadcast broadcasts a message to all workers.
	CmdBroadcast CommandType = "broadcast"

	// State Transition Commands

	// CmdReportComplete signals that a worker's implementation is done.
	CmdReportComplete CommandType = "report_complete"
	// CmdReportVerdict signals a reviewer's approval or denial verdict.
	CmdReportVerdict CommandType = "report_verdict"
	// CmdTransitionPhase is an internal command for phase changes.
	CmdTransitionPhase CommandType = "transition_phase"
	// BD Task Status Commands

	// CmdMarkTaskComplete marks a BD task as completed.
	CmdMarkTaskComplete CommandType = "mark_task_complete"
	// CmdMarkTaskFailed marks a BD task as failed with a reason.
	CmdMarkTaskFailed CommandType = "mark_task_failed"

	// Unified Process Commands (for both coordinator and workers)

	// CmdSpawnProcess creates a new process (worker or coordinator).
	CmdSpawnProcess CommandType = "spawn_process"
	// CmdRetireProcess terminates a process gracefully.
	CmdRetireProcess CommandType = "retire_process"
	// CmdReplaceProcess retires and respawns a process with fresh context.
	CmdReplaceProcess CommandType = "replace_process"
	// CmdSendToProcess sends a message to a process (coordinator or worker).
	CmdSendToProcess CommandType = "send_to_process"
	// CmdDeliverProcessQueued delivers queued messages to a process.
	CmdDeliverProcessQueued CommandType = "deliver_process_queued"
	// CmdProcessTurnComplete signals a process's AI turn finished.
	CmdProcessTurnComplete CommandType = "process_turn_complete"
	// CmdPauseProcess pauses a coordinator/process (Ready/Working → Paused).
	CmdPauseProcess CommandType = "pause_process"
	// CmdResumeProcess resumes a paused coordinator/process (Paused → Ready).
	CmdResumeProcess CommandType = "resume_process"

	// Aggregation Commands

	// CmdGenerateAccountabilitySummary spawns a worker to aggregate accountability summaries.
	CmdGenerateAccountabilitySummary CommandType = "generate_accountability_summary"

	// Process Control Commands

	// CmdStopProcess stops a process (coordinator or worker) with optional forceful termination.
	CmdStopProcess CommandType = "stop_process"

	// Workflow Lifecycle Commands

	// CmdSignalWorkflowComplete signals that the workflow has completed.
	CmdSignalWorkflowComplete CommandType = "signal_workflow_complete"

	// User Interaction Commands

	// CmdNotifyUser requests user attention (e.g., for human review checkpoints).
	CmdNotifyUser CommandType = "notify_user"
)

// String returns the string representation of the CommandType.
func (ct CommandType) String() string {
	return string(ct)
}

// CommandSource identifies where the command originated.
type CommandSource string

const (
	// SourceMCPTool indicates the command came from an MCP tool call.
	SourceMCPTool CommandSource = "mcp_tool"
	// SourceInternal indicates the command was system-generated (e.g., queue drain).
	SourceInternal CommandSource = "internal"
	// SourceCallback indicates the command came from a worker state callback.
	SourceCallback CommandSource = "callback"
	// SourceUser indicates the command came from direct user input (TUI).
	SourceUser CommandSource = "user"
)

// String returns the string representation of the CommandSource.
func (cs CommandSource) String() string {
	return string(cs)
}

// BaseCommand provides common fields for all commands.
// Concrete command types should embed this struct.
type BaseCommand struct {
	id          string
	cmdType     CommandType
	priority    int
	createdAt   time.Time
	source      CommandSource
	traceID     string
	spanContext trace.SpanContext // For OpenTelemetry trace propagation
}

// NewBaseCommand creates a BaseCommand with a generated UUID and current timestamp.
func NewBaseCommand(cmdType CommandType, source CommandSource) BaseCommand {
	return BaseCommand{
		id:        uuid.New().String(),
		cmdType:   cmdType,
		priority:  0,
		createdAt: time.Now(),
		source:    source,
		traceID:   "",
	}
}

// ID returns the unique command identifier.
func (b *BaseCommand) ID() string {
	return b.id
}

// Type returns the command type for handler routing.
func (b *BaseCommand) Type() CommandType {
	return b.cmdType
}

// Priority returns the execution priority (0=normal, 1=urgent).
func (b *BaseCommand) Priority() int {
	return b.priority
}

// CreatedAt returns when the command was created.
func (b *BaseCommand) CreatedAt() time.Time {
	return b.createdAt
}

// Source returns the origin of this command.
func (b *BaseCommand) Source() CommandSource {
	return b.source
}

// TraceID returns the correlation ID for related commands.
// If a valid SpanContext is set, the trace ID is derived from it.
// Otherwise, falls back to the manually set traceID string.
func (b *BaseCommand) TraceID() string {
	if b.spanContext.IsValid() {
		return b.spanContext.TraceID().String()
	}
	return b.traceID
}

// SetTraceID sets the correlation ID for command tracing.
// This is used when receiving trace IDs as strings (e.g., from MCP tool calls).
// When a SpanContext is set, TraceID() will prefer the SpanContext's trace ID.
func (b *BaseCommand) SetTraceID(traceID string) {
	b.traceID = traceID
}

// SpanContext returns the OpenTelemetry span context for trace propagation.
func (b *BaseCommand) SpanContext() trace.SpanContext {
	return b.spanContext
}

// SetSpanContext sets the OpenTelemetry span context for trace propagation.
// This also clears the manual traceID since it will be derived from SpanContext.
func (b *BaseCommand) SetSpanContext(sc trace.SpanContext) {
	b.spanContext = sc
}

// SetPriority sets the execution priority.
func (b *BaseCommand) SetPriority(priority int) {
	b.priority = priority
}

// Validate is a no-op for BaseCommand. Concrete commands should override this.
func (b *BaseCommand) Validate() error {
	return nil
}

// CommandResult contains the outcome of command execution.
type CommandResult struct {
	// Success indicates whether the command executed successfully.
	Success bool
	// Events contains events to emit (for UI updates, etc.).
	Events []any
	// FollowUp contains commands to enqueue after the current one.
	FollowUp []Command
	// Error contains the error if Success is false.
	Error error
	// Data contains optional result data for the caller.
	Data any
}

// ErrQueueFull is returned when the command queue has reached capacity.
var ErrQueueFull = errors.New("command queue is full")
