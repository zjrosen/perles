package orchestration

import (
	"sync"
	"time"
)

// NudgeBatcher accumulates worker message notifications and sends
// a single consolidated nudge after a debounce window.
type NudgeBatcher struct {
	debounce time.Duration
	senders  map[string]struct{} // Unique worker IDs
	timer    *time.Timer
	mu       sync.Mutex

	// Callback to send the nudge (set by TUI)
	onNudge func(workerIDs []string)
}

// NewNudgeBatcher creates a batcher with the given debounce duration.
func NewNudgeBatcher(debounce time.Duration) *NudgeBatcher {
	return &NudgeBatcher{
		debounce: debounce,
		senders:  make(map[string]struct{}),
	}
}

// SetOnNudge sets the callback invoked when debounce completes.
func (nb *NudgeBatcher) SetOnNudge(fn func(workerIDs []string)) {
	nb.mu.Lock()
	defer nb.mu.Unlock()
	nb.onNudge = fn
}

// Add records a worker that sent a message and resets the debounce timer.
func (nb *NudgeBatcher) Add(workerID string) {
	nb.mu.Lock()
	defer nb.mu.Unlock()

	// Add to pending senders
	nb.senders[workerID] = struct{}{}

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

	// Collect worker IDs
	workerIDs := make([]string, 0, len(nb.senders))
	for id := range nb.senders {
		workerIDs = append(workerIDs, id)
	}

	// Clear state
	nb.senders = make(map[string]struct{})
	nb.timer = nil

	// Invoke callback
	if nb.onNudge != nil {
		nb.onNudge(workerIDs)
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
	nb.senders = make(map[string]struct{})
}
