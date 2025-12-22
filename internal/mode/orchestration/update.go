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

	"perles/internal/log"
	"perles/internal/orchestration/amp"
	"perles/internal/orchestration/client"
	"perles/internal/orchestration/coordinator"
	"perles/internal/orchestration/events"
	"perles/internal/orchestration/mcp"
	"perles/internal/orchestration/message"
	"perles/internal/orchestration/pool"
	"perles/internal/ui/shared/modal"

	"perles/internal/pubsub"
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
			key.WithKeys("ctrl+p"),
			key.WithHelp("ctrl+p", "pause/resume"),
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

// Coordinator event messages

// StartCoordinatorMsg signals to start the coordinator.
type StartCoordinatorMsg struct{}

// CoordinatorStartedMsg indicates the coordinator started successfully.
type CoordinatorStartedMsg struct{}

// CoordinatorStartFailedMsg indicates the coordinator failed to start.
type CoordinatorStartFailedMsg struct {
	Error error
}

// CoordinatorStoppedMsg indicates the coordinator stopped.
type CoordinatorStoppedMsg struct{}

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
	// Handle modal cancel (dismisses critical error)
	case modal.CancelMsg:
		m = m.ClearError()
		return m, nil

	case tea.KeyMsg:
		// Forward key events to error modal when it's visible
		if m.errorModal != nil {
			var cmd tea.Cmd
			*m.errorModal, cmd = m.errorModal.Update(msg)
			return m, cmd
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
				return m, func() tea.Msg { return QuitMsg{} }
			}
			return m, nil
		}

		// Normal mode: input is focused, most keys go to input
		switch {
		case key.Matches(msg, keys.Enter):
			content := strings.TrimSpace(m.input.Value())
			if content != "" {
				target := m.messageTarget
				m.input.Reset()
				return m, func() tea.Msg {
					return UserInputMsg{Content: content, Target: target}
				}
			}
			return m, nil

		case key.Matches(msg, keys.Quit), msg.Type == tea.KeyCtrlC:
			return m, func() tea.Msg { return QuitMsg{} }

		case key.Matches(msg, keys.Pause):
			return m, func() tea.Msg { return PauseMsg{} }

		case key.Matches(msg, keys.Replace):
			return m, func() tea.Msg { return ReplaceCoordinatorMsg{} }

		case key.Matches(msg, keys.Tab):
			m = m.CycleMessageTarget()
			return m, nil
		}

		// Forward other keys to text input
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

		// Calculate pane boundaries based on layout (35%/32%/33%)
		leftWidth := m.width * 35 / 100
		middleWidth := m.width * 32 / 100
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
			// Filter to active workers (same logic as renderWorkerPanes)
			var activeWorkerIDs []string
			for _, workerID := range m.workerPane.workerIDs {
				status := m.workerPane.workerStatus[workerID]
				if status != pool.WorkerRetired {
					activeWorkerIDs = append(activeWorkerIDs, workerID)
				}
			}

			if len(activeWorkerIDs) > 0 {
				// Calculate height per worker pane (matches renderWorkerPanes)
				minHeightPerWorker := 5
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

	case CoordinatorStartedMsg:
		// Start listening for coordinator, worker, and message events via pub/sub
		// Each listener.Listen() returns a tea.Cmd that waits for the next event
		return m, tea.Batch(
			m.coordListener.Listen(),
			m.workerListener.Listen(),
			m.messageListener.Listen(),
		)

	case CoordinatorStartFailedMsg:
		m = m.SetError(msg.Error.Error())
		return m, nil

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
		// Shutdown HTTP MCP server
		if m.mcpServer != nil {
			go func() {
				if err := m.mcpServer.Shutdown(context.Background()); err != nil {
					log.Debug("orchestration", "MCP server shutdown error", "error", err)
				}
			}()
		}
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
		log.Debug("orchestration", "Worker error", "workerID", msg.WorkerID, "error", msg.Error)
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

	switch payload.Type {
	case events.CoordinatorChat:
		m = m.AddChatMessage(payload.Role, payload.Content)

	case events.CoordinatorStatusChange:
		m = m.updateStatusFromCoordinator(convertCoordinatorStatus(payload.Status))

	case events.CoordinatorError:
		m = m.SetError(payload.Error.Error())

	case events.CoordinatorTokenUsage:
		if payload.Metrics != nil {
			m.coordinatorMetrics = payload.Metrics
			log.Debug("orchestration", "Coordinator token usage updated",
				"contextTokens", payload.Metrics.ContextTokens,
				"contextWindow", payload.Metrics.ContextWindow,
				"totalCost", payload.Metrics.TotalCostUSD)
		}

	case events.CoordinatorWorking:
		m.coordinatorWorking = true

	case events.CoordinatorReady:
		m.coordinatorWorking = false
	}

	// Always continue listening for more events
	return m, m.coordListener.Listen()
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
		log.Debug("orchestration", "Worker spawned in TUI", "workerID", payload.WorkerID, "taskID", payload.TaskID, "status", payload.Status)
		m = m.UpdateWorker(payload.WorkerID, payload.Status)

	case events.WorkerOutput:
		log.Debug("orchestration", "Worker output received", "workerID", payload.WorkerID, "outputLen", len(payload.Output))
		if payload.Output != "" {
			m = m.AddWorkerMessage(payload.WorkerID, payload.Output)
		}

	case events.WorkerStatusChange:
		log.Debug("orchestration", "Worker status changed", "workerID", payload.WorkerID, "status", payload.Status)
		m = m.UpdateWorker(payload.WorkerID, payload.Status)

	case events.WorkerTokenUsage:
		if payload.Metrics != nil {
			log.Debug("orchestration", "Worker token usage",
				"workerID", payload.WorkerID,
				"contextTokens", payload.Metrics.ContextTokens,
				"contextWindow", payload.Metrics.ContextWindow,
				"totalCost", payload.Metrics.TotalCostUSD)
			m.workerPane.workerMetrics[payload.WorkerID] = payload.Metrics
		}

	case events.WorkerIncoming:
		log.Debug("orchestration", "Coordinator message to worker",
			"workerID", payload.WorkerID,
			"messageLen", len(payload.Message))
		m = m.AddWorkerMessageWithRole(payload.WorkerID, "coordinator", payload.Message)

	case events.WorkerError:
		log.Debug("orchestration", "Worker error", "workerID", payload.WorkerID, "error", payload.Error)
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

		// Nudge coordinator if message is to COORDINATOR or ALL (via debounced batcher)
		entry := payload.Entry
		if entry.To == message.ActorCoordinator || entry.To == message.ActorAll {
			if m.coord != nil && m.nudgeBatcher != nil && entry.From != message.ActorCoordinator {
				m.nudgeBatcher.Add(entry.From)
			}
		}
	}

	// Always continue listening for more events
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

// --- Handler methods ---

// handleStartCoordinator initializes the coordinator and starts it.
func (m Model) handleStartCoordinator() (Model, tea.Cmd) {
	if m.workDir == "" {
		m = m.SetError("Work directory not configured")
		return m, nil
	}

	// Determine client type from config, defaulting to "claude"
	clientType := client.ClientType(m.clientType)
	if clientType == "" {
		clientType = client.ClientClaude
	}

	// Create the AI client based on configuration
	aiClient, err := client.NewClient(clientType)
	if err != nil {
		return m, func() tea.Msg {
			return CoordinatorStartFailedMsg{Error: fmt.Errorf("failed to create AI client: %w", err)}
		}
	}

	// Build extensions map for provider-specific configuration
	extensions := make(map[string]any)
	switch clientType {
	case client.ClientClaude:
		if m.claudeModel != "" {
			extensions[client.ExtClaudeModel] = m.claudeModel
		}
	case client.ClientAmp:
		if m.ampModel != "" {
			extensions[client.ExtAmpModel] = m.ampModel
		}
		if m.ampMode != "" {
			extensions[amp.ExtAmpMode] = m.ampMode
		}
	}

	log.Debug("orchestration", "Creating AI client",
		"clientType", clientType,
		"claudeModel", m.claudeModel,
		"ampModel", m.ampModel,
		"ampMode", m.ampMode)

	// Create worker pool with the client
	workerPool := pool.NewWorkerPool(pool.Config{
		Client: aiClient,
	})
	m.pool = workerPool

	// Create in-memory message log
	msgIssue := message.New()
	m.messageLog = msgIssue

	// Shut down any existing MCP server before starting a new one
	if m.mcpServer != nil {
		log.Debug("orchestration", "Shutting down existing MCP server")
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = m.mcpServer.Shutdown(ctx)
		cancel()
		m.mcpServer = nil
	}

	// Start HTTP MCP server with shared pool, client, and provider extensions
	mcpServer := mcp.NewCoordinatorServer(aiClient, workerPool, msgIssue, m.workDir, extensions)

	// Create worker server cache that shares the same msgIssue
	workerServers := newWorkerServerCache(msgIssue)

	// Set up HTTP routes - coordinator and workers share the same server
	mux := http.NewServeMux()
	mux.Handle("/mcp", mcpServer.ServeHTTP())           // Coordinator tools
	mux.HandleFunc("/worker/", workerServers.ServeHTTP) // Worker tools (shared msgIssue)

	httpServer := &http.Server{
		Addr:              ":8765",
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	m.mcpServer = httpServer

	// Start HTTP server in background
	go func() {
		log.Debug("orchestration", "Starting MCP HTTP server", "addr", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Debug("orchestration", "MCP server error", "error", err)
		}
	}()

	// Create coordinator with the AI client
	coordCfg := coordinator.Config{
		WorkDir:      m.workDir,
		Client:       aiClient,
		Pool:         workerPool,
		MessageIssue: msgIssue,
	}

	coord, err := coordinator.New(coordCfg)
	if err != nil {
		return m, func() tea.Msg {
			return CoordinatorStartFailedMsg{Error: err}
		}
	}
	m.coord = coord

	// Set up nudge batcher for debouncing worker message notifications
	m.nudgeBatcher = NewNudgeBatcher(1 * time.Second)
	m.nudgeBatcher.SetOnNudge(func(workerIDs []string) {
		if m.coord == nil {
			return
		}

		var nudge string
		if len(workerIDs) == 1 {
			nudge = fmt.Sprintf("[%s sent a message] Use read_message_log to check for new messages.", workerIDs[0])
		} else {
			nudge = fmt.Sprintf("[%s sent messages] Use read_message_log to check for new messages.", strings.Join(workerIDs, ", "))
		}

		if err := m.coord.SendUserMessage(nudge); err != nil {
			log.Debug("orchestration", "Failed to nudge coordinator", "error", err)
		}
	})

	// Set up pub/sub subscriptions for coordinator, worker, and message events
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.coordListener = pubsub.NewContinuousListener(m.ctx, coord.Broker())
	m.workerListener = pubsub.NewContinuousListener(m.ctx, coord.Workers())
	m.messageListener = pubsub.NewContinuousListener(m.ctx, msgIssue.Broker())

	// Start the coordinator in background
	return m, func() tea.Msg {
		if err := coord.Start(); err != nil {
			return CoordinatorStartFailedMsg{Error: err}
		}
		return CoordinatorStartedMsg{}
	}
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
	log.Debug("orchestration", "Sending message to worker", "workerID", workerID)

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

		log.Debug("orchestration", "Message sent to worker", "workerID", workerID)
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
				log.Debug("orchestration", "Failed to build worker config for broadcast", "workerID", workerID, "error", err)
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

	log.Debug("orchestration", "Broadcast message sent", "targets", len(cmds))
	return m, tea.Batch(cmds...)
}

// generateWorkerMCPConfig returns the appropriate MCP config format based on client type.
func (m Model) generateWorkerMCPConfig(workerID string) (string, error) {
	if client.ClientType(m.clientType) == client.ClientAmp {
		return mcp.GenerateWorkerConfigAmp(8765, workerID)
	}
	return mcp.GenerateWorkerConfig(workerID, m.workDir)
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

// handleReplaceCoordinator replaces the coordinator process (hot swap).
// This preserves the session ID for Claude continuity while spawning a fresh process.
func (m Model) handleReplaceCoordinator() (Model, tea.Cmd) {
	if m.coord == nil {
		m = m.SetError("Coordinator not started")
		return m, nil
	}

	// Capture coordinator for closure
	coord := m.coord

	return m, func() tea.Msg {
		if err := coord.Replace(); err != nil {
			return CoordinatorErrorMsg{Error: err}
		}
		return nil
	}
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
	msgIssue *message.Issue
	servers  map[string]*mcp.WorkerServer
	mu       sync.RWMutex
}

// newWorkerServerCache creates a new worker server cache.
func newWorkerServerCache(msgIssue *message.Issue) *workerServerCache {
	return &workerServerCache{
		msgIssue: msgIssue,
		servers:  make(map[string]*mcp.WorkerServer),
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
	c.servers[workerID] = ws
	log.Debug("orchestration", "Created worker server", "workerID", workerID)
	return ws
}
