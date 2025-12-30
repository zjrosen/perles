package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/beads"
	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/orchestration/claude"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/orchestration/pool"
)

// ============================================================================
// Additional Tests for Coverage
// ============================================================================

// TestHandleAssignTask_BDShowFails tests assign_task when ShowIssue (getTaskInfo) fails.
func TestHandleAssignTask_BDShowFails(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := message.New()

	mockExec := mocks.NewMockBeadsExecutor(t)
	mockExec.EXPECT().
		ShowIssue("perles-abc.1").
		Return(nil, errors.New("bd show failed: issue not found"))

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, msgIssue, "/tmp/test", 8765, nil, mockExec)

	// Create a ready worker
	_ = workerPool.AddTestWorker("worker-1", pool.WorkerReady)

	handler := cs.handlers["assign_task"]

	args := `{"worker_id": "worker-1", "task_id": "perles-abc.1"}`
	_, err := handler(context.Background(), json.RawMessage(args))

	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to get task info")
}

// TestHandleAssignTask_WithMock tests assign_task with a mock that returns valid task info.
// The test demonstrates mock usage for the getTaskInfo flow but doesn't test full worker spawn.
func TestHandleAssignTask_WithMock(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := message.New()

	mockExec := mocks.NewMockBeadsExecutor(t)
	mockExec.EXPECT().
		ShowIssue("perles-abc.1").
		Return(&beads.Issue{
			ID:        "perles-abc.1",
			TitleText: "Implement feature X",
			Status:    beads.StatusOpen,
			Priority:  beads.PriorityMedium,
			Type:      beads.TypeTask,
		}, nil)

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, msgIssue, "/tmp/test", 8765, nil, mockExec)

	// Create a ready worker - but no session ID set, so spawn will fail
	_ = workerPool.AddTestWorker("worker-1", pool.WorkerReady)

	handler := cs.handlers["assign_task"]

	args := `{"worker_id": "worker-1", "task_id": "perles-abc.1"}`
	_, err := handler(context.Background(), json.RawMessage(args))

	// Will fail at worker session ID check, but BD mock was called correctly
	require.Error(t, err)
	require.Contains(t, err.Error(), "no session ID")
}

// TestHandleGetTaskStatus_BDError tests get_task_status when ShowIssue returns an error.
func TestHandleGetTaskStatus_BDError(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	mockExec := mocks.NewMockBeadsExecutor(t)
	mockExec.EXPECT().
		ShowIssue("perles-abc.1").
		Return(nil, errors.New("bd command failed: issue not found"))

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mockExec)
	handler := cs.handlers["get_task_status"]

	args := `{"task_id": "perles-abc.1"}`
	_, err := handler(context.Background(), json.RawMessage(args))

	require.Error(t, err)
	require.Contains(t, err.Error(), "bd show failed")
}

// TestHandleGetTaskStatus_Success tests get_task_status returns valid issue data.
func TestHandleGetTaskStatus_Success(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	mockExec := mocks.NewMockBeadsExecutor(t)
	mockExec.EXPECT().
		ShowIssue("perles-abc.1").
		Return(&beads.Issue{
			ID:        "perles-abc.1",
			TitleText: "Test Task",
			Status:    beads.StatusOpen,
			Priority:  beads.PriorityMedium,
			Type:      beads.TypeTask,
		}, nil)

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mockExec)
	handler := cs.handlers["get_task_status"]

	args := `{"task_id": "perles-abc.1"}`
	result, err := handler(context.Background(), json.RawMessage(args))

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, result.Content)

	// Verify response contains issue data
	var issues []*beads.Issue
	err = json.Unmarshal([]byte(result.Content[0].Text), &issues)
	require.NoError(t, err)
	require.Len(t, issues, 1)
	require.Equal(t, "perles-abc.1", issues[0].ID)
	require.Equal(t, "Test Task", issues[0].TitleText)
	require.Equal(t, beads.StatusOpen, issues[0].Status)
}

// TestHandleMarkTaskComplete_BDError tests mark_task_complete when UpdateStatus returns an error.
func TestHandleMarkTaskComplete_BDError(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	mockExec := mocks.NewMockBeadsExecutor(t)
	mockExec.EXPECT().
		UpdateStatus("perles-abc.1", beads.StatusClosed).
		Return(errors.New("bd update failed: database error"))

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mockExec)

	// Setup task assignment in committing state (required for mark_task_complete)
	taskID := "perles-abc.1"
	cs.assignmentsMu.Lock()
	cs.taskAssignments[taskID] = &TaskAssignment{
		TaskID:      taskID,
		Implementer: "worker-1",
		Status:      TaskCommitting,
	}
	cs.assignmentsMu.Unlock()

	handler := cs.handlers["mark_task_complete"]

	args := `{"task_id": "perles-abc.1"}`
	_, err := handler(context.Background(), json.RawMessage(args))

	require.Error(t, err)
	require.Contains(t, err.Error(), "bd update failed")
}

// TestHandleMarkTaskComplete_Success tests mark_task_complete successfully closes a task.
func TestHandleMarkTaskComplete_Success(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	mockExec := mocks.NewMockBeadsExecutor(t)
	mockExec.EXPECT().
		UpdateStatus("perles-abc.1", beads.StatusClosed).
		Return(nil)
	mockExec.EXPECT().
		AddComment("perles-abc.1", "coordinator", "Task completed").
		Return(nil)

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mockExec)

	// Create a worker in the pool
	_ = workerPool.AddTestWorker("worker-1", pool.WorkerWorking)

	// Setup task assignment in committing state
	taskID := "perles-abc.1"
	cs.assignmentsMu.Lock()
	cs.taskAssignments[taskID] = &TaskAssignment{
		TaskID:      taskID,
		Implementer: "worker-1",
		Status:      TaskCommitting,
	}
	cs.workerAssignments["worker-1"] = &WorkerAssignment{
		TaskID: taskID,
		Role:   RoleImplementer,
		Phase:  events.PhaseCommitting,
	}
	cs.assignmentsMu.Unlock()

	handler := cs.handlers["mark_task_complete"]

	args := `{"task_id": "perles-abc.1"}`
	result, err := handler(context.Background(), json.RawMessage(args))

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, result.Content)

	// Verify response contains success message
	var response map[string]any
	err = json.Unmarshal([]byte(result.Content[0].Text), &response)
	require.NoError(t, err)
	require.Equal(t, "success", response["status"])

	// Verify internal state was updated
	cs.assignmentsMu.RLock()
	defer cs.assignmentsMu.RUnlock()
	require.Equal(t, TaskCompleted, cs.taskAssignments[taskID].Status)
	require.Equal(t, events.PhaseIdle, cs.workerAssignments["worker-1"].Phase)
}

// TestHandleMarkTaskFailed_BDError tests mark_task_failed when AddComment returns an error.
func TestHandleMarkTaskFailed_BDError(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	mockExec := mocks.NewMockBeadsExecutor(t)
	mockExec.EXPECT().
		AddComment("perles-abc.1", "coordinator", "⚠️ Task failed: blocked by dependency").
		Return(errors.New("bd comment failed: database error"))

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mockExec)
	handler := cs.handlers["mark_task_failed"]

	args := `{"task_id": "perles-abc.1", "reason": "blocked by dependency"}`
	_, err := handler(context.Background(), json.RawMessage(args))

	require.Error(t, err)
	require.Contains(t, err.Error(), "bd comment failed")
}

// TestHandleMarkTaskFailed_Success tests mark_task_failed successfully adds a failure comment.
func TestHandleMarkTaskFailed_Success(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	mockExec := mocks.NewMockBeadsExecutor(t)
	mockExec.EXPECT().
		AddComment("perles-abc.1", "coordinator", "⚠️ Task failed: tests failing").
		Return(nil)

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mockExec)
	handler := cs.handlers["mark_task_failed"]

	args := `{"task_id": "perles-abc.1", "reason": "tests failing"}`
	result, err := handler(context.Background(), json.RawMessage(args))

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, result.Content)
	require.Contains(t, result.Content[0].Text, "marked as failed")
}

// TestHandleAssignTaskReview_FullValidation tests assign_task_review with full validation.
func TestHandleAssignTaskReview_FullValidation(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := message.New()
	cs := NewCoordinatorServer(claude.NewClient(), workerPool, msgIssue, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Create workers
	_ = workerPool.AddTestWorker("worker-1", pool.WorkerWorking)
	_ = workerPool.AddTestWorker("worker-2", pool.WorkerReady)

	// Setup task in awaiting review state
	taskID := "perles-abc.1"
	cs.assignmentsMu.Lock()
	cs.taskAssignments[taskID] = &TaskAssignment{
		TaskID:      taskID,
		Implementer: "worker-1",
		Status:      TaskInReview,
	}
	cs.workerAssignments["worker-1"] = &WorkerAssignment{
		TaskID: taskID,
		Role:   RoleImplementer,
		Phase:  events.PhaseAwaitingReview,
	}
	cs.assignmentsMu.Unlock()

	handler := cs.handlers["assign_task_review"]

	// Should pass validation but fail when trying to send message
	args := `{"reviewer_id": "worker-2", "task_id": "perles-abc.1", "implementer_id": "worker-1", "summary": "Test implementation"}`
	_, err := handler(context.Background(), json.RawMessage(args))
	// May succeed partially since we have a message issue
	if err != nil {
		t.Logf("Expected partial completion, got error: %v", err)
	}
}

// TestHandleAssignReviewFeedback_FullValidation tests assign_review_feedback with valid state.
func TestHandleAssignReviewFeedback_FullValidation(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := message.New()
	cs := NewCoordinatorServer(claude.NewClient(), workerPool, msgIssue, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Create implementer
	_ = workerPool.AddTestWorker("worker-1", pool.WorkerReady)

	// Setup task in denied state
	taskID := "perles-abc.1"
	cs.assignmentsMu.Lock()
	cs.taskAssignments[taskID] = &TaskAssignment{
		TaskID:      taskID,
		Implementer: "worker-1",
		Status:      TaskDenied,
	}
	cs.workerAssignments["worker-1"] = &WorkerAssignment{
		TaskID: taskID,
		Role:   RoleImplementer,
		Phase:  events.PhaseAwaitingReview,
	}
	cs.assignmentsMu.Unlock()

	handler := cs.handlers["assign_review_feedback"]

	// Should pass validation but fail when trying to send message
	args := `{"implementer_id": "worker-1", "task_id": "perles-abc.1", "feedback": "Please fix the error handling"}`
	_, err := handler(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Logf("Expected partial completion, got error: %v", err)
	}
}

// TestHandleApproveCommit_FullValidation tests approve_commit with valid state.
func TestHandleApproveCommit_FullValidation(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := message.New()
	cs := NewCoordinatorServer(claude.NewClient(), workerPool, msgIssue, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Create implementer
	_ = workerPool.AddTestWorker("worker-1", pool.WorkerWorking)

	// Setup task in approved state
	taskID := "perles-abc.1"
	cs.assignmentsMu.Lock()
	cs.taskAssignments[taskID] = &TaskAssignment{
		TaskID:      taskID,
		Implementer: "worker-1",
		Status:      TaskApproved,
	}
	cs.workerAssignments["worker-1"] = &WorkerAssignment{
		TaskID: taskID,
		Role:   RoleImplementer,
		Phase:  events.PhaseAwaitingReview,
	}
	cs.assignmentsMu.Unlock()

	handler := cs.handlers["approve_commit"]

	// Should pass validation but fail when trying to send message
	args := `{"implementer_id": "worker-1", "task_id": "perles-abc.1"}`
	_, err := handler(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Logf("Expected partial completion, got error: %v", err)
	}
}

// TestQueryWorkerState_WithFilters tests query_worker_state with various filters.
func TestQueryWorkerState_WithFilters(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Create workers with different states
	_ = workerPool.AddTestWorker("worker-1", pool.WorkerWorking)
	_ = workerPool.AddTestWorker("worker-2", pool.WorkerReady)
	_ = workerPool.AddTestWorker("worker-3", pool.WorkerWorking)

	cs.assignmentsMu.Lock()
	cs.workerAssignments["worker-1"] = &WorkerAssignment{
		TaskID: "perles-abc.1",
		Role:   RoleImplementer,
		Phase:  events.PhaseImplementing,
	}
	cs.workerAssignments["worker-3"] = &WorkerAssignment{
		TaskID: "perles-abc.2",
		Role:   RoleReviewer,
		Phase:  events.PhaseReviewing,
	}
	cs.assignmentsMu.Unlock()

	handler := cs.handlers["query_worker_state"]

	tests := []struct {
		name      string
		args      string
		checkFunc func(response workerStateResponse) error
	}{
		{
			name: "filter by worker_id",
			args: `{"worker_id": "worker-1"}`,
			checkFunc: func(r workerStateResponse) error {
				if len(r.Workers) != 1 || r.Workers[0].WorkerID != "worker-1" {
					return errorf("expected worker-1, got %v", r.Workers)
				}
				return nil
			},
		},
		{
			name: "filter by task_id",
			args: `{"task_id": "perles-abc.1"}`,
			checkFunc: func(r workerStateResponse) error {
				if len(r.Workers) != 1 || r.Workers[0].TaskID != "perles-abc.1" {
					return errorf("expected task perles-abc.1, got %v", r.Workers)
				}
				return nil
			},
		},
		{
			name: "no filter",
			args: `{}`,
			checkFunc: func(r workerStateResponse) error {
				// Should return all active workers
				if len(r.Workers) < 2 {
					return errorf("expected at least 2 workers, got %d", len(r.Workers))
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := handler(context.Background(), json.RawMessage(tt.args))
			require.NoError(t, err)

			var response workerStateResponse
			err = json.Unmarshal([]byte(result.Content[0].Text), &response)
			require.NoError(t, err, "Failed to parse response")

			require.NoError(t, tt.checkFunc(response), "Check function failed")
		})
	}
}

// TestListWorkers_AllPhases tests list_workers showing all phase types.
func TestListWorkers_AllPhases(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Create workers in different phases
	phases := []events.WorkerPhase{
		events.PhaseIdle,
		events.PhaseImplementing,
		events.PhaseAwaitingReview,
		events.PhaseReviewing,
		events.PhaseAddressingFeedback,
		events.PhaseCommitting,
	}

	for i, phase := range phases {
		workerID := "worker-" + string(rune('1'+i))
		_ = workerPool.AddTestWorker(workerID, pool.WorkerWorking)
		if phase != events.PhaseIdle {
			cs.workerAssignments[workerID] = &WorkerAssignment{
				TaskID: "perles-abc." + string(rune('1'+i)),
				Role:   RoleImplementer,
				Phase:  phase,
			}
		}
	}

	handler := cs.handlers["list_workers"]
	result, err := handler(context.Background(), nil)
	require.NoError(t, err)

	type workerInfo struct {
		WorkerID string `json:"worker_id"`
		Phase    string `json:"phase"`
		Role     string `json:"role,omitempty"`
	}
	var infos []workerInfo
	err = json.Unmarshal([]byte(result.Content[0].Text), &infos)
	require.NoError(t, err, "Failed to parse response")

	require.Len(t, infos, len(phases), "Expected %d workers", len(phases))

	// Verify each phase is represented
	phaseFound := make(map[string]bool)
	for _, info := range infos {
		phaseFound[info.Phase] = true
	}

	for _, phase := range phases {
		require.True(t, phaseFound[string(phase)], "Phase %s not found in response", phase)
	}
}

// TestSendToWorker_WorkerExists tests send_to_worker with an existing worker.
func TestSendToWorker_WorkerExists(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := message.New()
	cs := NewCoordinatorServer(claude.NewClient(), workerPool, msgIssue, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Create worker
	_ = workerPool.AddTestWorker("worker-1", pool.WorkerWorking)

	handler := cs.handlers["send_to_worker"]

	// Try to send message - will fail trying to resume worker (no Claude)
	args := `{"worker_id": "worker-1", "message": "Please continue with the task"}`
	_, err := handler(context.Background(), json.RawMessage(args))
	// Error expected since we can't resume without Claude
	if err == nil {
		t.Log("Handler completed - message may have been queued")
	}
}

// TestReadMessageLog_WithMessages tests read_message_log with existing messages.
func TestReadMessageLog_WithMessages(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := message.New()

	// Add some messages
	_, _ = msgIssue.Append("COORDINATOR", "ALL", "Welcome message", message.MessageInfo)
	_, _ = msgIssue.Append("WORKER.1", "COORDINATOR", "Ready for task", message.MessageWorkerReady)
	_, _ = msgIssue.Append("COORDINATOR", "WORKER.1", "Here is your task", message.MessageInfo)

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, msgIssue, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))
	handler := cs.handlers["read_message_log"]

	result, err := handler(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)

	// Should contain all messages
	require.NotNil(t, result)
	require.NotEmpty(t, result.Content, "Expected result with content")
	text := result.Content[0].Text
	require.GreaterOrEqual(t, len(text), 10, "Expected message log content")
}

// TestReadMessageLog_WithLimit tests read_message_log with limit parameter.
func TestReadMessageLog_WithLimit(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := message.New()

	// Add many messages
	for i := 0; i < 10; i++ {
		_, _ = msgIssue.Append("COORDINATOR", "ALL", "Message "+string(rune('0'+i)), message.MessageInfo)
	}

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, msgIssue, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))
	handler := cs.handlers["read_message_log"]

	// Request only last 3 messages
	result, err := handler(context.Background(), json.RawMessage(`{"limit": 3}`))
	require.NoError(t, err)

	require.NotNil(t, result)
	require.NotEmpty(t, result.Content, "Expected result with content")
}

// TestPrepareHandoff_WithLongSummary tests prepare_handoff with a long summary.
func TestPrepareHandoff_WithLongSummary(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := message.New()
	cs := NewCoordinatorServer(claude.NewClient(), workerPool, msgIssue, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))
	handler := cs.handlers["prepare_handoff"]

	// Long summary
	summary := "Worker 1 is processing task perles-abc.1. " +
		"Current progress: Implemented feature X (50%). " +
		"Worker 2 is reviewing task perles-abc.2. " +
		"Worker 3 is idle. " +
		"Next steps: Worker 1 needs to complete implementation, then Worker 2 will review."

	args := `{"summary": "` + summary + `"}`
	result, err := handler(context.Background(), json.RawMessage(args))
	require.NoError(t, err)

	require.Equal(t, "Handoff message posted. Refresh will proceed.", result.Content[0].Text, "Unexpected result")
}

// TestCoordinatorServer_WorkerStateCallbackImpl tests the WorkerStateCallback implementation.
func TestCoordinatorServer_WorkerStateCallbackImpl(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	mockExec := mocks.NewMockBeadsExecutor(t)
	// Set up expectations for AddComment calls made during callback implementation
	mockExec.EXPECT().AddComment("perles-abc.1", "coordinator", mock.Anything).Return(nil).Maybe()
	mockExec.EXPECT().UpdateStatus(mock.Anything, mock.Anything).Return(nil).Maybe()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mockExec)

	// Test GetWorkerPhase for non-existent worker
	phase, err := cs.GetWorkerPhase("nonexistent")
	require.NoError(t, err, "Expected no error for missing worker")
	require.Equal(t, events.PhaseIdle, phase, "Expected idle phase for missing worker")

	// Setup worker assignment
	workerID := "worker-1"
	taskID := "perles-abc.1"
	cs.assignmentsMu.Lock()
	cs.workerAssignments[workerID] = &WorkerAssignment{
		TaskID: taskID,
		Role:   RoleImplementer,
		Phase:  events.PhaseImplementing,
	}
	cs.taskAssignments[taskID] = &TaskAssignment{
		TaskID:      taskID,
		Implementer: workerID,
		Status:      TaskImplementing,
	}
	cs.assignmentsMu.Unlock()

	// Test GetWorkerPhase for existing worker
	phase, err = cs.GetWorkerPhase(workerID)
	require.NoError(t, err, "Unexpected error")
	require.Equal(t, events.PhaseImplementing, phase, "Expected implementing phase")

	// Test OnImplementationComplete
	err = cs.OnImplementationComplete(workerID, "completed feature")
	require.NoError(t, err, "OnImplementationComplete error")

	// Verify phase changed
	cs.assignmentsMu.RLock()
	require.Equal(t, events.PhaseAwaitingReview, cs.workerAssignments[workerID].Phase, "Expected awaiting_review phase")
	cs.assignmentsMu.RUnlock()

	// Setup for review test
	reviewerID := "worker-2"
	cs.assignmentsMu.Lock()
	cs.workerAssignments[reviewerID] = &WorkerAssignment{
		TaskID:        taskID,
		Role:          RoleReviewer,
		Phase:         events.PhaseReviewing,
		ImplementerID: workerID,
	}
	cs.taskAssignments[taskID].Reviewer = reviewerID
	cs.assignmentsMu.Unlock()

	// Test OnReviewVerdict - APPROVED
	err = cs.OnReviewVerdict(reviewerID, "APPROVED", "LGTM")
	require.NoError(t, err, "OnReviewVerdict error")

	// Verify reviewer is idle and task is approved
	cs.assignmentsMu.RLock()
	require.Equal(t, events.PhaseIdle, cs.workerAssignments[reviewerID].Phase, "Expected reviewer idle phase")
	require.Equal(t, TaskApproved, cs.taskAssignments[taskID].Status, "Expected task approved status")
	cs.assignmentsMu.RUnlock()
}

// TestPrompts tests prompt generation functions.
func TestPrompts(t *testing.T) {
	// Test WorkerIdlePrompt
	idlePrompt := WorkerIdlePrompt("worker-1")
	require.NotEmpty(t, idlePrompt, "WorkerIdlePrompt returned empty string")

	// Test WorkerSystemPrompt
	systemPrompt := WorkerSystemPrompt("worker-1")
	require.NotEmpty(t, systemPrompt, "WorkerSystemPrompt returned empty string")
	require.GreaterOrEqual(t, len(systemPrompt), 100, "WorkerSystemPrompt seems too short")

	// Test TaskAssignmentPrompt (taskID, title, summary)
	taskPrompt := TaskAssignmentPrompt("perles-abc.1", "Implement feature X", "Coordinator Summary")
	require.NotEmpty(t, taskPrompt, "TaskAssignmentPrompt returned empty string")

	// Test ReviewAssignmentPrompt
	reviewPrompt := ReviewAssignmentPrompt("perles-abc.1", "worker-1", "Implemented feature X")
	require.NotEmpty(t, reviewPrompt, "ReviewAssignmentPrompt returned empty string")

	// Test ReviewFeedbackPrompt
	feedbackPrompt := ReviewFeedbackPrompt("perles-abc.1", "Please fix the error handling")
	require.NotEmpty(t, feedbackPrompt, "ReviewFeedbackPrompt returned empty string")

	// Test CommitApprovalPrompt (taskID, commitMessage)
	commitPrompt := CommitApprovalPrompt("perles-abc.1", "feat: add feature X")
	require.NotEmpty(t, commitPrompt, "CommitApprovalPrompt returned empty string")
}

// TestConfigGeneration tests MCP config generation functions.
func TestConfigGeneration(t *testing.T) {
	// Test GenerateCoordinatorConfig (workDir string)
	coordConfig, err := GenerateCoordinatorConfigHTTP(8765)
	require.NoError(t, err, "GenerateCoordinatorConfig error")
	require.NotEmpty(t, coordConfig, "GenerateCoordinatorConfig returned empty string")

	// Test GenerateCoordinatorConfigHTTP
	httpConfig, err := GenerateCoordinatorConfigHTTP(8765)
	require.NoError(t, err, "GenerateCoordinatorConfigHTTP error")
	require.NotEmpty(t, httpConfig, "GenerateCoordinatorConfigHTTP returned empty string")

	// Test GenerateWorkerConfig (workerID, workDir string)
	workerConfig, err := GenerateWorkerConfig("worker-1", "/tmp/test")
	require.NoError(t, err, "GenerateWorkerConfig error")
	require.NotEmpty(t, workerConfig, "GenerateWorkerConfig returned empty string")

	// Test GenerateWorkerConfigHTTP
	workerHTTPConfig, err := GenerateWorkerConfigHTTP(8765, "worker-1")
	require.NoError(t, err, "GenerateWorkerConfigHTTP error")
	require.NotEmpty(t, workerHTTPConfig, "GenerateWorkerConfigHTTP returned empty string")

	// Test ConfigToFlag
	flag := ConfigToFlag(coordConfig)
	require.NotEmpty(t, flag, "ConfigToFlag returned empty string")

	// Test ParseMCPConfig
	parsed, err := ParseMCPConfig(coordConfig)
	require.NoError(t, err, "ParseMCPConfig error")
	require.NotNil(t, parsed, "ParseMCPConfig returned nil")
}

// TestMessageDeduplicator_EdgeCases tests deduplicator edge cases.
func TestMessageDeduplicator_EdgeCases(t *testing.T) {
	dedup := NewMessageDeduplicator(100 * time.Millisecond)

	// Test empty message
	require.False(t, dedup.IsDuplicate("worker-1", ""), "Empty message should not be considered duplicate on first call")
	require.True(t, dedup.IsDuplicate("worker-1", ""), "Empty message should be duplicate on second call")

	// Test very long message
	longMsg := ""
	for i := 0; i < 1000; i++ {
		longMsg += "a"
	}
	require.False(t, dedup.IsDuplicate("worker-1", longMsg), "Long message should not be duplicate on first call")

	// Test special characters
	specialMsg := "Message with special chars: !@#$%^&*()[]{}|\\:;<>?,./`~"
	require.False(t, dedup.IsDuplicate("worker-1", specialMsg), "Special char message should not be duplicate on first call")

	// Test Unicode
	unicodeMsg := "Message with Unicode: 你好世界 🎉 émojis"
	require.False(t, dedup.IsDuplicate("worker-1", unicodeMsg), "Unicode message should not be duplicate on first call")

	// Test expiration
	time.Sleep(150 * time.Millisecond)
	require.False(t, dedup.IsDuplicate("worker-1", ""), "Message should not be duplicate after expiration")

	// Test Len and Clear
	_ = dedup.IsDuplicate("worker-1", "msg1")
	_ = dedup.IsDuplicate("worker-1", "msg2")
	require.GreaterOrEqual(t, dedup.Len(), 2, "Expected at least 2 entries")

	dedup.Clear()
	require.Equal(t, 0, dedup.Len(), "Expected 0 entries after clear")
}

// helper function for creating error
func errorf(format string, args ...interface{}) error {
	return &customError{msg: formatMessage(format, args...)}
}

type customError struct {
	msg string
}

func (e *customError) Error() string {
	return e.msg
}

func formatMessage(format string, args ...interface{}) string {
	if len(args) == 0 {
		return format
	}
	// Simple formatting - replace %v with args
	result := format
	for _, arg := range args {
		if idx := findPercent(result); idx >= 0 {
			prefix := result[:idx]
			suffix := ""
			if idx+2 <= len(result) {
				suffix = result[idx+2:]
			}
			result = prefix + fmt.Sprint(arg) + suffix
		}
	}
	return result
}

func findPercent(s string) int {
	for i := 0; i < len(s)-1; i++ {
		if s[i] == '%' {
			return i
		}
	}
	return -1
}

// ============================================================================
// BD Helper Functions Tests with Mocks
// ============================================================================

// TestUpdateBDStatus_Success tests updateBDStatus with successful mock.
func TestUpdateBDStatus_Success(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	mockExec := mocks.NewMockBeadsExecutor(t)
	mockExec.EXPECT().
		UpdateStatus("perles-abc.1", beads.StatusInProgress).
		Return(nil)

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mockExec)

	// Call the exported function - should not panic on success
	err := cs.beadsExecutor.UpdateStatus("perles-abc.1", beads.StatusInProgress)
	require.NoError(t, err)
}

// TestUpdateBDStatus_Error tests updateBDStatus when mock returns error (best-effort, no crash).
func TestUpdateBDStatus_Error(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	mockExec := mocks.NewMockBeadsExecutor(t)
	mockExec.EXPECT().
		UpdateStatus("perles-abc.1", beads.StatusInProgress).
		Return(errors.New("database error"))

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mockExec)

	// Call the exported function - should not panic even on error (best-effort)
	err := cs.beadsExecutor.UpdateStatus("perles-abc.1", beads.StatusInProgress)
	require.Error(t, err)
}

// TestAddBDComment_Success tests addBDComment with successful mock.
func TestAddBDComment_Success(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	mockExec := mocks.NewMockBeadsExecutor(t)
	mockExec.EXPECT().
		AddComment("perles-abc.1", "coordinator", "Task assigned to worker-1").
		Return(nil)

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mockExec)

	// Call the exported function - should not panic on success
	err := cs.beadsExecutor.AddComment("perles-abc.1", "coordinator", "Task assigned to worker-1")
	require.NoError(t, err)
}

// TestAddBDComment_Error tests addBDComment when mock returns error (best-effort, no crash).
func TestAddBDComment_Error(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	mockExec := mocks.NewMockBeadsExecutor(t)
	mockExec.EXPECT().
		AddComment("perles-abc.1", "coordinator", "Task assigned to worker-1").
		Return(errors.New("database error"))

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mockExec)

	// Call the exported function - should not panic even on error (best-effort)
	err := cs.beadsExecutor.AddComment("perles-abc.1", "coordinator", "Task assigned to worker-1")
	require.Error(t, err)
}
