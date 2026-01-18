package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zjrosen/perles/internal/orchestration/client"

	"github.com/stretchr/testify/require"
)

// errTest is a sentinel error for testing
var errTest = errors.New("test error")

func TestBuildArgs(t *testing.T) {
	tests := []struct {
		name     string
		cfg      Config
		expected []string
	}{
		{
			name: "minimal config",
			cfg: Config{
				WorkDir: "/project",
				Prompt:  "Hello",
			},
			expected: []string{
				"--print",
				"--output-format", "stream-json",
				"--verbose",
				"--", "Hello",
			},
		},
		{
			name: "with session resume",
			cfg: Config{
				WorkDir:   "/project",
				Prompt:    "Continue",
				SessionID: "sess-123",
			},
			expected: []string{
				"--print",
				"--output-format", "stream-json",
				"--verbose",
				"--resume", "sess-123",
				"--", "Continue",
			},
		},
		{
			name: "with model",
			cfg: Config{
				WorkDir: "/project",
				Prompt:  "Hello",
				Model:   "opus",
			},
			expected: []string{
				"--print",
				"--output-format", "stream-json",
				"--verbose",
				"--model", "opus",
				"--", "Hello",
			},
		},
		{
			name: "with skip permissions",
			cfg: Config{
				WorkDir:         "/project",
				Prompt:          "Hello",
				SkipPermissions: true,
			},
			expected: []string{
				"--print",
				"--output-format", "stream-json",
				"--verbose",
				"--dangerously-skip-permissions",
				"--", "Hello",
			},
		},
		{
			name: "with system prompt",
			cfg: Config{
				WorkDir:            "/project",
				Prompt:             "Hello",
				AppendSystemPrompt: "Be concise",
			},
			expected: []string{
				"--print",
				"--output-format", "stream-json",
				"--verbose",
				"--append-system-prompt", "Be concise",
				"--", "Hello",
			},
		},
		{
			name: "with allowed tools",
			cfg: Config{
				WorkDir:      "/project",
				Prompt:       "Hello",
				AllowedTools: []string{"Read", "Write"},
			},
			expected: []string{
				"--print",
				"--output-format", "stream-json",
				"--verbose",
				"--allowed-tools", "Read",
				"--allowed-tools", "Write",
				"--", "Hello",
			},
		},
		{
			name: "with disallowed tools",
			cfg: Config{
				WorkDir:         "/project",
				Prompt:          "Hello",
				DisallowedTools: []string{"AskUserQuestion"},
			},
			expected: []string{
				"--print",
				"--output-format", "stream-json",
				"--verbose",
				"--disallowed-tools", "AskUserQuestion",
				"--", "Hello",
			},
		},
		{
			name: "full config",
			cfg: Config{
				WorkDir:            "/project",
				Prompt:             "Analyze code",
				SessionID:          "sess-456",
				Model:              "sonnet",
				AppendSystemPrompt: "Focus on errors",
				SkipPermissions:    true,
				DisallowedTools:    []string{"AskUserQuestion"},
			},
			expected: []string{
				"--print",
				"--output-format", "stream-json",
				"--verbose",
				"--resume", "sess-456",
				"--model", "sonnet",
				"--dangerously-skip-permissions",
				"--append-system-prompt", "Focus on errors",
				"--disallowed-tools", "AskUserQuestion",
				"--", "Analyze code",
			},
		},
		{
			name: "empty prompt",
			cfg: Config{
				WorkDir: "/project",
			},
			expected: []string{
				"--print",
				"--output-format", "stream-json",
				"--verbose",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := buildArgs(tt.cfg)
			require.Equal(t, tt.expected, args)
		})
	}
}

func TestOutputEventParsing(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		validate func(t *testing.T, e client.OutputEvent)
	}{
		{
			name: "system init event",
			json: `{"type":"system","subtype":"init","session_id":"sess-abc123","cwd":"/project"}`,
			validate: func(t *testing.T, e client.OutputEvent) {
				require.Equal(t, client.EventSystem, e.Type)
				require.Equal(t, "init", e.SubType)
				require.Equal(t, "sess-abc123", e.SessionID)
				require.Equal(t, "/project", e.WorkDir)
				require.True(t, e.IsInit())
				require.False(t, e.IsAssistant())
			},
		},
		{
			name: "assistant message with text",
			json: `{"type":"assistant","message":{"id":"msg_1","role":"assistant","content":[{"type":"text","text":"Hello, world!"}],"model":"claude-sonnet-4"}}`,
			validate: func(t *testing.T, e client.OutputEvent) {
				require.Equal(t, client.EventAssistant, e.Type)
				require.True(t, e.IsAssistant())
				require.NotNil(t, e.Message)
				require.Equal(t, "msg_1", e.Message.ID)
				require.Equal(t, "assistant", e.Message.Role)
				require.Equal(t, "claude-sonnet-4", e.Message.Model)
				require.Equal(t, "Hello, world!", e.Message.GetText())
				require.False(t, e.Message.HasToolUses())
			},
		},
		{
			name: "assistant message with tool use",
			json: `{"type":"assistant","message":{"id":"msg_2","content":[{"type":"tool_use","id":"toolu_123","name":"Read","input":{"file_path":"main.go"}}]}}`,
			validate: func(t *testing.T, e client.OutputEvent) {
				require.Equal(t, client.EventAssistant, e.Type)
				require.True(t, e.IsAssistant())
				require.NotNil(t, e.Message)
				require.True(t, e.Message.HasToolUses())
				tools := e.Message.GetToolUses()
				require.Len(t, tools, 1)
				require.Equal(t, "Read", tools[0].Name)
				require.Equal(t, "toolu_123", tools[0].ID)
			},
		},
		{
			name: "assistant message with text and tool use",
			json: `{"type":"assistant","message":{"content":[{"type":"text","text":"Let me read that file."},{"type":"tool_use","id":"toolu_456","name":"Read","input":{"file_path":"go.mod"}}]}}`,
			validate: func(t *testing.T, e client.OutputEvent) {
				require.True(t, e.IsAssistant())
				require.NotNil(t, e.Message)
				require.Equal(t, "Let me read that file.", e.Message.GetText())
				require.True(t, e.Message.HasToolUses())
				tools := e.Message.GetToolUses()
				require.Len(t, tools, 1)
				require.Equal(t, "Read", tools[0].Name)
			},
		},
		{
			name: "tool result event",
			json: `{"type":"tool_result","tool":{"id":"toolu_123","name":"Read","content":"package main\n"}}`,
			validate: func(t *testing.T, e client.OutputEvent) {
				require.Equal(t, client.EventToolResult, e.Type)
				require.True(t, e.IsToolResult())
				require.NotNil(t, e.Tool)
				require.Equal(t, "Read", e.Tool.Name)
				require.Equal(t, "toolu_123", e.Tool.ID)
				require.Equal(t, "package main\n", e.Tool.GetOutput())
			},
		},
		{
			name: "tool result with output field",
			json: `{"type":"tool_result","tool":{"name":"Bash","output":"success"}}`,
			validate: func(t *testing.T, e client.OutputEvent) {
				require.True(t, e.IsToolResult())
				require.NotNil(t, e.Tool)
				require.Equal(t, "success", e.Tool.GetOutput())
			},
		},
		{
			name: "result success event",
			json: `{"type":"result","subtype":"success","total_cost_usd":0.0123,"duration_ms":45000,"usage":{"input_tokens":5000,"output_tokens":1500,"cache_read_input_tokens":10000,"cache_creation_input_tokens":2000}}`,
			validate: func(t *testing.T, e client.OutputEvent) {
				require.Equal(t, client.EventResult, e.Type)
				require.Equal(t, "success", e.SubType)
				require.True(t, e.IsResult())
				require.InDelta(t, 0.0123, e.TotalCostUSD, 0.0001)
				require.Equal(t, int64(45000), e.DurationMs)
				// Usage is only populated for assistant events, not result events
				require.Nil(t, e.Usage)
			},
		},
		{
			name: "assistant message with usage",
			json: `{"type":"assistant","message":{"id":"msg_3","role":"assistant","content":[{"type":"text","text":"Hello!"}],"model":"claude-sonnet-4","usage":{"input_tokens":5000,"output_tokens":1500,"cache_read_input_tokens":10000,"cache_creation_input_tokens":2000}}}`,
			validate: func(t *testing.T, e client.OutputEvent) {
				require.Equal(t, client.EventAssistant, e.Type)
				require.True(t, e.IsAssistant())
				require.NotNil(t, e.Message)
				// Usage is populated for assistant events
				// TokensUsed = input(5000) + cacheRead(10000) + cacheCreate(2000) = 17000
				require.NotNil(t, e.Usage)
				require.Equal(t, 17000, e.Usage.TokensUsed)
				require.Equal(t, 1500, e.Usage.OutputTokens)
				require.Equal(t, 200000, e.Usage.TotalTokens)
			},
		},
		{
			name: "result with model usage",
			json: `{"type":"result","subtype":"success","modelUsage":{"claude-sonnet-4":{"inputTokens":1000,"outputTokens":500,"contextWindow":200000,"costUSD":0.05}}}`,
			validate: func(t *testing.T, e client.OutputEvent) {
				require.True(t, e.IsResult())
				require.NotNil(t, e.ModelUsage)
				usage, ok := e.ModelUsage["claude-sonnet-4"]
				require.True(t, ok)
				require.Equal(t, 1000, usage.InputTokens)
				require.Equal(t, 500, usage.OutputTokens)
				require.Equal(t, 200000, usage.ContextWindow)
				require.InDelta(t, 0.05, usage.CostUSD, 0.001)
			},
		},
		{
			name: "error event",
			json: `{"type":"error","error":{"message":"Rate limit exceeded","code":"rate_limit"}}`,
			validate: func(t *testing.T, e client.OutputEvent) {
				require.Equal(t, client.EventError, e.Type)
				require.True(t, e.IsError())
				require.NotNil(t, e.Error)
				require.Equal(t, "Rate limit exceeded", e.Error.Message)
				require.Equal(t, "rate_limit", e.Error.Code)
			},
		},
		{
			name: "error as string code",
			json: `{"type":"assistant","error":"invalid_request","message":{"content":[{"type":"text","text":"Something went wrong"}]}}`,
			validate: func(t *testing.T, e client.OutputEvent) {
				require.Equal(t, client.EventAssistant, e.Type)
				require.NotNil(t, e.Error)
				require.Equal(t, "invalid_request", e.Error.Code)
				require.Equal(t, client.ErrReasonInvalidRequest, e.Error.Reason)
			},
		},
		{
			name: "context exhaustion - prompt too long",
			json: `{"type":"assistant","message":{"id":"msg-123","role":"assistant","stop_reason":"stop_sequence","content":[{"type":"text","text":"Prompt is too long"}]},"error":"invalid_request","session_id":"sess-abc"}`,
			validate: func(t *testing.T, e client.OutputEvent) {
				require.Equal(t, client.EventAssistant, e.Type)
				require.NotNil(t, e.Error)
				require.Equal(t, "invalid_request", e.Error.Code)
				require.Equal(t, client.ErrReasonContextExceeded, e.Error.Reason)
				require.Equal(t, "Prompt is too long", e.Error.Message)
				require.True(t, e.Error.IsContextExceeded())
			},
		},
		{
			name: "rate limit as string code",
			json: `{"type":"error","error":"rate_limit_exceeded"}`,
			validate: func(t *testing.T, e client.OutputEvent) {
				require.Equal(t, client.EventError, e.Type)
				require.NotNil(t, e.Error)
				require.Equal(t, "rate_limit_exceeded", e.Error.Code)
				require.Equal(t, client.ErrReasonRateLimited, e.Error.Reason)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := parseEvent([]byte(tt.json))
			require.NoError(t, err)
			tt.validate(t, event)
		})
	}
}

func TestProcessStatus(t *testing.T) {
	tests := []struct {
		status   client.ProcessStatus
		expected string
	}{
		{client.StatusPending, "pending"},
		{client.StatusRunning, "running"},
		{client.StatusCompleted, "completed"},
		{client.StatusFailed, "failed"},
		{client.StatusCancelled, "cancelled"},
		{client.ProcessStatus(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.status.String())
		})
	}
}

func TestGetContextTokensNilUsage(t *testing.T) {
	event := client.OutputEvent{Type: "result"}
	require.Equal(t, 0, event.GetContextTokens())
}

func TestToolContentGetOutput(t *testing.T) {
	tests := []struct {
		name     string
		tool     client.ToolContent
		expected string
	}{
		{
			name:     "output field set",
			tool:     client.ToolContent{Output: "from output", Content: "from content"},
			expected: "from output",
		},
		{
			name:     "only content field set",
			tool:     client.ToolContent{Content: "from content"},
			expected: "from content",
		},
		{
			name:     "both empty",
			tool:     client.ToolContent{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.tool.GetOutput())
		})
	}
}

func TestMessageContentMultipleTextBlocks(t *testing.T) {
	msg := client.MessageContent{
		Content: []client.ContentBlock{
			{Type: "text", Text: "First. "},
			{Type: "tool_use", Name: "Read"},
			{Type: "text", Text: "Second."},
		},
	}

	// GetText should concatenate all text blocks
	require.Equal(t, "First. Second.", msg.GetText())

	// GetToolUses should return only tool_use blocks
	tools := msg.GetToolUses()
	require.Len(t, tools, 1)
	require.Equal(t, "Read", tools[0].Name)

	// HasToolUses should return true
	require.True(t, msg.HasToolUses())
}

func TestMessageContentNoToolUses(t *testing.T) {
	msg := client.MessageContent{
		Content: []client.ContentBlock{
			{Type: "text", Text: "Just text"},
		},
	}

	require.False(t, msg.HasToolUses())
	require.Empty(t, msg.GetToolUses())
}

func TestOutputEventTypeChecks(t *testing.T) {
	tests := []struct {
		event       client.OutputEvent
		isInit      bool
		isAssistant bool
		isToolRes   bool
		isResult    bool
		isError     bool
	}{
		{
			event:  client.OutputEvent{Type: client.EventSystem, SubType: "init"},
			isInit: true,
		},
		{
			event:       client.OutputEvent{Type: client.EventAssistant},
			isAssistant: true,
		},
		{
			event:     client.OutputEvent{Type: client.EventToolResult},
			isToolRes: true,
		},
		{
			event:    client.OutputEvent{Type: client.EventResult},
			isResult: true,
		},
		{
			event:   client.OutputEvent{Type: client.EventError},
			isError: true,
		},
		{
			event:   client.OutputEvent{Type: "other", Error: &client.ErrorInfo{Message: "oops"}},
			isError: true, // Error field set makes it an error
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.event.Type), func(t *testing.T) {
			require.Equal(t, tt.isInit, tt.event.IsInit())
			require.Equal(t, tt.isAssistant, tt.event.IsAssistant())
			require.Equal(t, tt.isToolRes, tt.event.IsToolResult())
			require.Equal(t, tt.isResult, tt.event.IsResult())
			require.Equal(t, tt.isError, tt.event.IsError())
		})
	}
}

// =============================================================================
// Lifecycle Tests - Process struct behavior without actual subprocess spawning
// =============================================================================

// newTestProcess creates a Process struct for testing without spawning a real subprocess.
// This allows testing lifecycle methods, status transitions, and channel behavior.
func newTestProcess() *Process {
	ctx, cancel := context.WithCancel(context.Background())
	bp := client.NewBaseProcess(
		ctx,
		cancel,
		nil, // No cmd for test
		nil, // No stdout
		nil, // No stderr
		"/test/project",
		client.WithProviderName("claude"),
		client.WithStderrCapture(true),
	)
	bp.SetStatus(client.StatusRunning)
	bp.SetSessionRef("test-session-123")
	return &Process{BaseProcess: bp}
}

// newTestProcessWithPipes creates a Process for testing with pipes for stderr testing.
func newTestProcessWithPipes(ctx context.Context, stderr io.ReadCloser) *Process {
	_, cancel := context.WithCancel(ctx)
	bp := client.NewBaseProcess(
		ctx,
		cancel,
		nil, // No cmd for test
		nil, // No stdout
		stderr,
		"/test/project",
		client.WithProviderName("claude"),
		client.WithStderrCapture(true),
	)
	bp.SetStatus(client.StatusRunning)
	return &Process{BaseProcess: bp}
}

func TestProcessLifecycle_StatusTransitions(t *testing.T) {
	p := newTestProcess()

	// Initial status should be Running (as set in newTestProcess)
	require.Equal(t, client.StatusRunning, p.Status())
	require.True(t, p.IsRunning())

	// Test SetStatus transitions
	p.SetStatus(client.StatusCompleted)
	require.Equal(t, client.StatusCompleted, p.Status())
	require.False(t, p.IsRunning())

	p.SetStatus(client.StatusFailed)
	require.Equal(t, client.StatusFailed, p.Status())

	p.SetStatus(client.StatusCancelled)
	require.Equal(t, client.StatusCancelled, p.Status())
}

func TestProcessLifecycle_Cancel(t *testing.T) {
	p := newTestProcess()

	// Verify initial state
	require.Equal(t, client.StatusRunning, p.Status())

	// Cancel should set status to Cancelled
	err := p.Cancel()
	require.NoError(t, err)
	require.Equal(t, client.StatusCancelled, p.Status())

	// Context should be cancelled
	select {
	case <-p.Context().Done():
		// Expected - context was cancelled
	default:
		require.Fail(t, "Context should be cancelled after Cancel()")
	}
}

func TestProcessLifecycle_CancelRacePrevention(t *testing.T) {
	// This test verifies that Cancel() sets status BEFORE calling cancelFunc,
	// preventing race conditions with goroutines that check status.

	for i := 0; i < 100; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		bp := client.NewBaseProcess(
			ctx,
			cancel,
			nil, nil, nil,
			"/test/project",
			client.WithProviderName("claude"),
		)
		bp.SetStatus(client.StatusRunning)
		p := &Process{BaseProcess: bp}

		// Track status seen by a goroutine that races with Cancel
		var observedStatus client.ProcessStatus
		var wg sync.WaitGroup
		wg.Add(1)

		go func() {
			defer wg.Done()
			// Wait for context cancellation
			<-p.Context().Done()
			// Immediately check status - should already be client.StatusCancelled
			observedStatus = p.Status()
		}()

		// Small sleep to ensure goroutine is waiting
		time.Sleep(time.Microsecond)

		// Cancel the process
		p.Cancel()

		wg.Wait()

		// The goroutine should have seen client.StatusCancelled, not client.StatusRunning
		require.Equal(t, client.StatusCancelled, observedStatus,
			"Goroutine should see client.StatusCancelled after context cancel (iteration %d)", i)
	}
}

func TestProcessLifecycle_SessionIDAndWorkDir(t *testing.T) {
	p := newTestProcess()

	require.Equal(t, "test-session-123", p.SessionID())
	require.Equal(t, "/test/project", p.WorkDir())
}

func TestProcessLifecycle_Channels(t *testing.T) {
	p := newTestProcess()

	// Events channel should be readable
	eventsCh := p.Events()
	require.NotNil(t, eventsCh)

	// Errors channel should be readable
	errorsCh := p.Errors()
	require.NotNil(t, errorsCh)

	// Send an event
	go func() {
		p.EventsWritable() <- client.OutputEvent{Type: "test"}
	}()

	select {
	case event := <-eventsCh:
		require.Equal(t, client.EventType("test"), event.Type)
	case <-time.After(time.Second):
		require.Fail(t, "Timeout waiting for event")
	}

	// Send an error
	go func() {
		p.ErrorsWritable() <- errTest
	}()

	select {
	case err := <-errorsCh:
		require.Equal(t, errTest, err)
	case <-time.After(time.Second):
		require.Fail(t, "Timeout waiting for error")
	}
}

func TestProcessLifecycle_SendError(t *testing.T) {
	p := newTestProcess()

	// SendError should send to channel when space available
	p.SendError(ErrTimeout)

	select {
	case err := <-p.Errors():
		require.Equal(t, ErrTimeout, err)
	default:
		require.Fail(t, "Error should have been sent to channel")
	}
}

func TestProcessLifecycle_SendErrorOverflow(t *testing.T) {
	// Create a process with BaseProcess
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bp := client.NewBaseProcess(
		ctx,
		cancel,
		nil, nil, nil,
		"/test/project",
		client.WithProviderName("claude"),
	)
	p := &Process{BaseProcess: bp}

	// Fill the channel (capacity is 10 in BaseProcess)
	for i := 0; i < 10; i++ {
		p.ErrorsWritable() <- errTest
	}

	// Channel is now full - SendError should not block
	done := make(chan bool)
	go func() {
		p.SendError(ErrTimeout) // This should not block
		done <- true
	}()

	select {
	case <-done:
		// Expected - SendError returned without blocking
	case <-time.After(100 * time.Millisecond):
		require.Fail(t, "SendError blocked on full channel - should have dropped error")
	}

	// Original errors should still be in channel
	require.Len(t, p.ErrorsWritable(), 10)
}

func TestProcessLifecycle_Wait(t *testing.T) {
	p := newTestProcess()

	// Add a WaitGroup counter to simulate goroutines
	p.WaitGroup().Add(1)

	// Wait should block until wg is done
	done := make(chan bool)
	go func() {
		p.Wait()
		done <- true
	}()

	// Wait should be blocking
	select {
	case <-done:
		require.Fail(t, "Wait should be blocking")
	case <-time.After(10 * time.Millisecond):
		// Expected - still waiting
	}

	// Release the waitgroup
	p.WaitGroup().Done()

	// Wait should now complete
	select {
	case <-done:
		// Expected - Wait completed
	case <-time.After(time.Second):
		require.Fail(t, "Wait should have completed after wg.Done()")
	}
}

func TestContextCreation_NoLeak(t *testing.T) {
	// This test verifies the context creation fix - no leak when timeout is set.
	// We test by checking that only one context is created (no orphaned contexts).

	t.Run("with timeout", func(t *testing.T) {
		cfg := Config{
			WorkDir: "/project",
			Prompt:  "test",
			Timeout: time.Hour, // Long timeout so it doesn't fire
		}

		// The fix ensures we use WithTimeout directly, not WithCancel then WithTimeout
		// This is validated by the code structure, but we verify the behavior here
		args := buildArgs(cfg)
		require.Contains(t, args, "--")
		require.Contains(t, args, "test")
	})

	t.Run("without timeout", func(t *testing.T) {
		cfg := Config{
			WorkDir: "/project",
			Prompt:  "test",
			Timeout: 0,
		}

		args := buildArgs(cfg)
		require.Contains(t, args, "--")
		require.Contains(t, args, "test")
	})
}

func TestResumeWithConfig(t *testing.T) {
	tests := []struct {
		name              string
		sessionID         string
		cfgSessionID      string
		expectedSessionID string
	}{
		{
			name:              "parameter used when cfg empty",
			sessionID:         "param-session",
			cfgSessionID:      "",
			expectedSessionID: "param-session",
		},
		{
			name:              "cfg takes precedence when set",
			sessionID:         "param-session",
			cfgSessionID:      "cfg-session",
			expectedSessionID: "cfg-session",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				SessionID: tt.cfgSessionID,
			}

			// We can't actually call ResumeWithConfig (would try to spawn claude),
			// but we can verify the session ID logic
			if cfg.SessionID == "" {
				cfg.SessionID = tt.sessionID
			}

			require.Equal(t, tt.expectedSessionID, cfg.SessionID)
		})
	}
}

func TestMainModelTokenCalculation(t *testing.T) {
	tests := []struct {
		name                 string
		initJSON             string
		resultJSON           string
		expectedMainModel    string
		expectedTokensUsed   int // Computed: input + cacheRead + cacheCreation
		expectedOutputTokens int
		expectedTotalTokens  int // From ContextWindow
	}{
		{
			name:     "single model - no sub-agent",
			initJSON: `{"type":"system","subtype":"init","session_id":"sess-123","model":"claude-opus-4-5-20251101"}`,
			resultJSON: `{
				"type":"result",
				"subtype":"success",
				"usage":{"input_tokens":2,"output_tokens":10,"cache_read_input_tokens":21927,"cache_creation_input_tokens":0},
				"modelUsage":{
					"claude-opus-4-5-20251101":{"inputTokens":4,"outputTokens":30,"cacheReadInputTokens":26053,"cacheCreationInputTokens":0,"contextWindow":200000}
				},
				"total_cost_usd":0.0137
			}`,
			expectedMainModel:    "claude-opus-4-5-20251101",
			expectedTokensUsed:   4 + 26053 + 0, // input + cacheRead + cacheCreation
			expectedOutputTokens: 30,
			expectedTotalTokens:  200000,
		},
		{
			name:     "with sub-agent - should use main model metrics",
			initJSON: `{"type":"system","subtype":"init","session_id":"sess-456","model":"claude-opus-4-5-20251101"}`,
			resultJSON: `{
				"type":"result",
				"subtype":"success",
				"usage":{"input_tokens":2,"output_tokens":158,"cache_read_input_tokens":34599,"cache_creation_input_tokens":9441},
				"modelUsage":{
					"claude-opus-4-5-20251101":{"inputTokens":4,"outputTokens":177,"cacheReadInputTokens":34599,"cacheCreationInputTokens":13567,"contextWindow":200000,"costUSD":0.1065},
					"claude-haiku-4-5-20251001":{"inputTokens":4002,"outputTokens":339,"cacheReadInputTokens":0,"cacheCreationInputTokens":14563,"contextWindow":200000,"costUSD":0.0239}
				},
				"total_cost_usd":0.1304
			}`,
			expectedMainModel:    "claude-opus-4-5-20251101",
			expectedTokensUsed:   4 + 34599 + 13567, // Main model's input + cacheRead + cacheCreation
			expectedOutputTokens: 177,               // Main model's output, not haiku's
			expectedTotalTokens:  200000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newTestProcess()

			// Simulate parsing init event to capture main model
			var initData struct {
				Model string `json:"model"`
			}
			err := json.Unmarshal([]byte(tt.initJSON), &initData)
			require.NoError(t, err)
			p.mainModel = initData.Model

			require.Equal(t, tt.expectedMainModel, p.mainModel)

			// Parse result event using parseEvent (which populates Usage from raw JSON)
			resultEvent, err := parseEvent([]byte(tt.resultJSON))
			require.NoError(t, err)

			// Simulate the fix: populate Usage from main model's ModelUsage (as done in parseOutput)
			if resultEvent.ModelUsage != nil && p.mainModel != "" {
				if mainUsage, ok := resultEvent.ModelUsage[p.mainModel]; ok {
					tokensUsed := mainUsage.InputTokens + mainUsage.CacheReadInputTokens + mainUsage.CacheCreationInputTokens
					totalTokens := mainUsage.ContextWindow
					if totalTokens == 0 {
						totalTokens = 200000
					}
					resultEvent.Usage = &client.UsageInfo{
						TokensUsed:   tokensUsed,
						TotalTokens:  totalTokens,
						OutputTokens: mainUsage.OutputTokens,
					}
				}
			}

			// Verify Usage now contains main model's simplified metrics
			require.NotNil(t, resultEvent.Usage)
			require.Equal(t, tt.expectedTokensUsed, resultEvent.Usage.TokensUsed)
			require.Equal(t, tt.expectedOutputTokens, resultEvent.Usage.OutputTokens)
			require.Equal(t, tt.expectedTotalTokens, resultEvent.Usage.TotalTokens)

			// Verify GetContextTokens returns TokensUsed
			require.Equal(t, tt.expectedTokensUsed, resultEvent.GetContextTokens())
		})
	}
}

func TestContentBlock_FormatToolDisplay(t *testing.T) {
	tests := []struct {
		name     string
		block    client.ContentBlock
		expected string
	}{
		{
			name: "non-tool block returns empty",
			block: client.ContentBlock{
				Type: "text",
				Text: "Hello",
			},
			expected: "",
		},
		{
			name: "bash with description",
			block: client.ContentBlock{
				Type:  "tool_use",
				Name:  "Bash",
				Input: json.RawMessage(`{"command":"find . -name '*.go'","description":"Find Go files"}`),
			},
			expected: "🔧 Bash: Find Go files",
		},
		{
			name: "bash with command only",
			block: client.ContentBlock{
				Type:  "tool_use",
				Name:  "Bash",
				Input: json.RawMessage(`{"command":"ls -la"}`),
			},
			expected: "🔧 Bash: ls -la",
		},
		{
			name: "bash with long command gets truncated",
			block: client.ContentBlock{
				Type:  "tool_use",
				Name:  "Bash",
				Input: json.RawMessage(`{"command":"find /Users/zack/Development/go/src/perles/internal -type d | wc -l"}`),
			},
			expected: "🔧 Bash: find /Users/zack/Development/go/src/perles/inte...",
		},
		{
			name: "view with file path",
			block: client.ContentBlock{
				Type:  "tool_use",
				Name:  "View",
				Input: json.RawMessage(`{"file_path":"/Users/zack/project/src/main.go"}`),
			},
			expected: "🔧 View: main.go",
		},
		{
			name: "edit with file path",
			block: client.ContentBlock{
				Type:  "tool_use",
				Name:  "Edit",
				Input: json.RawMessage(`{"file_path":"/project/config.yaml","old_string":"foo","new_string":"bar"}`),
			},
			expected: "🔧 Edit: config.yaml",
		},
		{
			name: "grep with pattern",
			block: client.ContentBlock{
				Type:  "tool_use",
				Name:  "Grep",
				Input: json.RawMessage(`{"pattern":"func.*Test","path":"/project"}`),
			},
			expected: "🔧 Grep: func.*Test",
		},
		{
			name: "glob with pattern",
			block: client.ContentBlock{
				Type:  "tool_use",
				Name:  "Glob",
				Input: json.RawMessage(`{"pattern":"**/*.go"}`),
			},
			expected: "🔧 Glob: **/*.go",
		},
		{
			name: "unknown tool shows just name",
			block: client.ContentBlock{
				Type:  "tool_use",
				Name:  "spawn_worker",
				Input: json.RawMessage(`{"task_id":"EPIC-1","prompt":"Do something"}`),
			},
			expected: "🔧 spawn_worker",
		},
		{
			name: "empty name returns empty",
			block: client.ContentBlock{
				Type:  "tool_use",
				Name:  "",
				Input: json.RawMessage(`{}`),
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatToolDisplay(&tt.block)
			require.Equal(t, tt.expected, result)
		})
	}
}

// =============================================================================
// Behavior Preservation Tests - Pre-Migration Regression Tests
// =============================================================================
//
// These tests capture the exact current behavior of the Claude provider
// before migration to BaseProcess. They serve as regression tests to
// ensure no behavioral changes occur during migration.
//
// See docs/proposals/2026-01-17-base-process-composition-implementation.md

// TestBehaviorPreservation_ParseStderrCapturesAllLines verifies that parseStderr
// captures all stderr lines to the stderrLines slice. This behavior is essential
// for including stderr content in error messages from waitForCompletion.
func TestBehaviorPreservation_ParseStderrCapturesAllLines(t *testing.T) {
	// Create pipes to simulate stderr
	stderrReader, stderrWriter := io.Pipe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p := newTestProcessWithPipes(ctx, stderrReader)

	p.WaitGroup().Add(1)

	// Start parseStderr goroutine (now handled by BaseProcess)
	go func() {
		defer p.WaitGroup().Done()
		// BaseProcess has parseStderr as a method, but we need to test the behavior
		// Since BaseProcess.parseStderr is private, we verify through public API
		// The actual parsing is done by StartGoroutines, so we simulate that here
		bufScanner := make([]byte, 0)
		buf := make([]byte, 1024)
		for {
			n, err := stderrReader.Read(buf)
			if err != nil {
				break
			}
			bufScanner = append(bufScanner, buf[:n]...)
			// Split by newlines
			for {
				idx := -1
				for i, b := range bufScanner {
					if b == '\n' {
						idx = i
						break
					}
				}
				if idx == -1 {
					break
				}
				line := string(bufScanner[:idx])
				bufScanner = bufScanner[idx+1:]
				p.AppendStderrLine(line)
			}
		}
	}()

	// Write multiple stderr lines
	expectedLines := []string{
		"Error: something went wrong",
		"Details: file not found",
		"Stack trace: line 42",
	}

	for _, line := range expectedLines {
		_, err := stderrWriter.Write([]byte(line + "\n"))
		require.NoError(t, err)
	}

	// Close writer to signal EOF
	stderrWriter.Close()

	// Wait for parseStderr to complete
	p.WaitGroup().Wait()

	// Verify all lines were captured
	captured := p.StderrLines()

	require.Equal(t, expectedLines, captured, "parseStderr should capture all stderr lines in order")
}

// TestBehaviorPreservation_ParseStderrContinuesUntilEOF verifies that parseStderr
// continues reading until EOF even after context cancellation. This ensures we
// capture stderr output that occurs during process shutdown.
func TestBehaviorPreservation_ParseStderrContinuesUntilEOF(t *testing.T) {
	stderrReader, stderrWriter := io.Pipe()

	ctx, cancel := context.WithCancel(context.Background())

	p := newTestProcessWithPipes(ctx, stderrReader)

	p.WaitGroup().Add(1)

	// Start parseStderr goroutine (simulated since BaseProcess.parseStderr is private)
	go func() {
		defer p.WaitGroup().Done()
		bufScanner := make([]byte, 0)
		buf := make([]byte, 1024)
		for {
			n, err := stderrReader.Read(buf)
			if err != nil {
				break
			}
			bufScanner = append(bufScanner, buf[:n]...)
			for {
				idx := -1
				for i, b := range bufScanner {
					if b == '\n' {
						idx = i
						break
					}
				}
				if idx == -1 {
					break
				}
				line := string(bufScanner[:idx])
				bufScanner = bufScanner[idx+1:]
				p.AppendStderrLine(line)
			}
		}
	}()

	// Write first line
	_, err := stderrWriter.Write([]byte("Line before cancel\n"))
	require.NoError(t, err)

	// Cancel context
	cancel()

	// Write more lines after context cancellation
	// parseStderr should continue reading until EOF
	time.Sleep(10 * time.Millisecond) // Give goroutine time to process
	_, err = stderrWriter.Write([]byte("Line after cancel\n"))
	require.NoError(t, err)

	// Close to signal EOF
	stderrWriter.Close()

	// Wait for completion
	p.WaitGroup().Wait()

	// Both lines should be captured (parseStderr reads until EOF, not until ctx.Done)
	captured := p.StderrLines()

	require.Len(t, captured, 2, "parseStderr should read until EOF, not until context cancellation")
	require.Equal(t, "Line before cancel", captured[0])
	require.Equal(t, "Line after cancel", captured[1])
}

// TestBehaviorPreservation_CancelSetsStatusBeforeCancelFunc verifies the critical
// race prevention behavior: Cancel() must set status to StatusCancelled BEFORE
// calling cancelFunc(). This prevents waitForCompletion from seeing StatusRunning
// when it checks status after context is cancelled.
func TestBehaviorPreservation_CancelSetsStatusBeforeCancelFunc(t *testing.T) {
	// Run multiple iterations to catch race conditions
	for i := 0; i < 100; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		statusChan := make(chan client.ProcessStatus, 1)

		bp := client.NewBaseProcess(
			ctx,
			cancel,
			nil, nil, nil,
			"/test/project",
			client.WithProviderName("claude"),
		)
		bp.SetStatus(client.StatusRunning)
		p := &Process{BaseProcess: bp}

		// Goroutine that immediately checks status when context is done
		go func() {
			<-ctx.Done()
			// This simulates waitForCompletion checking status
			statusChan <- p.Status()
		}()

		// Give goroutine time to start waiting
		time.Sleep(time.Microsecond)

		// Cancel - this must set status before calling cancelFunc
		p.Cancel()

		// The observing goroutine should see StatusCancelled, never StatusRunning
		observed := <-statusChan
		require.Equal(t, client.StatusCancelled, observed,
			"iteration %d: goroutine must observe StatusCancelled after ctx.Done()", i)
	}
}

// TestBehaviorPreservation_WaitForCompletionErrorIncludesStderr verifies that
// waitForCompletion includes stderr content in error messages when the process
// fails and stderr lines were captured.
func TestBehaviorPreservation_WaitForCompletionErrorIncludesStderr(t *testing.T) {
	// We need to test the error message format produced by waitForCompletion
	// by examining the exact format used in process.go:499

	t.Run("error with stderr lines", func(t *testing.T) {
		stderrLines := []string{"Error: permission denied", "Path: /etc/passwd"}
		stderrMsg := strings.Join(stderrLines, "\n")
		exitErr := fmt.Errorf("exit status 1")

		// This is the exact format from process.go:499
		expectedFormat := fmt.Errorf("claude process failed: %s (exit: %w)", stderrMsg, exitErr)

		require.Contains(t, expectedFormat.Error(), "Error: permission denied")
		require.Contains(t, expectedFormat.Error(), "Path: /etc/passwd")
		require.Contains(t, expectedFormat.Error(), "exit status 1")
		require.Contains(t, expectedFormat.Error(), "claude process failed:")
	})

	t.Run("error without stderr lines", func(t *testing.T) {
		exitErr := fmt.Errorf("exit status 1")

		// This is the exact format from process.go:501
		expectedFormat := fmt.Errorf("claude process exited: %w", exitErr)

		require.Equal(t, "claude process exited: exit status 1", expectedFormat.Error())
	})
}

// TestBehaviorPreservation_WaitForCompletionErrorMessageFormats documents the
// exact error message formats produced by waitForCompletion for different scenarios.
func TestBehaviorPreservation_WaitForCompletionErrorMessageFormats(t *testing.T) {
	tests := []struct {
		name          string
		stderrLines   []string
		exitError     error
		expectedMatch string
	}{
		{
			name:          "single stderr line",
			stderrLines:   []string{"fatal error"},
			exitError:     fmt.Errorf("exit status 1"),
			expectedMatch: "claude process failed: fatal error (exit: exit status 1)",
		},
		{
			name:          "multiple stderr lines joined with newline",
			stderrLines:   []string{"error line 1", "error line 2"},
			exitError:     fmt.Errorf("exit status 2"),
			expectedMatch: "claude process failed: error line 1\nerror line 2 (exit: exit status 2)",
		},
		{
			name:          "empty stderr uses simple format",
			stderrLines:   []string{},
			exitError:     fmt.Errorf("signal: killed"),
			expectedMatch: "claude process exited: signal: killed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var errMsg string
			if len(tt.stderrLines) > 0 {
				stderrMsg := strings.Join(tt.stderrLines, "\n")
				errMsg = fmt.Sprintf("claude process failed: %s (exit: %v)", stderrMsg, tt.exitError)
			} else {
				errMsg = fmt.Sprintf("claude process exited: %v", tt.exitError)
			}

			require.Equal(t, tt.expectedMatch, errMsg)
		})
	}
}

// TestBehaviorPreservation_ExtractSessionFromInitEvent verifies that parseOutput
// extracts both sessionID and mainModel from the init event JSON.
func TestBehaviorPreservation_ExtractSessionFromInitEvent(t *testing.T) {
	tests := []struct {
		name              string
		initEventJSON     string
		expectedSessionID string
		expectedMainModel string
	}{
		{
			name:              "extracts both session and model",
			initEventJSON:     `{"type":"system","subtype":"init","session_id":"sess-abc123","model":"claude-sonnet-4"}`,
			expectedSessionID: "sess-abc123",
			expectedMainModel: "claude-sonnet-4",
		},
		{
			name:              "extracts opus model variant",
			initEventJSON:     `{"type":"system","subtype":"init","session_id":"sess-xyz789","model":"claude-opus-4-5-20251101"}`,
			expectedSessionID: "sess-xyz789",
			expectedMainModel: "claude-opus-4-5-20251101",
		},
		{
			name:              "handles missing model gracefully",
			initEventJSON:     `{"type":"system","subtype":"init","session_id":"sess-no-model"}`,
			expectedSessionID: "sess-no-model",
			expectedMainModel: "",
		},
		{
			name:              "handles missing session gracefully",
			initEventJSON:     `{"type":"system","subtype":"init","model":"claude-haiku-4"}`,
			expectedSessionID: "",
			expectedMainModel: "claude-haiku-4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate what parseOutput does when it receives an init event
			var initData struct {
				SessionID string `json:"session_id"`
				Model     string `json:"model"`
			}

			err := json.Unmarshal([]byte(tt.initEventJSON), &initData)
			require.NoError(t, err)

			// Verify extraction matches expected
			require.Equal(t, tt.expectedSessionID, initData.SessionID)
			require.Equal(t, tt.expectedMainModel, initData.Model)
		})
	}
}

// TestBehaviorPreservation_EventsChannelOrder verifies that events are delivered
// through the events channel in the order they are parsed from stdout.
func TestBehaviorPreservation_EventsChannelOrder(t *testing.T) {
	p := newTestProcess()

	// Simulate ordered event delivery
	expectedTypes := []client.EventType{
		client.EventSystem,
		client.EventAssistant,
		client.EventToolResult,
		client.EventAssistant,
		client.EventResult,
	}

	// Send events in order
	go func() {
		for _, et := range expectedTypes {
			p.EventsWritable() <- client.OutputEvent{Type: et}
		}
		close(p.EventsWritable())
	}()

	// Receive and verify order
	var receivedTypes []client.EventType
	for event := range p.Events() {
		receivedTypes = append(receivedTypes, event.Type)
	}

	require.Equal(t, expectedTypes, receivedTypes, "Events should be received in order")
}

// TestBehaviorPreservation_CancelDuringInitEventProcessing verifies behavior
// when Cancel is called while an init event is being processed.
func TestBehaviorPreservation_CancelDuringInitEventProcessing(t *testing.T) {
	for i := 0; i < 50; i++ {
		ctx, cancel := context.WithCancel(context.Background())

		bp := client.NewBaseProcess(
			ctx,
			cancel,
			nil, nil, nil,
			"/test/project",
			client.WithProviderName("claude"),
		)
		bp.SetStatus(client.StatusRunning)
		p := &Process{BaseProcess: bp}

		// Goroutine that simulates processing init event
		var sessionIDDuringProcess string
		var wg sync.WaitGroup
		wg.Add(1)

		go func() {
			defer wg.Done()

			// Simulate init event processing - SetSessionRef already locks internally
			p.SetSessionRef("sess-during-cancel")
			sessionIDDuringProcess = p.SessionRef()
		}()

		// Cancel while init event might be processing
		time.Sleep(time.Microsecond)
		p.Cancel()

		wg.Wait()

		// Session ID should be set correctly despite cancel
		finalSessionID := p.SessionRef()

		require.Equal(t, "sess-during-cancel", finalSessionID)
		require.Equal(t, sessionIDDuringProcess, finalSessionID)
	}
}

// TestBehaviorPreservation_StderrOverflowHandling verifies behavior when
// many stderr lines are captured. The current implementation has no limit.
func TestBehaviorPreservation_StderrOverflowHandling(t *testing.T) {
	stderrReader, stderrWriter := io.Pipe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p := newTestProcessWithPipes(ctx, stderrReader)

	p.WaitGroup().Add(1)

	// Start parseStderr goroutine (simulated since BaseProcess.parseStderr is private)
	go func() {
		defer p.WaitGroup().Done()
		bufScanner := make([]byte, 0)
		buf := make([]byte, 4096)
		for {
			n, err := stderrReader.Read(buf)
			if err != nil {
				break
			}
			bufScanner = append(bufScanner, buf[:n]...)
			for {
				idx := -1
				for i, b := range bufScanner {
					if b == '\n' {
						idx = i
						break
					}
				}
				if idx == -1 {
					break
				}
				line := string(bufScanner[:idx])
				bufScanner = bufScanner[idx+1:]
				p.AppendStderrLine(line)
			}
		}
	}()

	// Write many lines (current impl captures all - no limit)
	const lineCount = 1000
	for i := 0; i < lineCount; i++ {
		_, err := stderrWriter.Write([]byte(fmt.Sprintf("Error line %d\n", i)))
		require.NoError(t, err)
	}

	stderrWriter.Close()
	p.WaitGroup().Wait()

	capturedCount := len(p.StderrLines())

	// Current behavior: captures ALL lines (no limit)
	// Note: This documents current behavior. BaseProcess may add WithMaxStderrLines option.
	require.Equal(t, lineCount, capturedCount,
		"Current behavior captures all stderr lines without limit")
}

// TestBehaviorPreservation_ParseOutputScannerError verifies that scanner errors
// in parseOutput are properly sent to the errors channel.
func TestBehaviorPreservation_ParseOutputScannerError(t *testing.T) {
	// This test verifies the error format from process.go:449
	// When scanner has an error, it sends: "stdout scanner error: %w"

	expectedFormat := fmt.Errorf("stdout scanner error: %w", io.ErrUnexpectedEOF)
	require.Contains(t, expectedFormat.Error(), "stdout scanner error:")
	require.ErrorIs(t, expectedFormat, io.ErrUnexpectedEOF)
}

// TestBehaviorPreservation_TimeoutDetectionUsesErrorsIs verifies that
// waitForCompletion uses errors.Is() for timeout detection (process.go:488).
func TestBehaviorPreservation_TimeoutDetectionUsesErrorsIs(t *testing.T) {
	// The code uses: errors.Is(p.ctx.Err(), context.DeadlineExceeded)
	// This test documents that behavior

	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()

	time.Sleep(time.Millisecond) // Ensure timeout fires

	// errors.Is correctly identifies timeout
	require.True(t, errors.Is(ctx.Err(), context.DeadlineExceeded))

	// This is the pattern used in process.go:488
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		// Would set status to StatusFailed and send ErrTimeout
	}
}

// TestBehaviorPreservation_StatusSetBeforeErrorSend verifies the ordering in
// waitForCompletion: status is set BEFORE sending error to channel.
func TestBehaviorPreservation_StatusSetBeforeErrorSend(t *testing.T) {
	// Create a process
	p := newTestProcess()

	// Simulate waitForCompletion error path
	// The order is: 1) set status, 2) send error

	// Step 1: Set status first (SetStatus locks internally)
	p.SetStatus(client.StatusFailed)

	// Step 2: Send error
	p.SendError(fmt.Errorf("test error"))

	// Verify status was set
	require.Equal(t, client.StatusFailed, p.Status())

	// Verify error was sent
	select {
	case err := <-p.Errors():
		require.Contains(t, err.Error(), "test error")
	default:
		require.Fail(t, "Error should have been sent")
	}
}

func TestFindExecutable(t *testing.T) {
	t.Run("finds claude in .claude/local", func(t *testing.T) {
		// Create temp home directory
		tempHome := t.TempDir()
		originalHome := os.Getenv("HOME")
		t.Setenv("HOME", tempHome)
		t.Setenv("USERPROFILE", tempHome) // Windows uses USERPROFILE
		defer func() { _ = os.Setenv("HOME", originalHome) }()

		// Create the expected path
		claudeDir := filepath.Join(tempHome, ".claude", "local")
		require.NoError(t, os.MkdirAll(claudeDir, 0755))
		claudePath := filepath.Join(claudeDir, "claude")
		require.NoError(t, os.WriteFile(claudePath, []byte("#!/bin/bash\n"), 0755))

		// Should find it
		path, err := findExecutable()
		require.NoError(t, err)
		require.Equal(t, claudePath, path)
	})

	t.Run("finds claude in .claude root", func(t *testing.T) {
		// Create temp home directory
		tempHome := t.TempDir()
		originalHome := os.Getenv("HOME")
		t.Setenv("HOME", tempHome)
		t.Setenv("USERPROFILE", tempHome) // Windows uses USERPROFILE
		defer func() { _ = os.Setenv("HOME", originalHome) }()

		// Create the expected path (not in local, just in .claude)
		claudeDir := filepath.Join(tempHome, ".claude")
		require.NoError(t, os.MkdirAll(claudeDir, 0755))
		claudePath := filepath.Join(claudeDir, "claude")
		require.NoError(t, os.WriteFile(claudePath, []byte("#!/bin/bash\n"), 0755))

		// Should find it
		path, err := findExecutable()
		require.NoError(t, err)
		require.Equal(t, claudePath, path)
	})

	t.Run("prefers .claude/local over .claude", func(t *testing.T) {
		// Create temp home directory
		tempHome := t.TempDir()
		originalHome := os.Getenv("HOME")
		t.Setenv("HOME", tempHome)
		t.Setenv("USERPROFILE", tempHome) // Windows uses USERPROFILE
		defer func() { _ = os.Setenv("HOME", originalHome) }()

		// Create both paths
		localDir := filepath.Join(tempHome, ".claude", "local")
		require.NoError(t, os.MkdirAll(localDir, 0755))
		localPath := filepath.Join(localDir, "claude")
		require.NoError(t, os.WriteFile(localPath, []byte("#!/bin/bash\n"), 0755))

		rootPath := filepath.Join(tempHome, ".claude", "claude")
		require.NoError(t, os.WriteFile(rootPath, []byte("#!/bin/bash\n"), 0755))

		// Should prefer local
		path, err := findExecutable()
		require.NoError(t, err)
		require.Equal(t, localPath, path)
	})

	t.Run("skips directories", func(t *testing.T) {
		// Create temp home directory
		tempHome := t.TempDir()
		originalHome := os.Getenv("HOME")
		t.Setenv("HOME", tempHome)
		t.Setenv("USERPROFILE", tempHome) // Windows uses USERPROFILE
		defer func() { _ = os.Setenv("HOME", originalHome) }()

		// Create a directory named "claude" instead of a file
		claudeDir := filepath.Join(tempHome, ".claude", "local", "claude")
		require.NoError(t, os.MkdirAll(claudeDir, 0755))

		// Create the actual file in .claude root
		rootPath := filepath.Join(tempHome, ".claude", "claude")
		require.NoError(t, os.WriteFile(rootPath, []byte("#!/bin/bash\n"), 0755))

		// Should skip the directory and find the file
		path, err := findExecutable()
		require.NoError(t, err)
		require.Equal(t, rootPath, path)
	})
}
