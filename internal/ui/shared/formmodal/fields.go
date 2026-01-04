package formmodal

import (
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
)

// subFocus tracks which part of a composite field has focus.
// Used by FieldTypeEditableList to track focus between list and input sections.
type subFocus int

const (
	// SubFocusList indicates focus is on the list portion of a composite field.
	SubFocusList subFocus = iota
	// SubFocusInput indicates focus is on the input portion of a composite field.
	SubFocusInput
)

// fieldState holds runtime state for a field.
type fieldState struct {
	config FieldConfig // Original configuration

	// Text field state
	textInput textinput.Model

	// Color field state
	selectedColor string // Current hex color

	// List field state
	listCursor int        // Cursor position within list
	listItems  []listItem // Items with selection state

	// EditableList field state
	addInput textinput.Model // Input for adding new items
	subFocus subFocus        // Which part of composite field has focus

	// Toggle field state
	toggleIndex int // 0 or 1 - which option is currently selected

	// SearchSelect field state
	searchInput    textinput.Model // Search/filter input
	searchFiltered []int           // Indices into listItems matching search
	scrollOffset   int             // First visible item for scrolling
	searchExpanded bool            // Whether search list is expanded (vs showing selected value)
}

// listItem tracks selection state for list items.
type listItem struct {
	label    string
	value    string
	selected bool
	color    lipgloss.TerminalColor // Optional color for the label
}

// newFieldState creates a fieldState from a FieldConfig.
func newFieldState(cfg FieldConfig) fieldState {
	fs := fieldState{
		config: cfg,
	}

	switch cfg.Type {
	case FieldTypeText:
		ti := textinput.New()
		ti.Placeholder = cfg.Placeholder
		ti.Prompt = ""
		if cfg.MaxLength > 0 {
			ti.CharLimit = cfg.MaxLength
		}
		if cfg.InitialValue != "" {
			ti.SetValue(cfg.InitialValue)
		}
		ti.Width = 36 // Default width, fits in 50-wide modal
		fs.textInput = ti

	case FieldTypeColor:
		fs.selectedColor = cfg.InitialColor
		if fs.selectedColor == "" {
			fs.selectedColor = "#73F59F" // Default green
		}

	case FieldTypeList, FieldTypeSelect:
		fs.listItems = make([]listItem, len(cfg.Options))
		for i, opt := range cfg.Options {
			fs.listItems[i] = listItem{
				label:    opt.Label,
				value:    opt.Value,
				selected: opt.Selected,
				color:    opt.Color,
			}
			// For FieldTypeSelect, initialize cursor to the selected option
			if cfg.Type == FieldTypeSelect && opt.Selected {
				fs.listCursor = i
			}
		}

	case FieldTypeEditableList:
		// Initialize list items from options (same as FieldTypeList)
		fs.listItems = make([]listItem, len(cfg.Options))
		for i, opt := range cfg.Options {
			fs.listItems[i] = listItem{
				label:    opt.Label,
				value:    opt.Value,
				selected: opt.Selected,
			}
		}
		// Initialize the add-item input
		ti := textinput.New()
		ti.Placeholder = cfg.InputPlaceholder
		ti.Prompt = ""
		ti.CharLimit = 100 // Reasonable default for labels/tags
		ti.Width = 36      // Match text field width
		fs.addInput = ti
		// Start with focus on the list
		fs.subFocus = SubFocusList

	case FieldTypeToggle:
		// Initialize toggle with the configured initial index (default: 0)
		fs.toggleIndex = cfg.InitialToggleIndex
		// Clamp to valid range [0, 1]
		if fs.toggleIndex < 0 {
			fs.toggleIndex = 0
		} else if fs.toggleIndex > 1 {
			fs.toggleIndex = 1
		}

	case FieldTypeSearchSelect:
		// Initialize list items from options
		fs.listItems = make([]listItem, len(cfg.Options))
		selectedIdx := -1
		for i, opt := range cfg.Options {
			fs.listItems[i] = listItem{
				label:    opt.Label,
				value:    opt.Value,
				selected: opt.Selected,
				color:    opt.Color,
			}
			if opt.Selected {
				selectedIdx = i
			}
		}

		// Initialize search input
		ti := textinput.New()
		ti.Placeholder = cfg.SearchPlaceholder
		if ti.Placeholder == "" {
			ti.Placeholder = "Search..."
		}
		ti.Prompt = ""
		ti.Width = 36
		fs.searchInput = ti

		// Initialize filtered list to show all items
		fs.searchFiltered = make([]int, len(cfg.Options))
		for i := range cfg.Options {
			fs.searchFiltered[i] = i
		}

		// Position cursor at the selected item if any
		if selectedIdx >= 0 {
			fs.listCursor = selectedIdx
		}
	}

	return fs
}

// value extracts the current value from the field state.
func (fs *fieldState) value() any {
	switch fs.config.Type {
	case FieldTypeText:
		return fs.textInput.Value()

	case FieldTypeColor:
		return fs.selectedColor

	case FieldTypeList:
		// Return slice of selected values
		var selected []string
		for _, item := range fs.listItems {
			if item.selected {
				selected = append(selected, item.value)
			}
		}
		return selected

	case FieldTypeSelect:
		// Return the selected item's value (not cursor position)
		for _, item := range fs.listItems {
			if item.selected {
				return item.value
			}
		}
		return ""

	case FieldTypeEditableList:
		// Return slice of selected values (same as FieldTypeList)
		var selected []string
		for _, item := range fs.listItems {
			if item.selected {
				selected = append(selected, item.value)
			}
		}
		return selected

	case FieldTypeToggle:
		// Return the value of the selected option
		if fs.toggleIndex >= 0 && fs.toggleIndex < len(fs.config.Options) {
			return fs.config.Options[fs.toggleIndex].Value
		}
		return ""

	case FieldTypeSearchSelect:
		// Return the selected item's value (same as FieldTypeSelect)
		for _, item := range fs.listItems {
			if item.selected {
				return item.value
			}
		}
		return ""
	}
	return nil
}
