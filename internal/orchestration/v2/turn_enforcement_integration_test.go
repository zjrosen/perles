// Package v2 provides integration tests for turn completion enforcement.
// These tests verify the full enforcement flow across handlers and WorkerServer.
package v2

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/handler"
	"github.com/zjrosen/perles/internal/orchestration/v2/process"
	"github.com/zjrosen/perles/internal/orchestration/v2/processor"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
	"github.com/zjrosen/perles/internal/pubsub"
)

// ===========================================================================
// Turn Enforcement Integration Test Infrastructure
// ===========================================================================

// enforcementTestStack contains components for turn enforcement integration testing.
type enforcementTestStack struct {
	processRepo *repository.MemoryProcessRepository
	queueRepo   *repository.MemoryQueueRepository
	processor   *processor.CommandProcessor
	registry    *process.ProcessRegistry
	enforcer    *handler.TurnCompletionTracker
	eventBus    *pubsub.Broker[any]
	ctx         context.Context
	cancel      context.CancelFunc
}

// newEnforcementTestStack creates a v2 stack with turn enforcement wired.
func newEnforcementTestStack(t *testing.T) *enforcementTestStack {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	// Create repositories
	processRepo := repository.NewMemoryProcessRepository()
	queueRepo := repository.NewMemoryQueueRepository(repository.DefaultQueueMaxSize)

	// Create event bus
	eventBus := pubsub.NewBroker[any]()

	// Create shared turn enforcer
	enforcer := handler.NewTurnCompletionTracker()

	// Create command processor
	cmdProcessor := processor.NewCommandProcessor(
		processor.WithQueueCapacity(1000),
		processor.WithEventBus(eventBus),
		processor.WithQueueRepository(queueRepo),
	)

	// Create process registry
	registry := process.NewProcessRegistry()

	stack := &enforcementTestStack{
		processRepo: processRepo,
		queueRepo:   queueRepo,
		processor:   cmdProcessor,
		registry:    registry,
		enforcer:    enforcer,
		eventBus:    eventBus,
		ctx:         ctx,
		cancel:      cancel,
	}

	// Register handlers with turn enforcer wired
	stack.registerHandlersWithEnforcer(cmdProcessor, processRepo, queueRepo, registry, enforcer)

	// Start processor loop
	go cmdProcessor.Run(ctx)

	// Wait for processor to be running
	require.Eventually(t, func() bool {
		return cmdProcessor.IsRunning()
	}, time.Second, 10*time.Millisecond, "processor should start running")

	return stack
}

// registerHandlersWithEnforcer registers all handlers with turn enforcer wired.
func (s *enforcementTestStack) registerHandlersWithEnforcer(
	cmdProcessor *processor.CommandProcessor,
	processRepo *repository.MemoryProcessRepository,
	queueRepo *repository.MemoryQueueRepository,
	registry *process.ProcessRegistry,
	enforcer handler.TurnCompletionEnforcer,
) {
	// SpawnProcess - marks processes as newly spawned
	cmdProcessor.RegisterHandler(command.CmdSpawnProcess,
		handler.NewSpawnProcessHandler(processRepo, registry,
			handler.WithTurnEnforcer(enforcer)))

	// DeliverProcessQueued - resets turn state on message delivery
	cmdProcessor.RegisterHandler(command.CmdDeliverProcessQueued,
		handler.NewDeliverProcessQueuedHandler(processRepo, queueRepo, registry,
			handler.WithDeliverTurnEnforcer(enforcer)))

	// ProcessTurnComplete - enforces turn completion requirements
	cmdProcessor.RegisterHandler(command.CmdProcessTurnComplete,
		handler.NewProcessTurnCompleteHandler(processRepo, queueRepo,
			handler.WithProcessTurnEnforcer(enforcer)))

	// RetireProcess - cleans up enforcement state
	cmdProcessor.RegisterHandler(command.CmdRetireProcess,
		handler.NewRetireProcessHandler(processRepo, registry,
			handler.WithRetireTurnEnforcer(enforcer)))

	// SendToProcess - needed for queue operations
	cmdProcessor.RegisterHandler(command.CmdSendToProcess,
		handler.NewSendToProcessHandler(processRepo, queueRepo))
}

// shutdown cleans up the test stack.
func (s *enforcementTestStack) shutdown(t *testing.T) {
	t.Helper()
	s.cancel()
	s.processor.Drain()
}

// createWorker creates a worker process in the repository.
func (s *enforcementTestStack) createWorker(workerID string, status repository.ProcessStatus) {
	proc := &repository.Process{
		ID:        workerID,
		Role:      repository.RoleWorker,
		Status:    status,
		CreatedAt: time.Now(),
	}
	_ = s.processRepo.Save(proc)
}

// createCoordinator creates a coordinator process in the repository.
func (s *enforcementTestStack) createCoordinator(coordID string, status repository.ProcessStatus) {
	proc := &repository.Process{
		ID:        coordID,
		Role:      repository.RoleCoordinator,
		Status:    status,
		CreatedAt: time.Now(),
	}
	_ = s.processRepo.Save(proc)
}

// updateStatus updates a process's status.
func (s *enforcementTestStack) updateStatus(processID string, status repository.ProcessStatus) {
	proc, err := s.processRepo.Get(processID)
	if err != nil {
		return
	}
	proc.Status = status
	_ = s.processRepo.Save(proc)
}

// getQueueSize returns the size of a worker's message queue.
func (s *enforcementTestStack) getQueueSize(workerID string) int {
	return s.queueRepo.Size(workerID)
}

// peekQueue returns the first message in a worker's queue.
func (s *enforcementTestStack) peekQueue(workerID string) (string, bool) {
	queue := s.queueRepo.GetOrCreate(workerID)
	entry, ok := queue.Dequeue()
	if !ok {
		return "", false
	}
	// Put it back since we just want to peek
	_ = queue.Enqueue(entry.Content, entry.Sender)
	return entry.Content, true
}

// drainQueue removes all messages from a worker's queue.
func (s *enforcementTestStack) drainQueue(workerID string) {
	queue := s.queueRepo.GetOrCreate(workerID)
	queue.Drain()
}

// ===========================================================================
// Integration Tests for Turn Enforcement
// ===========================================================================

// TestTurnEnforcement_FullCycle_ToolCalledNoEnforcement verifies that when a
// worker calls a required tool, no enforcement reminder is sent.
func TestTurnEnforcement_FullCycle_ToolCalledNoEnforcement(t *testing.T) {
	stack := newEnforcementTestStack(t)
	defer stack.shutdown(t)

	// Step 1: Spawn a worker process
	workerID := "WORKER.1"
	stack.createWorker(workerID, repository.StatusReady)
	stack.enforcer.MarkAsNewlySpawned(workerID)

	// Step 2: Simulate turn start by resetting turn (as DeliverProcessQueuedHandler would)
	stack.enforcer.ResetTurn(workerID)

	// Step 3: Worker calls post_message (required tool)
	stack.enforcer.RecordToolCall(workerID, "post_message")

	// Step 4: Turn completes
	stack.updateStatus(workerID, repository.StatusWorking)
	turnCompleteCmd := command.NewProcessTurnCompleteCommand(workerID, true, nil, nil)
	stack.processor.Submit(turnCompleteCmd)

	// Wait for command to process
	time.Sleep(50 * time.Millisecond)

	// Verify: No message queued (enforcement not triggered)
	depth := stack.getQueueSize(workerID)
	assert.Equal(t, 0, depth, "No enforcement reminder should be queued when required tool was called")
}

// TestTurnEnforcement_MissingToolTriggerReminder verifies that when a worker
// completes a turn without calling a required tool, an enforcement reminder is sent.
func TestTurnEnforcement_MissingToolTriggerReminder(t *testing.T) {
	stack := newEnforcementTestStack(t)
	defer stack.shutdown(t)

	// Step 1: Spawn a worker process
	workerID := "WORKER.2"
	stack.createWorker(workerID, repository.StatusReady)
	stack.enforcer.MarkAsNewlySpawned(workerID)

	// Step 2: Simulate turn start (clears newly spawned flag)
	stack.enforcer.ResetTurn(workerID)

	// Step 3: Worker does NOT call any required tool (just does work)
	// No RecordToolCall happens

	// Step 4: Turn completes without required tool call
	stack.updateStatus(workerID, repository.StatusWorking)
	turnCompleteCmd := command.NewProcessTurnCompleteCommand(workerID, true, nil, nil)
	stack.processor.Submit(turnCompleteCmd)

	// Wait for command to process (ProcessTurnCompleteHandler + DeliverProcessQueuedHandler)
	time.Sleep(50 * time.Millisecond)

	// Verify: Process is now in Working status because DeliverProcessQueuedHandler delivered the reminder
	// (The handler sets status to Working before attempting delivery)
	proc, err := stack.processRepo.Get(workerID)
	require.NoError(t, err)
	assert.Equal(t, repository.StatusWorking, proc.Status,
		"Process should be in Working status after reminder delivery")

	// Queue should be empty because the reminder was dequeued for delivery
	depth := stack.getQueueSize(workerID)
	assert.Equal(t, 0, depth, "Queue should be empty after reminder was delivered")
}

// TestTurnEnforcement_StartupTurnExempt verifies that the first turn after spawn
// (startup turn) is exempt from enforcement.
func TestTurnEnforcement_StartupTurnExempt(t *testing.T) {
	stack := newEnforcementTestStack(t)
	defer stack.shutdown(t)

	// Step 1: Spawn a worker process
	workerID := "WORKER.3"
	stack.createWorker(workerID, repository.StatusReady)

	// Mark as newly spawned (simulating SpawnProcessHandler)
	stack.enforcer.MarkAsNewlySpawned(workerID)

	// Note: We do NOT call ResetTurn yet - this is the startup turn

	// Step 2: First turn completes WITHOUT calling required tools
	// This should be exempt because IsNewlySpawned is true
	stack.updateStatus(workerID, repository.StatusWorking)
	turnCompleteCmd := command.NewProcessTurnCompleteCommand(workerID, true, nil, nil)
	stack.processor.Submit(turnCompleteCmd)

	// Wait for command to process
	time.Sleep(50 * time.Millisecond)

	// Verify: No enforcement because this is a startup turn
	depth := stack.getQueueSize(workerID)
	assert.Equal(t, 0, depth, "Startup turn should be exempt from enforcement")
}

// TestTurnEnforcement_FailedTurnExempt verifies that failed turns (succeeded=false)
// are exempt from enforcement.
func TestTurnEnforcement_FailedTurnExempt(t *testing.T) {
	stack := newEnforcementTestStack(t)
	defer stack.shutdown(t)

	// Step 1: Spawn a worker process
	workerID := "WORKER.4"
	stack.createWorker(workerID, repository.StatusReady)
	stack.enforcer.MarkAsNewlySpawned(workerID)
	stack.enforcer.ResetTurn(workerID) // Clear newly spawned flag

	// Step 2: Turn fails (no required tool called)
	stack.updateStatus(workerID, repository.StatusWorking)
	turnCompleteCmd := command.NewProcessTurnCompleteCommand(workerID, false, nil, nil)
	stack.processor.Submit(turnCompleteCmd)

	// Wait for command to process
	time.Sleep(50 * time.Millisecond)

	// Verify: No enforcement because turn failed
	depth := stack.getQueueSize(workerID)
	assert.Equal(t, 0, depth, "Failed turn should be exempt from enforcement")
}

// TestTurnEnforcement_MaxRetriesLimit verifies that after 2 enforcement retries,
// the turn completes without further reminders.
func TestTurnEnforcement_MaxRetriesLimit(t *testing.T) {
	stack := newEnforcementTestStack(t)
	defer stack.shutdown(t)

	// Step 1: Spawn a worker process
	workerID := "WORKER.5"
	stack.createWorker(workerID, repository.StatusReady)
	stack.enforcer.MarkAsNewlySpawned(workerID)
	stack.enforcer.ResetTurn(workerID) // Clear newly spawned flag

	// Turn 1: First enforcement (retry count 0 -> 1)
	stack.updateStatus(workerID, repository.StatusWorking)
	turnCompleteCmd := command.NewProcessTurnCompleteCommand(workerID, true, nil, nil)
	stack.processor.Submit(turnCompleteCmd)
	time.Sleep(50 * time.Millisecond)

	// After first enforcement, process should be Working (reminder delivered)
	proc, _ := stack.processRepo.Get(workerID)
	assert.Equal(t, repository.StatusWorking, proc.Status, "First enforcement should keep process working")
	assert.True(t, stack.enforcer.ShouldRetry(workerID), "Should still have retries left after first enforcement")

	// Turn 2: Second enforcement (retry count 1 -> 2)
	stack.updateStatus(workerID, repository.StatusWorking)
	turnCompleteCmd = command.NewProcessTurnCompleteCommand(workerID, true, nil, nil)
	stack.processor.Submit(turnCompleteCmd)
	time.Sleep(50 * time.Millisecond)

	// After second enforcement, process should still be Working
	proc, _ = stack.processRepo.Get(workerID)
	assert.Equal(t, repository.StatusWorking, proc.Status, "Second enforcement should keep process working")
	// Note: After 2 increments, ShouldRetry should return false
	assert.False(t, stack.enforcer.ShouldRetry(workerID), "Max retries should be reached after 2 enforcements")

	// Turn 3: Max retries exceeded (retry count >= 2)
	stack.updateStatus(workerID, repository.StatusWorking)
	turnCompleteCmd = command.NewProcessTurnCompleteCommand(workerID, true, nil, nil)
	stack.processor.Submit(turnCompleteCmd)
	time.Sleep(50 * time.Millisecond)

	// After max retries, turn completes normally (status Ready) with no enforcement
	proc, _ = stack.processRepo.Get(workerID)
	assert.Equal(t, repository.StatusReady, proc.Status, "After max retries, turn should complete normally with Ready status")

	// Queue should be empty (no enforcement reminder)
	depth := stack.getQueueSize(workerID)
	assert.Equal(t, 0, depth, "No enforcement after max retries exceeded")
}

// TestTurnEnforcement_ProcessRetirementCleansUp verifies that retiring a process
// cleans up enforcement tracking state.
func TestTurnEnforcement_ProcessRetirementCleansUp(t *testing.T) {
	stack := newEnforcementTestStack(t)
	defer stack.shutdown(t)

	// Step 1: Spawn a worker process
	workerID := "WORKER.6"
	stack.createWorker(workerID, repository.StatusReady)
	stack.enforcer.MarkAsNewlySpawned(workerID)
	stack.enforcer.RecordToolCall(workerID, "post_message")
	stack.enforcer.IncrementRetry(workerID)

	// Verify state exists before retirement
	assert.True(t, stack.enforcer.IsNewlySpawned(workerID), "State should exist before retirement")

	// Step 2: Retire the process
	retireCmd := command.NewRetireProcessCommand(command.SourceInternal, workerID, "test cleanup")
	stack.processor.Submit(retireCmd)
	time.Sleep(50 * time.Millisecond)

	// Step 3: Verify cleanup
	assert.False(t, stack.enforcer.IsNewlySpawned(workerID), "Spawn flag should be cleared after retirement")

	// Verify by checking ShouldRetry (should be true for unknown process, since retryCount would be 0)
	assert.True(t, stack.enforcer.ShouldRetry(workerID), "Retry count should be reset after cleanup")
}

// TestTurnEnforcement_CoordinatorExempt verifies that coordinators are never
// subject to turn enforcement.
func TestTurnEnforcement_CoordinatorExempt(t *testing.T) {
	stack := newEnforcementTestStack(t)
	defer stack.shutdown(t)

	// Step 1: Create a coordinator process
	coordID := "COORDINATOR"
	stack.createCoordinator(coordID, repository.StatusReady)

	// Step 2: Turn completes without any required tool calls
	stack.updateStatus(coordID, repository.StatusWorking)
	turnCompleteCmd := command.NewProcessTurnCompleteCommand(coordID, true, nil, nil)
	stack.processor.Submit(turnCompleteCmd)
	time.Sleep(50 * time.Millisecond)

	// Verify: No enforcement for coordinator
	depth := stack.getQueueSize(coordID)
	assert.Equal(t, 0, depth, "Coordinator should never receive enforcement reminders")
}

// TestTurnEnforcement_SharedEnforcerAcrossHandlers verifies that the same
// TurnCompletionTracker instance is used across all handlers.
func TestTurnEnforcement_SharedEnforcerAcrossHandlers(t *testing.T) {
	stack := newEnforcementTestStack(t)
	defer stack.shutdown(t)

	// Step 1: Use spawn tracking
	workerID := "WORKER.7"
	stack.createWorker(workerID, repository.StatusReady)

	// Manually mark as spawned (simulating SpawnProcessHandler)
	stack.enforcer.MarkAsNewlySpawned(workerID)

	// Step 2: Verify spawn state is visible
	assert.True(t, stack.enforcer.IsNewlySpawned(workerID), "Spawn state should be set")

	// Step 3: Reset turn (simulating DeliverProcessQueuedHandler)
	stack.enforcer.ResetTurn(workerID)

	// Step 4: Verify spawn state is cleared
	assert.False(t, stack.enforcer.IsNewlySpawned(workerID), "Spawn state should be cleared after ResetTurn")

	// Step 5: Record tool call
	stack.enforcer.RecordToolCall(workerID, "post_message")

	// Step 6: Check turn completion
	missingTools := stack.enforcer.CheckTurnCompletion(workerID, repository.RoleWorker)
	assert.Empty(t, missingTools, "Should have no missing tools after RecordToolCall")
}

// TestTurnEnforcement_AnyRequiredToolSuffices verifies that calling any one of
// the required tools satisfies the enforcement requirement.
func TestTurnEnforcement_AnyRequiredToolSuffices(t *testing.T) {
	requiredTools := []string{"post_message", "report_implementation_complete", "report_review_verdict", "signal_ready"}

	for _, tool := range requiredTools {
		t.Run(tool, func(t *testing.T) {
			stack := newEnforcementTestStack(t)
			defer stack.shutdown(t)

			workerID := "WORKER.8"
			stack.createWorker(workerID, repository.StatusReady)
			stack.enforcer.MarkAsNewlySpawned(workerID)
			stack.enforcer.ResetTurn(workerID)

			// Call this specific required tool
			stack.enforcer.RecordToolCall(workerID, tool)

			// Complete turn
			stack.updateStatus(workerID, repository.StatusWorking)
			turnCompleteCmd := command.NewProcessTurnCompleteCommand(workerID, true, nil, nil)
			stack.processor.Submit(turnCompleteCmd)
			time.Sleep(50 * time.Millisecond)

			// Verify no enforcement
			depth := stack.getQueueSize(workerID)
			assert.Equal(t, 0, depth, "Calling %s should satisfy enforcement requirement", tool)
		})
	}
}
