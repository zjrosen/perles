package formmodal

import (
	"errors"
	"os"
	"path/filepath"
	"perles/internal/ui/shared/colorpicker"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// getValues extracts field values from the model (test helper, accesses internal state)
func getValues(m Model) map[string]any {
	values := make(map[string]any)
	for i := range m.fields {
		values[m.fields[i].config.Key] = m.fields[i].value()
	}
	return values
}

// --- Focus Cycling Tests ---

func TestFocusCycling_Forward(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{Key: "field1", Type: FieldTypeText, Label: "Field 1"},
			{Key: "field2", Type: FieldTypeText, Label: "Field 2"},
		},
	}
	m := New(cfg)

	// Start on first field
	if m.focusedIndex != 0 {
		t.Errorf("expected focused index 0, got %d", m.focusedIndex)
	}

	// Tab to second field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.focusedIndex != 1 {
		t.Errorf("expected focused index 1, got %d", m.focusedIndex)
	}

	// Tab to submit button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.focusedIndex != -1 {
		t.Errorf("expected focused index -1 (buttons), got %d", m.focusedIndex)
	}
	if m.focusedButton != 0 {
		t.Errorf("expected focused button 0 (submit), got %d", m.focusedButton)
	}

	// Tab to cancel button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.focusedButton != 1 {
		t.Errorf("expected focused button 1 (cancel), got %d", m.focusedButton)
	}

	// Tab wraps to first field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.focusedIndex != 0 {
		t.Errorf("expected focused index 0 (wrapped), got %d", m.focusedIndex)
	}
}

func TestFocusCycling_Reverse(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{Key: "field1", Type: FieldTypeText, Label: "Field 1"},
			{Key: "field2", Type: FieldTypeText, Label: "Field 2"},
		},
	}
	m := New(cfg)

	// Start on first field
	if m.focusedIndex != 0 {
		t.Errorf("expected focused index 0, got %d", m.focusedIndex)
	}

	// Shift+Tab wraps to cancel button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.focusedIndex != -1 {
		t.Errorf("expected focused index -1 (buttons), got %d", m.focusedIndex)
	}
	if m.focusedButton != 1 {
		t.Errorf("expected focused button 1 (cancel), got %d", m.focusedButton)
	}

	// Shift+Tab to submit button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.focusedButton != 0 {
		t.Errorf("expected focused button 0 (submit), got %d", m.focusedButton)
	}

	// Shift+Tab to second field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.focusedIndex != 1 {
		t.Errorf("expected focused index 1, got %d", m.focusedIndex)
	}

	// Shift+Tab to first field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.focusedIndex != 0 {
		t.Errorf("expected focused index 0, got %d", m.focusedIndex)
	}
}

func TestFocusCycling_NoFields(t *testing.T) {
	cfg := FormConfig{
		Title: "Confirm",
	}
	m := New(cfg)

	// Start on submit button
	if m.focusedIndex != -1 {
		t.Errorf("expected focused index -1 (buttons), got %d", m.focusedIndex)
	}
	if m.focusedButton != 0 {
		t.Errorf("expected focused button 0 (submit), got %d", m.focusedButton)
	}

	// Tab to cancel button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.focusedButton != 1 {
		t.Errorf("expected focused button 1 (cancel), got %d", m.focusedButton)
	}

	// Tab wraps to submit button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.focusedButton != 0 {
		t.Errorf("expected focused button 0 (submit), got %d", m.focusedButton)
	}
}

// --- Keyboard Navigation Tests ---

func TestKeyboard_CtrlN_CtrlP(t *testing.T) {
	cfg := FormConfig{
		Title:  "Test Form",
		Fields: []FieldConfig{{Key: "field1", Type: FieldTypeText, Label: "Field 1"}},
	}
	m := New(cfg)

	// Ctrl+N should advance like Tab
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}, Alt: false})
	// Note: tea.KeyMsg with ctrl+n comes as string "ctrl+n"
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	if m.focusedIndex != -1 {
		t.Errorf("ctrl+n: expected focused index -1, got %d", m.focusedIndex)
	}

	// Ctrl+P should go back like Shift+Tab
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	if m.focusedIndex != 0 {
		t.Errorf("ctrl+p: expected focused index 0, got %d", m.focusedIndex)
	}
}

func TestKeyboard_Enter_AdvancesField(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{Key: "field1", Type: FieldTypeText, Label: "Field 1"},
			{Key: "field2", Type: FieldTypeText, Label: "Field 2"},
		},
	}
	m := New(cfg)

	// Enter on first field advances to second
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.focusedIndex != 1 {
		t.Errorf("expected focused index 1, got %d", m.focusedIndex)
	}
}

func TestKeyboard_Esc_SendsCancelMsg(t *testing.T) {
	cfg := FormConfig{
		Title:  "Test Form",
		Fields: []FieldConfig{{Key: "field1", Type: FieldTypeText, Label: "Field 1"}},
	}
	m := New(cfg)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected cancel command, got nil")
	}
	msg := cmd()
	if _, ok := msg.(CancelMsg); !ok {
		t.Errorf("expected CancelMsg, got %T", msg)
	}
}

func TestKeyboard_ButtonNavigation_LeftRight(t *testing.T) {
	cfg := FormConfig{
		Title: "Confirm",
	}
	m := New(cfg)

	// Start on submit button (0)
	if m.focusedButton != 0 {
		t.Errorf("expected button 0, got %d", m.focusedButton)
	}

	// Right/l moves to cancel
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.focusedButton != 1 {
		t.Errorf("expected button 1 after right, got %d", m.focusedButton)
	}

	// Left/h moves back to submit
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if m.focusedButton != 0 {
		t.Errorf("expected button 0 after left, got %d", m.focusedButton)
	}

	// Test with h/l keys
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if m.focusedButton != 1 {
		t.Errorf("expected button 1 after 'l', got %d", m.focusedButton)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if m.focusedButton != 0 {
		t.Errorf("expected button 0 after 'h', got %d", m.focusedButton)
	}
}

// --- Submit Tests ---

func TestSubmit_EnterOnSubmitButton(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{Key: "name", Type: FieldTypeText, Label: "Name", InitialValue: "test"},
		},
	}
	m := New(cfg)

	// Navigate to submit button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // to submit

	// Press Enter
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected submit command, got nil")
	}
	msg := cmd()
	submitMsg, ok := msg.(SubmitMsg)
	if !ok {
		t.Fatalf("expected SubmitMsg, got %T", msg)
	}
	if submitMsg.Values["name"] != "test" {
		t.Errorf("expected name='test', got %v", submitMsg.Values["name"])
	}
}

func TestSubmit_EnterOnCancelButton(t *testing.T) {
	cfg := FormConfig{
		Title: "Confirm",
	}
	m := New(cfg)

	// Navigate to cancel button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // to cancel

	// Press Enter
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected cancel command, got nil")
	}
	msg := cmd()
	if _, ok := msg.(CancelMsg); !ok {
		t.Errorf("expected CancelMsg, got %T", msg)
	}
}

// --- Validation Tests ---

func TestValidation_Error(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{Key: "name", Type: FieldTypeText, Label: "Name"},
		},
		Validate: func(values map[string]any) error {
			name := values["name"].(string)
			if strings.TrimSpace(name) == "" {
				return errors.New("Name is required")
			}
			return nil
		},
	}
	m := New(cfg)

	// Navigate to submit and press Enter with empty name
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // to submit
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should have validation error, no command
	if cmd != nil {
		t.Error("expected nil command due to validation error")
	}
	if m.validationError != "Name is required" {
		t.Errorf("expected validation error 'Name is required', got '%s'", m.validationError)
	}
}

func TestValidation_Success(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{Key: "name", Type: FieldTypeText, Label: "Name", InitialValue: "Alice"},
		},
		Validate: func(values map[string]any) error {
			name := values["name"].(string)
			if strings.TrimSpace(name) == "" {
				return errors.New("Name is required")
			}
			return nil
		},
	}
	m := New(cfg)

	// Navigate to submit and press Enter
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // to submit
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should succeed
	if cmd == nil {
		t.Fatal("expected submit command, got nil")
	}
	msg := cmd()
	if _, ok := msg.(SubmitMsg); !ok {
		t.Errorf("expected SubmitMsg, got %T", msg)
	}
}

// --- List Field Tests ---

func TestListField_Navigation(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:   "items",
				Type:  FieldTypeList,
				Label: "Items",
				Options: []ListOption{
					{Label: "Item 1", Value: "1"},
					{Label: "Item 2", Value: "2"},
					{Label: "Item 3", Value: "3"},
				},
			},
		},
	}
	m := New(cfg)

	// Cursor starts at 0
	if m.fields[0].listCursor != 0 {
		t.Errorf("expected cursor at 0, got %d", m.fields[0].listCursor)
	}

	// j/down moves cursor down
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.fields[0].listCursor != 1 {
		t.Errorf("expected cursor at 1 after 'j', got %d", m.fields[0].listCursor)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.fields[0].listCursor != 2 {
		t.Errorf("expected cursor at 2 after down, got %d", m.fields[0].listCursor)
	}

	// At boundary, doesn't go past
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.fields[0].listCursor != 2 {
		t.Errorf("expected cursor at 2 (boundary), got %d", m.fields[0].listCursor)
	}

	// k/up moves cursor up
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if m.fields[0].listCursor != 1 {
		t.Errorf("expected cursor at 1 after 'k', got %d", m.fields[0].listCursor)
	}
}

func TestListField_Selection_MultiSelect(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:         "items",
				Type:        FieldTypeList,
				Label:       "Items",
				MultiSelect: true,
				Options: []ListOption{
					{Label: "Item 1", Value: "1"},
					{Label: "Item 2", Value: "2"},
				},
			},
		},
	}
	m := New(cfg)

	// Space toggles selection
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if !m.fields[0].listItems[0].selected {
		t.Error("expected item 0 to be selected after space")
	}

	// Move to item 2 and select it too
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if !m.fields[0].listItems[1].selected {
		t.Error("expected item 1 to be selected after space")
	}

	// Both items should be selected (multi-select)
	if !m.fields[0].listItems[0].selected {
		t.Error("expected item 0 to remain selected in multi-select")
	}
}

func TestListField_Selection_SingleSelect(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:         "items",
				Type:        FieldTypeList,
				Label:       "Items",
				MultiSelect: false, // Single-select mode
				Options: []ListOption{
					{Label: "Item 1", Value: "1"},
					{Label: "Item 2", Value: "2"},
					{Label: "Item 3", Value: "3"},
				},
			},
		},
	}
	m := New(cfg)

	// Select first item
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if !m.fields[0].listItems[0].selected {
		t.Error("expected item 0 to be selected")
	}

	// Move to second item and select
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})

	// Second item should be selected
	if !m.fields[0].listItems[1].selected {
		t.Error("expected item 1 to be selected")
	}

	// First item should be deselected (single-select behavior)
	if m.fields[0].listItems[0].selected {
		t.Error("expected item 0 to be deselected in single-select mode")
	}
}

func TestListField_TabExitsList(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:   "items",
				Type:  FieldTypeList,
				Label: "Items",
				Options: []ListOption{
					{Label: "Item 1", Value: "1"},
					{Label: "Item 2", Value: "2"},
				},
			},
		},
	}
	m := New(cfg)

	// Start on list field
	if m.focusedIndex != 0 {
		t.Errorf("expected focused index 0, got %d", m.focusedIndex)
	}

	// Tab should move to buttons
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.focusedIndex != -1 {
		t.Errorf("expected focus on buttons (-1), got %d", m.focusedIndex)
	}
}

func TestListField_ShiftTabEntersFromNextField(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:   "items",
				Type:  FieldTypeList,
				Label: "Items",
				Options: []ListOption{
					{Label: "Item 1", Value: "1"},
				},
			},
			{Key: "name", Type: FieldTypeText, Label: "Name"},
		},
	}
	m := New(cfg)

	// Move to second field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.focusedIndex != 1 {
		t.Errorf("expected focused index 1, got %d", m.focusedIndex)
	}

	// Shift+Tab should go back to list
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.focusedIndex != 0 {
		t.Errorf("expected focused index 0 (list), got %d", m.focusedIndex)
	}
}

func TestListField_SubmitIncludesSelectedValues(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:         "items",
				Type:        FieldTypeList,
				Label:       "Items",
				MultiSelect: true,
				Options: []ListOption{
					{Label: "Item 1", Value: "val1"},
					{Label: "Item 2", Value: "val2", Selected: true},
					{Label: "Item 3", Value: "val3"},
				},
			},
		},
	}
	m := New(cfg)

	// Select first item too
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})

	// Navigate to submit and press Enter
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // to buttons
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("expected submit command, got nil")
	}
	msg := cmd()
	submitMsg, ok := msg.(SubmitMsg)
	if !ok {
		t.Fatalf("expected SubmitMsg, got %T", msg)
	}

	selected, ok := submitMsg.Values["items"].([]string)
	if !ok {
		t.Fatalf("expected []string for items, got %T", submitMsg.Values["items"])
	}

	// Should contain val1 (selected via space) and val2 (pre-selected)
	if len(selected) != 2 {
		t.Errorf("expected 2 selected items, got %d", len(selected))
	}
	// Check both values are present
	hasVal1, hasVal2 := false, false
	for _, v := range selected {
		if v == "val1" {
			hasVal1 = true
		}
		if v == "val2" {
			hasVal2 = true
		}
	}
	if !hasVal1 || !hasVal2 {
		t.Errorf("expected val1 and val2 in selected, got %v", selected)
	}
}

func TestListField_EmptyList(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:     "items",
				Type:    FieldTypeList,
				Label:   "Items",
				Options: []ListOption{}, // Empty list
			},
		},
		MinWidth: 50,
	}
	m := New(cfg).SetSize(80, 24)

	// Should render without panic
	view := m.View()
	if !strings.Contains(view, "(no items)") {
		t.Error("expected empty list to show '(no items)'")
	}
}

// --- Color Field Tests ---

func TestColorField_EnterOpensColorPicker(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{Key: "color", Type: FieldTypeColor, Label: "Color", InitialColor: "#73F59F"},
		},
	}
	m := New(cfg).SetSize(80, 24)

	// Initially colorpicker not shown
	if m.showColorPicker {
		t.Error("expected colorpicker to be hidden initially")
	}

	// Enter on color field opens colorpicker
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.showColorPicker {
		t.Error("expected colorpicker to be shown after Enter")
	}
}

func TestColorField_SelectMsgUpdatesColor(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{Key: "color", Type: FieldTypeColor, Label: "Color", InitialColor: "#73F59F"},
		},
	}
	m := New(cfg).SetSize(80, 24)

	// Open colorpicker
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Simulate colorpicker.SelectMsg
	m, _ = m.Update(colorpicker.SelectMsg{Hex: "#FF8787"})

	// Check colorpicker is closed
	if m.showColorPicker {
		t.Error("expected colorpicker to be closed after SelectMsg")
	}

	// Check color was updated
	values := getValues(m)
	if values["color"] != "#FF8787" {
		t.Errorf("expected color '#FF8787', got %v", values["color"])
	}
}

func TestColorField_CancelMsgKeepsOriginalColor(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{Key: "color", Type: FieldTypeColor, Label: "Color", InitialColor: "#73F59F"},
		},
	}
	m := New(cfg).SetSize(80, 24)

	// Open colorpicker
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Simulate colorpicker.CancelMsg
	m, _ = m.Update(colorpicker.CancelMsg{})

	// Check colorpicker is closed
	if m.showColorPicker {
		t.Error("expected colorpicker to be closed after CancelMsg")
	}

	// Check color was NOT changed
	values := getValues(m)
	if values["color"] != "#73F59F" {
		t.Errorf("expected color '#73F59F', got %v", values["color"])
	}
}

func TestColorField_TabSkipsWithoutOpeningPicker(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{Key: "color", Type: FieldTypeColor, Label: "Color"},
		},
	}
	m := New(cfg).SetSize(80, 24)

	// Tab should move to buttons, not open colorpicker
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	if m.showColorPicker {
		t.Error("Tab should not open colorpicker")
	}
	if m.focusedIndex != -1 {
		t.Errorf("expected focus on buttons (-1), got %d", m.focusedIndex)
	}
}

func TestColorField_DefaultColor(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{Key: "color", Type: FieldTypeColor, Label: "Color"}, // No InitialColor
		},
	}
	m := New(cfg)

	// Should default to #73F59F
	values := getValues(m)
	if values["color"] != "#73F59F" {
		t.Errorf("expected default color '#73F59F', got %v", values["color"])
	}
}

func TestColorField_SubmitIncludesColor(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{Key: "color", Type: FieldTypeColor, Label: "Color", InitialColor: "#FF8787"},
		},
	}
	m := New(cfg).SetSize(80, 24)

	// Navigate to submit and press Enter
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // to buttons
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("expected submit command, got nil")
	}
	msg := cmd()
	submitMsg, ok := msg.(SubmitMsg)
	if !ok {
		t.Fatalf("expected SubmitMsg, got %T", msg)
	}
	if submitMsg.Values["color"] != "#FF8787" {
		t.Errorf("expected color '#FF8787' in SubmitMsg, got %v", submitMsg.Values["color"])
	}
}

// --- Golden Tests ---

func TestGolden_TextFieldFocused(t *testing.T) {
	cfg := FormConfig{
		Title: "Create View",
		Fields: []FieldConfig{
			{Key: "name", Type: FieldTypeText, Label: "Name", Hint: "required"},
		},
		SubmitLabel: "Save",
		MinWidth:    50,
	}
	m := New(cfg).SetSize(80, 24)

	compareGolden(t, "text_field_focused", m.View())
}

func TestGolden_ButtonFocused(t *testing.T) {
	cfg := FormConfig{
		Title: "Create View",
		Fields: []FieldConfig{
			{Key: "name", Type: FieldTypeText, Label: "Name", Hint: "required"},
		},
		SubmitLabel: "Save",
		MinWidth:    50,
	}
	m := New(cfg).SetSize(80, 24)
	// Navigate to button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	compareGolden(t, "button_focused", m.View())
}

func TestGolden_ValidationError(t *testing.T) {
	cfg := FormConfig{
		Title: "Create View",
		Fields: []FieldConfig{
			{Key: "name", Type: FieldTypeText, Label: "Name", Hint: "required"},
		},
		SubmitLabel: "Save",
		MinWidth:    50,
		Validate: func(values map[string]any) error {
			return errors.New("Name is required")
		},
	}
	m := New(cfg).SetSize(80, 24)
	// Navigate to submit and trigger validation
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	compareGolden(t, "validation_error", m.View())
}

func TestGolden_MultipleFields(t *testing.T) {
	cfg := FormConfig{
		Title: "Create View",
		Fields: []FieldConfig{
			{Key: "viewName", Type: FieldTypeText, Label: "View Name", Hint: "required"},
			{Key: "columnName", Type: FieldTypeText, Label: "Column Name", Hint: "optional"},
		},
		SubmitLabel: "Save",
		MinWidth:    50,
	}
	m := New(cfg).SetSize(80, 24)
	// Focus second field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	compareGolden(t, "multiple_fields", m.View())
}

func TestGolden_ColorFieldFocused(t *testing.T) {
	cfg := FormConfig{
		Title: "Create View",
		Fields: []FieldConfig{
			{Key: "color", Type: FieldTypeColor, Label: "Color", Hint: "Enter to change", InitialColor: "#73F59F"},
		},
		SubmitLabel: "Save",
		MinWidth:    50,
	}
	m := New(cfg).SetSize(80, 24)

	compareGolden(t, "color_field_focused", m.View())
}

func TestGolden_ListFieldFocused(t *testing.T) {
	cfg := FormConfig{
		Title: "Add to Views",
		Fields: []FieldConfig{
			{
				Key:         "views",
				Type:        FieldTypeList,
				Label:       "Views",
				Hint:        "Space to toggle",
				MultiSelect: true,
				Options: []ListOption{
					{Label: "Backlog", Value: "0"},
					{Label: "Sprint", Value: "1", Selected: true}, // Pre-selected
					{Label: "Archive", Value: "2"},
				},
			},
		},
		SubmitLabel: "Save",
		MinWidth:    50,
	}
	m := New(cfg).SetSize(80, 24)
	// Move cursor to second item (Sprint)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	compareGolden(t, "list_field_focused", m.View())
}

// compareGolden compares output against a golden file.
// Set UPDATE_GOLDEN=1 to update golden files.
func compareGolden(t *testing.T, name, got string) {
	t.Helper()
	goldenPath := filepath.Join("testdata", name+".golden")

	if os.Getenv("UPDATE_GOLDEN") == "1" {
		err := os.WriteFile(goldenPath, []byte(got), 0644)
		if err != nil {
			t.Fatalf("failed to write golden file: %v", err)
		}
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("failed to read golden file %s: %v (run with UPDATE_GOLDEN=1 to create)", goldenPath, err)
	}

	if string(want) != got {
		t.Errorf("output does not match golden file %s\n\nWant:\n%s\n\nGot:\n%s", goldenPath, string(want), got)
	}
}

// --- Editable List Field Tests ---

func TestEditableListField_InitialState(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:  "tags",
				Type: FieldTypeEditableList,
				Options: []ListOption{
					{Label: "one", Value: "1", Selected: true},
					{Label: "two", Value: "2", Selected: false},
				},
			},
		},
	}
	m := New(cfg)

	// Initial focus should be on list
	if m.fields[0].subFocus != SubFocusList {
		t.Errorf("expected subFocus SubFocusList (0), got %d", m.fields[0].subFocus)
	}

	// Cursor should be at 0
	if m.fields[0].listCursor != 0 {
		t.Errorf("expected listCursor 0, got %d", m.fields[0].listCursor)
	}

	// Should have 2 list items
	if len(m.fields[0].listItems) != 2 {
		t.Errorf("expected 2 list items, got %d", len(m.fields[0].listItems))
	}
}

func TestEditableListField_Navigation_Tab(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:  "tags",
				Type: FieldTypeEditableList,
				Options: []ListOption{
					{Label: "one", Value: "1", Selected: true},
					{Label: "two", Value: "2", Selected: false},
				},
			},
		},
	}
	m := New(cfg)

	// Initial focus should be on list
	if m.fields[0].subFocus != SubFocusList {
		t.Errorf("expected SubFocusList, got %d", m.fields[0].subFocus)
	}

	// Tab moves to input within same field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.fields[0].subFocus != SubFocusInput {
		t.Errorf("expected SubFocusInput after Tab, got %d", m.fields[0].subFocus)
	}
	if m.focusedIndex != 0 {
		t.Errorf("expected focusedIndex 0 (same field), got %d", m.focusedIndex)
	}

	// Tab again moves to buttons
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.focusedIndex != -1 {
		t.Errorf("expected focusedIndex -1 (buttons), got %d", m.focusedIndex)
	}
}

func TestEditableListField_Navigation_ShiftTab(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:  "tags",
				Type: FieldTypeEditableList,
				Options: []ListOption{
					{Label: "one", Value: "1", Selected: true},
					{Label: "two", Value: "2", Selected: false},
				},
			},
		},
	}
	m := New(cfg)

	// Move to input first
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.fields[0].subFocus != SubFocusInput {
		t.Errorf("expected SubFocusInput, got %d", m.fields[0].subFocus)
	}

	// Shift+Tab moves back to list
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.fields[0].subFocus != SubFocusList {
		t.Errorf("expected SubFocusList after Shift+Tab, got %d", m.fields[0].subFocus)
	}
	// Cursor should be at bottom of list
	if m.fields[0].listCursor != 1 {
		t.Errorf("expected listCursor at bottom (1), got %d", m.fields[0].listCursor)
	}

	// Shift+Tab from list moves to cancel button (wraps)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.focusedIndex != -1 {
		t.Errorf("expected focusedIndex -1 (buttons), got %d", m.focusedIndex)
	}
	if m.focusedButton != 1 {
		t.Errorf("expected focusedButton 1 (cancel), got %d", m.focusedButton)
	}
}

func TestEditableListField_Navigation_JK(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:  "tags",
				Type: FieldTypeEditableList,
				Options: []ListOption{
					{Label: "one", Value: "1"},
					{Label: "two", Value: "2"},
					{Label: "three", Value: "3"},
				},
			},
		},
	}
	m := New(cfg)

	// Cursor starts at 0
	if m.fields[0].listCursor != 0 {
		t.Errorf("expected cursor at 0, got %d", m.fields[0].listCursor)
	}

	// j moves cursor down
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.fields[0].listCursor != 1 {
		t.Errorf("expected cursor at 1 after 'j', got %d", m.fields[0].listCursor)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.fields[0].listCursor != 2 {
		t.Errorf("expected cursor at 2 after down, got %d", m.fields[0].listCursor)
	}

	// At boundary, doesn't go past
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.fields[0].listCursor != 2 {
		t.Errorf("expected cursor at 2 (boundary), got %d", m.fields[0].listCursor)
	}

	// k moves cursor up
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if m.fields[0].listCursor != 1 {
		t.Errorf("expected cursor at 1 after 'k', got %d", m.fields[0].listCursor)
	}
}

func TestEditableListField_Navigation_UpFromTop(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:  "tags",
				Type: FieldTypeEditableList,
				Options: []ListOption{
					{Label: "one", Value: "1"},
					{Label: "two", Value: "2"},
				},
			},
		},
	}
	m := New(cfg)

	// Cursor at 0, k/up should move to input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if m.fields[0].subFocus != SubFocusInput {
		t.Errorf("expected SubFocusInput after k at top, got %d", m.fields[0].subFocus)
	}
}

func TestEditableListField_Navigation_DownFromInput(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:  "tags",
				Type: FieldTypeEditableList,
				Options: []ListOption{
					{Label: "one", Value: "1"},
				},
			},
			{Key: "name", Type: FieldTypeText, Label: "Name"},
		},
	}
	m := New(cfg)

	// Move to input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.fields[0].subFocus != SubFocusInput {
		t.Errorf("expected SubFocusInput, got %d", m.fields[0].subFocus)
	}

	// Down from input moves to next field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.focusedIndex != 1 {
		t.Errorf("expected focusedIndex 1 after down from input, got %d", m.focusedIndex)
	}
}

func TestEditableListField_Navigation_UpFromInput(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:  "tags",
				Type: FieldTypeEditableList,
				Options: []ListOption{
					{Label: "one", Value: "1"},
					{Label: "two", Value: "2"},
				},
			},
		},
	}
	m := New(cfg)

	// Move to input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Up from input moves to list at bottom
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.fields[0].subFocus != SubFocusList {
		t.Errorf("expected SubFocusList after up from input, got %d", m.fields[0].subFocus)
	}
	if m.fields[0].listCursor != 1 {
		t.Errorf("expected listCursor at bottom (1), got %d", m.fields[0].listCursor)
	}
}

func TestEditableListField_Toggle_Space(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:  "tags",
				Type: FieldTypeEditableList,
				Options: []ListOption{
					{Label: "one", Value: "1", Selected: false},
					{Label: "two", Value: "2", Selected: true},
				},
			},
		},
	}
	m := New(cfg)

	// Initial: item 0 not selected, item 1 selected
	if m.fields[0].listItems[0].selected {
		t.Error("expected item 0 to be unselected initially")
	}
	if !m.fields[0].listItems[1].selected {
		t.Error("expected item 1 to be selected initially")
	}

	// Space toggles selection of item at cursor
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if !m.fields[0].listItems[0].selected {
		t.Error("expected item 0 to be selected after space")
	}

	// Toggle again
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if m.fields[0].listItems[0].selected {
		t.Error("expected item 0 to be unselected after second space")
	}
}

func TestEditableListField_Toggle_EnterInList(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:  "tags",
				Type: FieldTypeEditableList,
				Options: []ListOption{
					{Label: "one", Value: "1", Selected: false},
				},
			},
		},
	}
	m := New(cfg)

	// Enter in list toggles selection
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.fields[0].listItems[0].selected {
		t.Error("expected item 0 to be selected after enter")
	}
}

func TestEditableListField_AddItem(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:  "tags",
				Type: FieldTypeEditableList,
			},
		},
	}
	m := New(cfg)

	// Should start with empty list
	if len(m.fields[0].listItems) != 0 {
		t.Errorf("expected 0 items, got %d", len(m.fields[0].listItems))
	}

	// Move to input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Type "newitem"
	for _, r := range "newitem" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Press Enter to add
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Verify item was added
	if len(m.fields[0].listItems) != 1 {
		t.Fatalf("expected 1 item, got %d", len(m.fields[0].listItems))
	}
	if m.fields[0].listItems[0].value != "newitem" {
		t.Errorf("expected value 'newitem', got '%s'", m.fields[0].listItems[0].value)
	}
	if m.fields[0].listItems[0].label != "newitem" {
		t.Errorf("expected label 'newitem', got '%s'", m.fields[0].listItems[0].label)
	}
	if !m.fields[0].listItems[0].selected {
		t.Error("expected new item to be selected")
	}

	// Input should be cleared
	if m.fields[0].addInput.Value() != "" {
		t.Errorf("expected input to be cleared, got '%s'", m.fields[0].addInput.Value())
	}
}

func TestEditableListField_AddItem_TrimWhitespace(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:  "tags",
				Type: FieldTypeEditableList,
			},
		},
	}
	m := New(cfg)

	// Move to input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Type " test " with leading/trailing spaces
	for _, r := range "  test  " {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Press Enter to add
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Verify item was added with trimmed value
	if len(m.fields[0].listItems) != 1 {
		t.Fatalf("expected 1 item, got %d", len(m.fields[0].listItems))
	}
	if m.fields[0].listItems[0].value != "test" {
		t.Errorf("expected trimmed value 'test', got '%s'", m.fields[0].listItems[0].value)
	}
}

func TestEditableListField_AddItem_EmptyIgnored(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:  "tags",
				Type: FieldTypeEditableList,
			},
		},
	}
	m := New(cfg)

	// Move to input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Press Enter with empty input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// No item should be added
	if len(m.fields[0].listItems) != 0 {
		t.Errorf("expected 0 items, got %d", len(m.fields[0].listItems))
	}

	// Try with only whitespace
	for _, r := range "   " {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(m.fields[0].listItems) != 0 {
		t.Errorf("expected 0 items for whitespace-only input, got %d", len(m.fields[0].listItems))
	}
}

func TestEditableListField_NoDuplicates(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:  "tags",
				Type: FieldTypeEditableList,
				Options: []ListOption{
					{Label: "existing", Value: "existing", Selected: true},
				},
				AllowDuplicates: false, // Default
			},
		},
	}
	m := New(cfg)

	// Move to input and try to add "existing"
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	for _, r := range "existing" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should still have only 1 item
	if len(m.fields[0].listItems) != 1 {
		t.Errorf("expected 1 item (duplicate rejected), got %d", len(m.fields[0].listItems))
	}
}

func TestEditableListField_AllowDuplicates(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:  "tags",
				Type: FieldTypeEditableList,
				Options: []ListOption{
					{Label: "existing", Value: "existing", Selected: true},
				},
				AllowDuplicates: true,
			},
		},
	}
	m := New(cfg)

	// Move to input and add "existing"
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	for _, r := range "existing" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should have 2 items (duplicates allowed)
	if len(m.fields[0].listItems) != 2 {
		t.Errorf("expected 2 items (duplicates allowed), got %d", len(m.fields[0].listItems))
	}
}

func TestEditableListField_ValueExtraction(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:  "tags",
				Type: FieldTypeEditableList,
				Options: []ListOption{
					{Label: "one", Value: "val1", Selected: true},
					{Label: "two", Value: "val2", Selected: false},
					{Label: "three", Value: "val3", Selected: true},
				},
			},
		},
	}
	m := New(cfg)

	values := getValues(m)
	selected, ok := values["tags"].([]string)
	if !ok {
		t.Fatalf("expected []string, got %T", values["tags"])
	}

	// Should contain val1 and val3 (the selected items)
	if len(selected) != 2 {
		t.Errorf("expected 2 selected items, got %d", len(selected))
	}

	hasVal1, hasVal3 := false, false
	for _, v := range selected {
		if v == "val1" {
			hasVal1 = true
		}
		if v == "val3" {
			hasVal3 = true
		}
	}
	if !hasVal1 || !hasVal3 {
		t.Errorf("expected val1 and val3 in selected, got %v", selected)
	}
}

func TestEditableListField_SubmitIncludesValues(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:  "tags",
				Type: FieldTypeEditableList,
				Options: []ListOption{
					{Label: "bug", Value: "bug", Selected: true},
					{Label: "feature", Value: "feature", Selected: false},
				},
			},
		},
	}
	m := New(cfg)

	// Toggle feature (make it selected)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}) // Move to feature
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})                     // Select it

	// Navigate to submit button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // submit

	// Submit
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected submit command, got nil")
	}
	msg := cmd()
	submitMsg, ok := msg.(SubmitMsg)
	if !ok {
		t.Fatalf("expected SubmitMsg, got %T", msg)
	}

	selected, ok := submitMsg.Values["tags"].([]string)
	if !ok {
		t.Fatalf("expected []string, got %T", submitMsg.Values["tags"])
	}

	if len(selected) != 2 {
		t.Errorf("expected 2 selected items, got %d", len(selected))
	}
}

func TestEditableListField_EmptyList_Navigation(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:  "tags",
				Type: FieldTypeEditableList,
				// No initial options
			},
		},
	}
	m := New(cfg)

	// j on empty list should not crash (doesn't move - no items)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.fields[0].subFocus != SubFocusList {
		t.Errorf("after j: expected SubFocusList, got %d", m.fields[0].subFocus)
	}

	// k on empty list at cursor 0 wraps to input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if m.fields[0].subFocus != SubFocusInput {
		t.Errorf("after k: expected SubFocusInput (wrap from top), got %d", m.fields[0].subFocus)
	}

	// Go back to list
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.fields[0].subFocus != SubFocusList {
		t.Errorf("after up: expected SubFocusList, got %d", m.fields[0].subFocus)
	}

	// Space on empty list should not crash (does nothing)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})

	// Enter on empty list should not crash (does nothing in list mode)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Tab should navigate to input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.fields[0].subFocus != SubFocusInput {
		t.Errorf("after Tab: expected SubFocusInput, got %d", m.fields[0].subFocus)
	}
}

func TestEditableListField_SpaceInInput(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:  "tags",
				Type: FieldTypeEditableList,
			},
		},
	}
	m := New(cfg)

	// Move to input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Type "hello world" (with space)
	for _, r := range "hello world" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Verify space was typed into input
	if m.fields[0].addInput.Value() != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", m.fields[0].addInput.Value())
	}
}

// --- Editable List Golden Tests ---

func TestGolden_EditableListFocusedOnList(t *testing.T) {
	cfg := FormConfig{
		Title: "Edit Tags",
		Fields: []FieldConfig{
			{
				Key:              "tags",
				Type:             FieldTypeEditableList,
				Label:            "Tags",
				Hint:             "Space to toggle",
				InputLabel:       "Add Tag",
				InputHint:        "Enter to add",
				InputPlaceholder: "Enter tag...",
				Options: []ListOption{
					{Label: "bug", Value: "bug", Selected: true},
					{Label: "feature", Value: "feature", Selected: false},
				},
			},
		},
		SubmitLabel: "Save",
		MinWidth:    50,
	}
	m := New(cfg).SetSize(80, 24)

	compareGolden(t, "editable_list_focused_on_list", m.View())
}

func TestGolden_EditableListFocusedOnInput(t *testing.T) {
	cfg := FormConfig{
		Title: "Edit Tags",
		Fields: []FieldConfig{
			{
				Key:              "tags",
				Type:             FieldTypeEditableList,
				Label:            "Tags",
				Hint:             "Space to toggle",
				InputLabel:       "Add Tag",
				InputHint:        "Enter to add",
				InputPlaceholder: "Enter tag...",
				Options: []ListOption{
					{Label: "bug", Value: "bug", Selected: true},
					{Label: "feature", Value: "feature", Selected: false},
				},
			},
		},
		SubmitLabel: "Save",
		MinWidth:    50,
	}
	m := New(cfg).SetSize(80, 24)

	// Move to input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	compareGolden(t, "editable_list_focused_on_input", m.View())
}

func TestGolden_EditableListEmpty(t *testing.T) {
	cfg := FormConfig{
		Title: "Edit Tags",
		Fields: []FieldConfig{
			{
				Key:              "tags",
				Type:             FieldTypeEditableList,
				Label:            "Tags",
				Hint:             "Space to toggle",
				InputLabel:       "Add Tag",
				InputHint:        "Enter to add",
				InputPlaceholder: "Enter tag...",
				// No initial options
			},
		},
		SubmitLabel: "Save",
		MinWidth:    50,
	}
	m := New(cfg).SetSize(80, 24)

	compareGolden(t, "editable_list_empty", m.View())
}

// --- OnSubmit/OnCancel Factory Tests ---

// Custom message types for factory tests
type CustomSubmitMsg struct {
	Name string
}

type CustomCancelMsg struct {
	Reason string
}

func TestOnSubmitFactory_ReturnsCustomMessage(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{Key: "name", Type: FieldTypeText, Label: "Name", InitialValue: "test"},
		},
		OnSubmit: func(values map[string]any) tea.Msg {
			return CustomSubmitMsg{Name: values["name"].(string)}
		},
	}
	m := New(cfg)

	// Navigate to submit button and press Enter
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // to submit
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("expected submit command, got nil")
	}
	msg := cmd()
	customMsg, ok := msg.(CustomSubmitMsg)
	if !ok {
		t.Fatalf("expected CustomSubmitMsg, got %T", msg)
	}
	if customMsg.Name != "test" {
		t.Errorf("expected Name='test', got %q", customMsg.Name)
	}
}

func TestOnSubmitFactory_NilReturnsSubmitMsg(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{Key: "name", Type: FieldTypeText, Label: "Name", InitialValue: "test"},
		},
		// OnSubmit is nil (default)
	}
	m := New(cfg)

	// Navigate to submit button and press Enter
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // to submit
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("expected submit command, got nil")
	}
	msg := cmd()
	submitMsg, ok := msg.(SubmitMsg)
	if !ok {
		t.Fatalf("expected SubmitMsg for nil OnSubmit, got %T", msg)
	}
	if submitMsg.Values["name"] != "test" {
		t.Errorf("expected name='test', got %v", submitMsg.Values["name"])
	}
}

func TestOnCancelFactory_ReturnsCustomMessageOnEsc(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{Key: "name", Type: FieldTypeText, Label: "Name"},
		},
		OnCancel: func() tea.Msg {
			return CustomCancelMsg{Reason: "user pressed esc"}
		},
	}
	m := New(cfg)

	// Press Esc
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if cmd == nil {
		t.Fatal("expected cancel command, got nil")
	}
	msg := cmd()
	customMsg, ok := msg.(CustomCancelMsg)
	if !ok {
		t.Fatalf("expected CustomCancelMsg, got %T", msg)
	}
	if customMsg.Reason != "user pressed esc" {
		t.Errorf("expected Reason='user pressed esc', got %q", customMsg.Reason)
	}
}

func TestOnCancelFactory_ReturnsCustomMessageOnCancelButton(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		OnCancel: func() tea.Msg {
			return CustomCancelMsg{Reason: "user clicked cancel"}
		},
	}
	m := New(cfg)

	// Navigate to cancel button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // to cancel

	// Press Enter on cancel button
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("expected cancel command, got nil")
	}
	msg := cmd()
	customMsg, ok := msg.(CustomCancelMsg)
	if !ok {
		t.Fatalf("expected CustomCancelMsg, got %T", msg)
	}
	if customMsg.Reason != "user clicked cancel" {
		t.Errorf("expected Reason='user clicked cancel', got %q", customMsg.Reason)
	}
}

func TestOnCancelFactory_NilReturnsCancelMsg(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{Key: "name", Type: FieldTypeText, Label: "Name"},
		},
		// OnCancel is nil (default)
	}
	m := New(cfg)

	// Press Esc
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if cmd == nil {
		t.Fatal("expected cancel command, got nil")
	}
	msg := cmd()
	if _, ok := msg.(CancelMsg); !ok {
		t.Fatalf("expected CancelMsg for nil OnCancel, got %T", msg)
	}
}

func TestOnSubmitFactory_ValidationFailureStillShowsError(t *testing.T) {
	factoryCalled := false
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{Key: "name", Type: FieldTypeText, Label: "Name"},
		},
		Validate: func(values map[string]any) error {
			name := values["name"].(string)
			if name == "" {
				return errors.New("Name is required")
			}
			return nil
		},
		OnSubmit: func(values map[string]any) tea.Msg {
			factoryCalled = true
			return CustomSubmitMsg{Name: values["name"].(string)}
		},
	}
	m := New(cfg)

	// Navigate to submit and press Enter with empty name
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // to submit
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should have validation error, no command
	if cmd != nil {
		t.Error("expected nil command due to validation error")
	}
	if m.validationError != "Name is required" {
		t.Errorf("expected validation error 'Name is required', got '%s'", m.validationError)
	}
	if factoryCalled {
		t.Error("OnSubmit factory should not be called when validation fails")
	}
}
