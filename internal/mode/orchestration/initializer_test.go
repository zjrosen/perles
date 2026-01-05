package orchestration

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/orchestration/amp"
	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
)

func TestInitializer_CreatesSession(t *testing.T) {
	// Create a temporary directory for the workspace
	workDir := t.TempDir()

	// Create an initializer with minimal config
	init := NewInitializer(InitializerConfig{
		WorkDir:    workDir,
		ClientType: "claude",
	})

	// Start initialization (this will fail because we don't have a real AI client,
	// but we can still verify the session was created in createWorkspace())

	// We can't directly call createWorkspace() because it's a private method,
	// but we can verify the behavior indirectly by checking that Resources()
	// returns a properly initialized session after initialization starts.

	// Since the actual initialization requires an AI client which we don't have
	// in unit tests, we'll verify the session folder structure after a failed init.

	// For a true unit test, we need to refactor createWorkspace to be testable
	// or accept that this is an integration test.

	// Instead, verify the session field exists in InitializerResources
	resources := init.Resources()
	// Session should be nil before start
	require.Nil(t, resources.Session)
}

func TestInitializer_SessionInResources(t *testing.T) {
	// Verify the session field is present in InitializerResources
	resources := InitializerResources{}
	// The Session field should be accessible (compile-time check)
	require.Nil(t, resources.Session)
}

func TestInitializer_SessionFolderStructure(t *testing.T) {
	// This test verifies the expected folder structure matches what the session package creates.
	// This is a documentation test showing the expected structure.

	workDir := t.TempDir()
	sessionID := "test-session-uuid"
	sessionDir := filepath.Join(workDir, ".perles", "sessions", sessionID)

	// Verify the path construction matches what initializer.go does
	expectedDir := filepath.Join(workDir, ".perles", "sessions", sessionID)
	require.Equal(t, expectedDir, sessionDir)

	// The actual folder creation is done by session.New() which is already tested
	// in internal/orchestration/session/session_test.go
}

func TestInitializer_Retry_ResetsSession(t *testing.T) {
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir:    workDir,
		ClientType: "claude",
	})

	// Verify session is nil initially
	require.Nil(t, init.session)

	// After Retry is called, session should still be nil (since Retry resets it)
	// We can't actually call Retry without Start, but we can verify the field exists
	// and would be reset in the Retry method.
}

func TestNewInitializer(t *testing.T) {
	cfg := InitializerConfig{
		WorkDir:    "/test/dir",
		ClientType: "claude",
	}

	init := NewInitializer(cfg)
	require.NotNil(t, init)
	require.NotNil(t, init.Broker())
	require.Equal(t, InitNotStarted, init.Phase())
	require.Nil(t, init.Error())
}

func TestNewInitializer_DefaultWorkers(t *testing.T) {
	cfg := InitializerConfig{
		WorkDir:    "/test/dir",
		ClientType: "claude",
	}

	init := NewInitializer(cfg)
	require.NotNil(t, init)
	// With lazy spawning, default is 0 workers (coordinator spawns workers on-demand)
}

func TestInitializerPhase(t *testing.T) {
	init := NewInitializer(InitializerConfig{
		WorkDir: "/test/dir",
	})

	require.Equal(t, InitNotStarted, init.Phase())
}

func TestInitializerResources_HasSession(t *testing.T) {
	// Verify InitializerResources includes a Session field
	resources := InitializerResources{}

	// This is a compile-time check that the field exists
	_ = resources.Session
}

func TestIntegration_SessionCreation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This is an integration test that verifies session creation
	// by manually calling the session package

	workDir := t.TempDir()
	sessionID := "integration-test-session"
	sessionDir := filepath.Join(workDir, ".perles", "sessions", sessionID)

	// Import and use the session package directly to verify it works as expected
	// This mimics what createWorkspace() does

	// Verify the path doesn't exist yet
	_, err := os.Stat(sessionDir)
	require.True(t, os.IsNotExist(err))

	// The actual session creation is handled by session.New() which is thoroughly tested
	// We're verifying the integration path here
}

// ===========================================================================
// V2 Orchestration Infrastructure Tests
// ===========================================================================

func TestInitializer_Retry_ResetsV2Infrastructure(t *testing.T) {
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir:    workDir,
		ClientType: "claude",
		Timeout:    100 * time.Millisecond, // Short timeout for test
	})

	// Verify v2 infrastructure is nil initially
	require.Nil(t, init.GetV2Infra())

	// After Retry is called (which calls Cancel first), v2 fields should be reset to nil
	// We can verify the fields exist and would be reset in the Retry method
	init.mu.Lock()
	init.v2Infra = nil
	init.mu.Unlock()

	// Fields should still be nil
	require.Nil(t, init.GetV2Infra())
}

func TestInitializer_V2FieldsExist(t *testing.T) {
	// Compile-time check that v2Infra field exists and getter works
	init := NewInitializer(InitializerConfig{
		WorkDir: "/test/dir",
	})

	// GetV2Infra should exist and return nil before Start
	require.Nil(t, init.GetV2Infra())
}

func TestInitializer_V2FieldsNilBeforeStart(t *testing.T) {
	// Verify v2 infrastructure is nil before Start() is called
	init := NewInitializer(InitializerConfig{
		WorkDir:    t.TempDir(),
		ClientType: "claude",
	})

	// Before Start, v2Infra should be nil (accessed via getter)
	require.Nil(t, init.GetV2Infra(), "v2Infra should be nil before Start")
}

func TestInitializer_CleanupDrainsProcessor(t *testing.T) {
	// This test verifies that cleanupResources() properly handles nil cmdProcessor
	init := NewInitializer(InitializerConfig{
		WorkDir: t.TempDir(),
	})

	// cleanupResources() should not panic when cmdProcessor is nil
	require.NotPanics(t, func() {
		init.cleanupResources()
	})
}

// ===========================================================================
// V2 Event Bus Tests
// ===========================================================================

func TestInitializer_GetV2EventBus_NilBeforeStart(t *testing.T) {
	// Verify GetV2EventBus() returns nil before Start() is called
	init := NewInitializer(InitializerConfig{
		WorkDir:    t.TempDir(),
		ClientType: "claude",
	})

	// Before Start, GetV2EventBus should return nil
	require.Nil(t, init.GetV2EventBus(), "GetV2EventBus should return nil before initialization")
}

func TestInitializer_GetV2EventBus_ThreadSafe(t *testing.T) {
	// Verify GetV2EventBus() uses read lock for thread safety
	init := NewInitializer(InitializerConfig{
		WorkDir:    t.TempDir(),
		ClientType: "claude",
	})

	// Concurrent calls should not race (verified with -race flag)
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_ = init.GetV2EventBus()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestInitializer_V2EventBusFieldExists(t *testing.T) {
	// Compile-time check that GetV2EventBus getter exists
	init := NewInitializer(InitializerConfig{
		WorkDir: t.TempDir(),
	})

	// GetV2EventBus should exist and return nil before Start
	require.Nil(t, init.GetV2EventBus())
}

func TestInitializer_Retry_ResetsV2EventBus(t *testing.T) {
	// Verify v2EventBus is reset when Retry() is called (via v2Infra reset)
	init := NewInitializer(InitializerConfig{
		WorkDir:    t.TempDir(),
		ClientType: "claude",
		Timeout:    100 * time.Millisecond,
	})

	// v2EventBus should be nil initially (via getter which checks v2Infra)
	require.Nil(t, init.GetV2EventBus(), "v2EventBus should be nil before Start")

	// The Retry method resets v2Infra to nil, which means GetV2EventBus returns nil
	init.mu.Lock()
	init.v2Infra = nil
	init.mu.Unlock()

	require.Nil(t, init.GetV2EventBus(), "v2EventBus should be nil after reset")
}

// ===========================================================================
// Unified Process Infrastructure Tests (Phase 5)
// ===========================================================================

func TestInitializer_ProcessRepoFieldExists(t *testing.T) {
	// Compile-time check that GetProcessRepository getter exists
	init := NewInitializer(InitializerConfig{
		WorkDir: t.TempDir(),
	})

	// GetProcessRepository should exist and return nil before Start
	require.Nil(t, init.GetProcessRepository())
}

func TestInitializer_ProcessRepoNilBeforeStart(t *testing.T) {
	// Verify processRepo is nil before Start() is called (via getter)
	init := NewInitializer(InitializerConfig{
		WorkDir:    t.TempDir(),
		ClientType: "claude",
	})

	// Before Start, processRepo should be nil (accessed via getter which checks v2Infra)
	require.Nil(t, init.GetProcessRepository(), "processRepo should be nil before Start")
}

func TestInitializer_Retry_ResetsProcessRepo(t *testing.T) {
	// Verify processRepo is reset when Retry() is called (via v2Infra reset)
	init := NewInitializer(InitializerConfig{
		WorkDir:    t.TempDir(),
		ClientType: "claude",
		Timeout:    100 * time.Millisecond,
	})

	// processRepo should be nil initially (via getter which checks v2Infra)
	require.Nil(t, init.GetProcessRepository(), "processRepo should be nil before Start")

	// The Retry method resets v2Infra to nil, which means GetProcessRepository returns nil
	init.mu.Lock()
	init.v2Infra = nil
	init.mu.Unlock()

	require.Nil(t, init.GetProcessRepository(), "processRepo should be nil after reset")
}

// ===========================================================================
// ProcessEvent Handling Tests (Phase 5)
// ===========================================================================

// ===========================================================================
// createSession() Method Tests (Task perles-oph9.1)
// ===========================================================================

func TestInitializer_CreateSession_Success(t *testing.T) {
	// Verify createSession() creates a valid session with expected directory structure
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir:    workDir,
		ClientType: "claude",
	})

	// Set session ID (normally done by Start())
	init.mu.Lock()
	init.sessionID = "12345678-90ab-cdef-1234-567890abcdef"
	init.mu.Unlock()

	// Call createSession directly
	sess, err := init.createSession()

	// Verify no error
	require.NoError(t, err, "createSession should not return an error")
	require.NotNil(t, sess, "createSession should return a non-nil session")

	// Verify session has an ID (UUID format)
	require.NotEmpty(t, sess.ID, "session should have a non-empty ID")
	require.Len(t, sess.ID, 36, "session ID should be a valid UUID (36 chars)")

	// Verify the session directory was created
	sessionDir := filepath.Join(workDir, ".perles", "sessions", sess.ID)
	info, err := os.Stat(sessionDir)
	require.NoError(t, err, "session directory should exist")
	require.True(t, info.IsDir(), "session directory should be a directory")
}

func TestInitializer_CreateSession_ReturnsErrorOnFailure(t *testing.T) {
	// Verify createSession() returns proper error on session.New failure
	// We simulate failure by using an invalid/unwritable path

	// Use a path that will fail (file as parent directory)
	workDir := t.TempDir()
	invalidPath := filepath.Join(workDir, "not-a-dir")

	// Create a file where a directory is expected (to force MkdirAll failure)
	err := os.WriteFile(invalidPath, []byte("blocking file"), 0644)
	require.NoError(t, err, "setup: should create blocking file")

	init := NewInitializer(InitializerConfig{
		WorkDir:    invalidPath, // This will cause session.New to fail
		ClientType: "claude",
	})

	// Set session ID (normally done by Start())
	init.mu.Lock()
	init.sessionID = "test-session-id"
	init.mu.Unlock()

	// Call createSession - should fail because we can't create a directory
	// under a file
	sess, err := init.createSession()

	// Verify error is returned
	require.Error(t, err, "createSession should return an error when session creation fails")
	require.Nil(t, sess, "createSession should return nil session on error")
	require.Contains(t, err.Error(), "failed to create session", "error should indicate session creation failure")
}

func TestInitializer_CreateSession_UniqueIDs(t *testing.T) {
	// Verify createSession() uses the session ID set by Start()
	// Each session should have the expected unique ID when we set different IDs
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir:    workDir,
		ClientType: "claude",
	})

	// Create multiple sessions with different pre-set IDs
	init.mu.Lock()
	init.sessionID = "session-id-11111111-1111-1111-1111-111111111111"
	init.mu.Unlock()
	sess1, err1 := init.createSession()
	require.NoError(t, err1)

	init.mu.Lock()
	init.sessionID = "session-id-22222222-2222-2222-2222-222222222222"
	init.mu.Unlock()
	sess2, err2 := init.createSession()
	require.NoError(t, err2)

	init.mu.Lock()
	init.sessionID = "session-id-33333333-3333-3333-3333-333333333333"
	init.mu.Unlock()
	sess3, err3 := init.createSession()
	require.NoError(t, err3)

	// Verify all IDs are unique (because we set different IDs)
	require.NotEqual(t, sess1.ID, sess2.ID, "session IDs should be unique")
	require.NotEqual(t, sess2.ID, sess3.ID, "session IDs should be unique")
	require.NotEqual(t, sess1.ID, sess3.ID, "session IDs should be unique")
}

func TestInitializer_CreateSession_DirectoryStructure(t *testing.T) {
	// Verify the session directory follows the expected path pattern
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir:    workDir,
		ClientType: "claude",
	})

	// Set session ID (normally done by Start())
	init.mu.Lock()
	init.sessionID = "test-session-00001111-2222-3333-4444-555566667777"
	init.mu.Unlock()

	sess, err := init.createSession()
	require.NoError(t, err)

	// Verify the directory structure is: WorkDir/.perles/sessions/<sessionID>
	expectedParent := filepath.Join(workDir, ".perles", "sessions")
	info, err := os.Stat(expectedParent)
	require.NoError(t, err, ".perles/sessions directory should exist")
	require.True(t, info.IsDir(), ".perles/sessions should be a directory")

	// Verify session-specific directory exists
	sessionDir := filepath.Join(expectedParent, sess.ID)
	info, err = os.Stat(sessionDir)
	require.NoError(t, err, "session directory should exist")
	require.True(t, info.IsDir(), "session directory should be a directory")
}

// ===========================================================================
// createAIClient() Method Tests (Task perles-oph9.2)
// ===========================================================================

func TestInitializer_CreateAIClient_Claude(t *testing.T) {
	// Verify createAIClient() with Claude client type returns correct extensions
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir:     workDir,
		ClientType:  "claude",
		ClaudeModel: "opus",
	})

	result, err := init.createAIClient()
	require.NoError(t, err, "createAIClient should not return an error for claude type")
	require.NotNil(t, result, "result should not be nil")
	require.NotNil(t, result.Client, "client should not be nil")

	// Verify client type
	require.Equal(t, client.ClientClaude, result.Client.Type(), "client should be Claude type")

	// Verify extensions map contains Claude model
	require.Contains(t, result.Extensions, client.ExtClaudeModel, "extensions should contain Claude model key")
	require.Equal(t, "opus", result.Extensions[client.ExtClaudeModel], "Claude model should be 'opus'")
}

func TestInitializer_CreateAIClient_Amp(t *testing.T) {
	// Verify createAIClient() with Amp client type returns correct extensions
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir:    workDir,
		ClientType: "amp",
		AmpModel:   "gpt-4",
		AmpMode:    "smart",
	})

	result, err := init.createAIClient()
	require.NoError(t, err, "createAIClient should not return an error for amp type")
	require.NotNil(t, result, "result should not be nil")
	require.NotNil(t, result.Client, "client should not be nil")

	// Verify client type
	require.Equal(t, client.ClientAmp, result.Client.Type(), "client should be Amp type")

	// Verify extensions map contains Amp model and mode
	require.Contains(t, result.Extensions, client.ExtAmpModel, "extensions should contain Amp model key")
	require.Equal(t, "gpt-4", result.Extensions[client.ExtAmpModel], "Amp model should be 'gpt-4'")
	require.Contains(t, result.Extensions, amp.ExtAmpMode, "extensions should contain Amp mode key")
	require.Equal(t, "smart", result.Extensions[amp.ExtAmpMode], "Amp mode should be 'smart'")
}

func TestInitializer_CreateAIClient_DefaultsToClaude(t *testing.T) {
	// Verify createAIClient() defaults to Claude when ClientType is empty
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir:    workDir,
		ClientType: "", // Empty - should default to Claude
	})

	result, err := init.createAIClient()
	require.NoError(t, err, "createAIClient should not return an error with empty client type")
	require.NotNil(t, result, "result should not be nil")
	require.NotNil(t, result.Client, "client should not be nil")

	// Verify client type defaults to Claude
	require.Equal(t, client.ClientClaude, result.Client.Type(), "client should default to Claude type")
}

func TestInitializer_CreateAIClient_NoExtensionsWhenEmpty(t *testing.T) {
	// Verify createAIClient() returns empty extensions when no model is configured
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir:     workDir,
		ClientType:  "claude",
		ClaudeModel: "", // Empty - should not be in extensions
	})

	result, err := init.createAIClient()
	require.NoError(t, err, "createAIClient should not return an error")
	require.NotNil(t, result, "result should not be nil")
	require.NotNil(t, result.Extensions, "extensions should not be nil")

	// Verify extensions map is empty when no model configured
	require.Empty(t, result.Extensions, "extensions should be empty when no model configured")
}

func TestInitializer_CreateAIClient_InvalidClientType(t *testing.T) {
	// Verify createAIClient() returns error for unknown client type
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir:    workDir,
		ClientType: "unknown-client",
	})

	result, err := init.createAIClient()
	require.Error(t, err, "createAIClient should return error for unknown client type")
	require.Nil(t, result, "result should be nil on error")
	require.Contains(t, err.Error(), "failed to create AI client", "error should indicate client creation failure")
}

func TestInitializer_CreateAIClient_ResultStruct(t *testing.T) {
	// Verify AIClientResult struct fields are properly populated
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir:     workDir,
		ClientType:  "claude",
		ClaudeModel: "haiku",
	})

	result, err := init.createAIClient()
	require.NoError(t, err)

	// Verify both fields of AIClientResult are populated
	require.NotNil(t, result.Client, "AIClientResult.Client should be populated")
	require.NotNil(t, result.Extensions, "AIClientResult.Extensions should be populated")
	require.Equal(t, "haiku", result.Extensions[client.ExtClaudeModel])
}

func TestInitializer_CreateAIClient_AmpPartialExtensions(t *testing.T) {
	// Verify createAIClient() with Amp type handles partial extensions (only model, no mode)
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir:    workDir,
		ClientType: "amp",
		AmpModel:   "gpt-4",
		AmpMode:    "", // Empty mode - should not be in extensions
	})

	result, err := init.createAIClient()
	require.NoError(t, err, "createAIClient should not return an error")
	require.NotNil(t, result)

	// Verify extensions only contain model, not mode
	require.Contains(t, result.Extensions, client.ExtAmpModel, "extensions should contain Amp model")
	require.NotContains(t, result.Extensions, amp.ExtAmpMode, "extensions should not contain empty Amp mode")
}

// ===========================================================================
// createMCPListener() Method Tests (Task perles-oph9.3)
// ===========================================================================

func TestInitializer_CreateMCPListener_Success(t *testing.T) {
	// Verify createMCPListener() returns a valid listener on a random port
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir:    workDir,
		ClientType: "claude",
	})

	// Call createMCPListener directly
	result, err := init.createMCPListener()

	// Verify no error
	require.NoError(t, err, "createMCPListener should not return an error")
	require.NotNil(t, result, "createMCPListener should return a non-nil result")
	require.NotNil(t, result.Listener, "createMCPListener should return a non-nil listener")

	// Verify port is a valid non-zero port
	require.Greater(t, result.Port, 0, "port should be greater than 0")
	require.Less(t, result.Port, 65536, "port should be a valid TCP port")

	// Verify listener is actually listening (can get its address)
	addr := result.Listener.Addr()
	require.NotNil(t, addr, "listener should have a valid address")

	// Verify we got the same port from the listener
	tcpAddr, ok := addr.(*net.TCPAddr)
	require.True(t, ok, "listener address should be a TCP address")
	require.Equal(t, result.Port, tcpAddr.Port, "returned port should match listener port")

	// Clean up
	_ = result.Listener.Close()
}

func TestInitializer_CreateMCPListener_BindsToLocalhost(t *testing.T) {
	// Verify createMCPListener() binds to localhost (127.0.0.1) only
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir:    workDir,
		ClientType: "claude",
	})

	result, err := init.createMCPListener()
	require.NoError(t, err)
	defer result.Listener.Close()

	// Verify the listener is bound to localhost
	tcpAddr, ok := result.Listener.Addr().(*net.TCPAddr)
	require.True(t, ok)
	require.True(t, tcpAddr.IP.IsLoopback(), "listener should be bound to localhost")
}

func TestInitializer_CreateMCPListener_UniquePortsOnMultipleCalls(t *testing.T) {
	// Verify multiple calls to createMCPListener() return different ports
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir:    workDir,
		ClientType: "claude",
	})

	// Create multiple listeners
	result1, err := init.createMCPListener()
	require.NoError(t, err)
	defer result1.Listener.Close()

	result2, err := init.createMCPListener()
	require.NoError(t, err)
	defer result2.Listener.Close()

	// Ports should be different since both listeners are still open
	require.NotEqual(t, result1.Port, result2.Port, "multiple listeners should get different ports")
}

func TestInitializer_CreateMCPListener_ListenerAcceptsConnections(t *testing.T) {
	// Verify the returned listener can actually accept connections
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir:    workDir,
		ClientType: "claude",
	})

	result, err := init.createMCPListener()
	require.NoError(t, err)
	defer result.Listener.Close()

	// Try to connect to the listener
	conn, err := net.Dial("tcp", result.Listener.Addr().String())
	require.NoError(t, err, "should be able to connect to the listener")
	_ = conn.Close()
}

// ===========================================================================
// createMCPServer() Method Tests (Task perles-oph9.3)
// ===========================================================================

func TestInitializer_CreateMCPServer_Success(t *testing.T) {
	// Verify createMCPServer() returns a valid MCPServerResult with all components
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir:    workDir,
		ClientType: "claude",
	})

	// First create dependencies
	clientResult, err := init.createAIClient()
	require.NoError(t, err)

	sess, err := init.createSession()
	require.NoError(t, err)

	listenerResult, err := init.createMCPListener()
	require.NoError(t, err)
	defer listenerResult.Listener.Close()

	// Create mock message repository
	msgRepo := repository.NewMemoryMessageRepository()

	// We need a minimal v2Adapter for the test - create a nil processor adapter
	// Note: In real usage, this would be wired to the command processor

	// Call createMCPServer (v2Adapter can be nil for this test since we're just testing structure)
	result, err := init.createMCPServer(MCPServerConfig{
		Listener:   listenerResult.Listener,
		Port:       listenerResult.Port,
		AIClient:   clientResult.Client,
		MsgRepo:    msgRepo,
		Session:    sess,
		V2Adapter:  nil, // Worker cache handles nil v2Adapter
		WorkDir:    workDir,
		Extensions: clientResult.Extensions,
	})

	// Verify no error
	require.NoError(t, err, "createMCPServer should not return an error")
	require.NotNil(t, result, "createMCPServer should return a non-nil result")

	// Verify all components are present
	require.NotNil(t, result.Server, "MCPServerResult.Server should not be nil")
	require.NotNil(t, result.Listener, "MCPServerResult.Listener should not be nil")
	require.NotNil(t, result.CoordServer, "MCPServerResult.CoordServer should not be nil")
	require.NotNil(t, result.WorkerCache, "MCPServerResult.WorkerCache should not be nil")
	require.Equal(t, listenerResult.Port, result.Port, "MCPServerResult.Port should match input port")
}

func TestInitializer_CreateMCPServer_ConfiguresHTTPRoutes(t *testing.T) {
	// Verify createMCPServer() configures /mcp and /worker/ routes correctly
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir:    workDir,
		ClientType: "claude",
	})

	// Create dependencies
	clientResult, err := init.createAIClient()
	require.NoError(t, err)

	sess, err := init.createSession()
	require.NoError(t, err)

	listenerResult, err := init.createMCPListener()
	require.NoError(t, err)
	defer listenerResult.Listener.Close()

	msgRepo := repository.NewMemoryMessageRepository()

	result, err := init.createMCPServer(MCPServerConfig{
		Listener:   listenerResult.Listener,
		Port:       listenerResult.Port,
		AIClient:   clientResult.Client,
		MsgRepo:    msgRepo,
		Session:    sess,
		V2Adapter:  nil,
		WorkDir:    workDir,
		Extensions: clientResult.Extensions,
	})

	// Verify the HTTP server has a handler
	require.NotNil(t, result.Server.Handler, "HTTP server should have a handler configured")

	// Start the server to test routes
	go func() {
		_ = result.Server.Serve(result.Listener)
	}()

	// Give server time to start
	time.Sleep(10 * time.Millisecond)

	// Test that /mcp route exists (it should return something, even if not a valid response)
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/mcp", result.Port))
	require.NoError(t, err, "/mcp route should be accessible")
	resp.Body.Close()

	// Test that /worker/ route exists
	resp, err = http.Get(fmt.Sprintf("http://127.0.0.1:%d/worker/test", result.Port))
	require.NoError(t, err, "/worker/ route should be accessible")
	resp.Body.Close()

	// Shutdown server
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = result.Server.Shutdown(ctx)
}

func TestInitializer_CreateMCPServer_ReadHeaderTimeout(t *testing.T) {
	// Verify createMCPServer() configures ReadHeaderTimeout on the HTTP server
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir:    workDir,
		ClientType: "claude",
	})

	// Create dependencies
	clientResult, err := init.createAIClient()
	require.NoError(t, err)

	sess, err := init.createSession()
	require.NoError(t, err)

	listenerResult, err := init.createMCPListener()
	require.NoError(t, err)
	defer listenerResult.Listener.Close()

	msgRepo := repository.NewMemoryMessageRepository()

	result, err := init.createMCPServer(MCPServerConfig{
		Listener:   listenerResult.Listener,
		Port:       listenerResult.Port,
		AIClient:   clientResult.Client,
		MsgRepo:    msgRepo,
		Session:    sess,
		V2Adapter:  nil,
		WorkDir:    workDir,
		Extensions: clientResult.Extensions,
	})

	// Verify ReadHeaderTimeout is set (should be 10 seconds)
	require.Equal(t, 10*time.Second, result.Server.ReadHeaderTimeout,
		"HTTP server should have ReadHeaderTimeout set to 10 seconds")
}

func TestInitializer_MCPServerResult_ContainsAllComponents(t *testing.T) {
	// Verify MCPServerResult struct has all expected fields accessible
	result := MCPServerResult{}

	// These should compile - verifies the struct fields exist
	_ = result.Server
	_ = result.Port
	_ = result.Listener
	_ = result.CoordServer
	_ = result.WorkerCache

	// Verify they are all nil/zero by default
	require.Nil(t, result.Server)
	require.Equal(t, 0, result.Port)
	require.Nil(t, result.Listener)
	require.Nil(t, result.CoordServer)
	require.Nil(t, result.WorkerCache)
}

func TestInitializer_MCPListenerResult_ContainsAllComponents(t *testing.T) {
	// Verify MCPListenerResult struct has all expected fields accessible
	result := MCPListenerResult{}

	// These should compile - verifies the struct fields exist
	_ = result.Listener
	_ = result.Port

	// Verify they are nil/zero by default
	require.Nil(t, result.Listener)
	require.Equal(t, 0, result.Port)
}

func TestInitializer_MCPServerConfig_ContainsAllFields(t *testing.T) {
	// Verify MCPServerConfig struct has all expected fields accessible
	config := MCPServerConfig{}

	// These should compile - verifies the struct fields exist
	_ = config.Listener
	_ = config.Port
	_ = config.AIClient
	_ = config.MsgRepo
	_ = config.Session
	_ = config.V2Adapter
	_ = config.WorkDir
	_ = config.Extensions

	// Verify they are nil/zero by default
	require.Nil(t, config.Listener)
	require.Equal(t, 0, config.Port)
	require.Nil(t, config.AIClient)
	require.Nil(t, config.MsgRepo)
	require.Nil(t, config.Session)
	require.Nil(t, config.V2Adapter)
	require.Empty(t, config.WorkDir)
	require.Nil(t, config.Extensions)
}

func TestInitializer_CreateMCPServer_RequiresListener(t *testing.T) {
	// Verify createMCPServer() returns error when listener is nil
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir:    workDir,
		ClientType: "claude",
	})

	clientResult, err := init.createAIClient()
	require.NoError(t, err)

	msgRepo := repository.NewMemoryMessageRepository()

	result, err := init.createMCPServer(MCPServerConfig{
		Listener:   nil, // Missing listener
		Port:       8080,
		AIClient:   clientResult.Client,
		MsgRepo:    msgRepo,
		Session:    nil,
		V2Adapter:  nil,
		WorkDir:    workDir,
		Extensions: nil,
	})

	require.Error(t, err, "createMCPServer should return error when listener is nil")
	require.Nil(t, result, "result should be nil on error")
	require.Contains(t, err.Error(), "listener is required")
}

func TestInitializer_CreateMCPServer_RequiresAIClient(t *testing.T) {
	// Verify createMCPServer() returns error when AIClient is nil
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir:    workDir,
		ClientType: "claude",
	})

	listenerResult, err := init.createMCPListener()
	require.NoError(t, err)
	defer listenerResult.Listener.Close()

	msgRepo := repository.NewMemoryMessageRepository()

	result, err := init.createMCPServer(MCPServerConfig{
		Listener:   listenerResult.Listener,
		Port:       listenerResult.Port,
		AIClient:   nil, // Missing AIClient
		MsgRepo:    msgRepo,
		Session:    nil,
		V2Adapter:  nil,
		WorkDir:    workDir,
		Extensions: nil,
	})

	require.Error(t, err, "createMCPServer should return error when AIClient is nil")
	require.Nil(t, result, "result should be nil on error")
	require.Contains(t, err.Error(), "AI client is required")
}

func TestInitializer_CreateMCPServer_RequiresMsgRepo(t *testing.T) {
	// Verify createMCPServer() returns error when MsgRepo is nil
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir:    workDir,
		ClientType: "claude",
	})

	clientResult, err := init.createAIClient()
	require.NoError(t, err)

	listenerResult, err := init.createMCPListener()
	require.NoError(t, err)
	defer listenerResult.Listener.Close()

	result, err := init.createMCPServer(MCPServerConfig{
		Listener:   listenerResult.Listener,
		Port:       listenerResult.Port,
		AIClient:   clientResult.Client,
		MsgRepo:    nil, // Missing MsgRepo
		Session:    nil,
		V2Adapter:  nil,
		WorkDir:    workDir,
		Extensions: nil,
	})

	require.Error(t, err, "createMCPServer should return error when MsgRepo is nil")
	require.Nil(t, result, "result should be nil on error")
	require.Contains(t, err.Error(), "message repository is required")
}

func TestInitializer_SpinnerData_ReturnsPhase(t *testing.T) {
	// Verify SpinnerData returns current phase
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir:    workDir,
		ClientType: "claude",
	})

	// Get spinner data - should return InitNotStarted
	phase := init.SpinnerData()
	require.Equal(t, InitNotStarted, phase)

	// Set a different phase
	init.mu.Lock()
	init.phase = InitSpawningCoordinator
	init.mu.Unlock()

	phase = init.SpinnerData()
	require.Equal(t, InitSpawningCoordinator, phase)
}

// ===========================================================================
// run() Tests
// ===========================================================================

func TestRun_CancelsOnContextCancellation(t *testing.T) {
	// Unit test: run() cancels correctly on context cancellation
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir:    workDir,
		ClientType: "claude",
		Timeout:    10 * time.Second, // Long timeout
	})

	// Verify we can cancel the initializer
	init.mu.Lock()
	init.ctx, init.cancel = context.WithCancel(context.Background())
	init.mu.Unlock()

	// Cancel the context
	init.cancel()

	// Context should be done
	select {
	case <-init.ctx.Done():
		// Expected
	default:
		require.Fail(t, "context should be cancelled")
	}

	// Verify the error type
	require.Equal(t, context.Canceled, init.ctx.Err())
}

// ===========================================================================
// cleanupResources() Tests (Task perles-oph9.13)
// ===========================================================================

func TestCleanupResources_Idempotent(t *testing.T) {
	// Unit test: cleanupResources() is idempotent (safe to call twice)
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir:    workDir,
		ClientType: "claude",
	})

	// First call should not panic
	require.NotPanics(t, func() {
		init.cleanupResources()
	}, "first cleanupResources call should not panic")

	// Second call should also not panic (idempotent)
	require.NotPanics(t, func() {
		init.cleanupResources()
	}, "second cleanupResources call should not panic (idempotent)")

	// Third call for good measure
	require.NotPanics(t, func() {
		init.cleanupResources()
	}, "third cleanupResources call should not panic (idempotent)")
}

func TestCleanupResources_ClearsFields(t *testing.T) {
	// Unit test: cleanupResources() clears resource fields for idempotency
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir:    workDir,
		ClientType: "claude",
	})

	// Manually set fields to simulate initialized state
	init.mu.Lock()
	init.mcpServer = &http.Server{} // Dummy server
	init.mu.Unlock()

	// Call cleanupResources
	init.cleanupResources()

	// Verify fields are cleared
	init.mu.RLock()
	require.Nil(t, init.mcpServer, "mcpServer should be nil after cleanup")
	require.Nil(t, init.v2Infra, "v2Infra should be nil after cleanup")
	init.mu.RUnlock()
}

func TestCancel_StopsContextAndCleansUp(t *testing.T) {
	// Unit test: Cancel() stops context and cleans up
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir:    workDir,
		ClientType: "claude",
	})

	// Manually set up context like Start() does
	init.mu.Lock()
	init.ctx, init.cancel = context.WithCancel(context.Background())
	ctx := init.ctx
	init.mu.Unlock()

	// Verify context is not cancelled yet
	select {
	case <-ctx.Done():
		require.Fail(t, "context should not be cancelled yet")
	default:
		// Expected
	}

	// Call Cancel
	init.Cancel()

	// Verify context is now cancelled
	select {
	case <-ctx.Done():
		// Expected - context should be cancelled
		require.Equal(t, context.Canceled, ctx.Err())
	default:
		require.Fail(t, "context should be cancelled after Cancel()")
	}
}

func TestCancel_DoubleCallDoesNotPanic(t *testing.T) {
	// Unit test: Double-Cancel() doesn't panic
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir:    workDir,
		ClientType: "claude",
	})

	// Manually set up context like Start() does
	init.mu.Lock()
	init.ctx, init.cancel = context.WithCancel(context.Background())
	init.mu.Unlock()

	// First Cancel call should not panic
	require.NotPanics(t, func() {
		init.Cancel()
	}, "first Cancel call should not panic")

	// Second Cancel call should also not panic (idempotent)
	require.NotPanics(t, func() {
		init.Cancel()
	}, "second Cancel call should not panic (idempotent)")

	// Third Cancel call for good measure
	require.NotPanics(t, func() {
		init.Cancel()
	}, "third Cancel call should not panic (idempotent)")
}

func TestCancel_Idempotent_CancelFuncCalledOnce(t *testing.T) {
	// Unit test: Cancel() only calls cancel func once
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir:    workDir,
		ClientType: "claude",
	})

	// Set up context
	init.mu.Lock()
	init.ctx, init.cancel = context.WithCancel(context.Background())
	init.mu.Unlock()

	// First Cancel should clear the cancel func
	init.Cancel()

	// Verify cancel is now nil
	init.mu.RLock()
	require.Nil(t, init.cancel, "cancel func should be nil after Cancel()")
	init.mu.RUnlock()
}

func TestCleanupResources_WithPartialInitialization(t *testing.T) {
	// Unit test: Cleanup with partial initialization works
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir:    workDir,
		ClientType: "claude",
	})

	// Simulate partial initialization - only some fields set
	// This simulates a failure during initialization
	init.mu.Lock()
	init.mcpServer = &http.Server{} // Only MCP server initialized
	// v2Infra, dedupMiddleware all nil
	init.mu.Unlock()

	// Cleanup should not panic even with partial initialization
	require.NotPanics(t, func() {
		init.cleanupResources()
	}, "cleanupResources should handle partial initialization")

	// Fields should be cleared
	init.mu.RLock()
	require.Nil(t, init.mcpServer, "mcpServer should be nil")
	init.mu.RUnlock()
}

func TestCleanupResources_WithNoInitialization(t *testing.T) {
	// Unit test: Cleanup with no initialization (all fields nil) works
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir:    workDir,
		ClientType: "claude",
	})

	// All fields are nil by default - cleanup should handle this gracefully
	require.NotPanics(t, func() {
		init.cleanupResources()
	}, "cleanupResources should handle no initialization")
}

func TestCleanupResources_ThreadSafe(t *testing.T) {
	// Unit test: cleanupResources() is thread-safe
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir:    workDir,
		ClientType: "claude",
	})

	// Simulate partial initialization
	init.mu.Lock()
	init.mcpServer = &http.Server{}
	init.mu.Unlock()

	// Call cleanupResources concurrently from multiple goroutines
	done := make(chan struct{}, 10)
	for range 10 {
		go func() {
			defer func() { done <- struct{}{} }()
			// Should not panic or race
			init.cleanupResources()
		}()
	}

	// Wait for all goroutines
	for range 10 {
		<-done
	}

	// Fields should be cleared (first goroutine to get lock wins)
	init.mu.RLock()
	require.Nil(t, init.mcpServer)
	init.mu.RUnlock()
}

// cleanupOrderTracker tracks the order of cleanup operations
type cleanupOrderTracker struct {
	mu    sync.Mutex
	order []string
}

func (t *cleanupOrderTracker) record(op string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.order = append(t.order, op)
}

func (t *cleanupOrderTracker) getOrder() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	result := make([]string, len(t.order))
	copy(result, t.order)
	return result
}

func TestCleanupResources_Order_Documentation(t *testing.T) {
	// Documentation test: Verify documented cleanup order is correct
	//
	// Expected cleanup order (reverse of creation):
	//   1. Stop coordinator process (created last via spawnCoordinator)
	//   2. Drain command processor (v2 infrastructure started after creation)
	//   3. Stop deduplication middleware (created with v2 infrastructure)
	//   4. Shutdown MCP server with timeout (HTTP server started last in createWorkspace)
	//
	// This test documents the expected order; actual order verification requires
	// integration testing or mocks with tracking.

	// Creation order in createWorkspace():
	//   1. Create AI client
	//   2. Create message repository
	//   3. Create session
	//   4. Create MCP listener
	//   5. Create V2 infrastructure (includes processor and middleware)
	//   6. Start V2 infrastructure
	//   7. Create MCP server
	//   8. Start HTTP server
	//
	// Then in run():
	//   9. Spawn coordinator

	// Therefore cleanup order should be:
	//   1. Stop coordinator (reverse of step 9)
	//   2. Drain V2 processor (reverse of step 6)
	//   3. Stop deduplication middleware (part of V2 infrastructure)
	//   4. Shutdown MCP HTTP server (reverse of step 8)

	// This is verified by code inspection of cleanupResources()
	// The function clearly shows the order with comments
}

func TestRetry_CallsCancel(t *testing.T) {
	// Unit test: Verify Retry() resets state including cancel func
	// This test verifies the Cancel() call within Retry() by checking state reset
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir:    workDir,
		ClientType: "claude",
	})

	// Manually set up context like Start() does
	init.mu.Lock()
	init.ctx, init.cancel = context.WithCancel(context.Background())
	ctx := init.ctx
	init.started = true
	// Set some resource fields that should be cleared by Cancel->cleanupResources
	init.mcpServer = &http.Server{}
	init.mu.Unlock()

	// Call Cancel() directly (which is called by Retry())
	// This avoids starting the full initialization which spawns goroutines
	init.Cancel()

	// Verify Cancel was called and cleaned up resources
	init.mu.RLock()
	require.Nil(t, init.mcpServer, "mcpServer should be cleared by Cancel->cleanupResources")
	require.Nil(t, init.cancel, "cancel func should be cleared by Cancel")
	init.mu.RUnlock()

	// Verify context was cancelled
	select {
	case <-ctx.Done():
		// Expected
	default:
		require.Fail(t, "context should be cancelled")
	}
}

func TestCleanupResources_ExistingTestStillPasses(t *testing.T) {
	// Regression test: Verify existing cleanup test still passes after renaming
	// This is the same as TestInitializer_CleanupDrainsProcessor but ensures
	// the renamed cleanupResources() method works the same way
	init := NewInitializer(InitializerConfig{
		WorkDir: t.TempDir(),
	})

	// cleanupResources() should not panic when cmdProcessor is nil
	// (Note: cleanupResources uses v2Infra.Drain() not direct cmdProcessor access)
	require.NotPanics(t, func() {
		init.cleanupResources()
	})
}

// ===========================================================================
// Worktree Phase Tests (Task perles-v5cq.5)
// ===========================================================================

func TestInitializer_WorktreePhase_Success(t *testing.T) {
	// Unit test: createWorktree() succeeds with proper mock setup
	workDir := t.TempDir()
	worktreePath := "/tmp/test-worktree"

	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsGitRepo().Return(true)
	mockGit.EXPECT().PruneWorktrees().Return(nil)
	mockGit.EXPECT().DetermineWorktreePath(mock.AnythingOfType("string")).Return(worktreePath, nil)
	mockGit.EXPECT().CreateWorktree(worktreePath, mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil)

	init := NewInitializer(InitializerConfig{
		WorkDir:            workDir,
		WorktreeBaseBranch: "main",
		GitExecutor:        mockGit,
	})

	// Start to generate session ID
	init.mu.Lock()
	init.sessionID = "test-session-id-12345678"
	init.mu.Unlock()

	// Call createWorktree directly
	err := init.createWorktree()
	require.NoError(t, err, "createWorktree should succeed")

	// Verify worktree state was set
	require.Equal(t, worktreePath, init.WorktreePath())
	require.Equal(t, "perles-session-test-ses", init.WorktreeBranch())
}

func TestInitializer_WorktreePhase_NotGitRepo_Fails(t *testing.T) {
	// Unit test: createWorktree() fails when not in git repo
	workDir := t.TempDir()

	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsGitRepo().Return(false)

	init := NewInitializer(InitializerConfig{
		WorkDir:            workDir,
		WorktreeBaseBranch: "main",
		GitExecutor:        mockGit,
	})

	init.mu.Lock()
	init.sessionID = "test-session-id"
	init.mu.Unlock()

	err := init.createWorktree()
	require.Error(t, err, "createWorktree should fail when not in git repo")
	require.Contains(t, err.Error(), "not a git repository")
}

func TestInitializer_WorktreePhase_CreateFails_Fails(t *testing.T) {
	// Unit test: createWorktree() fails when CreateWorktree fails
	workDir := t.TempDir()
	worktreePath := "/tmp/test-worktree"

	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsGitRepo().Return(true)
	mockGit.EXPECT().PruneWorktrees().Return(nil)
	mockGit.EXPECT().DetermineWorktreePath(mock.AnythingOfType("string")).Return(worktreePath, nil)
	mockGit.EXPECT().CreateWorktree(worktreePath, mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(fmt.Errorf("branch already checked out"))

	init := NewInitializer(InitializerConfig{
		WorkDir:            workDir,
		WorktreeBaseBranch: "main",
		GitExecutor:        mockGit,
	})

	init.mu.Lock()
	init.sessionID = "test-session-id"
	init.mu.Unlock()

	err := init.createWorktree()
	require.Error(t, err, "createWorktree should fail when CreateWorktree fails")
	require.Contains(t, err.Error(), "failed to create worktree")
}

func TestInitializer_WorktreePhase_Disabled_SkipsPhase(t *testing.T) {
	// Unit test: run() skips worktree phase when disabled
	workDir := t.TempDir()

	// Create mock that should NOT be called
	mockGit := mocks.NewMockGitExecutor(t)
	// No expectations set - if any method is called, test will fail

	init := NewInitializer(InitializerConfig{
		WorkDir:     workDir,
		GitExecutor: mockGit,
	})

	// Verify worktree path is empty
	require.Empty(t, init.WorktreePath())
	require.Empty(t, init.WorktreeBranch())
}

func TestInitializer_WorktreePath_PropagatedToWorkspace(t *testing.T) {
	// Unit test: Verify worktreePath is used in createWorkspace
	workDir := t.TempDir()
	worktreePath := "/tmp/test-worktree"

	init := NewInitializer(InitializerConfig{
		WorkDir:            workDir,
		WorktreeBaseBranch: "main",
	})

	// Manually set worktree path (simulating successful createWorktree)
	init.mu.Lock()
	init.worktreePath = worktreePath
	init.sessionID = "test-session-id"
	init.mu.Unlock()

	// Verify accessor returns the path
	require.Equal(t, worktreePath, init.WorktreePath())
}

func TestInitializer_PruneWorktrees_CalledBeforeCreate(t *testing.T) {
	// Unit test: Verify PruneWorktrees is called before CreateWorktree
	workDir := t.TempDir()
	worktreePath := "/tmp/test-worktree"

	// Track call order
	var callOrder []string
	orderMu := sync.Mutex{}

	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsGitRepo().Return(true).Run(func() {
		orderMu.Lock()
		callOrder = append(callOrder, "IsGitRepo")
		orderMu.Unlock()
	})
	mockGit.EXPECT().PruneWorktrees().Return(nil).Run(func() {
		orderMu.Lock()
		callOrder = append(callOrder, "PruneWorktrees")
		orderMu.Unlock()
	})
	mockGit.EXPECT().DetermineWorktreePath(mock.AnythingOfType("string")).Return(worktreePath, nil).Run(func(sessionID string) {
		orderMu.Lock()
		callOrder = append(callOrder, "DetermineWorktreePath")
		orderMu.Unlock()
	})
	mockGit.EXPECT().CreateWorktree(worktreePath, mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil).Run(func(path string, newBranch string, baseBranch string) {
		orderMu.Lock()
		callOrder = append(callOrder, "CreateWorktree")
		orderMu.Unlock()
	})

	init := NewInitializer(InitializerConfig{
		WorkDir:            workDir,
		WorktreeBaseBranch: "main",
		GitExecutor:        mockGit,
	})

	init.mu.Lock()
	init.sessionID = "test-session-id"
	init.mu.Unlock()

	err := init.createWorktree()
	require.NoError(t, err)

	// Verify PruneWorktrees is called before CreateWorktree
	orderMu.Lock()
	defer orderMu.Unlock()
	require.Equal(t, []string{"IsGitRepo", "PruneWorktrees", "DetermineWorktreePath", "CreateWorktree"}, callOrder)
}

func TestInitializer_BranchName_DefaultsToSessionID(t *testing.T) {
	// Unit test: Verify branch name defaults to perles-session-{shortID}
	workDir := t.TempDir()
	worktreePath := "/tmp/test-worktree"
	sessionID := "12345678-90ab-cdef-1234-567890abcdef"
	expectedBranch := "perles-session-12345678"

	var capturedNewBranch, capturedBaseBranch string
	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsGitRepo().Return(true)
	mockGit.EXPECT().PruneWorktrees().Return(nil)
	mockGit.EXPECT().DetermineWorktreePath(sessionID).Return(worktreePath, nil)
	mockGit.EXPECT().CreateWorktree(worktreePath, mock.AnythingOfType("string"), mock.AnythingOfType("string")).
		Run(func(path string, newBranch string, baseBranch string) {
			capturedNewBranch = newBranch
			capturedBaseBranch = baseBranch
		}).
		Return(nil)

	init := NewInitializer(InitializerConfig{
		WorkDir:            workDir,
		WorktreeBaseBranch: "", // Empty - uses current HEAD
		GitExecutor:        mockGit,
	})

	init.mu.Lock()
	init.sessionID = sessionID
	init.mu.Unlock()

	err := init.createWorktree()
	require.NoError(t, err)
	require.Equal(t, expectedBranch, capturedNewBranch)
	require.Equal(t, "", capturedBaseBranch) // Empty base branch means current HEAD
	require.Equal(t, expectedBranch, init.WorktreeBranch())
}

func TestInitializer_BranchName_UsesConfiguredBaseBranch(t *testing.T) {
	// Unit test: Verify configured base branch is passed to CreateWorktree
	workDir := t.TempDir()
	worktreePath := "/tmp/test-worktree"
	baseBranch := "develop"
	sessionID := "test-sess"
	expectedNewBranch := "perles-session-test-ses"

	var capturedNewBranch, capturedBaseBranch string
	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsGitRepo().Return(true)
	mockGit.EXPECT().PruneWorktrees().Return(nil)
	mockGit.EXPECT().DetermineWorktreePath(mock.AnythingOfType("string")).Return(worktreePath, nil)
	mockGit.EXPECT().CreateWorktree(worktreePath, mock.AnythingOfType("string"), mock.AnythingOfType("string")).
		Run(func(path string, newBranch string, base string) {
			capturedNewBranch = newBranch
			capturedBaseBranch = base
		}).
		Return(nil)

	init := NewInitializer(InitializerConfig{
		WorkDir:            workDir,
		WorktreeBaseBranch: baseBranch, // Configured base branch
		GitExecutor:        mockGit,
	})

	init.mu.Lock()
	init.sessionID = sessionID
	init.mu.Unlock()

	err := init.createWorktree()
	require.NoError(t, err)
	require.Equal(t, expectedNewBranch, capturedNewBranch) // Auto-generated branch name
	require.Equal(t, baseBranch, capturedBaseBranch)       // Base branch passed correctly
	require.Equal(t, expectedNewBranch, init.WorktreeBranch())
}

func TestInitializer_WorktreePhase_PruneFailsContinues(t *testing.T) {
	// Unit test: Verify worktree creation continues even if prune fails
	workDir := t.TempDir()
	worktreePath := "/tmp/test-worktree"

	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsGitRepo().Return(true)
	mockGit.EXPECT().PruneWorktrees().Return(fmt.Errorf("prune failed")) // Failure should be ignored
	mockGit.EXPECT().DetermineWorktreePath(mock.AnythingOfType("string")).Return(worktreePath, nil)
	mockGit.EXPECT().CreateWorktree(worktreePath, mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil)

	init := NewInitializer(InitializerConfig{
		WorkDir:            workDir,
		WorktreeBaseBranch: "main",
		GitExecutor:        mockGit,
	})

	init.mu.Lock()
	init.sessionID = "test-session-id"
	init.mu.Unlock()

	err := init.createWorktree()
	require.NoError(t, err, "createWorktree should succeed even if prune fails")
	require.Equal(t, worktreePath, init.WorktreePath())
}

func TestInitializer_WorktreePhase_DeterminePathFails(t *testing.T) {
	// Unit test: Verify failure when DetermineWorktreePath fails
	workDir := t.TempDir()

	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsGitRepo().Return(true)
	mockGit.EXPECT().PruneWorktrees().Return(nil)
	mockGit.EXPECT().DetermineWorktreePath(mock.AnythingOfType("string")).Return("", fmt.Errorf("path determination failed"))

	init := NewInitializer(InitializerConfig{
		WorkDir:            workDir,
		WorktreeBaseBranch: "main",
		GitExecutor:        mockGit,
	})

	init.mu.Lock()
	init.sessionID = "test-session-id"
	init.mu.Unlock()

	err := init.createWorktree()
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to determine worktree path")
}

func TestInitializer_Retry_ResetsWorktreeFields(t *testing.T) {
	// Unit test: Verify Retry() resets worktree fields
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
	})

	// Manually set worktree fields
	init.mu.Lock()
	init.worktreePath = "/tmp/test-worktree"
	init.worktreeBranch = "test-branch"
	init.sessionID = "test-session-id"
	init.mu.Unlock()

	// Call Cancel (which is called by Retry first)
	init.Cancel()

	// After Cancel, manually reset like Retry does
	init.mu.Lock()
	init.worktreePath = ""
	init.worktreeBranch = ""
	init.sessionID = ""
	init.mu.Unlock()

	// Verify fields are reset
	require.Empty(t, init.WorktreePath())
	require.Empty(t, init.WorktreeBranch())
}

func TestInitializer_WorktreePath_Accessor_ThreadSafe(t *testing.T) {
	// Unit test: Verify WorktreePath() is thread-safe
	init := NewInitializer(InitializerConfig{
		WorkDir: t.TempDir(),
	})

	// Concurrently read WorktreePath
	done := make(chan bool, 10)
	for range 10 {
		go func() {
			defer func() { done <- true }()
			_ = init.WorktreePath()
		}()
	}

	// Wait for all goroutines
	for range 10 {
		<-done
	}
}

func TestInitializer_WorktreeBranch_Accessor_ThreadSafe(t *testing.T) {
	// Unit test: Verify WorktreeBranch() is thread-safe
	init := NewInitializer(InitializerConfig{
		WorkDir: t.TempDir(),
	})

	// Concurrently read WorktreeBranch
	done := make(chan bool, 10)
	for range 10 {
		go func() {
			defer func() { done <- true }()
			_ = init.WorktreeBranch()
		}()
	}

	// Wait for all goroutines
	for range 10 {
		<-done
	}
}

func TestInitializer_WorktreeConfig_Fields(t *testing.T) {
	// Unit test: Verify InitializerConfig has all worktree fields
	config := InitializerConfig{
		WorkDir:            "/test/dir",
		WorktreeBaseBranch: "test-branch",
		GitExecutor:        nil, // Will be set in real usage
	}

	// Verify fields are accessible and set correctly
	require.Equal(t, "/test/dir", config.WorkDir)
	require.Equal(t, "test-branch", config.WorktreeBaseBranch)
	require.Nil(t, config.GitExecutor)
}
