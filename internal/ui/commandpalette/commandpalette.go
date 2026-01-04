// Package commandpalette provides a searchable picker modal component
// for workflow selection and other quick-select use cases.
package commandpalette

import (
	"strings"

	"github.com/zjrosen/perles/internal/keys"
	"github.com/zjrosen/perles/internal/ui/shared/overlay"
	"github.com/zjrosen/perles/internal/ui/styles"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Item represents a selectable item in the command palette.
type Item struct {
	ID          string                 // Unique identifier
	Name        string                 // Display name (shown bold on first line)
	Description string                 // Description (shown muted on second line)
	Color       lipgloss.TerminalColor // Optional color for the name
}

// Config defines command palette configuration.
type Config struct {
	Title           string             // Modal title (empty = no title bar)
	Placeholder     string             // Search input placeholder
	Items           []Item             // Available items
	OnSelect        func(Item) tea.Msg // Called when item selected (optional)
	OnCancel        func() tea.Msg     // Called on Esc (optional)
	MinWidth        int                // Minimum width (default 45)
	MaxWidth        int                // Maximum width (default 80)
	MaxVisibleItems int                // Max items visible before scrolling (default 5)
}

// SelectMsg is sent when an item is selected (if OnSelect is nil).
type SelectMsg struct {
	Item Item
}

// CancelMsg is sent when cancelled (if OnCancel is nil).
type CancelMsg struct{}

// Model holds the command palette state.
type Model struct {
	config         Config
	textInput      textinput.Model
	filtered       []Item // Items matching search
	cursor         int    // Currently selected item index in filtered list
	scrollOffset   int    // First visible item index for scrolling
	viewportWidth  int
	viewportHeight int
}

// New creates a new command palette with the given configuration.
func New(cfg Config) Model {
	ti := textinput.New()
	ti.Placeholder = cfg.Placeholder
	if ti.Placeholder == "" {
		ti.Placeholder = "Search..."
	}
	ti.Prompt = ""
	ti.Focus()

	m := Model{
		config:    cfg,
		textInput: ti,
		filtered:  cfg.Items,
		cursor:    0,
	}

	return m
}

// Init returns the initial command (starts cursor blink).
func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles messages for the command palette.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case msg.Type == tea.KeyDown, key.Matches(msg, keys.Component.Next):
			// Move cursor down (arrow keys or ctrl+n only, not j - conflicts with typing)
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
				m = m.ensureCursorVisible()
			}
			return m, nil

		case msg.Type == tea.KeyUp, key.Matches(msg, keys.Component.Prev):
			// Move cursor up (arrow keys or ctrl+p only, not k - conflicts with typing)
			if m.cursor > 0 {
				m.cursor--
				m = m.ensureCursorVisible()
			}
			return m, nil

		case key.Matches(msg, keys.Common.Enter):
			// Select current item
			return m, m.selectCmd()

		case key.Matches(msg, keys.Common.Escape), msg.Type == tea.KeyCtrlC:
			// Cancel
			return m, m.cancelCmd()

		case msg.Type == tea.KeyCtrlU:
			// Clear search
			m.textInput.SetValue("")
			m = m.updateFilter()
			return m, nil

		default:
			// Forward to text input for typing
			var cmd tea.Cmd
			m.textInput, cmd = m.textInput.Update(msg)
			m = m.updateFilter()
			return m, cmd
		}

	case tea.MouseMsg:
		// Only handle wheel events for scrolling
		if msg.Button != tea.MouseButtonWheelUp && msg.Button != tea.MouseButtonWheelDown {
			return m, nil
		}
		maxVisible := m.maxVisibleItems()
		maxOffset := max(0, len(m.filtered)-maxVisible)
		if msg.Button == tea.MouseButtonWheelUp {
			if m.scrollOffset > 0 {
				m.scrollOffset--
			}
		} else {
			if m.scrollOffset < maxOffset {
				m.scrollOffset++
			}
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.viewportWidth = msg.Width
		m.viewportHeight = msg.Height
	}

	return m, nil
}

// updateFilter filters items based on current search text.
func (m Model) updateFilter() Model {
	query := strings.ToLower(m.textInput.Value())

	if query == "" {
		m.filtered = m.config.Items
	} else {
		var nameMatches []Item
		var descMatches []Item

		for _, item := range m.config.Items {
			nameLower := strings.ToLower(item.Name)
			descLower := strings.ToLower(item.Description)

			if strings.Contains(nameLower, query) {
				nameMatches = append(nameMatches, item)
			} else if strings.Contains(descLower, query) {
				descMatches = append(descMatches, item)
			}
		}

		// Name matches first, then description-only matches
		m.filtered = append(nameMatches, descMatches...)
	}

	// Reset cursor and scroll offset if cursor is out of bounds
	if m.cursor >= len(m.filtered) {
		m.cursor = 0
		m.scrollOffset = 0
	}

	return m
}

// maxVisibleItems returns the max visible items.
// Uses configured value or default, only shrinks if viewport is too small.
func (m Model) maxVisibleItems() int {
	// Target visible items (configured or default)
	target := m.config.MaxVisibleItems
	if target <= 0 {
		target = 5
	}

	// Only shrink if viewport forces it
	if m.viewportHeight > 0 {
		// Fixed overhead: border (2) + title+divider (2) + search+divider (2) + footer+divider (2)
		// Each item takes ~3 lines (name, description, divider)
		overhead := 8
		availableLines := m.viewportHeight - overhead
		maxFromViewport := max(availableLines/3, 2)
		if maxFromViewport < target {
			return maxFromViewport
		}
	}

	return target
}

// ensureCursorVisible adjusts scroll offset to keep cursor in view.
func (m Model) ensureCursorVisible() Model {
	maxVisible := m.maxVisibleItems()

	// Scroll down if cursor is below visible area
	if m.cursor >= m.scrollOffset+maxVisible {
		m.scrollOffset = m.cursor - maxVisible + 1
	}

	// Scroll up if cursor is above visible area
	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	}

	return m
}

// selectCmd returns the appropriate select command.
func (m Model) selectCmd() tea.Cmd {
	if len(m.filtered) == 0 {
		return nil
	}

	selected := m.filtered[m.cursor]
	if m.config.OnSelect != nil {
		return func() tea.Msg { return m.config.OnSelect(selected) }
	}
	return func() tea.Msg { return SelectMsg{Item: selected} }
}

// cancelCmd returns the appropriate cancel command.
func (m Model) cancelCmd() tea.Cmd {
	if m.config.OnCancel != nil {
		return func() tea.Msg { return m.config.OnCancel() }
	}
	return func() tea.Msg { return CancelMsg{} }
}

// SetSize sets the viewport dimensions for overlay rendering.
func (m Model) SetSize(width, height int) Model {
	m.viewportWidth = width
	m.viewportHeight = height
	return m
}

// Selected returns the currently selected item.
func (m Model) Selected() (Item, bool) {
	if m.cursor >= 0 && m.cursor < len(m.filtered) {
		return m.filtered[m.cursor], true
	}
	return Item{}, false
}

// Cursor returns the current cursor position.
func (m Model) Cursor() int {
	return m.cursor
}

// FilteredItems returns the currently filtered items.
func (m Model) FilteredItems() []Item {
	return m.filtered
}

// SearchText returns the current search text.
func (m Model) SearchText() string {
	return m.textInput.Value()
}

// View renders the command palette box.
func (m Model) View() string {
	maxWidth := m.config.MaxWidth
	if maxWidth == 0 {
		maxWidth = 80
	}

	// Use maxWidth directly for consistent sizing
	contentWidth := maxWidth

	// Styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.OverlayTitleColor).
		PaddingLeft(1)
	hintsStyle := lipgloss.NewStyle().
		Foreground(styles.TextMutedColor)
	dividerStyle := lipgloss.NewStyle().Foreground(styles.OverlayBorderColor)
	divider := dividerStyle.Render(strings.Repeat("─", contentWidth))

	// Search input with icon
	searchIcon := lipgloss.NewStyle().Foreground(styles.TextMutedColor).Render(" > ")
	m.textInput.Width = contentWidth - 4
	searchLine := searchIcon + m.textInput.View()

	// Build content
	var content strings.Builder

	// Title with hints on the right (if provided)
	if m.config.Title != "" {
		title := titleStyle.Render(m.config.Title)
		hints := hintsStyle.Render("↑/↓ • Enter • Esc")
		padding := max(contentWidth-lipgloss.Width(title)-lipgloss.Width(hints)-1, 1)
		content.WriteString(title + strings.Repeat(" ", padding) + hints)
		content.WriteString("\n")
		content.WriteString(divider)
		content.WriteString("\n")
	}

	// Search input
	content.WriteString(searchLine)
	content.WriteString("\n")
	content.WriteString(divider)

	// Items with scrolling - fixed height to prevent modal shifting
	maxVisible := m.maxVisibleItems()
	emptyLine := strings.Repeat(" ", contentWidth)

	if len(m.filtered) == 0 {
		noResultsStyle := lipgloss.NewStyle().
			Foreground(styles.TextMutedColor).
			Italic(true).
			Padding(1, 1)
		content.WriteString("\n")
		content.WriteString(noResultsStyle.Render("No matching items"))
		// Note: Padding(1,1) already provides 3 lines (top pad + text + bottom pad)
		// Don't add extra \n here to maintain consistent height with items
		// Pad remaining slots
		for i := 1; i < maxVisible; i++ {
			content.WriteString("\n")
			content.WriteString(emptyLine)
			content.WriteString("\n")
			content.WriteString(emptyLine)
			content.WriteString("\n")
		}
	} else {
		endIdx := min(m.scrollOffset+maxVisible, len(m.filtered))
		hasMoreBelow := endIdx < len(m.filtered)

		// Visible items
		renderedCount := 0
		for i := m.scrollOffset; i < endIdx; i++ {
			item := m.filtered[i]
			content.WriteString("\n")
			content.WriteString(m.renderItem(item, i == m.cursor, contentWidth))
			content.WriteString("\n") // Empty line after each item for spacing
			renderedCount++
		}

		// Pad remaining slots to maintain fixed height (3 lines each: name, desc, spacing)
		for i := renderedCount; i < maxVisible; i++ {
			content.WriteString("\n")
			content.WriteString(emptyLine)
			content.WriteString("\n")
			content.WriteString(emptyLine)
			content.WriteString("\n")
		}

		// Show "more" indicator if there are items below
		if hasMoreBelow {
			moreStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)
			moreText := moreStyle.Render("↓ more")
			// Center the indicator
			padding := (contentWidth - lipgloss.Width(moreText)) / 2
			content.WriteString(strings.Repeat(" ", padding) + moreText)
		}
	}

	// Wrap in bordered box
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.OverlayBorderColor).
		Width(contentWidth)

	return boxStyle.Render(content.String())
}

// renderItem renders a single item with name and description.
func (m Model) renderItem(item Item, selected bool, width int) string {
	var result strings.Builder

	// Use item's color if set, otherwise default
	nameStyle := lipgloss.NewStyle()
	if item.Color != nil {
		nameStyle = nameStyle.Foreground(item.Color)
	}
	if selected {
		nameStyle = nameStyle.Bold(true)
	}

	// Selection indicator
	var indicator string
	if selected {
		indicator = styles.SelectionIndicatorStyle.Render(">")
	} else {
		indicator = " "
	}

	// Calculate available width for name
	nameWidth := width - 2

	// Truncate name if needed
	name := item.Name
	if lipgloss.Width(name) > nameWidth {
		name = name[:nameWidth-3] + "..."
	}

	result.WriteString(indicator + nameStyle.Render(name))

	// Description with word wrapping
	if item.Description != "" {
		descStyle := lipgloss.NewStyle().
			Foreground(styles.TextMutedColor).
			Width(width - 4)

		result.WriteString("\n  ")
		result.WriteString(descStyle.Render(item.Description))
	}

	return result.String()
}

// Overlay renders the command palette on top of a background view.
func (m Model) Overlay(background string) string {
	paletteBox := m.View()

	if background == "" {
		return lipgloss.Place(
			m.viewportWidth, m.viewportHeight,
			lipgloss.Center, lipgloss.Center,
			paletteBox,
		)
	}

	return overlay.Place(overlay.Config{
		Width:    m.viewportWidth,
		Height:   m.viewportHeight,
		Position: overlay.Center,
	}, paletteBox, background)
}
