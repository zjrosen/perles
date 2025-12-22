package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"sync"

	"perles/internal/log"
	"perles/internal/orchestration/client"
	"perles/internal/orchestration/message"
	"perles/internal/orchestration/pool"
)

// taskIDPattern validates bd task IDs to prevent command injection.
// Valid formats: "prefix-xxxx" or "prefix-xxxx.N" (for subtasks)
var taskIDPattern = regexp.MustCompile(`^[a-zA-Z]+-[a-zA-Z0-9]{2,10}(\.\d+)?$`)

// CoordinatorServer is an MCP server that exposes orchestration tools to the coordinator agent.
// It provides tools for spawning workers, managing tasks, and communicating via message issues.
type CoordinatorServer struct {
	*Server
	client     client.HeadlessClient
	pool       *pool.WorkerPool
	msgIssue   *message.Issue
	workDir    string
	extensions map[string]any // Provider-specific extensions (model, mode, etc.)

	// workerTaskMap maps workerID -> taskID for lookup
	workerTaskMap map[string]string
	taskMapMu     sync.RWMutex // protects workerTaskMap
}

// NewCoordinatorServer creates a new coordinator MCP server.
// The client is used for spawning and resuming AI processes for workers.
// The extensions map holds provider-specific configuration (model, mode, etc.)
// that will be passed to workers when they are spawned.
func NewCoordinatorServer(aiClient client.HeadlessClient, workerPool *pool.WorkerPool, msgIssue *message.Issue, workDir string, extensions map[string]any) *CoordinatorServer {
	cs := &CoordinatorServer{
		Server:        NewServer("perles-orchestrator", "1.0.0", WithInstructions(coordinatorInstructions)),
		client:        aiClient,
		pool:          workerPool,
		msgIssue:      msgIssue,
		workDir:       workDir,
		extensions:    extensions,
		workerTaskMap: make(map[string]string),
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
		return GenerateWorkerConfigAmp(8765, workerID)
	default:
		return GenerateWorkerConfig(workerID, cs.workDir)
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
		Description: "Read recent messages from the shared message log. Returns structured JSON with message metadata.",
		InputSchema: &InputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"limit": {Type: "number", Description: "Maximum number of messages to return (default: 20)"},
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
}

// Tool argument structs for JSON parsing.
type assignTaskArgs struct {
	WorkerID string `json:"worker_id"`
	TaskID   string `json:"task_id"`
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
	Limit int `json:"limit,omitempty"`
}

// SpawnIdleWorker spawns a new idle worker in the pool.
// This is called internally at startup, not exposed to the coordinator.
func (cs *CoordinatorServer) SpawnIdleWorker() (string, error) {
	// Pre-generate worker ID so it can be embedded in the MCP config
	workerID := cs.pool.NextWorkerID()

	// Generate MCP config for worker with the actual worker ID
	mcpConfig, err := cs.generateWorkerMCPConfig(workerID)
	if err != nil {
		log.Debug(logCat, "Failed to generate worker MCP config", "workerID", workerID, "error", err)
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
		log.Debug(logCat, "Failed to spawn worker", "workerID", workerID, "error", err)
		return "", fmt.Errorf("failed to spawn worker: %w", err)
	}

	log.Debug(logCat, "Spawned idle worker", "workerID", workerID)

	return workerID, nil
}

// handleSpawnWorker spawns a new idle worker in the pool.
func (cs *CoordinatorServer) handleSpawnWorker(_ context.Context, _ json.RawMessage) (*ToolCallResult, error) {
	workerID, err := cs.SpawnIdleWorker()
	if err != nil {
		return nil, fmt.Errorf("failed to spawn worker: %w", err)
	}

	log.Debug(logCat, "Spawned worker via MCP tool", "workerID", workerID)
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

	// Get task details from bd
	taskInfo, err := cs.getTaskInfo(args.TaskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task info: %w", err)
	}

	// Assign task to worker in pool
	if err := cs.pool.AssignTaskToWorker(args.WorkerID, args.TaskID); err != nil {
		return nil, fmt.Errorf("failed to assign task: %w", err)
	}

	// Track worker → task mapping
	cs.taskMapMu.Lock()
	cs.workerTaskMap[args.WorkerID] = args.TaskID
	cs.taskMapMu.Unlock()

	// Get worker's session ID
	worker := cs.pool.GetWorker(args.WorkerID)
	if worker == nil {
		return nil, fmt.Errorf("worker not found: %s", args.WorkerID)
	}

	sessionID := worker.GetSessionID()
	if sessionID == "" {
		return nil, fmt.Errorf("worker %s has no session ID yet", args.WorkerID)
	}

	// Generate MCP config for worker
	mcpConfig, err := cs.generateWorkerMCPConfig(args.WorkerID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate MCP config: %w", err)
	}

	// Build task assignment prompt
	prompt := TaskAssignmentPrompt(args.TaskID, taskInfo.Title, taskInfo.Description, taskInfo.Acceptance)

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

	log.Debug(logCat, "Assigned task to worker", "workerID", args.WorkerID, "taskID", args.TaskID)
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

	// Retire the old worker
	if err := cs.pool.RetireWorker(args.WorkerID); err != nil {
		return nil, fmt.Errorf("failed to retire worker: %w", err)
	}

	// Clear task mapping for old worker
	cs.taskMapMu.Lock()
	delete(cs.workerTaskMap, args.WorkerID)
	cs.taskMapMu.Unlock()

	// Spawn a fresh replacement
	newWorkerID, err := cs.SpawnIdleWorker()
	if err != nil {
		return nil, fmt.Errorf("failed to spawn replacement: %w", err)
	}

	reason := args.Reason
	if reason == "" {
		reason = "manual replacement"
	}

	log.Debug(logCat, "Replaced worker", "oldWorkerID", args.WorkerID, "newWorkerID", newWorkerID, "reason", reason)
	return SuccessResult(fmt.Sprintf("Worker %s retired (%s). Replacement: %s", args.WorkerID, reason, newWorkerID)), nil
}

// taskDetails holds task information from bd.
type taskDetails struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Acceptance  string `json:"acceptance"`
}

// getTaskInfo fetches task details from bd.
func (cs *CoordinatorServer) getTaskInfo(taskID string) (*taskDetails, error) {
	// Run bd show to get task details
	cmd := exec.Command("bd", "show", taskID, "--json")
	cmd.Dir = cs.workDir

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bd show failed: %w", err)
	}

	// bd show returns an array
	var issues []taskDetails
	if err := json.Unmarshal(output, &issues); err != nil {
		return nil, fmt.Errorf("parsing bd output: %w", err)
	}

	if len(issues) == 0 {
		return nil, fmt.Errorf("task %s not found", taskID)
	}

	return &issues[0], nil
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
		log.Debug(logCat, "Failed to generate worker MCP config for resume", "workerID", args.WorkerID, "error", err)
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
		log.Debug(logCat, "Failed to send to worker", "workerID", args.WorkerID, "error", err)
		return nil, fmt.Errorf("failed to send message: %w", err)
	}

	// Emit an event for the incoming message so it shows in the worker pane
	cs.pool.EmitIncomingMessage(args.WorkerID, worker.TaskID, args.Message)

	// Resume the worker in the pool so events are processed and visible in the TUI
	if err := cs.pool.ResumeWorker(args.WorkerID, proc); err != nil {
		log.Debug(logCat, "Failed to resume worker in pool", "workerID", args.WorkerID, "error", err)
		return nil, fmt.Errorf("failed to resume worker: %w", err)
	}

	log.Debug(logCat, "Sent message to worker", "workerID", args.WorkerID)
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
		log.Debug(logCat, "Failed to post message", "to", args.To, "error", err)
		return nil, fmt.Errorf("failed to post message: %w", err)
	}

	log.Debug(logCat, "Posted message", "to", args.To)
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

	// Run bd show to get task info
	cmd := exec.Command("bd", "show", args.TaskID, "--json") //nolint:gosec // G204: TaskID validated above
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		log.Debug(logCat, "bd show failed", "taskID", args.TaskID, "error", err, "stderr", stderr.String())
		return nil, fmt.Errorf("bd show failed: %s", strings.TrimSpace(stderr.String()))
	}

	// Return the JSON output directly
	return SuccessResult(strings.TrimSpace(stdout.String())), nil
}

// handleMarkTaskComplete marks a task as complete in bd.
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

	// Run bd update to set status to closed
	cmd := exec.Command("bd", "update", args.TaskID, "--status", "closed", "--json") //nolint:gosec // G204: TaskID validated above
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		log.Debug(logCat, "bd update failed", "taskID", args.TaskID, "error", err, "stderr", stderr.String())
		return nil, fmt.Errorf("bd update failed: %s", strings.TrimSpace(stderr.String()))
	}

	log.Debug(logCat, "Marked task complete", "taskID", args.TaskID)
	return SuccessResult(fmt.Sprintf("Task %s marked as closed", args.TaskID)), nil
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

	// Add comment with failure reason (TaskID already validated above)
	commentCmd := exec.Command("bd", "comment", args.TaskID, "--author", "coordinator", "--", fmt.Sprintf("⚠️ Task failed: %s", args.Reason)) //nolint:gosec // G204: TaskID validated above
	var commentStderr bytes.Buffer
	commentCmd.Stderr = &commentStderr

	if err := commentCmd.Run(); err != nil {
		log.Debug(logCat, "bd comment failed", "taskID", args.TaskID, "error", err, "stderr", commentStderr.String())
		return nil, fmt.Errorf("bd comment failed: %s", strings.TrimSpace(commentStderr.String()))
	}

	log.Debug(logCat, "Marked task failed", "taskID", args.TaskID, "reason", args.Reason)
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

	allEntries := cs.msgIssue.Entries()
	totalCount := len(allEntries)

	// Get the most recent entries up to limit
	entries := allEntries
	if len(entries) > limit {
		entries = entries[len(entries)-limit:]
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
		SessionID     string `json:"session_id"`
		StartedAt     string `json:"started_at"`
		ContextTokens int    `json:"context_tokens,omitempty"` // Total context used
		ContextWindow int    `json:"context_window,omitempty"` // Max context window
		ContextUsage  string `json:"context_usage,omitempty"`  // Human-readable usage (e.g., "27k/200k")
	}

	infos := make([]workerInfo, 0, len(workers))
	for _, w := range workers {
		info := workerInfo{
			WorkerID:  w.ID,
			PID:       w.GetPID(),
			TaskID:    w.GetTaskID(),
			Status:    w.GetStatus().String(),
			SessionID: w.GetSessionID(),
			StartedAt: w.GetStartedAt().Format("15:04:05"),
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
