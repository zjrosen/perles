// Package playground provides a component showcase and theme token viewer.
package playground

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/zjrosen/perles/internal/ui/styles"
)

// renderSidebar renders the component list sidebar.
// The height parameter is reserved for future scrolling support.
func renderSidebar(demos []ComponentDemo, selectedIndex, width, _ int, focused bool) string {
	var sb strings.Builder
	pad := " " // 1 char padding

	// Styles
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.TextPrimaryColor)
	selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.SelectionIndicatorColor)
	normalStyle := lipgloss.NewStyle().Foreground(styles.TextSecondaryColor)

	// Focus indicator
	if focused {
		headerStyle = headerStyle.Foreground(styles.StatusInProgressColor)
	}

	// Header
	sb.WriteString(pad + headerStyle.Render("Components"))
	sb.WriteString("\n")
	sb.WriteString(pad + lipgloss.NewStyle().Foreground(styles.BorderDefaultColor).Render(strings.Repeat("─", width-4)))
	sb.WriteString("\n")

	// Component list
	for i, demo := range demos {
		var line string

		if i == selectedIndex {
			// Selected: show selection indicator
			indicator := styles.SelectionIndicatorStyle.Render("●")
			name := selectedStyle.Render(demo.Name)
			line = pad + indicator + " " + name
		} else {
			// Not selected
			name := normalStyle.Render(demo.Name)
			line = pad + "  " + name
		}

		sb.WriteString(line)
		sb.WriteString("\n")
	}

	return sb.String()
}
