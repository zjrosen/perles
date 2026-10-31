package infrastructure

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadMetadata_Absent(t *testing.T) {
	meta, err := LoadMetadata(newBeadsDir(t))
	require.NoError(t, err)
	require.Nil(t, meta, "no metadata must not default to any backend")
}

func TestLoadMetadata_Fields(t *testing.T) {
	beadsDir := newBeadsDir(t)
	writeMetadata(t, beadsDir, `{
		"database": "dolt", "backend": "dolt", "dolt_mode": "server",
		"dolt_database": "perles", "dolt_server_host": "db.example.com",
		"dolt_server_port": 3310, "dolt_server_socket": "/tmp/dolt.sock",
		"dolt_server_user": "alice", "dolt_server_tls": true,
		"dolt_data_dir": "/data/dolt", "graph_mode": "dependency",
		"project_id": "ignored"
	}`)

	meta, err := LoadMetadata(beadsDir)
	require.NoError(t, err)
	require.Equal(t, &BeadsMetadata{
		Database: "dolt", Backend: "dolt", GraphMode: "dependency", DoltMode: "server",
		DoltDatabase: "perles", DoltServerHost: "db.example.com", DoltServerPort: 3310,
		DoltServerSocket: "/tmp/dolt.sock", DoltServerUser: "alice", DoltServerTLS: true,
		DoltDataDir: "/data/dolt",
	}, meta)
}

func TestLoadMetadata_LegacyConfigJSONReadInPlace(t *testing.T) {
	beadsDir := newBeadsDir(t)
	writeFile(t, beadsDir, "config.json", `{"backend": "dolt", "dolt_mode": "server"}`)

	meta, err := LoadMetadata(beadsDir)
	require.NoError(t, err)
	require.Equal(t, "server", meta.DoltMode)

	require.FileExists(t, filepath.Join(beadsDir, "config.json"), "legacy config.json must not be migrated")
	require.NoFileExists(t, filepath.Join(beadsDir, "metadata.json"))
}

func TestLoadMetadata_InvalidJSON(t *testing.T) {
	beadsDir := newBeadsDir(t)
	writeMetadata(t, beadsDir, `not valid json`)

	_, err := LoadMetadata(beadsDir)
	require.ErrorContains(t, err, "parsing metadata.json")
}

func TestBeadsYAML_NestedFlatAndMixedKeys(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"nested", "dolt:\n  mode: server\n  port: 3311\n  shared-server: true\n"},
		{"flat", "dolt.mode: server\ndolt.port: 3311\ndolt.shared-server: true\n"},
		{"quoted flat", "\"dolt.mode\": server\n\"dolt.port\": \"3311\"\n\"dolt.shared-server\": \"true\"\n"},
		{"upper case", "Dolt:\n  Mode: server\n  PORT: 3311\n  Shared-Server: TRUE\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateBeadsEnv(t)
			beadsDir := newBeadsDir(t)
			writeFile(t, beadsDir, "config.yaml", tt.content)

			y := loadBeadsYAML(beadsDir)
			require.Equal(t, "server", y.get("dolt.mode"))
			require.Equal(t, "3311", y.get("dolt.port"))
			require.True(t, y.getBool("dolt.shared-server"))
			require.Empty(t, y.get("dolt.host"))
			require.False(t, y.getBool("dolt.missing"))
		})
	}
}

func TestBeadsYAML_LayerPrecedence(t *testing.T) {
	home := isolateBeadsEnv(t)
	beadsDir := newBeadsDir(t)

	writeFile(t, home, ".beads/config.yaml", "dolt:\n  mode: legacy-user\n  host: legacy-host\n  user: legacy-user\n")
	writeFile(t, home, ".config/bd/config.yaml", "dolt:\n  mode: user\n  host: user-host\n")
	writeFile(t, beadsDir, "config.yaml", "dolt:\n  mode: project\n")

	y := loadBeadsYAML(beadsDir)
	require.Equal(t, "project", y.get("dolt.mode"), "project config.yaml wins")
	require.Equal(t, "user-host", y.get("dolt.host"), "~/.config/bd wins over ~/.beads")
	require.Equal(t, "legacy-user", y.get("dolt.user"), "~/.beads/config.yaml is the last fallback")

	writeFile(t, beadsDir, "config.local.yaml", "dolt.mode: local\n")
	require.Equal(t, "local", loadBeadsYAML(beadsDir).get("dolt.mode"), "config.local.yaml overrides config.yaml")

	t.Setenv("BD_DOLT_MODE", "env")
	require.Equal(t, "env", loadBeadsYAML(beadsDir).get("dolt.mode"), "BD_* environment wins")
}

func TestBeadsYAML_LocalConfigNeedsConfigYAML(t *testing.T) {
	isolateBeadsEnv(t)
	beadsDir := newBeadsDir(t)
	writeFile(t, beadsDir, "config.local.yaml", "dolt.mode: server\n")

	require.Empty(t, loadBeadsYAML(beadsDir).get("dolt.mode"))
}

func TestBeadsYAML_UnparseableFileSkipped(t *testing.T) {
	home := isolateBeadsEnv(t)
	beadsDir := newBeadsDir(t)
	writeFile(t, beadsDir, "config.yaml", "dolt: [unclosed\n")
	writeFile(t, home, ".config/bd/config.yaml", "dolt.mode: server\n")

	require.Equal(t, "server", loadBeadsYAML(beadsDir).get("dolt.mode"))
}

func TestBDEnvKey(t *testing.T) {
	require.Equal(t, "BD_DOLT_SHARED_SERVER", bdEnvKey("dolt.shared-server"))
	require.Equal(t, "BD_DOLT_MODE", bdEnvKey("dolt.mode"))
}

func TestDoltConfigListenerPort(t *testing.T) {
	doltDir := t.TempDir()
	require.Zero(t, doltConfigListenerPort(doltDir), "no config.yaml")

	writeFile(t, doltDir, "config.yaml", "log_level: warning\n# listener:\n#   port: 1234\n")
	require.Zero(t, doltConfigListenerPort(doltDir), "commented-out port is not a port")

	writeFile(t, doltDir, "config.yaml", "listener:\n  host: 127.0.0.1\n  port: 50362\n")
	require.Equal(t, 50362, doltConfigListenerPort(doltDir))

	require.NoError(t, os.Remove(filepath.Join(doltDir, "config.yaml")))
	require.Zero(t, doltConfigListenerPort(doltDir))
}
