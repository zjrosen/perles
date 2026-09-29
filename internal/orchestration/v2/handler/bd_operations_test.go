package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
	taskpkg "github.com/zjrosen/perles/internal/task"
)

// ===========================================================================
// MarkTaskCompleteHandler Tests
// ===========================================================================

func TestMarkTaskCompleteHandler_Success(t *testing.T) {
	bdExecutor := mocks.NewMockTaskExecutor(t)
	bdExecutor.EXPECT().UpdateStatus("perles-abc1.2", taskpkg.StatusClosed).Return(nil)
	bdExecutor.EXPECT().AddComment("perles-abc1.2", "coordinator", "Task completed").Return(nil)

	handler := NewMarkTaskCompleteHandler(bdExecutor, nil)

	cmd := command.NewMarkTaskCompleteCommand(command.SourceMCPTool, "perles-abc1.2")
	result, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)
	require.True(t, result.Success, "expected success, got failure: %v", result.Error)
}

func TestMarkTaskCompleteHandler_ReturnsResult(t *testing.T) {
	bdExecutor := mocks.NewMockTaskExecutor(t)
	bdExecutor.EXPECT().UpdateStatus("perles-abc1.2", taskpkg.StatusClosed).Return(nil)
	bdExecutor.EXPECT().AddComment("perles-abc1.2", "coordinator", "Task completed").Return(nil)

	handler := NewMarkTaskCompleteHandler(bdExecutor, nil)

	cmd := command.NewMarkTaskCompleteCommand(command.SourceMCPTool, "perles-abc1.2")
	result, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)

	completeResult, ok := result.Data.(*MarkTaskCompleteResult)
	require.True(t, ok, "expected MarkTaskCompleteResult, got: %T", result.Data)
	require.Equal(t, "perles-abc1.2", completeResult.TaskID)
}

func TestMarkTaskCompleteHandler_FailsOnUpdateStatusError(t *testing.T) {
	bdExecutor := mocks.NewMockTaskExecutor(t)
	bdExecutor.EXPECT().UpdateStatus(mock.Anything, mock.Anything).Return(errors.New("bd database locked"))

	handler := NewMarkTaskCompleteHandler(bdExecutor, nil)

	cmd := command.NewMarkTaskCompleteCommand(command.SourceMCPTool, "perles-abc1.2")
	_, err := handler.Handle(context.Background(), cmd)

	require.Error(t, err, "expected error on BD UpdateTaskStatus failure")
	require.Contains(t, err.Error(), "bd database locked")
	require.Contains(t, err.Error(), "failed to update BD task status")
}

func TestMarkTaskCompleteHandler_FailsOnAddCommentError(t *testing.T) {
	bdExecutor := mocks.NewMockTaskExecutor(t)
	bdExecutor.EXPECT().UpdateStatus(mock.Anything, mock.Anything).Return(nil)
	bdExecutor.EXPECT().AddComment(mock.Anything, mock.Anything, mock.Anything).Return(errors.New("bd comment service unavailable"))

	handler := NewMarkTaskCompleteHandler(bdExecutor, nil)

	cmd := command.NewMarkTaskCompleteCommand(command.SourceMCPTool, "perles-abc1.2")
	_, err := handler.Handle(context.Background(), cmd)

	require.Error(t, err, "expected error on BD AddComment failure")
	require.Contains(t, err.Error(), "bd comment service unavailable")
	require.Contains(t, err.Error(), "failed to add BD comment")
}

func TestMarkTaskCompleteHandler_PanicsIfBDExecutorNil(t *testing.T) {
	require.Panics(t, func() {
		NewMarkTaskCompleteHandler(nil, nil)
	}, "expected panic when bdExecutor is nil")
}

func TestMarkTaskCompleteHandler_DeletesTaskFromRepository(t *testing.T) {
	bdExecutor := mocks.NewMockTaskExecutor(t)
	bdExecutor.EXPECT().UpdateStatus("perles-abc1.2", taskpkg.StatusClosed).Return(nil)
	bdExecutor.EXPECT().AddComment("perles-abc1.2", "coordinator", "Task completed").Return(nil)

	// Create task repo with a task
	taskRepo := repository.NewMemoryTaskRepository()
	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Status:      repository.TaskImplementing,
	}
	require.NoError(t, taskRepo.Save(task))

	// Create process repo with the implementer worker
	processRepo := repository.NewMemoryProcessRepository()
	awaitingReview := events.ProcessPhaseAwaitingReview
	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusReady,
		Phase:  &awaitingReview,
		TaskID: "perles-abc1.2",
	}
	require.NoError(t, processRepo.Save(worker))

	// Verify task exists before handler
	_, err := taskRepo.Get("perles-abc1.2")
	require.NoError(t, err, "task should exist before handle")

	handler := NewMarkTaskCompleteHandler(bdExecutor, taskRepo,
		WithMarkTaskCompleteProcessRepo(processRepo))

	cmd := command.NewMarkTaskCompleteCommand(command.SourceMCPTool, "perles-abc1.2")
	result, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)
	require.True(t, result.Success)

	// Verify task was deleted
	_, err = taskRepo.Get("perles-abc1.2")
	require.ErrorIs(t, err, repository.ErrTaskNotFound, "task should be deleted after handle")

	// Verify implementer was reset to idle
	updated, err := processRepo.Get("worker-1")
	require.NoError(t, err)
	require.NotNil(t, updated.Phase)
	require.Equal(t, events.ProcessPhaseIdle, *updated.Phase)
	require.Equal(t, repository.StatusReady, updated.Status)
	require.Empty(t, updated.TaskID, "TaskID should be cleared")
}

func TestMarkTaskCompleteHandler_SucceedsWhenTaskNotInRepo(t *testing.T) {
	bdExecutor := mocks.NewMockTaskExecutor(t)
	bdExecutor.EXPECT().UpdateStatus("perles-abc1.2", taskpkg.StatusClosed).Return(nil)
	bdExecutor.EXPECT().AddComment("perles-abc1.2", "coordinator", "Task completed").Return(nil)

	// Create empty task repo (task doesn't exist in memory)
	taskRepo := repository.NewMemoryTaskRepository()

	handler := NewMarkTaskCompleteHandler(bdExecutor, taskRepo)

	cmd := command.NewMarkTaskCompleteCommand(command.SourceMCPTool, "perles-abc1.2")
	result, err := handler.Handle(context.Background(), cmd)

	// Should succeed even though task wasn't in memory
	require.NoError(t, err)
	require.True(t, result.Success)
}

func TestMarkTaskCompleteHandler_WorksWithNilTaskRepo(t *testing.T) {
	bdExecutor := mocks.NewMockTaskExecutor(t)
	bdExecutor.EXPECT().UpdateStatus("perles-abc1.2", taskpkg.StatusClosed).Return(nil)
	bdExecutor.EXPECT().AddComment("perles-abc1.2", "coordinator", "Task completed").Return(nil)

	// Construct handler with nil taskRepo (backward compatibility)
	handler := NewMarkTaskCompleteHandler(bdExecutor, nil)

	cmd := command.NewMarkTaskCompleteCommand(command.SourceMCPTool, "perles-abc1.2")
	result, err := handler.Handle(context.Background(), cmd)

	// Should succeed without panic even with nil taskRepo
	require.NoError(t, err)
	require.True(t, result.Success)

	completeResult, ok := result.Data.(*MarkTaskCompleteResult)
	require.True(t, ok, "expected MarkTaskCompleteResult, got: %T", result.Data)
	require.Equal(t, "perles-abc1.2", completeResult.TaskID)
}

func TestMarkTaskCompleteHandler_ResetsImplementerAndReviewer(t *testing.T) {
	bdExecutor := mocks.NewMockTaskExecutor(t)
	bdExecutor.EXPECT().UpdateStatus("perles-abc1.2", taskpkg.StatusClosed).Return(nil)
	bdExecutor.EXPECT().AddComment("perles-abc1.2", "coordinator", "Task completed").Return(nil)

	// Create task with both implementer and reviewer
	taskRepo := repository.NewMemoryTaskRepository()
	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Reviewer:    "worker-2",
		Status:      repository.TaskInReview,
	}
	require.NoError(t, taskRepo.Save(task))

	// Create processes for both workers in non-idle states
	processRepo := repository.NewMemoryProcessRepository()
	awaitingReview := events.ProcessPhaseAwaitingReview
	reviewing := events.ProcessPhaseReviewing
	worker1 := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusReady,
		Phase:  &awaitingReview,
		TaskID: "perles-abc1.2",
	}
	worker2 := &repository.Process{
		ID:     "worker-2",
		Role:   repository.RoleWorker,
		Status: repository.StatusWorking,
		Phase:  &reviewing,
		TaskID: "perles-abc1.2",
	}
	require.NoError(t, processRepo.Save(worker1))
	require.NoError(t, processRepo.Save(worker2))

	handler := NewMarkTaskCompleteHandler(bdExecutor, taskRepo,
		WithMarkTaskCompleteProcessRepo(processRepo))

	cmd := command.NewMarkTaskCompleteCommand(command.SourceMCPTool, "perles-abc1.2")
	result, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)
	require.True(t, result.Success)

	// Verify both workers were reset to idle
	for _, workerID := range []string{"worker-1", "worker-2"} {
		updated, err := processRepo.Get(workerID)
		require.NoError(t, err, "worker %s should exist", workerID)
		require.NotNil(t, updated.Phase, "worker %s phase should not be nil", workerID)
		require.Equal(t, events.ProcessPhaseIdle, *updated.Phase, "worker %s should be idle", workerID)
		require.Equal(t, repository.StatusReady, updated.Status, "worker %s should be ready", workerID)
		require.Empty(t, updated.TaskID, "worker %s TaskID should be cleared", workerID)
	}

	// Verify events were emitted for both workers
	require.NotNil(t, result.Events)
	require.Len(t, result.Events, 2, "should emit status change events for both workers")
}

func TestMarkTaskCompleteHandler_EmitsEventsForResetWorkers(t *testing.T) {
	bdExecutor := mocks.NewMockTaskExecutor(t)
	bdExecutor.EXPECT().UpdateStatus("perles-abc1.2", taskpkg.StatusClosed).Return(nil)
	bdExecutor.EXPECT().AddComment("perles-abc1.2", "coordinator", "Task completed").Return(nil)

	taskRepo := repository.NewMemoryTaskRepository()
	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Status:      repository.TaskImplementing,
	}
	require.NoError(t, taskRepo.Save(task))

	processRepo := repository.NewMemoryProcessRepository()
	implementing := events.ProcessPhaseImplementing
	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusWorking,
		Phase:  &implementing,
		TaskID: "perles-abc1.2",
	}
	require.NoError(t, processRepo.Save(worker))

	handler := NewMarkTaskCompleteHandler(bdExecutor, taskRepo,
		WithMarkTaskCompleteProcessRepo(processRepo))

	cmd := command.NewMarkTaskCompleteCommand(command.SourceMCPTool, "perles-abc1.2")
	result, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)
	require.True(t, result.Success)
	require.Len(t, result.Events, 1)

	// Verify the event is a ProcessStatusChange with idle phase
	event, ok := result.Events[0].(events.ProcessEvent)
	require.True(t, ok, "expected ProcessEvent, got %T", result.Events[0])
	require.Equal(t, events.ProcessStatusChange, event.Type)
	require.Equal(t, "worker-1", event.ProcessID)
	require.Equal(t, events.ProcessStatusReady, event.Status)
	require.NotNil(t, event.Phase)
	require.Equal(t, events.ProcessPhaseIdle, *event.Phase)
}

func TestMarkTaskCompleteHandler_SkipsRetiredProcess(t *testing.T) {
	bdExecutor := mocks.NewMockTaskExecutor(t)
	bdExecutor.EXPECT().UpdateStatus("perles-abc1.2", taskpkg.StatusClosed).Return(nil)
	bdExecutor.EXPECT().AddComment("perles-abc1.2", "coordinator", "Task completed").Return(nil)

	taskRepo := repository.NewMemoryTaskRepository()
	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Status:      repository.TaskImplementing,
	}
	require.NoError(t, taskRepo.Save(task))

	// Worker is already retired
	processRepo := repository.NewMemoryProcessRepository()
	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusRetired,
		TaskID: "perles-abc1.2",
	}
	require.NoError(t, processRepo.Save(worker))

	handler := NewMarkTaskCompleteHandler(bdExecutor, taskRepo,
		WithMarkTaskCompleteProcessRepo(processRepo))

	cmd := command.NewMarkTaskCompleteCommand(command.SourceMCPTool, "perles-abc1.2")
	result, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)
	require.True(t, result.Success)

	// Verify retired worker was NOT modified
	updated, err := processRepo.Get("worker-1")
	require.NoError(t, err)
	require.Equal(t, repository.StatusRetired, updated.Status, "retired worker should not be changed")
}

func TestMarkTaskCompleteHandler_GracefulWithMissingProcess(t *testing.T) {
	bdExecutor := mocks.NewMockTaskExecutor(t)
	bdExecutor.EXPECT().UpdateStatus("perles-abc1.2", taskpkg.StatusClosed).Return(nil)
	bdExecutor.EXPECT().AddComment("perles-abc1.2", "coordinator", "Task completed").Return(nil)

	taskRepo := repository.NewMemoryTaskRepository()
	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-99", // This worker doesn't exist in processRepo
		Status:      repository.TaskImplementing,
	}
	require.NoError(t, taskRepo.Save(task))

	// Empty process repo - worker-99 doesn't exist
	processRepo := repository.NewMemoryProcessRepository()

	handler := NewMarkTaskCompleteHandler(bdExecutor, taskRepo,
		WithMarkTaskCompleteProcessRepo(processRepo))

	cmd := command.NewMarkTaskCompleteCommand(command.SourceMCPTool, "perles-abc1.2")
	result, err := handler.Handle(context.Background(), cmd)

	// Should succeed gracefully even though process doesn't exist
	require.NoError(t, err)
	require.True(t, result.Success)
}

func TestMarkTaskCompleteHandler_SkipsAlreadyIdleProcess(t *testing.T) {
	bdExecutor := mocks.NewMockTaskExecutor(t)
	bdExecutor.EXPECT().UpdateStatus("perles-abc1.2", taskpkg.StatusClosed).Return(nil)
	bdExecutor.EXPECT().AddComment("perles-abc1.2", "coordinator", "Task completed").Return(nil)

	taskRepo := repository.NewMemoryTaskRepository()
	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Status:      repository.TaskImplementing,
	}
	require.NoError(t, taskRepo.Save(task))

	// Worker is already idle/ready with no task
	processRepo := repository.NewMemoryProcessRepository()
	idle := events.ProcessPhaseIdle
	worker := &repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusReady,
		Phase:  &idle,
		TaskID: "",
	}
	require.NoError(t, processRepo.Save(worker))

	handler := NewMarkTaskCompleteHandler(bdExecutor, taskRepo,
		WithMarkTaskCompleteProcessRepo(processRepo))

	cmd := command.NewMarkTaskCompleteCommand(command.SourceMCPTool, "perles-abc1.2")
	result, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)
	require.True(t, result.Success)

	// No events should be emitted - worker was already idle
	require.Empty(t, result.Events, "no events should be emitted for already-idle worker")

	// Worker should still be idle/ready
	updated, err := processRepo.Get("worker-1")
	require.NoError(t, err)
	require.Equal(t, repository.StatusReady, updated.Status)
	require.Equal(t, events.ProcessPhaseIdle, *updated.Phase)
}

func TestMarkTaskCompleteHandler_WorksWithNilProcessRepo(t *testing.T) {
	bdExecutor := mocks.NewMockTaskExecutor(t)
	bdExecutor.EXPECT().UpdateStatus("perles-abc1.2", taskpkg.StatusClosed).Return(nil)
	bdExecutor.EXPECT().AddComment("perles-abc1.2", "coordinator", "Task completed").Return(nil)

	// Task repo has a task but no processRepo provided (backward compatibility)
	taskRepo := repository.NewMemoryTaskRepository()
	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Status:      repository.TaskImplementing,
	}
	require.NoError(t, taskRepo.Save(task))

	// No options - processRepo is nil
	handler := NewMarkTaskCompleteHandler(bdExecutor, taskRepo)

	cmd := command.NewMarkTaskCompleteCommand(command.SourceMCPTool, "perles-abc1.2")
	result, err := handler.Handle(context.Background(), cmd)

	// Should succeed without process reset (backward compatible)
	require.NoError(t, err)
	require.True(t, result.Success)

	// Task should still be deleted
	_, err = taskRepo.Get("perles-abc1.2")
	require.ErrorIs(t, err, repository.ErrTaskNotFound)
}

// ===========================================================================
// MarkTaskFailedHandler Tests
// ===========================================================================

func TestMarkTaskFailedHandler_Success(t *testing.T) {
	bdExecutor := mocks.NewMockTaskExecutor(t)
	// MarkTaskFailed only calls AddComment, not UpdateStatus
	bdExecutor.EXPECT().AddComment("perles-abc1.2", "coordinator", "Task failed: Build failed due to missing dependency").Return(nil)

	handler := NewMarkTaskFailedHandler(bdExecutor)

	cmd := command.NewMarkTaskFailedCommand(command.SourceMCPTool, "perles-abc1.2", "Build failed due to missing dependency")
	result, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)
	require.True(t, result.Success, "expected success, got failure: %v", result.Error)
}

func TestMarkTaskFailedHandler_ReturnsResult(t *testing.T) {
	bdExecutor := mocks.NewMockTaskExecutor(t)
	bdExecutor.EXPECT().AddComment("perles-abc1.2", "coordinator", "Task failed: Tests failing").Return(nil)

	handler := NewMarkTaskFailedHandler(bdExecutor)

	cmd := command.NewMarkTaskFailedCommand(command.SourceMCPTool, "perles-abc1.2", "Tests failing")
	result, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)

	failedResult, ok := result.Data.(*MarkTaskFailedResult)
	require.True(t, ok, "expected MarkTaskFailedResult, got: %T", result.Data)
	require.Equal(t, "perles-abc1.2", failedResult.TaskID)
	require.Equal(t, "Tests failing", failedResult.Reason)
}

func TestMarkTaskFailedHandler_FailsOnAddCommentError(t *testing.T) {
	bdExecutor := mocks.NewMockTaskExecutor(t)
	bdExecutor.EXPECT().AddComment(mock.Anything, mock.Anything, mock.Anything).Return(errors.New("bd comment service unavailable"))

	handler := NewMarkTaskFailedHandler(bdExecutor)

	cmd := command.NewMarkTaskFailedCommand(command.SourceMCPTool, "perles-abc1.2", "Some reason")
	_, err := handler.Handle(context.Background(), cmd)

	require.Error(t, err, "expected error on BD AddComment failure")
	require.Contains(t, err.Error(), "bd comment service unavailable")
	require.Contains(t, err.Error(), "failed to add BD comment")
}

func TestMarkTaskFailedHandler_PanicsIfBDExecutorNil(t *testing.T) {
	require.Panics(t, func() {
		NewMarkTaskFailedHandler(nil)
	}, "expected panic when bdExecutor is nil")
}

func TestMarkTaskFailedHandler_DoesNotUpdateStatus(t *testing.T) {
	bdExecutor := mocks.NewMockTaskExecutor(t)
	// MarkTaskFailed only calls AddComment, not UpdateStatus - verify UpdateStatus is never called
	bdExecutor.EXPECT().AddComment("perles-abc1.2", "coordinator", "Task failed: Some reason").Return(nil)

	handler := NewMarkTaskFailedHandler(bdExecutor)

	cmd := command.NewMarkTaskFailedCommand(command.SourceMCPTool, "perles-abc1.2", "Some reason")
	_, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)
	// mockery will fail if UpdateStatus is unexpectedly called
}
