// Package formmodal provides a configuration-driven form modal component.
//
// Use formmodal when your modal needs:
//   - Multiple field types (text, color, list)
//   - Consistent keyboard navigation
//   - Built-in validation support
//   - Tab/Shift+Tab to cycle through fields
//   - Color picker overlay integration
//
// For simple confirmation dialogs, use shared/modal instead.
//
// Quick Start:
//
//	cfg := formmodal.FormConfig{
//	    Title: "Create Item",
//	    Fields: []formmodal.FieldConfig{
//	        {Key: "name", Type: formmodal.FieldTypeText, Label: "Name", Hint: "required"},
//	        {Key: "color", Type: formmodal.FieldTypeColor, Label: "Color"},
//	    },
//	    SubmitLabel: "Create",
//	    Validate: func(values map[string]any) error {
//	        if values["name"].(string) == "" {
//	            return errors.New("name is required")
//	        }
//	        return nil
//	    },
//	}
//	m := formmodal.New(cfg)
//
// Keyboard Navigation:
//
//	Tab, Ctrl+N      - Next field/button
//	Shift+Tab, Ctrl+P - Previous field/button
//	Enter            - Confirm (submit on button, open picker on color)
//	Esc              - Cancel modal
//	j/k              - Navigate within list fields
//	Space            - Toggle selection in list fields
//	h/l              - Navigate between buttons
//
// Message Flow:
//
// When the form is submitted successfully, formmodal sends a SubmitMsg containing
// a Values map with all field values keyed by FieldConfig.Key. When cancelled,
// it sends a CancelMsg.
//
// Wrapper Pattern (using message factories):
//
// To integrate formmodal into an existing modal component while preserving its API,
// use OnSubmit and OnCancel factories to produce your custom message types directly:
//
//	type Model struct {
//	    issueID string
//	    form    formmodal.Model
//	}
//
//	func New(issueID string) Model {
//	    m := Model{issueID: issueID}
//	    cfg := formmodal.FormConfig{
//	        Title: "Edit Item",
//	        Fields: []formmodal.FieldConfig{...},
//	        OnSubmit: func(values map[string]any) tea.Msg {
//	            return YourSaveMsg{ID: m.issueID, Data: values["data"].(string)}
//	        },
//	        OnCancel: func() tea.Msg { return YourCancelMsg{} },
//	    }
//	    m.form = formmodal.New(cfg)
//	    return m
//	}
//
//	func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
//	    var cmd tea.Cmd
//	    m.form, cmd = m.form.Update(msg)
//	    return m, cmd // formmodal produces YourSaveMsg/YourCancelMsg directly
//	}
//
// The factories eliminate command wrapping boilerplate. If OnSubmit/OnCancel
// are nil, formmodal produces the default SubmitMsg/CancelMsg types.
package formmodal

import (
	"perles/internal/ui/shared/modal"

	tea "github.com/charmbracelet/bubbletea"
)

// FieldType identifies the type of form field.
type FieldType int

const (
	// FieldTypeText is a single-line text input field.
	// Uses the textinput bubble for handling input.
	// Supports Placeholder, MaxLength, and InitialValue options.
	FieldTypeText FieldType = iota

	// FieldTypeColor shows a color swatch with hex value.
	// Press Enter to open the colorpicker overlay.
	// Supports InitialColor option (default: "#73F59F").
	FieldTypeColor

	// FieldTypeList is a checkable list with multi-select support.
	// Navigate with j/k, toggle with Space.
	// Supports Options and MultiSelect options.
	FieldTypeList

	// FieldTypeSelect is a single-select list (radio button style).
	// Navigate with j/k, selecting automatically deselects others.
	// Supports Options option (MultiSelect is ignored).
	FieldTypeSelect

	// FieldTypeEditableList is a list with an embedded input for adding items.
	// Navigate with j/k within the list, Tab between list and input.
	// Supports Options, MultiSelect, InputPlaceholder, InputHint, InputLabel, AllowDuplicates.
	FieldTypeEditableList

	// FieldTypeToggle is a binary toggle selector (radio button style).
	// Navigate between options with h/l or left/right keys.
	// Requires exactly 2 Options. Returns the selected option's Value.
	// Supports Options, InitialToggleIndex (0 or 1).
	// Visual pattern: ● Selected    ○ Unselected [←/→]
	FieldTypeToggle
)

// FieldConfig defines a single form field.
//
// Common fields for all types:
//   - Key: Unique identifier used as the map key in SubmitMsg.Values
//   - Type: One of FieldTypeText, FieldTypeColor, FieldTypeList, FieldTypeSelect, FieldTypeEditableList
//   - Label: Section header displayed above the field
//   - Hint: Displayed next to label (e.g., "required", "optional")
//
// Text field options (FieldTypeText):
//   - Placeholder: Gray text shown when empty
//   - MaxLength: Character limit (0 = unlimited)
//   - InitialValue: Pre-filled text value
//
// Color field options (FieldTypeColor):
//   - InitialColor: Starting hex color (default: "#73F59F")
//
// List field options (FieldTypeList, FieldTypeSelect):
//   - Options: Slice of ListOption defining available choices
//   - MultiSelect: If true, allows multiple selections (FieldTypeList only)
//
// EditableList field options (FieldTypeEditableList):
//   - Options: Slice of ListOption defining initial list items
//   - MultiSelect: If true, allows multiple selections
//   - InputPlaceholder: Placeholder for the add-item input
//   - InputHint: Hint shown for the input section (e.g., "Enter to add")
//   - InputLabel: Label for the input section (e.g., "Add Label")
//   - AllowDuplicates: Whether duplicate values are allowed (default: false)
type FieldConfig struct {
	Key   string    // Unique identifier for this field (used in SubmitMsg.Values)
	Type  FieldType // Type of field
	Label string    // Section label (e.g., "View Name")
	Hint  string    // Section hint (e.g., "required", "optional")

	// Text field options
	Placeholder  string // Placeholder text for text inputs
	MaxLength    int    // Character limit (0 = unlimited)
	InitialValue string // Pre-filled value

	// Color field options
	InitialColor string // Initial hex color (default: "#73F59F")

	// List/Select field options
	Options     []ListOption // Available options for list/select fields
	MultiSelect bool         // For FieldTypeList: allow multiple selections

	// EditableList field options (FieldTypeEditableList)
	InputPlaceholder string // Placeholder for the add-item input
	InputHint        string // Hint shown below input (e.g., "Enter to add")
	InputLabel       string // Label for input section (e.g., "Add Label")
	AllowDuplicates  bool   // Whether duplicate values are allowed (default: false)

	// Toggle field options (FieldTypeToggle)
	InitialToggleIndex int // 0 or 1 - which option is initially selected (default: 0)
}

// ListOption represents an item in a list or select field.
//
// Label is displayed to the user, Value is returned in SubmitMsg.Values.
// For FieldTypeList with MultiSelect=true, Selected sets initial state.
type ListOption struct {
	Label    string // Display text
	Value    string // Programmatic value (returned in SubmitMsg)
	Selected bool   // Initially selected (for multi-select lists)
}

// FormConfig defines the complete form modal configuration.
//
// Example:
//
//	cfg := FormConfig{
//	    Title:       "Save as Column",
//	    Fields:      []FieldConfig{...},
//	    SubmitLabel: " Save ",
//	    MinWidth:    50,
//	    Validate: func(values map[string]any) error {
//	        name := values["name"].(string)
//	        if name == "" {
//	            return errors.New("name is required")
//	        }
//	        return nil
//	    },
//	}
//
// The Validate function receives all field values and should return an error
// if validation fails. The error message is displayed above the buttons.
type FormConfig struct {
	Title         string                     // Modal title displayed at top
	Fields        []FieldConfig              // Form fields in display order
	SubmitLabel   string                     // Submit button label (default: "Save")
	SubmitVariant modal.ButtonVariant        // Primary or Danger button style
	CancelLabel   string                     // Cancel button label (default: "Cancel")
	MinWidth      int                        // Minimum modal width (default: 50)
	Validate      func(map[string]any) error // Validation function (optional)

	// OnSubmit produces a custom message when the form is submitted.
	// If nil, formmodal produces SubmitMsg{Values: values}.
	// Example: func(values map[string]any) tea.Msg { return MySubmitMsg{...} }
	OnSubmit func(values map[string]any) tea.Msg

	// OnCancel produces a custom message when the form is cancelled.
	// If nil, formmodal produces CancelMsg{}.
	// Example: func() tea.Msg { return MyCancelMsg{} }
	OnCancel func() tea.Msg
}
