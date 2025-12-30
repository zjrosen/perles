package coordinator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/orchestration/pool"
	"github.com/zjrosen/perles/internal/orchestration/queue"
	"github.com/zjrosen/perles/internal/pubsub"
)

// testMessageIssue creates a mock message issue for testing.
func testMessageIssue() *message.Issue {
	return message.New()
}

// testClient returns a mock client for testing.
func testClient(t *testing.T) client.HeadlessClient {
	mockClient := mocks.NewMockHeadlessClient(t)
	// Allow Type() to be called any number of times
	mockClient.EXPECT().Type().Return(client.ClientMock).Maybe()
	return mockClient
}

func TestNew(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := testMessageIssue()

	cfg := Config{
		WorkDir:      "/tmp",
		Client:       testClient(t),
		Pool:         workerPool,
		MessageIssue: msgIssue,
	}

	coord, err := New(cfg)
	require.NoError(t, err)
	require.NotNil(t, coord)
	require.Equal(t, StatusPending, coord.Status())
}

func TestNew_RequiresWorkDir(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := testMessageIssue()

	cfg := Config{
		Pool:         workerPool,
		MessageIssue: msgIssue,
	}

	_, err := New(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "work directory")
}

func TestNew_RequiresClient(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := testMessageIssue()

	cfg := Config{
		WorkDir:      "/tmp",
		Pool:         workerPool,
		MessageIssue: msgIssue,
	}

	_, err := New(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "headless client")
}

func TestNew_RequiresPool(t *testing.T) {
	msgIssue := testMessageIssue()

	cfg := Config{
		WorkDir:      "/tmp",
		Client:       testClient(t),
		MessageIssue: msgIssue,
	}

	_, err := New(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "worker pool")
}

func TestNew_RequiresMessageIssue(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cfg := Config{
		WorkDir: "/tmp",
		Client:  testClient(t),
		Pool:    workerPool,
	}

	_, err := New(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "message issue")
}

func TestNew_DefaultModel(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := testMessageIssue()

	cfg := Config{
		WorkDir:      "/tmp",
		Client:       testClient(t),
		Pool:         workerPool,
		MessageIssue: msgIssue,
	}

	coord, err := New(cfg)
	require.NoError(t, err)
	require.Equal(t, "sonnet", coord.model)
}

func TestNew_CustomModel(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := testMessageIssue()

	cfg := Config{
		WorkDir:      "/tmp",
		Client:       testClient(t),
		Pool:         workerPool,
		MessageIssue: msgIssue,
		Model:        "opus",
	}

	coord, err := New(cfg)
	require.NoError(t, err)
	require.Equal(t, "opus", coord.model)
}

func TestStatus_Strings(t *testing.T) {
	tests := []struct {
		status   Status
		expected string
	}{
		{StatusPending, "pending"},
		{StatusStarting, "starting"},
		{StatusRunning, "running"},
		{StatusPaused, "paused"},
		{StatusStopping, "stopping"},
		{StatusStopped, "stopped"},
		{StatusFailed, "failed"},
		{Status(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.status.String())
		})
	}
}

func TestCoordinator_IsRunning(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := testMessageIssue()

	coord, err := New(Config{
		Client:       testClient(t),
		WorkDir:      "/tmp",
		Pool:         workerPool,
		MessageIssue: msgIssue,
	})
	require.NoError(t, err)

	// Initially pending, not running
	require.False(t, coord.IsRunning())

	// Manually set to running
	coord.status.Store(int32(StatusRunning))
	require.True(t, coord.IsRunning())

	// Starting also counts as running
	coord.status.Store(int32(StatusStarting))
	require.True(t, coord.IsRunning())

	// Paused is not running
	coord.status.Store(int32(StatusPaused))
	require.False(t, coord.IsRunning())
}

func TestCoordinator_Subscribe(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := testMessageIssue()

	coord, err := New(Config{
		Client:       testClient(t),
		WorkDir:      "/tmp",
		Pool:         workerPool,
		MessageIssue: msgIssue,
	})
	require.NoError(t, err)

	// Subscribe to coordinator events with a context for cleanup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventsCh := coord.Broker().Subscribe(ctx)
	require.NotNil(t, eventsCh)

	// Emit an event using the new method
	coord.emitCoordinatorEvent(events.CoordinatorStatusChange, events.CoordinatorEvent{
		Status: events.StatusReady,
	})

	// Should receive it via subscription
	select {
	case event := <-eventsCh:
		require.Equal(t, pubsub.UpdatedEvent, event.Type)
		require.Equal(t, events.CoordinatorStatusChange, event.Payload.Type)
		require.Equal(t, events.StatusReady, event.Payload.Status)
	case <-time.After(100 * time.Millisecond):
		require.Fail(t, "timeout waiting for event")
	}
}

func TestCoordinator_WorkerBroker(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := testMessageIssue()

	coord, err := New(Config{
		Client:       testClient(t),
		WorkDir:      "/tmp",
		Pool:         workerPool,
		MessageIssue: msgIssue,
	})
	require.NoError(t, err)

	// Subscribe to worker events via Workers() accessor
	workerBroker := coord.Workers()
	require.NotNil(t, workerBroker)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventsCh := workerBroker.Subscribe(ctx)
	require.NotNil(t, eventsCh)

	// Publish a worker event directly
	workerBroker.Publish(pubsub.UpdatedEvent, events.WorkerEvent{
		Type:     events.WorkerSpawned,
		WorkerID: "worker-1",
		TaskID:   "task-1",
	})

	// Should receive it via subscription
	select {
	case event := <-eventsCh:
		require.Equal(t, pubsub.UpdatedEvent, event.Type)
		require.Equal(t, events.WorkerSpawned, event.Payload.Type)
		require.Equal(t, "worker-1", event.Payload.WorkerID)
	case <-time.After(100 * time.Millisecond):
		require.Fail(t, "timeout waiting for worker event")
	}
}

func TestCoordinator_SessionID(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := testMessageIssue()

	coord, err := New(Config{
		Client:       testClient(t),
		WorkDir:      "/tmp",
		Pool:         workerPool,
		MessageIssue: msgIssue,
	})
	require.NoError(t, err)

	// Initially empty
	require.Empty(t, coord.SessionID())
}

func TestCoordinator_DrainQueue_EmitsQueueCountAfterDequeue(t *testing.T) {
	// This test verifies that drainQueue emits CoordinatorMessageQueued events
	// with the remaining queue count after each dequeue operation.
	// This is critical for the UI to show [N queued] label until the queue is empty.

	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := testMessageIssue()

	coord, err := New(Config{
		Client:       testClient(t),
		WorkDir:      "/tmp",
		Pool:         workerPool,
		MessageIssue: msgIssue,
	})
	require.NoError(t, err)

	// Subscribe to coordinator events
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventsCh := coord.Broker().Subscribe(ctx)

	// Pre-populate the queue with 3 messages directly
	coord.queueMu.Lock()
	coord.messageQueue.Enqueue(queue.QueuedMessage{ID: "msg-1", Content: "first", From: "USER"})
	coord.messageQueue.Enqueue(queue.QueuedMessage{ID: "msg-2", Content: "second", From: "USER"})
	coord.messageQueue.Enqueue(queue.QueuedMessage{ID: "msg-3", Content: "third", From: "USER"})
	coord.working = true // Simulate that coordinator was working
	coord.queueMu.Unlock()

	// Call drainQueue which should:
	// 1. Dequeue one message
	// 2. Emit CoordinatorMessageQueued with remaining count (2)
	// Note: deliverMessageLocked will fail because process is nil, but we still verify the event
	coord.drainQueue()

	// Collect CoordinatorMessageQueued event
	var queueEvent events.CoordinatorEvent
	timeout := time.After(500 * time.Millisecond)
collectLoop:
	for {
		select {
		case evt := <-eventsCh:
			if evt.Payload.Type == events.CoordinatorMessageQueued {
				queueEvent = evt.Payload
				break collectLoop
			}
		case <-timeout:
			t.Fatal("timeout waiting for CoordinatorMessageQueued event")
		}
	}

	// Verify the event has the correct remaining count (3 - 1 = 2)
	require.Equal(t, 2, queueEvent.QueueCount, "queue count should be 2 after first dequeue")

	// Verify queue state: working should be false, 2 messages remain
	coord.queueMu.Lock()
	require.False(t, coord.working, "working flag should be false after drainQueue")
	require.Equal(t, 2, coord.messageQueue.Len(), "queue should have 2 messages remaining")
	coord.queueMu.Unlock()
}

func TestCoordinator_DrainQueue_EmitsZeroWhenLastMessageDequeued(t *testing.T) {
	// Test that when the last message is dequeued, the event has QueueCount=0

	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := testMessageIssue()

	coord, err := New(Config{
		Client:       testClient(t),
		WorkDir:      "/tmp",
		Pool:         workerPool,
		MessageIssue: msgIssue,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventsCh := coord.Broker().Subscribe(ctx)

	// Pre-populate with just 1 message
	coord.queueMu.Lock()
	coord.messageQueue.Enqueue(queue.QueuedMessage{ID: "msg-1", Content: "only", From: "USER"})
	coord.working = true
	coord.queueMu.Unlock()

	// Call drainQueue
	coord.drainQueue()

	// Collect CoordinatorMessageQueued event
	var queueEvent events.CoordinatorEvent
	timeout := time.After(500 * time.Millisecond)
collectLoop:
	for {
		select {
		case evt := <-eventsCh:
			if evt.Payload.Type == events.CoordinatorMessageQueued {
				queueEvent = evt.Payload
				break collectLoop
			}
		case <-timeout:
			t.Fatal("timeout waiting for CoordinatorMessageQueued event")
		}
	}

	// Verify the event has count 0 (1 - 1 = 0)
	require.Equal(t, 0, queueEvent.QueueCount, "queue count should be 0 after last message dequeued")

	// Verify queue is empty
	coord.queueMu.Lock()
	require.Equal(t, 0, coord.messageQueue.Len(), "queue should be empty")
	coord.queueMu.Unlock()
}

func TestCoordinator_DrainQueue_NoEventWhenEmpty(t *testing.T) {
	// Test that drainQueue doesn't emit an event when queue is empty

	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := testMessageIssue()

	coord, err := New(Config{
		Client:       testClient(t),
		WorkDir:      "/tmp",
		Pool:         workerPool,
		MessageIssue: msgIssue,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventsCh := coord.Broker().Subscribe(ctx)

	// Queue is empty, just set working to true
	coord.queueMu.Lock()
	coord.working = true
	coord.queueMu.Unlock()

	// Call drainQueue
	coord.drainQueue()

	// Verify no CoordinatorMessageQueued event is emitted
	select {
	case evt := <-eventsCh:
		if evt.Payload.Type == events.CoordinatorMessageQueued {
			t.Fatal("should not emit CoordinatorMessageQueued event when queue is empty")
		}
		// Other events are ok (like status changes)
	case <-time.After(100 * time.Millisecond):
		// Expected - no event should be emitted
	}

	// Verify working flag is still cleared
	coord.queueMu.Lock()
	require.False(t, coord.working, "working flag should be false after drainQueue")
	coord.queueMu.Unlock()
}
