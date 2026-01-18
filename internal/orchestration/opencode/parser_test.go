package opencode

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/orchestration/client"
)

func TestParser_NewParser(t *testing.T) {
	p := NewParser()
	require.NotNil(t, p, "NewParser should return non-nil parser")
}

func TestParser_ContextWindowSize(t *testing.T) {
	p := NewParser()
	size := p.ContextWindowSize()
	require.Equal(t, 200000, size, "ContextWindowSize should return 200000 for OpenCode")
}

func TestParser_ParseEvent_AssistantMessage(t *testing.T) {
	p := NewParser()

	// OpenCode text event with camelCase sessionID
	input := `{"type":"text","timestamp":1737161234567,"sessionID":"ses_abc123","part":{"type":"text","text":"Hello, I can help with that."}}`

	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.Equal(t, client.EventAssistant, event.Type)
	require.Equal(t, "ses_abc123", event.SessionID)
	require.NotNil(t, event.Message)
	require.Equal(t, "assistant", event.Message.Role)
	require.Len(t, event.Message.Content, 1)
	require.Equal(t, "text", event.Message.Content[0].Type)
	require.Equal(t, "Hello, I can help with that.", event.Message.Content[0].Text)
}

func TestParser_ParseEvent_CamelCaseJSONFields(t *testing.T) {
	p := NewParser()

	tests := []struct {
		name          string
		input         string
		wantSessionID string
	}{
		{
			name:          "sessionID in root",
			input:         `{"type":"text","sessionID":"ses_root123","part":{"type":"text","text":"test"}}`,
			wantSessionID: "ses_root123",
		},
		{
			name:          "callID in tool_use",
			input:         `{"type":"tool_use","sessionID":"ses_tool456","part":{"type":"tool","tool":"bash","callID":"call_789"}}`,
			wantSessionID: "ses_tool456",
		},
		{
			name:          "messageID in part",
			input:         `{"type":"text","sessionID":"ses_msg789","part":{"type":"text","text":"test","messageID":"msg_001"}}`,
			wantSessionID: "ses_msg789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := p.ParseEvent([]byte(tt.input))
			require.NoError(t, err)
			require.Equal(t, tt.wantSessionID, event.SessionID, "sessionID should be parsed from camelCase field")
		})
	}
}

func TestParser_ParseEvent_ErrorEvent(t *testing.T) {
	p := NewParser()

	// OpenCode error event
	input := `{"type":"error","sessionID":"ses_err123","error":{"code":"invalid_request","message":"Something went wrong"}}`

	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.Equal(t, client.EventError, event.Type)
	require.NotNil(t, event.Error)
	require.Equal(t, "invalid_request", event.Error.Code)
	require.Equal(t, "Something went wrong", event.Error.Message)
}

func TestParser_ParseEvent_ErrorWithTopLevelMessage(t *testing.T) {
	p := NewParser()

	// Some OpenCode errors have message at top level
	input := `{"type":"error","message":"Connection refused"}`

	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.Equal(t, client.EventError, event.Type)
	require.NotNil(t, event.Error)
	require.Equal(t, "Connection refused", event.Error.Message)
}

func TestParser_ParseEvent_NestedAPIError(t *testing.T) {
	p := NewParser()

	// Nested API error format from real OpenCode context exceeded error
	// {"type":"error","error":{"name":"APIError","data":{"message":"prompt is too long: 200561 tokens > 200000 maximum",...}}}
	input := `{"type":"error","timestamp":1768711215455,"sessionID":"ses_test123","error":{"name":"APIError","data":{"message":"prompt is too long: 200561 tokens > 200000 maximum","statusCode":400,"isRetryable":false}}}`

	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.Equal(t, client.EventError, event.Type)
	require.Equal(t, "ses_test123", event.SessionID)
	require.NotNil(t, event.Error)
	require.Equal(t, "prompt is too long: 200561 tokens > 200000 maximum", event.Error.Message)
	require.Equal(t, "APIError", event.Error.Code)
	// Should detect as context exhausted
	require.Equal(t, client.ErrReasonContextExceeded, event.Error.Reason)
}

func TestParser_ParseEvent_ToolUse(t *testing.T) {
	p := NewParser()

	// OpenCode tool_use event with camelCase callID
	input := `{"type":"tool_use","sessionID":"ses_tool123","part":{"type":"tool","tool":"bash","callID":"call_abc","state":{"status":"completed","input":{"command":"ls -la"},"output":"file1.txt\nfile2.txt"}}}`

	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.Equal(t, client.EventToolUse, event.Type)
	require.NotNil(t, event.Tool)
	require.Equal(t, "call_abc", event.Tool.ID)
	require.Equal(t, "bash", event.Tool.Name)
	require.Equal(t, "file1.txt\nfile2.txt", event.Result)
}

func TestParser_ParseEvent_StepFinishWithTokens(t *testing.T) {
	p := NewParser()

	// OpenCode step_finish event with token usage
	input := `{"type":"step_finish","sessionID":"ses_step123","part":{"type":"step-finish","reason":"tool-calls","tokens":{"input":5000,"output":1000,"reasoning":0,"cache":{"read":2000,"write":500}}}}`

	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.Equal(t, client.EventType("step_finish"), event.Type)
	require.Equal(t, "tool-calls", event.SubType)
	require.NotNil(t, event.Usage)
	require.Equal(t, 7000, event.Usage.TokensUsed) // input + cache.read
	require.Equal(t, 200000, event.Usage.TotalTokens)
	require.Equal(t, 1000, event.Usage.OutputTokens)
}

func TestParser_ExtractSessionRef_FromAnyEventType(t *testing.T) {
	p := NewParser()

	// CRITICAL: OpenCode extracts sessionID from ANY event type, not just init
	tests := []struct {
		name    string
		event   client.OutputEvent
		rawLine []byte
		want    string
	}{
		{
			name:    "text event with sessionID",
			event:   client.OutputEvent{Type: client.EventAssistant, SessionID: "ses_text123"},
			rawLine: nil,
			want:    "ses_text123",
		},
		{
			name:    "tool_use event with sessionID",
			event:   client.OutputEvent{Type: client.EventToolUse, SessionID: "ses_tool456"},
			rawLine: nil,
			want:    "ses_tool456",
		},
		{
			name:    "step_finish event with sessionID",
			event:   client.OutputEvent{Type: client.EventType("step_finish"), SessionID: "ses_step789"},
			rawLine: nil,
			want:    "ses_step789",
		},
		{
			name:    "error event with sessionID",
			event:   client.OutputEvent{Type: client.EventError, SessionID: "ses_err012"},
			rawLine: nil,
			want:    "ses_err012",
		},
		{
			name:    "system event with sessionID",
			event:   client.OutputEvent{Type: client.EventSystem, SessionID: "ses_sys345"},
			rawLine: nil,
			want:    "ses_sys345",
		},
		{
			name:    "empty event no sessionID",
			event:   client.OutputEvent{},
			rawLine: nil,
			want:    "",
		},
		{
			name:    "fallback to raw line camelCase",
			event:   client.OutputEvent{},
			rawLine: []byte(`{"sessionID":"ses_raw678"}`),
			want:    "ses_raw678",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.ExtractSessionRef(tt.event, tt.rawLine)
			require.Equal(t, tt.want, result)
		})
	}
}

func TestParser_ExtractSessionRef_CamelCaseField(t *testing.T) {
	p := NewParser()

	// Test that we handle camelCase sessionID (not snake_case session_id)
	event := client.OutputEvent{}
	rawLine := []byte(`{"sessionID":"ses_camel123","type":"text"}`)

	result := p.ExtractSessionRef(event, rawLine)
	require.Equal(t, "ses_camel123", result)

	// Verify snake_case does NOT work (should return empty)
	snakeCaseLine := []byte(`{"session_id":"ses_snake456","type":"text"}`)
	result2 := p.ExtractSessionRef(event, snakeCaseLine)
	require.Equal(t, "", result2, "snake_case session_id should NOT be extracted")
}

func TestParser_IsContextExhausted_UsesBaseParser(t *testing.T) {
	p := NewParser()

	tests := []struct {
		name  string
		event client.OutputEvent
		want  bool
	}{
		{
			name:  "empty event not exhausted",
			event: client.OutputEvent{},
			want:  false,
		},
		{
			name: "error without context pattern",
			event: client.OutputEvent{
				Type:  client.EventError,
				Error: &client.ErrorInfo{Message: "Connection refused"},
			},
			want: false,
		},
		{
			name: "error with ErrReasonContextExceeded",
			event: client.OutputEvent{
				Type:  client.EventError,
				Error: &client.ErrorInfo{Message: "error", Reason: client.ErrReasonContextExceeded},
			},
			want: true,
		},
		{
			name: "error with prompt too long pattern",
			event: client.OutputEvent{
				Type:  client.EventError,
				Error: &client.ErrorInfo{Message: "Prompt is too long: 300000 tokens"},
			},
			want: true,
		},
		{
			name: "error with context window exceeded pattern",
			event: client.OutputEvent{
				Type:  client.EventError,
				Error: &client.ErrorInfo{Message: "Context window exceeded"},
			},
			want: true,
		},
		{
			name: "error with token limit pattern",
			event: client.OutputEvent{
				Type:  client.EventError,
				Error: &client.ErrorInfo{Message: "Token limit exceeded for this model"},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.IsContextExhausted(tt.event)
			require.Equal(t, tt.want, result)
		})
	}
}

func TestParser_ParseEvent_ContextExhausted(t *testing.T) {
	p := NewParser()

	// Error event with context exhaustion pattern should set ErrReasonContextExceeded
	input := `{"type":"error","error":{"message":"Prompt is too long: 250000 tokens exceeds 200000 maximum"}}`

	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.Equal(t, client.EventError, event.Type)
	require.NotNil(t, event.Error)
	require.Equal(t, client.ErrReasonContextExceeded, event.Error.Reason)
}

func TestParser_ParseEvent_InvalidJSON(t *testing.T) {
	p := NewParser()

	_, err := p.ParseEvent([]byte("not json"))
	require.Error(t, err)
}

func TestParser_ParseEvent_EmptyObject(t *testing.T) {
	p := NewParser()

	event, err := p.ParseEvent([]byte("{}"))
	require.NoError(t, err)
	require.Equal(t, client.EventType(""), event.Type)
}

func TestParser_ImplementsEventParser(t *testing.T) {
	p := NewParser()

	// Verify at runtime that Parser implements EventParser
	var _ client.EventParser = p
}
