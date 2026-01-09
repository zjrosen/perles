package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadSessionIndex_Empty(t *testing.T) {
	// When file doesn't exist, should return empty index with version
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "sessions.json")

	index, err := LoadSessionIndex(indexPath)
	require.NoError(t, err)
	require.NotNil(t, index)
	require.Equal(t, SessionIndexVersion, index.Version)
	require.Empty(t, index.Sessions)
}

func TestLoadSessionIndex_Valid(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "sessions.json")

	// Create a valid index file
	now := time.Now().Truncate(time.Second)
	original := &SessionIndex{
		Version: "1.0",
		Sessions: []SessionIndexEntry{
			{
				ID:                        "session-123",
				StartTime:                 now,
				EndTime:                   now.Add(time.Hour),
				Status:                    StatusCompleted,
				EpicID:                    "perles-abc",
				WorkDir:                   "/test/dir",
				AccountabilitySummaryPath: ".perles/sessions/session-123/accountability_summary.md",
				WorkerCount:               3,
				TasksCompleted:            5,
				TotalCommits:              2,
			},
			{
				ID:                        "session-456",
				StartTime:                 now.Add(2 * time.Hour),
				EndTime:                   now.Add(4 * time.Hour),
				Status:                    StatusFailed,
				EpicID:                    "perles-def",
				WorkDir:                   "/test/dir2",
				AccountabilitySummaryPath: ".perles/sessions/session-456/accountability_summary.md",
				WorkerCount:               2,
				TasksCompleted:            1,
				TotalCommits:              0,
			},
		},
	}

	// Write the file
	data, err := json.MarshalIndent(original, "", "  ")
	require.NoError(t, err)
	err = os.WriteFile(indexPath, data, 0600)
	require.NoError(t, err)

	// Load and verify
	loaded, err := LoadSessionIndex(indexPath)
	require.NoError(t, err)
	require.Equal(t, original.Version, loaded.Version)
	require.Len(t, loaded.Sessions, 2)

	// Verify first entry
	require.Equal(t, "session-123", loaded.Sessions[0].ID)
	require.True(t, original.Sessions[0].StartTime.Equal(loaded.Sessions[0].StartTime))
	require.True(t, original.Sessions[0].EndTime.Equal(loaded.Sessions[0].EndTime))
	require.Equal(t, StatusCompleted, loaded.Sessions[0].Status)
	require.Equal(t, "perles-abc", loaded.Sessions[0].EpicID)
	require.Equal(t, "/test/dir", loaded.Sessions[0].WorkDir)
	require.Equal(t, ".perles/sessions/session-123/accountability_summary.md", loaded.Sessions[0].AccountabilitySummaryPath)
	require.Equal(t, 3, loaded.Sessions[0].WorkerCount)
	require.Equal(t, 5, loaded.Sessions[0].TasksCompleted)
	require.Equal(t, 2, loaded.Sessions[0].TotalCommits)

	// Verify second entry
	require.Equal(t, "session-456", loaded.Sessions[1].ID)
	require.Equal(t, StatusFailed, loaded.Sessions[1].Status)
}

func TestLoadSessionIndex_Invalid(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "sessions.json")

	// Write invalid JSON
	err := os.WriteFile(indexPath, []byte("not valid json {"), 0600)
	require.NoError(t, err)

	// Load should fail
	_, err = LoadSessionIndex(indexPath)
	require.Error(t, err)
	require.Contains(t, err.Error(), "parsing session index")
}

func TestLoadSessionIndex_PermissionError(t *testing.T) {
	// Skip on Windows where permissions work differently
	if runtime.GOOS == "windows" {
		t.Skip("skipping permission test on Windows")
	}

	dir := t.TempDir()
	indexPath := filepath.Join(dir, "sessions.json")

	// Create a file we can't read
	err := os.WriteFile(indexPath, []byte("{}"), 0000)
	require.NoError(t, err)
	t.Cleanup(func() {
		os.Chmod(indexPath, 0600) //nolint:errcheck // cleanup
	})

	// Load should fail with permission error
	_, err = LoadSessionIndex(indexPath)
	require.Error(t, err)
	require.Contains(t, err.Error(), "reading session index")
}

func TestSaveSessionIndex_AtomicRename(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "sessions.json")

	now := time.Now().Truncate(time.Second)
	index := &SessionIndex{
		Version: SessionIndexVersion,
		Sessions: []SessionIndexEntry{
			{
				ID:           "session-test",
				StartTime:    now,
				EndTime:      now.Add(time.Hour),
				Status:       StatusCompleted,
				WorkDir:      "/test",
				WorkerCount:  2,
				TotalCommits: 1,
			},
		},
	}

	// Save should create final file
	err := SaveSessionIndex(indexPath, index)
	require.NoError(t, err)

	// Final file should exist
	_, err = os.Stat(indexPath)
	require.NoError(t, err)

	// No temp files should remain (pattern: sessions.*.tmp)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, entry := range entries {
		require.False(t, strings.Contains(entry.Name(), ".tmp"),
			"temp file should not exist after save: %s", entry.Name())
	}

	// Verify content can be loaded back
	loaded, err := LoadSessionIndex(indexPath)
	require.NoError(t, err)
	require.Equal(t, index.Version, loaded.Version)
	require.Len(t, loaded.Sessions, 1)
	require.Equal(t, "session-test", loaded.Sessions[0].ID)
}

func TestSaveSessionIndex_OverwriteExisting(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "sessions.json")

	now := time.Now().Truncate(time.Second)

	// Create initial index
	initial := &SessionIndex{
		Version: SessionIndexVersion,
		Sessions: []SessionIndexEntry{
			{
				ID:        "session-1",
				StartTime: now,
				Status:    StatusCompleted,
				WorkDir:   "/test",
			},
		},
	}
	err := SaveSessionIndex(indexPath, initial)
	require.NoError(t, err)

	// Create updated index with more entries
	updated := &SessionIndex{
		Version: SessionIndexVersion,
		Sessions: []SessionIndexEntry{
			{
				ID:        "session-1",
				StartTime: now,
				Status:    StatusCompleted,
				WorkDir:   "/test",
			},
			{
				ID:        "session-2",
				StartTime: now.Add(time.Hour),
				Status:    StatusFailed,
				WorkDir:   "/test2",
			},
		},
	}
	err = SaveSessionIndex(indexPath, updated)
	require.NoError(t, err)

	// Load and verify
	loaded, err := LoadSessionIndex(indexPath)
	require.NoError(t, err)
	require.Len(t, loaded.Sessions, 2)
	require.Equal(t, "session-2", loaded.Sessions[1].ID)
}

func TestSaveSessionIndex_Concurrent(t *testing.T) {
	// Skip on Windows where concurrent file operations behave differently
	if runtime.GOOS == "windows" {
		t.Skip("skipping concurrent test on Windows due to different file locking behavior")
	}

	dir := t.TempDir()
	indexPath := filepath.Join(dir, "sessions.json")

	// Create initial empty index
	initial := &SessionIndex{
		Version:  SessionIndexVersion,
		Sessions: []SessionIndexEntry{},
	}
	err := SaveSessionIndex(indexPath, initial)
	require.NoError(t, err)

	// Simulate concurrent saves
	const numGoroutines = 10
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Each goroutine tries to save its own index
			index := &SessionIndex{
				Version: SessionIndexVersion,
				Sessions: []SessionIndexEntry{
					{
						ID:        "session-from-goroutine",
						StartTime: time.Now(),
						Status:    StatusCompleted,
						WorkDir:   "/test",
					},
				},
			}

			if err := SaveSessionIndex(indexPath, index); err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Collect any errors
	var errs []error
	for err := range errors {
		errs = append(errs, err)
	}
	require.Empty(t, errs, "concurrent saves should not error: %v", errs)

	// File should be valid JSON and loadable
	loaded, err := LoadSessionIndex(indexPath)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Equal(t, SessionIndexVersion, loaded.Version)
	// At least one session should exist (the last successful write)
	require.NotEmpty(t, loaded.Sessions)
}

func TestSessionIndexEntry_JSONRoundTrip(t *testing.T) {
	// Test that all fields serialize and deserialize correctly
	now := time.Now().Truncate(time.Second)
	entry := SessionIndexEntry{
		ID:                        "test-session-uuid",
		StartTime:                 now,
		EndTime:                   now.Add(2 * time.Hour),
		Status:                    StatusCompleted,
		EpicID:                    "perles-xyz.1",
		WorkDir:                   "/Users/test/project",
		AccountabilitySummaryPath: ".perles/sessions/test-session-uuid/accountability_summary.md",
		WorkerCount:               4,
		TasksCompleted:            7,
		TotalCommits:              3,
	}

	// Marshal
	data, err := json.Marshal(entry)
	require.NoError(t, err)

	// Unmarshal
	var loaded SessionIndexEntry
	err = json.Unmarshal(data, &loaded)
	require.NoError(t, err)

	// Verify all fields
	require.Equal(t, entry.ID, loaded.ID)
	require.True(t, entry.StartTime.Equal(loaded.StartTime))
	require.True(t, entry.EndTime.Equal(loaded.EndTime))
	require.Equal(t, entry.Status, loaded.Status)
	require.Equal(t, entry.EpicID, loaded.EpicID)
	require.Equal(t, entry.WorkDir, loaded.WorkDir)
	require.Equal(t, entry.AccountabilitySummaryPath, loaded.AccountabilitySummaryPath)
	require.Equal(t, entry.WorkerCount, loaded.WorkerCount)
	require.Equal(t, entry.TasksCompleted, loaded.TasksCompleted)
	require.Equal(t, entry.TotalCommits, loaded.TotalCommits)
}

func TestSessionIndexEntry_OmitEmptyFields(t *testing.T) {
	// Test that optional fields are omitted when empty
	now := time.Now().Truncate(time.Second)
	entry := SessionIndexEntry{
		ID:        "minimal-session",
		StartTime: now,
		Status:    StatusRunning,
		WorkDir:   "/test",
		// EpicID, AccountabilitySummaryPath intentionally empty
	}

	data, err := json.Marshal(entry)
	require.NoError(t, err)

	// Verify optional fields are omitted
	jsonStr := string(data)
	require.NotContains(t, jsonStr, "epic_id")
	require.NotContains(t, jsonStr, "accountability_summary_path")

	// But required fields are present
	require.Contains(t, jsonStr, "id")
	require.Contains(t, jsonStr, "start_time")
	require.Contains(t, jsonStr, "status")
	require.Contains(t, jsonStr, "work_dir")
}
