package vimtextarea

import (
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// enterVisualModeDirectly sets the model to visual mode at current cursor position.
// This is used instead of executing the 'v' command because 'v' is now a pending operator
// (for text object support like viw/vaw).
func enterVisualModeDirectly(m *Model) {
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: m.cursorRow, Col: m.cursorCol}
}

// ============================================================================
// InVisualMode Tests
// ============================================================================

func TestInVisualMode_Normal(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeNormal
	require.False(t, m.InVisualMode())
}

func TestInVisualMode_Insert(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeInsert
	require.False(t, m.InVisualMode())
}

func TestInVisualMode_Visual(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeVisual
	require.True(t, m.InVisualMode())
}

func TestInVisualMode_VisualLine(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeVisualLine
	require.True(t, m.InVisualMode())
}

// ============================================================================
// SelectionBounds Tests
// ============================================================================

func TestSelectionBounds_NotInVisualMode(t *testing.T) {
	m := newTestModelWithContent("hello", "world")
	m.mode = ModeNormal
	m.visualAnchor = Position{Row: 0, Col: 0}
	m.cursorRow = 0
	m.cursorCol = 3

	start, end := m.SelectionBounds()
	require.Equal(t, Position{}, start)
	require.Equal(t, Position{}, end)
}

func TestSelectionBounds_SingleLine_ForwardSelection(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 2}
	m.cursorRow = 0
	m.cursorCol = 7

	start, end := m.SelectionBounds()
	require.Equal(t, Position{Row: 0, Col: 2}, start)
	require.Equal(t, Position{Row: 0, Col: 7}, end)
}

func TestSelectionBounds_SingleLine_BackwardSelection(t *testing.T) {
	// Cursor before anchor - should be normalized
	m := newTestModelWithContent("hello world")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 7}
	m.cursorRow = 0
	m.cursorCol = 2

	start, end := m.SelectionBounds()
	// Should normalize: start <= end
	require.Equal(t, Position{Row: 0, Col: 2}, start)
	require.Equal(t, Position{Row: 0, Col: 7}, end)
}

func TestSelectionBounds_MultiLine_ForwardSelection(t *testing.T) {
	m := newTestModelWithContent("hello", "world", "test")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 2}
	m.cursorRow = 2
	m.cursorCol = 3

	start, end := m.SelectionBounds()
	require.Equal(t, Position{Row: 0, Col: 2}, start)
	require.Equal(t, Position{Row: 2, Col: 3}, end)
}

func TestSelectionBounds_MultiLine_BackwardSelection(t *testing.T) {
	// Cursor on earlier row than anchor
	m := newTestModelWithContent("hello", "world", "test")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 2, Col: 3}
	m.cursorRow = 0
	m.cursorCol = 2

	start, end := m.SelectionBounds()
	// Should normalize: start <= end
	require.Equal(t, Position{Row: 0, Col: 2}, start)
	require.Equal(t, Position{Row: 2, Col: 3}, end)
}

func TestSelectionBounds_SingleCharacter(t *testing.T) {
	// Anchor == cursor position
	m := newTestModelWithContent("hello")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 3}
	m.cursorRow = 0
	m.cursorCol = 3

	start, end := m.SelectionBounds()
	require.Equal(t, Position{Row: 0, Col: 3}, start)
	require.Equal(t, Position{Row: 0, Col: 3}, end)
}

func TestSelectionBounds_LinewiseMode_SingleLine(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.mode = ModeVisualLine
	m.visualAnchor = Position{Row: 0, Col: 5}
	m.cursorRow = 0
	m.cursorCol = 2

	start, end := m.SelectionBounds()
	// Line-wise mode should extend to full line
	require.Equal(t, 0, start.Col, "start.Col should be 0 for line-wise")
	require.Equal(t, 11, end.Col, "end.Col should be line length for line-wise")
	require.Equal(t, 0, start.Row)
	require.Equal(t, 0, end.Row)
}

func TestSelectionBounds_LinewiseMode_MultiLine(t *testing.T) {
	m := newTestModelWithContent("hello", "world", "test")
	m.mode = ModeVisualLine
	m.visualAnchor = Position{Row: 2, Col: 1}
	m.cursorRow = 0
	m.cursorCol = 3

	start, end := m.SelectionBounds()
	// Should normalize rows and extend to full lines
	require.Equal(t, Position{Row: 0, Col: 0}, start)
	require.Equal(t, Position{Row: 2, Col: 4}, end) // "test" is 4 chars
}

// ============================================================================
// SelectedText Tests
// ============================================================================

func TestSelectedText_NotInVisualMode(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.mode = ModeNormal
	m.visualAnchor = Position{Row: 0, Col: 0}
	m.cursorRow = 0
	m.cursorCol = 5

	text := m.SelectedText()
	require.Equal(t, "", text)
}

func TestSelectedText_SingleLine(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 0}
	m.cursorRow = 0
	m.cursorCol = 4

	text := m.SelectedText()
	require.Equal(t, "hello", text) // Includes character at cursor position
}

func TestSelectedText_SingleLine_MiddleOfLine(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 2}
	m.cursorRow = 0
	m.cursorCol = 7

	text := m.SelectedText()
	require.Equal(t, "llo wo", text)
}

func TestSelectedText_MultiLine(t *testing.T) {
	m := newTestModelWithContent("hello", "world", "test")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 2}
	m.cursorRow = 2
	m.cursorCol = 2

	text := m.SelectedText()
	// From "llo" on first line, all of "world", to "tes" on last line
	require.Equal(t, "llo\nworld\ntes", text)
}

func TestSelectedText_MultiLine_BackwardSelection(t *testing.T) {
	m := newTestModelWithContent("hello", "world", "test")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 2, Col: 2}
	m.cursorRow = 0
	m.cursorCol = 2

	text := m.SelectedText()
	// Should normalize and give same result
	require.Equal(t, "llo\nworld\ntes", text)
}

func TestSelectedText_SingleCharacter(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 2}
	m.cursorRow = 0
	m.cursorCol = 2

	text := m.SelectedText()
	require.Equal(t, "l", text)
}

func TestSelectedText_EmptyLine(t *testing.T) {
	m := newTestModelWithContent("")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 0}
	m.cursorRow = 0
	m.cursorCol = 0

	text := m.SelectedText()
	require.Equal(t, "", text)
}

func TestSelectedText_LinewiseMode(t *testing.T) {
	m := newTestModelWithContent("hello", "world")
	m.mode = ModeVisualLine
	m.visualAnchor = Position{Row: 0, Col: 2}
	m.cursorRow = 1
	m.cursorCol = 3

	text := m.SelectedText()
	// Line-wise should select full lines
	require.Equal(t, "hello\nworld", text)
}

func TestSelectedText_AtEndOfLine(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 3}
	m.cursorRow = 0
	m.cursorCol = 4 // Last character

	text := m.SelectedText()
	require.Equal(t, "lo", text)
}

// ============================================================================
// getSelectionRangeForRow Tests
// ============================================================================

func TestGetSelectionRangeForRow_NotInVisualMode(t *testing.T) {
	m := newTestModelWithContent("hello", "world")
	m.mode = ModeNormal

	startCol, endCol, inSelection := m.getSelectionRangeForRow(0)
	require.Equal(t, 0, startCol)
	require.Equal(t, 0, endCol)
	require.False(t, inSelection)
}

func TestGetSelectionRangeForRow_RowNotInSelection(t *testing.T) {
	m := newTestModelWithContent("hello", "world", "test")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 0}
	m.cursorRow = 0
	m.cursorCol = 4

	// Row 2 is not in the selection
	startCol, endCol, inSelection := m.getSelectionRangeForRow(2)
	require.Equal(t, 0, startCol)
	require.Equal(t, 0, endCol)
	require.False(t, inSelection)
}

func TestGetSelectionRangeForRow_SingleLineSelection(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 2}
	m.cursorRow = 0
	m.cursorCol = 7

	startCol, endCol, inSelection := m.getSelectionRangeForRow(0)
	require.True(t, inSelection)
	require.Equal(t, 2, startCol)
	require.Equal(t, 8, endCol) // Exclusive end (7 + 1)
}

func TestGetSelectionRangeForRow_MultiLine_FirstRow(t *testing.T) {
	m := newTestModelWithContent("hello", "world", "test")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 2}
	m.cursorRow = 2
	m.cursorCol = 2

	startCol, endCol, inSelection := m.getSelectionRangeForRow(0)
	require.True(t, inSelection)
	require.Equal(t, 2, startCol)
	require.Equal(t, 5, endCol) // To end of line "hello"
}

func TestGetSelectionRangeForRow_MultiLine_MiddleRow(t *testing.T) {
	m := newTestModelWithContent("hello", "world", "test")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 2}
	m.cursorRow = 2
	m.cursorCol = 2

	startCol, endCol, inSelection := m.getSelectionRangeForRow(1)
	require.True(t, inSelection)
	require.Equal(t, 0, startCol) // From start
	require.Equal(t, 5, endCol)   // To end of line "world"
}

func TestGetSelectionRangeForRow_MultiLine_LastRow(t *testing.T) {
	m := newTestModelWithContent("hello", "world", "test")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 2}
	m.cursorRow = 2
	m.cursorCol = 2

	startCol, endCol, inSelection := m.getSelectionRangeForRow(2)
	require.True(t, inSelection)
	require.Equal(t, 0, startCol)
	require.Equal(t, 3, endCol) // Exclusive end (2 + 1)
}

func TestGetSelectionRangeForRow_LinewiseMode(t *testing.T) {
	m := newTestModelWithContent("hello", "world")
	m.mode = ModeVisualLine
	m.visualAnchor = Position{Row: 0, Col: 2}
	m.cursorRow = 1
	m.cursorCol = 3

	startCol, endCol, inSelection := m.getSelectionRangeForRow(0)
	require.True(t, inSelection)
	require.Equal(t, 0, startCol) // Line-wise: full line
	require.Equal(t, 5, endCol)   // "hello" length

	startCol, endCol, inSelection = m.getSelectionRangeForRow(1)
	require.True(t, inSelection)
	require.Equal(t, 0, startCol)
	require.Equal(t, 5, endCol) // "world" length
}

func TestGetSelectionRangeForRow_EmptyLine(t *testing.T) {
	m := newTestModelWithContent("hello", "", "test")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 0}
	m.cursorRow = 2
	m.cursorCol = 3

	startCol, endCol, inSelection := m.getSelectionRangeForRow(1)
	require.True(t, inSelection)
	require.Equal(t, 0, startCol)
	require.Equal(t, 0, endCol) // Empty line
}

func TestGetSelectionRangeForRow_OutOfBoundsRow(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 0}
	m.cursorRow = 0
	m.cursorCol = 4

	// Row 5 doesn't exist
	startCol, endCol, inSelection := m.getSelectionRangeForRow(5)
	require.False(t, inSelection)
	require.Equal(t, 0, startCol)
	require.Equal(t, 0, endCol)
}

// ============================================================================
// SetValue Tests (Visual Mode Clearing)
// ============================================================================

func TestSetValue_ClearsVisualMode(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 2}

	m.SetValue("new content")

	require.Equal(t, ModeNormal, m.mode, "should exit visual mode")
	require.Equal(t, Position{}, m.visualAnchor, "should clear anchor")
	require.Equal(t, "new content", m.Value())
}

func TestSetValue_ClearsVisualLineMode(t *testing.T) {
	m := newTestModelWithContent("hello", "world")
	m.mode = ModeVisualLine
	m.visualAnchor = Position{Row: 0, Col: 0}

	m.SetValue("changed")

	require.Equal(t, ModeNormal, m.mode, "should exit visual line mode")
	require.Equal(t, Position{}, m.visualAnchor, "should clear anchor")
}

func TestSetValue_NoEffectInNormalMode(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeNormal

	m.SetValue("new content")

	require.Equal(t, ModeNormal, m.mode)
	require.Equal(t, "new content", m.Value())
}

func TestSetValue_NoEffectInInsertMode(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeInsert

	m.SetValue("new content")

	require.Equal(t, ModeInsert, m.mode)
	require.Equal(t, "new content", m.Value())
}

func TestSetValue_EmptyContent_ClearsVisualMode(t *testing.T) {
	m := newTestModelWithContent("hello", "world")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 1, Col: 3}

	m.SetValue("")

	require.Equal(t, ModeNormal, m.mode)
	require.Equal(t, Position{}, m.visualAnchor)
	require.Equal(t, "", m.Value())
}

// ============================================================================
// Property-Based Tests
// ============================================================================

func TestSelectionBounds_Property_StartAlwaysBeforeOrEqualEnd(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random positions
		anchorRow := rapid.IntRange(0, 10).Draw(t, "anchorRow")
		anchorCol := rapid.IntRange(0, 50).Draw(t, "anchorCol")
		cursorRow := rapid.IntRange(0, 10).Draw(t, "cursorRow")
		cursorCol := rapid.IntRange(0, 50).Draw(t, "cursorCol")

		// Create content with enough rows
		maxRow := max(anchorRow, cursorRow)
		content := make([]string, maxRow+1)
		for i := range content {
			content[i] = "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnop" // 50+ chars
		}

		m := &Model{
			content:      content,
			cursorRow:    cursorRow,
			cursorCol:    cursorCol,
			visualAnchor: Position{Row: anchorRow, Col: anchorCol},
			mode:         ModeVisual, // Character-wise mode
		}

		start, end := m.SelectionBounds()

		// Property: start should always be <= end lexicographically
		require.False(t, start.Row > end.Row, "start.Row (%d) > end.Row (%d)", start.Row, end.Row)
		require.False(t, start.Row == end.Row && start.Col > end.Col, "same row but start.Col (%d) > end.Col (%d)", start.Col, end.Col)
	})
}

func TestSelectionBounds_Property_LinewiseModeFullLines(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random positions
		anchorRow := rapid.IntRange(0, 5).Draw(t, "anchorRow")
		anchorCol := rapid.IntRange(0, 20).Draw(t, "anchorCol")
		cursorRow := rapid.IntRange(0, 5).Draw(t, "cursorRow")
		cursorCol := rapid.IntRange(0, 20).Draw(t, "cursorCol")

		// Create content with enough rows
		maxRow := max(anchorRow, cursorRow)
		content := make([]string, maxRow+1)
		lineLength := rapid.IntRange(5, 30).Draw(t, "lineLength")
		for i := range content {
			content[i] = string(make([]byte, lineLength)) // Fixed-length lines
			for j := range content[i] {
				content[i] = content[i][:j] + "x" + content[i][j+1:]
			}
			content[i] = "x" + content[i][1:] // Ensure at least one char
		}

		m := &Model{
			content:      content,
			cursorRow:    cursorRow,
			cursorCol:    cursorCol,
			visualAnchor: Position{Row: anchorRow, Col: anchorCol},
			mode:         ModeVisualLine, // Line-wise mode
		}

		start, end := m.SelectionBounds()

		// Property: in line-wise mode, start.Col should always be 0
		require.Equal(t, 0, start.Col, "line-wise mode should have start.Col = 0")

		// Property: in line-wise mode, end.Col should be line length
		if end.Row < len(content) {
			expectedEndCol := len(content[end.Row])
			require.Equal(t, expectedEndCol, end.Col, "line-wise mode should have end.Col = line length")
		}
	})
}

func TestSelectedText_Property_NotEmptyWhenInVisualMode(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate non-empty content
		numLines := rapid.IntRange(1, 5).Draw(t, "numLines")
		content := make([]string, numLines)
		for i := range content {
			lineLen := rapid.IntRange(1, 20).Draw(t, "lineLen")
			content[i] = rapid.StringMatching(`[a-z]+`).Draw(t, "line")
			if len(content[i]) > lineLen {
				content[i] = content[i][:lineLen]
			}
			// Ensure at least 1 character
			if len(content[i]) == 0 {
				content[i] = "a"
			}
		}

		// Generate valid positions within content
		anchorRow := rapid.IntRange(0, numLines-1).Draw(t, "anchorRow")
		cursorRow := rapid.IntRange(0, numLines-1).Draw(t, "cursorRow")
		anchorCol := rapid.IntRange(0, max(0, len(content[anchorRow])-1)).Draw(t, "anchorCol")
		cursorCol := rapid.IntRange(0, max(0, len(content[cursorRow])-1)).Draw(t, "cursorCol")

		m := &Model{
			content:      content,
			cursorRow:    cursorRow,
			cursorCol:    cursorCol,
			visualAnchor: Position{Row: anchorRow, Col: anchorCol},
			mode:         ModeVisual,
		}

		text := m.SelectedText()

		// Property: when all lines have content, SelectedText should return non-empty
		// (since cursor is within a valid character position)
		if len(text) == 0 {
			// Check if this is an edge case (empty line at selection position)
			hasContent := false
			for _, line := range content {
				if len(line) > 0 {
					hasContent = true
					break
				}
			}
			if hasContent && len(content[anchorRow]) > 0 && len(content[cursorRow]) > 0 {
				require.Fail(t, "expected non-empty selection, got empty",
					"anchor=(%d,%d) cursor=(%d,%d) content=%v", anchorRow, anchorCol, cursorRow, cursorCol, content)
			}
		}
	})
}

// ============================================================================
// Visual Mode Motion Integration Tests
// ============================================================================
// These tests verify that motion commands work correctly in visual mode,
// moving the cursor while the anchor stays fixed to extend selection.

// TestVisualMode_MoveLeft verifies 'h' moves cursor left, anchor stays fixed
func TestVisualMode_MoveLeft(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.mode = ModeNormal
	m.cursorRow = 0
	m.cursorCol = 6 // cursor on 'w'

	// Enter visual mode directly (v is now a pending operator for text object support)
	enterVisualMode(m)
	require.Equal(t, ModeVisual, m.mode)
	require.Equal(t, Position{Row: 0, Col: 6}, m.visualAnchor)

	// Move left with 'h'
	cmdH, ok := DefaultRegistry.Get(ModeVisual, "h")
	require.True(t, ok, "should have 'h' command for ModeVisual")
	result := cmdH.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, ModeVisual, m.mode, "mode should stay Visual")
	require.Equal(t, Position{Row: 0, Col: 6}, m.visualAnchor, "anchor should stay fixed")
	require.Equal(t, 5, m.cursorCol, "cursor should move left")
}

// TestVisualMode_MoveRight verifies 'l' extends selection right
func TestVisualMode_MoveRight(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.mode = ModeNormal
	m.cursorRow = 0
	m.cursorCol = 3 // cursor on 'l'

	// Enter visual mode directly (v is now a pending operator for text object support)
	enterVisualMode(m)
	require.Equal(t, ModeVisual, m.mode)
	require.Equal(t, Position{Row: 0, Col: 3}, m.visualAnchor)

	// Move right with 'l'
	cmdL, ok := DefaultRegistry.Get(ModeVisual, "l")
	require.True(t, ok, "should have 'l' command for ModeVisual")
	result := cmdL.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, ModeVisual, m.mode)
	require.Equal(t, Position{Row: 0, Col: 3}, m.visualAnchor, "anchor should stay fixed")
	require.Equal(t, 4, m.cursorCol, "cursor should move right")
}

// TestVisualMode_MoveUp verifies 'k' extends selection up
func TestVisualMode_MoveUp(t *testing.T) {
	m := newTestModelWithContent("hello", "world", "test")
	m.mode = ModeNormal
	m.cursorRow = 1
	m.cursorCol = 2

	// Enter visual mode directly (v is now a pending operator for text object support)
	enterVisualMode(m)
	require.Equal(t, ModeVisual, m.mode)
	require.Equal(t, Position{Row: 1, Col: 2}, m.visualAnchor)

	// Move up with 'k'
	cmdK, ok := DefaultRegistry.Get(ModeVisual, "k")
	require.True(t, ok, "should have 'k' command for ModeVisual")
	result := cmdK.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, ModeVisual, m.mode)
	require.Equal(t, Position{Row: 1, Col: 2}, m.visualAnchor, "anchor should stay fixed")
	require.Equal(t, 0, m.cursorRow, "cursor should move up")
}

// TestVisualMode_MoveDown verifies 'j' extends selection down
func TestVisualMode_MoveDown(t *testing.T) {
	m := newTestModelWithContent("hello", "world", "test")
	m.mode = ModeNormal
	m.cursorRow = 0
	m.cursorCol = 2

	// Enter visual mode directly (v is now a pending operator for text object support)
	enterVisualMode(m)
	require.Equal(t, ModeVisual, m.mode)
	require.Equal(t, Position{Row: 0, Col: 2}, m.visualAnchor)

	// Move down with 'j'
	cmdJ, ok := DefaultRegistry.Get(ModeVisual, "j")
	require.True(t, ok, "should have 'j' command for ModeVisual")
	result := cmdJ.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, ModeVisual, m.mode)
	require.Equal(t, Position{Row: 0, Col: 2}, m.visualAnchor, "anchor should stay fixed")
	require.Equal(t, 1, m.cursorRow, "cursor should move down")
}

// TestVisualMode_WordMotion verifies 'w', 'b', 'e' work in visual mode
func TestVisualMode_WordMotion(t *testing.T) {
	m := newTestModelWithContent("hello world test")
	m.mode = ModeNormal
	m.cursorRow = 0
	m.cursorCol = 0

	// Enter visual mode directly (v is now a pending operator for text object support)
	enterVisualMode(m)
	require.Equal(t, ModeVisual, m.mode)
	require.Equal(t, Position{Row: 0, Col: 0}, m.visualAnchor)

	// Move forward by word with 'w'
	cmdW, ok := DefaultRegistry.Get(ModeVisual, "w")
	require.True(t, ok, "should have 'w' command for ModeVisual")
	cmdW.Execute(m)
	require.Equal(t, 6, m.cursorCol, "cursor should move to 'world'")
	require.Equal(t, Position{Row: 0, Col: 0}, m.visualAnchor, "anchor should stay fixed")

	// Move backward by word with 'b'
	cmdB, ok := DefaultRegistry.Get(ModeVisual, "b")
	require.True(t, ok, "should have 'b' command for ModeVisual")
	cmdB.Execute(m)
	require.Equal(t, 0, m.cursorCol, "cursor should move back to 'hello'")

	// Move to word end with 'e'
	cmdE, ok := DefaultRegistry.Get(ModeVisual, "e")
	require.True(t, ok, "should have 'e' command for ModeVisual")
	cmdE.Execute(m)
	require.Equal(t, 4, m.cursorCol, "cursor should move to end of 'hello'")
}

// TestVisualMode_LineMotion verifies '0', '$', '^' work in visual mode
func TestVisualMode_LineMotion(t *testing.T) {
	m := newTestModelWithContent("  hello world  ")
	m.mode = ModeNormal
	m.cursorRow = 0
	m.cursorCol = 5 // cursor in middle

	// Enter visual mode directly (v is now a pending operator for text object support)
	enterVisualMode(m)
	require.Equal(t, ModeVisual, m.mode)
	require.Equal(t, Position{Row: 0, Col: 5}, m.visualAnchor)

	// Move to line start with '0'
	cmd0, ok := DefaultRegistry.Get(ModeVisual, "0")
	require.True(t, ok, "should have '0' command for ModeVisual")
	cmd0.Execute(m)
	require.Equal(t, 0, m.cursorCol, "cursor should move to column 0")
	require.Equal(t, Position{Row: 0, Col: 5}, m.visualAnchor, "anchor should stay fixed")

	// Move to line end with '$'
	cmdDollar, ok := DefaultRegistry.Get(ModeVisual, "$")
	require.True(t, ok, "should have '$' command for ModeVisual")
	cmdDollar.Execute(m)
	require.Equal(t, 14, m.cursorCol, "cursor should move to end of line")

	// Move to first non-blank with '^'
	cmdCaret, ok := DefaultRegistry.Get(ModeVisual, "^")
	require.True(t, ok, "should have '^' command for ModeVisual")
	cmdCaret.Execute(m)
	require.Equal(t, 2, m.cursorCol, "cursor should move to first non-blank")
}

// TestVisualMode_DocumentMotion verifies 'gg', 'G' work in visual mode
func TestVisualMode_DocumentMotion(t *testing.T) {
	m := newTestModelWithContent("line one", "line two", "line three")
	m.cursorRow = 1
	m.cursorCol = 3

	// Enter visual mode directly (v is now a pending operator, so we set mode directly)
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 1, Col: 3}
	require.Equal(t, ModeVisual, m.mode)
	require.Equal(t, Position{Row: 1, Col: 3}, m.visualAnchor)

	// Move to first line with 'gg'
	cmdGG, ok := DefaultRegistry.Get(ModeVisual, "gg")
	require.True(t, ok, "should have 'gg' command for ModeVisual")
	cmdGG.Execute(m)
	require.Equal(t, 0, m.cursorRow, "cursor should move to first line")
	require.Equal(t, Position{Row: 1, Col: 3}, m.visualAnchor, "anchor should stay fixed")

	// Move to last line with 'G'
	cmdG, ok := DefaultRegistry.Get(ModeVisual, "G")
	require.True(t, ok, "should have 'G' command for ModeVisual")
	cmdG.Execute(m)
	require.Equal(t, 2, m.cursorRow, "cursor should move to last line")
	require.Equal(t, Position{Row: 1, Col: 3}, m.visualAnchor, "anchor should stay fixed")
}

// TestVisualLineMode_VerticalOnly verifies j/k work in line-wise mode, h/l not registered
func TestVisualLineMode_VerticalOnly(t *testing.T) {
	m := newTestModelWithContent("line one", "line two", "line three")
	m.mode = ModeNormal
	m.cursorRow = 1
	m.cursorCol = 3

	// Enter visual line mode
	cmdShiftV, _ := DefaultRegistry.Get(ModeNormal, "V")
	cmdShiftV.Execute(m)
	require.Equal(t, ModeVisualLine, m.mode)
	require.Equal(t, Position{Row: 1, Col: 0}, m.visualAnchor)

	// Move down with 'j' - should work
	cmdJ, okJ := DefaultRegistry.Get(ModeVisualLine, "j")
	require.True(t, okJ, "should have 'j' command for ModeVisualLine")
	cmdJ.Execute(m)
	require.Equal(t, 2, m.cursorRow, "cursor should move down")

	// Move up with 'k' - should work
	cmdK, okK := DefaultRegistry.Get(ModeVisualLine, "k")
	require.True(t, okK, "should have 'k' command for ModeVisualLine")
	cmdK.Execute(m)
	cmdK.Execute(m) // Move up twice
	require.Equal(t, 0, m.cursorRow, "cursor should move up")

	// h and l should NOT be registered for VisualLine mode
	_, okH := DefaultRegistry.Get(ModeVisualLine, "h")
	require.False(t, okH, "should NOT have 'h' command for ModeVisualLine")

	_, okL := DefaultRegistry.Get(ModeVisualLine, "l")
	require.False(t, okL, "should NOT have 'l' command for ModeVisualLine")
}

// TestVisualMode_SelectionExtends verifies cursor moves, anchor fixed, selection computed
func TestVisualMode_SelectionExtends(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorRow = 0
	m.cursorCol = 0

	// Enter visual mode directly (v is now a pending operator, so we set mode directly)
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 0}
	require.Equal(t, ModeVisual, m.mode)
	require.Equal(t, Position{Row: 0, Col: 0}, m.visualAnchor)

	// Initial selection: single character 'h'
	text := m.SelectedText()
	require.Equal(t, "h", text)

	// Move right 4 times to extend selection to "hello"
	cmdL, _ := DefaultRegistry.Get(ModeVisual, "l")
	for i := 0; i < 4; i++ {
		cmdL.Execute(m)
	}

	require.Equal(t, Position{Row: 0, Col: 0}, m.visualAnchor, "anchor should still be at 0")
	require.Equal(t, 4, m.cursorCol, "cursor should be at position 4")

	text = m.SelectedText()
	require.Equal(t, "hello", text, "selection should extend to include 'hello'")

	// Move right one more to include space
	cmdL.Execute(m)
	text = m.SelectedText()
	require.Equal(t, "hello ", text, "selection should extend to include 'hello '")
}

// TestVisualLineMode_DocumentMotion verifies 'gg', 'G' work in line-wise mode
func TestVisualLineMode_DocumentMotion(t *testing.T) {
	m := newTestModelWithContent("line one", "line two", "line three")
	m.mode = ModeNormal
	m.cursorRow = 1
	m.cursorCol = 3

	// Enter visual line mode
	cmdShiftV, _ := DefaultRegistry.Get(ModeNormal, "V")
	cmdShiftV.Execute(m)
	require.Equal(t, ModeVisualLine, m.mode)
	require.Equal(t, Position{Row: 1, Col: 0}, m.visualAnchor)

	// Move to first line with 'gg'
	cmdGG, ok := DefaultRegistry.Get(ModeVisualLine, "gg")
	require.True(t, ok, "should have 'gg' command for ModeVisualLine")
	cmdGG.Execute(m)
	require.Equal(t, 0, m.cursorRow, "cursor should move to first line")
	require.Equal(t, Position{Row: 1, Col: 0}, m.visualAnchor, "anchor should stay fixed")

	// Verify line-wise selection covers both lines
	start, end := m.SelectionBounds()
	require.Equal(t, Position{Row: 0, Col: 0}, start)
	require.Equal(t, Position{Row: 1, Col: 8}, end) // "line two" is 8 chars

	// Move to last line with 'G'
	cmdG, ok := DefaultRegistry.Get(ModeVisualLine, "G")
	require.True(t, ok, "should have 'G' command for ModeVisualLine")
	cmdG.Execute(m)
	require.Equal(t, 2, m.cursorRow, "cursor should move to last line")

	// Verify line-wise selection covers lines 1-2
	start, end = m.SelectionBounds()
	require.Equal(t, Position{Row: 1, Col: 0}, start)
	require.Equal(t, Position{Row: 2, Col: 10}, end) // "line three" is 10 chars
}

// TestDefaultRegistry_HasAllVisualModeMotions verifies all motion commands are registered for visual modes
func TestDefaultRegistry_HasAllVisualModeMotions(t *testing.T) {
	// ModeVisual should have all motions
	visualMotionKeys := []string{"h", "l", "j", "k", "w", "b", "e", "0", "$", "^", "gg", "G"}
	for _, key := range visualMotionKeys {
		_, ok := DefaultRegistry.Get(ModeVisual, key)
		require.True(t, ok, "ModeVisual should have '%s' command", key)
	}

	// ModeVisualLine should have vertical and document motions only
	visualLineMotionKeys := []string{"j", "k", "gg", "G"}
	for _, key := range visualLineMotionKeys {
		_, ok := DefaultRegistry.Get(ModeVisualLine, key)
		require.True(t, ok, "ModeVisualLine should have '%s' command", key)
	}

	// ModeVisualLine should NOT have horizontal/word motions
	visualLineExcludedKeys := []string{"h", "l", "w", "b", "e", "0", "$", "^"}
	for _, key := range visualLineExcludedKeys {
		_, ok := DefaultRegistry.Get(ModeVisualLine, key)
		require.False(t, ok, "ModeVisualLine should NOT have '%s' command", key)
	}
}

// ============================================================================
// Visual Mode Emoji/Grapheme Tests (Phase 6)
// ============================================================================
// These tests verify that visual mode operations work correctly with emoji
// and other multi-byte Unicode characters.

func TestVisualMode_SelectEmoji_SingleUnit(t *testing.T) {
	// Visual mode 'v' should select emoji as a single unit
	m := newTestModelWithContent("h😀llo")
	m.cursorRow = 0
	m.cursorCol = 1 // cursor on 😀 (grapheme index 1)

	// Enter visual mode at emoji
	enterVisualModeDirectly(m)
	require.Equal(t, ModeVisual, m.mode)
	require.Equal(t, Position{Row: 0, Col: 1}, m.visualAnchor)

	// Selection should be the emoji
	text := m.SelectedText()
	require.Equal(t, "😀", text, "should select the entire emoji as one grapheme")
}

func TestVisualMode_ExtendSelectionRight_WithEmoji(t *testing.T) {
	// 'l' in visual mode should extend selection by one grapheme
	m := newTestModelWithContent("h😀llo")
	m.cursorRow = 0
	m.cursorCol = 0 // cursor on 'h'

	// Enter visual mode
	enterVisualModeDirectly(m)

	// Move right with 'l' - should move to emoji (grapheme 1)
	cmdL, ok := DefaultRegistry.Get(ModeVisual, "l")
	require.True(t, ok)
	cmdL.Execute(m)

	require.Equal(t, 1, m.cursorCol, "cursor should be at grapheme 1 (emoji)")
	text := m.SelectedText()
	require.Equal(t, "h😀", text, "selection should include 'h' and emoji")
}

func TestVisualMode_ContractSelectionLeft_WithEmoji(t *testing.T) {
	// 'h' in visual mode should contract selection by one grapheme
	m := newTestModelWithContent("h😀llo")
	m.cursorRow = 0
	m.cursorCol = 2 // cursor on first 'l' (grapheme index 2)

	// Enter visual mode with anchor at beginning
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 0}

	// Current selection is "h😀l"
	text := m.SelectedText()
	require.Equal(t, "h😀l", text)

	// Move left with 'h' - should move cursor to emoji (grapheme 1)
	cmdH, ok := DefaultRegistry.Get(ModeVisual, "h")
	require.True(t, ok)
	cmdH.Execute(m)

	require.Equal(t, 1, m.cursorCol, "cursor should be at grapheme 1 (emoji)")
	text = m.SelectedText()
	require.Equal(t, "h😀", text, "selection should contract to 'h' and emoji")
}

func TestVisualMode_Yank_CompleteEmoji(t *testing.T) {
	// 'y' in visual mode should yank complete emoji
	m := newTestModelWithContent("h😀llo")
	m.cursorRow = 0
	m.cursorCol = 1 // cursor on emoji

	// Enter visual mode and extend selection
	enterVisualModeDirectly(m)
	cmdL, _ := DefaultRegistry.Get(ModeVisual, "l")
	cmdL.Execute(m) // Select emoji and 'l'

	// Yank
	cmdY, ok := DefaultRegistry.Get(ModeVisual, "y")
	require.True(t, ok)
	cmdY.Execute(m)

	require.Equal(t, "😀l", m.lastYankedText, "yanked text should be complete emoji + 'l'")
	require.Equal(t, ModeNormal, m.mode, "should exit to normal mode after yank")
}

func TestVisualMode_Delete_CompleteEmoji(t *testing.T) {
	// 'd' in visual mode should delete complete emoji
	m := newTestModelWithContent("h😀llo")
	m.cursorRow = 0
	m.cursorCol = 1 // cursor on emoji

	// Enter visual mode - select just the emoji
	enterVisualModeDirectly(m)

	// Delete
	cmdD, ok := DefaultRegistry.Get(ModeVisual, "d")
	require.True(t, ok)
	cmdD.Execute(m)

	require.Equal(t, "hllo", m.content[0], "emoji should be completely deleted")
	require.Equal(t, ModeNormal, m.mode, "should exit to normal mode after delete")
	require.Equal(t, 1, m.cursorCol, "cursor should be at start of deleted region")
}

func TestVisualMode_Change_CompleteEmoji(t *testing.T) {
	// 'c' in visual mode should replace complete emoji
	m := newTestModelWithContent("h😀llo")
	m.cursorRow = 0
	m.cursorCol = 1 // cursor on emoji

	// Enter visual mode - select just the emoji
	enterVisualModeDirectly(m)

	// Change
	cmdC, ok := DefaultRegistry.Get(ModeVisual, "c")
	require.True(t, ok)
	cmdC.Execute(m)

	require.Equal(t, "hllo", m.content[0], "emoji should be completely deleted")
	require.Equal(t, ModeInsert, m.mode, "should enter insert mode after change")
	require.Equal(t, 1, m.cursorCol, "cursor should be at start of deleted region")
}

func TestVisualSelectedText_WithEmojiContent(t *testing.T) {
	// SelectedText() should return correct content with emoji
	m := newTestModelWithContent("h😀llo")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 0}
	m.cursorRow = 0
	m.cursorCol = 2 // Select "h😀l"

	text := m.SelectedText()
	require.Equal(t, "h😀l", text, "should select h, emoji, and l")
}

func TestVisualLineMode_WithEmojiLines(t *testing.T) {
	// Visual line mode 'V' should work with emoji-containing lines
	m := newTestModelWithContent("hello 😀", "world 🎉")
	m.cursorRow = 0
	m.cursorCol = 3

	// Enter visual line mode
	cmdV, _ := DefaultRegistry.Get(ModeNormal, "V")
	cmdV.Execute(m)
	require.Equal(t, ModeVisualLine, m.mode)

	// Extend to second line
	cmdJ, _ := DefaultRegistry.Get(ModeVisualLine, "j")
	cmdJ.Execute(m)

	// Yank
	cmdY, _ := DefaultRegistry.Get(ModeVisualLine, "y")
	cmdY.Execute(m)

	require.Equal(t, "hello 😀\nworld 🎉", m.lastYankedText, "should yank both emoji-containing lines")
}

func TestSelectionBounds_WithEmoji(t *testing.T) {
	// SelectionBounds should return grapheme indices with emoji
	m := newTestModelWithContent("h😀llo world")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 0}
	m.cursorRow = 0
	m.cursorCol = 4 // "h😀ll" - 5 graphemes

	start, end := m.SelectionBounds()
	require.Equal(t, Position{Row: 0, Col: 0}, start)
	require.Equal(t, Position{Row: 0, Col: 4}, end, "end.Col should be grapheme index 4")

	// Verify selected text matches
	text := m.SelectedText()
	require.Equal(t, "h😀llo", text)
}

func TestVisualMode_ZWJEmoji_SingleUnit(t *testing.T) {
	// ZWJ family emoji should be treated as single grapheme
	m := newTestModelWithContent("a👨‍👩‍👧‍👦b")
	m.cursorRow = 0
	m.cursorCol = 1 // cursor on family emoji

	// Enter visual mode
	enterVisualModeDirectly(m)

	text := m.SelectedText()
	require.Equal(t, "👨‍👩‍👧‍👦", text, "ZWJ family emoji should be selected as single unit")

	// Move right - should go to 'b'
	cmdL, _ := DefaultRegistry.Get(ModeVisual, "l")
	cmdL.Execute(m)

	require.Equal(t, 2, m.cursorCol, "cursor should move to grapheme 2 ('b')")
	text = m.SelectedText()
	require.Equal(t, "👨‍👩‍👧‍👦b", text, "selection should include complete ZWJ emoji and 'b'")
}

func TestVisualMode_SkinToneEmoji_SingleUnit(t *testing.T) {
	// Skin tone emoji should be treated as single grapheme
	m := newTestModelWithContent("wave:👋🏽!")
	m.cursorRow = 0
	m.cursorCol = 5 // cursor on skin tone emoji (after "wave:")

	// Enter visual mode
	enterVisualModeDirectly(m)

	text := m.SelectedText()
	require.Equal(t, "👋🏽", text, "skin tone emoji should be selected as single unit")
}

func TestVisualMode_FlagEmoji_SingleUnit(t *testing.T) {
	// Flag emoji should be treated as single grapheme
	m := newTestModelWithContent("US:🇺🇸!")
	m.cursorRow = 0
	m.cursorCol = 3 // cursor on flag emoji (after "US:")

	// Enter visual mode
	enterVisualModeDirectly(m)

	text := m.SelectedText()
	require.Equal(t, "🇺🇸", text, "flag emoji should be selected as single unit")
}

func TestVisualMode_MultiLineSelection_WithEmoji(t *testing.T) {
	// Multi-line selection with emoji on both lines
	m := newTestModelWithContent("hello 😀", "world 🎉 test")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 5} // After "hello" (on space)
	m.cursorRow = 1
	m.cursorCol = 7 // After "world 🎉" (on space)

	text := m.SelectedText()
	require.Equal(t, " 😀\nworld 🎉 ", text, "multi-line selection should preserve emoji")
}

func TestVisualMode_DeleteEmoji_Undo(t *testing.T) {
	// Delete emoji and undo should restore it correctly
	m := newTestModelWithContent("h😀llo")
	m.cursorRow = 0
	m.cursorCol = 1 // cursor on emoji

	// Enter visual mode
	enterVisualModeDirectly(m)

	// Delete
	cmdD, _ := DefaultRegistry.Get(ModeVisual, "d")
	m.executeCommand(cmdD)

	require.Equal(t, "hllo", m.content[0], "emoji should be deleted")

	// Undo
	m.history.Undo(m)

	require.Equal(t, "h😀llo", m.content[0], "emoji should be restored after undo")
}

func TestSelectedText_SelectionFromASCIIToEmoji(t *testing.T) {
	// Edge case: selection starting at ASCII and ending at emoji
	m := newTestModelWithContent("abc😀def")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 1} // 'b'
	m.cursorRow = 0
	m.cursorCol = 4 // 'd' (after emoji)

	text := m.SelectedText()
	require.Equal(t, "bc😀d", text, "selection should span from ASCII through emoji to ASCII")
}

func TestVisualMode_Property_SelectionAtGraphemeBoundary(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate content with emoji
		baseText := rapid.SliceOf(
			rapid.SampledFrom([]string{"a", "b", "c", "😀", "🎉", " "}),
		).Filter(func(s []string) bool {
			return len(s) > 0 && len(s) < 20
		}).Draw(t, "content")

		content := ""
		for _, s := range baseText {
			content += s
		}

		if content == "" {
			content = "a"
		}

		graphemeCount := GraphemeCount(content)
		if graphemeCount == 0 {
			return
		}

		anchorCol := rapid.IntRange(0, graphemeCount-1).Draw(t, "anchorCol")
		cursorCol := rapid.IntRange(0, graphemeCount-1).Draw(t, "cursorCol")

		m := &Model{
			content:      []string{content},
			cursorRow:    0,
			cursorCol:    cursorCol,
			visualAnchor: Position{Row: 0, Col: anchorCol},
			mode:         ModeVisual,
		}

		start, end := m.SelectionBounds()

		// Property: selection bounds should be valid grapheme indices
		require.True(t, start.Col >= 0 && start.Col < graphemeCount, "start.Col %d out of bounds [0, %d)", start.Col, graphemeCount)
		require.True(t, end.Col >= 0 && end.Col < graphemeCount, "end.Col %d out of bounds [0, %d)", end.Col, graphemeCount)

		// Property: selected text should not be empty (since we have valid bounds)
		text := m.SelectedText()
		require.False(t, text == "" && content != "", "expected non-empty selection for content %q with bounds (%d, %d)", content, start.Col, end.Col)

		// Property: selected text should not contain partial graphemes
		selectedGraphemes := GraphemeCount(text)
		expectedCount := end.Col - start.Col + 1
		require.Equal(t, expectedCount, selectedGraphemes, "selection grapheme count mismatch for selection %q", text)
	})
}
