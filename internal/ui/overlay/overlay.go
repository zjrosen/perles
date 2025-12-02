// Package overlay provides utilities for rendering modal content
// on top of background views without clearing the screen.
package overlay

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Position specifies where to place the overlay content.
type Position int

const (
	// Center places the overlay in the center of the viewport.
	Center Position = iota
	// Top places the overlay at the top center of the viewport.
	Top
	// Bottom places the overlay at the bottom center of the viewport.
	Bottom
)

// Config controls overlay rendering behavior.
type Config struct {
	// Width is the total viewport width.
	Width int
	// Height is the total viewport height.
	Height int
	// Position specifies where to place the overlay (Center, Top, Bottom).
	Position Position
	// PadX adds horizontal padding from edges (unused for Center position).
	PadX int
	// PadY adds vertical padding from edges (for Top/Bottom positions).
	PadY int
}

// Place renders foreground content on top of background.
// Uses ANSI-aware string manipulation to preserve styling in both
// the foreground and background content.
func Place(cfg Config, fg, bg string) string {
	fgLines := strings.Split(fg, "\n")
	bgLines := strings.Split(bg, "\n")

	// Pad background to full height
	for len(bgLines) < cfg.Height {
		bgLines = append(bgLines, strings.Repeat(" ", cfg.Width))
	}

	fgHeight := len(fgLines)
	fgWidth := lipgloss.Width(fg)

	// Calculate position
	startX, startY := calculatePosition(cfg, fgWidth, fgHeight)

	// Overlay foreground onto background
	for i, fgLine := range fgLines {
		bgY := startY + i
		if bgY >= len(bgLines) {
			break
		}

		bgLine := bgLines[bgY]
		fgLineWidth := ansi.StringWidth(fgLine)

		// Get left portion of background (ANSI-aware truncation)
		leftPart := ansi.Truncate(bgLine, startX, "")

		// Pad left part if background is shorter than startX
		leftWidth := ansi.StringWidth(leftPart)
		if leftWidth < startX {
			leftPart += strings.Repeat(" ", startX-leftWidth)
		}

		// Get right portion of background after the overlay
		endX := startX + fgLineWidth
		bgWidth := ansi.StringWidth(bgLine)
		var rightPart string
		if endX < bgWidth {
			// TruncateLeft removes chars from the left, keeping the right
			rightPart = ansi.TruncateLeft(bgLine, endX, "")
		}

		// Combine: left background + foreground + right background
		bgLines[bgY] = leftPart + fgLine + rightPart
	}

	return strings.Join(bgLines, "\n")
}

// calculatePosition determines the x,y starting coordinates for the overlay.
func calculatePosition(cfg Config, fgWidth, fgHeight int) (x, y int) {
	switch cfg.Position {
	case Top:
		x = (cfg.Width - fgWidth) / 2
		y = cfg.PadY
	case Bottom:
		x = (cfg.Width - fgWidth) / 2
		y = cfg.Height - fgHeight - cfg.PadY
	default: // Center
		x = (cfg.Width - fgWidth) / 2
		y = (cfg.Height - fgHeight) / 2
	}

	// Ensure non-negative
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	return x, y
}
