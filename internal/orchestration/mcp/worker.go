package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/message"
)

// MessageStore defines the interface for message storage operations.
// This allows for dependency injection and easier testing.
type MessageStore interface {
	UnreadFor(agentID string) []message.Entry
	MarkRead(agentID string)
	Append(from, to, content string, msgType message.MessageType) (*message.Entry, error)
}

// WorkerStateCallback defines the interface for workers to notify the coordinator
// of phase transitions. This allows the coordinator to update its state tracking
// when workers call tools like report_implementation_complete or report_review_verdict.
type WorkerStateCallback interface {
	// GetWorkerPhase returns the current phase for a worker.
	GetWorkerPhase(workerID string) (events.WorkerPhase, error)
	// OnImplementationComplete is called when a worker reports implementation is done.
	// Returns error if worker is not in PhaseImplementing.
	OnImplementationComplete(workerID, summary string) error
	// OnReviewVerdict is called when a reviewer reports their verdict.
	// Returns error if worker is not in PhaseReviewing or verdict is invalid.
	OnReviewVerdict(workerID, verdict, comments string) error
}

// WorkerServer is an MCP server that exposes communication tools to worker agents.
// Each worker gets its own MCP server instance with a unique worker ID.
type WorkerServer struct {
	*Server
	workerID      string
	msgStore      MessageStore
	stateCallback WorkerStateCallback
	// dedup tracks recent messages to prevent duplicate sends to coordinator
	dedup *MessageDeduplicator
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

// SetStateCallback sets the callback interface for worker state notifications.
// This must be called before the worker tools can perform state transitions.
func (ws *WorkerServer) SetStateCallback(callback WorkerStateCallback) {
	ws.stateCallback = callback
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
}

// Tool argument structs for JSON parsing.
type sendMessageArgs struct {
	To      string `json:"to"`
	Content string `json:"content"`
}

type reportImplementationCompleteArgs struct {
	Summary string `json:"summary"`
}

type reportReviewVerdictArgs struct {
	Verdict  string `json:"verdict"`
	Comments string `json:"comments"`
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

	log.Debug(log.CatMCP, "Message sent", "workerID", ws.workerID, "to", args.To)

	return SuccessResult(fmt.Sprintf("Message sent to %s", args.To)), nil
}

// handleSignalReady signals the coordinator that this worker is ready for task assignment.
func (ws *WorkerServer) handleSignalReady(_ context.Context, _ json.RawMessage) (*ToolCallResult, error) {
	if ws.msgStore == nil {
		return nil, fmt.Errorf("message store not available")
	}

	// Send ready signal to coordinator
	readyContent := fmt.Sprintf("Worker %s ready for task assignment", ws.workerID)
	_, err := ws.msgStore.Append(ws.workerID, message.ActorCoordinator, readyContent, message.MessageWorkerReady)
	if err != nil {
		log.Debug(log.CatMCP, "Failed to signal ready", "workerID", ws.workerID, "error", err)
		return nil, fmt.Errorf("failed to signal ready: %w", err)
	}

	log.Debug(log.CatMCP, "Worker signaled ready", "workerID", ws.workerID)

	return SuccessResult("Ready signal sent to coordinator"), nil
}

// handleReportImplementationComplete signals that implementation is complete and ready for review.
func (ws *WorkerServer) handleReportImplementationComplete(_ context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	var args reportImplementationCompleteArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if args.Summary == "" {
		return nil, fmt.Errorf("summary is required")
	}

	// Validate state callback is available
	if ws.stateCallback == nil {
		return nil, fmt.Errorf("state callback not configured - cannot perform state transitions")
	}

	// Validate worker is in PhaseImplementing
	phase, err := ws.stateCallback.GetWorkerPhase(ws.workerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get worker phase: %w", err)
	}
	if phase != events.PhaseImplementing && phase != events.PhaseAddressingFeedback {
		return nil, fmt.Errorf("worker %s is not in implementing or addressing_feedback phase (current: %s)", ws.workerID, phase)
	}

	// Call the callback to update coordinator state
	if err := ws.stateCallback.OnImplementationComplete(ws.workerID, args.Summary); err != nil {
		return nil, fmt.Errorf("failed to update state: %w", err)
	}

	// Post message to coordinator
	if ws.msgStore != nil {
		content := fmt.Sprintf("Implementation complete: %s", args.Summary)
		if _, err := ws.msgStore.Append(ws.workerID, message.ActorCoordinator, content, message.MessageInfo); err != nil {
			log.Warn(log.CatMCP, "Failed to post implementation complete message", "workerID", ws.workerID, "error", err)
		}
	}

	log.Debug(log.CatMCP, "Worker reported implementation complete", "workerID", ws.workerID, "summary", args.Summary)

	// Return structured response
	response := map[string]any{
		"status":  "success",
		"message": "Implementation complete, awaiting review",
		"phase":   string(events.PhaseAwaitingReview),
	}
	data, _ := json.MarshalIndent(response, "", "  ")
	return StructuredResult(string(data), response), nil
}

// handleReportReviewVerdict reports the code review verdict (APPROVED or DENIED).
func (ws *WorkerServer) handleReportReviewVerdict(_ context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	var args reportReviewVerdictArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if args.Verdict == "" {
		return nil, fmt.Errorf("verdict is required")
	}
	if args.Comments == "" {
		return nil, fmt.Errorf("comments is required")
	}

	// Validate verdict value
	if args.Verdict != "APPROVED" && args.Verdict != "DENIED" {
		return nil, fmt.Errorf("verdict must be 'APPROVED' or 'DENIED', got '%s'", args.Verdict)
	}

	// Validate state callback is available
	if ws.stateCallback == nil {
		return nil, fmt.Errorf("state callback not configured - cannot perform state transitions")
	}

	// Validate worker is in PhaseReviewing
	phase, err := ws.stateCallback.GetWorkerPhase(ws.workerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get worker phase: %w", err)
	}
	if phase != events.PhaseReviewing {
		return nil, fmt.Errorf("worker %s is not in reviewing phase (current: %s)", ws.workerID, phase)
	}

	// Call the callback to update coordinator state
	if err := ws.stateCallback.OnReviewVerdict(ws.workerID, args.Verdict, args.Comments); err != nil {
		return nil, fmt.Errorf("failed to update state: %w", err)
	}

	// Post message to coordinator
	if ws.msgStore != nil {
		content := fmt.Sprintf("Review verdict: %s - %s", args.Verdict, args.Comments)
		if _, err := ws.msgStore.Append(ws.workerID, message.ActorCoordinator, content, message.MessageInfo); err != nil {
			log.Warn(log.CatMCP, "Failed to post review verdict message", "workerID", ws.workerID, "error", err)
		}
	}

	log.Debug(log.CatMCP, "Worker reported review verdict", "workerID", ws.workerID, "verdict", args.Verdict)

	// Return structured response
	response := map[string]any{
		"status":   "success",
		"message":  fmt.Sprintf("Review verdict reported: %s", args.Verdict),
		"verdict":  args.Verdict,
		"phase":    string(events.PhaseIdle), // Reviewer returns to idle
		"comments": args.Comments,
	}
	data, _ := json.MarshalIndent(response, "", "  ")
	return StructuredResult(string(data), response), nil
}
