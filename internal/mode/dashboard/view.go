package dashboard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"

	"github.com/zjrosen/perles/internal/orchestration/controlplane"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/ui/shared/panes"
	"github.com/zjrosen/perles/internal/ui/shared/table"
	"github.com/zjrosen/perles/internal/ui/styles"
)

// Color constants for status and health indicators.
var (
	colorRunning   = lipgloss.Color("#00BFFF") // Blue
	colorPending   = lipgloss.Color("#808080") // Gray (per spec)
	colorPaused    = lipgloss.Color("#FFFF00") // Yellow (per spec)
	colorCompleted = lipgloss.Color("#00FF00") // Green (per spec)
	colorFailed    = lipgloss.Color("#FF0000") // Red
	colorStopped   = lipgloss.Color("#808080") // Gray
	colorDimmed    = lipgloss.Color("#666666") // Dimmed text
	colorHeader    = lipgloss.Color("#FFFFFF") // White for headers
)

// Status text labels for workflow states.
const (
	statusRunning   = "RUNNING"
	statusPending   = "PENDING"
	statusPaused    = "PAUSED"
	statusCompleted = "COMPLETED"
	statusFailed    = "FAILED"
	statusStopped   = "STOPPED"
)

// CoordinatorPanelWidth is the fixed width for the coordinator chat panel.
const CoordinatorPanelWidth = 60

// createWorkflowTableConfig creates the table configuration for the workflow list.
// The render callbacks close over the model to access controlPlane and services.Clock.
func (m Model) createWorkflowTableConfig() table.TableConfig {
	return table.TableConfig{
		Columns: []table.ColumnConfig{
			{
				Key:    "notify",
				Header: "🔔",
				Width:  2, // Bell emoji is 2 characters wide
				Type:   table.ColumnTypeIcon,
				Render: func(row any, _ string, _ int, _ bool) string {
					r := row.(WorkflowTableRow)
					if r.HasNotification {
						return lipgloss.NewStyle().
							Foreground(lipgloss.AdaptiveColor{Light: "#FF6B6B", Dark: "#FFD700"}).
							Render("🔔")
					}
					return "  " // Two spaces to match column width
				},
			},
			{
				Key:    "status",
				Header: "Status",
				Width:  9,
				Type:   table.ColumnTypeText,
				Render: func(row any, _ string, _ int, _ bool) string {
					r := row.(WorkflowTableRow)
					text, color := getStatusTextAndColor(r.Workflow.State)
					return lipgloss.NewStyle().Foreground(color).Render(text)
				},
			},
			{
				Key:      "name",
				Header:   "Name",
				MinWidth: 10,
				Type:     table.ColumnTypeText,
				Render: func(row any, _ string, w int, _ bool) string {
					r := row.(WorkflowTableRow)
					name := r.Workflow.Name
					if lipgloss.Width(name) > w {
						name = styles.TruncateString(name, w)
					}
					return name
				},
			},
			{
				Key:       "epicid",
				Header:    "EpicID",
				Width:     16,
				Type:      table.ColumnTypeText,
				HideBelow: 100, // Hide when table width < 100 (e.g., coordinator panel open)
				Render: func(row any, _ string, w int, _ bool) string {
					r := row.(WorkflowTableRow)
					epicID := r.Workflow.EpicID
					if epicID == "" {
						return "-"
					}
					if lipgloss.Width(epicID) > w {
						return styles.TruncateString(epicID, w)
					}
					return epicID
				},
			},
			{
				Key:       "workdir",
				Header:    "WorkDir",
				Width:     23,
				Type:      table.ColumnTypeText,
				HideBelow: 120, // Hide when table width < 120 (e.g., coordinator panel open)
				Render: func(row any, _ string, w int, _ bool) string {
					r := row.(WorkflowTableRow)
					wf := r.Workflow

					// Show worktree with tree icon
					if wf.WorktreePath != "" {
						display := "🌳 " + filepath.Base(wf.WorktreePath)
						if lipgloss.Width(display) > w {
							return styles.TruncateString(display, w)
						}
						return display
					}

					// Show custom workdir (not current directory)
					if wf.WorkDir != "" {
						cwd, _ := os.Getwd()
						if wf.WorkDir != cwd {
							display := filepath.Base(wf.WorkDir)
							if lipgloss.Width(display) > w {
								return styles.TruncateString(display, w)
							}
							return display
						}
					}

					// Using current directory
					return "·" // Middle dot for current directory (minimal noise)
				},
			},
			{
				Key:    "workers",
				Header: "Workers",
				Width:  8,
				Type:   table.ColumnTypeNumber,
				Render: func(row any, _ string, _ int, _ bool) string {
					r := row.(WorkflowTableRow)
					return fmt.Sprintf("%d", r.Workflow.ActiveWorkers)
				},
			},
			// TODO: Re-enable tokens column once token tracking is implemented
			// {
			// 	Key:    "tokens",
			// 	Header: "Tokens",
			// 	Width:  8,
			// 	Type:   table.ColumnTypeText,
			// 	Render: func(row any, _ string, _ int, _ bool) string {
			// 		r := row.(WorkflowTableRow)
			// 		return formatTokenCount(r.Workflow.TokensUsed)
			// 	},
			// },
			{
				Key:    "health",
				Header: "Health",
				Width:  8,
				Type:   table.ColumnTypeText,
				Render: func(row any, _ string, _ int, _ bool) string {
					r := row.(WorkflowTableRow)
					return m.getHealthDisplay(r.Workflow)
				},
			},
			{
				Key:    "uptime",
				Header: "Uptime",
				Width:  8,
				Type:   table.ColumnTypeText,
				Render: func(row any, _ string, _ int, _ bool) string {
					r := row.(WorkflowTableRow)
					return m.getUptimeDisplay(r.Workflow)
				},
			},
			{
				Key:       "started",
				Header:    "Started",
				Width:     14, // "01/02 03:04PM" = 13 chars + 1 padding
				Type:      table.ColumnTypeDate,
				HideBelow: 110, // Hide when table width < 110 (e.g., coordinator panel open)
				Render: func(row any, _ string, _ int, _ bool) string {
					r := row.(WorkflowTableRow)
					return m.getStartedDisplay(r.Workflow)
				},
			},
		},
		ShowHeader:         true,
		ShowBorder:         true,
		Scrollable:         true,
		EmptyMessage:       m.getEmptyMessage(),
		BorderColor:        styles.BorderDefaultColor,
		Focused:            m.focus == FocusTable,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
		Title:              m.getTableTitle(),
		RowZoneID: func(index int, _ any) string {
			return makeWorkflowZoneID(index)
		},
	}
}

// getEmptyMessage returns the appropriate empty state message.
func (m Model) getEmptyMessage() string {
	if m.filter.HasFilter() {
		return "No workflows match the filter. Press Esc to clear."
	}
	return "No workflows yet. Press 'n' to create one, or use the API."
}

// getTableTitle returns the title for the workflow table including API port.
func (m Model) getTableTitle() string {
	if m.apiPort > 0 {
		return fmt.Sprintf("Workflows · API ::%d", m.apiPort)
	}
	return "Workflows"
}

// renderView renders the complete dashboard view.
// This is a pure render function - it does not mutate model state.
func (m Model) renderView() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	// Footer section (action hints)
	footer := m.renderActionHints()

	// Calculate heights
	headerHeight := 0
	footerHeight := lipgloss.Height(footer)
	contentHeight := max(m.height-headerHeight-footerHeight, 5)

	// Calculate table/epic section heights
	// Only show epic section if there are workflows
	var tableHeight, epicSectionHeight int
	if len(m.workflows) == 0 {
		// No workflows - table takes full height (shows empty state)
		tableHeight = contentHeight
		epicSectionHeight = 0
	} else {
		// 45%/55% split (table/epic), but ensure minimum 6 rows for table
		// Table needs: 6 data rows + 3 (header + borders) = 9 minimum
		minTableHeight := minWorkflowTableRows + 3

		// Calculate 45% for table, 55% for epic section
		tableHeight = max(contentHeight*45/100, minTableHeight)
		epicSectionHeight = contentHeight - tableHeight
		if epicSectionHeight < 5 {
			// Not enough room for epic section - don't show it
			tableHeight = contentHeight
			epicSectionHeight = 0
		}
	}

	var mainContent string

	// Check if coordinator panel is visible
	if m.showCoordinatorPanel && m.coordinatorPanel != nil {
		// Split layout: workflow table on left, coordinator panel on right
		panelWidth := CoordinatorPanelWidth
		tableWidth := m.width - panelWidth

		// Render workflow table (narrower)
		tableView := m.renderBorderedWorkflowTable(tableWidth, tableHeight)

		// Render coordinator panel - it spans the full content height
		m.coordinatorPanel.SetSize(panelWidth, contentHeight)
		panelView := m.coordinatorPanel.View()

		// Build left column: table + epic section
		var leftColumn string
		if epicSectionHeight > 0 {
			epicSection := m.renderEpicSection(tableWidth, epicSectionHeight)
			leftColumn = lipgloss.JoinVertical(lipgloss.Left, tableView, epicSection)
		} else {
			leftColumn = tableView
		}

		// Join horizontally
		mainContent = lipgloss.JoinHorizontal(lipgloss.Top, leftColumn, panelView)
	} else {
		// Full width workflow table
		tableView := m.renderBorderedWorkflowTable(m.width, tableHeight)

		if epicSectionHeight > 0 {
			epicSection := m.renderEpicSection(m.width, epicSectionHeight)
			mainContent = lipgloss.JoinVertical(lipgloss.Left, tableView, epicSection)
		} else {
			mainContent = tableView
		}
	}

	// Compose the layout with JoinVertical
	view := lipgloss.JoinVertical(lipgloss.Left, mainContent, footer)

	// Use Place to position content in a fixed-size container
	// This ensures the layout fills the entire terminal with footer at bottom
	return lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, view)
}

// renderBorderedWorkflowTable renders the workflow table inside a bordered pane.
// This is a pure render function - it does not mutate model state.
// All state updates (scroll offset, config caching) must happen in Update().
func (m Model) renderBorderedWorkflowTable(width, height int) string {
	filtered := m.getFilteredWorkflows()

	// Convert workflows to table rows
	rows := make([]any, len(filtered))
	for i, wf := range filtered {
		// Check if this workflow has a pending notification
		hasNotification := false
		if uiState, exists := m.workflowUIState[wf.ID]; exists {
			hasNotification = uiState.HasNotification
		}
		rows[i] = WorkflowTableRow{
			Index:           i + 1,
			Workflow:        wf,
			HasNotification: hasNotification,
		}
	}

	// Build table for rendering (no mutation - use cached config from Update)
	// Note: EnsureVisible is called in Update when selection changes, not here
	tbl := m.workflowTable.
		SetConfig(m.tableConfigCache).
		SetRows(rows).
		SetSize(width, height).
		SetYOffset(m.tableScrollOffset)

	// Wrap in zone for mouse scroll detection
	return zone.Mark(zoneWorkflowTable, tbl.ViewWithSelection(m.selectedIndex))
}

// renderActionHints renders the quick action hints bar in a bordered pane.
// Shows a static set of hints that apply across all focus zones.
func (m Model) renderActionHints() string {
	hintStyle := lipgloss.NewStyle().Foreground(colorDimmed)
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(colorHeader)

	// Static hints that are always relevant
	hints := []string{
		fmt.Sprintf("%s nav", keyStyle.Render("j/k")),
		fmt.Sprintf("%s cycle", keyStyle.Render("tab")),
		fmt.Sprintf("%s new", keyStyle.Render("n")),
		fmt.Sprintf("%s start", keyStyle.Render("s")),
		fmt.Sprintf("%s stop", keyStyle.Render("x")),
		fmt.Sprintf("%s help", keyStyle.Render("?")),
		fmt.Sprintf("%s quit", keyStyle.Render("q")),
	}

	content := hintStyle.Render(strings.Join(hints, "  "))

	return panes.BorderedPane(panes.BorderConfig{
		Content:     content,
		Width:       m.width,
		Height:      3, // 1 line content + 2 for borders
		Focused:     false,
		BorderColor: styles.BorderDefaultColor,
	})
}

// getStatusTextAndColor returns the appropriate status text and color for a workflow state.
func getStatusTextAndColor(state controlplane.WorkflowState) (string, lipgloss.TerminalColor) {
	switch state {
	case controlplane.WorkflowRunning:
		return statusRunning, colorRunning
	case controlplane.WorkflowPending:
		return statusPending, colorPending
	case controlplane.WorkflowPaused:
		return statusPaused, colorPaused
	case controlplane.WorkflowCompleted:
		return statusCompleted, colorCompleted
	case controlplane.WorkflowFailed:
		return statusFailed, colorFailed
	case controlplane.WorkflowStopped:
		return statusStopped, colorStopped
	default:
		return statusPending, colorDimmed
	}
}

// getHealthDisplay returns the health display string for a workflow.
func (m Model) getHealthDisplay(wf *controlplane.WorkflowInstance) string {
	// Only show heartbeat for running workflows
	if !wf.IsRunning() {
		return "-"
	}

	// Query HealthMonitor for authoritative health status
	if m.controlPlane == nil {
		return "-"
	}

	status, ok := m.controlPlane.GetHealthStatus(wf.ID)
	if !ok {
		// Not tracked by HealthMonitor yet
		return "-"
	}

	// Get current time from clock (or use time.Now() if no clock configured)
	now := time.Now()
	if m.services.Clock != nil {
		now = m.services.Clock.Now()
	}

	elapsed := now.Sub(status.LastHeartbeatAt)

	if status.IsHealthy {
		return fmt.Sprintf("❤️ %s", formatDuration(elapsed))
	}

	// Unhealthy - exceeded timeout, show elapsed time since last heartbeat
	return fmt.Sprintf("💀 %s", formatDuration(elapsed))
}

// getUptimeDisplay returns the uptime display string for a workflow.
func (m Model) getUptimeDisplay(wf *controlplane.WorkflowInstance) string {
	if wf.StartedAt == nil {
		return "-"
	}

	// Get current time from clock (or use time.Now() if no clock configured)
	now := time.Now()
	if m.services.Clock != nil {
		now = m.services.Clock.Now()
	}

	elapsed := now.Sub(*wf.StartedAt)
	return formatDuration(elapsed)
}

// getStartedDisplay returns the started time display string for a workflow.
func (m Model) getStartedDisplay(wf *controlplane.WorkflowInstance) string {
	if wf.StartedAt == nil {
		return "-"
	}

	return wf.StartedAt.Format("01/02 03:04PM")
}

// phaseShortName returns a short display name for a workflow phase.
// Currently unused because WorkflowInstance doesn't expose Phase yet.
//
//nolint:unused // Retained for future use when Phase is exposed.
func phaseShortName(phase events.ProcessPhase) string {
	switch phase {
	case events.ProcessPhaseImplementing:
		return "impl"
	case events.ProcessPhaseAwaitingReview:
		return "await"
	case events.ProcessPhaseReviewing:
		return "review"
	case events.ProcessPhaseAddressingFeedback:
		return "feedback"
	case events.ProcessPhaseCommitting:
		return "commit"
	case events.ProcessPhaseIdle:
		return ""
	default:
		return ""
	}
}

// formatDuration formats a duration as a compact string like "1m", "30s", "2h".
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}

// minEpicSectionWidth is the minimum width required to render the epic section.
// Below this threshold, a warning message is shown instead of the tree/details panes.
const minEpicSectionWidth = 50

// minWorkflowTableRows is the minimum number of rows to show in the workflow table.
// This ensures the table remains usable even when the epic section is visible.
const minWorkflowTableRows = 6

// renderEpicSection renders the epic tree+details section below the workflow table.
// It handles empty states (no epic, empty tree, loading) and the 40%/60% horizontal split.
func (m Model) renderEpicSection(width, height int) string {
	// Check minimum width threshold
	if width < minEpicSectionWidth {
		emptyStyle := lipgloss.NewStyle().
			Foreground(colorDimmed).
			Italic(true).
			PaddingLeft(1)
		return emptyStyle.Render("Terminal too narrow for tree view (need 50+ chars)")
	}

	// Handle no epic associated with workflow
	wf := m.SelectedWorkflow()
	if wf == nil || wf.EpicID == "" {
		emptyStyle := lipgloss.NewStyle().
			Foreground(colorDimmed).
			Italic(true).
			PaddingLeft(1)
		return emptyStyle.Render("No epic associated with this workflow")
	}

	// Handle no tree loaded yet (tree loads fast, so just show empty state briefly)
	if m.epicTree == nil || m.epicTree.Root() == nil {
		emptyStyle := lipgloss.NewStyle().
			Foreground(colorDimmed).
			Italic(true).
			PaddingLeft(1)
		return emptyStyle.Render("No tasks in epic")
	}

	// Check if tree has only root with no children
	root := m.epicTree.Root()
	if len(root.Children) == 0 {
		emptyStyle := lipgloss.NewStyle().
			Foreground(colorDimmed).
			Italic(true).
			PaddingLeft(1)
		return emptyStyle.Render("Epic has no child tasks yet")
	}

	// Minimum width threshold to show details pane (tree + details needs at least 80 chars)
	const minWidthForDetails = 80

	// Determine if we should show details pane based on available width
	showDetails := width >= minWidthForDetails

	var treeWidth, detailsWidth int
	if showDetails {
		// Calculate widths: 40% tree, 60% details
		treeWidth = width * 40 / 100
		detailsWidth = width - treeWidth

		// Ensure minimum widths
		if treeWidth < 20 {
			treeWidth = 20
			detailsWidth = width - treeWidth
		}
		if detailsWidth < 20 {
			detailsWidth = 20
			treeWidth = width - detailsWidth
		}
	} else {
		// Narrow width: only show tree pane
		treeWidth = width
		detailsWidth = 0
	}

	// Set sizes for tree
	m.epicTree.SetSize(treeWidth-2, height-2) // -2 for borders

	// Render tree pane with border
	treeContent := m.epicTree.View()
	treePaneStyle := m.getEpicPaneBorderConfig(EpicFocusTree, treeWidth, height, "Epic")

	// Calculate progress bar for tree pane
	var progressBar string
	if m.epicTree.Root() != nil {
		closed, total := m.epicTree.Root().CalculateProgress()
		progressBar = renderCompactProgress(closed, total)
	}

	treePane := zone.Mark(zoneEpicTree, panes.BorderedPane(panes.BorderConfig{
		Content:            treeContent,
		Width:              treePaneStyle.width,
		Height:             treePaneStyle.height,
		TopLeft:            treePaneStyle.title,
		TopRight:           progressBar,
		Focused:            treePaneStyle.focused,
		TitleColor:         styles.OverlayTitleColor,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
	}))

	// If not showing details, just return the tree pane
	if !showDetails {
		return treePane
	}

	// Set details size and render
	if m.hasEpicDetail {
		m.epicDetails = m.epicDetails.SetSize(detailsWidth-2, height-2)
	}

	// Render details pane with border
	var detailsContent string
	if m.hasEpicDetail {
		detailsContent = m.epicDetails.View()
	} else {
		emptyStyle := lipgloss.NewStyle().
			Foreground(colorDimmed).
			Italic(true).
			PaddingLeft(1)
		detailsContent = emptyStyle.Render("Select an issue to view details")
	}
	detailsPaneStyle := m.getEpicPaneBorderConfig(EpicFocusDetails, detailsWidth, height, "Details")

	detailsPane := zone.Mark(zoneEpicDetails, panes.BorderedPane(panes.BorderConfig{
		Content:            detailsContent,
		Width:              detailsPaneStyle.width,
		Height:             detailsPaneStyle.height,
		TopLeft:            detailsPaneStyle.title,
		Focused:            detailsPaneStyle.focused,
		TitleColor:         styles.OverlayTitleColor,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
	}))

	return lipgloss.JoinHorizontal(lipgloss.Top, treePane, detailsPane)
}

// epicPaneBorderConfig holds the configuration for a bordered pane in the epic section.
type epicPaneBorderConfig struct {
	width   int
	height  int
	title   string
	focused bool
}

// getEpicPaneBorderConfig returns the border configuration for an epic view pane.
// The focused state is determined by comparing the pane's focus enum with the current epicViewFocus.
func (m Model) getEpicPaneBorderConfig(pane EpicViewFocus, width, height int, title string) epicPaneBorderConfig {
	// Pane is focused only when:
	// 1. Dashboard focus is on the epic view
	// 2. The specific pane within epic view matches
	focused := m.focus == FocusEpicView && m.epicViewFocus == pane

	return epicPaneBorderConfig{
		width:   width,
		height:  height,
		title:   title,
		focused: focused,
	}
}

// renderCompactProgress renders a compact progress bar with percentage and counts.
func renderCompactProgress(closed, total int) string {
	if total == 0 {
		return ""
	}
	percent := float64(closed) / float64(total) * 100
	barWidth := 10
	filledWidth := int(float64(barWidth) * float64(closed) / float64(total))

	filledStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)
	emptyStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)

	filled := filledStyle.Render(strings.Repeat("█", filledWidth))
	empty := emptyStyle.Render(strings.Repeat("░", barWidth-filledWidth))

	return fmt.Sprintf("%s%s %.0f%% (%d/%d)", filled, empty, percent, closed, total)
}

// ResourceSummary holds aggregated resource statistics.
type ResourceSummary struct {
	TotalWorkflows   int
	RunningWorkflows int
	ActiveWorkers    int
	TotalTokens      int64
	TotalAICalls     int
}

// NewResourceSummary creates a new empty resource summary.
func NewResourceSummary() ResourceSummary {
	return ResourceSummary{}
}

// Update recalculates the resource summary from the workflow list.
func (s ResourceSummary) Update(workflows []*controlplane.WorkflowInstance) ResourceSummary {
	s.TotalWorkflows = len(workflows)
	s.RunningWorkflows = 0
	s.ActiveWorkers = 0
	s.TotalTokens = 0

	for _, wf := range workflows {
		if wf.IsRunning() {
			s.RunningWorkflows++
		}
		s.ActiveWorkers += wf.ActiveWorkers
		s.TotalTokens += wf.TokensUsed
	}

	return s
}
