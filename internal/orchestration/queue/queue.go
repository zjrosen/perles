// Package queue provides a thread-safe FIFO message queue for orchestration.
package queue

import (
	"errors"
	"sync"
	"time"
)

// DefaultMaxSize is the default maximum number of messages a queue can hold.
const DefaultMaxSize = 100

// ErrQueueFull is returned when attempting to enqueue to a full queue.
var ErrQueueFull = errors.New("queue is full")

// QueuedMessage represents a message waiting to be delivered to a worker.
type QueuedMessage struct {
	ID         string    // Unique message identifier
	Content    string    // Message body
	From       string    // Sender (COORDINATOR, USER, worker ID)
	EnqueuedAt time.Time // For TTL/timeout handling
	Priority   int       // 0=normal, 1=urgent (unused in V1)
}

// MessageQueue is a thread-safe FIFO queue for pending worker messages.
type MessageQueue struct {
	entries []QueuedMessage
	mu      sync.Mutex
	maxSize int
}

// NewMessageQueue creates a new MessageQueue with the specified maximum size.
// If maxSize is <= 0, DefaultMaxSize (100) is used.
func NewMessageQueue(maxSize int) *MessageQueue {
	if maxSize <= 0 {
		maxSize = DefaultMaxSize
	}
	return &MessageQueue{
		entries: make([]QueuedMessage, 0),
		maxSize: maxSize,
	}
}

// Enqueue adds a message to the back of the queue.
// Returns ErrQueueFull if the queue is at maximum capacity.
func (q *MessageQueue) Enqueue(msg QueuedMessage) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.entries) >= q.maxSize {
		return ErrQueueFull
	}

	q.entries = append(q.entries, msg)
	return nil
}

// Dequeue removes and returns the message at the front of the queue.
// Returns (zero value, false) if the queue is empty.
func (q *MessageQueue) Dequeue() (QueuedMessage, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.entries) == 0 {
		return QueuedMessage{}, false
	}

	msg := q.entries[0]
	q.entries = q.entries[1:]
	return msg, true
}

// Len returns the current number of messages in the queue.
func (q *MessageQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	return len(q.entries)
}

// Peek returns the message at the front of the queue without removing it.
// Returns (zero value, false) if the queue is empty.
func (q *MessageQueue) Peek() (QueuedMessage, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.entries) == 0 {
		return QueuedMessage{}, false
	}

	return q.entries[0], true
}

// Drain removes and returns all messages from the queue, leaving it empty.
// Returns an empty slice if the queue was already empty.
func (q *MessageQueue) Drain() []QueuedMessage {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.entries) == 0 {
		return []QueuedMessage{}
	}

	// Return the current slice and reset to empty
	result := q.entries
	q.entries = make([]QueuedMessage, 0)
	return result
}
