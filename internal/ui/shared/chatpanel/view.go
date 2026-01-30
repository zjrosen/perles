package chatpanel

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/workflow"
	"github.com/zjrosen/perles/internal/ui/shared/chatrender"
	"github.com/zjrosen/perles/internal/ui/shared/panes"
	"github.com/zjrosen/perles/internal/ui/styles"
)

// Border colors for assistant status (matches orchestration mode)
var (
	assistantWorkingBorderColor = lipgloss.AdaptiveColor{Light: "#54A0FF", Dark: "#54A0FF"} // Blue - working
	inputFocusedBorderColor     = lipgloss.AdaptiveColor{Light: "#AAAAAA", Dark: "#888888"} // Slightly brighter when focused
)

// QueuedCountStyle is used for the queue count indicator.
// Uses orange color to draw attention to pending queued messages (matches orchestration mode).
var QueuedCountStyle = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#FFA500", Dark: "#FFB347"})

// View renders the chat panel with separate message pane and input area.
func (m Model) View() string {
	if !m.visible {
		return ""
	}

	// Early return for zero dimensions
	if m.width < 1 || m.height < 1 {
		return ""
	}

	// Input pane height: 6 lines (4 content lines + 2 borders)
	inputPaneHeight := 6

	// Message pane height: remaining space
	messagePaneHeight := max(m.height-inputPaneHeight, 3)

	// Calculate content height for tabs (subtract 2 for borders)
	contentHeight := max(messagePaneHeight-2, 1)

	// Render content based on active tab
	var tabContent string
	switch m.activeTab {
	case TabChat:
		// Get pane from active session for per-session scroll state and selection
		session := m.ActiveSession()
		if session == nil {
			tabContent = ""
			break
		}

		// Calculate viewport dimensions
		vpWidth := max(m.width-2, 1)

		// Show loading indicator while waiting for first response
		if session.Status == events.ProcessStatusPending {
			tabContent = m.renderLoadingIndicator(vpWidth, contentHeight)
			break
		}

		// Show error state if spawn failed
		if session.Status == events.ProcessStatusFailed {
			tabContent = m.renderErrorState(vpWidth, contentHeight)
			break
		}

		// Use VirtualSelectablePane for O(visible) rendering with text selection
		// SetMessages handles content conversion and auto-scroll behavior internally
		session.Pane.SetMessages(session.Messages, vpWidth, chatrender.RenderConfig{
			AgentLabel: "Assistant",
			AgentColor: chatrender.AssistantColor,
		})
		session.Pane.SetSize(vpWidth, contentHeight)

		tabContent = session.Pane.View()

	case TabSessions:
		tabContent = m.renderSessionsTab(contentHeight)
	case TabWorkflows:
		tabContent = m.renderWorkflowsTab(contentHeight)
	}

	// Build tabs for the top pane with zone IDs for click detection
	tabs := []panes.Tab{
		{Label: "Chat", Content: tabContent, ZoneID: makeTabZoneID(TabChat)},
		{Label: "Sessions", Content: tabContent, ZoneID: makeTabZoneID(TabSessions)},
		{Label: "Workflows", Content: tabContent, ZoneID: makeTabZoneID(TabWorkflows)},
	}

	// Get active session for state-driven UI rendering
	session := m.ActiveSession()

	// Determine border color based on active session's status
	borderColor := styles.BorderDefaultColor
	if session != nil && session.Status == events.ProcessStatusWorking {
		borderColor = assistantWorkingBorderColor
	}

	// Build bottom-left queue indicator from active session's queue count
	var bottomLeft string
	if session != nil && session.QueueCount > 0 {
		bottomLeft = QueuedCountStyle.Render(fmt.Sprintf("[%d queued]", session.QueueCount))
	}

	// Build bottom-right metrics display from active session's metrics
	var bottomRight string
	if session != nil && session.Metrics != nil && session.Metrics.TokensUsed > 0 {
		bottomRight = session.Metrics.FormatContextDisplay()
	}

	// Render the tabbed pane
	// BorderedPane handles hidden tabs internally, so we pass the raw activeTab index
	tabbedPane := panes.BorderedPane(panes.BorderConfig{
		Content:     tabContent,
		Width:       m.width,
		Height:      messagePaneHeight,
		Tabs:        tabs,
		ActiveTab:   m.activeTab,
		BorderColor: borderColor,
		BottomLeft:  bottomLeft,
		BottomRight: bottomRight,
	})

	// Render the input pane
	inputWidth := m.width - 2 - 2 // borders and padding
	inputContent := lipgloss.JoinHorizontal(lipgloss.Left,
		" ",
		lipgloss.NewStyle().Width(inputWidth).Render(m.input.View()),
		" ",
	)

	// Use slightly brighter border when focused
	inputBorderColor := styles.BorderDefaultColor
	if m.focused {
		inputBorderColor = inputFocusedBorderColor
	}

	// Show session ID in input pane top-left when multiple sessions exist
	var inputTopLeft string
	if len(m.sessions) > 1 {
		inputTopLeft = m.activeSessionID
	}

	inputPane := panes.BorderedPane(panes.BorderConfig{
		Content:     inputContent,
		Width:       m.width,
		Height:      inputPaneHeight,
		TopLeft:     inputTopLeft,
		BottomLeft:  m.input.ModeIndicator(),
		BorderColor: inputBorderColor,
		// PreWrapped true because vimtextarea handles its own soft-wrapping
		PreWrapped: true,
	})

	// Stack message pane and input pane vertically
	// Note: Do NOT call zone.Scan() here - it must be called at the app level
	// after the chatpanel is positioned, so zones are registered with correct screen coordinates.
	return lipgloss.JoinVertical(lipgloss.Left,
		tabbedPane,
		inputPane,
	)
}

// renderLoadingIndicator renders a centered loading indicator with spinner.
// Used during initial spawn phase when session is Pending or Starting.
// Uses theme-aware SpinnerColor for the spinner styling.
func (m Model) renderLoadingIndicator(width, height int) string {
	if width < 1 || height < 1 {
		return ""
	}

	spinnerStyle := lipgloss.NewStyle().
		Foreground(styles.SpinnerColor)

	messageStyle := lipgloss.NewStyle().
		Foreground(styles.TextSecondaryColor)

	// Get current spinner frame
	frame := spinnerFrames[m.spinnerFrame%len(spinnerFrames)]

	// Build loading content with animated braille spinner
	var lines []string
	lines = append(lines, "")
	lines = append(lines, spinnerStyle.Render(frame+" Starting assistant..."))
	lines = append(lines, "")
	lines = append(lines, messageStyle.Render("Please wait while the AI process initializes"))
	lines = append(lines, "")

	content := strings.Join(lines, "\n")

	// Center the content block
	style := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Align(lipgloss.Center, lipgloss.Center)

	return style.Render(content)
}

// renderErrorState renders a centered error state with recovery guidance.
// Used when session status is Failed (ProcessStatusFailed).
// Shows error message and instructions for recovery.
func (m Model) renderErrorState(width, height int) string {
	if width < 1 || height < 1 {
		return ""
	}

	warningStyle := lipgloss.NewStyle().
		Foreground(styles.StatusErrorColor).
		Bold(true)

	messageStyle := lipgloss.NewStyle().
		Foreground(styles.TextPrimaryColor)

	helpStyle := lipgloss.NewStyle().
		Foreground(styles.TextSecondaryColor)

	keyStyle := lipgloss.NewStyle().
		Foreground(styles.StatusInProgressColor)

	// Build error content
	var lines []string
	lines = append(lines, "")
	lines = append(lines, warningStyle.Render("Failed to start assistant"))
	lines = append(lines, "")
	lines = append(lines, messageStyle.Render("The AI process could not be initialized."))
	lines = append(lines, "")
	lines = append(lines, helpStyle.Render("Recovery options:"))
	lines = append(lines, keyStyle.Render("Ctrl+W")+" "+helpStyle.Render("to retry"))
	lines = append(lines, "")

	content := strings.Join(lines, "\n")

	// Center the content block
	style := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Align(lipgloss.Center, lipgloss.Center)

	return style.Render(content)
}

// renderSessionsTab renders the Sessions tab content with a selectable session list.
// Shows each session with: ● session-id (N msgs) - Status
// ● indicates unread content, ○ indicates viewed session.
// The first item is always "Create new session" option.
func (m Model) renderSessionsTab(contentHeight int) string {
	// Build session list
	var lines []string

	// Calculate inner width for padding (width minus borders)
	innerWidth := max(m.width-2, 1)

	// Background color for selection
	bgColor := styles.SelectionBackgroundColor

	// First item: "Create new session" option (cursor index 0)
	createNewSelected := m.sessionListCursor == 0 && m.activeTab == TabSessions
	createNewText := "+ Create new session"
	createNewPadding := max(innerWidth-lipgloss.Width(createNewText), 0)
	if createNewSelected {
		createNewStyle := lipgloss.NewStyle().Foreground(styles.StatusSuccessColor).Background(bgColor).Bold(true)
		spaceStyle := lipgloss.NewStyle().Background(bgColor)
		lines = append(lines, createNewStyle.Render(createNewText)+spaceStyle.Render(strings.Repeat(" ", createNewPadding)))
	} else {
		createNewStyle := lipgloss.NewStyle().Foreground(styles.StatusSuccessColor)
		lines = append(lines, createNewStyle.Render(createNewText))
	}

	// Session items start at cursor index 1 (index 0 is "Create new session")
	for i, sessionID := range m.sessionOrder {
		session := m.sessions[sessionID]
		if session == nil {
			continue
		}

		// Cursor index is i+1 because index 0 is "Create new session"
		isSelected := (i+1) == m.sessionListCursor && m.activeTab == TabSessions
		isPendingRetire := sessionID == m.pendingRetireSessionID

		// Build activity indicator: ● for new content, ○ for viewed
		var indicatorStyle lipgloss.Style
		var indicatorChar string
		if session.HasNewContent {
			indicatorChar = "●"
			indicatorStyle = lipgloss.NewStyle().Foreground(styles.StatusSuccessColor)
		} else {
			indicatorChar = "○"
			indicatorStyle = lipgloss.NewStyle().Foreground(styles.TextSecondaryColor)
		}

		// Build status display
		var statusText string
		var statusStyle lipgloss.Style
		if isPendingRetire {
			// Show confirmation prompt instead of status
			statusText = "Press d to confirm, esc to cancel"
			statusStyle = lipgloss.NewStyle().Foreground(styles.StatusWarningColor)
		} else if session.Status.IsTerminal() {
			statusText = "Session ended"
			statusStyle = lipgloss.NewStyle().Foreground(styles.StatusErrorColor)
		} else {
			switch session.Status {
			case events.ProcessStatusWorking:
				statusText = "Working"
				statusStyle = lipgloss.NewStyle().Foreground(styles.StatusInProgressColor)
			case events.ProcessStatusReady:
				statusText = "Ready"
				statusStyle = lipgloss.NewStyle().Foreground(styles.StatusSuccessColor)
			case events.ProcessStatusPending:
				statusText = "Pending"
				statusStyle = lipgloss.NewStyle().Foreground(styles.TextSecondaryColor)
			case events.ProcessStatusStarting:
				statusText = "Starting"
				statusStyle = lipgloss.NewStyle().Foreground(styles.TextSecondaryColor)
			case events.ProcessStatusPaused:
				statusText = "Paused"
				statusStyle = lipgloss.NewStyle().Foreground(styles.TextSecondaryColor)
			case events.ProcessStatusStopped:
				statusText = "Stopped"
				statusStyle = lipgloss.NewStyle().Foreground(styles.StatusErrorColor)
			default:
				statusText = string(session.Status)
				statusStyle = lipgloss.NewStyle().Foreground(styles.TextSecondaryColor)
			}
		}

		// Mark active session with asterisk
		activeMarker := " "
		if sessionID == m.activeSessionID {
			activeMarker = "*"
		}

		// Message count
		msgCountStr := fmt.Sprintf("(%d msgs)", len(session.Messages))

		// Build the line content (without styling) to calculate padding
		// Format: ● session-1* (5 msgs) - Ready (or confirmation prompt)
		plainContent := fmt.Sprintf("%s %s%s %s - %s", indicatorChar, session.ID, activeMarker, msgCountStr, statusText)
		padding := max(innerWidth-lipgloss.Width(plainContent), 0)

		// Build the styled line
		var result strings.Builder
		if isSelected {
			// Apply background to all segments
			spaceStyle := lipgloss.NewStyle().Background(bgColor)
			nameStyle := lipgloss.NewStyle().Foreground(styles.TextPrimaryColor).Background(bgColor).Bold(true)

			result.WriteString(indicatorStyle.Background(bgColor).Render(indicatorChar))
			result.WriteString(spaceStyle.Render(" "))
			result.WriteString(nameStyle.Render(session.ID + activeMarker))
			result.WriteString(spaceStyle.Render(" "))
			result.WriteString(spaceStyle.Render(msgCountStr))
			result.WriteString(spaceStyle.Render(" - "))
			result.WriteString(statusStyle.Background(bgColor).Render(statusText))
			result.WriteString(spaceStyle.Render(strings.Repeat(" ", padding)))
		} else {
			// Normal rendering without background
			nameStyle := lipgloss.NewStyle().Foreground(styles.TextPrimaryColor)

			result.WriteString(indicatorStyle.Render(indicatorChar))
			result.WriteString(" ")
			result.WriteString(nameStyle.Render(session.ID + activeMarker))
			result.WriteString(" ")
			result.WriteString(msgCountStr)
			result.WriteString(" - ")
			result.WriteString(statusStyle.Render(statusText))
		}

		lines = append(lines, result.String())
	}

	// Join lines and pad to content height if needed
	content := strings.Join(lines, "\n")
	contentLines := len(lines)
	if contentLines < contentHeight {
		// Pad with empty lines at bottom using strings.Builder for efficiency
		var sb strings.Builder
		sb.WriteString(content)
		for i := 0; i < contentHeight-contentLines; i++ {
			sb.WriteString("\n")
		}
		content = sb.String()
	}

	return content
}

// renderWorkflowsTab renders the Workflows tab content with a selectable workflow list.
// Shows each workflow in multi-line format:
//   - Line 1: ● Name
//   - Line 2+: Indented description (wrapped if long)
//
// ● is colored by source: green for user, blue for built-in.
func (m Model) renderWorkflowsTab(contentHeight int) string {
	workflows := m.getWorkflowsForTab()

	// Handle empty state
	if len(workflows) == 0 {
		return m.renderEmptyWorkflowsState(contentHeight)
	}

	var lines []string
	innerWidth := max(m.width-2, 1)
	bgColor := styles.SelectionBackgroundColor
	descIndent := "  " // 2 spaces to align description under name

	for i, wf := range workflows {
		isSelected := i == m.workflowListCursor && m.activeTab == TabWorkflows

		// Source indicator: green for user, blue for built-in
		var indicatorColor lipgloss.TerminalColor
		if wf.Source == workflow.SourceUser {
			indicatorColor = styles.StatusSuccessColor // green
		} else {
			indicatorColor = styles.StatusInProgressColor // blue
		}

		// Line 1: ● Name
		plainLine1 := fmt.Sprintf("● %s", wf.Name)
		line1Padding := max(innerWidth-lipgloss.Width(plainLine1), 0)

		// Description lines (wrapped)
		descWidth := innerWidth - len(descIndent)
		var descLines []string
		if wf.Description != "" {
			descLines = wrapText(wf.Description, descWidth)
		}

		if isSelected {
			// Render with full-width background on all lines
			spaceStyle := lipgloss.NewStyle().Background(bgColor)
			indicatorStyle := lipgloss.NewStyle().Foreground(indicatorColor).Background(bgColor)
			nameStyle := lipgloss.NewStyle().Foreground(styles.TextPrimaryColor).Bold(true).Background(bgColor)
			descStyle := lipgloss.NewStyle().Foreground(styles.TextDescriptionColor).Background(bgColor)

			// Line 1: ● Name with full-width background
			var line1 strings.Builder
			line1.WriteString(indicatorStyle.Render("●"))
			line1.WriteString(spaceStyle.Render(" "))
			line1.WriteString(nameStyle.Render(wf.Name))
			line1.WriteString(spaceStyle.Render(strings.Repeat(" ", line1Padding)))
			lines = append(lines, line1.String())

			// Description lines with full-width background
			for _, desc := range descLines {
				plainDesc := descIndent + desc
				descPadding := max(innerWidth-lipgloss.Width(plainDesc), 0)
				var descLine strings.Builder
				descLine.WriteString(spaceStyle.Render(descIndent))
				descLine.WriteString(descStyle.Render(desc))
				descLine.WriteString(spaceStyle.Render(strings.Repeat(" ", descPadding)))
				lines = append(lines, descLine.String())
			}
		} else {
			// Normal rendering without background
			indicatorStyle := lipgloss.NewStyle().Foreground(indicatorColor)
			nameStyle := lipgloss.NewStyle().Foreground(styles.TextPrimaryColor)
			descStyle := lipgloss.NewStyle().Foreground(styles.TextDescriptionColor)

			// Line 1: ● Name
			var line1 strings.Builder
			line1.WriteString(indicatorStyle.Render("●"))
			line1.WriteString(" ")
			line1.WriteString(nameStyle.Render(wf.Name))
			lines = append(lines, line1.String())

			// Description lines
			for _, desc := range descLines {
				lines = append(lines, descStyle.Render(descIndent+desc))
			}
		}
	}

	// Join lines and pad to content height if needed
	content := strings.Join(lines, "\n")
	contentLines := len(lines)
	if contentLines < contentHeight {
		// Pad with empty lines at bottom using strings.Builder for efficiency
		var sb strings.Builder
		sb.WriteString(content)
		for i := 0; i < contentHeight-contentLines; i++ {
			sb.WriteString("\n")
		}
		content = sb.String()
	}

	return content
}

// wrapText wraps text to fit within maxWidth characters using simple word wrapping.
// Returns a slice of lines, each fitting within maxWidth.
func wrapText(text string, maxWidth int) []string {
	if maxWidth <= 0 || text == "" {
		return nil
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	var lines []string
	var currentLine strings.Builder

	for _, word := range words {
		wordLen := lipgloss.Width(word)

		// If word is longer than maxWidth, add it as its own line (truncated)
		if wordLen > maxWidth {
			if currentLine.Len() > 0 {
				lines = append(lines, currentLine.String())
				currentLine.Reset()
			}
			lines = append(lines, styles.TruncateString(word, maxWidth))
			continue
		}

		// Check if adding this word would exceed maxWidth
		if currentLine.Len() > 0 {
			testLen := lipgloss.Width(currentLine.String()) + 1 + wordLen // +1 for space
			if testLen > maxWidth {
				lines = append(lines, currentLine.String())
				currentLine.Reset()
			}
		}

		if currentLine.Len() > 0 {
			currentLine.WriteString(" ")
		}
		currentLine.WriteString(word)
	}

	// Add remaining content
	if currentLine.Len() > 0 {
		lines = append(lines, currentLine.String())
	}

	return lines
}

// renderEmptyWorkflowsState renders a centered message when no workflows are available.
func (m Model) renderEmptyWorkflowsState(contentHeight int) string {
	innerWidth := max(m.width-2, 1)

	messageStyle := lipgloss.NewStyle().
		Foreground(styles.TextSecondaryColor)

	var lines []string
	lines = append(lines, "")
	lines = append(lines, messageStyle.Render("No workflows available"))
	lines = append(lines, "")
	lines = append(lines, messageStyle.Render("Add workflows to ~/.config/perles/workflows/"))

	content := strings.Join(lines, "\n")

	style := lipgloss.NewStyle().
		Width(innerWidth).
		Height(contentHeight).
		Align(lipgloss.Center, lipgloss.Center)

	return style.Render(content)
}
