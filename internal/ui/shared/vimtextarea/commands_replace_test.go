package vimtextarea

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

// ============================================================================
// ReplaceCharCommand Tests (r command)
// ============================================================================

// TestReplaceCharCommand_Execute verifies replacing character at cursor position
func TestReplaceCharCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 2 // position on first 'l'

	cmd := &ReplaceCharCommand{newChar: 'X'}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "heXlo", m.content[0])
	assert.Equal(t, 'l', cmd.oldChar)
	assert.Equal(t, 2, cmd.col)
	assert.Equal(t, 0, cmd.row)
}

// TestReplaceCharCommand_ExecuteAtStart verifies replacing first character
func TestReplaceCharCommand_ExecuteAtStart(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 0 // position at start

	cmd := &ReplaceCharCommand{newChar: 'H'}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "Hello", m.content[0])
	assert.Equal(t, 'h', cmd.oldChar)
}

// TestReplaceCharCommand_ExecuteAtEnd verifies replacing last character
func TestReplaceCharCommand_ExecuteAtEnd(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 4 // position on 'o'

	cmd := &ReplaceCharCommand{newChar: '!'}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "hell!", m.content[0])
	assert.Equal(t, 'o', cmd.oldChar)
}

// TestReplaceCharCommand_ExecuteEmptyLine verifies no-op on empty line
func TestReplaceCharCommand_ExecuteEmptyLine(t *testing.T) {
	m := newTestModelWithContent("")
	m.cursorCol = 0

	cmd := &ReplaceCharCommand{newChar: 'X'}
	result := cmd.Execute(m)

	assert.Equal(t, Skipped, result)
	assert.Equal(t, "", m.content[0])
}

// TestReplaceCharCommand_CursorUnchanged verifies cursor position doesn't change
func TestReplaceCharCommand_CursorUnchanged(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorRow = 0
	m.cursorCol = 2

	cmd := &ReplaceCharCommand{newChar: 'X'}
	_ = cmd.Execute(m)

	// Cursor should remain at the same position after replacement
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 2, m.cursorCol)
}

// TestReplaceCharCommand_Undo verifies restoring original character
func TestReplaceCharCommand_Undo(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 2

	cmd := &ReplaceCharCommand{newChar: 'X'}
	_ = cmd.Execute(m)
	assert.Equal(t, "heXlo", m.content[0])

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, "hello", m.content[0])
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 2, m.cursorCol)
}

// TestReplaceCharCommand_UndoAtStart verifies restoring first character
func TestReplaceCharCommand_UndoAtStart(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 0

	cmd := &ReplaceCharCommand{newChar: 'H'}
	_ = cmd.Execute(m)
	assert.Equal(t, "Hello", m.content[0])

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, "hello", m.content[0])
	assert.Equal(t, 0, m.cursorCol)
}

// TestReplaceCharCommand_UndoAtEnd verifies restoring last character
func TestReplaceCharCommand_UndoAtEnd(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 4

	cmd := &ReplaceCharCommand{newChar: '!'}
	_ = cmd.Execute(m)
	assert.Equal(t, "hell!", m.content[0])

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, "hello", m.content[0])
	assert.Equal(t, 4, m.cursorCol)
}

// TestReplaceCharCommand_ReplaceWithSpace verifies replacing with space character
func TestReplaceCharCommand_ReplaceWithSpace(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 2

	cmd := &ReplaceCharCommand{newChar: ' '}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "he lo", m.content[0])
}

// TestReplaceCharCommand_MultiLine verifies replacement on different lines
func TestReplaceCharCommand_MultiLine(t *testing.T) {
	m := newTestModelWithContent("hello", "world")
	m.cursorRow = 1
	m.cursorCol = 0

	cmd := &ReplaceCharCommand{newChar: 'W'}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "hello", m.content[0])
	assert.Equal(t, "World", m.content[1])
	assert.Equal(t, 1, cmd.row)
}

// TestReplaceCharCommand_Metadata verifies command metadata
func TestReplaceCharCommand_Metadata(t *testing.T) {
	cmd := &ReplaceCharCommand{}
	assert.Equal(t, []string{"r"}, cmd.Keys())
	assert.Equal(t, ModeNormal, cmd.Mode())
	assert.Equal(t, "replace.char", cmd.ID())
	assert.True(t, cmd.IsUndoable())
	assert.True(t, cmd.ChangesContent())
	assert.False(t, cmd.IsModeChange())
}

// ============================================================================
// Integration Tests for r command via pending mechanism
// ============================================================================

// newIntegrationModel creates a Model (value type) for integration tests.
func newIntegrationModel(content string) Model {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue(content)
	return m
}

// TestReplaceCommand_Integration_NormalFlow verifies r + char replaces character
func TestReplaceCommand_Integration_NormalFlow(t *testing.T) {
	m := newIntegrationModel("hello")
	m.cursorCol = 2

	// Press 'r' to enter pending state
	rMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}
	m, _ = m.Update(rMsg)

	// Verify pending state is set
	assert.Equal(t, 'r', m.pendingBuilder.Operator())

	// Press 'X' as replacement character
	xMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}}
	m, _ = m.Update(xMsg)

	// Verify replacement happened
	assert.Equal(t, "heXlo", m.content[0])
	assert.True(t, m.pendingBuilder.IsEmpty())
	assert.Equal(t, 2, m.cursorCol) // cursor unchanged
}

// TestReplaceCommand_Integration_EscapeCancels verifies r<Escape> cancels
func TestReplaceCommand_Integration_EscapeCancels(t *testing.T) {
	m := newIntegrationModel("hello")
	m.cursorCol = 2

	// Press 'r' to enter pending state
	rMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}
	m, _ = m.Update(rMsg)

	assert.Equal(t, 'r', m.pendingBuilder.Operator())

	// Press Escape to cancel
	escMsg := tea.KeyMsg{Type: tea.KeyEscape}
	m, _ = m.Update(escMsg)

	// Verify no change happened
	assert.Equal(t, "hello", m.content[0])
	assert.True(t, m.pendingBuilder.IsEmpty())
	assert.Equal(t, ModeNormal, m.mode)
}

// TestReplaceCommand_Integration_SpaceReplaces verifies r<Space> replaces with space
func TestReplaceCommand_Integration_SpaceReplaces(t *testing.T) {
	m := newIntegrationModel("hello")
	m.cursorCol = 2

	// Press 'r' to enter pending state
	rMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}
	m, _ = m.Update(rMsg)

	// Press Space as replacement character
	spaceMsg := tea.KeyMsg{Type: tea.KeySpace}
	m, _ = m.Update(spaceMsg)

	// Verify replacement with space happened
	assert.Equal(t, "he lo", m.content[0])
	assert.True(t, m.pendingBuilder.IsEmpty())
}

// TestReplaceCommand_Integration_EnterIgnored verifies r<Enter> is ignored
func TestReplaceCommand_Integration_EnterIgnored(t *testing.T) {
	m := newIntegrationModel("hello")
	m.cursorCol = 2

	// Press 'r' to enter pending state
	rMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}
	m, _ = m.Update(rMsg)

	// Press Enter (should be ignored, no line split)
	enterMsg := tea.KeyMsg{Type: tea.KeyEnter}
	m, _ = m.Update(enterMsg)

	// Verify no change happened
	assert.Equal(t, "hello", m.content[0])
	assert.Len(t, m.content, 1) // No line split
	assert.True(t, m.pendingBuilder.IsEmpty())
}

// TestReplaceCommand_Integration_EmptyLineSkips verifies r on empty line is no-op
func TestReplaceCommand_Integration_EmptyLineSkips(t *testing.T) {
	m := newIntegrationModel("")

	// Press 'r' to enter pending state
	rMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}
	m, _ = m.Update(rMsg)

	// Press 'X' as replacement character
	xMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}}
	m, _ = m.Update(xMsg)

	// Verify nothing happened (line is still empty)
	assert.Equal(t, "", m.content[0])
	assert.True(t, m.pendingBuilder.IsEmpty())
}

// TestReplaceCommand_Integration_UndoRestores verifies undo after r
func TestReplaceCommand_Integration_UndoRestores(t *testing.T) {
	m := newIntegrationModel("hello")
	m.cursorCol = 2

	// Press 'r' then 'X' to replace
	rMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}
	m, _ = m.Update(rMsg)
	xMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}}
	m, _ = m.Update(xMsg)

	assert.Equal(t, "heXlo", m.content[0])

	// Press 'u' to undo
	uMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}}
	m, _ = m.Update(uMsg)

	// Verify undo restored original
	assert.Equal(t, "hello", m.content[0])
}

// ============================================================================
// Replace Mode Tests (R command)
// ============================================================================

// TestEnterReplaceModeCommand_Execute verifies R enters Replace mode from Normal mode
func TestEnterReplaceModeCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeNormal

	cmd := &EnterReplaceModeCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, ModeReplace, m.mode)
}

// TestEnterReplaceModeCommand_Metadata verifies command metadata
func TestEnterReplaceModeCommand_Metadata(t *testing.T) {
	cmd := &EnterReplaceModeCommand{}
	assert.Equal(t, []string{"R"}, cmd.Keys())
	assert.Equal(t, ModeNormal, cmd.Mode())
	assert.Equal(t, "mode.replace", cmd.ID())
	assert.False(t, cmd.IsUndoable())
	assert.False(t, cmd.ChangesContent())
	assert.True(t, cmd.IsModeChange())
}

// TestReplaceModeCharCommand_Execute_Overwrite verifies typing overwrites character
func TestReplaceModeCharCommand_Execute_Overwrite(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeReplace
	m.cursorCol = 2 // position on first 'l'

	cmd := &ReplaceModeCharCommand{newChar: 'X'}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "heXlo", m.content[0])
	assert.Equal(t, 'l', cmd.oldChar)
	assert.Equal(t, 3, m.cursorCol) // cursor moved right
	assert.False(t, cmd.wasAtEOL)
}

// TestReplaceModeCharCommand_Execute_AtEOL_Appends verifies EOL appends character
func TestReplaceModeCharCommand_Execute_AtEOL_Appends(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeReplace
	m.cursorCol = 5 // position at end of line (after 'o')

	cmd := &ReplaceModeCharCommand{newChar: '!'}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "hello!", m.content[0])
	assert.Equal(t, rune(0), cmd.oldChar) // no character was replaced
	assert.Equal(t, 6, m.cursorCol)       // cursor at end
	assert.True(t, cmd.wasAtEOL)
}

// TestReplaceModeCharCommand_Execute_Sequence verifies multiple character overwrite
func TestReplaceModeCharCommand_Execute_Sequence(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeReplace
	m.cursorCol = 0

	// Type "XYZ" in Replace mode
	for _, ch := range "XYZ" {
		cmd := &ReplaceModeCharCommand{newChar: ch}
		result := cmd.Execute(m)
		assert.Equal(t, Executed, result)
	}

	assert.Equal(t, "XYZlo", m.content[0])
	assert.Equal(t, 3, m.cursorCol)
}

// TestReplaceModeCharCommand_Undo_Overwrite verifies undo restores replaced char
func TestReplaceModeCharCommand_Undo_Overwrite(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeReplace
	m.cursorCol = 2

	cmd := &ReplaceModeCharCommand{newChar: 'X'}
	_ = cmd.Execute(m)
	assert.Equal(t, "heXlo", m.content[0])
	assert.Equal(t, 3, m.cursorCol)

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, "hello", m.content[0])
	assert.Equal(t, 2, m.cursorCol) // cursor back to original position
}

// TestReplaceModeCharCommand_Undo_AtEOL verifies undo removes appended char
func TestReplaceModeCharCommand_Undo_AtEOL(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeReplace
	m.cursorCol = 5

	cmd := &ReplaceModeCharCommand{newChar: '!'}
	_ = cmd.Execute(m)
	assert.Equal(t, "hello!", m.content[0])
	assert.Equal(t, 6, m.cursorCol)

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, "hello", m.content[0])
	assert.Equal(t, 5, m.cursorCol)
}

// TestReplaceModeCharCommand_Metadata verifies command metadata
func TestReplaceModeCharCommand_Metadata(t *testing.T) {
	cmd := &ReplaceModeCharCommand{}
	assert.Nil(t, cmd.Keys()) // dynamically handled
	assert.Equal(t, ModeReplace, cmd.Mode())
	assert.Equal(t, "replace.mode_char", cmd.ID())
	assert.True(t, cmd.IsUndoable())
	assert.True(t, cmd.ChangesContent())
	assert.False(t, cmd.IsModeChange())
}

// TestReplaceModeEscapeCommand_Execute verifies Escape returns to Normal mode
func TestReplaceModeEscapeCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeReplace
	m.cursorCol = 3

	cmd := &ReplaceModeEscapeCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, ModeNormal, m.mode)
	assert.Equal(t, 2, m.cursorCol) // cursor moved back one
}

// TestReplaceModeEscapeCommand_Execute_AtStart verifies Escape at column 0
func TestReplaceModeEscapeCommand_Execute_AtStart(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeReplace
	m.cursorCol = 0

	cmd := &ReplaceModeEscapeCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, ModeNormal, m.mode)
	assert.Equal(t, 0, m.cursorCol) // cursor stays at 0
}

// TestReplaceModeEscapeCommand_Metadata verifies command metadata
func TestReplaceModeEscapeCommand_Metadata(t *testing.T) {
	cmd := &ReplaceModeEscapeCommand{}
	assert.Equal(t, []string{"<escape>"}, cmd.Keys())
	assert.Equal(t, ModeReplace, cmd.Mode())
	assert.Equal(t, "mode.replace_escape", cmd.ID())
	assert.False(t, cmd.IsUndoable())
	assert.False(t, cmd.ChangesContent())
	assert.True(t, cmd.IsModeChange())
}

// TestReplaceModeBackspaceCommand_Execute verifies backspace deletes previous char
func TestReplaceModeBackspaceCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeReplace
	m.cursorCol = 3

	cmd := &ReplaceModeBackspaceCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "helo", m.content[0])
	assert.Equal(t, 2, m.cursorCol)
	assert.Equal(t, 'l', cmd.deletedChar)
}

// TestReplaceModeBackspaceCommand_Execute_AtStart verifies backspace at column 0 is no-op
func TestReplaceModeBackspaceCommand_Execute_AtStart(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeReplace
	m.cursorCol = 0

	cmd := &ReplaceModeBackspaceCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Skipped, result)
	assert.Equal(t, "hello", m.content[0])
	assert.Equal(t, 0, m.cursorCol)
}

// TestReplaceModeBackspaceCommand_Undo verifies undo restores deleted char
func TestReplaceModeBackspaceCommand_Undo(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeReplace
	m.cursorCol = 3

	cmd := &ReplaceModeBackspaceCommand{}
	_ = cmd.Execute(m)
	assert.Equal(t, "helo", m.content[0])
	assert.Equal(t, 2, m.cursorCol)

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, "hello", m.content[0])
	assert.Equal(t, 3, m.cursorCol)
}

// TestReplaceModeBackspaceCommand_Metadata verifies command metadata
func TestReplaceModeBackspaceCommand_Metadata(t *testing.T) {
	cmd := &ReplaceModeBackspaceCommand{}
	assert.Equal(t, []string{"<backspace>"}, cmd.Keys())
	assert.Equal(t, ModeReplace, cmd.Mode())
	assert.Equal(t, "replace.mode_backspace", cmd.ID())
	assert.True(t, cmd.IsUndoable())
	assert.True(t, cmd.ChangesContent())
	assert.False(t, cmd.IsModeChange())
}

// ============================================================================
// Integration Tests for R command (Replace mode)
// ============================================================================

// TestReplaceMode_Integration_EnterAndType verifies R enters mode and typing overwrites
func TestReplaceMode_Integration_EnterAndType(t *testing.T) {
	m := newIntegrationModel("hello")
	m.cursorCol = 2

	// Press 'R' to enter Replace mode
	rMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}}
	m, _ = m.Update(rMsg)

	assert.Equal(t, ModeReplace, m.mode)

	// Type 'X' to overwrite 'l'
	xMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}}
	m, _ = m.Update(xMsg)

	assert.Equal(t, "heXlo", m.content[0])
	assert.Equal(t, 3, m.cursorCol)
	assert.Equal(t, ModeReplace, m.mode) // still in Replace mode
}

// TestReplaceMode_Integration_MultiChar verifies typing multiple characters
func TestReplaceMode_Integration_MultiChar(t *testing.T) {
	m := newIntegrationModel("hello")
	m.cursorCol = 0

	// Press 'R' to enter Replace mode
	rMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}}
	m, _ = m.Update(rMsg)

	// Type "ABC" to overwrite "hel"
	for _, ch := range "ABC" {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}}
		m, _ = m.Update(msg)
	}

	assert.Equal(t, "ABClo", m.content[0])
	assert.Equal(t, 3, m.cursorCol)
}

// TestReplaceMode_Integration_AtEOL verifies appending at end of line
func TestReplaceMode_Integration_AtEOL(t *testing.T) {
	m := newIntegrationModel("hi")
	m.cursorCol = 2 // at end of line

	// Press 'R' to enter Replace mode
	rMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}}
	m, _ = m.Update(rMsg)

	// Type "!" to append
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'!'}}
	m, _ = m.Update(msg)

	assert.Equal(t, "hi!", m.content[0])
	assert.Equal(t, 3, m.cursorCol)
}

// TestReplaceMode_Integration_Escape verifies Escape returns to Normal mode
func TestReplaceMode_Integration_Escape(t *testing.T) {
	m := newIntegrationModel("hello")
	m.cursorCol = 3

	// Press 'R' to enter Replace mode
	rMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}}
	m, _ = m.Update(rMsg)

	assert.Equal(t, ModeReplace, m.mode)

	// Press Escape to exit
	escMsg := tea.KeyMsg{Type: tea.KeyEscape}
	m, _ = m.Update(escMsg)

	assert.Equal(t, ModeNormal, m.mode)
	assert.Equal(t, 2, m.cursorCol) // cursor moved back one
}

// TestReplaceMode_Integration_EscapeAtStart verifies Escape at column 0
func TestReplaceMode_Integration_EscapeAtStart(t *testing.T) {
	m := newIntegrationModel("hello")
	m.cursorCol = 0

	// Press 'R' to enter Replace mode
	rMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}}
	m, _ = m.Update(rMsg)

	// Press Escape to exit
	escMsg := tea.KeyMsg{Type: tea.KeyEscape}
	m, _ = m.Update(escMsg)

	assert.Equal(t, ModeNormal, m.mode)
	assert.Equal(t, 0, m.cursorCol) // stays at 0
}

// TestReplaceMode_Integration_Backspace verifies backspace deletes in Replace mode
func TestReplaceMode_Integration_Backspace(t *testing.T) {
	m := newIntegrationModel("hello")
	m.cursorCol = 3

	// Press 'R' to enter Replace mode
	rMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}}
	m, _ = m.Update(rMsg)

	// Press backspace
	bsMsg := tea.KeyMsg{Type: tea.KeyBackspace}
	m, _ = m.Update(bsMsg)

	assert.Equal(t, "helo", m.content[0])
	assert.Equal(t, 2, m.cursorCol)
	assert.Equal(t, ModeReplace, m.mode) // still in Replace mode
}

// TestReplaceMode_Integration_UndoSingleChar verifies undo for single char replace
func TestReplaceMode_Integration_UndoSingleChar(t *testing.T) {
	m := newIntegrationModel("hello")
	m.cursorCol = 2

	// Press 'R' to enter Replace mode
	rMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}}
	m, _ = m.Update(rMsg)

	// Type 'X' to overwrite 'l'
	xMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}}
	m, _ = m.Update(xMsg)

	assert.Equal(t, "heXlo", m.content[0])

	// Press Escape first (to get to Normal mode for undo)
	escMsg := tea.KeyMsg{Type: tea.KeyEscape}
	m, _ = m.Update(escMsg)

	// Press 'u' to undo
	uMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}}
	m, _ = m.Update(uMsg)

	assert.Equal(t, "hello", m.content[0])
}

// TestReplaceMode_Integration_UndoMultiChar verifies undo for multi-char replace sequence
func TestReplaceMode_Integration_UndoMultiChar(t *testing.T) {
	m := newIntegrationModel("hello")
	m.cursorCol = 0

	// Press 'R' to enter Replace mode
	rMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}}
	m, _ = m.Update(rMsg)

	// Type "AB" to overwrite "he"
	for _, ch := range "AB" {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}}
		m, _ = m.Update(msg)
	}

	assert.Equal(t, "ABllo", m.content[0])

	// Press Escape first
	escMsg := tea.KeyMsg{Type: tea.KeyEscape}
	m, _ = m.Update(escMsg)

	// Press 'u' twice to undo both replacements (per-character undo)
	uMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}}
	m, _ = m.Update(uMsg)
	assert.Equal(t, "Aello", m.content[0]) // first undo restores 'e'

	m, _ = m.Update(uMsg)
	assert.Equal(t, "hello", m.content[0]) // second undo restores 'h'
}

// ============================================================================
// ReplaceModeSpaceCommand Tests
// ============================================================================

// TestReplaceModeSpaceCommand_Execute verifies space overwrites character
func TestReplaceModeSpaceCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeReplace
	m.cursorCol = 2

	cmd := &ReplaceModeSpaceCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "he lo", m.content[0])
	assert.Equal(t, 3, m.cursorCol)
}

// TestReplaceModeSpaceCommand_Execute_AtEOL verifies space appends at EOL
func TestReplaceModeSpaceCommand_Execute_AtEOL(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeReplace
	m.cursorCol = 5

	cmd := &ReplaceModeSpaceCommand{}
	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, "hello ", m.content[0])
	assert.Equal(t, 6, m.cursorCol)
}

// TestReplaceModeSpaceCommand_Undo verifies undo restores replaced char
func TestReplaceModeSpaceCommand_Undo(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.mode = ModeReplace
	m.cursorCol = 2

	cmd := &ReplaceModeSpaceCommand{}
	_ = cmd.Execute(m)
	assert.Equal(t, "he lo", m.content[0])

	err := cmd.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, "hello", m.content[0])
}

// TestReplaceModeSpaceCommand_Metadata verifies command metadata
func TestReplaceModeSpaceCommand_Metadata(t *testing.T) {
	cmd := &ReplaceModeSpaceCommand{}
	assert.Equal(t, []string{"<space>"}, cmd.Keys())
	assert.Equal(t, ModeReplace, cmd.Mode())
	assert.Equal(t, "replace.mode_space", cmd.ID())
	assert.True(t, cmd.IsUndoable())
	assert.True(t, cmd.ChangesContent())
	assert.False(t, cmd.IsModeChange())
}

// TestReplaceMode_Integration_Space verifies space key in Replace mode via integration
func TestReplaceMode_Integration_Space(t *testing.T) {
	m := newIntegrationModel("hello")
	m.cursorCol = 2

	// Press 'R' to enter Replace mode
	rMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}}
	m, _ = m.Update(rMsg)

	// Press Space to overwrite 'l' with space
	spaceMsg := tea.KeyMsg{Type: tea.KeySpace}
	m, _ = m.Update(spaceMsg)

	assert.Equal(t, "he lo", m.content[0])
	assert.Equal(t, 3, m.cursorCol)
	assert.Equal(t, ModeReplace, m.mode)
}
