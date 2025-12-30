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

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/coordinator"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/mcp"
	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/orchestration/pool"
	"github.com/zjrosen/perles/internal/orchestration/session"
	"github.com/zjrosen/perles/internal/ui/commandpalette"
	"github.com/zjrosen/perles/internal/ui/shared/modal"
	"github.com/zjrosen/perles/internal/ui/shared/vimtextarea"

	"github.com/zjrosen/perles/internal/pubsub"
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

// CoordinatorStoppedMsg indicates the coordinator stopped.
type CoordinatorStoppedMsg struct{}

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

// Update handles messages and returns updated model and commands.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	keys := DefaultKeyMap()

	// Clear dirty flags after View() has had a chance to use them
	// This ensures auto-scroll happens once when new content arrives,
	// then manual scroll position is preserved
	m.coordinatorPane.contentDirty = false
	m.messagePane.contentDirty = false
	// Clear per-worker dirty flags
	for workerID := range m.workerPane.contentDirty {
		m.workerPane.contentDirty[workerID] = false
	}

	switch msg := msg.(type) {
	// Handle quit confirmation submit
	case modal.SubmitMsg:
		if m.quitModal != nil {
			m.quitModal = nil
			return m, func() tea.Msg { return QuitMsg{} }
		}
		return m, nil

	// Handle modal cancel (dismisses quit modal or critical error)
	case modal.CancelMsg:
		if m.quitModal != nil {
			m.quitModal = nil
			return m, nil
		}
		m = m.ClearError()
		return m, nil

	case tea.KeyMsg:
		// If quit modal visible, handle force quit or forward to modal
		if m.quitModal != nil {
			// Ctrl+C again = force quit (bypass confirmation)
			if msg.Type == tea.KeyCtrlC {
				m.quitModal = nil
				return m, func() tea.Msg { return QuitMsg{} }
			}
			// Forward other keys to modal for navigation
			var cmd tea.Cmd
			*m.quitModal, cmd = m.quitModal.Update(msg)
			return m, cmd
		}

		// Forward key events to error modal when it's visible
		if m.errorModal != nil {
			var cmd tea.Cmd
			*m.errorModal, cmd = m.errorModal.Update(msg)
			return m, cmd
		}

		// Forward key events to workflow picker when it's visible
		if m.showWorkflowPicker && m.workflowPicker != nil {
			var cmd tea.Cmd
			updatedPicker, cmd := m.workflowPicker.Update(msg)
			m.workflowPicker = &updatedPicker
			return m, cmd
		}

		// Handle keys during initialization phases
		initPhase := m.getInitPhase()
		if initPhase != InitReady && initPhase != InitNotStarted {
			// In failed/timeout state: R retries, ESC/Ctrl+C exits
			if initPhase == InitFailed || initPhase == InitTimedOut {
				switch {
				case msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && (msg.Runes[0] == 'r' || msg.Runes[0] == 'R'):
					// Retry: use the initializer's Retry method
					if m.initializer != nil && m.initListener != nil {
						if err := m.initializer.Retry(); err != nil {
							m = m.SetError(err.Error())
							return m, nil
						}
						// Reset spinner frame for view
						m.spinnerFrame = 0
						return m, tea.Batch(spinnerTick(), m.initListener.Listen())
					}
					// Fallback if no initializer or listener (e.g., in tests) - restart initialization
					m.cleanup()
					return m, func() tea.Msg { return StartCoordinatorMsg{} }
				case key.Matches(msg, keys.Quit) || msg.Type == tea.KeyCtrlC:
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
				}
			case key.Matches(msg, keys.Quit) || msg.Type == tea.KeyCtrlC:
				return m.showQuitConfirmation(), nil
			}
			return m, nil
		}

		// When vim is disabled, or we are in normal mode, ESC should show quit confirmation directly
		if (!m.input.VimEnabled() || m.input.InNormalMode()) && msg.Type == tea.KeyEsc {
			return m.showQuitConfirmation(), nil
		}

		// When vim is disabled, or we are in normal mode, ctrl+c should show quit confirmation directly
		if (!m.input.VimEnabled() || m.input.InNormalMode()) && msg.Type == tea.KeyCtrlC {
			return m.showQuitConfirmation(), nil
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
			// Message pane
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

	// Handle coordinator events from pub/sub
	case pubsub.Event[events.CoordinatorEvent]:
		return m.handleCoordinatorEvent(msg)

	// Handle worker events from pub/sub
	case pubsub.Event[events.WorkerEvent]:
		return m.handleWorkerEvent(msg)

	// Handle message events from pub/sub
	case pubsub.Event[message.Event]:
		return m.handleMessageEvent(msg)

	case CoordinatorStoppedMsg:
		// Close session with appropriate status
		if m.session != nil {
			status := m.determineSessionStatus()
			if err := m.session.Close(status); err != nil {
				log.Debug(log.CatOrch, "Session close error", "subsystem", "update", "error", err)
			} else {
				log.Debug(log.CatOrch, "Session closed", "subsystem", "update", "status", status)
			}
		}

		// Shutdown HTTP MCP server
		if m.mcpServer != nil {
			go func() {
				if err := m.mcpServer.Shutdown(context.Background()); err != nil {
					log.Debug(log.CatOrch, "MCP server shutdown error", "subsystem", "update", "error", err)
				}
			}()
		}
		return m, nil

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
		m = m.SetError(msg.Error.Error())
		return m, nil

	// Handle worker errors from user input commands
	case WorkerErrorMsg:
		log.Debug(log.CatOrch, "Worker error", "subsystem", "update", "workerID", msg.WorkerID, "error", msg.Error)
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
			coord := m.coord
			msgLog := m.messageLog
			return m, func() tea.Msg {
				log.Debug(log.CatOrch, "Handoff timeout - proceeding with generic handoff", "subsystem", "update")
				// Post fallback message
				if msgLog != nil {
					_, _ = msgLog.Append(
						message.ActorCoordinator,
						message.ActorAll,
						"[HANDOFF] Context refresh initiated (coordinator did not respond)",
						message.MessageHandoff,
					)
				}
				if err := coord.Replace(); err != nil {
					return CoordinatorErrorMsg{Error: err}
				}
				return nil
			}
		}
		return m, nil
	}

	return m, nil
}

// --- Pub/sub event handlers ---

// handleCoordinatorEvent processes coordinator events from the pub/sub broker.
// CRITICAL: Always returns coordListener.Listen() to continue receiving events.
func (m Model) handleCoordinatorEvent(event pubsub.Event[events.CoordinatorEvent]) (Model, tea.Cmd) {
	if m.coordListener == nil {
		return m, nil
	}

	payload := event.Payload

	cmds := make([]tea.Cmd, 0)

	switch payload.Type {
	case events.CoordinatorChat:
		// Add message to coordinator pane
		m = m.AddChatMessage(payload.Role, payload.Content)

	case events.CoordinatorStatusChange:
		m = m.updateStatusFromCoordinator(convertCoordinatorStatus(payload.Status))

	case events.CoordinatorError:
		m = m.SetError(payload.Error.Error())

	case events.CoordinatorTokenUsage:
		if payload.Metrics != nil {
			m.coordinatorMetrics = payload.Metrics
			log.Debug(log.CatOrch, "Coordinator token usage updated",
				"subsystem", "update",
				"contextTokens", payload.Metrics.ContextTokens,
				"contextWindow", payload.Metrics.ContextWindow,
				"totalCost", payload.Metrics.TotalCostUSD)
		}

	case events.CoordinatorWorking:
		m.coordinatorWorking = true

	case events.CoordinatorReady:
		m.coordinatorWorking = false
	}

	cmds = append(cmds, m.coordListener.Listen())

	return m, tea.Batch(cmds...)
}

// handleWorkerEvent processes worker events from the pub/sub broker.
// CRITICAL: Always returns workerListener.Listen() to continue receiving events.
func (m Model) handleWorkerEvent(event pubsub.Event[events.WorkerEvent]) (Model, tea.Cmd) {
	if m.workerListener == nil {
		return m, nil
	}

	payload := event.Payload

	switch payload.Type {
	case events.WorkerSpawned:
		log.Debug(log.CatOrch, "Worker spawned in TUI", "subsystem", "update", "workerID", payload.WorkerID, "taskID", payload.TaskID, "status", payload.Status)
		m = m.UpdateWorker(payload.WorkerID, payload.Status)

	case events.WorkerOutput:
		log.Debug(log.CatOrch, "Worker output received", "subsystem", "update", "workerID", payload.WorkerID, "outputLen", len(payload.Output))
		if payload.Output != "" {
			m = m.AddWorkerMessage(payload.WorkerID, payload.Output)
		}

	case events.WorkerStatusChange:
		log.Debug(log.CatOrch, "Worker status changed", "subsystem", "update", "workerID", payload.WorkerID, "status", payload.Status)
		m = m.UpdateWorker(payload.WorkerID, payload.Status)

	case events.WorkerTokenUsage:
		if payload.Metrics != nil {
			log.Debug(log.CatOrch, "Worker token usage",
				"subsystem", "update",
				"workerID", payload.WorkerID,
				"contextTokens", payload.Metrics.ContextTokens,
				"contextWindow", payload.Metrics.ContextWindow,
				"totalCost", payload.Metrics.TotalCostUSD)
			m.workerPane.workerMetrics[payload.WorkerID] = payload.Metrics
		}

	case events.WorkerIncoming:
		log.Debug(log.CatOrch, "Coordinator message to worker",
			"subsystem", "update",
			"workerID", payload.WorkerID,
			"messageLen", len(payload.Message))
		m = m.AddWorkerMessageWithRole(payload.WorkerID, "coordinator", payload.Message)

	case events.WorkerError:
		log.Debug(log.CatOrch, "Worker error", "subsystem", "update", "workerID", payload.WorkerID, "error", payload.Error)
	}

	// Always continue listening for more events
	return m, m.workerListener.Listen()
}

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
			// Trigger the actual replacement
			coord := m.coord
			cmd := func() tea.Msg {
				if err := coord.Replace(); err != nil {
					return CoordinatorErrorMsg{Error: err}
				}
				return nil
			}
			return m, tea.Batch(m.messageListener.Listen(), cmd)
		}

		// Nudge coordinator if message is to COORDINATOR or ALL
		if entry.To == message.ActorCoordinator || entry.To == message.ActorAll {
			if m.coord != nil && m.nudgeBatcher != nil && entry.From != message.ActorCoordinator {
				// Determine message type based on the entry type
				msgType := WorkerNewMessage
				if entry.Type == message.MessageWorkerReady {
					msgType = WorkerReady
				}
				m.nudgeBatcher.Add(entry.From, msgType)
			}
		}
	}

	return m, m.messageListener.Listen()
}

// convertCoordinatorStatus converts events.CoordinatorStatus to coordinator.Status.
func convertCoordinatorStatus(s events.CoordinatorStatus) coordinator.Status {
	switch s {
	case events.StatusReady:
		return coordinator.StatusPending
	case events.StatusWorking:
		return coordinator.StatusRunning
	case events.StatusPaused:
		return coordinator.StatusPaused
	case events.StatusStopped:
		return coordinator.StatusStopped
	default:
		return coordinator.StatusPending
	}
}

// handleInitializerEvent processes events from the Initializer state machine.
func (m Model) handleInitializerEvent(event pubsub.Event[InitializerEvent]) (Model, tea.Cmd) {
	payload := event.Payload

	switch payload.Type {
	case InitEventPhaseChanged:
		// Set up TUI event subscriptions once coordinator is available
		// This allows panes to populate during loading
		if payload.Phase >= InitAwaitingFirstMessage && m.coordListener == nil {
			if coord := m.initializer.GetCoordinator(); coord != nil {
				m.coord = coord
				m.coordListener = pubsub.NewContinuousListener(m.ctx, coord.Broker())
				m.workerListener = pubsub.NewContinuousListener(m.ctx, coord.Workers())

				if msgLog := m.initializer.GetMessageLog(); msgLog != nil {
					m.messageLog = msgLog
					m.messageListener = pubsub.NewContinuousListener(m.ctx, msgLog.Broker())
				}

				// Set up nudge batcher early so worker ready messages get forwarded
				if m.nudgeBatcher == nil {
					m.nudgeBatcher = NewNudgeBatcher(1 * time.Second)
					m.nudgeBatcher.SetOnNudge(func(messagesByType map[MessageType][]string) {
						if m.coord == nil {
							return
						}

						var (
							nudge                 string
							readyMessageWorkerIds []string
							newMessageWorkerIds   []string
						)
						for messageType, workerIds := range messagesByType {
							switch messageType {
							case WorkerReady:
								readyMessageWorkerIds = append(readyMessageWorkerIds, workerIds...)
							case WorkerNewMessage:
								newMessageWorkerIds = append(newMessageWorkerIds, workerIds...)
							}
						}

						if len(readyMessageWorkerIds) > 0 {
							nudge = fmt.Sprintf("[%s] have started up and are now ready", strings.Join(readyMessageWorkerIds, ", "))
							if err := m.coord.SendUserMessage(nudge); err != nil {
								log.Debug(log.CatOrch, "Failed to nudge coordinator", "subsystem", "update", "error", err)
							}
						}

						if len(newMessageWorkerIds) > 0 {
							nudge = fmt.Sprintf("[%s sent messages] Use read_message_log to check for new messages.", strings.Join(newMessageWorkerIds, ", "))
							if err := m.coord.SendUserMessage(nudge); err != nil {
								log.Debug(log.CatOrch, "Failed to nudge coordinator", "subsystem", "update", "error", err)
							}
						}
					})
				}

				return m, tea.Batch(
					m.initListener.Listen(),
					m.coordListener.Listen(),
					m.workerListener.Listen(),
					m.messageListener.Listen(),
				)
			}
		}

		return m, m.initListener.Listen()

	case InitEventReady:
		// Grab all resources from the initializer
		res := m.initializer.Resources()
		m.aiClient = res.AIClient
		m.aiClientExtensions = res.Extensions
		m.pool = res.Pool
		m.messageLog = res.MessageLog
		m.mcpServer = res.MCPServer
		m.mcpPort = res.MCPPort
		m.coord = res.Coordinator
		m.session = res.Session

		// Set up pub/sub subscriptions if not already set up
		// (they may have been set up earlier when coordinator became available)
		var cmds []tea.Cmd
		if m.coordListener == nil {
			m.coordListener = pubsub.NewContinuousListener(m.ctx, m.coord.Broker())
			cmds = append(cmds, m.coordListener.Listen())
		}
		if m.workerListener == nil {
			m.workerListener = pubsub.NewContinuousListener(m.ctx, m.coord.Workers())
			cmds = append(cmds, m.workerListener.Listen())
		}
		if m.messageListener == nil {
			m.messageListener = pubsub.NewContinuousListener(m.ctx, m.messageLog.Broker())
			cmds = append(cmds, m.messageListener.Listen())
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

// --- Handler methods ---

// handleStartCoordinator kicks off the phased initialization process.
// It creates an Initializer and subscribes to its events.
func (m Model) handleStartCoordinator() (Model, tea.Cmd) {
	if m.workDir == "" {
		m = m.SetError("Work directory not configured")
		return m, nil
	}

	// Blur input during initialization - it will be re-focused when InitReady
	m.input.Blur()

	// Create and start the Initializer
	m.initializer = NewInitializer(InitializerConfig{
		WorkDir:         m.workDir,
		ClientType:      m.clientType,
		ClaudeModel:     m.claudeModel,
		AmpModel:        m.ampModel,
		AmpMode:         m.ampMode,
		ExpectedWorkers: 4,
		Timeout:         20 * time.Second,
	})

	// Create context for subscriptions
	m.ctx, m.cancel = context.WithCancel(context.Background())

	// Subscribe to initializer events
	m.initListener = pubsub.NewContinuousListener(m.ctx, m.initializer.Broker())

	// Reset spinner frame for view animation
	m.spinnerFrame = 0

	// Start the initializer
	if err := m.initializer.Start(); err != nil {
		m = m.SetError(err.Error())
		return m, nil
	}

	return m, tea.Batch(
		spinnerTick(),
		m.initListener.Listen(),
	)
}

// handleUserInput sends user input to the target (coordinator or worker).
func (m Model) handleUserInput(content, target string) (Model, tea.Cmd) {
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

// handleUserInputToCoordinator sends user input to the coordinator.
func (m Model) handleUserInputToCoordinator(content string) (Model, tea.Cmd) {
	if m.coord == nil {
		m = m.SetError("Coordinator not started")
		return m, nil
	}

	// Send to coordinator - it will emit an event that adds the message to chat
	return m, func() tea.Msg {
		if err := m.coord.SendUserMessage(content); err != nil {
			return CoordinatorErrorMsg{Error: err}
		}
		return nil
	}
}

// handleUserInputToWorker sends user input directly to a worker.
func (m Model) handleUserInputToWorker(content, workerID string) (Model, tea.Cmd) {
	if m.pool == nil {
		m = m.SetError("Worker pool not available")
		return m, nil
	}

	// Add message to worker pane immediately (optimistic update)
	m = m.AddWorkerMessageWithRole(workerID, "user", content)
	log.Debug(log.CatOrch, "Sending message to worker", "subsystem", "update", "workerID", workerID)

	// Get worker from pool and validate before spawning goroutine
	worker := m.pool.GetWorker(workerID)
	if worker == nil {
		return m, func() tea.Msg {
			return WorkerErrorMsg{WorkerID: workerID, Error: fmt.Errorf("worker not found: %s", workerID)}
		}
	}

	sessionID := worker.GetSessionID()
	if sessionID == "" {
		return m, func() tea.Msg {
			return WorkerErrorMsg{WorkerID: workerID, Error: fmt.Errorf("worker %s has no session ID yet", workerID)}
		}
	}

	// Build spawn config using shared helper
	cfg, err := m.buildWorkerSpawnConfig(workerID, sessionID, content)
	if err != nil {
		return m, func() tea.Msg {
			return WorkerErrorMsg{WorkerID: workerID, Error: err}
		}
	}

	// Capture pool for closure - client is accessed via pool.Client()
	workerPool := m.pool

	return m, func() tea.Msg {
		// Resume the worker's session with the user message
		proc, err := workerPool.Client().Spawn(context.Background(), cfg)
		if err != nil {
			return WorkerErrorMsg{WorkerID: workerID, Error: fmt.Errorf("failed to send message: %w", err)}
		}

		// Resume the worker in the pool so events are processed
		if err := workerPool.ResumeWorker(workerID, proc); err != nil {
			return WorkerErrorMsg{WorkerID: workerID, Error: fmt.Errorf("failed to resume worker: %w", err)}
		}

		log.Debug(log.CatOrch, "Message sent to worker", "subsystem", "update", "workerID", workerID)
		return nil
	}
}

// handleUserInputBroadcast sends user input to the coordinator and all active workers.
func (m Model) handleUserInputBroadcast(content string) (Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Send to coordinator (the coordinator will emit an event that adds the message to chat)
	if m.coord != nil {
		coordCmd := func() tea.Msg {
			if err := m.coord.SendUserMessage("[BROADCAST]\n" + content); err != nil {
				return CoordinatorErrorMsg{Error: err}
			}
			return nil
		}
		cmds = append(cmds, coordCmd)
	}

	// Send to all active workers
	if m.pool != nil {
		for _, workerID := range m.workerPane.workerIDs {
			status := m.workerPane.workerStatus[workerID]
			// Only send to ready or working workers (not retired)
			if status == pool.WorkerRetired {
				continue
			}

			// Get worker and validate
			worker := m.pool.GetWorker(workerID)
			if worker == nil {
				continue // Skip if worker not found
			}
			sessionID := worker.GetSessionID()
			if sessionID == "" {
				continue // Skip if no session yet
			}

			// Add message to worker pane
			m = m.AddWorkerMessageWithRole(workerID, "user", "[BROADCAST] "+content)

			// Build spawn config using shared helper
			cfg, err := m.buildWorkerSpawnConfig(workerID, sessionID, fmt.Sprintf("[BROADCAST FROM USER]\n%s", content))
			if err != nil {
				log.Debug(log.CatOrch, "Failed to build worker config for broadcast", "subsystem", "update", "workerID", workerID, "error", err)
				continue
			}

			// Capture for closure - client accessed via pool.Client()
			wid := workerID
			workerPool := m.pool
			spawnCfg := cfg

			workerCmd := func() tea.Msg {
				proc, err := workerPool.Client().Spawn(context.Background(), spawnCfg)
				if err != nil {
					return WorkerErrorMsg{WorkerID: wid, Error: err}
				}

				if err := workerPool.ResumeWorker(wid, proc); err != nil {
					return WorkerErrorMsg{WorkerID: wid, Error: err}
				}

				return nil
			}
			cmds = append(cmds, workerCmd)
		}
	}

	log.Debug(log.CatOrch, "Broadcast message sent", "subsystem", "update", "targets", len(cmds))
	return m, tea.Batch(cmds...)
}

// generateWorkerMCPConfig returns the appropriate MCP config format based on client type.
func (m Model) generateWorkerMCPConfig(workerID string) (string, error) {
	if client.ClientType(m.clientType) == client.ClientAmp {
		return mcp.GenerateWorkerConfigAmp(m.mcpPort, workerID)
	}
	return mcp.GenerateWorkerConfigHTTP(m.mcpPort, workerID)
}

// buildWorkerSpawnConfig creates a client.Config for resuming a worker session.
// This consolidates the common config-building logic used when sending messages to workers.
func (m Model) buildWorkerSpawnConfig(workerID, sessionID, prompt string) (client.Config, error) {
	mcpConfig, err := m.generateWorkerMCPConfig(workerID)
	if err != nil {
		return client.Config{}, fmt.Errorf("failed to generate MCP config: %w", err)
	}

	return client.Config{
		WorkDir:         m.workDir,
		SessionID:       sessionID,
		Prompt:          prompt,
		MCPConfig:       mcpConfig,
		SkipPermissions: true,
		DisallowedTools: []string{"AskUserQuestion"},
	}, nil
}

// handlePauseToggle toggles the paused state.
func (m Model) handlePauseToggle() (Model, tea.Cmd) {
	if m.coord == nil {
		return m, nil
	}

	if m.paused {
		if err := m.coord.Resume(); err != nil {
			m = m.SetError(err.Error())
			return m, nil
		}
		m.paused = false
	} else {
		if err := m.coord.Pause(); err != nil {
			m = m.SetError(err.Error())
			return m, nil
		}
		m.paused = true
	}

	return m, nil
}

// handleReplaceCoordinator requests a handoff from the coordinator before replacing.
// Sets pendingRefresh flag and sends a message to the coordinator asking it to post
// a handoff message via prepare_handoff. The actual Replace() is triggered when the
// handoff message is received in handleMessageEvent.
// Also starts a 5-second timeout to handle cases where coordinator doesn't respond.
func (m Model) handleReplaceCoordinator() (Model, tea.Cmd) {
	if m.coord == nil {
		m = m.SetError("Coordinator not started")
		return m, nil
	}

	// Set pending refresh flag
	m.pendingRefresh = true

	// Start timeout timer
	timeoutCmd := tea.Tick(15*time.Second, func(time.Time) tea.Msg {
		return RefreshTimeoutMsg{}
	})

	// Send message to coordinator to post handoff
	coord := m.coord
	sendCmd := func() tea.Msg {
		handoffMessage := `[CONTEXT REFRESH INITIATED]

Your context window is approaching limits. The user has initiated a coordinator refresh (Ctrl+R).

WHAT'S ABOUT TO HAPPEN:
- You will be replaced with a fresh coordinator session
- All workers will continue running (their state is preserved)
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

		err := coord.SendUserMessage(handoffMessage)
		if err != nil {
			return CoordinatorErrorMsg{Error: fmt.Errorf("failed to request handoff: %w", err)}
		}
		return nil
	}

	return m, tea.Batch(sendCmd, timeoutCmd)
}

// updateStatusFromCoordinator updates model state based on coordinator status.
func (m Model) updateStatusFromCoordinator(status coordinator.Status) Model {
	switch status {
	case coordinator.StatusRunning:
		m.paused = false
	case coordinator.StatusPaused:
		m.paused = true
	}
	return m
}

// workerServerCache manages worker MCP servers that share the same message store.
// Workers connect via HTTP to /worker/{workerID} and all share the coordinator's
// message issue instance, solving the in-memory cache isolation problem.
type workerServerCache struct {
	msgIssue      *message.Issue
	stateCallback mcp.WorkerStateCallback
	servers       map[string]*mcp.WorkerServer
	mu            sync.RWMutex
}

// newWorkerServerCache creates a new worker server cache.
// The stateCallback allows workers to update coordinator state when they report
// implementation complete or review verdicts.
func newWorkerServerCache(msgIssue *message.Issue, stateCallback mcp.WorkerStateCallback) *workerServerCache {
	return &workerServerCache{
		msgIssue:      msgIssue,
		stateCallback: stateCallback,
		servers:       make(map[string]*mcp.WorkerServer),
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

	ws = mcp.NewWorkerServer(workerID, c.msgIssue)
	// Set the state callback so workers can update coordinator state
	if c.stateCallback != nil {
		ws.SetStateCallback(c.stateCallback)
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

	// Check if there's an error modal showing (indicates error state)
	if m.errorModal != nil {
		return session.StatusFailed
	}

	// Default to completed (normal shutdown, user interrupt, etc.)
	return session.StatusCompleted
}
