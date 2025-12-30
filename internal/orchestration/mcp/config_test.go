package mcp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateWorkerConfig(t *testing.T) {
	// GenerateWorkerConfig now returns HTTP config
	configJSON, err := GenerateWorkerConfig("worker-1", "/work")
	require.NoError(t, err, "GenerateWorkerConfig failed")

	var config MCPConfig
	require.NoError(t, json.Unmarshal([]byte(configJSON), &config), "Failed to parse config JSON")

	server, ok := config.MCPServers["perles-worker"]
	require.True(t, ok, "Missing perles-worker server in config")

	// Check it's HTTP transport
	require.Equal(t, "http", server.Type, "Type should be 'http'")

	// Check URL includes worker ID
	expectedURL := "http://localhost:8765/worker/worker-1"
	require.Equal(t, expectedURL, server.URL, "URL mismatch")
}

func TestGenerateWorkerConfigHTTP(t *testing.T) {
	configJSON, err := GenerateWorkerConfigHTTP(9000, "WORKER.3")
	require.NoError(t, err, "GenerateWorkerConfigHTTP failed")

	var config MCPConfig
	require.NoError(t, json.Unmarshal([]byte(configJSON), &config), "Failed to parse config JSON")

	server, ok := config.MCPServers["perles-worker"]
	require.True(t, ok, "Missing perles-worker server in config")

	require.Equal(t, "http", server.Type, "Type should be 'http'")

	expectedURL := "http://localhost:9000/worker/WORKER.3"
	require.Equal(t, expectedURL, server.URL, "URL mismatch")
}

func TestConfigToFlag(t *testing.T) {
	input := `{"mcpServers":{"test":{"command":"test"}}}`
	result := ConfigToFlag(input)
	require.Equal(t, input, result, "ConfigToFlag mismatch")
}

func TestParseMCPConfig(t *testing.T) {
	input := `{
		"mcpServers": {
			"server1": {
				"command": "/bin/server1",
				"args": ["--flag"],
				"env": {"KEY": "VALUE"}
			},
			"server2": {
				"command": "/bin/server2"
			}
		}
	}`

	config, err := ParseMCPConfig(input)
	require.NoError(t, err, "ParseMCPConfig failed")

	require.Len(t, config.MCPServers, 2, "Server count mismatch")

	server1, ok := config.MCPServers["server1"]
	require.True(t, ok, "Missing server1")
	require.Equal(t, "/bin/server1", server1.Command, "server1.Command mismatch")
	require.Equal(t, []string{"--flag"}, server1.Args, "server1.Args mismatch")
	require.Equal(t, "VALUE", server1.Env["KEY"], "server1.Env[KEY] mismatch")
}

func TestParseMCPConfigInvalid(t *testing.T) {
	_, err := ParseMCPConfig("not valid json")
	require.Error(t, err, "Expected error for invalid JSON")
}

func TestMCPConfigSerialization(t *testing.T) {
	config := MCPConfig{
		MCPServers: map[string]MCPServerConfig{
			"test-server": {
				Command: "/usr/local/bin/mcp-server",
				Args:    []string{"--verbose", "--port", "8080"},
				Env: map[string]string{
					"LOG_LEVEL": "debug",
				},
			},
		},
	}

	data, err := json.Marshal(config)
	require.NoError(t, err, "Marshal failed")

	var parsed MCPConfig
	require.NoError(t, json.Unmarshal(data, &parsed), "Unmarshal failed")

	server, ok := parsed.MCPServers["test-server"]
	require.True(t, ok, "Missing test-server")
	require.Equal(t, config.MCPServers["test-server"].Command, server.Command, "Command mismatch")
}

func TestGenerateCoordinatorConfigHTTP(t *testing.T) {
	configJSON, err := GenerateCoordinatorConfigHTTP(9000)
	require.NoError(t, err, "GenerateCoordinatorConfigHTTP failed")

	var cfg MCPConfig
	require.NoError(t, json.Unmarshal([]byte(configJSON), &cfg), "Failed to parse config")

	server := cfg.MCPServers["perles-orchestrator"]
	require.Equal(t, "http", server.Type, "Type mismatch")
	expectedURL := "http://localhost:9000/mcp"
	require.Equal(t, expectedURL, server.URL, "URL mismatch")
}
