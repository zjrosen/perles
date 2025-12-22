package coordinator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"perles/internal/orchestration/claude"
	"perles/internal/orchestration/message"
	"perles/internal/orchestration/pool"
)

func TestStart_RequiresPendingStatus(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := message.New()

	coord, err := New(Config{
		Client:       claude.NewClient(),
		WorkDir:      "/tmp",
		Pool:         workerPool,
		MessageIssue: msgIssue,
	})
	require.NoError(t, err)

	// Manually set to running
	coord.status.Store(int32(StatusRunning))

	// Start should fail
	err = coord.Start()
	require.Error(t, err)
	require.Contains(t, err.Error(), "already started")
}

func TestPause(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := message.New()

	coord, err := New(Config{
		Client:       claude.NewClient(),
		WorkDir:      "/tmp",
		Pool:         workerPool,
		MessageIssue: msgIssue,
	})
	require.NoError(t, err)

	// Set to running (simulating started state)
	coord.status.Store(int32(StatusRunning))

	// Pause should succeed
	err = coord.Pause()
	require.NoError(t, err)
	require.Equal(t, StatusPaused, coord.Status())
}

func TestPause_RequiresRunning(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := message.New()

	coord, err := New(Config{
		Client:       claude.NewClient(),
		WorkDir:      "/tmp",
		Pool:         workerPool,
		MessageIssue: msgIssue,
	})
	require.NoError(t, err)

	// Try to pause while pending
	err = coord.Pause()
	require.Error(t, err)
	require.Contains(t, err.Error(), "not running")
}

func TestResume(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := message.New()

	coord, err := New(Config{
		Client:       claude.NewClient(),
		WorkDir:      "/tmp",
		Pool:         workerPool,
		MessageIssue: msgIssue,
	})
	require.NoError(t, err)

	// Set to paused
	coord.status.Store(int32(StatusPaused))

	// Resume should succeed
	err = coord.Resume()
	require.NoError(t, err)
	require.Equal(t, StatusRunning, coord.Status())
}

func TestResume_RequiresPaused(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := message.New()

	coord, err := New(Config{
		Client:       claude.NewClient(),
		WorkDir:      "/tmp",
		Pool:         workerPool,
		MessageIssue: msgIssue,
	})
	require.NoError(t, err)

	// Try to resume while running
	coord.status.Store(int32(StatusRunning))
	err = coord.Resume()
	require.Error(t, err)
	require.Contains(t, err.Error(), "not paused")
}

func TestCancel(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	// Don't defer Close since Cancel will close it

	msgIssue := message.New()

	coord, err := New(Config{
		Client:       claude.NewClient(),
		WorkDir:      "/tmp",
		Pool:         workerPool,
		MessageIssue: msgIssue,
	})
	require.NoError(t, err)

	// Set to running
	coord.status.Store(int32(StatusRunning))

	// Cancel should succeed
	err = coord.Cancel()
	require.NoError(t, err)
	require.Equal(t, StatusStopped, coord.Status())
}

func TestCancel_DoubleCallSafe(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})

	msgIssue := message.New()

	coord, err := New(Config{
		Client:       claude.NewClient(),
		WorkDir:      "/tmp",
		Pool:         workerPool,
		MessageIssue: msgIssue,
	})
	require.NoError(t, err)

	// Set to running
	coord.status.Store(int32(StatusRunning))

	// First cancel
	err = coord.Cancel()
	require.NoError(t, err)

	// Second cancel should not panic
	err = coord.Cancel()
	require.NoError(t, err)
	require.Equal(t, StatusStopped, coord.Status())
}

func TestCancel_WhenAlreadyStopped(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := message.New()

	coord, err := New(Config{
		Client:       claude.NewClient(),
		WorkDir:      "/tmp",
		Pool:         workerPool,
		MessageIssue: msgIssue,
	})
	require.NoError(t, err)

	// Set to already stopped
	coord.status.Store(int32(StatusStopped))

	// Cancel should be idempotent
	err = coord.Cancel()
	require.NoError(t, err)
}

func TestSendUserMessage_RequiresRunning(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := message.New()

	coord, err := New(Config{
		Client:       claude.NewClient(),
		WorkDir:      "/tmp",
		Pool:         workerPool,
		MessageIssue: msgIssue,
	})
	require.NoError(t, err)

	// Try to send message while pending
	err = coord.SendUserMessage("hello")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not running")
}

func TestSendUserMessage_RequiresProcess(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := message.New()

	coord, err := New(Config{
		Client:       claude.NewClient(),
		WorkDir:      "/tmp",
		Pool:         workerPool,
		MessageIssue: msgIssue,
	})
	require.NoError(t, err)

	// Set to running but no process
	coord.status.Store(int32(StatusRunning))

	// Should fail due to no process
	err = coord.SendUserMessage("hello")
	require.Error(t, err)
	require.Contains(t, err.Error(), "process not available")
}

func TestWait_ReturnsImmediately(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := message.New()

	coord, err := New(Config{
		Client:       claude.NewClient(),
		WorkDir:      "/tmp",
		Pool:         workerPool,
		MessageIssue: msgIssue,
	})
	require.NoError(t, err)

	// Wait should return immediately since no goroutines added
	err = coord.Wait()
	require.NoError(t, err)
}

func TestBrokersCloseIdempotent(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := message.New()

	coord, err := New(Config{
		Client:       claude.NewClient(),
		WorkDir:      "/tmp",
		Pool:         workerPool,
		MessageIssue: msgIssue,
	})
	require.NoError(t, err)

	// Multiple closes should not panic (broker.Close is idempotent)
	coord.Broker().Close()
	coord.Broker().Close()
	coord.Broker().Close()

	// Worker broker is managed by pool now - test pool's broker idempotency
	coord.Workers().Close()
	coord.Workers().Close()

	// Broker is closed - subscribe should still work but receive no events
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	eventsCh := coord.Broker().Subscribe(ctx)
	require.NotNil(t, eventsCh)
}
