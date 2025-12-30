package coordinator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/orchestration/claude"
	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/orchestration/pool"
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
	result, err := coord.SendUserMessage("hello")
	require.Error(t, err)
	require.Nil(t, result)
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
	result, err := coord.SendUserMessage("hello")
	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "process not available")
}

// ============================================================================
// Message Queue Tests
// ============================================================================

func TestSendUserMessage_QueuesWhenWorking(t *testing.T) {
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

	// Set to running with working=true (simulating busy state)
	coord.status.Store(int32(StatusRunning))
	coord.working = true

	// Send message while working - should be queued
	result, err := coord.SendUserMessage("hello")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Queued, "Message should be queued when coordinator is working")
	require.Equal(t, 1, result.QueuePosition)

	// Verify message is in queue
	require.Equal(t, 1, coord.messageQueue.Len())
}

func TestSendUserMessage_MultipleMessagesQueued(t *testing.T) {
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

	// Set to running with working=true
	coord.status.Store(int32(StatusRunning))
	coord.working = true

	// Queue multiple messages
	result1, err := coord.SendUserMessage("first")
	require.NoError(t, err)
	require.True(t, result1.Queued)
	require.Equal(t, 1, result1.QueuePosition)

	result2, err := coord.SendUserMessage("second")
	require.NoError(t, err)
	require.True(t, result2.Queued)
	require.Equal(t, 2, result2.QueuePosition)

	result3, err := coord.SendUserMessage("third")
	require.NoError(t, err)
	require.True(t, result3.Queued)
	require.Equal(t, 3, result3.QueuePosition)

	// Verify FIFO order
	require.Equal(t, 3, coord.messageQueue.Len())

	msg1, ok := coord.messageQueue.Dequeue()
	require.True(t, ok)
	require.Equal(t, "first", msg1.Content)
	require.Equal(t, "USER", msg1.From)

	msg2, ok := coord.messageQueue.Dequeue()
	require.True(t, ok)
	require.Equal(t, "second", msg2.Content)

	msg3, ok := coord.messageQueue.Dequeue()
	require.True(t, ok)
	require.Equal(t, "third", msg3.Content)
}

func TestDrainQueue_ClearsWorkingFlag(t *testing.T) {
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

	// Set working=true
	coord.working = true

	// Drain queue (no messages)
	coord.drainQueue()

	// Working should be false after drain
	require.False(t, coord.working, "Working flag should be cleared after drainQueue")
}

func TestDrainQueue_EmptyQueue(t *testing.T) {
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

	// Set to running
	coord.status.Store(int32(StatusRunning))
	coord.working = true

	// Drain empty queue - should not panic
	require.NotPanics(t, func() {
		coord.drainQueue()
	})

	// Working should be cleared
	require.False(t, coord.working)
}

func TestMessageQueue_InitializedOnNew(t *testing.T) {
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

	// Queue should be initialized and empty
	require.NotNil(t, coord.messageQueue)
	require.Equal(t, 0, coord.messageQueue.Len())
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

func TestBuildReplacePrompt_ContainsHandoffInstructions(t *testing.T) {
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

	prompt := coord.buildReplacePrompt()

	// Should mention reading the handoff message
	require.Contains(t, prompt, "READ THE HANDOFF FIRST", "prompt should instruct reading handoff")
	require.Contains(t, prompt, "handoff message", "prompt should mention handoff message")
	require.Contains(t, prompt, "previous coordinator", "prompt should mention previous coordinator")
	require.Contains(t, prompt, "read_message_log", "prompt should mention read_message_log tool")
}

func TestBuildReplacePrompt_WaitsForUser(t *testing.T) {
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

	prompt := coord.buildReplacePrompt()

	// Should instruct waiting for user direction
	require.Contains(t, prompt, "Wait for the user to provide direction", "prompt should say to wait for user")
	require.Contains(t, prompt, "Do NOT assign tasks", "prompt should prohibit autonomous task assignment")
	require.Contains(t, prompt, "Do NOT", "prompt should contain prohibitions")
	require.Contains(t, prompt, "until the user tells you what to do", "prompt should emphasize waiting for user")
}

func TestBuildReplacePrompt_NoImmediateActions(t *testing.T) {
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

	prompt := coord.buildReplacePrompt()

	// Should NOT contain the old "IMMEDIATE ACTIONS REQUIRED" section
	require.NotContains(t, prompt, "IMMEDIATE ACTIONS REQUIRED", "prompt should not contain immediate actions")

	// Should still be a valid prompt with header
	require.Contains(t, prompt, "[CONTEXT REFRESH - NEW SESSION]", "prompt should have header")
	require.Contains(t, prompt, "WHAT TO DO NOW", "prompt should have guidance section")
}
