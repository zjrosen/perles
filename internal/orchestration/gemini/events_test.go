package gemini

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

func TestParseEvent_Init(t *testing.T) {
	// Test: init -> EventSystem with SessionID extraction
	data := readTestData(t, "init.json")

	event, err := NewParser().ParseEvent(data)
	require.NoError(t, err)

	require.Equal(t, client.EventSystem, event.Type)
	require.Equal(t, "init", event.SubType)
	require.Equal(t, "gemini-sess-abc123", event.SessionID)
	require.True(t, event.IsInit())
	require.False(t, event.IsAssistant())
	require.False(t, event.IsToolUse())
}

func TestParseEvent_MessageAssistant(t *testing.T) {
	// Test: message (role: assistant) -> EventAssistant with text content
	data := readTestData(t, "message.json")

	event, err := NewParser().ParseEvent(data)
	require.NoError(t, err)

	require.Equal(t, client.EventAssistant, event.Type)
	require.True(t, event.IsAssistant())
	require.NotNil(t, event.Message)
	require.Equal(t, "assistant", event.Message.Role)
	require.Len(t, event.Message.Content, 1)
	require.Equal(t, "text", event.Message.Content[0].Type)
	require.Equal(t, "I'll help you analyze the codebase structure.", event.Message.Content[0].Text)
	require.Equal(t, "I'll help you analyze the codebase structure.", event.Message.GetText())
}

func TestParseEvent_MessageUser(t *testing.T) {
	// Test: message (role: user) -> EventToolResult (user messages are mapped differently)
	data := []byte(`{"type":"message","role":"user","content":"User input here"}`)

	event, err := NewParser().ParseEvent(data)
	require.NoError(t, err)

	// User messages are mapped to EventToolResult and don't populate Message
	require.Equal(t, client.EventToolResult, event.Type)
	require.Nil(t, event.Message) // User messages don't populate Message content
}

func TestParseEvent_MessageAssistantWithDelta(t *testing.T) {
	// Test: message with delta:true indicates streaming chunk
	data := []byte(`{"type":"message","role":"assistant","content":"Hello","delta":true}`)

	event, err := NewParser().ParseEvent(data)
	require.NoError(t, err)

	require.Equal(t, client.EventAssistant, event.Type)
	require.True(t, event.Delta, "Delta should be true for streaming chunks")
	require.NotNil(t, event.Message)
	require.Equal(t, "Hello", event.Message.Content[0].Text)
}

func TestParseEvent_MessageAssistantWithoutDelta(t *testing.T) {
	// Test: message without delta field (or delta:false) is a complete message
	data := []byte(`{"type":"message","role":"assistant","content":"Complete message","delta":false}`)

	event, err := NewParser().ParseEvent(data)
	require.NoError(t, err)

	require.Equal(t, client.EventAssistant, event.Type)
	require.False(t, event.Delta, "Delta should be false for complete messages")
	require.NotNil(t, event.Message)
	require.Equal(t, "Complete message", event.Message.Content[0].Text)
}

func TestParseEvent_MessageAssistantDeltaOmitted(t *testing.T) {
	// Test: message without delta field defaults to false
	data := readTestData(t, "message.json") // Uses existing fixture without delta field

	event, err := NewParser().ParseEvent(data)
	require.NoError(t, err)

	require.Equal(t, client.EventAssistant, event.Type)
	require.False(t, event.Delta, "Delta should default to false when omitted")
}

func TestParseEvent_ToolUse(t *testing.T) {
	// Test: tool_use -> EventToolUse with tool name and parameters
	data := readTestData(t, "tool_use.json")

	event, err := NewParser().ParseEvent(data)
	require.NoError(t, err)

	require.Equal(t, client.EventToolUse, event.Type)
	require.True(t, event.IsToolUse())
	require.NotNil(t, event.Tool)
	require.Equal(t, "tool_1", event.Tool.ID)
	require.Equal(t, "Read", event.Tool.Name)
	require.Contains(t, string(event.Tool.Input), "/tmp/test.go")

	// Also verify Message.Content is populated for display
	require.NotNil(t, event.Message)
	require.Equal(t, "assistant", event.Message.Role)
	require.Len(t, event.Message.Content, 1)
	require.Equal(t, "tool_use", event.Message.Content[0].Type)
	require.Equal(t, "Read", event.Message.Content[0].Name)
}

func TestParseEvent_ToolUse_TopLevelFormat(t *testing.T) {
	// Test: tool_use with top-level format (current Gemini format)
	// This format has tool_name, tool_id, and parameters at top level
	data := []byte(`{"type":"tool_use","timestamp":"2026-01-15T02:43:46.538Z","tool_name":"run_shell_command","tool_id":"run_shell_command-1768445026538-e87fd6c2461a7","parameters":{"description":"Show issues","command":"bd ready -n 5"}}`)

	event, err := NewParser().ParseEvent(data)
	require.NoError(t, err)

	require.Equal(t, client.EventToolUse, event.Type)
	require.True(t, event.IsToolUse())
	require.NotNil(t, event.Tool)
	require.Equal(t, "run_shell_command-1768445026538-e87fd6c2461a7", event.Tool.ID)
	require.Equal(t, "run_shell_command", event.Tool.Name)
	require.Contains(t, string(event.Tool.Input), "bd ready")

	// Also verify Message.Content is populated for display
	require.NotNil(t, event.Message)
	require.Equal(t, "assistant", event.Message.Role)
	require.Len(t, event.Message.Content, 1)
	require.Equal(t, "tool_use", event.Message.Content[0].Type)
	require.Equal(t, "run_shell_command", event.Message.Content[0].Name)
}

func TestParseEvent_ToolResult_TopLevelFormat(t *testing.T) {
	// Test: tool_result with top-level format (current Gemini format)
	data := []byte(`{"type":"tool_result","timestamp":"2026-01-15T02:43:51.808Z","tool_id":"run_shell_command-1768445026538-e87fd6c2461a7","status":"success","output":"No open issues"}`)

	event, err := NewParser().ParseEvent(data)
	require.NoError(t, err)

	require.Equal(t, client.EventToolResult, event.Type)
	require.True(t, event.IsToolResult())
	require.NotNil(t, event.Tool)
	require.Equal(t, "run_shell_command-1768445026538-e87fd6c2461a7", event.Tool.ID)
	require.Contains(t, event.Tool.Output, "No open issues")
	require.False(t, event.IsErrorResult)
}

func TestParseEvent_ToolResult(t *testing.T) {
	// Test: tool_result -> EventToolResult with status and output
	data := readTestData(t, "tool_result.json")

	event, err := NewParser().ParseEvent(data)
	require.NoError(t, err)

	require.Equal(t, client.EventToolResult, event.Type)
	require.True(t, event.IsToolResult())
	require.NotNil(t, event.Tool)
	require.Equal(t, "tool_1", event.Tool.ID)
	// Note: tool_result events in Gemini format don't include tool_name
	require.Contains(t, event.Tool.Output, "package main")
	require.Contains(t, event.Tool.Output, "Hello")
	require.False(t, event.IsErrorResult)
}

func TestParseEvent_ToolResultError(t *testing.T) {
	// Test: tool_result with error status
	data := []byte(`{"type":"tool_result","tool_id":"tool_2","status":"error","output":"command not found"}`)

	event, err := NewParser().ParseEvent(data)
	require.NoError(t, err)

	require.Equal(t, client.EventToolResult, event.Type)
	require.NotNil(t, event.Tool)
	require.Equal(t, "command not found", event.Tool.Output)
	require.True(t, event.IsErrorResult)
}

func TestParseEvent_ToolResultFailed(t *testing.T) {
	// Test: tool_result with failed status
	data := []byte(`{"type":"tool_result","tool_id":"tool_3","status":"failed","output":"permission denied"}`)

	event, err := NewParser().ParseEvent(data)
	require.NoError(t, err)

	require.Equal(t, client.EventToolResult, event.Type)
	require.True(t, event.IsErrorResult)
}

func TestParseEvent_Result(t *testing.T) {
	// Test: result -> EventResult with usage metrics
	data := readTestData(t, "result.json")

	event, err := NewParser().ParseEvent(data)
	require.NoError(t, err)

	require.Equal(t, client.EventResult, event.Type)
	require.True(t, event.IsResult())
	require.NotNil(t, event.Usage)

	// TokensUsed = tokens_prompt + tokens_cached = 5000 + 2000 = 7000
	require.Equal(t, 7000, event.Usage.TokensUsed)
	// Gemini has 1M token context window
	require.Equal(t, 1000000, event.Usage.TotalTokens)
	require.Equal(t, 1500, event.Usage.OutputTokens)
	require.Equal(t, int64(3500), event.DurationMs)
}

func TestParseEvent_Error(t *testing.T) {
	// Test: error -> EventError with message and code
	data := readTestData(t, "error.json")

	event, err := NewParser().ParseEvent(data)
	require.NoError(t, err)

	require.Equal(t, client.EventError, event.Type)
	require.True(t, event.IsError())
	require.NotNil(t, event.Error)
	require.Equal(t, "Rate limit exceeded", event.Error.Message)
	require.Equal(t, "RATE_LIMIT", event.Error.Code)
}

func TestParseEvent_ErrorWithoutCode(t *testing.T) {
	// Test: error without code field
	data := []byte(`{"type":"error","error":{"message":"Connection timeout"}}`)

	event, err := NewParser().ParseEvent(data)
	require.NoError(t, err)

	require.Equal(t, client.EventError, event.Type)
	require.NotNil(t, event.Error)
	require.Equal(t, "Connection timeout", event.Error.Message)
	require.Empty(t, event.Error.Code)
}

func TestParseEvent_UnknownEventType(t *testing.T) {
	// Test: Unknown event type is passed through
	data := []byte(`{"type":"custom.event.type","some_field":"value"}`)

	event, err := NewParser().ParseEvent(data)
	require.NoError(t, err)

	// Unknown types should be passed through as-is
	require.Equal(t, client.EventType("custom.event.type"), event.Type)
}

func TestParseEvent_MalformedJSON(t *testing.T) {
	// Test: Malformed JSON handling
	testCases := []struct {
		name string
		data string
	}{
		{"empty string", ""},
		{"not json", "this is not json"},
		{"incomplete json", `{"type":"init`},
		{"invalid json structure", `{"type": [`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewParser().ParseEvent([]byte(tc.data))
			require.Error(t, err)
		})
	}
}

func TestParseEvent_EmptyLine(t *testing.T) {
	// Test: Empty line handling (returns error for invalid JSON)
	_, err := NewParser().ParseEvent([]byte(""))
	require.Error(t, err)

	// Whitespace only also fails
	_, err = NewParser().ParseEvent([]byte("   "))
	require.Error(t, err)
}

func TestParseEvent_RawDataPreserved(t *testing.T) {
	// Test: Raw data is preserved for debugging
	data := readTestData(t, "init.json")

	event, err := NewParser().ParseEvent(data)
	require.NoError(t, err)

	require.NotNil(t, event.Raw)
	require.Contains(t, string(event.Raw), "gemini-sess-abc123")
	require.Contains(t, string(event.Raw), "gemini-2.5-pro")
}

func TestParseEvent_ResultWithoutStats(t *testing.T) {
	// Test: result event without stats field
	data := []byte(`{"type":"result"}`)

	event, err := NewParser().ParseEvent(data)
	require.NoError(t, err)

	require.Equal(t, client.EventResult, event.Type)
	require.Nil(t, event.Usage)
	require.Equal(t, 0, event.GetContextTokens())
}

func TestParseEvent_ToolUseWithoutTool(t *testing.T) {
	// Test: tool_use event without tool field (edge case)
	data := []byte(`{"type":"tool_use"}`)

	event, err := NewParser().ParseEvent(data)
	require.NoError(t, err)

	require.Equal(t, client.EventToolUse, event.Type)
	require.Nil(t, event.Tool)
}

func TestMapEventType(t *testing.T) {
	tests := []struct {
		name       string
		geminiType string
		role       string
		expected   client.EventType
	}{
		{
			name:       "init",
			geminiType: "init",
			role:       "",
			expected:   client.EventSystem,
		},
		{
			name:       "message assistant",
			geminiType: "message",
			role:       "assistant",
			expected:   client.EventAssistant,
		},
		{
			name:       "message user",
			geminiType: "message",
			role:       "user",
			expected:   client.EventToolResult,
		},
		{
			name:       "message no role",
			geminiType: "message",
			role:       "",
			expected:   client.EventToolResult,
		},
		{
			name:       "tool_use",
			geminiType: "tool_use",
			role:       "",
			expected:   client.EventToolUse,
		},
		{
			name:       "tool_result",
			geminiType: "tool_result",
			role:       "",
			expected:   client.EventToolResult,
		},
		{
			name:       "result",
			geminiType: "result",
			role:       "",
			expected:   client.EventResult,
		},
		{
			name:       "error",
			geminiType: "error",
			role:       "",
			expected:   client.EventError,
		},
		{
			name:       "unknown type",
			geminiType: "custom.event",
			role:       "",
			expected:   client.EventType("custom.event"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapEventType(tt.geminiType, tt.role)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestGetContextTokens(t *testing.T) {
	// Test: GetContextTokens() returns TokensUsed
	data := readTestData(t, "result.json")

	event, err := NewParser().ParseEvent(data)
	require.NoError(t, err)

	// TokensUsed = tokens_prompt + tokens_cached = 5000 + 2000 = 7000
	require.Equal(t, 7000, event.GetContextTokens())
}
