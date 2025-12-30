package vimtextarea

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ============================================================================
// ChangeWordCommand Tests
// ============================================================================

// TestChangeWordCommand_Execute verifies deleting word and entering insert mode
func TestChangeWordCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 0
	m.mode = ModeNormal

	cmd := &ChangeWordCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, " world", m.content[0]) // cw leaves the space (unlike dw)
	assert.Equal(t, "hello", cmd.deletedText)
	assert.Equal(t, ModeInsert, m.mode)
}

// TestChangeWordCommand_ExecuteMiddleOfWord verifies deleting from middle of word
func TestChangeWordCommand_ExecuteMiddleOfWord(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 2 // position on 'l'
	m.mode = ModeNormal

	cmd := &ChangeWordCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "he world", m.content[0]) // cw leaves the space
	assert.Equal(t, "llo", cmd.deletedText)
	assert.Equal(t, ModeInsert, m.mode)
}

// TestChangeWordCommand_ExecuteEmptyLine verifies entering insert mode on empty line
func TestChangeWordCommand_ExecuteEmptyLine(t *testing.T) {
	m := newTestModelWithContent("")
	m.mode = ModeNormal

	cmd := &ChangeWordCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "", m.content[0])
	assert.Equal(t, ModeInsert, m.mode)
}

// TestChangeWordCommand_Undo verifies restoring deleted word and returning to normal mode
func TestChangeWordCommand_Undo(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 0
	m.mode = ModeNormal

	cmd := &ChangeWordCommand{}
	_ = cmd.Execute(m)
	assert.Equal(t, " world", m.content[0])
	assert.Equal(t, ModeInsert, m.mode)

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, "hello world", m.content[0])
	assert.Equal(t, 0, m.cursorCol)
	assert.Equal(t, ModeNormal, m.mode)
}

// TestChangeWordCommand_IsModeChange verifies it returns true
func TestChangeWordCommand_IsModeChange(t *testing.T) {
	cmd := &ChangeWordCommand{}
	assert.True(t, cmd.IsModeChange())
}

// ============================================================================
// ChangeLineCommand Tests (cc)
// ============================================================================

// TestChangeLineCommand_Execute verifies clearing line and entering insert mode
func TestChangeLineCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 3

	cmd := &ChangeLineCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "", m.content[0])
	assert.Equal(t, 0, m.cursorCol)
	assert.Equal(t, ModeInsert, m.mode)
	assert.Equal(t, "hello world", cmd.deletedText)
}

// TestChangeLineCommand_ExecuteEmptyLine verifies changing empty line enters insert mode
func TestChangeLineCommand_ExecuteEmptyLine(t *testing.T) {
	m := newTestModelWithContent("")

	cmd := &ChangeLineCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "", m.content[0])
	assert.Equal(t, ModeInsert, m.mode)
}

// TestChangeLineCommand_Undo verifies restoring line content and returning to normal mode
func TestChangeLineCommand_Undo(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 3

	cmd := &ChangeLineCommand{}
	_ = cmd.Execute(m)
	assert.Equal(t, "", m.content[0])

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, "hello world", m.content[0])
	assert.Equal(t, 0, m.cursorCol)
	assert.Equal(t, ModeNormal, m.mode)
}

// TestChangeLineCommand_IsModeChange verifies it returns true
func TestChangeLineCommand_IsModeChange(t *testing.T) {
	cmd := &ChangeLineCommand{}
	assert.True(t, cmd.IsModeChange())
}

// ============================================================================
// ChangeToEOLCommand Tests (C)
// ============================================================================

// TestChangeToEOLCommand_Execute verifies deleting to EOL and entering insert mode
func TestChangeToEOLCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 6

	cmd := &ChangeToEOLCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "hello ", m.content[0])
	assert.Equal(t, 6, m.cursorCol)
	assert.Equal(t, ModeInsert, m.mode)
	assert.Equal(t, "world", cmd.deletedText)
}

// TestChangeToEOLCommand_ExecuteAtEnd verifies changing at EOL just enters insert mode
func TestChangeToEOLCommand_ExecuteAtEnd(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 5

	cmd := &ChangeToEOLCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "hello", m.content[0])
	assert.Equal(t, ModeInsert, m.mode)
	assert.Equal(t, "", cmd.deletedText)
}

// TestChangeToEOLCommand_Undo verifies restoring text and returning to normal mode
func TestChangeToEOLCommand_Undo(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 6

	cmd := &ChangeToEOLCommand{}
	_ = cmd.Execute(m)
	assert.Equal(t, "hello ", m.content[0])

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, "hello world", m.content[0])
	assert.Equal(t, 6, m.cursorCol)
	assert.Equal(t, ModeNormal, m.mode)
}

// TestChangeToEOLCommand_IsModeChange verifies it returns true
func TestChangeToEOLCommand_IsModeChange(t *testing.T) {
	cmd := &ChangeToEOLCommand{}
	assert.True(t, cmd.IsModeChange())
}

// ============================================================================
// ChangeToLineStartCommand Tests (c0)
// ============================================================================

// TestChangeToLineStartCommand_Execute verifies deleting to start and entering insert mode
func TestChangeToLineStartCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 6

	cmd := &ChangeToLineStartCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "world", m.content[0])
	assert.Equal(t, 0, m.cursorCol)
	assert.Equal(t, ModeInsert, m.mode)
	assert.Equal(t, "hello ", cmd.deletedText)
}

// TestChangeToLineStartCommand_ExecuteAtStart verifies changing at start just enters insert mode
func TestChangeToLineStartCommand_ExecuteAtStart(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 0

	cmd := &ChangeToLineStartCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "hello", m.content[0])
	assert.Equal(t, ModeInsert, m.mode)
	assert.Equal(t, "", cmd.deletedText)
}

// TestChangeToLineStartCommand_Undo verifies restoring text and returning to normal mode
func TestChangeToLineStartCommand_Undo(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 6

	cmd := &ChangeToLineStartCommand{}
	_ = cmd.Execute(m)
	assert.Equal(t, "world", m.content[0])

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, "hello world", m.content[0])
	assert.Equal(t, 6, m.cursorCol)
	assert.Equal(t, ModeNormal, m.mode)
}

// TestChangeToLineStartCommand_IsModeChange verifies it returns true
func TestChangeToLineStartCommand_IsModeChange(t *testing.T) {
	cmd := &ChangeToLineStartCommand{}
	assert.True(t, cmd.IsModeChange())
}

// ============================================================================
// ChangeLinesCommand Tests (cj/ck)
// ============================================================================

// TestChangeLinesCommand_Execute verifies changing multiple lines
func TestChangeLinesCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3", "line4")
	m.cursorRow = 1

	cmd := &ChangeLinesCommand{startRow: 1, count: 2}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Len(t, m.content, 3)
	assert.Equal(t, "line1", m.content[0])
	assert.Equal(t, "", m.content[1]) // Empty line for insertion
	assert.Equal(t, "line4", m.content[2])
	assert.Equal(t, ModeInsert, m.mode)
	assert.Equal(t, []string{"line2", "line3"}, cmd.deletedLines)
}

// TestChangeLinesCommand_ExecuteAllLines verifies changing all lines leaves empty line
func TestChangeLinesCommand_ExecuteAllLines(t *testing.T) {
	m := newTestModelWithContent("line1", "line2")
	m.cursorRow = 0

	cmd := &ChangeLinesCommand{startRow: 0, count: 2}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Len(t, m.content, 1)
	assert.Equal(t, "", m.content[0])
	assert.Equal(t, ModeInsert, m.mode)
}

// TestChangeLinesCommand_Undo verifies restoring lines and returning to normal mode
func TestChangeLinesCommand_Undo(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3", "line4")
	m.cursorRow = 1

	cmd := &ChangeLinesCommand{startRow: 1, count: 2}
	_ = cmd.Execute(m)
	assert.Len(t, m.content, 3)

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Len(t, m.content, 4)
	assert.Equal(t, "line1", m.content[0])
	assert.Equal(t, "line2", m.content[1])
	assert.Equal(t, "line3", m.content[2])
	assert.Equal(t, "line4", m.content[3])
	assert.Equal(t, ModeNormal, m.mode)
}

// TestChangeLinesCommand_UndoAllLines verifies restoring from empty line state
func TestChangeLinesCommand_UndoAllLines(t *testing.T) {
	m := newTestModelWithContent("line1", "line2")
	m.cursorRow = 0

	cmd := &ChangeLinesCommand{startRow: 0, count: 2}
	_ = cmd.Execute(m)
	assert.Equal(t, "", m.content[0])

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Len(t, m.content, 2)
	assert.Equal(t, "line1", m.content[0])
	assert.Equal(t, "line2", m.content[1])
	assert.Equal(t, ModeNormal, m.mode)
}

// TestChangeLinesCommand_IsModeChange verifies it returns true
func TestChangeLinesCommand_IsModeChange(t *testing.T) {
	cmd := &ChangeLinesCommand{}
	assert.True(t, cmd.IsModeChange())
}

// ============================================================================
// ChangeLinesDownCommand Tests (cj)
// ============================================================================

// TestChangeLinesDownCommand_Execute verifies changing current and below line
func TestChangeLinesDownCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.cursorRow = 0

	cmd := &ChangeLinesDownCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Len(t, m.content, 2)
	assert.Equal(t, "", m.content[0]) // Empty line for insertion
	assert.Equal(t, "line3", m.content[1])
	assert.Equal(t, ModeInsert, m.mode)
}

// TestChangeLinesDownCommand_ExecuteAtLastLine verifies changing only current line at bottom
func TestChangeLinesDownCommand_ExecuteAtLastLine(t *testing.T) {
	m := newTestModelWithContent("line1", "line2")
	m.cursorRow = 1 // last line

	cmd := &ChangeLinesDownCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Len(t, m.content, 2)
	assert.Equal(t, "line1", m.content[0])
	assert.Equal(t, "", m.content[1]) // Changed to empty line
	assert.Equal(t, ModeInsert, m.mode)
}

// TestChangeLinesDownCommand_Undo verifies restoring lines
func TestChangeLinesDownCommand_Undo(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.cursorRow = 0

	cmd := &ChangeLinesDownCommand{}
	_ = cmd.Execute(m)
	assert.Len(t, m.content, 2)

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Len(t, m.content, 3)
	assert.Equal(t, "line1", m.content[0])
	assert.Equal(t, "line2", m.content[1])
	assert.Equal(t, "line3", m.content[2])
	assert.Equal(t, ModeNormal, m.mode)
}

// ============================================================================
// ChangeLinesUpCommand Tests (ck)
// ============================================================================

// TestChangeLinesUpCommand_Execute verifies changing current and above line
func TestChangeLinesUpCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.cursorRow = 1

	cmd := &ChangeLinesUpCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Len(t, m.content, 2)
	assert.Equal(t, "", m.content[0]) // Empty line for insertion
	assert.Equal(t, "line3", m.content[1])
	assert.Equal(t, ModeInsert, m.mode)
}

// TestChangeLinesUpCommand_ExecuteAtFirstLine verifies changing only current line at top
func TestChangeLinesUpCommand_ExecuteAtFirstLine(t *testing.T) {
	m := newTestModelWithContent("line1", "line2")
	m.cursorRow = 0 // first line

	cmd := &ChangeLinesUpCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Len(t, m.content, 2)
	assert.Equal(t, "", m.content[0]) // Changed to empty line
	assert.Equal(t, "line2", m.content[1])
	assert.Equal(t, ModeInsert, m.mode)
}

// TestChangeLinesUpCommand_Undo verifies restoring lines
func TestChangeLinesUpCommand_Undo(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.cursorRow = 1

	cmd := &ChangeLinesUpCommand{}
	_ = cmd.Execute(m)
	assert.Len(t, m.content, 2)

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Len(t, m.content, 3)
	assert.Equal(t, "line1", m.content[0])
	assert.Equal(t, "line2", m.content[1])
	assert.Equal(t, "line3", m.content[2])
	assert.Equal(t, ModeNormal, m.mode)
}

// ============================================================================
// Metadata Tests for Change Commands
// ============================================================================

// TestChangeWordCommand_Metadata verifies command metadata
func TestChangeWordCommand_Metadata(t *testing.T) {
	cmd := &ChangeWordCommand{}
	assert.Equal(t, []string{"cw"}, cmd.Keys())
	assert.Equal(t, ModeNormal, cmd.Mode())
	assert.Equal(t, "change.word", cmd.ID())
	assert.True(t, cmd.IsUndoable())
	assert.True(t, cmd.ChangesContent())
	assert.True(t, cmd.IsModeChange())
}

// TestChangeLineCommand_Metadata verifies command metadata
func TestChangeLineCommand_Metadata(t *testing.T) {
	cmd := &ChangeLineCommand{}
	assert.Equal(t, []string{"cc"}, cmd.Keys())
	assert.Equal(t, ModeNormal, cmd.Mode())
	assert.Equal(t, "change.line", cmd.ID())
	assert.True(t, cmd.IsUndoable())
	assert.True(t, cmd.ChangesContent())
	assert.True(t, cmd.IsModeChange())
}

// TestChangeToEOLCommand_Metadata verifies command metadata
func TestChangeToEOLCommand_Metadata(t *testing.T) {
	cmd := &ChangeToEOLCommand{}
	assert.Equal(t, []string{"C"}, cmd.Keys())
	assert.Equal(t, ModeNormal, cmd.Mode())
	assert.Equal(t, "change.to_eol", cmd.ID())
	assert.True(t, cmd.IsUndoable())
	assert.True(t, cmd.ChangesContent())
	assert.True(t, cmd.IsModeChange())
}

// TestChangeToLineStartCommand_Metadata verifies command metadata
func TestChangeToLineStartCommand_Metadata(t *testing.T) {
	cmd := &ChangeToLineStartCommand{}
	assert.Equal(t, []string{"c0"}, cmd.Keys())
	assert.Equal(t, ModeNormal, cmd.Mode())
	assert.Equal(t, "change.to_line_start", cmd.ID())
	assert.True(t, cmd.IsUndoable())
	assert.True(t, cmd.ChangesContent())
	assert.True(t, cmd.IsModeChange())
}

// TestChangeLinesCommand_Metadata verifies command metadata
func TestChangeLinesCommand_Metadata(t *testing.T) {
	cmd := &ChangeLinesCommand{}
	assert.Equal(t, []string{"cj"}, cmd.Keys())
	assert.Equal(t, ModeNormal, cmd.Mode())
	assert.Equal(t, "change.lines", cmd.ID())
	assert.True(t, cmd.IsUndoable())
	assert.True(t, cmd.ChangesContent())
	assert.True(t, cmd.IsModeChange())
}

// TestChangeLinesDownCommand_Metadata verifies command metadata
func TestChangeLinesDownCommand_Metadata(t *testing.T) {
	cmd := &ChangeLinesDownCommand{}
	assert.Equal(t, []string{"cj"}, cmd.Keys())
	assert.Equal(t, ModeNormal, cmd.Mode())
	assert.Equal(t, "change.lines_down", cmd.ID())
	assert.True(t, cmd.IsUndoable())
	assert.True(t, cmd.ChangesContent())
	assert.True(t, cmd.IsModeChange())
}

// TestChangeLinesUpCommand_Metadata verifies command metadata
func TestChangeLinesUpCommand_Metadata(t *testing.T) {
	cmd := &ChangeLinesUpCommand{}
	assert.Equal(t, []string{"ck"}, cmd.Keys())
	assert.Equal(t, ModeNormal, cmd.Mode())
	assert.Equal(t, "change.lines_up", cmd.ID())
	assert.True(t, cmd.IsUndoable())
	assert.True(t, cmd.ChangesContent())
	assert.True(t, cmd.IsModeChange())
}

// ============================================================================
// ChangeToFirstLineCommand Tests (cgg)
// ============================================================================

func TestChangeToFirstLineCommand_Execute_FromMiddle(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3", "line4", "line5")
	m.cursorRow = 2 // On line3
	m.mode = ModeNormal

	cmd := &ChangeToFirstLineCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, []string{"", "line4", "line5"}, m.content)
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, ModeInsert, m.mode)
}

func TestChangeToFirstLineCommand_Execute_FromFirst(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.cursorRow = 0
	m.mode = ModeNormal

	cmd := &ChangeToFirstLineCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, []string{"", "line2", "line3"}, m.content)
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, ModeInsert, m.mode)
}

func TestChangeToFirstLineCommand_Execute_FromLast(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.cursorRow = 2 // On line3 (last line)
	m.mode = ModeNormal

	cmd := &ChangeToFirstLineCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, []string{""}, m.content)
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, ModeInsert, m.mode)
}

func TestChangeToFirstLineCommand_Undo(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3", "line4", "line5")
	m.cursorRow = 2
	m.mode = ModeNormal

	cmd := &ChangeToFirstLineCommand{}
	cmd.Execute(m)
	assert.Equal(t, []string{"", "line4", "line5"}, m.content)

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, []string{"line1", "line2", "line3", "line4", "line5"}, m.content)
	assert.Equal(t, 2, m.cursorRow)
	assert.Equal(t, ModeNormal, m.mode)
}

func TestChangeToFirstLineCommand_Metadata(t *testing.T) {
	cmd := &ChangeToFirstLineCommand{}
	assert.Equal(t, []string{"cgg"}, cmd.Keys())
	assert.Equal(t, ModeNormal, cmd.Mode())
	assert.Equal(t, "change.to_first_line", cmd.ID())
	assert.True(t, cmd.IsUndoable())
	assert.True(t, cmd.ChangesContent())
	assert.True(t, cmd.IsModeChange())
}

// ============================================================================
// ChangeToLastLineCommand Tests (cG)
// ============================================================================

func TestChangeToLastLineCommand_Execute_FromMiddle(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3", "line4", "line5")
	m.cursorRow = 2 // On line3
	m.mode = ModeNormal

	cmd := &ChangeToLastLineCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, []string{"line1", "line2", ""}, m.content)
	assert.Equal(t, 2, m.cursorRow)
	assert.Equal(t, ModeInsert, m.mode)
}

func TestChangeToLastLineCommand_Execute_FromFirst(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.cursorRow = 0
	m.mode = ModeNormal

	cmd := &ChangeToLastLineCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, []string{""}, m.content)
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, ModeInsert, m.mode)
}

func TestChangeToLastLineCommand_Execute_FromLast(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.cursorRow = 2 // On line3 (last line)
	m.mode = ModeNormal

	cmd := &ChangeToLastLineCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, []string{"line1", "line2", ""}, m.content)
	assert.Equal(t, 2, m.cursorRow)
	assert.Equal(t, ModeInsert, m.mode)
}

func TestChangeToLastLineCommand_Undo(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3", "line4", "line5")
	m.cursorRow = 2
	m.mode = ModeNormal

	cmd := &ChangeToLastLineCommand{}
	cmd.Execute(m)
	assert.Equal(t, []string{"line1", "line2", ""}, m.content)

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, []string{"line1", "line2", "line3", "line4", "line5"}, m.content)
	assert.Equal(t, 2, m.cursorRow)
	assert.Equal(t, ModeNormal, m.mode)
}

func TestChangeToLastLineCommand_Metadata(t *testing.T) {
	cmd := &ChangeToLastLineCommand{}
	assert.Equal(t, []string{"cG"}, cmd.Keys())
	assert.Equal(t, ModeNormal, cmd.Mode())
	assert.Equal(t, "change.to_last_line", cmd.ID())
	assert.True(t, cmd.IsUndoable())
	assert.True(t, cmd.ChangesContent())
	assert.True(t, cmd.IsModeChange())
}
