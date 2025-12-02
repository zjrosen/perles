// Package mode defines the mode controller interface and shared services.
package mode

import (
	"perles/internal/beads"
	"perles/internal/bql"
	"perles/internal/config"

	tea "github.com/charmbracelet/bubbletea"
)

// AppMode identifies the current application mode.
type AppMode int

const (
	ModeKanban AppMode = iota
	ModeSearch
)

// Controller defines the interface all modes must implement.
type Controller interface {
	// Init returns initial commands for the mode.
	Init() tea.Cmd

	// Update handles messages and returns updated model and commands.
	Update(msg tea.Msg) (Controller, tea.Cmd)

	// View renders the mode's UI.
	View() string

	// SetSize handles terminal resize events.
	SetSize(width, height int) Controller
}

// Services contains shared dependencies injected into mode controllers.
type Services struct {
	Client     *beads.Client
	Executor   *bql.Executor
	Config     *config.Config
	ConfigPath string
	DBPath     string
}
