// Package playground provides a component showcase and theme token viewer.
package playground

import (
	"github.com/zjrosen/perles/internal/ui/styles"
)

// GetTokenColor returns the hex color value for a token.
// This maps ColorToken constants to their actual hex values from the styles package.
func GetTokenColor(token styles.ColorToken) string {
	switch token {
	// Text hierarchy
	case styles.TokenTextPrimary:
		return styles.TextPrimaryColor.Dark
	case styles.TokenTextSecondary:
		return styles.TextSecondaryColor.Dark
	case styles.TokenTextMuted:
		return styles.TextMutedColor.Dark
	case styles.TokenTextDescription:
		return styles.TextDescriptionColor.Dark
	case styles.TokenTextPlaceholder:
		return styles.TextPlaceholderColor.Dark

	// Borders
	case styles.TokenBorderDefault:
		return styles.BorderDefaultColor.Dark
	case styles.TokenBorderFocus:
		return styles.FormTextInputFocusedBorderColor.Dark
	case styles.TokenBorderHighlight:
		return styles.BorderHighlightFocusColor.Dark

	// Status indicators
	case styles.TokenStatusSuccess:
		return styles.StatusSuccessColor.Dark
	case styles.TokenStatusWarning:
		return styles.StatusWarningColor.Dark
	case styles.TokenStatusError:
		return styles.StatusErrorColor.Dark

	// Selection
	case styles.TokenSelectionIndicator:
		return styles.SelectionIndicatorColor.Dark
	case styles.TokenSelectionBackground:
		return styles.SelectionBackgroundColor.Dark

	// Buttons
	case styles.TokenButtonText:
		return styles.ButtonTextColor.Dark
	case styles.TokenButtonPrimaryBg:
		return styles.ButtonPrimaryBgColor.Dark
	case styles.TokenButtonPrimaryFocusBg:
		return styles.ButtonPrimaryFocusBgColor.Dark
	case styles.TokenButtonSecondaryBg:
		return styles.ButtonSecondaryBgColor.Dark
	case styles.TokenButtonSecondaryFocusBg:
		return styles.ButtonSecondaryFocusBgColor.Dark
	case styles.TokenButtonDangerBg:
		return styles.ButtonDangerBgColor.Dark
	case styles.TokenButtonDangerFocusBg:
		return styles.ButtonDangerFocusBgColor.Dark
	case styles.TokenButtonDisabledBg:
		return styles.ButtonDisabledBgColor.Dark

	// Forms
	case styles.TokenFormBorder:
		return styles.FormTextInputBorderColor.Dark
	case styles.TokenFormBorderFocus:
		return styles.FormTextInputFocusedBorderColor.Dark
	case styles.TokenFormLabel:
		return styles.FormTextInputLabelColor.Dark
	case styles.TokenFormLabelFocus:
		return styles.FormTextInputFocusedLabelColor.Dark

	// Overlays/Modals
	case styles.TokenOverlayTitle:
		return styles.OverlayTitleColor.Dark
	case styles.TokenOverlayBorder:
		return styles.OverlayBorderColor.Dark

	// Toast notifications
	case styles.TokenToastSuccess:
		return styles.ToastBorderSuccessColor.Dark
	case styles.TokenToastError:
		return styles.ToastBorderErrorColor.Dark
	case styles.TokenToastInfo:
		return styles.ToastBorderInfoColor.Dark
	case styles.TokenToastWarn:
		return styles.ToastBorderWarnColor.Dark

	// Issue status
	case styles.TokenIssueOpen:
		return styles.StatusOpenColor.Dark
	case styles.TokenIssueInProgress:
		return styles.StatusInProgressColor.Dark
	case styles.TokenIssueClosed:
		return styles.StatusClosedColor.Dark

	// Issue priority
	case styles.TokenPriorityCritical:
		return styles.PriorityCriticalColor.Dark
	case styles.TokenPriorityHigh:
		return styles.PriorityHighColor.Dark
	case styles.TokenPriorityMedium:
		return styles.PriorityMediumColor.Dark
	case styles.TokenPriorityLow:
		return styles.PriorityLowColor.Dark
	case styles.TokenPriorityBacklog:
		return styles.PriorityBacklogColor.Dark

	// Issue type
	case styles.TokenTypeTask:
		return styles.IssueTaskColor.Dark
	case styles.TokenTypeChore:
		return styles.IssueChoreColor.Dark
	case styles.TokenTypeEpic:
		return styles.IssueEpicColor.Dark
	case styles.TokenTypeBug:
		return styles.IssueBugColor.Dark
	case styles.TokenTypeFeature:
		return styles.IssueFeatureColor.Dark
	case styles.TokenTypeMolecule:
		return styles.IssueMoleculeColor.Dark
	case styles.TokenTypeConvoy:
		return styles.IssueConvoyColor.Dark
	case styles.TokenTypeAgent:
		return styles.IssueAgentColor.Dark

	// BQL syntax highlighting
	case styles.TokenBQLKeyword:
		return styles.BQLKeywordColor.Dark
	case styles.TokenBQLOperator:
		return styles.BQLOperatorColor.Dark
	case styles.TokenBQLField:
		return styles.BQLFieldColor.Dark
	case styles.TokenBQLString:
		return styles.BQLStringColor.Dark
	case styles.TokenBQLLiteral:
		return styles.BQLLiteralColor.Dark
	case styles.TokenBQLParen:
		return styles.BQLParenColor.Dark
	case styles.TokenBQLComma:
		return styles.BQLCommaColor.Dark

	// Misc
	case styles.TokenSpinner:
		return styles.SpinnerColor.Dark

	default:
		return ""
	}
}

// TokenCategory groups tokens by category for display.
type TokenCategory struct {
	Name   string
	Tokens []styles.ColorToken
}

// GetTokenCategories returns all token categories for the theme viewer.
func GetTokenCategories() []TokenCategory {
	return []TokenCategory{
		{
			Name: "Text",
			Tokens: []styles.ColorToken{
				styles.TokenTextPrimary,
				styles.TokenTextSecondary,
				styles.TokenTextMuted,
				styles.TokenTextDescription,
				styles.TokenTextPlaceholder,
			},
		},
		{
			Name: "Borders",
			Tokens: []styles.ColorToken{
				styles.TokenBorderDefault,
				styles.TokenBorderFocus,
				styles.TokenBorderHighlight,
			},
		},
		{
			Name: "Status",
			Tokens: []styles.ColorToken{
				styles.TokenStatusSuccess,
				styles.TokenStatusWarning,
				styles.TokenStatusError,
			},
		},
		{
			Name: "Selection",
			Tokens: []styles.ColorToken{
				styles.TokenSelectionIndicator,
				styles.TokenSelectionBackground,
			},
		},
		{
			Name: "Buttons",
			Tokens: []styles.ColorToken{
				styles.TokenButtonText,
				styles.TokenButtonPrimaryBg,
				styles.TokenButtonPrimaryFocusBg,
				styles.TokenButtonSecondaryBg,
				styles.TokenButtonSecondaryFocusBg,
				styles.TokenButtonDangerBg,
				styles.TokenButtonDangerFocusBg,
				styles.TokenButtonDisabledBg,
			},
		},
		{
			Name: "Forms",
			Tokens: []styles.ColorToken{
				styles.TokenFormBorder,
				styles.TokenFormBorderFocus,
				styles.TokenFormLabel,
				styles.TokenFormLabelFocus,
			},
		},
		{
			Name: "Overlays",
			Tokens: []styles.ColorToken{
				styles.TokenOverlayTitle,
				styles.TokenOverlayBorder,
			},
		},
		{
			Name: "Toast",
			Tokens: []styles.ColorToken{
				styles.TokenToastSuccess,
				styles.TokenToastError,
				styles.TokenToastInfo,
				styles.TokenToastWarn,
			},
		},
		{
			Name: "Issue Status",
			Tokens: []styles.ColorToken{
				styles.TokenIssueOpen,
				styles.TokenIssueInProgress,
				styles.TokenIssueClosed,
			},
		},
		{
			Name: "Priority",
			Tokens: []styles.ColorToken{
				styles.TokenPriorityCritical,
				styles.TokenPriorityHigh,
				styles.TokenPriorityMedium,
				styles.TokenPriorityLow,
				styles.TokenPriorityBacklog,
			},
		},
		{
			Name: "Issue Type",
			Tokens: []styles.ColorToken{
				styles.TokenTypeTask,
				styles.TokenTypeChore,
				styles.TokenTypeEpic,
				styles.TokenTypeBug,
				styles.TokenTypeFeature,
				styles.TokenTypeMolecule,
				styles.TokenTypeConvoy,
				styles.TokenTypeAgent,
			},
		},
		{
			Name: "BQL Syntax",
			Tokens: []styles.ColorToken{
				styles.TokenBQLKeyword,
				styles.TokenBQLOperator,
				styles.TokenBQLField,
				styles.TokenBQLString,
				styles.TokenBQLLiteral,
				styles.TokenBQLParen,
				styles.TokenBQLComma,
			},
		},
		{
			Name: "Misc",
			Tokens: []styles.ColorToken{
				styles.TokenSpinner,
			},
		},
	}
}
