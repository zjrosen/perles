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

	initPhase := m.getInitPhase()

	// During InitNotStarted, show blank background (with modal overlay if present).
	// This prevents flash of main view before worktree modal appears.
	if initPhase == InitNotStarted {
		blankBg := lipgloss.NewStyle().Width(m.width).Height(m.height).Render("")
		if m.worktreeModal != nil {
			return m.worktreeModal.Overlay(blankBg)
		}
		if m.branchSelectModal != nil {
			return m.branchSelectModal.Overlay(blankBg)
		}
		// No modal yet, but still show blank to avoid flash
		return blankBg
	}

	// During initialization phases, show the init screen instead of main view
	if initPhase != InitReady && initPhase != InitNotStarted {
		return m.renderInitScreen()
	}

	return m.renderMainView()
}

// renderMainView renders the three-pane orchestration mode UI.
func (m Model) renderMainView() string {
	// Calculate dynamic input bar height based on textarea content
	inputHeight := m.calculateInputHeight()

	// Content height is remaining space after input
	contentHeight := max(m.height-inputHeight, 5)

	var mainPanes string

	if m.fullscreenPaneType != PaneNone {
		// Fullscreen mode: render only the selected pane
		switch m.fullscreenPaneType {
		case PaneCoordinator:
			mainPanes = m.renderCoordinatorPane(m.width, contentHeight, true)
		case PaneMessages:
			mainPanes = m.renderMessagePane(m.width, contentHeight, true)
		case PaneWorker:
			mainPanes = m.renderFullscreenWorkerPane(m.width, contentHeight)
		case PaneCommand:
			mainPanes = renderCommandPane(&m.commandPane, m.width, contentHeight)
		}
	} else {
		// Normal three-pane layout
		leftWidth := m.width * leftPanePercent / 100
		middleWidth := m.width * middlePanePercent / 100
		rightWidth := m.width - leftWidth - middleWidth

		// Render left and right panes (full content height)
		leftPane := m.renderCoordinatorPane(leftWidth, contentHeight, false)
		rightPanes := m.renderWorkerPanes(rightWidth, contentHeight)

		// Render middle column: command pane (if visible) + message log
		var middleColumn string
		if m.showCommandPane {
			// Split middle column height 30/70 between command pane and message log
			cmdPaneHeight := contentHeight * 30 / 100
			messagePaneHeight := contentHeight - cmdPaneHeight
			// Ensure minimum height of 5 for both panes
			if cmdPaneHeight < 5 {
				cmdPaneHeight = 5
			}
			if messagePaneHeight < 5 {
				messagePaneHeight = 5
			}
			cmdPane := renderCommandPane(&m.commandPane, middleWidth, cmdPaneHeight)
			msgPane := m.renderMessagePane(middleWidth, messagePaneHeight, false)
			middleColumn = lipgloss.JoinVertical(lipgloss.Left, cmdPane, msgPane)
		} else {
			middleColumn = m.renderMessagePane(middleWidth, contentHeight, false)
		}

		// Join panes horizontally
		mainPanes = lipgloss.JoinHorizontal(lipgloss.Top,
			leftPane,
			middleColumn,
			rightPanes,
		)
	}

	panesContent := mainPanes

	// Render input bar
	inputBar := m.renderInputBar()

	// Stack vertically
	mainView := lipgloss.JoinVertical(lipgloss.Left, panesContent, inputBar)

	// Show workflow picker overlay
	if m.showWorkflowPicker && m.workflowPicker != nil {
		mainView = m.workflowPicker.Overlay(mainView)
	}

	// Show quit confirmation modal (below error modal)
	if m.quitModal.IsVisible() {
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
		navHelp := "1-4=workers  5=coordinator  6=messages  7=commands  esc=exit"
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
