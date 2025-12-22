package orchestration

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"perles/internal/orchestration/message"
	"perles/internal/ui/styles"
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
func (m Model) renderMessagePane(width, height int) string {
	// Calculate viewport dimensions (subtract 2 for borders)
	vpWidth := max(width-2, 1)
	vpHeight := max(height-2, 1)

	// Build pre-wrapped content
	content := m.renderMessageContent(vpWidth)

	// Pad content to push it to the bottom when it's shorter than viewport
	// This preserves the "latest content at bottom" behavior
	contentLines := strings.Split(content, "\n")
	if len(contentLines) < vpHeight {
		padding := make([]string, vpHeight-len(contentLines))
		contentLines = append(padding, contentLines...)
		content = strings.Join(contentLines, "\n")
	}

	// Get or create viewport for this pane
	vp := m.messagePane.viewports[viewportKey]
	vp.Width = vpWidth
	vp.Height = vpHeight

	// Check if user was at bottom BEFORE SetContent() changes the viewport state
	// This enables smart auto-scroll: only follow new content if user was at bottom
	wasAtBottom := vp.AtBottom()

	vp.SetContent(content)

	// Smart auto-scroll: only scroll to bottom if content is dirty AND user was at bottom
	// This preserves scroll position when user has scrolled up to read history
	if m.messagePane.contentDirty && wasAtBottom {
		vp.GotoBottom()
	}

	// Store updated viewport back (maps are reference types, so this persists)
	m.messagePane.viewports[viewportKey] = vp

	// Get viewport view (handles scrolling and clipping)
	viewportContent := vp.View()

	// Colors for title and border (neutral color to match other panes)
	titleColor := styles.TextSecondaryColor
	focusedBorderColor := lipgloss.AdaptiveColor{Light: "#54A0FF", Dark: "#54A0FF"}

	// Build right title with scroll indicator and new content indicator
	var rightParts []string

	// Add new content indicator if scrolled up and new content arrived
	if m.messagePane.hasNewContent {
		rightParts = append(rightParts, newContentIndicatorStyle.Render("↓New"))
	}

	// Add scroll indicator if scrolled up from bottom
	if scrollIndicator := buildScrollIndicator(vp); scrollIndicator != "" {
		rightParts = append(rightParts, scrollIndicator)
	}

	rightTitle := strings.Join(rightParts, " ")

	// Render pane with bordered title
	return styles.RenderWithTitleBorder(
		viewportContent,
		"MESSAGE LOG",
		rightTitle,
		width,
		height,
		false,
		titleColor,
		focusedBorderColor,
	)
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

// renderFullscreenMessagePane renders the message pane in fullscreen mode.
func (m Model) renderFullscreenMessagePane(width, height int) string {
	// Calculate viewport dimensions (subtract 2 for borders)
	vpWidth := max(width-2, 1)
	vpHeight := max(height-2, 1)

	// Build pre-wrapped content
	content := m.renderMessageContent(vpWidth)

	// Pad content to push it to the bottom when it's shorter than viewport
	contentLines := strings.Split(content, "\n")
	if len(contentLines) < vpHeight {
		padding := make([]string, vpHeight-len(contentLines))
		contentLines = append(padding, contentLines...)
		content = strings.Join(contentLines, "\n")
	}

	// Get or create viewport for this pane
	vp := m.messagePane.viewports[viewportKey]
	vp.Width = vpWidth
	vp.Height = vpHeight

	wasAtBottom := vp.AtBottom()
	vp.SetContent(content)

	if m.messagePane.contentDirty && wasAtBottom {
		vp.GotoBottom()
	}

	m.messagePane.viewports[viewportKey] = vp
	viewportContent := vp.View()

	leftTitle := "MESSAGES"

	return styles.RenderWithTitleBorder(
		viewportContent,
		leftTitle,
		"",
		width,
		height,
		false,
		styles.TextMutedColor,
		styles.TextMutedColor,
	)
}
