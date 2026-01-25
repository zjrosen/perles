// Package dashboard implements the multi-workflow dashboard TUI mode.
//
// The dashboard provides a centralized view of all running workflows with:
//   - Resource summary bar showing workflow/worker/token counts
//   - Workflow list with status, priority, health, and resource usage
//   - Quick actions for starting, pausing, and stopping workflows
//   - Real-time updates via ControlPlane event subscription
package dashboard

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"

	beads "github.com/zjrosen/perles/internal/beads/domain"
	appgit "github.com/zjrosen/perles/internal/git/application"
	domaingit "github.com/zjrosen/perles/internal/git/domain"
	"github.com/zjrosen/perles/internal/mode"
	"github.com/zjrosen/perles/internal/orchestration/controlplane"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/orchestration/metrics"
	"github.com/zjrosen/perles/internal/orchestration/v2/processor"
	appreg "github.com/zjrosen/perles/internal/registry/application"
	"github.com/zjrosen/perles/internal/ui/details"
	"github.com/zjrosen/perles/internal/ui/modals/help"
	"github.com/zjrosen/perles/internal/ui/shared/chatrender"
	"github.com/zjrosen/perles/internal/ui/shared/table"
	"github.com/zjrosen/perles/internal/ui/shared/toaster"
	"github.com/zjrosen/perles/internal/ui/shared/vimtextarea"
	"github.com/zjrosen/perles/internal/ui/tree"
)

// heartbeatRefreshInterval is how often to refresh the view for heartbeat display updates.
const heartbeatRefreshInterval = 5 * time.Second

// heartbeatTickMsg triggers a view refresh for heartbeat display.
type heartbeatTickMsg struct{}

// DashboardFocus represents which zone has focus in the dashboard.
type DashboardFocus int

const (
	// FocusTable indicates the workflow table has focus.
	FocusTable DashboardFocus = iota
	// FocusEpicView indicates the epic tree/details section has focus.
	FocusEpicView
	// FocusCoordinator indicates the coordinator chat panel has focus.
	FocusCoordinator
)

// EpicViewFocus represents which pane within the epic view has focus.
type EpicViewFocus int

const (
	// EpicFocusTree indicates the tree pane has focus.
	EpicFocusTree EpicViewFocus = iota
	// EpicFocusDetails indicates the details pane has focus.
	EpicFocusDetails
)

// epicTreeLoadedMsg is sent when the epic tree data has been loaded.
type epicTreeLoadedMsg struct {
	Issues []beads.Issue
	RootID string
	Err    error
}

// Model holds the dashboard mode state.
type Model struct {
	// ControlPlane provides workflow management and event subscription
	controlPlane controlplane.ControlPlane

	// Services contains shared dependencies
	services mode.Services

	// RegistryService provides template listing, validation, and access to epic_driven.md
	registryService *appreg.RegistryService

	// WorkflowCreator creates epics and tasks in beads from workflow DAGs
	workflowCreator *appreg.WorkflowCreator

	// Workflow state
	workflows         []*controlplane.WorkflowInstance
	selectedIndex     int
	tableScrollOffset int               // Persisted scroll offset for workflow table
	workflowTable     table.Model       // Shared table component for workflow list
	tableConfigCache  table.TableConfig // Cached table config to avoid recreating closures
	lastTableFocus    bool              // Track focus state to detect when config needs rebuild
	workflowList      WorkflowList      // Component for sorting/filtering state
	resourceSummary   ResourceSummary   // Component for resource bar

	// New workflow modal state (nil when not showing modal)
	newWorkflowModal *NewWorkflowModal

	// Help modal state
	showHelp  bool
	helpModal help.Model

	// Filter state
	filter FilterState

	// Per-workflow UI state cache (kept for future detail view)
	workflowUIState map[controlplane.WorkflowID]*WorkflowUIState

	// Coordinator chat panel (shown on right side when toggled)
	coordinatorPanel     *CoordinatorPanel
	showCoordinatorPanel bool

	// Epic tree view state (always visible section below workflow table)
	epicTree         *tree.Model    // Tree component for epic task hierarchy
	epicDetails      details.Model  // Details component for selected issue
	hasEpicDetail    bool           // Whether epicDetails has valid content
	epicViewFocus    EpicViewFocus  // Which pane within epic view has focus
	lastLoadedEpicID string         // ID of the last loaded epic (for stale response detection)
	focus            DashboardFocus // Which zone has focus (table, epic, coordinator)

	// Event subscription (global - all workflows)
	eventCh     <-chan controlplane.ControlPlaneEvent
	unsubscribe func()
	ctx         context.Context
	cancel      context.CancelFunc

	// Git worktree support
	gitExecutorFactory func(path string) appgit.GitExecutor
	workDir            string

	// API server port (for display in header)
	apiPort int

	// Debug mode enables command log tab in coordinator panel
	debugMode bool

	// Vim mode enables vim keybindings in text input areas
	vimMode bool

	// Dimensions
	width  int
	height int
}

// WorkflowTableRow wraps a workflow with its display index for table rendering.
type WorkflowTableRow struct {
	Index           int                            // 1-based row number
	Workflow        *controlplane.WorkflowInstance // The workflow data
	HasNotification bool                           // Whether this workflow has a pending notification
}

// Config holds configuration for creating a dashboard Model.
type Config struct {
	ControlPlane controlplane.ControlPlane
	Services     mode.Services
	// RegistryService provides template listing, validation, and access to epic_driven.md.
	// If nil, template listing returns empty options.
	RegistryService *appreg.RegistryService
	// WorkflowCreator creates epics and tasks in beads from workflow DAGs.
	// If nil, epic creation is skipped and workflow is created directly.
	WorkflowCreator *appreg.WorkflowCreator
	// GitExecutorFactory creates git executors for worktree operations.
	// If nil, worktree options are disabled in the new workflow modal.
	GitExecutorFactory func(path string) appgit.GitExecutor
	// WorkDir is the application root directory (where perles was invoked).
	// Used to create git executors for the current working directory.
	WorkDir string
	// APIPort is the port the HTTP API server is running on.
	// Shown in the dashboard header for external tool integration.
	APIPort int
	// DebugMode enables the command log tab in the coordinator panel.
	// When true, an additional tab showing command processing activity is displayed.
	DebugMode bool
	// VimMode enables vim keybindings in text input areas.
	// When true, the coordinator panel input uses vim mode.
	VimMode bool
}

// New creates a new dashboard mode model with the given configuration.
func New(cfg Config) Model {
	ctx, cancel := context.WithCancel(context.Background())

	m := Model{
		controlPlane:       cfg.ControlPlane,
		services:           cfg.Services,
		registryService:    cfg.RegistryService,
		workflowCreator:    cfg.WorkflowCreator,
		workflows:          make([]*controlplane.WorkflowInstance, 0),
		selectedIndex:      0,
		workflowList:       NewWorkflowList(),
		resourceSummary:    NewResourceSummary(),
		helpModal:          help.NewDashboard(),
		filter:             NewFilterState(),
		workflowUIState:    make(map[controlplane.WorkflowID]*WorkflowUIState),
		focus:              FocusTable,
		ctx:                ctx,
		cancel:             cancel,
		gitExecutorFactory: cfg.GitExecutorFactory,
		workDir:            cfg.WorkDir,
		apiPort:            cfg.APIPort,
		debugMode:          cfg.DebugMode,
		vimMode:            cfg.VimMode,
	}

	// Initialize the workflow table with config
	m.tableConfigCache = m.createWorkflowTableConfig()
	m.lastTableFocus = m.focus == FocusTable
	m.workflowTable = table.New(m.tableConfigCache)

	return m
}

// Init returns initial commands for the mode.
// It subscribes to ControlPlane events and loads the initial workflow list.
func (m Model) Init() tea.Cmd {
	// Play welcome sound when entering orchestration dashboard
	m.services.Sounds.Play("orchestration_welcome", "orchestration_welcome")

	return tea.Batch(
		m.subscribeToEvents(),
		m.loadWorkflows(),
		m.startHeartbeatTick(),
	)
}

// startHeartbeatTick returns a command that triggers periodic view refreshes for heartbeat display.
func (m Model) startHeartbeatTick() tea.Cmd {
	return tea.Tick(heartbeatRefreshInterval, func(time.Time) tea.Msg {
		return heartbeatTickMsg{}
	})
}

// Update handles messages and returns updated model and commands.
func (m Model) Update(msg tea.Msg) (mode.Controller, tea.Cmd) {
	// Handle heartbeat tick regardless of modal state - this keeps the UI refreshing
	// for time-based displays (health, uptime) even when modals are open
	if _, ok := msg.(heartbeatTickMsg); ok {
		return m, m.startHeartbeatTick()
	}

	// If new workflow modal is open, delegate to modal
	if m.newWorkflowModal != nil {
		switch msg := msg.(type) {
		case CreateWorkflowMsg:
			m.newWorkflowModal = nil
			// Always start the workflow immediately after creation
			if msg.WorkflowID != "" {
				return m, tea.Batch(
					m.startWorkflow(msg.WorkflowID),
					m.loadWorkflows(),
				)
			}
			return m, m.loadWorkflows()

		case CancelNewWorkflowMsg:
			m.newWorkflowModal = nil
			return m, nil

		case tea.WindowSizeMsg:
			m.width = msg.Width
			m.height = msg.Height
			m.newWorkflowModal = m.newWorkflowModal.SetSize(msg.Width, msg.Height)
			return m, nil

		case controlplane.ControlPlaneEvent:
			// Handle control plane events even when modal is open to maintain event subscription.
			// This is critical: the listenForEvents() goroutine must be restarted after each event,
			// otherwise we stop receiving events entirely.
			return m.handleControlPlaneEvent(msg)

		case eventSubscriptionReadyMsg:
			// Handle event subscription setup even when modal is open.
			// The subscription is initiated by Init() and may complete after the modal opens.
			m.eventCh = msg.eventCh
			m.unsubscribe = msg.unsubscribe
			return m, m.listenForEvents()

		default:
			var cmd tea.Cmd
			m.newWorkflowModal, cmd = m.newWorkflowModal.Update(msg)
			return m, cmd
		}
	}

	// Handle mouse events for zone clicks and scrolling
	if mouseMsg, ok := msg.(tea.MouseMsg); ok {
		return m.handleMouseMsg(mouseMsg)
	}

	// If coordinator panel is open and in insert mode, forward key events to it
	// This ensures typing in the chat input works correctly before focus cycling intercepts
	if m.showCoordinatorPanel && m.coordinatorPanel != nil && m.coordinatorPanel.IsFocused() && !m.coordinatorPanel.IsInputInNormalMode() {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			// Ctrl+C shows quit modal (consistent with other modes)
			if keyMsg.String() == "ctrl+c" {
				return m, func() tea.Msg { return QuitMsg{} }
			}

			// Allow tab/shift+tab to pass through for focus cycling even in insert mode
			if keyMsg.Type == tea.KeyTab || keyMsg.Type == tea.KeyShiftTab ||
				keyMsg.String() == "ctrl+n" || keyMsg.String() == "ctrl+p" {
				// Fall through to handleKeyMsg for focus cycling
			} else {
				// Forward all other key events to panel (ESC will switch to normal mode via vimtextarea)
				var cmd tea.Cmd
				m.coordinatorPanel, cmd = m.coordinatorPanel.Update(msg)
				return m, cmd
			}
		}
	}

	// Dashboard view handling
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case workflowsLoadedMsg:
		// Preserve selection by workflow ID when list is reloaded.
		// Workflows are sorted newest-first, so indices change when new workflows are created.
		// Without this, the panel silently switches to a different workflow.
		previouslySelectedID := controlplane.WorkflowID("")
		if m.SelectedWorkflow() != nil {
			previouslySelectedID = m.SelectedWorkflow().ID
		}

		m.workflows = msg.workflows
		m.workflowList = m.workflowList.SetWorkflows(m.workflows)
		m.resourceSummary = m.resourceSummary.Update(m.workflows)

		// Find the previously selected workflow's new index in the reordered list
		if previouslySelectedID != "" {
			for i, wf := range m.workflows {
				if wf.ID == previouslySelectedID {
					m.selectedIndex = i
					break
				}
			}
			// If not found (workflow was removed), clamp to valid range
			if m.selectedIndex >= len(m.workflows) {
				m.selectedIndex = max(0, len(m.workflows)-1)
			}
		}

		// If no workflows, automatically open the new workflow modal
		if len(m.workflows) == 0 && m.newWorkflowModal == nil {
			return m.openNewWorkflowModal()
		}

		// Load cached state for initial selection if needed
		if len(m.workflows) > 0 {
			m.loadSelectedWorkflowState()
		}
		// Open coordinator panel by default if not already open
		if !m.showCoordinatorPanel && len(m.workflows) > 0 {
			m.openCoordinatorPanelForSelected()
		} else if m.showCoordinatorPanel && m.coordinatorPanel != nil {
			// Panel is already open - sync it with current selection
			// (workflow list may have been reordered after new workflow created)
			wf := m.SelectedWorkflow()
			if wf != nil {
				uiState := m.getOrCreateUIState(wf.ID)
				m.coordinatorPanel.SetWorkflow(wf.ID, uiState)
			}
		}

		// Trigger epic tree load for the selected workflow
		cmd := m.triggerEpicTreeLoad()
		return m, cmd

	case eventSubscriptionReadyMsg:
		m.eventCh = msg.eventCh
		m.unsubscribe = msg.unsubscribe
		return m, m.listenForEvents()

	case controlplane.ControlPlaneEvent:
		return m.handleControlPlaneEvent(msg)

	case StartWorkflowFailedMsg:
		return m.handleStartWorkflowFailed(msg)

	case CoordinatorPanelSubmitMsg:
		// Check for slash commands first
		if strings.HasPrefix(msg.Content, "/") {
			return m.handleSlashCommand(msg.WorkflowID, msg.Content)
		}
		// Send message to coordinator
		return m, m.sendToCoordinator(msg.WorkflowID, msg.Content)

	case vimtextarea.SubmitMsg:
		// Forward to coordinator panel if open
		if m.showCoordinatorPanel && m.coordinatorPanel != nil {
			var cmd tea.Cmd
			m.coordinatorPanel, cmd = m.coordinatorPanel.Update(msg)
			return m, cmd
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Update coordinator panel size if visible
		if m.coordinatorPanel != nil {
			m.coordinatorPanel.SetSize(CoordinatorPanelWidth, m.height)
		}
		return m, nil

	case epicTreeLoadedMsg:
		return m.handleEpicTreeLoaded(msg)
	}

	return m, nil
}

// View renders the dashboard UI.
func (m Model) View() string {
	// Get the base dashboard view
	dashboardView := m.renderView()

	// If help modal is showing, render it as an overlay
	if m.showHelp {
		return zone.Scan(m.helpModal.Overlay(dashboardView))
	}

	// If new workflow modal is open, render it as an overlay
	// Note: formmodal already calls zone.Scan() internally, so we don't scan here
	if m.newWorkflowModal != nil {
		return m.newWorkflowModal.Overlay(dashboardView)
	}

	return zone.Scan(dashboardView)
}

// SetSize handles terminal resize events.
func (m Model) SetSize(width, height int) mode.Controller {
	m.width = width
	m.height = height
	if m.newWorkflowModal != nil {
		m.newWorkflowModal = m.newWorkflowModal.SetSize(width, height)
	}
	m.helpModal = m.helpModal.SetSize(width, height)

	// Recalculate tree and details dimensions
	if m.epicTree != nil {
		// Calculate available height for epic section (same logic as renderView)
		footerHeight := 3 // Action hints pane
		contentHeight := max(height-footerHeight, 5)

		// 55%/45% split (table/epic)
		minTableHeight := minWorkflowTableRows + 3 // header/borders
		tableHeight := max(contentHeight*55/100, minTableHeight)
		epicSectionHeight := contentHeight - tableHeight

		if epicSectionHeight >= 5 {
			// Calculate widths accounting for coordinator panel
			epicWidth := width
			if m.showCoordinatorPanel && m.coordinatorPanel != nil {
				epicWidth = width - CoordinatorPanelWidth
			}

			// 40%/60% split for tree/details
			treeWidth := epicWidth * 40 / 100
			detailsWidth := epicWidth - treeWidth

			// Set tree size
			m.epicTree.SetSize(treeWidth-2, epicSectionHeight-2)

			// Set details size if available
			if m.hasEpicDetail {
				m.epicDetails = m.epicDetails.SetSize(detailsWidth-2, epicSectionHeight-2)
			}
		}
	}

	return m
}

// Cleanup releases resources when exiting the dashboard mode.
func (m *Model) Cleanup() {
	// Clean up global subscription
	if m.unsubscribe != nil {
		m.unsubscribe()
	}
	if m.cancel != nil {
		m.cancel()
	}
}

// IsInitialized returns true if the dashboard has been initialized with a control plane.
func (m Model) IsInitialized() bool {
	return m.controlPlane != nil
}

// RefreshWorkflows returns a command to reload the workflow list.
// Used when re-entering dashboard mode to ensure the list is current.
func (m Model) RefreshWorkflows() tea.Cmd {
	return m.loadWorkflows()
}

// === Internal message types ===

// QuitMsg requests returning to kanban mode from the dashboard.
type QuitMsg struct{}

// StartWorkflowFailedMsg is sent when a workflow fails to start.
type StartWorkflowFailedMsg struct {
	WorkflowID controlplane.WorkflowID
	Err        error
}

// workflowsLoadedMsg contains loaded workflow list.
type workflowsLoadedMsg struct {
	workflows []*controlplane.WorkflowInstance
	err       error
}

// === Command generators ===

// eventSubscriptionReadyMsg indicates the event subscription is ready.
type eventSubscriptionReadyMsg struct {
	eventCh     <-chan controlplane.ControlPlaneEvent
	unsubscribe func()
}

// subscribeToEvents returns a command that subscribes to ControlPlane events.
func (m Model) subscribeToEvents() tea.Cmd {
	return func() tea.Msg {
		if m.controlPlane == nil {
			return nil
		}
		eventCh, unsubscribe := m.controlPlane.Subscribe(m.ctx)
		return eventSubscriptionReadyMsg{eventCh: eventCh, unsubscribe: unsubscribe}
	}
}

// loadWorkflows returns a command that loads all workflows from ControlPlane.
func (m Model) loadWorkflows() tea.Cmd {
	return func() tea.Msg {
		if m.controlPlane == nil {
			return workflowsLoadedMsg{workflows: make([]*controlplane.WorkflowInstance, 0)}
		}
		workflows, err := m.controlPlane.List(context.Background(), controlplane.ListQuery{})
		return workflowsLoadedMsg{workflows: workflows, err: err}
	}
}

// listenForEvents returns a command that waits for the next ControlPlane event.
func (m Model) listenForEvents() tea.Cmd {
	if m.eventCh == nil {
		return nil
	}
	return func() tea.Msg {
		event, ok := <-m.eventCh
		if !ok {
			return nil
		}
		return event
	}
}

// === Event handlers ===

// handleKeyMsg handles keyboard input.
func (m Model) handleKeyMsg(msg tea.KeyMsg) (mode.Controller, tea.Cmd) {
	// If help modal is showing, handle help-specific keys
	if m.showHelp {
		switch msg.String() {
		case "?", "esc":
			m.showHelp = false
			return m, nil
		}
		return m, nil
	}

	// If filter is active, delegate to filter
	if m.filter.IsActive() {
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		// Reset selection when filter changes
		m.selectedIndex = 0
		return m, cmd
	}

	// Handle focus cycling keys (work regardless of current focus)
	switch msg.String() {
	case "tab", "ctrl+n": // Cycle focus forward
		m.cycleFocusForward()
		return m, nil

	case "shift+tab", "ctrl+p": // Cycle focus backward
		m.cycleFocusBackward()
		return m, nil
	}

	// Dispatch based on current focus
	switch m.focus {
	case FocusTable:
		return m.handleTableKeys(msg)
	case FocusEpicView:
		return m.handleEpicTreeKeys(msg)
	case FocusCoordinator:
		return m.handleCoordinatorKeys(msg)
	}

	return m, nil
}

// handleTableKeys handles key events when the workflow table is focused.
func (m Model) handleTableKeys(msg tea.KeyMsg) (mode.Controller, tea.Cmd) {
	// Get filtered workflows for navigation bounds
	filteredWorkflows := m.getFilteredWorkflows()
	workflowCount := len(filteredWorkflows)

	// Navigation
	switch msg.String() {
	case "j", "down":
		if workflowCount > 0 && m.selectedIndex < workflowCount-1 {
			cmd := m.handleWorkflowSelectionChange(m.selectedIndex + 1)
			return m, cmd
		}
		return m, nil

	case "k", "up":
		if m.selectedIndex > 0 {
			cmd := m.handleWorkflowSelectionChange(m.selectedIndex - 1)
			return m, cmd
		}
		return m, nil

	case "g": // Go to first workflow
		cmd := m.handleWorkflowSelectionChange(0)
		return m, cmd

	case "G": // Go to last workflow
		if workflowCount > 0 {
			cmd := m.handleWorkflowSelectionChange(workflowCount - 1)
			return m, cmd
		}
		return m, nil
	}

	// Global actions (available from table focus)
	switch msg.String() {
	// Filter
	case "/": // Activate filter
		m.filter = m.filter.Activate()
		return m, m.filter.Init()

	case "esc": // Clear filter, or quit if no filter
		if m.filter.HasFilter() {
			m.filter = m.filter.Clear()
			m.selectedIndex = 0
			return m, nil
		}
		return m, func() tea.Msg { return QuitMsg{} }

	// Help
	case "?": // Toggle help
		m.showHelp = !m.showHelp
		m.helpModal = m.helpModal.SetSize(m.width, m.height)
		return m, nil

	// Quick actions
	case "s": // Start or Resume workflow
		workflow := m.SelectedWorkflow()
		if workflow == nil {
			return m, nil
		}
		if workflow.State == controlplane.WorkflowPaused {
			return m.resumeSelectedWorkflow()
		}
		return m.startSelectedWorkflow()

	case "x": // Pause workflow
		return m.pauseSelectedWorkflow()

	case "n", "N": // New workflow (always starts immediately)
		return m.openNewWorkflowModal()

	case "ctrl+w": // Toggle coordinator chat panel
		return m.toggleCoordinatorPanel()

	case "[": // Previous tab in coordinator panel
		if m.showCoordinatorPanel && m.coordinatorPanel != nil {
			m.coordinatorPanel.PrevTab()
			return m, nil
		}
		return m, nil

	case "]": // Next tab in coordinator panel
		if m.showCoordinatorPanel && m.coordinatorPanel != nil {
			m.coordinatorPanel.NextTab()
			return m, nil
		}
		return m, nil

	case "enter": // Focus coordinator panel for selected workflow
		// Clear notification flag for the selected workflow
		if wf := m.SelectedWorkflow(); wf != nil {
			m.clearNotificationForWorkflow(wf.ID)
		}

		if m.showCoordinatorPanel && m.coordinatorPanel != nil {
			m.focus = FocusCoordinator
			m.updateComponentFocusStates()
			return m, nil
		}
		// If panel not open, open it and focus
		m.openCoordinatorPanelForSelected()
		if m.coordinatorPanel != nil {
			m.focus = FocusCoordinator
			m.updateComponentFocusStates()
		}
		return m, nil

	case "q", "ctrl+c":
		return m, func() tea.Msg { return QuitMsg{} }
	}

	return m, nil
}

// handleEpicTreeKeys handles key events when the epic tree/details section is focused.
// Dispatches to tree pane or details pane handler based on epicViewFocus.
func (m Model) handleEpicTreeKeys(msg tea.KeyMsg) (mode.Controller, tea.Cmd) {
	// Handle global keys that should work from epic view
	switch msg.String() {
	case "?": // Toggle help
		m.showHelp = !m.showHelp
		m.helpModal = m.helpModal.SetSize(m.width, m.height)
		return m, nil

	case "ctrl+w": // Toggle coordinator chat panel
		return m.toggleCoordinatorPanel()

	case "q", "ctrl+c", "esc":
		return m, func() tea.Msg { return QuitMsg{} }
	}

	// Dispatch to pane-specific handler
	switch m.epicViewFocus {
	case EpicFocusTree:
		return m.handleEpicTreeKeysFocusTree(msg)
	case EpicFocusDetails:
		return m.handleEpicTreeKeysFocusDetails(msg)
	}

	return m, nil
}

// handleCoordinatorKeys handles key events when the coordinator panel is focused.
func (m Model) handleCoordinatorKeys(msg tea.KeyMsg) (mode.Controller, tea.Cmd) {
	switch msg.String() {
	case "?": // Toggle help
		m.showHelp = !m.showHelp
		m.helpModal = m.helpModal.SetSize(m.width, m.height)
		return m, nil

	case "ctrl+w": // Toggle coordinator chat panel (closes it)
		m.showCoordinatorPanel = false
		m.coordinatorPanel = nil
		m.focus = FocusTable
		m.updateComponentFocusStates()
		return m, nil

	case "[": // Previous tab in coordinator panel
		if m.coordinatorPanel != nil {
			m.coordinatorPanel.PrevTab()
			return m, nil
		}
		return m, nil

	case "]": // Next tab in coordinator panel
		if m.coordinatorPanel != nil {
			m.coordinatorPanel.NextTab()
			return m, nil
		}
		return m, nil

	case "q", "ctrl+c", "esc":
		return m, func() tea.Msg { return QuitMsg{} }
	}

	return m, nil
}

// handleMouseMsg handles mouse input for zone clicks and scrolling.
func (m Model) handleMouseMsg(msg tea.MouseMsg) (mode.Controller, tea.Cmd) {
	// Only handle left-click release events for zone selection
	if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionRelease {
		// Check workflow row zones
		filtered := m.getFilteredWorkflows()
		for i := range filtered {
			zoneID := makeWorkflowZoneID(i)
			if z := zone.Get(zoneID); z != nil && z.InBounds(msg) {
				m.focus = FocusTable
				m.updateComponentFocusStates()
				cmd := m.handleWorkflowSelectionChange(i)
				// Clear notification flag for the clicked workflow
				m.clearNotificationForWorkflow(filtered[i].ID)
				return m, cmd
			}
		}

		// Check workflow table zone (container click - focuses table)
		if z := zone.Get(zoneWorkflowTable); z != nil && z.InBounds(msg) {
			m.focus = FocusTable
			m.updateComponentFocusStates()
			return m, nil
		}

		// Check epic zone clicks
		// Check epic tree issue clicks first (more specific than tree container)
		if m.epicTree != nil {
			for _, issueID := range m.epicTree.VisibleIssueIDs() {
				zoneID := zoneEpicIssuePrefix + issueID
				if z := zone.Get(zoneID); z != nil && z.InBounds(msg) {
					m.epicTree.SelectByIssueID(issueID)
					m.updateEpicDetail()
					m.focus = FocusEpicView
					m.epicViewFocus = EpicFocusTree
					m.updateComponentFocusStates()
					return m, nil
				}
			}
		}

		// Check epic tree zone (container click - focuses tree pane)
		if z := zone.Get(zoneEpicTree); z != nil && z.InBounds(msg) {
			m.focus = FocusEpicView
			m.epicViewFocus = EpicFocusTree
			m.updateComponentFocusStates()
			return m, nil
		}

		// Check epic details zone
		if z := zone.Get(zoneEpicDetails); z != nil && z.InBounds(msg) {
			m.focus = FocusEpicView
			m.epicViewFocus = EpicFocusDetails
			m.updateComponentFocusStates()
			return m, nil
		}

		// Check tab zones (only if coordinator panel is open)
		if m.showCoordinatorPanel && m.coordinatorPanel != nil {
			tabCount := m.coordinatorPanel.tabCount()
			for i := range tabCount {
				zoneID := makeTabZoneID(i)
				if z := zone.Get(zoneID); z != nil && z.InBounds(msg) {
					m.coordinatorPanel.activeTab = i
					return m, nil
				}
			}

			// Check chat input zone - clicking focuses the input and updates dashboard focus
			if z := zone.Get(zoneChatInput); z != nil && z.InBounds(msg) {
				m.focus = FocusCoordinator
				m.coordinatorPanel.Focus()
				m.updateComponentFocusStates()
				return m, nil
			}
		}
	}

	// Handle scroll events - route to the appropriate zone based on mouse position
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		// Check if scrolling in workflow table zone
		if z := zone.Get(zoneWorkflowTable); z != nil && z.InBounds(msg) {
			scrollAmount := 3 // Scroll 3 rows at a time
			filteredCount := len(m.getFilteredWorkflows())
			if msg.Button == tea.MouseButtonWheelUp {
				m.tableScrollOffset = max(0, m.tableScrollOffset-scrollAmount)
			} else {
				m.tableScrollOffset += scrollAmount
				// Clamp to prevent scrolling past the end
				// Max offset is roughly rows - visible_rows, but we don't know visible_rows
				// So clamp to rows-1 as a safe upper bound (will be further refined in render)
				maxOffset := max(0, filteredCount-1)
				m.tableScrollOffset = min(m.tableScrollOffset, maxOffset)
			}
			return m, nil
		}

		// Check if scrolling in epic details zone
		if z := zone.Get(zoneEpicDetails); z != nil && z.InBounds(msg) {
			if m.hasEpicDetail {
				var cmd tea.Cmd
				m.epicDetails, cmd = m.epicDetails.Update(msg)
				return m, cmd
			}
			return m, nil
		}

		// Forward scroll events to coordinator panel if scrolling in that area
		if m.showCoordinatorPanel && m.coordinatorPanel != nil {
			var cmd tea.Cmd
			m.coordinatorPanel, cmd = m.coordinatorPanel.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

// toggleCoordinatorPanel toggles the coordinator chat panel for the selected workflow.
func (m Model) toggleCoordinatorPanel() (mode.Controller, tea.Cmd) {
	if m.showCoordinatorPanel {
		// Close the panel
		m.showCoordinatorPanel = false
		m.coordinatorPanel = nil
		return m, nil
	}

	m.openCoordinatorPanelForSelected()
	return m, nil
}

// openCoordinatorPanelForSelected opens the coordinator panel for the currently selected workflow.
func (m *Model) openCoordinatorPanelForSelected() {
	wf := m.SelectedWorkflow()
	if wf == nil {
		return
	}

	// Create new panel (pass debugMode for command log tab, vimMode for input)
	panel := NewCoordinatorPanel(m.debugMode, m.vimMode)
	panel.SetSize(CoordinatorPanelWidth, m.height)

	// Load cached state for this workflow (ensures state exists)
	uiState := m.getOrCreateUIState(wf.ID)
	panel.SetWorkflow(wf.ID, uiState)

	m.coordinatorPanel = panel
	m.showCoordinatorPanel = true
}

// getFilteredWorkflows returns workflows after applying the current filter.
func (m Model) getFilteredWorkflows() []*controlplane.WorkflowInstance {
	return m.filter.FilterWorkflows(m.workflows)
}

// handleControlPlaneEvent handles events from the ControlPlane subscription.
// It updates the cached WorkflowUIState for any workflow that sends events,
// regardless of whether that workflow is currently selected.
func (m Model) handleControlPlaneEvent(event controlplane.ControlPlaneEvent) (mode.Controller, tea.Cmd) {
	// Handle EventWorkflowFailed: proactively clean up state for failed workflows
	if event.Type == controlplane.EventWorkflowFailed && event.WorkflowID != "" {
		delete(m.workflowUIState, event.WorkflowID)
	}

	// Refresh workflow list on any lifecycle event
	if event.Type.IsLifecycleEvent() {
		return m, tea.Batch(
			m.loadWorkflows(),
			m.listenForEvents(),
		)
	}

	// Update cached UI state for this workflow (even if not currently selected)
	if event.WorkflowID != "" {
		m.updateCachedUIState(event)

		// If coordinator panel is showing for this workflow, update it with new state
		if m.showCoordinatorPanel && m.coordinatorPanel != nil && m.coordinatorPanel.workflowID == event.WorkflowID {
			// updateCachedUIState already called getOrCreateUIState, so state exists
			uiState := m.getOrCreateUIState(event.WorkflowID)
			m.coordinatorPanel.SetWorkflow(event.WorkflowID, uiState)
		}
	}

	// For other events, just continue listening
	return m, m.listenForEvents()
}

// handleStartWorkflowFailed handles errors when starting a workflow fails.
// It converts worktree-specific errors to user-friendly messages.
func (m Model) handleStartWorkflowFailed(msg StartWorkflowFailedMsg) (mode.Controller, tea.Cmd) {
	errMsg := msg.Err.Error()

	// Check for worktree-specific errors and provide user-friendly messages
	switch {
	case errors.Is(msg.Err, controlplane.ErrUncommittedChanges):
		errMsg = "Worktree has uncommitted changes. Commit or discard changes first."
	case errors.Is(msg.Err, domaingit.ErrBranchAlreadyCheckedOut):
		errMsg = "Branch is already checked out in another worktree."
	case errors.Is(msg.Err, domaingit.ErrPathAlreadyExists):
		errMsg = "Worktree path already exists. Try a different branch name."
	}

	// Return a toast message to show the error
	return m, func() tea.Msg {
		return mode.ShowToastMsg{
			Message: errMsg,
			Style:   toaster.StyleError,
		}
	}
}

// updateCachedUIState updates the cached WorkflowUIState based on the incoming event.
// This ensures state accumulates even when not viewing a workflow's detail panes.
func (m *Model) updateCachedUIState(event controlplane.ControlPlaneEvent) {
	// Get or create UI state for the event's workflow
	uiState := m.getOrCreateUIState(event.WorkflowID)

	// Update the appropriate fields based on event type
	switch event.Type {
	case controlplane.EventCoordinatorSpawned:
		// Coordinator started - set initial status to Ready
		if payload, ok := event.Payload.(events.ProcessEvent); ok {
			uiState.CoordinatorStatus = payload.Status
			uiState.CoordinatorQueueCount = payload.QueueCount
		}

	case controlplane.EventCoordinatorOutput:
		if payload, ok := event.Payload.(events.ProcessEvent); ok {
			// Handle Ready/Working state transitions
			switch payload.Type {
			case events.ProcessReady:
				uiState.CoordinatorStatus = events.ProcessStatusReady
			case events.ProcessWorking:
				uiState.CoordinatorStatus = events.ProcessStatusWorking
			case events.ProcessOutput:
				// Output events - append message to chat
				m.appendCoordinatorMessageToCache(uiState, payload)
			case events.ProcessTokenUsage:
				// Token usage events - update metrics
				if payload.Metrics != nil {
					uiState.CoordinatorMetrics = payload.Metrics
				}
			default:
				// For other event types, use the Status field if present
				if payload.Status != "" {
					uiState.CoordinatorStatus = payload.Status
				}
				// Still append output if present
				if payload.Output != "" {
					m.appendCoordinatorMessageToCache(uiState, payload)
				}
			}
			// Always update queue count
			uiState.CoordinatorQueueCount = payload.QueueCount
		}

	case controlplane.EventCoordinatorIncoming:
		// Message delivered to coordinator - use sender from event
		if payload, ok := event.Payload.(events.ProcessEvent); ok {
			if payload.Message != "" {
				uiState.CoordinatorMessages = append(uiState.CoordinatorMessages, chatrender.Message{
					Role:    payload.Sender,
					Content: payload.Message,
				})
			}
		}

	case controlplane.EventWorkerOutput:
		if payload, ok := event.Payload.(events.ProcessEvent); ok {
			workerID := payload.ProcessID
			// Ensure worker exists in cache
			if !slices.Contains(uiState.WorkerIDs, workerID) {
				m.addWorkerToCache(uiState, workerID)
			}
			// Update worker status based on event type
			switch payload.Type {
			case events.ProcessReady:
				uiState.WorkerStatus[workerID] = events.ProcessStatusReady
			case events.ProcessWorking:
				uiState.WorkerStatus[workerID] = events.ProcessStatusWorking
			case events.ProcessOutput:
				// Output events - append message to chat
				m.appendWorkerMessageToCache(uiState, payload)
			case events.ProcessTokenUsage:
				// Token usage events - update metrics
				if payload.Metrics != nil {
					if uiState.WorkerMetrics == nil {
						uiState.WorkerMetrics = make(map[string]*metrics.TokenMetrics)
					}
					uiState.WorkerMetrics[workerID] = payload.Metrics
				}
			default:
				// For other event types, use the Status field if present
				if payload.Status != "" {
					uiState.WorkerStatus[workerID] = payload.Status
				}
				// Still append output if present
				if payload.Output != "" {
					m.appendWorkerMessageToCache(uiState, payload)
				}
			}
			// Update phase and queue count
			if payload.Phase != nil {
				uiState.WorkerPhases[workerID] = *payload.Phase
			}
			uiState.WorkerQueueCounts[workerID] = payload.QueueCount
		}

	case controlplane.EventWorkerSpawned:
		if payload, ok := event.Payload.(events.ProcessEvent); ok {
			m.addWorkerToCache(uiState, payload.ProcessID)
		}

	case controlplane.EventWorkerRetired:
		if payload, ok := event.Payload.(events.ProcessEvent); ok {
			m.removeWorkerFromCache(uiState, payload.ProcessID)
		}

	case controlplane.EventWorkerIncoming:
		// Message delivered to worker (from coordinator) - add as coordinator message
		if payload, ok := event.Payload.(events.ProcessEvent); ok {
			workerID := payload.ProcessID
			if payload.Message != "" {
				// Ensure worker exists in cache
				if !slices.Contains(uiState.WorkerIDs, workerID) {
					m.addWorkerToCache(uiState, workerID)
				}
				messages := uiState.WorkerMessages[workerID]
				messages = append(messages, chatrender.Message{
					Role:    "coordinator",
					Content: payload.Message,
				})
				uiState.WorkerMessages[workerID] = messages
			}
		}

	case controlplane.EventMessagePosted:
		if payload, ok := event.Payload.(message.Event); ok {
			uiState.MessageEntries = append(uiState.MessageEntries, payload.Entry)
		}

	case controlplane.EventUserNotification:
		// Set notification flag to highlight this workflow row
		uiState.HasNotification = true

	case controlplane.EventCommandLog:
		// Command log events for debug mode display
		if payload, ok := event.Payload.(processor.CommandLogEvent); ok {
			errorStr := ""
			if payload.Error != nil {
				errorStr = payload.Error.Error()
			}
			entry := CommandLogEntry{
				Timestamp:   payload.Timestamp,
				CommandType: payload.CommandType,
				CommandID:   payload.CommandID,
				Source:      payload.Source,
				Success:     payload.Success,
				Error:       errorStr,
				Duration:    payload.Duration,
				TraceID:     payload.TraceID,
			}
			uiState.CommandLogEntries = append(uiState.CommandLogEntries, entry)

			// Apply max entry bounds checking (FIFO eviction)
			if len(uiState.CommandLogEntries) > maxCommandLogEntries {
				uiState.CommandLogEntries = uiState.CommandLogEntries[1:]
			}
		}
	}

	// Update timestamp (handle nil Clock for tests)
	if m.services.Clock != nil {
		uiState.LastUpdated = m.services.Clock.Now()
	}
}

// appendCoordinatorMessageToCache appends a coordinator message to the cached UI state.
func (m *Model) appendCoordinatorMessageToCache(state *WorkflowUIState, payload events.ProcessEvent) {
	// Skip empty output (status change signals without actual content)
	if payload.Output == "" {
		return
	}

	isToolCall := strings.HasPrefix(payload.Output, "🔧")

	// Handle streaming deltas by appending to the last message if same role
	if payload.Delta && len(state.CoordinatorMessages) > 0 {
		lastIdx := len(state.CoordinatorMessages) - 1
		lastMsg := &state.CoordinatorMessages[lastIdx]
		if lastMsg.Role == "assistant" && !lastMsg.IsToolCall {
			lastMsg.Content += payload.Output
			return
		}
	}

	state.CoordinatorMessages = append(state.CoordinatorMessages, chatrender.Message{
		Role:       "assistant",
		Content:    payload.Output,
		IsToolCall: isToolCall,
	})
}

// appendWorkerMessageToCache appends a worker message to the cached UI state.
func (m *Model) appendWorkerMessageToCache(state *WorkflowUIState, payload events.ProcessEvent) {
	// Skip empty output (status change signals without actual content)
	if payload.Output == "" {
		return
	}

	workerID := payload.ProcessID
	isToolCall := strings.HasPrefix(payload.Output, "🔧")
	messages := state.WorkerMessages[workerID]

	// Handle streaming deltas by appending to the last message if same role
	if payload.Delta && len(messages) > 0 {
		lastIdx := len(messages) - 1
		lastMsg := messages[lastIdx]
		if lastMsg.Role == "assistant" && !lastMsg.IsToolCall {
			messages[lastIdx].Content += payload.Output
			state.WorkerMessages[workerID] = messages
			return
		}
	}

	messages = append(messages, chatrender.Message{
		Role:       "assistant",
		Content:    payload.Output,
		IsToolCall: isToolCall,
	})
	state.WorkerMessages[workerID] = messages
}

// addWorkerToCache adds a worker to the cached UI state.
func (m *Model) addWorkerToCache(state *WorkflowUIState, workerID string) {
	// Check if worker already exists
	if slices.Contains(state.WorkerIDs, workerID) {
		return
	}
	state.WorkerIDs = append(state.WorkerIDs, workerID)
	state.WorkerStatus[workerID] = events.ProcessStatusReady
}

// removeWorkerFromCache marks a worker as retired in the cached UI state.
func (m *Model) removeWorkerFromCache(state *WorkflowUIState, workerID string) {
	state.WorkerStatus[workerID] = events.ProcessStatusRetired

	// Remove from worker IDs list
	newIDs := make([]string, 0, len(state.WorkerIDs))
	for _, id := range state.WorkerIDs {
		if id != workerID {
			newIDs = append(newIDs, id)
		}
	}
	state.WorkerIDs = newIDs
}

// === Action handlers ===

// startSelectedWorkflow starts the currently selected workflow.
func (m Model) startSelectedWorkflow() (mode.Controller, tea.Cmd) {
	wf := m.SelectedWorkflow()
	if wf == nil {
		return m, nil
	}
	if wf.State != controlplane.WorkflowPending {
		// Show warning toast for already running/paused workflows
		var msg string
		switch wf.State {
		case controlplane.WorkflowRunning:
			msg = "Workflow is already running"
		case controlplane.WorkflowPaused:
			msg = "Workflow is paused. Press 's' again to resume."
		default:
			msg = "Cannot start workflow in current state"
		}
		return m, func() tea.Msg {
			return mode.ShowToastMsg{Message: msg, Style: toaster.StyleWarn}
		}
	}

	return m, m.startWorkflow(wf.ID)
}

// resumeSelectedWorkflow resumes a paused workflow.
func (m Model) resumeSelectedWorkflow() (mode.Controller, tea.Cmd) {
	workflow := m.SelectedWorkflow()
	if workflow == nil {
		return m, nil
	}
	if workflow.State != controlplane.WorkflowPaused {
		return m, nil // Can only resume paused workflows
	}

	return m, func() tea.Msg {
		if m.controlPlane == nil {
			return nil
		}
		_ = m.controlPlane.Resume(context.Background(), workflow.ID)
		// Workflow state change will be received via event subscription
		return nil
	}
}

// pauseSelectedWorkflow pauses the currently selected workflow.
func (m Model) pauseSelectedWorkflow() (mode.Controller, tea.Cmd) {
	workflow := m.SelectedWorkflow()
	if workflow == nil {
		return m, nil
	}
	if !workflow.IsRunning() {
		// Show warning toast for already paused/pending workflows
		var msg string
		switch workflow.State {
		case controlplane.WorkflowPaused:
			msg = "Workflow is already paused"
		case controlplane.WorkflowPending:
			msg = "Workflow hasn't started yet. Press 's' to start."
		default:
			msg = "Cannot pause workflow in current state"
		}
		return m, func() tea.Msg {
			return mode.ShowToastMsg{Message: msg, Style: toaster.StyleWarn}
		}
	}

	return m, func() tea.Msg {
		if m.controlPlane == nil {
			return nil
		}
		_ = m.controlPlane.Pause(context.Background(), workflow.ID)
		// Workflow state change will be received via event subscription
		return nil
	}
}

// SelectedWorkflow returns the currently selected workflow, or nil if none.
// This uses the filtered workflow list when a filter is active.
func (m Model) SelectedWorkflow() *controlplane.WorkflowInstance {
	filtered := m.getFilteredWorkflows()
	if len(filtered) == 0 || m.selectedIndex >= len(filtered) {
		return nil
	}
	return filtered[m.selectedIndex]
}

// Workflows returns the current list of workflows.
func (m Model) Workflows() []*controlplane.WorkflowInstance {
	return m.workflows
}

// openNewWorkflowModal opens the new workflow creation modal.
func (m Model) openNewWorkflowModal() (mode.Controller, tea.Cmd) {
	// Create a GitExecutor if we have a factory and workDir
	var gitExec appgit.GitExecutor
	if m.gitExecutorFactory != nil && m.workDir != "" {
		gitExec = m.gitExecutorFactory(m.workDir)
	}
	m.newWorkflowModal = NewNewWorkflowModal(
		m.registryService,
		m.controlPlane,
		gitExec,
		m.workflowCreator,
	).SetSize(m.width, m.height)
	return m, m.newWorkflowModal.Init()
}

// startWorkflow starts a workflow by ID.
func (m Model) startWorkflow(id controlplane.WorkflowID) tea.Cmd {
	return func() tea.Msg {
		if m.controlPlane == nil {
			return nil
		}
		err := m.controlPlane.Start(context.Background(), id)
		if err != nil {
			return StartWorkflowFailedMsg{WorkflowID: id, Err: err}
		}
		return nil
	}
}

// InNewWorkflowModal returns true if the new workflow modal is showing.
func (m Model) InNewWorkflowModal() bool {
	return m.newWorkflowModal != nil
}

// NewWorkflowModalRef returns the current new workflow modal, or nil if not showing.
func (m Model) NewWorkflowModalRef() *NewWorkflowModal {
	return m.newWorkflowModal
}

// === UI State Management ===

// loadSelectedWorkflowState loads cached state for the selected workflow.
// This is called when workflows are loaded to ensure the UI state cache is populated.
func (m *Model) loadSelectedWorkflowState() {
	workflow := m.SelectedWorkflow()
	if workflow == nil {
		return
	}

	// Load cached state (or initialize from MessageRepo if empty)
	uiState := m.getOrCreateUIState(workflow.ID)

	// If cached message entries are empty and workflow has a MessageRepo,
	// load history from the repository to populate the cache
	if len(uiState.MessageEntries) == 0 && workflow.MessageRepo != nil {
		entries := workflow.MessageRepo.Entries()
		uiState.MessageEntries = entries
		if m.services.Clock != nil {
			uiState.LastUpdated = m.services.Clock.Now()
		}
	}
}

// handleWorkflowSelectionChange handles workflow selection changes during navigation.
// It updates the selected index, loads cached state for the new selection, and
// triggers epic tree loading if conditions are met.
// Returns a tea.Cmd for any async operations (e.g., debounced epic tree load).
// All workflow events are received via the global subscription and cached automatically.
func (m *Model) handleWorkflowSelectionChange(newIndex int) tea.Cmd {
	// Don't do anything if selection isn't actually changing
	if newIndex == m.selectedIndex {
		return nil
	}

	// Save epic tree state for the current workflow before switching
	if currentWf := m.SelectedWorkflow(); currentWf != nil {
		m.saveEpicTreeState(string(currentWf.ID))
	}

	// Update selection
	m.selectedIndex = newIndex

	// Load cached state for the new selection
	m.loadSelectedWorkflowState()

	// Update coordinator panel if open
	if m.showCoordinatorPanel && m.coordinatorPanel != nil {
		wf := m.SelectedWorkflow()
		if wf != nil {
			// Use getOrCreateUIState to ensure we have valid state (loadSelectedWorkflowState already called above)
			uiState := m.getOrCreateUIState(wf.ID)
			m.coordinatorPanel.SetWorkflow(wf.ID, uiState)
		}
	}

	// Trigger epic tree load if conditions are met (has epicID, different epic)
	return m.triggerEpicTreeLoad()
}

// clearNotificationForWorkflow clears the notification flag for a workflow.
// Called when user interacts with a workflow row (Enter key or mouse click).
func (m *Model) clearNotificationForWorkflow(workflowID controlplane.WorkflowID) {
	if uiState, exists := m.workflowUIState[workflowID]; exists {
		uiState.HasNotification = false
	}
}

// getOrCreateUIState returns the cached UI state for a workflow, creating if needed.
func (m *Model) getOrCreateUIState(workflowID controlplane.WorkflowID) *WorkflowUIState {
	if state, exists := m.workflowUIState[workflowID]; exists {
		return state
	}
	state := NewWorkflowUIState()
	m.workflowUIState[workflowID] = state
	// Evict oldest entries if we exceed the cache limit
	m.evictOldestUIState()
	return state
}

// evictOldestUIState removes the oldest non-running, non-selected workflow from the cache
// when the cache exceeds maxCachedWorkflows.
func (m *Model) evictOldestUIState() {
	if len(m.workflowUIState) <= maxCachedWorkflows {
		return
	}

	selected := m.SelectedWorkflow()
	var oldestID controlplane.WorkflowID
	var oldestTime *WorkflowUIState

	for id, state := range m.workflowUIState {
		// Don't evict running workflows or currently selected
		if m.isWorkflowRunning(id) {
			continue
		}
		if selected != nil && selected.ID == id {
			continue
		}
		if oldestTime == nil || state.LastUpdated.Before(oldestTime.LastUpdated) {
			oldestID = id
			oldestTime = state
		}
	}

	if oldestID != "" {
		delete(m.workflowUIState, oldestID)
	}
}

// isWorkflowRunning returns true if the workflow with the given ID is currently running.
func (m *Model) isWorkflowRunning(id controlplane.WorkflowID) bool {
	for _, wf := range m.workflows {
		if wf.ID == id {
			return wf.IsRunning()
		}
	}
	return false
}

// === DB Change Handling ===

// HandleDBChanged processes database change notifications from the app.
// This is called by app.go when the centralized watcher detects changes.
// It triggers a tree refresh if an epic is loaded.
func (m Model) HandleDBChanged() (Model, tea.Cmd) {
	// Skip if no epic is loaded
	if m.lastLoadedEpicID == "" {
		return m, nil
	}

	// Trigger a tree refresh by loading the epic tree again
	return m, loadEpicTree(m.lastLoadedEpicID, m.services.Executor)
}

// === Focus Management ===

// cycleFocusForward cycles focus to the next zone and updates component focus states.
// Order: Table → EpicTree → EpicDetails → Coordinator → Table
func (m *Model) cycleFocusForward() {
	switch m.focus {
	case FocusTable:
		m.focus = FocusEpicView
		m.epicViewFocus = EpicFocusTree
	case FocusEpicView:
		if m.epicViewFocus == EpicFocusTree {
			// Tree → Details
			m.epicViewFocus = EpicFocusDetails
		} else {
			// Details → Coordinator (or Table if no coordinator)
			if m.showCoordinatorPanel && m.coordinatorPanel != nil {
				m.focus = FocusCoordinator
			} else {
				m.focus = FocusTable
			}
		}
	case FocusCoordinator:
		m.focus = FocusTable
	}
	m.updateComponentFocusStates()
}

// cycleFocusBackward cycles focus to the previous zone and updates component focus states.
// Order: Table ← EpicTree ← EpicDetails ← Coordinator ← Table
func (m *Model) cycleFocusBackward() {
	switch m.focus {
	case FocusTable:
		// Table → Coordinator (or Details if no coordinator)
		if m.showCoordinatorPanel && m.coordinatorPanel != nil {
			m.focus = FocusCoordinator
		} else {
			m.focus = FocusEpicView
			m.epicViewFocus = EpicFocusDetails
		}
	case FocusEpicView:
		if m.epicViewFocus == EpicFocusDetails {
			// Details → Tree
			m.epicViewFocus = EpicFocusTree
		} else {
			// Tree → Table
			m.focus = FocusTable
		}
	case FocusCoordinator:
		m.focus = FocusEpicView
		m.epicViewFocus = EpicFocusDetails
	}
	m.updateComponentFocusStates()
}

// updateComponentFocusStates updates the focus state of sub-components based on m.focus.
// This ensures the coordinator panel and table config cache reflect the current focus correctly.
func (m *Model) updateComponentFocusStates() {
	if m.coordinatorPanel != nil {
		if m.focus == FocusCoordinator {
			m.coordinatorPanel.Focus()
		} else {
			m.coordinatorPanel.Blur()
		}
	}

	// Update table config cache when focus changes (affects border styling)
	currentFocus := m.focus == FocusTable
	if m.lastTableFocus != currentFocus {
		m.tableConfigCache = m.createWorkflowTableConfig()
		m.lastTableFocus = currentFocus
	}
}
