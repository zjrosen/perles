package handler

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/v2/process"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
)

func TestProcessRegistrySessionProvider_New(t *testing.T) {
	registry := process.NewProcessRegistry()
	aiClient := &mockHeadlessClient{clientType: client.ClientClaude}

	provider := NewProcessRegistrySessionProvider(registry, aiClient, aiClient, "/work/dir", 8765)

	require.NotNil(t, provider)
}

func TestProcessRegistrySessionProvider_GetProcessSessionID(t *testing.T) {
	registry := process.NewProcessRegistry()
	provider := NewProcessRegistrySessionProvider(registry, nil, nil, "/work/dir", 8765)

	// Create a mock headless process and wrap it in a Process
	mockProc := newMockHeadlessProcess("session-ref")
	proc := process.New("worker-1", repository.RoleWorker, mockProc, nil, nil)
	registry.Register(proc)

	// Simulate receiving init event that would set session ID
	// Since Process.sessionID is private, we'll test the GetProcessSessionID behavior
	// When no session ID is set, it returns an empty string
	sessionID, err := provider.GetProcessSessionID("worker-1")
	require.NoError(t, err)
	require.Empty(t, sessionID) // SessionID not set yet
}

func TestProcessRegistrySessionProvider_GetProcessSessionID_NotFound(t *testing.T) {
	registry := process.NewProcessRegistry()
	provider := NewProcessRegistrySessionProvider(registry, nil, nil, "/work/dir", 8765)

	sessionID, err := provider.GetProcessSessionID("nonexistent")
	require.Error(t, err)
	require.Contains(t, err.Error(), "process nonexistent not found")
	require.Empty(t, sessionID)
}

func TestProcessRegistrySessionProvider_GetProcessSessionID_EmptyID(t *testing.T) {
	registry := process.NewProcessRegistry()
	provider := NewProcessRegistrySessionProvider(registry, nil, nil, "/work/dir", 8765)

	sessionID, err := provider.GetProcessSessionID("")
	require.Error(t, err)
	require.Contains(t, err.Error(), "process ID cannot be empty")
	require.Empty(t, sessionID)
}

func TestProcessRegistrySessionProvider_GenerateProcessMCPConfig_Worker_HTTP(t *testing.T) {
	registry := process.NewProcessRegistry()
	aiClient := &mockHeadlessClient{clientType: client.ClientClaude}
	provider := NewProcessRegistrySessionProvider(registry, aiClient, aiClient, "/work/dir", 8765)

	config, err := provider.GenerateProcessMCPConfig("worker-1")
	require.NoError(t, err)
	require.Contains(t, config, "http://localhost:8765/worker/worker-1")
	require.Contains(t, config, "perles-worker")
	require.Contains(t, config, "mcpServers") // Claude format includes mcpServers wrapper
}

func TestProcessRegistrySessionProvider_GenerateProcessMCPConfig_Worker_Amp(t *testing.T) {
	registry := process.NewProcessRegistry()
	aiClient := &mockHeadlessClient{clientType: client.ClientAmp}
	provider := NewProcessRegistrySessionProvider(registry, aiClient, aiClient, "/work/dir", 9999)

	config, err := provider.GenerateProcessMCPConfig("worker-2")
	require.NoError(t, err)
	require.Contains(t, config, "http://localhost:9999/worker/worker-2")
	require.Contains(t, config, "perles-worker")
	// Amp format doesn't have mcpServers wrapper
	require.NotContains(t, config, "mcpServers")
}

func TestProcessRegistrySessionProvider_GenerateProcessMCPConfig_Coordinator_HTTP(t *testing.T) {
	registry := process.NewProcessRegistry()
	aiClient := &mockHeadlessClient{clientType: client.ClientClaude}
	provider := NewProcessRegistrySessionProvider(registry, aiClient, aiClient, "/work/dir", 8765)

	// Use the well-known coordinator ID
	config, err := provider.GenerateProcessMCPConfig(repository.CoordinatorID)
	require.NoError(t, err)
	require.Contains(t, config, "http://localhost:8765/mcp")
	require.Contains(t, config, "perles-orchestrator") // Coordinator server name
	require.Contains(t, config, "mcpServers")          // Claude format includes mcpServers wrapper
}

func TestProcessRegistrySessionProvider_GenerateProcessMCPConfig_Coordinator_Amp(t *testing.T) {
	registry := process.NewProcessRegistry()
	aiClient := &mockHeadlessClient{clientType: client.ClientAmp}
	provider := NewProcessRegistrySessionProvider(registry, aiClient, aiClient, "/work/dir", 9999)

	config, err := provider.GenerateProcessMCPConfig(repository.CoordinatorID)
	require.NoError(t, err)
	require.Contains(t, config, "http://localhost:9999/mcp")
	require.Contains(t, config, "perles-orchestrator") // Coordinator server name
	// Amp format doesn't have mcpServers wrapper
	require.NotContains(t, config, "mcpServers")
}

func TestProcessRegistrySessionProvider_GenerateProcessMCPConfig_NilClient(t *testing.T) {
	registry := process.NewProcessRegistry()
	// nil client should default to HTTP format
	provider := NewProcessRegistrySessionProvider(registry, nil, nil, "/work/dir", 8765)

	config, err := provider.GenerateProcessMCPConfig("worker-1")
	require.NoError(t, err)
	require.Contains(t, config, "http://localhost:8765/worker/worker-1")
	require.Contains(t, config, "mcpServers") // Default is Claude/HTTP format
}

func TestProcessRegistrySessionProvider_GetWorkDir(t *testing.T) {
	registry := process.NewProcessRegistry()
	provider := NewProcessRegistrySessionProvider(registry, nil, nil, "/my/work/dir", 8765)

	require.Equal(t, "/my/work/dir", provider.GetWorkDir())
}

func TestProcessRegistrySessionProvider_GetWorkDir_Empty(t *testing.T) {
	registry := process.NewProcessRegistry()
	provider := NewProcessRegistrySessionProvider(registry, nil, nil, "", 8765)

	require.Equal(t, "", provider.GetWorkDir())
}

// Verify ProcessRegistrySessionProvider implements SessionProvider interface
// by checking it has all the required methods via a local interface definition.
func TestProcessRegistrySessionProvider_ImplementsSessionProvider(t *testing.T) {
	registry := process.NewProcessRegistry()
	provider := NewProcessRegistrySessionProvider(registry, nil, nil, "/work", 8765)

	// Define a local interface matching the integration.SessionProvider
	type SessionProvider interface {
		GetProcessSessionID(processID string) (string, error)
		GenerateProcessMCPConfig(processID string) (string, error)
		GetWorkDir() string
	}

	var _ SessionProvider = provider

	// If the above compiles, the interface is implemented
}

func TestProcessRegistrySessionProvider_SessionCorrectlyWrapsProcess(t *testing.T) {
	registry := process.NewProcessRegistry()
	aiClient := &mockHeadlessClient{clientType: client.ClientClaude}
	provider := NewProcessRegistrySessionProvider(registry, aiClient, aiClient, "/project", 8765)

	// Create and register a process
	mockProc := newMockHeadlessProcess("session-ref")
	proc := process.New("worker-session-test", repository.RoleWorker, mockProc, nil, nil)
	registry.Register(proc)

	// The process is in the registry, so GetProcessSessionID should work
	sessionID, err := provider.GetProcessSessionID("worker-session-test")
	require.NoError(t, err)
	// Session ID is empty initially (would be set by init event)
	require.Empty(t, sessionID)

	// MCP config should work regardless of session ID
	config, err := provider.GenerateProcessMCPConfig("worker-session-test")
	require.NoError(t, err)
	require.Contains(t, config, "worker-session-test")

	// Work dir should be consistent
	require.Equal(t, "/project", provider.GetWorkDir())
}
