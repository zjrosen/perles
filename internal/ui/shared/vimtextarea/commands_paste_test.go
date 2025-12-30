package vimtextarea

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// ============================================================================
// PasteAfterCommand Tests (p)
// ============================================================================

// TestPasteAfterCommand_CharacterwiseInsert tests p with character-wise yank inserts after cursor
func TestPasteAfterCommand_CharacterwiseInsert(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 4 // At 'o' in "hello"
	m.lastYankedText = "XX"
	m.lastYankWasLinewise = false

	cmd := &PasteAfterCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "helloXX world", m.content[0])
	// Cursor should be on last pasted character
	require.Equal(t, 6, m.cursorCol) // 'X' at position 6 (last char of "XX")
}

// TestPasteAfterCommand_LinewiseInsert tests p with line-wise yank inserts new line below
func TestPasteAfterCommand_LinewiseInsert(t *testing.T) {
	m := newTestModelWithContent("line1", "line2")
	m.cursorRow = 0
	m.cursorCol = 2
	m.lastYankedText = "new line"
	m.lastYankWasLinewise = true

	cmd := &PasteAfterCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, 3, len(m.content))
	require.Equal(t, "line1", m.content[0])
	require.Equal(t, "new line", m.content[1])
	require.Equal(t, "line2", m.content[2])
	// Cursor should be on first non-blank of new line
	require.Equal(t, 1, m.cursorRow)
	require.Equal(t, 0, m.cursorCol)
}

// TestPasteAfterCommand_EmptyRegister tests p with empty register returns Skipped
func TestPasteAfterCommand_EmptyRegister(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.lastYankedText = ""
	m.lastYankWasLinewise = false

	cmd := &PasteAfterCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Skipped, result)
	require.Equal(t, "hello", m.content[0])
}

// TestPasteAfterCommand_AtEndOfBuffer tests p at end of buffer (line-wise)
func TestPasteAfterCommand_AtEndOfBuffer(t *testing.T) {
	m := newTestModelWithContent("only line")
	m.cursorRow = 0
	m.lastYankedText = "new line"
	m.lastYankWasLinewise = true

	cmd := &PasteAfterCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, 2, len(m.content))
	require.Equal(t, "only line", m.content[0])
	require.Equal(t, "new line", m.content[1])
	require.Equal(t, 1, m.cursorRow)
}

// TestPasteAfterCommand_MultiLineCharacterwise tests multi-line character-wise paste
func TestPasteAfterCommand_MultiLineCharacterwise(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 4 // At 'o' in "hello"
	m.lastYankedText = "foo\nbar"
	m.lastYankWasLinewise = false

	cmd := &PasteAfterCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, 2, len(m.content))
	require.Equal(t, "hellofoo", m.content[0])
	require.Equal(t, "bar world", m.content[1])
}

// TestPasteAfterCommand_Undo tests undo removes pasted text
func TestPasteAfterCommand_Undo(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 4 // At 'o' in "hello"
	m.lastYankedText = "XX"
	m.lastYankWasLinewise = false

	cmd := &PasteAfterCommand{}
	cmd.Execute(m)

	// Verify pasted
	require.Equal(t, "helloXX world", m.content[0])

	// Undo
	err := cmd.Undo(m)
	require.NoError(t, err)
	require.Equal(t, "hello world", m.content[0])
	require.Equal(t, 4, m.cursorCol) // Cursor restored
}

// TestPasteAfterCommand_UndoLinewise tests undo removes pasted lines
func TestPasteAfterCommand_UndoLinewise(t *testing.T) {
	m := newTestModelWithContent("line1", "line2")
	m.cursorRow = 0
	m.lastYankedText = "new line"
	m.lastYankWasLinewise = true

	cmd := &PasteAfterCommand{}
	cmd.Execute(m)

	require.Equal(t, 3, len(m.content))

	// Undo
	err := cmd.Undo(m)
	require.NoError(t, err)
	require.Equal(t, 2, len(m.content))
	require.Equal(t, "line1", m.content[0])
	require.Equal(t, "line2", m.content[1])
}

// TestPasteAfterCommand_Keys tests command keys
func TestPasteAfterCommand_Keys(t *testing.T) {
	cmd := &PasteAfterCommand{}
	require.Equal(t, []string{"p"}, cmd.Keys())
}

// TestPasteAfterCommand_Mode tests command mode
func TestPasteAfterCommand_Mode(t *testing.T) {
	cmd := &PasteAfterCommand{}
	require.Equal(t, ModeNormal, cmd.Mode())
}

// TestPasteAfterCommand_ID tests command ID
func TestPasteAfterCommand_ID(t *testing.T) {
	cmd := &PasteAfterCommand{}
	require.Equal(t, "paste.after", cmd.ID())
}

// TestPasteAfterCommand_IsUndoable tests paste is undoable
func TestPasteAfterCommand_IsUndoable(t *testing.T) {
	cmd := &PasteAfterCommand{}
	require.True(t, cmd.IsUndoable())
}

// TestPasteAfterCommand_ChangesContent tests paste changes content
func TestPasteAfterCommand_ChangesContent(t *testing.T) {
	cmd := &PasteAfterCommand{}
	require.True(t, cmd.ChangesContent())
}

// ============================================================================
// PasteBeforeCommand Tests (P)
// ============================================================================

// TestPasteBeforeCommand_CharacterwiseInsert tests P with character-wise yank inserts before cursor
func TestPasteBeforeCommand_CharacterwiseInsert(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 5 // At ' ' (space)
	m.lastYankedText = "XX"
	m.lastYankWasLinewise = false

	cmd := &PasteBeforeCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "helloXX world", m.content[0])
	// Cursor should be on last pasted character
	require.Equal(t, 6, m.cursorCol)
}

// TestPasteBeforeCommand_LinewiseInsert tests P with line-wise yank inserts new line above
func TestPasteBeforeCommand_LinewiseInsert(t *testing.T) {
	m := newTestModelWithContent("line1", "line2")
	m.cursorRow = 1
	m.cursorCol = 2
	m.lastYankedText = "new line"
	m.lastYankWasLinewise = true

	cmd := &PasteBeforeCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, 3, len(m.content))
	require.Equal(t, "line1", m.content[0])
	require.Equal(t, "new line", m.content[1])
	require.Equal(t, "line2", m.content[2])
	// Cursor should be on first non-blank of new line
	require.Equal(t, 1, m.cursorRow)
	require.Equal(t, 0, m.cursorCol)
}

// TestPasteBeforeCommand_EmptyRegister tests P with empty register returns Skipped
func TestPasteBeforeCommand_EmptyRegister(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.lastYankedText = ""
	m.lastYankWasLinewise = false

	cmd := &PasteBeforeCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Skipped, result)
	require.Equal(t, "hello", m.content[0])
}

// TestPasteBeforeCommand_AtBeginningOfBuffer tests P at beginning of buffer (line-wise)
func TestPasteBeforeCommand_AtBeginningOfBuffer(t *testing.T) {
	m := newTestModelWithContent("only line")
	m.cursorRow = 0
	m.lastYankedText = "new line"
	m.lastYankWasLinewise = true

	cmd := &PasteBeforeCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, 2, len(m.content))
	require.Equal(t, "new line", m.content[0])
	require.Equal(t, "only line", m.content[1])
	require.Equal(t, 0, m.cursorRow)
}

// TestPasteBeforeCommand_MultiLineCharacterwise tests multi-line character-wise paste
func TestPasteBeforeCommand_MultiLineCharacterwise(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 5 // At ' ' (space)
	m.lastYankedText = "foo\nbar"
	m.lastYankWasLinewise = false

	cmd := &PasteBeforeCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, 2, len(m.content))
	require.Equal(t, "hellofoo", m.content[0])
	require.Equal(t, "bar world", m.content[1])
}

// TestPasteBeforeCommand_Undo tests undo removes pasted text
func TestPasteBeforeCommand_Undo(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 5 // At ' ' (space)
	m.lastYankedText = "XX"
	m.lastYankWasLinewise = false

	cmd := &PasteBeforeCommand{}
	cmd.Execute(m)

	// Verify pasted
	require.Equal(t, "helloXX world", m.content[0])

	// Undo
	err := cmd.Undo(m)
	require.NoError(t, err)
	require.Equal(t, "hello world", m.content[0])
	require.Equal(t, 5, m.cursorCol) // Cursor restored
}

// TestPasteBeforeCommand_UndoLinewise tests undo removes pasted lines
func TestPasteBeforeCommand_UndoLinewise(t *testing.T) {
	m := newTestModelWithContent("line1", "line2")
	m.cursorRow = 1
	m.lastYankedText = "new line"
	m.lastYankWasLinewise = true

	cmd := &PasteBeforeCommand{}
	cmd.Execute(m)

	require.Equal(t, 3, len(m.content))

	// Undo
	err := cmd.Undo(m)
	require.NoError(t, err)
	require.Equal(t, 2, len(m.content))
	require.Equal(t, "line1", m.content[0])
	require.Equal(t, "line2", m.content[1])
}

// TestPasteBeforeCommand_Keys tests command keys
func TestPasteBeforeCommand_Keys(t *testing.T) {
	cmd := &PasteBeforeCommand{}
	require.Equal(t, []string{"P"}, cmd.Keys())
}

// TestPasteBeforeCommand_Mode tests command mode
func TestPasteBeforeCommand_Mode(t *testing.T) {
	cmd := &PasteBeforeCommand{}
	require.Equal(t, ModeNormal, cmd.Mode())
}

// TestPasteBeforeCommand_ID tests command ID
func TestPasteBeforeCommand_ID(t *testing.T) {
	cmd := &PasteBeforeCommand{}
	require.Equal(t, "paste.before", cmd.ID())
}

// ============================================================================
// Integration Tests
// ============================================================================

// TestYankYankPasteDuplicatesLine tests yy + p exactly duplicates current line
func TestYankYankPasteDuplicatesLine(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorRow = 0
	m.cursorCol = 3

	// yy - yank line
	yankCmd := &YankLineCommand{}
	yankCmd.Execute(m)

	require.Equal(t, "hello world", m.lastYankedText)
	require.True(t, m.lastYankWasLinewise)

	// p - paste after
	pasteCmd := &PasteAfterCommand{}
	pasteCmd.Execute(m)

	require.Equal(t, 2, len(m.content))
	require.Equal(t, "hello world", m.content[0])
	require.Equal(t, "hello world", m.content[1])
}

// TestDeletePasteRestoresCharacter tests x + P restores original character
func TestDeletePasteRestoresCharacter(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 2 // At 'l'

	// x - delete character (also yanks)
	deleteCmd := &DeleteCharCommand{}
	deleteCmd.Execute(m)

	require.Equal(t, "helo", m.content[0])
	require.Equal(t, "l", m.lastYankedText)
	require.False(t, m.lastYankWasLinewise)

	// P - paste before (restores)
	pasteCmd := &PasteBeforeCommand{}
	pasteCmd.Execute(m)

	require.Equal(t, "hello", m.content[0])
}

// TestVisualLineYankPastesOnNewLine tests visual line yank followed by p pastes on new line
func TestVisualLineYankPastesOnNewLine(t *testing.T) {
	m := newTestModelWithContent("line1", "line2", "line3")
	m.mode = ModeVisualLine
	m.visualAnchor = Position{Row: 0, Col: 0}
	m.cursorRow = 0
	m.cursorCol = 2

	// Visual line yank - note we set lastYankWasLinewise manually since
	// VisualYankCommand execution requires mode to be set
	m.lastYankedText = "line1"
	m.lastYankWasLinewise = true

	// Back to normal mode
	m.mode = ModeNormal
	m.cursorRow = 1 // Move to line2

	// p - paste after
	pasteCmd := &PasteAfterCommand{}
	pasteCmd.Execute(m)

	require.Equal(t, 4, len(m.content))
	require.Equal(t, "line1", m.content[0])
	require.Equal(t, "line2", m.content[1])
	require.Equal(t, "line1", m.content[2]) // Pasted below line2
	require.Equal(t, "line3", m.content[3])
}

// ============================================================================
// Registry Tests
// ============================================================================

// TestDefaultRegistry_HasPasteCommands verifies paste commands are registered
func TestDefaultRegistry_HasPasteCommands(t *testing.T) {
	// p should be registered
	cmd, ok := DefaultRegistry.Get(ModeNormal, "p")
	require.True(t, ok, "p command should be registered")
	require.Equal(t, "paste.after", cmd.ID())

	// P should be registered
	cmd, ok = DefaultRegistry.Get(ModeNormal, "P")
	require.True(t, ok, "P command should be registered")
	require.Equal(t, "paste.before", cmd.ID())
}

// ============================================================================
// Edge Case Tests
// ============================================================================

// TestPasteAfterCommand_EmptyLine tests p on empty line
func TestPasteAfterCommand_EmptyLine(t *testing.T) {
	m := newTestModelWithContent("")
	m.lastYankedText = "text"
	m.lastYankWasLinewise = false

	cmd := &PasteAfterCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "text", m.content[0])
}

// TestPasteBeforeCommand_EmptyLine tests P on empty line
func TestPasteBeforeCommand_EmptyLine(t *testing.T) {
	m := newTestModelWithContent("")
	m.lastYankedText = "text"
	m.lastYankWasLinewise = false

	cmd := &PasteBeforeCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "text", m.content[0])
}

// TestPasteAfterCommand_AtEndOfLine tests p at end of line
func TestPasteAfterCommand_AtEndOfLine(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 4 // At last 'o'
	m.lastYankedText = "XX"
	m.lastYankWasLinewise = false

	cmd := &PasteAfterCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "helloXX", m.content[0])
}

// TestPasteBeforeCommand_AtStartOfLine tests P at start of line
func TestPasteBeforeCommand_AtStartOfLine(t *testing.T) {
	m := newTestModelWithContent("hello")
	m.cursorCol = 0
	m.lastYankedText = "XX"
	m.lastYankWasLinewise = false

	cmd := &PasteBeforeCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "XXhello", m.content[0])
}

// TestPasteAfterCommand_LinewiseWithIndentation tests p preserves indentation for first non-blank
func TestPasteAfterCommand_LinewiseWithIndentation(t *testing.T) {
	m := newTestModelWithContent("line1")
	m.cursorRow = 0
	m.lastYankedText = "    indented"
	m.lastYankWasLinewise = true

	cmd := &PasteAfterCommand{}
	cmd.Execute(m)

	require.Equal(t, 2, len(m.content))
	require.Equal(t, "    indented", m.content[1])
	// Cursor should be at first non-blank (position 4)
	require.Equal(t, 1, m.cursorRow)
	require.Equal(t, 4, m.cursorCol)
}

// TestPasteAfterCommand_MultiLineLinewise tests linewise paste with multiple lines
func TestPasteAfterCommand_MultiLineLinewise(t *testing.T) {
	m := newTestModelWithContent("original")
	m.cursorRow = 0
	m.lastYankedText = "line1\nline2\nline3"
	m.lastYankWasLinewise = true

	cmd := &PasteAfterCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, 4, len(m.content))
	require.Equal(t, "original", m.content[0])
	require.Equal(t, "line1", m.content[1])
	require.Equal(t, "line2", m.content[2])
	require.Equal(t, "line3", m.content[3])
}

// TestPasteBeforeCommand_MultiLineLinewise tests linewise paste before with multiple lines
func TestPasteBeforeCommand_MultiLineLinewise(t *testing.T) {
	m := newTestModelWithContent("original")
	m.cursorRow = 0
	m.lastYankedText = "line1\nline2\nline3"
	m.lastYankWasLinewise = true

	cmd := &PasteBeforeCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, 4, len(m.content))
	require.Equal(t, "line1", m.content[0])
	require.Equal(t, "line2", m.content[1])
	require.Equal(t, "line3", m.content[2])
	require.Equal(t, "original", m.content[3])
}

// TestFindFirstNonBlank tests the helper function
func TestFindFirstNonBlank(t *testing.T) {
	tests := []struct {
		line     string
		expected int
	}{
		{"hello", 0},
		{"  hello", 2},
		{"\thello", 1},
		{"  \thello", 3},
		{"", 0},
		{"   ", 0},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			result := findFirstNonBlank(tt.line)
			require.Equal(t, tt.expected, result)
		})
	}
}
