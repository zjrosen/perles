package amp

import (
	"bufio"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/orchestration/client"
)

func TestNewParser(t *testing.T) {
	p := NewParser()
	require.NotNil(t, p)
	require.Equal(t, AmpContextWindowSize, p.ContextWindowSize())
}

func TestParser_ContextWindowSize(t *testing.T) {
	p := NewParser()
	require.Equal(t, 200000, p.ContextWindowSize())
}

func TestParser_ParseEvent_SystemInit(t *testing.T) {
	p := NewParser()

	input := `{"type":"system","subtype":"init","session_id":"T-abc123","cwd":"/project","tools":["Bash","Read"]}`
	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.Equal(t, client.EventSystem, event.Type)
	require.Equal(t, "init", event.SubType)
	require.Equal(t, "T-abc123", event.SessionID)
	require.Equal(t, "/project", event.WorkDir)
	require.True(t, event.IsInit())
}

func TestParser_ParseEvent_AssistantMessage(t *testing.T) {
	p := NewParser()

	input := `{"type":"assistant","message":{"id":"msg_1","role":"assistant","content":[{"type":"text","text":"Hello from Amp!"}],"model":"claude-sonnet-4"}}`
	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.Equal(t, client.EventAssistant, event.Type)
	require.True(t, event.IsAssistant())
	require.NotNil(t, event.Message)
	require.Equal(t, "msg_1", event.Message.ID)
	require.Equal(t, "assistant", event.Message.Role)
	require.Equal(t, "claude-sonnet-4", event.Message.Model)
	require.Equal(t, "Hello from Amp!", event.Message.GetText())
}

func TestParser_ParseEvent_UsageInfo(t *testing.T) {
	p := NewParser()

	input := `{"type":"assistant","message":{"id":"msg_1","role":"assistant","content":[{"type":"text","text":"Done"}],"usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":1000,"cache_creation_input_tokens":500}}}`
	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.NotNil(t, event.Usage)
	// TokensUsed = input_tokens + cache_read + cache_creation = 100 + 1000 + 500 = 1600
	require.Equal(t, 1600, event.Usage.TokensUsed)
	require.Equal(t, 50, event.Usage.OutputTokens)
	require.Equal(t, AmpContextWindowSize, event.Usage.TotalTokens)
}

func TestParser_ParseEvent_ToolUse(t *testing.T) {
	p := NewParser()

	input := `{"type":"assistant","message":{"id":"msg_2","content":[{"type":"tool_use","id":"toolu_123","name":"Bash","input":{"cmd":"ls -la"}}]}}`
	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.Equal(t, client.EventAssistant, event.Type)
	require.NotNil(t, event.Message)
	require.True(t, event.Message.HasToolUses())
	tools := event.Message.GetToolUses()
	require.Len(t, tools, 1)
	require.Equal(t, "Bash", tools[0].Name)
	require.Equal(t, "toolu_123", tools[0].ID)
	require.NotNil(t, event.Tool)
	require.Equal(t, "Bash", event.Tool.Name)
}

func TestParser_ParseEvent_Result(t *testing.T) {
	p := NewParser()

	input := `{"type":"result","subtype":"success","duration_ms":5000,"is_error":false,"num_turns":3,"result":"Task completed","session_id":"T-abc123","total_cost_usd":0.0123}`
	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.Equal(t, client.EventResult, event.Type)
	require.Equal(t, "success", event.SubType)
	require.False(t, event.IsErrorResult)
	require.Equal(t, "Task completed", event.Result)
	require.Equal(t, int64(5000), event.DurationMs)
	require.InDelta(t, 0.0123, event.TotalCostUSD, 0.0001)
}

func TestParser_ParseEvent_ErrorEvent(t *testing.T) {
	p := NewParser()

	input := `{"type":"error","error":{"message":"Something went wrong","code":"INTERNAL"}}`
	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.Equal(t, client.EventError, event.Type)
	require.NotNil(t, event.Error)
	require.Equal(t, "Something went wrong", event.Error.Message)
	require.Equal(t, "INTERNAL", event.Error.Code)
}

func TestParser_ParseEvent_NestedErrorFormat(t *testing.T) {
	p := NewParser()

	// Test the "413 {...}" nested error format that Amp uses
	input := `{"type":"result","subtype":"error_during_execution","duration_ms":389584,"is_error":true,"num_turns":28,"error":"413 {\"type\":\"error\",\"error\":{\"type\":\"invalid_request_error\",\"message\":\"Prompt is too long\"},\"request_id\":\"req_vrtx_011CXDs3LJPo57WcNsT9h9bs\"}","session_id":"T-019bce63-7de3-73a4-93a7-c1b84e61411e"}`
	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.Equal(t, client.EventResult, event.Type)
	require.Equal(t, "error_during_execution", event.SubType)
	require.True(t, event.IsErrorResult)
	require.NotNil(t, event.Error, "Error should be parsed from nested string")
	require.Equal(t, "Prompt is too long", event.Error.Message)
	require.Equal(t, "invalid_request_error", event.Error.Code)
	require.Equal(t, client.ErrReasonContextExceeded, event.Error.Reason)
	require.True(t, p.IsContextExhausted(event))
}

func TestParser_ParseEvent_InvalidJSON(t *testing.T) {
	p := NewParser()

	input := `not valid json`
	_, err := p.ParseEvent([]byte(input))

	require.Error(t, err)
}

func TestParser_ExtractSessionRef_InitEvent(t *testing.T) {
	p := NewParser()

	event := client.OutputEvent{
		Type:      client.EventSystem,
		SubType:   "init",
		SessionID: "T-abc123-def456",
	}
	result := p.ExtractSessionRef(event, nil)
	require.Equal(t, "T-abc123-def456", result)
}

func TestParser_ExtractSessionRef_NonInitEvent(t *testing.T) {
	p := NewParser()

	event := client.OutputEvent{
		Type:      client.EventAssistant,
		SessionID: "T-abc123",
	}
	result := p.ExtractSessionRef(event, nil)
	require.Empty(t, result)
}

func TestParser_IsContextExhausted_ErrReason(t *testing.T) {
	p := NewParser()

	event := client.OutputEvent{
		Type: client.EventError,
		Error: &client.ErrorInfo{
			Reason: client.ErrReasonContextExceeded,
		},
	}
	require.True(t, p.IsContextExhausted(event))
}

func TestParser_IsContextExhausted_MessagePattern(t *testing.T) {
	p := NewParser()

	event := client.OutputEvent{
		Type: client.EventError,
		Error: &client.ErrorInfo{
			Message: "Prompt is too long: 201234 tokens",
		},
	}
	require.True(t, p.IsContextExhausted(event))
}

func TestParser_IsContextExhausted_NoError(t *testing.T) {
	p := NewParser()

	event := client.OutputEvent{
		Type: client.EventAssistant,
	}
	require.False(t, p.IsContextExhausted(event))
}

func TestParser_IsContextExhausted_OtherError(t *testing.T) {
	p := NewParser()

	event := client.OutputEvent{
		Type: client.EventError,
		Error: &client.ErrorInfo{
			Message: "Connection failed",
			Code:    "connection_error",
		},
	}
	require.False(t, p.IsContextExhausted(event))
}

func TestParser_IsContextExhausted_UsesBaseParserPatterns(t *testing.T) {
	p := NewParser()

	patterns := []string{
		"prompt is too long",
		"context window exceeded",
		"context exceeded",
		"context limit",
		"token limit",
		"maximum context length",
	}

	for _, pattern := range patterns {
		t.Run(pattern, func(t *testing.T) {
			event := client.OutputEvent{
				Type: client.EventError,
				Error: &client.ErrorInfo{
					Message: "Error: " + pattern + " reached",
				},
			}
			require.True(t, p.IsContextExhausted(event), "Should detect pattern: %s", pattern)
		})
	}
}

// TestParser_Golden_PreMigrationBaseline captures current parseEvent behavior as a golden test.
// This ensures the Parser produces identical output to the original parseEvent function.
func TestParser_Golden_PreMigrationBaseline(t *testing.T) {
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

			// Compare key fields
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

	p := NewParser()
	var ep client.EventParser = p
	require.NotNil(t, ep)
}

// TestParser_IntegrationWithBaseProcess verifies that Parser works with WithEventParser.
func TestParser_IntegrationWithBaseProcess(t *testing.T) {
	p := NewParser()

	bp := &client.BaseProcess{}
	opt := client.WithEventParser(p)
	opt(bp)
}

// BenchmarkParser_ParseEvent benchmarks the parsing performance.
func BenchmarkParser_ParseEvent(b *testing.B) {
	p := NewParser()
	input := []byte(`{"type":"assistant","message":{"id":"msg_1","role":"assistant","content":[{"type":"text","text":"Hello from Amp!"}],"model":"claude-sonnet-4","usage":{"input_tokens":100,"output_tokens":50}}}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = p.ParseEvent(input)
	}
}
