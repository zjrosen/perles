package fabric

import (
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

	broker := NewBroker(BrokerConfig{
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
	time.Sleep(50 * time.Millisecond)

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

	broker := NewBroker(BrokerConfig{
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
	time.Sleep(50 * time.Millisecond)

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

	broker := NewBroker(BrokerConfig{
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
	time.Sleep(50 * time.Millisecond)

	// WORKER.2 should be notified via explicit mention even with mode=none
	cmds := submitter.getCommands()
	require.Len(t, cmds, 1) // Explicit mentions always notify
}

func TestBroker_NoSelfNotification(t *testing.T) {
	subs := repository.NewMemorySubscriptionRepository()
	submitter := &mockCommandSubmitter{}

	broker := NewBroker(BrokerConfig{
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
	time.Sleep(50 * time.Millisecond)

	// No notification - sender is excluded
	cmds := submitter.getCommands()
	assert.Len(t, cmds, 0)
}

func TestBroker_BatchMultipleSenders(t *testing.T) {
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

	time.Sleep(50 * time.Millisecond)

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

func TestBroker_ChannelSlugLookup(t *testing.T) {
	subs := repository.NewMemorySubscriptionRepository()
	submitter := &mockCommandSubmitter{}
	slugLookup := &mockSlugLookup{
		slugs: map[string]string{
			"channel-123": "tasks",
		},
	}

	broker := NewBroker(BrokerConfig{
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
	time.Sleep(50 * time.Millisecond)

	cmds := submitter.getCommands()
	require.Len(t, cmds, 1)

	sendCmd, ok := cmds[0].(*command.SendToProcessCommand)
	require.True(t, ok)
	assert.Contains(t, sendCmd.Content, "#tasks") // Resolved slug
}

func TestBroker_ReplyEventNotification(t *testing.T) {
	subs := repository.NewMemorySubscriptionRepository()
	submitter := &mockCommandSubmitter{}

	broker := NewBroker(BrokerConfig{
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
	time.Sleep(50 * time.Millisecond)

	cmds := submitter.getCommands()
	require.Len(t, cmds, 1)

	sendCmd, ok := cmds[0].(*command.SendToProcessCommand)
	require.True(t, ok)
	assert.Equal(t, "WORKER.1", sendCmd.ProcessID)
}

func TestBroker_IgnoresNonMessageEvents(t *testing.T) {
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
	time.Sleep(50 * time.Millisecond)

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

	broker := NewBroker(BrokerConfig{
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
	time.Sleep(30 * time.Millisecond)

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

	broker := NewBroker(BrokerConfig{
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
	time.Sleep(50 * time.Millisecond)

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

	broker := NewBroker(BrokerConfig{
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
	time.Sleep(50 * time.Millisecond)

	// worker-1 should NOT be notified - observer channel suppresses all notifications
	cmds := submitter.getCommands()
	assert.Len(t, cmds, 0, "observer channel should suppress all subscription notifications")
}

func TestBroker_ObserverChannel_SuppressesThreadParticipantNotifications(t *testing.T) {
	// Test that reply events in #observer do NOT notify thread participants
	subs := repository.NewMemorySubscriptionRepository()
	submitter := &mockCommandSubmitter{}
	slugLookup := &mockSlugLookup{
		slugs: map[string]string{"channel-observer": domain.SlugObserver},
	}

	broker := NewBroker(BrokerConfig{
		CmdSubmitter:  submitter,
		Subscriptions: subs,
		SlugLookup:    slugLookup,
		Debounce:      10 * time.Millisecond,
	})

	broker.Start()
	defer broker.Stop()

	// Observer sends a reply to a thread in #observer
	// The parent thread has participants: user, observer, worker-1 (who observer was discussing)
	event := Event{
		Type:      EventReplyPosted,
		ChannelID: "channel-observer",
		ParentID:  "parent-thread-1",
		Thread: &domain.Thread{
			ID:        "reply-1",
			Type:      domain.ThreadMessage,
			CreatedBy: "observer",
			Content:   "worker-1 seems to be making progress now",
		},
		Mentions:     []string{},
		Participants: []string{"user", "observer", "worker-1"},
	}

	broker.HandleEvent(event)

	// Wait for debounce to flush
	time.Sleep(50 * time.Millisecond)

	// NO participants should be notified - observer channel suppresses all notifications
	cmds := submitter.getCommands()
	assert.Len(t, cmds, 0, "observer channel should suppress all thread participant notifications")
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

	broker := NewBroker(BrokerConfig{
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
	time.Sleep(50 * time.Millisecond)

	cmds := submitter.getCommands()
	require.Len(t, cmds, 1, "#tasks should still send mention notifications")

	sendCmd, ok := cmds[0].(*command.SendToProcessCommand)
	require.True(t, ok)
	assert.Equal(t, "worker-1", sendCmd.ProcessID)
	assert.Contains(t, sendCmd.Content, "#tasks")
}
