package vimtextarea

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// ============================================================================
// DeleteTextObjectCommand Tests (diw, daw)
// ============================================================================

func TestDeleteTextObjectCommand_InnerWord_DeletesWordWithoutWhitespace(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 2 // cursor in "hello"

	cmd := &DeleteTextObjectCommand{object: 'w', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, " world", m.content[0])
	require.Equal(t, 0, m.cursorCol) // cursor at start of deleted region
}

func TestDeleteTextObjectCommand_AroundWord_DeletesWordWithTrailingWhitespace(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 2 // cursor in "hello"

	cmd := &DeleteTextObjectCommand{object: 'w', inner: false}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "world", m.content[0])
	require.Equal(t, 0, m.cursorCol)
}

func TestDeleteTextObjectCommand_CursorAtWordStart(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 0 // cursor at start of "hello"

	cmd := &DeleteTextObjectCommand{object: 'w', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, " world", m.content[0])
}

func TestDeleteTextObjectCommand_CursorAtWordMiddle(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 2 // cursor in middle of "hello"

	cmd := &DeleteTextObjectCommand{object: 'w', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, " world", m.content[0])
}

func TestDeleteTextObjectCommand_CursorAtWordEnd(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 4 // cursor at end of "hello"

	cmd := &DeleteTextObjectCommand{object: 'w', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, " world", m.content[0])
}

func TestDeleteTextObjectCommand_PunctuationAsSeparateWord(t *testing.T) {
	m := newTestModelWithContent("foo.bar")
	m.cursorCol = 3 // cursor at '.'

	cmd := &DeleteTextObjectCommand{object: 'w', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "foobar", m.content[0])
}

func TestDeleteTextObjectCommand_EmptyLineIsNoOp(t *testing.T) {
	m := newTestModelWithContent("")
	m.cursorCol = 0

	cmd := &DeleteTextObjectCommand{object: 'w', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Skipped, result)
	require.Equal(t, "", m.content[0])
}

func TestDeleteTextObjectCommand_CursorOnWhitespaceIsNoOp(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 5 // cursor on space

	cmd := &DeleteTextObjectCommand{object: 'w', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Skipped, result)
	require.Equal(t, "hello world", m.content[0])
}

func TestDeleteTextObjectCommand_Undo_RestoresContent(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 2

	cmd := &DeleteTextObjectCommand{object: 'w', inner: true}
	cmd.Execute(m)
	require.Equal(t, " world", m.content[0])

	err := cmd.Undo(m)
	require.NoError(t, err)
	require.Equal(t, "hello world", m.content[0])
	require.Equal(t, 2, m.cursorCol) // cursor restored
}

func TestDeleteTextObjectCommand_Undo_RestoresCursorPosition(t *testing.T) {
	m := newTestModelWithContent("one two three")
	m.cursorCol = 5 // cursor at 'w' in "two"

	cmd := &DeleteTextObjectCommand{object: 'w', inner: false}
	cmd.Execute(m)

	err := cmd.Undo(m)
	require.NoError(t, err)
	require.Equal(t, 0, m.cursorRow)
	require.Equal(t, 5, m.cursorCol) // original cursor position restored
}

func TestDeleteTextObjectCommand_Redo_RestoresExecutedState(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 2

	cmd := &DeleteTextObjectCommand{object: 'w', inner: true}
	cmd.Execute(m)
	executedContent := m.content[0]
	executedCursorCol := m.cursorCol

	// Undo
	cmd.Undo(m)
	require.Equal(t, "hello world", m.content[0])

	// Redo (re-execute)
	cmd.Execute(m)
	require.Equal(t, executedContent, m.content[0])
	require.Equal(t, executedCursorCol, m.cursorCol)
}

func TestDeleteTextObjectCommand_YankRegister_Populated(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 2

	cmd := &DeleteTextObjectCommand{object: 'w', inner: true}
	cmd.Execute(m)

	require.Equal(t, "hello", m.lastYankedText)
	require.False(t, m.lastYankWasLinewise)
}

func TestDeleteTextObjectCommand_Keys(t *testing.T) {
	cmd := &DeleteTextObjectCommand{object: 'w', inner: true}
	require.Equal(t, []string{"diw"}, cmd.Keys())

	cmd2 := &DeleteTextObjectCommand{object: 'w', inner: false}
	require.Equal(t, []string{"daw"}, cmd2.Keys())
}

func TestDeleteTextObjectCommand_ID(t *testing.T) {
	cmd := &DeleteTextObjectCommand{object: 'w', inner: true}
	require.Equal(t, "delete.textobject.inner_w", cmd.ID())

	cmd2 := &DeleteTextObjectCommand{object: 'w', inner: false}
	require.Equal(t, "delete.textobject.around_w", cmd2.ID())
}

func TestDeleteTextObjectCommand_Mode(t *testing.T) {
	cmd := &DeleteTextObjectCommand{object: 'w', inner: true}
	require.Equal(t, ModeNormal, cmd.Mode())
}

func TestDeleteTextObjectCommand_BaseProperties(t *testing.T) {
	cmd := &DeleteTextObjectCommand{object: 'w', inner: true}
	require.True(t, cmd.IsUndoable())
	require.True(t, cmd.ChangesContent())
	require.False(t, cmd.IsModeChange())
}

// ============================================================================
// ChangeTextObjectCommand Tests (ciw, caw)
// ============================================================================

func TestChangeTextObjectCommand_InnerWord_DeletesWordAndEntersInsertMode(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 2 // cursor in "hello"

	cmd := &ChangeTextObjectCommand{object: 'w', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, " world", m.content[0])
	require.Equal(t, ModeInsert, m.mode)
	require.Equal(t, 0, m.cursorCol) // cursor at deletion point
}

func TestChangeTextObjectCommand_AroundWord_DeletesWordWithWhitespaceAndEntersInsertMode(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 2 // cursor in "hello"

	cmd := &ChangeTextObjectCommand{object: 'w', inner: false}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "world", m.content[0])
	require.Equal(t, ModeInsert, m.mode)
	require.Equal(t, 0, m.cursorCol)
}

func TestChangeTextObjectCommand_AroundWordAtLineEnd_IncludesLeadingWhitespace(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 7 // cursor in "world"

	cmd := &ChangeTextObjectCommand{object: 'w', inner: false}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	// "world" is at end, so aw includes leading whitespace
	require.Equal(t, "hello", m.content[0])
	require.Equal(t, ModeInsert, m.mode)
}

func TestChangeTextObjectCommand_CursorAtWordStart(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 0 // cursor at start of "hello"

	cmd := &ChangeTextObjectCommand{object: 'w', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, " world", m.content[0])
	require.Equal(t, ModeInsert, m.mode)
}

func TestChangeTextObjectCommand_CursorAtWordMiddle(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 2 // cursor in middle of "hello"

	cmd := &ChangeTextObjectCommand{object: 'w', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, " world", m.content[0])
	require.Equal(t, ModeInsert, m.mode)
}

func TestChangeTextObjectCommand_CursorAtWordEnd(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 4 // cursor at end of "hello"

	cmd := &ChangeTextObjectCommand{object: 'w', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, " world", m.content[0])
	require.Equal(t, ModeInsert, m.mode)
}

func TestChangeTextObjectCommand_PunctuationAsSeparateWord(t *testing.T) {
	m := newTestModelWithContent("foo.bar")
	m.cursorCol = 3 // cursor at '.'

	cmd := &ChangeTextObjectCommand{object: 'w', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "foobar", m.content[0])
	require.Equal(t, ModeInsert, m.mode)
}

func TestChangeTextObjectCommand_EmptyLineIsNoOp(t *testing.T) {
	m := newTestModelWithContent("")
	m.cursorCol = 0

	cmd := &ChangeTextObjectCommand{object: 'w', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Skipped, result)
	require.Equal(t, "", m.content[0])
	require.Equal(t, ModeNormal, m.mode) // mode should not change on skip
}

func TestChangeTextObjectCommand_Undo_RestoresContentAndMode(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 2

	cmd := &ChangeTextObjectCommand{object: 'w', inner: true}
	cmd.Execute(m)
	require.Equal(t, " world", m.content[0])
	require.Equal(t, ModeInsert, m.mode)

	err := cmd.Undo(m)
	require.NoError(t, err)
	require.Equal(t, "hello world", m.content[0])
	require.Equal(t, ModeNormal, m.mode) // mode restored
	require.Equal(t, 2, m.cursorCol)     // cursor restored
}

func TestChangeTextObjectCommand_Undo_RestoresCursorPosition(t *testing.T) {
	m := newTestModelWithContent("one two three")
	m.cursorCol = 5 // cursor at 'w' in "two"

	cmd := &ChangeTextObjectCommand{object: 'w', inner: false}
	cmd.Execute(m)

	err := cmd.Undo(m)
	require.NoError(t, err)
	require.Equal(t, 0, m.cursorRow)
	require.Equal(t, 5, m.cursorCol) // original cursor position restored
}

func TestChangeTextObjectCommand_Redo_RestoresExecutedState(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 2

	cmd := &ChangeTextObjectCommand{object: 'w', inner: true}
	cmd.Execute(m)
	executedContent := m.content[0]
	executedCursorCol := m.cursorCol
	executedMode := m.mode

	// Undo
	cmd.Undo(m)
	require.Equal(t, "hello world", m.content[0])
	require.Equal(t, ModeNormal, m.mode)

	// Redo (re-execute)
	cmd.Execute(m)
	require.Equal(t, executedContent, m.content[0])
	require.Equal(t, executedCursorCol, m.cursorCol)
	require.Equal(t, executedMode, m.mode)
}

func TestChangeTextObjectCommand_Keys(t *testing.T) {
	cmd := &ChangeTextObjectCommand{object: 'w', inner: true}
	require.Equal(t, []string{"ciw"}, cmd.Keys())

	cmd2 := &ChangeTextObjectCommand{object: 'w', inner: false}
	require.Equal(t, []string{"caw"}, cmd2.Keys())
}

func TestChangeTextObjectCommand_ID(t *testing.T) {
	cmd := &ChangeTextObjectCommand{object: 'w', inner: true}
	require.Equal(t, "change.textobject.inner_w", cmd.ID())

	cmd2 := &ChangeTextObjectCommand{object: 'w', inner: false}
	require.Equal(t, "change.textobject.around_w", cmd2.ID())
}

func TestChangeTextObjectCommand_Mode(t *testing.T) {
	cmd := &ChangeTextObjectCommand{object: 'w', inner: true}
	require.Equal(t, ModeNormal, cmd.Mode())
}

func TestChangeTextObjectCommand_BaseProperties(t *testing.T) {
	cmd := &ChangeTextObjectCommand{object: 'w', inner: true}
	require.True(t, cmd.IsUndoable())
	require.True(t, cmd.ChangesContent())
	require.True(t, cmd.IsModeChange()) // ChangeBase marks as mode change
}

// ============================================================================
// Integration Tests - Command Registration
// ============================================================================

func TestTextObjectCommands_RegisteredInPendingRegistry(t *testing.T) {
	// Verify diw, daw, ciw, caw are registered
	tests := []struct {
		operator rune
		sequence string
		inner    bool
	}{
		{'d', "iw", true},
		{'d', "aw", false},
		{'c', "iw", true},
		{'c', "aw", false},
	}

	for _, tc := range tests {
		t.Run(string(tc.operator)+tc.sequence, func(t *testing.T) {
			cmd, ok := DefaultPendingRegistry.Get(tc.operator, tc.sequence)
			require.True(t, ok, "command should be registered")
			require.NotNil(t, cmd)
		})
	}
}

// ============================================================================
// Edge Cases
// ============================================================================

func TestDeleteTextObjectCommand_SecondWordOnLine(t *testing.T) {
	m := newTestModelWithContent("one two three")
	m.cursorCol = 5 // cursor at 'w' in "two"

	cmd := &DeleteTextObjectCommand{object: 'w', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "one  three", m.content[0])
}

func TestDeleteTextObjectCommand_AroundSecondWordOnLine(t *testing.T) {
	m := newTestModelWithContent("one two three")
	m.cursorCol = 5 // cursor at 'w' in "two"

	cmd := &DeleteTextObjectCommand{object: 'w', inner: false}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "one three", m.content[0])
}

func TestDeleteTextObjectCommand_UnknownTextObject(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 2

	cmd := &DeleteTextObjectCommand{object: 'X', inner: true} // unknown object
	result := cmd.Execute(m)

	require.Equal(t, Skipped, result)
	require.Equal(t, "hello world", m.content[0]) // unchanged
}

func TestChangeTextObjectCommand_UnknownTextObject(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 2

	cmd := &ChangeTextObjectCommand{object: 'X', inner: true} // unknown object
	result := cmd.Execute(m)

	require.Equal(t, Skipped, result)
	require.Equal(t, "hello world", m.content[0]) // unchanged
	require.Equal(t, ModeNormal, m.mode)          // mode unchanged
}

func TestDeleteTextObjectCommand_SingleCharacterWord(t *testing.T) {
	m := newTestModelWithContent("a b c")
	m.cursorCol = 2 // cursor at 'b'

	cmd := &DeleteTextObjectCommand{object: 'w', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "a  c", m.content[0])
}

func TestChangeTextObjectCommand_SingleCharacterWord(t *testing.T) {
	m := newTestModelWithContent("a b c")
	m.cursorCol = 2 // cursor at 'b'

	cmd := &ChangeTextObjectCommand{object: 'w', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "a  c", m.content[0])
	require.Equal(t, ModeInsert, m.mode)
}

func TestDeleteTextObjectCommand_CursorClamping(t *testing.T) {
	// After deleting, if cursor would be past end of line, it should be clamped
	m := newTestModelWithContent("x")
	m.cursorCol = 0

	cmd := &DeleteTextObjectCommand{object: 'w', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "", m.content[0])
	require.Equal(t, 0, m.cursorCol) // clamped to 0
}

// ============================================================================
// YankTextObjectCommand Tests (yiw, yaw)
// ============================================================================

func TestYankTextObjectCommand_InnerWord_YanksWordToRegisterWithoutModifyingContent(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 2 // cursor in "hello"

	cmd := &YankTextObjectCommand{object: 'w', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "hello world", m.content[0]) // content unchanged
	require.Equal(t, "hello", m.lastYankedText)   // yanked word
	require.False(t, m.lastYankWasLinewise)       // character-wise yank
}

func TestYankTextObjectCommand_AroundWord_YanksWordWithWhitespaceToRegister(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 2 // cursor in "hello"

	cmd := &YankTextObjectCommand{object: 'w', inner: false}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "hello world", m.content[0]) // content unchanged
	require.Equal(t, "hello ", m.lastYankedText)  // yanked word with trailing whitespace
	require.False(t, m.lastYankWasLinewise)
}

func TestYankTextObjectCommand_CursorAtWordStart(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 0 // cursor at start of "hello"

	cmd := &YankTextObjectCommand{object: 'w', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "hello world", m.content[0])
	require.Equal(t, "hello", m.lastYankedText)
}

func TestYankTextObjectCommand_CursorAtWordMiddle(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 2 // cursor in middle of "hello"

	cmd := &YankTextObjectCommand{object: 'w', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "hello world", m.content[0])
	require.Equal(t, "hello", m.lastYankedText)
}

func TestYankTextObjectCommand_CursorAtWordEnd(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 4 // cursor at end of "hello"

	cmd := &YankTextObjectCommand{object: 'w', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "hello world", m.content[0])
	require.Equal(t, "hello", m.lastYankedText)
}

func TestYankTextObjectCommand_EmptyLineIsNoOp(t *testing.T) {
	m := newTestModelWithContent("")
	m.cursorCol = 0
	m.lastYankedText = "previous" // set previous value to verify no change

	cmd := &YankTextObjectCommand{object: 'w', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Skipped, result)
	require.Equal(t, "", m.content[0])
	require.Equal(t, "previous", m.lastYankedText) // register unchanged
}

func TestYankTextObjectCommand_CursorOnWhitespaceIsNoOp(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 5 // cursor on space
	m.lastYankedText = "previous"

	cmd := &YankTextObjectCommand{object: 'w', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Skipped, result)
	require.Equal(t, "hello world", m.content[0])
	require.Equal(t, "previous", m.lastYankedText) // register unchanged
}

func TestYankTextObjectCommand_IsNotUndoable(t *testing.T) {
	cmd := &YankTextObjectCommand{object: 'w', inner: true}
	require.False(t, cmd.IsUndoable())
}

func TestYankTextObjectCommand_DoesNotChangeContent(t *testing.T) {
	cmd := &YankTextObjectCommand{object: 'w', inner: true}
	require.False(t, cmd.ChangesContent())
}

func TestYankTextObjectCommand_IsNotModeChange(t *testing.T) {
	cmd := &YankTextObjectCommand{object: 'w', inner: true}
	require.False(t, cmd.IsModeChange())
}

func TestYankTextObjectCommand_Keys(t *testing.T) {
	cmd := &YankTextObjectCommand{object: 'w', inner: true}
	require.Equal(t, []string{"yiw"}, cmd.Keys())

	cmd2 := &YankTextObjectCommand{object: 'w', inner: false}
	require.Equal(t, []string{"yaw"}, cmd2.Keys())
}

func TestYankTextObjectCommand_ID(t *testing.T) {
	cmd := &YankTextObjectCommand{object: 'w', inner: true}
	require.Equal(t, "yank.textobject.inner_w", cmd.ID())

	cmd2 := &YankTextObjectCommand{object: 'w', inner: false}
	require.Equal(t, "yank.textobject.around_w", cmd2.ID())
}

func TestYankTextObjectCommand_Mode(t *testing.T) {
	cmd := &YankTextObjectCommand{object: 'w', inner: true}
	require.Equal(t, ModeNormal, cmd.Mode())
}

func TestYankTextObjectCommand_LastYankWasLinewise_IsFalse(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 2
	m.lastYankWasLinewise = true // set to true before test

	cmd := &YankTextObjectCommand{object: 'w', inner: true}
	cmd.Execute(m)

	require.False(t, m.lastYankWasLinewise) // should be false for word objects
}

func TestYankTextObjectCommand_SecondWordOnLine(t *testing.T) {
	m := newTestModelWithContent("one two three")
	m.cursorCol = 5 // cursor at 'w' in "two"

	cmd := &YankTextObjectCommand{object: 'w', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "one two three", m.content[0]) // unchanged
	require.Equal(t, "two", m.lastYankedText)
}

func TestYankTextObjectCommand_AroundSecondWordOnLine(t *testing.T) {
	m := newTestModelWithContent("one two three")
	m.cursorCol = 5 // cursor at 'w' in "two"

	cmd := &YankTextObjectCommand{object: 'w', inner: false}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "one two three", m.content[0]) // unchanged
	require.Equal(t, "two ", m.lastYankedText)      // includes trailing whitespace
}

func TestYankTextObjectCommand_YankHighlightRegion_ReturnsCorrectPositions(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 2 // cursor in "hello"

	cmd := &YankTextObjectCommand{object: 'w', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)

	start, end, linewise, show := cmd.YankHighlightRegion()
	require.Equal(t, Position{Row: 0, Col: 0}, start) // start of "hello"
	require.Equal(t, Position{Row: 0, Col: 4}, end)   // end of "hello"
	require.False(t, linewise)                        // word yank is not linewise
	require.True(t, show)                             // should show highlight
}

func TestYankTextObjectCommand_YankHighlightRegion_ShowFalseWhenNoMatch(t *testing.T) {
	m := newTestModelWithContent("")
	m.cursorCol = 0

	cmd := &YankTextObjectCommand{object: 'w', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Skipped, result)

	_, _, _, show := cmd.YankHighlightRegion()
	require.False(t, show) // no highlight when skipped
}

func TestYankTextObjectCommand_YankHighlightRegion_BracketObject(t *testing.T) {
	m := newTestModelWithContent("foo(bar)baz")
	m.cursorCol = 5 // cursor at 'a' in "bar"

	cmd := &YankTextObjectCommand{object: 'b', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)

	start, end, linewise, show := cmd.YankHighlightRegion()
	require.Equal(t, Position{Row: 0, Col: 4}, start) // after '('
	require.Equal(t, Position{Row: 0, Col: 6}, end)   // before ')'
	require.False(t, linewise)
	require.True(t, show)
}

func TestYankTextObjectCommand_YankHighlightRegion_AroundBracket(t *testing.T) {
	m := newTestModelWithContent("foo(bar)baz")
	m.cursorCol = 5 // cursor at 'a' in "bar"

	cmd := &YankTextObjectCommand{object: 'b', inner: false}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)

	start, end, linewise, show := cmd.YankHighlightRegion()
	require.Equal(t, Position{Row: 0, Col: 3}, start) // at '('
	require.Equal(t, Position{Row: 0, Col: 7}, end)   // at ')'
	require.False(t, linewise)
	require.True(t, show)
}

func TestYankTextObjectCommand_YankHighlightRegion_QuoteObject(t *testing.T) {
	m := newTestModelWithContent(`say "hello" now`)
	m.cursorCol = 6 // cursor at 'l' inside quotes

	cmd := &YankTextObjectCommand{object: '"', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)

	start, end, linewise, show := cmd.YankHighlightRegion()
	require.Equal(t, Position{Row: 0, Col: 5}, start) // after opening "
	require.Equal(t, Position{Row: 0, Col: 9}, end)   // before closing "
	require.False(t, linewise)
	require.True(t, show)
}

func TestYankTextObjectCommand_ImplementsYankHighlighter(t *testing.T) {
	// This test verifies that YankTextObjectCommand implements the YankHighlighter interface
	var cmd interface{} = &YankTextObjectCommand{object: 'w', inner: true}
	_, ok := cmd.(YankHighlighter)
	require.True(t, ok, "YankTextObjectCommand should implement YankHighlighter interface")
}

// ============================================================================
// VisualSelectTextObjectCommand Tests (viw, vaw)
// ============================================================================

func TestVisualSelectTextObjectCommand_InnerWord_EntersVisualModeWithWordSelected(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 2 // cursor in "hello"

	cmd := &VisualSelectTextObjectCommand{object: 'w', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, ModeVisual, m.mode)
	require.Equal(t, Position{Row: 0, Col: 0}, m.visualAnchor) // anchor at word start
	require.Equal(t, 0, m.cursorRow)
	require.Equal(t, 4, m.cursorCol) // cursor at word end (inclusive)
}

func TestVisualSelectTextObjectCommand_AroundWord_EntersVisualModeWithWordAndWhitespaceSelected(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 2 // cursor in "hello"

	cmd := &VisualSelectTextObjectCommand{object: 'w', inner: false}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, ModeVisual, m.mode)
	require.Equal(t, Position{Row: 0, Col: 0}, m.visualAnchor) // anchor at word start
	require.Equal(t, 0, m.cursorRow)
	require.Equal(t, 5, m.cursorCol) // cursor includes trailing space
}

func TestVisualSelectTextObjectCommand_CursorAtWordStart(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 0 // cursor at start of "hello"

	cmd := &VisualSelectTextObjectCommand{object: 'w', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, ModeVisual, m.mode)
	require.Equal(t, Position{Row: 0, Col: 0}, m.visualAnchor)
	require.Equal(t, 4, m.cursorCol) // cursor at word end
}

func TestVisualSelectTextObjectCommand_CursorAtWordMiddle(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 2 // cursor in middle of "hello"

	cmd := &VisualSelectTextObjectCommand{object: 'w', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, ModeVisual, m.mode)
	require.Equal(t, Position{Row: 0, Col: 0}, m.visualAnchor)
	require.Equal(t, 4, m.cursorCol)
}

func TestVisualSelectTextObjectCommand_CursorAtWordEnd(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 4 // cursor at end of "hello"

	cmd := &VisualSelectTextObjectCommand{object: 'w', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, ModeVisual, m.mode)
	require.Equal(t, Position{Row: 0, Col: 0}, m.visualAnchor)
	require.Equal(t, 4, m.cursorCol)
}

func TestVisualSelectTextObjectCommand_EmptyLineIsNoOp(t *testing.T) {
	m := newTestModelWithContent("")
	m.cursorCol = 0

	cmd := &VisualSelectTextObjectCommand{object: 'w', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Skipped, result)
	require.Equal(t, ModeNormal, m.mode) // mode should not change on skip
}

func TestVisualSelectTextObjectCommand_CursorOnWhitespaceIsNoOp(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 5 // cursor on space

	cmd := &VisualSelectTextObjectCommand{object: 'w', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Skipped, result)
	require.Equal(t, ModeNormal, m.mode)
}

func TestVisualSelectTextObjectCommand_VisualAnchorPositionedCorrectly(t *testing.T) {
	m := newTestModelWithContent("one two three")
	m.cursorCol = 5 // cursor at 'w' in "two"

	cmd := &VisualSelectTextObjectCommand{object: 'w', inner: true}
	cmd.Execute(m)

	// Anchor should be at start of "two" (col 4)
	require.Equal(t, Position{Row: 0, Col: 4}, m.visualAnchor)
	// Cursor should be at end of "two" (col 6)
	require.Equal(t, 6, m.cursorCol)
}

func TestVisualSelectTextObjectCommand_FollowedByDeleteDeletesWord(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 2 // cursor in "hello"

	// First, enter visual mode with word selected
	viwCmd := &VisualSelectTextObjectCommand{object: 'w', inner: true}
	viwCmd.Execute(m)

	require.Equal(t, ModeVisual, m.mode)
	require.Equal(t, Position{Row: 0, Col: 0}, m.visualAnchor)
	require.Equal(t, 4, m.cursorCol)

	// Now delete the selection
	deleteCmd := &VisualDeleteCommand{mode: ModeVisual}
	result := deleteCmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, " world", m.content[0]) // "hello" deleted
	require.Equal(t, ModeNormal, m.mode)     // back to normal mode
}

func TestVisualSelectTextObjectCommand_Keys(t *testing.T) {
	cmd := &VisualSelectTextObjectCommand{object: 'w', inner: true}
	require.Equal(t, []string{"viw"}, cmd.Keys())

	cmd2 := &VisualSelectTextObjectCommand{object: 'w', inner: false}
	require.Equal(t, []string{"vaw"}, cmd2.Keys())
}

func TestVisualSelectTextObjectCommand_ID(t *testing.T) {
	cmd := &VisualSelectTextObjectCommand{object: 'w', inner: true}
	require.Equal(t, "visual.textobject.inner_w", cmd.ID())

	cmd2 := &VisualSelectTextObjectCommand{object: 'w', inner: false}
	require.Equal(t, "visual.textobject.around_w", cmd2.ID())
}

func TestVisualSelectTextObjectCommand_Mode(t *testing.T) {
	cmd := &VisualSelectTextObjectCommand{object: 'w', inner: true}
	require.Equal(t, ModeNormal, cmd.Mode())
}

func TestVisualSelectTextObjectCommand_BaseProperties(t *testing.T) {
	cmd := &VisualSelectTextObjectCommand{object: 'w', inner: true}
	require.False(t, cmd.IsUndoable())
	require.False(t, cmd.ChangesContent())
	require.True(t, cmd.IsModeChange()) // ModeEntryBase marks as mode change
}

func TestVisualSelectTextObjectCommand_ClearsPendingBuilder(t *testing.T) {
	m := newTestModelWithContent("hello world")
	m.cursorCol = 2
	m.pendingBuilder.operator = 'v'
	m.pendingBuilder.keyBuffer = "iw"

	cmd := &VisualSelectTextObjectCommand{object: 'w', inner: true}
	cmd.Execute(m)

	require.Equal(t, rune(0), m.pendingBuilder.operator)
	require.Equal(t, "", m.pendingBuilder.keyBuffer)
}

func TestVisualSelectTextObjectCommand_SecondWordOnLine(t *testing.T) {
	m := newTestModelWithContent("one two three")
	m.cursorCol = 5 // cursor at 'w' in "two"

	cmd := &VisualSelectTextObjectCommand{object: 'w', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, ModeVisual, m.mode)
	require.Equal(t, Position{Row: 0, Col: 4}, m.visualAnchor) // start of "two"
	require.Equal(t, 6, m.cursorCol)                           // end of "two"
}

func TestVisualSelectTextObjectCommand_AroundSecondWordOnLine(t *testing.T) {
	m := newTestModelWithContent("one two three")
	m.cursorCol = 5 // cursor at 'w' in "two"

	cmd := &VisualSelectTextObjectCommand{object: 'w', inner: false}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, ModeVisual, m.mode)
	require.Equal(t, Position{Row: 0, Col: 4}, m.visualAnchor) // start of "two"
	require.Equal(t, 7, m.cursorCol)                           // includes trailing space
}

// ============================================================================
// Integration Tests - Command Registration for yiw/yaw/viw/vaw
// ============================================================================

func TestYankAndVisualSelectTextObjectCommands_RegisteredInPendingRegistry(t *testing.T) {
	tests := []struct {
		operator rune
		sequence string
	}{
		{'y', "iw"},
		{'y', "aw"},
		{'v', "iw"},
		{'v', "aw"},
	}

	for _, tc := range tests {
		t.Run(string(tc.operator)+tc.sequence, func(t *testing.T) {
			cmd, ok := DefaultPendingRegistry.Get(tc.operator, tc.sequence)
			require.True(t, ok, "command should be registered")
			require.NotNil(t, cmd)
		})
	}
}

// ============================================================================
// WORD Object Tests (diW, daW, ciW, caW, yiW, yaW, viW, vaW)
// ============================================================================

func TestDeleteTextObjectCommand_InnerWORD_DeletesEntireNonWhitespaceSequence(t *testing.T) {
	// WORD includes punctuation - foo.bar is one WORD
	m := newTestModelWithContent("foo.bar baz")
	m.cursorCol = 2 // cursor in "foo.bar"

	cmd := &DeleteTextObjectCommand{object: 'W', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, " baz", m.content[0]) // entire foo.bar deleted
	require.Equal(t, 0, m.cursorCol)
}

func TestDeleteTextObjectCommand_AroundWORD_IncludesTrailingWhitespace(t *testing.T) {
	m := newTestModelWithContent("foo.bar baz")
	m.cursorCol = 2 // cursor in "foo.bar"

	cmd := &DeleteTextObjectCommand{object: 'W', inner: false}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "baz", m.content[0]) // foo.bar + space deleted
	require.Equal(t, 0, m.cursorCol)
}

func TestDeleteTextObjectCommand_WORD_FooDotBarIsOneWORD(t *testing.T) {
	// Unlike word which sees foo, ., bar as 3 parts, WORD sees it as one
	m := newTestModelWithContent("x foo.bar y")
	m.cursorCol = 4 // cursor at 'o' in "foo.bar"

	cmd := &DeleteTextObjectCommand{object: 'W', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "x  y", m.content[0])
}

func TestChangeTextObjectCommand_InnerWORD_DeletesEntireSequenceAndEntersInsertMode(t *testing.T) {
	m := newTestModelWithContent("foo.bar(baz) qux")
	m.cursorCol = 5 // cursor in the middle

	cmd := &ChangeTextObjectCommand{object: 'W', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, " qux", m.content[0])
	require.Equal(t, ModeInsert, m.mode)
	require.Equal(t, 0, m.cursorCol)
}

func TestYankTextObjectCommand_InnerWORD_YanksNonWhitespaceSequence(t *testing.T) {
	m := newTestModelWithContent("foo.bar baz")
	m.cursorCol = 2 // cursor in "foo.bar"

	cmd := &YankTextObjectCommand{object: 'W', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "foo.bar baz", m.content[0]) // content unchanged
	require.Equal(t, "foo.bar", m.lastYankedText) // yanked the WORD
	require.False(t, m.lastYankWasLinewise)
}

func TestYankTextObjectCommand_AroundWORD_YanksWithTrailingWhitespace(t *testing.T) {
	m := newTestModelWithContent("foo.bar baz")
	m.cursorCol = 2 // cursor in "foo.bar"

	cmd := &YankTextObjectCommand{object: 'W', inner: false}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "foo.bar baz", m.content[0])  // content unchanged
	require.Equal(t, "foo.bar ", m.lastYankedText) // yanked WORD + space
}

func TestVisualSelectTextObjectCommand_InnerWORD_SelectsNonWhitespaceSequence(t *testing.T) {
	m := newTestModelWithContent("foo.bar baz")
	m.cursorCol = 2 // cursor in "foo.bar"

	cmd := &VisualSelectTextObjectCommand{object: 'W', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, ModeVisual, m.mode)
	require.Equal(t, Position{Row: 0, Col: 0}, m.visualAnchor) // anchor at WORD start
	require.Equal(t, 6, m.cursorCol)                           // cursor at WORD end
}

func TestVisualSelectTextObjectCommand_AroundWORD_SelectsWithWhitespace(t *testing.T) {
	m := newTestModelWithContent("foo.bar baz")
	m.cursorCol = 2 // cursor in "foo.bar"

	cmd := &VisualSelectTextObjectCommand{object: 'W', inner: false}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, ModeVisual, m.mode)
	require.Equal(t, Position{Row: 0, Col: 0}, m.visualAnchor)
	require.Equal(t, 7, m.cursorCol) // includes trailing space
}

func TestWORDTextObject_AtLineEnd_UsesLeadingWhitespaceForAround(t *testing.T) {
	m := newTestModelWithContent("foo bar.baz")
	m.cursorCol = 6 // cursor in "bar.baz"

	cmd := &DeleteTextObjectCommand{object: 'W', inner: false}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	// "bar.baz" is at end, so aW includes leading whitespace
	require.Equal(t, "foo", m.content[0])
}

func TestWORDTextObject_WhitespaceOnlyLine_ReturnsFalse(t *testing.T) {
	m := newTestModelWithContent("   ")
	m.cursorCol = 1 // cursor on whitespace

	cmd := &DeleteTextObjectCommand{object: 'W', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Skipped, result)
	require.Equal(t, "   ", m.content[0]) // unchanged
}

func TestDeleteTextObjectCommand_WORD_Undo(t *testing.T) {
	m := newTestModelWithContent("foo.bar baz")
	m.cursorCol = 2

	cmd := &DeleteTextObjectCommand{object: 'W', inner: true}
	cmd.Execute(m)
	require.Equal(t, " baz", m.content[0])

	err := cmd.Undo(m)
	require.NoError(t, err)
	require.Equal(t, "foo.bar baz", m.content[0])
	require.Equal(t, 2, m.cursorCol)
}

func TestChangeTextObjectCommand_WORD_Undo_RestoresContentAndMode(t *testing.T) {
	m := newTestModelWithContent("foo.bar baz")
	m.cursorCol = 2

	cmd := &ChangeTextObjectCommand{object: 'W', inner: true}
	cmd.Execute(m)
	require.Equal(t, " baz", m.content[0])
	require.Equal(t, ModeInsert, m.mode)

	err := cmd.Undo(m)
	require.NoError(t, err)
	require.Equal(t, "foo.bar baz", m.content[0])
	require.Equal(t, ModeNormal, m.mode)
	require.Equal(t, 2, m.cursorCol)
}

func TestDeleteTextObjectCommand_WORD_Keys(t *testing.T) {
	cmd := &DeleteTextObjectCommand{object: 'W', inner: true}
	require.Equal(t, []string{"diW"}, cmd.Keys())

	cmd2 := &DeleteTextObjectCommand{object: 'W', inner: false}
	require.Equal(t, []string{"daW"}, cmd2.Keys())
}

func TestChangeTextObjectCommand_WORD_Keys(t *testing.T) {
	cmd := &ChangeTextObjectCommand{object: 'W', inner: true}
	require.Equal(t, []string{"ciW"}, cmd.Keys())

	cmd2 := &ChangeTextObjectCommand{object: 'W', inner: false}
	require.Equal(t, []string{"caW"}, cmd2.Keys())
}

func TestYankTextObjectCommand_WORD_Keys(t *testing.T) {
	cmd := &YankTextObjectCommand{object: 'W', inner: true}
	require.Equal(t, []string{"yiW"}, cmd.Keys())

	cmd2 := &YankTextObjectCommand{object: 'W', inner: false}
	require.Equal(t, []string{"yaW"}, cmd2.Keys())
}

func TestVisualSelectTextObjectCommand_WORD_Keys(t *testing.T) {
	cmd := &VisualSelectTextObjectCommand{object: 'W', inner: true}
	require.Equal(t, []string{"viW"}, cmd.Keys())

	cmd2 := &VisualSelectTextObjectCommand{object: 'W', inner: false}
	require.Equal(t, []string{"vaW"}, cmd2.Keys())
}

// ============================================================================
// Integration Tests - WORD Command Registration
// ============================================================================

func TestWORDTextObjectCommands_RegisteredInPendingRegistry(t *testing.T) {
	tests := []struct {
		operator rune
		sequence string
	}{
		{'d', "iW"},
		{'d', "aW"},
		{'c', "iW"},
		{'c', "aW"},
		{'y', "iW"},
		{'y', "aW"},
		{'v', "iW"},
		{'v', "aW"},
	}

	for _, tc := range tests {
		t.Run(string(tc.operator)+tc.sequence, func(t *testing.T) {
			cmd, ok := DefaultPendingRegistry.Get(tc.operator, tc.sequence)
			require.True(t, ok, "command should be registered")
			require.NotNil(t, cmd)
		})
	}
}

// ============================================================================
// Quote Object Tests (di", da", ci", ca", yi", ya", vi", va")
// ============================================================================

func TestDeleteTextObjectCommand_InnerDoubleQuote_DeletesContentInsideQuotes(t *testing.T) {
	m := newTestModelWithContent(`say "hello world" now`)
	m.cursorCol = 7 // cursor at 'l' inside quotes

	cmd := &DeleteTextObjectCommand{object: '"', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, `say "" now`, m.content[0])
	require.Equal(t, 5, m.cursorCol) // cursor positioned after opening "
}

func TestDeleteTextObjectCommand_AroundDoubleQuote_DeletesContentIncludingQuotes(t *testing.T) {
	m := newTestModelWithContent(`say "hello" now`)
	m.cursorCol = 6 // cursor at 'l' inside quotes

	cmd := &DeleteTextObjectCommand{object: '"', inner: false}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, `say  now`, m.content[0])
	require.Equal(t, 4, m.cursorCol)
}

func TestChangeTextObjectCommand_InnerSingleQuote_DeletesAndEntersInsertMode(t *testing.T) {
	m := newTestModelWithContent(`say 'hello' now`)
	m.cursorCol = 6 // cursor at 'l' inside quotes

	cmd := &ChangeTextObjectCommand{object: '\'', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, `say '' now`, m.content[0])
	require.Equal(t, ModeInsert, m.mode)
	require.Equal(t, 5, m.cursorCol) // cursor positioned inside empty quotes
}

func TestDeleteTextObjectCommand_Quote_CursorInsideQuotes(t *testing.T) {
	m := newTestModelWithContent(`"hello world"`)
	m.cursorCol = 6 // cursor at 'w'

	cmd := &DeleteTextObjectCommand{object: '"', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, `""`, m.content[0])
}

func TestDeleteTextObjectCommand_Quote_CursorOnOpeningQuote(t *testing.T) {
	m := newTestModelWithContent(`say "hello" now`)
	m.cursorCol = 4 // cursor on opening "

	cmd := &DeleteTextObjectCommand{object: '"', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, `say "" now`, m.content[0])
}

func TestDeleteTextObjectCommand_Quote_CursorOnClosingQuote(t *testing.T) {
	m := newTestModelWithContent(`say "hello" now`)
	m.cursorCol = 10 // cursor on closing "

	cmd := &DeleteTextObjectCommand{object: '"', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, `say "" now`, m.content[0])
}

func TestDeleteTextObjectCommand_Quote_EscapedQuoteNotDelimiter(t *testing.T) {
	// The \" in the middle should not be treated as a delimiter
	m := newTestModelWithContent(`say "hello \"world\" end" now`)
	m.cursorCol = 7 // cursor at 'l' inside quotes

	cmd := &DeleteTextObjectCommand{object: '"', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, `say "" now`, m.content[0])
	require.Equal(t, `hello \"world\" end`, m.lastYankedText)
}

func TestDeleteTextObjectCommand_Quote_ConsecutiveEscapes(t *testing.T) {
	// "\\" is an escaped backslash, followed by " which IS a delimiter
	m := newTestModelWithContent(`say "foo\\" now`)
	m.cursorCol = 6 // cursor at 'o' inside quotes

	cmd := &DeleteTextObjectCommand{object: '"', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, `say "" now`, m.content[0])
	require.Equal(t, `foo\\`, m.lastYankedText)
}

func TestDeleteTextObjectCommand_Quote_UnclosedQuoteIsNoOp(t *testing.T) {
	m := newTestModelWithContent(`say "hello world`)
	m.cursorCol = 7 // cursor at 'l' - inside unclosed quote

	cmd := &DeleteTextObjectCommand{object: '"', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Skipped, result)
	require.Equal(t, `say "hello world`, m.content[0]) // unchanged
}

func TestDeleteTextObjectCommand_Quote_CursorOutsideQuotesIsNoOp(t *testing.T) {
	m := newTestModelWithContent(`say "hello" now`)
	m.cursorCol = 0 // cursor at 's' - outside quotes

	cmd := &DeleteTextObjectCommand{object: '"', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Skipped, result)
	require.Equal(t, `say "hello" now`, m.content[0]) // unchanged
}

func TestDeleteTextObjectCommand_Quote_NestedSingleInsideDouble(t *testing.T) {
	// Cursor inside double quotes - di" should get the whole thing including nested single quotes
	m := newTestModelWithContent(`say "foo 'bar' baz" now`)
	m.cursorCol = 10 // cursor at 'b' in 'bar'

	cmd := &DeleteTextObjectCommand{object: '"', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, `say "" now`, m.content[0])
	require.Equal(t, "foo 'bar' baz", m.lastYankedText)
}

func TestDeleteTextObjectCommand_Quote_SingleQuoteInsideDouble_SelectsSingleQuote(t *testing.T) {
	// Using single quote object on cursor inside single quotes within double quotes
	m := newTestModelWithContent(`say "foo 'bar' baz" now`)
	m.cursorCol = 11 // cursor at 'a' in 'bar'

	cmd := &DeleteTextObjectCommand{object: '\'', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, `say "foo '' baz" now`, m.content[0])
	require.Equal(t, "bar", m.lastYankedText)
}

func TestYankTextObjectCommand_InnerQuote_YanksQuotedContent(t *testing.T) {
	m := newTestModelWithContent(`say "hello world" now`)
	m.cursorCol = 7 // cursor at 'l' inside quotes

	cmd := &YankTextObjectCommand{object: '"', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, `say "hello world" now`, m.content[0]) // unchanged
	require.Equal(t, "hello world", m.lastYankedText)
	require.False(t, m.lastYankWasLinewise)
}

func TestYankTextObjectCommand_AroundQuote_YanksWithQuotes(t *testing.T) {
	m := newTestModelWithContent(`say "hello" now`)
	m.cursorCol = 6 // cursor at 'l' inside quotes

	cmd := &YankTextObjectCommand{object: '"', inner: false}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, `say "hello" now`, m.content[0]) // unchanged
	require.Equal(t, `"hello"`, m.lastYankedText)
}

func TestVisualSelectTextObjectCommand_InnerQuote_SelectsQuotedContent(t *testing.T) {
	m := newTestModelWithContent(`say "hello" now`)
	m.cursorCol = 6 // cursor at 'l' inside quotes

	cmd := &VisualSelectTextObjectCommand{object: '"', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, ModeVisual, m.mode)
	require.Equal(t, Position{Row: 0, Col: 5}, m.visualAnchor) // after opening "
	require.Equal(t, 9, m.cursorCol)                           // before closing "
}

func TestVisualSelectTextObjectCommand_AroundQuote_SelectsWithQuotes(t *testing.T) {
	m := newTestModelWithContent(`say "hello" now`)
	m.cursorCol = 6 // cursor at 'l' inside quotes

	cmd := &VisualSelectTextObjectCommand{object: '"', inner: false}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, ModeVisual, m.mode)
	require.Equal(t, Position{Row: 0, Col: 4}, m.visualAnchor) // opening "
	require.Equal(t, 10, m.cursorCol)                          // closing "
}

func TestDeleteTextObjectCommand_Quote_Undo_RestoresContent(t *testing.T) {
	m := newTestModelWithContent(`say "hello" now`)
	m.cursorCol = 6

	cmd := &DeleteTextObjectCommand{object: '"', inner: true}
	cmd.Execute(m)
	require.Equal(t, `say "" now`, m.content[0])

	err := cmd.Undo(m)
	require.NoError(t, err)
	require.Equal(t, `say "hello" now`, m.content[0])
	require.Equal(t, 6, m.cursorCol)
}

func TestDeleteTextObjectCommand_Quote_Redo_RestoresExecutedState(t *testing.T) {
	m := newTestModelWithContent(`say "hello" now`)
	m.cursorCol = 6

	cmd := &DeleteTextObjectCommand{object: '"', inner: true}
	cmd.Execute(m)
	executedContent := m.content[0]

	// Undo
	cmd.Undo(m)
	require.Equal(t, `say "hello" now`, m.content[0])

	// Redo (re-execute)
	cmd.Execute(m)
	require.Equal(t, executedContent, m.content[0])
}

func TestChangeTextObjectCommand_Quote_Undo_RestoresContentAndMode(t *testing.T) {
	m := newTestModelWithContent(`say "hello" now`)
	m.cursorCol = 6

	cmd := &ChangeTextObjectCommand{object: '"', inner: true}
	cmd.Execute(m)
	require.Equal(t, `say "" now`, m.content[0])
	require.Equal(t, ModeInsert, m.mode)

	err := cmd.Undo(m)
	require.NoError(t, err)
	require.Equal(t, `say "hello" now`, m.content[0])
	require.Equal(t, ModeNormal, m.mode)
	require.Equal(t, 6, m.cursorCol)
}

func TestDeleteTextObjectCommand_Quote_EmptyQuotes(t *testing.T) {
	m := newTestModelWithContent(`say "" now`)
	m.cursorCol = 4 // cursor on opening "

	// Inner delete on empty quotes - should handle gracefully
	cmd := &DeleteTextObjectCommand{object: '"', inner: true}
	result := cmd.Execute(m)

	// For empty quotes, we get start > end, so deletion should handle it gracefully
	require.Equal(t, Executed, result)
	// Nothing to delete inside empty quotes
	require.Equal(t, `say "" now`, m.content[0])
}

func TestDeleteTextObjectCommand_Quote_AroundEmptyQuotes(t *testing.T) {
	m := newTestModelWithContent(`say "" now`)
	m.cursorCol = 4 // cursor on opening "

	cmd := &DeleteTextObjectCommand{object: '"', inner: false}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, `say  now`, m.content[0])
}

func TestDeleteTextObjectCommand_Quote_MultipleQuotePairs_SecondPair(t *testing.T) {
	m := newTestModelWithContent(`"first" and "second"`)
	m.cursorCol = 15 // cursor at 'c' in "second"

	cmd := &DeleteTextObjectCommand{object: '"', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, `"first" and ""`, m.content[0])
	require.Equal(t, "second", m.lastYankedText)
}

func TestDeleteTextObjectCommand_Quote_MultipleQuotePairs_FirstPair(t *testing.T) {
	m := newTestModelWithContent(`"first" and "second"`)
	m.cursorCol = 3 // cursor at 'r' in "first"

	cmd := &DeleteTextObjectCommand{object: '"', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, `"" and "second"`, m.content[0])
	require.Equal(t, "first", m.lastYankedText)
}

func TestDeleteTextObjectCommand_Quote_Keys(t *testing.T) {
	cmd := &DeleteTextObjectCommand{object: '"', inner: true}
	require.Equal(t, []string{`di"`}, cmd.Keys())

	cmd2 := &DeleteTextObjectCommand{object: '"', inner: false}
	require.Equal(t, []string{`da"`}, cmd2.Keys())

	cmd3 := &DeleteTextObjectCommand{object: '\'', inner: true}
	require.Equal(t, []string{`di'`}, cmd3.Keys())

	cmd4 := &DeleteTextObjectCommand{object: '\'', inner: false}
	require.Equal(t, []string{`da'`}, cmd4.Keys())
}

func TestChangeTextObjectCommand_Quote_Keys(t *testing.T) {
	cmd := &ChangeTextObjectCommand{object: '"', inner: true}
	require.Equal(t, []string{`ci"`}, cmd.Keys())

	cmd2 := &ChangeTextObjectCommand{object: '"', inner: false}
	require.Equal(t, []string{`ca"`}, cmd2.Keys())
}

func TestYankTextObjectCommand_Quote_Keys(t *testing.T) {
	cmd := &YankTextObjectCommand{object: '"', inner: true}
	require.Equal(t, []string{`yi"`}, cmd.Keys())

	cmd2 := &YankTextObjectCommand{object: '"', inner: false}
	require.Equal(t, []string{`ya"`}, cmd2.Keys())
}

func TestVisualSelectTextObjectCommand_Quote_Keys(t *testing.T) {
	cmd := &VisualSelectTextObjectCommand{object: '"', inner: true}
	require.Equal(t, []string{`vi"`}, cmd.Keys())

	cmd2 := &VisualSelectTextObjectCommand{object: '"', inner: false}
	require.Equal(t, []string{`va"`}, cmd2.Keys())
}

// ============================================================================
// Integration Tests - Quote Command Registration
// ============================================================================

func TestQuoteTextObjectCommands_RegisteredInPendingRegistry(t *testing.T) {
	tests := []struct {
		operator rune
		sequence string
	}{
		{'d', `i"`},
		{'d', `a"`},
		{'c', `i"`},
		{'c', `a"`},
		{'y', `i"`},
		{'y', `a"`},
		{'v', `i"`},
		{'v', `a"`},
		{'d', `i'`},
		{'d', `a'`},
		{'c', `i'`},
		{'c', `a'`},
		{'y', `i'`},
		{'y', `a'`},
		{'v', `i'`},
		{'v', `a'`},
	}

	for _, tc := range tests {
		t.Run(string(tc.operator)+tc.sequence, func(t *testing.T) {
			cmd, ok := DefaultPendingRegistry.Get(tc.operator, tc.sequence)
			require.True(t, ok, "command should be registered")
			require.NotNil(t, cmd)
		})
	}
}

// ============================================================================
// Bracket Text Object Tests (dib, dab, cib, cab, yib, yab, vib, vab)
// ============================================================================

func TestBracketTextObjectCommands_RegisteredInPendingRegistry(t *testing.T) {
	tests := []struct {
		operator rune
		sequence string
	}{
		{'d', "ib"},
		{'d', "ab"},
		{'c', "ib"},
		{'c', "ab"},
		{'y', "ib"},
		{'y', "ab"},
		{'v', "ib"},
		{'v', "ab"},
	}

	for _, tc := range tests {
		t.Run(string(tc.operator)+tc.sequence, func(t *testing.T) {
			cmd, ok := DefaultPendingRegistry.Get(tc.operator, tc.sequence)
			require.True(t, ok, "command should be registered")
			require.NotNil(t, cmd)
		})
	}
}

// ============================================================================
// DeleteTextObjectCommand - Inner Bracket (dib)
// ============================================================================

func TestDeleteTextObjectCommand_InnerBracket_Parentheses(t *testing.T) {
	m := newTestModelWithContent("foo(bar)baz")
	m.cursorCol = 5 // cursor at 'a' in "bar"

	cmd := &DeleteTextObjectCommand{object: 'b', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "foo()baz", m.content[0])
	require.Equal(t, "bar", m.lastYankedText)
}

func TestDeleteTextObjectCommand_InnerBracket_SquareBrackets(t *testing.T) {
	m := newTestModelWithContent("foo[bar]baz")
	m.cursorCol = 5 // cursor at 'a' in "bar"

	cmd := &DeleteTextObjectCommand{object: 'b', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "foo[]baz", m.content[0])
	require.Equal(t, "bar", m.lastYankedText)
}

func TestDeleteTextObjectCommand_InnerBracket_CurlyBraces(t *testing.T) {
	m := newTestModelWithContent("foo{bar}baz")
	m.cursorCol = 5 // cursor at 'a' in "bar"

	cmd := &DeleteTextObjectCommand{object: 'b', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "foo{}baz", m.content[0])
	require.Equal(t, "bar", m.lastYankedText)
}

func TestDeleteTextObjectCommand_InnerBracket_NestedSelectsInnermost(t *testing.T) {
	// {[foo]} with cursor on foo should select []
	m := newTestModelWithContent("{[foo]}")
	m.cursorCol = 3 // cursor at 'o' in "foo"

	cmd := &DeleteTextObjectCommand{object: 'b', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "{[]}", m.content[0])
	require.Equal(t, "foo", m.lastYankedText)
}

func TestDeleteTextObjectCommand_InnerBracket_NestedSameType(t *testing.T) {
	// ((foo)) with cursor on foo should select inner ()
	m := newTestModelWithContent("((foo))")
	m.cursorCol = 3 // cursor at 'o' in "foo"

	cmd := &DeleteTextObjectCommand{object: 'b', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "(())", m.content[0])
	require.Equal(t, "foo", m.lastYankedText)
}

func TestDeleteTextObjectCommand_InnerBracket_NoBrackets_Skipped(t *testing.T) {
	m := newTestModelWithContent("foo bar baz")
	m.cursorCol = 5 // cursor at 'a' in "bar"

	cmd := &DeleteTextObjectCommand{object: 'b', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Skipped, result)
	require.Equal(t, "foo bar baz", m.content[0]) // unchanged
}

func TestDeleteTextObjectCommand_InnerBracket_CursorOnOpeningDelimiter(t *testing.T) {
	m := newTestModelWithContent("foo(bar)baz")
	m.cursorCol = 3 // cursor on '('

	cmd := &DeleteTextObjectCommand{object: 'b', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "foo()baz", m.content[0])
}

func TestDeleteTextObjectCommand_InnerBracket_CursorOnClosingDelimiter(t *testing.T) {
	m := newTestModelWithContent("foo(bar)baz")
	m.cursorCol = 7 // cursor on ')'

	cmd := &DeleteTextObjectCommand{object: 'b', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "foo()baz", m.content[0])
}

// ============================================================================
// DeleteTextObjectCommand - Around Bracket (dab)
// ============================================================================

func TestDeleteTextObjectCommand_AroundBracket_Parentheses(t *testing.T) {
	m := newTestModelWithContent("foo(bar)baz")
	m.cursorCol = 5 // cursor at 'a' in "bar"

	cmd := &DeleteTextObjectCommand{object: 'b', inner: false}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "foobaz", m.content[0])
	require.Equal(t, "(bar)", m.lastYankedText)
}

func TestDeleteTextObjectCommand_AroundBracket_SquareBrackets(t *testing.T) {
	m := newTestModelWithContent("foo[bar]baz")
	m.cursorCol = 5 // cursor at 'a' in "bar"

	cmd := &DeleteTextObjectCommand{object: 'b', inner: false}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "foobaz", m.content[0])
	require.Equal(t, "[bar]", m.lastYankedText)
}

func TestDeleteTextObjectCommand_AroundBracket_CurlyBraces(t *testing.T) {
	m := newTestModelWithContent("foo{bar}baz")
	m.cursorCol = 5 // cursor at 'a' in "bar"

	cmd := &DeleteTextObjectCommand{object: 'b', inner: false}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "foobaz", m.content[0])
	require.Equal(t, "{bar}", m.lastYankedText)
}

func TestDeleteTextObjectCommand_AroundBracket_NestedSelectsInnermost(t *testing.T) {
	// {[foo]} with cursor on foo should select [foo]
	m := newTestModelWithContent("{[foo]}")
	m.cursorCol = 3 // cursor at 'o' in "foo"

	cmd := &DeleteTextObjectCommand{object: 'b', inner: false}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "{}", m.content[0])
	require.Equal(t, "[foo]", m.lastYankedText)
}

// ============================================================================
// ChangeTextObjectCommand - Inner Bracket (cib)
// ============================================================================

func TestChangeTextObjectCommand_InnerBracket_DeletesAndEntersInsertMode(t *testing.T) {
	m := newTestModelWithContent("foo(bar)baz")
	m.cursorCol = 5 // cursor at 'a' in "bar"

	cmd := &ChangeTextObjectCommand{object: 'b', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "foo()baz", m.content[0])
	require.Equal(t, ModeInsert, m.mode)
	require.Equal(t, 4, m.cursorCol) // cursor inside empty brackets
}

func TestChangeTextObjectCommand_InnerBracket_SquareBrackets(t *testing.T) {
	m := newTestModelWithContent("arr[index]")
	m.cursorCol = 5 // cursor in "index"

	cmd := &ChangeTextObjectCommand{object: 'b', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "arr[]", m.content[0])
	require.Equal(t, ModeInsert, m.mode)
}

func TestChangeTextObjectCommand_InnerBracket_CurlyBraces(t *testing.T) {
	m := newTestModelWithContent("obj{field}")
	m.cursorCol = 5 // cursor in "field"

	cmd := &ChangeTextObjectCommand{object: 'b', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "obj{}", m.content[0])
	require.Equal(t, ModeInsert, m.mode)
}

// ============================================================================
// ChangeTextObjectCommand - Around Bracket (cab)
// ============================================================================

func TestChangeTextObjectCommand_AroundBracket_DeletesAllAndEntersInsertMode(t *testing.T) {
	m := newTestModelWithContent("foo(bar)baz")
	m.cursorCol = 5 // cursor at 'a' in "bar"

	cmd := &ChangeTextObjectCommand{object: 'b', inner: false}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "foobaz", m.content[0])
	require.Equal(t, ModeInsert, m.mode)
	require.Equal(t, 3, m.cursorCol) // cursor where brackets were
}

// ============================================================================
// YankTextObjectCommand - Inner Bracket (yib)
// ============================================================================

func TestYankTextObjectCommand_InnerBracket_YanksContentWithoutModifying(t *testing.T) {
	m := newTestModelWithContent("foo(bar)baz")
	m.cursorCol = 5 // cursor at 'a' in "bar"

	cmd := &YankTextObjectCommand{object: 'b', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "foo(bar)baz", m.content[0]) // content unchanged
	require.Equal(t, "bar", m.lastYankedText)
	require.False(t, m.lastYankWasLinewise)
}

func TestYankTextObjectCommand_InnerBracket_SquareBrackets(t *testing.T) {
	m := newTestModelWithContent("arr[index]")
	m.cursorCol = 5 // cursor in "index"

	cmd := &YankTextObjectCommand{object: 'b', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "arr[index]", m.content[0]) // unchanged
	require.Equal(t, "index", m.lastYankedText)
}

// ============================================================================
// YankTextObjectCommand - Around Bracket (yab)
// ============================================================================

func TestYankTextObjectCommand_AroundBracket_YanksBracketsAndContent(t *testing.T) {
	m := newTestModelWithContent("foo(bar)baz")
	m.cursorCol = 5 // cursor at 'a' in "bar"

	cmd := &YankTextObjectCommand{object: 'b', inner: false}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, "foo(bar)baz", m.content[0]) // content unchanged
	require.Equal(t, "(bar)", m.lastYankedText)
}

// ============================================================================
// VisualSelectTextObjectCommand - Inner Bracket (vib)
// ============================================================================

func TestVisualSelectTextObjectCommand_InnerBracket_EntersVisualWithSelection(t *testing.T) {
	m := newTestModelWithContent("foo(bar)baz")
	m.cursorCol = 5 // cursor at 'a' in "bar"

	cmd := &VisualSelectTextObjectCommand{object: 'b', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, ModeVisual, m.mode)
	require.Equal(t, Position{Row: 0, Col: 4}, m.visualAnchor) // after '('
	require.Equal(t, 6, m.cursorCol)                           // before ')'
}

func TestVisualSelectTextObjectCommand_InnerBracket_SquareBrackets(t *testing.T) {
	m := newTestModelWithContent("arr[index]")
	m.cursorCol = 5 // cursor in "index"

	cmd := &VisualSelectTextObjectCommand{object: 'b', inner: true}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, ModeVisual, m.mode)
	require.Equal(t, Position{Row: 0, Col: 4}, m.visualAnchor) // after '['
	require.Equal(t, 8, m.cursorCol)                           // before ']'
}

// ============================================================================
// VisualSelectTextObjectCommand - Around Bracket (vab)
// ============================================================================

func TestVisualSelectTextObjectCommand_AroundBracket_EntersVisualWithSelection(t *testing.T) {
	m := newTestModelWithContent("foo(bar)baz")
	m.cursorCol = 5 // cursor at 'a' in "bar"

	cmd := &VisualSelectTextObjectCommand{object: 'b', inner: false}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	require.Equal(t, ModeVisual, m.mode)
	require.Equal(t, Position{Row: 0, Col: 3}, m.visualAnchor) // at '('
	require.Equal(t, 7, m.cursorCol)                           // at ')'
}

// ============================================================================
// Bracket Text Object - Undo/Redo Tests
// ============================================================================

func TestDeleteTextObjectCommand_InnerBracket_Undo(t *testing.T) {
	m := newTestModelWithContent("foo(bar)baz")
	m.cursorCol = 5

	cmd := &DeleteTextObjectCommand{object: 'b', inner: true}
	cmd.Execute(m)
	require.Equal(t, "foo()baz", m.content[0])

	err := cmd.Undo(m)
	require.NoError(t, err)
	require.Equal(t, "foo(bar)baz", m.content[0])
	require.Equal(t, 5, m.cursorCol) // cursor restored
}

func TestDeleteTextObjectCommand_AroundBracket_Undo(t *testing.T) {
	m := newTestModelWithContent("foo(bar)baz")
	m.cursorCol = 5

	cmd := &DeleteTextObjectCommand{object: 'b', inner: false}
	cmd.Execute(m)
	require.Equal(t, "foobaz", m.content[0])

	err := cmd.Undo(m)
	require.NoError(t, err)
	require.Equal(t, "foo(bar)baz", m.content[0])
	require.Equal(t, 5, m.cursorCol)
}

func TestChangeTextObjectCommand_InnerBracket_Undo_RestoresContentAndMode(t *testing.T) {
	m := newTestModelWithContent("foo(bar)baz")
	m.cursorCol = 5

	cmd := &ChangeTextObjectCommand{object: 'b', inner: true}
	cmd.Execute(m)
	require.Equal(t, "foo()baz", m.content[0])
	require.Equal(t, ModeInsert, m.mode)

	err := cmd.Undo(m)
	require.NoError(t, err)
	require.Equal(t, "foo(bar)baz", m.content[0])
	require.Equal(t, ModeNormal, m.mode)
	require.Equal(t, 5, m.cursorCol)
}

func TestDeleteTextObjectCommand_InnerBracket_Redo(t *testing.T) {
	m := newTestModelWithContent("foo(bar)baz")
	m.cursorCol = 5

	cmd := &DeleteTextObjectCommand{object: 'b', inner: true}
	cmd.Execute(m)
	executedContent := m.content[0]

	// Undo
	cmd.Undo(m)
	require.Equal(t, "foo(bar)baz", m.content[0])

	// Redo (re-execute)
	cmd.Execute(m)
	require.Equal(t, executedContent, m.content[0])
}

// ============================================================================
// Bracket Text Object - Keys Tests
// ============================================================================

func TestDeleteTextObjectCommand_Bracket_Keys(t *testing.T) {
	cmd := &DeleteTextObjectCommand{object: 'b', inner: true}
	require.Equal(t, []string{"dib"}, cmd.Keys())

	cmd2 := &DeleteTextObjectCommand{object: 'b', inner: false}
	require.Equal(t, []string{"dab"}, cmd2.Keys())
}

func TestChangeTextObjectCommand_Bracket_Keys(t *testing.T) {
	cmd := &ChangeTextObjectCommand{object: 'b', inner: true}
	require.Equal(t, []string{"cib"}, cmd.Keys())

	cmd2 := &ChangeTextObjectCommand{object: 'b', inner: false}
	require.Equal(t, []string{"cab"}, cmd2.Keys())
}

func TestYankTextObjectCommand_Bracket_Keys(t *testing.T) {
	cmd := &YankTextObjectCommand{object: 'b', inner: true}
	require.Equal(t, []string{"yib"}, cmd.Keys())

	cmd2 := &YankTextObjectCommand{object: 'b', inner: false}
	require.Equal(t, []string{"yab"}, cmd2.Keys())
}

func TestVisualSelectTextObjectCommand_Bracket_Keys(t *testing.T) {
	cmd := &VisualSelectTextObjectCommand{object: 'b', inner: true}
	require.Equal(t, []string{"vib"}, cmd.Keys())

	cmd2 := &VisualSelectTextObjectCommand{object: 'b', inner: false}
	require.Equal(t, []string{"vab"}, cmd2.Keys())
}
