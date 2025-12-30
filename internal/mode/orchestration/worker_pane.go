package orchestration

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/pool"
	"github.com/zjrosen/perles/internal/ui/shared/panes"
	"github.com/zjrosen/perles/internal/ui/styles"
)

// Worker pane styles
var (
	workerReadyStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#73F59F"}) // Green like open issues

	workerWorkingStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#54A0FF", Dark: "#54A0FF"}) // Blue - actively working

	workerRetiredStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#FF6B6B", Dark: "#FF8787"}) // Red - retired

	workerMessageStyle = lipgloss.NewStyle().
				Foreground(WorkerColor)
)

// maxRetiredWorkerViewports is the maximum number of retired worker viewports to keep.
// Older retired workers' viewports are cleaned up to prevent unbounded memory growth.
const maxRetiredWorkerViewports = 5

// cleanupRetiredWorkerViewports removes viewports for the oldest retired workers
// when we have more than maxRetiredWorkerViewports retired workers tracked.
// This prevents unbounded memory growth from accumulating retired worker data.
func (m Model) cleanupRetiredWorkerViewports() Model {
	if len(m.workerPane.retiredOrder) <= maxRetiredWorkerViewports {
		return m
	}

	// Remove oldest retired workers (those at the beginning of retiredOrder)
	toRemove := len(m.workerPane.retiredOrder) - maxRetiredWorkerViewports
	for i := range toRemove {
		workerID := m.workerPane.retiredOrder[i]
		delete(m.workerPane.viewports, workerID)
		delete(m.workerPane.contentDirty, workerID)
		delete(m.workerPane.hasNewContent, workerID)
		delete(m.workerPane.workerMessages, workerID)
		delete(m.workerPane.workerMetrics, workerID)
		delete(m.workerPane.workerStatus, workerID)
	}

	// Keep only the newest retired workers
	m.workerPane.retiredOrder = m.workerPane.retiredOrder[toRemove:]

	return m
}

// statusIndicator returns the status indicator character and style for a worker status.
func statusIndicator(status pool.WorkerStatus) (string, lipgloss.Style) {
	switch status {
	case pool.WorkerReady:
		return "○", workerReadyStyle // Green circle - ready/available
	case pool.WorkerWorking:
		return "●", workerWorkingStyle // Blue filled circle - actively working
	case pool.WorkerRetired:
		return "✗", workerRetiredStyle // Red X - retired
	default:
		return "?", workerReadyStyle
	}
}

// phaseShortName returns a short display name for a workflow phase.
// Used in pane titles to keep them compact.
func phaseShortName(phase events.WorkerPhase) string {
	switch phase {
	case events.PhaseImplementing:
		return "impl"
	case events.PhaseAwaitingReview:
		return "await"
	case events.PhaseReviewing:
		return "review"
	case events.PhaseAddressingFeedback:
		return "feedback"
	case events.PhaseCommitting:
		return "commit"
	case events.PhaseIdle:
		return "" // No display for idle
	default:
		return "" // Unknown phases get no display
	}
}

// formatWorkerTitle builds the left title for a worker pane.
// Format: "● WORKER-1 perles-abc.1 (impl)" when working with task,
// or "○ WORKER-1" when idle/ready.
func formatWorkerTitle(workerID string, status pool.WorkerStatus, taskID string, phase events.WorkerPhase) string {
	indicator, indicatorStyle := statusIndicator(status)

	// Base title: "● WORKER-1"
	title := fmt.Sprintf("%s %s", indicatorStyle.Render(indicator), strings.ToUpper(workerID))

	// Add task context if present
	if taskID != "" {
		title += " " + TitleContextStyle.Render(taskID)

		// Add phase in parentheses if not idle
		if shortName := phaseShortName(phase); shortName != "" {
			title += " " + TitleContextStyle.Render("("+shortName+")")
		}
	}

	return title
}

// renderWorkerPanes renders all worker panes stacked vertically.
// Each worker gets its own bordered pane with title.
// Retired workers are filtered out.
func (m Model) renderWorkerPanes(width, height int) string {
	activeWorkerIDs := m.ActiveWorkerIDs()

	if len(activeWorkerIDs) == 0 {
		// No active workers - show placeholder pane
		return m.renderEmptyWorkerPane(width, height)
	}

	// Calculate height per worker pane (minimum for border + content)
	heightPerWorker := max(height/len(activeWorkerIDs), minHeightPerWorker)

	// Render each active worker as its own pane
	var panes []string
	for i, workerID := range activeWorkerIDs {
		// Last pane gets remaining height to avoid gaps
		paneHeight := heightPerWorker
		if i == len(activeWorkerIDs)-1 {
			paneHeight = height - (heightPerWorker * i)
		}

		pane := m.renderSingleWorkerPane(workerID, width, paneHeight)
		panes = append(panes, pane)
	}

	// Stack panes vertically
	return lipgloss.JoinVertical(lipgloss.Left, panes...)
}

// renderEmptyWorkerPane renders a centered placeholder when no workers exist.
func (m Model) renderEmptyWorkerPane(width, height int) string {
	msg := "No workers spawned yet"
	styledMsg := lipgloss.NewStyle().
		Foreground(styles.TextSecondaryColor).
		Render(msg)

	// Center vertically and horizontally
	msgWidth := lipgloss.Width(styledMsg)
	leftPad := max((width-msgWidth)/2, 0)
	topPad := height / 2

	var lines []string
	for range topPad {
		lines = append(lines, "")
	}
	lines = append(lines, strings.Repeat(" ", leftPad)+styledMsg)

	// Pad remaining height
	for len(lines) < height {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

// renderSingleWorkerPane renders a single worker's pane with its own border and title.
func (m Model) renderSingleWorkerPane(workerID string, width, height int) string {
	status := m.workerPane.workerStatus[workerID]

	// Get or create viewport for this worker
	vp, exists := m.workerPane.viewports[workerID]
	if !exists {
		vpWidth := max(width-2, 1)
		vpHeight := max(height-2, 1)
		vp = viewport.New(vpWidth, vpHeight)
	}

	// Get task ID and phase from pool (if available)
	var taskID string
	var phase events.WorkerPhase
	if m.pool != nil {
		if worker := m.pool.GetWorker(workerID); worker != nil {
			taskID = worker.GetTaskID()
			phase = worker.GetPhase()
		}
	}

	// Build title with task context: "● WORKER-1 perles-abc.1 (impl)"
	leftTitle := formatWorkerTitle(workerID, status, taskID, phase)

	// Build metrics display for right title
	var metricsDisplay string
	if workerMetrics, ok := m.workerPane.workerMetrics[workerID]; ok && workerMetrics != nil && workerMetrics.ContextTokens > 0 {
		metricsDisplay = workerMetrics.FormatContextDisplay()
	}

	// Build bottom-left queue indicator
	var bottomLeft string
	if queueCount := m.workerPane.workerQueueCounts[workerID]; queueCount > 0 {
		bottomLeft = QueuedCountStyle.Render(fmt.Sprintf("[%d queued]", queueCount))
	}

	// Use panes.ScrollablePane helper for viewport setup, padding, and auto-scroll
	result := panes.ScrollablePane(width, height, panes.ScrollableConfig{
		Viewport:       &vp,
		ContentDirty:   m.workerPane.contentDirty[workerID],
		HasNewContent:  m.workerPane.hasNewContent[workerID],
		MetricsDisplay: metricsDisplay,
		LeftTitle:      leftTitle,
		BottomLeft:     bottomLeft,
		TitleColor:     WorkerColor,
		BorderColor:    styles.BorderDefaultColor,
	}, func(wrapWidth int) string {
		return m.renderWorkerContent(workerID, wrapWidth, 0)
	})

	// Store updated viewport back to map (helper modified via pointer)
	m.workerPane.viewports[workerID] = vp

	return result
}

// renderFullscreenWorkerPane renders a single worker pane in fullscreen mode.
func (m Model) renderFullscreenWorkerPane(width, height int) string {
	activeWorkerIDs := m.ActiveWorkerIDs()

	// Validate index
	if m.fullscreenWorkerIndex >= len(activeWorkerIDs) {
		return m.renderEmptyWorkerPane(width, height)
	}

	workerID := activeWorkerIDs[m.fullscreenWorkerIndex]

	// Render single worker pane at full dimensions
	return m.renderSingleWorkerPane(workerID, width, height)
}

// renderWorkerContent builds the pre-wrapped content string for the viewport.
func (m Model) renderWorkerContent(workerID string, wrapWidth, _ int) string {
	messages := m.workerPane.workerMessages[workerID]

	if len(messages) == 0 {
		// Waiting placeholder
		return lipgloss.NewStyle().Foreground(styles.TextSecondaryColor).Render("  Waiting for output...")
	}

	return renderChatContent(messages, wrapWidth, ChatRenderConfig{
		AgentLabel:              "Worker",
		AgentColor:              workerMessageStyle.GetForeground().(lipgloss.AdaptiveColor),
		ShowCoordinatorInWorker: true,
	})
}
