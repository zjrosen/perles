package vimtextarea

import (
	"strings"
	"time"

	"github.com/zjrosen/perles/internal/ui/styles"

	"github.com/charmbracelet/lipgloss"
)

// ANSI codes for cursor and selection
// Cursor uses reverse video (bold highlight), selection uses a dimmer background
const (
	cursorOn  = "\x1b[7m"  // reverse video on (bold, high contrast)
	cursorOff = "\x1b[27m" // reverse video off (not full reset)
	// Selection uses a subtle background color (gray) to distinguish from cursor
	// 48;5;238 = 256-color background (dark gray)
	// 38;5;255 = 256-color foreground (bright white for contrast)
	selectionOn  = "\x1b[48;5;238;38;5;255m" // dark gray background, white text
	selectionOff = "\x1b[49;39m"             // reset background and foreground
	// Yank highlight uses a yellow/gold background for brief flash effect
	// 48;5;178 = 256-color background (gold/yellow)
	// 38;5;232 = 256-color foreground (dark text for contrast)
	yankHighlightOn  = "\x1b[48;5;178;38;5;232m" // gold background, dark text
	yankHighlightOff = "\x1b[49;39m"             // reset background and foreground
)

// Style definitions for the vimtextarea
var (
	placeholderStyle = lipgloss.NewStyle().Foreground(styles.TextPlaceholderColor)
)

// View renders the textarea with cursor.
// This implements the tea.Model View interface.
// Note: Mode indicator is NOT rendered here - clients should use Mode() and ModeChangeMsg
// to display mode information in their own UI (e.g., in a BorderedPane footer).
func (m Model) View() string {
	return m.renderContent()
}

// renderContent renders the text content with cursor, handling soft-wrap.
func (m Model) renderContent() string {
	// Handle empty content with placeholder
	if m.isEmpty() {
		return m.renderEmpty()
	}

	// Compute scroll offset in display rows (ensures cursor is visible)
	scrollDisplayRow := m.computeDisplayScrollOffset()

	// Build visible display lines with wrapping
	var displayLines []string
	currentDisplayRow := 0

	// Determine if we need selection rendering
	inVisualMode := m.focused && m.InVisualMode()
	// Check if yank highlight is active and not expired
	hasYankHighlight := m.yankHighlight != nil && time.Now().Before(m.yankHighlight.Expiry)

	for logicalRow, line := range m.content {
		wrappedLines, graphemeStarts := m.wrapLineWithInfo(line)

		for wrapIdx, wrappedLine := range wrappedLines {
			// Skip lines before scroll offset
			if currentDisplayRow < scrollDisplayRow {
				currentDisplayRow++
				continue
			}

			// Stop if we've filled the visible area
			if m.height > 0 && len(displayLines) >= m.height {
				break
			}

			// Check if cursor is on this display line
			isCursorDisplayLine := m.focused && logicalRow == m.cursorRow && wrapIdx == m.cursorWrapLine()

			// Calculate cursor grapheme index within this wrapped segment
			// cursorCol is a grapheme index in the full line; subtract the segment's starting grapheme index
			segmentStartGrapheme := 0
			if wrapIdx < len(graphemeStarts) {
				segmentStartGrapheme = graphemeStarts[wrapIdx]
			}
			colInWrap := max(m.cursorCol-segmentStartGrapheme, 0)

			if inVisualMode {
				// Visual mode: use selection rendering
				// For soft-wrapped lines, we need to adjust selection range for the wrapped segment
				renderedLine := m.renderWrappedLineWithSelection(wrappedLine, logicalRow, wrapIdx, colInWrap, isCursorDisplayLine, segmentStartGrapheme)
				displayLines = append(displayLines, renderedLine)
			} else if hasYankHighlight {
				// Yank highlight: brief flash on yanked region
				renderedLine := m.renderLineWithYankHighlight(wrappedLine, logicalRow, wrapIdx, colInWrap, isCursorDisplayLine, segmentStartGrapheme)
				displayLines = append(displayLines, renderedLine)
			} else if isCursorDisplayLine {
				// Normal/Insert mode with cursor on this line
				// Apply syntax highlighting as base layer, then cursor on top
				displayLines = append(displayLines, m.renderLineWithSyntaxAndCursor(wrappedLine, logicalRow, wrapIdx, colInWrap))
			} else {
				// No cursor, no selection - apply syntax highlighting only
				if wrappedLine == "" {
					displayLines = append(displayLines, " ")
				} else {
					styledLine := m.applySyntaxToSegment(wrappedLine, logicalRow, wrapIdx, segmentStartGrapheme)
					displayLines = append(displayLines, styledLine)
				}
			}
			currentDisplayRow++
		}

		// Stop if we've filled the visible area
		if m.height > 0 && len(displayLines) >= m.height {
			break
		}
	}

	return strings.Join(displayLines, "\n")
}

// renderWrappedLineWithSelection renders a wrapped line segment with selection highlighting.
// This handles the soft-wrap case by mapping selection columns to the wrapped segment.
// Layer order: syntax highlighting (base) → selection (background) → cursor (reverse video)
// segmentStartGrapheme is the grapheme index where this wrapped segment begins in the full line.
// cursorColInWrap and selection bounds are all grapheme indices relative to the segment.
func (m Model) renderWrappedLineWithSelection(wrappedLine string, logicalRow int, wrapIdx int, cursorColInWrap int, isCursorDisplayLine bool, segmentStartGrapheme int) string {
	// Get selection range for the full logical row (grapheme indices)
	startCol, endCol, inSelection := m.getSelectionRangeForRow(logicalRow)

	if !inSelection {
		// Not in selection - render with syntax highlighting and cursor
		if isCursorDisplayLine {
			return m.renderLineWithSyntaxAndCursor(wrappedLine, logicalRow, wrapIdx, cursorColInWrap)
		}
		if wrappedLine == "" {
			return " "
		}
		return m.applySyntaxToSegment(wrappedLine, logicalRow, wrapIdx, segmentStartGrapheme)
	}

	// Calculate grapheme count for this segment
	segmentGraphemeCount := GraphemeCount(wrappedLine)

	// Map selection to this wrapped segment (grapheme indices relative to segment start)
	segmentSelStart := max(startCol-segmentStartGrapheme, 0)
	segmentSelEnd := min(endCol-segmentStartGrapheme, segmentGraphemeCount)

	// Check if selection overlaps with this wrapped segment
	if segmentSelEnd <= 0 || segmentSelStart >= segmentGraphemeCount {
		// No overlap - render with syntax highlighting and cursor
		if isCursorDisplayLine {
			return m.renderLineWithSyntaxAndCursor(wrappedLine, logicalRow, wrapIdx, cursorColInWrap)
		}
		if wrappedLine == "" {
			return " "
		}
		return m.applySyntaxToSegment(wrappedLine, logicalRow, wrapIdx, segmentStartGrapheme)
	}

	// Handle empty wrapped line with selection
	if wrappedLine == "" {
		if isCursorDisplayLine {
			return cursorOn + " " + cursorOff
		}
		return selectionOn + " " + selectionOff
	}

	// Clamp segment selection bounds
	segmentSelStart = max(segmentSelStart, 0)
	segmentSelEnd = min(segmentSelEnd, segmentGraphemeCount)

	// Build byte-to-style map for syntax highlighting on non-selected parts
	var byteStyles map[int]*lipgloss.Style
	if m.lexer != nil {
		fullLine := ""
		if logicalRow < len(m.content) {
			fullLine = m.content[logicalRow]
		}
		if fullLine != "" {
			tokens := m.lexer.Tokenize(fullLine)
			if len(tokens) > 0 {
				segmentStartByte := GraphemeToByteOffset(fullLine, segmentStartGrapheme)
				segmentTokens := m.mapTokensToSegment(tokens, segmentStartByte, len(wrappedLine))
				byteStyles = make(map[int]*lipgloss.Style)
				for _, tok := range segmentTokens {
					for bytePos := tok.Start; bytePos < tok.End && bytePos < len(wrappedLine); bytePos++ {
						style := tok.Style
						byteStyles[bytePos] = &style
					}
				}
			}
		}
	}

	// Build output by iterating through graphemes and batching consecutive selections
	var result strings.Builder
	var selectionBuffer strings.Builder
	inSelectionRun := false
	iter := NewGraphemeIterator(wrappedLine)

	for iter.Next() {
		graphemeIdx := iter.Index()
		cluster := iter.Cluster()
		bytePos := iter.BytePos()

		// Determine if this grapheme is in selection
		isSelected := graphemeIdx >= segmentSelStart && graphemeIdx < segmentSelEnd
		// Determine if cursor is on this grapheme
		isCursor := isCursorDisplayLine && graphemeIdx == cursorColInWrap

		if isCursor {
			// Flush any pending selection
			if inSelectionRun {
				result.WriteString(selectionOn)
				result.WriteString(selectionBuffer.String())
				result.WriteString(selectionOff)
				selectionBuffer.Reset()
				inSelectionRun = false
			}
			// Cursor takes precedence - use reverse video
			result.WriteString(cursorOn)
			result.WriteString(cluster)
			result.WriteString(cursorOff)
		} else if isSelected {
			// Batch consecutive selected graphemes
			selectionBuffer.WriteString(cluster)
			inSelectionRun = true
		} else {
			// Flush any pending selection
			if inSelectionRun {
				result.WriteString(selectionOn)
				result.WriteString(selectionBuffer.String())
				result.WriteString(selectionOff)
				selectionBuffer.Reset()
				inSelectionRun = false
			}
			// Not selected, not cursor - apply syntax highlighting if available
			if style, hasStyle := byteStyles[bytePos]; hasStyle {
				result.WriteString(style.Render(cluster))
			} else {
				result.WriteString(cluster)
			}
		}
	}

	// Flush any remaining selection
	if inSelectionRun {
		result.WriteString(selectionOn)
		result.WriteString(selectionBuffer.String())
		result.WriteString(selectionOff)
	}

	return result.String()
}

// wrapLine splits a line into segments that fit within the display width.
// This function iterates by grapheme cluster and breaks at grapheme boundaries
// to ensure that emoji and other multi-byte characters are never split.
// m.width is in display columns (terminal cells), not bytes or graphemes.

// wrapLineWithInfo wraps a line and returns both the wrapped segments and
// the starting grapheme index of each segment. This is used for cursor and
// selection positioning in wrapped lines.
func (m Model) wrapLineWithInfo(line string) ([]string, []int) {
	if m.width <= 0 || len(line) == 0 {
		return []string{line}, []int{0}
	}

	var wrapped []string
	var graphemeStarts []int
	var current strings.Builder
	currentWidth := 0
	segmentStartGrapheme := 0
	currentGrapheme := 0

	iter := NewGraphemeIterator(line)
	for iter.Next() {
		clusterWidth := GraphemeDisplayWidth(iter.Cluster())
		// If adding this grapheme would exceed width, wrap
		if currentWidth+clusterWidth > m.width && currentWidth > 0 {
			wrapped = append(wrapped, current.String())
			graphemeStarts = append(graphemeStarts, segmentStartGrapheme)
			current.Reset()
			currentWidth = 0
			segmentStartGrapheme = currentGrapheme
		}
		current.WriteString(iter.Cluster())
		currentWidth += clusterWidth
		currentGrapheme++
	}

	// Add the remaining content (or empty string if line was empty)
	if current.Len() > 0 || len(wrapped) == 0 {
		wrapped = append(wrapped, current.String())
		graphemeStarts = append(graphemeStarts, segmentStartGrapheme)
	}

	return wrapped, graphemeStarts
}

// renderEmpty renders the view when content is empty.
func (m Model) renderEmpty() string {
	// Show cursor if focused
	if m.focused {
		return cursorOn + " " + cursorOff
	}

	// Show placeholder if set
	if m.config.Placeholder != "" {
		return placeholderStyle.Render(m.config.Placeholder)
	}

	return ""
}

// renderLineWithCursor renders a single line with the cursor at the specified column.
// This version does NOT apply syntax highlighting - used as fallback.
// col is a grapheme index, not a byte offset.
func (m Model) renderLineWithCursor(line string, col int) string {
	// If line is empty, show cursor as a space
	if line == "" {
		return cursorOn + " " + cursorOff
	}

	graphemeCount := GraphemeCount(line)

	// In Insert mode, cursor can be at grapheme count (after the last character)
	// In Normal mode, cursor is clamped to grapheme count - 1 (on the last character)
	if col >= graphemeCount {
		// Cursor is at end - append cursor block
		return line + cursorOn + " " + cursorOff
	}

	// Cursor is within the line - highlight the grapheme cluster under cursor
	var result strings.Builder
	iter := NewGraphemeIterator(line)
	for iter.Next() {
		if iter.Index() == col {
			result.WriteString(cursorOn)
			result.WriteString(iter.Cluster())
			result.WriteString(cursorOff)
		} else {
			result.WriteString(iter.Cluster())
		}
	}
	return result.String()
}

// renderLineWithSyntaxAndCursor renders a line with syntax highlighting as the base layer
// and cursor overlay on top. The cursor uses reverse video which overrides syntax colors.
// cursorColInWrap is a grapheme index within the wrapped segment.
func (m Model) renderLineWithSyntaxAndCursor(segment string, logicalRow int, wrapIdx int, cursorColInWrap int) string {
	// If segment is empty, show cursor as a space
	if segment == "" {
		return cursorOn + " " + cursorOff
	}

	// If no lexer, use simple cursor rendering
	if m.lexer == nil {
		return m.renderLineWithCursor(segment, cursorColInWrap)
	}

	segmentGraphemeCount := GraphemeCount(segment)

	// Get the full logical line for tokenization
	fullLine := ""
	if logicalRow < len(m.content) {
		fullLine = m.content[logicalRow]
	}
	if fullLine == "" {
		return m.renderLineWithCursor(segment, cursorColInWrap)
	}

	// Tokenize the full logical line (tokens use byte offsets)
	tokens := m.lexer.Tokenize(fullLine)
	if len(tokens) == 0 {
		return m.renderLineWithCursor(segment, cursorColInWrap)
	}

	// Calculate the byte offset where this segment starts in the full line
	// We need segmentStartGrapheme which we don't have in this function signature
	// For now, calculate it from wrapIdx
	segmentStartGrapheme := wrapIdx * m.width
	segmentStartByte := GraphemeToByteOffset(fullLine, segmentStartGrapheme)

	// Map tokens to this segment's byte coordinates
	segmentTokens := m.mapTokensToSegment(tokens, segmentStartByte, len(segment))

	// Create a map of byte positions to their styles
	byteStyles := make(map[int]*lipgloss.Style)
	for _, tok := range segmentTokens {
		for bytePos := tok.Start; bytePos < tok.End && bytePos < len(segment); bytePos++ {
			style := tok.Style
			byteStyles[bytePos] = &style
		}
	}

	// Build output by iterating through graphemes
	var result strings.Builder
	iter := NewGraphemeIterator(segment)

	for iter.Next() {
		graphemeIdx := iter.Index()
		cluster := iter.Cluster()
		bytePos := iter.BytePos()

		// Determine if cursor is on this grapheme
		isCursor := graphemeIdx == cursorColInWrap

		if isCursor {
			// Cursor takes precedence - use reverse video
			result.WriteString(cursorOn)
			result.WriteString(cluster)
			result.WriteString(cursorOff)
		} else {
			// Apply syntax highlighting if available for this byte position
			if style, hasStyle := byteStyles[bytePos]; hasStyle {
				result.WriteString(style.Render(cluster))
			} else {
				result.WriteString(cluster)
			}
		}
	}

	// Cursor at end of line
	if cursorColInWrap >= segmentGraphemeCount {
		result.WriteString(cursorOn + " " + cursorOff)
	}

	return result.String()
}

// renderLineWithYankHighlight renders a line with yank highlight overlay.
// The yank highlight takes precedence over normal rendering but still shows cursor.
// Layer order: syntax highlighting (base) → yank highlight (background) → cursor (reverse video)
// segmentStartGrapheme is the grapheme index where this wrapped segment begins in the full line.
func (m Model) renderLineWithYankHighlight(wrappedLine string, logicalRow int, wrapIdx int, cursorColInWrap int, isCursorDisplayLine bool, segmentStartGrapheme int) string {
	// Get yank highlight range for this row (grapheme indices)
	startCol, endCol, inHighlight := m.getYankHighlightRangeForRow(logicalRow)

	if !inHighlight {
		// Not in highlight - render with syntax highlighting and cursor
		if isCursorDisplayLine {
			return m.renderLineWithSyntaxAndCursor(wrappedLine, logicalRow, wrapIdx, cursorColInWrap)
		}
		if wrappedLine == "" {
			return " "
		}
		return m.applySyntaxToSegment(wrappedLine, logicalRow, wrapIdx, segmentStartGrapheme)
	}

	// Calculate grapheme count for this segment
	segmentGraphemeCount := GraphemeCount(wrappedLine)

	// Map highlight to this wrapped segment (grapheme indices relative to segment start)
	segmentHighlightStart := max(startCol-segmentStartGrapheme, 0)
	segmentHighlightEnd := min(endCol-segmentStartGrapheme, segmentGraphemeCount)

	// Check if highlight overlaps with this wrapped segment
	if segmentHighlightEnd <= 0 || segmentHighlightStart >= segmentGraphemeCount {
		// No overlap - render with syntax highlighting and cursor
		if isCursorDisplayLine {
			return m.renderLineWithSyntaxAndCursor(wrappedLine, logicalRow, wrapIdx, cursorColInWrap)
		}
		if wrappedLine == "" {
			return " "
		}
		return m.applySyntaxToSegment(wrappedLine, logicalRow, wrapIdx, segmentStartGrapheme)
	}

	// Handle empty wrapped line with highlight
	if wrappedLine == "" {
		if isCursorDisplayLine {
			return cursorOn + " " + cursorOff
		}
		return yankHighlightOn + " " + yankHighlightOff
	}

	// Clamp segment highlight bounds
	segmentHighlightStart = max(segmentHighlightStart, 0)
	segmentHighlightEnd = min(segmentHighlightEnd, segmentGraphemeCount)

	// Build output by iterating through graphemes and batching consecutive highlights
	var result strings.Builder
	var highlightBuffer strings.Builder
	inHighlightRun := false
	iter := NewGraphemeIterator(wrappedLine)

	for iter.Next() {
		graphemeIdx := iter.Index()
		cluster := iter.Cluster()

		// Determine if this grapheme is in highlight
		isHighlighted := graphemeIdx >= segmentHighlightStart && graphemeIdx < segmentHighlightEnd
		// Determine if cursor is on this grapheme
		isCursor := isCursorDisplayLine && graphemeIdx == cursorColInWrap

		if isCursor {
			// Flush any pending highlight
			if inHighlightRun {
				result.WriteString(yankHighlightOn)
				result.WriteString(highlightBuffer.String())
				result.WriteString(yankHighlightOff)
				highlightBuffer.Reset()
				inHighlightRun = false
			}
			// Cursor takes precedence - use reverse video
			result.WriteString(cursorOn)
			result.WriteString(cluster)
			result.WriteString(cursorOff)
		} else if isHighlighted {
			// Batch consecutive highlighted graphemes
			highlightBuffer.WriteString(cluster)
			inHighlightRun = true
		} else {
			// Flush any pending highlight
			if inHighlightRun {
				result.WriteString(yankHighlightOn)
				result.WriteString(highlightBuffer.String())
				result.WriteString(yankHighlightOff)
				highlightBuffer.Reset()
				inHighlightRun = false
			}
			// Not highlighted, not cursor - just output the grapheme
			result.WriteString(cluster)
		}
	}

	// Flush any remaining highlight
	if inHighlightRun {
		result.WriteString(yankHighlightOn)
		result.WriteString(highlightBuffer.String())
		result.WriteString(yankHighlightOff)
	}

	// Handle cursor at end of line
	if isCursorDisplayLine && cursorColInWrap >= segmentGraphemeCount {
		result.WriteString(cursorOn + " " + cursorOff)
	}

	return result.String()
}

// getYankHighlightRangeForRow returns the column range to highlight on a given row.
// Returns (startCol, endCol, inHighlight) where endCol is exclusive.
// Column values are grapheme indices, not byte offsets.
func (m Model) getYankHighlightRangeForRow(row int) (startCol, endCol int, inHighlight bool) {
	if m.yankHighlight == nil {
		return 0, 0, false
	}

	hl := m.yankHighlight
	start, end := hl.Start, hl.End

	// Check if row is in highlight range
	if row < start.Row || row > end.Row {
		return 0, 0, false
	}

	// Handle empty content case
	if row >= len(m.content) {
		return 0, 0, false
	}

	line := m.content[row]
	lineGraphemeCount := GraphemeCount(line)

	if hl.Linewise {
		// Line-wise: entire line is highlighted (grapheme count)
		return 0, lineGraphemeCount, true
	}

	// Character-wise highlight (grapheme indices)
	if row == start.Row && row == end.Row {
		// Single line: start.Col to end.Col+1 (exclusive)
		return start.Col, min(end.Col+1, lineGraphemeCount), true
	} else if row == start.Row {
		// First line of multi-line: start.Col to end of line
		return start.Col, lineGraphemeCount, true
	} else if row == end.Row {
		// Last line of multi-line: start of line to end.Col+1 (exclusive)
		return 0, min(end.Col+1, lineGraphemeCount), true
	} else {
		// Middle line: entire line
		return 0, lineGraphemeCount, true
	}
}

// isEmpty returns true if the content is empty (single empty line).
func (m Model) isEmpty() bool {
	return len(m.content) == 1 && m.content[0] == ""
}

// ============================================================================
// Soft-wrap helpers
// ============================================================================

// displayLinesForLine returns how many display lines a logical line takes when wrapped.
// A line of width 0 or empty line takes 1 display line.
// This uses display width calculation to account for emoji and CJK characters.
func (m Model) displayLinesForLine(line string) int {
	if m.width <= 0 || len(line) == 0 {
		return 1
	}
	// Use display width for accurate calculation with emoji/CJK
	displayWidth := StringDisplayWidth(line)
	if displayWidth == 0 {
		return 1
	}
	// Ceiling division: (displayWidth + width - 1) / width
	return (displayWidth + m.width - 1) / m.width
}

// totalDisplayLines returns the total number of display lines for all content.
func (m Model) totalDisplayLines() int {
	return m.TotalDisplayLines()
}

// TotalDisplayLines returns the total number of display lines for all content,
// accounting for soft-wrap based on the current width.
func (m Model) TotalDisplayLines() int {
	total := 0
	for _, line := range m.content {
		total += m.displayLinesForLine(line)
	}
	return total
}

// cursorDisplayRow returns which display row the cursor is on (0-indexed).
// This accounts for wrapped lines above the cursor row and uses display column
// calculation for correct positioning with emoji and wide characters.
func (m Model) cursorDisplayRow() int {
	displayRow := 0
	// Count display lines for all rows before cursor row
	for i := 0; i < m.cursorRow; i++ {
		displayRow += m.displayLinesForLine(m.content[i])
	}
	// Add the wrapped line offset within the current row
	// This uses cursorWrapLine() which converts grapheme index to display column
	displayRow += m.cursorWrapLine()
	return displayRow
}

// cursorWrapLine returns which wrapped line within the current row the cursor is on (0-indexed).
// cursorCol is a grapheme index, so we need to calculate the display column
// (sum of display widths of graphemes before cursor) and divide by m.width.
func (m Model) cursorWrapLine() int {
	if m.width <= 0 {
		return 0
	}

	// Get the current line
	if m.cursorRow >= len(m.content) {
		return 0
	}
	line := m.content[m.cursorRow]

	// Calculate display column for cursor position
	// cursorCol is a grapheme index, we need to sum display widths of graphemes before cursor
	displayCol := 0
	idx := 0
	iter := NewGraphemeIterator(line)
	for iter.Next() {
		if idx >= m.cursorCol {
			break
		}
		displayCol += GraphemeDisplayWidth(iter.Cluster())
		idx++
	}

	return displayCol / m.width
}

// computeDisplayScrollOffset returns the scroll offset in display rows that ensures cursor is visible.
// This is a pure function that doesn't modify the model.
func (m Model) computeDisplayScrollOffset() int {
	// If no height restriction, no scrolling needed
	if m.height <= 0 {
		return 0
	}

	cursorDisplayRow := m.cursorDisplayRow()
	scrollOffset := m.scrollOffset // scrollOffset is now in display rows

	// If cursor is above visible area, scroll up
	scrollOffset = min(scrollOffset, cursorDisplayRow)

	// If cursor is below visible area, scroll down
	if cursorDisplayRow >= scrollOffset+m.height {
		scrollOffset = cursorDisplayRow - m.height + 1
	}

	// Clamp scroll offset to valid range
	totalDisplay := m.totalDisplayLines()
	maxOffset := max(totalDisplay-m.height, 0)
	scrollOffset = min(scrollOffset, maxOffset)
	scrollOffset = max(scrollOffset, 0)

	return scrollOffset
}

// ensureCursorVisible adjusts scrollOffset to ensure the cursor is visible.
// scrollOffset is stored in display rows to support soft-wrap scrolling.
func (m *Model) ensureCursorVisible() {
	// If no height restriction, no scrolling needed
	if m.height <= 0 {
		m.scrollOffset = 0
		return
	}

	cursorDisplayRow := m.cursorDisplayRow()

	// If cursor is above visible area, scroll up
	m.scrollOffset = min(m.scrollOffset, cursorDisplayRow)

	// If cursor is below visible area, scroll down
	if cursorDisplayRow >= m.scrollOffset+m.height {
		m.scrollOffset = cursorDisplayRow - m.height + 1
	}

	// Clamp scroll offset to valid range
	totalDisplay := m.totalDisplayLines()
	maxOffset := max(totalDisplay-m.height, 0)
	m.scrollOffset = min(m.scrollOffset, maxOffset)
	m.scrollOffset = max(m.scrollOffset, 0)
}

// ScrollOffset returns the current scroll offset (first visible line).
func (m Model) ScrollOffset() int {
	return m.scrollOffset
}

// SetScrollOffset sets the scroll offset (first visible line).
func (m *Model) SetScrollOffset(offset int) {
	m.scrollOffset = offset
	m.ensureCursorVisible()
}

// ============================================================================
// Syntax Highlighting
// ============================================================================

// mapTokensToSegment maps tokens from logical line coordinates to wrapped segment coordinates.
// This handles soft-wrap by offsetting token positions and filtering tokens that don't overlap.
func (m Model) mapTokensToSegment(tokens []SyntaxToken, wrapStartCol, segmentLen int) []SyntaxToken {
	if len(tokens) == 0 || segmentLen == 0 {
		return nil
	}

	wrapEndCol := wrapStartCol + segmentLen
	var result []SyntaxToken

	for _, tok := range tokens {
		// Skip tokens that don't overlap with this segment
		if tok.End <= wrapStartCol || tok.Start >= wrapEndCol {
			continue
		}

		// Map token to segment coordinates
		mappedStart := max(tok.Start-wrapStartCol, 0)
		mappedEnd := min(tok.End-wrapStartCol, segmentLen)

		result = append(result, SyntaxToken{
			Start: mappedStart,
			End:   mappedEnd,
			Style: tok.Style,
		})
	}

	return result
}

// applySyntaxToSegment applies syntax highlighting to a wrapped segment.
// It tokenizes the full logical line and maps tokens to the segment.
// segmentStartGrapheme is the grapheme index where this wrapped segment begins in the full line.
// Note: Syntax highlighting is temporarily simplified for grapheme-aware rendering.
// The lexer returns byte-based tokens which require translation via ByteToGraphemeOffset().
func (m Model) applySyntaxToSegment(segment string, logicalRow int, _ int, segmentStartGrapheme int) string {
	if m.lexer == nil || segment == "" {
		return segment
	}

	// Get the full logical line for tokenization
	fullLine := ""
	if logicalRow < len(m.content) {
		fullLine = m.content[logicalRow]
	}
	if fullLine == "" {
		return segment
	}

	// Tokenize the full logical line (tokens use byte offsets)
	tokens := m.lexer.Tokenize(fullLine)
	if len(tokens) == 0 {
		return segment
	}

	// Calculate the byte offset where this segment starts in the full line
	segmentStartByte := GraphemeToByteOffset(fullLine, segmentStartGrapheme)
	segmentEndByte := segmentStartByte + len(segment)

	// Map tokens to this segment's byte coordinates
	segmentTokens := m.mapTokensToSegment(tokens, segmentStartByte, len(segment))
	if len(segmentTokens) == 0 {
		return segment
	}

	// Build the styled string using byte-based positions
	// Note: This still uses byte positions which works because the segment string
	// is extracted by bytes. The tokens map byte offsets within this segment.
	var result strings.Builder
	pos := 0

	for _, tok := range segmentTokens {
		// Gap before token (plain text)
		if tok.Start > pos {
			result.WriteString(segment[pos:tok.Start])
		}

		// Token with style (foreground color only)
		endPos := min(tok.End, len(segment))
		result.WriteString(tok.Style.Render(segment[tok.Start:endPos]))
		pos = endPos
	}

	// Remainder after last token
	if pos < len(segment) {
		result.WriteString(segment[pos:])
	}

	// Suppress unused variable warning
	_ = segmentEndByte

	return result.String()
}
