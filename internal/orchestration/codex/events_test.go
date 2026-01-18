package codex

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/orchestration/client"
)

// readTestData reads a test fixture file from the testdata directory.
func readTestData(t *testing.T, filename string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", filename))
	require.NoError(t, err, "failed to read test fixture: %s", filename)
	return data
}

func TestParseEvent_ThreadStarted(t *testing.T) {
	// Test: thread.started -> EventSystem with SessionID extraction
	data := readTestData(t, "thread_started.json")

	event, err := ParseEvent(data)
	require.NoError(t, err)

	require.Equal(t, client.EventSystem, event.Type)
	require.Equal(t, "init", event.SubType)
	require.Equal(t, "0199a213-81c0-7800-8aa1-bbab2a035a53", event.SessionID)
	require.True(t, event.IsInit())
	require.False(t, event.IsAssistant())
	require.False(t, event.IsToolUse())
}

func TestParseEvent_ItemCompletedAgentMessage(t *testing.T) {
	// Test: item.completed (agent_message) -> EventAssistant with text
	data := readTestData(t, "item_completed_agent_message.json")

	event, err := ParseEvent(data)
	require.NoError(t, err)

	require.Equal(t, client.EventAssistant, event.Type)
	require.True(t, event.IsAssistant())
	require.NotNil(t, event.Message)
	require.Equal(t, "item_3", event.Message.ID)
	require.Equal(t, "assistant", event.Message.Role)
	require.Len(t, event.Message.Content, 1)
	require.Equal(t, "text", event.Message.Content[0].Type)
	require.Equal(t, "Repo contains docs, sdk, and examples directories.", event.Message.Content[0].Text)
	require.Equal(t, "Repo contains docs, sdk, and examples directories.", event.Message.GetText())
}

func TestParseEvent_ItemStartedCommand(t *testing.T) {
	// Test: item.started (command_execution) -> EventToolUse with command
	data := readTestData(t, "item_started_command.json")

	event, err := ParseEvent(data)
	require.NoError(t, err)

	require.Equal(t, client.EventToolUse, event.Type)
	require.True(t, event.IsToolUse())
	require.NotNil(t, event.Tool)
	require.Equal(t, "item_1", event.Tool.ID)
	require.Equal(t, "Bash", event.Tool.Name)
	require.Contains(t, string(event.Tool.Input), "bash -lc ls")
}

func TestParseEvent_ItemCompletedCommand(t *testing.T) {
	// Test: item.completed (command_execution) -> EventToolResult with output
	data := readTestData(t, "item_completed_command.json")

	event, err := ParseEvent(data)
	require.NoError(t, err)

	require.Equal(t, client.EventToolResult, event.Type)
	require.True(t, event.IsToolResult())
	require.NotNil(t, event.Tool)
	require.Equal(t, "item_1", event.Tool.ID)
	require.Equal(t, "Bash", event.Tool.Name)
	require.Contains(t, event.Tool.Output, "docs")
	require.Contains(t, event.Tool.Output, "examples")
	require.Contains(t, event.Tool.Output, "sdk")
	require.False(t, event.IsErrorResult)
}

func TestParseEvent_ItemCompletedCommandError(t *testing.T) {
	// Test: item.completed (command_execution) with non-zero exit code
	data := readTestData(t, "item_completed_command_error.json")

	event, err := ParseEvent(data)
	require.NoError(t, err)

	require.Equal(t, client.EventToolResult, event.Type)
	require.True(t, event.IsToolResult())
	require.NotNil(t, event.Tool)
	require.Equal(t, "item_2", event.Tool.ID)
	require.Contains(t, event.Tool.Output, "No such file or directory")
	require.True(t, event.IsErrorResult)
}

func TestParseEvent_TurnCompleted(t *testing.T) {
	// Test: turn.completed -> EventResult with usage (including cache token mapping)
	data := readTestData(t, "turn_completed.json")

	event, err := ParseEvent(data)
	require.NoError(t, err)

	require.Equal(t, client.EventResult, event.Type)
	require.True(t, event.IsResult())
	require.NotNil(t, event.Usage)

	// Verify token usage (TokensUsed = input_tokens + cached_input_tokens)
	require.Equal(t, 24763+24448, event.Usage.TokensUsed) // input + cached
	require.Equal(t, 200000, event.Usage.TotalTokens)     // default context window
	require.Equal(t, 122, event.Usage.OutputTokens)
}

func TestParseEvent_TurnFailed(t *testing.T) {
	// Test: turn.failed -> EventError (string format)
	data := readTestData(t, "turn_failed.json")

	event, err := ParseEvent(data)
	require.NoError(t, err)

	require.Equal(t, client.EventError, event.Type)
	require.True(t, event.IsError())
	require.NotNil(t, event.Error)
	require.Equal(t, "API rate limit exceeded", event.Error.Message)
}

func TestParseEvent_TurnFailedObjectError(t *testing.T) {
	// Test: turn.failed with object error format -> EventError
	// This is the format Codex uses for usage limit errors
	data := []byte(`{"type":"turn.failed","error":{"message":"You've hit your usage limit. Try again at 6:55 PM."}}`)

	event, err := ParseEvent(data)
	require.NoError(t, err)

	require.Equal(t, client.EventError, event.Type)
	require.True(t, event.IsError())
	require.NotNil(t, event.Error)
	require.Equal(t, "You've hit your usage limit. Try again at 6:55 PM.", event.Error.Message)
}

func TestParseEvent_ErrorEventTopLevelMessage(t *testing.T) {
	// Test: error event with top-level message field -> EventError
	// Format: {"type":"error","message":"..."}
	data := []byte(`{"type":"error","message":"You've hit your usage limit. Try again at 6:55 PM."}`)

	event, err := ParseEvent(data)
	require.NoError(t, err)

	require.Equal(t, client.EventError, event.Type)
	require.True(t, event.IsError())
	require.NotNil(t, event.Error)
	require.Equal(t, "You've hit your usage limit. Try again at 6:55 PM.", event.Error.Message)
}

func TestParseEvent_MalformedJSON(t *testing.T) {
	// Test: Malformed JSON handling (no panic)
	testCases := []struct {
		name string
		data string
	}{
		{"empty string", ""},
		{"not json", "this is not json"},
		{"incomplete json", `{"type":"thread`},
		{"invalid json structure", `{"type": [`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Should not panic
			_, err := ParseEvent([]byte(tc.data))
			require.Error(t, err)
		})
	}
}

func TestParseEvent_UnknownEventType(t *testing.T) {
	// Test: Unknown event type handling
	data := []byte(`{"type":"unknown.event.type","some_field":"value"}`)

	event, err := ParseEvent(data)
	require.NoError(t, err)

	// Unknown types should be passed through as-is
	require.Equal(t, client.EventType("unknown.event.type"), event.Type)
}

func TestGetContextTokens(t *testing.T) {
	// Test: GetContextTokens() returns TokensUsed
	data := readTestData(t, "turn_completed.json")

	event, err := ParseEvent(data)
	require.NoError(t, err)

	// GetContextTokens() now returns TokensUsed directly
	// TokensUsed = input_tokens + cached_input_tokens = 24763 + 24448 = 49211
	require.Equal(t, 24763+24448, event.GetContextTokens())
}

func TestParseEvent_RawDataPreserved(t *testing.T) {
	// Note: Raw data is now set by BaseProcess.parseOutput() after calling ParseEvent,
	// not by ParseEvent itself. This test verifies that ParseEvent parses correctly
	// and that Raw can be set externally (as BaseProcess does).
	data := readTestData(t, "thread_started.json")

	event, err := ParseEvent(data)
	require.NoError(t, err)

	// ParseEvent doesn't set Raw (BaseProcess does that after calling ParseEvent)
	// but we can verify the event parsed correctly
	require.Equal(t, "init", event.SubType)
	require.Equal(t, "0199a213-81c0-7800-8aa1-bbab2a035a53", event.SessionID)

	// Simulate what BaseProcess does after ParseEvent returns
	event.Raw = make([]byte, len(data))
	copy(event.Raw, data)

	require.NotNil(t, event.Raw)
	require.Contains(t, string(event.Raw), "thread.started")
	require.Contains(t, string(event.Raw), "0199a213-81c0-7800-8aa1-bbab2a035a53")
}

func TestParseEvent_TurnStarted(t *testing.T) {
	// Test: turn.started is handled (maps to system event)
	data := []byte(`{"type":"turn.started"}`)

	event, err := ParseEvent(data)
	require.NoError(t, err)

	require.Equal(t, client.EventSystem, event.Type)
}

func TestParseEvent_ErrorEvent(t *testing.T) {
	// Test: error event -> EventError
	data := []byte(`{"type":"error","error":"Connection timeout"}`)

	event, err := ParseEvent(data)
	require.NoError(t, err)

	require.Equal(t, client.EventError, event.Type)
	require.True(t, event.IsError())
	require.NotNil(t, event.Error)
	require.Equal(t, "Connection timeout", event.Error.Message)
}

func TestParseEvent_ItemStartedMCPToolCall(t *testing.T) {
	// Test: item.started (mcp_tool_call) -> EventToolUse
	// Uses real Codex format: "tool" instead of "tool_name", "arguments" instead of "tool_input"
	data := []byte(`{"type":"item.started","item":{"id":"mcp_1","type":"mcp_tool_call","server":"perles-worker","tool":"read_file","arguments":{"path":"/tmp/test.txt"},"status":"in_progress"}}`)

	event, err := ParseEvent(data)
	require.NoError(t, err)

	require.Equal(t, client.EventToolUse, event.Type)
	require.True(t, event.IsToolUse())
	require.NotNil(t, event.Tool)
	require.Equal(t, "mcp_1", event.Tool.ID)
	require.Equal(t, "read_file", event.Tool.Name)
	require.Contains(t, string(event.Tool.Input), "/tmp/test.txt")
}

func TestParseEvent_ItemCompletedMCPToolCall(t *testing.T) {
	// Test: item.completed (mcp_tool_call) -> EventToolResult
	// Uses real Codex format: "tool" and "result" with content array
	data := []byte(`{"type":"item.completed","item":{"id":"mcp_1","type":"mcp_tool_call","server":"perles-worker","tool":"read_file","result":{"content":[{"type":"text","text":"File contents here"}]}}}`)

	event, err := ParseEvent(data)
	require.NoError(t, err)

	require.Equal(t, client.EventToolResult, event.Type)
	require.True(t, event.IsToolResult())
	require.NotNil(t, event.Tool)
	require.Equal(t, "mcp_1", event.Tool.ID)
	require.Equal(t, "read_file", event.Tool.Name)
	require.Equal(t, "File contents here", event.Tool.Output)
}

func TestParseEvent_ReasoningEventIgnored(t *testing.T) {
	// Test: reasoning events are handled but not exposed as user-facing content
	data := []byte(`{"type":"item.completed","item":{"id":"reason_1","type":"reasoning","text":"Internal thinking process..."}}`)

	event, err := ParseEvent(data)
	require.NoError(t, err)

	// Event should parse successfully
	require.Equal(t, client.EventAssistant, event.Type)
	// But message content should not be set for reasoning items
	require.Nil(t, event.Message)
}

func TestParseEvent_ItemUpdated(t *testing.T) {
	// Test: item.updated for agent_message
	data := []byte(`{"type":"item.updated","item":{"id":"item_5","type":"agent_message","text":"Partial response..."}}`)

	event, err := ParseEvent(data)
	require.NoError(t, err)

	require.Equal(t, client.EventAssistant, event.Type)
	require.NotNil(t, event.Message)
	require.Equal(t, "Partial response...", event.Message.Content[0].Text)
}

func TestMapEventType(t *testing.T) {
	tests := []struct {
		name     string
		codexTyp string
		item     *codexItem
		expected client.EventType
	}{
		{
			name:     "thread.started",
			codexTyp: "thread.started",
			item:     nil,
			expected: client.EventSystem,
		},
		{
			name:     "turn.completed",
			codexTyp: "turn.completed",
			item:     nil,
			expected: client.EventResult,
		},
		{
			name:     "turn.started",
			codexTyp: "turn.started",
			item:     nil,
			expected: client.EventSystem,
		},
		{
			name:     "turn.failed",
			codexTyp: "turn.failed",
			item:     nil,
			expected: client.EventError,
		},
		{
			name:     "error",
			codexTyp: "error",
			item:     nil,
			expected: client.EventError,
		},
		{
			name:     "item.started command_execution",
			codexTyp: "item.started",
			item:     &codexItem{Type: "command_execution"},
			expected: client.EventToolUse,
		},
		{
			name:     "item.started mcp_tool_call",
			codexTyp: "item.started",
			item:     &codexItem{Type: "mcp_tool_call"},
			expected: client.EventToolUse,
		},
		{
			name:     "item.started agent_message",
			codexTyp: "item.started",
			item:     &codexItem{Type: "agent_message"},
			expected: client.EventAssistant,
		},
		{
			name:     "item.completed agent_message",
			codexTyp: "item.completed",
			item:     &codexItem{Type: "agent_message"},
			expected: client.EventAssistant,
		},
		{
			name:     "item.completed command_execution",
			codexTyp: "item.completed",
			item:     &codexItem{Type: "command_execution"},
			expected: client.EventToolResult,
		},
		{
			name:     "item.completed mcp_tool_call",
			codexTyp: "item.completed",
			item:     &codexItem{Type: "mcp_tool_call"},
			expected: client.EventToolResult,
		},
		{
			name:     "unknown type",
			codexTyp: "custom.event",
			item:     nil,
			expected: client.EventType("custom.event"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapEventType(tt.codexTyp, tt.item)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestParseEvent_NilUsage(t *testing.T) {
	// Test: turn.completed without usage field
	data := []byte(`{"type":"turn.completed"}`)

	event, err := ParseEvent(data)
	require.NoError(t, err)

	require.Equal(t, client.EventResult, event.Type)
	require.Nil(t, event.Usage)
	require.Equal(t, 0, event.GetContextTokens())
}

func TestParseEvent_ZeroExitCode(t *testing.T) {
	// Test: command with explicit exit_code: 0 should not be marked as error
	data := []byte(`{"type":"item.completed","item":{"id":"cmd_1","type":"command_execution","aggregated_output":"success","exit_code":0}}`)

	event, err := ParseEvent(data)
	require.NoError(t, err)

	require.False(t, event.IsErrorResult)
}

func TestParseEvent_NilExitCode(t *testing.T) {
	// Test: command without exit_code field
	data := []byte(`{"type":"item.completed","item":{"id":"cmd_1","type":"command_execution","aggregated_output":"output"}}`)

	event, err := ParseEvent(data)
	require.NoError(t, err)

	require.False(t, event.IsErrorResult)
}
