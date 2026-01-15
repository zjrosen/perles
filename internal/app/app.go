// Package app contains the root application model.
package app

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zjrosen/perles/internal/beads"
	"github.com/zjrosen/perles/internal/bql"
	"github.com/zjrosen/perles/internal/cachemanager"
	"github.com/zjrosen/perles/internal/config"
	"github.com/zjrosen/perles/internal/flags"
	"github.com/zjrosen/perles/internal/git"
	"github.com/zjrosen/perles/internal/keys"
	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/mode"
	"github.com/zjrosen/perles/internal/mode/kanban"
	"github.com/zjrosen/perles/internal/mode/orchestration"
	"github.com/zjrosen/perles/internal/mode/search"
	"github.com/zjrosen/perles/internal/mode/shared"
	v2 "github.com/zjrosen/perles/internal/orchestration/v2"
	"github.com/zjrosen/perles/internal/orchestration/workflow"
	"github.com/zjrosen/perles/internal/pubsub"
	"github.com/zjrosen/perles/internal/sound"

	"github.com/zjrosen/perles/internal/ui/shared/chatpanel"
	"github.com/zjrosen/perles/internal/ui/shared/diffviewer"
	"github.com/zjrosen/perles/internal/ui/shared/logoverlay"
	"github.com/zjrosen/perles/internal/ui/shared/quitmodal"
	"github.com/zjrosen/perles/internal/ui/shared/toaster"
	"github.com/zjrosen/perles/internal/ui/shared/vimtextarea"
	"github.com/zjrosen/perles/internal/ui/styles"
	"github.com/zjrosen/perles/internal/watcher"
)

// Model is the root application state.
type Model struct {
	// Mode management
	currentMode   mode.AppMode
	kanban        kanban.Model
	search        search.Model
	orchestration orchestration.Model

	// Shared services (passed to mode controllers)
	services mode.Services

	// Global state
	width  int
	height int

	// Centralized toaster - owned by app, not individual modes
	toaster toaster.Model

	debugMode    bool
	logOverlay   logoverlay.Model
	logListenCmd tea.Cmd

	// Diff viewer overlay
	diffViewer diffviewer.Model

	// Chat panel for Kanban/Search modes (excluded from orchestration)
	chatPanel        chatpanel.Model
	chatPanelFocused bool
	chatInfra        *v2.SimpleInfrastructure

	// Cache Managers
	bqlCache      cachemanager.CacheManager[string, []beads.Issue]
	depGraphCache cachemanager.CacheManager[string, *bql.DependencyGraph]

	// File watcher for auto-refresh (pubsub-based)
	watcherHandle   *watcher.Watcher
	watcherCtx      context.Context
	watcherCancel   context.CancelFunc
	watcherListener *pubsub.ContinuousListener[watcher.WatcherEvent]

	// Quit confirmation modal (for chat panel Ctrl+C)
	quitModal quitmodal.Model

	// Workflow registry (shared between chat panel and orchestration mode)
	workflowRegistry *workflow.Registry
}

// NewWithConfig creates a new application model with the provided configuration.
// dbPath is the path to the database file for watching changes.
// configPath is the path to the config file for saving column changes.
// debugMode enables the log overlay (Ctrl+X toggle).
func NewWithConfig(
	client *beads.Client,
	cfg config.Config,
	bqlCache cachemanager.CacheManager[string, []beads.Issue],
	depGraphCache cachemanager.CacheManager[string, *bql.DependencyGraph],
	dbPath,
	configPath,
	workDir string,
	debugMode bool,
) Model {
	// Initialize file watcher if auto-refresh is enabled
	var (
		watcherHandle   *watcher.Watcher
		watcherCtx      context.Context
		watcherCancel   context.CancelFunc
		watcherListener *pubsub.ContinuousListener[watcher.WatcherEvent]
	)

	if cfg.AutoRefresh && dbPath != "" {
		w, err := watcher.New(watcher.DefaultConfig(dbPath))
		if err == nil {
			if err := w.Start(); err == nil {
				watcherHandle = w
				watcherCtx, watcherCancel = context.WithCancel(context.Background())
				watcherListener = pubsub.NewContinuousListener(watcherCtx, w.Broker())
			} else {
				// Cleanup on start failure
				_ = w.Stop()
			}
		}
		// Silently ignore watcher init errors - app works fine without auto-refresh
	}

	// Apply theme colors from config
	themeCfg := styles.ThemeConfig{
		Preset: cfg.Theme.Preset,
		Mode:   cfg.Theme.Mode,
		Colors: cfg.Theme.FlattenedColors(),
	}
	_ = styles.ApplyTheme(themeCfg)

	flagService := flags.New(cfg.Flags)

	// Create shared services
	services := mode.Services{
		Client:     client,
		Config:     &cfg,
		ConfigPath: configPath,
		DBPath:     dbPath,
		WorkDir:    workDir,
		Executor:   bql.NewExecutor(client.DB(), bqlCache, depGraphCache),
		Clipboard:  shared.SystemClipboard{},
		Clock:      shared.RealClock{},
		Flags:      flagService,
		Sounds:     sound.NewSystemSoundService(flagService, cfg.Sound.EnabledSounds),
		GitExecutorFactory: func(path string) git.GitExecutor {
			return git.NewRealExecutor(path)
		},
	}

	// Create log overlay and start listening if debug mode is enabled
	overlay := logoverlay.New()
	var logListenCmd tea.Cmd
	if debugMode {
		logListenCmd = overlay.StartListening()
	}

	// Create diff viewer with git executor factory for workDir
	// Uses factory pattern to enable worktree switching
	dv := diffviewer.NewWithGitExecutorFactory(
		func(path string) git.GitExecutor {
			return git.NewRealExecutor(path)
		},
		workDir,
	).SetClipboard(services.Clipboard)

	// Ensure user workflow directory exists and load workflow registry
	// Registry is shared between chat panel and orchestration mode
	_, _ = workflow.EnsureUserWorkflowDir() // Ignore errors, directory creation is best-effort
	workflowRegistry, err := workflow.NewRegistryWithConfig(cfg.Orchestration)
	if err != nil {
		log.Warn(log.CatMode, "Failed to load workflow registry", "error", err)
		// Continue without workflows - not a fatal error
	}

	// Create chat panel with config from services
	// Panel defaults to hidden (visible = false)
	chatPanelCfg := chatpanel.Config{
		ClientType:       cfg.Orchestration.Client,
		WorkDir:          workDir,
		SessionTimeout:   chatpanel.DefaultConfig().SessionTimeout,
		WorkflowRegistry: workflowRegistry,
		VimMode:          cfg.UI.VimMode,
	}
	cp := chatpanel.New(chatPanelCfg)

	return Model{
		currentMode:      mode.ModeKanban,
		kanban:           kanban.New(services),
		search:           search.New(services),
		services:         services,
		bqlCache:         bqlCache,
		depGraphCache:    depGraphCache,
		logOverlay:       overlay,
		debugMode:        debugMode,
		logListenCmd:     logListenCmd,
		diffViewer:       dv,
		chatPanel:        cp,
		watcherHandle:    watcherHandle,
		watcherCtx:       watcherCtx,
		watcherCancel:    watcherCancel,
		watcherListener:  watcherListener,
		workflowRegistry: workflowRegistry,
		quitModal: quitmodal.New(quitmodal.Config{
			Title:   "Exit Application?",
			Message: "Are you sure you want to quit?",
		}),
	}
}

// Init implements tea.Model interface.
// Defaults the application to Kanban mode and starts the watcher listener
// if auto-refresh is enabled.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.kanban.Init(),
	}

	// Start watcher listener if available
	if m.watcherListener != nil {
		cmds = append(cmds, m.watcherListener.Listen())
	}

	if m.logListenCmd != nil {
		cmds = append(cmds, m.logListenCmd)
	}
	return tea.Batch(cmds...)
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle quit modal first when visible (captures all input)
	if m.quitModal.IsVisible() {
		var cmd tea.Cmd
		var result quitmodal.Result
		m.quitModal, cmd, result = m.quitModal.Update(msg)
		switch result {
		case quitmodal.ResultQuit:
			return m, tea.Quit
		case quitmodal.ResultCancel:
			return m, nil
		}
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Calculate main content width (reduced when chat panel is visible)
		mainWidth := msg.Width
		if m.chatPanel.Visible() && m.currentMode != mode.ModeOrchestration {
			mainWidth = msg.Width - m.chatPanelWidth()
		}

		m.kanban = m.kanban.SetSize(mainWidth, msg.Height)
		m.search = m.search.SetSize(mainWidth, msg.Height)
		m.orchestration = m.orchestration.SetSize(msg.Width, msg.Height)
		m.toaster = m.toaster.SetSize(msg.Width, msg.Height)
		m.logOverlay.SetSize(msg.Width, msg.Height)
		m.diffViewer = m.diffViewer.SetSize(msg.Width, msg.Height)
		m.chatPanel = m.chatPanel.SetSize(m.chatPanelWidth(), m.chatPanelHeight())
		m.quitModal.SetSize(msg.Width, msg.Height)

		// Auto-close chat panel if terminal resizes below minimum width
		if m.chatPanel.Visible() && msg.Width < MinChatPanelTerminalWidth {
			m.chatPanel.Cleanup()
			m.chatPanel = m.chatPanel.Toggle().Blur() // Set hidden and unfocused
			m.chatPanelFocused = false
			log.Info(log.CatMode, "Chat panel auto-closed due to terminal resize", "width", msg.Width)
			return m, func() tea.Msg {
				return mode.ShowToastMsg{
					Message: "Terminal too narrow for chat panel",
					Style:   toaster.StyleInfo,
				}
			}
		}

		return m, nil

	case tea.MouseMsg:
		// Route mouse events to log overlay when visible
		if m.logOverlay.Visible() {
			var cmd tea.Cmd
			m.logOverlay, cmd = m.logOverlay.Update(msg)
			return m, cmd
		}

		// Route mouse events to diff viewer when visible
		if m.diffViewer.Visible() {
			var cmd tea.Cmd
			m.diffViewer, cmd = m.diffViewer.Update(msg)
			return m, cmd
		}

		// Route mouse events to chat panel when visible
		if m.chatPanel.Visible() {
			var cmd tea.Cmd
			m.chatPanel, cmd = m.chatPanel.Update(msg)
			return m, cmd
		}

	case log.LogEvent:
		// Route to log overlay (handles accumulation and listening)
		var cmd tea.Cmd
		m.logOverlay, cmd = m.logOverlay.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		if m.debugMode && key.Matches(msg, keys.Component.Close) {
			m.logOverlay.Toggle()
			return m, nil
		}

		// If the debug log overlay is visible it takes precedence for updates
		if m.logOverlay.Visible() {
			var cmd tea.Cmd
			m.logOverlay, cmd = m.logOverlay.Update(msg)

			return m, cmd
		}

		// Handle Ctrl+W to toggle chat panel (not in orchestration mode)
		if key.Matches(msg, keys.App.ToggleChatPanel) && m.currentMode != mode.ModeOrchestration {
			return m.handleToggleChatPanel()
		}

		// Tab toggles focus between main view and chat panel when panel is visible
		if msg.Type == tea.KeyTab && m.chatPanel.Visible() && m.currentMode != mode.ModeOrchestration {
			if m.chatPanelFocused {
				m.chatPanel = m.chatPanel.Blur()
				m.chatPanelFocused = false
				m.kanban = m.kanban.SetBoardFocused(true)
			} else {
				m.chatPanel = m.chatPanel.Focus()
				m.chatPanelFocused = true
				m.kanban = m.kanban.SetBoardFocused(false)
			}
			return m, nil
		}

		// Chat panel focus routing - when focused, panel takes precedence for key events
		if m.chatPanelFocused && m.chatPanel.Visible() && m.currentMode != mode.ModeOrchestration {
			var cmd tea.Cmd
			m.chatPanel, cmd = m.chatPanel.Update(msg)
			return m, cmd
		}

		// Diff viewer takes precedence when visible
		if m.diffViewer.Visible() {
			var cmd tea.Cmd
			m.diffViewer, cmd = m.diffViewer.Update(msg)
			return m, cmd
		}

		// Handle global mode switching between Kanban and Search
		// (Ctrl+Space, which is ctrl+@ in terminals)
		if key.Matches(msg, keys.Kanban.SwitchMode) {
			return m.switchMode()
		}

	case kanban.SwitchToSearchMsg:
		m.currentMode = mode.ModeSearch
		log.Info(log.CatMode, "Switching mode", "from", "kanban", "to", "search", "subMode", msg.SubMode, "query", msg.Query, "issue", msg.IssueID)

		// Calculate main content width based on chatpanel state and set size
		// BEFORE emitting EnterMsg so search has correct dimensions when processing
		mainWidth := m.width
		if m.chatPanel.Visible() {
			mainWidth = m.width - m.chatPanelWidth()
		}
		m.search = m.search.SetSize(mainWidth, m.height)

		return m, func() tea.Msg {
			return search.EnterMsg{SubMode: msg.SubMode, Query: msg.Query, IssueID: msg.IssueID}
		}

	case search.ExitToKanbanMsg:
		// Switch back to kanban mode from search
		log.Info(log.CatMode, "Switching mode", "from", "search", "to", "kanban")
		m.currentMode = mode.ModeKanban

		// Calculate main content width based on chatpanel state and set size
		// BEFORE RefreshFromConfig() so kanban has correct dimensions before layout recalculation
		mainWidth := m.width
		if m.chatPanel.Visible() {
			mainWidth = m.width - m.chatPanelWidth()
		}
		m.kanban = m.kanban.SetSize(mainWidth, m.height)

		// Rebuild kanban from config to reflect any column changes made in search mode
		var cmd tea.Cmd
		m.kanban, cmd = m.kanban.RefreshFromConfig()
		return m, cmd

	case kanban.SwitchToOrchestrationMsg:
		// Log mode switch with resume status
		isResume := msg.ResumeSessionDir != ""
		if isResume {
			log.Info(log.CatMode, "Switching mode", "from", "kanban", "to", "orchestration", "resume", true, "sessionDir", msg.ResumeSessionDir)
		} else {
			log.Info(log.CatMode, "Switching mode", "from", "kanban", "to", "orchestration", "resume", false)
		}

		// Close chat panel if open to prevent "two AIs" confusion
		// Must call Cleanup() before entering orchestration to stop the AI process
		// and prevent goroutine leaks (context cancelled, processor drained)
		if m.chatPanel.Visible() {
			m.chatPanel.Cleanup()
			m.chatPanel = m.chatPanel.Toggle().Blur() // Set hidden and unfocused
			m.chatPanelFocused = false
			log.Info(log.CatMode, "Chat panel closed on orchestration entry")
		}

		m.currentMode = mode.ModeOrchestration

		// Get orchestration config from services.Config
		orchConfig := m.services.Config.Orchestration

		// Use shared workflow registry (created at app startup)
		m.orchestration = orchestration.New(orchestration.Config{
			Services:             m.services,
			WorkDir:              m.services.WorkDir,
			AgentProvider:        orchConfig.AgentProvider(),
			WorkflowRegistry:     m.workflowRegistry,
			VimMode:              m.services.Config.UI.VimMode,
			DebugMode:            m.debugMode,
			DisableWorktrees:     orchConfig.DisableWorktrees,
			TracingConfig:        orchConfig.Tracing,
			SessionStorageConfig: orchConfig.SessionStorage,
			ResumeSessionDir:     msg.ResumeSessionDir,
		}).SetSize(m.width, m.height)
		return m, m.orchestration.Init()

	case orchestration.QuitMsg:
		// Switch back to kanban mode from orchestration
		log.Info(log.CatMode, "Switching mode", "from", "orchestration", "to", "kanban")

		// Clean up all orchestration resources (processes, MCP server, subscriptions)
		m.orchestration.Cleanup()

		// Log exit message if present (worktree cleanup info)
		if exitMsg := m.orchestration.ExitMessage(); exitMsg != "" {
			log.Info(log.CatMode, "Orchestration exit", "message", exitMsg)
		}

		m.currentMode = mode.ModeKanban

		// Calculate main content width based on chatpanel state and set size
		// BEFORE RefreshFromConfig() so kanban has correct dimensions before layout recalculation
		// Note: Panel is always hidden after orchestration (closed on entry), but this is defensive
		mainWidth := m.width
		if m.chatPanel.Visible() {
			mainWidth = m.width - m.chatPanelWidth()
		}
		m.kanban = m.kanban.SetSize(mainWidth, m.height)

		// Refresh kanban to show any changes made during orchestration
		var cmd tea.Cmd
		m.kanban, cmd = m.kanban.RefreshFromConfig()
		return m, cmd

	case search.SaveSearchAsColumnMsg:
		return m.handleSaveSearchAsColumn(msg)

	case search.SaveSearchToNewViewMsg:
		return m.handleSaveSearchToNewView(msg)

	case search.SaveTreeToNewViewMsg:
		return m.handleSaveTreeToNewView(msg)

	case search.SaveTreeAsColumnMsg:
		return m.handleSaveTreeAsColumn(msg)

	case pubsub.Event[watcher.WatcherEvent]:
		switch msg.Payload.Type {
		case watcher.DBChanged:
			if err := m.bqlCache.Flush(context.Background()); err != nil {
				log.Warn(log.CatCache, "Failed to flush BQL cache on DB change", "error", err)
			}
			if err := m.depGraphCache.Flush(context.Background()); err != nil {
				log.Warn(log.CatCache, "Failed to flush dep graph cache on DB change", "error", err)
			}

			log.Debug(log.CatMode, "DB changed, refreshing active mode", "mode", m.currentMode)
			var modeCmd tea.Cmd
			switch m.currentMode {
			case mode.ModeKanban:
				m.kanban, modeCmd = m.kanban.HandleDBChanged()
			case mode.ModeSearch:
				m.search, modeCmd = m.search.HandleDBChanged()
			}
			return m, tea.Batch(modeCmd, m.watcherListener.Listen())

		case watcher.WatcherError:
			log.Warn(log.CatWatcher, "Watcher error received", "error", msg.Payload.Error)
			return m, m.watcherListener.Listen()
		}

		// Continue listening for unknown event types
		return m, m.watcherListener.Listen()

	// Forward vimtextarea.SubmitMsg to chatPanel for processing
	// This is emitted when user presses Enter in the chat input
	case vimtextarea.SubmitMsg:
		if m.chatPanelFocused && m.chatPanel.Visible() && m.currentMode != mode.ModeOrchestration {
			var cmd tea.Cmd
			m.chatPanel, cmd = m.chatPanel.Update(msg)
			return m, cmd
		}
		// Fall through to mode handler (orchestration mode handles its own SubmitMsg)

	// Forward chat panel pubsub events (from SimpleChatInfrastructure)
	// Always forward to chat panel to keep listener active (even when hidden).
	// This prevents the listener chain from breaking when the panel is toggled off.
	case pubsub.Event[any]:
		if m.chatPanel.HasInfrastructure() && m.currentMode != mode.ModeOrchestration {
			var cmd tea.Cmd
			m.chatPanel, cmd = m.chatPanel.Update(msg)
			// Don't return - let mode also process if needed (though modes won't
			// receive chatPanel's events since they're from different brokers)
			if cmd != nil {
				return m, cmd
			}
		}
		// Fall through to mode handler

	// Forward spinner tick to chat panel for loading animation
	case chatpanel.SpinnerTickMsg:
		if m.chatPanel.Visible() && m.currentMode != mode.ModeOrchestration {
			var cmd tea.Cmd
			m.chatPanel, cmd = m.chatPanel.Update(msg)
			return m, cmd
		}

	// Handle SendMessageMsg from chatPanel (user submitted a message)
	case chatpanel.SendMessageMsg:
		if m.chatInfra != nil && m.chatInfra.ProcessRegistry.Get(chatpanel.ChatPanelProcessID) != nil {
			sendCmd := m.chatPanel.SendMessage(msg.Content)
			if sendCmd != nil {
				return m, sendCmd
			}
		}
		return m, nil

	// Handle AssistantErrorMsg from chatPanel (infrastructure error)
	case chatpanel.AssistantErrorMsg:
		return m, func() tea.Msg {
			return mode.ShowToastMsg{
				Message: "Chat error: " + msg.Error.Error(),
				Style:   toaster.StyleError,
			}
		}

	// Handle RequestQuitMsg from chatPanel (user pressed Ctrl+C in normal mode)
	case chatpanel.RequestQuitMsg:
		m.quitModal.Show()
		return m, nil

	// Handle NewSessionRequestMsg from chatPanel (user pressed Ctrl+N)
	case chatpanel.NewSessionRequestMsg:
		return m.handleNewSessionRequest()

	// Handle RequestQuitMsg from kanban/search modes (user pressed Ctrl+C)
	case mode.RequestQuitMsg:
		m.quitModal.Show()
		return m, nil

	case mode.ShowToastMsg:
		m.toaster = m.toaster.Show(msg.Message, msg.Style)

		return m, toaster.ScheduleDismiss(3 * time.Second)

	case toaster.DismissMsg:
		m.toaster = m.toaster.Hide()

		return m, nil

	case logoverlay.CloseMsg:
		m.logOverlay.Hide()

		return m, nil

	case diffviewer.ShowDiffViewerMsg:
		var cmd tea.Cmd
		m.diffViewer, cmd = m.diffViewer.ShowAndLoad()
		m.diffViewer = m.diffViewer.SetSize(m.width, m.height)
		return m, cmd

	case diffviewer.HideDiffViewerMsg:
		m.diffViewer = m.diffViewer.Hide()
		return m, nil

	case diffviewer.CommitsLoadedMsg:
		var cmd tea.Cmd
		m.diffViewer, cmd = m.diffViewer.Update(msg)
		return m, cmd

	case diffviewer.WorkingDirDiffLoadedMsg:
		var cmd tea.Cmd
		m.diffViewer, cmd = m.diffViewer.Update(msg)
		return m, cmd

	case diffviewer.CommitFilesLoadedMsg:
		var cmd tea.Cmd
		m.diffViewer, cmd = m.diffViewer.Update(msg)
		return m, cmd

	case diffviewer.CommitPreviewLoadedMsg:
		var cmd tea.Cmd
		m.diffViewer, cmd = m.diffViewer.Update(msg)
		return m, cmd

	// Forward branch/worktree loaded messages to diffViewer
	case diffviewer.WorktreesLoadedMsg:
		var cmd tea.Cmd
		m.diffViewer, cmd = m.diffViewer.Update(msg)
		return m, cmd

	case diffviewer.BranchesLoadedMsg:
		var cmd tea.Cmd
		m.diffViewer, cmd = m.diffViewer.Update(msg)
		return m, cmd

	case diffviewer.CommitsForBranchLoadedMsg:
		var cmd tea.Cmd
		m.diffViewer, cmd = m.diffViewer.Update(msg)
		return m, cmd

	case diffviewer.HunkCopiedMsg:
		if msg.Err != nil {
			return m, func() tea.Msg {
				return mode.ShowToastMsg{Message: "Copy failed: " + msg.Err.Error(), Style: toaster.StyleError}
			}
		}
		return m, func() tea.Msg {
			return mode.ShowToastMsg{Message: fmt.Sprintf("Copied %d lines", msg.LineCount), Style: toaster.StyleSuccess}
		}

	case diffviewer.ViewModeConstrainedMsg:
		// User tried to switch to side-by-side but terminal is too narrow
		return m, func() tea.Msg {
			return mode.ShowToastMsg{
				Message: fmt.Sprintf("Terminal too narrow for side-by-side view (need %d cols, have %d)", msg.MinWidth, msg.CurrentWidth),
				Style:   toaster.StyleInfo,
			}
		}
	}

	// Delegate all messages to active mode controller
	switch m.currentMode {
	case mode.ModeKanban:
		var cmd tea.Cmd
		m.kanban, cmd = m.kanban.Update(msg)

		return m, cmd

	case mode.ModeSearch:
		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)

		return m, cmd

	case mode.ModeOrchestration:
		var cmd tea.Cmd
		m.orchestration, cmd = m.orchestration.Update(msg)

		return m, cmd
	}

	return m, nil
}

// switchMode toggles between Kanban and Search modes.
func (m Model) switchMode() (tea.Model, tea.Cmd) {
	// Calculate main content width based on chatpanel state
	mainWidth := m.width
	if m.chatPanel.Visible() {
		mainWidth = m.width - m.chatPanelWidth()
	}

	switch m.currentMode {
	case mode.ModeKanban:
		log.Info(log.CatMode, "Switching mode", "from", "kanban", "to", "search", "subMode", "list")
		m.currentMode = mode.ModeSearch
		m.search = m.search.SetSize(mainWidth, m.height)

		return m, func() tea.Msg {
			return search.EnterMsg{SubMode: mode.SubModeList, Query: ""}
		}
	case mode.ModeSearch:
		log.Info(log.CatMode, "Switching mode", "from", "search", "to", "kanban")
		m.currentMode = mode.ModeKanban
		m.kanban = m.kanban.SetSize(mainWidth, m.height)
		// Rebuild kanban from config to reflect any column changes made in search mode
		var cmd tea.Cmd
		m.kanban, cmd = m.kanban.RefreshFromConfig()
		return m, cmd
	}
	return m, nil
}

// handleSaveSearchAsColumn processes a save-search-as-column request.
func (m Model) handleSaveSearchAsColumn(msg search.SaveSearchAsColumnMsg) (tea.Model, tea.Cmd) {
	// Create new column config
	newCol := config.ColumnConfig{
		Name:  msg.ColumnName,
		Query: msg.Query,
		Color: msg.Color,
	}

	// Add column to each selected view
	for _, viewIdx := range msg.ViewIndices {
		// Persist to YAML
		err := config.InsertColumnInView(
			m.services.ConfigPath,
			viewIdx,
			0, // Insert at beginning
			newCol,
			m.services.Config.Views,
		)
		if err != nil {
			// Log error, continue with other views
			continue
		}

		// Update in-memory config
		cols := m.services.Config.Views[viewIdx].Columns
		cols = append([]config.ColumnConfig{newCol}, cols...)
		m.services.Config.SetColumnsForView(viewIdx, cols)
	}

	// Refresh kanban if we switch back to it (will pick up new columns)
	return m, nil
}

// handleSaveSearchToNewView processes a request to create a new view from search.
func (m Model) handleSaveSearchToNewView(msg search.SaveSearchToNewViewMsg) (tea.Model, tea.Cmd) {
	// Create the column config
	col := config.ColumnConfig{
		Name:  msg.ColumnName,
		Query: msg.Query,
		Color: msg.Color,
	}

	// Create the view config
	newView := config.ViewConfig{
		Name:    msg.ViewName,
		Columns: []config.ColumnConfig{col},
	}

	// Persist to YAML
	err := config.AddView(m.services.ConfigPath, newView, m.services.Config.Views)
	if err != nil {
		// Error already shown in search mode toast, just return
		return m, nil
	}

	// Update in-memory config
	m.services.Config.Views = append(m.services.Config.Views, newView)

	return m, nil
}

// handleSaveTreeToNewView creates a new view with a tree column.
func (m Model) handleSaveTreeToNewView(msg search.SaveTreeToNewViewMsg) (tea.Model, tea.Cmd) {
	// Create tree column config
	col := config.ColumnConfig{
		Name:     msg.ColumnName,
		Type:     "tree",
		IssueID:  msg.IssueID,
		TreeMode: msg.TreeMode,
		Color:    msg.Color,
	}

	// Create the view config
	newView := config.ViewConfig{
		Name:    msg.ViewName,
		Columns: []config.ColumnConfig{col},
	}

	// Persist to YAML
	err := config.AddView(m.services.ConfigPath, newView, m.services.Config.Views)
	if err != nil {
		// Error already shown in search mode toast, just return
		return m, nil
	}

	// Update in-memory config
	m.services.Config.Views = append(m.services.Config.Views, newView)

	return m, nil
}

// handleSaveTreeAsColumn adds a tree column to existing views.
func (m Model) handleSaveTreeAsColumn(msg search.SaveTreeAsColumnMsg) (tea.Model, tea.Cmd) {
	// Create tree column config
	col := config.ColumnConfig{
		Name:     msg.ColumnName,
		Type:     "tree",
		IssueID:  msg.IssueID,
		TreeMode: msg.TreeMode,
		Color:    msg.Color,
	}

	// Add column to each selected view
	for _, viewIdx := range msg.ViewIndices {
		// Persist to YAML
		err := config.InsertColumnInView(
			m.services.ConfigPath,
			viewIdx,
			0, // Insert at beginning
			col,
			m.services.Config.Views,
		)
		if err != nil {
			// Log error, continue with other views
			continue
		}

		// Update in-memory config
		cols := m.services.Config.Views[viewIdx].Columns
		cols = append([]config.ColumnConfig{col}, cols...)
		m.services.Config.SetColumnsForView(viewIdx, cols)
	}

	// Refresh kanban if we switch back to it (will pick up new columns)
	return m, nil
}

// chatPanelWidth returns the fixed width for the chat panel.
func (m Model) chatPanelWidth() int {
	return 50
}

// chatPanelHeight returns the height for the chat panel.
// Accounts for status bar when in Kanban mode with status bar visible.
func (m Model) chatPanelHeight() int {
	height := m.height
	// In Kanban mode, account for status bar if visible
	if m.currentMode == mode.ModeKanban && m.kanban.ShowStatusBar() {
		height-- // Status bar takes 1 line
	}
	return height
}

// MinChatPanelTerminalWidth is the minimum terminal width required to open the chat panel.
const MinChatPanelTerminalWidth = 100

// handleToggleChatPanel handles Ctrl+W to toggle the chat panel.
// If opening and terminal is too narrow, shows a toast instead.
// When toggling, also transfers focus to/from the panel.
// On first open, lazily creates SimpleChatInfrastructure and spawns the assistant.
func (m Model) handleToggleChatPanel() (tea.Model, tea.Cmd) {
	// If panel is currently hidden, we're trying to open it
	if !m.chatPanel.Visible() {
		// Check minimum width requirement
		if m.width < MinChatPanelTerminalWidth {
			return m, func() tea.Msg {
				return mode.ShowToastMsg{
					Message: fmt.Sprintf("Terminal too narrow for chat panel (need %d cols, have %d)", MinChatPanelTerminalWidth, m.width),
					Style:   toaster.StyleInfo,
				}
			}
		}

		var cmds []tea.Cmd

		// Lazily create infrastructure on first open
		if m.chatInfra == nil {
			m.services.Sounds.Play("greeting", "chat_welcome")

			// Get AgentProvider from config (includes model settings from user config)
			provider := m.services.Config.Orchestration.AgentProvider()

			// Create v2 SimpleInfrastructure with AgentProvider
			infra, err := v2.NewSimpleInfrastructure(v2.SimpleInfrastructureConfig{
				AgentProvider: provider,
				WorkDir:       m.chatPanel.Config().WorkDir,
				SystemPrompt:  chatpanel.BuildAssistantSystemPrompt(),
				InitialPrompt: chatpanel.BuildAssistantInitialPrompt(),
			})
			if err != nil {
				log.Warn(log.CatMode, "Failed to create chat infrastructure", "error", err)
				return m, func() tea.Msg {
					return mode.ShowToastMsg{
						Message: "Failed to create chat infrastructure: " + err.Error(),
						Style:   toaster.StyleError,
					}
				}
			}

			if err := infra.Start(); err != nil {
				log.Warn(log.CatMode, "Failed to start chat infrastructure", "error", err)
				return m, func() tea.Msg {
					return mode.ShowToastMsg{
						Message: "Failed to start chat assistant",
						Style:   toaster.StyleError,
					}
				}
			}
			m.chatInfra = infra
			m.chatPanel = m.chatPanel.SetInfrastructure(infra)
			cmds = append(cmds, m.chatPanel.InitListener())
			log.Info(log.CatMode, "Chat infrastructure created and started")
		}

		// Open panel and focus it
		m.chatPanel = m.chatPanel.Toggle().Focus()
		m.chatPanelFocused = true
		m.kanban = m.kanban.SetBoardFocused(false)

		// Resize the current mode to account for chat panel width
		mainWidth := m.width - m.chatPanelWidth()
		switch m.currentMode {
		case mode.ModeKanban:
			m.kanban = m.kanban.SetSize(mainWidth, m.height)
		case mode.ModeSearch:
			m.search = m.search.SetSize(mainWidth, m.height)
		}

		// Spawn assistant if not already spawned
		if m.chatInfra != nil && m.chatInfra.ProcessRegistry.Get(chatpanel.ChatPanelProcessID) == nil {
			var spawnCmd tea.Cmd
			m.chatPanel, spawnCmd = m.chatPanel.SpawnAssistant()
			if spawnCmd != nil {
				cmds = append(cmds, spawnCmd)
			}
			log.Info(log.CatMode, "Spawning chat assistant")
		} else {
			// Process exists but session may still be Pending - restart spinner if needed
			if spinnerCmd := m.chatPanel.StartSpinner(); spinnerCmd != nil {
				cmds = append(cmds, spinnerCmd)
			}
		}

		return m, tea.Batch(cmds...)
	}

	// Panel is visible, close it and unfocus
	m.chatPanel = m.chatPanel.Toggle().Blur()
	m.chatPanelFocused = false
	m.kanban = m.kanban.SetBoardFocused(true)

	// Resize the current mode back to full terminal width
	// This ensures the main content area expands to fill the space
	switch m.currentMode {
	case mode.ModeKanban:
		m.kanban = m.kanban.SetSize(m.width, m.height)
	case mode.ModeSearch:
		m.search = m.search.SetSize(m.width, m.height)
	}

	return m, nil
}

// handleNewSessionRequest handles Ctrl+N from the chat panel to create a new session.
// It generates a new session ID, creates the session in the chat panel,
// spawns a process for it, and returns a NewSessionCreatedMsg on success.
func (m Model) handleNewSessionRequest() (tea.Model, tea.Cmd) {
	// Require infrastructure to be set up
	if m.chatInfra == nil {
		return m, func() tea.Msg {
			return mode.ShowToastMsg{
				Message: "Chat panel not initialized",
				Style:   toaster.StyleError,
			}
		}
	}

	// Generate sequential session ID
	sessionID := m.chatPanel.NextSessionID()

	// Create the session in the chat panel
	var session *chatpanel.SessionData
	m.chatPanel, session = m.chatPanel.CreateSession(sessionID)

	// Use session ID as process ID for simplicity
	processID := sessionID

	// Update session's process ID and reverse lookup
	session.ProcessID = processID
	m.chatPanel = m.chatPanel.SetSessionProcessID(sessionID, processID)

	// Spawn process for the new session
	var spawnCmd tea.Cmd
	m.chatPanel, spawnCmd = m.chatPanel.SpawnAssistantForSession(processID)

	log.Info(log.CatMode, "Created new chat session", "sessionID", sessionID, "processID", processID)

	// Return spawn command batched with NewSessionCreatedMsg
	return m, tea.Batch(
		spawnCmd,
		func() tea.Msg {
			return chatpanel.NewSessionCreatedMsg{SessionID: sessionID}
		},
	)
}

// View implements tea.Model.
func (m Model) View() string {
	// Determine if chat panel should be shown (excluded from orchestration mode)
	showChatPanel := m.chatPanel.Visible() && m.currentMode != mode.ModeOrchestration

	var view string
	switch m.currentMode {
	case mode.ModeSearch:
		view = m.search.View()
	case mode.ModeOrchestration:
		view = m.orchestration.View()
	default:
		view = m.kanban.View()
	}

	// Compose main content with chat panel when visible
	if showChatPanel {
		panelWidth := m.chatPanelWidth()
		m.chatPanel = m.chatPanel.SetSize(panelWidth, m.chatPanelHeight())
		view = lipgloss.JoinHorizontal(lipgloss.Top, view, m.chatPanel.View())
	}

	// Overlay toaster on top of active mode's view
	if m.toaster.Visible() {
		view = m.toaster.Overlay(view, m.width, m.height)
	}

	// Overlay diff viewer when visible
	if m.diffViewer.Visible() {
		view = m.diffViewer.Overlay(view)
	}

	// Overlay log viewer on top (only in debug mode when visible)
	if m.debugMode && m.logOverlay.Visible() {
		view = m.logOverlay.Overlay(view)
	}

	// Overlay quit modal on top when visible
	if m.quitModal.IsVisible() {
		view = m.quitModal.Overlay(view)
	}

	return view
}

// Close releases resources held by the application.
func (m *Model) Close() error {
	m.logOverlay.StopListening()

	// Clean up orchestration mode resources if active
	// Cleanup is handled via CmdRetireProcess in model.cleanup()
	if m.currentMode == mode.ModeOrchestration {
		m.orchestration.CancelSubscriptions()
	}

	// Clean up chat panel infrastructure
	m.chatPanel.Cleanup()

	// Close mode controllers
	if err := m.kanban.Close(); err != nil {
		return err
	}

	// Cancel watcher subscription context (stops listener)
	if m.watcherCancel != nil {
		m.watcherCancel()
	}

	// Close watcher if we own it
	if m.watcherHandle != nil {
		if err := m.watcherHandle.Stop(); err != nil {
			return err
		}
	}

	return nil
}
