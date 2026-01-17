package handler_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/metrics"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/handler"
	"github.com/zjrosen/perles/internal/orchestration/v2/process"
	"github.com/zjrosen/perles/internal/orchestration/v2/prompt"
	"github.com/zjrosen/perles/internal/orchestration/v2/prompt/roles"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
)

// ===========================================================================
// Test Helpers
// ===========================================================================

func setupProcessRepos() (*repository.MemoryProcessRepository, *repository.MemoryQueueRepository) {
	return repository.NewMemoryProcessRepository(), repository.NewMemoryQueueRepository(1000)
}

// mockProcessSpawner is a test implementation of ProcessSpawner.
type mockProcessSpawner struct {
	spawnCalls []spawnCall
	spawnErr   error
}

type spawnCall struct {
	ID        string
	Role      repository.ProcessRole
	AgentType roles.AgentType
}

func (m *mockProcessSpawner) SpawnProcess(ctx context.Context, id string, role repository.ProcessRole, opts handler.SpawnOptions) (*process.Process, error) {
	m.spawnCalls = append(m.spawnCalls, spawnCall{ID: id, Role: role, AgentType: opts.AgentType})
	if m.spawnErr != nil {
		return nil, m.spawnErr
	}
	// Return a mock process - in real usage this would be a real process
	return nil, nil
}

// ===========================================================================
// SendToProcessHandler Tests
// ===========================================================================

func TestSendToProcessHandler_SendToCoordinatorWhileWorking_QueuesMessage(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()

	// Create coordinator in Working status
	coord := &repository.Process{
		ID:     repository.CoordinatorID,
		Role:   repository.RoleCoordinator,
		Status: repository.StatusWorking,
	}
	processRepo.AddProcess(coord)

	h := handler.NewSendToProcessHandler(processRepo, queueRepo)

	cmd := command.NewSendToProcessCommand(command.SourceUser, repository.CoordinatorID, "test message")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify message was queued
	queue := queueRepo.GetOrCreate(repository.CoordinatorID)
	assert.Equal(t, 1, queue.Size())

	// Verify result indicates queued
	sendResult := result.Data.(*handler.SendToProcessResult)
	assert.True(t, sendResult.Queued)
	assert.Equal(t, 1, sendResult.QueueSize)

	// Verify ProcessQueueChanged event was emitted
	require.Len(t, result.Events, 1)
	event := result.Events[0].(events.ProcessEvent)
	assert.Equal(t, events.ProcessQueueChanged, event.Type)
	assert.Equal(t, repository.CoordinatorID, event.ProcessID)
	assert.Equal(t, events.RoleCoordinator, event.Role)
}

func TestSendToProcessHandler_SendToCoordinatorWhileReady_TriggersDelivery(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()

	// Create coordinator in Ready status
	coord := &repository.Process{
		ID:     repository.CoordinatorID,
		Role:   repository.RoleCoordinator,
		Status: repository.StatusReady,
	}
	processRepo.AddProcess(coord)

	h := handler.NewSendToProcessHandler(processRepo, queueRepo)

	cmd := command.NewSendToProcessCommand(command.SourceUser, repository.CoordinatorID, "test message")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify result indicates not queued (will be delivered)
	sendResult := result.Data.(*handler.SendToProcessResult)
	assert.False(t, sendResult.Queued)

	// Verify follow-up command to deliver
	require.Len(t, result.FollowUp, 1)
	followUp := result.FollowUp[0].(*command.DeliverProcessQueuedCommand)
	assert.Equal(t, repository.CoordinatorID, followUp.ProcessID)
}

func TestSendToProcessHandler_SendToWorkerWhileWorking_QueuesMessage(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()

	// Create worker in Working status
	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusWorking,
	}
	processRepo.AddProcess(worker)

	h := handler.NewSendToProcessHandler(processRepo, queueRepo)

	cmd := command.NewSendToProcessCommand(command.SourceMCPTool, "worker-1", "task instructions")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify message was queued
	queue := queueRepo.GetOrCreate("worker-1")
	assert.Equal(t, 1, queue.Size())

	// Verify event has correct role
	event := result.Events[0].(events.ProcessEvent)
	assert.Equal(t, events.RoleWorker, event.Role)
}

func TestSendToProcessHandler_SendToWorkerWhileReady_TriggersDelivery(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()

	// Create worker in Ready status
	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusReady,
	}
	processRepo.AddProcess(worker)

	h := handler.NewSendToProcessHandler(processRepo, queueRepo)

	cmd := command.NewSendToProcessCommand(command.SourceMCPTool, "worker-1", "task instructions")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)
	require.Len(t, result.FollowUp, 1)

	followUp := result.FollowUp[0].(*command.DeliverProcessQueuedCommand)
	assert.Equal(t, "worker-1", followUp.ProcessID)
}

func TestSendToProcessHandler_SendToUnknownProcess_ReturnsError(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()

	h := handler.NewSendToProcessHandler(processRepo, queueRepo)

	cmd := command.NewSendToProcessCommand(command.SourceUser, "unknown-process", "message")
	result, err := h.Handle(context.Background(), cmd)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, handler.ErrProcessNotFound)
}

func TestSendToProcessHandler_SendToRetiredProcess_ReturnsError(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()

	// Create retired worker
	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusRetired,
	}
	processRepo.AddProcess(worker)

	h := handler.NewSendToProcessHandler(processRepo, queueRepo)

	cmd := command.NewSendToProcessCommand(command.SourceMCPTool, "worker-1", "message")
	result, err := h.Handle(context.Background(), cmd)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, handler.ErrProcessRetired)
}

// ===========================================================================
// DeliverProcessQueuedHandler Tests
// ===========================================================================

func TestDeliverProcessQueuedHandler_DeliverToCoordinator_DequeuesAndResumes(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()
	registry := process.NewProcessRegistry()

	// Create coordinator in Ready status
	coord := &repository.Process{
		ID:     repository.CoordinatorID,
		Role:   repository.RoleCoordinator,
		Status: repository.StatusReady,
	}
	processRepo.AddProcess(coord)

	// Queue a message
	queue := queueRepo.GetOrCreate(repository.CoordinatorID)
	_ = queue.Enqueue("queued message", repository.SenderUser)

	h := handler.NewDeliverProcessQueuedHandler(processRepo, queueRepo, registry)

	cmd := command.NewDeliverProcessQueuedCommand(command.SourceInternal, repository.CoordinatorID)
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify queue was drained
	assert.True(t, queue.IsEmpty())

	// Verify status updated to Working
	updatedCoord, _ := processRepo.Get(repository.CoordinatorID)
	assert.Equal(t, repository.StatusWorking, updatedCoord.Status)

	// Verify result
	deliverResult := result.Data.(*handler.DeliverProcessQueuedResult)
	assert.True(t, deliverResult.Delivered)
	assert.Equal(t, "queued message", deliverResult.Content)
}

func TestDeliverProcessQueuedHandler_DeliverToWorker_DequeuesAndResumes(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()
	registry := process.NewProcessRegistry()

	// Create worker in Ready status
	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusReady,
		TaskID: "task-123",
	}
	processRepo.AddProcess(worker)

	// Queue a message
	queue := queueRepo.GetOrCreate("worker-1")
	_ = queue.Enqueue("worker instructions", repository.SenderUser)

	h := handler.NewDeliverProcessQueuedHandler(processRepo, queueRepo, registry)

	cmd := command.NewDeliverProcessQueuedCommand(command.SourceInternal, "worker-1")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify status updated
	updatedWorker, _ := processRepo.Get("worker-1")
	assert.Equal(t, repository.StatusWorking, updatedWorker.Status)
}

func TestDeliverProcessQueuedHandler_UpdatesStatusToWorking(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()
	registry := process.NewProcessRegistry()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusReady,
	}
	processRepo.AddProcess(worker)

	queue := queueRepo.GetOrCreate("worker-1")
	_ = queue.Enqueue("message", repository.SenderUser)

	h := handler.NewDeliverProcessQueuedHandler(processRepo, queueRepo, registry)

	cmd := command.NewDeliverProcessQueuedCommand(command.SourceInternal, "worker-1")
	_, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)

	updated, _ := processRepo.Get("worker-1")
	assert.Equal(t, repository.StatusWorking, updated.Status)
}

func TestDeliverProcessQueuedHandler_EmitsProcessWorkingEvent(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()
	registry := process.NewProcessRegistry()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusReady,
	}
	processRepo.AddProcess(worker)

	queue := queueRepo.GetOrCreate("worker-1")
	_ = queue.Enqueue("message", repository.SenderUser)

	h := handler.NewDeliverProcessQueuedHandler(processRepo, queueRepo, registry)

	cmd := command.NewDeliverProcessQueuedCommand(command.SourceInternal, "worker-1")
	result, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)

	// Find ProcessWorking event
	var foundWorking bool
	for _, evt := range result.Events {
		if pe, ok := evt.(events.ProcessEvent); ok && pe.Type == events.ProcessWorking {
			foundWorking = true
			assert.Equal(t, "worker-1", pe.ProcessID)
			assert.Equal(t, events.RoleWorker, pe.Role)
		}
	}
	assert.True(t, foundWorking, "ProcessWorking event not found")
}

func TestDeliverProcessQueuedHandler_EmitsProcessIncomingEvent(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()
	registry := process.NewProcessRegistry()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusReady,
	}
	processRepo.AddProcess(worker)

	queue := queueRepo.GetOrCreate("worker-1")
	_ = queue.Enqueue("the message content", repository.SenderUser)

	h := handler.NewDeliverProcessQueuedHandler(processRepo, queueRepo, registry)

	cmd := command.NewDeliverProcessQueuedCommand(command.SourceInternal, "worker-1")
	result, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)

	// Find ProcessIncoming event
	var foundIncoming bool
	for _, evt := range result.Events {
		if pe, ok := evt.(events.ProcessEvent); ok && pe.Type == events.ProcessIncoming {
			foundIncoming = true
			assert.Equal(t, "the message content", pe.Message)
		}
	}
	assert.True(t, foundIncoming, "ProcessIncoming event not found")
}

func TestDeliverProcessQueuedHandler_EmptyQueue_ReturnsError(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()
	registry := process.NewProcessRegistry()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusReady,
	}
	processRepo.AddProcess(worker)

	// Don't add any messages to queue

	h := handler.NewDeliverProcessQueuedHandler(processRepo, queueRepo, registry)

	cmd := command.NewDeliverProcessQueuedCommand(command.SourceInternal, "worker-1")
	result, err := h.Handle(context.Background(), cmd)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, handler.ErrQueueEmpty)
}

func TestDeliverProcessQueuedHandler_CallsResetTurnWhenDeliveringMessage(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()
	registry := process.NewProcessRegistry()
	enforcer := handler.NewTurnCompletionTracker()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusReady,
	}
	processRepo.AddProcess(worker)

	// Set up some state that should be cleared by ResetTurn
	enforcer.MarkAsNewlySpawned("worker-1")
	enforcer.RecordToolCall("worker-1", "post_message")
	enforcer.IncrementRetry("worker-1")

	// Verify state exists before delivery
	assert.True(t, enforcer.IsNewlySpawned("worker-1"))

	// Queue a message
	queue := queueRepo.GetOrCreate("worker-1")
	_ = queue.Enqueue("message", repository.SenderUser)

	h := handler.NewDeliverProcessQueuedHandler(processRepo, queueRepo, registry,
		handler.WithDeliverTurnEnforcer(enforcer))

	cmd := command.NewDeliverProcessQueuedCommand(command.SourceInternal, "worker-1")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify ResetTurn was called - all state should be cleared
	assert.False(t, enforcer.IsNewlySpawned("worker-1"),
		"newlySpawned flag should be cleared by ResetTurn")
	assert.True(t, enforcer.ShouldRetry("worker-1"),
		"retry count should be reset (ShouldRetry returns true when count is 0)")
}

func TestDeliverProcessQueuedHandler_WorksCorrectlyWhenEnforcerIsNil(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()
	registry := process.NewProcessRegistry()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusReady,
	}
	processRepo.AddProcess(worker)

	// Queue a message
	queue := queueRepo.GetOrCreate("worker-1")
	_ = queue.Enqueue("message", repository.SenderUser)

	// Create handler WITHOUT enforcer (default behavior)
	h := handler.NewDeliverProcessQueuedHandler(processRepo, queueRepo, registry)

	cmd := command.NewDeliverProcessQueuedCommand(command.SourceInternal, "worker-1")
	result, err := h.Handle(context.Background(), cmd)

	// Should not panic and should complete successfully
	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestDeliverProcessQueuedHandler_ResetTurnClearsNewlySpawnedFlagAfterFirstTurn(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()
	registry := process.NewProcessRegistry()
	enforcer := handler.NewTurnCompletionTracker()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusReady,
	}
	processRepo.AddProcess(worker)

	// Simulate worker being newly spawned (first turn after spawn)
	enforcer.MarkAsNewlySpawned("worker-1")
	assert.True(t, enforcer.IsNewlySpawned("worker-1"))

	// Queue a message for the first turn
	queue := queueRepo.GetOrCreate("worker-1")
	_ = queue.Enqueue("first turn message", repository.SenderUser)

	h := handler.NewDeliverProcessQueuedHandler(processRepo, queueRepo, registry,
		handler.WithDeliverTurnEnforcer(enforcer))

	cmd := command.NewDeliverProcessQueuedCommand(command.SourceInternal, "worker-1")
	_, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)

	// After delivery, newlySpawned flag should be cleared
	assert.False(t, enforcer.IsNewlySpawned("worker-1"),
		"newlySpawned flag should be cleared after first turn delivery")
}

func TestDeliverProcessQueuedHandler_ToolCallsFromPreviousTurnAreCleared(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()
	registry := process.NewProcessRegistry()
	enforcer := handler.NewTurnCompletionTracker()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusReady,
	}
	processRepo.AddProcess(worker)

	// Simulate tool calls from previous turn
	enforcer.RecordToolCall("worker-1", "post_message")
	enforcer.RecordToolCall("worker-1", "signal_ready")

	// Before delivery, tool calls exist - CheckTurnCompletion returns empty (compliant)
	missingBefore := enforcer.CheckTurnCompletion("worker-1", repository.RoleWorker)
	assert.Empty(t, missingBefore, "should be compliant with tool calls recorded")

	// Queue a message to start new turn
	queue := queueRepo.GetOrCreate("worker-1")
	_ = queue.Enqueue("new turn message", repository.SenderUser)

	h := handler.NewDeliverProcessQueuedHandler(processRepo, queueRepo, registry,
		handler.WithDeliverTurnEnforcer(enforcer))

	cmd := command.NewDeliverProcessQueuedCommand(command.SourceInternal, "worker-1")
	_, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)

	// After delivery (new turn started), tool calls should be cleared
	// CheckTurnCompletion should now return missing tools since no tools called this turn
	missingAfter := enforcer.CheckTurnCompletion("worker-1", repository.RoleWorker)
	assert.NotEmpty(t, missingAfter,
		"tool calls from previous turn should be cleared, making new turn non-compliant")
}

// ===========================================================================
// ProcessTurnCompleteHandler Tests
// ===========================================================================

func TestProcessTurnCompleteHandler_CoordinatorUpdatesStatusToReady(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()

	coord := &repository.Process{
		ID:     repository.CoordinatorID,
		Role:   repository.RoleCoordinator,
		Status: repository.StatusWorking,
	}
	processRepo.AddProcess(coord)

	h := handler.NewProcessTurnCompleteHandler(processRepo, queueRepo)

	cmd := command.NewProcessTurnCompleteCommand(repository.CoordinatorID, true, nil, nil)
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	updated, _ := processRepo.Get(repository.CoordinatorID)
	assert.Equal(t, repository.StatusReady, updated.Status)
}

func TestProcessTurnCompleteHandler_WorkerUpdatesStatusToReady(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusWorking,
	}
	processRepo.AddProcess(worker)

	h := handler.NewProcessTurnCompleteHandler(processRepo, queueRepo)

	cmd := command.NewProcessTurnCompleteCommand("worker-1", true, nil, nil)
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	updated, _ := processRepo.Get("worker-1")
	assert.Equal(t, repository.StatusReady, updated.Status)
}

func TestProcessTurnCompleteHandler_UpdatesLastActivityAt(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()

	before := time.Now().Add(-time.Hour)
	worker := &repository.Process{
		ID:             "worker-1",
		Role:           repository.RoleWorker,
		Status:         repository.StatusWorking,
		LastActivityAt: before,
	}
	processRepo.AddProcess(worker)

	h := handler.NewProcessTurnCompleteHandler(processRepo, queueRepo)

	cmd := command.NewProcessTurnCompleteCommand("worker-1", true, nil, nil)
	_, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)

	updated, _ := processRepo.Get("worker-1")
	assert.True(t, updated.LastActivityAt.After(before))
}

func TestProcessTurnCompleteHandler_UpdatesMetricsWhenProvided(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusWorking,
	}
	processRepo.AddProcess(worker)

	h := handler.NewProcessTurnCompleteHandler(processRepo, queueRepo)

	m := &metrics.TokenMetrics{
		TokensUsed:   1000,
		OutputTokens: 500,
	}
	cmd := command.NewProcessTurnCompleteCommand("worker-1", true, m, nil)
	_, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)

	updated, _ := processRepo.Get("worker-1")
	assert.NotNil(t, updated.Metrics)
	assert.Equal(t, 1000, updated.Metrics.TokensUsed)
	assert.Equal(t, 500, updated.Metrics.OutputTokens)
}

func TestProcessTurnCompleteHandler_EmitsProcessReadyEvent(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusWorking,
	}
	processRepo.AddProcess(worker)

	h := handler.NewProcessTurnCompleteHandler(processRepo, queueRepo)

	cmd := command.NewProcessTurnCompleteCommand("worker-1", true, nil, nil)
	result, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)

	var foundReady bool
	for _, evt := range result.Events {
		if pe, ok := evt.(events.ProcessEvent); ok && pe.Type == events.ProcessReady {
			foundReady = true
			assert.Equal(t, "worker-1", pe.ProcessID)
			assert.Equal(t, events.ProcessStatusReady, pe.Status)
		}
	}
	assert.True(t, foundReady)
}

// NOTE: ProcessTokenUsage is emitted by process.go when result events arrive,
// NOT by the handler. This avoids double-counting in session token tracking.
// See process.go publishTokenUsageEvent() for the single source of truth.

func TestProcessTurnCompleteHandler_ReturnsDeliverProcessQueuedCommandIfQueueNotEmpty(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusWorking,
	}
	processRepo.AddProcess(worker)

	// Add message to queue
	queue := queueRepo.GetOrCreate("worker-1")
	_ = queue.Enqueue("pending message", repository.SenderUser)

	h := handler.NewProcessTurnCompleteHandler(processRepo, queueRepo)

	cmd := command.NewProcessTurnCompleteCommand("worker-1", true, nil, nil)
	result, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)

	require.Len(t, result.FollowUp, 1)
	deliverCmd := result.FollowUp[0].(*command.DeliverProcessQueuedCommand)
	assert.Equal(t, "worker-1", deliverCmd.ProcessID)
}

func TestProcessTurnCompleteHandler_SucceededFalseTransitionsToFailed(t *testing.T) {
	// Tests that mid-session failures (after first successful turn) transition to Failed
	// and emit ProcessError. This catches resume failures, context exceeded, etc.
	processRepo, queueRepo := setupProcessRepos()

	worker := &repository.Process{
		ID:               "worker-1",
		Role:             repository.RoleWorker,
		Status:           repository.StatusWorking,
		HasCompletedTurn: true, // Already had a successful turn - this is a mid-session failure
	}
	processRepo.AddProcess(worker)

	h := handler.NewProcessTurnCompleteHandler(processRepo, queueRepo)

	// succeeded=false (mid-session failure)
	testErr := fmt.Errorf("resume failed: session not found")
	cmd := command.NewProcessTurnCompleteCommand("worker-1", false, nil, testErr)
	result, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)
	assert.True(t, result.Success)

	// Should transition to Failed
	turnResult := result.Data.(*handler.ProcessTurnCompleteResult)
	assert.Equal(t, repository.StatusFailed, turnResult.NewStatus)

	// Verify process is Failed in repo
	updated, _ := processRepo.Get("worker-1")
	assert.Equal(t, repository.StatusFailed, updated.Status)

	// Verify ProcessError event is emitted
	require.NotEmpty(t, result.Events)
	foundError := false
	for _, ev := range result.Events {
		if pe, ok := ev.(events.ProcessEvent); ok {
			if pe.Type == events.ProcessError {
				foundError = true
				assert.Equal(t, events.ProcessStatusReady, pe.Status)
				assert.NotNil(t, pe.Error)
			}
		}
	}
	assert.True(t, foundError, "ProcessError event should be emitted for mid-session failure")
}

func TestProcessTurnCompleteHandler_StartupFailure_TransitionsToFailed(t *testing.T) {
	// Tests that first turn failures (before any success) transition to Failed
	// and emit ProcessError instead of ProcessReady.
	processRepo, queueRepo := setupProcessRepos()

	worker := &repository.Process{
		ID:               "worker-1",
		Role:             repository.RoleWorker,
		Status:           repository.StatusWorking,
		HasCompletedTurn: false, // First turn - never succeeded
	}
	processRepo.AddProcess(worker)

	h := handler.NewProcessTurnCompleteHandler(processRepo, queueRepo)

	// First turn failed with error
	testErr := fmt.Errorf("claude process failed: Error: Input must be provided")
	cmd := command.NewProcessTurnCompleteCommand("worker-1", false, nil, testErr)
	result, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)
	assert.True(t, result.Success)

	// Should transition to Failed (not Ready)
	turnResult := result.Data.(*handler.ProcessTurnCompleteResult)
	assert.Equal(t, repository.StatusFailed, turnResult.NewStatus)

	// Verify process is Failed in repo
	updated, _ := processRepo.Get("worker-1")
	assert.Equal(t, repository.StatusFailed, updated.Status)
	assert.False(t, updated.HasCompletedTurn, "should remain false since never succeeded")

	// Verify ProcessError event is emitted (not ProcessReady)
	require.NotEmpty(t, result.Events)
	foundError := false
	foundReady := false
	for _, e := range result.Events {
		if pe, ok := e.(events.ProcessEvent); ok {
			if pe.Type == events.ProcessError {
				foundError = true
				assert.Equal(t, testErr, pe.Error)
			}
			if pe.Type == events.ProcessReady {
				foundReady = true
			}
		}
	}
	assert.True(t, foundError, "ProcessError event should be emitted")
	assert.False(t, foundReady, "ProcessReady event should NOT be emitted for startup failures")
}

func TestProcessTurnCompleteHandler_CoordinatorStartupFailure_TransitionsToFailed(t *testing.T) {
	// Tests coordinator-specific startup failure handling
	processRepo, queueRepo := setupProcessRepos()

	coord := &repository.Process{
		ID:               "coordinator",
		Role:             repository.RoleCoordinator,
		Status:           repository.StatusWorking,
		HasCompletedTurn: false, // First turn
	}
	processRepo.AddProcess(coord)

	h := handler.NewProcessTurnCompleteHandler(processRepo, queueRepo)

	// Coordinator first turn failed
	testErr := fmt.Errorf("claude process failed: Error: stdin required")
	cmd := command.NewProcessTurnCompleteCommand("coordinator", false, nil, testErr)
	result, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)
	assert.True(t, result.Success)

	// Should transition to Failed
	turnResult := result.Data.(*handler.ProcessTurnCompleteResult)
	assert.Equal(t, repository.StatusFailed, turnResult.NewStatus)

	// Verify ProcessError is emitted with the actual error
	foundError := false
	for _, e := range result.Events {
		if pe, ok := e.(events.ProcessEvent); ok {
			if pe.Type == events.ProcessError {
				foundError = true
				assert.Equal(t, testErr, pe.Error)
				assert.Equal(t, events.ProcessStatusFailed, pe.Status)
			}
		}
	}
	assert.True(t, foundError, "ProcessError should be emitted for coordinator startup failure")
}

// ===========================================================================
// ProcessTurnCompleteHandler Context Exceeded Tests
// ===========================================================================

func TestProcessTurnCompleteHandler_WorkerContextExceeded_NotifiesCoordinator(t *testing.T) {
	// Tests that when a worker runs out of context, the handler:
	// 1. Transitions the worker to Failed status
	// 2. Sends a message to the coordinator to replace the worker
	// 3. Returns a DeliverProcessQueuedCommand to deliver the message
	processRepo, queueRepo := setupProcessRepos()

	// Create coordinator
	coord := &repository.Process{
		ID:     repository.CoordinatorID,
		Role:   repository.RoleCoordinator,
		Status: repository.StatusReady,
	}
	processRepo.AddProcess(coord)

	// Create worker with a task
	worker := &repository.Process{
		ID:               "worker-1",
		Role:             repository.RoleWorker,
		Status:           repository.StatusWorking,
		TaskID:           "task-123",
		HasCompletedTurn: true,
	}
	processRepo.AddProcess(worker)

	h := handler.NewProcessTurnCompleteHandler(processRepo, queueRepo)

	// Simulate context exceeded error
	contextErr := &process.ContextExceededError{}
	cmd := command.NewProcessTurnCompleteCommand("worker-1", false, nil, contextErr)
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify worker transitioned to Failed
	turnResult := result.Data.(*handler.ProcessTurnCompleteResult)
	assert.Equal(t, repository.StatusFailed, turnResult.NewStatus)
	assert.True(t, turnResult.QueuedDelivery, "should queue delivery to coordinator")

	// Verify worker is Failed in repo
	updated, _ := processRepo.Get("worker-1")
	assert.Equal(t, repository.StatusFailed, updated.Status)

	// Verify message was enqueued to coordinator
	coordQueue := queueRepo.GetOrCreate(repository.CoordinatorID)
	require.False(t, coordQueue.IsEmpty(), "coordinator queue should have message")
	entry, _ := coordQueue.Dequeue()
	assert.Contains(t, entry.Content, "WORKER CONTEXT EXHAUSTED")
	assert.Contains(t, entry.Content, "worker-1")
	assert.Contains(t, entry.Content, "replace_worker")
	assert.Contains(t, entry.Content, "task-123") // Should mention the task

	// Verify follow-up command to deliver to coordinator
	require.Len(t, result.FollowUp, 1)
	deliverCmd, ok := result.FollowUp[0].(*command.DeliverProcessQueuedCommand)
	require.True(t, ok)
	assert.Equal(t, repository.CoordinatorID, deliverCmd.ProcessID)
}

func TestProcessTurnCompleteHandler_WorkerContextExceeded_NoTask(t *testing.T) {
	// Tests context exceeded for worker without assigned task
	processRepo, queueRepo := setupProcessRepos()

	// Create coordinator
	coord := &repository.Process{
		ID:     repository.CoordinatorID,
		Role:   repository.RoleCoordinator,
		Status: repository.StatusReady,
	}
	processRepo.AddProcess(coord)

	// Create worker without a task
	worker := &repository.Process{
		ID:               "worker-2",
		Role:             repository.RoleWorker,
		Status:           repository.StatusWorking,
		TaskID:           "", // No task assigned
		HasCompletedTurn: true,
	}
	processRepo.AddProcess(worker)

	h := handler.NewProcessTurnCompleteHandler(processRepo, queueRepo)

	contextErr := &process.ContextExceededError{}
	cmd := command.NewProcessTurnCompleteCommand("worker-2", false, nil, contextErr)
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)

	// Verify message mentions no assigned task
	coordQueue := queueRepo.GetOrCreate(repository.CoordinatorID)
	require.False(t, coordQueue.IsEmpty())
	entry, _ := coordQueue.Dequeue()
	assert.Contains(t, entry.Content, "worker-2")
	assert.Contains(t, entry.Content, "no assigned task")

	// Should still have follow-up command
	require.Len(t, result.FollowUp, 1)
}

func TestProcessTurnCompleteHandler_CoordinatorContextExceeded_NotHandledAsWorker(t *testing.T) {
	// Tests that context exceeded for coordinator is NOT handled by the special
	// worker context exceeded logic (only workers get this special handling)
	processRepo, queueRepo := setupProcessRepos()

	coord := &repository.Process{
		ID:               repository.CoordinatorID,
		Role:             repository.RoleCoordinator,
		Status:           repository.StatusWorking,
		HasCompletedTurn: true,
	}
	processRepo.AddProcess(coord)

	h := handler.NewProcessTurnCompleteHandler(processRepo, queueRepo)

	// Coordinator with context exceeded error
	contextErr := &process.ContextExceededError{}
	cmd := command.NewProcessTurnCompleteCommand(repository.CoordinatorID, false, nil, contextErr)
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)

	// Should be handled as a normal mid-session failure, not the special worker path
	turnResult := result.Data.(*handler.ProcessTurnCompleteResult)
	assert.Equal(t, repository.StatusFailed, turnResult.NewStatus)

	// Should NOT queue message to coordinator (can't message itself about context exhaustion)
	coordQueue := queueRepo.GetOrCreate(repository.CoordinatorID)
	assert.True(t, coordQueue.IsEmpty(), "coordinator queue should be empty")
}

func TestProcessTurnCompleteHandler_WorkerContextExceeded_TraceIDPropagated(t *testing.T) {
	// Tests that trace ID is propagated to follow-up command
	processRepo, queueRepo := setupProcessRepos()

	coord := &repository.Process{
		ID:     repository.CoordinatorID,
		Role:   repository.RoleCoordinator,
		Status: repository.StatusReady,
	}
	processRepo.AddProcess(coord)

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusWorking,
	}
	processRepo.AddProcess(worker)

	h := handler.NewProcessTurnCompleteHandler(processRepo, queueRepo)

	contextErr := &process.ContextExceededError{}
	cmd := command.NewProcessTurnCompleteCommand("worker-1", false, nil, contextErr)
	cmd.SetTraceID("trace-123")

	result, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)

	// Verify trace ID is propagated
	require.Len(t, result.FollowUp, 1)
	deliverCmd := result.FollowUp[0].(*command.DeliverProcessQueuedCommand)
	assert.Equal(t, "trace-123", deliverCmd.TraceID())
}

// ===========================================================================
// ProcessTurnCompleteHandler Turn Enforcement Tests
// ===========================================================================

func TestProcessTurnCompleteHandler_WorkerWithoutRequiredToolCall_GetsReminder(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()
	enforcer := handler.NewTurnCompletionTracker()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusWorking,
	}
	processRepo.AddProcess(worker)

	// NO tool calls recorded - worker didn't call post_message or report_implementation_complete

	h := handler.NewProcessTurnCompleteHandler(processRepo, queueRepo,
		handler.WithProcessTurnEnforcer(enforcer))

	cmd := command.NewProcessTurnCompleteCommand("worker-1", true, nil, nil)
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify enforcement was triggered
	turnResult := result.Data.(*handler.ProcessTurnCompleteResult)
	assert.True(t, turnResult.EnforcementTriggered, "enforcement should be triggered")
	assert.Equal(t, repository.StatusReady, turnResult.NewStatus, "should transition to Ready for delivery")
	assert.True(t, turnResult.QueuedDelivery, "should have queued delivery")

	// Verify follow-up DeliverProcessQueuedCommand to deliver the reminder
	require.Len(t, result.FollowUp, 1, "should have follow-up command")
	deliverCmd, ok := result.FollowUp[0].(*command.DeliverProcessQueuedCommand)
	require.True(t, ok, "follow-up should be DeliverProcessQueuedCommand")
	assert.Equal(t, "worker-1", deliverCmd.ProcessID)

	// Verify reminder was enqueued
	queue := queueRepo.GetOrCreate("worker-1")
	require.False(t, queue.IsEmpty(), "queue should contain reminder")
	entry, _ := queue.Dequeue()
	assert.Contains(t, entry.Content, "SYSTEM REMINDER")
	assert.Contains(t, entry.Content, "post_message")
	assert.Equal(t, repository.SenderSystem, entry.Sender)

	// Verify ProcessReady event was NOT emitted (to avoid UI confusion)
	for _, evt := range result.Events {
		if pe, ok := evt.(events.ProcessEvent); ok {
			assert.NotEqual(t, events.ProcessReady, pe.Type, "ProcessReady should not be emitted during enforcement")
		}
	}
}

func TestProcessTurnCompleteHandler_WorkerWithRequiredToolCall_NoReminder(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()
	enforcer := handler.NewTurnCompletionTracker()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusWorking,
	}
	processRepo.AddProcess(worker)

	// Worker called post_message during turn
	enforcer.RecordToolCall("worker-1", "post_message")

	h := handler.NewProcessTurnCompleteHandler(processRepo, queueRepo,
		handler.WithProcessTurnEnforcer(enforcer))

	cmd := command.NewProcessTurnCompleteCommand("worker-1", true, nil, nil)
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify enforcement was NOT triggered
	turnResult := result.Data.(*handler.ProcessTurnCompleteResult)
	assert.False(t, turnResult.EnforcementTriggered, "enforcement should NOT be triggered")
	assert.Equal(t, repository.StatusReady, turnResult.NewStatus, "should transition to Ready")

	// Verify ProcessReady event was emitted
	var foundReady bool
	for _, evt := range result.Events {
		if pe, ok := evt.(events.ProcessEvent); ok && pe.Type == events.ProcessReady {
			foundReady = true
		}
	}
	assert.True(t, foundReady, "ProcessReady event should be emitted")
}

func TestProcessTurnCompleteHandler_CoordinatorsNeverGetEnforcement(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()
	enforcer := handler.NewTurnCompletionTracker()

	coord := &repository.Process{
		ID:     repository.CoordinatorID,
		Role:   repository.RoleCoordinator,
		Status: repository.StatusWorking,
	}
	processRepo.AddProcess(coord)

	// Coordinator didn't call any tools - should NOT trigger enforcement

	h := handler.NewProcessTurnCompleteHandler(processRepo, queueRepo,
		handler.WithProcessTurnEnforcer(enforcer))

	cmd := command.NewProcessTurnCompleteCommand(repository.CoordinatorID, true, nil, nil)
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify enforcement was NOT triggered
	turnResult := result.Data.(*handler.ProcessTurnCompleteResult)
	assert.False(t, turnResult.EnforcementTriggered, "coordinators should never get enforcement")
	assert.Equal(t, repository.StatusReady, turnResult.NewStatus)

	// Verify ProcessReady event was emitted
	var foundReady bool
	for _, evt := range result.Events {
		if pe, ok := evt.(events.ProcessEvent); ok && pe.Type == events.ProcessReady {
			foundReady = true
		}
	}
	assert.True(t, foundReady, "ProcessReady should be emitted for coordinator")
}

func TestProcessTurnCompleteHandler_FailedTurns_SkipEnforcement(t *testing.T) {
	// Tests that mid-session failures skip enforcement and go to Failed.
	// Note: Startup failures (!HasCompletedTurn) go to Failed before enforcement runs.
	processRepo, queueRepo := setupProcessRepos()
	enforcer := handler.NewTurnCompletionTracker()

	worker := &repository.Process{
		ID:               "worker-1",
		Role:             repository.RoleWorker,
		Status:           repository.StatusWorking,
		HasCompletedTurn: true, // Mid-session failure (already had successful turn)
	}
	processRepo.AddProcess(worker)

	// Worker didn't call any tools but turn FAILED (mid-session)

	h := handler.NewProcessTurnCompleteHandler(processRepo, queueRepo,
		handler.WithProcessTurnEnforcer(enforcer))

	// succeeded=false means the turn failed (crash, error, context exceeded)
	cmd := command.NewProcessTurnCompleteCommand("worker-1", false, nil, nil)
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify enforcement was NOT triggered for failed turns (goes to Failed before enforcement)
	turnResult := result.Data.(*handler.ProcessTurnCompleteResult)
	assert.False(t, turnResult.EnforcementTriggered, "failed turns should skip enforcement")
	assert.Equal(t, repository.StatusFailed, turnResult.NewStatus)
}

func TestProcessTurnCompleteHandler_StartupTurns_SkipEnforcement(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()
	enforcer := handler.NewTurnCompletionTracker()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusWorking,
	}
	processRepo.AddProcess(worker)

	// Mark as newly spawned - first turn after spawn
	enforcer.MarkAsNewlySpawned("worker-1")

	// Worker didn't call required tools but this is startup turn

	h := handler.NewProcessTurnCompleteHandler(processRepo, queueRepo,
		handler.WithProcessTurnEnforcer(enforcer))

	cmd := command.NewProcessTurnCompleteCommand("worker-1", true, nil, nil)
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify enforcement was NOT triggered for startup turns
	turnResult := result.Data.(*handler.ProcessTurnCompleteResult)
	assert.False(t, turnResult.EnforcementTriggered, "startup turns should skip enforcement")
	assert.Equal(t, repository.StatusReady, turnResult.NewStatus)
}

func TestProcessTurnCompleteHandler_MaxRetriesThenOnMaxRetriesExceededCalled(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()

	// Track if OnMaxRetriesExceeded was called
	var maxRetriesExceededCalled bool
	var maxRetriesProcessID string
	var maxRetriesMissingTools []string

	enforcer := handler.NewTurnCompletionTrackerWithOptions(
		handler.WithLogger(func(format string, args ...any) {
			// This is called by OnMaxRetriesExceeded
			maxRetriesExceededCalled = true
			if len(args) >= 2 {
				maxRetriesProcessID, _ = args[0].(string)
				maxRetriesMissingTools, _ = args[1].([]string)
			}
		}),
	)

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusWorking,
	}
	processRepo.AddProcess(worker)

	h := handler.NewProcessTurnCompleteHandler(processRepo, queueRepo,
		handler.WithProcessTurnEnforcer(enforcer))

	// First turn without tools - should get reminder (retry 1)
	cmd1 := command.NewProcessTurnCompleteCommand("worker-1", true, nil, nil)
	result1, err := h.Handle(context.Background(), cmd1)
	require.NoError(t, err)
	assert.True(t, result1.Data.(*handler.ProcessTurnCompleteResult).EnforcementTriggered)
	assert.False(t, maxRetriesExceededCalled, "should not exceed max retries yet")

	// Second turn without tools - should get reminder (retry 2)
	cmd2 := command.NewProcessTurnCompleteCommand("worker-1", true, nil, nil)
	result2, err := h.Handle(context.Background(), cmd2)
	require.NoError(t, err)
	assert.True(t, result2.Data.(*handler.ProcessTurnCompleteResult).EnforcementTriggered)
	assert.False(t, maxRetriesExceededCalled, "should not exceed max retries yet")

	// Third turn without tools - max retries exceeded, should complete normally
	cmd3 := command.NewProcessTurnCompleteCommand("worker-1", true, nil, nil)
	result3, err := h.Handle(context.Background(), cmd3)
	require.NoError(t, err)

	// Verify max retries exceeded was called and turn completed
	turnResult := result3.Data.(*handler.ProcessTurnCompleteResult)
	assert.False(t, turnResult.EnforcementTriggered, "should not trigger enforcement after max retries")
	assert.Equal(t, repository.StatusReady, turnResult.NewStatus, "should transition to Ready")
	assert.True(t, maxRetriesExceededCalled, "OnMaxRetriesExceeded should be called")
	assert.Equal(t, "worker-1", maxRetriesProcessID)
	assert.NotEmpty(t, maxRetriesMissingTools)
}

func TestProcessTurnCompleteHandler_AfterMaxRetries_TurnCompletesWithReadyEvent(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()
	enforcer := handler.NewTurnCompletionTracker()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusWorking,
	}
	processRepo.AddProcess(worker)

	h := handler.NewProcessTurnCompleteHandler(processRepo, queueRepo,
		handler.WithProcessTurnEnforcer(enforcer))

	// Exhaust retries (max is 2)
	for i := 0; i < 2; i++ {
		cmd := command.NewProcessTurnCompleteCommand("worker-1", true, nil, nil)
		_, err := h.Handle(context.Background(), cmd)
		require.NoError(t, err)
	}

	// Third turn - should complete normally
	cmd := command.NewProcessTurnCompleteCommand("worker-1", true, nil, nil)
	result, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)

	// Verify turn completed with Ready event
	var foundReady bool
	for _, evt := range result.Events {
		if pe, ok := evt.(events.ProcessEvent); ok && pe.Type == events.ProcessReady {
			foundReady = true
		}
	}
	assert.True(t, foundReady, "ProcessReady event should be emitted after max retries")
}

func TestProcessTurnCompleteHandler_ReminderMessageIncludesMissingToolNames(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()
	enforcer := handler.NewTurnCompletionTracker()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusWorking,
	}
	processRepo.AddProcess(worker)

	h := handler.NewProcessTurnCompleteHandler(processRepo, queueRepo,
		handler.WithProcessTurnEnforcer(enforcer))

	cmd := command.NewProcessTurnCompleteCommand("worker-1", true, nil, nil)
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	require.Len(t, result.FollowUp, 1)

	// Verify follow-up is DeliverProcessQueuedCommand
	_, ok := result.FollowUp[0].(*command.DeliverProcessQueuedCommand)
	require.True(t, ok, "follow-up should be DeliverProcessQueuedCommand")

	// Get the reminder from the queue and verify it includes all the required tool names
	queue := queueRepo.GetOrCreate("worker-1")
	entry, _ := queue.Dequeue()
	assert.Contains(t, entry.Content, "post_message")
	assert.Contains(t, entry.Content, "report_implementation_complete")
	assert.Contains(t, entry.Content, "report_review_verdict")
	assert.Contains(t, entry.Content, "signal_ready")
}

func TestProcessTurnCompleteHandler_WorksCorrectlyWhenEnforcerIsNil(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusWorking,
	}
	processRepo.AddProcess(worker)

	// Create handler WITHOUT enforcer (default behavior)
	h := handler.NewProcessTurnCompleteHandler(processRepo, queueRepo)

	cmd := command.NewProcessTurnCompleteCommand("worker-1", true, nil, nil)
	result, err := h.Handle(context.Background(), cmd)

	// Should not panic and should complete successfully
	require.NoError(t, err)
	assert.True(t, result.Success)

	turnResult := result.Data.(*handler.ProcessTurnCompleteResult)
	assert.Equal(t, repository.StatusReady, turnResult.NewStatus)
	assert.False(t, turnResult.EnforcementTriggered)
}

func TestProcessTurnCompleteHandler_EnforcementWithSignalReadyTool_NoReminder(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()
	enforcer := handler.NewTurnCompletionTracker()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusWorking,
	}
	processRepo.AddProcess(worker)

	// Worker called signal_ready (one of the required tools)
	enforcer.RecordToolCall("worker-1", "signal_ready")

	h := handler.NewProcessTurnCompleteHandler(processRepo, queueRepo,
		handler.WithProcessTurnEnforcer(enforcer))

	cmd := command.NewProcessTurnCompleteCommand("worker-1", true, nil, nil)
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)

	// Verify no enforcement triggered - signal_ready satisfies requirement
	turnResult := result.Data.(*handler.ProcessTurnCompleteResult)
	assert.False(t, turnResult.EnforcementTriggered)
	assert.Equal(t, repository.StatusReady, turnResult.NewStatus)
}

func TestProcessTurnCompleteHandler_EnforcementWithReportReviewVerdict_NoReminder(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()
	enforcer := handler.NewTurnCompletionTracker()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusWorking,
	}
	processRepo.AddProcess(worker)

	// Worker called report_review_verdict (one of the required tools)
	enforcer.RecordToolCall("worker-1", "report_review_verdict")

	h := handler.NewProcessTurnCompleteHandler(processRepo, queueRepo,
		handler.WithProcessTurnEnforcer(enforcer))

	cmd := command.NewProcessTurnCompleteCommand("worker-1", true, nil, nil)
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)

	// Verify no enforcement triggered
	turnResult := result.Data.(*handler.ProcessTurnCompleteResult)
	assert.False(t, turnResult.EnforcementTriggered)
	assert.Equal(t, repository.StatusReady, turnResult.NewStatus)
}

func TestProcessTurnCompleteHandler_EnforcementTransitionsToReadyForDelivery(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()
	enforcer := handler.NewTurnCompletionTracker()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusWorking,
	}
	processRepo.AddProcess(worker)

	h := handler.NewProcessTurnCompleteHandler(processRepo, queueRepo,
		handler.WithProcessTurnEnforcer(enforcer))

	cmd := command.NewProcessTurnCompleteCommand("worker-1", true, nil, nil)
	result, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)

	// Verify process transitions to Ready so DeliverProcessQueuedCommand can deliver the reminder
	updated, _ := processRepo.Get("worker-1")
	assert.Equal(t, repository.StatusReady, updated.Status,
		"process should transition to Ready for reminder delivery")

	// Verify a DeliverProcessQueuedCommand follow-up is returned
	require.Len(t, result.FollowUp, 1)
	_, ok := result.FollowUp[0].(*command.DeliverProcessQueuedCommand)
	assert.True(t, ok, "follow-up should be DeliverProcessQueuedCommand")
}

func TestProcessTurnCompleteHandler_EnforcementPreservesTraceID(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()
	enforcer := handler.NewTurnCompletionTracker()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusWorking,
	}
	processRepo.AddProcess(worker)

	h := handler.NewProcessTurnCompleteHandler(processRepo, queueRepo,
		handler.WithProcessTurnEnforcer(enforcer))

	cmd := command.NewProcessTurnCompleteCommand("worker-1", true, nil, nil)
	cmd.SetTraceID("test-trace-123")

	result, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)

	// Verify the follow-up command has the trace ID
	require.Len(t, result.FollowUp, 1)
	deliverCmd := result.FollowUp[0].(*command.DeliverProcessQueuedCommand)
	assert.Equal(t, "test-trace-123", deliverCmd.TraceID())
}

// ===========================================================================
// ProcessTurnCompleteHandler Session Ref Capture Tests
// ===========================================================================

// mockSessionRefNotifier is a test implementation of SessionRefNotifier.
type mockSessionRefNotifier struct {
	calls   []sessionRefCall
	callErr error
}

type sessionRefCall struct {
	ProcessID  string
	SessionRef string
	WorkDir    string
}

func (m *mockSessionRefNotifier) NotifySessionRef(processID, sessionRef, workDir string) error {
	m.calls = append(m.calls, sessionRefCall{processID, sessionRef, workDir})
	return m.callErr
}

func TestProcessTurnCompleteHandler_FirstSuccessfulTurn_CapturesSessionRef(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()
	registry := process.NewProcessRegistry()
	notifier := &mockSessionRefNotifier{}

	// Create a coordinator in Working status, not yet completed first turn
	coord := &repository.Process{
		ID:               repository.CoordinatorID,
		Role:             repository.RoleCoordinator,
		Status:           repository.StatusWorking,
		HasCompletedTurn: false,
	}
	processRepo.AddProcess(coord)

	// Create a live process and set session ID via init event (normal flow)
	mockProc := newMockHeadlessProcess(12345)
	liveProcess := process.New(repository.CoordinatorID, repository.RoleCoordinator, mockProc, nil, nil)
	liveProcess.Start()
	mockProc.SendInitEvent("session-xyz-123")
	time.Sleep(10 * time.Millisecond) // Allow event to be processed
	registry.Register(liveProcess)

	h := handler.NewProcessTurnCompleteHandler(processRepo, queueRepo,
		handler.WithTurnCompleteProcessRegistry(registry),
		handler.WithSessionRefNotifier(notifier))

	cmd := command.NewProcessTurnCompleteCommand(repository.CoordinatorID, true, nil, nil)
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify notifier was called with correct parameters
	require.Len(t, notifier.calls, 1)
	assert.Equal(t, repository.CoordinatorID, notifier.calls[0].ProcessID)
	assert.Equal(t, "session-xyz-123", notifier.calls[0].SessionRef)
	assert.Equal(t, "/test", notifier.calls[0].WorkDir) // From mockHeadlessProcess.WorkDir()

	// Verify repository entity has SessionID set
	updated, _ := processRepo.Get(repository.CoordinatorID)
	assert.Equal(t, "session-xyz-123", updated.SessionID)
}

func TestProcessTurnCompleteHandler_SecondTurn_DoesNotRecaptureSessionRef(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()
	registry := process.NewProcessRegistry()
	notifier := &mockSessionRefNotifier{}

	// Create a worker that already completed first turn
	worker := &repository.Process{
		ID:               "worker-1",
		Role:             repository.RoleWorker,
		Status:           repository.StatusWorking,
		HasCompletedTurn: true, // Already completed first turn
		SessionID:        "previous-session",
	}
	processRepo.AddProcess(worker)

	// Create a live process and set session ID via init event (simulating a new session after refresh)
	mockProc := newMockHeadlessProcess(12345)
	liveProcess := process.New("worker-1", repository.RoleWorker, mockProc, nil, nil)
	liveProcess.Start()
	mockProc.SendInitEvent("new-session")
	time.Sleep(10 * time.Millisecond) // Allow event to be processed
	registry.Register(liveProcess)

	h := handler.NewProcessTurnCompleteHandler(processRepo, queueRepo,
		handler.WithTurnCompleteProcessRegistry(registry),
		handler.WithSessionRefNotifier(notifier))

	cmd := command.NewProcessTurnCompleteCommand("worker-1", true, nil, nil)
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify notifier was NOT called (second turn should not recapture)
	assert.Len(t, notifier.calls, 0)

	// Verify repository entity still has the original SessionID
	updated, _ := processRepo.Get("worker-1")
	assert.Equal(t, "previous-session", updated.SessionID)
}

func TestProcessTurnCompleteHandler_FailedFirstTurn_DoesNotCaptureSessionRef(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()
	registry := process.NewProcessRegistry()
	notifier := &mockSessionRefNotifier{}

	// Create a worker that has not completed first turn
	worker := &repository.Process{
		ID:               "worker-1",
		Role:             repository.RoleWorker,
		Status:           repository.StatusWorking,
		HasCompletedTurn: false,
	}
	processRepo.AddProcess(worker)

	// Create a live process and set session ID via init event
	mockProc := newMockHeadlessProcess(12345)
	liveProcess := process.New("worker-1", repository.RoleWorker, mockProc, nil, nil)
	liveProcess.Start()
	mockProc.SendInitEvent("some-session")
	time.Sleep(10 * time.Millisecond) // Allow event to be processed
	registry.Register(liveProcess)

	h := handler.NewProcessTurnCompleteHandler(processRepo, queueRepo,
		handler.WithTurnCompleteProcessRegistry(registry),
		handler.WithSessionRefNotifier(notifier))

	// Failed first turn - this should transition to Failed status (startup failure)
	cmd := command.NewProcessTurnCompleteCommand("worker-1", false, nil, nil)
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify notifier was NOT called (failed turn should not capture)
	assert.Len(t, notifier.calls, 0)

	// Verify process transitioned to Failed
	updated, _ := processRepo.Get("worker-1")
	assert.Equal(t, repository.StatusFailed, updated.Status)
	assert.Empty(t, updated.SessionID)
}

func TestProcessTurnCompleteHandler_MissingRegistry_GracefullySkipsCapture(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()
	notifier := &mockSessionRefNotifier{}
	// No registry provided

	coord := &repository.Process{
		ID:               repository.CoordinatorID,
		Role:             repository.RoleCoordinator,
		Status:           repository.StatusWorking,
		HasCompletedTurn: false,
	}
	processRepo.AddProcess(coord)

	h := handler.NewProcessTurnCompleteHandler(processRepo, queueRepo,
		// Note: WithTurnCompleteProcessRegistry NOT called
		handler.WithSessionRefNotifier(notifier))

	cmd := command.NewProcessTurnCompleteCommand(repository.CoordinatorID, true, nil, nil)
	result, err := h.Handle(context.Background(), cmd)

	// Should succeed without panic
	require.NoError(t, err)
	assert.True(t, result.Success)

	// Notifier should not be called (no registry to get process from)
	assert.Len(t, notifier.calls, 0)

	// Process should still transition to Ready
	updated, _ := processRepo.Get(repository.CoordinatorID)
	assert.Equal(t, repository.StatusReady, updated.Status)
}

func TestProcessTurnCompleteHandler_MissingNotifier_GracefullySkipsCapture(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()
	registry := process.NewProcessRegistry()
	// No notifier provided

	coord := &repository.Process{
		ID:               repository.CoordinatorID,
		Role:             repository.RoleCoordinator,
		Status:           repository.StatusWorking,
		HasCompletedTurn: false,
	}
	processRepo.AddProcess(coord)

	// Create a live process and set session ID via init event
	mockProc := newMockHeadlessProcess(12345)
	liveProcess := process.New(repository.CoordinatorID, repository.RoleCoordinator, mockProc, nil, nil)
	liveProcess.Start()
	mockProc.SendInitEvent("session-abc")
	time.Sleep(10 * time.Millisecond) // Allow event to be processed
	registry.Register(liveProcess)

	h := handler.NewProcessTurnCompleteHandler(processRepo, queueRepo,
		handler.WithTurnCompleteProcessRegistry(registry))
	// Note: WithSessionRefNotifier NOT called

	cmd := command.NewProcessTurnCompleteCommand(repository.CoordinatorID, true, nil, nil)
	result, err := h.Handle(context.Background(), cmd)

	// Should succeed without panic
	require.NoError(t, err)
	assert.True(t, result.Success)

	// Repository entity should still have SessionID set (even if notifier not called)
	updated, _ := processRepo.Get(repository.CoordinatorID)
	assert.Equal(t, "session-abc", updated.SessionID)
}

func TestProcessTurnCompleteHandler_EmptySessionRef_DoesNotCallNotifier(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()
	registry := process.NewProcessRegistry()
	notifier := &mockSessionRefNotifier{}

	coord := &repository.Process{
		ID:               repository.CoordinatorID,
		Role:             repository.RoleCoordinator,
		Status:           repository.StatusWorking,
		HasCompletedTurn: false,
	}
	processRepo.AddProcess(coord)

	// Create a live process WITHOUT setting a session ID (empty string)
	mockProc := newMockHeadlessProcess(12345)
	liveProcess := process.New(repository.CoordinatorID, repository.RoleCoordinator, mockProc, nil, nil)
	// Note: NOT calling SetSessionIDForTest - session ID remains empty
	registry.Register(liveProcess)

	h := handler.NewProcessTurnCompleteHandler(processRepo, queueRepo,
		handler.WithTurnCompleteProcessRegistry(registry),
		handler.WithSessionRefNotifier(notifier))

	cmd := command.NewProcessTurnCompleteCommand(repository.CoordinatorID, true, nil, nil)
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Notifier should NOT be called (empty session ref)
	assert.Len(t, notifier.calls, 0)

	// Repository SessionID should remain empty
	updated, _ := processRepo.Get(repository.CoordinatorID)
	assert.Empty(t, updated.SessionID)
}

func TestProcessTurnCompleteHandler_RepositorySessionID_SetOnFirstSuccessfulTurn(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()
	registry := process.NewProcessRegistry()
	// No notifier - just testing repository update

	worker := &repository.Process{
		ID:               "worker-1",
		Role:             repository.RoleWorker,
		Status:           repository.StatusWorking,
		HasCompletedTurn: false,
	}
	processRepo.AddProcess(worker)

	// Create a live process and set session ID via init event
	mockProc := newMockHeadlessProcess(12345)
	liveProcess := process.New("worker-1", repository.RoleWorker, mockProc, nil, nil)
	liveProcess.Start()
	mockProc.SendInitEvent("repo-session-test")
	time.Sleep(10 * time.Millisecond) // Allow event to be processed
	registry.Register(liveProcess)

	h := handler.NewProcessTurnCompleteHandler(processRepo, queueRepo,
		handler.WithTurnCompleteProcessRegistry(registry))

	cmd := command.NewProcessTurnCompleteCommand("worker-1", true, nil, nil)
	_, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)

	// Verify repository Process.SessionID is set
	updated, _ := processRepo.Get("worker-1")
	assert.Equal(t, "repo-session-test", updated.SessionID)
	assert.True(t, updated.HasCompletedTurn)
}

// ===========================================================================
// RetireProcessHandler Tests
// ===========================================================================

func TestRetireProcessHandler_UpdatesStatusToRetired(t *testing.T) {
	processRepo, _ := setupProcessRepos()
	registry := process.NewProcessRegistry()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusReady,
	}
	processRepo.AddProcess(worker)

	h := handler.NewRetireProcessHandler(processRepo, registry)

	cmd := command.NewRetireProcessCommand(command.SourceMCPTool, "worker-1", "context full")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	updated, _ := processRepo.Get("worker-1")
	assert.Equal(t, repository.StatusRetired, updated.Status)
}

func TestRetireProcessHandler_SetsRetiredAtTimestamp(t *testing.T) {
	processRepo, _ := setupProcessRepos()
	registry := process.NewProcessRegistry()

	before := time.Now()
	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusReady,
	}
	processRepo.AddProcess(worker)

	h := handler.NewRetireProcessHandler(processRepo, registry)

	cmd := command.NewRetireProcessCommand(command.SourceMCPTool, "worker-1", "")
	_, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)

	updated, _ := processRepo.Get("worker-1")
	assert.False(t, updated.RetiredAt.IsZero())
	assert.True(t, updated.RetiredAt.After(before) || updated.RetiredAt.Equal(before))
}

func TestRetireProcessHandler_EmitsProcessStatusChangeEvent(t *testing.T) {
	processRepo, _ := setupProcessRepos()
	registry := process.NewProcessRegistry()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusReady,
	}
	processRepo.AddProcess(worker)

	h := handler.NewRetireProcessHandler(processRepo, registry)

	cmd := command.NewRetireProcessCommand(command.SourceMCPTool, "worker-1", "")
	result, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)

	require.Len(t, result.Events, 1)
	event := result.Events[0].(events.ProcessEvent)
	assert.Equal(t, events.ProcessStatusChange, event.Type)
	assert.Equal(t, "worker-1", event.ProcessID)
	assert.Equal(t, events.ProcessStatusRetired, event.Status)
}

func TestRetireProcessHandler_UnknownProcess_ReturnsError(t *testing.T) {
	processRepo, _ := setupProcessRepos()
	registry := process.NewProcessRegistry()

	h := handler.NewRetireProcessHandler(processRepo, registry)

	cmd := command.NewRetireProcessCommand(command.SourceMCPTool, "unknown", "")
	result, err := h.Handle(context.Background(), cmd)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, handler.ErrProcessNotFound)
}

func TestRetireProcessHandler_AlreadyRetired_ReturnsSuccessNoOp(t *testing.T) {
	processRepo, _ := setupProcessRepos()
	registry := process.NewProcessRegistry()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusRetired,
	}
	processRepo.AddProcess(worker)

	h := handler.NewRetireProcessHandler(processRepo, registry)

	cmd := command.NewRetireProcessCommand(command.SourceMCPTool, "worker-1", "")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	retireResult := result.Data.(*handler.RetireProcessResult)
	assert.True(t, retireResult.WasNoOp)
}

func TestRetireProcessHandler_CallsCleanupProcessWhenRetiring(t *testing.T) {
	processRepo, _ := setupProcessRepos()
	registry := process.NewProcessRegistry()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusReady,
	}
	processRepo.AddProcess(worker)

	// Create enforcer and populate state
	enforcer := handler.NewTurnCompletionTracker()
	enforcer.RecordToolCall("worker-1", "some_tool")
	enforcer.MarkAsNewlySpawned("worker-1")
	enforcer.IncrementRetry("worker-1")

	// Verify state exists before retire
	assert.True(t, enforcer.IsNewlySpawned("worker-1"))
	assert.True(t, enforcer.ShouldRetry("worker-1")) // retry count is 1, still < 2

	h := handler.NewRetireProcessHandler(processRepo, registry,
		handler.WithRetireTurnEnforcer(enforcer))

	cmd := command.NewRetireProcessCommand(command.SourceMCPTool, "worker-1", "context full")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify CleanupProcess was called - all state should be removed
	assert.False(t, enforcer.IsNewlySpawned("worker-1"))
	// After cleanup, retry count should be 0, so ShouldRetry returns true (0 < 2)
	// We verify cleanup by checking the CheckTurnCompletion returns RequiredTools (no calls recorded)
	missingTools := enforcer.CheckTurnCompletion("worker-1", repository.RoleWorker)
	assert.Equal(t, handler.RequiredTools, missingTools)
}

func TestRetireProcessHandler_WorksWithNilEnforcer(t *testing.T) {
	processRepo, _ := setupProcessRepos()
	registry := process.NewProcessRegistry()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusReady,
	}
	processRepo.AddProcess(worker)

	// Create handler without enforcer (nil by default)
	h := handler.NewRetireProcessHandler(processRepo, registry)

	cmd := command.NewRetireProcessCommand(command.SourceMCPTool, "worker-1", "")
	result, err := h.Handle(context.Background(), cmd)

	// Should succeed without panic
	require.NoError(t, err)
	assert.True(t, result.Success)

	updated, _ := processRepo.Get("worker-1")
	assert.Equal(t, repository.StatusRetired, updated.Status)
}

func TestRetireProcessHandler_CleanupDoesNotAffectOtherProcesses(t *testing.T) {
	processRepo, _ := setupProcessRepos()
	registry := process.NewProcessRegistry()

	worker1 := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusReady,
	}
	worker2 := &repository.Process{
		ID:     "worker-2",
		Role:   repository.RoleWorker,
		Status: repository.StatusWorking,
	}
	processRepo.AddProcess(worker1)
	processRepo.AddProcess(worker2)

	// Create enforcer and populate state for both workers
	enforcer := handler.NewTurnCompletionTracker()
	enforcer.RecordToolCall("worker-1", "post_message")
	enforcer.MarkAsNewlySpawned("worker-1")
	enforcer.RecordToolCall("worker-2", "signal_ready")
	enforcer.MarkAsNewlySpawned("worker-2")

	h := handler.NewRetireProcessHandler(processRepo, registry,
		handler.WithRetireTurnEnforcer(enforcer))

	// Retire worker-1 only
	cmd := command.NewRetireProcessCommand(command.SourceMCPTool, "worker-1", "")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify worker-1 state is cleaned up
	assert.False(t, enforcer.IsNewlySpawned("worker-1"))
	missingTools1 := enforcer.CheckTurnCompletion("worker-1", repository.RoleWorker)
	assert.Equal(t, handler.RequiredTools, missingTools1) // No calls recorded

	// Verify worker-2 state is preserved
	assert.True(t, enforcer.IsNewlySpawned("worker-2"))
	missingTools2 := enforcer.CheckTurnCompletion("worker-2", repository.RoleWorker)
	assert.Empty(t, missingTools2) // signal_ready was recorded
}

func TestRetireProcessHandler_CleanupRemovesAllStateForProcess(t *testing.T) {
	processRepo, _ := setupProcessRepos()
	registry := process.NewProcessRegistry()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusReady,
	}
	processRepo.AddProcess(worker)

	// Create enforcer and populate ALL types of state
	enforcer := handler.NewTurnCompletionTracker()

	// Record tool calls
	enforcer.RecordToolCall("worker-1", "post_message")
	enforcer.RecordToolCall("worker-1", "signal_ready")

	// Mark as newly spawned
	enforcer.MarkAsNewlySpawned("worker-1")

	// Set retry count to 2 (max)
	enforcer.IncrementRetry("worker-1")
	enforcer.IncrementRetry("worker-1")

	// Verify state exists before retire
	assert.True(t, enforcer.IsNewlySpawned("worker-1"))
	assert.False(t, enforcer.ShouldRetry("worker-1")) // retry count is 2, at max
	missingBefore := enforcer.CheckTurnCompletion("worker-1", repository.RoleWorker)
	assert.Empty(t, missingBefore) // Tools were recorded

	h := handler.NewRetireProcessHandler(processRepo, registry,
		handler.WithRetireTurnEnforcer(enforcer))

	cmd := command.NewRetireProcessCommand(command.SourceMCPTool, "worker-1", "")
	_, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)

	// Verify ALL state is removed after cleanup
	// 1. newlySpawned flag should be cleared
	assert.False(t, enforcer.IsNewlySpawned("worker-1"))

	// 2. Tool calls should be cleared - CheckTurnCompletion returns RequiredTools
	missingAfter := enforcer.CheckTurnCompletion("worker-1", repository.RoleWorker)
	assert.Equal(t, handler.RequiredTools, missingAfter)

	// 3. Retry count should be cleared - ShouldRetry returns true (0 < 2)
	assert.True(t, enforcer.ShouldRetry("worker-1"))
}

// ===========================================================================
// SpawnProcessHandler Tests
// ===========================================================================

func TestSpawnProcessHandler_SpawnCoordinator_CreatesWithRoleCoordinator(t *testing.T) {
	processRepo, _ := setupProcessRepos()
	registry := process.NewProcessRegistry()

	h := handler.NewSpawnProcessHandler(processRepo, registry)

	cmd := command.NewSpawnProcessCommand(command.SourceInternal, repository.RoleCoordinator)
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	spawnResult := result.Data.(*handler.SpawnProcessResult)
	assert.Equal(t, repository.CoordinatorID, spawnResult.ProcessID)
	assert.Equal(t, repository.RoleCoordinator, spawnResult.Role)
}

func TestSpawnProcessHandler_SpawnCoordinator_UsesCoordinatorID(t *testing.T) {
	processRepo, _ := setupProcessRepos()
	registry := process.NewProcessRegistry()

	h := handler.NewSpawnProcessHandler(processRepo, registry)

	cmd := command.NewSpawnProcessCommand(command.SourceInternal, repository.RoleCoordinator)
	_, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)

	coord, err := processRepo.GetCoordinator()
	require.NoError(t, err)
	assert.Equal(t, repository.CoordinatorID, coord.ID)
}

func TestSpawnProcessHandler_SpawnCoordinator_FailsIfCoordinatorExists(t *testing.T) {
	processRepo, _ := setupProcessRepos()
	registry := process.NewProcessRegistry()

	// Pre-create coordinator
	coord := &repository.Process{
		ID:     repository.CoordinatorID,
		Role:   repository.RoleCoordinator,
		Status: repository.StatusReady,
	}
	processRepo.AddProcess(coord)

	h := handler.NewSpawnProcessHandler(processRepo, registry)

	cmd := command.NewSpawnProcessCommand(command.SourceInternal, repository.RoleCoordinator)
	result, err := h.Handle(context.Background(), cmd)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, handler.ErrCoordinatorExists)
}

func TestSpawnProcessHandler_SpawnWorker_CreatesWithRoleWorker(t *testing.T) {
	processRepo, _ := setupProcessRepos()
	registry := process.NewProcessRegistry()

	h := handler.NewSpawnProcessHandler(processRepo, registry)

	cmd := command.NewSpawnProcessCommand(command.SourceInternal, repository.RoleWorker)
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	spawnResult := result.Data.(*handler.SpawnProcessResult)
	assert.Equal(t, repository.RoleWorker, spawnResult.Role)
}

func TestSpawnProcessHandler_SpawnWorker_GeneratesUniqueID(t *testing.T) {
	processRepo, _ := setupProcessRepos()
	registry := process.NewProcessRegistry()

	h := handler.NewSpawnProcessHandler(processRepo, registry)

	// Spawn first worker
	cmd1 := command.NewSpawnProcessCommand(command.SourceInternal, repository.RoleWorker)
	result1, err := h.Handle(context.Background(), cmd1)
	require.NoError(t, err)
	id1 := result1.Data.(*handler.SpawnProcessResult).ProcessID

	// Spawn second worker
	cmd2 := command.NewSpawnProcessCommand(command.SourceInternal, repository.RoleWorker)
	result2, err := h.Handle(context.Background(), cmd2)
	require.NoError(t, err)
	id2 := result2.Data.(*handler.SpawnProcessResult).ProcessID

	assert.NotEqual(t, id1, id2)
	assert.Contains(t, id1, "worker-")
	assert.Contains(t, id2, "worker-")
}

func TestSpawnProcessHandler_SavesProcessToRepository(t *testing.T) {
	processRepo, _ := setupProcessRepos()
	registry := process.NewProcessRegistry()

	h := handler.NewSpawnProcessHandler(processRepo, registry)

	cmd := command.NewSpawnProcessCommand(command.SourceInternal, repository.RoleWorker)
	_, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)

	workers := processRepo.Workers()
	assert.Len(t, workers, 1)
}

func TestSpawnProcessHandler_EmitsProcessSpawnedEvent(t *testing.T) {
	processRepo, _ := setupProcessRepos()
	registry := process.NewProcessRegistry()

	h := handler.NewSpawnProcessHandler(processRepo, registry)

	cmd := command.NewSpawnProcessCommand(command.SourceInternal, repository.RoleWorker)
	result, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)

	require.Len(t, result.Events, 1)
	event := result.Events[0].(events.ProcessEvent)
	assert.Equal(t, events.ProcessSpawned, event.Type)
	assert.Equal(t, events.RoleWorker, event.Role)
}

func TestSpawnProcessHandler_CallsMarkAsNewlySpawned(t *testing.T) {
	processRepo, _ := setupProcessRepos()
	registry := process.NewProcessRegistry()
	enforcer := handler.NewTurnCompletionTracker()

	h := handler.NewSpawnProcessHandler(processRepo, registry, handler.WithTurnEnforcer(enforcer))

	cmd := command.NewSpawnProcessCommand(command.SourceInternal, repository.RoleWorker)
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Get the spawned process ID from result
	spawnResult := result.Data.(*handler.SpawnProcessResult)

	// Verify enforcer was notified
	assert.True(t, enforcer.IsNewlySpawned(spawnResult.ProcessID),
		"spawned process should be marked as newly spawned")
}

func TestSpawnProcessHandler_WorksCorrectlyWhenEnforcerIsNil(t *testing.T) {
	processRepo, _ := setupProcessRepos()
	registry := process.NewProcessRegistry()

	// Create handler WITHOUT enforcer (default behavior)
	h := handler.NewSpawnProcessHandler(processRepo, registry)

	cmd := command.NewSpawnProcessCommand(command.SourceInternal, repository.RoleWorker)
	result, err := h.Handle(context.Background(), cmd)

	// Should not panic and should complete successfully
	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestSpawnProcessHandler_MarkAsNewlySpawnedNotCalledIfProcessCreationFails(t *testing.T) {
	processRepo, _ := setupProcessRepos()
	registry := process.NewProcessRegistry()
	enforcer := handler.NewTurnCompletionTracker()

	// Create a mock spawner that returns an error
	failingSpawner := &mockProcessSpawner{spawnErr: assert.AnError}

	h := handler.NewSpawnProcessHandler(processRepo, registry,
		handler.WithTurnEnforcer(enforcer),
		handler.WithUnifiedSpawner(failingSpawner))

	cmd := command.NewSpawnProcessCommand(command.SourceInternal, repository.RoleWorker)
	_, err := h.Handle(context.Background(), cmd)

	// Should fail
	require.Error(t, err)

	// Verify no process was marked as newly spawned
	// Check that no worker IDs are marked as newly spawned
	workers := processRepo.Workers()
	for _, w := range workers {
		assert.False(t, enforcer.IsNewlySpawned(w.ID),
			"no process should be marked as newly spawned when spawn fails")
	}
}

func TestSpawnProcessHandler_MarksCoordinatorAsNewlySpawned(t *testing.T) {
	processRepo, _ := setupProcessRepos()
	registry := process.NewProcessRegistry()
	enforcer := handler.NewTurnCompletionTracker()

	h := handler.NewSpawnProcessHandler(processRepo, registry, handler.WithTurnEnforcer(enforcer))

	cmd := command.NewSpawnProcessCommand(command.SourceInternal, repository.RoleCoordinator)
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify coordinator is also marked as newly spawned
	assert.True(t, enforcer.IsNewlySpawned(repository.CoordinatorID),
		"coordinator should be marked as newly spawned")
}

func TestSpawnProcessHandler_PassesAgentTypeToSpawner(t *testing.T) {
	processRepo, _ := setupProcessRepos()
	spawner := &mockProcessSpawner{}

	// Pass nil registry to avoid registering the nil process returned by mock
	h := handler.NewSpawnProcessHandler(processRepo, nil, handler.WithUnifiedSpawner(spawner))

	// Create command with specific agent type
	cmd := command.NewSpawnProcessCommand(command.SourceMCPTool, repository.RoleWorker, command.WithAgentType(roles.AgentTypeImplementer))
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify spawner was called with correct agent type
	require.Len(t, spawner.spawnCalls, 1)
	assert.Equal(t, roles.AgentTypeImplementer, spawner.spawnCalls[0].AgentType)
}

func TestSpawnProcessHandler_PassesDefaultAgentTypeToSpawner(t *testing.T) {
	processRepo, _ := setupProcessRepos()
	spawner := &mockProcessSpawner{}

	// Pass nil registry to avoid registering the nil process returned by mock
	h := handler.NewSpawnProcessHandler(processRepo, nil, handler.WithUnifiedSpawner(spawner))

	// Create command without specifying agent type (defaults to generic)
	cmd := command.NewSpawnProcessCommand(command.SourceMCPTool, repository.RoleWorker)
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify spawner was called with generic agent type
	require.Len(t, spawner.spawnCalls, 1)
	assert.Equal(t, roles.AgentTypeGeneric, spawner.spawnCalls[0].AgentType)
}

func TestSpawnProcessHandler_PassesAllAgentTypesToSpawner(t *testing.T) {
	testCases := []struct {
		name      string
		agentType roles.AgentType
	}{
		{"generic", roles.AgentTypeGeneric},
		{"implementer", roles.AgentTypeImplementer},
		{"reviewer", roles.AgentTypeReviewer},
		{"researcher", roles.AgentTypeResearcher},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			processRepo, _ := setupProcessRepos()
			spawner := &mockProcessSpawner{}

			// Pass nil registry to avoid registering the nil process returned by mock
			h := handler.NewSpawnProcessHandler(processRepo, nil, handler.WithUnifiedSpawner(spawner))

			cmd := command.NewSpawnProcessCommand(command.SourceMCPTool, repository.RoleWorker, command.WithAgentType(tc.agentType))
			result, err := h.Handle(context.Background(), cmd)

			require.NoError(t, err)
			assert.True(t, result.Success)

			require.Len(t, spawner.spawnCalls, 1)
			assert.Equal(t, tc.agentType, spawner.spawnCalls[0].AgentType)
		})
	}
}

// ===========================================================================
// ReplaceProcessHandler Tests
// ===========================================================================

func TestReplaceProcessHandler_ReplaceCoordinator_CreatesHandoffPrompt(t *testing.T) {
	processRepo, _ := setupProcessRepos()
	registry := process.NewProcessRegistry()

	coord := &repository.Process{
		ID:     repository.CoordinatorID,
		Role:   repository.RoleCoordinator,
		Status: repository.StatusReady,
	}
	processRepo.AddProcess(coord)

	h := handler.NewReplaceProcessHandler(processRepo, registry)

	cmd := command.NewReplaceProcessCommand(command.SourceMCPTool, repository.CoordinatorID, "context window full")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	replaceResult := result.Data.(*handler.ReplaceProcessResult)
	assert.Equal(t, repository.CoordinatorID, replaceResult.OldProcessID)
	assert.Equal(t, repository.CoordinatorID, replaceResult.NewProcessID)
	assert.Equal(t, repository.RoleCoordinator, replaceResult.Role)
}

func TestReplaceProcessHandler_ReplaceWorker_RetiresAndSpawnsNew(t *testing.T) {
	processRepo, _ := setupProcessRepos()
	registry := process.NewProcessRegistry()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusReady,
	}
	processRepo.AddProcess(worker)

	h := handler.NewReplaceProcessHandler(processRepo, registry)

	cmd := command.NewReplaceProcessCommand(command.SourceMCPTool, "worker-1", "context full")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	replaceResult := result.Data.(*handler.ReplaceProcessResult)
	assert.Equal(t, "worker-1", replaceResult.OldProcessID)
	assert.NotEqual(t, "worker-1", replaceResult.NewProcessID) // New ID
	assert.Equal(t, repository.RoleWorker, replaceResult.Role)

	// Old worker should be retired
	oldWorker, _ := processRepo.Get("worker-1")
	assert.Equal(t, repository.StatusRetired, oldWorker.Status)
}

func TestReplaceProcessHandler_UnknownProcess_ReturnsError(t *testing.T) {
	processRepo, _ := setupProcessRepos()
	registry := process.NewProcessRegistry()

	h := handler.NewReplaceProcessHandler(processRepo, registry)

	cmd := command.NewReplaceProcessCommand(command.SourceMCPTool, "unknown", "")
	result, err := h.Handle(context.Background(), cmd)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, handler.ErrProcessNotFound)
}

func TestReplaceProcessHandler_EmitsRetiredAndSpawnedEvents(t *testing.T) {
	processRepo, _ := setupProcessRepos()
	registry := process.NewProcessRegistry()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusReady,
	}
	processRepo.AddProcess(worker)

	h := handler.NewReplaceProcessHandler(processRepo, registry)

	cmd := command.NewReplaceProcessCommand(command.SourceMCPTool, "worker-1", "")
	result, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)

	// Should have 2 events: retired and spawned
	require.Len(t, result.Events, 2)

	// First event: status change to retired
	retiredEvent := result.Events[0].(events.ProcessEvent)
	assert.Equal(t, events.ProcessStatusChange, retiredEvent.Type)
	assert.Equal(t, events.ProcessStatusRetired, retiredEvent.Status)

	// Second event: new process spawned
	spawnedEvent := result.Events[1].(events.ProcessEvent)
	assert.Equal(t, events.ProcessSpawned, spawnedEvent.Type)
}

// ===========================================================================
// PauseProcessHandler Tests
// ===========================================================================

func TestPauseProcessHandler_PauseFromReady_TransitionsToPaused(t *testing.T) {
	processRepo, _ := setupProcessRepos()

	coord := &repository.Process{
		ID:     repository.CoordinatorID,
		Role:   repository.RoleCoordinator,
		Status: repository.StatusReady,
	}
	processRepo.AddProcess(coord)

	h := handler.NewPauseProcessHandler(processRepo)

	cmd := command.NewPauseProcessCommand(command.SourceUser, repository.CoordinatorID, "user requested pause")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify status updated to Paused
	updated, _ := processRepo.Get(repository.CoordinatorID)
	assert.Equal(t, repository.StatusPaused, updated.Status)

	// Verify result
	pauseResult := result.Data.(*handler.PauseProcessResult)
	assert.Equal(t, repository.CoordinatorID, pauseResult.ProcessID)
	assert.False(t, pauseResult.WasNoOp)
}

func TestPauseProcessHandler_PauseFromWorking_TransitionsToPaused(t *testing.T) {
	processRepo, _ := setupProcessRepos()

	coord := &repository.Process{
		ID:     repository.CoordinatorID,
		Role:   repository.RoleCoordinator,
		Status: repository.StatusWorking,
	}
	processRepo.AddProcess(coord)

	h := handler.NewPauseProcessHandler(processRepo)

	cmd := command.NewPauseProcessCommand(command.SourceUser, repository.CoordinatorID, "")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	updated, _ := processRepo.Get(repository.CoordinatorID)
	assert.Equal(t, repository.StatusPaused, updated.Status)
}

func TestPauseProcessHandler_PauseWorkerFromReady_TransitionsToPaused(t *testing.T) {
	processRepo, _ := setupProcessRepos()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusReady,
		TaskID: "task-123",
	}
	processRepo.AddProcess(worker)

	h := handler.NewPauseProcessHandler(processRepo)

	cmd := command.NewPauseProcessCommand(command.SourceUser, "worker-1", "")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	updated, _ := processRepo.Get("worker-1")
	assert.Equal(t, repository.StatusPaused, updated.Status)
}

func TestPauseProcessHandler_AlreadyPaused_ReturnsSuccessNoOp(t *testing.T) {
	processRepo, _ := setupProcessRepos()

	coord := &repository.Process{
		ID:     repository.CoordinatorID,
		Role:   repository.RoleCoordinator,
		Status: repository.StatusPaused,
	}
	processRepo.AddProcess(coord)

	h := handler.NewPauseProcessHandler(processRepo)

	cmd := command.NewPauseProcessCommand(command.SourceUser, repository.CoordinatorID, "")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	pauseResult := result.Data.(*handler.PauseProcessResult)
	assert.True(t, pauseResult.WasNoOp)
}

func TestPauseProcessHandler_EmitsProcessStatusChangeEvent(t *testing.T) {
	processRepo, _ := setupProcessRepos()

	coord := &repository.Process{
		ID:     repository.CoordinatorID,
		Role:   repository.RoleCoordinator,
		Status: repository.StatusReady,
	}
	processRepo.AddProcess(coord)

	h := handler.NewPauseProcessHandler(processRepo)

	cmd := command.NewPauseProcessCommand(command.SourceUser, repository.CoordinatorID, "")
	result, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)

	require.Len(t, result.Events, 1)
	event := result.Events[0].(events.ProcessEvent)
	assert.Equal(t, events.ProcessStatusChange, event.Type)
	assert.Equal(t, repository.CoordinatorID, event.ProcessID)
	assert.Equal(t, events.ProcessStatusPaused, event.Status)
	assert.Equal(t, events.RoleCoordinator, event.Role)
}

func TestPauseProcessHandler_UnknownProcess_ReturnsError(t *testing.T) {
	processRepo, _ := setupProcessRepos()

	h := handler.NewPauseProcessHandler(processRepo)

	cmd := command.NewPauseProcessCommand(command.SourceUser, "unknown", "")
	result, err := h.Handle(context.Background(), cmd)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, handler.ErrProcessNotFound)
}

func TestPauseProcessHandler_RetiredProcess_ReturnsError(t *testing.T) {
	processRepo, _ := setupProcessRepos()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusRetired,
	}
	processRepo.AddProcess(worker)

	h := handler.NewPauseProcessHandler(processRepo)

	cmd := command.NewPauseProcessCommand(command.SourceUser, "worker-1", "")
	result, err := h.Handle(context.Background(), cmd)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, handler.ErrProcessRetired)
}

// ===========================================================================
// ResumeProcessHandler Tests
// ===========================================================================

func TestResumeProcessHandler_ResumeFromPaused_TransitionsToReady(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()

	coord := &repository.Process{
		ID:     repository.CoordinatorID,
		Role:   repository.RoleCoordinator,
		Status: repository.StatusPaused,
	}
	processRepo.AddProcess(coord)

	h := handler.NewResumeProcessHandler(processRepo, queueRepo)

	cmd := command.NewResumeProcessCommand(command.SourceUser, repository.CoordinatorID)
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify status updated to Ready
	updated, _ := processRepo.Get(repository.CoordinatorID)
	assert.Equal(t, repository.StatusReady, updated.Status)

	// Verify result
	resumeResult := result.Data.(*handler.ResumeProcessResult)
	assert.Equal(t, repository.CoordinatorID, resumeResult.ProcessID)
	assert.False(t, resumeResult.WasNoOp)
}

func TestResumeProcessHandler_ResumeWorker_TransitionsToReady(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusPaused,
		TaskID: "task-123",
	}
	processRepo.AddProcess(worker)

	h := handler.NewResumeProcessHandler(processRepo, queueRepo)

	cmd := command.NewResumeProcessCommand(command.SourceUser, "worker-1")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	updated, _ := processRepo.Get("worker-1")
	assert.Equal(t, repository.StatusReady, updated.Status)
}

func TestResumeProcessHandler_ResumeFromStopped_TransitionsToReady(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusStopped,
		TaskID: "task-123",
	}
	processRepo.AddProcess(worker)

	h := handler.NewResumeProcessHandler(processRepo, queueRepo)

	cmd := command.NewResumeProcessCommand(command.SourceUser, "worker-1")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	updated, _ := processRepo.Get("worker-1")
	assert.Equal(t, repository.StatusReady, updated.Status)

	resumeResult := result.Data.(*handler.ResumeProcessResult)
	assert.False(t, resumeResult.WasNoOp)
}

func TestResumeProcessHandler_AlreadyReady_ReturnsSuccessNoOp(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()

	coord := &repository.Process{
		ID:     repository.CoordinatorID,
		Role:   repository.RoleCoordinator,
		Status: repository.StatusReady,
	}
	processRepo.AddProcess(coord)

	h := handler.NewResumeProcessHandler(processRepo, queueRepo)

	cmd := command.NewResumeProcessCommand(command.SourceUser, repository.CoordinatorID)
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	resumeResult := result.Data.(*handler.ResumeProcessResult)
	assert.True(t, resumeResult.WasNoOp)
}

func TestResumeProcessHandler_AlreadyWorking_ReturnsSuccessNoOp(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()

	coord := &repository.Process{
		ID:     repository.CoordinatorID,
		Role:   repository.RoleCoordinator,
		Status: repository.StatusWorking,
	}
	processRepo.AddProcess(coord)

	h := handler.NewResumeProcessHandler(processRepo, queueRepo)

	cmd := command.NewResumeProcessCommand(command.SourceUser, repository.CoordinatorID)
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	resumeResult := result.Data.(*handler.ResumeProcessResult)
	assert.True(t, resumeResult.WasNoOp)
}

func TestResumeProcessHandler_EmitsProcessStatusChangeEvent(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()

	coord := &repository.Process{
		ID:     repository.CoordinatorID,
		Role:   repository.RoleCoordinator,
		Status: repository.StatusPaused,
	}
	processRepo.AddProcess(coord)

	h := handler.NewResumeProcessHandler(processRepo, queueRepo)

	cmd := command.NewResumeProcessCommand(command.SourceUser, repository.CoordinatorID)
	result, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)

	require.Len(t, result.Events, 1)
	event := result.Events[0].(events.ProcessEvent)
	assert.Equal(t, events.ProcessStatusChange, event.Type)
	assert.Equal(t, repository.CoordinatorID, event.ProcessID)
	assert.Equal(t, events.ProcessStatusReady, event.Status)
	assert.Equal(t, events.RoleCoordinator, event.Role)
}

func TestResumeProcessHandler_TriggersQueueDrainWhenMessagesPending(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()

	coord := &repository.Process{
		ID:     repository.CoordinatorID,
		Role:   repository.RoleCoordinator,
		Status: repository.StatusPaused,
	}
	processRepo.AddProcess(coord)

	// Queue a message while paused
	queue := queueRepo.GetOrCreate(repository.CoordinatorID)
	_ = queue.Enqueue("pending message", repository.SenderUser)

	h := handler.NewResumeProcessHandler(processRepo, queueRepo)

	cmd := command.NewResumeProcessCommand(command.SourceUser, repository.CoordinatorID)
	result, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)

	// Verify follow-up command to deliver queued messages
	require.Len(t, result.FollowUp, 1)
	deliverCmd := result.FollowUp[0].(*command.DeliverProcessQueuedCommand)
	assert.Equal(t, repository.CoordinatorID, deliverCmd.ProcessID)

	// Verify result indicates queued delivery
	resumeResult := result.Data.(*handler.ResumeProcessResult)
	assert.True(t, resumeResult.QueuedDelivery)
}

func TestResumeProcessHandler_NoQueueDrainWhenQueueEmpty(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()

	coord := &repository.Process{
		ID:     repository.CoordinatorID,
		Role:   repository.RoleCoordinator,
		Status: repository.StatusPaused,
	}
	processRepo.AddProcess(coord)

	// No messages in queue

	h := handler.NewResumeProcessHandler(processRepo, queueRepo)

	cmd := command.NewResumeProcessCommand(command.SourceUser, repository.CoordinatorID)
	result, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)

	// No follow-up commands
	assert.Empty(t, result.FollowUp)

	resumeResult := result.Data.(*handler.ResumeProcessResult)
	assert.False(t, resumeResult.QueuedDelivery)
}

func TestResumeProcessHandler_UnknownProcess_ReturnsError(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()

	h := handler.NewResumeProcessHandler(processRepo, queueRepo)

	cmd := command.NewResumeProcessCommand(command.SourceUser, "unknown")
	result, err := h.Handle(context.Background(), cmd)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, handler.ErrProcessNotFound)
}

func TestResumeProcessHandler_RetiredProcess_ReturnsError(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()

	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusRetired,
	}
	processRepo.AddProcess(worker)

	h := handler.NewResumeProcessHandler(processRepo, queueRepo)

	cmd := command.NewResumeProcessCommand(command.SourceUser, "worker-1")
	result, err := h.Handle(context.Background(), cmd)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, handler.ErrProcessRetired)
}

func TestResumeProcessHandler_UpdatesLastActivityAt(t *testing.T) {
	processRepo, queueRepo := setupProcessRepos()

	before := time.Now().Add(-time.Hour)
	coord := &repository.Process{
		ID:             repository.CoordinatorID,
		Role:           repository.RoleCoordinator,
		Status:         repository.StatusPaused,
		LastActivityAt: before,
	}
	processRepo.AddProcess(coord)

	h := handler.NewResumeProcessHandler(processRepo, queueRepo)

	cmd := command.NewResumeProcessCommand(command.SourceUser, repository.CoordinatorID)
	_, err := h.Handle(context.Background(), cmd)
	require.NoError(t, err)

	updated, _ := processRepo.Get(repository.CoordinatorID)
	assert.True(t, updated.LastActivityAt.After(before))
}

// ===========================================================================
// ReplaceCoordinatorHandler Tests
// ===========================================================================

// mockMessagePoster is a test implementation of MessagePoster.
type mockMessagePoster struct {
	handoffMessages []string
	postErr         error
}

func (m *mockMessagePoster) PostHandoff(content string) error {
	if m.postErr != nil {
		return m.postErr
	}
	m.handoffMessages = append(m.handoffMessages, content)
	return nil
}

// mockCoordinatorSpawnerWithPrompt is a test implementation of CoordinatorSpawnerWithPrompt.
type mockCoordinatorSpawnerWithPrompt struct {
	spawnCalls []coordinatorSpawnCall
	spawnErr   error
}

type coordinatorSpawnCall struct {
	Prompt string
}

func (m *mockCoordinatorSpawnerWithPrompt) SpawnCoordinatorWithPrompt(ctx context.Context, prompt string) (*process.Process, error) {
	m.spawnCalls = append(m.spawnCalls, coordinatorSpawnCall{Prompt: prompt})
	if m.spawnErr != nil {
		return nil, m.spawnErr
	}
	return nil, nil
}

func TestReplaceCoordinatorHandler_PostsHandoffMessage(t *testing.T) {
	processRepo, _ := setupProcessRepos()
	poster := &mockMessagePoster{}

	// Create coordinator in Ready status
	coord := &repository.Process{
		ID:     repository.CoordinatorID,
		Role:   repository.RoleCoordinator,
		Status: repository.StatusReady,
	}
	processRepo.AddProcess(coord)

	h := handler.NewReplaceCoordinatorHandler(processRepo, nil,
		handler.WithMessagePoster(poster))

	cmd := command.NewReplaceCoordinatorCommand(command.SourceUser, "context window limit")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify handoff message was posted
	require.Len(t, poster.handoffMessages, 1)
	assert.Contains(t, poster.handoffMessages[0], "SYSTEM HANDOFF")
	assert.Contains(t, poster.handoffMessages[0], "context window limit")
}

func TestReplaceCoordinatorHandler_SpawnsNewCoordinatorWithReplacePrompt(t *testing.T) {
	processRepo, _ := setupProcessRepos()
	spawner := &mockCoordinatorSpawnerWithPrompt{}

	// Create coordinator in Ready status
	coord := &repository.Process{
		ID:     repository.CoordinatorID,
		Role:   repository.RoleCoordinator,
		Status: repository.StatusReady,
	}
	processRepo.AddProcess(coord)

	h := handler.NewReplaceCoordinatorHandler(processRepo, nil,
		handler.WithCoordinatorSpawnerWithPrompt(spawner))

	cmd := command.NewReplaceCoordinatorCommand(command.SourceUser, "")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify new coordinator was spawned with replace prompt
	require.Len(t, spawner.spawnCalls, 1)
	assert.Contains(t, spawner.spawnCalls[0].Prompt, "CONTEXT REFRESH - NEW SESSION")
	assert.Contains(t, spawner.spawnCalls[0].Prompt, "READ THE HANDOFF FIRST")
}

func TestReplaceCoordinatorHandler_RetiresOldCoordinator(t *testing.T) {
	processRepo, _ := setupProcessRepos()

	// Create coordinator in Working status
	coord := &repository.Process{
		ID:     repository.CoordinatorID,
		Role:   repository.RoleCoordinator,
		Status: repository.StatusWorking,
	}
	processRepo.AddProcess(coord)

	h := handler.NewReplaceCoordinatorHandler(processRepo, nil)

	cmd := command.NewReplaceCoordinatorCommand(command.SourceUser, "")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// The process should exist and be in Working status now (new coordinator)
	// because the handler creates a new coordinator entity after retiring the old one
	updated, err := processRepo.Get(repository.CoordinatorID)
	require.NoError(t, err)
	assert.Equal(t, repository.StatusWorking, updated.Status)
}

func TestReplaceCoordinatorHandler_EmitsCorrectEvents(t *testing.T) {
	processRepo, _ := setupProcessRepos()

	coord := &repository.Process{
		ID:     repository.CoordinatorID,
		Role:   repository.RoleCoordinator,
		Status: repository.StatusReady,
	}
	processRepo.AddProcess(coord)

	h := handler.NewReplaceCoordinatorHandler(processRepo, nil)

	cmd := command.NewReplaceCoordinatorCommand(command.SourceUser, "")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	require.Len(t, result.Events, 3)

	// Event 1: Old coordinator retired
	retiredEvent := result.Events[0].(events.ProcessEvent)
	assert.Equal(t, events.ProcessStatusChange, retiredEvent.Type)
	assert.Equal(t, events.ProcessStatusRetired, retiredEvent.Status)
	assert.Equal(t, events.RoleCoordinator, retiredEvent.Role)

	// Event 2: Output notification
	outputEvent := result.Events[1].(events.ProcessEvent)
	assert.Equal(t, events.ProcessOutput, outputEvent.Type)
	assert.Contains(t, outputEvent.Output, "replaced with fresh context window")

	// Event 3: New coordinator working
	workingEvent := result.Events[2].(events.ProcessEvent)
	assert.Equal(t, events.ProcessWorking, workingEvent.Type)
	assert.Equal(t, events.ProcessStatusWorking, workingEvent.Status)
}

func TestReplaceCoordinatorHandler_UnknownCoordinator_ReturnsError(t *testing.T) {
	processRepo, _ := setupProcessRepos()
	// No coordinator added

	h := handler.NewReplaceCoordinatorHandler(processRepo, nil)

	cmd := command.NewReplaceCoordinatorCommand(command.SourceUser, "")
	result, err := h.Handle(context.Background(), cmd)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, handler.ErrProcessNotFound)
}

func TestReplaceCoordinatorHandler_RetiredCoordinator_ReturnsError(t *testing.T) {
	processRepo, _ := setupProcessRepos()

	coord := &repository.Process{
		ID:     repository.CoordinatorID,
		Role:   repository.RoleCoordinator,
		Status: repository.StatusRetired,
	}
	processRepo.AddProcess(coord)

	h := handler.NewReplaceCoordinatorHandler(processRepo, nil)

	cmd := command.NewReplaceCoordinatorCommand(command.SourceUser, "")
	result, err := h.Handle(context.Background(), cmd)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, handler.ErrProcessRetired)
}

func TestReplaceCoordinatorHandler_ResultContainsReplacePrompt(t *testing.T) {
	processRepo, _ := setupProcessRepos()

	coord := &repository.Process{
		ID:     repository.CoordinatorID,
		Role:   repository.RoleCoordinator,
		Status: repository.StatusReady,
	}
	processRepo.AddProcess(coord)

	h := handler.NewReplaceCoordinatorHandler(processRepo, nil)

	cmd := command.NewReplaceCoordinatorCommand(command.SourceUser, "")
	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)

	replaceResult := result.Data.(*handler.ReplaceCoordinatorResult)
	assert.Equal(t, repository.CoordinatorID, replaceResult.OldProcessID)
	assert.Equal(t, repository.CoordinatorID, replaceResult.NewProcessID)
	assert.Contains(t, replaceResult.ReplacePrompt, "CONTEXT REFRESH")
	assert.Contains(t, replaceResult.ReplacePrompt, "READ THE HANDOFF FIRST")
}

// ===========================================================================
// BuildReplacePrompt Tests (v2 version)
// ===========================================================================

func TestBuildReplacePrompt_ContainsHandoffInstructions(t *testing.T) {
	p := prompt.BuildReplacePrompt()

	// Should mention reading the handoff message
	assert.Contains(t, p, "READ THE HANDOFF FIRST")
	assert.Contains(t, p, "handoff message")
	assert.Contains(t, p, "previous coordinator")
	assert.Contains(t, p, "read_message_log")
}

func TestBuildReplacePrompt_WaitsForUser(t *testing.T) {
	p := prompt.BuildReplacePrompt()

	// Should instruct waiting for user direction
	assert.Contains(t, p, "Wait for the user to provide direction")
	assert.Contains(t, p, "Do NOT assign tasks")
	assert.Contains(t, p, "Do NOT")
	assert.Contains(t, p, "until the user tells you what to do")
}

func TestBuildReplacePrompt_NoImmediateActions(t *testing.T) {
	p := prompt.BuildReplacePrompt()

	// Should NOT contain the old "IMMEDIATE ACTIONS REQUIRED" section
	assert.NotContains(t, p, "IMMEDIATE ACTIONS REQUIRED")

	// Should still be a valid prompt with header
	assert.Contains(t, p, "[CONTEXT REFRESH - NEW SESSION]")
	assert.Contains(t, p, "WHAT TO DO NOW")
}
