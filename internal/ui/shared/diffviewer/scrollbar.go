// Package diffviewer provides a TUI component for viewing git diffs.
package diffviewer

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/zjrosen/perles/internal/ui/styles"
)

// Scrollbar characters
const (
	scrollbarThumbChar = '█' // Full block
	scrollbarTrackChar = '░' // Light shade
)

// ScrollbarConfig configures scrollbar rendering.
type ScrollbarConfig struct {
	// Dimensions
	TotalLines     int // Total lines in content
	ViewportHeight int // Visible lines in viewport
	ScrollOffset   int // Current scroll position (top line)

	// Style configuration
	TrackChar string // Track character (default: "░")
	ThumbChar string // Thumb character (default: "█")
}

// DefaultScrollbarConfig returns default configuration.
func DefaultScrollbarConfig() ScrollbarConfig {
	return ScrollbarConfig{
		TrackChar: string(scrollbarTrackChar),
		ThumbChar: string(scrollbarThumbChar),
	}
}

// calculateThumbBounds returns the start row and height of the scroll thumb.
// Formula: thumbHeight = max(1, viewportHeight * viewportHeight / totalLines)
// Position: start = scrollOffset * viewportHeight / totalLines
func calculateThumbBounds(cfg ScrollbarConfig) (start, height int) {
	if cfg.TotalLines <= 0 || cfg.ViewportHeight <= 0 {
		return 0, 0
	}

	// If content fits in viewport, thumb fills entire track
	if cfg.TotalLines <= cfg.ViewportHeight {
		return 0, cfg.ViewportHeight
	}

	// Thumb height proportional to visible/total ratio
	// Minimum height is 1 to ensure thumb is always visible
	height = max(1, cfg.ViewportHeight*cfg.ViewportHeight/cfg.TotalLines)

	// Calculate thumb position
	maxOffset := cfg.TotalLines - cfg.ViewportHeight
	if maxOffset <= 0 {
		return 0, height
	}

	// Scrollable track area (total height minus thumb size)
	scrollableTrack := cfg.ViewportHeight - height
	if scrollableTrack <= 0 {
		return 0, height
	}

	// Position thumb proportionally within scrollable track
	start = scrollableTrack * cfg.ScrollOffset / maxOffset

	// Clamp to valid range
	start = max(0, min(start, cfg.ViewportHeight-height))

	return start, height
}

// RenderScrollbar renders the scrollbar as a string (height lines joined by \n).
// Returns empty string if config is invalid or content fits in viewport.
// Simple scrollbar: track (░) and thumb (█) to show scroll position.
func RenderScrollbar(cfg ScrollbarConfig) string {
	if cfg.ViewportHeight <= 0 || cfg.TotalLines <= 0 {
		return ""
	}

	// If content fits in viewport, no scrollbar needed (return spaces)
	if cfg.TotalLines <= cfg.ViewportHeight {
		lines := make([]string, cfg.ViewportHeight)
		for i := range lines {
			lines[i] = " "
		}
		return strings.Join(lines, "\n")
	}

	// Calculate thumb bounds
	thumbStart, thumbHeight := calculateThumbBounds(cfg)

	// Prepare styles
	trackStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)
	thumbStyle := lipgloss.NewStyle().Foreground(styles.TextSecondaryColor)

	trackChar := cfg.TrackChar
	if trackChar == "" {
		trackChar = string(scrollbarTrackChar)
	}
	thumbChar := cfg.ThumbChar
	if thumbChar == "" {
		thumbChar = string(scrollbarThumbChar)
	}

	// Render each row - simple track + thumb
	lines := make([]string, cfg.ViewportHeight)

	for row := range cfg.ViewportHeight {
		isThumb := row >= thumbStart && row < thumbStart+thumbHeight
		if isThumb {
			lines[row] = thumbStyle.Render(thumbChar)
		} else {
			lines[row] = trackStyle.Render(trackChar)
		}
	}

	return strings.Join(lines, "\n")
}
