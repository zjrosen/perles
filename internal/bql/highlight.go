package bql

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Highlight applies syntax highlighting to a BQL query string.
// Returns the query with ANSI color codes applied based on token types.
// Empty strings return empty strings. Invalid/partial queries are highlighted
// for their valid portions.
func Highlight(query string) string {
	if query == "" {
		return ""
	}

	lexer := NewLexer(query)
	var result strings.Builder
	lastPos := 0

	// Track context for smart identifier highlighting
	// After comparison operators, identifiers are values, not field names
	// After "in (" identifiers are values until ")"
	inValueList := false
	afterOperator := false
	prevToken := TokenEOF

	for {
		tok := lexer.NextToken()
		if tok.Type == TokenEOF {
			break
		}

		// Track when we enter/exit value list context (after "in (")
		if tok.Type == TokenLParen && prevToken == TokenIn {
			inValueList = true
		} else if tok.Type == TokenRParen && inValueList {
			inValueList = false
		}

		// Track when we're after a comparison operator (value context)
		// Reset on logical operators (and, or, not) which start a new expression
		switch tok.Type {
		case TokenEq, TokenNeq, TokenLt, TokenGt, TokenLte, TokenGte,
			TokenContains, TokenNotContains:
			afterOperator = true
		case TokenAnd, TokenOr, TokenNot, TokenOrder:
			afterOperator = false
		}

		// The lexer's Pos is 1-indexed (points to next position after current char),
		// so we adjust to get the actual 0-indexed position
		tokenPos := tok.Pos - 1

		// Preserve whitespace between tokens
		if tokenPos > lastPos {
			result.WriteString(query[lastPos:tokenPos])
		}

		// Apply style based on token type
		// For string tokens, we need to include the quotes from the original input
		// since the lexer strips them from the literal
		var tokenLen int
		if tok.Type == TokenString {
			// String tokens: position points to opening quote, literal doesn't include quotes
			// We need to render the full quoted string from the original
			// Handle unterminated strings (e.g., just `"`) by bounds checking
			tokenLen = len(tok.Literal) + 2 // +2 for quotes (if terminated)
			endPos := tokenPos + tokenLen
			if endPos > len(query) {
				// Unterminated string - just use what's available
				endPos = len(query)
				tokenLen = endPos - tokenPos
			}
			styled := styleToken(Token{Type: tok.Type, Literal: query[tokenPos:endPos]})
			result.WriteString(styled)
		} else if tok.Type == TokenIdent && (inValueList || afterOperator) {
			// Identifiers after operators or inside "in (...)" are values, not field names
			// Render them unstyled (plain text)
			tokenLen = len(tok.Literal)
			result.WriteString(tok.Literal)
			afterOperator = false // consumed the value
		} else {
			tokenLen = len(tok.Literal)
			styled := styleToken(tok)
			result.WriteString(styled)
		}

		lastPos = tokenPos + tokenLen
		prevToken = tok.Type
	}

	// Append any trailing content (whitespace after last token)
	if lastPos < len(query) {
		result.WriteString(query[lastPos:])
	}

	return result.String()
}

// styleToken returns the styled string for a token.
func styleToken(tok Token) string {
	style := tokenStyle(tok.Type)
	return style.Render(tok.Literal)
}

// tokenStyle returns the appropriate style for a token type.
func tokenStyle(t TokenType) lipgloss.Style {
	switch t {
	// Keywords
	case TokenAnd, TokenOr, TokenNot, TokenIn,
		TokenOrder, TokenBy, TokenAsc, TokenDesc:
		return KeywordStyle

	// Comparison operators
	case TokenEq, TokenNeq, TokenLt, TokenGt,
		TokenLte, TokenGte, TokenContains, TokenNotContains:
		return OperatorStyle

	// Delimiters
	case TokenLParen, TokenRParen:
		return ParenStyle
	case TokenComma:
		return CommaStyle

	// Values
	case TokenString:
		return StringStyle
	case TokenNumber, TokenTrue, TokenFalse:
		return LiteralStyle

	// Identifiers (field names)
	case TokenIdent:
		return FieldStyle

	default:
		return DefaultStyle
	}
}
