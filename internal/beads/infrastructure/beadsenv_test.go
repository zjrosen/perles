package infrastructure

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadBeadsEnvFile_ImportsOnlyUnsetBeadsKeys(t *testing.T) {
	beadsDir := t.TempDir()
	content := "BEADS_DOLT_SERVER_USER=beads\n" +
		"export BEADS_DOLT_PASSWORD=\"s3cret\"\n" +
		"BEADS_DOLT_SERVER_PORT=4000\n" +
		"BD_ACTOR=agent\n" +
		"BEADS_DIR=/elsewhere\n" +
		"ANTHROPIC_API_KEY=do-not-import\n"
	require.NoError(t, os.WriteFile(filepath.Join(beadsDir, ".env"), []byte(content), 0o600))

	// Already set: must not be overridden.
	t.Setenv("BEADS_DOLT_SERVER_PORT", "5000")
	// Cleared up front so t.Setenv restores the originals after the test.
	for _, key := range []string{"BEADS_DOLT_SERVER_USER", "BEADS_DOLT_PASSWORD", "BD_ACTOR", "BEADS_DIR", "ANTHROPIC_API_KEY"} {
		t.Setenv(key, "")
		require.NoError(t, os.Unsetenv(key))
	}

	set := loadBeadsEnvFile(beadsDir)
	sort.Strings(set)

	require.Equal(t, []string{"BD_ACTOR", "BEADS_DOLT_PASSWORD", "BEADS_DOLT_SERVER_USER"}, set)
	require.Equal(t, "beads", os.Getenv("BEADS_DOLT_SERVER_USER"))
	require.Equal(t, "s3cret", os.Getenv("BEADS_DOLT_PASSWORD"))
	require.Equal(t, "5000", os.Getenv("BEADS_DOLT_SERVER_PORT"))
	_, hasDir := os.LookupEnv("BEADS_DIR")
	require.False(t, hasDir, "BEADS_DIR must not be imported")
	_, hasKey := os.LookupEnv("ANTHROPIC_API_KEY")
	require.False(t, hasKey, "non-beads keys must not be imported")
}

func TestLoadBeadsEnvFile_NoFile(t *testing.T) {
	require.Empty(t, loadBeadsEnvFile(t.TempDir()))
	require.Empty(t, loadBeadsEnvFile(""))
}
