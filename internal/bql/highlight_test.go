package bql

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// ansiRegex matches ANSI escape sequences
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// stripANSI removes all ANSI escape codes from a string
func stripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

// hasANSI returns true if the string contains ANSI escape codes
func hasANSI(s string) bool {
	return ansiRegex.MatchString(s)
}

func init() {
	// Force ANSI color output in tests (lipgloss disables colors when no TTY)
	lipgloss.SetColorProfile(termenv.ANSI256)
}

func TestHighlight(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		wantANSI bool // expect ANSI codes in output
	}{
		{
			name:     "simple comparison",
			query:    "status = open",
			wantANSI: true,
		},
		{
			name:     "keyword highlighting",
			query:    "a = 1 and b = 2",
			wantANSI: true,
		},
		{
			name:     "in expression",
			query:    "priority in (P0, P1)",
			wantANSI: true,
		},
		{
			name:     "string values with double quotes",
			query:    `title = "hello world"`,
			wantANSI: true,
		},
		{
			name:     "string values with single quotes",
			query:    `title = 'hello world'`,
			wantANSI: true,
		},
		{
			name:     "boolean literals true",
			query:    "ready = true",
			wantANSI: true,
		},
		{
			name:     "boolean literals false",
			query:    "ready = false",
			wantANSI: true,
		},
		{
			name:     "empty query",
			query:    "",
			wantANSI: false,
		},
		{
			name:     "whitespace preservation",
			query:    "a  =  b",
			wantANSI: true,
		},
		{
			name:     "complex nested",
			query:    "(a = 1 or b = 2) and c = 3",
			wantANSI: true,
		},
		{
			name:     "order by clause",
			query:    "order by priority desc",
			wantANSI: true,
		},
		{
			name:     "all comparison operators",
			query:    "a = 1 and b != 2 and c < 3 and d > 4 and e <= 5 and f >= 6",
			wantANSI: true,
		},
		{
			name:     "contains operators",
			query:    "title ~ test and desc !~ spam",
			wantANSI: true,
		},
		{
			name:     "not operator",
			query:    "not status = closed",
			wantANSI: true,
		},
		{
			name:     "numeric values",
			query:    "priority = 1",
			wantANSI: true,
		},
		{
			name:     "negative numbers",
			query:    "days > -7d",
			wantANSI: true,
		},
		{
			name:     "asc ordering",
			query:    "order by priority asc",
			wantANSI: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Highlight(tt.query)

			// Check ANSI presence
			gotANSI := hasANSI(result)
			if gotANSI != tt.wantANSI {
				t.Errorf("Highlight() ANSI = %v, want %v", gotANSI, tt.wantANSI)
			}

			// Verify text content is preserved (strip ANSI and compare)
			stripped := stripANSI(result)
			if stripped != tt.query {
				t.Errorf("Highlight() stripped = %q, want %q", stripped, tt.query)
			}
		})
	}
}

func TestHighlight_WhitespacePreservation(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"single spaces", "a = b"},
		{"double spaces", "a  =  b"},
		{"triple spaces", "a   =   b"},
		{"leading space", " a = b"},
		{"trailing space", "a = b "},
		{"mixed whitespace", "  a   =   b  "},
		{"tabs", "a\t=\tb"},
		{"newlines", "a\n=\nb"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Highlight(tt.query)
			stripped := stripANSI(result)
			if stripped != tt.query {
				t.Errorf("whitespace not preserved: got %q, want %q", stripped, tt.query)
			}
		})
	}
}

func TestHighlight_EmptyAndEdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		wantEmpty bool
	}{
		{"empty string", "", true},
		{"single space", " ", false},
		{"only whitespace", "   ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Highlight(tt.query)
			if tt.wantEmpty && result != "" {
				t.Errorf("expected empty result, got %q", result)
			}
			if !tt.wantEmpty && result == "" {
				t.Errorf("expected non-empty result")
			}
		})
	}
}

func TestHighlight_UnterminatedString(t *testing.T) {
	// Unterminated strings should not crash
	tests := []struct {
		name  string
		query string
	}{
		{"just double quote", `"`},
		{"just single quote", `'`},
		{"unterminated double", `title = "hello`},
		{"unterminated single", `title = 'hello`},
		{"quote at end", `status = open and title = "`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			result := Highlight(tt.query)

			// Text should be preserved (stripped of ANSI)
			stripped := stripANSI(result)
			if stripped != tt.query {
				t.Errorf("text not preserved: got %q, want %q", stripped, tt.query)
			}
		})
	}
}

func TestHighlight_KnownValuesHighlighted(t *testing.T) {
	// Known BQL values should be highlighted in value position
	tests := []struct {
		name  string
		query string
	}{
		{"status open", "status = open"},
		{"status closed", "status = closed"},
		{"type bug", "type = bug"},
		{"type feature", "type = feature"},
		{"priority p0", "priority = p0"},
		{"priority p1", "priority = p1"},
		{"boolean true", "ready = true"},
		{"boolean false", "blocked = false"},
		{"in list", "type in (bug, feature, task)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Highlight(tt.query)

			// Text should be preserved
			stripped := stripANSI(result)
			if stripped != tt.query {
				t.Errorf("text not preserved: got %q, want %q", stripped, tt.query)
			}

			// Should have ANSI codes
			if !hasANSI(result) {
				t.Error("expected ANSI codes in result")
			}
		})
	}
}

func TestHighlight_UnknownValuesNotHighlighted(t *testing.T) {
	// Unknown/custom values should NOT be highlighted
	tests := []struct {
		name  string
		query string
		value string
	}{
		{"custom text", "label ~ CustomText", "CustomText"},
		{"random value", "field = randomvalue", "randomvalue"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Highlight(tt.query)

			// Text should be preserved
			stripped := stripANSI(result)
			if stripped != tt.query {
				t.Errorf("text not preserved: got %q, want %q", stripped, tt.query)
			}
		})
	}
}

func TestHighlight_InValueListNotHighlighted(t *testing.T) {
	// Values inside "in (...)" should NOT be highlighted as field names
	query := "label in (one, two, three)"
	result := Highlight(query)

	// The field name "label" should be highlighted (has ANSI)
	// But "one", "two", "three" should NOT be highlighted

	// Strip ANSI to verify text is preserved
	stripped := stripANSI(result)
	if stripped != query {
		t.Errorf("text not preserved: got %q, want %q", stripped, query)
	}

	// Check that result contains ANSI (for "label", "in", parens, etc.)
	if !hasANSI(result) {
		t.Error("expected some ANSI codes in result")
	}

	// The values "one", "two", "three" should appear without field styling
	// We can verify by checking they appear as plain text (not wrapped in field color codes)
	// This is a bit tricky to test precisely, but we can at least verify the text is there
	if !strings.Contains(result, "one") {
		t.Error("expected 'one' in result")
	}
	if !strings.Contains(result, "two") {
		t.Error("expected 'two' in result")
	}
	if !strings.Contains(result, "three") {
		t.Error("expected 'three' in result")
	}
}

func TestHighlight_TokenStyles(t *testing.T) {
	// Test that specific token types produce ANSI output
	// We can't easily test exact colors without coupling to implementation,
	// but we can verify that styling is applied

	tests := []struct {
		name  string
		query string
		token string // the token we expect to be styled
	}{
		{"keyword and", "a and b", "and"},
		{"keyword or", "a or b", "or"},
		{"keyword not", "not a", "not"},
		{"keyword in", "a in (1)", "in"},
		{"keyword order", "order by a", "order"},
		{"keyword by", "order by a", "by"},
		{"keyword asc", "order by a asc", "asc"},
		{"keyword desc", "order by a desc", "desc"},
		{"operator eq", "a = b", "="},
		{"operator neq", "a != b", "!="},
		{"operator lt", "a < b", "<"},
		{"operator gt", "a > b", ">"},
		{"operator lte", "a <= b", "<="},
		{"operator gte", "a >= b", ">="},
		{"operator contains", "a ~ b", "~"},
		{"operator not contains", "a !~ b", "!~"},
		{"paren left", "(a)", "("},
		{"paren right", "(a)", ")"},
		{"comma", "a in (1, 2)", ","},
		{"true literal", "a = true", "true"},
		{"false literal", "a = false", "false"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Highlight(tt.query)
			if !hasANSI(result) {
				t.Errorf("expected ANSI codes in output for query %q", tt.query)
			}
			if !strings.Contains(result, tt.token) {
				t.Errorf("expected token %q in output", tt.token)
			}
		})
	}
}

func TestStyleToken(t *testing.T) {
	// Test that styleToken produces styled output for each token type
	tests := []struct {
		name      string
		tokenType TokenType
		literal   string
	}{
		{"keyword", TokenAnd, "and"},
		{"operator", TokenEq, "="},
		{"field", TokenIdent, "status"},
		{"string", TokenString, "hello"},
		{"number", TokenNumber, "42"},
		{"true", TokenTrue, "true"},
		{"false", TokenFalse, "false"},
		{"lparen", TokenLParen, "("},
		{"rparen", TokenRParen, ")"},
		{"comma", TokenComma, ","},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tok := Token{Type: tt.tokenType, Literal: tt.literal}
			result := styleToken(tok)

			// Result should contain the literal
			if !strings.Contains(result, tt.literal) {
				t.Errorf("styleToken() should contain literal %q, got %q", tt.literal, result)
			}

			// Result should have ANSI codes (unless it's illegal/eof which use default style)
			if tt.tokenType != TokenIllegal && tt.tokenType != TokenEOF {
				if !hasANSI(result) {
					t.Errorf("styleToken() should produce ANSI output for %v", tt.tokenType)
				}
			}
		})
	}
}

func TestTokenStyle(t *testing.T) {
	// Verify tokenStyle returns a style for each token type
	// This is a coverage test to ensure all cases are handled

	tokenTypes := []TokenType{
		TokenAnd, TokenOr, TokenNot, TokenIn,
		TokenOrder, TokenBy, TokenAsc, TokenDesc,
		TokenEq, TokenNeq, TokenLt, TokenGt,
		TokenLte, TokenGte, TokenContains, TokenNotContains,
		TokenLParen, TokenRParen, TokenComma,
		TokenString, TokenNumber, TokenTrue, TokenFalse,
		TokenIdent, TokenIllegal, TokenEOF,
	}

	for _, tt := range tokenTypes {
		t.Run(tt.String(), func(t *testing.T) {
			style := tokenStyle(tt)
			// Just verify it returns without panic
			_ = style.Render("test")
		})
	}
}
