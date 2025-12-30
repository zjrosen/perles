package bql

import (
	"github.com/zjrosen/perles/internal/ui/shared/vimtextarea"

	"github.com/charmbracelet/lipgloss"
)

// BQLSyntaxLexer adapts the BQL lexer to vimtextarea's SyntaxLexer interface.
// It provides context-aware syntax highlighting that distinguishes field names
// from values based on their position relative to operators.
type BQLSyntaxLexer struct{}

// NewSyntaxLexer creates a new BQL syntax lexer for vimtextarea.
func NewSyntaxLexer() *BQLSyntaxLexer {
	return &BQLSyntaxLexer{}
}

// Tokenize implements vimtextarea.SyntaxLexer.
// It returns styled tokens for a single line of BQL text.
func (l *BQLSyntaxLexer) Tokenize(line string) []vimtextarea.SyntaxToken {
	if line == "" {
		return nil
	}

	lexer := NewLexer(line)
	var tokens []vimtextarea.SyntaxToken

	// Track context for smart identifier highlighting (reused from highlight.go)
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

		// Get style for this token
		style := l.styleForToken(tok.Type, inValueList, afterOperator)
		if style == nil {
			// Skip unstyled tokens (values after operators)
			afterOperator = false
			prevToken = tok.Type
			continue
		}

		// BQL lexer's tok.Pos is set at the start of NextToken() to l.pos,
		// which points to the position AFTER the current character (1-indexed).
		// So tok.Pos - 1 gives the 0-indexed start position of the token.
		start := tok.Pos - 1

		// Handle string tokens which include quotes in the original but not in Literal
		if tok.Type == TokenString {
			// The lexer's readString skips the opening quote before recording start,
			// and tok.Pos was set BEFORE readString was called (pointing after the opening quote).
			// tok.Pos points to position of opening quote + 1, so the opening quote is at tok.Pos - 1.
			// But wait - readString is called with the quote char, so tok.Pos is set before that.
			// Let me trace: when ch='"', tok.Pos = l.pos (position after '"'), then readString is called.
			// So tok.Pos - 1 points to the opening quote. That's our start!
			// The end should be start + len(Literal) + 2 (for both quotes)
			end := min(start+len(tok.Literal)+2, len(line)) // Handle unterminated strings
			tokens = append(tokens, vimtextarea.SyntaxToken{
				Start: start,
				End:   end,
				Style: *style,
			})
		} else {
			end := min(start+len(tok.Literal), len(line))
			tokens = append(tokens, vimtextarea.SyntaxToken{
				Start: start,
				End:   end,
				Style: *style,
			})
		}

		// Reset afterOperator only when we've consumed a value (numbers, strings, booleans, identifiers)
		// Note: identifiers that ARE values (afterOperator=true) were already skipped via continue above
		switch tok.Type {
		case TokenNumber, TokenString, TokenTrue, TokenFalse:
			afterOperator = false
		}

		prevToken = tok.Type
	}

	return tokens
}

// styleForToken returns the lipgloss style for a token type.
// Returns nil for tokens that should not be styled (values after operators).
func (l *BQLSyntaxLexer) styleForToken(t TokenType, inValueList, afterOperator bool) *lipgloss.Style {
	// Identifiers after operators or in value lists are values, not field names
	if t == TokenIdent && (inValueList || afterOperator) {
		return nil // Plain text
	}

	switch t {
	// Keywords
	case TokenAnd, TokenOr, TokenNot, TokenIn,
		TokenOrder, TokenBy, TokenAsc, TokenDesc,
		TokenExpand, TokenDepth:
		return &KeywordStyle

	// Comparison operators
	case TokenEq, TokenNeq, TokenLt, TokenGt,
		TokenLte, TokenGte, TokenContains, TokenNotContains:
		return &OperatorStyle

	// Special operators (for expand depth *)
	case TokenStar:
		return &OperatorStyle

	// Delimiters
	case TokenLParen, TokenRParen:
		return &ParenStyle
	case TokenComma:
		return &CommaStyle

	// Values
	case TokenString:
		return &StringStyle
	case TokenNumber, TokenTrue, TokenFalse:
		return &LiteralStyle

	// Field names (identifiers not in value context)
	case TokenIdent:
		return &FieldStyle

	default:
		return nil
	}
}
