// Package labeleditor provides a modal for editing issue labels.
package labeleditor

import (
	"perles/internal/ui/overlay"
	"perles/internal/ui/styles"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Field identifies which element is focused.
type Field int

const (
	FieldLabelList Field = iota
	FieldInput
	FieldDone
)

// labelItem represents a label with its enabled state.
type labelItem struct {
	name    string
	enabled bool
}

// Model holds the label editor state.
type Model struct {
	issueID        string
	labels         []labelItem // Labels with enabled/disabled state
	originalLabels []string    // Original labels for comparison
	selectedLabel  int         // Index of selected label in list
	input          textinput.Model
	focusedField   Field
	width          int
	height         int
}

// SaveMsg is sent when the user confirms label changes.
type SaveMsg struct {
	IssueID string
	Labels  []string // Final label set to persist
}

// CancelMsg is sent when the user cancels the editor.
type CancelMsg struct{}

// New creates a new label editor.
func New(issueID string, currentLabels []string) Model {
	ti := textinput.New()
	ti.Placeholder = "Enter label name..."
	ti.Width = 30
	ti.Prompt = ""

	// Convert to labelItems with enabled=true
	items := make([]labelItem, len(currentLabels))
	for i, label := range currentLabels {
		items[i] = labelItem{name: label, enabled: true}
	}

	return Model{
		issueID:        issueID,
		labels:         items,
		originalLabels: append([]string{}, currentLabels...), // For comparison
		selectedLabel:  0,
		input:          ti,
		focusedField:   FieldLabelList,
	}
}

// SetSize sets the viewport dimensions for overlay rendering.
func (m Model) SetSize(width, height int) Model {
	m.width = width
	m.height = height
	return m
}

// Init initializes the model.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			// Cancel - discard changes
			return m, func() tea.Msg { return CancelMsg{} }

		case "tab":
			m = m.cycleField(false)
			return m, nil

		case "shift+tab":
			m = m.cycleField(true)
			return m, nil

		case "j", "down", "ctrl+n":
			if m.focusedField == FieldInput && msg.String() == "j" {
				// Let j fall through to text input (but down/ctrl+n navigate)
			} else if m.focusedField == FieldLabelList {
				if len(m.labels) > 0 && m.selectedLabel < len(m.labels)-1 {
					// Move down within label list
					m.selectedLabel++
				} else {
					// At bottom of list or empty list, move to next field
					m = m.cycleField(false)
				}
				return m, nil
			} else {
				// In Input/Save fields, move to next field
				m = m.cycleField(false)
				return m, nil
			}

		case "k", "up", "ctrl+p":
			if m.focusedField == FieldInput && msg.String() == "k" {
				// Let k fall through to text input (but up/ctrl+p navigate)
			} else if m.focusedField == FieldLabelList {
				if len(m.labels) > 0 && m.selectedLabel > 0 {
					// Move up within label list
					m.selectedLabel--
				} else {
					// At top of list or empty list, move to previous field
					m = m.cycleField(true)
				}
				return m, nil
			} else {
				// In Input/Save fields, move to previous field
				m = m.cycleField(true)
				return m, nil
			}

		case " ":
			// Toggle selected label enabled state (only in label list)
			if m.focusedField == FieldLabelList && len(m.labels) > 0 {
				m.labels[m.selectedLabel].enabled = !m.labels[m.selectedLabel].enabled
				return m, nil
			}
			// Otherwise fall through to let text input handle space

		case "enter":
			switch m.focusedField {
			case FieldLabelList:
				// Enter on label list toggles the label
				if len(m.labels) > 0 {
					m.labels[m.selectedLabel].enabled = !m.labels[m.selectedLabel].enabled
				}
			case FieldInput:
				// Add label to working list (not persisted yet)
				label := strings.TrimSpace(m.input.Value())
				if label != "" && !m.containsLabel(label) {
					m.labels = append(m.labels, labelItem{name: label, enabled: true})
					m.input.SetValue("")
				}
			case FieldDone:
				// Save and close - persist only enabled labels
				enabledLabels := m.enabledLabels()
				return m, func() tea.Msg {
					return SaveMsg{IssueID: m.issueID, Labels: enabledLabels}
				}
			}
			return m, nil
		}
	}

	// Forward to text input when focused
	if m.focusedField == FieldInput {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	return m, nil
}

// cycleField moves focus to the next/previous field.
func (m Model) cycleField(reverse bool) Model {
	fields := []Field{FieldLabelList, FieldInput, FieldDone}
	current := 0
	for i, f := range fields {
		if f == m.focusedField {
			current = i
			break
		}
	}

	if reverse {
		current--
		if current < 0 {
			current = len(fields) - 1
		}
	} else {
		current = (current + 1) % len(fields)
	}

	m.focusedField = fields[current]

	// Reset selectedLabel when entering label list
	if m.focusedField == FieldLabelList && len(m.labels) > 0 {
		if reverse {
			// Coming from Input, start at last label
			m.selectedLabel = len(m.labels) - 1
		} else {
			// Coming from Done (wrapping), start at first label
			m.selectedLabel = 0
		}
	}

	// Update text input focus state
	if m.focusedField == FieldInput {
		m.input.Focus()
	} else {
		m.input.Blur()
	}

	return m
}

// View renders the label editor modal.
func (m Model) View() string {
	width := 42
	sectionWidth := width - 2 // Section width with padding

	// Build label rows for Labels section
	var labelRows []string
	if len(m.labels) == 0 {
		emptyStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)
		labelRows = append(labelRows, emptyStyle.Render("  (no labels)"))
	} else {
		for i, item := range m.labels {
			prefix := "  "
			if m.focusedField == FieldLabelList && i == m.selectedLabel {
				prefix = "> "
			}
			// Show [x] for enabled, [ ] for disabled
			checkbox := "[x]"
			if !item.enabled {
				checkbox = "[ ]"
			}
			labelRows = append(labelRows, prefix+checkbox+" "+item.name)
		}
	}

	// Build input row for Add Label section
	inputPrefix := "  "
	if m.focusedField == FieldInput {
		inputPrefix = "> "
	}
	inputRows := []string{
		inputPrefix + m.input.View(),
	}

	// Save button
	saveStyle := styles.PrimaryButtonStyle
	if m.focusedField == FieldDone {
		saveStyle = styles.PrimaryButtonFocusedStyle
	}
	saveButton := saveStyle.Render(" Save ")

	// Build sections
	labelsSection := styles.RenderFormSection(labelRows, "Labels", "Space to toggle", sectionWidth, m.focusedField == FieldLabelList, styles.BorderHighlightFocusColor)
	addSection := styles.RenderFormSection(inputRows, "Add Label", "Enter to add", sectionWidth, m.focusedField == FieldInput, styles.BorderHighlightFocusColor)

	// Title with bottom border (full width, edge-to-edge)
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.OverlayTitleColor)
	borderStyle := lipgloss.NewStyle().Foreground(styles.BorderDefaultColor)
	titleBorder := borderStyle.Render(strings.Repeat("─", width)) // Full content width

	// Content style adds horizontal padding to everything except the border
	contentPadding := lipgloss.NewStyle().PaddingLeft(1)

	// Build content
	content := contentPadding.Render(titleStyle.Render("Edit Labels")) + "\n" +
		titleBorder + "\n\n" +
		contentPadding.Render(labelsSection) + "\n\n" +
		contentPadding.Render(addSection) + "\n\n" +
		contentPadding.Render(" "+saveButton) + "\n"

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.OverlayBorderColor).
		Width(width)

	return boxStyle.Render(content)
}

// Overlay renders the label editor on top of a background view.
func (m Model) Overlay(background string) string {
	editorBox := m.View()

	if background == "" {
		return lipgloss.Place(
			m.width, m.height,
			lipgloss.Center, lipgloss.Center,
			editorBox,
		)
	}

	return overlay.Place(overlay.Config{
		Width:    m.width,
		Height:   m.height,
		Position: overlay.Center,
	}, editorBox, background)
}

// containsLabel checks if any label (enabled or not) has the given name.
func (m Model) containsLabel(name string) bool {
	for _, item := range m.labels {
		if item.name == name {
			return true
		}
	}
	return false
}

// enabledLabels returns a slice of only the enabled label names.
func (m Model) enabledLabels() []string {
	var result []string
	for _, item := range m.labels {
		if item.enabled {
			result = append(result, item.name)
		}
	}
	return result
}
