package diffviewer

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/zjrosen/perles/internal/ui/styles"
)

// fileStatus represents the type of change to a file.
type fileStatus int

const (
	statusModified fileStatus = iota
	statusAdded
	statusDeleted
	statusRenamed
	statusBinary
	statusUntracked
	statusUnknown
)

// getFileStatus determines the status of a DiffFile.
func getFileStatus(f *DiffFile) fileStatus {
	if f == nil {
		return statusUnknown
	}
	switch {
	case f.IsBinary:
		return statusBinary
	case f.IsRenamed:
		return statusRenamed
	case f.IsUntracked:
		return statusUntracked
	case f.IsNew:
		return statusAdded
	case f.IsDeleted:
		return statusDeleted
	default:
		return statusModified
	}
}

// getStatusIndicatorStyle returns the style for a file status indicator.
func getStatusIndicatorStyle(status fileStatus) lipgloss.Style {
	switch status {
	case statusBinary:
		return lipgloss.NewStyle().Foreground(styles.TextMutedColor)
	case statusRenamed:
		return lipgloss.NewStyle().Foreground(styles.DiffHunkColor)
	case statusUntracked:
		return lipgloss.NewStyle().Foreground(styles.StatusWarningColor)
	case statusAdded:
		return lipgloss.NewStyle().Foreground(styles.DiffAdditionColor)
	case statusDeleted:
		return lipgloss.NewStyle().Foreground(styles.DiffDeletionColor)
	case statusModified:
		return lipgloss.NewStyle().Foreground(styles.DiffHunkColor)
	default:
		return lipgloss.NewStyle()
	}
}

// renderCenteredPlaceholder renders a centered message placeholder.
// When height=0, just returns the message without centering.
func renderCenteredPlaceholder(width, height int, msg string, color lipgloss.AdaptiveColor) string {
	if height == 0 {
		return msg
	}
	style := lipgloss.NewStyle().
		Foreground(color).
		Width(width).
		Height(height).
		Align(lipgloss.Center, lipgloss.Center)

	return style.Render(msg)
}
