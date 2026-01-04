package orchestration

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/zjrosen/perles/internal/ui/styles"
)

// Layout constants
const (
	// minHeightPerWorker is the minimum height for each worker pane in the stacked view.
	// Used by both rendering (worker_pane.go) and mouse routing (update.go) to ensure
	// consistent height calculations.
	minHeightPerWorker = 5

	// Pane width percentages for the three-column layout.
	// Used by view.go, update.go (mouse routing), and model.go (SetSize).
	leftPanePercent   = 35
	middlePanePercent = 32
	// rightPanePercent = 100 - leftPanePercent - middlePanePercent (calculated)
)

// Agent colors - consistent colors for each agent type across all panes
var (
	CoordinatorColor = lipgloss.AdaptiveColor{Light: "#179299", Dark: "#179299"}
	WorkerColor      = lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#43BF6D"}
	UserColor        = lipgloss.AdaptiveColor{Light: "#FB923C", Dark: "#FB923C"}
	SystemColor      = lipgloss.AdaptiveColor{Light: "#FF6B6B", Dark: "#FF8787"} // Red for system/enforcement messages
)

// TitleContextStyle is used for muted contextual info in pane titles (port numbers, task IDs, phases).
var TitleContextStyle = lipgloss.NewStyle().Foreground(styles.TextMutedColor)

// QueuedCountStyle is used for the queue count indicator in worker panes.
// Uses orange color to draw attention to pending queued messages.
var QueuedCountStyle = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#FFA500", Dark: "#FFB347"})
