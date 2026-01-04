package styles

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/require"
)

// renderPresetSample creates a visual sample showing key colors from a preset.
// This serves as the golden file content - if colors change, the test fails.
func renderPresetSample(presetName string) string {
	var b strings.Builder

	// Apply the preset
	cfg := ThemeConfig{Preset: presetName}
	if presetName == "default" {
		cfg.Preset = "" // Empty preset means default
	}
	if err := ApplyTheme(cfg); err != nil {
		return fmt.Sprintf("Error applying preset %s: %v", presetName, err)
	}

	// Rebuild styles after applying theme
	rebuildStyles()

	b.WriteString(fmt.Sprintf("=== %s Theme Sample ===\n\n", presetName))

	// Text colors
	b.WriteString("Text Colors:\n")
	textPrimary := lipgloss.NewStyle().Foreground(TextPrimaryColor)
	textSecondary := lipgloss.NewStyle().Foreground(TextSecondaryColor)
	textMuted := lipgloss.NewStyle().Foreground(TextMutedColor)
	b.WriteString(fmt.Sprintf("  %s  %s  %s\n",
		textPrimary.Render("Primary"),
		textSecondary.Render("Secondary"),
		textMuted.Render("Muted")))

	// Status colors
	b.WriteString("\nStatus Colors:\n")
	statusSuccess := lipgloss.NewStyle().Foreground(StatusSuccessColor)
	statusWarning := lipgloss.NewStyle().Foreground(StatusWarningColor)
	statusError := lipgloss.NewStyle().Foreground(StatusErrorColor)
	b.WriteString(fmt.Sprintf("  %s  %s  %s\n",
		statusSuccess.Render("Success"),
		statusWarning.Render("Warning"),
		statusError.Render("Error")))

	// Priority colors
	b.WriteString("\nPriority Colors:\n")
	b.WriteString(fmt.Sprintf("  %s  %s  %s  %s  %s\n",
		PriorityCriticalStyle.Render("P0"),
		PriorityHighStyle.Render("P1"),
		PriorityMediumStyle.Render("P2"),
		PriorityLowStyle.Render("P3"),
		PriorityBacklogStyle.Render("P4")))

	// Issue type colors
	b.WriteString("\nType Colors:\n")
	b.WriteString(fmt.Sprintf("  %s  %s  %s  %s  %s\n",
		TypeBugStyle.Render("bug"),
		TypeFeatureStyle.Render("feature"),
		TypeTaskStyle.Render("task"),
		TypeEpicStyle.Render("epic"),
		TypeChoreStyle.Render("chore")))

	// Button styles
	b.WriteString("\nButton Styles:\n")
	b.WriteString(fmt.Sprintf("  %s  %s  %s\n",
		PrimaryButtonStyle.Render("Primary"),
		SecondaryButtonStyle.Render("Secondary"),
		DangerButtonStyle.Render("Danger")))

	// Border colors (using a simple box)
	b.WriteString("\nBorders:\n")
	defaultBorder := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(BorderDefaultColor).
		Padding(0, 1)
	highlightBorder := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(BorderHighlightFocusColor).
		Padding(0, 1)
	b.WriteString(fmt.Sprintf("  %s  %s\n",
		defaultBorder.Render("Default"),
		highlightBorder.Render("Highlight")))

	return b.String()
}

func TestPreset_Default_Golden(t *testing.T) {
	output := renderPresetSample("default")
	teatest.RequireEqualOutput(t, []byte(output))
}

func TestPreset_CatppuccinMocha_Golden(t *testing.T) {
	output := renderPresetSample("catppuccin-mocha")
	teatest.RequireEqualOutput(t, []byte(output))
}

func TestPreset_Dracula_Golden(t *testing.T) {
	output := renderPresetSample("dracula")
	teatest.RequireEqualOutput(t, []byte(output))
}

func TestPreset_Nord_Golden(t *testing.T) {
	output := renderPresetSample("nord")
	teatest.RequireEqualOutput(t, []byte(output))
}

func TestPreset_HighContrast_Golden(t *testing.T) {
	output := renderPresetSample("high-contrast")
	teatest.RequireEqualOutput(t, []byte(output))
}

func TestPreset_CatppuccinLatte_Golden(t *testing.T) {
	output := renderPresetSample("catppuccin-latte")
	teatest.RequireEqualOutput(t, []byte(output))
}

// TestAllPresetsHaveTokenSelectionBackground verifies that all 7 theme presets
// define TokenSelectionBackground with the correct palette-appropriate colors.
func TestAllPresetsHaveTokenSelectionBackground(t *testing.T) {
	expectedColors := map[string]string{
		"default":          "#1A5276", // current hard-coded value
		"catppuccin-mocha": "#45475A", // surface1
		"catppuccin-latte": "#BCC0CC", // surface1 (light)
		"dracula":          "#44475A", // current line
		"nord":             "#434C5E", // polar night 3
		"high-contrast":    "#0000FF", // pure blue
		"gruvbox":          "#504945", // bg2
	}

	// Verify we're testing all presets
	require.Equal(t, len(Presets), len(expectedColors),
		"Test should cover all presets (expected %d, got %d)", len(Presets), len(expectedColors))

	for presetName, expectedColor := range expectedColors {
		t.Run(presetName, func(t *testing.T) {
			preset, exists := Presets[presetName]
			require.True(t, exists, "Preset %q should exist", presetName)

			actualColor, hasToken := preset.Colors[TokenSelectionBackground]
			require.True(t, hasToken,
				"Preset %q should define TokenSelectionBackground", presetName)
			require.Equal(t, expectedColor, actualColor,
				"Preset %q TokenSelectionBackground color mismatch", presetName)
		})
	}
}
