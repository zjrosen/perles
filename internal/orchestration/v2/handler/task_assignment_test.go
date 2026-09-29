package handler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
	"github.com/zjrosen/perles/internal/orchestration/v2/types"
	taskpkg "github.com/zjrosen/perles/internal/task"
)

// ===========================================================================
// Test Helpers
// ===========================================================================

// phasePtr returns a pointer to a ProcessPhase.
func phasePtr(p events.ProcessPhase) *events.ProcessPhase {
	return &p
}

// ===========================================================================
// AssignTaskHandler Tests
// ===========================================================================

func TestAssignTaskHandler_AssignsToReadyWorker(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	bdExecutor := mocks.NewMockTaskExecutor(t)
	// Mock for AssignTaskHandler: ShowIssue and UpdateStatus
	bdExecutor.EXPECT().ShowIssue(mock.Anything).Return(&taskpkg.Issue{ID: "perles-abc1.2", Status: taskpkg.StatusOpen}, nil).Maybe()
	bdExecutor.EXPECT().UpdateStatus(mock.Anything, mock.Anything).Return(nil).Maybe()

	// Add ready worker process
	proc := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     phasePtr(events.ProcessPhaseIdle),
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(proc)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewAssignTaskHandler(processRepo, taskRepo, WithBDExecutor(bdExecutor), WithQueueRepository(queueRepo))

	cmd := command.NewAssignTaskCommand(command.SourceMCPTool, "worker-1", "perles-abc1.2", "Implement feature X", "")
	result, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)
	require.True(t, result.Success, "expected success, got failure: %v", result.Error)

	// Verify process was updated - Status stays Ready until delivery
	updated, _ := processRepo.Get("worker-1")
	require.Equal(t, repository.StatusReady, updated.Status, "expected status StatusReady (delivery sets Working)")
	require.NotNil(t, updated.Phase)
	require.Equal(t, events.ProcessPhaseImplementing, *updated.Phase)
	require.Equal(t, "perles-abc1.2", updated.TaskID)

	// Verify follow-up command was created
	require.Len(t, result.FollowUp, 1)
	followUp, ok := result.FollowUp[0].(*command.DeliverProcessQueuedCommand)
	require.True(t, ok, "expected DeliverProcessQueuedCommand, got: %T", result.FollowUp[0])
	require.Equal(t, "worker-1", followUp.ProcessID)

	// Verify message was queued
	queue := queueRepo.GetOrCreate("worker-1")
	require.Equal(t, 1, queue.Size())

	// Verify task was created
	task, err := taskRepo.Get("perles-abc1.2")
	require.NoError(t, err, "task not found")
	require.Equal(t, "worker-1", task.Implementer)
	require.Equal(t, repository.TaskImplementing, task.Status)
}

func TestAssignTaskHandler_FailsIfWorkerNotReady(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	bdExecutor := mocks.NewMockTaskExecutor(t)

	// Add working process
	proc := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseImplementing),
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(proc)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewAssignTaskHandler(processRepo, taskRepo, WithBDExecutor(bdExecutor), WithQueueRepository(queueRepo))

	cmd := command.NewAssignTaskCommand(command.SourceMCPTool, "worker-1", "perles-abc1.2", "", "")
	_, err := handler.Handle(context.Background(), cmd)

	require.Error(t, err, "expected error for non-ready process")
	require.ErrorIs(t, err, types.ErrProcessNotReady)
}

func TestAssignTaskHandler_FailsIfWorkerNotIdlePhase(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	bdExecutor := mocks.NewMockTaskExecutor(t)

	// Add ready process but in non-idle phase
	proc := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     phasePtr(events.ProcessPhaseImplementing), // Wrong phase
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(proc)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewAssignTaskHandler(processRepo, taskRepo, WithBDExecutor(bdExecutor), WithQueueRepository(queueRepo))

	cmd := command.NewAssignTaskCommand(command.SourceMCPTool, "worker-1", "perles-abc1.2", "", "")
	_, err := handler.Handle(context.Background(), cmd)

	require.Error(t, err, "expected error for non-idle phase process")
	require.ErrorIs(t, err, types.ErrProcessNotIdle)
}

func TestAssignTaskHandler_FailsIfWorkerAlreadyHasTask(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	bdExecutor := mocks.NewMockTaskExecutor(t)

	// Add ready process with existing task
	proc := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     phasePtr(events.ProcessPhaseIdle),
		TaskID:    "existing-task",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(proc)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewAssignTaskHandler(processRepo, taskRepo, WithBDExecutor(bdExecutor), WithQueueRepository(queueRepo))

	cmd := command.NewAssignTaskCommand(command.SourceMCPTool, "worker-1", "perles-abc1.2", "", "")
	_, err := handler.Handle(context.Background(), cmd)

	require.Error(t, err, "expected error for process with existing task")
	require.ErrorIs(t, err, types.ErrProcessAlreadyAssigned)
}

func TestAssignTaskHandler_FailsIfWorkerAlreadyImplementer(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	bdExecutor := mocks.NewMockTaskExecutor(t)
	// Mock for ShowIssue (called before the check that fails)
	bdExecutor.EXPECT().ShowIssue(mock.Anything).Return(&taskpkg.Issue{ID: "perles-abc1.2", Status: taskpkg.StatusOpen}, nil).Maybe()

	// Add ready process
	proc := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     phasePtr(events.ProcessPhaseIdle),
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(proc)

	// Add existing task where process is implementer
	existingTask := &repository.TaskAssignment{
		TaskID:      "other-task",
		Implementer: "worker-1",
		Status:      repository.TaskImplementing,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(existingTask)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewAssignTaskHandler(processRepo, taskRepo, WithBDExecutor(bdExecutor), WithQueueRepository(queueRepo))

	cmd := command.NewAssignTaskCommand(command.SourceMCPTool, "worker-1", "perles-abc1.2", "", "")
	_, err := handler.Handle(context.Background(), cmd)

	require.Error(t, err, "expected error for process already implementing a task")
	require.ErrorIs(t, err, types.ErrProcessAlreadyAssigned)
}

func TestAssignTaskHandler_QueuesPromptAndCreatesDeliveryFollowUp(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	bdExecutor := mocks.NewMockTaskExecutor(t)
	// Mock for AssignTaskHandler: ShowIssue and UpdateStatus
	bdExecutor.EXPECT().ShowIssue(mock.Anything).Return(&taskpkg.Issue{ID: "perles-abc1.2", Status: taskpkg.StatusOpen}, nil).Maybe()
	bdExecutor.EXPECT().UpdateStatus(mock.Anything, mock.Anything).Return(nil).Maybe()

	proc := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     phasePtr(events.ProcessPhaseIdle),
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(proc)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewAssignTaskHandler(processRepo, taskRepo, WithBDExecutor(bdExecutor), WithQueueRepository(queueRepo))

	cmd := command.NewAssignTaskCommand(command.SourceMCPTool, "worker-1", "perles-abc1.2", "Implement feature", "")
	result, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)

	// Verify status remains Ready (DeliverProcessQueuedHandler sets Working)
	updated, _ := processRepo.Get("worker-1")
	require.Equal(t, repository.StatusReady, updated.Status)

	// Verify phase is set to Implementing
	require.NotNil(t, updated.Phase)
	require.Equal(t, events.ProcessPhaseImplementing, *updated.Phase)

	// Verify message was queued
	queue := queueRepo.GetOrCreate("worker-1")
	require.Equal(t, 1, queue.Size())

	// Verify follow-up command was created
	require.Len(t, result.FollowUp, 1)
	followUp, ok := result.FollowUp[0].(*command.DeliverProcessQueuedCommand)
	require.True(t, ok, "expected DeliverProcessQueuedCommand, got: %T", result.FollowUp[0])
	require.Equal(t, "worker-1", followUp.ProcessID)
}

func TestAssignTaskHandler_CreatesTaskAssignment(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	bdExecutor := mocks.NewMockTaskExecutor(t)
	// Mock for AssignTaskHandler: ShowIssue and UpdateStatus
	bdExecutor.EXPECT().ShowIssue(mock.Anything).Return(&taskpkg.Issue{ID: "perles-abc1.2", Status: taskpkg.StatusOpen}, nil).Maybe()
	bdExecutor.EXPECT().UpdateStatus(mock.Anything, mock.Anything).Return(nil).Maybe()

	proc := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     phasePtr(events.ProcessPhaseIdle),
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(proc)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewAssignTaskHandler(processRepo, taskRepo, WithBDExecutor(bdExecutor), WithQueueRepository(queueRepo))

	cmd := command.NewAssignTaskCommand(command.SourceMCPTool, "worker-1", "perles-abc1.2", "", "")
	_, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)

	task, err := taskRepo.Get("perles-abc1.2")
	require.NoError(t, err, "task not created")

	require.Equal(t, "worker-1", task.Implementer)
	require.Equal(t, repository.TaskImplementing, task.Status)
	require.False(t, task.StartedAt.IsZero(), "expected StartedAt to be set")
}

func TestAssignTaskHandler_EmitsStatusChangeEvent(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	bdExecutor := mocks.NewMockTaskExecutor(t)
	// Mock for AssignTaskHandler: ShowIssue and UpdateStatus
	bdExecutor.EXPECT().ShowIssue(mock.Anything).Return(&taskpkg.Issue{ID: "perles-abc1.2", Status: taskpkg.StatusOpen}, nil).Maybe()
	bdExecutor.EXPECT().UpdateStatus(mock.Anything, mock.Anything).Return(nil).Maybe()

	proc := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     phasePtr(events.ProcessPhaseIdle),
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(proc)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewAssignTaskHandler(processRepo, taskRepo, WithBDExecutor(bdExecutor), WithQueueRepository(queueRepo))

	cmd := command.NewAssignTaskCommand(command.SourceMCPTool, "worker-1", "perles-abc1.2", "", "")
	result, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)

	require.Len(t, result.Events, 1)

	event, ok := result.Events[0].(events.ProcessEvent)
	require.True(t, ok, "expected ProcessEvent, got: %T", result.Events[0])

	require.Equal(t, events.ProcessStatusChange, event.Type)
	// Status is still Ready - DeliverProcessQueuedHandler sets Working
	require.Equal(t, events.ProcessStatusReady, event.Status)
	require.NotNil(t, event.Phase)
	require.Equal(t, events.ProcessPhaseImplementing, *event.Phase)
	require.Equal(t, "perles-abc1.2", event.TaskID)
}

func TestAssignTaskHandler_FailsForUnknownWorker(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	bdExecutor := mocks.NewMockTaskExecutor(t)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewAssignTaskHandler(processRepo, taskRepo, WithBDExecutor(bdExecutor), WithQueueRepository(queueRepo))

	cmd := command.NewAssignTaskCommand(command.SourceMCPTool, "unknown-worker", "perles-abc1.2", "", "")
	_, err := handler.Handle(context.Background(), cmd)

	require.Error(t, err, "expected error for unknown process")
	require.ErrorIs(t, err, ErrProcessNotFound)
}

func TestAssignTaskHandler_ReturnsAssignTaskResult(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	bdExecutor := mocks.NewMockTaskExecutor(t)
	// Mock for AssignTaskHandler: ShowIssue and UpdateStatus
	bdExecutor.EXPECT().ShowIssue(mock.Anything).Return(&taskpkg.Issue{ID: "perles-abc1.2", Status: taskpkg.StatusOpen}, nil).Maybe()
	bdExecutor.EXPECT().UpdateStatus(mock.Anything, mock.Anything).Return(nil).Maybe()

	proc := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     phasePtr(events.ProcessPhaseIdle),
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(proc)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewAssignTaskHandler(processRepo, taskRepo, WithBDExecutor(bdExecutor), WithQueueRepository(queueRepo))

	cmd := command.NewAssignTaskCommand(command.SourceMCPTool, "worker-1", "perles-abc1.2", "Implement feature X", "")
	result, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)

	assignResult, ok := result.Data.(*AssignTaskResult)
	require.True(t, ok, "expected AssignTaskResult, got: %T", result.Data)

	require.Equal(t, "worker-1", assignResult.WorkerID)
	require.Equal(t, "perles-abc1.2", assignResult.TaskID)
	require.Equal(t, "Implement feature X", assignResult.Summary)
}

// ===========================================================================
// AssignReviewHandler Tests
// ===========================================================================

func TestAssignReviewHandler_AssignsReviewer(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()

	// Add implementer (already working)
	implementer := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseAwaitingReview),
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(implementer)

	// Add ready reviewer
	reviewer := &repository.Process{
		ID:        "worker-2",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     phasePtr(events.ProcessPhaseIdle),
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(reviewer)

	// Add existing task
	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Status:      repository.TaskImplementing,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewAssignReviewHandler(processRepo, taskRepo, queueRepo)

	cmd := command.NewAssignReviewCommand(command.SourceMCPTool, "worker-2", "perles-abc1.2", "worker-1", command.ReviewTypeComplex)
	result, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)
	require.True(t, result.Success, "expected success, got failure: %v", result.Error)

	// Verify reviewer was updated - Status stays Ready until delivery
	updated, _ := processRepo.Get("worker-2")
	require.Equal(t, repository.StatusReady, updated.Status, "expected status StatusReady (delivery sets Working)")
	require.NotNil(t, updated.Phase)
	require.Equal(t, events.ProcessPhaseReviewing, *updated.Phase)
	require.Equal(t, "perles-abc1.2", updated.TaskID)

	// Verify follow-up command was created
	require.Len(t, result.FollowUp, 1)
	followUp, ok := result.FollowUp[0].(*command.DeliverProcessQueuedCommand)
	require.True(t, ok, "expected DeliverProcessQueuedCommand, got: %T", result.FollowUp[0])
	require.Equal(t, "worker-2", followUp.ProcessID)

	// Verify message was queued
	queue := queueRepo.GetOrCreate("worker-2")
	require.Equal(t, 1, queue.Size())

	// Verify task was updated
	updatedTask, _ := taskRepo.Get("perles-abc1.2")
	require.Equal(t, "worker-2", updatedTask.Reviewer)
	require.Equal(t, repository.TaskInReview, updatedTask.Status)
}

func TestAssignReviewHandler_FailsIfReviewerIsImplementer(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewAssignReviewHandler(processRepo, taskRepo, queueRepo)

	cmd := command.NewAssignReviewCommand(command.SourceMCPTool, "worker-1", "perles-abc1.2", "worker-1", command.ReviewTypeComplex)
	_, err := handler.Handle(context.Background(), cmd)

	require.Error(t, err, "expected error for reviewer == implementer")
	require.ErrorIs(t, err, types.ErrReviewerIsImplementer)
}

func TestAssignReviewHandler_FailsIfReviewerNotReady(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()

	// Add working reviewer (not ready)
	reviewer := &repository.Process{
		ID:        "worker-2",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseImplementing),
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(reviewer)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewAssignReviewHandler(processRepo, taskRepo, queueRepo)

	cmd := command.NewAssignReviewCommand(command.SourceMCPTool, "worker-2", "perles-abc1.2", "worker-1", command.ReviewTypeComplex)
	_, err := handler.Handle(context.Background(), cmd)

	require.Error(t, err, "expected error for non-ready reviewer")
	require.ErrorIs(t, err, types.ErrProcessNotReady)
}

func TestAssignReviewHandler_FailsIfReviewerNotIdlePhase(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()

	// Add ready reviewer but not in idle phase
	reviewer := &repository.Process{
		ID:        "worker-2",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     phasePtr(events.ProcessPhaseReviewing), // Wrong phase
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(reviewer)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewAssignReviewHandler(processRepo, taskRepo, queueRepo)

	cmd := command.NewAssignReviewCommand(command.SourceMCPTool, "worker-2", "perles-abc1.2", "worker-1", command.ReviewTypeComplex)
	_, err := handler.Handle(context.Background(), cmd)

	require.Error(t, err, "expected error for non-idle phase reviewer")
	require.ErrorIs(t, err, types.ErrProcessNotIdle)
}

func TestAssignReviewHandler_UpdatesReviewerPhaseToReviewing(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()

	reviewer := &repository.Process{
		ID:        "worker-2",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     phasePtr(events.ProcessPhaseIdle),
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(reviewer)

	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Status:      repository.TaskImplementing,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewAssignReviewHandler(processRepo, taskRepo, queueRepo)

	cmd := command.NewAssignReviewCommand(command.SourceMCPTool, "worker-2", "perles-abc1.2", "worker-1", command.ReviewTypeComplex)
	_, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)

	updated, _ := processRepo.Get("worker-2")
	require.NotNil(t, updated.Phase)
	require.Equal(t, events.ProcessPhaseReviewing, *updated.Phase)
}

func TestAssignReviewHandler_FailsForUnknownReviewer(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewAssignReviewHandler(processRepo, taskRepo, queueRepo)

	cmd := command.NewAssignReviewCommand(command.SourceMCPTool, "unknown-worker", "perles-abc1.2", "worker-1", command.ReviewTypeComplex)
	_, err := handler.Handle(context.Background(), cmd)

	require.Error(t, err, "expected error for unknown reviewer")
	require.ErrorIs(t, err, ErrProcessNotFound)
}

func TestAssignReviewHandler_FailsForUnknownTask(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()

	reviewer := &repository.Process{
		ID:        "worker-2",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     phasePtr(events.ProcessPhaseIdle),
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(reviewer)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewAssignReviewHandler(processRepo, taskRepo, queueRepo)

	cmd := command.NewAssignReviewCommand(command.SourceMCPTool, "worker-2", "unknown-task", "worker-1", command.ReviewTypeComplex)
	_, err := handler.Handle(context.Background(), cmd)

	require.Error(t, err, "expected error for unknown task")
}

func TestAssignReviewHandler_FailsForMismatchedImplementer(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()

	reviewer := &repository.Process{
		ID:        "worker-2",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     phasePtr(events.ProcessPhaseIdle),
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(reviewer)

	// Task has different implementer
	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-3", // Different from command
		Status:      repository.TaskImplementing,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewAssignReviewHandler(processRepo, taskRepo, queueRepo)

	cmd := command.NewAssignReviewCommand(command.SourceMCPTool, "worker-2", "perles-abc1.2", "worker-1", command.ReviewTypeComplex)
	_, err := handler.Handle(context.Background(), cmd)

	require.Error(t, err, "expected error for mismatched implementer")
	require.ErrorIs(t, err, types.ErrProcessNotImplementer)
}

func TestAssignReviewHandler_EmitsStatusChangeEvent(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()

	reviewer := &repository.Process{
		ID:        "worker-2",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     phasePtr(events.ProcessPhaseIdle),
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(reviewer)

	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Status:      repository.TaskImplementing,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewAssignReviewHandler(processRepo, taskRepo, queueRepo)

	cmd := command.NewAssignReviewCommand(command.SourceMCPTool, "worker-2", "perles-abc1.2", "worker-1", command.ReviewTypeComplex)
	result, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)

	require.Len(t, result.Events, 1)

	event, ok := result.Events[0].(events.ProcessEvent)
	require.True(t, ok, "expected ProcessEvent, got: %T", result.Events[0])

	require.Equal(t, events.ProcessStatusChange, event.Type)
	// Status is still Ready - DeliverProcessQueuedHandler sets Working
	require.Equal(t, events.ProcessStatusReady, event.Status)
	require.NotNil(t, event.Phase)
	require.Equal(t, events.ProcessPhaseReviewing, *event.Phase)
}

func TestAssignReviewHandler_ReturnsAssignReviewResult(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()

	reviewer := &repository.Process{
		ID:        "worker-2",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     phasePtr(events.ProcessPhaseIdle),
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(reviewer)

	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Status:      repository.TaskImplementing,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewAssignReviewHandler(processRepo, taskRepo, queueRepo)

	cmd := command.NewAssignReviewCommand(command.SourceMCPTool, "worker-2", "perles-abc1.2", "worker-1", command.ReviewTypeComplex)
	result, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)

	reviewResult, ok := result.Data.(*AssignReviewResult)
	require.True(t, ok, "expected AssignReviewResult, got: %T", result.Data)

	require.Equal(t, "worker-2", reviewResult.ReviewerID)
	require.Equal(t, "perles-abc1.2", reviewResult.TaskID)
	require.Equal(t, "worker-1", reviewResult.ImplementerID)
}

func TestAssignReviewHandler_UsesSimplePromptForSimpleReviewType(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()

	reviewer := &repository.Process{
		ID:        "worker-2",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     phasePtr(events.ProcessPhaseIdle),
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(reviewer)

	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Status:      repository.TaskImplementing,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewAssignReviewHandler(processRepo, taskRepo, queueRepo)

	cmd := command.NewAssignReviewCommand(command.SourceMCPTool, "worker-2", "perles-abc1.2", "worker-1", command.ReviewTypeSimple)
	_, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)

	// Verify message was queued and contains simple review prompt markers
	queue := queueRepo.GetOrCreate("worker-2")
	require.Equal(t, 1, queue.Size())

	msg, _ := queue.Dequeue()
	require.Contains(t, msg.Content, "[REVIEW ASSIGNMENT]", "expected review prompt")
	require.Contains(t, msg.Content, "CRITICAL: Run the tests", "expected mandatory test execution language")
}

func TestAssignReviewHandler_UsesComplexPromptForComplexReviewType(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()

	reviewer := &repository.Process{
		ID:        "worker-2",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     phasePtr(events.ProcessPhaseIdle),
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(reviewer)

	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Status:      repository.TaskImplementing,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewAssignReviewHandler(processRepo, taskRepo, queueRepo)

	cmd := command.NewAssignReviewCommand(command.SourceMCPTool, "worker-2", "perles-abc1.2", "worker-1", command.ReviewTypeComplex)
	_, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)

	// Verify message was queued and contains complex review prompt markers
	queue := queueRepo.GetOrCreate("worker-2")
	require.Equal(t, 1, queue.Size())

	msg, _ := queue.Dequeue()
	require.Contains(t, msg.Content, "[REVIEW ASSIGNMENT]", "expected complex review prompt")
	require.Contains(t, msg.Content, "Spawn 4 sub-agents", "expected sub-agent instruction")
	require.NotContains(t, msg.Content, "[SIMPLE REVIEW ASSIGNMENT]", "expected not to be simple prompt")
}

func TestAssignReviewHandler_UsesComplexPromptWhenReviewTypeEmpty(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()

	reviewer := &repository.Process{
		ID:        "worker-2",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     phasePtr(events.ProcessPhaseIdle),
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(reviewer)

	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Status:      repository.TaskImplementing,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewAssignReviewHandler(processRepo, taskRepo, queueRepo)

	// Empty ReviewType should default to complex
	cmd := command.NewAssignReviewCommand(command.SourceMCPTool, "worker-2", "perles-abc1.2", "worker-1", "")
	_, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)

	// Verify message was queued and contains complex review prompt markers
	queue := queueRepo.GetOrCreate("worker-2")
	require.Equal(t, 1, queue.Size())

	msg, _ := queue.Dequeue()
	require.Contains(t, msg.Content, "[REVIEW ASSIGNMENT]", "expected complex review prompt for empty type")
	require.Contains(t, msg.Content, "Spawn 4 sub-agents", "expected sub-agent instruction for empty type")
	require.NotContains(t, msg.Content, "[SIMPLE REVIEW ASSIGNMENT]", "expected not to be simple prompt")
}

// ===========================================================================
// ApproveCommitHandler Tests
// ===========================================================================

func TestApproveCommitHandler_TransitionsToCommitting(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()

	// Add implementer awaiting review
	implementer := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseAwaitingReview),
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(implementer)

	// Add approved task
	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Reviewer:    "worker-2",
		Status:      repository.TaskApproved,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewApproveCommitHandler(processRepo, taskRepo, queueRepo)

	cmd := command.NewApproveCommitCommand(command.SourceMCPTool, "worker-1", "perles-abc1.2")
	result, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)
	require.True(t, result.Success, "expected success, got failure: %v", result.Error)

	// Verify implementer was updated
	updated, _ := processRepo.Get("worker-1")
	require.NotNil(t, updated.Phase)
	require.Equal(t, events.ProcessPhaseCommitting, *updated.Phase)

	// Verify task was updated
	updatedTask, _ := taskRepo.Get("perles-abc1.2")
	require.Equal(t, repository.TaskCommitting, updatedTask.Status)
}

func TestApproveCommitHandler_FailsIfNotApproved(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()

	implementer := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseAwaitingReview),
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(implementer)

	// Task is in review, not approved
	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Reviewer:    "worker-2",
		Status:      repository.TaskInReview,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewApproveCommitHandler(processRepo, taskRepo, queueRepo)

	cmd := command.NewApproveCommitCommand(command.SourceMCPTool, "worker-1", "perles-abc1.2")
	_, err := handler.Handle(context.Background(), cmd)

	require.Error(t, err, "expected error for non-approved task")
	require.ErrorIs(t, err, types.ErrTaskNotApproved)
}

func TestApproveCommitHandler_FailsIfNotAwaitingReview(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()

	// Implementer in wrong phase
	implementer := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseImplementing), // Wrong phase
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(implementer)

	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Reviewer:    "worker-2",
		Status:      repository.TaskApproved,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewApproveCommitHandler(processRepo, taskRepo, queueRepo)

	cmd := command.NewApproveCommitCommand(command.SourceMCPTool, "worker-1", "perles-abc1.2")
	_, err := handler.Handle(context.Background(), cmd)

	require.Error(t, err, "expected error for process not awaiting review")
	require.ErrorIs(t, err, types.ErrProcessNotAwaitingReview)
}

func TestApproveCommitHandler_FailsForUnknownTask(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewApproveCommitHandler(processRepo, taskRepo, queueRepo)

	cmd := command.NewApproveCommitCommand(command.SourceMCPTool, "worker-1", "unknown-task")
	_, err := handler.Handle(context.Background(), cmd)

	require.Error(t, err, "expected error for unknown task")
}

func TestApproveCommitHandler_FailsForMismatchedImplementer(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()

	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-3", // Different implementer
		Reviewer:    "worker-2",
		Status:      repository.TaskApproved,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewApproveCommitHandler(processRepo, taskRepo, queueRepo)

	cmd := command.NewApproveCommitCommand(command.SourceMCPTool, "worker-1", "perles-abc1.2")
	_, err := handler.Handle(context.Background(), cmd)

	require.Error(t, err, "expected error for mismatched implementer")
	require.ErrorIs(t, err, types.ErrProcessNotImplementer)
}

func TestApproveCommitHandler_FailsForUnknownImplementer(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()

	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "unknown-worker",
		Reviewer:    "worker-2",
		Status:      repository.TaskApproved,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewApproveCommitHandler(processRepo, taskRepo, queueRepo)

	cmd := command.NewApproveCommitCommand(command.SourceMCPTool, "unknown-worker", "perles-abc1.2")
	_, err := handler.Handle(context.Background(), cmd)

	require.Error(t, err, "expected error for unknown implementer")
	require.ErrorIs(t, err, ErrProcessNotFound)
}

func TestApproveCommitHandler_EmitsStatusChangeEvent(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()

	implementer := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseAwaitingReview),
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(implementer)

	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Reviewer:    "worker-2",
		Status:      repository.TaskApproved,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewApproveCommitHandler(processRepo, taskRepo, queueRepo)

	cmd := command.NewApproveCommitCommand(command.SourceMCPTool, "worker-1", "perles-abc1.2")
	result, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)

	require.Len(t, result.Events, 1)

	event, ok := result.Events[0].(events.ProcessEvent)
	require.True(t, ok, "expected ProcessEvent, got: %T", result.Events[0])

	require.Equal(t, events.ProcessStatusChange, event.Type)
	require.NotNil(t, event.Phase)
	require.Equal(t, events.ProcessPhaseCommitting, *event.Phase)
}

func TestApproveCommitHandler_ReturnsApproveCommitResult(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()

	implementer := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseAwaitingReview),
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(implementer)

	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Reviewer:    "worker-2",
		Status:      repository.TaskApproved,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewApproveCommitHandler(processRepo, taskRepo, queueRepo)

	cmd := command.NewApproveCommitCommand(command.SourceMCPTool, "worker-1", "perles-abc1.2")
	result, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)

	approveResult, ok := result.Data.(*ApproveCommitResult)
	require.True(t, ok, "expected ApproveCommitResult, got: %T", result.Data)

	require.Equal(t, "worker-1", approveResult.ImplementerID)
	require.Equal(t, "perles-abc1.2", approveResult.TaskID)
}

// ===========================================================================
// Integration Test: Full Assign-Review-Approve Workflow
// ===========================================================================

func TestFullAssignReviewApproveWorkflow(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	bdExecutor := mocks.NewMockTaskExecutor(t)
	// Mock for AssignTaskHandler: ShowIssue and UpdateStatus
	bdExecutor.EXPECT().ShowIssue(mock.Anything).Return(&taskpkg.Issue{ID: "perles-abc1.2", Status: taskpkg.StatusOpen}, nil).Maybe()
	bdExecutor.EXPECT().UpdateStatus(mock.Anything, mock.Anything).Return(nil).Maybe()

	// Add two processes
	implementer := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     phasePtr(events.ProcessPhaseIdle),
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(implementer)

	reviewer := &repository.Process{
		ID:        "worker-2",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     phasePtr(events.ProcessPhaseIdle),
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(reviewer)

	// Step 1: Assign task to implementer
	queueRepo := repository.NewMemoryQueueRepository(0)
	assignHandler := NewAssignTaskHandler(processRepo, taskRepo, WithBDExecutor(bdExecutor), WithQueueRepository(queueRepo))
	assignCmd := command.NewAssignTaskCommand(command.SourceMCPTool, "worker-1", "perles-abc1.2", "", "")
	result, err := assignHandler.Handle(context.Background(), assignCmd)
	require.NoError(t, err, "assign task error")
	require.True(t, result.Success, "assign task failed: %v", result.Error)

	// Verify implementer state - Status is still Ready (delivery sets Working)
	impl, _ := processRepo.Get("worker-1")
	require.Equal(t, repository.StatusReady, impl.Status)
	require.NotNil(t, impl.Phase)
	require.Equal(t, events.ProcessPhaseImplementing, *impl.Phase)

	// Simulate delivery handler being called - sets StatusWorking
	impl.Status = repository.StatusWorking
	_ = processRepo.Save(impl)

	// Step 2: Simulate implementation complete - transition to awaiting review
	awaitingReview := events.ProcessPhaseAwaitingReview
	impl.Phase = &awaitingReview
	_ = processRepo.Save(impl)

	// Update task status to be ready for review
	task, _ := taskRepo.Get("perles-abc1.2")
	task.Status = repository.TaskImplementing
	_ = taskRepo.Save(task)

	// Step 3: Assign reviewer
	reviewHandler := NewAssignReviewHandler(processRepo, taskRepo, queueRepo)
	reviewCmd := command.NewAssignReviewCommand(command.SourceMCPTool, "worker-2", "perles-abc1.2", "worker-1", command.ReviewTypeComplex)
	result, err = reviewHandler.Handle(context.Background(), reviewCmd)
	require.NoError(t, err, "assign review error")
	require.True(t, result.Success, "assign review failed: %v", result.Error)

	// Verify reviewer state - Status is still Ready (delivery sets Working)
	rev, _ := processRepo.Get("worker-2")
	require.Equal(t, repository.StatusReady, rev.Status)
	require.NotNil(t, rev.Phase)
	require.Equal(t, events.ProcessPhaseReviewing, *rev.Phase)

	// Simulate delivery handler being called - sets StatusWorking
	rev.Status = repository.StatusWorking
	_ = processRepo.Save(rev)

	// Step 4: Simulate review approval
	task, _ = taskRepo.Get("perles-abc1.2")
	task.Status = repository.TaskApproved
	_ = taskRepo.Save(task)

	// Step 5: Approve commit
	approveHandler := NewApproveCommitHandler(processRepo, taskRepo, queueRepo)
	approveCmd := command.NewApproveCommitCommand(command.SourceMCPTool, "worker-1", "perles-abc1.2")
	result, err = approveHandler.Handle(context.Background(), approveCmd)
	require.NoError(t, err, "approve commit error")
	require.True(t, result.Success, "approve commit failed: %v", result.Error)

	// Verify final state
	impl, _ = processRepo.Get("worker-1")
	require.NotNil(t, impl.Phase)
	require.Equal(t, events.ProcessPhaseCommitting, *impl.Phase)

	task, _ = taskRepo.Get("perles-abc1.2")
	require.Equal(t, repository.TaskCommitting, task.Status)
}

// ===========================================================================
// Synchronous BD Error Propagation Tests
// ===========================================================================

func TestAssignTaskHandler_FailsOnBDError(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	bdExecutor := mocks.NewMockTaskExecutor(t)
	// ShowIssue must succeed before UpdateStatus fails
	bdExecutor.EXPECT().ShowIssue(mock.Anything).Return(&taskpkg.Issue{ID: "perles-test123", Status: taskpkg.StatusOpen}, nil)
	bdExecutor.EXPECT().UpdateStatus(mock.Anything, mock.Anything).Return(errors.New("bd CLI connection failed"))

	proc := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     phasePtr(events.ProcessPhaseIdle),
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(proc)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewAssignTaskHandler(processRepo, taskRepo, WithBDExecutor(bdExecutor), WithQueueRepository(queueRepo))

	cmd := command.NewAssignTaskCommand(command.SourceMCPTool, "worker-1", "perles-test123", "Test task", "")
	_, err := handler.Handle(context.Background(), cmd)

	// Command should fail due to BD error
	require.Error(t, err, "expected error on BD failure, got nil")

	// Verify error contains the original BD error
	require.Contains(t, err.Error(), "bd CLI connection failed")
	require.Contains(t, err.Error(), "failed to update BD task status")
}

func TestAssignTaskHandler_PropagatesBDError(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	bdExecutor := mocks.NewMockTaskExecutor(t)
	// ShowIssue must succeed before UpdateStatus fails
	bdExecutor.EXPECT().ShowIssue(mock.Anything).Return(&taskpkg.Issue{ID: "perles-abc1.2", Status: taskpkg.StatusOpen}, nil)
	bdExecutor.EXPECT().UpdateStatus(mock.Anything, mock.Anything).Return(errors.New("bd database locked"))

	proc := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     phasePtr(events.ProcessPhaseIdle),
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(proc)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewAssignTaskHandler(processRepo, taskRepo, WithBDExecutor(bdExecutor), WithQueueRepository(queueRepo))

	cmd := command.NewAssignTaskCommand(command.SourceMCPTool, "worker-1", "perles-abc1.2", "Test task", "")
	_, err := handler.Handle(context.Background(), cmd)

	// Verify command failed with BD error
	require.Error(t, err, "expected error on BD failure, got nil")
	require.Contains(t, err.Error(), "bd database locked")
}

func TestAssignTaskHandler_PanicsIfBDExecutorNil(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()

	require.Panics(t, func() {
		NewAssignTaskHandler(processRepo, taskRepo)
	}, "expected panic when bdExecutor is nil")
}

// ===========================================================================
// AssignReviewFeedbackHandler Tests
// ===========================================================================

func TestAssignReviewFeedbackHandler_TransitionsToAddressingFeedback(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()

	// Add implementer awaiting review
	implementer := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseAwaitingReview),
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(implementer)

	// Add denied task
	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Reviewer:    "worker-2",
		Status:      repository.TaskDenied,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewAssignReviewFeedbackHandler(processRepo, taskRepo, queueRepo)

	cmd := command.NewAssignReviewFeedbackCommand(command.SourceMCPTool, "worker-1", "perles-abc1.2", "Please fix the error handling")
	result, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)
	require.True(t, result.Success, "expected success, got failure: %v", result.Error)

	// Verify implementer was updated
	updated, _ := processRepo.Get("worker-1")
	require.NotNil(t, updated.Phase)
	require.Equal(t, events.ProcessPhaseAddressingFeedback, *updated.Phase)

	// Verify task was updated back to implementing
	updatedTask, _ := taskRepo.Get("perles-abc1.2")
	require.Equal(t, repository.TaskImplementing, updatedTask.Status)

	// Verify message was queued
	queue := queueRepo.GetOrCreate("worker-1")
	require.Equal(t, 1, queue.Size())
}

func TestAssignReviewFeedbackHandler_FailsIfNotDenied(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()

	implementer := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseAwaitingReview),
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(implementer)

	// Task is in review, not denied
	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Reviewer:    "worker-2",
		Status:      repository.TaskInReview,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewAssignReviewFeedbackHandler(processRepo, taskRepo, queueRepo)

	cmd := command.NewAssignReviewFeedbackCommand(command.SourceMCPTool, "worker-1", "perles-abc1.2", "Feedback")
	_, err := handler.Handle(context.Background(), cmd)

	require.Error(t, err, "expected error for non-denied task")
	require.Contains(t, err.Error(), "denied")
}

func TestAssignReviewFeedbackHandler_FailsIfNotAwaitingReview(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()

	// Implementer in wrong phase
	implementer := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseImplementing), // Wrong phase
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(implementer)

	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Reviewer:    "worker-2",
		Status:      repository.TaskDenied,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewAssignReviewFeedbackHandler(processRepo, taskRepo, queueRepo)

	cmd := command.NewAssignReviewFeedbackCommand(command.SourceMCPTool, "worker-1", "perles-abc1.2", "Feedback")
	_, err := handler.Handle(context.Background(), cmd)

	require.Error(t, err, "expected error for process not awaiting review")
	require.ErrorIs(t, err, types.ErrProcessNotAwaitingReview)
}

func TestAssignReviewFeedbackHandler_PanicsIfQueueRepoNil(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()

	require.Panics(t, func() {
		NewAssignReviewFeedbackHandler(processRepo, taskRepo, nil)
	}, "expected panic when queueRepo is nil")
}
