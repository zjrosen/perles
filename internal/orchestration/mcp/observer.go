package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/zjrosen/perles/internal/orchestration/fabric"
	"github.com/zjrosen/perles/internal/orchestration/fabric/domain"
	fabricmcp "github.com/zjrosen/perles/internal/orchestration/fabric/mcp"
	"github.com/zjrosen/perles/internal/orchestration/v2/prompt"
)

// ObserverServer is an MCP server that exposes read-only fabric tools to the Observer agent.
// The Observer can read from all channels but can only write to #observer channel.
type ObserverServer struct {
	*Server
	observerID    string
	fabricService *fabric.Service
}

// NewObserverServer creates a new observer MCP server.
// Instructions are generated dynamically via prompt.ObserverMCPInstructions.
func NewObserverServer(observerID string) *ObserverServer {
	// Generate MCP instructions for the observer
	instructions := prompt.ObserverMCPInstructions()

	os := &ObserverServer{
		Server: NewServer("perles-observer", "1.0.0",
			WithInstructions(instructions),
			WithCallerInfo("observer", observerID),
		),
		observerID: observerID,
	}

	return os
}

// SetFabricService registers Fabric messaging tools with the observer MCP server.
// This enables the observer to use read-only fabric tools (inbox, history, etc.)
// and restricted write tools (send only to #observer, reply only to #observer threads).
func (os *ObserverServer) SetFabricService(svc *fabric.Service) {
	os.fabricService = svc
	handlers := fabricmcp.NewHandlers(svc, os.observerID)
	os.registerObserverFabricTools(handlers)
}

// registerObserverFabricTools registers read-only fabric tools and
// restricted write tools for the observer.
func (os *ObserverServer) registerObserverFabricTools(h *fabricmcp.Handlers) {
	// Read-only tools - no restrictions needed
	readOnlyTools := []string{
		"fabric_inbox",
		"fabric_history",
		"fabric_read_thread",
		"fabric_subscribe",
		"fabric_ack",
	}

	for _, tool := range fabricmcp.FabricTools() {
		// Only register read-only tools and restricted write tools
		isReadOnly := slices.Contains(readOnlyTools, tool.Name)

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
			// Wrap with channel restriction - only allow #observer
			handler = os.wrapChannelRestriction(h.HandleSend)
		case "fabric_reply":
			// Wrap with reply restriction - only allow replies to #observer threads
			handler = os.wrapReplyRestriction(h.HandleReply)
		case "fabric_ack":
			handler = h.HandleAck
		case "fabric_subscribe":
			handler = h.HandleSubscribe
		case "fabric_history":
			handler = h.HandleHistory
		case "fabric_read_thread":
			handler = h.HandleReadThread
		}

		// Register read-only tools and restricted write tools
		if handler != nil && (isReadOnly || tool.Name == "fabric_send" || tool.Name == "fabric_reply") {
			os.RegisterTool(mcpTool, handler)
		}
	}
}

// sendArgs mirrors the fabric_send arguments for parsing.
type sendArgs struct {
	Channel string `json:"channel"`
	Content string `json:"content"`
	Kind    string `json:"kind,omitempty"`
}

// wrapChannelRestriction wraps a fabric_send handler to restrict it to only the #observer channel.
// This enforces the "read-many, write-one" security model for the Observer.
func (os *ObserverServer) wrapChannelRestriction(originalHandler ToolHandler) ToolHandler {
	return func(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
		// Parse args to check channel
		var sendArgs sendArgs
		if err := json.Unmarshal(args, &sendArgs); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}

		// Only allow #observer channel
		if sendArgs.Channel != domain.SlugObserver {
			return nil, fmt.Errorf("observer can only send messages to #observer channel, not #%s", sendArgs.Channel)
		}

		// Channel is allowed, proceed with original handler
		return originalHandler(ctx, args)
	}
}

// replyArgs mirrors the fabric_reply arguments for parsing.
type replyArgs struct {
	MessageID string `json:"message_id"`
	Content   string `json:"content"`
	Kind      string `json:"kind,omitempty"`
}

// wrapReplyRestriction wraps a fabric_reply handler to restrict it to only reply to messages in the #observer channel.
// This looks up the parent message's channel via the fabric service to enforce the restriction.
func (os *ObserverServer) wrapReplyRestriction(originalHandler ToolHandler) ToolHandler {
	return func(ctx context.Context, args json.RawMessage) (*ToolCallResult, error) {
		// Parse args to get message_id
		var replyArgs replyArgs
		if err := json.Unmarshal(args, &replyArgs); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}

		if replyArgs.MessageID == "" {
			return nil, fmt.Errorf("message_id is required")
		}

		// Look up the parent message to get its channel
		parentChannel, err := os.getMessageChannel(replyArgs.MessageID)
		if err != nil {
			return nil, fmt.Errorf("failed to look up parent message: %w", err)
		}

		// Only allow replies to messages in #observer channel
		if parentChannel != domain.SlugObserver {
			return nil, fmt.Errorf("observer can only reply to messages in #observer channel, not #%s", parentChannel)
		}

		// Channel is allowed, proceed with original handler
		return originalHandler(ctx, args)
	}
}

// getMessageChannel looks up the channel slug for a given message ID.
// It traverses the thread hierarchy to find the channel the message belongs to.
func (os *ObserverServer) getMessageChannel(messageID string) (string, error) {
	if os.fabricService == nil {
		return "", fmt.Errorf("fabric service not configured")
	}

	// Get the message
	msg, err := os.fabricService.GetThread(messageID)
	if err != nil {
		return "", fmt.Errorf("message not found: %w", err)
	}

	// If the message is a channel, return its slug
	if msg.Type == domain.ThreadChannel {
		return msg.Slug, nil
	}

	// For messages, we need to find which channel they belong to
	// Try each known channel to see if this message is in it
	knownSlugs := []string{
		domain.SlugSystem,
		domain.SlugTasks,
		domain.SlugPlanning,
		domain.SlugGeneral,
		domain.SlugObserver,
	}

	for _, slug := range knownSlugs {
		// List messages in this channel and check if our message is there
		// This is a bit inefficient but correct
		messages, err := os.fabricService.ListMessages(slug, 0) // 0 = no limit
		if err != nil {
			continue
		}

		for _, m := range messages {
			// Check if message is a direct match or if our message is a reply to this one
			if m.ID == messageID {
				return slug, nil
			}
			// Check replies
			replies, _ := os.fabricService.GetReplies(m.ID)
			for _, r := range replies {
				if r.ID == messageID {
					return slug, nil
				}
			}
		}
	}

	return "", fmt.Errorf("could not determine channel for message %s", messageID)
}
