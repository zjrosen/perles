package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/zjrosen/perles/internal/beads"
	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/orchestration/pool"
)

// taskIDPattern validates bd task IDs to prevent command injection.
// Valid formats: "prefix-xxxx" or "prefix-xxxx.N" (for subtasks)
var taskIDPattern = regexp.MustCompile(`^[a-zA-Z]+-[a-zA-Z0-9]{2,10}(\.\d+)?$`)

// WorkerRole identifies what role a worker plays in the workflow.
type WorkerRole string

const (
	// RoleImplementer means the worker is implementing a task.
	RoleImplementer WorkerRole = "implementer"
	// RoleReviewer means the worker is reviewing another worker's implementation.
	RoleReviewer WorkerRole = "reviewer"
)

// WorkerAssignment tracks what a worker is currently doing.
type WorkerAssignment struct {
	TaskID        string             // bd task ID being worked on
	Role          WorkerRole         // implementer or reviewer
	Phase         events.WorkerPhase // Current workflow phase
	AssignedAt    time.Time          // When this assignment started
	ImplementerID string             // For reviewers: who implemented (empty for implementers)
	ReviewerID    string             // For implementers: who is reviewing (empty until assigned)
}

// TaskWorkflowStatus tracks where a task is in the workflow.
type TaskWorkflowStatus string

const (
	// TaskImplementing means the task is being implemented.
	TaskImplementing TaskWorkflowStatus = "implementing"
	// TaskInReview means the task is being reviewed.
	TaskInReview TaskWorkflowStatus = "in_review"
	// TaskApproved means the review approved the implementation.
	TaskApproved TaskWorkflowStatus = "approved"
	// TaskDenied means the review denied the implementation.
	TaskDenied TaskWorkflowStatus = "denied"
	// TaskCommitting means the implementer is creating a git commit.
	TaskCommitting TaskWorkflowStatus = "committing"
	// TaskCompleted means the task is finished.
	TaskCompleted TaskWorkflowStatus = "completed"
)

// TaskAssignment tracks all workers involved with a task.
type TaskAssignment struct {
	TaskID          string             // bd task ID
	Implementer     string             // Worker ID implementing
	Reviewer        string             // Worker ID reviewing (empty until assigned)
	Status          TaskWorkflowStatus // Current task workflow status
	StartedAt       time.Time          // When implementation started
	ReviewStartedAt time.Time          // When review started (zero until assigned)
}

// CoordinatorServer is an MCP server that exposes orchestration tools to the coordinator agent.
// It provides tools for spawning workers, managing tasks, and communicating via message issues.
type CoordinatorServer struct {
	*Server
	client        client.HeadlessClient
	pool          *pool.WorkerPool
	msgIssue      *message.Issue
	workDir       string
	port          int                 // HTTP server port for MCP config generation
	extensions    map[string]any      // Provider-specific extensions (model, mode, etc.)
	beadsExecutor beads.BeadsExecutor // BD command executor

	// State tracking for deterministic orchestration
	workerAssignments map[string]*WorkerAssignment // workerID -> assignment
	taskAssignments   map[string]*TaskAssignment   // taskID -> assignment
	assignmentsMu     sync.RWMutex                 // protects workerAssignments and taskAssignments

	// dedup tracks recent messages to prevent duplicate sends to workers
	dedup *MessageDeduplicator
}

// NewCoordinatorServer creates a new coordinator MCP server.
// The client is used for spawning and resuming AI processes for workers.
// The port is the HTTP server port used for MCP config generation.
// The extensions map holds provider-specific configuration (model, mode, etc.)
// that will be passed to workers when they are spawned.
// The beadsExec parameter is required and must not be nil.
func NewCoordinatorServer(
	aiClient client.HeadlessClient,
	workerPool *pool.WorkerPool,
	msgIssue *message.Issue,
	workDir string,
	port int,
	extensions map[string]any,
	beadsExec beads.BeadsExecutor,
) *CoordinatorServer {
	cs := &CoordinatorServer{
		Server:            NewServer("perles-orchestrator", "1.0.0", WithInstructions(coordinatorInstructions)),
		client:            aiClient,
		pool:              workerPool,
		msgIssue:          msgIssue,
		workDir:           workDir,
		port:              port,
		extensions:        extensions,
		beadsExecutor:     beadsExec,
		workerAssignments: make(map[string]*WorkerAssignment),
		taskAssignments:   make(map[string]*TaskAssignment),
		dedup:             NewMessageDeduplicator(DefaultDeduplicationWindow),
	}

	cs.registerTools()
	return cs
}

// coordinatorInstructions provides a brief description for the MCP server.
// Detailed instructions are in the coordinator's system prompt (see prompt.go).
const coordinatorInstructions = `Perles orchestrator MCP server providing worker management and task coordination tools.`

// generateWorkerMCPConfig returns the appropriate MCP config format for workers based on client type.
func (cs *CoordinatorServer) generateWorkerMCPConfig(workerID string) (string, error) {
	switch cs.client.Type() {
	case client.ClientAmp:
		return GenerateWorkerConfigAmp(cs.port, workerID)
	default:
		return GenerateWorkerConfigHTTP(cs.port, workerID)
	}
}

// registerTools registers all coordinator tools with the MCP server.
// In prompt mode, task-related tools (assign_task, get_task_status, mark_task_complete, mark_task_failed) are excluded.
func (cs *CoordinatorServer) registerTools() {
	// spawn_worker - Spawn a new idle worker
	cs.RegisterTool(Tool{
		Name:        "spawn_worker",
		Description: "Spawn a new idle worker in the pool. The worker starts in Ready state waiting for task assignment. Returns the new worker ID.",
		InputSchema: &InputSchema{
			Type:       "object",
			Properties: map[string]*PropertySchema{},
			Required:   []string{},
		},
	}, cs.handleSpawnWorker)

	// assign_task - Assign a task to a ready worker (available in both modes)
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

	// replace_worker - Retire a worker and spawn replacement
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

	// send_to_worker - Send message to a worker
	cs.RegisterTool(Tool{
		Name:        "send_to_worker",
		Description: "Send a follow-up message to a worker by resuming its session with new instructions.",
		InputSchema: &InputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"worker_id": {Type: "string", Description: "The worker ID to message"},
				"message":   {Type: "string", Description: "Message content to send"},
			},
			Required: []string{"worker_id", "message"},
		},
	}, cs.handleSendToWorker)

	// post_message - Post to message log
	cs.RegisterTool(Tool{
		Name:        "post_message",
		Description: "Post a message to the shared message log. Use 'ALL' to broadcast or a specific worker ID.",
		InputSchema: &InputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"to":      {Type: "string", Description: "Recipient: 'ALL' or a specific agent ID (e.g., 'WORKER.1')"},
				"content": {Type: "string", Description: "Message content"},
			},
			Required: []string{"to", "content"},
		},
	}, cs.handlePostMessage)

	// get_task_status - Get task status from bd
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

	// mark_task_complete - Mark task as done
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

	// mark_task_failed - Mark task as blocked/failed
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

	// read_message_log - Read recent messages
	cs.RegisterTool(Tool{
		Name:        "read_message_log",
		Description: "Read messages from the shared message log. By default returns only unread messages. Use read_all=true to get all messages.",
		InputSchema: &InputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"limit":    {Type: "number", Description: "Maximum number of messages to return (default: 20)"},
				"read_all": {Type: "boolean", Description: "Return all messages instead of just unread (default: false)"},
			},
			Required: []string{},
		},
		OutputSchema: &OutputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"total_count":    {Type: "number", Description: "Total number of messages in the log"},
				"returned_count": {Type: "number", Description: "Number of messages returned in this response"},
				"messages": {
					Type:        "array",
					Description: "List of messages in chronological order",
					Items: &PropertySchema{
						Type: "object",
						Properties: map[string]*PropertySchema{
							"timestamp": {Type: "string", Description: "Message timestamp (HH:MM:SS format)"},
							"from":      {Type: "string", Description: "Sender ID (COORDINATOR, WORKER.N, etc.)"},
							"to":        {Type: "string", Description: "Recipient ID (ALL, COORDINATOR, WORKER.N, etc.)"},
							"content":   {Type: "string", Description: "Message content"},
						},
						Required: []string{"timestamp", "from", "to", "content"},
					},
				},
			},
			Required: []string{"total_count", "returned_count", "messages"},
		},
	}, cs.handleReadMessageLog)

	// list_workers - List all workers
	cs.RegisterTool(Tool{
		Name:        "list_workers",
		Description: "List all workers (active and completed) with their current task assignments, status, session IDs, and context usage. Use context_usage field to identify workers running low on context window.",
		InputSchema: &InputSchema{
			Type:       "object",
			Properties: map[string]*PropertySchema{},
			Required:   []string{},
		},
	}, cs.handleListWorkers)

	// prepare_handoff - Post handoff message before coordinator refresh
	cs.RegisterTool(Tool{
		Name:        "prepare_handoff",
		Description: "Post a handoff message before coordinator context refresh. Call this when the user initiates a refresh (Ctrl+R). Include a summary of current work state, in-progress tasks, and any important context for the incoming coordinator.",
		InputSchema: &InputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"summary": {
					Type:        "string",
					Description: "Summary of current state: what work is in progress, decisions made, any blockers or issues, and recommendations for the incoming coordinator",
				},
			},
			Required: []string{"summary"},
		},
	}, cs.handlePrepareHandoff)

	// query_worker_state - Query current state of workers with role/phase details
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
	}, cs.handleQueryWorkerState)

	// assign_task_review - Assign a worker to review completed implementation
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
			},
			Required: []string{"reviewer_id", "task_id", "implementer_id", "summary"},
		},
	}, cs.handleAssignTaskReview)

	// assign_review_feedback - Send review feedback to implementer requiring changes
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

	// approve_commit - Approve implementation and instruct worker to commit
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
}

// Tool argument structs for JSON parsing.
type assignTaskArgs struct {
	WorkerID string `json:"worker_id"`
	TaskID   string `json:"task_id"`
	Summary  string `json:"summary,omitempty"` // Optional instructions/context for the worker
}

type replaceWorkerArgs struct {
	WorkerID string `json:"worker_id"`
	Reason   string `json:"reason"`
}

type sendToWorkerArgs struct {
	WorkerID string `json:"worker_id"`
	Message  string `json:"message"`
}

type postMessageArgs struct {
	To      string `json:"to"`
	Content string `json:"content"`
}

type taskIDArgs struct {
	TaskID string `json:"task_id"`
}

type markTaskFailedArgs struct {
	TaskID string `json:"task_id"`
	Reason string `json:"reason"`
}

type readMessageLogArgs struct {
	Limit   int  `json:"limit,omitempty"`
	ReadAll bool `json:"read_all,omitempty"`
}

type prepareHandoffArgs struct {
	Summary string `json:"summary"`
}

type queryWorkerStateArgs struct {
	WorkerID string `json:"worker_id,omitempty"`
	TaskID   string `json:"task_id,omitempty"`
}

type assignTaskReviewArgs struct {
	ReviewerID    string `json:"reviewer_id"`
	TaskID        string `json:"task_id"`
	ImplementerID string `json:"implementer_id"`
	Summary       string `json:"summary"`
}

type assignReviewFeedbackArgs struct {
	ImplementerID string `json:"implementer_id"`
	TaskID        string `json:"task_id"`
	Feedback      string `json:"feedback"`
}

type approveCommitArgs struct {
	ImplementerID string `json:"implementer_id"`
	TaskID        string `json:"task_id"`
	CommitMessage string `json:"commit_message,omitempty"`
}

// SpawnIdleWorker spawns a new idle worker in the pool.
// This is called internally at startup, not exposed to the coordinator.
func (cs *CoordinatorServer) SpawnIdleWorker() (string, error) {
	// Pre-generate worker ID so it can be embedded in the MCP config
	workerID := cs.pool.NextWorkerID()

	// Generate MCP config for worker with the actual worker ID
	mcpConfig, err := cs.generateWorkerMCPConfig(workerID)
	if err != nil {
		log.Debug(log.CatMCP, "Failed to generate worker MCP config", "workerID", workerID, "error", err)
		return "", fmt.Errorf("failed to generate MCP config: %w", err)
	}

	// Build config for idle worker
	cfg := client.Config{
		WorkDir:         cs.workDir,
		Prompt:          WorkerIdlePrompt(workerID),
		SystemPrompt:    WorkerSystemPrompt(workerID),
		MCPConfig:       mcpConfig,
		SkipPermissions: true,
		DisallowedTools: []string{"AskUserQuestion"},
		Extensions:      cs.extensions,
	}

	// Spawn worker with the pre-generated ID
	workerID, err = cs.pool.SpawnWorkerWithID(workerID, cfg)
	if err != nil {
		log.ErrorErr(log.CatMCP, "Failed to spawn worker", err,
			"workerID", workerID)
		return "", fmt.Errorf("failed to spawn worker: %w", err)
	}

	log.Debug(log.CatMCP, "Spawned idle worker", "workerID", workerID)

	return workerID, nil
}

// handleSpawnWorker spawns a new idle worker in the pool.
func (cs *CoordinatorServer) handleSpawnWorker(_ context.Context, _ json.RawMessage) (*ToolCallResult, error) {
	workerID, err := cs.SpawnIdleWorker()
	if err != nil {
		return nil, fmt.Errorf("failed to spawn worker: %w", err)
	}

	log.Debug(log.CatMCP, "Spawned worker via MCP tool", "workerID", workerID)
	return SuccessResult(fmt.Sprintf("Worker %s spawned and ready for task assignment", workerID)), nil
}

// handleAssignTask assigns a task to a ready worker.
func (cs *CoordinatorServer) handleAssignTask(ctx context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	var args assignTaskArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if args.WorkerID == "" {
		return nil, fmt.Errorf("worker_id is required")
	}
	if args.TaskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	if !isValidTaskID(args.TaskID) {
		return nil, fmt.Errorf("invalid task_id format: %s", args.TaskID)
	}

	// Validate assignment using new state tracking
	if err := cs.validateTaskAssignment(args.WorkerID, args.TaskID); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Get task details from bd
	taskInfo, err := cs.beadsExecutor.ShowIssue(args.TaskID)
	if err != nil {
		log.ErrorErr(log.CatMCP, "Failed to get task info", err,
			"taskID", args.TaskID,
			"workerID", args.WorkerID)
		return nil, fmt.Errorf("failed to get task info: %w", err)
	}

	// Assign task to worker in pool
	if err := cs.pool.AssignTaskToWorker(args.WorkerID, args.TaskID); err != nil {
		return nil, fmt.Errorf("failed to assign task: %w", err)
	}

	// Create assignment records for state tracking
	now := time.Now()
	cs.assignmentsMu.Lock()
	cs.workerAssignments[args.WorkerID] = &WorkerAssignment{
		TaskID:     args.TaskID,
		Role:       RoleImplementer,
		Phase:      events.PhaseImplementing,
		AssignedAt: now,
	}
	cs.taskAssignments[args.TaskID] = &TaskAssignment{
		TaskID:      args.TaskID,
		Implementer: args.WorkerID,
		Status:      TaskImplementing,
		StartedAt:   now,
	}
	cs.assignmentsMu.Unlock()

	// Set worker Phase to Implementing and get session ID
	worker := cs.pool.GetWorker(args.WorkerID)
	if worker == nil {
		return nil, fmt.Errorf("worker not found: %s", args.WorkerID)
	}
	worker.SetPhase(events.PhaseImplementing)

	sessionID := worker.GetSessionID()
	if sessionID == "" {
		return nil, fmt.Errorf("worker %s has no session ID yet", args.WorkerID)
	}

	// Update BD status to in_progress (best-effort)
	if err := cs.beadsExecutor.UpdateStatus(args.TaskID, beads.StatusInProgress); err != nil {
		log.Warn(log.CatMCP, "Failed to update BD status", "taskID", args.TaskID, "status", "in_progress", "error", err)
	}

	// Generate MCP config for worker
	mcpConfig, err := cs.generateWorkerMCPConfig(args.WorkerID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate MCP config: %w", err)
	}

	// Build task assignment prompt
	prompt := TaskAssignmentPrompt(args.TaskID, taskInfo.TitleText, args.Summary)

	// Resume worker with task assignment
	proc, err := cs.client.Spawn(ctx, client.Config{
		WorkDir:         cs.workDir,
		SessionID:       sessionID,
		Prompt:          prompt,
		MCPConfig:       mcpConfig,
		SkipPermissions: true,
		DisallowedTools: []string{"AskUserQuestion"},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to send task to worker: %w", err)
	}

	// Resume worker in pool
	if err := cs.pool.ResumeWorker(args.WorkerID, proc); err != nil {
		return nil, fmt.Errorf("failed to resume worker: %w", err)
	}

	// Emit event for TUI
	cs.pool.EmitIncomingMessage(args.WorkerID, args.TaskID, prompt)

	log.Debug(log.CatMCP, "Assigned task to worker", "workerID", args.WorkerID, "taskID", args.TaskID)
	return SuccessResult(fmt.Sprintf("Task %s assigned to %s", args.TaskID, args.WorkerID)), nil
}

// handleReplaceWorker retires a worker and spawns a fresh replacement.
func (cs *CoordinatorServer) handleReplaceWorker(ctx context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	var args replaceWorkerArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if args.WorkerID == "" {
		return nil, fmt.Errorf("worker_id is required")
	}

	// Clean up assignment state before retiring worker
	cs.assignmentsMu.Lock()
	if wa, ok := cs.workerAssignments[args.WorkerID]; ok && wa != nil {
		// If worker had a task, note it may be orphaned
		// The task assignment remains but implementer/reviewer reference is now invalid
		// detectOrphanedTasks() will find this
		log.Debug(log.CatMCP, "Retiring worker with active assignment",
			"workerID", args.WorkerID,
			"taskID", wa.TaskID,
			"phase", wa.Phase)
	}
	delete(cs.workerAssignments, args.WorkerID)
	cs.assignmentsMu.Unlock()

	// Retire the old worker
	if err := cs.pool.RetireWorker(args.WorkerID); err != nil {
		return nil, fmt.Errorf("failed to retire worker: %w", err)
	}

	// Spawn a fresh replacement
	newWorkerID, err := cs.SpawnIdleWorker()
	if err != nil {
		return nil, fmt.Errorf("failed to spawn replacement: %w", err)
	}

	reason := args.Reason
	if reason == "" {
		reason = "manual replacement"
	}

	log.Debug(log.CatMCP, "Replaced worker", "oldWorkerID", args.WorkerID, "newWorkerID", newWorkerID, "reason", reason)
	return SuccessResult(fmt.Sprintf("Worker %s retired (%s). Replacement: %s", args.WorkerID, reason, newWorkerID)), nil
}

// handleSendToWorker sends a message to a worker by resuming its session.
func (cs *CoordinatorServer) handleSendToWorker(ctx context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	var args sendToWorkerArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if args.WorkerID == "" {
		return nil, fmt.Errorf("worker_id is required")
	}
	if args.Message == "" {
		return nil, fmt.Errorf("message is required")
	}

	// Check for duplicate message within deduplication window
	if cs.dedup.IsDuplicate(args.WorkerID, args.Message) {
		log.Debug(log.CatMCP, "Duplicate message suppressed", "workerID", args.WorkerID)
		return SuccessResult(fmt.Sprintf("Message sent to %s", args.WorkerID)), nil
	}

	worker := cs.pool.GetWorker(args.WorkerID)
	if worker == nil {
		return nil, fmt.Errorf("worker not found: %s", args.WorkerID)
	}

	sessionID := worker.GetSessionID()
	if sessionID == "" {
		return nil, fmt.Errorf("worker %s has no session ID yet (may still be starting)", args.WorkerID)
	}

	// Regenerate MCP config for the worker so it has access to tools when resumed
	mcpConfig, err := cs.generateWorkerMCPConfig(args.WorkerID)
	if err != nil {
		log.Debug(log.CatMCP, "Failed to generate worker MCP config for resume", "workerID", args.WorkerID, "error", err)
		return nil, fmt.Errorf("failed to generate MCP config: %w", err)
	}

	// Resume the worker's session with the new message and full config (including MCP tools)
	proc, err := cs.client.Spawn(ctx, client.Config{
		WorkDir:         cs.workDir,
		SessionID:       sessionID,
		Prompt:          fmt.Sprintf("[MESSAGE FROM COORDINATOR]\n%s", args.Message),
		MCPConfig:       mcpConfig,
		SkipPermissions: true,
		DisallowedTools: []string{"AskUserQuestion"},
	})
	if err != nil {
		log.Debug(log.CatMCP, "Failed to send to worker", "workerID", args.WorkerID, "error", err)
		return nil, fmt.Errorf("failed to send message: %w", err)
	}

	// Emit an event for the incoming message so it shows in the worker pane
	cs.pool.EmitIncomingMessage(args.WorkerID, worker.TaskID, args.Message)

	// Resume the worker in the pool so events are processed and visible in the TUI
	if err := cs.pool.ResumeWorker(args.WorkerID, proc); err != nil {
		log.Debug(log.CatMCP, "Failed to resume worker in pool", "workerID", args.WorkerID, "error", err)
		return nil, fmt.Errorf("failed to resume worker: %w", err)
	}

	log.Debug(log.CatMCP, "Sent message to worker", "workerID", args.WorkerID)
	return SuccessResult(fmt.Sprintf("Message sent to %s", args.WorkerID)), nil
}

// handlePostMessage posts a message to the message log.
func (cs *CoordinatorServer) handlePostMessage(_ context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	var args postMessageArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if args.To == "" {
		return nil, fmt.Errorf("to is required")
	}
	if args.Content == "" {
		return nil, fmt.Errorf("content is required")
	}

	if cs.msgIssue == nil {
		return nil, fmt.Errorf("message issue not available")
	}

	_, err := cs.msgIssue.Append(message.ActorCoordinator, args.To, args.Content, message.MessageInfo)
	if err != nil {
		log.Debug(log.CatMCP, "Failed to post message", "to", args.To, "error", err)
		return nil, fmt.Errorf("failed to post message: %w", err)
	}

	log.Debug(log.CatMCP, "Posted message", "to", args.To)
	return SuccessResult(fmt.Sprintf("Message posted to %s", args.To)), nil
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
	data, err := json.MarshalIndent([]*beads.Issue{issue}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling issue: %w", err)
	}

	return SuccessResult(string(data)), nil
}

// handleMarkTaskComplete marks a task as complete in bd and updates internal state.
// This performs the TaskCommitting -> TaskCompleted transition and cleans up worker state.
func (cs *CoordinatorServer) handleMarkTaskComplete(_ context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
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

	// Validate task exists and is in correct state
	cs.assignmentsMu.RLock()
	ta, ok := cs.taskAssignments[args.TaskID]
	if !ok {
		cs.assignmentsMu.RUnlock()
		return nil, fmt.Errorf("task %s not found in assignments", args.TaskID)
	}
	if ta.Status != TaskCommitting {
		cs.assignmentsMu.RUnlock()
		return nil, fmt.Errorf("task %s is not in committing status (current: %s)", args.TaskID, ta.Status)
	}
	implementerID := ta.Implementer
	cs.assignmentsMu.RUnlock()

	// Update bd status to closed using BeadsExecutor (external state first)
	if err := cs.beadsExecutor.UpdateStatus(args.TaskID, beads.StatusClosed); err != nil {
		log.Debug(log.CatMCP, "bd update failed", "taskID", args.TaskID, "error", err)
		return nil, fmt.Errorf("bd update failed: %w", err)
	}

	// Update internal state atomically
	cs.assignmentsMu.Lock()
	if ta, ok := cs.taskAssignments[args.TaskID]; ok {
		ta.Status = TaskCompleted
	}
	if implAssignment, ok := cs.workerAssignments[implementerID]; ok {
		implAssignment.Phase = events.PhaseIdle
		implAssignment.TaskID = ""
	}
	cs.assignmentsMu.Unlock()

	// Add BD comment for audit trail
	if err := cs.beadsExecutor.AddComment(args.TaskID, "coordinator", "Task completed"); err != nil {
		log.Debug(log.CatMCP, "bd comment failed", "taskID", args.TaskID, "error", err)
	}

	log.Debug(log.CatMCP, "Marked task complete", "taskID", args.TaskID, "implementerID", implementerID)

	// Return structured response
	response := map[string]any{
		"status":  "success",
		"message": fmt.Sprintf("Task %s marked as completed", args.TaskID),
		"task_state": map[string]string{
			"task_id": args.TaskID,
			"status":  string(TaskCompleted),
		},
		"implementer_state": map[string]string{
			"worker_id": implementerID,
			"phase":     string(events.PhaseIdle),
		},
	}
	data, _ := json.MarshalIndent(response, "", "  ")
	return StructuredResult(string(data), response), nil
}

// handleMarkTaskFailed adds a failure comment to a task in bd.
func (cs *CoordinatorServer) handleMarkTaskFailed(_ context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	var args markTaskFailedArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if args.TaskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	if !isValidTaskID(args.TaskID) {
		return nil, fmt.Errorf("invalid task_id format: %s", args.TaskID)
	}
	if args.Reason == "" {
		return nil, fmt.Errorf("reason is required")
	}

	// Add comment with failure reason using BeadsExecutor
	failureComment := fmt.Sprintf("⚠️ Task failed: %s", args.Reason)
	if err := cs.beadsExecutor.AddComment(args.TaskID, "coordinator", failureComment); err != nil {
		log.Debug(log.CatMCP, "bd comment failed", "taskID", args.TaskID, "error", err)
		return nil, fmt.Errorf("bd comment failed: %w", err)
	}

	log.Debug(log.CatMCP, "Marked task failed", "taskID", args.TaskID, "reason", args.Reason)
	return SuccessResult(fmt.Sprintf("Task %s marked as failed with comment: %s", args.TaskID, args.Reason)), nil
}

// messageLogResponse is the structured response for read_message_log.
type messageLogResponse struct {
	TotalCount    int               `json:"total_count"`
	ReturnedCount int               `json:"returned_count"`
	Messages      []messageLogEntry `json:"messages"`
}

// messageLogEntry is a single message in the log response.
type messageLogEntry struct {
	Timestamp string `json:"timestamp"` // HH:MM:SS format
	From      string `json:"from"`
	To        string `json:"to"`
	Content   string `json:"content"`
}

// handleReadMessageLog reads recent messages from the message log.
func (cs *CoordinatorServer) handleReadMessageLog(_ context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	var args readMessageLogArgs
	if len(rawArgs) > 0 {
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}

	if cs.msgIssue == nil {
		return nil, fmt.Errorf("message issue not available")
	}

	limit := args.Limit
	if limit <= 0 {
		limit = 20
	}

	var entries []message.Entry
	var totalCount int

	if args.ReadAll {
		// Opt-in: return all messages (for initial context or debugging)
		allEntries := cs.msgIssue.Entries()
		totalCount = len(allEntries)
		entries = allEntries
		if len(entries) > limit {
			entries = entries[len(entries)-limit:]
		}
	} else {
		// Default: use existing readState pattern (same as workers)
		entries = cs.msgIssue.UnreadFor(message.ActorCoordinator)
		totalCount = len(entries)
		cs.msgIssue.MarkRead(message.ActorCoordinator)
	}

	// Build structured response
	messages := make([]messageLogEntry, len(entries))
	for i, entry := range entries {
		messages[i] = messageLogEntry{
			Timestamp: entry.Timestamp.Format("15:04:05"),
			From:      entry.From,
			To:        entry.To,
			Content:   entry.Content,
		}
	}

	response := messageLogResponse{
		TotalCount:    totalCount,
		ReturnedCount: len(messages),
		Messages:      messages,
	}

	data, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling response: %w", err)
	}

	return StructuredResult(string(data), response), nil
}

func (cs *CoordinatorServer) handleListWorkers(_ context.Context, _ json.RawMessage) (*ToolCallResult, error) {
	if cs.pool == nil {
		return nil, fmt.Errorf("worker pool not available")
	}

	// Only show active workers (not retired)
	workers := cs.pool.ActiveWorkers()

	if len(workers) == 0 {
		return SuccessResult("No active workers."), nil
	}

	// Format worker information
	type workerInfo struct {
		WorkerID      string `json:"worker_id"`
		PID           int    `json:"pid,omitempty"`     // Process ID of Claude process
		TaskID        string `json:"task_id,omitempty"` // Current task (empty if ready)
		Status        string `json:"status"`            // ready, working, retired
		Phase         string `json:"phase"`             // idle, implementing, awaiting_review, reviewing, etc.
		Role          string `json:"role,omitempty"`    // implementer, reviewer (empty if idle)
		SessionID     string `json:"session_id"`
		StartedAt     string `json:"started_at"`
		ContextTokens int    `json:"context_tokens,omitempty"` // Total context used
		ContextWindow int    `json:"context_window,omitempty"` // Max context window
		ContextUsage  string `json:"context_usage,omitempty"`  // Human-readable usage (e.g., "27k/200k")
	}

	// Get workerAssignments under lock for phase/role lookup
	cs.assignmentsMu.RLock()
	defer cs.assignmentsMu.RUnlock()

	infos := make([]workerInfo, 0, len(workers))
	for _, w := range workers {
		info := workerInfo{
			WorkerID:  w.ID,
			PID:       w.GetPID(),
			TaskID:    w.GetTaskID(),
			Status:    w.GetStatus().String(),
			Phase:     string(events.PhaseIdle), // Default to idle
			SessionID: w.GetSessionID(),
			StartedAt: w.GetStartedAt().Format("15:04:05"),
		}

		// Get phase and role from assignment if worker has one
		if wa, ok := cs.workerAssignments[w.ID]; ok && wa != nil {
			info.Phase = string(wa.Phase)
			if wa.Role != "" {
				info.Role = string(wa.Role)
			}
		}

		// Get context info from metrics if available
		if m := w.GetMetrics(); m != nil {
			info.ContextTokens = m.ContextTokens
			info.ContextWindow = m.ContextWindow
		}

		// Add human-readable context usage
		if info.ContextTokens > 0 && info.ContextWindow > 0 {
			info.ContextUsage = formatContextUsage(info.ContextTokens, info.ContextWindow)
		}

		infos = append(infos, info)
	}

	// Marshal to JSON for clean output
	data, err := json.MarshalIndent(infos, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling worker info: %w", err)
	}

	return SuccessResult(string(data)), nil
}

// handlePrepareHandoff posts a handoff message before coordinator context refresh.
func (cs *CoordinatorServer) handlePrepareHandoff(_ context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	var args prepareHandoffArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if args.Summary == "" {
		return nil, fmt.Errorf("summary is required")
	}

	if cs.msgIssue == nil {
		return nil, fmt.Errorf("message issue not available")
	}

	// Build handoff content with marker
	content := fmt.Sprintf("[HANDOFF]\n%s", args.Summary)

	_, err := cs.msgIssue.Append(
		message.ActorCoordinator,
		message.ActorAll,
		content,
		message.MessageHandoff,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to post handoff: %w", err)
	}

	log.Debug(log.CatMCP, "Posted handoff message")
	return SuccessResult("Handoff message posted. Refresh will proceed."), nil
}

// formatContextUsage returns a human-readable context usage string (e.g., "27k/200k (13%)").
func formatContextUsage(contextTokens, contextWindow int) string {
	tokensK := contextTokens / 1000
	windowK := contextWindow / 1000
	percentage := (contextTokens * 100) / contextWindow
	return fmt.Sprintf("%dk/%dk (%d%%)", tokensK, windowK, percentage)
}

// isValidTaskID validates that a task ID matches the expected format.
// Valid formats: "prefix-xxxx" or "prefix-xxxx.N" (for subtasks)
func isValidTaskID(taskID string) bool {
	return taskIDPattern.MatchString(taskID)
}

// validateTaskAssignment checks if a task can be assigned to a worker.
// Returns error if:
// - Task already has an implementer assigned
// - Worker already has an active assignment
// - Worker is not in Ready status
func (cs *CoordinatorServer) validateTaskAssignment(workerID, taskID string) error {
	cs.assignmentsMu.RLock()
	defer cs.assignmentsMu.RUnlock()

	// 1. Check if task already has an implementer
	if ta, ok := cs.taskAssignments[taskID]; ok && ta.Implementer != "" {
		return fmt.Errorf("task %s already assigned to %s", taskID, ta.Implementer)
	}

	// 2. Check if worker is already assigned to something
	if wa, ok := cs.workerAssignments[workerID]; ok && wa.TaskID != "" {
		return fmt.Errorf("worker %s already assigned to task %s", workerID, wa.TaskID)
	}

	// 3. Check worker is Ready
	worker := cs.pool.GetWorker(workerID)
	if worker == nil {
		return fmt.Errorf("worker %s not found", workerID)
	}
	if worker.GetStatus() != pool.WorkerReady {
		return fmt.Errorf("worker %s is not ready (status: %v)", workerID, worker.GetStatus())
	}

	return nil
}

// validateReviewAssignment checks if a reviewer can be assigned to a task.
// Returns error if:
// - Reviewer is the same as implementer
// - Task doesn't exist or implementer mismatch
// - Implementer is not in AwaitingReview phase
// - Task already has a reviewer assigned
// - Reviewer is not in Ready status
func (cs *CoordinatorServer) validateReviewAssignment(reviewerID, taskID, implementerID string) error {
	cs.assignmentsMu.RLock()
	defer cs.assignmentsMu.RUnlock()

	// 1. Reviewer must not be the implementer
	if reviewerID == implementerID {
		return fmt.Errorf("reviewer cannot be the same as implementer")
	}

	// 2. Task must exist and have matching implementer
	ta, ok := cs.taskAssignments[taskID]
	if !ok || ta.Implementer != implementerID {
		return fmt.Errorf("task %s not found or implementer mismatch", taskID)
	}

	// 3. Implementer must be in AwaitingReview phase
	implAssignment := cs.workerAssignments[implementerID]
	if implAssignment == nil || implAssignment.Phase != events.PhaseAwaitingReview {
		var phase events.WorkerPhase
		if implAssignment != nil {
			phase = implAssignment.Phase
		}
		return fmt.Errorf("implementer %s is not awaiting review (phase: %v)", implementerID, phase)
	}

	// 4. Task must not already have a reviewer
	if ta.Reviewer != "" {
		return fmt.Errorf("task %s already has reviewer %s", taskID, ta.Reviewer)
	}

	// 5. Reviewer must be Ready
	reviewer := cs.pool.GetWorker(reviewerID)
	if reviewer == nil || reviewer.GetStatus() != pool.WorkerReady {
		return fmt.Errorf("reviewer %s is not ready", reviewerID)
	}

	return nil
}

// detectOrphanedTasks finds tasks whose assigned workers have been retired.
// Returns list of orphaned task IDs.
// Used by coordinator agent via periodic health check (exposed via export_test.go).
//
//nolint:unused // Reserved for coordinator agent health monitoring
func (cs *CoordinatorServer) detectOrphanedTasks() []string {
	cs.assignmentsMu.RLock()
	defer cs.assignmentsMu.RUnlock()

	var orphans []string
	for taskID, ta := range cs.taskAssignments {
		// Check implementer
		if ta.Implementer != "" {
			implWorker := cs.pool.GetWorker(ta.Implementer)
			if implWorker == nil || implWorker.GetStatus() == pool.WorkerRetired {
				orphans = append(orphans, taskID)
				continue
			}
		}
		// Check reviewer
		if ta.Reviewer != "" {
			revWorker := cs.pool.GetWorker(ta.Reviewer)
			if revWorker == nil || revWorker.GetStatus() == pool.WorkerRetired {
				orphans = append(orphans, taskID)
			}
		}
	}
	return orphans
}

// MaxTaskDuration is the maximum time a worker should spend on a task before being considered stuck.
const MaxTaskDuration = 30 * time.Minute

// checkStuckWorkers finds workers that have exceeded MaxTaskDuration on their current task.
// Returns list of stuck worker IDs.
// Used by coordinator agent via periodic health check (exposed via export_test.go).
//
//nolint:unused // Reserved for coordinator agent health monitoring
func (cs *CoordinatorServer) checkStuckWorkers() []string {
	cs.assignmentsMu.RLock()
	defer cs.assignmentsMu.RUnlock()

	var stuck []string
	for workerID, wa := range cs.workerAssignments {
		if wa.TaskID != "" && time.Since(wa.AssignedAt) > MaxTaskDuration {
			stuck = append(stuck, workerID)
		}
	}
	return stuck
}

// workerStateInfo represents worker state in query_worker_state response.
type workerStateInfo struct {
	WorkerID     string `json:"worker_id"`
	Status       string `json:"status"`
	Phase        string `json:"phase"`
	Role         string `json:"role,omitempty"`
	TaskID       string `json:"task_id,omitempty"`
	ContextUsage string `json:"context_usage,omitempty"`
	StartedAt    string `json:"started_at"`
}

// taskAssignmentInfo represents task assignment in query_worker_state response.
type taskAssignmentInfo struct {
	Implementer string `json:"implementer"`
	Reviewer    string `json:"reviewer,omitempty"`
	Status      string `json:"status"`
}

// workerStateResponse is the response for query_worker_state tool.
type workerStateResponse struct {
	Workers         []workerStateInfo             `json:"workers"`
	TaskAssignments map[string]taskAssignmentInfo `json:"task_assignments"`
	ReadyWorkers    []string                      `json:"ready_workers"`
}

// handleQueryWorkerState returns detailed worker state including phase and role.
func (cs *CoordinatorServer) handleQueryWorkerState(_ context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	var args queryWorkerStateArgs
	if len(rawArgs) > 0 {
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}

	if cs.pool == nil {
		return nil, fmt.Errorf("worker pool not available")
	}

	// Get all active workers
	workers := cs.pool.ActiveWorkers()

	// Build response
	response := workerStateResponse{
		Workers:         make([]workerStateInfo, 0),
		TaskAssignments: make(map[string]taskAssignmentInfo),
		ReadyWorkers:    make([]string, 0),
	}

	cs.assignmentsMu.RLock()
	defer cs.assignmentsMu.RUnlock()

	for _, w := range workers {
		// Filter by worker_id if specified
		if args.WorkerID != "" && w.ID != args.WorkerID {
			continue
		}

		// Get assignment info if exists
		wa := cs.workerAssignments[w.ID]

		// Filter by task_id if specified
		if args.TaskID != "" {
			if wa == nil || wa.TaskID != args.TaskID {
				continue
			}
		}

		// Build worker info
		info := workerStateInfo{
			WorkerID:  w.ID,
			Status:    w.GetStatus().String(),
			Phase:     string(events.PhaseIdle), // Default to idle
			StartedAt: w.GetStartedAt().Format("15:04:05"),
		}

		// Add assignment details if exists
		if wa != nil {
			info.Phase = string(wa.Phase)
			info.Role = string(wa.Role)
			info.TaskID = wa.TaskID
		}

		// Add context usage
		if m := w.GetMetrics(); m != nil && m.ContextTokens > 0 && m.ContextWindow > 0 {
			info.ContextUsage = formatContextUsage(m.ContextTokens, m.ContextWindow)
		}

		response.Workers = append(response.Workers, info)

		// Track ready workers
		if w.GetStatus() == pool.WorkerReady && (wa == nil || wa.TaskID == "") {
			response.ReadyWorkers = append(response.ReadyWorkers, w.ID)
		}
	}

	// Build task assignments map
	for taskID, ta := range cs.taskAssignments {
		// Filter by task_id if specified
		if args.TaskID != "" && taskID != args.TaskID {
			continue
		}

		response.TaskAssignments[taskID] = taskAssignmentInfo{
			Implementer: ta.Implementer,
			Reviewer:    ta.Reviewer,
			Status:      string(ta.Status),
		}
	}

	data, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling response: %w", err)
	}

	return StructuredResult(string(data), response), nil
}

// handleAssignTaskReview assigns a reviewer to a completed implementation.
func (cs *CoordinatorServer) handleAssignTaskReview(ctx context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	var args assignTaskReviewArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	// Validate required fields
	if args.ReviewerID == "" {
		return nil, fmt.Errorf("reviewer_id is required")
	}
	if args.TaskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	if !isValidTaskID(args.TaskID) {
		return nil, fmt.Errorf("invalid task_id format: %s", args.TaskID)
	}
	if args.ImplementerID == "" {
		return nil, fmt.Errorf("implementer_id is required")
	}
	if args.Summary == "" {
		return nil, fmt.Errorf("summary is required")
	}

	// Validate the review assignment
	if err := cs.validateReviewAssignment(args.ReviewerID, args.TaskID, args.ImplementerID); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Get reviewer worker
	reviewer := cs.pool.GetWorker(args.ReviewerID)
	if reviewer == nil {
		return nil, fmt.Errorf("reviewer not found: %s", args.ReviewerID)
	}

	sessionID := reviewer.GetSessionID()
	if sessionID == "" {
		return nil, fmt.Errorf("reviewer %s has no session ID yet", args.ReviewerID)
	}

	// Update state atomically
	cs.assignmentsMu.Lock()

	// Create reviewer assignment
	cs.workerAssignments[args.ReviewerID] = &WorkerAssignment{
		TaskID:        args.TaskID,
		Role:          RoleReviewer,
		Phase:         events.PhaseReviewing,
		AssignedAt:    time.Now(),
		ImplementerID: args.ImplementerID,
	}

	// Update task assignment with reviewer
	if ta, ok := cs.taskAssignments[args.TaskID]; ok {
		ta.Reviewer = args.ReviewerID
		ta.Status = TaskInReview
		ta.ReviewStartedAt = time.Now()
	}

	// Update implementer's assignment to note who is reviewing
	if implAssignment, ok := cs.workerAssignments[args.ImplementerID]; ok {
		implAssignment.ReviewerID = args.ReviewerID
	}

	cs.assignmentsMu.Unlock()

	// Sync pool worker phase and task ID for TUI display
	if err := cs.pool.SetWorkerPhase(args.ReviewerID, events.PhaseReviewing); err != nil {
		log.Warn(log.CatMCP, "Failed to set reviewer phase", "reviewerID", args.ReviewerID, "error", err)
	}
	if err := cs.pool.SetWorkerTaskID(args.ReviewerID, args.TaskID); err != nil {
		log.Warn(log.CatMCP, "Failed to set reviewer task ID", "reviewerID", args.ReviewerID, "error", err)
	}

	// Generate MCP config for reviewer
	mcpConfig, err := cs.generateWorkerMCPConfig(args.ReviewerID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate MCP config: %w", err)
	}

	// Build review prompt
	prompt := ReviewAssignmentPrompt(args.TaskID, args.ImplementerID, args.Summary)

	// Resume reviewer with review assignment
	proc, err := cs.client.Spawn(ctx, client.Config{
		WorkDir:         cs.workDir,
		SessionID:       sessionID,
		Prompt:          prompt,
		MCPConfig:       mcpConfig,
		SkipPermissions: true,
		DisallowedTools: []string{"AskUserQuestion"},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to send review to reviewer: %w", err)
	}

	// Resume reviewer in pool
	if err := cs.pool.ResumeWorker(args.ReviewerID, proc); err != nil {
		return nil, fmt.Errorf("failed to resume reviewer: %w", err)
	}

	// Emit event for TUI
	cs.pool.EmitIncomingMessage(args.ReviewerID, args.TaskID, prompt)

	// Add BD comment for audit trail
	if err := cs.beadsExecutor.AddComment(args.TaskID, "coordinator", fmt.Sprintf("Review assigned to %s", args.ReviewerID)); err != nil {
		log.Debug(log.CatMCP, "bd comment failed", "taskID", args.TaskID, "error", err)
	}

	log.Debug(log.CatMCP, "Assigned review", "reviewerID", args.ReviewerID, "taskID", args.TaskID, "implementerID", args.ImplementerID)

	// Return structured response
	response := map[string]any{
		"status":  "success",
		"message": fmt.Sprintf("Review of %s assigned to %s", args.TaskID, args.ReviewerID),
		"reviewer_state": map[string]string{
			"worker_id": args.ReviewerID,
			"phase":     string(events.PhaseReviewing),
			"task_id":   args.TaskID,
		},
	}
	data, _ := json.MarshalIndent(response, "", "  ")
	return StructuredResult(string(data), response), nil
}

// handleAssignReviewFeedback sends review feedback to implementer requiring changes.
func (cs *CoordinatorServer) handleAssignReviewFeedback(ctx context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	var args assignReviewFeedbackArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	// Validate required fields
	if args.ImplementerID == "" {
		return nil, fmt.Errorf("implementer_id is required")
	}
	if args.TaskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	if !isValidTaskID(args.TaskID) {
		return nil, fmt.Errorf("invalid task_id format: %s", args.TaskID)
	}
	if args.Feedback == "" {
		return nil, fmt.Errorf("feedback is required")
	}

	// Validate state
	cs.assignmentsMu.RLock()
	ta, ok := cs.taskAssignments[args.TaskID]
	if !ok {
		cs.assignmentsMu.RUnlock()
		return nil, fmt.Errorf("task %s not found in assignments", args.TaskID)
	}
	if ta.Implementer != args.ImplementerID {
		cs.assignmentsMu.RUnlock()
		return nil, fmt.Errorf("worker %s is not the implementer of task %s", args.ImplementerID, args.TaskID)
	}
	if ta.Status != TaskDenied {
		cs.assignmentsMu.RUnlock()
		return nil, fmt.Errorf("task %s is not in denied status (current: %s)", args.TaskID, ta.Status)
	}
	cs.assignmentsMu.RUnlock()

	// Get implementer worker
	implementer := cs.pool.GetWorker(args.ImplementerID)
	if implementer == nil {
		return nil, fmt.Errorf("implementer not found: %s", args.ImplementerID)
	}

	sessionID := implementer.GetSessionID()
	if sessionID == "" {
		return nil, fmt.Errorf("implementer %s has no session ID yet", args.ImplementerID)
	}

	// Update state atomically
	cs.assignmentsMu.Lock()
	if implAssignment, ok := cs.workerAssignments[args.ImplementerID]; ok {
		implAssignment.Phase = events.PhaseAddressingFeedback
	}
	cs.assignmentsMu.Unlock()

	// Sync pool worker phase for TUI display
	if err := cs.pool.SetWorkerPhase(args.ImplementerID, events.PhaseAddressingFeedback); err != nil {
		log.Warn(log.CatMCP, "Failed to set implementer phase", "implementerID", args.ImplementerID, "error", err)
	}

	// Generate MCP config for implementer
	mcpConfig, err := cs.generateWorkerMCPConfig(args.ImplementerID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate MCP config: %w", err)
	}

	// Build feedback prompt
	prompt := ReviewFeedbackPrompt(args.TaskID, args.Feedback)

	// Resume implementer with feedback
	proc, err := cs.client.Spawn(ctx, client.Config{
		WorkDir:         cs.workDir,
		SessionID:       sessionID,
		Prompt:          prompt,
		MCPConfig:       mcpConfig,
		SkipPermissions: true,
		DisallowedTools: []string{"AskUserQuestion"},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to send feedback to implementer: %w", err)
	}

	// Resume implementer in pool
	if err := cs.pool.ResumeWorker(args.ImplementerID, proc); err != nil {
		return nil, fmt.Errorf("failed to resume implementer: %w", err)
	}

	// Emit event for TUI
	cs.pool.EmitIncomingMessage(args.ImplementerID, args.TaskID, prompt)

	// Add BD comment for audit trail
	if err := cs.beadsExecutor.AddComment(args.TaskID, "coordinator", fmt.Sprintf("Review feedback sent to %s: %s", args.ImplementerID, args.Feedback)); err != nil {
		log.Debug(log.CatMCP, "bd comment failed", "taskID", args.TaskID, "error", err)
	}

	log.Debug(log.CatMCP, "Sent review feedback", "implementerID", args.ImplementerID, "taskID", args.TaskID)

	// Return structured response
	response := map[string]any{
		"status":  "success",
		"message": fmt.Sprintf("Feedback sent to %s for %s", args.ImplementerID, args.TaskID),
		"implementer_state": map[string]string{
			"worker_id": args.ImplementerID,
			"phase":     string(events.PhaseAddressingFeedback),
			"task_id":   args.TaskID,
		},
	}
	data, _ := json.MarshalIndent(response, "", "  ")
	return StructuredResult(string(data), response), nil
}

// handleApproveCommit approves implementation and instructs worker to commit.
func (cs *CoordinatorServer) handleApproveCommit(ctx context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	var args approveCommitArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	// Validate required fields
	if args.ImplementerID == "" {
		return nil, fmt.Errorf("implementer_id is required")
	}
	if args.TaskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	if !isValidTaskID(args.TaskID) {
		return nil, fmt.Errorf("invalid task_id format: %s", args.TaskID)
	}

	// Validate state
	cs.assignmentsMu.RLock()
	ta, ok := cs.taskAssignments[args.TaskID]
	if !ok {
		cs.assignmentsMu.RUnlock()
		return nil, fmt.Errorf("task %s not found in assignments", args.TaskID)
	}
	if ta.Implementer != args.ImplementerID {
		cs.assignmentsMu.RUnlock()
		return nil, fmt.Errorf("worker %s is not the implementer of task %s", args.ImplementerID, args.TaskID)
	}
	if ta.Status != TaskApproved {
		cs.assignmentsMu.RUnlock()
		return nil, fmt.Errorf("task %s is not in approved status (current: %s)", args.TaskID, ta.Status)
	}
	cs.assignmentsMu.RUnlock()

	// Get implementer worker
	implementer := cs.pool.GetWorker(args.ImplementerID)
	if implementer == nil {
		return nil, fmt.Errorf("implementer not found: %s", args.ImplementerID)
	}

	sessionID := implementer.GetSessionID()
	if sessionID == "" {
		return nil, fmt.Errorf("implementer %s has no session ID yet", args.ImplementerID)
	}

	// Update state atomically
	cs.assignmentsMu.Lock()
	if implAssignment, ok := cs.workerAssignments[args.ImplementerID]; ok {
		implAssignment.Phase = events.PhaseCommitting
	}
	if ta, ok := cs.taskAssignments[args.TaskID]; ok {
		ta.Status = TaskCommitting
	}
	cs.assignmentsMu.Unlock()

	// Sync pool worker phase for TUI display
	if err := cs.pool.SetWorkerPhase(args.ImplementerID, events.PhaseCommitting); err != nil {
		log.Warn(log.CatMCP, "Failed to set implementer phase", "implementerID", args.ImplementerID, "error", err)
	}

	// Generate MCP config for implementer
	mcpConfig, err := cs.generateWorkerMCPConfig(args.ImplementerID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate MCP config: %w", err)
	}

	// Build commit prompt
	prompt := CommitApprovalPrompt(args.TaskID, args.CommitMessage)

	// Resume implementer with commit instruction
	proc, err := cs.client.Spawn(ctx, client.Config{
		WorkDir:         cs.workDir,
		SessionID:       sessionID,
		Prompt:          prompt,
		MCPConfig:       mcpConfig,
		SkipPermissions: true,
		DisallowedTools: []string{"AskUserQuestion"},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to send commit instruction to implementer: %w", err)
	}

	// Resume implementer in pool
	if err := cs.pool.ResumeWorker(args.ImplementerID, proc); err != nil {
		return nil, fmt.Errorf("failed to resume implementer: %w", err)
	}

	// Emit event for TUI
	cs.pool.EmitIncomingMessage(args.ImplementerID, args.TaskID, prompt)

	// Add BD comment for audit trail
	if err := cs.beadsExecutor.AddComment(args.TaskID, "coordinator", "Commit approved"); err != nil {
		log.Debug(log.CatMCP, "bd comment failed", "taskID", args.TaskID, "error", err)
	}

	log.Debug(log.CatMCP, "Approved commit", "implementerID", args.ImplementerID, "taskID", args.TaskID)

	// Return structured response
	response := map[string]any{
		"status":  "success",
		"message": fmt.Sprintf("Commit approved for %s", args.TaskID),
		"implementer_state": map[string]string{
			"worker_id": args.ImplementerID,
			"phase":     string(events.PhaseCommitting),
			"task_id":   args.TaskID,
		},
	}
	data, _ := json.MarshalIndent(response, "", "  ")
	return StructuredResult(string(data), response), nil
}

// WorkerStateCallback implementation - allows workers to update coordinator state.
// These methods are called by worker MCP tools (report_implementation_complete, report_review_verdict).

// GetWorkerPhase returns the current phase for a worker.
// Implements WorkerStateCallback interface.
func (cs *CoordinatorServer) GetWorkerPhase(workerID string) (events.WorkerPhase, error) {
	cs.assignmentsMu.RLock()
	defer cs.assignmentsMu.RUnlock()

	wa, ok := cs.workerAssignments[workerID]
	if !ok || wa == nil {
		return events.PhaseIdle, nil
	}
	return wa.Phase, nil
}

// OnImplementationComplete is called when a worker reports implementation is done.
// Updates the worker's phase to AwaitingReview and adds BD comment.
// Implements WorkerStateCallback interface.
func (cs *CoordinatorServer) OnImplementationComplete(workerID, summary string) error {
	cs.assignmentsMu.Lock()
	defer cs.assignmentsMu.Unlock()

	// Get worker assignment
	wa, ok := cs.workerAssignments[workerID]
	if !ok || wa == nil {
		return fmt.Errorf("worker %s has no active assignment", workerID)
	}

	// Validate phase - allow both implementing and addressing_feedback
	if wa.Phase != events.PhaseImplementing && wa.Phase != events.PhaseAddressingFeedback {
		return fmt.Errorf("worker %s is not in implementing or addressing_feedback phase (current: %s)", workerID, wa.Phase)
	}

	// Update worker phase
	wa.Phase = events.PhaseAwaitingReview

	// Sync pool worker phase for TUI display
	if err := cs.pool.SetWorkerPhase(workerID, events.PhaseAwaitingReview); err != nil {
		log.Warn(log.CatMCP, "Failed to set worker phase", "workerID", workerID, "error", err)
	}

	// Add BD comment for audit trail
	if wa.TaskID != "" {
		if err := cs.beadsExecutor.AddComment(wa.TaskID, "coordinator", fmt.Sprintf("Implementation complete by %s: %s", workerID, summary)); err != nil {
			log.Debug(log.CatMCP, "bd comment failed", "taskID", wa.TaskID, "error", err)
		}
	}

	log.Debug(log.CatMCP, "Worker implementation complete", "workerID", workerID, "taskID", wa.TaskID, "summary", summary)
	return nil
}

// OnReviewVerdict is called when a reviewer reports their verdict.
// Updates the task status to Approved or Denied, transitions reviewer to Idle.
// Implements WorkerStateCallback interface.
func (cs *CoordinatorServer) OnReviewVerdict(workerID, verdict, comments string) error {
	cs.assignmentsMu.Lock()
	defer cs.assignmentsMu.Unlock()

	// Get worker assignment
	wa, ok := cs.workerAssignments[workerID]
	if !ok || wa == nil {
		return fmt.Errorf("worker %s has no active assignment", workerID)
	}

	// Validate phase
	if wa.Phase != events.PhaseReviewing {
		return fmt.Errorf("worker %s is not in reviewing phase (current: %s)", workerID, wa.Phase)
	}

	// Validate verdict
	if verdict != "APPROVED" && verdict != "DENIED" {
		return fmt.Errorf("invalid verdict: %s (must be APPROVED or DENIED)", verdict)
	}

	taskID := wa.TaskID

	// Update task status based on verdict
	if ta, ok := cs.taskAssignments[taskID]; ok {
		if verdict == "APPROVED" {
			ta.Status = TaskApproved
		} else {
			ta.Status = TaskDenied
		}
	}

	// Transition reviewer to Idle (coordinator state only)
	// Note: Task ID is NOT cleared from pool worker - it persists as historical context
	wa.Phase = events.PhaseIdle
	wa.TaskID = ""
	wa.ImplementerID = ""

	// Sync pool worker phase to Idle for TUI display (task ID preserved)
	if err := cs.pool.SetWorkerPhase(workerID, events.PhaseIdle); err != nil {
		log.Warn(log.CatMCP, "Failed to set reviewer phase", "workerID", workerID, "error", err)
	}

	// Add BD comment for audit trail
	if taskID != "" {
		if err := cs.beadsExecutor.AddComment(taskID, "coordinator", fmt.Sprintf("Review verdict by %s: %s - %s", workerID, verdict, comments)); err != nil {
			log.Debug(log.CatMCP, "bd comment failed", "taskID", taskID, "error", err)
		}
	}

	log.Debug(log.CatMCP, "Worker review verdict", "workerID", workerID, "taskID", taskID, "verdict", verdict, "comments", comments)
	return nil
}

// SetWorkerAssignment allows tests to set worker assignments directly.
func (cs *CoordinatorServer) SetWorkerAssignment(workerID string, assignment *WorkerAssignment) {
	cs.assignmentsMu.Lock()
	defer cs.assignmentsMu.Unlock()
	cs.workerAssignments[workerID] = assignment
}

// SetTaskAssignment allows tests to set task assignments directly.
func (cs *CoordinatorServer) SetTaskAssignment(taskID string, assignment *TaskAssignment) {
	cs.assignmentsMu.Lock()
	defer cs.assignmentsMu.Unlock()
	cs.taskAssignments[taskID] = assignment
}
