package mcp

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/orchestration/claude"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/orchestration/pool"
)

// ============================================================================
// Integration Tests with Mock BD
// ============================================================================

// TestIntegration_FullTaskLifecycle tests the complete lifecycle of a task from assignment to completion.
func TestIntegration_FullTaskLifecycle(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := message.New()
	cs := NewCoordinatorServer(claude.NewClient(), workerPool, msgIssue, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	implementerID := "worker-1"
	reviewerID := "worker-2"
	taskID := "perles-abc.1"

	// Create workers
	_ = workerPool.AddTestWorker(implementerID, pool.WorkerReady)
	_ = workerPool.AddTestWorker(reviewerID, pool.WorkerReady)

	// Step 1: Assign task to implementer
	cs.assignmentsMu.Lock()
	cs.workerAssignments[implementerID] = &WorkerAssignment{
		TaskID:     taskID,
		Role:       RoleImplementer,
		Phase:      events.PhaseImplementing,
		AssignedAt: time.Now(),
	}
	cs.taskAssignments[taskID] = &TaskAssignment{
		TaskID:      taskID,
		Implementer: implementerID,
		Status:      TaskImplementing,
		StartedAt:   time.Now(),
	}
	cs.assignmentsMu.Unlock()

	// Verify state
	cs.assignmentsMu.RLock()
	require.Equal(t, events.PhaseImplementing, cs.workerAssignments[implementerID].Phase, "Phase mismatch")
	cs.assignmentsMu.RUnlock()

	// Step 2: Implementer completes implementation
	workerStore := newMockMessageStore()
	ws := NewWorkerServer(implementerID, workerStore)
	callback := &coordinatorStateCallback{cs: cs}
	ws.SetStateCallback(callback)

	handler := ws.handlers["report_implementation_complete"]
	_, err := handler(context.Background(), json.RawMessage(`{"summary": "Implemented feature X"}`))
	require.NoError(t, err, "report_implementation_complete failed")

	// Verify transition to awaiting review
	cs.assignmentsMu.RLock()
	require.Equal(t, events.PhaseAwaitingReview, cs.workerAssignments[implementerID].Phase, "Phase mismatch")
	cs.assignmentsMu.RUnlock()

	// Step 3: Assign reviewer
	cs.assignmentsMu.Lock()
	cs.workerAssignments[reviewerID] = &WorkerAssignment{
		TaskID:        taskID,
		Role:          RoleReviewer,
		Phase:         events.PhaseReviewing,
		AssignedAt:    time.Now(),
		ImplementerID: implementerID,
	}
	cs.taskAssignments[taskID].Reviewer = reviewerID
	cs.taskAssignments[taskID].Status = TaskInReview
	cs.taskAssignments[taskID].ReviewStartedAt = time.Now()
	cs.assignmentsMu.Unlock()

	// Step 4: Reviewer approves
	reviewerStore := newMockMessageStore()
	reviewerWs := NewWorkerServer(reviewerID, reviewerStore)
	reviewerWs.SetStateCallback(callback)

	reviewHandler := reviewerWs.handlers["report_review_verdict"]
	_, err = reviewHandler(context.Background(), json.RawMessage(`{"verdict": "APPROVED", "comments": "LGTM"}`))
	require.NoError(t, err, "report_review_verdict failed")

	// Verify reviewer back to idle
	cs.assignmentsMu.RLock()
	require.Equal(t, events.PhaseIdle, cs.workerAssignments[reviewerID].Phase, "Reviewer phase mismatch")
	cs.assignmentsMu.RUnlock()

	// Step 5: Approve commit (simulated coordinator action)
	cs.assignmentsMu.Lock()
	cs.taskAssignments[taskID].Status = TaskApproved
	cs.assignmentsMu.Unlock()

	// Step 6: Implementer commits
	cs.assignmentsMu.Lock()
	cs.workerAssignments[implementerID].Phase = events.PhaseCommitting
	cs.taskAssignments[taskID].Status = TaskCommitting
	cs.assignmentsMu.Unlock()

	// Step 7: Commit complete -> task done
	cs.assignmentsMu.Lock()
	cs.workerAssignments[implementerID].Phase = events.PhaseIdle
	cs.workerAssignments[implementerID].TaskID = ""
	cs.taskAssignments[taskID].Status = TaskCompleted
	cs.assignmentsMu.Unlock()

	// Final verification
	cs.assignmentsMu.RLock()
	require.Equal(t, TaskCompleted, cs.taskAssignments[taskID].Status, "Task status mismatch")
	require.Equal(t, events.PhaseIdle, cs.workerAssignments[implementerID].Phase, "Implementer phase mismatch")
	cs.assignmentsMu.RUnlock()
}

// TestIntegration_DenialCycle tests the denial -> feedback -> re-review cycle.
func TestIntegration_DenialCycle(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := message.New()
	cs := NewCoordinatorServer(claude.NewClient(), workerPool, msgIssue, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	implementerID := "worker-1"
	reviewerID := "worker-2"
	taskID := "perles-abc.1"

	// Create workers
	_ = workerPool.AddTestWorker(implementerID, pool.WorkerReady)
	_ = workerPool.AddTestWorker(reviewerID, pool.WorkerReady)

	// Setup: implementer has completed, reviewer is reviewing
	cs.assignmentsMu.Lock()
	cs.workerAssignments[implementerID] = &WorkerAssignment{
		TaskID:     taskID,
		Role:       RoleImplementer,
		Phase:      events.PhaseAwaitingReview,
		AssignedAt: time.Now(),
	}
	cs.workerAssignments[reviewerID] = &WorkerAssignment{
		TaskID:        taskID,
		Role:          RoleReviewer,
		Phase:         events.PhaseReviewing,
		AssignedAt:    time.Now(),
		ImplementerID: implementerID,
	}
	cs.taskAssignments[taskID] = &TaskAssignment{
		TaskID:          taskID,
		Implementer:     implementerID,
		Reviewer:        reviewerID,
		Status:          TaskInReview,
		StartedAt:       time.Now(),
		ReviewStartedAt: time.Now(),
	}
	cs.assignmentsMu.Unlock()

	// Reviewer denies
	reviewerStore := newMockMessageStore()
	reviewerWs := NewWorkerServer(reviewerID, reviewerStore)
	callback := &denialCycleCallback{cs: cs}
	reviewerWs.SetStateCallback(callback)

	reviewHandler := reviewerWs.handlers["report_review_verdict"]
	_, err := reviewHandler(context.Background(), json.RawMessage(`{"verdict": "DENIED", "comments": "Missing error handling"}`))
	require.NoError(t, err, "report_review_verdict failed")

	// Verify task status is denied and implementer gets feedback assignment
	cs.assignmentsMu.RLock()
	require.Equal(t, TaskDenied, cs.taskAssignments[taskID].Status, "Task status mismatch")
	cs.assignmentsMu.RUnlock()

	// Coordinator assigns feedback to implementer
	cs.assignmentsMu.Lock()
	cs.workerAssignments[implementerID].Phase = events.PhaseAddressingFeedback
	cs.assignmentsMu.Unlock()

	// Implementer addresses feedback and re-submits
	implementerStore := newMockMessageStore()
	implementerWs := NewWorkerServer(implementerID, implementerStore)
	feedbackCallback := &coordinatorStateCallback{cs: cs}
	implementerWs.SetStateCallback(feedbackCallback)

	implHandler := implementerWs.handlers["report_implementation_complete"]
	_, err = implHandler(context.Background(), json.RawMessage(`{"summary": "Fixed error handling"}`))
	require.NoError(t, err, "report_implementation_complete after feedback failed")

	// Verify back to awaiting review
	cs.assignmentsMu.RLock()
	require.Equal(t, events.PhaseAwaitingReview, cs.workerAssignments[implementerID].Phase, "Phase mismatch")
	cs.assignmentsMu.RUnlock()
}

// denialCycleCallback implements WorkerStateCallback for denial testing.
type denialCycleCallback struct {
	cs *CoordinatorServer
}

func (c *denialCycleCallback) GetWorkerPhase(workerID string) (events.WorkerPhase, error) {
	c.cs.assignmentsMu.RLock()
	defer c.cs.assignmentsMu.RUnlock()

	wa, ok := c.cs.workerAssignments[workerID]
	if !ok {
		return events.PhaseIdle, nil
	}
	return wa.Phase, nil
}

func (c *denialCycleCallback) OnImplementationComplete(workerID, summary string) error {
	c.cs.assignmentsMu.Lock()
	defer c.cs.assignmentsMu.Unlock()

	wa := c.cs.workerAssignments[workerID]
	wa.Phase = events.PhaseAwaitingReview

	if ta, ok := c.cs.taskAssignments[wa.TaskID]; ok {
		ta.Status = TaskInReview
	}

	return nil
}

func (c *denialCycleCallback) OnReviewVerdict(workerID, verdict, comments string) error {
	c.cs.assignmentsMu.Lock()
	defer c.cs.assignmentsMu.Unlock()

	wa := c.cs.workerAssignments[workerID]
	wa.Phase = events.PhaseIdle

	if verdict == "DENIED" {
		if ta, ok := c.cs.taskAssignments[wa.TaskID]; ok {
			ta.Status = TaskDenied
		}
	}

	return nil
}

// TestIntegration_MultipleWorkersMultipleTasks tests concurrent task management.
func TestIntegration_MultipleWorkersMultipleTasks(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := message.New()
	cs := NewCoordinatorServer(claude.NewClient(), workerPool, msgIssue, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Create 4 workers
	for i := 1; i <= 4; i++ {
		workerID := "worker-" + string(rune('0'+i))
		_ = workerPool.AddTestWorker(workerID, pool.WorkerReady)
	}

	// Assign 2 tasks to different workers
	tasks := []string{"perles-abc.1", "perles-abc.2"}

	cs.assignmentsMu.Lock()
	cs.workerAssignments["worker-1"] = &WorkerAssignment{
		TaskID:     tasks[0],
		Role:       RoleImplementer,
		Phase:      events.PhaseImplementing,
		AssignedAt: time.Now(),
	}
	cs.workerAssignments["worker-2"] = &WorkerAssignment{
		TaskID:     tasks[1],
		Role:       RoleImplementer,
		Phase:      events.PhaseImplementing,
		AssignedAt: time.Now(),
	}
	cs.taskAssignments[tasks[0]] = &TaskAssignment{
		TaskID:      tasks[0],
		Implementer: "worker-1",
		Status:      TaskImplementing,
		StartedAt:   time.Now(),
	}
	cs.taskAssignments[tasks[1]] = &TaskAssignment{
		TaskID:      tasks[1],
		Implementer: "worker-2",
		Status:      TaskImplementing,
		StartedAt:   time.Now(),
	}
	cs.assignmentsMu.Unlock()

	// Query state - should show both workers with tasks
	handler := cs.handlers["query_worker_state"]
	result, err := handler(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err, "query_worker_state failed")

	var response workerStateResponse
	err = json.Unmarshal([]byte(result.Content[0].Text), &response)
	require.NoError(t, err, "Failed to parse response")

	// Count workers in implementing phase
	implementingCount := 0
	for _, w := range response.Workers {
		if w.Phase == "implementing" {
			implementingCount++
		}
	}
	require.Equal(t, 2, implementingCount, "Expected 2 workers implementing")

	// Check task assignments
	require.Len(t, response.TaskAssignments, 2, "Expected 2 task assignments")
}

// TestIntegration_OrphanRecovery tests detection and recovery of orphaned tasks.
func TestIntegration_OrphanRecovery(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	taskID := "perles-abc.1"

	// Create worker and assign task
	_ = workerPool.AddTestWorker("worker-1", pool.WorkerWorking)
	cs.assignmentsMu.Lock()
	cs.taskAssignments[taskID] = &TaskAssignment{
		TaskID:      taskID,
		Implementer: "worker-1",
		Status:      TaskImplementing,
	}
	cs.assignmentsMu.Unlock()

	// No orphans yet
	orphans := cs.detectOrphanedTasks()
	require.Empty(t, orphans, "Expected no orphans")

	// Retire the worker (simulating crash/timeout)
	workerPool.GetWorker("worker-1").Retire()

	// Now task should be orphaned
	orphans = cs.detectOrphanedTasks()
	require.Len(t, orphans, 1, "Expected 1 orphan")
	require.Equal(t, taskID, orphans[0], "Expected orphan to be the task")
}

// TestIntegration_ConcurrentToolCalls tests handling of concurrent tool calls.
func TestIntegration_ConcurrentToolCalls(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := message.New()
	cs := NewCoordinatorServer(claude.NewClient(), workerPool, msgIssue, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Create workers
	for i := 1; i <= 5; i++ {
		workerID := "worker-" + string(rune('0'+i))
		_ = workerPool.AddTestWorker(workerID, pool.WorkerReady)
	}

	var wg sync.WaitGroup
	ctx := context.Background()

	// Concurrent list_workers calls
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			handler := cs.handlers["list_workers"]
			_, _ = handler(ctx, nil)
		}()
	}

	// Concurrent query_worker_state calls
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			handler := cs.handlers["query_worker_state"]
			_, _ = handler(ctx, json.RawMessage(`{}`))
		}()
	}

	// Concurrent state modifications
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(num int) {
			defer wg.Done()
			taskID := "perles-abc." + string(rune('1'+num))
			workerID := "worker-" + string(rune('1'+num))

			cs.assignmentsMu.Lock()
			cs.workerAssignments[workerID] = &WorkerAssignment{
				TaskID:     taskID,
				Role:       RoleImplementer,
				Phase:      events.PhaseImplementing,
				AssignedAt: time.Now(),
			}
			cs.taskAssignments[taskID] = &TaskAssignment{
				TaskID:      taskID,
				Implementer: workerID,
				Status:      TaskImplementing,
				StartedAt:   time.Now(),
			}
			cs.assignmentsMu.Unlock()
		}(i)
	}

	wg.Wait()

	// Verify state is consistent
	cs.assignmentsMu.RLock()
	for workerID, wa := range cs.workerAssignments {
		if wa.TaskID != "" {
			if ta, ok := cs.taskAssignments[wa.TaskID]; ok {
				require.Equal(t, workerID, ta.Implementer,
					"Inconsistent state: worker %s has task %s but task implementer is %s",
					workerID, wa.TaskID, ta.Implementer)
			}
		}
	}
	cs.assignmentsMu.RUnlock()
}

// TestIntegration_MessageFlow tests message flow between workers and coordinator.
func TestIntegration_MessageFlow(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := message.New()
	_ = NewCoordinatorServer(claude.NewClient(), workerPool, msgIssue, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	workerID := "worker-1"
	_ = workerPool.AddTestWorker(workerID, pool.WorkerReady)

	// Worker signals ready
	workerStore := newMockMessageStore()
	ws := NewWorkerServer(workerID, workerStore)

	readyHandler := ws.handlers["signal_ready"]
	_, err := readyHandler(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err, "signal_ready failed")

	// Verify ready message was posted
	require.Len(t, workerStore.appendCalls, 1)
	require.Equal(t, message.MessageWorkerReady, workerStore.appendCalls[0].Type, "Message type mismatch")

	// Worker posts status update
	postHandler := ws.handlers["post_message"]
	_, err = postHandler(context.Background(), json.RawMessage(`{"to": "COORDINATOR", "content": "Task 50% complete"}`))
	require.NoError(t, err, "post_message failed")

	require.Len(t, workerStore.appendCalls, 2)
}

// TestIntegration_ValidateAssignmentConstraints tests that all assignment constraints are enforced.
func TestIntegration_ValidateAssignmentConstraints(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Create workers
	_ = workerPool.AddTestWorker("worker-1", pool.WorkerReady)
	_ = workerPool.AddTestWorker("worker-2", pool.WorkerWorking) // Not ready
	_ = workerPool.AddTestWorker("worker-3", pool.WorkerRetired) // Retired

	taskID := "perles-abc.1"

	// Test 1: Assigning to ready worker should pass validation
	err := cs.validateTaskAssignment("worker-1", taskID)
	require.NoError(t, err, "Expected no error for ready worker")

	// Test 2: Assigning to working worker should fail
	err = cs.validateTaskAssignment("worker-2", taskID)
	require.Error(t, err, "Expected error for working worker")

	// Test 3: Assigning to non-existent worker should fail
	err = cs.validateTaskAssignment("nonexistent", taskID)
	require.Error(t, err, "Expected error for non-existent worker")

	// Test 4: Assigning when worker already has task should fail
	cs.workerAssignments["worker-1"] = &WorkerAssignment{
		TaskID: "perles-xyz.1",
		Role:   RoleImplementer,
		Phase:  events.PhaseImplementing,
	}
	err = cs.validateTaskAssignment("worker-1", taskID)
	require.Error(t, err, "Expected error when worker already has task")
}

// TestIntegration_StuckWorkerDetection tests stuck worker detection integration.
func TestIntegration_StuckWorkerDetection(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Create workers with different assignment times
	cs.workerAssignments["worker-1"] = &WorkerAssignment{
		TaskID:     "perles-abc.1",
		AssignedAt: time.Now().Add(-MaxTaskDuration - 5*time.Minute), // Stuck
	}
	cs.workerAssignments["worker-2"] = &WorkerAssignment{
		TaskID:     "perles-abc.2",
		AssignedAt: time.Now().Add(-10 * time.Minute), // Not stuck
	}
	cs.workerAssignments["worker-3"] = &WorkerAssignment{
		TaskID:     "", // Idle
		AssignedAt: time.Now().Add(-MaxTaskDuration - time.Hour),
	}

	stuck := cs.checkStuckWorkers()
	require.Len(t, stuck, 1, "Expected 1 stuck worker")
	require.Equal(t, "worker-1", stuck[0], "Expected stuck worker worker-1")
}
