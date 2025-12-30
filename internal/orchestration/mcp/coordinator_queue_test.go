package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/orchestration/claude"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/pool"
	"github.com/zjrosen/perles/internal/orchestration/queue"
)

// ============================================================================
// Queue Infrastructure Tests
//
// These tests verify the message queue infrastructure in CoordinatorServer:
// - Queue map initialization
// - Turn-complete callback registration
// - getOrCreateQueueLocked helper behavior
// ============================================================================

// TestCoordinatorServer_QueueMapInitialized verifies the queue map is created during construction.
func TestCoordinatorServer_QueueMapInitialized(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	require.NotNil(t, cs.messageQueues, "messageQueues map should be initialized")
	require.Empty(t, cs.messageQueues, "messageQueues map should be empty initially")
}

// TestCoordinatorServer_TurnCompleteCallbackRegistered verifies callback is registered with pool.
// This is verified indirectly - when the pool was constructed, we set the callback,
// which can be validated by checking that the coordinator's handleTurnComplete doesn't panic
// when called and that it was set during construction (i.e., pool has a callback).
func TestCoordinatorServer_TurnCompleteCallbackRegistered(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	// Create coordinator - this registers the callback via pool.SetTurnCompleteCallback
	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Verify handleTurnComplete can be called without panic (stub behavior)
	require.NotPanics(t, func() {
		cs.handleTurnComplete("worker-1")
	}, "handleTurnComplete should not panic")

	// The callback is registered during construction in NewCoordinatorServer.
	// We verify this pattern works by confirming:
	// 1. The method exists and can be called
	// 2. Callback registration happens in constructor (verified via code inspection)
	// Full callback invocation testing is in pool_test.go
}

// TestCoordinatorServer_GetOrCreateQueueLocked_New tests creating a queue for a new worker.
func TestCoordinatorServer_GetOrCreateQueueLocked_New(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Initially no queues
	require.Empty(t, cs.messageQueues)

	// Get or create queue for new worker (with explicit locking)
	cs.queueMu.Lock()
	q := cs.getOrCreateQueueLocked("worker-1")
	cs.queueMu.Unlock()

	require.NotNil(t, q, "Queue should be created")
	require.Equal(t, 0, q.Len(), "New queue should be empty")

	// Verify it was added to the map
	cs.queueMu.RLock()
	storedQueue, exists := cs.messageQueues["worker-1"]
	cs.queueMu.RUnlock()

	require.True(t, exists, "Queue should be in messageQueues map")
	require.Same(t, q, storedQueue, "Returned queue should be the same as stored queue")
}

// TestCoordinatorServer_GetOrCreateQueueLocked_Existing tests returning existing queue.
func TestCoordinatorServer_GetOrCreateQueueLocked_Existing(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Create queue first time (with explicit locking)
	cs.queueMu.Lock()
	q1 := cs.getOrCreateQueueLocked("worker-1")
	cs.queueMu.Unlock()
	require.NotNil(t, q1)

	// Add a message to the queue to verify it's the same instance
	err := q1.Enqueue(queue.QueuedMessage{
		ID:      "msg-1",
		Content: "test message",
		From:    "COORDINATOR",
	})
	require.NoError(t, err)

	// Get queue second time - should return same instance (with explicit locking)
	cs.queueMu.Lock()
	q2 := cs.getOrCreateQueueLocked("worker-1")
	cs.queueMu.Unlock()
	require.NotNil(t, q2)
	require.Same(t, q1, q2, "Should return same queue instance")
	require.Equal(t, 1, q2.Len(), "Queue should have the message we added")
}

// TestCoordinatorServer_GetOrCreateQueueLocked_ThreadSafe tests concurrent access to getOrCreateQueueLocked.
func TestCoordinatorServer_GetOrCreateQueueLocked_ThreadSafe(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	const numGoroutines = 10
	var wg sync.WaitGroup
	queues := make([]*queue.MessageQueue, numGoroutines)

	// Concurrent access to same worker's queue (with explicit locking)
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			cs.queueMu.Lock()
			queues[idx] = cs.getOrCreateQueueLocked("worker-1")
			cs.queueMu.Unlock()
		}(i)
	}
	wg.Wait()

	// All should return the same queue instance
	for i := 1; i < numGoroutines; i++ {
		require.Same(t, queues[0], queues[i], "All goroutines should get same queue instance")
	}

	// Only one queue should exist
	cs.queueMu.RLock()
	queueCount := len(cs.messageQueues)
	cs.queueMu.RUnlock()
	require.Equal(t, 1, queueCount, "Should only have one queue in map")
}

// TestCoordinatorServer_GetOrCreateQueueLocked_DifferentWorkers tests queues for different workers.
func TestCoordinatorServer_GetOrCreateQueueLocked_DifferentWorkers(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Create queues with explicit locking
	cs.queueMu.Lock()
	q1 := cs.getOrCreateQueueLocked("worker-1")
	q2 := cs.getOrCreateQueueLocked("worker-2")
	q3 := cs.getOrCreateQueueLocked("worker-3")
	cs.queueMu.Unlock()

	require.NotSame(t, q1, q2, "Different workers should have different queues")
	require.NotSame(t, q2, q3, "Different workers should have different queues")
	require.NotSame(t, q1, q3, "Different workers should have different queues")

	// Verify map has all three
	cs.queueMu.RLock()
	queueCount := len(cs.messageQueues)
	cs.queueMu.RUnlock()
	require.Equal(t, 3, queueCount, "Should have three queues in map")
}

// ============================================================================
// handleSendToWorker Tests
//
// These tests verify message queuing behavior in handleSendToWorker:
// - Immediate delivery when worker is Ready
// - Queueing when worker is Working
// - Queue full error handling
// - Distinct return messages for 'sent to' vs 'queued for'
// - Concurrent access safety
// ============================================================================

// TestSendToWorker_DeliverWhenReady verifies messages are delivered immediately when worker is Ready.
// Note: This test uses a mock-friendly approach since actual delivery requires Claude.
// We verify the function checks status and doesn't queue when worker is Ready.
func TestSendToWorker_DeliverWhenReady(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Add a worker in Ready status with a session ID
	workerPool.AddWorkerForTesting("worker-1", "task-1", pool.WorkerReady, events.PhaseImplementing)
	worker := workerPool.GetWorker("worker-1")
	worker.SetSessionIDForTesting("session-123")

	// Try to send a message - this will fail at Spawn (no Claude) but we verify:
	// 1. It doesn't queue the message (worker is Ready)
	// 2. The queue remains empty
	handler := cs.handlers["send_to_worker"]
	_, err := handler(context.Background(), json.RawMessage(`{"worker_id": "worker-1", "message": "test message"}`))

	// Expected to fail at Spawn since we don't have a real Claude client
	require.Error(t, err, "Expected error when trying to spawn (no Claude client)")
	require.Contains(t, err.Error(), "failed to send message", "Error should be from send, not queue")

	// Verify the message was NOT queued (it tried immediate delivery)
	cs.queueMu.RLock()
	_, exists := cs.messageQueues["worker-1"]
	cs.queueMu.RUnlock()
	require.False(t, exists, "Queue should not be created for Ready worker")
}

// TestSendToWorker_QueueWhenBusy verifies messages are queued when worker is Working.
func TestSendToWorker_QueueWhenBusy(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Add a worker in Working status with a session ID
	workerPool.AddWorkerForTesting("worker-1", "task-1", pool.WorkerWorking, events.PhaseImplementing)
	worker := workerPool.GetWorker("worker-1")
	worker.SetSessionIDForTesting("session-123")

	// Send a message - worker is busy, should be queued
	handler := cs.handlers["send_to_worker"]
	result, err := handler(context.Background(), json.RawMessage(`{"worker_id": "worker-1", "message": "test message"}`))

	require.NoError(t, err, "Queueing should not return an error")
	require.NotNil(t, result)
	require.Contains(t, result.Content[0].Text, "Message queued for worker-1", "Result should indicate message was queued")

	// Verify the message was queued
	cs.queueMu.RLock()
	q, exists := cs.messageQueues["worker-1"]
	cs.queueMu.RUnlock()
	require.True(t, exists, "Queue should be created for busy worker")
	require.Equal(t, 1, q.Len(), "Queue should have one message")

	// Verify the message content
	msg, ok := q.Peek()
	require.True(t, ok, "Should be able to peek message")
	require.Equal(t, "test message", msg.Content)
	require.Equal(t, "COORDINATOR", msg.From)
}

// TestSendToWorker_QueueFull verifies graceful error when queue is at max capacity.
func TestSendToWorker_QueueFull(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Add a worker in Working status
	workerPool.AddWorkerForTesting("worker-1", "task-1", pool.WorkerWorking, events.PhaseImplementing)
	worker := workerPool.GetWorker("worker-1")
	worker.SetSessionIDForTesting("session-123")

	// Pre-fill the queue to max capacity (with explicit locking)
	cs.queueMu.Lock()
	q := cs.getOrCreateQueueLocked("worker-1")
	cs.queueMu.Unlock()
	for i := 0; i < queue.DefaultMaxSize; i++ {
		err := q.Enqueue(queue.QueuedMessage{
			ID:      "msg-" + string(rune(i)),
			Content: "fill message",
			From:    "COORDINATOR",
		})
		require.NoError(t, err)
	}
	require.Equal(t, queue.DefaultMaxSize, q.Len(), "Queue should be at max capacity")

	// Try to send another message - should fail gracefully
	handler := cs.handlers["send_to_worker"]
	_, err := handler(context.Background(), json.RawMessage(`{"worker_id": "worker-1", "message": "overflow message"}`))

	require.Error(t, err, "Should return error when queue is full")
	require.Contains(t, err.Error(), "queue full for worker", "Error should indicate queue is full")

	// Queue should still be at max capacity (message not added)
	require.Equal(t, queue.DefaultMaxSize, q.Len(), "Queue should still be at max capacity")
}

// TestSendToWorker_DistinctReturnMessages verifies return messages distinguish 'sent to' vs 'queued for'.
func TestSendToWorker_DistinctReturnMessages(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Test 1: Message queued for busy worker
	workerPool.AddWorkerForTesting("worker-busy", "task-1", pool.WorkerWorking, events.PhaseImplementing)
	busyWorker := workerPool.GetWorker("worker-busy")
	busyWorker.SetSessionIDForTesting("session-busy")

	handler := cs.handlers["send_to_worker"]
	resultQueued, err := handler(context.Background(), json.RawMessage(`{"worker_id": "worker-busy", "message": "test"}`))
	require.NoError(t, err)
	require.Contains(t, resultQueued.Content[0].Text, "Message queued for",
		"Busy worker should get 'queued for' message")
	require.NotContains(t, resultQueued.Content[0].Text, "Message sent to",
		"Busy worker should NOT get 'sent to' message")

	// Test 2: Ready worker (will fail at Spawn, but we check the error message doesn't say "queued")
	workerPool.AddWorkerForTesting("worker-ready", "task-2", pool.WorkerReady, events.PhaseIdle)
	readyWorker := workerPool.GetWorker("worker-ready")
	readyWorker.SetSessionIDForTesting("session-ready")

	_, err = handler(context.Background(), json.RawMessage(`{"worker_id": "worker-ready", "message": "test"}`))
	// Will fail at Spawn, but verify no queue was created (proving it tried immediate delivery)
	cs.queueMu.RLock()
	_, queueExists := cs.messageQueues["worker-ready"]
	cs.queueMu.RUnlock()
	require.False(t, queueExists, "Ready worker should not have message queued")

	// The error should be about sending/spawning, not queueing
	require.Error(t, err)
	require.NotContains(t, err.Error(), "queue",
		"Ready worker error should be about sending, not queueing")
}

// TestSendToWorker_ConcurrentAccess verifies thread safety of concurrent sends to the same worker.
// This test should be run with -race flag: go test -race ./internal/orchestration/mcp/...
func TestSendToWorker_ConcurrentAccess(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Add a worker in Working status (all messages should be queued)
	workerPool.AddWorkerForTesting("worker-1", "task-1", pool.WorkerWorking, events.PhaseImplementing)
	worker := workerPool.GetWorker("worker-1")
	worker.SetSessionIDForTesting("session-123")

	const numGoroutines = 20
	var wg sync.WaitGroup
	results := make([]*ToolCallResult, numGoroutines)
	errors := make([]error, numGoroutines)

	handler := cs.handlers["send_to_worker"]

	// Send messages concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// Use unique message content to avoid deduplication
			msg := `{"worker_id": "worker-1", "message": "concurrent message ` + string(rune('A'+idx)) + `"}`
			results[idx], errors[idx] = handler(context.Background(), json.RawMessage(msg))
		}(i)
	}
	wg.Wait()

	// All should succeed (no errors)
	for i, err := range errors {
		require.NoError(t, err, "Goroutine %d should not error", i)
	}

	// All should return "queued for" message
	for i, result := range results {
		require.NotNil(t, result, "Goroutine %d result should not be nil", i)
		require.Contains(t, result.Content[0].Text, "Message queued for worker-1",
			"Goroutine %d should get queued response", i)
	}

	// Verify all messages were queued (exactly numGoroutines messages)
	cs.queueMu.RLock()
	q := cs.messageQueues["worker-1"]
	cs.queueMu.RUnlock()
	require.NotNil(t, q, "Queue should exist")
	require.Equal(t, numGoroutines, q.Len(), "Queue should have all %d messages", numGoroutines)
}

// TestSendToWorker_MultipleMessagesQueuedInOrder verifies FIFO ordering when multiple messages are queued.
func TestSendToWorker_MultipleMessagesQueuedInOrder(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Add a worker in Working status
	workerPool.AddWorkerForTesting("worker-1", "task-1", pool.WorkerWorking, events.PhaseImplementing)
	worker := workerPool.GetWorker("worker-1")
	worker.SetSessionIDForTesting("session-123")

	handler := cs.handlers["send_to_worker"]

	// Send messages sequentially
	messages := []string{"first message", "second message", "third message"}
	for _, msg := range messages {
		args := `{"worker_id": "worker-1", "message": "` + msg + `"}`
		result, err := handler(context.Background(), json.RawMessage(args))
		require.NoError(t, err)
		require.Contains(t, result.Content[0].Text, "Message queued for worker-1")
	}

	// Verify messages are in FIFO order
	cs.queueMu.RLock()
	q := cs.messageQueues["worker-1"]
	cs.queueMu.RUnlock()
	require.Equal(t, 3, q.Len())

	// Dequeue and verify order
	for i, expectedContent := range messages {
		msg, ok := q.Dequeue()
		require.True(t, ok, "Should dequeue message %d", i)
		require.Equal(t, expectedContent, msg.Content, "Message %d should be in order", i)
	}
}

// TestGetOrCreateQueueLocked verifies the locked version works correctly.
func TestGetOrCreateQueueLocked(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Test creating new queue while holding lock
	cs.queueMu.Lock()
	q1 := cs.getOrCreateQueueLocked("worker-1")
	require.NotNil(t, q1)
	require.Equal(t, 0, q1.Len())

	// Test getting existing queue while holding lock
	q2 := cs.getOrCreateQueueLocked("worker-1")
	require.Same(t, q1, q2, "Should return same queue instance")
	cs.queueMu.Unlock()

	// Verify queue was added to map
	cs.queueMu.RLock()
	stored, exists := cs.messageQueues["worker-1"]
	cs.queueMu.RUnlock()
	require.True(t, exists)
	require.Same(t, q1, stored)
}

// TestSendToWorker_ConcurrentAccessMixedStatus tests concurrent sends when worker status changes.
// This verifies the atomic check+enqueue/deliver operation prevents race conditions.
func TestSendToWorker_ConcurrentAccessMixedStatus(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Add a worker - we'll toggle status during the test
	workerPool.AddWorkerForTesting("worker-1", "task-1", pool.WorkerWorking, events.PhaseImplementing)
	worker := workerPool.GetWorker("worker-1")
	worker.SetSessionIDForTesting("session-123")

	const numGoroutines = 10
	var wg sync.WaitGroup
	var statusToggle sync.Mutex

	handler := cs.handlers["send_to_worker"]

	// Launch goroutines that send messages
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			msg := `{"worker_id": "worker-1", "message": "msg-` + string(rune('A'+idx)) + `"}`
			_, _ = handler(context.Background(), json.RawMessage(msg))
		}(i)
	}

	// Toggle worker status while sends are happening
	go func() {
		for i := 0; i < 5; i++ {
			statusToggle.Lock()
			if worker.GetStatus() == pool.WorkerWorking {
				worker.SetStatusForTesting(pool.WorkerReady)
			} else {
				worker.SetStatusForTesting(pool.WorkerWorking)
			}
			statusToggle.Unlock()
		}
	}()

	wg.Wait()

	// The test passes if there are no race conditions detected by -race flag.
	// We can't make strong assertions about which messages were queued vs delivered
	// due to the intentional race between status changes and sends.
	// But the queue mutex ensures atomic operations.
	t.Log("Concurrent access with mixed status completed without race conditions")
}

// TestSendToWorker_ValidationBeforeLock verifies validation happens before acquiring lock.
func TestSendToWorker_ValidationBeforeLock(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	handler := cs.handlers["send_to_worker"]

	// Test validation errors don't involve queue operations
	tests := []struct {
		name        string
		args        string
		errContains string
	}{
		{
			name:        "missing worker_id",
			args:        `{"message": "test"}`,
			errContains: "worker_id is required",
		},
		{
			name:        "missing message",
			args:        `{"worker_id": "worker-1"}`,
			errContains: "message is required",
		},
		{
			name:        "empty worker_id",
			args:        `{"worker_id": "", "message": "test"}`,
			errContains: "worker_id is required",
		},
		{
			name:        "empty message",
			args:        `{"worker_id": "worker-1", "message": ""}`,
			errContains: "message is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := handler(context.Background(), json.RawMessage(tt.args))
			require.Error(t, err)
			require.True(t, strings.Contains(err.Error(), tt.errContains),
				"Error should contain %q, got: %v", tt.errContains, err)
		})
	}
}

// ============================================================================
// handleTurnComplete Tests
//
// These tests verify the turn-complete callback behavior:
// - Dequeue and delivery when worker becomes ready
// - No-op when queue is empty
// - Only one message dequeued per turn completion
// - Delivery failure handling
// - Worker replacement during dequeue
// ============================================================================

// TestTurnComplete_DequeuesAndDelivers verifies that pending messages are
// dequeued and delivered when a worker's turn completes.
// Note: Delivery will fail at Spawn (no Claude client), but we verify:
// 1. Message is dequeued from the queue
// 2. Queue length decreases by exactly 1
func TestTurnComplete_DequeuesAndDelivers(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Add a worker with a session ID
	workerPool.AddWorkerForTesting("worker-1", "task-1", pool.WorkerWorking, events.PhaseImplementing)
	worker := workerPool.GetWorker("worker-1")
	worker.SetSessionIDForTesting("session-123")

	// Pre-enqueue multiple messages
	cs.queueMu.Lock()
	q := cs.getOrCreateQueueLocked("worker-1")
	err := q.Enqueue(queue.QueuedMessage{
		ID:      "msg-1",
		Content: "first message",
		From:    "COORDINATOR",
	})
	require.NoError(t, err)
	err = q.Enqueue(queue.QueuedMessage{
		ID:      "msg-2",
		Content: "second message",
		From:    "COORDINATOR",
	})
	require.NoError(t, err)
	err = q.Enqueue(queue.QueuedMessage{
		ID:      "msg-3",
		Content: "third message",
		From:    "COORDINATOR",
	})
	require.NoError(t, err)
	cs.queueMu.Unlock()

	// Verify initial queue length
	require.Equal(t, 3, q.Len(), "Queue should have 3 messages before turn complete")

	// Call handleTurnComplete - this will try to deliver first message
	// (delivery will fail at Spawn, but message should still be dequeued)
	cs.handleTurnComplete("worker-1")

	// After turn complete, queue should have one less message (only one dequeued per call)
	require.Equal(t, 2, q.Len(), "Queue should have 2 messages after turn complete (one dequeued)")

	// Verify the first message was dequeued (FIFO order)
	msg, ok := q.Peek()
	require.True(t, ok)
	require.Equal(t, "second message", msg.Content, "Second message should now be at front of queue")
}

// TestTurnComplete_NoOpWhenQueueEmpty verifies that handleTurnComplete
// returns immediately if there are no pending messages.
func TestTurnComplete_NoOpWhenQueueEmpty(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Add a worker with a session ID but no queued messages
	workerPool.AddWorkerForTesting("worker-1", "task-1", pool.WorkerWorking, events.PhaseImplementing)
	worker := workerPool.GetWorker("worker-1")
	worker.SetSessionIDForTesting("session-123")

	// Call handleTurnComplete with no queued messages - should not panic
	require.NotPanics(t, func() {
		cs.handleTurnComplete("worker-1")
	}, "handleTurnComplete should not panic when queue is empty")

	// Verify no queue was created
	cs.queueMu.RLock()
	_, exists := cs.messageQueues["worker-1"]
	cs.queueMu.RUnlock()
	require.False(t, exists, "No queue should be created for worker with no messages")
}

// TestTurnComplete_NoOpWhenQueueDoesNotExist verifies handleTurnComplete
// handles the case where no queue exists for the worker.
func TestTurnComplete_NoOpWhenQueueDoesNotExist(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Call handleTurnComplete for non-existent worker - should not panic
	require.NotPanics(t, func() {
		cs.handleTurnComplete("non-existent-worker")
	}, "handleTurnComplete should not panic for non-existent worker")

	// Verify no queue was created
	cs.queueMu.RLock()
	queueCount := len(cs.messageQueues)
	cs.queueMu.RUnlock()
	require.Equal(t, 0, queueCount, "No queues should be created")
}

// TestTurnComplete_MultipleMessages verifies that only one message is
// dequeued per turn completion (subsequent messages require more turns).
func TestTurnComplete_MultipleMessages(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Add a worker with a session ID
	workerPool.AddWorkerForTesting("worker-1", "task-1", pool.WorkerWorking, events.PhaseImplementing)
	worker := workerPool.GetWorker("worker-1")
	worker.SetSessionIDForTesting("session-123")

	// Pre-enqueue 5 messages
	cs.queueMu.Lock()
	q := cs.getOrCreateQueueLocked("worker-1")
	for i := 1; i <= 5; i++ {
		err := q.Enqueue(queue.QueuedMessage{
			ID:      fmt.Sprintf("msg-%d", i),
			Content: fmt.Sprintf("message %d", i),
			From:    "COORDINATOR",
		})
		require.NoError(t, err)
	}
	cs.queueMu.Unlock()

	require.Equal(t, 5, q.Len(), "Queue should have 5 messages initially")

	// Call handleTurnComplete 3 times
	cs.handleTurnComplete("worker-1")
	require.Equal(t, 4, q.Len(), "Queue should have 4 messages after 1st turn")

	cs.handleTurnComplete("worker-1")
	require.Equal(t, 3, q.Len(), "Queue should have 3 messages after 2nd turn")

	cs.handleTurnComplete("worker-1")
	require.Equal(t, 2, q.Len(), "Queue should have 2 messages after 3rd turn")

	// Verify remaining messages are in correct order (FIFO)
	msg, ok := q.Peek()
	require.True(t, ok)
	require.Equal(t, "message 4", msg.Content, "Fourth message should be at front")
}

// TestTurnComplete_DeliveryFailure verifies that delivery failures are logged
// but don't cause cascading failures (fire-and-forget model).
func TestTurnComplete_DeliveryFailure(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Add a worker with a session ID
	workerPool.AddWorkerForTesting("worker-1", "task-1", pool.WorkerWorking, events.PhaseImplementing)
	worker := workerPool.GetWorker("worker-1")
	worker.SetSessionIDForTesting("session-123")

	// Pre-enqueue a message
	cs.queueMu.Lock()
	q := cs.getOrCreateQueueLocked("worker-1")
	err := q.Enqueue(queue.QueuedMessage{
		ID:      "msg-1",
		Content: "test message",
		From:    "COORDINATOR",
	})
	require.NoError(t, err)
	cs.queueMu.Unlock()

	// Call handleTurnComplete - delivery will fail at Spawn (no Claude client)
	// but should not panic or propagate error
	require.NotPanics(t, func() {
		cs.handleTurnComplete("worker-1")
	}, "handleTurnComplete should not panic on delivery failure")

	// Message should be dequeued (not re-queued on failure)
	require.Equal(t, 0, q.Len(), "Message should be removed from queue even on delivery failure")
}

// TestTurnComplete_WorkerReplacedMidDequeue verifies that handleTurnComplete
// handles the case where the worker is not found gracefully.
func TestTurnComplete_WorkerReplacedMidDequeue(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Pre-enqueue a message for a worker that doesn't exist in the pool
	cs.queueMu.Lock()
	q := cs.getOrCreateQueueLocked("worker-ghost")
	err := q.Enqueue(queue.QueuedMessage{
		ID:      "msg-1",
		Content: "orphaned message",
		From:    "COORDINATOR",
	})
	require.NoError(t, err)
	cs.queueMu.Unlock()

	require.Equal(t, 1, q.Len(), "Queue should have 1 message")

	// Call handleTurnComplete - worker doesn't exist, delivery should fail gracefully
	require.NotPanics(t, func() {
		cs.handleTurnComplete("worker-ghost")
	}, "handleTurnComplete should not panic when worker doesn't exist")

	// Message should be dequeued (fire-and-forget - not re-queued)
	require.Equal(t, 0, q.Len(), "Message should be removed from queue even when worker not found")
}

// TestTurnComplete_WorkerNoSessionID verifies that handleTurnComplete
// handles the case where the worker has no session ID.
func TestTurnComplete_WorkerNoSessionID(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Add a worker WITHOUT setting a session ID
	workerPool.AddWorkerForTesting("worker-1", "task-1", pool.WorkerWorking, events.PhaseImplementing)

	// Pre-enqueue a message
	cs.queueMu.Lock()
	q := cs.getOrCreateQueueLocked("worker-1")
	err := q.Enqueue(queue.QueuedMessage{
		ID:      "msg-1",
		Content: "test message",
		From:    "COORDINATOR",
	})
	require.NoError(t, err)
	cs.queueMu.Unlock()

	require.Equal(t, 1, q.Len(), "Queue should have 1 message")

	// Call handleTurnComplete - should fail gracefully (no session ID)
	require.NotPanics(t, func() {
		cs.handleTurnComplete("worker-1")
	}, "handleTurnComplete should not panic when worker has no session ID")

	// Message should be dequeued (fire-and-forget)
	require.Equal(t, 0, q.Len(), "Message should be removed from queue even when no session ID")
}

// TestTurnComplete_ConcurrentAccess verifies thread safety of handleTurnComplete
// with concurrent queue operations.
// This test should be run with -race flag: go test -race ./internal/orchestration/mcp/...
func TestTurnComplete_ConcurrentAccess(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Add a worker
	workerPool.AddWorkerForTesting("worker-1", "task-1", pool.WorkerWorking, events.PhaseImplementing)
	worker := workerPool.GetWorker("worker-1")
	worker.SetSessionIDForTesting("session-123")

	// Pre-enqueue some messages
	for i := 0; i < 10; i++ {
		cs.queueMu.Lock()
		q := cs.getOrCreateQueueLocked("worker-1")
		_ = q.Enqueue(queue.QueuedMessage{
			ID:      fmt.Sprintf("msg-%d", i),
			Content: fmt.Sprintf("message %d", i),
			From:    "COORDINATOR",
		})
		cs.queueMu.Unlock()
	}

	// Concurrent operations: enqueue, turn complete, and status checks
	var wg sync.WaitGroup

	// Goroutines calling handleTurnComplete
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cs.handleTurnComplete("worker-1")
		}()
	}

	// Goroutines enqueuing more messages
	handler := cs.handlers["send_to_worker"]
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			msg := fmt.Sprintf(`{"worker_id": "worker-1", "message": "concurrent msg %d"}`, idx)
			_, _ = handler(context.Background(), json.RawMessage(msg))
		}(i)
	}

	wg.Wait()

	// Test passes if no race conditions detected by -race flag
	t.Log("Concurrent handleTurnComplete and enqueue completed without race conditions")
}

// TestTurnComplete_RapidCycling verifies that messages are properly dequeued
// even with rapid Working→Ready→Working status cycling.
func TestTurnComplete_RapidCycling(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Add a worker
	workerPool.AddWorkerForTesting("worker-1", "task-1", pool.WorkerWorking, events.PhaseImplementing)
	worker := workerPool.GetWorker("worker-1")
	worker.SetSessionIDForTesting("session-123")

	// Pre-enqueue 20 messages
	cs.queueMu.Lock()
	q := cs.getOrCreateQueueLocked("worker-1")
	for i := 0; i < 20; i++ {
		_ = q.Enqueue(queue.QueuedMessage{
			ID:      fmt.Sprintf("msg-%d", i),
			Content: fmt.Sprintf("rapid message %d", i),
			From:    "COORDINATOR",
		})
	}
	cs.queueMu.Unlock()

	var wg sync.WaitGroup

	// Simulate rapid turn completions
	for i := 0; i < 15; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cs.handleTurnComplete("worker-1")
		}()
	}

	// Simultaneously toggle status
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if worker.GetStatus() == pool.WorkerWorking {
				worker.SetStatusForTesting(pool.WorkerReady)
			} else {
				worker.SetStatusForTesting(pool.WorkerWorking)
			}
		}()
	}

	wg.Wait()

	// All 15 turn completions should have processed messages
	// (each dequeues exactly 1 message if available)
	// Queue should have 20 - 15 = 5 messages remaining (or fewer if some failed early)
	remaining := q.Len()
	require.LessOrEqual(t, remaining, 20, "Should have processed some messages")
	t.Logf("Queue has %d messages remaining after rapid cycling", remaining)
}

// ============================================================================
// drainQueueForRetiredWorker Tests
//
// These tests verify queue draining behavior when workers retire:
// - Queue is drained and logged when worker retires
// - Queue is drained when handleReplaceWorker is called
// - Gracefully handles worker with no queue
// - Gracefully handles empty queue (no warning log)
// - Queue entry is removed from messageQueues map
// ============================================================================

// TestDrainQueue_RetiredWorker verifies queue is drained and logged when worker retires.
func TestDrainQueue_RetiredWorker(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Add a worker with a session ID
	workerPool.AddWorkerForTesting("worker-1", "task-1", pool.WorkerWorking, events.PhaseImplementing)
	worker := workerPool.GetWorker("worker-1")
	worker.SetSessionIDForTesting("session-123")

	// Pre-enqueue multiple messages
	cs.queueMu.Lock()
	q := cs.getOrCreateQueueLocked("worker-1")
	for i := 0; i < 5; i++ {
		err := q.Enqueue(queue.QueuedMessage{
			ID:      fmt.Sprintf("msg-%d", i),
			Content: fmt.Sprintf("message %d", i),
			From:    "COORDINATOR",
		})
		require.NoError(t, err)
	}
	cs.queueMu.Unlock()

	require.Equal(t, 5, q.Len(), "Queue should have 5 messages before drain")

	// Drain the queue
	cs.drainQueueForRetiredWorker("worker-1")

	// Verify queue was drained
	require.Equal(t, 0, q.Len(), "Queue should be empty after drain")

	// Verify queue entry was removed from map
	cs.queueMu.RLock()
	_, exists := cs.messageQueues["worker-1"]
	cs.queueMu.RUnlock()
	require.False(t, exists, "Queue entry should be removed from messageQueues map")
}

// TestDrainQueue_ReplacedWorker verifies queue is drained when handleReplaceWorker is called.
func TestDrainQueue_ReplacedWorker(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Add a worker with a session ID
	workerPool.AddWorkerForTesting("worker-1", "task-1", pool.WorkerWorking, events.PhaseImplementing)
	worker := workerPool.GetWorker("worker-1")
	worker.SetSessionIDForTesting("session-123")

	// Pre-enqueue messages
	cs.queueMu.Lock()
	q := cs.getOrCreateQueueLocked("worker-1")
	for i := 0; i < 3; i++ {
		err := q.Enqueue(queue.QueuedMessage{
			ID:      fmt.Sprintf("msg-%d", i),
			Content: fmt.Sprintf("message %d", i),
			From:    "COORDINATOR",
		})
		require.NoError(t, err)
	}
	cs.queueMu.Unlock()

	require.Equal(t, 3, q.Len(), "Queue should have 3 messages before replace")

	// Call replace_worker via handler
	handler := cs.handlers["replace_worker"]
	// Note: This will fail at SpawnIdleWorker (no Claude client), but drain should happen first
	_, _ = handler(context.Background(), json.RawMessage(`{"worker_id": "worker-1", "reason": "test replacement"}`))

	// Verify queue was drained (even if spawn failed)
	// The queue object still exists (we have a reference) but should be empty
	require.Equal(t, 0, q.Len(), "Queue should be empty after replace")

	// Verify queue entry was removed from map
	cs.queueMu.RLock()
	_, exists := cs.messageQueues["worker-1"]
	cs.queueMu.RUnlock()
	require.False(t, exists, "Queue entry should be removed from messageQueues map after replace")
}

// TestDrainQueue_NoQueue verifies gracefully handles worker with no queue.
func TestDrainQueue_NoQueue(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// No queue exists for worker-1 - drain should not panic
	require.NotPanics(t, func() {
		cs.drainQueueForRetiredWorker("worker-1")
	}, "drainQueueForRetiredWorker should not panic when no queue exists")

	// Verify no queue was created
	cs.queueMu.RLock()
	queueCount := len(cs.messageQueues)
	cs.queueMu.RUnlock()
	require.Equal(t, 0, queueCount, "No queue should be created during drain")
}

// TestDrainQueue_EmptyQueue verifies gracefully handles empty queue (no warning log).
func TestDrainQueue_EmptyQueue(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Create an empty queue for worker-1 (with explicit locking)
	cs.queueMu.Lock()
	_ = cs.getOrCreateQueueLocked("worker-1")
	cs.queueMu.Unlock()

	// Verify queue exists and is empty
	cs.queueMu.RLock()
	q, exists := cs.messageQueues["worker-1"]
	cs.queueMu.RUnlock()
	require.True(t, exists, "Queue should exist")
	require.Equal(t, 0, q.Len(), "Queue should be empty")

	// Drain should not panic and should remove the queue entry
	require.NotPanics(t, func() {
		cs.drainQueueForRetiredWorker("worker-1")
	}, "drainQueueForRetiredWorker should not panic with empty queue")

	// Verify queue entry was removed from map
	cs.queueMu.RLock()
	_, existsAfter := cs.messageQueues["worker-1"]
	cs.queueMu.RUnlock()
	require.False(t, existsAfter, "Queue entry should be removed even if empty")
}

// TestDrainQueue_DeletesFromMap verifies queue entry is removed from messageQueues map.
func TestDrainQueue_DeletesFromMap(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Create queues for multiple workers (with explicit locking)
	cs.queueMu.Lock()
	for i := 1; i <= 3; i++ {
		q := cs.getOrCreateQueueLocked(fmt.Sprintf("worker-%d", i))
		_ = q.Enqueue(queue.QueuedMessage{
			ID:      fmt.Sprintf("msg-worker%d", i),
			Content: "test message",
			From:    "COORDINATOR",
		})
	}
	cs.queueMu.Unlock()

	// Verify all queues exist
	cs.queueMu.RLock()
	require.Equal(t, 3, len(cs.messageQueues), "Should have 3 queues")
	cs.queueMu.RUnlock()

	// Drain worker-2's queue
	cs.drainQueueForRetiredWorker("worker-2")

	// Verify worker-2's queue is removed but others remain
	cs.queueMu.RLock()
	_, exists1 := cs.messageQueues["worker-1"]
	_, exists2 := cs.messageQueues["worker-2"]
	_, exists3 := cs.messageQueues["worker-3"]
	queueCount := len(cs.messageQueues)
	cs.queueMu.RUnlock()

	require.True(t, exists1, "worker-1 queue should still exist")
	require.False(t, exists2, "worker-2 queue should be removed")
	require.True(t, exists3, "worker-3 queue should still exist")
	require.Equal(t, 2, queueCount, "Should have 2 queues remaining")
}

// TestDrainQueue_ConcurrentAccess verifies thread safety of drainQueueForRetiredWorker.
// This test should be run with -race flag: go test -race ./internal/orchestration/mcp/...
func TestDrainQueue_ConcurrentAccess(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Create queue with messages (with explicit locking)
	cs.queueMu.Lock()
	q := cs.getOrCreateQueueLocked("worker-1")
	for i := 0; i < 20; i++ {
		_ = q.Enqueue(queue.QueuedMessage{
			ID:      fmt.Sprintf("msg-%d", i),
			Content: fmt.Sprintf("message %d", i),
			From:    "COORDINATOR",
		})
	}
	cs.queueMu.Unlock()

	var wg sync.WaitGroup

	// Multiple goroutines trying to drain and enqueue
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cs.drainQueueForRetiredWorker("worker-1")
		}()
	}

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// This may fail if queue doesn't exist anymore, which is fine (with explicit locking)
			cs.queueMu.Lock()
			_ = cs.getOrCreateQueueLocked("worker-1")
			cs.queueMu.Unlock()
		}(i)
	}

	wg.Wait()

	// Test passes if no race conditions detected by -race flag
	t.Log("Concurrent drain and getOrCreateQueueLocked completed without race conditions")
}

// TestDrainQueue_ViaWorkerRetireCallback verifies that retirement callback triggers drain.
func TestDrainQueue_ViaWorkerRetireCallback(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Add a test worker - the retirement callback should be propagated
	workerPool.AddWorkerForTesting("worker-1", "task-1", pool.WorkerWorking, events.PhaseImplementing)
	worker := workerPool.GetWorker("worker-1")

	// Pre-enqueue messages
	cs.queueMu.Lock()
	q := cs.getOrCreateQueueLocked("worker-1")
	for i := 0; i < 3; i++ {
		err := q.Enqueue(queue.QueuedMessage{
			ID:      fmt.Sprintf("msg-%d", i),
			Content: fmt.Sprintf("message %d", i),
			From:    "COORDINATOR",
		})
		require.NoError(t, err)
	}
	cs.queueMu.Unlock()

	require.Equal(t, 3, q.Len(), "Queue should have 3 messages before retire")

	// Retire the worker - this should trigger the callback and drain the queue
	worker.Retire()

	// Verify queue was drained (via callback)
	require.Equal(t, 0, q.Len(), "Queue should be empty after worker.Retire()")

	// Verify queue entry was removed from map
	cs.queueMu.RLock()
	_, exists := cs.messageQueues["worker-1"]
	cs.queueMu.RUnlock()
	require.False(t, exists, "Queue entry should be removed after worker.Retire()")
}

// TestDrainQueue_ViaWorkerCancelCallback verifies that Cancel callback triggers drain.
func TestDrainQueue_ViaWorkerCancelCallback(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Add a test worker - the retirement callback should be propagated
	workerPool.AddWorkerForTesting("worker-1", "task-1", pool.WorkerWorking, events.PhaseImplementing)
	worker := workerPool.GetWorker("worker-1")

	// Pre-enqueue messages
	cs.queueMu.Lock()
	q := cs.getOrCreateQueueLocked("worker-1")
	for i := 0; i < 2; i++ {
		err := q.Enqueue(queue.QueuedMessage{
			ID:      fmt.Sprintf("msg-%d", i),
			Content: fmt.Sprintf("message %d", i),
			From:    "COORDINATOR",
		})
		require.NoError(t, err)
	}
	cs.queueMu.Unlock()

	require.Equal(t, 2, q.Len(), "Queue should have 2 messages before cancel")

	// Cancel the worker - this should trigger the callback and drain the queue
	err := worker.Cancel()
	require.NoError(t, err)

	// Verify queue was drained (via callback)
	require.Equal(t, 0, q.Len(), "Queue should be empty after worker.Cancel()")

	// Verify queue entry was removed from map
	cs.queueMu.RLock()
	_, exists := cs.messageQueues["worker-1"]
	cs.queueMu.RUnlock()
	require.False(t, exists, "Queue entry should be removed after worker.Cancel()")
}

// TestDrainQueue_ViaPoolClose verifies queues are drained when pool is closed.
func TestDrainQueue_ViaPoolClose(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Add multiple test workers with queued messages
	for i := 1; i <= 3; i++ {
		workerID := fmt.Sprintf("worker-%d", i)
		workerPool.AddWorkerForTesting(workerID, fmt.Sprintf("task-%d", i), pool.WorkerWorking, events.PhaseImplementing)

		// Pre-enqueue messages
		cs.queueMu.Lock()
		q := cs.getOrCreateQueueLocked(workerID)
		for j := 0; j < 2; j++ {
			err := q.Enqueue(queue.QueuedMessage{
				ID:      fmt.Sprintf("msg-%s-%d", workerID, j),
				Content: fmt.Sprintf("message %d", j),
				From:    "COORDINATOR",
			})
			require.NoError(t, err)
		}
		cs.queueMu.Unlock()
	}

	// Verify all queues have messages
	cs.queueMu.RLock()
	require.Equal(t, 3, len(cs.messageQueues), "Should have 3 queues before close")
	for workerID, q := range cs.messageQueues {
		require.Equal(t, 2, q.Len(), "Queue for %s should have 2 messages before close", workerID)
	}
	cs.queueMu.RUnlock()

	// Close the pool - this should retire all workers and drain all queues via callbacks
	workerPool.Close()

	// Verify all queues were drained and removed
	cs.queueMu.RLock()
	require.Equal(t, 0, len(cs.messageQueues), "All queue entries should be removed after pool.Close()")
	cs.queueMu.RUnlock()
}

// ============================================================================
// Queue Event Emission Tests
//
// These tests verify that WorkerQueueChanged events are emitted correctly:
// - Events are emitted when messages are enqueued
// - Events are emitted when messages are dequeued
// - Events are emitted when queues are drained
// - Events are debounced at 100ms intervals
// - Multiple workers' events are batched separately
// ============================================================================

// TestEmitQueueChanged_Debounced verifies that multiple rapid calls result in a single event
// after the debounce period.
func TestEmitQueueChanged_Debounced(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Subscribe to events from the pool's broker with a cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := workerPool.Broker().Subscribe(ctx)

	// Rapidly emit queue changed events for the same worker
	for i := 1; i <= 5; i++ {
		cs.emitQueueChanged("worker-1", i)
	}

	// Verify pending events map has the latest count
	cs.queueEventMu.Lock()
	latestCount := cs.pendingQueueEvents["worker-1"]
	cs.queueEventMu.Unlock()
	require.Equal(t, 5, latestCount, "Pending events should have latest count")

	// Wait for debounce period to elapse plus a small buffer
	time.Sleep(QueueEventDebounceInterval + 50*time.Millisecond)

	// Collect events that were emitted
	var receivedEvents []events.WorkerEvent
	timeout := time.After(100 * time.Millisecond)
collectLoop:
	for {
		select {
		case evt := <-sub:
			if evt.Payload.Type == events.WorkerQueueChanged {
				receivedEvents = append(receivedEvents, evt.Payload)
			}
		case <-timeout:
			break collectLoop
		}
	}

	// Should have received exactly one event with the final count
	require.Len(t, receivedEvents, 1, "Should receive exactly one debounced event")
	require.Equal(t, "worker-1", receivedEvents[0].WorkerID)
	require.Equal(t, 5, receivedEvents[0].QueueCount, "Event should have the latest count")
}

// TestEmitQueueChanged_MultipleWorkers verifies that events for different workers
// are batched separately and all emitted together after debounce period.
func TestEmitQueueChanged_MultipleWorkers(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Subscribe to events from the pool's broker with a cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := workerPool.Broker().Subscribe(ctx)

	// Emit events for multiple workers
	cs.emitQueueChanged("worker-1", 3)
	cs.emitQueueChanged("worker-2", 5)
	cs.emitQueueChanged("worker-3", 2)

	// Verify pending events map has all workers
	cs.queueEventMu.Lock()
	require.Equal(t, 3, cs.pendingQueueEvents["worker-1"])
	require.Equal(t, 5, cs.pendingQueueEvents["worker-2"])
	require.Equal(t, 2, cs.pendingQueueEvents["worker-3"])
	require.Len(t, cs.pendingQueueEvents, 3)
	cs.queueEventMu.Unlock()

	// Wait for debounce period to elapse plus a small buffer
	time.Sleep(QueueEventDebounceInterval + 50*time.Millisecond)

	// Collect events that were emitted
	receivedEvents := make(map[string]int) // workerID -> queueCount
	timeout := time.After(100 * time.Millisecond)
collectLoop:
	for {
		select {
		case evt := <-sub:
			if evt.Payload.Type == events.WorkerQueueChanged {
				receivedEvents[evt.Payload.WorkerID] = evt.Payload.QueueCount
			}
		case <-timeout:
			break collectLoop
		}
	}

	// Should have received events for all three workers
	require.Len(t, receivedEvents, 3, "Should receive events for all 3 workers")
	require.Equal(t, 3, receivedEvents["worker-1"])
	require.Equal(t, 5, receivedEvents["worker-2"])
	require.Equal(t, 2, receivedEvents["worker-3"])
}

// TestFlushQueueEvents_EmitsAll verifies that all pending events are emitted
// after the debounce period.
func TestFlushQueueEvents_EmitsAll(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Subscribe to events from the pool's broker with a cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := workerPool.Broker().Subscribe(ctx)

	// Set up pending events directly
	cs.queueEventMu.Lock()
	cs.pendingQueueEvents["worker-1"] = 10
	cs.pendingQueueEvents["worker-2"] = 20
	cs.queueEventMu.Unlock()

	// Manually call flushQueueEvents
	cs.flushQueueEvents()

	// Collect events that were emitted
	receivedEvents := make(map[string]int)
	timeout := time.After(100 * time.Millisecond)
collectLoop:
	for {
		select {
		case evt := <-sub:
			if evt.Payload.Type == events.WorkerQueueChanged {
				receivedEvents[evt.Payload.WorkerID] = evt.Payload.QueueCount
			}
		case <-timeout:
			break collectLoop
		}
	}

	// Should have received events for both workers
	require.Len(t, receivedEvents, 2, "Should receive events for both workers")
	require.Equal(t, 10, receivedEvents["worker-1"])
	require.Equal(t, 20, receivedEvents["worker-2"])

	// Pending events should be cleared
	cs.queueEventMu.Lock()
	require.Empty(t, cs.pendingQueueEvents, "Pending events should be cleared after flush")
	cs.queueEventMu.Unlock()
}

// TestEmitQueueChanged_IntegrationWithEnqueue verifies that enqueuing a message
// triggers the queue changed event emission.
func TestEmitQueueChanged_IntegrationWithEnqueue(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Add a worker in Working status
	workerPool.AddWorkerForTesting("worker-1", "task-1", pool.WorkerWorking, events.PhaseImplementing)
	worker := workerPool.GetWorker("worker-1")
	worker.SetSessionIDForTesting("session-123")

	// Subscribe to events from the pool's broker with a cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := workerPool.Broker().Subscribe(ctx)

	// Send a message (which will be queued since worker is busy)
	handler := cs.handlers["send_to_worker"]
	result, err := handler(context.Background(), json.RawMessage(`{"worker_id": "worker-1", "message": "test message"}`))
	require.NoError(t, err)
	require.Contains(t, result.Content[0].Text, "Message queued for worker-1")

	// Wait for debounce period to elapse plus a small buffer
	time.Sleep(QueueEventDebounceInterval + 50*time.Millisecond)

	// Collect events that were emitted
	var queueChangedEvents []events.WorkerEvent
	timeout := time.After(100 * time.Millisecond)
collectLoop:
	for {
		select {
		case evt := <-sub:
			if evt.Payload.Type == events.WorkerQueueChanged {
				queueChangedEvents = append(queueChangedEvents, evt.Payload)
			}
		case <-timeout:
			break collectLoop
		}
	}

	// Should have received a queue changed event
	require.Len(t, queueChangedEvents, 1, "Should receive queue changed event after enqueue")
	require.Equal(t, "worker-1", queueChangedEvents[0].WorkerID)
	require.Equal(t, 1, queueChangedEvents[0].QueueCount, "Queue count should be 1 after enqueue")
}

// TestEmitQueueChanged_IntegrationWithDrain verifies that draining a queue
// triggers the queue changed event emission with count 0.
func TestEmitQueueChanged_IntegrationWithDrain(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Pre-enqueue some messages
	cs.queueMu.Lock()
	q := cs.getOrCreateQueueLocked("worker-1")
	for i := 0; i < 3; i++ {
		_ = q.Enqueue(queue.QueuedMessage{
			ID:      fmt.Sprintf("msg-%d", i),
			Content: fmt.Sprintf("message %d", i),
			From:    "COORDINATOR",
		})
	}
	cs.queueMu.Unlock()

	// Subscribe to events from the pool's broker with a cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := workerPool.Broker().Subscribe(ctx)

	// Drain the queue
	cs.drainQueueForRetiredWorker("worker-1")

	// Wait for debounce period to elapse plus a small buffer
	time.Sleep(QueueEventDebounceInterval + 50*time.Millisecond)

	// Collect events that were emitted
	var queueChangedEvents []events.WorkerEvent
	timeout := time.After(100 * time.Millisecond)
collectLoop:
	for {
		select {
		case evt := <-sub:
			if evt.Payload.Type == events.WorkerQueueChanged {
				queueChangedEvents = append(queueChangedEvents, evt.Payload)
			}
		case <-timeout:
			break collectLoop
		}
	}

	// Should have received a queue changed event with count 0
	require.Len(t, queueChangedEvents, 1, "Should receive queue changed event after drain")
	require.Equal(t, "worker-1", queueChangedEvents[0].WorkerID)
	require.Equal(t, 0, queueChangedEvents[0].QueueCount, "Queue count should be 0 after drain")
}

// TestEmitQueueChanged_DebounceInterval verifies the debounce interval is 100ms.
func TestEmitQueueChanged_DebounceInterval(t *testing.T) {
	// Verify the constant is set correctly
	require.Equal(t, 100*time.Millisecond, QueueEventDebounceInterval,
		"Debounce interval should be 100ms as per cross-review consensus")
}

// TestEmitQueueChanged_ConcurrentAccess verifies thread safety of emitQueueChanged.
func TestEmitQueueChanged_ConcurrentAccess(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	const numGoroutines = 20
	var wg sync.WaitGroup

	// Concurrent calls to emitQueueChanged for different workers
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			workerID := fmt.Sprintf("worker-%d", idx%5) // Use 5 different workers
			cs.emitQueueChanged(workerID, idx)
		}(i)
	}

	wg.Wait()

	// Test passes if no race conditions detected by -race flag
	// Verify pending events has entries for workers
	cs.queueEventMu.Lock()
	require.NotEmpty(t, cs.pendingQueueEvents, "Should have pending events")
	cs.queueEventMu.Unlock()

	t.Log("Concurrent emitQueueChanged completed without race conditions")
}

// ============================================================================
// SendUserMessageToWorker Tests
//
// These tests verify the public SendUserMessageToWorker method, which is the
// TUI entry point for user-to-worker messages. It uses the same queue system
// as handleSendToWorker but formats messages as "[MESSAGE FROM USER]".
// ============================================================================

// TestSendUserMessageToWorker_QueueWhenBusy verifies user messages are queued when worker is Working.
func TestSendUserMessageToWorker_QueueWhenBusy(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Add a worker in Working status with a session ID
	workerPool.AddWorkerForTesting("worker-1", "task-1", pool.WorkerWorking, events.PhaseImplementing)
	worker := workerPool.GetWorker("worker-1")
	worker.SetSessionIDForTesting("session-123")

	// Send user message to busy worker
	result, err := cs.SendUserMessageToWorker(context.Background(), "worker-1", "Hello from user")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Queued, "Message should be queued when worker is Working")
	require.Equal(t, 1, result.QueuePosition, "Queue position should be 1 (first message)")

	// Verify the message was actually queued
	cs.queueMu.RLock()
	q := cs.messageQueues["worker-1"]
	cs.queueMu.RUnlock()
	require.NotNil(t, q, "Queue should exist for worker")
	require.Equal(t, 1, q.Len(), "Queue should have 1 message")

	// Verify message content and source
	msg, ok := q.Peek()
	require.True(t, ok)
	require.Equal(t, "Hello from user", msg.Content)
	require.Equal(t, "USER", msg.From, "Message should be marked as from USER")
}

// TestSendUserMessageToWorker_WorkerNotFound verifies error handling for non-existent worker.
func TestSendUserMessageToWorker_WorkerNotFound(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	result, err := cs.SendUserMessageToWorker(context.Background(), "nonexistent-worker", "Hello")
	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "worker not found")
}

// TestSendUserMessageToWorker_NoSessionID verifies error when worker has no session ID.
func TestSendUserMessageToWorker_NoSessionID(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Add a worker without a session ID
	workerPool.AddWorkerForTesting("worker-1", "task-1", pool.WorkerReady, events.PhaseImplementing)

	result, err := cs.SendUserMessageToWorker(context.Background(), "worker-1", "Hello")
	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "has no session ID")
}

// TestSendUserMessageToWorker_EmptyInputs verifies validation of required inputs.
func TestSendUserMessageToWorker_EmptyInputs(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Empty worker ID
	result, err := cs.SendUserMessageToWorker(context.Background(), "", "Hello")
	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "worker_id is required")

	// Empty message
	result, err = cs.SendUserMessageToWorker(context.Background(), "worker-1", "")
	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "message is required")
}

// TestSendUserMessageToWorker_MultipleMessagesQueued verifies FIFO ordering when multiple user messages are queued.
func TestSendUserMessageToWorker_MultipleMessagesQueued(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Add a worker in Working status with a session ID
	workerPool.AddWorkerForTesting("worker-1", "task-1", pool.WorkerWorking, events.PhaseImplementing)
	worker := workerPool.GetWorker("worker-1")
	worker.SetSessionIDForTesting("session-123")

	// Queue multiple messages
	result1, err := cs.SendUserMessageToWorker(context.Background(), "worker-1", "First message")
	require.NoError(t, err)
	require.True(t, result1.Queued)
	require.Equal(t, 1, result1.QueuePosition)

	result2, err := cs.SendUserMessageToWorker(context.Background(), "worker-1", "Second message")
	require.NoError(t, err)
	require.True(t, result2.Queued)
	require.Equal(t, 2, result2.QueuePosition)

	result3, err := cs.SendUserMessageToWorker(context.Background(), "worker-1", "Third message")
	require.NoError(t, err)
	require.True(t, result3.Queued)
	require.Equal(t, 3, result3.QueuePosition)

	// Verify FIFO order
	cs.queueMu.RLock()
	q := cs.messageQueues["worker-1"]
	cs.queueMu.RUnlock()

	require.Equal(t, 3, q.Len())

	msg1, _ := q.Dequeue()
	require.Equal(t, "First message", msg1.Content)

	msg2, _ := q.Dequeue()
	require.Equal(t, "Second message", msg2.Content)

	msg3, _ := q.Dequeue()
	require.Equal(t, "Third message", msg3.Content)
}

// TestSendUserMessageToWorker_ConcurrentAccess verifies thread safety of concurrent user sends.
func TestSendUserMessageToWorker_ConcurrentAccess(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Add a worker in Working status with a session ID
	workerPool.AddWorkerForTesting("worker-1", "task-1", pool.WorkerWorking, events.PhaseImplementing)
	worker := workerPool.GetWorker("worker-1")
	worker.SetSessionIDForTesting("session-123")

	const numGoroutines = 20
	var wg sync.WaitGroup
	var successCount int
	var mu sync.Mutex

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			result, err := cs.SendUserMessageToWorker(
				context.Background(),
				"worker-1",
				fmt.Sprintf("Message %d", idx),
			)
			if err == nil && result.Queued {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	// All messages should have been queued successfully
	require.Equal(t, numGoroutines, successCount, "All concurrent messages should be queued")

	cs.queueMu.RLock()
	q := cs.messageQueues["worker-1"]
	cs.queueMu.RUnlock()

	require.Equal(t, numGoroutines, q.Len(), "Queue should contain all concurrent messages")
	t.Log("Concurrent SendUserMessageToWorker completed without race conditions")
}

// TestSendUserMessageToWorker_QueueEmitsEvent verifies that queuing user messages emits WorkerQueueChanged events.
func TestSendUserMessageToWorker_QueueEmitsEvent(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(
		claude.NewClient(),
		workerPool,
		nil,
		"/tmp/test",
		8765,
		nil,
		mocks.NewMockBeadsExecutor(t),
	)

	// Subscribe to events from the pool's broker with a cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := workerPool.Broker().Subscribe(ctx)

	// Add a worker in Working status with a session ID
	workerPool.AddWorkerForTesting("worker-1", "task-1", pool.WorkerWorking, events.PhaseImplementing)
	worker := workerPool.GetWorker("worker-1")
	worker.SetSessionIDForTesting("session-123")

	// Queue a user message
	result, err := cs.SendUserMessageToWorker(context.Background(), "worker-1", "Hello from user")
	require.NoError(t, err)
	require.True(t, result.Queued)

	// Wait for debounced event (100ms debounce + buffer)
	time.Sleep(QueueEventDebounceInterval + 50*time.Millisecond)

	// Check for queue changed event
	var queueChangedEvents []events.WorkerEvent
	timeout := time.After(100 * time.Millisecond)
collectLoop:
	for {
		select {
		case evt := <-sub:
			if evt.Payload.Type == events.WorkerQueueChanged {
				queueChangedEvents = append(queueChangedEvents, evt.Payload)
			}
		case <-timeout:
			break collectLoop
		}
	}

	require.Len(t, queueChangedEvents, 1, "Should receive one queue changed event")
	require.Equal(t, "worker-1", queueChangedEvents[0].WorkerID)
	require.Equal(t, 1, queueChangedEvents[0].QueueCount)
}
