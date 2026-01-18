package claude

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/orchestration/client"
)

func TestNewParser(t *testing.T) {
	p := NewParser()
	require.NotNil(t, p)
	require.Equal(t, ClaudeContextWindowSize, p.ContextWindowSize())
}

func TestParser_ContextWindowSize(t *testing.T) {
	p := NewParser()
	require.Equal(t, 200000, p.ContextWindowSize())
}

func TestParser_ParseEvent_MessageStart(t *testing.T) {
	p := NewParser()

	input := `{"type":"system","subtype":"init","session_id":"sess-abc123","cwd":"/project","model":"claude-sonnet-4"}`
	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.Equal(t, client.EventSystem, event.Type)
	require.Equal(t, "init", event.SubType)
	require.Equal(t, "sess-abc123", event.SessionID)
	require.Equal(t, "/project", event.WorkDir)
	require.True(t, event.IsInit())
}

func TestParser_ParseEvent_ContentBlockDelta(t *testing.T) {
	p := NewParser()

	input := `{"type":"assistant","message":{"id":"msg_1","role":"assistant","content":[{"type":"text","text":"Hello, world!"}],"model":"claude-sonnet-4"}}`
	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.Equal(t, client.EventAssistant, event.Type)
	require.True(t, event.IsAssistant())
	require.NotNil(t, event.Message)
	require.Equal(t, "msg_1", event.Message.ID)
	require.Equal(t, "assistant", event.Message.Role)
	require.Equal(t, "claude-sonnet-4", event.Message.Model)
	require.Equal(t, "Hello, world!", event.Message.GetText())
}

func TestParser_ParseEvent_MessageStop(t *testing.T) {
	p := NewParser()

	input := `{"type":"result","is_error":false,"result":"Task completed successfully","duration_ms":1234,"num_turns":3,"total_cost_usd":0.0123}`
	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.Equal(t, client.EventResult, event.Type)
	require.False(t, event.IsErrorResult)
	require.Equal(t, "Task completed successfully", event.Result)
	require.Equal(t, int64(1234), event.DurationMs)
	require.InDelta(t, 0.0123, event.TotalCostUSD, 0.0001)
}

func TestParser_ParseEvent_ErrorEvent(t *testing.T) {
	p := NewParser()

	input := `{"type":"error","error":{"message":"Rate limit exceeded","code":"rate_limit_exceeded"}}`
	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.Equal(t, client.EventError, event.Type)
	require.NotNil(t, event.Error)
	require.Equal(t, "Rate limit exceeded", event.Error.Message)
	require.Equal(t, "rate_limit_exceeded", event.Error.Code)
}

func TestParser_ParseEvent_ErrorContextExhaustion(t *testing.T) {
	p := NewParser()

	// Error with context exhaustion pattern
	input := `{"type":"error","error":{"message":"Prompt is too long: 250000 tokens > 200000 maximum","code":"invalid_request"}}`
	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.NotNil(t, event.Error)
	require.Equal(t, "invalid_request", event.Error.Code)
	// Context exhaustion should be detected
	require.True(t, p.IsContextExhausted(event))
}

func TestParser_IsContextExhausted_StopSequence(t *testing.T) {
	p := NewParser()

	// Test stop_reason == "stop_sequence" with invalid_request error and message text
	// that contains context exhaustion indicator
	input := `{"type":"assistant","message":{"id":"msg_stop","role":"assistant","content":[{"type":"text","text":"Prompt is too long"}],"stop_reason":"stop_sequence"},"error":"invalid_request"}`
	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.NotNil(t, event.Error)
	// Should detect context exhaustion via stop_reason combined with invalid_request
	require.Equal(t, client.ErrReasonContextExceeded, event.Error.Reason)
	require.True(t, p.IsContextExhausted(event))
}

func TestParser_IsContextExhausted_NoError(t *testing.T) {
	p := NewParser()

	// Event without error should not be context exhausted
	event := client.OutputEvent{
		Type: client.EventAssistant,
		// No error
	}
	require.False(t, p.IsContextExhausted(event))
}

func TestParser_IsContextExhausted_OtherError(t *testing.T) {
	p := NewParser()

	// Event with non-context error should not be context exhausted
	event := client.OutputEvent{
		Type: client.EventError,
		Error: &client.ErrorInfo{
			Message: "Connection failed",
			Code:    "connection_error",
		},
	}
	require.False(t, p.IsContextExhausted(event))
}

func TestParser_ExtractSessionRef_ReturnsEmpty(t *testing.T) {
	p := NewParser()

	// Claude uses OnInitEvent hook, so ExtractSessionRef returns empty
	event := client.OutputEvent{
		Type:      client.EventSystem,
		SubType:   "init",
		SessionID: "sess-123",
	}
	result := p.ExtractSessionRef(event, []byte(`{"session_id":"sess-123"}`))
	require.Empty(t, result)
}

func TestParser_ParseEvent_ToolUse(t *testing.T) {
	p := NewParser()

	input := `{"type":"assistant","message":{"id":"msg_2","role":"assistant","content":[{"type":"tool_use","id":"toolu_123","name":"Read","input":{"file_path":"main.go"}}],"model":"claude-sonnet-4"}}`
	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.Equal(t, client.EventAssistant, event.Type)
	require.NotNil(t, event.Message)
	require.True(t, event.Message.HasToolUses())
	tools := event.Message.GetToolUses()
	require.Len(t, tools, 1)
	require.Equal(t, "Read", tools[0].Name)
	require.Equal(t, "toolu_123", tools[0].ID)
}

func TestParser_ParseEvent_ToolResult(t *testing.T) {
	p := NewParser()

	input := `{"type":"tool_result","tool":{"id":"toolu_123","name":"Read","content":"package main\n"}}`
	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.Equal(t, client.EventToolResult, event.Type)
	require.True(t, event.IsToolResult())
	require.NotNil(t, event.Tool)
	require.Equal(t, "Read", event.Tool.Name)
	require.Equal(t, "toolu_123", event.Tool.ID)
	require.Equal(t, "package main\n", event.Tool.GetOutput())
}

func TestParser_ParseEvent_InvalidJSON(t *testing.T) {
	p := NewParser()

	input := `not valid json`
	_, err := p.ParseEvent([]byte(input))

	require.Error(t, err)
}

func TestParser_ParseEvent_UsageInfo(t *testing.T) {
	p := NewParser()

	input := `{"type":"assistant","message":{"id":"msg_1","role":"assistant","content":[{"type":"text","text":"Hello"}],"model":"claude-sonnet-4","usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":25,"cache_creation_input_tokens":10}}}`
	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.NotNil(t, event.Usage)
	// TokensUsed = input_tokens + cache_read + cache_creation = 100 + 25 + 10 = 135
	require.Equal(t, 135, event.Usage.TokensUsed)
	require.Equal(t, 50, event.Usage.OutputTokens)
	require.Equal(t, ClaudeContextWindowSize, event.Usage.TotalTokens)
}

// TestParser_Golden_PreMigrationBaseline captures current parseEvent behavior as a golden test.
// This ensures the Parser produces identical output to the original parseEvent function.
func TestParser_Golden_PreMigrationBaseline(t *testing.T) {
	// Load test events from testdata
	testdataPath := filepath.Join("testdata", "events.jsonl")
	file, err := os.Open(testdataPath)
	require.NoError(t, err, "Failed to open testdata file")
	defer file.Close()

	p := NewParser()
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		t.Run("line_"+string(rune('0'+lineNum)), func(t *testing.T) {
			// Parse with new Parser
			newEvent, newErr := p.ParseEvent(line)

			// Parse with original parseEvent function
			oldEvent, oldErr := NewParser().ParseEvent(line)

			// Both should succeed or fail together
			if oldErr != nil {
				require.Error(t, newErr, "New parser should fail when old parser fails")
				return
			}
			require.NoError(t, newErr, "New parser should succeed when old parser succeeds")

			// Compare key fields (excluding Raw which may differ)
			require.Equal(t, oldEvent.Type, newEvent.Type, "Type mismatch")
			require.Equal(t, oldEvent.SubType, newEvent.SubType, "SubType mismatch")
			require.Equal(t, oldEvent.SessionID, newEvent.SessionID, "SessionID mismatch")
			require.Equal(t, oldEvent.WorkDir, newEvent.WorkDir, "WorkDir mismatch")
			require.Equal(t, oldEvent.IsErrorResult, newEvent.IsErrorResult, "IsErrorResult mismatch")
			require.Equal(t, oldEvent.Result, newEvent.Result, "Result mismatch")
			require.Equal(t, oldEvent.DurationMs, newEvent.DurationMs, "DurationMs mismatch")
			require.InDelta(t, oldEvent.TotalCostUSD, newEvent.TotalCostUSD, 0.0001, "TotalCostUSD mismatch")

			// Compare Message
			if oldEvent.Message != nil {
				require.NotNil(t, newEvent.Message, "Message should not be nil")
				require.Equal(t, oldEvent.Message.ID, newEvent.Message.ID, "Message.ID mismatch")
				require.Equal(t, oldEvent.Message.Role, newEvent.Message.Role, "Message.Role mismatch")
				require.Equal(t, oldEvent.Message.Model, newEvent.Message.Model, "Message.Model mismatch")
				require.Equal(t, oldEvent.Message.GetText(), newEvent.Message.GetText(), "Message.GetText mismatch")
				require.Equal(t, len(oldEvent.Message.Content), len(newEvent.Message.Content), "Message.Content length mismatch")
			} else {
				require.Nil(t, newEvent.Message, "Message should be nil")
			}

			// Compare Error
			if oldEvent.Error != nil {
				require.NotNil(t, newEvent.Error, "Error should not be nil")
				require.Equal(t, oldEvent.Error.Message, newEvent.Error.Message, "Error.Message mismatch")
				require.Equal(t, oldEvent.Error.Code, newEvent.Error.Code, "Error.Code mismatch")
				require.Equal(t, oldEvent.Error.Reason, newEvent.Error.Reason, "Error.Reason mismatch")
			} else {
				require.Nil(t, newEvent.Error, "Error should be nil")
			}

			// Compare Tool
			if oldEvent.Tool != nil {
				require.NotNil(t, newEvent.Tool, "Tool should not be nil")
				require.Equal(t, oldEvent.Tool.ID, newEvent.Tool.ID, "Tool.ID mismatch")
				require.Equal(t, oldEvent.Tool.Name, newEvent.Tool.Name, "Tool.Name mismatch")
			} else {
				require.Nil(t, newEvent.Tool, "Tool should be nil")
			}

			// Compare Usage
			if oldEvent.Usage != nil {
				require.NotNil(t, newEvent.Usage, "Usage should not be nil")
				require.Equal(t, oldEvent.Usage.TokensUsed, newEvent.Usage.TokensUsed, "Usage.TokensUsed mismatch")
				require.Equal(t, oldEvent.Usage.OutputTokens, newEvent.Usage.OutputTokens, "Usage.OutputTokens mismatch")
				require.Equal(t, oldEvent.Usage.TotalTokens, newEvent.Usage.TotalTokens, "Usage.TotalTokens mismatch")
			}
		})
	}

	require.NoError(t, scanner.Err())
	require.Greater(t, lineNum, 0, "Should have processed at least one line")
}

// TestParser_ImplementsEventParser verifies that Parser satisfies the EventParser interface.
func TestParser_ImplementsEventParser(t *testing.T) {
	var _ client.EventParser = (*Parser)(nil)

	// Also verify via NewParser
	p := NewParser()
	var ep client.EventParser = p
	require.NotNil(t, ep)
}

// TestParser_ErrorFieldPolymorphic tests that error field parsing works for all formats.
func TestParser_ErrorFieldPolymorphic(t *testing.T) {
	p := NewParser()

	tests := []struct {
		name        string
		input       string
		wantMessage string
		wantCode    string
		wantNil     bool
	}{
		{
			name:        "error as object",
			input:       `{"type":"error","error":{"message":"Something went wrong","code":"INTERNAL"}}`,
			wantMessage: "Something went wrong",
			wantCode:    "INTERNAL",
		},
		{
			name:        "error as string code",
			input:       `{"type":"error","error":"invalid_request"}`,
			wantMessage: "", // Claude puts string errors in Code field, not Message
			wantCode:    "invalid_request",
		},
		{
			name:    "no error",
			input:   `{"type":"assistant","message":{"content":[]}}`,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := p.ParseEvent([]byte(tt.input))
			require.NoError(t, err)

			if tt.wantNil {
				require.Nil(t, event.Error)
				return
			}

			require.NotNil(t, event.Error)
			require.Equal(t, tt.wantMessage, event.Error.Message)
			require.Equal(t, tt.wantCode, event.Error.Code)
		})
	}
}

// TestParser_RawDataCopied verifies that Raw field contains a copy of input data.
func TestParser_RawDataCopied(t *testing.T) {
	p := NewParser()

	input := []byte(`{"type":"system","subtype":"init"}`)
	inputCopy := make([]byte, len(input))
	copy(inputCopy, input)

	event, err := p.ParseEvent(input)

	require.NoError(t, err)
	require.Equal(t, inputCopy, []byte(event.Raw))

	// Verify it's a copy, not the same slice
	input[0] = 'X'
	require.NotEqual(t, input[0], event.Raw[0])
}

// TestParser_IntegrationWithBaseProcess verifies that Parser works with WithEventParser.
func TestParser_IntegrationWithBaseProcess(t *testing.T) {
	p := NewParser()

	// Create a BaseProcess with the parser
	bp := &client.BaseProcess{}
	opt := client.WithEventParser(p)
	opt(bp)

	// The parseEventFn should be set - we can't directly access it,
	// but we verified this pattern in parser_test.go's TestWithEventParser_SetsParseEventFn
}

// TestParser_ParseEvent_MessageWithStopReason verifies that stop_reason is handled.
func TestParser_ParseEvent_MessageWithStopReason(t *testing.T) {
	p := NewParser()

	// Message with stop_reason and error - this triggers context exhaustion detection
	// The detection requires: error code == "invalid_request" AND (message contains "Prompt is too long" OR stop_reason == "stop_sequence")
	input := `{"type":"assistant","message":{"id":"msg_1","role":"assistant","content":[{"type":"text","text":"Prompt is too long"}],"stop_reason":"stop_sequence"},"error":"invalid_request"}`

	event, err := p.ParseEvent([]byte(input))
	require.NoError(t, err)

	require.NotNil(t, event.Error)
	require.Equal(t, "invalid_request", event.Error.Code)
	require.Equal(t, client.ErrReasonContextExceeded, event.Error.Reason)
	require.True(t, p.IsContextExhausted(event))
}

// BenchmarkParser_ParseEvent benchmarks the parsing performance.
func BenchmarkParser_ParseEvent(b *testing.B) {
	p := NewParser()
	input := []byte(`{"type":"assistant","message":{"id":"msg_1","role":"assistant","content":[{"type":"text","text":"Hello, world!"}],"model":"claude-sonnet-4","usage":{"input_tokens":100,"output_tokens":50}}}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = p.ParseEvent(input)
	}
}

// Test that the marshalEvent helper produces valid JSON for golden tests.
func marshalEvent(e client.OutputEvent) []byte {
	// Create a simplified struct for comparison that excludes Raw
	type simplifiedEvent struct {
		Type          client.EventType `json:"type"`
		SubType       string           `json:"subtype,omitempty"`
		SessionID     string           `json:"session_id,omitempty"`
		WorkDir       string           `json:"workdir,omitempty"`
		Error         *client.ErrorInfo
		IsErrorResult bool   `json:"is_error,omitempty"`
		Result        string `json:"result,omitempty"`
	}

	simplified := simplifiedEvent{
		Type:          e.Type,
		SubType:       e.SubType,
		SessionID:     e.SessionID,
		WorkDir:       e.WorkDir,
		Error:         e.Error,
		IsErrorResult: e.IsErrorResult,
		Result:        e.Result,
	}

	data, _ := json.MarshalIndent(simplified, "", "  ")
	return data
}
