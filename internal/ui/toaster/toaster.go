// Package toaster provides a notification toast overlay component.
package toaster

import (
	"perles/internal/ui/overlay"
	"perles/internal/ui/styles"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Style determines the visual appearance of the toast.
type Style int

const (
	// StyleSuccess shows ✅ with green border.
	StyleSuccess Style = iota
	// StyleError shows ❌ with red background.
	StyleError
	// StyleInfo shows ℹ️ with blue border for informational messages.
	StyleInfo
	// StyleWarn shows ⚠️ with yellow background for warnings.
	StyleWarn
)

// Model holds the toaster state.
type Model struct {
	message string
	style   Style
	visible bool
	width   int
	height  int
}

// New creates a new toaster model.
func New() Model {
	return Model{}
}

// Show displays a toast with the given message and style.
// The appropriate emoji is automatically prepended based on style:
// ✅ success, ❌ error, ℹ️ info, ⚠️ warn.
func (m Model) Show(message string, style Style) Model {
	m.message = message
	m.style = style
	m.visible = true
	return m
}

// Hide dismisses the toast.
func (m Model) Hide() Model {
	m.visible = false
	m.message = ""
	return m
}

// Visible returns whether the toast is currently showing.
func (m Model) Visible() bool {
	return m.visible
}

// SetSize updates the viewport dimensions for overlay positioning.
func (m Model) SetSize(width, height int) Model {
	m.width = width
	m.height = height
	return m
}

// View renders the toast box.
func (m Model) View() string {
	if !m.visible || m.message == "" {
		return ""
	}

	style := lipgloss.NewStyle().
		Padding(0, 1).
		Border(lipgloss.RoundedBorder())

	var content string
	switch m.style {
	case StyleError:
		style = style.BorderForeground(styles.ToastBorderErrorColor)
		content = "❌ " + m.message
	case StyleInfo:
		style = style.BorderForeground(styles.ToastBorderInfoColor)
		content = "ℹ️ " + m.message
	case StyleWarn:
		style = style.BorderForeground(styles.ToastBorderWarnColor)
		content = "⚠️ " + m.message
	default: // StyleSuccess
		style = style.BorderForeground(styles.ToastBorderSuccessColor)
		content = "✅ " + m.message
	}

	return style.Render(content)
}

// Overlay renders the toast on top of a background view.
// Uses bottom-center positioning with padding from the bottom edge.
func (m Model) Overlay(bg string, width, height int) string {
	if !m.visible || m.message == "" {
		return bg
	}

	fg := m.View()

	cfg := overlay.Config{
		Width:    width,
		Height:   height,
		Position: overlay.Bottom,
		PadY:     1, // Padding from bottom edge
	}

	return overlay.Place(cfg, fg, bg)
}

// DismissMsg signals that the toast should be dismissed.
type DismissMsg struct{}

// ScheduleDismiss returns a command that dismisses the toast after a duration.
func ScheduleDismiss(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(_ time.Time) tea.Msg {
		return DismissMsg{}
	})
}
