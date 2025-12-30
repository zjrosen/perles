package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/mock"
	"github.com/zjrosen/perles/internal/orchestration/pool"
)

// TestCoordinatorServer_SendToWorker_Deduplication verifies that duplicate messages
// to the same worker are deduplicated, resulting in only one actual Spawn call.
func TestCoordinatorServer_SendToWorker_Deduplication(t *testing.T) {
	// Create mock client that tracks spawn calls
	mockClient := mock.NewClient()

	// Set up SpawnFunc to return a process with a session ID
	mockClient.SpawnFunc = func(ctx context.Context, cfg client.Config) (client.HeadlessProcess, error) {
		proc := mock.NewProcess()
		proc.SetSessionID("test-session-123")
		// Send init event so session ID is captured
		proc.SendInitEvent("test-session-123", "/tmp/test")
		return proc, nil
	}

	// Create worker pool with mock client
	workerPool := pool.NewWorkerPool(pool.Config{
		Client:     mockClient,
		MaxWorkers: 4,
	})
	defer workerPool.Close()

	// Spawn a worker so we have one in the pool
	workerID, err := workerPool.SpawnWorker(client.Config{
		WorkDir: "/tmp/test",
		Prompt:  "test prompt",
	})
	require.NoError(t, err)

	// Wait briefly for the worker to process init event and get session ID
	time.Sleep(50 * time.Millisecond)

	// Verify worker has session ID
	worker := workerPool.GetWorker(workerID)
	require.NotNil(t, worker)
	require.NotEmpty(t, worker.GetSessionID(), "worker should have session ID")

	// Transition worker to Ready so messages are delivered immediately (not queued)
	worker.SetStatusForTesting(pool.WorkerReady)

	// Reset spawn count so we only count calls from handleSendToWorker
	mockClient.Reset()

	// Create coordinator server with the mock client and pool
	cs := NewCoordinatorServer(mockClient, workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	ctx := context.Background()
	messageArgs := `{"worker_id": "` + workerID + `", "message": "test message content"}`

	// First send should succeed and trigger actual spawn
	result1, err := cs.handleSendToWorker(ctx, json.RawMessage(messageArgs))
	require.NoError(t, err)
	require.NotNil(t, result1)
	require.True(t, strings.Contains(result1.Content[0].Text, "Message sent"),
		"First send should succeed with 'Message sent' response")

	// Immediate duplicate should also return success (idempotent)
	result2, err := cs.handleSendToWorker(ctx, json.RawMessage(messageArgs))
	require.NoError(t, err)
	require.NotNil(t, result2)
	require.True(t, strings.Contains(result2.Content[0].Text, "Message sent"),
		"Duplicate send should still return success (idempotent)")

	// CRITICAL: Verify mock Spawn was only called once
	require.Equal(t, 1, mockClient.SpawnCount(),
		"Spawn should only be called once despite duplicate send_to_worker calls")
}

// TestCoordinatorServer_SendToWorker_DifferentMessages verifies that different messages
// to the same worker are NOT deduplicated.
func TestCoordinatorServer_SendToWorker_DifferentMessages(t *testing.T) {
	// Create mock client that tracks spawn calls
	mockClient := mock.NewClient()

	// Set up SpawnFunc to return a process with a session ID
	mockClient.SpawnFunc = func(ctx context.Context, cfg client.Config) (client.HeadlessProcess, error) {
		proc := mock.NewProcess()
		proc.SetSessionID("test-session-456")
		proc.SendInitEvent("test-session-456", "/tmp/test")
		return proc, nil
	}

	// Create worker pool with mock client
	workerPool := pool.NewWorkerPool(pool.Config{
		Client:     mockClient,
		MaxWorkers: 4,
	})
	defer workerPool.Close()

	// Spawn a worker
	workerID, err := workerPool.SpawnWorker(client.Config{
		WorkDir: "/tmp/test",
		Prompt:  "test prompt",
	})
	require.NoError(t, err)

	// Wait for session ID
	time.Sleep(50 * time.Millisecond)

	// Transition worker to Ready so messages are delivered immediately (not queued)
	worker := workerPool.GetWorker(workerID)
	require.NotNil(t, worker)
	worker.SetStatusForTesting(pool.WorkerReady)

	// Reset spawn count
	mockClient.Reset()

	// Create coordinator server
	cs := NewCoordinatorServer(mockClient, workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	ctx := context.Background()

	// Send first message
	args1 := `{"worker_id": "` + workerID + `", "message": "first message"}`
	result1, err := cs.handleSendToWorker(ctx, json.RawMessage(args1))
	require.NoError(t, err)
	require.NotNil(t, result1)

	// After first send, pool.ResumeWorker starts an async goroutine that sets status to Working.
	// Wait briefly to ensure that goroutine has run before we reset status.
	time.Sleep(10 * time.Millisecond)

	// Reset to Ready so second message is delivered immediately, not queued.
	worker.SetStatusForTesting(pool.WorkerReady)

	// Send different message - should NOT be deduplicated
	args2 := `{"worker_id": "` + workerID + `", "message": "second message"}`
	result2, err := cs.handleSendToWorker(ctx, json.RawMessage(args2))
	require.NoError(t, err)
	require.NotNil(t, result2)

	// Both messages should trigger spawns (different content = different hash)
	require.Equal(t, 2, mockClient.SpawnCount(),
		"Different messages should both trigger Spawn calls")
}

// TestCoordinatorServer_SendToWorker_DifferentWorkers verifies that the same message
// to different workers is NOT deduplicated.
func TestCoordinatorServer_SendToWorker_DifferentWorkers(t *testing.T) {
	// Create mock client
	mockClient := mock.NewClient()

	sessionCounter := 0
	var mu sync.Mutex
	mockClient.SpawnFunc = func(ctx context.Context, cfg client.Config) (client.HeadlessProcess, error) {
		mu.Lock()
		sessionCounter++
		sessionID := "test-session-" + string(rune('a'+sessionCounter))
		mu.Unlock()

		proc := mock.NewProcess()
		proc.SetSessionID(sessionID)
		proc.SendInitEvent(sessionID, "/tmp/test")
		return proc, nil
	}

	// Create worker pool with mock client
	workerPool := pool.NewWorkerPool(pool.Config{
		Client:     mockClient,
		MaxWorkers: 4,
	})
	defer workerPool.Close()

	// Spawn two workers
	workerID1, err := workerPool.SpawnWorker(client.Config{
		WorkDir: "/tmp/test",
		Prompt:  "test prompt",
	})
	require.NoError(t, err)

	workerID2, err := workerPool.SpawnWorker(client.Config{
		WorkDir: "/tmp/test",
		Prompt:  "test prompt",
	})
	require.NoError(t, err)

	// Wait for session IDs
	time.Sleep(50 * time.Millisecond)

	// Transition workers to Ready so messages are delivered immediately (not queued)
	worker1 := workerPool.GetWorker(workerID1)
	require.NotNil(t, worker1)
	worker1.SetStatusForTesting(pool.WorkerReady)

	worker2 := workerPool.GetWorker(workerID2)
	require.NotNil(t, worker2)
	worker2.SetStatusForTesting(pool.WorkerReady)

	// Reset spawn count
	mockClient.Reset()

	// Create coordinator server
	cs := NewCoordinatorServer(mockClient, workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	ctx := context.Background()
	sameMessage := "identical message content"

	// Send same message to worker 1
	args1 := `{"worker_id": "` + workerID1 + `", "message": "` + sameMessage + `"}`
	result1, err := cs.handleSendToWorker(ctx, json.RawMessage(args1))
	require.NoError(t, err)
	require.NotNil(t, result1)

	// Send same message to worker 2 - should NOT be deduplicated
	args2 := `{"worker_id": "` + workerID2 + `", "message": "` + sameMessage + `"}`
	result2, err := cs.handleSendToWorker(ctx, json.RawMessage(args2))
	require.NoError(t, err)
	require.NotNil(t, result2)

	// Both should trigger spawns (different worker = different hash key)
	require.Equal(t, 2, mockClient.SpawnCount(),
		"Same message to different workers should both trigger Spawn calls")
}

// TestCoordinatorServer_SendToWorker_Deduplication_Concurrent verifies that concurrent
// duplicate sends still only result in one actual spawn.
func TestCoordinatorServer_SendToWorker_Deduplication_Concurrent(t *testing.T) {
	// Create mock client
	mockClient := mock.NewClient()

	mockClient.SpawnFunc = func(ctx context.Context, cfg client.Config) (client.HeadlessProcess, error) {
		proc := mock.NewProcess()
		proc.SetSessionID("test-session-concurrent")
		proc.SendInitEvent("test-session-concurrent", "/tmp/test")
		return proc, nil
	}

	// Create worker pool with mock client
	workerPool := pool.NewWorkerPool(pool.Config{
		Client:     mockClient,
		MaxWorkers: 4,
	})
	defer workerPool.Close()

	// Spawn a worker
	workerID, err := workerPool.SpawnWorker(client.Config{
		WorkDir: "/tmp/test",
		Prompt:  "test prompt",
	})
	require.NoError(t, err)

	// Wait for session ID
	time.Sleep(50 * time.Millisecond)

	// Transition worker to Ready so messages are delivered immediately (not queued)
	worker := workerPool.GetWorker(workerID)
	require.NotNil(t, worker)
	worker.SetStatusForTesting(pool.WorkerReady)

	// Reset spawn count
	mockClient.Reset()

	// Create coordinator server
	cs := NewCoordinatorServer(mockClient, workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	ctx := context.Background()
	messageArgs := `{"worker_id": "` + workerID + `", "message": "concurrent test message"}`

	// Launch multiple concurrent sends of the same message
	const numGoroutines = 10
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := cs.handleSendToWorker(ctx, json.RawMessage(messageArgs))
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		require.NoError(t, err, "Unexpected error from concurrent send")
	}

	// CRITICAL: Despite 10 concurrent sends of identical message,
	// only ONE should have triggered an actual Spawn
	require.Equal(t, 1, mockClient.SpawnCount(),
		"Concurrent duplicate sends should only result in one Spawn call")
}
