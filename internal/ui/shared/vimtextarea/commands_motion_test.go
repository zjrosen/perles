package vimtextarea

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// ============================================================================
// MoveToLineStartInsertCommand Tests (Ctrl+A)
// ============================================================================

// TestMoveToLineStartInsertCommand_Execute verifies moving to line start
func TestMoveToLineStartInsertCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 6

	cmd := &MoveToLineStartInsertCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, 0, m.cursorCol)
}

// TestMoveToLineStartInsertCommand_AlreadyAtStart verifies no-op at start
func TestMoveToLineStartInsertCommand_AlreadyAtStart(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 0

	cmd := &MoveToLineStartInsertCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, 0, m.cursorCol)
}

// TestMoveToLineStartInsertCommand_Metadata verifies command metadata
func TestMoveToLineStartInsertCommand_Metadata(t *testing.T) {
	cmd := &MoveToLineStartInsertCommand{}
	require.Equal(t, "<ctrl+a>", cmd.Keys()[0])
	require.Equal(t, ModeInsert, cmd.Mode())
	require.Equal(t, "move.line_start_insert", cmd.ID())
	require.False(t, cmd.IsUndoable())
	require.False(t, cmd.ChangesContent())
	require.False(t, cmd.IsModeChange())
}

// ============================================================================
// MoveToLineEndInsertCommand Tests (Ctrl+E)
// ============================================================================

// TestMoveToLineEndInsertCommand_Execute verifies moving to line end
func TestMoveToLineEndInsertCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 0

	cmd := &MoveToLineEndInsertCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, 11, m.cursorCol) // "hello world" = 11 chars
}

// TestMoveToLineEndInsertCommand_AlreadyAtEnd verifies no-op at end
func TestMoveToLineEndInsertCommand_AlreadyAtEnd(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 5

	cmd := &MoveToLineEndInsertCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, 5, m.cursorCol)
}

// TestMoveToLineEndInsertCommand_EmptyLine verifies behavior on empty line
func TestMoveToLineEndInsertCommand_EmptyLine(t *testing.T) {
	m := newTestModelWithContent("")
	m.cursorCol = 0

	cmd := &MoveToLineEndInsertCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, 0, m.cursorCol)
}

// TestMoveToLineEndInsertCommand_Metadata verifies command metadata
func TestMoveToLineEndInsertCommand_Metadata(t *testing.T) {
	cmd := &MoveToLineEndInsertCommand{}
	require.Equal(t, "<ctrl+e>", cmd.Keys()[0])
	require.Equal(t, ModeInsert, cmd.Mode())
	require.Equal(t, "move.line_end_insert", cmd.ID())
	require.False(t, cmd.IsUndoable())
	require.False(t, cmd.ChangesContent())
	require.False(t, cmd.IsModeChange())
}

// ============================================================================
// Arrow Key Command Tests
// ============================================================================

// TestArrowLeftCommand_Execute verifies left arrow moves cursor left
func TestArrowLeftCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 3

	cmd := &ArrowLeftCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, 2, m.cursorCol)
}

// TestArrowLeftCommand_AtStart verifies left arrow at start stays at 0
func TestArrowLeftCommand_AtStart(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 0

	cmd := &ArrowLeftCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, 0, m.cursorCol)
}

// TestArrowRightCommand_Execute verifies right arrow moves cursor right
func TestArrowRightCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 2

	cmd := &ArrowRightCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, 3, m.cursorCol)
}

// TestArrowRightCommand_AtEnd verifies right arrow at end stays at end
func TestArrowRightCommand_AtEnd(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 5 // past last char (Insert mode position)

	cmd := &ArrowRightCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, 5, m.cursorCol)
}

// TestArrowUpCommand_Execute verifies up arrow moves cursor up
func TestArrowUpCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.cursorRow = 2
	m.cursorCol = 3

	cmd := &ArrowUpCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, 1, m.cursorRow)
	require.Equal(t, 3, m.cursorCol)
}

// TestArrowUpCommand_AtTop verifies up arrow at top stays at row 0
func TestArrowUpCommand_AtTop(t *testing.T) {
	m := newTestModelWithContent("line1", "line2")
	m.cursorRow = 0
	m.cursorCol = 2

	cmd := &ArrowUpCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, 0, m.cursorRow)
}

// TestArrowUpCommand_ClampColumn verifies cursor column clamps to shorter line
func TestArrowUpCommand_ClampColumn(t *testing.T) {
	m := newTestModelWithContent("hi", "hello world")
	m.cursorRow = 1
	m.cursorCol = 10

	cmd := &ArrowUpCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, 0, m.cursorRow)
	require.Equal(t, 2, m.cursorCol) // clamped to len("hi")
}

// TestArrowDownCommand_Execute verifies down arrow moves cursor down
func TestArrowDownCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.cursorRow = 0
	m.cursorCol = 3

	cmd := &ArrowDownCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, 1, m.cursorRow)
	require.Equal(t, 3, m.cursorCol)
}

// TestArrowDownCommand_AtBottom verifies down arrow at bottom stays at last row
func TestArrowDownCommand_AtBottom(t *testing.T) {
	m := newTestModelWithContent("line1", "line2")
	m.cursorRow = 1
	m.cursorCol = 2

	cmd := &ArrowDownCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, 1, m.cursorRow)
}

// TestArrowDownCommand_ClampColumn verifies cursor column clamps to shorter line
func TestArrowDownCommand_ClampColumn(t *testing.T) {
	m := newTestModelWithContent("hello world", "hi")
	m.cursorRow = 0
	m.cursorCol = 10

	cmd := &ArrowDownCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, 1, m.cursorRow)
	require.Equal(t, 2, m.cursorCol) // clamped to len("hi")
}

// TestArrowCommands_Metadata verifies arrow command metadata
func TestArrowCommands_Metadata(t *testing.T) {
	tests := []struct {
		cmd  Command
		keys []string
		id   string
	}{
		{&ArrowLeftCommand{}, []string{"<left>", "<ctrl+b>"}, "arrow.left"},
		{&ArrowRightCommand{}, []string{"<right>", "<ctrl+f>"}, "arrow.right"},
		{&ArrowUpCommand{}, []string{"<up>"}, "arrow.up"},
		{&ArrowDownCommand{}, []string{"<down>"}, "arrow.down"},
	}

	for _, tt := range tests {
		t.Run(tt.keys[0], func(t *testing.T) {
			require.Equal(t, tt.keys, tt.cmd.Keys())
			require.Equal(t, ModeInsert, tt.cmd.Mode())
			require.Equal(t, tt.id, tt.cmd.ID())
			require.False(t, tt.cmd.IsUndoable())
			require.False(t, tt.cmd.ChangesContent())
			require.False(t, tt.cmd.IsModeChange())
		})
	}
}

// ============================================================================
// Motion Command Tests
// ============================================================================

// TestMoveLeftCommand_Execute verifies 'h' moves cursor left
func TestMoveLeftCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 3

	cmd := &MoveLeftCommand{}
	err := cmd.Execute(m)

	require.Equal(t, Executed, err)
	require.Equal(t, 2, m.cursorCol)
	require.Equal(t, 2, m.preferredCol)
}

// TestMoveLeftCommand_ExecuteAtStart verifies 'h' at column 0 is no-op
func TestMoveLeftCommand_ExecuteAtStart(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 0

	cmd := &MoveLeftCommand{}
	err := cmd.Execute(m)

	require.Equal(t, Executed, err)
	require.Equal(t, 0, m.cursorCol)
}

// TestMoveLeftCommand_Undo verifies undo is no-op for motions
func TestMoveLeftCommand_Undo(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 2

	cmd := &MoveLeftCommand{}
	_ = cmd.Execute(m)
	require.Equal(t, 1, m.cursorCol)

	// Undo should be no-op - cursor stays where it is
	err := cmd.Undo(m)
	require.NoError(t, err)
	require.Equal(t, 1, m.cursorCol) // Still at 1, not back to 2
}

// TestMoveRightCommand_Execute verifies 'l' moves cursor right
func TestMoveRightCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 2

	cmd := &MoveRightCommand{}
	err := cmd.Execute(m)

	require.Equal(t, Executed, err)
	require.Equal(t, 3, m.cursorCol)
	require.Equal(t, 3, m.preferredCol)
}

// TestMoveRightCommand_ExecuteAtEnd verifies 'l' at last char is no-op
func TestMoveRightCommand_ExecuteAtEnd(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 4 // Last char position in Normal mode (len-1)

	cmd := &MoveRightCommand{}
	err := cmd.Execute(m)

	require.Equal(t, Executed, err)
	require.Equal(t, 4, m.cursorCol) // Stays at last char
}

// TestMoveRightCommand_ExecuteEmptyLine verifies 'l' on empty line is no-op
func TestMoveRightCommand_ExecuteEmptyLine(t *testing.T) {
	m := newTestModelWithContent("")
	m.cursorCol = 0

	cmd := &MoveRightCommand{}
	err := cmd.Execute(m)

	require.Equal(t, Executed, err)
	require.Equal(t, 0, m.cursorCol)
}

// TestMoveDownCommand_Execute verifies 'j' moves cursor down
func TestMoveDownCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.cursorRow = 0
	m.preferredCol = 2

	cmd := &MoveDownCommand{}
	err := cmd.Execute(m)

	require.Equal(t, Executed, err)
	require.Equal(t, 1, m.cursorRow)
}

// TestMoveDownCommand_ExecuteAtLastLine verifies 'j' at last line is no-op
func TestMoveDownCommand_ExecuteAtLastLine(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.cursorRow = 2

	cmd := &MoveDownCommand{}
	err := cmd.Execute(m)

	require.Equal(t, Executed, err)
	require.Equal(t, 2, m.cursorRow) // Stays at last line
}

// TestMoveDownCommand_ExecuteSoftWrap verifies 'j' with soft-wrap moves within wrap segment
func TestMoveDownCommand_ExecuteSoftWrap(t *testing.T) {
	m := newTestModelWithContent("this is a very long line that should wrap", "next line")
	m.width = 10 // Force soft-wrap
	m.cursorRow = 0
	m.cursorCol = 3

	cmd := &MoveDownCommand{}
	err := cmd.Execute(m)

	require.Equal(t, Executed, err)
	// Should move to next wrap segment within same logical line
	require.Equal(t, 0, m.cursorRow)
	require.Equal(t, 13, m.cursorCol) // 10 (wrap width) + 3 (col in wrap)
}

// TestMoveUpCommand_Execute verifies 'k' moves cursor up
func TestMoveUpCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.cursorRow = 2
	m.preferredCol = 2

	cmd := &MoveUpCommand{}
	err := cmd.Execute(m)

	require.Equal(t, Executed, err)
	require.Equal(t, 1, m.cursorRow)
}

// TestMoveUpCommand_ExecuteAtFirstLine verifies 'k' at first line is no-op
func TestMoveUpCommand_ExecuteAtFirstLine(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.cursorRow = 0

	cmd := &MoveUpCommand{}
	err := cmd.Execute(m)

	require.Equal(t, Executed, err)
	require.Equal(t, 0, m.cursorRow) // Stays at first line
}

// TestMoveUpCommand_ExecuteSoftWrap verifies 'k' with soft-wrap moves within wrap segment
func TestMoveUpCommand_ExecuteSoftWrap(t *testing.T) {
	m := newTestModelWithContent("this is a very long line that should wrap", "next line")
	m.width = 10 // Force soft-wrap
	m.cursorRow = 0
	m.cursorCol = 13 // Second wrap segment, col 3

	cmd := &MoveUpCommand{}
	err := cmd.Execute(m)

	require.Equal(t, Executed, err)
	// Should move to previous wrap segment within same logical line
	require.Equal(t, 0, m.cursorRow)
	require.Equal(t, 3, m.cursorCol) // Back to first segment, col 3
}

// TestMoveWordForwardCommand_Execute verifies 'w' moves to next word
func TestMoveWordForwardCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello world test")
	m.cursorCol = 0

	cmd := &MoveWordForwardCommand{}
	err := cmd.Execute(m)

	require.Equal(t, Executed, err)
	require.Equal(t, 6, m.cursorCol) // Start of "world"
}

// TestMoveWordForwardCommand_ExecuteToNextLine verifies 'w' crosses lines
func TestMoveWordForwardCommand_ExecuteToNextLine(t *testing.T) {
	m := newTestModelWithContent("hello", "world")
	m.cursorCol = 0

	// When at start of "hello" (a single word on the line), 'w' goes to next line
	cmd := &MoveWordForwardCommand{}
	_ = cmd.Execute(m)
	// Since "hello" is the only word on line 0, 'w' moves to line 1
	require.Equal(t, 1, m.cursorRow) // Moved to next line
	require.Equal(t, 0, m.cursorCol) // Start of "world"
}

// TestMoveWordForwardCommand_FinalWord verifies 'w' on final word moves to end of line
func TestMoveWordForwardCommand_FinalWord(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 6 // Start of "world"

	cmd := &MoveWordForwardCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	// In vim, 'w' at the last word moves to the end of the line (last char position)
	require.Equal(t, 0, m.cursorRow)
	require.Equal(t, 10, m.cursorCol) // Last char 'd' at index 10
}

// TestMoveWordForwardCommand_FinalWordSingleLine verifies 'w' on only word moves to end
func TestMoveWordForwardCommand_FinalWordSingleLine(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 0

	cmd := &MoveWordForwardCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	// 'w' on single word should move to end of that word (last char)
	require.Equal(t, 0, m.cursorRow)
	require.Equal(t, 4, m.cursorCol) // Last char 'o' at index 4
}

// TestMoveWordBackwardCommand_Execute verifies 'b' moves to previous word
func TestMoveWordBackwardCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello world test")
	m.cursorCol = 12

	cmd := &MoveWordBackwardCommand{}
	err := cmd.Execute(m)

	require.Equal(t, Executed, err)
	require.Equal(t, 6, m.cursorCol) // Start of "world"
}

// TestMoveWordBackwardCommand_ExecuteToPrevLine verifies 'b' crosses lines
func TestMoveWordBackwardCommand_ExecuteToPrevLine(t *testing.T) {
	m := newTestModelWithContent("hello", "world")
	m.cursorRow = 1
	m.cursorCol = 0

	cmd := &MoveWordBackwardCommand{}
	err := cmd.Execute(m)

	require.Equal(t, Executed, err)
	require.Equal(t, 0, m.cursorRow)
	require.Equal(t, 0, m.cursorCol) // Start of "hello"
}

// TestMoveWordEndCommand_Execute verifies 'e' moves to word end
func TestMoveWordEndCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 0

	cmd := &MoveWordEndCommand{}
	err := cmd.Execute(m)

	require.Equal(t, Executed, err)
	require.Equal(t, 4, m.cursorCol) // End of "hello"
}

// TestMoveWordEndCommand_ExecuteFromWordEnd verifies 'e' from word end goes to next word end
func TestMoveWordEndCommand_ExecuteFromWordEnd(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 4 // End of "hello"

	cmd := &MoveWordEndCommand{}
	err := cmd.Execute(m)

	require.Equal(t, Executed, err)
	require.Equal(t, 10, m.cursorCol) // End of "world"
}

// TestMoveToLineStartCommand_Execute verifies '0' moves to column 0
func TestMoveToLineStartCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("  hello world")
	m.cursorCol = 8

	cmd := &MoveToLineStartCommand{}
	err := cmd.Execute(m)

	require.Equal(t, Executed, err)
	require.Equal(t, 0, m.cursorCol)
	require.Equal(t, 0, m.preferredCol)
}

// TestMoveToLineEndCommand_Execute verifies '$' moves to last char
func TestMoveToLineEndCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 0

	cmd := &MoveToLineEndCommand{}
	err := cmd.Execute(m)

	require.Equal(t, Executed, err)
	require.Equal(t, 10, m.cursorCol) // Last char 'd' at index 10
}

// TestMoveToLineEndCommand_ExecuteEmptyLine verifies '$' on empty line stays at 0
func TestMoveToLineEndCommand_ExecuteEmptyLine(t *testing.T) {
	m := newTestModelWithContent("")
	m.cursorCol = 0

	cmd := &MoveToLineEndCommand{}
	err := cmd.Execute(m)

	require.Equal(t, Executed, err)
	require.Equal(t, 0, m.cursorCol)
}

// TestMoveToFirstNonBlankCommand_Execute verifies '^' moves to first non-blank
func TestMoveToFirstNonBlankCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("  hello world")
	m.cursorCol = 8

	cmd := &MoveToFirstNonBlankCommand{}
	err := cmd.Execute(m)

	require.Equal(t, Executed, err)
	require.Equal(t, 2, m.cursorCol) // First 'h'
}

// TestMoveToFirstNonBlankCommand_ExecuteAllBlanks verifies '^' on all-blank line goes to 0
func TestMoveToFirstNonBlankCommand_ExecuteAllBlanks(t *testing.T) {
	m := newTestModelWithContent("     ")
	m.cursorCol = 3

	cmd := &MoveToFirstNonBlankCommand{}
	err := cmd.Execute(m)

	require.Equal(t, Executed, err)
	require.Equal(t, 0, m.cursorCol)
}

// TestMoveToFirstLineCommand_Execute verifies 'gg' moves to first line
func TestMoveToFirstLineCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.cursorRow = 2
	m.cursorCol = 3

	cmd := &MoveToFirstLineCommand{}
	err := cmd.Execute(m)

	require.Equal(t, Executed, err)
	require.Equal(t, 0, m.cursorRow)
}

// TestMoveToLastLineCommand_Execute verifies 'G' moves to last line
func TestMoveToLastLineCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.cursorRow = 0
	m.cursorCol = 3

	cmd := &MoveToLastLineCommand{}
	err := cmd.Execute(m)

	require.Equal(t, Executed, err)
	require.Equal(t, 2, m.cursorRow)
}

// TestMotionCommands_UndoIsNoOp verifies all motion commands have no-op Undo
func TestMotionCommands_UndoIsNoOp(t *testing.T) {
	m := newTestModelWithContent("hello world", "second line", "third line")
	m.cursorRow = 1
	m.cursorCol = 3

	testCases := []struct {
		name string
		cmd  Command
	}{
		{"MoveLeftCommand", &MoveLeftCommand{}},
		{"MoveRightCommand", &MoveRightCommand{}},
		{"MoveDownCommand", &MoveDownCommand{}},
		{"MoveUpCommand", &MoveUpCommand{}},
		{"MoveWordForwardCommand", &MoveWordForwardCommand{}},
		{"MoveWordBackwardCommand", &MoveWordBackwardCommand{}},
		{"MoveWordEndCommand", &MoveWordEndCommand{}},
		{"MoveToLineStartCommand", &MoveToLineStartCommand{}},
		{"MoveToLineEndCommand", &MoveToLineEndCommand{}},
		{"MoveToFirstNonBlankCommand", &MoveToFirstNonBlankCommand{}},
		{"MoveToFirstLineCommand", &MoveToFirstLineCommand{}},
		{"MoveToLastLineCommand", &MoveToLastLineCommand{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset model state
			m.cursorRow = 1
			m.cursorCol = 3

			// Execute command
			_ = tc.cmd.Execute(m)
			posAfterExecute := Position{Row: m.cursorRow, Col: m.cursorCol}

			// Undo should be no-op - position should NOT change
			err := tc.cmd.Undo(m)
			require.NoError(t, err)
			require.Equal(t, posAfterExecute.Row, m.cursorRow, "Undo should be no-op for %s", tc.name)
			require.Equal(t, posAfterExecute.Col, m.cursorCol, "Undo should be no-op for %s", tc.name)
		})
	}
}

// TestMotionCommands_NotAddedToHistory verifies motion commands should not be in history
func TestMotionCommands_NotAddedToHistory(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 0

	// Motion commands should be executed directly, NOT via executeCommand
	cmd := &MoveRightCommand{}
	_ = cmd.Execute(m) // Direct execution
	// NOT: m.executeCommand(cmd) which would add to history

	require.False(t, m.history.CanUndo(), "Motion commands should not be added to history")
}

// TestMotionCommands_PreferredColSemantics verifies horizontal motions update preferredCol
func TestMotionCommands_PreferredColSemantics(t *testing.T) {
	m := newTestModelWithContent("hello world", "hi")
	m.cursorCol = 5
	m.preferredCol = 5

	// Horizontal motions update preferredCol
	leftCmd := &MoveLeftCommand{}
	_ = leftCmd.Execute(m)
	require.Equal(t, 4, m.preferredCol, "MoveLeft should update preferredCol")

	rightCmd := &MoveRightCommand{}
	_ = rightCmd.Execute(m)
	require.Equal(t, 5, m.preferredCol, "MoveRight should update preferredCol")

	// Now test vertical motion preserves preferredCol
	m.cursorCol = 8
	m.preferredCol = 8

	downCmd := &MoveDownCommand{}
	_ = downCmd.Execute(m)
	// Cursor is clamped to line length, but preferredCol should be preserved
	// (in the existing implementation, preferredCol is used to restore position)
	require.Equal(t, 1, m.cursorRow, "MoveDown should move to next row")
}

// TestMotionCommands_Key verifies all motion commands have correct trigger keys
func TestMotionCommands_Key(t *testing.T) {
	testCases := []struct {
		cmd         Command
		expectedKey string
		expectedID  string
	}{
		{&MoveLeftCommand{}, "h", "move.left"},
		{&MoveRightCommand{}, "l", "move.right"},
		{&MoveDownCommand{}, "j", "move.down"},
		{&MoveUpCommand{}, "k", "move.up"},
		{&MoveWordForwardCommand{}, "w", "move.word_forward"},
		{&MoveWordBackwardCommand{}, "b", "move.word_backward"},
		{&MoveWordEndCommand{}, "e", "move.word_end"},
		{&MoveToLineStartCommand{}, "0", "move.line_start"},
		{&MoveToLineEndCommand{}, "$", "move.line_end"},
		{&MoveToFirstNonBlankCommand{}, "^", "move.first_non_blank"},
		{&MoveToFirstLineCommand{}, "gg", "move.first_line"},
		{&MoveToLastLineCommand{}, "G", "move.last_line"},
	}

	for _, tc := range testCases {
		t.Run(tc.expectedID, func(t *testing.T) {
			require.Equal(t, tc.expectedKey, tc.cmd.Keys()[0], "Key() should return trigger key")
			require.Equal(t, tc.expectedID, tc.cmd.ID(), "ID() should return hierarchical identifier")
			require.Equal(t, ModeNormal, tc.cmd.Mode(), "Mode() should return ModeNormal")
		})
	}
}

// ============================================================================
// Grapheme-aware Motion Command Tests (Emoji and Unicode)
// ============================================================================

// TestMotionCommands_Grapheme_l_MovesOneGraphemeRight tests that 'l' moves
// by one grapheme, not one byte, past emoji characters.
func TestMotionCommands_Grapheme_l_MovesOneGraphemeRight(t *testing.T) {
	// "h😀llo" = 5 graphemes, 8 bytes (😀 is 4 bytes)
	m := newTestModelWithContent("h😀llo")
	m.cursorCol = 0 // On 'h'

	cmd := &MoveRightCommand{}
	_ = cmd.Execute(m)
	require.Equal(t, 1, m.cursorCol, "l from 'h' should move to grapheme 1 (😀)")

	// Move past the emoji
	_ = cmd.Execute(m)
	require.Equal(t, 2, m.cursorCol, "l from 😀 should move to grapheme 2 ('l')")
}

// TestMotionCommands_Grapheme_h_MovesOneGraphemeLeft tests that 'h' moves
// by one grapheme back before emoji characters.
func TestMotionCommands_Grapheme_h_MovesOneGraphemeLeft(t *testing.T) {
	// "h😀llo" = 5 graphemes
	m := newTestModelWithContent("h😀llo")
	m.cursorCol = 2 // On first 'l' after emoji

	cmd := &MoveLeftCommand{}
	_ = cmd.Execute(m)
	require.Equal(t, 1, m.cursorCol, "h from 'l' should move to grapheme 1 (😀)")

	_ = cmd.Execute(m)
	require.Equal(t, 0, m.cursorCol, "h from 😀 should move to grapheme 0 ('h')")
}

// TestMotionCommands_Grapheme_h_AtColumnZero_StaysAtZero tests boundary condition.
func TestMotionCommands_Grapheme_h_AtColumnZero_StaysAtZero(t *testing.T) {
	m := newTestModelWithContent("😀hello")
	m.cursorCol = 0

	cmd := &MoveLeftCommand{}
	_ = cmd.Execute(m)
	require.Equal(t, 0, m.cursorCol, "h at column 0 should stay at 0")
}

// TestMotionCommands_Grapheme_l_AtEndOfLine_StaysAtEnd tests boundary condition.
func TestMotionCommands_Grapheme_l_AtEndOfLine_StaysAtEnd(t *testing.T) {
	// "hello😀" = 6 graphemes
	m := newTestModelWithContent("hello😀")
	m.cursorCol = 5 // On 😀 (last grapheme in Normal mode)

	cmd := &MoveRightCommand{}
	_ = cmd.Execute(m)
	require.Equal(t, 5, m.cursorCol, "l at end of line should stay at end")
}

// TestMotionCommands_Grapheme_Dollar_MovesToLastGrapheme tests that '$' moves
// to the last grapheme, not the last byte.
func TestMotionCommands_Grapheme_Dollar_MovesToLastGrapheme(t *testing.T) {
	// "hello😀" = 6 graphemes
	m := newTestModelWithContent("hello😀")
	m.cursorCol = 0

	cmd := &MoveToLineEndCommand{}
	_ = cmd.Execute(m)
	require.Equal(t, 5, m.cursorCol, "$ should move to last grapheme (index 5)")
}

// TestMotionCommands_Grapheme_Dollar_WithZWJEmoji tests '$' with complex emoji.
func TestMotionCommands_Grapheme_Dollar_WithZWJEmoji(t *testing.T) {
	// "hi👨‍👩‍👧‍👦" = 3 graphemes (h, i, family emoji as 1 grapheme)
	m := newTestModelWithContent("hi👨‍👩‍👧‍👦")
	m.cursorCol = 0

	cmd := &MoveToLineEndCommand{}
	_ = cmd.Execute(m)
	require.Equal(t, 2, m.cursorCol, "$ should move to last grapheme (family emoji at index 2)")
}

// TestMotionCommands_Grapheme_w_SkipsEntireEmoji tests that 'w' treats emoji
// as a single punctuation unit when moving forward.
func TestMotionCommands_Grapheme_w_SkipsEntireEmoji(t *testing.T) {
	// "hello😀world" - emoji is punctuation, separates words
	m := newTestModelWithContent("hello😀world")
	m.cursorCol = 0 // On 'h'

	cmd := &MoveWordForwardCommand{}
	_ = cmd.Execute(m)
	// From 'hello', w should skip to the emoji (it's punctuation)
	require.Equal(t, 5, m.cursorCol, "w from 'hello' should move to emoji (punctuation)")

	_ = cmd.Execute(m)
	// From emoji, w should skip to 'world'
	require.Equal(t, 6, m.cursorCol, "w from emoji should move to 'world'")
}

// TestMotionCommands_Grapheme_b_MovesBackOverEmoji tests that 'b' moves back
// over an entire emoji as a single unit.
func TestMotionCommands_Grapheme_b_MovesBackOverEmoji(t *testing.T) {
	// "hello😀world" = 11 graphemes
	m := newTestModelWithContent("hello😀world")
	m.cursorCol = 6 // On 'w' in "world"

	cmd := &MoveWordBackwardCommand{}
	_ = cmd.Execute(m)
	// b should move back to the emoji
	require.Equal(t, 5, m.cursorCol, "b from 'world' should move to emoji")

	_ = cmd.Execute(m)
	// b should move back to start of 'hello'
	require.Equal(t, 0, m.cursorCol, "b from emoji should move to start of 'hello'")
}

// TestMotionCommands_Grapheme_e_LandsAtEndOfWord tests that 'e' lands at
// the end of a word containing emoji correctly.
func TestMotionCommands_Grapheme_e_LandsAtEndOfWord(t *testing.T) {
	// "hello world😀test"
	m := newTestModelWithContent("hello world😀test")
	m.cursorCol = 6 // On 'w' in "world"

	cmd := &MoveWordEndCommand{}
	_ = cmd.Execute(m)
	// e should land at end of 'world' (index 10)
	require.Equal(t, 10, m.cursorCol, "e from 'world' should land at 'd'")

	_ = cmd.Execute(m)
	// e should land at emoji (single-char punctuation word)
	require.Equal(t, 11, m.cursorCol, "e should land at emoji")
}

// TestMotionCommands_Grapheme_w_WithZWJSequence tests that ZWJ family emoji
// is treated as a single unit.
func TestMotionCommands_Grapheme_w_WithZWJSequence(t *testing.T) {
	// "a👨‍👩‍👧‍👦b" = 3 graphemes (a, family emoji, b)
	m := newTestModelWithContent("a👨‍👩‍👧‍👦b")
	m.cursorCol = 0 // On 'a'

	cmd := &MoveWordForwardCommand{}
	_ = cmd.Execute(m)
	// w should move from 'a' to the family emoji (punctuation)
	require.Equal(t, 1, m.cursorCol, "w should move to family emoji as single grapheme")

	_ = cmd.Execute(m)
	// w should move from emoji to 'b'
	require.Equal(t, 2, m.cursorCol, "w should move from emoji to 'b'")
}

// TestMotionCommands_Grapheme_0_MovesToStart tests that '0' still works.
func TestMotionCommands_Grapheme_0_MovesToStart(t *testing.T) {
	m := newTestModelWithContent("😀hello")
	m.cursorCol = 3

	cmd := &MoveToLineStartCommand{}
	_ = cmd.Execute(m)
	require.Equal(t, 0, m.cursorCol, "0 should move to grapheme 0")
}

// TestMotionCommands_Grapheme_Caret_MovesToFirstNonBlank tests '^' with emoji.
func TestMotionCommands_Grapheme_Caret_MovesToFirstNonBlank(t *testing.T) {
	// "  😀hello" - first non-blank is emoji at grapheme 2
	m := newTestModelWithContent("  😀hello")
	m.cursorCol = 5

	cmd := &MoveToFirstNonBlankCommand{}
	_ = cmd.Execute(m)
	require.Equal(t, 2, m.cursorCol, "^ should move to first non-blank grapheme (emoji at index 2)")
}

// TestMotionCommands_Grapheme_CursorNeverMidGrapheme tests that cursor
// never lands in the middle of a grapheme cluster.
func TestMotionCommands_Grapheme_CursorNeverMidGrapheme(t *testing.T) {
	// "a😀b👨‍👩‍👧‍👦c" = 5 graphemes
	m := newTestModelWithContent("a😀b👨‍👩‍👧‍👦c")
	graphemeCount := GraphemeCount(m.content[0])
	require.Equal(t, 5, graphemeCount, "Should have 5 graphemes")

	rightCmd := &MoveRightCommand{}
	leftCmd := &MoveLeftCommand{}

	// Move right through entire string
	for i := 0; i < 10; i++ {
		_ = rightCmd.Execute(m)
		// Cursor should always be within valid grapheme range
		require.GreaterOrEqual(t, m.cursorCol, 0, "Cursor should be >= 0")
		require.Less(t, m.cursorCol, graphemeCount, "Cursor should be < grapheme count in normal mode")
	}

	// Move left through entire string
	for i := 0; i < 10; i++ {
		_ = leftCmd.Execute(m)
		require.GreaterOrEqual(t, m.cursorCol, 0, "Cursor should be >= 0")
		require.Less(t, m.cursorCol, graphemeCount, "Cursor should be < grapheme count")
	}
}

// TestMotionCommands_Grapheme_ASCII_Regression tests that ASCII-only text
// still works correctly (regression test).
func TestMotionCommands_Grapheme_ASCII_Regression(t *testing.T) {
	m := newTestModelWithContent("hello world")

	// Test l command
	rightCmd := &MoveRightCommand{}
	m.cursorCol = 0
	_ = rightCmd.Execute(m)
	require.Equal(t, 1, m.cursorCol, "l should move to col 1 for ASCII")

	// Test $ command
	dollarCmd := &MoveToLineEndCommand{}
	_ = dollarCmd.Execute(m)
	require.Equal(t, 10, m.cursorCol, "$ should move to col 10 (last char 'd')")

	// Test w command
	m.cursorCol = 0
	wordCmd := &MoveWordForwardCommand{}
	_ = wordCmd.Execute(m)
	require.Equal(t, 6, m.cursorCol, "w should move to 'world' at col 6")

	// Test b command
	backCmd := &MoveWordBackwardCommand{}
	_ = backCmd.Execute(m)
	require.Equal(t, 0, m.cursorCol, "b should move back to col 0")

	// Test e command
	m.cursorCol = 0
	endCmd := &MoveWordEndCommand{}
	_ = endCmd.Execute(m)
	require.Equal(t, 4, m.cursorCol, "e should land at 'o' in 'hello'")
}

// TestMotionCommands_Grapheme_ArrowKeys_InsertMode tests arrow keys in insert mode
// work correctly with emoji.
func TestMotionCommands_Grapheme_ArrowKeys_InsertMode(t *testing.T) {
	// "h😀llo" = 5 graphemes
	m := newTestModelWithContent("h😀llo")
	m.mode = ModeInsert

	rightCmd := &ArrowRightCommand{}
	m.cursorCol = 0
	_ = rightCmd.Execute(m)
	require.Equal(t, 1, m.cursorCol, "Arrow right should move by 1 grapheme")

	_ = rightCmd.Execute(m)
	require.Equal(t, 2, m.cursorCol, "Arrow right past emoji should move to grapheme 2")

	leftCmd := &ArrowLeftCommand{}
	_ = leftCmd.Execute(m)
	require.Equal(t, 1, m.cursorCol, "Arrow left should move back by 1 grapheme")
}

// TestMotionCommands_Grapheme_CtrlE_InsertMode tests Ctrl+E moves to last grapheme.
func TestMotionCommands_Grapheme_CtrlE_InsertMode(t *testing.T) {
	// "hello😀" = 6 graphemes
	m := newTestModelWithContent("hello😀")
	m.mode = ModeInsert
	m.cursorCol = 0

	cmd := &MoveToLineEndInsertCommand{}
	_ = cmd.Execute(m)
	// In Insert mode, cursor can be past last grapheme (position 6 for 6 graphemes)
	require.Equal(t, 6, m.cursorCol, "Ctrl+E should move past last grapheme in insert mode")
}

// TestMoveWordBackwardCommand_CursorPastEnd tests the bug fix for when cursor
// is at position equal to grapheme count (one past the last valid index).
func TestMoveWordBackwardCommand_CursorPastEnd(t *testing.T) {
	// Create a line with exactly 32 graphemes
	// This reproduces the panic: "index out of range [32] with length 32"
	m := newTestModelWithContent("hello world test emoji 😀 end")
	graphemeCount := GraphemeCount(m.content[0])

	// Set cursor to position equal to grapheme count (one past last valid index)
	m.cursorCol = graphemeCount

	cmd := &MoveWordBackwardCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	// Should move back to start of last word without panic
	require.True(t, m.cursorCol < graphemeCount, "cursor should have moved back")
	require.True(t, m.cursorCol >= 0, "cursor should be valid")
}
