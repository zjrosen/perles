package vimtextarea

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPasteAfterCommand_Emoji tests pasting emoji does not corrupt UTF-8
func TestPasteAfterCommand_Emoji(t *testing.T) {
	m := newTestModelWithContent("i know wat ")
	m.lastYankedText = "😀"
	m.lastYankWasLinewise = false
	m.cursorCol = 11 // After "wat "

	cmd := &PasteAfterCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	// Should paste emoji without corruption
	require.Equal(t, "i know wat 😀", m.content[0], "emoji should paste without UTF-8 corruption")
	require.Equal(t, 11, m.cursorCol, "cursor should be on the pasted emoji (grapheme index 11)")
}

// TestPasteBeforeCommand_Emoji tests pasting emoji before cursor
func TestPasteBeforeCommand_Emoji(t *testing.T) {
	m := newTestModelWithContent("i know wat ")
	m.lastYankedText = "😀"
	m.lastYankWasLinewise = false
	m.cursorCol = 11 // After "wat "

	cmd := &PasteBeforeCommand{}
	result := cmd.Execute(m)

	require.Equal(t, Executed, result)
	// Should paste emoji without corruption
	require.Equal(t, "i know wat 😀", m.content[0], "emoji should paste without UTF-8 corruption")
	require.Equal(t, 11, m.cursorCol, "cursor should be on the pasted emoji")
}
