package vimtextarea

import "strings"

// ============================================================================
// Visual Mode Operation Commands
// These commands perform operations on visually selected text.
// ============================================================================

// VisualDeleteCommand deletes the selected text in visual modes ('d' or 'x').
// After deletion, exits to normal mode with cursor at start of deleted region.
type VisualDeleteCommand struct {
	DeleteBase
	deletedContent []string // Captured for undo
	anchorPos      Position // Anchor position at time of delete
	cursorPos      Position // Cursor position at time of delete
	wasLinewise    bool     // Whether it was line-wise selection
	startPos       Position // Normalized start position (for undo cursor placement)
	mode           Mode     // Which mode this command operates in
	executed       bool     // True after first execution (for redo detection)
}

// Execute deletes the selected text and exits to normal mode.
// On redo (when executed flag is set), re-applies the deletion
// using captured state without requiring visual mode.
func (c *VisualDeleteCommand) Execute(m *Model) ExecuteResult {
	// Check if this is a redo (command was already executed, so has captured state)
	if c.executed {
		// Redo: re-apply deletion using captured state
		// Position cursor at startPos and delete the same content
		if c.startPos.Row >= len(m.content) {
			return Skipped
		}

		// Re-delete using the captured start position and size
		m.deleteSelection(c.startPos, c.computeEndPos(), c.wasLinewise)

		// Ensure we're in normal mode
		m.mode = ModeNormal
		m.visualAnchor = Position{}
		return Executed
	}

	// First execution: only operate in visual modes
	if m.mode != ModeVisual && m.mode != ModeVisualLine {
		return Skipped
	}

	// Capture state for undo
	c.anchorPos = m.visualAnchor
	c.cursorPos = Position{Row: m.cursorRow, Col: m.cursorCol}
	c.wasLinewise = m.mode == ModeVisualLine

	// Get normalized selection bounds
	start, end := m.SelectionBounds()
	c.startPos = start

	// Validate bounds
	if start.Row >= len(m.content) || end.Row >= len(m.content) {
		// Invalid bounds - exit visual mode without deleting
		m.mode = ModeNormal
		m.visualAnchor = Position{}
		return Skipped
	}

	// Delete the selection
	c.deletedContent = m.deleteSelection(start, end, c.wasLinewise)

	// Populate yank register (vim behavior: deletes also yank)
	m.lastYankedText = strings.Join(c.deletedContent, "\n")
	m.lastYankWasLinewise = c.wasLinewise

	// Mark as executed for redo detection
	c.executed = true

	// Exit visual mode
	m.mode = ModeNormal
	m.visualAnchor = Position{}
	m.pendingBuilder.Clear()

	return Executed
}

// computeEndPos calculates the end position from captured state for redo.
// Note: Position.Col values are grapheme indices, not byte offsets.
func (c *VisualDeleteCommand) computeEndPos() Position {
	if c.wasLinewise {
		// For linewise, end row is start row + number of lines - 1
		numLines := len(c.deletedContent)
		endRow := c.startPos.Row + numLines - 1
		return Position{Row: endRow, Col: 0}
	}

	// For character-wise, calculate from deleted content using grapheme count
	if len(c.deletedContent) == 1 {
		// Single line deletion - use grapheme count
		return Position{Row: c.startPos.Row, Col: c.startPos.Col + GraphemeCount(c.deletedContent[0]) - 1}
	}

	// Multi-line deletion - use grapheme count for last line
	numLines := len(c.deletedContent)
	endRow := c.startPos.Row + numLines - 1
	lastLineGraphemeCount := GraphemeCount(c.deletedContent[numLines-1])
	return Position{Row: endRow, Col: lastLineGraphemeCount - 1}
}

// Undo restores the deleted content at the original position.
// Note: c.startPos.Col is a grapheme index, so we use grapheme-aware slicing.
func (c *VisualDeleteCommand) Undo(m *Model) error {
	if len(c.deletedContent) == 0 {
		return nil
	}

	if c.wasLinewise {
		// Line-wise undo: insert lines back at startPos.Row
		insertRow := min(c.startPos.Row, len(m.content))

		// Check if we're restoring to an empty buffer that was completely deleted
		if len(m.content) == 1 && m.content[0] == "" && insertRow == 0 {
			// Replace empty line with first deleted line, then insert rest
			m.content[0] = c.deletedContent[0]
			if len(c.deletedContent) > 1 {
				newContent := make([]string, len(c.deletedContent))
				copy(newContent, c.deletedContent)
				m.content = newContent
			}
		} else {
			// Insert deleted lines
			newContent := make([]string, 0, len(m.content)+len(c.deletedContent))
			newContent = append(newContent, m.content[:insertRow]...)
			newContent = append(newContent, c.deletedContent...)
			newContent = append(newContent, m.content[insertRow:]...)
			m.content = newContent
		}

		// Position cursor at start of restored region
		m.cursorRow = c.startPos.Row
		m.cursorCol = 0
	} else {
		// Character-wise undo using grapheme-aware slicing
		if len(c.deletedContent) == 1 {
			// Single line restore
			line := m.content[c.startPos.Row]
			lineGraphemeCount := GraphemeCount(line)
			// Use grapheme-aware slicing since startPos.Col is a grapheme index
			prefix := SliceByGraphemes(line, 0, c.startPos.Col)
			suffix := SliceByGraphemes(line, c.startPos.Col, lineGraphemeCount)
			restored := prefix + c.deletedContent[0] + suffix
			m.content[c.startPos.Row] = restored
		} else {
			// Multi-line restore
			// The current state has: prefix + suffix joined at startPos.Row
			currentLine := m.content[c.startPos.Row]
			currentLineGraphemeCount := GraphemeCount(currentLine)
			prefix := SliceByGraphemes(currentLine, 0, c.startPos.Col)
			suffix := SliceByGraphemes(currentLine, c.startPos.Col, currentLineGraphemeCount)

			// Calculate where the suffix actually starts
			// The last deleted line piece joined with suffix
			lastDeletedLine := c.deletedContent[len(c.deletedContent)-1]

			// Build restored content
			newContent := make([]string, 0, len(m.content)+len(c.deletedContent)-1)

			// Lines before the start row
			newContent = append(newContent, m.content[:c.startPos.Row]...)

			// First line: prefix + first deleted piece
			newContent = append(newContent, prefix+c.deletedContent[0])

			// Middle lines: full deleted lines
			for i := 1; i < len(c.deletedContent)-1; i++ {
				newContent = append(newContent, c.deletedContent[i])
			}

			// Last line: last deleted piece + suffix
			newContent = append(newContent, lastDeletedLine+suffix)

			// Lines after the end row (in the current content, that's startPos.Row+1)
			if c.startPos.Row+1 < len(m.content) {
				newContent = append(newContent, m.content[c.startPos.Row+1:]...)
			}

			m.content = newContent
		}

		// Position cursor at start of restored region (grapheme index)
		m.cursorRow = c.startPos.Row
		m.cursorCol = c.startPos.Col
	}

	// Ensure we're in normal mode after undo
	m.mode = ModeNormal
	m.visualAnchor = Position{}

	return nil
}

// Keys returns the trigger keys for this command.
func (c *VisualDeleteCommand) Keys() []string {
	return []string{"d", "x"}
}

// Mode returns the mode this command operates in.
func (c *VisualDeleteCommand) Mode() Mode {
	return c.mode
}

// ID returns the hierarchical identifier for this command.
func (c *VisualDeleteCommand) ID() string {
	return "visual.delete"
}

// String representation for debugging
func (c *VisualDeleteCommand) String() string {
	return "VisualDeleteCommand{deleted:" + strings.Join(c.deletedContent, "\\n") + "}"
}

// ============================================================================
// VisualYankCommand
// ============================================================================

// VisualYankCommand copies the selected text in visual modes ('y').
// After yanking, exits to normal mode with cursor staying at current position.
// This command is NOT undoable since it doesn't modify content.
type VisualYankCommand struct {
	MotionBase          // Not undoable, doesn't change content, doesn't change mode (base methods)
	mode         Mode   // Which mode this command operates in
	isModeChange bool   // Set to true after execute to indicate mode change
	yankedText   string // Text that was yanked (for debugging/inspection)
	wasLinewise  bool   // Whether it was line-wise selection

	// Highlight positions captured during execute
	highlightStart Position
	highlightEnd   Position
	showHighlight  bool
}

// Execute copies the selected text and exits to normal mode.
// Cursor stays at its current position after yank.
func (c *VisualYankCommand) Execute(m *Model) ExecuteResult {
	// Only operate in visual modes
	if m.mode != ModeVisual && m.mode != ModeVisualLine {
		return Skipped
	}

	// Get the selected text
	selectedText := m.SelectedText()
	if selectedText == "" {
		// Empty selection - still exit visual mode
		m.mode = ModeNormal
		m.visualAnchor = Position{}
		m.pendingBuilder.Clear()
		c.isModeChange = true
		c.showHighlight = false
		return Executed
	}

	// Store the yanked text in the model
	m.lastYankedText = selectedText
	c.wasLinewise = m.mode == ModeVisualLine
	m.lastYankWasLinewise = c.wasLinewise
	c.yankedText = selectedText

	// Capture selection bounds for highlight
	start, end := m.SelectionBounds()
	c.highlightStart = start
	c.highlightEnd = end
	c.showHighlight = true

	// Exit visual mode - cursor stays at current position
	m.mode = ModeNormal
	m.visualAnchor = Position{}
	m.pendingBuilder.Clear()
	c.isModeChange = true

	return Executed
}

// YankHighlightRegion returns the region to highlight after yank.
func (c *VisualYankCommand) YankHighlightRegion() (start, end Position, linewise bool, show bool) {
	return c.highlightStart, c.highlightEnd, c.wasLinewise, c.showHighlight
}

// Undo is a no-op since yank doesn't change content.
func (c *VisualYankCommand) Undo(m *Model) error {
	return nil
}

// Keys returns the trigger key for this command.
func (c *VisualYankCommand) Keys() []string {
	return []string{"y"}
}

// Mode returns the mode this command operates in.
func (c *VisualYankCommand) Mode() Mode {
	return c.mode
}

// ID returns the hierarchical identifier for this command.
func (c *VisualYankCommand) ID() string {
	return "visual.yank"
}

// IsModeChange returns true after execution since yank exits visual mode.
// Note: This overrides MotionBase.IsModeChange() which returns false.
func (c *VisualYankCommand) IsModeChange() bool {
	return c.isModeChange
}

// IsUndoable returns false since yank doesn't modify content.
func (c *VisualYankCommand) IsUndoable() bool {
	return false
}

// ChangesContent returns false since yank doesn't modify content.
func (c *VisualYankCommand) ChangesContent() bool {
	return false
}

// String representation for debugging
func (c *VisualYankCommand) String() string {
	return "VisualYankCommand{yanked:" + c.yankedText + "}"
}

// ============================================================================
// VisualChangeCommand
// ============================================================================

// VisualChangeCommand deletes the selected text and enters insert mode ('c').
// After deletion, enters insert mode with cursor at start of deleted region.
// This is essentially a delete followed by entering insert mode.
type VisualChangeCommand struct {
	ChangeBase
	deletedContent []string // Captured for undo
	anchorPos      Position // Anchor position at time of change
	cursorPos      Position // Cursor position at time of change
	wasLinewise    bool     // Whether it was line-wise selection
	startPos       Position // Normalized start position (for undo cursor placement)
	mode           Mode     // Which mode this command operates in
	executed       bool     // True after first execution (for redo detection)
}

// Execute deletes the selected text and enters insert mode.
// On redo (when executed flag is set), re-applies the deletion
// using captured state without requiring visual mode.
func (c *VisualChangeCommand) Execute(m *Model) ExecuteResult {
	// Check if this is a redo (command was already executed, so has captured state)
	if c.executed {
		// Redo: re-apply deletion using captured state
		// Position cursor at startPos and delete the same content
		if c.startPos.Row >= len(m.content) {
			return Skipped
		}

		// Re-delete using the captured start position and size
		m.deleteSelection(c.startPos, c.computeEndPos(), c.wasLinewise)

		// Enter insert mode
		m.mode = ModeInsert
		m.visualAnchor = Position{}
		return Executed
	}

	// First execution: only operate in visual modes
	if m.mode != ModeVisual && m.mode != ModeVisualLine {
		return Skipped
	}

	// Capture state for undo
	c.anchorPos = m.visualAnchor
	c.cursorPos = Position{Row: m.cursorRow, Col: m.cursorCol}
	c.wasLinewise = m.mode == ModeVisualLine

	// Get normalized selection bounds
	start, end := m.SelectionBounds()
	c.startPos = start

	// Validate bounds
	if start.Row >= len(m.content) || end.Row >= len(m.content) {
		// Invalid bounds - exit visual mode without changing
		m.mode = ModeNormal
		m.visualAnchor = Position{}
		return Skipped
	}

	// Delete the selection
	c.deletedContent = m.deleteSelection(start, end, c.wasLinewise)

	// Mark as executed for redo detection
	c.executed = true

	// Exit visual mode and enter insert mode
	m.mode = ModeInsert
	m.visualAnchor = Position{}
	m.pendingBuilder.Clear()

	// For insert mode, cursor should be at start.Col (not clamped for normal mode)
	// deleteSelection clamps cursor for normal mode, but insert mode allows cursor at end of line
	m.cursorRow = start.Row
	m.cursorCol = start.Col

	return Executed
}

// computeEndPos calculates the end position from captured state for redo.
// Note: Position.Col values are grapheme indices, not byte offsets.
func (c *VisualChangeCommand) computeEndPos() Position {
	if c.wasLinewise {
		// For linewise, end row is start row + number of lines - 1
		numLines := len(c.deletedContent)
		endRow := c.startPos.Row + numLines - 1
		return Position{Row: endRow, Col: 0}
	}

	// For character-wise, calculate from deleted content using grapheme count
	if len(c.deletedContent) == 1 {
		// Single line deletion - use grapheme count
		return Position{Row: c.startPos.Row, Col: c.startPos.Col + GraphemeCount(c.deletedContent[0]) - 1}
	}

	// Multi-line deletion - use grapheme count for last line
	numLines := len(c.deletedContent)
	endRow := c.startPos.Row + numLines - 1
	lastLineGraphemeCount := GraphemeCount(c.deletedContent[numLines-1])
	return Position{Row: endRow, Col: lastLineGraphemeCount - 1}
}

// Undo restores the deleted content and returns to normal mode.
// Note: Undo returns to Normal mode, not Insert mode (vim behavior).
// Note: c.startPos.Col is a grapheme index, so we use grapheme-aware slicing.
func (c *VisualChangeCommand) Undo(m *Model) error {
	if len(c.deletedContent) == 0 {
		return nil
	}

	if c.wasLinewise {
		// Line-wise undo: insert lines back at startPos.Row
		insertRow := min(c.startPos.Row, len(m.content))

		// Check if we're restoring to an empty buffer that was completely deleted
		if len(m.content) == 1 && m.content[0] == "" && insertRow == 0 {
			// Replace empty line with first deleted line, then insert rest
			m.content[0] = c.deletedContent[0]
			if len(c.deletedContent) > 1 {
				newContent := make([]string, len(c.deletedContent))
				copy(newContent, c.deletedContent)
				m.content = newContent
			}
		} else {
			// Insert deleted lines
			newContent := make([]string, 0, len(m.content)+len(c.deletedContent))
			newContent = append(newContent, m.content[:insertRow]...)
			newContent = append(newContent, c.deletedContent...)
			newContent = append(newContent, m.content[insertRow:]...)
			m.content = newContent
		}

		// Position cursor at start of restored region
		m.cursorRow = c.startPos.Row
		m.cursorCol = 0
	} else {
		// Character-wise undo using grapheme-aware slicing
		if len(c.deletedContent) == 1 {
			// Single line restore
			line := m.content[c.startPos.Row]
			lineGraphemeCount := GraphemeCount(line)
			// Use grapheme-aware slicing since startPos.Col is a grapheme index
			prefix := SliceByGraphemes(line, 0, c.startPos.Col)
			suffix := SliceByGraphemes(line, c.startPos.Col, lineGraphemeCount)
			restored := prefix + c.deletedContent[0] + suffix
			m.content[c.startPos.Row] = restored
		} else {
			// Multi-line restore
			// The current state has: prefix + suffix joined at startPos.Row
			currentLine := m.content[c.startPos.Row]
			currentLineGraphemeCount := GraphemeCount(currentLine)
			prefix := SliceByGraphemes(currentLine, 0, c.startPos.Col)
			suffix := SliceByGraphemes(currentLine, c.startPos.Col, currentLineGraphemeCount)

			// Calculate where the suffix actually starts
			// The last deleted line piece joined with suffix
			lastDeletedLine := c.deletedContent[len(c.deletedContent)-1]

			// Build restored content
			newContent := make([]string, 0, len(m.content)+len(c.deletedContent)-1)

			// Lines before the start row
			newContent = append(newContent, m.content[:c.startPos.Row]...)

			// First line: prefix + first deleted piece
			newContent = append(newContent, prefix+c.deletedContent[0])

			// Middle lines: full deleted lines
			for i := 1; i < len(c.deletedContent)-1; i++ {
				newContent = append(newContent, c.deletedContent[i])
			}

			// Last line: last deleted piece + suffix
			newContent = append(newContent, lastDeletedLine+suffix)

			// Lines after the end row (in the current content, that's startPos.Row+1)
			if c.startPos.Row+1 < len(m.content) {
				newContent = append(newContent, m.content[c.startPos.Row+1:]...)
			}

			m.content = newContent
		}

		// Position cursor at start of restored region (grapheme index)
		m.cursorRow = c.startPos.Row
		m.cursorCol = c.startPos.Col
	}

	// Ensure we're in normal mode after undo (NOT insert mode)
	m.mode = ModeNormal
	m.visualAnchor = Position{}

	return nil
}

// Keys returns the trigger key for this command.
func (c *VisualChangeCommand) Keys() []string {
	return []string{"c"}
}

// Mode returns the mode this command operates in.
func (c *VisualChangeCommand) Mode() Mode {
	return c.mode
}

// ID returns the hierarchical identifier for this command.
func (c *VisualChangeCommand) ID() string {
	return "visual.change"
}

// String representation for debugging
func (c *VisualChangeCommand) String() string {
	return "VisualChangeCommand{deleted:" + strings.Join(c.deletedContent, "\\n") + "}"
}

// ============================================================================
// VisualSwapAnchorCommand - Swap cursor and anchor positions ('o')
// ============================================================================

// VisualSwapAnchorCommand swaps the cursor position with the visual anchor ('o').
// This allows moving the "other end" of the selection in visual mode.
// After swapping, the selection remains the same but the cursor moves to where
// the anchor was, and the anchor moves to where the cursor was.
type VisualSwapAnchorCommand struct {
	MotionBase
	mode Mode // Which visual mode this command operates in
}

// Execute swaps the cursor and anchor positions.
func (c *VisualSwapAnchorCommand) Execute(m *Model) ExecuteResult {
	// Only operate in visual modes
	if m.mode != ModeVisual && m.mode != ModeVisualLine {
		return Skipped
	}

	// Swap cursor and anchor
	oldCursorRow := m.cursorRow
	oldCursorCol := m.cursorCol

	m.cursorRow = m.visualAnchor.Row
	m.cursorCol = m.visualAnchor.Col

	m.visualAnchor.Row = oldCursorRow
	m.visualAnchor.Col = oldCursorCol

	// Clamp cursor to valid bounds
	if m.cursorRow >= len(m.content) {
		m.cursorRow = len(m.content) - 1
	}
	if m.cursorRow < 0 {
		m.cursorRow = 0
	}
	m.clampCursorCol()

	return Executed
}

// Keys returns the trigger key for this command.
func (c *VisualSwapAnchorCommand) Keys() []string {
	return []string{"o"}
}

// Mode returns the mode this command operates in.
func (c *VisualSwapAnchorCommand) Mode() Mode {
	return c.mode
}

// ID returns the hierarchical identifier for this command.
func (c *VisualSwapAnchorCommand) ID() string {
	return "visual.swap_anchor"
}
