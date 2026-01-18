package gemini

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
// Authentication Validation Tests
// =============================================================================

func TestValidateAuth_OAuthToken(t *testing.T) {
	// Create temp home directory with OAuth token
	tempDir := t.TempDir()
	geminiDir := filepath.Join(tempDir, ".gemini")
	require.NoError(t, os.MkdirAll(geminiDir, 0755))

	tokenPath := filepath.Join(geminiDir, "mcp-oauth-tokens-v2.json")
	require.NoError(t, os.WriteFile(tokenPath, []byte(`{"tokens": "test"}`), 0600))

	// Override HOME/USERPROFILE for this test (t.Setenv handles both platforms)
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir)

	// Clear API key env vars to ensure OAuth is the only valid auth
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")

	err := validateAuth()
	require.NoError(t, err)
}

func TestValidateAuth_GeminiAPIKey(t *testing.T) {
	// Set HOME to non-existent path to disable OAuth check
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", "/non-existent-path")
	defer os.Setenv("HOME", originalHome)

	// Set GEMINI_API_KEY
	originalKey := os.Getenv("GEMINI_API_KEY")
	os.Setenv("GEMINI_API_KEY", "test-api-key")
	defer os.Setenv("GEMINI_API_KEY", originalKey)

	// Ensure GOOGLE_API_KEY is not set
	os.Unsetenv("GOOGLE_API_KEY")

	err := validateAuth()
	require.NoError(t, err)
}

func TestValidateAuth_GoogleAPIKey(t *testing.T) {
	// Set HOME to non-existent path to disable OAuth check
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", "/non-existent-path")
	defer os.Setenv("HOME", originalHome)

	// Clear GEMINI_API_KEY
	os.Unsetenv("GEMINI_API_KEY")

	// Set GOOGLE_API_KEY
	originalKey := os.Getenv("GOOGLE_API_KEY")
	os.Setenv("GOOGLE_API_KEY", "test-google-key")
	defer os.Setenv("GOOGLE_API_KEY", originalKey)

	err := validateAuth()
	require.NoError(t, err)
}

func TestValidateAuth_NoAuth(t *testing.T) {
	// Set HOME to non-existent path to disable OAuth check
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", "/non-existent-path")
	defer os.Setenv("HOME", originalHome)

	// Clear all API keys
	geminiKey := os.Getenv("GEMINI_API_KEY")
	googleKey := os.Getenv("GOOGLE_API_KEY")
	os.Unsetenv("GEMINI_API_KEY")
	os.Unsetenv("GOOGLE_API_KEY")
	defer func() {
		if geminiKey != "" {
			os.Setenv("GEMINI_API_KEY", geminiKey)
		}
		if googleKey != "" {
			os.Setenv("GOOGLE_API_KEY", googleKey)
		}
	}()

	err := validateAuth()
	require.Error(t, err)
	require.Equal(t, ErrNoAuth, err)
	require.Contains(t, err.Error(), "no authentication found")
}

// =============================================================================
// Executable Discovery Tests
// =============================================================================

func TestFindExecutable_NpmPath(t *testing.T) {
	// Create temp home directory with npm gemini
	tempDir := t.TempDir()
	npmBinDir := filepath.Join(tempDir, ".npm", "bin")
	require.NoError(t, os.MkdirAll(npmBinDir, 0755))

	// On Windows, executables need .exe extension; on Unix, no extension
	execName := "gemini"
	if os.PathSeparator == '\\' {
		execName = "gemini.exe"
	}
	geminiPath := filepath.Join(npmBinDir, execName)
	require.NoError(t, os.WriteFile(geminiPath, []byte("#!/bin/bash\necho test"), 0755))

	// Override HOME/USERPROFILE for this test (t.Setenv handles both platforms)
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir)

	path, err := findExecutable()
	require.NoError(t, err)
	require.Equal(t, geminiPath, path)
}

func TestFindExecutable_LocalBin(t *testing.T) {
	// Skip if we don't have permission to check /usr/local/bin
	// or if there's already a gemini there
	_, err := os.Stat("/usr/local/bin/gemini")
	if err == nil {
		// gemini exists in /usr/local/bin, test passes
		path, err := findExecutable()
		// Need to disable npm path check
		originalHome := os.Getenv("HOME")
		os.Setenv("HOME", "/non-existent-path")
		defer os.Setenv("HOME", originalHome)

		path, err = findExecutable()
		require.NoError(t, err)
		require.Equal(t, "/usr/local/bin/gemini", path)
	} else {
		t.Skip("gemini not found in /usr/local/bin")
	}
}

func TestFindExecutable_PathFallback(t *testing.T) {
	// Set HOME to non-existent path to disable npm check
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", "/non-existent-path")
	defer os.Setenv("HOME", originalHome)

	// This test depends on whether gemini is in PATH
	// If it is, it should be found; if not, test will verify error handling
	path, err := findExecutable()
	if err != nil {
		// gemini not in PATH, verify it's the expected error
		require.Equal(t, ErrNotFound, err)
	} else {
		// gemini found in PATH
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
// Lifecycle Tests - Process struct behavior without actual subprocess spawning
// =============================================================================

// newTestProcess creates a Process struct for testing without spawning a real subprocess.
// This allows testing lifecycle methods, status transitions, and channel behavior.
func newTestProcess() *Process {
	ctx, cancel := context.WithCancel(context.Background())
	bp := client.NewBaseProcess(
		ctx,
		cancel,
		nil, // cmd
		nil, // stdout
		nil, // stderr
		"/test/project",
		client.WithProviderName("gemini"),
	)
	bp.SetSessionRef("gemini-test-session-12345")
	bp.SetStatus(client.StatusRunning)
	return &Process{BaseProcess: bp}
}

func TestProcess_ChannelBufferSizes(t *testing.T) {
	p := newTestProcess()

	// Events channel should have capacity 100
	require.Equal(t, 100, cap(p.EventsWritable()))

	// Errors channel should have capacity 10
	require.Equal(t, 10, cap(p.ErrorsWritable()))
}

func TestProcess_StatusTransitions_PendingToRunning(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bp := client.NewBaseProcess(
		ctx,
		cancel,
		nil,
		nil,
		nil,
		"/test/project",
		client.WithProviderName("gemini"),
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
			nil,
			nil,
			nil,
			"/test/project",
			client.WithProviderName("gemini"),
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
				nil,
				nil,
				nil,
				"/test/project",
				client.WithProviderName("gemini"),
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
		nil,
		nil,
		nil,
		"/test/project",
		client.WithProviderName("gemini"),
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
	require.Equal(t, "gemini-test-session-12345", p.SessionRef())
}

func TestProcess_SessionRef_InitiallyEmpty(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bp := client.NewBaseProcess(
		ctx,
		cancel,
		nil,
		nil,
		nil,
		"/test/project",
		client.WithProviderName("gemini"),
	)
	bp.SetStatus(client.StatusRunning)
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
	// cmd is nil, so PID should return -1
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
	// Create a process with a full error channel
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bp := client.NewBaseProcess(
		ctx,
		cancel,
		nil,
		nil,
		nil,
		"/test/project",
		client.WithProviderName("gemini"),
	)
	p := &Process{BaseProcess: bp}

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
	case err := <-p.ErrorsWritable():
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

func TestErrNoAuth(t *testing.T) {
	require.NotNil(t, ErrNoAuth)
	require.Contains(t, ErrNoAuth.Error(), "no authentication found")
}

func TestErrNotFound(t *testing.T) {
	require.NotNil(t, ErrNotFound)
	require.Contains(t, ErrNotFound.Error(), "executable not found")
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

	// Verify .gemini directory was NOT created
	geminiDir := filepath.Join(tempDir, ".gemini")
	_, err = os.Stat(geminiDir)
	require.True(t, os.IsNotExist(err), ".gemini directory should not be created for empty MCPConfig")
}

func TestSetupMCPConfig_NewFile_Created(t *testing.T) {
	tempDir := t.TempDir()

	mcpConfig := `{
		"mcpServers": {
			"perles-worker": {
				"httpUrl": "http://localhost:8080/worker/1",
				"timeout": 30000
			}
		}
	}`

	cfg := Config{
		WorkDir:   tempDir,
		MCPConfig: mcpConfig,
	}

	err := setupMCPConfig(cfg)
	require.NoError(t, err)

	// Verify settings.json was created
	settingsPath := filepath.Join(tempDir, ".gemini", "settings.json")
	data, err := os.ReadFile(settingsPath)
	require.NoError(t, err)

	var settings map[string]any
	err = json.Unmarshal(data, &settings)
	require.NoError(t, err)

	// Verify mcpServers was written
	mcpServers, ok := settings["mcpServers"].(map[string]any)
	require.True(t, ok, "mcpServers should be a map")

	worker, ok := mcpServers["perles-worker"].(map[string]any)
	require.True(t, ok, "perles-worker should be a map")

	require.Equal(t, "http://localhost:8080/worker/1", worker["httpUrl"])
	require.Equal(t, float64(30000), worker["timeout"]) // JSON numbers are float64
}

func TestSetupMCPConfig_ExistingSettings_AddsMCPServers(t *testing.T) {
	tempDir := t.TempDir()

	// Create existing settings.json without mcpServers
	geminiDir := filepath.Join(tempDir, ".gemini")
	require.NoError(t, os.MkdirAll(geminiDir, 0755))

	existingSettings := `{
		"theme": "dark",
		"autoSave": true
	}`
	settingsPath := filepath.Join(geminiDir, "settings.json")
	require.NoError(t, os.WriteFile(settingsPath, []byte(existingSettings), 0644))

	mcpConfig := `{
		"mcpServers": {
			"perles-worker": {
				"httpUrl": "http://localhost:8080/worker/1",
				"timeout": 30000
			}
		}
	}`

	cfg := Config{
		WorkDir:   tempDir,
		MCPConfig: mcpConfig,
	}

	err := setupMCPConfig(cfg)
	require.NoError(t, err)

	// Verify settings.json was updated
	data, err := os.ReadFile(settingsPath)
	require.NoError(t, err)

	var settings map[string]any
	err = json.Unmarshal(data, &settings)
	require.NoError(t, err)

	// Verify existing settings were preserved
	require.Equal(t, "dark", settings["theme"])
	require.Equal(t, true, settings["autoSave"])

	// Verify mcpServers was added
	mcpServers, ok := settings["mcpServers"].(map[string]any)
	require.True(t, ok, "mcpServers should be a map")

	worker, ok := mcpServers["perles-worker"].(map[string]any)
	require.True(t, ok, "perles-worker should be a map")

	require.Equal(t, "http://localhost:8080/worker/1", worker["httpUrl"])
}

func TestSetupMCPConfig_ExistingMCPServers_Merged(t *testing.T) {
	tempDir := t.TempDir()

	// Create existing settings.json with mcpServers
	geminiDir := filepath.Join(tempDir, ".gemini")
	require.NoError(t, os.MkdirAll(geminiDir, 0755))

	existingSettings := `{
		"theme": "dark",
		"mcpServers": {
			"existing-server": {
				"httpUrl": "http://localhost:9000/existing",
				"timeout": 5000
			}
		}
	}`
	settingsPath := filepath.Join(geminiDir, "settings.json")
	require.NoError(t, os.WriteFile(settingsPath, []byte(existingSettings), 0644))

	// New config adds perles-worker
	mcpConfig := `{
		"mcpServers": {
			"perles-worker": {
				"httpUrl": "http://localhost:8080/worker/1",
				"timeout": 30000
			}
		}
	}`

	cfg := Config{
		WorkDir:   tempDir,
		MCPConfig: mcpConfig,
	}

	err := setupMCPConfig(cfg)
	require.NoError(t, err)

	// Verify settings.json was updated
	data, err := os.ReadFile(settingsPath)
	require.NoError(t, err)

	var settings map[string]any
	err = json.Unmarshal(data, &settings)
	require.NoError(t, err)

	// Verify existing settings preserved
	require.Equal(t, "dark", settings["theme"])

	// Verify both servers exist
	mcpServers, ok := settings["mcpServers"].(map[string]any)
	require.True(t, ok, "mcpServers should be a map")
	require.Len(t, mcpServers, 2, "should have both servers")

	// Verify existing server preserved
	existingServer, ok := mcpServers["existing-server"].(map[string]any)
	require.True(t, ok, "existing-server should be preserved")
	require.Equal(t, "http://localhost:9000/existing", existingServer["httpUrl"])

	// Verify new server added
	newServer, ok := mcpServers["perles-worker"].(map[string]any)
	require.True(t, ok, "perles-worker should be added")
	require.Equal(t, "http://localhost:8080/worker/1", newServer["httpUrl"])
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

	// Create existing settings.json with invalid JSON
	geminiDir := filepath.Join(tempDir, ".gemini")
	require.NoError(t, os.MkdirAll(geminiDir, 0755))

	settingsPath := filepath.Join(geminiDir, "settings.json")
	require.NoError(t, os.WriteFile(settingsPath, []byte("{ invalid json }"), 0644))

	mcpConfig := `{
		"mcpServers": {
			"perles-worker": {
				"httpUrl": "http://localhost:8080/worker/1"
			}
		}
	}`

	cfg := Config{
		WorkDir:   tempDir,
		MCPConfig: mcpConfig,
	}

	err := setupMCPConfig(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to parse existing settings.json")

	// Verify original file was NOT modified (no corruption)
	data, readErr := os.ReadFile(settingsPath)
	require.NoError(t, readErr)
	require.Equal(t, "{ invalid json }", string(data), "original file should not be corrupted")
}

func TestSetupMCPConfig_DirectoryCreation(t *testing.T) {
	tempDir := t.TempDir()

	// Ensure .gemini directory does not exist
	geminiDir := filepath.Join(tempDir, ".gemini")
	_, err := os.Stat(geminiDir)
	require.True(t, os.IsNotExist(err))

	mcpConfig := `{
		"mcpServers": {
			"perles-worker": {
				"httpUrl": "http://localhost:8080/worker/1"
			}
		}
	}`

	cfg := Config{
		WorkDir:   tempDir,
		MCPConfig: mcpConfig,
	}

	err = setupMCPConfig(cfg)
	require.NoError(t, err)

	// Verify .gemini directory was created
	info, err := os.Stat(geminiDir)
	require.NoError(t, err)
	require.True(t, info.IsDir())
}

func TestSetupMCPConfig_ProperFormatting(t *testing.T) {
	tempDir := t.TempDir()

	mcpConfig := `{"mcpServers":{"perles-worker":{"httpUrl":"http://localhost:8080/worker/1"}}}`

	cfg := Config{
		WorkDir:   tempDir,
		MCPConfig: mcpConfig,
	}

	err := setupMCPConfig(cfg)
	require.NoError(t, err)

	// Verify settings.json is properly formatted (indented)
	settingsPath := filepath.Join(tempDir, ".gemini", "settings.json")
	data, err := os.ReadFile(settingsPath)
	require.NoError(t, err)

	// Should contain indentation (2 spaces)
	require.Contains(t, string(data), "  ")
	// Should have newlines for formatting
	require.Contains(t, string(data), "\n")
}

func TestSetupMCPConfig_OverwritesSameServer(t *testing.T) {
	tempDir := t.TempDir()

	// Create existing settings.json with perles-worker
	geminiDir := filepath.Join(tempDir, ".gemini")
	require.NoError(t, os.MkdirAll(geminiDir, 0755))

	existingSettings := `{
		"mcpServers": {
			"perles-worker": {
				"httpUrl": "http://localhost:OLD/worker/OLD",
				"timeout": 1000
			}
		}
	}`
	settingsPath := filepath.Join(geminiDir, "settings.json")
	require.NoError(t, os.WriteFile(settingsPath, []byte(existingSettings), 0644))

	// New config updates perles-worker
	mcpConfig := `{
		"mcpServers": {
			"perles-worker": {
				"httpUrl": "http://localhost:8080/worker/NEW",
				"timeout": 30000
			}
		}
	}`

	cfg := Config{
		WorkDir:   tempDir,
		MCPConfig: mcpConfig,
	}

	err := setupMCPConfig(cfg)
	require.NoError(t, err)

	// Verify settings.json was updated
	data, err := os.ReadFile(settingsPath)
	require.NoError(t, err)

	var settings map[string]any
	err = json.Unmarshal(data, &settings)
	require.NoError(t, err)

	mcpServers, ok := settings["mcpServers"].(map[string]any)
	require.True(t, ok)
	require.Len(t, mcpServers, 1, "should only have one server")

	worker, ok := mcpServers["perles-worker"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "http://localhost:8080/worker/NEW", worker["httpUrl"], "should be updated URL")
	require.Equal(t, float64(30000), worker["timeout"], "should be updated timeout")
}

// =============================================================================
// Session Extraction Tests
// =============================================================================

func TestExtractSession_InitEvent(t *testing.T) {
	event := client.OutputEvent{
		Type:      client.EventSystem,
		SubType:   "init",
		SessionID: "gemini-session-123",
	}

	sessionID := extractSession(event, nil)
	require.Equal(t, "gemini-session-123", sessionID)
}

func TestExtractSession_NonInitEvent(t *testing.T) {
	event := client.OutputEvent{
		Type:      client.EventAssistant,
		SessionID: "gemini-session-123",
	}

	sessionID := extractSession(event, nil)
	require.Equal(t, "", sessionID) // Only init events should return session ID
}

func TestExtractSession_EmptySessionID(t *testing.T) {
	event := client.OutputEvent{
		Type:      client.EventSystem,
		SubType:   "init",
		SessionID: "",
	}

	sessionID := extractSession(event, nil)
	require.Equal(t, "", sessionID)
}
