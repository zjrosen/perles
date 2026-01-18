// Package repository provides domain entity definitions and repository interfaces
// for the v2 orchestration architecture.
package repository

import (
	"sync"

	"github.com/zjrosen/perles/internal/orchestration/events"
)

// DefaultQueueMaxSize is the default maximum size for message queues.
const DefaultQueueMaxSize = 1000

// ===========================================================================
// MemoryTaskRepository
// ===========================================================================

// MemoryTaskRepository is an in-memory implementation of TaskRepository.
// It is thread-safe using sync.RWMutex for concurrent access.
type MemoryTaskRepository struct {
	mu    sync.RWMutex
	tasks map[string]*TaskAssignment
}

// NewMemoryTaskRepository creates a new in-memory task repository.
func NewMemoryTaskRepository() *MemoryTaskRepository {
	return &MemoryTaskRepository{
		tasks: make(map[string]*TaskAssignment),
	}
}

// Get retrieves a task assignment by task ID.
// Returns ErrTaskNotFound if the task does not exist.
func (r *MemoryTaskRepository) Get(taskID string) (*TaskAssignment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	task, ok := r.tasks[taskID]
	if !ok {
		return nil, ErrTaskNotFound
	}
	return task, nil
}

// Save persists a task assignment. Creates new or updates existing.
func (r *MemoryTaskRepository) Save(task *TaskAssignment) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tasks[task.TaskID] = task
	return nil
}

// GetByWorker retrieves the task currently assigned to a worker (as implementer or reviewer).
// Returns ErrTaskNotFound if no task is assigned to the worker.
func (r *MemoryTaskRepository) GetByWorker(workerID string) (*TaskAssignment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, task := range r.tasks {
		if task.Implementer == workerID || task.Reviewer == workerID {
			return task, nil
		}
	}
	return nil, ErrTaskNotFound
}

// GetByImplementer retrieves all tasks where the worker is the implementer.
func (r *MemoryTaskRepository) GetByImplementer(workerID string) ([]*TaskAssignment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*TaskAssignment, 0)
	for _, task := range r.tasks {
		if task.Implementer == workerID {
			result = append(result, task)
		}
	}
	return result, nil
}

// All returns all task assignments in the repository.
func (r *MemoryTaskRepository) All() []*TaskAssignment {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*TaskAssignment, 0, len(r.tasks))
	for _, task := range r.tasks {
		result = append(result, task)
	}
	return result
}

// Delete removes a task assignment from the repository.
func (r *MemoryTaskRepository) Delete(taskID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.tasks, taskID)
	return nil
}

// Reset clears all state from the repository. Useful for test setup/teardown.
func (r *MemoryTaskRepository) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tasks = make(map[string]*TaskAssignment)
}

// AddTask adds a preset task for tests. This is a convenience method
// that bypasses any validation for test setup.
func (r *MemoryTaskRepository) AddTask(task *TaskAssignment) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tasks[task.TaskID] = task
}

// ===========================================================================
// MemoryQueueRepository
// ===========================================================================

// MemoryQueueRepository is an in-memory implementation of QueueRepository.
// It is thread-safe using sync.RWMutex for concurrent access.
type MemoryQueueRepository struct {
	mu      sync.RWMutex
	queues  map[string]*MessageQueue
	maxSize int // Default max size for new queues
}

// NewMemoryQueueRepository creates a new in-memory queue repository.
// The maxSize parameter sets the default maximum size for new queues.
// Use 0 for unlimited capacity.
func NewMemoryQueueRepository(maxSize int) *MemoryQueueRepository {
	return &MemoryQueueRepository{
		queues:  make(map[string]*MessageQueue),
		maxSize: maxSize,
	}
}

// GetOrCreate retrieves or creates a message queue for a worker.
// Never returns nil - creates a new queue if one doesn't exist.
func (r *MemoryQueueRepository) GetOrCreate(workerID string) *MessageQueue {
	r.mu.Lock()
	defer r.mu.Unlock()

	queue, ok := r.queues[workerID]
	if !ok {
		queue = NewMessageQueue(workerID, r.maxSize)
		r.queues[workerID] = queue
	}
	return queue
}

// Delete removes a worker's message queue from the repository.
func (r *MemoryQueueRepository) Delete(workerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.queues, workerID)
}

// Size returns the number of messages in a worker's queue.
// Returns 0 if the worker has no queue.
func (r *MemoryQueueRepository) Size(workerID string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	queue, ok := r.queues[workerID]
	if !ok {
		return 0
	}
	return queue.Size()
}

// Reset clears all state from the repository. Useful for test setup/teardown.
func (r *MemoryQueueRepository) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.queues = make(map[string]*MessageQueue)
}

// AddQueue adds a preset queue for tests. This is a convenience method
// that bypasses any validation for test setup.
func (r *MemoryQueueRepository) AddQueue(queue *MessageQueue) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.queues[queue.WorkerID] = queue
}

// ===========================================================================
// MemoryProcessRepository
// ===========================================================================

// MemoryProcessRepository is an in-memory implementation of ProcessRepository.
// It is thread-safe using sync.RWMutex for concurrent access.
type MemoryProcessRepository struct {
	mu        sync.RWMutex
	processes map[string]*Process
}

// NewMemoryProcessRepository creates a new in-memory process repository.
func NewMemoryProcessRepository() *MemoryProcessRepository {
	return &MemoryProcessRepository{
		processes: make(map[string]*Process),
	}
}

// Get retrieves a process by ID.
// Returns ErrProcessNotFound if the process does not exist.
// Returns a copy of the process to avoid races with concurrent modifications.
func (r *MemoryProcessRepository) Get(processID string) (*Process, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	process, ok := r.processes[processID]
	if !ok {
		return nil, ErrProcessNotFound
	}
	// Return a copy to avoid races with handler modifications
	copy := *process
	return &copy, nil
}

// Save persists a process. Creates new or updates existing.
func (r *MemoryProcessRepository) Save(process *Process) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.processes[process.ID] = process
	return nil
}

// List returns all processes in the repository.
func (r *MemoryProcessRepository) List() []*Process {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Process, 0, len(r.processes))
	for _, process := range r.processes {
		result = append(result, process)
	}
	return result
}

// GetCoordinator returns the coordinator process.
// Returns ErrProcessNotFound if coordinator hasn't been created.
func (r *MemoryProcessRepository) GetCoordinator() (*Process, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, process := range r.processes {
		if process.Role == RoleCoordinator {
			return process, nil
		}
	}
	return nil, ErrProcessNotFound
}

// Workers returns all worker processes (excluding coordinator).
func (r *MemoryProcessRepository) Workers() []*Process {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Process, 0)
	for _, process := range r.processes {
		if process.Role == RoleWorker {
			result = append(result, process)
		}
	}
	return result
}

// ActiveWorkers returns workers not in terminal state (not Retired or Failed).
func (r *MemoryProcessRepository) ActiveWorkers() []*Process {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Process, 0)
	for _, process := range r.processes {
		if process.Role == RoleWorker && process.Status != StatusRetired && process.Status != StatusFailed {
			result = append(result, process)
		}
	}
	return result
}

// ReadyWorkers returns workers available for assignment (Ready status, Idle phase).
func (r *MemoryProcessRepository) ReadyWorkers() []*Process {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Process, 0)
	for _, process := range r.processes {
		if process.Role == RoleWorker && process.Status == StatusReady {
			// Check if Phase is nil or Idle
			if process.Phase == nil || *process.Phase == events.ProcessPhaseIdle {
				result = append(result, process)
			}
		}
	}
	return result
}

// RetiredWorkers returns workers in terminal state (Retired or Failed).
func (r *MemoryProcessRepository) RetiredWorkers() []*Process {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Process, 0)
	for _, process := range r.processes {
		if process.Role == RoleWorker && (process.Status == StatusRetired || process.Status == StatusFailed) {
			result = append(result, process)
		}
	}
	return result
}

// Reset clears all state from the repository. Useful for test setup/teardown.
func (r *MemoryProcessRepository) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.processes = make(map[string]*Process)
}

// AddProcess adds a preset process for tests. This is a convenience method
// that bypasses any validation for test setup.
func (r *MemoryProcessRepository) AddProcess(process *Process) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.processes[process.ID] = process
}
