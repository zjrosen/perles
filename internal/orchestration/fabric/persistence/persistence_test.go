package persistence

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
	err := RestoreFabricState(events, threads, deps, subs, acks)
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

	channelIDs, err := RestoreFabricService(tmpDir, threads, deps, subs, acks)
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
