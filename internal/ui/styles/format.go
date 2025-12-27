// Package styles contains Lip Gloss style definitions.
package styles

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// TruncateString truncates a string to fit within maxWidth, adding ellipsis if needed.
func TruncateString(s string, maxWidth int) string {
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

// FormatCommentIndicator returns the comment indicator string.
// Returns empty string when count is 0.
func FormatCommentIndicator(count int) string {
	if count <= 0 {
		return ""
	}
	return fmt.Sprintf("%d\U0001F4AC", count) // 💬
}
