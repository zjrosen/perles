// Package styles contains Lip Gloss style definitions.
package styles

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/zjrosen/perles/internal/task"
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

	// Selection background color (used to highlight selected items in lists)
	SelectionBackgroundColor = lipgloss.AdaptiveColor{Light: "#1A5276", Dark: "#1A5276"}

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
	StatusDeferredColor   = lipgloss.AdaptiveColor{Light: "#9B59B6", Dark: "#B07CC6"}
	StatusBlockedColor    = lipgloss.AdaptiveColor{Light: "#FF6B6B", Dark: "#FF8787"}

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
	IssueTaskColor      = lipgloss.AdaptiveColor{Light: "#54A0FF", Dark: "#54A0FF"}
	IssueChoreColor     = lipgloss.AdaptiveColor{Light: "#888888", Dark: "#777777"}
	IssueEpicColor      = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}
	IssueBugColor       = lipgloss.AdaptiveColor{Light: "#FF6B6B", Dark: "#FF8787"}
	IssueFeatureColor   = lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#73F59F"}
	IssueMilestoneColor = lipgloss.AdaptiveColor{Light: "#E8A838", Dark: "#F0B84A"}
	IssueStoryColor     = lipgloss.AdaptiveColor{Light: "#2DD4BF", Dark: "#2DD4BF"}
	IssueSpikeColor     = lipgloss.AdaptiveColor{Light: "#E06C75", Dark: "#E06C75"}
	IssueMoleculeColor  = lipgloss.AdaptiveColor{Light: "#FF731A", Dark: "#FF731A"}
	IssueConvoyColor    = lipgloss.AdaptiveColor{Light: "#888888", Dark: "#888888"}
	IssueAgentColor     = lipgloss.AdaptiveColor{Light: "#5C6BC0", Dark: "#5C6BC0"}

	TypeBugStyle       = lipgloss.NewStyle().Foreground(IssueBugColor)
	TypeFeatureStyle   = lipgloss.NewStyle().Foreground(IssueFeatureColor)
	TypeTaskStyle      = lipgloss.NewStyle().Foreground(IssueTaskColor)
	TypeEpicStyle      = lipgloss.NewStyle().Foreground(IssueEpicColor)
	TypeChoreStyle     = lipgloss.NewStyle().Foreground(IssueChoreColor)
	TypeMilestoneStyle = lipgloss.NewStyle().Foreground(IssueMilestoneColor)
	TypeStoryStyle     = lipgloss.NewStyle().Foreground(IssueStoryColor)
	TypeSpikeStyle     = lipgloss.NewStyle().Foreground(IssueSpikeColor)
	TypeMoleculeStyle  = lipgloss.NewStyle().Foreground(IssueMoleculeColor)
	TypeConvoyStyle    = lipgloss.NewStyle().Foreground(IssueConvoyColor)
	TypeAgentStyle     = lipgloss.NewStyle().Foreground(IssueAgentColor)

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

	// Diff syntax highlighting colors
	DiffAdditionColor = lipgloss.AdaptiveColor{Light: "#40A02B", Dark: "#A6E3A1"} // green
	DiffDeletionColor = lipgloss.AdaptiveColor{Light: "#D20F39", Dark: "#F38BA8"} // red
	DiffContextColor  = lipgloss.AdaptiveColor{Light: "#666666", Dark: "#888888"} // gray
	DiffHunkColor     = lipgloss.AdaptiveColor{Light: "#1E66F5", Dark: "#89B4FA"} // blue

	// Word-level diff highlight background colors (from master plan v2 Appendix A)
	DiffWordAdditionBgColor = lipgloss.AdaptiveColor{Light: "#2d4a2d", Dark: "#2d4a2d"} // green background
	DiffWordDeletionBgColor = lipgloss.AdaptiveColor{Light: "#4a2d2d", Dark: "#4a2d2d"} // red background
)

// GetTypeIndicator returns the letter indicator for an issue type.
func GetTypeIndicator(t task.IssueType) string {
	switch t {
	case task.TypeBug:
		return "[B]"
	case task.TypeFeature:
		return "[F]"
	case task.TypeTask:
		return "[T]"
	case task.TypeEpic:
		return "[E]"
	case task.TypeChore:
		return "[C]"
	case task.TypeMilestone:
		return "[M]"
	case task.TypeStory:
		return "[S]"
	case task.TypeSpike:
		return "[Sp]"
	case task.IssueType("molecule"):
		return "[Mo]"
	case task.IssueType("convoy"):
		return "[🚚]"
	case task.IssueType("agent"):
		return "[👨‍💼]"
	default:
		return "[?]"
	}
}

// GetTypeStyle returns the style for an issue type.
func GetTypeStyle(t task.IssueType) lipgloss.Style {
	switch t {
	case task.TypeBug:
		return TypeBugStyle
	case task.TypeFeature:
		return TypeFeatureStyle
	case task.TypeTask:
		return TypeTaskStyle
	case task.TypeEpic:
		return TypeEpicStyle
	case task.TypeChore:
		return TypeChoreStyle
	case task.TypeMilestone:
		return TypeMilestoneStyle
	case task.TypeStory:
		return TypeStoryStyle
	case task.TypeSpike:
		return TypeSpikeStyle
	case task.IssueType("molecule"):
		return TypeMoleculeStyle
	case task.IssueType("convoy"):
		return TypeConvoyStyle
	case task.IssueType("agent"):
		return TypeAgentStyle
	default:
		return lipgloss.NewStyle()
	}
}

// GetPriorityStyle returns the style for a priority level.
func GetPriorityStyle(p task.Priority) lipgloss.Style {
	switch p {
	case task.PriorityCritical:
		return PriorityCriticalStyle
	case task.PriorityHigh:
		return PriorityHighStyle
	case task.PriorityMedium:
		return PriorityMediumStyle
	case task.PriorityLow:
		return PriorityLowStyle
	case task.PriorityBacklog:
		return PriorityBacklogStyle
	default:
		return lipgloss.NewStyle()
	}
}

// Legacy ApplyTheme with simple signature is now in apply.go
// The new ApplyTheme(cfg ThemeConfig) provides full theme support.
