package vimtextarea

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// ============================================================================
// InsertTextCommand Tests
// ============================================================================

func TestInsertTextCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 5

	cmd := &InsertTextCommand{row: 0, col: 5, text: " world"}
	err := cmd.Execute(m)

	require.Equal(t, Executed, err)
	require.Equal(t, "hello world", m.content[0])
	require.Equal(t, 11, m.cursorCol)
}

// TestInsertTextCommand_ExecuteMiddle verifies inserting text in middle of line
func TestInsertTextCommand_ExecuteMiddle(t *testing.T) {
	m := newTestModelWithContent("helloworld")
	m.cursorCol = 5

	cmd := &InsertTextCommand{row: 0, col: 5, text: " "}
	err := cmd.Execute(m)

	require.Equal(t, Executed, err)
	require.Equal(t, "hello world", m.content[0])
	require.Equal(t, 6, m.cursorCol)
}

// TestInsertTextCommand_ExecuteEmpty verifies inserting empty text is no-op
func TestInsertTextCommand_ExecuteEmpty(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 0

	cmd := &InsertTextCommand{row: 0, col: 0, text: ""}
	err := cmd.Execute(m)

	require.Equal(t, Executed, err)
	require.Equal(t, "hello", m.content[0])
}

// TestInsertTextCommand_ExecuteMultiLine verifies multi-line paste
func TestInsertTextCommand_ExecuteMultiLine(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 5

	cmd := &InsertTextCommand{row: 0, col: 5, text: "\nfoo\nbar"}
	err := cmd.Execute(m)

	require.Equal(t, Executed, err)
	require.Len(t, m.content, 3)
	require.Equal(t, "hello", m.content[0])
	require.Equal(t, "foo", m.content[1])
	require.Equal(t, "bar world", m.content[2])
	require.Equal(t, 2, m.cursorRow)
	require.Equal(t, 3, m.cursorCol)
}

// TestInsertTextCommand_Undo verifies removing inserted text
func TestInsertTextCommand_Undo(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 5

	cmd := &InsertTextCommand{row: 0, col: 5, text: " world"}
	_ = cmd.Execute(m)
	require.Equal(t, "hello world", m.content[0])

	err := cmd.Undo(m)
	require.NoError(t, err)
	require.Equal(t, "hello", m.content[0])
	require.Equal(t, 0, m.cursorRow)
	require.Equal(t, 5, m.cursorCol)
}

// TestInsertTextCommand_UndoMultiLine verifies undoing multi-line paste
func TestInsertTextCommand_UndoMultiLine(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 5

	cmd := &InsertTextCommand{row: 0, col: 5, text: "\nfoo\nbar"}
	_ = cmd.Execute(m)
	require.Len(t, m.content, 3)

	err := cmd.Undo(m)
	require.NoError(t, err)
	require.Len(t, m.content, 1)
	require.Equal(t, "hello world", m.content[0])
	require.Equal(t, 0, m.cursorRow)
	require.Equal(t, 5, m.cursorCol)
}

// ============================================================================
// SplitLineCommand Tests
// ============================================================================

// TestSplitLineCommand_Execute verifies splitting line at cursor
func TestSplitLineCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 5

	cmd := &SplitLineCommand{row: 0, col: 5}
	err := cmd.Execute(m)

	require.Equal(t, Executed, err)
	require.Len(t, m.content, 2)
	require.Equal(t, "hello", m.content[0])
	require.Equal(t, " world", m.content[1])
	require.Equal(t, 1, m.cursorRow)
	require.Equal(t, 0, m.cursorCol)
}

// TestSplitLineCommand_ExecuteAtStart verifies splitting at line start
func TestSplitLineCommand_ExecuteAtStart(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 0

	cmd := &SplitLineCommand{row: 0, col: 0}
	err := cmd.Execute(m)

	require.Equal(t, Executed, err)
	require.Len(t, m.content, 2)
	require.Equal(t, "", m.content[0])
	require.Equal(t, "hello", m.content[1])
}

// TestSplitLineCommand_ExecuteAtEnd verifies splitting at line end
func TestSplitLineCommand_ExecuteAtEnd(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 5

	cmd := &SplitLineCommand{row: 0, col: 5}
	err := cmd.Execute(m)

	require.Equal(t, Executed, err)
	require.Len(t, m.content, 2)
	require.Equal(t, "hello", m.content[0])
	require.Equal(t, "", m.content[1])
}

// TestSplitLineCommand_Undo verifies rejoining split lines
func TestSplitLineCommand_Undo(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 5

	cmd := &SplitLineCommand{row: 0, col: 5}
	_ = cmd.Execute(m)
	require.Len(t, m.content, 2)

	err := cmd.Undo(m)
	require.NoError(t, err)
	require.Len(t, m.content, 1)
	require.Equal(t, "hello world", m.content[0])
	require.Equal(t, 0, m.cursorRow)
	require.Equal(t, 5, m.cursorCol)
}

// ============================================================================
// SplitLineCommand Grapheme-Aware Tests
// ============================================================================

// TestSplitLineCommand_AfterEmoji verifies that splitting a line after an emoji
// correctly divides the line at the grapheme boundary, not the byte boundary.
// Emoji like "😀" are 4 bytes but 1 grapheme.
func TestSplitLineCommand_AfterEmoji(t *testing.T) {
	// Content: "a😀b" - 3 graphemes: 'a'=0, '😀'=1, 'b'=2
	m := newTestModelWithContent("a😀b")
	// Position cursor after the emoji (grapheme index 2, before 'b')
	m.cursorRow = 0
	m.cursorCol = 2

	cmd := &SplitLineCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Len(t, m.content, 2)
	// First line: 'a' and '😀' (everything before cursor)
	require.Equal(t, "a😀", m.content[0], "first line should contain graphemes before cursor")
	// Second line: 'b' (everything after cursor)
	require.Equal(t, "b", m.content[1], "second line should contain graphemes after cursor")
	require.Equal(t, 1, m.cursorRow, "cursor should be on new line")
	require.Equal(t, 0, m.cursorCol, "cursor should be at start of new line")
}

// TestSplitLineCommand_MidMultiByteSequence verifies that splitting at a grapheme
// boundary after multi-byte characters doesn't corrupt UTF-8 sequences.
// Family emoji: 👨‍👩‍👧‍👦 is 25 bytes but 1 grapheme.
func TestSplitLineCommand_MidMultiByteSequence(t *testing.T) {
	// Content: "x👨‍👩‍👧‍👦y" - 3 graphemes: 'x'=0, family=1, 'y'=2
	familyEmoji := "👨‍👩‍👧‍👦"
	m := newTestModelWithContent("x" + familyEmoji + "y")
	// Position cursor after 'x' and family emoji (grapheme index 2, before 'y')
	m.cursorRow = 0
	m.cursorCol = 2

	cmd := &SplitLineCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Len(t, m.content, 2)
	// First line should have 'x' and the complete family emoji
	require.Equal(t, "x"+familyEmoji, m.content[0], "first line should preserve family emoji intact")
	// Second line should have 'y'
	require.Equal(t, "y", m.content[1], "second line should contain 'y'")
	// Verify grapheme count to ensure no corruption
	require.Equal(t, 2, GraphemeCount(m.content[0]), "first line should have 2 graphemes")
	require.Equal(t, 1, GraphemeCount(m.content[1]), "second line should have 1 grapheme")
}

// TestSplitLineCommand_WithNBSP verifies that splitting a line containing NBSP
// (U+00A0) and NNBSP (U+202F) characters handles them correctly as single graphemes.
// This matches the original bug scenario with macOS screenshot paths.
func TestSplitLineCommand_WithNBSP(t *testing.T) {
	// NNBSP (U+202F) is 3 bytes, NBSP (U+00A0) is 2 bytes
	nnbsp := "\u202F"
	nbsp := "\u00A0"

	tests := []struct {
		name       string
		initial    string
		splitAt    int
		wantFirst  string
		wantSecond string
	}{
		{
			name:       "split after NNBSP",
			initial:    "Jan" + nnbsp + "17", // "Jan 17" with NNBSP, 5 graphemes
			splitAt:    4,                    // after "Jan" + NNBSP
			wantFirst:  "Jan" + nnbsp,
			wantSecond: "17",
		},
		{
			name:       "split before NNBSP",
			initial:    "Jan" + nnbsp + "17",
			splitAt:    3, // after "Jan", before NNBSP
			wantFirst:  "Jan",
			wantSecond: nnbsp + "17",
		},
		{
			name:       "split with NBSP in middle",
			initial:    "hello" + nbsp + "world", // 11 graphemes
			splitAt:    6,                        // after "hello" + NBSP
			wantFirst:  "hello" + nbsp,
			wantSecond: "world",
		},
		{
			name:       "split mixed multi-byte spaces",
			initial:    "a" + nnbsp + "b" + nbsp + "c", // 5 graphemes
			splitAt:    2,                              // after 'a' and NNBSP
			wantFirst:  "a" + nnbsp,
			wantSecond: "b" + nbsp + "c",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModelWithContent(tt.initial)
			m.cursorRow = 0
			m.cursorCol = tt.splitAt

			cmd := &SplitLineCommand{}
			result := cmd.Execute(m)

			require.Equal(t, Executed, result)
			require.Len(t, m.content, 2)
			require.Equal(t, tt.wantFirst, m.content[0], "first line content mismatch")
			require.Equal(t, tt.wantSecond, m.content[1], "second line content mismatch")
			require.Equal(t, 1, m.cursorRow)
			require.Equal(t, 0, m.cursorCol)
		})
	}
}

// ============================================================================
// InsertLineCommand Tests
// ============================================================================

func TestInsertLineCommand_ExecuteBelow(t *testing.T) {
	m := newTestModelWithContent("line1", "line2")
	m.cursorRow = 0

	cmd := &InsertLineCommand{above: false}
	err := cmd.Execute(m)

	require.Equal(t, Executed, err)
	require.Len(t, m.content, 3)
	require.Equal(t, "line1", m.content[0])
	require.Equal(t, "", m.content[1])
	require.Equal(t, "line2", m.content[2])
	require.Equal(t, 1, m.cursorRow)
	require.Equal(t, 0, m.cursorCol)
}

// TestInsertLineCommand_ExecuteAbove verifies 'O' inserts line above
func TestInsertLineCommand_ExecuteAbove(t *testing.T) {
	m := newTestModelWithContent("line1", "line2")
	m.cursorRow = 1

	cmd := &InsertLineCommand{above: true}
	err := cmd.Execute(m)

	require.Equal(t, Executed, err)
	require.Len(t, m.content, 3)
	require.Equal(t, "line1", m.content[0])
	require.Equal(t, "", m.content[1])
	require.Equal(t, "line2", m.content[2])
	require.Equal(t, 1, m.cursorRow)
	require.Equal(t, 0, m.cursorCol)
}

// TestInsertLineCommand_UndoBelow verifies undoing 'o'
func TestInsertLineCommand_UndoBelow(t *testing.T) {
	m := newTestModelWithContent("line1", "line2")
	m.cursorRow = 0

	cmd := &InsertLineCommand{above: false}
	_ = cmd.Execute(m)
	require.Len(t, m.content, 3)

	err := cmd.Undo(m)
	require.NoError(t, err)
	require.Len(t, m.content, 2)
	require.Equal(t, "line1", m.content[0])
	require.Equal(t, "line2", m.content[1])
	require.Equal(t, 0, m.cursorRow)
}

// TestInsertLineCommand_UndoAbove verifies undoing 'O'
func TestInsertLineCommand_UndoAbove(t *testing.T) {
	m := newTestModelWithContent("line1", "line2")
	m.cursorRow = 1

	cmd := &InsertLineCommand{above: true}
	_ = cmd.Execute(m)
	require.Len(t, m.content, 3)

	err := cmd.Undo(m)
	require.NoError(t, err)
	require.Len(t, m.content, 2)
	require.Equal(t, "line1", m.content[0])
	require.Equal(t, "line2", m.content[1])
	require.Equal(t, 1, m.cursorRow)
}

// ============================================================================
// Metadata Tests for Insert Commands
// ============================================================================

// TestInsertTextCommand_Metadata verifies command metadata
func TestInsertTextCommand_Metadata(t *testing.T) {
	cmd := &InsertTextCommand{}
	require.Equal(t, []string{"<char>"}, cmd.Keys())
	require.Equal(t, ModeInsert, cmd.Mode())
	require.Equal(t, "insert.text", cmd.ID())
	require.True(t, cmd.IsUndoable())
	require.True(t, cmd.ChangesContent())
	require.False(t, cmd.IsModeChange())
}

// TestSplitLineCommand_Metadata verifies command metadata
func TestSplitLineCommand_Metadata(t *testing.T) {
	cmd := &SplitLineCommand{}
	require.Equal(t, []string{"<alt+enter>"}, cmd.Keys())
	require.Equal(t, ModeInsert, cmd.Mode())
	require.Equal(t, "insert.split_line", cmd.ID())
	require.True(t, cmd.IsUndoable())
	require.True(t, cmd.ChangesContent())
	require.False(t, cmd.IsModeChange())
}

// TestInsertLineCommand_Metadata_Below verifies 'o' metadata
func TestInsertLineCommand_Metadata_Below(t *testing.T) {
	cmd := &InsertLineCommand{above: false}
	require.Equal(t, []string{"o"}, cmd.Keys())
	require.Equal(t, ModeNormal, cmd.Mode())
	require.Equal(t, "insert.line_below", cmd.ID())
	require.True(t, cmd.IsUndoable())
	require.True(t, cmd.ChangesContent())
	require.False(t, cmd.IsModeChange())
}

// TestInsertLineCommand_Metadata_Above verifies 'O' metadata
func TestInsertLineCommand_Metadata_Above(t *testing.T) {
	cmd := &InsertLineCommand{above: true}
	require.Equal(t, []string{"O"}, cmd.Keys())
	require.Equal(t, ModeNormal, cmd.Mode())
	require.Equal(t, "insert.line_above", cmd.ID())
	require.True(t, cmd.IsUndoable())
	require.True(t, cmd.ChangesContent())
	require.False(t, cmd.IsModeChange())
}

// TestInsertLineCommand_UndoInvalidRow verifies undo with invalid row
func TestInsertLineCommand_UndoInvalidRow(t *testing.T) {
	m := newTestModelWithContent("line1")
	cmd := &InsertLineCommand{row: 99} // out of range

	err := cmd.Undo(m)
	require.NoError(t, err)
	require.Len(t, m.content, 1) // unchanged
}

// ============================================================================
// SpaceCommand Tests
// ============================================================================

// TestSpaceCommand_Execute verifies inserting a space
func TestSpaceCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 5

	cmd := &SpaceCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "hello ", m.content[0])
	require.Equal(t, 6, m.cursorCol)
}

// TestSpaceCommand_ExecuteAtCharLimit verifies space is rejected at limit
func TestSpaceCommand_ExecuteAtCharLimit(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 5
	m.config.CharLimit = 5 // Already at limit

	cmd := &SpaceCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Skipped, result)
	require.Equal(t, "hello", m.content[0])
}

// TestSpaceCommand_Undo verifies removing inserted space
func TestSpaceCommand_Undo(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 5

	cmd := &SpaceCommand{}
	_ = cmd.Execute(m)
	require.Equal(t, "hello ", m.content[0])

	err := cmd.Undo(m)
	require.NoError(t, err)
	require.Equal(t, "hello", m.content[0])
	require.Equal(t, 5, m.cursorCol)
}

// TestSpaceCommand_UndoNil verifies undo is safe when not executed
func TestSpaceCommand_UndoNil(t *testing.T) {
	m := newTestModelWithContent("hello")
	cmd := &SpaceCommand{} // Not executed

	err := cmd.Undo(m)
	require.NoError(t, err)
	require.Equal(t, "hello", m.content[0])
}

// TestSpaceCommand_Metadata verifies command metadata
func TestSpaceCommand_Metadata(t *testing.T) {
	cmd := &SpaceCommand{}
	require.Equal(t, []string{"<space>"}, cmd.Keys())
	require.Equal(t, ModeInsert, cmd.Mode())
	require.Equal(t, "insert.space", cmd.ID())
	require.True(t, cmd.IsUndoable())
	require.True(t, cmd.ChangesContent())
	require.False(t, cmd.IsModeChange())
}

// ============================================================================
// InsertTextCommand Grapheme-Aware Tests
// ============================================================================

// TestInsertTextCommand_EmojiPreservesCursorPosition verifies that inserting
// an emoji correctly updates cursorCol as grapheme count, not byte count.
// Emoji like "😀" are 4 bytes but 1 grapheme.
func TestInsertTextCommand_EmojiPreservesCursorPosition(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 5

	// Insert an emoji (4 bytes, 1 grapheme)
	cmd := &InsertTextCommand{row: 0, col: 5, text: "😀"}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "hello😀", m.content[0])
	// Cursor should be at grapheme position 6 (5 + 1), not 9 (5 + 4 bytes)
	require.Equal(t, 6, m.cursorCol, "cursorCol should be grapheme count, not byte count")
}

// TestInsertTextCommand_TypeAfterEmoji verifies that typing a character after
// an emoji inserts at the correct position without UTF-8 corruption.
func TestInsertTextCommand_TypeAfterEmoji(t *testing.T) {
	m := newTestModelWithContent("a😀b")
	// Position cursor after the emoji (grapheme index 2: 'a'=0, '😀'=1, cursor before 'b')
	m.cursorCol = 2

	// Type a character at position 2
	cmd := &InsertTextCommand{row: 0, col: 2, text: "x"}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "a😀xb", m.content[0], "character should insert between emoji and 'b'")
	require.Equal(t, 3, m.cursorCol, "cursor should be at grapheme position 3")
}

// TestInsertTextCommand_MultiByteSpaces verifies handling of NBSP (U+00A0)
// and NNBSP (U+202F), which are multi-byte but single grapheme characters.
// This is the original bug scenario with macOS screenshot paths.
func TestInsertTextCommand_MultiByteSpaces(t *testing.T) {
	// NNBSP (U+202F) is 3 bytes, NBSP (U+00A0) is 2 bytes
	nnbsp := "\u202F"
	nbsp := "\u00A0"

	tests := []struct {
		name     string
		initial  string
		insertAt int
		insert   string
		expected string
		wantCol  int
	}{
		{
			name:     "insert after NNBSP",
			initial:  "a" + nnbsp + "b",
			insertAt: 2, // after 'a' and NNBSP
			insert:   "x",
			expected: "a" + nnbsp + "xb",
			wantCol:  3,
		},
		{
			name:     "insert NNBSP",
			initial:  "hello",
			insertAt: 5,
			insert:   nnbsp,
			expected: "hello" + nnbsp,
			wantCol:  6, // 5 + 1 grapheme, not 5 + 3 bytes
		},
		{
			name:     "insert after NBSP",
			initial:  "a" + nbsp + "b",
			insertAt: 2,
			insert:   "y",
			expected: "a" + nbsp + "yb",
			wantCol:  3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModelWithContent(tt.initial)
			m.cursorCol = tt.insertAt

			cmd := &InsertTextCommand{row: 0, col: tt.insertAt, text: tt.insert}
			result := cmd.Execute(m)

			require.Equal(t, Executed, result)
			require.Equal(t, tt.expected, m.content[0])
			require.Equal(t, tt.wantCol, m.cursorCol, "cursorCol should be grapheme-based")
		})
	}
}

// TestInsertTextCommand_InsertMidMultiByte verifies that inserting text at a
// position after a multi-byte character correctly handles the grapheme boundary.
func TestInsertTextCommand_InsertMidMultiByte(t *testing.T) {
	// Family emoji: 👨‍👩‍👧‍👦 is 25 bytes but 1 grapheme
	familyEmoji := "👨‍👩‍👧‍👦"

	m := newTestModelWithContent("x" + familyEmoji + "y")
	// Position after 'x' and the family emoji (grapheme index 2)
	m.cursorCol = 2

	cmd := &InsertTextCommand{row: 0, col: 2, text: "!"}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "x"+familyEmoji+"!y", m.content[0], "should insert between emoji and 'y'")
	require.Equal(t, 3, m.cursorCol, "cursor should be at grapheme position 3")

	// Verify the string is valid UTF-8 and grapheme count is correct
	require.Equal(t, 4, GraphemeCount(m.content[0]), "should have 4 graphemes: x, emoji, !, y")
}

// ============================================================================
// InsertTextCommand Multi-Line Grapheme-Aware Tests
// ============================================================================

// TestInsertTextCommand_MultiLineEmojiPaste verifies that multi-line paste with
// emoji correctly splits lines at grapheme boundaries, not byte boundaries.
func TestInsertTextCommand_MultiLineEmojiPaste(t *testing.T) {
	// Content has an emoji in the middle
	m := newTestModelWithContent("a😀bc")
	// Position cursor after the emoji (grapheme index 2: 'a'=0, '😀'=1, cursor before 'b')
	m.cursorCol = 2

	// Paste multi-line text at position 2
	cmd := &InsertTextCommand{row: 0, col: 2, text: "line1\nline2"}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Len(t, m.content, 2)
	// First line: everything before cursor + first pasted line
	require.Equal(t, "a😀line1", m.content[0], "should split at grapheme boundary")
	// Second line: last pasted line + everything after cursor
	require.Equal(t, "line2bc", m.content[1])
	require.Equal(t, 1, m.cursorRow)
	require.Equal(t, 5, m.cursorCol, "cursor should be at grapheme position of 'line2'")
}

// TestInsertTextCommand_MultiLinePasteCursorPosition verifies that after a
// multi-line paste, the cursor is at the correct grapheme position (not byte position)
// when the last pasted line contains multi-byte characters.
func TestInsertTextCommand_MultiLinePasteCursorPosition(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 5

	// Paste text where last line contains emoji
	cmd := &InsertTextCommand{row: 0, col: 5, text: "\nfirst\n😀😀😀"}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Len(t, m.content, 3)
	require.Equal(t, "hello", m.content[0])
	require.Equal(t, "first", m.content[1])
	require.Equal(t, "😀😀😀 world", m.content[2])
	require.Equal(t, 2, m.cursorRow)
	// Cursor should be at grapheme position 3 (3 emoji), not byte position 12 (3 * 4 bytes)
	require.Equal(t, 3, m.cursorCol, "cursorCol should be grapheme count, not byte count")
}

// TestInsertTextCommand_MultiLinePasteWithNBSP verifies that multi-line paste
// containing NBSP (U+00A0) and NNBSP (U+202F) characters handles them correctly
// as single graphemes at line boundaries.
func TestInsertTextCommand_MultiLinePasteWithNBSP(t *testing.T) {
	// NNBSP (U+202F) is 3 bytes, NBSP (U+00A0) is 2 bytes
	nnbsp := "\u202F"
	nbsp := "\u00A0"

	// Content has NNBSP in it (like macOS screenshot paths)
	m := newTestModelWithContent("Jan" + nnbsp + "17")
	// Position cursor at grapheme index 4 (after "Jan" + NNBSP)
	m.cursorCol = 4

	// Paste multi-line text that contains NBSP
	cmd := &InsertTextCommand{row: 0, col: 4, text: "x" + nbsp + "y\nend"}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Len(t, m.content, 2)
	// First line: "Jan" + NNBSP + first pasted line ("x" + NBSP + "y")
	require.Equal(t, "Jan"+nnbsp+"x"+nbsp+"y", m.content[0])
	// Second line: "end" + rest after cursor ("17")
	require.Equal(t, "end17", m.content[1])
	require.Equal(t, 1, m.cursorRow)
	// Cursor should be at grapheme position 3 (e, n, d), not affected by multi-byte chars
	require.Equal(t, 3, m.cursorCol, "cursorCol should be grapheme count")
}

// ============================================================================
// InsertTextCommand Undo Grapheme-Aware Tests
// ============================================================================

// TestInsertTextCommand_UndoEmoji verifies that undoing an emoji insertion
// correctly removes the emoji using grapheme-aware operations, not byte-based.
func TestInsertTextCommand_UndoEmoji(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 5

	// Insert an emoji (4 bytes, 1 grapheme)
	cmd := &InsertTextCommand{row: 0, col: 5, text: "😀"}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "hello😀", m.content[0])
	require.Equal(t, 6, m.cursorCol)

	// Undo the insertion
	err := cmd.Undo(m)
	require.NoError(t, err)
	require.Equal(t, "hello", m.content[0], "emoji should be removed completely")
	require.Equal(t, 0, m.cursorRow)
	require.Equal(t, 5, m.cursorCol, "cursor should be restored to position before emoji")
}

// TestInsertTextCommand_UndoAfterMultiByteChar verifies that undoing text insertion
// that was placed after a multi-byte character correctly preserves the multi-byte char.
func TestInsertTextCommand_UndoAfterMultiByteChar(t *testing.T) {
	// Content has an emoji at the beginning
	m := newTestModelWithContent("😀hello")
	m.cursorCol = 1 // after emoji

	// Insert text after the emoji
	cmd := &InsertTextCommand{row: 0, col: 1, text: "X"}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "😀Xhello", m.content[0])
	require.Equal(t, 2, m.cursorCol)

	// Undo the insertion
	err := cmd.Undo(m)
	require.NoError(t, err)
	require.Equal(t, "😀hello", m.content[0], "should restore original content with emoji intact")
	require.Equal(t, 0, m.cursorRow)
	require.Equal(t, 1, m.cursorCol, "cursor should be restored to position after emoji")
}

// TestInsertTextCommand_UndoMultiLineEmoji verifies that undoing a multi-line paste
// containing emoji correctly reconstructs the original line using grapheme boundaries.
func TestInsertTextCommand_UndoMultiLineEmoji(t *testing.T) {
	// Content with emoji
	m := newTestModelWithContent("a😀b")
	m.cursorCol = 2 // after emoji

	// Multi-line paste containing emoji
	cmd := &InsertTextCommand{row: 0, col: 2, text: "line1\n🎉end"}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Len(t, m.content, 2)
	require.Equal(t, "a😀line1", m.content[0])
	require.Equal(t, "🎉endb", m.content[1])
	require.Equal(t, 1, m.cursorRow)
	require.Equal(t, 4, m.cursorCol) // 🎉(1) + e(1) + n(1) + d(1) = 4 graphemes

	// Undo the multi-line insertion
	err := cmd.Undo(m)
	require.NoError(t, err)
	require.Len(t, m.content, 1, "should restore to single line")
	require.Equal(t, "a😀b", m.content[0], "should restore original content with emoji intact")
	require.Equal(t, 0, m.cursorRow)
	require.Equal(t, 2, m.cursorCol, "cursor should be restored to position after first emoji")
}
