package orchestration

import (
	"sync"
	"time"
)

// MessageType represents the type of worker message.
type MessageType int

const (
	// WorkerReady indicates the worker is ready for task assignment.
	WorkerReady MessageType = iota
	// WorkerNewMessage indicates a regular worker message.
	WorkerNewMessage
)

// NudgeBatcher accumulates worker message notifications and sends
// a single consolidated nudge after a debounce window.
type NudgeBatcher struct {
	debounce time.Duration
	senders  map[string]MessageType // Worker IDs to their message types
	timer    *time.Timer
	mu       sync.Mutex

	// Callback to send the nudge (set by TUI)
	// Map groups worker IDs by their message type
	onNudge func(messagesByType map[MessageType][]string)
}

// NewNudgeBatcher creates a batcher with the given debounce duration.
func NewNudgeBatcher(debounce time.Duration) *NudgeBatcher {
	return &NudgeBatcher{
		debounce: debounce,
		senders:  make(map[string]MessageType),
	}
}

// SetOnNudge sets the callback invoked when debounce completes.
// The callback receives a map grouping worker IDs by their message type.
func (nb *NudgeBatcher) SetOnNudge(fn func(messagesByType map[MessageType][]string)) {
	nb.mu.Lock()
	defer nb.mu.Unlock()
	nb.onNudge = fn
}

// Add records a worker that sent a message and resets the debounce timer.
// If a worker sends multiple messages with different types, the most recent type wins.
func (nb *NudgeBatcher) Add(workerID string, msgType MessageType) {
	nb.mu.Lock()
	defer nb.mu.Unlock()

	// Add to pending senders (or update type if already exists)
	nb.senders[workerID] = msgType

	// Reset or start timer
	if nb.timer != nil {
		nb.timer.Stop()
	}
	nb.timer = time.AfterFunc(nb.debounce, nb.flush)
}

// flush sends the consolidated nudge and clears state.
func (nb *NudgeBatcher) flush() {
	nb.mu.Lock()
	defer nb.mu.Unlock()

	if len(nb.senders) == 0 {
		return
	}

	// Group worker IDs by message type
	messagesByType := make(map[MessageType][]string)
	for workerID, msgType := range nb.senders {
		messagesByType[msgType] = append(messagesByType[msgType], workerID)
	}

	// Clear state
	nb.senders = make(map[string]MessageType)
	nb.timer = nil

	// Invoke callback
	if nb.onNudge != nil {
		nb.onNudge(messagesByType)
	}
}

// Stop cancels any pending nudge.
func (nb *NudgeBatcher) Stop() {
	nb.mu.Lock()
	defer nb.mu.Unlock()

	if nb.timer != nil {
		nb.timer.Stop()
		nb.timer = nil
	}
	nb.senders = make(map[string]MessageType)
}
