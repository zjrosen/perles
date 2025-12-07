package bql

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLexer_BasicTokens(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []Token
	}{
		{
			name:  "simple equality",
			input: "type = task",
			expected: []Token{
				{Type: TokenIdent, Literal: "type", Pos: 1},
				{Type: TokenEq, Literal: "=", Pos: 6},
				{Type: TokenIdent, Literal: "task", Pos: 8},
				{Type: TokenEOF, Literal: "", Pos: 12},
			},
		},
		{
			name:  "comparison operators",
			input: "priority < P2",
			expected: []Token{
				{Type: TokenIdent, Literal: "priority", Pos: 1},
				{Type: TokenLt, Literal: "<", Pos: 10},
				{Type: TokenIdent, Literal: "P2", Pos: 12},
				{Type: TokenEOF, Literal: "", Pos: 14},
			},
		},
		{
			name:  "in expression",
			input: "status in (open, closed)",
			expected: []Token{
				{Type: TokenIdent, Literal: "status", Pos: 1},
				{Type: TokenIn, Literal: "in", Pos: 8},
				{Type: TokenLParen, Literal: "(", Pos: 11},
				{Type: TokenIdent, Literal: "open", Pos: 12},
				{Type: TokenComma, Literal: ",", Pos: 16},
				{Type: TokenIdent, Literal: "closed", Pos: 18},
				{Type: TokenRParen, Literal: ")", Pos: 24},
				{Type: TokenEOF, Literal: "", Pos: 25},
			},
		},
		{
			name:  "and/or keywords",
			input: "type = bug and priority = P0",
			expected: []Token{
				{Type: TokenIdent, Literal: "type", Pos: 1},
				{Type: TokenEq, Literal: "=", Pos: 6},
				{Type: TokenIdent, Literal: "bug", Pos: 8},
				{Type: TokenAnd, Literal: "and", Pos: 12},
				{Type: TokenIdent, Literal: "priority", Pos: 16},
				{Type: TokenEq, Literal: "=", Pos: 25},
				{Type: TokenIdent, Literal: "P0", Pos: 27},
				{Type: TokenEOF, Literal: "", Pos: 29},
			},
		},
		{
			name:  "not keyword",
			input: "not blocked = true",
			expected: []Token{
				{Type: TokenNot, Literal: "not", Pos: 1},
				{Type: TokenIdent, Literal: "blocked", Pos: 5},
				{Type: TokenEq, Literal: "=", Pos: 13},
				{Type: TokenTrue, Literal: "true", Pos: 15},
				{Type: TokenEOF, Literal: "", Pos: 19},
			},
		},
		{
			name:  "order by clause",
			input: "order by created desc",
			expected: []Token{
				{Type: TokenOrder, Literal: "order", Pos: 1},
				{Type: TokenBy, Literal: "by", Pos: 7},
				{Type: TokenIdent, Literal: "created", Pos: 10},
				{Type: TokenDesc, Literal: "desc", Pos: 18},
				{Type: TokenEOF, Literal: "", Pos: 22},
			},
		},
		{
			name:  "contains operator",
			input: "title ~ auth",
			expected: []Token{
				{Type: TokenIdent, Literal: "title", Pos: 1},
				{Type: TokenContains, Literal: "~", Pos: 7},
				{Type: TokenIdent, Literal: "auth", Pos: 9},
				{Type: TokenEOF, Literal: "", Pos: 13},
			},
		},
		{
			name:  "not contains operator",
			input: "title !~ test",
			expected: []Token{
				{Type: TokenIdent, Literal: "title", Pos: 1},
				{Type: TokenNotContains, Literal: "!~", Pos: 7},
				{Type: TokenIdent, Literal: "test", Pos: 10},
				{Type: TokenEOF, Literal: "", Pos: 14},
			},
		},
		{
			name:  "not equals operator",
			input: "priority != P4",
			expected: []Token{
				{Type: TokenIdent, Literal: "priority", Pos: 1},
				{Type: TokenNeq, Literal: "!=", Pos: 10},
				{Type: TokenIdent, Literal: "P4", Pos: 13},
				{Type: TokenEOF, Literal: "", Pos: 15},
			},
		},
		{
			name:  "less than or equal",
			input: "priority <= P1",
			expected: []Token{
				{Type: TokenIdent, Literal: "priority", Pos: 1},
				{Type: TokenLte, Literal: "<=", Pos: 10},
				{Type: TokenIdent, Literal: "P1", Pos: 13},
				{Type: TokenEOF, Literal: "", Pos: 15},
			},
		},
		{
			name:  "greater than or equal",
			input: "updated >= yesterday",
			expected: []Token{
				{Type: TokenIdent, Literal: "updated", Pos: 1},
				{Type: TokenGte, Literal: ">=", Pos: 9},
				{Type: TokenIdent, Literal: "yesterday", Pos: 12},
				{Type: TokenEOF, Literal: "", Pos: 21},
			},
		},
		{
			name:  "quoted string double",
			input: `title = "hello world"`,
			expected: []Token{
				{Type: TokenIdent, Literal: "title", Pos: 1},
				{Type: TokenEq, Literal: "=", Pos: 7},
				{Type: TokenString, Literal: "hello world", Pos: 9},
				{Type: TokenEOF, Literal: "", Pos: 22},
			},
		},
		{
			name:  "quoted string single",
			input: `title = 'hello world'`,
			expected: []Token{
				{Type: TokenIdent, Literal: "title", Pos: 1},
				{Type: TokenEq, Literal: "=", Pos: 7},
				{Type: TokenString, Literal: "hello world", Pos: 9},
				{Type: TokenEOF, Literal: "", Pos: 22},
			},
		},
		{
			name:  "negative date offset",
			input: "created > -7d",
			expected: []Token{
				{Type: TokenIdent, Literal: "created", Pos: 1},
				{Type: TokenGt, Literal: ">", Pos: 9},
				{Type: TokenNumber, Literal: "-7d", Pos: 11},
				{Type: TokenEOF, Literal: "", Pos: 14},
			},
		},
		{
			name:  "hour offset",
			input: "created > -24h",
			expected: []Token{
				{Type: TokenIdent, Literal: "created", Pos: 1},
				{Type: TokenGt, Literal: ">", Pos: 9},
				{Type: TokenNumber, Literal: "-24h", Pos: 11},
				{Type: TokenEOF, Literal: "", Pos: 15},
			},
		},
		{
			name:  "single hour offset",
			input: "updated >= -1h",
			expected: []Token{
				{Type: TokenIdent, Literal: "updated", Pos: 1},
				{Type: TokenGte, Literal: ">=", Pos: 9},
				{Type: TokenNumber, Literal: "-1h", Pos: 12},
				{Type: TokenEOF, Literal: "", Pos: 15},
			},
		},
		{
			name:  "month offset",
			input: "created > -3m",
			expected: []Token{
				{Type: TokenIdent, Literal: "created", Pos: 1},
				{Type: TokenGt, Literal: ">", Pos: 9},
				{Type: TokenNumber, Literal: "-3m", Pos: 11},
				{Type: TokenEOF, Literal: "", Pos: 14},
			},
		},
		{
			name:  "single month offset",
			input: "updated >= -1m",
			expected: []Token{
				{Type: TokenIdent, Literal: "updated", Pos: 1},
				{Type: TokenGte, Literal: ">=", Pos: 9},
				{Type: TokenNumber, Literal: "-1m", Pos: 12},
				{Type: TokenEOF, Literal: "", Pos: 15},
			},
		},
		{
			name:  "case insensitive keywords",
			input: "type = bug AND priority = P0 OR status = open",
			expected: []Token{
				{Type: TokenIdent, Literal: "type", Pos: 1},
				{Type: TokenEq, Literal: "=", Pos: 6},
				{Type: TokenIdent, Literal: "bug", Pos: 8},
				{Type: TokenAnd, Literal: "AND", Pos: 12},
				{Type: TokenIdent, Literal: "priority", Pos: 16},
				{Type: TokenEq, Literal: "=", Pos: 25},
				{Type: TokenIdent, Literal: "P0", Pos: 27},
				{Type: TokenOr, Literal: "OR", Pos: 30},
				{Type: TokenIdent, Literal: "status", Pos: 33},
				{Type: TokenEq, Literal: "=", Pos: 40},
				{Type: TokenIdent, Literal: "open", Pos: 42},
				{Type: TokenEOF, Literal: "", Pos: 46},
			},
		},
		{
			name:  "parentheses for grouping",
			input: "(type = bug or type = task)",
			expected: []Token{
				{Type: TokenLParen, Literal: "(", Pos: 1},
				{Type: TokenIdent, Literal: "type", Pos: 2},
				{Type: TokenEq, Literal: "=", Pos: 7},
				{Type: TokenIdent, Literal: "bug", Pos: 9},
				{Type: TokenOr, Literal: "or", Pos: 13},
				{Type: TokenIdent, Literal: "type", Pos: 16},
				{Type: TokenEq, Literal: "=", Pos: 21},
				{Type: TokenIdent, Literal: "task", Pos: 23},
				{Type: TokenRParen, Literal: ")", Pos: 27},
				{Type: TokenEOF, Literal: "", Pos: 28},
			},
		},
		{
			name:  "boolean false",
			input: "blocked = false",
			expected: []Token{
				{Type: TokenIdent, Literal: "blocked", Pos: 1},
				{Type: TokenEq, Literal: "=", Pos: 9},
				{Type: TokenFalse, Literal: "false", Pos: 11},
				{Type: TokenEOF, Literal: "", Pos: 16},
			},
		},
		{
			name:     "empty input",
			input:    "",
			expected: []Token{{Type: TokenEOF, Literal: "", Pos: 1}},
		},
		{
			name:     "whitespace only",
			input:    "   \t\n  ",
			expected: []Token{{Type: TokenEOF, Literal: "", Pos: 8}},
		},
		{
			name:  "identifier with hyphen",
			input: "id = perles-123",
			expected: []Token{
				{Type: TokenIdent, Literal: "id", Pos: 1},
				{Type: TokenEq, Literal: "=", Pos: 4},
				{Type: TokenIdent, Literal: "perles-123", Pos: 6},
				{Type: TokenEOF, Literal: "", Pos: 16},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			for i, expected := range tt.expected {
				tok := lexer.NextToken()
				require.Equal(t, expected.Type, tok.Type, "token %d type mismatch", i)
				require.Equal(t, expected.Literal, tok.Literal, "token %d literal mismatch", i)
			}
		})
	}
}

func TestLexer_AllOperators(t *testing.T) {
	operators := map[string]TokenType{
		"=":  TokenEq,
		"!=": TokenNeq,
		"<":  TokenLt,
		">":  TokenGt,
		"<=": TokenLte,
		">=": TokenGte,
		"~":  TokenContains,
		"!~": TokenNotContains,
	}

	for op, expected := range operators {
		t.Run(op, func(t *testing.T) {
			lexer := NewLexer("field " + op + " value")
			lexer.NextToken() // skip field
			tok := lexer.NextToken()
			require.Equal(t, expected, tok.Type)
			require.Equal(t, op, tok.Literal)
		})
	}
}

func TestLexer_AllKeywords(t *testing.T) {
	keywords := map[string]TokenType{
		"and":    TokenAnd,
		"AND":    TokenAnd,
		"And":    TokenAnd,
		"or":     TokenOr,
		"OR":     TokenOr,
		"Or":     TokenOr,
		"not":    TokenNot,
		"NOT":    TokenNot,
		"in":     TokenIn,
		"IN":     TokenIn,
		"order":  TokenOrder,
		"ORDER":  TokenOrder,
		"by":     TokenBy,
		"BY":     TokenBy,
		"asc":    TokenAsc,
		"ASC":    TokenAsc,
		"desc":   TokenDesc,
		"DESC":   TokenDesc,
		"true":   TokenTrue,
		"TRUE":   TokenTrue,
		"false":  TokenFalse,
		"FALSE":  TokenFalse,
		"expand": TokenExpand,
		"EXPAND": TokenExpand,
		"Expand": TokenExpand,
		"depth":  TokenDepth,
		"DEPTH":  TokenDepth,
		"Depth":  TokenDepth,
	}

	for kw, expected := range keywords {
		t.Run(kw, func(t *testing.T) {
			lexer := NewLexer(kw)
			tok := lexer.NextToken()
			require.Equal(t, expected, tok.Type)
		})
	}
}

func TestLexer_ExpandKeyword(t *testing.T) {
	l := NewLexer("expand children depth 2")

	tok := l.NextToken()
	require.Equal(t, TokenExpand, tok.Type)
	require.Equal(t, "expand", tok.Literal)

	tok = l.NextToken()
	require.Equal(t, TokenIdent, tok.Type)
	require.Equal(t, "children", tok.Literal)

	tok = l.NextToken()
	require.Equal(t, TokenDepth, tok.Type)
	require.Equal(t, "depth", tok.Literal)

	tok = l.NextToken()
	require.Equal(t, TokenNumber, tok.Type)
	require.Equal(t, "2", tok.Literal)

	tok = l.NextToken()
	require.Equal(t, TokenEOF, tok.Type)
}

func TestLexer_ExpandUnlimitedDepth(t *testing.T) {
	l := NewLexer("expand all depth *")

	tok := l.NextToken()
	require.Equal(t, TokenExpand, tok.Type)
	require.Equal(t, "expand", tok.Literal)

	tok = l.NextToken()
	require.Equal(t, TokenIdent, tok.Type)
	require.Equal(t, "all", tok.Literal)

	tok = l.NextToken()
	require.Equal(t, TokenDepth, tok.Type)
	require.Equal(t, "depth", tok.Literal)

	tok = l.NextToken()
	require.Equal(t, TokenStar, tok.Type)
	require.Equal(t, "*", tok.Literal)

	tok = l.NextToken()
	require.Equal(t, TokenEOF, tok.Type)
}

func TestLexer_ExtendedIdentifierChars(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"dot - semver", "v1.2.3", "v1.2.3"},
		{"slash - path", "feature/auth", "feature/auth"},
		{"at - mention", "@user", "@user"},
		{"hash - issue ref", "issue#123", "issue#123"},
		{"plus - compound", "C++", "C++"},
		{"semver with prerelease", "v1.0.0-beta+build", "v1.0.0-beta+build"},
		{"all special chars", "a_b-c:d.e/f@g#h+i", "a_b-c:d.e/f@g#h+i"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			tok := lexer.NextToken()
			require.Equal(t, TokenIdent, tok.Type)
			require.Equal(t, tt.expected, tok.Literal)
		})
	}
}

func TestLexer_IdentifierStopsAtOperator(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		firstIdent    string
		secondTokType TokenType
	}{
		{"stops at equals", "label=value", "label", TokenEq},
		{"stops at greater than", "v1.0.0>v2.0.0", "v1.0.0", TokenGt},
		{"stops at less than", "a/b<c/d", "a/b", TokenLt},
		{"stops at not equals", "v1.0.0!=v2.0.0", "v1.0.0", TokenNeq},
		{"stops at contains", "path/to~search", "path/to", TokenContains},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			tok := lexer.NextToken()
			require.Equal(t, TokenIdent, tok.Type)
			require.Equal(t, tt.firstIdent, tok.Literal)

			tok = lexer.NextToken()
			require.Equal(t, tt.secondTokType, tok.Type)
		})
	}
}

func TestLexer_ExpandWithFilter(t *testing.T) {
	l := NewLexer("type = epic expand children")

	tok := l.NextToken()
	require.Equal(t, TokenIdent, tok.Type)
	require.Equal(t, "type", tok.Literal)

	tok = l.NextToken()
	require.Equal(t, TokenEq, tok.Type)

	tok = l.NextToken()
	require.Equal(t, TokenIdent, tok.Type)
	require.Equal(t, "epic", tok.Literal)

	tok = l.NextToken()
	require.Equal(t, TokenExpand, tok.Type)
	require.Equal(t, "expand", tok.Literal)

	tok = l.NextToken()
	require.Equal(t, TokenIdent, tok.Type)
	require.Equal(t, "children", tok.Literal)

	tok = l.NextToken()
	require.Equal(t, TokenEOF, tok.Type)
}
