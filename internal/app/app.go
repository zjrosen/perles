// Package app contains the root application model.
package app

import (
	"perles/internal/beads"
	"perles/internal/bql"
	"perles/internal/config"
	"perles/internal/mode"
	"perles/internal/mode/kanban"
	"perles/internal/mode/search"
	"perles/internal/watcher"

	tea "github.com/charmbracelet/bubbletea"
)

// DBChangedMsg signals that the database has changed.
// This message is sent by the app-level watcher and routed to the active mode.
type DBChangedMsg struct{}

// Model is the root application state.
type Model struct {
	// Mode management
	currentMode mode.AppMode
	kanban      kanban.Model
	search      search.Model

	// Shared services (passed to mode controllers)
	services mode.Services

	// Global state
	width  int
	height int

	// File watcher for auto-refresh
	dbWatcher     <-chan struct{}
	watcherHandle *watcher.Watcher
}

// New creates a new application model with default settings.
func New(client *beads.Client) Model {
	return NewWithConfig(client, config.Defaults(), "", "")
}

// NewWithConfig creates a new application model with the provided configuration.
// dbPath is the path to the database file for watching changes.
// configPath is the path to the config file for saving column changes.
func NewWithConfig(client *beads.Client, cfg config.Config, dbPath, configPath string) Model {
	// Create BQL executor for column self-loading
	executor := bql.NewExecutor(client.DB())

	// Initialize file watcher if auto-refresh is enabled
	var dbWatcher <-chan struct{}
	var watcherHandle *watcher.Watcher
	if cfg.AutoRefresh && dbPath != "" {
		w, err := watcher.New(watcher.Config{
			DBPath:      dbPath,
			DebounceDur: cfg.AutoRefreshDebounce,
		})
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
		Executor:   executor,
		Config:     &cfg,
		ConfigPath: configPath,
		DBPath:     dbPath,
	}

	return Model{
		currentMode:   mode.ModeKanban,
		kanban:        kanban.New(services),
		search:        search.New(services),
		services:      services,
		dbWatcher:     dbWatcher,
		watcherHandle: watcherHandle,
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	// Initialize the default mode (kanban) and start the database watcher
	return tea.Batch(m.kanban.Init(), m.watchDatabase())
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Pass size to both modes
		m.kanban = m.kanban.SetSize(msg.Width, msg.Height)
		m.search = m.search.SetSize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		// Global mode switching (Ctrl+Space, which is ctrl+@ in terminals)
		if msg.String() == "ctrl+@" {
			return m.switchMode()
		}

		// Global quit is handled by kanban mode (ctrl+c, q)
		// Fall through to delegate to active mode

	case kanban.SwitchToSearchMsg:
		// Switch to search mode with pre-populated query
		m.currentMode = mode.ModeSearch
		m.search = m.search.SetQuery(msg.Query)
		return m, m.search.Init()

	case search.SaveSearchAsColumnMsg:
		return m.handleSaveSearchAsColumn(msg)

	case search.SaveSearchToNewViewMsg:
		return m.handleSaveSearchToNewView(msg)

	case DBChangedMsg:
		// Route to active mode and re-subscribe
		var modeCmd tea.Cmd
		switch m.currentMode {
		case mode.ModeKanban:
			m.kanban, modeCmd = m.kanban.HandleDBChanged()
		case mode.ModeSearch:
			m.search, modeCmd = m.search.HandleDBChanged()
		}
		return m, tea.Batch(modeCmd, m.watchDatabase())
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
	}

	return m, nil
}

// switchMode toggles between Kanban and Search modes.
func (m Model) switchMode() (tea.Model, tea.Cmd) {
	switch m.currentMode {
	case mode.ModeKanban:
		m.currentMode = mode.ModeSearch
		return m, m.search.Init()
	case mode.ModeSearch:
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

// View implements tea.Model.
func (m Model) View() string {
	switch m.currentMode {
	case mode.ModeSearch:
		return m.search.View()
	default:
		return m.kanban.View()
	}
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
