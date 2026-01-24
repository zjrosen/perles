package table

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"

	"github.com/zjrosen/perles/internal/ui/shared/panes"
	"github.com/zjrosen/perles/internal/ui/styles"
)

// Model holds table rendering state.
// The table supports optional scrolling via an internal viewport.
// Use View() for rendering without selection, or ViewWithSelection() for highlighting.
// When Scrollable is enabled in config, call Update() to handle scroll events.
type Model struct {
	config        TableConfig
	rows          []any
	width         int
	height        int
	viewport      viewport.Model // Internal viewport for scrollable tables
	targetYOffset int            // Desired scroll offset (applied after SetContent)
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

	m := Model{
		config: cfg,
		rows:   make([]any, 0),
	}

	// Initialize viewport for scrollable tables
	if cfg.Scrollable {
		m.viewport = viewport.New(0, 0)
		m.viewport.MouseWheelEnabled = true
	}

	return m
}

// SetRows updates the row data and returns a new Model (immutable pattern).
func (m Model) SetRows(rows []any) Model {
	m.rows = rows
	return m
}

// SetConfig updates the table configuration without recreating the viewport.
// Use this to update dynamic config values like Focused state.
func (m Model) SetConfig(cfg TableConfig) Model {
	m.config = cfg
	return m
}

// SetSize sets the available dimensions and returns a new Model (immutable pattern).
func (m Model) SetSize(width, height int) Model {
	m.width = width
	m.height = height

	// Update viewport size if scrollable
	if m.config.Scrollable {
		// Calculate viewport dimensions (inside borders, minus header)
		vpWidth := width
		vpHeight := height
		if m.config.ShowBorder {
			vpWidth -= 2
			vpHeight -= 2
		}
		if m.config.ShowHeader {
			vpHeight--
		}
		m.viewport.Width = max(0, vpWidth)
		m.viewport.Height = max(0, vpHeight)

		// Clamp scroll offset to valid range
		m.targetYOffset = m.clampYOffset(m.targetYOffset)
	}

	return m
}

// SetYOffset sets the vertical scroll offset for scrollable tables.
// For non-scrollable tables, this is a no-op.
// The offset is applied after SetContent during rendering.
func (m Model) SetYOffset(offset int) Model {
	if m.config.Scrollable {
		m.targetYOffset = m.clampYOffset(offset)
	}
	return m
}

// YOffset returns the current vertical scroll offset.
func (m Model) YOffset() int {
	return m.targetYOffset
}

// clampYOffset ensures the offset is within valid bounds.
func (m Model) clampYOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	// Max offset is total rows minus viewport height
	maxOffset := max(len(m.rows)-m.viewport.Height, 0)
	if offset > maxOffset {
		return maxOffset
	}
	return offset
}

// RowCount returns the number of rows in the table.
func (m Model) RowCount() int {
	return len(m.rows)
}

// Update handles messages for scrollable tables.
// For non-scrollable tables, this is a no-op.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.config.Scrollable {
		return m, nil
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// EnsureVisible scrolls to make the given row index visible.
// For non-scrollable tables, this is a no-op.
func (m Model) EnsureVisible(rowIndex int) Model {
	if !m.config.Scrollable || rowIndex < 0 || rowIndex >= len(m.rows) {
		return m
	}

	// Each row is 1 line high
	rowOffset := rowIndex

	// If row is above viewport, scroll up to it
	if rowOffset < m.targetYOffset {
		m.targetYOffset = m.clampYOffset(rowOffset)
	}

	// If row is below viewport, scroll down to show it at bottom
	viewportHeight := m.viewport.Height
	if rowOffset >= m.targetYOffset+viewportHeight {
		m.targetYOffset = m.clampYOffset(rowOffset - viewportHeight + 1)
	}

	return m
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
	} else if m.config.Scrollable {
		// Scrollable table: header + viewport with all rows
		content = m.renderScrollableContent(visibleColumns, widths, innerWidth, innerHeight, selectedIndex)
	} else {
		// Non-scrollable table: header + limited rows + padding
		content = m.renderStaticContent(visibleColumns, widths, innerWidth, innerHeight, contentHeight, selectedIndex)
	}

	// Wrap in border if configured
	if m.config.ShowBorder {
		return panes.BorderedPane(panes.BorderConfig{
			Content:            content,
			Width:              m.width,
			Height:             m.height,
			TopLeft:            m.config.Title,
			BorderColor:        m.config.BorderColor,
			Focused:            m.config.Focused,
			FocusedBorderColor: m.config.FocusedBorderColor,
			PreWrapped:         true, // Content already handles its own width/height
		})
	}

	return content
}

// renderStaticContent renders the table content for non-scrollable tables.
func (m Model) renderStaticContent(visibleColumns []ColumnConfig, widths []int, innerWidth, innerHeight, contentHeight, selectedIndex int) string {
	var lines []string

	// Header row
	if m.config.ShowHeader {
		headerLine := renderHeader(visibleColumns, widths)
		headerStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)
		headerLine = headerStyle.Render(headerLine)
		lines = append(lines, headerLine)
	}

	// Data rows (limited to visible area)
	dataRowCount := min(len(m.rows), contentHeight)
	for i := range dataRowCount {
		selected := i == selectedIndex
		rowLine := renderRow(m.rows[i], visibleColumns, widths, selected, innerWidth)

		if m.config.RowZoneID != nil {
			zoneID := m.config.RowZoneID(i, m.rows[i])
			if zoneID != "" {
				rowLine = zone.Mark(zoneID, rowLine)
			}
		}

		lines = append(lines, rowLine)
	}

	// Pad remaining height with empty lines
	for i := len(lines); i < innerHeight; i++ {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

// renderScrollableContent renders the table content for scrollable tables.
// Header stays sticky, only visible rows are rendered (not all rows).
func (m Model) renderScrollableContent(visibleColumns []ColumnConfig, widths []int, innerWidth, innerHeight, selectedIndex int) string {
	var lines []string

	// Sticky header row
	if m.config.ShowHeader {
		headerLine := renderHeader(visibleColumns, widths)
		headerStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)
		headerLine = headerStyle.Render(headerLine)
		lines = append(lines, headerLine)
	}

	// Calculate visible row range based on scroll offset and viewport height
	viewportHeight := m.viewport.Height
	if viewportHeight <= 0 {
		viewportHeight = innerHeight
		if m.config.ShowHeader {
			viewportHeight--
		}
	}

	startRow := m.targetYOffset
	endRow := min(startRow+viewportHeight, len(m.rows))
	if startRow < 0 {
		startRow = 0
	}

	// Only render visible rows (not all rows)
	var rowLines []string
	for i := startRow; i < endRow; i++ {
		selected := i == selectedIndex
		rowLine := renderRow(m.rows[i], visibleColumns, widths, selected, innerWidth)

		if m.config.RowZoneID != nil {
			zoneID := m.config.RowZoneID(i, m.rows[i])
			if zoneID != "" {
				rowLine = zone.Mark(zoneID, rowLine)
			}
		}

		rowLines = append(rowLines, rowLine)
	}

	// Join visible rows directly (no viewport needed since we're managing scroll ourselves)
	lines = append(lines, strings.Join(rowLines, "\n"))

	// Pad remaining height with empty lines
	renderedRows := endRow - startRow
	for i := renderedRows; i < viewportHeight; i++ {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}
