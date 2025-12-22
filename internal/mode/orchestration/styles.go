package orchestration

import "github.com/charmbracelet/lipgloss"

// Agent colors - consistent colors for each agent type across all panes
var (
	CoordinatorColor = lipgloss.AdaptiveColor{Light: "#179299", Dark: "#179299"}
	WorkerColor      = lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#43BF6D"}
	UserColor        = lipgloss.AdaptiveColor{Light: "#FB923C", Dark: "#FB923C"}
)
