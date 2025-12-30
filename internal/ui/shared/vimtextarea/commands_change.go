package vimtextarea

// ============================================================================
// Change Commands
// ============================================================================

// ChangeWordCommand deletes from cursor to start of next word and enters insert mode (cw command).
// It captures the deleted text for undo.
type ChangeWordCommand struct {
	ChangeBase
	row         int    // Row where deletion occurred (captured in Execute)
	col         int    // Column where deletion started (captured in Execute)
	deletedText string // Text that was deleted (for undo)
}

// Execute deletes from cursor to end of current word and enters insert mode.
// Unlike dw which deletes to start of next word (including trailing space),
// cw deletes only to the end of the current word (excluding trailing space).
func (c *ChangeWordCommand) Execute(m *Model) ExecuteResult {
	// Capture position from model
	c.row = m.cursorRow
	c.col = m.cursorCol
	line := m.content[c.row]

	// If at end of line or empty line, just enter insert mode
	if len(line) == 0 || c.col >= len(line) {
		m.mode = ModeInsert
		return Executed
	}

	// Find end of current word (not including trailing whitespace)
	endCol := c.col

	// Skip whitespace if we're on it (move to next word)
	for endCol < len(line) && isWhitespace(rune(line[endCol])) {
		endCol++
	}

	// If we only had whitespace, delete it and enter insert mode
	if endCol >= len(line) {
		c.deletedText = line[c.col:]
		m.content[c.row] = line[:c.col]
		m.mode = ModeInsert
		return Executed
	}

	// Now find the end of the word (stop at whitespace or end of line)
	for endCol < len(line) && !isWhitespace(rune(line[endCol])) {
		endCol++
	}

	// Capture what we're deleting (from cursor to end of word, not including trailing space)
	c.deletedText = line[c.col:endCol]
	m.content[c.row] = line[:c.col] + line[endCol:]

	// Enter insert mode (cursor stays at deletion point)
	m.mode = ModeInsert

	return Executed
}

// Undo restores the deleted word and returns to normal mode.
func (c *ChangeWordCommand) Undo(m *Model) error {
	line := m.content[c.row]

	// Restore deleted text at the original position
	m.content[c.row] = line[:c.col] + c.deletedText + line[c.col:]

	// Restore cursor position and mode
	m.cursorRow = c.row
	m.cursorCol = c.col
	m.mode = ModeNormal

	return nil
}

// Keys returns the trigger keys for this command.
func (c *ChangeWordCommand) Keys() []string {
	return []string{"cw"}
}

// Mode returns the mode this command operates in.
func (c *ChangeWordCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *ChangeWordCommand) ID() string {
	return "change.word"
}

// ChangeLineCommand clears the current line content and enters insert mode (cc command).
// It captures the deleted content for undo.
type ChangeLineCommand struct {
	ChangeBase
	row         int    // Row that was changed
	deletedText string // Content that was deleted (for undo)
}

// Execute clears the current line and enters insert mode.
func (c *ChangeLineCommand) Execute(m *Model) ExecuteResult {
	// Capture state for undo
	c.row = m.cursorRow
	c.deletedText = m.content[m.cursorRow]

	// Clear the line content (keep the line itself)
	m.content[m.cursorRow] = ""
	m.cursorCol = 0

	// Enter insert mode
	m.mode = ModeInsert

	return Executed
}

// Undo restores the deleted line content and returns to normal mode.
func (c *ChangeLineCommand) Undo(m *Model) error {
	m.content[c.row] = c.deletedText
	m.cursorRow = c.row
	m.cursorCol = 0
	m.mode = ModeNormal
	return nil
}

// Keys returns the trigger keys for this command.
func (c *ChangeLineCommand) Keys() []string {
	return []string{"cc"}
}

// Mode returns the mode this command operates in.
func (c *ChangeLineCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *ChangeLineCommand) ID() string {
	return "change.line"
}

// ChangeToEOLCommand deletes from cursor to end of line and enters insert mode (C or c$ command).
// It captures the deleted text for undo.
type ChangeToEOLCommand struct {
	ChangeBase
	row         int    // Row where change occurred (captured in Execute)
	col         int    // Column where change started (captured in Execute)
	deletedText string // Text that was deleted (for undo)
}

// Execute deletes from cursor to end of line and enters insert mode.
func (c *ChangeToEOLCommand) Execute(m *Model) ExecuteResult {
	// Capture position from model
	c.row = m.cursorRow
	c.col = m.cursorCol
	line := m.content[c.row]

	// If cursor is at or past end, just enter insert mode
	if c.col >= len(line) {
		c.deletedText = ""
		m.mode = ModeInsert
		return Executed
	}

	// Capture deleted text for undo
	c.deletedText = line[c.col:]

	// Delete from cursor to end
	m.content[c.row] = line[:c.col]

	// Enter insert mode (cursor stays at deletion point)
	m.mode = ModeInsert

	return Executed
}

// Undo restores the deleted text and returns to normal mode.
func (c *ChangeToEOLCommand) Undo(m *Model) error {
	line := m.content[c.row]

	// Restore deleted text at the original position
	m.content[c.row] = line[:c.col] + c.deletedText + line[c.col:]

	// Restore cursor position and mode
	m.cursorRow = c.row
	m.cursorCol = c.col
	m.mode = ModeNormal

	return nil
}

// Keys returns the trigger keys for this command.
func (c *ChangeToEOLCommand) Keys() []string {
	return []string{"C"}
}

// Mode returns the mode this command operates in.
func (c *ChangeToEOLCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *ChangeToEOLCommand) ID() string {
	return "change.to_eol"
}

// ChangeToLineStartCommand deletes from cursor to start of line and enters insert mode (c0 command).
// It captures the deleted text for undo.
type ChangeToLineStartCommand struct {
	ChangeBase
	row         int    // Row where change occurred (captured in Execute)
	col         int    // Column where change started (captured in Execute)
	deletedText string // Text that was deleted (for undo)
}

// Execute deletes from cursor to start of line and enters insert mode.
func (c *ChangeToLineStartCommand) Execute(m *Model) ExecuteResult {
	// Capture position from model
	c.row = m.cursorRow
	c.col = m.cursorCol
	line := m.content[c.row]

	// If cursor is at start, just enter insert mode
	if c.col == 0 {
		c.deletedText = ""
		m.mode = ModeInsert
		return Executed
	}

	// Capture deleted text for undo
	c.deletedText = line[:c.col]

	// Delete from start to cursor
	m.content[c.row] = line[c.col:]

	// Move cursor to start of line and enter insert mode
	m.cursorCol = 0
	m.mode = ModeInsert

	return Executed
}

// Undo restores the deleted text and returns to normal mode.
func (c *ChangeToLineStartCommand) Undo(m *Model) error {
	line := m.content[c.row]

	// Restore deleted text at the start
	m.content[c.row] = c.deletedText + line

	// Restore cursor position and mode
	m.cursorRow = c.row
	m.cursorCol = c.col
	m.mode = ModeNormal

	return nil
}

// Keys returns the trigger keys for this command.
func (c *ChangeToLineStartCommand) Keys() []string {
	return []string{"c0"}
}

// Mode returns the mode this command operates in.
func (c *ChangeToLineStartCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *ChangeToLineStartCommand) ID() string {
	return "change.to_line_start"
}

// ChangeLinesCommand changes multiple lines (cj/ck commands).
// It captures the deleted lines for undo, replaces them with an empty line, and enters insert mode.
type ChangeLinesCommand struct {
	ChangeBase
	startRow     int      // First row to change
	count        int      // Number of lines to change
	deletedLines []string // Lines that were deleted (for undo)
	cursorCol    int      // Original cursor column (for undo)
}

// Execute deletes count lines starting from startRow, inserts an empty line, and enters insert mode.
func (c *ChangeLinesCommand) Execute(m *Model) ExecuteResult {
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
		// Changing all lines - leave one empty line
		m.content = []string{""}
		m.cursorRow = 0
		m.cursorCol = 0
	} else {
		// Replace the deleted lines with a single empty line
		newContent := make([]string, len(m.content)-actualCount+1)
		copy(newContent[:c.startRow], m.content[:c.startRow])
		newContent[c.startRow] = "" // Empty line for insertion
		copy(newContent[c.startRow+1:], m.content[c.startRow+actualCount:])
		m.content = newContent

		// Position cursor at the empty line
		m.cursorRow = c.startRow
		m.cursorCol = 0
	}

	// Enter insert mode
	m.mode = ModeInsert

	return Executed
}

// Undo restores the deleted lines and returns to normal mode.
func (c *ChangeLinesCommand) Undo(m *Model) error {
	if len(c.deletedLines) == 0 {
		return nil
	}

	// Check if we're restoring from a single empty line state
	if len(m.content) == 1 && m.content[0] == "" && c.startRow == 0 {
		// Replace the empty line with deleted lines
		m.content = make([]string, len(c.deletedLines))
		copy(m.content, c.deletedLines)
	} else {
		// Remove the empty line we inserted and restore deleted lines
		// First, remove the line at startRow
		contentWithoutEmpty := make([]string, len(m.content)-1)
		copy(contentWithoutEmpty[:c.startRow], m.content[:c.startRow])
		copy(contentWithoutEmpty[c.startRow:], m.content[c.startRow+1:])

		// Then insert the deleted lines back
		newContent := make([]string, len(contentWithoutEmpty)+len(c.deletedLines))
		copy(newContent[:c.startRow], contentWithoutEmpty[:c.startRow])
		copy(newContent[c.startRow:c.startRow+len(c.deletedLines)], c.deletedLines)
		copy(newContent[c.startRow+len(c.deletedLines):], contentWithoutEmpty[c.startRow:])
		m.content = newContent
	}

	// Restore cursor position and mode
	m.cursorRow = c.startRow
	m.cursorCol = c.cursorCol
	m.clampCursorCol()
	m.mode = ModeNormal

	return nil
}

// Keys returns the trigger keys for this command.
// Note: ChangeLinesCommand is created via cj or ck, so this returns "cj" as default.
func (c *ChangeLinesCommand) Keys() []string {
	return []string{"cj"}
}

// Mode returns the mode this command operates in.
func (c *ChangeLinesCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *ChangeLinesCommand) ID() string {
	return "change.lines"
}

// ============================================================================
// ChangeLinesDownCommand (cj) - Change current line and line below
// ============================================================================

// ChangeLinesDownCommand changes the current line and the line below (cj).
type ChangeLinesDownCommand struct {
	ChangeBase
	startRow     int      // First row to change
	deletedLines []string // Lines that were deleted (for undo)
	cursorCol    int      // Original cursor column (for undo)
}

// Execute changes current line and line below.
func (c *ChangeLinesDownCommand) Execute(m *Model) ExecuteResult {
	// Capture state for undo
	c.startRow = m.cursorRow
	c.cursorCol = m.cursorCol

	// Change 2 lines (current and below), clamped to available lines
	count := 2
	if c.startRow+count > len(m.content) {
		count = len(m.content) - c.startRow
	}

	// Capture deleted lines for undo
	c.deletedLines = make([]string, count)
	copy(c.deletedLines, m.content[c.startRow:c.startRow+count])

	if len(m.content) <= count {
		// Changing all lines - leave one empty line
		m.content = []string{""}
		m.cursorRow = 0
		m.cursorCol = 0
	} else {
		// Replace the deleted lines with a single empty line
		newContent := make([]string, len(m.content)-count+1)
		copy(newContent[:c.startRow], m.content[:c.startRow])
		newContent[c.startRow] = "" // Empty line for insertion
		copy(newContent[c.startRow+1:], m.content[c.startRow+count:])
		m.content = newContent

		// Position cursor at the empty line
		m.cursorRow = c.startRow
		m.cursorCol = 0
	}

	// Enter insert mode
	m.mode = ModeInsert

	return Executed
}

// Undo restores the deleted lines and returns to normal mode.
func (c *ChangeLinesDownCommand) Undo(m *Model) error {
	if len(c.deletedLines) == 0 {
		return nil
	}

	// Check if we're restoring from a single empty line state
	if len(m.content) == 1 && m.content[0] == "" && c.startRow == 0 {
		// Replace the empty line with deleted lines
		m.content = make([]string, len(c.deletedLines))
		copy(m.content, c.deletedLines)
	} else {
		// Remove the empty line we inserted and restore deleted lines
		// First, remove the line at startRow
		contentWithoutEmpty := make([]string, len(m.content)-1)
		copy(contentWithoutEmpty[:c.startRow], m.content[:c.startRow])
		copy(contentWithoutEmpty[c.startRow:], m.content[c.startRow+1:])

		// Then insert the deleted lines back
		newContent := make([]string, len(contentWithoutEmpty)+len(c.deletedLines))
		copy(newContent[:c.startRow], contentWithoutEmpty[:c.startRow])
		copy(newContent[c.startRow:c.startRow+len(c.deletedLines)], c.deletedLines)
		copy(newContent[c.startRow+len(c.deletedLines):], contentWithoutEmpty[c.startRow:])
		m.content = newContent
	}

	// Restore cursor position and mode
	m.cursorRow = c.startRow
	m.cursorCol = c.cursorCol
	m.clampCursorCol()
	m.mode = ModeNormal

	return nil
}

// Keys returns the trigger keys for this command.
func (c *ChangeLinesDownCommand) Keys() []string { return []string{"cj"} }

// Mode returns the mode this command operates in.
func (c *ChangeLinesDownCommand) Mode() Mode { return ModeNormal }

// ID returns the hierarchical identifier for this command.
func (c *ChangeLinesDownCommand) ID() string { return "change.lines_down" }

// ============================================================================
// ChangeLinesUpCommand (ck) - Change current line and line above
// ============================================================================

// ChangeLinesUpCommand changes the current line and the line above (ck).
type ChangeLinesUpCommand struct {
	ChangeBase
	startRow     int      // First row to change
	deletedLines []string // Lines that were deleted (for undo)
	cursorCol    int      // Original cursor column (for undo)
}

// Execute changes current line and line above.
func (c *ChangeLinesUpCommand) Execute(m *Model) ExecuteResult {
	// Capture state for undo
	c.cursorCol = m.cursorCol

	// Determine start row and count
	currentRow := m.cursorRow
	c.startRow = currentRow
	count := 2

	if currentRow > 0 {
		// Move cursor up first, then change from new position
		m.cursorRow--
		c.startRow = m.cursorRow
	} else {
		// At first line, just change current line
		count = 1
	}

	// Clamp count to available lines
	if c.startRow+count > len(m.content) {
		count = len(m.content) - c.startRow
	}

	// Capture deleted lines for undo
	c.deletedLines = make([]string, count)
	copy(c.deletedLines, m.content[c.startRow:c.startRow+count])

	if len(m.content) <= count {
		// Changing all lines - leave one empty line
		m.content = []string{""}
		m.cursorRow = 0
		m.cursorCol = 0
	} else {
		// Replace the deleted lines with a single empty line
		newContent := make([]string, len(m.content)-count+1)
		copy(newContent[:c.startRow], m.content[:c.startRow])
		newContent[c.startRow] = "" // Empty line for insertion
		copy(newContent[c.startRow+1:], m.content[c.startRow+count:])
		m.content = newContent

		// Position cursor at the empty line
		m.cursorRow = c.startRow
		m.cursorCol = 0
	}

	// Enter insert mode
	m.mode = ModeInsert

	return Executed
}

// Undo restores the deleted lines and returns to normal mode.
func (c *ChangeLinesUpCommand) Undo(m *Model) error {
	if len(c.deletedLines) == 0 {
		return nil
	}

	// Check if we're restoring from a single empty line state
	if len(m.content) == 1 && m.content[0] == "" && c.startRow == 0 {
		// Replace the empty line with deleted lines
		m.content = make([]string, len(c.deletedLines))
		copy(m.content, c.deletedLines)
	} else {
		// Remove the empty line we inserted and restore deleted lines
		// First, remove the line at startRow
		contentWithoutEmpty := make([]string, len(m.content)-1)
		copy(contentWithoutEmpty[:c.startRow], m.content[:c.startRow])
		copy(contentWithoutEmpty[c.startRow:], m.content[c.startRow+1:])

		// Then insert the deleted lines back
		newContent := make([]string, len(contentWithoutEmpty)+len(c.deletedLines))
		copy(newContent[:c.startRow], contentWithoutEmpty[:c.startRow])
		copy(newContent[c.startRow:c.startRow+len(c.deletedLines)], c.deletedLines)
		copy(newContent[c.startRow+len(c.deletedLines):], contentWithoutEmpty[c.startRow:])
		m.content = newContent
	}

	// Restore cursor position and mode
	m.cursorRow = c.startRow
	m.cursorCol = c.cursorCol
	m.clampCursorCol()
	m.mode = ModeNormal

	return nil
}

// Keys returns the trigger keys for this command.
func (c *ChangeLinesUpCommand) Keys() []string { return []string{"ck"} }

// Mode returns the mode this command operates in.
func (c *ChangeLinesUpCommand) Mode() Mode { return ModeNormal }

// ID returns the hierarchical identifier for this command.
func (c *ChangeLinesUpCommand) ID() string { return "change.lines_up" }

// ============================================================================
// ChangeToFirstLineCommand (cgg) - Change from current line to first line
// ============================================================================

// ChangeToFirstLineCommand changes from current line to first line (cgg).
type ChangeToFirstLineCommand struct {
	ChangeBase
	endRow       int      // Last row deleted (original cursor row)
	deletedLines []string // Lines that were deleted (for undo)
	cursorCol    int      // Original cursor column (for undo)
}

// Execute deletes from current line to first line and enters insert mode.
func (c *ChangeToFirstLineCommand) Execute(m *Model) ExecuteResult {
	// Capture state for undo
	c.endRow = m.cursorRow
	c.cursorCol = m.cursorCol

	// Number of lines to delete (from first line to current, inclusive)
	count := c.endRow + 1

	// Capture deleted lines for undo
	c.deletedLines = make([]string, count)
	copy(c.deletedLines, m.content[:count])

	if len(m.content) <= count {
		// Changing all lines - leave one empty line
		m.content = []string{""}
		m.cursorRow = 0
		m.cursorCol = 0
	} else {
		// Replace deleted lines with a single empty line
		newContent := make([]string, len(m.content)-count+1)
		newContent[0] = "" // Empty line for insertion
		copy(newContent[1:], m.content[count:])
		m.content = newContent
		m.cursorRow = 0
		m.cursorCol = 0
	}

	// Enter insert mode
	m.mode = ModeInsert

	return Executed
}

// Undo restores the deleted lines and returns to normal mode.
func (c *ChangeToFirstLineCommand) Undo(m *Model) error {
	if len(c.deletedLines) == 0 {
		return nil
	}

	// Check if we're restoring from a single empty line state
	if len(m.content) == 1 && m.content[0] == "" {
		m.content = make([]string, len(c.deletedLines))
		copy(m.content, c.deletedLines)
	} else {
		// Remove the empty line we inserted and restore deleted lines
		contentWithoutEmpty := m.content[1:]
		newContent := make([]string, len(c.deletedLines)+len(contentWithoutEmpty))
		copy(newContent[:len(c.deletedLines)], c.deletedLines)
		copy(newContent[len(c.deletedLines):], contentWithoutEmpty)
		m.content = newContent
	}

	// Restore cursor position and mode
	m.cursorRow = c.endRow
	m.cursorCol = c.cursorCol
	m.clampCursorCol()
	m.mode = ModeNormal

	return nil
}

// Keys returns the trigger keys for this command.
func (c *ChangeToFirstLineCommand) Keys() []string { return []string{"cgg"} }

// Mode returns the mode this command operates in.
func (c *ChangeToFirstLineCommand) Mode() Mode { return ModeNormal }

// ID returns the hierarchical identifier for this command.
func (c *ChangeToFirstLineCommand) ID() string { return "change.to_first_line" }

// ============================================================================
// ChangeToLastLineCommand (cG) - Change from current line to last line
// ============================================================================

// ChangeToLastLineCommand changes from current line to last line (cG).
type ChangeToLastLineCommand struct {
	ChangeBase
	startRow     int      // First row deleted (original cursor row)
	deletedLines []string // Lines that were deleted (for undo)
	cursorCol    int      // Original cursor column (for undo)
}

// Execute deletes from current line to last line and enters insert mode.
func (c *ChangeToLastLineCommand) Execute(m *Model) ExecuteResult {
	// Capture state for undo
	c.startRow = m.cursorRow
	c.cursorCol = m.cursorCol

	// Number of lines to delete (from current to end)
	count := len(m.content) - c.startRow

	// Capture deleted lines for undo
	c.deletedLines = make([]string, count)
	copy(c.deletedLines, m.content[c.startRow:])

	if c.startRow == 0 {
		// Changing all lines - leave one empty line
		m.content = []string{""}
		m.cursorRow = 0
		m.cursorCol = 0
	} else {
		// Replace deleted lines with a single empty line
		newContent := make([]string, c.startRow+1)
		copy(newContent[:c.startRow], m.content[:c.startRow])
		newContent[c.startRow] = "" // Empty line for insertion
		m.content = newContent
		m.cursorRow = c.startRow
		m.cursorCol = 0
	}

	// Enter insert mode
	m.mode = ModeInsert

	return Executed
}

// Undo restores the deleted lines and returns to normal mode.
func (c *ChangeToLastLineCommand) Undo(m *Model) error {
	if len(c.deletedLines) == 0 {
		return nil
	}

	// Check if we're restoring from a single empty line state
	if len(m.content) == 1 && m.content[0] == "" && c.startRow == 0 {
		m.content = make([]string, len(c.deletedLines))
		copy(m.content, c.deletedLines)
	} else {
		// Remove the empty line we inserted and restore deleted lines
		contentWithoutEmpty := m.content[:c.startRow]
		newContent := make([]string, len(contentWithoutEmpty)+len(c.deletedLines))
		copy(newContent[:len(contentWithoutEmpty)], contentWithoutEmpty)
		copy(newContent[len(contentWithoutEmpty):], c.deletedLines)
		m.content = newContent
	}

	// Restore cursor position and mode
	m.cursorRow = c.startRow
	m.cursorCol = c.cursorCol
	m.clampCursorCol()
	m.mode = ModeNormal

	return nil
}

// Keys returns the trigger keys for this command.
func (c *ChangeToLastLineCommand) Keys() []string { return []string{"cG"} }

// Mode returns the mode this command operates in.
func (c *ChangeToLastLineCommand) Mode() Mode { return ModeNormal }

// ID returns the hierarchical identifier for this command.
func (c *ChangeToLastLineCommand) ID() string { return "change.to_last_line" }
