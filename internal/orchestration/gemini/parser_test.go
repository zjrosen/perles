package gemini

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
	require.Equal(t, 1000000, size, "Gemini context window should be 1M tokens")
}

func TestParser_ParseEvent_ModelContent(t *testing.T) {
	p := NewParser()

	// Test assistant message event
	input := `{"type":"message","role":"assistant","content":"Hello, world!"}`
	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.Equal(t, client.EventAssistant, event.Type)
	require.NotNil(t, event.Message)
	require.Equal(t, "assistant", event.Message.Role)
	require.Len(t, event.Message.Content, 1)
	require.Equal(t, "Hello, world!", event.Message.Content[0].Text)
}

func TestParser_ParseEvent_UserVsModelRoleDiscrimination(t *testing.T) {
	p := NewParser()

	tests := []struct {
		name         string
		input        string
		expectedType client.EventType
		hasMessage   bool
	}{
		{
			name:         "assistant message has content",
			input:        `{"type":"message","role":"assistant","content":"Hello!"}`,
			expectedType: client.EventAssistant,
			hasMessage:   true,
		},
		{
			name:         "user message is ignored",
			input:        `{"type":"message","role":"user","content":"User input"}`,
			expectedType: client.EventToolResult, // User messages map to EventToolResult
			hasMessage:   false,                  // User message content is not populated
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := p.ParseEvent([]byte(tt.input))
			require.NoError(t, err)
			require.Equal(t, tt.expectedType, event.Type)
			if tt.hasMessage {
				require.NotNil(t, event.Message, "Assistant message should have content")
			} else {
				// User messages result in nil Message.Content blocks
				if event.Message != nil {
					require.Empty(t, event.Message.Content, "User message should not have populated content")
				}
			}
		})
	}
}

func TestParser_ParseEvent_ErrorEvent(t *testing.T) {
	p := NewParser()

	input := `{"type":"error","error":{"message":"Something went wrong","code":"internal_error"}}`
	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.Equal(t, client.EventError, event.Type)
	require.NotNil(t, event.Error)
	require.Equal(t, "Something went wrong", event.Error.Message)
	require.Equal(t, "internal_error", event.Error.Code)
}

func TestParser_ParseEvent_InitEvent(t *testing.T) {
	p := NewParser()

	input := `{"type":"init","session_id":"sess-123","model":"gemini-pro"}`
	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.Equal(t, client.EventSystem, event.Type)
	require.Equal(t, "init", event.SubType)
	require.Equal(t, "sess-123", event.SessionID)
}

func TestParser_ParseEvent_ToolUse(t *testing.T) {
	p := NewParser()

	input := `{"type":"tool_use","tool_name":"read_file","tool_id":"tool-1","parameters":{"path":"/tmp/test.txt"}}`
	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.Equal(t, client.EventToolUse, event.Type)
	require.NotNil(t, event.Tool)
	require.Equal(t, "read_file", event.Tool.Name)
	require.Equal(t, "tool-1", event.Tool.ID)
}

func TestParser_ParseEvent_ResultWithUsage(t *testing.T) {
	p := NewParser()

	input := `{"type":"result","stats":{"tokens_prompt":1000,"tokens_candidates":500,"tokens_cached":100}}`
	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.Equal(t, client.EventResult, event.Type)
	require.NotNil(t, event.Usage)
	require.Equal(t, 1100, event.Usage.TokensUsed) // prompt + cached
	require.Equal(t, 500, event.Usage.OutputTokens)
	require.Equal(t, 1000000, event.Usage.TotalTokens) // 1M context window
}

func TestParser_ExtractSessionRef(t *testing.T) {
	p := NewParser()

	tests := []struct {
		name     string
		event    client.OutputEvent
		expected string
	}{
		{
			name: "extracts session from init event",
			event: client.OutputEvent{
				Type:      client.EventSystem,
				SubType:   "init",
				SessionID: "gemini-session-123",
			},
			expected: "gemini-session-123",
		},
		{
			name: "returns empty for non-init event",
			event: client.OutputEvent{
				Type: client.EventAssistant,
			},
			expected: "",
		},
		{
			name: "returns empty for init event without session ID",
			event: client.OutputEvent{
				Type:    client.EventSystem,
				SubType: "init",
			},
			expected: "",
		},
		{
			name:     "returns empty for empty event",
			event:    client.OutputEvent{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.ExtractSessionRef(tt.event, nil)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestParser_IsContextExhausted(t *testing.T) {
	p := NewParser()

	tests := []struct {
		name     string
		event    client.OutputEvent
		expected bool
	}{
		{
			name:     "returns false for empty event",
			event:    client.OutputEvent{},
			expected: false,
		},
		{
			name: "returns true for ErrReasonContextExceeded",
			event: client.OutputEvent{
				Error: &client.ErrorInfo{
					Message: "Context exceeded",
					Reason:  client.ErrReasonContextExceeded,
				},
			},
			expected: true,
		},
		{
			name: "returns true for prompt too long message",
			event: client.OutputEvent{
				Error: &client.ErrorInfo{
					Message: "Prompt is too long: 1100000 tokens",
				},
			},
			expected: true,
		},
		{
			name: "returns true for context window exceeded message",
			event: client.OutputEvent{
				Error: &client.ErrorInfo{
					Message: "Context window exceeded",
				},
			},
			expected: true,
		},
		{
			name: "returns true for token limit message",
			event: client.OutputEvent{
				Error: &client.ErrorInfo{
					Message: "Token limit exceeded",
				},
			},
			expected: true,
		},
		{
			name: "returns false for non-context errors",
			event: client.OutputEvent{
				Error: &client.ErrorInfo{
					Message: "Rate limit exceeded",
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.IsContextExhausted(tt.event)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestParser_ImplementsEventParser(t *testing.T) {
	p := NewParser()
	var _ client.EventParser = p
}
