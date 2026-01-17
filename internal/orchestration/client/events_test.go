package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ===========================================================================
// ErrorReason Tests
// ===========================================================================

func TestErrorReason_Constants(t *testing.T) {
	// Verify error reason constants are defined correctly
	assert.Equal(t, ErrorReason(""), ErrReasonUnknown)
	assert.Equal(t, ErrorReason("context_exceeded"), ErrReasonContextExceeded)
	assert.Equal(t, ErrorReason("rate_limited"), ErrReasonRateLimited)
	assert.Equal(t, ErrorReason("invalid_request"), ErrReasonInvalidRequest)
}

// ===========================================================================
// ErrorInfo Tests
// ===========================================================================

func TestErrorInfo_IsContextExceeded_ReturnsTrue(t *testing.T) {
	err := &ErrorInfo{
		Message: "Prompt is too long: 201234 tokens > 200000 maximum",
		Code:    "PROMPT_TOO_LONG",
		Reason:  ErrReasonContextExceeded,
	}

	assert.True(t, err.IsContextExceeded())
}

func TestErrorInfo_IsContextExceeded_ReturnsFalseForOtherReasons(t *testing.T) {
	tests := []struct {
		name   string
		reason ErrorReason
	}{
		{"unknown", ErrReasonUnknown},
		{"rate_limited", ErrReasonRateLimited},
		{"invalid_request", ErrReasonInvalidRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &ErrorInfo{
				Message: "some error",
				Reason:  tt.reason,
			}
			assert.False(t, err.IsContextExceeded())
		})
	}
}

func TestErrorInfo_IsContextExceeded_ReturnsFalseForNil(t *testing.T) {
	var err *ErrorInfo
	assert.False(t, err.IsContextExceeded())
}

func TestErrorInfo_IsContextExceeded_ReturnsFalseForEmptyReason(t *testing.T) {
	err := &ErrorInfo{
		Message: "Some error without reason",
	}
	assert.False(t, err.IsContextExceeded())
}

// ===========================================================================
// OutputEvent with ErrorInfo Tests
// ===========================================================================

func TestOutputEvent_AssistantWithContextExceededError(t *testing.T) {
	// This tests the pattern where Claude returns an assistant message with an error
	// indicating context window exhaustion
	event := OutputEvent{
		Type: EventAssistant,
		Message: &MessageContent{
			Role: "assistant",
		},
		Error: &ErrorInfo{
			Message: "Prompt is too long",
			Code:    "PROMPT_TOO_LONG",
			Reason:  ErrReasonContextExceeded,
		},
	}

	require.True(t, event.IsAssistant())
	require.NotNil(t, event.Error)
	require.True(t, event.Error.IsContextExceeded())
}

func TestOutputEvent_AssistantWithOtherError(t *testing.T) {
	event := OutputEvent{
		Type: EventAssistant,
		Error: &ErrorInfo{
			Message: "Rate limit exceeded",
			Reason:  ErrReasonRateLimited,
		},
	}

	require.True(t, event.IsAssistant())
	require.NotNil(t, event.Error)
	require.False(t, event.Error.IsContextExceeded())
}

func TestOutputEvent_AssistantWithoutError(t *testing.T) {
	event := OutputEvent{
		Type: EventAssistant,
		Message: &MessageContent{
			Content: []ContentBlock{
				{Type: "text", Text: "Hello"},
			},
		},
	}

	require.True(t, event.IsAssistant())
	require.Nil(t, event.Error)
}
