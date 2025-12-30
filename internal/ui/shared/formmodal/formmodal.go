package formmodal

import (
	"strings"

	"github.com/zjrosen/perles/internal/keys"
	"github.com/zjrosen/perles/internal/ui/shared/colorpicker"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// SubmitMsg is sent when the form is submitted successfully.
//
// The Values map contains all field values keyed by FieldConfig.Key.
// Value types depend on field type:
//   - FieldTypeText: string
//   - FieldTypeColor: string (hex color, e.g., "#73F59F")
//   - FieldTypeList: []string (selected values)
//   - FieldTypeSelect: string (single selected value)
//
// Example:
//
//	switch msg := msg.(type) {
//	case formmodal.SubmitMsg:
//	    name := msg.Values["name"].(string)
//	    color := msg.Values["color"].(string)
//	    views := msg.Values["views"].([]string)
//	}
type SubmitMsg struct {
	Values map[string]any // Field values keyed by FieldConfig.Key
}

// CancelMsg is sent when the form is cancelled (via Esc key or Cancel button).
type CancelMsg struct{}

// Model is the form modal state.
//
// Create a new Model with New(cfg). The Model implements the Bubble Tea
// Model interface with Init(), Update(), and View() methods.
//
// Model is immutable - all methods return a new Model rather than
// modifying the receiver.
type Model struct {
	config        FormConfig
	fields        []fieldState
	focusedIndex  int // Index into fields (-1 = on buttons)
	focusedButton int // 0 = submit, 1 = cancel (when focusedIndex == -1)

	// Viewport for overlay positioning
	width, height int

	// Sub-overlay (e.g., colorpicker)
	colorPicker     colorpicker.Model
	showColorPicker bool

	// Validation error
	validationError string
}

// New creates a new form modal with the given configuration.
//
// The form starts with focus on the first field (or the submit button
// if there are no fields). Use SetSize to set viewport dimensions
// before rendering.
//
// Example:
//
//	cfg := FormConfig{Title: "Edit", Fields: []FieldConfig{...}}
//	m := New(cfg).SetSize(80, 24)
func New(cfg FormConfig) Model {
	m := Model{
		config:       cfg,
		fields:       make([]fieldState, len(cfg.Fields)),
		focusedIndex: 0,
		colorPicker:  colorpicker.New(),
	}

	// Initialize field states
	for i, fieldCfg := range cfg.Fields {
		m.fields[i] = newFieldState(fieldCfg)
	}

	// Focus the first text input if it exists
	if len(m.fields) > 0 && m.fields[0].config.Type == FieldTypeText {
		m.fields[0].textInput.Focus()
	}

	// If no fields, start on submit button
	if len(m.fields) == 0 {
		m.focusedIndex = -1
	}

	return m
}

// Init returns the initial command for the Bubble Tea model.
// Returns a cursor blink command if the first focused field is a text input.
func (m Model) Init() tea.Cmd {
	// Find the first focused text input and return its blink command
	if m.focusedIndex >= 0 && m.focusedIndex < len(m.fields) {
		if m.fields[m.focusedIndex].config.Type == FieldTypeText {
			return textinput.Blink
		}
	}
	return nil
}

// Update handles messages for the form modal.
//
// Returns SubmitMsg when form is submitted successfully, CancelMsg when
// cancelled. Returns nil commands for internal state changes.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	// Handle colorpicker result messages first
	switch msg := msg.(type) {
	case colorpicker.SelectMsg:
		m.showColorPicker = false
		// Update the focused color field with selected color
		if m.focusedIndex >= 0 && m.focusedIndex < len(m.fields) {
			fs := &m.fields[m.focusedIndex]
			if fs.config.Type == FieldTypeColor {
				fs.selectedColor = msg.Hex
			}
		}
		return m, nil

	case colorpicker.CancelMsg:
		m.showColorPicker = false
		return m, nil
	}

	// Forward all messages to colorpicker when it's open
	if m.showColorPicker {
		var cmd tea.Cmd
		m.colorPicker, cmd = m.colorPicker.Update(msg)
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	// Forward to focused text input if applicable
	if m.focusedIndex >= 0 && m.focusedIndex < len(m.fields) {
		fs := &m.fields[m.focusedIndex]
		if fs.config.Type == FieldTypeText {
			var cmd tea.Cmd
			fs.textInput, cmd = fs.textInput.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

// handleKeyMsg processes keyboard input.
func (m Model) handleKeyMsg(msg tea.KeyMsg) (Model, tea.Cmd) {
	// Handle Esc globally
	if key.Matches(msg, keys.Common.Escape) {
		if m.config.OnCancel != nil {
			return m, func() tea.Msg { return m.config.OnCancel() }
		}
		return m, func() tea.Msg { return CancelMsg{} }
	}

	// Dispatch to editable list handler if that field type is focused
	if m.focusedIndex >= 0 && m.focusedIndex < len(m.fields) {
		fs := &m.fields[m.focusedIndex]
		if fs.config.Type == FieldTypeEditableList {
			return m.handleKeyForEditableList(msg, fs)
		}
	}

	switch {
	case key.Matches(msg, keys.Component.Tab), key.Matches(msg, keys.Component.Next):
		m = m.nextField()
		return m, m.blinkCmd()

	case key.Matches(msg, keys.Component.ShiftTab), key.Matches(msg, keys.Component.Prev):
		m = m.prevField()
		return m, m.blinkCmd()

	case key.Matches(msg, keys.Common.Enter):
		return m.handleEnter()

	case key.Matches(msg, keys.Common.Left):
		// For toggle fields, switch to first option
		if m.focusedIndex >= 0 && m.focusedIndex < len(m.fields) {
			fs := &m.fields[m.focusedIndex]
			if fs.config.Type == FieldTypeToggle && fs.toggleIndex == 1 {
				fs.toggleIndex = 0
				return m, nil
			}
		}
		// Navigate between buttons when focused on buttons
		if m.focusedIndex == -1 && m.focusedButton == 1 {
			m.focusedButton = 0
			return m, nil
		}

	case key.Matches(msg, keys.Common.Right):
		// For toggle fields, switch to second option
		if m.focusedIndex >= 0 && m.focusedIndex < len(m.fields) {
			fs := &m.fields[m.focusedIndex]
			if fs.config.Type == FieldTypeToggle && fs.toggleIndex == 0 {
				fs.toggleIndex = 1
				return m, nil
			}
		}
		// Navigate between buttons when focused on buttons
		if m.focusedIndex == -1 && m.focusedButton == 0 {
			m.focusedButton = 1
			return m, nil
		}

	case key.Matches(msg, keys.Common.Down):
		// For list fields, navigate within the list
		if m.focusedIndex >= 0 && m.focusedIndex < len(m.fields) {
			fs := &m.fields[m.focusedIndex]
			if fs.config.Type == FieldTypeList || fs.config.Type == FieldTypeSelect {
				if fs.listCursor < len(fs.listItems)-1 {
					fs.listCursor++
				}
				return m, nil
			}
			// For toggle fields, j/down moves to next field (not within toggle)
			if fs.config.Type == FieldTypeToggle {
				m = m.nextField()
				return m, m.blinkCmd()
			}
		} else if m.focusedIndex == -1 {
			// On buttons: Save -> Cancel -> first field
			if m.focusedButton == 0 {
				m.focusedButton = 1
				return m, nil
			} else if len(m.fields) > 0 {
				m.focusedIndex = 0
				m.focusNextFieldByType()
				return m, m.blinkCmd()
			}
		}

	case key.Matches(msg, keys.Common.Up):
		// For list fields, navigate within the list
		if m.focusedIndex >= 0 && m.focusedIndex < len(m.fields) {
			fs := &m.fields[m.focusedIndex]
			if fs.config.Type == FieldTypeList || fs.config.Type == FieldTypeSelect {
				if fs.listCursor > 0 {
					fs.listCursor--
				}
				return m, nil
			}
			// For toggle fields, k/up moves to previous field (not within toggle)
			if fs.config.Type == FieldTypeToggle {
				m = m.prevField()
				return m, m.blinkCmd()
			}
		} else if m.focusedIndex == -1 {
			// On buttons: Cancel -> Save -> last field
			if m.focusedButton == 1 {
				m.focusedButton = 0
				return m, nil
			} else if len(m.fields) > 0 {
				m.focusedIndex = len(m.fields) - 1
				m.focusPrevFieldByType()
				return m, m.blinkCmd()
			}
		}

	case key.Matches(msg, keys.Component.Toggle):
		// For list fields, toggle selection
		if m.focusedIndex >= 0 && m.focusedIndex < len(m.fields) {
			fs := &m.fields[m.focusedIndex]
			if fs.config.Type == FieldTypeList {
				if fs.listCursor >= 0 && fs.listCursor < len(fs.listItems) {
					if fs.config.MultiSelect {
						// Multi-select: toggle current item
						fs.listItems[fs.listCursor].selected = !fs.listItems[fs.listCursor].selected
					} else {
						// Single-select: select current, deselect others
						for i := range fs.listItems {
							fs.listItems[i].selected = (i == fs.listCursor)
						}
					}
				}
				return m, nil
			}
			if fs.config.Type == FieldTypeSelect {
				// Single-select: select current, deselect others
				if fs.listCursor >= 0 && fs.listCursor < len(fs.listItems) {
					for i := range fs.listItems {
						fs.listItems[i].selected = (i == fs.listCursor)
					}
				}
				return m, nil
			}
		}
	}

	// Forward to focused text input for character input
	if m.focusedIndex >= 0 && m.focusedIndex < len(m.fields) {
		fs := &m.fields[m.focusedIndex]
		if fs.config.Type == FieldTypeText {
			var cmd tea.Cmd
			fs.textInput, cmd = fs.textInput.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

// handleEnter processes Enter key based on current focus.
func (m Model) handleEnter() (Model, tea.Cmd) {
	if m.focusedIndex >= 0 && m.focusedIndex < len(m.fields) {
		fs := &m.fields[m.focusedIndex]

		// Color field: open colorpicker overlay
		if fs.config.Type == FieldTypeColor {
			m.showColorPicker = true
			m.colorPicker = m.colorPicker.SetSelected(fs.selectedColor).SetSize(m.width, m.height)
			return m, nil
		}

		// Other fields: advance to next field
		m = m.nextField()
		return m, m.blinkCmd()
	}

	// On buttons
	switch m.focusedButton {
	case 0: // Submit
		return m.submit()
	case 1: // Cancel
		if m.config.OnCancel != nil {
			return m, func() tea.Msg { return m.config.OnCancel() }
		}
		return m, func() tea.Msg { return CancelMsg{} }
	}

	return m, nil
}

// submit validates and submits the form.
func (m Model) submit() (Model, tea.Cmd) {
	// Clear previous error
	m.validationError = ""

	// Build values map
	values := make(map[string]any)
	for i := range m.fields {
		values[m.fields[i].config.Key] = m.fields[i].value()
	}

	// Run validation if provided
	if m.config.Validate != nil {
		if err := m.config.Validate(values); err != nil {
			m.validationError = err.Error()
			return m, nil
		}
	}

	// Use factory if provided, otherwise default SubmitMsg
	if m.config.OnSubmit != nil {
		return m, func() tea.Msg { return m.config.OnSubmit(values) }
	}
	return m, func() tea.Msg { return SubmitMsg{Values: values} }
}

// nextField moves focus to the next field or button.
func (m Model) nextField() Model {
	if m.focusedIndex >= 0 {
		// Blur current field
		fs := &m.fields[m.focusedIndex]
		switch fs.config.Type {
		case FieldTypeText:
			fs.textInput.Blur()
		case FieldTypeEditableList:
			fs.addInput.Blur()
			fs.subFocus = SubFocusList // Reset for next time
		}

		if m.focusedIndex < len(m.fields)-1 {
			// Move to next field
			m.focusedIndex++
			m.focusNextFieldByType()
		} else {
			// Move to submit button
			m.focusedIndex = -1
			m.focusedButton = 0
		}
	} else {
		// On buttons
		if m.focusedButton == 0 {
			m.focusedButton = 1
		} else {
			// Wrap to first field (or stay on buttons if no fields)
			if len(m.fields) > 0 {
				m.focusedIndex = 0
				m.focusNextFieldByType()
			} else {
				m.focusedButton = 0
			}
		}
	}
	return m
}

// focusNextFieldByType sets focus on the current field based on its type.
// Called when navigating forward into a field.
func (m *Model) focusNextFieldByType() {
	fs := &m.fields[m.focusedIndex]
	switch fs.config.Type {
	case FieldTypeText:
		fs.textInput.Focus()
	case FieldTypeEditableList:
		fs.subFocus = SubFocusList // Start on list when navigating forward
	}
}

// prevField moves focus to the previous field or button.
func (m Model) prevField() Model {
	if m.focusedIndex >= 0 {
		// Blur current field
		fs := &m.fields[m.focusedIndex]
		switch fs.config.Type {
		case FieldTypeText:
			fs.textInput.Blur()
		case FieldTypeEditableList:
			fs.addInput.Blur()
			fs.subFocus = SubFocusList // Reset for next time
		}

		if m.focusedIndex > 0 {
			// Move to previous field
			m.focusedIndex--
			m.focusPrevFieldByType()
		} else {
			// Wrap to cancel button
			m.focusedIndex = -1
			m.focusedButton = 1
		}
	} else {
		// On buttons
		if m.focusedButton == 1 {
			m.focusedButton = 0
		} else {
			// Move to last field (or stay on buttons if no fields)
			if len(m.fields) > 0 {
				m.focusedIndex = len(m.fields) - 1
				m.focusPrevFieldByType()
			} else {
				m.focusedButton = 1
			}
		}
	}
	return m
}

// focusPrevFieldByType sets focus on the current field based on its type.
// Called when navigating backward into a field.
func (m *Model) focusPrevFieldByType() {
	fs := &m.fields[m.focusedIndex]
	switch fs.config.Type {
	case FieldTypeText:
		fs.textInput.Focus()
	case FieldTypeEditableList:
		// When navigating backward, land on the input section first
		fs.subFocus = SubFocusInput
		fs.addInput.Focus()
	}
}

// blinkCmd returns the blink command if the currently focused field is a text input.
func (m Model) blinkCmd() tea.Cmd {
	if m.focusedIndex >= 0 && m.focusedIndex < len(m.fields) {
		fs := &m.fields[m.focusedIndex]
		switch fs.config.Type {
		case FieldTypeText:
			return textinput.Blink
		case FieldTypeEditableList:
			if fs.subFocus == SubFocusInput {
				return textinput.Blink
			}
		}
	}
	return nil
}

// SetSize sets the viewport dimensions for overlay rendering.
// Call this before View() or Overlay() to ensure proper centering.
func (m Model) SetSize(w, h int) Model {
	m.width = w
	m.height = h
	return m
}

// listContains checks if the editable list already contains a value.
// Used for duplicate detection when AllowDuplicates is false.
func (m Model) listContains(fs *fieldState, value string) bool {
	for _, item := range fs.listItems {
		if item.value == value {
			return true
		}
	}
	return false
}

// handleKeyForEditableList processes keyboard input for editable list fields.
// The editable list has two sub-sections: list and input.
// Navigation rules:
//   - Tab: list->input, input->next field
//   - Shift+Tab: input->list, list->prev field
//   - j/Down: navigate down in list, or next field from input
//   - k/Up: navigate up in list (at top->input), or list (at bottom) from input
//   - Space: toggle in list, insert in input
//   - Enter: toggle in list, add item from input
func (m Model) handleKeyForEditableList(msg tea.KeyMsg, fs *fieldState) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Component.Tab):
		if fs.subFocus == SubFocusList {
			// Move to input within same field
			fs.subFocus = SubFocusInput
			fs.addInput.Focus()
			return m, textinput.Blink
		}
		// Move to next field
		return m.nextField(), m.blinkCmd()

	case key.Matches(msg, keys.Component.ShiftTab):
		if fs.subFocus == SubFocusInput {
			// Move back to list within same field
			fs.subFocus = SubFocusList
			fs.addInput.Blur()
			// Position cursor at bottom of list
			if len(fs.listItems) > 0 {
				fs.listCursor = len(fs.listItems) - 1
			}
			return m, nil
		}
		// Move to previous field
		return m.prevField(), m.blinkCmd()

	case msg.String() == "j" || msg.String() == "k":
		// j/k only navigate in list mode; in input they type characters
		// Keep as msg.String() to allow typing in text inputs
		if fs.subFocus == SubFocusList {
			if msg.String() == "j" {
				if len(fs.listItems) > 0 && fs.listCursor < len(fs.listItems)-1 {
					fs.listCursor++
					return m, nil
				}
				// At bottom of list, move to input section
				fs.subFocus = SubFocusInput
				fs.addInput.Focus()
				return m, textinput.Blink
			}
			// k
			if fs.listCursor > 0 {
				fs.listCursor--
				return m, nil
			}
			// At top of list, go to previous field (wraps to cancel)
			return m.prevField(), m.blinkCmd()
		}
		// In input, let j/k type characters - fall through to input handler

	case key.Matches(msg, keys.Common.Down), key.Matches(msg, keys.Component.Next):
		if fs.subFocus == SubFocusList {
			if len(fs.listItems) > 0 && fs.listCursor < len(fs.listItems)-1 {
				fs.listCursor++
				return m, nil
			}
			// At bottom of list, move to input section
			fs.subFocus = SubFocusInput
			fs.addInput.Focus()
			return m, textinput.Blink
		}
		// In input, down/ctrl+n moves to next field
		fs.addInput.Blur()
		return m.nextField(), m.blinkCmd()

	case key.Matches(msg, keys.Common.Up), key.Matches(msg, keys.Component.Prev):
		if fs.subFocus == SubFocusList {
			if fs.listCursor > 0 {
				fs.listCursor--
				return m, nil
			}
			// At top of list, go to previous field (wraps to cancel)
			return m.prevField(), m.blinkCmd()
		}
		// In input, up/ctrl+p moves to list (at bottom)
		fs.subFocus = SubFocusList
		fs.addInput.Blur()
		if len(fs.listItems) > 0 {
			fs.listCursor = len(fs.listItems) - 1
		}
		return m, nil

	case key.Matches(msg, keys.Component.Toggle):
		if fs.subFocus == SubFocusList && len(fs.listItems) > 0 {
			fs.listItems[fs.listCursor].selected = !fs.listItems[fs.listCursor].selected
			return m, nil
		}
		// Fall through to let input handle space

	case key.Matches(msg, keys.Common.Enter):
		if fs.subFocus == SubFocusList && len(fs.listItems) > 0 {
			// Toggle in list
			fs.listItems[fs.listCursor].selected = !fs.listItems[fs.listCursor].selected
			return m, nil
		}
		if fs.subFocus == SubFocusInput {
			// Add item to list
			value := strings.TrimSpace(fs.addInput.Value())
			if value != "" && (fs.config.AllowDuplicates || !m.listContains(fs, value)) {
				fs.listItems = append(fs.listItems, listItem{
					label:    value,
					value:    value,
					selected: true, // New items start selected
				})
				fs.addInput.SetValue("")
			}
			return m, nil
		}
	}

	// Forward other keys to input when focused
	if fs.subFocus == SubFocusInput {
		var cmd tea.Cmd
		fs.addInput, cmd = fs.addInput.Update(msg)
		return m, cmd
	}

	return m, nil
}
