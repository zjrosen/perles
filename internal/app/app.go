// Package app contains the root application model.
package app

import (
	"context"
	"path/filepath"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"perles/internal/beads"
	"perles/internal/bql"
	"perles/internal/config"
	"perles/internal/keys"
	"perles/internal/log"
	"perles/internal/mode"
	"perles/internal/mode/kanban"
	"perles/internal/mode/orchestration"
	"perles/internal/mode/search"
	"perles/internal/mode/shared"

	"perles/internal/ui/shared/logoverlay"
	"perles/internal/ui/shared/toaster"
	"perles/internal/watcher"
)

// DBChangedMsg signals that the database has changed.
// This message is sent by the app-level watcher and routed to the active mode.
type DBChangedMsg struct{}

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

	// Log overlay for debug mode (Ctrl+X toggle)
	logOverlay logoverlay.Model
	debugMode  bool

	// File watcher for auto-refresh
	dbWatcher     <-chan struct{}
	watcherHandle *watcher.Watcher
}

// NewWithConfig creates a new application model with the provided configuration.
// dbPath is the path to the database file for watching changes.
// configPath is the path to the config file for saving column changes.
// debugMode enables the log overlay (Ctrl+X toggle).
func NewWithConfig(client *beads.Client, cfg config.Config, dbPath, configPath string, debugMode bool) Model {
	// Initialize file watcher if auto-refresh is enabled
	var dbWatcher <-chan struct{}
	var watcherHandle *watcher.Watcher
	if cfg.AutoRefresh && dbPath != "" {
		w, err := watcher.New(watcher.DefaultConfig(dbPath))
		if err == nil {
			onChange, err := w.Start()
			if err == nil {
				watcherHandle = w
				dbWatcher = onChange
			} else {
				// Cleanup on start failure
				_ = w.Stop()
			}
		}
		// Silently ignore watcher init errors - app works fine without auto-refresh
	}

	// Create shared services
	services := mode.Services{
		Client:     client,
		Config:     &cfg,
		ConfigPath: configPath,
		DBPath:     dbPath,
		Executor:   bql.NewExecutor(client.DB()),
		Clipboard:  shared.SystemClipboard{},
		Clock:      shared.RealClock{},
	}

	return Model{
		currentMode:   mode.ModeKanban,
		kanban:        kanban.New(services),
		search:        search.New(services),
		services:      services,
		logOverlay:    logoverlay.New(),
		debugMode:     debugMode,
		dbWatcher:     dbWatcher,
		watcherHandle: watcherHandle,
	}
}

// Init implements tea.Model interface.
// Defaults the application Kanan mode and creates a subscription
// to the beads database if we are watching for changes.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.kanban.Init(),
		m.watchDatabase(),
	)
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		m.kanban = m.kanban.SetSize(msg.Width, msg.Height)
		m.search = m.search.SetSize(msg.Width, msg.Height)
		m.orchestration = m.orchestration.SetSize(msg.Width, msg.Height)
		m.toaster = m.toaster.SetSize(msg.Width, msg.Height)
		m.logOverlay.SetSize(msg.Width, msg.Height)

		return m, nil

	case tea.KeyMsg:
		if m.debugMode && key.Matches(msg, keys.Component.Close) {
			m.logOverlay.Toggle()
			log.Debug(log.CatUI, "Log overlay toggled", "visible", m.logOverlay.Visible())

			return m, nil
		}

		// If the debug log overlay is visible it takes precedence for updates
		if m.logOverlay.Visible() {
			var cmd tea.Cmd
			m.logOverlay, cmd = m.logOverlay.Update(msg)

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
		return m, func() tea.Msg {
			return search.EnterMsg{SubMode: msg.SubMode, Query: msg.Query, IssueID: msg.IssueID}
		}

	case search.ExitToKanbanMsg:
		// Switch back to kanban mode from search
		log.Info(log.CatMode, "Switching mode", "from", "search", "to", "kanban")
		m.currentMode = mode.ModeKanban

		// Rebuild kanban from config to reflect any column changes made in search mode
		var cmd tea.Cmd
		m.kanban, cmd = m.kanban.RefreshFromConfig()
		return m, cmd

	case kanban.SwitchToOrchestrationMsg:
		log.Info(log.CatMode, "Switching mode", "from", "kanban", "to", "orchestration")
		m.currentMode = mode.ModeOrchestration

		// Work directory is the project root (parent of .beads)
		beadsDir := filepath.Dir(m.services.DBPath)
		workDir := filepath.Dir(beadsDir)

		// Get orchestration config from services.Config
		orchConfig := m.services.Config.Orchestration

		m.orchestration = orchestration.New(orchestration.Config{
			Services:    m.services,
			WorkDir:     workDir,
			ClientType:  orchConfig.Client,
			ClaudeModel: orchConfig.Claude.Model,
			AmpModel:    orchConfig.Amp.Model,
			AmpMode:     orchConfig.Amp.Mode,
		}).SetSize(m.width, m.height)
		return m, m.orchestration.Init()

	case orchestration.QuitMsg:
		// Switch back to kanban mode from orchestration
		log.Info(log.CatMode, "Switching mode", "from", "orchestration", "to", "kanban")

		// Cancel pub/sub subscriptions first
		m.orchestration.CancelSubscriptions()

		// Clean up coordinator and pool before switching modes
		if coord := m.orchestration.Coordinator(); coord != nil {
			log.Debug(log.CatMode, "Cancelling coordinator")
			_ = coord.Cancel() // Preserves state for resume
		}
		if pool := m.orchestration.Pool(); pool != nil {
			log.Debug(log.CatMode, "Closing worker pool")
			pool.Close()
		}
		// Shut down MCP server synchronously to free the port
		if srv := m.orchestration.MCPServer(); srv != nil {
			log.Debug(log.CatMode, "Shutting down MCP server")
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = srv.Shutdown(ctx)
			cancel()
		}

		m.currentMode = mode.ModeKanban
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

	case DBChangedMsg:
		// Route to active mode and re-subscribe
		log.Debug(log.CatMode, "DB changed, refreshing active mode", "mode", m.currentMode)
		var modeCmd tea.Cmd
		switch m.currentMode {
		case mode.ModeKanban:
			m.kanban, modeCmd = m.kanban.HandleDBChanged()
		case mode.ModeSearch:
			m.search, modeCmd = m.search.HandleDBChanged()
		}

		return m, tea.Batch(modeCmd, m.watchDatabase())

	case mode.ShowToastMsg:
		m.toaster = m.toaster.Show(msg.Message, msg.Style)

		return m, toaster.ScheduleDismiss(3 * time.Second)

	case toaster.DismissMsg:
		m.toaster = m.toaster.Hide()

		return m, nil

	case logoverlay.CloseMsg:
		m.logOverlay.Hide()

		return m, nil
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
	switch m.currentMode {
	case mode.ModeKanban:
		log.Info(log.CatMode, "Switching mode", "from", "kanban", "to", "search", "subMode", "list")
		m.currentMode = mode.ModeSearch

		return m, func() tea.Msg {
			return search.EnterMsg{SubMode: mode.SubModeList, Query: ""}
		}
	case mode.ModeSearch:
		log.Info(log.CatMode, "Switching mode", "from", "search", "to", "kanban")
		m.currentMode = mode.ModeKanban
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

// View implements tea.Model.
func (m Model) View() string {
	log.Debug(log.CatMode, "View: rendering mode", "current_mode", m.currentMode)

	var view string
	switch m.currentMode {
	case mode.ModeSearch:
		view = m.search.View()
	case mode.ModeOrchestration:
		view = m.orchestration.View()
	default:
		view = m.kanban.View()
	}

	// Overlay toaster on top of active mode's view
	if m.toaster.Visible() {
		view = m.toaster.Overlay(view, m.width, m.height)
	}

	// Overlay log viewer on top (only in debug mode when visible)
	if m.debugMode && m.logOverlay.Visible() {
		view = m.logOverlay.Overlay(view)
	}

	return view
}

// watchDatabase returns a command that waits for database changes.
func (m Model) watchDatabase() tea.Cmd {
	if m.dbWatcher == nil {
		return nil
	}
	return func() tea.Msg {
		<-m.dbWatcher
		return DBChangedMsg{}
	}
}

// Close releases resources held by the application.
func (m *Model) Close() error {
	// Clean up orchestration mode resources if active
	if m.currentMode == mode.ModeOrchestration {
		if coord := m.orchestration.Coordinator(); coord != nil {
			_ = coord.Cancel() // Preserves state for resume
		}
		if pool := m.orchestration.Pool(); pool != nil {
			pool.Close()
		}
	}

	// Close mode controllers
	if err := m.kanban.Close(); err != nil {
		return err
	}

	// Close watcher if we own it (kanban mode may also have one)
	if m.watcherHandle != nil {
		if err := m.watcherHandle.Stop(); err != nil {
			return err
		}
	}

	return nil
}
