package vimtextarea

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ============================================================================
// DeleteCharCommand Tests (x)
// ============================================================================

func TestDeleteCharCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 2 // position on 'l'

	cmd := &DeleteCharCommand{row: 0, col: 2}
	err := cmd.Execute(m)

	assert.Equal(t, Executed, err)
	assert.Equal(t, "helo", m.content[0])
	assert.Equal(t, "l", cmd.deletedGrapheme)
}

// TestDeleteCharCommand_ExecuteEmptyLine verifies deleting on empty line is no-op
func TestDeleteCharCommand_ExecuteEmptyLine(t *testing.T) {
	m := newTestModelWithContent("")

	cmd := &DeleteCharCommand{row: 0, col: 0}
	err := cmd.Execute(m)

	assert.Equal(t, Executed, err)
	assert.Equal(t, "", m.content[0])
}

// TestDeleteCharCommand_Undo verifies restoring deleted character
func TestDeleteCharCommand_Undo(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 2

	cmd := &DeleteCharCommand{row: 0, col: 2}
	_ = cmd.Execute(m)
	assert.Equal(t, "helo", m.content[0])

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, "hello", m.content[0])
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 2, m.cursorCol)
}

// TestDeleteCharCommand_LastChar verifies deleting last char moves cursor back
func TestDeleteCharCommand_LastChar(t *testing.T) {
	m := newTestModelWithContent("ab")
	m.cursorCol = 1 // position on 'b'

	cmd := &DeleteCharCommand{row: 0, col: 1}
	_ = cmd.Execute(m)

	assert.Equal(t, "a", m.content[0])
	assert.Equal(t, 0, m.cursorCol) // cursor should move back
}

// ============================================================================
// DeleteLineCommand Tests (dd)
// ============================================================================

// TestDeleteLineCommand_Execute verifies deleting entire line
func TestDeleteLineCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.cursorRow = 1 // position on line2

	cmd := &DeleteLineCommand{}
	err := cmd.Execute(m)

	assert.Equal(t, Executed, err)
	assert.Len(t, m.content, 2)
	assert.Equal(t, "line1", m.content[0])
	assert.Equal(t, "line3", m.content[1])
	assert.Equal(t, "line2", cmd.deletedLine)
}

// TestDeleteLineCommand_ExecuteOnlyLine verifies deleting only line clears it
func TestDeleteLineCommand_ExecuteOnlyLine(t *testing.T) {
	m := newTestModelWithContent("only line")

	cmd := &DeleteLineCommand{}
	err := cmd.Execute(m)

	assert.Equal(t, Executed, err)
	assert.Len(t, m.content, 1)
	assert.Equal(t, "", m.content[0])
	assert.True(t, cmd.wasOnlyLine)
}

// TestDeleteLineCommand_ExecuteLastLine verifies deleting last line moves cursor up
func TestDeleteLineCommand_ExecuteLastLine(t *testing.T) {
	m := newTestModelWithContent("line1", "line2")
	m.cursorRow = 1

	cmd := &DeleteLineCommand{}
	_ = cmd.Execute(m)

	assert.Len(t, m.content, 1)
	assert.Equal(t, "line1", m.content[0])
	assert.Equal(t, 0, m.cursorRow) // cursor should move up
}

// TestDeleteLineCommand_Undo verifies restoring deleted line
func TestDeleteLineCommand_Undo(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.cursorRow = 1

	cmd := &DeleteLineCommand{}
	_ = cmd.Execute(m)
	assert.Len(t, m.content, 2)

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Len(t, m.content, 3)
	assert.Equal(t, "line1", m.content[0])
	assert.Equal(t, "line2", m.content[1])
	assert.Equal(t, "line3", m.content[2])
	assert.Equal(t, 1, m.cursorRow)
}

// TestDeleteLineCommand_UndoOnlyLine verifies restoring cleared only line
func TestDeleteLineCommand_UndoOnlyLine(t *testing.T) {
	m := newTestModelWithContent("only line")

	cmd := &DeleteLineCommand{}
	_ = cmd.Execute(m)
	assert.Equal(t, "", m.content[0])

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, "only line", m.content[0])
}

// ============================================================================
// DeleteToEOLCommand Tests (D)
// ============================================================================

// TestDeleteToEOLCommand_Execute verifies deleting from cursor to end of line
func TestDeleteToEOLCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 5 // position at space

	cmd := &DeleteToEOLCommand{row: 0, col: 5}
	err := cmd.Execute(m)

	assert.Equal(t, Executed, err)
	assert.Equal(t, "hello", m.content[0])
	assert.Equal(t, " world", cmd.deletedText)
}

// TestDeleteToEOLCommand_ExecuteAtEnd verifies deleting at end of line is no-op
func TestDeleteToEOLCommand_ExecuteAtEnd(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 5

	cmd := &DeleteToEOLCommand{row: 0, col: 5}
	err := cmd.Execute(m)

	assert.Equal(t, Executed, err)
	assert.Equal(t, "hello", m.content[0])
	assert.Equal(t, "", cmd.deletedText)
}

// TestDeleteToEOLCommand_Undo verifies restoring deleted text
func TestDeleteToEOLCommand_Undo(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 5

	cmd := &DeleteToEOLCommand{row: 0, col: 5}
	_ = cmd.Execute(m)
	assert.Equal(t, "hello", m.content[0])

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, "hello world", m.content[0])
	assert.Equal(t, 5, m.cursorCol)
}

// ============================================================================
// DeleteWordCommand Tests (dw)
// ============================================================================

// TestDeleteWordCommand_Execute verifies deleting word
func TestDeleteWordCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 0

	cmd := &DeleteWordCommand{row: 0, col: 0}
	err := cmd.Execute(m)

	assert.Equal(t, Executed, err)
	assert.Equal(t, "world", m.content[0])
	assert.Equal(t, "hello ", cmd.deletedText)
}

// TestDeleteWordCommand_ExecuteMiddleOfWord verifies deleting from middle of word
func TestDeleteWordCommand_ExecuteMiddleOfWord(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 2 // position on 'l'

	cmd := &DeleteWordCommand{row: 0, col: 2}
	err := cmd.Execute(m)

	assert.Equal(t, Executed, err)
	assert.Equal(t, "heworld", m.content[0])
	assert.Equal(t, "llo ", cmd.deletedText)
}

// TestDeleteWordCommand_ExecuteEmptyLine verifies deleting on empty line is no-op
func TestDeleteWordCommand_ExecuteEmptyLine(t *testing.T) {
	m := newTestModelWithContent("")

	cmd := &DeleteWordCommand{row: 0, col: 0}
	err := cmd.Execute(m)

	assert.Equal(t, Executed, err)
	assert.Equal(t, "", m.content[0])
}

// TestDeleteWordCommand_Undo verifies restoring deleted word
func TestDeleteWordCommand_Undo(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 0

	cmd := &DeleteWordCommand{row: 0, col: 0}
	_ = cmd.Execute(m)
	assert.Equal(t, "world", m.content[0])

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, "hello world", m.content[0])
	assert.Equal(t, 0, m.cursorCol)
}

// ============================================================================
// DeleteLinesCommand Tests (dj/dk)
// ============================================================================

func TestDeleteLinesCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3", "line4")
	m.cursorRow = 1 // position on line2

	cmd := &DeleteLinesCommand{startRow: 1, count: 2}
	err := cmd.Execute(m)

	assert.Equal(t, Executed, err)
	assert.Len(t, m.content, 2)
	assert.Equal(t, "line1", m.content[0])
	assert.Equal(t, "line4", m.content[1])
	assert.Equal(t, []string{"line2", "line3"}, cmd.deletedLines)
}

// TestDeleteLinesCommand_ExecuteFromTop verifies deleting lines starting from first line (dk at line 1)
func TestDeleteLinesCommand_ExecuteFromTop(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.cursorRow = 0

	cmd := &DeleteLinesCommand{startRow: 0, count: 2}
	err := cmd.Execute(m)

	assert.Equal(t, Executed, err)
	assert.Len(t, m.content, 1)
	assert.Equal(t, "line3", m.content[0])
}

// TestDeleteLinesCommand_ExecuteAllLines verifies deleting all lines leaves empty line
func TestDeleteLinesCommand_ExecuteAllLines(t *testing.T) {
	m := newTestModelWithContent("line1", "line2")
	m.cursorRow = 0

	cmd := &DeleteLinesCommand{startRow: 0, count: 2}
	err := cmd.Execute(m)

	assert.Equal(t, Executed, err)
	assert.Len(t, m.content, 1)
	assert.Equal(t, "", m.content[0])
}

// TestDeleteLinesCommand_Undo verifies restoring deleted lines
func TestDeleteLinesCommand_Undo(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3", "line4")
	m.cursorRow = 1

	cmd := &DeleteLinesCommand{startRow: 1, count: 2}
	_ = cmd.Execute(m)
	assert.Len(t, m.content, 2)

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Len(t, m.content, 4)
	assert.Equal(t, "line1", m.content[0])
	assert.Equal(t, "line2", m.content[1])
	assert.Equal(t, "line3", m.content[2])
	assert.Equal(t, "line4", m.content[3])
	assert.Equal(t, 1, m.cursorRow)
}

// TestDeleteLinesCommand_UndoAllLines verifies restoring from empty line state
func TestDeleteLinesCommand_UndoAllLines(t *testing.T) {
	m := newTestModelWithContent("line1", "line2")
	m.cursorRow = 0

	cmd := &DeleteLinesCommand{startRow: 0, count: 2}
	_ = cmd.Execute(m)
	assert.Equal(t, "", m.content[0])

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Len(t, m.content, 2)
	assert.Equal(t, "line1", m.content[0])
	assert.Equal(t, "line2", m.content[1])
}

// ============================================================================
// BackspaceCommand Tests
// ============================================================================

func TestBackspaceCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 3

	cmd := &BackspaceCommand{}
	err := cmd.Execute(m)

	assert.Equal(t, Executed, err)
	assert.Equal(t, "helo", m.content[0])
	assert.Equal(t, 2, m.cursorCol)
	assert.Equal(t, "l", cmd.deletedGrapheme)
	assert.False(t, cmd.joinedLine)
}

// TestBackspaceCommand_ExecuteAtLineStart verifies joining with previous line
func TestBackspaceCommand_ExecuteAtLineStart(t *testing.T) {
	m := newTestModelWithContent("hello", "world")
	m.cursorRow = 1
	m.cursorCol = 0

	cmd := &BackspaceCommand{}
	err := cmd.Execute(m)

	assert.Equal(t, Executed, err)
	assert.Len(t, m.content, 1)
	assert.Equal(t, "helloworld", m.content[0])
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 5, m.cursorCol)
	assert.True(t, cmd.joinedLine)
	assert.Equal(t, "world", cmd.joinedText)
}

// TestBackspaceCommand_ExecuteAtContentStart verifies no-op at content start
func TestBackspaceCommand_ExecuteAtContentStart(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorRow = 0
	m.cursorCol = 0

	cmd := &BackspaceCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Skipped, result)
	assert.Equal(t, "hello", m.content[0])
}

// TestBackspaceCommand_Undo verifies restoring deleted char
func TestBackspaceCommand_Undo(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 3

	cmd := &BackspaceCommand{}
	_ = cmd.Execute(m)
	assert.Equal(t, "helo", m.content[0])

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, "hello", m.content[0])
	assert.Equal(t, 3, m.cursorCol)
}

// TestBackspaceCommand_UndoLineJoin verifies restoring split lines
func TestBackspaceCommand_UndoLineJoin(t *testing.T) {
	m := newTestModelWithContent("hello", "world")
	m.cursorRow = 1
	m.cursorCol = 0

	cmd := &BackspaceCommand{}
	_ = cmd.Execute(m)
	assert.Len(t, m.content, 1)

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Len(t, m.content, 2)
	assert.Equal(t, "hello", m.content[0])
	assert.Equal(t, "world", m.content[1])
	assert.Equal(t, 1, m.cursorRow)
	assert.Equal(t, 0, m.cursorCol)
}

// ============================================================================
// DeleteKeyCommand Tests
// ============================================================================

// TestDeleteKeyCommand_Execute verifies deleting char at cursor
func TestDeleteKeyCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 2

	cmd := &DeleteKeyCommand{}
	err := cmd.Execute(m)

	assert.Equal(t, Executed, err)
	assert.Equal(t, "helo", m.content[0])
	assert.Equal(t, "l", cmd.deletedGrapheme)
	assert.False(t, cmd.joinedLine)
}

// TestDeleteKeyCommand_ExecuteAtLineEnd verifies joining with next line
func TestDeleteKeyCommand_ExecuteAtLineEnd(t *testing.T) {
	m := newTestModelWithContent("hello", "world")
	m.cursorRow = 0
	m.cursorCol = 5

	cmd := &DeleteKeyCommand{}
	err := cmd.Execute(m)

	assert.Equal(t, Executed, err)
	assert.Len(t, m.content, 1)
	assert.Equal(t, "helloworld", m.content[0])
	assert.True(t, cmd.joinedLine)
	assert.Equal(t, "world", cmd.joinedText)
}

// TestDeleteKeyCommand_ExecuteAtLastLineEnd verifies Skipped at end of content
func TestDeleteKeyCommand_ExecuteAtLastLineEnd(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 5

	cmd := &DeleteKeyCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Skipped, result)
	assert.Equal(t, "hello", m.content[0])
}

// TestDeleteKeyCommand_Undo verifies restoring deleted char
func TestDeleteKeyCommand_Undo(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 2

	cmd := &DeleteKeyCommand{}
	_ = cmd.Execute(m)
	assert.Equal(t, "helo", m.content[0])

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, "hello", m.content[0])
	assert.Equal(t, 2, m.cursorCol)
}

// TestDeleteKeyCommand_UndoLineJoin verifies restoring split lines
func TestDeleteKeyCommand_UndoLineJoin(t *testing.T) {
	m := newTestModelWithContent("hello", "world")
	m.cursorRow = 0
	m.cursorCol = 5

	cmd := &DeleteKeyCommand{}
	_ = cmd.Execute(m)
	assert.Len(t, m.content, 1)

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Len(t, m.content, 2)
	assert.Equal(t, "hello", m.content[0])
	assert.Equal(t, "world", m.content[1])
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 5, m.cursorCol)
}

// ============================================================================
// KillToLineStartCommand Tests (Ctrl+U)
// ============================================================================

// TestKillToLineStartCommand_Execute verifies deleting to line start
func TestKillToLineStartCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 6

	cmd := &KillToLineStartCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "world", m.content[0])
	assert.Equal(t, 0, m.cursorCol)
	assert.Equal(t, "hello ", cmd.deletedText)
}

// TestKillToLineStartCommand_ExecuteAtStart verifies no-op at line start
func TestKillToLineStartCommand_ExecuteAtStart(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 0

	cmd := &KillToLineStartCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Skipped, result)
	assert.Equal(t, "hello", m.content[0])
}

// TestKillToLineStartCommand_ExecuteAtEnd verifies deleting entire line content
func TestKillToLineStartCommand_ExecuteAtEnd(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 5

	cmd := &KillToLineStartCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "", m.content[0])
	assert.Equal(t, 0, m.cursorCol)
	assert.Equal(t, "hello", cmd.deletedText)
}

// TestKillToLineStartCommand_Undo verifies restoring deleted text
func TestKillToLineStartCommand_Undo(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 6

	cmd := &KillToLineStartCommand{}
	_ = cmd.Execute(m)
	assert.Equal(t, "world", m.content[0])

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, "hello world", m.content[0])
	assert.Equal(t, 6, m.cursorCol)
}

// ============================================================================
// KillToLineEndCommand Tests (Ctrl+K)
// ============================================================================

// TestKillToLineEndCommand_Execute verifies deleting to line end
func TestKillToLineEndCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 6

	cmd := &KillToLineEndCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "hello ", m.content[0])
	assert.Equal(t, 6, m.cursorCol)
	assert.Equal(t, "world", cmd.deletedText)
}

// TestKillToLineEndCommand_ExecuteAtEnd verifies no-op at line end
func TestKillToLineEndCommand_ExecuteAtEnd(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 5

	cmd := &KillToLineEndCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Skipped, result)
	assert.Equal(t, "hello", m.content[0])
}

// TestKillToLineEndCommand_ExecuteAtStart verifies deleting entire line content
func TestKillToLineEndCommand_ExecuteAtStart(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 0

	cmd := &KillToLineEndCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "", m.content[0])
	assert.Equal(t, 0, m.cursorCol)
	assert.Equal(t, "hello", cmd.deletedText)
}

// TestKillToLineEndCommand_Undo verifies restoring deleted text
func TestKillToLineEndCommand_Undo(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 6

	cmd := &KillToLineEndCommand{}
	_ = cmd.Execute(m)
	assert.Equal(t, "hello ", m.content[0])

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, "hello world", m.content[0])
	assert.Equal(t, 6, m.cursorCol)
}

// TestKillToLineEndCommand_Metadata verifies command metadata
func TestKillToLineEndCommand_Metadata(t *testing.T) {
	cmd := &KillToLineEndCommand{}
	assert.Equal(t, "<ctrl+k>", cmd.Keys()[0])
	assert.Equal(t, ModeInsert, cmd.Mode())
	assert.Equal(t, "kill.to_line_end", cmd.ID())
	assert.True(t, cmd.IsUndoable())
	assert.True(t, cmd.ChangesContent())
	assert.False(t, cmd.IsModeChange())
}

// TestKillToLineStartCommand_Metadata verifies command metadata
func TestKillToLineStartCommand_Metadata(t *testing.T) {
	cmd := &KillToLineStartCommand{}
	assert.Equal(t, "<ctrl+u>", cmd.Keys()[0])
	assert.Equal(t, ModeInsert, cmd.Mode())
	assert.Equal(t, "kill.to_line_start", cmd.ID())
	assert.True(t, cmd.IsUndoable())
	assert.True(t, cmd.ChangesContent())
	assert.False(t, cmd.IsModeChange())
}

// ============================================================================
// Metadata Tests for Delete Commands
// ============================================================================

// TestDeleteCharCommand_Metadata verifies command metadata
func TestDeleteCharCommand_Metadata(t *testing.T) {
	cmd := &DeleteCharCommand{}
	assert.Equal(t, []string{"x"}, cmd.Keys())
	assert.Equal(t, ModeNormal, cmd.Mode())
	assert.Equal(t, "delete.char", cmd.ID())
	assert.True(t, cmd.IsUndoable())
	assert.True(t, cmd.ChangesContent())
	assert.False(t, cmd.IsModeChange())
}

// TestDeleteLineCommand_Metadata verifies command metadata
func TestDeleteLineCommand_Metadata(t *testing.T) {
	cmd := &DeleteLineCommand{}
	assert.Equal(t, []string{"dd"}, cmd.Keys())
	assert.Equal(t, ModeNormal, cmd.Mode())
	assert.Equal(t, "delete.line", cmd.ID())
	assert.True(t, cmd.IsUndoable())
	assert.True(t, cmd.ChangesContent())
	assert.False(t, cmd.IsModeChange())
}

// TestDeleteToEOLCommand_Metadata verifies command metadata
func TestDeleteToEOLCommand_Metadata(t *testing.T) {
	cmd := &DeleteToEOLCommand{}
	assert.Equal(t, []string{"D"}, cmd.Keys())
	assert.Equal(t, ModeNormal, cmd.Mode())
	assert.Equal(t, "delete.to_eol", cmd.ID())
	assert.True(t, cmd.IsUndoable())
	assert.True(t, cmd.ChangesContent())
	assert.False(t, cmd.IsModeChange())
}

// TestDeleteWordCommand_Metadata verifies command metadata
func TestDeleteWordCommand_Metadata(t *testing.T) {
	cmd := &DeleteWordCommand{}
	assert.Equal(t, []string{"dw"}, cmd.Keys())
	assert.Equal(t, ModeNormal, cmd.Mode())
	assert.Equal(t, "delete.word", cmd.ID())
	assert.True(t, cmd.IsUndoable())
	assert.True(t, cmd.ChangesContent())
	assert.False(t, cmd.IsModeChange())
}

// TestDeleteLinesCommand_Metadata verifies command metadata
func TestDeleteLinesCommand_Metadata(t *testing.T) {
	cmd := &DeleteLinesCommand{}
	assert.Equal(t, []string{"dj"}, cmd.Keys())
	assert.Equal(t, ModeNormal, cmd.Mode())
	assert.Equal(t, "delete.lines", cmd.ID())
	assert.True(t, cmd.IsUndoable())
	assert.True(t, cmd.ChangesContent())
	assert.False(t, cmd.IsModeChange())
}

// TestDeleteLinesDownCommand_Execute verifies dj command
func TestDeleteLinesDownCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.cursorRow = 0

	cmd := &DeleteLinesDownCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Len(t, m.content, 1)
	assert.Equal(t, "line3", m.content[0])
}

// TestDeleteLinesDownCommand_ExecuteAtLastLine verifies dj at last line
func TestDeleteLinesDownCommand_ExecuteAtLastLine(t *testing.T) {
	m := newTestModelWithContent("line1", "line2")
	m.cursorRow = 1 // last line

	cmd := &DeleteLinesDownCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Len(t, m.content, 1)
	assert.Equal(t, "line1", m.content[0])
}

// TestDeleteLinesDownCommand_Undo verifies undoing dj
func TestDeleteLinesDownCommand_Undo(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.cursorRow = 0

	cmd := &DeleteLinesDownCommand{}
	_ = cmd.Execute(m)
	assert.Len(t, m.content, 1)

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Len(t, m.content, 3)
	assert.Equal(t, "line1", m.content[0])
	assert.Equal(t, "line2", m.content[1])
	assert.Equal(t, "line3", m.content[2])
}

// TestDeleteLinesDownCommand_UndoOnlyLine verifies undoing when all lines deleted
func TestDeleteLinesDownCommand_UndoOnlyLine(t *testing.T) {
	m := newTestModelWithContent("line1", "line2")
	m.cursorRow = 0

	cmd := &DeleteLinesDownCommand{}
	_ = cmd.Execute(m)
	assert.Len(t, m.content, 1)
	assert.Equal(t, "", m.content[0])

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Len(t, m.content, 2)
	assert.Equal(t, "line1", m.content[0])
	assert.Equal(t, "line2", m.content[1])
}

// TestDeleteLinesDownCommand_Metadata verifies command metadata
func TestDeleteLinesDownCommand_Metadata(t *testing.T) {
	cmd := &DeleteLinesDownCommand{}
	assert.Equal(t, []string{"dj"}, cmd.Keys())
	assert.Equal(t, ModeNormal, cmd.Mode())
	assert.Equal(t, "delete.lines_down", cmd.ID())
	assert.True(t, cmd.IsUndoable())
	assert.True(t, cmd.ChangesContent())
	assert.False(t, cmd.IsModeChange())
}

// TestDeleteLinesUpCommand_Execute verifies dk command
func TestDeleteLinesUpCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.cursorRow = 1

	cmd := &DeleteLinesUpCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Len(t, m.content, 1)
	assert.Equal(t, "line3", m.content[0])
}

// TestDeleteLinesUpCommand_ExecuteAtFirstLine verifies dk at first line
func TestDeleteLinesUpCommand_ExecuteAtFirstLine(t *testing.T) {
	m := newTestModelWithContent("line1", "line2")
	m.cursorRow = 0 // first line

	cmd := &DeleteLinesUpCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Len(t, m.content, 1)
	assert.Equal(t, "line2", m.content[0])
}

// TestDeleteLinesUpCommand_Undo verifies undoing dk
func TestDeleteLinesUpCommand_Undo(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.cursorRow = 1

	cmd := &DeleteLinesUpCommand{}
	_ = cmd.Execute(m)
	assert.Len(t, m.content, 1)

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Len(t, m.content, 3)
	assert.Equal(t, "line1", m.content[0])
	assert.Equal(t, "line2", m.content[1])
	assert.Equal(t, "line3", m.content[2])
}

// TestDeleteLinesUpCommand_UndoOnlyLine verifies undoing when all lines deleted
func TestDeleteLinesUpCommand_UndoOnlyLine(t *testing.T) {
	m := newTestModelWithContent("line1", "line2")
	m.cursorRow = 1

	cmd := &DeleteLinesUpCommand{}
	_ = cmd.Execute(m)
	assert.Len(t, m.content, 1)
	assert.Equal(t, "", m.content[0])

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Len(t, m.content, 2)
	assert.Equal(t, "line1", m.content[0])
	assert.Equal(t, "line2", m.content[1])
}

// TestDeleteLinesUpCommand_Metadata verifies command metadata
func TestDeleteLinesUpCommand_Metadata(t *testing.T) {
	cmd := &DeleteLinesUpCommand{}
	assert.Equal(t, []string{"dk"}, cmd.Keys())
	assert.Equal(t, ModeNormal, cmd.Mode())
	assert.Equal(t, "delete.lines_up", cmd.ID())
	assert.True(t, cmd.IsUndoable())
	assert.True(t, cmd.ChangesContent())
	assert.False(t, cmd.IsModeChange())
}

// TestBackspaceCommand_Metadata verifies command metadata
func TestBackspaceCommand_Metadata(t *testing.T) {
	cmd := &BackspaceCommand{}
	assert.Equal(t, []string{"<backspace>"}, cmd.Keys())
	assert.Equal(t, ModeInsert, cmd.Mode())
	assert.Equal(t, "delete.backspace", cmd.ID())
	assert.True(t, cmd.IsUndoable())
	assert.True(t, cmd.ChangesContent())
	assert.False(t, cmd.IsModeChange())
}

// TestDeleteKeyCommand_Metadata verifies command metadata
func TestDeleteKeyCommand_Metadata(t *testing.T) {
	cmd := &DeleteKeyCommand{}
	assert.Equal(t, []string{"<delete>"}, cmd.Keys())
	assert.Equal(t, ModeInsert, cmd.Mode())
	assert.Equal(t, "delete.key", cmd.ID())
	assert.True(t, cmd.IsUndoable())
	assert.True(t, cmd.ChangesContent())
	assert.False(t, cmd.IsModeChange())
}

// ============================================================================
// DeleteToFirstLineCommand Tests (dgg)
// ============================================================================

func TestDeleteToFirstLineCommand_Execute_FromMiddle(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3", "line4", "line5")
	m.cursorRow = 2 // On line3

	cmd := &DeleteToFirstLineCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, []string{"line4", "line5"}, m.content)
	assert.Equal(t, 0, m.cursorRow)
}

func TestDeleteToFirstLineCommand_Execute_FromFirst(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.cursorRow = 0

	cmd := &DeleteToFirstLineCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, []string{"line2", "line3"}, m.content)
	assert.Equal(t, 0, m.cursorRow)
}

func TestDeleteToFirstLineCommand_Execute_FromLast(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.cursorRow = 2 // On line3 (last line)

	cmd := &DeleteToFirstLineCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, []string{""}, m.content) // All deleted, leaves empty line
	assert.Equal(t, 0, m.cursorRow)
}

func TestDeleteToFirstLineCommand_Undo(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3", "line4", "line5")
	m.cursorRow = 2

	cmd := &DeleteToFirstLineCommand{}
	cmd.Execute(m)
	assert.Equal(t, []string{"line4", "line5"}, m.content)

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, []string{"line1", "line2", "line3", "line4", "line5"}, m.content)
	assert.Equal(t, 2, m.cursorRow)
}

func TestDeleteToFirstLineCommand_Metadata(t *testing.T) {
	cmd := &DeleteToFirstLineCommand{}
	assert.Equal(t, []string{"dgg"}, cmd.Keys())
	assert.Equal(t, ModeNormal, cmd.Mode())
	assert.Equal(t, "delete.to_first_line", cmd.ID())
	assert.True(t, cmd.IsUndoable())
	assert.True(t, cmd.ChangesContent())
	assert.False(t, cmd.IsModeChange())
}

// ============================================================================
// DeleteToLastLineCommand Tests (dG)
// ============================================================================

func TestDeleteToLastLineCommand_Execute_FromMiddle(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3", "line4", "line5")
	m.cursorRow = 2 // On line3

	cmd := &DeleteToLastLineCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, []string{"line1", "line2"}, m.content)
	assert.Equal(t, 1, m.cursorRow) // On last remaining line
}

func TestDeleteToLastLineCommand_Execute_FromFirst(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.cursorRow = 0

	cmd := &DeleteToLastLineCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, []string{""}, m.content) // All deleted, leaves empty line
	assert.Equal(t, 0, m.cursorRow)
}

func TestDeleteToLastLineCommand_Execute_FromLast(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.cursorRow = 2 // On line3 (last line)

	cmd := &DeleteToLastLineCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, []string{"line1", "line2"}, m.content)
	assert.Equal(t, 1, m.cursorRow)
}

func TestDeleteToLastLineCommand_Undo(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3", "line4", "line5")
	m.cursorRow = 2

	cmd := &DeleteToLastLineCommand{}
	cmd.Execute(m)
	assert.Equal(t, []string{"line1", "line2"}, m.content)

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, []string{"line1", "line2", "line3", "line4", "line5"}, m.content)
	assert.Equal(t, 2, m.cursorRow)
}

func TestDeleteToLastLineCommand_Metadata(t *testing.T) {
	cmd := &DeleteToLastLineCommand{}
	assert.Equal(t, []string{"dG"}, cmd.Keys())
	assert.Equal(t, ModeNormal, cmd.Mode())
	assert.Equal(t, "delete.to_last_line", cmd.ID())
	assert.True(t, cmd.IsUndoable())
	assert.True(t, cmd.ChangesContent())
	assert.False(t, cmd.IsModeChange())
}

// ============================================================================
// Yank Register Population Tests
// These tests verify that delete commands populate lastYankedText and lastYankWasLinewise
// ============================================================================

// TestDeleteCharCommand_PopulatesYankRegister verifies x sets lastYankedText and lastYankWasLinewise = false
func TestDeleteCharCommand_PopulatesYankRegister(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 2 // position on 'l'

	cmd := &DeleteCharCommand{}
	cmd.Execute(m)

	assert.Equal(t, "l", m.lastYankedText)
	assert.False(t, m.lastYankWasLinewise)
}

// TestDeleteLineCommand_PopulatesYankRegister verifies dd sets lastYankedText and lastYankWasLinewise = true
func TestDeleteLineCommand_PopulatesYankRegister(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.cursorRow = 1 // position on line2

	cmd := &DeleteLineCommand{}
	cmd.Execute(m)

	assert.Equal(t, "line2", m.lastYankedText)
	assert.True(t, m.lastYankWasLinewise)
}

// TestDeleteWordCommand_PopulatesYankRegister verifies dw sets lastYankedText and lastYankWasLinewise = false
func TestDeleteWordCommand_PopulatesYankRegister(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 0

	cmd := &DeleteWordCommand{}
	cmd.Execute(m)

	assert.Equal(t, "hello ", m.lastYankedText)
	assert.False(t, m.lastYankWasLinewise)
}

// TestDeleteToEOLCommand_PopulatesYankRegister verifies D sets lastYankedText and lastYankWasLinewise = false
func TestDeleteToEOLCommand_PopulatesYankRegister(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 5 // position at space

	cmd := &DeleteToEOLCommand{}
	cmd.Execute(m)

	assert.Equal(t, " world", m.lastYankedText)
	assert.False(t, m.lastYankWasLinewise)
}

// ============================================================================
// Emoji/Grapheme-Aware Delete Tests
// These tests verify that delete commands properly handle multi-byte Unicode
// characters like emoji, ZWJ sequences, and combining characters.
// ============================================================================

// TestDeleteCharCommand_Emoji verifies x deletes single emoji as one unit
func TestDeleteCharCommand_Emoji(t *testing.T) {
	m := newTestModelWithContent("h😀llo")
	m.cursorCol = 1 // position on emoji (grapheme index 1)

	cmd := &DeleteCharCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "hllo", m.content[0])
	assert.Equal(t, "😀", cmd.deletedGrapheme)
	assert.Equal(t, 1, m.cursorCol) // cursor stays at position 1
}

// TestDeleteCharCommand_ZWJSequence verifies x deletes ZWJ family emoji as one unit
func TestDeleteCharCommand_ZWJSequence(t *testing.T) {
	m := newTestModelWithContent("a👨‍👩‍👧‍👦b") // Family emoji is 1 grapheme
	m.cursorCol = 1                           // position on family emoji

	cmd := &DeleteCharCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "ab", m.content[0])
	assert.Equal(t, "👨‍👩‍👧‍👦", cmd.deletedGrapheme)
}

// TestDeleteCharCommand_CombiningCharacter verifies x deletes combining sequence as one unit
func TestDeleteCharCommand_CombiningCharacter(t *testing.T) {
	// é as e + combining acute accent = 1 grapheme
	m := newTestModelWithContent("café")
	m.cursorCol = 3 // position on 'é'

	cmd := &DeleteCharCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "caf", m.content[0])
	// The deleted grapheme should be the combining sequence
	assert.Equal(t, 1, GraphemeCount(cmd.deletedGrapheme)) // 1 grapheme, possibly multiple bytes
}

// TestDeleteCharCommand_LastGrapheme verifies x on last grapheme of line
func TestDeleteCharCommand_LastGrapheme(t *testing.T) {
	m := newTestModelWithContent("ab😀")
	m.cursorCol = 2 // position on emoji (last grapheme)

	cmd := &DeleteCharCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "ab", m.content[0])
	assert.Equal(t, "😀", cmd.deletedGrapheme)
	assert.Equal(t, 1, m.cursorCol) // cursor moves back since we deleted last grapheme
}

// TestDeleteCharCommand_Undo_Emoji verifies undo restores emoji correctly
func TestDeleteCharCommand_Undo_Emoji(t *testing.T) {
	m := newTestModelWithContent("h😀llo")
	m.cursorCol = 1

	cmd := &DeleteCharCommand{}
	cmd.Execute(m)
	assert.Equal(t, "hllo", m.content[0])

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, "h😀llo", m.content[0])
	assert.Equal(t, 1, m.cursorCol)
}

// TestBackspaceCommand_Emoji verifies backspace deletes preceding emoji as one unit
func TestBackspaceCommand_Emoji(t *testing.T) {
	m := newTestModelWithContent("h😀llo")
	m.cursorCol = 2 // position after emoji

	cmd := &BackspaceCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "hllo", m.content[0])
	assert.Equal(t, "😀", cmd.deletedGrapheme)
	assert.Equal(t, 1, m.cursorCol) // cursor moves back 1 grapheme
}

// TestBackspaceCommand_ZWJSequence verifies backspace deletes ZWJ sequence as one unit
func TestBackspaceCommand_ZWJSequence(t *testing.T) {
	m := newTestModelWithContent("a👨‍👩‍👧‍👦b")
	m.cursorCol = 2 // position after family emoji

	cmd := &BackspaceCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "ab", m.content[0])
	assert.Equal(t, "👨‍👩‍👧‍👦", cmd.deletedGrapheme)
	assert.Equal(t, 1, m.cursorCol)
}

// TestBackspaceCommand_Undo_Emoji verifies undo restores emoji correctly
func TestBackspaceCommand_Undo_Emoji(t *testing.T) {
	m := newTestModelWithContent("h😀llo")
	m.cursorCol = 2

	cmd := &BackspaceCommand{}
	cmd.Execute(m)
	assert.Equal(t, "hllo", m.content[0])

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, "h😀llo", m.content[0])
	assert.Equal(t, 2, m.cursorCol)
}

// TestDeleteWordCommand_Emoji verifies dw deletes word (emoji is punctuation, not part of word)
func TestDeleteWordCommand_Emoji(t *testing.T) {
	m := newTestModelWithContent("hello😀 world")
	m.cursorCol = 0

	cmd := &DeleteWordCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	// "hello" is a word (alphanumeric), emoji is punctuation
	// dw from start of word deletes the word, which is "hello"
	// The emoji stays since it's punctuation (different word type)
	assert.Equal(t, "😀 world", m.content[0])
	assert.Equal(t, "hello", cmd.deletedText)
}

// TestDeleteWordCommand_EmojiAtStart verifies dw starting at emoji
func TestDeleteWordCommand_EmojiAtStart(t *testing.T) {
	m := newTestModelWithContent("😀🎉 hello")
	m.cursorCol = 0

	cmd := &DeleteWordCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	// Emoji sequence is punctuation, so dw deletes punctuation + space
	assert.Equal(t, "hello", m.content[0])
}

// TestDeleteWordCommand_Undo_Emoji verifies undo restores word with emoji
func TestDeleteWordCommand_Undo_Emoji(t *testing.T) {
	m := newTestModelWithContent("hello😀 world")
	m.cursorCol = 0

	cmd := &DeleteWordCommand{}
	cmd.Execute(m)
	assert.Equal(t, "😀 world", m.content[0])

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, "hello😀 world", m.content[0])
	assert.Equal(t, 0, m.cursorCol)
}

// TestDeleteToEOLCommand_Emoji verifies D deletes from cursor (at emoji) to end of line
func TestDeleteToEOLCommand_Emoji(t *testing.T) {
	m := newTestModelWithContent("he😀llo world")
	m.cursorCol = 2 // position at emoji

	cmd := &DeleteToEOLCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "he", m.content[0])
	assert.Equal(t, "😀llo world", cmd.deletedText)
}

// TestDeleteToEOLCommand_Undo_Emoji verifies undo restores line with emoji
func TestDeleteToEOLCommand_Undo_Emoji(t *testing.T) {
	m := newTestModelWithContent("he😀llo world")
	m.cursorCol = 2

	cmd := &DeleteToEOLCommand{}
	cmd.Execute(m)
	assert.Equal(t, "he", m.content[0])

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, "he😀llo world", m.content[0])
	assert.Equal(t, 2, m.cursorCol)
}

// TestDeleteKeyCommand_Emoji verifies delete key removes emoji at cursor
func TestDeleteKeyCommand_Emoji(t *testing.T) {
	m := newTestModelWithContent("h😀llo")
	m.cursorCol = 1 // position on emoji

	cmd := &DeleteKeyCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "hllo", m.content[0])
	assert.Equal(t, "😀", cmd.deletedGrapheme)
}

// TestDeleteKeyCommand_Undo_Emoji verifies undo restores emoji
func TestDeleteKeyCommand_Undo_Emoji(t *testing.T) {
	m := newTestModelWithContent("h😀llo")
	m.cursorCol = 1

	cmd := &DeleteKeyCommand{}
	cmd.Execute(m)
	assert.Equal(t, "hllo", m.content[0])

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, "h😀llo", m.content[0])
	assert.Equal(t, 1, m.cursorCol)
}

// TestKillToLineStartCommand_Emoji verifies Ctrl+U with emoji
func TestKillToLineStartCommand_Emoji(t *testing.T) {
	m := newTestModelWithContent("h😀llo world")
	m.cursorCol = 3 // position after "h😀l"

	cmd := &KillToLineStartCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "lo world", m.content[0])
	assert.Equal(t, "h😀l", cmd.deletedText)
	assert.Equal(t, 0, m.cursorCol)
}

// TestKillToLineStartCommand_Undo_Emoji verifies undo with emoji
func TestKillToLineStartCommand_Undo_Emoji(t *testing.T) {
	m := newTestModelWithContent("h😀llo world")
	m.cursorCol = 3

	cmd := &KillToLineStartCommand{}
	cmd.Execute(m)
	assert.Equal(t, "lo world", m.content[0])

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, "h😀llo world", m.content[0])
	assert.Equal(t, 3, m.cursorCol)
}

// TestKillToLineEndCommand_Emoji verifies Ctrl+K with emoji
func TestKillToLineEndCommand_Emoji(t *testing.T) {
	m := newTestModelWithContent("hello 😀🎉 world")
	m.cursorCol = 6 // position at first emoji

	cmd := &KillToLineEndCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "hello ", m.content[0])
	assert.Equal(t, "😀🎉 world", cmd.deletedText)
}

// TestKillToLineEndCommand_Undo_Emoji verifies undo with emoji
func TestKillToLineEndCommand_Undo_Emoji(t *testing.T) {
	m := newTestModelWithContent("hello 😀🎉 world")
	m.cursorCol = 6

	cmd := &KillToLineEndCommand{}
	cmd.Execute(m)
	assert.Equal(t, "hello ", m.content[0])

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, "hello 😀🎉 world", m.content[0])
	assert.Equal(t, 6, m.cursorCol)
}

// TestDeleteLineCommand_WithEmoji verifies dd unchanged with emoji content
func TestDeleteLineCommand_WithEmoji(t *testing.T) {
	m := newTestModelWithContent("line1", "hello 😀 world", "line3")
	m.cursorRow = 1

	cmd := &DeleteLineCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Len(t, m.content, 2)
	assert.Equal(t, "line1", m.content[0])
	assert.Equal(t, "line3", m.content[1])
	assert.Equal(t, "hello 😀 world", cmd.deletedLine)
}

// TestDeleteLineCommand_Undo_WithEmoji verifies undo restores line with emoji
func TestDeleteLineCommand_Undo_WithEmoji(t *testing.T) {
	m := newTestModelWithContent("line1", "hello 😀 world", "line3")
	m.cursorRow = 1

	cmd := &DeleteLineCommand{}
	cmd.Execute(m)
	assert.Len(t, m.content, 2)

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Len(t, m.content, 3)
	assert.Equal(t, "hello 😀 world", m.content[1])
}

// TestBackspaceCommand_JoinLine_WithEmoji verifies joining lines with emoji
func TestBackspaceCommand_JoinLine_WithEmoji(t *testing.T) {
	m := newTestModelWithContent("hello 😀", "world")
	m.cursorRow = 1
	m.cursorCol = 0

	cmd := &BackspaceCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Len(t, m.content, 1)
	assert.Equal(t, "hello 😀world", m.content[0])
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 7, m.cursorCol) // GraphemeCount("hello 😀") = 7
}

// TestBackspaceCommand_JoinLine_Undo_WithEmoji verifies undo split with emoji
func TestBackspaceCommand_JoinLine_Undo_WithEmoji(t *testing.T) {
	m := newTestModelWithContent("hello 😀", "world")
	m.cursorRow = 1
	m.cursorCol = 0

	cmd := &BackspaceCommand{}
	cmd.Execute(m)
	assert.Len(t, m.content, 1)

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Len(t, m.content, 2)
	assert.Equal(t, "hello 😀", m.content[0])
	assert.Equal(t, "world", m.content[1])
	assert.Equal(t, 1, m.cursorRow)
	assert.Equal(t, 0, m.cursorCol)
}

// TestNoPartialEmojiCorruption verifies no partial emoji corruption after x
func TestNoPartialEmojiCorruption(t *testing.T) {
	// Test that deleting a character doesn't leave partial bytes
	testCases := []struct {
		name     string
		content  string
		cursorAt int
		expected string
	}{
		{"delete before emoji", "a😀b", 0, "😀b"},
		{"delete emoji", "a😀b", 1, "ab"},
		{"delete after emoji", "a😀b", 2, "a😀"},
		{"delete first emoji in sequence", "😀🎉c", 0, "🎉c"},
		{"delete second emoji in sequence", "😀🎉c", 1, "😀c"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModelWithContent(tc.content)
			m.cursorCol = tc.cursorAt

			cmd := &DeleteCharCommand{}
			cmd.Execute(m)

			assert.Equal(t, tc.expected, m.content[0])
			// Verify the result is valid UTF-8 by counting graphemes
			// (this would panic or return wrong count if there are partial bytes)
			_ = GraphemeCount(m.content[0])
		})
	}
}
