package mcp

import (
	"encoding/json"
	"fmt"
)

// MCPServerConfig represents the configuration for an MCP server.
// This is used to generate the --mcp-config JSON for Claude CLI.
type MCPServerConfig struct {
	Command string            `json:"command,omitempty"` // For stdio transport
	Args    []string          `json:"args,omitempty"`    // For stdio transport
	Env     map[string]string `json:"env,omitempty"`     // For stdio transport
	Type    string            `json:"type,omitempty"`    // "http" for HTTP transport
	URL     string            `json:"url,omitempty"`     // URL for HTTP transport
	Headers map[string]string `json:"headers,omitempty"` // HTTP headers (optional)
}

// MCPConfig represents the full MCP configuration with multiple servers.
// Used for Claude CLI which expects {"mcpServers": {...}}.
type MCPConfig struct {
	MCPServers map[string]MCPServerConfig `json:"mcpServers"`
}

// AmpMCPConfig represents MCP configuration for Amp CLI.
// Amp expects a flat structure: {"serverName": {"url": "..."}} without the mcpServers wrapper.
type AmpMCPConfig map[string]MCPServerConfig

// GenerateCoordinatorConfigHTTP creates an MCP config that connects to an HTTP server.
// The server should be running on localhost at the specified port.
func GenerateCoordinatorConfigHTTP(port int) (string, error) {
	config := MCPConfig{
		MCPServers: map[string]MCPServerConfig{
			"perles-orchestrator": {
				Type: "http",
				URL:  fmt.Sprintf("http://localhost:%d/mcp", port),
			},
		},
	}

	data, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("marshaling config: %w", err)
	}

	return string(data), nil
}

// GenerateCoordinatorConfigAmp creates an MCP config for Amp CLI.
// Amp expects a flat format: {"serverName": {"url": "..."}} without mcpServers wrapper.
func GenerateCoordinatorConfigAmp(port int) (string, error) {
	config := AmpMCPConfig{
		"perles-orchestrator": {
			URL: fmt.Sprintf("http://localhost:%d/mcp", port),
		},
	}

	data, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("marshaling config: %w", err)
	}

	return string(data), nil
}

// GenerateWorkerConfigHTTP creates an MCP config for a worker that connects to the
// shared HTTP MCP server. This allows workers to share the same message store as
// the coordinator, solving the in-memory cache isolation problem in prompt mode.
func GenerateWorkerConfigHTTP(port int, workerID string) (string, error) {
	config := MCPConfig{
		MCPServers: map[string]MCPServerConfig{
			"perles-worker": {
				Type: "http",
				URL:  fmt.Sprintf("http://localhost:%d/worker/%s", port, workerID),
			},
		},
	}

	data, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("marshaling config: %w", err)
	}

	return string(data), nil
}

// GenerateWorkerConfigAmp creates an MCP config for a worker using Amp CLI format.
func GenerateWorkerConfigAmp(port int, workerID string) (string, error) {
	config := AmpMCPConfig{
		"perles-worker": {
			URL: fmt.Sprintf("http://localhost:%d/worker/%s", port, workerID),
		},
	}

	data, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("marshaling config: %w", err)
	}

	return string(data), nil
}

// GenerateWorkerConfig creates the MCP config JSON for a worker agent.
// Workers connect to the shared HTTP MCP server to share the message store
// with the coordinator.
//
// Parameters:
//   - workerID: The worker's ID for identification in messages
//   - workDir: Working directory for the MCP server (unused, kept for API compatibility)
//
// Returns the JSON string suitable for --mcp-config flag.
func GenerateWorkerConfig(workerID, workDir string) (string, error) {
	return GenerateWorkerConfigHTTP(8765, workerID)
}

// ConfigToFlag formats the config as a command line flag value.
// Returns the string suitable for: claude --mcp-config '<result>'
func ConfigToFlag(configJSON string) string {
	return configJSON
}

// ParseMCPConfig parses an MCP config JSON string.
func ParseMCPConfig(configJSON string) (*MCPConfig, error) {
	var config MCPConfig
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return nil, fmt.Errorf("parsing MCP config: %w", err)
	}
	return &config, nil
}
