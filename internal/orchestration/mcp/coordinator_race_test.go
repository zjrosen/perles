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
// Race Detection Tests
//
// These tests are designed to expose data races when run with -race flag.
// They perform concurrent operations on shared state to verify thread safety.
// ============================================================================

// TestRace_ConcurrentWorkerAssignments tests concurrent access to workerAssignments map.
func TestRace_ConcurrentWorkerAssignments(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Create multiple workers
	for i := 0; i < 10; i++ {
		id := workerID(i)
		_ = workerPool.AddTestWorker(id, pool.WorkerReady)
	}

	var wg sync.WaitGroup
	errChan := make(chan error, 100)

	// Concurrent writers: set assignments
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				cs.assignmentsMu.Lock()
				cs.workerAssignments[workerID(idx)] = &WorkerAssignment{
					TaskID:     taskID(j),
					Role:       RoleImplementer,
					Phase:      events.PhaseImplementing,
					AssignedAt: time.Now(),
				}
				cs.assignmentsMu.Unlock()
				time.Sleep(time.Millisecond)
			}
		}(i)
	}

	// Concurrent readers: query worker state
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				// Simulate query_worker_state
				cs.assignmentsMu.RLock()
				workers := make([]workerStateInfo, 0)
				for wID, wa := range cs.workerAssignments {
					if wa != nil {
						workers = append(workers, workerStateInfo{
							WorkerID: wID,
							Phase:    string(wa.Phase),
							Role:     string(wa.Role),
							TaskID:   wa.TaskID,
						})
					}
				}
				cs.assignmentsMu.RUnlock()
				_ = workers // Use result to prevent optimization
				time.Sleep(time.Millisecond)
			}
		}()
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		require.NoError(t, err, "Concurrent operation error")
	}
}

// TestRace_ConcurrentTaskAssignments tests concurrent access to taskAssignments map.
func TestRace_ConcurrentTaskAssignments(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	var wg sync.WaitGroup

	// Concurrent writers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				tID := taskID((idx*20)+j%10) + "." + workerID(idx)[len(workerID(idx))-1:]
				cs.assignmentsMu.Lock()
				cs.taskAssignments[tID] = &TaskAssignment{
					TaskID:      tID,
					Implementer: workerID(idx),
					Status:      TaskImplementing,
					StartedAt:   time.Now(),
				}
				cs.assignmentsMu.Unlock()
				time.Sleep(time.Millisecond)
			}
		}(i)
	}

	// Concurrent readers: check orphaned tasks
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				orphans := cs.detectOrphanedTasks()
				_ = orphans // Use result
				time.Sleep(time.Millisecond)
			}
		}()
	}

	// Concurrent readers: check stuck workers
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				stuck := cs.checkStuckWorkers()
				_ = stuck // Use result
				time.Sleep(time.Millisecond)
			}
		}()
	}

	wg.Wait()
}

// TestRace_ConcurrentToolCalls tests concurrent tool handler invocations.
func TestRace_ConcurrentToolCalls(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := message.New()
	cs := NewCoordinatorServer(claude.NewClient(), workerPool, msgIssue, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Create workers
	for i := 0; i < 5; i++ {
		_ = workerPool.AddTestWorker(workerID(i), pool.WorkerReady)
	}

	var wg sync.WaitGroup
	ctx := context.Background()

	// Concurrent list_workers calls
	wg.Add(1)
	go func() {
		defer wg.Done()
		handler := cs.handlers["list_workers"]
		for i := 0; i < 50; i++ {
			_, _ = handler(ctx, nil)
			time.Sleep(500 * time.Microsecond)
		}
	}()

	// Concurrent query_worker_state calls
	wg.Add(1)
	go func() {
		defer wg.Done()
		handler := cs.handlers["query_worker_state"]
		for i := 0; i < 50; i++ {
			_, _ = handler(ctx, json.RawMessage(`{}`))
			time.Sleep(500 * time.Microsecond)
		}
	}()

	// Concurrent post_message calls
	wg.Add(1)
	go func() {
		defer wg.Done()
		handler := cs.handlers["post_message"]
		for i := 0; i < 20; i++ {
			args := `{"to": "ALL", "content": "test message ` + workerID(i) + `"}`
			_, _ = handler(ctx, json.RawMessage(args))
			time.Sleep(500 * time.Microsecond)
		}
	}()

	// Concurrent read_message_log calls
	wg.Add(1)
	go func() {
		defer wg.Done()
		handler := cs.handlers["read_message_log"]
		for i := 0; i < 50; i++ {
			_, _ = handler(ctx, nil)
			time.Sleep(500 * time.Microsecond)
		}
	}()

	wg.Wait()
}

// TestRace_AssignTaskWhileValidating tests concurrent assignment and validation.
func TestRace_AssignTaskWhileValidating(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Create workers
	for i := 0; i < 10; i++ {
		_ = workerPool.AddTestWorker(workerID(i), pool.WorkerReady)
	}

	var wg sync.WaitGroup

	// Multiple goroutines trying to validate and assign the same task
	taskIDToAssign := "perles-abc.1"

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			wID := workerID(idx)

			// Try to validate
			err := cs.validateTaskAssignment(wID, taskIDToAssign)
			if err == nil {
				// If validation passes, try to actually assign
				// This simulates the race condition where multiple workers
				// might pass validation before any assignment is recorded
				cs.assignmentsMu.Lock()

				// Double-check inside lock
				if _, alreadyAssigned := cs.taskAssignments[taskIDToAssign]; !alreadyAssigned {
					cs.workerAssignments[wID] = &WorkerAssignment{
						TaskID:     taskIDToAssign,
						Role:       RoleImplementer,
						Phase:      events.PhaseImplementing,
						AssignedAt: time.Now(),
					}
					cs.taskAssignments[taskIDToAssign] = &TaskAssignment{
						TaskID:      taskIDToAssign,
						Implementer: wID,
						Status:      TaskImplementing,
						StartedAt:   time.Now(),
					}
				}
				cs.assignmentsMu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	// Verify only one worker got the task
	cs.assignmentsMu.RLock()
	defer cs.assignmentsMu.RUnlock()

	assignedWorkers := []string{}
	for wID, wa := range cs.workerAssignments {
		if wa != nil && wa.TaskID == taskIDToAssign {
			assignedWorkers = append(assignedWorkers, wID)
		}
	}

	require.LessOrEqual(t, len(assignedWorkers), 1, "Race condition: Task assigned to multiple workers: %v", assignedWorkers)
}

// TestRace_ReviewAssignmentWhileImplementing tests concurrent review and implementation.
func TestRace_ReviewAssignmentWhileImplementing(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Create workers
	for i := 0; i < 10; i++ {
		_ = workerPool.AddTestWorker(workerID(i), pool.WorkerReady)
	}

	// Set up task awaiting review
	cs.SetTaskAssignment("perles-abc.1", &TaskAssignment{
		TaskID:      "perles-abc.1",
		Implementer: "worker-0",
		Status:      TaskImplementing,
	})
	cs.SetWorkerAssignment("worker-0", &WorkerAssignment{
		TaskID: "perles-abc.1",
		Role:   RoleImplementer,
		Phase:  events.PhaseAwaitingReview,
	})

	var wg sync.WaitGroup

	// Multiple goroutines trying to become the reviewer
	for i := 1; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			wID := workerID(idx)

			err := cs.validateReviewAssignment(wID, "perles-abc.1", "worker-0")
			if err == nil {
				cs.assignmentsMu.Lock()
				// Double-check inside lock
				ta := cs.taskAssignments["perles-abc.1"]
				if ta != nil && ta.Reviewer == "" {
					cs.workerAssignments[wID] = &WorkerAssignment{
						TaskID:        "perles-abc.1",
						Role:          RoleReviewer,
						Phase:         events.PhaseReviewing,
						ImplementerID: "worker-0",
						AssignedAt:    time.Now(),
					}
					ta.Reviewer = wID
					ta.Status = TaskInReview
				}
				cs.assignmentsMu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	// Verify only one reviewer was assigned
	cs.assignmentsMu.RLock()
	defer cs.assignmentsMu.RUnlock()

	reviewers := []string{}
	for wID, wa := range cs.workerAssignments {
		if wa != nil && wa.Role == RoleReviewer && wa.TaskID == "perles-abc.1" {
			reviewers = append(reviewers, wID)
		}
	}

	require.LessOrEqual(t, len(reviewers), 1, "Race condition: Task has multiple reviewers: %v", reviewers)
}

// TestRace_MessagePostWhileRead tests concurrent message posting and reading.
func TestRace_MessagePostWhileRead(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := message.New()
	cs := NewCoordinatorServer(claude.NewClient(), workerPool, msgIssue, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	var wg sync.WaitGroup
	ctx := context.Background()

	// Posters
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			handler := cs.handlers["post_message"]
			for j := 0; j < 20; j++ {
				args := `{"to": "ALL", "content": "message from ` + workerID(idx) + ` #` + taskID(j) + `"}`
				_, _ = handler(ctx, json.RawMessage(args))
				time.Sleep(100 * time.Microsecond)
			}
		}(i)
	}

	// Readers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			handler := cs.handlers["read_message_log"]
			for j := 0; j < 20; j++ {
				_, _ = handler(ctx, nil)
				time.Sleep(100 * time.Microsecond)
			}
		}()
	}

	wg.Wait()
}

// TestRace_UnreadMessageTracking tests concurrent access to unread message tracking.
// This specifically targets the UnreadFor/MarkRead sequence under load with
// interleaved post_message and read_message_log operations.
func TestRace_UnreadMessageTracking(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := message.New()
	cs := NewCoordinatorServer(claude.NewClient(), workerPool, msgIssue, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	var wg sync.WaitGroup
	ctx := context.Background()

	// Pre-populate with some messages to stress the unread tracking
	for i := 0; i < 10; i++ {
		_, _ = msgIssue.Append("worker-setup", message.ActorCoordinator, "setup message", message.MessageInfo)
	}

	// Multiple concurrent readers using default unread mode (UnreadFor + MarkRead)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			handler := cs.handlers["read_message_log"]
			for j := 0; j < 30; j++ {
				// Default mode (no read_all) triggers UnreadFor + MarkRead
				_, _ = handler(ctx, nil)
				time.Sleep(50 * time.Microsecond)
			}
		}(i)
	}

	// Multiple concurrent readers using read_all mode (just Entries)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			handler := cs.handlers["read_message_log"]
			for j := 0; j < 30; j++ {
				// read_all=true uses Entries() without affecting read state
				_, _ = handler(ctx, json.RawMessage(`{"read_all": true}`))
				time.Sleep(50 * time.Microsecond)
			}
		}(i)
	}

	// Concurrent posters to interleave with reads
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			handler := cs.handlers["post_message"]
			for j := 0; j < 30; j++ {
				args := `{"to": "ALL", "content": "concurrent message from worker-` + string(rune('0'+idx%10)) + ` iteration ` + string(rune('0'+j%10)) + `"}`
				_, _ = handler(ctx, json.RawMessage(args))
				time.Sleep(50 * time.Microsecond)
			}
		}(i)
	}

	// Direct UnreadFor/MarkRead calls on the message issue to stress the readState map
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			agentID := "test-agent-" + string(rune('0'+idx%10))
			for j := 0; j < 30; j++ {
				// Interleave UnreadFor and MarkRead calls
				unread := msgIssue.UnreadFor(agentID)
				_ = unread // Use result
				if j%2 == 0 {
					msgIssue.MarkRead(agentID)
				}
				time.Sleep(50 * time.Microsecond)
			}
		}(i)
	}

	wg.Wait()

	// Verify no corruption: total count should be consistent
	totalCount := msgIssue.Count()
	require.GreaterOrEqual(t, totalCount, 10, "Should have at least the setup messages")

	// Verify we can still read all messages after concurrent operations
	allEntries := msgIssue.Entries()
	require.Len(t, allEntries, totalCount, "Entries count should match Count()")
}

// Helper functions for generating IDs
func workerID(i int) string {
	return "worker-" + string(rune('0'+i%10))
}

func taskID(i int) string {
	return "perles-abc" + string(rune('0'+i%10))
}
