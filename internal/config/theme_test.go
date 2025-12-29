package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/ui/styles"
)

// TestThemeConfig_WithPreset tests loading a config file with a preset.
func TestThemeConfig_WithPreset(t *testing.T) {
	configYAML := `
theme:
  preset: catppuccin-mocha
`
	cfg := loadConfigFromYAML(t, configYAML)

	require.Equal(t, "catppuccin-mocha", cfg.Theme.Preset)

	// Apply theme and verify colors changed
	themeCfg := styles.ThemeConfig{
		Preset: cfg.Theme.Preset,
		Mode:   cfg.Theme.Mode,
		Colors: cfg.Theme.Colors,
	}
	err := styles.ApplyTheme(themeCfg)
	require.NoError(t, err)

	// Catppuccin Mocha uses #CDD6F4 for text.primary
	require.Equal(t, "#CDD6F4", styles.TextPrimaryColor.Dark)
}

// TestThemeConfig_WithColorOverrides tests applying color overrides programmatically.
func TestThemeConfig_WithColorOverrides(t *testing.T) {
	cfg := Config{
		Theme: ThemeConfig{
			Colors: map[string]string{
				"text.primary": "#FF0000",
				"status.error": "#00FF00",
			},
		},
	}

	require.NotNil(t, cfg.Theme.Colors)
	require.Equal(t, "#FF0000", cfg.Theme.Colors["text.primary"])
	require.Equal(t, "#00FF00", cfg.Theme.Colors["status.error"])

	// Apply theme and verify colors applied
	themeCfg := styles.ThemeConfig{
		Preset: cfg.Theme.Preset,
		Mode:   cfg.Theme.Mode,
		Colors: cfg.Theme.Colors,
	}
	err := styles.ApplyTheme(themeCfg)
	require.NoError(t, err)

	require.Equal(t, "#FF0000", styles.TextPrimaryColor.Dark)
	require.Equal(t, "#00FF00", styles.StatusErrorColor.Dark)
}

// TestThemeConfig_WithColorOverridesFromYAML tests that dotted color tokens
// in YAML config files are correctly parsed when using custom viper key delimiter.
func TestThemeConfig_WithColorOverridesFromYAML(t *testing.T) {
	configYAML := `
theme:
  colors:
    text.primary: "#FF0000"
    status.error: "#00FF00"
    selection.indicator: "#0000FF"
`
	cfg := loadConfigFromYAML(t, configYAML)

	require.NotNil(t, cfg.Theme.Colors)
	require.Equal(t, "#FF0000", cfg.Theme.Colors["text.primary"])
	require.Equal(t, "#00FF00", cfg.Theme.Colors["status.error"])
	require.Equal(t, "#0000FF", cfg.Theme.Colors["selection.indicator"])

	// Apply theme and verify colors applied
	themeCfg := styles.ThemeConfig{
		Preset: cfg.Theme.Preset,
		Mode:   cfg.Theme.Mode,
		Colors: cfg.Theme.Colors,
	}
	err := styles.ApplyTheme(themeCfg)
	require.NoError(t, err)

	require.Equal(t, "#FF0000", styles.TextPrimaryColor.Dark)
	require.Equal(t, "#00FF00", styles.StatusErrorColor.Dark)
	require.Equal(t, "#0000FF", styles.SelectionIndicatorColor.Dark)
}

// TestThemeConfig_PresetWithOverrides tests that color overrides take precedence over preset.
func TestThemeConfig_PresetWithOverrides(t *testing.T) {
	cfg := Config{
		Theme: ThemeConfig{
			Preset: "dracula",
			Colors: map[string]string{
				"text.primary": "#123456",
			},
		},
	}

	require.Equal(t, "dracula", cfg.Theme.Preset)
	require.Equal(t, "#123456", cfg.Theme.Colors["text.primary"])

	// Apply theme
	themeCfg := styles.ThemeConfig{
		Preset: cfg.Theme.Preset,
		Mode:   cfg.Theme.Mode,
		Colors: cfg.Theme.Colors,
	}
	err := styles.ApplyTheme(themeCfg)
	require.NoError(t, err)

	// Override should take precedence
	require.Equal(t, "#123456", styles.TextPrimaryColor.Dark)
	// Dracula's status error should still be applied (#FF5555)
	require.Equal(t, "#FF5555", styles.StatusErrorColor.Dark)
}

// TestThemeConfig_InvalidPreset tests that invalid preset returns error.
func TestThemeConfig_InvalidPreset(t *testing.T) {
	configYAML := `
theme:
  preset: nonexistent-theme
`
	cfg := loadConfigFromYAML(t, configYAML)

	themeCfg := styles.ThemeConfig{
		Preset: cfg.Theme.Preset,
		Mode:   cfg.Theme.Mode,
		Colors: cfg.Theme.Colors,
	}
	err := styles.ApplyTheme(themeCfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown theme preset")
}

// TestThemeConfig_InvalidColorToken tests that invalid color token returns error.
func TestThemeConfig_InvalidColorToken(t *testing.T) {
	cfg := Config{
		Theme: ThemeConfig{
			Colors: map[string]string{
				"invalid.token.name": "#FF0000",
			},
		},
	}

	themeCfg := styles.ThemeConfig{
		Preset: cfg.Theme.Preset,
		Mode:   cfg.Theme.Mode,
		Colors: cfg.Theme.Colors,
	}
	err := styles.ApplyTheme(themeCfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown color token")
}

// TestThemeConfig_InvalidHexColor tests that invalid hex color returns error.
func TestThemeConfig_InvalidHexColor(t *testing.T) {
	cfg := Config{
		Theme: ThemeConfig{
			Colors: map[string]string{
				"text.primary": "not-a-color",
			},
		},
	}

	themeCfg := styles.ThemeConfig{
		Preset: cfg.Theme.Preset,
		Mode:   cfg.Theme.Mode,
		Colors: cfg.Theme.Colors,
	}
	err := styles.ApplyTheme(themeCfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid hex color")
}

// TestThemeConfig_EmptyConfig tests that empty theme config applies defaults.
func TestThemeConfig_EmptyConfig(t *testing.T) {
	configYAML := `
auto_refresh: true
`
	cfg := loadConfigFromYAML(t, configYAML)

	// Empty theme should result in empty/nil values
	require.Empty(t, cfg.Theme.Preset)
	require.Nil(t, cfg.Theme.Colors)

	// Apply should succeed with default colors
	themeCfg := styles.ThemeConfig{
		Preset: cfg.Theme.Preset,
		Mode:   cfg.Theme.Mode,
		Colors: cfg.Theme.Colors,
	}
	err := styles.ApplyTheme(themeCfg)
	require.NoError(t, err)

	// Default preset should be applied (#CCCCCC for text.primary)
	require.Equal(t, "#CCCCCC", styles.TextPrimaryColor.Dark)
}

// TestThemeConfig_AllPresets tests that all built-in presets load correctly.
func TestThemeConfig_AllPresets(t *testing.T) {
	presets := []string{
		"default",
		"catppuccin-mocha",
		"catppuccin-latte",
		"dracula",
		"nord",
		"high-contrast",
	}

	for _, preset := range presets {
		t.Run(preset, func(t *testing.T) {
			configYAML := `
theme:
  preset: ` + preset + `
`
			if preset == "default" {
				configYAML = `
theme:
  preset: ""
`
			}
			cfg := loadConfigFromYAML(t, configYAML)

			themeCfg := styles.ThemeConfig{
				Preset: cfg.Theme.Preset,
				Mode:   cfg.Theme.Mode,
				Colors: cfg.Theme.Colors,
			}
			err := styles.ApplyTheme(themeCfg)
			require.NoError(t, err, "preset %s should apply without error", preset)
		})
	}
}

// loadConfigFromYAML is a helper to load config from YAML string.
func loadConfigFromYAML(t *testing.T, yaml string) Config {
	t.Helper()

	// Create temp file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte(yaml), 0644)
	require.NoError(t, err)

	// Use custom key delimiter "::" to allow dotted keys like "text.primary"
	// in the theme.colors map without viper treating them as nested paths.
	v := viper.NewWithOptions(viper.KeyDelimiter("::"))
	v.SetConfigFile(configPath)
	err = v.ReadInConfig()
	require.NoError(t, err)

	// Unmarshal to Config struct
	var cfg Config
	err = v.Unmarshal(&cfg)
	require.NoError(t, err)

	return cfg
}
