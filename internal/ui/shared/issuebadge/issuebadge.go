// Package issuebadge provides functions for rendering the issue type/priority/id/title badge pattern.
// This shared component eliminates duplication across board, search, and tree views.
package issuebadge

import (
	"fmt"
	"strings"

	"github.com/zjrosen/perles/internal/beads"
	"github.com/zjrosen/perles/internal/ui/styles"

	"github.com/charmbracelet/lipgloss"
)

// Config configures the rendering of issue badges.
type Config struct {
	// ShowSelection enables the selection indicator ("> " prefix when selected, "  " when not).
	ShowSelection bool

	// MaxWidth is the maximum width for the entire rendered line (0 = no limit).
	// When set, the title will be truncated to fit.
	MaxWidth int

	// Selected indicates whether this item is currently selected.
	// Only has effect when ShowSelection is true.
	Selected bool
}

// RenderBadge returns the issue badge without the title: [T][Pn][id]
// When the issue is pinned, prepends 📌: 📌[T][Pn][id]
// This is useful when callers need to add their own metadata after the badge.
func RenderBadge(issue beads.Issue) string {
	priorityText := fmt.Sprintf("[P%d]", issue.Priority)
	typeText := styles.GetTypeIndicator(issue.Type)
	issueID := fmt.Sprintf("[%s]", issue.ID)

	priorityStyle := styles.GetPriorityStyle(issue.Priority)
	typeStyle := styles.GetTypeStyle(issue.Type)
	issueIDStyle := lipgloss.NewStyle().Foreground(styles.TextSecondaryColor)

	var parts []string

	// Add pin indicator if issue is pinned
	if issue.Pinned != nil && *issue.Pinned {
		parts = append(parts, "📌")
	}

	parts = append(parts,
		typeStyle.Render(typeText),
		priorityStyle.Render(priorityText),
		issueIDStyle.Render(issueID),
	)

	return strings.Join(parts, "")
}

// Render returns the full issue line with badge and title.
// Format: [selection][T][Pn][id] title
func Render(issue beads.Issue, cfg Config) string {
	badge := RenderBadge(issue)
	title := issue.TitleText

	// Build the line parts
	var parts []string

	// Selection indicator
	if cfg.ShowSelection {
		if cfg.Selected {
			parts = append(parts, styles.SelectionIndicatorStyle.Render(">"))
		} else {
			parts = append(parts, " ")
		}
	}

	parts = append(parts, badge)

	// Calculate available width for title
	if cfg.MaxWidth > 0 {
		// Calculate the width used by prefix + badge + space before title
		prefixWidth := 0
		for _, p := range parts {
			prefixWidth += lipgloss.Width(p)
		}
		// Account for space before title
		prefixWidth += 1

		availableWidth := cfg.MaxWidth - prefixWidth
		if availableWidth > 0 {
			title = styles.TruncateString(title, availableWidth)
		} else {
			title = ""
		}
	}

	if title != "" {
		parts = append(parts, " "+title)
	}

	return strings.Join(parts, "")
}
