package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/zjrosen/perles/internal/orchestration/fabric"
	"github.com/zjrosen/perles/internal/orchestration/fabric/domain"
	"github.com/zjrosen/perles/internal/orchestration/mcp/types"
)

// ToolCallResult is an alias for the shared MCP types package.
type ToolCallResult = types.ToolCallResult

// ToolHandler is a function that handles an MCP tool call.
type ToolHandler func(ctx context.Context, args json.RawMessage) (*ToolCallResult, error)

// ToolRegistrar is the interface required for registering Fabric tools.
// This is satisfied by orchestration/mcp.Server.
type ToolRegistrar interface {
	RegisterTool(tool Tool, handler ToolHandler)
}

// Handlers provides MCP tool handlers for Fabric messaging.
type Handlers struct {
	service *fabric.Service
	agentID string // The agent ID for this handler instance
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(service *fabric.Service, agentID string) *Handlers {
	return &Handlers{
		service: service,
		agentID: agentID,
	}
}

// RegisterAll registers all Fabric tools with the MCP server.
func (h *Handlers) RegisterAll(server ToolRegistrar) {
	server.RegisterTool(ToolFabricInbox, h.HandleInbox)
	server.RegisterTool(ToolFabricSend, h.HandleSend)
	server.RegisterTool(ToolFabricReply, h.HandleReply)
	server.RegisterTool(ToolFabricAck, h.HandleAck)
	server.RegisterTool(ToolFabricSubscribe, h.HandleSubscribe)
	server.RegisterTool(ToolFabricUnsubscribe, h.HandleUnsubscribe)
	server.RegisterTool(ToolFabricAttach, h.HandleAttach)
	server.RegisterTool(ToolFabricHistory, h.HandleHistory)
	server.RegisterTool(ToolFabricReadThread, h.HandleReadThread)
}

// HandleInbox handles the fabric_inbox tool call.
func (h *Handlers) HandleInbox(_ context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	unacked, err := h.service.GetUnacked(h.agentID)
	if err != nil {
		return nil, fmt.Errorf("get unacked: %w", err)
	}

	response := InboxResponse{
		Channels:     make([]ChannelInbox, 0),
		TotalUnacked: 0,
	}

	// Convert unacked map to response format
	slugMap := map[string]string{
		h.service.GetChannelID(domain.SlugSystem):   domain.SlugSystem,
		h.service.GetChannelID(domain.SlugTasks):    domain.SlugTasks,
		h.service.GetChannelID(domain.SlugPlanning): domain.SlugPlanning,
		h.service.GetChannelID(domain.SlugGeneral):  domain.SlugGeneral,
		h.service.GetChannelID(domain.SlugObserver): domain.SlugObserver,
	}

	for channelID, summary := range unacked {
		slug := slugMap[channelID]
		if slug == "" {
			continue
		}

		inbox := ChannelInbox{
			ChannelID:   channelID,
			ChannelSlug: slug,
			Unacked:     summary.Count,
			Messages:    make([]InboxMessage, 0, len(summary.ThreadIDs)),
		}

		// Fetch message details
		for _, threadID := range summary.ThreadIDs {
			thread, err := h.service.GetThread(threadID)
			if err != nil {
				continue
			}

			inbox.Messages = append(inbox.Messages, InboxMessage{
				ID:        thread.ID,
				Content:   thread.Content,
				CreatedBy: thread.CreatedBy,
				CreatedAt: thread.CreatedAt,
				Mentions:  thread.Mentions,
			})
		}

		response.Channels = append(response.Channels, inbox)
		response.TotalUnacked += summary.Count
	}

	return types.StructuredResult(
		fmt.Sprintf("Found %d unread messages across %d channels", response.TotalUnacked, len(response.Channels)),
		response,
	), nil
}

// sendArgs are arguments for fabric_send.
type sendArgs struct {
	Channel string `json:"channel"`
	Content string `json:"content"`
	Kind    string `json:"kind,omitempty"`
}

// HandleSend handles the fabric_send tool call.
func (h *Handlers) HandleSend(_ context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	var args sendArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if args.Channel == "" {
		return nil, fmt.Errorf("channel is required")
	}
	if args.Content == "" {
		return nil, fmt.Errorf("content is required")
	}

	kind := domain.MessageKind(args.Kind)
	if kind == "" {
		kind = domain.KindInfo
	}

	msg, err := h.service.SendMessage(fabric.SendMessageInput{
		ChannelSlug: args.Channel,
		Content:     args.Content,
		Kind:        kind,
		CreatedBy:   h.agentID,
	})
	if err != nil {
		return nil, fmt.Errorf("send message: %w", err)
	}

	channelID := h.service.GetChannelID(args.Channel)

	response := SendResponse{
		ID:        msg.ID,
		Seq:       msg.Seq,
		ChannelID: channelID,
		Mentions:  msg.Mentions,
	}

	return types.StructuredResult(
		fmt.Sprintf("Message sent to #%s (id: %s)", args.Channel, msg.ID),
		response,
	), nil
}

// replyArgs are arguments for fabric_reply.
type replyArgs struct {
	MessageID string `json:"message_id"`
	Content   string `json:"content"`
	Kind      string `json:"kind,omitempty"`
}

// HandleReply handles the fabric_reply tool call.
func (h *Handlers) HandleReply(_ context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	var args replyArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if args.MessageID == "" {
		return nil, fmt.Errorf("message_id is required")
	}
	if args.Content == "" {
		return nil, fmt.Errorf("content is required")
	}

	kind := domain.MessageKind(args.Kind)
	if kind == "" {
		kind = domain.KindResponse
	}

	reply, err := h.service.Reply(fabric.ReplyInput{
		MessageID: args.MessageID,
		Content:   args.Content,
		Kind:      kind,
		CreatedBy: h.agentID,
	})
	if err != nil {
		return nil, fmt.Errorf("reply: %w", err)
	}

	// Get reply count for thread position
	replies, _ := h.service.GetReplies(args.MessageID)
	threadPosition := len(replies)

	response := ReplyResponse{
		ID:             reply.ID,
		Seq:            reply.Seq,
		ParentID:       args.MessageID,
		Mentions:       reply.Mentions,
		ThreadDepth:    1,
		ThreadPosition: threadPosition,
	}

	return types.StructuredResult(
		fmt.Sprintf("Reply posted (id: %s, position: %d)", reply.ID, threadPosition),
		response,
	), nil
}

// ackArgs are arguments for fabric_ack.
type ackArgs struct {
	MessageIDs []string `json:"message_ids"`
}

// HandleAck handles the fabric_ack tool call.
func (h *Handlers) HandleAck(_ context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	var args ackArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if len(args.MessageIDs) == 0 {
		return nil, fmt.Errorf("message_ids is required")
	}

	if err := h.service.Ack(h.agentID, args.MessageIDs...); err != nil {
		return nil, fmt.Errorf("ack: %w", err)
	}

	response := AckResponse{
		AckedCount: len(args.MessageIDs),
	}

	return types.StructuredResult(
		fmt.Sprintf("Acknowledged %d messages", len(args.MessageIDs)),
		response,
	), nil
}

// subscribeArgs are arguments for fabric_subscribe.
type subscribeArgs struct {
	Channel string `json:"channel"`
	Mode    string `json:"mode,omitempty"`
}

// HandleSubscribe handles the fabric_subscribe tool call.
func (h *Handlers) HandleSubscribe(_ context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	var args subscribeArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if args.Channel == "" {
		return nil, fmt.Errorf("channel is required")
	}

	mode := domain.SubscriptionMode(args.Mode)
	if mode == "" {
		mode = domain.ModeAll
	}

	sub, err := h.service.Subscribe(args.Channel, h.agentID, mode)
	if err != nil {
		return nil, fmt.Errorf("subscribe: %w", err)
	}

	response := SubscribeResponse{
		ChannelID: sub.ChannelID,
		Mode:      string(sub.Mode),
	}

	return types.StructuredResult(
		fmt.Sprintf("Subscribed to #%s with mode '%s'", args.Channel, mode),
		response,
	), nil
}

// unsubscribeArgs are arguments for fabric_unsubscribe.
type unsubscribeArgs struct {
	Channel string `json:"channel"`
}

// HandleUnsubscribe handles the fabric_unsubscribe tool call.
func (h *Handlers) HandleUnsubscribe(_ context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	var args unsubscribeArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if args.Channel == "" {
		return nil, fmt.Errorf("channel is required")
	}

	if err := h.service.Unsubscribe(args.Channel, h.agentID); err != nil {
		return nil, fmt.Errorf("unsubscribe: %w", err)
	}

	response := UnsubscribeResponse{
		Success: true,
	}

	return types.StructuredResult(
		fmt.Sprintf("Unsubscribed from #%s", args.Channel),
		response,
	), nil
}

// attachArgs are arguments for fabric_attach.
type attachArgs struct {
	TargetID string `json:"target_id"`
	Path     string `json:"path"`
	Name     string `json:"name,omitempty"`
}

// HandleAttach handles the fabric_attach tool call.
func (h *Handlers) HandleAttach(_ context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	var args attachArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if args.TargetID == "" {
		return nil, fmt.Errorf("target_id is required")
	}
	if args.Path == "" {
		return nil, fmt.Errorf("path is required")
	}

	artifact, err := h.service.AttachArtifact(fabric.AttachArtifactInput{
		TargetID:  args.TargetID,
		Path:      args.Path,
		Name:      args.Name,
		CreatedBy: h.agentID,
	})
	if err != nil {
		return nil, fmt.Errorf("attach artifact: %w", err)
	}

	response := AttachResponse{
		ID:        artifact.ID,
		Name:      artifact.Name,
		SizeBytes: artifact.SizeBytes,
	}

	return types.StructuredResult(
		fmt.Sprintf("Attached '%s' (%d bytes)", artifact.Name, artifact.SizeBytes),
		response,
	), nil
}

// historyArgs are arguments for fabric_history.
type historyArgs struct {
	Channel      string `json:"channel"`
	Limit        int    `json:"limit,omitempty"`
	IncludeAcked *bool  `json:"include_acked,omitempty"`
}

// HandleHistory handles the fabric_history tool call.
func (h *Handlers) HandleHistory(_ context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	var args historyArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if args.Channel == "" {
		return nil, fmt.Errorf("channel is required")
	}

	limit := args.Limit
	if limit <= 0 {
		limit = 50
	}

	messages, err := h.service.ListMessages(args.Channel, limit)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}

	channelID := h.service.GetChannelID(args.Channel)

	// Get acked thread IDs for this agent
	ackedIDs := make(map[string]bool)
	unacked, _ := h.service.GetUnacked(h.agentID)
	for _, summary := range unacked {
		for _, id := range summary.ThreadIDs {
			ackedIDs[id] = false // false = not acked
		}
	}

	response := HistoryResponse{
		ChannelID:   channelID,
		ChannelSlug: args.Channel,
		Messages:    make([]HistoryMessage, 0, len(messages)),
		TotalCount:  len(messages),
	}

	for _, msg := range messages {
		// Check for replies
		replies, _ := h.service.GetReplies(msg.ID)

		// Check for artifacts
		artifacts, _ := h.service.GetArtifacts(msg.ID)

		// Determine if acked
		_, unackedExists := ackedIDs[msg.ID]
		isAcked := !unackedExists

		response.Messages = append(response.Messages, HistoryMessage{
			ID:          msg.ID,
			Seq:         msg.Seq,
			Content:     msg.Content,
			Kind:        msg.Kind,
			CreatedBy:   msg.CreatedBy,
			CreatedAt:   msg.CreatedAt,
			ReplyCount:  len(replies),
			IsAcked:     isAcked,
			Mentions:    msg.Mentions,
			HasArtifact: len(artifacts) > 0,
		})
	}

	return types.StructuredResult(
		fmt.Sprintf("Retrieved %d messages from #%s", len(response.Messages), args.Channel),
		response,
	), nil
}

// readThreadArgs are arguments for fabric_read_thread.
type readThreadArgs struct {
	MessageID        string `json:"message_id"`
	IncludeArtifacts *bool  `json:"include_artifacts,omitempty"`
}

// HandleReadThread handles the fabric_read_thread tool call.
func (h *Handlers) HandleReadThread(_ context.Context, rawArgs json.RawMessage) (*ToolCallResult, error) {
	var args readThreadArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if args.MessageID == "" {
		return nil, fmt.Errorf("message_id is required")
	}

	// Get root message
	msg, err := h.service.GetThread(args.MessageID)
	if err != nil {
		return nil, fmt.Errorf("get thread: %w", err)
	}

	if msg.Type != domain.ThreadMessage {
		return nil, fmt.Errorf("thread %s is not a message", args.MessageID)
	}

	// Get replies
	replies, err := h.service.GetReplies(args.MessageID)
	if err != nil {
		return nil, fmt.Errorf("get replies: %w", err)
	}

	response := ReadThreadResponse{
		Message: ThreadMessage{
			ID:        msg.ID,
			Seq:       msg.Seq,
			Content:   msg.Content,
			Kind:      msg.Kind,
			CreatedBy: msg.CreatedBy,
			CreatedAt: msg.CreatedAt,
			Mentions:  msg.Mentions,
		},
		Replies:      make([]ThreadMessage, 0, len(replies)),
		Participants: []string{msg.CreatedBy},
	}

	participantSet := map[string]bool{msg.CreatedBy: true}

	for _, reply := range replies {
		response.Replies = append(response.Replies, ThreadMessage{
			ID:        reply.ID,
			Seq:       reply.Seq,
			Content:   reply.Content,
			Kind:      reply.Kind,
			CreatedBy: reply.CreatedBy,
			CreatedAt: reply.CreatedAt,
			Mentions:  reply.Mentions,
		})

		if !participantSet[reply.CreatedBy] {
			participantSet[reply.CreatedBy] = true
			response.Participants = append(response.Participants, reply.CreatedBy)
		}
	}

	// Get artifacts if requested (default: true)
	includeArtifacts := args.IncludeArtifacts == nil || *args.IncludeArtifacts
	if includeArtifacts {
		artifacts, _ := h.service.GetArtifacts(args.MessageID)
		response.Artifacts = make([]ThreadArtifact, 0, len(artifacts))

		for _, art := range artifacts {
			preview := ""
			if art.MediaType == "text/plain" || art.MediaType == "text/x-diff" {
				content, _ := h.service.GetArtifactContent(art.ID)
				if len(content) > 200 {
					preview = string(content[:200]) + "..."
				} else {
					preview = string(content)
				}
			}

			response.Artifacts = append(response.Artifacts, ThreadArtifact{
				ID:        art.ID,
				Name:      art.Name,
				MediaType: art.MediaType,
				SizeBytes: art.SizeBytes,
				CreatedBy: art.CreatedBy,
				CreatedAt: art.CreatedAt,
				Preview:   preview,
			})
		}
	}

	return types.StructuredResult(
		fmt.Sprintf("Thread with %d replies, %d participants", len(response.Replies), len(response.Participants)),
		response,
	), nil
}
