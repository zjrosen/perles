package vimtextarea

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/require"
)

func init() {
	// Force ANSI color output in tests (lipgloss disables colors when no TTY)
	lipgloss.SetColorProfile(termenv.ANSI256)
}

// ============================================================================
// Unit Tests: View() returns valid string
// ============================================================================

func TestView_ReturnsValidString(t *testing.T) {
	m := New(Config{})
	view := m.View()
	require.NotNil(t, view)
}

func TestView_EmptyContent_NotFocused(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	view := m.View()

	// Should show empty content (no cursor, no mode indicator - mode is client's responsibility)
	require.Empty(t, view)
}

func TestView_EmptyContent_Focused(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.Focus()
	view := m.View()

	// Should show cursor (mode indicator is client's responsibility)
	require.Contains(t, view, cursorOn)
	require.Contains(t, view, cursorOff)
}

func TestView_WithContent_NotFocused(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello world")
	view := m.View()

	// Should show content (mode indicator is client's responsibility)
	require.Contains(t, view, "hello world")
	// Should NOT show cursor when not focused
	require.NotContains(t, view, cursorOn)
}

func TestView_WithContent_Focused(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello")
	m.Focus()
	view := m.View()

	// Should show content with cursor (mode indicator is client's responsibility)
	require.Contains(t, view, cursorOn)
}

// ============================================================================
// Unit Tests: Cursor position correct in rendered output
// ============================================================================

func TestView_CursorAtStart(t *testing.T) {
	m := New(Config{VimEnabled: true})
	m.SetValue("hello")
	m.cursorCol = 0
	m.Focus()
	view := m.View()

	// Cursor should be on 'h'
	// Expected: [reverse]h[/reverse]ello
	require.Contains(t, view, cursorOn+"h"+cursorOff+"ello")
}

func TestView_CursorInMiddle(t *testing.T) {
	m := New(Config{VimEnabled: true})
	m.SetValue("hello")
	m.cursorCol = 2
	m.Focus()
	view := m.View()

	// Cursor should be on 'l' (index 2)
	// Expected: he[reverse]l[/reverse]lo
	require.Contains(t, view, "he"+cursorOn+"l"+cursorOff+"lo")
}

func TestView_CursorAtEnd_InsertMode(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("hello")
	m.cursorCol = 5 // After the last character
	m.Focus()
	view := m.View()

	// Cursor should be after 'o' (append cursor)
	// Expected: hello[reverse] [/reverse]
	require.Contains(t, view, "hello"+cursorOn+" "+cursorOff)
}

func TestView_CursorOnEmptyLine(t *testing.T) {
	m := New(Config{VimEnabled: true})
	m.SetValue("")
	m.Focus()
	view := m.View()

	// Cursor should be a space in reverse video
	require.Contains(t, view, cursorOn+" "+cursorOff)
}

// ============================================================================
// Unit Tests: Mode tracking (clients use Mode() method)
// ============================================================================

func TestView_Mode_NormalMode(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})

	// Mode should be accessible via Mode() method
	require.Equal(t, ModeNormal, m.Mode())

	// View should NOT contain mode indicator (client's responsibility)
	view := m.View()
	require.NotContains(t, view, "[NORMAL]")
	require.NotContains(t, view, "[INSERT]")
}

func TestView_Mode_InsertMode(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})

	// Mode should be accessible via Mode() method
	require.Equal(t, ModeInsert, m.Mode())

	// View should NOT contain mode indicator (client's responsibility)
	view := m.View()
	require.NotContains(t, view, "[INSERT]")
	require.NotContains(t, view, "[NORMAL]")
}

func TestView_Mode_NoIndicatorInOutput(t *testing.T) {
	// Test that mode indicator is never in output, regardless of vim setting
	m := New(Config{VimEnabled: true})
	view := m.View()
	require.NotContains(t, view, "[NORMAL]")
	require.NotContains(t, view, "[INSERT]")
	require.NotContains(t, view, "[VISUAL]")

	m2 := New(Config{VimEnabled: false})
	view2 := m2.View()
	require.NotContains(t, view2, "[NORMAL]")
	require.NotContains(t, view2, "[INSERT]")
	require.NotContains(t, view2, "[VISUAL]")
}

// ============================================================================
// Unit Tests: Placeholder text
// ============================================================================

func TestView_Placeholder_ShownWhenEmpty(t *testing.T) {
	m := New(Config{
		VimEnabled:  false,
		Placeholder: "Enter text here...",
	})
	view := m.View()

	require.Contains(t, view, "Enter text here...")
}

func TestView_Placeholder_HiddenWhenContentPresent(t *testing.T) {
	m := New(Config{
		VimEnabled:  false,
		Placeholder: "Enter text here...",
	})
	m.SetValue("some content")
	view := m.View()

	require.NotContains(t, view, "Enter text here...")
	require.Contains(t, view, "some content")
}

func TestView_Placeholder_HiddenWhenFocused(t *testing.T) {
	m := New(Config{
		VimEnabled:  false,
		Placeholder: "Enter text here...",
	})
	m.Focus()
	view := m.View()

	// When focused, show cursor instead of placeholder
	require.NotContains(t, view, "Enter text here...")
	require.Contains(t, view, cursorOn)
}

// ============================================================================
// Unit Tests: Scrolling for multi-line content
// ============================================================================

func TestView_MultiLine_NoScrollingNeeded(t *testing.T) {
	m := New(Config{VimEnabled: true})
	m.SetValue("line1\nline2\nline3")
	m.SetSize(80, 10) // Height larger than content
	view := m.View()

	// All lines should be visible
	require.Contains(t, view, "line1")
	require.Contains(t, view, "line2")
	require.Contains(t, view, "line3")
}

func TestView_ScrollOffset_CursorAtTop(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("line1\nline2\nline3\nline4\nline5")
	m.SetSize(80, 3) // Only 3 lines visible
	m.cursorRow = 0
	m.cursorCol = 0
	m.Focus()

	// Ensure cursor is visible
	m.ensureCursorVisible()

	// Scroll offset should be 0 (cursor at top)
	require.Equal(t, 0, m.scrollOffset)

	view := m.View()
	// First 3 lines should be visible (line1 has cursor on 'l')
	require.Contains(t, view, cursorOn+"l"+cursorOff+"ine1") // cursor on first char
	require.Contains(t, view, "line2")
	require.Contains(t, view, "line3")
}

func TestView_ScrollOffset_CursorAtBottom(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("line1\nline2\nline3\nline4\nline5")
	m.SetSize(80, 3) // Only 3 lines visible
	m.cursorRow = 4  // Last line
	m.cursorCol = 0
	m.Focus()

	// Ensure cursor is visible
	m.ensureCursorVisible()

	// Scroll offset should be 2 (showing lines 3,4,5 at indices 2,3,4)
	require.Equal(t, 2, m.scrollOffset)

	view := m.View()
	// Last 3 lines should be visible (line5 has cursor on 'l')
	require.Contains(t, view, "line3")
	require.Contains(t, view, "line4")
	require.Contains(t, view, cursorOn+"l"+cursorOff+"ine5") // cursor on first char
	// First line should NOT be visible
	require.NotContains(t, view, "ine1") // Check for unique part without cursor interference
}

func TestView_ScrollOffset_UpdatesWhenCursorMoves(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10")
	m.SetSize(80, 3) // Only 3 lines visible
	m.cursorRow = 0
	m.Focus()

	// Initially at top
	m.ensureCursorVisible()
	require.Equal(t, 0, m.scrollOffset)

	// Move cursor down to line 5 (index 4)
	m.cursorRow = 4
	m.ensureCursorVisible()

	// Scroll should have adjusted
	require.Equal(t, 2, m.scrollOffset)

	// Move cursor down to line 10 (index 9)
	m.cursorRow = 9
	m.ensureCursorVisible()

	// Scroll should be at max (showing lines 8,9,10)
	require.Equal(t, 7, m.scrollOffset)
}

func TestView_ScrollOffset_NoHeightRestriction(t *testing.T) {
	m := New(Config{VimEnabled: true})
	m.SetValue("line1\nline2\nline3\nline4\nline5")
	m.SetSize(80, 0) // No height restriction
	m.cursorRow = 4

	m.ensureCursorVisible()

	// With no height restriction, scroll offset should be 0
	require.Equal(t, 0, m.scrollOffset)
}

func TestView_ScrollOffset_SoftWrap_CursorVisible(t *testing.T) {
	// Create a model with a single long line that will wrap
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("This is a very long line that will definitely wrap multiple times")
	m.SetSize(20, 3) // Width 20, Height 3 - so line wraps to ~4 display lines
	m.Focus()

	// Line length is 66, width is 20, so:
	// Display line 0: "This is a very long " (chars 0-19)
	// Display line 1: "line that will defin" (chars 20-39)
	// Display line 2: "itely wrap multiple " (chars 40-59)
	// Display line 3: "times" (chars 60-65)
	// Total: 4 display lines, viewport shows 3

	// Initially cursor at start, should be visible
	m.cursorCol = 0
	m.ensureCursorVisible()
	require.Equal(t, 0, m.scrollOffset)
	require.Equal(t, 0, m.cursorDisplayRow())

	// Move cursor to middle of second wrap segment (display line 1)
	m.cursorCol = 25
	m.ensureCursorVisible()
	require.Equal(t, 0, m.scrollOffset, "Cursor on display line 1, viewport shows 0-2, should be visible")
	require.Equal(t, 1, m.cursorDisplayRow())

	// Move cursor to third wrap segment (display line 2)
	m.cursorCol = 45
	m.ensureCursorVisible()
	require.Equal(t, 0, m.scrollOffset, "Cursor on display line 2, viewport shows 0-2, should be visible")
	require.Equal(t, 2, m.cursorDisplayRow())

	// Move cursor to fourth wrap segment (display line 3) - this should trigger scroll!
	m.cursorCol = 62
	m.ensureCursorVisible()
	require.Equal(t, 3, m.cursorDisplayRow(), "Cursor should be on display line 3")
	require.Equal(t, 1, m.scrollOffset, "Should scroll to show display lines 1-3")

	// Verify cursor is actually visible in the view
	view := m.View()
	require.Contains(t, view, cursorOn, "Cursor should be visible in rendered view")
}

func TestView_ScrollOffset_SoftWrap_ViaKeyPress(t *testing.T) {
	// Test that scrolling works through the Update loop (key presses)
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("This is a very long line that will definitely wrap multiple times")
	m.SetSize(20, 3) // Width 20, Height 3 - line wraps to 4 display lines
	m.Focus()

	// Use 'j' to move down through wrapped segments
	// 'j' in soft-wrap mode moves to next display line within same logical line

	// Initial state
	require.Equal(t, 0, m.cursorCol)
	require.Equal(t, 0, m.scrollOffset)

	// Press 'j' to move to second wrap segment
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	t.Logf("After 1st j: cursorCol=%d, scrollOffset=%d, displayRow=%d", m.cursorCol, m.scrollOffset, m.cursorDisplayRow())
	require.Equal(t, 1, m.cursorDisplayRow(), "Should be on display line 1")
	require.Equal(t, 0, m.scrollOffset, "No scroll needed yet")

	// Press 'j' to move to third wrap segment
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	t.Logf("After 2nd j: cursorCol=%d, scrollOffset=%d, displayRow=%d", m.cursorCol, m.scrollOffset, m.cursorDisplayRow())
	require.Equal(t, 2, m.cursorDisplayRow(), "Should be on display line 2")
	require.Equal(t, 0, m.scrollOffset, "No scroll needed yet")

	// Press 'j' to move to fourth wrap segment - this should trigger scroll!
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	t.Logf("After 3rd j: cursorCol=%d, scrollOffset=%d, displayRow=%d", m.cursorCol, m.scrollOffset, m.cursorDisplayRow())
	require.Equal(t, 3, m.cursorDisplayRow(), "Should be on display line 3")
	require.Equal(t, 1, m.scrollOffset, "Should scroll to show cursor")

	// Verify cursor is visible in the rendered view
	view := m.View()
	require.Contains(t, view, cursorOn, "Cursor should be visible in rendered view")
}

func TestView_ScrollOffset_SoftWrap_ViaHorizontalMovement(t *testing.T) {
	// Test scrolling when moving horizontally with 'l' across wrapped boundaries
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("This is a very long line that will definitely wrap multiple times")
	m.SetSize(20, 3) // Width 20, Height 3 - line wraps to 4 display lines
	m.Focus()

	// Move to end of line using '$'
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'$'}})
	t.Logf("After $: cursorCol=%d, scrollOffset=%d, displayRow=%d", m.cursorCol, m.scrollOffset, m.cursorDisplayRow())

	// Cursor should be at last char (col 64 for 65-char line), on display line 3
	require.Equal(t, 64, m.cursorCol, "Cursor should be at last character")
	require.Equal(t, 3, m.cursorDisplayRow(), "Should be on display line 3")
	require.Equal(t, 1, m.scrollOffset, "Should scroll to show cursor on last wrap segment")

	// Verify cursor is visible
	view := m.View()
	require.Contains(t, view, cursorOn, "Cursor should be visible")
}

// ============================================================================
// Unit Tests: Multi-line cursor rendering
// ============================================================================

func TestView_MultiLine_CursorOnCorrectLine(t *testing.T) {
	m := New(Config{VimEnabled: true})
	m.SetValue("first\nsecond\nthird")
	m.cursorRow = 1
	m.cursorCol = 0
	m.Focus()

	view := m.View()

	// Cursor should be on 's' in "second"
	require.Contains(t, view, cursorOn+"s"+cursorOff+"econd")
	// "first" should NOT have cursor
	require.Contains(t, view, "first")
	require.NotContains(t, view, cursorOn+"f"+cursorOff)
}

// ============================================================================
// Unit Tests: isEmpty helper
// ============================================================================

func TestIsEmpty_EmptyContent(t *testing.T) {
	m := New(Config{})
	require.True(t, m.isEmpty())
}

func TestIsEmpty_SingleEmptyLine(t *testing.T) {
	m := New(Config{})
	m.content = []string{""}
	require.True(t, m.isEmpty())
}

func TestIsEmpty_WithContent(t *testing.T) {
	m := New(Config{})
	m.SetValue("hello")
	require.False(t, m.isEmpty())
}

func TestIsEmpty_MultipleLines(t *testing.T) {
	m := New(Config{})
	m.SetValue("line1\nline2")
	require.False(t, m.isEmpty())
}

// ============================================================================
// Unit Tests: ScrollOffset getter/setter
// ============================================================================

func TestScrollOffset_GetSet(t *testing.T) {
	m := New(Config{})
	m.SetValue("line1\nline2\nline3\nline4\nline5")
	m.SetSize(80, 3)
	// Put cursor on line 5 (index 4) so scroll offset 2 is valid
	// With scroll offset 2, lines at indices 2,3,4 are visible
	// Cursor at index 4 is the last visible line
	m.cursorRow = 4

	m.SetScrollOffset(2)
	require.Equal(t, 2, m.ScrollOffset())
}

func TestScrollOffset_ClampedToValidRange(t *testing.T) {
	m := New(Config{})
	m.SetValue("line1\nline2\nline3")
	m.SetSize(80, 2)
	// Put cursor on last line so scroll offset is clamped correctly
	m.cursorRow = 2

	// Try to set offset beyond valid range
	m.SetScrollOffset(10)
	// Should be clamped to max valid offset (1, since 3 lines - 2 height = 1)
	require.Equal(t, 1, m.ScrollOffset())
}

// ============================================================================
// Golden Tests
// ============================================================================

func TestView_Golden_Empty(t *testing.T) {
	m := New(Config{VimEnabled: false})
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_EmptyFocused(t *testing.T) {
	m := New(Config{VimEnabled: false})
	m.Focus()
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_Placeholder(t *testing.T) {
	m := New(Config{
		VimEnabled:  false,
		Placeholder: "Type your message...",
	})
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_NormalMode(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("Hello, world!")
	m.cursorCol = 0
	m.Focus()
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_InsertMode(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("Hello, world!")
	m.cursorCol = 7 // After comma+space
	m.Focus()
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_MultiLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("First line\nSecond line\nThird line")
	m.cursorRow = 1
	m.cursorCol = 0
	m.Focus()
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_MultiLineWithScroll(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("Line 1\nLine 2\nLine 3\nLine 4\nLine 5\nLine 6\nLine 7\nLine 8")
	m.SetSize(80, 3) // Only 3 lines visible
	m.cursorRow = 6  // Line 7 (index 6)
	m.cursorCol = 0
	m.Focus()
	// ensureCursorVisible is called within getVisibleLines
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_VimDisabled(t *testing.T) {
	m := New(Config{VimEnabled: false})
	m.SetValue("Simple text without vim mode")
	m.Focus()
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// ============================================================================
// Edge Cases
// ============================================================================

func TestView_CursorCol_ClampedInRender(t *testing.T) {
	m := New(Config{VimEnabled: true})
	m.SetValue("abc")
	m.cursorCol = 100 // Way beyond line length
	m.Focus()

	// Should not panic, cursor should be rendered at end
	view := m.View()
	require.Contains(t, view, cursorOn)
}

func TestView_EmptyLineInMultiLine(t *testing.T) {
	m := New(Config{VimEnabled: true})
	m.SetValue("first\n\nthird")
	m.Focus()

	view := m.View()
	lines := strings.Split(view, "\n")

	// Should have content for all lines including empty middle line
	require.Equal(t, 3, len(lines), "Should have exactly 3 content lines")
}

// ============================================================================
// Golden Tests: Visual Mode Selection
// ============================================================================

func TestView_Golden_VisualModeSelection(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("Hello, world!")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 0}
	m.cursorRow = 0
	m.cursorCol = 5 // Selecting "Hello," (0-5)
	m.Focus()
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_VisualModeMultiLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("First line\nSecond line\nThird line")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 3}
	m.cursorRow = 2
	m.cursorCol = 5 // Selecting from "st line" through "Second line" to "Third "
	m.Focus()
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_VisualLineMode(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("First line\nSecond line\nThird line")
	m.mode = ModeVisualLine
	m.visualAnchor = Position{Row: 0, Col: 0}
	m.cursorRow = 1
	m.cursorCol = 3 // Two full lines selected
	m.Focus()
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_VisualModeEmptyLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("First\n\nThird")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 0}
	m.cursorRow = 2
	m.cursorCol = 2 // Selecting across empty line
	m.Focus()
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_VisualModeSoftWrap(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	// Create a line that will wrap with width 20
	m.SetValue("This is a very long line that will definitely wrap")
	m.SetSize(20, 10) // Width 20 will cause wrapping
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 5}
	m.cursorRow = 0
	m.cursorCol = 30 // Selection spans multiple wrapped display lines
	m.Focus()
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_VisualModeCursorInSelection(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("Hello, world!")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 0}
	m.cursorRow = 0
	m.cursorCol = 7 // Cursor in middle of selection "Hello, w|orld!"
	m.Focus()
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}
