// Package repository provides domain entity definitions and repository interfaces
// for the v2 orchestration architecture.
package repository

import (
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/pubsub"
)

// ===========================================================================
// MemoryMessageRepository
// ===========================================================================

// Compile-time interface assertion
var _ MessageRepository = (*MemoryMessageRepository)(nil)

// MemoryMessageRepository is an in-memory implementation of MessageRepository.
// It is thread-safe using sync.RWMutex for concurrent access.
// This is a native implementation that maintains its own internal state.
type MemoryMessageRepository struct {
	mu        sync.RWMutex
	entries   []Message
	readState map[string]int // agent -> last read index
	broker    *pubsub.Broker[message.Event]
}

// NewMemoryMessageRepository creates a new in-memory message repository.
// The broker is created in the constructor and is never nil.
func NewMemoryMessageRepository() *MemoryMessageRepository {
	return &MemoryMessageRepository{
		entries:   make([]Message, 0),
		readState: make(map[string]int),
		broker:    pubsub.NewBroker[message.Event](),
	}
}

// Append adds a new message to the log.
// Returns the created message with generated UUID and timestamp.
// Automatically marks the sender as having read the message.
// Publishes an event to the broker for real-time TUI updates.
func (r *MemoryMessageRepository) Append(from, to, content string, msgType message.MessageType) (*Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry := Message{
		ID:        uuid.New().String(),
		Timestamp: time.Now(),
		From:      from,
		To:        to,
		Content:   content,
		Type:      msgType,
		ReadBy:    []string{from}, // Sender has implicitly read their own message
	}

	r.entries = append(r.entries, entry)

	// Publish event to all subscribers
	r.broker.Publish(pubsub.UpdatedEvent, message.Event{
		Type:  message.EventPosted,
		Entry: entry,
	})

	log.Debug(log.CatBeads, "Message appended",
		"from", from,
		"to", to,
		"type", msgType)

	return &entry, nil
}

// Entries returns a copy of all messages in the log, ordered by timestamp.
// The returned slice is safe to modify without affecting the repository.
func (r *MemoryMessageRepository) Entries() []Message {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Message, len(r.entries))
	copy(result, r.entries)
	return result
}

// UnreadFor returns all messages that the given agent hasn't read yet.
// Does not modify read state - call MarkRead to update.
// Note: All agents see all messages regardless of the To field (broadcast semantics).
func (r *MemoryMessageRepository) UnreadFor(agentID string) []Message {
	r.mu.RLock()
	defer r.mu.RUnlock()

	lastRead := r.readState[agentID]
	if lastRead >= len(r.entries) {
		return nil
	}

	// Return all unread entries (no recipient filtering)
	unread := make([]Message, len(r.entries)-lastRead)
	copy(unread, r.entries[lastRead:])
	return unread
}

// MarkRead marks all current messages as read by the given agent.
// Updates the read state index and adds the agent to ReadBy on all entries.
func (r *MemoryMessageRepository) MarkRead(agentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.readState[agentID] = len(r.entries)

	// Also update ReadBy on individual entries
	for i := range r.entries {
		if !slices.Contains(r.entries[i].ReadBy, agentID) {
			r.entries[i].ReadBy = append(r.entries[i].ReadBy, agentID)
		}
	}
}

// Count returns the total number of messages in the log.
func (r *MemoryMessageRepository) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.entries)
}

// Broker returns the pub/sub broker for subscribing to message events.
// The broker emits message.Event payloads when messages are appended.
func (r *MemoryMessageRepository) Broker() *pubsub.Broker[message.Event] {
	return r.broker
}

// AppendRestored adds a message with existing ID and timestamp (for session restoration).
// Unlike Append(), this preserves the entry's existing fields.
//
// CRITICAL: Does NOT publish to broker - TUI state is restored separately.
// Publishing would cause duplicate display since messagePane.entries is already set.
func (r *MemoryMessageRepository) AppendRestored(entry Message) (*Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.entries = append(r.entries, entry)

	return &entry, nil
}

// ===========================================================================
// Test Helpers
// ===========================================================================

// Reset clears all state from the repository. Useful for test setup/teardown.
// Creates fresh internal state with a new broker.
func (r *MemoryMessageRepository) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.entries = make([]Message, 0)
	r.readState = make(map[string]int)
	r.broker = pubsub.NewBroker[message.Event]()
}

// AddMessage adds a message to the repository for test setup.
// This is a convenience method that uses Append internally.
func (r *MemoryMessageRepository) AddMessage(from, to, content string, msgType message.MessageType) (*Message, error) {
	return r.Append(from, to, content, msgType)
}
