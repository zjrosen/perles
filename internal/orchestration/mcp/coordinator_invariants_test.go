package mcp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/orchestration/claude"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/pool"
)

// ============================================================================
// Property-Based Tests for State Machine Invariants
//
// These tests verify that critical invariants hold after sequences of operations.
// Instead of testing specific scenarios, they test that properties ALWAYS hold.
// ============================================================================

// TestInvariant_NoTaskHasMultipleImplementers verifies that after any sequence of
// assign_task calls, no task ever has more than one implementer.
// Invariant: ∀ task: count(implementers) <= 1
func TestInvariant_NoTaskHasMultipleImplementers(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Create multiple ready workers
	_ = workerPool.AddTestWorker("worker-1", pool.WorkerReady)
	_ = workerPool.AddTestWorker("worker-2", pool.WorkerReady)
	_ = workerPool.AddTestWorker("worker-3", pool.WorkerReady)
	_ = workerPool.AddTestWorker("worker-4", pool.WorkerReady)

	// Define test scenarios: sequences of (workerID, taskID) assignment attempts
	scenarios := []struct {
		name       string
		operations []struct {
			workerID string
			taskID   string
		}
	}{
		{
			name: "single_task_multiple_workers",
			operations: []struct {
				workerID string
				taskID   string
			}{
				{"worker-1", "perles-abc.1"},
				{"worker-2", "perles-abc.1"}, // Should be rejected
				{"worker-3", "perles-abc.1"}, // Should be rejected
			},
		},
		{
			name: "multiple_tasks_same_worker",
			operations: []struct {
				workerID string
				taskID   string
			}{
				{"worker-1", "perles-abc.1"},
				{"worker-1", "perles-abc.2"}, // Should be rejected (worker already assigned)
				{"worker-1", "perles-abc.3"}, // Should be rejected
			},
		},
		{
			name: "interleaved_assignments",
			operations: []struct {
				workerID string
				taskID   string
			}{
				{"worker-1", "perles-abc.1"},
				{"worker-2", "perles-xyz.1"},
				{"worker-3", "perles-abc.1"}, // Should be rejected (task already assigned)
				{"worker-4", "perles-xyz.2"},
			},
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// Reset state
			cs.assignmentsMu.Lock()
			cs.workerAssignments = make(map[string]*WorkerAssignment)
			cs.taskAssignments = make(map[string]*TaskAssignment)
			cs.assignmentsMu.Unlock()

			// Reset workers to Ready state
			for _, w := range workerPool.ActiveWorkers() {
				w.CompleteTask()
			}

			// Execute operations
			for _, op := range scenario.operations {
				// Try to assign (validation may reject)
				err := cs.validateTaskAssignment(op.workerID, op.taskID)
				if err == nil {
					// Assignment is valid, simulate it
					now := time.Now()
					cs.assignmentsMu.Lock()
					cs.workerAssignments[op.workerID] = &WorkerAssignment{
						TaskID:     op.taskID,
						Role:       RoleImplementer,
						Phase:      events.PhaseImplementing,
						AssignedAt: now,
					}
					cs.taskAssignments[op.taskID] = &TaskAssignment{
						TaskID:      op.taskID,
						Implementer: op.workerID,
						Status:      TaskImplementing,
						StartedAt:   now,
					}
					cs.assignmentsMu.Unlock()

					// Also update pool worker state
					worker := workerPool.GetWorker(op.workerID)
					if worker != nil {
						_ = worker.AssignTask(op.taskID)
					}
				}

				// INVARIANT CHECK: After every operation, verify no task has multiple implementers
				verifyNoTaskHasMultipleImplementers(t, cs)
			}
		})
	}
}

// verifyNoTaskHasMultipleImplementers checks the invariant after each operation.
func verifyNoTaskHasMultipleImplementers(t *testing.T, cs *CoordinatorServer) {
	t.Helper()

	cs.assignmentsMu.RLock()
	defer cs.assignmentsMu.RUnlock()

	// Build a map of taskID -> implementers
	taskImplementers := make(map[string][]string)

	for workerID, wa := range cs.workerAssignments {
		if wa != nil && wa.Role == RoleImplementer && wa.TaskID != "" {
			taskImplementers[wa.TaskID] = append(taskImplementers[wa.TaskID], workerID)
		}
	}

	// Check each task
	for taskID, implementers := range taskImplementers {
		require.LessOrEqual(t, len(implementers), 1, "INVARIANT VIOLATION: Task %s has multiple implementers: %v", taskID, implementers)
	}
}

// TestInvariant_NoTaskHasMultipleReviewers verifies that after any sequence of
// assign_task_review calls, no task ever has more than one reviewer.
// Invariant: ∀ task: count(reviewers) <= 1
func TestInvariant_NoTaskHasMultipleReviewers(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Create ready workers
	_ = workerPool.AddTestWorker("worker-1", pool.WorkerReady)
	_ = workerPool.AddTestWorker("worker-2", pool.WorkerReady)
	_ = workerPool.AddTestWorker("worker-3", pool.WorkerReady)
	_ = workerPool.AddTestWorker("worker-4", pool.WorkerReady)

	// Set up a task that is awaiting review
	cs.SetTaskAssignment("perles-abc.1", &TaskAssignment{
		TaskID:      "perles-abc.1",
		Implementer: "worker-1",
		Status:      TaskImplementing,
	})
	cs.SetWorkerAssignment("worker-1", &WorkerAssignment{
		TaskID: "perles-abc.1",
		Role:   RoleImplementer,
		Phase:  events.PhaseAwaitingReview, // Ready for review
	})

	// Try to assign multiple reviewers to the same task
	reviewers := []string{"worker-2", "worker-3", "worker-4"}

	for _, reviewerID := range reviewers {
		err := cs.validateReviewAssignment(reviewerID, "perles-abc.1", "worker-1")
		if err == nil {
			// First reviewer should succeed
			cs.assignmentsMu.Lock()
			cs.workerAssignments[reviewerID] = &WorkerAssignment{
				TaskID:        "perles-abc.1",
				Role:          RoleReviewer,
				Phase:         events.PhaseReviewing,
				ImplementerID: "worker-1",
			}
			if ta := cs.taskAssignments["perles-abc.1"]; ta != nil {
				ta.Reviewer = reviewerID
				ta.Status = TaskInReview
			}
			cs.assignmentsMu.Unlock()
		}

		// INVARIANT CHECK: After every attempt, verify no task has multiple reviewers
		verifyNoTaskHasMultipleReviewers(t, cs)
	}
}

// verifyNoTaskHasMultipleReviewers checks the invariant.
func verifyNoTaskHasMultipleReviewers(t *testing.T, cs *CoordinatorServer) {
	t.Helper()

	cs.assignmentsMu.RLock()
	defer cs.assignmentsMu.RUnlock()

	// Build a map of taskID -> reviewers
	taskReviewers := make(map[string][]string)

	for workerID, wa := range cs.workerAssignments {
		if wa != nil && wa.Role == RoleReviewer && wa.TaskID != "" {
			taskReviewers[wa.TaskID] = append(taskReviewers[wa.TaskID], workerID)
		}
	}

	// Check each task
	for taskID, reviewers := range taskReviewers {
		require.LessOrEqual(t, len(reviewers), 1, "INVARIANT VIOLATION: Task %s has multiple reviewers: %v", taskID, reviewers)
	}
}

// TestInvariant_WorkerHasSingleActiveTask verifies that after any sequence of
// assignments, each worker has at most one active task.
// Invariant: ∀ worker: |assigned_tasks| <= 1
func TestInvariant_WorkerHasSingleActiveTask(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Create workers
	_ = workerPool.AddTestWorker("worker-1", pool.WorkerReady)
	_ = workerPool.AddTestWorker("worker-2", pool.WorkerReady)

	// Define operations: try assigning multiple tasks to same worker
	operations := []struct {
		workerID string
		taskID   string
	}{
		{"worker-1", "perles-abc.1"},
		{"worker-1", "perles-abc.2"}, // Should fail - worker already has task
		{"worker-2", "perles-abc.3"},
		{"worker-2", "perles-abc.4"}, // Should fail - worker already has task
	}

	for _, op := range operations {
		err := cs.validateTaskAssignment(op.workerID, op.taskID)
		if err == nil {
			// Assignment is valid
			now := time.Now()
			cs.assignmentsMu.Lock()
			cs.workerAssignments[op.workerID] = &WorkerAssignment{
				TaskID:     op.taskID,
				Role:       RoleImplementer,
				Phase:      events.PhaseImplementing,
				AssignedAt: now,
			}
			cs.taskAssignments[op.taskID] = &TaskAssignment{
				TaskID:      op.taskID,
				Implementer: op.workerID,
				Status:      TaskImplementing,
				StartedAt:   now,
			}
			cs.assignmentsMu.Unlock()

			worker := workerPool.GetWorker(op.workerID)
			if worker != nil {
				_ = worker.AssignTask(op.taskID)
			}
		}

		// INVARIANT CHECK: After every operation
		verifyWorkerHasSingleActiveTask(t, cs)
	}
}

// verifyWorkerHasSingleActiveTask checks the invariant.
func verifyWorkerHasSingleActiveTask(t *testing.T, cs *CoordinatorServer) {
	t.Helper()

	cs.assignmentsMu.RLock()
	defer cs.assignmentsMu.RUnlock()

	// Count tasks per worker
	workerTaskCount := make(map[string]int)

	for workerID, wa := range cs.workerAssignments {
		if wa != nil && wa.TaskID != "" {
			workerTaskCount[workerID]++
		}
	}

	for workerID, count := range workerTaskCount {
		require.LessOrEqual(t, count, 1, "INVARIANT VIOLATION: Worker %s has %d active tasks (should be <= 1)", workerID, count)
	}
}

// TestInvariant_ReviewerNotImplementer verifies that for all tasks in review,
// the reviewer is never the same as the implementer.
// Invariant: ∀ task in review: reviewer != implementer
func TestInvariant_ReviewerNotImplementer(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Create workers
	_ = workerPool.AddTestWorker("worker-1", pool.WorkerReady)
	_ = workerPool.AddTestWorker("worker-2", pool.WorkerReady)

	// Set up a task with implementer awaiting review
	cs.SetTaskAssignment("perles-abc.1", &TaskAssignment{
		TaskID:      "perles-abc.1",
		Implementer: "worker-1",
		Status:      TaskImplementing,
	})
	cs.SetWorkerAssignment("worker-1", &WorkerAssignment{
		TaskID: "perles-abc.1",
		Role:   RoleImplementer,
		Phase:  events.PhaseAwaitingReview,
	})

	// Test cases: try various reviewer assignments including self-review
	testCases := []struct {
		name         string
		reviewerID   string
		expectReject bool
	}{
		{"self_review_rejected", "worker-1", true},
		{"different_reviewer_allowed", "worker-2", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := cs.validateReviewAssignment(tc.reviewerID, "perles-abc.1", "worker-1")

			if tc.expectReject {
				require.Error(t, err, "Expected self-review to be rejected, but it was allowed")
			} else {
				require.NoError(t, err, "Expected review to be allowed")
			}

			// If valid, apply assignment
			if err == nil {
				cs.assignmentsMu.Lock()
				cs.workerAssignments[tc.reviewerID] = &WorkerAssignment{
					TaskID:        "perles-abc.1",
					Role:          RoleReviewer,
					Phase:         events.PhaseReviewing,
					ImplementerID: "worker-1",
				}
				if ta := cs.taskAssignments["perles-abc.1"]; ta != nil {
					ta.Reviewer = tc.reviewerID
					ta.Status = TaskInReview
				}
				cs.assignmentsMu.Unlock()
			}

			// INVARIANT CHECK
			verifyReviewerNotImplementer(t, cs)
		})
	}
}

// verifyReviewerNotImplementer checks the invariant.
func verifyReviewerNotImplementer(t *testing.T, cs *CoordinatorServer) {
	t.Helper()

	cs.assignmentsMu.RLock()
	defer cs.assignmentsMu.RUnlock()

	for taskID, ta := range cs.taskAssignments {
		if ta != nil && ta.Reviewer != "" && ta.Status == TaskInReview {
			require.NotEqual(t, ta.Implementer, ta.Reviewer, "INVARIANT VIOLATION: Task %s has same implementer and reviewer: %s", taskID, ta.Implementer)
		}
	}
}

// TestInvariant_TaskInReviewHasImplementerAwaitingReview verifies that when a task
// is in review, the implementer must be in AwaitingReview phase.
// Invariant: ∀ task in review: implementer.phase == AwaitingReview
func TestInvariant_TaskInReviewHasImplementerAwaitingReview(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Create workers
	_ = workerPool.AddTestWorker("worker-1", pool.WorkerReady)
	_ = workerPool.AddTestWorker("worker-2", pool.WorkerReady)

	// Set up task NOT awaiting review
	cs.SetTaskAssignment("perles-abc.1", &TaskAssignment{
		TaskID:      "perles-abc.1",
		Implementer: "worker-1",
		Status:      TaskImplementing,
	})
	cs.SetWorkerAssignment("worker-1", &WorkerAssignment{
		TaskID: "perles-abc.1",
		Role:   RoleImplementer,
		Phase:  events.PhaseImplementing, // NOT awaiting review
	})

	// Try to assign reviewer - should fail because implementer not awaiting review
	err := cs.validateReviewAssignment("worker-2", "perles-abc.1", "worker-1")
	require.Error(t, err, "Expected review assignment to be rejected when implementer not awaiting review")

	// Now set implementer to awaiting review
	cs.SetWorkerAssignment("worker-1", &WorkerAssignment{
		TaskID: "perles-abc.1",
		Role:   RoleImplementer,
		Phase:  events.PhaseAwaitingReview, // Now awaiting review
	})

	// Should now succeed
	err = cs.validateReviewAssignment("worker-2", "perles-abc.1", "worker-1")
	require.NoError(t, err, "Expected review assignment to succeed when implementer awaiting review")
}

// TestInvariant_TaskWithReviewerHasReviewerInReviewingPhase verifies that when
// a task has a reviewer, that reviewer must be in PhaseReviewing.
// Invariant: ∀ task with reviewer: reviewer.phase == Reviewing
func TestInvariant_TaskWithReviewerHasReviewerInReviewingPhase(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Create proper state: task in review with reviewer
	cs.SetTaskAssignment("perles-abc.1", &TaskAssignment{
		TaskID:      "perles-abc.1",
		Implementer: "worker-1",
		Reviewer:    "worker-2",
		Status:      TaskInReview,
	})
	cs.SetWorkerAssignment("worker-1", &WorkerAssignment{
		TaskID:     "perles-abc.1",
		Role:       RoleImplementer,
		Phase:      events.PhaseAwaitingReview,
		ReviewerID: "worker-2",
	})
	cs.SetWorkerAssignment("worker-2", &WorkerAssignment{
		TaskID:        "perles-abc.1",
		Role:          RoleReviewer,
		Phase:         events.PhaseReviewing, // Correct phase
		ImplementerID: "worker-1",
	})

	// Verify invariant holds
	verifyTaskWithReviewerPhase(t, cs, true)

	// Now introduce inconsistent state (for testing invariant detection)
	// This simulates a bug where reviewer phase wasn't updated
	cs.SetWorkerAssignment("worker-2", &WorkerAssignment{
		TaskID:        "perles-abc.1",
		Role:          RoleReviewer,
		Phase:         events.PhaseIdle, // WRONG - should be Reviewing
		ImplementerID: "worker-1",
	})

	// The invariant check should detect this
	verifyTaskWithReviewerPhase(t, cs, false) // Expect failure
}

// verifyTaskWithReviewerPhase checks the invariant. If expectValid is false,
// it expects a violation to be detected.
func verifyTaskWithReviewerPhase(t *testing.T, cs *CoordinatorServer, expectValid bool) {
	t.Helper()

	cs.assignmentsMu.RLock()
	defer cs.assignmentsMu.RUnlock()

	violations := []string{}

	for taskID, ta := range cs.taskAssignments {
		if ta != nil && ta.Reviewer != "" && ta.Status == TaskInReview {
			// Check reviewer's phase
			if wa, ok := cs.workerAssignments[ta.Reviewer]; ok && wa != nil {
				if wa.Phase != events.PhaseReviewing {
					violations = append(violations, taskID)
				}
			}
		}
	}

	if expectValid {
		require.Empty(t, violations, "INVARIANT VIOLATION: Tasks with reviewer not in Reviewing phase: %v", violations)
	} else {
		require.NotEmpty(t, violations, "Expected invariant violation to be detected, but none found")
	}
}

// TestInvariant_AfterAllOperations runs a comprehensive sequence of operations
// and verifies ALL invariants hold after each operation.
func TestInvariant_AfterAllOperations(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Create workers
	_ = workerPool.AddTestWorker("worker-1", pool.WorkerReady)
	_ = workerPool.AddTestWorker("worker-2", pool.WorkerReady)
	_ = workerPool.AddTestWorker("worker-3", pool.WorkerReady)
	_ = workerPool.AddTestWorker("worker-4", pool.WorkerReady)

	// Sequence of operations simulating a typical workflow
	t.Log("Step 1: Assign task to worker-1")
	{
		err := cs.validateTaskAssignment("worker-1", "perles-abc.1")
		require.NoError(t, err, "Unexpected error")
		cs.SetTaskAssignment("perles-abc.1", &TaskAssignment{
			TaskID:      "perles-abc.1",
			Implementer: "worker-1",
			Status:      TaskImplementing,
		})
		cs.SetWorkerAssignment("worker-1", &WorkerAssignment{
			TaskID:     "perles-abc.1",
			Role:       RoleImplementer,
			Phase:      events.PhaseImplementing,
			AssignedAt: time.Now(),
		})
		_ = workerPool.GetWorker("worker-1").AssignTask("perles-abc.1")
		verifyAllInvariants(t, cs)
	}

	t.Log("Step 2: Worker-1 reports implementation complete")
	{
		cs.SetWorkerAssignment("worker-1", &WorkerAssignment{
			TaskID:     "perles-abc.1",
			Role:       RoleImplementer,
			Phase:      events.PhaseAwaitingReview,
			AssignedAt: time.Now(),
		})
		verifyAllInvariants(t, cs)
	}

	t.Log("Step 3: Assign reviewer (worker-2)")
	{
		err := cs.validateReviewAssignment("worker-2", "perles-abc.1", "worker-1")
		require.NoError(t, err, "Unexpected error")
		cs.assignmentsMu.Lock()
		cs.workerAssignments["worker-2"] = &WorkerAssignment{
			TaskID:        "perles-abc.1",
			Role:          RoleReviewer,
			Phase:         events.PhaseReviewing,
			ImplementerID: "worker-1",
			AssignedAt:    time.Now(),
		}
		cs.workerAssignments["worker-1"].ReviewerID = "worker-2"
		cs.taskAssignments["perles-abc.1"].Reviewer = "worker-2"
		cs.taskAssignments["perles-abc.1"].Status = TaskInReview
		cs.assignmentsMu.Unlock()
		verifyAllInvariants(t, cs)
	}

	t.Log("Step 4: Reviewer approves")
	{
		cs.assignmentsMu.Lock()
		cs.taskAssignments["perles-abc.1"].Status = TaskApproved
		cs.workerAssignments["worker-2"].Phase = events.PhaseIdle
		cs.workerAssignments["worker-2"].TaskID = ""
		cs.assignmentsMu.Unlock()
		verifyAllInvariants(t, cs)
	}

	t.Log("Step 5: Assign new task to different worker while first completes")
	{
		err := cs.validateTaskAssignment("worker-3", "perles-abc.2")
		require.NoError(t, err, "Unexpected error")
		cs.SetTaskAssignment("perles-abc.2", &TaskAssignment{
			TaskID:      "perles-abc.2",
			Implementer: "worker-3",
			Status:      TaskImplementing,
		})
		cs.SetWorkerAssignment("worker-3", &WorkerAssignment{
			TaskID:     "perles-abc.2",
			Role:       RoleImplementer,
			Phase:      events.PhaseImplementing,
			AssignedAt: time.Now(),
		})
		verifyAllInvariants(t, cs)
	}
}

// verifyAllInvariants runs all invariant checks.
func verifyAllInvariants(t *testing.T, cs *CoordinatorServer) {
	t.Helper()
	verifyNoTaskHasMultipleImplementers(t, cs)
	verifyNoTaskHasMultipleReviewers(t, cs)
	verifyWorkerHasSingleActiveTask(t, cs)
	verifyReviewerNotImplementer(t, cs)
}
