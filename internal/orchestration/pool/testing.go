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
	worker.onTurnComplete = p.onTurnComplete   // Propagate callback to test workers
	worker.onWorkerRetired = p.onWorkerRetired // Propagate retirement callback to test workers
	p.workers[workerID] = worker
}

// SetSessionIDForTesting sets the session ID for a worker.
// This is only intended for use in tests to set up workers with session IDs
// without going through the normal spawning process.
func (w *Worker) SetSessionIDForTesting(sessionID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.SessionID = sessionID
}

// SetStatusForTesting sets the status for a worker.
// This is only intended for use in tests to manipulate worker state
// for testing race conditions and edge cases.
func (w *Worker) SetStatusForTesting(status WorkerStatus) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.Status = status
}
