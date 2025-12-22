package orchestration

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"perles/internal/ui/styles"
)

// View styles
var (
	inputPromptStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#54A0FF", Dark: "#54A0FF"})

	statusPausedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#FECA57", Dark: "#FECA57"}).
				Bold(true)
)

// View renders the orchestration mode UI.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	return m.renderMainView()
}

// renderMainView renders the three-pane orchestration mode UI.
func (m Model) renderMainView() string {
	// Reserve 4 lines for input bar
	contentHeight := max(m.height-4, 5)
	if m.height < 16 {
		contentHeight = max(m.height-3, 3)
	}

	var panes string

	if m.fullscreenPaneType != PaneNone {
		// Fullscreen mode: render only the selected pane
		switch m.fullscreenPaneType {
		case PaneCoordinator:
			panes = m.renderFullscreenCoordinatorPane(m.width, contentHeight)
		case PaneMessages:
			panes = m.renderFullscreenMessagePane(m.width, contentHeight)
		case PaneWorker:
			panes = m.renderFullscreenWorkerPane(m.width, contentHeight)
		}
	} else {
		// Normal three-pane layout
		leftWidth := m.width * 35 / 100
		middleWidth := m.width * 32 / 100
		rightWidth := m.width - leftWidth - middleWidth

		// Render each pane
		leftPane := m.renderCoordinatorPane(leftWidth, contentHeight)
		middlePane := m.renderMessagePane(middleWidth, contentHeight)
		rightPanes := m.renderWorkerPanes(rightWidth, contentHeight)

		// Join panes horizontally
		panes = lipgloss.JoinHorizontal(lipgloss.Top,
			leftPane,
			middlePane,
			rightPanes,
		)
	}

	// Render input bar
	inputBar := m.renderInputBar()

	// Stack vertically
	mainView := lipgloss.JoinVertical(lipgloss.Left, panes, inputBar)

	// Show error modal overlay
	if m.errorModal != nil {
		mainView = m.errorModal.Overlay(mainView)
	}

	return mainView
}

// renderInputBar renders the bottom input bar.
func (m Model) renderInputBar() string {
	inputHeight := 4
	if m.height < 16 {
		inputHeight = 3
	}
	// Navigation mode: show help instead of input
	if m.navigationMode {
		navHelp := "1-4=workers  5=coordinator  6=messages  esc=exit"
		navStyle := lipgloss.NewStyle().
			Foreground(styles.TextSecondaryColor)

		content := lipgloss.NewStyle().
			Width(m.width-4).
			Padding(0, 1).
			Render(navStyle.Render(navHelp))

		greyColor := lipgloss.AdaptiveColor{Light: "#666666", Dark: "#888888"}
		return styles.RenderWithTitleBorder(
			content,
			"Select fullscreen pane:",
			"",
			m.width,
			inputHeight,
			true,
			greyColor,
			greyColor,
		)
	}

	// Color based on target for visual distinction
	var titleColor, borderColor lipgloss.AdaptiveColor
	switch {
	case m.messageTarget == "BROADCAST":
		// Orange/amber for broadcast (attention-grabbing)
		titleColor = UserColor
		borderColor = UserColor
	case strings.HasPrefix(m.messageTarget, "worker"):
		// Green for workers
		titleColor = WorkerColor
		borderColor = WorkerColor
	default:
		// Teal for coordinator (default)
		titleColor = CoordinatorColor
		borderColor = CoordinatorColor
	}

	// Input bar - show target in title and input field
	prompt := inputPromptStyle.Render("> ")

	// Calculate available width for input (accounting for borders and spacing)
	innerWidth := m.width - 2                // Remove left/right border
	inputWidth := innerWidth - len("> ") - 4 // Space for prompt and padding

	// Get input view (may need to truncate/scroll)
	inputView := m.input.View()

	// Build the bar content with padding
	content := lipgloss.JoinHorizontal(lipgloss.Left,
		" ", // Left padding
		prompt,
		lipgloss.NewStyle().Width(inputWidth).Render(inputView),
		" ", // Right padding
	)

	// Build target label for left title - show who we're messaging (uppercase for consistency)
	targetLabel := strings.ToUpper(m.messageTarget)

	return styles.RenderWithTitleBorder(
		content,
		targetLabel, // Left title shows target
		"",          // No right title
		m.width,
		inputHeight,       // Taller input area
		m.input.Focused(), // Highlight border when input is focused
		titleColor,
		borderColor,
	)
}

// Resume prompt styles
