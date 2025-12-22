package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"perles/internal/orchestration/claude"
	"perles/internal/orchestration/pool"
)

// TestCoordinatorServer_RegistersAllTools verifies all coordinator tools are registered.
func TestCoordinatorServer_RegistersAllTools(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", nil)

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
	}

	for _, toolName := range expectedTools {
		if _, ok := cs.tools[toolName]; !ok {
			t.Errorf("Tool %q not registered", toolName)
		}
		if _, ok := cs.handlers[toolName]; !ok {
			t.Errorf("Handler for %q not registered", toolName)
		}
	}

	if len(cs.tools) != len(expectedTools) {
		t.Errorf("Tool count = %d, want %d", len(cs.tools), len(expectedTools))
	}
}

// TestCoordinatorServer_ToolSchemas verifies tool schemas are valid.
func TestCoordinatorServer_ToolSchemas(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", nil)

	for name, tool := range cs.tools {
		t.Run(name, func(t *testing.T) {
			if tool.Name == "" {
				t.Error("Tool name is empty")
			}
			if tool.Description == "" {
				t.Error("Tool description is empty")
			}
			if tool.InputSchema == nil {
				t.Error("Tool inputSchema is nil")
			}
			if tool.InputSchema != nil && tool.InputSchema.Type != "object" {
				t.Errorf("InputSchema.Type = %q, want %q", tool.InputSchema.Type, "object")
			}
		})
	}
}

// TestCoordinatorServer_SpawnWorker tests spawn_worker (takes no args).
// Note: Actual spawning will fail in unit tests without Claude, but we can test it doesn't error on empty args.
func TestCoordinatorServer_SpawnWorker(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", nil)
	handler := cs.handlers["spawn_worker"]

	// spawn_worker takes no args, so empty args should be accepted (but will fail to actually spawn)
	_, err := handler(context.Background(), json.RawMessage(`{}`))
	// Expect error because we can't actually spawn Claude in a unit test
	if err == nil {
		t.Error("Expected error when spawning worker (no Claude available)")
	}
}

// TestCoordinatorServer_AssignTaskValidation tests input validation for assign_task.
func TestCoordinatorServer_AssignTaskValidation(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", nil)
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
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

// TestCoordinatorServer_ReplaceWorkerValidation tests input validation for replace_worker.
func TestCoordinatorServer_ReplaceWorkerValidation(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", nil)
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
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

// TestCoordinatorServer_SendToWorkerValidation tests input validation for send_to_worker.
func TestCoordinatorServer_SendToWorkerValidation(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", nil)
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
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

// TestCoordinatorServer_PostMessageValidation tests input validation for post_message.
func TestCoordinatorServer_PostMessageValidation(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	// No message issue available
	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", nil)
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
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

// TestCoordinatorServer_GetTaskStatusValidation tests input validation for get_task_status.
func TestCoordinatorServer_GetTaskStatusValidation(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", nil)
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
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

// TestCoordinatorServer_MarkTaskCompleteValidation tests input validation for mark_task_complete.
func TestCoordinatorServer_MarkTaskCompleteValidation(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", nil)
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
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

// TestCoordinatorServer_MarkTaskFailedValidation tests input validation for mark_task_failed.
func TestCoordinatorServer_MarkTaskFailedValidation(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", nil)
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
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

// TestCoordinatorServer_ReadMessageLogNoIssue tests read_message_log when no issue is available.
func TestCoordinatorServer_ReadMessageLogNoIssue(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", nil)
	handler := cs.handlers["read_message_log"]

	_, err := handler(context.Background(), json.RawMessage(`{}`))
	if err == nil {
		t.Error("Expected error when message issue is nil")
	}
}

// TestCoordinatorServer_GetPool tests the pool accessor.
func TestCoordinatorServer_GetPool(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", nil)

	if cs.GetPool() != workerPool {
		t.Error("GetPool() did not return the expected pool")
	}
}

// TestCoordinatorServer_GetMessageIssue tests the message issue accessor.
func TestCoordinatorServer_GetMessageIssue(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", nil)

	if cs.GetMessageIssue() != nil {
		t.Error("GetMessageIssue() should return nil when no issue is set")
	}
}

// TestCoordinatorServer_Instructions tests that instructions are set correctly.
func TestCoordinatorServer_Instructions(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", nil)

	if cs.instructions == "" {
		t.Error("Instructions should be set")
	}
	if cs.info.Name != "perles-orchestrator" {
		t.Errorf("Server name = %q, want %q", cs.info.Name, "perles-orchestrator")
	}
	if cs.info.Version != "1.0.0" {
		t.Errorf("Server version = %q, want %q", cs.info.Version, "1.0.0")
	}
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
			got := IsValidTaskID(tt.taskID)
			if got != tt.want {
				t.Errorf("IsValidTaskID(%q) = %v, want %v", tt.taskID, got, tt.want)
			}
		})
	}
}

// TestCoordinatorServer_AssignTaskInvalidTaskID tests assign_task rejects invalid task IDs.
func TestCoordinatorServer_AssignTaskInvalidTaskID(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", nil)
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
			if err == nil {
				t.Errorf("Expected error for invalid task_id %q", tt.taskID)
			}
		})
	}
}

// TestCoordinatorServer_ListWorkers_NoWorkers verifies list_workers returns appropriate message when no workers exist.
func TestCoordinatorServer_ListWorkers_NoWorkers(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", nil)
	handler := cs.handlers["list_workers"]

	result, err := handler(context.Background(), nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.Content[0].Text != "No active workers." {
		t.Errorf("Expected 'No active workers.', got %q", result.Content[0].Text)
	}
}

// TestCoordinatorServer_ListWorkers_WithWorkers verifies list_workers returns worker info JSON.
func TestCoordinatorServer_ListWorkers_WithWorkers(t *testing.T) {
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	cs := NewCoordinatorServer(claude.NewClient(), workerPool, nil, "/tmp/test", nil)

	// Note: We cannot easily spawn real workers in a unit test without full Claude integration.
	// This test verifies the handler executes without error when the pool is empty.
	// Integration tests should verify the tool works with actual workers.
	handler := cs.handlers["list_workers"]

	result, err := handler(context.Background(), nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.Content[0].Text == "" {
		t.Error("Expected non-empty result")
	}
}
