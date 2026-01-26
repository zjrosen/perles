// Package selection provides text selection functionality for TUI viewports.
// It handles drag-to-select behavior with proper coordinate mapping and text extraction.
package selection

import (
	"strings"

	"github.com/rivo/uniseg"
)

// sliceByDisplayCols extracts a substring between display column positions.
// This properly handles wide characters like emojis.
func sliceByDisplayCols(s string, startCol, endCol int) string {
	if startCol >= endCol || s == "" {
		return ""
	}

	var result strings.Builder
	currentCol := 0
	state := -1

	for len(s) > 0 {
		cluster, rest, _, newState := uniseg.StepString(s, state)
		width := uniseg.StringWidth(cluster)

		// Check if this cluster is within our selection range
		clusterEnd := currentCol + width
		if currentCol >= startCol && clusterEnd <= endCol {
			result.WriteString(cluster)
		} else if currentCol < endCol && clusterEnd > startCol {
			// Cluster partially overlaps - include it
			result.WriteString(cluster)
		}

		currentCol = clusterEnd
		if currentCol >= endCol {
			break
		}

		s = rest
		state = newState
	}

	return result.String()
}

// sliceFromDisplayCol extracts substring from a display column to end of string.
func sliceFromDisplayCol(s string, startCol int) string {
	if startCol <= 0 {
		return s
	}
	if s == "" {
		return ""
	}

	currentCol := 0
	state := -1

	for len(s) > 0 {
		cluster, rest, _, newState := uniseg.StepString(s, state)
		width := uniseg.StringWidth(cluster)

		if currentCol >= startCol {
			// Return from current position
			return s
		}

		currentCol += width
		s = rest
		state = newState
	}

	return "" // Past end of string
}

// sliceToDisplayCol extracts substring from start to a display column.
func sliceToDisplayCol(s string, endCol int) string {
	if endCol <= 0 {
		return ""
	}
	if s == "" {
		return ""
	}

	var result strings.Builder
	currentCol := 0
	state := -1

	for len(s) > 0 {
		cluster, rest, _, newState := uniseg.StepString(s, state)
		width := uniseg.StringWidth(cluster)

		if currentCol+width > endCol {
			break
		}

		result.WriteString(cluster)
		currentCol += width
		s = rest
		state = newState
	}

	return result.String()
}

// stringDisplayWidth returns the display width of a string in terminal columns.
func stringDisplayWidth(s string) int {
	return uniseg.StringWidth(s)
}

// Point represents a position in content for text selection.
type Point struct {
	Line int // Line number in content (0-indexed)
	Col  int // Column/character position in line (0-indexed)
}

// TextSelection manages text selection state for a viewport.
type TextSelection struct {
	isSelecting bool
	start       Point
	end         Point
	plainLines  []string
	dirty       bool // Whether selection state changed (for triggering re-render)
}

// New creates a new TextSelection.
func New() *TextSelection {
	return &TextSelection{}
}

// Start begins a new selection at the given position.
func (s *TextSelection) Start(pos Point) {
	s.isSelecting = true
	s.start = pos
	s.end = pos
	s.dirty = true
}

// Update updates the selection end point during drag.
// Returns true if the selection changed.
func (s *TextSelection) Update(pos Point) bool {
	if !s.isSelecting {
		return false
	}
	if pos == s.end {
		return false
	}
	s.end = pos
	s.dirty = true
	return true
}

// Finalize ends the selection and returns the selected text.
// Returns empty string if no text was selected.
func (s *TextSelection) Finalize() string {
	s.isSelecting = false
	text := s.GetSelectedText()
	return text
}

// IsSelecting returns true if a drag selection is in progress.
func (s *TextSelection) IsSelecting() bool {
	return s.isSelecting
}

// HasSelection returns true if there is a non-empty selection.
func (s *TextSelection) HasSelection() bool {
	return s.start != s.end
}

// Clear resets the selection state.
func (s *TextSelection) Clear() {
	s.start = Point{}
	s.end = Point{}
	s.isSelecting = false
	s.dirty = true
}

// SetPlainLines sets the plain text content for selection extraction.
// This should be called after rendering content to keep lines in sync.
func (s *TextSelection) SetPlainLines(lines []string) {
	s.plainLines = lines
}

// PlainLines returns the current plain text lines.
func (s *TextSelection) PlainLines() []string {
	return s.plainLines
}

// Dirty returns true if the selection state changed since last check.
func (s *TextSelection) Dirty() bool {
	return s.dirty
}

// ClearDirty resets the dirty flag.
func (s *TextSelection) ClearDirty() {
	s.dirty = false
}

// Normalized returns the selection points in order (start before end).
func (s *TextSelection) Normalized() (start, end Point) {
	start, end = s.start, s.end
	// Swap if end is before start
	if end.Line < start.Line || (end.Line == start.Line && end.Col < start.Col) {
		start, end = end, start
	}
	return start, end
}

// GetSelectedText extracts the selected text from the plain text content.
// Col values are treated as display columns (accounting for wide characters like emojis).
func (s *TextSelection) GetSelectedText() string {
	if !s.HasSelection() {
		return ""
	}
	if len(s.plainLines) == 0 {
		return ""
	}

	start, end := s.Normalized()

	// Clamp to valid ranges
	if start.Line >= len(s.plainLines) {
		return ""
	}
	if end.Line >= len(s.plainLines) {
		end.Line = len(s.plainLines) - 1
		end.Col = stringDisplayWidth(s.plainLines[end.Line])
	}

	// Single line selection - use display-column-aware slicing
	if start.Line == end.Line {
		line := s.plainLines[start.Line]
		return sliceByDisplayCols(line, start.Col, end.Col)
	}

	// Multi-line selection
	var result strings.Builder

	// First line (from start column to end of line)
	firstLine := s.plainLines[start.Line]
	result.WriteString(sliceFromDisplayCol(firstLine, start.Col))
	result.WriteString("\n")

	// Middle lines (full lines)
	for i := start.Line + 1; i < end.Line; i++ {
		result.WriteString(s.plainLines[i])
		result.WriteString("\n")
	}

	// Last line (from start to end column)
	lastLine := s.plainLines[end.Line]
	result.WriteString(sliceToDisplayCol(lastLine, end.Col))

	return result.String()
}

// SelectionBounds returns pointers to start and end points if there's an active selection.
// Returns nil, nil if no selection is active.
func (s *TextSelection) SelectionBounds() (*Point, *Point) {
	if !s.HasSelection() && !s.isSelecting {
		return nil, nil
	}
	start, end := s.Normalized()
	return &start, &end
}
