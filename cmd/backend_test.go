package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/config"
	"github.com/zjrosen/perles/internal/pubsub"
	"github.com/zjrosen/perles/internal/watcher"
)

// TestNewBackend_EmbeddedWorkspace verifies that newBackend, which perles
// daemon calls before anything else, opens an embedded Dolt workspace
// instead of failing with "creating backend": it returns the snapshot
// backend, which runs no bd until the first query and publishes its own
// change events instead of a file to watch.
func TestNewBackend_EmbeddedWorkspace(t *testing.T) {
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "BEADS_") || strings.HasPrefix(name, "BD_") {
			t.Setenv(name, "")
		}
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("PATH", "") // nothing may reach a real bd

	workDir := t.TempDir()
	beadsDir := filepath.Join(workDir, ".beads")
	manifest := filepath.Join(beadsDir, "embeddeddolt", "beads", ".dolt", "noms", "manifest")
	require.NoError(t, os.MkdirAll(filepath.Dir(manifest), 0o750))
	require.NoError(t, os.WriteFile(manifest, []byte("manifest"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(beadsDir, "metadata.json"),
		[]byte(`{"backend":"dolt","dolt_mode":"embedded","dolt_database":"beads"}`), 0o600))

	cfg := config.Defaults()
	cfg.ResolvedBeadsDir = beadsDir
	backend, err := newBackend(&cfg, workDir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, backend.Close()) })

	require.Empty(t, backend.Services().DBPath)
	_, notifies := backend.(interface {
		Changes() *pubsub.Broker[watcher.WatcherEvent]
	})
	require.True(t, notifies, "the app listens to the backend's changes")
}
