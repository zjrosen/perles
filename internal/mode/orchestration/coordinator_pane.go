package orchestration

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"perles/internal/orchestration/coordinator"
	"perles/internal/ui/styles"
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

	newContentIndicatorStyle = lipgloss.NewStyle().
					Foreground(lipgloss.AdaptiveColor{Light: "#FECA57", Dark: "#FECA57"}). // Yellow/amber for attention
					Bold(true)

	scrollIndicatorStyle = lipgloss.NewStyle().
				Foreground(styles.TextMutedColor) // Muted color for scroll position
)

// renderCoordinatorPane renders the left pane showing coordinator chat history.
func (m Model) renderCoordinatorPane(width, height int) string {
	// Calculate viewport dimensions (subtract 2 for borders)
	vpWidth := max(width-2, 1)
	vpHeight := max(height-2, 1)

	// Build pre-wrapped content
	content := m.renderCoordinatorContent(vpWidth)

	// Pad content to push it to the bottom when it's shorter than viewport
	// This preserves the "latest content at bottom" behavior
	contentLines := strings.Split(content, "\n")
	if len(contentLines) < vpHeight {
		padding := make([]string, vpHeight-len(contentLines))
		contentLines = append(padding, contentLines...)
		content = strings.Join(contentLines, "\n")
	}

	// Get or create viewport for this pane
	vp := m.coordinatorPane.viewports[viewportKey]
	vp.Width = vpWidth
	vp.Height = vpHeight

	// Check if user was at bottom BEFORE SetContent() changes the viewport state
	// This enables smart auto-scroll: only follow new content if user was at bottom
	wasAtBottom := vp.AtBottom()

	vp.SetContent(content)

	// Smart auto-scroll: only scroll to bottom if content is dirty AND user was at bottom
	// This preserves scroll position when user has scrolled up to read history
	if m.coordinatorPane.contentDirty && wasAtBottom {
		vp.GotoBottom()
	}

	// Store updated viewport back (maps are reference types, so this persists)
	m.coordinatorPane.viewports[viewportKey] = vp

	// Get viewport view (handles scrolling and clipping)
	viewportContent := vp.View()

	// Colors for title and border (use CoordinatorColor like worker pane uses WorkerColor)
	titleColor := CoordinatorColor
	focusedBorderColor := lipgloss.AdaptiveColor{Light: "#54A0FF", Dark: "#54A0FF"}

	// Build title with status indicator first, like worker pane: "● COORDINATOR"
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

	leftTitle := fmt.Sprintf("%s COORDINATOR", indicatorStyle.Render(indicator))

	// Build right title with new content indicator, scroll indicator and token count
	var rightParts []string

	// Add new content indicator if scrolled up and new content arrived
	if m.coordinatorPane.hasNewContent {
		rightParts = append(rightParts, newContentIndicatorStyle.Render("↓New"))
	}

	// Add scroll indicator if scrolled up from bottom
	if scrollIndicator := buildScrollIndicator(vp); scrollIndicator != "" {
		rightParts = append(rightParts, scrollIndicator)
	}

	// Add context usage if available (format: "27k/200k") - muted style
	if m.coordinatorMetrics != nil && m.coordinatorMetrics.ContextTokens > 0 {
		rightParts = append(rightParts, scrollIndicatorStyle.Render(
			m.coordinatorMetrics.FormatContextDisplay(),
		))
	}

	rightTitle := strings.Join(rightParts, " ")

	// Render pane with bordered title (height includes title + border, no adjustment needed)
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

// renderCoordinatorContent builds the pre-wrapped content string for the viewport.
func (m Model) renderCoordinatorContent(wrapWidth int) string {
	var content strings.Builder

	for i, msg := range m.coordinatorPane.messages {
		// Check if we're in a tool call sequence
		isFirstToolInSequence := msg.IsToolCall && (i == 0 || !m.coordinatorPane.messages[i-1].IsToolCall)
		isLastToolInSequence := msg.IsToolCall && (i == len(m.coordinatorPane.messages)-1 || !m.coordinatorPane.messages[i+1].IsToolCall)

		if msg.Role == "user" {
			// User messages always get full label
			roleLabel := roleStyle.Foreground(userMessageStyle.GetForeground()).Render("User")
			wrappedContent := wordWrap(msg.Content, wrapWidth-4)
			content.WriteString(roleLabel)
			content.WriteString("\n")
			content.WriteString(wrappedContent)
			content.WriteString("\n\n")

		} else if msg.IsToolCall {
			// Tool calls get grouped with tree characters
			var prefix string
			if isFirstToolInSequence {
				// First tool in sequence - show role label
				roleLabel := roleStyle.Foreground(coordinatorMessageStyle.GetForeground()).Render("Coordinator")
				content.WriteString(roleLabel)
				content.WriteString("\n")
			}

			// Choose tree character based on position
			if isLastToolInSequence {
				prefix = "╰╴ "
			} else {
				prefix = "├╴ "
			}

			// Remove 🔧 emoji prefix since we're using tree characters
			toolName := strings.TrimPrefix(msg.Content, "🔧 ")
			// Apply lighter color to tool calls
			content.WriteString(toolCallStyle.Render(prefix + toolName))
			content.WriteString("\n")

			// Add spacing after last tool in sequence
			if isLastToolInSequence {
				content.WriteString("\n")
			}

		} else {
			// Regular coordinator text message
			roleLabel := roleStyle.Foreground(coordinatorMessageStyle.GetForeground()).Render("Coordinator")
			wrappedContent := wordWrap(msg.Content, wrapWidth-4)
			content.WriteString(roleLabel)
			content.WriteString("\n")
			content.WriteString(wrappedContent)
			content.WriteString("\n\n")
		}
	}

	return strings.TrimRight(content.String(), "\n")
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

// buildScrollIndicator returns a styled scroll position indicator for the viewport.
// Returns empty string if content fits in viewport or if at bottom (live view).
// Returns styled "↑XX%" when scrolled up from bottom.
func buildScrollIndicator(vp viewport.Model) string {
	if vp.TotalLineCount() <= vp.Height {
		return "" // Content fits, no indicator needed
	}
	if vp.AtBottom() {
		return "" // At live position, no indicator needed
	}
	return scrollIndicatorStyle.Render(fmt.Sprintf("↑%.0f%%", vp.ScrollPercent()*100))
}

// renderFullscreenCoordinatorPane renders the coordinator pane in fullscreen mode.
func (m Model) renderFullscreenCoordinatorPane(width, height int) string {
	// Calculate viewport dimensions (subtract 2 for borders)
	vpWidth := max(width-2, 1)
	vpHeight := max(height-2, 1)

	// Build pre-wrapped content
	content := m.renderCoordinatorContent(vpWidth)

	// Pad content to push it to the bottom when it's shorter than viewport
	contentLines := strings.Split(content, "\n")
	if len(contentLines) < vpHeight {
		padding := make([]string, vpHeight-len(contentLines))
		contentLines = append(padding, contentLines...)
		content = strings.Join(contentLines, "\n")
	}

	// Get or create viewport for this pane
	vp := m.coordinatorPane.viewports[viewportKey]
	vp.Width = vpWidth
	vp.Height = vpHeight

	wasAtBottom := vp.AtBottom()
	vp.SetContent(content)

	if m.coordinatorPane.contentDirty && wasAtBottom {
		vp.GotoBottom()
	}

	m.coordinatorPane.viewports[viewportKey] = vp
	viewportContent := vp.View()

	leftTitle := "● COORDINATOR"

	return styles.RenderWithTitleBorder(
		viewportContent,
		leftTitle,
		"",
		width,
		height,
		false,
		CoordinatorColor,
		CoordinatorColor,
	)
}
