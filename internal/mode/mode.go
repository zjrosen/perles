// Package mode defines the mode controller interface and shared services.
package mode

import (
	tea "github.com/charmbracelet/bubbletea"

	appbeads "github.com/zjrosen/perles/internal/beads/application"
	"github.com/zjrosen/perles/internal/bql"
	"github.com/zjrosen/perles/internal/config"
	"github.com/zjrosen/perles/internal/flags"
	appgit "github.com/zjrosen/perles/internal/git/application"
	"github.com/zjrosen/perles/internal/mode/shared"
	"github.com/zjrosen/perles/internal/sound"
	"github.com/zjrosen/perles/internal/ui/shared/toaster"
)

// AppMode identifies the current application mode.
type AppMode int

const (
	ModeKanban AppMode = iota
	ModeSearch
	ModeDashboard
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

// BeadsClient combines version and comment reading for mode controllers.
type BeadsClient interface {
	appbeads.VersionReader
	appbeads.CommentReader
}

// Services contains shared dependencies injected into mode controllers.
type Services struct {
	Client        BeadsClient
	Executor      bql.BQLExecutor
	BeadsExecutor appbeads.IssueExecutor // Executor for BD CLI commands (with proper BEADS_DIR)
	Config        *config.Config
	ConfigPath    string
	DBPath        string
	WorkDir       string // Application root directory (where perles was invoked)
	Clipboard     shared.Clipboard
	Clock         shared.Clock
	Flags         *flags.Registry
	Sounds        sound.SoundService
	// GitExecutorFactory creates git executors for a given path.
	// Used by orchestration mode to check uncommitted changes in worktrees.
	GitExecutorFactory func(path string) appgit.GitExecutor
}

// ShowToastMsg requests displaying a toast notification.
// Modes return this message instead of managing toasters directly.
// The app handles this message and manages the centralized toaster.
type ShowToastMsg struct {
	Message string
	Style   toaster.Style
}

// RequestQuitMsg requests showing the quit confirmation modal.
// Modes bubble this up instead of handling quit directly, allowing the app
// to manage a centralized quit modal with consistent behavior.
type RequestQuitMsg struct{}
