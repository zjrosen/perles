package amp

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/orchestration/client"
)

// errTest is a sentinel error for testing
var errTest = errors.New("test error")

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
		nil, // no cmd
		nil, // no stdout
		nil, // no stderr
		"/test/project",
		client.WithProviderName("amp"),
	)
	bp.SetSessionRef("T-abc123-def456")
	bp.SetStatus(client.StatusRunning)
	return &Process{BaseProcess: bp}
}

func TestProcessLifecycle_StatusTransitions_PendingToRunning(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bp := client.NewBaseProcess(
		ctx,
		cancel,
		nil,
		nil,
		nil,
		"/test/project",
		client.WithProviderName("amp"),
	)
	// Status starts as Pending by default in NewBaseProcess
	p := &Process{BaseProcess: bp}

	require.Equal(t, client.StatusPending, p.Status())
	require.False(t, p.IsRunning())

	bp.SetStatus(client.StatusRunning)
	require.Equal(t, client.StatusRunning, p.Status())
	require.True(t, p.IsRunning())
}

func TestProcessLifecycle_StatusTransitions_RunningToCompleted(t *testing.T) {
	p := newTestProcess()

	require.Equal(t, client.StatusRunning, p.Status())
	require.True(t, p.IsRunning())

	p.BaseProcess.SetStatus(client.StatusCompleted)
	require.Equal(t, client.StatusCompleted, p.Status())
	require.False(t, p.IsRunning())
}

func TestProcessLifecycle_StatusTransitions_RunningToFailed(t *testing.T) {
	p := newTestProcess()

	require.Equal(t, client.StatusRunning, p.Status())
	require.True(t, p.IsRunning())

	p.BaseProcess.SetStatus(client.StatusFailed)
	require.Equal(t, client.StatusFailed, p.Status())
	require.False(t, p.IsRunning())
}

func TestProcessLifecycle_StatusTransitions_RunningToCancelled(t *testing.T) {
	p := newTestProcess()

	require.Equal(t, client.StatusRunning, p.Status())
	require.True(t, p.IsRunning())

	err := p.Cancel()
	require.NoError(t, err)
	require.Equal(t, client.StatusCancelled, p.Status())
	require.False(t, p.IsRunning())
}

func TestProcessLifecycle_Cancel_TerminatesAndSetsStatus(t *testing.T) {
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

func TestProcessLifecycle_Cancel_RacePrevention(t *testing.T) {
	// This test verifies that Cancel() sets status BEFORE calling cancelFunc,
	// preventing race conditions with goroutines that check status.
	// Run multiple iterations to catch potential race conditions.

	for i := 0; i < 100; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		bp := client.NewBaseProcess(
			ctx,
			cancel,
			nil,
			nil,
			nil,
			"/test/project",
			client.WithProviderName("amp"),
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
			// Immediately check status - should already be StatusCancelled
			observedStatus = p.Status()
		}()

		// Small sleep to ensure goroutine is waiting
		time.Sleep(time.Microsecond)

		// Cancel the process
		p.Cancel()

		wg.Wait()

		// The goroutine should have seen StatusCancelled, not StatusRunning
		require.Equal(t, client.StatusCancelled, observedStatus,
			"Goroutine should see StatusCancelled after context cancel (iteration %d)", i)
	}
}

func TestProcessLifecycle_Cancel_DoesNotOverrideTerminalState(t *testing.T) {
	tests := []struct {
		name           string
		initialStatus  client.ProcessStatus
		expectedStatus client.ProcessStatus
	}{
		{
			name:           "does not override completed",
			initialStatus:  client.StatusCompleted,
			expectedStatus: client.StatusCompleted,
		},
		{
			name:           "does not override failed",
			initialStatus:  client.StatusFailed,
			expectedStatus: client.StatusFailed,
		},
		{
			name:           "does not override already cancelled",
			initialStatus:  client.StatusCancelled,
			expectedStatus: client.StatusCancelled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			bp := client.NewBaseProcess(
				ctx,
				cancel,
				nil,
				nil,
				nil,
				"/test/project",
				client.WithProviderName("amp"),
			)
			bp.SetStatus(tt.initialStatus)
			p := &Process{BaseProcess: bp}

			err := p.Cancel()
			require.NoError(t, err)
			require.Equal(t, tt.expectedStatus, p.Status())
		})
	}
}

func TestProcessLifecycle_ThreadID(t *testing.T) {
	p := newTestProcess()

	// ThreadID should return the session ref (Amp's thread ID)
	require.Equal(t, "T-abc123-def456", p.ThreadID())
	// SessionRef should return the same value
	require.Equal(t, "T-abc123-def456", p.SessionRef())
}

func TestProcessLifecycle_SessionRef_InitiallyEmpty(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bp := client.NewBaseProcess(
		ctx,
		cancel,
		nil,
		nil,
		nil,
		"/test/project",
		client.WithProviderName("amp"),
	)
	bp.SetStatus(client.StatusRunning)
	// Don't set session ref
	p := &Process{BaseProcess: bp}

	// SessionRef should be empty until init event is processed
	require.Equal(t, "", p.SessionRef())
	require.Equal(t, "", p.ThreadID())
}

func TestProcessLifecycle_WorkDir(t *testing.T) {
	p := newTestProcess()
	require.Equal(t, "/test/project", p.WorkDir())
}

func TestProcessLifecycle_PID_NilProcess(t *testing.T) {
	p := newTestProcess()
	// cmd is nil, so PID should return -1 (BaseProcess returns -1 for nil)
	require.Equal(t, -1, p.PID())
}

func TestProcessLifecycle_Wait_BlocksUntilCompletion(t *testing.T) {
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

func TestProcessLifecycle_SendError_NonBlocking(t *testing.T) {
	p := newTestProcess()

	// Fill the channel (capacity is 10)
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
		// Expected - sendError returned without blocking
	case <-time.After(100 * time.Millisecond):
		require.Fail(t, "sendError blocked on full channel - should have dropped error")
	}

	// Original errors should still be in channel
	require.Len(t, p.ErrorsWritable(), 10)
}

func TestProcessLifecycle_SendError_SuccessWhenSpaceAvailable(t *testing.T) {
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

func TestProcessLifecycle_EventsChannelCapacity(t *testing.T) {
	p := newTestProcess()

	// Events channel should have capacity 100
	require.Equal(t, 100, cap(p.EventsWritable()))
}

func TestProcessLifecycle_ErrorsChannelCapacity(t *testing.T) {
	p := newTestProcess()

	// Errors channel should have capacity 10
	require.Equal(t, 10, cap(p.ErrorsWritable()))
}

func TestProcessLifecycle_EventsChannel(t *testing.T) {
	p := newTestProcess()

	// Events channel should be readable
	eventsCh := p.Events()
	require.NotNil(t, eventsCh)

	// Send an event
	go func() {
		p.EventsWritable() <- client.OutputEvent{Type: client.EventSystem, SubType: "init"}
	}()

	select {
	case event := <-eventsCh:
		require.Equal(t, client.EventSystem, event.Type)
		require.Equal(t, "init", event.SubType)
	case <-time.After(time.Second):
		require.Fail(t, "Timeout waiting for event")
	}
}

func TestProcessLifecycle_ErrorsChannel(t *testing.T) {
	p := newTestProcess()

	// Errors channel should be readable
	errorsCh := p.Errors()
	require.NotNil(t, errorsCh)

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

// =============================================================================
// Interface Compliance Tests
// =============================================================================

func TestProcess_ImplementsHeadlessProcess(t *testing.T) {
	// This test verifies at runtime that Process implements HeadlessProcess.
	// The compile-time check in process.go handles this, but this provides
	// additional runtime verification.
	var p client.HeadlessProcess = newTestProcess()
	require.NotNil(t, p)

	// Verify all interface methods are callable
	_ = p.Events()
	_ = p.Errors()
	_ = p.SessionRef()
	_ = p.Status()
	_ = p.IsRunning()
	_ = p.WorkDir()
	_ = p.PID()
}

// =============================================================================
// ErrTimeout Tests
// =============================================================================

func TestAmpErrTimeout(t *testing.T) {
	require.NotNil(t, ErrTimeout)
	require.Contains(t, ErrTimeout.Error(), "timed out")
}

// =============================================================================
// extractSession Tests
// =============================================================================

func TestExtractSession(t *testing.T) {
	tests := []struct {
		name     string
		event    client.OutputEvent
		rawLine  []byte
		expected string
	}{
		{
			name: "extracts thread ID from init event",
			event: client.OutputEvent{
				Type:      client.EventSystem,
				SubType:   "init",
				SessionID: "T-abc123",
			},
			rawLine:  []byte(`{"type":"system","subtype":"init","session_id":"T-abc123"}`),
			expected: "T-abc123",
		},
		{
			name: "returns empty for non-init event",
			event: client.OutputEvent{
				Type:      client.EventAssistant,
				SessionID: "T-abc123",
			},
			rawLine:  []byte(`{"type":"assistant"}`),
			expected: "",
		},
		{
			name: "returns empty for init event without session ID",
			event: client.OutputEvent{
				Type:    client.EventSystem,
				SubType: "init",
			},
			rawLine:  []byte(`{"type":"system","subtype":"init"}`),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractSession(tt.event, tt.rawLine)
			require.Equal(t, tt.expected, result)
		})
	}
}

// =============================================================================
// buildArgs Tests (existing tests below)
// =============================================================================

func TestBuildArgs(t *testing.T) {
	tests := []struct {
		name     string
		cfg      Config
		isResume bool
		expected []string
	}{
		{
			name: "minimal config",
			cfg: Config{
				WorkDir: "/project",
			},
			isResume: false,
			expected: []string{
				"-x", "--stream-json",
				"--no-notifications",
			},
		},
		{
			name: "with skip permissions",
			cfg: Config{
				WorkDir:         "/project",
				SkipPermissions: true,
			},
			isResume: false,
			expected: []string{
				"-x", "--stream-json",
				"--dangerously-allow-all",
				"--no-notifications",
			},
		},
		{
			name: "with disable IDE",
			cfg: Config{
				WorkDir:    "/project",
				DisableIDE: true,
			},
			isResume: false,
			expected: []string{
				"-x", "--stream-json",
				"--no-notifications",
				"--no-ide",
			},
		},
		{
			name: "with sonnet model",
			cfg: Config{
				WorkDir: "/project",
				Model:   "sonnet",
			},
			isResume: false,
			expected: []string{
				"-x", "--stream-json",
				"--no-notifications",
				"--use-sonnet",
			},
		},
		{
			name: "with mode",
			cfg: Config{
				WorkDir: "/project",
				Mode:    "rush",
			},
			isResume: false,
			expected: []string{
				"-x", "--stream-json",
				"--no-notifications",
				"-m", "rush",
			},
		},
		{
			name: "with MCP config",
			cfg: Config{
				WorkDir:   "/project",
				MCPConfig: `{"servers":{}}`,
			},
			isResume: false,
			expected: []string{
				"-x", "--stream-json",
				"--no-notifications",
				"--mcp-config", `{"servers":{}}`,
			},
		},
		{
			name: "resume with thread ID",
			cfg: Config{
				WorkDir:  "/project",
				ThreadID: "T-abc123",
			},
			isResume: true,
			expected: []string{
				"threads", "continue", "T-abc123",
				"-x", "--stream-json",
				"--no-notifications",
			},
		},
		{
			name: "full config",
			cfg: Config{
				WorkDir:         "/project",
				ThreadID:        "T-xyz789",
				Model:           "sonnet",
				Mode:            "smart",
				SkipPermissions: true,
				DisableIDE:      true,
				MCPConfig:       `{"mcpServers":{}}`,
			},
			isResume: true,
			expected: []string{
				"threads", "continue", "T-xyz789",
				"-x", "--stream-json",
				"--dangerously-allow-all",
				"--no-notifications",
				"--no-ide",
				"--use-sonnet",
				"-m", "smart",
				"--mcp-config", `{"mcpServers":{}}`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := buildArgs(tt.cfg, tt.isResume)
			require.Equal(t, tt.expected, args)
		})
	}
}

func TestParseEvent(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		validate func(t *testing.T, e client.OutputEvent)
	}{
		{
			name: "system init event",
			json: `{"type":"system","subtype":"init","session_id":"T-abc123","cwd":"/project","tools":["Bash","Read"]}`,
			validate: func(t *testing.T, e client.OutputEvent) {
				require.Equal(t, client.EventSystem, e.Type)
				require.Equal(t, "init", e.SubType)
				require.Equal(t, "T-abc123", e.SessionID)
				require.Equal(t, "/project", e.WorkDir)
				require.True(t, e.IsInit())
				require.False(t, e.IsAssistant())
			},
		},
		{
			name: "assistant message with text",
			json: `{"type":"assistant","message":{"id":"msg_1","role":"assistant","content":[{"type":"text","text":"Hello from Amp!"}],"model":"claude-sonnet-4"}}`,
			validate: func(t *testing.T, e client.OutputEvent) {
				require.Equal(t, client.EventAssistant, e.Type)
				require.True(t, e.IsAssistant())
				require.NotNil(t, e.Message)
				require.Equal(t, "assistant", e.Message.Role)
				require.Equal(t, "claude-sonnet-4", e.Message.Model)
				require.Equal(t, "Hello from Amp!", e.Message.GetText())
			},
		},
		{
			name: "assistant message with tool use",
			json: `{"type":"assistant","message":{"id":"msg_2","content":[{"type":"tool_use","id":"toolu_123","name":"Bash","input":{"cmd":"ls -la"}}]}}`,
			validate: func(t *testing.T, e client.OutputEvent) {
				require.Equal(t, client.EventAssistant, e.Type)
				require.True(t, e.IsAssistant())
				require.NotNil(t, e.Message)
				require.True(t, e.Message.HasToolUses())
				tools := e.Message.GetToolUses()
				require.Len(t, tools, 1)
				require.Equal(t, "Bash", tools[0].Name)
				require.Equal(t, "toolu_123", tools[0].ID)
			},
		},
		{
			name: "user/tool_result event",
			json: `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_123","content":"success"}]}}`,
			validate: func(t *testing.T, e client.OutputEvent) {
				require.Equal(t, client.EventToolResult, e.Type)
				require.True(t, e.IsToolResult())
			},
		},
		{
			name: "result success event",
			json: `{"type":"result","subtype":"success","duration_ms":5000,"is_error":false,"num_turns":3,"result":"Task completed","session_id":"T-abc123"}`,
			validate: func(t *testing.T, e client.OutputEvent) {
				require.Equal(t, client.EventResult, e.Type)
				require.Equal(t, "success", e.SubType)
				require.True(t, e.IsResult())
				require.False(t, e.IsErrorResult)
				require.Equal(t, "Task completed", e.Result)
				require.Equal(t, int64(5000), e.DurationMs)
				require.Equal(t, "T-abc123", e.SessionID)
			},
		},
		{
			name: "result error event - context exceeded",
			json: `{"type":"result","subtype":"error","is_error":true,"result":"Context window exceeded"}`,
			validate: func(t *testing.T, e client.OutputEvent) {
				require.Equal(t, client.EventResult, e.Type)
				require.True(t, e.IsResult())
				require.True(t, e.IsErrorResult)
				require.Equal(t, "Context window exceeded", e.Result)
				// Verify context exceeded detection
				require.NotNil(t, e.Error, "Error should be populated for context exceeded")
				require.Equal(t, client.ErrReasonContextExceeded, e.Error.Reason)
				require.True(t, e.Error.IsContextExceeded())
				require.Equal(t, "Context window exceeded", e.Error.Message)
			},
		},
		{
			name: "result error event - prompt too long",
			json: `{"type":"result","subtype":"error","is_error":true,"result":"Prompt is too long: 201234 tokens > 200000 maximum"}`,
			validate: func(t *testing.T, e client.OutputEvent) {
				require.Equal(t, client.EventResult, e.Type)
				require.True(t, e.IsErrorResult)
				require.NotNil(t, e.Error)
				require.Equal(t, client.ErrReasonContextExceeded, e.Error.Reason)
				require.True(t, e.Error.IsContextExceeded())
			},
		},
		{
			name: "result error event - non-context error",
			json: `{"type":"result","subtype":"error","is_error":true,"result":"Connection failed"}`,
			validate: func(t *testing.T, e client.OutputEvent) {
				require.Equal(t, client.EventResult, e.Type)
				require.True(t, e.IsErrorResult)
				// Non-context errors should NOT set context exceeded
				if e.Error != nil {
					require.NotEqual(t, client.ErrReasonContextExceeded, e.Error.Reason)
				}
			},
		},
		{
			name: "error event",
			json: `{"type":"error","error":{"message":"Something went wrong","code":"INTERNAL"}}`,
			validate: func(t *testing.T, e client.OutputEvent) {
				require.Equal(t, client.EventError, e.Type)
				require.NotNil(t, e.Error)
				require.Equal(t, "Something went wrong", e.Error.Message)
				require.Equal(t, "INTERNAL", e.Error.Code)
			},
		},
		{
			name: "assistant with usage info",
			json: `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Done"}],"usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":1000,"cache_creation_input_tokens":500,"max_tokens":168000}}}`,
			validate: func(t *testing.T, e client.OutputEvent) {
				require.Equal(t, client.EventAssistant, e.Type)
				require.NotNil(t, e.Usage)
				// TokensUsed = input + cache_read + cache_creation = 100 + 1000 + 500 = 1600
				require.Equal(t, 1600, e.Usage.TokensUsed)
				require.Equal(t, 50, e.Usage.OutputTokens)
				require.Equal(t, 200000, e.Usage.TotalTokens) // Default context window
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

func TestParseEvent_InvalidJSON(t *testing.T) {
	_, err := parseEvent([]byte("not json"))
	require.Error(t, err)
}

func TestMapEventType(t *testing.T) {
	tests := []struct {
		input    string
		expected client.EventType
	}{
		{"system", client.EventSystem},
		{"assistant", client.EventAssistant},
		{"user", client.EventToolResult}, // Amp uses "user" for tool results
		{"result", client.EventResult},
		{"error", client.EventError},
		{"unknown", client.EventType("unknown")},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := mapEventType(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestConfigFromClient(t *testing.T) {
	tests := []struct {
		name     string
		cfg      client.Config
		validate func(t *testing.T, c Config)
	}{
		{
			name: "basic config",
			cfg: client.Config{
				WorkDir: "/project",
				Prompt:  "Hello",
			},
			validate: func(t *testing.T, c Config) {
				require.Equal(t, "/project", c.WorkDir)
				require.Equal(t, "Hello", c.Prompt)
				require.True(t, c.DisableIDE) // Always disabled in headless mode
			},
		},
		{
			name: "with system prompt prepended",
			cfg: client.Config{
				WorkDir:      "/project",
				SystemPrompt: "You are helpful",
				Prompt:       "Hello",
			},
			validate: func(t *testing.T, c Config) {
				require.Equal(t, "You are helpful\n\nHello", c.Prompt)
			},
		},
		{
			name: "session ID maps to thread ID",
			cfg: client.Config{
				WorkDir:   "/project",
				SessionID: "session-123",
			},
			validate: func(t *testing.T, c Config) {
				require.Equal(t, "session-123", c.ThreadID)
			},
		},
		{
			name: "skip permissions",
			cfg: client.Config{
				WorkDir:         "/project",
				SkipPermissions: true,
			},
			validate: func(t *testing.T, c Config) {
				require.True(t, c.SkipPermissions)
			},
		},
		{
			name: "MCP config",
			cfg: client.Config{
				WorkDir:   "/project",
				MCPConfig: `{"servers":{}}`,
			},
			validate: func(t *testing.T, c Config) {
				require.Equal(t, `{"servers":{}}`, c.MCPConfig)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ampCfg := configFromClient(tt.cfg)
			tt.validate(t, ampCfg)
		})
	}
}

func TestToolExtraction(t *testing.T) {
	// Test that tool_use content blocks are properly extracted
	jsonStr := `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_abc","name":"Read","input":{"path":"/file.txt"}}]}}`

	event, err := parseEvent([]byte(jsonStr))
	require.NoError(t, err)
	require.NotNil(t, event.Tool)
	require.Equal(t, "toolu_abc", event.Tool.ID)
	require.Equal(t, "Read", event.Tool.Name)

	// Verify input is preserved as raw JSON
	var input struct {
		Path string `json:"path"`
	}
	err = json.Unmarshal(event.Tool.Input, &input)
	require.NoError(t, err)
	require.Equal(t, "/file.txt", input.Path)
}

func TestMultipleToolUses(t *testing.T) {
	// Test that multiple tool_use blocks are detected
	jsonStr := `{"type":"assistant","message":{"content":[
		{"type":"text","text":"Let me check both files."},
		{"type":"tool_use","id":"toolu_1","name":"Read","input":{"path":"a.go"}},
		{"type":"tool_use","id":"toolu_2","name":"Read","input":{"path":"b.go"}}
	]}}`

	event, err := parseEvent([]byte(jsonStr))
	require.NoError(t, err)
	require.NotNil(t, event.Message)
	require.True(t, event.Message.HasToolUses())

	tools := event.Message.GetToolUses()
	require.Len(t, tools, 2)
	require.Equal(t, "toolu_1", tools[0].ID)
	require.Equal(t, "toolu_2", tools[1].ID)
}

func TestIsContextExceededMessage(t *testing.T) {
	tests := []struct {
		msg      string
		expected bool
	}{
		// Should detect context exceeded
		{"Prompt is too long", true},
		{"Prompt is too long: 201234 tokens > 200000 maximum", true},
		{"Context window exceeded", true},
		{"context exceeded", true},
		{"The context limit has been reached", true},
		{"Token limit exceeded", true},
		{"token limit reached", true},

		// Case insensitive
		{"PROMPT IS TOO LONG", true},
		{"CONTEXT WINDOW EXCEEDED", true},

		// Should NOT detect as context exceeded
		{"Connection failed", false},
		{"Rate limit exceeded", false},
		{"Invalid request", false},
		{"", false},
		{"Something went wrong", false},
		{"Error processing request", false},
	}

	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			result := isContextExceededMessage(tt.msg)
			require.Equal(t, tt.expected, result, "isContextExceededMessage(%q)", tt.msg)
		})
	}
}

func TestParseEvent_ErrorEventWithContextExceeded(t *testing.T) {
	// Test error event type with context exceeded message
	jsonStr := `{"type":"error","error":{"message":"Prompt is too long: 250000 tokens","code":"CONTEXT_EXCEEDED"}}`

	event, err := parseEvent([]byte(jsonStr))
	require.NoError(t, err)
	require.Equal(t, client.EventError, event.Type)
	require.NotNil(t, event.Error)
	require.Equal(t, client.ErrReasonContextExceeded, event.Error.Reason)
	require.True(t, event.Error.IsContextExceeded())
}

func TestParseEvent_ErrorAsStringWithEmbeddedJSON(t *testing.T) {
	// Test the exact format seen in production:
	// error is a string containing "413 {...json...}"
	jsonStr := `{"type":"result","subtype":"error_during_execution","duration_ms":389584,"is_error":true,"num_turns":28,"error":"413 {\"type\":\"error\",\"error\":{\"type\":\"invalid_request_error\",\"message\":\"Prompt is too long\"},\"request_id\":\"req_vrtx_011CXDs3LJPo57WcNsT9h9bs\"}","session_id":"T-019bce63-7de3-73a4-93a7-c1b84e61411e"}`

	event, err := parseEvent([]byte(jsonStr))
	require.NoError(t, err)
	require.Equal(t, client.EventResult, event.Type)
	require.Equal(t, "error_during_execution", event.SubType)
	require.True(t, event.IsErrorResult)
	require.NotNil(t, event.Error, "Error should be parsed from string")
	require.Equal(t, "Prompt is too long", event.Error.Message)
	require.Equal(t, "invalid_request_error", event.Error.Code)
	require.Equal(t, client.ErrReasonContextExceeded, event.Error.Reason)
	require.True(t, event.Error.IsContextExceeded())
}

func TestParseErrorField(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantMessage string
		wantCode    string
	}{
		{
			name:        "nil input",
			input:       "",
			wantMessage: "",
		},
		{
			name:        "object with message and code",
			input:       `{"message":"Something went wrong","code":"INTERNAL"}`,
			wantMessage: "Something went wrong",
			wantCode:    "INTERNAL",
		},
		{
			name:        "string error with embedded JSON",
			input:       `"413 {\"type\":\"error\",\"error\":{\"type\":\"invalid_request_error\",\"message\":\"Prompt is too long\"}}"`,
			wantMessage: "Prompt is too long",
			wantCode:    "invalid_request_error",
		},
		{
			name:        "plain string error",
			input:       `"Connection refused"`,
			wantMessage: "Connection refused",
			wantCode:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var raw json.RawMessage
			if tt.input != "" {
				raw = json.RawMessage(tt.input)
			}
			result := parseErrorField(raw)

			if tt.wantMessage == "" {
				require.Nil(t, result)
				return
			}

			require.NotNil(t, result)
			require.Equal(t, tt.wantMessage, result.Message)
			require.Equal(t, tt.wantCode, result.Code)
		})
	}
}
