package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/orchestration/metrics"
	"github.com/zjrosen/perles/internal/orchestration/v2/processor"
	"github.com/zjrosen/perles/internal/orchestration/workflow"
	"github.com/zjrosen/perles/internal/pubsub"
	"github.com/zjrosen/perles/internal/ui/shared/chatrender"
)

// phasePtr is a helper to create *ProcessPhase from ProcessPhase constants.
func phasePtr(p events.ProcessPhase) *events.ProcessPhase {
	return &p
}

func TestStatus_String(t *testing.T) {
	tests := []struct {
		status   Status
		expected string
	}{
		{StatusRunning, "running"},
		{StatusCompleted, "completed"},
		{StatusFailed, "failed"},
		{StatusTimedOut, "timed_out"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.status.String())
		})
	}
}

func TestAccountabilitySummaryFileConstant(t *testing.T) {
	// Verify the accountabilitySummaryFile constant is defined correctly
	require.Equal(t, "accountability_summary.md", accountabilitySummaryFile)
}

func TestMetadata_Save_Load(t *testing.T) {
	dir := t.TempDir()

	now := time.Now().Truncate(time.Second) // Truncate for JSON round-trip
	meta := &Metadata{
		SessionID:     "test-session-123",
		StartTime:     now,
		EndTime:       now.Add(time.Hour),
		Status:        StatusCompleted,
		SessionDir:    "/test/work/dir",
		CoordinatorID: "coord-abc",
		Workers: []WorkerMetadata{
			{
				ID:         "worker-1",
				SpawnedAt:  now.Add(time.Minute),
				RetiredAt:  now.Add(30 * time.Minute),
				FinalPhase: "idle",
			},
			{
				ID:         "worker-2",
				SpawnedAt:  now.Add(2 * time.Minute),
				RetiredAt:  now.Add(45 * time.Minute),
				FinalPhase: "implementing",
			},
		},
		ClientType: "claude",
		Model:      "sonnet",
		TokenUsage: TokenUsageSummary{
			ContextTokens:     125000,
			TotalOutputTokens: 45000,
			TotalCostUSD:      2.35,
		},
	}

	// Save metadata
	err := meta.Save(dir)
	require.NoError(t, err)

	// Verify file exists
	path := filepath.Join(dir, metadataFilename)
	_, err = os.Stat(path)
	require.NoError(t, err)

	// Load metadata
	loaded, err := Load(dir)
	require.NoError(t, err)

	// Verify all fields
	require.Equal(t, meta.SessionID, loaded.SessionID)
	require.True(t, meta.StartTime.Equal(loaded.StartTime), "StartTime mismatch: expected %v, got %v", meta.StartTime, loaded.StartTime)
	require.True(t, meta.EndTime.Equal(loaded.EndTime), "EndTime mismatch: expected %v, got %v", meta.EndTime, loaded.EndTime)
	require.Equal(t, meta.Status, loaded.Status)
	require.Equal(t, meta.SessionDir, loaded.SessionDir)
	require.Equal(t, meta.CoordinatorID, loaded.CoordinatorID)
	require.Equal(t, meta.ClientType, loaded.ClientType)
	require.Equal(t, meta.Model, loaded.Model)
	require.Equal(t, meta.TokenUsage.ContextTokens, loaded.TokenUsage.ContextTokens)
	require.Equal(t, meta.TokenUsage.TotalOutputTokens, loaded.TokenUsage.TotalOutputTokens)
	require.Equal(t, meta.TokenUsage.TotalCostUSD, loaded.TokenUsage.TotalCostUSD)

	// Verify workers
	require.Len(t, loaded.Workers, 2)
	require.Equal(t, "worker-1", loaded.Workers[0].ID)
	require.True(t, meta.Workers[0].SpawnedAt.Equal(loaded.Workers[0].SpawnedAt))
	require.True(t, meta.Workers[0].RetiredAt.Equal(loaded.Workers[0].RetiredAt))
	require.Equal(t, "idle", loaded.Workers[0].FinalPhase)
	require.Equal(t, "worker-2", loaded.Workers[1].ID)
}

func TestMetadata_SaveCreatesDir(t *testing.T) {
	baseDir := t.TempDir()
	nestedDir := filepath.Join(baseDir, "nested", "deep", "session")

	meta := &Metadata{
		SessionID:  "test-nested",
		StartTime:  time.Now(),
		Status:     StatusRunning,
		SessionDir: "/test",
	}

	// Save should create nested directories
	err := meta.Save(nestedDir)
	require.NoError(t, err)

	// Verify directory was created
	info, err := os.Stat(nestedDir)
	require.NoError(t, err)
	require.True(t, info.IsDir())

	// Verify file exists
	_, err = os.Stat(filepath.Join(nestedDir, metadataFilename))
	require.NoError(t, err)
}

func TestMetadata_LoadNotFound(t *testing.T) {
	dir := t.TempDir()

	// Try to load from empty directory
	_, err := Load(dir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "metadata file not found")
}

func TestMetadata_LoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()

	// Write invalid JSON
	path := filepath.Join(dir, metadataFilename)
	err := os.WriteFile(path, []byte("not valid json"), 0640)
	require.NoError(t, err)

	// Load should fail
	_, err = Load(dir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unmarshaling metadata")
}

func TestMetadata_EmptyWorkers(t *testing.T) {
	dir := t.TempDir()

	meta := &Metadata{
		SessionID:  "empty-workers",
		StartTime:  time.Now().Truncate(time.Second),
		Status:     StatusRunning,
		SessionDir: "/test",
		Workers:    []WorkerMetadata{}, // Empty slice
	}

	err := meta.Save(dir)
	require.NoError(t, err)

	loaded, err := Load(dir)
	require.NoError(t, err)
	require.NotNil(t, loaded.Workers)
	require.Empty(t, loaded.Workers)
}

func TestMetadata_ZeroValueFields(t *testing.T) {
	dir := t.TempDir()

	// Minimal metadata with zero values
	meta := &Metadata{
		SessionID:  "minimal",
		StartTime:  time.Now().Truncate(time.Second),
		Status:     StatusRunning,
		SessionDir: "/test",
	}

	err := meta.Save(dir)
	require.NoError(t, err)

	// Load and verify zero values are handled correctly
	loaded, err := Load(dir)
	require.NoError(t, err)
	require.True(t, loaded.EndTime.IsZero())
	require.Empty(t, loaded.CoordinatorID)
	require.Empty(t, loaded.Model)
	require.Empty(t, loaded.EpicID)
	require.Empty(t, loaded.AccountabilitySummaryPath)
	require.Equal(t, 0, loaded.TokenUsage.ContextTokens)
	require.Equal(t, 0, loaded.TokenUsage.TotalOutputTokens)
	require.Equal(t, 0.0, loaded.TokenUsage.TotalCostUSD)
}

func TestMetadata_EpicIDAndAccountabilitySummaryPath(t *testing.T) {
	dir := t.TempDir()

	now := time.Now().Truncate(time.Second)
	meta := &Metadata{
		SessionID:                 "test-epic-session",
		StartTime:                 now,
		EndTime:                   now.Add(time.Hour),
		Status:                    StatusCompleted,
		SessionDir:                "/test/work/dir",
		EpicID:                    "perles-abc",
		AccountabilitySummaryPath: ".perles/sessions/test-session-123/accountability_summary.md",
		Workers:                   []WorkerMetadata{},
	}

	// Save metadata
	err := meta.Save(dir)
	require.NoError(t, err)

	// Load metadata
	loaded, err := Load(dir)
	require.NoError(t, err)

	// Verify new fields
	require.Equal(t, "perles-abc", loaded.EpicID)
	require.Equal(t, ".perles/sessions/test-session-123/accountability_summary.md", loaded.AccountabilitySummaryPath)
}

func TestMetadata_BackwardCompatibility(t *testing.T) {
	// Test that old metadata JSON (without EpicID/AccountabilitySummaryPath) can still be loaded
	dir := t.TempDir()

	// Write old-style metadata JSON without the new fields
	oldJSON := `{
  "session_id": "old-session-123",
  "start_time": "2026-01-01T10:00:00Z",
  "status": "completed",
  "work_dir": "/test",
  "workers": [],
  "client_type": "claude"
}`
	err := os.WriteFile(filepath.Join(dir, metadataFilename), []byte(oldJSON), 0600)
	require.NoError(t, err)

	// Load should succeed and new fields should be empty
	loaded, err := Load(dir)
	require.NoError(t, err)
	require.Equal(t, "old-session-123", loaded.SessionID)
	require.Equal(t, StatusCompleted, loaded.Status)
	require.Empty(t, loaded.EpicID)
	require.Empty(t, loaded.AccountabilitySummaryPath)
}

func TestMetadata_ApplicationContextFields(t *testing.T) {
	// Test that new application context fields serialize and deserialize correctly
	dir := t.TempDir()

	now := time.Now().Truncate(time.Second)
	meta := &Metadata{
		SessionID:       "context-test-session",
		StartTime:       now,
		Status:          StatusRunning,
		SessionDir:      "/test/project",
		Workers:         []WorkerMetadata{},
		ClientType:      "claude",
		ApplicationName: "my-app",
		WorkDir:         "/Users/dev/my-app",
		DatePartition:   "2026-01-11",
	}

	// Save metadata
	err := meta.Save(dir)
	require.NoError(t, err)

	// Load metadata
	loaded, err := Load(dir)
	require.NoError(t, err)

	// Verify all new fields are preserved
	require.Equal(t, "my-app", loaded.ApplicationName)
	require.Equal(t, "/Users/dev/my-app", loaded.WorkDir)
	require.Equal(t, "2026-01-11", loaded.DatePartition)
}

func TestMetadata_ApplicationContextFields_OmitEmpty(t *testing.T) {
	// Test that empty application context fields are omitted from JSON
	dir := t.TempDir()

	meta := &Metadata{
		SessionID:  "omit-empty-test",
		StartTime:  time.Now(),
		Status:     StatusRunning,
		SessionDir: "/test",
		Workers:    []WorkerMetadata{},
		ClientType: "claude",
		// ApplicationName, WorkDir, DatePartition intentionally empty
	}

	err := meta.Save(dir)
	require.NoError(t, err)

	// Read raw JSON
	data, err := os.ReadFile(filepath.Join(dir, metadataFilename))
	require.NoError(t, err)

	jsonStr := string(data)
	// Verify optional fields are omitted when empty
	require.NotContains(t, jsonStr, "application_name")
	require.NotContains(t, jsonStr, `"work_dir"`)
	require.NotContains(t, jsonStr, "date_partition")
}

func TestMetadata_BackwardCompatibility_NewContextFields(t *testing.T) {
	// Test that metadata JSON without optional context fields can still be loaded
	dir := t.TempDir()

	// Write metadata JSON without the optional context fields
	minimalJSON := `{
  "session_id": "old-session-456",
  "start_time": "2026-01-01T10:00:00Z",
  "status": "running",
  "session_dir": "/test/old",
  "workers": [],
  "client_type": "claude",
  "model": "sonnet"
}`
	err := os.WriteFile(filepath.Join(dir, metadataFilename), []byte(minimalJSON), 0600)
	require.NoError(t, err)

	// Load should succeed and optional context fields should be empty
	loaded, err := Load(dir)
	require.NoError(t, err)
	require.Equal(t, "old-session-456", loaded.SessionID)
	require.Equal(t, StatusRunning, loaded.Status)
	require.Empty(t, loaded.ApplicationName)
	require.Empty(t, loaded.WorkDir)
	require.Empty(t, loaded.DatePartition)
}

func TestMetadata_PartialContextFields(t *testing.T) {
	// Test that metadata with only some context fields can be loaded correctly
	dir := t.TempDir()

	// JSON with only ApplicationName set, other context fields missing
	partialJSON := `{
  "session_id": "partial-context",
  "start_time": "2026-01-11T15:30:00Z",
  "status": "running",
  "session_dir": "/test",
  "workers": [],
  "client_type": "claude",
  "application_name": "partial-app"
}`
	err := os.WriteFile(filepath.Join(dir, metadataFilename), []byte(partialJSON), 0600)
	require.NoError(t, err)

	// Load should succeed
	loaded, err := Load(dir)
	require.NoError(t, err)
	require.Equal(t, "partial-context", loaded.SessionID)
	require.Equal(t, "partial-app", loaded.ApplicationName)
	require.Empty(t, loaded.WorkDir)
	require.Empty(t, loaded.DatePartition)
}

// Tests for session resumption metadata fields

func TestMetadata_SessionResumptionFields(t *testing.T) {
	// Test that CoordinatorSessionRef and Resumable fields serialize correctly
	dir := t.TempDir()

	now := time.Now().Truncate(time.Second)
	meta := &Metadata{
		SessionID:             "resumption-test-session",
		StartTime:             now,
		Status:                StatusRunning,
		SessionDir:            "/test/project",
		CoordinatorID:         "coordinator",
		CoordinatorSessionRef: "claude-session-xyz-123",
		Resumable:             true,
		Workers:               []WorkerMetadata{},
		ClientType:            "claude",
	}

	// Save metadata
	err := meta.Save(dir)
	require.NoError(t, err)

	// Load metadata
	loaded, err := Load(dir)
	require.NoError(t, err)

	// Verify session resumption fields are preserved
	require.Equal(t, "claude-session-xyz-123", loaded.CoordinatorSessionRef)
	require.True(t, loaded.Resumable)
}

func TestMetadata_SessionResumptionFields_OmitEmpty(t *testing.T) {
	// Test that empty CoordinatorSessionRef and false Resumable are omitted from JSON
	dir := t.TempDir()

	meta := &Metadata{
		SessionID:  "omit-empty-resumption-test",
		StartTime:  time.Now(),
		Status:     StatusRunning,
		SessionDir: "/test",
		Workers:    []WorkerMetadata{},
		ClientType: "claude",
		// CoordinatorSessionRef intentionally empty
		// Resumable intentionally false (default)
	}

	err := meta.Save(dir)
	require.NoError(t, err)

	// Read raw JSON
	data, err := os.ReadFile(filepath.Join(dir, metadataFilename))
	require.NoError(t, err)

	jsonStr := string(data)
	// Verify optional fields are omitted when empty/false
	require.NotContains(t, jsonStr, "coordinator_session_ref")
	require.NotContains(t, jsonStr, "resumable")
}

func TestMetadata_BackwardCompatibility_SessionResumption(t *testing.T) {
	// Test that old metadata JSON without session resumption fields can still be loaded
	dir := t.TempDir()

	// Write old-style metadata JSON without the new resumption fields
	oldJSON := `{
  "session_id": "old-session-no-resumption",
  "start_time": "2026-01-01T10:00:00Z",
  "status": "completed",
  "session_dir": "/test",
  "coordinator_id": "coordinator",
  "workers": [],
  "client_type": "claude"
}`
	err := os.WriteFile(filepath.Join(dir, metadataFilename), []byte(oldJSON), 0600)
	require.NoError(t, err)

	// Load should succeed and new resumption fields should have zero values
	loaded, err := Load(dir)
	require.NoError(t, err)
	require.Equal(t, "old-session-no-resumption", loaded.SessionID)
	require.Equal(t, "coordinator", loaded.CoordinatorID)
	require.Empty(t, loaded.CoordinatorSessionRef)
	require.False(t, loaded.Resumable)
}

func TestWorkerMetadata_SessionResumptionFields(t *testing.T) {
	// Test that HeadlessSessionRef and WorkDir fields serialize correctly
	dir := t.TempDir()

	now := time.Now().Truncate(time.Second)
	meta := &Metadata{
		SessionID:  "worker-resumption-test",
		StartTime:  now,
		Status:     StatusRunning,
		SessionDir: "/test/project",
		Workers: []WorkerMetadata{
			{
				ID:                 "worker-1",
				SpawnedAt:          now.Add(time.Minute),
				FinalPhase:         "implementing",
				HeadlessSessionRef: "claude-worker-session-abc",
				WorkDir:            "/Users/dev/project",
			},
			{
				ID:        "worker-2",
				SpawnedAt: now.Add(2 * time.Minute),
				// HeadlessSessionRef and WorkDir intentionally empty
			},
		},
		ClientType: "claude",
	}

	// Save metadata
	err := meta.Save(dir)
	require.NoError(t, err)

	// Load metadata
	loaded, err := Load(dir)
	require.NoError(t, err)

	// Verify workers
	require.Len(t, loaded.Workers, 2)

	// Verify first worker has session resumption fields
	require.Equal(t, "worker-1", loaded.Workers[0].ID)
	require.Equal(t, "claude-worker-session-abc", loaded.Workers[0].HeadlessSessionRef)
	require.Equal(t, "/Users/dev/project", loaded.Workers[0].WorkDir)

	// Verify second worker has empty session resumption fields
	require.Equal(t, "worker-2", loaded.Workers[1].ID)
	require.Empty(t, loaded.Workers[1].HeadlessSessionRef)
	require.Empty(t, loaded.Workers[1].WorkDir)
}

func TestWorkerMetadata_SessionResumptionFields_OmitEmpty(t *testing.T) {
	// Test that empty HeadlessSessionRef and WorkDir are omitted from JSON
	dir := t.TempDir()

	meta := &Metadata{
		SessionID:  "worker-omit-empty-test",
		StartTime:  time.Now(),
		Status:     StatusRunning,
		SessionDir: "/test",
		Workers: []WorkerMetadata{
			{
				ID:        "worker-1",
				SpawnedAt: time.Now(),
				// HeadlessSessionRef and WorkDir intentionally empty
			},
		},
		ClientType: "claude",
	}

	err := meta.Save(dir)
	require.NoError(t, err)

	// Read raw JSON
	data, err := os.ReadFile(filepath.Join(dir, metadataFilename))
	require.NoError(t, err)

	jsonStr := string(data)
	// Verify optional fields are omitted when empty
	require.NotContains(t, jsonStr, "headless_session_ref")
	// Note: We check for worker work_dir specifically, not session work_dir
	// The workers array should not contain work_dir for empty values
	// Parse to verify structure
	var parsed map[string]interface{}
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	workers := parsed["workers"].([]interface{})
	require.Len(t, workers, 1)
	worker := workers[0].(map[string]interface{})
	_, hasWorkDir := worker["work_dir"]
	require.False(t, hasWorkDir, "work_dir should be omitted when empty")
}

func TestWorkerMetadata_BackwardCompatibility_SessionResumption(t *testing.T) {
	// Test that old WorkerMetadata JSON without session resumption fields can still be loaded
	dir := t.TempDir()

	// Write old-style metadata JSON with workers that lack the new resumption fields
	oldJSON := `{
  "session_id": "old-worker-session",
  "start_time": "2026-01-01T10:00:00Z",
  "status": "completed",
  "session_dir": "/test",
  "workers": [
    {
      "id": "worker-1",
      "spawned_at": "2026-01-01T10:01:00Z",
      "retired_at": "2026-01-01T10:30:00Z",
      "final_phase": "idle"
    }
  ],
  "client_type": "claude"
}`
	err := os.WriteFile(filepath.Join(dir, metadataFilename), []byte(oldJSON), 0600)
	require.NoError(t, err)

	// Load should succeed and new worker resumption fields should have zero values
	loaded, err := Load(dir)
	require.NoError(t, err)
	require.Len(t, loaded.Workers, 1)
	require.Equal(t, "worker-1", loaded.Workers[0].ID)
	require.Equal(t, "idle", loaded.Workers[0].FinalPhase)
	require.Empty(t, loaded.Workers[0].HeadlessSessionRef)
	require.Empty(t, loaded.Workers[0].WorkDir)
}

func TestMetadata_FullSessionResumption(t *testing.T) {
	// Integration test: full metadata with all session resumption fields
	dir := t.TempDir()

	now := time.Now().Truncate(time.Second)
	meta := &Metadata{
		SessionID:             "full-resumption-session",
		StartTime:             now,
		EndTime:               now.Add(time.Hour),
		Status:                StatusCompleted,
		SessionDir:            "/home/user/.perles/sessions/2026-01-11/full-resumption-session",
		CoordinatorID:         "coordinator",
		CoordinatorSessionRef: "claude-coord-session-main-12345",
		Resumable:             true,
		Workers: []WorkerMetadata{
			{
				ID:                 "worker-1",
				SpawnedAt:          now.Add(time.Minute),
				RetiredAt:          now.Add(30 * time.Minute),
				FinalPhase:         "idle",
				HeadlessSessionRef: "claude-worker-session-1-abc",
				WorkDir:            "/home/user/project",
			},
			{
				ID:                 "worker-2",
				SpawnedAt:          now.Add(5 * time.Minute),
				FinalPhase:         "implementing",
				HeadlessSessionRef: "claude-worker-session-2-def",
				WorkDir:            "/home/user/project",
			},
		},
		ClientType:      "claude",
		Model:           "sonnet",
		ApplicationName: "my-project",
		WorkDir:         "/home/user/project",
		DatePartition:   "2026-01-11",
		TokenUsage: TokenUsageSummary{
			ContextTokens:     50000,
			TotalOutputTokens: 15000,
			TotalCostUSD:      1.25,
		},
	}

	// Save metadata
	err := meta.Save(dir)
	require.NoError(t, err)

	// Load metadata
	loaded, err := Load(dir)
	require.NoError(t, err)

	// Verify all session resumption fields
	require.Equal(t, "claude-coord-session-main-12345", loaded.CoordinatorSessionRef)
	require.True(t, loaded.Resumable)

	// Verify worker session resumption fields
	require.Len(t, loaded.Workers, 2)
	require.Equal(t, "claude-worker-session-1-abc", loaded.Workers[0].HeadlessSessionRef)
	require.Equal(t, "/home/user/project", loaded.Workers[0].WorkDir)
	require.Equal(t, "claude-worker-session-2-def", loaded.Workers[1].HeadlessSessionRef)
	require.Equal(t, "/home/user/project", loaded.Workers[1].WorkDir)

	// Verify non-resumption fields still work
	require.Equal(t, "full-resumption-session", loaded.SessionID)
	require.Equal(t, "coordinator", loaded.CoordinatorID)
	require.Equal(t, "my-project", loaded.ApplicationName)
}

// Tests for New() constructor

func TestNew_CreatesDirectoryStructure(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-session-123"
	sessionDir := filepath.Join(baseDir, ".perles", "sessions", sessionID)

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	require.NotNil(t, session)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	// Verify session fields
	require.Equal(t, sessionID, session.ID)
	require.Equal(t, sessionDir, session.Dir)
	require.Equal(t, StatusRunning, session.Status)
	require.False(t, session.StartTime.IsZero())

	// Verify main session directory exists
	info, err := os.Stat(sessionDir)
	require.NoError(t, err)
	require.True(t, info.IsDir())

	// Verify coordinator directory exists
	coordDir := filepath.Join(sessionDir, "coordinator")
	info, err = os.Stat(coordDir)
	require.NoError(t, err)
	require.True(t, info.IsDir())

	// Verify coordinator/messages.jsonl exists (structured chat messages)
	coordMessages := filepath.Join(coordDir, "messages.jsonl")
	info, err = os.Stat(coordMessages)
	require.NoError(t, err)
	require.False(t, info.IsDir())

	// Verify coordinator/raw.jsonl exists
	coordRaw := filepath.Join(coordDir, "raw.jsonl")
	info, err = os.Stat(coordRaw)
	require.NoError(t, err)
	require.False(t, info.IsDir())

	// Verify workers directory exists
	workersDir := filepath.Join(sessionDir, "workers")
	info, err = os.Stat(workersDir)
	require.NoError(t, err)
	require.True(t, info.IsDir())

	// Verify messages.jsonl exists
	messagesFile := filepath.Join(sessionDir, "messages.jsonl")
	info, err = os.Stat(messagesFile)
	require.NoError(t, err)
	require.False(t, info.IsDir())

	// Verify mcp_requests.jsonl exists
	mcpFile := filepath.Join(sessionDir, "mcp_requests.jsonl")
	info, err = os.Stat(mcpFile)
	require.NoError(t, err)
	require.False(t, info.IsDir())

	// Verify metadata.json exists
	metadataFile := filepath.Join(sessionDir, "metadata.json")
	info, err = os.Stat(metadataFile)
	require.NoError(t, err)
	require.False(t, info.IsDir())
}

func TestNew_WritesInitialMetadata(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-session-456"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	require.NotNil(t, session)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	// Load and verify the metadata
	meta, err := Load(sessionDir)
	require.NoError(t, err)

	require.Equal(t, sessionID, meta.SessionID)
	require.Equal(t, StatusRunning, meta.Status)
	require.Equal(t, sessionDir, meta.SessionDir)
	require.False(t, meta.StartTime.IsZero())
	require.True(t, meta.EndTime.IsZero())
	require.Empty(t, meta.CoordinatorID)
	require.NotNil(t, meta.Workers)
	require.Empty(t, meta.Workers)
}

func TestNew_FailsOnInvalidDir(t *testing.T) {
	// Skip on Windows where permissions work differently
	if runtime.GOOS == "windows" {
		t.Skip("skipping permission test on Windows")
	}

	// Test 1: Non-existent parent with no write permission
	// Create a directory and make it read-only
	baseDir := t.TempDir()
	readOnlyDir := filepath.Join(baseDir, "readonly")
	err := os.MkdirAll(readOnlyDir, 0500) // read + execute only
	require.NoError(t, err)

	// Try to create a session in a subdirectory of the read-only dir
	sessionDir := filepath.Join(readOnlyDir, "sessions", "test-id")
	session, err := New("test-id", sessionDir)
	require.Error(t, err)
	require.Nil(t, session)
	require.Contains(t, err.Error(), "creating session directory")

	// Cleanup: restore permissions so t.TempDir can clean up
	_ = os.Chmod(readOnlyDir, 0750)
}

func TestNew_FailsOnExistingReadOnlyDir(t *testing.T) {
	// Skip on Windows where permissions work differently
	if runtime.GOOS == "windows" {
		t.Skip("skipping permission test on Windows")
	}

	// Create a session directory that's read-only
	baseDir := t.TempDir()
	sessionDir := filepath.Join(baseDir, "session")
	err := os.MkdirAll(sessionDir, 0500) // read + execute only
	require.NoError(t, err)

	session, err := New("test-id", sessionDir)
	require.Error(t, err)
	require.Nil(t, session)
	require.Contains(t, err.Error(), "not writable")

	// Cleanup: restore permissions
	_ = os.Chmod(sessionDir, 0750)
}

func TestNew_SessionFieldsInitialized(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-session-789"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	require.NotNil(t, session)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	// Verify all session fields are properly initialized
	require.Equal(t, sessionID, session.ID)
	require.Equal(t, sessionDir, session.Dir)
	require.Equal(t, StatusRunning, session.Status)
	require.False(t, session.StartTime.IsZero())
	require.NotNil(t, session.workerRaws)
	require.Empty(t, session.workerRaws)
	require.NotNil(t, session.workerMessages)
	require.Empty(t, session.workerMessages)
	require.False(t, session.closed)

	// File handles should be non-nil
	require.NotNil(t, session.coordRaw)
	require.NotNil(t, session.coordMessages)
	require.NotNil(t, session.messageLog)
	require.NotNil(t, session.mcpLog)
}

func TestNew_MetadataJSONFormat(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "json-format-test"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	// Read the raw JSON file
	metadataPath := filepath.Join(sessionDir, "metadata.json")
	data, err := os.ReadFile(metadataPath)
	require.NoError(t, err)

	// Verify JSON structure contains expected fields
	jsonStr := string(data)
	require.Contains(t, jsonStr, `"session_id"`)
	require.Contains(t, jsonStr, `"start_time"`)
	require.Contains(t, jsonStr, `"status"`)
	require.Contains(t, jsonStr, `"session_dir"`)
	require.Contains(t, jsonStr, `"workers"`)

	// Verify status is "running"
	require.Contains(t, jsonStr, `"status": "running"`)

	// Verify session_id matches
	require.Contains(t, jsonStr, `"session_id": "json-format-test"`)
}

// TestSession_New_CreatesMessagesJSONL verifies that New() creates the coordinator/messages.jsonl file.
// This file is used for structured chat message persistence.
func TestSession_New_CreatesMessagesJSONL(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-messages-jsonl-creation"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	require.NotNil(t, session)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	// Verify coordinator/messages.jsonl exists
	msgsPath := filepath.Join(sessionDir, "coordinator", "messages.jsonl")
	info, err := os.Stat(msgsPath)
	require.NoError(t, err, "coordinator/messages.jsonl should be created by New()")
	require.False(t, info.IsDir())
	require.Equal(t, int64(0), info.Size(), "messages.jsonl should be empty initially")

	// Verify coordMessages field is initialized
	require.NotNil(t, session.coordMessages, "coordMessages writer should be initialized")

	// Verify workerMessages map is initialized (empty)
	require.NotNil(t, session.workerMessages, "workerMessages map should be initialized")
	require.Empty(t, session.workerMessages, "workerMessages should be empty initially")
}

// Tests for BufferedWriter

func TestBufferedWriter_Write(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.log")

	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
	require.NoError(t, err)

	writer := NewBufferedWriter(file)
	require.NotNil(t, writer)

	// Write some data
	err = writer.Write([]byte("line 1\n"))
	require.NoError(t, err)

	err = writer.Write([]byte("line 2\n"))
	require.NoError(t, err)

	// Buffer should have 2 items
	require.Equal(t, 2, writer.Len())

	// Close triggers final flush
	err = writer.Close()
	require.NoError(t, err)

	// Verify file contents
	data, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.Equal(t, "line 1\nline 2\n", string(data))
}

func TestBufferedWriter_FlushOnCapacity(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.log")

	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
	require.NoError(t, err)

	// Use a small buffer size for testing (16 events, flush at 12 = 75%)
	writer := NewBufferedWriterWithConfig(file, 16, 10*time.Second) // Long flush interval to avoid timer flush
	require.NotNil(t, writer)

	// Write 11 events (below threshold of 12)
	for i := 0; i < 11; i++ {
		err = writer.Write([]byte("event\n"))
		require.NoError(t, err)
	}

	// Buffer should still have 11 items (not flushed yet)
	require.Equal(t, 11, writer.Len())

	// Check file is still empty (no flush yet)
	data, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.Empty(t, data)

	// Write 12th event (triggers flush at 75% = 12/16)
	err = writer.Write([]byte("event\n"))
	require.NoError(t, err)

	// Buffer should be empty after flush
	require.Equal(t, 0, writer.Len())

	// Verify file has 12 events
	data, err = os.ReadFile(filePath)
	require.NoError(t, err)
	require.Equal(t, 12, countLines(data))

	err = writer.Close()
	require.NoError(t, err)
}

func TestBufferedWriter_FlushOnTimer(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.log")

	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
	require.NoError(t, err)

	// Use a short flush interval for testing (50ms)
	writer := NewBufferedWriterWithConfig(file, 256, 50*time.Millisecond)
	require.NotNil(t, writer)

	// Write a few events (well below threshold)
	for i := 0; i < 5; i++ {
		err = writer.Write([]byte("event\n"))
		require.NoError(t, err)
	}

	// Buffer should have 5 items
	require.Equal(t, 5, writer.Len())

	// Wait for periodic flush (with some margin)
	time.Sleep(100 * time.Millisecond)

	// Buffer should be empty after timer flush
	require.Equal(t, 0, writer.Len())

	// Verify file has the events
	data, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.Equal(t, 5, countLines(data))

	err = writer.Close()
	require.NoError(t, err)
}

func TestBufferedWriter_ConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.log")

	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
	require.NoError(t, err)

	writer := NewBufferedWriter(file)
	require.NotNil(t, writer)

	// Spawn 10 goroutines, each writing 100 events
	const numGoroutines = 10
	const eventsPerGoroutine = 100
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				data := []byte(time.Now().Format(time.RFC3339Nano) + " event\n")
				_ = writer.Write(data)
			}
		}(i)
	}

	wg.Wait()

	// Close to flush remaining events
	err = writer.Close()
	require.NoError(t, err)

	// Verify file has all events (1000 total)
	data, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.Equal(t, numGoroutines*eventsPerGoroutine, countLines(data))
}

func TestBufferedWriter_Close(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.log")

	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
	require.NoError(t, err)

	// Use a long flush interval to ensure timer doesn't flush
	writer := NewBufferedWriterWithConfig(file, 256, 10*time.Second)
	require.NotNil(t, writer)

	// Write some events
	for i := 0; i < 10; i++ {
		err = writer.Write([]byte("event\n"))
		require.NoError(t, err)
	}

	// Buffer should have 10 items
	require.Equal(t, 10, writer.Len())

	// Close should flush all remaining events
	err = writer.Close()
	require.NoError(t, err)

	// Verify file has all events
	data, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.Equal(t, 10, countLines(data))

	// Writing after close should fail
	err = writer.Write([]byte("should fail\n"))
	require.Error(t, err)
	require.Equal(t, os.ErrClosed, err)

	// Closing again should fail
	err = writer.Close()
	require.Error(t, err)
	require.Equal(t, os.ErrClosed, err)
}

func TestBufferedWriter_ErrorTracking(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.log")

	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
	require.NoError(t, err)

	writer := NewBufferedWriter(file)
	require.NotNil(t, writer)

	// Initial state: no errors
	require.Equal(t, int64(0), writer.ErrorCount())
	require.Nil(t, writer.LastError())

	// Write some data
	err = writer.Write([]byte("line 1\n"))
	require.NoError(t, err)

	// Close the underlying file to simulate a write error
	file.Close()

	// Trigger a flush (this should fail)
	err = writer.Flush()
	require.Error(t, err) // Flush returns error

	// Error should be tracked
	require.Greater(t, writer.ErrorCount(), int64(0))
	require.NotNil(t, writer.LastError())

	// Close the writer (cleanup)
	_ = writer.Close()
}

func TestBufferedWriter_ExplicitFlush(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.log")

	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
	require.NoError(t, err)

	// Use a long flush interval to ensure timer doesn't interfere
	writer := NewBufferedWriterWithConfig(file, 256, 10*time.Second)
	require.NotNil(t, writer)

	// Write some events
	for i := 0; i < 5; i++ {
		err = writer.Write([]byte("event\n"))
		require.NoError(t, err)
	}

	// Buffer should have 5 items
	require.Equal(t, 5, writer.Len())

	// Explicit flush
	err = writer.Flush()
	require.NoError(t, err)

	// Buffer should be empty
	require.Equal(t, 0, writer.Len())

	// Verify file has the events
	data, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.Equal(t, 5, countLines(data))

	err = writer.Close()
	require.NoError(t, err)
}

func TestBufferedWriter_EmptyFlush(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.log")

	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
	require.NoError(t, err)

	writer := NewBufferedWriter(file)
	require.NotNil(t, writer)

	// Flush with empty buffer should succeed
	err = writer.Flush()
	require.NoError(t, err)

	// Buffer should still be empty
	require.Equal(t, 0, writer.Len())

	err = writer.Close()
	require.NoError(t, err)
}

func TestBufferedWriter_DataIntegrity(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.log")

	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
	require.NoError(t, err)

	writer := NewBufferedWriter(file)
	require.NotNil(t, writer)

	// Write data and then modify the original slice
	data := []byte("original data\n")
	err = writer.Write(data)
	require.NoError(t, err)

	// Modify the original slice
	copy(data, []byte("MODIFIED!!!!!"))

	// Close to flush
	err = writer.Close()
	require.NoError(t, err)

	// Verify the original data was written (not the modified version)
	fileData, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.Equal(t, "original data\n", string(fileData))
}

// countLines counts the number of newline characters in the data.
func countLines(data []byte) int {
	count := 0
	for _, b := range data {
		if b == '\n' {
			count++
		}
	}
	return count
}

// Tests for WriteCoordinatorMessage

func TestSession_WriteCoordinatorMessage(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-coord-message"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	require.NotNil(t, session)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	// Write coordinator messages
	ts1 := time.Date(2026, 1, 11, 14, 30, 0, 0, time.UTC)
	msg1 := chatrender.Message{
		Role:      "assistant",
		Content:   "Hello, I'm starting work.",
		Timestamp: &ts1,
	}
	err = session.WriteCoordinatorMessage(msg1)
	require.NoError(t, err)

	ts2 := time.Date(2026, 1, 11, 14, 31, 0, 0, time.UTC)
	msg2 := chatrender.Message{
		Role:       "assistant",
		Content:    "🔧 Read(file.go)",
		IsToolCall: true,
		Timestamp:  &ts2,
	}
	err = session.WriteCoordinatorMessage(msg2)
	require.NoError(t, err)

	// Close to flush buffers
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify file contents (JSONL format)
	msgsPath := filepath.Join(sessionDir, "coordinator", "messages.jsonl")
	data, err := os.ReadFile(msgsPath)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 2)

	// Verify first message is valid JSON and has correct content
	var decoded1 chatrender.Message
	err = json.Unmarshal([]byte(lines[0]), &decoded1)
	require.NoError(t, err)
	require.Equal(t, "assistant", decoded1.Role)
	require.Equal(t, "Hello, I'm starting work.", decoded1.Content)
	require.NotNil(t, decoded1.Timestamp)
	require.True(t, decoded1.Timestamp.Equal(ts1))

	// Verify second message (tool call)
	var decoded2 chatrender.Message
	err = json.Unmarshal([]byte(lines[1]), &decoded2)
	require.NoError(t, err)
	require.Equal(t, "assistant", decoded2.Role)
	require.Equal(t, "🔧 Read(file.go)", decoded2.Content)
	require.True(t, decoded2.IsToolCall)
}

func TestSession_WriteCoordinatorMessage_AfterClose(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-coord-message-closed"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Writing after close should fail
	ts := time.Now()
	msg := chatrender.Message{
		Role:      "assistant",
		Content:   "should fail",
		Timestamp: &ts,
	}
	err = session.WriteCoordinatorMessage(msg)
	require.Error(t, err)
	require.Equal(t, os.ErrClosed, err)
}

// Tests for WriteWorkerMessage

func TestSession_WriteWorkerMessage(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-worker-message"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	require.NotNil(t, session)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	// Write messages for different workers
	ts1 := time.Date(2026, 1, 11, 14, 30, 0, 0, time.UTC)
	msg1 := chatrender.Message{
		Role:      "assistant",
		Content:   "Starting task implementation...",
		Timestamp: &ts1,
	}
	err = session.WriteWorkerMessage("worker-1", msg1)
	require.NoError(t, err)

	ts2 := time.Date(2026, 1, 11, 14, 31, 0, 0, time.UTC)
	msg2 := chatrender.Message{
		Role:      "assistant",
		Content:   "Ready for task assignment",
		Timestamp: &ts2,
	}
	err = session.WriteWorkerMessage("worker-2", msg2)
	require.NoError(t, err)

	// Write another message for worker-1
	ts3 := time.Date(2026, 1, 11, 14, 32, 0, 0, time.UTC)
	msg3 := chatrender.Message{
		Role:      "assistant",
		Content:   "Task completed",
		Timestamp: &ts3,
	}
	err = session.WriteWorkerMessage("worker-1", msg3)
	require.NoError(t, err)

	// Close to flush buffers
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify worker-1 messages.jsonl
	worker1MsgsPath := filepath.Join(sessionDir, "workers", "worker-1", "messages.jsonl")
	data1, err := os.ReadFile(worker1MsgsPath)
	require.NoError(t, err)

	lines1 := strings.Split(strings.TrimSpace(string(data1)), "\n")
	require.Len(t, lines1, 2)

	var decoded1 chatrender.Message
	err = json.Unmarshal([]byte(lines1[0]), &decoded1)
	require.NoError(t, err)
	require.Equal(t, "Starting task implementation...", decoded1.Content)

	var decoded3 chatrender.Message
	err = json.Unmarshal([]byte(lines1[1]), &decoded3)
	require.NoError(t, err)
	require.Equal(t, "Task completed", decoded3.Content)

	// Verify worker-2 messages.jsonl
	worker2MsgsPath := filepath.Join(sessionDir, "workers", "worker-2", "messages.jsonl")
	data2, err := os.ReadFile(worker2MsgsPath)
	require.NoError(t, err)

	lines2 := strings.Split(strings.TrimSpace(string(data2)), "\n")
	require.Len(t, lines2, 1)

	var decoded2 chatrender.Message
	err = json.Unmarshal([]byte(lines2[0]), &decoded2)
	require.NoError(t, err)
	require.Equal(t, "Ready for task assignment", decoded2.Content)
}

func TestSession_WriteWorkerMessage_LazyCreation(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-worker-message-lazy"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	// Worker directory shouldn't exist yet (workers/worker-3)
	worker3Path := filepath.Join(sessionDir, "workers", "worker-3")
	_, err = os.Stat(worker3Path)
	require.True(t, os.IsNotExist(err))

	// Write message - should create directory and file
	ts := time.Now()
	msg := chatrender.Message{
		Role:      "assistant",
		Content:   "Hello from worker-3",
		Timestamp: &ts,
	}
	err = session.WriteWorkerMessage("worker-3", msg)
	require.NoError(t, err)

	// Worker directory should now exist
	info, err := os.Stat(worker3Path)
	require.NoError(t, err)
	require.True(t, info.IsDir())

	// messages.jsonl should exist
	msgsPath := filepath.Join(worker3Path, "messages.jsonl")
	_, err = os.Stat(msgsPath)
	require.NoError(t, err)

	err = session.Close(StatusCompleted)
	require.NoError(t, err)
}

// Tests for WriteCoordinatorRawJSON

func TestSession_WriteCoordinatorRawJSON(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-coord-raw"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	// Write raw JSON entries
	json1 := []byte(`{"type":"chat","content":"Hello"}`)
	err = session.WriteCoordinatorRawJSON(time.Now(), json1)
	require.NoError(t, err)

	json2 := []byte(`{"type":"tool_result","content":"Success"}`)
	err = session.WriteCoordinatorRawJSON(time.Now(), json2)
	require.NoError(t, err)

	// Close to flush
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify file contents (JSONL format - each line is a JSON object)
	rawPath := filepath.Join(sessionDir, "coordinator", "raw.jsonl")
	data, err := os.ReadFile(rawPath)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 2)
	require.Equal(t, `{"type":"chat","content":"Hello"}`, lines[0])
	require.Equal(t, `{"type":"tool_result","content":"Success"}`, lines[1])
}

func TestSession_WriteCoordinatorRawJSON_AddsNewline(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-coord-raw-newline"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	// Write JSON without trailing newline
	json1 := []byte(`{"type":"test"}`)
	err = session.WriteCoordinatorRawJSON(time.Now(), json1)
	require.NoError(t, err)

	// Write JSON with trailing newline
	json2 := []byte("{\"type\":\"test2\"}\n")
	err = session.WriteCoordinatorRawJSON(time.Now(), json2)
	require.NoError(t, err)

	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Both should be properly formatted as JSONL
	rawPath := filepath.Join(sessionDir, "coordinator", "raw.jsonl")
	data, err := os.ReadFile(rawPath)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 2)
}

// Tests for WriteWorkerRawJSON

func TestSession_WriteWorkerRawJSON(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-worker-raw"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	// Write raw JSON for a worker
	json1 := []byte(`{"type":"output","content":"Working on task..."}`)
	err = session.WriteWorkerRawJSON("worker-1", time.Now(), json1)
	require.NoError(t, err)

	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify file exists and has correct content
	rawPath := filepath.Join(sessionDir, "workers", "worker-1", "raw.jsonl")
	data, err := os.ReadFile(rawPath)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 1)
	require.Equal(t, `{"type":"output","content":"Working on task..."}`, lines[0])
}

// Tests for WriteMessage

func TestSession_WriteMessage(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-message"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	// Write message entries
	timestamp := time.Date(2025, 1, 15, 10, 30, 45, 123456789, time.UTC)
	entry1 := message.Entry{
		ID:        "msg-001",
		Timestamp: timestamp,
		From:      "COORDINATOR",
		To:        "ALL",
		Content:   "Starting work on epic",
		Type:      message.MessageInfo,
	}
	err = session.WriteMessage(entry1)
	require.NoError(t, err)

	entry2 := message.Entry{
		ID:        "msg-002",
		Timestamp: timestamp.Add(time.Second),
		From:      "WORKER.1",
		To:        "COORDINATOR",
		Content:   "Ready for task",
		Type:      message.MessageWorkerReady,
	}
	err = session.WriteMessage(entry2)
	require.NoError(t, err)

	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify JSONL format
	messagesPath := filepath.Join(sessionDir, "messages.jsonl")
	data, err := os.ReadFile(messagesPath)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 2)

	// Parse and verify first message
	var parsed1 message.Entry
	err = json.Unmarshal([]byte(lines[0]), &parsed1)
	require.NoError(t, err)
	require.Equal(t, "msg-001", parsed1.ID)
	require.Equal(t, "COORDINATOR", parsed1.From)
	require.Equal(t, "ALL", parsed1.To)
	require.Equal(t, "Starting work on epic", parsed1.Content)
	require.Equal(t, message.MessageInfo, parsed1.Type)

	// Parse and verify second message
	var parsed2 message.Entry
	err = json.Unmarshal([]byte(lines[1]), &parsed2)
	require.NoError(t, err)
	require.Equal(t, "msg-002", parsed2.ID)
	require.Equal(t, "WORKER.1", parsed2.From)
}

// Tests for WriteMCPEvent

func TestSession_WriteMCPEvent(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-mcp-event"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	// Write MCP events
	timestamp := time.Date(2025, 1, 15, 10, 30, 45, 0, time.UTC)
	event1 := events.MCPEvent{
		Timestamp:    timestamp,
		Type:         events.MCPToolCall,
		Method:       "tools/call",
		ToolName:     "spawn_worker",
		WorkerID:     "",
		RequestJSON:  []byte(`{"workerID":"worker-1"}`),
		ResponseJSON: nil,
		Duration:     50 * time.Millisecond,
	}
	err = session.WriteMCPEvent(event1)
	require.NoError(t, err)

	event2 := events.MCPEvent{
		Timestamp:    timestamp.Add(100 * time.Millisecond),
		Type:         events.MCPToolResult,
		Method:       "tools/call",
		ToolName:     "spawn_worker",
		WorkerID:     "",
		RequestJSON:  nil,
		ResponseJSON: []byte(`{"success":true,"workerID":"worker-1"}`),
		Duration:     50 * time.Millisecond,
	}
	err = session.WriteMCPEvent(event2)
	require.NoError(t, err)

	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify JSONL format
	mcpPath := filepath.Join(sessionDir, "mcp_requests.jsonl")
	data, err := os.ReadFile(mcpPath)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 2)

	// Parse and verify first event
	var parsed1 events.MCPEvent
	err = json.Unmarshal([]byte(lines[0]), &parsed1)
	require.NoError(t, err)
	require.Equal(t, events.MCPToolCall, parsed1.Type)
	require.Equal(t, "spawn_worker", parsed1.ToolName)
	require.Equal(t, "tools/call", parsed1.Method)

	// Parse and verify second event
	var parsed2 events.MCPEvent
	err = json.Unmarshal([]byte(lines[1]), &parsed2)
	require.NoError(t, err)
	require.Equal(t, events.MCPToolResult, parsed2.Type)
}

func TestSession_WriteCommandEvent(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-command-event"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	// Write command events
	timestamp := time.Date(2025, 1, 15, 10, 30, 45, 0, time.UTC)
	event1 := processor.CommandEvent{
		CommandID:   "cmd-1",
		CommandType: "spawn_process",
		Source:      "mcp_tool",
		Success:     true,
		Error:       "",
		DurationMs:  50,
		Timestamp:   timestamp,
		TraceID:     "trace-123",
	}
	err = session.WriteCommandEvent(event1)
	require.NoError(t, err)

	event2 := processor.CommandEvent{
		CommandID:   "cmd-2",
		CommandType: "assign_task",
		Source:      "internal",
		Success:     false,
		Error:       "task not found",
		DurationMs:  25,
		Timestamp:   timestamp.Add(100 * time.Millisecond),
		TraceID:     "",
	}
	err = session.WriteCommandEvent(event2)
	require.NoError(t, err)

	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify JSONL format
	commandsPath := filepath.Join(sessionDir, "commands.jsonl")
	data, err := os.ReadFile(commandsPath)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 2)

	// Parse and verify first event
	var parsed1 processor.CommandEvent
	err = json.Unmarshal([]byte(lines[0]), &parsed1)
	require.NoError(t, err)
	require.Equal(t, "cmd-1", parsed1.CommandID)
	require.Equal(t, "spawn_process", parsed1.CommandType)
	require.Equal(t, "mcp_tool", parsed1.Source)
	require.True(t, parsed1.Success)
	require.Empty(t, parsed1.Error)
	require.Equal(t, int64(50), parsed1.DurationMs)
	require.Equal(t, "trace-123", parsed1.TraceID)

	// Parse and verify second event
	var parsed2 processor.CommandEvent
	err = json.Unmarshal([]byte(lines[1]), &parsed2)
	require.NoError(t, err)
	require.Equal(t, "cmd-2", parsed2.CommandID)
	require.Equal(t, "assign_task", parsed2.CommandType)
	require.False(t, parsed2.Success)
	require.Equal(t, "task not found", parsed2.Error)
}

// Tests for Close

func TestSession_Close(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-close"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	// Write some messages before closing
	ts := time.Now().UTC()
	err = session.WriteCoordinatorMessage(chatrender.Message{
		Role:      "assistant",
		Content:   "Starting...",
		Timestamp: &ts,
	})
	require.NoError(t, err)

	err = session.WriteWorkerMessage("worker-1", chatrender.Message{
		Role:      "assistant",
		Content:   "Working...",
		Timestamp: &ts,
	})
	require.NoError(t, err)

	// Close with completed status
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify metadata was updated
	meta, err := Load(sessionDir)
	require.NoError(t, err)
	require.Equal(t, StatusCompleted, meta.Status)
	require.False(t, meta.EndTime.IsZero())

	// Verify summary.md was generated
	summaryPath := filepath.Join(sessionDir, "summary.md")
	summaryData, err := os.ReadFile(summaryPath)
	require.NoError(t, err)
	require.Contains(t, string(summaryData), "# Session Summary")
	require.Contains(t, string(summaryData), sessionID)
	require.Contains(t, string(summaryData), "completed")
}

func TestSession_Close_FlushesBuffers(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-close-flush"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	// Write several messages (below flush threshold)
	for i := 0; i < 10; i++ {
		ts := time.Date(2026, 1, 11, 14, 30, i, 0, time.UTC)
		err = session.WriteCoordinatorMessage(chatrender.Message{
			Role:      "assistant",
			Content:   "Event",
			Timestamp: &ts,
		})
		require.NoError(t, err)
	}

	// Close should flush all buffered messages
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify all messages were written to messages.jsonl
	msgsPath := filepath.Join(sessionDir, "coordinator", "messages.jsonl")
	data, err := os.ReadFile(msgsPath)
	require.NoError(t, err)
	require.Equal(t, 10, countLines(data))
}

// TestSession_Close_FlushesMessagesJSONL verifies that Close() properly flushes
// buffered chat messages to coordinator/messages.jsonl.
func TestSession_Close_FlushesMessagesJSONL(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-close-flush-messages"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)

	// Write several messages (below flush threshold)
	for i := 0; i < 10; i++ {
		ts := time.Date(2026, 1, 11, 14, 30, i, 0, time.UTC)
		msg := chatrender.Message{
			Role:      "assistant",
			Content:   fmt.Sprintf("Message %d", i),
			Timestamp: &ts,
		}
		err = session.WriteCoordinatorMessage(msg)
		require.NoError(t, err)
	}

	// Close should flush all buffered messages
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify all messages were written to messages.jsonl
	msgsPath := filepath.Join(sessionDir, "coordinator", "messages.jsonl")
	data, err := os.ReadFile(msgsPath)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 10, "should have 10 JSONL lines")

	// Verify each line is valid JSON
	for i, line := range lines {
		var msg chatrender.Message
		err = json.Unmarshal([]byte(line), &msg)
		require.NoError(t, err, "line %d should be valid JSON", i)
		require.Equal(t, "assistant", msg.Role)
		require.Contains(t, msg.Content, "Message")
		require.NotNil(t, msg.Timestamp)
	}
}

func TestSession_Close_DoubleClose(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-double-close"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	// First close should succeed
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Second close should fail
	err = session.Close(StatusFailed)
	require.Error(t, err)
	require.Equal(t, os.ErrClosed, err)
}

func TestSession_Close_DifferentStatuses(t *testing.T) {
	statuses := []Status{StatusCompleted, StatusFailed, StatusTimedOut}

	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			baseDir := t.TempDir()
			sessionID := "test-status-" + string(status)
			sessionDir := filepath.Join(baseDir, "session")

			session, err := New(sessionID, sessionDir)
			require.NoError(t, err)
			t.Cleanup(func() { _ = session.Close(StatusCompleted) })

			err = session.Close(status)
			require.NoError(t, err)

			// Verify metadata has correct status
			meta, err := Load(sessionDir)
			require.NoError(t, err)
			require.Equal(t, status, meta.Status)

			// Verify summary mentions the status
			summaryPath := filepath.Join(sessionDir, "summary.md")
			summaryData, err := os.ReadFile(summaryPath)
			require.NoError(t, err)
			require.Contains(t, string(summaryData), string(status))
		})
	}
}

func TestSession_Close_GeneratesSummary(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-summary"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify summary.md structure
	summaryPath := filepath.Join(sessionDir, "summary.md")
	data, err := os.ReadFile(summaryPath)
	require.NoError(t, err)

	content := string(data)
	require.Contains(t, content, "# Session Summary")
	require.Contains(t, content, "**Session ID:**")
	require.Contains(t, content, "**Status:**")
	require.Contains(t, content, "**Start Time:**")
	require.Contains(t, content, "**End Time:**")
	require.Contains(t, content, "**Duration:**")
}

// Tests for AttachToBrokers

func TestSession_AttachToBrokers(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-attach-brokers"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	require.NotNil(t, session)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create brokers (coordinator events flow through v2EventBus as ProcessEvent)
	processBroker := pubsub.NewBroker[events.ProcessEvent]()
	msgBroker := pubsub.NewBroker[message.Event]()
	mcpBroker := pubsub.NewBroker[events.MCPEvent]()

	// Attach to all brokers
	session.AttachToBrokers(ctx, processBroker, msgBroker, mcpBroker)

	// Give goroutines time to start
	time.Sleep(10 * time.Millisecond)

	// Verify subscribers are attached (broker subscriber count should be 1 for each)
	require.Equal(t, 1, processBroker.SubscriberCount())
	require.Equal(t, 1, msgBroker.SubscriberCount())
	require.Equal(t, 1, mcpBroker.SubscriberCount())

	// Cleanup
	cancel()
	processBroker.Close()
	msgBroker.Close()
	mcpBroker.Close()
	_ = session.Close(StatusCompleted)
}

func TestSession_AttachToBrokers_NilBrokers(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-nil-brokers"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Should not panic with nil brokers
	session.AttachToBrokers(ctx, nil, nil, nil)

	// Give time for any potential goroutines
	time.Sleep(10 * time.Millisecond)

	_ = session.Close(StatusCompleted)
}

func TestSession_CoordinatorSubscriber(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-coord-subscriber"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Coordinator events flow through v2EventBus as ProcessEvent with Role=RoleCoordinator
	v2EventBus := pubsub.NewBroker[any]()
	defer v2EventBus.Close()

	session.AttachV2EventBus(ctx, v2EventBus)

	// Give goroutine time to start
	time.Sleep(10 * time.Millisecond)

	// Publish a coordinator output event (equivalent to CoordinatorChat)
	v2EventBus.Publish(pubsub.UpdatedEvent, events.ProcessEvent{
		Type:    events.ProcessOutput,
		Role:    events.RoleCoordinator,
		Output:  "Starting orchestration session...",
		RawJSON: []byte(`{"type":"chat","content":"Starting orchestration session..."}`),
	})

	// Publish a token usage event
	v2EventBus.Publish(pubsub.UpdatedEvent, events.ProcessEvent{
		Type:      events.ProcessTokenUsage,
		ProcessID: "coordinator",
		Role:      events.RoleCoordinator,
		Metrics: &metrics.TokenMetrics{
			TokensUsed:   100,
			OutputTokens: 50,
			TotalCostUSD: 0.05,
		},
	})

	// Give time for events to be processed
	time.Sleep(50 * time.Millisecond)

	// Close session to flush buffers
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify coordinator messages.jsonl has the output event (structured JSONL)
	msgsPath := filepath.Join(sessionDir, "coordinator", "messages.jsonl")
	data, err := os.ReadFile(msgsPath)
	require.NoError(t, err)
	require.Contains(t, string(data), `"role":"coordinator"`)
	require.Contains(t, string(data), "Starting orchestration session...")

	// Verify raw.jsonl has the raw JSON
	rawPath := filepath.Join(sessionDir, "coordinator", "raw.jsonl")
	rawData, err := os.ReadFile(rawPath)
	require.NoError(t, err)
	require.Contains(t, string(rawData), `"type":"chat"`)

	// Verify metadata has token usage
	meta, err := Load(sessionDir)
	require.NoError(t, err)
	require.Equal(t, 50, meta.TokenUsage.TotalOutputTokens)
	require.InDelta(t, 0.05, meta.TokenUsage.TotalCostUSD, 0.001)
}

func TestSession_WorkerSubscriber(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-worker-subscriber"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	processBroker := pubsub.NewBroker[events.ProcessEvent]()
	defer processBroker.Close()

	session.AttachToBrokers(ctx, processBroker, nil, nil)

	// Give goroutine time to start
	time.Sleep(10 * time.Millisecond)

	// Publish worker spawned event
	processBroker.Publish(pubsub.UpdatedEvent, events.ProcessEvent{
		Type:      events.ProcessSpawned,
		Role:      events.RoleWorker,
		ProcessID: "worker-1",
	})

	// Publish worker output event
	processBroker.Publish(pubsub.UpdatedEvent, events.ProcessEvent{
		Type:      events.ProcessOutput,
		Role:      events.RoleWorker,
		ProcessID: "worker-1",
		Output:    "Starting implementation...",
		RawJSON:   []byte(`{"type":"output","content":"Starting implementation..."}`),
	})

	// Publish worker status change (retired)
	processBroker.Publish(pubsub.UpdatedEvent, events.ProcessEvent{
		Type:      events.ProcessStatusChange,
		Role:      events.RoleWorker,
		ProcessID: "worker-1",
		Status:    events.ProcessStatusRetired,
		Phase:     phasePtr(events.ProcessPhaseIdle),
	})

	// Give time for events to be processed
	time.Sleep(50 * time.Millisecond)

	// Close session to flush buffers
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify worker messages.jsonl has content (structured JSONL)
	msgsPath := filepath.Join(sessionDir, "workers", "worker-1", "messages.jsonl")
	data, err := os.ReadFile(msgsPath)
	require.NoError(t, err)
	require.Contains(t, string(data), "Worker spawned")
	require.Contains(t, string(data), "Starting implementation...")
	// Status changes no longer write to messages.jsonl (not user-visible chat)
	// But metadata is still updated

	// Verify raw.jsonl has content
	rawPath := filepath.Join(sessionDir, "workers", "worker-1", "raw.jsonl")
	rawData, err := os.ReadFile(rawPath)
	require.NoError(t, err)
	require.Contains(t, string(rawData), `"type":"output"`)

	// Verify metadata has worker info
	meta, err := Load(sessionDir)
	require.NoError(t, err)
	require.Len(t, meta.Workers, 1)
	require.Equal(t, "worker-1", meta.Workers[0].ID)
	require.False(t, meta.Workers[0].SpawnedAt.IsZero())
	require.False(t, meta.Workers[0].RetiredAt.IsZero())
	require.Equal(t, "idle", meta.Workers[0].FinalPhase)
}

func TestSession_MessageSubscriber(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-message-subscriber"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	msgBroker := pubsub.NewBroker[message.Event]()
	defer msgBroker.Close()

	session.AttachToBrokers(ctx, nil, msgBroker, nil)

	// Give goroutine time to start
	time.Sleep(10 * time.Millisecond)

	// Publish a message event
	timestamp := time.Date(2025, 1, 15, 10, 30, 45, 0, time.UTC)
	msgBroker.Publish(pubsub.UpdatedEvent, message.Event{
		Type: message.EventPosted,
		Entry: message.Entry{
			ID:        "msg-001",
			Timestamp: timestamp,
			From:      "COORDINATOR",
			To:        "ALL",
			Content:   "Starting work on epic",
			Type:      message.MessageInfo,
		},
	})

	// Give time for event to be processed
	time.Sleep(50 * time.Millisecond)

	// Close session to flush buffers
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify messages.jsonl has the message
	messagesPath := filepath.Join(sessionDir, "messages.jsonl")
	data, err := os.ReadFile(messagesPath)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 1)

	// Parse and verify message
	var parsed message.Entry
	err = json.Unmarshal([]byte(lines[0]), &parsed)
	require.NoError(t, err)
	require.Equal(t, "msg-001", parsed.ID)
	require.Equal(t, "COORDINATOR", parsed.From)
	require.Equal(t, "Starting work on epic", parsed.Content)
}

func TestSession_MCPSubscriber(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-mcp-subscriber"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mcpBroker := pubsub.NewBroker[events.MCPEvent]()
	defer mcpBroker.Close()

	session.AttachToBrokers(ctx, nil, nil, mcpBroker)

	// Give goroutine time to start
	time.Sleep(10 * time.Millisecond)

	// Publish MCP events
	timestamp := time.Date(2025, 1, 15, 10, 30, 45, 0, time.UTC)
	mcpBroker.Publish(pubsub.UpdatedEvent, events.MCPEvent{
		Timestamp:    timestamp,
		Type:         events.MCPToolCall,
		Method:       "tools/call",
		ToolName:     "spawn_worker",
		WorkerID:     "",
		RequestJSON:  []byte(`{"workerID":"worker-1"}`),
		ResponseJSON: nil,
		Duration:     50 * time.Millisecond,
	})

	mcpBroker.Publish(pubsub.UpdatedEvent, events.MCPEvent{
		Timestamp:    timestamp.Add(100 * time.Millisecond),
		Type:         events.MCPToolResult,
		Method:       "tools/call",
		ToolName:     "spawn_worker",
		WorkerID:     "",
		RequestJSON:  nil,
		ResponseJSON: []byte(`{"success":true,"workerID":"worker-1"}`),
		Duration:     50 * time.Millisecond,
	})

	// Give time for events to be processed
	time.Sleep(50 * time.Millisecond)

	// Close session to flush buffers
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify mcp_requests.jsonl has the events
	mcpPath := filepath.Join(sessionDir, "mcp_requests.jsonl")
	data, err := os.ReadFile(mcpPath)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 2)

	// Parse and verify first event
	var parsed1 events.MCPEvent
	err = json.Unmarshal([]byte(lines[0]), &parsed1)
	require.NoError(t, err)
	require.Equal(t, events.MCPToolCall, parsed1.Type)
	require.Equal(t, "spawn_worker", parsed1.ToolName)

	// Parse and verify second event
	var parsed2 events.MCPEvent
	err = json.Unmarshal([]byte(lines[1]), &parsed2)
	require.NoError(t, err)
	require.Equal(t, events.MCPToolResult, parsed2.Type)
}

func TestSession_ContextCancellation(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-context-cancel"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	ctx, cancel := context.WithCancel(context.Background())

	// Create brokers (coordinator events flow through v2EventBus)
	processBroker := pubsub.NewBroker[events.ProcessEvent]()
	msgBroker := pubsub.NewBroker[message.Event]()
	mcpBroker := pubsub.NewBroker[events.MCPEvent]()

	// Attach to all brokers
	session.AttachToBrokers(ctx, processBroker, msgBroker, mcpBroker)

	// Give goroutines time to start
	time.Sleep(10 * time.Millisecond)

	// Verify subscribers are attached
	require.Equal(t, 1, processBroker.SubscriberCount())
	require.Equal(t, 1, msgBroker.SubscriberCount())
	require.Equal(t, 1, mcpBroker.SubscriberCount())

	// Cancel the context
	cancel()

	// Give time for cleanup
	time.Sleep(50 * time.Millisecond)

	// Verify subscribers are cleaned up
	require.Equal(t, 0, processBroker.SubscriberCount())
	require.Equal(t, 0, msgBroker.SubscriberCount())
	require.Equal(t, 0, mcpBroker.SubscriberCount())

	// Cleanup
	processBroker.Close()
	msgBroker.Close()
	mcpBroker.Close()
	_ = session.Close(StatusCompleted)
}

func TestSession_HighThroughput(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping high-throughput test in short mode")
	}

	baseDir := t.TempDir()
	sessionID := "test-high-throughput"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create brokers with larger buffer for high throughput
	processBroker := pubsub.NewBrokerWithBuffer[events.ProcessEvent](256)
	defer processBroker.Close()

	session.AttachToBrokers(ctx, processBroker, nil, nil)

	// Give goroutine time to start
	time.Sleep(10 * time.Millisecond)

	// Publish 100 events with small delays to simulate normal load (not stress test)
	// This tests that under normal operation (< 100 events/sec) no events are dropped
	const eventCount = 100
	for i := 0; i < eventCount; i++ {
		processBroker.Publish(pubsub.UpdatedEvent, events.ProcessEvent{
			Type:      events.ProcessOutput,
			Role:      events.RoleWorker,
			ProcessID: "worker-1",
			Output:    fmt.Sprintf("Event %d", i),
		})
		// Small delay to allow subscriber to process (simulates realistic event rate)
		if i%10 == 0 {
			time.Sleep(time.Millisecond)
		}
	}

	// Give time for all events to be processed
	time.Sleep(200 * time.Millisecond)

	// Close session to flush buffers
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify worker messages.jsonl has content (structured JSONL format)
	msgsPath := filepath.Join(sessionDir, "workers", "worker-1", "messages.jsonl")
	data, err := os.ReadFile(msgsPath)
	require.NoError(t, err)

	// Under normal load (with small delays), all events should be captured
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Equal(t, eventCount, len(lines), "Expected all events to be captured under normal load")

	// Verify first message is valid JSONL
	var decoded chatrender.Message
	err = json.Unmarshal([]byte(lines[0]), &decoded)
	require.NoError(t, err)
	require.Equal(t, "assistant", decoded.Role)
	require.Equal(t, "Event 0", decoded.Content)
}

func TestSession_TokenUsageAggregation(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-token-aggregation"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use v2EventBus for both coordinator and worker events
	v2EventBus := pubsub.NewBroker[any]()
	defer v2EventBus.Close()

	session.AttachV2EventBus(ctx, v2EventBus)

	// Give goroutines time to start
	time.Sleep(10 * time.Millisecond)

	// Add workers to session first (required for per-worker token tracking)
	now := time.Now()
	session.addWorker("worker-1", now, "/project")
	session.addWorker("worker-2", now, "/project")

	// Publish multiple token usage events from coordinator and workers
	v2EventBus.Publish(pubsub.UpdatedEvent, events.ProcessEvent{
		Type:      events.ProcessTokenUsage,
		ProcessID: "coordinator",
		Role:      events.RoleCoordinator,
		Metrics: &metrics.TokenMetrics{
			TokensUsed:   100,
			OutputTokens: 50,
			TotalCostUSD: 0.01,
		},
	})

	v2EventBus.Publish(pubsub.UpdatedEvent, events.ProcessEvent{
		Type:      events.ProcessTokenUsage,
		Role:      events.RoleWorker,
		ProcessID: "worker-1",
		Metrics: &metrics.TokenMetrics{
			TokensUsed:   200,
			OutputTokens: 75,
			TotalCostUSD: 0.02,
		},
	})

	v2EventBus.Publish(pubsub.UpdatedEvent, events.ProcessEvent{
		Type:      events.ProcessTokenUsage,
		Role:      events.RoleWorker,
		ProcessID: "worker-2",
		Metrics: &metrics.TokenMetrics{
			TokensUsed:   300,
			OutputTokens: 100,
			TotalCostUSD: 0.03,
		},
	})

	// Give time for events to be processed
	time.Sleep(50 * time.Millisecond)

	// Close session to flush and update metadata
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify token usage:
	// - TotalOutputTokens: accumulated (output tokens are incremental per-turn)
	// - TotalCostUSD: accumulated (cost is incremental per-turn)
	meta, err := Load(sessionDir)
	require.NoError(t, err)
	require.Equal(t, 225, meta.TokenUsage.TotalOutputTokens) // 50 + 75 + 100
	require.InDelta(t, 0.06, meta.TokenUsage.TotalCostUSD, 0.001)

	// Verify per-process tracking
	require.Equal(t, 50, meta.CoordinatorTokenUsage.TotalOutputTokens)
	require.InDelta(t, 0.01, meta.CoordinatorTokenUsage.TotalCostUSD, 0.001)
}

func TestSession_WorkerMetadataUpdates(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-worker-metadata"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	processBroker := pubsub.NewBroker[events.ProcessEvent]()
	defer processBroker.Close()

	session.AttachToBrokers(ctx, processBroker, nil, nil)

	// Give goroutine time to start
	time.Sleep(10 * time.Millisecond)

	// Spawn multiple workers
	processBroker.Publish(pubsub.UpdatedEvent, events.ProcessEvent{
		Type:      events.ProcessSpawned,
		Role:      events.RoleWorker,
		ProcessID: "worker-1",
	})
	processBroker.Publish(pubsub.UpdatedEvent, events.ProcessEvent{
		Type:      events.ProcessSpawned,
		Role:      events.RoleWorker,
		ProcessID: "worker-2",
	})
	processBroker.Publish(pubsub.UpdatedEvent, events.ProcessEvent{
		Type:      events.ProcessSpawned,
		Role:      events.RoleWorker,
		ProcessID: "worker-3",
	})

	// Status changes
	processBroker.Publish(pubsub.UpdatedEvent, events.ProcessEvent{
		Type:      events.ProcessStatusChange,
		Role:      events.RoleWorker,
		ProcessID: "worker-1",
		Status:    events.ProcessStatusWorking,
		Phase:     phasePtr(events.ProcessPhaseImplementing),
	})
	processBroker.Publish(pubsub.UpdatedEvent, events.ProcessEvent{
		Type:      events.ProcessStatusChange,
		Role:      events.RoleWorker,
		ProcessID: "worker-2",
		Status:    events.ProcessStatusWorking,
		Phase:     phasePtr(events.ProcessPhaseReviewing),
	})

	// Retire one worker
	processBroker.Publish(pubsub.UpdatedEvent, events.ProcessEvent{
		Type:      events.ProcessStatusChange,
		Role:      events.RoleWorker,
		ProcessID: "worker-1",
		Status:    events.ProcessStatusRetired,
		Phase:     phasePtr(events.ProcessPhaseIdle),
	})

	// Give time for events to be processed
	time.Sleep(50 * time.Millisecond)

	// Close session
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify worker metadata
	meta, err := Load(sessionDir)
	require.NoError(t, err)
	require.Len(t, meta.Workers, 3)

	// Find worker-1 and verify retirement
	var worker1Found, worker2Found bool
	for _, w := range meta.Workers {
		if w.ID == "worker-1" {
			worker1Found = true
			require.False(t, w.RetiredAt.IsZero(), "worker-1 should have retirement time")
			require.Equal(t, "idle", w.FinalPhase)
		}
		if w.ID == "worker-2" {
			worker2Found = true
			require.True(t, w.RetiredAt.IsZero(), "worker-2 should not be retired")
			require.Equal(t, "reviewing", w.FinalPhase)
		}
	}
	require.True(t, worker1Found, "worker-1 should be in metadata")
	require.True(t, worker2Found, "worker-2 should be in metadata")
}

// Tests for late broker attachment (simulating real initialization flow)

func TestSession_LateCoordinatorAttach(t *testing.T) {
	// This test verifies that the v2EventBus can be attached separately
	// after the session is created, and handles both coordinator and worker events.
	// In the new architecture, coordinator events flow through v2EventBus as
	// ProcessEvent with Role=RoleCoordinator.

	baseDir := t.TempDir()
	sessionID := "test-late-coord-attach"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Step 1: First attach pool and message brokers
	processBroker := pubsub.NewBroker[events.ProcessEvent]()
	msgBroker := pubsub.NewBroker[message.Event]()
	defer processBroker.Close()
	defer msgBroker.Close()

	session.AttachToBrokers(ctx, processBroker, msgBroker, nil)

	// Give goroutines time to start
	time.Sleep(10 * time.Millisecond)

	// Verify process and message brokers are attached
	require.Equal(t, 1, processBroker.SubscriberCount())
	require.Equal(t, 1, msgBroker.SubscriberCount())

	// Publish some worker events before v2EventBus is attached
	processBroker.Publish(pubsub.UpdatedEvent, events.ProcessEvent{
		Type:      events.ProcessSpawned,
		Role:      events.RoleWorker,
		ProcessID: "worker-1",
	})

	// Step 2: Later, attach v2EventBus (handles both coordinator and worker events)
	v2EventBus := pubsub.NewBroker[any]()
	defer v2EventBus.Close()

	session.AttachV2EventBus(ctx, v2EventBus)

	// Give goroutine time to start
	time.Sleep(10 * time.Millisecond)

	// Verify v2EventBus is now attached
	require.Equal(t, 1, v2EventBus.SubscriberCount())

	// Publish coordinator events after attachment (via ProcessEvent with RoleCoordinator)
	v2EventBus.Publish(pubsub.UpdatedEvent, events.ProcessEvent{
		Type:   events.ProcessOutput,
		Role:   events.RoleCoordinator,
		Output: "First coordinator message",
	})

	// Give time for events to be processed
	time.Sleep(50 * time.Millisecond)

	// Close session to flush buffers
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify worker events were captured (from before v2EventBus attached)
	// ProcessSpawned events now write to messages.jsonl as system messages
	worker1Msgs := filepath.Join(sessionDir, "workers", "worker-1", "messages.jsonl")
	workerData, err := os.ReadFile(worker1Msgs)
	require.NoError(t, err)
	require.Contains(t, string(workerData), "Worker spawned")

	// Verify coordinator events were captured (from after attachment)
	// Now writes to messages.jsonl as structured JSONL
	coordMsgs := filepath.Join(sessionDir, "coordinator", "messages.jsonl")
	coordData, err := os.ReadFile(coordMsgs)
	require.NoError(t, err)
	require.Contains(t, string(coordData), `"role":"coordinator"`)
	require.Contains(t, string(coordData), "First coordinator message")
}

func TestSession_MCPBrokerAttach(t *testing.T) {
	// This test verifies that the MCP broker can be attached separately
	// after the MCP server is created during initialization.

	baseDir := t.TempDir()
	sessionID := "test-mcp-attach"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Step 1: First attach pool and message brokers (with nil for MCP)
	processBroker := pubsub.NewBroker[events.ProcessEvent]()
	msgBroker := pubsub.NewBroker[message.Event]()
	defer processBroker.Close()
	defer msgBroker.Close()

	session.AttachToBrokers(ctx, processBroker, msgBroker, nil)

	// Give goroutines time to start
	time.Sleep(10 * time.Millisecond)

	// Step 2: Later, attach MCP broker (simulates after MCP server creation)
	mcpBroker := pubsub.NewBroker[events.MCPEvent]()
	defer mcpBroker.Close()

	session.AttachMCPBroker(ctx, mcpBroker)

	// Give goroutine time to start
	time.Sleep(10 * time.Millisecond)

	// Verify MCP broker is attached
	require.Equal(t, 1, mcpBroker.SubscriberCount())

	// Publish MCP events after attachment
	timestamp := time.Date(2025, 1, 15, 10, 30, 45, 0, time.UTC)
	mcpBroker.Publish(pubsub.UpdatedEvent, events.MCPEvent{
		Timestamp:   timestamp,
		Type:        events.MCPToolCall,
		Method:      "tools/call",
		ToolName:    "spawn_worker",
		RequestJSON: []byte(`{"workerID":"worker-1"}`),
		Duration:    50 * time.Millisecond,
	})

	mcpBroker.Publish(pubsub.UpdatedEvent, events.MCPEvent{
		Timestamp:    timestamp.Add(100 * time.Millisecond),
		Type:         events.MCPToolResult,
		Method:       "tools/call",
		ToolName:     "spawn_worker",
		ResponseJSON: []byte(`{"success":true}`),
		Duration:     50 * time.Millisecond,
	})

	// Give time for events to be processed
	time.Sleep(50 * time.Millisecond)

	// Close session to flush buffers
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify MCP events were captured
	mcpPath := filepath.Join(sessionDir, "mcp_requests.jsonl")
	mcpData, err := os.ReadFile(mcpPath)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(mcpData)), "\n")
	require.Len(t, lines, 2)

	// Parse and verify events
	var parsed1 events.MCPEvent
	err = json.Unmarshal([]byte(lines[0]), &parsed1)
	require.NoError(t, err)
	require.Equal(t, events.MCPToolCall, parsed1.Type)
	require.Equal(t, "spawn_worker", parsed1.ToolName)

	var parsed2 events.MCPEvent
	err = json.Unmarshal([]byte(lines[1]), &parsed2)
	require.NoError(t, err)
	require.Equal(t, events.MCPToolResult, parsed2.Type)
}

func TestSession_AllFourBrokersFromAllBrokers(t *testing.T) {
	// Integration test: Events from all broker types captured in session files
	// This simulates the full initialization flow with staggered broker attachment
	// In v2 architecture, coordinator events flow through v2EventBus as ProcessEvent

	baseDir := t.TempDir()
	sessionID := "test-all-four-brokers"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create all brokers (coordinator events flow through v2EventBus)
	v2EventBus := pubsub.NewBroker[any]()
	processBroker := pubsub.NewBroker[events.ProcessEvent]()
	msgBroker := pubsub.NewBroker[message.Event]()
	mcpBroker := pubsub.NewBroker[events.MCPEvent]()
	defer v2EventBus.Close()
	defer processBroker.Close()
	defer msgBroker.Close()
	defer mcpBroker.Close()

	// Phase 1: Attach process and message brokers first (simulating createWorkspace)
	session.AttachToBrokers(ctx, processBroker, msgBroker, nil)
	time.Sleep(10 * time.Millisecond)

	// Phase 2: Attach MCP broker (simulating after MCP server creation)
	session.AttachMCPBroker(ctx, mcpBroker)
	time.Sleep(10 * time.Millisecond)

	// Phase 3: Attach v2EventBus (handles both coordinator and worker events)
	session.AttachV2EventBus(ctx, v2EventBus)
	time.Sleep(10 * time.Millisecond)

	// Verify all brokers are attached
	require.Equal(t, 1, v2EventBus.SubscriberCount(), "v2EventBus should have 1 subscriber")
	require.Equal(t, 1, processBroker.SubscriberCount(), "worker broker should have 1 subscriber")
	require.Equal(t, 1, msgBroker.SubscriberCount(), "message broker should have 1 subscriber")
	require.Equal(t, 1, mcpBroker.SubscriberCount(), "MCP broker should have 1 subscriber")

	// Publish events from all sources
	timestamp := time.Now()

	// Coordinator event (via v2EventBus as ProcessEvent)
	v2EventBus.Publish(pubsub.UpdatedEvent, events.ProcessEvent{
		Type:   events.ProcessOutput,
		Role:   events.RoleCoordinator,
		Output: "Orchestration started",
	})

	// Worker event
	processBroker.Publish(pubsub.UpdatedEvent, events.ProcessEvent{
		Type:      events.ProcessSpawned,
		Role:      events.RoleWorker,
		ProcessID: "worker-1",
	})

	// Message event
	msgBroker.Publish(pubsub.UpdatedEvent, message.Event{
		Type: message.EventPosted,
		Entry: message.Entry{
			ID:        "msg-001",
			Timestamp: timestamp,
			From:      "COORDINATOR",
			To:        "ALL",
			Content:   "Starting work",
			Type:      message.MessageInfo,
		},
	})

	// MCP event
	mcpBroker.Publish(pubsub.UpdatedEvent, events.MCPEvent{
		Timestamp:   timestamp,
		Type:        events.MCPToolCall,
		Method:      "tools/call",
		ToolName:    "test_tool",
		RequestJSON: []byte(`{"test":true}`),
		Duration:    10 * time.Millisecond,
	})

	// Give time for all events to be processed
	time.Sleep(100 * time.Millisecond)

	// Close session to flush buffers
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify all 4 log files have content

	// 1. Coordinator chat messages (now uses messages.jsonl with structured JSONL)
	coordMsgs := filepath.Join(sessionDir, "coordinator", "messages.jsonl")
	coordData, err := os.ReadFile(coordMsgs)
	require.NoError(t, err)
	require.Contains(t, string(coordData), "Orchestration started")
	require.Contains(t, string(coordData), `"role":"coordinator"`)

	// 2. Worker chat messages (now uses messages.jsonl with structured JSONL)
	workerMsgs := filepath.Join(sessionDir, "workers", "worker-1", "messages.jsonl")
	workerData, err := os.ReadFile(workerMsgs)
	require.NoError(t, err)
	require.Contains(t, string(workerData), "Worker spawned")

	// 3. Inter-agent messages log (unchanged)
	messagesPath := filepath.Join(sessionDir, "messages.jsonl")
	messagesData, err := os.ReadFile(messagesPath)
	require.NoError(t, err)
	require.Contains(t, string(messagesData), "Starting work")

	// 4. MCP log (unchanged)
	mcpPath := filepath.Join(sessionDir, "mcp_requests.jsonl")
	mcpData, err := os.ReadFile(mcpPath)
	require.NoError(t, err)
	require.Contains(t, string(mcpData), "test_tool")
}

// Tests for WriteWorkerAccountabilitySummary

func TestWriteWorkerAccountabilitySummary_Success(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-accountability-success"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	require.NotNil(t, session)

	// Write an accountability summary (taskID is now in YAML frontmatter)
	content := []byte("---\ntask_id: perles-abc.1\nworker_id: worker-1\n---\n\n# Worker Accountability Summary\n\n**Worker:** worker-1\n**Task:** perles-abc.1\n\n## Summary\n\nImplemented user validation with regex patterns.\n")
	filePath, err := session.WriteWorkerAccountabilitySummary("worker-1", content)
	require.NoError(t, err)
	require.NotEmpty(t, filePath)

	// Verify file path
	expectedPath := filepath.Join(sessionDir, "workers", "worker-1", "accountability_summary.md")
	require.Equal(t, expectedPath, filePath)

	// Verify file exists and has correct content
	data, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.Equal(t, content, data)

	err = session.Close(StatusCompleted)
	require.NoError(t, err)
}

func TestWriteWorkerAccountabilitySummary_CreatesWorkerDirectory(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-accountability-creates-dir"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)

	// Worker directory shouldn't exist yet
	workerPath := filepath.Join(sessionDir, "workers", "worker-new")
	_, err = os.Stat(workerPath)
	require.True(t, os.IsNotExist(err))

	// Write accountability summary - should create directory
	content := []byte("---\ntask_id: task-123\n---\n\n# Accountability Summary")
	filePath, err := session.WriteWorkerAccountabilitySummary("worker-new", content)
	require.NoError(t, err)
	require.NotEmpty(t, filePath)

	// Worker directory should now exist
	info, err := os.Stat(workerPath)
	require.NoError(t, err)
	require.True(t, info.IsDir())

	// accountability_summary.md should exist
	summaryPath := filepath.Join(workerPath, "accountability_summary.md")
	_, err = os.Stat(summaryPath)
	require.NoError(t, err)

	err = session.Close(StatusCompleted)
	require.NoError(t, err)
}

func TestWriteWorkerAccountabilitySummary_SessionClosed(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-accountability-closed"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)

	// Close the session first
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Writing after close should fail
	content := []byte("---\ntask_id: task-123\n---\n\n# Accountability Summary")
	_, err = session.WriteWorkerAccountabilitySummary("worker-1", content)
	require.Error(t, err)
	require.Equal(t, os.ErrClosed, err)
}

func TestWriteWorkerAccountabilitySummary_OverwritesExisting(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-accountability-overwrite"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)

	// Write first accountability summary
	content1 := []byte("---\ntask_id: task-1\n---\n\n# First Summary\n\nOriginal content")
	filePath1, err := session.WriteWorkerAccountabilitySummary("worker-1", content1)
	require.NoError(t, err)

	// Verify first content
	data1, err := os.ReadFile(filePath1)
	require.NoError(t, err)
	require.Equal(t, content1, data1)

	// Write second accountability summary (different task) - should overwrite
	content2 := []byte("---\ntask_id: task-2\n---\n\n# Second Summary\n\nUpdated content for different task")
	filePath2, err := session.WriteWorkerAccountabilitySummary("worker-1", content2)
	require.NoError(t, err)

	// Paths should be the same (same worker, single accountability_summary.md file)
	require.Equal(t, filePath1, filePath2)

	// Verify content was overwritten
	data2, err := os.ReadFile(filePath2)
	require.NoError(t, err)
	require.Equal(t, content2, data2)

	err = session.Close(StatusCompleted)
	require.NoError(t, err)
}

func TestWriteWorkerAccountabilitySummary_FilePermissions(t *testing.T) {
	// Skip on Windows where file permissions work differently
	if runtime.GOOS == "windows" {
		t.Skip("skipping permission test on Windows")
	}

	baseDir := t.TempDir()
	sessionID := "test-accountability-permissions"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)

	// Write an accountability summary
	content := []byte("---\ntask_id: task-123\n---\n\n# Accountability Summary\n\nTest content for permission check")
	filePath, err := session.WriteWorkerAccountabilitySummary("worker-1", content)
	require.NoError(t, err)

	// Verify file permissions are 0600 (owner read/write only)
	info, err := os.Stat(filePath)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())

	err = session.Close(StatusCompleted)
	require.NoError(t, err)
}

func TestWriteWorkerAccountabilitySummary_EmptyContent(t *testing.T) {
	// Edge case: Empty content should write an empty file (valid, not an error)
	baseDir := t.TempDir()
	sessionID := "test-accountability-empty"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)

	// Write empty content
	content := []byte{}
	filePath, err := session.WriteWorkerAccountabilitySummary("worker-1", content)
	require.NoError(t, err)
	require.NotEmpty(t, filePath)

	// Verify file exists and is empty
	data, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.Empty(t, data)

	err = session.Close(StatusCompleted)
	require.NoError(t, err)
}

func TestWriteWorkerAccountabilitySummary_MultipleWorkers(t *testing.T) {
	// Verify each worker gets their own accountability_summary.md file with no cross-contamination
	baseDir := t.TempDir()
	sessionID := "test-accountability-multiple"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)

	// Write accountability summaries for multiple workers
	content1 := []byte("---\ntask_id: task-1\n---\n\n# Summary Worker 1\n\nWorker-1 specific content")
	content2 := []byte("---\ntask_id: task-2\n---\n\n# Summary Worker 2\n\nWorker-2 specific content")
	content3 := []byte("---\ntask_id: task-3\n---\n\n# Summary Worker 3\n\nWorker-3 specific content")

	path1, err := session.WriteWorkerAccountabilitySummary("worker-1", content1)
	require.NoError(t, err)

	path2, err := session.WriteWorkerAccountabilitySummary("worker-2", content2)
	require.NoError(t, err)

	path3, err := session.WriteWorkerAccountabilitySummary("worker-3", content3)
	require.NoError(t, err)

	// Verify paths are different
	require.NotEqual(t, path1, path2)
	require.NotEqual(t, path2, path3)
	require.NotEqual(t, path1, path3)

	// Verify each file has correct content
	data1, err := os.ReadFile(path1)
	require.NoError(t, err)
	require.Equal(t, content1, data1)

	data2, err := os.ReadFile(path2)
	require.NoError(t, err)
	require.Equal(t, content2, data2)

	data3, err := os.ReadFile(path3)
	require.NoError(t, err)
	require.Equal(t, content3, data3)

	err = session.Close(StatusCompleted)
	require.NoError(t, err)
}

// Tests for updateSessionIndex

func TestUpdateSessionIndex_NewIndex(t *testing.T) {
	// When sessions.json doesn't exist, updateSessionIndex should create it
	baseDir := t.TempDir()
	sessionsDir := filepath.Join(baseDir, "sessions")
	err := os.MkdirAll(sessionsDir, 0750)
	require.NoError(t, err)

	sessionID := "test-new-index"
	sessionDir := filepath.Join(sessionsDir, sessionID)

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)

	// Close the session - this calls updateSessionIndex
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify sessions.json was created
	indexPath := filepath.Join(sessionsDir, "sessions.json")
	_, err = os.Stat(indexPath)
	require.NoError(t, err, "sessions.json should be created")

	// Load and verify the index
	index, err := LoadSessionIndex(indexPath)
	require.NoError(t, err)
	require.Len(t, index.Sessions, 1)
	require.Equal(t, sessionID, index.Sessions[0].ID)
	require.Equal(t, StatusCompleted, index.Sessions[0].Status)
	require.False(t, index.Sessions[0].StartTime.IsZero())
	require.False(t, index.Sessions[0].EndTime.IsZero())
}

func TestUpdateSessionIndex_AppendToExisting(t *testing.T) {
	// When sessions.json already has entries, updateSessionIndex should append
	baseDir := t.TempDir()
	sessionsDir := filepath.Join(baseDir, "sessions")
	err := os.MkdirAll(sessionsDir, 0750)
	require.NoError(t, err)

	// Create first session
	sessionID1 := "session-first"
	sessionDir1 := filepath.Join(sessionsDir, sessionID1)
	session1, err := New(sessionID1, sessionDir1)
	require.NoError(t, err)
	err = session1.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify first session is in the index
	indexPath := filepath.Join(sessionsDir, "sessions.json")
	index, err := LoadSessionIndex(indexPath)
	require.NoError(t, err)
	require.Len(t, index.Sessions, 1)

	// Create second session
	sessionID2 := "session-second"
	sessionDir2 := filepath.Join(sessionsDir, sessionID2)
	session2, err := New(sessionID2, sessionDir2)
	require.NoError(t, err)
	err = session2.Close(StatusFailed)
	require.NoError(t, err)

	// Verify both sessions are in the index
	index, err = LoadSessionIndex(indexPath)
	require.NoError(t, err)
	require.Len(t, index.Sessions, 2)
	require.Equal(t, sessionID1, index.Sessions[0].ID)
	require.Equal(t, StatusCompleted, index.Sessions[0].Status)
	require.Equal(t, sessionID2, index.Sessions[1].ID)
	require.Equal(t, StatusFailed, index.Sessions[1].Status)
}

func TestClose_UpdatesSessionIndex(t *testing.T) {
	// Verify Close() creates/updates the sessions.json index with correct data
	baseDir := t.TempDir()
	sessionsDir := filepath.Join(baseDir, "sessions")
	err := os.MkdirAll(sessionsDir, 0750)
	require.NoError(t, err)

	sessionID := "test-close-index"
	sessionDir := filepath.Join(sessionsDir, sessionID)

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)

	// Write an accountability summary so the path gets included
	content := []byte("---\ntask_id: task-123\n---\n\n# Test Summary")
	_, err = session.WriteWorkerAccountabilitySummary("worker-1", content)
	require.NoError(t, err)

	// Also create the session-level accountability summary
	accountabilityPath := filepath.Join(sessionDir, "accountability_summary.md")
	err = os.WriteFile(accountabilityPath, []byte("# Session Summary"), 0600)
	require.NoError(t, err)

	// Close the session
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify sessions.json was created
	indexPath := filepath.Join(sessionsDir, "sessions.json")
	index, err := LoadSessionIndex(indexPath)
	require.NoError(t, err)
	require.Len(t, index.Sessions, 1)

	entry := index.Sessions[0]
	require.Equal(t, sessionID, entry.ID)
	require.Equal(t, StatusCompleted, entry.Status)
	require.False(t, entry.StartTime.IsZero())
	require.False(t, entry.EndTime.IsZero())
	// Accountability summary path should be set since we created it
	require.Equal(t, accountabilityPath, entry.AccountabilitySummaryPath)
	// WorkerCount should reflect the workers from session metadata
	// (workers need to be added via events, which we haven't simulated)
	require.Equal(t, 0, entry.WorkerCount)
}

// TestNew_WithoutOptions verifies backward compatibility - New() without options works as before.
func TestNew_WithoutOptions(t *testing.T) {
	baseDir := t.TempDir()
	sessionsDir := filepath.Join(baseDir, "sessions")
	sessionID := "test-session-no-opts"
	sessionDir := filepath.Join(sessionsDir, sessionID)

	// Create session without any options (backward compatible)
	sess, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	defer sess.Close(StatusCompleted)

	// Verify basic session creation works
	require.Equal(t, sessionID, sess.ID)
	require.Equal(t, sessionDir, sess.Dir)
	require.Equal(t, StatusRunning, sess.Status)

	// Verify metadata was saved without application context fields
	meta, err := Load(sessionDir)
	require.NoError(t, err)
	require.Equal(t, sessionID, meta.SessionID)
	require.Empty(t, meta.ApplicationName, "ApplicationName should be empty without option")
	require.Empty(t, meta.WorkDir, "WorkDir should be empty without option")
	require.Empty(t, meta.DatePartition, "DatePartition should be empty without option")
}

// TestNew_WithWorkDir verifies WithWorkDir sets the metadata field.
func TestNew_WithWorkDir(t *testing.T) {
	baseDir := t.TempDir()
	sessionsDir := filepath.Join(baseDir, "sessions")
	sessionID := "test-session-workdir"
	sessionDir := filepath.Join(sessionsDir, sessionID)
	workDir := "/home/user/projects/my-app"

	sess, err := New(sessionID, sessionDir, WithWorkDir(workDir))
	require.NoError(t, err)
	defer sess.Close(StatusCompleted)

	// Verify metadata includes the work dir
	meta, err := Load(sessionDir)
	require.NoError(t, err)
	require.Equal(t, workDir, meta.WorkDir)
}

// TestNew_WithApplicationName verifies WithApplicationName sets the metadata field.
func TestNew_WithApplicationName(t *testing.T) {
	baseDir := t.TempDir()
	sessionsDir := filepath.Join(baseDir, "sessions")
	sessionID := "test-session-appname"
	sessionDir := filepath.Join(sessionsDir, sessionID)
	appName := "perles"

	sess, err := New(sessionID, sessionDir, WithApplicationName(appName))
	require.NoError(t, err)
	defer sess.Close(StatusCompleted)

	// Verify metadata includes the application name
	meta, err := Load(sessionDir)
	require.NoError(t, err)
	require.Equal(t, appName, meta.ApplicationName)
}

// TestNew_WithDatePartition verifies WithDatePartition sets the metadata field.
func TestNew_WithDatePartition(t *testing.T) {
	baseDir := t.TempDir()
	sessionsDir := filepath.Join(baseDir, "sessions")
	sessionID := "test-session-datepart"
	sessionDir := filepath.Join(sessionsDir, sessionID)
	datePartition := "2026-01-11"

	sess, err := New(sessionID, sessionDir, WithDatePartition(datePartition))
	require.NoError(t, err)
	defer sess.Close(StatusCompleted)

	// Verify metadata includes the date partition
	meta, err := Load(sessionDir)
	require.NoError(t, err)
	require.Equal(t, datePartition, meta.DatePartition)
}

// TestNew_WithMultipleOptions verifies all options can be applied together.
func TestNew_WithMultipleOptions(t *testing.T) {
	baseDir := t.TempDir()
	sessionsDir := filepath.Join(baseDir, "sessions")
	sessionID := "test-session-multi"
	sessionDir := filepath.Join(sessionsDir, sessionID)

	appName := "my-application"
	workDir := "/home/user/projects/my-application"
	datePartition := "2026-01-11"

	sess, err := New(sessionID, sessionDir,
		WithApplicationName(appName),
		WithWorkDir(workDir),
		WithDatePartition(datePartition),
	)
	require.NoError(t, err)
	defer sess.Close(StatusCompleted)

	// Verify all options are applied
	meta, err := Load(sessionDir)
	require.NoError(t, err)
	require.Equal(t, appName, meta.ApplicationName)
	require.Equal(t, workDir, meta.WorkDir)
	require.Equal(t, datePartition, meta.DatePartition)
}

// TestNew_MetadataIncludesNewFieldsWhenSet verifies metadata.json includes new fields when set.
func TestNew_MetadataIncludesNewFieldsWhenSet(t *testing.T) {
	baseDir := t.TempDir()
	sessionsDir := filepath.Join(baseDir, "sessions")
	sessionID := "test-session-json-fields"
	sessionDir := filepath.Join(sessionsDir, sessionID)

	appName := "test-app"
	workDir := "/test/workdir"
	datePartition := "2026-01-11"

	sess, err := New(sessionID, sessionDir,
		WithApplicationName(appName),
		WithWorkDir(workDir),
		WithDatePartition(datePartition),
	)
	require.NoError(t, err)
	defer sess.Close(StatusCompleted)

	// Read raw JSON to verify fields are in the file
	metadataPath := filepath.Join(sessionDir, metadataFilename)
	data, err := os.ReadFile(metadataPath)
	require.NoError(t, err)

	// Parse as generic map to check fields
	var raw map[string]interface{}
	err = json.Unmarshal(data, &raw)
	require.NoError(t, err)

	require.Equal(t, appName, raw["application_name"])
	require.Equal(t, workDir, raw["work_dir"])
	require.Equal(t, datePartition, raw["date_partition"])
}

// TestClose_PreservesApplicationContextOnReload verifies Close() preserves application context
// when it reloads metadata.
func TestClose_PreservesApplicationContextOnReload(t *testing.T) {
	baseDir := t.TempDir()
	sessionsDir := filepath.Join(baseDir, "sessions")
	sessionID := "test-session-close-preserve"
	sessionDir := filepath.Join(sessionsDir, sessionID)

	appName := "preserved-app"
	workDir := "/preserved/workdir"
	datePartition := "2026-01-11"

	sess, err := New(sessionID, sessionDir,
		WithApplicationName(appName),
		WithWorkDir(workDir),
		WithDatePartition(datePartition),
	)
	require.NoError(t, err)

	// Close the session (this reloads and re-saves metadata)
	err = sess.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify application context fields are preserved after close
	meta, err := Load(sessionDir)
	require.NoError(t, err)
	require.Equal(t, appName, meta.ApplicationName)
	require.Equal(t, workDir, meta.WorkDir)
	require.Equal(t, datePartition, meta.DatePartition)
	require.Equal(t, StatusCompleted, meta.Status)
}

// Tests for dual index updates (application + global)

func TestClose_UpdatesBothIndexes_WithPathBuilder(t *testing.T) {
	// Setup centralized storage structure
	baseDir := t.TempDir()
	appName := "dual-index-app"
	datePartition := "2026-01-11"
	sessionID := "dual-index-session"

	// Create path builder for centralized storage
	pathBuilder := NewSessionPathBuilder(baseDir, appName)
	sessionDir := pathBuilder.SessionDir(sessionID, time.Now())

	// Create session with path builder
	sess, err := New(sessionID, sessionDir,
		WithApplicationName(appName),
		WithWorkDir("/original/workdir"),
		WithDatePartition(datePartition),
		WithPathBuilder(pathBuilder),
	)
	require.NoError(t, err)

	// Close session
	err = sess.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify application index was created
	appIndexPath := pathBuilder.ApplicationIndexPath()
	appIndex, err := LoadApplicationIndex(appIndexPath)
	require.NoError(t, err)
	require.Equal(t, appName, appIndex.ApplicationName)
	require.Len(t, appIndex.Sessions, 1)
	require.Equal(t, sessionID, appIndex.Sessions[0].ID)
	require.Equal(t, appName, appIndex.Sessions[0].ApplicationName)
	require.Equal(t, "/original/workdir", appIndex.Sessions[0].WorkDir)
	require.Equal(t, datePartition, appIndex.Sessions[0].DatePartition)

	// Verify global index was created
	globalIndexPath := pathBuilder.IndexPath()
	globalIndex, err := LoadSessionIndex(globalIndexPath)
	require.NoError(t, err)
	require.Len(t, globalIndex.Sessions, 1)
	require.Equal(t, sessionID, globalIndex.Sessions[0].ID)
	require.Equal(t, appName, globalIndex.Sessions[0].ApplicationName)
}

func TestClose_UpdatesLegacyIndex_WithoutPathBuilder(t *testing.T) {
	// Setup legacy directory structure (no path builder)
	baseDir := t.TempDir()
	sessionsDir := filepath.Join(baseDir, "sessions")
	sessionID := "legacy-index-session"
	sessionDir := filepath.Join(sessionsDir, sessionID)

	// Create session without path builder (legacy mode)
	sess, err := New(sessionID, sessionDir,
		WithApplicationName("legacy-app"),
		WithWorkDir("/legacy/workdir"),
		WithDatePartition("2026-01-11"),
	)
	require.NoError(t, err)

	// Close session
	err = sess.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify legacy index was created at parent directory
	legacyIndexPath := filepath.Join(sessionsDir, "sessions.json")
	legacyIndex, err := LoadSessionIndex(legacyIndexPath)
	require.NoError(t, err)
	require.Len(t, legacyIndex.Sessions, 1)
	require.Equal(t, sessionID, legacyIndex.Sessions[0].ID)
	// Verify metadata fields are still included in legacy mode
	require.Equal(t, "legacy-app", legacyIndex.Sessions[0].ApplicationName)
	require.Equal(t, "/legacy/workdir", legacyIndex.Sessions[0].WorkDir)
	require.Equal(t, "2026-01-11", legacyIndex.Sessions[0].DatePartition)
}

func TestClose_AppendsToExistingIndexes(t *testing.T) {
	// Setup centralized storage
	baseDir := t.TempDir()
	appName := "append-test-app"

	// Create path builder
	pathBuilder := NewSessionPathBuilder(baseDir, appName)

	// Pre-create indexes with existing sessions
	existingEntry := SessionIndexEntry{
		ID:              "existing-session",
		StartTime:       time.Now().Add(-time.Hour),
		Status:          StatusCompleted,
		ApplicationName: appName,
	}

	// Save existing application index
	appIndexPath := pathBuilder.ApplicationIndexPath()
	appIndex := &ApplicationSessionIndex{
		Version:         SessionIndexVersion,
		ApplicationName: appName,
		Sessions:        []SessionIndexEntry{existingEntry},
	}
	err := SaveApplicationIndex(appIndexPath, appIndex)
	require.NoError(t, err)

	// Save existing global index
	globalIndexPath := pathBuilder.IndexPath()
	globalIndex := &SessionIndex{
		Version:  SessionIndexVersion,
		Sessions: []SessionIndexEntry{existingEntry},
	}
	err = SaveSessionIndex(globalIndexPath, globalIndex)
	require.NoError(t, err)

	// Create and close a new session
	sessionID := "new-session"
	sessionDir := pathBuilder.SessionDir(sessionID, time.Now())

	sess, err := New(sessionID, sessionDir,
		WithApplicationName(appName),
		WithWorkDir("/new/workdir"),
		WithDatePartition("2026-01-11"),
		WithPathBuilder(pathBuilder),
	)
	require.NoError(t, err)

	err = sess.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify application index now has two entries
	loadedAppIndex, err := LoadApplicationIndex(appIndexPath)
	require.NoError(t, err)
	require.Len(t, loadedAppIndex.Sessions, 2)
	require.Equal(t, "existing-session", loadedAppIndex.Sessions[0].ID)
	require.Equal(t, "new-session", loadedAppIndex.Sessions[1].ID)

	// Verify global index now has two entries
	loadedGlobalIndex, err := LoadSessionIndex(globalIndexPath)
	require.NoError(t, err)
	require.Len(t, loadedGlobalIndex.Sessions, 2)
	require.Equal(t, "existing-session", loadedGlobalIndex.Sessions[0].ID)
	require.Equal(t, "new-session", loadedGlobalIndex.Sessions[1].ID)
}

func TestClose_ResumedSession_UpdatesInPlace(t *testing.T) {
	// Setup centralized storage
	baseDir := t.TempDir()
	appName := "resume-test-app"
	sessionID := "resumed-session"
	pathBuilder := NewSessionPathBuilder(baseDir, appName)

	// Create and close a session (first run)
	sessionDir := pathBuilder.SessionDir(sessionID, time.Now())
	sess, err := New(sessionID, sessionDir,
		WithApplicationName(appName),
		WithWorkDir("/project/path"),
		WithDatePartition("2026-01-17"),
		WithPathBuilder(pathBuilder),
	)
	require.NoError(t, err)

	err = sess.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify initial state - both indexes have exactly one entry
	appIndexPath := pathBuilder.ApplicationIndexPath()
	globalIndexPath := pathBuilder.IndexPath()

	loadedAppIndex, err := LoadApplicationIndex(appIndexPath)
	require.NoError(t, err)
	require.Len(t, loadedAppIndex.Sessions, 1)
	require.Equal(t, sessionID, loadedAppIndex.Sessions[0].ID)
	require.Equal(t, StatusCompleted, loadedAppIndex.Sessions[0].Status)

	loadedGlobalIndex, err := LoadSessionIndex(globalIndexPath)
	require.NoError(t, err)
	require.Len(t, loadedGlobalIndex.Sessions, 1)

	// "Resume" the session (simulate by creating same ID again)
	sess2, err := New(sessionID, sessionDir,
		WithApplicationName(appName),
		WithWorkDir("/project/path"),
		WithDatePartition("2026-01-17"),
		WithPathBuilder(pathBuilder),
	)
	require.NoError(t, err)

	// Close with different status to prove it updated
	err = sess2.Close(StatusFailed)
	require.NoError(t, err)

	// Verify indexes still have exactly one entry (updated in place, not appended)
	loadedAppIndex, err = LoadApplicationIndex(appIndexPath)
	require.NoError(t, err)
	require.Len(t, loadedAppIndex.Sessions, 1, "Should update in place, not append duplicate")
	require.Equal(t, sessionID, loadedAppIndex.Sessions[0].ID)
	require.Equal(t, StatusFailed, loadedAppIndex.Sessions[0].Status, "Status should be updated")

	loadedGlobalIndex, err = LoadSessionIndex(globalIndexPath)
	require.NoError(t, err)
	require.Len(t, loadedGlobalIndex.Sessions, 1, "Should update in place, not append duplicate")
	require.Equal(t, StatusFailed, loadedGlobalIndex.Sessions[0].Status, "Status should be updated")
}

func TestClose_IndexEntryContainsAllMetadataFields(t *testing.T) {
	baseDir := t.TempDir()
	appName := "metadata-fields-app"
	workDir := "/original/project/path"
	datePartition := "2026-01-11"
	sessionID := "metadata-fields-session"

	pathBuilder := NewSessionPathBuilder(baseDir, appName)
	sessionDir := pathBuilder.SessionDir(sessionID, time.Now())

	sess, err := New(sessionID, sessionDir,
		WithApplicationName(appName),
		WithWorkDir(workDir),
		WithDatePartition(datePartition),
		WithPathBuilder(pathBuilder),
	)
	require.NoError(t, err)

	// Add a worker to verify worker count
	sess.addWorker("worker-1", time.Now(), workDir)

	err = sess.Close(StatusFailed)
	require.NoError(t, err)

	// Verify entry in application index has all fields
	appIndex, err := LoadApplicationIndex(pathBuilder.ApplicationIndexPath())
	require.NoError(t, err)
	require.Len(t, appIndex.Sessions, 1)

	entry := appIndex.Sessions[0]
	require.Equal(t, sessionID, entry.ID)
	require.Equal(t, StatusFailed, entry.Status)
	require.Equal(t, appName, entry.ApplicationName)
	require.Equal(t, workDir, entry.WorkDir)
	require.Equal(t, datePartition, entry.DatePartition)
	require.Equal(t, sessionDir, entry.SessionDir)
	require.Equal(t, 1, entry.WorkerCount)
	require.False(t, entry.StartTime.IsZero())
	require.False(t, entry.EndTime.IsZero())
}

func TestClose_PreservesObserverSessionRef(t *testing.T) {
	sessionDir := t.TempDir()
	sessionID := "observer-ref-test"

	sess, err := New(sessionID, sessionDir)
	require.NoError(t, err)

	// Set observer session ref (simulating observer's first successful turn)
	expectedObserverRef := "observer-session-abc123"
	err = sess.SetObserverSessionRef(expectedObserverRef)
	require.NoError(t, err)

	// Close the session (simulating TUI exit)
	err = sess.Close(StatusCompleted)
	require.NoError(t, err)

	// Reload metadata and verify observer ref is preserved
	meta, err := Load(sessionDir)
	require.NoError(t, err)

	require.NotNil(t, meta.Observer, "Observer metadata should be present after close")
	require.Equal(t, expectedObserverRef, meta.Observer.HeadlessSessionRef,
		"Observer HeadlessSessionRef must be preserved on close for --resume to work")
}

func TestClose_PreservesCoordinatorSessionRef(t *testing.T) {
	sessionDir := t.TempDir()
	sessionID := "coordinator-ref-test"

	sess, err := New(sessionID, sessionDir)
	require.NoError(t, err)

	// Set coordinator session ref
	expectedCoordRef := "coordinator-session-xyz789"
	err = sess.SetCoordinatorSessionRef(expectedCoordRef)
	require.NoError(t, err)

	// Close the session
	err = sess.Close(StatusCompleted)
	require.NoError(t, err)

	// Reload metadata and verify coordinator ref is preserved
	meta, err := Load(sessionDir)
	require.NoError(t, err)

	require.Equal(t, expectedCoordRef, meta.CoordinatorSessionRef,
		"CoordinatorSessionRef must be preserved on close for --resume to work")
}

func TestWithPathBuilder_Option(t *testing.T) {
	baseDir := t.TempDir()
	appName := "option-test-app"

	pathBuilder := NewSessionPathBuilder(baseDir, appName)
	sessionDir := filepath.Join(baseDir, "test-session")

	sess, err := New("test-session", sessionDir,
		WithPathBuilder(pathBuilder),
	)
	require.NoError(t, err)

	// Verify path builder was set (indirectly via behavior)
	require.NotNil(t, sess)

	// Clean up
	_ = sess.Close(StatusCompleted)
}

// Tests for handleCoordinatorProcessEvent

func TestSession_HandleCoordinatorProcessEvent_Output(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-coord-event-output"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	// Simulate a coordinator output event
	event := events.ProcessEvent{
		Type:      events.ProcessOutput,
		ProcessID: "coordinator",
		Role:      events.RoleCoordinator,
		Output:    "Hello, I'm the coordinator starting work.",
	}
	session.handleCoordinatorProcessEvent(event)

	// Close to flush buffers
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify message was written to coordinator/messages.jsonl
	msgsPath := filepath.Join(sessionDir, "coordinator", "messages.jsonl")
	data, err := os.ReadFile(msgsPath)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 1)

	var decoded chatrender.Message
	err = json.Unmarshal([]byte(lines[0]), &decoded)
	require.NoError(t, err)
	require.Equal(t, "coordinator", decoded.Role)
	require.Equal(t, "Hello, I'm the coordinator starting work.", decoded.Content)
	require.False(t, decoded.IsToolCall)
	require.NotNil(t, decoded.Timestamp)
}

func TestSession_HandleCoordinatorProcessEvent_Incoming(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-coord-event-incoming"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	// Simulate a user input event to coordinator
	event := events.ProcessEvent{
		Type:      events.ProcessIncoming,
		ProcessID: "coordinator",
		Role:      events.RoleCoordinator,
		Message:   "Please start work on the epic.",
		Sender:    "user",
	}
	session.handleCoordinatorProcessEvent(event)

	// Close to flush buffers
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify message was written with role="user"
	msgsPath := filepath.Join(sessionDir, "coordinator", "messages.jsonl")
	data, err := os.ReadFile(msgsPath)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 1)

	var decoded chatrender.Message
	err = json.Unmarshal([]byte(lines[0]), &decoded)
	require.NoError(t, err)
	require.Equal(t, "user", decoded.Role)
	require.Equal(t, "Please start work on the epic.", decoded.Content)
}

func TestSession_HandleCoordinatorProcessEvent_Error(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-coord-event-error"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	// Simulate an error event
	event := events.ProcessEvent{
		Type:      events.ProcessError,
		ProcessID: "coordinator",
		Role:      events.RoleCoordinator,
		Error:     fmt.Errorf("connection timeout"),
	}
	session.handleCoordinatorProcessEvent(event)

	// Close to flush buffers
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify error message was written with role="system"
	msgsPath := filepath.Join(sessionDir, "coordinator", "messages.jsonl")
	data, err := os.ReadFile(msgsPath)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 1)

	var decoded chatrender.Message
	err = json.Unmarshal([]byte(lines[0]), &decoded)
	require.NoError(t, err)
	require.Equal(t, "system", decoded.Role)
	require.Equal(t, "Error: connection timeout", decoded.Content)
}

func TestSession_HandleCoordinatorProcessEvent_ToolCall(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-coord-event-toolcall"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	// Simulate a tool call output (detected by 🔧 prefix)
	event := events.ProcessEvent{
		Type:      events.ProcessOutput,
		ProcessID: "coordinator",
		Role:      events.RoleCoordinator,
		Output:    "🔧 Read(file.go)",
	}
	session.handleCoordinatorProcessEvent(event)

	// Close to flush buffers
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify tool call was marked
	msgsPath := filepath.Join(sessionDir, "coordinator", "messages.jsonl")
	data, err := os.ReadFile(msgsPath)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 1)

	var decoded chatrender.Message
	err = json.Unmarshal([]byte(lines[0]), &decoded)
	require.NoError(t, err)
	require.Equal(t, "coordinator", decoded.Role)
	require.Equal(t, "🔧 Read(file.go)", decoded.Content)
	require.True(t, decoded.IsToolCall)
}

func TestSession_HandleCoordinatorProcessEvent_TokenUsage(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-coord-event-tokenusage"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	// Simulate a token usage event
	event := events.ProcessEvent{
		Type:      events.ProcessTokenUsage,
		ProcessID: "coordinator",
		Role:      events.RoleCoordinator,
		Metrics: &metrics.TokenMetrics{
			TokensUsed:   1000,
			OutputTokens: 500,
			TotalCostUSD: 0.05,
		},
	}
	session.handleCoordinatorProcessEvent(event)

	// Close to flush buffers
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify token usage was updated in metadata
	meta, err := Load(sessionDir)
	require.NoError(t, err)
	require.Equal(t, 500, meta.TokenUsage.TotalOutputTokens)
	require.InDelta(t, 0.05, meta.TokenUsage.TotalCostUSD, 0.001)
	// Verify coordinator-specific tracking
	require.Equal(t, 500, meta.CoordinatorTokenUsage.TotalOutputTokens)
	require.InDelta(t, 0.05, meta.CoordinatorTokenUsage.TotalCostUSD, 0.001)
}

// Tests for handleProcessEvent (worker events)

func TestSession_HandleProcessEvent_WorkerOutput(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-worker-event-output"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	// Simulate a worker output event
	event := events.ProcessEvent{
		Type:      events.ProcessOutput,
		ProcessID: "worker-1",
		Role:      events.RoleWorker,
		Output:    "Starting implementation of task...",
	}
	session.handleProcessEvent(event)

	// Close to flush buffers
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify message was written to worker's messages.jsonl
	msgsPath := filepath.Join(sessionDir, "workers", "worker-1", "messages.jsonl")
	data, err := os.ReadFile(msgsPath)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 1)

	var decoded chatrender.Message
	err = json.Unmarshal([]byte(lines[0]), &decoded)
	require.NoError(t, err)
	require.Equal(t, "assistant", decoded.Role)
	require.Equal(t, "Starting implementation of task...", decoded.Content)
	require.False(t, decoded.IsToolCall)
}

func TestSession_HandleProcessEvent_WorkerIncoming(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-worker-event-incoming"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	// Simulate an incoming message from coordinator to worker
	event := events.ProcessEvent{
		Type:      events.ProcessIncoming,
		ProcessID: "worker-1",
		Role:      events.RoleWorker,
		Message:   "Please work on task ms-123.",
		Sender:    "coordinator",
	}
	session.handleProcessEvent(event)

	// Close to flush buffers
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify message was written with role="coordinator"
	msgsPath := filepath.Join(sessionDir, "workers", "worker-1", "messages.jsonl")
	data, err := os.ReadFile(msgsPath)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 1)

	var decoded chatrender.Message
	err = json.Unmarshal([]byte(lines[0]), &decoded)
	require.NoError(t, err)
	require.Equal(t, "coordinator", decoded.Role)
	require.Equal(t, "Please work on task ms-123.", decoded.Content)
}

func TestSession_HandleProcessEvent_WorkerIncoming_DefaultRole(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-worker-event-incoming-default"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	// Simulate an incoming message with empty Sender (should default to "coordinator")
	event := events.ProcessEvent{
		Type:      events.ProcessIncoming,
		ProcessID: "worker-1",
		Role:      events.RoleWorker,
		Message:   "Work on this task.",
		Sender:    "", // Empty - should default to "coordinator"
	}
	session.handleProcessEvent(event)

	// Close to flush buffers
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify message was written with role="coordinator" (default)
	msgsPath := filepath.Join(sessionDir, "workers", "worker-1", "messages.jsonl")
	data, err := os.ReadFile(msgsPath)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 1)

	var decoded chatrender.Message
	err = json.Unmarshal([]byte(lines[0]), &decoded)
	require.NoError(t, err)
	require.Equal(t, "coordinator", decoded.Role)
}

func TestSession_HandleProcessEvent_WorkerError(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-worker-event-error"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	// Simulate a worker error event
	event := events.ProcessEvent{
		Type:      events.ProcessError,
		ProcessID: "worker-1",
		Role:      events.RoleWorker,
		Error:     fmt.Errorf("task failed: test compilation error"),
	}
	session.handleProcessEvent(event)

	// Close to flush buffers
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify error message was written with role="system"
	msgsPath := filepath.Join(sessionDir, "workers", "worker-1", "messages.jsonl")
	data, err := os.ReadFile(msgsPath)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 1)

	var decoded chatrender.Message
	err = json.Unmarshal([]byte(lines[0]), &decoded)
	require.NoError(t, err)
	require.Equal(t, "system", decoded.Role)
	require.Equal(t, "Error: task failed: test compilation error", decoded.Content)
}

func TestSession_HandleProcessEvent_WorkerToolCall(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-worker-event-toolcall"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	// Simulate a worker tool call output (detected by 🔧 prefix)
	event := events.ProcessEvent{
		Type:      events.ProcessOutput,
		ProcessID: "worker-1",
		Role:      events.RoleWorker,
		Output:    "🔧 Edit(session.go)",
	}
	session.handleProcessEvent(event)

	// Close to flush buffers
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify tool call was marked
	msgsPath := filepath.Join(sessionDir, "workers", "worker-1", "messages.jsonl")
	data, err := os.ReadFile(msgsPath)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 1)

	var decoded chatrender.Message
	err = json.Unmarshal([]byte(lines[0]), &decoded)
	require.NoError(t, err)
	require.Equal(t, "assistant", decoded.Role)
	require.Equal(t, "🔧 Edit(session.go)", decoded.Content)
	require.True(t, decoded.IsToolCall)
}

func TestSession_HandleProcessEvent_WorkerSpawned(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-worker-event-spawned"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	// Simulate a worker spawned event
	event := events.ProcessEvent{
		Type:      events.ProcessSpawned,
		ProcessID: "worker-1",
		Role:      events.RoleWorker,
	}
	session.handleProcessEvent(event)

	// Close to flush buffers
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify spawn message was written as system message
	msgsPath := filepath.Join(sessionDir, "workers", "worker-1", "messages.jsonl")
	data, err := os.ReadFile(msgsPath)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 1)

	var decoded chatrender.Message
	err = json.Unmarshal([]byte(lines[0]), &decoded)
	require.NoError(t, err)
	require.Equal(t, "system", decoded.Role)
	require.Equal(t, "Worker spawned", decoded.Content)
}

// Tests for Session Resumption Methods (perles-v1n6.2)

func TestSession_SetCoordinatorSessionRef_PersistsImmediately(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-coord-session-ref"
	sessionDir := filepath.Join(baseDir, "session")

	sess, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close(StatusCompleted) })

	// Set coordinator session ref
	err = sess.SetCoordinatorSessionRef("claude-session-xyz-123")
	require.NoError(t, err)

	// Verify metadata was persisted immediately (without closing session)
	meta, err := Load(sessionDir)
	require.NoError(t, err)
	require.Equal(t, "claude-session-xyz-123", meta.CoordinatorSessionRef)
}

func TestSession_SetCoordinatorSessionRef_ReturnsErrClosedWhenClosed(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-coord-ref-closed"
	sessionDir := filepath.Join(baseDir, "session")

	sess, err := New(sessionID, sessionDir)
	require.NoError(t, err)

	// Close the session
	err = sess.Close(StatusCompleted)
	require.NoError(t, err)

	// Attempt to set coordinator session ref
	err = sess.SetCoordinatorSessionRef("some-ref")
	require.ErrorIs(t, err, os.ErrClosed)
}

func TestSession_SetWorkerSessionRef_UpdatesCorrectWorker(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-worker-session-ref"
	sessionDir := filepath.Join(baseDir, "session")
	workDir := "/path/to/project"

	sess, err := New(sessionID, sessionDir, WithWorkDir(workDir))
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close(StatusCompleted) })

	// Add workers
	sess.addWorker("worker-1", time.Now(), workDir)
	sess.addWorker("worker-2", time.Now(), workDir)

	// Set session ref for worker-2
	err = sess.SetWorkerSessionRef("worker-2", "worker-2-session-abc", "/project/worktree-2")
	require.NoError(t, err)

	// Verify metadata was persisted with correct worker updated
	meta, err := Load(sessionDir)
	require.NoError(t, err)
	require.Len(t, meta.Workers, 2)

	// Find worker-2 and verify its fields
	var worker2 *WorkerMetadata
	for i := range meta.Workers {
		if meta.Workers[i].ID == "worker-2" {
			worker2 = &meta.Workers[i]
			break
		}
	}
	require.NotNil(t, worker2, "worker-2 should exist in metadata")
	require.Equal(t, "worker-2-session-abc", worker2.HeadlessSessionRef)
	require.Equal(t, "/project/worktree-2", worker2.WorkDir)

	// worker-1 should not be affected
	var worker1 *WorkerMetadata
	for i := range meta.Workers {
		if meta.Workers[i].ID == "worker-1" {
			worker1 = &meta.Workers[i]
			break
		}
	}
	require.NotNil(t, worker1, "worker-1 should exist in metadata")
	require.Empty(t, worker1.HeadlessSessionRef)
}

func TestSession_SetWorkerSessionRef_ReturnsErrorForUnknownWorker(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-unknown-worker"
	sessionDir := filepath.Join(baseDir, "session")

	sess, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close(StatusCompleted) })

	// Try to set session ref for non-existent worker
	err = sess.SetWorkerSessionRef("worker-unknown", "some-ref", "/some/path")
	require.Error(t, err)
	require.Contains(t, err.Error(), "worker not found")
	require.Contains(t, err.Error(), "worker-unknown")
}

func TestSession_SetWorkerSessionRef_ReturnsErrClosedWhenClosed(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-worker-ref-closed"
	sessionDir := filepath.Join(baseDir, "session")

	sess, err := New(sessionID, sessionDir)
	require.NoError(t, err)

	// Add a worker before closing
	sess.addWorker("worker-1", time.Now(), "/path")

	// Close the session
	err = sess.Close(StatusCompleted)
	require.NoError(t, err)

	// Attempt to set worker session ref
	err = sess.SetWorkerSessionRef("worker-1", "some-ref", "/some/path")
	require.ErrorIs(t, err, os.ErrClosed)
}

func TestSession_MarkResumable_SetsAndPersists(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-mark-resumable"
	sessionDir := filepath.Join(baseDir, "session")

	sess, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close(StatusCompleted) })

	// Initially, resumable should be false
	meta, err := Load(sessionDir)
	require.NoError(t, err)
	require.False(t, meta.Resumable)

	// Mark resumable
	err = sess.MarkResumable()
	require.NoError(t, err)

	// Verify metadata was persisted with resumable=true
	meta, err = Load(sessionDir)
	require.NoError(t, err)
	require.True(t, meta.Resumable)
}

func TestSession_MarkResumable_ReturnsErrClosedWhenClosed(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-resumable-closed"
	sessionDir := filepath.Join(baseDir, "session")

	sess, err := New(sessionID, sessionDir)
	require.NoError(t, err)

	// Close the session
	err = sess.Close(StatusCompleted)
	require.NoError(t, err)

	// Attempt to mark resumable
	err = sess.MarkResumable()
	require.ErrorIs(t, err, os.ErrClosed)
}

func TestSession_NotifySessionRef_RoutesCoordinatorCorrectly(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-notify-coord"
	sessionDir := filepath.Join(baseDir, "session")

	sess, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close(StatusCompleted) })

	// Notify coordinator session ref
	err = sess.NotifySessionRef("coordinator", "coord-session-123", "/project/path")
	require.NoError(t, err)

	// Verify both coordinator session ref and resumable flag were set
	meta, err := Load(sessionDir)
	require.NoError(t, err)
	require.Equal(t, "coord-session-123", meta.CoordinatorSessionRef)
	require.True(t, meta.Resumable)
}

func TestSession_NotifySessionRef_RoutesWorkerCorrectly(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-notify-worker"
	sessionDir := filepath.Join(baseDir, "session")
	workDir := "/project/path"

	sess, err := New(sessionID, sessionDir, WithWorkDir(workDir))
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close(StatusCompleted) })

	// First add the worker
	sess.addWorker("worker-1", time.Now(), workDir)

	// Notify worker session ref
	err = sess.NotifySessionRef("worker-1", "worker-session-456", "/project/worktree-1")
	require.NoError(t, err)

	// Verify worker session ref was set
	meta, err := Load(sessionDir)
	require.NoError(t, err)
	require.Len(t, meta.Workers, 1)
	require.Equal(t, "worker-session-456", meta.Workers[0].HeadlessSessionRef)
	require.Equal(t, "/project/worktree-1", meta.Workers[0].WorkDir)

	// Verify resumable was NOT set (only coordinator triggers resumable)
	require.False(t, meta.Resumable)
}

func TestSession_ConcurrentSetCoordinatorSessionRef_ThreadSafe(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-concurrent-coord-ref"
	sessionDir := filepath.Join(baseDir, "session")

	sess, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close(StatusCompleted) })

	// Run concurrent calls
	const numGoroutines = 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := range numGoroutines {
		go func(idx int) {
			defer wg.Done()
			ref := fmt.Sprintf("session-ref-%d", idx)
			err := sess.SetCoordinatorSessionRef(ref)
			// Should not fail (no race conditions)
			if err != nil {
				t.Errorf("SetCoordinatorSessionRef failed: %v", err)
			}
		}(i)
	}

	wg.Wait()

	// Verify metadata is valid (one of the refs was written)
	meta, err := Load(sessionDir)
	require.NoError(t, err)
	require.NotEmpty(t, meta.CoordinatorSessionRef)
	require.Contains(t, meta.CoordinatorSessionRef, "session-ref-")
}

func TestSession_AddWorkerWithWorkDir(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-addworker-workdir"
	sessionDir := filepath.Join(baseDir, "session")
	workDir := "/custom/work/dir"

	sess, err := New(sessionID, sessionDir, WithWorkDir(workDir))
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close(StatusCompleted) })

	// Add worker with workDir
	spawnTime := time.Now().Truncate(time.Second)
	sess.addWorker("worker-test", spawnTime, workDir)

	// Close to persist metadata
	err = sess.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify worker was added with workDir
	meta, err := Load(sessionDir)
	require.NoError(t, err)
	require.Len(t, meta.Workers, 1)
	require.Equal(t, "worker-test", meta.Workers[0].ID)
	require.Equal(t, workDir, meta.Workers[0].WorkDir)
	require.True(t, meta.Workers[0].SpawnedAt.Equal(spawnTime))
}

func TestSession_SaveMetadataLocked_CreatesMetadataIfMissing(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-save-creates-meta"
	sessionDir := filepath.Join(baseDir, "session")

	sess, err := New(sessionID, sessionDir,
		WithApplicationName("test-app"),
		WithWorkDir("/test/project"),
		WithDatePartition("2026-01-11"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close(StatusCompleted) })

	// Delete the metadata file to simulate corruption/missing
	metaPath := filepath.Join(sessionDir, "metadata.json")
	err = os.Remove(metaPath)
	require.NoError(t, err)

	// Set coordinator session ref (which calls saveMetadataLocked)
	err = sess.SetCoordinatorSessionRef("new-session-ref")
	require.NoError(t, err)

	// Verify metadata was recreated with session context
	meta, err := Load(sessionDir)
	require.NoError(t, err)
	require.Equal(t, sessionID, meta.SessionID)
	require.Equal(t, "new-session-ref", meta.CoordinatorSessionRef)
	require.Equal(t, "test-app", meta.ApplicationName)
	require.Equal(t, "/test/project", meta.WorkDir)
	require.Equal(t, "2026-01-11", meta.DatePartition)
}

// =============================================================================
// Reopen Tests
// =============================================================================

func TestReopen_ReturnsSessionWithCorrectID(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-reopen-id"
	sessionDir := filepath.Join(baseDir, "session")

	// Create initial session
	origSess, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	err = origSess.Close(StatusCompleted)
	require.NoError(t, err)

	// Reopen the session
	sess, err := Reopen(sessionID, sessionDir)
	require.NoError(t, err)
	defer func() { _ = sess.Close(StatusCompleted) }()

	// Verify session has correct ID and directory
	require.Equal(t, sessionID, sess.ID)
	require.Equal(t, sessionDir, sess.Dir)
	require.Equal(t, StatusRunning, sess.Status)
}

func TestReopen_SetsStartTime(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-reopen-starttime"
	sessionDir := filepath.Join(baseDir, "session")

	// Create initial session
	origSess, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	originalStartTime := origSess.StartTime
	err = origSess.Close(StatusCompleted)
	require.NoError(t, err)

	// Wait a moment to ensure different timestamp
	time.Sleep(10 * time.Millisecond)

	// Reopen the session
	sess, err := Reopen(sessionID, sessionDir)
	require.NoError(t, err)
	defer func() { _ = sess.Close(StatusCompleted) }()

	// StartTime should be set to current time (not original)
	require.True(t, sess.StartTime.After(originalStartTime))
}

func TestReopen_AppliesOptions(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-reopen-options"
	sessionDir := filepath.Join(baseDir, "session")

	// Create initial session
	origSess, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	err = origSess.Close(StatusCompleted)
	require.NoError(t, err)

	// Reopen with options
	sess, err := Reopen(sessionID, sessionDir,
		WithWorkDir("/new/work/dir"),
		WithApplicationName("new-app-name"),
		WithDatePartition("2026-01-12"),
	)
	require.NoError(t, err)
	defer func() { _ = sess.Close(StatusCompleted) }()

	// Verify options were applied
	require.Equal(t, "/new/work/dir", sess.workDir)
	require.Equal(t, "new-app-name", sess.applicationName)
	require.Equal(t, "2026-01-12", sess.datePartition)
}

func TestReopen_OpensFilesInAppendMode(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-reopen-append"
	sessionDir := filepath.Join(baseDir, "session")

	// Create initial session and write some content
	origSess, err := New(sessionID, sessionDir)
	require.NoError(t, err)

	// Write initial messages
	now := time.Now()
	initialMsg := chatrender.Message{
		Role:      "user",
		Content:   "Initial message",
		Timestamp: &now,
	}
	err = origSess.WriteCoordinatorMessage(initialMsg)
	require.NoError(t, err)

	err = origSess.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify initial content exists
	coordMsgsPath := filepath.Join(sessionDir, "coordinator", "messages.jsonl")
	initialContent, err := os.ReadFile(coordMsgsPath)
	require.NoError(t, err)
	require.Contains(t, string(initialContent), "Initial message")

	// Reopen and write more content
	sess, err := Reopen(sessionID, sessionDir)
	require.NoError(t, err)

	newMsg := chatrender.Message{
		Role:      "assistant",
		Content:   "New message after reopen",
		Timestamp: &now,
	}
	err = sess.WriteCoordinatorMessage(newMsg)
	require.NoError(t, err)

	err = sess.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify both messages exist (append, not overwrite)
	finalContent, err := os.ReadFile(coordMsgsPath)
	require.NoError(t, err)
	require.Contains(t, string(finalContent), "Initial message")
	require.Contains(t, string(finalContent), "New message after reopen")
}

func TestReopen_AppendsToExistingMessages(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-reopen-append-verify"
	sessionDir := filepath.Join(baseDir, "session")

	// Create initial session and write messages
	origSess, err := New(sessionID, sessionDir)
	require.NoError(t, err)

	// Write to messages.jsonl (inter-agent messages)
	entry1 := message.Entry{
		ID:      "msg-1",
		From:    "coordinator",
		To:      "worker-1",
		Content: "First inter-agent message",
	}
	err = origSess.WriteMessage(entry1)
	require.NoError(t, err)

	err = origSess.Close(StatusCompleted)
	require.NoError(t, err)

	// Reopen and write more
	sess, err := Reopen(sessionID, sessionDir)
	require.NoError(t, err)

	entry2 := message.Entry{
		ID:      "msg-2",
		From:    "worker-1",
		To:      "coordinator",
		Content: "Second message after reopen",
	}
	err = sess.WriteMessage(entry2)
	require.NoError(t, err)

	err = sess.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify both messages exist in JSONL file
	messagesPath := filepath.Join(sessionDir, "messages.jsonl")
	content, err := os.ReadFile(messagesPath)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	require.Len(t, lines, 2, "Expected 2 JSONL lines")
	require.Contains(t, lines[0], "First inter-agent message")
	require.Contains(t, lines[1], "Second message after reopen")
}

func TestReopen_HandlesNonexistentDir(t *testing.T) {
	baseDir := t.TempDir()
	sessionDir := filepath.Join(baseDir, "nonexistent", "session")

	// Attempt to reopen non-existent directory
	_, err := Reopen("test-id", sessionDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "session directory does not exist")
}

func TestReopen_CleansUpOnPartialFailure(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-reopen-cleanup"
	sessionDir := filepath.Join(baseDir, "session")

	// Create initial session structure
	origSess, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	err = origSess.Close(StatusCompleted)
	require.NoError(t, err)

	// Delete coordinator/messages.jsonl to cause partial failure
	coordMsgsPath := filepath.Join(sessionDir, "coordinator", "messages.jsonl")
	err = os.Remove(coordMsgsPath)
	require.NoError(t, err)

	// Make coordinator directory read-only to prevent file creation
	coordDir := filepath.Join(sessionDir, "coordinator")
	err = os.Chmod(coordDir, 0500)
	require.NoError(t, err)
	t.Cleanup(func() {
		// Restore permissions for cleanup
		_ = os.Chmod(coordDir, 0750)
	})

	// Skip this test on non-Unix systems where permissions might work differently
	if runtime.GOOS == "windows" {
		t.Skip("Skipping permission-based test on Windows")
	}

	// Attempt to reopen (should fail when trying to open messages.jsonl)
	_, err = Reopen(sessionID, sessionDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "reopening coordinator messages.jsonl")

	// Verify the previously opened file (raw.jsonl) was cleaned up
	// We can't directly check file descriptors, but we can verify the error
	// message indicates proper cleanup flow (it got to messages.jsonl after raw.jsonl)
}

func TestReopen_ThenWriteMessages(t *testing.T) {
	// Integration test: Full flow of create -> close -> reopen -> write -> verify append
	baseDir := t.TempDir()
	sessionID := "test-reopen-integration"
	sessionDir := filepath.Join(baseDir, "session")

	// Phase 1: Create and populate initial session
	origSess, err := New(sessionID, sessionDir,
		WithWorkDir("/project"),
		WithApplicationName("test-app"),
	)
	require.NoError(t, err)

	// Add workers and messages
	now := time.Now()
	origSess.addWorker("worker-1", now, "/project")

	msg1 := chatrender.Message{Role: "user", Content: "User question", Timestamp: &now}
	err = origSess.WriteCoordinatorMessage(msg1)
	require.NoError(t, err)

	msg2 := chatrender.Message{Role: "assistant", Content: "Initial response", Timestamp: &now}
	err = origSess.WriteCoordinatorMessage(msg2)
	require.NoError(t, err)

	// Set session refs for resumability
	err = origSess.SetCoordinatorSessionRef("coord-session-123")
	require.NoError(t, err)
	err = origSess.MarkResumable()
	require.NoError(t, err)
	err = origSess.SetWorkerSessionRef("worker-1", "worker-session-456", "/project")
	require.NoError(t, err)

	err = origSess.Close(StatusCompleted)
	require.NoError(t, err)

	// Phase 2: Reopen and verify state restoration
	sess, err := Reopen(sessionID, sessionDir)
	require.NoError(t, err)

	// Verify workers were restored from metadata
	require.Len(t, sess.workers, 1)
	require.Equal(t, "worker-1", sess.workers[0].ID)
	require.Equal(t, "worker-session-456", sess.workers[0].HeadlessSessionRef)

	// Verify session refs were restored
	require.Equal(t, "coord-session-123", sess.coordinatorSessionRef)
	require.True(t, sess.resumable)

	// Verify application context was restored
	require.Equal(t, "/project", sess.workDir)
	require.Equal(t, "test-app", sess.applicationName)

	// Phase 3: Write new content
	msg3 := chatrender.Message{Role: "user", Content: "Follow-up question", Timestamp: &now}
	err = sess.WriteCoordinatorMessage(msg3)
	require.NoError(t, err)

	msg4 := chatrender.Message{Role: "assistant", Content: "Continued response", Timestamp: &now}
	err = sess.WriteCoordinatorMessage(msg4)
	require.NoError(t, err)

	err = sess.Close(StatusCompleted)
	require.NoError(t, err)

	// Phase 4: Verify all content was preserved and appended
	coordMsgsPath := filepath.Join(sessionDir, "coordinator", "messages.jsonl")
	content, err := os.ReadFile(coordMsgsPath)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	require.Len(t, lines, 4, "Expected 4 JSONL lines (2 original + 2 new)")

	// Verify content order (original messages first, then new)
	require.Contains(t, lines[0], "User question")
	require.Contains(t, lines[1], "Initial response")
	require.Contains(t, lines[2], "Follow-up question")
	require.Contains(t, lines[3], "Continued response")
}

func TestReopen_RestoresWorkersFromMetadata(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-reopen-restore-workers"
	sessionDir := filepath.Join(baseDir, "session")

	// Create initial session with workers
	origSess, err := New(sessionID, sessionDir)
	require.NoError(t, err)

	now := time.Now().Truncate(time.Second)
	origSess.addWorker("worker-1", now, "/project")
	origSess.addWorker("worker-2", now.Add(time.Minute), "/project")

	// Set session refs
	err = origSess.SetWorkerSessionRef("worker-1", "session-ref-1", "/project")
	require.NoError(t, err)
	err = origSess.SetWorkerSessionRef("worker-2", "session-ref-2", "/project")
	require.NoError(t, err)

	// Retire one worker
	origSess.retireWorker("worker-1", now.Add(30*time.Minute), "completed")

	err = origSess.Close(StatusCompleted)
	require.NoError(t, err)

	// Reopen and verify workers were restored
	sess, err := Reopen(sessionID, sessionDir)
	require.NoError(t, err)
	defer func() { _ = sess.Close(StatusCompleted) }()

	require.Len(t, sess.workers, 2)

	// Worker 1 (retired)
	require.Equal(t, "worker-1", sess.workers[0].ID)
	require.Equal(t, "session-ref-1", sess.workers[0].HeadlessSessionRef)
	require.Equal(t, "completed", sess.workers[0].FinalPhase)
	require.False(t, sess.workers[0].RetiredAt.IsZero())

	// Worker 2 (active)
	require.Equal(t, "worker-2", sess.workers[1].ID)
	require.Equal(t, "session-ref-2", sess.workers[1].HeadlessSessionRef)
	require.True(t, sess.workers[1].RetiredAt.IsZero())
}

func TestReopen_CanAddNewWorkersAfterReopen(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-reopen-add-workers"
	sessionDir := filepath.Join(baseDir, "session")

	// Create initial session with one worker
	origSess, err := New(sessionID, sessionDir)
	require.NoError(t, err)

	now := time.Now().Truncate(time.Second)
	origSess.addWorker("worker-1", now, "/project")

	err = origSess.Close(StatusCompleted)
	require.NoError(t, err)

	// Reopen and add new worker
	sess, err := Reopen(sessionID, sessionDir)
	require.NoError(t, err)

	// Add new worker
	sess.addWorker("worker-2", now.Add(time.Hour), "/project")

	err = sess.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify both workers exist in metadata
	meta, err := Load(sessionDir)
	require.NoError(t, err)
	require.Len(t, meta.Workers, 2)
	require.Equal(t, "worker-1", meta.Workers[0].ID)
	require.Equal(t, "worker-2", meta.Workers[1].ID)
}

func TestReopen_InvalidPath_NotADirectory(t *testing.T) {
	baseDir := t.TempDir()

	// Create a file instead of a directory
	filePath := filepath.Join(baseDir, "not-a-dir")
	err := os.WriteFile(filePath, []byte("test"), 0600)
	require.NoError(t, err)

	// Attempt to reopen
	_, err = Reopen("test-id", filePath)
	require.Error(t, err)
	require.Contains(t, err.Error(), "session path is not a directory")
}

func TestReopen_MissingMetadata(t *testing.T) {
	baseDir := t.TempDir()
	sessionDir := filepath.Join(baseDir, "session")

	// Create directory without metadata
	err := os.MkdirAll(sessionDir, 0750)
	require.NoError(t, err)

	// Attempt to reopen
	_, err = Reopen("test-id", sessionDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "loading session metadata")
}

// Tests for GetWorkflowCompletedAt and UpdateWorkflowCompletion

func TestSession_GetWorkflowCompletedAt_ReturnsZeroWhenNotSet(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-workflow-not-set"
	sessionDir := filepath.Join(baseDir, "session")

	sess, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close(StatusCompleted) })

	// Workflow completed at should be zero when not set
	completedAt := sess.GetWorkflowCompletedAt()
	require.True(t, completedAt.IsZero())
}

func TestSession_GetWorkflowCompletedAt_ReturnsZeroWhenClosed(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-workflow-closed"
	sessionDir := filepath.Join(baseDir, "session")

	sess, err := New(sessionID, sessionDir)
	require.NoError(t, err)

	// Close the session
	err = sess.Close(StatusCompleted)
	require.NoError(t, err)

	// Should return zero time when session is closed
	completedAt := sess.GetWorkflowCompletedAt()
	require.True(t, completedAt.IsZero())
}

func TestSession_GetWorkflowCompletedAt_ReturnsTimestampWhenSet(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-workflow-set"
	sessionDir := filepath.Join(baseDir, "session")

	sess, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close(StatusCompleted) })

	// Set workflow completion
	expectedTime := time.Now().UTC().Truncate(time.Second)
	err = sess.UpdateWorkflowCompletion("success", "All tasks completed", expectedTime)
	require.NoError(t, err)

	// Should return the completion timestamp
	completedAt := sess.GetWorkflowCompletedAt()
	require.True(t, expectedTime.Equal(completedAt),
		"Expected %v, got %v", expectedTime, completedAt)
}

func TestSession_UpdateWorkflowCompletion_PersistsImmediately(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-workflow-persist"
	sessionDir := filepath.Join(baseDir, "session")

	sess, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close(StatusCompleted) })

	// Update workflow completion
	completedAt := time.Now().UTC().Truncate(time.Second)
	err = sess.UpdateWorkflowCompletion("partial", "Completed 3 of 5 tasks", completedAt)
	require.NoError(t, err)

	// Verify metadata was persisted immediately (without closing session)
	meta, err := Load(sessionDir)
	require.NoError(t, err)
	require.Equal(t, "partial", meta.WorkflowCompletionStatus)
	require.Equal(t, "Completed 3 of 5 tasks", meta.WorkflowSummary)
	require.True(t, completedAt.Equal(meta.WorkflowCompletedAt),
		"Expected %v, got %v", completedAt, meta.WorkflowCompletedAt)
}

func TestSession_UpdateWorkflowCompletion_ReturnsErrClosedWhenClosed(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-workflow-err-closed"
	sessionDir := filepath.Join(baseDir, "session")

	sess, err := New(sessionID, sessionDir)
	require.NoError(t, err)

	// Close the session
	err = sess.Close(StatusCompleted)
	require.NoError(t, err)

	// Attempt to update workflow completion
	err = sess.UpdateWorkflowCompletion("success", "summary", time.Now())
	require.ErrorIs(t, err, os.ErrClosed)
}

func TestSession_UpdateWorkflowCompletion_AllStatusValues(t *testing.T) {
	testCases := []struct {
		status  string
		summary string
	}{
		{"success", "All tasks completed successfully"},
		{"partial", "Completed 3 of 5 tasks"},
		{"aborted", "User cancelled workflow"},
	}

	for _, tc := range testCases {
		t.Run(tc.status, func(t *testing.T) {
			baseDir := t.TempDir()
			sessionID := "test-workflow-status-" + tc.status
			sessionDir := filepath.Join(baseDir, "session")

			sess, err := New(sessionID, sessionDir)
			require.NoError(t, err)
			t.Cleanup(func() { _ = sess.Close(StatusCompleted) })

			completedAt := time.Now().UTC().Truncate(time.Second)
			err = sess.UpdateWorkflowCompletion(tc.status, tc.summary, completedAt)
			require.NoError(t, err)

			// Verify via Load
			meta, err := Load(sessionDir)
			require.NoError(t, err)
			require.Equal(t, tc.status, meta.WorkflowCompletionStatus)
			require.Equal(t, tc.summary, meta.WorkflowSummary)
			require.True(t, completedAt.Equal(meta.WorkflowCompletedAt))
		})
	}
}

func TestSession_UpdateWorkflowCompletion_OverwritesPreviousValues(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-workflow-overwrite"
	sessionDir := filepath.Join(baseDir, "session")

	sess, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close(StatusCompleted) })

	// First update
	firstTime := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	err = sess.UpdateWorkflowCompletion("partial", "First summary", firstTime)
	require.NoError(t, err)

	// Second update (overwrites)
	secondTime := time.Now().UTC().Truncate(time.Second)
	err = sess.UpdateWorkflowCompletion("success", "Second summary", secondTime)
	require.NoError(t, err)

	// Verify second values are persisted
	meta, err := Load(sessionDir)
	require.NoError(t, err)
	require.Equal(t, "success", meta.WorkflowCompletionStatus)
	require.Equal(t, "Second summary", meta.WorkflowSummary)
	require.True(t, secondTime.Equal(meta.WorkflowCompletedAt))
}

func TestSession_UpdateWorkflowCompletion_PreservesOtherMetadata(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-workflow-preserve"
	sessionDir := filepath.Join(baseDir, "session")
	workDir := "/project/path"

	sess, err := New(sessionID, sessionDir,
		WithWorkDir(workDir),
		WithApplicationName("test-app"),
		WithDatePartition("2026-01-14"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close(StatusCompleted) })

	// Add a worker
	sess.addWorker("worker-1", time.Now(), workDir)

	// Set coordinator session ref
	err = sess.SetCoordinatorSessionRef("coord-session-abc")
	require.NoError(t, err)

	// Update workflow completion
	completedAt := time.Now().UTC().Truncate(time.Second)
	err = sess.UpdateWorkflowCompletion("success", "All done", completedAt)
	require.NoError(t, err)

	// Verify other metadata fields are preserved
	meta, err := Load(sessionDir)
	require.NoError(t, err)

	// Workflow fields
	require.Equal(t, "success", meta.WorkflowCompletionStatus)
	require.Equal(t, "All done", meta.WorkflowSummary)

	// Other fields should be preserved
	require.Equal(t, sessionID, meta.SessionID)
	require.Equal(t, StatusRunning, meta.Status)
	require.Equal(t, workDir, meta.WorkDir)
	require.Equal(t, "test-app", meta.ApplicationName)
	require.Equal(t, "2026-01-14", meta.DatePartition)
}

// =============================================================================
// Workflow State Management Tests
// =============================================================================

func TestSession_SetActiveWorkflowState_PersistsToFile(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-workflow-set"
	sessionDir := filepath.Join(baseDir, "session")

	sess, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close(StatusCompleted) })

	startedAt := time.Now().UTC().Truncate(time.Second)
	state := &workflow.WorkflowState{
		WorkflowID:      "wf-abc123",
		WorkflowName:    "Test Workflow",
		WorkflowContent: "# Test\nSome workflow content",
		StartedAt:       startedAt,
	}

	err = sess.SetActiveWorkflowState(state)
	require.NoError(t, err)

	// Verify file was created
	statePath := filepath.Join(sessionDir, workflow.WorkflowStateFilename)
	_, err = os.Stat(statePath)
	require.NoError(t, err, "workflow state file should exist")

	// Verify file contents
	data, err := os.ReadFile(statePath)
	require.NoError(t, err)

	var loaded workflow.WorkflowState
	err = json.Unmarshal(data, &loaded)
	require.NoError(t, err)

	require.Equal(t, "wf-abc123", loaded.WorkflowID)
	require.Equal(t, "Test Workflow", loaded.WorkflowName)
	require.Equal(t, "# Test\nSome workflow content", loaded.WorkflowContent)
	require.True(t, startedAt.Equal(loaded.StartedAt))
}

func TestSession_GetActiveWorkflowState_ReturnsCached(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-workflow-cached"
	sessionDir := filepath.Join(baseDir, "session")

	sess, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close(StatusCompleted) })

	state := &workflow.WorkflowState{
		WorkflowID:   "wf-cached",
		WorkflowName: "Cached Workflow",
	}

	err = sess.SetActiveWorkflowState(state)
	require.NoError(t, err)

	// Get should return cached value
	got, err := sess.GetActiveWorkflowState()
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "wf-cached", got.WorkflowID)
	require.Equal(t, "Cached Workflow", got.WorkflowName)

	// Verify it's the same pointer (cached)
	require.Same(t, state, got)
}

func TestSession_GetActiveWorkflowState_LoadsFromDiskWhenCacheEmpty(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-workflow-disk"
	sessionDir := filepath.Join(baseDir, "session")

	// Create session and set state
	sess, err := New(sessionID, sessionDir)
	require.NoError(t, err)

	startedAt := time.Now().UTC().Truncate(time.Second)
	state := &workflow.WorkflowState{
		WorkflowID:      "wf-disk",
		WorkflowName:    "Disk Workflow",
		WorkflowContent: "Content from disk",
		StartedAt:       startedAt,
	}

	err = sess.SetActiveWorkflowState(state)
	require.NoError(t, err)
	_ = sess.Close(StatusCompleted)

	// Reopen session (cache is empty)
	sess2, err := Reopen(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess2.Close(StatusCompleted) })

	// Get should load from disk
	got, err := sess2.GetActiveWorkflowState()
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "wf-disk", got.WorkflowID)
	require.Equal(t, "Disk Workflow", got.WorkflowName)
	require.Equal(t, "Content from disk", got.WorkflowContent)
	require.True(t, startedAt.Equal(got.StartedAt))
}

func TestSession_GetActiveWorkflowState_ReturnsNilWhenNoFile(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-workflow-nofile"
	sessionDir := filepath.Join(baseDir, "session")

	sess, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close(StatusCompleted) })

	// Get without setting should return nil
	got, err := sess.GetActiveWorkflowState()
	require.NoError(t, err)
	require.Nil(t, got, "should return nil when no workflow state file exists")
}

func TestSession_ClearActiveWorkflowState_RemovesFileAndCache(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-workflow-clear"
	sessionDir := filepath.Join(baseDir, "session")

	sess, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close(StatusCompleted) })

	// Set state first
	state := &workflow.WorkflowState{
		WorkflowID:   "wf-clear",
		WorkflowName: "Clear Workflow",
	}
	err = sess.SetActiveWorkflowState(state)
	require.NoError(t, err)

	// Verify file exists
	statePath := filepath.Join(sessionDir, workflow.WorkflowStateFilename)
	_, err = os.Stat(statePath)
	require.NoError(t, err, "workflow state file should exist before clear")

	// Clear the state
	err = sess.ClearActiveWorkflowState()
	require.NoError(t, err)

	// Verify file is deleted
	_, err = os.Stat(statePath)
	require.True(t, os.IsNotExist(err), "workflow state file should be deleted")

	// Verify cache is cleared
	got, err := sess.GetActiveWorkflowState()
	require.NoError(t, err)
	require.Nil(t, got, "should return nil after clear")
}

func TestSession_ClearActiveWorkflowState_NoErrorWhenNoFile(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-workflow-clear-nofile"
	sessionDir := filepath.Join(baseDir, "session")

	sess, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close(StatusCompleted) })

	// Clear without setting should not error
	err = sess.ClearActiveWorkflowState()
	require.NoError(t, err, "clearing non-existent workflow state should not error")
}

func TestSession_WorkflowState_RoundTrip(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-workflow-roundtrip"
	sessionDir := filepath.Join(baseDir, "session")

	sess, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close(StatusCompleted) })

	startedAt := time.Now().UTC().Truncate(time.Second)
	original := &workflow.WorkflowState{
		WorkflowID:      "wf-roundtrip-xyz",
		WorkflowName:    "Complex Workflow Name",
		WorkflowContent: "# Workflow\n\nStep 1: Do X\nStep 2: Do Y\n\n## Notes\n\nSome notes here.",
		StartedAt:       startedAt,
	}

	// Set
	err = sess.SetActiveWorkflowState(original)
	require.NoError(t, err)

	// Clear cache to force disk read
	sess.mu.Lock()
	sess.activeWorkflowState = nil
	sess.mu.Unlock()

	// Get
	got, err := sess.GetActiveWorkflowState()
	require.NoError(t, err)
	require.NotNil(t, got)

	// Verify all fields preserved
	require.Equal(t, original.WorkflowID, got.WorkflowID)
	require.Equal(t, original.WorkflowName, got.WorkflowName)
	require.Equal(t, original.WorkflowContent, got.WorkflowContent)
	require.True(t, original.StartedAt.Equal(got.StartedAt), "StartedAt should be preserved")
}

func TestSession_WorkflowState_ClosedSessionErrors(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-workflow-closed"
	sessionDir := filepath.Join(baseDir, "session")

	sess, err := New(sessionID, sessionDir)
	require.NoError(t, err)

	// Close the session
	err = sess.Close(StatusCompleted)
	require.NoError(t, err)

	state := &workflow.WorkflowState{
		WorkflowID:   "wf-closed",
		WorkflowName: "Test",
	}

	// SetActiveWorkflowState should error
	err = sess.SetActiveWorkflowState(state)
	require.ErrorIs(t, err, os.ErrClosed)

	// GetActiveWorkflowState should error
	_, err = sess.GetActiveWorkflowState()
	require.ErrorIs(t, err, os.ErrClosed)

	// ClearActiveWorkflowState should error
	err = sess.ClearActiveWorkflowState()
	require.ErrorIs(t, err, os.ErrClosed)
}

// TestSessionResume_MetricsLoaded verifies that when a session is reopened,
// token usage metrics ARE loaded from the prior metadata (not reset to zero).
// This is the correct behavior because:
// 1. Processes publish turn costs (not cumulative) via setMetrics()
// 2. updateTokenUsage() accumulates turn costs with +=
// 3. Loading prior metrics ensures the total reflects all work done
func TestSessionResume_MetricsLoaded(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-resume-metrics-loaded"
	sessionDir := filepath.Join(baseDir, "session")

	// Phase 1: Create initial session with token usage
	origSess, err := New(sessionID, sessionDir)
	require.NoError(t, err)

	// Simulate token usage from coordinator in original session
	// Parameters: processID, contextTokens, outputTokens, costUSD
	origSess.updateTokenUsage("coordinator", 10000, 500, 1.50) // 10k context, 500 output, $1.50

	err = origSess.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify metadata was saved with token usage
	meta, err := Load(sessionDir)
	require.NoError(t, err)
	require.Equal(t, 10000, meta.TokenUsage.ContextTokens)
	require.Equal(t, 500, meta.TokenUsage.TotalOutputTokens)
	require.InDelta(t, 1.50, meta.TokenUsage.TotalCostUSD, 0.001)
	require.Equal(t, 500, meta.CoordinatorTokenUsage.TotalOutputTokens)

	// Phase 2: Reopen and verify metrics ARE loaded (not reset)
	sess, err := Reopen(sessionID, sessionDir)
	require.NoError(t, err)
	defer func() { _ = sess.Close(StatusCompleted) }()

	// Verify the reopened session has the prior token usage loaded
	// This is the key assertion: metrics are NOT reset to zero
	sess.mu.Lock()
	contextTokens := sess.tokenUsage.ContextTokens
	outputTokens := sess.tokenUsage.TotalOutputTokens
	cost := sess.tokenUsage.TotalCostUSD
	coordOutput := sess.coordinatorTokenUsage.TotalOutputTokens
	sess.mu.Unlock()

	require.Equal(t, 10000, contextTokens, "Context tokens should be loaded from prior metadata")
	require.Equal(t, 500, outputTokens, "Output tokens should be loaded from prior metadata")
	require.InDelta(t, 1.50, cost, 0.001, "Cost should be loaded from prior metadata")
	require.Equal(t, 500, coordOutput, "Coordinator output tokens should be loaded from prior metadata")

	// Phase 3: Simulate additional turn cost and verify accumulation
	sess.updateTokenUsage("coordinator", 12000, 100, 0.25) // 12k context (replaces), 100 output (adds), $0.25 (adds)

	sess.mu.Lock()
	finalContext := sess.tokenUsage.ContextTokens
	finalOutput := sess.tokenUsage.TotalOutputTokens
	finalCost := sess.tokenUsage.TotalCostUSD
	sess.mu.Unlock()

	// Context tokens are REPLACED (current context window usage)
	require.Equal(t, 12000, finalContext, "Context tokens should be replaced: 12000")
	// Output tokens are ACCUMULATED
	require.Equal(t, 600, finalOutput, "Output tokens should be accumulated: 500 + 100 = 600")
	// Cost is ACCUMULATED from turn costs
	require.InDelta(t, 1.75, finalCost, 0.001, "Cost should be accumulated: 1.50 + 0.25 = 1.75")
}

// TestSessionResume_PriorMetadataPreserved verifies that reopening a session
// does not modify the metadata file until the reopened session is closed.
// The original metadata values are preserved and accessible.
func TestSessionResume_PriorMetadataPreserved(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-resume-metadata-preserved"
	sessionDir := filepath.Join(baseDir, "session")

	// Phase 1: Create initial session with known metadata
	origSess, err := New(sessionID, sessionDir)
	require.NoError(t, err)

	// Set token usage and workers
	// Parameters: processID, contextTokens, outputTokens, costUSD
	origSess.updateTokenUsage("coordinator", 20000, 800, 2.50)
	now := time.Now().Truncate(time.Second)
	origSess.addWorker("worker-1", now, "/project")

	err = origSess.Close(StatusCompleted)
	require.NoError(t, err)

	// Record original metadata state
	origMeta, err := Load(sessionDir)
	require.NoError(t, err)
	origContext := origMeta.TokenUsage.ContextTokens
	origOutputTokens := origMeta.TokenUsage.TotalOutputTokens
	origCost := origMeta.TokenUsage.TotalCostUSD
	origWorkerCount := len(origMeta.Workers)

	// Phase 2: Reopen session - verify metrics are loaded but not modified until new activity
	sess, err := Reopen(sessionID, sessionDir)
	require.NoError(t, err)

	// Verify original values are loaded
	sess.mu.Lock()
	loadedContext := sess.tokenUsage.ContextTokens
	loadedOutput := sess.tokenUsage.TotalOutputTokens
	loadedCost := sess.tokenUsage.TotalCostUSD
	sess.mu.Unlock()
	require.Equal(t, origContext, loadedContext, "Context tokens should be loaded")
	require.Equal(t, origOutputTokens, loadedOutput, "Output tokens should be loaded")
	require.InDelta(t, origCost, loadedCost, 0.001, "Cost should be loaded")

	// Add new activity to reopened session
	sess.updateTokenUsage("coordinator", 25000, 200, 0.50) // 25k context (replaces), 200 output, $0.50

	// Close the reopened session
	err = sess.Close(StatusCompleted)
	require.NoError(t, err)

	// Phase 3: Verify metadata is now updated with new totals
	finalMeta, err := Load(sessionDir)
	require.NoError(t, err)

	// Context replaced (25000), output accumulated (800+200=1000), cost accumulated (2.50+0.50=3.00)
	require.Equal(t, 25000, finalMeta.TokenUsage.ContextTokens, "Context should be replaced")
	require.Equal(t, 1000, finalMeta.TokenUsage.TotalOutputTokens, "Output should be accumulated")
	require.InDelta(t, 3.00, finalMeta.TokenUsage.TotalCostUSD, 0.001, "Cost should be accumulated")
	require.Equal(t, origWorkerCount, len(finalMeta.Workers), "Workers should be preserved")
}

// TestUpdateTokenUsage_CostOnlyEventDoesNotResetContext verifies that cost-only
// events (with contextTokens=0) do not reset previously tracked context tokens.
// This can happen when result events report cost but don't include token counts.
func TestUpdateTokenUsage_CostOnlyEventDoesNotResetContext(t *testing.T) {
	sessionDir := t.TempDir()
	sess, err := New("test-cost-only", sessionDir)
	require.NoError(t, err)
	defer func() { _ = sess.Close(StatusCompleted) }()

	// Add a worker
	sess.addWorker("worker-1", time.Now(), "/project")

	// First event: normal token usage with context
	sess.updateTokenUsage("coordinator", 50000, 100, 0.50)
	sess.updateTokenUsage("worker-1", 30000, 50, 0.25)

	// Verify initial context tokens
	sess.mu.Lock()
	require.Equal(t, 50000, sess.coordinatorTokenUsage.ContextTokens)
	require.Equal(t, 30000, sess.workers[0].TokenUsage.ContextTokens)
	sess.mu.Unlock()

	// Second event: cost-only (contextTokens=0) - should NOT reset context
	sess.updateTokenUsage("coordinator", 0, 200, 1.00)
	sess.updateTokenUsage("worker-1", 0, 100, 0.50)

	// Verify context tokens are preserved (not reset to 0)
	sess.mu.Lock()
	require.Equal(t, 50000, sess.coordinatorTokenUsage.ContextTokens,
		"Coordinator context should be preserved after cost-only event")
	require.Equal(t, 300, sess.coordinatorTokenUsage.TotalOutputTokens,
		"Coordinator output should be accumulated: 100 + 200 = 300")
	require.InDelta(t, 1.50, sess.coordinatorTokenUsage.TotalCostUSD, 0.001,
		"Coordinator cost should be accumulated: 0.50 + 1.00 = 1.50")

	require.Equal(t, 30000, sess.workers[0].TokenUsage.ContextTokens,
		"Worker context should be preserved after cost-only event")
	require.Equal(t, 150, sess.workers[0].TokenUsage.TotalOutputTokens,
		"Worker output should be accumulated: 50 + 100 = 150")
	require.InDelta(t, 0.75, sess.workers[0].TokenUsage.TotalCostUSD, 0.001,
		"Worker cost should be accumulated: 0.25 + 0.50 = 0.75")
	sess.mu.Unlock()

	// Third event: new context tokens should still replace
	sess.updateTokenUsage("coordinator", 60000, 50, 0.25)
	sess.updateTokenUsage("worker-1", 35000, 25, 0.10)

	sess.mu.Lock()
	require.Equal(t, 60000, sess.coordinatorTokenUsage.ContextTokens,
		"Coordinator context should be replaced with new value")
	require.Equal(t, 35000, sess.workers[0].TokenUsage.ContextTokens,
		"Worker context should be replaced with new value")
	sess.mu.Unlock()

	// Verify session totals are correct
	sess.mu.Lock()
	require.Equal(t, 60000+35000, sess.tokenUsage.ContextTokens,
		"Session context should be sum of coordinator + worker")
	require.Equal(t, 350+175, sess.tokenUsage.TotalOutputTokens,
		"Session output should be sum of all outputs")
	sess.mu.Unlock()
}
