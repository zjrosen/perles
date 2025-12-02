package modal

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
)

func TestNew_InputMode(t *testing.T) {
	cfg := Config{
		Title: "Test Modal",
		Inputs: []InputConfig{
			{Key: "name", Label: "Name", Placeholder: "Enter something..."},
		},
	}

	m := New(cfg)

	if !m.hasInputs {
		t.Error("expected hasInputs to be true when Inputs is set")
	}
	if len(m.inputs) != 1 {
		t.Errorf("expected 1 input, got %d", len(m.inputs))
	}
	if m.inputs[0].Placeholder != cfg.Inputs[0].Placeholder {
		t.Errorf("expected placeholder %q, got %q", cfg.Inputs[0].Placeholder, m.inputs[0].Placeholder)
	}
}

func TestNew_ConfirmMode(t *testing.T) {
	cfg := Config{
		Title:   "Confirm Delete",
		Message: "Are you sure?",
		// No Inputs = confirmation mode
	}

	m := New(cfg)

	if m.hasInputs {
		t.Error("expected hasInputs to be false when Inputs is empty")
	}
	if m.focusedInput != -1 {
		t.Errorf("expected focusedInput -1 for confirm mode, got %d", m.focusedInput)
	}
	if m.focusedField != FieldSave {
		t.Errorf("expected focusedField FieldSave for confirm mode, got %d", m.focusedField)
	}
}

func TestNew_WithInitialValue(t *testing.T) {
	cfg := Config{
		Title: "Edit Name",
		Inputs: []InputConfig{
			{Key: "name", Label: "Name", Placeholder: "Enter name...", Value: "initial value"},
		},
	}

	m := New(cfg)

	if m.inputs[0].Value() != cfg.Inputs[0].Value {
		t.Errorf("expected initial value %q, got %q", cfg.Inputs[0].Value, m.inputs[0].Value())
	}
}

func TestNew_WithMaxLength(t *testing.T) {
	cfg := Config{
		Title: "Short Input",
		Inputs: []InputConfig{
			{Key: "name", Label: "Name", Placeholder: "Enter...", MaxLength: 10},
		},
	}

	m := New(cfg)

	if m.inputs[0].CharLimit != cfg.Inputs[0].MaxLength {
		t.Errorf("expected CharLimit %d, got %d", cfg.Inputs[0].MaxLength, m.inputs[0].CharLimit)
	}
}

func TestNew_MultipleInputs(t *testing.T) {
	cfg := Config{
		Title: "Multiple Inputs",
		Inputs: []InputConfig{
			{Key: "first", Label: "First", Placeholder: "First..."},
			{Key: "second", Label: "Second", Placeholder: "Second..."},
			{Key: "third", Label: "Third", Placeholder: "Third..."},
		},
	}

	m := New(cfg)

	if len(m.inputs) != 3 {
		t.Errorf("expected 3 inputs, got %d", len(m.inputs))
	}
	if m.inputKeys[0] != "first" || m.inputKeys[1] != "second" || m.inputKeys[2] != "third" {
		t.Errorf("input keys not set correctly: %v", m.inputKeys)
	}
}

func TestInit_InputMode(t *testing.T) {
	m := New(Config{
		Title: "Test",
		Inputs: []InputConfig{
			{Key: "name", Label: "Name", Placeholder: "Enter..."},
		},
	})

	cmd := m.Init()
	if cmd == nil {
		t.Error("expected Init() to return textinput.Blink command for input mode")
	}
}

func TestInit_ConfirmMode(t *testing.T) {
	m := New(Config{
		Title: "Confirm",
	})

	cmd := m.Init()
	if cmd != nil {
		t.Error("expected Init() to return nil for confirmation mode")
	}
}

func TestUpdate_Submit(t *testing.T) {
	m := New(Config{
		Title: "Test",
		Inputs: []InputConfig{
			{Key: "name", Label: "Name", Placeholder: "Enter...", Value: "my value"},
		},
	})

	// Navigate to Save button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.focusedInput != -1 || m.focusedField != FieldSave {
		t.Fatalf("expected focus on Save button, got input=%d field=%d", m.focusedInput, m.focusedField)
	}

	// Press enter on Save
	enterMsg := tea.KeyMsg{Type: tea.KeyEnter}
	m, cmd := m.Update(enterMsg)

	if cmd == nil {
		t.Fatal("expected command from Enter key on Save")
	}

	msg := cmd()
	submitMsg, ok := msg.(SubmitMsg)
	if !ok {
		t.Fatalf("expected SubmitMsg, got %T", msg)
	}
	if submitMsg.Values["name"] != "my value" {
		t.Errorf("expected value %q, got %q", "my value", submitMsg.Values["name"])
	}
}

func TestUpdate_Cancel(t *testing.T) {
	m := New(Config{
		Title: "Test",
		Inputs: []InputConfig{
			{Key: "name", Label: "Name", Placeholder: "Enter..."},
		},
	})

	escMsg := tea.KeyMsg{Type: tea.KeyEscape}
	_, cmd := m.Update(escMsg)

	if cmd == nil {
		t.Fatal("expected command from Esc key")
	}

	msg := cmd()
	_, ok := msg.(CancelMsg)
	if !ok {
		t.Fatalf("expected CancelMsg, got %T", msg)
	}
}

func TestUpdate_CancelButton(t *testing.T) {
	m := New(Config{
		Title: "Test",
		Inputs: []InputConfig{
			{Key: "name", Label: "Name", Placeholder: "Enter..."},
		},
	})

	// Navigate to Cancel button (tab to Save, then right to Cancel)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})

	if m.focusedField != FieldCancel {
		t.Fatalf("expected focus on Cancel, got %d", m.focusedField)
	}

	// Press enter on Cancel
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("expected command from Enter on Cancel")
	}

	msg := cmd()
	_, ok := msg.(CancelMsg)
	if !ok {
		t.Fatalf("expected CancelMsg, got %T", msg)
	}
}

func TestUpdate_EmptySubmit(t *testing.T) {
	// In input mode, Save with empty input should NOT submit
	m := New(Config{
		Title: "Test",
		Inputs: []InputConfig{
			{Key: "name", Label: "Name", Placeholder: "Enter..."},
		},
	})

	// Navigate to Save button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Press enter - should not submit because input is empty
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd != nil {
		msg := cmd()
		if _, ok := msg.(SubmitMsg); ok {
			t.Error("expected no SubmitMsg when input is empty in input mode")
		}
	}
}

func TestUpdate_ConfirmSubmit(t *testing.T) {
	// In confirmation mode, Enter submits immediately (no input required)
	m := New(Config{
		Title:   "Confirm Delete",
		Message: "Are you sure?",
	})

	// Should start on Save button
	if m.focusedInput != -1 || m.focusedField != FieldSave {
		t.Fatalf("expected focus on Save, got input=%d field=%d", m.focusedInput, m.focusedField)
	}

	enterMsg := tea.KeyMsg{Type: tea.KeyEnter}
	m, cmd := m.Update(enterMsg)

	if cmd == nil {
		t.Fatal("expected command from Enter key in confirmation mode")
	}

	msg := cmd()
	submitMsg, ok := msg.(SubmitMsg)
	if !ok {
		t.Fatalf("expected SubmitMsg, got %T", msg)
	}
	if len(submitMsg.Values) != 0 {
		t.Errorf("expected empty values for confirmation mode, got %v", submitMsg.Values)
	}
}

func TestUpdate_WindowSizeMsg(t *testing.T) {
	m := New(Config{
		Title: "Test",
	})

	sizeMsg := tea.WindowSizeMsg{Width: 100, Height: 50}
	m, _ = m.Update(sizeMsg)

	if m.width != 100 {
		t.Errorf("expected width 100, got %d", m.width)
	}
	if m.height != 50 {
		t.Errorf("expected height 50, got %d", m.height)
	}
}

func TestUpdate_Navigation(t *testing.T) {
	m := New(Config{
		Title: "Test",
		Inputs: []InputConfig{
			{Key: "first", Label: "First", Placeholder: "First..."},
			{Key: "second", Label: "Second", Placeholder: "Second..."},
		},
	})

	// Should start on first input
	if m.focusedInput != 0 {
		t.Errorf("expected focusedInput 0, got %d", m.focusedInput)
	}

	// Tab to second input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.focusedInput != 1 {
		t.Errorf("expected focusedInput 1 after tab, got %d", m.focusedInput)
	}

	// Tab to Save button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.focusedInput != -1 || m.focusedField != FieldSave {
		t.Errorf("expected Save button focus, got input=%d field=%d", m.focusedInput, m.focusedField)
	}

	// Tab to Cancel button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.focusedField != FieldCancel {
		t.Errorf("expected Cancel button focus, got %d", m.focusedField)
	}

	// Tab wraps to first input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.focusedInput != 0 {
		t.Errorf("expected wrap to first input, got %d", m.focusedInput)
	}
}

func TestUpdate_NavigationReverse(t *testing.T) {
	m := New(Config{
		Title: "Test",
		Inputs: []InputConfig{
			{Key: "name", Label: "Name", Placeholder: "Name..."},
		},
	})

	// Start on input, shift+tab should go to Cancel
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.focusedField != FieldCancel {
		t.Errorf("expected Cancel from shift+tab, got %d", m.focusedField)
	}

	// Shift+tab to Save
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.focusedField != FieldSave {
		t.Errorf("expected Save from shift+tab, got %d", m.focusedField)
	}

	// Shift+tab wraps to input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.focusedInput != 0 {
		t.Errorf("expected wrap to input, got %d", m.focusedInput)
	}
}

func TestUpdate_HorizontalNavigation(t *testing.T) {
	m := New(Config{
		Title: "Test",
	})

	// Confirm mode starts on Save
	if m.focusedField != FieldSave {
		t.Fatalf("expected Save focus, got %d", m.focusedField)
	}

	// Right to Cancel
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.focusedField != FieldCancel {
		t.Errorf("expected Cancel after right, got %d", m.focusedField)
	}

	// Left back to Save
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if m.focusedField != FieldSave {
		t.Errorf("expected Save after left, got %d", m.focusedField)
	}
}

func TestView_InputMode(t *testing.T) {
	m := New(Config{
		Title: "New View",
		Inputs: []InputConfig{
			{Key: "name", Label: "View Name", Placeholder: "Enter view name..."},
		},
	})

	view := m.View()

	// Should contain title
	if !containsString(view, "New View") {
		t.Error("expected view to contain title")
	}

	// Should contain input label
	if !containsString(view, "View Name") {
		t.Error("expected view to contain input label")
	}

	// Should contain Save button
	if !containsString(view, "Save") {
		t.Error("expected view to contain 'Save' button")
	}

	// Should contain Cancel button
	if !containsString(view, "Cancel") {
		t.Error("expected view to contain 'Cancel' button")
	}
}

func TestView_ConfirmMode(t *testing.T) {
	m := New(Config{
		Title:   "Delete View",
		Message: "Are you sure you want to delete?",
	})

	view := m.View()

	// Should contain title
	if !containsString(view, "Delete View") {
		t.Error("expected view to contain title")
	}

	// Should contain message
	if !containsString(view, "Are you sure") {
		t.Error("expected view to contain message")
	}

	// Should contain Confirm button
	if !containsString(view, "Confirm") {
		t.Error("expected view to contain 'Confirm' button")
	}

	// Should contain Cancel button
	if !containsString(view, "Cancel") {
		t.Error("expected view to contain 'Cancel' button")
	}
}

func TestSetSize(t *testing.T) {
	m := New(Config{Title: "Test"})

	m.SetSize(200, 100)

	if m.width != 200 {
		t.Errorf("expected width 200, got %d", m.width)
	}
	if m.height != 100 {
		t.Errorf("expected height 100, got %d", m.height)
	}
}

func TestOverlay(t *testing.T) {
	m := New(Config{
		Title: "Test Modal",
	})
	m.SetSize(80, 24)

	// Create a simple background
	bg := ""
	for i := 0; i < 24; i++ {
		for j := 0; j < 80; j++ {
			bg += "."
		}
		if i < 23 {
			bg += "\n"
		}
	}

	result := m.Overlay(bg)

	// Result should contain modal content
	if !containsString(result, "Test Modal") {
		t.Error("expected overlay to contain modal content")
	}

	// Result should still have some background dots
	if !containsString(result, "...") {
		t.Error("expected overlay to preserve some background")
	}
}

// containsString checks if s contains substr, ignoring ANSI escape sequences
func containsString(s, substr string) bool {
	// Simple check - could be improved to strip ANSI codes
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr)) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	// Also check by iterating through runes (in case of ANSI sequences)
	return stringContains(s, substr)
}

func stringContains(s, substr string) bool {
	// Brute force search that handles ANSI codes better
	subRunes := []rune(substr)
	sRunes := []rune(s)

	for i := 0; i <= len(sRunes)-len(subRunes); i++ {
		match := true
		for j := 0; j < len(subRunes); j++ {
			if sRunes[i+j] != subRunes[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// Golden tests for modal rendering

func TestView_InputMode_Golden(t *testing.T) {
	m := New(Config{
		Title: "New View",
		Inputs: []InputConfig{
			{Key: "name", Label: "View Name", Placeholder: "Enter view name..."},
		},
	})
	teatest.RequireEqualOutput(t, []byte(m.View()))
}

func TestView_ConfirmMode_Golden(t *testing.T) {
	m := New(Config{
		Title:   "Delete View",
		Message: "Are you sure you want to delete?",
	})
	teatest.RequireEqualOutput(t, []byte(m.View()))
}

func TestView_MultipleInputs_Golden(t *testing.T) {
	m := New(Config{
		Title: "Create Item",
		Inputs: []InputConfig{
			{Key: "name", Label: "Name", Placeholder: "Enter name..."},
			{Key: "color", Label: "Color", Placeholder: "#RRGGBB"},
		},
	})
	teatest.RequireEqualOutput(t, []byte(m.View()))
}
