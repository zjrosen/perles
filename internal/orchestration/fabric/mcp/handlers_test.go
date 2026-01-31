package mcp

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/orchestration/fabric"
	"github.com/zjrosen/perles/internal/orchestration/fabric/domain"
	"github.com/zjrosen/perles/internal/orchestration/fabric/repository"
)

func newTestHandlers(t *testing.T) (*Handlers, *fabric.Service) {
	t.Helper()

	threadRepo := repository.NewMemoryThreadRepository()
	depRepo := repository.NewMemoryDependencyRepository()
	subRepo := repository.NewMemorySubscriptionRepository()
	ackRepo := repository.NewMemoryAckRepository(depRepo, threadRepo, subRepo)

	svc := fabric.NewService(threadRepo, depRepo, subRepo, ackRepo)
	err := svc.InitSession("system")
	require.NoError(t, err)

	handlers := NewHandlers(svc, "COORDINATOR")
	return handlers, svc
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

	svc := fabric.NewService(threadRepo, depRepo, subRepo, ackRepo)
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
