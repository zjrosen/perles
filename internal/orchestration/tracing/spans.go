package tracing

// Span attribute keys for orchestration tracing.
// These constants define the semantic conventions for span attributes
// in the orchestration system.
const (
	// Command attributes
	AttrCommandID       = "command.id"
	AttrCommandType     = "command.type"
	AttrCommandPriority = "command.priority"
	AttrCommandSource   = "command.source"

	// Process attributes
	AttrProcessID   = "process.id"
	AttrProcessRole = "process.role"

	// Worker attributes
	AttrWorkerID    = "worker.id"
	AttrWorkerPhase = "worker.phase"

	// Task attributes
	AttrTaskID = "task.id"

	// MCP attributes
	AttrMCPToolName   = "mcp.tool.name"
	AttrMCPRequestID  = "mcp.request.id"
	AttrMCPCallerRole = "mcp.caller.role"
	AttrMCPCallerID   = "mcp.caller.id"

	// Session attributes
	AttrSessionID    = "session.id"
	AttrWorkflowName = "workflow.name"
	AttrClientType   = "client.type"

	// Review attributes
	AttrVerdict       = "review.verdict"
	AttrReviewerID    = "reviewer.id"
	AttrImplementerID = "implementer.id"

	// Error attributes
	AttrErrorMessage = "error.message"
	AttrErrorType    = "error.type"
)

// SpanKind constants for categorizing span types.
const (
	SpanKindCommand  = "command"
	SpanKindHandler  = "handler"
	SpanKindMCP      = "mcp"
	SpanKindWorker   = "worker"
	SpanKindSession  = "session"
	SpanKindWorkflow = "workflow"
)

// Span name prefixes for consistent naming.
const (
	SpanPrefixCommand = "command.process."
	SpanPrefixHandler = "handler."
	SpanPrefixMCP     = "mcp.tool."
	SpanPrefixWorker  = "worker."
	SpanPrefixRepo    = "repo."
)

// Event names for span events.
const (
	EventCommandValidated = "command.validated"
	EventHandlerStarted   = "handler.started"
	EventRepositoryQuery  = "repository.query"
	EventMessageQueued    = "message.queued"
	EventMessageDelivered = "message.delivered"
	EventErrorOccurred    = "error.occurred"
	EventFollowUpCreated  = "follow_up.created"

	// Handler-specific events
	EventWorkerLookup  = "worker.lookup"
	EventTaskValidated = "task.validated"
	EventTaskAssigned  = "task.assigned"
)
