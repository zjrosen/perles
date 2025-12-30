package vimtextarea

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// PendingCommandBuilder Tests
// ============================================================================

// TestPendingCommandBuilder_New verifies new builder is empty
func TestPendingCommandBuilder_New(t *testing.T) {
	b := NewPendingCommandBuilder()

	require.True(t, b.IsEmpty())
	require.Equal(t, rune(0), b.Operator())
	require.Equal(t, 1, b.count) // Default count
}

// TestPendingCommandBuilder_SetOperator verifies setting operator
func TestPendingCommandBuilder_SetOperator(t *testing.T) {
	b := NewPendingCommandBuilder()

	b.SetOperator('d')

	require.False(t, b.IsEmpty())
	require.Equal(t, 'd', b.Operator())
}

// TestPendingCommandBuilder_Clear verifies clearing state
func TestPendingCommandBuilder_Clear(t *testing.T) {
	b := NewPendingCommandBuilder()
	b.SetOperator('d')

	b.Clear()

	require.True(t, b.IsEmpty())
	require.Equal(t, rune(0), b.Operator())
	require.Equal(t, 1, b.count)
}

// ============================================================================
// UndoCommand Tests
// ============================================================================

// TestUndoCommand_Execute verifies undo executes when history available
func TestUndoCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello")
	// Add something to history
	insertCmd := &InsertTextCommand{row: 0, col: 5, text: " world"}
	_ = insertCmd.Execute(m)
	m.history.Push(insertCmd)
	require.Equal(t, "hello world", m.content[0])

	cmd := &UndoCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "hello", m.content[0])
}

// TestUndoCommand_ExecuteEmpty verifies undo is no-op with no history
func TestUndoCommand_ExecuteEmpty(t *testing.T) {
	m := newTestModelWithContent("hello")

	cmd := &UndoCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result) // Still returns Executed
	require.Equal(t, "hello", m.content[0])
}

// TestUndoCommand_Undo verifies undo's Undo is a no-op
func TestUndoCommand_Undo(t *testing.T) {
	cmd := &UndoCommand{}
	err := cmd.Undo(nil)
	require.NoError(t, err)
}

// TestUndoCommand_Metadata verifies command metadata
func TestUndoCommand_Metadata(t *testing.T) {
	cmd := &UndoCommand{}
	require.Equal(t, []string{"u"}, cmd.Keys())
	require.Equal(t, ModeNormal, cmd.Mode())
	require.Equal(t, "history.undo", cmd.ID())
	require.False(t, cmd.IsUndoable())
	require.True(t, cmd.ChangesContent())
	require.False(t, cmd.IsModeChange())
}

// ============================================================================
// RedoCommand Tests
// ============================================================================

// TestRedoCommand_Execute verifies redo executes when history available
func TestRedoCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello")
	// Add something to history and undo it
	insertCmd := &InsertTextCommand{row: 0, col: 5, text: " world"}
	_ = insertCmd.Execute(m)
	m.history.Push(insertCmd)
	_ = m.history.Undo(m) // Now we can redo
	require.Equal(t, "hello", m.content[0])

	cmd := &RedoCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "hello world", m.content[0])
}

// TestRedoCommand_ExecuteEmpty verifies redo is no-op with no redo history
func TestRedoCommand_ExecuteEmpty(t *testing.T) {
	m := newTestModelWithContent("hello")

	cmd := &RedoCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result) // Still returns Executed
	require.Equal(t, "hello", m.content[0])
}

// TestRedoCommand_Undo verifies redo's Undo is a no-op
func TestRedoCommand_Undo(t *testing.T) {
	cmd := &RedoCommand{}
	err := cmd.Undo(nil)
	require.NoError(t, err)
}

// TestRedoCommand_Metadata verifies command metadata
func TestRedoCommand_Metadata(t *testing.T) {
	cmd := &RedoCommand{}
	require.Equal(t, []string{"<ctrl+r>"}, cmd.Keys())
	require.Equal(t, ModeNormal, cmd.Mode())
	require.Equal(t, "history.redo", cmd.ID())
	require.False(t, cmd.IsUndoable())
	require.True(t, cmd.ChangesContent())
	require.False(t, cmd.IsModeChange())
}

// ============================================================================
// ConditionalRedoCommand Tests
// ============================================================================

// TestConditionalRedoCommand_Execute verifies redo when available
func TestConditionalRedoCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello")
	// Add something to history and undo it
	insertCmd := &InsertTextCommand{row: 0, col: 5, text: " world"}
	_ = insertCmd.Execute(m)
	m.history.Push(insertCmd)
	_ = m.history.Undo(m)
	require.Equal(t, "hello", m.content[0])

	cmd := &ConditionalRedoCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "hello world", m.content[0])
}

// TestConditionalRedoCommand_ExecutePassThrough verifies pass through when no redo
func TestConditionalRedoCommand_ExecutePassThrough(t *testing.T) {
	m := newTestModelWithContent("hello")

	cmd := &ConditionalRedoCommand{}
	result := cmd.Execute(m)

	require.Equal(t, PassThrough, result)
	require.Equal(t, "hello", m.content[0])
}

// TestConditionalRedoCommand_Undo verifies undo is a no-op
func TestConditionalRedoCommand_Undo(t *testing.T) {
	cmd := &ConditionalRedoCommand{}
	err := cmd.Undo(nil)
	require.NoError(t, err)
}

// TestConditionalRedoCommand_Metadata verifies command metadata
func TestConditionalRedoCommand_Metadata(t *testing.T) {
	cmd := &ConditionalRedoCommand{}
	require.Equal(t, []string{"<ctrl+r>"}, cmd.Keys())
	require.Equal(t, ModeNormal, cmd.Mode())
	require.Equal(t, "history.redo_conditional", cmd.ID())
	require.False(t, cmd.IsUndoable())
	require.True(t, cmd.ChangesContent())
	require.False(t, cmd.IsModeChange())
}

// ============================================================================
// StartPendingCommand Tests
// ============================================================================

// TestStartPendingCommand_Execute verifies setting pending operator
func TestStartPendingCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello")

	cmd := &StartPendingCommand{operator: 'd'}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, 'd', m.pendingBuilder.Operator())
}

// TestStartPendingCommand_Metadata verifies command metadata
func TestStartPendingCommand_Metadata(t *testing.T) {
	cmd := &StartPendingCommand{operator: 'd'}
	require.Equal(t, []string{"d"}, cmd.Keys())
	require.Equal(t, ModeNormal, cmd.Mode())
	require.Equal(t, "pending.d", cmd.ID())
	require.False(t, cmd.IsUndoable())
	require.False(t, cmd.ChangesContent())
	require.False(t, cmd.IsModeChange())
}

// TestStartPendingCommand_V_Execute verifies setting v as pending operator
func TestStartPendingCommand_V_Execute(t *testing.T) {
	m := newTestModelWithContent("hello world")

	cmd := &StartPendingCommand{operator: 'v'}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, 'v', m.pendingBuilder.Operator())
	// Should NOT be in visual mode yet - just pending state
	require.Equal(t, ModeNormal, m.mode)
}

// TestStartPendingCommand_V_Metadata verifies v operator metadata
func TestStartPendingCommand_V_Metadata(t *testing.T) {
	cmd := &StartPendingCommand{operator: 'v'}
	require.Equal(t, []string{"v"}, cmd.Keys())
	require.Equal(t, ModeNormal, cmd.Mode())
	require.Equal(t, "pending.v", cmd.ID())
	require.False(t, cmd.IsUndoable())
	require.False(t, cmd.ChangesContent())
	require.False(t, cmd.IsModeChange())
}

// TestDefaultRegistry_V_IsPendingOperator verifies 'v' in DefaultRegistry returns StartPendingCommand
func TestDefaultRegistry_V_IsPendingOperator(t *testing.T) {
	cmd, ok := DefaultRegistry.Get(ModeNormal, "v")
	require.True(t, ok, "DefaultRegistry should have 'v' command for Normal mode")
	require.NotNil(t, cmd)

	// Verify it's a StartPendingCommand (overwrites EnterVisualModeCommand)
	_, isPending := cmd.(*StartPendingCommand)
	require.True(t, isPending, "'v' should be a StartPendingCommand for text object support")
}

// ============================================================================
// Visual Operator Fallback Tests
// ============================================================================

// newVisualIntegrationModel creates a Model (value type) for visual mode integration tests.
func newVisualIntegrationModel(content ...string) Model {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue(strings.Join(content, "\n"))
	return m
}

// TestVisualOperatorFallback_NonTextObjectKey verifies 'v' followed by non-text-object key enters visual mode
func TestVisualOperatorFallback_NonTextObjectKey(t *testing.T) {
	m := newVisualIntegrationModel("hello world", "second line")

	// Press 'v' - enters pending state
	vMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}}
	m, _ = m.Update(vMsg)

	// Verify we're in pending state (not visual mode yet)
	require.Equal(t, ModeNormal, m.mode, "Should still be in Normal mode after 'v'")
	require.Equal(t, 'v', m.pendingBuilder.Operator(), "Pending operator should be 'v'")

	// Press 'j' - should fall back to visual mode and execute motion
	jMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}
	m, _ = m.Update(jMsg)

	// Verify fallback behavior: entered visual mode
	require.Equal(t, ModeVisual, m.mode, "'vj' should enter visual mode via fallback")
	require.True(t, m.pendingBuilder.IsEmpty(), "Pending should be cleared")
}

// TestVisualOperatorFallback_MotionKeyMovesSelection verifies fallback correctly applies motion
func TestVisualOperatorFallback_MotionKeyMovesSelection(t *testing.T) {
	m := newVisualIntegrationModel("hello", "world")
	m.cursorCol = 2 // cursor on 'l'

	// Press 'v' then 'j' (should enter visual and move down)
	vMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}}
	m, _ = m.Update(vMsg)

	jMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}
	m, _ = m.Update(jMsg)

	// Should be in visual mode with selection
	require.Equal(t, ModeVisual, m.mode)
	// Anchor should be at original position (row 0, col 2)
	require.Equal(t, 0, m.visualAnchor.Row)
	require.Equal(t, 2, m.visualAnchor.Col)
	// Cursor should have moved down
	require.Equal(t, 1, m.cursorRow)
}

// TestVisualOperatorFallback_UnrecognizedKey verifies unrecognized key just enters visual mode
func TestVisualOperatorFallback_UnrecognizedKey(t *testing.T) {
	m := newVisualIntegrationModel("hello world")
	m.cursorCol = 3

	// Press 'v' then 'z' (not a recognized visual mode command)
	vMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}}
	m, _ = m.Update(vMsg)

	zMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'z'}}
	m, _ = m.Update(zMsg)

	// Should still enter visual mode (fallback just enters visual)
	require.Equal(t, ModeVisual, m.mode, "Should enter visual mode even with unrecognized key")
	require.Equal(t, Position{Row: 0, Col: 3}, m.visualAnchor, "Anchor should be at cursor position")
}

// TestVisualOperatorFallback_PendingCleared verifies pending is cleared after fallback
func TestVisualOperatorFallback_PendingCleared(t *testing.T) {
	m := newVisualIntegrationModel("hello")

	// Press 'v' then 'h'
	vMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}}
	m, _ = m.Update(vMsg)

	hMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}}
	m, _ = m.Update(hMsg)

	// Pending should be cleared
	require.True(t, m.pendingBuilder.IsEmpty(), "Pending should be cleared after fallback")
}

// ============================================================================
// SubmitCommand Tests
// ============================================================================

// TestSubmitCommand_Execute verifies submit always executes
func TestSubmitCommand_Execute(t *testing.T) {
	m := newTestModelWithContent("hello")

	cmd := &SubmitCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
}

// TestSubmitCommand_Metadata verifies command metadata
func TestSubmitCommand_Metadata(t *testing.T) {
	cmd := &SubmitCommand{}
	require.Equal(t, []string{"<enter>", "<ctrl+j>"}, cmd.Keys())
	require.Equal(t, ModeInsert, cmd.Mode())
	require.Equal(t, "submit", cmd.ID())
	require.False(t, cmd.IsUndoable())
	require.False(t, cmd.ChangesContent())
	require.False(t, cmd.IsModeChange())
	require.True(t, cmd.IsSubmit())
}
