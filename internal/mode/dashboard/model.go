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

	appgit "github.com/zjrosen/perles/internal/git/application"
	domaingit "github.com/zjrosen/perles/internal/git/domain"
	"github.com/zjrosen/perles/internal/mode"
	"github.com/zjrosen/perles/internal/orchestration/controlplane"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/message"
	appreg "github.com/zjrosen/perles/internal/registry/application"
	"github.com/zjrosen/perles/internal/ui/modals/help"
	"github.com/zjrosen/perles/internal/ui/shared/chatrender"
	"github.com/zjrosen/perles/internal/ui/shared/table"
	"github.com/zjrosen/perles/internal/ui/shared/toaster"
)

// heartbeatRefreshInterval is how often to refresh the view for heartbeat display updates.
const heartbeatRefreshInterval = 5 * time.Second

// heartbeatTickMsg triggers a view refresh for heartbeat display.
type heartbeatTickMsg struct{}

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
	workflows       []*controlplane.WorkflowInstance
	selectedIndex   int
	workflowTable   table.Model     // Shared table component for workflow list
	workflowList    WorkflowList    // Component for sorting/filtering state
	resourceSummary ResourceSummary // Component for resource bar

	// New workflow modal state (nil when not showing modal)
	newWorkflowModal *NewWorkflowModal

	// Help modal state
	showHelp  bool
	helpModal help.Model

	// Filter state
	filter FilterState

	// Per-workflow UI state cache (kept for future detail view)
	workflowUIState map[controlplane.WorkflowID]*WorkflowUIState

	// Event subscription (global - all workflows)
	eventCh     <-chan controlplane.ControlPlaneEvent
	unsubscribe func()
	ctx         context.Context
	cancel      context.CancelFunc

	// Git worktree support
	gitExecutorFactory func(path string) appgit.GitExecutor
	workDir            string

	// Dimensions
	width  int
	height int
}

// WorkflowTableRow wraps a workflow with its display index for table rendering.
type WorkflowTableRow struct {
	Index    int                            // 1-based row number
	Workflow *controlplane.WorkflowInstance // The workflow data
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
		ctx:                ctx,
		cancel:             cancel,
		gitExecutorFactory: cfg.GitExecutorFactory,
		workDir:            cfg.WorkDir,
	}

	// Initialize the workflow table with config
	// Note: The table config is recreated on each render to capture current model state
	// in render callback closures, but we initialize it here for the initial state.
	m.workflowTable = table.New(m.createWorkflowTableConfig())

	return m
}

// Init returns initial commands for the mode.
// It subscribes to ControlPlane events and loads the initial workflow list.
func (m Model) Init() tea.Cmd {
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

		default:
			var cmd tea.Cmd
			m.newWorkflowModal, cmd = m.newWorkflowModal.Update(msg)
			return m, cmd
		}
	}

	// Dashboard view handling
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case workflowsLoadedMsg:
		m.workflows = msg.workflows
		m.workflowList = m.workflowList.SetWorkflows(m.workflows)
		m.resourceSummary = m.resourceSummary.Update(m.workflows)
		// Load cached state for initial selection if needed
		if len(m.workflows) > 0 {
			m.loadSelectedWorkflowState()
		}
		return m, nil

	case eventSubscriptionReadyMsg:
		m.eventCh = msg.eventCh
		m.unsubscribe = msg.unsubscribe
		return m, m.listenForEvents()

	case controlplane.ControlPlaneEvent:
		return m.handleControlPlaneEvent(msg)

	case StartWorkflowFailedMsg:
		return m.handleStartWorkflowFailed(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	}

	return m, nil
}

// View renders the dashboard UI.
func (m Model) View() string {
	// Get the base dashboard view
	dashboardView := m.renderView()

	// If help modal is showing, render it as an overlay
	if m.showHelp {
		return m.helpModal.Overlay(dashboardView)
	}

	// If new workflow modal is open, render it as an overlay
	if m.newWorkflowModal != nil {
		return m.newWorkflowModal.Overlay(dashboardView)
	}

	return dashboardView
}

// SetSize handles terminal resize events.
func (m Model) SetSize(width, height int) mode.Controller {
	m.width = width
	m.height = height
	if m.newWorkflowModal != nil {
		m.newWorkflowModal = m.newWorkflowModal.SetSize(width, height)
	}
	m.helpModal = m.helpModal.SetSize(width, height)
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

	// Get filtered workflows for navigation bounds
	filteredWorkflows := m.getFilteredWorkflows()
	workflowCount := len(filteredWorkflows)

	// Navigation
	switch msg.String() {
	case "j", "down":
		if workflowCount > 0 {
			newIndex := (m.selectedIndex + 1) % workflowCount
			m.handleWorkflowSelectionChange(newIndex)
		}
		return m, nil

	case "k", "up":
		if m.selectedIndex > 0 {
			m.handleWorkflowSelectionChange(m.selectedIndex - 1)
		}
		return m, nil

	case "g": // Go to first workflow
		m.handleWorkflowSelectionChange(0)
		return m, nil

	case "G": // Go to last workflow
		if workflowCount > 0 {
			m.handleWorkflowSelectionChange(workflowCount - 1)
		}
		return m, nil
	}

	switch msg.String() {
	// Filter
	case "/": // Activate filter
		m.filter = m.filter.Activate()
		return m, m.filter.Init()

	case "esc": // Clear filter (when not in filter input mode)
		if m.filter.HasFilter() {
			m.filter = m.filter.Clear()
			m.selectedIndex = 0
			return m, nil
		}
		return m, nil

	// Help
	case "?": // Toggle help
		m.showHelp = !m.showHelp
		m.helpModal = m.helpModal.SetSize(m.width, m.height)
		return m, nil

	// Quick actions
	case "s": // Start workflow
		return m.startSelectedWorkflow()

	case "x": // Stop workflow
		return m.stopSelectedWorkflow()

	case "n", "N": // New workflow (always starts immediately)
		return m.openNewWorkflowModal()

	case "q", "ctrl+c":
		return m, func() tea.Msg { return QuitMsg{} }
	}

	return m, nil
}

// getFilteredWorkflows returns workflows after applying the current filter.
func (m Model) getFilteredWorkflows() []*controlplane.WorkflowInstance {
	return m.filter.FilterWorkflows(m.workflows)
}

// handleControlPlaneEvent handles events from the ControlPlane subscription.
// It updates the cached WorkflowUIState for any workflow that sends events,
// regardless of whether that workflow is currently selected.
func (m Model) handleControlPlaneEvent(event controlplane.ControlPlaneEvent) (mode.Controller, tea.Cmd) {
	// Handle EventWorkflowStopped: proactively clean up state for stopped workflows
	if event.Type == controlplane.EventWorkflowStopped && event.WorkflowID != "" {
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
	case controlplane.EventCoordinatorOutput:
		if payload, ok := event.Payload.(events.ProcessEvent); ok {
			m.appendCoordinatorMessageToCache(uiState, payload)
		}

	case controlplane.EventWorkerOutput:
		if payload, ok := event.Payload.(events.ProcessEvent); ok {
			m.appendWorkerMessageToCache(uiState, payload)
		}

	case controlplane.EventWorkerSpawned:
		if payload, ok := event.Payload.(events.ProcessEvent); ok {
			m.addWorkerToCache(uiState, payload.ProcessID)
		}

	case controlplane.EventWorkerRetired:
		if payload, ok := event.Payload.(events.ProcessEvent); ok {
			m.removeWorkerFromCache(uiState, payload.ProcessID)
		}

	case controlplane.EventMessagePosted:
		if payload, ok := event.Payload.(message.Event); ok {
			uiState.MessageEntries = append(uiState.MessageEntries, payload.Entry)
		}
	}

	// Update timestamp (handle nil Clock for tests)
	if m.services.Clock != nil {
		uiState.LastUpdated = m.services.Clock.Now()
	}
}

// appendCoordinatorMessageToCache appends a coordinator message to the cached UI state.
func (m *Model) appendCoordinatorMessageToCache(state *WorkflowUIState, payload events.ProcessEvent) {
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
	if wf.State != controlplane.WorkflowPending && wf.State != controlplane.WorkflowPaused {
		return m, nil // Can only start pending or paused workflows
	}

	return m, m.startWorkflow(wf.ID)
}

// stopSelectedWorkflow stops the currently selected workflow.
func (m Model) stopSelectedWorkflow() (mode.Controller, tea.Cmd) {
	workflow := m.SelectedWorkflow()
	if workflow == nil {
		return m, nil
	}
	if workflow.IsTerminal() {
		return m, nil // Can't stop terminal workflows
	}

	return m, func() tea.Msg {
		if m.controlPlane == nil {
			return nil
		}
		_ = m.controlPlane.Stop(context.Background(), workflow.ID, controlplane.StopOptions{
			Reason: "stopped from dashboard",
		})
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
// It updates the selected index and loads cached state for the new selection.
// All workflow events are received via the global subscription and cached automatically.
func (m *Model) handleWorkflowSelectionChange(newIndex int) {
	// Don't do anything if selection isn't actually changing
	if newIndex == m.selectedIndex {
		return
	}

	// Update selection
	m.selectedIndex = newIndex

	// Load cached state for the new selection
	m.loadSelectedWorkflowState()
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
