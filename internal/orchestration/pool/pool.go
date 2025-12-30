package pool

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/pubsub"
)

// WorkerEvent is an alias to events.WorkerEvent for backward compatibility.
type WorkerEvent = events.WorkerEvent

// WorkerEventType is an alias to events.WorkerEventType for backward compatibility.
type WorkerEventType = events.WorkerEventType

// DefaultMaxWorkers is the default maximum number of concurrent workers.
// In the pool model, we maintain a fixed pool of 4 workers.
const DefaultMaxWorkers = 4

// DefaultBufferCapacity is the default number of lines each worker's output buffer holds.
const DefaultBufferCapacity = 100

// ErrPoolClosed is returned when operations are attempted on a closed pool.
var ErrPoolClosed = fmt.Errorf("worker pool is closed")

// ErrMaxWorkers is returned when spawning would exceed the max worker limit.
var ErrMaxWorkers = fmt.Errorf("max workers limit reached")

// Config holds configuration for the worker pool.
type Config struct {
	// Client is the headless client for spawning AI processes.
	// This allows injection of different providers (Claude, Amp, etc.)
	// or mock clients for testing.
	Client client.HeadlessClient

	MaxWorkers     int // Maximum concurrent workers (default: 4)
	BufferCapacity int // Lines per worker output buffer (default: 100)
}

// WorkerPool manages multiple concurrent AI processes.
type WorkerPool struct {
	client         client.HeadlessClient
	workers        map[string]*Worker
	maxWorkers     int
	bufferCapacity int
	broker         *pubsub.Broker[events.WorkerEvent]
	ctx            context.Context
	cancel         context.CancelFunc
	mu             sync.RWMutex
	closed         atomic.Bool
	workerCounter  atomic.Int64
	wg             sync.WaitGroup
}

// NewWorkerPool creates a new worker pool with the given configuration.
// Note: Config.Client must be set for the pool to spawn workers.
func NewWorkerPool(cfg Config) *WorkerPool {
	if cfg.MaxWorkers <= 0 {
		cfg.MaxWorkers = DefaultMaxWorkers
	}
	if cfg.BufferCapacity <= 0 {
		cfg.BufferCapacity = DefaultBufferCapacity
	}

	ctx, cancel := context.WithCancel(context.Background())

	p := &WorkerPool{
		client:         cfg.Client,
		workers:        make(map[string]*Worker),
		maxWorkers:     cfg.MaxWorkers,
		bufferCapacity: cfg.BufferCapacity,
		broker:         pubsub.NewBroker[events.WorkerEvent](),
		ctx:            ctx,
		cancel:         cancel,
	}

	return p
}

// NextWorkerID returns the next worker ID without actually creating a worker.
// This is useful for pre-generating worker IDs for MCP config generation.
func (p *WorkerPool) NextWorkerID() string {
	workerNum := p.workerCounter.Add(1)
	return fmt.Sprintf("worker-%d", workerNum)
}

// SpawnWorker creates and starts a new idle worker in the pool.
// Workers start in Ready state, waiting for task assignment.
// Returns the worker ID and an error if spawning fails.
func (p *WorkerPool) SpawnWorker(cfg client.Config) (string, error) {
	return p.SpawnWorkerWithID("", cfg)
}

// SpawnWorkerWithID creates and starts a new idle worker with a specific ID.
// If workerID is empty, a new ID will be generated using NextWorkerID.
// Workers start in Ready state, waiting for task assignment.
// Returns the worker ID and an error if spawning fails.
func (p *WorkerPool) SpawnWorkerWithID(workerID string, cfg client.Config) (string, error) {
	if p.closed.Load() {
		return "", ErrPoolClosed
	}

	if p.client == nil {
		return "", fmt.Errorf("worker pool has no client configured")
	}

	p.mu.Lock()

	// Check worker limit (only count active workers, not retired)
	activeCount := 0
	for _, w := range p.workers {
		if w.GetStatus().IsActive() {
			activeCount++
		}
	}
	if activeCount >= p.maxWorkers {
		p.mu.Unlock()
		return "", ErrMaxWorkers
	}

	// Use provided worker ID or generate a new one
	if workerID == "" {
		workerNum := p.workerCounter.Add(1)
		workerID = fmt.Sprintf("worker-%d", workerNum)
	}

	// Create worker in Ready state
	worker := newWorker(workerID, p.bufferCapacity)
	p.workers[workerID] = worker
	p.mu.Unlock()

	log.Debug(log.CatOrch, "Spawning worker", "subsystem", "pool", "workerID", workerID)

	// Spawn AI process
	proc, err := p.client.Spawn(p.ctx, cfg)
	if err != nil {
		p.mu.Lock()
		delete(p.workers, workerID)
		p.mu.Unlock()
		log.ErrorErr(log.CatOrch, "Failed to spawn AI process", err,
			"subsystem", "pool",
			"workerID", workerID)
		return "", fmt.Errorf("failed to spawn AI process: %w", err)
	}

	// Start worker goroutine (worker.start() emits WorkerSpawned event with correct status)
	p.wg.Add(1)
	go func(wID string) {
		defer p.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				log.Error(log.CatOrch, "Worker panic recovered",
					"subsystem", "pool",
					"panic", r,
					"workerID", wID,
					"stack", string(debug.Stack()))
			}
		}()
		worker.start(p.ctx, proc, p.broker)
	}(workerID)

	return workerID, nil
}

// ResumeWorker resumes a worker by providing a new AI process.
// This is used when send_to_worker is called to continue a conversation with a worker.
// Returns an error if the worker doesn't exist.
func (p *WorkerPool) ResumeWorker(workerID string, proc client.HeadlessProcess) error {
	if p.closed.Load() {
		return ErrPoolClosed
	}

	p.mu.Lock()
	worker := p.workers[workerID]
	p.mu.Unlock()

	if worker == nil {
		return fmt.Errorf("worker not found: %s", workerID)
	}

	log.Debug(log.CatOrch, "Resuming worker", "subsystem", "pool", "workerID", workerID)

	// Start worker goroutine to process events from resumed process
	p.wg.Add(1)
	go func(wID string) {
		defer p.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				log.Error(log.CatOrch, "Worker panic recovered",
					"subsystem", "pool",
					"panic", r,
					"workerID", wID,
					"stack", string(debug.Stack()))
			}
		}()
		worker.resume(p.ctx, proc, p.broker)
	}(workerID)

	return nil
}

// EmitIncomingMessage emits an event indicating a message was sent to a worker.
// This is used to display coordinator messages in the worker pane.
func (p *WorkerPool) EmitIncomingMessage(workerID, taskID, message string) {
	if p.closed.Load() {
		return
	}

	p.broker.Publish(pubsub.UpdatedEvent, WorkerEvent{
		WorkerID: workerID,
		TaskID:   taskID,
		Type:     events.WorkerIncoming,
		Message:  message,
	})
}

// CancelWorker terminates a specific worker.
func (p *WorkerPool) CancelWorker(workerID string) error {
	p.mu.RLock()
	worker, ok := p.workers[workerID]
	p.mu.RUnlock()

	if !ok {
		return fmt.Errorf("worker not found: %s", workerID)
	}

	log.Debug(log.CatOrch, "Cancelling worker", "subsystem", "pool", "workerID", workerID)
	return worker.Cancel()
}

// RetireAll retires all active workers.
func (p *WorkerPool) RetireAll() {
	p.mu.RLock()
	workers := make([]*Worker, 0, len(p.workers))
	for _, w := range p.workers {
		workers = append(workers, w)
	}
	p.mu.RUnlock()

	log.Debug(log.CatOrch, "Retiring all workers", "subsystem", "pool", "count", len(workers))
	for _, w := range workers {
		w.Retire()
	}
}

// Close shuts down the pool and all workers.
// After Close, no new workers can be spawned.
func (p *WorkerPool) Close() {
	if p.closed.Swap(true) {
		return // Already closed
	}

	log.Debug(log.CatOrch, "Closing worker pool", "subsystem", "pool")
	p.RetireAll()
	p.cancel()
	p.wg.Wait()
	p.broker.Close()
}

// GetWorker returns the worker with the given ID, or nil if not found.
func (p *WorkerPool) GetWorker(workerID string) *Worker {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.workers[workerID]
}

// ActiveWorkers returns all workers that are not in a terminal state.
func (p *WorkerPool) ActiveWorkers() []*Worker {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var active []*Worker
	for _, w := range p.workers {
		if !w.GetStatus().IsDone() {
			active = append(active, w)
		}
	}
	return active
}

// Broker returns the pub/sub broker for worker events.
// Subscribers can use this to receive events about worker spawns, status changes, and output.
func (p *WorkerPool) Broker() *pubsub.Broker[WorkerEvent] {
	return p.broker
}

// MaxWorkers returns the maximum number of concurrent workers.
func (p *WorkerPool) MaxWorkers() int {
	return p.maxWorkers
}

// Client returns the headless client used by this pool to spawn workers.
func (p *WorkerPool) Client() client.HeadlessClient {
	return p.client
}

// AssignTaskToWorker assigns a task to a ready worker.
// Returns an error if the worker is not ready or doesn't exist.
func (p *WorkerPool) AssignTaskToWorker(workerID, taskID string) error {
	p.mu.RLock()
	worker := p.workers[workerID]
	p.mu.RUnlock()

	if worker == nil {
		return fmt.Errorf("worker not found: %s", workerID)
	}

	if err := worker.AssignTask(taskID); err != nil {
		return err
	}

	// Publish status change event so TUI updates
	p.broker.Publish(pubsub.UpdatedEvent, WorkerEvent{
		WorkerID: workerID,
		TaskID:   taskID,
		Type:     events.WorkerStatusChange,
		Status:   WorkerWorking,
	})

	return nil
}

// SetWorkerPhase updates a worker's phase without affecting the task ID.
// This allows the coordinator to sync workflow phase transitions to the pool
// for TUI display while preserving the task association.
// Returns an error if the worker doesn't exist.
func (p *WorkerPool) SetWorkerPhase(workerID string, phase events.WorkerPhase) error {
	p.mu.RLock()
	worker := p.workers[workerID]
	p.mu.RUnlock()

	if worker == nil {
		return fmt.Errorf("worker not found: %s", workerID)
	}

	worker.SetPhase(phase)
	return nil
}

// SetWorkerTaskID updates a worker's task ID.
// This allows the coordinator to set task ID on a worker (e.g., when assigning a reviewer).
// Returns an error if the worker doesn't exist.
func (p *WorkerPool) SetWorkerTaskID(workerID string, taskID string) error {
	p.mu.RLock()
	worker := p.workers[workerID]
	p.mu.RUnlock()

	if worker == nil {
		return fmt.Errorf("worker not found: %s", workerID)
	}

	worker.SetTaskID(taskID)
	return nil
}

// RetireWorker retires a worker and removes it from the active pool.
// Returns an error if the worker doesn't exist.
func (p *WorkerPool) RetireWorker(workerID string) error {
	p.mu.RLock()
	worker := p.workers[workerID]
	p.mu.RUnlock()

	if worker == nil {
		return fmt.Errorf("worker not found: %s", workerID)
	}

	// Cancel any running process and retire
	_ = worker.Cancel() // Cancel also sets status to Retired

	// Publish status change event so TUI is notified
	p.broker.Publish(pubsub.UpdatedEvent, WorkerEvent{
		WorkerID: workerID,
		Type:     events.WorkerStatusChange,
		Status:   WorkerRetired,
	})

	log.Debug(log.CatOrch, "Retired worker", "subsystem", "pool", "workerID", workerID)
	return nil
}

// ReadyWorkers returns all workers in Ready state.
func (p *WorkerPool) ReadyWorkers() []*Worker {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var ready []*Worker
	for _, w := range p.workers {
		if w.GetStatus() == WorkerReady {
			ready = append(ready, w)
		}
	}
	return ready
}

// AddTestWorker adds a worker directly to the pool for testing purposes.
// This bypasses the normal spawn process and should only be used in tests.
// The worker is created in WorkerWorking status with the given ID.
func (p *WorkerPool) AddTestWorker(id string, status WorkerStatus) *Worker {
	p.mu.Lock()
	defer p.mu.Unlock()

	worker := newWorker(id, 100)
	worker.Status = status
	p.workers[id] = worker
	return worker
}
