package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/orchestration/v2/adapter"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
	"github.com/zjrosen/perles/internal/orchestration/validation"
)

// Validation constants for post_accountability_summary tool.
const (
	// MinSummaryLength is the minimum length for an accountability summary (at least a sentence).
	MinSummaryLength = 20
)

// MessageStore defines the interface for message storage operations.
// This allows for dependency injection and easier testing.
// Note: This interface is a subset of repository.MessageRepository.
// MemoryMessageRepository satisfies this interface, verified by compile-time assertion below.
type MessageStore interface {
	UnreadFor(agentID string) []message.Entry
	MarkRead(agentID string)
	Append(from, to, content string, msgType message.MessageType) (*message.Entry, error)
}

// Compile-time interface assertion: MemoryMessageRepository satisfies MessageStore.
// This ensures compatibility with the v2 MessageRepository migration.
var _ MessageStore = (*repository.MemoryMessageRepository)(nil)

// AccountabilityWriter defines the interface for writing worker accountability summaries.
// This allows the session service to handle storage without tight coupling.
type AccountabilityWriter interface {
	// WriteWorkerAccountabilitySummary saves a worker's accountability summary to their session directory.
	// Returns the file path where the summary was saved.
	// Note: taskID is embedded in the YAML frontmatter of the content, not passed as parameter.
	WriteWorkerAccountabilitySummary(workerID string, content []byte) (string, error)
}

// ToolCallRecorder defines the interface for recording tool calls during worker turns.
// This is a subset of the TurnCompletionEnforcer interface from handler package,
// defined here to avoid import cycles. The handler.TurnCompletionTracker implements
// this interface.
type ToolCallRecorder interface {
	// RecordToolCall records that a worker called a specific tool.
	// Called from MCP tool handlers when a required tool is invoked.
	RecordToolCall(processID, toolName string)
}

// WorkerServer is an MCP server that exposes communication tools to worker agents.
// Each worker gets its own MCP server instance with a unique worker ID.
type WorkerServer struct {
	*Server
	workerID             string
	msgStore             MessageStore
	accountabilityWriter AccountabilityWriter
	// dedup tracks recent messages to prevent duplicate sends to coordinator
	dedup *MessageDeduplicator

	// V2 adapter for command-based processing
	// See docs/proposals/orchestration-v2-architecture.md for architecture details
	v2Adapter *adapter.V2Adapter

	// enforcer tracks tool calls for turn completion enforcement.
	// When set, required tool calls are recorded so the orchestrator can verify
	// workers properly complete their turns.
	enforcer ToolCallRecorder
}

// NewWorkerServer creates a new worker MCP server.
// Note: Instructions are generated dynamically via WorkerSystemPrompt.
// The full instructions are provided via AppendSystemPrompt when spawning the worker.
func NewWorkerServer(workerID string, msgStore MessageStore) *WorkerServer {
	// Generate instructions for this worker
	instructions := WorkerSystemPrompt(workerID)

	ws := &WorkerServer{
		Server:   NewServer("perles-worker", "1.0.0", WithInstructions(instructions)),
		workerID: workerID,
		msgStore: msgStore,
		dedup:    NewMessageDeduplicator(DefaultDeduplicationWindow),
	}

	ws.registerTools()
	return ws
}

// SetAccountabilityWriter sets the accountability writer for saving worker accountability summaries.
// This must be called before the post_accountability_summary tool can be used.
func (ws *WorkerServer) SetAccountabilityWriter(writer AccountabilityWriter) {
	ws.accountabilityWriter = writer
}

// SetV2Adapter allows setting the v2 adapter after construction.
func (ws *WorkerServer) SetV2Adapter(adapter *adapter.V2Adapter) {
	ws.v2Adapter = adapter
}

// SetTurnEnforcer sets the turn completion enforcer for tracking tool calls.
// When set, required tool calls (post_message, report_implementation_complete,
// report_review_verdict, signal_ready) are recorded so the orchestrator can
// verify workers properly complete their turns.
// The enforcer should implement ToolCallRecorder (handler.TurnCompletionTracker satisfies this).
func (ws *WorkerServer) SetTurnEnforcer(enforcer ToolCallRecorder) {
	ws.enforcer = enforcer
}

// registerTools registers all worker tools with the MCP server.
func (ws *WorkerServer) registerTools() {
	// check_messages - Pull-based message retrieval
	ws.RegisterTool(Tool{
		Name:        "check_messages",
		Description: "Check for new messages addressed to this worker. Returns structured JSON with unread messages from the coordinator or other workers.",
		InputSchema: &InputSchema{
			Type:       "object",
			Properties: map[string]*PropertySchema{},
			Required:   []string{},
		},
		OutputSchema: &OutputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"unread_count": {Type: "number", Description: "Number of unread messages"},
				"messages": {
					Type:        "array",
					Description: "List of unread messages",
					Items: &PropertySchema{
						Type: "object",
						Properties: map[string]*PropertySchema{
							"timestamp": {Type: "string", Description: "Message timestamp (HH:MM:SS format)"},
							"from":      {Type: "string", Description: "Sender ID (COORDINATOR, WORKER.N, etc.)"},
							"to":        {Type: "string", Description: "Recipient ID (ALL, WORKER.N, etc.)"},
							"content":   {Type: "string", Description: "Message content"},
						},
						Required: []string{"timestamp", "from", "to", "content"},
					},
				},
			},
			Required: []string{"unread_count", "messages"},
		},
	}, ws.handleCheckMessages)

	// post_message - Post a message to the message log
	ws.RegisterTool(Tool{
		Name:        "post_message",
		Description: "Send a message to the coordinator, other workers, or ALL agents.",
		InputSchema: &InputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"to":      {Type: "string", Description: "Recipient: 'COORDINATOR', 'ALL', or a worker ID (e.g., 'WORKER.2')"},
				"content": {Type: "string", Description: "Message content"},
			},
			Required: []string{"to", "content"},
		},
	}, ws.handlePostMessage)

	// signal_ready - Worker ready notification
	ws.RegisterTool(Tool{
		Name:        "signal_ready",
		Description: "Signal that you are ready for task assignment. Call this once when you first boot up.",
		InputSchema: &InputSchema{
			Type:       "object",
			Properties: map[string]*PropertySchema{},
			Required:   []string{},
		},
	}, ws.handleSignalReady)

	// report_implementation_complete - Signal implementation is done
	ws.RegisterTool(Tool{
		Name:        "report_implementation_complete",
		Description: "Signal that implementation is complete and ready for review. Call this when you have finished implementing the assigned task.",
		InputSchema: &InputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"summary": {Type: "string", Description: "Brief summary of what was implemented"},
			},
			Required: []string{"summary"},
		},
	}, ws.handleReportImplementationComplete)

	// report_review_verdict - Report code review verdict
	ws.RegisterTool(Tool{
		Name:        "report_review_verdict",
		Description: "Report your code review verdict. Use APPROVED if the implementation meets all criteria, DENIED if changes are required.",
		InputSchema: &InputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"verdict":  {Type: "string", Description: "Review verdict: 'APPROVED' or 'DENIED'"},
				"comments": {Type: "string", Description: "Review comments explaining the verdict"},
			},
			Required: []string{"verdict", "comments"},
		},
	}, ws.handleReportReviewVerdict)

	// post_accountability_summary - Save worker accountability summary to session directory
	ws.RegisterTool(Tool{
		Name:        "post_accountability_summary",
		Description: "Save your accountability summary for the completed task. Call this after committing to document what was accomplished, commits made, issues discovered/closed, verification points, and retro feedback.",
		InputSchema: &InputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"task_id":             {Type: "string", Description: "The task ID this summary is for"},
				"summary":             {Type: "string", Description: "What was accomplished (narrative, 2-3 sentences)"},
				"commits":             {Type: "array", Description: "List of commit hashes made (optional)", Items: &PropertySchema{Type: "string"}},
				"issues_discovered":   {Type: "array", Description: "bd IDs of bugs/blockers found during work (optional)", Items: &PropertySchema{Type: "string"}},
				"issues_closed":       {Type: "array", Description: "bd IDs of issues closed this session (optional)", Items: &PropertySchema{Type: "string"}},
				"verification_points": {Type: "array", Description: "How acceptance criteria were verified (optional)", Items: &PropertySchema{Type: "string"}},
				"retro": {
					Type:        "object",
					Description: "Structured retro feedback (optional)",
					Properties: map[string]*PropertySchema{
						"went_well": {Type: "string", Description: "What went well during the task"},
						"friction":  {Type: "string", Description: "What caused friction or slowdowns"},
						"patterns":  {Type: "string", Description: "Patterns noticed that could be applied elsewhere"},
						"takeaways": {Type: "string", Description: "Key takeaways for future work"},
					},
				},
				"next_steps": {Type: "string", Description: "Recommendations for follow-up work (optional)"},
			},
			Required: []string{"task_id", "summary"},
		},
		OutputSchema: &OutputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"status":    {Type: "string", Description: "Success or error status"},
				"file_path": {Type: "string", Description: "Path where accountability summary was saved"},
				"message":   {Type: "string", Description: "Human-readable result message"},
			},
			Required: []string{"status", "message"},
		},
	}, ws.handlePostAccountabilitySummary)
}

// Tool argument structs for JSON parsing.
type sendMessageArgs struct {
	To      string `json:"to"`
	Content string `json:"content"`
}

// RetroFeedback contains structured retrospective feedback for accountability summaries.
type RetroFeedback struct {
	WentWell  string `json:"went_well,omitempty"`
	Friction  string `json:"friction,omitempty"`
	Patterns  string `json:"patterns,omitempty"`
	Takeaways string `json:"takeaways,omitempty"`
}

// postAccountabilitySummaryArgs defines the arguments for the post_accountability_summary tool.
type postAccountabilitySummaryArgs struct {
	TaskID             string         `json:"task_id"`
	Summary            string         `json:"summary"`
	Commits            []string       `json:"commits,omitempty"`
	IssuesDiscovered   []string       `json:"issues_discovered,omitempty"`
	IssuesClosed       []string       `json:"issues_closed,omitempty"`
	VerificationPoints []string       `json:"verification_points,omitempty"`
	Retro              *RetroFeedback `json:"retro,omitempty"`
	NextSteps          string         `json:"next_steps,omitempty"`
}

// checkMessagesResponse is the structured response for check_messages.
type checkMessagesResponse struct {
	UnreadCount int                 `json:"unread_count"`
	Messages    []checkMessageEntry `json:"messages"`
}

// checkMessageEntry is a single unread message.
type checkMessageEntry struct {
	Timestamp string `json:"timestamp"` // HH:MM:SS format
	From      string `json:"from"`
	To        string `json:"to"`
	Content   string `json:"content"`
}

// handleCheckMessages returns unread messages for this worker.
func (ws *WorkerServer) handleCheckMessages(_ context.Context, _ json.RawMessage) (*ToolCallResult, error) {
	if ws.msgStore == nil {
		return nil, fmt.Errorf("message store not available")
	}

	// Get unread messages for this worker
	unread := ws.msgStore.UnreadFor(ws.workerID)

	// Mark as read after retrieving
	ws.msgStore.MarkRead(ws.workerID)

	// Build structured response
	messages := make([]checkMessageEntry, len(unread))
	for i, entry := range unread {
		messages[i] = checkMessageEntry{
			Timestamp: entry.Timestamp.Format("15:04:05"),
			From:      entry.From,
			To:        entry.To,
			Content:   entry.Content,
		}
	}

	response := checkMessagesResponse{
		UnreadCount: len(messages),
		Messages:    messages,
	}

	data, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling response: %w", err)
	}

	log.Debug(log.CatMCP, "Returned unread messages", "workerID", ws.workerID, "count", len(unread))
	return SuccessResult(string(data)), nil
}

// handlePostMessage posts a message to the message log from this worker.
func (ws *WorkerServer) handlePostMessage(_ context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	var args sendMessageArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if args.To == "" {
		return nil, fmt.Errorf("to is required")
	}
	if args.Content == "" {
		return nil, fmt.Errorf("content is required")
	}

	if ws.msgStore == nil {
		return nil, fmt.Errorf("message store not available")
	}

	// Check for duplicate message within deduplication window
	if ws.dedup.IsDuplicate(ws.workerID, args.Content) {
		log.Debug(log.CatMCP, "Duplicate message suppressed", "workerID", ws.workerID)
		return SuccessResult(fmt.Sprintf("Message sent to %s", args.To)), nil
	}

	_, err := ws.msgStore.Append(ws.workerID, args.To, args.Content, message.MessageInfo)
	if err != nil {
		log.Debug(log.CatMCP, "Failed to send message", "workerID", ws.workerID, "to", args.To, "error", err)
		return nil, fmt.Errorf("failed to send message: %w", err)
	}

	// Record tool call for turn completion enforcement
	if ws.enforcer != nil {
		ws.enforcer.RecordToolCall(ws.workerID, "post_message")
	}

	log.Debug(log.CatMCP, "Message sent", "workerID", ws.workerID, "to", args.To)

	return SuccessResult(fmt.Sprintf("Message sent to %s", args.To)), nil
}

// handleSignalReady signals the coordinator that this worker is ready for task assignment.
func (ws *WorkerServer) handleSignalReady(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
	result, err := ws.v2Adapter.HandleSignalReady(ctx, args, ws.workerID)
	if err != nil {
		return result, err
	}

	// Record tool call for turn completion enforcement
	if ws.enforcer != nil {
		ws.enforcer.RecordToolCall(ws.workerID, "signal_ready")
	}

	return result, nil
}

// handleReportImplementationComplete signals that implementation is complete and ready for review.
func (ws *WorkerServer) handleReportImplementationComplete(ctx context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	result, err := ws.v2Adapter.HandleReportImplementationComplete(ctx, rawArgs, ws.workerID)
	if err != nil {
		return result, err
	}

	// Record tool call for turn completion enforcement
	if ws.enforcer != nil {
		ws.enforcer.RecordToolCall(ws.workerID, "report_implementation_complete")
	}

	return result, nil
}

// handleReportReviewVerdict reports the code review verdict (APPROVED or DENIED).
func (ws *WorkerServer) handleReportReviewVerdict(ctx context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	result, err := ws.v2Adapter.HandleReportReviewVerdict(ctx, rawArgs, ws.workerID)
	if err != nil {
		return result, err
	}

	// Record tool call for turn completion enforcement
	if ws.enforcer != nil {
		ws.enforcer.RecordToolCall(ws.workerID, "report_review_verdict")
	}

	return result, nil
}

// validateAccountabilitySummaryArgs validates the arguments for the post_accountability_summary tool.
// It checks task_id format (to prevent path traversal), summary length bounds,
// and total content length.
func validateAccountabilitySummaryArgs(args postAccountabilitySummaryArgs) error {
	// Validate task_id is not empty
	if args.TaskID == "" {
		return fmt.Errorf("task_id is required")
	}

	// Validate task_id format to prevent path traversal attacks
	// Reject patterns containing ".." or "/" which could escape the session directory
	if strings.Contains(args.TaskID, "..") || strings.Contains(args.TaskID, "/") {
		return fmt.Errorf("invalid task_id format: contains path traversal characters")
	}

	// Validate task_id matches expected format
	if !validation.IsValidTaskID(args.TaskID) {
		return fmt.Errorf("invalid task_id format: %s", args.TaskID)
	}

	// Validate summary is not empty
	if args.Summary == "" {
		return fmt.Errorf("summary is required")
	}

	// Validate summary length bounds
	if len(args.Summary) < MinSummaryLength {
		return fmt.Errorf("summary too short (min %d chars, got %d)", MinSummaryLength, len(args.Summary))
	}

	return nil
}

// buildAccountabilitySummaryMarkdown generates the markdown content for a worker accountability summary.
// It includes YAML frontmatter for programmatic access and a markdown body for human readability.
func buildAccountabilitySummaryMarkdown(workerID string, args postAccountabilitySummaryArgs) string {
	var b strings.Builder
	timestamp := time.Now().Format(time.RFC3339)

	// YAML frontmatter
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("task_id: %s\n", args.TaskID))
	b.WriteString(fmt.Sprintf("worker_id: %s\n", workerID))
	b.WriteString(fmt.Sprintf("timestamp: %s\n", timestamp))

	// Optional array fields in frontmatter
	if len(args.Commits) > 0 {
		b.WriteString("commits:\n")
		for _, commit := range args.Commits {
			b.WriteString(fmt.Sprintf("  - %s\n", commit))
		}
	}
	if len(args.IssuesDiscovered) > 0 {
		b.WriteString("issues_discovered:\n")
		for _, issue := range args.IssuesDiscovered {
			b.WriteString(fmt.Sprintf("  - %s\n", issue))
		}
	}
	if len(args.IssuesClosed) > 0 {
		b.WriteString("issues_closed:\n")
		for _, issue := range args.IssuesClosed {
			b.WriteString(fmt.Sprintf("  - %s\n", issue))
		}
	}
	b.WriteString("---\n\n")

	// Markdown body - Header with metadata
	b.WriteString("# Worker Accountability Summary\n\n")
	b.WriteString(fmt.Sprintf("**Worker:** %s\n", workerID))
	b.WriteString(fmt.Sprintf("**Task:** %s\n", args.TaskID))
	b.WriteString(fmt.Sprintf("**Date:** %s\n\n", time.Now().Format("2006-01-02 15:04:05")))

	// What I Accomplished section (always included)
	b.WriteString("## What I Accomplished\n\n")
	b.WriteString(args.Summary)
	b.WriteString("\n\n")

	// Verification Points section (optional)
	if len(args.VerificationPoints) > 0 {
		b.WriteString("## Verification Points\n\n")
		for _, point := range args.VerificationPoints {
			b.WriteString(fmt.Sprintf("- %s\n", point))
		}
		b.WriteString("\n")
	}

	// Issues Discovered section (optional)
	if len(args.IssuesDiscovered) > 0 {
		b.WriteString("## Issues Discovered\n\n")
		for _, issue := range args.IssuesDiscovered {
			b.WriteString(fmt.Sprintf("- %s\n", issue))
		}
		b.WriteString("\n")
	}

	// Retro section (optional)
	if args.Retro != nil && (args.Retro.WentWell != "" || args.Retro.Friction != "" || args.Retro.Patterns != "" || args.Retro.Takeaways != "") {
		b.WriteString("## Retro\n\n")
		if args.Retro.WentWell != "" {
			b.WriteString("### What Went Well\n\n")
			b.WriteString(args.Retro.WentWell)
			b.WriteString("\n\n")
		}
		if args.Retro.Friction != "" {
			b.WriteString("### Friction\n\n")
			b.WriteString(args.Retro.Friction)
			b.WriteString("\n\n")
		}
		if args.Retro.Patterns != "" {
			b.WriteString("### Patterns Noticed\n\n")
			b.WriteString(args.Retro.Patterns)
			b.WriteString("\n\n")
		}
		if args.Retro.Takeaways != "" {
			b.WriteString("### Takeaways\n\n")
			b.WriteString(args.Retro.Takeaways)
			b.WriteString("\n\n")
		}
	}

	// Next Steps section (optional)
	if args.NextSteps != "" {
		b.WriteString("## Next Steps\n\n")
		b.WriteString(args.NextSteps)
		b.WriteString("\n\n")
	}

	return b.String()
}

// handlePostAccountabilitySummary saves a worker's accountability summary to their session directory.
func (ws *WorkerServer) handlePostAccountabilitySummary(_ context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	var args postAccountabilitySummaryArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	// Validate input using the dedicated validation function
	if err := validateAccountabilitySummaryArgs(args); err != nil {
		return nil, err
	}

	// Check that accountabilityWriter is configured (graceful error, not panic)
	if ws.accountabilityWriter == nil {
		return nil, fmt.Errorf("accountability writer not configured")
	}

	// Build markdown content with YAML frontmatter
	content := buildAccountabilitySummaryMarkdown(ws.workerID, args)

	// Write to session directory
	filePath, err := ws.accountabilityWriter.WriteWorkerAccountabilitySummary(ws.workerID, []byte(content))
	if err != nil {
		log.Debug(log.CatMCP, "Failed to write accountability summary", "workerID", ws.workerID, "error", err)
		return nil, fmt.Errorf("failed to save accountability summary: %w", err)
	}

	log.Debug(log.CatMCP, "Worker posted accountability summary", "workerID", ws.workerID, "taskID", args.TaskID, "path", filePath)

	// Return structured response with status, file_path, message
	response := map[string]any{
		"status":    "success",
		"file_path": filePath,
		"message":   fmt.Sprintf("Accountability summary saved to %s", filePath),
	}
	data, _ := json.MarshalIndent(response, "", "  ")
	return StructuredResult(string(data), response), nil
}
