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
	Type    string            `json:"type,omitempty"`    // "http" for HTTP transport (Claude)
	URL     string            `json:"url,omitempty"`     // URL for HTTP transport (Claude) or SSE (Gemini)
	HTTPUrl string            `json:"httpUrl,omitempty"` // URL for streamable HTTP transport (Gemini)
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
// This format is used by Claude CLI which expects {"type": "http", "url": "..."}.
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

// GenerateCoordinatorConfigGemini creates an MCP config for Gemini CLI.
// Gemini CLI uses "httpUrl" for streamable HTTP transport (not "url" which is SSE).
func GenerateCoordinatorConfigGemini(port int) (string, error) {
	config := MCPConfig{
		MCPServers: map[string]MCPServerConfig{
			"perles-orchestrator": {
				HTTPUrl: fmt.Sprintf("http://localhost:%d/mcp", port),
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

// GenerateCoordinatorConfigCodex creates an MCP config for Codex CLI.
// Codex expects TOML syntax for the -c flag: mcp_servers.perles-orchestrator={url="http://localhost:PORT/mcp"}
func GenerateCoordinatorConfigCodex(port int) string {
	return fmt.Sprintf(`mcp_servers.perles-orchestrator={url="http://localhost:%d/mcp"}`, port)
}

// GenerateWorkerConfigHTTP creates an MCP config for a worker that connects to the
// shared HTTP MCP server. This allows workers to share the same message store as
// the coordinator, solving the in-memory cache isolation problem in prompt mode.
// This format is used by Claude CLI which expects {"type": "http", "url": "..."}.
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

// GenerateWorkerConfigGemini creates an MCP config for a worker using Gemini CLI format.
// Gemini CLI uses "httpUrl" for streamable HTTP transport.
func GenerateWorkerConfigGemini(port int, workerID string) (string, error) {
	config := MCPConfig{
		MCPServers: map[string]MCPServerConfig{
			"perles-worker": {
				HTTPUrl: fmt.Sprintf("http://localhost:%d/worker/%s", port, workerID),
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

// GenerateWorkerConfigCodex creates an MCP config for a worker using Codex CLI format.
// Codex expects TOML syntax for the -c flag: mcp_servers.perles-worker={url="http://localhost:PORT/worker/ID"}
func GenerateWorkerConfigCodex(port int, workerID string) string {
	return fmt.Sprintf(`mcp_servers.perles-worker={url="http://localhost:%d/worker/%s"}`, port, workerID)
}

// GenerateCoordinatorConfigOpenCode creates an MCP config for the coordinator using OpenCode format.
// OpenCode expects {"mcp": {"serverName": {"type": "remote", "url": "..."}}} in opencode.jsonc.
// Includes permission overrides for doom_loop and external_directory to auto-approve in headless mode,
// preventing interactive prompts that would block automation.
func GenerateCoordinatorConfigOpenCode(port int) (string, error) {
	config := map[string]any{
		"permission": map[string]any{
			"*": map[string]any{
				"*": "allow",
			},
		},
		"mcp": map[string]any{
			"perles-orchestrator": map[string]any{
				"type": "remote",
				"url":  fmt.Sprintf("http://localhost:%d/mcp", port),
			},
		},
	}

	data, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("marshaling config: %w", err)
	}

	return string(data), nil
}

// GenerateWorkerConfigOpenCode creates an MCP config for a worker using OpenCode format.
// OpenCode expects {"mcp": {"serverName": {"type": "remote", "url": "..."}}} in opencode.jsonc.
// Includes permission overrides for doom_loop and external_directory to auto-approve in headless mode,
// preventing interactive prompts that would block automation.
func GenerateWorkerConfigOpenCode(port int, workerID string) (string, error) {
	config := map[string]any{
		"permission": map[string]any{
			"*": map[string]any{
				"*": "allow",
			},
		},
		"mcp": map[string]any{
			"perles-worker": map[string]any{
				"type": "remote",
				"url":  fmt.Sprintf("http://localhost:%d/worker/%s", port, workerID),
			},
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
