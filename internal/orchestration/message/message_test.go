package message

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWorkerID(t *testing.T) {
	tests := []struct {
		index    int
		expected string
	}{
		{1, "WORKER.1"},
		{2, "WORKER.2"},
		{10, "WORKER.10"},
	}

	for _, tt := range tests {
		require.Equal(t, tt.expected, WorkerID(tt.index))
	}
}

func TestActorConstants(t *testing.T) {
	// Verify constants are what we expect
	require.Equal(t, "COORDINATOR", ActorCoordinator)
	require.Equal(t, "USER", ActorUser)
	require.Equal(t, "ALL", ActorAll)
}

func TestMessageTypeConstants(t *testing.T) {
	// Verify message types are string-able for JSON serialization
	require.Equal(t, "info", string(MessageInfo))
	require.Equal(t, "request", string(MessageRequest))
	require.Equal(t, "response", string(MessageResponse))
	require.Equal(t, "completion", string(MessageCompletion))
	require.Equal(t, "error", string(MessageError))
	require.Equal(t, "handoff", string(MessageHandoff))
	require.Equal(t, "worker-ready", string(MessageWorkerReady))
}

func TestMessageHandoffType(t *testing.T) {
	// Verify MessageHandoff constant exists and has value "handoff"
	require.Equal(t, MessageType("handoff"), MessageHandoff)
	require.Equal(t, "handoff", string(MessageHandoff))
}

func TestEntry_ReadByTracking(t *testing.T) {
	entry := Entry{
		ID:        "test-id",
		Timestamp: time.Now(),
		From:      ActorCoordinator,
		To:        ActorAll,
		Content:   "Test message",
		Type:      MessageInfo,
		ReadBy:    []string{ActorCoordinator},
	}

	// Initially only sender has read
	require.Len(t, entry.ReadBy, 1)
	require.Equal(t, ActorCoordinator, entry.ReadBy[0])

	// Add more readers
	entry.ReadBy = append(entry.ReadBy, WorkerID(1))
	entry.ReadBy = append(entry.ReadBy, WorkerID(2))

	require.Len(t, entry.ReadBy, 3)
	require.Contains(t, entry.ReadBy, ActorCoordinator)
	require.Contains(t, entry.ReadBy, "WORKER.1")
	require.Contains(t, entry.ReadBy, "WORKER.2")
}
