package bql

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/ui/shared/vimtextarea"
)

// verifyInterfaceCompliance ensures BQLSyntaxLexer implements SyntaxLexer
var _ vimtextarea.SyntaxLexer = (*BQLSyntaxLexer)(nil)

func TestBQLSyntaxLexer_Tokenize(t *testing.T) {
	lexer := NewSyntaxLexer()

	tests := []struct {
		name     string
		input    string
		wantLen  int
		validate func(t *testing.T, tokens []vimtextarea.SyntaxToken)
	}{
		{
			name:    "empty string returns nil",
			input:   "",
			wantLen: 0,
			validate: func(t *testing.T, tokens []vimtextarea.SyntaxToken) {
				require.Nil(t, tokens, "expected nil for empty string")
			},
		},
		{
			name:    "simple query: status = open",
			input:   "status = open",
			wantLen: 2, // "status" (field) and "=" (operator); "open" is plain text (value after operator)
			validate: func(t *testing.T, tokens []vimtextarea.SyntaxToken) {
				// First token: "status" at position 0-6
				require.Equal(t, 0, tokens[0].Start, "token 0 Start")
				require.Equal(t, 6, tokens[0].End, "token 0 End")
				// Second token: "=" at position 7-8
				require.Equal(t, 7, tokens[1].Start, "token 1 Start")
				require.Equal(t, 8, tokens[1].End, "token 1 End")
			},
		},
		{
			name:    "keyword highlighting: status = open and priority < 2",
			input:   "status = open and priority < 2",
			wantLen: 6, // "status", "=", "and", "priority", "<", "2" (number is styled)
			validate: func(t *testing.T, tokens []vimtextarea.SyntaxToken) {
				// Check "and" keyword is present (should be around position 14-17)
				found := false
				for _, tok := range tokens {
					if tok.Start == 14 && tok.End == 17 {
						found = true
						break
					}
				}
				require.True(t, found, "keyword 'and' not found at expected position 14-17")
			},
		},
		{
			name:    "string literal highlighting",
			input:   `title ~ "bug"`,
			wantLen: 3, // "title", "~", and "\"bug\""
			validate: func(t *testing.T, tokens []vimtextarea.SyntaxToken) {
				// String token should include quotes: position 8-13
				stringTok := tokens[2]
				require.Equal(t, 8, stringTok.Start, "string token Start")
				require.Equal(t, 13, stringTok.End, "string token End")
			},
		},
		{
			name:    "token positions accuracy",
			input:   "a = 1",
			wantLen: 3, // "a", "=", "1" (number is always styled)
			validate: func(t *testing.T, tokens []vimtextarea.SyntaxToken) {
				// "a" at 0-1
				require.Equal(t, 0, tokens[0].Start, "token 0 'a' Start")
				require.Equal(t, 1, tokens[0].End, "token 0 'a' End")
				// "=" at 2-3
				require.Equal(t, 2, tokens[1].Start, "token 1 '=' Start")
				require.Equal(t, 3, tokens[1].End, "token 1 '=' End")
				// "1" at 4-5
				require.Equal(t, 4, tokens[2].Start, "token 2 '1' Start")
				require.Equal(t, 5, tokens[2].End, "token 2 '1' End")
			},
		},
		{
			name:    "context-aware field vs value: assignee = john",
			input:   "assignee = john",
			wantLen: 2, // "assignee" (field), "=" (operator); "john" is plain text
			validate: func(t *testing.T, tokens []vimtextarea.SyntaxToken) {
				// "assignee" at 0-8
				require.Equal(t, 0, tokens[0].Start, "field token Start")
				require.Equal(t, 8, tokens[0].End, "field token End")
				// Verify we don't have a token for "john" (position 11-15)
				for _, tok := range tokens {
					require.NotEqual(t, 11, tok.Start, "unexpected token at position 11 - 'john' should be plain text")
				}
			},
		},
		{
			name:    "IN clause highlighting",
			input:   "status in (open, closed)",
			wantLen: 5, // "status", "in", "(", ",", ")"
			validate: func(t *testing.T, tokens []vimtextarea.SyntaxToken) {
				// "in" keyword at position 7-9
				found := false
				for _, tok := range tokens {
					if tok.Start == 7 && tok.End == 9 {
						found = true
						break
					}
				}
				require.True(t, found, "keyword 'in' not found at expected position 7-9")
				// Values "open" and "closed" should NOT be tokenized (plain text)
				for _, tok := range tokens {
					require.False(t, tok.Start == 11 || tok.Start == 17,
						"unexpected token at position %d - values in IN clause should be plain", tok.Start)
				}
			},
		},
		{
			name:    "partial query: status =",
			input:   "status =",
			wantLen: 2, // "status" and "="
			validate: func(t *testing.T, tokens []vimtextarea.SyntaxToken) {
				require.Equal(t, 0, tokens[0].Start, "token 0 Start")
				require.Equal(t, 6, tokens[0].End, "token 0 End")
				require.Equal(t, 7, tokens[1].Start, "token 1 Start")
				require.Equal(t, 8, tokens[1].End, "token 1 End")
			},
		},
		{
			name:    "just whitespace",
			input:   "   ",
			wantLen: 0,
			validate: func(t *testing.T, tokens []vimtextarea.SyntaxToken) {
				require.Empty(t, tokens, "expected empty slice for whitespace")
			},
		},
		{
			name:    "order by clause",
			input:   "order by priority desc",
			wantLen: 4, // "order", "by", "priority" (field), "desc"
			validate: func(t *testing.T, tokens []vimtextarea.SyntaxToken) {
				// "order" at 0-5
				require.Equal(t, 0, tokens[0].Start, "'order' Start")
				require.Equal(t, 5, tokens[0].End, "'order' End")
				// "by" at 6-8
				require.Equal(t, 6, tokens[1].Start, "'by' Start")
				require.Equal(t, 8, tokens[1].End, "'by' End")
				// "desc" at 18-22
				require.Equal(t, 18, tokens[3].Start, "'desc' Start")
				require.Equal(t, 22, tokens[3].End, "'desc' End")
			},
		},
		{
			name:    "boolean literal",
			input:   "active = true",
			wantLen: 3, // "active", "=", "true" (literal is always styled)
			validate: func(t *testing.T, tokens []vimtextarea.SyntaxToken) {
				// "true" at position 9-13 should be styled
				trueTok := tokens[2]
				require.Equal(t, 9, trueTok.Start, "'true' Start")
				require.Equal(t, 13, trueTok.End, "'true' End")
			},
		},
		{
			name:    "numeric literal",
			input:   "priority < 5",
			wantLen: 3, // "priority", "<", "5" (numbers are always styled)
			validate: func(t *testing.T, tokens []vimtextarea.SyntaxToken) {
				// "5" at position 11-12
				numTok := tokens[2]
				require.Equal(t, 11, numTok.Start, "'5' Start")
				require.Equal(t, 12, numTok.End, "'5' End")
			},
		},
		{
			name:    "expand clause with star",
			input:   "expand depth *",
			wantLen: 3, // "expand", "depth", "*"
			validate: func(t *testing.T, tokens []vimtextarea.SyntaxToken) {
				// "*" at position 13-14
				require.Equal(t, 13, tokens[2].Start, "'*' Start")
				require.Equal(t, 14, tokens[2].End, "'*' End")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := lexer.Tokenize(tt.input)

			// Check token count
			if tt.wantLen > 0 {
				require.Len(t, tokens, tt.wantLen, "token count mismatch")
			}

			// Run custom validation
			if tt.validate != nil {
				tt.validate(t, tokens)
			}
		})
	}
}

func TestBQLSyntaxLexer_TokensAreSorted(t *testing.T) {
	lexer := NewSyntaxLexer()
	input := "status = open and priority < 2 order by created_at desc"

	tokens := lexer.Tokenize(input)

	// Verify tokens are sorted by Start position (ascending)
	for i := 1; i < len(tokens); i++ {
		require.GreaterOrEqual(t, tokens[i].Start, tokens[i-1].Start,
			"tokens not sorted: token[%d].Start=%d < token[%d].Start=%d",
			i, tokens[i].Start, i-1, tokens[i-1].Start)
	}
}

func TestBQLSyntaxLexer_TokensNonOverlapping(t *testing.T) {
	lexer := NewSyntaxLexer()
	input := "status in (open, closed) and priority >= 1"

	tokens := lexer.Tokenize(input)

	// Verify tokens don't overlap
	for i := 1; i < len(tokens); i++ {
		require.GreaterOrEqual(t, tokens[i].Start, tokens[i-1].End,
			"tokens overlap: token[%d] ends at %d, token[%d] starts at %d",
			i-1, tokens[i-1].End, i, tokens[i].Start)
	}
}

func TestBQLSyntaxLexer_StringWithSingleQuotes(t *testing.T) {
	lexer := NewSyntaxLexer()
	input := `title ~ 'test'`

	tokens := lexer.Tokenize(input)

	// Should have 3 tokens: "title", "~", "'test'"
	require.Len(t, tokens, 3)

	// String token with single quotes at position 8-14
	stringTok := tokens[2]
	require.Equal(t, 8, stringTok.Start, "string token Start")
	require.Equal(t, 14, stringTok.End, "string token End")
}

func TestBQLSyntaxLexer_UnterminatedString(t *testing.T) {
	lexer := NewSyntaxLexer()
	input := `title ~ "unterminated`

	tokens := lexer.Tokenize(input)

	// Should still produce tokens without panicking
	require.GreaterOrEqual(t, len(tokens), 2, "expected at least 2 tokens for partial string")

	// Verify no token extends past the input length
	for i, tok := range tokens {
		require.LessOrEqual(t, tok.End, len(input),
			"token[%d] End=%d exceeds input length %d", i, tok.End, len(input))
	}
}

func TestBQLSyntaxLexer_NotOperator(t *testing.T) {
	lexer := NewSyntaxLexer()
	input := "not status = closed"

	tokens := lexer.Tokenize(input)

	// Should have: "not", "status", "="; "closed" is plain
	require.Len(t, tokens, 3)

	// "not" at position 0-3
	require.Equal(t, 0, tokens[0].Start, "'not' Start")
	require.Equal(t, 3, tokens[0].End, "'not' End")

	// After "not", "status" should be styled as a field (afterOperator was reset)
	require.Equal(t, 4, tokens[1].Start, "'status' Start")
	require.Equal(t, 10, tokens[1].End, "'status' End")
}

func TestBQLSyntaxLexer_ContainsOperator(t *testing.T) {
	lexer := NewSyntaxLexer()
	input := "title ~ bug"

	tokens := lexer.Tokenize(input)

	// Should have: "title", "~"; "bug" is plain (value after operator)
	require.Len(t, tokens, 2)

	// "~" at position 6-7
	require.Equal(t, 6, tokens[1].Start, "'~' Start")
	require.Equal(t, 7, tokens[1].End, "'~' End")
}

func TestBQLSyntaxLexer_NotContainsOperator(t *testing.T) {
	lexer := NewSyntaxLexer()
	input := "title !~ spam"

	tokens := lexer.Tokenize(input)

	// Should have: "title", "!~"; "spam" is plain
	require.Len(t, tokens, 2)

	// "!~" at position 6-8
	require.Equal(t, 6, tokens[1].Start, "'!~' Start")
	require.Equal(t, 8, tokens[1].End, "'!~' End")
}
