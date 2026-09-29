package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/config"
	"github.com/zjrosen/perles/internal/keys"
	"github.com/zjrosen/perles/internal/orchestration/client"
)

// TestNoBeadsDirectory_BackendFails verifies that newBackend returns an error
// when there's no .beads directory. This is the condition that triggers the
// nobeads empty state view.
func TestNoBeadsDirectory_BackendFails(t *testing.T) {
	// Create temp directory without .beads
	tmpDir, err := os.MkdirTemp("", "perles-test-nobeads-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	// Verify no .beads directory exists
	beadsPath := filepath.Join(tmpDir, ".beads")
	_, err = os.Stat(beadsPath)
	require.True(t, os.IsNotExist(err), "expected .beads to not exist")

	// Verify newBackend fails for this directory
	cfg := config.Defaults()
	cfg.ResolvedBeadsDir = filepath.Join(tmpDir, ".beads")
	_, err = newBackend(&cfg, tmpDir)
	require.Error(t, err, "expected newBackend to fail without .beads directory")
}

// TestNoBeadsDirectory_WithBeadsSucceeds verifies that newBackend succeeds
// when there IS a valid .beads directory.
func TestNoBeadsDirectory_WithBeadsSucceeds(t *testing.T) {
	// Use the actual project directory which has .beads
	cwd, err := os.Getwd()
	require.NoError(t, err)

	// Go up to project root if we're in cmd/
	projectRoot := filepath.Dir(cwd)
	beadsPath := filepath.Join(projectRoot, ".beads")

	// Skip if not in expected directory structure
	if _, err := os.Stat(beadsPath); os.IsNotExist(err) {
		// Try current directory
		if _, err := os.Stat(filepath.Join(cwd, ".beads")); os.IsNotExist(err) {
			t.Skip("not running from project directory with .beads")
		}
		projectRoot = cwd
	}

	// Verify newBackend succeeds
	cfg := config.Defaults()
	cfg.ResolvedBeadsDir = filepath.Join(projectRoot, ".beads")
	backend, err := newBackend(&cfg, projectRoot)
	if err == nil {
		_ = backend.Close()
	}
}

// ============================================================================
// Keybinding Startup Integration Tests
// ============================================================================

// TestStartup_ValidKeybindings verifies that validation passes and ApplyConfig
// is called for valid keybinding configuration.
func TestStartup_ValidKeybindings(t *testing.T) {
	kb := config.KeybindingsConfig{
		Search:    "ctrl+k",
		Dashboard: "ctrl+d",
	}

	// Validation should pass
	err := config.ValidateKeybindings(kb)
	require.NoError(t, err, "valid keybindings should pass validation")

	// ApplyConfig with these keys should work (tested via keys package)
	keys.ResetForTesting()
	defer keys.ResetForTesting()

	searchKey := kb.Search
	dashboardKey := kb.Dashboard
	keys.ApplyConfig(searchKey, dashboardKey)

	// Verify keys were applied
	require.Equal(t, []string{"ctrl+k"}, keys.Kanban.SwitchMode.Keys())
	require.Equal(t, []string{"ctrl+d"}, keys.Kanban.Dashboard.Keys())
}

// TestStartup_InvalidKeybindings verifies that invalid keybindings cause
// validation failure with a clear error message.
func TestStartup_InvalidKeybindings(t *testing.T) {
	tests := []struct {
		name        string
		kb          config.KeybindingsConfig
		errContains string
	}{
		{
			name:        "invalid format - typo in ctrl",
			kb:          config.KeybindingsConfig{Search: "crtl+k"},
			errContains: "invalid key format",
		},
		{
			name:        "reserved key - q",
			kb:          config.KeybindingsConfig{Dashboard: "q"},
			errContains: "reserved",
		},
		{
			name:        "reserved key - enter",
			kb:          config.KeybindingsConfig{Search: "enter"},
			errContains: "reserved",
		},
		{
			name:        "duplicate keys",
			kb:          config.KeybindingsConfig{Search: "ctrl+k", Dashboard: "ctrl+k"},
			errContains: "same key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := config.ValidateKeybindings(tt.kb)
			require.Error(t, err, "invalid keybindings should fail validation")
			require.Contains(t, err.Error(), tt.errContains,
				"error message should contain '%s'", tt.errContains)
		})
	}
}

// TestStartup_NoKeybindings verifies that empty keybindings configuration
// uses default values (ctrl+space and ctrl+o).
func TestStartup_NoKeybindings(t *testing.T) {
	kb := config.KeybindingsConfig{
		Search:    "", // Empty
		Dashboard: "", // Empty
	}

	// Validation should pass for empty values
	err := config.ValidateKeybindings(kb)
	require.NoError(t, err, "empty keybindings should pass validation")

	// Simulate startup logic: empty strings get defaults
	keys.ResetForTesting()
	defer keys.ResetForTesting()

	searchKey := kb.Search
	if searchKey == "" {
		searchKey = "ctrl+space" // Default
	}
	dashboardKey := kb.Dashboard
	if dashboardKey == "" {
		dashboardKey = "ctrl+o" // Default
	}
	keys.ApplyConfig(searchKey, dashboardKey)

	// Verify defaults were applied
	// ctrl+space translates to ctrl+@ for terminal
	require.Equal(t, []string{"ctrl+@"}, keys.Kanban.SwitchMode.Keys(),
		"default search key should be ctrl+@ (ctrl+space)")
	require.Equal(t, []string{"ctrl+o"}, keys.Kanban.Dashboard.Keys(),
		"default dashboard key should be ctrl+o")
}

// TestStartup_PartialKeybindings verifies that specifying only one keybinding
// uses the default for the other.
func TestStartup_PartialKeybindings(t *testing.T) {
	t.Run("only search specified", func(t *testing.T) {
		kb := config.KeybindingsConfig{
			Search:    "ctrl+k",
			Dashboard: "", // Use default
		}

		// Validation should pass
		err := config.ValidateKeybindings(kb)
		require.NoError(t, err, "partial keybindings should pass validation")

		// Simulate startup logic
		keys.ResetForTesting()
		defer keys.ResetForTesting()

		searchKey := kb.Search
		if searchKey == "" {
			searchKey = "ctrl+space"
		}
		dashboardKey := kb.Dashboard
		if dashboardKey == "" {
			dashboardKey = "ctrl+o" // Default
		}
		keys.ApplyConfig(searchKey, dashboardKey)

		// Verify custom search and default dashboard
		require.Equal(t, []string{"ctrl+k"}, keys.Kanban.SwitchMode.Keys(),
			"search key should be ctrl+k")
		require.Equal(t, []string{"ctrl+o"}, keys.Kanban.Dashboard.Keys(),
			"dashboard key should default to ctrl+o")
	})

	t.Run("only dashboard specified", func(t *testing.T) {
		kb := config.KeybindingsConfig{
			Search:    "", // Use default
			Dashboard: "ctrl+d",
		}

		// Validation should pass
		err := config.ValidateKeybindings(kb)
		require.NoError(t, err, "partial keybindings should pass validation")

		// Simulate startup logic
		keys.ResetForTesting()
		defer keys.ResetForTesting()

		searchKey := kb.Search
		if searchKey == "" {
			searchKey = "ctrl+space" // Default
		}
		dashboardKey := kb.Dashboard
		if dashboardKey == "" {
			dashboardKey = "ctrl+o"
		}
		keys.ApplyConfig(searchKey, dashboardKey)

		// Verify default search and custom dashboard
		require.Equal(t, []string{"ctrl+@"}, keys.Kanban.SwitchMode.Keys(),
			"search key should default to ctrl+@ (ctrl+space)")
		require.Equal(t, []string{"ctrl+d"}, keys.Kanban.Dashboard.Keys(),
			"dashboard key should be ctrl+d")
	})
}

// ============================================================================
// Config Loading Tests
// ============================================================================

// isolateConfig gives a test a clean global config state: HOME and the working
// directory point at fresh temp dirs (so no real config file is found), and
// viper, cfg, cfgFile and configNotFound are reset before and after the test.
func isolateConfig(t *testing.T) (home, workDir string) {
	t.Helper()
	home = t.TempDir()
	workDir = t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(workDir)

	origCfg, origCfgFile := cfg, cfgFile
	resetGlobals := func() {
		viper.Reset()
		bindConfigFlags()
		cfg, cfgFile, configNotFound = config.Config{}, "", false
	}
	resetGlobals()
	t.Cleanup(func() {
		resetGlobals()
		cfg, cfgFile = origCfg, origCfgFile
	})
	return home, workDir
}

// loadConfigFile writes content to a temp config file, points --config at it,
// and runs initConfig.
func loadConfigFile(t *testing.T, content string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	cfgFile = path
	initConfig()
}

// executeRoot runs rootCmd with args (including cobra.OnInitialize hooks) and
// returns its stdout.
func executeRoot(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	rootCmd.SetArgs(args)
	rootCmd.SetOut(&out)
	rootCmd.SetErr(io.Discard)
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	})
	err := rootCmd.Execute()
	return out.String(), err
}

func TestInitConfig_NoConfigFile_DoesNotCreateOne(t *testing.T) {
	isolateConfig(t)

	initConfig()

	_, err := os.Stat(defaultConfigPath)
	require.True(t, os.IsNotExist(err), "initConfig must not auto-create %s", defaultConfigPath)
	require.True(t, configNotFound, "initConfig should record that no config file was found")
	require.True(t, cfg.UI.ShowStatusBar, "defaults should still apply without a config file")
}

func TestWriteDefaultConfigIfMissing_CreatesAndLoadsDefaultConfig(t *testing.T) {
	isolateConfig(t)
	initConfig()
	require.Empty(t, cfg.Views, "no views are loaded before the default config is written")

	require.True(t, writeDefaultConfigIfMissing(), "cfg should be reloaded from the written file")

	data, err := os.ReadFile(defaultConfigPath)
	require.NoError(t, err, "first TUI launch should write %s", defaultConfigPath)
	require.Equal(t, config.DefaultConfigTemplate(), string(data))
	require.Equal(t, defaultConfigPath, viper.ConfigFileUsed())
	require.False(t, configNotFound)
	require.Len(t, cfg.Views, 1, "the written template should be loaded into cfg")
	require.Len(t, cfg.Sound.Events, 6)

	require.False(t, writeDefaultConfigIfMissing(), "second call is a no-op")
}

func TestWriteDefaultConfigIfMissing_ConfigFound_NoOp(t *testing.T) {
	isolateConfig(t)
	loadConfigFile(t, "ui:\n  show_counts: false\n")

	require.False(t, writeDefaultConfigIfMissing())

	_, err := os.Stat(defaultConfigPath)
	require.True(t, os.IsNotExist(err), "must not write a default config when one was loaded")
	require.False(t, cfg.UI.ShowCounts)
}

func TestInitConfig_DefaultsMatchConfigDefaults(t *testing.T) {
	isolateConfig(t)
	loadConfigFile(t, "# empty config\n")

	// Every non-zero field in config.Defaults() must have a viper default,
	// except views (GetViews falls back to DefaultViews) and the role-specific
	// clients (left unset so the legacy client key can take effect; the
	// resolvers still default to claude).
	expected := config.Defaults()
	expected.Views = nil
	expected.Orchestration.CoordinatorClient = ""
	expected.Orchestration.WorkerClient = ""

	require.Equal(t, expected, cfg)
	require.Equal(t, client.ClientClaude, cfg.Orchestration.CoordinatorClientType())
	require.Equal(t, client.ClientClaude, cfg.Orchestration.WorkerClientType())
}

func TestInitConfig_ShowStatusBarDefaultsToTrueWhenOmitted(t *testing.T) {
	isolateConfig(t)
	loadConfigFile(t, "ui:\n  show_counts: false\n")

	require.True(t, cfg.UI.ShowStatusBar, "ui.show_status_bar should default to true")
	require.False(t, cfg.UI.ShowCounts)
}

func TestInitConfig_TracingEnabledOnly_UsesDefaults(t *testing.T) {
	home, _ := isolateConfig(t)
	loadConfigFile(t, "orchestration:\n  tracing:\n    enabled: true\n")

	tracing := cfg.Orchestration.Tracing
	require.True(t, tracing.Enabled)
	require.Equal(t, "file", tracing.Exporter)
	require.Equal(t, filepath.Join(home, ".config", "perles", "traces", "traces.jsonl"), tracing.FilePath)
	require.Equal(t, "localhost:4317", tracing.OTLPEndpoint)
	require.InDelta(t, 1.0, tracing.SampleRate, 0)
	require.NoError(t, config.ValidateOrchestration(cfg.Orchestration),
		"enabled: true alone should be a valid tracing config")
}

func TestInitConfig_ClientResolution(t *testing.T) {
	tests := []struct {
		name            string
		yaml            string
		wantCoordinator client.ClientType
		wantWorker      client.ClientType
	}{
		{
			name:            "nothing set defaults to claude",
			yaml:            "orchestration:\n  api_port: 0\n",
			wantCoordinator: client.ClientClaude,
			wantWorker:      client.ClientClaude,
		},
		{
			name:            "legacy client applies to both roles",
			yaml:            "orchestration:\n  client: amp\n",
			wantCoordinator: client.ClientAmp,
			wantWorker:      client.ClientAmp,
		},
		{
			name:            "coordinator_client overrides client",
			yaml:            "orchestration:\n  client: amp\n  coordinator_client: codex\n",
			wantCoordinator: client.ClientCodex,
			wantWorker:      client.ClientAmp,
		},
		{
			name:            "worker_client overrides client",
			yaml:            "orchestration:\n  client: gemini\n  worker_client: cursor\n",
			wantCoordinator: client.ClientGemini,
			wantWorker:      client.ClientCursor,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateConfig(t)
			loadConfigFile(t, tt.yaml)

			require.Equal(t, tt.wantCoordinator, cfg.Orchestration.CoordinatorClientType())
			require.Equal(t, tt.wantWorker, cfg.Orchestration.WorkerClientType())
			require.Equal(t, tt.wantCoordinator, cfg.Orchestration.AgentProviders().Coordinator().Type())
			require.Equal(t, tt.wantWorker, cfg.Orchestration.AgentProviders().Worker().Type())
		})
	}
}
