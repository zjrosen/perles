// Package newviewmodal provides a modal for creating a new view from a search query.
package newviewmodal

import (
	"fmt"
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
	FieldViewName Field = iota
	FieldColumnName
	FieldColor
	FieldSave
)

// SaveMsg is sent when the user confirms.
type SaveMsg struct {
	ViewName   string
	ColumnName string
	Color      string
	Query      string
}

// CancelMsg is sent when the user cancels.
type CancelMsg struct{}

// Model holds the modal state.
type Model struct {
	query           string
	viewNameInput   textinput.Model
	columnNameInput textinput.Model
	colorPicker     colorpicker.Model
	selectedColor   colorpicker.PresetColor
	showColorPicker bool
	focused         Field
	width           int
	height          int
	saveError       string
	existingViews   []config.ViewConfig // For duplicate name validation
}

// New creates a new view modal.
func New(query string) Model {
	viewInput := textinput.New()
	viewInput.Placeholder = "View name"
	viewInput.CharLimit = 50
	viewInput.Width = 30
	viewInput.Prompt = ""
	viewInput.Focus()

	colInput := textinput.New()
	colInput.Placeholder = "defaults to view name"
	colInput.CharLimit = 30
	colInput.Width = 30
	colInput.Prompt = ""

	// Initialize color picker with default selection (Green)
	cp := colorpicker.New().SetSelected("#73F59F")

	return Model{
		query:           query,
		viewNameInput:   viewInput,
		columnNameInput: colInput,
		colorPicker:     cp,
		selectedColor:   colorpicker.PresetColor{Name: "Green", Hex: "#73F59F"},
		focused:         FieldViewName,
	}
}

// SetSize sets the viewport dimensions for overlay rendering.
func (m Model) SetSize(width, height int) Model {
	m.width = width
	m.height = height
	return m
}

// SetExistingViews sets the list of existing views for duplicate name validation.
func (m Model) SetExistingViews(views []config.ViewConfig) Model {
	m.existingViews = views
	return m
}

// Focused returns the currently focused field.
func (m Model) Focused() Field {
	return m.focused
}

// ViewName returns the current view name input value.
func (m Model) ViewName() string {
	return m.viewNameInput.Value()
}

// ColumnName returns the current column name input value.
func (m Model) ColumnName() string {
	return m.columnNameInput.Value()
}

// SelectedColor returns the currently selected color.
func (m Model) SelectedColor() colorpicker.PresetColor {
	return m.selectedColor
}

// HasError returns whether there is a validation error.
func (m Model) HasError() bool {
	return m.saveError != ""
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	// Handle colorpicker result messages
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
		var cmd tea.Cmd
		m.colorPicker, cmd = m.colorPicker.Update(msg)
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return CancelMsg{} }

		case "tab", "ctrl+n":
			m = m.cycleField(false)
			return m, nil

		case "shift+tab", "ctrl+p":
			m = m.cycleField(true)
			return m, nil

		case "enter":
			switch m.focused {
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
		}
	}

	// Forward to focused text input
	switch m.focused {
	case FieldViewName:
		var cmd tea.Cmd
		m.viewNameInput, cmd = m.viewNameInput.Update(msg)
		return m, cmd
	case FieldColumnName:
		var cmd tea.Cmd
		m.columnNameInput, cmd = m.columnNameInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

// cycleField moves focus to the next/previous field.
func (m Model) cycleField(reverse bool) Model {
	fields := []Field{FieldViewName, FieldColumnName, FieldColor, FieldSave}
	current := 0
	for i, f := range fields {
		if f == m.focused {
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

	m.focused = fields[current]

	// Update text input focus states
	switch m.focused {
	case FieldViewName:
		m.viewNameInput.Focus()
		m.columnNameInput.Blur()
	case FieldColumnName:
		m.viewNameInput.Blur()
		m.columnNameInput.Focus()
	default:
		m.viewNameInput.Blur()
		m.columnNameInput.Blur()
	}

	return m
}

// save validates input and returns a SaveMsg command.
func (m Model) save() (Model, tea.Cmd) {
	// Clear previous error
	m.saveError = ""

	// Validate view name
	viewName := strings.TrimSpace(m.viewNameInput.Value())
	if viewName == "" {
		m.saveError = "View name is required"
		return m, nil
	}

	// Check for duplicate view name
	for _, v := range m.existingViews {
		if strings.EqualFold(v.Name, viewName) {
			m.saveError = fmt.Sprintf("View '%s' already exists", v.Name)
			return m, nil
		}
	}

	// Column name defaults to view name if empty
	columnName := strings.TrimSpace(m.columnNameInput.Value())
	if columnName == "" {
		columnName = viewName
	}

	return m, func() tea.Msg {
		return SaveMsg{
			ViewName:   viewName,
			ColumnName: columnName,
			Color:      m.selectedColor.Hex,
			Query:      m.query,
		}
	}
}

// View renders the modal.
func (m Model) View() string {
	width := 50
	sectionWidth := width - 2

	// View name input section
	viewRows := []string{m.viewNameInput.View()}
	viewSection := styles.RenderFormSection(viewRows, "View Name", "required", sectionWidth, m.focused == FieldViewName, styles.BorderHighlightFocusColor)

	// Column name input section
	colRows := []string{m.columnNameInput.View()}
	colSection := styles.RenderFormSection(colRows, "Column Name", "optional", sectionWidth, m.focused == FieldColumnName, styles.BorderHighlightFocusColor)

	// Color section - shows selected color with swatch
	swatch := lipgloss.NewStyle().
		Background(lipgloss.Color(m.selectedColor.Hex)).
		Render("  ")
	colorRow := swatch + " " + m.selectedColor.Hex
	colorSection := styles.RenderFormSection([]string{colorRow}, "Color", "Enter to change", sectionWidth, m.focused == FieldColor, styles.BorderHighlightFocusColor)

	// Error message (if any)
	errorLine := ""
	if m.saveError != "" {
		errorStyle := lipgloss.NewStyle().Foreground(styles.StatusErrorColor)
		errorLine = errorStyle.Render(m.saveError)
	}

	// Save button
	saveStyle := styles.PrimaryButtonStyle
	if m.focused == FieldSave {
		saveStyle = styles.PrimaryButtonFocusedStyle
	}
	saveButton := saveStyle.Render(" Save ")

	// Title with bottom border
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.OverlayTitleColor)
	borderStyle := lipgloss.NewStyle().Foreground(styles.BorderDefaultColor)
	titleBorder := borderStyle.Render(strings.Repeat("─", width))

	// Content style adds horizontal padding
	contentPadding := lipgloss.NewStyle().PaddingLeft(1)

	// Build content
	content := contentPadding.Render(titleStyle.Render("Create New View")) + "\n" +
		titleBorder + "\n\n" +
		contentPadding.Render(viewSection) + "\n\n" +
		contentPadding.Render(colSection) + "\n\n" +
		contentPadding.Render(colorSection) + "\n"

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

// Overlay renders the modal on top of a background view.
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
