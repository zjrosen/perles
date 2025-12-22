package message

import (
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"perles/internal/log"
	"perles/internal/pubsub"
)

// Issue manages an in-memory message log for orchestration.
// It provides thread-safe methods for reading and appending messages.
type Issue struct {
	// entries is the in-memory list of messages
	entries []Entry

	// readState tracks which agents have read up to which message
	readState map[string]int // agent -> last read index

	mu sync.RWMutex

	// broker is the pub/sub broker for message events
	broker *pubsub.Broker[Event]
}

// New creates a new in-memory message log.
func New() *Issue {
	return &Issue{
		entries:   make([]Entry, 0),
		readState: make(map[string]int),
		broker:    pubsub.NewBroker[Event](),
	}
}

// Broker returns the pub/sub broker for subscribing to message events.
func (mi *Issue) Broker() *pubsub.Broker[Event] {
	return mi.broker
}

// Append adds a new message entry to the log.
func (mi *Issue) Append(from, to, content string, msgType MessageType) (*Entry, error) {
	mi.mu.Lock()
	defer mi.mu.Unlock()

	entry := Entry{
		ID:        uuid.New().String(),
		Timestamp: time.Now(),
		From:      from,
		To:        to,
		Content:   content,
		Type:      msgType,
		ReadBy:    []string{from}, // Sender has implicitly read their own message
	}

	mi.entries = append(mi.entries, entry)

	// Publish event to all subscribers
	mi.broker.Publish(pubsub.UpdatedEvent, Event{
		Type:  EventPosted,
		Entry: entry,
	})

	log.Debug(log.CatBeads, "Message appended",
		"from", from,
		"to", to,
		"type", msgType)

	return &entry, nil
}

// Entries returns a copy of all message entries.
func (mi *Issue) Entries() []Entry {
	mi.mu.RLock()
	defer mi.mu.RUnlock()

	result := make([]Entry, len(mi.entries))
	copy(result, mi.entries)
	return result
}

// UnreadFor returns all messages that the given agent hasn't read yet.
func (mi *Issue) UnreadFor(agentID string) []Entry {
	mi.mu.RLock()
	defer mi.mu.RUnlock()

	lastRead := mi.readState[agentID]
	if lastRead >= len(mi.entries) {
		return nil
	}

	// Return all unread entries (no recipient filtering)
	unread := make([]Entry, len(mi.entries)-lastRead)
	copy(unread, mi.entries[lastRead:])
	return unread
}

// MarkRead marks all messages up to now as read by the given agent.
func (mi *Issue) MarkRead(agentID string) {
	mi.mu.Lock()
	defer mi.mu.Unlock()

	mi.readState[agentID] = len(mi.entries)

	// Also update ReadBy on individual entries
	for i := range mi.entries {
		if !slices.Contains(mi.entries[i].ReadBy, agentID) {
			mi.entries[i].ReadBy = append(mi.entries[i].ReadBy, agentID)
		}
	}
}

// Count returns the total number of messages.
func (mi *Issue) Count() int {
	mi.mu.RLock()
	defer mi.mu.RUnlock()
	return len(mi.entries)
}
