package fabric

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/orchestration/fabric/domain"
	"github.com/zjrosen/perles/internal/orchestration/fabric/repository"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
)

// mockCommandSubmitter captures submitted commands.
type mockCommandSubmitter struct {
	mu       sync.Mutex
	commands []command.Command
}

func (m *mockCommandSubmitter) Submit(cmd command.Command) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.commands = append(m.commands, cmd)
}

func (m *mockCommandSubmitter) getCommands() []command.Command {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]command.Command(nil), m.commands...)
}

// mockSlugLookup provides channel slug lookup.
type mockSlugLookup struct {
	slugs map[string]string
}

func (m *mockSlugLookup) GetChannelSlug(channelID string) string {
	return m.slugs[channelID]
}

// The broker handles events on its own goroutine and only sends notifications
// once a debounce timer fires. Sleeping a fixed time and then asserting is racy:
// on a slow or loaded runner (Windows CI in particular, where timer and
// goroutine wakeups are coarse) the timer can fire after the test has looked,
// and a stalled loop can let the timer fire between events that should batch.
// testBroker removes both races: its timers only fire when the test fires them,
// and a barrier event tells the test when the loop has handled everything sent
// before it.

// settleTimeout bounds each wait on the broker loop. It is only reached if the
// broker is broken, so it is generous enough to survive a heavily starved
// runner.
const settleTimeout = 30 * time.Second

// barrierChannelPrefix marks the channel ID of testBroker barrier events.
const barrierChannelPrefix = "test-barrier-"

// fakeClock is a Clock whose timers only fire when the test fires them.
type fakeClock struct {
	mu     sync.Mutex
	timers []*fakeTimer
}

func (c *fakeClock) Now() time.Time { return time.Unix(0, 0) }

func (c *fakeClock) NewTimer(d time.Duration) Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	tm := &fakeTimer{ch: make(chan time.Time), d: d}
	c.timers = append(c.timers, tm)
	return tm
}

// durations returns the duration of every timer created so far, oldest first.
func (c *fakeClock) durations() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	ds := make([]time.Duration, len(c.timers))
	for i, tm := range c.timers {
		ds[i] = tm.d
	}
	return ds
}

// takeActiveTimer marks the most recently created timer fired and returns it,
// or returns nil if there is none or it was already stopped or fired.
func (c *fakeClock) takeActiveTimer() *fakeTimer {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.timers) == 0 {
		return nil
	}
	tm := c.timers[len(c.timers)-1]
	if !tm.markDone() {
		return nil
	}
	return tm
}

// fakeTimer has an unbuffered channel, so a completed send means the broker
// loop has received the tick.
type fakeTimer struct {
	ch   chan time.Time
	d    time.Duration
	mu   sync.Mutex
	done bool // stopped or fired
}

func (t *fakeTimer) C() <-chan time.Time { return t.ch }

func (t *fakeTimer) Stop() bool { return t.markDone() }

// markDone marks the timer stopped or fired and reports whether it was active.
func (t *fakeTimer) markDone() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	wasActive := !t.done
	t.done = true
	return wasActive
}

// barrierSubscriptions reports barrier events as the broker loop handles them.
// Every message event looks up its channel's subscribers, so a lookup for a
// barrier channel means the loop has finished every event queued before it.
type barrierSubscriptions struct {
	repository.SubscriptionRepository
	reached chan<- string
}

func (s *barrierSubscriptions) ListForChannel(channelID string) ([]domain.Subscription, error) {
	if strings.HasPrefix(channelID, barrierChannelPrefix) {
		s.reached <- channelID
		return nil, nil
	}
	return s.SubscriptionRepository.ListForChannel(channelID)
}

// testBroker is a Broker driven by a fakeClock, with helpers to wait for its
// event loop instead of sleeping.
type testBroker struct {
	*Broker
	t           *testing.T
	clock       *fakeClock
	barriers    chan string
	barrierSeen int
}

// newTestBroker creates (but does not start) a Broker from cfg, replacing its
// clock with a fakeClock and wrapping its subscriptions to detect barriers.
func newTestBroker(t *testing.T, cfg BrokerConfig) *testBroker {
	t.Helper()
	require.Nil(t, cfg.Clock, "newTestBroker installs its own clock")

	clock := &fakeClock{}
	barriers := make(chan string, 1)
	cfg.Clock = clock
	cfg.Subscriptions = &barrierSubscriptions{
		SubscriptionRepository: cfg.Subscriptions,
		reached:                barriers,
	}
	return &testBroker{Broker: NewBroker(cfg), t: t, clock: clock, barriers: barriers}
}

// drain blocks until the broker loop has handled every event sent so far. The
// barrier event it sends has no subscribers or mentions, so it notifies no one.
func (b *testBroker) drain() {
	b.t.Helper()
	b.barrierSeen++
	id := fmt.Sprintf("%s%d", barrierChannelPrefix, b.barrierSeen)
	b.HandleEvent(Event{
		Type:      EventMessagePosted,
		ChannelID: id,
		Thread:    &domain.Thread{ID: id, Type: domain.ThreadMessage, CreatedBy: "test-barrier"},
	})

	select {
	case got := <-b.barriers:
		require.Equal(b.t, id, got, "broker loop reached barriers out of order")
	case <-time.After(settleTimeout):
		b.t.Fatalf("broker loop did not handle barrier %s within %s", id, settleTimeout)
	}
}

// settle ends the debounce window: it waits for the broker loop to handle every
// event sent so far, fires the debounce timer if one is running, and waits for
// the resulting flush to finish, so all notifications have been submitted.
func (b *testBroker) settle() {
	b.t.Helper()
	b.drain()

	tm := b.clock.takeActiveTimer()
	if tm == nil {
		return // nothing pending, so nothing to flush
	}
	select {
	case tm.ch <- b.clock.Now():
	case <-time.After(settleTimeout):
		b.t.Fatalf("broker loop did not receive the debounce timer within %s", settleTimeout)
	}
	// The loop flushes right after receiving the tick, before it can read the
	// next event, so handling another barrier means the flush has finished.
	b.drain()
}

func TestBroker_New(t *testing.T) {
	subs := repository.NewMemorySubscriptionRepository()
	submitter := &mockCommandSubmitter{}

	broker := NewBroker(BrokerConfig{
		CmdSubmitter:  submitter,
		Subscriptions: subs,
	})

	require.NotNil(t, broker)
	assert.Equal(t, DefaultDebounce, broker.debounce)
}

func TestBroker_MentionBasedNotification(t *testing.T) {
	subs := repository.NewMemorySubscriptionRepository()
	submitter := &mockCommandSubmitter{}

	broker := newTestBroker(t, BrokerConfig{
		CmdSubmitter:  submitter,
		Subscriptions: subs,
		Debounce:      10 * time.Millisecond,
	})

	// Subscribe coordinator to tasks channel with mentions-only mode
	channelID := "channel-tasks"
	_, err := subs.Subscribe(channelID, "COORDINATOR", domain.ModeMentions)
	require.NoError(t, err)

	broker.Start()
	defer broker.Stop()

	// Send event with @COORDINATOR mention
	event := Event{
		Type:      EventMessagePosted,
		ChannelID: channelID,
		Thread: &domain.Thread{
			ID:        "msg-1",
			Type:      domain.ThreadMessage,
			CreatedBy: "WORKER.1",
			Mentions:  []string{"COORDINATOR"},
		},
		Mentions: []string{"COORDINATOR"},
	}

	broker.HandleEvent(event)

	// Wait for debounce to flush
	broker.settle()

	cmds := submitter.getCommands()
	require.Len(t, cmds, 1)

	sendCmd, ok := cmds[0].(*command.SendToProcessCommand)
	require.True(t, ok)
	assert.Equal(t, "COORDINATOR", sendCmd.ProcessID)
	assert.Contains(t, sendCmd.Content, "WORKER.1")
	assert.Contains(t, sendCmd.Content, "fabric_inbox")
}

func TestBroker_SubscriptionModeAll(t *testing.T) {
	subs := repository.NewMemorySubscriptionRepository()
	submitter := &mockCommandSubmitter{}

	broker := newTestBroker(t, BrokerConfig{
		CmdSubmitter:  submitter,
		Subscriptions: subs,
		Debounce:      10 * time.Millisecond,
	})

	channelID := "channel-general"
	_, err := subs.Subscribe(channelID, "WORKER.2", domain.ModeAll)
	require.NoError(t, err)

	broker.Start()
	defer broker.Stop()

	// Send event WITHOUT mentioning WORKER.2
	event := Event{
		Type:      EventMessagePosted,
		ChannelID: channelID,
		Thread: &domain.Thread{
			ID:        "msg-1",
			Type:      domain.ThreadMessage,
			CreatedBy: "COORDINATOR",
			Mentions:  []string{}, // No mentions
		},
		Mentions: []string{},
	}

	broker.HandleEvent(event)
	broker.settle()

	// WORKER.2 should still be notified (mode=all)
	cmds := submitter.getCommands()
	require.Len(t, cmds, 1)

	sendCmd, ok := cmds[0].(*command.SendToProcessCommand)
	require.True(t, ok)
	assert.Equal(t, "WORKER.2", sendCmd.ProcessID)
}

func TestBroker_SubscriptionModeNone(t *testing.T) {
	subs := repository.NewMemorySubscriptionRepository()
	submitter := &mockCommandSubmitter{}

	broker := newTestBroker(t, BrokerConfig{
		CmdSubmitter:  submitter,
		Subscriptions: subs,
		Debounce:      10 * time.Millisecond,
	})

	channelID := "channel-general"
	_, err := subs.Subscribe(channelID, "WORKER.2", domain.ModeNone)
	require.NoError(t, err)

	broker.Start()
	defer broker.Stop()

	// Send event mentioning WORKER.2 but subscription mode is none
	event := Event{
		Type:      EventMessagePosted,
		ChannelID: channelID,
		Thread: &domain.Thread{
			ID:        "msg-1",
			Type:      domain.ThreadMessage,
			CreatedBy: "COORDINATOR",
			Mentions:  []string{"WORKER.2"},
		},
		Mentions: []string{"WORKER.2"},
	}

	broker.HandleEvent(event)
	broker.settle()

	// WORKER.2 should be notified via explicit mention even with mode=none
	cmds := submitter.getCommands()
	require.Len(t, cmds, 1) // Explicit mentions always notify
}

func TestBroker_NoSelfNotification(t *testing.T) {
	subs := repository.NewMemorySubscriptionRepository()
	submitter := &mockCommandSubmitter{}

	broker := newTestBroker(t, BrokerConfig{
		CmdSubmitter:  submitter,
		Subscriptions: subs,
		Debounce:      10 * time.Millisecond,
	})

	channelID := "channel-tasks"
	_, err := subs.Subscribe(channelID, "WORKER.1", domain.ModeAll)
	require.NoError(t, err)

	broker.Start()
	defer broker.Stop()

	// WORKER.1 sends a message - should not notify themselves
	event := Event{
		Type:      EventMessagePosted,
		ChannelID: channelID,
		Thread: &domain.Thread{
			ID:        "msg-1",
			Type:      domain.ThreadMessage,
			CreatedBy: "WORKER.1",
			Mentions:  []string{"WORKER.1"}, // Self-mention
		},
		Mentions: []string{"WORKER.1"},
	}

	broker.HandleEvent(event)
	broker.settle()

	// No notification - sender is excluded
	cmds := submitter.getCommands()
	assert.Len(t, cmds, 0)
}

func TestBroker_BatchMultipleSenders(t *testing.T) {
	subs := repository.NewMemorySubscriptionRepository()
	submitter := &mockCommandSubmitter{}

	broker := newTestBroker(t, BrokerConfig{
		CmdSubmitter:  submitter,
		Subscriptions: subs,
		Debounce:      10 * time.Millisecond,
	})

	channelID := "channel-tasks"
	_, err := subs.Subscribe(channelID, "COORDINATOR", domain.ModeAll)
	require.NoError(t, err)

	broker.Start()
	defer broker.Stop()

	// Multiple workers send messages rapidly
	for _, workerID := range []string{"WORKER.1", "WORKER.2", "WORKER.3"} {
		event := Event{
			Type:      EventMessagePosted,
			ChannelID: channelID,
			Thread: &domain.Thread{
				ID:        "msg-" + workerID,
				Type:      domain.ThreadMessage,
				CreatedBy: workerID,
			},
		}
		broker.HandleEvent(event)
	}

	// All three messages are handled but the debounce window is still open.
	broker.drain()
	require.Empty(t, submitter.getCommands(), "no notification before the debounce timer fires")
	timers := broker.clock.durations()
	require.NotEmpty(t, timers, "handling a message should start the debounce timer")
	for _, d := range timers {
		require.Equal(t, 10*time.Millisecond, d, "debounce timer should use the configured duration")
	}

	broker.settle()

	// Single batched notification to coordinator
	cmds := submitter.getCommands()
	require.Len(t, cmds, 1)

	sendCmd, ok := cmds[0].(*command.SendToProcessCommand)
	require.True(t, ok)
	assert.Equal(t, "COORDINATOR", sendCmd.ProcessID)
	// All workers should be mentioned in the batched message
	assert.Contains(t, sendCmd.Content, "WORKER.1")
	assert.Contains(t, sendCmd.Content, "WORKER.2")
	assert.Contains(t, sendCmd.Content, "WORKER.3")
}

// TestBroker_RealClockFlush covers the RealClock timer that the other tests
// replace with fakeClock. It waits for the notification rather than sleeping a
// fixed time, so a slow runner only makes it take longer.
func TestBroker_RealClockFlush(t *testing.T) {
	subs := repository.NewMemorySubscriptionRepository()
	submitter := &mockCommandSubmitter{}

	broker := NewBroker(BrokerConfig{
		CmdSubmitter:  submitter,
		Subscriptions: subs,
		Debounce:      10 * time.Millisecond,
	})

	channelID := "channel-tasks"
	_, err := subs.Subscribe(channelID, "COORDINATOR", domain.ModeAll)
	require.NoError(t, err)

	broker.Start()
	defer broker.Stop()

	broker.HandleEvent(Event{
		Type:      EventMessagePosted,
		ChannelID: channelID,
		Thread: &domain.Thread{
			ID:        "msg-1",
			Type:      domain.ThreadMessage,
			CreatedBy: "WORKER.1",
		},
	})

	// One message for one recipient produces exactly one notification.
	require.Eventually(t, func() bool {
		return len(submitter.getCommands()) > 0
	}, settleTimeout, time.Millisecond, "debounce timer never flushed the notification")

	cmds := submitter.getCommands()
	require.Len(t, cmds, 1)
	sendCmd, ok := cmds[0].(*command.SendToProcessCommand)
	require.True(t, ok)
	assert.Equal(t, "COORDINATOR", sendCmd.ProcessID)
	assert.Contains(t, sendCmd.Content, "WORKER.1")
}

func TestBroker_ChannelSlugLookup(t *testing.T) {
	subs := repository.NewMemorySubscriptionRepository()
	submitter := &mockCommandSubmitter{}
	slugLookup := &mockSlugLookup{
		slugs: map[string]string{
			"channel-123": "tasks",
		},
	}

	broker := newTestBroker(t, BrokerConfig{
		CmdSubmitter:  submitter,
		Subscriptions: subs,
		Debounce:      10 * time.Millisecond,
		SlugLookup:    slugLookup,
	})

	channelID := "channel-123"
	_, err := subs.Subscribe(channelID, "COORDINATOR", domain.ModeAll)
	require.NoError(t, err)

	broker.Start()
	defer broker.Stop()

	event := Event{
		Type:      EventMessagePosted,
		ChannelID: channelID,
		Thread: &domain.Thread{
			ID:        "msg-1",
			Type:      domain.ThreadMessage,
			CreatedBy: "WORKER.1",
		},
	}

	broker.HandleEvent(event)
	broker.settle()

	cmds := submitter.getCommands()
	require.Len(t, cmds, 1)

	sendCmd, ok := cmds[0].(*command.SendToProcessCommand)
	require.True(t, ok)
	assert.Contains(t, sendCmd.Content, "#tasks") // Resolved slug
}

func TestBroker_ReplyEventNotification(t *testing.T) {
	subs := repository.NewMemorySubscriptionRepository()
	submitter := &mockCommandSubmitter{}

	broker := newTestBroker(t, BrokerConfig{
		CmdSubmitter:  submitter,
		Subscriptions: subs,
		Debounce:      10 * time.Millisecond,
	})

	channelID := "channel-tasks"
	_, err := subs.Subscribe(channelID, "WORKER.1", domain.ModeMentions)
	require.NoError(t, err)

	broker.Start()
	defer broker.Stop()

	// Reply event with mention
	event := Event{
		Type:      EventReplyPosted,
		ChannelID: channelID,
		Thread: &domain.Thread{
			ID:        "reply-1",
			Type:      domain.ThreadMessage,
			CreatedBy: "COORDINATOR",
			Mentions:  []string{"WORKER.1"},
		},
		Mentions: []string{"WORKER.1"},
	}

	broker.HandleEvent(event)
	broker.settle()

	cmds := submitter.getCommands()
	require.Len(t, cmds, 1)

	sendCmd, ok := cmds[0].(*command.SendToProcessCommand)
	require.True(t, ok)
	assert.Equal(t, "WORKER.1", sendCmd.ProcessID)
}

func TestBroker_IgnoresNonMessageEvents(t *testing.T) {
	subs := repository.NewMemorySubscriptionRepository()
	submitter := &mockCommandSubmitter{}

	broker := newTestBroker(t, BrokerConfig{
		CmdSubmitter:  submitter,
		Subscriptions: subs,
		Debounce:      10 * time.Millisecond,
	})

	channelID := "channel-tasks"
	_, err := subs.Subscribe(channelID, "COORDINATOR", domain.ModeAll)
	require.NoError(t, err)

	broker.Start()
	defer broker.Stop()

	// Channel created event - should be ignored
	event := Event{
		Type:      EventChannelCreated,
		ChannelID: channelID,
		Thread: &domain.Thread{
			ID:   "channel-1",
			Type: domain.ThreadChannel,
		},
	}

	broker.HandleEvent(event)
	broker.settle()

	cmds := submitter.getCommands()
	assert.Len(t, cmds, 0)
}

func TestContainsMention(t *testing.T) {
	tests := []struct {
		name     string
		mentions []string
		agentID  string
		expected bool
	}{
		{
			name:     "exact match",
			mentions: []string{"COORDINATOR", "WORKER.1"},
			agentID:  "COORDINATOR",
			expected: true,
		},
		{
			name:     "case insensitive",
			mentions: []string{"coordinator"},
			agentID:  "COORDINATOR",
			expected: true,
		},
		{
			name:     "not found",
			mentions: []string{"WORKER.1", "WORKER.2"},
			agentID:  "COORDINATOR",
			expected: false,
		},
		{
			name:     "empty mentions",
			mentions: []string{},
			agentID:  "COORDINATOR",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsMention(tt.mentions, tt.agentID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBroker_ParticipantNotification(t *testing.T) {
	// Test that reply events notify all parent thread participants
	subs := repository.NewMemorySubscriptionRepository()
	submitter := &mockCommandSubmitter{}
	slugLookup := &mockSlugLookup{
		slugs: map[string]string{"channel-planning": "planning"},
	}

	broker := newTestBroker(t, BrokerConfig{
		CmdSubmitter:  submitter,
		Subscriptions: subs,
		SlugLookup:    slugLookup,
		Debounce:      10 * time.Millisecond,
	})

	broker.Start()
	defer broker.Stop()

	// Send a reply event with participants from parent thread
	// Worker.1 replies, parent has COORDINATOR, WORKER.1, WORKER.2, WORKER.3 as participants
	event := Event{
		Type:      EventReplyPosted,
		ChannelID: "channel-planning",
		ParentID:  "parent-msg-1",
		Thread: &domain.Thread{
			ID:        "reply-1",
			Type:      domain.ThreadMessage,
			CreatedBy: "WORKER.1",
			Content:   "I think we should start with the API",
		},
		Mentions:     []string{}, // No explicit mentions in reply
		Participants: []string{"COORDINATOR", "WORKER.1", "WORKER.2", "WORKER.3"},
	}
	broker.HandleEvent(event)

	// Wait for debounce
	broker.settle()

	// Should have notified COORDINATOR, WORKER.2, WORKER.3 (not WORKER.1 - the sender)
	cmds := submitter.getCommands()
	require.Len(t, cmds, 3, "should notify 3 participants (excluding sender)")

	// Collect notified agents
	notified := make(map[string]bool)
	for _, cmd := range cmds {
		if sendCmd, ok := cmd.(*command.SendToProcessCommand); ok {
			notified[sendCmd.ProcessID] = true
		}
	}

	assert.True(t, notified["COORDINATOR"], "COORDINATOR should be notified")
	assert.True(t, notified["WORKER.2"], "WORKER.2 should be notified")
	assert.True(t, notified["WORKER.3"], "WORKER.3 should be notified")
	assert.False(t, notified["WORKER.1"], "WORKER.1 (sender) should NOT be notified")
}

func TestIsNotificationSuppressedChannel(t *testing.T) {
	tests := []struct {
		name     string
		slug     string
		expected bool
	}{
		{
			name:     "observer channel is suppressed",
			slug:     domain.SlugObserver,
			expected: true,
		},
		{
			name:     "tasks channel is not suppressed",
			slug:     domain.SlugTasks,
			expected: false,
		},
		{
			name:     "planning channel is not suppressed",
			slug:     domain.SlugPlanning,
			expected: false,
		},
		{
			name:     "general channel is not suppressed",
			slug:     domain.SlugGeneral,
			expected: false,
		},
		{
			name:     "system channel is not suppressed",
			slug:     domain.SlugSystem,
			expected: false,
		},
		{
			name:     "empty string is not suppressed",
			slug:     "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNotificationSuppressedChannel(tt.slug)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBroker_ObserverChannel_SuppressesMentionNotifications(t *testing.T) {
	// Test that explicit @mentions in #observer do NOT trigger notifications
	subs := repository.NewMemorySubscriptionRepository()
	submitter := &mockCommandSubmitter{}
	slugLookup := &mockSlugLookup{
		slugs: map[string]string{"channel-observer": domain.SlugObserver},
	}

	broker := newTestBroker(t, BrokerConfig{
		CmdSubmitter:  submitter,
		Subscriptions: subs,
		SlugLookup:    slugLookup,
		Debounce:      10 * time.Millisecond,
	})

	broker.Start()
	defer broker.Stop()

	// Observer sends message with @worker-1 mention in #observer channel
	event := Event{
		Type:      EventMessagePosted,
		ChannelID: "channel-observer",
		Thread: &domain.Thread{
			ID:        "msg-1",
			Type:      domain.ThreadMessage,
			CreatedBy: "observer",
			Content:   "@worker-1 appears to be struggling with the task",
			Mentions:  []string{"worker-1"},
		},
		Mentions: []string{"worker-1"},
	}

	broker.HandleEvent(event)

	// Wait for debounce to flush
	broker.settle()

	// worker-1 should NOT be notified - observer channel suppresses all notifications
	cmds := submitter.getCommands()
	assert.Len(t, cmds, 0, "observer channel should suppress all mention notifications")
}

func TestBroker_ObserverChannel_SuppressesSubscriptionNotifications(t *testing.T) {
	// Test that ModeAll subscribers to #observer are NOT notified
	subs := repository.NewMemorySubscriptionRepository()
	submitter := &mockCommandSubmitter{}
	slugLookup := &mockSlugLookup{
		slugs: map[string]string{"channel-observer": domain.SlugObserver},
	}

	broker := newTestBroker(t, BrokerConfig{
		CmdSubmitter:  submitter,
		Subscriptions: subs,
		SlugLookup:    slugLookup,
		Debounce:      10 * time.Millisecond,
	})

	// Subscribe worker-1 to #observer with ModeAll
	channelID := "channel-observer"
	_, err := subs.Subscribe(channelID, "worker-1", domain.ModeAll)
	require.NoError(t, err)

	broker.Start()
	defer broker.Stop()

	// Observer sends a message (no mentions)
	event := Event{
		Type:      EventMessagePosted,
		ChannelID: channelID,
		Thread: &domain.Thread{
			ID:        "msg-1",
			Type:      domain.ThreadMessage,
			CreatedBy: "observer",
			Content:   "I've noticed some interesting patterns",
		},
		Mentions: []string{},
	}

	broker.HandleEvent(event)

	// Wait for debounce to flush
	broker.settle()

	// worker-1 should NOT be notified - observer channel suppresses all notifications
	cmds := submitter.getCommands()
	assert.Len(t, cmds, 0, "observer channel should suppress all subscription notifications")
}

func TestBroker_ObserverChannel_SuppressesThreadParticipantNotifications(t *testing.T) {
	// Test that reply events in #observer do NOT notify non-owner thread participants
	subs := repository.NewMemorySubscriptionRepository()
	submitter := &mockCommandSubmitter{}
	slugLookup := &mockSlugLookup{
		slugs: map[string]string{"channel-observer": domain.SlugObserver},
	}

	broker := newTestBroker(t, BrokerConfig{
		CmdSubmitter:  submitter,
		Subscriptions: subs,
		SlugLookup:    slugLookup,
		Debounce:      10 * time.Millisecond,
	})

	broker.Start()
	defer broker.Stop()

	// Observer sends a reply to a thread in #observer
	// The parent thread has participants: user, OBSERVER, worker-1 (who observer was discussing)
	event := Event{
		Type:      EventReplyPosted,
		ChannelID: "channel-observer",
		ParentID:  "parent-thread-1",
		Thread: &domain.Thread{
			ID:        "reply-1",
			Type:      domain.ThreadMessage,
			CreatedBy: "OBSERVER", // Observer is the sender
			Content:   "worker-1 seems to be making progress now",
		},
		Mentions:     []string{},
		Participants: []string{"user", "OBSERVER", "worker-1"},
	}

	broker.HandleEvent(event)

	// Wait for debounce to flush
	broker.settle()

	// worker-1 should NOT be notified - they're not the channel owner
	// user is also not notified (not an agent)
	cmds := submitter.getCommands()
	assert.Len(t, cmds, 0, "observer channel should suppress notifications to non-owner participants")
}

func TestBroker_ObserverChannel_NotifiesObserverOnUserReply(t *testing.T) {
	// Test that OBSERVER is notified when user replies in #observer thread
	subs := repository.NewMemorySubscriptionRepository()
	submitter := &mockCommandSubmitter{}
	slugLookup := &mockSlugLookup{
		slugs: map[string]string{"channel-observer": domain.SlugObserver},
	}

	broker := newTestBroker(t, BrokerConfig{
		CmdSubmitter:  submitter,
		Subscriptions: subs,
		SlugLookup:    slugLookup,
		Debounce:      10 * time.Millisecond,
	})

	broker.Start()
	defer broker.Stop()

	// User sends a reply to a thread in #observer
	// The parent thread has participants: user, OBSERVER
	event := Event{
		Type:      EventReplyPosted,
		ChannelID: "channel-observer",
		ParentID:  "parent-thread-1",
		Thread: &domain.Thread{
			ID:        "reply-1",
			Type:      domain.ThreadMessage,
			CreatedBy: "user", // User is the sender
			Content:   "Can you give me a summary of what's happening?",
		},
		Mentions:     []string{},
		Participants: []string{"user", "OBSERVER"},
	}

	broker.HandleEvent(event)

	// Wait for debounce to flush
	broker.settle()

	// OBSERVER should be notified since they own the #observer channel
	cmds := submitter.getCommands()
	require.Len(t, cmds, 1, "OBSERVER should be notified when user replies in #observer")
	sendCmd, ok := cmds[0].(*command.SendToProcessCommand)
	require.True(t, ok, "command should be SendToProcessCommand")
	assert.Equal(t, "OBSERVER", sendCmd.ProcessID)
}

func TestBroker_OtherChannels_NotificationsWork(t *testing.T) {
	// Regression test: ensure #tasks, #planning, #general still send notifications normally
	subs := repository.NewMemorySubscriptionRepository()
	submitter := &mockCommandSubmitter{}
	slugLookup := &mockSlugLookup{
		slugs: map[string]string{
			"channel-tasks":    domain.SlugTasks,
			"channel-planning": domain.SlugPlanning,
			"channel-general":  domain.SlugGeneral,
		},
	}

	broker := newTestBroker(t, BrokerConfig{
		CmdSubmitter:  submitter,
		Subscriptions: subs,
		SlugLookup:    slugLookup,
		Debounce:      10 * time.Millisecond,
	})

	broker.Start()
	defer broker.Stop()

	// Test 1: @mention in #tasks should notify
	event1 := Event{
		Type:      EventMessagePosted,
		ChannelID: "channel-tasks",
		Thread: &domain.Thread{
			ID:        "msg-1",
			Type:      domain.ThreadMessage,
			CreatedBy: "COORDINATOR",
			Content:   "@worker-1 please handle this task",
			Mentions:  []string{"worker-1"},
		},
		Mentions: []string{"worker-1"},
	}
	broker.HandleEvent(event1)

	// Wait for debounce to flush
	broker.settle()

	cmds := submitter.getCommands()
	require.Len(t, cmds, 1, "#tasks should still send mention notifications")

	sendCmd, ok := cmds[0].(*command.SendToProcessCommand)
	require.True(t, ok)
	assert.Equal(t, "worker-1", sendCmd.ProcessID)
	assert.Contains(t, sendCmd.Content, "#tasks")
}

func TestBroker_HereMention_RequiresParticipantRegistry(t *testing.T) {
	// Test that @here does NOT work when no participant registry is configured.
	// Agents must join via fabric_join to receive @here broadcasts.
	subs := repository.NewMemorySubscriptionRepository()
	submitter := &mockCommandSubmitter{}

	channelID := "channel-tasks"
	slugLookup := &mockSlugLookup{slugs: map[string]string{channelID: "tasks"}}

	broker := newTestBroker(t, BrokerConfig{
		CmdSubmitter:  submitter,
		Subscriptions: subs,
		// Participants NOT set - @here should not work
		Debounce:   10 * time.Millisecond,
		SlugLookup: slugLookup,
	})

	// Subscribe agents with ModeNone - they should NOT get notifications
	// (ModeAll would trigger regardless of @here, obscuring the test)
	_, err := subs.Subscribe(channelID, "worker-1", domain.ModeNone)
	require.NoError(t, err)
	_, err = subs.Subscribe(channelID, "worker-2", domain.ModeNone)
	require.NoError(t, err)
	_, err = subs.Subscribe(channelID, "coordinator", domain.ModeNone)
	require.NoError(t, err)

	broker.Start()
	defer broker.Stop()

	// Send message with @here mention from worker-1
	event := Event{
		Type:      EventMessagePosted,
		ChannelID: channelID,
		Thread: &domain.Thread{
			ID:        "msg-1",
			Type:      domain.ThreadMessage,
			CreatedBy: "worker-1",
			Mentions:  []string{domain.MentionHere},
		},
		Mentions: []string{domain.MentionHere},
	}
	broker.HandleEvent(event)

	// Wait for debounce
	broker.settle()

	cmds := submitter.getCommands()
	// @here should NOT notify anyone when participant registry is not configured
	// (with ModeNone subscriptions and no participant registry, no one gets notified)
	require.Len(t, cmds, 0, "@here should not work without participant registry")
}

func TestBroker_HereMention_ObserverChannelSuppressed(t *testing.T) {
	subs := repository.NewMemorySubscriptionRepository()
	submitter := &mockCommandSubmitter{}

	channelID := "channel-observer"
	slugLookup := &mockSlugLookup{slugs: map[string]string{channelID: domain.SlugObserver}}

	broker := newTestBroker(t, BrokerConfig{
		CmdSubmitter:  submitter,
		Subscriptions: subs,
		Debounce:      10 * time.Millisecond,
		SlugLookup:    slugLookup,
	})

	// Subscribe coordinator and observer to the channel
	_, err := subs.Subscribe(channelID, "COORDINATOR", domain.ModeAll)
	require.NoError(t, err)
	_, err = subs.Subscribe(channelID, "OBSERVER", domain.ModeAll)
	require.NoError(t, err)

	broker.Start()
	defer broker.Stop()

	// Send @here from user - should only notify OBSERVER, not coordinator
	event := Event{
		Type:      EventMessagePosted,
		ChannelID: channelID,
		Thread: &domain.Thread{
			ID:        "msg-1",
			Type:      domain.ThreadMessage,
			CreatedBy: "user",
			Mentions:  []string{domain.MentionHere},
		},
		Mentions: []string{domain.MentionHere},
	}
	broker.HandleEvent(event)

	broker.settle()

	cmds := submitter.getCommands()
	require.Len(t, cmds, 1, "@here in #observer should only notify OBSERVER")

	sendCmd, ok := cmds[0].(*command.SendToProcessCommand)
	require.True(t, ok)
	assert.Equal(t, "OBSERVER", sendCmd.ProcessID)
}

func TestBroker_HereMention_UsesParticipantRegistry(t *testing.T) {
	subs := repository.NewMemorySubscriptionRepository()
	participants := repository.NewMemoryParticipantRepository()
	submitter := &mockCommandSubmitter{}

	channelID := "channel-general"
	slugLookup := &mockSlugLookup{slugs: map[string]string{channelID: "general"}}

	broker := newTestBroker(t, BrokerConfig{
		CmdSubmitter:  submitter,
		Subscriptions: subs,
		Participants:  participants,
		Debounce:      10 * time.Millisecond,
		SlugLookup:    slugLookup,
	})

	// Subscribe agents with ModeMentions (won't get notified unless mentioned)
	_, err := subs.Subscribe(channelID, "worker-1", domain.ModeMentions)
	require.NoError(t, err)
	_, err = subs.Subscribe(channelID, "worker-2", domain.ModeMentions)
	require.NoError(t, err)
	_, err = subs.Subscribe(channelID, "worker-3", domain.ModeMentions)
	require.NoError(t, err)

	// Only register some as participants (simulating they called fabric_join)
	_, err = participants.Join("worker-1", domain.RoleWorker)
	require.NoError(t, err)
	_, err = participants.Join("worker-2", domain.RoleWorker)
	require.NoError(t, err)
	_, err = participants.Join("coordinator", domain.RoleCoordinator)
	require.NoError(t, err)
	// worker-3 is subscribed but NOT a participant (never called fabric_join)

	broker.Start()
	defer broker.Stop()

	// Send @here from coordinator
	event := Event{
		Type:      EventMessagePosted,
		ChannelID: channelID,
		Thread: &domain.Thread{
			ID:        "msg-1",
			Type:      domain.ThreadMessage,
			CreatedBy: "coordinator",
			Mentions:  []string{domain.MentionHere},
		},
		Mentions: []string{domain.MentionHere},
	}
	broker.HandleEvent(event)

	broker.settle()

	cmds := submitter.getCommands()
	// Should notify worker-1 and worker-2 (participants, excluding sender)
	// worker-3 is NOT notified because they're not a participant
	require.Len(t, cmds, 2, "@here should only notify participants")

	notified := make(map[string]bool)
	for _, cmd := range cmds {
		sendCmd, ok := cmd.(*command.SendToProcessCommand)
		require.True(t, ok)
		notified[sendCmd.ProcessID] = true
	}

	assert.True(t, notified["worker-1"], "worker-1 (participant) should be notified")
	assert.True(t, notified["worker-2"], "worker-2 (participant) should be notified")
	assert.False(t, notified["worker-3"], "worker-3 (not participant) should NOT be notified")
	assert.False(t, notified["coordinator"], "coordinator (sender) should NOT be notified")
}
