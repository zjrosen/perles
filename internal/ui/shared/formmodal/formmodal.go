package formmodal

import (
	"strings"

	zone "github.com/lrstanley/bubblezone"

	"github.com/zjrosen/perles/internal/keys"
	"github.com/zjrosen/perles/internal/ui/shared/colorpicker"
	"github.com/zjrosen/perles/internal/ui/shared/vimtextarea"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
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

	// Body scrolling viewport
	bodyViewport viewport.Model

	// Sub-overlay (e.g., colorpicker)
	colorPicker     colorpicker.Model
	showColorPicker bool

	// Validation error
	validationError string

	// loadingText, if non-empty, shows a loading indicator instead of buttons.
	loadingText string
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
		bodyViewport: viewport.New(0, 0),
	}

	// Initialize field states
	for i, fieldCfg := range cfg.Fields {
		m.fields[i] = newFieldState(fieldCfg)
	}

	// Find the first visible field to focus
	firstVisible := m.firstVisibleFieldIndex()
	if firstVisible >= 0 {
		m.focusedIndex = firstVisible
		// Focus the first visible focusable input
		fs := &m.fields[firstVisible]
		switch fs.config.Type {
		case FieldTypeText:
			fs.textInput.Focus()
		case FieldTypeTextArea:
			fs.textArea.Focus()
		case FieldTypeSearchSelect:
			// Start collapsed - don't focus search input yet
			fs.searchExpanded = false
		}
	} else {
		// No visible fields, start on submit button
		m.focusedIndex = -1
	}

	return m
}

// Init returns the initial command for the Bubble Tea model.
// Returns a cursor blink command if the first focused field has a text input.
func (m Model) Init() tea.Cmd {
	if m.focusedIndex >= 0 && m.focusedIndex < len(m.fields) {
		fs := &m.fields[m.focusedIndex]
		switch fs.config.Type {
		case FieldTypeText:
			return textinput.Blink
		case FieldTypeSearchSelect:
			if fs.searchExpanded {
				return textinput.Blink
			}
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

	case vimtextarea.SubmitMsg:
		// When a textarea submits (Enter key), advance to next field
		if m.focusedIndex >= 0 && m.focusedIndex < len(m.fields) {
			fs := &m.fields[m.focusedIndex]
			if fs.config.Type == FieldTypeTextArea {
				m = m.nextField()
				return m, m.blinkCmd()
			}
		}
		return m, nil
	}

	// Forward all messages to colorpicker when it's open
	if m.showColorPicker {
		var cmd tea.Cmd
		m.colorPicker, cmd = m.colorPicker.Update(msg)
		return m, cmd
	}

	// Ignore keyboard input when loading
	if m.loadingText != "" {
		if _, ok := msg.(tea.KeyMsg); ok {
			return m, nil
		}
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case tea.MouseMsg:
		// Handle left-click release for zone clicks
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionRelease {
			// Check for button clicks first
			if cmd := m.handleButtonClick(msg); cmd != nil {
				return m, cmd
			}
			// Check for item clicks (more specific zones)
			if m.handleItemClick(msg) {
				return m, nil
			}
			// Check for field clicks (focus the field)
			if m.handleFieldClick(msg) {
				return m, m.blinkCmd()
			}
		}
		// Forward mouse events to viewport for scrolling
		var cmd tea.Cmd
		m.bodyViewport, cmd = m.bodyViewport.Update(msg)
		return m, cmd

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.bodyViewport.Height = m.calculateMaxBodyHeight()
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
	// Handle Esc - check if it should be consumed by the current field first
	if key.Matches(msg, keys.Common.Escape) {
		if m.focusedIndex >= 0 && m.focusedIndex < len(m.fields) {
			fs := &m.fields[m.focusedIndex]
			// If a SearchSelect field is expanded, collapse it instead of closing modal
			if fs.config.Type == FieldTypeSearchSelect && fs.searchExpanded {
				fs.searchExpanded = false
				fs.searchInput.Blur()
				return m, nil
			}
			// If a TextArea field has vim enabled and is in Insert mode, let Esc switch to Normal mode
			if fs.config.Type == FieldTypeTextArea && fs.config.VimEnabled && fs.textArea.Mode() == vimtextarea.ModeInsert {
				var cmd tea.Cmd
				fs.textArea, cmd = fs.textArea.Update(msg)
				return m, cmd
			}
		}
		// Otherwise, cancel the modal
		if m.config.OnCancel != nil {
			return m, func() tea.Msg { return m.config.OnCancel() }
		}
		return m, func() tea.Msg { return CancelMsg{} }
	}

	// Handle Ctrl+S globally (save from any field)
	if key.Matches(msg, keys.Component.Save) {
		return m.submit()
	}

	// Dispatch to specialized handlers for composite field types
	if m.focusedIndex >= 0 && m.focusedIndex < len(m.fields) {
		fs := &m.fields[m.focusedIndex]
		switch fs.config.Type {
		case FieldTypeEditableList:
			return m.handleKeyForEditableList(msg, fs)
		case FieldTypeSearchSelect:
			return m.handleKeyForSearchSelect(msg, fs)
		case FieldTypeTextArea:
			return m.handleKeyForTextArea(msg, fs)
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
		// j/k should type in text inputs, not navigate - let them fall through
		if msg.String() == "j" && m.focusedIndex >= 0 && m.focusedIndex < len(m.fields) {
			if m.fields[m.focusedIndex].config.Type == FieldTypeText {
				break // Fall through to text input handler
			}
		}
		// For text fields, arrow down moves to next field
		if m.focusedIndex >= 0 && m.focusedIndex < len(m.fields) {
			fs := &m.fields[m.focusedIndex]
			if fs.config.Type == FieldTypeText {
				m = m.nextField()
				return m, m.blinkCmd()
			}
		}
		// For list fields, navigate within the list or escape at boundary
		if m.focusedIndex >= 0 && m.focusedIndex < len(m.fields) {
			fs := &m.fields[m.focusedIndex]
			if fs.config.Type == FieldTypeList || fs.config.Type == FieldTypeSelect {
				// At bottom (or empty list), escape to next field
				if len(fs.listItems) == 0 || fs.listCursor >= len(fs.listItems)-1 {
					m = m.nextField()
					return m, m.blinkCmd()
				}
				// Otherwise navigate down in list
				fs.listCursor++
				return m, nil
			}
			// For toggle fields, j/down moves to next field (not within toggle)
			if fs.config.Type == FieldTypeToggle {
				m = m.nextField()
				return m, m.blinkCmd()
			}
			// For color fields (when picker not open), j/down moves to next field
			if fs.config.Type == FieldTypeColor && !m.showColorPicker {
				m = m.nextField()
				return m, m.blinkCmd()
			}
		} else if m.focusedIndex == -1 {
			// On buttons: Save -> Cancel -> first field
			if m.focusedButton == 0 {
				m.focusedButton = 1
				return m, nil
			} else if firstVisible := m.firstVisibleFieldIndex(); firstVisible >= 0 {
				m.focusedIndex = firstVisible
				m.focusNextFieldByType()
				return m, m.blinkCmd()
			}
		}

	case key.Matches(msg, keys.Common.Up):
		// j/k should type in text inputs, not navigate - let them fall through
		if msg.String() == "k" && m.focusedIndex >= 0 && m.focusedIndex < len(m.fields) {
			if m.fields[m.focusedIndex].config.Type == FieldTypeText {
				break // Fall through to text input handler
			}
		}
		// For text fields, arrow up moves to previous field
		if m.focusedIndex >= 0 && m.focusedIndex < len(m.fields) {
			fs := &m.fields[m.focusedIndex]
			if fs.config.Type == FieldTypeText {
				m = m.prevField()
				return m, m.blinkCmd()
			}
		}
		// For list fields, navigate within the list or escape at boundary
		if m.focusedIndex >= 0 && m.focusedIndex < len(m.fields) {
			fs := &m.fields[m.focusedIndex]
			if fs.config.Type == FieldTypeList || fs.config.Type == FieldTypeSelect {
				// At top (or empty list), escape to previous field
				if len(fs.listItems) == 0 || fs.listCursor <= 0 {
					m = m.prevField()
					return m, m.blinkCmd()
				}
				// Otherwise navigate up in list
				fs.listCursor--
				return m, nil
			}
			// For toggle fields, k/up moves to previous field (not within toggle)
			if fs.config.Type == FieldTypeToggle {
				m = m.prevField()
				return m, m.blinkCmd()
			}
			// For color fields (when picker not open), k/up moves to previous field
			if fs.config.Type == FieldTypeColor && !m.showColorPicker {
				m = m.prevField()
				return m, m.blinkCmd()
			}
		} else if m.focusedIndex == -1 {
			// On buttons: Cancel -> Save -> last field
			if m.focusedButton == 1 {
				m.focusedButton = 0
				return m, nil
			} else if lastVisible := m.lastVisibleFieldIndex(); lastVisible >= 0 {
				m.focusedIndex = lastVisible
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

		// Select field: toggle selection (same as Space)
		if fs.config.Type == FieldTypeSelect {
			if fs.listCursor >= 0 && fs.listCursor < len(fs.listItems) {
				for i := range fs.listItems {
					fs.listItems[i].selected = (i == fs.listCursor)
				}
			}
			return m, nil
		}

		// List field: toggle selection (same as Space)
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

	// Build values map (only include visible fields)
	values := make(map[string]any)
	for i := range m.fields {
		if m.isFieldVisible(i) {
			values[m.fields[i].config.Key] = m.fields[i].value()
		}
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

// nextField moves focus to the next visible field or button.
func (m Model) nextField() Model {
	if m.focusedIndex >= 0 {
		// Blur current field
		fs := &m.fields[m.focusedIndex]
		switch fs.config.Type {
		case FieldTypeText:
			fs.textInput.Blur()
		case FieldTypeTextArea:
			fs.textArea.Blur()
		case FieldTypeEditableList:
			fs.addInput.Blur()
			fs.subFocus = SubFocusList // Reset for next time
		case FieldTypeSearchSelect:
			fs.searchInput.Blur()
			fs.searchExpanded = false // Collapse when leaving field
		}

		// Find next visible field
		nextIdx := m.focusedIndex + 1
		for nextIdx < len(m.fields) && !m.isFieldVisible(nextIdx) {
			nextIdx++
		}

		if nextIdx < len(m.fields) {
			// Move to next visible field
			m.focusedIndex = nextIdx
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
			// Wrap to first visible field (or stay on buttons if no visible fields)
			firstVisible := m.firstVisibleFieldIndex()
			if firstVisible >= 0 {
				m.focusedIndex = firstVisible
				m.focusNextFieldByType()
			} else {
				m.focusedButton = 0
			}
		}
	}
	m.ensureFocusedFieldVisible()
	return m
}

// firstVisibleFieldIndex returns the index of the first visible field, or -1 if none.
func (m Model) firstVisibleFieldIndex() int {
	for i := range m.fields {
		if m.isFieldVisible(i) {
			return i
		}
	}
	return -1
}

// lastVisibleFieldIndex returns the index of the last visible field, or -1 if none.
func (m Model) lastVisibleFieldIndex() int {
	for i := len(m.fields) - 1; i >= 0; i-- {
		if m.isFieldVisible(i) {
			return i
		}
	}
	return -1
}

// focusNextFieldByType sets focus on the current field based on its type.
// Called when navigating forward into a field.
func (m *Model) focusNextFieldByType() {
	fs := &m.fields[m.focusedIndex]
	switch fs.config.Type {
	case FieldTypeText:
		fs.textInput.Focus()
	case FieldTypeTextArea:
		fs.textArea.Focus()
	case FieldTypeEditableList:
		fs.subFocus = SubFocusList // Start on list when navigating forward
	case FieldTypeList, FieldTypeSelect:
		// Position cursor at first item when entering from above
		fs.listCursor = 0
	case FieldTypeSearchSelect:
		// Start collapsed - user must press Enter to expand
		fs.searchExpanded = false
	}
}

// prevField moves focus to the previous visible field or button.
func (m Model) prevField() Model {
	if m.focusedIndex >= 0 {
		// Blur current field
		fs := &m.fields[m.focusedIndex]
		switch fs.config.Type {
		case FieldTypeText:
			fs.textInput.Blur()
		case FieldTypeTextArea:
			fs.textArea.Blur()
		case FieldTypeEditableList:
			fs.addInput.Blur()
			fs.subFocus = SubFocusList // Reset for next time
		case FieldTypeSearchSelect:
			fs.searchInput.Blur()
			fs.searchExpanded = false // Collapse when leaving field
		}

		// Find previous visible field
		prevIdx := m.focusedIndex - 1
		for prevIdx >= 0 && !m.isFieldVisible(prevIdx) {
			prevIdx--
		}

		if prevIdx >= 0 {
			// Move to previous visible field
			m.focusedIndex = prevIdx
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
			// Move to last visible field (or stay on buttons if no visible fields)
			lastVisible := m.lastVisibleFieldIndex()
			if lastVisible >= 0 {
				m.focusedIndex = lastVisible
				m.focusPrevFieldByType()
			} else {
				m.focusedButton = 1
			}
		}
	}
	m.ensureFocusedFieldVisible()
	return m
}

// focusPrevFieldByType sets focus on the current field based on its type.
// Called when navigating backward into a field.
func (m *Model) focusPrevFieldByType() {
	fs := &m.fields[m.focusedIndex]
	switch fs.config.Type {
	case FieldTypeText:
		fs.textInput.Focus()
	case FieldTypeTextArea:
		fs.textArea.Focus()
	case FieldTypeEditableList:
		// When navigating backward, land on the input section first
		fs.subFocus = SubFocusInput
		fs.addInput.Focus()
	case FieldTypeList, FieldTypeSelect:
		// Position cursor at last item when entering from below
		if len(fs.listItems) > 0 {
			fs.listCursor = len(fs.listItems) - 1
		}
	case FieldTypeSearchSelect:
		// Start collapsed - user must press Enter to expand
		fs.searchExpanded = false
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
		case FieldTypeSearchSelect:
			if fs.searchExpanded {
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
	// Update body viewport height (width is not constrained - content flows naturally)
	m.bodyViewport.Height = m.calculateMaxBodyHeight()
	return m
}

// SetLoading sets the loading state of the form.
// When text is non-empty, the form displays a loading indicator instead of buttons
// and ignores keyboard input. Pass empty string to clear loading state.
func (m Model) SetLoading(text string) Model {
	m.loadingText = text
	return m
}

// IsLoading returns true if the form is in loading state.
func (m Model) IsLoading() bool {
	return m.loadingText != ""
}

// SetError sets the validation error message.
// Pass empty string to clear the error.
func (m Model) SetError(text string) Model {
	m.validationError = text
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

// currentValues returns a map of all current field values.
// Used by VisibleWhen callbacks to check other field states.
func (m Model) currentValues() map[string]any {
	values := make(map[string]any)
	for i := range m.fields {
		values[m.fields[i].config.Key] = m.fields[i].value()
	}
	return values
}

// isFieldVisible returns whether a field should be visible based on its VisibleWhen callback.
// If no callback is set, the field is always visible.
func (m Model) isFieldVisible(index int) bool {
	if index < 0 || index >= len(m.fields) {
		return false
	}
	fs := &m.fields[index]
	if fs.config.VisibleWhen == nil {
		return true
	}
	return fs.config.VisibleWhen(m.currentValues())
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

// handleKeyForSearchSelect processes keyboard input for searchable select fields.
// The field has two states:
//   - Collapsed: Shows selected value, Enter expands to search mode
//   - Expanded: Shows search input + filtered list, Enter selects and collapses
//
// Uses arrow keys (not j/k) for list navigation to avoid conflicts with typing.
func (m Model) handleKeyForSearchSelect(msg tea.KeyMsg, fs *fieldState) (Model, tea.Cmd) {
	// Handle collapsed state (showing selected value)
	if !fs.searchExpanded {
		switch {
		case key.Matches(msg, keys.Component.Tab), msg.Type == tea.KeyDown, key.Matches(msg, keys.Component.Next), msg.String() == "j":
			return m.nextField(), m.blinkCmd()
		case key.Matches(msg, keys.Component.ShiftTab), msg.Type == tea.KeyUp, key.Matches(msg, keys.Component.Prev), msg.String() == "k":
			return m.prevField(), m.blinkCmd()
		case key.Matches(msg, keys.Common.Enter):
			// Expand to show search + list
			fs.searchExpanded = true
			fs.searchInput.SetValue("")
			fs.searchInput.Focus()
			// Reset filter to show all items
			m = m.updateSearchFilter(fs)
			// Position cursor at selected item
			for i, idx := range fs.searchFiltered {
				if fs.listItems[idx].selected {
					fs.listCursor = i
					break
				}
			}
			fs.scrollOffset = 0
			m = m.ensureSearchCursorVisible(fs)
			return m, textinput.Blink
		}
		return m, nil
	}

	// Handle expanded state (search + list visible)
	switch {
	case key.Matches(msg, keys.Component.Tab):
		// Tab collapses and moves to next field
		fs.searchExpanded = false
		fs.searchInput.Blur()
		return m.nextField(), m.blinkCmd()

	case key.Matches(msg, keys.Component.ShiftTab):
		// Shift+Tab collapses and moves to previous field
		fs.searchExpanded = false
		fs.searchInput.Blur()
		return m.prevField(), m.blinkCmd()

	// Note: Escape is handled in handleKeyMsg before dispatch to collapse search

	case msg.Type == tea.KeyDown, key.Matches(msg, keys.Component.Next):
		// Arrow down or ctrl+n navigates list
		if len(fs.searchFiltered) > 0 && fs.listCursor < len(fs.searchFiltered)-1 {
			fs.listCursor++
			m = m.ensureSearchCursorVisible(fs)
		}
		return m, nil

	case msg.Type == tea.KeyUp, key.Matches(msg, keys.Component.Prev):
		// Arrow up or ctrl+p navigates list
		if fs.listCursor > 0 {
			fs.listCursor--
			m = m.ensureSearchCursorVisible(fs)
		}
		return m, nil

	case key.Matches(msg, keys.Common.Enter):
		// Enter selects current item and collapses
		if len(fs.searchFiltered) > 0 {
			// Deselect all first
			for i := range fs.listItems {
				fs.listItems[i].selected = false
			}
			// Select current
			actualIdx := fs.searchFiltered[fs.listCursor]
			fs.listItems[actualIdx].selected = true
		}
		// Collapse back to showing selected value
		fs.searchExpanded = false
		fs.searchInput.Blur()
		return m, nil

	default:
		// Forward all other keys to search input (including j/k for typing)
		var cmd tea.Cmd
		fs.searchInput, cmd = fs.searchInput.Update(msg)
		m = m.updateSearchFilter(fs)
		return m, cmd
	}
}

// updateSearchFilter filters items based on current search text.
func (m Model) updateSearchFilter(fs *fieldState) Model {
	query := strings.ToLower(fs.searchInput.Value())

	if query == "" {
		// Show all items
		fs.searchFiltered = make([]int, len(fs.listItems))
		for i := range fs.listItems {
			fs.searchFiltered[i] = i
		}
	} else {
		// Filter items by label
		fs.searchFiltered = nil
		for i, item := range fs.listItems {
			if strings.Contains(strings.ToLower(item.label), query) {
				fs.searchFiltered = append(fs.searchFiltered, i)
			}
		}
	}

	// Reset cursor if out of bounds
	if fs.listCursor >= len(fs.searchFiltered) {
		fs.listCursor = 0
		fs.scrollOffset = 0
	}

	return m
}

// ensureSearchCursorVisible adjusts scroll offset to keep cursor in view.
func (m Model) ensureSearchCursorVisible(fs *fieldState) Model {
	maxVisible := fs.config.MaxVisibleItems
	if maxVisible <= 0 {
		maxVisible = 5
	}

	if fs.listCursor >= fs.scrollOffset+maxVisible {
		fs.scrollOffset = fs.listCursor - maxVisible + 1
	}
	if fs.listCursor < fs.scrollOffset {
		fs.scrollOffset = fs.listCursor
	}

	return m
}

// handleKeyForTextArea processes keyboard input for textarea fields.
// Tab/Shift+Tab/Ctrl+N/Ctrl+P navigate between fields. All other keys are forwarded to vimtextarea.
// Enter is handled by vimtextarea which emits SubmitMsg (handled in Update).
func (m Model) handleKeyForTextArea(msg tea.KeyMsg, fs *fieldState) (Model, tea.Cmd) {
	// Tab or Ctrl+N navigates to next field
	if key.Matches(msg, keys.Component.Tab) || key.Matches(msg, keys.Component.Next) {
		m = m.nextField()
		return m, m.blinkCmd()
	}

	// Shift+Tab or Ctrl+P navigates to previous field
	if key.Matches(msg, keys.Component.ShiftTab) || key.Matches(msg, keys.Component.Prev) {
		m = m.prevField()
		return m, m.blinkCmd()
	}

	// Forward all other keys to the vimtextarea
	var cmd tea.Cmd
	fs.textArea, cmd = fs.textArea.Update(msg)
	return m, cmd
}

// handleMouseMsg processes mouse events for scrolling.
// calculateMaxBodyHeight returns the maximum height available for the body content.
// This accounts for title, header, buttons, and modal chrome.
func (m Model) calculateMaxBodyHeight() int {
	// Modal chrome (fixed parts that are always rendered):
	// - 2 lines for top/bottom border
	// - 1 line for title + padding
	// - 1 line for title border
	// - 2 lines for spacing after title border (blank line + first field margin)
	// - 2 lines for buttons section (spacing + button line)
	// - 1 line for closing padding
	chrome := 9

	// Add header height if present (estimate 4 lines typically)
	if m.config.HeaderContent != nil {
		chrome += 4
	}

	// Available height is window height minus chrome, with minimum of 5
	return max(m.height-chrome, 5)
}

// ensureFocusedFieldVisible scrolls the viewport to keep the focused field visible.
func (m *Model) ensureFocusedFieldVisible() {
	if m.focusedIndex < 0 {
		// On buttons - scroll to bottom
		m.bodyViewport.GotoBottom()
		return
	}

	// Estimate line position of focused field
	// Each field is approximately 5 lines (label + content + spacing)
	visibleIdx := 0
	for i := 0; i < m.focusedIndex; i++ {
		if m.isFieldVisible(i) {
			visibleIdx++
		}
	}

	// Scroll to approximately where the field should be
	targetLine := visibleIdx * 5
	if targetLine < m.bodyViewport.YOffset {
		m.bodyViewport.SetYOffset(targetLine)
	} else if targetLine >= m.bodyViewport.YOffset+m.bodyViewport.Height {
		m.bodyViewport.SetYOffset(targetLine - m.bodyViewport.Height + 5)
	}
}

// handleFieldClick checks if a field zone was clicked and focuses it.
// Returns true if a field was focused.
func (m *Model) handleFieldClick(msg tea.MouseMsg) bool {
	for i := range m.fields {
		if !m.isFieldVisible(i) {
			continue
		}

		fs := &m.fields[i]

		// Check for editable list input zone first (more specific than field zone)
		if fs.config.Type == FieldTypeEditableList {
			inputZoneID := makeFieldInputZoneID(i)
			if z := zone.Get(inputZoneID); z != nil && z.InBounds(msg) {
				// Blur current field
				m.blurCurrentField()
				// Focus this field's input sub-section
				m.focusedIndex = i
				fs.subFocus = SubFocusInput
				fs.addInput.Focus()
				return true
			}
		}

		zoneID := makeFieldZoneID(i)
		if z := zone.Get(zoneID); z != nil && z.InBounds(msg) {
			// Blur current field
			m.blurCurrentField()
			// Focus new field
			m.focusedIndex = i
			m.focusField(i)

			// Special handling for SearchSelect: toggle expanded state on click
			if fs.config.Type == FieldTypeSearchSelect {
				if fs.searchExpanded {
					// Clicking when expanded collapses it
					fs.searchExpanded = false
					fs.searchInput.Blur()
				} else {
					// Clicking when collapsed expands it
					fs.searchExpanded = true
					fs.searchInput.SetValue("")
					fs.searchInput.Focus()
					// Reset filter to show all items
					*m = m.updateSearchFilter(fs)
					// Position cursor at selected item
					for j, idx := range fs.searchFiltered {
						if fs.listItems[idx].selected {
							fs.listCursor = j
							break
						}
					}
					fs.scrollOffset = 0
					*m = m.ensureSearchCursorVisible(fs)
				}
			}
			return true
		}
	}
	return false
}

// handleItemClick checks if a list item zone was clicked and handles it.
// Returns true if an item was clicked and handled.
func (m *Model) handleItemClick(msg tea.MouseMsg) bool {
	for fieldIdx := range m.fields {
		if !m.isFieldVisible(fieldIdx) {
			continue
		}
		fs := &m.fields[fieldIdx]

		// Check list/select/editable-list items
		switch fs.config.Type {
		case FieldTypeList, FieldTypeEditableList:
			for itemIdx := range fs.listItems {
				zoneID := makeItemZoneID(fieldIdx, itemIdx)
				if z := zone.Get(zoneID); z != nil && z.InBounds(msg) {
					// Focus this field first
					m.blurCurrentField()
					m.focusedIndex = fieldIdx
					m.focusField(fieldIdx)
					// Move cursor and toggle
					fs.listCursor = itemIdx
					fs.listItems[itemIdx].selected = !fs.listItems[itemIdx].selected
					return true
				}
			}

		case FieldTypeSelect:
			for itemIdx := range fs.listItems {
				zoneID := makeItemZoneID(fieldIdx, itemIdx)
				if z := zone.Get(zoneID); z != nil && z.InBounds(msg) {
					// Focus this field first
					m.blurCurrentField()
					m.focusedIndex = fieldIdx
					m.focusField(fieldIdx)
					// Deselect all and select clicked item
					for i := range fs.listItems {
						fs.listItems[i].selected = false
					}
					fs.listCursor = itemIdx
					fs.listItems[itemIdx].selected = true
					return true
				}
			}

		case FieldTypeSearchSelect:
			// Only handle clicks when expanded
			if fs.searchExpanded {
				for itemIdx := range fs.listItems {
					// Check multiple row zones per item (label + subtext lines)
					// Format: formmodal-item-{field}-{item}-r{row}
					// We check up to 10 rows per item (should be plenty for wrapped subtext)
					for rowIdx := range 10 {
						zoneID := makeItemRowZoneID(fieldIdx, itemIdx, rowIdx)
						if z := zone.Get(zoneID); z != nil && z.InBounds(msg) {
							// Deselect all and select clicked item
							for i := range fs.listItems {
								fs.listItems[i].selected = false
							}
							fs.listItems[itemIdx].selected = true
							// Collapse the search
							fs.searchExpanded = false
							fs.searchInput.Blur()
							return true
						}
					}
				}
			}

		case FieldTypeToggle:
			// Check both toggle options (index 0 and 1)
			for optIdx := range 2 {
				zoneID := makeItemZoneID(fieldIdx, optIdx)
				if z := zone.Get(zoneID); z != nil && z.InBounds(msg) {
					// Focus this field and select the clicked option
					m.blurCurrentField()
					m.focusedIndex = fieldIdx
					m.focusField(fieldIdx)
					fs.toggleIndex = optIdx
					return true
				}
			}
		}
	}
	return false
}

// blurCurrentField removes focus from the currently focused field.
func (m *Model) blurCurrentField() {
	if m.focusedIndex < 0 || m.focusedIndex >= len(m.fields) {
		return
	}
	fs := &m.fields[m.focusedIndex]
	switch fs.config.Type {
	case FieldTypeText:
		fs.textInput.Blur()
	case FieldTypeTextArea:
		fs.textArea.Blur()
	case FieldTypeSearchSelect:
		fs.searchInput.Blur()
	case FieldTypeEditableList:
		fs.addInput.Blur()
	}
}

// focusField gives focus to the specified field.
func (m *Model) focusField(index int) {
	if index < 0 || index >= len(m.fields) {
		return
	}
	fs := &m.fields[index]
	switch fs.config.Type {
	case FieldTypeText:
		fs.textInput.Focus()
	case FieldTypeTextArea:
		fs.textArea.Focus()
	case FieldTypeSearchSelect:
		// Don't auto-expand, just focus
		if fs.searchExpanded {
			fs.searchInput.Focus()
		}
	case FieldTypeEditableList:
		// Focus list section by default
		fs.subFocus = SubFocusList
	}
}

// handleButtonClick checks if a button zone was clicked and handles it.
// Returns a tea.Cmd if a button was clicked, nil otherwise.
func (m *Model) handleButtonClick(msg tea.MouseMsg) tea.Cmd {
	// Check submit button
	if z := zone.Get(zoneSubmitButton); z != nil && z.InBounds(msg) {
		// Focus on buttons and select submit
		m.blurCurrentField()
		m.focusedIndex = -1
		m.focusedButton = 0
		// Trigger submit - must assign returned model back since submit() has value receiver
		newM, cmd := m.submit()
		*m = newM
		return cmd
	}

	// Check cancel button
	if z := zone.Get(zoneCancelButton); z != nil && z.InBounds(msg) {
		// Focus on buttons and select cancel
		m.blurCurrentField()
		m.focusedIndex = -1
		m.focusedButton = 1
		// Trigger cancel
		if m.config.OnCancel != nil {
			return func() tea.Msg { return m.config.OnCancel() }
		}
		return func() tea.Msg { return CancelMsg{} }
	}

	return nil
}
