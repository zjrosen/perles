package vimtextarea

import "github.com/charmbracelet/lipgloss"

// SyntaxToken represents a styled region within a line of text.
// Tokens are used by syntax highlighters to apply visual styling during rendering.
type SyntaxToken struct {
	// Start is the starting byte offset within the line (0-indexed).
	Start int

	// End is the ending byte offset within the line (exclusive, like Go slices).
	// For example, to highlight "hello" at the beginning of a line: Start=0, End=5.
	End int

	// Style is the lipgloss style to apply to this token's text.
	Style lipgloss.Style
}

// SyntaxLexer tokenizes text for syntax highlighting.
// Implementations must produce tokens that satisfy the following contract:
//   - Tokens MUST be non-overlapping
//   - Tokens MUST be sorted by Start position (ascending)
//   - Gaps between tokens render as plain text
//   - Empty slice means no highlighting for this line
//
// Example for the line "status = open":
//
//	[]SyntaxToken{
//	    {Start: 0, End: 6, Style: fieldStyle},    // "status"
//	    {Start: 7, End: 8, Style: operatorStyle}, // "="
//	}
//
// The gap at positions 6-7 (space) and 8-12 ("open") render as plain text.
type SyntaxLexer interface {
	// Tokenize returns styled tokens for a single line of text.
	// Returns nil or an empty slice for lines with no highlighting.
	Tokenize(line string) []SyntaxToken
}
