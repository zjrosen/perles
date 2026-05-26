package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/orchestration/fabric"
	"github.com/zjrosen/perles/internal/orchestration/fabric/domain"
	fabricpersist "github.com/zjrosen/perles/internal/orchestration/fabric/persistence"
	"github.com/zjrosen/perles/internal/orchestration/fabric/repository"
)

func newTestHandlers(t *testing.T) (*Handlers, *fabric.Service) {
	t.Helper()

	threadRepo := repository.NewMemoryThreadRepository()
	depRepo := repository.NewMemoryDependencyRepository()
	subRepo := repository.NewMemorySubscriptionRepository()
	ackRepo := repository.NewMemoryAckRepository(depRepo, threadRepo, subRepo)
	participantRepo := repository.NewMemoryParticipantRepository()

	svc := fabric.NewService(threadRepo, depRepo, subRepo, ackRepo, participantRepo)
	err := svc.InitSession("system")
	require.NoError(t, err)

	handlers := NewHandlers(svc, "COORDINATOR")
	return handlers, svc
}

func newReplaySeedService(t *testing.T, sessionDir string) (*fabric.Service, *fabricpersist.EventLogger) {
	t.Helper()

	logger, err := fabricpersist.NewEventLogger(sessionDir)
	require.NoError(t, err)

	threadRepo := repository.NewMemoryThreadRepository()
	depRepo := repository.NewMemoryDependencyRepository()
	subRepo := repository.NewMemorySubscriptionRepository()
	ackRepo := repository.NewMemoryAckRepository(depRepo, threadRepo, subRepo)
	participantRepo := repository.NewMemoryParticipantRepository()

	svc := fabric.NewService(threadRepo, depRepo, subRepo, ackRepo, participantRepo)
	svc.SetEventHandler(logger.HandleEvent)
	require.NoError(t, svc.InitSession("COORDINATOR"))

	return svc, logger
}

func newRestoredHandlersFromSession(t *testing.T, sessionDir string) (*Handlers, *fabric.Service) {
	t.Helper()

	threadRepo := repository.NewMemoryThreadRepository()
	depRepo := repository.NewMemoryDependencyRepository()
	subRepo := repository.NewMemorySubscriptionRepository()
	ackRepo := repository.NewMemoryAckRepository(depRepo, threadRepo, subRepo)
	participantRepo := repository.NewMemoryParticipantRepository()
	reactionRepo := repository.NewInMemoryReactionRepository()

	_, err := fabricpersist.RestoreFabricService(sessionDir, threadRepo, depRepo, subRepo, ackRepo, participantRepo, reactionRepo)
	require.NoError(t, err)

	svc := fabric.NewService(threadRepo, depRepo, subRepo, ackRepo, participantRepo)
	require.NoError(t, svc.RestoreChannelIDs())

	return NewHandlers(svc, "COORDINATOR"), svc
}

func decodeStructuredContent[T any](t *testing.T, result *ToolCallResult) T {
	t.Helper()

	var response T
	responseBytes, err := json.Marshal(result.StructuredContent)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(responseBytes, &response))

	return response
}

func TestHandlers_Inbox(t *testing.T) {
	h, svc := newTestHandlers(t)

	// Subscribe COORDINATOR to both channels so they can see messages
	_, err := svc.Subscribe(domain.SlugTasks, "COORDINATOR", domain.ModeAll)
	require.NoError(t, err)
	_, err = svc.Subscribe(domain.SlugGeneral, "COORDINATOR", domain.ModeAll)
	require.NoError(t, err)

	// Send some messages
	_, err = svc.SendMessage(fabric.SendMessageInput{
		ChannelSlug: domain.SlugTasks,
		Content:     "Task 1 @COORDINATOR",
		CreatedBy:   "WORKER.1",
	})
	require.NoError(t, err)

	_, err = svc.SendMessage(fabric.SendMessageInput{
		ChannelSlug: domain.SlugGeneral,
		Content:     "Hello team",
		CreatedBy:   "WORKER.2",
	})
	require.NoError(t, err)

	// Check inbox
	result, err := h.HandleInbox(context.Background(), nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)

	var response InboxResponse
	responseBytes, _ := json.Marshal(result.StructuredContent)
	err = json.Unmarshal(responseBytes, &response)
	require.NoError(t, err)

	require.Equal(t, 2, response.TotalUnacked)
	require.Len(t, response.Channels, 2)
}

func TestHandlers_Inbox_IncludesObserverChannel(t *testing.T) {
	// Create observer-specific handlers
	threadRepo := repository.NewMemoryThreadRepository()
	depRepo := repository.NewMemoryDependencyRepository()
	subRepo := repository.NewMemorySubscriptionRepository()
	ackRepo := repository.NewMemoryAckRepository(depRepo, threadRepo, subRepo)
	participantRepo := repository.NewMemoryParticipantRepository()

	svc := fabric.NewService(threadRepo, depRepo, subRepo, ackRepo, participantRepo)
	err := svc.InitSession("system")
	require.NoError(t, err)

	// Create handlers for OBSERVER agent
	h := NewHandlers(svc, "OBSERVER")

	// Subscribe OBSERVER to observer channel (as it would during boot)
	_, err = svc.Subscribe(domain.SlugObserver, "OBSERVER", domain.ModeAll)
	require.NoError(t, err)

	// Send a message to observer channel from user/system
	_, err = svc.SendMessage(fabric.SendMessageInput{
		ChannelSlug: domain.SlugObserver,
		Content:     "Hello observer!",
		CreatedBy:   "USER",
	})
	require.NoError(t, err)

	// Check inbox - should include observer channel
	result, err := h.HandleInbox(context.Background(), nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)

	var response InboxResponse
	responseBytes, _ := json.Marshal(result.StructuredContent)
	err = json.Unmarshal(responseBytes, &response)
	require.NoError(t, err)

	require.Equal(t, 1, response.TotalUnacked, "Should have 1 unread message")
	require.Len(t, response.Channels, 1, "Should have 1 channel with messages")
	require.Equal(t, domain.SlugObserver, response.Channels[0].ChannelSlug, "Channel should be observer")
	require.Equal(t, "Hello observer!", response.Channels[0].Messages[0].Content)
}

func TestHandlers_Send(t *testing.T) {
	h, _ := newTestHandlers(t)

	args := sendArgs{
		Channel: domain.SlugTasks,
		Content: "New task @worker-1",
		Kind:    "request",
	}
	argsJSON, _ := json.Marshal(args)

	result, err := h.HandleSend(context.Background(), argsJSON)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)

	var response SendResponse
	responseBytes, _ := json.Marshal(result.StructuredContent)
	err = json.Unmarshal(responseBytes, &response)
	require.NoError(t, err)

	require.NotEmpty(t, response.ID)
	require.NotEmpty(t, response.ChannelID)
	require.Contains(t, response.Mentions, "worker-1")
}

func TestHandlers_Send_ValidationErrors(t *testing.T) {
	h, _ := newTestHandlers(t)

	tests := []struct {
		name    string
		args    sendArgs
		wantErr string
	}{
		{
			name:    "missing channel",
			args:    sendArgs{Content: "hello"},
			wantErr: "channel is required",
		},
		{
			name:    "missing content",
			args:    sendArgs{Channel: domain.SlugTasks},
			wantErr: "content is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			argsJSON, _ := json.Marshal(tt.args)
			_, err := h.HandleSend(context.Background(), argsJSON)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestHandlers_Reply(t *testing.T) {
	h, svc := newTestHandlers(t)

	// Create a message to reply to
	msg, err := svc.SendMessage(fabric.SendMessageInput{
		ChannelSlug: domain.SlugTasks,
		Content:     "Task: Implement login",
		CreatedBy:   "COORDINATOR",
	})
	require.NoError(t, err)

	// Reply
	args := replyArgs{
		MessageID: msg.ID,
		Content:   "Starting work @coordinator",
	}
	argsJSON, _ := json.Marshal(args)

	result, err := h.HandleReply(context.Background(), argsJSON)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)

	var response ReplyResponse
	responseBytes, _ := json.Marshal(result.StructuredContent)
	err = json.Unmarshal(responseBytes, &response)
	require.NoError(t, err)

	require.NotEmpty(t, response.ID)
	require.Equal(t, msg.ID, response.ParentID)
	require.Equal(t, 1, response.ThreadPosition)
	require.Contains(t, response.Mentions, "coordinator")
}

func TestHandlers_Ack(t *testing.T) {
	h, svc := newTestHandlers(t)

	// Create messages
	msg1, _ := svc.SendMessage(fabric.SendMessageInput{
		ChannelSlug: domain.SlugTasks,
		Content:     "Message 1",
		CreatedBy:   "WORKER.1",
	})
	msg2, _ := svc.SendMessage(fabric.SendMessageInput{
		ChannelSlug: domain.SlugTasks,
		Content:     "Message 2",
		CreatedBy:   "WORKER.2",
	})

	// Ack both
	args := ackArgs{
		MessageIDs: []string{msg1.ID, msg2.ID},
	}
	argsJSON, _ := json.Marshal(args)

	result, err := h.HandleAck(context.Background(), argsJSON)
	require.NoError(t, err)
	require.NotNil(t, result)

	var response AckResponse
	responseBytes, _ := json.Marshal(result.StructuredContent)
	err = json.Unmarshal(responseBytes, &response)
	require.NoError(t, err)

	require.Equal(t, 2, response.AckedCount)

	// Verify inbox is empty for these messages
	inboxResult, _ := h.HandleInbox(context.Background(), nil)
	var inboxResp InboxResponse
	inboxBytes, _ := json.Marshal(inboxResult.StructuredContent)
	_ = json.Unmarshal(inboxBytes, &inboxResp)
	require.Equal(t, 0, inboxResp.TotalUnacked)
}

func TestHandlers_Ack_RejectsNonexistentMessageIDs(t *testing.T) {
	h, svc := newTestHandlers(t)

	// Create one real message
	msg, err := svc.SendMessage(fabric.SendMessageInput{
		ChannelSlug: domain.SlugTasks,
		Content:     "Real message",
		CreatedBy:   "WORKER.1",
	})
	require.NoError(t, err)

	t.Run("all IDs nonexistent", func(t *testing.T) {
		args := ackArgs{
			MessageIDs: []string{"fake-id-1", "fake-id-2"},
		}
		argsJSON, _ := json.Marshal(args)

		_, err := h.HandleAck(context.Background(), argsJSON)
		require.Error(t, err, "Should reject nonexistent message IDs")
		require.Contains(t, err.Error(), "fake-id-1")
		require.Contains(t, err.Error(), "fake-id-2")
		require.Contains(t, err.Error(), "unknown message_ids")
	})

	t.Run("mix of valid and nonexistent", func(t *testing.T) {
		args := ackArgs{
			MessageIDs: []string{msg.ID, "does-not-exist"},
		}
		argsJSON, _ := json.Marshal(args)

		_, err := h.HandleAck(context.Background(), argsJSON)
		require.Error(t, err, "Should reject when any ID is nonexistent")
		require.Contains(t, err.Error(), "does-not-exist")
		require.NotContains(t, err.Error(), msg.ID, "Valid ID should not appear in error")
	})

	t.Run("valid IDs still succeed", func(t *testing.T) {
		args := ackArgs{
			MessageIDs: []string{msg.ID},
		}
		argsJSON, _ := json.Marshal(args)

		result, err := h.HandleAck(context.Background(), argsJSON)
		require.NoError(t, err)
		require.NotNil(t, result)

		var response AckResponse
		responseBytes, _ := json.Marshal(result.StructuredContent)
		require.NoError(t, json.Unmarshal(responseBytes, &response))
		require.Equal(t, 1, response.AckedCount)
	})
}

func TestHandlers_History_IncludeAckedFalseFiltersAckedMessages(t *testing.T) {
	h, svc := newTestHandlers(t)

	// Subscribe so we can track ack state
	_, err := svc.Subscribe(domain.SlugTasks, "COORDINATOR", domain.ModeAll)
	require.NoError(t, err)

	// Create 3 messages
	msg1, err := svc.SendMessage(fabric.SendMessageInput{
		ChannelSlug: domain.SlugTasks,
		Content:     "Message 1",
		CreatedBy:   "WORKER.1",
	})
	require.NoError(t, err)

	_, err = svc.SendMessage(fabric.SendMessageInput{
		ChannelSlug: domain.SlugTasks,
		Content:     "Message 2",
		CreatedBy:   "WORKER.2",
	})
	require.NoError(t, err)

	msg3, err := svc.SendMessage(fabric.SendMessageInput{
		ChannelSlug: domain.SlugTasks,
		Content:     "Message 3",
		CreatedBy:   "WORKER.3",
	})
	require.NoError(t, err)

	// Ack messages 1 and 3, leave 2 unacked
	err = svc.Ack("COORDINATOR", msg1.ID, msg3.ID)
	require.NoError(t, err)

	t.Run("include_acked=false excludes acked messages", func(t *testing.T) {
		includeAcked := false
		args := historyArgs{
			Channel:      domain.SlugTasks,
			Limit:        10,
			IncludeAcked: &includeAcked,
		}
		argsJSON, _ := json.Marshal(args)

		result, err := h.HandleHistory(context.Background(), argsJSON)
		require.NoError(t, err)

		response := decodeStructuredContent[HistoryResponse](t, result)
		require.Len(t, response.Messages, 1, "Only unacked message should be returned")
		require.Equal(t, "Message 2", response.Messages[0].Content)
		require.False(t, response.Messages[0].IsAcked)
		require.Equal(t, 1, response.TotalCount, "TotalCount should match filtered count")
	})

	t.Run("include_acked=true returns all messages", func(t *testing.T) {
		includeAcked := true
		args := historyArgs{
			Channel:      domain.SlugTasks,
			Limit:        10,
			IncludeAcked: &includeAcked,
		}
		argsJSON, _ := json.Marshal(args)

		result, err := h.HandleHistory(context.Background(), argsJSON)
		require.NoError(t, err)

		response := decodeStructuredContent[HistoryResponse](t, result)
		require.Len(t, response.Messages, 3, "All messages should be returned")
		require.Equal(t, 3, response.TotalCount)
	})

	t.Run("include_acked omitted returns all messages (default)", func(t *testing.T) {
		args := historyArgs{
			Channel: domain.SlugTasks,
			Limit:   10,
		}
		argsJSON, _ := json.Marshal(args)

		result, err := h.HandleHistory(context.Background(), argsJSON)
		require.NoError(t, err)

		response := decodeStructuredContent[HistoryResponse](t, result)
		require.Len(t, response.Messages, 3, "All messages should be returned when flag is omitted")
	})
}

func TestHandlers_Subscribe(t *testing.T) {
	h, _ := newTestHandlers(t)

	args := subscribeArgs{
		Channel: domain.SlugTasks,
		Mode:    "mentions",
	}
	argsJSON, _ := json.Marshal(args)

	result, err := h.HandleSubscribe(context.Background(), argsJSON)
	require.NoError(t, err)
	require.NotNil(t, result)

	var response SubscribeResponse
	responseBytes, _ := json.Marshal(result.StructuredContent)
	err = json.Unmarshal(responseBytes, &response)
	require.NoError(t, err)

	require.NotEmpty(t, response.ChannelID)
	require.Equal(t, "mentions", response.Mode)
}

func TestHandlers_Unsubscribe(t *testing.T) {
	h, svc := newTestHandlers(t)

	// First subscribe
	_, err := svc.Subscribe(domain.SlugTasks, "COORDINATOR", domain.ModeAll)
	require.NoError(t, err)

	// Then unsubscribe
	args := unsubscribeArgs{
		Channel: domain.SlugTasks,
	}
	argsJSON, _ := json.Marshal(args)

	result, err := h.HandleUnsubscribe(context.Background(), argsJSON)
	require.NoError(t, err)
	require.NotNil(t, result)

	var response UnsubscribeResponse
	responseBytes, _ := json.Marshal(result.StructuredContent)
	err = json.Unmarshal(responseBytes, &response)
	require.NoError(t, err)

	require.True(t, response.Success)
}

func TestHandlers_Attach(t *testing.T) {
	h, svc := newTestHandlers(t)

	// Create a message to attach to
	msg, err := svc.SendMessage(fabric.SendMessageInput{
		ChannelSlug: domain.SlugTasks,
		Content:     "Here's my implementation",
		CreatedBy:   "WORKER.1",
	})
	require.NoError(t, err)

	// Create a temp file for the artifact
	tmpFile, err := os.CreateTemp("", "test-attach-*.diff")
	require.NoError(t, err)
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	_, err = tmpFile.WriteString("--- a/file.go\n+++ b/file.go\n@@ -1 +1 @@\n-old\n+new")
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())

	args := attachArgs{
		TargetID: msg.ID,
		Path:     tmpFile.Name(),
		Name:     "changes.diff",
	}
	argsJSON, _ := json.Marshal(args)

	result, err := h.HandleAttach(context.Background(), argsJSON)
	require.NoError(t, err)
	require.NotNil(t, result)

	var response AttachResponse
	responseBytes, _ := json.Marshal(result.StructuredContent)
	err = json.Unmarshal(responseBytes, &response)
	require.NoError(t, err)

	require.NotEmpty(t, response.ID)
	require.Equal(t, "changes.diff", response.Name)
	require.Greater(t, response.SizeBytes, int64(0))
}

func TestHandlers_History(t *testing.T) {
	h, svc := newTestHandlers(t)

	// Create some messages
	for i := 0; i < 5; i++ {
		_, err := svc.SendMessage(fabric.SendMessageInput{
			ChannelSlug: domain.SlugTasks,
			Content:     "Message",
			CreatedBy:   "COORDINATOR",
		})
		require.NoError(t, err)
	}

	args := historyArgs{
		Channel: domain.SlugTasks,
		Limit:   3,
	}
	argsJSON, _ := json.Marshal(args)

	result, err := h.HandleHistory(context.Background(), argsJSON)
	require.NoError(t, err)
	require.NotNil(t, result)

	var response HistoryResponse
	responseBytes, _ := json.Marshal(result.StructuredContent)
	err = json.Unmarshal(responseBytes, &response)
	require.NoError(t, err)

	require.Equal(t, domain.SlugTasks, response.ChannelSlug)
	require.Len(t, response.Messages, 3)
}

func TestHandlers_ReadThread(t *testing.T) {
	h, svc := newTestHandlers(t)

	// Create a message with replies
	msg, err := svc.SendMessage(fabric.SendMessageInput{
		ChannelSlug: domain.SlugTasks,
		Content:     "Task: Implement feature",
		CreatedBy:   "COORDINATOR",
	})
	require.NoError(t, err)

	// Add replies
	_, err = svc.Reply(fabric.ReplyInput{
		MessageID: msg.ID,
		Content:   "Starting...",
		CreatedBy: "WORKER.1",
	})
	require.NoError(t, err)

	_, err = svc.Reply(fabric.ReplyInput{
		MessageID: msg.ID,
		Content:   "Done!",
		CreatedBy: "WORKER.1",
	})
	require.NoError(t, err)

	// Read thread
	args := readThreadArgs{
		MessageID: msg.ID,
	}
	argsJSON, _ := json.Marshal(args)

	result, err := h.HandleReadThread(context.Background(), argsJSON)
	require.NoError(t, err)
	require.NotNil(t, result)

	var response ReadThreadResponse
	responseBytes, _ := json.Marshal(result.StructuredContent)
	err = json.Unmarshal(responseBytes, &response)
	require.NoError(t, err)

	require.Equal(t, msg.ID, response.Message.ID)
	require.Len(t, response.Replies, 2)
	require.Len(t, response.Participants, 2)
	require.Contains(t, response.Participants, "COORDINATOR")
	require.Contains(t, response.Participants, "WORKER.1")
}

func TestHandlers_ReadThread_RestoredRepliesAfterReplay(t *testing.T) {
	sessionDir := t.TempDir()
	seedSvc, logger := newReplaySeedService(t, sessionDir)

	root, err := seedSvc.SendMessage(fabric.SendMessageInput{
		ChannelSlug: domain.SlugGeneral,
		Content:     "Root message restored from replay",
		CreatedBy:   "COORDINATOR",
	})
	require.NoError(t, err)

	replyOne, err := seedSvc.Reply(fabric.ReplyInput{
		MessageID: root.ID,
		Content:   "First restored reply",
		CreatedBy: "WORKER.1",
	})
	require.NoError(t, err)

	replyTwo, err := seedSvc.Reply(fabric.ReplyInput{
		MessageID: root.ID,
		Content:   "Second restored reply",
		CreatedBy: "WORKER.2",
	})
	require.NoError(t, err)
	require.NoError(t, logger.Close())

	h, _ := newRestoredHandlersFromSession(t, sessionDir)

	argsJSON, err := json.Marshal(readThreadArgs{MessageID: root.ID})
	require.NoError(t, err)

	result, err := h.HandleReadThread(context.Background(), argsJSON)
	require.NoError(t, err)
	require.NotNil(t, result)

	response := decodeStructuredContent[ReadThreadResponse](t, result)
	require.Equal(t, root.ID, response.Message.ID)
	require.Len(t, response.Replies, 2)
	require.Equal(t, []string{replyOne.ID, replyTwo.ID}, []string{response.Replies[0].ID, response.Replies[1].ID})
	require.Equal(t, []string{"COORDINATOR", "WORKER.1", "WORKER.2"}, response.Participants)
}

func TestHandlers_History_RestoredRepliesVisibleAndOrderedAfterReplay(t *testing.T) {
	sessionDir := t.TempDir()
	seedSvc, logger := newReplaySeedService(t, sessionDir)

	firstRoot, err := seedSvc.SendMessage(fabric.SendMessageInput{
		ChannelSlug: domain.SlugGeneral,
		Content:     "First root message",
		CreatedBy:   "COORDINATOR",
	})
	require.NoError(t, err)

	secondRoot, err := seedSvc.SendMessage(fabric.SendMessageInput{
		ChannelSlug: domain.SlugGeneral,
		Content:     "Second root message",
		CreatedBy:   "COORDINATOR",
	})
	require.NoError(t, err)

	_, err = seedSvc.Reply(fabric.ReplyInput{
		MessageID: firstRoot.ID,
		Content:   "Reply attached to first root",
		CreatedBy: "WORKER.1",
	})
	require.NoError(t, err)

	_, err = seedSvc.Reply(fabric.ReplyInput{
		MessageID: secondRoot.ID,
		Content:   "Reply one on second root",
		CreatedBy: "WORKER.2",
	})
	require.NoError(t, err)

	_, err = seedSvc.Reply(fabric.ReplyInput{
		MessageID: secondRoot.ID,
		Content:   "Reply two on second root",
		CreatedBy: "WORKER.3",
	})
	require.NoError(t, err)
	require.NoError(t, logger.Close())

	h, _ := newRestoredHandlersFromSession(t, sessionDir)

	argsJSON, err := json.Marshal(historyArgs{
		Channel: domain.SlugGeneral,
		Limit:   10,
	})
	require.NoError(t, err)

	result, err := h.HandleHistory(context.Background(), argsJSON)
	require.NoError(t, err)
	require.NotNil(t, result)

	response := decodeStructuredContent[HistoryResponse](t, result)
	require.Equal(t, domain.SlugGeneral, response.ChannelSlug)
	require.Equal(t, 2, response.TotalCount)
	require.Len(t, response.Messages, 2, "replies should remain thread-only and not appear as channel history rows")
	require.Equal(t, firstRoot.ID, response.Messages[0].ID)
	require.Equal(t, secondRoot.ID, response.Messages[1].ID)
	require.Equal(t, 1, response.Messages[0].ReplyCount)
	require.Equal(t, 2, response.Messages[1].ReplyCount)
}

func TestHandlers_RestoredMalformedOrSkippedRepliesDoNotCrash(t *testing.T) {
	sessionDir := t.TempDir()
	seedSvc, logger := newReplaySeedService(t, sessionDir)

	root, err := seedSvc.SendMessage(fabric.SendMessageInput{
		ChannelSlug: domain.SlugGeneral,
		Content:     "Root for malformed replay test",
		CreatedBy:   "COORDINATOR",
	})
	require.NoError(t, err)

	validReply, err := seedSvc.Reply(fabric.ReplyInput{
		MessageID: root.ID,
		Content:   "Valid restored reply",
		CreatedBy: "WORKER.1",
	})
	require.NoError(t, err)

	generalChannelID := seedSvc.GetChannelID(domain.SlugGeneral)
	now := time.Now()

	logger.HandleEvent(fabric.Event{
		Type:      fabric.EventReplyPosted,
		Timestamp: now,
		ChannelID: generalChannelID,
		Thread: &domain.Thread{
			ID:        "reply-missing-parent-id",
			Type:      domain.ThreadMessage,
			Content:   "Reply missing parent id",
			Kind:      string(domain.KindResponse),
			CreatedAt: now,
			CreatedBy: "WORKER.2",
		},
	})

	logger.HandleEvent(fabric.Event{
		Type:      fabric.EventReplyPosted,
		Timestamp: now.Add(time.Second),
		ChannelID: generalChannelID,
		ParentID:  "msg-missing-parent",
		Thread: &domain.Thread{
			ID:        "reply-unresolved-parent",
			Type:      domain.ThreadMessage,
			Content:   "Reply with unresolved parent",
			Kind:      string(domain.KindResponse),
			CreatedAt: now.Add(time.Second),
			CreatedBy: "WORKER.3",
		},
	})

	require.NoError(t, logger.Close())

	eventsFile, err := os.OpenFile(filepath.Join(sessionDir, fabricpersist.FabricEventsFile), os.O_APPEND|os.O_WRONLY, 0644)
	require.NoError(t, err)
	_, err = eventsFile.WriteString("{this is malformed json\n")
	require.NoError(t, err)
	require.NoError(t, eventsFile.Close())

	h, _ := newRestoredHandlersFromSession(t, sessionDir)

	readArgsJSON, err := json.Marshal(readThreadArgs{MessageID: root.ID})
	require.NoError(t, err)
	readResult, err := h.HandleReadThread(context.Background(), readArgsJSON)
	require.NoError(t, err)
	readResponse := decodeStructuredContent[ReadThreadResponse](t, readResult)
	require.Len(t, readResponse.Replies, 1)
	require.Equal(t, validReply.ID, readResponse.Replies[0].ID)

	historyArgsJSON, err := json.Marshal(historyArgs{
		Channel: domain.SlugGeneral,
		Limit:   10,
	})
	require.NoError(t, err)
	historyResult, err := h.HandleHistory(context.Background(), historyArgsJSON)
	require.NoError(t, err)
	historyResponse := decodeStructuredContent[HistoryResponse](t, historyResult)
	require.Len(t, historyResponse.Messages, 1)
	require.Equal(t, root.ID, historyResponse.Messages[0].ID)
	require.Equal(t, 1, historyResponse.Messages[0].ReplyCount)
}
