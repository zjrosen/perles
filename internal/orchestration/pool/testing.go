package pool

import "github.com/zjrosen/perles/internal/orchestration/events"

// AddWorkerForTesting adds a worker directly to the pool for testing purposes.
// This bypasses the normal spawning process and allows tests to set up
// workers with specific task IDs and phases.
// This function is only intended for use in tests.
func (p *WorkerPool) AddWorkerForTesting(workerID, taskID string, status WorkerStatus, phase events.WorkerPhase) {
	p.mu.Lock()
	defer p.mu.Unlock()

	worker := newWorker(workerID, p.bufferCapacity)
	worker.TaskID = taskID
	worker.Status = status
	worker.Phase = phase
	p.workers[workerID] = worker
}
