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

func TestGenerateCoordinatorConfigGemini(t *testing.T) {
	configJSON, err := GenerateCoordinatorConfigGemini(9000)
	require.NoError(t, err, "GenerateCoordinatorConfigGemini failed")

	var cfg MCPConfig
	require.NoError(t, json.Unmarshal([]byte(configJSON), &cfg), "Failed to parse config")

	server := cfg.MCPServers["perles-orchestrator"]
	// Gemini uses httpUrl, not url or type
	require.Empty(t, server.Type, "Type should be empty for Gemini")
	require.Empty(t, server.URL, "URL should be empty for Gemini")
	require.Equal(t, "http://localhost:9000/mcp", server.HTTPUrl, "HTTPUrl mismatch")
}

func TestGenerateWorkerConfigGemini(t *testing.T) {
	configJSON, err := GenerateWorkerConfigGemini(9000, "WORKER.1")
	require.NoError(t, err, "GenerateWorkerConfigGemini failed")

	var cfg MCPConfig
	require.NoError(t, json.Unmarshal([]byte(configJSON), &cfg), "Failed to parse config")

	server := cfg.MCPServers["perles-worker"]
	// Gemini uses httpUrl, not url or type
	require.Empty(t, server.Type, "Type should be empty for Gemini")
	require.Empty(t, server.URL, "URL should be empty for Gemini")
	require.Equal(t, "http://localhost:9000/worker/WORKER.1", server.HTTPUrl, "HTTPUrl mismatch")
}

func TestGenerateWorkerConfigCodex(t *testing.T) {
	t.Run("returns correct TOML syntax", func(t *testing.T) {
		result := GenerateWorkerConfigCodex(8765, "WORKER.1")

		// Verify TOML syntax structure
		require.Contains(t, result, "mcp_servers.perles-worker=", "Missing mcp_servers prefix")
		require.Contains(t, result, `{url="`, "Missing inline table syntax")
		require.Contains(t, result, `"}`, "Missing closing brace and quote")
	})

	t.Run("port and workerID are correctly interpolated", func(t *testing.T) {
		result := GenerateWorkerConfigCodex(9000, "worker-5")

		expected := `mcp_servers.perles-worker={url="http://localhost:9000/worker/worker-5"}`
		require.Equal(t, expected, result, "Config string mismatch")
	})

	t.Run("URL format matches expected MCP HTTP transport format", func(t *testing.T) {
		result := GenerateWorkerConfigCodex(8080, "WORKER.test")

		// Verify URL components
		require.Contains(t, result, "http://localhost:8080", "Port not interpolated correctly")
		require.Contains(t, result, "/worker/WORKER.test", "WorkerID not interpolated correctly")
	})

	t.Run("handles different port values", func(t *testing.T) {
		testCases := []struct {
			port     int
			workerID string
			expected string
		}{
			{8765, "WORKER.1", `mcp_servers.perles-worker={url="http://localhost:8765/worker/WORKER.1"}`},
			{9999, "worker-99", `mcp_servers.perles-worker={url="http://localhost:9999/worker/worker-99"}`},
			{1234, "test", `mcp_servers.perles-worker={url="http://localhost:1234/worker/test"}`},
		}

		for _, tc := range testCases {
			result := GenerateWorkerConfigCodex(tc.port, tc.workerID)
			require.Equal(t, tc.expected, result, "Mismatch for port=%d, workerID=%s", tc.port, tc.workerID)
		}
	})
}

func TestGenerateCoordinatorConfigOpenCode(t *testing.T) {
	t.Run("returns valid JSON", func(t *testing.T) {
		configJSON, err := GenerateCoordinatorConfigOpenCode(9000)
		require.NoError(t, err, "GenerateCoordinatorConfigOpenCode failed")

		var parsed map[string]any
		require.NoError(t, json.Unmarshal([]byte(configJSON), &parsed), "Output should be valid JSON")
	})

	t.Run("output format matches expected OpenCode structure", func(t *testing.T) {
		configJSON, err := GenerateCoordinatorConfigOpenCode(9000)
		require.NoError(t, err, "GenerateCoordinatorConfigOpenCode failed")

		// Parse and verify structure: {"permission": "allow", "mcp": {"perles-orchestrator": {"type": "remote", "url": "..."}}}
		var parsed map[string]any
		require.NoError(t, json.Unmarshal([]byte(configJSON), &parsed), "Failed to parse JSON")

		// Check "permission" contains wildcard allow-all for headless mode
		perm, ok := parsed["permission"].(map[string]any)
		require.True(t, ok, "Permission should be an object for headless mode")
		starPerm, ok := perm["*"].(map[string]any)
		require.True(t, ok, "Permission should have '*' wildcard key")
		require.Equal(t, "allow", starPerm["*"], "Wildcard permission should be 'allow'")

		// Check "mcp" key exists
		mcp, ok := parsed["mcp"].(map[string]any)
		require.True(t, ok, "Missing 'mcp' wrapper key")

		// Check "perles-orchestrator" server exists
		server, ok := mcp["perles-orchestrator"].(map[string]any)
		require.True(t, ok, "Missing 'perles-orchestrator' server in mcp config")

		// Check "type" is "remote"
		require.Equal(t, "remote", server["type"], "Type should be 'remote'")

		// Check "url" is correctly formatted
		expectedURL := "http://localhost:9000/mcp"
		require.Equal(t, expectedURL, server["url"], "URL mismatch")
	})

	t.Run("handles different port values", func(t *testing.T) {
		testCases := []struct {
			port     int
			expected string
		}{
			{8765, "http://localhost:8765/mcp"},
			{9000, "http://localhost:9000/mcp"},
			{1234, "http://localhost:1234/mcp"},
		}

		for _, tc := range testCases {
			configJSON, err := GenerateCoordinatorConfigOpenCode(tc.port)
			require.NoError(t, err, "GenerateCoordinatorConfigOpenCode failed for port=%d", tc.port)
			require.Contains(t, configJSON, tc.expected, "URL mismatch for port=%d", tc.port)
		}
	})
}

func TestGenerateWorkerConfigOpenCode(t *testing.T) {
	t.Run("returns valid JSON", func(t *testing.T) {
		configJSON, err := GenerateWorkerConfigOpenCode(9000, "WORKER.1")
		require.NoError(t, err, "GenerateWorkerConfigOpenCode failed")

		// Verify it's valid JSON by unmarshaling
		var parsed map[string]any
		require.NoError(t, json.Unmarshal([]byte(configJSON), &parsed), "Output is not valid JSON")
	})

	t.Run("output format matches expected OpenCode structure", func(t *testing.T) {
		configJSON, err := GenerateWorkerConfigOpenCode(9000, "WORKER.1")
		require.NoError(t, err, "GenerateWorkerConfigOpenCode failed")

		// Parse and verify structure: {"permission": {...}, "mcp": {"perles-worker": {"type": "remote", "url": "..."}}}
		var parsed map[string]any
		require.NoError(t, json.Unmarshal([]byte(configJSON), &parsed), "Failed to parse JSON")

		// Check "permission" contains wildcard allow-all for headless mode
		perm, ok := parsed["permission"].(map[string]any)
		require.True(t, ok, "Permission should be an object for headless mode")
		starPerm, ok := perm["*"].(map[string]any)
		require.True(t, ok, "Permission should have '*' wildcard key")
		require.Equal(t, "allow", starPerm["*"], "Wildcard permission should be 'allow'")

		// Check "mcp" key exists
		mcp, ok := parsed["mcp"].(map[string]any)
		require.True(t, ok, "Missing 'mcp' wrapper key")

		// Check "perles-worker" server exists
		server, ok := mcp["perles-worker"].(map[string]any)
		require.True(t, ok, "Missing 'perles-worker' server in mcp config")

		// Check "type" is "remote"
		require.Equal(t, "remote", server["type"], "Type should be 'remote'")

		// Check "url" is correctly formatted
		expectedURL := "http://localhost:9000/worker/WORKER.1"
		require.Equal(t, expectedURL, server["url"], "URL mismatch")
	})

	t.Run("serverURL is correctly embedded", func(t *testing.T) {
		configJSON, err := GenerateWorkerConfigOpenCode(8765, "worker-5")
		require.NoError(t, err, "GenerateWorkerConfigOpenCode failed")

		require.Contains(t, configJSON, "http://localhost:8765/worker/worker-5", "URL not correctly embedded")
	})

	t.Run("workerID is correctly handled in URL", func(t *testing.T) {
		testCases := []struct {
			port     int
			workerID string
			expected string
		}{
			{8765, "WORKER.1", "http://localhost:8765/worker/WORKER.1"},
			{9000, "worker-99", "http://localhost:9000/worker/worker-99"},
			{1234, "test-worker", "http://localhost:1234/worker/test-worker"},
		}

		for _, tc := range testCases {
			configJSON, err := GenerateWorkerConfigOpenCode(tc.port, tc.workerID)
			require.NoError(t, err, "GenerateWorkerConfigOpenCode failed for workerID=%s", tc.workerID)
			require.Contains(t, configJSON, tc.expected, "URL mismatch for workerID=%s", tc.workerID)
		}
	})
}
