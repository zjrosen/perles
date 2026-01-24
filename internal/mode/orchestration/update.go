package orchestration

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/zjrosen/perles/internal/flags"
	infragit "github.com/zjrosen/perles/internal/git/infrastructure"
	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/mcp"
	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/orchestration/session"
	"github.com/zjrosen/perles/internal/orchestration/v2/adapter"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/processor"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
	"github.com/zjrosen/perles/internal/pubsub"
	"github.com/zjrosen/perles/internal/ui/commandpalette"
	"github.com/zjrosen/perles/internal/ui/shared/formmodal"
	"github.com/zjrosen/perles/internal/ui/shared/modal"
	"github.com/zjrosen/perles/internal/ui/shared/quitmodal"
	"github.com/zjrosen/perles/internal/ui/shared/vimtextarea"
)

// KeyMap defines the keybindings for orchestration mode.
type KeyMap struct {
	Tab        key.Binding
	Enter      key.Binding
	Pause      key.Binding
	Replace    key.Binding
	Quit       key.Binding
	Help       key.Binding
	Fullscreen key.Binding
	Workflows  key.Binding
}

// DefaultKeyMap returns the default keybindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "cycle target"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "send message"),
		),
		Pause: key.NewBinding(
			key.WithKeys("ctrl+z"),
			key.WithHelp("ctrl+z", "pause/resume"),
		),
		Replace: key.NewBinding(
			key.WithKeys("ctrl+r"),
			key.WithHelp("ctrl+r", "replace coordinator"),
		),
		Quit: key.NewBinding(
			key.WithKeys("esc", "ctrl+c"),
			key.WithHelp("esc/ctrl+c", "quit"),
		),
		Help: key.NewBinding(
			key.WithKeys("ctrl+?"),
			key.WithHelp("ctrl+?", "toggle help"),
		),
		Fullscreen: key.NewBinding(
			key.WithKeys("ctrl+f"),
			key.WithHelp("ctrl+f", "toggle navigation mode"),
		),
		Workflows: key.NewBinding(
			key.WithKeys("ctrl+p"),
			key.WithHelp("ctrl+p", "workflow templates"),
		),
	}
}

// Message types for orchestration mode.

// UserInputMsg is sent when the user submits input to a target.
type UserInputMsg struct {
	Content string
	Target  string // "COORDINATOR" or worker ID
}

// QuitMsg requests exiting orchestration mode.
type QuitMsg struct{}

// PauseMsg toggles pause/resume of the workflow.
type PauseMsg struct{}

// ReplaceCoordinatorMsg requests replacing the coordinator process.
type ReplaceCoordinatorMsg struct{}

// RefreshTimeoutMsg indicates handoff timed out.
type RefreshTimeoutMsg struct{}

// Coordinator event messages

// StartCoordinatorMsg signals to start the coordinator.
type StartCoordinatorMsg struct{}

// spinnerTick returns a command that sends SpinnerTickMsg after 80ms.
// Used to animate the braille spinner during initialization loading phases.
func spinnerTick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg {
		return SpinnerTickMsg{}
	})
}

// CoordinatorErrorMsg indicates an error from the coordinator.
// Still used by handleUserInputToCoordinator for error handling.
type CoordinatorErrorMsg struct {
	Error error
}

// WorkerErrorMsg indicates an error from a worker.
// Still used by handleUserInputToWorker and handleUserInputBroadcast.
type WorkerErrorMsg struct {
	WorkerID string
	Error    error
}

// UserMessageQueuedMsg indicates a user message was queued for a busy worker.
// This allows the TUI to show feedback to the user.
type UserMessageQueuedMsg struct {
	WorkerID      string
	QueuePosition int
}

// CoordinatorMessageQueuedMsg indicates a user message was queued for the busy coordinator.
// This allows the TUI to show feedback to the user.
type CoordinatorMessageQueuedMsg struct {
	QueuePosition int
}

// Update handles messages and returns updated model and commands.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	keys := DefaultKeyMap()

	// Clear dirty flags after View() has had a chance to use them
	// This ensures auto-scroll happens once when new content arrives,
	// then manual scroll position is preserved
	m.coordinatorPane.contentDirty = false
	m.messagePane.contentDirty = false
	m.commandPane.contentDirty = false
	// Clear per-worker dirty flags
	for workerID := range m.workerPane.contentDirty {
		m.workerPane.contentDirty[workerID] = false
	}

	// Handle uncommitted changes warning modal first when visible
	if m.uncommittedModal.IsVisible() {
		var cmd tea.Cmd
		var result quitmodal.Result
		m.uncommittedModal, cmd, result = m.uncommittedModal.Update(msg)
		switch result {
		case quitmodal.ResultQuit:
			// User confirmed discard - proceed with exit
			return m, func() tea.Msg { return QuitMsg{} }
		case quitmodal.ResultCancel:
			// User cancelled - return to orchestration
			return m, nil
		}
		return m, cmd
	}

	// Handle quit modal messages first when visible
	// Note: Uncommitted changes are checked BEFORE showing quit modal (see showQuitModal).
	// If we reach this point with quitModal visible, the worktree is clean.
	if m.quitModal.IsVisible() {
		var cmd tea.Cmd
		var result quitmodal.Result
		m.quitModal, cmd, result = m.quitModal.Update(msg)
		switch result {
		case quitmodal.ResultQuit:
			// Worktree is clean (checked before showing modal) - proceed with exit
			return m, func() tea.Msg { return QuitMsg{} }
		case quitmodal.ResultCancel:
			return m, nil
		}
		return m, cmd
	}

	switch msg := msg.(type) {
	// Handle modal cancel for error modal (quit modal handled above via quitmodal.Result)
	case modal.CancelMsg:
		// Check if this is from worktree modal (user chose "No")
		if m.worktreeModal != nil {
			m.worktreeModal = nil
			m.worktreeDecisionMade = true
			m.worktreeEnabled = false
			// Continue to start coordinator without worktree
			return m.handleStartCoordinator()
		}
		return m, nil

	// Handle modal submit for worktree modal
	case modal.SubmitMsg:
		// Worktree modal confirmed - user chose "Yes"
		if m.worktreeModal != nil {
			m.worktreeModal = nil
			m.worktreeEnabled = true

			// Always show branch selection modal so user can choose base branch
			return m.showBranchSelectionModal()
		}

		return m, nil

	// Handle formmodal submit for branch selection
	case formmodal.SubmitMsg:
		if m.branchSelectModal != nil {
			m.branchSelectModal = nil
			m.worktreeDecisionMade = true
			// The msg.Values will contain the selected base branch
			if branch, ok := msg.Values["branch"].(string); ok && branch != "" {
				m.worktreeBaseBranch = branch
			}
			// Extract custom branch name if provided
			if customBranch, ok := msg.Values["custom_branch"].(string); ok {
				m.worktreeCustomBranch = strings.TrimSpace(customBranch)
			}
			return m.handleStartCoordinator()
		}
		return m, nil

	// Handle formmodal cancel for branch selection
	case formmodal.CancelMsg:
		if m.branchSelectModal != nil {
			// User cancelled branch selection - go back to worktree modal
			m.branchSelectModal = nil
			m.worktreeDecisionMade = false
			return m.handleStartCoordinator()
		}
		return m, nil

	case tea.KeyMsg:

		// Forward key events to workflow picker when it's visible
		if m.showWorkflowPicker && m.workflowPicker != nil {
			var cmd tea.Cmd
			updatedPicker, cmd := m.workflowPicker.Update(msg)
			m.workflowPicker = &updatedPicker
			return m, cmd
		}

		// Forward key events to worktree modal when it's visible
		if m.worktreeModal != nil {
			var cmd tea.Cmd
			*m.worktreeModal, cmd = m.worktreeModal.Update(msg)
			return m, cmd
		}

		// Forward key events to branch selection modal when it's visible
		if m.branchSelectModal != nil {
			var cmd tea.Cmd
			*m.branchSelectModal, cmd = m.branchSelectModal.Update(msg)
			return m, cmd
		}

		// Handle keys during initialization phases
		initPhase := m.getInitPhase()
		if initPhase != InitReady && initPhase != InitNotStarted {
			// In failed/timeout state: R retries, S skips (worktree only), ESC/Ctrl+C exits
			if initPhase == InitFailed || initPhase == InitTimedOut {
				failedPhase := m.getFailedPhase()
				switch {
				case msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && (msg.Runes[0] == 'r' || msg.Runes[0] == 'R'):
					// Retry: use the initializer's Retry method
					if m.initializer != nil && m.initListener != nil {
						if err := m.initializer.Retry(); err != nil {
							return m.SetError(err.Error())
						}
						// Reset spinner frame for view
						m.spinnerFrame = 0
						return m, tea.Batch(spinnerTick(), m.initListener.Listen())
					}
					// Fallback if no initializer or listener (e.g., in tests) - restart initialization
					m.Cleanup()
					return m, func() tea.Msg { return StartCoordinatorMsg{} }
				case msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && (msg.Runes[0] == 's' || msg.Runes[0] == 'S'):
					// Skip worktree: only available when worktree creation failed
					if failedPhase == InitCreatingWorktree {
						// Disable worktree and restart initialization
						m.worktreeEnabled = false
						m.worktreeDecisionMade = true
						m.Cleanup()
						return m, func() tea.Msg { return StartCoordinatorMsg{} }
					}
				case key.Matches(msg, keys.Quit) || msg.Type == tea.KeyCtrlC || msg.Type == tea.KeyEsc:
					return m, func() tea.Msg { return QuitMsg{} }
				}
				return m, nil // Ignore all other keys in error/timeout state
			}

			// During active loading phases: only ESC/Ctrl+C cancels
			if key.Matches(msg, keys.Quit) || msg.Type == tea.KeyCtrlC {
				return m, func() tea.Msg { return QuitMsg{} }
			}
			return m, nil // Block all other input during loading
		}

		// ctrl+f toggles navigation mode (works in both modes)
		if key.Matches(msg, keys.Fullscreen) {
			m = m.toggleNavigationMode()
			return m, nil
		}

		// Navigation mode: number keys select panes, esc exits
		if m.navigationMode {
			switch {
			case msg.Type == tea.KeyEsc:
				m = m.exitNavigationMode()
				return m, nil
			case msg.Type == tea.KeyRunes && len(msg.Runes) == 1:
				switch msg.Runes[0] {
				case '1', '2', '3', '4':
					workerIndex := int(msg.Runes[0] - '1')
					m = m.toggleFullscreenPane(PaneWorker, workerIndex)
					return m, nil
				case '5':
					m = m.toggleFullscreenPane(PaneCoordinator, 0)
					return m, nil
				case '6':
					m = m.toggleFullscreenPane(PaneMessages, 0)
					return m, nil
				case '7':
					m = m.toggleFullscreenPane(PaneCommand, 0)
					return m, nil
				}
			case key.Matches(msg, keys.Quit) || msg.Type == tea.KeyCtrlC:
				m.showQuitModal()
				return m, nil
			}
			return m, nil
		}

		// When vim is disabled, or we are in normal mode, ESC should show quit confirmation directly
		if (!m.input.VimEnabled() || m.input.InNormalMode()) && msg.Type == tea.KeyEsc {
			m.showQuitModal()
			return m, nil
		}

		// When vim is disabled, or we are in normal mode, ctrl+c should show quit confirmation directly
		if (!m.input.VimEnabled() || m.input.InNormalMode()) && msg.Type == tea.KeyCtrlC {
			m.showQuitModal()
			return m, nil
		}

		switch {
		case key.Matches(msg, keys.Pause):
			return m, func() tea.Msg { return PauseMsg{} }

		case key.Matches(msg, keys.Replace):
			return m, func() tea.Msg { return ReplaceCoordinatorMsg{} }

		case key.Matches(msg, keys.Tab):
			m = m.CycleMessageTarget()
			return m, nil

		case key.Matches(msg, keys.Workflows):
			m = m.openWorkflowPicker()
			return m, nil
		}

		// Forward all other keys to vimtextarea (including ESC which switches to Normal when vim enabled)
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd

	case tea.WindowSizeMsg:
		m = m.SetSize(msg.Width, msg.Height)
		return m, nil

	case tea.MouseMsg:
		// Forward mouse events to workflow picker when it's visible
		if m.showWorkflowPicker && m.workflowPicker != nil {
			var cmd tea.Cmd
			updatedPicker, cmd := m.workflowPicker.Update(msg)
			m.workflowPicker = &updatedPicker
			return m, cmd
		}

		// Only handle wheel events for scrolling
		if msg.Button != tea.MouseButtonWheelUp && msg.Button != tea.MouseButtonWheelDown {
			return m, nil
		}

		// Calculate pane boundaries based on layout
		leftWidth := m.width * leftPanePercent / 100
		middleWidth := m.width * middlePanePercent / 100
		contentHeight := m.height - 4 // Reserve 4 lines for input bar

		// Ignore mouse events in input bar area (bottom 4 lines)
		if msg.Y >= contentHeight {
			return m, nil
		}

		// Determine scroll direction
		scrollLines := 1
		scrollUp := msg.Button == tea.MouseButtonWheelUp

		// Route scroll to appropriate pane based on X coordinate
		switch {
		case msg.X < leftWidth:
			// Coordinator pane
			vp := m.coordinatorPane.viewports[viewportKey]
			if scrollUp {
				vp.ScrollUp(scrollLines)
			} else {
				vp.ScrollDown(scrollLines)
			}
			m.coordinatorPane.viewports[viewportKey] = vp
			// Clear new content indicator when scrolled to bottom
			if vp.AtBottom() {
				m.coordinatorPane.hasNewContent = false
			}

		case msg.X < leftWidth+middleWidth:
			// Middle column: command pane (if visible) + message pane
			// Command pane gets 30% of content height when visible
			cmdPaneHeight := contentHeight * 30 / 100
			if m.showCommandPane && msg.Y < cmdPaneHeight {
				// Command pane scroll (top of middle column when visible)
				vp := m.commandPane.viewports[viewportKey]
				if scrollUp {
					vp.ScrollUp(scrollLines)
				} else {
					vp.ScrollDown(scrollLines)
				}
				m.commandPane.viewports[viewportKey] = vp
				// Clear new content indicator when scrolled to bottom
				if vp.AtBottom() {
					m.commandPane.hasNewContent = false
				}
			} else {
				// Message pane scroll
				vp := m.messagePane.viewports[viewportKey]
				if scrollUp {
					vp.ScrollUp(scrollLines)
				} else {
					vp.ScrollDown(scrollLines)
				}
				m.messagePane.viewports[viewportKey] = vp
				// Clear new content indicator when scrolled to bottom
				if vp.AtBottom() {
					m.messagePane.hasNewContent = false
				}
			}

		default:
			// Worker pane - determine which stacked worker pane based on Y coordinate
			activeWorkerIDs := m.ActiveWorkerIDs()

			if len(activeWorkerIDs) > 0 {
				// Calculate height per worker pane (matches renderWorkerPanes)
				heightPerWorker := max(contentHeight/len(activeWorkerIDs), minHeightPerWorker)

				// Determine which worker pane the mouse is in based on Y
				workerIndex := msg.Y / heightPerWorker
				if workerIndex >= len(activeWorkerIDs) {
					workerIndex = len(activeWorkerIDs) - 1
				}
				targetWorkerID := activeWorkerIDs[workerIndex]
				if vp, ok := m.workerPane.viewports[targetWorkerID]; ok {
					if scrollUp {
						vp.ScrollUp(scrollLines)
					} else {
						vp.ScrollDown(scrollLines)
					}
					m.workerPane.viewports[targetWorkerID] = vp
					// Clear new content indicator when scrolled to bottom
					if vp.AtBottom() {
						m.workerPane.hasNewContent[targetWorkerID] = false
					}
				}
			}
		}

		return m, nil

	// Coordinator lifecycle messages
	case StartCoordinatorMsg:
		return m.handleStartCoordinator()

	// Handle message events from pub/sub
	case pubsub.Event[message.Event]:
		return m.handleMessageEvent(msg)

	// Handle v2 orchestration events from pub/sub (includes all worker events)
	case pubsub.Event[any]:
		return m.handleV2Event(msg)

	// Handle vimtextarea submit (Shift+Enter)
	case vimtextarea.SubmitMsg:
		content := strings.TrimSpace(msg.Content)
		if content != "" {
			target := m.messageTarget
			m.input.Reset()
			return m, func() tea.Msg {
				return UserInputMsg{Content: content, Target: target}
			}
		}
		return m, nil

	// Handle vimtextarea mode change
	case vimtextarea.ModeChangeMsg:
		m.vimMode = msg.Mode
		return m, nil

	// Wire UserInputMsg to target (coordinator or worker)
	case UserInputMsg:
		return m.handleUserInput(msg.Content, msg.Target)

	// Wire PauseMsg to coordinator
	case PauseMsg:
		return m.handlePauseToggle()

	// Wire ReplaceCoordinatorMsg to coordinator
	case ReplaceCoordinatorMsg:
		return m.handleReplaceCoordinator()

	// Handle coordinator errors from user input commands
	case CoordinatorErrorMsg:
		return m.SetError(msg.Error.Error())

	// Handle worker errors from user input commands
	case WorkerErrorMsg:
		log.Debug(log.CatOrch, "Worker error", "subsystem", "update", "workerID", msg.WorkerID, "error", msg.Error)
		return m, nil

	// Handle user message queued feedback (for workers)
	case UserMessageQueuedMsg:
		// Add a system message to the worker pane showing the message was queued
		queuedFeedback := fmt.Sprintf("Message queued (position %d) - worker is busy", msg.QueuePosition)
		m = m.AddWorkerMessage(msg.WorkerID, "system", queuedFeedback, false)
		log.Debug(log.CatOrch, "User message queued feedback shown", "subsystem", "update", "workerID", msg.WorkerID, "position", msg.QueuePosition)
		return m, nil

	// Handle coordinator message queued feedback
	case CoordinatorMessageQueuedMsg:
		// Add a system message to the coordinator pane showing the message was queued
		queuedFeedback := fmt.Sprintf("Message queued (position %d) - coordinator is busy", msg.QueuePosition)
		m = m.AddChatMessage("system", queuedFeedback, false)
		log.Debug(log.CatOrch, "Coordinator message queued feedback shown", "subsystem", "update", "position", msg.QueuePosition)
		return m, nil

	// Handle workflow picker selection
	case commandpalette.SelectMsg:
		return m.handleWorkflowSelected(msg.Item)

	// Handle workflow picker cancel
	case commandpalette.CancelMsg:
		m.showWorkflowPicker = false
		m.workflowPicker = nil
		return m, nil

	// Handle spinner animation tick
	case SpinnerTickMsg:
		// Only continue ticking during active loading phases
		phase := m.getInitPhase()
		if phase == InitReady ||
			phase == InitFailed ||
			phase == InitTimedOut ||
			phase == InitNotStarted {
			// Terminal or inactive state - stop spinning
			return m, nil
		}
		// Advance to next frame and continue ticking
		m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerFrames)
		return m, spinnerTick()

	// Handle initialization timeout (used by tests to simulate timeout)
	case InitTimeoutMsg:
		// This message is used by tests to simulate timeout
		// The actual timeout is handled by the Initializer
		return m, nil

	// Handle initializer events from the state machine
	case pubsub.Event[InitializerEvent]:
		return m.handleInitializerEvent(msg)

	// Handle refresh timeout - coordinator didn't respond to handoff request
	case RefreshTimeoutMsg:
		if m.pendingRefresh {
			m.pendingRefresh = false
			cmdSubmitter := m.cmdSubmitter()
			if cmdSubmitter == nil {
				return m.SetError("Command submitter not available")
			}
			msgRepo := m.messageRepo
			return m, func() tea.Msg {
				log.Debug(log.CatOrch, "Handoff timeout - proceeding with generic handoff", "subsystem", "update")
				// Post fallback message
				if msgRepo != nil {
					_, _ = msgRepo.Append(
						message.ActorCoordinator,
						message.ActorAll,
						"[HANDOFF] Context refresh initiated (coordinator did not respond)",
						message.MessageHandoff,
					)
				}
				// Submit replace command via v2
				cmd := command.NewReplaceProcessCommand(command.SourceUser, repository.CoordinatorID, "handoff_timeout")
				cmdSubmitter.Submit(cmd)
				return nil
			}
		}
		return m, nil

	// Handle session resumption messages
	case ResumeSessionMsg:
		return m.handleResumeSession(msg)

	case StartRestoredSessionMsg:
		return m.handleStartRestoredSession(msg)
	}

	return m, nil
}

// --- Pub/sub event handlers ---

// handleMessageEvent processes message events from the pub/sub broker.
// CRITICAL: Always returns messageListener.Listen() to continue receiving events.
func (m Model) handleMessageEvent(event pubsub.Event[message.Event]) (Model, tea.Cmd) {
	if m.messageListener == nil {
		return m, nil
	}

	payload := event.Payload

	switch payload.Type {
	case message.EventPosted:
		// Append entry to message pane directly (real-time, no polling needed)
		m = m.AppendMessageEntry(payload.Entry)

		entry := payload.Entry

		// Check if this is a handoff message while we're waiting for refresh
		if m.pendingRefresh && entry.Type == message.MessageHandoff {
			m.pendingRefresh = false
			// Trigger the actual replacement via v2 command
			if cmdSubmitter := m.cmdSubmitter(); cmdSubmitter != nil {
				cmd := command.NewReplaceProcessCommand(command.SourceUser, repository.CoordinatorID, "handoff_received")
				cmdSubmitter.Submit(cmd)
			}
			return m, m.messageListener.Listen()
		}
	}

	return m, m.messageListener.Listen()
}

// handleV2Event processes v2 orchestration events from the unified v2EventBus.
// This is the single source of truth for all process events in the TUI.
// Events are routed based on ProcessEvent.Role for coordinator vs worker handling.
// CRITICAL: Always returns v2Listener.Listen() to continue receiving events.
func (m Model) handleV2Event(event pubsub.Event[any]) (Model, tea.Cmd) {
	if m.v2Listener == nil {
		return m, nil
	}

	// Type-assert to known event types
	switch payload := event.Payload.(type) {
	case events.ProcessEvent:
		// Handle unified ProcessEvent - route based on Role
		if payload.IsCoordinator() {
			m = m.handleCoordinatorProcessEvent(payload)
		} else {
			m = m.handleWorkerProcessEvent(payload)
		}

	case processor.CommandErrorEvent:
		log.Debug(log.CatOrch, "command error", "type", payload.CommandType, "error", payload.Error)

	case processor.CommandLogEvent:
		// CRITICAL: Always append entries regardless of showCommandPane state.
		// This ensures debugging history is available when users toggle the pane on.
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
		m.commandPane.entries = append(m.commandPane.entries, entry)

		// Apply max entry bounds checking (FIFO eviction)
		if len(m.commandPane.entries) > maxCommandLogEntries {
			m.commandPane.entries = m.commandPane.entries[1:]
		}

		m.commandPane.contentDirty = true

		// Only check hasNewContent if pane is visible and not at bottom
		if m.showCommandPane && !m.commandPane.viewports[viewportKey].AtBottom() {
			m.commandPane.hasNewContent = true
		}

	default:
		// Unknown event types are handled gracefully - just continue listening
	}

	return m, m.v2Listener.Listen()
}

// handleCoordinatorProcessEvent handles ProcessEvent events for the coordinator role.
// Routes coordinator-specific events to update coordinator pane state.
func (m Model) handleCoordinatorProcessEvent(evt events.ProcessEvent) Model {
	switch evt.Type {
	case events.ProcessSpawned:
		// Coordinator pane initialization is handled elsewhere (initializer)

	case events.ProcessOutput:
		if evt.Output != "" {
			m = m.AddChatMessage("coordinator", evt.Output, evt.Delta)
		}

	case events.ProcessReady:
		m.coordinatorWorking = false
		m.coordinatorStatus = events.ProcessStatusReady

	case events.ProcessWorking:
		m.coordinatorWorking = true
		m.coordinatorStatus = events.ProcessStatusWorking

	case events.ProcessIncoming:
		if evt.Message != "" {
			m = m.AddChatMessage("user", evt.Message, false)
		}

	case events.ProcessTokenUsage:
		if evt.Metrics != nil {
			m.coordinatorMetrics = evt.Metrics
		}

	case events.ProcessError:
		log.Debug(log.CatOrch, "coordinator error", "error", evt.Error)
		// Only show error toast when past initialization - init screen shows errors inline
		// Note: We can't return a cmd from this handler, so we log the error instead
		// The error is already shown in the coordinator output via "⚠️ Error: ..." message
		if evt.Error != nil && m.getInitPhase() == InitReady {
			log.Debug(log.CatOrch, "coordinator error (shown in output)", "error", evt.Error)
		}

	case events.ProcessQueueChanged:
		m.coordinatorPane.queueCount = evt.QueueCount

	case events.ProcessStatusChange:
		// Update coordinator status for UI rendering
		m.coordinatorStatus = evt.Status
		m = m.updateStatusFromProcessStatus(evt.Status)

	case events.ProcessAutoRefreshRequired:
		// Display notification to user about auto-refresh
		// Note: The actual refresh is triggered by ProcessTurnCompleteHandler, not here
		m = m.AddChatMessage("system", "⚡ Context limit reached. Auto-refreshing coordinator...", false)
	}

	return m
}

// handleWorkerProcessEvent handles ProcessEvent events for worker roles.
// Routes worker-specific events to update worker pane state.
func (m Model) handleWorkerProcessEvent(evt events.ProcessEvent) Model {
	workerID := evt.ProcessID

	// Update task ID and phase if present in any event
	if evt.TaskID != "" {
		m.workerPane.workerTaskIDs[workerID] = evt.TaskID
	}
	if evt.Phase != nil {
		m.workerPane.workerPhases[workerID] = *evt.Phase
	}

	switch evt.Type {
	case events.ProcessSpawned:
		m = m.UpdateWorker(workerID, evt.Status)

	case events.ProcessOutput:
		if evt.Output != "" {
			m = m.AddWorkerMessage(workerID, "worker", evt.Output, evt.Delta)
		}

	case events.ProcessReady:
		m = m.UpdateWorker(workerID, events.ProcessStatusReady)

	case events.ProcessWorking:
		m = m.UpdateWorker(workerID, events.ProcessStatusWorking)

	case events.ProcessIncoming:
		// ProcessIncoming indicates a message was delivered to the worker.
		// Display messages from both user and coordinator.
		if evt.Message != "" {
			m = m.AddWorkerMessage(workerID, evt.Sender, evt.Message, false)
		}

	case events.ProcessTokenUsage:
		if evt.Metrics != nil {
			m.workerPane.workerMetrics[workerID] = evt.Metrics
		}

	case events.ProcessError:
		log.Debug(log.CatOrch, "worker error", "workerID", workerID, "error", evt.Error)
		// Worker errors are logged but not shown in modal (non-fatal)

	case events.ProcessQueueChanged:
		m = m.SetQueueCount(workerID, evt.QueueCount)

	case events.ProcessStatusChange:
		m = m.UpdateWorker(workerID, evt.Status)
	}

	return m
}

// updateStatusFromProcessStatus updates model state based on ProcessStatus.
func (m Model) updateStatusFromProcessStatus(status events.ProcessStatus) Model {
	switch status {
	case events.ProcessStatusWorking, events.ProcessStatusReady:
		m.paused = false
	case events.ProcessStatusPaused:
		m.paused = true
	}
	return m
}

// handleInitializerEvent processes events from the Initializer state machine.
func (m Model) handleInitializerEvent(event pubsub.Event[InitializerEvent]) (Model, tea.Cmd) {
	payload := event.Payload

	switch payload.Type {
	case InitEventPhaseChanged:
		// Set up TUI event subscriptions once coordinator is available
		// This allows panes to populate during loading
		// Note: Coordinator events now flow through v2EventBus via ProcessEvent
		if payload.Phase >= InitAwaitingFirstMessage && m.v2Listener == nil {
			// Get v2Infra for process state queries and command submission
			if v2Infra := m.initializer.GetV2Infra(); v2Infra != nil {
				m.v2Infra = v2Infra

				// Wire up the workflow config provider so spawned workers get workflow-specific prompts
				v2Infra.Core.Adapter.SetWorkflowConfigProvider(&m)

				if msgRepo := m.initializer.GetMessageRepo(); msgRepo != nil {
					m.messageRepo = msgRepo
					m.messageListener = pubsub.NewContinuousListener(m.ctx, msgRepo.Broker())
				}

				// Set up v2 event subscription before workers spawn
				// This is the single source of truth for all process events (coordinator + workers)
				if v2Bus := m.initializer.GetV2EventBus(); v2Bus != nil {
					m.v2Listener = pubsub.NewContinuousListener(m.ctx, v2Bus)
				}

				cmds := []tea.Cmd{
					m.initListener.Listen(),
				}
				if m.messageListener != nil {
					cmds = append(cmds, m.messageListener.Listen())
				}
				if m.v2Listener != nil {
					cmds = append(cmds, m.v2Listener.Listen())
				}
				return m, tea.Batch(cmds...)
			}
		}

		return m, m.initListener.Listen()

	case InitEventReady:
		// Grab all resources from the initializer
		res := m.initializer.Resources()
		m.messageRepo = res.MessageRepo
		m.mcpServer = res.MCPServer
		m.mcpPort = res.MCPPort
		m.mcpCoordServer = res.MCPCoordServer
		m.session = res.Session

		// Get v2Infra if not already set
		if m.v2Infra == nil {
			m.v2Infra = res.V2Infra
			// Wire up the workflow config provider so spawned workers get workflow-specific prompts
			m.v2Infra.Core.Adapter.SetWorkflowConfigProvider(&m)
		}

		// Store worktree info for cleanup on exit
		m.worktreePath = m.initializer.WorktreePath()
		m.worktreeBranch = m.initializer.WorktreeBranch()

		// Set up pub/sub subscriptions if not already set up
		// (they may have been set up earlier when coordinator became available)
		// Note: All process events (coordinator + workers) flow through v2EventBus
		var cmds []tea.Cmd
		if m.messageListener == nil {
			m.messageListener = pubsub.NewContinuousListener(m.ctx, m.messageRepo.Broker())
			cmds = append(cmds, m.messageListener.Listen())
		}
		// Set up v2 event subscription if not already set up
		// This is the single source of truth for all process events (coordinator + workers)
		if m.v2Listener == nil {
			if v2Bus := m.initializer.GetV2EventBus(); v2Bus != nil {
				m.v2Listener = pubsub.NewContinuousListener(m.ctx, v2Bus)
				cmds = append(cmds, m.v2Listener.Listen())
			}
		}

		// Mark all panes dirty so they auto-scroll to bottom on first render
		m.coordinatorPane.contentDirty = true
		m.messagePane.contentDirty = true
		for workerID := range m.workerPane.contentDirty {
			m.workerPane.contentDirty[workerID] = true
		}

		// Focus input
		m.input.Focus()

		return m, tea.Batch(cmds...)

	case InitEventFailed:
		// Error state - Initializer handles the state
		return m, nil

	case InitEventTimedOut:
		// Timeout state - Initializer handles the state
		return m, nil
	}

	return m, m.initListener.Listen()
}

// --- Helper methods ---

// showQuitModal checks for uncommitted changes and shows the appropriate quit modal.
// If the worktree has uncommitted changes, shows the uncommittedModal (warning about data loss).
// If the worktree is clean or doesn't exist, shows the regular quitModal.
// This implements a single-modal flow: only ONE modal is ever shown.
func (m *Model) showQuitModal() {
	// Check for uncommitted changes FIRST (before showing any modal)
	if m.worktreePath != "" && m.services.Flags.Enabled(flags.FlagRemoveWorktree) {
		worktreeExecutor := m.worktreeExecutorFactory(m.worktreePath)
		hasChanges, err := worktreeExecutor.HasUncommittedChanges()
		if err != nil {
			// Fail-safe: assume changes exist on error and show warning
			log.Warn(log.CatOrch, "Failed to check uncommitted changes", "error", err)
			hasChanges = true
		}
		if hasChanges {
			// Dirty worktree: show uncommitted warning modal directly
			m.uncommittedModal.Show()
			return
		}
	}
	// Clean worktree or no worktree: show regular quit modal
	m.quitModal.Show()
}

// --- Handler methods ---

// handleStartCoordinator kicks off the phased initialization process.
// It creates an Initializer and subscribes to its events.
// If in a git repo and worktree decision hasn't been made, shows worktree prompt modal first.
func (m Model) handleStartCoordinator() (Model, tea.Cmd) {
	if m.workDir == "" {
		return m.SetError("Work directory not configured")
	}

	// Initialize GitExecutor if not set
	if m.gitExecutor == nil {
		m.gitExecutor = infragit.NewRealExecutor(m.workDir)
	}

	// Check if worktrees are disabled in config - bypass prompt entirely
	if m.disableWorktrees {
		m.worktreeDecisionMade = true
		m.worktreeEnabled = false
	} else if m.gitExecutor.IsGitRepo() && !m.worktreeDecisionMade {
		// Show worktree prompt modal
		mdl := modal.New(modal.Config{
			Title:       "Use Git Worktree?",
			Message:     "Create an isolated workspace for this session?\n\n• Yes: Changes happen in a separate worktree\n• No: Changes happen in current directory",
			ConfirmText: "Yes",
			CancelText:  "No",
			Required:    true, // User must explicitly choose Yes or No
			MinWidth:    46,
		})
		mdl.SetSize(m.width, m.height)
		m.worktreeModal = &mdl
		return m, nil
	}

	// Blur input during initialization - it will be re-focused when InitReady
	m.input.Blur()

	// Create InitializerConfig using builder pattern.
	// The builder centralizes Model→InitializerConfig transformation while
	// allowing runtime-only fields (Timeouts, GitExecutor, etc.) to be set separately.
	// Timeouts are loaded from config.yaml and passed through timeoutsConfig.
	// BeadsDir is extracted from services.Config for propagation to spawned AI processes.
	var beadsDir string
	if m.services.Config != nil {
		beadsDir = m.services.Config.ResolvedBeadsDir
	}
	initConfig := NewInitializerConfigFromModel(
		m.workDir,
		beadsDir,
		m.agentProviders,
		m.worktreeBaseBranch,
		m.worktreeCustomBranch,
		m.tracingConfig,
		m.sessionStorageConfig,
	).
		WithTimeouts(m.timeoutsConfig).
		WithGitExecutor(m.gitExecutor).
		WithRestoredSession(m.resumedSession).
		WithSoundService(m.services.Sounds).
		Build()

	// Create and start the Initializer
	m.initializer = NewInitializer(initConfig)

	// Create context for subscriptions
	m.ctx, m.cancel = context.WithCancel(context.Background())

	// Subscribe to initializer events
	m.initListener = pubsub.NewContinuousListener(m.ctx, m.initializer.Broker())

	// Reset spinner frame for view animation
	m.spinnerFrame = 0

	// Start the initializer
	if err := m.initializer.Start(); err != nil {
		return m.SetError(err.Error())
	}

	return m, tea.Batch(
		spinnerTick(),
		m.initListener.Listen(),
	)
}

// handleUserInput sends user input to the target (coordinator or worker).
func (m Model) handleUserInput(content, target string) (Model, tea.Cmd) {
	// Check for known slash commands first (intercept before routing to coordinator/workers)
	if strings.HasPrefix(content, "/") {
		if newModel, cmd, handled := m.handleSlashCommand(content); handled {
			return newModel, cmd
		}
		// Unknown slash commands fall through to normal message routing
	}

	// Route based on target
	switch target {
	case "COORDINATOR", "":
		return m.handleUserInputToCoordinator(content)
	case "BROADCAST":
		return m.handleUserInputBroadcast(content)
	default:
		return m.handleUserInputToWorker(content, target)
	}
}

// handleSlashCommand routes slash commands to their respective handlers.
// Returns (model, cmd, handled) where handled indicates if the command was recognized.
// Unknown commands return handled=false so they can fall through to normal message routing.
func (m Model) handleSlashCommand(content string) (Model, tea.Cmd, bool) {
	parts := strings.Fields(content)
	if len(parts) == 0 {
		return m, nil, false
	}

	// Handle two-word commands first (e.g., "/show commands", "/hide commands")
	if len(parts) >= 2 {
		twoWordCmd := parts[0] + " " + parts[1]
		switch twoWordCmd {
		case "/show commands":
			m.showCommandPane = true
			m.commandPane.contentDirty = true
			return m, nil, true
		case "/hide commands":
			m.showCommandPane = false
			return m, nil, true
		}
	}

	slashCmd := parts[0]
	switch slashCmd {
	case "/stop":
		m, cmd := m.handleStopProcessCommand(content)
		return m, cmd, true
	case "/spawn":
		m, cmd := m.handleSpawnWorkerCommand()
		return m, cmd, true
	case "/retire":
		m, cmd := m.handleRetireWorkerCommand(content)
		return m, cmd, true
	case "/replace":
		m, cmd := m.handleReplaceWorkerCommand(content)
		return m, cmd, true
	default:
		// Unknown commands are not handled - fall through to normal routing
		return m, nil, false
	}
}

// handleUserInputToCoordinator sends user input to the coordinator.
// Uses v2 command submission via CmdSendToProcess.
func (m Model) handleUserInputToCoordinator(content string) (Model, tea.Cmd) {
	cmdSubmitter := m.cmdSubmitter()
	if cmdSubmitter == nil {
		return m.SetError("Command submitter not available")
	}

	// Submit v2 command to send message to coordinator
	// The command handler will add the message to chat via event emission
	cmd := command.NewSendToProcessCommand(command.SourceUser, repository.CoordinatorID, content)
	cmdSubmitter.Submit(cmd)

	return m, nil
}

// handleUserInputToWorker sends user input directly to a worker.
// Uses v2 command submission via CmdSendToProcess.
func (m Model) handleUserInputToWorker(content, workerID string) (Model, tea.Cmd) {
	cmdSubmitter := m.cmdSubmitter()
	if cmdSubmitter == nil {
		return m.SetError("Command submitter not available")
	}

	// Submit v2 command to send message to worker
	// The command handler will add the message to chat via event emission
	cmd := command.NewSendToProcessCommand(command.SourceUser, workerID, content)
	cmdSubmitter.Submit(cmd)

	return m, nil
}

// handleUserInputBroadcast sends user input to the coordinator and all active workers.
// Uses v2 command submission via CmdSendToProcess.
func (m Model) handleUserInputBroadcast(content string) (Model, tea.Cmd) {
	cmdSubmitter := m.cmdSubmitter()
	if cmdSubmitter == nil {
		return m.SetError("Command submitter not available")
	}

	// Send to coordinator via v2 command
	cmd := command.NewSendToProcessCommand(command.SourceUser, repository.CoordinatorID, "[BROADCAST]\n"+content)
	cmdSubmitter.Submit(cmd)

	// Send to all active workers via v2 command
	for _, workerID := range m.workerPane.workerIDs {
		status := m.workerPane.workerStatus[workerID]
		// Only send to ready or working workers (not retired)
		if status == events.ProcessStatusRetired || status == events.ProcessStatusFailed {
			continue
		}

		broadcastContent := fmt.Sprintf("[BROADCAST]\n%s", content)
		cmd := command.NewSendToProcessCommand(command.SourceUser, workerID, broadcastContent)
		cmdSubmitter.Submit(cmd)
	}

	log.Debug(log.CatOrch, "Broadcast message sent", "subsystem", "update")
	return m, nil
}

// handlePauseToggle toggles the paused state using v2 commands.
func (m Model) handlePauseToggle() (Model, tea.Cmd) {
	cmdSubmitter := m.cmdSubmitter()
	if cmdSubmitter == nil {
		return m, nil
	}

	if m.paused {
		// Submit resume command - the handler will update m.paused via ProcessStatusChange event
		cmd := command.NewResumeProcessCommand(command.SourceUser, repository.CoordinatorID)
		cmdSubmitter.Submit(cmd)
	} else {
		// Submit pause command - the handler will update m.paused via ProcessStatusChange event
		cmd := command.NewPauseProcessCommand(command.SourceUser, repository.CoordinatorID, "user_requested")
		cmdSubmitter.Submit(cmd)
	}

	return m, nil
}

// handleReplaceCoordinator requests a handoff from the coordinator before replacing.
// Sets pendingRefresh flag and sends a message to the coordinator asking it to post
// a handoff message via prepare_handoff. The actual Replace() is triggered when the
// handoff message is received in handleMessageEvent.
func (m Model) handleReplaceCoordinator() (Model, tea.Cmd) {
	cmdSubmitter := m.cmdSubmitter()
	if cmdSubmitter == nil {
		return m.SetError("Command submitter not available")
	}

	// Set pending refresh flag
	m.pendingRefresh = true

	// Start timeout timer
	timeoutCmd := tea.Tick(120*time.Second, func(time.Time) tea.Msg {
		return RefreshTimeoutMsg{}
	})

	// Send message to coordinator to post handoff via v2 command
	handoffMessage := `[CONTEXT REFRESH INITIATED]

Your context window is approaching limits. The user has initiated a coordinator refresh.

WHAT'S ABOUT TO HAPPEN:
- You will be replaced with a fresh coordinator session
- All workers will continue running
- External state (message log, bd tasks, etc.) is preserved
- The new coordinator will start with a clean context window

YOUR TASK:
Call ` + "`prepare_handoff`" + ` with a comprehensive summary for the incoming coordinator. This summary is CRITICAL - it's the primary way the new coordinator will understand what work is in progress.

WHAT TO INCLUDE IN THE HANDOFF:
1. Current work state: Which workers are doing what? What tasks are in progress?
2. Recent decisions: What approach did you take? Why?
3. Blockers or issues: Anything the new coordinator should know about?
4. Recommendations: What should the new coordinator do next?
5. Context that isn't in the message log: Internal reasoning, strategy, patterns you've noticed

The more detailed your handoff, the smoother the transition will be. Think of this as briefing your replacement.

When you're ready, call: ` + "`prepare_handoff`" + ` with your summary.`

	cmd := command.NewSendToProcessCommand(command.SourceUser, repository.CoordinatorID, handoffMessage)
	cmdSubmitter.Submit(cmd)

	return m, timeoutCmd
}

// handleStopProcessCommand parses and handles the /stop <process-id> [--force] command.
// Syntax: /stop worker-1 [--force] or /stop coordinator [--force]
func (m Model) handleStopProcessCommand(content string) (Model, tea.Cmd) {
	parts := strings.Fields(content)
	if len(parts) < 2 {
		return m.SetError("Usage: /stop <process-id> [--force]")
	}

	processID := parts[1]
	force := len(parts) > 2 && parts[2] == "--force"

	cmdSubmitter := m.cmdSubmitter()
	if cmdSubmitter == nil {
		return m.SetError("Command submitter not available")
	}

	cmd := command.NewStopProcessCommand(command.SourceUser, processID, force, "user_requested")
	cmdSubmitter.Submit(cmd)

	return m, nil
}

// handleSpawnWorkerCommand handles the /spawn command to spawn a new worker.
// Syntax: /spawn (no arguments expected)
func (m Model) handleSpawnWorkerCommand() (Model, tea.Cmd) {
	cmdSubmitter := m.cmdSubmitter()
	if cmdSubmitter == nil {
		return m.SetError("Command submitter not available")
	}

	cmd := command.NewSpawnProcessCommand(command.SourceUser, repository.RoleWorker)
	cmdSubmitter.Submit(cmd)

	return m, nil
}

// handleRetireWorkerCommand handles the /retire command to retire a worker.
// Syntax: /retire <worker-id> [reason]
func (m Model) handleRetireWorkerCommand(content string) (Model, tea.Cmd) {
	parts := strings.Fields(content)
	if len(parts) < 2 {
		return m.SetError("Usage: /retire <worker-id> [reason]")
	}

	workerID := parts[1]

	// Block retiring the coordinator
	if workerID == repository.CoordinatorID {
		return m.SetError("Cannot retire coordinator. Use Ctrl+R to replace coordinator instead.")
	}

	// Pre-validate worker exists for immediate feedback
	repo := m.processRepo()
	if repo != nil {
		if _, err := repo.Get(workerID); err != nil {
			return m.SetError(fmt.Sprintf("Worker %s not found", workerID))
		}
	}

	cmdSubmitter := m.cmdSubmitter()
	if cmdSubmitter == nil {
		return m.SetError("Command submitter not available")
	}

	// Build reason from remaining arguments, default to "user_requested"
	reason := "user_requested"
	if len(parts) > 2 {
		reason = strings.Join(parts[2:], " ")
	}

	cmd := command.NewRetireProcessCommand(command.SourceUser, workerID, reason)
	cmdSubmitter.Submit(cmd)

	return m, nil
}

// handleReplaceWorkerCommand handles the /replace command to replace a worker.
// Syntax: /replace <worker-id> [reason]
// Note: Unlike /retire, /replace coordinator IS allowed (equivalent to Ctrl+R).
func (m Model) handleReplaceWorkerCommand(content string) (Model, tea.Cmd) {
	parts := strings.Fields(content)
	if len(parts) < 2 {
		return m.SetError("Usage: /replace <worker-id> [reason]")
	}

	workerID := parts[1]

	// Pre-validate worker exists for immediate feedback
	repo := m.processRepo()
	if repo != nil {
		if _, err := repo.Get(workerID); err != nil {
			return m.SetError(fmt.Sprintf("Worker %s not found", workerID))
		}
	}

	cmdSubmitter := m.cmdSubmitter()
	if cmdSubmitter == nil {
		return m.SetError("Command submitter not available")
	}

	// Build reason from remaining arguments, default to "user_requested"
	reason := "user_requested"
	if len(parts) > 2 {
		reason = strings.Join(parts[2:], " ")
	}

	cmd := command.NewReplaceProcessCommand(command.SourceUser, workerID, reason)
	cmdSubmitter.Submit(cmd)

	return m, nil
}

// workerServerCache manages worker MCP servers that share the same message store.
// Workers connect via HTTP to /worker/{workerID} and all share the coordinator's
// message repository instance, solving the in-memory cache isolation problem.
type workerServerCache struct {
	msgStore             mcp.MessageStore
	accountabilityWriter mcp.AccountabilityWriter
	v2Adapter            *adapter.V2Adapter
	turnEnforcer         mcp.ToolCallRecorder
	servers              map[string]*mcp.WorkerServer
	mu                   sync.RWMutex
}

// newWorkerServerCache creates a new worker server cache.
// The accountabilityWriter allows workers to save accountability summaries to session storage.
// The v2Adapter routes all worker MCP tool handlers through v2 orchestration.
// The turnEnforcer tracks tool calls during worker turns for compliance enforcement.
func newWorkerServerCache(msgStore mcp.MessageStore, accountabilityWriter mcp.AccountabilityWriter, v2Adapter *adapter.V2Adapter, turnEnforcer mcp.ToolCallRecorder) *workerServerCache {
	return &workerServerCache{
		msgStore:             msgStore,
		accountabilityWriter: accountabilityWriter,
		v2Adapter:            v2Adapter,
		turnEnforcer:         turnEnforcer,
		servers:              make(map[string]*mcp.WorkerServer),
	}
}

// ServeHTTP handles HTTP requests for worker MCP endpoints.
// Extracts the worker ID from the URL path and routes to the appropriate server.
func (c *workerServerCache) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Extract worker ID from path: /worker/{workerID}
	workerID := strings.TrimPrefix(r.URL.Path, "/worker/")
	if workerID == "" {
		http.Error(w, "worker ID required in path", http.StatusBadRequest)
		return
	}

	ws := c.getOrCreate(workerID)
	ws.ServeHTTP().ServeHTTP(w, r)
}

// getOrCreate returns an existing worker server or creates a new one.
func (c *workerServerCache) getOrCreate(workerID string) *mcp.WorkerServer {
	c.mu.RLock()
	ws, ok := c.servers[workerID]
	c.mu.RUnlock()
	if ok {
		return ws
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if ws, ok := c.servers[workerID]; ok {
		return ws
	}

	ws = mcp.NewWorkerServer(workerID, c.msgStore)
	// Set the accountability writer so workers can save accountability summaries to session storage
	if c.accountabilityWriter != nil {
		ws.SetAccountabilityWriter(c.accountabilityWriter)
	}
	// Set the v2 adapter so all handlers route through v2 orchestration
	if c.v2Adapter != nil {
		ws.SetV2Adapter(c.v2Adapter)
	}
	// Set the turn enforcer so tool calls are tracked for turn completion enforcement
	if c.turnEnforcer != nil {
		ws.SetTurnEnforcer(c.turnEnforcer)
	}
	c.servers[workerID] = ws
	log.Debug(log.CatOrch, "Created worker server", "subsystem", "update", "workerID", workerID)
	return ws
}

// determineSessionStatus maps the current model state to a session status.
// Returns the appropriate session.Status based on:
//   - InitTimedOut phase -> StatusTimedOut
//   - InitFailed phase -> StatusFailed
//   - Error modal present -> StatusFailed
//   - Default (normal completion) -> StatusCompleted
func (m Model) determineSessionStatus() session.Status {
	// Check if initialization timed out
	initPhase := m.getInitPhase()
	if initPhase == InitTimedOut {
		return session.StatusTimedOut
	}

	// Check if initialization failed
	if initPhase == InitFailed {
		return session.StatusFailed
	}

	// Default to completed (normal shutdown, user interrupt, etc.)
	return session.StatusCompleted
}

// showBranchSelectionModal shows the branch selection modal when user is not on main branch.
// Uses a searchable select field populated with available branches.
func (m Model) showBranchSelectionModal() (Model, tea.Cmd) {
	// Get current branch for header context
	currentBranch := "current"
	if m.gitExecutor != nil {
		if b, err := m.gitExecutor.GetCurrentBranch(); err == nil {
			currentBranch = b
		}
	}

	// Build branch options from git
	var options []formmodal.ListOption
	if m.gitExecutor != nil {
		branches, err := m.gitExecutor.ListBranches()
		if err == nil {
			for _, b := range branches {
				options = append(options, formmodal.ListOption{
					Label:    b.Name,
					Value:    b.Name,
					Selected: b.IsCurrent, // Pre-select current branch
				})
			}
		}
	}

	// Fallback if no branches found
	if len(options) == 0 {
		options = []formmodal.ListOption{
			{Label: "main", Value: "main", Selected: true},
		}
	}

	// Create branch selection modal with searchable select
	mdl := formmodal.New(formmodal.FormConfig{
		Title:       "Select Base Branch",
		MinWidth:    47,
		SubmitLabel: "Continue",
		Fields: []formmodal.FieldConfig{
			{
				Key:               "branch",
				Type:              formmodal.FieldTypeSearchSelect,
				Label:             "Base Branch",
				Hint:              "enter to change",
				Options:           options,
				SearchPlaceholder: "Search branches...",
				MaxVisibleItems:   7,
			},
			{
				Key:         "custom_branch",
				Type:        formmodal.FieldTypeText,
				Label:       "Custom Branch Name",
				Hint:        "optional",
				Placeholder: "e.g., feature/my-work",
				MaxLength:   100,
			},
		},
		Validate: func(values map[string]any) error {
			branch, _ := values["branch"].(string)
			if strings.TrimSpace(branch) == "" {
				return fmt.Errorf("please select a branch")
			}
			// Verify branch still exists
			if m.gitExecutor != nil && !m.gitExecutor.BranchExists(branch) {
				return fmt.Errorf("branch '%s' no longer exists", branch)
			}
			// Validate custom branch name if provided
			if customBranch, ok := values["custom_branch"].(string); ok {
				customBranch = strings.TrimSpace(customBranch)
				if customBranch != "" {
					if m.gitExecutor != nil {
						if err := m.gitExecutor.ValidateBranchName(customBranch); err != nil {
							return fmt.Errorf("invalid branch name: cannot contain spaces or special characters (~^:?*[)")
						}
						if m.gitExecutor.BranchExists(customBranch) {
							return fmt.Errorf("branch '%s' already exists", customBranch)
						}
					}
				}
			}
			return nil
		},
		HeaderContent: func(width int) string {
			return fmt.Sprintf("You're on '%s'. The worktree creates an isolated copy from your chosen base branch. Your current work remains untouched.", currentBranch)
		},
	})
	mdl = mdl.SetSize(m.width, m.height)
	m.branchSelectModal = &mdl

	return m, mdl.Init()
}
