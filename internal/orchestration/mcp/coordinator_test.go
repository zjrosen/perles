package mcp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/orchestration/claude"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/orchestration/pool"
)

// TestNewCoordinatorServer_ProvidedBeadsExecutorIsUsed verifies mock injection works.
func TestNewCoordinatorServer_ProvidedBeadsExecutorIsUsed(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	mockExec := mocks.NewMockBeadsExecutor(t)

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mockExec)

	// beadsExecutor should be the mock we provided
	require.NotNil(t, cs.beadsExecutor, "beadsExecutor should not be nil")
	require.Equal(t, mockExec, cs.beadsExecutor, "beadsExecutor should be the provided mock")
}

// TestCoordinatorServer_RegistersAllTools verifies all coordinator tools are registered.
func TestCoordinatorServer_RegistersAllTools(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	expectedTools := []string{
		"spawn_worker",
		"assign_task",
		"replace_worker",
		"send_to_worker",
		"post_message",
		"get_task_status",
		"mark_task_complete",
		"mark_task_failed",
		"read_message_log",
		"list_workers",
		"prepare_handoff",
		"query_worker_state",
		"assign_task_review",
		"assign_review_feedback",
		"approve_commit",
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
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

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
// Note: Actual spawning will fail in unit tests without Claude, but we can test it doesn't error on empty args.
func TestCoordinatorServer_SpawnWorker(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))
	handler := cs.handlers["spawn_worker"]

	// spawn_worker takes no args, so empty args should be accepted (but will fail to actually spawn)
	_, err := handler(context.Background(), json.RawMessage(`{}`))
	// Expect error because we can't actually spawn Claude in a unit test
	require.Error(t, err, "Expected error when spawning worker (no Claude available)")
}

// TestCoordinatorServer_AssignTaskValidation tests input validation for assign_task.
func TestCoordinatorServer_AssignTaskValidation(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))
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
func TestCoordinatorServer_ReplaceWorkerValidation(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))
	handler := cs.handlers["replace_worker"]

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
			name:    "worker not found",
			args:    `{"worker_id": "nonexistent"}`,
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
func TestCoordinatorServer_SendToWorkerValidation(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))
	handler := cs.handlers["send_to_worker"]

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
		{
			name:    "worker not found",
			args:    `{"worker_id": "nonexistent", "message": "hello"}`,
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
func TestCoordinatorServer_PostMessageValidation(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	// No message issue available
	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))
	handler := cs.handlers["post_message"]

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
		{
			name:    "message issue not available",
			args:    `{"to": "ALL", "content": "hello"}`,
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
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))
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
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))
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
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))
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
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))
	handler := cs.handlers["read_message_log"]

	_, err := handler(context.Background(), json.RawMessage(`{}`))
	require.Error(t, err, "Expected error when message issue is nil")
}

// TestCoordinatorServer_GetPool tests the pool accessor.
func TestCoordinatorServer_GetPool(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	require.Equal(t, workerPool, cs.pool, "GetPool() did not return the expected pool")
}

// TestCoordinatorServer_GetMessageIssue tests the message issue accessor.
func TestCoordinatorServer_GetMessageIssue(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	require.Nil(t, cs.msgIssue, "GetMessageIssue() should return nil when no issue is set")
}

// TestCoordinatorServer_Instructions tests that instructions are set correctly.
func TestCoordinatorServer_Instructions(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

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
		{"mixed case prefix", "Perles-abc", true},
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
		{"too long suffix", "perles-abcdefghijk", false},
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

// TestCoordinatorServer_AssignTaskInvalidTaskID tests assign_task rejects invalid task IDs.
func TestCoordinatorServer_AssignTaskInvalidTaskID(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))
	handler := cs.handlers["assign_task"]

	tests := []struct {
		name   string
		taskID string
	}{
		{"shell injection", "perles-abc; rm -rf /"},
		{"path traversal", "../etc/passwd"},
		{"flag injection", "--help"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := `{"worker_id": "worker-1", "task_id": "` + tt.taskID + `"}`
			_, err := handler(context.Background(), json.RawMessage(args))
			require.Error(t, err, "Expected error for invalid task_id %q", tt.taskID)
		})
	}
}

// TestCoordinatorServer_ListWorkers_NoWorkers verifies list_workers returns appropriate message when no workers exist.
func TestCoordinatorServer_ListWorkers_NoWorkers(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))
	handler := cs.handlers["list_workers"]

	result, err := handler(context.Background(), nil)
	require.NoError(t, err, "Unexpected error")

	require.Equal(t, "No active workers.", result.Content[0].Text, "Expected 'No active workers.'")
}

// TestCoordinatorServer_ListWorkers_WithWorkers verifies list_workers returns worker info JSON.
func TestCoordinatorServer_ListWorkers_WithWorkers(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Note: We cannot easily spawn real workers in a unit test without full Claude integration.
	// This test verifies the handler executes without error when the pool is empty.
	// Integration tests should verify the tool works with actual workers.
	handler := cs.handlers["list_workers"]

	result, err := handler(context.Background(), nil)
	require.NoError(t, err, "Unexpected error")

	require.NotEmpty(t, result.Content[0].Text, "Expected non-empty result")
}

// TestPrepareHandoff_PostsMessage verifies tool posts message with correct type and content.
func TestPrepareHandoff_PostsMessage(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := message.New()
	cs := NewCoordinatorServer(claude.NewClient(), workerPool, msgIssue, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))
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
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := message.New()
	cs := NewCoordinatorServer(claude.NewClient(), workerPool, msgIssue, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))
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
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	// No message issue
	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))
	handler := cs.handlers["prepare_handoff"]

	args := `{"summary": "Test summary"}`
	_, err := handler(context.Background(), json.RawMessage(args))
	require.Error(t, err, "Expected error when message issue is nil")
}

// TestWorkerRole_Values verifies WorkerRole constant values.
func TestWorkerRole_Values(t *testing.T) {
	tests := []struct {
		role     WorkerRole
		expected string
	}{
		{RoleImplementer, "implementer"},
		{RoleReviewer, "reviewer"},
	}

	for _, tt := range tests {
		t.Run(string(tt.role), func(t *testing.T) {
			require.Equal(t, tt.expected, string(tt.role), "WorkerRole mismatch")
		})
	}
}

// TestTaskWorkflowStatus_Values verifies TaskWorkflowStatus constant values.
func TestTaskWorkflowStatus_Values(t *testing.T) {
	tests := []struct {
		status   TaskWorkflowStatus
		expected string
	}{
		{TaskImplementing, "implementing"},
		{TaskInReview, "in_review"},
		{TaskApproved, "approved"},
		{TaskDenied, "denied"},
		{TaskCommitting, "committing"},
		{TaskCompleted, "completed"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			require.Equal(t, tt.expected, string(tt.status), "TaskWorkflowStatus mismatch")
		})
	}
}

// TestWorkerAssignment_Fields verifies WorkerAssignment struct can be created and fields are accessible.
func TestWorkerAssignment_Fields(t *testing.T) {
	now := time.Now()
	wa := WorkerAssignment{
		TaskID:        "perles-abc.1",
		Role:          RoleImplementer,
		Phase:         events.PhaseImplementing,
		AssignedAt:    now,
		ImplementerID: "",
		ReviewerID:    "",
	}

	require.Equal(t, "perles-abc.1", wa.TaskID, "TaskID mismatch")
	require.Equal(t, RoleImplementer, wa.Role, "Role mismatch")
	require.Equal(t, events.PhaseImplementing, wa.Phase, "Phase mismatch")
	require.True(t, wa.AssignedAt.Equal(now), "AssignedAt mismatch")
}

// TestWorkerAssignment_ReviewerFields verifies reviewer-specific fields.
func TestWorkerAssignment_ReviewerFields(t *testing.T) {
	now := time.Now()
	wa := WorkerAssignment{
		TaskID:        "perles-abc.1",
		Role:          RoleReviewer,
		Phase:         events.PhaseReviewing,
		AssignedAt:    now,
		ImplementerID: "worker-1",
		ReviewerID:    "",
	}

	require.Equal(t, RoleReviewer, wa.Role, "Role mismatch")
	require.Equal(t, "worker-1", wa.ImplementerID, "ImplementerID mismatch")
}

// TestTaskAssignment_Fields verifies TaskAssignment struct can be created and fields are accessible.
func TestTaskAssignment_Fields(t *testing.T) {
	startTime := time.Now()
	reviewTime := startTime.Add(30 * time.Minute)
	ta := TaskAssignment{
		TaskID:          "perles-abc.1",
		Implementer:     "worker-1",
		Reviewer:        "worker-2",
		Status:          TaskInReview,
		StartedAt:       startTime,
		ReviewStartedAt: reviewTime,
	}

	require.Equal(t, "perles-abc.1", ta.TaskID, "TaskID mismatch")
	require.Equal(t, "worker-1", ta.Implementer, "Implementer mismatch")
	require.Equal(t, "worker-2", ta.Reviewer, "Reviewer mismatch")
	require.Equal(t, TaskInReview, ta.Status, "Status mismatch")
	require.True(t, ta.StartedAt.Equal(startTime), "StartedAt mismatch")
	require.True(t, ta.ReviewStartedAt.Equal(reviewTime), "ReviewStartedAt mismatch")
}

// TestCoordinatorServer_MapsInitialized verifies workerAssignments and taskAssignments maps are initialized.
func TestCoordinatorServer_MapsInitialized(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	require.NotNil(t, cs.workerAssignments, "workerAssignments map is nil, should be initialized")
	require.NotNil(t, cs.taskAssignments, "taskAssignments map is nil, should be initialized")

	// Verify maps are empty but usable
	require.Empty(t, cs.workerAssignments, "workerAssignments should be empty")
	require.Empty(t, cs.taskAssignments, "taskAssignments should be empty")

	// Verify we can write to and read from the maps
	cs.workerAssignments["worker-1"] = &WorkerAssignment{TaskID: "test-task"}
	require.Equal(t, "test-task", cs.workerAssignments["worker-1"].TaskID, "Failed to write/read workerAssignments")

	cs.taskAssignments["test-task"] = &TaskAssignment{Implementer: "worker-1"}
	require.Equal(t, "worker-1", cs.taskAssignments["test-task"].Implementer, "Failed to write/read taskAssignments")
}

// TestValidateTaskAssignment_TaskAlreadyAssigned verifies error when task already has an implementer.
func TestValidateTaskAssignment_TaskAlreadyAssigned(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Pre-assign task to a different worker
	cs.taskAssignments["perles-abc.1"] = &TaskAssignment{
		TaskID:      "perles-abc.1",
		Implementer: "worker-1",
		Status:      TaskImplementing,
	}

	err := cs.validateTaskAssignment("worker-2", "perles-abc.1")
	require.Error(t, err, "Expected error when task already assigned")
	require.Equal(t, "task perles-abc.1 already assigned to worker-1", err.Error(), "Unexpected error message")
}

// TestValidateTaskAssignment_WorkerAlreadyHasTask verifies error when worker already has an assignment.
func TestValidateTaskAssignment_WorkerAlreadyHasTask(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Pre-assign worker to a different task
	cs.workerAssignments["worker-1"] = &WorkerAssignment{
		TaskID: "perles-xyz.1",
		Role:   RoleImplementer,
		Phase:  events.PhaseImplementing,
	}

	err := cs.validateTaskAssignment("worker-1", "perles-abc.1")
	require.Error(t, err, "Expected error when worker already has task")
	require.Equal(t, "worker worker-1 already assigned to task perles-xyz.1", err.Error(), "Unexpected error message")
}

// TestValidateTaskAssignment_WorkerNotFound verifies error when worker doesn't exist.
func TestValidateTaskAssignment_WorkerNotFound(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	err := cs.validateTaskAssignment("nonexistent-worker", "perles-abc.1")
	require.Error(t, err, "Expected error when worker not found")
	require.Equal(t, "worker nonexistent-worker not found", err.Error(), "Unexpected error message")
}

// TestValidateTaskAssignment_WorkerNotReady verifies error when worker is not in Ready status.
func TestValidateTaskAssignment_WorkerNotReady(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Create a worker that is Working (not Ready)
	_ = workerPool.AddTestWorker("worker-1", pool.WorkerWorking)

	err := cs.validateTaskAssignment("worker-1", "perles-abc.1")
	require.Error(t, err, "Expected error when worker not ready")
	expectedMsg := "worker worker-1 is not ready (status: working)"
	require.Equal(t, expectedMsg, err.Error(), "Error message mismatch")
}

// TestValidateTaskAssignment_Success verifies no error when all conditions are met.
func TestValidateTaskAssignment_Success(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Create a worker that is Ready
	_ = workerPool.AddTestWorker("worker-1", pool.WorkerReady)

	err := cs.validateTaskAssignment("worker-1", "perles-abc.1")
	require.NoError(t, err, "Expected no error")
}

// TestValidateReviewAssignment_SameAsImplementer verifies error when reviewer == implementer.
func TestValidateReviewAssignment_SameAsImplementer(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	err := cs.validateReviewAssignment("worker-1", "perles-abc.1", "worker-1")
	require.Error(t, err, "Expected error when reviewer is same as implementer")
	require.Equal(t, "reviewer cannot be the same as implementer", err.Error(), "Unexpected error message")
}

// TestValidateReviewAssignment_TaskNotFound verifies error when task doesn't exist.
func TestValidateReviewAssignment_TaskNotFound(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	err := cs.validateReviewAssignment("worker-2", "perles-abc.1", "worker-1")
	require.Error(t, err, "Expected error when task not found")
	require.Equal(t, "task perles-abc.1 not found or implementer mismatch", err.Error(), "Unexpected error message")
}

// TestValidateReviewAssignment_ImplementerMismatch verifies error when implementer doesn't match.
func TestValidateReviewAssignment_ImplementerMismatch(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Task exists but with different implementer
	cs.taskAssignments["perles-abc.1"] = &TaskAssignment{
		TaskID:      "perles-abc.1",
		Implementer: "worker-3", // Different from passed implementer
	}

	err := cs.validateReviewAssignment("worker-2", "perles-abc.1", "worker-1")
	require.Error(t, err, "Expected error when implementer mismatch")
	require.Equal(t, "task perles-abc.1 not found or implementer mismatch", err.Error(), "Unexpected error message")
}

// TestValidateReviewAssignment_NotAwaitingReview verifies error when implementer not in AwaitingReview phase.
func TestValidateReviewAssignment_NotAwaitingReview(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Setup task with correct implementer
	cs.taskAssignments["perles-abc.1"] = &TaskAssignment{
		TaskID:      "perles-abc.1",
		Implementer: "worker-1",
	}

	// Implementer is still in Implementing phase (not awaiting review)
	cs.workerAssignments["worker-1"] = &WorkerAssignment{
		TaskID: "perles-abc.1",
		Role:   RoleImplementer,
		Phase:  events.PhaseImplementing, // Should be PhaseAwaitingReview
	}

	err := cs.validateReviewAssignment("worker-2", "perles-abc.1", "worker-1")
	require.Error(t, err, "Expected error when implementer not awaiting review")
	expectedMsg := "implementer worker-1 is not awaiting review (phase: implementing)"
	require.Equal(t, expectedMsg, err.Error(), "Error message mismatch")
}

// TestValidateReviewAssignment_AlreadyHasReviewer verifies error when task already has a reviewer.
func TestValidateReviewAssignment_AlreadyHasReviewer(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Setup task with implementer and existing reviewer
	cs.taskAssignments["perles-abc.1"] = &TaskAssignment{
		TaskID:      "perles-abc.1",
		Implementer: "worker-1",
		Reviewer:    "worker-3", // Already has a reviewer
	}

	// Implementer is awaiting review
	cs.workerAssignments["worker-1"] = &WorkerAssignment{
		TaskID: "perles-abc.1",
		Role:   RoleImplementer,
		Phase:  events.PhaseAwaitingReview,
	}

	err := cs.validateReviewAssignment("worker-2", "perles-abc.1", "worker-1")
	require.Error(t, err, "Expected error when task already has reviewer")
	require.Equal(t, "task perles-abc.1 already has reviewer worker-3", err.Error(), "Unexpected error message")
}

// TestValidateReviewAssignment_ReviewerNotReady verifies error when reviewer is not Ready.
func TestValidateReviewAssignment_ReviewerNotReady(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Setup task and implementer correctly
	cs.taskAssignments["perles-abc.1"] = &TaskAssignment{
		TaskID:      "perles-abc.1",
		Implementer: "worker-1",
	}
	cs.workerAssignments["worker-1"] = &WorkerAssignment{
		TaskID: "perles-abc.1",
		Role:   RoleImplementer,
		Phase:  events.PhaseAwaitingReview,
	}

	// Reviewer doesn't exist in pool
	err := cs.validateReviewAssignment("worker-2", "perles-abc.1", "worker-1")
	require.Error(t, err, "Expected error when reviewer not found")
	require.Equal(t, "reviewer worker-2 is not ready", err.Error(), "Unexpected error message")
}

// TestValidateReviewAssignment_Success verifies no error when all conditions are met.
func TestValidateReviewAssignment_Success(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Setup task and implementer correctly
	cs.taskAssignments["perles-abc.1"] = &TaskAssignment{
		TaskID:      "perles-abc.1",
		Implementer: "worker-1",
	}
	cs.workerAssignments["worker-1"] = &WorkerAssignment{
		TaskID: "perles-abc.1",
		Role:   RoleImplementer,
		Phase:  events.PhaseAwaitingReview,
	}

	// Create ready reviewer in pool
	_ = workerPool.AddTestWorker("worker-2", pool.WorkerReady)

	err := cs.validateReviewAssignment("worker-2", "perles-abc.1", "worker-1")
	require.NoError(t, err, "Expected no error")
}

// TestDetectOrphanedTasks_NoOrphans verifies empty result when no orphans.
func TestDetectOrphanedTasks_NoOrphans(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Create active workers
	_ = workerPool.AddTestWorker("worker-1", pool.WorkerWorking)
	_ = workerPool.AddTestWorker("worker-2", pool.WorkerWorking)

	// Setup task with active workers
	cs.taskAssignments["perles-abc.1"] = &TaskAssignment{
		TaskID:      "perles-abc.1",
		Implementer: "worker-1",
		Reviewer:    "worker-2",
	}

	orphans := cs.detectOrphanedTasks()
	require.Empty(t, orphans, "Expected no orphans")
}

// TestDetectOrphanedTasks_RetiredImplementer verifies orphan detected when implementer is retired.
func TestDetectOrphanedTasks_RetiredImplementer(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Create a retired worker
	_ = workerPool.AddTestWorker("worker-1", pool.WorkerRetired)

	// Setup task with retired implementer
	cs.taskAssignments["perles-abc.1"] = &TaskAssignment{
		TaskID:      "perles-abc.1",
		Implementer: "worker-1",
	}

	orphans := cs.detectOrphanedTasks()
	require.Len(t, orphans, 1, "Expected 1 orphan")
	require.Equal(t, "perles-abc.1", orphans[0], "Expected orphan perles-abc.1")
}

// TestDetectOrphanedTasks_MissingImplementer verifies orphan detected when implementer is missing from pool.
func TestDetectOrphanedTasks_MissingImplementer(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Setup task with non-existent implementer
	cs.taskAssignments["perles-abc.1"] = &TaskAssignment{
		TaskID:      "perles-abc.1",
		Implementer: "nonexistent-worker",
	}

	orphans := cs.detectOrphanedTasks()
	require.Len(t, orphans, 1, "Expected 1 orphan")
}

// TestDetectOrphanedTasks_RetiredReviewer verifies orphan detected when reviewer is retired.
func TestDetectOrphanedTasks_RetiredReviewer(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Create active implementer and retired reviewer
	_ = workerPool.AddTestWorker("worker-1", pool.WorkerWorking)
	_ = workerPool.AddTestWorker("worker-2", pool.WorkerRetired)

	// Setup task with retired reviewer
	cs.taskAssignments["perles-abc.1"] = &TaskAssignment{
		TaskID:      "perles-abc.1",
		Implementer: "worker-1",
		Reviewer:    "worker-2",
	}

	orphans := cs.detectOrphanedTasks()
	require.Len(t, orphans, 1, "Expected 1 orphan")
}

// TestCheckStuckWorkers_NoStuck verifies empty result when no stuck workers.
func TestCheckStuckWorkers_NoStuck(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Worker assigned recently (within MaxTaskDuration)
	cs.workerAssignments["worker-1"] = &WorkerAssignment{
		TaskID:     "perles-abc.1",
		AssignedAt: time.Now(), // Just assigned
	}

	stuck := cs.checkStuckWorkers()
	require.Empty(t, stuck, "Expected no stuck workers")
}

// TestCheckStuckWorkers_ExceededDuration verifies stuck worker detected when exceeding MaxTaskDuration.
func TestCheckStuckWorkers_ExceededDuration(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Worker assigned more than MaxTaskDuration ago
	cs.workerAssignments["worker-1"] = &WorkerAssignment{
		TaskID:     "perles-abc.1",
		AssignedAt: time.Now().Add(-MaxTaskDuration - time.Minute), // Exceeded
	}

	stuck := cs.checkStuckWorkers()
	require.Len(t, stuck, 1, "Expected 1 stuck worker")
	require.Equal(t, "worker-1", stuck[0], "Expected stuck worker worker-1")
}

// TestCheckStuckWorkers_NoTask verifies workers without tasks are not considered stuck.
func TestCheckStuckWorkers_NoTask(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Worker with empty TaskID (idle) shouldn't be considered stuck
	cs.workerAssignments["worker-1"] = &WorkerAssignment{
		TaskID:     "",                                             // No active task
		AssignedAt: time.Now().Add(-MaxTaskDuration - time.Minute), // Old assignment
	}

	stuck := cs.checkStuckWorkers()
	require.Empty(t, stuck, "Expected no stuck workers (idle worker)")
}

// TestMaxTaskDuration verifies the constant value.
func TestMaxTaskDuration(t *testing.T) {
	expected := 30 * time.Minute
	require.Equal(t, expected, MaxTaskDuration, "MaxTaskDuration mismatch")
}

// TestQueryWorkerState_NoWorkers verifies query_worker_state returns empty when no workers exist.
func TestQueryWorkerState_NoWorkers(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))
	handler := cs.handlers["query_worker_state"]

	result, err := handler(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err, "Unexpected error")

	// Parse response
	var response workerStateResponse
	err = json.Unmarshal([]byte(result.Content[0].Text), &response)
	require.NoError(t, err, "Failed to parse response")

	require.Empty(t, response.Workers, "Expected 0 workers")
	require.Empty(t, response.TaskAssignments, "Expected 0 task assignments")
	require.Empty(t, response.ReadyWorkers, "Expected 0 ready workers")
}

// TestQueryWorkerState_WithWorkerAndAssignment verifies query_worker_state returns worker with phase and role.
func TestQueryWorkerState_WithWorkerAndAssignment(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Add a worker
	_ = workerPool.AddTestWorker("worker-1", pool.WorkerWorking)

	// Add assignment
	cs.workerAssignments["worker-1"] = &WorkerAssignment{
		TaskID: "perles-abc.1",
		Role:   RoleImplementer,
		Phase:  events.PhaseImplementing,
	}
	cs.taskAssignments["perles-abc.1"] = &TaskAssignment{
		TaskID:      "perles-abc.1",
		Implementer: "worker-1",
		Status:      TaskImplementing,
	}

	handler := cs.handlers["query_worker_state"]
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
	require.Equal(t, "implementer", worker.Role, "Role mismatch")
	require.Equal(t, "perles-abc.1", worker.TaskID, "TaskID mismatch")

	// Check task assignments
	require.Len(t, response.TaskAssignments, 1, "Expected 1 task assignment")
	ta := response.TaskAssignments["perles-abc.1"]
	require.Equal(t, "worker-1", ta.Implementer, "TaskAssignment.Implementer mismatch")
}

// TestQueryWorkerState_FilterByWorkerID verifies query_worker_state filters by worker_id.
func TestQueryWorkerState_FilterByWorkerID(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Add multiple workers
	_ = workerPool.AddTestWorker("worker-1", pool.WorkerWorking)
	_ = workerPool.AddTestWorker("worker-2", pool.WorkerReady)

	handler := cs.handlers["query_worker_state"]
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
func TestQueryWorkerState_FilterByTaskID(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Add workers with different tasks
	_ = workerPool.AddTestWorker("worker-1", pool.WorkerWorking)
	_ = workerPool.AddTestWorker("worker-2", pool.WorkerWorking)

	cs.workerAssignments["worker-1"] = &WorkerAssignment{
		TaskID: "perles-abc.1",
		Role:   RoleImplementer,
		Phase:  events.PhaseImplementing,
	}
	cs.workerAssignments["worker-2"] = &WorkerAssignment{
		TaskID: "perles-xyz.1",
		Role:   RoleImplementer,
		Phase:  events.PhaseImplementing,
	}

	handler := cs.handlers["query_worker_state"]
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
func TestQueryWorkerState_ReturnsReadyWorkers(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Add a ready worker
	_ = workerPool.AddTestWorker("worker-1", pool.WorkerReady)

	handler := cs.handlers["query_worker_state"]
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

// TestAssignTaskReview_SelfReviewRejected verifies assign_task_review rejects self-review.
func TestAssignTaskReview_SelfReviewRejected(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))
	handler := cs.handlers["assign_task_review"]

	args := `{"reviewer_id": "worker-1", "task_id": "perles-abc.1", "implementer_id": "worker-1", "summary": "test"}`
	_, err := handler(context.Background(), json.RawMessage(args))
	require.Error(t, err, "Expected error for self-review")
	require.Contains(t, err.Error(), "reviewer cannot be the same as implementer", "Unexpected error message")
}

// TestAssignTaskReview_TaskNotAwaitingReview verifies assign_task_review rejects if task not awaiting review.
func TestAssignTaskReview_TaskNotAwaitingReview(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Setup task and implementer in wrong phase
	cs.taskAssignments["perles-abc.1"] = &TaskAssignment{
		TaskID:      "perles-abc.1",
		Implementer: "worker-1",
		Status:      TaskImplementing,
	}
	cs.workerAssignments["worker-1"] = &WorkerAssignment{
		TaskID: "perles-abc.1",
		Role:   RoleImplementer,
		Phase:  events.PhaseImplementing, // Not awaiting review
	}
	_ = workerPool.AddTestWorker("worker-2", pool.WorkerReady)

	handler := cs.handlers["assign_task_review"]
	args := `{"reviewer_id": "worker-2", "task_id": "perles-abc.1", "implementer_id": "worker-1", "summary": "test"}`
	_, err := handler(context.Background(), json.RawMessage(args))
	require.Error(t, err, "Expected error when task not awaiting review")
}

// TestAssignTaskReview_ValidationRequired verifies required field validation.
func TestAssignTaskReview_ValidationRequired(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))
	handler := cs.handlers["assign_task_review"]

	tests := []struct {
		name string
		args string
	}{
		{"missing reviewer_id", `{"task_id": "perles-abc.1", "implementer_id": "worker-1", "summary": "test"}`},
		{"missing task_id", `{"reviewer_id": "worker-2", "implementer_id": "worker-1", "summary": "test"}`},
		{"missing implementer_id", `{"reviewer_id": "worker-2", "task_id": "perles-abc.1", "summary": "test"}`},
		{"missing summary", `{"reviewer_id": "worker-2", "task_id": "perles-abc.1", "implementer_id": "worker-1"}`},
		{"invalid task_id", `{"reviewer_id": "worker-2", "task_id": "invalid", "implementer_id": "worker-1", "summary": "test"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := handler(context.Background(), json.RawMessage(tt.args))
			require.Error(t, err, "Expected error for %s", tt.name)
		})
	}
}

// TestAssignReviewFeedback_TaskNotDenied verifies assign_review_feedback rejects if task not denied.
func TestAssignReviewFeedback_TaskNotDenied(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Setup task in approved state (not denied)
	cs.taskAssignments["perles-abc.1"] = &TaskAssignment{
		TaskID:      "perles-abc.1",
		Implementer: "worker-1",
		Status:      TaskApproved, // Not denied
	}
	cs.workerAssignments["worker-1"] = &WorkerAssignment{
		TaskID: "perles-abc.1",
		Role:   RoleImplementer,
		Phase:  events.PhaseAwaitingReview,
	}
	_ = workerPool.AddTestWorker("worker-1", pool.WorkerReady)

	handler := cs.handlers["assign_review_feedback"]
	args := `{"implementer_id": "worker-1", "task_id": "perles-abc.1", "feedback": "fix bugs"}`
	_, err := handler(context.Background(), json.RawMessage(args))
	require.Error(t, err, "Expected error when task not denied")
	require.Contains(t, err.Error(), "not in denied status", "Unexpected error message")
}

// TestAssignReviewFeedback_ValidationRequired verifies required field validation.
func TestAssignReviewFeedback_ValidationRequired(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))
	handler := cs.handlers["assign_review_feedback"]

	tests := []struct {
		name string
		args string
	}{
		{"missing implementer_id", `{"task_id": "perles-abc.1", "feedback": "fix"}`},
		{"missing task_id", `{"implementer_id": "worker-1", "feedback": "fix"}`},
		{"missing feedback", `{"implementer_id": "worker-1", "task_id": "perles-abc.1"}`},
		{"empty feedback", `{"implementer_id": "worker-1", "task_id": "perles-abc.1", "feedback": ""}`},
		{"invalid task_id", `{"implementer_id": "worker-1", "task_id": "invalid", "feedback": "fix"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := handler(context.Background(), json.RawMessage(tt.args))
			require.Error(t, err, "Expected error for %s", tt.name)
		})
	}
}

// TestApproveCommit_TaskNotApproved verifies approve_commit rejects if task not approved.
func TestApproveCommit_TaskNotApproved(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Setup task in denied state (not approved)
	cs.taskAssignments["perles-abc.1"] = &TaskAssignment{
		TaskID:      "perles-abc.1",
		Implementer: "worker-1",
		Status:      TaskDenied, // Not approved
	}

	handler := cs.handlers["approve_commit"]
	args := `{"implementer_id": "worker-1", "task_id": "perles-abc.1"}`
	_, err := handler(context.Background(), json.RawMessage(args))
	require.Error(t, err, "Expected error when task not approved")
	require.Contains(t, err.Error(), "not in approved status", "Unexpected error message")
}

// TestApproveCommit_ValidationRequired verifies required field validation.
func TestApproveCommit_ValidationRequired(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))
	handler := cs.handlers["approve_commit"]

	tests := []struct {
		name string
		args string
	}{
		{"missing implementer_id", `{"task_id": "perles-abc.1"}`},
		{"missing task_id", `{"implementer_id": "worker-1"}`},
		{"empty implementer_id", `{"implementer_id": "", "task_id": "perles-abc.1"}`},
		{"empty task_id", `{"implementer_id": "worker-1", "task_id": ""}`},
		{"invalid task_id", `{"implementer_id": "worker-1", "task_id": "invalid"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := handler(context.Background(), json.RawMessage(tt.args))
			require.Error(t, err, "Expected error for %s", tt.name)
		})
	}
}

// TestApproveCommit_ImplementerMismatch verifies approve_commit rejects wrong implementer.
func TestApproveCommit_ImplementerMismatch(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Setup task with different implementer
	cs.taskAssignments["perles-abc.1"] = &TaskAssignment{
		TaskID:      "perles-abc.1",
		Implementer: "worker-1", // Actual implementer
		Status:      TaskApproved,
	}

	handler := cs.handlers["approve_commit"]
	args := `{"implementer_id": "worker-2", "task_id": "perles-abc.1"}` // Wrong implementer
	_, err := handler(context.Background(), json.RawMessage(args))
	require.Error(t, err, "Expected error for wrong implementer")
	require.Contains(t, err.Error(), "not the implementer", "Unexpected error message")
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
// Phase 5 Tests: Updated assign_task and list_workers with state tracking
// ============================================================================

// TestAssignTask_ValidatesAssignment verifies assign_task calls validateTaskAssignment.
func TestAssignTask_ValidatesAssignment(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))
	handler := cs.handlers["assign_task"]

	// No worker exists - should fail validation
	args := `{"worker_id": "worker-1", "task_id": "perles-abc.1"}`
	_, err := handler(context.Background(), json.RawMessage(args))
	require.Error(t, err, "Expected error when worker not found (validation)")
	require.Contains(t, err.Error(), "validation failed", "Expected validation error")
}

// TestAssignTask_RejectsWhenTaskAlreadyAssigned verifies assign_task rejects duplicate task assignment.
func TestAssignTask_RejectsWhenTaskAlreadyAssigned(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Create a ready worker
	_ = workerPool.AddTestWorker("worker-2", pool.WorkerReady)

	// Pre-assign the task to another worker
	cs.taskAssignments["perles-abc.1"] = &TaskAssignment{
		TaskID:      "perles-abc.1",
		Implementer: "worker-1",
		Status:      TaskImplementing,
	}

	handler := cs.handlers["assign_task"]
	args := `{"worker_id": "worker-2", "task_id": "perles-abc.1"}`
	_, err := handler(context.Background(), json.RawMessage(args))
	require.Error(t, err, "Expected error when task already assigned")
	require.Contains(t, err.Error(), "already assigned", "Expected 'already assigned' error")
}

// TestAssignTask_RejectsWhenWorkerAlreadyHasTask verifies assign_task rejects if worker busy.
func TestAssignTask_RejectsWhenWorkerAlreadyHasTask(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Create a worker that has an assignment
	_ = workerPool.AddTestWorker("worker-1", pool.WorkerReady)
	cs.workerAssignments["worker-1"] = &WorkerAssignment{
		TaskID: "perles-xyz.1",
		Role:   RoleImplementer,
		Phase:  events.PhaseImplementing,
	}

	handler := cs.handlers["assign_task"]
	args := `{"worker_id": "worker-1", "task_id": "perles-abc.1"}`
	_, err := handler(context.Background(), json.RawMessage(args))
	require.Error(t, err, "Expected error when worker already has task")
	require.Contains(t, err.Error(), "already assigned", "Expected 'already assigned' error")
}

// TestListWorkers_IncludesPhaseAndRole verifies list_workers returns phase and role.
func TestListWorkers_IncludesPhaseAndRole(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Add a worker with an assignment
	_ = workerPool.AddTestWorker("worker-1", pool.WorkerWorking)
	cs.workerAssignments["worker-1"] = &WorkerAssignment{
		TaskID: "perles-abc.1",
		Role:   RoleImplementer,
		Phase:  events.PhaseImplementing,
	}

	handler := cs.handlers["list_workers"]
	result, err := handler(context.Background(), nil)
	require.NoError(t, err, "Unexpected error")

	// Parse response
	type workerInfo struct {
		WorkerID string `json:"worker_id"`
		Phase    string `json:"phase"`
		Role     string `json:"role,omitempty"`
	}
	var infos []workerInfo
	err = json.Unmarshal([]byte(result.Content[0].Text), &infos)
	require.NoError(t, err, "Failed to parse response")

	require.Len(t, infos, 1, "Expected 1 worker")

	info := infos[0]
	require.Equal(t, "implementing", info.Phase, "Phase mismatch")
	require.Equal(t, "implementer", info.Role, "Role mismatch")
}

// TestListWorkers_ShowsIdlePhaseForNoAssignment verifies workers without assignments show idle phase.
func TestListWorkers_ShowsIdlePhaseForNoAssignment(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Add a worker without any assignment
	_ = workerPool.AddTestWorker("worker-1", pool.WorkerReady)

	handler := cs.handlers["list_workers"]
	result, err := handler(context.Background(), nil)
	require.NoError(t, err, "Unexpected error")

	// Parse response
	type workerInfo struct {
		WorkerID string `json:"worker_id"`
		Phase    string `json:"phase"`
		Role     string `json:"role,omitempty"`
	}
	var infos []workerInfo
	err = json.Unmarshal([]byte(result.Content[0].Text), &infos)
	require.NoError(t, err, "Failed to parse response")

	require.Len(t, infos, 1, "Expected 1 worker")

	info := infos[0]
	require.Equal(t, "idle", info.Phase, "Phase mismatch")
	require.Empty(t, info.Role, "Role should be empty for idle worker")
}

// TestListWorkers_ShowsReviewerRole verifies reviewer workers show correct role.
func TestListWorkers_ShowsReviewerRole(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Add a worker as reviewer
	_ = workerPool.AddTestWorker("worker-2", pool.WorkerWorking)
	cs.workerAssignments["worker-2"] = &WorkerAssignment{
		TaskID:        "perles-abc.1",
		Role:          RoleReviewer,
		Phase:         events.PhaseReviewing,
		ImplementerID: "worker-1",
	}

	handler := cs.handlers["list_workers"]
	result, err := handler(context.Background(), nil)
	require.NoError(t, err, "Unexpected error")

	// Parse response
	type workerInfo struct {
		WorkerID string `json:"worker_id"`
		Phase    string `json:"phase"`
		Role     string `json:"role,omitempty"`
	}
	var infos []workerInfo
	err = json.Unmarshal([]byte(result.Content[0].Text), &infos)
	require.NoError(t, err, "Failed to parse response")

	require.Len(t, infos, 1, "Expected 1 worker")

	info := infos[0]
	require.Equal(t, "reviewing", info.Phase, "Phase mismatch")
	require.Equal(t, "reviewer", info.Role, "Role mismatch")
}

// TestReplaceWorker_CleansUpWorkerAssignments verifies replace_worker removes assignment.
func TestReplaceWorker_CleansUpWorkerAssignments(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Add a worker with an assignment
	_ = workerPool.AddTestWorker("worker-1", pool.WorkerWorking)
	cs.workerAssignments["worker-1"] = &WorkerAssignment{
		TaskID: "perles-abc.1",
		Role:   RoleImplementer,
		Phase:  events.PhaseImplementing,
	}
	cs.taskAssignments["perles-abc.1"] = &TaskAssignment{
		TaskID:      "perles-abc.1",
		Implementer: "worker-1",
		Status:      TaskImplementing,
	}

	// Verify assignment exists before replace
	_, ok := cs.workerAssignments["worker-1"]
	require.True(t, ok, "Worker assignment should exist before replace")

	handler := cs.handlers["replace_worker"]
	_, err := handler(context.Background(), json.RawMessage(`{"worker_id": "worker-1"}`))
	// Note: This will fail to spawn replacement worker (no Claude) but should still cleanup
	// We're testing the cleanup logic, which happens before the spawn attempt

	// Even if spawn fails, the assignment should be cleaned up
	// In this case, the error is expected because we can't spawn without Claude
	_ = err // We acknowledge the error but verify the cleanup happened

	// Verify assignment was cleaned up
	cs.assignmentsMu.RLock()
	_, stillExists := cs.workerAssignments["worker-1"]
	cs.assignmentsMu.RUnlock()

	require.False(t, stillExists, "Worker assignment should be cleaned up after replace")

	// Task assignment should still exist (for orphan detection)
	cs.assignmentsMu.RLock()
	_, taskExists := cs.taskAssignments["perles-abc.1"]
	cs.assignmentsMu.RUnlock()

	require.True(t, taskExists, "Task assignment should still exist after worker replaced (for orphan detection)")
}

// TestTaskAssignmentPrompt_WithSummary verifies TaskAssignmentPrompt includes summary when provided.
func TestTaskAssignmentPrompt_WithSummary(t *testing.T) {
	prompt := TaskAssignmentPrompt("perles-abc.1", "Test Task", "Focus on error handling.")

	require.True(t, containsInternal(prompt, "Coordinator Instructions:"), "Prompt should contain 'Coordinator Instructions:' section when summary provided")
	require.True(t, containsInternal(prompt, "Focus on error handling."), "Prompt should contain the summary content")
}

// TestTaskAssignmentPrompt_WithoutSummary verifies TaskAssignmentPrompt excludes summary section when empty.
func TestTaskAssignmentPrompt_WithoutSummary(t *testing.T) {
	prompt := TaskAssignmentPrompt("perles-abc.1", "Test Task", "")

	require.False(t, containsInternal(prompt, "Coordinator Instructions:"), "Prompt should NOT contain 'Coordinator Instructions:' section when summary is empty")
}

// TestTaskAssignmentPrompt_AllSections verifies TaskAssignmentPrompt includes all sections when provided.
func TestTaskAssignmentPrompt_AllSections(t *testing.T) {
	prompt := TaskAssignmentPrompt(
		"perles-abc.1",
		"Implement Feature X",
		"Important: Check existing patterns in module Y",
	)

	// Verify all sections are present
	sections := []string{
		"[TASK ASSIGNMENT]",
		"Task ID: perles-abc.1",
		"Title: Implement Feature X",
		"Coordinator Instructions:",
		"Important: Check existing patterns in module Y",
		"report_implementation_complete",
	}

	for _, section := range sections {
		require.True(t, containsInternal(prompt, section), "Prompt should contain %q", section)
	}
}

// TestAssignTaskArgs_SummaryField verifies assignTaskArgs struct includes Summary field.
func TestAssignTaskArgs_SummaryField(t *testing.T) {
	args := assignTaskArgs{
		WorkerID: "worker-1",
		TaskID:   "perles-abc.1",
		Summary:  "Key instructions for the worker",
	}

	require.Equal(t, "Key instructions for the worker", args.Summary, "Summary mismatch")
}

// TestAssignTaskArgs_SummaryOmitempty verifies summary is optional.
func TestAssignTaskArgs_SummaryOmitempty(t *testing.T) {
	// Test that JSON with no summary field unmarshals correctly
	jsonStr := `{"worker_id": "worker-1", "task_id": "perles-abc.1"}`
	var args assignTaskArgs
	err := json.Unmarshal([]byte(jsonStr), &args)
	require.NoError(t, err, "Failed to unmarshal")

	require.Equal(t, "worker-1", args.WorkerID, "WorkerID mismatch")
	require.Equal(t, "perles-abc.1", args.TaskID, "TaskID mismatch")
	require.Empty(t, args.Summary, "Summary should be empty")
}

// TestAssignTaskArgs_SummaryInJSON verifies summary is included when provided in JSON.
func TestAssignTaskArgs_SummaryInJSON(t *testing.T) {
	jsonStr := `{"worker_id": "worker-1", "task_id": "perles-abc.1", "summary": "Focus on the FetchData method"}`
	var args assignTaskArgs
	err := json.Unmarshal([]byte(jsonStr), &args)
	require.NoError(t, err, "Failed to unmarshal")

	require.Equal(t, "Focus on the FetchData method", args.Summary, "Summary mismatch")
}

// TestCoordinatorServer_AssignTaskSchemaIncludesSummary verifies the tool schema includes summary parameter.
func TestCoordinatorServer_AssignTaskSchemaIncludesSummary(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

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

// TestIntegration_AssignListReplaceFlow tests the full flow maintains consistent state.
func TestIntegration_AssignListReplaceFlow(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))

	// Create a ready worker
	_ = workerPool.AddTestWorker("worker-1", pool.WorkerReady)

	// Pre-populate assignments to simulate a successful assign_task call
	// (We can't actually run assign_task without bd/Claude)
	cs.assignmentsMu.Lock()
	cs.workerAssignments["worker-1"] = &WorkerAssignment{
		TaskID:     "perles-abc.1",
		Role:       RoleImplementer,
		Phase:      events.PhaseImplementing,
		AssignedAt: time.Now(),
	}
	cs.taskAssignments["perles-abc.1"] = &TaskAssignment{
		TaskID:      "perles-abc.1",
		Implementer: "worker-1",
		Status:      TaskImplementing,
		StartedAt:   time.Now(),
	}
	cs.assignmentsMu.Unlock()

	// Change worker status to Working to reflect assignment
	workerPool.GetWorker("worker-1").AssignTask("perles-abc.1")

	// List workers - should show implementing phase
	listHandler := cs.handlers["list_workers"]
	result, err := listHandler(context.Background(), nil)
	require.NoError(t, err, "list_workers error")

	type workerInfo struct {
		WorkerID string `json:"worker_id"`
		Phase    string `json:"phase"`
		Role     string `json:"role,omitempty"`
	}
	var infos []workerInfo
	err = json.Unmarshal([]byte(result.Content[0].Text), &infos)
	require.NoError(t, err, "Failed to parse list_workers response")

	require.Len(t, infos, 1, "Expected 1 worker")
	require.Equal(t, "implementing", infos[0].Phase, "Expected implementing phase")

	// Query worker state - should show same info
	queryHandler := cs.handlers["query_worker_state"]
	result, err = queryHandler(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err, "query_worker_state error")

	var stateResponse workerStateResponse
	err = json.Unmarshal([]byte(result.Content[0].Text), &stateResponse)
	require.NoError(t, err, "Failed to parse query_worker_state response")

	// Both list_workers and query_worker_state should report same phase
	require.Len(t, stateResponse.Workers, 1, "Expected 1 worker in state response")
	require.Equal(t, "implementing", stateResponse.Workers[0].Phase, "query_worker_state phase mismatch")

	// Task assignments should be tracked
	require.Len(t, stateResponse.TaskAssignments, 1, "Expected 1 task assignment")
	ta := stateResponse.TaskAssignments["perles-abc.1"]
	require.Equal(t, "worker-1", ta.Implementer, "TaskAssignment implementer mismatch")
}

// ============================================================================
// Unread Message Tracking Tests
// ============================================================================

// TestReadMessageLog_UnreadDefault_Basic tests that sequential read calls return only new messages.
func TestReadMessageLog_UnreadDefault_Basic(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := message.New()
	cs := NewCoordinatorServer(claude.NewClient(), workerPool, msgIssue, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))
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
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := message.New()

	// Post messages before creating coordinator (simulates messages existing before coordinator joins)
	_, _ = msgIssue.Append("WORKER.1", "COORDINATOR", "Hello", message.MessageInfo)
	_, _ = msgIssue.Append("WORKER.2", "COORDINATOR", "World", message.MessageInfo)

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, msgIssue, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))
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
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := message.New()
	cs := NewCoordinatorServer(claude.NewClient(), workerPool, msgIssue, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))
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
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := message.New()
	cs := NewCoordinatorServer(claude.NewClient(), workerPool, msgIssue, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))
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
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	msgIssue := message.New()
	cs := NewCoordinatorServer(claude.NewClient(), workerPool, msgIssue, "/tmp/test", 8765, nil, mocks.NewMockBeadsExecutor(t))
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
