package vimtextarea

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"
)

// ============================================================================
// CommandHistory Tests
// ============================================================================

// TestNewCommandHistory verifies that a new CommandHistory is properly initialized.
func TestNewCommandHistory(t *testing.T) {
	h := NewCommandHistory()

	assert.NotNil(t, h)
	assert.NotNil(t, h.commands)
	assert.Empty(t, h.commands)
	assert.Equal(t, -1, h.undoIndex)
	assert.False(t, h.CanUndo())
	assert.False(t, h.CanRedo())
}

// TestCommandHistory_Push verifies that Push adds commands and updates the index correctly.
func TestCommandHistory_Push(t *testing.T) {
	h := NewCommandHistory()
	cmd1 := newMockCommand("cmd1")
	cmd2 := newMockCommand("cmd2")

	// Push first command
	h.Push(cmd1)
	assert.Len(t, h.commands, 1)
	assert.Equal(t, 0, h.undoIndex)
	assert.True(t, h.CanUndo())
	assert.False(t, h.CanRedo())

	// Push second command
	h.Push(cmd2)
	assert.Len(t, h.commands, 2)
	assert.Equal(t, 1, h.undoIndex)
	assert.True(t, h.CanUndo())
	assert.False(t, h.CanRedo())
}

// TestCommandHistory_Undo verifies that Undo calls command.Undo and decrements the index.
func TestCommandHistory_Undo(t *testing.T) {
	h := NewCommandHistory()
	m := &Model{}
	cmd := newMockCommand("test")

	// Push and then undo
	h.Push(cmd)
	err := h.Undo(m)

	assert.NoError(t, err)
	assert.True(t, cmd.undone)
	assert.Equal(t, -1, h.undoIndex)
	assert.False(t, h.CanUndo())
	assert.True(t, h.CanRedo())
}

// TestCommandHistory_Redo verifies that Redo calls command.Execute and increments the index.
func TestCommandHistory_Redo(t *testing.T) {
	h := NewCommandHistory()
	m := &Model{}
	cmd := newMockCommand("test")

	// Push, undo, then redo
	h.Push(cmd)
	_ = h.Undo(m)

	// Reset the mock state to verify redo
	cmd.executed = false
	cmd.undone = false

	err := h.Redo(m)

	assert.NoError(t, err)
	assert.True(t, cmd.executed)
	assert.Equal(t, 0, h.undoIndex)
	assert.True(t, h.CanUndo())
	assert.False(t, h.CanRedo())
}

// TestCommandHistory_PushAfterUndo verifies that Push after undo truncates future commands.
func TestCommandHistory_PushAfterUndo(t *testing.T) {
	h := NewCommandHistory()
	m := &Model{}
	cmd1 := newMockCommand("cmd1")
	cmd2 := newMockCommand("cmd2")
	cmd3 := newMockCommand("cmd3")

	// Push two commands
	h.Push(cmd1)
	h.Push(cmd2)
	assert.Len(t, h.commands, 2)

	// Undo one
	_ = h.Undo(m)
	assert.Equal(t, 0, h.undoIndex)
	assert.True(t, h.CanRedo())

	// Push a new command - should invalidate redo (truncate cmd2)
	h.Push(cmd3)
	assert.Len(t, h.commands, 2) // cmd1 + cmd3, cmd2 is gone
	assert.Equal(t, 1, h.undoIndex)
	assert.False(t, h.CanRedo()) // Redo is now invalid
	assert.Same(t, cmd1, h.commands[0])
	assert.Same(t, cmd3, h.commands[1])
}

// TestCommandHistory_CanUndo verifies CanUndo returns false at base state.
func TestCommandHistory_CanUndo(t *testing.T) {
	h := NewCommandHistory()
	m := &Model{}

	// Empty history
	assert.False(t, h.CanUndo())

	// After push
	cmd := newMockCommand("test")
	h.Push(cmd)
	assert.True(t, h.CanUndo())

	// After undo back to base
	_ = h.Undo(m)
	assert.False(t, h.CanUndo())
}

// TestCommandHistory_CanRedo verifies CanRedo returns false at latest command.
func TestCommandHistory_CanRedo(t *testing.T) {
	h := NewCommandHistory()
	m := &Model{}

	// Empty history
	assert.False(t, h.CanRedo())

	// After push (at latest)
	cmd := newMockCommand("test")
	h.Push(cmd)
	assert.False(t, h.CanRedo())

	// After undo
	_ = h.Undo(m)
	assert.True(t, h.CanRedo())

	// After redo (back at latest)
	_ = h.Redo(m)
	assert.False(t, h.CanRedo())
}

// TestCommandHistory_Clear verifies that Clear resets to empty state.
func TestCommandHistory_Clear(t *testing.T) {
	h := NewCommandHistory()
	cmd1 := newMockCommand("cmd1")
	cmd2 := newMockCommand("cmd2")

	// Add some commands
	h.Push(cmd1)
	h.Push(cmd2)
	assert.Len(t, h.commands, 2)
	assert.Equal(t, 1, h.undoIndex)

	// Clear
	h.Clear()
	assert.Empty(t, h.commands)
	assert.Equal(t, -1, h.undoIndex)
	assert.False(t, h.CanUndo())
	assert.False(t, h.CanRedo())
}

// TestCommandHistory_UndoAtBase verifies that Undo at base state is a no-op.
func TestCommandHistory_UndoAtBase(t *testing.T) {
	h := NewCommandHistory()
	m := &Model{}

	// Undo on empty history - should be no-op
	err := h.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, -1, h.undoIndex)

	// Add and undo a command
	cmd := newMockCommand("test")
	h.Push(cmd)
	_ = h.Undo(m)

	// Try to undo again at base - should be no-op
	err = h.Undo(m)
	assert.NoError(t, err)
	assert.Equal(t, -1, h.undoIndex)
}

// TestCommandHistory_RedoAtLatest verifies that Redo at latest command is a no-op.
func TestCommandHistory_RedoAtLatest(t *testing.T) {
	h := NewCommandHistory()
	m := &Model{}

	// Redo on empty history - should be no-op
	err := h.Redo(m)
	assert.NoError(t, err)
	assert.Equal(t, -1, h.undoIndex)

	// Add a command (now at latest)
	cmd := newMockCommand("test")
	h.Push(cmd)

	// Try to redo at latest - should be no-op
	err = h.Redo(m)
	assert.NoError(t, err)
	assert.Equal(t, 0, h.undoIndex)
}

// TestCommandHistory_UndoError verifies error propagation from command.Undo.
func TestCommandHistory_UndoError(t *testing.T) {
	h := NewCommandHistory()
	m := &Model{}
	cmd := &mockCommand{key: "test.failing", failUndo: true}

	h.Push(cmd)
	err := h.Undo(m)

	assert.Error(t, err)
	// Note: undoIndex is still decremented even on error (current behavior)
	// This matches the design where errors are informational
}

// TestCommandHistory_RedoError verifies redo with failing command returns no error.
// Note: With ExecuteResult, errors are not propagated through Redo - just the result is discarded.
func TestCommandHistory_RedoError(t *testing.T) {
	h := NewCommandHistory()
	m := &Model{}
	cmd := &mockCommand{key: "test.failing", failExecute: true}

	h.Push(cmd)
	_ = h.Undo(m)

	// Now redo should succeed (error is not propagated from Execute result)
	cmd.failExecute = true
	err := h.Redo(m)

	assert.NoError(t, err)
}

// TestCommandHistory_MultipleUndoRedo verifies a sequence of undo/redo operations.
func TestCommandHistory_MultipleUndoRedo(t *testing.T) {
	h := NewCommandHistory()
	m := &Model{}
	cmd1 := newMockCommand("cmd1")
	cmd2 := newMockCommand("cmd2")
	cmd3 := newMockCommand("cmd3")

	// Push three commands
	h.Push(cmd1)
	h.Push(cmd2)
	h.Push(cmd3)
	assert.Equal(t, 2, h.undoIndex)

	// Undo all three
	_ = h.Undo(m)
	assert.Equal(t, 1, h.undoIndex)
	assert.True(t, cmd3.undone)

	_ = h.Undo(m)
	assert.Equal(t, 0, h.undoIndex)
	assert.True(t, cmd2.undone)

	_ = h.Undo(m)
	assert.Equal(t, -1, h.undoIndex)
	assert.True(t, cmd1.undone)

	// Verify we can't undo further
	assert.False(t, h.CanUndo())

	// Redo all three
	_ = h.Redo(m)
	assert.Equal(t, 0, h.undoIndex)
	assert.True(t, cmd1.executed)

	_ = h.Redo(m)
	assert.Equal(t, 1, h.undoIndex)
	assert.True(t, cmd2.executed)

	_ = h.Redo(m)
	assert.Equal(t, 2, h.undoIndex)
	assert.True(t, cmd3.executed)

	// Verify we can't redo further
	assert.False(t, h.CanRedo())
}

// TestCommandHistory_BranchingHistory verifies that pushing after undo creates a new branch.
func TestCommandHistory_BranchingHistory(t *testing.T) {
	h := NewCommandHistory()
	m := &Model{}
	cmdA := newMockCommand("A")
	cmdB := newMockCommand("B")
	cmdC := newMockCommand("C")
	cmdD := newMockCommand("D")

	// Push A, B, C
	h.Push(cmdA)
	h.Push(cmdB)
	h.Push(cmdC)
	assert.Len(t, h.commands, 3)

	// Undo to A
	_ = h.Undo(m) // undo C
	_ = h.Undo(m) // undo B
	assert.Equal(t, 0, h.undoIndex)

	// Push D - creates new branch, B and C are lost
	h.Push(cmdD)
	assert.Len(t, h.commands, 2)
	assert.Same(t, cmdA, h.commands[0])
	assert.Same(t, cmdD, h.commands[1])
	assert.Equal(t, 1, h.undoIndex)

	// Verify B and C are no longer accessible
	assert.False(t, h.CanRedo())
}

// ============================================================================
// Equivalence Tests
// ============================================================================

// TestEquivalence_DeleteCharCommand_x verifies 'x' command matches old deleteCharUnderCursor()
func TestEquivalence_DeleteCharCommand_x(t *testing.T) {
	// Setup two identical models
	m1 := newTestModelWithContent("hello")
	m1.cursorCol = 2
	m2 := newTestModelWithContent("hello")
	m2.cursorCol = 2

	// Execute using old method (via copy to simulate)
	oldContent := m1.content[0][:2] + m1.content[0][3:] // "helo"

	// Execute using new command
	cmd := &DeleteCharCommand{row: 0, col: 2}
	_ = cmd.Execute(m2)

	assert.Equal(t, oldContent, m2.content[0])
}

// TestEquivalence_DeleteLineCommand_dd verifies 'dd' command matches old deleteLine()
func TestEquivalence_DeleteLineCommand_dd(t *testing.T) {
	// Setup two identical models
	m1 := newTestModelWithContent("line1", "line2", "line3")
	m1.cursorRow = 1
	m2 := newTestModelWithContent("line1", "line2", "line3")
	m2.cursorRow = 1

	// Expected result after deleting line2
	expectedContent := []string{"line1", "line3"}

	// Execute using new command
	cmd := &DeleteLineCommand{}
	_ = cmd.Execute(m2)

	assert.Equal(t, expectedContent, m2.content)
}

// ============================================================================
// Integration Tests - Command + Undo/Redo via History
// ============================================================================

// TestIntegration_DeleteLine_ThenUndo verifies 'dd' then 'u' restores line
func TestIntegration_DeleteLine_ThenUndo(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.cursorRow = 1

	// Execute delete line command
	cmd := &DeleteLineCommand{}
	_ = cmd.Execute(m)
	m.history.Push(cmd)

	assert.Len(t, m.content, 2)
	assert.Equal(t, "line1", m.content[0])
	assert.Equal(t, "line3", m.content[1])

	// Undo
	err := m.history.Undo(m)
	assert.NoError(t, err)

	assert.Len(t, m.content, 3)
	assert.Equal(t, "line1", m.content[0])
	assert.Equal(t, "line2", m.content[1])
	assert.Equal(t, "line3", m.content[2])
}

// TestIntegration_MultipleDeletes_ThenUndo verifies multiple deletes can be undone
func TestIntegration_MultipleDeletes_ThenUndo(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 0

	// Delete word
	cmd1 := &DeleteWordCommand{row: 0, col: 0}
	_ = cmd1.Execute(m)
	m.history.Push(cmd1)
	assert.Equal(t, "world", m.content[0])

	// Delete char
	cmd2 := &DeleteCharCommand{row: 0, col: 0}
	_ = cmd2.Execute(m)
	m.history.Push(cmd2)
	assert.Equal(t, "orld", m.content[0])

	// Undo char delete
	_ = m.history.Undo(m)
	assert.Equal(t, "world", m.content[0])

	// Undo word delete
	_ = m.history.Undo(m)
	assert.Equal(t, "hello world", m.content[0])
}

// TestIntegration_Delete_Undo_Redo verifies full undo/redo cycle
func TestIntegration_Delete_Undo_Redo(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 0

	// Delete char
	cmd := &DeleteCharCommand{row: 0, col: 0}
	_ = cmd.Execute(m)
	m.history.Push(cmd)
	assert.Equal(t, "ello", m.content[0])

	// Undo
	_ = m.history.Undo(m)
	assert.Equal(t, "hello", m.content[0])

	// Redo
	_ = m.history.Redo(m)
	assert.Equal(t, "ello", m.content[0])
}

// TestIntegration_o_TypeText_Esc_Undo verifies 'o' + type + ESC + 'u'
func TestIntegration_o_TypeText_Esc_Undo(t *testing.T) {
	m := newTestModelWithContent("line1", "line2")
	m.cursorRow = 0

	// Execute 'o' (insert line below)
	insertCmd := &InsertLineCommand{above: false}
	_ = insertCmd.Execute(m)
	m.history.Push(insertCmd)

	assert.Len(t, m.content, 3)
	assert.Equal(t, 1, m.cursorRow)

	// Type some text on the new line
	textCmd := &InsertTextCommand{row: 1, col: 0, text: "new content"}
	_ = textCmd.Execute(m)
	m.history.Push(textCmd)

	assert.Equal(t, "new content", m.content[1])

	// Undo typing
	_ = m.history.Undo(m)
	assert.Equal(t, "", m.content[1])

	// Undo line insertion
	_ = m.history.Undo(m)
	assert.Len(t, m.content, 2)
	assert.Equal(t, "line1", m.content[0])
	assert.Equal(t, "line2", m.content[1])
}

// ============================================================================
// CommandRegistry Tests
// ============================================================================

// TestNewCommandRegistry verifies that a new CommandRegistry is properly initialized
func TestNewCommandRegistry(t *testing.T) {
	r := NewCommandRegistry()

	assert.NotNil(t, r)
	assert.NotNil(t, r.commands)
	assert.Empty(t, r.commands)
}

// TestCommandRegistry_Register verifies command registration
func TestCommandRegistry_Register(t *testing.T) {
	r := NewCommandRegistry()

	r.Register(&MoveLeftCommand{})

	_, ok := r.Get(ModeNormal, "h")
	assert.True(t, ok)
}

// TestCommandRegistry_Create_NotFoundKey verifies Create returns false for unregistered commands
func TestCommandRegistry_Create_NotFoundKey(t *testing.T) {
	r := NewCommandRegistry()

	_, ok := r.Get(ModeNormal, "nonexistent")
	assert.False(t, ok)
}

// TestCommandRegistry_Create verifies command creation
func TestCommandRegistry_Create(t *testing.T) {
	r := NewCommandRegistry()
	r.Register(&DeleteCharCommand{})

	cmd, ok := r.Get(ModeNormal, "x")

	assert.True(t, ok)
	assert.NotNil(t, cmd)
	assert.Equal(t, "x", cmd.Keys()[0])

	// Commands capture state in Execute(), not at creation
	deleteCmd := cmd.(*DeleteCharCommand)
	assert.Equal(t, 0, deleteCmd.row) // Initial zero value
	assert.Equal(t, 0, deleteCmd.col) // Initial zero value
}

// TestCommandRegistry_Create_NotFound verifies Create returns false for unregistered commands
func TestCommandRegistry_Create_NotFound(t *testing.T) {
	r := NewCommandRegistry()

	cmd, ok := r.Get(ModeNormal, "nonexistent")

	assert.False(t, ok)
	assert.Nil(t, cmd)
}

// TestCommandRegistry_MultipleCommands verifies multiple command registration
func TestCommandRegistry_MultipleCommands(t *testing.T) {
	r := NewCommandRegistry()
	r.Register(&MoveLeftCommand{})
	r.Register(&MoveRightCommand{})
	r.Register(&MoveDownCommand{})

	// Verify all registered commands can be created
	for _, key := range []string{"h", "l", "j"} {
		cmd, ok := r.Get(ModeNormal, key)
		assert.True(t, ok, "Should create command for key: %s", key)
		assert.NotNil(t, cmd)
	}
}

// TestDefaultRegistry_HasAllNormalModeCommands verifies DefaultRegistry has all Normal mode commands registered
// Note: Multi-key delete commands (d$, dj, dk) are handled by PendingCommandBuilder, not the registry
func TestDefaultRegistry_HasAllNormalModeCommands(t *testing.T) {
	expectedKeys := []string{
		// Motion commands
		"h", "l", "j", "k", "w", "b", "e", "0", "$", "^", "G", "gg",
		// Delete commands (single-key or first-key of multi-key sequences)
		"x", "D", "dd", "dw",
		// Mode entry commands
		"i", "a", "A", "I", "o", "O",
		// Visual mode entry commands
		"v", "V",
		// Special commands
		"u", "<ctrl+r>", "g", "d",
	}

	for _, key := range expectedKeys {
		t.Run(key, func(t *testing.T) {
			_, ok := DefaultRegistry.Get(ModeNormal, key)
			assert.True(t, ok, "DefaultRegistry should have Normal mode command: %s", key)
		})
	}
}

// TestDefaultRegistry_HasAllInsertModeCommands verifies DefaultRegistry has all Insert mode commands registered
func TestDefaultRegistry_HasAllInsertModeCommands(t *testing.T) {
	expectedKeys := []string{
		"<escape>", "<backspace>", "<delete>", "<enter>",
	}

	for _, key := range expectedKeys {
		t.Run(key, func(t *testing.T) {
			_, ok := DefaultRegistry.Get(ModeInsert, key)
			assert.True(t, ok, "DefaultRegistry should have Insert mode command: %s", key)
		})
	}
}

// TestDefaultRegistry_CommandsCanBeCreated verifies all registered commands can be created
func TestDefaultRegistry_CommandsCanBeCreated(t *testing.T) {
	m := newTestModelWithContent("hello")

	// Test a subset of known Normal mode keys
	normalModeKeys := []string{"h", "j", "k", "l", "w", "b", "e", "0", "$", "^", "G", "x", "D", "u"}
	for _, key := range normalModeKeys {
		t.Run(key, func(t *testing.T) {
			cmd, ok := DefaultRegistry.Get(ModeNormal, key)
			assert.True(t, ok, "Should be able to create command for key: %s", key)
			assert.NotNil(t, cmd)
			assert.NotEmpty(t, cmd.ID(), "Command ID should not be empty")
		})
	}
	_ = m // Keep unused variable
}

// ============================================================================
// IsUndoable Tests
// ============================================================================

// TestIsUndoable verifies isUndoable identifies undoable commands correctly
// (IsMotionKey was removed and replaced with isUndoable which uses type assertions)
func TestIsUndoable(t *testing.T) {
	m := newTestModelWithContent("hello")

	// Motion commands are NOT undoable
	motionCommands := []Command{
		&MoveLeftCommand{},
		&MoveRightCommand{},
		&MoveDownCommand{},
		&MoveUpCommand{},
		&MoveWordForwardCommand{},
		&MoveWordBackwardCommand{},
		&MoveWordEndCommand{},
		&MoveToLineStartCommand{},
		&MoveToLineEndCommand{},
		&MoveToFirstNonBlankCommand{},
		&MoveToFirstLineCommand{},
		&MoveToLastLineCommand{},
	}
	for _, cmd := range motionCommands {
		t.Run("motion_"+cmd.ID(), func(t *testing.T) {
			assert.False(t, isUndoable(cmd), "Motion command %s should not be undoable", cmd.ID())
		})
	}

	// Mode entry commands (without content change) are NOT undoable
	modeCommands := []Command{
		&EnterInsertModeCommand{},
		&EnterInsertModeAfterCommand{},
		&EnterInsertModeAtEndCommand{},
		&EnterInsertModeAtStartCommand{},
		&EnterVisualModeCommand{},
		&EnterVisualLineModeCommand{},
		&EscapeCommand{},
		&StartPendingCommand{operator: 'd'},
	}
	for _, cmd := range modeCommands {
		t.Run("mode_"+cmd.ID(), func(t *testing.T) {
			assert.False(t, isUndoable(cmd), "Mode command %s should not be undoable", cmd.ID())
		})
	}

	// Undo/Redo commands are NOT added to history
	historyCommands := []Command{
		&UndoCommand{},
		&RedoCommand{},
	}
	for _, cmd := range historyCommands {
		t.Run("history_"+cmd.ID(), func(t *testing.T) {
			assert.False(t, isUndoable(cmd), "History command %s should not be undoable", cmd.ID())
		})
	}

	// Content-mutating commands ARE undoable
	contentCommands := []Command{
		&DeleteCharCommand{row: m.cursorRow, col: m.cursorCol},
		&DeleteLineCommand{},
		&DeleteToEOLCommand{row: m.cursorRow, col: m.cursorCol},
		&DeleteWordCommand{row: m.cursorRow, col: m.cursorCol},
		&DeleteLinesCommand{startRow: m.cursorRow, count: 1},
		&InsertTextCommand{row: m.cursorRow, col: m.cursorCol, text: "x"},
		&SplitLineCommand{row: m.cursorRow, col: m.cursorCol},
		&BackspaceCommand{},
		&DeleteKeyCommand{},
		&InsertLineCommand{above: false},
		&InsertLineBelowCommand{},
		&InsertLineAboveCommand{},
	}
	for _, cmd := range contentCommands {
		t.Run("content_"+cmd.ID(), func(t *testing.T) {
			assert.True(t, isUndoable(cmd), "Content command %s should be undoable", cmd.ID())
		})
	}
}

// TestAllCommandKeys_AreUnique verifies no duplicate keys exist among all command types
func TestAllCommandKeys_AreUnique(t *testing.T) {
	m := newTestModelWithContent("hello")

	// Collect all command keys
	commands := []Command{
		&DeleteCharCommand{},
		&DeleteLineCommand{},
		&DeleteToEOLCommand{},
		&DeleteWordCommand{},
		&DeleteLinesCommand{},
		&BackspaceCommand{},
		&DeleteKeyCommand{},
		&InsertTextCommand{},
		&SplitLineCommand{},
		&InsertLineCommand{above: false},
		&InsertLineCommand{above: true},
		&MoveLeftCommand{},
		&MoveRightCommand{},
		&MoveDownCommand{},
		&MoveUpCommand{},
		&MoveWordForwardCommand{},
		&MoveWordBackwardCommand{},
		&MoveWordEndCommand{},
		&MoveToLineStartCommand{},
		&MoveToLineEndCommand{},
		&MoveToFirstNonBlankCommand{},
		&MoveToFirstLineCommand{},
		&MoveToLastLineCommand{},
	}

	keys := make(map[string]bool)
	for _, cmd := range commands {
		key := cmd.Keys()[0]
		assert.False(t, keys[key], "Duplicate key found: %s", key)
		keys[key] = true
	}

	// InsertLineCommand has two keys based on 'above' field - verify both are different
	aboveCmd := &InsertLineCommand{above: true}
	belowCmd := &InsertLineCommand{above: false}
	assert.NotEqual(t, aboveCmd.Keys()[0], belowCmd.Keys()[0])

	// Ignore unused variable warning
	_ = m
}

// ============================================================================
// Property-Based Tests
// ============================================================================

// TestProperty_UndoReversesCommands verifies that undoing all commands
// returns the model to its original state.
func TestProperty_UndoReversesCommands(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Create initial model with some content
		initialLines := rapid.SliceOfN(
			rapid.StringMatching(`[a-zA-Z0-9 ]{0,20}`),
			1, 5,
		).Draw(t, "initialLines")

		m := newTestModelWithContent(initialLines...)

		// Capture initial state
		initialContent := make([]string, len(m.content))
		copy(initialContent, m.content)

		// Generate and execute a sequence of content-mutating commands
		numCommands := rapid.IntRange(1, 10).Draw(t, "numCommands")
		for i := 0; i < numCommands; i++ {
			cmd := generateContentMutatingCommand(t, m)
			_ = cmd.Execute(m)
			m.history.Push(cmd)

			// Ensure cursor is valid after each command
			m.clampCursor()
		}

		// Undo all commands
		for m.history.CanUndo() {
			_ = m.history.Undo(m)
		}

		// Verify we're back to initial state
		assert.Equal(t, initialContent, m.content, "Content should be restored after undoing all commands")
	})
}

// TestProperty_RedoConsistency verifies that undo then redo returns to pre-undo state.
func TestProperty_RedoConsistency(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Create initial model with some content
		initialLines := rapid.SliceOfN(
			rapid.StringMatching(`[a-zA-Z0-9 ]{0,20}`),
			1, 5,
		).Draw(t, "initialLines")

		m := newTestModelWithContent(initialLines...)

		// Execute a single command
		cmd := generateContentMutatingCommand(t, m)
		_ = cmd.Execute(m)
		m.history.Push(cmd)

		// Capture state after command
		contentAfterCommand := make([]string, len(m.content))
		copy(contentAfterCommand, m.content)
		rowAfterCommand := m.cursorRow
		colAfterCommand := m.cursorCol

		// Undo
		_ = m.history.Undo(m)

		// Redo
		_ = m.history.Redo(m)

		// Verify state is restored to post-command state
		assert.Equal(t, contentAfterCommand, m.content, "Content should match after undo+redo")
		assert.Equal(t, rowAfterCommand, m.cursorRow, "CursorRow should match after undo+redo")
		assert.Equal(t, colAfterCommand, m.cursorCol, "CursorCol should match after undo+redo")
	})
}

// TestProperty_CursorAlwaysValid verifies cursor is always within valid bounds after any command.
func TestProperty_CursorAlwaysValid(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Create initial model with some content
		initialLines := rapid.SliceOfN(
			rapid.StringMatching(`[a-zA-Z0-9 ]{0,20}`),
			1, 5,
		).Draw(t, "initialLines")

		m := newTestModelWithContent(initialLines...)

		// Execute a sequence of mixed commands (content-mutating and motions)
		numCommands := rapid.IntRange(1, 20).Draw(t, "numCommands")
		for i := 0; i < numCommands; i++ {
			// 50% chance of motion command, 50% chance of content command
			isMotion := rapid.Bool().Draw(t, "isMotion")

			if isMotion {
				// Execute a random motion command
				motionType := rapid.IntRange(0, 11).Draw(t, "motionType")
				var cmd Command
				switch motionType {
				case 0:
					cmd = &MoveLeftCommand{}
				case 1:
					cmd = &MoveRightCommand{}
				case 2:
					cmd = &MoveDownCommand{}
				case 3:
					cmd = &MoveUpCommand{}
				case 4:
					cmd = &MoveWordForwardCommand{}
				case 5:
					cmd = &MoveWordBackwardCommand{}
				case 6:
					cmd = &MoveWordEndCommand{}
				case 7:
					cmd = &MoveToLineStartCommand{}
				case 8:
					cmd = &MoveToLineEndCommand{}
				case 9:
					cmd = &MoveToFirstNonBlankCommand{}
				case 10:
					cmd = &MoveToFirstLineCommand{}
				case 11:
					cmd = &MoveToLastLineCommand{}
				}
				_ = cmd.Execute(m)
				// Motion commands are NOT pushed to history
			} else {
				cmd := generateContentMutatingCommand(t, m)
				_ = cmd.Execute(m)
				m.history.Push(cmd)
			}

			// After every command, cursor should be valid
			assert.GreaterOrEqual(t, m.cursorRow, 0, "cursorRow should be >= 0")
			assert.Less(t, m.cursorRow, len(m.content), "cursorRow should be < len(content)")
			assert.GreaterOrEqual(t, m.cursorCol, 0, "cursorCol should be >= 0")
			// In Insert mode, cursor can be at len(line) (after last char)
			// In Normal mode, cursor should be at most len(line)-1 or 0 for empty lines
			maxCol := len(m.content[m.cursorRow])
			// Note: We allow cursor at maxCol for Insert mode, which is valid
			// (cursor can be after the last character in Insert mode)
			assert.LessOrEqual(t, m.cursorCol, maxCol, "cursorCol should be within line bounds")
		}
	})
}

// TestProperty_ContentIntegrity verifies content never contains nil or invalid state.
func TestProperty_ContentIntegrity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Create initial model with some content
		initialLines := rapid.SliceOfN(
			rapid.StringMatching(`[a-zA-Z0-9 ]{0,20}`),
			1, 5,
		).Draw(t, "initialLines")

		m := newTestModelWithContent(initialLines...)

		// Execute a sequence of commands
		numCommands := rapid.IntRange(1, 15).Draw(t, "numCommands")
		for i := 0; i < numCommands; i++ {
			cmd := generateContentMutatingCommand(t, m)
			_ = cmd.Execute(m)
			m.history.Push(cmd)

			// After every command, content should be valid
			assert.NotNil(t, m.content, "content should never be nil")
			assert.GreaterOrEqual(t, len(m.content), 1, "content should always have at least one line")

			// Every line should be a valid string (not nil in Go, so just check it exists)
			for lineIdx, line := range m.content {
				assert.NotNil(t, line, "line %d should not be nil", lineIdx)
				// Lines should not contain newlines (they should be separate lines)
				assert.NotContains(t, line, "\n", "line %d should not contain embedded newlines", lineIdx)
			}
		}

		// Also check after undoing all commands
		for m.history.CanUndo() {
			_ = m.history.Undo(m)

			assert.NotNil(t, m.content, "content should never be nil after undo")
			assert.GreaterOrEqual(t, len(m.content), 1, "content should always have at least one line after undo")
		}
	})
}

// ============================================================================
// Visual Mode Entry Integration Tests
// ============================================================================

// TestVisualMode_Entry verifies pressing 'v' in normal mode enters pending state for text objects
// With the v pending operator, 'v' now enters pending state first (for viw/vaw text object support).
// Direct visual mode entry happens when 'v' is followed by a non-text-object key.
func TestVisualMode_Entry(t *testing.T) {
	m := newTestModelWithContent("hello world", "second line")
	m.mode = ModeNormal
	m.cursorRow = 0
	m.cursorCol = 6 // cursor on 'w'

	// Get and execute the 'v' command from registry - now it's StartPendingCommand
	cmd, ok := DefaultRegistry.Get(ModeNormal, "v")
	assert.True(t, ok, "DefaultRegistry should have 'v' command for Normal mode")

	result := cmd.Execute(m)

	// 'v' now enters pending state (for text object support like viw/vaw)
	assert.Equal(t, Executed, result)
	assert.Equal(t, 'v', m.pendingBuilder.Operator())
	assert.Equal(t, ModeNormal, m.mode) // Still in Normal mode, pending state
	assert.False(t, cmd.IsModeChange()) // StartPendingCommand doesn't change mode
}

// TestVisualLineMode_Entry verifies pressing 'V' in normal mode enters ModeVisualLine with anchor.Col=0
func TestVisualLineMode_Entry(t *testing.T) {
	m := newTestModelWithContent("hello world", "second line")
	m.mode = ModeNormal
	m.cursorRow = 1
	m.cursorCol = 3 // cursor on 'o' in "second"

	// Get and execute the 'V' command from registry
	cmd, ok := DefaultRegistry.Get(ModeNormal, "V")
	assert.True(t, ok, "DefaultRegistry should have 'V' command for Normal mode")

	result := cmd.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, ModeVisualLine, m.mode)
	// Line-wise mode should set anchor.Col to 0 regardless of cursor position
	assert.Equal(t, Position{Row: 1, Col: 0}, m.visualAnchor)
	assert.True(t, cmd.IsModeChange())
}

// TestDefaultRegistry_HasVisualModeEntryCommands verifies DefaultRegistry has visual mode entry commands
func TestDefaultRegistry_HasVisualModeEntryCommands(t *testing.T) {
	// 'v' is now a pending operator (for text object support like viw/vaw)
	// It enters pending state first, then falls back to visual mode for non-text-object keys
	cmdV, okV := DefaultRegistry.Get(ModeNormal, "v")
	assert.True(t, okV, "DefaultRegistry should have 'v' command for Normal mode")
	assert.NotNil(t, cmdV)
	assert.Equal(t, "pending.v", cmdV.ID())

	// 'V' for line-wise visual mode
	cmdShiftV, okShiftV := DefaultRegistry.Get(ModeNormal, "V")
	assert.True(t, okShiftV, "DefaultRegistry should have 'V' command for Normal mode")
	assert.NotNil(t, cmdShiftV)
	assert.Equal(t, "mode.visual_line", cmdShiftV.ID())
}

// ============================================================================
// Visual Mode Escape Integration Tests
// ============================================================================

// TestVisualMode_EscapeExits verifies full ESC flow from visual mode
func TestVisualMode_EscapeExits(t *testing.T) {
	m := newTestModelWithContent("hello world", "second line")
	m.mode = ModeNormal
	m.cursorRow = 0
	m.cursorCol = 6

	// Enter visual mode directly (v is now a pending operator for text object support)
	enterVisualMode(m)
	assert.Equal(t, ModeVisual, m.mode)
	assert.Equal(t, Position{Row: 0, Col: 6}, m.visualAnchor)

	// Now press ESC to exit
	cmdEsc, ok := DefaultRegistry.Get(ModeVisual, "<escape>")
	assert.True(t, ok, "DefaultRegistry should have '<escape>' command for Visual mode")

	result := cmdEsc.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, ModeNormal, m.mode)
	assert.Equal(t, Position{}, m.visualAnchor) // Anchor should be cleared
}

// TestVisualLineMode_EscapeExits verifies full ESC flow from visual line mode
func TestVisualLineMode_EscapeExits(t *testing.T) {
	m := newTestModelWithContent("hello world", "second line")
	m.mode = ModeNormal
	m.cursorRow = 1
	m.cursorCol = 3

	// Enter visual line mode first
	cmdV, ok := DefaultRegistry.Get(ModeNormal, "V")
	assert.True(t, ok)
	cmdV.Execute(m)
	assert.Equal(t, ModeVisualLine, m.mode)

	// Now press ESC to exit
	cmdEsc, ok := DefaultRegistry.Get(ModeVisualLine, "<escape>")
	assert.True(t, ok, "DefaultRegistry should have '<escape>' command for VisualLine mode")

	result := cmdEsc.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, ModeNormal, m.mode)
	assert.Equal(t, Position{}, m.visualAnchor)
}

// ============================================================================
// Visual Mode Toggle Integration Tests
// ============================================================================

// TestVisualMode_ToggleBehavior verifies v toggles off, V switches type
func TestVisualMode_ToggleBehavior(t *testing.T) {
	m := newTestModelWithContent("hello world", "second line")
	m.mode = ModeNormal
	m.cursorRow = 0
	m.cursorCol = 6

	// Enter visual mode directly (v is now a pending operator for text object support)
	enterVisualMode(m)
	assert.Equal(t, ModeVisual, m.mode)

	// Press 'v' again - should toggle off (return to Normal)
	cmdVVisual, ok := DefaultRegistry.Get(ModeVisual, "v")
	assert.True(t, ok, "DefaultRegistry should have 'v' command for Visual mode")
	result := cmdVVisual.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, ModeNormal, m.mode)
	assert.Equal(t, Position{}, m.visualAnchor)
}

// TestVisualMode_SwitchToVisualLine verifies 'V' in ModeVisual switches to VisualLine
func TestVisualMode_SwitchToVisualLine(t *testing.T) {
	m := newTestModelWithContent("hello world", "second line")
	m.mode = ModeNormal
	m.cursorRow = 0
	m.cursorCol = 6

	// Enter visual mode directly (v is now a pending operator for text object support)
	enterVisualMode(m)
	assert.Equal(t, ModeVisual, m.mode)
	assert.Equal(t, Position{Row: 0, Col: 6}, m.visualAnchor)

	// Press 'V' - should switch to VisualLine, anchor.Col becomes 0
	cmdShiftV, ok := DefaultRegistry.Get(ModeVisual, "V")
	assert.True(t, ok, "DefaultRegistry should have 'V' command for Visual mode")
	result := cmdShiftV.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, ModeVisualLine, m.mode)
	// Row preserved, Col should be 0 for line-wise
	assert.Equal(t, Position{Row: 0, Col: 0}, m.visualAnchor)
}

// TestVisualLineMode_SwitchToVisual verifies 'v' in ModeVisualLine switches to Visual
func TestVisualLineMode_SwitchToVisual(t *testing.T) {
	m := newTestModelWithContent("hello world", "second line")
	m.mode = ModeNormal
	m.cursorRow = 1
	m.cursorCol = 3

	// Enter visual line mode
	cmdShiftVNormal, _ := DefaultRegistry.Get(ModeNormal, "V")
	cmdShiftVNormal.Execute(m)
	assert.Equal(t, ModeVisualLine, m.mode)
	assert.Equal(t, Position{Row: 1, Col: 0}, m.visualAnchor)

	// Press 'v' - should switch to Visual mode
	cmdV, ok := DefaultRegistry.Get(ModeVisualLine, "v")
	assert.True(t, ok, "DefaultRegistry should have 'v' command for VisualLine mode")
	result := cmdV.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, ModeVisual, m.mode)
	// Anchor should be preserved
	assert.Equal(t, Position{Row: 1, Col: 0}, m.visualAnchor)
}

// TestVisualLineMode_ToggleBehavior verifies 'V' toggles off in ModeVisualLine
func TestVisualLineMode_ToggleBehavior(t *testing.T) {
	m := newTestModelWithContent("hello world", "second line")
	m.mode = ModeNormal
	m.cursorRow = 1
	m.cursorCol = 3

	// Enter visual line mode
	cmdShiftVNormal, _ := DefaultRegistry.Get(ModeNormal, "V")
	cmdShiftVNormal.Execute(m)
	assert.Equal(t, ModeVisualLine, m.mode)

	// Press 'V' again - should toggle off (return to Normal)
	cmdShiftVVisualLine, ok := DefaultRegistry.Get(ModeVisualLine, "V")
	assert.True(t, ok, "DefaultRegistry should have 'V' command for VisualLine mode")
	result := cmdShiftVVisualLine.Execute(m)

	assert.Equal(t, Executed, result)
	assert.Equal(t, ModeNormal, m.mode)
	assert.Equal(t, Position{}, m.visualAnchor)
}

// TestDefaultRegistry_HasVisualModeToggleCommands verifies DefaultRegistry has all visual mode toggle commands
func TestDefaultRegistry_HasVisualModeToggleCommands(t *testing.T) {
	// 'v' in ModeVisual -> toggle off
	cmdV, okV := DefaultRegistry.Get(ModeVisual, "v")
	assert.True(t, okV, "DefaultRegistry should have 'v' command for Visual mode")
	assert.NotNil(t, cmdV)
	assert.Equal(t, "mode.visual_toggle_v", cmdV.ID())

	// 'V' in ModeVisual -> switch to VisualLine
	cmdShiftV, okShiftV := DefaultRegistry.Get(ModeVisual, "V")
	assert.True(t, okShiftV, "DefaultRegistry should have 'V' command for Visual mode")
	assert.NotNil(t, cmdShiftV)
	assert.Equal(t, "mode.visual_toggle_shift_v", cmdShiftV.ID())

	// 'v' in ModeVisualLine -> switch to Visual
	cmdVLine, okVLine := DefaultRegistry.Get(ModeVisualLine, "v")
	assert.True(t, okVLine, "DefaultRegistry should have 'v' command for VisualLine mode")
	assert.NotNil(t, cmdVLine)
	assert.Equal(t, "mode.visual_line_toggle_v", cmdVLine.ID())

	// 'V' in ModeVisualLine -> toggle off
	cmdShiftVLine, okShiftVLine := DefaultRegistry.Get(ModeVisualLine, "V")
	assert.True(t, okShiftVLine, "DefaultRegistry should have 'V' command for VisualLine mode")
	assert.NotNil(t, cmdShiftVLine)
	assert.Equal(t, "mode.visual_line_toggle_shift_v", cmdShiftVLine.ID())

	// ESC in ModeVisual -> exit
	cmdEscVisual, okEscVisual := DefaultRegistry.Get(ModeVisual, "<escape>")
	assert.True(t, okEscVisual, "DefaultRegistry should have '<escape>' command for Visual mode")
	assert.NotNil(t, cmdEscVisual)
	assert.Equal(t, "mode.visual_escape", cmdEscVisual.ID())

	// ESC in ModeVisualLine -> exit
	cmdEscVisualLine, okEscVisualLine := DefaultRegistry.Get(ModeVisualLine, "<escape>")
	assert.True(t, okEscVisualLine, "DefaultRegistry should have '<escape>' command for VisualLine mode")
	assert.NotNil(t, cmdEscVisualLine)
	assert.Equal(t, "mode.visual_escape", cmdEscVisualLine.ID())
}
