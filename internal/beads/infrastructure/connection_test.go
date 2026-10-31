package infrastructure

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

const serverMetadata = `{"backend":"dolt","dolt_mode":"server"}`

// resolveKind resolves beadsDir without bd and returns the unsupported kind,
// "embedded" for an embedded workspace, or "server" when the workspace
// resolves to a connection.
func resolveKind(t *testing.T, beadsDir string) string {
	t.Helper()
	conn, err := ResolveConnection(beadsDir, fallbackOpts)
	if err != nil {
		if _, ok := errors.AsType[*EmbeddedWorkspaceError](err); ok {
			return doltModeEmbedded
		}
		var uw *UnsupportedWorkspaceError
		require.True(t, errors.As(err, &uw), "unexpected error: %v", err)
		return string(uw.Kind)
	}
	require.NotNil(t, conn)
	return doltModeServer
}

func TestResolveConnection_Mode(t *testing.T) {
	const (
		server   = doltModeServer
		embedded = doltModeEmbedded
		proxied  = string(WorkspaceProxiedServer)
	)
	tests := []struct {
		name     string
		metadata string
		config   string // .beads/config.yaml
		user     string // ~/.config/bd/config.yaml
		env      map[string]string
		want     string
	}{
		{name: "metadata server", metadata: serverMetadata, want: server},
		{name: "metadata server, mixed case", metadata: `{"backend":"dolt","dolt_mode":"Server"}`, want: server},
		{name: "metadata embedded", metadata: `{"backend":"dolt","dolt_mode":"embedded"}`, want: embedded},
		{name: "no dolt_mode defaults to embedded", metadata: `{"backend":"dolt"}`, want: embedded},
		{name: "empty backend is dolt", metadata: `{"dolt_mode":"server"}`, want: server},
		{name: "proxied-server", metadata: `{"backend":"dolt","dolt_mode":"proxied-server"}`, want: proxied},
		{
			name: "proxied-server wins over BEADS_DOLT_SERVER_MODE", metadata: `{"dolt_mode":"proxied-server"}`,
			env: map[string]string{"BEADS_DOLT_SERVER_MODE": "1"}, want: proxied,
		},
		{
			name: "BEADS_DOLT_SERVER_MODE=1 overrides embedded", metadata: `{"dolt_mode":"embedded"}`,
			env: map[string]string{"BEADS_DOLT_SERVER_MODE": "1"}, want: server,
		},
		{
			name: "BEADS_DOLT_SERVER_MODE must be 1", metadata: `{"dolt_mode":"embedded"}`,
			env: map[string]string{"BEADS_DOLT_SERVER_MODE": "true"}, want: embedded,
		},
		{
			name: "BEADS_DOLT_SHARED_SERVER overrides embedded", metadata: `{"dolt_mode":"embedded"}`,
			env: map[string]string{"BEADS_DOLT_SHARED_SERVER": "TRUE"}, want: server,
		},
		{
			name: "config.yaml shared-server overrides embedded metadata", metadata: `{"dolt_mode":"embedded"}`,
			config: "dolt:\n  shared-server: true\n", want: server,
		},
		{
			name: "BD_DOLT_SHARED_SERVER", metadata: `{"dolt_mode":"embedded"}`,
			env: map[string]string{"BD_DOLT_SHARED_SERVER": "1"}, want: server,
		},
		{name: "config.yaml nested dolt.mode without metadata", config: "dolt:\n  mode: server\n", want: server},
		{name: "config.yaml flat dolt.mode without metadata", config: "dolt.mode: server\n", want: server},
		{name: "user config dolt.mode", metadata: `{"backend":"dolt"}`, user: "dolt:\n  mode: server\n", want: server},
		{
			name: "metadata dolt_mode wins over config.yaml", metadata: `{"dolt_mode":"embedded"}`,
			config: "dolt.mode: server\n", want: embedded,
		},
		{name: "remote metadata host implies server", metadata: `{"dolt_server_host":"db.example.com"}`, want: server},
		{
			name: "explicit embedded suppresses metadata host", metadata: `{"dolt_mode":"embedded","dolt_server_host":"db.example.com"}`,
			want: embedded,
		},
		{
			name: "remote env host overrides explicit embedded", metadata: `{"dolt_mode":"embedded"}`,
			env: map[string]string{"BEADS_DOLT_SERVER_HOST": "10.0.0.5"}, want: server,
		},
		{
			name: "local env host suppresses remote metadata host", metadata: `{"dolt_server_host":"db.example.com"}`,
			env: map[string]string{"BEADS_DOLT_SERVER_HOST": "localhost"}, want: embedded,
		},
		{name: "remote config.yaml host implies server", metadata: `{}`, config: "dolt:\n  host: db.example.com\n", want: server},
		{
			name: "config.yaml mode suppresses its host", metadata: `{}`,
			config: "dolt:\n  mode: embedded\n  host: db.example.com\n", want: embedded,
		},
		{name: "IPv6 loopback host is local", metadata: `{"dolt_server_host":"::1"}`, want: embedded},
		{name: "bracketed IPv6 loopback is local", metadata: `{"dolt_server_host":"[::1]"}`, want: embedded},
		{name: "other IPv4 loopback implies server", metadata: `{"dolt_server_host":"127.0.0.2"}`, want: server},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := isolateBeadsEnv(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			beadsDir := newBeadsDir(t)
			if tt.metadata != "" {
				writeMetadata(t, beadsDir, tt.metadata)
			}
			if tt.config != "" {
				writeFile(t, beadsDir, "config.yaml", tt.config)
			}
			if tt.user != "" {
				writeFile(t, home, ".config/bd/config.yaml", tt.user)
			}

			require.Equal(t, tt.want, resolveKind(t, beadsDir))
		})
	}
}

func TestResolveConnection_NoWorkspace(t *testing.T) {
	isolateBeadsEnv(t)
	called := false
	opts := ResolveOptions{BDContext: func(context.Context, string, string) ([]byte, error) {
		called = true
		return nil, errors.New("unused")
	}}

	_, err := ResolveConnection(filepath.Join(t.TempDir(), ".beads"), opts)
	require.ErrorIs(t, err, ErrWorkspaceNotFound, "missing directory")

	_, err = ResolveConnection(newBeadsDir(t), opts)
	require.ErrorIs(t, err, ErrWorkspaceNotFound, "empty directory")

	require.False(t, called, "bd must not run without a workspace")

	// An empty .beads still connects when the environment selects a server.
	t.Setenv("BEADS_DOLT_SERVER_MODE", "1")
	t.Setenv("BEADS_DOLT_SERVER_PORT", "3310")
	conn, err := ResolveConnection(newBeadsDir(t), fallbackOpts)
	require.NoError(t, err)
	require.Equal(t, 3310, conn.Port)
}

func TestResolveConnection_UnsupportedBeforeBD(t *testing.T) {
	isolateBeadsEnv(t)
	beadsDir := newBeadsDir(t)
	writeMetadata(t, beadsDir, `{"backend":"http"}`)
	called := false
	opts := ResolveOptions{BDContext: func(context.Context, string, string) ([]byte, error) {
		called = true
		return []byte(sampleBDContext), nil
	}}

	_, err := ResolveConnection(beadsDir, opts)
	var uw *UnsupportedWorkspaceError
	require.ErrorAs(t, err, &uw)
	require.Equal(t, WorkspaceHTTPBackend, uw.Kind)
	require.False(t, called)
}

func TestResolveConnection_InvalidMetadata(t *testing.T) {
	isolateBeadsEnv(t)
	beadsDir := newBeadsDir(t)
	writeMetadata(t, beadsDir, `{broken`)

	_, err := ResolveConnection(beadsDir, fallbackOpts)
	require.ErrorContains(t, err, "parsing metadata.json")

	writeFile(t, beadsDir, "graph-preview-format", "")
	_, err = ResolveConnection(beadsDir, fallbackOpts)
	var uw *UnsupportedWorkspaceError
	require.ErrorAs(t, err, &uw)
	require.Equal(t, WorkspaceGraph, uw.Kind)
}

func TestResolveConnection_Port(t *testing.T) {
	tests := []struct {
		name     string
		metadata string
		config   string
		user     string
		files    map[string]string // relative to .beads
		shared   map[string]string // relative to the shared-server dir
		env      map[string]string
		wantPort int
		wantSrc  PortSource
	}{
		{name: "nothing names a port for a local server", metadata: serverMetadata, wantPort: 0, wantSrc: PortSourceNone},
		{
			name: "env beats port file", metadata: serverMetadata,
			files: map[string]string{"dolt-server.port": "50362"},
			env:   map[string]string{"BEADS_DOLT_SERVER_PORT": "4000"}, wantPort: 4000, wantSrc: PortSourceEnv,
		},
		{
			name: "port file beats every config source", metadata: `{"dolt_mode":"server","dolt_server_port":3999}`,
			config: "dolt.port: 3998\n",
			files: map[string]string{
				"dolt-server.port": " 50362\n",
				"dolt/config.yaml": "listener:\n  port: 3997\n",
			},
			wantPort: 50362, wantSrc: PortSourcePortFile,
		},
		{
			name: "dolt listener.port beats config.yaml", metadata: `{"dolt_mode":"server","dolt_server_port":3999}`,
			config: "dolt.port: 3998\n", files: map[string]string{"dolt/config.yaml": "listener:\n  port: 3997\n"},
			wantPort: 3997, wantSrc: PortSourceDoltConfig,
		},
		{
			name: "config.yaml dolt.port beats metadata", metadata: `{"dolt_mode":"server","dolt_server_port":3999}`,
			config: "dolt:\n  port: 3998\n", wantPort: 3998, wantSrc: PortSourceConfigYAML,
		},
		{
			name: "user config dolt.port", metadata: serverMetadata,
			user: "dolt.port: 3996\n", wantPort: 3996, wantSrc: PortSourceConfigYAML,
		},
		{
			name: "metadata dolt_server_port last", metadata: `{"dolt_mode":"server","dolt_server_port":3999}`,
			wantPort: 3999, wantSrc: PortSourceMetadata,
		},
		{
			name: "invalid port file ignored", metadata: `{"dolt_mode":"server","dolt_server_port":3999}`,
			files: map[string]string{"dolt-server.port": "not-a-port"}, wantPort: 3999, wantSrc: PortSourceMetadata,
		},
		{
			name: "BEADS_DOLT_DATA_DIR listener.port", metadata: serverMetadata,
			files: map[string]string{"custom/config.yaml": "listener:\n  port: 3995\n"},
			env:   map[string]string{"BEADS_DOLT_DATA_DIR": "custom"}, wantPort: 3995, wantSrc: PortSourceDoltConfig,
		},
		{
			name: "metadata dolt_data_dir listener.port", metadata: `{"dolt_mode":"server","dolt_data_dir":"data"}`,
			files:    map[string]string{"data/config.yaml": "listener:\n  port: 3994\n", "dolt/config.yaml": "listener:\n  port: 1\n"},
			wantPort: 3994, wantSrc: PortSourceDoltConfig,
		},
		{
			name: "port file ignored for a remote host", metadata: `{"dolt_mode":"server","dolt_server_host":"db.example.com","dolt_server_port":3999}`,
			files: map[string]string{"dolt-server.port": "50362"}, wantPort: 3999, wantSrc: PortSourceMetadata,
		},
		{
			name: "remote host defaults to 3307", metadata: `{"dolt_server_host":"db.example.com"}`,
			files: map[string]string{"dolt-server.port": "50362"}, wantPort: 3307, wantSrc: PortSourceExternalDefault,
		},
		{
			name: "legacy BEADS_DOLT_PORT replaces the port file", metadata: serverMetadata,
			files: map[string]string{"dolt-server.port": "50362"},
			env:   map[string]string{"BEADS_DOLT_PORT": "4100"}, wantPort: 4100, wantSrc: PortSourceLegacyEnv,
		},
		{
			name: "legacy BEADS_DOLT_PORT fills an unresolved port", metadata: serverMetadata,
			env: map[string]string{"BEADS_DOLT_PORT": "4100"}, wantPort: 4100, wantSrc: PortSourceLegacyEnv,
		},
		{
			name: "legacy BEADS_DOLT_PORT never replaces config.yaml dolt.port", metadata: serverMetadata,
			config: "dolt.port: 3998\n",
			env:    map[string]string{"BEADS_DOLT_PORT": "4100"}, wantPort: 3998, wantSrc: PortSourceConfigYAML,
		},
		{
			name: "legacy BEADS_DOLT_PORT for a remote host", metadata: `{"dolt_server_host":"db.example.com"}`,
			env: map[string]string{"BEADS_DOLT_PORT": "4100"}, wantPort: 4100, wantSrc: PortSourceLegacyEnv,
		},
		{
			name: "shared server port file", metadata: serverMetadata, config: "dolt:\n  shared-server: true\n",
			files:  map[string]string{"dolt-server.port": "50362"},
			shared: map[string]string{"dolt-server.port": "3412"}, wantPort: 3412, wantSrc: PortSourcePortFile,
		},
		{
			name: "shared server listener.port", metadata: serverMetadata,
			env:    map[string]string{"BEADS_DOLT_SHARED_SERVER": "1"},
			shared: map[string]string{"dolt/config.yaml": "listener:\n  port: 3413\n"}, wantPort: 3413, wantSrc: PortSourceDoltConfig,
		},
		{
			name: "shared server default 3308", metadata: serverMetadata,
			env: map[string]string{"BEADS_DOLT_SHARED_SERVER": "1"}, files: map[string]string{"dolt-server.port": "50362"},
			wantPort: 3308, wantSrc: PortSourceSharedDefault,
		},
		{
			name: "shared server ignores the workspace's metadata port", metadata: `{"dolt_mode":"server","dolt_server_port":3999}`,
			env: map[string]string{"BEADS_DOLT_SHARED_SERVER": "1"}, wantPort: 3308, wantSrc: PortSourceSharedDefault,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := isolateBeadsEnv(t)
			sharedDir := filepath.Join(t.TempDir(), "shared-server")
			t.Setenv("BEADS_SHARED_SERVER_DIR", sharedDir)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			beadsDir := newBeadsDir(t)
			writeMetadata(t, beadsDir, tt.metadata)
			if tt.config != "" {
				writeFile(t, beadsDir, "config.yaml", tt.config)
			}
			if tt.user != "" {
				writeFile(t, home, ".config/bd/config.yaml", tt.user)
			}
			for name, content := range tt.files {
				writeFile(t, beadsDir, name, content)
			}
			for name, content := range tt.shared {
				writeFile(t, sharedDir, name, content)
			}

			conn, err := ResolveConnection(beadsDir, fallbackOpts)
			require.NoError(t, err)
			require.Equal(t, tt.wantPort, conn.Port)
			require.Equal(t, tt.wantSrc, conn.PortSource)
		})
	}
}

func TestResolveConnection_SharedServerDefaultsToHome(t *testing.T) {
	home := isolateBeadsEnv(t)
	t.Setenv("BEADS_DOLT_SHARED_SERVER", "1")
	writeFile(t, home, ".beads/shared-server/dolt-server.port", "3555")
	beadsDir := newBeadsDir(t)
	writeMetadata(t, beadsDir, serverMetadata)

	conn, err := ResolveConnection(beadsDir, fallbackOpts)
	require.NoError(t, err)
	require.Equal(t, 3555, conn.Port)
	require.True(t, conn.SharedServer)
	require.True(t, conn.ManagedByBD())
}

func TestResolveConnection_Fields(t *testing.T) {
	tests := []struct {
		name     string
		metadata string
		config   string
		env      map[string]string
		check    func(t *testing.T, c *Connection)
	}{
		{
			name: "defaults", metadata: serverMetadata,
			check: func(t *testing.T, c *Connection) {
				require.Equal(t, "127.0.0.1", c.Host)
				require.Equal(t, "root", c.User)
				require.Equal(t, "beads", c.Database)
				require.Empty(t, c.Socket)
				require.False(t, c.TLS)
				require.False(t, c.SharedServer)
				require.False(t, c.FromBDContext)
			},
		},
		{
			name:     "metadata values",
			metadata: `{"dolt_mode":"server","dolt_server_host":"db.example.com","dolt_server_user":"alice","dolt_database":"proj","dolt_server_socket":"/run/dolt.sock","dolt_server_tls":true}`,
			check: func(t *testing.T, c *Connection) {
				require.Equal(t, "db.example.com", c.Host)
				require.Equal(t, "alice", c.User)
				require.Equal(t, "proj", c.Database)
				require.Equal(t, "/run/dolt.sock", c.Socket)
				require.True(t, c.TLS)
			},
		},
		{
			name:     "environment overrides metadata",
			metadata: `{"dolt_mode":"server","dolt_server_host":"db.example.com","dolt_server_user":"alice","dolt_database":"proj","dolt_server_socket":"/run/dolt.sock","dolt_server_tls":true}`,
			env: map[string]string{
				"BEADS_DOLT_SERVER_HOST": "10.1.1.1", "BEADS_DOLT_SERVER_USER": "bob",
				"BEADS_DOLT_SERVER_DATABASE": "envdb", "BEADS_DOLT_SERVER_SOCKET": "/tmp/env.sock",
				"BEADS_DOLT_SERVER_TLS": "0",
			},
			check: func(t *testing.T, c *Connection) {
				require.Equal(t, "10.1.1.1", c.Host)
				require.Equal(t, "bob", c.User)
				require.Equal(t, "envdb", c.Database)
				require.Equal(t, "/tmp/env.sock", c.Socket)
				require.False(t, c.TLS)
			},
		},
		{
			name: "TLS from env", metadata: serverMetadata, env: map[string]string{"BEADS_DOLT_SERVER_TLS": "true"},
			check: func(t *testing.T, c *Connection) { require.True(t, c.TLS) },
		},
		{
			name: "config.yaml host", metadata: `{"backend":"dolt"}`, config: "dolt:\n  host: db.internal\n",
			check: func(t *testing.T, c *Connection) {
				require.Equal(t, "db.internal", c.Host)
				require.Equal(t, 3307, c.Port, "a remote server bd does not run defaults to 3307")
				require.Equal(t, PortSourceExternalDefault, c.PortSource)
			},
		},
		{
			// An explicit dolt_mode suppresses host inference, so bd treats the
			// server as its own and names no default port (it would refuse
			// to dial an unresolved port).
			name: "config.yaml host with explicit server mode", metadata: serverMetadata, config: "dolt:\n  host: db.internal\n",
			check: func(t *testing.T, c *Connection) {
				require.Equal(t, "db.internal", c.Host)
				require.Zero(t, c.Port)
			},
		},
		{
			name: "bracketed IPv6 host", metadata: `{"dolt_mode":"server","dolt_server_host":"[::1]","dolt_server_port":3307}`,
			check: func(t *testing.T, c *Connection) {
				require.Equal(t, "[::1]:3307", c.Addr())
				require.Equal(t, "::1", c.DisplayHost())
				require.False(t, c.ManagedByBD(), "bd's server never listens on ::1")
			},
		},
		{
			name: "bare IPv6 host", metadata: `{"dolt_mode":"server","dolt_server_host":"::1","dolt_server_port":3307}`,
			check: func(t *testing.T, c *Connection) { require.Equal(t, "[::1]:3307", c.Addr()) },
		},
		{
			name: "password from env", metadata: serverMetadata, env: map[string]string{"BEADS_DOLT_PASSWORD": "s3cret"},
			check: func(t *testing.T, c *Connection) { require.Equal(t, "s3cret", c.Password) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateBeadsEnv(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			beadsDir := newBeadsDir(t)
			writeMetadata(t, beadsDir, tt.metadata)
			if tt.config != "" {
				writeFile(t, beadsDir, "config.yaml", tt.config)
			}

			conn, err := ResolveConnection(beadsDir, fallbackOpts)
			require.NoError(t, err)
			tt.check(t, conn)
		})
	}
}

func TestResolveConnection_PasswordUsesFinalPort(t *testing.T) {
	isolateBeadsEnv(t)
	t.Setenv("BEADS_CREDENTIALS_FILE", writeFile(t, t.TempDir(), "credentials",
		"[127.0.0.1:3307]\npassword=wrong\n[127.0.0.1:50362]\npassword=right\n"))
	beadsDir := newBeadsDir(t)
	writeMetadata(t, beadsDir, serverMetadata)
	writeFile(t, beadsDir, "dolt-server.port", "50362")

	conn, err := ResolveConnection(beadsDir, fallbackOpts)
	require.NoError(t, err)
	require.Equal(t, "right", conn.Password)

	conn, err = ResolveConnection(beadsDir, ResolveOptions{BDContext: bdContextJSON(
		`{"backend":"dolt","dolt_mode":"server","server_host":"127.0.0.1","server_port":3307,"database":"beads","bd_version":"1.3.2"}`)})
	require.NoError(t, err)
	require.Equal(t, "wrong", conn.Password, "the credentials entry follows the port bd context reports")
}

func TestResolveConnection_BDContext(t *testing.T) {
	tests := []struct {
		name     string
		metadata string
		config   string
		files    map[string]string
		ctx      string
		wantKind WorkspaceKind // "" when a connection is expected
		// wantEmbedded is the expected classification, BeadsDir aside.
		wantEmbedded *EmbeddedWorkspaceError
		check        func(t *testing.T, c *Connection)
	}{
		{
			name: "bd context supplies host, port and database", metadata: serverMetadata,
			files: map[string]string{"dolt-server.port": "1111"},
			ctx:   sampleBDContext,
			check: func(t *testing.T, c *Connection) {
				require.True(t, c.FromBDContext)
				require.Equal(t, "127.0.0.1", c.Host)
				require.Equal(t, 50362, c.Port)
				require.Equal(t, PortSourceBDContext, c.PortSource)
				require.Equal(t, "perles", c.Database)
			},
		},
		{
			name: "enveloped bd context", metadata: serverMetadata,
			ctx: `{"schema_version":1,"data":{"backend":"dolt","dolt_mode":"server","server_host":"db.example.com","server_port":3320,"database":"team","bd_version":"1.1.0"}}`,
			check: func(t *testing.T, c *Connection) {
				require.Equal(t, "db.example.com:3320", c.Addr())
				require.Equal(t, "team", c.Database)
			},
		},
		{
			name: "bd context without a port falls back to the port chain", metadata: serverMetadata,
			files: map[string]string{"dolt-server.port": "50999"},
			ctx:   `{"backend":"dolt","dolt_mode":"server","server_host":"127.0.0.1","database":"beads","bd_version":"1.3.2"}`,
			check: func(t *testing.T, c *Connection) {
				require.Equal(t, 50999, c.Port)
				require.Equal(t, PortSourcePortFile, c.PortSource)
				require.True(t, c.FromBDContext)
			},
		},
		{
			name:     "bd context socket, TLS and user come from metadata",
			metadata: `{"dolt_mode":"server","dolt_server_socket":"/run/dolt.sock","dolt_server_tls":true,"dolt_server_user":"alice"}`,
			ctx:      sampleBDContext,
			check: func(t *testing.T, c *Connection) {
				require.Equal(t, "/run/dolt.sock", c.Socket)
				require.True(t, c.TLS)
				require.Equal(t, "alice", c.User)
			},
		},
		{
			name: "bd context embedded wins over local server guess", metadata: serverMetadata,
			ctx:          `{"backend":"dolt","dolt_mode":"embedded","database":"beads","bd_version":"1.3.2"}`,
			wantEmbedded: &EmbeddedWorkspaceError{Database: "beads", BDVersion: "1.3.2"},
		},
		{
			name: "bd context server wins over embedded metadata", metadata: `{"dolt_mode":"embedded"}`,
			ctx: sampleBDContext,
			check: func(t *testing.T, c *Connection) {
				require.Equal(t, 50362, c.Port)
			},
		},
		{
			name: "bd context proxied-server", metadata: serverMetadata,
			ctx:      `{"backend":"dolt","dolt_mode":"proxied-server","proxied_dir":"/p/.beads/dolt","database":"beads","bd_version":"1.3.2"}`,
			wantKind: WorkspaceProxiedServer,
		},
		{
			name: "bd context http backend", metadata: serverMetadata,
			ctx:      `{"backend":"http","dolt_mode":"","database":"","bd_version":"1.3.2"}`,
			wantKind: WorkspaceHTTPBackend,
		},
		{
			name: "bd context embedded but config.yaml enables the shared server", metadata: `{"dolt_mode":"embedded"}`,
			config: "dolt:\n  shared-server: true\n",
			ctx:    `{"backend":"dolt","dolt_mode":"embedded","database":"beads","bd_version":"1.3.2"}`,
			check: func(t *testing.T, c *Connection) {
				require.True(t, c.SharedServer)
				require.Equal(t, 3308, c.Port)
			},
		},
		{
			name: "old bd context ignored", metadata: serverMetadata,
			files: map[string]string{"dolt-server.port": "50362"},
			ctx:   `{"backend":"dolt","dolt_mode":"server","server_host":"127.0.0.1","server_port":3307,"database":"beads","bd_version":"0.60.0"}`,
			check: func(t *testing.T, c *Connection) {
				require.False(t, c.FromBDContext)
				require.Equal(t, 50362, c.Port)
			},
		},
		{
			name: "bd context error falls back", metadata: serverMetadata,
			files: map[string]string{"dolt-server.port": "50362"},
			ctx:   `{"error":"cannot resolve repo context"}`,
			check: func(t *testing.T, c *Connection) {
				require.False(t, c.FromBDContext)
				require.Equal(t, 50362, c.Port)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateBeadsEnv(t)
			t.Setenv("BEADS_SHARED_SERVER_DIR", filepath.Join(t.TempDir(), "shared"))
			beadsDir := newBeadsDir(t)
			writeMetadata(t, beadsDir, tt.metadata)
			if tt.config != "" {
				writeFile(t, beadsDir, "config.yaml", tt.config)
			}
			for name, content := range tt.files {
				writeFile(t, beadsDir, name, content)
			}

			conn, err := ResolveConnection(beadsDir, ResolveOptions{WorkDir: "/work", BDContext: bdContextJSON(tt.ctx)})
			if tt.wantEmbedded != nil {
				ew, ok := errors.AsType[*EmbeddedWorkspaceError](err)
				require.True(t, ok, "got %v", err)
				want := *tt.wantEmbedded
				want.BeadsDir = beadsDir
				require.Equal(t, &want, ew)
				return
			}
			if tt.wantKind != "" {
				var uw *UnsupportedWorkspaceError
				require.ErrorAs(t, err, &uw)
				require.Equal(t, tt.wantKind, uw.Kind)
				return
			}
			require.NoError(t, err)
			tt.check(t, conn)
		})
	}
}

func TestConnection_DSN(t *testing.T) {
	tests := []struct {
		name     string
		conn     Connection
		socket   string
		wantNet  string
		wantAddr string
		wantTLS  string
	}{
		{
			name: "tcp", conn: Connection{Host: "127.0.0.1", Port: 50362, User: "root", Database: "perles"},
			wantNet: "tcp", wantAddr: "127.0.0.1:50362", wantTLS: "false",
		},
		{
			name: "tcp with password and TLS", conn: Connection{Host: "db.example.com", Port: 3307, User: "alice", Password: "p@ss:w/rd", Database: "team", TLS: true},
			wantNet: "tcp", wantAddr: "db.example.com:3307", wantTLS: "true",
		},
		{
			name: "IPv6", conn: Connection{Host: "[::1]", Port: 3307, User: "root", Database: "beads"},
			wantNet: "tcp", wantAddr: "[::1]:3307", wantTLS: "false",
		},
		{
			name: "socket", conn: Connection{Host: "127.0.0.1", Port: 3307, Socket: "/run/dolt.sock", User: "root", Database: "beads"},
			socket: "/run/dolt.sock", wantNet: "unix", wantAddr: "/run/dolt.sock", wantTLS: "false",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := mysql.ParseDSN(tt.conn.dsn(tt.socket))
			require.NoError(t, err)
			require.Equal(t, tt.wantNet, cfg.Net)
			require.Equal(t, tt.wantAddr, cfg.Addr)
			require.Equal(t, tt.conn.User, cfg.User)
			require.Equal(t, tt.conn.Password, cfg.Passwd)
			require.Equal(t, tt.conn.Database, cfg.DBName)
			require.Equal(t, tt.wantTLS, cfg.TLSConfig)
			require.True(t, cfg.ParseTime)
		})
	}
}

func TestConnection_LocationAndManaged(t *testing.T) {
	require.Equal(t, "127.0.0.1:3307", Connection{Host: "127.0.0.1", Port: 3307}.Location())
	require.Equal(t, "unix:/run/d.sock", Connection{Host: "127.0.0.1", Port: 3307, Socket: "/run/d.sock"}.Location())
	require.Equal(t, "127.0.0.1 (no port found)", Connection{Host: "127.0.0.1"}.Location())
	require.False(t, Connection{Host: "127.0.0.1"}.Dialable())
	require.True(t, Connection{Socket: "/s"}.Dialable())

	require.True(t, Connection{Host: "localhost"}.ManagedByBD())
	require.True(t, Connection{Host: "db.example.com", SharedServer: true}.ManagedByBD())
	require.False(t, Connection{Host: "db.example.com"}.ManagedByBD())
	require.False(t, Connection{Host: "127.0.0.1", Socket: "/s"}.ManagedByBD())
}

func TestClassifyHost(t *testing.T) {
	tests := map[string]hostKind{
		"":                 hostBdServer,
		"localhost":        hostBdServer,
		"LOCALHOST":        hostBdServer,
		"127.0.0.1":        hostBdServer,
		" 127.0.0.1 ":      hostBdServer,
		"0.0.0.0":          hostBdServer,
		"::ffff:127.0.0.1": hostBdServer,
		"::1":              hostIPv6Loopback,
		"[::1]":            hostIPv6Loopback,
		"::":               hostIPv6Loopback,
		"127.0.0.2":        hostOtherLoopback,
		"10.0.0.5":         hostRemote,
		"db.example.com":   hostRemote,
		"fe80::1%en0":      hostRemote,
	}
	for host, want := range tests {
		require.Equal(t, want, classifyHost(host), "host %q", host)
	}
}
