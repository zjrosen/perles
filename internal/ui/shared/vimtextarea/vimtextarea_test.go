package vimtextarea

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_CreatesModelWithCorrectDefaults(t *testing.T) {
	m := New(Config{})

	assert.Equal(t, []string{""}, m.content)
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 0, m.cursorCol)
	assert.False(t, m.focused)
	// When VimEnabled is false (default), should start in Insert mode
	assert.Equal(t, ModeInsert, m.mode)
}

func TestNew_WithVimEnabled(t *testing.T) {
	m := New(Config{
		VimEnabled:  true,
		DefaultMode: ModeNormal,
	})

	assert.True(t, m.VimEnabled())
	assert.Equal(t, ModeNormal, m.Mode())
}

func TestNew_ConfigOptionsApplied(t *testing.T) {
	cfg := Config{
		VimEnabled:  true,
		DefaultMode: ModeInsert,
		Placeholder: "Enter text...",
		CharLimit:   100,
		MaxHeight:   5,
	}
	m := New(cfg)

	assert.True(t, m.VimEnabled())
	assert.Equal(t, ModeInsert, m.Mode())
	assert.Equal(t, "Enter text...", m.config.Placeholder)
	assert.Equal(t, 100, m.config.CharLimit)
	assert.Equal(t, 5, m.config.MaxHeight)
}

func TestSetSize_UpdatesDimensions(t *testing.T) {
	m := New(Config{})

	m.SetSize(80, 24)

	assert.Equal(t, 80, m.width)
	assert.Equal(t, 24, m.height)
}

func TestFocus_Blur_TogglesState(t *testing.T) {
	m := New(Config{})

	assert.False(t, m.Focused())

	m.Focus()
	assert.True(t, m.Focused())

	m.Blur()
	assert.False(t, m.Focused())
}

func TestBlur_ClearsPendingCommand(t *testing.T) {
	m := New(Config{VimEnabled: true})
	m.pendingBuilder.SetOperator('d') // Simulate partial delete command

	m.Blur()

	assert.True(t, m.pendingBuilder.IsEmpty())
}

func TestSetValue_Value_RoundTrips(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty string", ""},
		{"single line", "hello world"},
		{"multiple lines", "line1\nline2\nline3"},
		{"trailing newline", "hello\n"},
		{"empty lines", "a\n\nb"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(Config{})
			m.SetValue(tt.input)
			got := m.Value()
			assert.Equal(t, tt.input, got)
		})
	}
}

func TestLines_ReturnsContentAsSlice(t *testing.T) {
	m := New(Config{})
	m.SetValue("line1\nline2\nline3")

	lines := m.Lines()

	require.Len(t, lines, 3)
	assert.Equal(t, "line1", lines[0])
	assert.Equal(t, "line2", lines[1])
	assert.Equal(t, "line3", lines[2])
}

func TestMode_StringRepresentations(t *testing.T) {
	tests := []struct {
		mode     Mode
		expected string
	}{
		{ModeNormal, "NORMAL"},
		{ModeInsert, "INSERT"},
		{ModeVisual, "VISUAL"},
		{ModeVisualLine, "VISUAL LINE"},
		{ModeReplace, "REPLACE"},
		{Mode(99), "UNKNOWN"}, // Invalid mode
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.mode.String())
		})
	}
}

func TestModeReplace_ConstantValue(t *testing.T) {
	// Verify ModeReplace has correct iota value (after ModeVisualLine)
	assert.Equal(t, Mode(4), ModeReplace)
}

func TestIsMouseEscapeSequence(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"SGR scroll up", "[<65;87;15M", true},
		{"SGR scroll down", "[<64;42;10M", true},
		{"SGR button release", "[<35;50;20m", true},
		{"SGR without bracket", "<65;87;15M", true},
		{"normal text", "hello", false},
		{"short input", "[<1M", false},
		{"partial sequence", "[<65;87", false},
		{"wrong ending", "[<65;87;15X", false},
		{"single char", "a", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isMouseEscapeSequence([]rune(tt.input))
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMouseEscapeSequence_NotInsertedInInsertMode(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("hello")

	// Simulate SGR mouse scroll event that wasn't parsed by bubbletea
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[<65;87;15M")})

	// Should remain unchanged - mouse escape sequence should be filtered
	assert.Equal(t, "hello", m.Value())
}

func TestMouseEscapeSequence_NotInsertedInReplaceMode(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeReplace})
	m.SetValue("hello")
	m.cursorCol = 0

	// Simulate SGR mouse scroll event that wasn't parsed by bubbletea
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[<65;87;15M")})

	// Should remain unchanged - mouse escape sequence should be filtered
	assert.Equal(t, "hello", m.Value())
}

func TestPaste_AtEnd(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("hello")
	m.cursorCol = 5 // At end

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" world")})

	assert.Equal(t, "hello world", m.Value())
	assert.Equal(t, 11, m.cursorCol) // Cursor moved past inserted text
}

func TestPaste_Middle(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("helloworld")
	m.cursorCol = 5 // Between hello and world

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})

	assert.Equal(t, "hello world", m.Value())
	assert.Equal(t, 6, m.cursorCol)
}

func TestPaste_MultiLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("before after")
	m.cursorCol = 7 // After "before "

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("line1\nline2\nline3")})

	assert.Equal(t, "before line1\nline2\nline3after", m.Value())
	assert.Equal(t, 2, m.cursorRow) // Cursor on last inserted line
	assert.Equal(t, 5, m.cursorCol) // After "line3"
}

func TestPaste_MultiLine_EmptyLines(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("start")
	m.cursorCol = 5 // At end

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("\n\n")})

	assert.Equal(t, "start\n\n", m.Value())
	assert.Equal(t, 2, m.cursorRow)
	assert.Equal(t, 0, m.cursorCol)
}

func TestPaste_TwoLines(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.Focus()

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hello\nworld")})

	assert.Equal(t, "hello\nworld", m.Value())
	assert.Equal(t, 1, m.cursorRow)
	assert.Equal(t, 5, m.cursorCol)
}

func TestCursorPosition(t *testing.T) {
	m := New(Config{})
	m.SetValue("line1\nline2\nline3")
	m.cursorRow = 1
	m.cursorCol = 3

	pos := m.CursorPosition()

	assert.Equal(t, 1, pos.Row)
	assert.Equal(t, 3, pos.Col)
}

func TestSetVimEnabled(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	assert.True(t, m.VimEnabled())
	assert.Equal(t, ModeNormal, m.Mode())

	// Disabling vim should switch to Insert mode
	m.SetVimEnabled(false)
	assert.False(t, m.VimEnabled())
	assert.Equal(t, ModeInsert, m.Mode())

	// Re-enabling vim doesn't automatically restore previous mode
	m.SetVimEnabled(true)
	assert.True(t, m.VimEnabled())
	assert.Equal(t, ModeInsert, m.Mode()) // Stays in Insert
}

func TestClearPendingCommand(t *testing.T) {
	m := New(Config{VimEnabled: true})
	m.pendingBuilder.SetOperator('d')

	m.ClearPendingCommand()

	assert.True(t, m.pendingBuilder.IsEmpty())
}

func TestSetMode(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})

	m.SetMode(ModeInsert)
	assert.Equal(t, ModeInsert, m.Mode())

	m.SetMode(ModeVisual)
	assert.Equal(t, ModeVisual, m.Mode())

	m.SetMode(ModeNormal)
	assert.Equal(t, ModeNormal, m.Mode())
}

func TestInNormalMode_TrueWhenInNormalMode(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})

	assert.True(t, m.InNormalMode())
}

func TestInNormalMode_FalseWhenInInsertMode(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})

	assert.False(t, m.InNormalMode())
}

func TestInNormalMode_FalseWhenInVisualMode(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetMode(ModeVisual)

	assert.False(t, m.InNormalMode())
}

func TestInNormalMode_FalseWhenInVisualLineMode(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetMode(ModeVisualLine)

	assert.False(t, m.InNormalMode())
}

func TestInInsertMode_TrueWhenInInsertMode(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})

	assert.True(t, m.InInsertMode())
}

func TestInInsertMode_FalseWhenInNormalMode(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})

	assert.False(t, m.InInsertMode())
}

func TestInInsertMode_FalseWhenInVisualMode(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetMode(ModeVisual)

	assert.False(t, m.InInsertMode())
}

func TestInInsertMode_FalseWhenInVisualLineMode(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetMode(ModeVisualLine)

	assert.False(t, m.InInsertMode())
}

func TestInVisualMode_TrueWhenInVisualMode(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetMode(ModeVisual)

	assert.True(t, m.InVisualMode())
}

func TestInVisualMode_TrueWhenInVisualLineMode(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetMode(ModeVisualLine)

	assert.True(t, m.InVisualMode())
}

func TestInVisualMode_FalseWhenInNormalMode(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})

	assert.False(t, m.InVisualMode())
}

func TestInVisualMode_FalseWhenInInsertMode(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})

	assert.False(t, m.InVisualMode())
}

// ============================================================================
// Init, Reset, SetPlaceholder, ModeIndicator Tests
// ============================================================================

func TestInit_ReturnsNil(t *testing.T) {
	m := New(Config{})

	cmd := m.Init()

	assert.Nil(t, cmd)
}

func TestReset_ClearsContent(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello\nworld")
	m.cursorRow = 1
	m.cursorCol = 3

	m.Reset()

	assert.Equal(t, []string{""}, m.content)
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 0, m.cursorCol)
}

func TestReset_ClearsHistory(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello")
	// Trigger an undo-able action to populate history
	m, _ = m.Update(keyMsg('x')) // delete character

	m.Reset()

	// After reset, undo should have no effect (history cleared)
	assert.Equal(t, []string{""}, m.content)
}

func TestReset_ClearsPendingCommand(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello")
	m, _ = m.Update(keyMsg('d')) // Start pending 'd' command

	require.False(t, m.pendingBuilder.IsEmpty())

	m.Reset()

	assert.True(t, m.pendingBuilder.IsEmpty())
}

func TestSetPlaceholder_SetsPlaceholderText(t *testing.T) {
	m := New(Config{})

	m.SetPlaceholder("Enter text here...")

	assert.Equal(t, "Enter text here...", m.config.Placeholder)
}

func TestSetPlaceholder_OverwritesExistingPlaceholder(t *testing.T) {
	m := New(Config{Placeholder: "Old placeholder"})

	m.SetPlaceholder("New placeholder")

	assert.Equal(t, "New placeholder", m.config.Placeholder)
}

func TestModeIndicator_ReturnsEmptyWhenVimDisabled(t *testing.T) {
	m := New(Config{VimEnabled: false})

	indicator := m.ModeIndicator()

	assert.Equal(t, "", indicator)
}

func TestModeIndicator_ContainsModeNameWhenVimEnabled(t *testing.T) {
	tests := []struct {
		name     string
		mode     Mode
		expected string
	}{
		{"Normal", ModeNormal, "NORMAL"},
		{"Insert", ModeInsert, "INSERT"},
		{"Visual", ModeVisual, "VISUAL"},
		{"VisualLine", ModeVisualLine, "VISUAL LINE"},
		{"Replace", ModeReplace, "REPLACE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
			m.SetMode(tt.mode)

			indicator := m.ModeIndicator()

			assert.Contains(t, indicator, tt.expected)
			assert.Contains(t, indicator, "[")
			assert.Contains(t, indicator, "]")
		})
	}
}

func TestSetValue_ClampsCursor(t *testing.T) {
	m := New(Config{})
	// Set cursor beyond what will be valid
	m.cursorRow = 10
	m.cursorCol = 100

	m.SetValue("short")

	// Cursor should be clamped to valid position
	assert.Equal(t, 0, m.cursorRow)
	assert.LessOrEqual(t, m.cursorCol, 5)
}

// ============================================================================
// keyToString Coverage Tests
// ============================================================================

func TestKeyToString_ArrowKeys_InInsertMode(t *testing.T) {
	// Arrow keys should be handled in insert mode
	tests := []struct {
		name    string
		keyType tea.KeyType
	}{
		{"Left", tea.KeyLeft},
		{"Right", tea.KeyRight},
		{"Up", tea.KeyUp},
		{"Down", tea.KeyDown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
			m.SetValue("hello\nworld")
			m.cursorRow = 0
			m.cursorCol = 2

			// Just verify Update doesn't panic with arrow keys
			m, _ = m.Update(tea.KeyMsg{Type: tt.keyType})
			// Arrow keys in insert mode should be handled without error
			assert.NotNil(t, m)
		})
	}
}

func TestKeyToString_CtrlKeys_InInsertMode(t *testing.T) {
	tests := []struct {
		name    string
		keyType tea.KeyType
	}{
		{"CtrlA", tea.KeyCtrlA},
		{"CtrlE", tea.KeyCtrlE},
		{"CtrlK", tea.KeyCtrlK},
		{"CtrlU", tea.KeyCtrlU},
		{"CtrlF", tea.KeyCtrlF},
		{"CtrlB", tea.KeyCtrlB},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
			m.SetValue("hello world")
			m.cursorCol = 5

			// Verify Update handles these keys without panic
			m, _ = m.Update(tea.KeyMsg{Type: tt.keyType})
			assert.NotNil(t, m)
		})
	}
}

func TestKeyToString_AltEnter_SplitsLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("hello")
	m.cursorCol = 3

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter, Alt: true})

	// Alt+Enter splits the line
	assert.Equal(t, 2, len(m.content))
	assert.Equal(t, "hel", m.content[0])
	assert.Equal(t, "lo", m.content[1])
}

func TestKeyToString_CtrlJ_TriggersSubmit(t *testing.T) {
	submitted := false
	m := New(Config{
		VimEnabled:  true,
		DefaultMode: ModeInsert,
		OnSubmit: func(s string) tea.Msg {
			submitted = true
			return nil
		},
	})
	m.SetValue("hello")

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})

	// Execute the command to trigger the callback
	if cmd != nil {
		cmd()
	}
	assert.True(t, submitted)
}

func TestKeyToString_UnknownKeyType_ReturnsEmpty(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello")

	// KeyTab is not explicitly handled, should be ignored gracefully
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Content should be unchanged
	assert.Equal(t, "hello", m.Value())
}

// ============================================================================
// Mode Switching Tests (perles-nz7d.3)
// ============================================================================

func keyMsg(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func escapeKey() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyEscape}
}

func TestInsertMode_i_SwitchesToInsertMode(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	require.Equal(t, ModeNormal, m.Mode())

	m, cmd := m.Update(keyMsg('i'))

	assert.Equal(t, ModeInsert, m.Mode())
	assert.NotNil(t, cmd)
}

func TestInsertMode_a_MovesRightThenSwitchesToInsert(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello")
	m.cursorCol = 2 // Cursor at 'l'

	m, cmd := m.Update(keyMsg('a'))

	assert.Equal(t, ModeInsert, m.Mode())
	assert.Equal(t, 3, m.cursorCol) // Cursor moved right
	assert.NotNil(t, cmd)
}

func TestInsertMode_a_AtEndOfLine_StaysAtEnd(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello")
	m.cursorCol = 5 // Cursor at end of line

	m, cmd := m.Update(keyMsg('a'))

	assert.Equal(t, ModeInsert, m.Mode())
	assert.Equal(t, 5, m.cursorCol) // Cursor stays at end (can't go further)
	assert.NotNil(t, cmd)
}

func TestInsertMode_A_MovesToEndOfLineThenSwitchesToInsert(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello world")
	m.cursorCol = 2

	m, cmd := m.Update(keyMsg('A'))

	assert.Equal(t, ModeInsert, m.Mode())
	assert.Equal(t, 11, m.cursorCol) // Cursor at end of line
	assert.NotNil(t, cmd)
}

func TestInsertMode_I_MovesToFirstNonBlankThenSwitchesToInsert(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("   hello") // 3 spaces before hello
	m.cursorCol = 6

	m, cmd := m.Update(keyMsg('I'))

	assert.Equal(t, ModeInsert, m.Mode())
	assert.Equal(t, 3, m.cursorCol) // Cursor at first non-blank (the 'h')
	assert.NotNil(t, cmd)
}

func TestInsertMode_I_OnEmptyLine_StaysAtStart(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("")

	m, cmd := m.Update(keyMsg('I'))

	assert.Equal(t, ModeInsert, m.Mode())
	assert.Equal(t, 0, m.cursorCol)
	assert.NotNil(t, cmd)
}

func TestInsertMode_I_OnWhitespaceOnlyLine_StaysAtStart(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("     ") // Only whitespace

	m, cmd := m.Update(keyMsg('I'))

	assert.Equal(t, ModeInsert, m.Mode())
	assert.Equal(t, 0, m.cursorCol) // No non-blank found, stays at start
	assert.NotNil(t, cmd)
}

func TestInsertMode_o_InsertsLineBelowAndSwitchesToInsert(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("line1\nline2")
	m.cursorRow = 0

	m, cmd := m.Update(keyMsg('o'))

	assert.Equal(t, ModeInsert, m.Mode())
	assert.Equal(t, 1, m.cursorRow)        // Cursor on new line
	assert.Equal(t, 0, m.cursorCol)        // At start of new line
	assert.Equal(t, 3, len(m.content))     // Now 3 lines
	assert.Equal(t, "line1", m.content[0]) // Original first line
	assert.Equal(t, "", m.content[1])      // New empty line
	assert.Equal(t, "line2", m.content[2]) // Original second line moved down
	assert.NotNil(t, cmd)
}

func TestInsertMode_o_AtLastLine_AppendsCorrectly(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("only line")
	m.cursorRow = 0

	m, cmd := m.Update(keyMsg('o'))

	assert.Equal(t, ModeInsert, m.Mode())
	assert.Equal(t, 1, m.cursorRow)
	assert.Equal(t, 2, len(m.content))
	assert.Equal(t, "only line", m.content[0])
	assert.Equal(t, "", m.content[1])
	assert.NotNil(t, cmd)
}

func TestInsertMode_O_InsertsLineAboveAndSwitchesToInsert(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("line1\nline2")
	m.cursorRow = 1

	m, cmd := m.Update(keyMsg('O'))

	assert.Equal(t, ModeInsert, m.Mode())
	assert.Equal(t, 1, m.cursorRow)        // Cursor on new line (same row number)
	assert.Equal(t, 0, m.cursorCol)        // At start of new line
	assert.Equal(t, 3, len(m.content))     // Now 3 lines
	assert.Equal(t, "line1", m.content[0]) // Original first line
	assert.Equal(t, "", m.content[1])      // New empty line
	assert.Equal(t, "line2", m.content[2]) // Original second line moved down
	assert.NotNil(t, cmd)
}

func TestInsertMode_O_AtFirstLine_PrependsCorrectly(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("first line")
	m.cursorRow = 0

	m, cmd := m.Update(keyMsg('O'))

	assert.Equal(t, ModeInsert, m.Mode())
	assert.Equal(t, 0, m.cursorRow) // Cursor on new first line
	assert.Equal(t, 2, len(m.content))
	assert.Equal(t, "", m.content[0])           // New empty line at top
	assert.Equal(t, "first line", m.content[1]) // Original line moved down
	assert.NotNil(t, cmd)
}

func TestESC_InInsertMode_SwitchesToNormal_Consumed(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.cursorCol = 3

	m, cmd := m.Update(escapeKey())

	assert.Equal(t, ModeNormal, m.Mode())
	// Cursor should move back one position in vim
	assert.Equal(t, 2, m.cursorCol)
	assert.NotNil(t, cmd) // ModeChangeMsg is emitted
}

func TestESC_InInsertMode_AtStartOfLine_StaysAtStart(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.cursorCol = 0

	m, cmd := m.Update(escapeKey())

	assert.Equal(t, ModeNormal, m.Mode())
	assert.Equal(t, 0, m.cursorCol) // Can't go negative
	assert.NotNil(t, cmd)
}

func TestESC_InNormalMode_ReturnsUnconsumed(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	initialMode := m.Mode()

	m, cmd := m.Update(escapeKey())

	// Mode should not change
	assert.Equal(t, initialMode, m.Mode())
	// No command should be emitted (ESC passes through)
	assert.Nil(t, cmd)
}

func TestModeChangeMsg_EmittedOnModeSwitch(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})

	m, cmd := m.Update(keyMsg('i'))

	require.NotNil(t, cmd)
	msg := cmd()
	modeChangeMsg, ok := msg.(ModeChangeMsg)
	require.True(t, ok, "Expected ModeChangeMsg, got %T", msg)
	assert.Equal(t, ModeInsert, modeChangeMsg.Mode)
	assert.Equal(t, ModeNormal, modeChangeMsg.Previous)
}

func TestModeChangeMsg_WithCustomCallback(t *testing.T) {
	type CustomMsg struct {
		CurrentMode  Mode
		PreviousMode Mode
	}

	callbackCalled := false
	m := New(Config{
		VimEnabled:  true,
		DefaultMode: ModeNormal,
		OnModeChange: func(mode, previous Mode) tea.Msg {
			callbackCalled = true
			return CustomMsg{CurrentMode: mode, PreviousMode: previous}
		},
	})

	m, cmd := m.Update(keyMsg('i'))

	require.NotNil(t, cmd)
	msg := cmd()
	customMsg, ok := msg.(CustomMsg)
	require.True(t, ok, "Expected CustomMsg, got %T", msg)
	assert.Equal(t, ModeInsert, customMsg.CurrentMode)
	assert.Equal(t, ModeNormal, customMsg.PreviousMode)
	assert.True(t, callbackCalled)
}

func TestPendingCommand_ClearedOnInvalidSequence(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.pendingBuilder.SetOperator('d') // Simulate partial delete command

	// 'd' + 'i' is a valid prefix for 'diw', so pending should NOT be cleared
	m, _ = m.Update(keyMsg('i'))
	assert.False(t, m.pendingBuilder.IsEmpty(), "d+i should buffer for potential text object")

	// 'd' + 'i' + 'z' is not a valid sequence, so pending should be cleared
	m, _ = m.Update(keyMsg('z'))
	assert.True(t, m.pendingBuilder.IsEmpty(), "invalid sequence should clear pending state")
}

func TestModeSwitching_VimDisabled_NoModeChanges(t *testing.T) {
	m := New(Config{VimEnabled: false})
	initialMode := m.Mode()

	// Try to switch to normal mode with ESC - should stay in Insert mode (vim disabled)
	m, _ = m.Update(escapeKey())
	assert.Equal(t, initialMode, m.Mode())

	// 'i' should type the character 'i', not switch modes (vim is disabled)
	m, _ = m.Update(keyMsg('i'))
	assert.Equal(t, initialMode, m.Mode())
	// Character 'i' was inserted
	assert.Equal(t, "i", m.Value())
}

func TestRapidModeSwitch_i_then_ESC(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})

	// Enter insert mode
	m, _ = m.Update(keyMsg('i'))
	assert.Equal(t, ModeInsert, m.Mode())

	// Immediately exit
	m, _ = m.Update(escapeKey())
	assert.Equal(t, ModeNormal, m.Mode())
}

// ============================================================================
// Character Motion Tests (perles-nz7d.4)
// ============================================================================

func TestMotion_h_MovesLeft(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello")
	m.cursorCol = 3 // Cursor at 'l'

	m, _ = m.Update(keyMsg('h'))

	assert.Equal(t, 2, m.cursorCol) // Moved left by one
}

func TestMotion_h_AtColumnZero_StaysAtZero(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello")
	m.cursorCol = 0 // Already at start

	m, _ = m.Update(keyMsg('h'))

	assert.Equal(t, 0, m.cursorCol) // Should not go negative
}

func TestMotion_l_MovesRight(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello")
	m.cursorCol = 2 // Cursor at second 'l'

	m, _ = m.Update(keyMsg('l'))

	assert.Equal(t, 3, m.cursorCol) // Moved right by one
}

func TestMotion_l_AtEndOfLine_StaysAtEnd(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello")
	m.cursorCol = 4 // At 'o' (last char, index 4 for 5-char word)

	m, _ = m.Update(keyMsg('l'))

	assert.Equal(t, 4, m.cursorCol) // Should stay at last char
}

func TestMotion_l_OnEmptyLine_StaysAtZero(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("")
	m.cursorCol = 0

	m, _ = m.Update(keyMsg('l'))

	assert.Equal(t, 0, m.cursorCol) // Can't move right on empty line
}

func TestMotion_j_MovesDown(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("line1\nline2\nline3")
	m.cursorRow = 0
	m.cursorCol = 2
	m.preferredCol = 2 // Set preferred column to match current position

	m, _ = m.Update(keyMsg('j'))

	assert.Equal(t, 1, m.cursorRow) // Moved down
	assert.Equal(t, 2, m.cursorCol) // Column preserved
}

func TestMotion_j_AtLastLine_StaysAtLastLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("line1\nline2\nline3")
	m.cursorRow = 2 // Last line

	m, _ = m.Update(keyMsg('j'))

	assert.Equal(t, 2, m.cursorRow) // Should stay at last line
}

func TestMotion_k_MovesUp(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("line1\nline2\nline3")
	m.cursorRow = 2
	m.cursorCol = 2
	m.preferredCol = 2 // Set preferred column to match current position

	m, _ = m.Update(keyMsg('k'))

	assert.Equal(t, 1, m.cursorRow) // Moved up
	assert.Equal(t, 2, m.cursorCol) // Column preserved
}

func TestMotion_k_AtFirstLine_StaysAtFirstLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("line1\nline2\nline3")
	m.cursorRow = 0 // First line

	m, _ = m.Update(keyMsg('k'))

	assert.Equal(t, 0, m.cursorRow) // Should stay at first line
}

func TestMotion_j_ToShorterLine_ClampsCursorColumn(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello world\nhi")
	m.cursorRow = 0
	m.cursorCol = 10 // At 'd' in "world"
	m.preferredCol = 10

	m, _ = m.Update(keyMsg('j'))

	assert.Equal(t, 1, m.cursorRow)
	// Line "hi" has length 2, so max col is 1 (index of 'i')
	assert.Equal(t, 1, m.cursorCol)
}

func TestMotion_k_BackToLongerLine_RestoresPreferredColumn(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello world\nhi\nhello world")
	m.cursorRow = 0
	m.cursorCol = 10 // At 'd' in first "world"
	m.preferredCol = 10

	// Move down to short line
	m, _ = m.Update(keyMsg('j'))
	assert.Equal(t, 1, m.cursorRow)
	assert.Equal(t, 1, m.cursorCol) // Clamped to "hi"

	// Move down to long line again - preferred column should be restored
	m, _ = m.Update(keyMsg('j'))
	assert.Equal(t, 2, m.cursorRow)
	assert.Equal(t, 10, m.cursorCol) // Restored to preferred
}

// Soft-wrap navigation tests (j/k move by display line, like vim's gj/gk)

func TestMotion_j_SoftWrap_MovesWithinWrappedLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	// Line longer than width should wrap
	m.SetValue("0123456789ABCDEFGHIJ") // 20 chars
	m.SetSize(10, 5)                   // Width 10, so line wraps to 2 display lines
	m.cursorCol = 3                    // On first wrap segment

	m, _ = m.Update(keyMsg('j'))

	// Should move to second wrap segment at same relative column
	assert.Equal(t, 0, m.cursorRow)  // Same logical row
	assert.Equal(t, 13, m.cursorCol) // 10 + 3 = position on second segment
}

func TestMotion_k_SoftWrap_MovesWithinWrappedLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("0123456789ABCDEFGHIJ") // 20 chars
	m.SetSize(10, 5)                   // Width 10
	m.cursorCol = 15                   // On second wrap segment (col 5 within segment)

	m, _ = m.Update(keyMsg('k'))

	// Should move to first wrap segment at same relative column
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 5, m.cursorCol) // Same column within wrap segment
}

func TestMotion_j_SoftWrap_MovesToNextLogicalLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("0123456789ABCDEFGHIJ\nshort") // First line wraps, second doesn't
	m.SetSize(10, 5)
	m.cursorCol = 15 // On second wrap segment of first line

	m, _ = m.Update(keyMsg('j'))

	// Should move to next logical line (since we're on last wrap segment)
	assert.Equal(t, 1, m.cursorRow)
	assert.Equal(t, 4, m.cursorCol) // Clamped to "short" length minus 1 (Normal mode)
}

func TestMotion_k_SoftWrap_MovesToPrevLogicalLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("0123456789ABCDEFGHIJ\nshort") // First line wraps
	m.SetSize(10, 5)
	m.cursorRow = 1
	m.cursorCol = 3

	m, _ = m.Update(keyMsg('k'))

	// Should move to last wrap segment of previous line
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 13, m.cursorCol) // Last segment (10) + col within segment (3)
}

func TestMotion_hjkl_InInsertMode_TypeCharacters(t *testing.T) {
	// In Insert mode, hjkl should type characters, NOT move the cursor as motions
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("hello")
	m.cursorRow = 0
	m.cursorCol = 2

	// Typing 'h' inserts the character and advances cursor
	m, _ = m.Update(keyMsg('h'))
	assert.Equal(t, "hehllo", m.Value())
	assert.Equal(t, 3, m.cursorCol) // Cursor advanced after insertion

	// Typing 'j' inserts the character
	m, _ = m.Update(keyMsg('j'))
	assert.Equal(t, "hehjllo", m.Value())
	assert.Equal(t, 4, m.cursorCol)

	// Typing 'k' inserts the character
	m, _ = m.Update(keyMsg('k'))
	assert.Equal(t, "hehjkllo", m.Value())
	assert.Equal(t, 5, m.cursorCol)

	// Typing 'l' inserts the character
	m, _ = m.Update(keyMsg('l'))
	assert.Equal(t, "hehjklllo", m.Value())
	assert.Equal(t, 6, m.cursorCol)
}

func TestMotion_h_UpdatesPreferredColumn(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello\nworld")
	m.cursorRow = 0
	m.cursorCol = 4
	m.preferredCol = 4

	// Move left
	m, _ = m.Update(keyMsg('h'))
	assert.Equal(t, 3, m.cursorCol)
	assert.Equal(t, 3, m.preferredCol) // Preferred column updated
}

func TestMotion_l_UpdatesPreferredColumn(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello\nworld")
	m.cursorRow = 0
	m.cursorCol = 2
	m.preferredCol = 2

	// Move right
	m, _ = m.Update(keyMsg('l'))
	assert.Equal(t, 3, m.cursorCol)
	assert.Equal(t, 3, m.preferredCol) // Preferred column updated
}

// ============================================================================
// Word Motion Tests (perles-nz7d.5)
// ============================================================================

func TestMotion_w_MovesToNextWordStart(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello world")
	m.cursorCol = 0 // At 'h'

	m, _ = m.Update(keyMsg('w'))

	assert.Equal(t, 6, m.cursorCol) // At 'w' of "world"
}

func TestMotion_w_FromMiddleOfWord(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello world")
	m.cursorCol = 2 // At 'l' in "hello"

	m, _ = m.Update(keyMsg('w'))

	assert.Equal(t, 6, m.cursorCol) // At 'w' of "world"
}

func TestMotion_w_SkipsPunctuation(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello, world")
	m.cursorCol = 0 // At 'h'

	m, _ = m.Update(keyMsg('w'))
	assert.Equal(t, 5, m.cursorCol) // At ','

	m, _ = m.Update(keyMsg('w'))
	assert.Equal(t, 7, m.cursorCol) // At 'w' of "world"
}

func TestMotion_w_FromLastWordOnLine_MovesToNextLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello\nworld")
	m.cursorRow = 0
	m.cursorCol = 0 // At 'h'

	m, _ = m.Update(keyMsg('w'))
	// Should wrap to next line since "hello" is the only word on line 0
	assert.Equal(t, 1, m.cursorRow) // On line 1
	assert.Equal(t, 0, m.cursorCol) // At 'w' of "world"
}

func TestMotion_w_AtEndOfContent_StaysAtEnd(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello")
	m.cursorCol = 4 // At 'o' (last char)

	m, _ = m.Update(keyMsg('w'))

	// Should stay at end since there's no next word
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 4, m.cursorCol)
}

func TestMotion_w_MultipleWordsOnLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("one two three")
	m.cursorCol = 0

	m, _ = m.Update(keyMsg('w'))
	assert.Equal(t, 4, m.cursorCol) // At 't' of "two"

	m, _ = m.Update(keyMsg('w'))
	assert.Equal(t, 8, m.cursorCol) // At 't' of "three"
}

func TestMotion_b_MovesToPrevWordStart(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello world")
	m.cursorCol = 6 // At 'w' of "world"

	m, _ = m.Update(keyMsg('b'))

	assert.Equal(t, 0, m.cursorCol) // At 'h' of "hello"
}

func TestMotion_b_FromMiddleOfWord(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello world")
	m.cursorCol = 8 // At 'r' in "world"

	m, _ = m.Update(keyMsg('b'))

	assert.Equal(t, 6, m.cursorCol) // At 'w' of "world"
}

func TestMotion_b_SkipsPunctuation(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello, world")
	m.cursorCol = 7 // At 'w' of "world"

	m, _ = m.Update(keyMsg('b'))
	assert.Equal(t, 5, m.cursorCol) // At ','

	m, _ = m.Update(keyMsg('b'))
	assert.Equal(t, 0, m.cursorCol) // At 'h' of "hello"
}

func TestMotion_b_FromFirstWordOnLine_MovesToPrevLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello\nworld")
	m.cursorRow = 1
	m.cursorCol = 0 // At 'w' of "world"

	m, _ = m.Update(keyMsg('b'))

	assert.Equal(t, 0, m.cursorRow) // On line 0
	assert.Equal(t, 0, m.cursorCol) // At 'h' of "hello"
}

func TestMotion_b_AtStartOfContent_StaysAtStart(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello")
	m.cursorCol = 0 // At 'h'

	m, _ = m.Update(keyMsg('b'))

	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 0, m.cursorCol)
}

func TestMotion_e_MovesToWordEnd(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello world")
	m.cursorCol = 0 // At 'h'

	m, _ = m.Update(keyMsg('e'))

	assert.Equal(t, 4, m.cursorCol) // At 'o' of "hello"
}

func TestMotion_e_FromMiddleOfWord(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello world")
	m.cursorCol = 2 // At 'l' in "hello"

	m, _ = m.Update(keyMsg('e'))

	assert.Equal(t, 4, m.cursorCol) // At 'o' of "hello"
}

func TestMotion_e_FromEndOfWord_MovesToNextWordEnd(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello world")
	m.cursorCol = 4 // At 'o' of "hello" (end of word)

	m, _ = m.Update(keyMsg('e'))

	assert.Equal(t, 10, m.cursorCol) // At 'd' of "world"
}

func TestMotion_e_AtEndOfLine_MovesToNextLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello\nworld")
	m.cursorRow = 0
	m.cursorCol = 4 // At 'o' of "hello"

	m, _ = m.Update(keyMsg('e'))

	assert.Equal(t, 1, m.cursorRow) // On line 1
	assert.Equal(t, 4, m.cursorCol) // At 'd' of "world"
}

func TestMotion_e_AtEndOfContent_StaysAtEnd(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello")
	m.cursorCol = 4 // At 'o' (last char)

	m, _ = m.Update(keyMsg('e'))

	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 4, m.cursorCol)
}

func TestMotion_e_WithPunctuation(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello,world")
	m.cursorCol = 0 // At 'h'

	m, _ = m.Update(keyMsg('e'))
	assert.Equal(t, 4, m.cursorCol) // At 'o' of "hello"

	m, _ = m.Update(keyMsg('e'))
	assert.Equal(t, 5, m.cursorCol) // At ',' (punctuation is a word)

	m, _ = m.Update(keyMsg('e'))
	assert.Equal(t, 10, m.cursorCol) // At 'd' of "world"
}

// Edge cases

func TestMotion_w_EmptyLine_SkipsToNextLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello\n\nworld")
	m.cursorRow = 0
	m.cursorCol = 0

	m, _ = m.Update(keyMsg('w'))
	// Skips past "hello" to empty line, then to "world"
	assert.Equal(t, 1, m.cursorRow)
	assert.Equal(t, 0, m.cursorCol)

	m, _ = m.Update(keyMsg('w'))
	assert.Equal(t, 2, m.cursorRow)
	assert.Equal(t, 0, m.cursorCol)
}

func TestMotion_b_EmptyLine_SkipsToPrevLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello\n\nworld")
	m.cursorRow = 2
	m.cursorCol = 0 // At 'w' of "world"

	m, _ = m.Update(keyMsg('b'))
	// Should skip empty line and go to "hello"
	assert.Equal(t, 1, m.cursorRow)
	assert.Equal(t, 0, m.cursorCol)

	m, _ = m.Update(keyMsg('b'))
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 0, m.cursorCol)
}

func TestMotion_e_EmptyLine_SkipsToNextLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello\n\nworld")
	m.cursorRow = 0
	m.cursorCol = 4 // At 'o' of "hello"

	// First 'e' from end of "hello" goes to empty line
	m, _ = m.Update(keyMsg('e'))
	assert.Equal(t, 1, m.cursorRow)
	assert.Equal(t, 0, m.cursorCol)

	// Second 'e' from empty line goes to end of "world"
	m, _ = m.Update(keyMsg('e'))
	assert.Equal(t, 2, m.cursorRow)
	assert.Equal(t, 4, m.cursorCol) // At 'd' of "world"
}

func TestMotion_w_SingleCharacterWords(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("a b c")
	m.cursorCol = 0 // At 'a'

	m, _ = m.Update(keyMsg('w'))
	assert.Equal(t, 2, m.cursorCol) // At 'b'

	m, _ = m.Update(keyMsg('w'))
	assert.Equal(t, 4, m.cursorCol) // At 'c'
}

func TestMotion_e_SingleCharacterWords(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("a b c")
	m.cursorCol = 0 // At 'a'

	// From position 0 ('a'), 'e' moves to next word end since 'a' is already at word end
	m, _ = m.Update(keyMsg('e'))
	assert.Equal(t, 2, m.cursorCol) // At 'b'

	m, _ = m.Update(keyMsg('e'))
	assert.Equal(t, 4, m.cursorCol) // At 'c'
}

func TestMotion_w_MixedAlphanumericAndPunctuation(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("foo_bar(baz)")
	m.cursorCol = 0 // At 'f'

	m, _ = m.Update(keyMsg('w'))
	assert.Equal(t, 7, m.cursorCol) // At '('

	m, _ = m.Update(keyMsg('w'))
	assert.Equal(t, 8, m.cursorCol) // At 'b' of "baz"

	m, _ = m.Update(keyMsg('w'))
	assert.Equal(t, 11, m.cursorCol) // At ')'
}

func TestMotion_b_MixedAlphanumericAndPunctuation(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("foo_bar(baz)")
	m.cursorCol = 11 // At ')'

	m, _ = m.Update(keyMsg('b'))
	assert.Equal(t, 8, m.cursorCol) // At 'b' of "baz"

	m, _ = m.Update(keyMsg('b'))
	assert.Equal(t, 7, m.cursorCol) // At '('

	m, _ = m.Update(keyMsg('b'))
	assert.Equal(t, 0, m.cursorCol) // At 'f' of "foo_bar"
}

func TestMotion_wbe_InInsertMode_TypeCharacters(t *testing.T) {
	// In Insert mode, w/b/e should type characters, NOT move the cursor as motions
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("hello world")
	m.cursorCol = 0

	// Typing 'w' inserts the character and advances cursor
	m, _ = m.Update(keyMsg('w'))
	assert.Equal(t, "whello world", m.Value())
	assert.Equal(t, 1, m.cursorCol)

	// Typing 'b' inserts the character
	m, _ = m.Update(keyMsg('b'))
	assert.Equal(t, "wbhello world", m.Value())
	assert.Equal(t, 2, m.cursorCol)

	// Typing 'e' inserts the character
	m, _ = m.Update(keyMsg('e'))
	assert.Equal(t, "wbehello world", m.Value())
	assert.Equal(t, 3, m.cursorCol)
}

func TestMotion_w_UpdatesPreferredColumn(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello world")
	m.cursorCol = 0
	m.preferredCol = 0

	m, _ = m.Update(keyMsg('w'))
	assert.Equal(t, 6, m.cursorCol)
	assert.Equal(t, 6, m.preferredCol) // Preferred column updated
}

func TestMotion_b_UpdatesPreferredColumn(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello world")
	m.cursorCol = 6
	m.preferredCol = 6

	m, _ = m.Update(keyMsg('b'))
	assert.Equal(t, 0, m.cursorCol)
	assert.Equal(t, 0, m.preferredCol) // Preferred column updated
}

func TestMotion_e_UpdatesPreferredColumn(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello world")
	m.cursorCol = 0
	m.preferredCol = 0

	m, _ = m.Update(keyMsg('e'))
	assert.Equal(t, 4, m.cursorCol)
	assert.Equal(t, 4, m.preferredCol) // Preferred column updated
}

// ============================================================================
// Line Motion Tests (perles-nz7d.6)
// ============================================================================

func TestMotion_0_MovesToColumnZero(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello world")
	m.cursorCol = 6 // At 'w'

	m, _ = m.Update(keyMsg('0'))

	assert.Equal(t, 0, m.cursorCol) // Moved to column 0
	assert.Equal(t, 0, m.preferredCol)
}

func TestMotion_0_AtColumnZero_StaysAtZero(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello world")
	m.cursorCol = 0

	m, _ = m.Update(keyMsg('0'))

	assert.Equal(t, 0, m.cursorCol)
}

func TestMotion_0_OnEmptyLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("")
	m.cursorCol = 0

	m, _ = m.Update(keyMsg('0'))

	assert.Equal(t, 0, m.cursorCol)
}

func TestMotion_Dollar_MovesToEndOfLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello")
	m.cursorCol = 0

	m, _ = m.Update(keyMsg('$'))

	assert.Equal(t, 4, m.cursorCol) // At 'o' (last char, index 4)
	assert.Equal(t, 4, m.preferredCol)
}

func TestMotion_Dollar_OnEmptyLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("")
	m.cursorCol = 0

	m, _ = m.Update(keyMsg('$'))

	assert.Equal(t, 0, m.cursorCol) // Empty line, stays at 0
}

func TestMotion_Dollar_AlreadyAtEnd(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello")
	m.cursorCol = 4 // Already at 'o'

	m, _ = m.Update(keyMsg('$'))

	assert.Equal(t, 4, m.cursorCol) // Stays at end
}

func TestMotion_Caret_MovesToFirstNonBlank(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("   hello")
	m.cursorCol = 6 // At 'l'

	m, _ = m.Update(keyMsg('^'))

	assert.Equal(t, 3, m.cursorCol) // At 'h' (first non-blank)
	assert.Equal(t, 3, m.preferredCol)
}

func TestMotion_Caret_NoLeadingWhitespace(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello")
	m.cursorCol = 3

	m, _ = m.Update(keyMsg('^'))

	assert.Equal(t, 0, m.cursorCol) // First char is non-blank
}

func TestMotion_Caret_WhitespaceOnlyLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("     ")
	m.cursorCol = 3

	m, _ = m.Update(keyMsg('^'))

	assert.Equal(t, 0, m.cursorCol) // No non-blank, goes to 0
}

func TestMotion_Caret_OnEmptyLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("")
	m.cursorCol = 0

	m, _ = m.Update(keyMsg('^'))

	assert.Equal(t, 0, m.cursorCol)
}

func TestMotion_Caret_MixedWhitespace(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("\t  hello") // Tab + 2 spaces before hello
	m.cursorCol = 5

	m, _ = m.Update(keyMsg('^'))

	assert.Equal(t, 3, m.cursorCol) // At 'h'
}

func TestMotion_gg_MovesToFirstLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("line1\nline2\nline3")
	m.cursorRow = 2
	m.cursorCol = 2

	// First 'g' sets pending command
	m, _ = m.Update(keyMsg('g'))
	assert.Equal(t, 'g', m.pendingBuilder.Operator())

	// Second 'g' executes gg
	m, _ = m.Update(keyMsg('g'))

	assert.Equal(t, 0, m.cursorRow)
	assert.True(t, m.pendingBuilder.IsEmpty()) // Cleared
}

func TestMotion_gg_AtFirstLine_StaysAtFirstLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("line1\nline2\nline3")
	m.cursorRow = 0

	m, _ = m.Update(keyMsg('g'))
	m, _ = m.Update(keyMsg('g'))

	assert.Equal(t, 0, m.cursorRow)
}

func TestMotion_G_MovesToLastLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("line1\nline2\nline3")
	m.cursorRow = 0
	m.cursorCol = 2

	m, _ = m.Update(keyMsg('G'))

	assert.Equal(t, 2, m.cursorRow) // Last line (0-indexed)
}

func TestMotion_G_AtLastLine_StaysAtLastLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("line1\nline2\nline3")
	m.cursorRow = 2

	m, _ = m.Update(keyMsg('G'))

	assert.Equal(t, 2, m.cursorRow)
}

func TestMotion_G_SingleLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("only line")
	m.cursorRow = 0

	m, _ = m.Update(keyMsg('G'))

	assert.Equal(t, 0, m.cursorRow)
}

func TestMotion_g_FollowedByNonG_ClearsPending(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("line1\nline2\nline3")
	m.cursorRow = 2
	initialRow := m.cursorRow

	// 'g' starts pending command
	m, _ = m.Update(keyMsg('g'))
	assert.Equal(t, 'g', m.pendingBuilder.Operator())

	// 'x' should clear pending (not a valid g-command)
	m, _ = m.Update(keyMsg('x'))

	assert.True(t, m.pendingBuilder.IsEmpty()) // Cleared
	assert.Equal(t, initialRow, m.cursorRow)   // Position unchanged
}

func TestMotion_g_Alone_WaitsForNextKey(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("line1\nline2\nline3")
	m.cursorRow = 2

	m, _ = m.Update(keyMsg('g'))

	assert.Equal(t, 'g', m.pendingBuilder.Operator())
	// Position should not change yet
	assert.Equal(t, 2, m.cursorRow)
}

func TestMotion_gg_ClampsCursorColumn(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hi\nlong line here")
	m.cursorRow = 1
	m.cursorCol = 10 // Valid on long line
	m.preferredCol = 10

	m, _ = m.Update(keyMsg('g'))
	m, _ = m.Update(keyMsg('g'))

	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 1, m.cursorCol) // Clamped to "hi" length - 1
}

func TestMotion_G_ClampsCursorColumn(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("long line here\nhi")
	m.cursorRow = 0
	m.cursorCol = 10
	m.preferredCol = 10

	m, _ = m.Update(keyMsg('G'))

	assert.Equal(t, 1, m.cursorRow)
	assert.Equal(t, 1, m.cursorCol) // Clamped to "hi" length - 1
}

func TestMotion_LineMotions_InInsertMode_TypeCharacters(t *testing.T) {
	// In Insert mode, 0, $, ^ should type characters, NOT move the cursor as motions
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("hello")
	m.cursorCol = 2

	// Typing '0' inserts the character and advances cursor
	m, _ = m.Update(keyMsg('0'))
	assert.Equal(t, "he0llo", m.Value())
	assert.Equal(t, 3, m.cursorCol)

	// Typing '$' inserts the character
	m, _ = m.Update(keyMsg('$'))
	assert.Equal(t, "he0$llo", m.Value())
	assert.Equal(t, 4, m.cursorCol)

	// Typing '^' inserts the character
	m, _ = m.Update(keyMsg('^'))
	assert.Equal(t, "he0$^llo", m.Value())
	assert.Equal(t, 5, m.cursorCol)
}

func TestMotion_G_InInsertMode_TypesCharacter(t *testing.T) {
	// In Insert mode, 'G' should type the character, NOT move to last line
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("line1\nline2")
	m.cursorRow = 0
	m.cursorCol = 0

	m, _ = m.Update(keyMsg('G'))

	// 'G' was typed at the start of line1
	assert.Equal(t, "Gline1\nline2", m.Value())
	assert.Equal(t, 0, m.cursorRow) // Row unchanged, just typed 'G'
	assert.Equal(t, 1, m.cursorCol) // Cursor advanced after typing
}

func TestPendingCommand_ClearedOnBlur(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("test")

	m, _ = m.Update(keyMsg('g'))
	assert.Equal(t, 'g', m.pendingBuilder.Operator())

	m.Blur()
	assert.True(t, m.pendingBuilder.IsEmpty())
}

// ============================================================================
// Delete Operation Tests (perles-nz7d.7)
// ============================================================================

func TestDelete_x_DeletesCharacterUnderCursor(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello")
	m.cursorCol = 2 // At 'l'

	m, cmd := m.Update(keyMsg('x'))

	assert.Equal(t, "helo", m.Value())
	assert.Equal(t, 2, m.cursorCol) // Cursor stays at same position
	// OnChange callback was not set, so cmd should be nil
	assert.Nil(t, cmd)
}

func TestDelete_x_AtEndOfLine_DeletesLastCharacter(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello")
	m.cursorCol = 4 // At 'o' (last char)

	m, _ = m.Update(keyMsg('x'))

	assert.Equal(t, "hell", m.Value())
	assert.Equal(t, 3, m.cursorCol) // Cursor moves back since we deleted the last char
}

func TestDelete_x_OnEmptyLine_DoesNothing(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("")
	m.cursorCol = 0

	m, _ = m.Update(keyMsg('x'))

	assert.Equal(t, "", m.Value())
	assert.Equal(t, 0, m.cursorCol)
}

func TestDelete_x_SingleCharLine_LeavesEmptyLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("a")
	m.cursorCol = 0

	m, _ = m.Update(keyMsg('x'))

	assert.Equal(t, "", m.Value())
	assert.Equal(t, 0, m.cursorCol)
}

func TestDelete_dd_DeletesEntireLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("line1\nline2\nline3")
	m.cursorRow = 1 // On "line2"
	m.cursorCol = 2

	// Press d twice for dd
	m, _ = m.Update(keyMsg('d'))
	assert.Equal(t, 'd', m.pendingBuilder.Operator())

	m, _ = m.Update(keyMsg('d'))

	assert.Equal(t, "line1\nline3", m.Value())
	assert.Equal(t, 1, m.cursorRow) // Still on row 1, now pointing to "line3"
	assert.True(t, m.pendingBuilder.IsEmpty())
}

func TestDelete_dd_OnSingleLine_LeavesEmptyContent(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("only line")
	m.cursorRow = 0

	m, _ = m.Update(keyMsg('d'))
	m, _ = m.Update(keyMsg('d'))

	assert.Equal(t, "", m.Value())
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 0, m.cursorCol)
}

func TestDelete_dd_OnLastLine_MovesCursorUp(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("line1\nline2\nline3")
	m.cursorRow = 2 // On "line3"

	m, _ = m.Update(keyMsg('d'))
	m, _ = m.Update(keyMsg('d'))

	assert.Equal(t, "line1\nline2", m.Value())
	assert.Equal(t, 1, m.cursorRow) // Cursor moved up to previous line
}

func TestDelete_D_DeletesFromCursorToEndOfLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello world")
	m.cursorCol = 5 // At space

	m, _ = m.Update(keyMsg('D'))

	assert.Equal(t, "hello", m.Value())
	assert.Equal(t, 4, m.cursorCol) // Cursor on last remaining char
}

func TestDelete_D_AtStartOfLine_DeletesEntireLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello world")
	m.cursorCol = 0

	m, _ = m.Update(keyMsg('D'))

	assert.Equal(t, "", m.Value())
	assert.Equal(t, 0, m.cursorCol)
}

func TestDelete_D_AtEndOfLine_DoesNothing(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello")
	m.cursorCol = 5 // Past end

	initialValue := m.Value()
	m, _ = m.Update(keyMsg('D'))

	assert.Equal(t, initialValue, m.Value())
}

func TestDelete_dDollar_EquivalentToD(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello world")
	m.cursorCol = 5

	m, _ = m.Update(keyMsg('d'))
	m, _ = m.Update(keyMsg('$'))

	assert.Equal(t, "hello", m.Value())
	assert.Equal(t, 4, m.cursorCol)
}

func TestDelete_dw_DeletesWord(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello world")
	m.cursorCol = 0 // At 'h'

	m, _ = m.Update(keyMsg('d'))
	m, _ = m.Update(keyMsg('w'))

	assert.Equal(t, "world", m.Value())
	assert.Equal(t, 0, m.cursorCol)
}

func TestDelete_dw_FromMiddleOfWord(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello world")
	m.cursorCol = 2 // At 'l' in "hello"

	m, _ = m.Update(keyMsg('d'))
	m, _ = m.Update(keyMsg('w'))

	assert.Equal(t, "heworld", m.Value())
	assert.Equal(t, 2, m.cursorCol)
}

func TestDelete_dw_AtLastWord_DeletesWordAndTrailingWhitespace(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello world")
	m.cursorCol = 6 // At 'w' of "world"

	m, _ = m.Update(keyMsg('d'))
	m, _ = m.Update(keyMsg('w'))

	assert.Equal(t, "hello ", m.Value())
	assert.Equal(t, 5, m.cursorCol) // Cursor at space (which is now last char)
}

func TestDelete_dw_OnEmptyLine_DoesNothing(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("")
	m.cursorCol = 0

	m, _ = m.Update(keyMsg('d'))
	m, _ = m.Update(keyMsg('w'))

	assert.Equal(t, "", m.Value())
}

func TestDelete_d_AloneEntersPendingState(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello")

	m, _ = m.Update(keyMsg('d'))

	assert.Equal(t, 'd', m.pendingBuilder.Operator())
	assert.Equal(t, "hello", m.Value()) // No change yet
}

func TestDelete_d_FollowedByNonMotion_ClearsPending(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello")

	m, _ = m.Update(keyMsg('d'))
	m, _ = m.Update(keyMsg('x')) // 'x' is not a valid d-motion

	assert.True(t, m.pendingBuilder.IsEmpty())
	assert.Equal(t, "hello", m.Value()) // No deletion occurred
}

func TestDelete_dj_DeletesCurrentAndNextLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("line1\nline2\nline3")
	m.cursorRow = 0

	m, _ = m.Update(keyMsg('d'))
	m, _ = m.Update(keyMsg('j'))

	assert.Equal(t, "line3", m.Value())
	assert.Equal(t, 0, m.cursorRow)
}

func TestDelete_dk_DeletesCurrentAndPreviousLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("line1\nline2\nline3")
	m.cursorRow = 2 // On "line3"

	m, _ = m.Update(keyMsg('d'))
	m, _ = m.Update(keyMsg('k'))

	assert.Equal(t, "line1", m.Value())
	assert.Equal(t, 0, m.cursorRow)
}

func TestDelete_dk_AtFirstLine_DeletesOnlyCurrentLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("line1\nline2")
	m.cursorRow = 0

	m, _ = m.Update(keyMsg('d'))
	m, _ = m.Update(keyMsg('k'))

	assert.Equal(t, "line2", m.Value())
	assert.Equal(t, 0, m.cursorRow)
}

func TestDelete_UndoHistoryPopulatedBeforeDelete(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello")
	m.cursorCol = 0

	assert.False(t, m.CanUndo()) // No undo states initially

	m, _ = m.Update(keyMsg('x')) // Delete 'h'

	assert.True(t, m.CanUndo())
	// Verify undo works correctly - restores original content and cursor
	m, _ = m.Update(keyMsg('u'))
	assert.Equal(t, "hello", m.Value())
	assert.Equal(t, 0, m.cursorCol)
}

func TestDelete_OnChangeCallback_Triggered(t *testing.T) {
	type ChangeMsg struct {
		Content string
	}

	callbackCalled := false
	var changedContent string

	m := New(Config{
		VimEnabled: true,
		OnChange: func(content string) tea.Msg {
			callbackCalled = true
			changedContent = content
			return ChangeMsg{Content: content}
		},
	})
	m.SetValue("hello")
	m.cursorCol = 0

	m, cmd := m.Update(keyMsg('x')) // Delete 'h'

	require.NotNil(t, cmd)
	msg := cmd()
	changeMsg, ok := msg.(ChangeMsg)
	require.True(t, ok, "Expected ChangeMsg, got %T", msg)

	assert.True(t, callbackCalled)
	assert.Equal(t, "ello", changedContent)
	assert.Equal(t, "ello", changeMsg.Content)
}

func TestDelete_MultipleUndosWork(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello world")
	m.cursorCol = 0

	// First delete
	m, _ = m.Update(keyMsg('x'))
	assert.True(t, m.CanUndo())

	// Second delete
	m, _ = m.Update(keyMsg('x'))
	assert.True(t, m.CanUndo())

	// Third delete
	m, _ = m.Update(keyMsg('x'))
	assert.True(t, m.CanUndo())

	assert.Equal(t, "lo world", m.Value())

	// Verify all three undos work
	m, _ = m.Update(keyMsg('u'))
	assert.Equal(t, "llo world", m.Value())
	m, _ = m.Update(keyMsg('u'))
	assert.Equal(t, "ello world", m.Value())
	m, _ = m.Update(keyMsg('u'))
	assert.Equal(t, "hello world", m.Value())
}

func TestDelete_dw_WithPunctuation(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello, world")
	m.cursorCol = 0 // At 'h'

	m, _ = m.Update(keyMsg('d'))
	m, _ = m.Update(keyMsg('w'))

	// Deletes "hello" up to (but not including) ","
	assert.Equal(t, ", world", m.Value())
}

func TestDelete_InInsertMode_TypesCharacters(t *testing.T) {
	// In Insert mode, x, d, D are just regular characters that get typed
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("hello")
	m.cursorCol = 2

	// 'x' should type 'x', not delete
	m, _ = m.Update(keyMsg('x'))
	assert.Equal(t, "hexllo", m.Value())
	assert.Equal(t, 3, m.cursorCol)

	// 'D' should type 'D', not delete to end of line
	m, _ = m.Update(keyMsg('D'))
	assert.Equal(t, "hexDllo", m.Value())
	assert.Equal(t, 4, m.cursorCol)

	// 'd' should type 'd', not start delete operator
	m, _ = m.Update(keyMsg('d'))
	assert.Equal(t, "hexDdllo", m.Value())
	assert.Equal(t, 5, m.cursorCol)
	assert.True(t, m.pendingBuilder.IsEmpty()) // No pending command in Insert mode
}

func TestDelete_dj_AtLastLine_DeletesOnlyCurrentLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("line1\nline2")
	m.cursorRow = 1 // On "line2" (last line)

	m, _ = m.Update(keyMsg('d'))
	m, _ = m.Update(keyMsg('j'))

	// dj at last line should delete just the current line (like dd but with 2-line intent clamped)
	assert.Equal(t, "line1", m.Value())
	assert.Equal(t, 0, m.cursorRow)
}

// ============================================================================
// Undo/Redo Tests (perles-nz7d.8)
// ============================================================================

func ctrlRKey() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyCtrlR}
}

func TestUndo_u_RestoresPreviousContent(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello")
	m.cursorCol = 0

	// Delete 'h' with x
	m, _ = m.Update(keyMsg('x'))
	assert.Equal(t, "ello", m.Value())

	// Undo with u
	m, _ = m.Update(keyMsg('u'))

	assert.Equal(t, "hello", m.Value())
}

func TestUndo_u_RestoresPreviousCursorPosition(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello world")
	m.cursorRow = 0
	m.cursorCol = 6 // At 'w' of "world"

	// Delete word with dw
	m, _ = m.Update(keyMsg('d'))
	m, _ = m.Update(keyMsg('w'))
	assert.Equal(t, "hello ", m.Value())

	// Undo with u
	m, _ = m.Update(keyMsg('u'))

	assert.Equal(t, "hello world", m.Value())
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 6, m.cursorCol) // Cursor restored to original position
}

func TestUndo_u_WithEmptyUndoStack_DoesNothing(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello")
	m.cursorCol = 2
	initialValue := m.Value()
	initialRow := m.cursorRow
	initialCol := m.cursorCol

	// u with empty undo stack should do nothing
	m, _ = m.Update(keyMsg('u'))

	assert.Equal(t, initialValue, m.Value())
	assert.Equal(t, initialRow, m.cursorRow)
	assert.Equal(t, initialCol, m.cursorCol)
	assert.False(t, m.CanUndo())
}

func TestRedo_CtrlR_AfterUndo_RestoresChange(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello")
	m.cursorCol = 0

	// Delete 'h' with x
	m, _ = m.Update(keyMsg('x'))
	assert.Equal(t, "ello", m.Value())

	// Undo with u
	m, _ = m.Update(keyMsg('u'))
	assert.Equal(t, "hello", m.Value())

	// Redo with Ctrl+R
	m, _ = m.Update(ctrlRKey())

	assert.Equal(t, "ello", m.Value())
}

func TestRedo_CtrlR_WithEmptyRedoStack_DoesNothing(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello")
	m.cursorCol = 0
	initialValue := m.Value()

	// Ctrl+R with empty redo stack should do nothing
	m, _ = m.Update(ctrlRKey())

	assert.Equal(t, initialValue, m.Value())
	assert.False(t, m.CanRedo())
}

func TestUndo_NewChange_ClearsRedoStack(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello")
	m.cursorCol = 0

	// Delete 'h' with x
	m, _ = m.Update(keyMsg('x'))
	assert.Equal(t, "ello", m.Value())

	// Undo with u - this puts state on redo stack
	m, _ = m.Update(keyMsg('u'))
	assert.Equal(t, "hello", m.Value())
	assert.True(t, m.CanRedo())

	// Make a new change - this should clear redo stack
	m.cursorCol = 0
	m, _ = m.Update(keyMsg('x'))
	assert.Equal(t, "ello", m.Value())

	// Redo stack should be cleared
	assert.False(t, m.CanRedo())
}

func TestUndo_Multiple_u_StepsBackThroughHistory(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello")
	m.cursorCol = 0

	// Delete 'h' with x
	m, _ = m.Update(keyMsg('x'))
	assert.Equal(t, "ello", m.Value())
	assert.True(t, m.CanUndo())

	// Delete 'e' with x
	m, _ = m.Update(keyMsg('x'))
	assert.Equal(t, "llo", m.Value())
	assert.True(t, m.CanUndo())

	// Delete 'l' with x
	m, _ = m.Update(keyMsg('x'))
	assert.Equal(t, "lo", m.Value())
	assert.True(t, m.CanUndo())

	// Undo once - restores "llo"
	m, _ = m.Update(keyMsg('u'))
	assert.Equal(t, "llo", m.Value())
	assert.True(t, m.CanUndo())

	// Undo twice - restores "ello"
	m, _ = m.Update(keyMsg('u'))
	assert.Equal(t, "ello", m.Value())
	assert.True(t, m.CanUndo())

	// Undo three times - restores "hello"
	m, _ = m.Update(keyMsg('u'))
	assert.Equal(t, "hello", m.Value())
	assert.False(t, m.CanUndo())
}

func TestUndoRedo_u_Then_CtrlR_IsIdempotent(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello")
	m.cursorCol = 0
	m.cursorRow = 0

	// Delete 'h' with x
	m, _ = m.Update(keyMsg('x'))
	afterDeleteContent := m.Value()
	afterDeleteRow := m.cursorRow
	afterDeleteCol := m.cursorCol

	// Undo with u
	m, _ = m.Update(keyMsg('u'))
	assert.Equal(t, "hello", m.Value())

	// Redo with Ctrl+R
	m, _ = m.Update(ctrlRKey())

	// Should be back to state after delete
	assert.Equal(t, afterDeleteContent, m.Value())
	assert.Equal(t, afterDeleteRow, m.cursorRow)
	assert.Equal(t, afterDeleteCol, m.cursorCol)
}

func TestRedo_CtrlR_PassesThroughWhenRedoStackEmpty(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello")
	m.cursorCol = 0

	// Verify redo stack is empty
	assert.False(t, m.CanRedo())

	// Ctrl+R should pass through (return nil cmd and no state change)
	m, cmd := m.Update(ctrlRKey())

	// No command should be emitted (passes through to parent)
	assert.Nil(t, cmd)
	// Value should be unchanged
	assert.Equal(t, "hello", m.Value())
}

func TestRedo_CtrlR_ConsumedWhenRedoAvailable(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello")
	m.cursorCol = 0

	// Delete 'h' with x
	m, _ = m.Update(keyMsg('x'))

	// Undo with u - this populates redo stack
	m, _ = m.Update(keyMsg('u'))
	assert.True(t, m.CanRedo())

	// Ctrl+R should be consumed (returns onChangeCmd)
	m, cmd := m.Update(ctrlRKey())

	// Even though OnChange is not set, the redo was consumed (state changed)
	assert.Equal(t, "ello", m.Value())
	// cmd is nil because OnChange callback is not set, but the redo was still consumed
	assert.Nil(t, cmd)
	// Redo stack should now be empty
	assert.False(t, m.CanRedo())
}

func TestRedo_CtrlR_ConsumedWhenRedoAvailable_WithOnChangeCallback(t *testing.T) {
	type ChangeMsg struct {
		Content string
	}

	callbackCalled := false
	m := New(Config{
		VimEnabled: true,
		OnChange: func(content string) tea.Msg {
			callbackCalled = true
			return ChangeMsg{Content: content}
		},
	})
	m.SetValue("hello")
	m.cursorCol = 0

	// Delete 'h' with x
	m, _ = m.Update(keyMsg('x'))

	// Undo with u
	m, _ = m.Update(keyMsg('u'))
	callbackCalled = false // Reset for redo test

	// Ctrl+R should be consumed
	m, cmd := m.Update(ctrlRKey())

	require.NotNil(t, cmd)
	msg := cmd()
	changeMsg, ok := msg.(ChangeMsg)
	require.True(t, ok, "Expected ChangeMsg, got %T", msg)

	assert.True(t, callbackCalled)
	assert.Equal(t, "ello", changeMsg.Content)
}

func TestUndo_u_InInsertMode_TypesCharacter(t *testing.T) {
	// In Insert mode, 'u' should type the character 'u', not undo
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("hello")
	m.cursorCol = 0

	// 'u' in insert mode types the character
	m, _ = m.Update(keyMsg('u'))

	assert.Equal(t, "uhello", m.Value())
	assert.Equal(t, 1, m.cursorCol)
	// Should still be in Insert mode
	assert.Equal(t, ModeInsert, m.Mode())
}

func TestRedo_MultipleRedos_StepsThroughHistory(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello")
	m.cursorCol = 0

	// Make three deletions
	m, _ = m.Update(keyMsg('x')) // "ello"
	m, _ = m.Update(keyMsg('x')) // "llo"
	m, _ = m.Update(keyMsg('x')) // "lo"

	// Undo all three
	m, _ = m.Update(keyMsg('u')) // "llo"
	m, _ = m.Update(keyMsg('u')) // "ello"
	m, _ = m.Update(keyMsg('u')) // "hello"
	assert.Equal(t, "hello", m.Value())

	// Redo all three
	m, _ = m.Update(ctrlRKey()) // "ello"
	assert.Equal(t, "ello", m.Value())

	m, _ = m.Update(ctrlRKey()) // "llo"
	assert.Equal(t, "llo", m.Value())

	m, _ = m.Update(ctrlRKey()) // "lo"
	assert.Equal(t, "lo", m.Value())
}

func TestUndo_RestoresMultiLineContent(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("line1\nline2\nline3")
	m.cursorRow = 1 // On "line2"

	// Delete line2 with dd
	m, _ = m.Update(keyMsg('d'))
	m, _ = m.Update(keyMsg('d'))
	assert.Equal(t, "line1\nline3", m.Value())
	assert.Equal(t, 2, len(m.Lines()))

	// Undo with u
	m, _ = m.Update(keyMsg('u'))

	assert.Equal(t, "line1\nline2\nline3", m.Value())
	assert.Equal(t, 3, len(m.Lines()))
	assert.Equal(t, 1, m.cursorRow) // Cursor restored to line2
}

// ============================================================================
// Insert Mode Text Input Tests (perles-nz7d.9)
// ============================================================================

func enterKey() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyEnter}
}

func backspaceKey() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyBackspace}
}

func deleteKey() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyDelete}
}

func escKey() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyEsc}
}

// Character Input Tests

func TestInsertMode_CharacterTyped_InsertsAtCursor(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("hello")
	m.cursorCol = 2

	m, _ = m.Update(keyMsg('X'))

	assert.Equal(t, "heXllo", m.Value())
}

func TestInsertMode_CharacterTyped_CursorAdvances(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("hello")
	m.cursorCol = 0

	m, _ = m.Update(keyMsg('a'))

	assert.Equal(t, "ahello", m.Value())
	assert.Equal(t, 1, m.cursorCol) // Cursor advanced
}

func TestInsertMode_MultipleCharacters_InsertedSequentially(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("")
	m.cursorCol = 0

	m, _ = m.Update(keyMsg('H'))
	m, _ = m.Update(keyMsg('i'))
	m, _ = m.Update(keyMsg('!'))

	assert.Equal(t, "Hi!", m.Value())
	assert.Equal(t, 3, m.cursorCol)
}

func TestInsertMode_CharacterAtEndOfLine_AppendsCorrectly(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("test")
	m.cursorCol = 4 // At end of line

	m, _ = m.Update(keyMsg('s'))

	assert.Equal(t, "tests", m.Value())
	assert.Equal(t, 5, m.cursorCol)
}

func TestInsertMode_SpaceKey_InsertsSpace(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("helloworld")
	m.cursorCol = 5

	// Space key is sent as tea.KeySpace, not tea.KeyRunes
	spaceMsg := tea.KeyMsg{Type: tea.KeySpace}
	m, _ = m.Update(spaceMsg)

	assert.Equal(t, "hello world", m.Value())
	assert.Equal(t, 6, m.cursorCol)
}

func TestInsertMode_SpaceKey_AtStartOfLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("hello")
	m.cursorCol = 0

	spaceMsg := tea.KeyMsg{Type: tea.KeySpace}
	m, _ = m.Update(spaceMsg)

	assert.Equal(t, " hello", m.Value())
	assert.Equal(t, 1, m.cursorCol)
}

func TestInsertMode_SpaceKey_MultipleSpaces(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("ab")
	m.cursorCol = 1

	spaceMsg := tea.KeyMsg{Type: tea.KeySpace}
	m, _ = m.Update(spaceMsg)
	m, _ = m.Update(spaceMsg)
	m, _ = m.Update(spaceMsg)

	assert.Equal(t, "a   b", m.Value())
	assert.Equal(t, 4, m.cursorCol)
}

// Backspace Tests

func TestInsertMode_Backspace_DeletesCharacterBeforeCursor(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("hello")
	m.cursorCol = 3 // After 'l'

	m, _ = m.Update(backspaceKey())

	assert.Equal(t, "helo", m.Value())
	assert.Equal(t, 2, m.cursorCol)
}

func TestInsertMode_Backspace_AtStartOfLine_JoinsWithPreviousLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("line1\nline2")
	m.cursorRow = 1
	m.cursorCol = 0 // At start of line2

	m, _ = m.Update(backspaceKey())

	assert.Equal(t, "line1line2", m.Value())
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 5, m.cursorCol) // Cursor at join point
	assert.Equal(t, 1, len(m.Lines()))
}

func TestInsertMode_Backspace_OnEmptyContent_DoesNothing(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("")
	m.cursorRow = 0
	m.cursorCol = 0

	m, _ = m.Update(backspaceKey())

	assert.Equal(t, "", m.Value())
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 0, m.cursorCol)
}

func TestInsertMode_Backspace_AtStartOfFirstLine_DoesNothing(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("hello")
	m.cursorRow = 0
	m.cursorCol = 0

	m, _ = m.Update(backspaceKey())

	assert.Equal(t, "hello", m.Value())
	assert.Equal(t, 0, m.cursorCol)
}

// Delete Key Tests

func TestInsertMode_Delete_DeletesCharacterAtCursor(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("hello")
	m.cursorCol = 2 // At 'l'

	m, _ = m.Update(deleteKey())

	assert.Equal(t, "helo", m.Value())
	assert.Equal(t, 2, m.cursorCol) // Cursor stays in place
}

func TestInsertMode_Delete_AtEndOfLine_JoinsWithNextLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("line1\nline2")
	m.cursorRow = 0
	m.cursorCol = 5 // At end of line1

	m, _ = m.Update(deleteKey())

	assert.Equal(t, "line1line2", m.Value())
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 5, m.cursorCol)
	assert.Equal(t, 1, len(m.Lines()))
}

func TestInsertMode_Delete_AtEndOfLastLine_DoesNothing(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("hello")
	m.cursorCol = 5 // At end of line

	m, _ = m.Update(deleteKey())

	assert.Equal(t, "hello", m.Value())
	assert.Equal(t, 5, m.cursorCol)
}

// Enter Key Tests - Enter now submits, Alt+Enter splits lines

func TestInsertMode_Enter_TriggersSubmit(t *testing.T) {
	submitCalled := false
	m := New(Config{
		VimEnabled:  true,
		DefaultMode: ModeInsert,
		OnSubmit: func(content string) tea.Msg {
			submitCalled = true
			return nil
		},
	})
	m.SetValue("hello")
	m.cursorCol = 5

	m, cmd := m.Update(enterKey())

	require.NotNil(t, cmd) // Should produce submit command
	_ = cmd()              // Execute it
	assert.True(t, submitCalled)
}

func TestInsertMode_Enter_EmitsSubmitMsg_WhenNoCallback(t *testing.T) {
	m := New(Config{VimEnabled: true})
	m.SetValue("test content")

	m, cmd := m.Update(enterKey())

	require.NotNil(t, cmd)
	msg := cmd()
	submitMsg, ok := msg.(SubmitMsg)
	require.True(t, ok, "Expected SubmitMsg, got %T", msg)

	assert.Equal(t, "test content", submitMsg.Content)
}

func TestNormalMode_Enter_EmitsSubmitMsg(t *testing.T) {
	// Verify that submit works from Normal mode (not just Insert mode)
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("normal mode content")

	// Verify we're in Normal mode
	require.Equal(t, ModeNormal, m.Mode())

	m, cmd := m.Update(enterKey())

	require.NotNil(t, cmd)
	msg := cmd()
	submitMsg, ok := msg.(SubmitMsg)
	require.True(t, ok, "Expected SubmitMsg, got %T", msg)
	assert.Equal(t, "normal mode content", submitMsg.Content)
}

// Alt+Enter Split Line Tests - Alt+Enter now splits lines

func altEnterKey() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyEnter, Alt: true}
}

func TestInsertMode_AltEnter_SplitsLineAtCursor(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("helloworld")
	m.cursorCol = 5 // After "hello"

	m, _ = m.Update(altEnterKey())

	assert.Equal(t, "hello\nworld", m.Value())
	assert.Equal(t, 1, m.cursorRow)
	assert.Equal(t, 0, m.cursorCol) // At start of new line
}

func TestInsertMode_AltEnter_AtEndOfLine_CreatesNewEmptyLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("hello")
	m.cursorCol = 5 // At end of line

	m, _ = m.Update(altEnterKey())

	assert.Equal(t, "hello\n", m.Value())
	assert.Equal(t, 1, m.cursorRow)
	assert.Equal(t, 0, m.cursorCol)
	assert.Equal(t, 2, len(m.Lines()))
	assert.Equal(t, "", m.Lines()[1])
}

func TestInsertMode_AltEnter_AtStartOfLine_CreatesNewEmptyLineAbove(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("hello")
	m.cursorCol = 0 // At start of line

	m, _ = m.Update(altEnterKey())

	assert.Equal(t, "\nhello", m.Value())
	assert.Equal(t, 1, m.cursorRow)
	assert.Equal(t, 0, m.cursorCol)
	assert.Equal(t, "", m.Lines()[0])
	assert.Equal(t, "hello", m.Lines()[1])
}

func TestInsertMode_AltEnter_DoesNotTriggerSubmit(t *testing.T) {
	submitCalled := false
	m := New(Config{
		VimEnabled:  true,
		DefaultMode: ModeInsert,
		OnSubmit: func(content string) tea.Msg {
			submitCalled = true
			return nil
		},
	})
	m.SetValue("hello")
	m.cursorCol = 5

	m, _ = m.Update(altEnterKey())

	assert.False(t, submitCalled) // Alt+Enter should NOT submit
	assert.Equal(t, "hello\n", m.Value())
}

// Ctrl+J Submit Tests (alternative submit key)

func TestInsertMode_CtrlJ_TriggersOnSubmitCallback(t *testing.T) {
	type CustomSubmitMsg struct {
		Content string
	}

	var receivedContent string
	m := New(Config{
		VimEnabled: true,
		OnSubmit: func(content string) tea.Msg {
			receivedContent = content
			return CustomSubmitMsg{Content: content}
		},
	})
	m.SetValue("hello world")
	m.cursorCol = 5

	ctrlJMsg := tea.KeyMsg{Type: tea.KeyCtrlJ}
	m, cmd := m.Update(ctrlJMsg)

	require.NotNil(t, cmd)
	msg := cmd()
	submitMsg, ok := msg.(CustomSubmitMsg)
	require.True(t, ok, "Expected CustomSubmitMsg, got %T", msg)

	assert.Equal(t, "hello world", receivedContent)
	assert.Equal(t, "hello world", submitMsg.Content)
}

func TestNormalMode_CtrlJ_EmitsSubmitMsg(t *testing.T) {
	// Verify that Ctrl+J submit works from Normal mode
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("normal mode content")

	// Verify we're in Normal mode
	require.Equal(t, ModeNormal, m.Mode())

	ctrlJMsg := tea.KeyMsg{Type: tea.KeyCtrlJ}
	m, cmd := m.Update(ctrlJMsg)

	require.NotNil(t, cmd)
	msg := cmd()
	submitMsg, ok := msg.(SubmitMsg)
	require.True(t, ok, "Expected SubmitMsg from Normal mode, got %T", msg)
	assert.Equal(t, "normal mode content", submitMsg.Content)
}

func TestInsertMode_SubmitMsg_ContainsCorrectContent(t *testing.T) {
	m := New(Config{VimEnabled: true})
	m.SetValue("line1\nline2\nline3")

	m, cmd := m.submitContent()

	require.NotNil(t, cmd)
	msg := cmd()
	submitMsg := msg.(SubmitMsg)

	assert.Equal(t, "line1\nline2\nline3", submitMsg.Content)
}

// OnChange Callback Tests

func TestInsertMode_OnChange_CalledOnTextModification(t *testing.T) {
	type ChangeMsg struct {
		Content string
	}

	var changes []string
	m := New(Config{
		VimEnabled:  true,
		DefaultMode: ModeInsert,
		OnChange: func(content string) tea.Msg {
			changes = append(changes, content)
			return ChangeMsg{Content: content}
		},
	})
	m.SetValue("hi")
	m.cursorCol = 2

	// Type a character
	m, cmd := m.Update(keyMsg('!'))
	require.NotNil(t, cmd)
	cmd()

	assert.Equal(t, []string{"hi!"}, changes)
	assert.Equal(t, "hi!", m.Value())
}

// CharLimit Tests

func TestInsertMode_CharLimit_Enforced(t *testing.T) {
	m := New(Config{
		VimEnabled:  true,
		DefaultMode: ModeInsert,
		CharLimit:   5,
	})
	m.SetValue("hello")
	m.cursorCol = 5

	// Try to insert beyond limit
	m, _ = m.Update(keyMsg('!'))

	assert.Equal(t, "hello", m.Value()) // No change, at limit
	assert.Equal(t, 5, m.cursorCol)     // Cursor unchanged
}

func TestInsertMode_CharLimit_TruncatesInput(t *testing.T) {
	m := New(Config{
		VimEnabled:  true,
		DefaultMode: ModeInsert,
		CharLimit:   10,
	})
	m.SetValue("hello") // 5 chars
	m.cursorCol = 5

	// Try to insert - only 5 more chars allowed
	m, _ = m.Update(keyMsg('1'))
	m, _ = m.Update(keyMsg('2'))
	m, _ = m.Update(keyMsg('3'))
	m, _ = m.Update(keyMsg('4'))
	m, _ = m.Update(keyMsg('5'))
	m, _ = m.Update(keyMsg('6')) // Should be rejected

	assert.Equal(t, "hello12345", m.Value()) // Only first 5 added
}

func TestInsertMode_CharLimit_CountsNewlines(t *testing.T) {
	m := New(Config{
		VimEnabled:  true,
		DefaultMode: ModeInsert,
		CharLimit:   10, // "hi\nworld" = 2 + 1 (newline) + 5 = 8
	})
	m.SetValue("hi")
	m.cursorCol = 2

	// Insert newline (counts as 1 char) - use Alt+Enter for line split
	m, _ = m.Update(altEnterKey())
	// Now at 3 chars: "hi\n"

	// Add "world" (5 chars) = total 8
	m, _ = m.Update(keyMsg('w'))
	m, _ = m.Update(keyMsg('o'))
	m, _ = m.Update(keyMsg('r'))
	m, _ = m.Update(keyMsg('l'))
	m, _ = m.Update(keyMsg('d'))
	// Now at 8 chars: "hi\nworld"
	m, _ = m.Update(keyMsg('!')) // 9th char - allowed
	m, _ = m.Update(keyMsg('?')) // 10th char - allowed
	m, _ = m.Update(keyMsg('#')) // 11th char - rejected

	assert.Equal(t, "hi\nworld!?", m.Value()) // 10 chars total
}

// Undo Tests for Insert Mode

func TestInsertMode_UndoStack_PopulatedOnCharacterInsert(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("hello")
	m.cursorCol = 0

	assert.False(t, m.CanUndo())

	m, _ = m.Update(keyMsg('X'))

	assert.True(t, m.CanUndo())
	// Verify undo restores the original content
	m, _ = m.Update(escKey())    // Exit insert mode
	m, _ = m.Update(keyMsg('u')) // Undo
	assert.Equal(t, "hello", m.Value())
}

func TestInsertMode_UndoStack_PopulatedOnBackspace(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("hello")
	m.cursorCol = 3

	m, _ = m.Update(backspaceKey())

	assert.True(t, m.CanUndo())
	// Verify undo restores the original content
	m, _ = m.Update(escKey())    // Exit insert mode
	m, _ = m.Update(keyMsg('u')) // Undo
	assert.Equal(t, "hello", m.Value())
}

func TestInsertMode_UndoStack_PopulatedOnDelete(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("hello")
	m.cursorCol = 2

	m, _ = m.Update(deleteKey())

	assert.True(t, m.CanUndo())
	// Verify undo restores the original content
	m, _ = m.Update(escKey())    // Exit insert mode
	m, _ = m.Update(keyMsg('u')) // Undo
	assert.Equal(t, "hello", m.Value())
}

func TestInsertMode_UndoStack_PopulatedOnAltEnter(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("hello")
	m.cursorCol = 2

	m, _ = m.Update(altEnterKey()) // Alt+Enter splits line

	assert.True(t, m.CanUndo())
	// Verify undo restores the original content
	m, _ = m.Update(escKey())    // Exit insert mode
	m, _ = m.Update(keyMsg('u')) // Undo
	assert.Equal(t, "hello", m.Value())
}

// Edge Case Tests

func TestInsertMode_BackspaceAtStartOfLine_JoinsLines(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("first\nsecond\nthird")
	m.cursorRow = 1
	m.cursorCol = 0

	m, _ = m.Update(backspaceKey())

	assert.Equal(t, "firstsecond\nthird", m.Value())
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 5, m.cursorCol) // After "first"
}

func TestInsertMode_DeleteAtEndOfLine_JoinsLines(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("first\nsecond\nthird")
	m.cursorRow = 0
	m.cursorCol = 5

	m, _ = m.Update(deleteKey())

	assert.Equal(t, "firstsecond\nthird", m.Value())
	assert.Equal(t, 0, m.cursorRow)
	assert.Equal(t, 5, m.cursorCol)
}

func TestInsertMode_VimDisabled_CharacterInput(t *testing.T) {
	// When vim is disabled, typing should still work (always in Insert mode)
	m := New(Config{VimEnabled: false})
	m.SetValue("")
	m.cursorCol = 0

	m, _ = m.Update(keyMsg('h'))
	m, _ = m.Update(keyMsg('e'))
	m, _ = m.Update(keyMsg('l'))
	m, _ = m.Update(keyMsg('l'))
	m, _ = m.Update(keyMsg('o'))

	assert.Equal(t, "hello", m.Value())
	assert.Equal(t, ModeInsert, m.Mode()) // Still in Insert mode
}

func TestInsertMode_VimDisabled_BackspaceWorks(t *testing.T) {
	m := New(Config{VimEnabled: false})
	m.SetValue("hello")
	m.cursorCol = 5

	m, _ = m.Update(backspaceKey())

	assert.Equal(t, "hell", m.Value())
}

func TestInsertMode_VimDisabled_EnterSubmits(t *testing.T) {
	// When vim is disabled, Enter submits instead of inserting a newline
	m := New(Config{VimEnabled: false})
	m.SetValue("hello")
	m.cursorCol = 2

	_, cmd := m.Update(enterKey())

	// Should emit SubmitMsg, not insert a newline
	require.NotNil(t, cmd)
	msg := cmd()
	submitMsg, ok := msg.(SubmitMsg)
	require.True(t, ok, "expected SubmitMsg, got %T", msg)
	assert.Equal(t, "hello", submitMsg.Content)
}

// ============================================================================
// Space Key Tests (perles-nz7d.14)
// ============================================================================

func spaceKey() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeySpace}
}

func TestInsertMode_Space_InsertsSpaceCharacter(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("helloworld")
	m.cursorCol = 5 // Between "hello" and "world"

	m, _ = m.Update(spaceKey())

	assert.Equal(t, "hello world", m.Value())
	assert.Equal(t, 6, m.cursorCol)
}

func TestInsertMode_Space_AtBeginning(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("hello")
	m.cursorCol = 0

	m, _ = m.Update(spaceKey())

	assert.Equal(t, " hello", m.Value())
	assert.Equal(t, 1, m.cursorCol)
}

func TestInsertMode_Space_AtEnd(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeInsert})
	m.SetValue("hello")
	m.cursorCol = 5

	m, _ = m.Update(spaceKey())

	assert.Equal(t, "hello ", m.Value())
	assert.Equal(t, 6, m.cursorCol)
}

func TestInsertMode_Space_VimDisabled(t *testing.T) {
	m := New(Config{VimEnabled: false})
	m.SetValue("helloworld")
	m.cursorCol = 5

	m, _ = m.Update(spaceKey())

	assert.Equal(t, "hello world", m.Value())
}

// ============================================================================
// Visual Mode Tests
// ============================================================================

func TestVisualMode_v_EntersPendingState(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello world")
	m.cursorCol = 0

	m, _ = m.Update(keyMsg('v'))

	// 'v' now enters pending state (for text object support like viw/vaw)
	assert.Equal(t, ModeNormal, m.Mode())
	assert.Equal(t, 'v', m.pendingBuilder.Operator())
}

func TestVisualMode_v_FollowedByMotion_EntersVisualMode(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello world")
	m.cursorCol = 0

	// 'v' followed by 'l' should enter visual mode via fallback
	m, _ = m.Update(keyMsg('v'))
	m, _ = m.Update(keyMsg('l'))

	assert.Equal(t, ModeVisual, m.Mode())
}

func TestVisualMode_V_EntersVisualLineMode(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello\nworld")
	m.cursorRow = 0

	m, _ = m.Update(keyMsg('V'))

	assert.Equal(t, ModeVisualLine, m.Mode())
}

func TestVisualMode_Escape_ExitsToNormalMode(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello")
	m.cursorCol = 2 // Start in middle
	// Enter visual mode via fallback: 'v' + 'l' enters visual and moves
	m, _ = m.Update(keyMsg('v'))
	m, _ = m.Update(keyMsg('l'))
	require.Equal(t, ModeVisual, m.Mode())

	m, _ = m.Update(escapeKey())

	assert.Equal(t, ModeNormal, m.Mode())
}

func TestVisualMode_d_DeletesSelection(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello world")
	m.cursorCol = 0

	// Enter visual mode and select "hello"
	m, _ = m.Update(keyMsg('v'))
	m, _ = m.Update(keyMsg('l'))
	m, _ = m.Update(keyMsg('l'))
	m, _ = m.Update(keyMsg('l'))
	m, _ = m.Update(keyMsg('l')) // Now selecting h-e-l-l-o

	// Delete selection
	m, _ = m.Update(keyMsg('d'))

	assert.Equal(t, " world", m.Value())
	assert.Equal(t, ModeNormal, m.Mode())
}

func TestVisualMode_d_Undo_RestoresContent(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello world")
	m.cursorCol = 0

	// Enter visual mode and select "hello"
	m, _ = m.Update(keyMsg('v'))
	m, _ = m.Update(keyMsg('l'))
	m, _ = m.Update(keyMsg('l'))
	m, _ = m.Update(keyMsg('l'))
	m, _ = m.Update(keyMsg('l'))

	// Delete selection
	m, _ = m.Update(keyMsg('d'))
	require.Equal(t, " world", m.Value())

	// Undo
	m, _ = m.Update(keyMsg('u'))

	assert.Equal(t, "hello world", m.Value())
}

func TestVisualMode_y_YanksSelection(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello world")
	m.cursorCol = 0

	// Enter visual mode and select "hello"
	m, _ = m.Update(keyMsg('v'))
	m, _ = m.Update(keyMsg('l'))
	m, _ = m.Update(keyMsg('l'))
	m, _ = m.Update(keyMsg('l'))
	m, _ = m.Update(keyMsg('l'))

	// Yank selection
	m, _ = m.Update(keyMsg('y'))

	// Content should be unchanged
	assert.Equal(t, "hello world", m.Value())
	assert.Equal(t, ModeNormal, m.Mode())
}

func TestVisualMode_c_ChangesSelection(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello world")
	m.cursorCol = 0

	// Enter visual mode and select "hello"
	m, _ = m.Update(keyMsg('v'))
	m, _ = m.Update(keyMsg('l'))
	m, _ = m.Update(keyMsg('l'))
	m, _ = m.Update(keyMsg('l'))
	m, _ = m.Update(keyMsg('l'))

	// Change selection
	m, _ = m.Update(keyMsg('c'))

	assert.Equal(t, " world", m.Value())
	assert.Equal(t, ModeInsert, m.Mode()) // Change enters insert mode
}

func TestVisualLineMode_d_DeletesEntireLine(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("line one\nline two\nline three")
	m.cursorRow = 1
	m.cursorCol = 0

	// Enter visual line mode
	m, _ = m.Update(keyMsg('V'))
	require.Equal(t, ModeVisualLine, m.Mode())

	// Delete the line
	m, _ = m.Update(keyMsg('d'))

	assert.Equal(t, "line one\nline three", m.Value())
	assert.Equal(t, ModeNormal, m.Mode())
}

func TestVisualLineMode_d_DeletesMultipleLines(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("line one\nline two\nline three\nline four")
	m.cursorRow = 1
	m.cursorCol = 0

	// Enter visual line mode
	m, _ = m.Update(keyMsg('V'))
	// Extend selection down
	m, _ = m.Update(keyMsg('j'))

	// Delete the lines
	m, _ = m.Update(keyMsg('d'))

	assert.Equal(t, "line one\nline four", m.Value())
}

func TestVisualLineMode_d_Undo_RestoresLines(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("line one\nline two\nline three")
	m.cursorRow = 1

	// Enter visual line mode and delete
	m, _ = m.Update(keyMsg('V'))
	m, _ = m.Update(keyMsg('d'))
	require.Equal(t, "line one\nline three", m.Value())

	// Undo
	m, _ = m.Update(keyMsg('u'))

	assert.Equal(t, "line one\nline two\nline three", m.Value())
}

func TestVisualMode_x_DeletesSelection(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello world")
	m.cursorCol = 0

	// Enter visual mode and select "hel"
	m, _ = m.Update(keyMsg('v'))
	m, _ = m.Update(keyMsg('l'))
	m, _ = m.Update(keyMsg('l'))

	// Delete with x (same as d in visual mode)
	m, _ = m.Update(keyMsg('x'))

	assert.Equal(t, "lo world", m.Value())
}

func TestVisualMode_o_SwapsAnchorAndCursor(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello world")
	m.cursorCol = 0

	// Enter visual mode
	m, _ = m.Update(keyMsg('v'))
	// Move right to select
	m, _ = m.Update(keyMsg('l'))
	m, _ = m.Update(keyMsg('l'))
	m, _ = m.Update(keyMsg('l'))

	originalCursorCol := m.cursorCol

	// Swap anchor and cursor
	m, _ = m.Update(keyMsg('o'))

	// Cursor should now be at original anchor position
	assert.NotEqual(t, originalCursorCol, m.cursorCol)
	assert.Equal(t, ModeVisual, m.Mode()) // Still in visual mode
}

func TestVisualMode_MultiLine_Delete(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello\nworld\nfoo")
	m.cursorRow = 0
	m.cursorCol = 2

	// Enter visual mode
	m, _ = m.Update(keyMsg('v'))
	// Select to next line
	m, _ = m.Update(keyMsg('j'))

	// Delete
	m, _ = m.Update(keyMsg('d'))

	// Should have deleted from "llo" on first line through "wo" on second
	assert.Equal(t, 2, len(m.content))
	assert.Equal(t, ModeNormal, m.Mode())
}

func TestVisualMode_MultiLine_Undo(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello\nworld\nfoo")
	m.cursorRow = 0
	m.cursorCol = 2

	// Enter visual mode, select to next line, delete
	m, _ = m.Update(keyMsg('v'))
	m, _ = m.Update(keyMsg('j'))
	m, _ = m.Update(keyMsg('d'))

	// Undo
	m, _ = m.Update(keyMsg('u'))

	assert.Equal(t, "hello\nworld\nfoo", m.Value())
}

func TestVisualMode_c_Undo_RestoresContent(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("hello world")
	m.cursorCol = 0

	// Enter visual mode and select "hello"
	m, _ = m.Update(keyMsg('v'))
	m, _ = m.Update(keyMsg('l'))
	m, _ = m.Update(keyMsg('l'))
	m, _ = m.Update(keyMsg('l'))
	m, _ = m.Update(keyMsg('l'))

	// Change selection (this deletes "hello" and enters insert mode)
	m, _ = m.Update(keyMsg('c'))
	require.Equal(t, " world", m.Value())
	require.Equal(t, ModeInsert, m.Mode())

	// Exit insert mode without typing anything
	m, _ = m.Update(escapeKey())

	// Undo the change (should restore the deleted text)
	m, _ = m.Update(keyMsg('u'))

	assert.Equal(t, "hello world", m.Value())
}

func TestVisualLineMode_y_YanksLines(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("line one\nline two\nline three")
	m.cursorRow = 1

	// Enter visual line mode
	m, _ = m.Update(keyMsg('V'))

	// Yank
	m, _ = m.Update(keyMsg('y'))

	// Content should be unchanged
	assert.Equal(t, "line one\nline two\nline three", m.Value())
	assert.Equal(t, ModeNormal, m.Mode())
}

func TestVisualLineMode_c_ChangesLines(t *testing.T) {
	m := New(Config{VimEnabled: true, DefaultMode: ModeNormal})
	m.SetValue("line one\nline two\nline three")
	m.cursorRow = 1

	// Enter visual line mode
	m, _ = m.Update(keyMsg('V'))

	// Change
	m, _ = m.Update(keyMsg('c'))

	assert.Equal(t, ModeInsert, m.Mode())
	// Line should be deleted, ready for new content
	assert.Equal(t, 2, len(m.content))
}
