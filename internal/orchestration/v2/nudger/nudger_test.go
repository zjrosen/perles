package nudger

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/pubsub"
)

// mockClock implements Clock for deterministic testing.
type mockClock struct {
	mu      sync.Mutex
	now     time.Time
	timers  []*mockTimer
	advance chan time.Duration
}

func newMockClock() *mockClock {
	return &mockClock{
		now:     time.Date(2026, 1, 17, 12, 0, 0, 0, time.UTC),
		advance: make(chan time.Duration, 10),
	}
}

func (c *mockClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *mockClock) NewTimer(d time.Duration) Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &mockTimer{
		deadline: c.now.Add(d),
		ch:       make(chan time.Time, 1),
		stopped:  false,
	}
	c.timers = append(c.timers, t)
	return t
}

// Advance moves time forward and fires any expired timers.
func (c *mockClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	now := c.now
	timers := c.timers
	c.mu.Unlock()

	// Fire expired timers outside the lock to avoid deadlock
	for _, t := range timers {
		t.mu.Lock()
		if !t.stopped && !t.fired && !t.deadline.After(now) {
			t.fired = true
			select {
			case t.ch <- now:
			default:
			}
		}
		t.mu.Unlock()
	}
}

type mockTimer struct {
	mu       sync.Mutex
	deadline time.Time
	ch       chan time.Time
	stopped  bool
	fired    bool
}

func (t *mockTimer) Stop() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	wasRunning := !t.stopped && !t.fired
	t.stopped = true
	return wasRunning
}

func (t *mockTimer) C() <-chan time.Time {
	return t.ch
}

// mockCommandSubmitter captures submitted commands.
type mockCommandSubmitter struct {
	mu       sync.Mutex
	commands []command.Command
	notify   chan struct{}
}

func newMockSubmitter() *mockCommandSubmitter {
	return &mockCommandSubmitter{
		commands: make([]command.Command, 0),
		notify:   make(chan struct{}, 10),
	}
}

func (m *mockCommandSubmitter) Submit(cmd command.Command) {
	m.mu.Lock()
	m.commands = append(m.commands, cmd)
	m.mu.Unlock()
	select {
	case m.notify <- struct{}{}:
	default:
	}
}

func (m *mockCommandSubmitter) Commands() []command.Command {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]command.Command, len(m.commands))
	copy(result, m.commands)
	return result
}

func (m *mockCommandSubmitter) WaitForCommands(n int, timeout time.Duration) bool {
	deadline := time.After(timeout)
	for {
		m.mu.Lock()
		count := len(m.commands)
		m.mu.Unlock()
		if count >= n {
			return true
		}
		select {
		case <-m.notify:
		case <-deadline:
			return false
		}
	}
}

// Helper to post a message event to the broker
func postMessage(broker *pubsub.Broker[message.Event], from, to string, msgType message.MessageType) {
	broker.Publish(pubsub.UpdatedEvent, message.Event{
		Type: message.EventPosted,
		Entry: message.Entry{
			ID:        "test-msg",
			Timestamp: time.Now(),
			From:      from,
			To:        to,
			Content:   "test message",
			Type:      msgType,
		},
	})
}

// TestCoordinatorNudger_New verifies constructor creates instance with defaults.
func TestCoordinatorNudger_New(t *testing.T) {
	broker := pubsub.NewBroker[message.Event]()
	submitter := newMockSubmitter()

	nudger := New(Config{
		MsgBroker:    broker,
		CmdSubmitter: submitter,
	})

	require.NotNil(t, nudger)
	require.Equal(t, DefaultDebounce, nudger.debounce)
	require.NotNil(t, nudger.clock)
	require.NotNil(t, nudger.senders)
}

// TestCoordinatorNudger_SingleMessage verifies a single message is delivered after debounce.
func TestCoordinatorNudger_SingleMessage(t *testing.T) {
	broker := pubsub.NewBroker[message.Event]()
	submitter := newMockSubmitter()
	clock := newMockClock()

	nudger := New(Config{
		Debounce:     100 * time.Millisecond,
		MsgBroker:    broker,
		CmdSubmitter: submitter,
		Clock:        clock,
	})
	nudger.Start()
	defer nudger.Stop()

	// Allow goroutine to start and subscribe
	time.Sleep(10 * time.Millisecond)

	// Post a message from worker to coordinator
	postMessage(broker, "WORKER.1", message.ActorCoordinator, message.MessageInfo)

	// Allow event to be processed
	time.Sleep(10 * time.Millisecond)

	// Should not fire immediately
	require.Empty(t, submitter.Commands())

	// Advance past debounce
	clock.Advance(100 * time.Millisecond)

	// Wait for command submission
	require.True(t, submitter.WaitForCommands(1, 100*time.Millisecond))

	cmds := submitter.Commands()
	require.Len(t, cmds, 1)
	sendCmd, ok := cmds[0].(*command.SendToProcessCommand)
	require.True(t, ok)
	require.Contains(t, sendCmd.Content, "WORKER.1")
	require.Contains(t, sendCmd.Content, "sent messages")
}

// TestCoordinatorNudger_MultipleMessagesBatched verifies multiple workers are batched.
func TestCoordinatorNudger_MultipleMessagesBatched(t *testing.T) {
	broker := pubsub.NewBroker[message.Event]()
	submitter := newMockSubmitter()
	clock := newMockClock()

	nudger := New(Config{
		Debounce:     100 * time.Millisecond,
		MsgBroker:    broker,
		CmdSubmitter: submitter,
		Clock:        clock,
	})
	nudger.Start()
	defer nudger.Stop()

	// Allow goroutine to start
	time.Sleep(10 * time.Millisecond)

	// Post messages from multiple workers with different types
	postMessage(broker, "WORKER.1", message.ActorCoordinator, message.MessageInfo)
	postMessage(broker, "WORKER.2", message.ActorCoordinator, message.MessageWorkerReady)
	postMessage(broker, "WORKER.3", message.ActorCoordinator, message.MessageInfo)

	// Allow events to be processed
	time.Sleep(10 * time.Millisecond)

	// Advance past debounce
	clock.Advance(100 * time.Millisecond)

	// Wait for commands (should be 2: one for ready, one for new messages)
	require.True(t, submitter.WaitForCommands(2, 100*time.Millisecond))

	cmds := submitter.Commands()
	require.Len(t, cmds, 2)

	// Check that both message types are present
	var hasReady, hasNewMessage bool
	for _, cmd := range cmds {
		sendCmd := cmd.(*command.SendToProcessCommand)
		if strings.Contains(sendCmd.Content, "have started up") {
			hasReady = true
			require.Contains(t, sendCmd.Content, "WORKER.2")
		}
		if strings.Contains(sendCmd.Content, "sent messages") {
			hasNewMessage = true
			require.Contains(t, sendCmd.Content, "WORKER.1")
			require.Contains(t, sendCmd.Content, "WORKER.3")
		}
	}
	require.True(t, hasReady, "should have ready message")
	require.True(t, hasNewMessage, "should have new message notification")
}

// TestCoordinatorNudger_SlidingWindow verifies timer resets on new message.
func TestCoordinatorNudger_SlidingWindow(t *testing.T) {
	broker := pubsub.NewBroker[message.Event]()
	submitter := newMockSubmitter()
	clock := newMockClock()

	nudger := New(Config{
		Debounce:     100 * time.Millisecond,
		MsgBroker:    broker,
		CmdSubmitter: submitter,
		Clock:        clock,
	})
	nudger.Start()
	defer nudger.Stop()

	// Allow goroutine to start
	time.Sleep(10 * time.Millisecond)

	// Post first message
	postMessage(broker, "WORKER.1", message.ActorCoordinator, message.MessageInfo)
	time.Sleep(5 * time.Millisecond)

	// Advance 50ms (not enough to trigger)
	clock.Advance(50 * time.Millisecond)
	time.Sleep(5 * time.Millisecond)

	// Post second message - this should reset timer
	postMessage(broker, "WORKER.2", message.ActorCoordinator, message.MessageInfo)
	time.Sleep(5 * time.Millisecond)

	// Advance another 50ms (still not enough since timer reset)
	clock.Advance(50 * time.Millisecond)
	time.Sleep(5 * time.Millisecond)

	// Should still be empty
	require.Empty(t, submitter.Commands())

	// Post third message
	postMessage(broker, "WORKER.3", message.ActorCoordinator, message.MessageInfo)
	time.Sleep(5 * time.Millisecond)

	// Now advance past debounce from last message
	clock.Advance(100 * time.Millisecond)

	// Wait for command
	require.True(t, submitter.WaitForCommands(1, 100*time.Millisecond))

	// Should be exactly one command with all three workers
	cmds := submitter.Commands()
	require.Len(t, cmds, 1)
	sendCmd := cmds[0].(*command.SendToProcessCommand)
	require.Contains(t, sendCmd.Content, "WORKER.1")
	require.Contains(t, sendCmd.Content, "WORKER.2")
	require.Contains(t, sendCmd.Content, "WORKER.3")
}

// TestCoordinatorNudger_DuplicateSenders verifies duplicates are deduplicated.
func TestCoordinatorNudger_DuplicateSenders(t *testing.T) {
	broker := pubsub.NewBroker[message.Event]()
	submitter := newMockSubmitter()
	clock := newMockClock()

	nudger := New(Config{
		Debounce:     100 * time.Millisecond,
		MsgBroker:    broker,
		CmdSubmitter: submitter,
		Clock:        clock,
	})
	nudger.Start()
	defer nudger.Stop()

	// Allow goroutine to start
	time.Sleep(10 * time.Millisecond)

	// Post same worker multiple times
	postMessage(broker, "WORKER.1", message.ActorCoordinator, message.MessageInfo)
	postMessage(broker, "WORKER.1", message.ActorCoordinator, message.MessageInfo)
	postMessage(broker, "WORKER.1", message.ActorCoordinator, message.MessageInfo)

	// Allow events to be processed
	time.Sleep(10 * time.Millisecond)

	// Advance past debounce
	clock.Advance(100 * time.Millisecond)

	// Wait for command
	require.True(t, submitter.WaitForCommands(1, 100*time.Millisecond))

	cmds := submitter.Commands()
	require.Len(t, cmds, 1)
	sendCmd := cmds[0].(*command.SendToProcessCommand)
	// WORKER.1 should appear exactly once
	count := strings.Count(sendCmd.Content, "WORKER.1")
	require.Equal(t, 1, count)
}

// TestCoordinatorNudger_StopCancelsPending verifies Stop cancels pending nudges.
func TestCoordinatorNudger_StopCancelsPending(t *testing.T) {
	broker := pubsub.NewBroker[message.Event]()
	submitter := newMockSubmitter()
	clock := newMockClock()

	nudger := New(Config{
		Debounce:     100 * time.Millisecond,
		MsgBroker:    broker,
		CmdSubmitter: submitter,
		Clock:        clock,
	})
	nudger.Start()

	// Allow goroutine to start
	time.Sleep(10 * time.Millisecond)

	// Post a message
	postMessage(broker, "WORKER.1", message.ActorCoordinator, message.MessageInfo)

	// Allow event to be processed
	time.Sleep(10 * time.Millisecond)

	// Stop before debounce fires
	nudger.Stop()

	// Advance past debounce
	clock.Advance(200 * time.Millisecond)

	// Give time for any errant commands
	time.Sleep(20 * time.Millisecond)

	// Should have no commands
	require.Empty(t, submitter.Commands())
}

// TestCoordinatorNudger_NoFlushIfNoMessages verifies no nudge without messages.
func TestCoordinatorNudger_NoFlushIfNoMessages(t *testing.T) {
	broker := pubsub.NewBroker[message.Event]()
	submitter := newMockSubmitter()
	clock := newMockClock()

	nudger := New(Config{
		Debounce:     50 * time.Millisecond,
		MsgBroker:    broker,
		CmdSubmitter: submitter,
		Clock:        clock,
	})
	nudger.Start()
	defer nudger.Stop()

	// Allow goroutine to start
	time.Sleep(10 * time.Millisecond)

	// Don't add any messages

	// Advance past debounce window
	clock.Advance(100 * time.Millisecond)

	// Give time for any errant commands
	time.Sleep(20 * time.Millisecond)

	// Should have no commands
	require.Empty(t, submitter.Commands())
}

// TestCoordinatorNudger_ConcurrentAdds verifies thread-safety of concurrent adds.
func TestCoordinatorNudger_ConcurrentAdds(t *testing.T) {
	broker := pubsub.NewBroker[message.Event]()
	submitter := newMockSubmitter()
	clock := newMockClock()

	nudger := New(Config{
		Debounce:     100 * time.Millisecond,
		MsgBroker:    broker,
		CmdSubmitter: submitter,
		Clock:        clock,
	})
	nudger.Start()
	defer nudger.Stop()

	// Allow goroutine to start
	time.Sleep(10 * time.Millisecond)

	// Concurrently post from multiple goroutines
	var wg sync.WaitGroup
	for i := 1; i <= 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			postMessage(broker, message.WorkerID(id), message.ActorCoordinator, message.MessageInfo)
		}(i)
	}
	wg.Wait()

	// Allow events to be processed
	time.Sleep(20 * time.Millisecond)

	// Advance past debounce
	clock.Advance(100 * time.Millisecond)

	// Wait for command
	require.True(t, submitter.WaitForCommands(1, 100*time.Millisecond))

	cmds := submitter.Commands()
	require.Len(t, cmds, 1)
	sendCmd := cmds[0].(*command.SendToProcessCommand)

	// All 5 workers should be present
	for i := 1; i <= 5; i++ {
		require.Contains(t, sendCmd.Content, message.WorkerID(i))
	}
}

// TestCoordinatorNudger_MessageTypesTracked verifies correct grouping by type.
func TestCoordinatorNudger_MessageTypesTracked(t *testing.T) {
	broker := pubsub.NewBroker[message.Event]()
	submitter := newMockSubmitter()
	clock := newMockClock()

	nudger := New(Config{
		Debounce:     100 * time.Millisecond,
		MsgBroker:    broker,
		CmdSubmitter: submitter,
		Clock:        clock,
	})
	nudger.Start()
	defer nudger.Stop()

	// Allow goroutine to start
	time.Sleep(10 * time.Millisecond)

	// Add messages with different types
	postMessage(broker, "WORKER.1", message.ActorCoordinator, message.MessageWorkerReady)
	postMessage(broker, "WORKER.2", message.ActorCoordinator, message.MessageInfo)

	// Allow events to be processed
	time.Sleep(10 * time.Millisecond)

	// Advance past debounce
	clock.Advance(100 * time.Millisecond)

	// Wait for commands
	require.True(t, submitter.WaitForCommands(2, 100*time.Millisecond))

	cmds := submitter.Commands()
	require.Len(t, cmds, 2)

	// Verify each type is in a separate command
	var readyCmd, messageCmd *command.SendToProcessCommand
	for _, cmd := range cmds {
		sendCmd := cmd.(*command.SendToProcessCommand)
		if strings.Contains(sendCmd.Content, "have started up") {
			readyCmd = sendCmd
		} else if strings.Contains(sendCmd.Content, "sent messages") {
			messageCmd = sendCmd
		}
	}

	require.NotNil(t, readyCmd)
	require.NotNil(t, messageCmd)
	require.Contains(t, readyCmd.Content, "WORKER.1")
	require.Contains(t, messageCmd.Content, "WORKER.2")
}

// TestCoordinatorNudger_LastTypeWins verifies last type wins for same sender.
func TestCoordinatorNudger_LastTypeWins(t *testing.T) {
	broker := pubsub.NewBroker[message.Event]()
	submitter := newMockSubmitter()
	clock := newMockClock()

	nudger := New(Config{
		Debounce:     100 * time.Millisecond,
		MsgBroker:    broker,
		CmdSubmitter: submitter,
		Clock:        clock,
	})
	nudger.Start()
	defer nudger.Stop()

	// Allow goroutine to start
	time.Sleep(10 * time.Millisecond)

	// Same worker sends different types - last one should win
	postMessage(broker, "WORKER.1", message.ActorCoordinator, message.MessageWorkerReady)
	time.Sleep(5 * time.Millisecond)
	postMessage(broker, "WORKER.1", message.ActorCoordinator, message.MessageInfo)

	// Allow events to be processed
	time.Sleep(10 * time.Millisecond)

	// Advance past debounce
	clock.Advance(100 * time.Millisecond)

	// Wait for command
	require.True(t, submitter.WaitForCommands(1, 100*time.Millisecond))

	cmds := submitter.Commands()
	require.Len(t, cmds, 1)
	sendCmd := cmds[0].(*command.SendToProcessCommand)

	// Should be in the new message group, not ready group
	require.Contains(t, sendCmd.Content, "sent messages")
	require.NotContains(t, sendCmd.Content, "have started up")
}

// TestCoordinatorNudger_StopDuringFlush verifies no deadlock if Stop called during flush.
func TestCoordinatorNudger_StopDuringFlush(t *testing.T) {
	broker := pubsub.NewBroker[message.Event]()
	submitter := newMockSubmitter()
	clock := newMockClock()

	nudger := New(Config{
		Debounce:     50 * time.Millisecond,
		MsgBroker:    broker,
		CmdSubmitter: submitter,
		Clock:        clock,
	})
	nudger.Start()

	// Allow goroutine to start
	time.Sleep(10 * time.Millisecond)

	// Post a message
	postMessage(broker, "WORKER.1", message.ActorCoordinator, message.MessageInfo)
	time.Sleep(5 * time.Millisecond)

	// Advance to trigger flush
	clock.Advance(50 * time.Millisecond)

	// Immediately call Stop - should not deadlock
	done := make(chan struct{})
	go func() {
		nudger.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success - Stop returned without deadlock
	case <-time.After(1 * time.Second):
		t.Fatal("Stop() deadlocked")
	}
}

// TestCoordinatorNudger_StopBeforeStart verifies Stop is idempotent before Start.
func TestCoordinatorNudger_StopBeforeStart(t *testing.T) {
	broker := pubsub.NewBroker[message.Event]()
	submitter := newMockSubmitter()
	clock := newMockClock()

	nudger := New(Config{
		Debounce:     50 * time.Millisecond,
		MsgBroker:    broker,
		CmdSubmitter: submitter,
		Clock:        clock,
	})

	// Call Stop without Start - should not panic or deadlock
	done := make(chan struct{})
	go func() {
		nudger.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("Stop() deadlocked when called before Start()")
	}
}

// TestCoordinatorNudger_IgnoresCoordinatorMessages verifies coordinator messages are ignored.
func TestCoordinatorNudger_IgnoresCoordinatorMessages(t *testing.T) {
	broker := pubsub.NewBroker[message.Event]()
	submitter := newMockSubmitter()
	clock := newMockClock()

	nudger := New(Config{
		Debounce:     50 * time.Millisecond,
		MsgBroker:    broker,
		CmdSubmitter: submitter,
		Clock:        clock,
	})
	nudger.Start()
	defer nudger.Stop()

	// Allow goroutine to start
	time.Sleep(10 * time.Millisecond)

	// Post a message FROM coordinator (should be ignored)
	postMessage(broker, message.ActorCoordinator, message.ActorAll, message.MessageInfo)

	// Allow event to be processed
	time.Sleep(10 * time.Millisecond)

	// Advance past debounce
	clock.Advance(100 * time.Millisecond)

	// Give time for any errant commands
	time.Sleep(20 * time.Millisecond)

	// Should have no commands
	require.Empty(t, submitter.Commands())
}

// TestCoordinatorNudger_HandlesAllRecipient verifies messages to ALL trigger nudges.
func TestCoordinatorNudger_HandlesAllRecipient(t *testing.T) {
	broker := pubsub.NewBroker[message.Event]()
	submitter := newMockSubmitter()
	clock := newMockClock()

	nudger := New(Config{
		Debounce:     50 * time.Millisecond,
		MsgBroker:    broker,
		CmdSubmitter: submitter,
		Clock:        clock,
	})
	nudger.Start()
	defer nudger.Stop()

	// Allow goroutine to start
	time.Sleep(10 * time.Millisecond)

	// Post a message to ALL (should trigger nudge)
	postMessage(broker, "WORKER.1", message.ActorAll, message.MessageInfo)

	// Allow event to be processed
	time.Sleep(10 * time.Millisecond)

	// Advance past debounce
	clock.Advance(100 * time.Millisecond)

	// Wait for command
	require.True(t, submitter.WaitForCommands(1, 100*time.Millisecond))

	cmds := submitter.Commands()
	require.Len(t, cmds, 1)
	sendCmd := cmds[0].(*command.SendToProcessCommand)
	require.Contains(t, sendCmd.Content, "WORKER.1")
}

// TestCoordinatorNudger_IgnoresWorkerToWorkerMessages verifies worker-to-worker messages are ignored.
func TestCoordinatorNudger_IgnoresWorkerToWorkerMessages(t *testing.T) {
	broker := pubsub.NewBroker[message.Event]()
	submitter := newMockSubmitter()
	clock := newMockClock()

	nudger := New(Config{
		Debounce:     50 * time.Millisecond,
		MsgBroker:    broker,
		CmdSubmitter: submitter,
		Clock:        clock,
	})
	nudger.Start()
	defer nudger.Stop()

	// Allow goroutine to start
	time.Sleep(10 * time.Millisecond)

	// Post a message from worker to another worker (should be ignored)
	postMessage(broker, "WORKER.1", "WORKER.2", message.MessageInfo)

	// Allow event to be processed
	time.Sleep(10 * time.Millisecond)

	// Advance past debounce
	clock.Advance(100 * time.Millisecond)

	// Give time for any errant commands
	time.Sleep(20 * time.Millisecond)

	// Should have no commands
	require.Empty(t, submitter.Commands())
}
