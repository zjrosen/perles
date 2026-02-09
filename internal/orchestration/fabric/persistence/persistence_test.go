package persistence

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/fabric"
	"github.com/zjrosen/perles/internal/orchestration/fabric/domain"
	"github.com/zjrosen/perles/internal/orchestration/fabric/repository"
)

func TestEventLogger_WriteAndLoad(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()

	// Create event logger
	logger, err := NewEventLogger(tmpDir)
	require.NoError(t, err)
	defer func() { _ = logger.Close() }()

	// Create test events
	channelThread := &domain.Thread{
		ID:        "ch-1",
		Type:      domain.ThreadChannel,
		Slug:      "general",
		Title:     "General",
		CreatedAt: time.Now(),
		CreatedBy: "SYSTEM",
	}

	messageThread := &domain.Thread{
		ID:        "msg-1",
		Type:      domain.ThreadMessage,
		Content:   "Hello @COORDINATOR",
		Kind:      string(domain.KindInfo),
		CreatedAt: time.Now(),
		CreatedBy: "WORKER.1",
		Mentions:  []string{"COORDINATOR"},
	}

	// Write events
	logger.HandleEvent(fabric.NewChannelCreatedEvent(channelThread))
	logger.HandleEvent(fabric.NewMessagePostedEvent(messageThread, "ch-1", "tasks"))

	// Check stats
	written, errors, lastErr := logger.Stats()
	require.Equal(t, int64(2), written)
	require.Equal(t, int64(0), errors)
	require.Nil(t, lastErr)

	// Close and reload
	require.NoError(t, logger.Close())

	// Load events
	events, err := LoadPersistedEvents(tmpDir)
	require.NoError(t, err)
	require.Len(t, events, 2)

	// Verify first event
	require.Equal(t, currentVersion, events[0].Version)
	require.Equal(t, fabric.EventChannelCreated, events[0].Event.Type)
	require.Equal(t, "ch-1", events[0].Event.Thread.ID)
	require.Equal(t, "general", events[0].Event.Thread.Slug)

	// Verify second event
	require.Equal(t, fabric.EventMessagePosted, events[1].Event.Type)
	require.Equal(t, "msg-1", events[1].Event.Thread.ID)
	require.Equal(t, "ch-1", events[1].Event.ChannelID)
	require.Equal(t, []string{"COORDINATOR"}, events[1].Event.Mentions)
}

func TestEventLogger_ArtifactEvent(t *testing.T) {
	tmpDir := t.TempDir()

	logger, err := NewEventLogger(tmpDir)
	require.NoError(t, err)
	defer func() { _ = logger.Close() }()

	// Create artifact event with StorageURI (path-based, no content stored)
	artifactThread := &domain.Thread{
		ID:         "art-1",
		Type:       domain.ThreadArtifact,
		Name:       "test.js",
		MediaType:  "application/javascript",
		SizeBytes:  27,
		StorageURI: "file:///path/to/test.js",
		Sha256:     "abc123",
		CreatedAt:  time.Now(),
		CreatedBy:  "WORKER.1",
	}

	logger.HandleEvent(fabric.NewArtifactAddedEvent(artifactThread, "msg-1"))

	require.NoError(t, logger.Close())

	// Load and verify - artifact metadata stored, no content
	events, err := LoadPersistedEvents(tmpDir)
	require.NoError(t, err)
	require.Len(t, events, 1)

	require.Equal(t, fabric.EventArtifactAdded, events[0].Event.Type)
	require.Equal(t, "art-1", events[0].Event.Thread.ID)
	require.Equal(t, "file:///path/to/test.js", events[0].Event.Thread.StorageURI)
	require.Equal(t, "abc123", events[0].Event.Thread.Sha256)
}

func TestLoadPersistedEvents_FileNotExists(t *testing.T) {
	tmpDir := t.TempDir()

	events, err := LoadPersistedEvents(tmpDir)
	require.NoError(t, err)
	require.Empty(t, events)
}

func TestLoadPersistedEvents_MalformedLines(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, FabricEventsFile)

	// Write a file with some valid and some invalid lines
	content := `{"version":1,"timestamp":"2024-01-01T00:00:00Z","event":{"type":"channel.created"}}
not valid json
{"version":1,"timestamp":"2024-01-01T00:00:01Z","event":{"type":"message.posted"}}
`
	err := os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(t, err)

	// Load should skip malformed lines
	events, err := LoadPersistedEvents(tmpDir)
	require.NoError(t, err)
	require.Len(t, events, 2) // Two valid events
}

func TestRestoreFabricState(t *testing.T) {
	// Create repositories
	threads := repository.NewMemoryThreadRepository()
	deps := repository.NewMemoryDependencyRepository()
	subs := repository.NewMemorySubscriptionRepository()
	acks := repository.NewMemoryAckRepository(deps, threads, subs)
	participants := repository.NewMemoryParticipantRepository()
	reactions := repository.NewInMemoryReactionRepository()

	// Create persisted events
	now := time.Now()
	events := []PersistedEvent{
		{
			Version:   currentVersion,
			Timestamp: now,
			Event: fabric.Event{
				Type:      fabric.EventChannelCreated,
				Timestamp: now,
				ChannelID: "ch-root",
				Thread: &domain.Thread{
					ID:        "ch-root",
					Type:      domain.ThreadChannel,
					Slug:      "root",
					Title:     "Root",
					CreatedAt: now,
					CreatedBy: "SYSTEM",
				},
			},
		},
		{
			Version:   currentVersion,
			Timestamp: now,
			Event: fabric.Event{
				Type:      fabric.EventChannelCreated,
				Timestamp: now,
				ChannelID: "ch-general",
				Thread: &domain.Thread{
					ID:        "ch-general",
					Type:      domain.ThreadChannel,
					Slug:      "general",
					Title:     "General",
					CreatedAt: now,
					CreatedBy: "SYSTEM",
				},
			},
		},
		{
			Version:   currentVersion,
			Timestamp: now,
			Event: fabric.Event{
				Type:      fabric.EventMessagePosted,
				Timestamp: now,
				ChannelID: "ch-general",
				Thread: &domain.Thread{
					ID:        "msg-1",
					Type:      domain.ThreadMessage,
					Content:   "Hello world",
					Kind:      string(domain.KindInfo),
					CreatedAt: now,
					CreatedBy: "COORDINATOR",
				},
			},
		},
		{
			Version:   currentVersion,
			Timestamp: now,
			Event: fabric.Event{
				Type:      fabric.EventSubscribed,
				Timestamp: now,
				ChannelID: "ch-general",
				AgentID:   "WORKER.1",
				Subscription: &domain.Subscription{
					ChannelID: "ch-general",
					AgentID:   "WORKER.1",
					Mode:      domain.ModeAll,
					CreatedAt: now,
				},
			},
		},
	}

	// Restore state
	err := RestoreFabricState(events, threads, deps, subs, acks, participants, reactions)
	require.NoError(t, err)

	// Verify channels restored
	root, err := threads.GetBySlug("root")
	require.NoError(t, err)
	require.Equal(t, "ch-root", root.ID)
	require.Equal(t, domain.ThreadChannel, root.Type)

	general, err := threads.GetBySlug("general")
	require.NoError(t, err)
	require.Equal(t, "ch-general", general.ID)

	// Verify message restored
	msg, err := threads.Get("msg-1")
	require.NoError(t, err)
	require.Equal(t, "Hello world", msg.Content)
	require.Equal(t, domain.ThreadMessage, msg.Type)

	// Verify dependency created
	childDeps, err := deps.GetChildren("ch-general", nil)
	require.NoError(t, err)
	require.Len(t, childDeps, 1)
	require.Equal(t, "msg-1", childDeps[0].ThreadID)
	require.Equal(t, domain.RelationChildOf, childDeps[0].Relation)

	// Verify subscription restored
	agentSubs, err := subs.ListForAgent("WORKER.1")
	require.NoError(t, err)
	require.Len(t, agentSubs, 1)
	require.Equal(t, "ch-general", agentSubs[0].ChannelID)
	require.Equal(t, domain.ModeAll, agentSubs[0].Mode)
}

func TestRestoreFabricState_ReplayReplyPostedRestoresReplyToDependencyFromParentID(t *testing.T) {
	threads := repository.NewMemoryThreadRepository()
	deps := repository.NewMemoryDependencyRepository()
	subs := repository.NewMemorySubscriptionRepository()
	acks := repository.NewMemoryAckRepository(deps, threads, subs)
	participants := repository.NewMemoryParticipantRepository()
	reactions := repository.NewInMemoryReactionRepository()

	now := time.Now()
	events := []PersistedEvent{
		{
			Version:   currentVersion,
			Timestamp: now,
			Event: fabric.Event{
				Type:      fabric.EventChannelCreated,
				Timestamp: now,
				ChannelID: "ch-general",
				Thread: &domain.Thread{
					ID:        "ch-general",
					Type:      domain.ThreadChannel,
					Slug:      "general",
					Title:     "General",
					CreatedAt: now,
					CreatedBy: "SYSTEM",
				},
			},
		},
		{
			Version:   currentVersion,
			Timestamp: now.Add(time.Second),
			Event: fabric.Event{
				Type:      fabric.EventMessagePosted,
				Timestamp: now.Add(time.Second),
				ChannelID: "ch-general",
				Thread: &domain.Thread{
					ID:        "msg-1",
					Type:      domain.ThreadMessage,
					Content:   "Root message",
					Kind:      string(domain.KindInfo),
					CreatedAt: now.Add(time.Second),
					CreatedBy: "COORDINATOR",
				},
			},
		},
		{
			Version:   currentVersion,
			Timestamp: now.Add(2 * time.Second),
			Event: fabric.Event{
				Type:      fabric.EventReplyPosted,
				Timestamp: now.Add(2 * time.Second),
				ChannelID: "ch-general",
				ParentID:  "msg-1",
				Thread: &domain.Thread{
					ID:        "reply-1",
					Type:      domain.ThreadMessage,
					Content:   "Reply message",
					Kind:      string(domain.KindResponse),
					CreatedAt: now.Add(2 * time.Second),
					CreatedBy: "worker-1",
				},
			},
		},
	}

	err := RestoreFabricState(events, threads, deps, subs, acks, participants, reactions)
	require.NoError(t, err)

	relation := domain.RelationReplyTo
	replyDeps, err := deps.GetChildren("msg-1", &relation)
	require.NoError(t, err)
	require.Len(t, replyDeps, 1)
	require.Equal(t, "reply-1", replyDeps[0].ThreadID)
	require.Equal(t, "msg-1", replyDeps[0].DependsOnID)
	require.Equal(t, domain.RelationReplyTo, replyDeps[0].Relation)
}

func TestRestoreFabricState_ReplayReplyPostedDuplicateEventIsIdempotent(t *testing.T) {
	threads := repository.NewMemoryThreadRepository()
	deps := repository.NewMemoryDependencyRepository()
	subs := repository.NewMemorySubscriptionRepository()
	acks := repository.NewMemoryAckRepository(deps, threads, subs)
	participants := repository.NewMemoryParticipantRepository()
	reactions := repository.NewInMemoryReactionRepository()

	now := time.Now()
	replyEvent := fabric.Event{
		Type:      fabric.EventReplyPosted,
		Timestamp: now.Add(2 * time.Second),
		ChannelID: "ch-general",
		ParentID:  "msg-1",
		Thread: &domain.Thread{
			ID:        "reply-1",
			Type:      domain.ThreadMessage,
			Content:   "Reply message",
			Kind:      string(domain.KindResponse),
			CreatedAt: now.Add(2 * time.Second),
			CreatedBy: "worker-1",
		},
	}

	events := []PersistedEvent{
		{
			Version:   currentVersion,
			Timestamp: now,
			Event: fabric.Event{
				Type:      fabric.EventChannelCreated,
				Timestamp: now,
				ChannelID: "ch-general",
				Thread: &domain.Thread{
					ID:        "ch-general",
					Type:      domain.ThreadChannel,
					Slug:      "general",
					Title:     "General",
					CreatedAt: now,
					CreatedBy: "SYSTEM",
				},
			},
		},
		{
			Version:   currentVersion,
			Timestamp: now.Add(time.Second),
			Event: fabric.Event{
				Type:      fabric.EventMessagePosted,
				Timestamp: now.Add(time.Second),
				ChannelID: "ch-general",
				Thread: &domain.Thread{
					ID:        "msg-1",
					Type:      domain.ThreadMessage,
					Content:   "Root message",
					Kind:      string(domain.KindInfo),
					CreatedAt: now.Add(time.Second),
					CreatedBy: "COORDINATOR",
				},
			},
		},
		{
			Version:   currentVersion,
			Timestamp: now.Add(2 * time.Second),
			Event:     replyEvent,
		},
		{
			Version:   currentVersion,
			Timestamp: now.Add(3 * time.Second),
			Event:     replyEvent,
		},
	}

	err := RestoreFabricState(events, threads, deps, subs, acks, participants, reactions)
	require.NoError(t, err)

	relation := domain.RelationReplyTo
	replyDeps, err := deps.GetChildren("msg-1", &relation)
	require.NoError(t, err)
	require.Len(t, replyDeps, 1)

	threadsList, err := threads.List(repository.ListOptions{Type: ptrThreadType(domain.ThreadMessage)})
	require.NoError(t, err)
	require.Len(t, threadsList, 2) // root message + single reply
}

func TestRestoreFabricState_ReplayReplyPostedMissingParentIDSkipsWithDiagnostic(t *testing.T) {
	logPath, closeLogger := initTestLogger(t)
	defer closeLogger()

	threads := repository.NewMemoryThreadRepository()
	deps := repository.NewMemoryDependencyRepository()
	subs := repository.NewMemorySubscriptionRepository()
	acks := repository.NewMemoryAckRepository(deps, threads, subs)
	participants := repository.NewMemoryParticipantRepository()
	reactions := repository.NewInMemoryReactionRepository()

	now := time.Now()
	events := []PersistedEvent{
		{
			Version:   currentVersion,
			Timestamp: now,
			Event: fabric.Event{
				Type:      fabric.EventChannelCreated,
				Timestamp: now,
				ChannelID: "ch-general",
				Thread: &domain.Thread{
					ID:        "ch-general",
					Type:      domain.ThreadChannel,
					Slug:      "general",
					Title:     "General",
					CreatedAt: now,
					CreatedBy: "SYSTEM",
				},
			},
		},
		{
			Version:   currentVersion,
			Timestamp: now.Add(time.Second),
			Event: fabric.Event{
				Type:      fabric.EventMessagePosted,
				Timestamp: now.Add(time.Second),
				ChannelID: "ch-general",
				Thread: &domain.Thread{
					ID:        "msg-1",
					Type:      domain.ThreadMessage,
					Content:   "Root message",
					Kind:      string(domain.KindInfo),
					CreatedAt: now.Add(time.Second),
					CreatedBy: "COORDINATOR",
				},
			},
		},
		{
			Version:   currentVersion,
			Timestamp: now.Add(2 * time.Second),
			Event: fabric.Event{
				Type:      fabric.EventReplyPosted,
				Timestamp: now.Add(2 * time.Second),
				ChannelID: "ch-general",
				Thread: &domain.Thread{
					ID:        "reply-1",
					Type:      domain.ThreadMessage,
					Content:   "Reply message",
					Kind:      string(domain.KindResponse),
					CreatedAt: now.Add(2 * time.Second),
					CreatedBy: "worker-1",
				},
			},
		},
	}

	err := RestoreFabricState(events, threads, deps, subs, acks, participants, reactions)
	require.NoError(t, err)

	relation := domain.RelationReplyTo
	replyDeps, err := deps.GetChildren("msg-1", &relation)
	require.NoError(t, err)
	require.Len(t, replyDeps, 0)
	requireRestoreSkipDiagnostic(t, logPath, replyRestoreSkipReasonMissingParentID)
}

func TestRestoreFabricState_ReplayReplyPostedUnresolvedParentSkipsWithDiagnostic(t *testing.T) {
	logPath, closeLogger := initTestLogger(t)
	defer closeLogger()

	threads := repository.NewMemoryThreadRepository()
	deps := repository.NewMemoryDependencyRepository()
	subs := repository.NewMemorySubscriptionRepository()
	acks := repository.NewMemoryAckRepository(deps, threads, subs)
	participants := repository.NewMemoryParticipantRepository()
	reactions := repository.NewInMemoryReactionRepository()

	now := time.Now()
	events := []PersistedEvent{
		{
			Version:   currentVersion,
			Timestamp: now,
			Event: fabric.Event{
				Type:      fabric.EventReplyPosted,
				Timestamp: now,
				ChannelID: "ch-general",
				ParentID:  "msg-missing",
				Thread: &domain.Thread{
					ID:        "reply-1",
					Type:      domain.ThreadMessage,
					Content:   "Reply message",
					Kind:      string(domain.KindResponse),
					CreatedAt: now,
					CreatedBy: "worker-1",
				},
			},
		},
	}

	err := RestoreFabricState(events, threads, deps, subs, acks, participants, reactions)
	require.NoError(t, err)

	relation := domain.RelationReplyTo
	replyDeps, err := deps.GetChildren("msg-missing", &relation)
	require.NoError(t, err)
	require.Len(t, replyDeps, 0)
	requireRestoreSkipDiagnostic(t, logPath, replyRestoreSkipReasonUnresolvedParent)
}

func TestRestoreFabricState_Reactions(t *testing.T) {
	// Create repositories
	threads := repository.NewMemoryThreadRepository()
	deps := repository.NewMemoryDependencyRepository()
	subs := repository.NewMemorySubscriptionRepository()
	acks := repository.NewMemoryAckRepository(deps, threads, subs)
	participants := repository.NewMemoryParticipantRepository()
	reactions := repository.NewInMemoryReactionRepository()

	now := time.Now()

	// Create events including a message and reactions
	events := []PersistedEvent{
		{
			Version:   currentVersion,
			Timestamp: now,
			Event: fabric.Event{
				Type:      fabric.EventChannelCreated,
				Timestamp: now,
				ChannelID: "ch-general",
				Thread: &domain.Thread{
					ID:        "ch-general",
					Type:      domain.ThreadChannel,
					Slug:      "general",
					Title:     "General",
					CreatedAt: now,
					CreatedBy: "SYSTEM",
				},
			},
		},
		{
			Version:   currentVersion,
			Timestamp: now,
			Event: fabric.Event{
				Type:      fabric.EventMessagePosted,
				Timestamp: now,
				ChannelID: "ch-general",
				Thread: &domain.Thread{
					ID:        "msg-1",
					Type:      domain.ThreadMessage,
					Content:   "Hello world",
					Kind:      string(domain.KindInfo),
					CreatedAt: now,
					CreatedBy: "COORDINATOR",
				},
			},
		},
		{
			Version:   currentVersion,
			Timestamp: now,
			Event: fabric.Event{
				Type:      fabric.EventReactionAdded,
				Timestamp: now,
				ChannelID: "ch-general",
				AgentID:   "worker-1",
				Reaction: &domain.Reaction{
					ThreadID:  "msg-1",
					AgentID:   "worker-1",
					Emoji:     "👍",
					CreatedAt: now,
				},
			},
		},
		{
			Version:   currentVersion,
			Timestamp: now,
			Event: fabric.Event{
				Type:      fabric.EventReactionAdded,
				Timestamp: now,
				ChannelID: "ch-general",
				AgentID:   "worker-2",
				Reaction: &domain.Reaction{
					ThreadID:  "msg-1",
					AgentID:   "worker-2",
					Emoji:     "👍",
					CreatedAt: now,
				},
			},
		},
		{
			Version:   currentVersion,
			Timestamp: now,
			Event: fabric.Event{
				Type:      fabric.EventReactionAdded,
				Timestamp: now,
				ChannelID: "ch-general",
				AgentID:   "coordinator",
				Reaction: &domain.Reaction{
					ThreadID:  "msg-1",
					AgentID:   "coordinator",
					Emoji:     "✅",
					CreatedAt: now,
				},
			},
		},
	}

	// Restore state
	err := RestoreFabricState(events, threads, deps, subs, acks, participants, reactions)
	require.NoError(t, err)

	// Verify reactions restored
	reactionList, err := reactions.ListForThread("msg-1")
	require.NoError(t, err)
	require.Len(t, reactionList, 3)

	// Verify reaction summary
	summary, err := reactions.GetSummary("msg-1")
	require.NoError(t, err)
	require.Len(t, summary, 2) // 👍 and ✅

	// Find the 👍 summary
	var thumbsUp *domain.ReactionSummary
	for i := range summary {
		if summary[i].Emoji == "👍" {
			thumbsUp = &summary[i]
			break
		}
	}
	require.NotNil(t, thumbsUp)
	require.Equal(t, 2, thumbsUp.Count)
	require.Contains(t, thumbsUp.AgentIDs, "worker-1")
	require.Contains(t, thumbsUp.AgentIDs, "worker-2")
}

func initTestLogger(t *testing.T) (string, func()) {
	t.Helper()

	logPath := filepath.Join(t.TempDir(), "restore.log")
	cleanup, err := log.InitWithTeaLog(logPath, "persistence-test")
	require.NoError(t, err)
	log.SetEnabled(true)

	return logPath, cleanup
}

func requireRestoreSkipDiagnostic(t *testing.T, logPath, reason string) {
	t.Helper()

	data, err := os.ReadFile(logPath)
	require.NoError(t, err)

	content := string(data)
	require.Contains(t, content, "diagnostic="+replyRestoreSkippedDiagnosticKey)
	require.Contains(t, content, "reason="+reason)
}

func ptrThreadType(tt domain.ThreadType) *domain.ThreadType {
	return &tt
}

func TestRestoreFabricState_ReactionRemoved(t *testing.T) {
	// Create repositories
	threads := repository.NewMemoryThreadRepository()
	deps := repository.NewMemoryDependencyRepository()
	subs := repository.NewMemorySubscriptionRepository()
	acks := repository.NewMemoryAckRepository(deps, threads, subs)
	participants := repository.NewMemoryParticipantRepository()
	reactions := repository.NewInMemoryReactionRepository()

	now := time.Now()

	// Create events: add reaction then remove it
	events := []PersistedEvent{
		{
			Version:   currentVersion,
			Timestamp: now,
			Event: fabric.Event{
				Type:      fabric.EventReactionAdded,
				Timestamp: now,
				ChannelID: "ch-general",
				AgentID:   "worker-1",
				Reaction: &domain.Reaction{
					ThreadID:  "msg-1",
					AgentID:   "worker-1",
					Emoji:     "👍",
					CreatedAt: now,
				},
			},
		},
		{
			Version:   currentVersion,
			Timestamp: now.Add(time.Second),
			Event: fabric.Event{
				Type:      fabric.EventReactionRemoved,
				Timestamp: now.Add(time.Second),
				ChannelID: "ch-general",
				AgentID:   "worker-1",
				Reaction: &domain.Reaction{
					ThreadID: "msg-1",
					AgentID:  "worker-1",
					Emoji:    "👍",
				},
			},
		},
	}

	// Restore state
	err := RestoreFabricState(events, threads, deps, subs, acks, participants, reactions)
	require.NoError(t, err)

	// Verify reaction was removed
	reactionList, err := reactions.ListForThread("msg-1")
	require.NoError(t, err)
	require.Len(t, reactionList, 0)
}

func TestRestoreFabricService(t *testing.T) {
	tmpDir := t.TempDir()

	// Create and populate a logger with full session state
	logger, err := NewEventLogger(tmpDir)
	require.NoError(t, err)

	now := time.Now()

	// Simulate session initialization (fixed channels)
	for i, ch := range domain.FixedChannels() {
		ch.ID = "ch-" + ch.Slug
		ch.CreatedAt = now
		ch.CreatedBy = "SYSTEM"
		ch.Seq = int64(i + 1)
		logger.HandleEvent(fabric.NewChannelCreatedEvent(&ch))
	}

	// Add a message
	msg := &domain.Thread{
		ID:        "msg-1",
		Type:      domain.ThreadMessage,
		Content:   "Task: Implement login",
		Kind:      string(domain.KindRequest),
		CreatedAt: now,
		CreatedBy: "COORDINATOR",
		Mentions:  []string{"WORKER.1"},
		Seq:       10,
	}
	logger.HandleEvent(fabric.NewMessagePostedEvent(msg, "ch-tasks", "tasks"))

	require.NoError(t, logger.Close())

	// Restore into fresh repositories
	threads := repository.NewMemoryThreadRepository()
	deps := repository.NewMemoryDependencyRepository()
	subs := repository.NewMemorySubscriptionRepository()
	acks := repository.NewMemoryAckRepository(deps, threads, subs)
	participants := repository.NewMemoryParticipantRepository()
	reactions := repository.NewInMemoryReactionRepository()

	channelIDs, err := RestoreFabricService(tmpDir, threads, deps, subs, acks, participants, reactions)
	require.NoError(t, err)

	// Verify channel IDs returned
	require.Equal(t, "ch-root", channelIDs["root"])
	require.Equal(t, "ch-system", channelIDs["system"])
	require.Equal(t, "ch-tasks", channelIDs["tasks"])
	require.Equal(t, "ch-planning", channelIDs["planning"])
	require.Equal(t, "ch-general", channelIDs["general"])

	// Verify message restored with dependency
	msg2, err := threads.Get("msg-1")
	require.NoError(t, err)
	require.Equal(t, "Task: Implement login", msg2.Content)

	taskChildren, err := deps.GetChildren("ch-tasks", nil)
	require.NoError(t, err)
	require.Len(t, taskChildren, 1)
	require.Equal(t, "msg-1", taskChildren[0].ThreadID)
}

func TestHasPersistedFabricState(t *testing.T) {
	tmpDir := t.TempDir()

	// No file yet
	require.False(t, HasPersistedFabricState(tmpDir))

	// Empty file
	filePath := filepath.Join(tmpDir, FabricEventsFile)
	err := os.WriteFile(filePath, []byte{}, 0644)
	require.NoError(t, err)
	require.False(t, HasPersistedFabricState(tmpDir))

	// Non-empty file
	err = os.WriteFile(filePath, []byte(`{"version":1}`), 0644)
	require.NoError(t, err)
	require.True(t, HasPersistedFabricState(tmpDir))
}

func TestChainHandler(t *testing.T) {
	var calls []string

	h1 := func(e fabric.Event) { calls = append(calls, "h1:"+string(e.Type)) }
	h2 := func(e fabric.Event) { calls = append(calls, "h2:"+string(e.Type)) }

	chained := ChainHandler(h1, h2)

	event := fabric.Event{Type: fabric.EventMessagePosted}
	chained(event)

	require.Equal(t, []string{"h1:message.posted", "h2:message.posted"}, calls)
}

func TestChainHandler_NilHandlers(t *testing.T) {
	var calls []string
	h1 := func(e fabric.Event) { calls = append(calls, "h1") }

	// Should not panic on nil handlers
	chained := ChainHandler(nil, h1, nil)

	event := fabric.Event{Type: fabric.EventMessagePosted}
	chained(event)

	require.Equal(t, []string{"h1"}, calls)
}

// === RestoreVolatileState tests ===

func TestRestoreVolatileState_SkipsThreadsAndDeps(t *testing.T) {
	// Create repositories for volatile state
	threads := repository.NewMemoryThreadRepository()
	deps := repository.NewMemoryDependencyRepository()
	subs := repository.NewMemorySubscriptionRepository()
	acks := repository.NewMemoryAckRepository(deps, threads, subs)
	participants := repository.NewMemoryParticipantRepository()
	reactions := repository.NewInMemoryReactionRepository()

	// Create events with both graph and volatile data
	now := time.Now()
	events := []PersistedEvent{
		// Graph events (should be skipped)
		{
			Version:   currentVersion,
			Timestamp: now,
			Event: fabric.Event{
				Type:      fabric.EventChannelCreated,
				Timestamp: now,
				ChannelID: "ch-general",
				Thread: &domain.Thread{
					ID: "ch-general", Type: domain.ThreadChannel, Slug: "general",
					Title: "General", CreatedAt: now, CreatedBy: "SYSTEM",
				},
			},
		},
		{
			Version:   currentVersion,
			Timestamp: now,
			Event: fabric.Event{
				Type:      fabric.EventMessagePosted,
				Timestamp: now,
				ChannelID: "ch-general",
				Thread: &domain.Thread{
					ID: "msg-1", Type: domain.ThreadMessage, Content: "Hello",
					Kind: string(domain.KindInfo), CreatedAt: now, CreatedBy: "worker-1",
				},
			},
		},
		// Volatile events (should be restored)
		{
			Version:   currentVersion,
			Timestamp: now,
			Event: fabric.Event{
				Type:      fabric.EventSubscribed,
				Timestamp: now,
				ChannelID: "ch-general",
				AgentID:   "worker-1",
				Subscription: &domain.Subscription{
					ChannelID: "ch-general",
					AgentID:   "worker-1",
					Mode:      domain.ModeAll,
					CreatedAt: now,
				},
			},
		},
		{
			Version:   currentVersion,
			Timestamp: now,
			Event: fabric.Event{
				Type:      fabric.EventParticipantJoined,
				Timestamp: now,
				ChannelID: "ch-general",
				AgentID:   "worker-1",
				Participant: &domain.Participant{
					AgentID:  "worker-1",
					Role:     domain.RoleWorker,
					JoinedAt: now,
				},
			},
		},
	}

	// Restore only volatile state
	err := RestoreVolatileState(events, subs, acks, participants, reactions)
	require.NoError(t, err)

	// Verify threads were NOT created (skipped)
	allThreads, err := threads.List(repository.ListOptions{})
	require.NoError(t, err)
	require.Len(t, allThreads, 0, "threads should not be restored by RestoreVolatileState")

	// Verify subscription WAS restored
	agentSubs, err := subs.ListForAgent("worker-1")
	require.NoError(t, err)
	require.Len(t, agentSubs, 1, "subscriptions should be restored")
	require.Equal(t, "ch-general", agentSubs[0].ChannelID)

	// Verify participant WAS restored
	participant, err := participants.Get("worker-1")
	require.NoError(t, err)
	require.NotNil(t, participant, "participants should be restored")
	require.Equal(t, "worker-1", participant.AgentID)
}
