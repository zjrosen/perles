package cmd

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/config"
)

// TestInitCommand_NoConfigAnywhere_CreatesConfig runs `perles init` through
// rootCmd, so the cobra.OnInitialize(initConfig) hook runs first. The hook used
// to auto-create .perles/config.yaml, which made init always fail with
// "config file already exists".
func TestInitCommand_NoConfigAnywhere_CreatesConfig(t *testing.T) {
	isolateConfig(t)

	out, err := executeRoot(t, "init")

	require.NoError(t, err)
	require.Contains(t, out, "Created "+defaultConfigPath)
	data, err := os.ReadFile(defaultConfigPath)
	require.NoError(t, err)
	require.Equal(t, config.DefaultConfigTemplate(), string(data))
}

func TestInitCommand_ConfigExists_Refuses(t *testing.T) {
	isolateConfig(t)
	require.NoError(t, os.MkdirAll(".perles", 0o750))
	require.NoError(t, os.WriteFile(defaultConfigPath, []byte("ui:\n  show_counts: false\n"), 0o600))

	_, err := executeRoot(t, "init")

	require.Error(t, err)
	require.Contains(t, err.Error(), "config file already exists")
	data, readErr := os.ReadFile(defaultConfigPath)
	require.NoError(t, readErr)
	require.Equal(t, "ui:\n  show_counts: false\n", string(data), "existing config must be left untouched")
}

func TestInitCommand_RunTwice_SecondRunRefuses(t *testing.T) {
	isolateConfig(t)

	_, err := executeRoot(t, "init")
	require.NoError(t, err)

	_, err = executeRoot(t, "init")
	require.Error(t, err)
	require.Contains(t, err.Error(), "config file already exists")
}
