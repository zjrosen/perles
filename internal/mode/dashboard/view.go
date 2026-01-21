package dashboard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/zjrosen/perles/internal/orchestration/controlplane"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/ui/shared/panes"
	"github.com/zjrosen/perles/internal/ui/shared/table"
	"github.com/zjrosen/perles/internal/ui/styles"
)

// Color constants for status and health indicators.
var (
	colorRunning   = lipgloss.Color("#00FF00") // Green
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

// createWorkflowTableConfig creates the table configuration for the workflow list.
// The render callbacks close over the model to access controlPlane and services.Clock.
func (m Model) createWorkflowTableConfig() table.TableConfig {
	return table.TableConfig{
		Columns: []table.ColumnConfig{
			{
				Key:    "index",
				Header: "#",
				Width:  3,
				Type:   table.ColumnTypeNumber,
				Render: func(row any, _ string, _ int, _ bool) string {
					r := row.(WorkflowTableRow)
					return fmt.Sprintf("%d", r.Index)
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
				Key:      "workdir",
				Header:   "WorkDir",
				MinWidth: 15,
				Type:     table.ColumnTypeText,
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
			{
				Key:    "tokens",
				Header: "Tokens",
				Width:  8,
				Type:   table.ColumnTypeText,
				Render: func(row any, _ string, _ int, _ bool) string {
					r := row.(WorkflowTableRow)
					return formatTokenCount(r.Workflow.TokensUsed)
				},
			},
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
				Key:    "started",
				Header: "Started",
				Width:  14, // "01/02 03:04PM" = 13 chars + 1 padding
				Type:   table.ColumnTypeDate,
				Render: func(row any, _ string, _ int, _ bool) string {
					r := row.(WorkflowTableRow)
					return m.getStartedDisplay(r.Workflow)
				},
			},
			{
				Key:      "epicid",
				Header:   "EpicID",
				MinWidth: 8,
				Type:     table.ColumnTypeText,
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
		},
		ShowHeader:   true,
		ShowBorder:   true,
		EmptyMessage: "No workflows match the filter. Press Esc to clear.",
		BorderColor:  styles.BorderDefaultColor,
	}
}

// renderView renders the complete dashboard view.
func (m *Model) renderView() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	// Check for empty state - render centered text instead of bordered pane
	filteredWorkflows := m.getFilteredWorkflows()
	if len(filteredWorkflows) == 0 && !m.filter.HasFilter() {
		return m.renderEmptyState()
	}

	// Footer section (action hints)
	footer := m.renderActionHints()

	// Calculate heights
	headerHeight := 0
	footerHeight := lipgloss.Height(footer)
	contentHeight := max(m.height-headerHeight-footerHeight, 5)

	// Content section - bordered workflow table
	content := m.renderBorderedWorkflowTable(m.width, contentHeight)

	// Compose the layout with JoinVertical
	view := lipgloss.JoinVertical(lipgloss.Left, content, footer)

	// Use Place to position content in a fixed-size container
	// This ensures the layout fills the entire terminal with footer at bottom
	return lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, view)
}

// renderBorderedWorkflowTable renders the workflow table inside a bordered pane.
func (m Model) renderBorderedWorkflowTable(width, height int) string {
	filtered := m.getFilteredWorkflows()

	// Convert workflows to table rows
	rows := make([]any, len(filtered))
	for i, wf := range filtered {
		rows[i] = WorkflowTableRow{
			Index:    i + 1,
			Workflow: wf,
		}
	}

	// Create fresh table config to capture current model state in render closures
	tbl := table.New(m.createWorkflowTableConfig()).
		SetRows(rows).
		SetSize(width, height)

	return tbl.ViewWithSelection(m.selectedIndex)
}

// renderEmptyState renders the empty state message centered on screen.
func (m Model) renderEmptyState() string {
	emptyStyle := lipgloss.NewStyle().
		Foreground(colorDimmed)

	text := emptyStyle.Render("No workflows yet. Press 'n' to create a new workflow.")

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, text)
}

// renderActionHints renders the quick action hints bar in a bordered pane.
func (m Model) renderActionHints() string {
	hintStyle := lipgloss.NewStyle().Foreground(colorDimmed)
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(colorHeader)

	hints := []string{
		fmt.Sprintf("%s stop", keyStyle.Render("[x]")),
		fmt.Sprintf("%s new", keyStyle.Render("[n]")),
		fmt.Sprintf("%s help", keyStyle.Render("[?]")),
		fmt.Sprintf("%s quit", keyStyle.Render("[q]")),
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

// formatTokenCount formats a token count for display.
func formatTokenCount(tokens int64) string {
	if tokens == 0 {
		return "-"
	}
	if tokens < 1000 {
		return fmt.Sprintf("%d", tokens)
	}
	if tokens < 1000000 {
		return fmt.Sprintf("%.1fK", float64(tokens)/1000)
	}
	return fmt.Sprintf("%.1fM", float64(tokens)/1000000)
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
