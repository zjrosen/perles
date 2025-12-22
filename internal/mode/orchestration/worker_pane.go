package orchestration

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"perles/internal/orchestration/pool"
	"perles/internal/ui/styles"
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

// renderWorkerPanes renders all worker panes stacked vertically.
// Each worker gets its own bordered pane with title.
// Retired workers are filtered out.
func (m Model) renderWorkerPanes(width, height int) string {
	// Filter out retired workers
	var activeWorkerIDs []string
	for _, workerID := range m.workerPane.workerIDs {
		status := m.workerPane.workerStatus[workerID]
		if status != pool.WorkerRetired {
			activeWorkerIDs = append(activeWorkerIDs, workerID)
		}
	}

	if len(activeWorkerIDs) == 0 {
		// No active workers - show placeholder pane
		return m.renderEmptyWorkerPane(width, height)
	}

	// Calculate height per worker pane (minimum 5 lines for border + content)
	minHeightPerWorker := 3
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
	indicator, indicatorStyle := statusIndicator(status)

	// Calculate viewport dimensions (subtract 2 for borders)
	vpWidth := max(width-2, 1)
	vpHeight := max(height-2, 1)

	// Build pre-wrapped content
	content := m.renderWorkerContent(workerID, vpWidth, vpHeight)

	// Pad content to push it to the bottom when it's shorter than viewport
	// This preserves the "latest content at bottom" behavior
	contentLines := strings.Split(content, "\n")
	if len(contentLines) < vpHeight {
		padding := make([]string, vpHeight-len(contentLines))
		contentLines = append(padding, contentLines...)
		content = strings.Join(contentLines, "\n")
	}

	// Get or create viewport for this worker
	vp, exists := m.workerPane.viewports[workerID]
	if !exists {
		vp = viewport.New(vpWidth, vpHeight)
		m.workerPane.viewports[workerID] = vp
	}
	vp.Width = vpWidth
	vp.Height = vpHeight

	// Check if user was at bottom BEFORE SetContent() changes the viewport state
	// This enables smart auto-scroll: only follow new content if user was at bottom
	wasAtBottom := vp.AtBottom()

	vp.SetContent(content)

	// Smart auto-scroll: only scroll to bottom if content is dirty AND user was at bottom
	// This preserves scroll position when user has scrolled up to read history
	if m.workerPane.contentDirty[workerID] && wasAtBottom {
		vp.GotoBottom()
	}

	// Store updated viewport back
	m.workerPane.viewports[workerID] = vp

	// Get viewport view (handles scrolling and clipping)
	viewportContent := vp.View()

	// Build title: "● WORKER-1" with colored indicator
	leftTitle := fmt.Sprintf("%s %s", indicatorStyle.Render(indicator), strings.ToUpper(workerID))

	// Build right title with scroll indicator, new content indicator, and token count
	var rightParts []string

	// Add new content indicator if scrolled up and new content arrived
	if m.workerPane.hasNewContent[workerID] {
		rightParts = append(rightParts, newContentIndicatorStyle.Render("↓New"))
	}

	// Add scroll indicator if scrolled up from bottom
	if scrollIndicator := buildScrollIndicator(vp); scrollIndicator != "" {
		rightParts = append(rightParts, scrollIndicator)
	}

	// Add context usage if available (format: "27k/200k") - muted style
	if workerMetrics, ok := m.workerPane.workerMetrics[workerID]; ok && workerMetrics != nil && workerMetrics.ContextTokens > 0 {
		rightParts = append(rightParts, scrollIndicatorStyle.Render(
			workerMetrics.FormatContextDisplay(),
		))
	}

	rightTitle := strings.Join(rightParts, " ")

	// Colors based on status
	titleColor := WorkerColor
	focusedBorderColor := lipgloss.AdaptiveColor{Light: "#54A0FF", Dark: "#54A0FF"}

	return styles.RenderWithTitleBorder(
		viewportContent,
		leftTitle,
		rightTitle,
		width,
		height,
		false,
		titleColor,
		focusedBorderColor,
	)
}

// renderFullscreenWorkerPane renders a single worker pane in fullscreen mode.
func (m Model) renderFullscreenWorkerPane(width, height int) string {
	// Get active workers
	var activeWorkerIDs []string
	for _, workerID := range m.workerPane.workerIDs {
		status := m.workerPane.workerStatus[workerID]
		if status != pool.WorkerRetired {
			activeWorkerIDs = append(activeWorkerIDs, workerID)
		}
	}

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

	var content strings.Builder

	// Process messages and build content
	for i, msg := range messages {
		isFirstToolInSequence := msg.IsToolCall && (i == 0 || !messages[i-1].IsToolCall)
		isLastToolInSequence := msg.IsToolCall && (i == len(messages)-1 || !messages[i+1].IsToolCall)

		if msg.IsToolCall {
			// Tool calls get grouped with tree characters
			if isFirstToolInSequence {
				roleLabel := roleStyle.Foreground(workerMessageStyle.GetForeground()).Render("Worker")
				content.WriteString(roleLabel)
				content.WriteString("\n")
			}

			var prefix string
			if isLastToolInSequence {
				prefix = "╰╴ "
			} else {
				prefix = "├╴ "
			}

			toolName := strings.TrimPrefix(msg.Content, "🔧 ")
			content.WriteString(toolCallStyle.Render(prefix + toolName))
			content.WriteString("\n")

			if isLastToolInSequence {
				content.WriteString("\n")
			}
		} else {
			// Regular text message
			var roleLabel string
			switch msg.Role {
			case "user":
				roleLabel = roleStyle.Foreground(userMessageStyle.GetForeground()).Render("User")
			case "coordinator":
				roleLabel = roleStyle.Foreground(CoordinatorColor).Render("Coordinator")
			default:
				roleLabel = roleStyle.Foreground(workerMessageStyle.GetForeground()).Render("Worker")
			}

			content.WriteString(roleLabel)
			content.WriteString("\n")
			wrapped := wordWrap(msg.Content, wrapWidth-4)
			content.WriteString(wrapped)
			content.WriteString("\n\n")
		}
	}

	return strings.TrimRight(content.String(), "\n")
}
