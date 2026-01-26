package dashboard

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"

	"github.com/zjrosen/perles/internal/mode"
	"github.com/zjrosen/perles/internal/mode/shared"
	"github.com/zjrosen/perles/internal/orchestration/controlplane"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/orchestration/metrics"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/process"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
	"github.com/zjrosen/perles/internal/ui/shared/chatrender"
	"github.com/zjrosen/perles/internal/ui/shared/panes"
	"github.com/zjrosen/perles/internal/ui/shared/selection"
	"github.com/zjrosen/perles/internal/ui/shared/toaster"
	"github.com/zjrosen/perles/internal/ui/shared/vimtextarea"
	"github.com/zjrosen/perles/internal/ui/styles"
)

// Tab indices for the coordinator panel.
// Dynamic worker tabs start at TabFirstWorker and increment by worker order.
const (
	TabCoordinator = 0 // Coordinator chat
	TabMessages    = 1 // Message log
	TabFirstWorker = 2 // First dynamic worker tab (if any)
)

// CoordinatorPanel manages the coordinator chat panel for a workflow.
type CoordinatorPanel struct {
	// Input for sending messages to coordinator
	input vimtextarea.Model

	// Clipboard for copy operations
	clipboard shared.Clipboard

	// Currently selected workflow ID
	workflowID controlplane.WorkflowID

	// Tab state
	activeTab int // Current tab index

	// Coordinator state (uses SelectablePane for viewport + selection)
	coordinatorPane     *selection.SelectablePane
	coordinatorMessages []chatrender.Message
	coordinatorStatus   events.ProcessStatus
	coordinatorQueue    int

	// Message log state (uses SelectablePane for viewport + selection)
	messagePane    *selection.SelectablePane
	messageEntries []message.Entry

	// Worker state (dynamic tabs)
	workerIDs      []string                             // Active worker IDs in display order
	workerPanes    map[string]*selection.SelectablePane // SelectablePane per worker
	workerMessages map[string][]chatrender.Message      // Messages per worker
	workerStatus   map[string]events.ProcessStatus      // Status per worker
	workerPhases   map[string]events.ProcessPhase       // Phase per worker
	workerQueues   map[string]int                       // Queue count per worker

	// Token metrics for display
	coordinatorMetrics *metrics.TokenMetrics
	workerMetrics      map[string]*metrics.TokenMetrics

	// Command log state (for debug mode)
	commandLogViewport viewport.Model
	commandLogEntries  []CommandLogEntry
	commandLogDirty    bool
	debugMode          bool // When true, show command log tab

	// Focus state
	focused bool

	// Dimensions
	width  int
	height int

	// Screen position for mouse coordinate mapping
	screenXOffset int // X offset from left edge of terminal
	screenYOffset int // Y offset from top (where viewports start, after tab bar)
}

// coordinatorTitleColor is the base color for coordinator title text.
// Uses the shared CoordinatorColor from chatrender for consistency across all chat UIs.
var coordinatorTitleColor = chatrender.CoordinatorColor

// workerTitleColor is the base color for worker title text.
var workerTitleColor = chatrender.WorkerColor

// Message pane styles (matches orchestration mode)
var (
	messageTimestampStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#666666", Dark: "#696969"})

	coordinatorSenderStyle = lipgloss.NewStyle().
				Foreground(chatrender.CoordinatorColor).
				Bold(true)

	workerSenderStyle = lipgloss.NewStyle().
				Foreground(chatrender.WorkerColor).
				Bold(true)

	userSenderStyle = lipgloss.NewStyle().
			Foreground(chatrender.UserColor).
			Bold(true)

	errorSenderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#FF6B6B", Dark: "#FF8787"}).
				Bold(true)

	// Border styles for left message border (no bold)
	coordinatorBorderStyle = lipgloss.NewStyle().Foreground(chatrender.CoordinatorColor)
	workerBorderStyle      = lipgloss.NewStyle().Foreground(chatrender.WorkerColor)
	userBorderStyle        = lipgloss.NewStyle().Foreground(chatrender.UserColor)
	errorBorderStyle       = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#FF6B6B", Dark: "#FF8787"})
)

// Command log pane styles (matches orchestration mode command_pane.go)
var (
	commandTimestampStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#666666", Dark: "#696969"})

	commandSourceStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#8888FF", Dark: "#9999FF"})

	commandTypeStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#CCCCCC", Dark: "#AAAAAA"})

	commandSuccessStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#43BF6D"})

	commandFailStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#FF6B6B", Dark: "#FF8787"})

	commandDurationStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#888888", Dark: "#777777"})

	commandIDStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#555555", Dark: "#666666"})

	commandTraceIDStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#777777", Dark: "#555555"})
)

// NewCoordinatorPanel creates a new coordinator panel.
// The panel starts unfocused - use Focus() to give it input focus.
// If debugMode is true, the command log tab is shown.
// If vimMode is true, vim keybindings are enabled in the input.
// Clipboard is used for copy operations during text selection.
func NewCoordinatorPanel(debugMode, vimMode bool, clipboard shared.Clipboard) *CoordinatorPanel {
	input := vimtextarea.New(vimtextarea.Config{
		VimEnabled:  vimMode,
		DefaultMode: vimtextarea.ModeInsert,
		Placeholder: "Message coordinator...",
		CharLimit:   0,
		MaxHeight:   4,
	})
	// Don't focus input by default - panel starts unfocused
	input.Blur()

	panel := &CoordinatorPanel{
		input:               input,
		clipboard:           clipboard,
		activeTab:           TabCoordinator,
		coordinatorMessages: make([]chatrender.Message, 0),
		messageEntries:      make([]message.Entry, 0),
		workerIDs:           make([]string, 0),
		workerPanes:         make(map[string]*selection.SelectablePane),
		workerMessages:      make(map[string][]chatrender.Message),
		workerStatus:        make(map[string]events.ProcessStatus),
		workerPhases:        make(map[string]events.ProcessPhase),
		workerQueues:        make(map[string]int),
		workerMetrics:       make(map[string]*metrics.TokenMetrics),
		commandLogViewport:  viewport.New(0, 0),
		commandLogEntries:   make([]CommandLogEntry, 0),
		commandLogDirty:     true,
		debugMode:           debugMode,
		focused:             false,
	}

	// Initialize SelectablePanes using the helper method
	panel.coordinatorPane = selection.NewPane(selection.PaneConfig{
		Clipboard: clipboard,
		MakeToast: panel.makeToastFunc(),
	})
	panel.messagePane = selection.NewPane(selection.PaneConfig{
		Clipboard: clipboard,
		MakeToast: panel.makeToastFunc(),
	})

	return panel
}

// makeToastFunc returns a toast factory function for SelectablePane.
func (p *CoordinatorPanel) makeToastFunc() selection.ToastFunc {
	return func(message string, isError bool) tea.Cmd {
		style := toaster.StyleSuccess
		if isError {
			style = toaster.StyleError
		}
		return func() tea.Msg {
			return mode.ShowToastMsg{Message: message, Style: style}
		}
	}
}

// SetSize updates the panel dimensions.
func (p *CoordinatorPanel) SetSize(width, height int) {
	p.width = width
	p.height = height
	// Update input size for proper soft-wrap calculation and scrolling
	// Width: panel width - 4 (2 for borders, 2 for padding)
	// Height: 4 lines (allows input to grow/scroll properly)
	p.input.SetSize(max(width-4, 1), 4)
}

// SetScreenXOffset sets the panel's X position on screen for mouse coordinate mapping.
func (p *CoordinatorPanel) SetScreenXOffset(offset int) {
	p.screenXOffset = offset
	p.coordinatorPane.SetScreenXOffset(offset)
	p.messagePane.SetScreenXOffset(offset)
	for _, pane := range p.workerPanes {
		pane.SetScreenXOffset(offset)
	}
}

// SetScreenYOffset sets the panel's Y position on screen for mouse coordinate mapping.
func (p *CoordinatorPanel) SetScreenYOffset(offset int) {
	viewportY := offset
	p.screenYOffset = viewportY
	p.coordinatorPane.SetScreenYOffset(viewportY)
	p.messagePane.SetScreenYOffset(viewportY)
	for _, pane := range p.workerPanes {
		pane.SetScreenYOffset(viewportY)
	}
}

// SetWorkflow updates the panel to show data for the given workflow.
// Syncs all state from WorkflowUIState including coordinator, messages, and workers.
func (p *CoordinatorPanel) SetWorkflow(workflowID controlplane.WorkflowID, state *WorkflowUIState) {
	workflowChanged := p.workflowID != workflowID
	p.workflowID = workflowID

	if state == nil {
		// Clear state for nil workflow
		p.coordinatorMessages = make([]chatrender.Message, 0)
		p.coordinatorStatus = events.ProcessStatusPending
		p.coordinatorQueue = 0
		p.coordinatorMetrics = nil
		p.messageEntries = make([]message.Entry, 0)
		p.workerIDs = make([]string, 0)
		clear(p.workerMetrics)
		return
	}

	// Sync coordinator state
	if workflowChanged || len(state.CoordinatorMessages) != len(p.coordinatorMessages) {
		p.coordinatorMessages = state.CoordinatorMessages
	}
	p.coordinatorStatus = state.CoordinatorStatus
	p.coordinatorQueue = state.CoordinatorQueueCount
	p.coordinatorMetrics = state.CoordinatorMetrics

	// Sync message log state
	if workflowChanged || len(state.MessageEntries) != len(p.messageEntries) {
		p.messageEntries = state.MessageEntries
	}

	// Sync worker state
	if workflowChanged || len(state.WorkerIDs) != len(p.workerIDs) {
		p.workerIDs = state.WorkerIDs
		// Initialize SelectablePanes for new workers
		for _, wid := range p.workerIDs {
			if _, exists := p.workerPanes[wid]; !exists {
				p.workerPanes[wid] = selection.NewPane(selection.PaneConfig{
					Clipboard: p.clipboard,
					MakeToast: p.makeToastFunc(),
				})
				p.workerPanes[wid].SetScreenXOffset(p.screenXOffset)
				p.workerPanes[wid].SetScreenYOffset(p.screenYOffset)
			}
		}
	}

	// Sync per-worker data
	for _, wid := range p.workerIDs {
		// Sync messages
		stateMessages := state.WorkerMessages[wid]
		if len(stateMessages) != len(p.workerMessages[wid]) {
			p.workerMessages[wid] = stateMessages
		}
		// Sync status and phase
		p.workerStatus[wid] = state.WorkerStatus[wid]
		p.workerPhases[wid] = state.WorkerPhases[wid]
		p.workerQueues[wid] = state.WorkerQueueCounts[wid]
	}

	// Sync worker metrics (clear first to avoid stale entries from previous workflow)
	clear(p.workerMetrics)
	if state.WorkerMetrics != nil {
		maps.Copy(p.workerMetrics, state.WorkerMetrics)
	}

	// Sync command log state (only relevant in debug mode)
	if p.debugMode {
		if workflowChanged || len(state.CommandLogEntries) != len(p.commandLogEntries) {
			p.commandLogEntries = state.CommandLogEntries
			p.commandLogDirty = true
		}
	}

	// If the active tab is a worker tab that no longer exists, reset to coordinator
	firstWorker := p.firstWorkerTabIndex()
	if p.activeTab >= firstWorker {
		workerIdx := p.activeTab - firstWorker
		if workerIdx >= len(p.workerIDs) {
			p.activeTab = TabCoordinator
		}
	}
}

// tabCount returns the total number of tabs (coordinator + messages + [cmdlog] + workers).
func (p *CoordinatorPanel) tabCount() int {
	return p.firstWorkerTabIndex() + len(p.workerIDs)
}

// firstWorkerTabIndex returns the tab index where worker tabs start.
// This accounts for the optional command log tab in debug mode.
func (p *CoordinatorPanel) firstWorkerTabIndex() int {
	if p.debugMode {
		return 3 // Coordinator(0), Messages(1), CommandLog(2), Workers(3+)
	}
	return TabFirstWorker // Coordinator(0), Messages(1), Workers(2+)
}

// NextTab switches to the next tab.
func (p *CoordinatorPanel) NextTab() {
	p.activeTab = (p.activeTab + 1) % p.tabCount()
}

// PrevTab switches to the previous tab.
func (p *CoordinatorPanel) PrevTab() {
	count := p.tabCount()
	p.activeTab = (p.activeTab - 1 + count) % count
}

// ActiveTab returns the current active tab index.
func (p *CoordinatorPanel) ActiveTab() int {
	return p.activeTab
}

// Update handles messages for the coordinator panel.
func (p *CoordinatorPanel) Update(msg tea.Msg) (*CoordinatorPanel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle tab navigation keys (always, regardless of focus)
		switch msg.String() {
		case "[":
			p.PrevTab()
			return p, nil
		case "]":
			p.NextTab()
			return p, nil
		}

		// Handle input when focused - forward all keys including ESC for vim mode switching
		if p.focused {
			var cmd tea.Cmd
			p.input, cmd = p.input.Update(msg)
			return p, cmd
		}

	case tea.MouseMsg:
		// Handle text selection and scrolling for the active tab
		switch p.activeTab {
		case TabCoordinator:
			if cmd := p.coordinatorPane.HandleMouse(msg); cmd != nil {
				return p, cmd
			}
		case TabMessages:
			if cmd := p.messagePane.HandleMouse(msg); cmd != nil {
				return p, cmd
			}
		default:
			// Worker tab
			firstWorker := p.firstWorkerTabIndex()
			workerIdx := p.activeTab - firstWorker
			if workerIdx >= 0 && workerIdx < len(p.workerIDs) {
				workerID := p.workerIDs[workerIdx]
				if pane, exists := p.workerPanes[workerID]; exists {
					if cmd := pane.HandleMouse(msg); cmd != nil {
						return p, cmd
					}
				}
			}
		}

	case vimtextarea.SubmitMsg:
		// Handle submit from input - parent will send the message
		content := strings.TrimSpace(msg.Content)
		if content != "" {
			p.input.Reset()
			return p, func() tea.Msg {
				return CoordinatorPanelSubmitMsg{
					WorkflowID: p.workflowID,
					Content:    content,
				}
			}
		}
	}

	return p, nil
}

// View renders the coordinator panel with tabs.
func (p *CoordinatorPanel) View() string {
	if p.width == 0 || p.height == 0 {
		return ""
	}

	// Calculate input height (6 lines: 3 content lines + 2 borders + 1 padding)
	inputHeight := 6
	contentHeight := p.height - inputHeight

	// Build tabs
	tabs := p.buildTabs(contentHeight)

	// Determine border color based on active tab's status
	borderColor := p.getActiveBorderColor()

	// Determine bottom-left indicator based on active tab
	bottomLeft := p.getActiveBottomIndicators()

	// Render the tabbed pane
	tabbedPane := panes.BorderedPane(panes.BorderConfig{
		Width:       p.width,
		Height:      contentHeight,
		Tabs:        tabs,
		ActiveTab:   p.activeTab,
		BorderColor: borderColor,
		BottomLeft:  bottomLeft,
		BottomRight: p.getActiveMetricsDisplay(),
	})

	// Render input pane with zone mark for click detection
	inputView := zone.Mark(zoneChatInput, p.renderInputPane(p.width, inputHeight))

	return lipgloss.JoinVertical(lipgloss.Left, tabbedPane, inputView)
}

// buildTabs constructs the tab slice for the panel.
// Tab labels have colored status indicators but muted text for inactive tabs.
func (p *CoordinatorPanel) buildTabs(contentHeight int) []panes.Tab {
	tabs := make([]panes.Tab, 0, p.tabCount())

	// Muted style for inactive tab text
	mutedStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)

	// Tab 0: Coordinator with status indicator
	coordIndicator, coordIndicatorStyle := chatrender.StatusIndicator(p.coordinatorStatus)
	coordLabel := p.formatTabLabel(coordIndicator, coordIndicatorStyle, "Coord", p.activeTab == TabCoordinator, mutedStyle)
	tabs = append(tabs, panes.Tab{
		Label:   coordLabel,
		Content: p.renderCoordinatorContent(contentHeight),
		Color:   coordinatorTitleColor,
		ZoneID:  makeTabZoneID(TabCoordinator),
	})

	// Tab 1: Messages (no status indicator)
	msgsLabel := "Msgs"
	if p.activeTab != TabMessages {
		msgsLabel = mutedStyle.Render(msgsLabel)
	}
	tabs = append(tabs, panes.Tab{
		Label:   msgsLabel,
		Content: p.renderMessageLogContent(contentHeight),
		ZoneID:  makeTabZoneID(TabMessages),
	})

	// Tab 2 (debug mode only): Command Log
	if p.debugMode {
		cmdLogLabel := "CmdLog"
		if p.activeTab != 2 {
			cmdLogLabel = mutedStyle.Render(cmdLogLabel)
		}
		tabs = append(tabs, panes.Tab{
			Label:   cmdLogLabel,
			Content: p.renderCommandLogContent(contentHeight),
			ZoneID:  makeTabZoneID(2),
		})
	}

	// Dynamic worker tabs with status indicators
	firstWorker := p.firstWorkerTabIndex()
	for i, workerID := range p.workerIDs {
		tabIndex := firstWorker + i
		status := p.workerStatus[workerID]
		indicator, indicatorStyle := chatrender.StatusIndicator(status)
		label := p.formatTabLabel(indicator, indicatorStyle, p.formatWorkerTabLabel(workerID), p.activeTab == tabIndex, mutedStyle)
		tabs = append(tabs, panes.Tab{
			Label:   label,
			Content: p.renderWorkerContent(workerID, contentHeight),
			Color:   workerTitleColor,
			ZoneID:  makeTabZoneID(tabIndex),
		})
	}

	return tabs
}

// formatTabLabel builds a tab label with colored indicator and conditionally muted text.
// When active, both indicator and text use their natural colors.
// When inactive, indicator stays colored but text becomes muted.
func (p *CoordinatorPanel) formatTabLabel(indicator string, indicatorStyle lipgloss.Style, text string, isActive bool, mutedStyle lipgloss.Style) string {
	styledIndicator := indicatorStyle.Render(indicator)
	if isActive {
		return styledIndicator + " " + text
	}
	return styledIndicator + " " + mutedStyle.Render(text)
}

// formatWorkerTabLabel returns a short label for a worker tab.
func (p *CoordinatorPanel) formatWorkerTabLabel(workerID string) string {
	// Extract just the number from worker IDs like "worker-1"
	if suffix, found := strings.CutPrefix(workerID, "worker-"); found {
		return "W" + suffix
	}
	// Truncate long worker IDs
	if len(workerID) > 6 {
		return workerID[:6]
	}
	return workerID
}

// getActiveBorderColor returns the border color based on the active tab's status.
func (p *CoordinatorPanel) getActiveBorderColor() lipgloss.AdaptiveColor {
	switch p.activeTab {
	case TabCoordinator:
		return chatrender.StatusBorderColor(p.coordinatorStatus)
	case TabMessages:
		return styles.BorderDefaultColor
	default:
		// Worker tab
		workerIdx := p.activeTab - TabFirstWorker
		if workerIdx >= 0 && workerIdx < len(p.workerIDs) {
			workerID := p.workerIDs[workerIdx]
			if status, ok := p.workerStatus[workerID]; ok {
				return chatrender.StatusBorderColor(status)
			}
		}
		return styles.BorderDefaultColor
	}
}

// getActiveBottomIndicators returns the bottom-left indicator for the active tab.
func (p *CoordinatorPanel) getActiveBottomIndicators() string {
	switch p.activeTab {
	case TabCoordinator:
		return chatrender.FormatQueueCount(p.coordinatorQueue)
	case TabMessages:
		return ""
	default:
		// Worker tab
		workerIdx := p.activeTab - TabFirstWorker
		if workerIdx >= 0 && workerIdx < len(p.workerIDs) {
			workerID := p.workerIDs[workerIdx]
			queueCount := p.workerQueues[workerID]
			return chatrender.FormatQueueCount(queueCount)
		}
		return ""
	}
}

// getActiveMetricsDisplay returns the metrics display string for the active tab.
// Returns formatted token usage (e.g., "27k/200k") for coordinator or worker tabs,
// or empty string for the message log tab or when no metrics are available.
func (p *CoordinatorPanel) getActiveMetricsDisplay() string {
	switch p.activeTab {
	case TabCoordinator:
		return chatrender.FormatMetricsDisplay(p.coordinatorMetrics)
	case TabMessages:
		return "" // No metrics for message log
	default:
		// Worker tab
		workerIdx := p.activeTab - TabFirstWorker
		if workerIdx >= 0 && workerIdx < len(p.workerIDs) {
			workerID := p.workerIDs[workerIdx]
			return chatrender.FormatMetricsDisplay(p.workerMetrics[workerID])
		}
		return ""
	}
}

// renderCoordinatorContent renders the coordinator chat content for the viewport.
// Also tracks line positions of each message for click-to-copy detection.
func (p *CoordinatorPanel) renderCoordinatorContent(height int) string {
	vpWidth := max(p.width-2, 1)
	vpHeight := max(height-2, 1)

	// Get selection bounds from pane
	selStart, selEnd := p.coordinatorPane.SelectionBounds()

	// Render content with selection highlighting
	content, plainLines := renderChatContentWithSelection(p.coordinatorMessages, vpWidth, chatrender.RenderConfig{
		AgentLabel: "Coordinator",
		AgentColor: coordinatorTitleColor,
		UserLabel:  "User",
	}, selStart, selEnd)

	// Pad both styled content and plain lines to keep them aligned
	content, plainLines, topPadding := padContentAndLinesToBottom(content, plainLines, vpHeight)

	// Update pane with content, plain lines, and padding offset for coordinate mapping
	p.coordinatorPane.SetSize(vpWidth, vpHeight)
	p.coordinatorPane.SetTopPadding(topPadding)
	p.coordinatorPane.SetContent(content, plainLines, true)
	p.coordinatorPane.ClearDirty()

	return p.coordinatorPane.View()
}

// renderMessageLogContent renders the message log content for the viewport.
func (p *CoordinatorPanel) renderMessageLogContent(height int) string {
	vpWidth := max(p.width-2, 1)
	vpHeight := max(height-2, 1)

	// Get selection bounds from pane
	selStart, selEnd := p.messagePane.SelectionBounds()

	// Render content with selection highlighting
	content, plainLines := p.renderMessageEntriesWithSelection(vpWidth, selStart, selEnd)

	// Pad both styled content and plain lines to keep them aligned
	content, plainLines, topPadding := padContentAndLinesToBottom(content, plainLines, vpHeight)

	// Update pane with content, plain lines, and padding offset for coordinate mapping
	p.messagePane.SetSize(vpWidth, vpHeight)
	p.messagePane.SetTopPadding(topPadding)
	p.messagePane.SetContent(content, plainLines, true)
	p.messagePane.ClearDirty()

	return p.messagePane.View()
}

// renderWorkerContent renders a worker's chat content for the viewport.
func (p *CoordinatorPanel) renderWorkerContent(workerID string, height int) string {
	vpWidth := max(p.width-2, 1)
	vpHeight := max(height-2, 1)

	// Get or create pane for this worker
	pane, exists := p.workerPanes[workerID]
	if !exists {
		pane = selection.NewPane(selection.PaneConfig{
			Clipboard: p.clipboard,
			MakeToast: p.makeToastFunc(),
		})
		pane.SetScreenXOffset(p.screenXOffset)
		pane.SetScreenYOffset(p.screenYOffset)
		p.workerPanes[workerID] = pane
	}

	// Get selection bounds from pane
	selStart, selEnd := pane.SelectionBounds()

	// Render content with selection highlighting
	messages := p.workerMessages[workerID]
	content, plainLines := renderChatContentWithSelection(messages, vpWidth, chatrender.RenderConfig{
		AgentLabel:              "Worker",
		AgentColor:              workerTitleColor,
		UserLabel:               "User",
		ShowCoordinatorInWorker: true,
	}, selStart, selEnd)

	// Pad both styled content and plain lines to keep them aligned
	content, plainLines, topPadding := padContentAndLinesToBottom(content, plainLines, vpHeight)

	// Update pane with content, plain lines, and padding offset for coordinate mapping
	pane.SetSize(vpWidth, vpHeight)
	pane.SetTopPadding(topPadding)
	pane.SetContent(content, plainLines, true)
	pane.ClearDirty()

	return pane.View()
}

// renderCommandLogContent renders the command log content for the viewport.
// This is shown in debug mode to display command processing activity.
func (p *CoordinatorPanel) renderCommandLogContent(height int) string {
	vpWidth := max(p.width-2, 1)
	vpHeight := max(height-2, 1)

	content := p.renderCommandLogEntries(vpWidth)
	content = padContentToBottom(content, vpHeight)

	// Update viewport
	wasAtBottom := p.commandLogViewport.AtBottom()
	p.commandLogViewport.Width = vpWidth
	p.commandLogViewport.Height = vpHeight
	p.commandLogViewport.SetContent(content)
	if wasAtBottom {
		p.commandLogViewport.GotoBottom()
	}

	p.commandLogDirty = false
	return p.commandLogViewport.View()
}

// renderCommandLogEntries renders the command log entries for the viewport.
// Format: "15:04:05 [source] command_type (id) ✓/✗ [traceID] (duration)"
func (p *CoordinatorPanel) renderCommandLogEntries(wrapWidth int) string {
	if len(p.commandLogEntries) == 0 {
		emptyStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)
		return emptyStyle.Render("No commands processed yet.")
	}

	var content strings.Builder

	// Display constants
	const (
		commandIDDisplayLength = 8
		traceIDDisplayLength   = 8
		maxErrorDisplayLength  = 200
	)

	for _, entry := range p.commandLogEntries {
		// Format timestamp
		timestamp := commandTimestampStyle.Render(entry.Timestamp.Format("15:04:05"))

		// Format source in brackets
		source := commandSourceStyle.Render("[" + string(entry.Source) + "]")

		// Format command type
		cmdType := commandTypeStyle.Render(string(entry.CommandType))

		// Format shortened command ID (last 8 chars)
		shortID := entry.CommandID
		if len(shortID) > commandIDDisplayLength {
			shortID = shortID[len(shortID)-commandIDDisplayLength:]
		}
		cmdID := commandIDStyle.Render("(" + shortID + ")")

		// Format status (success/failure)
		var status string
		if entry.Success {
			status = commandSuccessStyle.Render("✓")
		} else {
			// Truncate error message if too long
			errMsg := entry.Error
			if len(errMsg) > maxErrorDisplayLength {
				errMsg = errMsg[:maxErrorDisplayLength] + "..."
			}
			if errMsg != "" {
				status = commandFailStyle.Render("✗ " + errMsg)
			} else {
				status = commandFailStyle.Render("✗")
			}
		}

		// Format trace ID (abbreviated to first 8 chars, only show when present)
		var traceIDDisplay string
		if entry.TraceID != "" {
			shortTraceID := entry.TraceID
			if len(shortTraceID) > traceIDDisplayLength {
				shortTraceID = shortTraceID[:traceIDDisplayLength]
			}
			traceIDDisplay = " " + commandTraceIDStyle.Render("["+shortTraceID+"]")
		}

		// Format duration
		duration := commandDurationStyle.Render(fmt.Sprintf("(%s)", formatCommandDuration(entry.Duration)))

		// Build the line
		line := fmt.Sprintf("%s %s %s %s %s%s %s", timestamp, source, cmdType, cmdID, status, traceIDDisplay, duration)

		// Apply ANSI-aware truncation if line exceeds viewport width
		if ansi.StringWidth(line) > wrapWidth {
			line = ansi.Truncate(line, wrapWidth-3, "...")
		}

		content.WriteString(line)
		content.WriteString("\n")
	}

	return strings.TrimRight(content.String(), "\n")
}

// formatCommandDuration formats a duration for display in the command log.
func formatCommandDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

// renderMessageEntriesWithSelection renders the message log entries with optional selection highlighting.
// Returns: rendered content, plain text lines for selection extraction.
func (p *CoordinatorPanel) renderMessageEntriesWithSelection(wrapWidth int, selStart, selEnd *selection.Point) (string, []string) {
	if len(p.messageEntries) == 0 {
		emptyStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)
		return emptyStyle.Render("No inter-agent messages yet."), nil
	}

	var content strings.Builder
	var plainLines []string
	currentLine := 0

	for _, entry := range p.messageEntries {
		// Check if sender is a worker
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

		// Build plain header
		headerPlain := fmt.Sprintf("%s %s → %s", entry.Timestamp.Format("15:04"), entry.From, entry.To)

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

		// Format styled header: timestamp | SENDER → RECIPIENT
		headerStyled := fmt.Sprintf("%s %s → %s", timestamp, senderStyled, entry.To)

		// Word wrap content (account for left border + space)
		wrappedContent := chatrender.WordWrap(entry.Content, wrapWidth-4)
		wrappedLines := strings.Split(wrappedContent, "\n")

		// Build plain lines for this entry
		plainLines = append(plainLines, headerPlain)
		plainLines = append(plainLines, wrappedLines...)
		plainLines = append(plainLines, "") // blank line

		// Header line with optional selection
		content.WriteString(leftBorder + " " + renderLineWithSelection(headerStyled, headerPlain, currentLine, wrapWidth, selStart, selEnd))
		content.WriteString("\n")
		currentLine++

		// Content lines with optional selection (unstyled, matches coordinator pane)
		for _, line := range wrappedLines {
			content.WriteString(leftBorder + " " + renderLineWithSelection(line, line, currentLine, wrapWidth, selStart, selEnd))
			content.WriteString("\n")
			currentLine++
		}

		// Blank line
		content.WriteString(renderLineWithSelection("", "", currentLine, wrapWidth, selStart, selEnd))
		content.WriteString("\n")
		currentLine++
	}

	return strings.TrimRight(content.String(), "\n"), plainLines
}

// padContentToBottom pads content to push it to the bottom of the viewport.
func padContentToBottom(content string, vpHeight int) string {
	contentLines := strings.Split(content, "\n")
	if len(contentLines) < vpHeight {
		padding := make([]string, vpHeight-len(contentLines))
		contentLines = append(padding, contentLines...)
		content = strings.Join(contentLines, "\n")
	}
	return content
}

// padContentAndLinesToBottom pads both styled content and plain lines to keep them aligned.
// This ensures line indices match between styled viewport content and plain text for selection.
// Returns the padded content, padded plain lines, and the number of padding lines added.
func padContentAndLinesToBottom(content string, plainLines []string, vpHeight int) (string, []string, int) {
	contentLines := strings.Split(content, "\n")
	paddingCount := 0
	if len(contentLines) < vpHeight {
		paddingCount = vpHeight - len(contentLines)
		contentPadding := make([]string, paddingCount)
		plainPadding := make([]string, paddingCount)
		contentLines = append(contentPadding, contentLines...)
		plainLines = append(plainPadding, plainLines...)
	}
	return strings.Join(contentLines, "\n"), plainLines, paddingCount
}

// renderInputPane renders the input area.
func (p *CoordinatorPanel) renderInputPane(width, height int) string {
	// Get input view
	inputView := p.input.View()

	// Build content with explicit space padding (matches chatpanel pattern)
	inputWidth := width - 2 - 2 // borders and padding
	content := lipgloss.JoinHorizontal(lipgloss.Left,
		" ",
		lipgloss.NewStyle().Width(inputWidth).Render(inputView),
		" ",
	)

	// Use default border color for input pane (no highlighting)
	return panes.BorderedPane(panes.BorderConfig{
		Content:            content,
		Width:              width,
		Height:             height,
		BottomLeft:         p.input.ModeIndicator(),
		Focused:            false, // Don't show focused border styling
		TitleColor:         styles.BorderDefaultColor,
		FocusedBorderColor: styles.BorderDefaultColor,
		// PreWrapped true because vimtextarea handles its own soft-wrapping
		PreWrapped: true,
	})
}

// Focus gives focus to the input.
func (p *CoordinatorPanel) Focus() {
	p.focused = true
	p.input.Focus()
}

// Blur removes focus from the input.
func (p *CoordinatorPanel) Blur() {
	p.focused = false
	p.input.Blur()
}

// IsFocused returns whether the panel is focused.
func (p *CoordinatorPanel) IsFocused() bool {
	return p.focused
}

// IsInputInNormalMode returns true if the vim input is in normal mode.
// Used by parent to determine if ESC should close the panel.
func (p *CoordinatorPanel) IsInputInNormalMode() bool {
	return p.input.InNormalMode()
}

// selectionBgStyle is the background highlight for selected text.
var selectionBgStyle = lipgloss.NewStyle().Background(lipgloss.AdaptiveColor{Light: "#ADD8E6", Dark: "#264F78"})

// renderChatContentWithSelection renders chat messages with optional selection highlighting.
// If selStart and selEnd are provided, the selected range will be highlighted.
// Returns: rendered content, plain text lines for selection extraction.
func renderChatContentWithSelection(messages []chatrender.Message, wrapWidth int, cfg chatrender.RenderConfig, selStart, selEnd *selection.Point) (string, []string) {
	userLabel := cfg.UserLabel
	if userLabel == "" {
		userLabel = "You"
	}

	// Filter to non-empty text messages (skip tool calls)
	var filtered []chatrender.Message
	for _, msg := range messages {
		if msg.Content != "" && !msg.IsToolCall {
			filtered = append(filtered, msg)
		}
	}

	if len(filtered) == 0 {
		emptyStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor).PaddingLeft(1).PaddingBottom(1)
		return emptyStyle.Render("Waiting for the coordinator to initialize."), nil
	}

	var content strings.Builder
	var plainLines []string
	currentLine := 0

	for _, msg := range filtered {
		// Role label (plain text for selection)
		var rolePlain string
		var roleLabel string
		switch msg.Role {
		case "user":
			rolePlain = userLabel
			roleLabel = chatrender.RoleStyle.Foreground(chatrender.UserMessageStyle.GetForeground()).Render(userLabel)
		case "system":
			rolePlain = "System"
			roleLabel = chatrender.RoleStyle.Foreground(chatrender.SystemColor).Render("System")
		case "coordinator":
			if cfg.ShowCoordinatorInWorker {
				rolePlain = "Coordinator"
				roleLabel = chatrender.RoleStyle.Foreground(chatrender.CoordinatorColor).Render("Coordinator")
			} else {
				rolePlain = cfg.AgentLabel
				roleLabel = chatrender.RoleStyle.Foreground(cfg.AgentColor).Render(cfg.AgentLabel)
			}
		default:
			rolePlain = cfg.AgentLabel
			roleLabel = chatrender.RoleStyle.Foreground(cfg.AgentColor).Render(cfg.AgentLabel)
		}

		// Content lines (word-wrapped)
		wrapped := chatrender.WordWrap(msg.Content, wrapWidth-4)
		wrappedLines := strings.Split(wrapped, "\n")

		// Build plain lines for this message
		plainLines = append(plainLines, rolePlain)
		plainLines = append(plainLines, wrappedLines...)
		plainLines = append(plainLines, "") // blank line

		// Role line with optional selection
		content.WriteString(renderLineWithSelection(roleLabel, rolePlain, currentLine, wrapWidth, selStart, selEnd))
		content.WriteString("\n")
		currentLine++

		// Content lines with optional selection
		for _, line := range wrappedLines {
			content.WriteString(renderLineWithSelection(line, line, currentLine, wrapWidth, selStart, selEnd))
			content.WriteString("\n")
			currentLine++
		}

		// Blank line
		content.WriteString(renderLineWithSelection("", "", currentLine, wrapWidth, selStart, selEnd))
		content.WriteString("\n")
		currentLine++
	}

	return strings.TrimRight(content.String(), "\n"), plainLines
}

// renderLineWithSelection renders a line with selection highlighting applied.
func renderLineWithSelection(styledLine, plainLine string, lineNum, width int, selStart, selEnd *selection.Point) string {
	if selStart == nil || selEnd == nil {
		return styledLine
	}

	// Check if this line is within the selection range
	if lineNum < selStart.Line || lineNum > selEnd.Line {
		return styledLine
	}

	// Determine selection columns for this line
	startCol := 0
	endCol := len(plainLine)

	if lineNum == selStart.Line {
		startCol = selStart.Col
	}
	if lineNum == selEnd.Line {
		endCol = selEnd.Col
	}

	// Clamp to valid range
	if startCol > len(plainLine) {
		startCol = len(plainLine)
	}
	if endCol > len(plainLine) {
		endCol = len(plainLine)
	}
	if startCol >= endCol {
		return styledLine
	}

	// For simplicity, we'll highlight using the plain line
	// This loses ANSI styling but ensures accurate selection
	before := plainLine[:startCol]
	selected := plainLine[startCol:endCol]
	after := plainLine[endCol:]

	result := before + selectionBgStyle.Render(selected) + after

	// Pad to width for consistent appearance
	return padLineToWidth(result, width)
}

// padLineToWidth pads a line with spaces to the given width for consistent hover backgrounds.
func padLineToWidth(line string, width int) string {
	lineWidth := ansi.StringWidth(line)
	if lineWidth >= width {
		return line
	}
	return line + strings.Repeat(" ", width-lineWidth)
}

// CoordinatorPanelSubmitMsg is sent when the user submits a message.
type CoordinatorPanelSubmitMsg struct {
	WorkflowID controlplane.WorkflowID
	Content    string
}

// sendToCoordinator sends a message to the coordinator of the specified workflow.
func (m Model) sendToCoordinator(workflowID controlplane.WorkflowID, content string) tea.Cmd {
	return func() tea.Msg {
		if m.controlPlane == nil {
			return nil
		}

		// Get the workflow to access its v2 infrastructure
		wf, err := m.controlPlane.Get(context.Background(), workflowID)
		if err != nil || wf == nil {
			return nil
		}

		// Get the command submitter from the workflow's infrastructure
		if wf.Infrastructure == nil {
			return nil
		}

		cmdSubmitter := wf.Infrastructure.Core.CmdSubmitter
		if cmdSubmitter == nil {
			return nil
		}

		// Submit v2 command to send message to coordinator
		cmd := command.NewSendToProcessCommand(command.SourceUser, repository.CoordinatorID, content)
		cmdSubmitter.Submit(cmd)

		return nil
	}
}

// handleSlashCommand routes slash commands to their respective handlers.
// Returns (model, cmd) - unknown commands fall through to normal message routing.
func (m Model) handleSlashCommand(workflowID controlplane.WorkflowID, content string) (Model, tea.Cmd) {
	parts := strings.Fields(content)
	if len(parts) == 0 {
		return m, nil
	}

	slashCmd := parts[0]
	switch slashCmd {
	case "/stop":
		return m.handleStopCommand(workflowID, parts)
	case "/spawn":
		return m.handleSpawnCommand(workflowID)
	case "/retire":
		return m.handleRetireCommand(workflowID, parts)
	case "/replace":
		return m.handleReplaceCommand(workflowID, parts)
	default:
		// Unknown slash commands are sent to coordinator as-is
		return m, m.sendToCoordinator(workflowID, content)
	}
}

// handleStopCommand handles the /stop <process-id> [--force] command.
func (m Model) handleStopCommand(workflowID controlplane.WorkflowID, parts []string) (Model, tea.Cmd) {
	if len(parts) < 2 {
		return m, showWarning("Usage: /stop <process-id> [--force]")
	}

	processID := parts[1]
	force := len(parts) > 2 && parts[2] == "--force"

	return m, m.submitCommand(workflowID, func(submitter process.CommandSubmitter) {
		cmd := command.NewStopProcessCommand(command.SourceUser, processID, force, "user_requested")
		submitter.Submit(cmd)
	})
}

// handleSpawnCommand handles the /spawn command to spawn a new worker.
func (m Model) handleSpawnCommand(workflowID controlplane.WorkflowID) (Model, tea.Cmd) {
	return m, m.submitCommand(workflowID, func(submitter process.CommandSubmitter) {
		cmd := command.NewSpawnProcessCommand(command.SourceUser, repository.RoleWorker)
		submitter.Submit(cmd)
	})
}

// handleRetireCommand handles the /retire <worker-id> [reason] command.
func (m Model) handleRetireCommand(workflowID controlplane.WorkflowID, parts []string) (Model, tea.Cmd) {
	if len(parts) < 2 {
		return m, showWarning("Usage: /retire <worker-id> [reason]")
	}

	workerID := parts[1]

	// Block retiring the coordinator
	if workerID == repository.CoordinatorID {
		return m, showWarning("Cannot retire coordinator. Use /replace coordinator instead.")
	}

	reason := "user_requested"
	if len(parts) > 2 {
		reason = strings.Join(parts[2:], " ")
	}

	return m, m.submitCommand(workflowID, func(submitter process.CommandSubmitter) {
		cmd := command.NewRetireProcessCommand(command.SourceUser, workerID, reason)
		submitter.Submit(cmd)
	})
}

// handleReplaceCommand handles the /replace <process-id> [reason] command.
func (m Model) handleReplaceCommand(workflowID controlplane.WorkflowID, parts []string) (Model, tea.Cmd) {
	if len(parts) < 2 {
		return m, showWarning("Usage: /replace <process-id> [reason]")
	}

	processID := parts[1]

	reason := "user_requested"
	if len(parts) > 2 {
		reason = strings.Join(parts[2:], " ")
	}

	return m, m.submitCommand(workflowID, func(submitter process.CommandSubmitter) {
		cmd := command.NewReplaceProcessCommand(command.SourceUser, processID, reason)
		submitter.Submit(cmd)
	})
}

// showWarning returns a command that shows a warning toast.
func showWarning(msg string) tea.Cmd {
	return func() tea.Msg {
		return mode.ShowToastMsg{Message: msg, Style: toaster.StyleWarn}
	}
}

// submitCommand submits a command to the specified workflow's command submitter.
func (m Model) submitCommand(workflowID controlplane.WorkflowID, fn func(process.CommandSubmitter)) tea.Cmd {
	return func() tea.Msg {
		if m.controlPlane == nil {
			return nil
		}

		wf, err := m.controlPlane.Get(context.Background(), workflowID)
		if err != nil || wf == nil || wf.Infrastructure == nil {
			return nil
		}

		cmdSubmitter := wf.Infrastructure.Core.CmdSubmitter
		if cmdSubmitter == nil {
			return nil
		}

		fn(cmdSubmitter)
		return nil
	}
}
