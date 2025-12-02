package watcher_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"perles/internal/watcher"
)

func TestWatcher_DebounceMultipleWrites(t *testing.T) {
	// Create temp database file
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "beads.db")
	err := os.WriteFile(dbPath, []byte("test"), 0644)
	require.NoError(t, err, "failed to create test file")

	// Create watcher with short debounce
	w, err := watcher.New(watcher.Config{
		DBPath:      dbPath,
		DebounceDur: 50 * time.Millisecond,
	})
	require.NoError(t, err, "failed to create watcher")
	defer func() { _ = w.Stop() }()

	onChange, err := w.Start()
	require.NoError(t, err, "failed to start watcher")

	// Rapid writes should coalesce into single notification
	for i := 0; i < 10; i++ {
		err := os.WriteFile(dbPath, []byte(fmt.Sprintf("test%d", i)), 0644)
		require.NoError(t, err, "failed to write file")
		time.Sleep(10 * time.Millisecond)
	}

	// Should receive exactly one notification
	select {
	case <-onChange:
		// Expected
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected notification but got timeout")
	}

	// No second notification should come quickly
	select {
	case <-onChange:
		t.Fatal("unexpected second notification")
	case <-time.After(100 * time.Millisecond):
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

	onChange, err := w.Start()
	require.NoError(t, err, "failed to start watcher")

	// Write to unrelated file (not Create, since it already exists)
	err = os.WriteFile(otherPath, []byte("other content"), 0644)
	require.NoError(t, err, "failed to write other file")

	select {
	case <-onChange:
		t.Fatal("should not notify for unrelated files")
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

	_, err = w.Start()
	require.NoError(t, err, "failed to start watcher")

	// Stop should not hang or panic
	done := make(chan struct{})
	go func() {
		err := w.Stop()
		assert.NoError(t, err, "Stop returned error")
		close(done)
	}()

	select {
	case <-done:
		// Expected - stop completed successfully
	case <-time.After(1 * time.Second):
		t.Fatal("Stop() timed out - possible deadlock")
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

	onChange, err := w.Start()
	require.NoError(t, err, "failed to start watcher")

	// Write to WAL file should trigger notification
	err = os.WriteFile(walPath, []byte("wal data"), 0644)
	require.NoError(t, err, "failed to write WAL file")

	select {
	case <-onChange:
		// Expected - WAL writes should trigger notification
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected notification for WAL file write")
	}
}

func TestDefaultConfig(t *testing.T) {
	dbPath := "/test/beads.db"
	cfg := watcher.DefaultConfig(dbPath)

	assert.Equal(t, dbPath, cfg.DBPath)
	assert.Equal(t, 1*time.Second, cfg.DebounceDur)
}
