// Package board contains the kanban board component.
package board

import (
	"strings"

	"perles/internal/beads"
	"perles/internal/bql"
	"perles/internal/config"
	"perles/internal/ui/styles"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ColumnIndex identifies kanban columns (backward compatibility).
// Deprecated: Use int directly with NewFromConfig for custom columns.
type ColumnIndex = int

// Default column indices for backward compatibility with New().
const (
	ColBlocked    ColumnIndex = 0
	ColReady      ColumnIndex = 1
	ColInProgress ColumnIndex = 2
	ColClosed     ColumnIndex = 3
)

// View represents a named collection of columns.
type View struct {
	name    string
	columns []Column
	configs []config.ColumnConfig
	loaded  bool // true if this view has been loaded at least once
}

// Model holds the board state with dynamic columns and multi-view support.
type Model struct {
	// View management
	views       []View // all configured views
	currentView int    // index of active view

	// Active view's columns (for backward compatibility)
	columns  []Column
	configs  []config.ColumnConfig
	executor *bql.Executor // BQL executor for column loading
	focused  int
	width    int
	height   int
}

// New creates a new board with default columns (backward compatibility).
func New() Model {
	return NewFromConfig(config.DefaultColumns())
}

// NewFromConfig creates a board with columns from configuration.
// For self-loading columns, use NewFromConfigWithExecutor instead.
func NewFromConfig(configs []config.ColumnConfig) Model {
	return NewFromConfigWithExecutor(configs, nil)
}

// NewFromConfigWithExecutor creates a board with columns that can self-load via BQL.
func NewFromConfigWithExecutor(configs []config.ColumnConfig, executor *bql.Executor) Model {
	columns := make([]Column, len(configs))

	for i, cfg := range configs {
		// Extract primary status from Query for column rendering
		primaryStatus := extractPrimaryStatus(cfg.Query)

		// Create column with executor for self-loading
		col := NewColumnWithExecutor(cfg.Name, cfg.Query, executor)
		col.status = primaryStatus
		if cfg.Color != "" {
			col = col.SetColor(lipgloss.Color(cfg.Color))
		}
		columns[i] = col
	}

	// Default focus to second column (Ready equivalent) or first
	focusIdx := 0
	if len(columns) > 1 {
		focusIdx = 1
	}

	return Model{
		columns:  columns,
		configs:  configs,
		executor: executor,
		focused:  focusIdx,
	}
}

// NewFromViews creates a board from multiple view configurations.
func NewFromViews(viewConfigs []config.ViewConfig, executor *bql.Executor) Model {
	views := make([]View, len(viewConfigs))

	for i, vc := range viewConfigs {
		columns := make([]Column, len(vc.Columns))
		for j, cc := range vc.Columns {
			primaryStatus := extractPrimaryStatus(cc.Query)
			col := NewColumnWithExecutor(cc.Name, cc.Query, executor)
			col.status = primaryStatus
			if cc.Color != "" {
				col = col.SetColor(lipgloss.Color(cc.Color))
			}
			columns[j] = col
		}
		views[i] = View{
			name:    vc.Name,
			columns: columns,
			configs: vc.Columns,
			loaded:  false,
		}
	}

	// Default focus to second column (Ready equivalent) or first
	focusIdx := 0
	var columns []Column
	var configs []config.ColumnConfig
	if len(views) > 0 {
		columns = views[0].columns
		configs = views[0].configs
		if len(columns) > 1 {
			focusIdx = 1
		}
	}

	return Model{
		views:       views,
		currentView: 0,
		columns:     columns,
		configs:     configs,
		executor:    executor,
		focused:     focusIdx,
	}
}

// ColCount returns the number of columns.
func (m Model) ColCount() int {
	return len(m.columns)
}

// SetSize updates board dimensions.
func (m Model) SetSize(width, height int) Model {
	m.width = width
	m.height = height

	colCount := len(m.columns)
	if colCount == 0 {
		return m
	}

	contentWidth := width / colCount
	contentHeight := height

	for i := range m.columns {
		m.columns[i] = m.columns[i].SetSize(contentWidth, contentHeight)
	}
	return m
}

// SetShowCounts sets whether to display counts in column titles.
func (m Model) SetShowCounts(show bool) Model {
	for i := range m.columns {
		m.columns[i] = m.columns[i].SetShowCounts(show)
	}
	return m
}

// SelectedIssue returns the currently selected issue.
func (m Model) SelectedIssue() *beads.Issue {
	if m.focused < 0 || m.focused >= len(m.columns) {
		return nil
	}
	return m.columns[m.focused].SelectedItem()
}

// FocusedColumn returns the currently focused column index.
func (m Model) FocusedColumn() int {
	return m.focused
}

// SetFocus sets the focused column.
func (m Model) SetFocus(col int) Model {
	if col >= 0 && col < len(m.columns) {
		m.focused = col
	}
	return m
}

// SelectByID finds an issue by ID across all columns and selects it.
// Returns the model and true if found, false otherwise.
func (m Model) SelectByID(id string) (Model, bool) {
	// Search all columns for the issue
	for i := range m.columns {
		col, found := m.columns[i].SelectByID(id)
		if found {
			m.columns[i] = col
			m.focused = i
			return m, true
		}
	}
	return m, false
}

// Column returns the column at the given index.
func (m Model) Column(idx int) Column {
	if idx < 0 || idx >= len(m.columns) {
		return Column{}
	}
	return m.columns[idx]
}

// IsEmpty returns true if all columns have no items.
func (m Model) IsEmpty() bool {
	for _, col := range m.columns {
		if len(col.Items()) > 0 {
			return false
		}
	}
	return true
}

// CurrentViewName returns the name of the active view.
func (m Model) CurrentViewName() string {
	if m.currentView < len(m.views) {
		return m.views[m.currentView].name
	}
	return ""
}

// ViewCount returns the total number of configured views.
func (m Model) ViewCount() int {
	return len(m.views)
}

// CurrentViewIndex returns the 0-based index of the current view.
func (m Model) CurrentViewIndex() int {
	return m.currentView
}

// CycleViewNext moves to the next view (Shift+J).
func (m Model) CycleViewNext() (Model, tea.Cmd) {
	if len(m.views) <= 1 {
		return m, nil // Nothing to cycle
	}

	nextView := (m.currentView + 1) % len(m.views)
	return m.switchToView(nextView)
}

// CycleViewPrev moves to the previous view (Shift+K).
func (m Model) CycleViewPrev() (Model, tea.Cmd) {
	if len(m.views) <= 1 {
		return m, nil
	}

	prevView := m.currentView - 1
	if prevView < 0 {
		prevView = len(m.views) - 1
	}
	return m.switchToView(prevView)
}

// SwitchToView changes to the specified view index (public API).
func (m Model) SwitchToView(viewIndex int) (Model, tea.Cmd) {
	return m.switchToView(viewIndex)
}

// switchToView changes to the specified view index.
func (m Model) switchToView(viewIndex int) (Model, tea.Cmd) {
	if viewIndex < 0 || viewIndex >= len(m.views) {
		return m, nil
	}

	m.currentView = viewIndex
	m.columns = m.views[viewIndex].columns
	m.configs = m.views[viewIndex].configs
	m.focused = 0 // Reset focus to first column

	// Apply current dimensions to the new view's columns
	if m.width > 0 && m.height > 0 {
		m = m.SetSize(m.width, m.height)
		// Sync sized columns back to view
		m.views[viewIndex].columns = m.columns
	}

	// Load this view if not already loaded
	if !m.views[viewIndex].loaded {
		return m, m.LoadCurrentViewCmd()
	}

	return m, nil
}

// LoadCurrentViewCmd loads only the current view's columns.
func (m Model) LoadCurrentViewCmd() tea.Cmd {
	if len(m.views) == 0 || m.currentView >= len(m.views) {
		return nil
	}

	view := m.views[m.currentView]
	cmds := make([]tea.Cmd, 0, len(view.columns))

	for i := range m.columns {
		m.columns[i] = m.columns[i].SetLoading(true)
		if cmd := m.columns[i].LoadIssuesCmdForView(m.currentView); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// InvalidateViews resets the loaded flag for all views so they will reload
// when switched to. Call this when data has changed (refresh, issue updates).
func (m Model) InvalidateViews() Model {
	for i := range m.views {
		m.views[i].loaded = false
	}
	return m
}

// LoadAllColumns returns a batch of commands to load all columns.
// Each column will load its issues via BQL and send a ColumnLoadedMsg when done.
func (m Model) LoadAllColumns() tea.Cmd {
	var cmds []tea.Cmd
	for i := range m.columns {
		m.columns[i] = m.columns[i].SetLoading(true)
		// Use current view index so messages aren't filtered out
		if cmd := m.columns[i].LoadIssuesCmdForView(m.currentView); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ColumnLoadedMsg:
		// Only update if this message is for our current view (or no views configured)
		if len(m.views) > 0 && msg.ViewIndex != m.currentView {
			return m, nil // Ignore stale messages from other views
		}

		// Find the column by title and update it
		for i := range m.columns {
			if m.columns[i].title == msg.ColumnTitle {
				m.columns[i] = m.columns[i].SetLoading(false)
				if msg.Err != nil {
					// Store error in column (loadError field)
					m.columns[i].loadError = msg.Err
				} else {
					m.columns[i] = m.columns[i].SetItems(msg.Issues)
				}
				break
			}
		}

		// Mark view as loaded
		if len(m.views) > 0 && m.currentView < len(m.views) {
			m.views[m.currentView].loaded = true
			// Also sync columns back to view (columns slice is a copy)
			m.views[m.currentView].columns = m.columns
		}

		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "h", "left":
			if m.focused > 0 {
				m.focused--
			}
			return m, nil

		case "l", "right":
			if m.focused < len(m.columns)-1 {
				m.focused++
			}
			return m, nil

		case "j", "down", "k", "up":
			if m.focused >= 0 && m.focused < len(m.columns) {
				col := m.columns[m.focused]
				col, _ = col.Update(msg)
				m.columns[m.focused] = col
			}
			return m, nil
		}
	}
	return m, nil
}

// View renders the board.
func (m Model) View() string {
	// Handle empty columns case
	if len(m.columns) == 0 {
		return m.renderEmptyState()
	}

	var cols []string

	// Use height as-is - caller should account for status bar
	contentHeight := m.height
	if contentHeight < 3 {
		contentHeight = 3
	}

	for i, col := range m.columns {
		isFocused := i == m.focused

		// Set focused state for selection rendering
		col = col.SetFocused(isFocused)

		// Use column's own color
		colColor := col.Color()

		// Render column with bordered title
		rendered := styles.RenderWithTitleBorder(
			col.View(),
			col.Title(),
			col.width,
			contentHeight,
			isFocused,
			colColor,
			colColor,
		)
		cols = append(cols, rendered)
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, cols...)
}

// renderEmptyState renders a centered message when no columns are configured.
func (m Model) renderEmptyState() string {
	emptyStyle := lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Align(lipgloss.Center, lipgloss.Center)

	messageStyle := lipgloss.NewStyle().
		Foreground(styles.TextMutedColor).
		Italic(true)

	hintStyle := lipgloss.NewStyle().
		Foreground(styles.TextPrimaryColor)

	content := messageStyle.Render("No columns configured") + "\n\n" +
		hintStyle.Render("Press 'a' to add a column")

	return emptyStyle.Render(content)
}

// extractPrimaryStatus extracts the primary status from a BQL query.
// This is used for column rendering hints.
func extractPrimaryStatus(query string) beads.Status {
	query = strings.ToLower(query)
	if strings.Contains(query, "status = open") {
		return beads.StatusOpen
	}
	if strings.Contains(query, "status = in_progress") {
		return beads.StatusInProgress
	}
	if strings.Contains(query, "status = closed") {
		return beads.StatusClosed
	}
	return ""
}
