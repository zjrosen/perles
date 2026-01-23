package table

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"

	"github.com/zjrosen/perles/internal/ui/shared/panes"
	"github.com/zjrosen/perles/internal/ui/styles"
)

// Model holds table rendering state.
// The table is a pure render component with external state management.
// Use View() for rendering without selection, or ViewWithSelection() for highlighting.
type Model struct {
	config TableConfig
	rows   []any
	width  int
	height int
}

// New creates a table with the given configuration.
// Panics if the configuration is invalid (no columns or missing Render callbacks).
func New(cfg TableConfig) Model {
	if err := ValidateConfig(cfg); err != nil {
		panic(err)
	}

	// Apply defaults
	if cfg.EmptyMessage == "" {
		cfg.EmptyMessage = "No data"
	}

	return Model{
		config: cfg,
		rows:   make([]any, 0),
	}
}

// SetRows updates the row data and returns a new Model (immutable pattern).
func (m Model) SetRows(rows []any) Model {
	m.rows = rows
	return m
}

// SetSize sets the available dimensions and returns a new Model (immutable pattern).
func (m Model) SetSize(width, height int) Model {
	m.width = width
	m.height = height
	return m
}

// RowCount returns the number of rows in the table.
func (m Model) RowCount() int {
	return len(m.rows)
}

// View renders the table without selection highlighting.
func (m Model) View() string {
	return m.renderTable(-1)
}

// ViewWithSelection renders the table with the specified row highlighted.
// Out-of-bounds selection index is treated as no selection.
func (m Model) ViewWithSelection(selectedIndex int) string {
	return m.renderTable(selectedIndex)
}

// renderTable is the internal rendering function.
// selectedIndex < 0 or >= len(rows) means no selection.
func (m Model) renderTable(selectedIndex int) string {
	// Return empty string for zero or negative dimensions
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	// Calculate inner width (inside borders if ShowBorder is true)
	innerWidth := m.width
	innerHeight := m.height
	if m.config.ShowBorder {
		innerWidth -= 2  // left and right border
		innerHeight -= 2 // top and bottom border
	}

	if innerWidth <= 0 || innerHeight <= 0 {
		return ""
	}

	// Filter columns based on width thresholds (responsive hiding)
	visibleColumns := filterVisibleColumns(m.config.Columns, m.width)

	// Calculate column widths for visible columns only
	widths := calculateColumnWidths(visibleColumns, innerWidth)

	// Determine content height available for rows
	contentHeight := innerHeight
	if m.config.ShowHeader {
		contentHeight-- // header row takes 1 line
	}

	// Render content
	var content string
	if len(m.rows) == 0 {
		// Empty state
		content = renderEmptyState(m.config.EmptyMessage, innerWidth, innerHeight)
	} else {
		// Build table content
		var lines []string

		// Header row
		if m.config.ShowHeader {
			headerLine := renderHeader(visibleColumns, widths)
			// Style header with muted color
			headerStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)
			headerLine = headerStyle.Render(headerLine)
			lines = append(lines, headerLine)
		}

		// Data rows
		dataRowCount := min(len(m.rows), contentHeight)
		for i := range dataRowCount {
			selected := i == selectedIndex
			rowLine := renderRow(m.rows[i], visibleColumns, widths, selected, innerWidth)

			// Wrap row with zone mark if RowZoneID callback is set
			if m.config.RowZoneID != nil {
				zoneID := m.config.RowZoneID(i, m.rows[i])
				if zoneID != "" {
					rowLine = zone.Mark(zoneID, rowLine)
				}
			}

			lines = append(lines, rowLine)
		}

		// Pad remaining height with empty lines
		currentLines := len(lines)
		for i := currentLines; i < innerHeight; i++ {
			lines = append(lines, "")
		}

		content = strings.Join(lines, "\n")
	}

	// Wrap in border if configured
	if m.config.ShowBorder {
		return panes.BorderedPane(panes.BorderConfig{
			Content:     content,
			Width:       m.width,
			Height:      m.height,
			TopLeft:     m.config.Title,
			BorderColor: m.config.BorderColor,
			PreWrapped:  true, // Content already handles its own width/height
		})
	}

	return content
}
