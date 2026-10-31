package adapter

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	infrabeads "github.com/zjrosen/perles/internal/beads/infrastructure"
	"github.com/zjrosen/perles/internal/task"
)

func TestTranslateClientError_UnsupportedWorkspace(t *testing.T) {
	kinds := map[infrabeads.WorkspaceKind]task.UnsupportedWorkspaceKind{
		infrabeads.WorkspaceEmbeddedDolt:   task.WorkspaceEmbeddedDolt,
		infrabeads.WorkspaceProxiedServer:  task.WorkspaceProxiedServer,
		infrabeads.WorkspaceHTTPBackend:    task.WorkspaceHTTPBackend,
		infrabeads.WorkspaceGraph:          task.WorkspaceGraph,
		infrabeads.WorkspaceLegacySQLite:   task.WorkspaceLegacySQLite,
		infrabeads.WorkspaceUnknownBackend: task.WorkspaceUnknownBackend,
	}
	for infraKind, want := range kinds {
		t.Run(string(infraKind), func(t *testing.T) {
			err := translateClientError(&infrabeads.UnsupportedWorkspaceError{Kind: infraKind, BeadsDir: "/p/.beads", Detail: "d"})

			uw, ok := errors.AsType[*task.UnsupportedWorkspaceError](err)
			require.True(t, ok, "got %v", err)
			require.Equal(t, want, uw.Kind)
			require.Equal(t, "/p/.beads", uw.BeadsDir)
			require.Equal(t, "d", uw.Detail)
		})
	}
}

func TestTranslateClientError_EmbeddedWorkspace(t *testing.T) {
	err := translateClientError(&infrabeads.EmbeddedWorkspaceError{
		BeadsDir:     "/p/.beads",
		Database:     "beads",
		BDVersion:    "1.3.2",
		BDContextErr: errors.New("unused"),
	})

	uw, ok := errors.AsType[*task.UnsupportedWorkspaceError](err)
	require.True(t, ok, "got %v", err)
	require.Equal(t, &task.UnsupportedWorkspaceError{Kind: task.WorkspaceEmbeddedDolt, BeadsDir: "/p/.beads"}, uw,
		"embedded keeps today's unsupported screen: empty Detail")
}

func TestTranslateClientError_ServerDown(t *testing.T) {
	cause := errors.New("connection refused")
	err := translateClientError(&infrabeads.ServerUnreachableError{
		Conn: infrabeads.Connection{Host: "[::1]", Port: 3307},
		Err:  cause,
	})

	sd, ok := errors.AsType[*task.ServerDownError](err)
	require.True(t, ok, "got %v", err)
	require.Equal(t, "::1", sd.Host)
	require.Equal(t, 3307, sd.Port)
	require.Equal(t, "[::1]:3307", sd.Address())
	require.False(t, sd.ManagedByBD)
	require.ErrorIs(t, err, cause)

	err = translateClientError(&infrabeads.ServerUnreachableError{
		Conn: infrabeads.Connection{Host: "127.0.0.1", Socket: "/run/dolt.sock"},
		Err:  cause,
	})
	sd, ok = errors.AsType[*task.ServerDownError](err)
	require.True(t, ok)
	require.Equal(t, "unix:/run/dolt.sock", sd.Address())

	err = translateClientError(&infrabeads.ServerUnreachableError{Conn: infrabeads.Connection{Host: "127.0.0.1"}, Err: cause})
	sd, ok = errors.AsType[*task.ServerDownError](err)
	require.True(t, ok)
	require.True(t, sd.ManagedByBD)
	require.Equal(t, "127.0.0.1", sd.Address())
}

func TestTranslateClientError_Other(t *testing.T) {
	cause := errors.New("parsing metadata.json: boom")
	err := translateClientError(cause)

	require.ErrorIs(t, err, cause)
	_, isServerDown := errors.AsType[*task.ServerDownError](err)
	require.False(t, isServerDown)
	_, isUnsupported := errors.AsType[*task.UnsupportedWorkspaceError](err)
	require.False(t, isUnsupported)
}

func TestNewBeadsBackend_EmbeddedWorkspace(t *testing.T) {
	isolateEnv(t)
	beadsDir := filepath.Join(t.TempDir(), ".beads")
	require.NoError(t, os.MkdirAll(beadsDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(`{"backend":"dolt","dolt_mode":"embedded"}`), 0o600))

	_, err := NewBeadsBackend(beadsDir, filepath.Dir(beadsDir))

	uw, ok := errors.AsType[*task.UnsupportedWorkspaceError](err)
	require.True(t, ok, "got %v", err)
	require.Equal(t, task.WorkspaceEmbeddedDolt, uw.Kind)
	require.Equal(t, beadsDir, uw.BeadsDir)
	require.Empty(t, uw.Detail)
}

func TestNewBeadsBackend_ServerDown(t *testing.T) {
	isolateEnv(t)
	beadsDir := filepath.Join(t.TempDir(), ".beads")
	require.NoError(t, os.MkdirAll(beadsDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(`{"backend":"dolt","dolt_mode":"server"}`), 0o600))

	_, err := NewBeadsBackend(beadsDir, filepath.Dir(beadsDir))

	sd, ok := errors.AsType[*task.ServerDownError](err)
	require.True(t, ok, "got %v", err)
	require.Zero(t, sd.Port, "no port file: nothing is listening")
	require.True(t, sd.ManagedByBD)
}

func TestDoltWatcherConfig(t *testing.T) {
	cfg := doltWatcherConfig()
	require.Equal(t, []string{"last-touched"}, cfg.RelevantFiles)
	require.Equal(t, 500*time.Millisecond, cfg.DebounceDuration)
}

// isolateEnv clears BEADS_* and BD_* variables and redirects HOME so the
// backend sees only the workspace a test writes.
func isolateEnv(t *testing.T) {
	t.Helper()
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "BEADS_") || strings.HasPrefix(name, "BD_") {
			t.Setenv(name, "")
		}
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
}
