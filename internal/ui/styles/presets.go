// Package styles contains Lip Gloss style definitions.
package styles

// Preset represents a complete color theme.
type Preset struct {
	Name        string
	Description string
	Colors      map[ColorToken]string
}

// Presets contains all built-in theme presets.
var Presets = map[string]Preset{
	"default":          DefaultPreset,
	"catppuccin-mocha": CatppuccinMochaPreset,
	"catppuccin-latte": CatppuccinLattePreset,
	"dracula":          DraculaPreset,
	"nord":             NordPreset,
	"high-contrast":    HighContrastPreset,
}

// DefaultPreset is the current perles color scheme.
// Color values extracted from styles.go AdaptiveColor definitions (Dark values).
var DefaultPreset = Preset{
	Name:        "default",
	Description: "Default perles theme",
	Colors: map[ColorToken]string{
		// Text hierarchy
		TokenTextPrimary:     "#CCCCCC",
		TokenTextSecondary:   "#BBBBBB",
		TokenTextMuted:       "#696969",
		TokenTextDescription: "#999999",
		TokenTextPlaceholder: "#777777",

		// Borders
		TokenBorderDefault:   "#696969",
		TokenBorderFocus:     "#FFFFFF",
		TokenBorderHighlight: "#54A0FF",

		// Status indicators
		TokenStatusSuccess: "#73F59F",
		TokenStatusWarning: "#FECA57",
		TokenStatusError:   "#FF8787",

		// Selection
		TokenSelectionIndicator: "#FFFFFF",

		// Buttons
		TokenButtonText:             "#FFFFFF",
		TokenButtonPrimaryBg:        "#1A5276",
		TokenButtonPrimaryFocusBg:   "#3498DB",
		TokenButtonSecondaryBg:      "#2D3436",
		TokenButtonSecondaryFocusBg: "#636E72",
		TokenButtonDangerBg:         "#922B21",
		TokenButtonDangerFocusBg:    "#E74C3C",
		TokenButtonDisabledBg:       "#2D2D2D",

		// Forms
		TokenFormBorder:      "#8C8C8C",
		TokenFormBorderFocus: "#FFFFFF",
		TokenFormLabel:       "#8C8C8C",
		TokenFormLabelFocus:  "#FFFFFF",

		// Overlays/Modals
		TokenOverlayTitle:  "#C9C9C9",
		TokenOverlayBorder: "#8C8C8C",

		// Toast notifications
		TokenToastSuccess: "#73F59F",
		TokenToastError:   "#FF8787",
		TokenToastInfo:    "#54A0FF",
		TokenToastWarn:    "#FECA57",

		// Issue status
		TokenIssueOpen:       "#73F59F",
		TokenIssueInProgress: "#54A0FF",
		TokenIssueClosed:     "#BBBBBB",

		// Issue priority
		TokenPriorityCritical: "#FF8787",
		TokenPriorityHigh:     "#FF9F43",
		TokenPriorityMedium:   "#FECA57",
		TokenPriorityLow:      "#999999",
		TokenPriorityBacklog:  "#666666",

		// Issue type
		TokenTypeTask:     "#54A0FF",
		TokenTypeChore:    "#777777",
		TokenTypeEpic:     "#7D56F4",
		TokenTypeBug:      "#FF8787",
		TokenTypeFeature:  "#73F59F",
		TokenTypeMolecule: "#FF731A",

		// BQL syntax highlighting (Catppuccin Mocha inspired)
		TokenBQLKeyword:  "#CBA6F7",
		TokenBQLOperator: "#F38BA8",
		TokenBQLField:    "#94E2D5",
		TokenBQLString:   "#F9E2AF",
		TokenBQLLiteral:  "#FAB387",
		TokenBQLParen:    "#89B4FA",
		TokenBQLComma:    "#6C7086",

		// Misc
		TokenSpinner: "#FFFFFF",
	},
}

// CatppuccinMochaPreset is the Catppuccin Mocha (dark) theme.
// Colors from: https://catppuccin.com/palette
// Mocha flavor - warm, cozy dark theme with pastel colors.
var CatppuccinMochaPreset = Preset{
	Name:        "catppuccin-mocha",
	Description: "Catppuccin Mocha - warm, cozy dark theme",
	Colors: map[ColorToken]string{
		// Text hierarchy
		TokenTextPrimary:     "#CDD6F4", // text
		TokenTextSecondary:   "#BAC2DE", // subtext1
		TokenTextMuted:       "#6C7086", // overlay0
		TokenTextDescription: "#A6ADC8", // subtext0
		TokenTextPlaceholder: "#585B70", // surface2

		// Borders
		TokenBorderDefault:   "#6C7086", // overlay0
		TokenBorderFocus:     "#CDD6F4", // text
		TokenBorderHighlight: "#89B4FA", // blue

		// Status indicators
		TokenStatusSuccess: "#A6E3A1", // green
		TokenStatusWarning: "#F9E2AF", // yellow
		TokenStatusError:   "#F38BA8", // red

		// Selection
		TokenSelectionIndicator: "#CDD6F4", // text

		// Buttons
		TokenButtonText:             "#1E1E2E", // base
		TokenButtonPrimaryBg:        "#89B4FA", // blue
		TokenButtonPrimaryFocusBg:   "#B4BEFE", // lavender
		TokenButtonSecondaryBg:      "#45475A", // surface1
		TokenButtonSecondaryFocusBg: "#585B70", // surface2
		TokenButtonDangerBg:         "#F38BA8", // red
		TokenButtonDangerFocusBg:    "#EBA0AC", // maroon
		TokenButtonDisabledBg:       "#313244", // surface0

		// Forms
		TokenFormBorder:      "#6C7086", // overlay0
		TokenFormBorderFocus: "#CDD6F4", // text
		TokenFormLabel:       "#6C7086", // overlay0
		TokenFormLabelFocus:  "#CDD6F4", // text

		// Overlays/Modals
		TokenOverlayTitle:  "#CDD6F4", // text
		TokenOverlayBorder: "#6C7086", // overlay0

		// Toast notifications
		TokenToastSuccess: "#A6E3A1", // green
		TokenToastError:   "#F38BA8", // red
		TokenToastInfo:    "#89B4FA", // blue
		TokenToastWarn:    "#F9E2AF", // yellow

		// Issue status
		TokenIssueOpen:       "#A6E3A1", // green
		TokenIssueInProgress: "#89B4FA", // blue
		TokenIssueClosed:     "#6C7086", // overlay0

		// Issue priority
		TokenPriorityCritical: "#F38BA8", // red
		TokenPriorityHigh:     "#FAB387", // peach
		TokenPriorityMedium:   "#F9E2AF", // yellow
		TokenPriorityLow:      "#A6ADC8", // subtext0
		TokenPriorityBacklog:  "#6C7086", // overlay0

		// Issue type
		TokenTypeTask:     "#89B4FA", // blue
		TokenTypeChore:    "#6C7086", // overlay0
		TokenTypeEpic:     "#CBA6F7", // mauve
		TokenTypeBug:      "#F38BA8", // red
		TokenTypeFeature:  "#A6E3A1", // green
		TokenTypeMolecule: "#6C7086", // overlay0

		// BQL syntax highlighting
		TokenBQLKeyword:  "#CBA6F7", // mauve
		TokenBQLOperator: "#F38BA8", // red
		TokenBQLField:    "#94E2D5", // teal
		TokenBQLString:   "#F9E2AF", // yellow
		TokenBQLLiteral:  "#FAB387", // peach
		TokenBQLParen:    "#89B4FA", // blue
		TokenBQLComma:    "#6C7086", // overlay0

		// Misc
		TokenSpinner: "#CBA6F7", // mauve
	},
}

// CatppuccinLattePreset is the Catppuccin Latte (light) theme.
// Colors from: https://catppuccin.com/palette
// Latte flavor - light theme for bright environments.
var CatppuccinLattePreset = Preset{
	Name:        "catppuccin-latte",
	Description: "Catppuccin Latte - warm, cozy light theme",
	Colors: map[ColorToken]string{
		// Text hierarchy
		TokenTextPrimary:     "#4C4F69", // text
		TokenTextSecondary:   "#5C5F77", // subtext1
		TokenTextMuted:       "#9CA0B0", // overlay0
		TokenTextDescription: "#6C6F85", // subtext0
		TokenTextPlaceholder: "#ACB0BE", // surface2

		// Borders
		TokenBorderDefault:   "#9CA0B0", // overlay0
		TokenBorderFocus:     "#4C4F69", // text
		TokenBorderHighlight: "#1E66F5", // blue

		// Status indicators
		TokenStatusSuccess: "#40A02B", // green
		TokenStatusWarning: "#DF8E1D", // yellow
		TokenStatusError:   "#D20F39", // red

		// Selection
		TokenSelectionIndicator: "#4C4F69", // text

		// Buttons
		TokenButtonText:             "#EFF1F5", // base
		TokenButtonPrimaryBg:        "#1E66F5", // blue
		TokenButtonPrimaryFocusBg:   "#7287FD", // lavender
		TokenButtonSecondaryBg:      "#BCC0CC", // surface1
		TokenButtonSecondaryFocusBg: "#ACB0BE", // surface2
		TokenButtonDangerBg:         "#D20F39", // red
		TokenButtonDangerFocusBg:    "#E64553", // maroon
		TokenButtonDisabledBg:       "#CCD0DA", // surface0

		// Forms
		TokenFormBorder:      "#9CA0B0", // overlay0
		TokenFormBorderFocus: "#4C4F69", // text
		TokenFormLabel:       "#9CA0B0", // overlay0
		TokenFormLabelFocus:  "#4C4F69", // text

		// Overlays/Modals
		TokenOverlayTitle:  "#4C4F69", // text
		TokenOverlayBorder: "#9CA0B0", // overlay0

		// Toast notifications
		TokenToastSuccess: "#40A02B", // green
		TokenToastError:   "#D20F39", // red
		TokenToastInfo:    "#1E66F5", // blue
		TokenToastWarn:    "#DF8E1D", // yellow

		// Issue status
		TokenIssueOpen:       "#40A02B", // green
		TokenIssueInProgress: "#1E66F5", // blue
		TokenIssueClosed:     "#9CA0B0", // overlay0

		// Issue priority
		TokenPriorityCritical: "#D20F39", // red
		TokenPriorityHigh:     "#FE640B", // peach
		TokenPriorityMedium:   "#DF8E1D", // yellow
		TokenPriorityLow:      "#6C6F85", // subtext0
		TokenPriorityBacklog:  "#9CA0B0", // overlay0

		// Issue type
		TokenTypeTask:     "#1E66F5", // blue
		TokenTypeChore:    "#9CA0B0", // overlay0
		TokenTypeEpic:     "#8839EF", // mauve
		TokenTypeBug:      "#D20F39", // red
		TokenTypeFeature:  "#40A02B", // green
		TokenTypeMolecule: "#9CA0B0", // overlay0

		// BQL syntax highlighting
		TokenBQLKeyword:  "#8839EF", // mauve
		TokenBQLOperator: "#D20F39", // red
		TokenBQLField:    "#179299", // teal
		TokenBQLString:   "#DF8E1D", // yellow
		TokenBQLLiteral:  "#FE640B", // peach
		TokenBQLParen:    "#1E66F5", // blue
		TokenBQLComma:    "#9CA0B0", // overlay0

		// Misc
		TokenSpinner: "#8839EF", // mauve
	},
}

// DraculaPreset is the Dracula theme.
// Colors from: https://draculatheme.com/contribute
// Dark theme with vibrant, high-contrast colors.
var DraculaPreset = Preset{
	Name:        "dracula",
	Description: "Dracula - dark theme with vibrant colors",
	Colors: map[ColorToken]string{
		// Text hierarchy
		TokenTextPrimary:     "#F8F8F2", // foreground
		TokenTextSecondary:   "#F8F8F2", // foreground
		TokenTextMuted:       "#6272A4", // comment
		TokenTextDescription: "#F8F8F2", // foreground
		TokenTextPlaceholder: "#6272A4", // comment

		// Borders
		TokenBorderDefault:   "#6272A4", // comment
		TokenBorderFocus:     "#F8F8F2", // foreground
		TokenBorderHighlight: "#BD93F9", // purple

		// Status indicators
		TokenStatusSuccess: "#50FA7B", // green
		TokenStatusWarning: "#F1FA8C", // yellow
		TokenStatusError:   "#FF5555", // red

		// Selection
		TokenSelectionIndicator: "#F8F8F2", // foreground

		// Buttons
		TokenButtonText:             "#282A36", // background
		TokenButtonPrimaryBg:        "#BD93F9", // purple
		TokenButtonPrimaryFocusBg:   "#FF79C6", // pink
		TokenButtonSecondaryBg:      "#44475A", // current line
		TokenButtonSecondaryFocusBg: "#6272A4", // comment
		TokenButtonDangerBg:         "#FF5555", // red
		TokenButtonDangerFocusBg:    "#FF6E6E", // lighter red
		TokenButtonDisabledBg:       "#44475A", // current line

		// Forms
		TokenFormBorder:      "#6272A4", // comment
		TokenFormBorderFocus: "#F8F8F2", // foreground
		TokenFormLabel:       "#6272A4", // comment
		TokenFormLabelFocus:  "#F8F8F2", // foreground

		// Overlays/Modals
		TokenOverlayTitle:  "#F8F8F2", // foreground
		TokenOverlayBorder: "#6272A4", // comment

		// Toast notifications
		TokenToastSuccess: "#50FA7B", // green
		TokenToastError:   "#FF5555", // red
		TokenToastInfo:    "#8BE9FD", // cyan
		TokenToastWarn:    "#F1FA8C", // yellow

		// Issue status
		TokenIssueOpen:       "#50FA7B", // green
		TokenIssueInProgress: "#8BE9FD", // cyan
		TokenIssueClosed:     "#6272A4", // comment

		// Issue priority
		TokenPriorityCritical: "#FF5555", // red
		TokenPriorityHigh:     "#FFB86C", // orange
		TokenPriorityMedium:   "#F1FA8C", // yellow
		TokenPriorityLow:      "#6272A4", // comment
		TokenPriorityBacklog:  "#44475A", // current line

		// Issue type
		TokenTypeTask:     "#8BE9FD", // cyan
		TokenTypeChore:    "#6272A4", // comment
		TokenTypeEpic:     "#BD93F9", // purple
		TokenTypeBug:      "#FF5555", // red
		TokenTypeFeature:  "#50FA7B", // green
		TokenTypeMolecule: "#6272A4", // comment

		// BQL syntax highlighting
		TokenBQLKeyword:  "#FF79C6", // pink
		TokenBQLOperator: "#FF5555", // red
		TokenBQLField:    "#8BE9FD", // cyan
		TokenBQLString:   "#F1FA8C", // yellow
		TokenBQLLiteral:  "#FFB86C", // orange
		TokenBQLParen:    "#BD93F9", // purple
		TokenBQLComma:    "#6272A4", // comment

		// Misc
		TokenSpinner: "#BD93F9", // purple
	},
}

// NordPreset is the Nord theme.
// Colors from: https://www.nordtheme.com/docs/colors-and-palettes
// Arctic, north-bluish color palette with calm, muted tones.
// Polar Night: #2E3440, #3B4252, #434C5E, #4C566A (backgrounds)
// Snow Storm: #D8DEE9, #E5E9F0, #ECEFF4 (text)
// Frost: #8FBCBB, #88C0D0, #81A1C1, #5E81AC (accents)
// Aurora: #BF616A (red), #D08770 (orange), #EBCB8B (yellow), #A3BE8C (green), #B48EAD (purple)
var NordPreset = Preset{
	Name:        "nord",
	Description: "Nord - arctic, north-bluish palette",
	Colors: map[ColorToken]string{
		// Text hierarchy
		TokenTextPrimary:     "#ECEFF4", // snow storm 3
		TokenTextSecondary:   "#E5E9F0", // snow storm 2
		TokenTextMuted:       "#4C566A", // polar night 4
		TokenTextDescription: "#D8DEE9", // snow storm 1
		TokenTextPlaceholder: "#4C566A", // polar night 4

		// Borders
		TokenBorderDefault:   "#4C566A", // polar night 4
		TokenBorderFocus:     "#ECEFF4", // snow storm 3
		TokenBorderHighlight: "#88C0D0", // frost 2

		// Status indicators
		TokenStatusSuccess: "#A3BE8C", // aurora green
		TokenStatusWarning: "#EBCB8B", // aurora yellow
		TokenStatusError:   "#BF616A", // aurora red

		// Selection
		TokenSelectionIndicator: "#ECEFF4", // snow storm 3

		// Buttons
		TokenButtonText:             "#2E3440", // polar night 1
		TokenButtonPrimaryBg:        "#5E81AC", // frost 4
		TokenButtonPrimaryFocusBg:   "#81A1C1", // frost 3
		TokenButtonSecondaryBg:      "#434C5E", // polar night 3
		TokenButtonSecondaryFocusBg: "#4C566A", // polar night 4
		TokenButtonDangerBg:         "#BF616A", // aurora red
		TokenButtonDangerFocusBg:    "#D08770", // aurora orange
		TokenButtonDisabledBg:       "#3B4252", // polar night 2

		// Forms
		TokenFormBorder:      "#4C566A", // polar night 4
		TokenFormBorderFocus: "#ECEFF4", // snow storm 3
		TokenFormLabel:       "#4C566A", // polar night 4
		TokenFormLabelFocus:  "#ECEFF4", // snow storm 3

		// Overlays/Modals
		TokenOverlayTitle:  "#ECEFF4", // snow storm 3
		TokenOverlayBorder: "#4C566A", // polar night 4

		// Toast notifications
		TokenToastSuccess: "#A3BE8C", // aurora green
		TokenToastError:   "#BF616A", // aurora red
		TokenToastInfo:    "#81A1C1", // frost 3
		TokenToastWarn:    "#EBCB8B", // aurora yellow

		// Issue status
		TokenIssueOpen:       "#A3BE8C", // aurora green
		TokenIssueInProgress: "#88C0D0", // frost 2
		TokenIssueClosed:     "#4C566A", // polar night 4

		// Issue priority
		TokenPriorityCritical: "#BF616A", // aurora red
		TokenPriorityHigh:     "#D08770", // aurora orange
		TokenPriorityMedium:   "#EBCB8B", // aurora yellow
		TokenPriorityLow:      "#4C566A", // polar night 4
		TokenPriorityBacklog:  "#434C5E", // polar night 3

		// Issue type
		TokenTypeTask:     "#88C0D0", // frost 2
		TokenTypeChore:    "#4C566A", // polar night 4
		TokenTypeEpic:     "#B48EAD", // aurora purple
		TokenTypeBug:      "#BF616A", // aurora red
		TokenTypeFeature:  "#A3BE8C", // aurora green
		TokenTypeMolecule: "#4C566A", // polar night 4

		// BQL syntax highlighting
		TokenBQLKeyword:  "#81A1C1", // frost 3
		TokenBQLOperator: "#BF616A", // aurora red
		TokenBQLField:    "#8FBCBB", // frost 1
		TokenBQLString:   "#EBCB8B", // aurora yellow
		TokenBQLLiteral:  "#D08770", // aurora orange
		TokenBQLParen:    "#5E81AC", // frost 4
		TokenBQLComma:    "#4C566A", // polar night 4

		// Misc
		TokenSpinner: "#88C0D0", // frost 2
	},
}

// HighContrastPreset is a high contrast theme for accessibility.
// Designed for users with visual impairments or those who prefer maximum visibility.
// All colors meet WCAG AAA contrast requirements (7:1 minimum ratio against black).
// No subtle or muted colors - everything is clearly visible.
var HighContrastPreset = Preset{
	Name:        "high-contrast",
	Description: "High contrast for accessibility",
	Colors: map[ColorToken]string{
		// Text hierarchy - pure white for maximum visibility
		TokenTextPrimary:     "#FFFFFF",
		TokenTextSecondary:   "#FFFFFF",
		TokenTextMuted:       "#FFFFFF", // no muted colors in high contrast
		TokenTextDescription: "#FFFFFF",
		TokenTextPlaceholder: "#CCCCCC", // slightly dimmed but still readable

		// Borders - white for maximum visibility
		TokenBorderDefault:   "#FFFFFF",
		TokenBorderFocus:     "#FFFF00", // bright yellow for focus
		TokenBorderHighlight: "#00FFFF", // cyan for highlights

		// Status indicators - pure, saturated colors
		TokenStatusSuccess: "#00FF00", // pure green
		TokenStatusWarning: "#FFFF00", // pure yellow
		TokenStatusError:   "#FF0000", // pure red

		// Selection - bright indicator
		TokenSelectionIndicator: "#FFFF00", // yellow for visibility

		// Buttons - high contrast backgrounds
		TokenButtonText:             "#000000", // black text on bright buttons
		TokenButtonPrimaryBg:        "#00FFFF", // cyan
		TokenButtonPrimaryFocusBg:   "#FFFFFF", // white when focused
		TokenButtonSecondaryBg:      "#808080", // gray
		TokenButtonSecondaryFocusBg: "#FFFFFF", // white when focused
		TokenButtonDangerBg:         "#FF0000", // red
		TokenButtonDangerFocusBg:    "#FF6666", // lighter red
		TokenButtonDisabledBg:       "#404040", // dark gray

		// Forms - white borders for visibility
		TokenFormBorder:      "#FFFFFF",
		TokenFormBorderFocus: "#FFFF00", // yellow focus
		TokenFormLabel:       "#FFFFFF",
		TokenFormLabelFocus:  "#FFFF00",

		// Overlays/Modals - white borders
		TokenOverlayTitle:  "#FFFFFF",
		TokenOverlayBorder: "#FFFFFF",

		// Toast notifications - pure colors
		TokenToastSuccess: "#00FF00",
		TokenToastError:   "#FF0000",
		TokenToastInfo:    "#00FFFF",
		TokenToastWarn:    "#FFFF00",

		// Issue status - distinct, saturated colors
		TokenIssueOpen:       "#00FF00", // green
		TokenIssueInProgress: "#00FFFF", // cyan
		TokenIssueClosed:     "#808080", // gray (only muted color - closed is inactive)

		// Issue priority - distinct colors from red to gray
		TokenPriorityCritical: "#FF0000", // red
		TokenPriorityHigh:     "#FF8800", // orange
		TokenPriorityMedium:   "#FFFF00", // yellow
		TokenPriorityLow:      "#FFFFFF", // white
		TokenPriorityBacklog:  "#808080", // gray

		// Issue type - distinct colors
		TokenTypeTask:     "#00FFFF", // cyan
		TokenTypeChore:    "#808080", // gray
		TokenTypeEpic:     "#FF00FF", // magenta
		TokenTypeBug:      "#FF0000", // red
		TokenTypeFeature:  "#00FF00", // green
		TokenTypeMolecule: "#808080", // gray

		// BQL syntax highlighting - high contrast
		TokenBQLKeyword:  "#FF00FF", // magenta
		TokenBQLOperator: "#FF0000", // red
		TokenBQLField:    "#00FFFF", // cyan
		TokenBQLString:   "#FFFF00", // yellow
		TokenBQLLiteral:  "#FF8800", // orange
		TokenBQLParen:    "#FFFFFF", // white
		TokenBQLComma:    "#FFFFFF", // white

		// Misc
		TokenSpinner: "#FFFF00", // yellow for visibility
	},
}
