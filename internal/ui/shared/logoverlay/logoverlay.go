// Package logoverlay provides an in-app log viewer overlay that shows
// recent log entries without leaving the TUI.
package logoverlay

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"perles/internal/log"
	"perles/internal/ui/shared/overlay"
	"perles/internal/ui/styles"
)

const (
	viewportMaxHeight = 25  // Fixed viewport height in lines
	viewportMinHeight = 5   // Minimum viewport height for very small screens
	boxMaxWidth       = 160 // Maximum box width in characters
	boxMinWidth       = 40  // Minimum box width in characters
)

// CloseMsg is sent when the overlay should be closed.
type CloseMsg struct{}

// Model is the log overlay component state.
type Model struct {
	visible  bool
	minLevel log.Level
	width    int
	height   int
	viewport viewport.Model
}

// New creates a new log overlay model.
func New() Model {
	return Model{
		visible:  false,
		minLevel: log.LevelDebug,
	}
}

// NewWithSize creates a new log overlay with the given dimensions.
func NewWithSize(width, height int) Model {
	return Model{
		visible:  false,
		minLevel: log.LevelDebug,
		width:    width,
		height:   height,
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages for the log overlay.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.visible {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "c":
			// Clear buffer
			log.ClearBuffer()
			m.refreshViewport()
			return m, nil

		case "d":
			// Filter to DEBUG and above
			m.minLevel = log.LevelDebug
			m.refreshViewport()
			return m, nil

		case "i":
			// Filter to INFO and above
			m.minLevel = log.LevelInfo
			m.refreshViewport()
			return m, nil

		case "w":
			// Filter to WARN and above
			m.minLevel = log.LevelWarn
			m.refreshViewport()
			return m, nil

		case "e":
			// Filter to ERROR only
			m.minLevel = log.LevelError
			m.refreshViewport()
			return m, nil

		case "j", "down":
			m.viewport.ScrollDown(1)
			return m, nil

		case "k", "up":
			m.viewport.ScrollUp(1)
			return m, nil

		case "g":
			m.viewport.GotoTop()
			return m, nil

		case "G":
			m.viewport.GotoBottom()
			return m, nil

		case "ctrl+c":
			// Quit the app
			return m, tea.Quit

		case "ctrl+x", "esc":
			// Close overlay
			m.visible = false
			return m, func() tea.Msg { return CloseMsg{} }
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.refreshViewport()
	}

	return m, nil
}

// View renders the log overlay content.
func (m Model) View() string {
	if !m.visible {
		return ""
	}

	boxWidth := m.boxWidth()

	// Styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.OverlayTitleColor).
		PaddingLeft(1)

	dividerStyle := lipgloss.NewStyle().
		Foreground(styles.OverlayBorderColor)
	divider := dividerStyle.Render(strings.Repeat("─", boxWidth))

	// Build header
	header := titleStyle.Render("Logs")

	// Build log content for viewport
	content := m.viewport.View()

	// Build footer with key hints
	footerDivider := dividerStyle.Render(strings.Repeat("─", boxWidth))
	filterHint := m.buildFilterHint()

	// Assemble layout
	var result strings.Builder
	result.WriteString(header)
	result.WriteString("\n")
	result.WriteString(divider)
	result.WriteString("\n")
	result.WriteString(content)
	result.WriteString("\n")
	result.WriteString(footerDivider)
	result.WriteString("\n")
	result.WriteString(filterHint)

	// Wrap in bordered box
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.OverlayBorderColor).
		Width(boxWidth)

	return boxStyle.Render(result.String())
}

// getFilteredLogs returns log entries matching the current filter level.
func (m Model) getFilteredLogs() []string {
	// Get all logs (pass large number to get entire buffer)
	logs := log.GetRecentLogs(10000)
	var filtered []string
	for _, entry := range logs {
		if m.matchesLevel(entry) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

// buildLogContent builds the log content string for display.
func (m Model) buildLogContent(contentWidth int) string {
	filtered := m.getFilteredLogs()

	if len(filtered) == 0 {
		emptyStyle := lipgloss.NewStyle().
			Foreground(styles.TextMutedColor).
			Italic(true)
		return emptyStyle.Render("No logs to display")
	}

	var lines []string
	for _, entry := range filtered {
		lines = append(lines, m.colorizeEntry(entry, contentWidth))
	}
	return strings.Join(lines, "\n")
}

// refreshViewport initializes or updates the viewport with current log content.
func (m *Model) refreshViewport() {
	if m.width == 0 || m.height == 0 {
		return
	}

	contentWidth := m.contentWidth()

	// Use fixed 25-line height, constrained by screen size
	// Account for header (2 lines), footer (2 lines), borders (2 lines) = 6 lines overhead
	maxAllowed := m.height - 6
	viewportHeight := min(viewportMaxHeight, maxAllowed)
	viewportHeight = max(viewportHeight, viewportMinHeight)

	m.viewport = viewport.New(contentWidth, viewportHeight)
	m.viewport.SetContent(m.buildLogContent(contentWidth))
}

// Overlay renders the log overlay centered on the given background.
func (m Model) Overlay(bg string) string {
	if !m.visible {
		return bg
	}
	fg := m.View()
	return overlay.Place(overlay.Config{
		Width:    m.width,
		Height:   m.height,
		Position: overlay.Center,
	}, fg, bg)
}

// Visible returns whether the overlay is currently visible.
func (m Model) Visible() bool {
	return m.visible
}

// boxWidth returns the calculated box width based on screen size.
func (m Model) boxWidth() int {
	return max(min(m.width-4, boxMaxWidth), boxMinWidth)
}

// contentWidth returns the content width (box width minus borders).
func (m Model) contentWidth() int {
	return m.boxWidth() - 2
}

// Toggle toggles the overlay visibility.
func (m *Model) Toggle() {
	m.visible = !m.visible
	if m.visible {
		m.refreshViewport()
	}
}

// Show makes the overlay visible.
func (m *Model) Show() {
	m.visible = true
	m.refreshViewport()
}

// Hide makes the overlay invisible.
func (m *Model) Hide() {
	m.visible = false
}

// SetSize updates the overlay's knowledge of viewport size.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.refreshViewport()
}

// matchesLevel checks if a log entry matches the current filter level.
// Log levels are ordered: DEBUG(0) < INFO(1) < WARN(2) < ERROR(3).
// The minLevel filter shows entries at or above that level.
// Example: minLevel=WARN shows WARN and ERROR entries, filters out DEBUG and INFO.
func (m Model) matchesLevel(entry string) bool {
	// Determine the level of this log entry
	var entryLevel log.Level
	switch {
	case strings.Contains(entry, "[ERROR]"):
		entryLevel = log.LevelError
	case strings.Contains(entry, "[WARN]"):
		entryLevel = log.LevelWarn
	case strings.Contains(entry, "[INFO]"):
		entryLevel = log.LevelInfo
	case strings.Contains(entry, "[DEBUG]"):
		entryLevel = log.LevelDebug
	default:
		return true // Unknown level entries always shown
	}
	// Show entry if its level is >= the minimum filter level
	return entryLevel >= m.minLevel
}

// colorizeEntry applies color to a log entry based on its level.
func (m Model) colorizeEntry(entry string, maxWidth int) string {
	// Remove trailing newline if present
	entry = strings.TrimSuffix(entry, "\n")

	// Truncate long entries using ANSI-aware truncation (handles UTF-8 correctly)
	if ansi.StringWidth(entry) > maxWidth {
		entry = ansi.Truncate(entry, maxWidth-3, "...")
	}

	var style lipgloss.Style
	switch {
	case strings.Contains(entry, "[ERROR]"):
		style = lipgloss.NewStyle().Foreground(styles.StatusErrorColor)
	case strings.Contains(entry, "[WARN]"):
		style = lipgloss.NewStyle().Foreground(styles.StatusWarningColor)
	case strings.Contains(entry, "[INFO]"):
		style = lipgloss.NewStyle().Foreground(styles.ToastBorderInfoColor)
	case strings.Contains(entry, "[DEBUG]"):
		style = lipgloss.NewStyle().Foreground(styles.TextMutedColor)
	default:
		style = lipgloss.NewStyle().Foreground(styles.TextPrimaryColor)
	}

	return style.Render(entry)
}

// buildFilterHint creates the footer hint showing filter options.
// The active filter level is highlighted with bold styling.
func (m Model) buildFilterHint() string {
	hintStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)
	activeStyle := lipgloss.NewStyle().
		Foreground(styles.TextPrimaryColor).
		Bold(true)

	hints := []string{hintStyle.Render("[c] Clear")}

	// Add filter options with active one highlighted
	if m.minLevel == log.LevelDebug {
		hints = append(hints, activeStyle.Render("[d] Debug"))
	} else {
		hints = append(hints, hintStyle.Render("[d] Debug"))
	}

	if m.minLevel == log.LevelInfo {
		hints = append(hints, activeStyle.Render("[i] Info"))
	} else {
		hints = append(hints, hintStyle.Render("[i] Info"))
	}

	if m.minLevel == log.LevelWarn {
		hints = append(hints, activeStyle.Render("[w] Warn"))
	} else {
		hints = append(hints, hintStyle.Render("[w] Warn"))
	}

	if m.minLevel == log.LevelError {
		hints = append(hints, activeStyle.Render("[e] Error"))
	} else {
		hints = append(hints, hintStyle.Render("[e] Error"))
	}

	return strings.Join(hints, "  ")
}
