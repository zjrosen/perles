package styles

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Border characters (rounded) - used by FormSection.
// Note: These are also defined in ui/shared/panes/border.go for the pane components.
const (
	borderTopLeft     = "╭"
	borderTopRight    = "╮"
	borderBottomLeft  = "╰"
	borderBottomRight = "╯"
	borderHorizontal  = "─"
	borderVertical    = "│"
)

// FormSectionConfig configures the appearance of a form section with auto-height.
// This mirrors BorderConfig but is designed for form fields that size to content.
type FormSectionConfig struct {
	// Content is the lines to render inside the border.
	// Height is determined by the number of content lines (auto-height).
	Content []string

	// Width is the total width including borders.
	Width int

	// Title positions (all optional)
	TopLeft     string // Primary title (e.g., "Name", "BQL Query")
	TopLeftHint string // Hint shown after title in parentheses (e.g., "required")
	TopRight    string // Right-aligned title on top border
	BottomLeft  string // Left-aligned on bottom border (e.g., "[NORMAL]")
	BottomRight string // Right-aligned on bottom border

	// Styling
	Focused            bool                   // Whether the section has focus
	FocusedBorderColor lipgloss.TerminalColor // Border/title color when focused
}

// FormSection renders a bordered section with auto-height based on content.
// Border color is BorderDefaultColor when unfocused, FocusedBorderColor when focused.
// Title text is bold and uses the same color as the border.
func FormSection(cfg FormSectionConfig) string {
	// Resolve colors based on focus state
	var borderColor lipgloss.TerminalColor = BorderDefaultColor
	var titleColor lipgloss.TerminalColor = BorderDefaultColor
	if cfg.Focused {
		if cfg.FocusedBorderColor != nil {
			borderColor = cfg.FocusedBorderColor
			titleColor = cfg.FocusedBorderColor
		}
	}

	borderStyle := lipgloss.NewStyle().Foreground(borderColor)
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(titleColor)
	hintStyle := lipgloss.NewStyle().Foreground(TextMutedColor)

	innerWidth := max(cfg.Width-2, 1) // Account for left/right borders

	// Build top border: ╭─ TopLeft (hint) ────── TopRight ─╮
	topBorder := buildFormTopBorder(cfg.TopLeft, cfg.TopLeftHint, cfg.TopRight, innerWidth, borderStyle, titleStyle, hintStyle)

	// Build content lines with side borders
	var contentLines []string
	for _, row := range cfg.Content {
		lineWidth := lipgloss.Width(row)
		padding := ""
		if lineWidth < innerWidth {
			padding = strings.Repeat(" ", innerWidth-lineWidth)
		}
		contentLines = append(contentLines, borderStyle.Render(borderVertical)+row+padding+borderStyle.Render(borderVertical))
	}

	// Build bottom border: ╰─ BottomLeft ────── BottomRight ─╯
	bottomBorder := buildFormBottomBorder(cfg.BottomLeft, cfg.BottomRight, innerWidth, borderStyle, titleStyle)

	return topBorder + "\n" + strings.Join(contentLines, "\n") + "\n" + bottomBorder
}

// buildFormTopBorder creates the top border with title, hint, and optional right title.
// Format: ╭─ Title (hint) ────────── RightTitle ─╮
func buildFormTopBorder(title, hint, rightTitle string, innerWidth int, borderStyle, titleStyle, hintStyle lipgloss.Style) string {
	if title == "" && rightTitle == "" {
		return borderStyle.Render(borderTopLeft + strings.Repeat(borderHorizontal, innerWidth) + borderTopRight)
	}

	// Calculate left part width (title + hint)
	leftPart := ""
	leftPartWidth := 0
	if title != "" {
		leftPart = titleStyle.Render(title)
		leftPartWidth = lipgloss.Width(title)
		if hint != "" {
			hintText := " " + hintStyle.Render("("+hint+")")
			leftPart += hintText
			leftPartWidth += 1 + lipgloss.Width("("+hint+")") // space + hint with parens
		}
	}

	// Calculate right part width
	rightPart := ""
	rightPartWidth := 0
	if rightTitle != "" {
		rightPart = titleStyle.Render(rightTitle)
		rightPartWidth = lipgloss.Width(rightTitle)
	}

	// Calculate dashes needed
	// Format: ╭─ leftPart ─────── rightPart ─╮
	// Inner = "─ " + leftPart + " " + dashes + " " + rightPart + " ─"
	//       = 2 + leftPartWidth + 1 + dashes + 1 + rightPartWidth + 2 (if both)
	var dashesNeeded int
	if title != "" && rightTitle != "" {
		dashesNeeded = innerWidth - leftPartWidth - rightPartWidth - 6
	} else if title != "" {
		// Just left: "─ " + title + " " + dashes
		dashesNeeded = innerWidth - leftPartWidth - 3
	} else {
		// Just right: dashes + " " + rightTitle + " ─"
		dashesNeeded = innerWidth - rightPartWidth - 3
	}
	dashesNeeded = max(dashesNeeded, 1)

	// Build the border
	var result strings.Builder
	result.WriteString(borderStyle.Render(borderTopLeft))

	if title != "" {
		result.WriteString(borderStyle.Render(borderHorizontal + " "))
		result.WriteString(leftPart)
		result.WriteString(borderStyle.Render(" "))
	}

	result.WriteString(borderStyle.Render(strings.Repeat(borderHorizontal, dashesNeeded)))

	if rightTitle != "" {
		result.WriteString(borderStyle.Render(" "))
		result.WriteString(rightPart)
		result.WriteString(borderStyle.Render(" " + borderHorizontal))
	}

	result.WriteString(borderStyle.Render(borderTopRight))

	return result.String()
}

// buildFormBottomBorder creates the bottom border with optional left and right titles.
// Format: ╰─ BottomLeft ────────── BottomRight ─╯
func buildFormBottomBorder(leftTitle, rightTitle string, innerWidth int, borderStyle, titleStyle lipgloss.Style) string {
	if leftTitle == "" && rightTitle == "" {
		return borderStyle.Render(borderBottomLeft + strings.Repeat(borderHorizontal, innerWidth) + borderBottomRight)
	}

	leftPartWidth := lipgloss.Width(leftTitle)
	rightPartWidth := lipgloss.Width(rightTitle)

	// Calculate dashes needed
	var dashesNeeded int
	if leftTitle != "" && rightTitle != "" {
		dashesNeeded = innerWidth - leftPartWidth - rightPartWidth - 6
	} else if leftTitle != "" {
		dashesNeeded = innerWidth - leftPartWidth - 3
	} else {
		dashesNeeded = innerWidth - rightPartWidth - 3
	}
	dashesNeeded = max(dashesNeeded, 1)

	// Build the border
	var result strings.Builder
	result.WriteString(borderStyle.Render(borderBottomLeft))

	if leftTitle != "" {
		result.WriteString(borderStyle.Render(borderHorizontal + " "))
		// BottomLeft content is usually pre-styled (e.g., vim mode indicator), render as-is
		result.WriteString(leftTitle)
		result.WriteString(borderStyle.Render(" "))
	}

	result.WriteString(borderStyle.Render(strings.Repeat(borderHorizontal, dashesNeeded)))

	if rightTitle != "" {
		result.WriteString(borderStyle.Render(" "))
		result.WriteString(titleStyle.Render(rightTitle))
		result.WriteString(borderStyle.Render(" " + borderHorizontal))
	}

	result.WriteString(borderStyle.Render(borderBottomRight))

	return result.String()
}
