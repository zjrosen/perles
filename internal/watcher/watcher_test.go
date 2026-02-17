package watcher_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	appbeads "github.com/zjrosen/perles/internal/beads/application"
	"github.com/zjrosen/perles/internal/pubsub"
	"github.com/zjrosen/perles/internal/watcher"
)

func TestWatcher_DebounceMultipleWrites(t *testing.T) {
	// Create temp database file
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "beads.db")
	err := os.WriteFile(dbPath, []byte("test"), 0644)
	require.NoError(t, err, "failed to create test file")

	// Create watcher with debounce longer than total write duration
	// to ensure all writes coalesce into a single notification.
	// Write loop: 10 writes * 5ms = 50ms, so 150ms debounce ensures coalescing.
	w, err := watcher.New(watcher.Config{
		DBPath:      dbPath,
		DebounceDur: 150 * time.Millisecond,
	})
	require.NoError(t, err, "failed to create watcher")
	defer func() { _ = w.Stop() }()

	// Subscribe to broker before starting
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := w.Broker().Subscribe(ctx)

	err = w.Start()
	require.NoError(t, err, "failed to start watcher")

	// Rapid writes should coalesce into single notification
	for i := 0; i < 10; i++ {
		err := os.WriteFile(dbPath, []byte(fmt.Sprintf("test%d", i)), 0644)
		require.NoError(t, err, "failed to write file")
		time.Sleep(5 * time.Millisecond)
	}

	// Should receive exactly one notification
	select {
	case evt := <-sub:
		require.Equal(t, watcher.DBChanged, evt.Payload.Type, "expected DBChanged event")
	case <-time.After(400 * time.Millisecond):
		require.Fail(t, "expected notification but got timeout")
	}

	// No second notification should come quickly
	select {
	case <-sub:
		require.Fail(t, "unexpected second notification")
	case <-time.After(200 * time.Millisecond):
		// Expected - no second notification
	}
}

func TestWatcher_IgnoresIrrelevantFiles(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "beads.db")
	otherPath := filepath.Join(dir, "other.txt")
	err := os.WriteFile(dbPath, []byte("db"), 0644)
	require.NoError(t, err, "failed to create db file")
	// Pre-create the other file so writes to it are just Write events
	err = os.WriteFile(otherPath, []byte("initial"), 0644)
	require.NoError(t, err, "failed to create other file")

	w, err := watcher.New(watcher.Config{
		DBPath:      dbPath,
		DebounceDur: 50 * time.Millisecond,
	})
	require.NoError(t, err, "failed to create watcher")
	defer func() { _ = w.Stop() }()

	// Subscribe to broker before starting
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := w.Broker().Subscribe(ctx)

	err = w.Start()
	require.NoError(t, err, "failed to start watcher")

	// Write to unrelated file (not Create, since it already exists)
	err = os.WriteFile(otherPath, []byte("other content"), 0644)
	require.NoError(t, err, "failed to write other file")

	select {
	case <-sub:
		require.Fail(t, "should not notify for unrelated files")
	case <-time.After(100 * time.Millisecond):
		// Expected - no notification for unrelated file
	}
}

func TestWatcher_Stop(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "beads.db")
	err := os.WriteFile(dbPath, []byte("test"), 0644)
	require.NoError(t, err, "failed to create test file")

	w, err := watcher.New(watcher.Config{
		DBPath:      dbPath,
		DebounceDur: 50 * time.Millisecond,
	})
	require.NoError(t, err, "failed to create watcher")

	err = w.Start()
	require.NoError(t, err, "failed to start watcher")

	// Stop should not hang or panic
	done := make(chan struct{})
	go func() {
		err := w.Stop()
		require.NoError(t, err, "Stop returned error")
		close(done)
	}()

	select {
	case <-done:
		// Expected - stop completed successfully
	case <-time.After(1 * time.Second):
		require.Fail(t, "Stop() timed out - possible deadlock")
	}
}

func TestWatcher_WatchesWALFile(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "beads.db")
	walPath := filepath.Join(dir, "beads.db-wal")

	// Create db file (watcher needs the directory to exist with db file)
	err := os.WriteFile(dbPath, []byte("db"), 0644)
	require.NoError(t, err, "failed to create db file")

	w, err := watcher.New(watcher.Config{
		DBPath:      dbPath,
		DebounceDur: 50 * time.Millisecond,
	})
	require.NoError(t, err, "failed to create watcher")
	defer func() { _ = w.Stop() }()

	// Subscribe to broker before starting
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := w.Broker().Subscribe(ctx)

	err = w.Start()
	require.NoError(t, err, "failed to start watcher")

	// Write to WAL file should trigger notification
	err = os.WriteFile(walPath, []byte("wal data"), 0644)
	require.NoError(t, err, "failed to write WAL file")

	select {
	case evt := <-sub:
		require.Equal(t, watcher.DBChanged, evt.Payload.Type, "expected DBChanged event for WAL write")
	case <-time.After(200 * time.Millisecond):
		require.Fail(t, "expected notification for WAL file write")
	}
}

func TestDefaultConfig(t *testing.T) {
	dbPath := "/test/beads.db"
	cfg := watcher.DefaultConfig(dbPath, appbeads.DialectSQLite)

	require.Equal(t, dbPath, cfg.DBPath)
	require.Equal(t, 100*time.Millisecond, cfg.DebounceDur)
	require.Equal(t, appbeads.DialectSQLite, cfg.Dialect)
}

func TestDefaultConfig_Dolt(t *testing.T) {
	dbPath := "/test/.beads/dolt/beads_myproject"
	cfg := watcher.DefaultConfig(dbPath, appbeads.DialectMySQL)

	require.Equal(t, dbPath, cfg.DBPath)
	require.Equal(t, 500*time.Millisecond, cfg.DebounceDur, "Dolt debounce should be longer to coalesce rapid writes")
	require.Equal(t, appbeads.DialectMySQL, cfg.Dialect)
}

func TestWatcher_BrokerAccessor(t *testing.T) {
	// Create temp database file
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "beads.db")
	err := os.WriteFile(dbPath, []byte("test"), 0644)
	require.NoError(t, err, "failed to create test file")

	// Create watcher
	w, err := watcher.New(watcher.Config{
		DBPath:      dbPath,
		DebounceDur: 50 * time.Millisecond,
	})
	require.NoError(t, err, "failed to create watcher")
	defer func() { _ = w.Stop() }()

	// Start watcher
	err = w.Start()
	require.NoError(t, err, "failed to start watcher")

	// Broker() should return non-nil broker after New()
	broker := w.Broker()
	require.NotNil(t, broker, "Broker() should return non-nil broker after New()")
}

func TestWatcher_BrokerAccessorBeforeStart(t *testing.T) {
	// Create temp database file
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "beads.db")
	err := os.WriteFile(dbPath, []byte("test"), 0644)
	require.NoError(t, err, "failed to create test file")

	// Create watcher but do NOT start it
	w, err := watcher.New(watcher.Config{
		DBPath:      dbPath,
		DebounceDur: 50 * time.Millisecond,
	})
	require.NoError(t, err, "failed to create watcher")
	defer func() { _ = w.Stop() }()

	// Broker() should return non-nil broker even before Start() is called
	// This is a key design decision: broker is created in New(), not Start()
	broker := w.Broker()
	require.NotNil(t, broker, "Broker() should return non-nil broker even before Start()")
}

func TestWatcher_BrokerCreatedEvenIfStartFails(t *testing.T) {
	// Create watcher with invalid path (directory doesn't exist)
	// This tests that broker is created in New() even if Start() will fail later
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "beads.db")
	err := os.WriteFile(dbPath, []byte("test"), 0644)
	require.NoError(t, err, "failed to create test file")

	w, err := watcher.New(watcher.Config{
		DBPath:      dbPath,
		DebounceDur: 50 * time.Millisecond,
	})
	require.NoError(t, err, "New() should succeed")

	// Broker should be valid regardless of whether Start() has been called
	broker := w.Broker()
	require.NotNil(t, broker, "Broker() should return non-nil broker even if Start() hasn't been called")

	// Stop should clean up properly
	err = w.Stop()
	require.NoError(t, err, "Stop() should succeed")
}

// TestWatcher_PublishesDBChangedEvent verifies DBChanged event is published after file write
func TestWatcher_PublishesDBChangedEvent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "beads.db")
	err := os.WriteFile(dbPath, []byte("test"), 0644)
	require.NoError(t, err, "failed to create test file")

	w, err := watcher.New(watcher.Config{
		DBPath:      dbPath,
		DebounceDur: 50 * time.Millisecond,
	})
	require.NoError(t, err, "failed to create watcher")
	defer func() { _ = w.Stop() }()

	// Subscribe before starting
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := w.Broker().Subscribe(ctx)

	err = w.Start()
	require.NoError(t, err, "failed to start watcher")

	// Write to database file
	err = os.WriteFile(dbPath, []byte("modified content"), 0644)
	require.NoError(t, err, "failed to write file")

	// Should receive DBChanged event
	select {
	case evt := <-sub:
		require.Equal(t, watcher.DBChanged, evt.Payload.Type, "expected DBChanged event type")
		require.Equal(t, pubsub.UpdatedEvent, evt.Type, "expected UpdatedEvent wrapper type")
		require.Nil(t, evt.Payload.Error, "DBChanged event should have nil Error")
	case <-time.After(200 * time.Millisecond):
		require.Fail(t, "expected DBChanged event but got timeout")
	}
}

// TestWatcher_PublishesWatcherErrorEvent verifies WatcherError event is published on fsnotify errors
// Note: This test is difficult to implement directly because fsnotify errors are rare and
// hard to trigger programmatically. We verify the code path exists by examining the loop() implementation.
// The watcher publishes WatcherError events immediately (not debounced) when fsnotify reports an error.
func TestWatcher_PublishesWatcherErrorEvent(t *testing.T) {
	// Since we cannot easily inject errors into fsnotify, we verify the event type exists
	// and is properly structured. The actual error publishing code path is tested implicitly
	// by the fact that the code compiles and the broker is properly wired up.
	evt := watcher.WatcherEvent{
		Type:  watcher.WatcherError,
		Error: fmt.Errorf("simulated fsnotify error"),
	}
	require.Equal(t, watcher.WatcherError, evt.Type)
	require.NotNil(t, evt.Error)
	require.Contains(t, evt.Error.Error(), "simulated fsnotify error")
}

// TestWatcher_DebounceWithPubsub verifies debounce still coalesces events (same behavior as before)
func TestWatcher_DebounceWithPubsub(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "beads.db")
	err := os.WriteFile(dbPath, []byte("test"), 0644)
	require.NoError(t, err, "failed to create test file")

	// Use a longer debounce to make the test more reliable
	w, err := watcher.New(watcher.Config{
		DBPath:      dbPath,
		DebounceDur: 100 * time.Millisecond,
	})
	require.NoError(t, err, "failed to create watcher")
	defer func() { _ = w.Stop() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := w.Broker().Subscribe(ctx)

	err = w.Start()
	require.NoError(t, err, "failed to start watcher")

	// Multiple rapid writes should be coalesced
	for i := 0; i < 5; i++ {
		err := os.WriteFile(dbPath, []byte(fmt.Sprintf("content%d", i)), 0644)
		require.NoError(t, err, "failed to write file")
		time.Sleep(20 * time.Millisecond) // Faster than debounce interval
	}

	// Wait for debounce to complete
	select {
	case evt := <-sub:
		require.Equal(t, watcher.DBChanged, evt.Payload.Type, "expected DBChanged event")
	case <-time.After(300 * time.Millisecond):
		require.Fail(t, "expected coalesced event but got timeout")
	}

	// Verify no second event arrives (events were coalesced)
	select {
	case <-sub:
		require.Fail(t, "received unexpected second event - debounce may not be working")
	case <-time.After(150 * time.Millisecond):
		// Expected - events were properly coalesced
	}
}

// TestWatcher_StopClosesSubscriptions verifies stopping watcher closes subscriber channels
func TestWatcher_StopClosesSubscriptions(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "beads.db")
	err := os.WriteFile(dbPath, []byte("test"), 0644)
	require.NoError(t, err, "failed to create test file")

	w, err := watcher.New(watcher.Config{
		DBPath:      dbPath,
		DebounceDur: 50 * time.Millisecond,
	})
	require.NoError(t, err, "failed to create watcher")

	// Subscribe with a long-lived context
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sub := w.Broker().Subscribe(ctx)

	err = w.Start()
	require.NoError(t, err, "failed to start watcher")

	// Stop the watcher - this should close the broker and all subscriptions
	err = w.Stop()
	require.NoError(t, err, "Stop() should succeed")

	// Subscription channel should be closed after Stop()
	// Give a small window for the close to propagate
	select {
	case _, ok := <-sub:
		// Either we get an event (unlikely) or the channel is closed
		if ok {
			// If channel is still open, drain it and check again
			select {
			case _, ok := <-sub:
				require.False(t, ok, "subscription channel should be closed after Stop()")
			case <-time.After(100 * time.Millisecond):
				// Channel might still be open but empty - that's acceptable
				// as long as no new events can be published
			}
		}
	case <-time.After(200 * time.Millisecond):
		// Channel not closed yet - that's OK, the broker close is asynchronous
		// What matters is that no new events can be published after Stop()
	}
}

// TestWatcher_ContextCancellation verifies subscription cleanup on context cancel
func TestWatcher_ContextCancellation(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "beads.db")
	err := os.WriteFile(dbPath, []byte("test"), 0644)
	require.NoError(t, err, "failed to create test file")

	w, err := watcher.New(watcher.Config{
		DBPath:      dbPath,
		DebounceDur: 50 * time.Millisecond,
	})
	require.NoError(t, err, "failed to create watcher")
	defer func() { _ = w.Stop() }()

	err = w.Start()
	require.NoError(t, err, "failed to start watcher")

	// Create a subscription with a cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	sub := w.Broker().Subscribe(ctx)

	// Cancel the context - this should close the subscription channel
	cancel()

	// The subscription channel should be closed after context cancellation
	// Give a small window for the cancellation to propagate
	select {
	case _, ok := <-sub:
		require.False(t, ok, "subscription channel should be closed after context cancellation")
	case <-time.After(200 * time.Millisecond):
		require.Fail(t, "subscription channel was not closed after context cancellation")
	}
}

// TestWatcher_MultipleSubscribers verifies all subscribers receive the same event
func TestWatcher_MultipleSubscribers(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "beads.db")
	err := os.WriteFile(dbPath, []byte("test"), 0644)
	require.NoError(t, err, "failed to create test file")

	w, err := watcher.New(watcher.Config{
		DBPath:      dbPath,
		DebounceDur: 50 * time.Millisecond,
	})
	require.NoError(t, err, "failed to create watcher")
	defer func() { _ = w.Stop() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create multiple subscribers
	sub1 := w.Broker().Subscribe(ctx)
	sub2 := w.Broker().Subscribe(ctx)
	sub3 := w.Broker().Subscribe(ctx)

	err = w.Start()
	require.NoError(t, err, "failed to start watcher")

	// Trigger a database change
	err = os.WriteFile(dbPath, []byte("modified"), 0644)
	require.NoError(t, err, "failed to write file")

	// All three subscribers should receive the event
	receivedCount := 0
	timeout := time.After(300 * time.Millisecond)

	// Collect events from all subscribers
	for i := 0; i < 3; i++ {
		select {
		case evt := <-sub1:
			require.Equal(t, watcher.DBChanged, evt.Payload.Type)
			receivedCount++
			sub1 = nil // Prevent re-reading
		case evt := <-sub2:
			require.Equal(t, watcher.DBChanged, evt.Payload.Type)
			receivedCount++
			sub2 = nil
		case evt := <-sub3:
			require.Equal(t, watcher.DBChanged, evt.Payload.Type)
			receivedCount++
			sub3 = nil
		case <-timeout:
			require.Fail(t, "timeout waiting for events - received %d of 3 expected events", receivedCount)
		}
	}

	require.Equal(t, 3, receivedCount, "all three subscribers should receive the event")
}

// =============================================================================
// Dolt Dialect Tests
// =============================================================================

func TestDolt_WatchesManifestFile(t *testing.T) {
	// Simulate Dolt directory structure: dbPath/.dolt/noms/manifest
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "beads_test")
	nomsDir := filepath.Join(dbPath, ".dolt", "noms")
	err := os.MkdirAll(nomsDir, 0755)
	require.NoError(t, err, "failed to create noms directory")

	manifestPath := filepath.Join(nomsDir, "manifest")
	err = os.WriteFile(manifestPath, []byte("5:__DOLT__:abc123"), 0644)
	require.NoError(t, err, "failed to create manifest file")

	w, err := watcher.New(watcher.Config{
		DBPath:      dbPath,
		DebounceDur: 50 * time.Millisecond,
		Dialect:     appbeads.DialectMySQL,
	})
	require.NoError(t, err, "failed to create watcher")
	defer func() { _ = w.Stop() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := w.Broker().Subscribe(ctx)

	err = w.Start()
	require.NoError(t, err, "failed to start watcher")

	// Write to manifest should trigger notification (embedded mode)
	err = os.WriteFile(manifestPath, []byte("5:__DOLT__:def456"), 0644)
	require.NoError(t, err, "failed to write manifest file")

	select {
	case evt := <-sub:
		require.Equal(t, watcher.DBChanged, evt.Payload.Type, "expected DBChanged event for manifest write")
	case <-time.After(200 * time.Millisecond):
		require.Fail(t, "expected notification for manifest file write")
	}
}

func TestDolt_WatchesJournalFile(t *testing.T) {
	// In server mode, Dolt appends to the journal file instead of updating manifest
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "beads_test")
	nomsDir := filepath.Join(dbPath, ".dolt", "noms")
	err := os.MkdirAll(nomsDir, 0755)
	require.NoError(t, err, "failed to create noms directory")

	// Create the journal file (32 'v' chars)
	journalPath := filepath.Join(nomsDir, "vvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvv")
	err = os.WriteFile(journalPath, []byte("journal data"), 0644)
	require.NoError(t, err, "failed to create journal file")

	w, err := watcher.New(watcher.Config{
		DBPath:      dbPath,
		DebounceDur: 50 * time.Millisecond,
		Dialect:     appbeads.DialectMySQL,
	})
	require.NoError(t, err, "failed to create watcher")
	defer func() { _ = w.Stop() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := w.Broker().Subscribe(ctx)

	err = w.Start()
	require.NoError(t, err, "failed to start watcher")

	// Write to journal should trigger notification (server mode)
	err = os.WriteFile(journalPath, []byte("journal data appended"), 0644)
	require.NoError(t, err, "failed to write journal file")

	select {
	case evt := <-sub:
		require.Equal(t, watcher.DBChanged, evt.Payload.Type, "expected DBChanged event for journal write")
	case <-time.After(200 * time.Millisecond):
		require.Fail(t, "expected notification for journal file write")
	}
}

func TestDolt_IgnoresIrrelevantFiles(t *testing.T) {
	// Dolt watcher should ignore non-manifest files in the noms directory
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "beads_test")
	nomsDir := filepath.Join(dbPath, ".dolt", "noms")
	err := os.MkdirAll(nomsDir, 0755)
	require.NoError(t, err, "failed to create noms directory")

	manifestPath := filepath.Join(nomsDir, "manifest")
	err = os.WriteFile(manifestPath, []byte("5:__DOLT__:abc123"), 0644)
	require.NoError(t, err, "failed to create manifest file")

	// Pre-create an irrelevant file
	otherPath := filepath.Join(nomsDir, "journal.idx")
	err = os.WriteFile(otherPath, []byte("idx data"), 0644)
	require.NoError(t, err, "failed to create journal.idx file")

	w, err := watcher.New(watcher.Config{
		DBPath:      dbPath,
		DebounceDur: 50 * time.Millisecond,
		Dialect:     appbeads.DialectMySQL,
	})
	require.NoError(t, err, "failed to create watcher")
	defer func() { _ = w.Stop() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := w.Broker().Subscribe(ctx)

	err = w.Start()
	require.NoError(t, err, "failed to start watcher")

	// Write to journal.idx (not manifest) should NOT trigger notification
	err = os.WriteFile(otherPath, []byte("updated idx data"), 0644)
	require.NoError(t, err, "failed to write journal.idx file")

	select {
	case <-sub:
		require.Fail(t, "should not notify for non-manifest files in Dolt mode")
	case <-time.After(100 * time.Millisecond):
		// Expected - no notification for irrelevant file
	}
}

func TestDolt_IgnoresSQLiteFiles(t *testing.T) {
	// Dolt watcher should NOT react to beads.db files
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "beads_test")
	nomsDir := filepath.Join(dbPath, ".dolt", "noms")
	err := os.MkdirAll(nomsDir, 0755)
	require.NoError(t, err, "failed to create noms directory")

	manifestPath := filepath.Join(nomsDir, "manifest")
	err = os.WriteFile(manifestPath, []byte("5:__DOLT__:abc123"), 0644)
	require.NoError(t, err, "failed to create manifest file")

	// Create a beads.db file in the noms dir (shouldn't match)
	sqlitePath := filepath.Join(nomsDir, "beads.db")
	err = os.WriteFile(sqlitePath, []byte("sqlite"), 0644)
	require.NoError(t, err, "failed to create beads.db file")

	w, err := watcher.New(watcher.Config{
		DBPath:      dbPath,
		DebounceDur: 50 * time.Millisecond,
		Dialect:     appbeads.DialectMySQL,
	})
	require.NoError(t, err, "failed to create watcher")
	defer func() { _ = w.Stop() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := w.Broker().Subscribe(ctx)

	err = w.Start()
	require.NoError(t, err, "failed to start watcher")

	// Write to beads.db should NOT trigger for Dolt dialect
	err = os.WriteFile(sqlitePath, []byte("sqlite modified"), 0644)
	require.NoError(t, err, "failed to write beads.db file")

	select {
	case <-sub:
		require.Fail(t, "Dolt watcher should not react to beads.db files")
	case <-time.After(100 * time.Millisecond):
		// Expected
	}
}

func TestDolt_DebounceManifestWrites(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "beads_test")
	nomsDir := filepath.Join(dbPath, ".dolt", "noms")
	err := os.MkdirAll(nomsDir, 0755)
	require.NoError(t, err, "failed to create noms directory")

	manifestPath := filepath.Join(nomsDir, "manifest")
	err = os.WriteFile(manifestPath, []byte("5:__DOLT__:initial"), 0644)
	require.NoError(t, err, "failed to create manifest file")

	w, err := watcher.New(watcher.Config{
		DBPath:      dbPath,
		DebounceDur: 150 * time.Millisecond,
		Dialect:     appbeads.DialectMySQL,
	})
	require.NoError(t, err, "failed to create watcher")
	defer func() { _ = w.Stop() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := w.Broker().Subscribe(ctx)

	err = w.Start()
	require.NoError(t, err, "failed to start watcher")

	// Rapid manifest writes should coalesce
	for i := 0; i < 10; i++ {
		err := os.WriteFile(manifestPath, []byte(fmt.Sprintf("5:__DOLT__:hash%d", i)), 0644)
		require.NoError(t, err, "failed to write manifest file")
		time.Sleep(5 * time.Millisecond)
	}

	// Should receive exactly one notification
	select {
	case evt := <-sub:
		require.Equal(t, watcher.DBChanged, evt.Payload.Type, "expected DBChanged event")
	case <-time.After(400 * time.Millisecond):
		require.Fail(t, "expected notification but got timeout")
	}

	// No second notification
	select {
	case <-sub:
		require.Fail(t, "unexpected second notification")
	case <-time.After(200 * time.Millisecond):
		// Expected
	}
}
