// Package styles contains Lip Gloss style definitions.
package styles

import (
	"github.com/zjrosen/perles/internal/beads"

	"github.com/charmbracelet/lipgloss"
)

var (
	// Semantic color names - Text hierarchy
	TextPrimaryColor     = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#CCCCCC"} // Main/primary text
	TextSecondaryColor   = lipgloss.AdaptiveColor{Light: "#AAAAAA", Dark: "#BBBBBB"} // Issue IDs, secondary info
	TextMutedColor       = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#696969"} // Hints, help text, footers
	TextDescriptionColor = lipgloss.AdaptiveColor{Light: "#666666", Dark: "#999999"} // Description/body text
	TextPlaceholderColor = lipgloss.AdaptiveColor{Light: "#666666", Dark: "#777777"} // Input placeholders

	// Semantic color names - Border
	BorderDefaultColor = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#696969"} // Unfocused borders

	// Semantic color names - Status
	StatusSuccessColor = lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#73F59F"} // Success states
	StatusWarningColor = lipgloss.AdaptiveColor{Light: "#FECA57", Dark: "#FECA57"} // Warnings
	StatusErrorColor   = lipgloss.AdaptiveColor{Light: "#FF6B6B", Dark: "#FF8787"} // Errors

	// Selection indicator color (used for ">" prefix in lists)
	SelectionIndicatorColor = lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#FFFFFF"}

	// Button colors
	ButtonTextColor             = lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#FFFFFF"}
	ButtonPrimaryBgColor        = lipgloss.AdaptiveColor{Light: "#1A5276", Dark: "#1A5276"}
	ButtonPrimaryFocusBgColor   = lipgloss.AdaptiveColor{Light: "#3498DB", Dark: "#3498DB"}
	ButtonSecondaryBgColor      = lipgloss.AdaptiveColor{Light: "#2D3436", Dark: "#2D3436"}
	ButtonSecondaryFocusBgColor = lipgloss.AdaptiveColor{Light: "#636E72", Dark: "#636E72"}
	ButtonDangerBgColor         = lipgloss.AdaptiveColor{Light: "#922B21", Dark: "#922B21"}
	ButtonDangerFocusBgColor    = lipgloss.AdaptiveColor{Light: "#E74C3C", Dark: "#E74C3C"}
	ButtonDisabledBgColor       = lipgloss.AdaptiveColor{Light: "#2D2D2D", Dark: "#2D2D2D"}

	// BQL syntax highlighting colors (Catppuccin Mocha)
	BQLKeywordColor  = lipgloss.AdaptiveColor{Light: "#8839EF", Dark: "#CBA6F7"} // mauve
	BQLOperatorColor = lipgloss.AdaptiveColor{Light: "#D20F39", Dark: "#F38BA8"} // red
	BQLFieldColor    = lipgloss.AdaptiveColor{Light: "#179299", Dark: "#94E2D5"} // teal
	BQLStringColor   = lipgloss.AdaptiveColor{Light: "#DF8E1D", Dark: "#F9E2AF"} // yellow
	BQLLiteralColor  = lipgloss.AdaptiveColor{Light: "#FE640B", Dark: "#FAB387"} // peach
	BQLParenColor    = lipgloss.AdaptiveColor{Light: "#1E66F5", Dark: "#89B4FA"} // blue
	BQLCommaColor    = lipgloss.AdaptiveColor{Light: "#9CA0B0", Dark: "#6C7086"} // overlay0

	// Selection indicator style (used for ">" prefix in lists: picker, column, search, etc.)
	SelectionIndicatorStyle = lipgloss.NewStyle().Bold(true).Foreground(SelectionIndicatorColor)

	// Button colors
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

	// Form colors
	FormTextInputBorderColor        = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#8C8C8C"}
	FormTextInputFocusedBorderColor = lipgloss.AdaptiveColor{Light: "#FFF", Dark: "#FFF"}
	FormTextInputLabelColor         = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#8C8C8C"}
	FormTextInputFocusedLabelColor  = lipgloss.AdaptiveColor{Light: "#FFF", Dark: "#FFF"}

	// Overlay colors
	OverlayTitleColor         = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#C9C9C9"}
	OverlayBorderColor        = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#8C8C8C"}
	BorderHighlightFocusColor = lipgloss.AdaptiveColor{Light: "#54A0FF", Dark: "#54A0FF"}

	// Toast notification colors
	ToastBorderSuccessColor = lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#73F59F"}
	ToastBorderErrorColor   = lipgloss.AdaptiveColor{Light: "#FF6B6B", Dark: "#FF8787"}
	ToastBorderInfoColor    = lipgloss.AdaptiveColor{Light: "#54A0FF", Dark: "#54A0FF"}
	ToastBorderWarnColor    = lipgloss.AdaptiveColor{Light: "#FECA57", Dark: "#FECA57"}

	// Issue status colors
	StatusOpenColor       = lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#73F59F"}
	StatusInProgressColor = lipgloss.AdaptiveColor{Light: "#54A0FF", Dark: "#54A0FF"}
	StatusClosedColor     = lipgloss.AdaptiveColor{Light: "#AAAAAA", Dark: "#BBBBBB"}

	// Issue priority colors
	PriorityCriticalColor = lipgloss.AdaptiveColor{Light: "#FF6B6B", Dark: "#FF8787"}
	PriorityHighColor     = lipgloss.AdaptiveColor{Light: "#FF9F43", Dark: "#FF9F43"}
	PriorityMediumColor   = lipgloss.AdaptiveColor{Light: "#FECA57", Dark: "#FECA57"}
	PriorityLowColor      = lipgloss.AdaptiveColor{Light: "#666666", Dark: "#999999"}
	PriorityBacklogColor  = lipgloss.AdaptiveColor{Light: "#999999", Dark: "#666666"}

	PriorityCriticalStyle = lipgloss.NewStyle().Foreground(PriorityCriticalColor).Bold(true)
	PriorityHighStyle     = lipgloss.NewStyle().Foreground(PriorityHighColor)
	PriorityMediumStyle   = lipgloss.NewStyle().Foreground(PriorityMediumColor)
	PriorityLowStyle      = lipgloss.NewStyle().Foreground(PriorityLowColor)
	PriorityBacklogStyle  = lipgloss.NewStyle().Foreground(PriorityBacklogColor)

	// Issue type colors
	IssueTaskColor     = lipgloss.AdaptiveColor{Light: "#54A0FF", Dark: "#54A0FF"}
	IssueChoreColor    = lipgloss.AdaptiveColor{Light: "#888888", Dark: "#777777"}
	IssueEpicColor     = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}
	IssueBugColor      = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}
	IssueFeatureColor  = lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#73F59F"}
	IssueMoleculeColor = lipgloss.AdaptiveColor{Light: "#FF731A", Dark: "#FF731A"}

	TypeBugStyle      = lipgloss.NewStyle().Foreground(StatusErrorColor)
	TypeFeatureStyle  = lipgloss.NewStyle().Foreground(IssueFeatureColor)
	TypeTaskStyle     = lipgloss.NewStyle().Foreground(IssueTaskColor)
	TypeEpicStyle     = lipgloss.NewStyle().Foreground(IssueEpicColor)
	TypeChoreStyle    = lipgloss.NewStyle().Foreground(IssueChoreColor)
	TypeMoleculeStyle = lipgloss.NewStyle().Foreground(IssueMoleculeColor)

	// Status bar
	StatusBarStyle = lipgloss.NewStyle().
			Foreground(TextSecondaryColor).
			Padding(0, 1)

	// Error display
	ErrorStyle = lipgloss.NewStyle().
			Foreground(StatusErrorColor).
			Bold(true).
			Padding(1, 2)

	// Loading spinner color
	SpinnerColor = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#FFF"}

	// Vim mode indicator colors
	VimNormalModeColor  = lipgloss.AdaptiveColor{Light: "#1E66F5", Dark: "#89B4FA"} // blue
	VimInsertModeColor  = lipgloss.AdaptiveColor{Light: "#40A02B", Dark: "#A6E3A1"} // green
	VimVisualModeColor  = lipgloss.AdaptiveColor{Light: "#8839EF", Dark: "#CBA6F7"} // mauve/purple
	VimReplaceModeColor = lipgloss.AdaptiveColor{Light: "#FE640B", Dark: "#FAB387"} // peach/orange - danger/overwrite
)

// GetTypeIndicator returns the letter indicator for an issue type.
func GetTypeIndicator(t beads.IssueType) string {
	switch t {
	case beads.TypeBug:
		return "[B]"
	case beads.TypeFeature:
		return "[F]"
	case beads.TypeTask:
		return "[T]"
	case beads.TypeEpic:
		return "[E]"
	case beads.TypeChore:
		return "[C]"
	case beads.TypeMolecule:
		return "[M]"
	default:
		return "[?]"
	}
}

// GetTypeStyle returns the style for an issue type.
func GetTypeStyle(t beads.IssueType) lipgloss.Style {
	switch t {
	case beads.TypeBug:
		return TypeBugStyle
	case beads.TypeFeature:
		return TypeFeatureStyle
	case beads.TypeTask:
		return TypeTaskStyle
	case beads.TypeEpic:
		return TypeEpicStyle
	case beads.TypeChore:
		return TypeChoreStyle
	case beads.TypeMolecule:
		return TypeMoleculeStyle
	default:
		return lipgloss.NewStyle()
	}
}

// GetPriorityStyle returns the style for a priority level.
func GetPriorityStyle(p beads.Priority) lipgloss.Style {
	switch p {
	case beads.PriorityCritical:
		return PriorityCriticalStyle
	case beads.PriorityHigh:
		return PriorityHighStyle
	case beads.PriorityMedium:
		return PriorityMediumStyle
	case beads.PriorityLow:
		return PriorityLowStyle
	case beads.PriorityBacklog:
		return PriorityBacklogStyle
	default:
		return lipgloss.NewStyle()
	}
}

// Legacy ApplyTheme with simple signature is now in apply.go
// The new ApplyTheme(cfg ThemeConfig) provides full theme support.
