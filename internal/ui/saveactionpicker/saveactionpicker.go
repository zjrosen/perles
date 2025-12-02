// Package saveactionpicker provides a picker to choose between saving to an existing view or creating a new view.
package saveactionpicker

import (
	"perles/internal/ui/overlay"
	"perles/internal/ui/styles"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Action represents the user's choice.
type Action int

const (
	ActionExistingView Action = iota
	ActionNewView
)

// SelectMsg is sent when the user makes a selection.
type SelectMsg struct {
	Action Action
	Query  string
}

// CancelMsg is sent when the user cancels.
type CancelMsg struct{}

// Model holds the picker state.
type Model struct {
	query    string
	selected int
	width    int
	height   int
}

// New creates a new save action picker.
func New(query string) Model {
	return Model{
		query:    query,
		selected: 0,
	}
}

// SetSize sets the viewport dimensions for overlay rendering.
func (m Model) SetSize(width, height int) Model {
	m.width = width
	m.height = height
	return m
}

// Selected returns the currently selected index.
func (m Model) Selected() int {
	return m.selected
}

// Query returns the stored query string.
func (m Model) Query() string {
	return m.query
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down", "ctrl+n":
			m.selected = (m.selected + 1) % 2
		case "k", "up", "ctrl+p":
			m.selected = (m.selected + 1) % 2
		case "enter":
			action := ActionExistingView
			if m.selected == 1 {
				action = ActionNewView
			}
			return m, func() tea.Msg {
				return SelectMsg{
					Action: action,
					Query:  m.query,
				}
			}
		case "esc":
			return m, func() tea.Msg { return CancelMsg{} }
		}
	}
	return m, nil
}

// View renders the picker box.
func (m Model) View() string {
	options := []string{
		"Save to existing view",
		"Save to new view",
	}

	boxWidth := 30

	// Title
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.OverlayTitleColor).
		PaddingLeft(1)

	// Build options
	var optionsBuilder strings.Builder
	for i, opt := range options {
		var line string
		if i == m.selected {
			labelStyle := lipgloss.NewStyle().Bold(true)
			line = styles.SelectionIndicatorStyle.Render(">") + labelStyle.Render(opt)
		} else {
			line = " " + opt
		}
		optionsBuilder.WriteString(line)
		if i < len(options)-1 {
			optionsBuilder.WriteString("\n")
		}
	}

	// Divider
	dividerStyle := lipgloss.NewStyle().Foreground(styles.OverlayBorderColor)
	divider := dividerStyle.Render(strings.Repeat("─", boxWidth))

	// Build content
	content := titleStyle.Render("Save search query as column:") + "\n" +
		divider + "\n" +
		optionsBuilder.String()

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.OverlayBorderColor).
		Width(boxWidth)

	return boxStyle.Render(content)
}

// Overlay renders the picker on top of a background view.
func (m Model) Overlay(background string) string {
	pickerBox := m.View()

	if background == "" {
		return lipgloss.Place(
			m.width, m.height,
			lipgloss.Center, lipgloss.Center,
			pickerBox,
		)
	}

	return overlay.Place(overlay.Config{
		Width:    m.width,
		Height:   m.height,
		Position: overlay.Center,
	}, pickerBox, background)
}
