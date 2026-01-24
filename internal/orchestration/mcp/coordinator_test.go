package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/orchestration/v2/adapter"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/processor"
	"github.com/zjrosen/perles/internal/orchestration/v2/prompt"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
)

// ptr returns a pointer to the given ProcessPhase value.
func ptr(p events.ProcessPhase) *events.ProcessPhase {
	return &p
}

// messageLogResponse is used for test JSON unmarshaling of read_message_log responses.
type messageLogResponse struct {
	TotalCount    int               `json:"total_count"`
	ReturnedCount int               `json:"returned_count"`
	Messages      []messageLogEntry `json:"messages"`
}

// messageLogEntry is a single message in the log response.
type messageLogEntry struct {
	Timestamp string `json:"timestamp"`
	From      string `json:"from"`
	To        string `json:"to"`
	Content   string `json:"content"`
}

// newCoordinatorServerWithV2 creates a CoordinatorServer with a properly configured v2Adapter for testing.
func newCoordinatorServerWithV2(t *testing.T, msgRepo repository.MessageRepository) *CoordinatorServer {
	t.Helper()
	cs := NewCoordinatorServer(msgRepo, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))
	proc := processor.NewCommandProcessor()
	v2Adapter := adapter.NewV2Adapter(proc, adapter.WithMessageRepository(msgRepo))
	cs.SetV2Adapter(v2Adapter)
	return cs
}

// workerStateInfo is used for test JSON unmarshaling of query_worker_state responses.
type workerStateInfo struct {
	WorkerID     string `json:"worker_id"`
	Status       string `json:"status"`
	Phase        string `json:"phase"`
	TaskID       string `json:"task_id,omitempty"`
	ContextUsage string `json:"context_usage,omitempty"`
	StartedAt    string `json:"started_at"`
}

// workerStateResponse is used for test JSON unmarshaling of query_worker_state responses.
type workerStateResponse struct {
	Workers      []workerStateInfo `json:"workers"`
	ReadyWorkers []string          `json:"ready_workers"`
}

// TestNewCoordinatorServer_ProvidedBeadsExecutorIsUsed verifies mock injection works.
func TestNewCoordinatorServer_ProvidedBeadsExecutorIsUsed(t *testing.T) {
	mockExec := mocks.NewMockIssueExecutor(t)

	cs := NewCoordinatorServer(nil, "/tmp/test", 8765, mockExec)

	// beadsExecutor should be the mock we provided
	require.NotNil(t, cs.beadsExecutor, "beadsExecutor should not be nil")
	require.Equal(t, mockExec, cs.beadsExecutor, "beadsExecutor should be the provided mock")
}

// TestCoordinatorServer_RegistersAllTools verifies all coordinator tools are registered.
func TestCoordinatorServer_RegistersAllTools(t *testing.T) {
	cs := NewCoordinatorServer(nil, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))

	expectedTools := []string{
		"spawn_worker",
		"assign_task",
		"replace_worker",
		"retire_worker",
		"send_to_worker",
		"post_message",
		"get_task_status",
		"mark_task_complete",
		"mark_task_failed",
		"read_message_log",
		"prepare_handoff",
		"query_worker_state",
		"assign_task_review",
		"assign_review_feedback",
		"approve_commit",
		"stop_worker",
		"generate_accountability_summary",
		"signal_workflow_complete",
		"notify_user",
	}

	for _, toolName := range expectedTools {
		_, ok := cs.tools[toolName]
		require.True(t, ok, "Tool %q not registered", toolName)
		_, ok = cs.handlers[toolName]
		require.True(t, ok, "Handler for %q not registered", toolName)
	}

	require.Equal(t, len(expectedTools), len(cs.tools), "Tool count mismatch")
}

// TestCoordinatorServer_ToolSchemas verifies tool schemas are valid.
func TestCoordinatorServer_ToolSchemas(t *testing.T) {
	cs := NewCoordinatorServer(nil, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))

	for name, tool := range cs.tools {
		t.Run(name, func(t *testing.T) {
			require.NotEmpty(t, tool.Name, "Tool name is empty")
			require.NotEmpty(t, tool.Description, "Tool description is empty")
			require.NotNil(t, tool.InputSchema, "Tool inputSchema is nil")
			if tool.InputSchema != nil {
				require.Equal(t, "object", tool.InputSchema.Type, "InputSchema.Type mismatch")
			}
		})
	}
}

// TestCoordinatorServer_SpawnWorker tests spawn_worker (takes no args).
// Note: With v2 routing, spawn_worker routes to v2 adapter.
func TestCoordinatorServer_SpawnWorker(t *testing.T) {
	cs := NewCoordinatorServer(nil, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))

	// Inject v2 adapter for test
	v2handler, cleanup := injectV2AdapterToCoordinator(t, cs)
	defer cleanup()

	// Configure v2 handler to return success
	v2handler.SetResult(&command.CommandResult{
		Success: true,
		Data:    "Worker spawned successfully",
	})

	handler := cs.handlers["spawn_worker"]

	// spawn_worker takes no args - v2 adapter returns success
	result, err := handler(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, result.Content[0].Text, "spawned")
}

// TestCoordinatorServer_AssignTaskValidation tests input validation for assign_task.
func TestCoordinatorServer_AssignTaskValidation(t *testing.T) {
	cs := NewCoordinatorServer(nil, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))
	handler := cs.handlers["assign_task"]

	tests := []struct {
		name    string
		args    string
		wantErr bool
	}{
		{
			name:    "missing worker_id",
			args:    `{"task_id": "perles-abc"}`,
			wantErr: true,
		},
		{
			name:    "missing task_id",
			args:    `{"worker_id": "worker-1"}`,
			wantErr: true,
		},
		{
			name:    "empty worker_id",
			args:    `{"worker_id": "", "task_id": "perles-abc"}`,
			wantErr: true,
		},
		{
			name:    "empty task_id",
			args:    `{"worker_id": "worker-1", "task_id": ""}`,
			wantErr: true,
		},
		{
			name:    "invalid json",
			args:    `not json`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := handler(context.Background(), json.RawMessage(tt.args))
			require.Equal(t, tt.wantErr, err != nil, "error = %v, wantErr = %v", err, tt.wantErr)
		})
	}
}

// TestCoordinatorServer_ReplaceWorkerValidation tests input validation for replace_worker.
// Note: With v2 routing, validation happens in v2 adapter.
func TestCoordinatorServer_ReplaceWorkerValidation(t *testing.T) {
	cs := NewCoordinatorServer(nil, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))

	// Inject v2 adapter for test
	_, cleanup := injectV2AdapterToCoordinator(t, cs)
	defer cleanup()

	handler := cs.handlers["replace_worker"]

	// Test only input validation errors (missing/empty fields)
	// Business logic errors (worker not found) are handled in v2 handler tests
	tests := []struct {
		name    string
		args    string
		wantErr bool
	}{
		{
			name:    "missing worker_id",
			args:    `{}`,
			wantErr: true,
		},
		{
			name:    "empty worker_id",
			args:    `{"worker_id": ""}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := handler(context.Background(), json.RawMessage(tt.args))
			require.Equal(t, tt.wantErr, err != nil, "error = %v, wantErr = %v", err, tt.wantErr)
		})
	}
}

// TestCoordinatorServer_SendToWorkerValidation tests input validation for send_to_worker.
// Note: With v2 routing, validation happens in v2 adapter.
func TestCoordinatorServer_SendToWorkerValidation(t *testing.T) {
	cs := NewCoordinatorServer(nil, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))

	// Inject v2 adapter for test
	_, cleanup := injectV2AdapterToCoordinator(t, cs)
	defer cleanup()

	handler := cs.handlers["send_to_worker"]

	// Test only input validation errors (missing/empty fields)
	// Business logic errors (worker not found) are handled in v2 handler tests
	tests := []struct {
		name    string
		args    string
		wantErr bool
	}{
		{
			name:    "missing worker_id",
			args:    `{"message": "hello"}`,
			wantErr: true,
		},
		{
			name:    "missing message",
			args:    `{"worker_id": "worker-1"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := handler(context.Background(), json.RawMessage(tt.args))
			require.Equal(t, tt.wantErr, err != nil, "error = %v, wantErr = %v", err, tt.wantErr)
		})
	}
}

// TestCoordinatorServer_PostMessageValidation tests input validation for post_message.
// Note: With v2 routing, validation happens in v2 adapter.
func TestCoordinatorServer_PostMessageValidation(t *testing.T) {
	// No message issue available
	cs := NewCoordinatorServer(nil, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))

	// Inject v2 adapter for test
	_, cleanup := injectV2AdapterToCoordinator(t, cs)
	defer cleanup()

	handler := cs.handlers["post_message"]

	// Test only input validation errors (missing/empty fields)
	// Business logic errors (message issue not available) are handled in v2 handler tests
	tests := []struct {
		name    string
		args    string
		wantErr bool
	}{
		{
			name:    "missing to",
			args:    `{"content": "hello"}`,
			wantErr: true,
		},
		{
			name:    "missing content",
			args:    `{"to": "ALL"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := handler(context.Background(), json.RawMessage(tt.args))
			require.Equal(t, tt.wantErr, err != nil, "error = %v, wantErr = %v", err, tt.wantErr)
		})
	}
}

// TestCoordinatorServer_GetTaskStatusValidation tests input validation for get_task_status.
func TestCoordinatorServer_GetTaskStatusValidation(t *testing.T) {
	cs := NewCoordinatorServer(nil, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))
	handler := cs.handlers["get_task_status"]

	tests := []struct {
		name    string
		args    string
		wantErr bool
	}{
		{
			name:    "missing task_id",
			args:    `{}`,
			wantErr: true,
		},
		{
			name:    "empty task_id",
			args:    `{"task_id": ""}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := handler(context.Background(), json.RawMessage(tt.args))
			require.Equal(t, tt.wantErr, err != nil, "error = %v, wantErr = %v", err, tt.wantErr)
		})
	}
}

// TestCoordinatorServer_MarkTaskCompleteValidation tests input validation for mark_task_complete.
func TestCoordinatorServer_MarkTaskCompleteValidation(t *testing.T) {
	cs := NewCoordinatorServer(nil, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))
	handler := cs.handlers["mark_task_complete"]

	tests := []struct {
		name    string
		args    string
		wantErr bool
	}{
		{
			name:    "missing task_id",
			args:    `{}`,
			wantErr: true,
		},
		{
			name:    "empty task_id",
			args:    `{"task_id": ""}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := handler(context.Background(), json.RawMessage(tt.args))
			require.Equal(t, tt.wantErr, err != nil, "error = %v, wantErr = %v", err, tt.wantErr)
		})
	}
}

// TestCoordinatorServer_MarkTaskFailedValidation tests input validation for mark_task_failed.
func TestCoordinatorServer_MarkTaskFailedValidation(t *testing.T) {
	cs := NewCoordinatorServer(nil, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))
	handler := cs.handlers["mark_task_failed"]

	tests := []struct {
		name    string
		args    string
		wantErr bool
	}{
		{
			name:    "missing task_id",
			args:    `{"reason": "blocked"}`,
			wantErr: true,
		},
		{
			name:    "missing reason",
			args:    `{"task_id": "perles-abc"}`,
			wantErr: true,
		},
		{
			name:    "empty task_id",
			args:    `{"task_id": "", "reason": "blocked"}`,
			wantErr: true,
		},
		{
			name:    "empty reason",
			args:    `{"task_id": "perles-abc", "reason": ""}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := handler(context.Background(), json.RawMessage(tt.args))
			require.Equal(t, tt.wantErr, err != nil, "error = %v, wantErr = %v", err, tt.wantErr)
		})
	}
}

// TestCoordinatorServer_ReadMessageLogNoIssue tests read_message_log when no issue is available.
func TestCoordinatorServer_ReadMessageLogNoIssue(t *testing.T) {
	cs := NewCoordinatorServer(nil, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))
	handler := cs.handlers["read_message_log"]

	_, err := handler(context.Background(), json.RawMessage(`{}`))
	require.Error(t, err, "Expected error when message issue is nil")
}

// TestCoordinatorServer_GetMessageRepository tests the message repository accessor.
func TestCoordinatorServer_GetMessageRepository(t *testing.T) {
	cs := NewCoordinatorServer(nil, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))

	require.Nil(t, cs.msgRepo, "msgRepo should return nil when no repository is set")
}

// TestCoordinatorServer_Instructions tests that instructions are set correctly.
func TestCoordinatorServer_Instructions(t *testing.T) {
	cs := NewCoordinatorServer(nil, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))

	require.NotEmpty(t, cs.instructions, "Instructions should be set")
	require.Equal(t, "perles-orchestrator", cs.info.Name, "Server name mismatch")
	require.Equal(t, "1.0.0", cs.info.Version, "Server version mismatch")
}

// TestIsValidTaskID tests task ID validation.
func TestIsValidTaskID(t *testing.T) {
	tests := []struct {
		name   string
		taskID string
		want   bool
	}{
		// Valid formats
		{"simple task", "perles-abc", true},
		{"4 char suffix", "perles-abcd", true},
		{"mixed case prefix", "Perles-abc", false}, // regex only allows lowercase
		{"numeric suffix", "perles-1234", true},
		{"alphanumeric suffix", "perles-a1b2", true},
		{"subtask", "perles-abc.1", true},
		{"subtask multi-digit", "perles-abc.123", true},
		{"long suffix", "perles-abcdefghij", true},
		{"short prefix", "ms-abc", true},

		// Invalid formats
		{"empty", "", false},
		{"no prefix", "-abc", false},
		{"no suffix", "perles-", false},
		{"single char suffix", "perles-a", false},
		{"too long suffix", "perles-abcdefghijk", true}, // no max length in regex
		{"spaces", "perles abc", false},
		{"shell injection attempt", "perles-abc; rm -rf /", false},
		{"path traversal", "../etc/passwd", false},
		{"flag injection", "--help", false},
		{"newline", "perles-abc\n", false},
		{"special chars", "perles-abc$FOO", false},
		{"underscore in suffix", "perles-abc_def", false},
		{"double dot subtask", "perles-abc..1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidTaskID(tt.taskID)
			require.Equal(t, tt.want, got, "IsValidTaskID(%q) = %v, want %v", tt.taskID, got, tt.want)
		})
	}
}

// TestCoordinatorServer_AssignTaskRouting tests assign_task routes to v2 adapter.
// Note: Task ID format validation is now in v2 handler, not coordinator.
// Security validation tests should be in v2 handler tests.
func TestCoordinatorServer_AssignTaskRouting(t *testing.T) {
	cs := NewCoordinatorServer(nil, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))

	// Inject v2 adapter for test
	v2handler, cleanup := injectV2AdapterToCoordinator(t, cs)
	defer cleanup()

	// Configure v2 handler to return success
	v2handler.SetResult(&command.CommandResult{
		Success: true,
		Data:    "Task assigned",
	})

	handler := cs.handlers["assign_task"]

	// Verify handler routes to v2 adapter
	args := `{"worker_id": "worker-1", "task_id": "perles-abc.1"}`
	result, err := handler(context.Background(), json.RawMessage(args))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, result.Content[0].Text, "assigned")

	// Verify command was routed to v2
	cmds := v2handler.GetCommands()
	require.Len(t, cmds, 1, "Expected one command")
	require.Equal(t, command.CmdAssignTask, cmds[0].Type())
}

// TestPrepareHandoff_PostsMessage verifies tool posts message with correct type and content.
func TestPrepareHandoff_PostsMessage(t *testing.T) {
	msgIssue := repository.NewMemoryMessageRepository()
	cs := NewCoordinatorServer(msgIssue, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))
	handler := cs.handlers["prepare_handoff"]

	summary := "Worker 1 is processing task perles-abc. Task is 50% complete."
	args := `{"summary": "` + summary + `"}`

	result, err := handler(context.Background(), json.RawMessage(args))
	require.NoError(t, err, "Unexpected error")

	require.Equal(t, "Handoff message posted. Refresh will proceed.", result.Content[0].Text, "Unexpected result")

	// Verify message was posted to the issue
	entries := msgIssue.Entries()
	require.Len(t, entries, 1, "Expected 1 message")

	entry := entries[0]
	require.Equal(t, message.MessageHandoff, entry.Type, "Message type mismatch")
	require.Equal(t, message.ActorCoordinator, entry.From, "From mismatch")
	require.Equal(t, message.ActorAll, entry.To, "To mismatch")
	expectedContent := "[HANDOFF]\n" + summary
	require.Equal(t, expectedContent, entry.Content, "Content mismatch")
}

// TestPrepareHandoff_EmptySummary verifies error returned when summary is empty.
func TestPrepareHandoff_EmptySummary(t *testing.T) {
	msgIssue := repository.NewMemoryMessageRepository()
	cs := NewCoordinatorServer(msgIssue, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))
	handler := cs.handlers["prepare_handoff"]

	tests := []struct {
		name string
		args string
	}{
		{
			name: "empty string summary",
			args: `{"summary": ""}`,
		},
		{
			name: "missing summary",
			args: `{}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := handler(context.Background(), json.RawMessage(tt.args))
			require.Error(t, err, "Expected error for empty summary")
		})
	}
}

// TestPrepareHandoff_NoMessageIssue verifies error when message issue is nil.
func TestPrepareHandoff_NoMessageIssue(t *testing.T) {
	// No message issue
	cs := NewCoordinatorServer(nil, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))
	handler := cs.handlers["prepare_handoff"]

	args := `{"summary": "Test summary"}`
	_, err := handler(context.Background(), json.RawMessage(args))
	require.Error(t, err, "Expected error when message issue is nil")
}

// TestQueryWorkerState_NoWorkers verifies query_worker_state returns empty when no workers exist.
// This test uses the v2 adapter since handleQueryWorkerState delegates to it.
func TestQueryWorkerState_NoWorkers(t *testing.T) {
	// Use NewTestCoordinatorServer which includes v2 adapter with repositories
	tcs := NewTestCoordinatorServer(t)
	defer tcs.Close()

	handler := tcs.handlers["query_worker_state"]

	result, err := handler(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err, "Unexpected error")

	// Parse response
	var response workerStateResponse
	err = json.Unmarshal([]byte(result.Content[0].Text), &response)
	require.NoError(t, err, "Failed to parse response")

	require.Empty(t, response.Workers, "Expected 0 workers")
	require.Empty(t, response.ReadyWorkers, "Expected 0 ready workers")
}

// TestQueryWorkerState_WithWorkerAndPhase verifies query_worker_state returns worker with phase.
// This test uses the v2 adapter since handleQueryWorkerState delegates to it.
func TestQueryWorkerState_WithWorkerAndPhase(t *testing.T) {
	// Use NewTestCoordinatorServer which includes v2 adapter with repositories
	tcs := NewTestCoordinatorServer(t)
	defer tcs.Close()

	// Add a worker to the v2 repository
	_ = tcs.ProcessRepo.Save(&repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusWorking,
		Phase:  ptr(events.ProcessPhaseImplementing),
		TaskID: "perles-abc.1",
	})

	handler := tcs.handlers["query_worker_state"]
	result, err := handler(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err, "Unexpected error")

	// Parse response
	var response workerStateResponse
	err = json.Unmarshal([]byte(result.Content[0].Text), &response)
	require.NoError(t, err, "Failed to parse response")

	require.Len(t, response.Workers, 1, "Expected 1 worker")

	worker := response.Workers[0]
	require.Equal(t, "worker-1", worker.WorkerID, "WorkerID mismatch")
	require.Equal(t, "implementing", worker.Phase, "Phase mismatch")
	require.Equal(t, "perles-abc.1", worker.TaskID, "TaskID mismatch")
}

// TestQueryWorkerState_FilterByWorkerID verifies query_worker_state filters by worker_id.
// This test uses the v2 adapter since handleQueryWorkerState delegates to it.
func TestQueryWorkerState_FilterByWorkerID(t *testing.T) {
	// Use NewTestCoordinatorServer which includes v2 adapter with repositories
	tcs := NewTestCoordinatorServer(t)
	defer tcs.Close()

	// Add multiple workers to the v2 repository
	_ = tcs.ProcessRepo.Save(&repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusWorking,
		Phase:  ptr(events.ProcessPhaseImplementing),
	})
	_ = tcs.ProcessRepo.Save(&repository.Process{
		ID:     "worker-2",
		Role:   repository.RoleWorker,
		Status: repository.StatusReady,
		Phase:  ptr(events.ProcessPhaseIdle),
	})

	handler := tcs.handlers["query_worker_state"]
	result, err := handler(context.Background(), json.RawMessage(`{"worker_id": "worker-1"}`))
	require.NoError(t, err, "Unexpected error")

	// Parse response
	var response workerStateResponse
	err = json.Unmarshal([]byte(result.Content[0].Text), &response)
	require.NoError(t, err, "Failed to parse response")

	require.Len(t, response.Workers, 1, "Expected 1 worker (filtered)")
	if len(response.Workers) > 0 {
		require.Equal(t, "worker-1", response.Workers[0].WorkerID, "Expected worker-1")
	}
}

// TestQueryWorkerState_FilterByTaskID verifies query_worker_state filters by task_id.
// This test uses the v2 adapter since handleQueryWorkerState delegates to it.
func TestQueryWorkerState_FilterByTaskID(t *testing.T) {
	// Use NewTestCoordinatorServer which includes v2 adapter with repositories
	tcs := NewTestCoordinatorServer(t)
	defer tcs.Close()

	// Add workers with different tasks to the v2 repository
	_ = tcs.ProcessRepo.Save(&repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusWorking,
		Phase:  ptr(events.ProcessPhaseImplementing),
		TaskID: "perles-abc.1",
	})
	_ = tcs.ProcessRepo.Save(&repository.Process{
		ID:     "worker-2",
		Role:   repository.RoleWorker,
		Status: repository.StatusWorking,
		Phase:  ptr(events.ProcessPhaseImplementing),
		TaskID: "perles-xyz.1",
	})

	handler := tcs.handlers["query_worker_state"]
	result, err := handler(context.Background(), json.RawMessage(`{"task_id": "perles-abc.1"}`))
	require.NoError(t, err, "Unexpected error")

	// Parse response
	var response workerStateResponse
	err = json.Unmarshal([]byte(result.Content[0].Text), &response)
	require.NoError(t, err, "Failed to parse response")

	require.Len(t, response.Workers, 1, "Expected 1 worker (filtered by task)")
	if len(response.Workers) > 0 {
		require.Equal(t, "perles-abc.1", response.Workers[0].TaskID, "Expected task perles-abc.1")
	}
}

// TestQueryWorkerState_ReturnsReadyWorkers verifies ready_workers list is populated.
// This test uses the v2 adapter since handleQueryWorkerState delegates to it.
func TestQueryWorkerState_ReturnsReadyWorkers(t *testing.T) {
	// Use NewTestCoordinatorServer which includes v2 adapter with repositories
	tcs := NewTestCoordinatorServer(t)
	defer tcs.Close()

	// Add a ready worker to the v2 repository
	_ = tcs.ProcessRepo.Save(&repository.Process{
		ID:     "worker-1",
		Role:   repository.RoleWorker,
		Status: repository.StatusReady,
		Phase:  ptr(events.ProcessPhaseIdle),
	})

	handler := tcs.handlers["query_worker_state"]
	result, err := handler(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err, "Unexpected error")

	// Parse response
	var response workerStateResponse
	err = json.Unmarshal([]byte(result.Content[0].Text), &response)
	require.NoError(t, err, "Failed to parse response")

	require.Len(t, response.ReadyWorkers, 1, "Expected 1 ready worker")
	if len(response.ReadyWorkers) > 0 {
		require.Equal(t, "worker-1", response.ReadyWorkers[0], "Expected ready worker worker-1")
	}
}

// TestAssignTaskReview_Routing verifies assign_task_review routes to v2 adapter.
// Note: Self-review rejection is now a v2 handler responsibility, tested in v2 handler tests.
func TestAssignTaskReview_Routing(t *testing.T) {
	cs := NewCoordinatorServer(nil, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))

	// Inject v2 adapter for test
	v2handler, cleanup := injectV2AdapterToCoordinator(t, cs)
	defer cleanup()

	// Configure v2 handler to return success
	v2handler.SetResult(&command.CommandResult{
		Success: true,
		Data:    "Review assigned",
	})

	handler := cs.handlers["assign_task_review"]

	// Verify handler routes to v2 adapter
	args := `{"reviewer_id": "worker-2", "task_id": "perles-abc.1", "implementer_id": "worker-1"}`
	result, err := handler(context.Background(), json.RawMessage(args))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, result.Content[0].Text, "assigned")

	// Verify command was routed to v2
	cmds := v2handler.GetCommands()
	require.Len(t, cmds, 1, "Expected one command")
	require.Equal(t, command.CmdAssignReview, cmds[0].Type())
}

// TestAssignTaskReview_ValidationRequired verifies required field validation.
// Note: Business logic validation (task not awaiting review, self-review, invalid task_id) is now in v2 handler tests.
func TestAssignTaskReview_ValidationRequired(t *testing.T) {
	cs := NewCoordinatorServer(nil, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))

	// Inject v2 adapter for test
	_, cleanup := injectV2AdapterToCoordinator(t, cs)
	defer cleanup()

	handler := cs.handlers["assign_task_review"]

	// Test only input validation errors (missing required fields)
	tests := []struct {
		name string
		args string
	}{
		{"missing reviewer_id", `{"task_id": "perles-abc.1", "implementer_id": "worker-1"}`},
		{"missing task_id", `{"reviewer_id": "worker-2", "implementer_id": "worker-1"}`},
		{"missing implementer_id", `{"reviewer_id": "worker-2", "task_id": "perles-abc.1"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := handler(context.Background(), json.RawMessage(tt.args))
			require.Error(t, err, "Expected error for %s", tt.name)
		})
	}
}

// TestAssignReviewFeedback_ValidationRequired verifies required field validation.
// Note: Business logic tests (task not denied, etc.) are now in v2 handler tests.
func TestAssignReviewFeedback_ValidationRequired(t *testing.T) {
	cs := NewCoordinatorServer(nil, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))

	// Inject v2 adapter for test
	_, cleanup := injectV2AdapterToCoordinator(t, cs)
	defer cleanup()

	handler := cs.handlers["assign_review_feedback"]

	// Test only input validation errors (missing required fields)
	tests := []struct {
		name string
		args string
	}{
		{"missing implementer_id", `{"task_id": "perles-abc.1", "feedback": "fix"}`},
		{"missing task_id", `{"implementer_id": "worker-1", "feedback": "fix"}`},
		{"missing feedback", `{"implementer_id": "worker-1", "task_id": "perles-abc.1"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := handler(context.Background(), json.RawMessage(tt.args))
			require.Error(t, err, "Expected error for %s", tt.name)
		})
	}
}

// TestApproveCommit_ValidationRequired verifies required field validation.
// Note: Business logic tests (task not approved, implementer mismatch) are now in v2 handler tests.
func TestApproveCommit_ValidationRequired(t *testing.T) {
	cs := NewCoordinatorServer(nil, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))

	// Inject v2 adapter for test
	_, cleanup := injectV2AdapterToCoordinator(t, cs)
	defer cleanup()

	handler := cs.handlers["approve_commit"]

	// Test only input validation errors (missing/empty required fields)
	tests := []struct {
		name string
		args string
	}{
		{"missing implementer_id", `{"task_id": "perles-abc.1"}`},
		{"missing task_id", `{"implementer_id": "worker-1"}`},
		{"empty implementer_id", `{"implementer_id": "", "task_id": "perles-abc.1"}`},
		{"empty task_id", `{"implementer_id": "worker-1", "task_id": ""}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := handler(context.Background(), json.RawMessage(tt.args))
			require.Error(t, err, "Expected error for %s", tt.name)
		})
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && (s[:len(substr)] == substr || (len(s) > len(substr) && containsInternal(s, substr))))
}

func containsInternal(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ============================================================================
// Phase 5 Tests: Updated assign_task with state tracking
// Note: Business logic tests for assign_task (validates assignment, rejects duplicate,
// rejects worker busy) are now in v2 handler tests since validation moved to v2.
// ============================================================================

// TestReplaceWorker_Routing verifies replace_worker routes to v2 adapter.
// Note: Business logic tests (cleans up assignments) are now in v2 handler tests.
func TestReplaceWorker_Routing(t *testing.T) {
	cs := NewCoordinatorServer(nil, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))

	// Inject v2 adapter for test
	v2handler, cleanup := injectV2AdapterToCoordinator(t, cs)
	defer cleanup()

	// Configure v2 handler to return success
	v2handler.SetResult(&command.CommandResult{
		Success: true,
		Data:    "Worker replaced successfully",
	})

	handler := cs.handlers["replace_worker"]
	result, err := handler(context.Background(), json.RawMessage(`{"worker_id": "worker-1"}`))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, result.Content[0].Text, "replaced")

	// Verify command was routed to v2
	cmds := v2handler.GetCommands()
	require.Len(t, cmds, 1, "Expected one command")
	require.Equal(t, command.CmdReplaceProcess, cmds[0].Type())
}

// TestTaskAssignmentPrompt_WithSummary verifies TaskAssignmentPrompt includes summary when provided.
func TestTaskAssignmentPrompt_WithSummary(t *testing.T) {
	p := prompt.TaskAssignmentPrompt("perles-abc.1", "Test Task", "Focus on error handling.")

	require.True(t, containsInternal(p, "## Coordinator Instructions"), "Prompt should contain 'Coordinator Instructions:' section when summary provided")
	require.True(t, containsInternal(p, "Focus on error handling."), "Prompt should contain the summary content")
}

// TestTaskAssignmentPrompt_WithoutSummary verifies TaskAssignmentPrompt excludes summary section when empty.
func TestTaskAssignmentPrompt_WithoutSummary(t *testing.T) {
	p := prompt.TaskAssignmentPrompt("perles-abc.1", "Test Task", "")

	require.False(t, containsInternal(p, "## Coordinator Instructions"), "Prompt should NOT contain 'Coordinator Instructions:' section when summary is empty")
}

// TestTaskAssignmentPrompt_AllSections verifies TaskAssignmentPrompt includes all sections when provided.
func TestTaskAssignmentPrompt_AllSections(t *testing.T) {
	p := prompt.TaskAssignmentPrompt(
		"perles-abc.1",
		"Implement Feature X",
		"Important: Check existing patterns in module Y",
	)

	// Verify all sections are present
	sections := []string{
		"[TASK ASSIGNMENT]",
		"**Task ID:** perles-abc.1",
		"**Title:** Implement Feature X",
		"## Coordinator Instructions",
		"Important: Check existing patterns in module Y",
		"report_implementation_complete",
	}

	for _, section := range sections {
		require.True(t, containsInternal(p, section), "Prompt should contain %q", section)
	}
}

// TestCoordinatorServer_AssignTaskSchemaIncludesSummary verifies the tool schema includes summary parameter.
func TestCoordinatorServer_AssignTaskSchemaIncludesSummary(t *testing.T) {
	cs := NewCoordinatorServer(nil, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))

	tool, ok := cs.tools["assign_task"]
	require.True(t, ok, "assign_task tool not registered")

	require.NotNil(t, tool.InputSchema, "assign_task InputSchema is nil")

	summaryProp, ok := tool.InputSchema.Properties["summary"]
	require.True(t, ok, "assign_task schema should include 'summary' property")

	require.Equal(t, "string", summaryProp.Type, "summary property type mismatch")

	require.NotEmpty(t, summaryProp.Description, "summary property should have a description")

	// Verify summary is NOT in required list (it's optional)
	for _, req := range tool.InputSchema.Required {
		require.NotEqual(t, "summary", req, "summary should NOT be in Required list (it's optional)")
	}
}

// TestIntegration_QueryWorkerState verifies query_worker_state returns correct data from v2 repository.
func TestIntegration_QueryWorkerState(t *testing.T) {
	// Use NewTestCoordinatorServer which includes v2 adapter with repositories
	tcs := NewTestCoordinatorServer(t)
	defer tcs.Close()

	// Create a worker in the v2 repository
	_ = tcs.ProcessRepo.Save(&repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     ptr(events.ProcessPhaseImplementing),
		TaskID:    "perles-abc.1",
		SessionID: "session-1",
	})

	// Query worker state - should show worker info from v2 repo
	queryHandler := tcs.handlers["query_worker_state"]
	result, err := queryHandler(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err, "query_worker_state error")

	var stateResponse workerStateResponse
	err = json.Unmarshal([]byte(result.Content[0].Text), &stateResponse)
	require.NoError(t, err, "Failed to parse query_worker_state response")

	require.Len(t, stateResponse.Workers, 1, "Expected 1 worker in state response")
	require.Equal(t, "worker-1", stateResponse.Workers[0].WorkerID, "WorkerID mismatch")
	require.Equal(t, "implementing", stateResponse.Workers[0].Phase, "Phase mismatch")
	require.Equal(t, "perles-abc.1", stateResponse.Workers[0].TaskID, "TaskID mismatch")
}

// ============================================================================
// Unread Message Tracking Tests
// ============================================================================

// TestReadMessageLog_UnreadDefault_Basic tests that sequential read calls return only new messages.
func TestReadMessageLog_UnreadDefault_Basic(t *testing.T) {
	msgIssue := repository.NewMemoryMessageRepository()
	cs := newCoordinatorServerWithV2(t, msgIssue)
	handler := cs.handlers["read_message_log"]

	// Post 3 initial messages
	_, _ = msgIssue.Append("COORDINATOR", "ALL", "Message 1", message.MessageInfo)
	_, _ = msgIssue.Append("WORKER.1", "COORDINATOR", "Message 2", message.MessageInfo)
	_, _ = msgIssue.Append("COORDINATOR", "WORKER.1", "Message 3", message.MessageInfo)

	// First call should return all 3 messages (first call returns all)
	result, err := handler(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)

	var resp messageLogResponse
	err = json.Unmarshal([]byte(result.Content[0].Text), &resp)
	require.NoError(t, err)
	require.Equal(t, 3, resp.TotalCount, "First call should return all 3 messages")
	require.Equal(t, 3, resp.ReturnedCount)

	// Post 2 more messages
	_, _ = msgIssue.Append("WORKER.2", "COORDINATOR", "Message 4", message.MessageInfo)
	_, _ = msgIssue.Append("COORDINATOR", "WORKER.2", "Message 5", message.MessageInfo)

	// Second call should return only 2 new messages
	result, err = handler(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)

	err = json.Unmarshal([]byte(result.Content[0].Text), &resp)
	require.NoError(t, err)
	require.Equal(t, 2, resp.TotalCount, "Second call should return only 2 new messages")
	require.Equal(t, 2, resp.ReturnedCount)

	// Verify the messages are the new ones
	require.Len(t, resp.Messages, 2)
	require.Contains(t, resp.Messages[0].Content, "Message 4")
	require.Contains(t, resp.Messages[1].Content, "Message 5")
}

// TestReadMessageLog_UnreadDefault_FirstCall tests that first call with no prior read state returns all messages.
func TestReadMessageLog_UnreadDefault_FirstCall(t *testing.T) {
	msgIssue := repository.NewMemoryMessageRepository()

	// Post messages before creating coordinator (simulates messages existing before coordinator joins)
	_, _ = msgIssue.Append("WORKER.1", "COORDINATOR", "Hello", message.MessageInfo)
	_, _ = msgIssue.Append("WORKER.2", "COORDINATOR", "World", message.MessageInfo)

	cs := newCoordinatorServerWithV2(t, msgIssue)
	handler := cs.handlers["read_message_log"]

	// First call from coordinator should return all existing messages
	result, err := handler(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)

	var resp messageLogResponse
	err = json.Unmarshal([]byte(result.Content[0].Text), &resp)
	require.NoError(t, err)
	require.Equal(t, 2, resp.TotalCount, "First call should return all 2 messages")
	require.Equal(t, 2, resp.ReturnedCount)
}

// TestReadMessageLog_UnreadDefault_Empty tests read_message_log on empty log.
func TestReadMessageLog_UnreadDefault_Empty(t *testing.T) {
	msgIssue := repository.NewMemoryMessageRepository()
	cs := newCoordinatorServerWithV2(t, msgIssue)
	handler := cs.handlers["read_message_log"]

	// Call on empty log
	result, err := handler(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)

	var resp messageLogResponse
	err = json.Unmarshal([]byte(result.Content[0].Text), &resp)
	require.NoError(t, err)
	require.Equal(t, 0, resp.TotalCount, "Empty log should have total_count: 0")
	require.Equal(t, 0, resp.ReturnedCount)
	require.Empty(t, resp.Messages)
}

// TestReadMessageLog_UnreadDefault_NoNewMessages tests that calling without new messages returns empty.
func TestReadMessageLog_UnreadDefault_NoNewMessages(t *testing.T) {
	msgIssue := repository.NewMemoryMessageRepository()
	cs := newCoordinatorServerWithV2(t, msgIssue)
	handler := cs.handlers["read_message_log"]

	// Post some messages
	_, _ = msgIssue.Append("COORDINATOR", "ALL", "Initial message", message.MessageInfo)

	// First call reads all
	result, err := handler(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)

	var resp messageLogResponse
	err = json.Unmarshal([]byte(result.Content[0].Text), &resp)
	require.NoError(t, err)
	require.Equal(t, 1, resp.TotalCount)

	// Second call without new messages should return empty
	result, err = handler(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)

	err = json.Unmarshal([]byte(result.Content[0].Text), &resp)
	require.NoError(t, err)
	require.Equal(t, 0, resp.TotalCount, "No new messages should return total_count: 0")
	require.Equal(t, 0, resp.ReturnedCount)
	require.Empty(t, resp.Messages)
}

// TestReadMessageLog_ReadAll tests that read_all=true returns all messages and doesn't affect readState.
func TestReadMessageLog_ReadAll(t *testing.T) {
	msgIssue := repository.NewMemoryMessageRepository()
	cs := newCoordinatorServerWithV2(t, msgIssue)
	handler := cs.handlers["read_message_log"]

	// Post 3 messages
	_, _ = msgIssue.Append("COORDINATOR", "ALL", "Message 1", message.MessageInfo)
	_, _ = msgIssue.Append("WORKER.1", "COORDINATOR", "Message 2", message.MessageInfo)
	_, _ = msgIssue.Append("COORDINATOR", "WORKER.1", "Message 3", message.MessageInfo)

	// First default call marks all as read
	result, err := handler(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)

	var resp messageLogResponse
	err = json.Unmarshal([]byte(result.Content[0].Text), &resp)
	require.NoError(t, err)
	require.Equal(t, 3, resp.TotalCount)

	// Second default call should return empty (all marked as read)
	result, err = handler(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)

	err = json.Unmarshal([]byte(result.Content[0].Text), &resp)
	require.NoError(t, err)
	require.Equal(t, 0, resp.TotalCount, "Should have no unread messages")

	// Call with read_all=true should return all messages
	result, err = handler(context.Background(), json.RawMessage(`{"read_all": true}`))
	require.NoError(t, err)

	err = json.Unmarshal([]byte(result.Content[0].Text), &resp)
	require.NoError(t, err)
	require.Equal(t, 3, resp.TotalCount, "read_all=true should return all 3 messages")
	require.Equal(t, 3, resp.ReturnedCount)

	// Verify read_all=true didn't update readState - next default call should still be empty
	result, err = handler(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)

	err = json.Unmarshal([]byte(result.Content[0].Text), &resp)
	require.NoError(t, err)
	require.Equal(t, 0, resp.TotalCount, "readState should be unchanged after read_all=true")
}

// TestCommitApprovalPrompt_IncludesAccountabilityInstructions verifies that the
// CommitApprovalPrompt includes post_accountability_summary instructions for the worker.
func TestCommitApprovalPrompt_IncludesAccountabilityInstructions(t *testing.T) {
	taskID := "perles-abc.1"
	p := prompt.CommitApprovalPrompt(taskID, "")

	require.Contains(t, p, "post_accountability_summary", "Prompt should include post_accountability_summary instruction")
	require.Contains(t, p, "After Committing", "Prompt should instruct to document after committing")
	require.Contains(t, p, "task_id", "Prompt should show task_id parameter")
	require.Contains(t, p, "summary", "Prompt should show summary parameter")
	require.Contains(t, p, "commits", "Prompt should show commits parameter")
	require.Contains(t, p, "verification_points", "Prompt should show verification_points parameter")
}

// TestCommitApprovalPrompt_TaskIDInterpolated verifies that the task ID is interpolated
// into the post_accountability_summary example in the prompt.
func TestCommitApprovalPrompt_TaskIDInterpolated(t *testing.T) {
	taskID := "perles-xyz.42"
	p := prompt.CommitApprovalPrompt(taskID, "")

	// The task ID should appear twice - once in the approval message, once in the example
	occurrences := strings.Count(p, taskID)
	require.GreaterOrEqual(t, occurrences, 2, "Task ID should appear at least twice (in message and example)")

	// Verify it appears in the post_accountability_summary example format
	require.Contains(t, p, `task_id="`+taskID+`"`, "Task ID should be in the post_accountability_summary example")
}

// TestCommitApprovalPrompt_WithCommitMessage verifies commit message is included when provided.
func TestCommitApprovalPrompt_WithCommitMessage(t *testing.T) {
	taskID := "perles-test.1"
	commitMsg := "feat(orchestration): add reflection support"
	p := prompt.CommitApprovalPrompt(taskID, commitMsg)

	require.Contains(t, p, commitMsg, "Prompt should include the suggested commit message")
	require.Contains(t, p, "Suggested commit message", "Prompt should have commit message section")
}

// ============================================================================
// Stop Worker MCP Tool Tests
// ============================================================================

// TestCoordinatorMCP_StopWorkerTool_Registered verifies stop_worker tool is registered.
func TestCoordinatorMCP_StopWorkerTool_Registered(t *testing.T) {
	cs := NewCoordinatorServer(nil, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))

	// Verify tool is registered
	tool, ok := cs.tools["stop_worker"]
	require.True(t, ok, "stop_worker tool should be registered")
	require.NotNil(t, tool, "stop_worker tool should not be nil")

	// Verify tool schema
	require.Equal(t, "stop_worker", tool.Name)
	require.NotEmpty(t, tool.Description)
	require.NotNil(t, tool.InputSchema)

	// Verify required properties exist in schema
	require.NotNil(t, tool.InputSchema.Properties["worker_id"], "worker_id property should exist")
	require.NotNil(t, tool.InputSchema.Properties["force"], "force property should exist")
	require.NotNil(t, tool.InputSchema.Properties["reason"], "reason property should exist")

	// Verify worker_id is required
	require.Contains(t, tool.InputSchema.Required, "worker_id", "worker_id should be required")

	// Verify handler is registered
	_, ok = cs.handlers["stop_worker"]
	require.True(t, ok, "Handler for stop_worker should be registered")
}

// TestCoordinatorMCP_StopWorkerTool_CallsAdapter verifies stop_worker calls adapter.HandleStopProcess.
func TestCoordinatorMCP_StopWorkerTool_CallsAdapter(t *testing.T) {
	cs := NewCoordinatorServer(nil, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))

	// Inject v2 adapter for test
	v2handler, cleanup := injectV2AdapterToCoordinator(t, cs)
	defer cleanup()

	// Configure v2 handler to return success for stop worker command
	v2handler.SetResult(&command.CommandResult{
		Success: true,
		Data:    "Worker stopped",
	})

	handler := cs.handlers["stop_worker"]

	// Call with valid arguments
	args := `{"worker_id": "worker-1", "force": true, "reason": "testing"}`
	result, err := handler(context.Background(), json.RawMessage(args))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, result.Content[0].Text, "Worker stop command submitted")

	// Wait for async command to be processed (HandleStopProcess uses Submit which is fire-and-forget)
	time.Sleep(50 * time.Millisecond)

	// Verify command was routed to v2 (StopProcessCommand gets submitted to processor)
	cmds := v2handler.GetCommands()
	require.Len(t, cmds, 1, "Expected one command")
	require.Equal(t, command.CmdStopProcess, cmds[0].Type())
}

// TestCoordinatorMCP_StopWorkerTool_RequiresWorkerID verifies validation error without worker_id.
func TestCoordinatorMCP_StopWorkerTool_RequiresWorkerID(t *testing.T) {
	cs := NewCoordinatorServer(nil, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))

	// Inject v2 adapter for test (required even for validation tests)
	_, cleanup := injectV2AdapterToCoordinator(t, cs)
	defer cleanup()

	handler := cs.handlers["stop_worker"]

	tests := []struct {
		name    string
		args    string
		wantErr bool
	}{
		{
			name:    "missing worker_id",
			args:    `{}`,
			wantErr: true,
		},
		{
			name:    "empty worker_id",
			args:    `{"worker_id": ""}`,
			wantErr: true,
		},
		{
			name:    "only force provided",
			args:    `{"force": true}`,
			wantErr: true,
		},
		{
			name:    "only reason provided",
			args:    `{"reason": "test"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := handler(context.Background(), json.RawMessage(tt.args))
			require.Equal(t, tt.wantErr, err != nil, "error = %v, wantErr = %v", err, tt.wantErr)
			if tt.wantErr {
				require.Contains(t, err.Error(), "worker_id is required")
			}
		})
	}
}

// TestCoordinatorMCP_SpawnWorkerSchema_IncludesAgentType verifies spawn_worker schema has agent_type property.
func TestCoordinatorMCP_SpawnWorkerSchema_IncludesAgentType(t *testing.T) {
	cs := NewCoordinatorServer(nil, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))

	tool, ok := cs.tools["spawn_worker"]
	require.True(t, ok, "spawn_worker tool not registered")

	// Verify tool schema
	require.Equal(t, "spawn_worker", tool.Name)
	require.NotEmpty(t, tool.Description)
	require.NotNil(t, tool.InputSchema)

	// Verify agent_type property exists
	agentTypeProp, ok := tool.InputSchema.Properties["agent_type"]
	require.True(t, ok, "spawn_worker schema should include 'agent_type' property")

	// Verify property definition
	require.Equal(t, "string", agentTypeProp.Type, "agent_type property type mismatch")
	require.NotEmpty(t, agentTypeProp.Description, "agent_type property should have a description")
	require.Contains(t, agentTypeProp.Description, "implementer", "description should mention implementer")
	require.Contains(t, agentTypeProp.Description, "reviewer", "description should mention reviewer")
	require.Contains(t, agentTypeProp.Description, "researcher", "description should mention researcher")

	// Verify enum values
	require.NotNil(t, agentTypeProp.Enum, "agent_type property should have Enum values")
	require.Len(t, agentTypeProp.Enum, 3, "agent_type should have 3 enum values")
	require.Contains(t, agentTypeProp.Enum, "implementer", "enum should include 'implementer'")
	require.Contains(t, agentTypeProp.Enum, "reviewer", "enum should include 'reviewer'")
	require.Contains(t, agentTypeProp.Enum, "researcher", "enum should include 'researcher'")

	// Verify agent_type is NOT in required list (it's optional)
	for _, req := range tool.InputSchema.Required {
		require.NotEqual(t, "agent_type", req, "agent_type should NOT be in Required list (it's optional)")
	}
}

// ============================================================================
// Signal Workflow Complete MCP Tool Tests
// ============================================================================

// TestSignalWorkflowComplete_ToolRegistered verifies signal_workflow_complete tool is registered with correct name.
func TestSignalWorkflowComplete_ToolRegistered(t *testing.T) {
	cs := NewCoordinatorServer(nil, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))

	// Verify tool is registered
	tool, ok := cs.tools["signal_workflow_complete"]
	require.True(t, ok, "signal_workflow_complete tool should be registered")
	require.NotNil(t, tool, "signal_workflow_complete tool should not be nil")
	require.Equal(t, "signal_workflow_complete", tool.Name)

	// Verify handler is registered
	_, ok = cs.handlers["signal_workflow_complete"]
	require.True(t, ok, "Handler for signal_workflow_complete should be registered")
}

// TestSignalWorkflowComplete_SchemaHasRequiredFields verifies input schema has correct required/optional fields.
func TestSignalWorkflowComplete_SchemaHasRequiredFields(t *testing.T) {
	cs := NewCoordinatorServer(nil, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))

	tool, ok := cs.tools["signal_workflow_complete"]
	require.True(t, ok, "signal_workflow_complete tool not registered")

	// Verify input schema structure
	require.NotNil(t, tool.InputSchema, "InputSchema should not be nil")
	require.Equal(t, "object", tool.InputSchema.Type, "InputSchema type should be object")

	// Verify required fields
	require.Contains(t, tool.InputSchema.Required, "status", "status should be required")
	require.Contains(t, tool.InputSchema.Required, "summary", "summary should be required")
	require.Len(t, tool.InputSchema.Required, 2, "should have exactly 2 required fields")

	// Verify status property
	statusProp, ok := tool.InputSchema.Properties["status"]
	require.True(t, ok, "status property should exist")
	require.Equal(t, "string", statusProp.Type, "status property type should be string")
	require.NotNil(t, statusProp.Enum, "status property should have enum values")
	require.Contains(t, statusProp.Enum, "success", "status enum should include 'success'")
	require.Contains(t, statusProp.Enum, "partial", "status enum should include 'partial'")
	require.Contains(t, statusProp.Enum, "aborted", "status enum should include 'aborted'")
	require.Len(t, statusProp.Enum, 3, "status should have exactly 3 enum values")

	// Verify summary property
	summaryProp, ok := tool.InputSchema.Properties["summary"]
	require.True(t, ok, "summary property should exist")
	require.Equal(t, "string", summaryProp.Type, "summary property type should be string")

	// Verify optional fields exist
	epicIDProp, ok := tool.InputSchema.Properties["epic_id"]
	require.True(t, ok, "epic_id property should exist")
	require.Equal(t, "string", epicIDProp.Type, "epic_id property type should be string")
	require.NotContains(t, tool.InputSchema.Required, "epic_id", "epic_id should NOT be required")

	tasksClosedProp, ok := tool.InputSchema.Properties["tasks_closed"]
	require.True(t, ok, "tasks_closed property should exist")
	require.Equal(t, "number", tasksClosedProp.Type, "tasks_closed property type should be number")
	require.NotContains(t, tool.InputSchema.Required, "tasks_closed", "tasks_closed should NOT be required")
}

// TestSignalWorkflowComplete_ValidCall verifies valid tool call succeeds and routes to v2 adapter.
func TestSignalWorkflowComplete_ValidCall(t *testing.T) {
	cs := NewCoordinatorServer(nil, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))

	// Inject v2 adapter for test
	v2handler, cleanup := injectV2AdapterToCoordinator(t, cs)
	defer cleanup()

	// Configure v2 handler to return success
	v2handler.SetResult(&command.CommandResult{
		Success: true,
		Data:    "Workflow completed",
	})

	handler := cs.handlers["signal_workflow_complete"]

	// Call with valid arguments
	args := `{"status": "success", "summary": "All tasks completed successfully"}`
	result, err := handler(context.Background(), json.RawMessage(args))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError, "Result should not be an error")
	require.NotEmpty(t, result.Content, "Result content should not be empty")

	// Verify command was routed to v2
	cmds := v2handler.GetCommands()
	require.Len(t, cmds, 1, "Expected one command")
	require.Equal(t, command.CmdSignalWorkflowComplete, cmds[0].Type())
}

// TestSignalWorkflowComplete_MissingStatusReturnsError verifies missing required 'status' field returns error.
func TestSignalWorkflowComplete_MissingStatusReturnsError(t *testing.T) {
	cs := NewCoordinatorServer(nil, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))

	// Inject v2 adapter for test (validation happens in adapter)
	_, cleanup := injectV2AdapterToCoordinator(t, cs)
	defer cleanup()

	handler := cs.handlers["signal_workflow_complete"]

	// Call without status field
	args := `{"summary": "Some summary"}`
	_, err := handler(context.Background(), json.RawMessage(args))
	require.Error(t, err, "Should return error when status is missing")
	require.Contains(t, err.Error(), "status", "Error should mention 'status'")
}

// TestSignalWorkflowComplete_MissingSummaryReturnsError verifies missing required 'summary' field returns error.
func TestSignalWorkflowComplete_MissingSummaryReturnsError(t *testing.T) {
	cs := NewCoordinatorServer(nil, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))

	// Inject v2 adapter for test (validation happens in adapter)
	_, cleanup := injectV2AdapterToCoordinator(t, cs)
	defer cleanup()

	handler := cs.handlers["signal_workflow_complete"]

	// Call without summary field
	args := `{"status": "success"}`
	_, err := handler(context.Background(), json.RawMessage(args))
	require.Error(t, err, "Should return error when summary is missing")
	require.Contains(t, err.Error(), "summary", "Error should mention 'summary'")
}

// TestSignalWorkflowComplete_InvalidStatusEnumReturnsError verifies invalid status enum value returns validation error.
func TestSignalWorkflowComplete_InvalidStatusEnumReturnsError(t *testing.T) {
	cs := NewCoordinatorServer(nil, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))

	// Inject v2 adapter for test (validation happens in adapter)
	_, cleanup := injectV2AdapterToCoordinator(t, cs)
	defer cleanup()

	handler := cs.handlers["signal_workflow_complete"]

	// Call with invalid status value
	args := `{"status": "invalid_status", "summary": "Some summary"}`
	_, err := handler(context.Background(), json.RawMessage(args))
	require.Error(t, err, "Should return error for invalid status enum value")
	require.Contains(t, err.Error(), "status", "Error should mention 'status'")
}

// TestSignalWorkflowComplete_CoordinatorOnly verifies tool is NOT available to workers.
func TestSignalWorkflowComplete_CoordinatorOnly(t *testing.T) {
	// Create coordinator server and verify tool is registered
	cs := NewCoordinatorServer(nil, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))
	_, coordHas := cs.tools["signal_workflow_complete"]
	require.True(t, coordHas, "Coordinator should have signal_workflow_complete tool")

	// Create worker server and verify tool is NOT registered
	ws := NewWorkerServer("worker-1", nil)
	_, workerHas := ws.tools["signal_workflow_complete"]
	require.False(t, workerHas, "Worker should NOT have signal_workflow_complete tool")
}

// TestSignalWorkflowComplete_WithOptionalFields verifies optional fields are handled correctly.
func TestSignalWorkflowComplete_WithOptionalFields(t *testing.T) {
	cs := NewCoordinatorServer(nil, "/tmp/test", 8765, mocks.NewMockIssueExecutor(t))

	// Inject v2 adapter for test
	v2handler, cleanup := injectV2AdapterToCoordinator(t, cs)
	defer cleanup()

	// Configure v2 handler to return success
	v2handler.SetResult(&command.CommandResult{
		Success: true,
		Data:    "Workflow completed",
	})

	handler := cs.handlers["signal_workflow_complete"]

	// Call with all fields including optional ones
	args := `{"status": "success", "summary": "Epic completed", "epic_id": "perles-abc1", "tasks_closed": 5}`
	result, err := handler(context.Background(), json.RawMessage(args))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError, "Result should not be an error")

	// Verify command was routed to v2
	cmds := v2handler.GetCommands()
	require.Len(t, cmds, 1, "Expected one command")
	require.Equal(t, command.CmdSignalWorkflowComplete, cmds[0].Type())
}
