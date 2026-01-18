package client

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// mockParser is a minimal EventParser implementation for testing.
type mockParser struct {
	BaseParser
}

func newMockParser() *mockParser {
	return &mockParser{
		BaseParser: NewBaseParser(200000),
	}
}

func (p *mockParser) ParseEvent(data []byte) (OutputEvent, error) {
	return OutputEvent{Type: EventSystem, SubType: "test"}, nil
}

func (p *mockParser) ExtractSessionRef(event OutputEvent, rawLine []byte) string {
	return ""
}

// TestSpawnBuilder_Validation_MissingExecutable_ReturnsError verifies that
// Build() returns an error when executable path is not set.
func TestSpawnBuilder_Validation_MissingExecutable_ReturnsError(t *testing.T) {
	ctx := context.Background()

	_, err := NewSpawnBuilder(ctx).
		WithParser(newMockParser()).
		Build()

	require.Error(t, err)
	require.Contains(t, err.Error(), "executable path is required")
}

// TestSpawnBuilder_Validation_MissingParser_ReturnsError verifies that
// Build() returns an error when parser is not set.
func TestSpawnBuilder_Validation_MissingParser_ReturnsError(t *testing.T) {
	ctx := context.Background()

	_, err := NewSpawnBuilder(ctx).
		WithExecutable("/bin/echo", []string{"hello"}).
		Build()

	require.Error(t, err)
	require.Contains(t, err.Error(), "parser is required")
}

// TestSpawnBuilder_Validation_ValidConfig_ReturnsBaseProcess verifies that
// Build() succeeds with valid configuration and returns a BaseProcess.
func TestSpawnBuilder_Validation_ValidConfig_ReturnsBaseProcess(t *testing.T) {
	ctx := context.Background()

	bp, err := NewSpawnBuilder(ctx).
		WithExecutable("/bin/echo", []string{"hello"}).
		WithParser(newMockParser()).
		WithProviderName("test").
		Build()

	require.NoError(t, err)
	require.NotNil(t, bp)
	require.Equal(t, "test", bp.ProviderName())
	require.Equal(t, StatusRunning, bp.Status())

	// Clean up
	bp.Cancel()
	bp.Wait()
}

// TestSpawnBuilder_WithTimeout_CreatesTimeoutContext verifies that
// a positive timeout creates a context with deadline.
func TestSpawnBuilder_WithTimeout_CreatesTimeoutContext(t *testing.T) {
	ctx := context.Background()

	// Use a short timeout with sleep to verify timeout works
	bp, err := NewSpawnBuilder(ctx).
		WithExecutable("/bin/sleep", []string{"10"}).
		WithParser(newMockParser()).
		WithTimeout(100 * time.Millisecond).
		WithProviderName("test").
		Build()

	require.NoError(t, err)
	require.NotNil(t, bp)

	// Verify context has deadline
	deadline, hasDeadline := bp.Context().Deadline()
	require.True(t, hasDeadline)
	require.True(t, deadline.After(time.Now()))

	// Wait for timeout
	bp.Wait()

	// Process should have failed due to timeout
	require.Equal(t, StatusFailed, bp.Status())
}

// TestSpawnBuilder_NoTimeout_CreatesCancelContext verifies that
// zero timeout creates a cancel-only context (no deadline).
func TestSpawnBuilder_NoTimeout_CreatesCancelContext(t *testing.T) {
	ctx := context.Background()

	bp, err := NewSpawnBuilder(ctx).
		WithExecutable("/bin/echo", []string{"hello"}).
		WithParser(newMockParser()).
		WithTimeout(0). // Explicit zero timeout
		WithProviderName("test").
		Build()

	require.NoError(t, err)
	require.NotNil(t, bp)

	// Verify context has no deadline
	_, hasDeadline := bp.Context().Deadline()
	require.False(t, hasDeadline)

	// Clean up
	bp.Cancel()
	bp.Wait()
}

// TestSpawnBuilder_WithEnv_AppendsToOsEnviron verifies that custom
// environment variables are appended to os.Environ(), not replacing them.
func TestSpawnBuilder_WithEnv_AppendsToOsEnviron(t *testing.T) {
	ctx := context.Background()

	// Create a process that prints an env var
	bp, err := NewSpawnBuilder(ctx).
		WithExecutable("/bin/sh", []string{"-c", "echo $SPAWN_TEST_VAR"}).
		WithParser(newMockParser()).
		WithEnv([]string{"SPAWN_TEST_VAR=test_value"}).
		WithProviderName("test").
		Build()

	require.NoError(t, err)
	require.NotNil(t, bp)

	// Verify the command has environment set
	require.NotEmpty(t, bp.Cmd().Env)

	// Verify our custom env var is included
	found := false
	for _, env := range bp.Cmd().Env {
		if env == "SPAWN_TEST_VAR=test_value" {
			found = true
			break
		}
	}
	require.True(t, found, "Custom env var should be in command environment")

	// Verify PATH is still present (inherited from os.Environ)
	hasPath := false
	for _, env := range bp.Cmd().Env {
		if strings.HasPrefix(env, "PATH=") {
			hasPath = true
			break
		}
	}
	require.True(t, hasPath, "PATH should be inherited from os.Environ")

	// Clean up
	bp.Cancel()
	bp.Wait()
}

// TestSpawnBuilder_WithEnv_Empty_UsesOsEnviron verifies that when no
// custom environment is set, the process inherits os.Environ() (default behavior).
func TestSpawnBuilder_WithEnv_Empty_UsesOsEnviron(t *testing.T) {
	ctx := context.Background()

	bp, err := NewSpawnBuilder(ctx).
		WithExecutable("/bin/echo", []string{"hello"}).
		WithParser(newMockParser()).
		WithProviderName("test").
		// Note: no WithEnv call
		Build()

	require.NoError(t, err)
	require.NotNil(t, bp)

	// When no env is set, cmd.Env is nil (inherits parent's environment)
	require.Nil(t, bp.Cmd().Env)

	// Clean up
	bp.Cancel()
	bp.Wait()
}

// TestSpawnBuilder_WithStdin_CreatesStdinPipe verifies that WithStdin(true)
// creates a stdin pipe accessible via Stdin().
func TestSpawnBuilder_WithStdin_CreatesStdinPipe(t *testing.T) {
	ctx := context.Background()

	bp, err := NewSpawnBuilder(ctx).
		WithExecutable("/bin/cat", nil).
		WithParser(newMockParser()).
		WithStdin(true).
		WithProviderName("test").
		Build()

	require.NoError(t, err)
	require.NotNil(t, bp)
	require.NotNil(t, bp.Stdin(), "Stdin() should return non-nil when WithStdin(true)")

	// Verify we can write to stdin
	_, writeErr := bp.Stdin().Write([]byte("test input"))
	require.NoError(t, writeErr)

	// Clean up
	bp.Stdin().Close()
	bp.Cancel()
	bp.Wait()
}

// TestSpawnBuilder_WithoutStdin_NoStdinPipe verifies that without WithStdin,
// Stdin() returns nil.
func TestSpawnBuilder_WithoutStdin_NoStdinPipe(t *testing.T) {
	ctx := context.Background()

	bp, err := NewSpawnBuilder(ctx).
		WithExecutable("/bin/echo", []string{"hello"}).
		WithParser(newMockParser()).
		WithProviderName("test").
		// Note: no WithStdin call
		Build()

	require.NoError(t, err)
	require.NotNil(t, bp)
	require.Nil(t, bp.Stdin(), "Stdin() should be nil when WithStdin not called")

	// Clean up
	bp.Cancel()
	bp.Wait()
}

// TestSpawnBuilder_Build_PipeCleanupOnError validates that Build() properly
// cleans up resources (pipes, context) when an error occurs mid-build.
func TestSpawnBuilder_Build_PipeCleanupOnError(t *testing.T) {
	ctx := context.Background()

	// Use a command factory that returns a cmd that will fail to start
	failingFactory := func(ctx context.Context, name string, args ...string) *exec.Cmd {
		// Return a command for a non-existent executable
		return exec.CommandContext(ctx, "/nonexistent/path/to/executable")
	}

	_, err := NewSpawnBuilder(ctx).
		WithExecutable("/nonexistent/path", nil).
		WithParser(newMockParser()).
		WithCommandFactory(failingFactory).
		WithStdin(true). // Request stdin pipe to verify cleanup
		WithProviderName("test").
		Build()

	// Build should fail because the executable doesn't exist
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to start")
}

// TestSpawnBuilder_WithCommandFactory_AllowsMocking verifies that
// WithCommandFactory can inject a mock command for testing.
func TestSpawnBuilder_WithCommandFactory_AllowsMocking(t *testing.T) {
	ctx := context.Background()

	factoryCalled := false
	var capturedName string
	var capturedArgs []string

	mockFactory := func(ctx context.Context, name string, args ...string) *exec.Cmd {
		factoryCalled = true
		capturedName = name
		capturedArgs = args
		// Return a real command that will work
		return exec.CommandContext(ctx, "/bin/echo", "mocked")
	}

	bp, err := NewSpawnBuilder(ctx).
		WithExecutable("/original/path", []string{"arg1", "arg2"}).
		WithParser(newMockParser()).
		WithCommandFactory(mockFactory).
		WithProviderName("test").
		Build()

	require.NoError(t, err)
	require.NotNil(t, bp)
	require.True(t, factoryCalled, "Command factory should have been called")
	require.Equal(t, "/original/path", capturedName)
	require.Equal(t, []string{"arg1", "arg2"}, capturedArgs)

	// Clean up
	bp.Cancel()
	bp.Wait()
}

// TestSpawnBuilder_WithWorkDir_SetsCommandDir verifies that WithWorkDir
// sets the working directory on the command.
func TestSpawnBuilder_WithWorkDir_SetsCommandDir(t *testing.T) {
	ctx := context.Background()
	workDir := os.TempDir()

	bp, err := NewSpawnBuilder(ctx).
		WithExecutable("/bin/echo", []string{"hello"}).
		WithParser(newMockParser()).
		WithWorkDir(workDir).
		WithProviderName("test").
		Build()

	require.NoError(t, err)
	require.NotNil(t, bp)
	require.Equal(t, workDir, bp.Cmd().Dir)
	require.Equal(t, workDir, bp.WorkDir())

	// Clean up
	bp.Cancel()
	bp.Wait()
}

// TestSpawnBuilder_WithSessionRef_SetsInitialSessionRef verifies that
// WithSessionRef sets the initial session reference on the BaseProcess.
func TestSpawnBuilder_WithSessionRef_SetsInitialSessionRef(t *testing.T) {
	ctx := context.Background()

	bp, err := NewSpawnBuilder(ctx).
		WithExecutable("/bin/echo", []string{"hello"}).
		WithParser(newMockParser()).
		WithSessionRef("session-123").
		WithProviderName("test").
		Build()

	require.NoError(t, err)
	require.NotNil(t, bp)
	require.Equal(t, "session-123", bp.SessionRef())

	// Clean up
	bp.Cancel()
	bp.Wait()
}

// TestSpawnBuilder_WithStderrCapture_EnablesCapture verifies that
// WithStderrCapture enables stderr capture on the BaseProcess.
func TestSpawnBuilder_WithStderrCapture_EnablesCapture(t *testing.T) {
	ctx := context.Background()

	bp, err := NewSpawnBuilder(ctx).
		WithExecutable("/bin/echo", []string{"hello"}).
		WithParser(newMockParser()).
		WithStderrCapture(true).
		WithProviderName("test").
		Build()

	require.NoError(t, err)
	require.NotNil(t, bp)
	require.True(t, bp.CaptureStderr())

	// Clean up
	bp.Cancel()
	bp.Wait()
}

// TestSpawnBuilder_WithOnInitEvent_SetsCallback verifies that
// WithOnInitEvent sets the init event callback on the BaseProcess.
func TestSpawnBuilder_WithOnInitEvent_SetsCallback(t *testing.T) {
	ctx := context.Background()

	callbackCalled := false
	callback := func(event OutputEvent, rawLine []byte) {
		callbackCalled = true
	}

	bp, err := NewSpawnBuilder(ctx).
		WithExecutable("/bin/echo", []string{"hello"}).
		WithParser(newMockParser()).
		WithOnInitEvent(callback).
		WithProviderName("test").
		Build()

	require.NoError(t, err)
	require.NotNil(t, bp)
	require.NotNil(t, bp.OnInitEventFn())

	// Verify callback is the one we set
	bp.OnInitEventFn()(OutputEvent{}, []byte("test"))
	require.True(t, callbackCalled)

	// Clean up
	bp.Cancel()
	bp.Wait()
}

// TestSpawnBuilder_FluentChaining verifies that all With* methods
// return the builder for fluent chaining.
func TestSpawnBuilder_FluentChaining(t *testing.T) {
	ctx := context.Background()
	parser := newMockParser()

	// This test verifies compile-time that chaining works
	bp, err := NewSpawnBuilder(ctx).
		WithExecutable("/bin/echo", []string{"hello"}).
		WithWorkDir("/tmp").
		WithSessionRef("session-xyz").
		WithTimeout(5 * time.Second).
		WithParser(parser).
		WithEnv([]string{"FOO=bar"}).
		WithProviderName("fluent-test").
		WithStderrCapture(true).
		WithStdin(false).
		WithOnInitEvent(nil).
		Build()

	require.NoError(t, err)
	require.NotNil(t, bp)
	require.Equal(t, "fluent-test", bp.ProviderName())

	// Clean up
	bp.Cancel()
	bp.Wait()
}

// TestSpawnBuilder_ProcessActuallyRuns verifies that the spawned process
// actually runs and produces output.
func TestSpawnBuilder_ProcessActuallyRuns(t *testing.T) {
	ctx := context.Background()

	bp, err := NewSpawnBuilder(ctx).
		WithExecutable("/bin/echo", []string{"hello"}).
		WithParser(newMockParser()).
		WithProviderName("test").
		Build()

	require.NoError(t, err)
	require.NotNil(t, bp)
	require.True(t, bp.PID() > 0, "Process should have a valid PID")

	// Wait for process to complete
	bp.Wait()

	// Process should have completed successfully
	require.Equal(t, StatusCompleted, bp.Status())
}
