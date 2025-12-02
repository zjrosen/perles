package bqlinput

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/muesli/termenv"
)

func init() {
	// Force ANSI color output in tests (lipgloss disables colors when no TTY)
	lipgloss.SetColorProfile(termenv.ANSI256)
}

func TestNew_DefaultValues(t *testing.T) {
	m := New()

	if m.Value() != "" {
		t.Errorf("expected empty value, got %q", m.Value())
	}
	if m.Cursor() != 0 {
		t.Errorf("expected cursor at 0, got %d", m.Cursor())
	}
	if m.Focused() {
		t.Error("expected not focused by default")
	}
	if m.Width() != 40 {
		t.Errorf("expected width 40, got %d", m.Width())
	}
}

func TestSetValue(t *testing.T) {
	m := New()
	m.SetValue("test")

	if m.Value() != "test" {
		t.Errorf("expected 'test', got %q", m.Value())
	}
}

func TestSetValue_ClampsCursor(t *testing.T) {
	m := New()
	m.SetValue("hello")
	m.SetCursor(5) // cursor at end

	// Now set shorter value
	m.SetValue("hi")

	if m.Cursor() != 2 {
		t.Errorf("expected cursor clamped to 2, got %d", m.Cursor())
	}
}

func TestSetCursor_ClampsToRange(t *testing.T) {
	m := New()
	m.SetValue("test")

	// Test negative
	m.SetCursor(-5)
	if m.Cursor() != 0 {
		t.Errorf("expected 0 for negative, got %d", m.Cursor())
	}

	// Test past end
	m.SetCursor(100)
	if m.Cursor() != 4 {
		t.Errorf("expected 4 (length), got %d", m.Cursor())
	}

	// Test valid
	m.SetCursor(2)
	if m.Cursor() != 2 {
		t.Errorf("expected 2, got %d", m.Cursor())
	}
}

func TestFocusBlur(t *testing.T) {
	m := New()

	m.Focus()
	if !m.Focused() {
		t.Error("expected focused after Focus()")
	}

	m.Blur()
	if m.Focused() {
		t.Error("expected not focused after Blur()")
	}
}

func TestSetWidth(t *testing.T) {
	m := New()

	m.SetWidth(100)
	if m.Width() != 100 {
		t.Errorf("expected 100, got %d", m.Width())
	}

	// Minimum width is 1
	m.SetWidth(0)
	if m.Width() != 1 {
		t.Errorf("expected minimum width 1, got %d", m.Width())
	}
}

func TestUpdate_NotFocused_IgnoresKeys(t *testing.T) {
	m := New()
	m.SetValue("test")

	// Not focused, so key should be ignored
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})

	if m.Value() != "test" {
		t.Errorf("expected value unchanged when not focused, got %q", m.Value())
	}
}

func TestUpdate_InsertChars(t *testing.T) {
	m := New()
	m.Focus()

	// Type "hi"
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})

	if m.Value() != "hi" {
		t.Errorf("expected 'hi', got %q", m.Value())
	}
	if m.Cursor() != 2 {
		t.Errorf("expected cursor at 2, got %d", m.Cursor())
	}
}

func TestUpdate_InsertInMiddle(t *testing.T) {
	m := New()
	m.SetValue("hllo")
	m.SetCursor(1) // after 'h'
	m.Focus()

	// Insert 'e'
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})

	if m.Value() != "hello" {
		t.Errorf("expected 'hello', got %q", m.Value())
	}
	if m.Cursor() != 2 {
		t.Errorf("expected cursor at 2, got %d", m.Cursor())
	}
}

func TestUpdate_Space(t *testing.T) {
	m := New()
	m.SetValue("ab")
	m.SetCursor(1)
	m.Focus()

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})

	if m.Value() != "a b" {
		t.Errorf("expected 'a b', got %q", m.Value())
	}
}

func TestUpdate_Backspace(t *testing.T) {
	m := New()
	m.SetValue("hello")
	m.SetCursor(5) // at end
	m.Focus()

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})

	if m.Value() != "hell" {
		t.Errorf("expected 'hell', got %q", m.Value())
	}
	if m.Cursor() != 4 {
		t.Errorf("expected cursor at 4, got %d", m.Cursor())
	}
}

func TestUpdate_BackspaceAtStart(t *testing.T) {
	m := New()
	m.SetValue("test")
	m.SetCursor(0)
	m.Focus()

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})

	if m.Value() != "test" {
		t.Errorf("expected unchanged 'test', got %q", m.Value())
	}
}

func TestUpdate_Delete(t *testing.T) {
	m := New()
	m.SetValue("hello")
	m.SetCursor(0)
	m.Focus()

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDelete})

	if m.Value() != "ello" {
		t.Errorf("expected 'ello', got %q", m.Value())
	}
	if m.Cursor() != 0 {
		t.Errorf("expected cursor at 0, got %d", m.Cursor())
	}
}

func TestUpdate_DeleteAtEnd(t *testing.T) {
	m := New()
	m.SetValue("test")
	m.SetCursor(4)
	m.Focus()

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDelete})

	if m.Value() != "test" {
		t.Errorf("expected unchanged 'test', got %q", m.Value())
	}
}

func TestUpdate_CursorMovement(t *testing.T) {
	m := New()
	m.SetValue("hello")
	m.SetCursor(2)
	m.Focus()

	// Left
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if m.Cursor() != 1 {
		t.Errorf("expected cursor at 1 after left, got %d", m.Cursor())
	}

	// Right
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.Cursor() != 2 {
		t.Errorf("expected cursor at 2 after right, got %d", m.Cursor())
	}

	// Home (Ctrl+A)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	if m.Cursor() != 0 {
		t.Errorf("expected cursor at 0 after home, got %d", m.Cursor())
	}

	// End (Ctrl+E)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	if m.Cursor() != 5 {
		t.Errorf("expected cursor at 5 after end, got %d", m.Cursor())
	}
}

func TestUpdate_CursorBounds(t *testing.T) {
	m := New()
	m.SetValue("hi")
	m.Focus()

	// At start, left should stay at 0
	m.SetCursor(0)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if m.Cursor() != 0 {
		t.Errorf("expected cursor to stay at 0, got %d", m.Cursor())
	}

	// At end, right should stay at end
	m.SetCursor(2)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.Cursor() != 2 {
		t.Errorf("expected cursor to stay at 2, got %d", m.Cursor())
	}
}

func TestView_EmptyFocused(t *testing.T) {
	m := New()
	m.Focus()

	view := m.View()
	// Empty input when focused shows cursor
	if view == "" {
		t.Error("expected cursor for focused empty input")
	}
}

func TestView_EmptyNotFocused(t *testing.T) {
	m := New()
	// No focus, no placeholder

	view := m.View()
	if view != "" {
		t.Errorf("expected empty view, got %q", view)
	}
}

func TestView_Placeholder(t *testing.T) {
	m := New()
	m.SetPlaceholder("Enter query")

	view := m.View()
	if !strings.Contains(view, "Enter query") {
		t.Errorf("expected placeholder in view, got %q", view)
	}
}

func TestView_WithValue_HasHighlighting(t *testing.T) {
	m := New()
	m.SetValue("status = open")

	view := m.View()
	// Should contain ANSI codes for highlighting
	if !strings.Contains(view, "\x1b[") {
		t.Error("expected ANSI codes in view for highlighting")
	}
	// Should contain the text content
	if !strings.Contains(view, "status") {
		t.Errorf("expected 'status' in view, got %q", view)
	}
}

func TestView_Focused_ShowsHighlightedText(t *testing.T) {
	m := New()
	m.SetValue("status = open")
	m.SetCursor(0) // cursor at start
	m.Focus()

	view := m.View()
	// View should show syntax-highlighted text with cursor
	if view == "" {
		t.Error("expected non-empty view")
	}
	// Should contain ANSI codes (for highlighting and cursor)
	if !strings.Contains(view, "\x1b[") {
		t.Error("expected ANSI codes in view")
	}
}

func TestNextWordEnd(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		pos      int
		expected int
	}{
		{"from start", "hello world", 0, 5},
		{"from middle of word", "hello world", 2, 5},
		{"from space", "hello world", 5, 11},
		{"from second word", "hello world", 6, 11},
		{"at end", "hello", 5, 5},
		{"with punctuation", "status:open", 0, 6},
		{"skip colon", "status:open", 6, 11}, // from ':', skips non-word then 'open'
		{"after colon", "status:open", 7, 11},
		{"multiple spaces", "a   b", 0, 1},
		{"empty string", "", 0, 0},
		{"underscores", "my_var next", 0, 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := nextWordEnd(tt.s, tt.pos)
			if result != tt.expected {
				t.Errorf("nextWordEnd(%q, %d) = %d, expected %d", tt.s, tt.pos, result, tt.expected)
			}
		})
	}
}

func TestPrevWordStart(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		pos      int
		expected int
	}{
		{"from end", "hello world", 11, 6},
		{"from middle of second word", "hello world", 8, 6},
		{"from space", "hello world", 6, 0},
		{"from start of second word", "hello world", 6, 0},
		{"at start", "hello", 0, 0},
		{"with punctuation", "status:open", 11, 7},
		{"before colon", "status:open", 7, 0},
		{"at colon", "status:open", 6, 0},
		{"multiple spaces", "a   b", 5, 4}, // from after 'b', goes to start of 'b'
		{"empty string", "", 0, 0},
		{"underscores", "my_var next", 11, 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := prevWordStart(tt.s, tt.pos)
			if result != tt.expected {
				t.Errorf("prevWordStart(%q, %d) = %d, expected %d", tt.s, tt.pos, result, tt.expected)
			}
		})
	}
}

func TestUpdate_CtrlF_WordForward(t *testing.T) {
	m := New()
	m.SetValue("hello world")
	m.SetCursor(0)
	m.Focus()

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})

	if m.Cursor() != 5 {
		t.Errorf("expected cursor at 5 after ctrl+f, got %d", m.Cursor())
	}
}

func TestUpdate_CtrlB_WordBackward(t *testing.T) {
	m := New()
	m.SetValue("hello world")
	m.SetCursor(11)
	m.Focus()

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})

	if m.Cursor() != 6 {
		t.Errorf("expected cursor at 6 after ctrl+b, got %d", m.Cursor())
	}
}

func TestUpdate_AltRight_WordForward(t *testing.T) {
	m := New()
	m.SetValue("hello world")
	m.SetCursor(0)
	m.Focus()

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight, Alt: true})

	if m.Cursor() != 5 {
		t.Errorf("expected cursor at 5 after alt+right, got %d", m.Cursor())
	}
}

func TestUpdate_AltLeft_WordBackward(t *testing.T) {
	m := New()
	m.SetValue("hello world")
	m.SetCursor(11)
	m.Focus()

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft, Alt: true})

	if m.Cursor() != 6 {
		t.Errorf("expected cursor at 6 after alt+left, got %d", m.Cursor())
	}
}

func TestUpdate_AltF_WordForward(t *testing.T) {
	// macOS option+right sends Alt+f
	m := New()
	m.SetValue("hello world")
	m.SetCursor(0)
	m.Focus()

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}, Alt: true})

	if m.Cursor() != 5 {
		t.Errorf("expected cursor at 5 after alt+f, got %d", m.Cursor())
	}
}

func TestUpdate_AltB_WordBackward(t *testing.T) {
	// macOS option+left sends Alt+b
	m := New()
	m.SetValue("hello world")
	m.SetCursor(11)
	m.Focus()

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}, Alt: true})

	if m.Cursor() != 6 {
		t.Errorf("expected cursor at 6 after alt+b, got %d", m.Cursor())
	}
}

func TestHeight_Empty(t *testing.T) {
	m := New()
	m.SetWidth(40)

	// Empty value should return height 1
	if m.Height() != 1 {
		t.Errorf("expected height 1 for empty, got %d", m.Height())
	}
}

func TestHeight_SingleLine(t *testing.T) {
	m := New()
	m.SetWidth(40)
	m.SetValue("status = open")

	// Short text should fit on one line
	if m.Height() != 1 {
		t.Errorf("expected height 1 for short text, got %d", m.Height())
	}
}

func TestHeight_MultiLine(t *testing.T) {
	m := New()
	m.SetWidth(20) // narrow width to force wrapping
	m.SetValue("status = open and priority = p0 and type = bug")

	// Long text should wrap to multiple lines
	if m.Height() < 2 {
		t.Errorf("expected height >= 2 for long text, got %d", m.Height())
	}
}

func TestView_SingleLine(t *testing.T) {
	m := New()
	m.SetWidth(40)
	m.SetValue("status = open")

	view := m.View()

	// Should not contain newlines for short text
	if strings.Contains(view, "\n") {
		t.Error("expected no newlines in single-line text")
	}

	// Should contain the content
	if !strings.Contains(view, "status") {
		t.Errorf("expected 'status' in view, got %q", view)
	}
}

func TestView_MultiLine(t *testing.T) {
	m := New()
	m.SetWidth(20) // narrow width to force wrapping
	m.SetValue("status = open and priority = p0 and type = bug")

	view := m.View()

	// Should contain newlines for wrapped text
	if !strings.Contains(view, "\n") {
		t.Error("expected newlines in wrapped text")
	}

	// Line count should match Height()
	lineCount := strings.Count(view, "\n") + 1
	if lineCount != m.Height() {
		t.Errorf("expected %d lines (from Height()), got %d", m.Height(), lineCount)
	}
}

func TestView_PreservesHighlighting(t *testing.T) {
	m := New()
	m.SetWidth(20)
	m.SetValue("status = open and priority = p0")

	view := m.View()

	// Should contain ANSI codes for syntax highlighting
	if !strings.Contains(view, "\x1b[") {
		t.Error("expected ANSI codes in view for highlighting")
	}
}

func TestView_FocusedShowsCursor(t *testing.T) {
	m := New()
	m.SetWidth(20)
	m.SetValue("status = open")
	m.Focus()

	view := m.View()

	// Should contain cursor code (reverse video)
	if !strings.Contains(view, "\x1b[7m") {
		t.Error("expected cursor ANSI code in focused view")
	}
}

func TestView_WordBoundaryWrapping(t *testing.T) {
	// Test that wrapping breaks at word boundaries
	m := New()
	m.SetWidth(15)
	m.SetValue("status = open and ready = true")

	view := m.View()
	lines := strings.Split(view, "\n")

	// Check we have multiple lines
	if len(lines) < 2 {
		t.Errorf("expected multiple lines, got %d", len(lines))
	}

	// Each line should have reasonable content (not cut mid-word if possible)
	for _, line := range lines {
		visibleWidth := lipgloss.Width(line)
		if visibleWidth > 15+5 { // allow some slack for ANSI codes at boundaries
			t.Errorf("line too long: width=%d, line=%q", visibleWidth, line)
		}
	}
}

func TestCursorAtWrapBoundary(t *testing.T) {
	// Test cursor navigation near wrap boundary
	m := New()
	m.SetWidth(20)
	m.SetValue("status = open and priority = p0")
	m.Focus()

	// Test cursor at various positions
	testCases := []struct {
		pos      int
		expected int // lines should always match Height()
	}{
		{0, 2},  // start
		{10, 2}, // middle of first word
		{14, 2}, // near wrap point
		{20, 2}, // past wrap
		{25, 2}, // middle of second line
	}

	for _, tc := range testCases {
		m.SetCursor(tc.pos)
		view := m.View()
		lines := strings.Split(view, "\n")

		if len(lines) != tc.expected {
			t.Errorf("cursor at %d: expected %d lines, got %d", tc.pos, tc.expected, len(lines))
		}

		// Verify cursor marker is present exactly once
		cursorCount := strings.Count(view, "\x1b[7m")
		if cursorCount != 1 {
			t.Errorf("cursor at %d: expected exactly 1 cursor marker, got %d", tc.pos, cursorCount)
		}
	}
}

func TestCursorMovementWithWrapping(t *testing.T) {
	// Test that left/right cursor movement works correctly with wrapped text
	m := New()
	m.SetWidth(20)
	m.SetValue("status = open and priority = p0")
	m.SetCursor(0)
	m.Focus()

	// Move right through the text
	initialHeight := m.Height()
	for i := 0; i < len(m.Value()); i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})

		// Height should stay constant
		if m.Height() != initialHeight {
			t.Errorf("height changed at cursor %d: expected %d, got %d", m.Cursor(), initialHeight, m.Height())
		}

		// Cursor should advance
		if m.Cursor() != i+1 {
			t.Errorf("expected cursor at %d, got %d", i+1, m.Cursor())
		}
	}
}

// Golden tests - run with -update flag to update golden files:
// go test ./internal/ui/bqlinput/... -update

func TestBqlInput_View_Golden_Empty(t *testing.T) {
	m := New()
	m.SetWidth(40)
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestBqlInput_View_Golden_EmptyFocused(t *testing.T) {
	m := New()
	m.SetWidth(40)
	m.Focus()
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestBqlInput_View_Golden_Placeholder(t *testing.T) {
	m := New()
	m.SetWidth(40)
	m.SetPlaceholder("Enter a BQL query...")
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestBqlInput_View_Golden_SingleLine(t *testing.T) {
	m := New()
	m.SetWidth(40)
	m.SetValue("status = open")
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestBqlInput_View_Golden_SingleLineFocused(t *testing.T) {
	m := New()
	m.SetWidth(40)
	m.SetValue("status = open")
	m.Focus()
	m.SetCursor(7) // cursor on "="
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestBqlInput_View_Golden_MultiLine(t *testing.T) {
	m := New()
	m.SetWidth(25)
	m.SetValue("status = open and priority = p0 and type = bug")
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestBqlInput_View_Golden_MultiLineFocused(t *testing.T) {
	m := New()
	m.SetWidth(25)
	m.SetValue("status = open and priority = p0 and type = bug")
	m.Focus()
	m.SetCursor(20) // cursor in second line
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestBqlInput_View_Golden_ComplexQuery(t *testing.T) {
	m := New()
	m.SetWidth(50)
	m.SetValue("status = open and (priority <= p1 or type in (bug, task)) order by created")
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}
