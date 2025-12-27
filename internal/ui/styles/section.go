package styles

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Border characters (rounded) - used by RenderFormSection.
// Note: These are also defined in ui/shared/panes/border.go for the pane components.
const (
	borderTopLeft     = "╭"
	borderTopRight    = "╮"
	borderBottomLeft  = "╰"
	borderBottomRight = "╯"
	borderHorizontal  = "─"
	borderVertical    = "│"
)

// RenderFormSection renders a bordered section with an optional title and hint.
// When focused is true, the border uses focusedBorderColor instead of BorderDefaultColor.
// This is the shared renderer for form components (coleditor, viewselector, labeleditor, modal).
func RenderFormSection(content []string, title, hint string, width int, focused bool, focusedBorderColor lipgloss.TerminalColor) string {
	// Border color: use focusedBorderColor when focused, BorderDefaultColor when not
	var borderColor lipgloss.TerminalColor = BorderDefaultColor
	var titleColor lipgloss.TerminalColor = BorderDefaultColor
	if focused {
		borderColor = focusedBorderColor
		titleColor = focusedBorderColor
	}

	borderStyle := lipgloss.NewStyle().Foreground(borderColor)
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(titleColor)
	hintStyle := lipgloss.NewStyle().Foreground(TextMutedColor)

	innerWidth := max(width-2, 1) // Account for left/right borders

	// Build top border with inline title: ╭─ Title (hint) ──────╮
	var topBorder string
	if title == "" {
		topBorder = borderStyle.Render(borderTopLeft + strings.Repeat(borderHorizontal, innerWidth) + borderTopRight)
	} else {
		// Calculate title text length for dash calculation
		titleLen := lipgloss.Width(title)
		if hint != "" {
			titleLen = lipgloss.Width(title + " (" + hint + ")")
		}
		dashesAfter := max(innerWidth-titleLen-3, 0) // -3 for "─ " before and " " after title

		// Build: ╭─ Title (hint) ──────╮
		topBorder = borderStyle.Render(borderTopLeft+borderHorizontal+" ") + titleStyle.Render(title)
		if hint != "" {
			topBorder += " " + hintStyle.Render("("+hint+")")
		}
		topBorder += borderStyle.Render(" " + strings.Repeat(borderHorizontal, dashesAfter) + borderTopRight)
	}

	// Build content lines with side borders
	var contentLines []string
	for _, row := range content {
		lineWidth := lipgloss.Width(row)
		padding := ""
		if lineWidth < innerWidth {
			padding = strings.Repeat(" ", innerWidth-lineWidth)
		}
		contentLines = append(contentLines, borderStyle.Render(borderVertical)+row+padding+borderStyle.Render(borderVertical))
	}

	// Build bottom border
	bottomBorder := borderStyle.Render(borderBottomLeft + strings.Repeat(borderHorizontal, innerWidth) + borderBottomRight)

	return topBorder + "\n" + strings.Join(contentLines, "\n") + "\n" + bottomBorder
}
