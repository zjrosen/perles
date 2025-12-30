package vimtextarea

// ============================================================================
// Delete Commands
// ============================================================================

// DeleteCharCommand deletes a single grapheme cluster under the cursor (x command).
// It captures the deleted grapheme for undo.
// Note: cursorCol and col are grapheme indices, not byte offsets.
type DeleteCharCommand struct {
	DeleteBase
	row             int    // Row where deletion occurred (captured in Execute)
	col             int    // Column (grapheme index) where deletion occurred (captured in Execute)
	deletedGrapheme string // Grapheme cluster that was deleted (for undo)
}

// Execute deletes the grapheme cluster at the current cursor position.
// cursorCol is a grapheme index, not a byte offset.
func (c *DeleteCharCommand) Execute(m *Model) ExecuteResult {
	// Capture position from model
	c.row = m.cursorRow
	line := m.content[c.row]

	// If line is empty, nothing to delete
	graphemeCount := GraphemeCount(line)
	if graphemeCount == 0 {
		return Executed
	}

	// Clamp column (grapheme index) to valid range and capture for undo
	c.col = min(m.cursorCol, graphemeCount-1)

	// Capture deleted grapheme cluster for undo
	c.deletedGrapheme = GraphemeAt(line, c.col)

	// Delete grapheme at cursor position using grapheme-aware slicing
	// SliceByGraphemes: line[:col] + line[col+1:]
	m.content[c.row] = SliceByGraphemes(line, 0, c.col) + SliceByGraphemes(line, c.col+1, graphemeCount)

	// Populate yank register (vim behavior: deletes also yank)
	m.lastYankedText = c.deletedGrapheme
	m.lastYankWasLinewise = false

	// If cursor is now past end of line, move back
	newGraphemeCount := GraphemeCount(m.content[m.cursorRow])
	if m.cursorCol >= newGraphemeCount && m.cursorCol > 0 {
		m.cursorCol = newGraphemeCount - 1
	}
	if m.cursorCol < 0 {
		m.cursorCol = 0
	}

	return Executed
}

// Undo restores the deleted grapheme cluster.
func (c *DeleteCharCommand) Undo(m *Model) error {
	line := m.content[c.row]

	// Insert the deleted grapheme back at the original grapheme position
	// Use InsertAtGrapheme for grapheme-aware insertion
	m.content[c.row] = InsertAtGrapheme(line, c.col, c.deletedGrapheme)

	// Restore cursor position (col is grapheme index)
	m.cursorRow = c.row
	m.cursorCol = c.col

	return nil
}

// Keys returns the trigger keys for this command.
func (c *DeleteCharCommand) Keys() []string {
	return []string{"x"}
}

// Mode returns the mode this command operates in.
func (c *DeleteCharCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *DeleteCharCommand) ID() string {
	return "delete.char"
}

// DeleteLineCommand deletes an entire line (dd command).
// It captures the deleted line content for undo.
type DeleteLineCommand struct {
	DeleteBase
	row         int    // Row that was deleted
	deletedLine string // Content of deleted line (for undo)
	wasLastLine bool   // Whether deleted line was the last line (affects undo cursor position)
	wasOnlyLine bool   // Whether this was the only line (content cleared instead of removed)
}

// Execute deletes the entire current line.
func (c *DeleteLineCommand) Execute(m *Model) ExecuteResult {
	// Capture state for undo
	c.row = m.cursorRow
	c.deletedLine = m.content[m.cursorRow]
	c.wasOnlyLine = len(m.content) == 1
	c.wasLastLine = m.cursorRow == len(m.content)-1

	// Populate yank register (vim behavior: deletes also yank)
	m.lastYankedText = c.deletedLine
	m.lastYankWasLinewise = true

	if c.wasOnlyLine {
		// Only one line - clear it but keep empty line
		m.content[0] = ""
		m.cursorCol = 0
	} else {
		// Remove the current line
		newContent := make([]string, len(m.content)-1)
		copy(newContent[:m.cursorRow], m.content[:m.cursorRow])
		copy(newContent[m.cursorRow:], m.content[m.cursorRow+1:])
		m.content = newContent

		// If we deleted the last line, move cursor up
		if m.cursorRow >= len(m.content) {
			m.cursorRow = len(m.content) - 1
		}

		// Clamp cursor column to new line
		m.clampCursorCol()
	}

	return Executed
}

// Undo restores the deleted line.
func (c *DeleteLineCommand) Undo(m *Model) error {
	if c.wasOnlyLine {
		// Restore the cleared line
		m.content[0] = c.deletedLine
	} else {
		// Insert the deleted line back
		newContent := make([]string, len(m.content)+1)
		copy(newContent[:c.row], m.content[:c.row])
		newContent[c.row] = c.deletedLine
		copy(newContent[c.row+1:], m.content[c.row:])
		m.content = newContent
	}

	// Restore cursor position
	m.cursorRow = c.row
	m.cursorCol = 0
	m.clampCursorCol()

	return nil
}

// Keys returns the trigger keys for this command.
func (c *DeleteLineCommand) Keys() []string {
	return []string{"dd"}
}

// Mode returns the mode this command operates in.
func (c *DeleteLineCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *DeleteLineCommand) ID() string {
	return "delete.line"
}

// DeleteToEOLCommand deletes from cursor to end of line (D or d$ command).
// It captures the deleted text for undo.
// Note: cursorCol and col are grapheme indices, not byte offsets.
type DeleteToEOLCommand struct {
	DeleteBase
	row         int    // Row where deletion occurred (captured in Execute)
	col         int    // Column (grapheme index) where deletion started (captured in Execute)
	deletedText string // Text that was deleted (for undo)
}

// Execute deletes from cursor to end of line.
// cursorCol is a grapheme index, not a byte offset.
func (c *DeleteToEOLCommand) Execute(m *Model) ExecuteResult {
	// Capture position from model
	c.row = m.cursorRow
	c.col = m.cursorCol
	line := m.content[c.row]

	graphemeCount := GraphemeCount(line)

	// If cursor is at or past end (in graphemes), nothing to delete
	if c.col >= graphemeCount {
		return Executed
	}

	// Capture deleted text for undo (using grapheme-aware slicing)
	c.deletedText = SliceByGraphemes(line, c.col, graphemeCount)

	// Delete from cursor to end (keep content before cursor)
	m.content[c.row] = SliceByGraphemes(line, 0, c.col)

	// Populate yank register (vim behavior: deletes also yank)
	m.lastYankedText = c.deletedText
	m.lastYankWasLinewise = false

	// Move cursor back one if we're now past the end
	newGraphemeCount := GraphemeCount(m.content[m.cursorRow])
	if m.cursorCol > 0 && m.cursorCol >= newGraphemeCount {
		m.cursorCol = newGraphemeCount - 1
	}
	if m.cursorCol < 0 {
		m.cursorCol = 0
	}

	return Executed
}

// Undo restores the deleted text.
func (c *DeleteToEOLCommand) Undo(m *Model) error {
	line := m.content[c.row]

	// Restore deleted text at the original grapheme position
	// The deleted text goes at position c.col (end of current content)
	m.content[c.row] = InsertAtGrapheme(line, c.col, c.deletedText)

	// Restore cursor position
	m.cursorRow = c.row
	m.cursorCol = c.col

	return nil
}

// Keys returns the trigger keys for this command.
func (c *DeleteToEOLCommand) Keys() []string {
	return []string{"D"}
}

// Mode returns the mode this command operates in.
func (c *DeleteToEOLCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *DeleteToEOLCommand) ID() string {
	return "delete.to_eol"
}

// DeleteWordCommand deletes from cursor to start of next word (dw command).
// It captures the deleted text for undo.
// Note: cursorCol and col are grapheme indices, not byte offsets.
type DeleteWordCommand struct {
	DeleteBase
	row         int    // Row where deletion occurred (captured in Execute)
	col         int    // Column (grapheme index) where deletion started (captured in Execute)
	deletedText string // Text that was deleted (for undo)
}

// Execute deletes from cursor to start of next word.
// cursorCol is a grapheme index. findNextWordStart already returns grapheme indices.
func (c *DeleteWordCommand) Execute(m *Model) ExecuteResult {
	// Capture position from model
	c.row = m.cursorRow
	c.col = m.cursorCol
	line := m.content[c.row]

	graphemeCount := GraphemeCount(line)

	// If at end of line or empty line, nothing to delete
	if graphemeCount == 0 || c.col >= graphemeCount {
		return Executed
	}

	// Find where the next word starts (this is where we delete to)
	// findNextWordStart already returns grapheme indices
	endCol := m.findNextWordStart(line, c.col)

	// Capture what we're deleting using grapheme-aware slicing
	if endCol >= graphemeCount {
		c.deletedText = SliceByGraphemes(line, c.col, graphemeCount)
		m.content[c.row] = SliceByGraphemes(line, 0, c.col)
	} else {
		c.deletedText = SliceByGraphemes(line, c.col, endCol)
		m.content[c.row] = SliceByGraphemes(line, 0, c.col) + SliceByGraphemes(line, endCol, graphemeCount)
	}

	// Populate yank register (vim behavior: deletes also yank)
	m.lastYankedText = c.deletedText
	m.lastYankWasLinewise = false

	// Clamp cursor (using grapheme count)
	newGraphemeCount := GraphemeCount(m.content[m.cursorRow])
	if m.cursorCol >= newGraphemeCount && m.cursorCol > 0 {
		m.cursorCol = newGraphemeCount - 1
	}
	if m.cursorCol < 0 {
		m.cursorCol = 0
	}

	return Executed
}

// Undo restores the deleted word.
func (c *DeleteWordCommand) Undo(m *Model) error {
	line := m.content[c.row]

	// Restore deleted text at the original grapheme position
	m.content[c.row] = InsertAtGrapheme(line, c.col, c.deletedText)

	// Restore cursor position
	m.cursorRow = c.row
	m.cursorCol = c.col

	return nil
}

// Keys returns the trigger keys for this command.
func (c *DeleteWordCommand) Keys() []string {
	return []string{"dw"}
}

// Mode returns the mode this command operates in.
func (c *DeleteWordCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *DeleteWordCommand) ID() string {
	return "delete.word"
}

// DeleteLinesCommand deletes multiple lines (dj/dk commands).
// It captures the deleted lines for undo.
type DeleteLinesCommand struct {
	DeleteBase
	startRow     int      // First row to delete
	count        int      // Number of lines to delete
	deletedLines []string // Lines that were deleted (for undo)
	cursorCol    int      // Original cursor column (for undo)
}

// Execute deletes count lines starting from startRow.
func (c *DeleteLinesCommand) Execute(m *Model) ExecuteResult {
	// Capture state for undo
	c.cursorCol = m.cursorCol

	// Clamp count to available lines
	actualCount := c.count
	if c.startRow+actualCount > len(m.content) {
		actualCount = len(m.content) - c.startRow
	}

	// Capture deleted lines for undo
	c.deletedLines = make([]string, actualCount)
	copy(c.deletedLines, m.content[c.startRow:c.startRow+actualCount])

	if len(m.content) <= actualCount {
		// Deleting all lines - leave one empty line
		m.content = []string{""}
		m.cursorRow = 0
		m.cursorCol = 0
	} else {
		// Remove count lines starting from startRow
		newContent := make([]string, len(m.content)-actualCount)
		copy(newContent[:c.startRow], m.content[:c.startRow])
		copy(newContent[c.startRow:], m.content[c.startRow+actualCount:])
		m.content = newContent

		// Position cursor at the start row (or last line if beyond)
		m.cursorRow = c.startRow
		if m.cursorRow >= len(m.content) {
			m.cursorRow = len(m.content) - 1
		}

		// Clamp cursor column
		m.clampCursorCol()
	}

	return Executed
}

// Undo restores the deleted lines.
func (c *DeleteLinesCommand) Undo(m *Model) error {
	if len(c.deletedLines) == 0 {
		return nil
	}

	// Check if we're restoring from a single empty line state
	if len(m.content) == 1 && m.content[0] == "" && c.startRow == 0 {
		// Replace the empty line with deleted lines
		m.content = make([]string, len(c.deletedLines))
		copy(m.content, c.deletedLines)
	} else {
		// Insert the deleted lines back at the original position
		newContent := make([]string, len(m.content)+len(c.deletedLines))
		copy(newContent[:c.startRow], m.content[:c.startRow])
		copy(newContent[c.startRow:c.startRow+len(c.deletedLines)], c.deletedLines)
		copy(newContent[c.startRow+len(c.deletedLines):], m.content[c.startRow:])
		m.content = newContent
	}

	// Restore cursor position
	m.cursorRow = c.startRow
	m.cursorCol = c.cursorCol
	m.clampCursorCol()

	return nil
}

// Keys returns the trigger keys for this command.
// Note: DeleteLinesCommand is created via dj or dk, so this returns "dj" as default.
func (c *DeleteLinesCommand) Keys() []string {
	return []string{"dj"}
}

// Mode returns the mode this command operates in.
func (c *DeleteLinesCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *DeleteLinesCommand) ID() string {
	return "delete.lines"
}

// ============================================================================
// DeleteLinesDownCommand (dj) - Delete current line and line below
// ============================================================================

// DeleteLinesDownCommand deletes the current line and the line below (dj).
// Wraps DeleteLinesCommand with the appropriate startRow and count.
type DeleteLinesDownCommand struct {
	inner *DeleteLinesCommand
}

// Execute deletes current line and line below.
func (c *DeleteLinesDownCommand) Execute(m *Model) ExecuteResult {
	// Capture position from model
	c.inner = &DeleteLinesCommand{startRow: m.cursorRow, count: 2}
	return c.inner.Execute(m)
}

// Undo restores the deleted lines.
func (c *DeleteLinesDownCommand) Undo(m *Model) error {
	if c.inner == nil {
		return nil
	}
	return c.inner.Undo(m)
}

// Keys returns the trigger keys for this command.
func (c *DeleteLinesDownCommand) Keys() []string { return []string{"dj"} }

// Mode returns the mode this command operates in.
func (c *DeleteLinesDownCommand) Mode() Mode { return ModeNormal }

// ID returns the hierarchical identifier for this command.
func (c *DeleteLinesDownCommand) ID() string { return "delete.lines_down" }

// IsUndoable returns true - delete commands are undoable.
func (c *DeleteLinesDownCommand) IsUndoable() bool { return true }

// ChangesContent returns true - delete commands change content.
func (c *DeleteLinesDownCommand) ChangesContent() bool { return true }

// IsModeChange returns false - delete doesn't change mode.
func (c *DeleteLinesDownCommand) IsModeChange() bool { return false }

// ============================================================================
// DeleteLinesUpCommand (dk) - Delete current line and line above
// ============================================================================

// DeleteLinesUpCommand deletes the current line and the line above (dk).
// Wraps DeleteLinesCommand with cursor adjustment for upward deletion.
type DeleteLinesUpCommand struct {
	inner *DeleteLinesCommand
}

// Execute deletes current line and line above.
func (c *DeleteLinesUpCommand) Execute(m *Model) ExecuteResult {
	// Capture position from model and determine start row
	currentRow := m.cursorRow
	startRow := currentRow
	count := 2

	if currentRow > 0 {
		// Move cursor up first, then delete from new position
		m.cursorRow--
		startRow = m.cursorRow
	} else {
		// At first line, just delete current line
		count = 1
	}

	c.inner = &DeleteLinesCommand{startRow: startRow, count: count}
	return c.inner.Execute(m)
}

// Undo restores the deleted lines.
// Note: Cursor position is handled by inner DeleteLinesCommand - it restores to startRow,
// which is where the cursor was after moving up (matching vim behavior).
func (c *DeleteLinesUpCommand) Undo(m *Model) error {
	if c.inner == nil {
		return nil
	}
	return c.inner.Undo(m)
}

// Keys returns the trigger keys for this command.
func (c *DeleteLinesUpCommand) Keys() []string { return []string{"dk"} }

// Mode returns the mode this command operates in.
func (c *DeleteLinesUpCommand) Mode() Mode { return ModeNormal }

// ID returns the hierarchical identifier for this command.
func (c *DeleteLinesUpCommand) ID() string { return "delete.lines_up" }

// IsUndoable returns true - delete commands are undoable.
func (c *DeleteLinesUpCommand) IsUndoable() bool { return true }

// ChangesContent returns true - delete commands change content.
func (c *DeleteLinesUpCommand) ChangesContent() bool { return true }

// IsModeChange returns false - delete doesn't change mode.
func (c *DeleteLinesUpCommand) IsModeChange() bool { return false }

// ============================================================================
// DeleteToFirstLineCommand (dgg) - Delete from current line to first line
// ============================================================================

// DeleteToFirstLineCommand deletes from current line to first line (dgg).
type DeleteToFirstLineCommand struct {
	DeleteBase
	startRow     int      // First row deleted (always 0)
	endRow       int      // Last row deleted (original cursor row)
	deletedLines []string // Lines that were deleted (for undo)
	cursorCol    int      // Original cursor column (for undo)
}

// Execute deletes from current line to first line (inclusive).
func (c *DeleteToFirstLineCommand) Execute(m *Model) ExecuteResult {
	// Capture state for undo
	c.startRow = 0
	c.endRow = m.cursorRow
	c.cursorCol = m.cursorCol

	// Number of lines to delete (from first line to current, inclusive)
	count := c.endRow + 1

	// Capture deleted lines for undo
	c.deletedLines = make([]string, count)
	copy(c.deletedLines, m.content[:count])

	if len(m.content) <= count {
		// Deleting all lines - leave one empty line
		m.content = []string{""}
		m.cursorRow = 0
		m.cursorCol = 0
	} else {
		// Remove lines from start to current row
		m.content = m.content[count:]
		m.cursorRow = 0
		m.clampCursorCol()
	}

	return Executed
}

// Undo restores the deleted lines.
func (c *DeleteToFirstLineCommand) Undo(m *Model) error {
	if len(c.deletedLines) == 0 {
		return nil
	}

	// Check if we're restoring from a single empty line state
	if len(m.content) == 1 && m.content[0] == "" {
		m.content = make([]string, len(c.deletedLines))
		copy(m.content, c.deletedLines)
	} else {
		// Prepend the deleted lines
		newContent := make([]string, len(c.deletedLines)+len(m.content))
		copy(newContent[:len(c.deletedLines)], c.deletedLines)
		copy(newContent[len(c.deletedLines):], m.content)
		m.content = newContent
	}

	// Restore cursor position
	m.cursorRow = c.endRow
	m.cursorCol = c.cursorCol
	m.clampCursorCol()

	return nil
}

// Keys returns the trigger keys for this command.
func (c *DeleteToFirstLineCommand) Keys() []string { return []string{"dgg"} }

// Mode returns the mode this command operates in.
func (c *DeleteToFirstLineCommand) Mode() Mode { return ModeNormal }

// ID returns the hierarchical identifier for this command.
func (c *DeleteToFirstLineCommand) ID() string { return "delete.to_first_line" }

// IsUndoable returns true - delete commands are undoable.
func (c *DeleteToFirstLineCommand) IsUndoable() bool { return true }

// ChangesContent returns true - delete commands change content.
func (c *DeleteToFirstLineCommand) ChangesContent() bool { return true }

// IsModeChange returns false - delete doesn't change mode.
func (c *DeleteToFirstLineCommand) IsModeChange() bool { return false }

// ============================================================================
// DeleteToLastLineCommand (dG) - Delete from current line to last line
// ============================================================================

// DeleteToLastLineCommand deletes from current line to last line (dG).
type DeleteToLastLineCommand struct {
	DeleteBase
	startRow     int      // First row deleted (original cursor row)
	deletedLines []string // Lines that were deleted (for undo)
	cursorCol    int      // Original cursor column (for undo)
}

// Execute deletes from current line to last line (inclusive).
func (c *DeleteToLastLineCommand) Execute(m *Model) ExecuteResult {
	// Capture state for undo
	c.startRow = m.cursorRow
	c.cursorCol = m.cursorCol

	// Number of lines to delete (from current to end)
	count := len(m.content) - c.startRow

	// Capture deleted lines for undo
	c.deletedLines = make([]string, count)
	copy(c.deletedLines, m.content[c.startRow:])

	if c.startRow == 0 {
		// Deleting all lines - leave one empty line
		m.content = []string{""}
		m.cursorRow = 0
		m.cursorCol = 0
	} else {
		// Remove lines from current row to end
		m.content = m.content[:c.startRow]
		m.cursorRow = len(m.content) - 1
		m.clampCursorCol()
	}

	return Executed
}

// Undo restores the deleted lines.
func (c *DeleteToLastLineCommand) Undo(m *Model) error {
	if len(c.deletedLines) == 0 {
		return nil
	}

	// Check if we're restoring from a single empty line state
	if len(m.content) == 1 && m.content[0] == "" && c.startRow == 0 {
		m.content = make([]string, len(c.deletedLines))
		copy(m.content, c.deletedLines)
	} else {
		// Append the deleted lines at startRow
		newContent := make([]string, c.startRow+len(c.deletedLines))
		copy(newContent[:c.startRow], m.content[:c.startRow])
		copy(newContent[c.startRow:], c.deletedLines)
		m.content = newContent
	}

	// Restore cursor position
	m.cursorRow = c.startRow
	m.cursorCol = c.cursorCol
	m.clampCursorCol()

	return nil
}

// Keys returns the trigger keys for this command.
func (c *DeleteToLastLineCommand) Keys() []string { return []string{"dG"} }

// Mode returns the mode this command operates in.
func (c *DeleteToLastLineCommand) Mode() Mode { return ModeNormal }

// ID returns the hierarchical identifier for this command.
func (c *DeleteToLastLineCommand) ID() string { return "delete.to_last_line" }

// IsUndoable returns true - delete commands are undoable.
func (c *DeleteToLastLineCommand) IsUndoable() bool { return true }

// ChangesContent returns true - delete commands change content.
func (c *DeleteToLastLineCommand) ChangesContent() bool { return true }

// IsModeChange returns false - delete doesn't change mode.
func (c *DeleteToLastLineCommand) IsModeChange() bool { return false }

// ============================================================================
// Insert-mode Delete Commands
// ============================================================================

// BackspaceCommand deletes the grapheme cluster before the cursor.
// If at line start, joins with the previous line.
// Note: cursorCol and col are grapheme indices, not byte offsets.
type BackspaceCommand struct {
	DeleteBase
	row             int    // Row where deletion occurred
	col             int    // Column (grapheme index) where deletion occurred (original cursor position)
	deletedGrapheme string // Grapheme cluster that was deleted (if not joining lines)
	joinedLine      bool   // True if lines were joined
	joinedText      string // Text that was on the deleted line (if joining)
}

// Execute deletes the grapheme cluster before the cursor or joins lines.
// cursorCol is a grapheme index, not a byte offset.
func (c *BackspaceCommand) Execute(m *Model) ExecuteResult {
	c.row = m.cursorRow
	c.col = m.cursorCol

	// If at the very beginning of content, nothing to delete
	if c.row == 0 && c.col == 0 {
		return Skipped
	}

	if c.col > 0 {
		// Delete grapheme before cursor on current line
		line := m.content[c.row]
		graphemeCount := GraphemeCount(line)

		// Get the grapheme at col-1 (the one before cursor)
		c.deletedGrapheme = GraphemeAt(line, c.col-1)
		c.joinedLine = false

		// Delete using grapheme-aware slicing: line[:col-1] + line[col:]
		m.content[c.row] = SliceByGraphemes(line, 0, c.col-1) + SliceByGraphemes(line, c.col, graphemeCount)
		m.cursorCol--
	} else {
		// At start of line - join with previous line
		c.joinedLine = true
		prevLine := m.content[c.row-1]
		currentLine := m.content[c.row]
		c.joinedText = currentLine

		// New cursor position is at the grapheme count of the previous line
		newCursorCol := GraphemeCount(prevLine)

		// Join lines
		m.content[c.row-1] = prevLine + currentLine

		// Remove current line
		newContent := make([]string, len(m.content)-1)
		copy(newContent[:c.row], m.content[:c.row])
		copy(newContent[c.row:], m.content[c.row+1:])
		m.content = newContent

		// Move cursor to join point
		m.cursorRow--
		m.cursorCol = newCursorCol
	}

	return Executed
}

// Undo restores the deleted grapheme cluster or splits joined lines.
func (c *BackspaceCommand) Undo(m *Model) error {
	if c.row == 0 && c.col == 0 {
		// Nothing was deleted
		return nil
	}

	if !c.joinedLine {
		// Restore deleted grapheme cluster at grapheme position col-1
		line := m.content[c.row]
		m.content[c.row] = InsertAtGrapheme(line, c.col-1, c.deletedGrapheme)
		m.cursorRow = c.row
		m.cursorCol = c.col
	} else {
		// Split the joined line back
		// joinedText was the text on the current line before joining
		// Find the split point using grapheme count of the original previous line
		joinedLine := m.content[c.row-1]
		// The join point in graphemes is: total graphemes - graphemes in joinedText
		totalGraphemes := GraphemeCount(joinedLine)
		joinedTextGraphemes := GraphemeCount(c.joinedText)
		joinPointGrapheme := totalGraphemes - joinedTextGraphemes

		firstPart := SliceByGraphemes(joinedLine, 0, joinPointGrapheme)
		secondPart := SliceByGraphemes(joinedLine, joinPointGrapheme, totalGraphemes)

		// Create new content with split
		newContent := make([]string, len(m.content)+1)
		copy(newContent[:c.row], m.content[:c.row])
		newContent[c.row-1] = firstPart
		newContent[c.row] = secondPart
		copy(newContent[c.row+1:], m.content[c.row:])
		m.content = newContent

		// Restore cursor position
		m.cursorRow = c.row
		m.cursorCol = c.col
	}

	return nil
}

// Keys returns the trigger keys for this command.
func (c *BackspaceCommand) Keys() []string {
	return []string{"<backspace>"}
}

// Mode returns the mode this command operates in.
func (c *BackspaceCommand) Mode() Mode {
	return ModeInsert
}

// ID returns the hierarchical identifier for this command.
func (c *BackspaceCommand) ID() string {
	return "delete.backspace"
}

// DeleteKeyCommand deletes the grapheme cluster at the cursor position.
// If at end of line, joins with the next line.
// Note: cursorCol and col are grapheme indices, not byte offsets.
type DeleteKeyCommand struct {
	DeleteBase
	row             int    // Row where deletion occurred
	col             int    // Column (grapheme index) where deletion occurred
	deletedGrapheme string // Grapheme cluster that was deleted (if not joining lines)
	joinedLine      bool   // True if lines were joined
	joinedText      string // Text from the next line that was joined
}

// Execute deletes the grapheme cluster at the cursor or joins with next line.
// cursorCol is a grapheme index, not a byte offset.
func (c *DeleteKeyCommand) Execute(m *Model) ExecuteResult {
	c.row = m.cursorRow
	c.col = m.cursorCol
	line := m.content[c.row]

	graphemeCount := GraphemeCount(line)

	if c.col < graphemeCount {
		// Delete grapheme at cursor position
		c.deletedGrapheme = GraphemeAt(line, c.col)
		c.joinedLine = false
		// Delete using grapheme-aware slicing: line[:col] + line[col+1:]
		m.content[c.row] = SliceByGraphemes(line, 0, c.col) + SliceByGraphemes(line, c.col+1, graphemeCount)
	} else if c.row < len(m.content)-1 {
		// At end of line - join with next line
		c.joinedLine = true
		nextLine := m.content[c.row+1]
		c.joinedText = nextLine

		// Join lines
		m.content[c.row] = line + nextLine

		// Remove next line
		newContent := make([]string, len(m.content)-1)
		copy(newContent[:c.row+1], m.content[:c.row+1])
		copy(newContent[c.row+1:], m.content[c.row+2:])
		m.content = newContent
	} else {
		// At end of last line, nothing to delete
		return Skipped
	}

	return Executed
}

// Undo restores the deleted grapheme cluster or splits joined lines.
func (c *DeleteKeyCommand) Undo(m *Model) error {
	line := m.content[c.row]

	if !c.joinedLine {
		// Restore deleted grapheme at grapheme position col
		m.content[c.row] = InsertAtGrapheme(line, c.col, c.deletedGrapheme)
	} else {
		// Split the joined line back
		// joinedText was the next line content
		// Find the split point using grapheme count
		totalGraphemes := GraphemeCount(line)
		joinedTextGraphemes := GraphemeCount(c.joinedText)
		splitPointGrapheme := totalGraphemes - joinedTextGraphemes

		firstPart := SliceByGraphemes(line, 0, splitPointGrapheme)
		secondPart := SliceByGraphemes(line, splitPointGrapheme, totalGraphemes)

		// Create new content with split
		newContent := make([]string, len(m.content)+1)
		copy(newContent[:c.row+1], m.content[:c.row+1])
		newContent[c.row] = firstPart
		newContent[c.row+1] = secondPart
		copy(newContent[c.row+2:], m.content[c.row+1:])
		m.content = newContent
	}

	// Restore cursor position
	m.cursorRow = c.row
	m.cursorCol = c.col

	return nil
}

// Keys returns the trigger keys for this command.
func (c *DeleteKeyCommand) Keys() []string {
	return []string{"<delete>"}
}

// Mode returns the mode this command operates in.
func (c *DeleteKeyCommand) Mode() Mode {
	return ModeInsert
}

// ID returns the hierarchical identifier for this command.
func (c *DeleteKeyCommand) ID() string {
	return "delete.key"
}

// ============================================================================
// Readline-style Kill Commands (Insert mode)
// ============================================================================

// KillToLineStartCommand deletes from cursor to start of line (Ctrl+U).
// Standard readline/emacs binding that works in Insert mode.
// Note: cursorCol and col are grapheme indices, not byte offsets.
type KillToLineStartCommand struct {
	InsertBase
	row         int    // Row where deletion occurred
	col         int    // Column (grapheme index) where deletion started
	deletedText string // Text that was deleted (for undo)
}

// Execute deletes from cursor to start of line.
// cursorCol is a grapheme index, not a byte offset.
func (c *KillToLineStartCommand) Execute(m *Model) ExecuteResult {
	c.row = m.cursorRow
	c.col = m.cursorCol
	line := m.content[c.row]

	if c.col == 0 {
		return Skipped // Nothing to delete
	}

	graphemeCount := GraphemeCount(line)

	// Capture deleted text for undo (using grapheme-aware slicing)
	c.deletedText = SliceByGraphemes(line, 0, c.col)

	// Delete from start to cursor (keep content from cursor onward)
	m.content[c.row] = SliceByGraphemes(line, c.col, graphemeCount)
	m.cursorCol = 0

	return Executed
}

// Undo restores the deleted text.
func (c *KillToLineStartCommand) Undo(m *Model) error {
	line := m.content[c.row]
	// Prepend deleted text to current line
	m.content[c.row] = c.deletedText + line
	m.cursorCol = c.col
	return nil
}

// Keys returns the trigger keys for this command.
func (c *KillToLineStartCommand) Keys() []string { return []string{"<ctrl+u>"} }

// Mode returns the mode this command operates in.
func (c *KillToLineStartCommand) Mode() Mode { return ModeInsert }

// ID returns the hierarchical identifier for this command.
func (c *KillToLineStartCommand) ID() string { return "kill.to_line_start" }

// KillToLineEndCommand deletes from cursor to end of line (Ctrl+K).
// Standard readline/emacs binding that works in Insert mode.
// Note: cursorCol and col are grapheme indices, not byte offsets.
type KillToLineEndCommand struct {
	InsertBase
	row         int    // Row where deletion occurred
	col         int    // Column (grapheme index) where deletion started
	deletedText string // Text that was deleted (for undo)
}

// Execute deletes from cursor to end of line.
// cursorCol is a grapheme index, not a byte offset.
func (c *KillToLineEndCommand) Execute(m *Model) ExecuteResult {
	c.row = m.cursorRow
	c.col = m.cursorCol
	line := m.content[c.row]

	graphemeCount := GraphemeCount(line)

	if c.col >= graphemeCount {
		return Skipped // Nothing to delete
	}

	// Capture deleted text for undo (using grapheme-aware slicing)
	c.deletedText = SliceByGraphemes(line, c.col, graphemeCount)

	// Delete from cursor to end (keep content before cursor)
	m.content[c.row] = SliceByGraphemes(line, 0, c.col)

	return Executed
}

// Undo restores the deleted text.
func (c *KillToLineEndCommand) Undo(m *Model) error {
	line := m.content[c.row]
	// Append deleted text at the original grapheme position
	m.content[c.row] = InsertAtGrapheme(line, c.col, c.deletedText)
	m.cursorCol = c.col
	return nil
}

// Keys returns the trigger keys for this command.
func (c *KillToLineEndCommand) Keys() []string { return []string{"<ctrl+k>"} }

// Mode returns the mode this command operates in.
func (c *KillToLineEndCommand) Mode() Mode { return ModeInsert }

// ID returns the hierarchical identifier for this command.
func (c *KillToLineEndCommand) ID() string { return "kill.to_line_end" }
