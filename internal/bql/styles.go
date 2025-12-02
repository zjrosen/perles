package bql

import (
	"perles/internal/ui/styles"

	"github.com/charmbracelet/lipgloss"
)

// Token highlight styles for BQL syntax highlighting.
// Uses centralized color constants from the styles package.
var (
	// KeywordStyle for logical operators: and, or, not, in, order, by, asc, desc
	KeywordStyle = lipgloss.NewStyle().
			Foreground(styles.BQLKeywordColor).
			Bold(true)

	// OperatorStyle for comparison operators: =, !=, <, >, <=, >=, ~, !~
	OperatorStyle = lipgloss.NewStyle().
			Foreground(styles.BQLOperatorColor)

	// FieldStyle for field names (identifiers)
	FieldStyle = lipgloss.NewStyle().
			Foreground(styles.BQLFieldColor)

	// StringStyle for quoted string values
	StringStyle = lipgloss.NewStyle().
			Foreground(styles.BQLStringColor)

	// LiteralStyle for boolean and numeric values: true, false, numbers
	LiteralStyle = lipgloss.NewStyle().
			Foreground(styles.BQLLiteralColor)

	// ParenStyle for parentheses
	ParenStyle = lipgloss.NewStyle().
			Foreground(styles.BQLParenColor).
			Bold(true)

	// CommaStyle for comma separators
	CommaStyle = lipgloss.NewStyle().
			Foreground(styles.BQLCommaColor)

	// DefaultStyle for unrecognized tokens
	DefaultStyle = lipgloss.NewStyle()
)
