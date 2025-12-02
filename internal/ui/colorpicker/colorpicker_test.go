package colorpicker

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
)

func TestNew(t *testing.T) {
	m := New()

	if len(m.columns) != 4 {
		t.Errorf("expected 4 columns, got %d", len(m.columns))
	}

	if len(m.columns[0]) != 10 {
		t.Errorf("expected 10 presets in column 1, got %d", len(m.columns[0]))
	}

	if m.column != 0 {
		t.Errorf("expected column=0, got %d", m.column)
	}

	if m.selected != 0 {
		t.Errorf("expected selected=0, got %d", m.selected)
	}

	if !m.customEnabled {
		t.Error("expected customEnabled=true")
	}

	if m.inCustomMode {
		t.Error("expected inCustomMode=false")
	}
}

func TestDefaultPresets(t *testing.T) {
	expected := []struct {
		name string
		hex  string
	}{
		{"Red", "#FF8787"},
		{"Green", "#73F59F"},
		{"Blue", "#54A0FF"},
		{"Purple", "#7D56F4"},
		{"Yellow", "#FECA57"},
		{"Orange", "#FF9F43"},
		{"Teal", "#89DCEB"},
		{"Gray", "#BBBBBB"},
		{"Pink", "#CBA6F7"},
		{"Coral", "#FF6B6B"},
	}

	if len(DefaultPresets) != len(expected) {
		t.Fatalf("expected %d presets, got %d", len(expected), len(DefaultPresets))
	}

	for i, exp := range expected {
		if DefaultPresets[i].Name != exp.name {
			t.Errorf("preset[%d]: expected name %q, got %q", i, exp.name, DefaultPresets[i].Name)
		}
		if DefaultPresets[i].Hex != exp.hex {
			t.Errorf("preset[%d]: expected hex %q, got %q", i, exp.hex, DefaultPresets[i].Hex)
		}
	}
}

func TestNavigationDown(t *testing.T) {
	m := New()

	// Navigate down within column 1
	for i := 0; i < 9; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		if m.selected != i+1 {
			t.Errorf("after %d j presses, expected selected=%d, got %d", i+1, i+1, m.selected)
		}
	}

	// Try to go beyond bounds
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.selected != 9 {
		t.Errorf("should not exceed bounds, expected selected=9, got %d", m.selected)
	}
}

func TestNavigationUp(t *testing.T) {
	m := New()
	m.selected = 5

	// Navigate up
	for i := 5; i > 0; i-- {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		if m.selected != i-1 {
			t.Errorf("expected selected=%d, got %d", i-1, m.selected)
		}
	}

	// Try to go below zero
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if m.selected != 0 {
		t.Errorf("should not go below 0, expected selected=0, got %d", m.selected)
	}
}

func TestNavigationArrowKeys(t *testing.T) {
	m := New()

	// Down arrow
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.selected != 1 {
		t.Errorf("down arrow: expected selected=1, got %d", m.selected)
	}

	// Up arrow
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.selected != 0 {
		t.Errorf("up arrow: expected selected=0, got %d", m.selected)
	}
}

func TestColumnNavigation(t *testing.T) {
	m := New()

	// Start in column 0
	if m.column != 0 {
		t.Errorf("expected to start in column 0, got %d", m.column)
	}

	// Move right with 'l' - through all 4 columns
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if m.column != 1 {
		t.Errorf("expected column=1 after 'l', got %d", m.column)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.column != 2 {
		t.Errorf("expected column=2 after right, got %d", m.column)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if m.column != 3 {
		t.Errorf("expected column=3 after 'l', got %d", m.column)
	}

	// Can't go beyond rightmost column (column 3)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if m.column != 3 {
		t.Errorf("expected column=3 (capped), got %d", m.column)
	}

	// Move left with 'h'
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if m.column != 2 {
		t.Errorf("expected column=2 after 'h', got %d", m.column)
	}

	// Move left with left arrow
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if m.column != 1 {
		t.Errorf("expected column=1 after left, got %d", m.column)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if m.column != 0 {
		t.Errorf("expected column=0 after left, got %d", m.column)
	}

	// Can't go beyond leftmost column
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if m.column != 0 {
		t.Errorf("expected column=0 (capped), got %d", m.column)
	}
}

func TestColumnNavigationClampsSelection(t *testing.T) {
	m := New()

	// Set selection to row 5
	m.selected = 5

	// Move to column 1 (should keep row 5 since all columns have 10 items)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if m.selected != 5 {
		t.Errorf("expected selected=5 preserved, got %d", m.selected)
	}
}

func TestSelectionEnter(t *testing.T) {
	m := New()
	m.selected = 2 // Blue

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}

	msg := cmd()
	selectMsg, ok := msg.(SelectMsg)
	if !ok {
		t.Fatalf("expected SelectMsg, got %T", msg)
	}

	if selectMsg.Hex != "#54A0FF" {
		t.Errorf("expected hex=#54A0FF, got %s", selectMsg.Hex)
	}
}

func TestCancellation(t *testing.T) {
	m := New()

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}

	msg := cmd()
	_, ok := msg.(CancelMsg)
	if !ok {
		t.Fatalf("expected CancelMsg, got %T", msg)
	}
}

func TestCustomModeToggle(t *testing.T) {
	m := New()

	if m.inCustomMode {
		t.Error("should start in normal mode")
	}

	// Press 'c' to enter custom mode
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})

	if !m.inCustomMode {
		t.Error("pressing 'c' should enter custom mode")
	}
}

func TestCustomModeEscape(t *testing.T) {
	m := New()

	// Enter custom mode
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if !m.inCustomMode {
		t.Fatal("should be in custom mode")
	}

	// Press Esc to exit custom mode
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if m.inCustomMode {
		t.Error("Esc should exit custom mode")
	}
}

func TestCustomModeValidHex(t *testing.T) {
	m := New()

	// Enter custom mode
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})

	// Type valid hex
	for _, r := range "#AABBCC" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Press Enter to move to Save button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Press Enter again to save
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}

	msg := cmd()
	selectMsg, ok := msg.(SelectMsg)
	if !ok {
		t.Fatalf("expected SelectMsg, got %T", msg)
	}

	if selectMsg.Hex != "#AABBCC" {
		t.Errorf("expected hex=#AABBCC, got %s", selectMsg.Hex)
	}
}

func TestCustomModeInvalidHex(t *testing.T) {
	m := New()

	// Enter custom mode
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})

	// Type invalid hex
	for _, r := range "notahex" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Press Enter - should stay in custom mode
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd != nil {
		t.Error("invalid hex should not produce a command")
	}

	if !m.inCustomMode {
		t.Error("should still be in custom mode after invalid hex")
	}
}

func TestSetSelected(t *testing.T) {
	m := New()

	// Set to a known color in column 0
	m = m.SetSelected("#7D56F4") // Purple at index 3 in column 0
	if m.column != 0 || m.selected != 3 {
		t.Errorf("expected column=0, selected=3 for Purple, got column=%d, selected=%d", m.column, m.selected)
	}

	// Case insensitive
	m = m.SetSelected("#ff8787") // Red at index 0 in column 0
	if m.column != 0 || m.selected != 0 {
		t.Errorf("expected column=0, selected=0 for Red (case insensitive), got column=%d, selected=%d", m.column, m.selected)
	}

	// Set to a color in column 1
	m = m.SetSelected("#A3E635") // Lime at index 0 in column 1
	if m.column != 1 || m.selected != 0 {
		t.Errorf("expected column=1, selected=0 for Lime, got column=%d, selected=%d", m.column, m.selected)
	}

	// Set to a color in column 2
	m = m.SetSelected("#DC143C") // Crimson at index 0 in column 2
	if m.column != 2 || m.selected != 0 {
		t.Errorf("expected column=2, selected=0 for Crimson, got column=%d, selected=%d", m.column, m.selected)
	}

	// Set to a color in column 3 (grayscale)
	m = m.SetSelected("#000000") // Black at index 9 in column 3
	if m.column != 3 || m.selected != 9 {
		t.Errorf("expected column=3, selected=9 for Black, got column=%d, selected=%d", m.column, m.selected)
	}

	// Unknown color - should default to first selection
	m.column = 2
	m.selected = 5
	m = m.SetSelected("#123456")
	if m.column != 0 || m.selected != 0 {
		t.Errorf("unknown color should default to first selection, got column=%d, selected=%d", m.column, m.selected)
	}
}

func TestSetSize(t *testing.T) {
	m := New()
	m = m.SetSize(80, 24)

	if m.viewportWidth != 80 {
		t.Errorf("expected viewportWidth=80, got %d", m.viewportWidth)
	}
	if m.viewportHeight != 24 {
		t.Errorf("expected viewportHeight=24, got %d", m.viewportHeight)
	}
}

func TestSetBoxWidth(t *testing.T) {
	m := New()
	m = m.SetBoxWidth(40)

	if m.boxWidth != 40 {
		t.Errorf("expected boxWidth=40, got %d", m.boxWidth)
	}
}

func TestSelected(t *testing.T) {
	m := New()
	m.selected = 1 // Green

	preset := m.Selected()
	if preset.Name != "Green" {
		t.Errorf("expected Name=Green, got %s", preset.Name)
	}
	if preset.Hex != "#73F59F" {
		t.Errorf("expected Hex=#73F59F, got %s", preset.Hex)
	}
}

func TestSelectedOutOfBounds(t *testing.T) {
	m := New()
	m.selected = -1

	preset := m.Selected()
	if preset.Name != "" || preset.Hex != "" {
		t.Error("out of bounds should return empty PresetColor")
	}

	m.selected = 100
	preset = m.Selected()
	if preset.Name != "" || preset.Hex != "" {
		t.Error("out of bounds should return empty PresetColor")
	}
}

func TestInCustomMode(t *testing.T) {
	m := New()

	if m.InCustomMode() {
		t.Error("should not be in custom mode initially")
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})

	if !m.InCustomMode() {
		t.Error("should be in custom mode after pressing 'c'")
	}
}

func TestCustomModeDisabled(t *testing.T) {
	m := New()
	m.customEnabled = false

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})

	if m.inCustomMode {
		t.Error("pressing 'c' should not enter custom mode when disabled")
	}
}

func TestCustomModeFocusCycling(t *testing.T) {
	m := New()

	// Enter custom mode
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if m.customFocus != customFocusInput {
		t.Error("should start on input field")
	}

	// ctrl+n cycles: Input -> Save -> Cancel -> Input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	if m.customFocus != customFocusSave {
		t.Errorf("ctrl+n from Input: expected customFocusSave, got %d", m.customFocus)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	if m.customFocus != customFocusCancel {
		t.Errorf("ctrl+n from Save: expected customFocusCancel, got %d", m.customFocus)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	if m.customFocus != customFocusInput {
		t.Errorf("ctrl+n from Cancel: expected customFocusInput (cycle), got %d", m.customFocus)
	}

	// ctrl+p cycles: Input -> Cancel -> Save -> Input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	if m.customFocus != customFocusCancel {
		t.Errorf("ctrl+p from Input: expected customFocusCancel (cycle), got %d", m.customFocus)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	if m.customFocus != customFocusSave {
		t.Errorf("ctrl+p from Cancel: expected customFocusSave, got %d", m.customFocus)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	if m.customFocus != customFocusInput {
		t.Errorf("ctrl+p from Save: expected customFocusInput, got %d", m.customFocus)
	}
}

func TestIsValidHex(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"#AABBCC", true},
		{"#aabbcc", true},
		{"#123456", true},
		{"#FF8787", true},
		{"AABBCC", false},    // Missing #
		{"#ABC", false},      // Too short
		{"#AABBCCDD", false}, // Too long
		{"#GGGGGG", false},   // Invalid chars
		{"", false},
		{"hello", false},
	}

	for _, tt := range tests {
		result := isValidHex(tt.input)
		if result != tt.valid {
			t.Errorf("isValidHex(%q): expected %v, got %v", tt.input, tt.valid, result)
		}
	}
}

func TestViewRendersWithoutPanic(t *testing.T) {
	m := New()
	m = m.SetSize(80, 24)

	// Normal mode
	view := m.View()
	if view == "" {
		t.Error("View() returned empty string in normal mode")
	}

	// Custom mode
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	view = m.View()
	if view == "" {
		t.Error("View() returned empty string in custom mode")
	}
}

func TestOverlayRendersWithoutPanic(t *testing.T) {
	m := New()
	m = m.SetSize(80, 24)

	// With empty background
	result := m.Overlay("")
	if result == "" {
		t.Error("Overlay() returned empty string with empty background")
	}

	// With background
	background := "Some background content"
	result = m.Overlay(background)
	if result == "" {
		t.Error("Overlay() returned empty string with background")
	}
}

// TestCustomModeJKInputPassthrough verifies j/k keys are passed to text input when focused.
func TestCustomModeJKInputPassthrough(t *testing.T) {
	m := New()

	// Enter custom mode - focus starts on input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if m.customFocus != customFocusInput {
		t.Fatal("expected focus on input")
	}

	// Type 'j' - should be added to input, not navigate
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.customInput.Value() != "j" {
		t.Errorf("expected input value 'j', got %q", m.customInput.Value())
	}
	if m.customFocus != customFocusInput {
		t.Errorf("focus should stay on input, got %d", m.customFocus)
	}

	// Type 'k' - should be added to input, not navigate
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if m.customInput.Value() != "jk" {
		t.Errorf("expected input value 'jk', got %q", m.customInput.Value())
	}
	if m.customFocus != customFocusInput {
		t.Errorf("focus should stay on input, got %d", m.customFocus)
	}
}

// TestSetSelectedResetsCustomMode verifies SetSelected exits custom mode.
func TestSetSelectedResetsCustomMode(t *testing.T) {
	m := New()

	// Enter custom mode
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if !m.inCustomMode {
		t.Fatal("should be in custom mode")
	}

	// SetSelected should reset to preset mode
	m = m.SetSelected("#FF8787")
	if m.inCustomMode {
		t.Error("SetSelected should exit custom mode")
	}
	if m.customFocus != customFocusInput {
		t.Error("SetSelected should reset customFocus to input")
	}
	if m.showCustomError {
		t.Error("SetSelected should clear custom error")
	}
}

// TestCustomModeJKNavigationWhenNotOnInput verifies j/k navigate when not on input.
func TestCustomModeJKNavigationWhenNotOnInput(t *testing.T) {
	m := New()

	// Enter custom mode and move to Save button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Move to Save
	if m.customFocus != customFocusSave {
		t.Fatalf("expected focus on Save, got %d", m.customFocus)
	}

	// 'j' should navigate to Cancel
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.customFocus != customFocusCancel {
		t.Errorf("expected focus on Cancel after 'j', got %d", m.customFocus)
	}

	// 'k' should navigate back to Save
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if m.customFocus != customFocusSave {
		t.Errorf("expected focus on Save after 'k', got %d", m.customFocus)
	}
}

// TestColorPicker_View_Golden uses teatest golden file comparison.
// Run with -update flag to update golden files: go test -update ./internal/ui/colorpicker/...
func TestColorPicker_View_Golden(t *testing.T) {
	m := New().SetSize(80, 24)
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestColorPicker_View_CustomMode_Golden tests the custom hex entry view.
func TestColorPicker_View_CustomMode_Golden(t *testing.T) {
	m := New().SetSize(80, 24)
	// Enter custom mode
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	// Type a valid hex color
	for _, r := range "#FF0000" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}
