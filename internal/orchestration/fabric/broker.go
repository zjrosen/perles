// Package fabric provides graph-based messaging for multi-agent orchestration.
package fabric

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/fabric/domain"
	"github.com/zjrosen/perles/internal/orchestration/fabric/repository"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/process"
)

// DefaultDebounce is the default debounce duration for batching nudges.
// 3 seconds allows multiple worker completions to be batched into a single coordinator nudge.
const DefaultDebounce = 3 * time.Second

// Clock provides time-related operations for testability.
type Clock interface {
	Now() time.Time
	NewTimer(d time.Duration) Timer
}

// Timer represents a timer that can be stopped and provides a channel.
type Timer interface {
	Stop() bool
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

type realTimer struct {
	timer *time.Timer
}

func (t *realTimer) Stop() bool          { return t.timer.Stop() }
func (t *realTimer) C() <-chan time.Time { return t.timer.C }

// pendingNudge tracks a pending notification for an agent.
type pendingNudge struct {
	channelSlug string
	senders     map[string]bool // unique sender IDs
}

// ChannelSlugLookup provides channel ID to slug resolution.
type ChannelSlugLookup interface {
	GetChannelSlug(channelID string) string
}

// Broker accumulates @mention notifications and sends consolidated nudges
// to agents after a debounce window. It listens to Fabric events and respects
// subscription modes (all/mentions/none).
type Broker struct {
	debounce      time.Duration
	clock         Clock
	cmdSubmitter  process.CommandSubmitter
	subscriptions repository.SubscriptionRepository
	slugLookup    ChannelSlugLookup

	mu      sync.Mutex
	pending map[string]*pendingNudge // agentID -> pending nudge
	timer   Timer

	eventCh   chan Event
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	closeOnce sync.Once
}

// BrokerConfig holds configuration for creating a Broker.
type BrokerConfig struct {
	// Debounce is the duration to wait before sending a batched nudge.
	// Defaults to DefaultDebounce (1 second) if zero.
	Debounce time.Duration

	// CmdSubmitter is used to submit commands to processes.
	// Required.
	CmdSubmitter process.CommandSubmitter

	// Subscriptions is used to look up agent subscription modes.
	// Required.
	Subscriptions repository.SubscriptionRepository

	// SlugLookup resolves channel IDs to slugs for notification messages.
	// Optional - falls back to "channel" if nil.
	SlugLookup ChannelSlugLookup

	// Clock provides time operations. Defaults to RealClock if nil.
	Clock Clock
}

// NewBroker creates a new Fabric Broker with the given configuration.
func NewBroker(cfg BrokerConfig) *Broker {
	debounce := cfg.Debounce
	if debounce == 0 {
		debounce = DefaultDebounce
	}

	clock := cfg.Clock
	if clock == nil {
		clock = RealClock{}
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &Broker{
		debounce:      debounce,
		clock:         clock,
		cmdSubmitter:  cfg.CmdSubmitter,
		subscriptions: cfg.Subscriptions,
		slugLookup:    cfg.SlugLookup,
		pending:       make(map[string]*pendingNudge),
		eventCh:       make(chan Event, 100),
		ctx:           ctx,
		cancel:        cancel,
		done:          make(chan struct{}),
	}
}

// Start begins listening to events and batching nudges.
// Must be called after construction. Safe to call only once.
func (b *Broker) Start() {
	go b.loop()
}

// Stop terminates the broker and releases resources.
// Blocks until the event loop has exited. Safe to call multiple times.
func (b *Broker) Stop() {
	b.cancel()
	b.closeDone()
	<-b.done

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
	b.pending = make(map[string]*pendingNudge)
}

// closeDone safely closes the done channel exactly once.
func (b *Broker) closeDone() {
	b.closeOnce.Do(func() {
		close(b.done)
	})
}

// HandleEvent is the callback to be registered with FabricService.SetEventHandler().
// It queues events for processing by the broker's event loop.
func (b *Broker) HandleEvent(event Event) {
	select {
	case b.eventCh <- event:
	default:
		// Channel full, drop event (non-blocking)
	}
}

// loop is the main event loop that processes Fabric events with debouncing.
func (b *Broker) loop() {
	defer b.closeDone()

	for {
		timerCh := b.timerChan()

		select {
		case event, ok := <-b.eventCh:
			if !ok {
				b.flush()
				return
			}
			b.handleEvent(event)

		case <-timerCh:
			b.flush()

		case <-b.ctx.Done():
			return
		}
	}
}

// timerChan returns the timer's channel, or nil if no timer is active.
func (b *Broker) timerChan() <-chan time.Time {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.timer != nil {
		return b.timer.C()
	}
	return nil
}

// handleEvent processes a Fabric event and potentially queues notifications.
func (b *Broker) handleEvent(event Event) {
	// Only process message and reply events
	if event.Type != EventMessagePosted && event.Type != EventReplyPosted {
		return
	}

	if event.Thread == nil {
		return
	}

	channelID := event.ChannelID
	sender := event.Thread.CreatedBy
	mentions := event.Mentions

	// Get channel slug for notification message
	channelSlug := b.channelSlugForID(channelID)

	// Skip all notifications for suppressed channels (e.g., #observer)
	if isNotificationSuppressedChannel(channelSlug) {
		log.Debug(log.CatOrch, "notifications suppressed for channel", "channel", channelSlug)
		return
	}

	// Get subscribers to this channel
	subscribers, err := b.subscriptions.ListForChannel(channelID)
	if err != nil {
		return
	}

	// Determine who gets notified based on subscription mode and mentions
	for _, sub := range subscribers {
		// Don't notify the sender
		if sub.AgentID == sender {
			continue
		}

		shouldNotify := false

		switch sub.Mode {
		case domain.ModeAll:
			// Notify on all messages in subscribed channels
			shouldNotify = true
		case domain.ModeMentions:
			// Only notify if explicitly @mentioned
			shouldNotify = containsMention(mentions, sub.AgentID)
		case domain.ModeNone:
			// Never notify via subscription
			shouldNotify = false
		}

		if shouldNotify {
			b.addPending(sub.AgentID, channelSlug, sender)
		}
	}

	// Also notify anyone @mentioned who isn't subscribed (explicit mention always notifies)
	for _, mentionedID := range mentions {
		if mentionedID == sender {
			continue
		}
		b.addPending(mentionedID, channelSlug, sender)
	}

	// For replies: notify all participants of the parent thread
	// This enables thread-following behavior (once you're in a thread, you see all replies)
	if event.Type == EventReplyPosted {
		for _, participantID := range event.Participants {
			if participantID == sender {
				continue
			}
			b.addPending(participantID, channelSlug, sender)
		}
	}
}

// addPending adds a pending notification for an agent and resets the debounce timer.
func (b *Broker) addPending(agentID, channelSlug, senderID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	p, exists := b.pending[agentID]
	if !exists {
		p = &pendingNudge{
			channelSlug: channelSlug,
			senders:     make(map[string]bool),
		}
		b.pending[agentID] = p
	}
	p.senders[senderID] = true

	// Reset or start timer
	if b.timer != nil {
		b.timer.Stop()
	}
	b.timer = b.clock.NewTimer(b.debounce)
}

// flush sends consolidated nudges and clears state.
func (b *Broker) flush() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.pending) == 0 {
		return
	}

	// Submit nudges for each pending agent
	for agentID, nudge := range b.pending {
		senders := make([]string, 0, len(nudge.senders))
		for s := range nudge.senders {
			senders = append(senders, s)
		}
		sort.Strings(senders)

		var msg string
		if len(senders) == 1 {
			msg = fmt.Sprintf("[%s sent a message in #%s] Use fabric_inbox to check messages.",
				senders[0], nudge.channelSlug)
		} else {
			msg = fmt.Sprintf("[%s sent messages in #%s] Use fabric_inbox to check messages.",
				strings.Join(senders, ", "), nudge.channelSlug)
		}

		if b.cmdSubmitter != nil {
			cmd := command.NewSendToProcessCommand(command.SourceInternal, agentID, msg)
			b.cmdSubmitter.Submit(cmd)
		}
	}

	// Clear state
	b.pending = make(map[string]*pendingNudge)
	b.timer = nil
}

// channelSlugForID returns a channel slug for display. Falls back to "channel" if unknown.
func (b *Broker) channelSlugForID(channelID string) string {
	if b.slugLookup != nil {
		if slug := b.slugLookup.GetChannelSlug(channelID); slug != "" {
			return slug
		}
	}
	return "channel"
}

// containsMention checks if agentID is in the mentions list (case-insensitive).
func containsMention(mentions []string, agentID string) bool {
	lower := strings.ToLower(agentID)
	for _, m := range mentions {
		if strings.ToLower(m) == lower {
			return true
		}
	}
	return false
}

// isNotificationSuppressedChannel returns true if the channel suppresses all notifications.
// The #observer channel is a dedicated private channel between Observer and User.
// If more channels need this behavior, consider adding a SuppressNotifications property
// to the Thread/Channel domain type instead of extending this function.
func isNotificationSuppressedChannel(channelSlug string) bool {
	return channelSlug == domain.SlugObserver
}
