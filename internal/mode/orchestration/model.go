// Package orchestration implements the three-pane orchestration mode TUI.
//
// The orchestration mode provides a visualization layer for coordinating
// multiple Claude agents working on an epic. It displays:
//   - Left pane (~25%): Interactive chat with the coordinator agent
//   - Middle pane (~40%): Message log from the epic's .msg issue
//   - Right pane (~35%): Cycleable output from worker agents
package orchestration

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zjrosen/perles/internal/mode"
	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/coordinator"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/orchestration/metrics"
	"github.com/zjrosen/perles/internal/orchestration/pool"
	"github.com/zjrosen/perles/internal/orchestration/session"
	"github.com/zjrosen/perles/internal/orchestration/workflow"
	"github.com/zjrosen/perles/internal/ui/commandpalette"
	"github.com/zjrosen/perles/internal/ui/shared/modal"
	"github.com/zjrosen/perles/internal/ui/shared/vimtextarea"
	"github.com/zjrosen/perles/internal/ui/styles"

	"github.com/zjrosen/perles/internal/pubsub"
)

// viewportKey is the map key for single-viewport panes (coordinator, message).
// Using a map instead of a direct field allows changes to persist in View methods
// since maps are reference types.
const viewportKey = "main"

// Fullscreen pane type constants
const (
	PaneNone        = 0
	PaneCoordinator = 1
	PaneMessages    = 2
	PaneWorker      = 3
)

// ChatMessage represents a single message in the coordinator chat history.
type ChatMessage struct {
	Role       string // "user" or "coordinator"
	Content    string
	IsToolCall bool // True if this is a tool call (for grouped rendering)
}

// InitPhase represents the current initialization phase.
type InitPhase int

const (
	InitNotStarted           InitPhase = iota
	InitCreatingWorkspace              // Consolidates client, pool, message log, MCP server creation
	InitSpawningCoordinator            // Coordinator process started
	InitAwaitingFirstMessage           // Waiting for coordinator's first response
	InitSpawningWorkers                // Workers are being spawned
	InitWorkersReady                   // All workers have reported ready
	InitReady
	InitFailed
	InitTimedOut
)

// InitTimeoutMsg signals initialization timeout (used by tests).
type InitTimeoutMsg struct{}

// SpinnerTickMsg advances the spinner frame.
type SpinnerTickMsg struct{}

// Model holds the orchestration mode state.
type Model struct {
	// Pane components
	coordinatorPane CoordinatorPane
	messagePane     MessagePane
	workerPane      WorkerPane

	// User input
	input   vimtextarea.Model
	vimMode vimtextarea.Mode // Track current vim mode for display

	// Error display (modal overlay)
	errorModal *modal.Model

	// Quit confirmation modal (nil when not showing)
	quitModal *modal.Model

	// Workflow state
	paused bool

	// Initialization state machine
	initializer  *Initializer
	initListener *pubsub.ContinuousListener[InitializerEvent]

	// Spinner animation frame (view-only, advanced by SpinnerTickMsg)
	spinnerFrame int

	// Backend integration (the actual coordinator and worker pool)
	coord              *coordinator.Coordinator
	pool               *pool.WorkerPool
	messageLog         *message.Issue
	mcpServer          *http.Server // HTTP MCP server for in-process tool handling
	mcpPort            int          // Dynamic port for MCP server
	workDir            string
	services           mode.Services
	coordinatorMetrics *metrics.TokenMetrics // Token usage and cost data for coordinator
	coordinatorWorking bool                  // True when coordinator is processing, false when waiting for input
	session            *session.Session      // Session tracking for this orchestration run

	// AI client configuration
	clientType  string // "claude" (default) or "amp"
	claudeModel string // Claude model: sonnet, opus, haiku
	ampModel    string // Amp model: opus, sonnet
	ampMode     string // Amp mode: free, rush, smart

	// Phased initialization state (used during startup)
	aiClient           client.HeadlessClient // Created during InitCreatingClient phase
	aiClientExtensions map[string]any        // Provider-specific extensions

	// Pub/sub subscriptions (initialized when coordinator starts)
	coordListener   *pubsub.ContinuousListener[events.CoordinatorEvent] // Coordinator events listener
	workerListener  *pubsub.ContinuousListener[events.WorkerEvent]      // Worker events listener
	messageListener *pubsub.ContinuousListener[message.Event]           // Message events listener
	ctx             context.Context                                     // Context for subscription lifetime
	cancel          context.CancelFunc                                  // Cancel function for subscriptions

	// Nudge batching (debounces coordinator nudges when multiple workers send messages)
	nudgeBatcher *NudgeBatcher

	// Message routing - who we're sending to (COORDINATOR or worker ID)
	messageTarget string

	// Fullscreen/navigation state
	navigationMode        bool // When true, input blurred and number keys select panes
	fullscreenPaneType    int  // Which pane type is fullscreen: 0=none, 1=coordinator, 2=messages, 3=worker
	fullscreenWorkerIndex int  // -1 = no fullscreen, 0-3 = worker index (only used when fullscreenPaneType=3)

	// Workflow template picker
	workflowPicker     *commandpalette.Model
	showWorkflowPicker bool
	workflowRegistry   *workflow.Registry

	// Coordinator refresh handoff state
	pendingRefresh bool // True when waiting for handoff before refresh

	// Dimensions
	width  int
	height int
}

// CoordinatorPane shows the chat with the coordinator agent.
type CoordinatorPane struct {
	messages      []ChatMessage
	viewports     map[string]viewport.Model // Use map so changes persist in View (maps are reference types)
	contentDirty  bool
	hasNewContent bool // True when new content arrived while scrolled up
}

// MessagePane shows the message log from the .msg issue.
type MessagePane struct {
	entries       []message.Entry
	viewports     map[string]viewport.Model // Use map so changes persist in View (maps are reference types)
	contentDirty  bool
	hasNewContent bool // True when new content arrived while scrolled up
}

// WorkerPane shows output from one worker at a time.
type WorkerPane struct {
	workerIndex    int      // Currently displayed worker
	workerIDs      []string // Worker IDs in display order (active workers only)
	workerStatus   map[string]events.WorkerStatus
	workerMessages map[string][]ChatMessage         // Structured messages per worker (like coordinator)
	workerMetrics  map[string]*metrics.TokenMetrics // Token usage and cost per worker
	viewports      map[string]viewport.Model        // Viewport per worker for scrolling
	contentDirty   map[string]bool                  // Dirty flag per worker
	hasNewContent  map[string]bool                  // True when new content arrived while scrolled up (per worker)
	retiredOrder   []string                         // Order in which workers retired (oldest first)
}

// Config holds configuration for creating an orchestration Model.
type Config struct {
	Services   mode.Services
	WorkDir    string
	ClientType string // "claude" (default) or "amp"
	// Claude-specific settings
	ClaudeModel string // sonnet (default), opus, haiku
	// Amp-specific settings
	AmpModel string // opus (default), sonnet
	AmpMode  string // free, rush, smart (default)
	// Workflow templates
	WorkflowRegistry *workflow.Registry // Pre-loaded workflow registry (optional)
	// UI settings
	VimMode bool // Enable vim keybindings in text input areas
}

// New creates a new orchestration mode model with the given configuration.
func New(cfg Config) Model {
	defaultMode := vimtextarea.ModeInsert // Start in Insert mode for immediate typing
	ta := vimtextarea.New(vimtextarea.Config{
		VimEnabled:  cfg.VimMode,
		DefaultMode: defaultMode,
		Placeholder: "Type message to coordinator...",
		CharLimit:   0,
		MaxHeight:   2, // Allow wrapping within 2 lines
	})
	ta.Focus() // Focus input by default

	return Model{
		input:                 ta,
		vimMode:               defaultMode, // Initialize mode tracking
		coordinatorPane:       newCoordinatorPane(),
		messagePane:           newMessagePane(),
		workerPane:            newWorkerPane(),
		services:              cfg.Services,
		workDir:               cfg.WorkDir,
		messageTarget:         "COORDINATOR", // Default to coordinator
		fullscreenWorkerIndex: -1,            // No fullscreen by default
		clientType:            cfg.ClientType,
		claudeModel:           cfg.ClaudeModel,
		ampModel:              cfg.AmpModel,
		ampMode:               cfg.AmpMode,
		workflowRegistry:      cfg.WorkflowRegistry,
	}
}

func newCoordinatorPane() CoordinatorPane {
	vps := make(map[string]viewport.Model)
	vps[viewportKey] = viewport.New(0, 0)
	return CoordinatorPane{
		messages:     make([]ChatMessage, 0),
		viewports:    vps,
		contentDirty: true, // Start dirty to trigger initial render
	}
}

func newMessagePane() MessagePane {
	vps := make(map[string]viewport.Model)
	vps[viewportKey] = viewport.New(0, 0)
	return MessagePane{
		entries:      make([]message.Entry, 0),
		viewports:    vps,
		contentDirty: true, // Start dirty to trigger initial render
	}
}

func newWorkerPane() WorkerPane {
	return WorkerPane{
		workerIDs:      make([]string, 0),
		workerStatus:   make(map[string]events.WorkerStatus),
		workerMessages: make(map[string][]ChatMessage),
		workerMetrics:  make(map[string]*metrics.TokenMetrics),
		viewports:      make(map[string]viewport.Model),
		contentDirty:   make(map[string]bool),
		hasNewContent:  make(map[string]bool),
	}
}

// Init returns initial commands for the mode.
func (m Model) Init() tea.Cmd {
	return func() tea.Msg { return StartCoordinatorMsg{} }
}

// SetSize handles terminal resize.
// It preserves scroll position proportionally when resizing.
func (m Model) SetSize(width, height int) Model {
	m.width = width
	m.height = height

	// Input takes full width (accounting for borders and padding)
	// Height is set to max allowed (4 lines) so content can grow
	// Actual visible height is controlled by calculateInputHeight()
	m.input.SetSize(width-4, 4)

	// Calculate pane dimensions (matches view.go layout)
	inputHeight := m.calculateInputHeight()
	contentHeight := max(height-inputHeight, 5)
	leftWidth := width * leftPanePercent / 100
	middleWidth := width * middlePanePercent / 100
	rightWidth := width - leftWidth - middleWidth

	// Update coordinator viewport with proportional scroll preservation
	coordVpWidth := max(leftWidth-2, 1)
	coordVpHeight := max(contentHeight-2, 1)
	if m.coordinatorPane.viewports == nil {
		m.coordinatorPane.viewports = make(map[string]viewport.Model)
		m.coordinatorPane.viewports[viewportKey] = viewport.New(0, 0)
	}
	m.coordinatorPane.viewports[viewportKey] = resizeViewportProportional(
		m.coordinatorPane.viewports[viewportKey], coordVpWidth, coordVpHeight)
	m.coordinatorPane.contentDirty = true // Re-render on resize

	// Update message pane viewport with proportional scroll preservation
	msgVpWidth := max(middleWidth-2, 1)
	msgVpHeight := max(contentHeight-2, 1)
	if m.messagePane.viewports == nil {
		m.messagePane.viewports = make(map[string]viewport.Model)
		m.messagePane.viewports[viewportKey] = viewport.New(0, 0)
	}
	m.messagePane.viewports[viewportKey] = resizeViewportProportional(
		m.messagePane.viewports[viewportKey], msgVpWidth, msgVpHeight)
	m.messagePane.contentDirty = true // Re-render on resize

	// Update worker pane viewports with proportional scroll preservation
	// Workers are stacked vertically, so they share the rightWidth
	numWorkers := len(m.workerPane.workerIDs)
	if numWorkers > 0 {
		// Calculate height per worker (matches renderWorkerPanes logic)
		minHeightPerWorker := 5
		heightPerWorker := max(contentHeight/numWorkers, minHeightPerWorker)

		workerVpWidth := max(rightWidth-2, 1)

		for i, workerID := range m.workerPane.workerIDs {
			// Last worker gets remaining height
			paneHeight := heightPerWorker
			if i == numWorkers-1 {
				paneHeight = contentHeight - (heightPerWorker * i)
			}
			workerVpHeight := max(paneHeight-2, 1)

			if vp, ok := m.workerPane.viewports[workerID]; ok {
				m.workerPane.viewports[workerID] = resizeViewportProportional(
					vp, workerVpWidth, workerVpHeight)
				m.workerPane.contentDirty[workerID] = true
			}
		}
	}

	// Update error modal size if present
	if m.errorModal != nil {
		m.errorModal.SetSize(width, height)
	}

	// Update workflow picker size if present
	if m.workflowPicker != nil {
		picker := m.workflowPicker.SetSize(width, height)
		m.workflowPicker = &picker
	}

	return m
}

// resizeViewportProportional resizes a viewport while preserving scroll position proportionally.
// If the user was at 50% scroll, they'll stay at 50% after resize.
// If at bottom (live view), stays at bottom.
func resizeViewportProportional(vp viewport.Model, newWidth, newHeight int) viewport.Model {
	// Capture scroll state before resize
	wasAtBottom := vp.AtBottom()
	oldPercent := vp.ScrollPercent()

	// Update dimensions
	vp.Width = newWidth
	vp.Height = newHeight

	// Restore scroll position
	if wasAtBottom {
		// Keep at bottom for live view experience
		vp.GotoBottom()
	} else if oldPercent > 0 {
		// Restore proportional position
		// Note: The actual content will be re-set by the render functions,
		// which will recalculate TotalLineCount. We store the percentage
		// and the render functions handle the rest via contentDirty flag.
		// For immediate effect, we estimate based on current content.
		totalLines := vp.TotalLineCount()
		if totalLines > vp.Height {
			newOffset := int(oldPercent * float64(totalLines-vp.Height))
			vp.SetYOffset(newOffset)
		}
	}

	return vp
}

// AddChatMessage appends a message to the coordinator chat history.
func (m Model) AddChatMessage(role, content string) Model {
	// Detect tool calls by the 🔧 prefix
	isToolCall := strings.HasPrefix(content, "🔧")

	m.coordinatorPane.messages = append(m.coordinatorPane.messages, ChatMessage{
		Role:       role,
		Content:    content,
		IsToolCall: isToolCall,
	})
	m.coordinatorPane.contentDirty = true

	// Track new content arrival when scrolled up
	if !m.coordinatorPane.viewports[viewportKey].AtBottom() {
		m.coordinatorPane.hasNewContent = true
	}

	return m
}

// SetMessageEntries updates the message log entries.
// Used in tests.
func (m Model) SetMessageEntries(entries []message.Entry) Model {
	// Only mark as new content if entries actually changed
	if len(entries) > len(m.messagePane.entries) {
		// Track new content arrival when scrolled up
		if !m.messagePane.viewports[viewportKey].AtBottom() {
			m.messagePane.hasNewContent = true
		}
	}
	m.messagePane.entries = entries
	m.messagePane.contentDirty = true
	return m
}

// AppendMessageEntry appends a single message entry to the message pane.
// Used by the real-time pub/sub handler for immediate updates.
func (m Model) AppendMessageEntry(entry message.Entry) Model {
	// Track new content arrival when scrolled up
	if !m.messagePane.viewports[viewportKey].AtBottom() {
		m.messagePane.hasNewContent = true
	}
	m.messagePane.entries = append(m.messagePane.entries, entry)
	m.messagePane.contentDirty = true
	return m
}

// UpdateWorker updates the status for a worker.
// If the status is WorkerRetired, the worker is removed from the active display list
// but its viewport data is retained for cleanup based on retirement order.
func (m Model) UpdateWorker(workerID string, status events.WorkerStatus) Model {
	if status == events.WorkerRetired {
		// Check if this worker is currently fullscreen and exit fullscreen if so
		if m.fullscreenPaneType == PaneWorker && m.fullscreenWorkerIndex >= 0 {
			// Build active workers list to find the retiring worker's index
			var activeWorkerIDs []string
			for _, wID := range m.workerPane.workerIDs {
				wStatus := m.workerPane.workerStatus[wID]
				if wStatus != events.WorkerRetired {
					activeWorkerIDs = append(activeWorkerIDs, wID)
				}
			}

			// Find the index of the retiring worker in the active list
			for i, wID := range activeWorkerIDs {
				if wID == workerID && i == m.fullscreenWorkerIndex {
					// This worker is currently fullscreen, exit fullscreen
					m.fullscreenPaneType = PaneNone
					m.fullscreenWorkerIndex = -1
					break
				}
			}
		}

		// Remove retired worker from active display list
		m.workerPane.workerIDs = slices.DeleteFunc(m.workerPane.workerIDs, func(id string) bool {
			return id == workerID
		})

		// Track retirement order (only if not already retired)
		if !slices.Contains(m.workerPane.retiredOrder, workerID) {
			m.workerPane.retiredOrder = append(m.workerPane.retiredOrder, workerID)
		}

		// Update status to retired (keep other data for now)
		m.workerPane.workerStatus[workerID] = status

		// Adjust display index if needed
		if m.workerPane.workerIndex >= len(m.workerPane.workerIDs) && m.workerPane.workerIndex > 0 {
			m.workerPane.workerIndex = len(m.workerPane.workerIDs) - 1
		}

		// Cleanup oldest retired workers if over limit
		m = m.cleanupRetiredWorkerViewports()
		return m
	}

	// Add to worker list if new
	if !slices.Contains(m.workerPane.workerIDs, workerID) {
		m.workerPane.workerIDs = append(m.workerPane.workerIDs, workerID)
	}

	m.workerPane.workerStatus[workerID] = status
	return m
}

// AddWorkerMessage appends a message to a worker's chat history.
func (m Model) AddWorkerMessage(workerID, content string) Model {
	return m.AddWorkerMessageWithRole(workerID, "worker", content)
}

// AddWorkerMessageWithRole appends a message to a worker's chat history with a specific role.
// Role can be "worker" or "coordinator" to indicate who sent the message.
func (m Model) AddWorkerMessageWithRole(workerID, role, content string) Model {
	// Detect tool calls by prefix
	isToolCall := strings.HasPrefix(content, "🔧")

	messages := m.workerPane.workerMessages[workerID]
	messages = append(messages, ChatMessage{
		Role:       role,
		Content:    content,
		IsToolCall: isToolCall,
	})
	m.workerPane.workerMessages[workerID] = messages

	// Mark content as dirty for this worker
	m.workerPane.contentDirty[workerID] = true

	// Track new content arrival when scrolled up
	if vp, ok := m.workerPane.viewports[workerID]; ok && !vp.AtBottom() {
		m.workerPane.hasNewContent[workerID] = true
	}

	return m
}

// CycleWorker moves to the next or previous worker in the list.
func (m Model) CycleWorker(forward bool) Model {
	if len(m.workerPane.workerIDs) == 0 {
		return m
	}

	if forward {
		m.workerPane.workerIndex = (m.workerPane.workerIndex + 1) % len(m.workerPane.workerIDs)
	} else {
		m.workerPane.workerIndex = (m.workerPane.workerIndex - 1 + len(m.workerPane.workerIDs)) % len(m.workerPane.workerIDs)
	}
	return m
}

// CurrentWorkerID returns the ID of the currently displayed worker, or empty if none.
func (m Model) CurrentWorkerID() string {
	if len(m.workerPane.workerIDs) == 0 {
		return ""
	}
	return m.workerPane.workerIDs[m.workerPane.workerIndex]
}

// WorkerCount returns the total number of workers and active count.
func (m Model) WorkerCount() (total, active int) {
	total = len(m.workerPane.workerIDs)
	for _, status := range m.workerPane.workerStatus {
		if !status.IsDone() {
			active++
		}
	}
	return
}

// ActiveWorkerIDs returns a list of worker IDs that are not retired.
// Used for filtering active workers in multiple locations.
func (m Model) ActiveWorkerIDs() []string {
	var active []string
	for _, workerID := range m.workerPane.workerIDs {
		if m.workerPane.workerStatus[workerID] != pool.WorkerRetired {
			active = append(active, workerID)
		}
	}
	return active
}

// CycleMessageTarget cycles through available message targets (COORDINATOR, BROADCAST, workers).
func (m Model) CycleMessageTarget() Model {
	// Build list of all targets: COORDINATOR, BROADCAST, then any workers
	targets := []string{"COORDINATOR", "BROADCAST"}
	targets = append(targets, m.workerPane.workerIDs...)

	// Find current index
	currentIdx := 0
	for i, t := range targets {
		if t == m.messageTarget {
			currentIdx = i
			break
		}
	}

	// Cycle to next
	nextIdx := (currentIdx + 1) % len(targets)
	m.messageTarget = targets[nextIdx]

	// Update input placeholder based on target
	switch m.messageTarget {
	case "COORDINATOR":
		m.input.SetPlaceholder("Type message to coordinator...")
	case "BROADCAST":
		m.input.SetPlaceholder("Type message to everyone...")
	default:
		m.input.SetPlaceholder("Type message to " + strings.ToUpper(m.messageTarget) + "...")
	}

	return m
}

// SetError displays an error in a modal overlay.
// Clears any active quit confirmation modal since errors take priority.
func (m Model) SetError(msg string) Model {
	mdl := modal.New(modal.Config{
		Title:       "Error",
		Message:     msg + "\n\nPress Esc to dismiss",
		HideButtons: true,
	})
	mdl.SetSize(m.width, m.height)
	m.errorModal = &mdl
	m.quitModal = nil // Clear quit modal - error takes priority
	return m
}

// ClearError clears the error display.
func (m Model) ClearError() Model {
	m.errorModal = nil
	return m
}

// showQuitConfirmation creates and shows the quit confirmation modal.
// Uses ButtonDanger for destructive action emphasis.
func (m Model) showQuitConfirmation() Model {
	mdl := modal.New(modal.Config{
		Title:          "Exit Orchestration Mode?",
		Message:        "Active workers will be stopped.\n\nPress Enter to exit or Esc to cancel.",
		ConfirmVariant: modal.ButtonDanger,
	})
	mdl.SetSize(m.width, m.height)
	m.quitModal = &mdl
	return m
}

// Coordinator returns the coordinator instance, if any.
func (m Model) Coordinator() *coordinator.Coordinator {
	return m.coord
}

// Pool returns the worker pool instance, if any.
func (m Model) Pool() *pool.WorkerPool {
	return m.pool
}

// MCPServer returns the HTTP MCP server instance, if any.
func (m Model) MCPServer() *http.Server {
	return m.mcpServer
}

// toggleNavigationMode toggles between normal and navigation mode.
// In navigation mode, input is blurred and number keys select panes.
func (m Model) toggleNavigationMode() Model {
	m.navigationMode = !m.navigationMode
	if m.navigationMode {
		m.input.Blur()
	} else {
		m.input.Focus()
		// Exit fullscreen when leaving navigation mode
		m.fullscreenPaneType = PaneNone
		m.fullscreenWorkerIndex = -1
	}
	return m
}

// exitNavigationMode exits navigation mode and returns to normal mode.
func (m Model) exitNavigationMode() Model {
	m.navigationMode = false
	m.fullscreenPaneType = PaneNone
	m.fullscreenWorkerIndex = -1
	m.input.Focus()
	return m
}

// toggleFullscreenPane toggles a pane between fullscreen and normal view.
// paneType is one of PaneCoordinator, PaneMessages, or PaneWorker.
// workerIndex is only used when paneType is PaneWorker.
func (m Model) toggleFullscreenPane(paneType int, workerIndex int) Model {
	// For worker panes, validate the worker index
	if paneType == PaneWorker {
		activeWorkerIDs := m.ActiveWorkerIDs()
		if workerIndex >= len(activeWorkerIDs) {
			return m
		}
	}

	// Toggle: if already fullscreen on this pane, exit fullscreen
	if m.fullscreenPaneType == paneType && (paneType != PaneWorker || m.fullscreenWorkerIndex == workerIndex) {
		m.fullscreenPaneType = PaneNone
		m.fullscreenWorkerIndex = -1
	} else {
		m.fullscreenPaneType = paneType
		if paneType == PaneWorker {
			m.fullscreenWorkerIndex = workerIndex
		} else {
			m.fullscreenWorkerIndex = -1
		}
	}

	return m
}

// CancelSubscriptions cancels the pub/sub subscription context.
// This cleans up subscription goroutines when exiting orchestration mode.
// Safe to call multiple times or on nil cancel function.
func (m *Model) CancelSubscriptions() {
	if m.cancel != nil {
		m.cancel()
	}
	if m.nudgeBatcher != nil {
		m.nudgeBatcher.Stop()
	}
}

// cleanup cleans up any partial initialization state before retrying.
// This is called when the user presses R to retry after a failed or timed out initialization.
func (m *Model) cleanup() {
	// Cancel any active subscriptions
	m.CancelSubscriptions()

	// Shutdown MCP server if running
	if m.mcpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = m.mcpServer.Shutdown(ctx)
		cancel()
		m.mcpServer = nil
	}

	// Stop coordinator if running
	if m.coord != nil {
		_ = m.coord.Cancel()
		m.coord = nil
	}

	// Clear pool
	m.pool = nil

	// Clear message log
	m.messageLog = nil

	// Clear AI client
	m.aiClient = nil
	m.aiClientExtensions = nil

	// Reset listeners
	m.coordListener = nil
	m.workerListener = nil
	m.messageListener = nil
	m.ctx = nil
	m.cancel = nil
	m.nudgeBatcher = nil
}

// openWorkflowPicker creates and shows the workflow picker modal.
// If no workflow registry is available, this is a no-op.
func (m Model) openWorkflowPicker() Model {
	if m.workflowRegistry == nil {
		return m
	}

	// Build items from workflow registry
	workflows := m.workflowRegistry.List()
	items := make([]commandpalette.Item, 0, len(workflows))
	for _, wf := range workflows {
		// Color based on source: blue for built-in, green for user-defined
		var color lipgloss.TerminalColor
		if wf.Source == workflow.SourceUser {
			color = styles.IssueFeatureColor // Green
		} else {
			color = styles.StatusInProgressColor // Blue
		}
		items = append(items, commandpalette.Item{
			ID:          wf.ID,
			Name:        wf.Name,
			Description: wf.Description,
			Color:       color,
		})
	}

	// Create the picker
	picker := commandpalette.New(commandpalette.Config{
		Title:       "Workflow Templates",
		Placeholder: "Search workflows...",
		Items:       items,
	})
	picker = picker.SetSize(m.width, m.height)
	m.workflowPicker = &picker
	m.showWorkflowPicker = true

	return m
}

// handleWorkflowSelected handles when a workflow is selected from the picker.
func (m Model) handleWorkflowSelected(item commandpalette.Item) (Model, tea.Cmd) {
	m.showWorkflowPicker = false
	m.workflowPicker = nil

	return m.sendWorkflowToCoordinator(item.ID)
}

// sendWorkflowToCoordinator sends the selected workflow content to the coordinator.
func (m Model) sendWorkflowToCoordinator(workflowID string) (Model, tea.Cmd) {
	if m.workflowRegistry == nil {
		m = m.SetError("Workflow registry not available")
		return m, nil
	}

	wf, ok := m.workflowRegistry.Get(workflowID)
	if !ok {
		m = m.SetError("Workflow not found: " + workflowID)
		return m, nil
	}

	// Format as instruction to coordinator
	content := fmt.Sprintf("[WORKFLOW: %s]\n\n%s", wf.Name, wf.Content)

	return m.handleUserInputToCoordinator(content)
}
