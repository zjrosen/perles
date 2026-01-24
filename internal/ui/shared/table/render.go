package table

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/zjrosen/perles/internal/ui/styles"
)

// Cached styles to avoid repeated allocations during rendering.
// These are initialized once and reused across all table renders.
var (
	selectionBgStyle = lipgloss.NewStyle().Background(styles.SelectionBackgroundColor)
)

// renderHeader renders the table header row.
// Returns the header row string with column alignment applied.
func renderHeader(cols []ColumnConfig, widths []int) string {
	if len(cols) == 0 || len(widths) == 0 {
		return ""
	}

	var parts []string
	for i, col := range cols {
		w := widths[i]
		header := col.Header

		// Truncate header if needed
		if lipgloss.Width(header) > w {
			header = styles.TruncateString(header, w)
		}

		// Apply alignment
		cell := alignText(header, w, col.Align)
		parts = append(parts, cell)
	}

	// Join with single space separator
	return strings.Join(parts, " ")
}

// renderRow renders a single data row.
// selected indicates whether this row should be highlighted.
// fullWidth is the total available width for the row (used to extend selection background).
func renderRow(row any, cols []ColumnConfig, widths []int, selected bool, fullWidth int) string {
	if len(cols) == 0 || len(widths) == 0 {
		return ""
	}

	// For selected rows, we need to apply background to each part individually.
	// The key is that each styled segment must have BOTH foreground AND background
	// applied in the same style - we can't wrap pre-styled content with a background.
	// Use cached style to avoid repeated allocations.
	bgColor := styles.SelectionBackgroundColor

	var result strings.Builder
	for i, col := range cols {
		w := widths[i]

		// Add separator before each cell (except first)
		if i > 0 {
			if selected {
				result.WriteString(selectionBgStyle.Render(" "))
			} else {
				result.WriteString(" ")
			}
		}

		// Render the cell - pass selected so render callback can include background
		cell := renderCellWithBackground(row, col, w, selected, bgColor)
		result.WriteString(cell)
	}

	content := result.String()

	// Pad to full available width so selection extends to the right edge
	if selected {
		contentWidth := lipgloss.Width(content)
		if contentWidth < fullWidth {
			content += selectionBgStyle.Render(strings.Repeat(" ", fullWidth-contentWidth))
		}
	}

	return content
}

// renderCellWithBackground renders a cell with optional selection background.
// When selected, it applies the background color to the entire cell content,
// preserving any foreground styling from the render callback.
func renderCellWithBackground(row any, col ColumnConfig, width int, selected bool, bgColor lipgloss.AdaptiveColor) string {
	// Get cell content via Render callback with panic recovery
	content := safeRenderCallback(row, col, width, selected)

	// Truncate if needed
	if lipgloss.Width(content) > width {
		content = styles.TruncateString(content, width)
	}

	// Apply alignment (pad to width)
	// For selected cells, we need to apply background to the padding too
	contentWidth := lipgloss.Width(content)
	padding := width - contentWidth

	if selected {
		// Use cached style to avoid allocations
		bgStyle := selectionBgStyle

		// Build the aligned content with background applied to each segment
		var result strings.Builder

		switch col.Align {
		case lipgloss.Right:
			if padding > 0 {
				result.WriteString(bgStyle.Render(strings.Repeat(" ", padding)))
			}
			// Apply background to the content itself
			result.WriteString(applyBackgroundToStyledContent(content, bgColor))
		case lipgloss.Center:
			leftPad := padding / 2
			rightPad := padding - leftPad
			if leftPad > 0 {
				result.WriteString(bgStyle.Render(strings.Repeat(" ", leftPad)))
			}
			result.WriteString(applyBackgroundToStyledContent(content, bgColor))
			if rightPad > 0 {
				result.WriteString(bgStyle.Render(strings.Repeat(" ", rightPad)))
			}
		default: // lipgloss.Left
			result.WriteString(applyBackgroundToStyledContent(content, bgColor))
			if padding > 0 {
				result.WriteString(bgStyle.Render(strings.Repeat(" ", padding)))
			}
		}

		return result.String()
	}

	// Non-selected: just apply alignment
	return alignText(content, width, col.Align)
}

// Cached ANSI prefix for selection background (computed once).
var selectionBgPrefix string

func init() {
	// Pre-compute the ANSI prefix for selection background
	bgRendered := selectionBgStyle.Render(" ")
	selectionBgPrefix = strings.TrimSuffix(bgRendered, " \x1b[0m")
}

// applyBackgroundToStyledContent applies a background color to content that may
// already have foreground styling. It handles ANSI reset sequences by replacing
// them with sequences that only reset foreground while preserving background.
func applyBackgroundToStyledContent(content string, _ lipgloss.AdaptiveColor) string {
	// If content has no ANSI codes, just apply background directly
	if !strings.Contains(content, "\x1b[") {
		return selectionBgStyle.Render(content)
	}

	// Content has ANSI styling. The problem is that ANSI reset codes (\x1b[0m)
	// inside the content will reset the background color we apply.
	// Solution: Replace full resets with resets that restore the background.

	// Use pre-computed ANSI prefix for selection background
	bgPrefix := selectionBgPrefix

	// Replace full resets with: reset + background restore
	// \x1b[0m -> \x1b[0m + bgPrefix
	contentWithBg := strings.ReplaceAll(content, "\x1b[0m", "\x1b[0m"+bgPrefix)

	// Now wrap the whole thing with background
	return bgPrefix + contentWithBg + "\x1b[0m"
}

// safeRenderCallback invokes the column's Render callback with panic recovery.
// If the callback panics (e.g., from bad type assertion), returns a placeholder string.
func safeRenderCallback(row any, col ColumnConfig, width int, selected bool) (result string) {
	// Set up panic recovery
	defer func() {
		if r := recover(); r != nil {
			// Return error indicator on panic
			result = styles.TruncateString(fmt.Sprintf("!ERR:%v", r), width)
		}
	}()

	if col.Render == nil {
		return ""
	}

	return col.Render(row, col.Key, width, selected)
}

// renderEmptyState renders the centered empty state message.
// msg is the message to display, width/height define the available space.
func renderEmptyState(msg string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}

	if msg == "" {
		msg = "No data"
	}

	// Style the message with muted color
	styledMsg := lipgloss.NewStyle().
		Foreground(styles.TextMutedColor).
		Render(msg)

	// Truncate if message is too wide
	msgWidth := lipgloss.Width(styledMsg)
	if msgWidth > width {
		styledMsg = styles.TruncateString(msg, width)
		msgWidth = lipgloss.Width(styledMsg)
	}

	// Center horizontally
	leftPad := max((width-msgWidth)/2, 0)
	centeredLine := strings.Repeat(" ", leftPad) + styledMsg

	// Center vertically
	topPad := max((height-1)/2, 0)

	var lines []string
	for range topPad {
		lines = append(lines, "")
	}
	lines = append(lines, centeredLine)
	// Fill remaining height
	remaining := height - topPad - 1
	for range remaining {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

// alignText aligns text within the given width according to position.
func alignText(text string, width int, align lipgloss.Position) string {
	textWidth := lipgloss.Width(text)
	if textWidth >= width {
		return text
	}

	padding := width - textWidth

	switch align {
	case lipgloss.Right:
		return strings.Repeat(" ", padding) + text
	case lipgloss.Center:
		leftPad := padding / 2
		rightPad := padding - leftPad
		return strings.Repeat(" ", leftPad) + text + strings.Repeat(" ", rightPad)
	default: // lipgloss.Left or any other value
		return text + strings.Repeat(" ", padding)
	}
}
