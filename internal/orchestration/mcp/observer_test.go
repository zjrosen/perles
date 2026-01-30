package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/orchestration/fabric"
	fabricrepo "github.com/zjrosen/perles/internal/orchestration/fabric/repository"
	"github.com/zjrosen/perles/internal/orchestration/v2/prompt"
)

// ============================================================================
// Tests for ObserverServer Registration
// ============================================================================

// TestObserverServer_ReadOnlyTools verifies that only read-only fabric tools plus
// restricted fabric_send and fabric_reply are registered (no coordinator tools).
func TestObserverServer_ReadOnlyTools(t *testing.T) {
	os := NewObserverServer("observer")
	svc := newTestFabricService()
	os.SetFabricService(svc)

	// Expected tools: read-only fabric tools + restricted fabric_send + restricted fabric_reply
	expectedTools := []string{
		"fabric_inbox",
		"fabric_history",
		"fabric_read_thread",
		"fabric_subscribe",
		"fabric_ack",
		"fabric_send",
		"fabric_reply",
	}

	// Verify expected tools are registered
	for _, toolName := range expectedTools {
		_, ok := os.tools[toolName]
		require.True(t, ok, "Tool %q should be registered", toolName)
		_, ok = os.handlers[toolName]
		require.True(t, ok, "Handler for %q should be registered", toolName)
	}

	// Verify coordinator/worker tools are NOT registered
	forbiddenTools := []string{
		"spawn_worker",
		"assign_task",
		"send_to_worker",
		"get_task_status",
		"mark_task_complete",
		"mark_task_failed",
		"signal_ready",
		"report_implementation_complete",
		"report_review_verdict",
		"fabric_unsubscribe",
		"fabric_attach",
	}

	for _, toolName := range forbiddenTools {
		_, ok := os.tools[toolName]
		require.False(t, ok, "Tool %q should NOT be registered for Observer", toolName)
	}

	// Verify exact tool count
	require.Equal(t, len(expectedTools), len(os.tools), "Tool count mismatch")
}

// ============================================================================
// Tests for fabric_send Channel Restriction
// ============================================================================

// TestObserverServer_ChannelRestriction_Send verifies that fabric_send to non-observer
// channels returns an error.
func TestObserverServer_ChannelRestriction_Send(t *testing.T) {
	os := NewObserverServer("observer")
	svc := newTestFabricService()
	os.SetFabricService(svc)

	handler := os.handlers["fabric_send"]
	require.NotNil(t, handler, "fabric_send handler should be registered")

	// Try to send to #tasks channel - should be blocked
	args := json.RawMessage(`{"channel": "tasks", "content": "This should fail"}`)
	_, err := handler(context.Background(), args)
	require.Error(t, err, "Sending to #tasks should be blocked")
	require.Contains(t, err.Error(), "observer can only send messages to #observer channel")
	require.Contains(t, err.Error(), "#tasks")
}

// TestObserverServer_ChannelRestriction_SendObserver verifies that fabric_send to
// #observer channel succeeds.
func TestObserverServer_ChannelRestriction_SendObserver(t *testing.T) {
	os := NewObserverServer("observer")
	svc := newTestFabricService()
	os.SetFabricService(svc)

	handler := os.handlers["fabric_send"]
	require.NotNil(t, handler, "fabric_send handler should be registered")

	// Send to #observer channel - should succeed
	args := json.RawMessage(`{"channel": "observer", "content": "Hello from observer"}`)
	result, err := handler(context.Background(), args)
	require.NoError(t, err, "Sending to #observer should succeed")
	require.NotNil(t, result, "Expected result")
	require.False(t, result.IsError, "Expected success result")
}

// TestObserverServer_ChannelRestriction_AllChannelsBlocked tests that all non-observer
// channels are blocked.
func TestObserverServer_ChannelRestriction_AllChannelsBlocked(t *testing.T) {
	os := NewObserverServer("observer")
	svc := newTestFabricService()
	os.SetFabricService(svc)

	handler := os.handlers["fabric_send"]
	require.NotNil(t, handler, "fabric_send handler should be registered")

	blockedChannels := []string{"tasks", "planning", "general", "system"}

	for _, channel := range blockedChannels {
		t.Run(channel, func(t *testing.T) {
			args := json.RawMessage(`{"channel": "` + channel + `", "content": "test"}`)
			_, err := handler(context.Background(), args)
			require.Error(t, err, "Sending to #%s should be blocked", channel)
			require.Contains(t, err.Error(), "observer can only send messages to #observer channel")
		})
	}
}

// TestObserverServer_ReplyRestriction_NonObserver verifies that fabric_reply to a message
// in #tasks channel returns an error.
func TestObserverServer_ReplyRestriction_NonObserver(t *testing.T) {
	os := NewObserverServer("observer")
	svc := newTestFabricService()
	os.SetFabricService(svc)

	// First send a message to #tasks as coordinator
	msg, err := svc.SendMessage(fabric.SendMessageInput{
		ChannelSlug: "tasks",
		Content:     "Task assignment for worker-1",
		CreatedBy:   "coordinator",
	})
	require.NoError(t, err, "Failed to create test message")

	handler := os.handlers["fabric_reply"]
	require.NotNil(t, handler, "fabric_reply handler should be registered")

	// Try to reply to the #tasks message - should be blocked
	args := json.RawMessage(`{"message_id": "` + msg.ID + `", "content": "This should fail"}`)
	_, err = handler(context.Background(), args)
	require.Error(t, err, "Replying to #tasks message should be blocked")
	require.Contains(t, err.Error(), "observer can only reply to messages in #observer channel")
	require.Contains(t, err.Error(), "#tasks")
}

// TestObserverServer_ReplyRestriction_Observer verifies that fabric_reply to a message
// in #observer channel succeeds.
func TestObserverServer_ReplyRestriction_Observer(t *testing.T) {
	os := NewObserverServer("observer")
	svc := newTestFabricService()
	os.SetFabricService(svc)

	// First send a message to #observer
	msg, err := svc.SendMessage(fabric.SendMessageInput{
		ChannelSlug: "observer",
		Content:     "User question to observer",
		CreatedBy:   "user",
	})
	require.NoError(t, err, "Failed to create test message")

	handler := os.handlers["fabric_reply"]
	require.NotNil(t, handler, "fabric_reply handler should be registered")

	// Reply to the #observer message - should succeed
	args := json.RawMessage(`{"message_id": "` + msg.ID + `", "content": "Here is my response"}`)
	result, err := handler(context.Background(), args)
	require.NoError(t, err, "Replying to #observer message should succeed")
	require.NotNil(t, result, "Expected result")
	require.False(t, result.IsError, "Expected success result")
}

// TestObserverServer_ReplyRestriction_AllChannelsBlocked tests that replies to all
// non-observer channels are blocked.
func TestObserverServer_ReplyRestriction_AllChannelsBlocked(t *testing.T) {
	os := NewObserverServer("observer")
	svc := newTestFabricService()
	os.SetFabricService(svc)

	handler := os.handlers["fabric_reply"]
	require.NotNil(t, handler, "fabric_reply handler should be registered")

	blockedChannels := []string{"tasks", "planning", "general", "system"}

	for _, channel := range blockedChannels {
		t.Run(channel, func(t *testing.T) {
			// Create a message in the channel
			msg, err := svc.SendMessage(fabric.SendMessageInput{
				ChannelSlug: channel,
				Content:     "Test message",
				CreatedBy:   "coordinator",
			})
			require.NoError(t, err, "Failed to create test message in #%s", channel)

			// Try to reply - should be blocked
			args := json.RawMessage(`{"message_id": "` + msg.ID + `", "content": "test reply"}`)
			_, err = handler(context.Background(), args)
			require.Error(t, err, "Replying to #%s message should be blocked", channel)
			require.Contains(t, err.Error(), "observer can only reply to messages in #observer channel")
		})
	}
}

// TestObserverServer_ReplyRestriction_ToReply tests that replying to a reply in a
// non-observer channel is also blocked.
func TestObserverServer_ReplyRestriction_ToReply(t *testing.T) {
	os := NewObserverServer("observer")
	svc := newTestFabricService()
	os.SetFabricService(svc)

	// Create a message in #tasks
	msg, err := svc.SendMessage(fabric.SendMessageInput{
		ChannelSlug: "tasks",
		Content:     "Task assignment",
		CreatedBy:   "coordinator",
	})
	require.NoError(t, err)

	// Create a reply to that message
	reply, err := svc.Reply(fabric.ReplyInput{
		MessageID: msg.ID,
		Content:   "Working on it",
		CreatedBy: "worker-1",
	})
	require.NoError(t, err)

	handler := os.handlers["fabric_reply"]
	require.NotNil(t, handler)

	// Try to reply to the reply - should be blocked because it's in #tasks
	args := json.RawMessage(`{"message_id": "` + reply.ID + `", "content": "This should fail"}`)
	_, err = handler(context.Background(), args)
	require.Error(t, err, "Replying to #tasks thread reply should be blocked")
	require.Contains(t, err.Error(), "observer can only reply to messages in #observer channel")
}

// ============================================================================
// Tests for ObserverMCPInstructions
// ============================================================================

// TestObserverMCPInstructions verifies that instructions are non-empty and mention
// the Observer role.
func TestObserverMCPInstructions(t *testing.T) {
	instructions := prompt.ObserverMCPInstructions()

	require.NotEmpty(t, instructions, "Instructions should not be empty")
	require.Contains(t, instructions, "Observer", "Instructions should mention 'Observer'")
	require.Contains(t, instructions, "passive", "Instructions should mention 'passive'")
	require.Contains(t, instructions, "#observer", "Instructions should mention '#observer' channel")
	require.Contains(t, instructions, "fabric_inbox", "Instructions should mention read-only tools")
	require.Contains(t, instructions, "fabric_send", "Instructions should mention fabric_send")
	require.Contains(t, instructions, "fabric_reply", "Instructions should mention fabric_reply")
}

// ============================================================================
// Tests for ObserverServer Initialization
// ============================================================================

// TestObserverServer_Instructions tests that instructions are set correctly.
func TestObserverServer_Instructions(t *testing.T) {
	os := NewObserverServer("observer")

	require.NotEmpty(t, os.instructions, "Instructions should be set")
	require.Equal(t, "perles-observer", os.info.Name, "Server name mismatch")
	require.Equal(t, "1.0.0", os.info.Version, "Server version mismatch")
	require.Contains(t, os.instructions, "Observer", "Instructions should mention Observer")
}

// TestObserverServer_CallerInfo tests that caller info is set correctly.
func TestObserverServer_CallerInfo(t *testing.T) {
	os := NewObserverServer("observer")

	require.Equal(t, "observer", os.callerRole, "Caller role should be 'observer'")
	require.Equal(t, "observer", os.callerID, "Caller ID should be 'observer'")
}

// TestObserverServer_ReadOnlyToolsWork verifies that read-only tools actually work.
func TestObserverServer_ReadOnlyToolsWork(t *testing.T) {
	os := NewObserverServer("observer")
	svc := newTestFabricService()
	os.SetFabricService(svc)

	// Create some test data
	_, err := svc.SendMessage(fabric.SendMessageInput{
		ChannelSlug: "general",
		Content:     "Test message",
		CreatedBy:   "coordinator",
	})
	require.NoError(t, err)

	// Test fabric_inbox
	t.Run("fabric_inbox", func(t *testing.T) {
		handler := os.handlers["fabric_inbox"]
		require.NotNil(t, handler)
		result, err := handler(context.Background(), json.RawMessage(`{}`))
		require.NoError(t, err, "fabric_inbox should succeed")
		require.NotNil(t, result)
	})

	// Test fabric_history
	t.Run("fabric_history", func(t *testing.T) {
		handler := os.handlers["fabric_history"]
		require.NotNil(t, handler)
		result, err := handler(context.Background(), json.RawMessage(`{"channel": "general"}`))
		require.NoError(t, err, "fabric_history should succeed")
		require.NotNil(t, result)
	})

	// Test fabric_subscribe
	t.Run("fabric_subscribe", func(t *testing.T) {
		handler := os.handlers["fabric_subscribe"]
		require.NotNil(t, handler)
		result, err := handler(context.Background(), json.RawMessage(`{"channel": "tasks", "mode": "all"}`))
		require.NoError(t, err, "fabric_subscribe should succeed")
		require.NotNil(t, result)
	})
}

// ============================================================================
// Helper functions for testing
// ============================================================================

// newTestFabricServiceForObserver creates a fabric service for observer testing.
// Reuses the existing helper from worker_test.go.
func newTestFabricServiceForObserver() *fabric.Service {
	threadRepo := fabricrepo.NewMemoryThreadRepository()
	depRepo := fabricrepo.NewMemoryDependencyRepository()
	subRepo := fabricrepo.NewMemorySubscriptionRepository()
	ackRepo := fabricrepo.NewMemoryAckRepository(depRepo, threadRepo, subRepo)
	svc := fabric.NewService(threadRepo, depRepo, subRepo, ackRepo)
	// Initialize session to create channels
	_ = svc.InitSession("coordinator")
	return svc
}
