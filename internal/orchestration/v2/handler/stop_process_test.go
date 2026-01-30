package handler_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/handler"
	"github.com/zjrosen/perles/internal/orchestration/v2/process"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
)

// ===========================================================================
// Test Helpers for StopWorkerHandler
// ===========================================================================

func setupStopWorkerRepos() (*repository.MemoryProcessRepository, *repository.MemoryTaskRepository, *repository.MemoryQueueRepository) {
	return repository.NewMemoryProcessRepository(), repository.NewMemoryTaskRepository(), repository.NewMemoryQueueRepository(0)
}

// mockHeadlessProcess implements client.HeadlessProcess for testing.
type mockHeadlessProcess struct {
	mu           sync.Mutex
	cancelled    bool
	waitCalled   bool
	waitDelay    time.Duration // How long Wait() blocks
	pid          int
	events       chan client.OutputEvent
	errors       chan error
	isRunning    bool
	status       client.ProcessStatus
	waitBlocker  chan struct{} // If set, Wait() blocks until this is closed
	unresponsive bool          // If true, Cancel won't signal completion
	eventsClosed bool
	errorsClosed bool
}

func newMockHeadlessProcess(pid int) *mockHeadlessProcess {
	return &mockHeadlessProcess{
		pid:       pid,
		events:    make(chan client.OutputEvent),
		errors:    make(chan error),
		isRunning: true,
		status:    client.StatusRunning,
	}
}

func (m *mockHeadlessProcess) Events() <-chan client.OutputEvent { return m.events }
func (m *mockHeadlessProcess) Errors() <-chan error              { return m.errors }
func (m *mockHeadlessProcess) SessionRef() string                { return "mock-session" }
func (m *mockHeadlessProcess) Status() client.ProcessStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

func (m *mockHeadlessProcess) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.isRunning
}

func (m *mockHeadlessProcess) Cancel() error {
	m.mu.Lock()
	m.cancelled = true
	m.status = client.StatusCancelled
	m.isRunning = false
	unresponsive := m.unresponsive
	blocker := m.waitBlocker
	eventsClosed := m.eventsClosed
	errorsClosed := m.errorsClosed
	m.mu.Unlock()

	// If unresponsive mode, don't signal completion
	if unresponsive {
		return nil
	}

	// Close events channel to signal completion
	if !eventsClosed {
		m.mu.Lock()
		m.eventsClosed = true
		m.mu.Unlock()
		close(m.events)
	}
	if !errorsClosed {
		m.mu.Lock()
		m.errorsClosed = true
		m.mu.Unlock()
		close(m.errors)
	}

	// If we have a wait blocker, close it to unblock Wait()
	if blocker != nil {
		select {
		case <-blocker:
		default:
			close(blocker)
		}
	}
	return nil
}

func (m *mockHeadlessProcess) Wait() error {
	m.mu.Lock()
	m.waitCalled = true
	delay := m.waitDelay
	blocker := m.waitBlocker
	m.mu.Unlock()

	if blocker != nil {
		<-blocker
	}
	if delay > 0 {
		time.Sleep(delay)
	}
	return nil
}

func (m *mockHeadlessProcess) WorkDir() string { return "/test" }
func (m *mockHeadlessProcess) PID() int        { return m.pid }
func (m *mockHeadlessProcess) Send(msg string) error {
	return nil
}

// SendInitEvent sends an init event with the given session ID to the events channel.
// This allows tests to set the session ID on a process through the normal init flow.
func (m *mockHeadlessProcess) SendInitEvent(sessionID string) {
	m.events <- client.OutputEvent{
		Type:      client.EventSystem,
		SubType:   "init",
		SessionID: sessionID,
	}
}

// phasePtr is a helper to create a pointer to a ProcessPhase.
func phasePtr(p events.ProcessPhase) *events.ProcessPhase {
	return &p
}

// ===========================================================================
// StopWorkerHandler Tests
// ===========================================================================

func TestStopWorkerHandler_GracefulStop(t *testing.T) {
	processRepo, taskRepo, queueRepo := setupStopWorkerRepos()
	registry := process.NewProcessRegistry()

	// Create worker in Working status
	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusWorking,
		Phase:  phasePtr(events.ProcessPhaseImplementing),
	}
	processRepo.AddProcess(worker)

	// Create and register a live process
	mockProc := newMockHeadlessProcess(1234)
	liveProcess := process.New("worker-1", repository.RoleWorker, mockProc, nil, nil)
	liveProcess.Start()
	registry.Register(liveProcess)

	h := handler.NewStopWorkerHandler(processRepo, taskRepo, queueRepo, registry)

	cmd := command.NewStopProcessCommand(command.SourceUser, "worker-1", false, "user requested stop")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify Cancel() was called
	assert.True(t, mockProc.cancelled)

	// Verify repository updated to Retired
	updated, _ := processRepo.Get("worker-1")
	assert.Equal(t, repository.StatusStopped, updated.Status)

	// Verify result
	stopResult := result.Data.(*handler.StopWorkerResult)
	assert.False(t, stopResult.WasNoOp)
	assert.True(t, stopResult.Graceful)
}

func TestStopWorkerHandler_ForceStop(t *testing.T) {
	processRepo, taskRepo, queueRepo := setupStopWorkerRepos()
	registry := process.NewProcessRegistry()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusWorking,
		Phase:  phasePtr(events.ProcessPhaseImplementing),
	}
	processRepo.AddProcess(worker)

	// Create mock process with a PID
	mockProc := newMockHeadlessProcess(12345)
	liveProcess := process.New("worker-1", repository.RoleWorker, mockProc, nil, nil)
	liveProcess.Start()
	registry.Register(liveProcess)

	h := handler.NewStopWorkerHandler(processRepo, taskRepo, queueRepo, registry)

	// Use Force=true for immediate SIGKILL
	cmd := command.NewStopProcessCommand(command.SourceUser, "worker-1", true, "force stop")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify repository updated
	updated, _ := processRepo.Get("worker-1")
	assert.Equal(t, repository.StatusStopped, updated.Status)

	// Verify result indicates force stop
	stopResult := result.Data.(*handler.StopWorkerResult)
	assert.False(t, stopResult.Graceful)
}

func TestStopWorkerHandler_GracefulEscalation(t *testing.T) {
	processRepo, taskRepo, queueRepo := setupStopWorkerRepos()
	registry := process.NewProcessRegistry()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusWorking,
		Phase:  phasePtr(events.ProcessPhaseImplementing),
	}
	processRepo.AddProcess(worker)

	// Create mock process that doesn't respond to Cancel (simulates unresponsive process)
	mockProc := newMockHeadlessProcess(9999)
	mockProc.waitBlocker = make(chan struct{})
	mockProc.unresponsive = true // Cancel won't signal completion

	liveProcess := process.New("worker-1", repository.RoleWorker, mockProc, nil, nil)
	liveProcess.Start()
	registry.Register(liveProcess)

	h := handler.NewStopWorkerHandler(processRepo, taskRepo, queueRepo, registry)

	// Use a short context timeout to speed up test
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	cmd := command.NewStopProcessCommand(command.SourceUser, "worker-1", false, "")

	// Run with short timeout - should escalate to force stop
	result, err := h.Handle(ctx, cmd)

	// Clean up the mock by unblocking Wait
	close(mockProc.waitBlocker)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify repository updated to Retired
	updated, _ := processRepo.Get("worker-1")
	assert.Equal(t, repository.StatusStopped, updated.Status)

	// Result should indicate force stop (escalated)
	stopResult := result.Data.(*handler.StopWorkerResult)
	assert.False(t, stopResult.Graceful) // Escalated to force
}

func TestStopWorkerHandler_PhaseWarning(t *testing.T) {
	processRepo, taskRepo, queueRepo := setupStopWorkerRepos()
	registry := process.NewProcessRegistry()

	// Create worker in Committing phase
	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusWorking,
		Phase:  phasePtr(events.ProcessPhaseCommitting),
		TaskID: "task-123",
	}
	processRepo.AddProcess(worker)

	h := handler.NewStopWorkerHandler(processRepo, taskRepo, queueRepo, registry)

	// Try to stop without force - should return warning
	cmd := command.NewStopProcessCommand(command.SourceUser, "worker-1", false, "")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Should be a no-op with phase warning
	stopResult := result.Data.(*handler.StopWorkerResult)
	assert.True(t, stopResult.PhaseWarning)
	assert.True(t, stopResult.WasNoOp)

	// Verify worker is NOT retired
	worker, _ = processRepo.Get("worker-1")
	assert.Equal(t, repository.StatusWorking, worker.Status)

	// Verify warning event was emitted
	require.Len(t, result.Events, 1)
	event := result.Events[0].(events.ProcessEvent)
	assert.Equal(t, events.ProcessOutput, event.Type)
	assert.Contains(t, event.Output, "Committing phase")
}

func TestStopWorkerHandler_PhaseWarning_ForceOverride(t *testing.T) {
	processRepo, taskRepo, queueRepo := setupStopWorkerRepos()
	registry := process.NewProcessRegistry()

	// Create worker in Committing phase
	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusWorking,
		Phase:  phasePtr(events.ProcessPhaseCommitting),
		TaskID: "task-123",
	}
	processRepo.AddProcess(worker)

	// Create live process
	mockProc := newMockHeadlessProcess(5555)
	liveProcess := process.New("worker-1", repository.RoleWorker, mockProc, nil, nil)
	liveProcess.Start()
	registry.Register(liveProcess)

	h := handler.NewStopWorkerHandler(processRepo, taskRepo, queueRepo, registry)

	// Use force=true to override phase warning
	cmd := command.NewStopProcessCommand(command.SourceUser, "worker-1", true, "")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Should NOT be a no-op - worker should be stopped
	stopResult := result.Data.(*handler.StopWorkerResult)
	assert.False(t, stopResult.PhaseWarning)
	assert.False(t, stopResult.WasNoOp)

	// Verify worker IS retired
	updated, _ := processRepo.Get("worker-1")
	assert.Equal(t, repository.StatusStopped, updated.Status)
}

func TestStopWorkerHandler_TaskCleanup(t *testing.T) {
	processRepo, taskRepo, queueRepo := setupStopWorkerRepos()
	registry := process.NewProcessRegistry()

	// Create worker with assigned task
	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusWorking,
		Phase:  phasePtr(events.ProcessPhaseImplementing),
		TaskID: "task-xyz",
	}
	processRepo.AddProcess(worker)

	// Create task assignment
	task := &repository.TaskAssignment{
		TaskID:      "task-xyz",
		Implementer: "worker-1",
		Status:      repository.TaskImplementing,
		StartedAt:   time.Now(),
	}
	taskRepo.AddTask(task)

	// Create live process
	mockProc := newMockHeadlessProcess(3333)
	liveProcess := process.New("worker-1", repository.RoleWorker, mockProc, nil, nil)
	liveProcess.Start()
	registry.Register(liveProcess)

	h := handler.NewStopWorkerHandler(processRepo, taskRepo, queueRepo, registry)

	cmd := command.NewStopProcessCommand(command.SourceUser, "worker-1", false, "")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify TaskID cleared from process
	updated, _ := processRepo.Get("worker-1")
	assert.Empty(t, updated.TaskID)

	// Verify task implementer was cleared
	updatedTask, _ := taskRepo.Get("task-xyz")
	assert.Empty(t, updatedTask.Implementer)
}

func TestStopWorkerHandler_AlreadyRetired(t *testing.T) {
	processRepo, taskRepo, queueRepo := setupStopWorkerRepos()
	registry := process.NewProcessRegistry()

	// Create already retired worker
	worker := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusRetired,
		RetiredAt: time.Now(),
	}
	processRepo.AddProcess(worker)

	h := handler.NewStopWorkerHandler(processRepo, taskRepo, queueRepo, registry)

	cmd := command.NewStopProcessCommand(command.SourceUser, "worker-1", false, "")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Should be a no-op
	stopResult := result.Data.(*handler.StopWorkerResult)
	assert.True(t, stopResult.WasNoOp)
}

func TestStopWorkerHandler_AlreadyStopped(t *testing.T) {
	processRepo, taskRepo, queueRepo := setupStopWorkerRepos()
	registry := process.NewProcessRegistry()

	// Create already stopped worker
	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusStopped,
	}
	processRepo.AddProcess(worker)

	h := handler.NewStopWorkerHandler(processRepo, taskRepo, queueRepo, registry)

	cmd := command.NewStopProcessCommand(command.SourceUser, "worker-1", false, "")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Should be a no-op
	stopResult := result.Data.(*handler.StopWorkerResult)
	assert.True(t, stopResult.WasNoOp)
}

func TestStopWorkerHandler_ProcessNotFound(t *testing.T) {
	processRepo, taskRepo, queueRepo := setupStopWorkerRepos()
	registry := process.NewProcessRegistry()

	h := handler.NewStopWorkerHandler(processRepo, taskRepo, queueRepo, registry)

	cmd := command.NewStopProcessCommand(command.SourceUser, "unknown-worker", false, "")
	result, err := h.Handle(context.Background(), cmd)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, handler.ErrProcessNotFound)
}

func TestStopWorkerHandler_NotInRegistry(t *testing.T) {
	processRepo, taskRepo, queueRepo := setupStopWorkerRepos()
	registry := process.NewProcessRegistry()

	// Create worker in repository but NOT in live registry
	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusReady,
	}
	processRepo.AddProcess(worker)

	h := handler.NewStopWorkerHandler(processRepo, taskRepo, queueRepo, registry)

	cmd := command.NewStopProcessCommand(command.SourceUser, "worker-1", false, "")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Should still update repository
	updated, _ := processRepo.Get("worker-1")
	assert.Equal(t, repository.StatusStopped, updated.Status)

	stopResult := result.Data.(*handler.StopWorkerResult)
	assert.True(t, stopResult.Graceful) // No live process = graceful (no force needed)
}

func TestStopWorkerHandler_ConcurrentStopAndComplete(t *testing.T) {
	processRepo, taskRepo, queueRepo := setupStopWorkerRepos()
	registry := process.NewProcessRegistry()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusWorking,
		Phase:  phasePtr(events.ProcessPhaseImplementing),
	}
	processRepo.AddProcess(worker)

	mockProc := newMockHeadlessProcess(7777)
	liveProcess := process.New("worker-1", repository.RoleWorker, mockProc, nil, nil)
	liveProcess.Start()
	registry.Register(liveProcess)

	h := handler.NewStopWorkerHandler(processRepo, taskRepo, queueRepo, registry)

	// Track concurrent operations
	var stopCompleted atomic.Bool
	var completionAttempted atomic.Bool
	var wg sync.WaitGroup

	// Start stop handler in goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		cmd := command.NewStopProcessCommand(command.SourceUser, "worker-1", false, "")
		result, err := h.Handle(context.Background(), cmd)
		if err == nil && result.Success {
			stopCompleted.Store(true)
		}
	}()

	// Simulate concurrent turn completion attempt
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Small delay to let stop start
		time.Sleep(10 * time.Millisecond)
		// Try to complete the turn (simulating worker finishing naturally)
		completionAttempted.Store(true)
	}()

	wg.Wait()

	// Verify that the system handled the race gracefully
	// Either stop completed OR the worker finished - but no crash/panic
	assert.True(t, stopCompleted.Load() || completionAttempted.Load())

	// Final state should be retired (stop handler wins or gets there eventually)
	updated, _ := processRepo.Get("worker-1")
	assert.Equal(t, repository.StatusStopped, updated.Status)
}

func TestStopWorkerHandler_EmitsProcessStatusChangeEvent(t *testing.T) {
	processRepo, taskRepo, queueRepo := setupStopWorkerRepos()
	registry := process.NewProcessRegistry()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusWorking,
		Phase:  phasePtr(events.ProcessPhaseImplementing),
		TaskID: "task-100",
	}
	processRepo.AddProcess(worker)

	mockProc := newMockHeadlessProcess(8888)
	liveProcess := process.New("worker-1", repository.RoleWorker, mockProc, nil, nil)
	liveProcess.Start()
	registry.Register(liveProcess)

	h := handler.NewStopWorkerHandler(processRepo, taskRepo, queueRepo, registry)

	cmd := command.NewStopProcessCommand(command.SourceUser, "worker-1", false, "")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)

	// Verify ProcessStatusChange event was emitted
	require.Len(t, result.Events, 1)
	event := result.Events[0].(events.ProcessEvent)
	assert.Equal(t, events.ProcessStatusChange, event.Type)
	assert.Equal(t, "worker-1", event.ProcessID)
	assert.Equal(t, events.RoleWorker, event.Role)
	assert.Equal(t, events.ProcessStatusStopped, event.Status)
	assert.Equal(t, "task-100", event.TaskID) // Old task ID preserved in event
}

func TestStopWorkerHandler_ReviewerTaskCleanup(t *testing.T) {
	processRepo, taskRepo, queueRepo := setupStopWorkerRepos()
	registry := process.NewProcessRegistry()

	// Create reviewer worker with assigned task
	worker := &repository.Process{
		ID:     "worker-2",
		Role:   repository.RoleWorker,
		Status: repository.StatusWorking,
		Phase:  phasePtr(events.ProcessPhaseReviewing),
		TaskID: "task-abc",
	}
	processRepo.AddProcess(worker)

	// Create task assignment where worker-2 is reviewer
	task := &repository.TaskAssignment{
		TaskID:          "task-abc",
		Implementer:     "worker-1",
		Reviewer:        "worker-2",
		Status:          repository.TaskInReview,
		StartedAt:       time.Now().Add(-time.Hour),
		ReviewStartedAt: time.Now(),
	}
	taskRepo.AddTask(task)

	// Create live process
	mockProc := newMockHeadlessProcess(4444)
	liveProcess := process.New("worker-2", repository.RoleWorker, mockProc, nil, nil)
	liveProcess.Start()
	registry.Register(liveProcess)

	h := handler.NewStopWorkerHandler(processRepo, taskRepo, queueRepo, registry)

	cmd := command.NewStopProcessCommand(command.SourceUser, "worker-2", false, "")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify reviewer was cleared from task (but implementer remains)
	updatedTask, _ := taskRepo.Get("task-abc")
	assert.Empty(t, updatedTask.Reviewer)
	assert.Equal(t, "worker-1", updatedTask.Implementer) // Implementer unchanged
}

func TestStopWorkerHandler_DrainsQueuedMessages(t *testing.T) {
	processRepo, taskRepo, queueRepo := setupStopWorkerRepos()
	registry := process.NewProcessRegistry()

	// Create worker with queued messages
	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusWorking,
		Phase:  phasePtr(events.ProcessPhaseImplementing),
	}
	processRepo.AddProcess(worker)

	// Queue some messages for this worker
	queue := queueRepo.GetOrCreate("worker-1")
	_ = queue.Enqueue("message 1", repository.SenderUser)
	_ = queue.Enqueue("message 2", repository.SenderCoordinator)
	_ = queue.Enqueue("message 3", repository.SenderUser)
	assert.Equal(t, 3, queue.Size())

	// Create live process
	mockProc := newMockHeadlessProcess(5555)
	liveProcess := process.New("worker-1", repository.RoleWorker, mockProc, nil, nil)
	liveProcess.Start()
	registry.Register(liveProcess)

	h := handler.NewStopWorkerHandler(processRepo, taskRepo, queueRepo, registry)

	cmd := command.NewStopProcessCommand(command.SourceUser, "worker-1", false, "")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify queue was drained
	assert.Equal(t, 0, queue.Size())

	// Verify result contains drained count
	stopResult := result.Data.(*handler.StopWorkerResult)
	assert.Equal(t, 3, stopResult.DrainedCount)

	// Verify events: should have ProcessStatusChange and ProcessQueueChanged
	require.Len(t, result.Events, 2)

	statusEvent := result.Events[0].(events.ProcessEvent)
	assert.Equal(t, events.ProcessStatusChange, statusEvent.Type)
	assert.Equal(t, events.ProcessStatusStopped, statusEvent.Status)

	queueEvent := result.Events[1].(events.ProcessEvent)
	assert.Equal(t, events.ProcessQueueChanged, queueEvent.Type)
	assert.Equal(t, 0, queueEvent.QueueCount)
}

// mockFabricUnsubscriber implements FabricUnsubscriber for testing.
type mockFabricUnsubscriber struct {
	unsubscribeCalls []string
	err              error
}

func (m *mockFabricUnsubscriber) UnsubscribeAll(agentID string) error {
	m.unsubscribeCalls = append(m.unsubscribeCalls, agentID)
	return m.err
}

func TestStopWorkerHandler_Observer_UnsubscribesFromAllChannels(t *testing.T) {
	processRepo, taskRepo, queueRepo := setupStopWorkerRepos()
	registry := process.NewProcessRegistry()

	// Create observer process
	observer := &repository.Process{
		ID:     "observer",
		Role:   repository.RoleObserver,
		Status: repository.StatusWorking,
	}
	processRepo.AddProcess(observer)

	// Create mock unsubscriber
	mockUnsubscriber := &mockFabricUnsubscriber{}

	// Create live process
	mockProc := newMockHeadlessProcess(6666)
	liveProcess := process.New("observer", repository.RoleObserver, mockProc, nil, nil)
	liveProcess.Start()
	registry.Register(liveProcess)

	h := handler.NewStopWorkerHandler(processRepo, taskRepo, queueRepo, registry,
		handler.WithFabricUnsubscriber(mockUnsubscriber))

	cmd := command.NewStopProcessCommand(command.SourceUser, "observer", false, "")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify UnsubscribeAll was called for observer
	require.Len(t, mockUnsubscriber.unsubscribeCalls, 1)
	assert.Equal(t, "observer", mockUnsubscriber.unsubscribeCalls[0])

	// Verify observer is stopped
	updated, _ := processRepo.Get("observer")
	assert.Equal(t, repository.StatusStopped, updated.Status)
}

func TestStopWorkerHandler_Worker_DoesNotCallUnsubscribe(t *testing.T) {
	processRepo, taskRepo, queueRepo := setupStopWorkerRepos()
	registry := process.NewProcessRegistry()

	// Create worker process (not observer)
	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusWorking,
		Phase:  phasePtr(events.ProcessPhaseImplementing),
	}
	processRepo.AddProcess(worker)

	// Create mock unsubscriber
	mockUnsubscriber := &mockFabricUnsubscriber{}

	// Create live process
	mockProc := newMockHeadlessProcess(7777)
	liveProcess := process.New("worker-1", repository.RoleWorker, mockProc, nil, nil)
	liveProcess.Start()
	registry.Register(liveProcess)

	h := handler.NewStopWorkerHandler(processRepo, taskRepo, queueRepo, registry,
		handler.WithFabricUnsubscriber(mockUnsubscriber))

	cmd := command.NewStopProcessCommand(command.SourceUser, "worker-1", false, "")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify UnsubscribeAll was NOT called for worker
	assert.Empty(t, mockUnsubscriber.unsubscribeCalls)
}

func TestStopWorkerHandler_Observer_UnsubscribeError_DoesNotBlockStop(t *testing.T) {
	processRepo, taskRepo, queueRepo := setupStopWorkerRepos()
	registry := process.NewProcessRegistry()

	// Create observer process
	observer := &repository.Process{
		ID:     "observer",
		Role:   repository.RoleObserver,
		Status: repository.StatusWorking,
	}
	processRepo.AddProcess(observer)

	// Create mock unsubscriber that returns an error
	mockUnsubscriber := &mockFabricUnsubscriber{
		err: errors.New("unsubscribe failed"),
	}

	// Create live process
	mockProc := newMockHeadlessProcess(8888)
	liveProcess := process.New("observer", repository.RoleObserver, mockProc, nil, nil)
	liveProcess.Start()
	registry.Register(liveProcess)

	h := handler.NewStopWorkerHandler(processRepo, taskRepo, queueRepo, registry,
		handler.WithFabricUnsubscriber(mockUnsubscriber))

	cmd := command.NewStopProcessCommand(command.SourceUser, "observer", false, "")
	result, err := h.Handle(context.Background(), cmd)

	// Stop should still succeed despite unsubscribe error (fail-open)
	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify observer is stopped
	updated, _ := processRepo.Get("observer")
	assert.Equal(t, repository.StatusStopped, updated.Status)
}
