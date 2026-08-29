// Package threadpicker provides thread selection UI for fabric channels.
package threadpicker

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zjrosen/perles/internal/orchestration/fabric/domain"
	"github.com/zjrosen/perles/internal/ui/styles"
)

// Model holds the thread picker state.
type Model struct {
	// Available threads in current channel
	threads []domain.Thread

	// Current state
	active       bool   // Whether picker is showing
	query        string // Filter query
	filtered     []domain.Thread
	cursor       int // Selected item in filtered list
	maxVisible   int // Max items to show before scrolling
	scrollOffset int // First visible item index
}

// New creates a new thread picker model.
func New() Model {
	return Model{
		threads:    make([]domain.Thread, 0),
		filtered:   make([]domain.Thread, 0),
		maxVisible: 6,
	}
}

// SetThreads updates the list of available threads.
func (m Model) SetThreads(threads []domain.Thread) Model {
	m.threads = threads
	if m.active {
		m = m.updateFilter()
	}
	return m
}

// IsActive returns whether the picker is currently showing.
func (m Model) IsActive() bool {
	return m.active
}

// ThreadCount returns the number of available threads.
func (m Model) ThreadCount() int {
	return len(m.threads)
}

// Selected returns the currently selected thread, or nil if none.
func (m Model) Selected() *domain.Thread {
	if !m.active || len(m.filtered) == 0 || m.cursor >= len(m.filtered) {
		return nil
	}
	return &m.filtered[m.cursor]
}

// Activate opens the picker with the given threads.
func (m Model) Activate(threads []domain.Thread) Model {
	m.threads = threads
	m.active = true
	m.query = ""
	m.cursor = 0
	m.scrollOffset = 0
	m = m.updateFilter()
	return m
}

// Deactivate closes the picker.
func (m Model) Deactivate() Model {
	m.active = false
	m.query = ""
	m.cursor = 0
	m.scrollOffset = 0
	m.filtered = nil
	return m
}

// UpdateQuery updates the filter query and re-filters.
// Returns false if no matches (caller may choose to keep open or close).
func (m Model) UpdateQuery(query string) (Model, bool) {
	m.query = query
	m = m.updateFilter()
	return m, len(m.filtered) > 0
}

// updateFilter filters threads based on current query.
func (m Model) updateFilter() Model {
	query := strings.ToLower(m.query)

	m.filtered = make([]domain.Thread, 0, len(m.threads))
	for _, t := range m.threads {
		contentLower := strings.ToLower(t.Content)
		authorLower := strings.ToLower(t.CreatedBy)
		if query == "" || strings.Contains(contentLower, query) || strings.Contains(authorLower, query) {
			m.filtered = append(m.filtered, t)
		}
	}

	// Reset cursor if out of bounds
	if m.cursor >= len(m.filtered) {
		m.cursor = 0
		m.scrollOffset = 0
	}

	return m
}

// Next moves to the next item.
func (m Model) Next() Model {
	if len(m.filtered) == 0 {
		return m
	}
	m.cursor = (m.cursor + 1) % len(m.filtered)
	m = m.ensureVisible()
	return m
}

// Prev moves to the previous item.
func (m Model) Prev() Model {
	if len(m.filtered) == 0 {
		return m
	}
	m.cursor = (m.cursor - 1 + len(m.filtered)) % len(m.filtered)
	m = m.ensureVisible()
	return m
}

// ensureVisible ensures the cursor is visible within the scroll window.
func (m Model) ensureVisible() Model {
	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	} else if m.cursor >= m.scrollOffset+m.maxVisible {
		m.scrollOffset = m.cursor - m.maxVisible + 1
	}
	return m
}

// HandleKey processes key events during picker display.
// Returns (updated model, consumed bool, selected *domain.Thread if enter pressed).
func (m Model) HandleKey(msg tea.KeyMsg) (Model, bool, *domain.Thread) {
	if !m.active {
		return m, false, nil
	}

	switch msg.String() {
	case "ctrl+n", "down", "j":
		return m.Next(), true, nil
	case "ctrl+p", "up", "k":
		return m.Prev(), true, nil
	case "enter":
		selected := m.Selected()
		if selected != nil {
			return m.Deactivate(), true, selected
		}
		return m, true, nil
	case "esc":
		return m.Deactivate(), true, nil
	}

	return m, false, nil
}

// View renders the thread picker popup.
func (m Model) View(maxWidth int) string {
	if !m.active || len(m.filtered) == 0 {
		return ""
	}

	// Calculate visible items
	visibleCount := min(m.maxVisible, len(m.filtered))
	endIdx := min(m.scrollOffset+visibleCount, len(m.filtered))

	// Column widths
	idWidth := 8      // "abc12345"
	authorWidth := 10 // "worker-1" or "coord"
	// Content gets remaining space minus separators, padding, and border
	// Layout: " id │ author │ content " = 1 + idWidth + 3 + authorWidth + 3 + content + 1
	fixedWidth := 1 + idWidth + 3 + authorWidth + 3 + 1 + 2 // +2 for border
	contentWidth := max(maxWidth-fixedWidth, 10)

	// Total inner width (without border)
	innerWidth := maxWidth - 2

	// Styles
	normalStyle := lipgloss.NewStyle().
		Foreground(styles.TextPrimaryColor).
		Width(innerWidth)
	selectedStyle := lipgloss.NewStyle().
		Foreground(styles.TextPrimaryColor).
		Background(styles.SelectionBackgroundColor).
		Width(innerWidth)
	mutedStyle := lipgloss.NewStyle().
		Foreground(styles.TextMutedColor)

	// Build items
	var lines []string
	for i := m.scrollOffset; i < endIdx; i++ {
		t := m.filtered[i]

		// Format ID (first 8 chars)
		id := t.ID
		if len(id) > idWidth {
			id = id[:idWidth]
		}

		// Format author (truncate if needed)
		author := t.CreatedBy
		if author == "coordinator" {
			author = "coord"
		}
		if len(author) > authorWidth {
			author = author[:authorWidth]
		}

		// Format content preview (first line, truncated)
		content, _, _ := strings.Cut(t.Content, "\n")
		if len(content) > contentWidth {
			content = content[:contentWidth-3] + "..."
		}

		// Build row: " id │ author │ content " (no cursor char, highlight shows selection)
		row := fmt.Sprintf(" %-*s │ %-*s │ %-*s ",
			idWidth, id,
			authorWidth, author,
			contentWidth, content)

		// Apply selection styling (width is set in style, highlight shows selection)
		if i == m.cursor {
			lines = append(lines, selectedStyle.Render(row))
		} else {
			lines = append(lines, normalStyle.Render(row))
		}
	}

	// Add scroll indicator if needed
	if len(m.filtered) > m.maxVisible {
		scrollInfo := fmt.Sprintf(" %d-%d of %d threads",
			m.scrollOffset+1,
			min(m.scrollOffset+m.maxVisible, len(m.filtered)),
			len(m.filtered))
		lines = append(lines, mutedStyle.Render(scrollInfo))
	}

	// Build popup box
	content := strings.Join(lines, "\n")

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.BorderDefaultColor)

	return borderStyle.Render(content)
}
