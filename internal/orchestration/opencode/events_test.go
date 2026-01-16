package opencode

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/orchestration/client"
)

func TestParseEvent_StepStart(t *testing.T) {
	jsonLine := `{"type":"step_start","timestamp":1768539296394,"sessionID":"ses_43ad8482affef0ocxqD4UJi3Ym","part":{"id":"prt_bc528128a001i5fTcV5LsEJJt0","sessionID":"ses_43ad8482affef0ocxqD4UJi3Ym","messageID":"msg_bc527b89a0011pTYxdlkzIIRSC","type":"step-start","snapshot":"b2a39d9a8ce035534a3b6832dee918391341c6fa"}}`

	event, err := parseEvent([]byte(jsonLine))
	require.NoError(t, err)

	require.Equal(t, client.EventType("step_start"), event.Type)
	require.Equal(t, "ses_43ad8482affef0ocxqD4UJi3Ym", event.SessionID)

	// Verify raw data is preserved
	require.Equal(t, jsonLine, string(event.Raw))
}

func TestParseEvent_TextEvent(t *testing.T) {
	jsonLine := `{"type":"text","timestamp":1768539298870,"sessionID":"ses_43ad8482affef0ocxqD4UJi3Ym","part":{"id":"prt_bc5281899001mErid8gs789wna","sessionID":"ses_43ad8482affef0ocxqD4UJi3Ym","messageID":"msg_bc527b89a0011pTYxdlkzIIRSC","type":"text","text":"I'll gather some context about your project before we start.","time":{"start":1768539298870,"end":1768539298870}}}`

	event, err := parseEvent([]byte(jsonLine))
	require.NoError(t, err)

	require.Equal(t, client.EventAssistant, event.Type)
	require.True(t, event.IsAssistant())
	require.NotNil(t, event.Message)
	require.Equal(t, "assistant", event.Message.Role)
	require.Equal(t, "I'll gather some context about your project before we start.", event.Message.GetText())
	require.False(t, event.Message.HasToolUses())
}

func TestParseEvent_ToolUseEvent(t *testing.T) {
	jsonLine := `{"type":"tool_use","timestamp":1768539298840,"sessionID":"ses_43ad8482affef0ocxqD4UJi3Ym","part":{"id":"prt_bc5281aff001MDkJAHVrM49Lf1","sessionID":"ses_43ad8482affef0ocxqD4UJi3Ym","messageID":"msg_bc527b89a0011pTYxdlkzIIRSC","type":"tool","callID":"call_ca7a77bbc3784e959917e32c","tool":"bash","state":{"status":"completed","input":{"command":"bd ready -n 5","description":"Show top 5 ready issues"},"output":"\n✨ No ready work found\n\n","title":"Show top 5 ready issues","metadata":{"output":"\n✨ No ready work found\n\n","exit":0,"description":"Show top 5 ready issues","truncated":false},"time":{"start":1768539298570,"end":1768539298840}}}}`

	event, err := parseEvent([]byte(jsonLine))
	require.NoError(t, err)

	require.Equal(t, client.EventToolUse, event.Type)
	require.NotNil(t, event.Tool)
	require.Equal(t, "call_ca7a77bbc3784e959917e32c", event.Tool.ID)
	require.Equal(t, "bash", event.Tool.Name)

	// Verify tool input
	var input struct {
		Command     string `json:"command"`
		Description string `json:"description"`
	}
	err = json.Unmarshal(event.Tool.Input, &input)
	require.NoError(t, err)
	require.Equal(t, "bd ready -n 5", input.Command)

	// Verify output is captured
	require.Contains(t, event.Result, "No ready work found")

	// Message should also be populated
	require.NotNil(t, event.Message)
	require.True(t, event.Message.HasToolUses())
}

func TestParseEvent_StepFinishWithTokens(t *testing.T) {
	jsonLine := `{"type":"step_finish","timestamp":1768539299091,"sessionID":"ses_43ad8482affef0ocxqD4UJi3Ym","part":{"id":"prt_bc5281cdf001DJPSpp3k4sgvS0","sessionID":"ses_43ad8482affef0ocxqD4UJi3Ym","messageID":"msg_bc527b89a0011pTYxdlkzIIRSC","type":"step-finish","reason":"tool-calls","snapshot":"b2a39d9a8ce035534a3b6832dee918391341c6fa","cost":0,"tokens":{"input":20923,"output":154,"reasoning":92,"cache":{"read":467,"write":0}}}}`

	event, err := parseEvent([]byte(jsonLine))
	require.NoError(t, err)

	require.Equal(t, client.EventType("step_finish"), event.Type)
	require.Equal(t, "tool-calls", event.SubType)

	// Verify token usage
	require.NotNil(t, event.Usage)
	// TokensUsed = input(20923) + cacheRead(467) = 21390
	require.Equal(t, 21390, event.Usage.TokensUsed)
	require.Equal(t, 154, event.Usage.OutputTokens)
	require.Equal(t, 200000, event.Usage.TotalTokens) // Claude context window
}

func TestParseEvent_StepFinishStopReason(t *testing.T) {
	jsonLine := `{"type":"step_finish","timestamp":1768539307264,"sessionID":"ses_abc","part":{"type":"step-finish","reason":"stop","cost":0,"tokens":{"input":21363,"output":216,"reasoning":96,"cache":{"read":468,"write":0}}}}`

	event, err := parseEvent([]byte(jsonLine))
	require.NoError(t, err)

	require.Equal(t, client.EventType("step_finish"), event.Type)
	require.Equal(t, "stop", event.SubType)
	require.NotNil(t, event.Usage)
}

func TestParseEvent_MalformedJSON(t *testing.T) {
	invalidJSON := `{"type":"text",invalid}`

	_, err := parseEvent([]byte(invalidJSON))
	require.Error(t, err)
}

func TestParseEvent_EmptyLine(t *testing.T) {
	_, err := parseEvent([]byte(""))
	require.Error(t, err)
}

func TestParseEvent_MinimalEvent(t *testing.T) {
	// Event with just type
	jsonLine := `{"type":"step_start"}`

	event, err := parseEvent([]byte(jsonLine))
	require.NoError(t, err)

	require.Equal(t, client.EventType("step_start"), event.Type)
	require.Empty(t, event.SubType)
	require.Empty(t, event.SessionID)
}

func TestParseEvent_PreservesRawData(t *testing.T) {
	jsonLine := `{"type":"text","sessionID":"ses_123","part":{"type":"text","text":"test"}}`

	event, err := parseEvent([]byte(jsonLine))
	require.NoError(t, err)

	// Raw should be a copy, not the same slice
	require.Equal(t, jsonLine, string(event.Raw))
	require.Len(t, event.Raw, len(jsonLine))
}

func TestMapEventType_AllKnownTypes(t *testing.T) {
	tests := []struct {
		input    string
		expected client.EventType
	}{
		{"system", client.EventSystem},
		{"init", client.EventSystem},
		{"text", client.EventAssistant},
		{"tool_use", client.EventToolUse},
		{"step_start", client.EventType("step_start")},
		{"step_finish", client.EventType("step_finish")},
		{"result", client.EventResult},
		{"error", client.EventError},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := mapEventType(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestMapEventType_UnknownTypesPassThrough(t *testing.T) {
	// Unknown types should pass through as-is for forward compatibility
	unknownTypes := []string{"custom_event", "future_type", "special"}

	for _, ut := range unknownTypes {
		t.Run(ut, func(t *testing.T) {
			result := mapEventType(ut)
			require.Equal(t, client.EventType(ut), result)
		})
	}
}

func TestParseEvent_TextEventExtractsContent(t *testing.T) {
	jsonLine := `{"type":"text","sessionID":"ses_test","part":{"id":"prt_123","type":"text","text":"Hello, world!"}}`

	event, err := parseEvent([]byte(jsonLine))
	require.NoError(t, err)

	require.True(t, event.IsAssistant())
	require.NotNil(t, event.Message)
	require.Equal(t, "Hello, world!", event.Message.GetText())
	require.Equal(t, "prt_123", event.Message.ID)
}

func TestParseEvent_ToolUseWithReadTool(t *testing.T) {
	jsonLine := `{"type":"tool_use","sessionID":"ses_test","part":{"callID":"call_123","tool":"Read","state":{"status":"completed","input":{"file_path":"/project/main.go"},"output":"package main\n\nfunc main() {}"}}}`

	event, err := parseEvent([]byte(jsonLine))
	require.NoError(t, err)

	require.Equal(t, client.EventToolUse, event.Type)
	require.NotNil(t, event.Tool)
	require.Equal(t, "Read", event.Tool.Name)
	require.Equal(t, "call_123", event.Tool.ID)

	// Verify input
	var input struct {
		FilePath string `json:"file_path"`
	}
	err = json.Unmarshal(event.Tool.Input, &input)
	require.NoError(t, err)
	require.Equal(t, "/project/main.go", input.FilePath)

	// Output should be in Result
	require.Equal(t, "package main\n\nfunc main() {}", event.Result)
}

func TestParseEvent_ToolUseWithoutState(t *testing.T) {
	// Tool use event before execution (no state yet)
	jsonLine := `{"type":"tool_use","sessionID":"ses_test","part":{"callID":"call_456","tool":"Bash"}}`

	event, err := parseEvent([]byte(jsonLine))
	require.NoError(t, err)

	require.Equal(t, client.EventToolUse, event.Type)
	require.NotNil(t, event.Tool)
	require.Equal(t, "Bash", event.Tool.Name)
	require.Empty(t, event.Result) // No output yet
}

func TestParseEvent_StepFinishWithoutTokens(t *testing.T) {
	jsonLine := `{"type":"step_finish","sessionID":"ses_test","part":{"type":"step-finish","reason":"stop"}}`

	event, err := parseEvent([]byte(jsonLine))
	require.NoError(t, err)

	require.Equal(t, client.EventType("step_finish"), event.Type)
	require.Equal(t, "stop", event.SubType)
	require.Nil(t, event.Usage) // No tokens means no usage
}

func TestParseEvent_StepFinishWithNilCache(t *testing.T) {
	jsonLine := `{"type":"step_finish","sessionID":"ses_test","part":{"type":"step-finish","reason":"stop","tokens":{"input":1000,"output":100}}}`

	event, err := parseEvent([]byte(jsonLine))
	require.NoError(t, err)

	require.NotNil(t, event.Usage)
	// TokensUsed = input(1000) + cacheRead(0) = 1000
	require.Equal(t, 1000, event.Usage.TokensUsed)
	require.Equal(t, 100, event.Usage.OutputTokens)
}

// Golden tests based on actual OpenCode output

func TestGolden_ActualTextEvent(t *testing.T) {
	// Exact JSON from actual opencode run
	jsonLine := `{"type":"text","timestamp":1768539307196,"sessionID":"ses_43ad8482affef0ocxqD4UJi3Ym","part":{"id":"prt_bc5283112001cVq9ECSKmsy7GQ","sessionID":"ses_43ad8482affef0ocxqD4UJi3Ym","messageID":"msg_bc5281d3a001vFhgU23qpX5hvZ","type":"text","text":"Hi! I'm ready to help with your Perles project.\n\n**Current Status:**\n- No ready work available","time":{"start":1768539307195,"end":1768539307195}}}`

	event, err := parseEvent([]byte(jsonLine))
	require.NoError(t, err)

	require.Equal(t, client.EventAssistant, event.Type)
	require.True(t, event.IsAssistant())
	require.NotNil(t, event.Message)
	require.Contains(t, event.Message.GetText(), "Hi! I'm ready to help")
	require.Contains(t, event.Message.GetText(), "**Current Status:**")
}

func TestGolden_ActualToolUseEvent(t *testing.T) {
	// Exact JSON from actual opencode run (shortened output for test)
	jsonLine := `{"type":"tool_use","timestamp":1768539299039,"sessionID":"ses_43ad8482affef0ocxqD4UJi3Ym","part":{"id":"prt_bc5281c12001aVZtMO7Vwb5i4U","sessionID":"ses_43ad8482affef0ocxqD4UJi3Ym","messageID":"msg_bc527b89a0011pTYxdlkzIIRSC","type":"tool","callID":"call_8f7d72b588794fea8be69bd1","tool":"bash","state":{"status":"completed","input":{"command":"bd activity --limit 10","description":"Show recent project activity"},"output":"[23:21:04] Activity output here\n","title":"Show recent project activity","time":{"start":1768539298837,"end":1768539299038}}}}`

	event, err := parseEvent([]byte(jsonLine))
	require.NoError(t, err)

	require.Equal(t, client.EventToolUse, event.Type)
	require.NotNil(t, event.Tool)
	require.Equal(t, "bash", event.Tool.Name)
	require.Equal(t, "call_8f7d72b588794fea8be69bd1", event.Tool.ID)

	var input struct {
		Command     string `json:"command"`
		Description string `json:"description"`
	}
	err = json.Unmarshal(event.Tool.Input, &input)
	require.NoError(t, err)
	require.Equal(t, "bd activity --limit 10", input.Command)
	require.Contains(t, event.Result, "Activity output here")
}

func TestGolden_ActualStepFinishEvent(t *testing.T) {
	// Exact JSON from actual opencode run
	jsonLine := `{"type":"step_finish","timestamp":1768539307264,"sessionID":"ses_43ad8482affef0ocxqD4UJi3Ym","part":{"id":"prt_bc5283cbc0019uZE74M8MOnGKR","sessionID":"ses_43ad8482affef0ocxqD4UJi3Ym","messageID":"msg_bc5281d3a001vFhgU23qpX5hvZ","type":"step-finish","reason":"stop","snapshot":"b2a39d9a8ce035534a3b6832dee918391341c6fa","cost":0,"tokens":{"input":21363,"output":216,"reasoning":96,"cache":{"read":468,"write":0}}}}`

	event, err := parseEvent([]byte(jsonLine))
	require.NoError(t, err)

	require.Equal(t, client.EventType("step_finish"), event.Type)
	require.Equal(t, "stop", event.SubType)
	require.NotNil(t, event.Usage)
	// TokensUsed = 21363 + 468 = 21831
	require.Equal(t, 21831, event.Usage.TokensUsed)
	require.Equal(t, 216, event.Usage.OutputTokens)
}

// Test the event types work with client.OutputEvent methods
func TestEventTypeCompatibility(t *testing.T) {
	tests := []struct {
		name     string
		jsonLine string
		check    func(t *testing.T, e client.OutputEvent)
	}{
		{
			name:     "text is assistant",
			jsonLine: `{"type":"text","part":{"type":"text","text":"hello"}}`,
			check: func(t *testing.T, e client.OutputEvent) {
				require.True(t, e.IsAssistant())
			},
		},
		{
			name:     "tool_use type",
			jsonLine: `{"type":"tool_use","part":{"tool":"bash"}}`,
			check: func(t *testing.T, e client.OutputEvent) {
				require.Equal(t, client.EventToolUse, e.Type)
			},
		},
		{
			name:     "error type",
			jsonLine: `{"type":"error"}`,
			check: func(t *testing.T, e client.OutputEvent) {
				require.True(t, e.IsError())
			},
		},
		{
			name:     "result type",
			jsonLine: `{"type":"result"}`,
			check: func(t *testing.T, e client.OutputEvent) {
				require.True(t, e.IsResult())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := parseEvent([]byte(tt.jsonLine))
			require.NoError(t, err)
			tt.check(t, event)
		})
	}
}
