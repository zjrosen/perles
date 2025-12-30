package orchestration

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/zjrosen/perles/internal/ui/shared/panes"
	"github.com/zjrosen/perles/internal/ui/styles"
)

// View styles
var (
	statusPausedStyle = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#FECA57", Dark: "#FECA57"}).
		Bold(true)
)

// View renders the orchestration mode UI.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	// During initialization phases, show the init screen instead of main view
	initPhase := m.getInitPhase()
	if initPhase != InitReady && initPhase != InitNotStarted {
		return m.renderInitScreen()
	}

	return m.renderMainView()
}

// renderMainView renders the three-pane orchestration mode UI.
func (m Model) renderMainView() string {
	// Calculate dynamic input bar height based on textarea content
	inputHeight := m.calculateInputHeight()
	contentHeight := max(m.height-inputHeight, 5)

	var panes string

	if m.fullscreenPaneType != PaneNone {
		// Fullscreen mode: render only the selected pane
		switch m.fullscreenPaneType {
		case PaneCoordinator:
			panes = m.renderCoordinatorPane(m.width, contentHeight, true)
		case PaneMessages:
			panes = m.renderMessagePane(m.width, contentHeight, true)
		case PaneWorker:
			panes = m.renderFullscreenWorkerPane(m.width, contentHeight)
		}
	} else {
		// Normal three-pane layout
		leftWidth := m.width * leftPanePercent / 100
		middleWidth := m.width * middlePanePercent / 100
		rightWidth := m.width - leftWidth - middleWidth

		// Render each pane
		leftPane := m.renderCoordinatorPane(leftWidth, contentHeight, false)
		middlePane := m.renderMessagePane(middleWidth, contentHeight, false)
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

	// Show workflow picker overlay
	if m.showWorkflowPicker && m.workflowPicker != nil {
		mainView = m.workflowPicker.Overlay(mainView)
	}

	// Show quit confirmation modal (below error modal)
	if m.quitModal != nil {
		mainView = m.quitModal.Overlay(mainView)
	}

	// Show error modal overlay (on top - errors take priority)
	if m.errorModal != nil {
		mainView = m.errorModal.Overlay(mainView)
	}

	return mainView
}

// calculateInputHeight returns the height of the input bar based on content.
// Height starts at 4 (2 content lines + 2 borders) and can grow to 6 (4 content + 2 borders).
// Uses display lines (accounting for soft-wrap) rather than logical lines.
func (m Model) calculateInputHeight() int {
	// Border takes 2 lines (top + bottom)
	displayLines := m.input.TotalDisplayLines()

	// Height = display lines + 2 for borders, clamped to [4, 6]
	return max(min(displayLines+2, 6), 4)
}

// renderInputBar renders the bottom input bar.
func (m Model) renderInputBar() string {
	inputHeight := m.calculateInputHeight()
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
		return panes.BorderedPane(panes.BorderConfig{
			Content:            content,
			Width:              m.width,
			Height:             inputHeight,
			TopLeft:            "Select fullscreen pane:",
			Focused:            true,
			TitleColor:         greyColor,
			FocusedBorderColor: greyColor,
		})
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

	// Calculate available width for input (accounting for borders and padding)
	innerWidth := m.width - 2    // Remove left/right border
	inputWidth := innerWidth - 2 // Space for padding

	// Get input view (may need to truncate/scroll)
	inputView := m.input.View()

	// Build the bar content with padding
	content := lipgloss.JoinHorizontal(lipgloss.Left,
		" ", // Left padding
		lipgloss.NewStyle().Width(inputWidth).Render(inputView),
		" ", // Right padding
	)

	// Build target label for left title - show who we're messaging (uppercase for consistency)
	targetLabel := strings.ToUpper(m.messageTarget)

	return panes.BorderedPane(panes.BorderConfig{
		Content:            content,
		Width:              m.width,
		Height:             inputHeight,
		TopLeft:            targetLabel,             // Left title shows target
		BottomLeft:         m.input.ModeIndicator(), // Vim mode indicator (styled by component)
		Focused:            m.input.Focused(),       // Highlight border when input is focused
		TitleColor:         titleColor,
		FocusedBorderColor: borderColor,
	})
}

// Resume prompt styles
