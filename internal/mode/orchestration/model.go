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

	"github.com/zjrosen/perles/internal/config"
	"github.com/zjrosen/perles/internal/flags"
	appgit "github.com/zjrosen/perles/internal/git/application"
	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/mode"
	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/mcp"
	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/orchestration/metrics"
	"github.com/zjrosen/perles/internal/orchestration/session"
	v2 "github.com/zjrosen/perles/internal/orchestration/v2"
	"github.com/zjrosen/perles/internal/orchestration/v2/process"
	"github.com/zjrosen/perles/internal/orchestration/v2/prompt/roles"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
	"github.com/zjrosen/perles/internal/orchestration/workflow"
	"github.com/zjrosen/perles/internal/pubsub"
	"github.com/zjrosen/perles/internal/ui/commandpalette"
	"github.com/zjrosen/perles/internal/ui/shared/chatrender"
	"github.com/zjrosen/perles/internal/ui/shared/formmodal"
	"github.com/zjrosen/perles/internal/ui/shared/modal"
	"github.com/zjrosen/perles/internal/ui/shared/quitmodal"
	"github.com/zjrosen/perles/internal/ui/shared/toaster"
	"github.com/zjrosen/perles/internal/ui/shared/vimtextarea"
	"github.com/zjrosen/perles/internal/ui/styles"
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
	PaneCommand     = 4
)

// ChatMessage is an alias for the shared chatrender.Message type.
// Used for coordinator and worker chat history.
type ChatMessage = chatrender.Message

// activeWorkflowHolder holds a reference to the active workflow.
// This is a pointer type so it survives Bubble Tea's value-based Model copies.
// The adapter's WorkflowConfigProvider can read from this shared reference.
type activeWorkflowHolder struct {
	workflow *workflow.Workflow
}

// InitPhase represents the current initialization phase.
type InitPhase int

const (
	InitNotStarted           InitPhase = iota
	InitCreatingWorktree               // Create git worktree before workspace
	InitCreatingWorkspace              // Consolidates client, pool, message log, MCP server creation
	InitSpawningCoordinator            // Coordinator process started
	InitAwaitingFirstMessage           // Waiting for coordinator's first response
	InitReady
	InitFailed
	InitTimedOut
)

// InitTimeoutMsg signals initialization timeout (used by tests).
type InitTimeoutMsg struct{}

// SpinnerTickMsg advances the spinner frame.
type SpinnerTickMsg struct{}

// ProcessEventMsg wraps a ProcessEvent for the Bubble Tea message loop.
// This is the unified event type for both coordinator and worker processes.
type ProcessEventMsg struct {
	Event events.ProcessEvent
}

// Model holds the orchestration mode state.
type Model struct {
	// Pane components
	coordinatorPane CoordinatorPane
	messagePane     MessagePane
	workerPane      WorkerPane
	commandPane     CommandPane // Command log pane (toggleable)

	// Command pane visibility toggle (hidden by default)
	showCommandPane bool

	// User input
	input   vimtextarea.Model
	vimMode vimtextarea.Mode // Track current vim mode for display

	// Quit confirmation modal
	quitModal quitmodal.Model

	// Uncommitted changes warning modal (shown when exiting with dirty worktree)
	uncommittedModal quitmodal.Model

	// Workflow state
	paused bool

	// Initialization state machine
	initializer  *Initializer
	initListener *pubsub.ContinuousListener[InitializerEvent]

	// Spinner animation frame (view-only, advanced by SpinnerTickMsg)
	spinnerFrame int

	// Backend integration (v2 infrastructure owns process lifecycle)
	v2Infra            *v2.Infrastructure           // V2 orchestration infrastructure (for shutdown and repo access)
	coordinatorStatus  events.ProcessStatus         // Current coordinator status from v2 events
	messageRepo        repository.MessageRepository // Message repository for inter-agent messaging
	mcpServer          *http.Server                 // HTTP MCP server for in-process tool handling
	mcpPort            int                          // Dynamic port for MCP server
	mcpCoordServer     *mcp.CoordinatorServer       // MCP coordinator server for direct worker messaging
	workDir            string
	worktreeEnabled    bool // Whether worktree isolation is enabled
	disableWorktrees   bool // Skip worktree prompt and always run in current directory
	services           mode.Services
	coordinatorMetrics *metrics.TokenMetrics     // Token usage and cost data for coordinator
	coordinatorWorking bool                      // True when coordinator is processing, false when waiting for input
	session            *session.Session          // Session tracking for this orchestration run
	resumedSession     *session.ResumableSession // Loaded session data for restoration (set during resume flow)
	resumeSessionDir   string                    // Path to session directory to resume (from Config)
	isResumedSession   bool                      // True if this is a resumed session (set after RestoreFromSession)
	resumedAt          time.Time                 // When the session was resumed
	originalStartTime  time.Time                 // Original session start time (from loaded session)

	// AI client configuration
	agentProviders client.AgentProviders // Maps roles to their AI client providers

	// Pub/sub subscriptions (initialized when coordinator starts)
	// Note: Worker events flow through v2EventBus, not a separate workerListener
	// Note: Coordinator events also flow through v2EventBus via ProcessEvent
	messageListener *pubsub.ContinuousListener[message.Event] // Message events listener
	v2Listener      *pubsub.ContinuousListener[any]           // V2 orchestration events listener (includes all process events)
	ctx             context.Context                           // Context for subscription lifetime
	cancel          context.CancelFunc                        // Cancel function for subscriptions

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
	activeWorkflowRef  *activeWorkflowHolder // Shared reference to active workflow (survives Model copies)

	// Coordinator refresh handoff state
	pendingRefresh bool // True when waiting for handoff before refresh

	// Worktree modal state
	worktreeModal           *modal.Model                         // Worktree prompt modal ("Use Git Worktree?")
	branchSelectModal       *formmodal.Model                     // Branch selection modal (when not on main)
	worktreeDecisionMade    bool                                 // True after user has made worktree decision
	gitExecutor             appgit.GitExecutor                   // Git executor for worktree operations
	worktreeBaseBranch      string                               // Branch to base worktree on (set by branch modal)
	worktreeCustomBranch    string                               // Optional custom branch name from user input (set by branch modal)
	worktreeBranch          string                               // Auto-generated branch name (set after worktree creation)
	worktreePath            string                               // Path to the worktree directory (set after creation)
	worktreeExecutorFactory func(path string) appgit.GitExecutor // Factory for creating worktree-scoped executor (injected via Config)

	// Exit state
	exitMessage string // Message to display when exiting (e.g., worktree cleanup info)

	// Debug mode flag (enables full session ID display in input bar)
	debugMode      bool
	tracingConfig  config.TracingConfig  // Tracing configuration (passed to Initializer)
	timeoutsConfig config.TimeoutsConfig // Timeouts configuration (passed to Initializer)

	// Session storage configuration
	sessionStorageConfig config.SessionStorageConfig // Session storage configuration (passed to Initializer)

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
	queueCount    int  // Number of messages queued for coordinator (for UI display)
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
	workerIDs         []string // Worker IDs in display order (active workers only)
	workerStatus      map[string]events.ProcessStatus
	workerTaskIDs     map[string]string                // Current task ID per worker
	workerPhases      map[string]events.ProcessPhase   // Current workflow phase per worker
	workerMessages    map[string][]ChatMessage         // Structured messages per worker (like coordinator)
	workerMetrics     map[string]*metrics.TokenMetrics // Token usage and cost per worker
	workerQueueCounts map[string]int                   // Queue count per worker (for UI display)
	viewports         map[string]viewport.Model        // Viewport per worker for scrolling
	contentDirty      map[string]bool                  // Dirty flag per worker
	hasNewContent     map[string]bool                  // True when new content arrived while scrolled up (per worker)
	retiredOrder      []string                         // Order in which workers retired (oldest first)
}

// Config holds configuration for creating an orchestration Model.
type Config struct {
	Services       mode.Services
	WorkDir        string
	AgentProviders client.AgentProviders // Maps roles to their AI client providers
	// Workflow templates
	WorkflowRegistry *workflow.Registry // Pre-loaded workflow registry (optional)
	// UI settings
	VimMode   bool // Enable vim keybindings in text input areas
	DebugMode bool // Show command pane by default when true
	// Worktree settings
	DisableWorktrees bool // Skip worktree prompt and always run in current directory
	// Tracing settings
	TracingConfig config.TracingConfig // Distributed tracing configuration
	// Session storage settings
	SessionStorageConfig config.SessionStorageConfig // Centralized session storage configuration
	// Session resumption settings
	ResumeSessionDir string // Path to session directory to resume (empty = new session)
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
	// Wire up clipboard for yank operations
	if cfg.Services.Clipboard != nil {
		ta = ta.SetClipboard(cfg.Services.Clipboard)
	}
	ta.Focus() // Focus input by default

	// Get timeouts config from services, falling back to defaults if not available
	var timeoutsConfig config.TimeoutsConfig
	if cfg.Services.Config != nil {
		timeoutsConfig = cfg.Services.Config.Orchestration.Timeouts
	}

	return Model{
		input:                 ta,
		vimMode:               defaultMode, // Initialize mode tracking
		coordinatorPane:       newCoordinatorPane(),
		messagePane:           newMessagePane(),
		workerPane:            newWorkerPane(),
		commandPane:           newCommandPane(),
		showCommandPane:       cfg.DebugMode, // Command pane visible by default only in debug mode
		debugMode:             cfg.DebugMode, // Store for trace ID display format
		services:              cfg.Services,
		workDir:               cfg.WorkDir,
		disableWorktrees:      cfg.DisableWorktrees,
		messageTarget:         "COORDINATOR", // Default to coordinator
		fullscreenWorkerIndex: -1,            // No fullscreen by default
		agentProviders:        cfg.AgentProviders,
		workflowRegistry:      cfg.WorkflowRegistry,
		activeWorkflowRef:     &activeWorkflowHolder{},
		tracingConfig:         cfg.TracingConfig,
		timeoutsConfig:        timeoutsConfig,
		sessionStorageConfig:  cfg.SessionStorageConfig,
		resumeSessionDir:      cfg.ResumeSessionDir,
		quitModal: quitmodal.New(quitmodal.Config{
			Title:   "Exit Orchestration Mode?",
			Message: "Active workers will be stopped.",
		}),
		uncommittedModal: quitmodal.New(quitmodal.Config{
			Title:   "Uncommitted Changes Detected",
			Message: "You have uncommitted changes in the worktree.\n\nThese changes will be LOST if you exit.\n\nCancel to return and commit your changes.",
		}),
		// Get git executor factory from services (injected from app layer)
		worktreeExecutorFactory: cfg.Services.GitExecutorFactory,
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
		workerIDs:         make([]string, 0),
		workerStatus:      make(map[string]events.ProcessStatus),
		workerTaskIDs:     make(map[string]string),
		workerPhases:      make(map[string]events.ProcessPhase),
		workerMessages:    make(map[string][]ChatMessage),
		workerMetrics:     make(map[string]*metrics.TokenMetrics),
		workerQueueCounts: make(map[string]int),
		viewports:         make(map[string]viewport.Model),
		contentDirty:      make(map[string]bool),
		hasNewContent:     make(map[string]bool),
	}
}

// Init returns initial commands for the mode.
// If resumeSessionDir is set, returns ResumeSessionMsg to trigger session resumption.
// Otherwise returns StartCoordinatorMsg to start a new session.
func (m Model) Init() tea.Cmd {
	if m.resumeSessionDir != "" {
		return func() tea.Msg {
			return ResumeSessionMsg{SessionDir: m.resumeSessionDir}
		}
	}
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

	// Mark all panes as dirty for re-render on resize.
	// Viewport dimensions and proportional scroll preservation are handled
	// by ScrollablePane during render (single source of truth).
	m.coordinatorPane.contentDirty = true
	m.messagePane.contentDirty = true
	m.commandPane.contentDirty = true
	for workerID := range m.workerPane.viewports {
		m.workerPane.contentDirty[workerID] = true
	}

	// Update quit modal size (always update to cache dimensions)
	m.quitModal.SetSize(width, height)

	// Update uncommitted modal size (always update to cache dimensions)
	m.uncommittedModal.SetSize(width, height)

	// Update workflow picker size if present
	if m.workflowPicker != nil {
		picker := m.workflowPicker.SetSize(width, height)
		m.workflowPicker = &picker
	}

	return m
}

// AddChatMessage appends or accumulates a message to the coordinator chat history.
// When delta is true, the content is appended to the last message if it has the same role.
// This supports streaming output where multiple chunks form a single logical message.
func (m Model) AddChatMessage(role, content string, delta bool) Model {
	// Detect tool calls by the 🔧 prefix
	isToolCall := strings.HasPrefix(content, "🔧")

	// If delta mode and last message has the same role (and is not a tool call), accumulate
	if delta && len(m.coordinatorPane.messages) > 0 {
		lastIdx := len(m.coordinatorPane.messages) - 1
		lastMsg := &m.coordinatorPane.messages[lastIdx]
		if lastMsg.Role == role && !lastMsg.IsToolCall {
			lastMsg.Content += content
			m.coordinatorPane.contentDirty = true
			return m
		}
	}

	// Otherwise, add as new message
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
// If the status is ProcessStatusRetired or ProcessStatusFailed, the worker is removed from
// the active display list but its viewport data is retained for cleanup based on retirement order.
func (m Model) UpdateWorker(workerID string, status events.ProcessStatus) Model {
	if status == events.ProcessStatusRetired || status == events.ProcessStatusFailed {
		// Check if this worker is currently fullscreen and exit fullscreen if so
		if m.fullscreenPaneType == PaneWorker && m.fullscreenWorkerIndex >= 0 {
			// Build active workers list to find the retiring worker's index
			var activeWorkerIDs []string
			for _, wID := range m.workerPane.workerIDs {
				wStatus := m.workerPane.workerStatus[wID]
				if wStatus != events.ProcessStatusRetired {
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

// AddWorkerMessage appends or accumulates a message to a worker's chat history.
// When delta is true, the content is appended to the last message if it has the same role.
func (m Model) AddWorkerMessage(workerID, role, content string, delta bool) Model {
	// Detect tool calls by prefix
	isToolCall := strings.HasPrefix(content, "🔧")

	messages := m.workerPane.workerMessages[workerID]

	// If delta mode and last message has the same role (and is not a tool call), accumulate
	if delta && len(messages) > 0 {
		lastIdx := len(messages) - 1
		lastMsg := messages[lastIdx]
		if lastMsg.Role == role && !lastMsg.IsToolCall {
			messages[lastIdx].Content += content
			m.workerPane.workerMessages[workerID] = messages
			m.workerPane.contentDirty[workerID] = true
			return m
		}
	}

	// Otherwise, add as new message
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

// SetQueueCount updates the queue count for a worker.
// Used by the UI to display pending queued messages.
func (m Model) SetQueueCount(workerID string, count int) Model {
	m.workerPane.workerQueueCounts[workerID] = count
	return m
}

// SetWorkerTask updates the task ID and phase for a worker.
// Used by events and tests to set worker context for display.
func (m Model) SetWorkerTask(workerID, taskID string, phase events.ProcessPhase) Model {
	m.workerPane.workerTaskIDs[workerID] = taskID
	m.workerPane.workerPhases[workerID] = phase
	return m
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
		if m.workerPane.workerStatus[workerID] != events.ProcessStatusRetired {
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

// SetError returns a command that displays an error toast notification.
func (m Model) SetError(msg string) (Model, tea.Cmd) {
	return m, func() tea.Msg {
		return mode.ShowToastMsg{
			Message: msg,
			Style:   toaster.StyleError,
		}
	}
}

// processRepo returns the process repository from v2Infra, or nil if not initialized.
func (m Model) processRepo() repository.ProcessRepository {
	if m.v2Infra == nil {
		return nil
	}
	return m.v2Infra.Repositories.ProcessRepo
}

// cmdSubmitter returns the command submitter from v2Infra, or nil if not initialized.
func (m Model) cmdSubmitter() process.CommandSubmitter {
	if m.v2Infra == nil {
		return nil
	}
	return m.v2Infra.Core.CmdSubmitter
}

// Coordinator returns nil - coordinator state is now managed via ProcessRepository.
// This method is retained for API compatibility but callers should use
// ProcessRepository.GetCoordinator() for status queries and CmdRetireProcess for cleanup.
// Deprecated: Use v2Infra.Repositories.ProcessRepo.GetCoordinator() for status or CmdRetireProcess for cleanup.
func (m Model) Coordinator() *repository.Process {
	repo := m.processRepo()
	if repo == nil {
		return nil
	}
	proc, err := repo.GetCoordinator()
	if err != nil {
		return nil
	}
	return proc
}

// MCPServer returns the HTTP MCP server instance, if any.
func (m Model) MCPServer() *http.Server {
	return m.mcpServer
}

// SetV2Infra sets the v2 infrastructure for testing purposes.
func (m Model) SetV2Infra(infra *v2.Infrastructure) Model {
	m.v2Infra = infra
	return m
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
}

// Cleanup cleans up all orchestration resources.
// This is called when exiting orchestration mode or retrying after a failed initialization.
// It stops all processes, shuts down the MCP server, and clears state.
func (m *Model) Cleanup() {
	// Cancel any active subscriptions
	m.CancelSubscriptions()

	// Cancel initializer if running (stop background goroutine)
	if m.initializer != nil {
		m.initializer.Cancel()
	}

	// Close session with appropriate status (must happen before v2Infra.Shutdown
	// so we can still determine status from process state)
	if m.session != nil {
		status := m.determineSessionStatus()
		if err := m.session.Close(status); err != nil {
			log.Debug(log.CatOrch, "Session close error", "subsystem", "cleanup", "error", err)
		} else {
			log.Debug(log.CatOrch, "Session closed", "subsystem", "cleanup", "status", status)
		}
		m.session = nil
	}

	// Shutdown v2 infrastructure (stops all processes and drains command processor)
	if m.v2Infra != nil {
		m.v2Infra.Shutdown()
		m.v2Infra = nil
	}

	// Shutdown MCP server if running
	if m.mcpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = m.mcpServer.Shutdown(ctx)
		cancel()
		m.mcpServer = nil
	}

	// Clear message repository
	m.messageRepo = nil

	// Get worktree path - check initializer first (for early exit during init),
	// then fall back to model's worktreePath (for normal exit after init complete)
	worktreePath := m.worktreePath
	worktreeBranch := m.worktreeBranch
	if worktreePath == "" && m.initializer != nil {
		worktreePath = m.initializer.WorktreePath()
		worktreeBranch = m.initializer.WorktreeBranch()
	}

	// Cleanup worktree if created (removes directory, preserves branch)
	if worktreePath != "" && m.gitExecutor != nil {
		// Set exit message with branch info before cleanup
		if worktreeBranch != "" {
			m.exitMessage = fmt.Sprintf(
				"Worktree cleaned up. Your work is preserved on branch '%s'.\nTo resume: git checkout %s",
				worktreeBranch, worktreeBranch,
			)
		}

		if m.services.Flags.Enabled(flags.FlagRemoveWorktree) {
			if err := m.gitExecutor.RemoveWorktree(worktreePath); err != nil {
				log.Warn(log.CatOrch, "Failed to remove worktree", "path", worktreePath, "error", err)
				// Update exit message to indicate failure
				if worktreeBranch != "" {
					m.exitMessage = fmt.Sprintf(
						"Warning: Failed to remove worktree at %s.\nYour work is preserved on branch '%s'.\nTo resume: git checkout %s\nTo manually clean up: git worktree remove %s",
						worktreePath, worktreeBranch, worktreeBranch, worktreePath,
					)
				}
			} else {
				log.Info(log.CatOrch, "Worktree removed", "path", worktreePath, "branch", worktreeBranch)
			}
		}
	}

	// Reset listeners
	m.messageListener = nil
	m.v2Listener = nil
	m.ctx = nil
	m.cancel = nil
}

// ExitMessage returns the exit message to display to the user after cleanup.
// Returns empty string if there's no message to display.
func (m *Model) ExitMessage() string {
	return m.exitMessage
}

// openWorkflowPicker creates and shows the workflow picker modal.
// If no workflow registry is available, this is a no-op.
func (m Model) openWorkflowPicker() Model {
	if m.workflowRegistry == nil {
		return m
	}

	// Build items from workflow registry - only show orchestration-targeted workflows
	workflows := m.workflowRegistry.ListByTargetMode(workflow.TargetOrchestration)
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
		return m.SetError("Workflow registry not available")
	}

	wf, ok := m.workflowRegistry.Get(workflowID)
	if !ok {
		return m.SetError("Workflow not found: " + workflowID)
	}

	// Track the active workflow for prompt customization
	// Use a shared holder so it survives Bubble Tea's value-based Model copies
	m.activeWorkflowRef.workflow = &wf

	// Format as instruction to coordinator
	content := fmt.Sprintf("[WORKFLOW: %s]\n\n%s", wf.Name, wf.Content)

	// Persist workflow state to session for coordinator refresh continuity
	if m.session != nil {
		workflowState := &workflow.WorkflowState{
			WorkflowID:      wf.ID,
			WorkflowName:    wf.Name,
			WorkflowContent: wf.Content,
			StartedAt:       time.Now(),
		}
		if err := m.session.SetActiveWorkflowState(workflowState); err != nil {
			// Log warning but don't fail - workflow still works without persistence
			log.Warn(log.CatOrch, "Failed to persist workflow state", "workflowID", wf.ID, "error", err)
		}
	}

	return m.handleUserInputToCoordinator(content)
}

// GetWorkflowConfig implements adapter.WorkflowConfigProvider.
// Returns the workflow-specific prompt config for the given agent type,
// or nil if no workflow is active or no customizations exist for this agent type.
func (m *Model) GetWorkflowConfig(agentType roles.AgentType) *roles.WorkflowConfig {
	if m.activeWorkflowRef == nil || m.activeWorkflowRef.workflow == nil {
		return nil
	}
	activeWf := m.activeWorkflowRef.workflow
	if activeWf.AgentRoles == nil {
		return nil
	}

	roleConfig, ok := activeWf.AgentRoles[string(agentType)]
	if !ok {
		log.Debug(log.CatOrch, "GetWorkflowConfig: no config for agent_type=%s in workflow", agentType)
		return nil
	}

	return &roles.WorkflowConfig{
		SystemPromptAppend:   roleConfig.SystemPromptAppend,
		SystemPromptOverride: roleConfig.SystemPromptOverride,
		Constraints:          roleConfig.Constraints,
	}
}

// RestoreFromSession populates the TUI panes from a RestoredUIState.
// This method is called during session resumption to restore the visual state
// from a previously saved session.
//
// It restores:
// - Coordinator pane: messages
// - Worker pane: workerIDs, workerStatus, workerPhases, workerMessages, retiredOrder
// - Message pane: entries
//
// All viewports are initialized for workers (including retired) and all
// contentDirty flags are set to true to trigger re-rendering.
// Queue counts are zeroed since live queue state is not persisted.
func (m Model) RestoreFromSession(state *session.RestoredUIState) Model {
	if state == nil {
		return m
	}

	// === Restore Coordinator Pane ===
	m.coordinatorPane.messages = state.CoordinatorMessages
	m.coordinatorPane.contentDirty = true

	// === Restore Worker Pane ===
	// First, ensure all maps are initialized to prevent nil map panics.
	// These should be initialized by newWorkerPane(), but we check just in case.
	if m.workerPane.workerStatus == nil {
		m.workerPane.workerStatus = make(map[string]events.ProcessStatus)
	}
	if m.workerPane.workerPhases == nil {
		m.workerPane.workerPhases = make(map[string]events.ProcessPhase)
	}
	if m.workerPane.workerMessages == nil {
		m.workerPane.workerMessages = make(map[string][]ChatMessage)
	}
	if m.workerPane.workerTaskIDs == nil {
		m.workerPane.workerTaskIDs = make(map[string]string)
	}
	if m.workerPane.workerMetrics == nil {
		m.workerPane.workerMetrics = make(map[string]*metrics.TokenMetrics)
	}
	if m.workerPane.workerQueueCounts == nil {
		m.workerPane.workerQueueCounts = make(map[string]int)
	}
	if m.workerPane.viewports == nil {
		m.workerPane.viewports = make(map[string]viewport.Model)
	}
	if m.workerPane.contentDirty == nil {
		m.workerPane.contentDirty = make(map[string]bool)
	}
	if m.workerPane.hasNewContent == nil {
		m.workerPane.hasNewContent = make(map[string]bool)
	}

	// Restore workerIDs (display order: active workers first, then retired)
	// Note: RestoredUIState.WorkerIDs already has the correct order
	m.workerPane.workerIDs = make([]string, 0, len(state.WorkerIDs))
	for _, workerID := range state.WorkerIDs {
		// Only add active workers to workerIDs (retired are tracked via retiredOrder)
		if state.WorkerStatus[workerID] != events.ProcessStatusRetired {
			m.workerPane.workerIDs = append(m.workerPane.workerIDs, workerID)
		}
	}

	// Restore retired order
	m.workerPane.retiredOrder = make([]string, len(state.RetiredOrder))
	copy(m.workerPane.retiredOrder, state.RetiredOrder)

	// Restore per-worker state for ALL workers (active and retired)
	for _, workerID := range state.WorkerIDs {
		// Status
		if status, ok := state.WorkerStatus[workerID]; ok {
			m.workerPane.workerStatus[workerID] = status
		}

		// Phase
		if phase, ok := state.WorkerPhases[workerID]; ok {
			m.workerPane.workerPhases[workerID] = phase
		}

		// Messages
		if msgs, ok := state.WorkerMessages[workerID]; ok {
			m.workerPane.workerMessages[workerID] = msgs
		}

		// Initialize viewport (size 0,0 since actual size is set during render)
		m.workerPane.viewports[workerID] = viewport.New(0, 0)

		// Set dirty flag to trigger re-render
		m.workerPane.contentDirty[workerID] = true

		// Clear new content indicator
		m.workerPane.hasNewContent[workerID] = false

		// Zero queue count (not restored from session)
		m.workerPane.workerQueueCounts[workerID] = 0
	}

	// === Restore Message Pane ===
	m.messagePane.entries = state.MessageLogEntries
	m.messagePane.contentDirty = true

	return m
}
