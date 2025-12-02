// Package modal provides a reusable modal component for confirmation dialogs
// and input prompts.
package modal

import (
	"perles/internal/ui/overlay"
	"perles/internal/ui/styles"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ButtonVariant controls the styling of the confirm/save button.
type ButtonVariant int

const (
	ButtonPrimary ButtonVariant = iota // Blue (default)
	ButtonDanger                       // Red (for destructive actions)
)

// InputConfig defines a single input field.
type InputConfig struct {
	Key         string // Identifier for this input (used in SubmitMsg.Values)
	Label       string // Label displayed in the input section border
	Placeholder string // Placeholder text shown when empty
	Value       string // Initial value (optional)
	MaxLength   int    // Character limit (0 = unlimited)
}

// Config controls modal appearance and behavior.
type Config struct {
	Title          string        // Modal title (e.g., "New View", "Confirm Delete")
	Message        string        // Optional message/prompt text
	Inputs         []InputConfig // Input fields; if empty, modal is in confirmation mode
	ConfirmVariant ButtonVariant // Style for confirm button (default: ButtonPrimary)
	MinWidth       int           // Minimum width (0 = default 40)
}

// SubmitMsg is sent when the user confirms the modal (Enter on Save button).
// Values contains input values keyed by InputConfig.Key.
type SubmitMsg struct {
	Values map[string]string
}

// CancelMsg is sent when the user cancels the modal (Esc key or Cancel button).
type CancelMsg struct{}

// Field identifies which element is focused.
type Field int

const (
	FieldSave Field = iota
	FieldCancel
)

// Model is the modal component state.
type Model struct {
	config       Config
	inputs       []textinput.Model
	inputKeys    []string // Maps input index to key
	hasInputs    bool
	focusedInput int   // Which input is focused (-1 if on buttons)
	focusedField Field // Which button is focused (when focusedInput == -1)
	width        int
	height       int
}

// New creates a new modal with the given configuration.
// If Inputs is non-empty, the modal operates in input mode with text fields.
// Otherwise, it operates in confirmation mode (just confirm/cancel).
func New(cfg Config) Model {
	m := Model{
		config:       cfg,
		hasInputs:    len(cfg.Inputs) > 0,
		focusedInput: 0, // Start on first input
		focusedField: FieldSave,
	}

	if m.hasInputs {
		m.inputs = make([]textinput.Model, len(cfg.Inputs))
		m.inputKeys = make([]string, len(cfg.Inputs))

		for i, inputCfg := range cfg.Inputs {
			ti := textinput.New()
			ti.Placeholder = inputCfg.Placeholder
			ti.Width = 36 // Fits within minWidth (40) minus borders/padding
			ti.Prompt = ""
			if inputCfg.MaxLength > 0 {
				ti.CharLimit = inputCfg.MaxLength
			}
			if inputCfg.Value != "" {
				ti.SetValue(inputCfg.Value)
			}
			if i == 0 {
				ti.Focus() // Focus first input
			}
			m.inputs[i] = ti
			m.inputKeys[i] = inputCfg.Key
		}
	} else {
		// No inputs, start on Save button
		m.focusedInput = -1
	}

	return m
}

// Init returns the initial command. For input mode, starts the cursor blink.
func (m Model) Init() tea.Cmd {
	if m.hasInputs {
		return textinput.Blink
	}
	return nil
}

// Update handles messages for the modal.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "down", "ctrl+n":
			m = m.nextField()
			return m, nil

		case "shift+tab", "up", "ctrl+p":
			m = m.prevField()
			return m, nil

		case "left", "h":
			// Navigate between Save and Cancel (only when on buttons)
			if m.focusedInput == -1 && m.focusedField == FieldCancel {
				m.focusedField = FieldSave
				return m, nil
			}

		case "right", "l":
			// Navigate between Save and Cancel (only when on buttons)
			if m.focusedInput == -1 && m.focusedField == FieldSave {
				m.focusedField = FieldCancel
				return m, nil
			}

		case "enter":
			if m.focusedInput >= 0 {
				// On an input - move to next field
				m = m.nextField()
				return m, nil
			}
			// On buttons
			switch m.focusedField {
			case FieldSave:
				// Check all inputs have values (if in input mode)
				if m.hasInputs {
					allFilled := true
					for _, input := range m.inputs {
						if input.Value() == "" {
							allFilled = false
							break
						}
					}
					if !allFilled {
						return m, nil // Don't submit if any input empty
					}
				}
				// Build values map
				values := make(map[string]string)
				for i, input := range m.inputs {
					values[m.inputKeys[i]] = input.Value()
				}
				return m, func() tea.Msg { return SubmitMsg{Values: values} }
			case FieldCancel:
				return m, func() tea.Msg { return CancelMsg{} }
			}

		case "esc":
			return m, func() tea.Msg { return CancelMsg{} }
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	// Forward to focused text input
	if m.hasInputs && m.focusedInput >= 0 && m.focusedInput < len(m.inputs) {
		var cmd tea.Cmd
		m.inputs[m.focusedInput], cmd = m.inputs[m.focusedInput].Update(msg)
		return m, cmd
	}

	return m, nil
}

// nextField moves focus to the next field.
func (m Model) nextField() Model {
	if m.focusedInput >= 0 {
		// Currently on an input
		m.inputs[m.focusedInput].Blur()
		if m.focusedInput < len(m.inputs)-1 {
			// Move to next input
			m.focusedInput++
			m.inputs[m.focusedInput].Focus()
		} else {
			// Move to Save button
			m.focusedInput = -1
			m.focusedField = FieldSave
		}
	} else {
		// On buttons
		if m.focusedField == FieldSave {
			m.focusedField = FieldCancel
		} else {
			// Wrap to first input (or Save if no inputs)
			if m.hasInputs {
				m.focusedInput = 0
				m.inputs[0].Focus()
			} else {
				m.focusedField = FieldSave
			}
		}
	}
	return m
}

// prevField moves focus to the previous field.
func (m Model) prevField() Model {
	if m.focusedInput >= 0 {
		// Currently on an input
		m.inputs[m.focusedInput].Blur()
		if m.focusedInput > 0 {
			// Move to previous input
			m.focusedInput--
			m.inputs[m.focusedInput].Focus()
		} else {
			// Wrap to Cancel button
			m.focusedInput = -1
			m.focusedField = FieldCancel
		}
	} else {
		// On buttons
		if m.focusedField == FieldCancel {
			m.focusedField = FieldSave
		} else {
			// Move to last input (or Cancel if no inputs)
			if m.hasInputs {
				m.focusedInput = len(m.inputs) - 1
				m.inputs[m.focusedInput].Focus()
			} else {
				m.focusedField = FieldCancel
			}
		}
	}
	return m
}

// View renders the modal content (without overlay).
func (m Model) View() string {
	// Calculate box width (content width + padding for content area)
	minWidth := 40
	if m.config.MinWidth > minWidth {
		minWidth = m.config.MinWidth
	}
	contentWidth := minWidth
	titleLen := lipgloss.Width(m.config.Title)
	if titleLen > contentWidth {
		contentWidth = titleLen
	}
	boxWidth := contentWidth + 2 // Account for content padding

	// Title style with left padding
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.OverlayTitleColor).
		PaddingLeft(1)

	// Divider below title
	dividerStyle := lipgloss.NewStyle().
		Foreground(styles.OverlayBorderColor)
	divider := dividerStyle.Render(strings.Repeat("─", boxWidth))

	// Build content with padding
	var content strings.Builder

	// Optional message
	if m.config.Message != "" {
		msgStyle := lipgloss.NewStyle().
			Foreground(styles.TextPrimaryColor).
			Width(contentWidth)
		content.WriteString(msgStyle.Render(m.config.Message))
		content.WriteString("\n\n")
	}

	// Input fields in bordered sections
	for i, inputCfg := range m.config.Inputs {
		content.WriteString(m.renderInputSection(i, inputCfg.Label, contentWidth))
		content.WriteString("\n\n")
	}

	// Buttons
	content.WriteString(m.renderButtons())

	// Build final layout: title (flush), divider, padded content
	var result strings.Builder
	result.WriteString(titleStyle.Render(m.config.Title))
	result.WriteString("\n")
	result.WriteString(divider)
	result.WriteString("\n")
	// Add padding to content area only
	contentStyle := lipgloss.NewStyle().Padding(1, 1)
	result.WriteString(contentStyle.Render(content.String()))

	// Wrap in bordered box with explicit width (like picker)
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.OverlayBorderColor).
		Width(boxWidth)

	return boxStyle.Render(result.String())
}

// renderInputSection renders an input field wrapped in a bordered section.
func (m Model) renderInputSection(index int, label string, width int) string {
	if label == "" {
		label = "Input"
	}

	// Determine if this input is focused
	isFocused := m.focusedInput == index

	inputView := m.inputs[index].View()
	return styles.RenderFormSection([]string{inputView}, label, "", width, isFocused, styles.BorderHighlightFocusColor)
}

// renderButtons renders Save and Cancel buttons styled like coleditor.
func (m Model) renderButtons() string {
	// Determine if on buttons
	onButtons := m.focusedInput == -1

	// Select button style based on variant
	var saveStyle lipgloss.Style
	switch m.config.ConfirmVariant {
	case ButtonDanger:
		saveStyle = styles.DangerButtonStyle
		if onButtons && m.focusedField == FieldSave {
			saveStyle = styles.DangerButtonFocusedStyle
		}
	default: // ButtonPrimary
		saveStyle = styles.PrimaryButtonStyle
		if onButtons && m.focusedField == FieldSave {
			saveStyle = styles.PrimaryButtonFocusedStyle
		}
	}

	var saveLabel string
	if m.hasInputs {
		saveLabel = "Save"
	} else {
		saveLabel = "Confirm"
	}
	saveBtn := saveStyle.Render(saveLabel)

	// Cancel button - dark grey, lighter when focused
	cancelStyle := styles.SecondaryButtonStyle
	if onButtons && m.focusedField == FieldCancel {
		cancelStyle = styles.SecondaryButtonFocusedStyle
	}
	cancelBtn := cancelStyle.Render("Cancel")

	return saveBtn + "  " + cancelBtn
}

// Overlay renders the modal centered on the given background.
func (m Model) Overlay(bg string) string {
	fg := m.View()
	return overlay.Place(overlay.Config{
		Width:    m.width,
		Height:   m.height,
		Position: overlay.Center,
	}, fg, bg)
}

// SetSize updates the modal's knowledge of viewport size for overlay centering.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// FocusedInput returns the currently focused input index (-1 if on buttons).
func (m Model) FocusedInput() int {
	return m.focusedInput
}

// FocusedField returns the currently focused button field.
func (m Model) FocusedField() Field {
	return m.focusedField
}
