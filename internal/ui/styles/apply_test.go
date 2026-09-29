package styles

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/task"
)

// resetThemeAfter restores the default theme once the test finishes so
// global color state doesn't leak between tests.
func resetThemeAfter(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { _ = ApplyTheme(ThemeConfig{}) })
}

// themeColor mirrors how ApplyTheme builds colors from a hex value.
func themeColor(hex string) lipgloss.AdaptiveColor {
	return lipgloss.AdaptiveColor{Light: hex, Dark: hex}
}

func TestApplyTheme_Default(t *testing.T) {
	err := ApplyTheme(ThemeConfig{})
	require.NoError(t, err)
	// Should apply default preset colors
	require.Equal(t, DefaultPreset.Colors[TokenTextPrimary], TextPrimaryColor.Dark)
}

func TestApplyTheme_Preset(t *testing.T) {
	// First add a test preset
	TestPreset := Preset{
		Name:        "test",
		Description: "Test preset",
		Colors: map[ColorToken]string{
			TokenTextPrimary: "#FF0000",
		},
	}
	Presets["test"] = TestPreset
	defer delete(Presets, "test")

	err := ApplyTheme(ThemeConfig{Preset: "test"})
	require.NoError(t, err)
	require.Equal(t, "#FF0000", TextPrimaryColor.Dark)
}

func TestApplyTheme_ColorOverride(t *testing.T) {
	err := ApplyTheme(ThemeConfig{
		Colors: map[string]string{
			"text.primary": "#00FF00",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "#00FF00", TextPrimaryColor.Dark)
}

func TestApplyTheme_PresetWithOverride(t *testing.T) {
	// Color override should take precedence over preset
	TestPreset := Preset{
		Name:        "test2",
		Description: "Test preset 2",
		Colors: map[ColorToken]string{
			TokenTextPrimary:   "#FF0000",
			TokenTextSecondary: "#0000FF",
		},
	}
	Presets["test2"] = TestPreset
	defer delete(Presets, "test2")

	err := ApplyTheme(ThemeConfig{
		Preset: "test2",
		Colors: map[string]string{
			"text.primary": "#00FF00", // Override preset
		},
	})
	require.NoError(t, err)
	require.Equal(t, "#00FF00", TextPrimaryColor.Dark)   // Overridden
	require.Equal(t, "#0000FF", TextSecondaryColor.Dark) // From preset
}

func TestApplyTheme_InvalidPreset(t *testing.T) {
	err := ApplyTheme(ThemeConfig{Preset: "nonexistent"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown theme preset")
}

func TestApplyTheme_InvalidToken(t *testing.T) {
	err := ApplyTheme(ThemeConfig{
		Colors: map[string]string{
			"invalid.token": "#FF0000",
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown color token")
}

func TestApplyTheme_InvalidHexColor(t *testing.T) {
	err := ApplyTheme(ThemeConfig{
		Colors: map[string]string{
			"text.primary": "not-a-color",
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid hex color")
}

func TestIsValidToken(t *testing.T) {
	tests := []struct {
		token ColorToken
		valid bool
	}{
		{TokenTextPrimary, true},
		{TokenStatusError, true},
		{TokenSelectionBackground, true},
		{ColorToken("selection.background"), true},
		{ColorToken("invalid.token"), false},
		{ColorToken(""), false},
	}
	for _, tt := range tests {
		t.Run(string(tt.token), func(t *testing.T) {
			require.Equal(t, tt.valid, isValidToken(tt.token))
		})
	}
}

func TestTokenSelectionBackgroundInAllTokens(t *testing.T) {
	tokens := AllTokens()
	found := false
	for _, token := range tokens {
		if token == TokenSelectionBackground {
			found = true
			break
		}
	}
	require.True(t, found, "TokenSelectionBackground should be in AllTokens()")
}

func TestApplyTheme_SelectionBackgroundColor(t *testing.T) {
	// Create a test preset with a specific SelectionBackgroundColor
	TestPreset := Preset{
		Name:        "test-selection-bg",
		Description: "Test preset for SelectionBackgroundColor",
		Colors: map[ColorToken]string{
			TokenSelectionBackground: "#AABBCC",
		},
	}
	Presets["test-selection-bg"] = TestPreset
	defer delete(Presets, "test-selection-bg")

	err := ApplyTheme(ThemeConfig{Preset: "test-selection-bg"})
	require.NoError(t, err)
	require.Equal(t, "#AABBCC", SelectionBackgroundColor.Dark,
		"ApplyTheme should update SelectionBackgroundColor.Dark")
}

func TestApplyTheme_SelectionBackgroundColorOverride(t *testing.T) {
	// Config override should take precedence over preset value
	TestPreset := Preset{
		Name:        "test-selection-bg-override",
		Description: "Test preset for SelectionBackgroundColor override",
		Colors: map[ColorToken]string{
			TokenSelectionBackground: "#111111", // Preset value
		},
	}
	Presets["test-selection-bg-override"] = TestPreset
	defer delete(Presets, "test-selection-bg-override")

	err := ApplyTheme(ThemeConfig{
		Preset: "test-selection-bg-override",
		Colors: map[string]string{
			"selection.background": "#222222", // Override value
		},
	})
	require.NoError(t, err)
	require.Equal(t, "#222222", SelectionBackgroundColor.Dark,
		"Config override should take precedence over preset value for SelectionBackgroundColor")
}

func TestIsValidHexColor(t *testing.T) {
	tests := []struct {
		color string
		valid bool
	}{
		{"#FFF", true},
		{"#FFFFFF", true},
		{"#abc", true},
		{"#AbCdEf", true},
		{"#123456", true},
		{"FFFFFF", false},   // Missing #
		{"#FF", false},      // Too short
		{"#FFFFFFF", false}, // Too long
		{"#GGGGGG", false},  // Invalid chars
		{"not-color", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.color, func(t *testing.T) {
			require.Equal(t, tt.valid, isValidHexColor(tt.color))
		})
	}
}

func TestApplyTheme_TypeBugDrivesTypeBugStyle(t *testing.T) {
	resetThemeAfter(t)

	err := ApplyTheme(ThemeConfig{
		Colors: map[string]string{
			"type.bug":     "#123456",
			"status.error": "#654321",
		},
	})
	require.NoError(t, err)
	require.Equal(t, themeColor("#123456"), IssueBugColor)
	require.Equal(t, themeColor("#123456"), TypeBugStyle.GetForeground(),
		"type.bug should color bug type badges")
	require.Equal(t, themeColor("#654321"), ErrorStyle.GetForeground(),
		"status.error should still drive error display")
	require.Equal(t, themeColor("#123456"), GetTypeStyle(task.TypeBug).GetForeground())
}

func TestApplyTheme_PresetTypeBugMatchesStatusError(t *testing.T) {
	// Every built-in preset sets type.bug to its status.error color, so moving
	// TypeBugStyle onto type.bug must not change how bug badges render.
	resetThemeAfter(t)

	for name, preset := range Presets {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, preset.Colors[TokenStatusError], preset.Colors[TokenTypeBug])

			require.NoError(t, ApplyTheme(ThemeConfig{Preset: name}))
			require.Equal(t, themeColor(preset.Colors[TokenStatusError]), TypeBugStyle.GetForeground())
		})
	}
}

func TestApplyTheme_BorderFocusAliasesBorderHighlight(t *testing.T) {
	resetThemeAfter(t)

	err := ApplyTheme(ThemeConfig{
		Colors: map[string]string{"border.focus": "#ABCDEF"},
	})
	require.NoError(t, err)
	require.Equal(t, themeColor("#ABCDEF"), BorderHighlightFocusColor)
	// Form focus colors keep coming from form.border.focus / form.label.focus.
	require.Equal(t, DefaultPreset.Colors[TokenFormBorderFocus], FormTextInputFocusedBorderColor.Dark)
	require.Equal(t, DefaultPreset.Colors[TokenFormLabelFocus], FormTextInputFocusedLabelColor.Dark)
}

func TestApplyTheme_BorderFocusOverridesPresetBorderHighlight(t *testing.T) {
	resetThemeAfter(t)

	err := ApplyTheme(ThemeConfig{
		Preset: "nord",
		Colors: map[string]string{"border.focus": "#ABCDEF"},
	})
	require.NoError(t, err)
	require.Equal(t, themeColor("#ABCDEF"), BorderHighlightFocusColor)
}

func TestApplyTheme_BorderHighlightWinsOverBorderFocus(t *testing.T) {
	resetThemeAfter(t)

	err := ApplyTheme(ThemeConfig{
		Colors: map[string]string{
			"border.focus":     "#ABCDEF",
			"border.highlight": "#FEDCBA",
		},
	})
	require.NoError(t, err)
	require.Equal(t, themeColor("#FEDCBA"), BorderHighlightFocusColor)
}

func TestApplyTheme_PresetBorderFocusDoesNotAffectRendering(t *testing.T) {
	resetThemeAfter(t)

	// Built-in presets define border.focus alongside border.highlight; only
	// border.highlight should reach the focused border color.
	for name, preset := range Presets {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, ApplyTheme(ThemeConfig{Preset: name}))
			require.Equal(t, themeColor(preset.Colors[TokenBorderHighlight]), BorderHighlightFocusColor)
		})
	}

	// A preset that sets only border.focus is not aliased either.
	Presets["test-border-focus"] = Preset{
		Name:   "test-border-focus",
		Colors: map[ColorToken]string{TokenBorderFocus: "#ABCDEF"},
	}
	defer delete(Presets, "test-border-focus")

	require.NoError(t, ApplyTheme(ThemeConfig{Preset: "test-border-focus"}))
	require.Equal(t, themeColor(DefaultPreset.Colors[TokenBorderHighlight]), BorderHighlightFocusColor)
}
