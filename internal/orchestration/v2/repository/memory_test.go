package repository

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ===========================================================================
// MemoryTaskRepository Tests
// ===========================================================================

func TestMemoryTaskRepository_Get_ReturnsTask(t *testing.T) {
	repo := NewMemoryTaskRepository()

	task := &TaskAssignment{
		TaskID:      "perles-abc.1",
		Implementer: "worker-1",
		Status:      TaskImplementing,
	}
	require.NoError(t, repo.Save(task))

	got, err := repo.Get("perles-abc.1")
	require.NoError(t, err)
	assert.Equal(t, task, got)
}

func TestMemoryTaskRepository_Get_ReturnsErrorForUnknown(t *testing.T) {
	repo := NewMemoryTaskRepository()

	got, err := repo.Get("nonexistent")
	assert.Nil(t, got)
	assert.ErrorIs(t, err, ErrTaskNotFound)
}

func TestMemoryTaskRepository_Save_CreatesNewTask(t *testing.T) {
	repo := NewMemoryTaskRepository()

	task := &TaskAssignment{
		TaskID:      "perles-abc.1",
		Implementer: "worker-1",
		Status:      TaskImplementing,
		StartedAt:   time.Now(),
	}
	err := repo.Save(task)
	require.NoError(t, err)

	got, err := repo.Get("perles-abc.1")
	require.NoError(t, err)
	assert.Equal(t, "perles-abc.1", got.TaskID)
	assert.Equal(t, "worker-1", got.Implementer)
}

func TestMemoryTaskRepository_Save_UpdatesExistingTask(t *testing.T) {
	repo := NewMemoryTaskRepository()

	task := &TaskAssignment{
		TaskID:      "perles-abc.1",
		Implementer: "worker-1",
		Status:      TaskImplementing,
	}
	require.NoError(t, repo.Save(task))

	// Update the task
	task.Reviewer = "worker-2"
	task.Status = TaskInReview
	require.NoError(t, repo.Save(task))

	got, err := repo.Get("perles-abc.1")
	require.NoError(t, err)
	assert.Equal(t, "worker-2", got.Reviewer)
	assert.Equal(t, TaskInReview, got.Status)
}

func TestMemoryTaskRepository_GetByWorker_FindsByImplementer(t *testing.T) {
	repo := NewMemoryTaskRepository()

	task := &TaskAssignment{
		TaskID:      "perles-abc.1",
		Implementer: "worker-1",
		Status:      TaskImplementing,
	}
	require.NoError(t, repo.Save(task))

	got, err := repo.GetByWorker("worker-1")
	require.NoError(t, err)
	assert.Equal(t, task, got)
}

func TestMemoryTaskRepository_GetByWorker_FindsByReviewer(t *testing.T) {
	repo := NewMemoryTaskRepository()

	task := &TaskAssignment{
		TaskID:      "perles-abc.1",
		Implementer: "worker-1",
		Reviewer:    "worker-2",
		Status:      TaskInReview,
	}
	require.NoError(t, repo.Save(task))

	got, err := repo.GetByWorker("worker-2")
	require.NoError(t, err)
	assert.Equal(t, task, got)
}

func TestMemoryTaskRepository_GetByWorker_ReturnsErrorForUnassigned(t *testing.T) {
	repo := NewMemoryTaskRepository()

	task := &TaskAssignment{
		TaskID:      "perles-abc.1",
		Implementer: "worker-1",
		Status:      TaskImplementing,
	}
	require.NoError(t, repo.Save(task))

	got, err := repo.GetByWorker("worker-99")
	assert.Nil(t, got)
	assert.ErrorIs(t, err, ErrTaskNotFound)
}

func TestMemoryTaskRepository_GetByImplementer_ReturnsAllTasks(t *testing.T) {
	repo := NewMemoryTaskRepository()

	task1 := &TaskAssignment{
		TaskID:      "perles-abc.1",
		Implementer: "worker-1",
	}
	task2 := &TaskAssignment{
		TaskID:      "perles-abc.2",
		Implementer: "worker-1",
	}
	task3 := &TaskAssignment{
		TaskID:      "perles-abc.3",
		Implementer: "worker-2", // Different worker
	}
	require.NoError(t, repo.Save(task1))
	require.NoError(t, repo.Save(task2))
	require.NoError(t, repo.Save(task3))

	tasks, err := repo.GetByImplementer("worker-1")
	require.NoError(t, err)
	assert.Len(t, tasks, 2)

	ids := make(map[string]bool)
	for _, task := range tasks {
		ids[task.TaskID] = true
	}
	assert.True(t, ids["perles-abc.1"])
	assert.True(t, ids["perles-abc.2"])
}

func TestMemoryTaskRepository_GetByImplementer_ReturnsEmptySlice(t *testing.T) {
	repo := NewMemoryTaskRepository()

	tasks, err := repo.GetByImplementer("worker-99")
	require.NoError(t, err)
	assert.Len(t, tasks, 0)
	assert.NotNil(t, tasks)
}

func TestMemoryTaskRepository_Delete_RemovesTask(t *testing.T) {
	repo := NewMemoryTaskRepository()

	task := &TaskAssignment{
		TaskID:      "perles-abc.1",
		Implementer: "worker-1",
	}
	require.NoError(t, repo.Save(task))

	// Verify it exists
	_, err := repo.Get("perles-abc.1")
	require.NoError(t, err)

	// Delete it
	err = repo.Delete("perles-abc.1")
	require.NoError(t, err)

	// Verify it's gone
	_, err = repo.Get("perles-abc.1")
	assert.ErrorIs(t, err, ErrTaskNotFound)
}

func TestMemoryTaskRepository_Delete_NoErrorForNonexistent(t *testing.T) {
	repo := NewMemoryTaskRepository()

	// Deleting a non-existent task should not error
	err := repo.Delete("nonexistent")
	assert.NoError(t, err)
}

func TestMemoryTaskRepository_Reset(t *testing.T) {
	repo := NewMemoryTaskRepository()

	task := &TaskAssignment{TaskID: "perles-abc.1", Implementer: "worker-1"}
	require.NoError(t, repo.Save(task))

	repo.Reset()

	_, err := repo.Get("perles-abc.1")
	assert.ErrorIs(t, err, ErrTaskNotFound)
}

func TestMemoryTaskRepository_AddTask(t *testing.T) {
	repo := NewMemoryTaskRepository()

	task := &TaskAssignment{
		TaskID:      "perles-abc.1",
		Implementer: "worker-1",
		Status:      TaskImplementing,
	}
	repo.AddTask(task)

	got, err := repo.Get("perles-abc.1")
	require.NoError(t, err)
	assert.Equal(t, task, got)
}

// ===========================================================================
// MemoryQueueRepository Tests
// ===========================================================================

func TestMemoryQueueRepository_GetOrCreate_CreatesNewQueue(t *testing.T) {
	repo := NewMemoryQueueRepository(100)

	queue := repo.GetOrCreate("worker-1")
	assert.NotNil(t, queue)
	assert.Equal(t, "worker-1", queue.WorkerID)
	assert.Equal(t, 0, queue.Size())
	assert.Equal(t, 100, queue.MaxSize())
}

func TestMemoryQueueRepository_GetOrCreate_ReturnsExistingQueue(t *testing.T) {
	repo := NewMemoryQueueRepository(100)

	// Create queue and add a message
	queue1 := repo.GetOrCreate("worker-1")
	require.NoError(t, queue1.Enqueue("test message", SenderUser))

	// Get the same queue
	queue2 := repo.GetOrCreate("worker-1")
	assert.Equal(t, queue1, queue2)
	assert.Equal(t, 1, queue2.Size())
}

func TestMemoryQueueRepository_Delete_RemovesQueue(t *testing.T) {
	repo := NewMemoryQueueRepository(100)

	// Create queue and add a message
	queue := repo.GetOrCreate("worker-1")
	require.NoError(t, queue.Enqueue("test message", SenderUser))
	assert.Equal(t, 1, repo.Size("worker-1"))

	// Delete the queue
	repo.Delete("worker-1")

	// Getting it again should create a new empty queue
	newQueue := repo.GetOrCreate("worker-1")
	assert.Equal(t, 0, newQueue.Size())
}

func TestMemoryQueueRepository_Delete_NoErrorForNonexistent(t *testing.T) {
	repo := NewMemoryQueueRepository(100)

	// Should not panic or error
	repo.Delete("nonexistent")
}

func TestMemoryQueueRepository_Size_ReturnsQueueSize(t *testing.T) {
	repo := NewMemoryQueueRepository(100)

	queue := repo.GetOrCreate("worker-1")
	require.NoError(t, queue.Enqueue("msg1", SenderUser))
	require.NoError(t, queue.Enqueue("msg2", SenderUser))
	require.NoError(t, queue.Enqueue("msg3", SenderUser))

	assert.Equal(t, 3, repo.Size("worker-1"))
}

func TestMemoryQueueRepository_Size_ReturnsZeroForNonexistent(t *testing.T) {
	repo := NewMemoryQueueRepository(100)

	assert.Equal(t, 0, repo.Size("nonexistent"))
}

func TestMemoryQueueRepository_Reset(t *testing.T) {
	repo := NewMemoryQueueRepository(100)

	queue := repo.GetOrCreate("worker-1")
	require.NoError(t, queue.Enqueue("test", SenderUser))

	repo.Reset()

	assert.Equal(t, 0, repo.Size("worker-1"))
}

func TestMemoryQueueRepository_AddQueue(t *testing.T) {
	repo := NewMemoryQueueRepository(100)

	// Create a custom queue
	customQueue := NewMessageQueue("worker-1", 50)
	require.NoError(t, customQueue.Enqueue("preset message", SenderUser))

	repo.AddQueue(customQueue)

	got := repo.GetOrCreate("worker-1")
	assert.Equal(t, customQueue, got)
	assert.Equal(t, 1, got.Size())
	assert.Equal(t, 50, got.MaxSize())
}

func TestMemoryQueueRepository_UnlimitedCapacity(t *testing.T) {
	repo := NewMemoryQueueRepository(0) // 0 means unlimited

	queue := repo.GetOrCreate("worker-1")
	assert.Equal(t, 0, queue.MaxSize())

	// Should be able to enqueue many messages
	for i := 0; i < 100; i++ {
		err := queue.Enqueue("message", SenderUser)
		require.NoError(t, err)
	}
	assert.Equal(t, 100, queue.Size())
}

func TestMemoryQueueRepository_ClearAll_EmptyRepository(t *testing.T) {
	repo := NewMemoryQueueRepository(100)

	// ClearAll on empty repo should not panic
	repo.ClearAll()

	// Verify repo is still usable
	queue := repo.GetOrCreate("worker-1")
	assert.NotNil(t, queue)
	assert.Equal(t, 0, queue.Size())
}

func TestMemoryQueueRepository_ClearAll_SingleQueue(t *testing.T) {
	repo := NewMemoryQueueRepository(100)

	// Create a queue with messages
	queue := repo.GetOrCreate("worker-1")
	require.NoError(t, queue.Enqueue("msg1", SenderUser))
	require.NoError(t, queue.Enqueue("msg2", SenderCoordinator))
	assert.Equal(t, 2, repo.Size("worker-1"))

	// Clear all
	repo.ClearAll()

	// Queue should be gone (Size returns 0 for non-existent queue)
	assert.Equal(t, 0, repo.Size("worker-1"))
}

func TestMemoryQueueRepository_ClearAll_MultipleQueues(t *testing.T) {
	repo := NewMemoryQueueRepository(100)

	// Create multiple queues with messages
	for i := 0; i < 5; i++ {
		queue := repo.GetOrCreate(workerID(i))
		for j := 0; j < 3; j++ {
			require.NoError(t, queue.Enqueue("message", SenderUser))
		}
		assert.Equal(t, 3, repo.Size(workerID(i)))
	}

	// Clear all
	repo.ClearAll()

	// All queues should be gone
	for i := 0; i < 5; i++ {
		assert.Equal(t, 0, repo.Size(workerID(i)))
	}
}

func TestMemoryQueueRepository_ClearAll_GetOrCreateReturnsFreshQueue(t *testing.T) {
	repo := NewMemoryQueueRepository(100)

	// Create a queue with messages
	queue := repo.GetOrCreate("worker-1")
	require.NoError(t, queue.Enqueue("old message", SenderUser))
	assert.Equal(t, 1, queue.Size())

	// Clear all
	repo.ClearAll()

	// GetOrCreate should return a fresh empty queue
	newQueue := repo.GetOrCreate("worker-1")
	assert.NotNil(t, newQueue)
	assert.Equal(t, 0, newQueue.Size())
	assert.Equal(t, "worker-1", newQueue.WorkerID)
	assert.Equal(t, 100, newQueue.MaxSize())
}

func TestMemoryQueueRepository_ClearAll_ThreadSafety(t *testing.T) {
	repo := NewMemoryQueueRepository(1000)
	var wg sync.WaitGroup
	numGoroutines := 50

	// Pre-populate with queues
	for i := 0; i < 20; i++ {
		queue := repo.GetOrCreate(workerID(i))
		queue.Enqueue("message", SenderUser)
	}

	// Concurrent ClearAll, GetOrCreate, and Size operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(3)

		// ClearAll
		go func() {
			defer wg.Done()
			repo.ClearAll()
		}()

		// GetOrCreate
		go func(id int) {
			defer wg.Done()
			repo.GetOrCreate(workerID(id))
		}(i)

		// Size
		go func(id int) {
			defer wg.Done()
			repo.Size(workerID(id % 20))
		}(i)
	}

	wg.Wait()
	// Test passes if no panics or races occurred
}

// ===========================================================================
// Thread Safety Tests
// ===========================================================================

func TestMemoryTaskRepository_ThreadSafety(t *testing.T) {
	repo := NewMemoryTaskRepository()
	var wg sync.WaitGroup
	numGoroutines := 100

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			task := &TaskAssignment{
				TaskID:      taskID(id),
				Implementer: workerID(id % 10),
				Status:      TaskImplementing,
			}
			repo.Save(task)
		}(i)
	}

	wg.Wait()

	// Count tasks (some may have been overwritten by same TaskID)
	// Just verify no panics occurred
}

func TestMemoryTaskRepository_ConcurrentReadWrite(t *testing.T) {
	repo := NewMemoryTaskRepository()
	var wg sync.WaitGroup

	// Pre-populate
	for i := 0; i < 10; i++ {
		repo.Save(&TaskAssignment{
			TaskID:      taskID(i),
			Implementer: workerID(i),
		})
	}

	// Concurrent operations
	for i := 0; i < 50; i++ {
		wg.Add(4)

		// GetByWorker
		go func(id int) {
			defer wg.Done()
			repo.GetByWorker(workerID(id % 10))
		}(i)

		// GetByImplementer
		go func(id int) {
			defer wg.Done()
			repo.GetByImplementer(workerID(id % 10))
		}(i)

		// Save
		go func(id int) {
			defer wg.Done()
			repo.Save(&TaskAssignment{
				TaskID:      taskID(100 + id),
				Implementer: workerID(id),
			})
		}(i)

		// Delete
		go func(id int) {
			defer wg.Done()
			repo.Delete(taskID(200 + id))
		}(i)
	}

	wg.Wait()
}

func TestMemoryQueueRepository_ThreadSafety(t *testing.T) {
	repo := NewMemoryQueueRepository(1000)
	var wg sync.WaitGroup
	numGoroutines := 100

	// Concurrent GetOrCreate on different workers (each goroutine gets its own queue)
	// This tests the repository's map access thread safety
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// Each goroutine gets a unique worker ID to avoid racing on queue internals
			queue := repo.GetOrCreate(workerID(id))
			queue.Enqueue("message", SenderUser)
		}(i)
	}

	wg.Wait()

	// Verify all queues were created (no lost updates)
	for i := 0; i < numGoroutines; i++ {
		queue := repo.GetOrCreate(workerID(i))
		assert.Equal(t, 1, queue.Size(), "queue %d should have exactly 1 message", i)
	}
}

func TestMemoryQueueRepository_ConcurrentReadWrite(t *testing.T) {
	repo := NewMemoryQueueRepository(1000)
	var wg sync.WaitGroup

	// Pre-populate with many queues in isolated ranges
	// Range 0-99: for Size reads only
	// Range 200-249: for GetOrCreate writes
	// Range 300-349: for Delete operations
	for i := 0; i < 100; i++ {
		queue := repo.GetOrCreate(workerID(i))
		queue.Enqueue("initial message", SenderUser)
	}

	// Concurrent repository operations on non-overlapping ranges
	for i := 0; i < 50; i++ {
		wg.Add(3)

		// GetOrCreate on unique workers (200-249 range)
		go func(id int) {
			defer wg.Done()
			queue := repo.GetOrCreate(workerID(200 + id))
			queue.Enqueue("new queue message", SenderUser)
		}(i)

		// Size on pre-populated queues (0-99 range, read-only after pre-populate)
		go func(id int) {
			defer wg.Done()
			repo.Size(workerID(id % 100))
		}(i)

		// Delete on non-existent queues (300-349 range)
		go func(id int) {
			defer wg.Done()
			repo.Delete(workerID(300 + id))
		}(i)
	}

	wg.Wait()
}

// TestMemoryQueueRepository_ConcurrentSizeDuringQueueMutation verifies that
// reading a queue's size through the repository (as read-only queries like
// query_worker_state do) is safe while handlers mutate the same queue.
func TestMemoryQueueRepository_ConcurrentSizeDuringQueueMutation(t *testing.T) {
	repo := NewMemoryQueueRepository(0)
	queue := repo.GetOrCreate("shared-worker")
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = queue.Enqueue("message", SenderUser)
			queue.Dequeue()
		}
		queue.Drain()
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			repo.Size("shared-worker")
			queue.IsEmpty()
		}
	}()

	wg.Wait()
	assert.Equal(t, 0, repo.Size("shared-worker"))
}

// TestMemoryQueueRepository_ConcurrentGetOrCreate verifies that concurrent
// GetOrCreate calls for the same worker ID return the same queue instance.
func TestMemoryQueueRepository_ConcurrentGetOrCreate(t *testing.T) {
	repo := NewMemoryQueueRepository(1000)
	var wg sync.WaitGroup
	numGoroutines := 50

	queues := make(chan *MessageQueue, numGoroutines)

	// Multiple goroutines all trying to GetOrCreate the same queue
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			queue := repo.GetOrCreate("shared-worker")
			queues <- queue
		}()
	}

	wg.Wait()
	close(queues)

	// All goroutines should have received the same queue instance
	var first *MessageQueue
	for q := range queues {
		if first == nil {
			first = q
		} else {
			assert.Equal(t, first, q, "all GetOrCreate calls should return the same queue")
		}
	}
}

// Helper functions for generating test IDs
func workerID(n int) string {
	return fmt.Sprintf("worker-%d", n)
}

func taskID(n int) string {
	return fmt.Sprintf("perles-test.%d", n)
}
