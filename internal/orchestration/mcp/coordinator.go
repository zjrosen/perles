package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"go.opentelemetry.io/otel/trace"

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/fabric"
	fabricmcp "github.com/zjrosen/perles/internal/orchestration/fabric/mcp"
	"github.com/zjrosen/perles/internal/orchestration/v2/adapter"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
	"github.com/zjrosen/perles/internal/orchestration/validation"
	taskpkg "github.com/zjrosen/perles/internal/task"
)

// CoordinatorServer is an MCP server that exposes orchestration tools to the coordinator agent.
// It provides tools for spawning workers, managing tasks, and communicating via Fabric channels.
type CoordinatorServer struct {
	*Server
	workDir       string
	port          int                  // HTTP server port for MCP config generation
	beadsExecutor taskpkg.TaskExecutor // BD command executor

	// dedup tracks recent messages to prevent duplicate sends to workers
	dedup *MessageDeduplicator

	// V2 adapter for command-based processing
	// See docs/proposals/orchestration-v2-architecture.md for architecture details
	v2Adapter *adapter.V2Adapter

	// fabricService provides graph-based messaging for task assignments
	fabricService *fabric.Service
}

// NewCoordinatorServer creates a new coordinator MCP server.
// The port is the HTTP server port used for MCP config generation.
// The beadsExec parameter is required and must not be nil.
func NewCoordinatorServer(
	workDir string,
	port int,
	beadsExec taskpkg.TaskExecutor,
) *CoordinatorServer {
	return NewCoordinatorServerWithV2Adapter(workDir, port, beadsExec, nil)
}

// NewCoordinatorServerWithV2Adapter creates a new coordinator MCP server with a v2 adapter.
// The v2 adapter handles command-based processing for all orchestration operations.
func NewCoordinatorServerWithV2Adapter(
	workDir string,
	port int,
	beadsExec taskpkg.TaskExecutor,
	v2Adapter *adapter.V2Adapter,
) *CoordinatorServer {
	cs := &CoordinatorServer{
		Server:        NewServer("perles-orchestrator", "1.0.0", WithInstructions(coordinatorInstructions)),
		workDir:       workDir,
		port:          port,
		beadsExecutor: beadsExec,
		dedup:         NewMessageDeduplicator(DefaultDeduplicationWindow),
		v2Adapter:     v2Adapter,
	}

	cs.registerTools()
	return cs
}

// coordinatorInstructions provides a brief description for the MCP server.
// Detailed instructions are in the coordinator's system prompt (see prompt.go).
const coordinatorInstructions = `Perles orchestrator MCP server providing worker management and task coordination tools.`

// SetV2Adapter allows setting the v2 adapter after construction.
// This is useful for testing and for setting up the adapter after initialization.
func (cs *CoordinatorServer) SetV2Adapter(adapter *adapter.V2Adapter) {
	cs.v2Adapter = adapter
}

// SetTracer sets the tracer for distributed tracing of MCP tool calls.
// This delegates to the embedded Server's tracer field.
func (cs *CoordinatorServer) SetTracer(tracer trace.Tracer) {
	cs.tracer = tracer
}

// SetFabricService registers Fabric messaging tools with the coordinator MCP server.
// This enables the coordinator to use fabric_inbox, fabric_send, fabric_reply, etc.
// The agentID is set to "coordinator" for proper message tracking.
// Also stores the service reference for assign_task to post to #tasks.
func (cs *CoordinatorServer) SetFabricService(svc *fabric.Service) {
	cs.fabricService = svc
	handlers := fabricmcp.NewHandlers(svc, repository.CoordinatorID)
	registerFabricTools(cs.Server, handlers)
}

// registerFabricTools registers all Fabric MCP tools with an MCP server.
// This bridges the fabric/mcp types to orchestration/mcp types.
func registerFabricTools(server *Server, h *fabricmcp.Handlers) {
	for _, tool := range fabricmcp.FabricTools() {
		// Convert fabric/mcp.Tool to mcp.Tool
		mcpTool := Tool{
			Name:        tool.Name,
			Description: tool.Description,
		}
		if tool.InputSchema != nil {
			mcpTool.InputSchema = convertInputSchema(tool.InputSchema)
		}
		if tool.OutputSchema != nil {
			mcpTool.OutputSchema = convertOutputSchema(tool.OutputSchema)
		}

		// Get the handler for this tool
		var handler ToolHandler
		switch tool.Name {
		case "fabric_inbox":
			handler = h.HandleInbox
		case "fabric_send":
			handler = h.HandleSend
		case "fabric_reply":
			handler = h.HandleReply
		case "fabric_ack":
			handler = h.HandleAck
		case "fabric_subscribe":
			handler = h.HandleSubscribe
		case "fabric_unsubscribe":
			handler = h.HandleUnsubscribe
		case "fabric_attach":
			handler = h.HandleAttach
		case "fabric_history":
			handler = h.HandleHistory
		case "fabric_read_thread":
			handler = h.HandleReadThread
		case "fabric_react":
			handler = h.HandleReact
		}

		if handler != nil {
			server.RegisterTool(mcpTool, handler)
		}
	}
}

func convertInputSchema(in *fabricmcp.InputSchema) *InputSchema {
	if in == nil {
		return nil
	}
	return &InputSchema{
		Type:       in.Type,
		Properties: convertProperties(in.Properties),
		Required:   in.Required,
	}
}

func convertOutputSchema(out *fabricmcp.OutputSchema) *OutputSchema {
	if out == nil {
		return nil
	}
	return &OutputSchema{
		Type:       out.Type,
		Properties: convertProperties(out.Properties),
		Required:   out.Required,
		Items:      convertPropertySchema(out.Items),
	}
}

func convertProperties(props map[string]*fabricmcp.PropertySchema) map[string]*PropertySchema {
	if props == nil {
		return nil
	}
	result := make(map[string]*PropertySchema, len(props))
	for k, v := range props {
		result[k] = convertPropertySchema(v)
	}
	return result
}

func convertPropertySchema(p *fabricmcp.PropertySchema) *PropertySchema {
	if p == nil {
		return nil
	}
	return &PropertySchema{
		Type:        p.Type,
		Description: p.Description,
		Properties:  convertProperties(p.Properties),
		Items:       convertPropertySchema(p.Items),
		Required:    p.Required,
		Enum:        p.Enum,
	}
}

// registerTools registers all coordinator tools with the MCP server.
// In prompt mode, task-related tools (assign_task, get_task_status, mark_task_complete, mark_task_failed) are excluded.
func (cs *CoordinatorServer) registerTools() {
	cs.RegisterTool(Tool{
		Name:        "spawn_worker",
		Description: "Spawn a new idle worker. The worker starts in Ready state waiting for task assignment. Returns the new worker ID. Optionally specify agent_type for specialized agents.",
		InputSchema: &InputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"agent_type": {
					Type:        "string",
					Description: "Optional agent specialization: 'implementer' (code implementation), 'reviewer' (code review), 'researcher' (codebase exploration). Defaults to generic if omitted.",
					Enum:        []string{"implementer", "reviewer", "researcher"},
				},
			},
			Required: []string{},
		},
	}, cs.handleSpawnWorker)

	cs.RegisterTool(Tool{
		Name:        "assign_task",
		Description: "Assign a task to a ready worker. Fetches task details from bd and sends to the worker.",
		InputSchema: &InputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"worker_id": {Type: "string", Description: "The worker ID to assign (e.g., 'worker-1')"},
				"task_id":   {Type: "string", Description: "The bd task ID to work on (e.g., 'perles-abc.1')"},
				"summary":   {Type: "string", Description: "Optional detailed instructions or context to include with the task assignment. Use for task-specific guidance, key files to modify, or implementation hints."},
			},
			Required: []string{"worker_id", "task_id"},
		},
	}, cs.handleAssignTask)

	cs.RegisterTool(Tool{
		Name:        "replace_worker",
		Description: "Retire a worker (e.g., due to token limit) and spawn a fresh replacement. Returns the new worker ID.",
		InputSchema: &InputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"worker_id": {Type: "string", Description: "The worker ID to retire"},
				"reason":    {Type: "string", Description: "Reason for replacement (e.g., 'token limit', 'stuck')"},
			},
			Required: []string{"worker_id"},
		},
	}, cs.handleReplaceWorker)

	cs.RegisterTool(Tool{
		Name:        "retire_worker",
		Description: "Retires a worker",
		InputSchema: &InputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"worker_id": {Type: "string", Description: "The worker ID to retire"},
				"reason":    {Type: "string", Description: "Reason for replacement (e.g., 'token limit', 'stuck')"},
			},
			Required: []string{"worker_id"},
		},
	}, cs.handleRetireWorker)

	cs.RegisterTool(Tool{
		Name:        "get_task_status",
		Description: "Get the current status of a task from the bd tracker.",
		InputSchema: &InputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"task_id": {Type: "string", Description: "The bd task ID to check"},
			},
			Required: []string{"task_id"},
		},
	}, cs.handleGetTaskStatus)

	cs.RegisterTool(Tool{
		Name:        "mark_task_complete",
		Description: "Mark a task as completed in the bd tracker.",
		InputSchema: &InputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"task_id": {Type: "string", Description: "The bd task ID to mark complete"},
			},
			Required: []string{"task_id"},
		},
	}, cs.handleMarkTaskComplete)

	cs.RegisterTool(Tool{
		Name:        "mark_task_failed",
		Description: "Mark a task as blocked or failed in the bd tracker.",
		InputSchema: &InputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"task_id": {Type: "string", Description: "The bd task ID to mark as failed"},
				"reason":  {Type: "string", Description: "Reason for failure/block"},
			},
			Required: []string{"task_id", "reason"},
		},
	}, cs.handleMarkTaskFailed)

	cs.RegisterTool(Tool{
		Name:        "query_worker_state",
		Description: "Query current state of workers with role/phase details. Use before assignments to check availability and prevent duplicates.",
		InputSchema: &InputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"worker_id": {Type: "string", Description: "Specific worker to query (omit for all workers)"},
				"task_id":   {Type: "string", Description: "Query workers assigned to specific task (omit for all)"},
			},
			Required: []string{},
		},
		OutputSchema: &OutputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"workers": {
					Type:        "array",
					Description: "Active workers with current state",
					Items: &PropertySchema{
						Type: "object",
						Properties: map[string]*PropertySchema{
							"worker_id": {Type: "string", Description: "Worker ID (e.g., worker-1)"},
							"status":    {Type: "string", Description: "Current status (Pending, Ready, Working, Paused)"},
							"phase":     {Type: "string", Description: "Current phase (Idle, Implementing, Reviewing, etc.)"},
							"task_id":   {Type: "string", Description: "Assigned task ID if any"},
						},
						Required: []string{"worker_id", "status"},
					},
				},
				"ready_workers": {
					Type:        "array",
					Description: "Worker IDs available for task assignment (Ready status, Idle phase)",
					Items:       &PropertySchema{Type: "string"},
				},
				"retired_workers": {
					Type:        "array",
					Description: "Worker IDs that were gracefully retired",
					Items:       &PropertySchema{Type: "string"},
				},
				"failed_workers": {
					Type:        "array",
					Description: "Worker IDs that failed (session expired, crashed, etc.) - may need replacement",
					Items:       &PropertySchema{Type: "string"},
				},
				"tasks": {
					Type:        "object",
					Description: "Map of task ID to assignment info",
				},
			},
			Required: []string{"workers", "ready_workers", "retired_workers", "failed_workers", "tasks"},
		},
	}, cs.handleQueryWorkerState)

	cs.RegisterTool(Tool{
		Name:        "assign_task_review",
		Description: "Assign a worker to review completed implementation. Validates reviewer is ready and different from implementer.",
		InputSchema: &InputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"reviewer_id":    {Type: "string", Description: "Worker ID to assign as reviewer (e.g., 'worker-2')"},
				"task_id":        {Type: "string", Description: "The bd task ID being reviewed"},
				"implementer_id": {Type: "string", Description: "Worker ID who implemented the task"},
				"summary":        {Type: "string", Description: "Brief summary of what was implemented"},
				"review_type":    {Type: "string", Description: "Review complexity: 'simple' (reviewer checks all dimensions directly) or 'complex' (spawn sub-agents for thorough parallel review). Defaults to 'complex'."},
			},
			Required: []string{"reviewer_id", "task_id", "implementer_id", "summary"},
		},
	}, cs.handleAssignTaskReview)

	cs.RegisterTool(Tool{
		Name:        "assign_review_feedback",
		Description: "Send review feedback to implementer requiring changes. Used when reviewer denies and implementer needs to fix issues.",
		InputSchema: &InputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"implementer_id": {Type: "string", Description: "Worker ID to send feedback to"},
				"task_id":        {Type: "string", Description: "The bd task ID"},
				"feedback":       {Type: "string", Description: "Specific feedback about required changes"},
			},
			Required: []string{"implementer_id", "task_id", "feedback"},
		},
	}, cs.handleAssignReviewFeedback)

	cs.RegisterTool(Tool{
		Name:        "approve_commit",
		Description: "Approve implementation and instruct worker to commit. Called after reviewer approves.",
		InputSchema: &InputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"implementer_id": {Type: "string", Description: "Worker ID to instruct to commit"},
				"task_id":        {Type: "string", Description: "The bd task ID"},
				"commit_message": {Type: "string", Description: "Suggested commit message (optional)"},
			},
			Required: []string{"implementer_id", "task_id"},
		},
	}, cs.handleApproveCommit)

	cs.RegisterTool(Tool{
		Name:        "stop_worker",
		Description: "Stop a running worker process. Supports graceful (default) and forceful termination.",
		InputSchema: &InputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"worker_id": {Type: "string", Description: "ID of the worker to stop (e.g., 'worker-1')"},
				"force":     {Type: "boolean", Description: "If true, immediately SIGKILL without graceful timeout. Default: false"},
				"reason":    {Type: "string", Description: "Optional reason for stopping (for audit logging)"},
			},
			Required: []string{"worker_id"},
		},
	}, cs.handleStopProcess)

	cs.RegisterTool(Tool{
		Name:        "generate_accountability_summary",
		Description: "Assign an aggregation task to a worker to collect and merge accountability summaries from all workers into a unified session summary.",
		InputSchema: &InputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"worker_id": {Type: "string", Description: "The worker ID to assign the aggregation task (e.g., 'worker-1')"},
			},
			Required: []string{"worker_id"},
		},
	}, cs.handleGenerateAccountabilitySummary)

	cs.RegisterTool(Tool{
		Name:        "signal_workflow_complete",
		Description: "Signal that the workflow has completed. Call this when the orchestration workflow reaches its natural conclusion.",
		InputSchema: &InputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"status": {
					Type:        "string",
					Description: "Completion status: 'success' (all goals achieved), 'partial' (some goals achieved), or 'aborted' (workflow terminated early)",
					Enum:        []string{"success", "partial", "aborted"},
				},
				"summary": {
					Type:        "string",
					Description: "Summary of what was accomplished during the workflow",
				},
				"epic_id": {
					Type:        "string",
					Description: "Optional: The epic ID that was completed (if workflow was epic-based)",
				},
				"tasks_closed": {
					Type:        "number",
					Description: "Optional: Number of tasks closed during the workflow",
				},
			},
			Required: []string{"status", "summary"},
		},
	}, cs.handleSignalWorkflowComplete)

	cs.RegisterTool(Tool{
		Name:        "notify_user",
		Description: "Request user attention for a human checkpoint. Use this during DAG workflow phases that require human review or input (e.g., clarification-review). Plays a notification sound and displays the message to the user.",
		InputSchema: &InputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"message": {
					Type:        "string",
					Description: "Message to display to the user explaining what action is needed",
				},
				"phase": {
					Type:        "string",
					Description: "Optional: The workflow phase name (e.g., 'clarification-review')",
				},
				"task_id": {
					Type:        "string",
					Description: "Optional: The task ID associated with this notification",
				},
			},
			Required: []string{"message"},
		},
	}, cs.handleNotifyUser)
}

// Tool argument structs for JSON parsing.
type taskIDArgs struct {
	TaskID string `json:"task_id"`
}

type stopWorkerArgs struct {
	WorkerID string `json:"worker_id"`
	Force    bool   `json:"force"`
	Reason   string `json:"reason,omitempty"`
}

type assignTaskArgs struct {
	WorkerID string `json:"worker_id"`
	TaskID   string `json:"task_id"`
	Summary  string `json:"summary,omitempty"`
}

// SpawnIdleWorker spawns a new idle worker via v2Adapter.
// This is called internally at startup, not exposed to the coordinator.
func (cs *CoordinatorServer) SpawnIdleWorker() (string, error) {
	if cs.v2Adapter == nil {
		return "", fmt.Errorf("v2Adapter required for SpawnIdleWorker")
	}

	// Use v2Adapter to handle spawn - it returns the process ID in the result message
	result, err := cs.v2Adapter.HandleSpawnProcess(context.Background(), nil)
	if err != nil {
		log.ErrorErr(log.CatMCP, "Failed to spawn worker via v2", err)
		return "", fmt.Errorf("failed to spawn worker: %w", err)
	}

	if result.IsError {
		log.Debug(log.CatMCP, "Spawn worker returned error", "content", result.Content)
		return "", fmt.Errorf("spawn worker failed: %v", result.Content)
	}

	// Extract worker ID from result message
	// Format: "Process worker-X spawned and ready"
	var workerID string
	if len(result.Content) > 0 && result.Content[0].Text != "" {
		if _, err := fmt.Sscanf(result.Content[0].Text, "Process %s spawned", &workerID); err != nil {
			// Fallback: just log and return empty ID
			log.Debug(log.CatMCP, "Spawned idle worker (ID extraction failed)", "result", result.Content[0].Text)
			return "", nil
		}
	} else {
		log.Debug(log.CatMCP, "Spawned idle worker (no content in result)")
		return "", nil
	}

	log.Debug(log.CatMCP, "Spawned idle worker", "workerID", workerID)
	return workerID, nil
}

// handleSpawnWorker spawns a new idle worker.
func (cs *CoordinatorServer) handleSpawnWorker(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	return cs.v2Adapter.HandleSpawnProcess(ctx, args)
}

// handleAssignTask assigns a task to a ready worker.
// Posts task assignment to #tasks channel first (no @mention) to create the task thread,
// then passes the thread ID through to the v2Adapter so the worker knows where to reply.
func (cs *CoordinatorServer) handleAssignTask(ctx context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	// Parse args to get task details for Fabric message
	var args assignTaskArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	// Post to Fabric first to create the task thread (no @mention - avoids double notification)
	var threadID string
	if cs.fabricService != nil {
		summary := args.Summary
		if summary == "" {
			summary = "Task assignment"
		}
		content := fmt.Sprintf("Task: %s [%s] assigned to %s", summary, args.TaskID, args.WorkerID)

		thread, postErr := cs.fabricService.SendMessage(fabric.SendMessageInput{
			ChannelSlug: "tasks",
			Content:     content,
			CreatedBy:   repository.CoordinatorID,
			// No mentions - worker gets notified via the v2 delivery mechanism
		})
		if postErr != nil {
			// Log but continue - we can still assign the task without the Fabric thread
			log.Debug(log.CatMCP, "Failed to post task assignment to #tasks",
				"error", postErr, "taskID", args.TaskID, "workerID", args.WorkerID)
		} else {
			threadID = thread.ID
		}
	}

	// Inject threadID into the args for the v2Adapter
	// Re-marshal with the threadID included
	enrichedArgs := struct {
		WorkerID string `json:"worker_id"`
		TaskID   string `json:"task_id"`
		Summary  string `json:"summary,omitempty"`
		ThreadID string `json:"thread_id,omitempty"`
	}{
		WorkerID: args.WorkerID,
		TaskID:   args.TaskID,
		Summary:  args.Summary,
		ThreadID: threadID,
	}
	enrichedRawArgs, err := json.Marshal(enrichedArgs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal enriched args: %w", err)
	}

	// Submit command via v2Adapter with threadID included
	return cs.v2Adapter.HandleAssignTask(ctx, enrichedRawArgs)
}

// handleReplaceWorker retires a worker and spawns a fresh replacement.
func (cs *CoordinatorServer) handleReplaceWorker(ctx context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	return cs.v2Adapter.HandleReplaceProcess(ctx, rawArgs)
}

// handleReplaceWorker retires a worker and spawns a fresh replacement.
func (cs *CoordinatorServer) handleRetireWorker(ctx context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	return cs.v2Adapter.HandleRetireProcess(ctx, rawArgs)
}

// handleGetTaskStatus gets task status from bd.
func (cs *CoordinatorServer) handleGetTaskStatus(_ context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	var args taskIDArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if args.TaskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	if !isValidTaskID(args.TaskID) {
		return nil, fmt.Errorf("invalid task_id format: %s", args.TaskID)
	}

	// Get task info using BeadsExecutor
	issue, err := cs.beadsExecutor.ShowIssue(args.TaskID)
	if err != nil {
		log.Debug(log.CatMCP, "bd show failed", "taskID", args.TaskID, "error", err)
		return nil, fmt.Errorf("bd show failed: %w", err)
	}

	// Return the issue as JSON wrapped in an array (for backward compatibility with bd show output)
	data, err := json.MarshalIndent([]*taskpkg.Issue{issue}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling issue: %w", err)
	}

	return SuccessResult(string(data)), nil
}

// handleMarkTaskComplete marks a task as complete in bd.
// Routes through v2Adapter which uses the command processor to update BD.
func (cs *CoordinatorServer) handleMarkTaskComplete(ctx context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	return cs.v2Adapter.HandleMarkTaskComplete(ctx, rawArgs)
}

// handleMarkTaskFailed adds a failure comment to a task in bd.
// Routes through v2Adapter which uses the command processor to update BD.
func (cs *CoordinatorServer) handleMarkTaskFailed(ctx context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	return cs.v2Adapter.HandleMarkTaskFailed(ctx, rawArgs)
}

// handleQueryWorkerState returns detailed worker state including phase.
// Task assignment details are managed by v2 repositories.
func (cs *CoordinatorServer) handleQueryWorkerState(ctx context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	return cs.v2Adapter.HandleQueryWorkerState(ctx, rawArgs)
}

// handleAssignTaskReview assigns a reviewer to a completed implementation.
func (cs *CoordinatorServer) handleAssignTaskReview(ctx context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	return cs.v2Adapter.HandleAssignTaskReview(ctx, rawArgs)
}

// handleAssignReviewFeedback sends review feedback to implementer requiring changes.
func (cs *CoordinatorServer) handleAssignReviewFeedback(ctx context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	return cs.v2Adapter.HandleAssignReviewFeedback(ctx, rawArgs)
}

// handleApproveCommit approves implementation and instructs worker to commit.
func (cs *CoordinatorServer) handleApproveCommit(ctx context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	return cs.v2Adapter.HandleApproveCommit(ctx, rawArgs)
}

// handleStopProcess stops a running worker process.
func (cs *CoordinatorServer) handleStopProcess(_ context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	var args stopWorkerArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if args.WorkerID == "" {
		return nil, fmt.Errorf("worker_id is required")
	}

	if cs.v2Adapter == nil {
		return nil, fmt.Errorf("v2Adapter required for stop_worker")
	}

	if err := cs.v2Adapter.HandleStopProcess(args.WorkerID, args.Force, args.Reason); err != nil {
		return nil, fmt.Errorf("stop_worker failed: %w", err)
	}

	return SuccessResult("Worker stop command submitted"), nil
}

// isValidTaskID validates that a task ID matches the expected format.
// Valid formats: "prefix-xxxx" or "prefix-xxxx.N" (for subtasks)
func isValidTaskID(taskID string) bool {
	return validation.IsValidTaskID(taskID)
}

// handleGenerateAccountabilitySummary assigns an aggregation task to a worker to merge accountability summaries.
func (cs *CoordinatorServer) handleGenerateAccountabilitySummary(ctx context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	if cs.v2Adapter == nil {
		return nil, fmt.Errorf("v2Adapter required for generate_accountability_summary")
	}
	return cs.v2Adapter.HandleGenerateAccountabilitySummary(ctx, rawArgs)
}

// handleSignalWorkflowComplete signals that the workflow has completed.
func (cs *CoordinatorServer) handleSignalWorkflowComplete(ctx context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	if cs.v2Adapter == nil {
		return nil, fmt.Errorf("v2Adapter required for signal_workflow_complete")
	}
	return cs.v2Adapter.HandleSignalWorkflowComplete(ctx, rawArgs)
}

// handleNotifyUser requests user attention for a human checkpoint.
func (cs *CoordinatorServer) handleNotifyUser(ctx context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	if cs.v2Adapter == nil {
		return nil, fmt.Errorf("v2Adapter required for notify_user")
	}
	return cs.v2Adapter.HandleNotifyUser(ctx, rawArgs)
}
