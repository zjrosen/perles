// Package repository provides domain entity definitions and repository interfaces
// for the v2 orchestration architecture. This package defines the contracts for
// state management and the domain entities that handlers operate on.
package repository

import (
	"errors"
	"time"

	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/orchestration/metrics"
	"github.com/zjrosen/perles/internal/pubsub"
)

// ===========================================================================
// Error Sentinel Values
// ===========================================================================

// ErrTaskNotFound is returned when a task ID does not exist in the repository.
var ErrTaskNotFound = errors.New("task not found")

// ErrQueueFull is returned when attempting to enqueue to a full message queue.
var ErrQueueFull = errors.New("message queue is full")

// ErrProcessNotFound is returned when a process ID does not exist in the repository.
var ErrProcessNotFound = errors.New("process not found")

// ===========================================================================
// Process Constants and Types (Unified Coordinator/Worker Model)
// ===========================================================================

// CoordinatorID is the well-known ID for the coordinator process.
const CoordinatorID = "coordinator"

// ProcessRole identifies what kind of process this is.
// This is a type alias to events.ProcessRole to avoid duplicate definitions
// and eliminate conversion overhead between repository and event layers.
type ProcessRole = events.ProcessRole

// Role constants - aliases to events package for backward compatibility.
const (
	RoleCoordinator = events.RoleCoordinator
	RoleWorker      = events.RoleWorker
)

// ProcessStatus represents the process's current operational state.
// This is a type alias to events.ProcessStatus to avoid duplicate definitions
// and eliminate conversion overhead between repository and event layers.
type ProcessStatus = events.ProcessStatus

// Status constants - aliases to events package for backward compatibility.
const (
	StatusPending  = events.ProcessStatusPending
	StatusStarting = events.ProcessStatusStarting
	StatusReady    = events.ProcessStatusReady
	StatusWorking  = events.ProcessStatusWorking
	StatusPaused   = events.ProcessStatusPaused
	StatusStopped  = events.ProcessStatusStopped
	StatusRetired  = events.ProcessStatusRetired
	StatusFailed   = events.ProcessStatusFailed
)

// ===========================================================================
// Domain Entities
// ===========================================================================

// Process represents any AI process in the orchestration system.
// The coordinator is the process where Role == RoleCoordinator.
// This is the unified entity replacing separate Worker and Coordinator entities.
type Process struct {
	// ID is the unique identifier (e.g., "coordinator", "worker-1", "worker-2").
	ID string
	// Role identifies whether this is the coordinator or a worker.
	Role ProcessRole
	// Status is the process's current operational state.
	Status ProcessStatus
	// SessionID is the Claude/Amp session ID for resuming conversations.
	SessionID string
	// Metrics contains token usage and cost data.
	Metrics *metrics.TokenMetrics
	// CreatedAt is when this process was spawned.
	CreatedAt time.Time
	// LastActivityAt is when the process last completed a turn.
	LastActivityAt time.Time

	// Worker-specific fields (nil/empty for coordinator)

	// Phase is the worker's workflow phase (nil for coordinator).
	Phase *events.ProcessPhase
	// TaskID is the current task assigned (empty for coordinator).
	TaskID string
	// RetiredAt is when this process was retired (zero if still active).
	RetiredAt time.Time
}

// IsCoordinator returns true if this is the coordinator process.
func (p *Process) IsCoordinator() bool {
	return p.Role == RoleCoordinator
}

// IsWorker returns true if this is a worker process.
func (p *Process) IsWorker() bool {
	return p.Role == RoleWorker
}

// IsActive returns true if the process can receive messages.
// Only Ready and Working processes are active.
func (p *Process) IsActive() bool {
	return p.Status == StatusReady || p.Status == StatusWorking
}

// TaskStatus represents the status of a task assignment.
type TaskStatus string

const (
	// TaskImplementing means the task is being implemented.
	TaskImplementing TaskStatus = "implementing"
	// TaskInReview means the task is being reviewed.
	TaskInReview TaskStatus = "in_review"
	// TaskApproved means the task has been approved by the reviewer.
	TaskApproved TaskStatus = "approved"
	// TaskDenied means the task was denied by the reviewer.
	TaskDenied TaskStatus = "denied"
	// TaskCommitting means the task is in the commit phase.
	TaskCommitting TaskStatus = "committing"
	// TaskCompleted means the task is complete.
	TaskCompleted TaskStatus = "completed"
)

// TaskAssignment represents a task assigned to workers for implementation and review.
// This is the aggregate root for the Task bounded context.
type TaskAssignment struct {
	// TaskID is the bd task ID (e.g., "perles-abc1.2").
	TaskID string
	// Implementer is the worker ID assigned to implement this task.
	Implementer string
	// Reviewer is the worker ID assigned to review this task (empty if not yet assigned).
	Reviewer string
	// Status is the current status of this task assignment.
	Status TaskStatus
	// StartedAt is when implementation began.
	StartedAt time.Time
	// ReviewStartedAt is when review began (zero if not yet in review).
	ReviewStartedAt time.Time
}

// SenderType identifies who sent a message.
type SenderType string

const (
	// SenderUser indicates the message came from the user (TUI).
	SenderUser SenderType = "user"
	// SenderCoordinator indicates the message came from the coordinator process.
	SenderCoordinator SenderType = "coordinator"
	// SenderSystem indicates the message came from the orchestration system (e.g., enforcement reminders).
	SenderSystem SenderType = "system"
)

// QueueEntry represents a single message in a worker's message queue.
type QueueEntry struct {
	// Content is the message content.
	Content string
	// Sender identifies who sent this message (user or coordinator).
	Sender SenderType
	// Timestamp is when this entry was enqueued.
	Timestamp time.Time
}

// ===========================================================================
// MessageQueue Entity
// ===========================================================================

// MessageQueue is a domain entity representing a worker's message queue.
// The QueueRepository provides access to these entities.
// MessageQueue maintains FIFO ordering and bounded capacity.
type MessageQueue struct {
	// WorkerID identifies which worker this queue belongs to.
	WorkerID string
	// entries holds the queued messages in FIFO order.
	entries []QueueEntry
	// maxSize is the maximum number of entries allowed (0 means unlimited).
	maxSize int
}

// NewMessageQueue creates a new MessageQueue for a worker with the specified max size.
// A maxSize of 0 means unlimited capacity.
func NewMessageQueue(workerID string, maxSize int) *MessageQueue {
	return &MessageQueue{
		WorkerID: workerID,
		entries:  make([]QueueEntry, 0),
		maxSize:  maxSize,
	}
}

// Enqueue adds a message to the end of the queue with the specified sender.
// Returns ErrQueueFull if the queue has reached maxSize (and maxSize > 0).
func (q *MessageQueue) Enqueue(content string, sender SenderType) error {
	if q.maxSize > 0 && len(q.entries) >= q.maxSize {
		return ErrQueueFull
	}
	q.entries = append(q.entries, QueueEntry{
		Content:   content,
		Sender:    sender,
		Timestamp: time.Now(),
	})
	return nil
}

// Dequeue removes and returns the first message from the queue.
// Returns the entry and true if the queue had a message, or an empty entry and false if empty.
func (q *MessageQueue) Dequeue() (*QueueEntry, bool) {
	if len(q.entries) == 0 {
		return nil, false
	}
	entry := q.entries[0]
	q.entries = q.entries[1:]
	return &entry, true
}

// Drain removes and returns all messages from the queue, emptying it.
// Returns an empty slice if the queue was already empty.
func (q *MessageQueue) Drain() []QueueEntry {
	entries := q.entries
	q.entries = make([]QueueEntry, 0)
	return entries
}

// Size returns the current number of messages in the queue.
func (q *MessageQueue) Size() int {
	return len(q.entries)
}

// IsEmpty returns true if the queue has no messages.
func (q *MessageQueue) IsEmpty() bool {
	return len(q.entries) == 0
}

// MaxSize returns the maximum capacity of the queue (0 means unlimited).
func (q *MessageQueue) MaxSize() int {
	return q.maxSize
}

// ===========================================================================
// Repository Interfaces
// ===========================================================================

// TaskRepository provides aggregate access for TaskAssignment entities.
// Implementations must be thread-safe.
type TaskRepository interface {
	// Get retrieves a task assignment by task ID.
	// Returns ErrTaskNotFound if the task does not exist.
	Get(taskID string) (*TaskAssignment, error)

	// Save persists a task assignment. Creates new or updates existing.
	Save(task *TaskAssignment) error

	// GetByWorker retrieves the task currently assigned to a worker (as implementer or reviewer).
	// Returns ErrTaskNotFound if no task is assigned to the worker.
	GetByWorker(workerID string) (*TaskAssignment, error)

	// GetByImplementer retrieves all tasks where the worker is the implementer.
	GetByImplementer(workerID string) ([]*TaskAssignment, error)

	// All returns all task assignments in the repository.
	All() []*TaskAssignment

	// Delete removes a task assignment from the repository.
	Delete(taskID string) error
}

// QueueRepository provides aggregate access for MessageQueue entities.
// Implementations must be thread-safe.
type QueueRepository interface {
	// GetOrCreate retrieves or creates a message queue for a worker.
	// Never returns nil - creates a new queue if one doesn't exist.
	GetOrCreate(workerID string) *MessageQueue

	// Delete removes a worker's message queue from the repository.
	Delete(workerID string)

	// Size returns the number of messages in a worker's queue.
	// Returns 0 if the worker has no queue.
	Size(workerID string) int
}

// ProcessRepository provides aggregate access for Process entities.
// This is the unified repository for both coordinator and worker processes.
// Implementations must be thread-safe.
type ProcessRepository interface {
	// Get retrieves a process by ID.
	// Returns ErrProcessNotFound if the process does not exist.
	Get(processID string) (*Process, error)

	// Save persists a process. Creates new or updates existing.
	Save(process *Process) error

	// List returns all processes in the repository.
	List() []*Process

	// GetCoordinator returns the coordinator process.
	// Returns ErrProcessNotFound if coordinator hasn't been created.
	GetCoordinator() (*Process, error)

	// Workers returns all worker processes (excluding coordinator).
	Workers() []*Process

	// ActiveWorkers returns workers not in terminal state (not Retired or Failed).
	ActiveWorkers() []*Process

	// ReadyWorkers returns workers available for assignment (Ready status, Idle phase).
	ReadyWorkers() []*Process

	// RetiredWorkers returns workers in terminal state (Retired or Failed).
	RetiredWorkers() []*Process
}

// ===========================================================================
// Message Domain Entity
// ===========================================================================

// Message is a type alias for message.Entry, representing a single message
// in the inter-agent communication log. Using a type alias avoids conversion
// overhead while achieving repository-pattern ownership of the domain entity.
type Message = message.Entry

// ===========================================================================
// MessageRepository Interface
// ===========================================================================

// MessageRepository provides aggregate access for inter-agent messages.
// This is a log-style repository (append-only, ordered by timestamp).
// Implementations must be thread-safe using sync.RWMutex or equivalent.
type MessageRepository interface {
	// Append adds a new message to the log.
	// Returns the created message with generated UUID and timestamp.
	// Automatically marks the sender as having read the message.
	// Publishes an event to the broker for real-time TUI updates.
	Append(from, to, content string, msgType message.MessageType) (*Message, error)

	// Entries returns a copy of all messages in the log, ordered by timestamp.
	// The returned slice is safe to modify without affecting the repository.
	Entries() []Message

	// UnreadFor returns all messages that the given agent hasn't read yet.
	// Does not modify read state - call MarkRead to update.
	// Note: All agents see all messages regardless of the To field (broadcast semantics).
	UnreadFor(agentID string) []Message

	// MarkRead marks all current messages as read by the given agent.
	// Updates the read state index and adds the agent to ReadBy on all entries.
	MarkRead(agentID string)

	// Count returns the total number of messages in the log.
	Count() int

	// Broker returns the pub/sub broker for subscribing to message events.
	// The broker emits message.Event payloads when messages are appended.
	// Returns nil if pub/sub is not supported by this implementation.
	Broker() *pubsub.Broker[message.Event]
}
