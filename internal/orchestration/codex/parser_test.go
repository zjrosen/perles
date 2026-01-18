package codex

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/orchestration/client"
)

func TestNewParser(t *testing.T) {
	p := NewParser()
	require.NotNil(t, p)
	require.Equal(t, CodexContextWindowSize, p.ContextWindowSize())
}

func TestParser_ContextWindowSize(t *testing.T) {
	p := NewParser()
	require.Equal(t, 200000, p.ContextWindowSize())
}

func TestParser_ParseEvent_ThreadStarted(t *testing.T) {
	p := NewParser()

	input := `{"type":"thread.started","thread_id":"thread-abc123"}`
	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.Equal(t, client.EventSystem, event.Type)
	require.Equal(t, "init", event.SubType)
	require.Equal(t, "thread-abc123", event.SessionID)
	require.True(t, event.IsInit())
}

func TestParser_ParseEvent_ItemCompletedAgentMessage(t *testing.T) {
	p := NewParser()

	input := `{"type":"item.completed","item":{"id":"item-1","type":"agent_message","text":"Hello, world!"}}`
	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.Equal(t, client.EventAssistant, event.Type)
	require.NotNil(t, event.Message)
	require.Equal(t, "item-1", event.Message.ID)
	require.Equal(t, "assistant", event.Message.Role)
	require.Equal(t, "Hello, world!", event.Message.GetText())
}

func TestParser_ParseEvent_ItemStartedCommandExecution(t *testing.T) {
	p := NewParser()

	input := `{"type":"item.started","item":{"id":"item-2","type":"command_execution","command":"ls -la"}}`
	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.Equal(t, client.EventToolUse, event.Type)
	require.NotNil(t, event.Tool)
	require.Equal(t, "Bash", event.Tool.Name)
	require.Equal(t, "item-2", event.Tool.ID)
}

func TestParser_ParseEvent_ItemCompletedCommandExecution(t *testing.T) {
	p := NewParser()

	input := `{"type":"item.completed","item":{"id":"item-3","type":"command_execution","aggregated_output":"file1.txt\nfile2.txt","exit_code":0}}`
	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.Equal(t, client.EventToolResult, event.Type)
	require.NotNil(t, event.Tool)
	require.Equal(t, "Bash", event.Tool.Name)
	require.Equal(t, "file1.txt\nfile2.txt", event.Tool.Output)
}

func TestParser_ParseEvent_ItemUpdatedAgentMessage(t *testing.T) {
	p := NewParser()

	input := `{"type":"item.updated","item":{"id":"item-4","type":"agent_message","text":"Thinking..."}}`
	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.Equal(t, client.EventAssistant, event.Type)
	require.NotNil(t, event.Message)
	require.Equal(t, "Thinking...", event.Message.GetText())
}

func TestParser_ParseEvent_TurnCompleted(t *testing.T) {
	p := NewParser()

	input := `{"type":"turn.completed","usage":{"input_tokens":100,"cached_input_tokens":50,"output_tokens":25}}`
	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.Equal(t, client.EventResult, event.Type)
	require.NotNil(t, event.Usage)
	// TokensUsed = input_tokens + cached_input_tokens = 100 + 50 = 150
	require.Equal(t, 150, event.Usage.TokensUsed)
	require.Equal(t, 25, event.Usage.OutputTokens)
	require.Equal(t, CodexContextWindowSize, event.Usage.TotalTokens)
}

func TestParser_ParseEvent_TurnFailed(t *testing.T) {
	p := NewParser()

	input := `{"type":"turn.failed","error":{"message":"Something went wrong"}}`
	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.Equal(t, client.EventError, event.Type)
	require.NotNil(t, event.Error)
	require.Equal(t, "Something went wrong", event.Error.Message)
}

func TestParser_ParseEvent_ErrorEvent(t *testing.T) {
	p := NewParser()

	input := `{"type":"error","message":"Stream connection failed"}`
	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.Equal(t, client.EventError, event.Type)
	require.NotNil(t, event.Error)
	require.Equal(t, "Stream connection failed", event.Error.Message)
}

func TestParser_ParseEvent_MCPToolCall(t *testing.T) {
	p := NewParser()

	input := `{"type":"item.started","item":{"id":"item-5","type":"mcp_tool_call","server":"my-server","tool":"my_tool","arguments":{"key":"value"}}}`
	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.Equal(t, client.EventToolUse, event.Type)
	require.NotNil(t, event.Tool)
	require.Equal(t, "my_tool", event.Tool.Name)
	require.Equal(t, "item-5", event.Tool.ID)
}

func TestParser_ParseEvent_MCPToolResult(t *testing.T) {
	p := NewParser()

	input := `{"type":"item.completed","item":{"id":"item-6","type":"mcp_tool_call","tool":"my_tool","result":{"content":[{"type":"text","text":"Tool output here"}]}}}`
	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.Equal(t, client.EventToolResult, event.Type)
	require.NotNil(t, event.Tool)
	require.Equal(t, "my_tool", event.Tool.Name)
	require.Equal(t, "Tool output here", event.Tool.Output)
}

func TestParser_ParseEvent_InvalidJSON(t *testing.T) {
	p := NewParser()

	input := `not valid json`
	_, err := p.ParseEvent([]byte(input))

	require.Error(t, err)
}

func TestParser_ExtractSessionRef_ThreadStarted(t *testing.T) {
	p := NewParser()

	event := client.OutputEvent{
		Type:      client.EventSystem,
		SubType:   "init",
		SessionID: "thread-123",
	}
	result := p.ExtractSessionRef(event, []byte(`{"thread_id":"thread-123"}`))
	require.Equal(t, "thread-123", result)
}

func TestParser_ExtractSessionRef_NonInitEvent(t *testing.T) {
	p := NewParser()

	event := client.OutputEvent{
		Type:      client.EventAssistant,
		SessionID: "thread-123",
	}
	result := p.ExtractSessionRef(event, nil)
	require.Empty(t, result)
}

func TestParser_ExtractSessionRef_EmptyEvent(t *testing.T) {
	p := NewParser()

	event := client.OutputEvent{}
	result := p.ExtractSessionRef(event, nil)
	require.Empty(t, result)
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
		},
	}
	require.False(t, p.IsContextExhausted(event))
}

func TestParser_IsContextExhausted_ContextReason(t *testing.T) {
	p := NewParser()

	event := client.OutputEvent{
		Type: client.EventError,
		Error: &client.ErrorInfo{
			Message: "Error occurred",
			Reason:  client.ErrReasonContextExceeded,
		},
	}
	require.True(t, p.IsContextExhausted(event))
}

func TestParser_IsContextExhausted_MessagePatterns(t *testing.T) {
	p := NewParser()

	patterns := []string{
		"Prompt is too long: 250000 tokens",
		"Context window exceeded",
		"The context exceeded the limit",
		"Token limit exceeded",
		"Maximum context length is 200000",
	}

	for _, pattern := range patterns {
		t.Run(pattern[:20], func(t *testing.T) {
			event := client.OutputEvent{
				Type: client.EventError,
				Error: &client.ErrorInfo{
					Message: pattern,
				},
			}
			require.True(t, p.IsContextExhausted(event), "Should detect pattern: %s", pattern)
		})
	}
}

func TestParser_ImplementsEventParser(t *testing.T) {
	var _ client.EventParser = (*Parser)(nil)

	p := NewParser()
	var ep client.EventParser = p
	require.NotNil(t, ep)
}

func TestParser_ParseEvent_NonZeroExitCode(t *testing.T) {
	p := NewParser()

	exitCode := 1
	input := `{"type":"item.completed","item":{"id":"item-7","type":"command_execution","aggregated_output":"command not found","exit_code":1}}`
	event, err := p.ParseEvent([]byte(input))

	require.NoError(t, err)
	require.Equal(t, client.EventToolResult, event.Type)
	require.True(t, event.IsErrorResult)
	_ = exitCode // used in the JSON
}

func TestParser_IntegrationWithBaseProcess(t *testing.T) {
	p := NewParser()

	// Create a BaseProcess with the parser
	bp := &client.BaseProcess{}
	opt := client.WithEventParser(p)
	opt(bp)

	// Verification that the parser integrates with BaseProcess
	// The parseEventFn is set internally
}

// BenchmarkParser_ParseEvent benchmarks the parsing performance.
func BenchmarkParser_ParseEvent(b *testing.B) {
	p := NewParser()
	input := []byte(`{"type":"item.completed","item":{"id":"item-1","type":"agent_message","text":"Hello, world!"}}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = p.ParseEvent(input)
	}
}
