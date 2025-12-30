package vimtextarea

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Mode Entry Command Tests
// ============================================================================

// TestEnterInsertModeCommand_Execute verifies 'i' enters insert mode
func TestEnterInsertModeCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeNormal

	cmd := &EnterInsertModeCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, ModeInsert, m.mode)
}

// TestEnterInsertModeCommand_Metadata verifies command metadata
func TestEnterInsertModeCommand_Metadata(t *testing.T) {
	cmd := &EnterInsertModeCommand{}
	require.Equal(t, []string{"i"}, cmd.Keys())
	require.Equal(t, ModeNormal, cmd.Mode())
	require.Equal(t, "mode.insert", cmd.ID())
	require.False(t, cmd.IsUndoable())
	require.False(t, cmd.ChangesContent())
	require.True(t, cmd.IsModeChange())
}

// TestEnterInsertModeCommand_Undo verifies Undo is a no-op (from ModeEntryBase)
func TestEnterInsertModeCommand_Undo(t *testing.T) {
	cmd := &EnterInsertModeCommand{}
	err := cmd.Undo(nil)
	require.NoError(t, err)
}

// TestEnterInsertModeAfterCommand_Execute verifies 'a' enters insert mode after cursor
func TestEnterInsertModeAfterCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeNormal
	m.cursorCol = 2

	cmd := &EnterInsertModeAfterCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, ModeInsert, m.mode)
	require.Equal(t, 3, m.cursorCol) // moved right
}

// TestEnterInsertModeAfterCommand_ExecuteAtEnd verifies 'a' at end of line
func TestEnterInsertModeAfterCommand_ExecuteAtEnd(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeNormal
	m.cursorCol = 5 // at end

	cmd := &EnterInsertModeAfterCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, ModeInsert, m.mode)
	require.Equal(t, 5, m.cursorCol) // stays at end
}

// TestEnterInsertModeAfterCommand_Metadata verifies command metadata
func TestEnterInsertModeAfterCommand_Metadata(t *testing.T) {
	cmd := &EnterInsertModeAfterCommand{}
	require.Equal(t, []string{"a"}, cmd.Keys())
	require.Equal(t, ModeNormal, cmd.Mode())
	require.Equal(t, "mode.insert_after", cmd.ID())
	require.False(t, cmd.IsUndoable())
	require.False(t, cmd.ChangesContent())
	require.True(t, cmd.IsModeChange())
}

// TestEnterInsertModeAtEndCommand_Execute verifies 'A' enters insert mode at line end
func TestEnterInsertModeAtEndCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeNormal
	m.cursorCol = 1

	cmd := &EnterInsertModeAtEndCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, ModeInsert, m.mode)
	require.Equal(t, 5, m.cursorCol) // moved to end
}

// TestEnterInsertModeAtEndCommand_Metadata verifies command metadata
func TestEnterInsertModeAtEndCommand_Metadata(t *testing.T) {
	cmd := &EnterInsertModeAtEndCommand{}
	require.Equal(t, []string{"A"}, cmd.Keys())
	require.Equal(t, ModeNormal, cmd.Mode())
	require.Equal(t, "mode.insert_at_end", cmd.ID())
	require.False(t, cmd.IsUndoable())
	require.False(t, cmd.ChangesContent())
	require.True(t, cmd.IsModeChange())
}

// TestEnterInsertModeAtStartCommand_Execute verifies 'I' enters at first non-blank
func TestEnterInsertModeAtStartCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("  hello")
	m.mode = ModeNormal
	m.cursorCol = 5

	cmd := &EnterInsertModeAtStartCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, ModeInsert, m.mode)
	require.Equal(t, 2, m.cursorCol) // at 'h'
}

// TestEnterInsertModeAtStartCommand_Metadata verifies command metadata
func TestEnterInsertModeAtStartCommand_Metadata(t *testing.T) {
	cmd := &EnterInsertModeAtStartCommand{}
	require.Equal(t, []string{"I"}, cmd.Keys())
	require.Equal(t, ModeNormal, cmd.Mode())
	require.Equal(t, "mode.insert_at_start", cmd.ID())
	require.False(t, cmd.IsUndoable())
	require.False(t, cmd.ChangesContent())
	require.True(t, cmd.IsModeChange())
}

// TestInsertLineBelowCommand_Execute verifies 'o' inserts line below
func TestInsertLineBelowCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("line1", "line2")
	m.cursorRow = 0
	m.mode = ModeNormal

	cmd := &InsertLineBelowCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Len(t, m.content, 3)
	require.Equal(t, "line1", m.content[0])
	require.Equal(t, "", m.content[1])
	require.Equal(t, "line2", m.content[2])
	require.Equal(t, 1, m.cursorRow)
	require.Equal(t, ModeInsert, m.mode)
}

// TestInsertLineBelowCommand_Undo verifies undoing 'o'
func TestInsertLineBelowCommand_Undo(t *testing.T) {
	m := newTestModelWithContent("line1", "line2")
	m.cursorRow = 0

	cmd := &InsertLineBelowCommand{}
	_ = cmd.Execute(m)
	require.Len(t, m.content, 3)

	err := cmd.Undo(m)
	require.NoError(t, err)
	require.Len(t, m.content, 2)
	require.Equal(t, "line1", m.content[0])
	require.Equal(t, "line2", m.content[1])
	require.Equal(t, 0, m.cursorRow)
}

// TestInsertLineBelowCommand_UndoInvalidRow verifies undo with invalid row
func TestInsertLineBelowCommand_UndoInvalidRow(t *testing.T) {
	m := newTestModelWithContent("line1")
	cmd := &InsertLineBelowCommand{insertedRow: 99} // out of range

	err := cmd.Undo(m)
	require.NoError(t, err)
	require.Len(t, m.content, 1) // unchanged
}

// TestInsertLineBelowCommand_Metadata verifies command metadata
func TestInsertLineBelowCommand_Metadata(t *testing.T) {
	cmd := &InsertLineBelowCommand{}
	require.Equal(t, []string{"o"}, cmd.Keys())
	require.Equal(t, ModeNormal, cmd.Mode())
	require.Equal(t, "mode.insert_line_below", cmd.ID())
	require.True(t, cmd.IsUndoable())
	require.True(t, cmd.ChangesContent())
	require.True(t, cmd.IsModeChange())
}

// TestInsertLineAboveCommand_Execute verifies 'O' inserts line above
func TestInsertLineAboveCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("line1", "line2")
	m.cursorRow = 1
	m.mode = ModeNormal

	cmd := &InsertLineAboveCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Len(t, m.content, 3)
	require.Equal(t, "line1", m.content[0])
	require.Equal(t, "", m.content[1])
	require.Equal(t, "line2", m.content[2])
	require.Equal(t, 1, m.cursorRow) // stays on new line
	require.Equal(t, ModeInsert, m.mode)
}

// TestInsertLineAboveCommand_Undo verifies undoing 'O'
func TestInsertLineAboveCommand_Undo(t *testing.T) {
	m := newTestModelWithContent("line1", "line2")
	m.cursorRow = 1

	cmd := &InsertLineAboveCommand{}
	_ = cmd.Execute(m)
	require.Len(t, m.content, 3)

	err := cmd.Undo(m)
	require.NoError(t, err)
	require.Len(t, m.content, 2)
	require.Equal(t, "line1", m.content[0])
	require.Equal(t, "line2", m.content[1])
	require.Equal(t, 1, m.cursorRow)
}

// TestInsertLineAboveCommand_UndoInvalidRow verifies undo with invalid row
func TestInsertLineAboveCommand_UndoInvalidRow(t *testing.T) {
	m := newTestModelWithContent("line1")
	cmd := &InsertLineAboveCommand{insertedRow: 99} // out of range

	err := cmd.Undo(m)
	require.NoError(t, err)
	require.Len(t, m.content, 1) // unchanged
}

// TestInsertLineAboveCommand_UndoAtEnd verifies undo when cursor would be past end
func TestInsertLineAboveCommand_UndoAtEnd(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	// Simulate an insert at the end
	cmd := &InsertLineAboveCommand{insertedRow: 2}
	// Manually remove the inserted line to simulate state after insert
	// The undo should handle the edge case of cursor row > content length

	// Actually test the guard in Undo
	err := cmd.Undo(m)
	require.NoError(t, err)
}

// TestInsertLineAboveCommand_Metadata verifies command metadata
func TestInsertLineAboveCommand_Metadata(t *testing.T) {
	cmd := &InsertLineAboveCommand{}
	require.Equal(t, []string{"O"}, cmd.Keys())
	require.Equal(t, ModeNormal, cmd.Mode())
	require.Equal(t, "mode.insert_line_above", cmd.ID())
	require.True(t, cmd.IsUndoable())
	require.True(t, cmd.ChangesContent())
	require.True(t, cmd.IsModeChange())
}

// TestEscapeCommand_Execute verifies escape exits insert mode
func TestEscapeCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeInsert
	m.cursorCol = 3
	m.config.VimEnabled = true

	cmd := &EscapeCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, ModeNormal, m.mode)
	require.Equal(t, 2, m.cursorCol) // moved back
}

// TestEscapeCommand_ExecuteVimDisabled verifies pass through when vim disabled
func TestEscapeCommand_ExecuteVimDisabled(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeInsert
	m.config.VimEnabled = false

	cmd := &EscapeCommand{}
	result := cmd.Execute(m)

	require.Equal(t, PassThrough, result)
	require.Equal(t, ModeInsert, m.mode) // unchanged
}

// TestEscapeCommand_ExecuteAtStart verifies cursor doesn't go negative
func TestEscapeCommand_ExecuteAtStart(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeInsert
	m.cursorCol = 0
	m.config.VimEnabled = true

	cmd := &EscapeCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, ModeNormal, m.mode)
	require.Equal(t, 0, m.cursorCol) // stays at 0
}

// TestEscapeCommand_Metadata verifies command metadata
func TestEscapeCommand_Metadata(t *testing.T) {
	cmd := &EscapeCommand{}
	require.Equal(t, []string{"<escape>", "<ctrl+c>"}, cmd.Keys())
	require.Equal(t, ModeInsert, cmd.Mode())
	require.Equal(t, "mode.escape", cmd.ID())
	require.False(t, cmd.IsUndoable())
	require.False(t, cmd.ChangesContent())
	require.True(t, cmd.IsModeChange())
}

// TestEscapeCommand_CtrlC_Integration verifies Ctrl+C key event exits insert mode
func TestEscapeCommand_CtrlC_Integration(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.config.VimEnabled = true
	m.mode = ModeInsert
	m.cursorCol = 3
	m.focused = true

	// Send actual Ctrl+C key message through Update
	ctrlCMsg := tea.KeyMsg{Type: tea.KeyCtrlC}
	updated, _ := m.Update(ctrlCMsg)

	require.Equal(t, ModeNormal, updated.mode, "Ctrl+C should exit insert mode")
	require.Equal(t, 2, updated.cursorCol, "cursor moves back one position")
}

// TestNormalModeEscapeCommand_Execute verifies escape in normal mode
func TestNormalModeEscapeCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeNormal
	m.pendingBuilder.SetOperator('d')

	cmd := &NormalModeEscapeCommand{}
	result := cmd.Execute(m)

	require.Equal(t, PassThrough, result)
	require.True(t, m.pendingBuilder.IsEmpty()) // cleared
}

// TestNormalModeEscapeCommand_Metadata verifies command metadata
func TestNormalModeEscapeCommand_Metadata(t *testing.T) {
	cmd := &NormalModeEscapeCommand{}
	require.Equal(t, []string{"<escape>"}, cmd.Keys())
	require.Equal(t, ModeNormal, cmd.Mode())
	require.Equal(t, "mode.escape_normal", cmd.ID())
	require.False(t, cmd.IsUndoable())
	require.False(t, cmd.ChangesContent())
	require.False(t, cmd.IsModeChange())
}

// ============================================================================
// Visual Mode Entry Command Tests
// ============================================================================

// TestEnterVisualModeCommand_Execute verifies 'v' enters visual mode with anchor
func TestEnterVisualModeCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeNormal
	m.cursorRow = 0
	m.cursorCol = 2

	cmd := &EnterVisualModeCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, ModeVisual, m.mode)
	require.Equal(t, Position{Row: 0, Col: 2}, m.visualAnchor)
}

// TestEnterVisualModeCommand_Keys verifies command returns correct keys
func TestEnterVisualModeCommand_Keys(t *testing.T) {
	cmd := &EnterVisualModeCommand{}
	require.Equal(t, []string{"v"}, cmd.Keys())
}

// TestEnterVisualModeCommand_Mode verifies command operates in ModeNormal
func TestEnterVisualModeCommand_Mode(t *testing.T) {
	cmd := &EnterVisualModeCommand{}
	require.Equal(t, ModeNormal, cmd.Mode())
}

// TestEnterVisualModeCommand_IsModeChange verifies command triggers mode change
func TestEnterVisualModeCommand_IsModeChange(t *testing.T) {
	cmd := &EnterVisualModeCommand{}
	require.True(t, cmd.IsModeChange())
}

// TestEnterVisualModeCommand_Metadata verifies command metadata
func TestEnterVisualModeCommand_Metadata(t *testing.T) {
	cmd := &EnterVisualModeCommand{}
	require.Equal(t, []string{"v"}, cmd.Keys())
	require.Equal(t, ModeNormal, cmd.Mode())
	require.Equal(t, "mode.visual", cmd.ID())
	require.False(t, cmd.IsUndoable())
	require.False(t, cmd.ChangesContent())
	require.True(t, cmd.IsModeChange())
}

// TestEnterVisualModeCommand_Undo verifies Undo is a no-op (from ModeEntryBase)
func TestEnterVisualModeCommand_Undo(t *testing.T) {
	cmd := &EnterVisualModeCommand{}
	err := cmd.Undo(nil)
	require.NoError(t, err)
}

// TestEnterVisualLineModeCommand_Execute verifies 'V' enters visual line mode with anchor.Col=0
func TestEnterVisualLineModeCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello", "world")
	m.mode = ModeNormal
	m.cursorRow = 1
	m.cursorCol = 3

	cmd := &EnterVisualLineModeCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, ModeVisualLine, m.mode)
	// Line-wise mode sets anchor.Col to 0
	require.Equal(t, Position{Row: 1, Col: 0}, m.visualAnchor)
}

// TestEnterVisualLineModeCommand_Keys verifies command returns correct keys
func TestEnterVisualLineModeCommand_Keys(t *testing.T) {
	cmd := &EnterVisualLineModeCommand{}
	require.Equal(t, []string{"V"}, cmd.Keys())
}

// TestEnterVisualLineModeCommand_Mode verifies command operates in ModeNormal
func TestEnterVisualLineModeCommand_Mode(t *testing.T) {
	cmd := &EnterVisualLineModeCommand{}
	require.Equal(t, ModeNormal, cmd.Mode())
}

// TestEnterVisualLineModeCommand_Metadata verifies command metadata
func TestEnterVisualLineModeCommand_Metadata(t *testing.T) {
	cmd := &EnterVisualLineModeCommand{}
	require.Equal(t, []string{"V"}, cmd.Keys())
	require.Equal(t, ModeNormal, cmd.Mode())
	require.Equal(t, "mode.visual_line", cmd.ID())
	require.False(t, cmd.IsUndoable())
	require.False(t, cmd.ChangesContent())
	require.True(t, cmd.IsModeChange())
}

// TestEnterVisualLineModeCommand_Undo verifies Undo is a no-op (from ModeEntryBase)
func TestEnterVisualLineModeCommand_Undo(t *testing.T) {
	cmd := &EnterVisualLineModeCommand{}
	err := cmd.Undo(nil)
	require.NoError(t, err)
}

// ============================================================================
// Visual Mode Escape Command Tests
// ============================================================================

// TestVisualModeEscapeCommand_Execute verifies ESC exits visual mode and clears anchor
func TestVisualModeEscapeCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello", "world")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 2}
	m.cursorRow = 1
	m.cursorCol = 3

	cmd := &VisualModeEscapeCommand{mode: ModeVisual}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, ModeNormal, m.mode)
	require.Equal(t, Position{}, m.visualAnchor) // Anchor should be cleared
}

// TestVisualModeEscapeCommand_Execute_FromVisualLine verifies ESC exits visual line mode
func TestVisualModeEscapeCommand_Execute_FromVisualLine(t *testing.T) {
	m := newTestModelWithContent("hello", "world")
	m.mode = ModeVisualLine
	m.visualAnchor = Position{Row: 0, Col: 0}
	m.cursorRow = 1
	m.cursorCol = 3

	cmd := &VisualModeEscapeCommand{mode: ModeVisualLine}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, ModeNormal, m.mode)
	require.Equal(t, Position{}, m.visualAnchor)
}

// TestVisualModeEscapeCommand_Keys verifies command returns correct keys
func TestVisualModeEscapeCommand_Keys(t *testing.T) {
	cmd := &VisualModeEscapeCommand{}
	require.Equal(t, []string{"<escape>", "<ctrl+c>"}, cmd.Keys())
}

// TestVisualModeEscapeCommand_Mode verifies command returns configured mode
func TestVisualModeEscapeCommand_Mode(t *testing.T) {
	cmdVisual := &VisualModeEscapeCommand{mode: ModeVisual}
	require.Equal(t, ModeVisual, cmdVisual.Mode())

	cmdVisualLine := &VisualModeEscapeCommand{mode: ModeVisualLine}
	require.Equal(t, ModeVisualLine, cmdVisualLine.Mode())
}

// TestVisualModeEscapeCommand_Metadata verifies command metadata
func TestVisualModeEscapeCommand_Metadata(t *testing.T) {
	cmd := &VisualModeEscapeCommand{mode: ModeVisual}
	require.Equal(t, []string{"<escape>", "<ctrl+c>"}, cmd.Keys())
	require.Equal(t, "mode.visual_escape", cmd.ID())
	require.False(t, cmd.IsUndoable())
	require.False(t, cmd.ChangesContent())
	require.True(t, cmd.IsModeChange())
}

// TestVisualModeEscapeCommand_CtrlC_Integration verifies Ctrl+C key event exits visual mode
func TestVisualModeEscapeCommand_CtrlC_Integration(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.config.VimEnabled = true
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 2}
	m.cursorRow = 0
	m.cursorCol = 6
	m.focused = true

	// Send actual Ctrl+C key message through Update
	ctrlCMsg := tea.KeyMsg{Type: tea.KeyCtrlC}
	updated, _ := m.Update(ctrlCMsg)

	require.Equal(t, ModeNormal, updated.mode, "Ctrl+C should exit visual mode")
	require.Equal(t, Position{}, updated.visualAnchor, "anchor should be cleared")
}

// TestVisualLineModeEscapeCommand_CtrlC_Integration verifies Ctrl+C key event exits visual line mode
func TestVisualLineModeEscapeCommand_CtrlC_Integration(t *testing.T) {
	m := newTestModelWithContent("hello", "world")
	m.config.VimEnabled = true
	m.mode = ModeVisualLine
	m.visualAnchor = Position{Row: 0, Col: 0}
	m.cursorRow = 1
	m.cursorCol = 2
	m.focused = true

	// Send actual Ctrl+C key message through Update
	ctrlCMsg := tea.KeyMsg{Type: tea.KeyCtrlC}
	updated, _ := m.Update(ctrlCMsg)

	require.Equal(t, ModeNormal, updated.mode, "Ctrl+C should exit visual line mode")
	require.Equal(t, Position{}, updated.visualAnchor, "anchor should be cleared")
}

// ============================================================================
// Visual Mode Toggle Command Tests
// ============================================================================

// TestVisualToggle_VInVisual verifies 'v' in ModeVisual returns to Normal
func TestVisualToggle_VInVisual(t *testing.T) {
	m := newTestModelWithContent("hello", "world")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 2}

	cmd := &VisualModeToggleVCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, ModeNormal, m.mode)
	require.Equal(t, Position{}, m.visualAnchor) // Anchor should be cleared
}

// TestVisualToggle_VInVisual_Keys verifies command returns correct keys
func TestVisualToggle_VInVisual_Keys(t *testing.T) {
	cmd := &VisualModeToggleVCommand{}
	require.Equal(t, []string{"v"}, cmd.Keys())
}

// TestVisualToggle_VInVisual_Mode verifies command operates in ModeVisual
func TestVisualToggle_VInVisual_Mode(t *testing.T) {
	cmd := &VisualModeToggleVCommand{}
	require.Equal(t, ModeVisual, cmd.Mode())
}

// TestVisualToggle_VInVisual_Metadata verifies command metadata
func TestVisualToggle_VInVisual_Metadata(t *testing.T) {
	cmd := &VisualModeToggleVCommand{}
	require.Equal(t, []string{"v"}, cmd.Keys())
	require.Equal(t, ModeVisual, cmd.Mode())
	require.Equal(t, "mode.visual_toggle_v", cmd.ID())
	require.False(t, cmd.IsUndoable())
	require.False(t, cmd.ChangesContent())
	require.True(t, cmd.IsModeChange())
}

// TestVisualToggle_VInVisualLine verifies 'V' in ModeVisualLine returns to Normal
func TestVisualToggle_VInVisualLine(t *testing.T) {
	m := newTestModelWithContent("hello", "world")
	m.mode = ModeVisualLine
	m.visualAnchor = Position{Row: 0, Col: 0}

	cmd := &VisualLineModeToggleShiftVCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, ModeNormal, m.mode)
	require.Equal(t, Position{}, m.visualAnchor) // Anchor should be cleared
}

// TestVisualToggle_VInVisualLine_Keys verifies command returns correct keys
func TestVisualToggle_VInVisualLine_Keys(t *testing.T) {
	cmd := &VisualLineModeToggleShiftVCommand{}
	require.Equal(t, []string{"V"}, cmd.Keys())
}

// TestVisualToggle_VInVisualLine_Mode verifies command operates in ModeVisualLine
func TestVisualToggle_VInVisualLine_Mode(t *testing.T) {
	cmd := &VisualLineModeToggleShiftVCommand{}
	require.Equal(t, ModeVisualLine, cmd.Mode())
}

// TestVisualToggle_VInVisualLine_Metadata verifies command metadata
func TestVisualToggle_VInVisualLine_Metadata(t *testing.T) {
	cmd := &VisualLineModeToggleShiftVCommand{}
	require.Equal(t, []string{"V"}, cmd.Keys())
	require.Equal(t, ModeVisualLine, cmd.Mode())
	require.Equal(t, "mode.visual_line_toggle_shift_v", cmd.ID())
	require.False(t, cmd.IsUndoable())
	require.False(t, cmd.ChangesContent())
	require.True(t, cmd.IsModeChange())
}

// TestVisualSwitch_ShiftVInVisual verifies 'V' in ModeVisual switches to VisualLine
func TestVisualSwitch_ShiftVInVisual(t *testing.T) {
	m := newTestModelWithContent("hello", "world")
	m.mode = ModeVisual
	m.visualAnchor = Position{Row: 0, Col: 2}

	cmd := &VisualModeToggleShiftVCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, ModeVisualLine, m.mode)
	// Anchor row should be preserved, Col should be set to 0 for line-wise
	require.Equal(t, Position{Row: 0, Col: 0}, m.visualAnchor)
}

// TestVisualSwitch_ShiftVInVisual_Keys verifies command returns correct keys
func TestVisualSwitch_ShiftVInVisual_Keys(t *testing.T) {
	cmd := &VisualModeToggleShiftVCommand{}
	require.Equal(t, []string{"V"}, cmd.Keys())
}

// TestVisualSwitch_ShiftVInVisual_Mode verifies command operates in ModeVisual
func TestVisualSwitch_ShiftVInVisual_Mode(t *testing.T) {
	cmd := &VisualModeToggleShiftVCommand{}
	require.Equal(t, ModeVisual, cmd.Mode())
}

// TestVisualSwitch_ShiftVInVisual_Metadata verifies command metadata
func TestVisualSwitch_ShiftVInVisual_Metadata(t *testing.T) {
	cmd := &VisualModeToggleShiftVCommand{}
	require.Equal(t, []string{"V"}, cmd.Keys())
	require.Equal(t, ModeVisual, cmd.Mode())
	require.Equal(t, "mode.visual_toggle_shift_v", cmd.ID())
	require.False(t, cmd.IsUndoable())
	require.False(t, cmd.ChangesContent())
	require.True(t, cmd.IsModeChange())
}

// TestVisualSwitch_VInVisualLine verifies 'v' in ModeVisualLine switches to Visual
func TestVisualSwitch_VInVisualLine(t *testing.T) {
	m := newTestModelWithContent("hello", "world")
	m.mode = ModeVisualLine
	m.visualAnchor = Position{Row: 0, Col: 0}

	cmd := &VisualLineModeToggleVCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, ModeVisual, m.mode)
	// Anchor should be preserved (still same position)
	require.Equal(t, Position{Row: 0, Col: 0}, m.visualAnchor)
}

// TestVisualSwitch_VInVisualLine_Keys verifies command returns correct keys
func TestVisualSwitch_VInVisualLine_Keys(t *testing.T) {
	cmd := &VisualLineModeToggleVCommand{}
	require.Equal(t, []string{"v"}, cmd.Keys())
}

// TestVisualSwitch_VInVisualLine_Mode verifies command operates in ModeVisualLine
func TestVisualSwitch_VInVisualLine_Mode(t *testing.T) {
	cmd := &VisualLineModeToggleVCommand{}
	require.Equal(t, ModeVisualLine, cmd.Mode())
}

// TestVisualSwitch_VInVisualLine_Metadata verifies command metadata
func TestVisualSwitch_VInVisualLine_Metadata(t *testing.T) {
	cmd := &VisualLineModeToggleVCommand{}
	require.Equal(t, []string{"v"}, cmd.Keys())
	require.Equal(t, ModeVisualLine, cmd.Mode())
	require.Equal(t, "mode.visual_line_toggle_v", cmd.ID())
	require.False(t, cmd.IsUndoable())
	require.False(t, cmd.ChangesContent())
	require.True(t, cmd.IsModeChange())
}
