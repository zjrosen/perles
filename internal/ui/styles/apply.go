// Package styles contains Lip Gloss style definitions.
package styles

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// styleRebuilders holds callbacks to rebuild styles in other packages.
// This avoids import cycles (styles can't import bql, but bql can register).
var styleRebuilders []func()

// RegisterStyleRebuilder adds a callback that will be called after ApplyTheme
// updates colors. Use this to rebuild styles in packages that depend on styles.
func RegisterStyleRebuilder(fn func()) {
	styleRebuilders = append(styleRebuilders, fn)
}

// ThemeConfig mirrors config.ThemeConfig to avoid circular imports.
type ThemeConfig struct {
	Preset string
	Mode   string
	Colors map[string]string
}

// ApplyTheme applies a complete theme configuration.
// Order of application:
// 1. Start with default colors
// 2. Apply preset (if specified)
// 3. Apply individual color overrides
// 4. Rebuild all Style objects
func ApplyTheme(cfg ThemeConfig) error {
	// Step 1: Start with default preset
	colors := maps.Clone(DefaultPreset.Colors)

	// Step 2: Apply preset if specified
	if cfg.Preset != "" && cfg.Preset != "default" {
		preset, ok := Presets[cfg.Preset]
		if !ok {
			return fmt.Errorf("unknown theme preset: %s", cfg.Preset)
		}
		maps.Copy(colors, preset.Colors)
	}

	// Step 3: Apply individual color overrides
	for key, value := range cfg.Colors {
		token := ColorToken(key)
		if !isValidToken(token) {
			return fmt.Errorf("unknown color token: %s", key)
		}
		if !isValidHexColor(value) {
			return fmt.Errorf("invalid hex color for %s: %s", key, value)
		}
		colors[token] = value
	}

	// Step 4: Apply colors to variables
	applyColors(colors)

	// Step 5: Rebuild all Style objects
	rebuildStyles()

	return nil
}

func applyColors(colors map[ColorToken]string) {
	// Helper to create adaptive color (uses same color for both modes)
	makeColor := func(hex string) lipgloss.AdaptiveColor {
		return lipgloss.AdaptiveColor{Light: hex, Dark: hex}
	}

	// Text hierarchy
	if c, ok := colors[TokenTextPrimary]; ok {
		TextPrimaryColor = makeColor(c)
	}
	if c, ok := colors[TokenTextSecondary]; ok {
		TextSecondaryColor = makeColor(c)
	}
	if c, ok := colors[TokenTextMuted]; ok {
		TextMutedColor = makeColor(c)
	}
	if c, ok := colors[TokenTextDescription]; ok {
		TextDescriptionColor = makeColor(c)
	}
	if c, ok := colors[TokenTextPlaceholder]; ok {
		TextPlaceholderColor = makeColor(c)
	}

	// Borders
	if c, ok := colors[TokenBorderDefault]; ok {
		BorderDefaultColor = makeColor(c)
	}
	if c, ok := colors[TokenBorderFocus]; ok {
		FormTextInputFocusedBorderColor = makeColor(c)
		FormTextInputFocusedLabelColor = makeColor(c)
	}
	if c, ok := colors[TokenBorderHighlight]; ok {
		BorderHighlightFocusColor = makeColor(c)
	}

	// Status
	if c, ok := colors[TokenStatusSuccess]; ok {
		StatusSuccessColor = makeColor(c)
	}
	if c, ok := colors[TokenStatusWarning]; ok {
		StatusWarningColor = makeColor(c)
	}
	if c, ok := colors[TokenStatusError]; ok {
		StatusErrorColor = makeColor(c)
	}

	// Selection
	if c, ok := colors[TokenSelectionIndicator]; ok {
		SelectionIndicatorColor = makeColor(c)
	}
	if c, ok := colors[TokenSelectionBackground]; ok {
		SelectionBackgroundColor = makeColor(c)
	}

	// Buttons
	if c, ok := colors[TokenButtonText]; ok {
		ButtonTextColor = makeColor(c)
	}
	if c, ok := colors[TokenButtonPrimaryBg]; ok {
		ButtonPrimaryBgColor = makeColor(c)
	}
	if c, ok := colors[TokenButtonPrimaryFocusBg]; ok {
		ButtonPrimaryFocusBgColor = makeColor(c)
	}
	if c, ok := colors[TokenButtonSecondaryBg]; ok {
		ButtonSecondaryBgColor = makeColor(c)
	}
	if c, ok := colors[TokenButtonSecondaryFocusBg]; ok {
		ButtonSecondaryFocusBgColor = makeColor(c)
	}
	if c, ok := colors[TokenButtonDangerBg]; ok {
		ButtonDangerBgColor = makeColor(c)
	}
	if c, ok := colors[TokenButtonDangerFocusBg]; ok {
		ButtonDangerFocusBgColor = makeColor(c)
	}
	if c, ok := colors[TokenButtonDisabledBg]; ok {
		ButtonDisabledBgColor = makeColor(c)
	}

	// Forms
	if c, ok := colors[TokenFormBorder]; ok {
		FormTextInputBorderColor = makeColor(c)
		FormTextInputLabelColor = makeColor(c)
	}
	if c, ok := colors[TokenFormBorderFocus]; ok {
		FormTextInputFocusedBorderColor = makeColor(c)
	}
	if c, ok := colors[TokenFormLabel]; ok {
		FormTextInputLabelColor = makeColor(c)
	}
	if c, ok := colors[TokenFormLabelFocus]; ok {
		FormTextInputFocusedLabelColor = makeColor(c)
	}

	// Overlays
	if c, ok := colors[TokenOverlayTitle]; ok {
		OverlayTitleColor = makeColor(c)
	}
	if c, ok := colors[TokenOverlayBorder]; ok {
		OverlayBorderColor = makeColor(c)
	}

	// Toast
	if c, ok := colors[TokenToastSuccess]; ok {
		ToastBorderSuccessColor = makeColor(c)
	}
	if c, ok := colors[TokenToastError]; ok {
		ToastBorderErrorColor = makeColor(c)
	}
	if c, ok := colors[TokenToastInfo]; ok {
		ToastBorderInfoColor = makeColor(c)
	}
	if c, ok := colors[TokenToastWarn]; ok {
		ToastBorderWarnColor = makeColor(c)
	}

	// Issue status
	if c, ok := colors[TokenIssueOpen]; ok {
		StatusOpenColor = makeColor(c)
	}
	if c, ok := colors[TokenIssueInProgress]; ok {
		StatusInProgressColor = makeColor(c)
	}
	if c, ok := colors[TokenIssueClosed]; ok {
		StatusClosedColor = makeColor(c)
	}

	// Priority
	if c, ok := colors[TokenPriorityCritical]; ok {
		PriorityCriticalColor = makeColor(c)
	}
	if c, ok := colors[TokenPriorityHigh]; ok {
		PriorityHighColor = makeColor(c)
	}
	if c, ok := colors[TokenPriorityMedium]; ok {
		PriorityMediumColor = makeColor(c)
	}
	if c, ok := colors[TokenPriorityLow]; ok {
		PriorityLowColor = makeColor(c)
	}
	if c, ok := colors[TokenPriorityBacklog]; ok {
		PriorityBacklogColor = makeColor(c)
	}

	// Issue type
	if c, ok := colors[TokenTypeTask]; ok {
		IssueTaskColor = makeColor(c)
	}
	if c, ok := colors[TokenTypeChore]; ok {
		IssueChoreColor = makeColor(c)
	}
	if c, ok := colors[TokenTypeEpic]; ok {
		IssueEpicColor = makeColor(c)
	}
	if c, ok := colors[TokenTypeBug]; ok {
		IssueBugColor = makeColor(c)
	}
	if c, ok := colors[TokenTypeFeature]; ok {
		IssueFeatureColor = makeColor(c)
	}
	if c, ok := colors[TokenTypeMolecule]; ok {
		IssueMoleculeColor = makeColor(c)
	}
	if c, ok := colors[TokenTypeConvoy]; ok {
		IssueConvoyColor = makeColor(c)
	}
	if c, ok := colors[TokenTypeAgent]; ok {
		IssueAgentColor = makeColor(c)
	}

	// BQL
	if c, ok := colors[TokenBQLKeyword]; ok {
		BQLKeywordColor = makeColor(c)
	}
	if c, ok := colors[TokenBQLOperator]; ok {
		BQLOperatorColor = makeColor(c)
	}
	if c, ok := colors[TokenBQLField]; ok {
		BQLFieldColor = makeColor(c)
	}
	if c, ok := colors[TokenBQLString]; ok {
		BQLStringColor = makeColor(c)
	}
	if c, ok := colors[TokenBQLLiteral]; ok {
		BQLLiteralColor = makeColor(c)
	}
	if c, ok := colors[TokenBQLParen]; ok {
		BQLParenColor = makeColor(c)
	}
	if c, ok := colors[TokenBQLComma]; ok {
		BQLCommaColor = makeColor(c)
	}

	// Misc
	if c, ok := colors[TokenSpinner]; ok {
		SpinnerColor = makeColor(c)
	}
}

// rebuildStyles recreates all Style objects with updated colors.
// This is necessary because lipgloss.Style objects capture colors at creation time.
func rebuildStyles() {
	// Selection indicator
	SelectionIndicatorStyle = lipgloss.NewStyle().Bold(true).Foreground(SelectionIndicatorColor)

	// Buttons
	baseButtonStyle = lipgloss.NewStyle().Padding(0, 2).Bold(true)

	PrimaryButtonStyle = baseButtonStyle.
		Foreground(ButtonTextColor).
		Background(ButtonPrimaryBgColor)

	PrimaryButtonFocusedStyle = baseButtonStyle.
		Foreground(ButtonTextColor).
		Background(ButtonPrimaryFocusBgColor).
		Underline(true).
		UnderlineSpaces(true)

	SecondaryButtonStyle = baseButtonStyle.
		Foreground(ButtonTextColor).
		Background(ButtonSecondaryBgColor)

	SecondaryButtonFocusedStyle = baseButtonStyle.
		Foreground(ButtonTextColor).
		Background(ButtonSecondaryFocusBgColor).
		Underline(true).
		UnderlineSpaces(true)

	DangerButtonStyle = baseButtonStyle.
		Foreground(ButtonTextColor).
		Background(ButtonDangerBgColor)

	DangerButtonFocusedStyle = baseButtonStyle.
		Foreground(ButtonTextColor).
		Background(ButtonDangerFocusBgColor).
		Underline(true).
		UnderlineSpaces(true)

	// Priority styles
	PriorityCriticalStyle = lipgloss.NewStyle().Foreground(PriorityCriticalColor).Bold(true)
	PriorityHighStyle = lipgloss.NewStyle().Foreground(PriorityHighColor)
	PriorityMediumStyle = lipgloss.NewStyle().Foreground(PriorityMediumColor)
	PriorityLowStyle = lipgloss.NewStyle().Foreground(PriorityLowColor)
	PriorityBacklogStyle = lipgloss.NewStyle().Foreground(PriorityBacklogColor)

	// Type styles
	TypeBugStyle = lipgloss.NewStyle().Foreground(StatusErrorColor)
	TypeFeatureStyle = lipgloss.NewStyle().Foreground(IssueFeatureColor)
	TypeTaskStyle = lipgloss.NewStyle().Foreground(IssueTaskColor)
	TypeEpicStyle = lipgloss.NewStyle().Foreground(IssueEpicColor)
	TypeChoreStyle = lipgloss.NewStyle().Foreground(IssueChoreColor)
	TypeMoleculeStyle = lipgloss.NewStyle().Foreground(IssueMoleculeColor)
	TypeConvoyStyle = lipgloss.NewStyle().Foreground(IssueConvoyColor)
	TypeAgentStyle = lipgloss.NewStyle().Foreground(IssueAgentColor)

	// Status bar
	StatusBarStyle = lipgloss.NewStyle().
		Foreground(TextSecondaryColor).
		Padding(0, 1)

	// Error display
	ErrorStyle = lipgloss.NewStyle().
		Foreground(StatusErrorColor).
		Bold(true).
		Padding(1, 2)

	// Call registered rebuilders (e.g., bql.RebuildStyles)
	for _, fn := range styleRebuilders {
		fn()
	}
}

func isValidToken(token ColorToken) bool {
	return slices.Contains(AllTokens(), token)
}

func isValidHexColor(s string) bool {
	if !strings.HasPrefix(s, "#") {
		return false
	}
	hex := s[1:]
	if len(hex) != 3 && len(hex) != 6 {
		return false
	}
	_, err := strconv.ParseUint(hex, 16, 64)
	return err == nil
}
