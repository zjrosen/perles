package message

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestIssue_InMemoryOperations tests operations that don't require bd CLI.
func TestIssue_InMemoryOperations(t *testing.T) {
	mi := New()

	// Initially empty
	require.Equal(t, 0, mi.Count())
	require.Empty(t, mi.Entries())

	// Add some entries directly (bypassing Append for testing)
	mi.entries = []Entry{
		{
			ID:        uuid.New().String(),
			Timestamp: time.Now(),
			From:      ActorCoordinator,
			To:        ActorAll,
			Content:   "Message 1",
			Type:      MessageInfo,
		},
		{
			ID:        uuid.New().String(),
			Timestamp: time.Now(),
			From:      WorkerID(1),
			To:        ActorCoordinator,
			Content:   "Message 2",
			Type:      MessageResponse,
		},
	}

	// Count and entries
	require.Equal(t, 2, mi.Count())
	entries := mi.Entries()
	require.Len(t, entries, 2)
	require.Equal(t, "Message 1", entries[0].Content)
	require.Equal(t, "Message 2", entries[1].Content)
}

func TestIssue_UnreadFor(t *testing.T) {
	mi := New()

	// Add entries
	mi.entries = []Entry{
		{
			ID:        "1",
			Timestamp: time.Now(),
			From:      ActorCoordinator,
			To:        ActorAll,
			Content:   "Broadcast message",
			Type:      MessageInfo,
		},
		{
			ID:        "2",
			Timestamp: time.Now(),
			From:      ActorCoordinator,
			To:        WorkerID(1),
			Content:   "Direct to Worker 1",
			Type:      MessageRequest,
		},
		{
			ID:        "3",
			Timestamp: time.Now(),
			From:      ActorCoordinator,
			To:        WorkerID(2),
			Content:   "Direct to Worker 2",
			Type:      MessageRequest,
		},
	}

	// All agents see all messages (no filtering by recipient)
	unread := mi.UnreadFor(WorkerID(1))
	require.Len(t, unread, 3)
	require.Equal(t, "Broadcast message", unread[0].Content)
	require.Equal(t, "Direct to Worker 1", unread[1].Content)
	require.Equal(t, "Direct to Worker 2", unread[2].Content)

	// Worker 2 also sees all messages
	unread = mi.UnreadFor(WorkerID(2))
	require.Len(t, unread, 3)

	// Coordinator sees all messages
	unread = mi.UnreadFor(ActorCoordinator)
	require.Len(t, unread, 3)
}

func TestIssue_MarkRead(t *testing.T) {
	mi := New()

	// Add entries
	mi.entries = []Entry{
		{ID: "1", From: ActorCoordinator, To: ActorAll, Content: "Message 1"},
		{ID: "2", From: ActorCoordinator, To: ActorAll, Content: "Message 2"},
	}

	// Worker 1 has 2 unread messages
	unread := mi.UnreadFor(WorkerID(1))
	require.Len(t, unread, 2)

	// Mark as read
	mi.MarkRead(WorkerID(1))

	// Now Worker 1 has 0 unread
	unread = mi.UnreadFor(WorkerID(1))
	require.Empty(t, unread)

	// Add another message
	mi.entries = append(mi.entries, Entry{
		ID:      "3",
		From:    ActorCoordinator,
		To:      ActorAll,
		Content: "Message 3",
	})

	// Worker 1 should see only the new message
	unread = mi.UnreadFor(WorkerID(1))
	require.Len(t, unread, 1)
	require.Equal(t, "Message 3", unread[0].Content)
}

func TestIssue_ConcurrentAccess(t *testing.T) {
	mi := New()

	// Pre-populate entries
	for i := 0; i < 100; i++ {
		mi.entries = append(mi.entries, Entry{
			ID:      uuid.New().String(),
			From:    ActorCoordinator,
			To:      ActorAll,
			Content: "Test",
		})
	}

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Multiple readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = mi.Entries()
				_ = mi.UnreadFor(WorkerID(workerID))
				_ = mi.Count()
			}
		}(i)
	}

	// Multiple MarkRead operations
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				mi.MarkRead(WorkerID(workerID))
				time.Sleep(time.Microsecond)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Should have no race conditions
	for err := range errors {
		require.Fail(t, "Concurrent access error", "%v", err)
	}
}

func TestIssue_EntriesReturnsCopy(t *testing.T) {
	mi := New()

	mi.entries = []Entry{
		{ID: "1", Content: "Original"},
	}

	// Get entries
	entries := mi.Entries()

	// Modify returned slice
	entries[0].Content = "Modified"

	// Original should be unchanged
	require.Equal(t, "Original", mi.entries[0].Content)
}

func TestIssue_Append_PublishesEvent(t *testing.T) {
	mi := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := mi.Broker().Subscribe(ctx)

	// Append a message
	_, err := mi.Append(WorkerID(1), ActorCoordinator, "Hello", MessageInfo)
	require.NoError(t, err)

	// Should receive event
	select {
	case event := <-ch:
		require.Equal(t, EventPosted, event.Payload.Type)
		require.Equal(t, WorkerID(1), event.Payload.Entry.From)
		require.Equal(t, ActorCoordinator, event.Payload.Entry.To)
		require.Equal(t, "Hello", event.Payload.Entry.Content)
		require.Equal(t, MessageInfo, event.Payload.Entry.Type)
	case <-time.After(time.Second):
		require.FailNow(t, "timeout waiting for event")
	}
}

func TestIssue_MultipleSubscribers(t *testing.T) {
	mi := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch1 := mi.Broker().Subscribe(ctx)
	ch2 := mi.Broker().Subscribe(ctx)

	_, err := mi.Append(WorkerID(1), ActorAll, "Broadcast", MessageInfo)
	require.NoError(t, err)

	// Both subscribers receive the event
	select {
	case e1 := <-ch1:
		select {
		case e2 := <-ch2:
			require.Equal(t, e1.Payload.Entry.ID, e2.Payload.Entry.ID)
			require.Equal(t, "Broadcast", e1.Payload.Entry.Content)
		case <-time.After(time.Second):
			require.FailNow(t, "timeout waiting for event on ch2")
		}
	case <-time.After(time.Second):
		require.FailNow(t, "timeout waiting for event on ch1")
	}
}

func TestIssue_Broker_NotNil(t *testing.T) {
	mi := New()
	require.NotNil(t, mi.Broker())
}
