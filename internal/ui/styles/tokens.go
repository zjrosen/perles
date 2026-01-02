// Package styles contains Lip Gloss style definitions.
package styles

// ColorToken represents a named, themeable color.
type ColorToken string

// Color tokens organized by category.
// These are the keys users can override in their config.
const (
	// Text hierarchy
	TokenTextPrimary     ColorToken = "text.primary"
	TokenTextSecondary   ColorToken = "text.secondary"
	TokenTextMuted       ColorToken = "text.muted"
	TokenTextDescription ColorToken = "text.description"
	TokenTextPlaceholder ColorToken = "text.placeholder"

	// Borders
	TokenBorderDefault   ColorToken = "border.default"
	TokenBorderFocus     ColorToken = "border.focus"
	TokenBorderHighlight ColorToken = "border.highlight"

	// Status indicators
	TokenStatusSuccess ColorToken = "status.success"
	TokenStatusWarning ColorToken = "status.warning"
	TokenStatusError   ColorToken = "status.error"

	// Selection
	TokenSelectionIndicator ColorToken = "selection.indicator"

	// Buttons
	TokenButtonText             ColorToken = "button.text"
	TokenButtonPrimaryBg        ColorToken = "button.primary.bg"
	TokenButtonPrimaryFocusBg   ColorToken = "button.primary.focus"
	TokenButtonSecondaryBg      ColorToken = "button.secondary.bg"
	TokenButtonSecondaryFocusBg ColorToken = "button.secondary.focus"
	TokenButtonDangerBg         ColorToken = "button.danger.bg"
	TokenButtonDangerFocusBg    ColorToken = "button.danger.focus"
	TokenButtonDisabledBg       ColorToken = "button.disabled.bg"

	// Forms
	TokenFormBorder      ColorToken = "form.border"
	TokenFormBorderFocus ColorToken = "form.border.focus" //nolint:gosec // UI color token, not credentials
	TokenFormLabel       ColorToken = "form.label"
	TokenFormLabelFocus  ColorToken = "form.label.focus"

	// Overlays/Modals
	TokenOverlayTitle  ColorToken = "overlay.title"
	TokenOverlayBorder ColorToken = "overlay.border"

	// Toast notifications
	TokenToastSuccess ColorToken = "toast.success"
	TokenToastError   ColorToken = "toast.error"
	TokenToastInfo    ColorToken = "toast.info"
	TokenToastWarn    ColorToken = "toast.warn"

	// Issue status
	TokenIssueOpen       ColorToken = "issue.status.open" //nolint:gosec // UI color token, not credentials
	TokenIssueInProgress ColorToken = "issue.status.in_progress"
	TokenIssueClosed     ColorToken = "issue.status.closed" //nolint:gosec // UI color token, not credentials

	// Issue priority
	TokenPriorityCritical ColorToken = "priority.critical"
	TokenPriorityHigh     ColorToken = "priority.high"
	TokenPriorityMedium   ColorToken = "priority.medium"
	TokenPriorityLow      ColorToken = "priority.low"
	TokenPriorityBacklog  ColorToken = "priority.backlog"

	// Issue type
	TokenTypeTask     ColorToken = "type.task"
	TokenTypeChore    ColorToken = "type.chore"
	TokenTypeEpic     ColorToken = "type.epic"
	TokenTypeBug      ColorToken = "type.bug"
	TokenTypeFeature  ColorToken = "type.feature"
	TokenTypeMolecule ColorToken = "type.molecule"
	TokenTypeConvoy   ColorToken = "type.convoy"

	// BQL syntax highlighting
	TokenBQLKeyword  ColorToken = "bql.keyword" //nolint:gosec // UI color token, not credentials
	TokenBQLOperator ColorToken = "bql.operator"
	TokenBQLField    ColorToken = "bql.field"
	TokenBQLString   ColorToken = "bql.string"
	TokenBQLLiteral  ColorToken = "bql.literal"
	TokenBQLParen    ColorToken = "bql.paren" //nolint:gosec // UI color token, not credentials
	TokenBQLComma    ColorToken = "bql.comma"

	// Misc
	TokenSpinner ColorToken = "spinner"
)

// AllTokens returns all valid color tokens for validation.
func AllTokens() []ColorToken {
	return []ColorToken{
		// Text hierarchy
		TokenTextPrimary,
		TokenTextSecondary,
		TokenTextMuted,
		TokenTextDescription,
		TokenTextPlaceholder,

		// Borders
		TokenBorderDefault,
		TokenBorderFocus,
		TokenBorderHighlight,

		// Status indicators
		TokenStatusSuccess,
		TokenStatusWarning,
		TokenStatusError,

		// Selection
		TokenSelectionIndicator,

		// Buttons
		TokenButtonText,
		TokenButtonPrimaryBg,
		TokenButtonPrimaryFocusBg,
		TokenButtonSecondaryBg,
		TokenButtonSecondaryFocusBg,
		TokenButtonDangerBg,
		TokenButtonDangerFocusBg,
		TokenButtonDisabledBg,

		// Forms
		TokenFormBorder,
		TokenFormBorderFocus,
		TokenFormLabel,
		TokenFormLabelFocus,

		// Overlays/Modals
		TokenOverlayTitle,
		TokenOverlayBorder,

		// Toast notifications
		TokenToastSuccess,
		TokenToastError,
		TokenToastInfo,
		TokenToastWarn,

		// Issue status
		TokenIssueOpen,
		TokenIssueInProgress,
		TokenIssueClosed,

		// Issue priority
		TokenPriorityCritical,
		TokenPriorityHigh,
		TokenPriorityMedium,
		TokenPriorityLow,
		TokenPriorityBacklog,

		// Issue type
		TokenTypeTask,
		TokenTypeChore,
		TokenTypeEpic,
		TokenTypeBug,
		TokenTypeFeature,
		TokenTypeMolecule,
		TokenTypeConvoy,

		// BQL syntax highlighting
		TokenBQLKeyword,
		TokenBQLOperator,
		TokenBQLField,
		TokenBQLString,
		TokenBQLLiteral,
		TokenBQLParen,
		TokenBQLComma,

		// Misc
		TokenSpinner,
	}
}
