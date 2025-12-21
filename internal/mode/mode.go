// Package mode defines the mode controller interface and shared services.
package mode

import (
	tea "github.com/charmbracelet/bubbletea"

	"perles/internal/beads"
	"perles/internal/bql"
	"perles/internal/config"
	"perles/internal/mode/shared"
	"perles/internal/ui/shared/toaster"
)

// AppMode identifies the current application mode.
type AppMode int

const (
	ModeKanban AppMode = iota
	ModeSearch
)

// SubMode represents the two rendering modes within search.
type SubMode int

const (
	SubModeList SubMode = iota // BQL query with flat results
	SubModeTree                // Issue ID with tree rendering
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
	Client     beads.BeadsClient
	Executor   bql.BQLExecutor
	Config     *config.Config
	ConfigPath string
	DBPath     string
	Clipboard  shared.Clipboard
	Clock      shared.Clock
}

// ShowToastMsg requests displaying a toast notification.
// Modes return this message instead of managing toasters directly.
// The app handles this message and manages the centralized toaster.
type ShowToastMsg struct {
	Message string
	Style   toaster.Style
}
