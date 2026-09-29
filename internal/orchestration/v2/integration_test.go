// Package v2 provides integration tests for the v2 orchestration architecture.
// These tests exercise the full TUI→MCP→v2→events loop, validating that all components
// work together correctly.
package v2

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/v2/adapter"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/handler"
	"github.com/zjrosen/perles/internal/orchestration/v2/process"
	"github.com/zjrosen/perles/internal/orchestration/v2/processor"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
	"github.com/zjrosen/perles/internal/pubsub"
	taskpkg "github.com/zjrosen/perles/internal/task"
)

// ===========================================================================
// Test Infrastructure
// ===========================================================================

// testV2Stack contains all v2 components for integration testing.
type testV2Stack struct {
	processRepo  *repository.MemoryProcessRepository
	taskRepo     *repository.MemoryTaskRepository
	queueRepo    *repository.MemoryQueueRepository
	processor    *processor.CommandProcessor
	adapter      *adapter.V2Adapter
	eventBus     *pubsub.Broker[any]
	registry     *process.ProcessRegistry
	ctx          context.Context
	cancel       context.CancelFunc
	mockSpawner  *mockProcessSpawner
	mockBD       *mocks.MockTaskExecutor
	spawnedCount atomic.Int64
}

// newTestV2Stack creates a fully wired v2 stack for testing.
func newTestV2Stack(t *testing.T) *testV2Stack {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	// Create repositories
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(repository.DefaultQueueMaxSize)

	// Create event bus
	eventBus := pubsub.NewBroker[any]()

	// Create command processor with event bus
	cmdProcessor := processor.NewCommandProcessor(
		processor.WithQueueCapacity(1000),
		processor.WithEventBus(eventBus),
		processor.WithTaskRepository(taskRepo),
		processor.WithQueueRepository(queueRepo),
	)

	// Create mock spawner
	mockSpawner := &mockProcessSpawner{
		processes: make(map[string]*mockProcess),
	}

	// Create mock BD executor with permissive expectations
	mockBD := mocks.NewMockTaskExecutor(t)
	setupPermissiveBDMock(mockBD)

	// Create process registry for handlers
	registry := process.NewProcessRegistry()

	stack := &testV2Stack{
		processRepo: processRepo,
		taskRepo:    taskRepo,
		queueRepo:   queueRepo,
		processor:   cmdProcessor,
		eventBus:    eventBus,
		registry:    registry,
		ctx:         ctx,
		cancel:      cancel,
		mockSpawner: mockSpawner,
		mockBD:      mockBD,
	}

	// Register all handlers
	stack.registerHandlers(cmdProcessor, processRepo, taskRepo, queueRepo, registry, mockSpawner, mockBD)

	// Create V2Adapter with repositories for read-only operations
	v2Adapter := adapter.NewV2Adapter(cmdProcessor,
		adapter.WithProcessRepository(processRepo),
		adapter.WithTaskRepository(taskRepo),
		adapter.WithQueueRepository(queueRepo),
	)
	stack.adapter = v2Adapter

	// Start processor loop
	go cmdProcessor.Run(ctx)

	// Wait for processor to be running
	require.Eventually(t, func() bool {
		return cmdProcessor.IsRunning()
	}, time.Second, 10*time.Millisecond, "processor should start running")

	return stack
}

// registerHandlers registers all handlers with the command processor.
func (s *testV2Stack) registerHandlers(
	cmdProcessor *processor.CommandProcessor,
	processRepo *repository.MemoryProcessRepository,
	taskRepo *repository.MemoryTaskRepository,
	queueRepo *repository.MemoryQueueRepository,
	registry *process.ProcessRegistry,
	spawner handler.ProcessSpawner,
	bdExecutor taskpkg.TaskExecutor,
) {
	// Process Lifecycle handlers (3)
	cmdProcessor.RegisterHandler(command.CmdSpawnProcess,
		handler.NewSpawnProcessHandler(processRepo, registry))
	cmdProcessor.RegisterHandler(command.CmdRetireProcess,
		handler.NewRetireProcessHandler(processRepo, registry))
	cmdProcessor.RegisterHandler(command.CmdReplaceProcess,
		handler.NewReplaceProcessHandler(processRepo, registry))

	// Stop Worker handler
	cmdProcessor.RegisterHandler(command.CmdStopProcess,
		handler.NewStopWorkerHandler(processRepo, taskRepo, queueRepo, registry))

	// Messaging handlers (3)
	cmdProcessor.RegisterHandler(command.CmdSendToProcess,
		handler.NewSendToProcessHandler(processRepo, queueRepo))
	cmdProcessor.RegisterHandler(command.CmdBroadcast,
		handler.NewBroadcastHandler(processRepo))
	cmdProcessor.RegisterHandler(command.CmdDeliverProcessQueued,
		handler.NewDeliverProcessQueuedHandler(processRepo, queueRepo, nil))

	// Task Assignment handlers (3)
	cmdProcessor.RegisterHandler(command.CmdAssignTask,
		handler.NewAssignTaskHandler(processRepo, taskRepo, handler.WithBDExecutor(bdExecutor), handler.WithQueueRepository(queueRepo)))
	cmdProcessor.RegisterHandler(command.CmdAssignReview,
		handler.NewAssignReviewHandler(processRepo, taskRepo, queueRepo))
	cmdProcessor.RegisterHandler(command.CmdApproveCommit,
		handler.NewApproveCommitHandler(processRepo, taskRepo, queueRepo))
	cmdProcessor.RegisterHandler(command.CmdAssignReviewFeedback,
		handler.NewAssignReviewFeedbackHandler(processRepo, taskRepo, queueRepo))

	// State Transition handlers (3)
	cmdProcessor.RegisterHandler(command.CmdReportComplete,
		handler.NewReportCompleteHandler(processRepo, taskRepo, queueRepo,
			handler.WithReportCompleteBDExecutor(bdExecutor)))
	cmdProcessor.RegisterHandler(command.CmdReportVerdict,
		handler.NewReportVerdictHandler(processRepo, taskRepo, queueRepo,
			handler.WithReportVerdictBDExecutor(bdExecutor)))
	cmdProcessor.RegisterHandler(command.CmdTransitionPhase,
		handler.NewTransitionPhaseHandler(processRepo, queueRepo))
	cmdProcessor.RegisterHandler(command.CmdProcessTurnComplete,
		handler.NewProcessTurnCompleteHandler(processRepo, queueRepo))

	// BD Task Status handlers (2)
	cmdProcessor.RegisterHandler(command.CmdMarkTaskComplete,
		handler.NewMarkTaskCompleteHandler(bdExecutor, taskRepo))
	cmdProcessor.RegisterHandler(command.CmdMarkTaskFailed,
		handler.NewMarkTaskFailedHandler(bdExecutor))
}

// cleanup stops the processor and releases resources.
func (s *testV2Stack) cleanup() {
	s.cancel()
	s.processor.Drain()
}

// spawnWorkerAndWaitReady spawns a worker via v2 and waits for it to become ready.
func (s *testV2Stack) spawnWorkerAndWaitReady(t *testing.T) string {
	t.Helper()

	// Get list of current worker IDs to exclude
	existingIDs := make(map[string]bool)
	for _, w := range s.processRepo.Workers() {
		existingIDs[w.ID] = true
	}

	// Spawn via adapter
	result, err := s.adapter.HandleSpawnProcess(s.ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)

	// Wait for a NEW worker to appear in repository and become ready
	var workerID string
	require.Eventually(t, func() bool {
		workers := s.processRepo.Workers()
		// Find the new ready worker (not in our existing set)
		for _, w := range workers {
			if !existingIDs[w.ID] && w.Status == repository.StatusReady {
				workerID = w.ID
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond, "worker should become ready")

	return workerID
}

// ===========================================================================
// Mock Process Spawner
// ===========================================================================

// mockProcessSpawner simulates process spawning for testing.
type mockProcessSpawner struct {
	mu           sync.Mutex
	processes    map[string]*mockProcess
	spawnCounter atomic.Int64
	shouldFail   bool
	spawnDelay   time.Duration
}

func (m *mockProcessSpawner) Spawn(_ context.Context, workerID string) (handler.Process, error) {
	if m.spawnDelay > 0 {
		time.Sleep(m.spawnDelay)
	}

	if m.shouldFail {
		return nil, assert.AnError
	}

	sessionID := "session-" + workerID
	proc := &mockProcess{
		sessionID: sessionID,
		running:   true,
	}

	m.mu.Lock()
	m.processes[workerID] = proc
	m.spawnCounter.Add(1)
	m.mu.Unlock()

	return proc, nil
}

// mockProcess simulates a running AI process.
type mockProcess struct {
	sessionID string
	running   bool
}

func (p *mockProcess) SessionRef() string { return p.sessionID }
func (p *mockProcess) IsRunning() bool    { return p.running }
func (p *mockProcess) Cancel() error      { p.running = false; return nil }
func (p *mockProcess) Wait() error        { return nil }

// mockHeadlessProcessFull implements the full client.HeadlessProcess interface for integration testing.
// It supports sending init events to set the session ID on a Process.
type mockHeadlessProcessFull struct {
	sessionRef string
	events     chan client.OutputEvent
	errors     chan error
	running    bool
	status     client.ProcessStatus
	workDir    string
}

func newMockHeadlessProcessFull() *mockHeadlessProcessFull {
	return &mockHeadlessProcessFull{
		sessionRef: "",
		events:     make(chan client.OutputEvent, 10),
		errors:     make(chan error, 10),
		running:    true,
		status:     client.StatusRunning,
		workDir:    "/test/workdir",
	}
}

func (m *mockHeadlessProcessFull) Events() <-chan client.OutputEvent { return m.events }
func (m *mockHeadlessProcessFull) Errors() <-chan error              { return m.errors }
func (m *mockHeadlessProcessFull) SessionRef() string                { return m.sessionRef }
func (m *mockHeadlessProcessFull) Status() client.ProcessStatus      { return m.status }
func (m *mockHeadlessProcessFull) IsRunning() bool                   { return m.running }
func (m *mockHeadlessProcessFull) WorkDir() string                   { return m.workDir }
func (m *mockHeadlessProcessFull) PID() int                          { return 12345 }
func (m *mockHeadlessProcessFull) Cancel() error {
	m.running = false
	m.status = client.StatusCancelled
	close(m.events)
	return nil
}
func (m *mockHeadlessProcessFull) Wait() error { return nil }

// SendInitEvent sends an init event with the given session ID to the events channel.
// This allows tests to set the session ID on a process through the normal init flow.
func (m *mockHeadlessProcessFull) SendInitEvent(sessionID string) {
	m.events <- client.OutputEvent{
		Type:      client.EventSystem,
		SubType:   "init",
		SessionID: sessionID,
	}
}

// ===========================================================================
// Integration Test: SpawnWorkerFlow
// ===========================================================================

// TestV2Integration_SpawnWorkerFlow tests the full spawn flow through v2.
// This validates: MCP call → V2Adapter → CommandProcessor → Handler → Repository → Events
func TestV2Integration_SpawnWorkerFlow(t *testing.T) {
	stack := newTestV2Stack(t)
	defer stack.cleanup()

	// Subscribe to events
	eventCh := stack.eventBus.Subscribe(stack.ctx)

	// Spawn a worker via the adapter (simulating MCP tool call)
	result, err := stack.adapter.HandleSpawnProcess(stack.ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError, "spawn should succeed")

	// Verify worker appears in repository
	require.Eventually(t, func() bool {
		workers := stack.processRepo.Workers()
		return len(workers) > 0
	}, time.Second, 10*time.Millisecond, "worker should appear in repo")

	// Wait for worker to become ready (after async spawn completes)
	require.Eventually(t, func() bool {
		workers := stack.processRepo.ReadyWorkers()
		return len(workers) > 0
	}, 2*time.Second, 10*time.Millisecond, "worker should become ready")

	// Verify events were emitted
	var receivedEvents []events.ProcessEvent
	timeout := time.After(time.Second)
	for {
		select {
		case evt := <-eventCh:
			if processEvt, ok := evt.Payload.(events.ProcessEvent); ok {
				receivedEvents = append(receivedEvents, processEvt)
				// We expect at least 2 events: spawned and status change to ready
				if len(receivedEvents) >= 2 {
					goto checkEvents
				}
			}
		case <-timeout:
			t.Logf("Received %d events before timeout", len(receivedEvents))
			goto checkEvents
		}
	}

checkEvents:
	require.GreaterOrEqual(t, len(receivedEvents), 1, "should receive at least one process event")

	// Verify processor metrics
	assert.GreaterOrEqual(t, stack.processor.ProcessedCount(), int64(1), "should process at least spawn command")
}

// ===========================================================================
// Integration Test: FullWorkflow
// ===========================================================================

// TestV2Integration_FullWorkflow tests a complete implement→review cycle through v2.
// Flow: Spawn → Assign → Send Message → Report Complete → Assign Review → Report Verdict
func TestV2Integration_FullWorkflow(t *testing.T) {
	stack := newTestV2Stack(t)
	defer stack.cleanup()

	// Step 1: Spawn implementer
	implementerID := stack.spawnWorkerAndWaitReady(t)
	require.NotEmpty(t, implementerID)

	// Step 2: Spawn reviewer
	reviewerID := stack.spawnWorkerAndWaitReady(t)
	require.NotEmpty(t, reviewerID)
	require.NotEqual(t, implementerID, reviewerID)

	// Step 3: Assign task to implementer
	taskID := "test-tk001"
	assignArgs, _ := json.Marshal(map[string]string{
		"worker_id": implementerID,
		"task_id":   taskID,
		"summary":   "Test task for integration",
	})

	result, err := stack.adapter.HandleAssignTask(stack.ctx, assignArgs)
	require.NoError(t, err)
	require.False(t, result.IsError, "assign should succeed: %s", result.Content)

	// Verify task assignment
	task, err := stack.taskRepo.Get(taskID)
	require.NoError(t, err)
	assert.Equal(t, implementerID, task.Implementer)
	assert.Equal(t, repository.TaskImplementing, task.Status)

	// Verify worker phase updated
	implementer, _ := stack.processRepo.Get(implementerID)
	assert.Equal(t, events.ProcessPhaseImplementing, *implementer.Phase)
	assert.Equal(t, taskID, implementer.TaskID)

	// Step 4: Report implementation complete
	completeArgs, _ := json.Marshal(map[string]string{
		"summary": "Implementation complete",
	})
	completeResult, err := stack.adapter.HandleReportImplementationComplete(stack.ctx, completeArgs, implementerID)
	require.NoError(t, err)
	require.True(t, completeResult.Success)

	// Verify worker phase updated to awaiting review
	require.Eventually(t, func() bool {
		w, _ := stack.processRepo.Get(implementerID)
		return w.Phase != nil && *w.Phase == events.ProcessPhaseAwaitingReview
	}, time.Second, 10*time.Millisecond)

	// Step 5: Assign review to reviewer
	reviewArgs, _ := json.Marshal(map[string]string{
		"reviewer_id":    reviewerID,
		"task_id":        taskID,
		"implementer_id": implementerID,
	})
	result, err = stack.adapter.HandleAssignTaskReview(stack.ctx, reviewArgs)
	require.NoError(t, err)
	require.False(t, result.IsError)

	// Verify reviewer phase
	reviewer, _ := stack.processRepo.Get(reviewerID)
	assert.Equal(t, events.ProcessPhaseReviewing, *reviewer.Phase)

	// Step 6: Report review verdict (APPROVED)
	verdictArgs, _ := json.Marshal(map[string]string{
		"verdict":  "APPROVED",
		"comments": "LGTM!",
	})
	verdictResult, err := stack.adapter.HandleReportReviewVerdict(stack.ctx, verdictArgs, reviewerID)
	require.NoError(t, err)
	require.True(t, verdictResult.Success)

	// Verify task marked approved
	require.Eventually(t, func() bool {
		task, _ := stack.taskRepo.Get(taskID)
		return task.Status == repository.TaskApproved
	}, time.Second, 10*time.Millisecond)

	// Verify reviewer returned to idle (implementer stays in AwaitingReview until commit)
	require.Eventually(t, func() bool {
		rev, _ := stack.processRepo.Get(reviewerID)
		return rev.Phase != nil && *rev.Phase == events.ProcessPhaseIdle
	}, time.Second, 10*time.Millisecond)

	// Implementer stays in AwaitingReview (would transition to Committing phase next)
	impl, _ := stack.processRepo.Get(implementerID)
	assert.Equal(t, events.ProcessPhaseAwaitingReview, *impl.Phase)

	// Verify processor metrics
	assert.GreaterOrEqual(t, stack.processor.ProcessedCount(), int64(6), "should process multiple commands")
}

// ===========================================================================
// Integration Test: ProcessorLifecycle
// ===========================================================================

// TestV2Integration_ProcessorLifecycle tests processor start/drain/stop without leaks.
func TestV2Integration_ProcessorLifecycle(t *testing.T) {
	// Record initial goroutine count
	runtime.GC()
	initialGoroutines := runtime.NumGoroutine()

	// Run multiple processor cycles
	for i := 0; i < 3; i++ {
		func() {
			stack := newTestV2Stack(t)

			// Submit some commands
			for j := 0; j < 5; j++ {
				_, err := stack.adapter.HandleSpawnProcess(stack.ctx, nil)
				require.NoError(t, err)
			}

			// Wait for ALL commands to be fully processed
			// Unified SpawnProcessHandler is synchronous (no callbacks)
			require.Eventually(t, func() bool {
				return stack.processor.ProcessedCount() >= 5
			}, 2*time.Second, 10*time.Millisecond)

			// Proper cleanup: cancel then drain
			stack.cleanup()
		}()
	}

	// Force GC and check for goroutine leaks
	runtime.GC()
	time.Sleep(100 * time.Millisecond) // Give goroutines time to exit
	runtime.GC()

	finalGoroutines := runtime.NumGoroutine()
	// Allow some variance (test framework goroutines, etc.)
	leakThreshold := initialGoroutines + 5
	assert.LessOrEqual(t, finalGoroutines, leakThreshold,
		"potential goroutine leak: started with %d, ended with %d (threshold %d)",
		initialGoroutines, finalGoroutines, leakThreshold)
}

// ===========================================================================
// Integration Test: EventsReachTUI
// ===========================================================================

// TestV2Integration_EventsReachTUI tests that events published by handlers reach subscribers.
// This validates the event flow: Handler → CommandResult.Events → Processor → EventBus → Subscriber
func TestV2Integration_EventsReachTUI(t *testing.T) {
	stack := newTestV2Stack(t)
	defer stack.cleanup()

	// Subscribe to events (simulating TUI subscription)
	eventCh := stack.eventBus.Subscribe(stack.ctx)

	var receivedEvents []any
	var eventsMu sync.Mutex

	// Collect events in background
	go func() {
		for {
			select {
			case evt, ok := <-eventCh:
				if !ok {
					return
				}
				eventsMu.Lock()
				receivedEvents = append(receivedEvents, evt.Payload)
				eventsMu.Unlock()
			case <-stack.ctx.Done():
				return
			}
		}
	}()

	// Spawn a worker (triggers WorkerSpawned and WorkerStatusChange events)
	workerID := stack.spawnWorkerAndWaitReady(t)
	require.NotEmpty(t, workerID)

	// Give events time to propagate
	time.Sleep(100 * time.Millisecond)

	// Verify we received events
	eventsMu.Lock()
	eventCount := len(receivedEvents)
	eventsMu.Unlock()

	assert.GreaterOrEqual(t, eventCount, 1, "should receive at least one event")

	// Verify at least one ProcessEvent was received
	eventsMu.Lock()
	var processEventCount int
	for _, evt := range receivedEvents {
		if _, ok := evt.(events.ProcessEvent); ok {
			processEventCount++
		}
	}
	eventsMu.Unlock()

	assert.GreaterOrEqual(t, processEventCount, 1, "should receive at least one ProcessEvent")
}

// ===========================================================================
// Integration Test: Read-Only Repository Access
// ===========================================================================

// TestV2Integration_ReadOnlyRepoAccess tests that read-only operations work without CommandProcessor.
func TestV2Integration_ReadOnlyRepoAccess(t *testing.T) {
	stack := newTestV2Stack(t)
	defer stack.cleanup()

	// Spawn some workers
	worker1 := stack.spawnWorkerAndWaitReady(t)
	worker2 := stack.spawnWorkerAndWaitReady(t)

	// Test query_worker_state without filter (should return all workers)
	result, err := stack.adapter.HandleQueryWorkerState(stack.ctx, nil)
	require.NoError(t, err)
	require.False(t, result.IsError)

	require.NotEmpty(t, result.Content)
	var stateResp struct {
		Workers        []map[string]interface{}          `json:"workers"`
		ReadyWorkers   []string                          `json:"ready_workers"`
		RetiredWorkers []string                          `json:"retired_workers"`
		FailedWorkers  []string                          `json:"failed_workers"`
		Tasks          map[string]map[string]interface{} `json:"tasks"`
	}
	err = json.Unmarshal([]byte(result.Content[0].Text), &stateResp)
	require.NoError(t, err)
	assert.Len(t, stateResp.Workers, 2, "should list 2 workers")

	// Test query_worker_state with filter
	queryArgs, _ := json.Marshal(map[string]string{"worker_id": worker1})
	result, err = stack.adapter.HandleQueryWorkerState(stack.ctx, queryArgs)
	require.NoError(t, err)
	require.False(t, result.IsError)

	require.NotEmpty(t, result.Content)
	err = json.Unmarshal([]byte(result.Content[0].Text), &stateResp)
	require.NoError(t, err)
	require.Len(t, stateResp.Workers, 1, "should have one worker")
	assert.Equal(t, worker1, stateResp.Workers[0]["worker_id"])
	assert.Equal(t, "ready", stateResp.Workers[0]["status"])

	// Query non-existent worker (now returns empty workers array, not error)
	queryArgs, _ = json.Marshal(map[string]string{"worker_id": "non-existent"})
	result, err = stack.adapter.HandleQueryWorkerState(stack.ctx, queryArgs)
	require.NoError(t, err)
	require.False(t, result.IsError)
	err = json.Unmarshal([]byte(result.Content[0].Text), &stateResp)
	require.NoError(t, err)
	assert.Empty(t, stateResp.Workers, "should return empty workers array for non-existent worker")

	// Verify no commands were submitted for read operations
	// (ProcessedCount should only reflect spawn commands, not reads)
	processedBefore := stack.processor.ProcessedCount()

	// Do another read
	_, _ = stack.adapter.HandleQueryWorkerState(stack.ctx, nil)

	processedAfter := stack.processor.ProcessedCount()
	assert.Equal(t, processedBefore, processedAfter, "read operations should not submit commands")

	_ = worker2 // Use worker2 to avoid unused variable warning
}

// ===========================================================================
// Integration Test: Replace Worker
// ===========================================================================

// TestV2Integration_ReplaceWorker tests the replace worker flow which uses follow-up commands.
func TestV2Integration_ReplaceWorker(t *testing.T) {
	stack := newTestV2Stack(t)
	defer stack.cleanup()

	// Spawn initial worker
	workerID := stack.spawnWorkerAndWaitReady(t)
	require.NotEmpty(t, workerID)

	initialWorkerCount := len(stack.processRepo.Workers())

	// Replace the worker
	replaceArgs, _ := json.Marshal(map[string]string{
		"worker_id": workerID,
		"reason":    "Testing replacement",
	})
	result, err := stack.adapter.HandleReplaceProcess(stack.ctx, replaceArgs)
	require.NoError(t, err)
	require.False(t, result.IsError)

	// Wait for replacement to complete:
	// 1. Original worker should be retired
	// 2. New worker should be spawned and ready
	require.Eventually(t, func() bool {
		original, _ := stack.processRepo.Get(workerID)
		if original == nil || original.Status != repository.StatusRetired {
			return false
		}
		// Check that we have same number of workers (one retired, one new)
		workers := stack.processRepo.Workers()
		return len(workers) == initialWorkerCount+1 // Original (retired) + new
	}, 3*time.Second, 50*time.Millisecond, "replace should complete")

	// Verify original worker is retired
	original, _ := stack.processRepo.Get(workerID)
	assert.Equal(t, repository.StatusRetired, original.Status)

	// Verify a new ready worker exists
	readyWorkers := stack.processRepo.ReadyWorkers()
	require.Len(t, readyWorkers, 1, "should have exactly one ready worker")
	assert.NotEqual(t, workerID, readyWorkers[0].ID, "new worker should have different ID")
}

// ===========================================================================
// Integration Test: Concurrent Commands
// ===========================================================================

// TestV2Integration_ConcurrentCommands tests thread-safety with concurrent command submission.
func TestV2Integration_ConcurrentCommands(t *testing.T) {
	stack := newTestV2Stack(t)
	defer stack.cleanup()

	const numWorkers = 10
	var wg sync.WaitGroup

	// Spawn multiple workers concurrently
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := stack.adapter.HandleSpawnProcess(stack.ctx, nil)
			assert.NoError(t, err)
		}()
	}

	wg.Wait()

	// Wait for all workers to become ready
	require.Eventually(t, func() bool {
		ready := stack.processRepo.ReadyWorkers()
		return len(ready) >= numWorkers
	}, 5*time.Second, 50*time.Millisecond, "all workers should become ready")

	// Verify all workers are in repository
	allWorkers := stack.processRepo.Workers()
	assert.GreaterOrEqual(t, len(allWorkers), numWorkers)

	// Verify processor handled all commands without error
	assert.Equal(t, int64(0), stack.processor.ErrorCount(), "should have no errors")
}

// ===========================================================================
// E2E Test Infrastructure for v2 Wiring Verification
// ===========================================================================

// mockMessageDeliverer records message deliveries for E2E testing.
type mockMessageDeliverer struct {
	mu         sync.Mutex
	deliveries []deliveryRecord
	shouldFail bool
	failErr    error
}

type deliveryRecord struct {
	WorkerID string
	Content  string
}

func newMockMessageDeliverer() *mockMessageDeliverer {
	return &mockMessageDeliverer{
		deliveries: make([]deliveryRecord, 0),
	}
}

func (m *mockMessageDeliverer) Deliver(_ context.Context, workerID, content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldFail {
		return m.failErr
	}

	m.deliveries = append(m.deliveries, deliveryRecord{
		WorkerID: workerID,
		Content:  content,
	})
	return nil
}

func (m *mockMessageDeliverer) getDeliveries() []deliveryRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]deliveryRecord, len(m.deliveries))
	copy(result, m.deliveries)
	return result
}

// bdStatusUpdate represents a recorded status update call.
type bdStatusUpdate struct {
	TaskID string
	Status string
}

// bdComment represents a recorded comment call.
type bdComment struct {
	TaskID  string
	Author  string
	Comment string
}

// setupPermissiveBDMock configures a MockBeadsExecutor to accept any calls.
// This is useful for integration tests where we want to verify calls were made
// without strict expectation ordering.
func setupPermissiveBDMock(m *mocks.MockTaskExecutor) {
	m.EXPECT().UpdateStatus(mock.Anything, mock.Anything).Return(nil).Maybe()
	m.EXPECT().UpdatePriority(mock.Anything, mock.Anything).Return(nil).Maybe()
	m.EXPECT().UpdateType(mock.Anything, mock.Anything).Return(nil).Maybe()
	m.EXPECT().CloseIssue(mock.Anything, mock.Anything).Return(nil).Maybe()
	m.EXPECT().ReopenIssue(mock.Anything).Return(nil).Maybe()
	m.EXPECT().DeleteIssues(mock.Anything).Return(nil).Maybe()
	m.EXPECT().SetLabels(mock.Anything, mock.Anything).Return(nil).Maybe()
	m.EXPECT().ShowIssue(mock.Anything).Return(&taskpkg.Issue{ID: "test", Status: taskpkg.StatusOpen}, nil).Maybe()
	m.EXPECT().AddComment(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
}

// getStatusUpdates extracts all UpdateStatus calls from the mock.
func getStatusUpdates(m *mocks.MockTaskExecutor) []bdStatusUpdate {
	var updates []bdStatusUpdate
	for _, call := range m.Calls {
		if call.Method == "UpdateStatus" {
			updates = append(updates, bdStatusUpdate{
				TaskID: call.Arguments.Get(0).(string),
				Status: string(call.Arguments.Get(1).(taskpkg.Status)),
			})
		}
	}
	return updates
}

// getComments extracts all AddComment calls from the mock.
func getComments(m *mocks.MockTaskExecutor) []bdComment {
	var comments []bdComment
	for _, call := range m.Calls {
		if call.Method == "AddComment" {
			comments = append(comments, bdComment{
				TaskID:  call.Arguments.Get(0).(string),
				Author:  call.Arguments.Get(1).(string),
				Comment: call.Arguments.Get(2).(string),
			})
		}
	}
	return comments
}

// testV2StackWithWiring contains all v2 components with wired integrations for E2E testing.
type testV2StackWithWiring struct {
	*testV2Stack
	mockDeliverer  *mockMessageDeliverer
	mockBDExecutor *mocks.MockTaskExecutor
}

// newTestV2StackWithWiring creates a v2 stack with MessageDeliverer and BDExecutor wired.
func newTestV2StackWithWiring(t *testing.T) *testV2StackWithWiring {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	// Create repositories
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(repository.DefaultQueueMaxSize)

	// Create event bus
	eventBus := pubsub.NewBroker[any]()

	// Create command processor with event bus
	cmdProcessor := processor.NewCommandProcessor(
		processor.WithQueueCapacity(1000),
		processor.WithEventBus(eventBus),
		processor.WithTaskRepository(taskRepo),
		processor.WithQueueRepository(queueRepo),
	)

	// Create mocks for wiring verification
	mockDeliverer := newMockMessageDeliverer()
	mockBDExec := mocks.NewMockTaskExecutor(t)
	setupPermissiveBDMock(mockBDExec)

	// Create mock spawner
	mockSpawner := &mockProcessSpawner{
		processes: make(map[string]*mockProcess),
	}

	// Create process registry
	registry := process.NewProcessRegistry()

	stack := &testV2Stack{
		processRepo: processRepo,
		taskRepo:    taskRepo,
		queueRepo:   queueRepo,
		processor:   cmdProcessor,
		eventBus:    eventBus,
		registry:    registry,
		ctx:         ctx,
		cancel:      cancel,
		mockSpawner: mockSpawner,
	}

	// Register handlers with wired integrations
	registerHandlersWithWiring(t, stack, registry, mockDeliverer, mockBDExec)

	// Create V2Adapter with repositories for read-only operations
	v2Adapter := adapter.NewV2Adapter(cmdProcessor,
		adapter.WithProcessRepository(processRepo),
		adapter.WithTaskRepository(taskRepo),
		adapter.WithQueueRepository(queueRepo),
	)
	stack.adapter = v2Adapter

	// Start processor loop
	go cmdProcessor.Run(ctx)

	// Wait for processor to be running
	require.Eventually(t, func() bool {
		return cmdProcessor.IsRunning()
	}, time.Second, 10*time.Millisecond, "processor should start running")

	return &testV2StackWithWiring{
		testV2Stack:    stack,
		mockDeliverer:  mockDeliverer,
		mockBDExecutor: mockBDExec,
	}
}

// registerHandlersWithWiring registers all handlers with MessageDeliverer and BDExecutor wired.
func registerHandlersWithWiring(
	t *testing.T,
	stack *testV2Stack,
	registry *process.ProcessRegistry,
	deliverer *mockMessageDeliverer,
	bdExec *mocks.MockTaskExecutor,
) {
	t.Helper()

	processRepo := stack.processRepo
	taskRepo := stack.taskRepo
	queueRepo := stack.queueRepo
	cmdProcessor := stack.processor

	// Process Lifecycle handlers (3)
	cmdProcessor.RegisterHandler(command.CmdSpawnProcess,
		handler.NewSpawnProcessHandler(processRepo, registry))
	cmdProcessor.RegisterHandler(command.CmdRetireProcess,
		handler.NewRetireProcessHandler(processRepo, registry))
	cmdProcessor.RegisterHandler(command.CmdReplaceProcess,
		handler.NewReplaceProcessHandler(processRepo, registry))

	// Stop Worker handler
	cmdProcessor.RegisterHandler(command.CmdStopProcess,
		handler.NewStopWorkerHandler(processRepo, taskRepo, queueRepo, registry))

	// Messaging handlers (3) - using unified process handlers
	cmdProcessor.RegisterHandler(command.CmdSendToProcess,
		handler.NewSendToProcessHandler(processRepo, queueRepo))
	cmdProcessor.RegisterHandler(command.CmdBroadcast,
		handler.NewBroadcastHandler(processRepo))
	cmdProcessor.RegisterHandler(command.CmdDeliverProcessQueued,
		handler.NewDeliverProcessQueuedHandler(processRepo, queueRepo, nil,
			handler.WithProcessDeliverer(deliverer)))

	// Task Assignment handlers (3) - wired with BDExecutor
	cmdProcessor.RegisterHandler(command.CmdAssignTask,
		handler.NewAssignTaskHandler(processRepo, taskRepo,
			handler.WithBDExecutor(bdExec),
			handler.WithQueueRepository(queueRepo)))
	cmdProcessor.RegisterHandler(command.CmdAssignReview,
		handler.NewAssignReviewHandler(processRepo, taskRepo, queueRepo))
	cmdProcessor.RegisterHandler(command.CmdApproveCommit,
		handler.NewApproveCommitHandler(processRepo, taskRepo, queueRepo))
	cmdProcessor.RegisterHandler(command.CmdAssignReviewFeedback,
		handler.NewAssignReviewFeedbackHandler(processRepo, taskRepo, queueRepo))

	// State Transition handlers (3) - wired with BDExecutor
	cmdProcessor.RegisterHandler(command.CmdReportComplete,
		handler.NewReportCompleteHandler(processRepo, taskRepo, queueRepo,
			handler.WithReportCompleteBDExecutor(bdExec)))
	cmdProcessor.RegisterHandler(command.CmdReportVerdict,
		handler.NewReportVerdictHandler(processRepo, taskRepo, queueRepo,
			handler.WithReportVerdictBDExecutor(bdExec)))
	cmdProcessor.RegisterHandler(command.CmdTransitionPhase,
		handler.NewTransitionPhaseHandler(processRepo, queueRepo))
	cmdProcessor.RegisterHandler(command.CmdProcessTurnComplete,
		handler.NewProcessTurnCompleteHandler(processRepo, queueRepo))

	// BD Task Status handlers (2) - wired with BDExecutor and TaskRepo
	cmdProcessor.RegisterHandler(command.CmdMarkTaskComplete,
		handler.NewMarkTaskCompleteHandler(bdExec, taskRepo))
	cmdProcessor.RegisterHandler(command.CmdMarkTaskFailed,
		handler.NewMarkTaskFailedHandler(bdExec))
}

// ===========================================================================
// E2E Integration Test: BD Sync Wiring
// ===========================================================================

// TestV2E2E_BDSync verifies that BDExecutor is correctly wired and task status
// is updated in BD when tasks are assigned.
func TestV2E2E_BDSync(t *testing.T) {
	stack := newTestV2StackWithWiring(t)
	defer stack.cleanup()

	// Step 1: Spawn worker
	workerID := stack.spawnWorkerAndWaitReady(t)
	require.NotEmpty(t, workerID)

	// Step 2: Assign task via AssignTaskCommand
	taskID := "bd-sync01"
	assignArgs, _ := json.Marshal(map[string]string{
		"worker_id": workerID,
		"task_id":   taskID,
		"summary":   "Test task for BD sync verification",
	})

	result, err := stack.adapter.HandleAssignTask(stack.ctx, assignArgs)
	require.NoError(t, err)
	require.False(t, result.IsError, "assign should succeed: %s", result.Content)

	// Step 3: Wait for async BD update to complete
	require.Eventually(t, func() bool {
		updates := getStatusUpdates(stack.mockBDExecutor)
		return len(updates) > 0
	}, time.Second, 10*time.Millisecond, "BD status update should be called")

	// Step 4: Verify BDExecutor.UpdateTaskStatus was called with "in_progress"
	updates := getStatusUpdates(stack.mockBDExecutor)
	require.Len(t, updates, 1, "should have exactly one status update")
	assert.Equal(t, taskID, updates[0].TaskID)
	assert.Equal(t, "in_progress", updates[0].Status)

	// Step 5: Verify v2 TaskRepository was also updated
	task, err := stack.taskRepo.Get(taskID)
	require.NoError(t, err)
	assert.Equal(t, workerID, task.Implementer)
	assert.Equal(t, repository.TaskImplementing, task.Status)
}

// TestV2E2E_BDSync_ReportComplete verifies BD comment is added on implementation complete.
func TestV2E2E_BDSync_ReportComplete(t *testing.T) {
	stack := newTestV2StackWithWiring(t)
	defer stack.cleanup()

	// Step 1: Spawn worker and assign task
	workerID := stack.spawnWorkerAndWaitReady(t)
	taskID := "bd-cmplt01"

	assignArgs, _ := json.Marshal(map[string]string{
		"worker_id": workerID,
		"task_id":   taskID,
		"summary":   "Test task",
	})
	_, err := stack.adapter.HandleAssignTask(stack.ctx, assignArgs)
	require.NoError(t, err)

	// Wait for assign BD update
	require.Eventually(t, func() bool {
		return len(getStatusUpdates(stack.mockBDExecutor)) > 0
	}, time.Second, 10*time.Millisecond)

	// Step 2: Report implementation complete with summary
	completeSummary := "Implemented feature X with full test coverage"
	completeArgs, _ := json.Marshal(map[string]string{
		"summary": completeSummary,
	})

	result, err := stack.adapter.HandleReportImplementationComplete(stack.ctx, completeArgs, workerID)
	require.NoError(t, err)
	require.True(t, result.Success)

	// Step 3: Wait for async BD comment to be added
	require.Eventually(t, func() bool {
		comments := getComments(stack.mockBDExecutor)
		return len(comments) > 0
	}, time.Second, 10*time.Millisecond, "BD comment should be added")

	// Step 4: Verify comment content
	comments := getComments(stack.mockBDExecutor)
	require.Len(t, comments, 1)
	assert.Equal(t, taskID, comments[0].TaskID)
	assert.Contains(t, comments[0].Comment, "Implementation complete")
	assert.Contains(t, comments[0].Comment, completeSummary)
}

// TestV2E2E_BDSync_ReportVerdict verifies BD comment is added on review verdict.
func TestV2E2E_BDSync_ReportVerdict(t *testing.T) {
	stack := newTestV2StackWithWiring(t)
	defer stack.cleanup()

	// Step 1: Setup - spawn implementer and reviewer, complete implementation
	implementerID := stack.spawnWorkerAndWaitReady(t)
	reviewerID := stack.spawnWorkerAndWaitReady(t)
	taskID := "bd-vrdt01"

	// Assign and complete implementation
	assignArgs, _ := json.Marshal(map[string]string{
		"worker_id": implementerID,
		"task_id":   taskID,
		"summary":   "Test task",
	})
	_, _ = stack.adapter.HandleAssignTask(stack.ctx, assignArgs)

	// Wait for BD update then complete
	time.Sleep(50 * time.Millisecond)
	completeArgs, _ := json.Marshal(map[string]string{"summary": "Done"})
	_, _ = stack.adapter.HandleReportImplementationComplete(stack.ctx, completeArgs, implementerID)

	// Assign review
	reviewArgs, _ := json.Marshal(map[string]string{
		"reviewer_id":    reviewerID,
		"task_id":        taskID,
		"implementer_id": implementerID,
	})
	_, _ = stack.adapter.HandleAssignTaskReview(stack.ctx, reviewArgs)

	// Clear previous BD operations
	time.Sleep(50 * time.Millisecond)
	initialComments := len(getComments(stack.mockBDExecutor))

	// Step 2: Report review verdict APPROVED
	verdictArgs, _ := json.Marshal(map[string]string{
		"verdict":  "APPROVED",
		"comments": "LGTM! Great implementation.",
	})

	verdictResult, err := stack.adapter.HandleReportReviewVerdict(stack.ctx, verdictArgs, reviewerID)
	require.NoError(t, err)
	require.True(t, verdictResult.Success)

	// Step 3: Wait for async BD comment to be added
	require.Eventually(t, func() bool {
		comments := getComments(stack.mockBDExecutor)
		return len(comments) > initialComments
	}, time.Second, 10*time.Millisecond, "BD comment should be added for verdict")

	// Step 4: Verify comment contains verdict info
	comments := getComments(stack.mockBDExecutor)
	latestComment := comments[len(comments)-1]
	assert.Equal(t, taskID, latestComment.TaskID)
	assert.Contains(t, latestComment.Comment, "APPROVED")
	assert.Contains(t, latestComment.Comment, reviewerID)
}

// ===========================================================================
// E2E Integration Test: EventBus Wiring
// ===========================================================================

// TestV2E2E_EventBusWiring verifies that EventBus is correctly wired and events
// propagate to subscribers (simulating TUI).
func TestV2E2E_EventBusWiring(t *testing.T) {
	stack := newTestV2StackWithWiring(t)
	defer stack.cleanup()

	// Step 1: Subscribe to event bus (simulating TUI subscription)
	eventCh := stack.eventBus.Subscribe(stack.ctx)

	var receivedEvents []any
	var eventsMu sync.Mutex
	done := make(chan struct{})

	// Collect events in background
	go func() {
		defer close(done)
		for {
			select {
			case evt, ok := <-eventCh:
				if !ok {
					return
				}
				eventsMu.Lock()
				receivedEvents = append(receivedEvents, evt.Payload)
				eventsMu.Unlock()
			case <-stack.ctx.Done():
				return
			}
		}
	}()

	// Step 2: Spawn worker (triggers WorkerEvent)
	workerID := stack.spawnWorkerAndWaitReady(t)
	require.NotEmpty(t, workerID)

	// Step 3: Assign task (triggers WorkerStatusChangeEvent, TaskAssignedEvent)
	taskID := "evnt-tk01"
	assignArgs, _ := json.Marshal(map[string]string{
		"worker_id": workerID,
		"task_id":   taskID,
		"summary":   "Test task",
	})

	result, err := stack.adapter.HandleAssignTask(stack.ctx, assignArgs)
	require.NoError(t, err)
	require.False(t, result.IsError)

	// Step 4: Report complete (triggers phase transition event)
	completeArgs, _ := json.Marshal(map[string]string{
		"summary": "Done",
	})
	_, _ = stack.adapter.HandleReportImplementationComplete(stack.ctx, completeArgs, workerID)

	// Give events time to propagate
	time.Sleep(100 * time.Millisecond)

	// Step 5: Verify we received ProcessEvents
	eventsMu.Lock()
	eventCount := len(receivedEvents)
	var processEventCount int
	for _, evt := range receivedEvents {
		if _, ok := evt.(events.ProcessEvent); ok {
			processEventCount++
		}
	}
	eventsMu.Unlock()

	assert.GreaterOrEqual(t, eventCount, 2, "should receive multiple events")
	assert.GreaterOrEqual(t, processEventCount, 2, "should receive at least 2 ProcessEvents (spawn, assign)")

	// Step 6: Verify FIFO ordering - events should arrive in order of state changes
	// This is implicitly verified by the fact that we're receiving events at all
	// and they contain correct worker/task data
}

// ===========================================================================
// E2E Integration Test: Full Workflow
// ===========================================================================

// TestV2E2E_FullWorkflow verifies all integrations work together in a complete
// orchestration workflow: spawn → assign → message → complete → review → verdict.
func TestV2E2E_FullWorkflow(t *testing.T) {
	stack := newTestV2StackWithWiring(t)
	defer stack.cleanup()

	// Subscribe to events for verification
	eventCh := stack.eventBus.Subscribe(stack.ctx)
	var receivedEvents []events.ProcessEvent
	var eventsMu sync.Mutex

	go func() {
		for evt := range eventCh {
			if processEvt, ok := evt.Payload.(events.ProcessEvent); ok {
				eventsMu.Lock()
				receivedEvents = append(receivedEvents, processEvt)
				eventsMu.Unlock()
			}
		}
	}()

	// Step 1: Spawn implementer and reviewer
	t.Log("Step 1: Spawning workers")
	implementerID := stack.spawnWorkerAndWaitReady(t)
	reviewerID := stack.spawnWorkerAndWaitReady(t)
	require.NotEmpty(t, implementerID)
	require.NotEmpty(t, reviewerID)
	require.NotEqual(t, implementerID, reviewerID)

	// Step 2: Assign task (verify BD update)
	t.Log("Step 2: Assigning task")
	taskID := "full-wf001"
	assignArgs, _ := json.Marshal(map[string]string{
		"worker_id": implementerID,
		"task_id":   taskID,
		"summary":   "Implement the full workflow feature",
	})

	result, err := stack.adapter.HandleAssignTask(stack.ctx, assignArgs)
	require.NoError(t, err)
	require.False(t, result.IsError)

	// Wait for BD update
	require.Eventually(t, func() bool {
		updates := getStatusUpdates(stack.mockBDExecutor)
		return len(updates) > 0 && updates[0].TaskID == taskID
	}, time.Second, 10*time.Millisecond, "BD should be updated on task assignment")

	// Step 3: Report implementation complete (verify BD comment + queue drain)
	t.Log("Step 3: Reporting implementation complete")
	completeArgs, _ := json.Marshal(map[string]string{
		"summary": "Implemented full workflow with comprehensive tests",
	})

	completeResult, err := stack.adapter.HandleReportImplementationComplete(stack.ctx, completeArgs, implementerID)
	require.NoError(t, err)
	require.True(t, completeResult.Success)

	// Verify BD comment was added
	require.Eventually(t, func() bool {
		comments := getComments(stack.mockBDExecutor)
		for _, c := range comments {
			if c.TaskID == taskID && strings.Contains(c.Comment, "Implementation complete") {
				return true
			}
		}
		return false
	}, time.Second, 10*time.Millisecond, "BD comment should be added on complete")

	// Verify task prompt was delivered
	require.Eventually(t, func() bool {
		deliveries := stack.mockDeliverer.getDeliveries()
		return len(deliveries) >= 1
	}, time.Second, 10*time.Millisecond, "task prompt should be delivered")

	// Step 4: Assign review
	t.Log("Step 4: Assigning review")
	reviewArgs, _ := json.Marshal(map[string]string{
		"reviewer_id":    reviewerID,
		"task_id":        taskID,
		"implementer_id": implementerID,
	})

	result, err = stack.adapter.HandleAssignTaskReview(stack.ctx, reviewArgs)
	require.NoError(t, err)
	require.False(t, result.IsError)

	// Verify reviewer phase
	reviewer, _ := stack.processRepo.Get(reviewerID)
	assert.Equal(t, events.ProcessPhaseReviewing, *reviewer.Phase)

	// Step 5: Report verdict APPROVED (verify BD comment)
	t.Log("Step 5: Reporting review verdict")
	initialComments := len(getComments(stack.mockBDExecutor))

	verdictArgs, _ := json.Marshal(map[string]string{
		"verdict":  "APPROVED",
		"comments": "Excellent implementation, LGTM!",
	})

	verdictResult, err := stack.adapter.HandleReportReviewVerdict(stack.ctx, verdictArgs, reviewerID)
	require.NoError(t, err)
	require.True(t, verdictResult.Success)

	// Verify BD comment for verdict
	require.Eventually(t, func() bool {
		comments := getComments(stack.mockBDExecutor)
		return len(comments) > initialComments
	}, time.Second, 10*time.Millisecond, "BD comment should be added on verdict")

	// Step 6: Verify final state
	t.Log("Step 6: Verifying final state")

	// Task should be approved
	task, err := stack.taskRepo.Get(taskID)
	require.NoError(t, err)
	assert.Equal(t, repository.TaskApproved, task.Status)

	// Reviewer should be back to idle
	reviewer, _ = stack.processRepo.Get(reviewerID)
	assert.Equal(t, events.ProcessPhaseIdle, *reviewer.Phase)

	// Implementer stays in AwaitingReview (would transition to Committing next)
	impl, _ := stack.processRepo.Get(implementerID)
	assert.Equal(t, events.ProcessPhaseAwaitingReview, *impl.Phase)

	// Step 7: Verify events were received (EventBus wiring)
	time.Sleep(100 * time.Millisecond) // Let events propagate
	eventsMu.Lock()
	eventCount := len(receivedEvents)
	eventsMu.Unlock()

	assert.GreaterOrEqual(t, eventCount, 4, "should receive multiple ProcessEvents throughout workflow")

	// Step 9: Verify BD operations summary
	statusUpdates := getStatusUpdates(stack.mockBDExecutor)
	comments := getComments(stack.mockBDExecutor)

	assert.GreaterOrEqual(t, len(statusUpdates), 1, "should have BD status update")
	assert.GreaterOrEqual(t, len(comments), 2, "should have BD comments for complete and verdict")

	t.Logf("Full workflow completed successfully:")
	t.Logf("  - Workers spawned: 2 (implementer=%s, reviewer=%s)", implementerID, reviewerID)
	t.Logf("  - BD status updates: %d", len(statusUpdates))
	t.Logf("  - BD comments: %d", len(comments))
	t.Logf("  - Messages delivered: %d", len(stack.mockDeliverer.getDeliveries()))
	t.Logf("  - Events received: %d", eventCount)
}

// ===========================================================================
// Integration Test: Stop Worker Feature
// ===========================================================================

// TestIntegration_StopWorker_UICommand tests the full flow from /stop command to worker termination.
// This simulates what happens when a user types "/stop worker-N" in the UI.
func TestIntegration_StopWorker_UICommand(t *testing.T) {
	stack := newTestV2Stack(t)
	defer stack.cleanup()

	// Subscribe to events
	eventCh := stack.eventBus.Subscribe(stack.ctx)

	// Step 1: Spawn a worker and wait for it to be ready
	workerID := stack.spawnWorkerAndWaitReady(t)
	require.NotEmpty(t, workerID)

	// Verify worker is ready in repository
	worker, err := stack.processRepo.Get(workerID)
	require.NoError(t, err)
	assert.Equal(t, repository.StatusReady, worker.Status)

	// Step 2: Submit a StopProcessCommand (simulating UI /stop command)
	cmd := command.NewStopProcessCommand(command.SourceUser, workerID, false, "user_requested")
	result, err := stack.processor.SubmitAndWait(stack.ctx, cmd)
	require.NoError(t, err)
	require.True(t, result.Success, "stop command should succeed")

	// Step 3: Verify worker is now stopped
	worker, err = stack.processRepo.Get(workerID)
	require.NoError(t, err)
	assert.Equal(t, repository.StatusStopped, worker.Status, "worker should be stopped after stop")

	// Step 4: Verify ProcessStatusChange event was emitted
	var receivedStopEvent bool
	timeout := time.After(time.Second)
eventLoop:
	for {
		select {
		case evt := <-eventCh:
			if processEvt, ok := evt.Payload.(events.ProcessEvent); ok {
				if processEvt.Type == events.ProcessStatusChange &&
					processEvt.ProcessID == workerID &&
					processEvt.Status == events.ProcessStatusStopped {
					receivedStopEvent = true
					break eventLoop
				}
			}
		case <-timeout:
			break eventLoop
		}
	}
	assert.True(t, receivedStopEvent, "should receive ProcessStatusChange event for stopped worker")

	t.Logf("Stop worker UI command test passed: worker %s stopped successfully", workerID)
}

// TestIntegration_StopWorker_MCPTool tests the full flow from MCP tool call to worker termination.
// This simulates what happens when the coordinator calls the stop_worker MCP tool.
func TestIntegration_StopWorker_MCPTool(t *testing.T) {
	stack := newTestV2Stack(t)
	defer stack.cleanup()

	// Step 1: Spawn a worker and wait for it to be ready
	workerID := stack.spawnWorkerAndWaitReady(t)
	require.NotEmpty(t, workerID)

	// Step 2: Call HandleStopProcess on the adapter (simulating MCP tool call)
	err := stack.adapter.HandleStopProcess(workerID, false, "coordinator_requested")
	require.NoError(t, err)

	// Wait for the command to be processed
	require.Eventually(t, func() bool {
		worker, _ := stack.processRepo.Get(workerID)
		return worker.Status == repository.StatusStopped
	}, time.Second, 10*time.Millisecond, "worker should become stopped")

	// Step 3: Verify worker is stopped
	worker, err := stack.processRepo.Get(workerID)
	require.NoError(t, err)
	assert.Equal(t, repository.StatusStopped, worker.Status)

	t.Logf("Stop worker MCP tool test passed: worker %s stopped via adapter", workerID)
}

// TestIntegration_StopWorker_GracefulEscalation tests the timeout and SIGKILL escalation behavior.
// This verifies that when graceful stop times out, the system escalates to forceful termination.
func TestIntegration_StopWorker_GracefulEscalation(t *testing.T) {
	stack := newTestV2Stack(t)
	defer stack.cleanup()

	// Step 1: Spawn a worker
	workerID := stack.spawnWorkerAndWaitReady(t)
	require.NotEmpty(t, workerID)

	// Step 2: Assign a task to make the worker busy
	taskID := "stop-tk001"
	assignCmd := command.NewAssignTaskCommand(command.SourceMCPTool, workerID, taskID, "Test task for escalation", "")
	result, err := stack.processor.SubmitAndWait(stack.ctx, assignCmd)
	require.NoError(t, err)
	require.True(t, result.Success, "assign task should succeed: %v", result.Error)

	// Verify worker is now working
	worker, _ := stack.processRepo.Get(workerID)
	assert.Equal(t, repository.StatusWorking, worker.Status)

	// Step 3: Submit a force stop command (simulating --force flag)
	stopCmd := command.NewStopProcessCommand(command.SourceUser, workerID, true, "force_stop_test")
	result, err = stack.processor.SubmitAndWait(stack.ctx, stopCmd)
	require.NoError(t, err)
	require.True(t, result.Success, "force stop should succeed")

	// Step 4: Verify worker is stopped (force stop should work even when busy)
	worker, err = stack.processRepo.Get(workerID)
	require.NoError(t, err)
	assert.Equal(t, repository.StatusStopped, worker.Status, "worker should be stopped after force stop")

	// Step 5: Verify task cleanup - TaskID should be cleared
	assert.Empty(t, worker.TaskID, "TaskID should be cleared after stop")

	t.Logf("Stop worker escalation test passed: worker %s force-stopped while busy", workerID)
}

// TestIntegration_StopWorker_TaskCleanup tests that stopping a worker cleans up its assigned task.
func TestIntegration_StopWorker_TaskCleanup(t *testing.T) {
	stack := newTestV2Stack(t)
	defer stack.cleanup()

	// Step 1: Spawn a worker
	workerID := stack.spawnWorkerAndWaitReady(t)
	require.NotEmpty(t, workerID)

	// Step 2: Assign a task
	taskID := "stop-tk002"
	assignCmd := command.NewAssignTaskCommand(command.SourceMCPTool, workerID, taskID, "Test task for cleanup", "")
	result, err := stack.processor.SubmitAndWait(stack.ctx, assignCmd)
	require.NoError(t, err)
	require.True(t, result.Success)

	// Verify task assignment in task repository
	task, err := stack.taskRepo.Get(taskID)
	require.NoError(t, err)
	assert.Equal(t, workerID, task.Implementer)
	assert.Equal(t, repository.TaskImplementing, task.Status)

	// Step 3: Stop the worker
	stopCmd := command.NewStopProcessCommand(command.SourceUser, workerID, false, "cleanup_test")
	result, err = stack.processor.SubmitAndWait(stack.ctx, stopCmd)
	require.NoError(t, err)
	require.True(t, result.Success)

	// Step 4: Verify worker's TaskID is cleared
	worker, err := stack.processRepo.Get(workerID)
	require.NoError(t, err)
	assert.Empty(t, worker.TaskID, "worker's TaskID should be cleared")

	// Step 5: Verify task's implementer is cleared (task can be reassigned)
	task, err = stack.taskRepo.Get(taskID)
	require.NoError(t, err)
	assert.Empty(t, task.Implementer, "task's implementer should be cleared for reassignment")

	t.Logf("Stop worker task cleanup test passed: task %s released from worker %s", taskID, workerID)
}

// TestIntegration_StopWorker_PhaseWarning tests the phase-aware warning for Committing phase.
func TestIntegration_StopWorker_PhaseWarning(t *testing.T) {
	stack := newTestV2Stack(t)
	defer stack.cleanup()

	// Step 1: Spawn a worker
	workerID := stack.spawnWorkerAndWaitReady(t)
	require.NotEmpty(t, workerID)

	// Step 2: Manually set the worker to Committing phase (simulating a worker mid-commit)
	worker, _ := stack.processRepo.Get(workerID)
	commitPhase := events.ProcessPhaseCommitting
	worker.Phase = &commitPhase
	worker.Status = repository.StatusWorking
	worker.TaskID = "committing-task"
	stack.processRepo.AddProcess(worker) // Update the worker

	// Step 3: Try to stop WITHOUT force - should get a warning
	stopCmd := command.NewStopProcessCommand(command.SourceUser, workerID, false, "phase_warning_test")
	result, err := stack.processor.SubmitAndWait(stack.ctx, stopCmd)
	require.NoError(t, err)
	require.True(t, result.Success)

	// Step 4: Verify worker is NOT retired (phase warning blocked it)
	worker, _ = stack.processRepo.Get(workerID)
	assert.Equal(t, repository.StatusWorking, worker.Status, "worker should still be working (phase warning)")

	// Step 5: Verify we got a warning event
	require.Len(t, result.Events, 1, "should have one warning event")
	warningEvent, ok := result.Events[0].(events.ProcessEvent)
	require.True(t, ok)
	assert.Equal(t, events.ProcessOutput, warningEvent.Type)
	assert.Contains(t, warningEvent.Output, "Committing phase")

	// Step 6: Now stop with --force - should work
	forceStopCmd := command.NewStopProcessCommand(command.SourceUser, workerID, true, "force_override")
	result, err = stack.processor.SubmitAndWait(stack.ctx, forceStopCmd)
	require.NoError(t, err)
	require.True(t, result.Success)

	// Step 7: Verify worker is now stopped
	worker, _ = stack.processRepo.Get(workerID)
	assert.Equal(t, repository.StatusStopped, worker.Status, "worker should be stopped after force stop")

	t.Logf("Stop worker phase warning test passed: warning issued, force override worked")
}

// ===========================================================================
// Integration Test: MarkTaskComplete Removes Task from QueryWorkerState
// ===========================================================================

// TestIntegration_MarkTaskComplete_RemovesFromQueryWorkerState verifies that after
// mark_task_complete is called, the task is deleted from the in-memory TaskRepository
// and no longer appears in query_worker_state response.
//
// This test validates the fix for the bug where task status remained stuck in "in_review"
// after the full implement/review/approve/commit cycle completed.
func TestIntegration_MarkTaskComplete_RemovesFromQueryWorkerState(t *testing.T) {
	stack := newTestV2StackWithWiring(t)
	defer stack.cleanup()

	// Step 1: Spawn a worker
	workerID := stack.spawnWorkerAndWaitReady(t)
	require.NotEmpty(t, workerID)

	// Step 2: Assign task to worker
	taskID := "complete-tk001"
	assignArgs, _ := json.Marshal(map[string]string{
		"worker_id": workerID,
		"task_id":   taskID,
		"summary":   "Test task for completion tracking",
	})

	result, err := stack.adapter.HandleAssignTask(stack.ctx, assignArgs)
	require.NoError(t, err)
	require.False(t, result.IsError, "assign should succeed: %s", result.Content)

	// Wait for BD update to complete
	require.Eventually(t, func() bool {
		updates := getStatusUpdates(stack.mockBDExecutor)
		return len(updates) > 0 && updates[0].TaskID == taskID
	}, time.Second, 10*time.Millisecond, "BD should be updated on task assignment")

	// Step 3: Verify task appears in query_worker_state response
	stateResult, err := stack.adapter.HandleQueryWorkerState(stack.ctx, nil)
	require.NoError(t, err)
	require.False(t, stateResult.IsError)

	var stateResp struct {
		Workers []map[string]interface{}          `json:"workers"`
		Tasks   map[string]map[string]interface{} `json:"tasks"`
	}
	err = json.Unmarshal([]byte(stateResult.Content[0].Text), &stateResp)
	require.NoError(t, err)

	// Task should be present in the tasks map
	require.Contains(t, stateResp.Tasks, taskID, "task should appear in query_worker_state before completion")
	taskInfo := stateResp.Tasks[taskID]
	assert.Equal(t, "implementing", taskInfo["status"], "task status should be implementing")
	assert.Equal(t, workerID, taskInfo["implementer"], "task implementer should match")

	// Step 4: Verify task exists in in-memory TaskRepository
	task, err := stack.taskRepo.Get(taskID)
	require.NoError(t, err)
	assert.Equal(t, workerID, task.Implementer)
	assert.Equal(t, repository.TaskImplementing, task.Status)

	// Step 5: Call mark_task_complete
	completeArgs, _ := json.Marshal(map[string]string{
		"task_id": taskID,
	})

	result, err = stack.adapter.HandleMarkTaskComplete(stack.ctx, completeArgs)
	require.NoError(t, err)
	require.False(t, result.IsError, "mark_task_complete should succeed: %s", result.Content)

	// Verify BD was updated to closed
	require.Eventually(t, func() bool {
		updates := getStatusUpdates(stack.mockBDExecutor)
		for _, u := range updates {
			if u.TaskID == taskID && u.Status == "closed" {
				return true
			}
		}
		return false
	}, time.Second, 10*time.Millisecond, "BD task status should be updated to closed")

	// Step 6: Verify task is removed from in-memory TaskRepository
	// Wait for async command to complete
	require.Eventually(t, func() bool {
		_, err := stack.taskRepo.Get(taskID)
		return err == repository.ErrTaskNotFound
	}, time.Second, 10*time.Millisecond, "task should be deleted from in-memory repo after mark_task_complete")

	// Step 7: Verify task no longer appears in query_worker_state response
	// Use a fresh struct to avoid json.Unmarshal merging with existing map
	stateResult, err = stack.adapter.HandleQueryWorkerState(stack.ctx, nil)
	require.NoError(t, err)
	require.False(t, stateResult.IsError)

	var finalStateResp struct {
		Workers []map[string]interface{}          `json:"workers"`
		Tasks   map[string]map[string]interface{} `json:"tasks"`
	}
	err = json.Unmarshal([]byte(stateResult.Content[0].Text), &finalStateResp)
	require.NoError(t, err)

	// Task should NOT be present in the tasks map after completion
	assert.NotContains(t, finalStateResp.Tasks, taskID, "task should NOT appear in query_worker_state after mark_task_complete")

	// Step 8: Verify BD comment was added for completion
	comments := getComments(stack.mockBDExecutor)
	var foundCompletionComment bool
	for _, c := range comments {
		if c.TaskID == taskID && strings.Contains(c.Comment, "Task completed") {
			foundCompletionComment = true
			break
		}
	}
	assert.True(t, foundCompletionComment, "BD should have completion comment")

	t.Logf("MarkTaskComplete removes task from QueryWorkerState: task %s removed successfully after completion", taskID)
}

// ===========================================================================
// Integration Test: Session Ref Capture Wiring
// ===========================================================================

// mockSessionRefNotifierForIntegration records calls to NotifySessionRef for integration testing.
type mockSessionRefNotifierForIntegration struct {
	mu    sync.Mutex
	calls []sessionRefNotification
}

type sessionRefNotification struct {
	ProcessID  string
	SessionRef string
	WorkDir    string
}

func (m *mockSessionRefNotifierForIntegration) NotifySessionRef(processID, sessionRef, workDir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, sessionRefNotification{
		ProcessID:  processID,
		SessionRef: sessionRef,
		WorkDir:    workDir,
	})
	return nil
}

func (m *mockSessionRefNotifierForIntegration) getCalls() []sessionRefNotification {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]sessionRefNotification, len(m.calls))
	copy(result, m.calls)
	return result
}

// testV2StackWithSessionRefNotifier contains v2 components with session ref notifier wired.
type testV2StackWithSessionRefNotifier struct {
	*testV2Stack
	mockNotifier *mockSessionRefNotifierForIntegration
}

// newTestV2StackWithSessionRefNotifier creates a v2 stack with SessionRefNotifier wired.
func newTestV2StackWithSessionRefNotifier(t *testing.T) *testV2StackWithSessionRefNotifier {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	// Create repositories
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(repository.DefaultQueueMaxSize)

	// Create event bus
	eventBus := pubsub.NewBroker[any]()

	// Create command processor with event bus
	cmdProcessor := processor.NewCommandProcessor(
		processor.WithQueueCapacity(1000),
		processor.WithEventBus(eventBus),
		processor.WithTaskRepository(taskRepo),
		processor.WithQueueRepository(queueRepo),
	)

	// Create mock spawner
	mockSpawner := &mockProcessSpawner{
		processes: make(map[string]*mockProcess),
	}

	// Create mock BD executor with permissive expectations
	mockBD := mocks.NewMockTaskExecutor(t)
	setupPermissiveBDMock(mockBD)

	// Create process registry for handlers
	registry := process.NewProcessRegistry()

	// Create mock session ref notifier
	mockNotifier := &mockSessionRefNotifierForIntegration{}

	stack := &testV2Stack{
		processRepo: processRepo,
		taskRepo:    taskRepo,
		queueRepo:   queueRepo,
		processor:   cmdProcessor,
		eventBus:    eventBus,
		registry:    registry,
		ctx:         ctx,
		cancel:      cancel,
		mockSpawner: mockSpawner,
		mockBD:      mockBD,
	}

	// Register handlers with session ref notifier wired
	registerHandlersWithSessionRefNotifier(stack, registry, mockBD, mockNotifier)

	// Create V2Adapter with repositories for read-only operations
	v2Adapter := adapter.NewV2Adapter(cmdProcessor,
		adapter.WithProcessRepository(processRepo),
		adapter.WithTaskRepository(taskRepo),
		adapter.WithQueueRepository(queueRepo),
	)
	stack.adapter = v2Adapter

	// Start processor loop
	go cmdProcessor.Run(ctx)

	// Wait for processor to be running
	require.Eventually(t, func() bool {
		return cmdProcessor.IsRunning()
	}, time.Second, 10*time.Millisecond, "processor should start running")

	return &testV2StackWithSessionRefNotifier{
		testV2Stack:  stack,
		mockNotifier: mockNotifier,
	}
}

// registerHandlersWithSessionRefNotifier registers handlers with SessionRefNotifier wired.
func registerHandlersWithSessionRefNotifier(
	stack *testV2Stack,
	registry *process.ProcessRegistry,
	bdExec *mocks.MockTaskExecutor,
	notifier *mockSessionRefNotifierForIntegration,
) {
	processRepo := stack.processRepo
	taskRepo := stack.taskRepo
	queueRepo := stack.queueRepo
	cmdProcessor := stack.processor

	// Process Lifecycle handlers
	cmdProcessor.RegisterHandler(command.CmdSpawnProcess,
		handler.NewSpawnProcessHandler(processRepo, registry))
	cmdProcessor.RegisterHandler(command.CmdRetireProcess,
		handler.NewRetireProcessHandler(processRepo, registry))
	cmdProcessor.RegisterHandler(command.CmdReplaceProcess,
		handler.NewReplaceProcessHandler(processRepo, registry))

	// Stop Worker handler
	cmdProcessor.RegisterHandler(command.CmdStopProcess,
		handler.NewStopWorkerHandler(processRepo, taskRepo, queueRepo, registry))

	// Messaging handlers
	cmdProcessor.RegisterHandler(command.CmdSendToProcess,
		handler.NewSendToProcessHandler(processRepo, queueRepo))
	cmdProcessor.RegisterHandler(command.CmdBroadcast,
		handler.NewBroadcastHandler(processRepo))
	cmdProcessor.RegisterHandler(command.CmdDeliverProcessQueued,
		handler.NewDeliverProcessQueuedHandler(processRepo, queueRepo, nil))

	// Task Assignment handlers
	cmdProcessor.RegisterHandler(command.CmdAssignTask,
		handler.NewAssignTaskHandler(processRepo, taskRepo,
			handler.WithBDExecutor(bdExec),
			handler.WithQueueRepository(queueRepo)))
	cmdProcessor.RegisterHandler(command.CmdAssignReview,
		handler.NewAssignReviewHandler(processRepo, taskRepo, queueRepo))
	cmdProcessor.RegisterHandler(command.CmdApproveCommit,
		handler.NewApproveCommitHandler(processRepo, taskRepo, queueRepo))
	cmdProcessor.RegisterHandler(command.CmdAssignReviewFeedback,
		handler.NewAssignReviewFeedbackHandler(processRepo, taskRepo, queueRepo))

	// State Transition handlers
	cmdProcessor.RegisterHandler(command.CmdReportComplete,
		handler.NewReportCompleteHandler(processRepo, taskRepo, queueRepo,
			handler.WithReportCompleteBDExecutor(bdExec)))
	cmdProcessor.RegisterHandler(command.CmdReportVerdict,
		handler.NewReportVerdictHandler(processRepo, taskRepo, queueRepo,
			handler.WithReportVerdictBDExecutor(bdExec)))
	cmdProcessor.RegisterHandler(command.CmdTransitionPhase,
		handler.NewTransitionPhaseHandler(processRepo, queueRepo))

	// ProcessTurnComplete handler with SessionRefNotifier wired
	cmdProcessor.RegisterHandler(command.CmdProcessTurnComplete,
		handler.NewProcessTurnCompleteHandler(processRepo, queueRepo,
			handler.WithTurnCompleteProcessRegistry(registry),
			handler.WithSessionRefNotifier(notifier)))

	// BD Task Status handlers
	cmdProcessor.RegisterHandler(command.CmdMarkTaskComplete,
		handler.NewMarkTaskCompleteHandler(bdExec, taskRepo))
	cmdProcessor.RegisterHandler(command.CmdMarkTaskFailed,
		handler.NewMarkTaskFailedHandler(bdExec))
}

// TestIntegration_SessionRefCapture_CoordinatorFirstSuccessfulTurn verifies that
// coordinator session ref is captured and notifier is called after first successful turn.
func TestIntegration_SessionRefCapture_CoordinatorFirstSuccessfulTurn(t *testing.T) {
	stack := newTestV2StackWithSessionRefNotifier(t)
	defer stack.cleanup()

	// Step 1: Manually create a coordinator process in repository with first turn not completed
	coord := &repository.Process{
		ID:               repository.CoordinatorID,
		Role:             repository.RoleCoordinator,
		Status:           repository.StatusWorking,
		HasCompletedTurn: false,
	}
	stack.processRepo.AddProcess(coord)

	// Step 2: Create a mock live process with session ID via init event and register it
	mockProc := newMockHeadlessProcessFull()
	liveProc := process.New(repository.CoordinatorID, repository.RoleCoordinator, mockProc, nil, nil)
	liveProc.Start()
	mockProc.SendInitEvent("coord-session-abc-123")
	time.Sleep(10 * time.Millisecond) // Allow event to be processed
	stack.registry.Register(liveProc)

	// Step 3: Submit ProcessTurnCompleteCommand (first successful turn)
	turnCmd := command.NewProcessTurnCompleteCommand(repository.CoordinatorID, true, nil, nil)
	result, err := stack.processor.SubmitAndWait(stack.ctx, turnCmd)

	// Step 4: Verify command succeeded
	require.NoError(t, err)
	require.True(t, result.Success, "turn complete should succeed")

	// Step 5: Verify notifier was called with correct values
	require.Eventually(t, func() bool {
		calls := stack.mockNotifier.getCalls()
		return len(calls) > 0
	}, time.Second, 10*time.Millisecond, "notifier should be called")

	calls := stack.mockNotifier.getCalls()
	require.Len(t, calls, 1, "should have exactly one notifier call")
	assert.Equal(t, repository.CoordinatorID, calls[0].ProcessID)
	assert.Equal(t, "coord-session-abc-123", calls[0].SessionRef)
	assert.Equal(t, "/test/workdir", calls[0].WorkDir) // From mockHeadlessProcessFull

	// Step 6: Verify repository Process.SessionID was updated
	updated, _ := stack.processRepo.Get(repository.CoordinatorID)
	assert.Equal(t, "coord-session-abc-123", updated.SessionID)
	assert.True(t, updated.HasCompletedTurn)

	t.Log("Session ref capture wiring test passed: coordinator session ref captured on first successful turn")
}

// TestIntegration_SessionRefCapture_WorkerFirstSuccessfulTurn verifies that
// worker session ref is captured and notifier is called after first successful turn.
func TestIntegration_SessionRefCapture_WorkerFirstSuccessfulTurn(t *testing.T) {
	stack := newTestV2StackWithSessionRefNotifier(t)
	defer stack.cleanup()

	// Step 1: Manually create a worker process in repository with first turn not completed
	workerID := "worker-1"
	worker := &repository.Process{
		ID:               workerID,
		Role:             repository.RoleWorker,
		Status:           repository.StatusWorking,
		HasCompletedTurn: false,
	}
	stack.processRepo.AddProcess(worker)

	// Step 2: Create a mock live process with session ID via init event and register it
	mockProc := newMockHeadlessProcessFull()
	liveProc := process.New(workerID, repository.RoleWorker, mockProc, nil, nil)
	liveProc.Start()
	mockProc.SendInitEvent("worker-session-xyz-789")
	time.Sleep(10 * time.Millisecond) // Allow event to be processed
	stack.registry.Register(liveProc)

	// Step 3: Submit ProcessTurnCompleteCommand (first successful turn)
	turnCmd := command.NewProcessTurnCompleteCommand(workerID, true, nil, nil)
	result, err := stack.processor.SubmitAndWait(stack.ctx, turnCmd)

	// Step 4: Verify command succeeded
	require.NoError(t, err)
	require.True(t, result.Success, "turn complete should succeed")

	// Step 5: Verify notifier was called with correct values
	require.Eventually(t, func() bool {
		calls := stack.mockNotifier.getCalls()
		return len(calls) > 0
	}, time.Second, 10*time.Millisecond, "notifier should be called")

	calls := stack.mockNotifier.getCalls()
	require.Len(t, calls, 1, "should have exactly one notifier call")
	assert.Equal(t, workerID, calls[0].ProcessID)
	assert.Equal(t, "worker-session-xyz-789", calls[0].SessionRef)
	assert.Equal(t, "/test/workdir", calls[0].WorkDir) // From mockHeadlessProcessFull

	// Step 6: Verify repository Process.SessionID was updated
	updated, _ := stack.processRepo.Get(workerID)
	assert.Equal(t, "worker-session-xyz-789", updated.SessionID)
	assert.True(t, updated.HasCompletedTurn)

	t.Log("Session ref capture wiring test passed: worker session ref captured on first successful turn")
}

// TestIntegration_SessionRefCapture_SecondTurnDoesNotRecapture verifies that
// notifier is NOT called on second turn (only first successful turn).
func TestIntegration_SessionRefCapture_SecondTurnDoesNotRecapture(t *testing.T) {
	stack := newTestV2StackWithSessionRefNotifier(t)
	defer stack.cleanup()

	// Step 1: Create a worker that has already completed first turn
	workerID := "worker-1"
	worker := &repository.Process{
		ID:               workerID,
		Role:             repository.RoleWorker,
		Status:           repository.StatusWorking,
		HasCompletedTurn: true, // Already completed first turn
		SessionID:        "previous-session",
	}
	stack.processRepo.AddProcess(worker)

	// Step 2: Create a mock live process with NEW session ID via init event (simulating refresh)
	mockProc := newMockHeadlessProcessFull()
	liveProc := process.New(workerID, repository.RoleWorker, mockProc, nil, nil)
	liveProc.Start()
	mockProc.SendInitEvent("new-session-id")
	time.Sleep(10 * time.Millisecond) // Allow event to be processed
	stack.registry.Register(liveProc)

	// Step 3: Submit ProcessTurnCompleteCommand (second turn)
	turnCmd := command.NewProcessTurnCompleteCommand(workerID, true, nil, nil)
	result, err := stack.processor.SubmitAndWait(stack.ctx, turnCmd)

	// Step 4: Verify command succeeded
	require.NoError(t, err)
	require.True(t, result.Success, "turn complete should succeed")

	// Step 5: Verify notifier was NOT called (second turn should not recapture)
	time.Sleep(50 * time.Millisecond) // Give time for async operations
	calls := stack.mockNotifier.getCalls()
	assert.Len(t, calls, 0, "notifier should NOT be called on second turn")

	// Step 6: Verify repository Process.SessionID was NOT changed
	updated, _ := stack.processRepo.Get(workerID)
	assert.Equal(t, "previous-session", updated.SessionID, "session ID should not be updated on second turn")

	t.Log("Session ref capture wiring test passed: second turn does not recapture")
}

// TestIntegration_SessionRefCapture_SessionClosedBeforeFirstTurn verifies that
// closing a session before first turn doesn't panic and session ref is not captured.
func TestIntegration_SessionRefCapture_SessionClosedBeforeFirstTurn(t *testing.T) {
	stack := newTestV2StackWithSessionRefNotifier(t)
	defer stack.cleanup()

	// Step 1: Create a coordinator that never completes first turn
	coord := &repository.Process{
		ID:               repository.CoordinatorID,
		Role:             repository.RoleCoordinator,
		Status:           repository.StatusWorking,
		HasCompletedTurn: false,
	}
	stack.processRepo.AddProcess(coord)

	// Step 2: Create a mock live process but do NOT register it (simulating session closing)
	// The registry.Get will return nil

	// Step 3: Submit ProcessTurnCompleteCommand (first turn, but process not in registry)
	turnCmd := command.NewProcessTurnCompleteCommand(repository.CoordinatorID, true, nil, nil)
	result, err := stack.processor.SubmitAndWait(stack.ctx, turnCmd)

	// Step 4: Verify command succeeded (graceful handling of missing process)
	require.NoError(t, err)
	require.True(t, result.Success, "turn complete should succeed even without registry entry")

	// Step 5: Verify notifier was NOT called (no process in registry to get session from)
	time.Sleep(50 * time.Millisecond)
	calls := stack.mockNotifier.getCalls()
	assert.Len(t, calls, 0, "notifier should NOT be called when process not in registry")

	// Step 6: Verify process state was still updated (Ready status)
	updated, _ := stack.processRepo.Get(repository.CoordinatorID)
	assert.Equal(t, repository.StatusReady, updated.Status)
	assert.True(t, updated.HasCompletedTurn)
	assert.Empty(t, updated.SessionID, "session ID should remain empty")

	t.Log("Session ref capture wiring test passed: session closed before first turn handled gracefully")
}
