package orchestration

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/zjrosen/perles/internal/orchestration/coordinator"
	"github.com/zjrosen/perles/internal/ui/shared/panes"
	"github.com/zjrosen/perles/internal/ui/styles"
)

// Coordinator pane styles
var (
	userMessageStyle = lipgloss.NewStyle().
				Foreground(UserColor)

	coordinatorMessageStyle = lipgloss.NewStyle().
				Foreground(CoordinatorColor)

	roleStyle = lipgloss.NewStyle().Bold(true)

	toolCallStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#999999", Dark: "#666666"})
)

// renderCoordinatorPane renders the left pane showing coordinator chat history.
// When fullscreen=true, renders in fullscreen mode with simplified title and no metrics.
func (m Model) renderCoordinatorPane(width, height int, fullscreen bool) string {
	// Get viewport from map (will be modified by helper via pointer)
	vp := m.coordinatorPane.viewports[viewportKey]

	// Build title and metrics based on fullscreen mode
	var leftTitle, metricsDisplay string
	var hasNewContent bool
	var borderColor lipgloss.AdaptiveColor

	if fullscreen {
		// Fullscreen: simplified title, no metrics or new content indicator
		leftTitle = "● COORDINATOR"
		metricsDisplay = ""
		hasNewContent = false
		borderColor = CoordinatorColor
	} else {
		// Normal: dynamic status title with metrics
		leftTitle = m.buildCoordinatorTitle()
		if m.coordinatorMetrics != nil && m.coordinatorMetrics.ContextTokens > 0 {
			metricsDisplay = m.coordinatorMetrics.FormatContextDisplay()
		}
		hasNewContent = m.coordinatorPane.hasNewContent
		borderColor = styles.BorderDefaultColor
	}

	// Use panes.ScrollablePane helper for viewport setup, padding, and auto-scroll
	result := panes.ScrollablePane(width, height, panes.ScrollableConfig{
		Viewport:       &vp,
		ContentDirty:   m.coordinatorPane.contentDirty,
		HasNewContent:  hasNewContent,
		MetricsDisplay: metricsDisplay,
		LeftTitle:      leftTitle,
		TitleColor:     CoordinatorColor,
		BorderColor:    borderColor,
	}, m.renderCoordinatorContent)

	// Store updated viewport back to map (helper modified via pointer)
	m.coordinatorPane.viewports[viewportKey] = vp

	return result
}

// buildCoordinatorTitle builds the left title with status indicator for the coordinator pane.
func (m Model) buildCoordinatorTitle() string {
	var indicator string
	var indicatorStyle lipgloss.Style

	if m.coord != nil {
		status := m.coord.Status()
		switch status {
		case coordinator.StatusRunning:
			// When running, show working (blue ●) or ready (green ○) based on activity
			if m.coordinatorWorking {
				indicator = "●"
				indicatorStyle = workerWorkingStyle // Blue - actively working
			} else {
				indicator = "○"
				indicatorStyle = workerReadyStyle // Green - ready/waiting for input
			}
		case coordinator.StatusPaused:
			indicator = "⏸"
			indicatorStyle = statusPausedStyle
		case coordinator.StatusFailed, coordinator.StatusStopped:
			indicator = "✗"
			indicatorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B"))
		default:
			indicator = "○"
			indicatorStyle = lipgloss.NewStyle().Foreground(styles.TextSecondaryColor)
		}
	} else {
		// No coordinator yet - show empty circle
		indicator = "○"
		indicatorStyle = lipgloss.NewStyle().Foreground(styles.TextSecondaryColor)
	}

	return fmt.Sprintf("%s COORDINATOR", indicatorStyle.Render(indicator))
}

// renderCoordinatorContent builds the pre-wrapped content string for the viewport.
func (m Model) renderCoordinatorContent(wrapWidth int) string {
	return renderChatContent(m.coordinatorPane.messages, wrapWidth, ChatRenderConfig{
		AgentLabel: "Coordinator",
		AgentColor: coordinatorMessageStyle.GetForeground().(lipgloss.AdaptiveColor),
	})
}

// wordWrap wraps text at the given width, preserving explicit newlines.
func wordWrap(text string, width int) string {
	if width <= 0 {
		return text
	}

	// Split by newlines first to preserve explicit line breaks
	lines := strings.Split(text, "\n")
	var result strings.Builder

	for lineIdx, line := range lines {
		if lineIdx > 0 {
			result.WriteString("\n")
		}

		// Skip empty lines
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Word wrap this line
		words := strings.Fields(line)
		var currentLine strings.Builder

		for i, word := range words {
			// Check if adding this word would exceed line width
			needsNewLine := currentLine.Len()+len(word)+1 > width && currentLine.Len() > 0

			if needsNewLine {
				result.WriteString(currentLine.String())
				result.WriteString("\n")
				currentLine.Reset()
			}

			if currentLine.Len() > 0 {
				currentLine.WriteString(" ")
			}
			currentLine.WriteString(word)

			// Write last word of this line
			if i == len(words)-1 && currentLine.Len() > 0 {
				result.WriteString(currentLine.String())
			}
		}
	}

	return result.String()
}
