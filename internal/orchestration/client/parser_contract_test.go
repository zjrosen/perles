package client_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/client/providers/amp"
	"github.com/zjrosen/perles/internal/orchestration/client/providers/claude"
	"github.com/zjrosen/perles/internal/orchestration/client/providers/codex"
	"github.com/zjrosen/perles/internal/orchestration/client/providers/gemini"
	"github.com/zjrosen/perles/internal/orchestration/client/providers/opencode"
)

// parserTestCase defines a provider parser to test against the contract.
type parserTestCase struct {
	name   string
	parser client.EventParser
}

// allParsers returns all EventParser implementations to test.
func allParsers() []parserTestCase {
	return []parserTestCase{
		{"Claude", claude.NewParser()},
		{"Amp", amp.NewParser()},
		{"Codex", codex.NewParser()},
		{"Gemini", gemini.NewParser()},
		{"OpenCode", opencode.NewParser()},
	}
}

// TestAllParsers_Contract_InvalidJSON verifies that all parsers return an error
// when given invalid JSON input.
func TestAllParsers_Contract_InvalidJSON(t *testing.T) {
	for _, tc := range allParsers() {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.parser.ParseEvent([]byte("not json"))
			require.Error(t, err, "ParseEvent should return error for invalid JSON")
		})
	}
}

// TestAllParsers_Contract_EmptyInput verifies that all parsers handle nil/empty
// input gracefully (either return error or empty event, but not panic).
func TestAllParsers_Contract_EmptyInput(t *testing.T) {
	for _, tc := range allParsers() {
		t.Run(tc.name+"/nil", func(t *testing.T) {
			// Should not panic - may return error or empty event
			require.NotPanics(t, func() {
				_, _ = tc.parser.ParseEvent(nil)
			})
		})
		t.Run(tc.name+"/empty", func(t *testing.T) {
			// Should not panic - may return error or empty event
			require.NotPanics(t, func() {
				_, _ = tc.parser.ParseEvent([]byte{})
			})
		})
		t.Run(tc.name+"/emptyString", func(t *testing.T) {
			// Should not panic - may return error or empty event
			require.NotPanics(t, func() {
				_, _ = tc.parser.ParseEvent([]byte(""))
			})
		})
	}
}

// TestAllParsers_Contract_ContextWindowPositive verifies that all parsers
// return a positive context window size.
func TestAllParsers_Contract_ContextWindowPositive(t *testing.T) {
	for _, tc := range allParsers() {
		t.Run(tc.name, func(t *testing.T) {
			size := tc.parser.ContextWindowSize()
			require.Greater(t, size, 0, "ContextWindowSize should return positive value")
		})
	}
}

// TestAllParsers_Contract_NoErrorNotExhausted verifies that all parsers return false
// for IsContextExhausted when given an empty event (no error).
func TestAllParsers_Contract_NoErrorNotExhausted(t *testing.T) {
	for _, tc := range allParsers() {
		t.Run(tc.name, func(t *testing.T) {
			emptyEvent := client.OutputEvent{}
			result := tc.parser.IsContextExhausted(emptyEvent)
			require.False(t, result, "IsContextExhausted should return false for empty event")
		})
	}
}

// TestAllParsers_Contract_ContextExhaustedReason verifies that all parsers
// detect ErrReasonContextExceeded in the error reason field.
func TestAllParsers_Contract_ContextExhaustedReason(t *testing.T) {
	for _, tc := range allParsers() {
		t.Run(tc.name, func(t *testing.T) {
			eventWithReason := client.OutputEvent{
				Type: client.EventError,
				Error: &client.ErrorInfo{
					Message: "some error",
					Reason:  client.ErrReasonContextExceeded,
				},
			}
			result := tc.parser.IsContextExhausted(eventWithReason)
			require.True(t, result, "IsContextExhausted should return true for ErrReasonContextExceeded")
		})
	}
}

// TestAllParsers_Contract_ContextExhaustedMessagePatterns verifies that all parsers
// detect context exhaustion via message pattern matching (the 6 known patterns).
func TestAllParsers_Contract_ContextExhaustedMessagePatterns(t *testing.T) {
	patterns := []string{
		"Prompt is too long: 250000 tokens > 200000 maximum",
		"Context window exceeded",
		"The context exceeded the limit",
		"The context limit has been reached",
		"Token limit exceeded",
		"This model's maximum context length is 200000 tokens",
	}

	for _, tc := range allParsers() {
		for _, pattern := range patterns {
			t.Run(tc.name+"/"+pattern[:20], func(t *testing.T) {
				eventWithMessage := client.OutputEvent{
					Type: client.EventError,
					Error: &client.ErrorInfo{
						Message: pattern,
						Reason:  client.ErrReasonUnknown, // Not pre-classified
					},
				}
				result := tc.parser.IsContextExhausted(eventWithMessage)
				require.True(t, result, "IsContextExhausted should detect pattern: %q", pattern)
			})
		}
	}
}

// TestAllParsers_Contract_SessionRefType verifies that ExtractSessionRef
// returns a string type (may be empty, but must not panic).
func TestAllParsers_Contract_SessionRefType(t *testing.T) {
	for _, tc := range allParsers() {
		t.Run(tc.name+"/emptyEvent", func(t *testing.T) {
			require.NotPanics(t, func() {
				_ = tc.parser.ExtractSessionRef(client.OutputEvent{}, nil)
			})
		})
		t.Run(tc.name+"/initEvent", func(t *testing.T) {
			initEvent := client.OutputEvent{
				Type:      client.EventSystem,
				SubType:   "init",
				SessionID: "test-session-123",
			}
			require.NotPanics(t, func() {
				result := tc.parser.ExtractSessionRef(initEvent, []byte(`{"session_id":"test-session-123"}`))
				// Result is a string (may be empty depending on provider)
				_ = result
			})
		})
	}
}

// TestAllParsers_Contract_ParseEventReturnsOutputEvent verifies that ParseEvent
// returns a valid OutputEvent struct for valid JSON.
func TestAllParsers_Contract_ParseEventReturnsOutputEvent(t *testing.T) {
	validJSONInputs := []struct {
		name  string
		input string
	}{
		{"emptyObject", `{}`},
		{"typeOnly", `{"type":"assistant"}`},
		{"systemInit", `{"type":"system","subtype":"init"}`},
	}

	for _, tc := range allParsers() {
		for _, input := range validJSONInputs {
			t.Run(tc.name+"/"+input.name, func(t *testing.T) {
				event, err := tc.parser.ParseEvent([]byte(input.input))
				// Should not panic, and either return an event or an error
				if err == nil {
					// If no error, event should be usable
					_ = event.Type
					_ = event.IsError()
				}
			})
		}
	}
}

// TestAllParsers_Contract_IsContextExhaustedFalseForNonContextErrors verifies that
// IsContextExhausted returns false for errors that are NOT context-related.
func TestAllParsers_Contract_IsContextExhaustedFalseForNonContextErrors(t *testing.T) {
	nonContextErrors := []string{
		"Connection refused",
		"Rate limit exceeded",
		"Invalid API key",
		"Network timeout",
		"Internal server error",
	}

	for _, tc := range allParsers() {
		for _, errMsg := range nonContextErrors {
			t.Run(tc.name+"/"+errMsg[:10], func(t *testing.T) {
				eventWithMessage := client.OutputEvent{
					Type: client.EventError,
					Error: &client.ErrorInfo{
						Message: errMsg,
						Reason:  client.ErrReasonUnknown,
					},
				}
				result := tc.parser.IsContextExhausted(eventWithMessage)
				require.False(t, result, "IsContextExhausted should return false for non-context error: %q", errMsg)
			})
		}
	}
}
