package handler

import (
	"fmt"

	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/mcp"
	"github.com/zjrosen/perles/internal/orchestration/v2/process"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
)

// ProcessRegistrySessionProvider implements the SessionProvider interface
// using the process.ProcessRegistry to look up process session information.
// Supports both coordinator and worker processes with different clients.
type ProcessRegistrySessionProvider struct {
	registry          *process.ProcessRegistry
	coordinatorClient client.HeadlessClient
	workerClient      client.HeadlessClient
	workDir           string
	port              int
}

// NewProcessRegistrySessionProvider creates a new ProcessRegistrySessionProvider.
//
// Parameters:
//   - registry: ProcessRegistry for looking up process sessions
//   - coordinatorClient: HeadlessClient for coordinator MCP config format
//   - workerClient: HeadlessClient for worker MCP config format
//   - workDir: Working directory for processes
//   - port: MCP server port for process connections
func NewProcessRegistrySessionProvider(
	registry *process.ProcessRegistry,
	coordinatorClient client.HeadlessClient,
	workerClient client.HeadlessClient,
	workDir string,
	port int,
) *ProcessRegistrySessionProvider {
	return &ProcessRegistrySessionProvider{
		registry:          registry,
		coordinatorClient: coordinatorClient,
		workerClient:      workerClient,
		workDir:           workDir,
		port:              port,
	}
}

// GetProcessSessionID returns the session ID for a process (coordinator or worker) from the registry.
// Returns an error if the process doesn't exist in the registry.
func (p *ProcessRegistrySessionProvider) GetProcessSessionID(processID string) (string, error) {
	if processID == "" {
		return "", fmt.Errorf("process ID cannot be empty")
	}

	proc := p.registry.Get(processID)
	if proc == nil {
		return "", fmt.Errorf("process %s not found in registry", processID)
	}
	return proc.SessionID(), nil
}

// GenerateProcessMCPConfig generates the appropriate MCP config JSON based on process role and client type.
// For coordinator, generates coordinator MCP config.
// For workers, generates worker MCP config with their ID.
func (p *ProcessRegistrySessionProvider) GenerateProcessMCPConfig(processID string) (string, error) {
	// Check if this is the coordinator
	if processID == repository.CoordinatorID {
		return p.generateCoordinatorMCPConfig()
	}
	return p.generateWorkerMCPConfig(processID)
}

// generateCoordinatorMCPConfig generates the coordinator-specific MCP config.
func (p *ProcessRegistrySessionProvider) generateCoordinatorMCPConfig() (string, error) {
	if p.coordinatorClient == nil {
		return mcp.GenerateCoordinatorConfigHTTP(p.port)
	}
	switch p.coordinatorClient.Type() {
	case client.ClientAmp:
		return mcp.GenerateCoordinatorConfigAmp(p.port)
	case client.ClientCodex:
		return mcp.GenerateCoordinatorConfigCodex(p.port), nil
	case client.ClientGemini:
		return mcp.GenerateCoordinatorConfigGemini(p.port)
	case client.ClientOpenCode:
		return mcp.GenerateCoordinatorConfigOpenCode(p.port)
	default:
		return mcp.GenerateCoordinatorConfigHTTP(p.port)
	}
}

// generateWorkerMCPConfig generates the worker-specific MCP config.
func (p *ProcessRegistrySessionProvider) generateWorkerMCPConfig(workerID string) (string, error) {
	if p.workerClient == nil {
		return mcp.GenerateWorkerConfigHTTP(p.port, workerID)
	}
	switch p.workerClient.Type() {
	case client.ClientAmp:
		return mcp.GenerateWorkerConfigAmp(p.port, workerID)
	case client.ClientCodex:
		return mcp.GenerateWorkerConfigCodex(p.port, workerID), nil
	case client.ClientGemini:
		return mcp.GenerateWorkerConfigGemini(p.port, workerID)
	case client.ClientOpenCode:
		return mcp.GenerateWorkerConfigOpenCode(p.port, workerID)
	default:
		return mcp.GenerateWorkerConfigHTTP(p.port, workerID)
	}
}

// GetWorkDir returns the working directory for processes.
func (p *ProcessRegistrySessionProvider) GetWorkDir() string {
	return p.workDir
}
