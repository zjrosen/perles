package vimtextarea

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ============================================================================
// VisualDeleteCommand Unit Tests
// ============================================================================

// TestVisualDeleteCommand_Execute_SingleChar tests deleting single selected character
func TestVisualDeleteCommand_Execute_SingleChar(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 3}
	m.cursorRow = 0
	m.cursorCol = 3

	cmd := &VisualDeleteCommand{mode: ModeVisual}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "helo world", m.content[0])
	assert.Equal(t, ModeNormal, m.mode)
	assert.Equal(t, Position{}, m.visualAnchor)
	assert.Equal(t, 3, m.cursorCol, "cursor should be at start of deleted region")
	assert.Equal(t, []string{"l"}, cmd.deletedContent)
}

// TestVisualDeleteCommand_Execute_SingleLine tests deleting part of a single line
func TestVisualDeleteCommand_Execute_SingleLine(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 0}
	m.cursorRow = 0
	m.cursorCol = 4

	cmd := &VisualDeleteCommand{mode: ModeVisual}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, " world", m.content[0])
	assert.Equal(t, ModeNormal, m.mode)
	assert.Equal(t, Position{}, m.visualAnchor)
	assert.Equal(t, 0, m.cursorCol, "cursor should be at start of deleted region")
	assert.Equal(t, []string{"hello"}, cmd.deletedContent)
}

// TestVisualDeleteCommand_Execute_MultiLine tests deleting across multiple lines
func TestVisualDeleteCommand_Execute_MultiLine(t *testing.T) {
	m := newTestModelWithContent("hello", "world", "test")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 2}
	m.cursorRow = 2
	m.cursorCol = 2

	cmd := &VisualDeleteCommand{mode: ModeVisual}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Len(t, m.content, 1)
	assert.Equal(t, "het", m.content[0], "should join first prefix 'he' with last suffix 't'")
	assert.Equal(t, ModeNormal, m.mode)
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 2, m.cursorCol, "cursor should be at start of deleted region")
}

// TestVisualDeleteCommand_Execute_LinewiseMode tests line-wise deletion
func TestVisualDeleteCommand_Execute_LinewiseMode(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.mode = ModeVisualLine
	m.visualAnchor = Position{Row: 0, Col: 0}
	m.cursorRow = 1
	m.cursorCol = 3

	cmd := &VisualDeleteCommand{mode: ModeVisualLine}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Len(t, m.content, 1)
	assert.Equal(t, "line3", m.content[0])
	assert.Equal(t, ModeNormal, m.mode)
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 0, m.cursorCol)
	assert.True(t, cmd.wasLinewise)
	assert.Equal(t, []string{"line1", "line2"}, cmd.deletedContent)
}

// TestVisualDeleteCommand_Execute_LinewiseMode_AllLines tests deleting all lines
func TestVisualDeleteCommand_Execute_LinewiseMode_AllLines(t *testing.T) {
	m := newTestModelWithContent("line1", "line2")
	m.mode = ModeVisualLine
	m.visualAnchor = Position{Row: 0, Col: 0}
	m.cursorRow = 1
	m.cursorCol = 3

	cmd := &VisualDeleteCommand{mode: ModeVisualLine}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Len(t, m.content, 1)
	assert.Equal(t, "", m.content[0], "should leave empty line when all content deleted")
	assert.Equal(t, ModeNormal, m.mode)
}

// TestVisualDeleteCommand_Execute_BackwardSelection tests selection with cursor before anchor
func TestVisualDeleteCommand_Execute_BackwardSelection(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 7}
	m.cursorRow = 0
	m.cursorCol = 2

	cmd := &VisualDeleteCommand{mode: ModeVisual}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "herld", m.content[0])
	assert.Equal(t, ModeNormal, m.mode)
	assert.Equal(t, 2, m.cursorCol, "cursor should be at normalized start")
}

// TestVisualDeleteCommand_Execute_NotInVisualMode tests skipping when not in visual mode
func TestVisualDeleteCommand_Execute_NotInVisualMode(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeNormal

	cmd := &VisualDeleteCommand{mode: ModeVisual}
	result := cmd.Execute(m)

	assert.Equal(t, Skipped, result)
	assert.Equal(t, "hello", m.content[0], "content should not change")
}

// TestVisualDeleteCommand_Keys tests the trigger keys
func TestVisualDeleteCommand_Keys(t *testing.T) {
	cmd := &VisualDeleteCommand{}
	keys := cmd.Keys()
	assert.Contains(t, keys, "d")
	assert.Contains(t, keys, "x")
}

// TestVisualDeleteCommand_Mode tests the mode getter
func TestVisualDeleteCommand_Mode(t *testing.T) {
	cmd := &VisualDeleteCommand{mode: ModeVisual}
	assert.Equal(t, ModeVisual, cmd.Mode())

	cmd2 := &VisualDeleteCommand{mode: ModeVisualLine}
	assert.Equal(t, ModeVisualLine, cmd2.Mode())
}

// TestVisualDeleteCommand_IsUndoable tests undoability
func TestVisualDeleteCommand_IsUndoable(t *testing.T) {
	cmd := &VisualDeleteCommand{}
	assert.True(t, cmd.IsUndoable())
}

// TestVisualDeleteCommand_ChangesContent tests content change flag
func TestVisualDeleteCommand_ChangesContent(t *testing.T) {
	cmd := &VisualDeleteCommand{}
	assert.True(t, cmd.ChangesContent())
}

// TestVisualDeleteCommand_ID tests command ID
func TestVisualDeleteCommand_ID(t *testing.T) {
	cmd := &VisualDeleteCommand{}
	assert.Equal(t, "visual.delete", cmd.ID())
}

// ============================================================================
// VisualDeleteCommand Undo Tests
// ============================================================================

// TestVisualDeleteCommand_Undo_SingleLine tests undoing single line deletion
func TestVisualDeleteCommand_Undo_SingleLine(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 0}
	m.cursorRow = 0
	m.cursorCol = 4

	cmd := &VisualDeleteCommand{mode: ModeVisual}
	cmd.Execute(m)
	assert.Equal(t, " world", m.content[0])

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, "hello world", m.content[0])
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 0, m.cursorCol, "cursor should be at start of restored region")
	assert.Equal(t, ModeNormal, m.mode, "should be in normal mode after undo")
}

// TestVisualDeleteCommand_Undo_MultiLine tests undoing multi-line deletion
func TestVisualDeleteCommand_Undo_MultiLine(t *testing.T) {
	m := newTestModelWithContent("hello", "world", "test")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 2}
	m.cursorRow = 2
	m.cursorCol = 2

	cmd := &VisualDeleteCommand{mode: ModeVisual}
	cmd.Execute(m)
	assert.Equal(t, "het", m.content[0])
	assert.Len(t, m.content, 1)

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Len(t, m.content, 3)
	assert.Equal(t, "hello", m.content[0])
	assert.Equal(t, "world", m.content[1])
	assert.Equal(t, "test", m.content[2])
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 2, m.cursorCol)
}

// TestVisualDeleteCommand_Undo_LinewiseMode tests undoing line-wise deletion
func TestVisualDeleteCommand_Undo_LinewiseMode(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.mode = ModeVisualLine
	m.visualAnchor = Position{Row: 0, Col: 0}
	m.cursorRow = 1
	m.cursorCol = 3

	cmd := &VisualDeleteCommand{mode: ModeVisualLine}
	cmd.Execute(m)
	assert.Equal(t, "line3", m.content[0])
	assert.Len(t, m.content, 1)

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Len(t, m.content, 3)
	assert.Equal(t, "line1", m.content[0])
	assert.Equal(t, "line2", m.content[1])
	assert.Equal(t, "line3", m.content[2])
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 0, m.cursorCol)
}

// TestVisualDeleteCommand_Undo_AllContent tests undoing deletion of all content
func TestVisualDeleteCommand_Undo_AllContent(t *testing.T) {
	m := newTestModelWithContent("hello", "world")
	m.mode = ModeVisualLine
	m.visualAnchor = Position{Row: 0, Col: 0}
	m.cursorRow = 1
	m.cursorCol = 4

	cmd := &VisualDeleteCommand{mode: ModeVisualLine}
	cmd.Execute(m)
	assert.Equal(t, "", m.content[0])
	assert.Len(t, m.content, 1)

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Len(t, m.content, 2)
	assert.Equal(t, "hello", m.content[0])
	assert.Equal(t, "world", m.content[1])
}

// ============================================================================
// Integration Tests - Visual Delete Flow
// ============================================================================

// TestVisualMode_Delete_Flow tests the full v -> move -> d flow
func TestVisualMode_Delete_Flow(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.mode = ModeNormal
	m.cursorRow = 0
	m.cursorCol = 0

	// Enter visual mode with 'v'
	cmdV := &EnterVisualModeCommand{}
	cmdV.Execute(m)
	assert.Equal(t, ModeVisual, m.mode)
	assert.Equal(t, Position{Row: 0, Col: 0}, m.visualAnchor)

	// Move right 4 times to select "hello"
	cmdL := &MoveRightCommand{}
	for i := 0; i < 4; i++ {
		cmdL.Execute(m)
	}
	assert.Equal(t, 4, m.cursorCol)

	// Delete with 'd' (use executeCommand to properly clone)
	cmd := &VisualDeleteCommand{mode: ModeVisual}
	m.executeCommand(cmd)

	assert.Equal(t, ModeNormal, m.mode)
	assert.Equal(t, " world", m.content[0])
	assert.Equal(t, 0, m.cursorCol)
}

// TestVisualMode_DeleteWithX tests deletion with 'x' key
func TestVisualMode_DeleteWithX(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 6}
	m.cursorRow = 0
	m.cursorCol = 10

	// Use executeCommand to properly clone the command (same as real execution)
	cmd := &VisualDeleteCommand{mode: ModeVisual}
	m.executeCommand(cmd)

	assert.Equal(t, ModeNormal, m.mode)
	assert.Equal(t, "hello ", m.content[0])
}

// TestVisualMode_UndoCycle tests delete -> undo -> verify content AND cursor
func TestVisualMode_UndoCycle(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 2}
	m.cursorRow = 0
	m.cursorCol = 7

	// Execute delete through model (so it goes into history)
	cmd := &VisualDeleteCommand{mode: ModeVisual}
	m.executeCommand(cmd)

	assert.Equal(t, "herld", m.content[0])
	assert.Equal(t, ModeNormal, m.mode)
	assert.Equal(t, 2, m.cursorCol)

	// Undo via history
	err := m.history.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, "hello world", m.content[0])
	assert.Equal(t, 2, m.cursorCol, "cursor should be at start of restored region")
	assert.Equal(t, ModeNormal, m.mode)

	// Redo
	err = m.history.Redo(m)
	assert.NoError(t, err)
	assert.Equal(t, "herld", m.content[0])
}

// TestVisualLineMode_Delete_Flow tests V -> move -> d flow
func TestVisualLineMode_Delete_Flow(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.mode = ModeNormal
	m.cursorRow = 0
	m.cursorCol = 0

	// Enter visual line mode with 'V'
	cmdShiftV := &EnterVisualLineModeCommand{}
	cmdShiftV.Execute(m)
	assert.Equal(t, ModeVisualLine, m.mode)

	// Move down to select two lines
	cmdJ := &MoveDownCommand{}
	cmdJ.Execute(m)
	assert.Equal(t, 1, m.cursorRow)

	// Delete with 'd' (use executeCommand to properly clone)
	cmd := &VisualDeleteCommand{mode: ModeVisualLine}
	m.executeCommand(cmd)

	assert.Equal(t, ModeNormal, m.mode)
	assert.Len(t, m.content, 1)
	assert.Equal(t, "line3", m.content[0])
}

// TestVisualMode_Delete_EmptySelection tests deletion when anchor equals cursor
func TestVisualMode_Delete_EmptySelection(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 2}
	m.cursorRow = 0
	m.cursorCol = 2 // Same as anchor - single char selected

	cmd := &VisualDeleteCommand{mode: ModeVisual}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "helo", m.content[0], "single char 'l' should be deleted")
	assert.Equal(t, ModeNormal, m.mode)
}

// TestDefaultRegistry_HasVisualDeleteCommands verifies delete commands are registered for visual modes
func TestDefaultRegistry_HasVisualDeleteCommands(t *testing.T) {
	// ModeVisual should have 'd' and 'x'
	cmdD, okD := DefaultRegistry.Get(ModeVisual, "d")
	assert.True(t, okD, "ModeVisual should have 'd' command")
	assert.NotNil(t, cmdD)

	cmdX, okX := DefaultRegistry.Get(ModeVisual, "x")
	assert.True(t, okX, "ModeVisual should have 'x' command")
	assert.NotNil(t, cmdX)

	// ModeVisualLine should have 'd' and 'x'
	cmdDLine, okDLine := DefaultRegistry.Get(ModeVisualLine, "d")
	assert.True(t, okDLine, "ModeVisualLine should have 'd' command")
	assert.NotNil(t, cmdDLine)

	cmdXLine, okXLine := DefaultRegistry.Get(ModeVisualLine, "x")
	assert.True(t, okXLine, "ModeVisualLine should have 'x' command")
	assert.NotNil(t, cmdXLine)
}

// ============================================================================
// VisualYankCommand Unit Tests
// ============================================================================

// TestVisualYankCommand_Execute tests yanking selected text
func TestVisualYankCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 0}
	m.cursorRow = 0
	m.cursorCol = 4

	cmd := &VisualYankCommand{mode: ModeVisual}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "hello world", m.content[0], "content should NOT be modified")
	assert.Equal(t, ModeNormal, m.mode, "should exit to normal mode")
	assert.Equal(t, Position{}, m.visualAnchor, "visualAnchor should be cleared")
	assert.Equal(t, 4, m.cursorCol, "cursor should stay at current position")
	assert.Equal(t, "hello", m.lastYankedText, "yanked text should be stored in model")
	assert.Equal(t, "hello", cmd.yankedText, "yanked text should be captured in command")
}

// TestVisualYankCommand_Keys tests the trigger key
func TestVisualYankCommand_Keys(t *testing.T) {
	cmd := &VisualYankCommand{}
	keys := cmd.Keys()
	assert.Contains(t, keys, "y")
	assert.Len(t, keys, 1)
}

// TestVisualYankCommand_Mode tests the mode getter
func TestVisualYankCommand_Mode(t *testing.T) {
	cmd := &VisualYankCommand{mode: ModeVisual}
	assert.Equal(t, ModeVisual, cmd.Mode())

	cmd2 := &VisualYankCommand{mode: ModeVisualLine}
	assert.Equal(t, ModeVisualLine, cmd2.Mode())
}

// TestVisualYankCommand_IsUndoable tests that yank is not undoable
func TestVisualYankCommand_IsUndoable(t *testing.T) {
	cmd := &VisualYankCommand{}
	assert.False(t, cmd.IsUndoable(), "yank should not be undoable")
}

// TestVisualYankCommand_ChangesContent tests that yank doesn't change content
func TestVisualYankCommand_ChangesContent(t *testing.T) {
	cmd := &VisualYankCommand{}
	assert.False(t, cmd.ChangesContent(), "yank should not change content")
}

// TestVisualYankCommand_CursorPosition tests cursor stays at current position
func TestVisualYankCommand_CursorPosition(t *testing.T) {
	// Test forward selection - cursor stays at end
	m := newTestModelWithContent("hello world")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 2}
	m.cursorRow = 0
	m.cursorCol = 7

	cmd := &VisualYankCommand{mode: ModeVisual}
	cmd.Execute(m)

	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 7, m.cursorCol, "cursor should stay at current position (col 7)")

	// Test backward selection - cursor stays at position before anchor
	m2 := newTestModelWithContent("hello world")
	m2.mode = ModeVisual
	m2.visualAnchor = Position{Row: 0, Col: 8}
	m2.cursorRow = 0
	m2.cursorCol = 3

	cmd2 := &VisualYankCommand{mode: ModeVisual}
	cmd2.Execute(m2)

	assert.Equal(t, 0, m2.cursorRow)
	assert.Equal(t, 3, m2.cursorCol, "cursor should stay at current position (col 3)")
}

// TestVisualYankCommand_SingleLine tests yanking part of a single line
func TestVisualYankCommand_SingleLine(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 6}
	m.cursorRow = 0
	m.cursorCol = 10

	cmd := &VisualYankCommand{mode: ModeVisual}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "world", m.lastYankedText)
	assert.Equal(t, "hello world", m.content[0], "content unchanged")
	assert.Equal(t, 10, m.cursorCol, "cursor stays at current position")
}

// TestVisualYankCommand_MultiLine tests yanking multiple lines
func TestVisualYankCommand_MultiLine(t *testing.T) {
	m := newTestModelWithContent("hello", "world", "test")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 2}
	m.cursorRow = 2
	m.cursorCol = 2

	cmd := &VisualYankCommand{mode: ModeVisual}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "llo\nworld\ntes", m.lastYankedText)
	// Content should be unchanged
	assert.Len(t, m.content, 3)
	assert.Equal(t, "hello", m.content[0])
	assert.Equal(t, "world", m.content[1])
	assert.Equal(t, "test", m.content[2])
	// Cursor stays at current position
	assert.Equal(t, 2, m.cursorRow)
	assert.Equal(t, 2, m.cursorCol)
}

// TestVisualYankCommand_LinewiseMode tests line-wise yanking
func TestVisualYankCommand_LinewiseMode(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.mode = ModeVisualLine
	m.visualAnchor = Position{Row: 0, Col: 0}
	m.cursorRow = 1
	m.cursorCol = 3

	cmd := &VisualYankCommand{mode: ModeVisualLine}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "line1\nline2", m.lastYankedText)
	assert.True(t, cmd.wasLinewise)
	// Content unchanged
	assert.Len(t, m.content, 3)
	assert.Equal(t, "line1", m.content[0])
	assert.Equal(t, "line2", m.content[1])
	assert.Equal(t, "line3", m.content[2])
	// Cursor stays at current position
	assert.Equal(t, 1, m.cursorRow)
	assert.Equal(t, 3, m.cursorCol)
}

// TestVisualYankCommand_NotInVisualMode tests skipping when not in visual mode
func TestVisualYankCommand_NotInVisualMode(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeNormal

	cmd := &VisualYankCommand{mode: ModeVisual}
	result := cmd.Execute(m)

	assert.Equal(t, Skipped, result)
	assert.Equal(t, "", m.lastYankedText, "no text should be yanked")
}

// TestVisualYankCommand_ID tests command ID
func TestVisualYankCommand_ID(t *testing.T) {
	cmd := &VisualYankCommand{}
	assert.Equal(t, "visual.yank", cmd.ID())
}

// TestVisualYankCommand_SingleCharSelection tests yanking single character (anchor == cursor)
func TestVisualYankCommand_SingleCharSelection(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 2}
	m.cursorRow = 0
	m.cursorCol = 2 // Same as anchor - single char selected

	cmd := &VisualYankCommand{mode: ModeVisual}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "l", m.lastYankedText, "single char 'l' should be yanked")
	assert.Equal(t, "hello", m.content[0], "content unchanged")
	assert.Equal(t, 2, m.cursorCol)
}

// ============================================================================
// Integration Tests - Visual Yank Flow
// ============================================================================

// TestVisualMode_Yank_Flow tests the full v -> move -> y -> verify cursor stays flow
func TestVisualMode_Yank_Flow(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.mode = ModeNormal
	m.cursorRow = 0
	m.cursorCol = 0

	// Enter visual mode with 'v'
	cmdV := &EnterVisualModeCommand{}
	cmdV.Execute(m)
	assert.Equal(t, ModeVisual, m.mode)
	assert.Equal(t, Position{Row: 0, Col: 0}, m.visualAnchor)

	// Move right 4 times to select "hello"
	cmdL := &MoveRightCommand{}
	for i := 0; i < 4; i++ {
		cmdL.Execute(m)
	}
	assert.Equal(t, 4, m.cursorCol)

	// Yank with 'y'
	cmdY := &VisualYankCommand{mode: ModeVisual}
	cmdY.Execute(m)

	assert.Equal(t, ModeNormal, m.mode)
	assert.Equal(t, "hello world", m.content[0], "content should NOT change")
	assert.Equal(t, "hello", m.lastYankedText, "should yank 'hello'")
	assert.Equal(t, 4, m.cursorCol, "cursor should stay at current position")
}

// TestVisualLineMode_Yank_Flow tests V -> move -> y flow
func TestVisualLineMode_Yank_Flow(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.mode = ModeNormal
	m.cursorRow = 0
	m.cursorCol = 0

	// Enter visual line mode with 'V'
	cmdShiftV := &EnterVisualLineModeCommand{}
	cmdShiftV.Execute(m)
	assert.Equal(t, ModeVisualLine, m.mode)

	// Move down to select two lines
	cmdJ := &MoveDownCommand{}
	cmdJ.Execute(m)
	assert.Equal(t, 1, m.cursorRow)

	// Yank with 'y'
	cmdY := &VisualYankCommand{mode: ModeVisualLine}
	cmdY.Execute(m)

	assert.Equal(t, ModeNormal, m.mode)
	assert.Len(t, m.content, 3, "content should NOT change")
	assert.Equal(t, "line1\nline2", m.lastYankedText, "should yank both lines")
	assert.Equal(t, 1, m.cursorRow, "cursor should stay at current position")
	assert.Equal(t, 0, m.cursorCol)
}

// TestDefaultRegistry_HasVisualYankCommands verifies yank commands are registered for visual modes
func TestDefaultRegistry_HasVisualYankCommands(t *testing.T) {
	// ModeVisual should have 'y'
	cmdY, okY := DefaultRegistry.Get(ModeVisual, "y")
	assert.True(t, okY, "ModeVisual should have 'y' command")
	assert.NotNil(t, cmdY)
	assert.Equal(t, "visual.yank", cmdY.ID())

	// ModeVisualLine should have 'y'
	cmdYLine, okYLine := DefaultRegistry.Get(ModeVisualLine, "y")
	assert.True(t, okYLine, "ModeVisualLine should have 'y' command")
	assert.NotNil(t, cmdYLine)
	assert.Equal(t, "visual.yank", cmdYLine.ID())
}

// ============================================================================
// VisualChangeCommand Unit Tests
// ============================================================================

// TestVisualChangeCommand_Execute tests basic change execution - deletes and enters insert mode
func TestVisualChangeCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 0}
	m.cursorRow = 0
	m.cursorCol = 4

	cmd := &VisualChangeCommand{mode: ModeVisual}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, " world", m.content[0], "selected text should be deleted")
	assert.Equal(t, ModeInsert, m.mode, "should enter insert mode")
	assert.Equal(t, Position{}, m.visualAnchor, "visualAnchor should be cleared")
	assert.Equal(t, 0, m.cursorCol, "cursor should be at start of deleted region")
	assert.Equal(t, []string{"hello"}, cmd.deletedContent)
}

// TestVisualChangeCommand_Undo tests undoing change restores content and returns to Normal mode
func TestVisualChangeCommand_Undo(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 0}
	m.cursorRow = 0
	m.cursorCol = 4

	cmd := &VisualChangeCommand{mode: ModeVisual}
	cmd.Execute(m)
	assert.Equal(t, " world", m.content[0])
	assert.Equal(t, ModeInsert, m.mode)

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, "hello world", m.content[0], "content should be restored")
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 0, m.cursorCol, "cursor should be at start of restored region")
	assert.Equal(t, ModeNormal, m.mode, "should return to Normal mode, NOT Insert")
}

// TestVisualChangeCommand_Keys tests the trigger key
func TestVisualChangeCommand_Keys(t *testing.T) {
	cmd := &VisualChangeCommand{}
	keys := cmd.Keys()
	assert.Contains(t, keys, "c")
	assert.Len(t, keys, 1)
}

// TestVisualChangeCommand_Mode tests the mode getter
func TestVisualChangeCommand_Mode(t *testing.T) {
	cmd := &VisualChangeCommand{mode: ModeVisual}
	assert.Equal(t, ModeVisual, cmd.Mode())

	cmd2 := &VisualChangeCommand{mode: ModeVisualLine}
	assert.Equal(t, ModeVisualLine, cmd2.Mode())
}

// TestVisualChangeCommand_IsUndoable tests undoability
func TestVisualChangeCommand_IsUndoable(t *testing.T) {
	cmd := &VisualChangeCommand{}
	assert.True(t, cmd.IsUndoable(), "change should be undoable")
}

// TestVisualChangeCommand_IsModeChange tests that change triggers mode change
func TestVisualChangeCommand_IsModeChange(t *testing.T) {
	cmd := &VisualChangeCommand{}
	assert.True(t, cmd.IsModeChange(), "change enters insert mode so is a mode change")
}

// TestVisualChangeCommand_ChangesContent tests content change flag
func TestVisualChangeCommand_ChangesContent(t *testing.T) {
	cmd := &VisualChangeCommand{}
	assert.True(t, cmd.ChangesContent(), "change deletes content")
}

// TestVisualChangeCommand_EntersInsert tests that mode becomes ModeInsert
func TestVisualChangeCommand_EntersInsert(t *testing.T) {
	m := newTestModelWithContent("test")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 0}
	m.cursorRow = 0
	m.cursorCol = 3

	cmd := &VisualChangeCommand{mode: ModeVisual}
	cmd.Execute(m)

	assert.Equal(t, ModeInsert, m.mode, "must enter Insert mode after change")
}

// TestVisualChangeCommand_CursorPosition tests cursor is at deletion start
func TestVisualChangeCommand_CursorPosition(t *testing.T) {
	// Test forward selection
	m := newTestModelWithContent("hello world")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 3}
	m.cursorRow = 0
	m.cursorCol = 7

	cmd := &VisualChangeCommand{mode: ModeVisual}
	cmd.Execute(m)

	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 3, m.cursorCol, "cursor should be at start of deleted region (col 3)")

	// Test backward selection (cursor before anchor)
	m2 := newTestModelWithContent("hello world")
	m2.mode = ModeVisual
	m2.visualAnchor = Position{Row: 0, Col: 8}
	m2.cursorRow = 0
	m2.cursorCol = 2

	cmd2 := &VisualChangeCommand{mode: ModeVisual}
	cmd2.Execute(m2)

	assert.Equal(t, 0, m2.cursorRow)
	assert.Equal(t, 2, m2.cursorCol, "cursor should be at normalized start (col 2)")
}

// TestVisualChangeCommand_SingleLine tests changing partial line
func TestVisualChangeCommand_SingleLine(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 6}
	m.cursorRow = 0
	m.cursorCol = 10

	cmd := &VisualChangeCommand{mode: ModeVisual}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "hello ", m.content[0])
	assert.Equal(t, ModeInsert, m.mode)
	assert.Equal(t, []string{"world"}, cmd.deletedContent)
	// In insert mode, cursor should be at the start of where selection was (col 6)
	// This allows typing to insert at the correct position
	assert.Equal(t, 6, m.cursorCol, "cursor at start of deleted selection for insert")
}

// TestVisualChangeCommand_MultiLine tests changing multiple lines
func TestVisualChangeCommand_MultiLine(t *testing.T) {
	m := newTestModelWithContent("hello", "world", "test")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 2}
	m.cursorRow = 2
	m.cursorCol = 2

	cmd := &VisualChangeCommand{mode: ModeVisual}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Len(t, m.content, 1)
	assert.Equal(t, "het", m.content[0], "should join first prefix 'he' with last suffix 't'")
	assert.Equal(t, ModeInsert, m.mode)
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 2, m.cursorCol, "cursor should be at start of deleted region")
}

// TestVisualChangeCommand_NotInVisualMode tests skipping when not in visual mode
func TestVisualChangeCommand_NotInVisualMode(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeNormal

	cmd := &VisualChangeCommand{mode: ModeVisual}
	result := cmd.Execute(m)

	assert.Equal(t, Skipped, result)
	assert.Equal(t, "hello", m.content[0], "content should not change")
	assert.Equal(t, ModeNormal, m.mode, "mode should not change")
}

// TestVisualChangeCommand_LinewiseMode tests line-wise change
func TestVisualChangeCommand_LinewiseMode(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.mode = ModeVisualLine
	m.visualAnchor = Position{Row: 0, Col: 0}
	m.cursorRow = 1
	m.cursorCol = 3

	cmd := &VisualChangeCommand{mode: ModeVisualLine}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Len(t, m.content, 1)
	assert.Equal(t, "line3", m.content[0])
	assert.Equal(t, ModeInsert, m.mode)
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 0, m.cursorCol)
	assert.True(t, cmd.wasLinewise)
	assert.Equal(t, []string{"line1", "line2"}, cmd.deletedContent)
}

// TestVisualChangeCommand_ID tests command ID
func TestVisualChangeCommand_ID(t *testing.T) {
	cmd := &VisualChangeCommand{}
	assert.Equal(t, "visual.change", cmd.ID())
}

// ============================================================================
// VisualChangeCommand Undo Tests
// ============================================================================

// TestVisualChangeCommand_Undo_MultiLine tests undoing multi-line change
func TestVisualChangeCommand_Undo_MultiLine(t *testing.T) {
	m := newTestModelWithContent("hello", "world", "test")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 2}
	m.cursorRow = 2
	m.cursorCol = 2

	cmd := &VisualChangeCommand{mode: ModeVisual}
	cmd.Execute(m)
	assert.Equal(t, "het", m.content[0])
	assert.Len(t, m.content, 1)
	assert.Equal(t, ModeInsert, m.mode)

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Len(t, m.content, 3)
	assert.Equal(t, "hello", m.content[0])
	assert.Equal(t, "world", m.content[1])
	assert.Equal(t, "test", m.content[2])
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 2, m.cursorCol)
	assert.Equal(t, ModeNormal, m.mode, "undo should return to Normal mode")
}

// TestVisualChangeCommand_Undo_LinewiseMode tests undoing line-wise change
func TestVisualChangeCommand_Undo_LinewiseMode(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.mode = ModeVisualLine
	m.visualAnchor = Position{Row: 0, Col: 0}
	m.cursorRow = 1
	m.cursorCol = 3

	cmd := &VisualChangeCommand{mode: ModeVisualLine}
	cmd.Execute(m)
	assert.Equal(t, "line3", m.content[0])
	assert.Len(t, m.content, 1)
	assert.Equal(t, ModeInsert, m.mode)

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Len(t, m.content, 3)
	assert.Equal(t, "line1", m.content[0])
	assert.Equal(t, "line2", m.content[1])
	assert.Equal(t, "line3", m.content[2])
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 0, m.cursorCol)
	assert.Equal(t, ModeNormal, m.mode)
}

// TestVisualChangeCommand_Undo_AllContent tests undoing change of all content
func TestVisualChangeCommand_Undo_AllContent(t *testing.T) {
	m := newTestModelWithContent("hello", "world")
	m.mode = ModeVisualLine
	m.visualAnchor = Position{Row: 0, Col: 0}
	m.cursorRow = 1
	m.cursorCol = 4

	cmd := &VisualChangeCommand{mode: ModeVisualLine}
	cmd.Execute(m)
	assert.Equal(t, "", m.content[0])
	assert.Len(t, m.content, 1)
	assert.Equal(t, ModeInsert, m.mode)

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Len(t, m.content, 2)
	assert.Equal(t, "hello", m.content[0])
	assert.Equal(t, "world", m.content[1])
	assert.Equal(t, ModeNormal, m.mode)
}

// ============================================================================
// Integration Tests - Visual Change Flow
// ============================================================================

// TestVisualMode_Change_Flow tests the full v -> move -> c -> type -> verify flow
func TestVisualMode_Change_Flow(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.mode = ModeNormal
	m.cursorRow = 0
	m.cursorCol = 0

	// Enter visual mode with 'v'
	cmdV := &EnterVisualModeCommand{}
	cmdV.Execute(m)
	assert.Equal(t, ModeVisual, m.mode)
	assert.Equal(t, Position{Row: 0, Col: 0}, m.visualAnchor)

	// Move right 4 times to select "hello"
	cmdL := &MoveRightCommand{}
	for i := 0; i < 4; i++ {
		cmdL.Execute(m)
	}
	assert.Equal(t, 4, m.cursorCol)

	// Change with 'c' (use executeCommand to properly clone)
	cmd := &VisualChangeCommand{mode: ModeVisual}
	m.executeCommand(cmd)

	assert.Equal(t, ModeInsert, m.mode, "should enter insert mode")
	assert.Equal(t, " world", m.content[0], "selected text deleted")
	assert.Equal(t, 0, m.cursorCol, "cursor at deletion start, ready for typing")
}

// TestVisualMode_ChangeUndo tests c -> ESC -> u -> verify restored
func TestVisualMode_ChangeUndo(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 2}
	m.cursorRow = 0
	m.cursorCol = 7

	// Execute change through model (so it goes into history)
	cmd := &VisualChangeCommand{mode: ModeVisual}
	m.executeCommand(cmd)

	assert.Equal(t, "herld", m.content[0])
	assert.Equal(t, ModeInsert, m.mode)
	assert.Equal(t, 2, m.cursorCol)

	// Simulate ESC to exit insert mode (would happen via EscapeCommand in real use)
	// For testing, just call undo via history
	err := m.history.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, "hello world", m.content[0], "content should be restored")
	assert.Equal(t, 2, m.cursorCol, "cursor should be at start of restored region")
	assert.Equal(t, ModeNormal, m.mode, "undo returns to Normal mode")

	// Redo
	err = m.history.Redo(m)
	assert.NoError(t, err)
	assert.Equal(t, "herld", m.content[0])
	assert.Equal(t, ModeInsert, m.mode, "redo should re-enter insert mode")
}

// TestVisualLineMode_Change_Flow tests V -> move -> c flow
func TestVisualLineMode_Change_Flow(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.mode = ModeNormal
	m.cursorRow = 0
	m.cursorCol = 0

	// Enter visual line mode with 'V'
	cmdShiftV := &EnterVisualLineModeCommand{}
	cmdShiftV.Execute(m)
	assert.Equal(t, ModeVisualLine, m.mode)

	// Move down to select two lines
	cmdJ := &MoveDownCommand{}
	cmdJ.Execute(m)
	assert.Equal(t, 1, m.cursorRow)

	// Change with 'c' (use executeCommand to properly clone)
	cmd := &VisualChangeCommand{mode: ModeVisualLine}
	m.executeCommand(cmd)

	assert.Equal(t, ModeInsert, m.mode)
	assert.Len(t, m.content, 1)
	assert.Equal(t, "line3", m.content[0])
}

// TestVisualMode_Change_SingleChar tests changing single character selection
func TestVisualMode_Change_SingleChar(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 2}
	m.cursorRow = 0
	m.cursorCol = 2 // Same as anchor - single char selected

	cmd := &VisualChangeCommand{mode: ModeVisual}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "helo", m.content[0], "single char 'l' should be deleted")
	assert.Equal(t, ModeInsert, m.mode)
	assert.Equal(t, 2, m.cursorCol)
}

// TestDefaultRegistry_HasVisualChangeCommands verifies change commands are registered for visual modes
func TestDefaultRegistry_HasVisualChangeCommands(t *testing.T) {
	// ModeVisual should have 'c'
	cmdC, okC := DefaultRegistry.Get(ModeVisual, "c")
	assert.True(t, okC, "ModeVisual should have 'c' command")
	assert.NotNil(t, cmdC)
	assert.Equal(t, "visual.change", cmdC.ID())

	// ModeVisualLine should have 'c'
	cmdCLine, okCLine := DefaultRegistry.Get(ModeVisualLine, "c")
	assert.True(t, okCLine, "ModeVisualLine should have 'c' command")
	assert.NotNil(t, cmdCLine)
	assert.Equal(t, "visual.change", cmdCLine.ID())
}

// ============================================================================
// VisualSwapAnchorCommand Tests ('o' in visual mode)
// ============================================================================

// TestVisualSwapAnchorCommand_Execute_SwapsPositions tests basic swap
func TestVisualSwapAnchorCommand_Execute_SwapsPositions(t *testing.T) {
	m := newTestModelWithContent("hello", "world", "test")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 1}
	m.cursorRow = 2
	m.cursorCol = 3

	cmd := &VisualSwapAnchorCommand{mode: ModeVisual}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	// Cursor should now be at anchor position
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 1, m.cursorCol)
	// Anchor should now be at old cursor position
	assert.Equal(t, 2, m.visualAnchor.Row)
	assert.Equal(t, 3, m.visualAnchor.Col)
	// Should still be in visual mode
	assert.Equal(t, ModeVisual, m.mode)
}

// TestVisualSwapAnchorCommand_Execute_DoubleSwapRestoresOriginal tests swap is reversible
func TestVisualSwapAnchorCommand_Execute_DoubleSwapRestoresOriginal(t *testing.T) {
	m := newTestModelWithContent("hello", "world", "test")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 2}
	m.cursorRow = 1
	m.cursorCol = 4

	cmd := &VisualSwapAnchorCommand{mode: ModeVisual}

	// First swap
	cmd.Execute(m)
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 2, m.cursorCol)
	assert.Equal(t, 1, m.visualAnchor.Row)
	assert.Equal(t, 4, m.visualAnchor.Col)

	// Second swap - should restore original
	cmd.Execute(m)
	assert.Equal(t, 1, m.cursorRow)
	assert.Equal(t, 4, m.cursorCol)
	assert.Equal(t, 0, m.visualAnchor.Row)
	assert.Equal(t, 2, m.visualAnchor.Col)
}

// TestVisualSwapAnchorCommand_Execute_VisualLineMode tests swap works in line mode
func TestVisualSwapAnchorCommand_Execute_VisualLineMode(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3", "line4")
	m.mode = ModeVisualLine
	m.visualAnchor = Position{Row: 1, Col: 0}
	m.cursorRow = 3
	m.cursorCol = 2

	cmd := &VisualSwapAnchorCommand{mode: ModeVisualLine}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, 1, m.cursorRow)
	assert.Equal(t, 0, m.cursorCol)
	assert.Equal(t, 3, m.visualAnchor.Row)
	assert.Equal(t, 2, m.visualAnchor.Col)
	assert.Equal(t, ModeVisualLine, m.mode)
}

// TestVisualSwapAnchorCommand_Execute_SkipsInNormalMode tests command skips in normal mode
func TestVisualSwapAnchorCommand_Execute_SkipsInNormalMode(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeNormal
	m.visualAnchor = Position{Row: 0, Col: 1}
	m.cursorRow = 0
	m.cursorCol = 3

	cmd := &VisualSwapAnchorCommand{mode: ModeVisual}
	result := cmd.Execute(m)

	assert.Equal(t, Skipped, result)
	// Positions should be unchanged
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 3, m.cursorCol)
}

// TestVisualSwapAnchorCommand_Metadata tests command metadata
func TestVisualSwapAnchorCommand_Metadata(t *testing.T) {
	cmd := &VisualSwapAnchorCommand{mode: ModeVisual}
	assert.Equal(t, []string{"o"}, cmd.Keys())
	assert.Equal(t, ModeVisual, cmd.Mode())
	assert.Equal(t, "visual.swap_anchor", cmd.ID())
	assert.False(t, cmd.IsUndoable())
	assert.False(t, cmd.ChangesContent())
	assert.False(t, cmd.IsModeChange())
}

// TestDefaultRegistry_HasVisualSwapAnchorCommands verifies 'o' is registered for visual modes
func TestDefaultRegistry_HasVisualSwapAnchorCommands(t *testing.T) {
	// ModeVisual should have 'o'
	cmdV, okV := DefaultRegistry.Get(ModeVisual, "o")
	assert.True(t, okV, "ModeVisual should have 'o' command")
	assert.NotNil(t, cmdV)
	assert.Equal(t, "visual.swap_anchor", cmdV.ID())

	// ModeVisualLine should have 'o'
	cmdVL, okVL := DefaultRegistry.Get(ModeVisualLine, "o")
	assert.True(t, okVL, "ModeVisualLine should have 'o' command")
	assert.NotNil(t, cmdVL)
	assert.Equal(t, "visual.swap_anchor", cmdVL.ID())
}

// ============================================================================
// Yank Register Population Tests (lastYankWasLinewise)
// These tests verify that visual commands properly set lastYankWasLinewise
// ============================================================================

// TestVisualDeleteCommand_PopulatesYankRegister_CharacterWise verifies character-wise delete
func TestVisualDeleteCommand_PopulatesYankRegister_CharacterWise(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 0}
	m.cursorRow = 0
	m.cursorCol = 4

	cmd := &VisualDeleteCommand{mode: ModeVisual}
	cmd.Execute(m)

	assert.Equal(t, "hello", m.lastYankedText)
	assert.False(t, m.lastYankWasLinewise, "character-wise visual delete should set lastYankWasLinewise = false")
}

// TestVisualDeleteCommand_PopulatesYankRegister_LinewiseMode verifies line-wise delete
func TestVisualDeleteCommand_PopulatesYankRegister_LinewiseMode(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.mode = ModeVisualLine
	m.visualAnchor = Position{Row: 0, Col: 0}
	m.cursorRow = 1
	m.cursorCol = 3

	cmd := &VisualDeleteCommand{mode: ModeVisualLine}
	cmd.Execute(m)

	assert.Equal(t, "line1\nline2", m.lastYankedText)
	assert.True(t, m.lastYankWasLinewise, "line-wise visual delete should set lastYankWasLinewise = true")
}

// TestVisualYankCommand_PopulatesYankRegister_CharacterWise verifies character-wise yank
func TestVisualYankCommand_PopulatesYankRegister_CharacterWise(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 0}
	m.cursorRow = 0
	m.cursorCol = 4

	cmd := &VisualYankCommand{mode: ModeVisual}
	cmd.Execute(m)

	assert.Equal(t, "hello", m.lastYankedText)
	assert.False(t, m.lastYankWasLinewise, "character-wise visual yank should set lastYankWasLinewise = false")
}

// TestVisualYankCommand_PopulatesYankRegister_LinewiseMode verifies line-wise yank sets lastYankWasLinewise = true
func TestVisualYankCommand_PopulatesYankRegister_LinewiseMode(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.mode = ModeVisualLine
	m.visualAnchor = Position{Row: 0, Col: 0}
	m.cursorRow = 1
	m.cursorCol = 3

	cmd := &VisualYankCommand{mode: ModeVisualLine}
	cmd.Execute(m)

	assert.Equal(t, "line1\nline2", m.lastYankedText)
	assert.True(t, m.lastYankWasLinewise, "line-wise visual yank should set lastYankWasLinewise = true")
}

// TestVisualYankCommand_MultiLineCharacterWise verifies multi-line character-wise yank is NOT linewise
func TestVisualYankCommand_MultiLineCharacterWise(t *testing.T) {
	m := newTestModelWithContent("hello", "world", "test")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 2}
	m.cursorRow = 2
	m.cursorCol = 2

	cmd := &VisualYankCommand{mode: ModeVisual}
	cmd.Execute(m)

	assert.Equal(t, "llo\nworld\ntes", m.lastYankedText)
	assert.False(t, m.lastYankWasLinewise, "character-wise multi-line yank should set lastYankWasLinewise = false")
}
