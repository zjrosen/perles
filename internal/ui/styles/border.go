// Package styles contains Lip Gloss style definitions.
package styles

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Border characters (rounded)
const (
	borderTopLeft     = "╭"
	borderTopRight    = "╮"
	borderBottomLeft  = "╰"
	borderBottomRight = "╯"
	borderHorizontal  = "─"
	borderVertical    = "│"
	borderMiddleLeft  = "├"
	borderMiddleRight = "┤"
)

// RenderWithTitleBorder renders content with a title embedded in the top border.
// Similar to lazygit's panel style: ╭─ Title ─────╮
// titleColor is used for the title text, focusedBorderColor is used for the border when focused.
func RenderWithTitleBorder(content, title string, width, height int, focused bool, titleColor, focusedBorderColor lipgloss.TerminalColor) string {
	// Border color: use focusedBorderColor when focused, BorderDefaultColor when not
	var borderColor lipgloss.TerminalColor = BorderDefaultColor
	if focused {
		borderColor = focusedBorderColor
	}

	borderStyle := lipgloss.NewStyle().Foreground(borderColor)
	titleStyle := lipgloss.NewStyle().Foreground(titleColor)

	// Calculate inner width (excluding border characters)
	innerWidth := width - 2 // -2 for left and right border
	if innerWidth < 1 {
		innerWidth = 1
	}

	// Build top border with embedded title
	// Format: ╭─ Title ─────────╮
	topBorder := buildTopBorder(title, innerWidth, borderStyle, titleStyle)

	// Build bottom border
	bottomBorder := borderStyle.Render(borderBottomLeft + strings.Repeat(borderHorizontal, innerWidth) + borderBottomRight)

	// Calculate content height (excluding top and bottom borders)
	contentHeight := height - 2
	if contentHeight < 1 {
		contentHeight = 1
	}

	// Use lipgloss to constrain content width (handles wrapping/truncation properly)
	contentStyle := lipgloss.NewStyle().Width(innerWidth).Height(contentHeight)
	constrainedContent := contentStyle.Render(content)

	// Split constrained content into lines
	contentLines := strings.Split(constrainedContent, "\n")
	paddedLines := make([]string, contentHeight)

	for i := 0; i < contentHeight; i++ {
		var line string
		if i < len(contentLines) {
			line = contentLines[i]
		}

		// Pad line to innerWidth to ensure right border aligns
		lineWidth := lipgloss.Width(line)
		if lineWidth < innerWidth {
			line = line + strings.Repeat(" ", innerWidth-lineWidth)
		}

		// Add side borders
		paddedLines[i] = borderStyle.Render(borderVertical) + line + borderStyle.Render(borderVertical)
	}

	// Join all parts
	var result strings.Builder
	result.WriteString(topBorder)
	result.WriteString("\n")
	result.WriteString(strings.Join(paddedLines, "\n"))
	result.WriteString("\n")
	result.WriteString(bottomBorder)

	return result.String()
}

// buildTopBorder creates the top border with embedded title.
// borderStyle is used for border characters, titleStyle for the title text.
func buildTopBorder(title string, innerWidth int, borderStyle, titleStyle lipgloss.Style) string {
	// Format: ╭─ Title ──────╮
	// Minimum: ╭─╮ (3 chars for just borders)

	if innerWidth < 1 {
		return borderStyle.Render(borderTopLeft + borderTopRight)
	}

	// If title is empty, just render a plain top border
	if title == "" {
		return borderStyle.Render(borderTopLeft + strings.Repeat(borderHorizontal, innerWidth) + borderTopRight)
	}

	// Calculate space for title
	// Format: ─ Title ─
	// We need at least 4 chars: "─ " + " ─"
	titlePartMinWidth := 4

	if innerWidth < titlePartMinWidth {
		// Too narrow for title, just render plain border
		return borderStyle.Render(borderTopLeft + strings.Repeat(borderHorizontal, innerWidth) + borderTopRight)
	}

	// Calculate available space for title text
	availableForTitle := innerWidth - 4 // "─ " before and " ─" after (minimum)

	displayTitle := title
	if lipgloss.Width(displayTitle) > availableForTitle {
		// Truncate title with ellipsis
		displayTitle = truncateString(displayTitle, availableForTitle)
	}

	// Calculate remaining width for trailing dashes
	// Inner: "─ " (2) + title + " " (1) + dashes = innerWidth
	// So: dashes = innerWidth - 3 - titleTextWidth
	titleTextWidth := lipgloss.Width(displayTitle)
	remainingWidth := innerWidth - 3 - titleTextWidth
	if remainingWidth < 0 {
		remainingWidth = 0
	}

	// Build: ╭─ Title ──────╮
	// Border parts use borderStyle, title text uses titleStyle
	return borderStyle.Render(borderTopLeft+borderHorizontal+" ") +
		titleStyle.Render(displayTitle) +
		borderStyle.Render(" "+strings.Repeat(borderHorizontal, remainingWidth)+borderTopRight)
}

// truncateString truncates a string to fit within maxWidth, adding ellipsis if needed.
func truncateString(s string, maxWidth int) string {
	if maxWidth < 1 {
		return ""
	}

	if lipgloss.Width(s) <= maxWidth {
		return s
	}

	// Need to truncate - leave room for ellipsis
	if maxWidth <= 3 {
		return strings.Repeat(".", maxWidth)
	}

	// Truncate rune by rune
	result := ""
	for _, r := range s {
		test := result + string(r)
		if lipgloss.Width(test) > maxWidth-3 {
			break
		}
		result = test
	}

	return result + "..."
}
