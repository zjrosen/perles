//go:build opencode_integration

// Package opencode provides integration tests for the OpenCode CLI provider.
// These tests require the opencode CLI to be installed and available in PATH.
//
// Run with: go test -tags=opencode_integration ./internal/orchestration/opencode/...
package opencode

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/orchestration/client"
)

// skipIfOpenCodeNotAvailable skips the test if opencode CLI is not installed.
func skipIfOpenCodeNotAvailable(t *testing.T) {
	t.Helper()
	_, err := findExecutable()
	if err != nil {
		t.Skip("opencode CLI not available, skipping integration test")
	}
}

// skipIfOpenCodeNotConfigured skips the test if opencode is not properly configured.
// This checks for common configuration requirements like API keys or auth tokens.
func skipIfOpenCodeNotConfigured(t *testing.T) {
	t.Helper()
	// Try running opencode --version to check if it's properly configured
	path, err := findExecutable()
	if err != nil {
		t.Skip("opencode CLI not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, "--version")
	if err := cmd.Run(); err != nil {
		t.Skipf("opencode CLI not properly configured: %v", err)
	}
}

// =============================================================================
// Integration Tests - Require opencode CLI
// =============================================================================

// TestIntegration_Spawn_ReceivesInitEvent tests that spawning an OpenCode process
// receives an init event with session ID.
func TestIntegration_Spawn_ReceivesInitEvent(t *testing.T) {
	skipIfOpenCodeNotAvailable(t)
	skipIfOpenCodeNotConfigured(t)

	// Create a temporary work directory
	workDir := t.TempDir()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := Config{
		WorkDir: workDir,
		Prompt:  "Say 'test' and nothing else",
		Model:   "anthropic/claude-opus-4-5",
		Timeout: 30 * time.Second,
	}

	process, err := Spawn(ctx, cfg)
	require.NoError(t, err, "Failed to spawn opencode process")
	require.NotNil(t, process)

	defer func() {
		_ = process.Cancel()
		_ = process.Wait()
	}()

	// Verify process is running
	require.Equal(t, client.StatusRunning, process.Status())
	require.True(t, process.IsRunning())
	require.Greater(t, process.PID(), 0, "Process should have a valid PID")

	// Wait for init event with session ID
	eventCh := process.Events()
	foundInit := false
	timeout := time.After(25 * time.Second)

	for !foundInit {
		select {
		case event, ok := <-eventCh:
			if !ok {
				// Channel closed, check if we got the init event
				if !foundInit {
					t.Fatal("Events channel closed without receiving init event")
				}
				return
			}
			t.Logf("Received event: type=%s subtype=%s", event.Type, event.SubType)

			// Check for init event
			if event.Type == client.EventSystem && event.SubType == "init" {
				foundInit = true
				// Session ID should be extracted
				sessionID := process.SessionRef()
				t.Logf("Init event received, session ID: %s", sessionID)
				// Session ID may or may not be populated depending on OpenCode's response format
			}

		case <-timeout:
			if !foundInit {
				t.Fatal("Timeout waiting for init event")
			}
			return

		case <-ctx.Done():
			t.Fatal("Context cancelled while waiting for init event")
		}
	}

	require.True(t, foundInit, "Should have received init event")
}

// TestIntegration_Spawn_ReceivesAssistantEvents tests that the process receives
// assistant response events.
func TestIntegration_Spawn_ReceivesAssistantEvents(t *testing.T) {
	skipIfOpenCodeNotAvailable(t)
	skipIfOpenCodeNotConfigured(t)

	workDir := t.TempDir()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cfg := Config{
		WorkDir: workDir,
		Prompt:  "Reply with exactly one word: hello",
		Model:   "anthropic/claude-opus-4-5",
		Timeout: 60 * time.Second,
	}

	process, err := Spawn(ctx, cfg)
	require.NoError(t, err)
	defer func() {
		_ = process.Cancel()
		_ = process.Wait()
	}()

	// Collect events until completion or timeout
	eventCh := process.Events()
	var receivedEvents []client.OutputEvent
	timeout := time.After(55 * time.Second)

eventLoop:
	for {
		select {
		case event, ok := <-eventCh:
			if !ok {
				break eventLoop
			}
			receivedEvents = append(receivedEvents, event)
			t.Logf("Event: type=%s subtype=%s", event.Type, event.SubType)

			// Break on result event (completion)
			if event.Type == client.EventResult {
				break eventLoop
			}

		case <-timeout:
			break eventLoop

		case <-ctx.Done():
			t.Fatal("Context cancelled")
		}
	}

	// Wait for process completion
	_ = process.Wait()

	// Verify we received some events
	require.NotEmpty(t, receivedEvents, "Should have received at least one event")

	// Log final status
	t.Logf("Process completed with status: %s", process.Status())
	t.Logf("Total events received: %d", len(receivedEvents))
}

// TestIntegration_ProcessCompletion tests that the process completes gracefully.
func TestIntegration_ProcessCompletion(t *testing.T) {
	skipIfOpenCodeNotAvailable(t)
	skipIfOpenCodeNotConfigured(t)

	workDir := t.TempDir()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	cfg := Config{
		WorkDir: workDir,
		Prompt:  "Say 'done'",
		Model:   "anthropic/claude-opus-4-5",
		Timeout: 45 * time.Second,
	}

	process, err := Spawn(ctx, cfg)
	require.NoError(t, err)

	// Drain events to allow process to complete
	go func() {
		for range process.Events() {
		}
	}()

	// Wait for process to complete
	err = process.Wait()
	require.NoError(t, err)

	// Process should be in a terminal state
	status := process.Status()
	require.True(t, status.IsTerminal(), "Process should be in terminal state, got: %s", status)
}

// TestIntegration_Cancel_TerminatesProcess tests that Cancel() properly terminates
// the running process.
func TestIntegration_Cancel_TerminatesProcess(t *testing.T) {
	skipIfOpenCodeNotAvailable(t)
	skipIfOpenCodeNotConfigured(t)

	workDir := t.TempDir()

	ctx := context.Background()

	cfg := Config{
		WorkDir: workDir,
		Prompt:  "Count from 1 to 1000 very slowly, one number per line",
		Model:   "anthropic/claude-opus-4-5",
		// No timeout - we'll cancel manually
	}

	process, err := Spawn(ctx, cfg)
	require.NoError(t, err)
	require.True(t, process.IsRunning())

	// Wait briefly to ensure process is active
	time.Sleep(500 * time.Millisecond)

	// Cancel the process
	err = process.Cancel()
	require.NoError(t, err)

	// Wait for cleanup
	_ = process.Wait()

	// Should be cancelled
	require.Equal(t, client.StatusCancelled, process.Status())
	require.False(t, process.IsRunning())
}

// TestIntegration_MCPConfig_Written tests that MCP configuration is properly
// written to opencode.jsonc when provided.
func TestIntegration_MCPConfig_Written(t *testing.T) {
	skipIfOpenCodeNotAvailable(t)

	workDir := t.TempDir()

	mcpConfig := `{
		"mcp": {
			"perles-worker": {
				"type": "remote",
				"url": "http://localhost:9999/test"
			}
		}
	}`

	cfg := Config{
		WorkDir:   workDir,
		MCPConfig: mcpConfig,
	}

	// Just test the MCP config setup without spawning
	err := setupMCPConfig(cfg)
	require.NoError(t, err)

	// Verify the config file was created
	configPath := filepath.Join(workDir, "opencode.jsonc")
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)

	require.Contains(t, string(data), "perles-worker")
	require.Contains(t, string(data), "http://localhost:9999/test")
}

// TestIntegration_SessionResume tests resuming an existing session.
// This test is more complex as it requires a valid session ID from a previous run.
func TestIntegration_SessionResume(t *testing.T) {
	skipIfOpenCodeNotAvailable(t)
	skipIfOpenCodeNotConfigured(t)

	workDir := t.TempDir()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// First, spawn a session and get its ID
	cfg := Config{
		WorkDir: workDir,
		Prompt:  "Say 'first message'",
		Model:   "anthropic/claude-opus-4-5",
		Timeout: 30 * time.Second,
	}

	process, err := Spawn(ctx, cfg)
	require.NoError(t, err)

	// Wait for init event to get session ID
	eventCh := process.Events()
	var sessionID string
	timeout := time.After(25 * time.Second)

waitForInit:
	for {
		select {
		case event, ok := <-eventCh:
			if !ok {
				break waitForInit
			}
			if event.Type == client.EventSystem && event.SubType == "init" {
				sessionID = process.SessionRef()
				if sessionID == "" && event.SessionID != "" {
					sessionID = event.SessionID
				}
				break waitForInit
			}
		case <-timeout:
			break waitForInit
		}
	}

	// Wait for first process to complete
	_ = process.Cancel()
	_ = process.Wait()

	// If we didn't get a session ID, skip the resume test
	if sessionID == "" {
		t.Skip("Could not extract session ID for resume test")
	}

	t.Logf("First session completed with ID: %s", sessionID)

	// Now try to resume the session
	ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel2()

	resumeCfg := Config{
		WorkDir:   workDir,
		Prompt:    "Say 'resumed'",
		Model:     "anthropic/claude-opus-4-5",
		SessionID: sessionID,
		Timeout:   30 * time.Second,
	}

	resumeProcess, err := Resume(ctx2, sessionID, resumeCfg)
	require.NoError(t, err)
	defer func() {
		_ = resumeProcess.Cancel()
		_ = resumeProcess.Wait()
	}()

	require.True(t, resumeProcess.IsRunning())
	t.Logf("Resume process started with PID: %d", resumeProcess.PID())

	// Drain events
	resumeEventCh := resumeProcess.Events()
	timeout2 := time.After(25 * time.Second)

drainLoop:
	for {
		select {
		case event, ok := <-resumeEventCh:
			if !ok {
				break drainLoop
			}
			t.Logf("Resume event: type=%s subtype=%s", event.Type, event.SubType)
			if event.Type == client.EventResult {
				break drainLoop
			}
		case <-timeout2:
			break drainLoop
		}
	}

	_ = resumeProcess.Wait()
	t.Logf("Resume process completed with status: %s", resumeProcess.Status())
}

// TestIntegration_WorkDir_Respected tests that the working directory is properly
// set for the spawned process.
func TestIntegration_WorkDir_Respected(t *testing.T) {
	skipIfOpenCodeNotAvailable(t)

	workDir := t.TempDir()

	// Create a test file in the work directory
	testFile := filepath.Join(workDir, "test-marker.txt")
	err := os.WriteFile(testFile, []byte("integration test marker"), 0644)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := Config{
		WorkDir: workDir,
		Prompt:  "test",
		Model:   "anthropic/claude-opus-4-5",
	}

	// Just verify config setup works with the work dir
	process, err := Spawn(ctx, cfg)
	if err != nil {
		// If opencode not configured, that's fine for this test
		t.Skipf("Could not spawn process: %v", err)
	}

	// Verify work directory is set correctly
	require.Equal(t, workDir, process.WorkDir())

	_ = process.Cancel()
	_ = process.Wait()
}
