// Package nudger provides coordinator notification batching for the orchestration layer.
// It accumulates worker message notifications and sends consolidated nudges to the
// coordinator after a debounce window, preventing message floods when multiple workers
// send messages in quick succession.
package nudger

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/process"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
	"github.com/zjrosen/perles/internal/pubsub"
)

// DefaultDebounce is the default debounce duration for batching nudges.
const DefaultDebounce = 1 * time.Second

// MessageType represents the type of worker message for nudge formatting.
type MessageType int

const (
	// WorkerReady indicates the worker is ready for task assignment.
	WorkerReady MessageType = iota
	// WorkerNewMessage indicates a regular worker message.
	WorkerNewMessage
)

// Clock provides time-related operations for testability.
// Use RealClock for production and mockClock for testing.
type Clock interface {
	// Now returns the current time.
	Now() time.Time
	// NewTimer creates a new Timer that will send the current time
	// on its channel after at least duration d.
	NewTimer(d time.Duration) Timer
}

// Timer represents a timer that can be stopped and provides a channel.
type Timer interface {
	// Stop prevents the Timer from firing. Returns true if the call stops
	// the timer, false if the timer has already expired or been stopped.
	Stop() bool
	// C returns the channel on which the time is delivered.
	C() <-chan time.Time
}

// RealClock implements Clock using the standard time package.
type RealClock struct{}

// Now returns the current time.
func (RealClock) Now() time.Time { return time.Now() }

// NewTimer creates a new time.Timer.
func (RealClock) NewTimer(d time.Duration) Timer {
	return &realTimer{timer: time.NewTimer(d)}
}

// realTimer wraps time.Timer to implement the Timer interface.
type realTimer struct {
	timer *time.Timer
}

func (t *realTimer) Stop() bool          { return t.timer.Stop() }
func (t *realTimer) C() <-chan time.Time { return t.timer.C }

// CoordinatorNudger accumulates worker message notifications and sends
// consolidated nudges to the coordinator after a debounce window.
// It subscribes directly to the MessageRepository broker and submits
// commands via CommandSubmitter.
type CoordinatorNudger struct {
	debounce     time.Duration
	clock        Clock
	msgBroker    *pubsub.Broker[message.Event]
	cmdSubmitter process.CommandSubmitter

	mu      sync.Mutex
	senders map[string]MessageType // Worker IDs to their message types
	timer   Timer

	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	closeOnce sync.Once
}

// Config holds configuration for creating a CoordinatorNudger.
type Config struct {
	// Debounce is the duration to wait before sending a batched nudge.
	// Defaults to DefaultDebounce (1 second) if zero.
	Debounce time.Duration
	// MsgBroker is the message broker to subscribe to for message events.
	// Required.
	MsgBroker *pubsub.Broker[message.Event]
	// CmdSubmitter is used to submit commands to the coordinator.
	// Required.
	CmdSubmitter process.CommandSubmitter
	// Clock provides time operations. Defaults to RealClock if nil.
	Clock Clock
}

// New creates a new CoordinatorNudger with the given configuration.
func New(cfg Config) *CoordinatorNudger {
	debounce := cfg.Debounce
	if debounce == 0 {
		debounce = DefaultDebounce
	}

	clock := cfg.Clock
	if clock == nil {
		clock = RealClock{}
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &CoordinatorNudger{
		debounce:     debounce,
		clock:        clock,
		msgBroker:    cfg.MsgBroker,
		cmdSubmitter: cfg.CmdSubmitter,
		senders:      make(map[string]MessageType),
		ctx:          ctx,
		cancel:       cancel,
		done:         make(chan struct{}),
	}
}

// Start begins listening to message events and batching nudges.
// Must be called after construction. Safe to call only once.
func (n *CoordinatorNudger) Start() {
	go n.loop()
}

// Stop terminates the nudger and releases resources.
// Blocks until the event loop has exited. Safe to call multiple times.
// Safe to call before Start() - will be a no-op.
func (n *CoordinatorNudger) Stop() {
	n.cancel()
	n.closeDone()
	<-n.done // Wait for loop to exit (or immediate if never started)

	n.mu.Lock()
	defer n.mu.Unlock()
	if n.timer != nil {
		n.timer.Stop()
		n.timer = nil
	}
	n.senders = make(map[string]MessageType)
}

// closeDone safely closes the done channel exactly once.
func (n *CoordinatorNudger) closeDone() {
	n.closeOnce.Do(func() {
		close(n.done)
	})
}

// loop is the main event loop that processes message events with debouncing.
// This follows the Watcher pattern: select-based event loop with timer channel.
func (n *CoordinatorNudger) loop() {
	defer n.closeDone()

	// Subscribe to message events
	var eventCh <-chan pubsub.Event[message.Event]
	if n.msgBroker != nil {
		eventCh = n.msgBroker.Subscribe(n.ctx)
	}

	for {
		// Get timer channel (nil if no timer)
		timerCh := n.timerChan()

		select {
		case event, ok := <-eventCh:
			if !ok {
				// Broker closed, flush any pending messages and exit
				n.flush()
				return
			}
			n.handleMessageEvent(event)

		case <-timerCh:
			n.flush()

		case <-n.ctx.Done():
			return
		}
	}
}

// timerChan returns the timer's channel, or nil if no timer is active.
func (n *CoordinatorNudger) timerChan() <-chan time.Time {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.timer != nil {
		return n.timer.C()
	}
	return nil
}

// handleMessageEvent processes a message event and potentially adds it to the batch.
func (n *CoordinatorNudger) handleMessageEvent(event pubsub.Event[message.Event]) {
	entry := event.Payload.Entry

	// Only nudge for messages TO coordinator (or ALL) FROM workers
	if entry.To != message.ActorCoordinator && entry.To != message.ActorAll {
		return
	}
	if entry.From == message.ActorCoordinator {
		return
	}

	// Determine message type
	msgType := WorkerNewMessage
	if entry.Type == message.MessageWorkerReady {
		msgType = WorkerReady
	}

	n.add(entry.From, msgType)
}

// add records a worker that sent a message and resets the debounce timer.
// If a worker sends multiple messages with different types, the most recent type wins.
func (n *CoordinatorNudger) add(workerID string, msgType MessageType) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Add to pending senders (or update type if already exists)
	n.senders[workerID] = msgType

	// Reset or start timer
	if n.timer != nil {
		n.timer.Stop()
	}
	n.timer = n.clock.NewTimer(n.debounce)
}

// flush sends the consolidated nudge and clears state.
func (n *CoordinatorNudger) flush() {
	n.mu.Lock()
	defer n.mu.Unlock()

	if len(n.senders) == 0 {
		return
	}

	// Group worker IDs by message type
	messagesByType := make(map[MessageType][]string)
	for workerID, msgType := range n.senders {
		messagesByType[msgType] = append(messagesByType[msgType], workerID)
	}

	// Sort worker IDs for deterministic output
	for msgType := range messagesByType {
		sort.Strings(messagesByType[msgType])
	}

	// Clear state
	n.senders = make(map[string]MessageType)
	n.timer = nil

	// Submit commands directly
	if n.cmdSubmitter == nil {
		return
	}

	if readyWorkers := messagesByType[WorkerReady]; len(readyWorkers) > 0 {
		nudge := fmt.Sprintf("[%s] have started up and are now ready", strings.Join(readyWorkers, ", "))
		cmd := command.NewSendToProcessCommand(command.SourceInternal, repository.CoordinatorID, nudge)
		n.cmdSubmitter.Submit(cmd)
	}

	if newMessageWorkers := messagesByType[WorkerNewMessage]; len(newMessageWorkers) > 0 {
		nudge := fmt.Sprintf("[%s sent messages] Use read_message_log to check for new messages.", strings.Join(newMessageWorkers, ", "))
		cmd := command.NewSendToProcessCommand(command.SourceInternal, repository.CoordinatorID, nudge)
		n.cmdSubmitter.Submit(cmd)
	}
}
