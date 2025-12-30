package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/orchestration/metrics"
	"github.com/zjrosen/perles/internal/pubsub"
)

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

func TestMetadata_Save_Load(t *testing.T) {
	dir := t.TempDir()

	now := time.Now().Truncate(time.Second) // Truncate for JSON round-trip
	meta := &Metadata{
		SessionID:     "test-session-123",
		StartTime:     now,
		EndTime:       now.Add(time.Hour),
		Status:        StatusCompleted,
		WorkDir:       "/test/work/dir",
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
			TotalInputTokens:  125000,
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
	require.Equal(t, meta.WorkDir, loaded.WorkDir)
	require.Equal(t, meta.CoordinatorID, loaded.CoordinatorID)
	require.Equal(t, meta.ClientType, loaded.ClientType)
	require.Equal(t, meta.Model, loaded.Model)
	require.Equal(t, meta.TokenUsage.TotalInputTokens, loaded.TokenUsage.TotalInputTokens)
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
		SessionID: "test-nested",
		StartTime: time.Now(),
		Status:    StatusRunning,
		WorkDir:   "/test",
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
		SessionID: "empty-workers",
		StartTime: time.Now().Truncate(time.Second),
		Status:    StatusRunning,
		WorkDir:   "/test",
		Workers:   []WorkerMetadata{}, // Empty slice
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
		SessionID: "minimal",
		StartTime: time.Now().Truncate(time.Second),
		Status:    StatusRunning,
		WorkDir:   "/test",
	}

	err := meta.Save(dir)
	require.NoError(t, err)

	// Load and verify zero values are handled correctly
	loaded, err := Load(dir)
	require.NoError(t, err)
	require.True(t, loaded.EndTime.IsZero())
	require.Empty(t, loaded.CoordinatorID)
	require.Empty(t, loaded.Model)
	require.Equal(t, 0, loaded.TokenUsage.TotalInputTokens)
	require.Equal(t, 0, loaded.TokenUsage.TotalOutputTokens)
	require.Equal(t, 0.0, loaded.TokenUsage.TotalCostUSD)
}

// Tests for New() constructor

func TestNew_CreatesDirectoryStructure(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-session-123"
	sessionDir := filepath.Join(baseDir, ".perles", "sessions", sessionID)

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	require.NotNil(t, session)

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

	// Verify coordinator/output.log exists
	coordLog := filepath.Join(coordDir, "output.log")
	info, err = os.Stat(coordLog)
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

	// Load and verify the metadata
	meta, err := Load(sessionDir)
	require.NoError(t, err)

	require.Equal(t, sessionID, meta.SessionID)
	require.Equal(t, StatusRunning, meta.Status)
	require.Equal(t, sessionDir, meta.WorkDir)
	require.False(t, meta.StartTime.IsZero())
	require.True(t, meta.EndTime.IsZero())
	require.Empty(t, meta.CoordinatorID)
	require.NotNil(t, meta.Workers)
	require.Empty(t, meta.Workers)
}

func TestNew_FailsOnInvalidDir(t *testing.T) {
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

	// Verify all session fields are properly initialized
	require.Equal(t, sessionID, session.ID)
	require.Equal(t, sessionDir, session.Dir)
	require.Equal(t, StatusRunning, session.Status)
	require.False(t, session.StartTime.IsZero())
	require.NotNil(t, session.workerLogs)
	require.Empty(t, session.workerLogs)
	require.False(t, session.closed)

	// File handles should be non-nil
	require.NotNil(t, session.coordLog)
	require.NotNil(t, session.messageLog)
	require.NotNil(t, session.mcpLog)
}

func TestNew_MetadataJSONFormat(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "json-format-test"
	sessionDir := filepath.Join(baseDir, "session")

	_, err := New(sessionID, sessionDir)
	require.NoError(t, err)

	// Read the raw JSON file
	metadataPath := filepath.Join(sessionDir, "metadata.json")
	data, err := os.ReadFile(metadataPath)
	require.NoError(t, err)

	// Verify JSON structure contains expected fields
	jsonStr := string(data)
	require.Contains(t, jsonStr, `"session_id"`)
	require.Contains(t, jsonStr, `"start_time"`)
	require.Contains(t, jsonStr, `"status"`)
	require.Contains(t, jsonStr, `"work_dir"`)
	require.Contains(t, jsonStr, `"workers"`)

	// Verify status is "running"
	require.Contains(t, jsonStr, `"status": "running"`)

	// Verify session_id matches
	require.Contains(t, jsonStr, `"session_id": "json-format-test"`)
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

// Tests for WriteCoordinatorEvent

func TestSession_WriteCoordinatorEvent(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-coord-event"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	require.NotNil(t, session)

	// Write a coordinator event
	timestamp := time.Date(2025, 1, 15, 10, 30, 45, 0, time.UTC)
	err = session.WriteCoordinatorEvent(timestamp, "coordinator", "Starting orchestration session...")
	require.NoError(t, err)

	// Write another event
	timestamp2 := timestamp.Add(time.Second)
	err = session.WriteCoordinatorEvent(timestamp2, "tool_result", "Worker spawned successfully")
	require.NoError(t, err)

	// Close to flush buffers
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify file contents
	logPath := filepath.Join(sessionDir, "coordinator", "output.log")
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)

	// Check format: {ISO8601_timestamp} [{role}] {content}\n
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 2)

	require.Contains(t, lines[0], "2025-01-15T10:30:45Z")
	require.Contains(t, lines[0], "[coordinator]")
	require.Contains(t, lines[0], "Starting orchestration session...")

	require.Contains(t, lines[1], "2025-01-15T10:30:46Z")
	require.Contains(t, lines[1], "[tool_result]")
	require.Contains(t, lines[1], "Worker spawned successfully")
}

func TestSession_WriteCoordinatorEvent_AfterClose(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-coord-closed"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)

	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Writing after close should fail
	err = session.WriteCoordinatorEvent(time.Now(), "coordinator", "should fail")
	require.Error(t, err)
	require.Equal(t, os.ErrClosed, err)
}

// Tests for WriteWorkerEvent

func TestSession_WriteWorkerEvent(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-worker-event"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	require.NotNil(t, session)

	// Write events for different workers
	timestamp := time.Date(2025, 1, 15, 10, 30, 45, 0, time.UTC)
	err = session.WriteWorkerEvent("worker-1", timestamp, "Starting task implementation...")
	require.NoError(t, err)

	timestamp2 := timestamp.Add(time.Second)
	err = session.WriteWorkerEvent("worker-2", timestamp2, "Ready for task assignment")
	require.NoError(t, err)

	// Write another event for worker-1
	timestamp3 := timestamp.Add(2 * time.Second)
	err = session.WriteWorkerEvent("worker-1", timestamp3, "Task completed")
	require.NoError(t, err)

	// Close to flush buffers
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify worker-1 log
	worker1Log := filepath.Join(sessionDir, "workers", "worker-1", "output.log")
	data1, err := os.ReadFile(worker1Log)
	require.NoError(t, err)

	lines1 := strings.Split(strings.TrimSpace(string(data1)), "\n")
	require.Len(t, lines1, 2)
	require.Contains(t, lines1[0], "2025-01-15T10:30:45Z")
	require.Contains(t, lines1[0], "Starting task implementation...")
	require.Contains(t, lines1[1], "Task completed")

	// Verify worker-2 log
	worker2Log := filepath.Join(sessionDir, "workers", "worker-2", "output.log")
	data2, err := os.ReadFile(worker2Log)
	require.NoError(t, err)

	lines2 := strings.Split(strings.TrimSpace(string(data2)), "\n")
	require.Len(t, lines2, 1)
	require.Contains(t, lines2[0], "Ready for task assignment")
}

func TestSession_WriteWorkerEvent_LazyCreatesDirectory(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-worker-lazy"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)

	// Worker directory shouldn't exist yet
	worker3Path := filepath.Join(sessionDir, "workers", "worker-3")
	_, err = os.Stat(worker3Path)
	require.True(t, os.IsNotExist(err))

	// Write event - should create directory
	err = session.WriteWorkerEvent("worker-3", time.Now(), "Hello from worker-3")
	require.NoError(t, err)

	// Worker directory should now exist
	info, err := os.Stat(worker3Path)
	require.NoError(t, err)
	require.True(t, info.IsDir())

	// output.log should exist
	logPath := filepath.Join(worker3Path, "output.log")
	_, err = os.Stat(logPath)
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

// Tests for Close

func TestSession_Close(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-close"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)

	// Write some events before closing
	err = session.WriteCoordinatorEvent(time.Now(), "coordinator", "Starting...")
	require.NoError(t, err)

	err = session.WriteWorkerEvent("worker-1", time.Now(), "Working...")
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

	// Write several events (below flush threshold)
	for i := 0; i < 10; i++ {
		err = session.WriteCoordinatorEvent(time.Now(), "coordinator", "Event")
		require.NoError(t, err)
	}

	// Close should flush all buffered events
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify all events were written
	logPath := filepath.Join(sessionDir, "coordinator", "output.log")
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	require.Equal(t, 10, countLines(data))
}

func TestSession_Close_DoubleClose(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-double-close"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)

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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create brokers
	coordBroker := pubsub.NewBroker[events.CoordinatorEvent]()
	workerBroker := pubsub.NewBroker[events.WorkerEvent]()
	msgBroker := pubsub.NewBroker[message.Event]()
	mcpBroker := pubsub.NewBroker[events.MCPEvent]()

	// Attach to all brokers
	session.AttachToBrokers(ctx, coordBroker, workerBroker, msgBroker, mcpBroker)

	// Give goroutines time to start
	time.Sleep(10 * time.Millisecond)

	// Verify subscribers are attached (broker subscriber count should be 1 for each)
	require.Equal(t, 1, coordBroker.SubscriberCount())
	require.Equal(t, 1, workerBroker.SubscriberCount())
	require.Equal(t, 1, msgBroker.SubscriberCount())
	require.Equal(t, 1, mcpBroker.SubscriberCount())

	// Cleanup
	cancel()
	coordBroker.Close()
	workerBroker.Close()
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Should not panic with nil brokers
	session.AttachToBrokers(ctx, nil, nil, nil, nil)

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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	coordBroker := pubsub.NewBroker[events.CoordinatorEvent]()
	defer coordBroker.Close()

	session.AttachCoordinatorBroker(ctx, coordBroker)

	// Give goroutine time to start
	time.Sleep(10 * time.Millisecond)

	// Publish a chat event
	coordBroker.Publish(pubsub.UpdatedEvent, events.CoordinatorEvent{
		Type:    events.CoordinatorChat,
		Role:    "coordinator",
		Content: "Starting orchestration session...",
		RawJSON: []byte(`{"type":"chat","content":"Starting orchestration session..."}`),
	})

	// Publish a token usage event
	coordBroker.Publish(pubsub.UpdatedEvent, events.CoordinatorEvent{
		Type: events.CoordinatorTokenUsage,
		Metrics: &metrics.TokenMetrics{
			InputTokens:  100,
			OutputTokens: 50,
			TotalCostUSD: 0.05,
		},
	})

	// Give time for events to be processed
	time.Sleep(50 * time.Millisecond)

	// Close session to flush buffers
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify coordinator output.log has the chat event
	logPath := filepath.Join(sessionDir, "coordinator", "output.log")
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	require.Contains(t, string(data), "[coordinator]")
	require.Contains(t, string(data), "Starting orchestration session...")

	// Verify raw.jsonl has the raw JSON
	rawPath := filepath.Join(sessionDir, "coordinator", "raw.jsonl")
	rawData, err := os.ReadFile(rawPath)
	require.NoError(t, err)
	require.Contains(t, string(rawData), `"type":"chat"`)

	// Verify metadata has token usage
	meta, err := Load(sessionDir)
	require.NoError(t, err)
	require.Equal(t, 100, meta.TokenUsage.TotalInputTokens)
	require.Equal(t, 50, meta.TokenUsage.TotalOutputTokens)
	require.Equal(t, 0.05, meta.TokenUsage.TotalCostUSD)
}

func TestSession_WorkerSubscriber(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-worker-subscriber"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	workerBroker := pubsub.NewBroker[events.WorkerEvent]()
	defer workerBroker.Close()

	session.AttachToBrokers(ctx, nil, workerBroker, nil, nil)

	// Give goroutine time to start
	time.Sleep(10 * time.Millisecond)

	// Publish worker spawned event
	workerBroker.Publish(pubsub.UpdatedEvent, events.WorkerEvent{
		Type:     events.WorkerSpawned,
		WorkerID: "worker-1",
	})

	// Publish worker output event
	workerBroker.Publish(pubsub.UpdatedEvent, events.WorkerEvent{
		Type:     events.WorkerOutput,
		WorkerID: "worker-1",
		Output:   "Starting implementation...",
		RawJSON:  []byte(`{"type":"output","content":"Starting implementation..."}`),
	})

	// Publish worker status change (retired)
	workerBroker.Publish(pubsub.UpdatedEvent, events.WorkerEvent{
		Type:     events.WorkerStatusChange,
		WorkerID: "worker-1",
		Status:   events.WorkerRetired,
		Phase:    events.PhaseIdle,
	})

	// Give time for events to be processed
	time.Sleep(50 * time.Millisecond)

	// Close session to flush buffers
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify worker output.log has content
	logPath := filepath.Join(sessionDir, "workers", "worker-1", "output.log")
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	require.Contains(t, string(data), "Worker spawned")
	require.Contains(t, string(data), "Starting implementation...")
	require.Contains(t, string(data), "Status: retired")

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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	msgBroker := pubsub.NewBroker[message.Event]()
	defer msgBroker.Close()

	session.AttachToBrokers(ctx, nil, nil, msgBroker, nil)

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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mcpBroker := pubsub.NewBroker[events.MCPEvent]()
	defer mcpBroker.Close()

	session.AttachToBrokers(ctx, nil, nil, nil, mcpBroker)

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

	ctx, cancel := context.WithCancel(context.Background())

	// Create brokers
	coordBroker := pubsub.NewBroker[events.CoordinatorEvent]()
	workerBroker := pubsub.NewBroker[events.WorkerEvent]()
	msgBroker := pubsub.NewBroker[message.Event]()
	mcpBroker := pubsub.NewBroker[events.MCPEvent]()

	// Attach to all brokers
	session.AttachToBrokers(ctx, coordBroker, workerBroker, msgBroker, mcpBroker)

	// Give goroutines time to start
	time.Sleep(10 * time.Millisecond)

	// Verify subscribers are attached
	require.Equal(t, 1, coordBroker.SubscriberCount())
	require.Equal(t, 1, workerBroker.SubscriberCount())
	require.Equal(t, 1, msgBroker.SubscriberCount())
	require.Equal(t, 1, mcpBroker.SubscriberCount())

	// Cancel the context
	cancel()

	// Give time for cleanup
	time.Sleep(50 * time.Millisecond)

	// Verify subscribers are cleaned up
	require.Equal(t, 0, coordBroker.SubscriberCount())
	require.Equal(t, 0, workerBroker.SubscriberCount())
	require.Equal(t, 0, msgBroker.SubscriberCount())
	require.Equal(t, 0, mcpBroker.SubscriberCount())

	// Cleanup
	coordBroker.Close()
	workerBroker.Close()
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create brokers with larger buffer for high throughput
	workerBroker := pubsub.NewBrokerWithBuffer[events.WorkerEvent](256)
	defer workerBroker.Close()

	session.AttachToBrokers(ctx, nil, workerBroker, nil, nil)

	// Give goroutine time to start
	time.Sleep(10 * time.Millisecond)

	// Publish 100 events with small delays to simulate normal load (not stress test)
	// This tests that under normal operation (< 100 events/sec) no events are dropped
	const eventCount = 100
	for i := 0; i < eventCount; i++ {
		workerBroker.Publish(pubsub.UpdatedEvent, events.WorkerEvent{
			Type:     events.WorkerOutput,
			WorkerID: "worker-1",
			Output:   fmt.Sprintf("Event %d", i),
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

	// Verify worker output.log has content
	logPath := filepath.Join(sessionDir, "workers", "worker-1", "output.log")
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)

	// Under normal load (with small delays), all events should be captured
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Equal(t, eventCount, len(lines), "Expected all events to be captured under normal load")
}

func TestSession_TokenUsageAggregation(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-token-aggregation"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	coordBroker := pubsub.NewBroker[events.CoordinatorEvent]()
	workerBroker := pubsub.NewBroker[events.WorkerEvent]()
	defer coordBroker.Close()
	defer workerBroker.Close()

	session.AttachToBrokers(ctx, coordBroker, workerBroker, nil, nil)

	// Give goroutines time to start
	time.Sleep(10 * time.Millisecond)

	// Publish multiple token usage events from coordinator and workers
	coordBroker.Publish(pubsub.UpdatedEvent, events.CoordinatorEvent{
		Type: events.CoordinatorTokenUsage,
		Metrics: &metrics.TokenMetrics{
			InputTokens:  100,
			OutputTokens: 50,
			TotalCostUSD: 0.01,
		},
	})

	workerBroker.Publish(pubsub.UpdatedEvent, events.WorkerEvent{
		Type:     events.WorkerTokenUsage,
		WorkerID: "worker-1",
		Metrics: &metrics.TokenMetrics{
			InputTokens:  200,
			OutputTokens: 75,
			TotalCostUSD: 0.02,
		},
	})

	workerBroker.Publish(pubsub.UpdatedEvent, events.WorkerEvent{
		Type:     events.WorkerTokenUsage,
		WorkerID: "worker-2",
		Metrics: &metrics.TokenMetrics{
			InputTokens:  300,
			OutputTokens: 100,
			TotalCostUSD: 0.03,
		},
	})

	// Give time for events to be processed
	time.Sleep(50 * time.Millisecond)

	// Close session to flush and update metadata
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify aggregated token usage
	meta, err := Load(sessionDir)
	require.NoError(t, err)
	require.Equal(t, 600, meta.TokenUsage.TotalInputTokens)  // 100 + 200 + 300
	require.Equal(t, 225, meta.TokenUsage.TotalOutputTokens) // 50 + 75 + 100
	require.InDelta(t, 0.06, meta.TokenUsage.TotalCostUSD, 0.001)
}

func TestSession_WorkerMetadataUpdates(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "test-worker-metadata"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	workerBroker := pubsub.NewBroker[events.WorkerEvent]()
	defer workerBroker.Close()

	session.AttachToBrokers(ctx, nil, workerBroker, nil, nil)

	// Give goroutine time to start
	time.Sleep(10 * time.Millisecond)

	// Spawn multiple workers
	workerBroker.Publish(pubsub.UpdatedEvent, events.WorkerEvent{
		Type:     events.WorkerSpawned,
		WorkerID: "worker-1",
	})
	workerBroker.Publish(pubsub.UpdatedEvent, events.WorkerEvent{
		Type:     events.WorkerSpawned,
		WorkerID: "worker-2",
	})
	workerBroker.Publish(pubsub.UpdatedEvent, events.WorkerEvent{
		Type:     events.WorkerSpawned,
		WorkerID: "worker-3",
	})

	// Status changes
	workerBroker.Publish(pubsub.UpdatedEvent, events.WorkerEvent{
		Type:     events.WorkerStatusChange,
		WorkerID: "worker-1",
		Status:   events.WorkerWorking,
		Phase:    events.PhaseImplementing,
	})
	workerBroker.Publish(pubsub.UpdatedEvent, events.WorkerEvent{
		Type:     events.WorkerStatusChange,
		WorkerID: "worker-2",
		Status:   events.WorkerWorking,
		Phase:    events.PhaseReviewing,
	})

	// Retire one worker
	workerBroker.Publish(pubsub.UpdatedEvent, events.WorkerEvent{
		Type:     events.WorkerStatusChange,
		WorkerID: "worker-1",
		Status:   events.WorkerRetired,
		Phase:    events.PhaseIdle,
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
	// This test verifies that the coordinator broker can be attached separately
	// after the session is created, simulating the real flow where:
	// 1. Session is created in createWorkspace()
	// 2. Pool and message brokers are attached immediately
	// 3. Coordinator broker is attached later after spawnCoordinator()

	baseDir := t.TempDir()
	sessionID := "test-late-coord-attach"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Step 1: First attach pool and message brokers (with nil for coordinator)
	workerBroker := pubsub.NewBroker[events.WorkerEvent]()
	msgBroker := pubsub.NewBroker[message.Event]()
	defer workerBroker.Close()
	defer msgBroker.Close()

	session.AttachToBrokers(ctx, nil, workerBroker, msgBroker, nil)

	// Give goroutines time to start
	time.Sleep(10 * time.Millisecond)

	// Verify pool and message brokers are attached
	require.Equal(t, 1, workerBroker.SubscriberCount())
	require.Equal(t, 1, msgBroker.SubscriberCount())

	// Publish some worker events before coordinator is attached
	workerBroker.Publish(pubsub.UpdatedEvent, events.WorkerEvent{
		Type:     events.WorkerSpawned,
		WorkerID: "worker-1",
	})

	// Step 2: Later, attach coordinator broker (simulates after spawnCoordinator)
	coordBroker := pubsub.NewBroker[events.CoordinatorEvent]()
	defer coordBroker.Close()

	session.AttachCoordinatorBroker(ctx, coordBroker)

	// Give goroutine time to start
	time.Sleep(10 * time.Millisecond)

	// Verify coordinator broker is now attached
	require.Equal(t, 1, coordBroker.SubscriberCount())

	// Publish coordinator events after attachment
	coordBroker.Publish(pubsub.UpdatedEvent, events.CoordinatorEvent{
		Type:    events.CoordinatorChat,
		Role:    "coordinator",
		Content: "First coordinator message",
	})

	// Give time for events to be processed
	time.Sleep(50 * time.Millisecond)

	// Close session to flush buffers
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify worker events were captured (from before coordinator attached)
	worker1Log := filepath.Join(sessionDir, "workers", "worker-1", "output.log")
	workerData, err := os.ReadFile(worker1Log)
	require.NoError(t, err)
	require.Contains(t, string(workerData), "Worker spawned")

	// Verify coordinator events were captured (from after attachment)
	coordLog := filepath.Join(sessionDir, "coordinator", "output.log")
	coordData, err := os.ReadFile(coordLog)
	require.NoError(t, err)
	require.Contains(t, string(coordData), "[coordinator]")
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Step 1: First attach pool and message brokers (with nil for MCP)
	workerBroker := pubsub.NewBroker[events.WorkerEvent]()
	msgBroker := pubsub.NewBroker[message.Event]()
	defer workerBroker.Close()
	defer msgBroker.Close()

	session.AttachToBrokers(ctx, nil, workerBroker, msgBroker, nil)

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
	// Integration test: Events from all 4 brokers captured in session files
	// This simulates the full initialization flow with staggered broker attachment

	baseDir := t.TempDir()
	sessionID := "test-all-four-brokers"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create all brokers
	coordBroker := pubsub.NewBroker[events.CoordinatorEvent]()
	workerBroker := pubsub.NewBroker[events.WorkerEvent]()
	msgBroker := pubsub.NewBroker[message.Event]()
	mcpBroker := pubsub.NewBroker[events.MCPEvent]()
	defer coordBroker.Close()
	defer workerBroker.Close()
	defer msgBroker.Close()
	defer mcpBroker.Close()

	// Phase 1: Attach pool and message brokers first (simulating createWorkspace)
	session.AttachToBrokers(ctx, nil, workerBroker, msgBroker, nil)
	time.Sleep(10 * time.Millisecond)

	// Phase 2: Attach MCP broker (simulating after MCP server creation)
	session.AttachMCPBroker(ctx, mcpBroker)
	time.Sleep(10 * time.Millisecond)

	// Phase 3: Attach coordinator broker (simulating after spawnCoordinator)
	session.AttachCoordinatorBroker(ctx, coordBroker)
	time.Sleep(10 * time.Millisecond)

	// Verify all brokers are attached
	require.Equal(t, 1, coordBroker.SubscriberCount(), "coordinator broker should have 1 subscriber")
	require.Equal(t, 1, workerBroker.SubscriberCount(), "worker broker should have 1 subscriber")
	require.Equal(t, 1, msgBroker.SubscriberCount(), "message broker should have 1 subscriber")
	require.Equal(t, 1, mcpBroker.SubscriberCount(), "MCP broker should have 1 subscriber")

	// Publish events from all 4 brokers
	timestamp := time.Now()

	// Coordinator event
	coordBroker.Publish(pubsub.UpdatedEvent, events.CoordinatorEvent{
		Type:    events.CoordinatorChat,
		Role:    "coordinator",
		Content: "Orchestration started",
	})

	// Worker event
	workerBroker.Publish(pubsub.UpdatedEvent, events.WorkerEvent{
		Type:     events.WorkerSpawned,
		WorkerID: "worker-1",
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

	// 1. Coordinator log
	coordLog := filepath.Join(sessionDir, "coordinator", "output.log")
	coordData, err := os.ReadFile(coordLog)
	require.NoError(t, err)
	require.Contains(t, string(coordData), "Orchestration started")

	// 2. Worker log
	workerLog := filepath.Join(sessionDir, "workers", "worker-1", "output.log")
	workerData, err := os.ReadFile(workerLog)
	require.NoError(t, err)
	require.Contains(t, string(workerData), "Worker spawned")

	// 3. Messages log
	messagesPath := filepath.Join(sessionDir, "messages.jsonl")
	messagesData, err := os.ReadFile(messagesPath)
	require.NoError(t, err)
	require.Contains(t, string(messagesData), "Starting work")

	// 4. MCP log
	mcpPath := filepath.Join(sessionDir, "mcp_requests.jsonl")
	mcpData, err := os.ReadFile(mcpPath)
	require.NoError(t, err)
	require.Contains(t, string(mcpData), "test_tool")
}
