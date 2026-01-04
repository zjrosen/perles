package styles

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
