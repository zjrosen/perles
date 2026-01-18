package client

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewBaseParser(t *testing.T) {
	tests := []struct {
		name           string
		contextWindow  int
		expectedWindow int
	}{
		{
			name:           "sets context window correctly - 200000",
			contextWindow:  200000,
			expectedWindow: 200000,
		},
		{
			name:           "sets context window correctly - 1000000",
			contextWindow:  1000000,
			expectedWindow: 1000000,
		},
		{
			name:           "handles zero context window",
			contextWindow:  0,
			expectedWindow: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewBaseParser(tt.contextWindow)
			require.Equal(t, tt.expectedWindow, parser.ContextWindowSize())
		})
	}
}

func TestBaseParser_ContextWindowSize(t *testing.T) {
	t.Run("returns configured value", func(t *testing.T) {
		parser := NewBaseParser(150000)
		require.Equal(t, 150000, parser.ContextWindowSize())
	})
}

func TestBaseParser_IsContextExhausted_NilError(t *testing.T) {
	parser := NewBaseParser(200000)

	// Event with nil error should return false
	event := OutputEvent{
		Type: EventAssistant,
		// Error is nil
	}
	require.False(t, parser.IsContextExhausted(event))
}

func TestBaseParser_IsContextExhausted_ErrReasonContextExceeded(t *testing.T) {
	parser := NewBaseParser(200000)

	// Event with ErrReasonContextExceeded should return true
	event := OutputEvent{
		Type: EventError,
		Error: &ErrorInfo{
			Message: "some error",
			Reason:  ErrReasonContextExceeded,
		},
	}
	require.True(t, parser.IsContextExhausted(event))
}

func TestBaseParser_IsContextExhausted_MessagePatterns(t *testing.T) {
	parser := NewBaseParser(200000)

	tests := []struct {
		name     string
		message  string
		expected bool
	}{
		// All 6 patterns should be detected
		{
			name:     "pattern: prompt is too long",
			message:  "Prompt is too long: 201234 tokens > 200000 maximum",
			expected: true,
		},
		{
			name:     "pattern: context window exceeded",
			message:  "Context window exceeded",
			expected: true,
		},
		{
			name:     "pattern: context exceeded",
			message:  "The context exceeded the limit",
			expected: true,
		},
		{
			name:     "pattern: context limit",
			message:  "The context limit has been reached",
			expected: true,
		},
		{
			name:     "pattern: token limit",
			message:  "Token limit exceeded",
			expected: true,
		},
		{
			name:     "pattern: maximum context length",
			message:  "This model's maximum context length is 200000 tokens",
			expected: true,
		},
		// Case insensitivity tests
		{
			name:     "case insensitive: PROMPT IS TOO LONG",
			message:  "PROMPT IS TOO LONG",
			expected: true,
		},
		{
			name:     "case insensitive: CONTEXT WINDOW EXCEEDED",
			message:  "CONTEXT WINDOW EXCEEDED",
			expected: true,
		},
		{
			name:     "case insensitive: MAXIMUM CONTEXT LENGTH",
			message:  "MAXIMUM CONTEXT LENGTH exceeded",
			expected: true,
		},
		// Edge cases - should NOT detect
		{
			name:     "empty message returns false",
			message:  "",
			expected: false,
		},
		{
			name:     "unrelated error: connection failed",
			message:  "Connection failed",
			expected: false,
		},
		{
			name:     "unrelated error: rate limit",
			message:  "Rate limit exceeded",
			expected: false,
		},
		{
			name:     "unrelated error: invalid request",
			message:  "Invalid request",
			expected: false,
		},
		{
			name:     "unrelated error: generic",
			message:  "Something went wrong",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := OutputEvent{
				Type: EventError,
				Error: &ErrorInfo{
					Message: tt.message,
					Reason:  ErrReasonUnknown, // Not pre-classified
				},
			}
			result := parser.IsContextExhausted(event)
			require.Equal(t, tt.expected, result, "IsContextExhausted for message: %q", tt.message)
		})
	}
}

func TestBaseParser_IsContextExhausted_ErrorResultWithContextMessage(t *testing.T) {
	parser := NewBaseParser(200000)

	// Test that result events with is_error=true are also checked
	// GetErrorMessage() falls back to Result field when Error.Message is empty
	event := OutputEvent{
		Type:          EventResult,
		IsErrorResult: true,
		Result:        "Prompt is too long",
		Error: &ErrorInfo{
			// Empty message, so GetErrorMessage will use Result field
			Message: "",
			Reason:  ErrReasonUnknown,
		},
	}
	require.True(t, parser.IsContextExhausted(event))
}

func TestIsContextExhaustedMessage(t *testing.T) {
	tests := []struct {
		msg      string
		expected bool
	}{
		// All 6 patterns
		{"Prompt is too long", true},
		{"Context window exceeded", true},
		{"context exceeded", true},
		{"context limit reached", true},
		{"token limit exceeded", true},
		{"maximum context length is 200000", true},

		// Case insensitivity
		{"PROMPT IS TOO LONG", true},
		{"CONTEXT WINDOW EXCEEDED", true},
		{"MAXIMUM CONTEXT LENGTH", true},

		// Negative cases
		{"", false},
		{"Connection failed", false},
		{"Rate limit exceeded", false},
		{"Invalid request", false},
		{"Something went wrong", false},
	}

	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			result := isContextExhaustedMessage(tt.msg)
			require.Equal(t, tt.expected, result, "isContextExhaustedMessage(%q)", tt.msg)
		})
	}
}

func TestParsePolymorphicError_StringError(t *testing.T) {
	// Simple string error message
	raw := json.RawMessage(`"Connection refused"`)
	result := ParsePolymorphicError(raw)

	require.NotNil(t, result)
	require.Equal(t, "Connection refused", result.Message)
	require.Empty(t, result.Code)
}

func TestParsePolymorphicError_ObjectError(t *testing.T) {
	// Error as object with code and message
	raw := json.RawMessage(`{"message":"Something went wrong","code":"INTERNAL"}`)
	result := ParsePolymorphicError(raw)

	require.NotNil(t, result)
	require.Equal(t, "Something went wrong", result.Message)
	require.Equal(t, "INTERNAL", result.Code)
}

func TestParsePolymorphicError_AmpNestedFormat(t *testing.T) {
	// Amp nested JSON format: "413 {\"type\":\"error\",\"error\":{...}}"
	raw := json.RawMessage(`"413 {\"type\":\"error\",\"error\":{\"type\":\"invalid_request_error\",\"message\":\"Prompt is too long\"}}"`)
	result := ParsePolymorphicError(raw)

	require.NotNil(t, result)
	require.Equal(t, "Prompt is too long", result.Message)
	require.Equal(t, "invalid_request_error", result.Code)
}

func TestParsePolymorphicError_NullEmpty(t *testing.T) {
	tests := []struct {
		name  string
		input json.RawMessage
	}{
		{"nil input", nil},
		{"empty input", json.RawMessage{}},
		{"empty slice", json.RawMessage([]byte{})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParsePolymorphicError(tt.input)
			require.Nil(t, result)
		})
	}
}

func TestParsePolymorphicError_MalformedNestedJSON(t *testing.T) {
	// Malformed JSON in nested error returns original message
	raw := json.RawMessage(`"413 {invalid json here}"`)
	result := ParsePolymorphicError(raw)

	require.NotNil(t, result)
	// Should fall back to entire string as message
	require.Equal(t, "413 {invalid json here}", result.Message)
	require.Empty(t, result.Code)
}

func TestParsePolymorphicError_ObjectWithCodeOnly(t *testing.T) {
	// Object with code but no message
	raw := json.RawMessage(`{"code":"NOT_FOUND"}`)
	result := ParsePolymorphicError(raw)

	require.NotNil(t, result)
	require.Empty(t, result.Message)
	require.Equal(t, "NOT_FOUND", result.Code)
}

func TestParsePolymorphicError_ObjectWithMessageOnly(t *testing.T) {
	// Object with message but no code
	raw := json.RawMessage(`{"message":"File not found"}`)
	result := ParsePolymorphicError(raw)

	require.NotNil(t, result)
	require.Equal(t, "File not found", result.Message)
	require.Empty(t, result.Code)
}

func TestParsePolymorphicError_EmptyObject(t *testing.T) {
	// Empty object should return nil (no message or code)
	raw := json.RawMessage(`{}`)
	result := ParsePolymorphicError(raw)

	require.Nil(t, result)
}

func TestParsePolymorphicError_EmptyString(t *testing.T) {
	// Empty string value (quoted) should return nil
	raw := json.RawMessage(`""`)
	result := ParsePolymorphicError(raw)

	require.Nil(t, result)
}

// mockEventParser is a test implementation of EventParser.
type mockEventParser struct {
	BaseParser
	parseEventCalled bool
	lastData         []byte
}

func (m *mockEventParser) ParseEvent(data []byte) (OutputEvent, error) {
	m.parseEventCalled = true
	m.lastData = data
	return OutputEvent{Type: EventAssistant, Result: "parsed"}, nil
}

func (m *mockEventParser) ExtractSessionRef(_ OutputEvent, _ []byte) string {
	return "test-session-ref"
}

func TestWithEventParser_SetsParseEventFn(t *testing.T) {
	// Create a mock parser
	mockParser := &mockEventParser{
		BaseParser: NewBaseParser(200000),
	}

	// Create a BaseProcess with WithEventParser option
	bp := &BaseProcess{}
	opt := WithEventParser(mockParser)
	opt(bp)

	// Verify parseEventFn is set
	require.NotNil(t, bp.parseEventFn)

	// Verify calling parseEventFn delegates to the parser
	testData := []byte(`{"type":"test"}`)
	event, err := bp.parseEventFn(testData)

	require.NoError(t, err)
	require.True(t, mockParser.parseEventCalled)
	require.Equal(t, testData, mockParser.lastData)
	require.Equal(t, EventAssistant, event.Type)
	require.Equal(t, "parsed", event.Result)
}
