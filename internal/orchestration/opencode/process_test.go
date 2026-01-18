package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/orchestration/client"
)

// errTest is a sentinel error for testing
var errTest = errors.New("test error")

// =============================================================================
// Executable Discovery Tests
// =============================================================================

func TestFindExecutable_LocalBinPath(t *testing.T) {
	// Create temp home directory with local bin opencode
	tempDir := t.TempDir()
	localBinDir := filepath.Join(tempDir, ".local", "bin")
	require.NoError(t, os.MkdirAll(localBinDir, 0755))

	// On Windows, executables need .exe extension; on Unix, no extension
	execName := "opencode"
	if os.PathSeparator == '\\' {
		execName = "opencode.exe"
	}
	opencodePath := filepath.Join(localBinDir, execName)
	require.NoError(t, os.WriteFile(opencodePath, []byte("#!/bin/bash\necho test"), 0755))

	// Override HOME/USERPROFILE for this test
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir)

	path, err := findExecutable()
	require.NoError(t, err)
	require.Equal(t, opencodePath, path)
}

func TestFindExecutable_PathFallback(t *testing.T) {
	// Set HOME to non-existent path to disable local bin check
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", "/non-existent-path")
	defer os.Setenv("HOME", originalHome)

	// This test depends on whether opencode is in PATH
	// If it is, it should be found; if not, test will verify error handling
	path, err := findExecutable()
	if err != nil {
		// opencode not in PATH, verify it's the expected error
		require.Equal(t, ErrNotFound, err)
	} else {
		// opencode found in PATH
		require.NotEmpty(t, path)
	}
}

func TestFindExecutable_NotFound(t *testing.T) {
	// Set HOME to non-existent path
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", "/non-existent-path-for-test")
	defer os.Setenv("HOME", originalHome)

	// Override PATH to empty
	originalPath := os.Getenv("PATH")
	os.Setenv("PATH", "")
	defer os.Setenv("PATH", originalPath)

	path, err := findExecutable()
	require.Error(t, err)
	require.Equal(t, ErrNotFound, err)
	require.Empty(t, path)
}

// =============================================================================
// JSONC Comment Stripping Tests
// =============================================================================

func TestStripJSONComments_BlockComments(t *testing.T) {
	input := []byte(`{"key": /* comment */ "value"}`)

	result := stripJSONComments(input)

	// Verify it's valid JSON after stripping
	var data map[string]any
	err := json.Unmarshal(result, &data)
	require.NoError(t, err)
	require.Equal(t, "value", data["key"])
}

func TestStripJSONComments_MultilineBlockComment(t *testing.T) {
	// Multi-line block comment
	input := []byte(`{"key": "value", /* this is
a multiline
comment */ "key2": "value2"}`)

	result := stripJSONComments(input)

	// Verify it's valid JSON after stripping
	var data map[string]any
	err := json.Unmarshal(result, &data)
	require.NoError(t, err)
	require.Equal(t, "value", data["key"])
	require.Equal(t, "value2", data["key2"])
}

func TestStripJSONComments_PreservesURLs(t *testing.T) {
	// Important: URLs contain // which should NOT be stripped
	input := []byte(`{"url": "https://example.com/path"}`)
	result := stripJSONComments(input)

	// URL should be preserved
	var data map[string]any
	err := json.Unmarshal(result, &data)
	require.NoError(t, err)
	require.Equal(t, "https://example.com/path", data["url"])
}

func TestStripJSONComments_NoComments(t *testing.T) {
	input := []byte(`{"key": "value", "number": 123}`)
	result := stripJSONComments(input)
	require.Equal(t, input, result)
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
		nil, // no cmd for test
		nil, // no stdout for test
		nil, // no stderr for test
		"/test/project",
		client.WithProviderName("opencode"),
		client.WithStderrCapture(true),
	)
	bp.SetSessionRef("opencode-test-session-12345")
	bp.SetStatus(client.StatusRunning)
	return &Process{BaseProcess: bp}
}

func TestProcess_ChannelBufferSizes(t *testing.T) {
	p := newTestProcess()

	// Events channel should have capacity 100 (access via EventsWritable for testing)
	require.Equal(t, 100, cap(p.EventsWritable()))

	// Errors channel should have capacity 10 (access via ErrorsWritable for testing)
	require.Equal(t, 10, cap(p.ErrorsWritable()))
}

func TestProcess_StatusTransitions_PendingToRunning(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bp := client.NewBaseProcess(
		ctx,
		cancel,
		nil, nil, nil,
		"/test",
		client.WithProviderName("opencode"),
	)
	p := &Process{BaseProcess: bp}

	require.Equal(t, client.StatusPending, p.Status())
	require.False(t, p.IsRunning())

	p.SetStatus(client.StatusRunning)
	require.Equal(t, client.StatusRunning, p.Status())
	require.True(t, p.IsRunning())
}

func TestProcess_StatusTransitions_RunningToCompleted(t *testing.T) {
	p := newTestProcess()

	require.Equal(t, client.StatusRunning, p.Status())
	require.True(t, p.IsRunning())

	p.SetStatus(client.StatusCompleted)
	require.Equal(t, client.StatusCompleted, p.Status())
	require.False(t, p.IsRunning())
}

func TestProcess_StatusTransitions_RunningToFailed(t *testing.T) {
	p := newTestProcess()

	require.Equal(t, client.StatusRunning, p.Status())
	require.True(t, p.IsRunning())

	p.SetStatus(client.StatusFailed)
	require.Equal(t, client.StatusFailed, p.Status())
	require.False(t, p.IsRunning())
}

func TestProcess_StatusTransitions_RunningToCancelled(t *testing.T) {
	p := newTestProcess()

	require.Equal(t, client.StatusRunning, p.Status())
	require.True(t, p.IsRunning())

	err := p.Cancel()
	require.NoError(t, err)
	require.Equal(t, client.StatusCancelled, p.Status())
	require.False(t, p.IsRunning())
}

func TestProcess_Cancel_TerminatesAndSetsStatus(t *testing.T) {
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

func TestProcess_Cancel_RacePrevention(t *testing.T) {
	// This test verifies that Cancel() sets status BEFORE calling cancelFunc,
	// preventing race conditions with goroutines that check status.
	// Run multiple iterations to catch potential race conditions.

	for i := 0; i < 100; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		bp := client.NewBaseProcess(
			ctx,
			cancel,
			nil, nil, nil,
			"/test",
			client.WithProviderName("opencode"),
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
		err := p.Cancel()
		require.NoError(t, err)

		wg.Wait()

		// The goroutine should have seen StatusCancelled, not StatusRunning
		require.Equal(t, client.StatusCancelled, observedStatus,
			"Goroutine should see StatusCancelled after context cancel (iteration %d)", i)
	}
}

func TestProcess_Cancel_DoesNotOverrideTerminalState(t *testing.T) {
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
				nil, nil, nil,
				"/test",
				client.WithProviderName("opencode"),
			)
			bp.SetStatus(tt.initialStatus)
			p := &Process{BaseProcess: bp}

			err := p.Cancel()
			require.NoError(t, err)
			require.Equal(t, tt.expectedStatus, p.Status())
		})
	}
}

func TestProcess_ContextTimeout_TriggersCancellation(t *testing.T) {
	// Create a context with a very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	bp := client.NewBaseProcess(
		ctx,
		cancel,
		nil, nil, nil,
		"/test",
		client.WithProviderName("opencode"),
	)
	bp.SetStatus(client.StatusRunning)
	p := &Process{BaseProcess: bp}

	// Wait for context to timeout
	<-p.Context().Done()

	// Verify context was cancelled due to deadline
	require.Equal(t, context.DeadlineExceeded, p.Context().Err())
}

func TestProcess_SessionRef_ReturnsSessionID(t *testing.T) {
	p := newTestProcess()

	// SessionRef should return the session ID
	require.Equal(t, "opencode-test-session-12345", p.SessionRef())
}

func TestProcess_SessionRef_InitiallyEmpty(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bp := client.NewBaseProcess(
		ctx,
		cancel,
		nil, nil, nil,
		"/test/project",
		client.WithProviderName("opencode"),
	)
	bp.SetStatus(client.StatusRunning)
	// Don't set session ID - it should be empty initially
	p := &Process{BaseProcess: bp}

	// SessionRef should be empty until init event is processed
	require.Equal(t, "", p.SessionRef())
}

func TestProcess_WorkDir(t *testing.T) {
	p := newTestProcess()
	require.Equal(t, "/test/project", p.WorkDir())
}

func TestProcess_PID_NilProcess(t *testing.T) {
	p := newTestProcess()
	// cmd is nil, so PID should return -1 (BaseProcess convention)
	require.Equal(t, -1, p.PID())
}

func TestProcess_Wait_BlocksUntilCompletion(t *testing.T) {
	p := newTestProcess()

	// Add a WaitGroup counter to simulate goroutines
	p.WaitGroup().Add(1)

	// Wait should block until wg is done
	done := make(chan bool)
	go func() {
		err := p.Wait()
		require.NoError(t, err)
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

func TestProcess_SendError_NonBlocking(t *testing.T) {
	// Create a process with a small error channel - use BaseProcess with custom setup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bp := client.NewBaseProcess(
		ctx,
		cancel,
		nil, nil, nil,
		"/test",
		client.WithProviderName("opencode"),
	)
	p := &Process{BaseProcess: bp}

	// Fill the channel (capacity is 10, fill with 10)
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

func TestProcess_SendError_SuccessWhenSpaceAvailable(t *testing.T) {
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

func TestProcess_EventsChannel(t *testing.T) {
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

func TestProcess_ErrorsChannel(t *testing.T) {
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
	var hp client.HeadlessProcess = newTestProcess()
	require.NotNil(t, hp)

	// Verify all interface methods are callable
	_ = hp.Events()
	_ = hp.Errors()
	_ = hp.SessionRef()
	_ = hp.Status()
	_ = hp.IsRunning()
	_ = hp.WorkDir()
	_ = hp.PID()
}

// =============================================================================
// Error Tests
// =============================================================================

func TestErrTimeout(t *testing.T) {
	require.NotNil(t, ErrTimeout)
	require.Contains(t, ErrTimeout.Error(), "timed out")
}

func TestErrNotFound(t *testing.T) {
	require.NotNil(t, ErrNotFound)
	require.Contains(t, ErrNotFound.Error(), "executable not found")
}

// =============================================================================
// OpenCode-Specific BaseProcess Integration Tests
// =============================================================================

func TestParser_ExtractSessionRef_FromAnyEvent(t *testing.T) {
	// CRITICAL: OpenCode extracts session ID from ANY event (not just init events)
	parser := NewParser()

	tests := []struct {
		name     string
		event    client.OutputEvent
		rawLine  []byte
		expected string
	}{
		{
			name:     "extracts session from text event",
			event:    client.OutputEvent{Type: client.EventAssistant, SessionID: "ses_abc123"},
			rawLine:  []byte(`{"type":"text","sessionID":"ses_abc123"}`),
			expected: "ses_abc123",
		},
		{
			name:     "extracts session from tool_use event",
			event:    client.OutputEvent{Type: client.EventToolUse, SessionID: "ses_def456"},
			rawLine:  []byte(`{"type":"tool_use","sessionID":"ses_def456"}`),
			expected: "ses_def456",
		},
		{
			name:     "extracts session from step_start event",
			event:    client.OutputEvent{Type: client.EventType("step_start"), SessionID: "ses_ghi789"},
			rawLine:  []byte(`{"type":"step_start","sessionID":"ses_ghi789"}`),
			expected: "ses_ghi789",
		},
		{
			name:     "extracts session from step_finish event",
			event:    client.OutputEvent{Type: client.EventType("step_finish"), SessionID: "ses_jkl012"},
			rawLine:  []byte(`{"type":"step_finish","sessionID":"ses_jkl012"}`),
			expected: "ses_jkl012",
		},
		{
			name:     "returns empty for event without sessionID",
			event:    client.OutputEvent{Type: client.EventAssistant},
			rawLine:  []byte(`{"type":"text"}`),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parser.ExtractSessionRef(tt.event, tt.rawLine)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestParser_EventParserRegisteredWithBaseProcess(t *testing.T) {
	// Verify that OpenCode uses WithEventParser which registers the parser
	// including the session extractor
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	parser := NewParser()
	bp := client.NewBaseProcess(
		ctx,
		cancel,
		nil, nil, nil,
		"/test",
		client.WithProviderName("opencode"),
		client.WithEventParser(parser),
	)

	// Verify the extractor is set (WithEventParser registers ExtractSessionRef)
	require.NotNil(t, bp.ExtractSessionFn())
}

func TestProcess_SessionExtraction_FromNonInitEvent(t *testing.T) {
	// This test verifies that session ID can be extracted from later events
	// (not just init), which is unique to OpenCode
	parser := NewParser()

	// Simulate receiving a text event with sessionID (not init)
	event := client.OutputEvent{
		Type:      client.EventAssistant,
		SessionID: "ses_from_text_event",
	}

	// Extract session - this should work because OpenCode extracts from any event
	sessionRef := parser.ExtractSessionRef(event, []byte(`{"type":"text","sessionID":"ses_from_text_event"}`))
	require.Equal(t, "ses_from_text_event", sessionRef)
}

func TestProcess_StderrCapture_Enabled(t *testing.T) {
	// Verify that OpenCode is configured with stderr capture
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bp := client.NewBaseProcess(
		ctx,
		cancel,
		nil, nil, nil,
		"/test",
		client.WithProviderName("opencode"),
		client.WithStderrCapture(true),
	)

	require.True(t, bp.CaptureStderr())
}

func TestProcess_ProviderName_IsOpenCode(t *testing.T) {
	// Verify the provider name is set correctly
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bp := client.NewBaseProcess(
		ctx,
		cancel,
		nil, nil, nil,
		"/test",
		client.WithProviderName("opencode"),
	)

	require.Equal(t, "opencode", bp.ProviderName())
}

func TestProcess_SessionID_Convenience(t *testing.T) {
	// Verify SessionID() is a convenience wrapper for SessionRef()
	p := newTestProcess()

	require.Equal(t, p.SessionRef(), p.SessionID())
	require.Equal(t, "opencode-test-session-12345", p.SessionID())
}

// =============================================================================
// MCP Configuration Tests
// =============================================================================

func TestSetupMCPConfig_EmptyMCPConfig_NoOp(t *testing.T) {
	tempDir := t.TempDir()

	cfg := Config{
		WorkDir:   tempDir,
		MCPConfig: "", // Empty - should be no-op
	}

	err := setupMCPConfig(cfg)
	require.NoError(t, err)

	// Verify opencode.jsonc was NOT created
	configPath := filepath.Join(tempDir, "opencode.jsonc")
	_, err = os.Stat(configPath)
	require.True(t, os.IsNotExist(err), "opencode.jsonc should not be created for empty MCPConfig")
}

func TestSetupMCPConfig_NewFile_Created(t *testing.T) {
	tempDir := t.TempDir()

	mcpConfig := `{
		"mcp": {
			"perles-worker": {
				"type": "remote",
				"url": "http://localhost:8080/worker/1"
			}
		}
	}`

	cfg := Config{
		WorkDir:   tempDir,
		MCPConfig: mcpConfig,
	}

	err := setupMCPConfig(cfg)
	require.NoError(t, err)

	// Verify opencode.jsonc was created
	configPath := filepath.Join(tempDir, "opencode.jsonc")
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)

	var settings map[string]any
	err = json.Unmarshal(data, &settings)
	require.NoError(t, err)

	// Verify mcp was written
	mcpServers, ok := settings["mcp"].(map[string]any)
	require.True(t, ok, "mcp should be a map")

	worker, ok := mcpServers["perles-worker"].(map[string]any)
	require.True(t, ok, "perles-worker should be a map")

	require.Equal(t, "http://localhost:8080/worker/1", worker["url"])
	require.Equal(t, "remote", worker["type"])
}

func TestSetupMCPConfig_ExistingSettings_AddsMCP(t *testing.T) {
	tempDir := t.TempDir()

	// Create existing opencode.jsonc without mcp
	existingSettings := `{
		"theme": "dark",
		"autoSave": true
	}`
	configPath := filepath.Join(tempDir, "opencode.jsonc")
	require.NoError(t, os.WriteFile(configPath, []byte(existingSettings), 0644))

	mcpConfig := `{
		"mcp": {
			"perles-worker": {
				"type": "remote",
				"url": "http://localhost:8080/worker/1"
			}
		}
	}`

	cfg := Config{
		WorkDir:   tempDir,
		MCPConfig: mcpConfig,
	}

	err := setupMCPConfig(cfg)
	require.NoError(t, err)

	// Verify opencode.jsonc was updated
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)

	var settings map[string]any
	err = json.Unmarshal(data, &settings)
	require.NoError(t, err)

	// Verify existing settings were preserved
	require.Equal(t, "dark", settings["theme"])
	require.Equal(t, true, settings["autoSave"])

	// Verify mcp was added
	mcpServers, ok := settings["mcp"].(map[string]any)
	require.True(t, ok, "mcp should be a map")

	worker, ok := mcpServers["perles-worker"].(map[string]any)
	require.True(t, ok, "perles-worker should be a map")

	require.Equal(t, "http://localhost:8080/worker/1", worker["url"])
}

func TestSetupMCPConfig_ExistingMCPServers_Merged(t *testing.T) {
	tempDir := t.TempDir()

	// Create existing opencode.jsonc with mcp servers
	existingSettings := `{
		"theme": "dark",
		"mcp": {
			"existing-server": {
				"type": "local",
				"command": ["node", "server.js"]
			}
		}
	}`
	configPath := filepath.Join(tempDir, "opencode.jsonc")
	require.NoError(t, os.WriteFile(configPath, []byte(existingSettings), 0644))

	// New config adds perles-worker
	mcpConfig := `{
		"mcp": {
			"perles-worker": {
				"type": "remote",
				"url": "http://localhost:8080/worker/1"
			}
		}
	}`

	cfg := Config{
		WorkDir:   tempDir,
		MCPConfig: mcpConfig,
	}

	err := setupMCPConfig(cfg)
	require.NoError(t, err)

	// Verify opencode.jsonc was updated
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)

	var settings map[string]any
	err = json.Unmarshal(data, &settings)
	require.NoError(t, err)

	// Verify existing settings preserved
	require.Equal(t, "dark", settings["theme"])

	// Verify both servers exist
	mcpServers, ok := settings["mcp"].(map[string]any)
	require.True(t, ok, "mcp should be a map")
	require.Len(t, mcpServers, 2, "should have both servers")

	// Verify existing server preserved
	existingServer, ok := mcpServers["existing-server"].(map[string]any)
	require.True(t, ok, "existing-server should be preserved")
	require.Equal(t, "local", existingServer["type"])

	// Verify new server added
	newServer, ok := mcpServers["perles-worker"].(map[string]any)
	require.True(t, ok, "perles-worker should be added")
	require.Equal(t, "http://localhost:8080/worker/1", newServer["url"])
}

func TestSetupMCPConfig_HandlesBlockComments(t *testing.T) {
	tempDir := t.TempDir()

	// Create existing opencode.jsonc with block comment (JSONC format)
	// Note: inline // comments after values can break JSON parsing, so we test block comments
	existingSettings := `{"$schema": "https://opencode.ai/config.json", /* comment */ "theme": "dark", "mcp": {"existing-server": {"type": "local"}}}`
	configPath := filepath.Join(tempDir, "opencode.jsonc")
	require.NoError(t, os.WriteFile(configPath, []byte(existingSettings), 0644))

	mcpConfig := `{
		"mcp": {
			"perles-worker": {
				"type": "remote",
				"url": "http://localhost:8080/worker/1"
			}
		}
	}`

	cfg := Config{
		WorkDir:   tempDir,
		MCPConfig: mcpConfig,
	}

	err := setupMCPConfig(cfg)
	require.NoError(t, err)

	// Verify opencode.jsonc was updated (note: comments will be stripped in output)
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)

	var settings map[string]any
	err = json.Unmarshal(data, &settings)
	require.NoError(t, err)

	// Verify settings were preserved
	require.Equal(t, "https://opencode.ai/config.json", settings["$schema"])
	require.Equal(t, "dark", settings["theme"])

	// Verify both servers exist
	mcpServers, ok := settings["mcp"].(map[string]any)
	require.True(t, ok, "mcp should be a map")
	require.Len(t, mcpServers, 2, "should have both servers")
}

func TestSetupMCPConfig_InvalidMCPConfigJSON_ReturnsError(t *testing.T) {
	tempDir := t.TempDir()

	cfg := Config{
		WorkDir:   tempDir,
		MCPConfig: "{ invalid json }",
	}

	err := setupMCPConfig(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to parse MCPConfig JSON")
}

func TestSetupMCPConfig_InvalidExistingSettingsJSON_ReturnsError(t *testing.T) {
	tempDir := t.TempDir()

	// Create existing opencode.jsonc with invalid JSON (even after comment stripping)
	configPath := filepath.Join(tempDir, "opencode.jsonc")
	require.NoError(t, os.WriteFile(configPath, []byte("{ invalid json without comment style }"), 0644))

	mcpConfig := `{
		"mcp": {
			"perles-worker": {
				"type": "remote",
				"url": "http://localhost:8080/worker/1"
			}
		}
	}`

	cfg := Config{
		WorkDir:   tempDir,
		MCPConfig: mcpConfig,
	}

	err := setupMCPConfig(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to parse existing opencode.jsonc")

	// Verify original file was NOT modified (no corruption)
	data, readErr := os.ReadFile(configPath)
	require.NoError(t, readErr)
	require.Equal(t, "{ invalid json without comment style }", string(data), "original file should not be corrupted")
}

func TestSetupMCPConfig_ProperFormatting(t *testing.T) {
	tempDir := t.TempDir()

	mcpConfig := `{"mcp":{"perles-worker":{"type":"remote","url":"http://localhost:8080/worker/1"}}}`

	cfg := Config{
		WorkDir:   tempDir,
		MCPConfig: mcpConfig,
	}

	err := setupMCPConfig(cfg)
	require.NoError(t, err)

	// Verify opencode.jsonc is properly formatted (indented)
	configPath := filepath.Join(tempDir, "opencode.jsonc")
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)

	// Should contain indentation (2 spaces)
	require.Contains(t, string(data), "  ")
	// Should have newlines for formatting
	require.Contains(t, string(data), "\n")
}

func TestSetupMCPConfig_OverwritesSameServer(t *testing.T) {
	tempDir := t.TempDir()

	// Create existing opencode.jsonc with perles-worker
	existingSettings := `{"mcp": {"perles-worker": {"type": "remote", "url": "http://localhost:OLD/worker/OLD"}}}`
	configPath := filepath.Join(tempDir, "opencode.jsonc")
	require.NoError(t, os.WriteFile(configPath, []byte(existingSettings), 0644))

	// New config updates perles-worker
	mcpConfig := `{
		"mcp": {
			"perles-worker": {
				"type": "remote",
				"url": "http://localhost:8080/worker/NEW"
			}
		}
	}`

	cfg := Config{
		WorkDir:   tempDir,
		MCPConfig: mcpConfig,
	}

	err := setupMCPConfig(cfg)
	require.NoError(t, err)

	// Verify opencode.jsonc was updated
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)

	var settings map[string]any
	err = json.Unmarshal(data, &settings)
	require.NoError(t, err)

	mcpServers, ok := settings["mcp"].(map[string]any)
	require.True(t, ok)
	require.Len(t, mcpServers, 1, "should only have one server")

	worker, ok := mcpServers["perles-worker"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "http://localhost:8080/worker/NEW", worker["url"], "should be updated URL")
}
