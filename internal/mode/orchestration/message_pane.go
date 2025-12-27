package orchestration

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/ui/shared/panes"
	"github.com/zjrosen/perles/internal/ui/styles"
)

// Message pane styles
var (
	messageTimestampStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#666666", Dark: "#696969"})

	coordinatorSenderStyle = lipgloss.NewStyle().
				Foreground(CoordinatorColor).
				Bold(true)

	workerSenderStyle = lipgloss.NewStyle().
				Foreground(WorkerColor).
				Bold(true)

	userSenderStyle = lipgloss.NewStyle().
			Foreground(UserColor).
			Bold(true)

	errorSenderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#FF6B6B", Dark: "#FF8787"}).
				Bold(true)

	messageContentStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#CCCCCC"})

	// Border styles for left message border (no bold)
	coordinatorBorderStyle = lipgloss.NewStyle().Foreground(CoordinatorColor)
	workerBorderStyle      = lipgloss.NewStyle().Foreground(WorkerColor)
	userBorderStyle        = lipgloss.NewStyle().Foreground(UserColor)
	errorBorderStyle       = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#FF6B6B", Dark: "#FF8787"})
)

// renderMessagePane renders the middle pane showing message log.
// When fullscreen=true, renders in fullscreen mode with simplified styling.
func (m Model) renderMessagePane(width, height int, fullscreen bool) string {
	// Get viewport from map (will be modified by helper via pointer)
	vp := m.messagePane.viewports[viewportKey]

	// Build config based on fullscreen mode
	var leftTitle string
	var hasNewContent bool
	var titleColor, borderColor lipgloss.AdaptiveColor

	if fullscreen {
		// Fullscreen: simplified title, muted colors, no new content indicator
		leftTitle = "MESSAGES"
		hasNewContent = false
		titleColor = styles.TextMutedColor
		borderColor = styles.BorderDefaultColor
	} else {
		// Normal: full title with new content indicator
		leftTitle = "MESSAGE LOG"
		hasNewContent = m.messagePane.hasNewContent
		titleColor = styles.TextSecondaryColor
		borderColor = styles.BorderDefaultColor
	}

	// Use panes.ScrollablePane helper for viewport setup, padding, and auto-scroll
	result := panes.ScrollablePane(width, height, panes.ScrollableConfig{
		Viewport:       &vp,
		ContentDirty:   m.messagePane.contentDirty,
		HasNewContent:  hasNewContent,
		MetricsDisplay: "", // No metrics for message pane
		LeftTitle:      leftTitle,
		TitleColor:     titleColor,
		BorderColor:    borderColor,
	}, m.renderMessageContent)

	// Store updated viewport back to map (helper modified via pointer)
	m.messagePane.viewports[viewportKey] = vp

	return result
}

// renderMessageContent builds the pre-wrapped content string for the viewport.
func (m Model) renderMessageContent(wrapWidth int) string {
	var content strings.Builder

	for _, entry := range m.messagePane.entries {
		// Check if sender is a worker (case-insensitive, handles both "WORKER.1" and "worker-1")
		fromUpper := strings.ToUpper(entry.From)
		isWorker := strings.HasPrefix(fromUpper, "WORKER")

		// Determine left border style based on sender
		var borderStyle lipgloss.Style
		switch {
		case entry.From == message.ActorCoordinator:
			borderStyle = coordinatorBorderStyle
		case entry.From == message.ActorUser:
			borderStyle = userBorderStyle
		case entry.Type == message.MessageError:
			borderStyle = errorBorderStyle
		case isWorker:
			borderStyle = workerBorderStyle
		default:
			borderStyle = messageTimestampStyle
		}

		leftBorder := borderStyle.Render("│")

		// Format timestamp
		timestamp := messageTimestampStyle.Render(entry.Timestamp.Format("15:04"))

		// Style sender based on who sent it
		var senderStyled string
		switch {
		case entry.From == message.ActorCoordinator:
			senderStyled = coordinatorSenderStyle.Render(entry.From)
		case entry.From == message.ActorUser:
			senderStyled = userSenderStyle.Render(entry.From)
		case entry.Type == message.MessageError:
			senderStyled = errorSenderStyle.Render(entry.From)
		case isWorker:
			senderStyled = workerSenderStyle.Render(entry.From)
		default:
			senderStyled = entry.From
		}

		// Format header: timestamp | SENDER → RECIPIENT
		header := fmt.Sprintf("%s %s → %s", timestamp, senderStyled, entry.To)

		// Word wrap content (account for left border + space)
		wrappedContent := wordWrap(entry.Content, wrapWidth-4)
		styledContent := messageContentStyle.Render(wrappedContent)

		// Add left border to header
		content.WriteString(leftBorder + " " + header)
		content.WriteString("\n")

		// Add left border to each content line
		for line := range strings.SplitSeq(styledContent, "\n") {
			content.WriteString(leftBorder + " " + line)
			content.WriteString("\n")
		}
		content.WriteString("\n")
	}

	return strings.TrimRight(content.String(), "\n")
}
