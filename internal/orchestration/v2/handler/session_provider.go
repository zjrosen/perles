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
// Supports both coordinator and worker processes.
type ProcessRegistrySessionProvider struct {
	registry *process.ProcessRegistry
	client   client.HeadlessClient
	workDir  string
	port     int
}

// NewProcessRegistrySessionProvider creates a new ProcessRegistrySessionProvider.
//
// Parameters:
//   - registry: ProcessRegistry for looking up process sessions
//   - aiClient: HeadlessClient for determining MCP config format (Amp vs HTTP)
//   - workDir: Working directory for processes
//   - port: MCP server port for process connections
func NewProcessRegistrySessionProvider(
	registry *process.ProcessRegistry,
	aiClient client.HeadlessClient,
	workDir string,
	port int,
) *ProcessRegistrySessionProvider {
	return &ProcessRegistrySessionProvider{
		registry: registry,
		client:   aiClient,
		workDir:  workDir,
		port:     port,
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
	if p.client == nil {
		return mcp.GenerateCoordinatorConfigHTTP(p.port)
	}
	switch p.client.Type() {
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
	if p.client == nil {
		return mcp.GenerateWorkerConfigHTTP(p.port, workerID)
	}
	switch p.client.Type() {
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
