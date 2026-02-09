package persistence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/orchestration/fabric"
	"github.com/zjrosen/perles/internal/orchestration/fabric/domain"
	"github.com/zjrosen/perles/internal/orchestration/fabric/repository"
)

// --- helpers ---

// newMemoryRepos creates a set of fresh in-memory Fabric repositories for testing.
func newMemoryRepos() (repository.ThreadRepository, repository.DependencyRepository) {
	return repository.NewMemoryThreadRepository(), repository.NewMemoryDependencyRepository()
}

// writeJSONL writes persisted events to a JSONL file at the given directory.
func writeJSONL(t *testing.T, dir string, events []PersistedEvent) string {
	t.Helper()
	filePath := filepath.Join(dir, FabricEventsFile)
	f, err := os.Create(filePath)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	enc := json.NewEncoder(f)
	for _, pe := range events {
		require.NoError(t, enc.Encode(pe))
	}
	return filePath
}

// makeChannelEvent creates a PersistedEvent for a channel creation.
func makeChannelEvent(id, slug string, ts time.Time) PersistedEvent {
	return PersistedEvent{
		Version:   currentVersion,
		Timestamp: ts,
		Event: fabric.Event{
			Type:      fabric.EventChannelCreated,
			Timestamp: ts,
			ChannelID: id,
			Thread: &domain.Thread{
				ID:        id,
				Type:      domain.ThreadChannel,
				Slug:      slug,
				Title:     slug,
				CreatedAt: ts,
				CreatedBy: "SYSTEM",
			},
		},
	}
}

// makeMessageEvent creates a PersistedEvent for a message post.
func makeMessageEvent(id, channelID string, ts time.Time) PersistedEvent {
	return PersistedEvent{
		Version:   currentVersion,
		Timestamp: ts,
		Event: fabric.Event{
			Type:      fabric.EventMessagePosted,
			Timestamp: ts,
			ChannelID: channelID,
			Thread: &domain.Thread{
				ID:        id,
				Type:      domain.ThreadMessage,
				Content:   "message " + id,
				Kind:      string(domain.KindInfo),
				CreatedAt: ts,
				CreatedBy: "worker-1",
			},
		},
	}
}

// makeReplyEvent creates a PersistedEvent for a reply.
func makeReplyEvent(id, channelID, parentID string, ts time.Time) PersistedEvent {
	return PersistedEvent{
		Version:   currentVersion,
		Timestamp: ts,
		Event: fabric.Event{
			Type:      fabric.EventReplyPosted,
			Timestamp: ts,
			ChannelID: channelID,
			ParentID:  parentID,
			Thread: &domain.Thread{
				ID:        id,
				Type:      domain.ThreadMessage,
				Content:   "reply " + id,
				Kind:      string(domain.KindResponse),
				CreatedAt: ts,
				CreatedBy: "worker-2",
			},
		},
	}
}

// makeArtifactEvent creates a PersistedEvent for an artifact.
func makeArtifactEvent(id, targetID string, ts time.Time) PersistedEvent {
	return PersistedEvent{
		Version:   currentVersion,
		Timestamp: ts,
		Event: fabric.Event{
			Type:      fabric.EventArtifactAdded,
			Timestamp: ts,
			ChannelID: targetID,
			Thread: &domain.Thread{
				ID:         id,
				Type:       domain.ThreadArtifact,
				Name:       "test.txt",
				MediaType:  "text/plain",
				StorageURI: "file:///tmp/test.txt",
				CreatedAt:  ts,
				CreatedBy:  "worker-1",
			},
		},
	}
}

// makeSubscriptionEvent creates a non-thread/dep event (subscription).
func makeSubscriptionEvent(channelID, agentID string, ts time.Time) PersistedEvent {
	return PersistedEvent{
		Version:   currentVersion,
		Timestamp: ts,
		Event: fabric.Event{
			Type:      fabric.EventSubscribed,
			Timestamp: ts,
			ChannelID: channelID,
			AgentID:   agentID,
			Subscription: &domain.Subscription{
				ChannelID: channelID,
				AgentID:   agentID,
				Mode:      domain.ModeAll,
				CreatedAt: ts,
			},
		},
	}
}

// sampleEvents returns a typical JSONL event sequence: 2 channels, 2 messages, 1 reply.
func sampleEvents() []PersistedEvent {
	now := time.Now()
	return []PersistedEvent{
		makeChannelEvent("ch-root", "root", now),
		makeChannelEvent("ch-general", "general", now.Add(time.Second)),
		makeMessageEvent("msg-1", "ch-general", now.Add(2*time.Second)),
		makeMessageEvent("msg-2", "ch-general", now.Add(3*time.Second)),
		makeReplyEvent("reply-1", "ch-general", "msg-1", now.Add(4*time.Second)),
	}
}

// populateSQLite creates threads and deps in the given repos matching sampleEvents.
func populateSQLite(t *testing.T, threads repository.ThreadRepository, deps repository.DependencyRepository) {
	t.Helper()
	now := time.Now()

	for _, ch := range []struct{ id, slug string }{
		{"ch-root", "root"},
		{"ch-general", "general"},
	} {
		_, err := threads.Create(domain.Thread{
			ID: ch.id, Type: domain.ThreadChannel, Slug: ch.slug,
			Title: ch.slug, CreatedAt: now, CreatedBy: "SYSTEM",
		})
		require.NoError(t, err)
	}

	for _, msg := range []struct{ id, channelID string }{
		{"msg-1", "ch-general"},
		{"msg-2", "ch-general"},
	} {
		_, err := threads.Create(domain.Thread{
			ID: msg.id, Type: domain.ThreadMessage, Content: "message " + msg.id,
			Kind: string(domain.KindInfo), CreatedAt: now, CreatedBy: "worker-1",
		})
		require.NoError(t, err)
		require.NoError(t, deps.Add(domain.NewDependency(msg.id, msg.channelID, domain.RelationChildOf)))
	}

	// Reply
	_, err := threads.Create(domain.Thread{
		ID: "reply-1", Type: domain.ThreadMessage, Content: "reply reply-1",
		Kind: string(domain.KindResponse), CreatedAt: now, CreatedBy: "worker-2",
	})
	require.NoError(t, err)
	require.NoError(t, deps.Add(domain.NewDependency("reply-1", "msg-1", domain.RelationReplyTo)))
}

// --- DetectState tests ---

func TestDetectState_JSONLOnly(t *testing.T) {
	tmpDir := t.TempDir()
	threads, deps := newMemoryRepos()

	jsonlPath := writeJSONL(t, tmpDir, sampleEvents())

	result, err := DetectState(threads, deps, jsonlPath)
	require.NoError(t, err)
	require.Equal(t, StateJSONLOnly, result.State)
	require.Equal(t, 5, result.ThreadCountJSONL, "5 unique threads in sample events")
	require.Equal(t, 3, result.DepCountJSONL, "2 child_of + 1 reply_to")
	require.Equal(t, 0, result.ThreadCountSQLite)
	require.Equal(t, 0, result.DepCountSQLite)
	require.NotEmpty(t, result.Diagnostics)
}

func TestDetectState_SQLiteOnly(t *testing.T) {
	tmpDir := t.TempDir()
	threads, deps := newMemoryRepos()

	// No JSONL file written
	jsonlPath := filepath.Join(tmpDir, FabricEventsFile)

	// Populate SQLite
	populateSQLite(t, threads, deps)

	result, err := DetectState(threads, deps, jsonlPath)
	require.NoError(t, err)
	require.Equal(t, StateSQLiteOnly, result.State)
	require.Equal(t, 0, result.ThreadCountJSONL)
	require.Equal(t, 0, result.DepCountJSONL)
	require.Equal(t, 5, result.ThreadCountSQLite)
	require.Greater(t, result.DepCountSQLite, 0)
	require.NotEmpty(t, result.Diagnostics)
}

func TestDetectState_BothMatch(t *testing.T) {
	tmpDir := t.TempDir()
	threads, deps := newMemoryRepos()

	// Write JSONL
	jsonlPath := writeJSONL(t, tmpDir, sampleEvents())

	// Populate SQLite with matching data
	populateSQLite(t, threads, deps)

	result, err := DetectState(threads, deps, jsonlPath)
	require.NoError(t, err)
	require.Equal(t, StateBothMatch, result.State)
	require.Equal(t, result.ThreadCountJSONL, result.ThreadCountSQLite)
	require.Equal(t, result.DepCountJSONL, result.DepCountSQLite)
	require.NotEmpty(t, result.Diagnostics)
}

func TestDetectState_BothMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	threads, deps := newMemoryRepos()

	// Write JSONL with 5 threads
	jsonlPath := writeJSONL(t, tmpDir, sampleEvents())

	// Populate SQLite with only 2 threads (channels only, missing messages)
	now := time.Now()
	_, err := threads.Create(domain.Thread{
		ID: "ch-root", Type: domain.ThreadChannel, Slug: "root",
		Title: "root", CreatedAt: now, CreatedBy: "SYSTEM",
	})
	require.NoError(t, err)
	_, err = threads.Create(domain.Thread{
		ID: "ch-general", Type: domain.ThreadChannel, Slug: "general",
		Title: "general", CreatedAt: now, CreatedBy: "SYSTEM",
	})
	require.NoError(t, err)

	result, err := DetectState(threads, deps, jsonlPath)
	require.NoError(t, err)
	require.Equal(t, StateBothMismatch, result.State)
	require.Equal(t, 5, result.ThreadCountJSONL)
	require.Equal(t, 2, result.ThreadCountSQLite)
	require.NotEmpty(t, result.Diagnostics)
}

func TestDetectState_EmptyJSONLFile(t *testing.T) {
	tmpDir := t.TempDir()
	threads, deps := newMemoryRepos()

	// Write empty JSONL file
	jsonlPath := filepath.Join(tmpDir, FabricEventsFile)
	require.NoError(t, os.WriteFile(jsonlPath, []byte{}, 0644))

	result, err := DetectState(threads, deps, jsonlPath)
	require.NoError(t, err)
	// Empty JSONL + empty SQLite = both match (clean slate)
	require.Equal(t, StateBothMatch, result.State)
}

func TestDetectState_JSONLOnlyNonThreadEvents(t *testing.T) {
	tmpDir := t.TempDir()
	threads, deps := newMemoryRepos()

	// Write JSONL with only subscription events (no threads/deps)
	now := time.Now()
	events := []PersistedEvent{
		makeSubscriptionEvent("ch-general", "worker-1", now),
		makeSubscriptionEvent("ch-general", "worker-2", now.Add(time.Second)),
	}
	jsonlPath := writeJSONL(t, tmpDir, events)

	result, err := DetectState(threads, deps, jsonlPath)
	require.NoError(t, err)
	// JSONL has content but 0 threads/deps, SQLite has 0 → counts match → both_match
	require.Equal(t, StateBothMatch, result.State)
	require.Equal(t, 0, result.ThreadCountJSONL)
	require.Equal(t, 0, result.DepCountJSONL)
}

// --- Backfill tests ---

func TestBackfill_PopulatesSQLite(t *testing.T) {
	tmpDir := t.TempDir()
	threads, deps := newMemoryRepos()

	jsonlPath := writeJSONL(t, tmpDir, sampleEvents())

	result := &ReconciliationResult{State: StateJSONLOnly}
	err := Backfill(result, threads, deps, jsonlPath)
	require.NoError(t, err)

	// Verify threads inserted
	allThreads, err := threads.List(repository.ListOptions{})
	require.NoError(t, err)
	require.Equal(t, 5, len(allThreads), "5 threads: 2 channels + 2 messages + 1 reply")

	// Verify deps inserted: child_of for msg-1, msg-2 + reply_to for reply-1
	childOfRel := domain.RelationChildOf
	children, err := deps.GetChildren("ch-general", &childOfRel)
	require.NoError(t, err)
	require.Equal(t, 2, len(children), "2 messages are children of ch-general")

	replyToRel := domain.RelationReplyTo
	replies, err := deps.GetChildren("msg-1", &replyToRel)
	require.NoError(t, err)
	require.Equal(t, 1, len(replies), "1 reply to msg-1")

	// Verify SQLite counts updated in result
	require.Equal(t, 5, result.ThreadCountSQLite)
	require.Equal(t, 3, result.DepCountSQLite)
}

func TestBackfill_IsIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	threads, deps := newMemoryRepos()

	jsonlPath := writeJSONL(t, tmpDir, sampleEvents())

	// First backfill
	result1 := &ReconciliationResult{State: StateJSONLOnly}
	err := Backfill(result1, threads, deps, jsonlPath)
	require.NoError(t, err)

	allThreads1, _ := threads.List(repository.ListOptions{})
	count1 := len(allThreads1)

	// Remove marker to allow re-run
	markerPath := migrationMarkerPath(jsonlPath)
	require.NoError(t, os.Remove(markerPath))

	// Second backfill
	result2 := &ReconciliationResult{State: StateJSONLOnly}
	err = Backfill(result2, threads, deps, jsonlPath)
	require.NoError(t, err)

	allThreads2, _ := threads.List(repository.ListOptions{})
	count2 := len(allThreads2)

	require.Equal(t, count1, count2, "Backfill is idempotent — same thread count after second run")
	require.Equal(t, result1.DepCountSQLite, result2.DepCountSQLite, "Same dep count after second run")
}

func TestBackfill_OutOfOrderEvents(t *testing.T) {
	tmpDir := t.TempDir()
	threads, deps := newMemoryRepos()

	now := time.Now()
	// Reply before parent message (out of order)
	events := []PersistedEvent{
		makeChannelEvent("ch-general", "general", now),
		makeReplyEvent("reply-1", "ch-general", "msg-1", now.Add(time.Second)),
		makeMessageEvent("msg-1", "ch-general", now.Add(2*time.Second)),
	}
	jsonlPath := writeJSONL(t, tmpDir, events)

	result := &ReconciliationResult{State: StateJSONLOnly}
	err := Backfill(result, threads, deps, jsonlPath)
	require.NoError(t, err)

	// All threads should be inserted regardless of order
	allThreads, err := threads.List(repository.ListOptions{})
	require.NoError(t, err)
	require.Equal(t, 3, len(allThreads))

	// Reply dep should still be created (reply_to for reply-1 → msg-1)
	// Note: In backfill, deps are added unconditionally without parent resolution checks
	replyToRel := domain.RelationReplyTo
	replyDeps, err := deps.GetChildren("msg-1", &replyToRel)
	require.NoError(t, err)
	require.Equal(t, 1, len(replyDeps))
}

func TestBackfill_MalformedJSONLLines(t *testing.T) {
	tmpDir := t.TempDir()
	threads, deps := newMemoryRepos()

	// Write JSONL with valid and invalid lines
	now := time.Now()
	filePath := filepath.Join(tmpDir, FabricEventsFile)
	f, err := os.Create(filePath)
	require.NoError(t, err)

	enc := json.NewEncoder(f)
	require.NoError(t, enc.Encode(makeChannelEvent("ch-root", "root", now)))

	_, err = f.WriteString("not valid json\n")
	require.NoError(t, err)

	require.NoError(t, enc.Encode(makeMessageEvent("msg-1", "ch-root", now.Add(time.Second))))
	require.NoError(t, f.Close())

	result := &ReconciliationResult{State: StateJSONLOnly}
	err = Backfill(result, threads, deps, filePath)
	require.NoError(t, err)

	// Should have skipped malformed line and still inserted 2 threads
	allThreads, err := threads.List(repository.ListOptions{})
	require.NoError(t, err)
	require.Equal(t, 2, len(allThreads))
}

func TestBackfill_WritesMigrationMarker(t *testing.T) {
	tmpDir := t.TempDir()
	threads, deps := newMemoryRepos()

	jsonlPath := writeJSONL(t, tmpDir, sampleEvents())
	markerPath := migrationMarkerPath(jsonlPath)

	// Marker should not exist before backfill
	require.False(t, markerExists(markerPath))

	result := &ReconciliationResult{State: StateJSONLOnly}
	err := Backfill(result, threads, deps, jsonlPath)
	require.NoError(t, err)

	// Marker should exist after backfill
	require.True(t, markerExists(markerPath), "Migration-complete marker should be written after backfill")

	// Verify marker has content
	data, err := os.ReadFile(markerPath)
	require.NoError(t, err)
	require.Contains(t, string(data), "migration completed at")

	// Verify diagnostics mention the marker
	found := false
	for _, d := range result.Diagnostics {
		if d == "Migration-complete marker written" {
			found = true
			break
		}
	}
	require.True(t, found, "Diagnostics should mention marker was written")
}

func TestBackfill_NoOpsWhenMarkerPresent(t *testing.T) {
	tmpDir := t.TempDir()
	threads, deps := newMemoryRepos()

	jsonlPath := writeJSONL(t, tmpDir, sampleEvents())

	// Pre-create the migration marker
	markerPath := migrationMarkerPath(jsonlPath)
	require.NoError(t, os.WriteFile(markerPath, []byte("pre-existing marker"), 0644))

	result := &ReconciliationResult{State: StateJSONLOnly}
	err := Backfill(result, threads, deps, jsonlPath)
	require.NoError(t, err)

	// No threads should be inserted — backfill was skipped
	allThreads, err := threads.List(repository.ListOptions{})
	require.NoError(t, err)
	require.Equal(t, 0, len(allThreads), "Backfill should be a no-op when marker is present")

	// Diagnostics should indicate skip
	found := false
	for _, d := range result.Diagnostics {
		if d == "Migration-complete marker present — skipping backfill" {
			found = true
			break
		}
	}
	require.True(t, found, "Diagnostics should indicate backfill was skipped")
}

// --- Reconcile tests ---

func TestReconcile_FillsMissingRows(t *testing.T) {
	tmpDir := t.TempDir()
	threads, deps := newMemoryRepos()

	// Write full JSONL
	jsonlPath := writeJSONL(t, tmpDir, sampleEvents())

	// Populate SQLite partially (only channels, no messages)
	now := time.Now()
	_, err := threads.Create(domain.Thread{
		ID: "ch-root", Type: domain.ThreadChannel, Slug: "root",
		Title: "root", CreatedAt: now, CreatedBy: "SYSTEM",
	})
	require.NoError(t, err)
	_, err = threads.Create(domain.Thread{
		ID: "ch-general", Type: domain.ThreadChannel, Slug: "general",
		Title: "general", CreatedAt: now, CreatedBy: "SYSTEM",
	})
	require.NoError(t, err)

	result := &ReconciliationResult{
		State:             StateBothMismatch,
		ThreadCountJSONL:  5,
		ThreadCountSQLite: 2,
		DepCountJSONL:     3,
		DepCountSQLite:    0,
	}

	updated, err := Reconcile(result, threads, deps, jsonlPath)
	require.NoError(t, err)

	// All threads should now be present
	require.Equal(t, 5, updated.ThreadCountSQLite, "SQLite should have all 5 threads after reconciliation")
	require.Equal(t, 3, updated.DepCountSQLite, "SQLite should have all 3 deps after reconciliation")
	require.NotEmpty(t, updated.Diagnostics)
}

func TestReconcile_NonMismatchState_NoAction(t *testing.T) {
	tmpDir := t.TempDir()
	threads, deps := newMemoryRepos()
	jsonlPath := filepath.Join(tmpDir, FabricEventsFile)

	result := &ReconciliationResult{State: StateBothMatch}
	updated, err := Reconcile(result, threads, deps, jsonlPath)
	require.NoError(t, err)
	require.Equal(t, StateBothMatch, updated.State)

	// Should have a diagnostic indicating no action
	found := false
	for _, d := range updated.Diagnostics {
		if d == "Reconcile called in non-mismatch state \"both_match\" — no action taken" {
			found = true
			break
		}
	}
	require.True(t, found, "Should indicate no action for non-mismatch state")
}

// --- ShouldFallback tests ---

func TestShouldFallback_OrphanEdgesExceedThreshold(t *testing.T) {
	result := &ReconciliationResult{
		OrphanEdges:       1,
		ThreadCountSQLite: 100,
	}
	config := DefaultReconciliationConfig()
	// Default MaxOrphanEdges is 0, so 1 orphan should trigger fallback
	require.True(t, ShouldFallback(result, config))
}

func TestShouldFallback_UnresolvedParentRateExceedsThreshold(t *testing.T) {
	result := &ReconciliationResult{
		OrphanEdges:       0,
		UnresolvedParents: 10,
		ThreadCountSQLite: 100,
	}
	config := DefaultReconciliationConfig()
	// 10/100 = 0.1 > 0.005 threshold
	require.True(t, ShouldFallback(result, config))
}

func TestShouldFallback_BackfillDurationExceedsThreshold(t *testing.T) {
	result := &ReconciliationResult{
		BackfillDuration:  31 * time.Second,
		ThreadCountSQLite: 100,
	}
	config := DefaultReconciliationConfig()
	// 31s > 30s threshold
	require.True(t, ShouldFallback(result, config))
}

func TestShouldFallback_AllThresholdsSatisfied(t *testing.T) {
	result := &ReconciliationResult{
		OrphanEdges:       0,
		UnresolvedParents: 0,
		BackfillDuration:  5 * time.Second,
		ThreadCountSQLite: 100,
	}
	config := DefaultReconciliationConfig()
	require.False(t, ShouldFallback(result, config))
}

func TestShouldFallback_ZeroThreads_NoFalsePositive(t *testing.T) {
	result := &ReconciliationResult{
		OrphanEdges:       0,
		UnresolvedParents: 0,
		BackfillDuration:  1 * time.Second,
		ThreadCountSQLite: 0,
		ThreadCountJSONL:  0,
	}
	config := DefaultReconciliationConfig()
	// With zero threads, rate calculation should not trigger fallback
	require.False(t, ShouldFallback(result, config))
}

// --- Edge cases ---

func TestBackfill_EmptyJSONL(t *testing.T) {
	tmpDir := t.TempDir()
	threads, deps := newMemoryRepos()

	// Write empty JSONL file
	jsonlPath := filepath.Join(tmpDir, FabricEventsFile)
	require.NoError(t, os.WriteFile(jsonlPath, []byte{}, 0644))

	result := &ReconciliationResult{State: StateJSONLOnly}
	// Empty file has no content so hasJSONLContent returns false.
	// But Backfill can be called regardless — it just inserts nothing.
	err := Backfill(result, threads, deps, jsonlPath)
	require.NoError(t, err)

	allThreads, err := threads.List(repository.ListOptions{})
	require.NoError(t, err)
	require.Equal(t, 0, len(allThreads))
}

func TestBackfill_ArtifactEvents(t *testing.T) {
	tmpDir := t.TempDir()
	threads, deps := newMemoryRepos()

	now := time.Now()
	events := []PersistedEvent{
		makeChannelEvent("ch-general", "general", now),
		makeMessageEvent("msg-1", "ch-general", now.Add(time.Second)),
		makeArtifactEvent("art-1", "msg-1", now.Add(2*time.Second)),
	}
	jsonlPath := writeJSONL(t, tmpDir, events)

	result := &ReconciliationResult{State: StateJSONLOnly}
	err := Backfill(result, threads, deps, jsonlPath)
	require.NoError(t, err)

	// Verify artifact thread created
	art, err := threads.Get("art-1")
	require.NoError(t, err)
	require.Equal(t, domain.ThreadArtifact, art.Type)

	// Verify references dep
	refsRel := domain.RelationReferences
	refs, err := deps.GetChildren("msg-1", &refsRel)
	require.NoError(t, err)
	require.Equal(t, 1, len(refs))
	require.Equal(t, "art-1", refs[0].ThreadID)
}

// --- countThreadsAndDeps helper tests ---

func TestCountThreadsAndDeps_DeduplicatesThreadIDs(t *testing.T) {
	now := time.Now()
	// Same thread ID appears in two events (e.g., reply creates thread + archive updates)
	events := []PersistedEvent{
		makeChannelEvent("ch-1", "general", now),
		// Simulate a duplicate (same ID in another event type)
		{
			Version:   currentVersion,
			Timestamp: now.Add(time.Second),
			Event: fabric.Event{
				Type:      fabric.EventChannelArchived,
				Timestamp: now.Add(time.Second),
				ChannelID: "ch-1",
				// Thread field set to simulate edge case
				Thread: &domain.Thread{
					ID:   "ch-1",
					Type: domain.ThreadChannel,
				},
			},
		},
	}

	threadCount, depCount := countThreadsAndDeps(events)
	require.Equal(t, 1, threadCount, "Should deduplicate by thread ID")
	require.Equal(t, 0, depCount)
}

// --- extractDeps helper tests ---

func TestExtractDeps_AllEventTypes(t *testing.T) {
	now := time.Now()

	// Message → child_of
	msgEvent := makeMessageEvent("msg-1", "ch-general", now)
	msgDeps := extractDeps(msgEvent)
	require.Len(t, msgDeps, 1)
	require.Equal(t, domain.RelationChildOf, msgDeps[0].Relation)
	require.Equal(t, "msg-1", msgDeps[0].ThreadID)
	require.Equal(t, "ch-general", msgDeps[0].DependsOnID)

	// Reply → reply_to
	replyEvent := makeReplyEvent("reply-1", "ch-general", "msg-1", now)
	replyDeps := extractDeps(replyEvent)
	require.Len(t, replyDeps, 1)
	require.Equal(t, domain.RelationReplyTo, replyDeps[0].Relation)

	// Artifact → references
	artEvent := makeArtifactEvent("art-1", "msg-1", now)
	artDeps := extractDeps(artEvent)
	require.Len(t, artDeps, 1)
	require.Equal(t, domain.RelationReferences, artDeps[0].Relation)

	// Subscription → no deps
	subEvent := makeSubscriptionEvent("ch-general", "worker-1", now)
	subDeps := extractDeps(subEvent)
	require.Len(t, subDeps, 0)
}

// --- DefaultReconciliationConfig ---

func TestDefaultReconciliationConfig(t *testing.T) {
	cfg := DefaultReconciliationConfig()
	require.Equal(t, 0, cfg.MaxOrphanEdges)
	require.InDelta(t, 0.005, cfg.MaxUnresolvedParentRate, 0.0001)
	require.Equal(t, 30*time.Second, cfg.MaxBackfillDuration)
}
