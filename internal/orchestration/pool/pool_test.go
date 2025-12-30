package pool

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/pubsub"
)

// testProcess is a test helper that wraps channels to simulate a HeadlessProcess.
// It provides convenience methods for sending events in tests.
type testProcess struct {
	events    chan client.OutputEvent
	errors    chan error
	sessionID string
	workDir   string
	status    client.ProcessStatus
	done      chan struct{}
}

func newTestProcess() *testProcess {
	return &testProcess{
		events: make(chan client.OutputEvent, 100),
		errors: make(chan error, 10),
		status: client.StatusRunning,
		done:   make(chan struct{}),
	}
}

func (p *testProcess) Events() <-chan client.OutputEvent { return p.events }
func (p *testProcess) Errors() <-chan error              { return p.errors }
func (p *testProcess) SessionRef() string                { return p.sessionID }
func (p *testProcess) Status() client.ProcessStatus      { return p.status }
func (p *testProcess) IsRunning() bool                   { return p.status == client.StatusRunning }
func (p *testProcess) WorkDir() string                   { return p.workDir }
func (p *testProcess) PID() int                          { return 0 }
func (p *testProcess) Cancel() error {
	if p.status == client.StatusRunning {
		p.status = client.StatusCancelled
		close(p.events)
		close(p.errors)
		close(p.done)
	}
	return nil
}
func (p *testProcess) Wait() error {
	<-p.done
	return nil
}

func (p *testProcess) SendInitEvent(sessionID, workDir string) {
	p.sessionID = sessionID
	p.workDir = workDir
	p.events <- client.OutputEvent{
		Type:      client.EventSystem,
		SubType:   "init",
		SessionID: sessionID,
		WorkDir:   workDir,
	}
}

func (p *testProcess) SendTextEvent(text string) {
	p.events <- client.OutputEvent{
		Type: client.EventAssistant,
		Message: &client.MessageContent{
			Role:    "assistant",
			Content: []client.ContentBlock{{Type: "text", Text: text}},
		},
	}
}

func (p *testProcess) Complete() {
	if p.status == client.StatusRunning {
		p.status = client.StatusCompleted
		close(p.events)
		close(p.errors)
		close(p.done)
	}
}

func TestNewWorkerPool_Defaults(t *testing.T) {
	pool := NewWorkerPool(Config{})
	defer pool.Close()

	require.Equal(t, DefaultMaxWorkers, pool.MaxWorkers())
	require.NotNil(t, pool.Broker())
}

func TestNewWorkerPool_CustomConfig(t *testing.T) {
	pool := NewWorkerPool(Config{
		MaxWorkers:     5,
		BufferCapacity: 200,
	})
	defer pool.Close()

	require.Equal(t, 5, pool.MaxWorkers())
}

func TestNewWorkerPool_InvalidValues(t *testing.T) {
	// Zero/negative values should use defaults
	pool := NewWorkerPool(Config{
		MaxWorkers:     0,
		BufferCapacity: -1,
	})
	defer pool.Close()

	require.Equal(t, DefaultMaxWorkers, pool.MaxWorkers())
}

func TestWorkerPool_GetWorker(t *testing.T) {
	pool := NewWorkerPool(Config{})
	defer pool.Close()

	// Non-existent worker
	require.Nil(t, pool.GetWorker("does-not-exist"))

	// Add a worker directly for testing
	pool.mu.Lock()
	pool.workers["test-worker"] = newWorker("test-worker", 100)
	pool.mu.Unlock()

	worker := pool.GetWorker("test-worker")
	require.NotNil(t, worker)
	require.Equal(t, "test-worker", worker.ID)
}

func TestWorkerPool_ActiveWorkers(t *testing.T) {
	pool := NewWorkerPool(Config{})
	defer pool.Close()

	// Add workers with different statuses
	pool.mu.Lock()
	pool.workers["w1"] = newWorker("w1", 100)
	pool.workers["w1"].Status = WorkerWorking

	pool.workers["w2"] = newWorker("w2", 100)
	pool.workers["w2"].Status = WorkerRetired

	pool.workers["w3"] = newWorker("w3", 100)
	pool.workers["w3"].Status = WorkerReady
	pool.mu.Unlock()

	active := pool.ActiveWorkers()
	require.Len(t, active, 2) // w1 (working) and w3 (ready)

	// Verify IDs
	ids := make(map[string]bool)
	for _, w := range active {
		ids[w.ID] = true
	}
	require.True(t, ids["w1"])
	require.True(t, ids["w3"])
	require.False(t, ids["w2"])
}

func TestWorkerPool_RetireWorker(t *testing.T) {
	pool := NewWorkerPool(Config{})
	defer pool.Close()

	pool.mu.Lock()
	pool.workers["w1"] = newWorker("w1", 100)
	pool.workers["w1"].Status = WorkerWorking
	pool.mu.Unlock()

	err := pool.RetireWorker("w1")
	require.NoError(t, err)
	require.Equal(t, WorkerRetired, pool.GetWorker("w1").GetStatus())
}

func TestWorkerPool_RetireWorker_NotFound(t *testing.T) {
	pool := NewWorkerPool(Config{})
	defer pool.Close()

	err := pool.RetireWorker("does-not-exist")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

func TestWorkerPool_RetireAll(t *testing.T) {
	pool := NewWorkerPool(Config{})
	defer pool.Close()

	pool.mu.Lock()
	pool.workers["w1"] = newWorker("w1", 100)
	pool.workers["w1"].Status = WorkerWorking

	pool.workers["w2"] = newWorker("w2", 100)
	pool.workers["w2"].Status = WorkerWorking

	pool.workers["w3"] = newWorker("w3", 100)
	pool.workers["w3"].Status = WorkerRetired // Already retired
	pool.mu.Unlock()

	pool.RetireAll()

	require.Equal(t, WorkerRetired, pool.GetWorker("w1").GetStatus())
	require.Equal(t, WorkerRetired, pool.GetWorker("w2").GetStatus())
	require.Equal(t, WorkerRetired, pool.GetWorker("w3").GetStatus())
}

func TestWorkerPool_Close(t *testing.T) {
	pool := NewWorkerPool(Config{})

	// Add some workers
	pool.mu.Lock()
	pool.workers["w1"] = newWorker("w1", 100)
	pool.workers["w1"].Status = WorkerWorking
	pool.mu.Unlock()

	// Close should not panic
	pool.Close()

	// Double close should be safe
	pool.Close()

	// Broker should be closed (subscribing to closed broker returns nil channel)
	// We verify by checking the pool is marked as closed
	require.True(t, pool.closed.Load())
}

func TestWorkerPool_SpawnAfterClose(t *testing.T) {
	pool := NewWorkerPool(Config{})
	pool.Close()

	_, err := pool.SpawnWorker(client.Config{
		WorkDir: "/tmp",
		Prompt:  "test",
	})
	require.ErrorIs(t, err, ErrPoolClosed)
}

func TestWorkerPool_MaxWorkerLimit(t *testing.T) {
	pool := NewWorkerPool(Config{MaxWorkers: 2})
	defer pool.Close()

	// Add 2 active workers directly
	pool.mu.Lock()
	pool.workers["w1"] = newWorker("w1", 100)
	pool.workers["w1"].Status = WorkerWorking

	pool.workers["w2"] = newWorker("w2", 100)
	pool.workers["w2"].Status = WorkerWorking
	pool.mu.Unlock()

	// SpawnWorker should fail (we can't actually spawn because claude isn't available,
	// but we can test that the limit check happens first)
	// In a unit test without claude available, we need to mock the limit check separately

	// Verify limit check logic: activeCount >= maxWorkers
	// Count active workers manually since ActiveWorkerCount was removed
	activeCount := len(pool.ActiveWorkers())
	require.Equal(t, 2, activeCount)
	require.Equal(t, 2, pool.MaxWorkers())
}

func TestWorkerPool_ConcurrentAccess(t *testing.T) {
	pool := NewWorkerPool(Config{})
	defer pool.Close()

	var wg sync.WaitGroup

	// Add workers
	pool.mu.Lock()
	for i := 0; i < 10; i++ {
		w := newWorker("w"+string(rune('0'+i)), 100)
		w.Status = WorkerWorking
		pool.workers[w.ID] = w
	}
	pool.mu.Unlock()

	// Concurrent reads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = pool.ActiveWorkers()
				_ = pool.ReadyWorkers()
				_ = pool.GetWorker("w0")
			}
		}()
	}

	// Concurrent retires
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = pool.RetireWorker("w" + string(rune('0'+n)))
		}(i)
	}

	wg.Wait()
	// Should not panic
}

func TestWorkerPool_Broker(t *testing.T) {
	pool := NewWorkerPool(Config{})
	defer pool.Close()

	broker := pool.Broker()
	require.NotNil(t, broker)

	// Subscribe to broker
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub := broker.Subscribe(ctx)
	require.NotNil(t, sub)

	// Publish an event via broker
	go func() {
		broker.Publish(pubsub.UpdatedEvent, WorkerEvent{WorkerID: "test", Type: events.WorkerStatusChange})
	}()

	select {
	case event := <-sub:
		require.Equal(t, "test", event.Payload.WorkerID)
	case <-time.After(time.Second):
		require.Fail(t, "Timeout waiting for event")
	}
}

func TestWorkerPool_WorkerIDGeneration(t *testing.T) {
	pool := NewWorkerPool(Config{})
	defer pool.Close()

	// Verify counter increments
	id1 := pool.workerCounter.Add(1)
	id2 := pool.workerCounter.Add(1)
	id3 := pool.workerCounter.Add(1)

	require.Equal(t, int64(1), id1)
	require.Equal(t, int64(2), id2)
	require.Equal(t, int64(3), id3)
}

// Tests using mock client for spawning workers

func TestWorkerPool_SpawnWorker_WithMockClient(t *testing.T) {
	mockClient := mocks.NewMockHeadlessClient(t)
	proc := newTestProcess()

	mockClient.EXPECT().Spawn(mock.Anything, mock.Anything).Return(proc, nil)

	pool := NewWorkerPool(Config{
		Client:     mockClient,
		MaxWorkers: 4,
	})
	defer pool.Close()

	ctx := context.Background()
	eventsCh := pool.Broker().Subscribe(ctx)

	// Spawn a worker
	workerID, err := pool.SpawnWorker(client.Config{
		WorkDir: "/test",
		Prompt:  "test prompt",
	})

	require.NoError(t, err)
	require.NotEmpty(t, workerID)

	// Verify worker was created
	worker := pool.GetWorker(workerID)
	require.NotNil(t, worker)

	// Verify spawn event was sent
	select {
	case event := <-eventsCh:
		require.Equal(t, events.WorkerSpawned, event.Payload.Type)
		require.Equal(t, workerID, event.Payload.WorkerID)
	case <-time.After(time.Second):
		require.FailNow(t, "timeout waiting for spawn event")
	}

	// Clean up
	proc.Complete()
}

func TestWorkerPool_SpawnWorker_Error(t *testing.T) {
	mockClient := mocks.NewMockHeadlessClient(t)
	mockClient.EXPECT().Spawn(mock.Anything, mock.Anything).Return(nil, errors.New("spawn failed"))

	pool := NewWorkerPool(Config{
		Client:     mockClient,
		MaxWorkers: 4,
	})
	defer pool.Close()

	_, err := pool.SpawnWorker(client.Config{
		WorkDir: "/test",
		Prompt:  "test prompt",
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "spawn failed")

	// Worker should not be in pool
	require.Len(t, pool.ActiveWorkers(), 0)
}

func TestWorkerPool_SpawnWorker_NoClient(t *testing.T) {
	pool := NewWorkerPool(Config{}) // No client configured
	defer pool.Close()

	_, err := pool.SpawnWorker(client.Config{
		WorkDir: "/test",
		Prompt:  "test",
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "no client configured")
}

func TestWorkerPool_SpawnWorker_MaxLimit(t *testing.T) {
	mockClient := mocks.NewMockHeadlessClient(t)
	proc1 := newTestProcess()
	proc2 := newTestProcess()

	mockClient.EXPECT().Spawn(mock.Anything, mock.Anything).Return(proc1, nil).Once()
	mockClient.EXPECT().Spawn(mock.Anything, mock.Anything).Return(proc2, nil).Once()

	pool := NewWorkerPool(Config{
		Client:     mockClient,
		MaxWorkers: 2,
	})
	defer pool.Close()

	// Spawn first worker
	_, err := pool.SpawnWorker(client.Config{WorkDir: "/test", Prompt: "1"})
	require.NoError(t, err)

	// Spawn second worker
	_, err = pool.SpawnWorker(client.Config{WorkDir: "/test", Prompt: "2"})
	require.NoError(t, err)

	// Third should fail (max limit check happens before Spawn is called)
	_, err = pool.SpawnWorker(client.Config{WorkDir: "/test", Prompt: "3"})
	require.ErrorIs(t, err, ErrMaxWorkers)

	// Clean up
	proc1.Complete()
	proc2.Complete()
}

func TestWorkerPool_SpawnWorkerWithID(t *testing.T) {
	mockClient := mocks.NewMockHeadlessClient(t)
	proc := newTestProcess()

	mockClient.EXPECT().Spawn(mock.Anything, mock.Anything).Return(proc, nil)

	pool := NewWorkerPool(Config{
		Client:     mockClient,
		MaxWorkers: 4,
	})
	defer pool.Close()

	// Spawn with specific ID
	workerID, err := pool.SpawnWorkerWithID("my-custom-worker", client.Config{
		WorkDir: "/test",
		Prompt:  "test",
	})

	require.NoError(t, err)
	require.Equal(t, "my-custom-worker", workerID)

	worker := pool.GetWorker("my-custom-worker")
	require.NotNil(t, worker)
	require.Equal(t, "my-custom-worker", worker.ID)

	// Clean up
	proc.Complete()
}

func TestWorkerPool_WorkerLifecycle_WithMockProcess(t *testing.T) {
	mockClient := mocks.NewMockHeadlessClient(t)
	proc := newTestProcess()

	mockClient.EXPECT().Spawn(mock.Anything, mock.Anything).Return(proc, nil)

	pool := NewWorkerPool(Config{
		Client:     mockClient,
		MaxWorkers: 4,
	})
	defer pool.Close()

	ctx := context.Background()
	eventsCh := pool.Broker().Subscribe(ctx)

	// Spawn worker
	workerID, err := pool.SpawnWorker(client.Config{
		WorkDir: "/test",
		Prompt:  "test",
	})
	require.NoError(t, err)

	// Wait for spawn event
	select {
	case event := <-eventsCh:
		require.Equal(t, events.WorkerSpawned, event.Payload.Type)
	case <-time.After(time.Second):
		require.Fail(t, "timeout waiting for spawn event")
	}

	// Send init event
	proc.SendInitEvent("sess-123", "/test")

	// Send output
	proc.SendTextEvent("Hello!")

	// Verify output event
	select {
	case event := <-eventsCh:
		require.Equal(t, events.WorkerOutput, event.Payload.Type)
		require.Equal(t, "Hello!", event.Payload.Output)
	case <-time.After(time.Second):
		require.Fail(t, "timeout waiting for output event")
	}

	// Complete process
	proc.Complete()

	// Wait for status change to Ready
	select {
	case event := <-eventsCh:
		require.Equal(t, events.WorkerStatusChange, event.Payload.Type)
		require.Equal(t, WorkerReady, event.Payload.Status)
	case <-time.After(time.Second):
		require.Fail(t, "timeout waiting for status change event")
	}

	// Verify worker is ready
	worker := pool.GetWorker(workerID)
	require.Equal(t, WorkerReady, worker.GetStatus())
	require.Equal(t, "sess-123", worker.GetSessionID())
}

func TestWorkerPool_ResumeWorker(t *testing.T) {
	mockClient := mocks.NewMockHeadlessClient(t)
	spawnProc := newTestProcess()
	resumeProc := newTestProcess()

	mockClient.EXPECT().Spawn(mock.Anything, mock.Anything).Return(spawnProc, nil)

	pool := NewWorkerPool(Config{
		Client:     mockClient,
		MaxWorkers: 4,
	})
	defer pool.Close()

	// Spawn worker
	workerID, err := pool.SpawnWorker(client.Config{WorkDir: "/test", Prompt: "init"})
	require.NoError(t, err)

	// Complete initial spawn
	spawnProc.SendInitEvent("sess-123", "/test")
	spawnProc.Complete()

	// Give worker time to process
	time.Sleep(50 * time.Millisecond)

	// Resume worker with new process
	err = pool.ResumeWorker(workerID, resumeProc)
	require.NoError(t, err)

	// Send events on resumed process
	resumeProc.SendTextEvent("Resumed!")

	// Give time for event processing
	time.Sleep(50 * time.Millisecond)

	// Complete resumed process
	resumeProc.Complete()

	// Worker should return to Ready
	time.Sleep(50 * time.Millisecond)
	worker := pool.GetWorker(workerID)
	require.Equal(t, WorkerReady, worker.GetStatus())
}

func TestWorkerPool_ResumeWorker_NotFound(t *testing.T) {
	pool := NewWorkerPool(Config{})
	defer pool.Close()

	proc := newTestProcess()
	err := pool.ResumeWorker("nonexistent", proc)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

func TestWorkerPool_AssignTask(t *testing.T) {
	mockClient := mocks.NewMockHeadlessClient(t)
	proc := newTestProcess()

	mockClient.EXPECT().Spawn(mock.Anything, mock.Anything).Return(proc, nil)

	pool := NewWorkerPool(Config{
		Client:     mockClient,
		MaxWorkers: 4,
	})
	defer pool.Close()

	// Spawn worker
	workerID, err := pool.SpawnWorker(client.Config{WorkDir: "/test", Prompt: "init"})
	require.NoError(t, err)

	// Complete initial processing
	proc.SendInitEvent("sess-123", "/test")
	proc.Complete()

	// Wait for worker to become Ready
	time.Sleep(50 * time.Millisecond)

	// Assign task
	err = pool.AssignTaskToWorker(workerID, "task-123")
	require.NoError(t, err)

	worker := pool.GetWorker(workerID)
	require.Equal(t, WorkerWorking, worker.GetStatus())
	require.Equal(t, "task-123", worker.GetTaskID())
}

func TestWorkerPool_CancelWorker(t *testing.T) {
	mockClient := mocks.NewMockHeadlessClient(t)
	proc := newTestProcess()

	mockClient.EXPECT().Spawn(mock.Anything, mock.Anything).Return(proc, nil)

	pool := NewWorkerPool(Config{
		Client:     mockClient,
		MaxWorkers: 4,
	})
	defer pool.Close()

	// Spawn worker
	workerID, err := pool.SpawnWorker(client.Config{WorkDir: "/test", Prompt: "init"})
	require.NoError(t, err)

	// Give time for worker to start
	time.Sleep(50 * time.Millisecond)

	// Cancel worker
	err = pool.CancelWorker(workerID)
	require.NoError(t, err)

	// Verify worker is retired
	worker := pool.GetWorker(workerID)
	require.Equal(t, WorkerRetired, worker.GetStatus())
}

func TestWorkerPool_EmitIncomingMessage(t *testing.T) {
	pool := NewWorkerPool(Config{})
	defer pool.Close()

	ctx := context.Background()
	eventsCh := pool.Broker().Subscribe(ctx)

	pool.EmitIncomingMessage("worker-1", "task-1", "Hello worker")

	select {
	case event := <-eventsCh:
		require.Equal(t, events.WorkerIncoming, event.Payload.Type)
		require.Equal(t, "worker-1", event.Payload.WorkerID)
		require.Equal(t, "task-1", event.Payload.TaskID)
		require.Equal(t, "Hello worker", event.Payload.Message)
	case <-time.After(time.Second):
		require.Fail(t, "timeout waiting for incoming message event")
	}
}

func TestWorkerPool_SetWorkerPhase(t *testing.T) {
	pool := NewWorkerPool(Config{})
	defer pool.Close()

	// Add a worker with a task
	pool.mu.Lock()
	worker := newWorker("w1", 100)
	worker.Status = WorkerWorking
	worker.TaskID = "task-123"
	worker.Phase = events.PhaseImplementing
	pool.workers["w1"] = worker
	pool.mu.Unlock()

	// Update phase
	err := pool.SetWorkerPhase("w1", events.PhaseAwaitingReview)
	require.NoError(t, err)

	// Verify phase was updated
	require.Equal(t, events.PhaseAwaitingReview, pool.GetWorker("w1").GetPhase())
}

func TestWorkerPool_SetWorkerPhase_NotFound(t *testing.T) {
	pool := NewWorkerPool(Config{})
	defer pool.Close()

	err := pool.SetWorkerPhase("does-not-exist", events.PhaseReviewing)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

func TestWorkerPool_SetWorkerPhase_PreservesTaskID(t *testing.T) {
	pool := NewWorkerPool(Config{})
	defer pool.Close()

	// Add a worker with a task
	pool.mu.Lock()
	worker := newWorker("w1", 100)
	worker.Status = WorkerWorking
	worker.TaskID = "task-456"
	worker.Phase = events.PhaseImplementing
	pool.workers["w1"] = worker
	pool.mu.Unlock()

	// Update phase multiple times
	err := pool.SetWorkerPhase("w1", events.PhaseAwaitingReview)
	require.NoError(t, err)

	err = pool.SetWorkerPhase("w1", events.PhaseAddressingFeedback)
	require.NoError(t, err)

	err = pool.SetWorkerPhase("w1", events.PhaseCommitting)
	require.NoError(t, err)

	// Verify task ID was preserved throughout all phase changes
	w := pool.GetWorker("w1")
	require.Equal(t, "task-456", w.GetTaskID())
	require.Equal(t, events.PhaseCommitting, w.GetPhase())
}

func TestWorkerPool_SetWorkerTaskID(t *testing.T) {
	pool := NewWorkerPool(Config{})
	defer pool.Close()

	// Add a worker
	pool.mu.Lock()
	worker := newWorker("w1", 100)
	worker.Status = WorkerReady
	pool.workers["w1"] = worker
	pool.mu.Unlock()

	// Set task ID
	err := pool.SetWorkerTaskID("w1", "task-reviewer")
	require.NoError(t, err)

	// Verify task ID was set
	require.Equal(t, "task-reviewer", pool.GetWorker("w1").GetTaskID())
}

func TestWorkerPool_SetWorkerTaskID_NotFound(t *testing.T) {
	pool := NewWorkerPool(Config{})
	defer pool.Close()

	err := pool.SetWorkerTaskID("does-not-exist", "task-123")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

func TestWorkerPool_TaskIDPreservedThroughProcessCompletion(t *testing.T) {
	mockClient := mocks.NewMockHeadlessClient(t)
	proc := newTestProcess()

	mockClient.EXPECT().Spawn(mock.Anything, mock.Anything).Return(proc, nil)

	pool := NewWorkerPool(Config{
		Client:     mockClient,
		MaxWorkers: 4,
	})
	defer pool.Close()

	// Spawn worker
	workerID, err := pool.SpawnWorker(client.Config{WorkDir: "/test", Prompt: "init"})
	require.NoError(t, err)

	// Complete initial processing
	proc.SendInitEvent("sess-123", "/test")
	proc.Complete()

	// Wait for worker to become Ready
	time.Sleep(50 * time.Millisecond)

	// Assign task
	err = pool.AssignTaskToWorker(workerID, "task-lifecycle")
	require.NoError(t, err)

	// Set phase
	err = pool.SetWorkerPhase(workerID, events.PhaseImplementing)
	require.NoError(t, err)

	// Simulate another process completion (like when AI responds to a message)
	// In real scenario, this happens when ResumeWorker's process completes
	worker := pool.GetWorker(workerID)

	// Directly call CompleteTask to simulate what handleProcessComplete does
	worker.CompleteTask()

	// Verify task ID is preserved
	require.Equal(t, "task-lifecycle", worker.GetTaskID(), "Task ID should be preserved after process completion")
	require.Equal(t, WorkerReady, worker.GetStatus())
}
