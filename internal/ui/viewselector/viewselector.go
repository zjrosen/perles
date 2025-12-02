// Package viewselector provides a modal for selecting views to add a column to.
package viewselector

import (
	"perles/internal/config"
	"perles/internal/ui/colorpicker"
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
	FieldColumnName Field = iota
	FieldColor
	FieldViewList
	FieldSave
)

// viewItem represents a view with its selection state.
type viewItem struct {
	name     string
	index    int // Index in config.Views
	selected bool
}

// Model holds the view selector state.
type Model struct {
	query           string
	columnName      textinput.Model
	colorPicker     colorpicker.Model
	selectedColor   colorpicker.PresetColor
	showColorPicker bool
	views           []viewItem
	selectedView    int // Index of highlighted view in list
	focusedField    Field
	width           int
	height          int
	saveError       string // Error message shown above save button
}

// SaveMsg is sent when the user confirms the save.
type SaveMsg struct {
	ColumnName  string
	Color       string
	Query       string
	ViewIndices []int // Indices of views to add the column to
}

// CancelMsg is sent when the user cancels.
type CancelMsg struct{}

// New creates a new view selector modal.
func New(query string, views []config.ViewConfig) Model {
	// Column name input
	nameInput := textinput.New()
	nameInput.Placeholder = "Enter column name..."
	nameInput.Width = 30
	nameInput.Prompt = ""
	nameInput.Focus()

	// Initialize color picker with default selection (Green)
	cp := colorpicker.New().SetSelected("#73F59F")

	// Convert views to selectable items (none selected by default)
	items := make([]viewItem, len(views))
	for i, v := range views {
		items[i] = viewItem{
			name:     v.Name,
			index:    i,
			selected: false,
		}
	}

	return Model{
		query:         query,
		columnName:    nameInput,
		colorPicker:   cp,
		selectedColor: colorpicker.PresetColor{Name: "Green", Hex: "#73F59F"},
		views:         items,
		focusedField:  FieldColumnName,
	}
}

// SetSize sets the viewport dimensions for overlay rendering.
func (m Model) SetSize(width, height int) Model {
	m.width = width
	m.height = height
	return m
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	// Handle colorpicker result messages (these come back from commands)
	switch msg := msg.(type) {
	case colorpicker.SelectMsg:
		m.showColorPicker = false
		m.selectedColor = colorpicker.PresetColor{
			Name: m.colorPicker.Selected().Name,
			Hex:  msg.Hex,
		}
		return m, nil

	case colorpicker.CancelMsg:
		m.showColorPicker = false
		return m, nil
	}

	// Handle color picker overlay if open
	if m.showColorPicker {
		return m.updateColorPicker(msg)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return CancelMsg{} }

		case "tab", "ctrl+n":
			// ctrl+n navigates within view list, then cycles to next field at boundary
			if msg.String() == "ctrl+n" && m.focusedField == FieldViewList && len(m.views) > 0 {
				if m.selectedView < len(m.views)-1 {
					m.selectedView++
					return m, nil
				}
				// At last item, fall through to cycle to Save
			}
			m = m.cycleField(false)
			return m, nil

		case "shift+tab", "ctrl+p":
			// ctrl+p navigates within view list, then cycles to prev field at boundary
			if msg.String() == "ctrl+p" && m.focusedField == FieldViewList && len(m.views) > 0 {
				if m.selectedView > 0 {
					m.selectedView--
					return m, nil
				}
				// At first item, fall through to cycle to Color
			}
			m = m.cycleField(true)
			return m, nil

		case " ":
			// Toggle view selection (only in view list)
			if m.focusedField == FieldViewList && len(m.views) > 0 {
				m.views[m.selectedView].selected = !m.views[m.selectedView].selected
				return m, nil
			}

		case "enter":
			switch m.focusedField {
			case FieldSave:
				return m.save()
			case FieldColor:
				// Open color picker overlay
				m.showColorPicker = true
				m.colorPicker = m.colorPicker.SetSelected(m.selectedColor.Hex).SetSize(m.width, m.height)
				return m, nil
			default:
				// Move to next field on enter in other fields
				m = m.cycleField(false)
				return m, nil
			}

		case "j", "down":
			if m.focusedField == FieldViewList {
				if m.selectedView < len(m.views)-1 {
					m.selectedView++
				}
				return m, nil
			}

		case "k", "up":
			if m.focusedField == FieldViewList {
				if m.selectedView > 0 {
					m.selectedView--
				}
				return m, nil
			}
		}
	}

	// Forward to column name input when focused
	if m.focusedField == FieldColumnName {
		var cmd tea.Cmd
		m.columnName, cmd = m.columnName.Update(msg)
		return m, cmd
	}

	return m, nil
}

// updateColorPicker handles input when color picker overlay is open.
func (m Model) updateColorPicker(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.colorPicker, cmd = m.colorPicker.Update(msg)
	return m, cmd
}

// cycleField moves focus to the next/previous field.
func (m Model) cycleField(reverse bool) Model {
	fields := []Field{FieldColumnName, FieldColor, FieldViewList, FieldSave}
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

	// Reset view list selection when entering the field
	if m.focusedField == FieldViewList && len(m.views) > 0 {
		if reverse {
			// Coming from Save (backward): start at last item
			m.selectedView = len(m.views) - 1
		} else {
			// Coming from Color (forward): start at first item
			m.selectedView = 0
		}
	}

	// Update text input focus state
	if m.focusedField == FieldColumnName {
		m.columnName.Focus()
	} else {
		m.columnName.Blur()
	}

	return m
}

// save validates input and returns a SaveMsg command.
func (m Model) save() (Model, tea.Cmd) {
	// Clear previous error
	m.saveError = ""

	// Validate column name
	name := strings.TrimSpace(m.columnName.Value())
	if name == "" {
		m.saveError = "Column name is required"
		return m, nil
	}

	// Collect selected view indices
	var indices []int
	for _, v := range m.views {
		if v.selected {
			indices = append(indices, v.index)
		}
	}

	// Must select at least one view
	if len(indices) == 0 {
		m.saveError = "Select at least one view"
		return m, nil
	}

	return m, func() tea.Msg {
		return SaveMsg{
			ColumnName:  name,
			Color:       m.selectedColor.Hex,
			Query:       m.query,
			ViewIndices: indices,
		}
	}
}

// View renders the view selector modal.
func (m Model) View() string {
	width := 50
	sectionWidth := width - 2

	// Column name input section
	nameRows := []string{" " + m.columnName.View()}
	nameSection := styles.RenderFormSection(nameRows, "Column Name", "required", sectionWidth, m.focusedField == FieldColumnName, styles.BorderHighlightFocusColor)

	// Color section - shows selected color with swatch
	swatch := lipgloss.NewStyle().
		Background(lipgloss.Color(m.selectedColor.Hex)).
		Render("  ")
	colorRow := " " + swatch + " " + m.selectedColor.Hex
	colorSection := styles.RenderFormSection([]string{colorRow}, "Color", "Enter to change", sectionWidth, m.focusedField == FieldColor, styles.BorderHighlightFocusColor)

	// View list section
	var viewRows []string
	if len(m.views) == 0 {
		emptyStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)
		viewRows = append(viewRows, emptyStyle.Render(" (no views)"))
	} else {
		for i, v := range m.views {
			prefix := " "
			if m.focusedField == FieldViewList && i == m.selectedView {
				prefix = styles.SelectionIndicatorStyle.Render(">")
			}
			checkbox := "[ ]"
			if v.selected {
				checkbox = "[x]"
			}
			viewRows = append(viewRows, prefix+checkbox+" "+v.name)
		}
	}
	viewSection := styles.RenderFormSection(viewRows, "Add to Views", "Space to toggle", sectionWidth, m.focusedField == FieldViewList, styles.BorderHighlightFocusColor)

	// Error message (if any)
	errorLine := ""
	if m.saveError != "" {
		errorStyle := lipgloss.NewStyle().Foreground(styles.StatusErrorColor)
		errorLine = errorStyle.Render(m.saveError)
	}

	// Save button
	saveStyle := styles.PrimaryButtonStyle
	if m.focusedField == FieldSave {
		saveStyle = styles.PrimaryButtonFocusedStyle
	}
	saveButton := saveStyle.Render(" Save ")

	// Title with bottom border (full width, edge-to-edge)
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.OverlayTitleColor)
	borderStyle := lipgloss.NewStyle().Foreground(styles.BorderDefaultColor)
	titleBorder := borderStyle.Render(strings.Repeat("─", width))

	// Content style adds horizontal padding to everything except the border
	contentPadding := lipgloss.NewStyle().PaddingLeft(1)

	// Build content
	content := contentPadding.Render(titleStyle.Render("Save as Column")) + "\n" +
		titleBorder + "\n\n" +
		contentPadding.Render(nameSection) + "\n\n" +
		contentPadding.Render(colorSection) + "\n\n" +
		contentPadding.Render(viewSection) + "\n"

	if errorLine != "" {
		content += contentPadding.Render(" "+errorLine) + "\n\n"
	} else {
		content += "\n"
	}

	content += contentPadding.Render(" "+saveButton) + "\n"

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.OverlayBorderColor).
		Width(width)

	return boxStyle.Render(content)
}

// Overlay renders the view selector on top of a background view.
func (m Model) Overlay(background string) string {
	modalView := m.View()

	if background == "" {
		return lipgloss.Place(
			m.width, m.height,
			lipgloss.Center, lipgloss.Center,
			modalView,
		)
	}

	// Place main modal
	result := overlay.Place(overlay.Config{
		Width:    m.width,
		Height:   m.height,
		Position: overlay.Center,
	}, modalView, background)

	// If color picker is open, overlay it on top
	if m.showColorPicker {
		result = m.colorPicker.Overlay(result)
	}

	return result
}
