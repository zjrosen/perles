// Package picker provides a generic option picker component.
package picker

import (
	"perles/internal/ui/overlay"
	"perles/internal/ui/styles"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Option represents a picker option with label and value.
type Option struct {
	Label string
	Value string
	Color lipgloss.TerminalColor // Optional color for the label
}

// Model holds the picker state.
type Model struct {
	title          string
	options        []Option
	selected       int
	boxWidth       int // Width of the picker box itself
	viewportWidth  int // Full viewport width for overlay centering
	viewportHeight int // Full viewport height for overlay centering
}

// New creates a new picker with the given title and options.
func New(title string, options []Option) Model {
	return Model{
		title:    title,
		options:  options,
		selected: 0,
	}
}

// SetSize sets the viewport dimensions for overlay rendering.
func (m Model) SetSize(width, height int) Model {
	m.viewportWidth = width
	m.viewportHeight = height
	return m
}

// SetBoxWidth sets the width of the picker box itself.
func (m Model) SetBoxWidth(width int) Model {
	m.boxWidth = width
	return m
}

// SetSelected sets the initially selected index.
func (m Model) SetSelected(index int) Model {
	if index >= 0 && index < len(m.options) {
		m.selected = index
	}
	return m
}

// Selected returns the currently selected option.
func (m Model) Selected() Option {
	if m.selected >= 0 && m.selected < len(m.options) {
		return m.options[m.selected]
	}
	return Option{}
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down", "ctrl+n":
			if m.selected < len(m.options)-1 {
				m.selected++
			}
		case "k", "up", "ctrl+p":
			if m.selected > 0 {
				m.selected--
			}
		}
	}
	return m, nil
}

// View renders the picker box (without positioning).
func (m Model) View() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.OverlayTitleColor).
		PaddingLeft(1)

	// Build picker box (use boxWidth or default to 25)
	width := m.boxWidth
	if width == 0 {
		width = 25
	}

	// Build options
	var options strings.Builder
	for i, opt := range m.options {
		var line string
		if i == m.selected {
			// Selected: white bold "> " prefix, then bold label (with optional color)
			labelStyle := lipgloss.NewStyle().Bold(true)
			if opt.Color != nil {
				labelStyle = labelStyle.Foreground(opt.Color)
			}
			line = styles.SelectionIndicatorStyle.Render(">") + labelStyle.Render(opt.Label)
		} else {
			// Not selected: " " prefix, then label (with optional color)
			labelStyle := lipgloss.NewStyle()
			if opt.Color != nil {
				labelStyle = labelStyle.Foreground(opt.Color)
			}
			line = " " + labelStyle.Render(opt.Label)
		}
		options.WriteString(line)
		if i < len(m.options)-1 {
			options.WriteString("\n")
		}
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.OverlayBorderColor).
		Width(width)

	// Divider spans full width (no padding)
	dividerStyle := lipgloss.NewStyle().Foreground(styles.OverlayBorderColor)
	divider := dividerStyle.Render(strings.Repeat("─", width))
	content := titleStyle.Render(m.title) + "\n" +
		divider + "\n" +
		options.String()

	return boxStyle.Render(content)
}

// Overlay renders the picker on top of a background view.
func (m Model) Overlay(background string) string {
	pickerBox := m.View()

	if background == "" {
		return lipgloss.Place(
			m.viewportWidth, m.viewportHeight,
			lipgloss.Center, lipgloss.Center,
			pickerBox,
		)
	}

	return overlay.Place(overlay.Config{
		Width:    m.viewportWidth,
		Height:   m.viewportHeight,
		Position: overlay.Center,
	}, pickerBox, background)
}

// CancelMsg is sent when the picker is cancelled.
type CancelMsg struct{}

// FindIndexByValue returns the index of the option with the given value.
func FindIndexByValue(options []Option, value string) int {
	for i, opt := range options {
		if opt.Value == value {
			return i
		}
	}
	return 0
}
