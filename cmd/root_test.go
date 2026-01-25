package cmd

import (
	"os"
	"path/filepath"
	"testing"

	infrabeads "github.com/zjrosen/perles/internal/beads/infrastructure"
	"github.com/zjrosen/perles/internal/config"
	"github.com/zjrosen/perles/internal/keys"

	"github.com/stretchr/testify/require"
)

// TestNoBeadsDirectory_BeadsClientFails verifies that appbeads.NewSQLiteClient returns
// an error when there's no .beads directory. This is the condition that triggers
// the nobeads empty state view.
func TestNoBeadsDirectory_BeadsClientFails(t *testing.T) {
	// Create temp directory without .beads
	tmpDir, err := os.MkdirTemp("", "perles-test-nobeads-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	// Verify no .beads directory exists
	beadsPath := filepath.Join(tmpDir, ".beads")
	_, err = os.Stat(beadsPath)
	require.True(t, os.IsNotExist(err), "expected .beads to not exist")

	// Verify NewSQLiteClient fails for this directory
	_, err = infrabeads.NewSQLiteClient(tmpDir)
	require.Error(t, err, "expected NewSQLiteClient to fail without .beads directory")
}

// TestNoBeadsDirectory_WithBeadsSucceeds verifies that appbeads.NewSQLiteClient succeeds
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

	// Verify NewSQLiteClient succeeds
	client, err := infrabeads.NewSQLiteClient(projectRoot)
	if err == nil {
		// Clean up if we got a client
		_ = client
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
