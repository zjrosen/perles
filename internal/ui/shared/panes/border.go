// Package panes contains reusable bordered pane UI components.
package panes

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/zjrosen/perles/internal/ui/styles"
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

// BorderConfig configures the appearance of a bordered panel.
// This is the new struct-based API that replaces the 8-parameter RenderWithTitleBorder function.
type BorderConfig struct {
	// Required fields
	Content string // The content to render inside the border
	Width   int    // Total width including borders
	Height  int    // Total height including borders

	// Title placement (6 positions - all optional)
	TopLeft      string // Title on top border, left-aligned
	TopMiddle    string // Title on top border, centered (not yet implemented)
	TopRight     string // Title on top border, right-aligned
	BottomLeft   string // Title on bottom border, left-aligned
	BottomMiddle string // Title on bottom border, centered (not yet implemented)
	BottomRight  string // Title on bottom border, right-aligned

	// Styling
	Focused            bool                   // Whether the panel has focus
	TitleColor         lipgloss.TerminalColor // Color for title text
	BorderColor        lipgloss.TerminalColor // Border color when not focused
	FocusedBorderColor lipgloss.TerminalColor // Border color when focused
}

// BorderedPane renders content within a bordered panel with optional titles.
// This is the new struct-based API. For backward compatibility, use RenderWithTitleBorder.
//
// Nil color fallback rules:
//   - Both BorderColor and FocusedBorderColor nil: use BorderDefaultColor for both states
//   - BorderColor set, FocusedBorderColor nil: inherit BorderColor for focused state
//   - BorderColor nil, FocusedBorderColor set: unfocused uses BorderDefaultColor, focused uses specified
//   - Both set: use appropriately based on Focused flag
func BorderedPane(cfg BorderConfig) string {
	// Resolve border color with fallback logic
	borderColor := resolveBorderColor(cfg.BorderColor, cfg.FocusedBorderColor, cfg.Focused)

	// Resolve title color (default to BorderDefaultColor if nil)
	titleColor := cfg.TitleColor
	if titleColor == nil {
		titleColor = styles.BorderDefaultColor
	}

	borderStyle := lipgloss.NewStyle().Foreground(borderColor)
	titleStyle := lipgloss.NewStyle().Foreground(titleColor)

	// Calculate inner width (excluding border characters)
	innerWidth := max(cfg.Width-2, 1) // -2 for left and right border

	// Build top border with embedded titles
	topBorder := buildDualTitleTopBorder(cfg.TopLeft, cfg.TopRight, innerWidth, borderStyle, titleStyle)

	// Build bottom border with embedded titles (handles empty titles correctly)
	// Format: ╰─ BottomLeft ─────────────────── BottomRight ─╯
	bottomBorder := buildDualTitleBottomBorder(cfg.BottomLeft, cfg.BottomRight, innerWidth, borderStyle, titleStyle)

	// Calculate content height (excluding top and bottom borders)
	contentHeight := max(cfg.Height-2, 1)

	// Use lipgloss to constrain content width (handles wrapping/truncation properly)
	contentStyle := lipgloss.NewStyle().Width(innerWidth).Height(contentHeight)
	constrainedContent := contentStyle.Render(cfg.Content)

	// Split constrained content into lines
	contentLines := strings.Split(constrainedContent, "\n")
	paddedLines := make([]string, contentHeight)

	for i := range contentHeight {
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

// resolveBorderColor implements the nil color fallback logic for border colors.
//
// Fallback rules:
//   - Both nil: use BorderDefaultColor
//   - BorderColor set, FocusedBorderColor nil: inherit BorderColor for focused state
//   - BorderColor nil, FocusedBorderColor set: unfocused uses BorderDefaultColor, focused uses specified
//   - Both set: use appropriately based on focused flag
func resolveBorderColor(borderColor, focusedBorderColor lipgloss.TerminalColor, focused bool) lipgloss.TerminalColor {
	// Case 1: Both nil - use default
	if borderColor == nil && focusedBorderColor == nil {
		return styles.BorderDefaultColor
	}

	// Case 2: BorderColor set, FocusedBorderColor nil - inherit BorderColor
	if borderColor != nil && focusedBorderColor == nil {
		return borderColor
	}

	// Case 3: BorderColor nil, FocusedBorderColor set
	if borderColor == nil && focusedBorderColor != nil {
		if focused {
			return focusedBorderColor
		}
		return styles.BorderDefaultColor
	}

	// Case 4: Both set - use appropriately based on focused flag
	if focused {
		return focusedBorderColor
	}
	return borderColor
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
		displayTitle = styles.TruncateString(displayTitle, availableForTitle)
	}

	// Calculate remaining width for trailing dashes
	// Inner: "─ " (2) + title + " " (1) + dashes = innerWidth
	// So: dashes = innerWidth - 3 - titleTextWidth
	titleTextWidth := lipgloss.Width(displayTitle)
	remainingWidth := max(innerWidth-3-titleTextWidth, 0)

	// Build: ╭─ Title ──────╮
	// Border parts use borderStyle, title text uses titleStyle
	return borderStyle.Render(borderTopLeft+borderHorizontal+" ") +
		titleStyle.Render(displayTitle) +
		borderStyle.Render(" "+strings.Repeat(borderHorizontal, remainingWidth)+borderTopRight)
}

// buildDualTitleTopBorder creates the top border with titles on both left and right.
// Format: ╭─ LeftTitle ─────────────────── RightTitle ─╮
func buildDualTitleTopBorder(leftTitle, rightTitle string, innerWidth int, borderStyle, titleStyle lipgloss.Style) string {
	if innerWidth < 1 {
		return borderStyle.Render(borderTopLeft + borderTopRight)
	}

	// If both titles are empty, just render a plain top border
	if leftTitle == "" && rightTitle == "" {
		return borderStyle.Render(borderTopLeft + strings.Repeat(borderHorizontal, innerWidth) + borderTopRight)
	}

	// Calculate widths
	leftTitleWidth := lipgloss.Width(leftTitle)
	rightTitleWidth := lipgloss.Width(rightTitle)

	// Minimum format: "─ Left ─ Right ─" = 2 + left + 1 + middle + 1 + right + 1
	// We need space for: "─ " + leftTitle + " " + middleDashes + " " + rightTitle + " ─"
	minRequired := 2 + leftTitleWidth + 1 + 1 + 1 + rightTitleWidth + 2
	if rightTitle == "" {
		// Just left title: "─ " + leftTitle + " " + dashes
		minRequired = 2 + leftTitleWidth + 1 + 1
	}
	if leftTitle == "" {
		// Just right title: dashes + " " + rightTitle + " ─"
		minRequired = 1 + 1 + rightTitleWidth + 2
	}

	if innerWidth < minRequired {
		// Too narrow, fall back to simple border or just left title
		if leftTitle != "" {
			return buildTopBorder(leftTitle, innerWidth, borderStyle, titleStyle)
		}
		return borderStyle.Render(borderTopLeft + strings.Repeat(borderHorizontal, innerWidth) + borderTopRight)
	}

	// Calculate middle dashes
	// Format: ╭─ Left ───────── Right ─╮
	// innerWidth = 2 + leftWidth + 1 + middleDashes + 1 + rightWidth + 2
	// middleDashes = innerWidth - 2 - leftWidth - 1 - 1 - rightWidth - 2
	//              = innerWidth - leftWidth - rightWidth - 6
	var middleDashes int
	if leftTitle != "" && rightTitle != "" {
		middleDashes = innerWidth - leftTitleWidth - rightTitleWidth - 6
	} else if leftTitle != "" {
		// No right title: ╭─ Left ─────────╮
		middleDashes = innerWidth - leftTitleWidth - 3
	} else {
		// No left title: ╭───────── Right ─╮
		middleDashes = innerWidth - rightTitleWidth - 3
	}
	middleDashes = max(middleDashes, 1)

	// Build the border
	var result strings.Builder
	result.WriteString(borderStyle.Render(borderTopLeft))

	if leftTitle != "" {
		result.WriteString(borderStyle.Render(borderHorizontal + " "))
		result.WriteString(titleStyle.Render(leftTitle))
		result.WriteString(borderStyle.Render(" "))
	}

	result.WriteString(borderStyle.Render(strings.Repeat(borderHorizontal, middleDashes)))

	if rightTitle != "" {
		result.WriteString(borderStyle.Render(" "))
		result.WriteString(titleStyle.Render(rightTitle))
		result.WriteString(borderStyle.Render(" " + borderHorizontal))
	}

	result.WriteString(borderStyle.Render(borderTopRight))

	return result.String()
}

// buildDualTitleBottomBorder creates the bottom border with titles on both left and right.
// Format: ╰─ LeftTitle ─────────────────── RightTitle ─╯
// This mirrors buildDualTitleTopBorder but uses bottom corner characters.
func buildDualTitleBottomBorder(leftTitle, rightTitle string, innerWidth int, borderStyle, titleStyle lipgloss.Style) string {
	if innerWidth < 1 {
		return borderStyle.Render(borderBottomLeft + borderBottomRight)
	}

	// If both titles are empty, just render a plain bottom border
	if leftTitle == "" && rightTitle == "" {
		return borderStyle.Render(borderBottomLeft + strings.Repeat(borderHorizontal, innerWidth) + borderBottomRight)
	}

	// Calculate widths
	leftTitleWidth := lipgloss.Width(leftTitle)
	rightTitleWidth := lipgloss.Width(rightTitle)

	// Minimum format: "─ Left ─ Right ─" = 2 + left + 1 + middle + 1 + right + 1
	// We need space for: "─ " + leftTitle + " " + middleDashes + " " + rightTitle + " ─"
	minRequired := 2 + leftTitleWidth + 1 + 1 + 1 + rightTitleWidth + 2
	if rightTitle == "" {
		// Just left title: "─ " + leftTitle + " " + dashes
		minRequired = 2 + leftTitleWidth + 1 + 1
	}
	if leftTitle == "" {
		// Just right title: dashes + " " + rightTitle + " ─"
		minRequired = 1 + 1 + rightTitleWidth + 2
	}

	if innerWidth < minRequired {
		// Too narrow, fall back to simple border or just left title
		if leftTitle != "" {
			return buildBottomBorder(leftTitle, innerWidth, borderStyle, titleStyle)
		}
		return borderStyle.Render(borderBottomLeft + strings.Repeat(borderHorizontal, innerWidth) + borderBottomRight)
	}

	// Calculate middle dashes
	// Format: ╰─ Left ───────── Right ─╯
	// innerWidth = 2 + leftWidth + 1 + middleDashes + 1 + rightWidth + 2
	// middleDashes = innerWidth - 2 - leftWidth - 1 - 1 - rightWidth - 2
	//              = innerWidth - leftWidth - rightWidth - 6
	var middleDashes int
	if leftTitle != "" && rightTitle != "" {
		middleDashes = innerWidth - leftTitleWidth - rightTitleWidth - 6
	} else if leftTitle != "" {
		// No right title: ╰─ Left ─────────╯
		middleDashes = innerWidth - leftTitleWidth - 3
	} else {
		// No left title: ╰───────── Right ─╯
		middleDashes = innerWidth - rightTitleWidth - 3
	}
	middleDashes = max(middleDashes, 1)

	// Build the border
	var result strings.Builder
	result.WriteString(borderStyle.Render(borderBottomLeft))

	if leftTitle != "" {
		result.WriteString(borderStyle.Render(borderHorizontal + " "))
		result.WriteString(titleStyle.Render(leftTitle))
		result.WriteString(borderStyle.Render(" "))
	}

	result.WriteString(borderStyle.Render(strings.Repeat(borderHorizontal, middleDashes)))

	if rightTitle != "" {
		result.WriteString(borderStyle.Render(" "))
		result.WriteString(titleStyle.Render(rightTitle))
		result.WriteString(borderStyle.Render(" " + borderHorizontal))
	}

	result.WriteString(borderStyle.Render(borderBottomRight))

	return result.String()
}

// buildBottomBorder creates the bottom border with embedded title.
// borderStyle is used for border characters, titleStyle for the title text.
// This mirrors buildTopBorder but uses bottom corner characters.
func buildBottomBorder(title string, innerWidth int, borderStyle, titleStyle lipgloss.Style) string {
	// Format: ╰─ Title ──────╯
	// Minimum: ╰─╯ (3 chars for just borders)

	if innerWidth < 1 {
		return borderStyle.Render(borderBottomLeft + borderBottomRight)
	}

	// If title is empty, just render a plain bottom border
	if title == "" {
		return borderStyle.Render(borderBottomLeft + strings.Repeat(borderHorizontal, innerWidth) + borderBottomRight)
	}

	// Calculate space for title
	// Format: ─ Title ─
	// We need at least 4 chars: "─ " + " ─"
	titlePartMinWidth := 4

	if innerWidth < titlePartMinWidth {
		// Too narrow for title, just render plain border
		return borderStyle.Render(borderBottomLeft + strings.Repeat(borderHorizontal, innerWidth) + borderBottomRight)
	}

	// Calculate available space for title text
	availableForTitle := innerWidth - 4 // "─ " before and " ─" after (minimum)

	displayTitle := title
	if lipgloss.Width(displayTitle) > availableForTitle {
		// Truncate title with ellipsis
		displayTitle = styles.TruncateString(displayTitle, availableForTitle)
	}

	// Calculate remaining width for trailing dashes
	// Inner: "─ " (2) + title + " " (1) + dashes = innerWidth
	// So: dashes = innerWidth - 3 - titleTextWidth
	titleTextWidth := lipgloss.Width(displayTitle)
	remainingWidth := max(innerWidth-3-titleTextWidth, 0)

	// Build: ╰─ Title ──────╯
	// Border parts use borderStyle, title text uses titleStyle
	return borderStyle.Render(borderBottomLeft+borderHorizontal+" ") +
		titleStyle.Render(displayTitle) +
		borderStyle.Render(" "+strings.Repeat(borderHorizontal, remainingWidth)+borderBottomRight)
}
