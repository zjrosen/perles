package infrastructure

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// isolateBeadsEnv clears every BEADS_* and BD_* variable and points HOME (and
// the user config dirs) at a fresh temp dir, so tests see only the
// configuration they write. It returns the fake home directory.
func isolateBeadsEnv(t *testing.T) string {
	t.Helper()
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "BEADS_") || strings.HasPrefix(name, "BD_") {
			t.Setenv(name, "")
		}
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("APPDATA", filepath.Join(home, "AppData"))
	return home
}

// newBeadsDir creates an empty .beads directory in a temp project.
func newBeadsDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), ".beads")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	return dir
}

// writeFile writes content to dir/name, creating parent directories.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func writeMetadata(t *testing.T, beadsDir, content string) {
	t.Helper()
	writeFile(t, beadsDir, "metadata.json", content)
}

// noBDContext is a runner for a machine without a usable bd.
func noBDContext(context.Context, string, string) ([]byte, error) {
	return nil, errors.New("bd: executable file not found in $PATH")
}

// bdContextJSON is a runner that prints out.
func bdContextJSON(out string) BDContextRunner {
	return func(context.Context, string, string) ([]byte, error) {
		return []byte(out), nil
	}
}

// fallbackOpts resolves without bd.
var fallbackOpts = ResolveOptions{BDContext: noBDContext}
