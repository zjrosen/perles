package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"perles/internal/log"
	"perles/internal/orchestration/message"
)

// MessageStore defines the interface for message storage operations.
// This allows for dependency injection and easier testing.
type MessageStore interface {
	UnreadFor(agentID string) []message.Entry
	MarkRead(agentID string)
	Append(from, to, content string, msgType message.MessageType) (*message.Entry, error)
}

// WorkerServer is an MCP server that exposes communication tools to worker agents.
// Each worker gets its own MCP server instance with a unique worker ID.
type WorkerServer struct {
	*Server
	workerID string
	msgStore MessageStore
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
	}

	ws.registerTools()
	return ws
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

	// signal_coordinator - Urgent notification
	ws.RegisterTool(Tool{
		Name:        "signal_coordinator",
		Description: "Send an urgent signal to the coordinator. Use when blocked, need a decision, or encountered an error.",
		InputSchema: &InputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"reason": {Type: "string", Description: "Reason for signaling (e.g., 'blocked on dependency', 'need clarification')"},
			},
			Required: []string{"reason"},
		},
	}, ws.handleSignalCoordinator)
}

// Tool argument structs for JSON parsing.
type sendMessageArgs struct {
	To      string `json:"to"`
	Content string `json:"content"`
}

type signalCoordinatorArgs struct {
	Reason string `json:"reason"`
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

	log.Debug(logCat, "Returned unread messages", "workerID", ws.workerID, "count", len(unread))
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

	_, err := ws.msgStore.Append(ws.workerID, args.To, args.Content, message.MessageInfo)
	if err != nil {
		log.Debug(logCat, "Failed to send message", "workerID", ws.workerID, "to", args.To, "error", err)
		return nil, fmt.Errorf("failed to send message: %w", err)
	}

	log.Debug(logCat, "Message sent", "workerID", ws.workerID, "to", args.To)

	return SuccessResult(fmt.Sprintf("Message sent to %s", args.To)), nil
}

// handleSignalCoordinator sends an urgent signal to the coordinator.
func (ws *WorkerServer) handleSignalCoordinator(_ context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	var args signalCoordinatorArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if args.Reason == "" {
		return nil, fmt.Errorf("reason is required")
	}

	if ws.msgStore == nil {
		return nil, fmt.Errorf("message store not available")
	}

	// Send as an urgent request message to coordinator
	signalContent := fmt.Sprintf("[URGENT SIGNAL] %s", args.Reason)
	_, err := ws.msgStore.Append(ws.workerID, message.ActorCoordinator, signalContent, message.MessageRequest)
	if err != nil {
		log.Debug(logCat, "Failed to signal coordinator", "workerID", ws.workerID, "reason", args.Reason, "error", err)
		return nil, fmt.Errorf("failed to signal coordinator: %w", err)
	}

	log.Debug(logCat, "Signaled coordinator", "workerID", ws.workerID, "reason", args.Reason)

	return SuccessResult(fmt.Sprintf("Urgent signal sent to coordinator: %s", args.Reason)), nil
}
