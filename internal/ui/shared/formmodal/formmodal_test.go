package formmodal

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zjrosen/perles/internal/ui/shared/colorpicker"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
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
	require.Equal(t, 0, m.focusedIndex)

	// Tab to second field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, 1, m.focusedIndex)

	// Tab to submit button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, -1, m.focusedIndex, "expected buttons focus")
	require.Equal(t, 0, m.focusedButton, "expected submit button")

	// Tab to cancel button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, 1, m.focusedButton, "expected cancel button")

	// Tab wraps to first field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, 0, m.focusedIndex, "expected wrapped to first field")
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
	require.Equal(t, 0, m.focusedIndex)

	// Shift+Tab wraps to cancel button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	require.Equal(t, -1, m.focusedIndex, "expected buttons focus")
	require.Equal(t, 1, m.focusedButton, "expected cancel button")

	// Shift+Tab to submit button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	require.Equal(t, 0, m.focusedButton, "expected submit button")

	// Shift+Tab to second field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	require.Equal(t, 1, m.focusedIndex)

	// Shift+Tab to first field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	require.Equal(t, 0, m.focusedIndex)
}

func TestFocusCycling_NoFields(t *testing.T) {
	cfg := FormConfig{
		Title: "Confirm",
	}
	m := New(cfg)

	// Start on submit button
	require.Equal(t, -1, m.focusedIndex, "expected buttons focus")
	require.Equal(t, 0, m.focusedButton, "expected submit button")

	// Tab to cancel button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, 1, m.focusedButton, "expected cancel button")

	// Tab wraps to submit button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, 0, m.focusedButton, "expected submit button wrap")
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
	require.Equal(t, -1, m.focusedIndex, "ctrl+n: expected buttons focus")

	// Ctrl+P should go back like Shift+Tab
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	require.Equal(t, 0, m.focusedIndex, "ctrl+p: expected field focus")
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
	require.Equal(t, 1, m.focusedIndex)
}

func TestKeyboard_Esc_SendsCancelMsg(t *testing.T) {
	cfg := FormConfig{
		Title:  "Test Form",
		Fields: []FieldConfig{{Key: "field1", Type: FieldTypeText, Label: "Field 1"}},
	}
	m := New(cfg)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	require.NotNil(t, cmd, "expected cancel command")
	msg := cmd()
	_, ok := msg.(CancelMsg)
	require.True(t, ok, "expected CancelMsg, got %T", msg)
}

func TestKeyboard_ButtonNavigation_LeftRight(t *testing.T) {
	cfg := FormConfig{
		Title: "Confirm",
	}
	m := New(cfg)

	// Start on submit button (0)
	require.Equal(t, 0, m.focusedButton)

	// Right/l moves to cancel
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	require.Equal(t, 1, m.focusedButton, "after right")

	// Left/h moves back to submit
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	require.Equal(t, 0, m.focusedButton, "after left")

	// Test with h/l keys
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	require.Equal(t, 1, m.focusedButton, "after 'l'")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	require.Equal(t, 0, m.focusedButton, "after 'h'")
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
	require.NotNil(t, cmd, "expected submit command")
	msg := cmd()
	submitMsg, ok := msg.(SubmitMsg)
	require.True(t, ok, "expected SubmitMsg, got %T", msg)
	require.Equal(t, "test", submitMsg.Values["name"])
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
	require.NotNil(t, cmd, "expected cancel command")
	msg := cmd()
	_, ok := msg.(CancelMsg)
	require.True(t, ok, "expected CancelMsg, got %T", msg)
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
	require.Nil(t, cmd, "expected nil command due to validation error")
	require.Equal(t, "Name is required", m.validationError)
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
	require.NotNil(t, cmd, "expected submit command")
	msg := cmd()
	_, ok := msg.(SubmitMsg)
	require.True(t, ok, "expected SubmitMsg, got %T", msg)
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
	require.Equal(t, 0, m.fields[0].listCursor)

	// j/down moves cursor down
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, 1, m.fields[0].listCursor, "after 'j'")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, 2, m.fields[0].listCursor, "after down")

	// k/up moves cursor up
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	require.Equal(t, 1, m.fields[0].listCursor, "after 'k'")
}

// --- List Field Boundary Escape Tests ---

func TestListField_JAtBottomEscapesToNextField(t *testing.T) {
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
			{Key: "name", Type: FieldTypeText, Label: "Name"},
		},
	}
	m := New(cfg)

	// Start on list field
	require.Equal(t, 0, m.focusedIndex)
	require.Equal(t, 0, m.fields[0].listCursor)

	// Navigate to last item
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, 1, m.fields[0].listCursor, "at last item")

	// j at bottom should escape to next field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, 1, m.focusedIndex, "should move to next field")
}

func TestListField_DownAtBottomEscapesToNextField(t *testing.T) {
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
			{Key: "name", Type: FieldTypeText, Label: "Name"},
		},
	}
	m := New(cfg)

	// Navigate to last item
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, 1, m.fields[0].listCursor, "at last item")

	// Down at bottom should escape to next field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, 1, m.focusedIndex, "should move to next field")
}

func TestListField_KAtTopEscapesToPrevField(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{Key: "name", Type: FieldTypeText, Label: "Name"},
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

	// Move to list field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, 1, m.focusedIndex)
	require.Equal(t, 0, m.fields[1].listCursor, "at first item")

	// k at top should escape to previous field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	require.Equal(t, 0, m.focusedIndex, "should move to previous field")
}

func TestListField_UpAtTopEscapesToPrevField(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{Key: "name", Type: FieldTypeText, Label: "Name"},
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

	// Move to list field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, 1, m.focusedIndex)
	require.Equal(t, 0, m.fields[1].listCursor, "at first item")

	// Up at top should escape to previous field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	require.Equal(t, 0, m.focusedIndex, "should move to previous field")
}

func TestSelectField_JAtBottomEscapesToNextField(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:   "priority",
				Type:  FieldTypeSelect,
				Label: "Priority",
				Options: []ListOption{
					{Label: "P0", Value: "0"},
					{Label: "P1", Value: "1"},
				},
			},
			{Key: "name", Type: FieldTypeText, Label: "Name"},
		},
	}
	m := New(cfg)

	// Navigate to last item
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, 1, m.fields[0].listCursor, "at last item")

	// j at bottom should escape to next field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, 1, m.focusedIndex, "should move to next field")
}

func TestSelectField_KAtTopEscapesToPrevField(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{Key: "name", Type: FieldTypeText, Label: "Name"},
			{
				Key:   "priority",
				Type:  FieldTypeSelect,
				Label: "Priority",
				Options: []ListOption{
					{Label: "P0", Value: "0"},
					{Label: "P1", Value: "1"},
				},
			},
		},
	}
	m := New(cfg)

	// Move to select field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, 1, m.focusedIndex)

	// k at top should escape to previous field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	require.Equal(t, 0, m.focusedIndex, "should move to previous field")
}

func TestListField_EmptyList_JEscapesToNextField(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:     "items",
				Type:    FieldTypeList,
				Label:   "Items",
				Options: []ListOption{}, // Empty list
			},
			{Key: "name", Type: FieldTypeText, Label: "Name"},
		},
	}
	m := New(cfg)

	// j on empty list should immediately escape to next field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, 1, m.focusedIndex, "should move to next field on empty list")
}

func TestListField_EmptyList_KWrapsToCancel(t *testing.T) {
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
	}
	m := New(cfg)

	// k on empty list should wrap to cancel button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	require.Equal(t, -1, m.focusedIndex, "should wrap to buttons")
	require.Equal(t, 1, m.focusedButton, "should be on cancel button")
}

func TestTextField_JK_TypesCharacters(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{Key: "name", Type: FieldTypeText, Label: "Name"},
		},
	}
	m := New(cfg)

	// Start on text field (focused)
	require.Equal(t, 0, m.focusedIndex)

	// Type 'j' and 'k' - should add characters to input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})

	// j and k should be typed, not navigate
	require.Equal(t, "jk", m.fields[0].textInput.Value(), "j/k should type in text field")
	require.Equal(t, 0, m.focusedIndex, "should stay on text field")
}

func TestEditableListField_JK_InInputTypesCharacters(t *testing.T) {
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
		},
	}
	m := New(cfg)

	// Move to input section
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, SubFocusInput, m.fields[0].subFocus)

	// Type 'j' and 'k' - should add characters to input (not navigate)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})

	require.Equal(t, "jk", m.fields[0].addInput.Value(), "j/k should type in editable list input")
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
	require.True(t, m.fields[0].listItems[0].selected, "expected item 0 selected after space")

	// Move to item 2 and select it too
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	require.True(t, m.fields[0].listItems[1].selected, "expected item 1 selected after space")

	// Both items should be selected (multi-select)
	require.True(t, m.fields[0].listItems[0].selected, "expected item 0 to remain selected in multi-select")
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
	require.True(t, m.fields[0].listItems[0].selected, "expected item 0 selected")

	// Move to second item and select
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})

	// Second item should be selected
	require.True(t, m.fields[0].listItems[1].selected, "expected item 1 selected")

	// First item should be deselected (single-select behavior)
	require.False(t, m.fields[0].listItems[0].selected, "expected item 0 deselected in single-select mode")
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
	require.Equal(t, 0, m.focusedIndex)

	// Tab should move to buttons
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, -1, m.focusedIndex, "expected focus on buttons")
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
	require.Equal(t, 1, m.focusedIndex)

	// Shift+Tab should go back to list
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	require.Equal(t, 0, m.focusedIndex, "expected focus on list")
}

// --- Cursor Positioning Tests ---

func TestListField_CursorPositionFromAbove(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{Key: "name", Type: FieldTypeText, Label: "Name"},
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

	// Start on text field
	require.Equal(t, 0, m.focusedIndex)

	// Tab to list field (entering from above)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, 1, m.focusedIndex, "should be on list field")

	// Cursor should be at first item (index 0)
	require.Equal(t, 0, m.fields[1].listCursor, "cursor should be at first item when entering from above")
}

func TestListField_CursorPositionFromBelow(t *testing.T) {
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
			{Key: "name", Type: FieldTypeText, Label: "Name"},
		},
	}
	m := New(cfg)

	// Move to second field (text field)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, 1, m.focusedIndex)

	// Shift+Tab to go back to list (entering from below)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	require.Equal(t, 0, m.focusedIndex, "should be on list field")

	// Cursor should be at last item (index 2)
	require.Equal(t, 2, m.fields[0].listCursor, "cursor should be at last item when entering from below")
}

func TestSelectField_CursorPositionFromAbove(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{Key: "name", Type: FieldTypeText, Label: "Name"},
			{
				Key:   "priority",
				Type:  FieldTypeSelect,
				Label: "Priority",
				Options: []ListOption{
					{Label: "P0", Value: "0"},
					{Label: "P1", Value: "1"},
					{Label: "P2", Value: "2"},
				},
			},
		},
	}
	m := New(cfg)

	// Start on text field
	require.Equal(t, 0, m.focusedIndex)

	// Tab to select field (entering from above)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, 1, m.focusedIndex, "should be on select field")

	// Cursor should be at first item (index 0)
	require.Equal(t, 0, m.fields[1].listCursor, "cursor should be at first item when entering from above")
}

func TestSelectField_CursorPositionFromBelow(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:   "priority",
				Type:  FieldTypeSelect,
				Label: "Priority",
				Options: []ListOption{
					{Label: "P0", Value: "0"},
					{Label: "P1", Value: "1"},
					{Label: "P2", Value: "2"},
					{Label: "P3", Value: "3"},
					{Label: "P4", Value: "4"},
				},
			},
			{Key: "name", Type: FieldTypeText, Label: "Name"},
		},
	}
	m := New(cfg)

	// Move to second field (text field)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, 1, m.focusedIndex)

	// Shift+Tab to go back to select (entering from below)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	require.Equal(t, 0, m.focusedIndex, "should be on select field")

	// Cursor should be at last item (index 4)
	require.Equal(t, 4, m.fields[0].listCursor, "cursor should be at last item when entering from below")
}

// --- FieldTypeSelect Value Tests ---
// These tests verify that FieldTypeSelect returns the selected item's value,
// not the cursor position, fixing the bug where tabbing through fields
// would return incorrect values.

func TestSelectField_ValueReturnsSelectedNotCursor(t *testing.T) {
	// This test verifies the core fix: value() returns the item with selected=true,
	// not the item at listCursor position.
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:   "priority",
				Type:  FieldTypeSelect,
				Label: "Priority",
				Options: []ListOption{
					{Label: "P0", Value: "0"},
					{Label: "P1", Value: "1"},
					{Label: "P2", Value: "2", Selected: true}, // P2 is selected
					{Label: "P3", Value: "3"},
					{Label: "P4", Value: "4"},
				},
			},
		},
	}
	m := New(cfg)

	// Initial value should be P2 (the selected one)
	values := getValues(m)
	require.Equal(t, "2", values["priority"], "initial value should be P2")

	// Move cursor without changing selection (j navigates, doesn't select)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}) // cursor to P3
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}) // cursor to P4

	// Value should STILL be P2 (cursor moved but selection didn't change)
	values = getValues(m)
	require.Equal(t, "2", values["priority"], "value should still be P2 after cursor movement")

	// Verify cursor is at P4 but selection is at P2
	require.Equal(t, 4, m.fields[0].listCursor, "cursor should be at P4")
	require.True(t, m.fields[0].listItems[2].selected, "P2 should still be selected")
	require.False(t, m.fields[0].listItems[4].selected, "P4 should not be selected")
}

func TestSelectField_SpaceChangesSelection(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:   "status",
				Type:  FieldTypeSelect,
				Label: "Status",
				Options: []ListOption{
					{Label: "Open", Value: "open", Selected: true},
					{Label: "In Progress", Value: "in_progress"},
					{Label: "Closed", Value: "closed"},
				},
			},
		},
	}
	m := New(cfg)

	// Initial value is "open"
	values := getValues(m)
	require.Equal(t, "open", values["status"])

	// Navigate to "In Progress" and press Space to select
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})

	// Value should now be "in_progress"
	values = getValues(m)
	require.Equal(t, "in_progress", values["status"], "value should change after Space")

	// Verify selection state
	require.False(t, m.fields[0].listItems[0].selected, "open should be deselected")
	require.True(t, m.fields[0].listItems[1].selected, "in_progress should be selected")
}

func TestSelectField_ValuePersistsAfterTabbing(t *testing.T) {
	// This is the exact bug scenario: tabbing to another field and back
	// should not change the selected value.
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:   "priority",
				Type:  FieldTypeSelect,
				Label: "Priority",
				Options: []ListOption{
					{Label: "P0", Value: "0"},
					{Label: "P1", Value: "1"},
					{Label: "P2", Value: "2", Selected: true},
					{Label: "P3", Value: "3"},
					{Label: "P4", Value: "4"},
				},
			},
			{
				Key:   "status",
				Type:  FieldTypeSelect,
				Label: "Status",
				Options: []ListOption{
					{Label: "Open", Value: "open"},
					{Label: "Closed", Value: "closed", Selected: true},
				},
			},
		},
	}
	m := New(cfg)

	// Initial values
	values := getValues(m)
	require.Equal(t, "2", values["priority"], "initial priority should be P2")
	require.Equal(t, "closed", values["status"], "initial status should be closed")

	// Tab to status field (this should NOT change priority)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, 1, m.focusedIndex, "should be on status field")

	// Navigate within status (cursor moves but we don't press Space)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}) // cursor to Open

	// Tab to buttons
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Values should be unchanged (cursor moved but no Space pressed)
	values = getValues(m)
	require.Equal(t, "2", values["priority"], "priority should still be P2")
	require.Equal(t, "closed", values["status"], "status should still be closed")
}

func TestSelectField_MultipleFieldsIndependent(t *testing.T) {
	// Verify that multiple FieldTypeSelect fields don't interfere with each other
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:   "priority",
				Type:  FieldTypeSelect,
				Label: "Priority",
				Options: []ListOption{
					{Label: "P0", Value: "0", Selected: true},
					{Label: "P1", Value: "1"},
					{Label: "P2", Value: "2"},
				},
			},
			{
				Key:   "status",
				Type:  FieldTypeSelect,
				Label: "Status",
				Options: []ListOption{
					{Label: "Open", Value: "open"},
					{Label: "Closed", Value: "closed", Selected: true},
				},
			},
			{
				Key:   "type",
				Type:  FieldTypeSelect,
				Label: "Type",
				Options: []ListOption{
					{Label: "Bug", Value: "bug"},
					{Label: "Feature", Value: "feature", Selected: true},
					{Label: "Task", Value: "task"},
				},
			},
		},
	}
	m := New(cfg)

	// Change priority to P2
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}) // P1
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}) // P2
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})                     // select P2

	// Tab to status, change to Open (cursor starts at 0 = Open)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace}) // select Open (cursor is already there)

	// Tab to type, leave as Feature (don't change)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Verify all values
	values := getValues(m)
	require.Equal(t, "2", values["priority"], "priority should be P2")
	require.Equal(t, "open", values["status"], "status should be open")
	require.Equal(t, "feature", values["type"], "type should still be feature")
}

func TestSelectField_CursorPositionOnFocus(t *testing.T) {
	// When tabbing into a FieldTypeSelect, cursor starts at first item
	// but value() still returns the selected item
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{Key: "name", Type: FieldTypeText, Label: "Name"},
			{
				Key:   "priority",
				Type:  FieldTypeSelect,
				Label: "Priority",
				Options: []ListOption{
					{Label: "P0", Value: "0"},
					{Label: "P1", Value: "1"},
					{Label: "P2", Value: "2", Selected: true}, // P2 selected
					{Label: "P3", Value: "3"},
					{Label: "P4", Value: "4"},
				},
			},
		},
	}
	m := New(cfg)

	// Tab to select field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, 1, m.focusedIndex)

	// Cursor is at first item (standard behavior)
	require.Equal(t, 0, m.fields[1].listCursor, "cursor starts at first item")

	// But value() returns the selected item P2
	values := getValues(m)
	require.Equal(t, "2", values["priority"], "value should be P2 (selected), not P0 (cursor)")
}

func TestSelectField_SubmitReturnsCorrectValue(t *testing.T) {
	var submittedValues map[string]any

	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:   "priority",
				Type:  FieldTypeSelect,
				Label: "Priority",
				Options: []ListOption{
					{Label: "P0", Value: "0"},
					{Label: "P1", Value: "1"},
					{Label: "P2", Value: "2", Selected: true},
				},
			},
			{
				Key:   "status",
				Type:  FieldTypeSelect,
				Label: "Status",
				Options: []ListOption{
					{Label: "Open", Value: "open", Selected: true},
					{Label: "Closed", Value: "closed"},
				},
			},
		},
		OnSubmit: func(values map[string]any) tea.Msg {
			submittedValues = values
			return SubmitMsg{Values: values}
		},
	}
	m := New(cfg)

	// Form starts on first field (priority), tab through to buttons
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // to status
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // to submit button

	// Press Enter to submit
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)
	cmd() // Execute the command

	// Verify submitted values match the selected items
	require.Equal(t, "2", submittedValues["priority"], "submitted priority should be P2")
	require.Equal(t, "open", submittedValues["status"], "submitted status should be open")
}

func TestListField_NavigationFlow_FromAboveToBelow(t *testing.T) {
	// Test the full navigation flow: text1 -> list (at top) -> through list -> text2 -> back to list (at bottom)
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{Key: "field1", Type: FieldTypeText, Label: "Field 1"},
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
			{Key: "field2", Type: FieldTypeText, Label: "Field 2"},
		},
	}
	m := New(cfg)

	// Start on field1
	require.Equal(t, 0, m.focusedIndex)

	// Tab to list (enters from above -> cursor at top)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, 1, m.focusedIndex, "on list")
	require.Equal(t, 0, m.fields[1].listCursor, "cursor at top")

	// Navigate down through list
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, 1, m.fields[1].listCursor)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, 2, m.fields[1].listCursor, "cursor at bottom")

	// Down at bottom escapes to field2
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, 2, m.focusedIndex, "escaped to field2")

	// Up from field2 should go back to list with cursor at bottom
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	require.Equal(t, 1, m.focusedIndex, "back on list")
	require.Equal(t, 2, m.fields[1].listCursor, "cursor at bottom when entering from below")
}

func TestListField_CursorPositionFromButtons(t *testing.T) {
	// When navigating up from buttons to list, should position cursor at bottom
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

	// Tab to buttons
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, -1, m.focusedIndex, "on buttons")

	// k or Shift+Tab from buttons should go to list with cursor at bottom
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	require.Equal(t, 0, m.focusedIndex, "back on list")
	require.Equal(t, 1, m.fields[0].listCursor, "cursor at bottom when entering from buttons")
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

	require.NotNil(t, cmd, "expected submit command")
	msg := cmd()
	submitMsg, ok := msg.(SubmitMsg)
	require.True(t, ok, "expected SubmitMsg, got %T", msg)

	selected, ok := submitMsg.Values["items"].([]string)
	require.True(t, ok, "expected []string for items, got %T", submitMsg.Values["items"])

	// Should contain val1 (selected via space) and val2 (pre-selected)
	require.Len(t, selected, 2)
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
	require.True(t, hasVal1 && hasVal2, "expected val1 and val2 in selected, got %v", selected)
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
	require.Contains(t, view, "(no items)", "expected empty list to show '(no items)'")
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
	require.False(t, m.showColorPicker, "expected colorpicker to be hidden initially")

	// Enter on color field opens colorpicker
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, m.showColorPicker, "expected colorpicker to be shown after Enter")
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
	require.False(t, m.showColorPicker, "expected colorpicker to be closed after SelectMsg")

	// Check color was updated
	values := getValues(m)
	require.Equal(t, "#FF8787", values["color"])
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
	require.False(t, m.showColorPicker, "expected colorpicker to be closed after CancelMsg")

	// Check color was NOT changed
	values := getValues(m)
	require.Equal(t, "#73F59F", values["color"])
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

	require.False(t, m.showColorPicker, "Tab should not open colorpicker")
	require.Equal(t, -1, m.focusedIndex, "expected focus on buttons")
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
	require.Equal(t, "#73F59F", values["color"], "expected default color")
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

	require.NotNil(t, cmd, "expected submit command")
	msg := cmd()
	submitMsg, ok := msg.(SubmitMsg)
	require.True(t, ok, "expected SubmitMsg, got %T", msg)
	require.Equal(t, "#FF8787", submitMsg.Values["color"])
}

func TestColorField_JNavigatesToNextFieldWhenPickerClosed(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{Key: "color", Type: FieldTypeColor, Label: "Color", InitialColor: "#FF8787"},
			{Key: "desc", Type: FieldTypeText, Label: "Description"},
		},
	}
	m := New(cfg).SetSize(80, 24)

	// Start on color field
	require.Equal(t, 0, m.focusedIndex, "should start on color field")
	require.False(t, m.showColorPicker, "picker should be closed")

	// j should move to next field when picker is closed
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, 1, m.focusedIndex, "j should move to next field when picker closed")
}

func TestColorField_KNavigatesToPrevFieldWhenPickerClosed(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{Key: "name", Type: FieldTypeText, Label: "Name"},
			{Key: "color", Type: FieldTypeColor, Label: "Color", InitialColor: "#FF8787"},
		},
	}
	m := New(cfg).SetSize(80, 24)

	// Move to color field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, 1, m.focusedIndex, "should be on color field")
	require.False(t, m.showColorPicker, "picker should be closed")

	// k should move to previous field when picker is closed
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	require.Equal(t, 0, m.focusedIndex, "k should move to previous field when picker closed")
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
		require.NoError(t, err, "failed to write golden file")
		return
	}

	want, err := os.ReadFile(goldenPath)
	require.NoError(t, err, "failed to read golden file %s (run with UPDATE_GOLDEN=1 to create)", goldenPath)

	require.Equal(t, string(want), got, "output does not match golden file %s", goldenPath)
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
	require.Equal(t, SubFocusList, m.fields[0].subFocus)

	// Cursor should be at 0
	require.Equal(t, 0, m.fields[0].listCursor)

	// Should have 2 list items
	require.Len(t, m.fields[0].listItems, 2)
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
	require.Equal(t, SubFocusList, m.fields[0].subFocus)

	// Tab moves to input within same field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, SubFocusInput, m.fields[0].subFocus, "expected SubFocusInput after Tab")
	require.Equal(t, 0, m.focusedIndex, "expected same field")

	// Tab again moves to buttons
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, -1, m.focusedIndex, "expected buttons focus")
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
	require.Equal(t, SubFocusInput, m.fields[0].subFocus)

	// Shift+Tab moves back to list
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	require.Equal(t, SubFocusList, m.fields[0].subFocus, "expected SubFocusList after Shift+Tab")
	// Cursor should be at bottom of list
	require.Equal(t, 1, m.fields[0].listCursor, "expected listCursor at bottom")

	// Shift+Tab from list moves to cancel button (wraps)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	require.Equal(t, -1, m.focusedIndex, "expected buttons focus")
	require.Equal(t, 1, m.focusedButton, "expected cancel button")
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
	require.Equal(t, 0, m.fields[0].listCursor)
	require.Equal(t, SubFocusList, m.fields[0].subFocus)

	// j moves cursor down
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, 1, m.fields[0].listCursor, "after 'j'")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, 2, m.fields[0].listCursor, "after down")

	// At bottom of list, j/down advances to input section
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, SubFocusInput, m.fields[0].subFocus, "at bottom -> input")

	// up from input returns to list at bottom (k types character in input)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	require.Equal(t, SubFocusList, m.fields[0].subFocus, "up returns to list")
	require.Equal(t, 2, m.fields[0].listCursor, "at bottom of list")

	// k moves cursor up
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	require.Equal(t, 1, m.fields[0].listCursor, "after 'k'")
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

	// Cursor at 0, k/up should wrap to cancel button (previous in cycle)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	require.Equal(t, -1, m.focusedIndex, "expected buttons focus")
	require.Equal(t, 1, m.focusedButton, "expected cancel button")
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
	require.Equal(t, SubFocusInput, m.fields[0].subFocus)

	// Down from input moves to next field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, 1, m.focusedIndex, "expected next field after down from input")
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
	require.Equal(t, SubFocusList, m.fields[0].subFocus, "expected SubFocusList after up from input")
	require.Equal(t, 1, m.fields[0].listCursor, "expected listCursor at bottom")
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
	require.False(t, m.fields[0].listItems[0].selected, "expected item 0 unselected initially")
	require.True(t, m.fields[0].listItems[1].selected, "expected item 1 selected initially")

	// Space toggles selection of item at cursor
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	require.True(t, m.fields[0].listItems[0].selected, "expected item 0 selected after space")

	// Toggle again
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	require.False(t, m.fields[0].listItems[0].selected, "expected item 0 unselected after second space")
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
	require.True(t, m.fields[0].listItems[0].selected, "expected item 0 selected after enter")
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
	require.Empty(t, m.fields[0].listItems)

	// Move to input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Type "newitem"
	for _, r := range "newitem" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Press Enter to add
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Verify item was added
	require.Len(t, m.fields[0].listItems, 1)
	require.Equal(t, "newitem", m.fields[0].listItems[0].value)
	require.Equal(t, "newitem", m.fields[0].listItems[0].label)
	require.True(t, m.fields[0].listItems[0].selected, "expected new item to be selected")

	// Input should be cleared
	require.Empty(t, m.fields[0].addInput.Value())
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
	require.Len(t, m.fields[0].listItems, 1)
	require.Equal(t, "test", m.fields[0].listItems[0].value, "expected trimmed value")
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
	require.Empty(t, m.fields[0].listItems)

	// Try with only whitespace
	for _, r := range "   " {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	require.Empty(t, m.fields[0].listItems, "expected 0 items for whitespace-only input")
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
	require.Len(t, m.fields[0].listItems, 1, "expected duplicate rejected")
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
	require.Len(t, m.fields[0].listItems, 2, "expected duplicates allowed")
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
	require.True(t, ok, "expected []string, got %T", values["tags"])

	// Should contain val1 and val3 (the selected items)
	require.Len(t, selected, 2)

	hasVal1, hasVal3 := false, false
	for _, v := range selected {
		if v == "val1" {
			hasVal1 = true
		}
		if v == "val3" {
			hasVal3 = true
		}
	}
	require.True(t, hasVal1 && hasVal3, "expected val1 and val3 in selected, got %v", selected)
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
	require.NotNil(t, cmd, "expected submit command")
	msg := cmd()
	submitMsg, ok := msg.(SubmitMsg)
	require.True(t, ok, "expected SubmitMsg, got %T", msg)

	selected, ok := submitMsg.Values["tags"].([]string)
	require.True(t, ok, "expected []string, got %T", submitMsg.Values["tags"])

	require.Len(t, selected, 2)
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

	// j on empty list advances to input (nothing to navigate in list)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, SubFocusInput, m.fields[0].subFocus, "after j: advances to input")

	// up from input wraps back to list (k types character in input)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	require.Equal(t, SubFocusList, m.fields[0].subFocus, "after up: back to list")

	// k on empty list at cursor 0 wraps to cancel button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	require.Equal(t, -1, m.focusedIndex, "after k: on buttons")
	require.Equal(t, 1, m.focusedButton, "after k: on cancel")

	// j from cancel goes to first field (list)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, 0, m.focusedIndex, "after j: back to field")
	require.Equal(t, SubFocusList, m.fields[0].subFocus, "after j: on list")

	// Space on empty list should not crash (does nothing)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})

	// Enter on empty list should not crash (does nothing in list mode)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Tab should navigate to input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, SubFocusInput, m.fields[0].subFocus, "after Tab")
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
	require.Equal(t, "hello world", m.fields[0].addInput.Value())
}

// --- SearchSelect Field Tests ---

func TestSearchSelectField_StartsCollapsed(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:   "branch",
				Type:  FieldTypeSearchSelect,
				Label: "Branch",
				Options: []ListOption{
					{Label: "main", Value: "main", Selected: true},
					{Label: "develop", Value: "develop"},
					{Label: "feature-1", Value: "feature-1"},
				},
			},
		},
	}
	m := New(cfg)

	// Should start collapsed
	require.False(t, m.fields[0].searchExpanded, "expected search to start collapsed")
}

func TestSearchSelectField_EnterExpands(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:   "branch",
				Type:  FieldTypeSearchSelect,
				Label: "Branch",
				Options: []ListOption{
					{Label: "main", Value: "main", Selected: true},
					{Label: "develop", Value: "develop"},
				},
			},
		},
	}
	m := New(cfg)

	// Press Enter to expand
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	require.True(t, m.fields[0].searchExpanded, "expected search to be expanded after Enter")
}

func TestSearchSelectField_EscapeCollapsesSearch(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:   "branch",
				Type:  FieldTypeSearchSelect,
				Label: "Branch",
				Options: []ListOption{
					{Label: "main", Value: "main", Selected: true},
					{Label: "develop", Value: "develop"},
				},
			},
		},
	}
	m := New(cfg)

	// Expand first
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, m.fields[0].searchExpanded, "precondition: should be expanded")

	// Escape should collapse, not cancel modal
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	require.False(t, m.fields[0].searchExpanded, "expected search to be collapsed after Escape")
	require.Nil(t, cmd, "expected no cancel command when collapsing search")
}

func TestSearchSelectField_EscapeWhenCollapsedCancelsModal(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:   "branch",
				Type:  FieldTypeSearchSelect,
				Label: "Branch",
				Options: []ListOption{
					{Label: "main", Value: "main", Selected: true},
				},
			},
		},
	}
	m := New(cfg)

	// Should start collapsed
	require.False(t, m.fields[0].searchExpanded, "precondition: should be collapsed")

	// Escape should cancel modal
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	require.NotNil(t, cmd, "expected cancel command")
	msg := cmd()
	_, ok := msg.(CancelMsg)
	require.True(t, ok, "expected CancelMsg, got %T", msg)
}

func TestSearchSelectField_EnterSelectsAndCollapses(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:   "branch",
				Type:  FieldTypeSearchSelect,
				Label: "Branch",
				Options: []ListOption{
					{Label: "main", Value: "main", Selected: true},
					{Label: "develop", Value: "develop"},
					{Label: "feature-1", Value: "feature-1"},
				},
			},
		},
	}
	m := New(cfg)

	// Expand
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, m.fields[0].searchExpanded)

	// Move to second item
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})

	// Enter to select and collapse
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	require.False(t, m.fields[0].searchExpanded, "expected collapsed after selection")
	require.True(t, m.fields[0].listItems[1].selected, "expected second item selected")
	require.False(t, m.fields[0].listItems[0].selected, "expected first item deselected")
}

func TestSearchSelectField_ArrowNavigation(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:   "branch",
				Type:  FieldTypeSearchSelect,
				Label: "Branch",
				Options: []ListOption{
					{Label: "main", Value: "main", Selected: true},
					{Label: "develop", Value: "develop"},
					{Label: "feature-1", Value: "feature-1"},
				},
			},
		},
	}
	m := New(cfg)

	// Expand
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Cursor starts at selected item (main = index 0)
	require.Equal(t, 0, m.fields[0].listCursor)

	// Arrow down
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, 1, m.fields[0].listCursor)

	// Arrow down again
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, 2, m.fields[0].listCursor)

	// Arrow up
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	require.Equal(t, 1, m.fields[0].listCursor)
}

func TestSearchSelectField_CollapsedArrowDownNavigatesToNextField(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:   "branch",
				Type:  FieldTypeSearchSelect,
				Label: "Branch",
				Options: []ListOption{
					{Label: "main", Value: "main", Selected: true},
					{Label: "develop", Value: "develop"},
				},
			},
			{
				Key:   "desc",
				Type:  FieldTypeText,
				Label: "Description",
			},
		},
	}
	m := New(cfg)

	// Start on SearchSelect field (collapsed)
	require.Equal(t, 0, m.focusedIndex, "should start on SearchSelect field")
	require.False(t, m.fields[0].searchExpanded, "should be collapsed")

	// Arrow down should move to next field (not expand)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, 1, m.focusedIndex, "arrow down should move to next field when collapsed")
}

func TestSearchSelectField_CollapsedArrowUpNavigatesToPrevField(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:   "name",
				Type:  FieldTypeText,
				Label: "Name",
			},
			{
				Key:   "branch",
				Type:  FieldTypeSearchSelect,
				Label: "Branch",
				Options: []ListOption{
					{Label: "main", Value: "main", Selected: true},
					{Label: "develop", Value: "develop"},
				},
			},
		},
	}
	m := New(cfg)

	// Move to SearchSelect field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, 1, m.focusedIndex, "should be on SearchSelect field")
	require.False(t, m.fields[1].searchExpanded, "should be collapsed")

	// Arrow up should move to previous field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	require.Equal(t, 0, m.focusedIndex, "arrow up should move to previous field when collapsed")
}

func TestSearchSelectField_CollapsedJNavigatesToNextField(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:   "branch",
				Type:  FieldTypeSearchSelect,
				Label: "Branch",
				Options: []ListOption{
					{Label: "main", Value: "main", Selected: true},
					{Label: "develop", Value: "develop"},
				},
			},
			{
				Key:   "desc",
				Type:  FieldTypeText,
				Label: "Description",
			},
		},
	}
	m := New(cfg)

	// Start on SearchSelect field (collapsed)
	require.Equal(t, 0, m.focusedIndex, "should start on SearchSelect field")
	require.False(t, m.fields[0].searchExpanded, "should be collapsed")

	// j should move to next field when collapsed
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, 1, m.focusedIndex, "j should move to next field when collapsed")
}

func TestSearchSelectField_CollapsedKNavigatesToPrevField(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:   "name",
				Type:  FieldTypeText,
				Label: "Name",
			},
			{
				Key:   "branch",
				Type:  FieldTypeSearchSelect,
				Label: "Branch",
				Options: []ListOption{
					{Label: "main", Value: "main", Selected: true},
					{Label: "develop", Value: "develop"},
				},
			},
		},
	}
	m := New(cfg)

	// Move to SearchSelect field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, 1, m.focusedIndex, "should be on SearchSelect field")
	require.False(t, m.fields[1].searchExpanded, "should be collapsed")

	// k should move to previous field when collapsed
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	require.Equal(t, 0, m.focusedIndex, "k should move to previous field when collapsed")
}

// --- SearchSelect Golden Tests ---

func TestGolden_SearchSelectCollapsed(t *testing.T) {
	cfg := FormConfig{
		Title: "Select Branch",
		Fields: []FieldConfig{
			{
				Key:   "branch",
				Type:  FieldTypeSearchSelect,
				Label: "Branch",
				Options: []ListOption{
					{Label: "main", Value: "main", Selected: true},
					{Label: "develop", Value: "develop"},
					{Label: "feature-auth", Value: "feature-auth"},
				},
			},
		},
		SubmitLabel: "Create",
		MinWidth:    50,
	}
	m := New(cfg).SetSize(80, 24)

	compareGolden(t, "search_select_collapsed", m.View())
}

func TestGolden_SearchSelectExpanded(t *testing.T) {
	cfg := FormConfig{
		Title: "Select Branch",
		Fields: []FieldConfig{
			{
				Key:   "branch",
				Type:  FieldTypeSearchSelect,
				Label: "Branch",
				Options: []ListOption{
					{Label: "main", Value: "main", Selected: true},
					{Label: "develop", Value: "develop"},
					{Label: "feature-auth", Value: "feature-auth"},
					{Label: "feature-login", Value: "feature-login"},
					{Label: "hotfix-123", Value: "hotfix-123"},
				},
			},
		},
		SubmitLabel: "Create",
		MinWidth:    50,
	}
	m := New(cfg).SetSize(80, 24)

	// Expand the search
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	compareGolden(t, "search_select_expanded", m.View())
}

func TestGolden_SearchSelectExpandedWithFilter(t *testing.T) {
	cfg := FormConfig{
		Title: "Select Branch",
		Fields: []FieldConfig{
			{
				Key:   "branch",
				Type:  FieldTypeSearchSelect,
				Label: "Branch",
				Options: []ListOption{
					{Label: "main", Value: "main", Selected: true},
					{Label: "develop", Value: "develop"},
					{Label: "feature-auth", Value: "feature-auth"},
					{Label: "feature-login", Value: "feature-login"},
					{Label: "hotfix-123", Value: "hotfix-123"},
				},
			},
		},
		SubmitLabel: "Create",
		MinWidth:    50,
	}
	m := New(cfg).SetSize(80, 24)

	// Expand and type filter
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f', 'e', 'a', 't'}})

	compareGolden(t, "search_select_expanded_filtered", m.View())
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

	require.NotNil(t, cmd, "expected submit command")
	msg := cmd()
	customMsg, ok := msg.(CustomSubmitMsg)
	require.True(t, ok, "expected CustomSubmitMsg, got %T", msg)
	require.Equal(t, "test", customMsg.Name)
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

	require.NotNil(t, cmd, "expected submit command")
	msg := cmd()
	submitMsg, ok := msg.(SubmitMsg)
	require.True(t, ok, "expected SubmitMsg for nil OnSubmit, got %T", msg)
	require.Equal(t, "test", submitMsg.Values["name"])
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

	require.NotNil(t, cmd, "expected cancel command")
	msg := cmd()
	customMsg, ok := msg.(CustomCancelMsg)
	require.True(t, ok, "expected CustomCancelMsg, got %T", msg)
	require.Equal(t, "user pressed esc", customMsg.Reason)
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

	require.NotNil(t, cmd, "expected cancel command")
	msg := cmd()
	customMsg, ok := msg.(CustomCancelMsg)
	require.True(t, ok, "expected CustomCancelMsg, got %T", msg)
	require.Equal(t, "user clicked cancel", customMsg.Reason)
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

	require.NotNil(t, cmd, "expected cancel command")
	msg := cmd()
	_, ok := msg.(CancelMsg)
	require.True(t, ok, "expected CancelMsg for nil OnCancel, got %T", msg)
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
	require.Nil(t, cmd, "expected nil command due to validation error")
	require.Equal(t, "Name is required", m.validationError)
	require.False(t, factoryCalled, "OnSubmit factory should not be called when validation fails")
}

// --- Toggle Field Tests ---

func TestToggleField_InitialState(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:   "mode",
				Type:  FieldTypeToggle,
				Label: "Mode",
				Options: []ListOption{
					{Label: "Option A", Value: "a"},
					{Label: "Option B", Value: "b"},
				},
			},
		},
	}
	m := New(cfg)

	// Default initial index is 0
	require.Equal(t, 0, m.fields[0].toggleIndex)

	// Value should be "a"
	values := getValues(m)
	require.Equal(t, "a", values["mode"])
}

func TestToggleField_InitialToggleIndex(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:                "mode",
				Type:               FieldTypeToggle,
				Label:              "Mode",
				InitialToggleIndex: 1, // Start on second option
				Options: []ListOption{
					{Label: "Option A", Value: "a"},
					{Label: "Option B", Value: "b"},
				},
			},
		},
	}
	m := New(cfg)

	// Initial index should be 1
	require.Equal(t, 1, m.fields[0].toggleIndex)

	// Value should be "b"
	values := getValues(m)
	require.Equal(t, "b", values["mode"])
}

func TestToggleField_Navigation_LeftRight(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:   "mode",
				Type:  FieldTypeToggle,
				Label: "Mode",
				Options: []ListOption{
					{Label: "Option A", Value: "a"},
					{Label: "Option B", Value: "b"},
				},
			},
		},
	}
	m := New(cfg)

	// Start at index 0
	require.Equal(t, 0, m.fields[0].toggleIndex)

	// Right key switches to index 1
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	require.Equal(t, 1, m.fields[0].toggleIndex, "after right")

	// Right again stays at 1 (boundary)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	require.Equal(t, 1, m.fields[0].toggleIndex, "at boundary")

	// Left key switches back to index 0
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	require.Equal(t, 0, m.fields[0].toggleIndex, "after left")

	// Left again stays at 0 (boundary)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	require.Equal(t, 0, m.fields[0].toggleIndex, "at boundary")
}

func TestToggleField_Navigation_HL(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:   "mode",
				Type:  FieldTypeToggle,
				Label: "Mode",
				Options: []ListOption{
					{Label: "Option A", Value: "a"},
					{Label: "Option B", Value: "b"},
				},
			},
		},
	}
	m := New(cfg)

	// Start at index 0
	require.Equal(t, 0, m.fields[0].toggleIndex)

	// 'l' switches to index 1
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	require.Equal(t, 1, m.fields[0].toggleIndex, "after 'l'")

	// 'h' switches back to index 0
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	require.Equal(t, 0, m.fields[0].toggleIndex, "after 'h'")
}

func TestToggleField_Navigation_JK_MovesToNextField(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:   "name",
				Type:  FieldTypeText,
				Label: "Name",
			},
			{
				Key:   "mode",
				Type:  FieldTypeToggle,
				Label: "Mode",
				Options: []ListOption{
					{Label: "Option A", Value: "a"},
					{Label: "Option B", Value: "b"},
				},
			},
			{
				Key:   "color",
				Type:  FieldTypeText,
				Label: "Color",
			},
		},
	}
	m := New(cfg)

	// Start on first field (name)
	require.Equal(t, 0, m.focusedIndex)

	// Tab to toggle field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, 1, m.focusedIndex, "on toggle field")

	// 'j' on toggle should move to next field (color), not toggle within
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, 2, m.focusedIndex, "after 'j' on toggle - should be on color field")

	// Reset to toggle field to test 'k'
	m.focusedIndex = 1 // toggle field

	// 'k' on toggle should move to previous field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	require.Equal(t, 0, m.focusedIndex, "after 'k' on toggle - should be on name field")
}

func TestToggleField_Navigation_DownUp_MovesToNextField(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:   "name",
				Type:  FieldTypeText,
				Label: "Name",
			},
			{
				Key:   "mode",
				Type:  FieldTypeToggle,
				Label: "Mode",
				Options: []ListOption{
					{Label: "Option A", Value: "a"},
					{Label: "Option B", Value: "b"},
				},
			},
			{
				Key:   "color",
				Type:  FieldTypeText,
				Label: "Color",
			},
		},
	}
	m := New(cfg)

	// Start on first field (name)
	require.Equal(t, 0, m.focusedIndex)

	// Tab to toggle field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, 1, m.focusedIndex, "on toggle field")

	// Down on toggle should move to next field (color), not toggle within
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, 2, m.focusedIndex, "after down on toggle - should be on color field")

	// Reset to toggle field to test Up
	m.focusedIndex = 1

	// Up on toggle should move to previous field (name)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	require.Equal(t, 0, m.focusedIndex, "after up on toggle - should be on name field")
}

func TestToggleField_TabExitsToggle(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:   "mode",
				Type:  FieldTypeToggle,
				Label: "Mode",
				Options: []ListOption{
					{Label: "Option A", Value: "a"},
					{Label: "Option B", Value: "b"},
				},
			},
		},
	}
	m := New(cfg)

	// Start on toggle field
	require.Equal(t, 0, m.focusedIndex)

	// Tab should move to buttons
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, -1, m.focusedIndex, "expected focus on buttons")
}

func TestToggleField_SubmitIncludesValue(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:   "mode",
				Type:  FieldTypeToggle,
				Label: "Mode",
				Options: []ListOption{
					{Label: "Option A", Value: "a"},
					{Label: "Option B", Value: "b"},
				},
			},
		},
	}
	m := New(cfg)

	// Switch to second option
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})

	// Navigate to submit and press Enter
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // to submit
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	require.NotNil(t, cmd, "expected submit command")
	msg := cmd()
	submitMsg, ok := msg.(SubmitMsg)
	require.True(t, ok, "expected SubmitMsg, got %T", msg)
	require.Equal(t, "b", submitMsg.Values["mode"])
}

func TestToggleField_InitialIndexClamping(t *testing.T) {
	// Test that out-of-range initial indices are clamped
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:                "mode",
				Type:               FieldTypeToggle,
				Label:              "Mode",
				InitialToggleIndex: 99, // Out of range
				Options: []ListOption{
					{Label: "Option A", Value: "a"},
					{Label: "Option B", Value: "b"},
				},
			},
		},
	}
	m := New(cfg)

	// Should be clamped to 1 (max valid index)
	require.Equal(t, 1, m.fields[0].toggleIndex)
}

func TestToggleField_NegativeInitialIndex(t *testing.T) {
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{
				Key:                "mode",
				Type:               FieldTypeToggle,
				Label:              "Mode",
				InitialToggleIndex: -1, // Negative
				Options: []ListOption{
					{Label: "Option A", Value: "a"},
					{Label: "Option B", Value: "b"},
				},
			},
		},
	}
	m := New(cfg)

	// Should be clamped to 0
	require.Equal(t, 0, m.fields[0].toggleIndex)
}

// --- Toggle Field Golden Tests ---

func TestGolden_ToggleFieldFocused_FirstSelected(t *testing.T) {
	cfg := FormConfig{
		Title: "Save Tree Column",
		Fields: []FieldConfig{
			{
				Key:   "mode",
				Type:  FieldTypeToggle,
				Label: "Tree Mode",
				Options: []ListOption{
					{Label: "Dependencies", Value: "deps"},
					{Label: "Parent-Child", Value: "children"},
				},
			},
		},
		SubmitLabel: "Save",
		MinWidth:    50,
	}
	m := New(cfg).SetSize(80, 24)

	compareGolden(t, "toggle_field_first_selected", m.View())
}

func TestGolden_ToggleFieldFocused_SecondSelected(t *testing.T) {
	cfg := FormConfig{
		Title: "Save Tree Column",
		Fields: []FieldConfig{
			{
				Key:                "mode",
				Type:               FieldTypeToggle,
				Label:              "Tree Mode",
				InitialToggleIndex: 1,
				Options: []ListOption{
					{Label: "Dependencies", Value: "deps"},
					{Label: "Parent-Child", Value: "children"},
				},
			},
		},
		SubmitLabel: "Save",
		MinWidth:    50,
	}
	m := New(cfg).SetSize(80, 24)

	compareGolden(t, "toggle_field_second_selected", m.View())
}

// --- HeaderContent Tests ---

func TestHeaderContent_NilCallback_NoHeaderSection(t *testing.T) {
	// Test that when HeaderContent is nil (the default), no header section appears
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{Key: "name", Type: FieldTypeText, Label: "Name"},
		},
		HeaderContent: nil, // explicitly nil
	}
	m := New(cfg).SetSize(80, 24)

	view := m.View()
	// The view should contain "Name" (the field label) but no header content
	// Since HeaderContent is nil, there should be no extra content between
	// the title border and the first field
	require.Contains(t, view, "Name")
	// Verify the config was stored correctly
	require.Nil(t, m.config.HeaderContent)
}

func TestHeaderContent_EmptyStringReturn_NoHeaderSection(t *testing.T) {
	// Test that when HeaderContent returns an empty string, no header section appears
	callCount := 0
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{Key: "name", Type: FieldTypeText, Label: "Name"},
		},
		HeaderContent: func(width int) string {
			callCount++
			return "" // return empty string
		},
	}
	m := New(cfg).SetSize(80, 24)

	view := m.View()
	// The view should contain "Name" (the field label)
	require.Contains(t, view, "Name")
	// Verify the callback is stored (not nil)
	require.NotNil(t, m.config.HeaderContent)
	// The callback is called during View(), but empty string produces no header
	require.Greater(t, callCount, 0, "HeaderContent callback should be called during View()")
}

func TestHeaderContent_ReceivesCorrectContentWidth(t *testing.T) {
	// Test that HeaderContent callback receives the inner content width
	// accounting for content padding on each side
	var receivedWidth int
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{Key: "name", Type: FieldTypeText, Label: "Name"},
		},
		MinWidth: 50, // contentWidth = 50 - 2 = 48, headerWidth = 48 - 2 = 46
		HeaderContent: func(width int) string {
			receivedWidth = width
			return "Header"
		},
	}
	m := New(cfg).SetSize(80, 24)

	_ = m.View()

	// headerWidth = MinWidth - 2 (modal border) - 2 (content padding)
	expectedWidth := 50 - 2 - 2
	require.Equal(t, expectedWidth, receivedWidth, "HeaderContent should receive inner content width")
}

func TestGolden_WithHeaderContent(t *testing.T) {
	cfg := FormConfig{
		Title: "Edit Issue",
		Fields: []FieldConfig{
			{Key: "priority", Type: FieldTypeText, Label: "Priority"},
		},
		SubmitLabel: "Save",
		MinWidth:    50,
		HeaderContent: func(width int) string {
			return "[F][P1] bd-123 Sample issue title"
		},
	}
	m := New(cfg).SetSize(80, 24)

	compareGolden(t, "with_header_content", m.View())
}

func TestGolden_HeaderContentEmpty(t *testing.T) {
	cfg := FormConfig{
		Title: "Edit Issue",
		Fields: []FieldConfig{
			{Key: "priority", Type: FieldTypeText, Label: "Priority"},
		},
		SubmitLabel: "Save",
		MinWidth:    50,
		HeaderContent: func(width int) string {
			return "" // empty string should produce no header section
		},
	}
	m := New(cfg).SetSize(80, 24)

	compareGolden(t, "header_content_empty", m.View())
}

// --- Ctrl+S Save Tests ---

func TestKeyboard_CtrlS_SubmitsFromField(t *testing.T) {
	cfg := FormConfig{
		Title:  "Test Form",
		Fields: []FieldConfig{{Key: "name", Type: FieldTypeText, Label: "Name", InitialValue: "test"}},
	}
	m := New(cfg)

	// Focus is on field, not button
	require.Equal(t, 0, m.focusedIndex)

	// Ctrl+S should trigger submit
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	require.NotNil(t, cmd, "expected submit command")
	msg := cmd()
	submitMsg, ok := msg.(SubmitMsg)
	require.True(t, ok, "expected SubmitMsg, got %T", msg)
	require.Equal(t, "test", submitMsg.Values["name"])
}

func TestKeyboard_CtrlS_SubmitsFromButton(t *testing.T) {
	cfg := FormConfig{
		Title:  "Test Form",
		Fields: []FieldConfig{{Key: "name", Type: FieldTypeText, Label: "Name", InitialValue: "test"}},
	}
	m := New(cfg)

	// Navigate to submit button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, -1, m.focusedIndex, "expected buttons focus")
	require.Equal(t, 0, m.focusedButton, "expected submit button")

	// Ctrl+S should trigger submit
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	require.NotNil(t, cmd, "expected submit command")
	msg := cmd()
	submitMsg, ok := msg.(SubmitMsg)
	require.True(t, ok, "expected SubmitMsg, got %T", msg)
	require.Equal(t, "test", submitMsg.Values["name"])
}

func TestKeyboard_CtrlS_RunsValidation(t *testing.T) {
	cfg := FormConfig{
		Title:  "Test Form",
		Fields: []FieldConfig{{Key: "name", Type: FieldTypeText, Label: "Name"}}, // No initial value = empty
		Validate: func(values map[string]any) error {
			name := values["name"].(string)
			if strings.TrimSpace(name) == "" {
				return errors.New("Name is required")
			}
			return nil
		},
	}
	m := New(cfg)

	// Focus is on field (empty name)
	require.Equal(t, 0, m.focusedIndex)

	// Ctrl+S should run validation and fail
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	require.Nil(t, cmd, "expected nil command due to validation error")
	require.Equal(t, "Name is required", m.validationError)
}

func TestKeyboard_CtrlS_NotInColorPicker(t *testing.T) {
	cfg := FormConfig{
		Title:  "Test Form",
		Fields: []FieldConfig{{Key: "color", Type: FieldTypeColor, Label: "Color", InitialColor: "#73F59F"}},
	}
	m := New(cfg).SetSize(80, 24)

	// Open colorpicker
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, m.showColorPicker, "expected colorpicker to be open")

	// Ctrl+S should be forwarded to colorpicker, not trigger submit
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})

	// Should NOT have triggered SubmitMsg - colorpicker intercepts
	if cmd != nil {
		msg := cmd()
		_, isSubmit := msg.(SubmitMsg)
		require.False(t, isSubmit, "Ctrl+S should not submit when colorpicker is open")
	}

	// Colorpicker should still be open
	require.True(t, m.showColorPicker, "colorpicker should still be open after Ctrl+S")
}

// --- TitleContent Tests ---

func TestTitleContent_NilCallback_NoTitleSection(t *testing.T) {
	// Test that when TitleContent is nil (the default), only the title appears
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{Key: "name", Type: FieldTypeText, Label: "Name"},
		},
		TitleContent: nil, // explicitly nil
	}
	m := New(cfg).SetSize(80, 24)

	view := m.View()
	// The view should contain "Test Form" (the title)
	require.Contains(t, view, "Test Form")
	// Verify the config was stored correctly
	require.Nil(t, m.config.TitleContent)
}

func TestTitleContent_EmptyStringReturn_NoTitleContent(t *testing.T) {
	// Test that when TitleContent returns an empty string, only the title appears
	callCount := 0
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{Key: "name", Type: FieldTypeText, Label: "Name"},
		},
		TitleContent: func(width int) string {
			callCount++
			return "" // return empty string
		},
	}
	m := New(cfg).SetSize(80, 24)

	view := m.View()
	// The view should contain "Test Form" (the title)
	require.Contains(t, view, "Test Form")
	// Verify the callback is stored (not nil)
	require.NotNil(t, m.config.TitleContent)
	// The callback is called during View()
	require.Greater(t, callCount, 0, "TitleContent callback should be called during View()")
}

func TestTitleContent_ReceivesCorrectContentWidth(t *testing.T) {
	// Test that TitleContent callback receives correct contentWidth (minWidth - 2)
	var receivedWidth int
	cfg := FormConfig{
		Title: "Test Form",
		Fields: []FieldConfig{
			{Key: "name", Type: FieldTypeText, Label: "Name"},
		},
		MinWidth: 50, // contentWidth should be 50 - 2 = 48
		TitleContent: func(width int) string {
			receivedWidth = width
			return "[F][P1]"
		},
	}
	m := New(cfg).SetSize(80, 24)

	_ = m.View()

	// contentWidth = MinWidth - 2 (for modal border)
	expectedWidth := 50 - 2
	require.Equal(t, expectedWidth, receivedWidth, "TitleContent should receive contentWidth (MinWidth - 2)")
}

func TestGolden_WithTitleContent(t *testing.T) {
	cfg := FormConfig{
		Title: "Edit Issue",
		Fields: []FieldConfig{
			{Key: "priority", Type: FieldTypeText, Label: "Priority"},
		},
		SubmitLabel: "Save",
		MinWidth:    50,
		TitleContent: func(width int) string {
			return "[F][P1][bd-123]"
		},
	}
	m := New(cfg).SetSize(80, 24)

	compareGolden(t, "with_title_content", m.View())
}

func TestGolden_TitleContentEmpty(t *testing.T) {
	cfg := FormConfig{
		Title: "Edit Issue",
		Fields: []FieldConfig{
			{Key: "priority", Type: FieldTypeText, Label: "Priority"},
		},
		SubmitLabel: "Save",
		MinWidth:    50,
		TitleContent: func(width int) string {
			return "" // empty string should produce title-only section
		},
	}
	m := New(cfg).SetSize(80, 24)

	compareGolden(t, "title_content_empty", m.View())
}
