package vimtextarea

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/require"
)

// mockLexer is a test implementation of SyntaxLexer.
type mockLexer struct {
	tokens []SyntaxToken
}

// Tokenize returns the pre-configured tokens for testing.
func (m *mockLexer) Tokenize(line string) []SyntaxToken {
	return m.tokens
}

// ============================================================================
// Unit Tests: SyntaxLexer interface
// ============================================================================

func TestMockLexer_ImplementsSyntaxLexer(t *testing.T) {
	// Verify that mockLexer implements the SyntaxLexer interface at compile time.
	var _ SyntaxLexer = (*mockLexer)(nil)
}

func TestMockLexer_ReturnsConfiguredTokens(t *testing.T) {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	tokens := []SyntaxToken{
		{Start: 0, End: 5, Style: style},
	}
	lexer := &mockLexer{tokens: tokens}

	result := lexer.Tokenize("hello")

	require.Len(t, result, 1)
	require.Equal(t, 0, result[0].Start)
	require.Equal(t, 5, result[0].End)
}

// ============================================================================
// Unit Tests: SyntaxToken struct
// ============================================================================

func TestSyntaxToken_FieldsAccessible(t *testing.T) {
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("2")).
		Bold(true)

	token := SyntaxToken{
		Start: 10,
		End:   20,
		Style: style,
	}

	// Verify all fields are correctly set and accessible
	require.Equal(t, 10, token.Start)
	require.Equal(t, 20, token.End)
	require.NotEmpty(t, token.Style.Render("test"))
}

func TestSyntaxToken_StartEndByteOffsets(t *testing.T) {
	// Verify that Start/End work as byte offsets for slicing
	line := "hello world"
	token := SyntaxToken{
		Start: 0,
		End:   5,
		Style: lipgloss.NewStyle(),
	}

	// Token should extract "hello" using Go slice semantics
	extracted := line[token.Start:token.End]
	require.Equal(t, "hello", extracted)
}

// ============================================================================
// Edge Case Tests
// ============================================================================

func TestSyntaxLexer_EmptySliceReturn(t *testing.T) {
	// A lexer returning an empty slice means no highlighting for the line
	lexer := &mockLexer{tokens: []SyntaxToken{}}

	result := lexer.Tokenize("no highlighting")

	require.Empty(t, result)
	require.Len(t, result, 0)
}

func TestSyntaxLexer_NilSliceReturn(t *testing.T) {
	// A lexer returning nil is equivalent to no highlighting
	lexer := &mockLexer{tokens: nil}

	result := lexer.Tokenize("also no highlighting")

	require.Nil(t, result)
}

func TestSyntaxLexer_SingleTokenEntireLine(t *testing.T) {
	// Single token covering the entire line
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	line := "keyword"
	tokens := []SyntaxToken{
		{Start: 0, End: len(line), Style: style},
	}
	lexer := &mockLexer{tokens: tokens}

	result := lexer.Tokenize(line)

	require.Len(t, result, 1)
	require.Equal(t, 0, result[0].Start)
	require.Equal(t, 7, result[0].End) // "keyword" is 7 bytes
	// Verify the token covers the entire line
	require.Equal(t, line, line[result[0].Start:result[0].End])
}

func TestSyntaxLexer_MultipleTokensWithGaps(t *testing.T) {
	// Multiple tokens with gaps (gaps render as plain text per contract)
	keywordStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
	operatorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("5"))

	// For line "status = open":
	// - "status" at 0-6 (keyword)
	// - " " at 6-7 (gap - plain text)
	// - "=" at 7-8 (operator)
	// - " open" at 8-13 (gap - plain text)
	tokens := []SyntaxToken{
		{Start: 0, End: 6, Style: keywordStyle},  // "status"
		{Start: 7, End: 8, Style: operatorStyle}, // "="
	}
	lexer := &mockLexer{tokens: tokens}
	line := "status = open"

	result := lexer.Tokenize(line)

	require.Len(t, result, 2)

	// First token: "status"
	require.Equal(t, 0, result[0].Start)
	require.Equal(t, 6, result[0].End)
	require.Equal(t, "status", line[result[0].Start:result[0].End])

	// Second token: "="
	require.Equal(t, 7, result[1].Start)
	require.Equal(t, 8, result[1].End)
	require.Equal(t, "=", line[result[1].Start:result[1].End])

	// Verify tokens are sorted by Start (ascending)
	require.Less(t, result[0].Start, result[1].Start)
}

// ============================================================================
// Unit Tests: Model.SetLexer() and Model.Lexer()
// ============================================================================

func TestModel_SetLexer_StoresLexerCorrectly(t *testing.T) {
	m := New(Config{})
	lexer := &mockLexer{tokens: []SyntaxToken{}}

	m.SetLexer(lexer)

	require.Same(t, lexer, m.Lexer())
}

func TestModel_Lexer_ReturnsConfiguredLexer(t *testing.T) {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	tokens := []SyntaxToken{{Start: 0, End: 5, Style: style}}
	lexer := &mockLexer{tokens: tokens}

	m := New(Config{})
	m.SetLexer(lexer)

	result := m.Lexer()
	require.NotNil(t, result)

	// Verify the lexer works correctly
	returnedTokens := result.Tokenize("hello")
	require.Len(t, returnedTokens, 1)
	require.Equal(t, 0, returnedTokens[0].Start)
	require.Equal(t, 5, returnedTokens[0].End)
}

func TestModel_SetLexerNil_DisablesHighlighting(t *testing.T) {
	// First set a lexer, then set nil to disable
	m := New(Config{})
	lexer := &mockLexer{tokens: []SyntaxToken{}}
	m.SetLexer(lexer)

	// Verify lexer is set
	require.NotNil(t, m.Lexer())

	// Disable by setting nil
	m.SetLexer(nil)

	require.Nil(t, m.Lexer())
}

func TestModel_DefaultLexerIsNil(t *testing.T) {
	// New Model instances should have nil lexer by default (no highlighting)
	m := New(Config{})

	require.Nil(t, m.Lexer())
}

// ============================================================================
// Unit Tests: mapTokensToSegment()
// ============================================================================

func TestMapTokensToSegment_EmptyTokens_ReturnsNil(t *testing.T) {
	m := New(Config{})

	result := m.mapTokensToSegment([]SyntaxToken{}, 0, 10)

	require.Nil(t, result)
}

func TestMapTokensToSegment_ZeroSegmentLen_ReturnsNil(t *testing.T) {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	tokens := []SyntaxToken{{Start: 0, End: 5, Style: style}}
	m := New(Config{})

	result := m.mapTokensToSegment(tokens, 0, 0)

	require.Nil(t, result)
}

func TestMapTokensToSegment_TokenFullyInSegment_MapsCorrectly(t *testing.T) {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	// Token at positions 0-5 in logical line
	tokens := []SyntaxToken{{Start: 0, End: 5, Style: style}}
	m := New(Config{})
	m.width = 20

	// Segment starts at column 0, length 10
	result := m.mapTokensToSegment(tokens, 0, 10)

	require.Len(t, result, 1)
	require.Equal(t, 0, result[0].Start)
	require.Equal(t, 5, result[0].End)
}

func TestMapTokensToSegment_TokenPartiallyInSegment_ClampsBounds(t *testing.T) {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	// Token at positions 5-15 in logical line
	tokens := []SyntaxToken{{Start: 5, End: 15, Style: style}}
	m := New(Config{})
	m.width = 10

	// First segment: columns 0-10
	result := m.mapTokensToSegment(tokens, 0, 10)

	require.Len(t, result, 1)
	require.Equal(t, 5, result[0].Start) // Token starts at 5 in first segment
	require.Equal(t, 10, result[0].End)  // Clamped to segment end
}

func TestMapTokensToSegment_TokenInSecondSegment_MapsCorrectly(t *testing.T) {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	// Token at positions 15-20 in logical line
	tokens := []SyntaxToken{{Start: 15, End: 20, Style: style}}
	m := New(Config{})
	m.width = 10

	// Second segment: columns 10-20 (wrapStartCol=10)
	result := m.mapTokensToSegment(tokens, 10, 10)

	require.Len(t, result, 1)
	require.Equal(t, 5, result[0].Start) // 15-10=5 in segment coords
	require.Equal(t, 10, result[0].End)  // 20-10=10, clamped to segment len
}

func TestMapTokensToSegment_TokenBeforeSegment_Skipped(t *testing.T) {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	// Token at positions 0-5 in logical line
	tokens := []SyntaxToken{{Start: 0, End: 5, Style: style}}
	m := New(Config{})
	m.width = 10

	// Second segment: columns 10-20 (token is before this segment)
	result := m.mapTokensToSegment(tokens, 10, 10)

	require.Nil(t, result)
}

func TestMapTokensToSegment_TokenAfterSegment_Skipped(t *testing.T) {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	// Token at positions 20-25 in logical line
	tokens := []SyntaxToken{{Start: 20, End: 25, Style: style}}
	m := New(Config{})
	m.width = 10

	// First segment: columns 0-10 (token is after this segment)
	result := m.mapTokensToSegment(tokens, 0, 10)

	require.Nil(t, result)
}

func TestMapTokensToSegment_TokenSpansWrapBoundary_SplitAcrossSegments(t *testing.T) {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	// Token at positions 5-15 spans the boundary at column 10
	tokens := []SyntaxToken{{Start: 5, End: 15, Style: style}}
	m := New(Config{})
	m.width = 10

	// First segment: columns 0-10
	result1 := m.mapTokensToSegment(tokens, 0, 10)
	require.Len(t, result1, 1)
	require.Equal(t, 5, result1[0].Start) // Token starts at 5
	require.Equal(t, 10, result1[0].End)  // Clamped to segment end

	// Second segment: columns 10-20
	result2 := m.mapTokensToSegment(tokens, 10, 10)
	require.Len(t, result2, 1)
	require.Equal(t, 0, result2[0].Start) // Token continues from segment start
	require.Equal(t, 5, result2[0].End)   // Token ends at 15, mapped to 15-10=5
}

// ============================================================================
// Unit Tests: applySyntaxToSegment()
// ============================================================================

func TestApplySyntaxToSegment_NilLexer_ReturnsUnchanged(t *testing.T) {
	m := New(Config{})
	m.SetValue("hello world test")
	m.width = 10

	result := m.applySyntaxToSegment("hello worl", 0, 0, 0)

	require.Equal(t, "hello worl", result)
}

func TestApplySyntaxToSegment_EmptySegment_ReturnsEmpty(t *testing.T) {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	lexer := &mockLexer{tokens: []SyntaxToken{{Start: 0, End: 5, Style: style}}}
	m := New(Config{})
	m.SetLexer(lexer)
	m.SetValue("hello")
	m.width = 10

	result := m.applySyntaxToSegment("", 0, 0, 0)

	require.Equal(t, "", result)
}

func TestApplySyntaxToSegment_WithTokens_AppliesStyles(t *testing.T) {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	// Token "hello" at 0-5
	lexer := &mockLexer{tokens: []SyntaxToken{{Start: 0, End: 5, Style: style}}}
	m := New(Config{})
	m.SetLexer(lexer)
	m.SetValue("hello world")
	m.width = 20

	result := m.applySyntaxToSegment("hello world", 0, 0, 0)

	// Should contain ANSI escape sequences
	require.Contains(t, result, "\x1b[")
	require.Contains(t, result, "hello")
	require.Contains(t, result, "world")
}

// ============================================================================
// Integration Tests: Syntax highlighting with cursor rendering
// ============================================================================

func TestRenderLineWithSyntaxAndCursor_NilLexer_UsesSimpleRendering(t *testing.T) {
	m := New(Config{})
	m.SetValue("hello")
	m.Focus()
	m.width = 20

	result := m.renderLineWithSyntaxAndCursor("hello", 0, 0, 0)

	// Should have cursor at first position (reverse video)
	require.Contains(t, result, cursorOn)
	require.Contains(t, result, cursorOff)
}

func TestRenderLineWithSyntaxAndCursor_WithLexer_AppliesBothSyntaxAndCursor(t *testing.T) {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	// Token "hello" at 0-5
	lexer := &mockLexer{tokens: []SyntaxToken{{Start: 0, End: 5, Style: style}}}
	m := New(Config{})
	m.SetLexer(lexer)
	m.SetValue("hello world")
	m.Focus()
	m.width = 20

	// Cursor at position 6 (on space between hello and world)
	result := m.renderLineWithSyntaxAndCursor("hello world", 0, 0, 6)

	// Should have both cursor and syntax highlighting
	require.Contains(t, result, cursorOn)
	require.Contains(t, result, cursorOff)
	// Should have ANSI codes for syntax highlighting
	require.Contains(t, result, "\x1b[")
}

func TestRenderLineWithSyntaxAndCursor_CursorAtEnd_ShowsBlockCursor(t *testing.T) {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	lexer := &mockLexer{tokens: []SyntaxToken{{Start: 0, End: 5, Style: style}}}
	m := New(Config{})
	m.SetLexer(lexer)
	m.SetValue("hello")
	m.Focus()
	m.width = 20

	// Cursor at end of line
	result := m.renderLineWithSyntaxAndCursor("hello", 0, 0, 5)

	// Should have cursor block at end
	require.Contains(t, result, cursorOn+" "+cursorOff)
}

func TestRenderLineWithSyntaxAndCursor_EmptyLine_ShowsCursorBlock(t *testing.T) {
	lexer := &mockLexer{tokens: []SyntaxToken{}}
	m := New(Config{})
	m.SetLexer(lexer)
	m.SetValue("")
	m.Focus()
	m.width = 20

	result := m.renderLineWithSyntaxAndCursor("", 0, 0, 0)

	// Should show cursor block for empty line
	require.Equal(t, cursorOn+" "+cursorOff, result)
}

// ============================================================================
// Integration Tests: Syntax highlighting with selection rendering
// ============================================================================

func TestRenderWithSelection_SyntaxAppliedOutsideSelection(t *testing.T) {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	// Tokens: "hello" (0-5), "world" (6-11)
	lexer := &mockLexer{tokens: []SyntaxToken{
		{Start: 0, End: 5, Style: style},
		{Start: 6, End: 11, Style: style},
	}}
	m := New(Config{VimEnabled: true, DefaultMode: ModeVisual})
	m.SetLexer(lexer)
	m.SetValue("hello world test")
	m.Focus()
	m.width = 50
	// Set up visual selection from 6-11 ("world")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 6}
	m.cursorRow = 0
	m.cursorCol = 10

	result := m.renderWrappedLineWithSelection("hello world test", 0, 0, 10, true, 0)

	// Should contain selection markers
	require.Contains(t, result, selectionOn)
	require.Contains(t, result, selectionOff)
	// Should contain syntax highlighting (outside selection)
	require.Contains(t, result, "\x1b[")
}
