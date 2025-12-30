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
