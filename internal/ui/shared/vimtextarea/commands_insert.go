package vimtextarea

import "strings"

// ============================================================================
// Insert Commands
// ============================================================================

// InsertTextCommand inserts text at a specific position.
// Handles character input and paste operations.
type InsertTextCommand struct {
	InsertBase
	row  int    // Row where insertion occurred
	col  int    // Column where insertion started
	text string // Text that was inserted

	// Undo tracking: number of lines added and final cursor position
	// These are computed during Execute for accurate undo
	linesAdded int // Number of new lines added (for multi-line paste)
	endRow     int // Cursor row after insertion
	endCol     int // Cursor col after insertion
}

// Execute inserts text at the stored position.
func (c *InsertTextCommand) Execute(m *Model) ExecuteResult {
	// Handle CharLimit before inserting
	text := c.text
	if m.config.CharLimit > 0 {
		currentLen := m.totalCharCount()
		if currentLen+len(text) > m.config.CharLimit {
			remaining := m.config.CharLimit - currentLen
			if remaining <= 0 {
				return Skipped // At limit, reject insertion
			}
			text = text[:remaining] // Truncate to fit
		}
	}
	c.text = text // Update with potentially truncated text

	// Get current line
	if c.row >= len(m.content) {
		c.row = len(m.content) - 1
	}
	line := m.content[c.row]

	// Clamp col to valid range
	col := min(c.col, len(line))
	c.col = col

	// Check if text contains newlines (multi-line paste)
	if !strings.Contains(c.text, "\n") {
		// Simple case: single line insertion
		m.content[c.row] = line[:col] + c.text + line[col:]
		m.cursorCol = col + len(c.text)
		c.linesAdded = 0
		c.endRow = c.row
		c.endCol = m.cursorCol
	} else {
		// Multi-line paste
		lines := strings.Split(c.text, "\n")
		beforeCursor := line[:col]
		afterCursor := line[col:]

		// First line: text before cursor + first pasted line
		m.content[c.row] = beforeCursor + lines[0]

		// Insert middle and last lines
		newContent := make([]string, len(m.content)+len(lines)-1)
		copy(newContent[:c.row+1], m.content[:c.row+1])

		for i := 1; i < len(lines); i++ {
			if i == len(lines)-1 {
				// Last line: last pasted line + text after cursor
				newContent[c.row+i] = lines[i] + afterCursor
			} else {
				newContent[c.row+i] = lines[i]
			}
		}

		copy(newContent[c.row+len(lines):], m.content[c.row+1:])
		m.content = newContent

		c.linesAdded = len(lines) - 1
		m.cursorRow = c.row + c.linesAdded
		m.cursorCol = len(lines[len(lines)-1])
		c.endRow = m.cursorRow
		c.endCol = m.cursorCol
	}

	return Executed
}

// Undo removes the inserted text.
func (c *InsertTextCommand) Undo(m *Model) error {
	if c.linesAdded == 0 {
		// Simple case: remove inserted text from single line
		line := m.content[c.row]
		if c.col+len(c.text) <= len(line) {
			m.content[c.row] = line[:c.col] + line[c.col+len(c.text):]
		}
	} else {
		// Multi-line: need to reconstruct original single line
		// Get text before insertion point (from first line)
		firstLine := m.content[c.row]
		lines := strings.Split(c.text, "\n")
		beforeCursor := firstLine[:c.col]

		// Get text after insertion point (from last line)
		lastLine := m.content[c.row+c.linesAdded]
		afterCursor := lastLine[len(lines[len(lines)-1]):]

		// Reconstruct original line
		originalLine := beforeCursor + afterCursor

		// Remove inserted lines and restore original
		newContent := make([]string, len(m.content)-c.linesAdded)
		copy(newContent[:c.row], m.content[:c.row])
		newContent[c.row] = originalLine
		copy(newContent[c.row+1:], m.content[c.row+c.linesAdded+1:])
		m.content = newContent
	}

	// Restore cursor position
	m.cursorRow = c.row
	m.cursorCol = c.col

	return nil
}

// Keys returns the trigger keys for this command.
// Note: InsertTextCommand is created for any printable character, so this is a placeholder.
func (c *InsertTextCommand) Keys() []string {
	return []string{"<char>"}
}

// Mode returns the mode this command operates in.
func (c *InsertTextCommand) Mode() Mode {
	return ModeInsert
}

// ID returns the hierarchical identifier for this command.
func (c *InsertTextCommand) ID() string {
	return "insert.text"
}

// SplitLineCommand splits a line at the cursor position (Enter key).
type SplitLineCommand struct {
	InsertBase
	row int // Row where split occurred
	col int // Column where split occurred
}

// Execute splits the line at the cursor position.
func (c *SplitLineCommand) Execute(m *Model) ExecuteResult {
	// Capture position from model
	c.row = m.cursorRow
	c.col = m.cursorCol

	if c.row >= len(m.content) {
		c.row = len(m.content) - 1
	}
	line := m.content[c.row]

	// Clamp col
	col := min(c.col, len(line))
	c.col = col

	// Split the line
	beforeCursor := line[:col]
	afterCursor := line[col:]

	// Create new content with the split
	newContent := make([]string, len(m.content)+1)
	copy(newContent[:c.row], m.content[:c.row])
	newContent[c.row] = beforeCursor
	newContent[c.row+1] = afterCursor
	copy(newContent[c.row+2:], m.content[c.row+1:])
	m.content = newContent

	// Move cursor to start of new line
	m.cursorRow = c.row + 1
	m.cursorCol = 0

	return Executed
}

// Undo rejoins the split lines.
func (c *SplitLineCommand) Undo(m *Model) error {
	if c.row+1 >= len(m.content) {
		return nil // Invalid state
	}

	// Get the two split parts
	firstPart := m.content[c.row]
	secondPart := m.content[c.row+1]

	// Join them back
	newContent := make([]string, len(m.content)-1)
	copy(newContent[:c.row], m.content[:c.row])
	newContent[c.row] = firstPart + secondPart
	copy(newContent[c.row+1:], m.content[c.row+2:])
	m.content = newContent

	// Restore cursor position
	m.cursorRow = c.row
	m.cursorCol = c.col

	return nil
}

// Keys returns the trigger keys for this command.
func (c *SplitLineCommand) Keys() []string {
	return []string{"<alt+enter>"}
}

// Mode returns the mode this command operates in.
func (c *SplitLineCommand) Mode() Mode {
	return ModeInsert
}

// ID returns the hierarchical identifier for this command.
func (c *SplitLineCommand) ID() string {
	return "insert.split_line"
}

// InsertLineCommand inserts a new empty line above or below the current line.
// Used for 'o' and 'O' commands - content change only, mode change handled separately.
type InsertLineCommand struct {
	DeleteBase
	row   int  // Row where line was inserted
	above bool // True if inserted above (O), false if below (o)
}

// Execute inserts a new empty line.
func (c *InsertLineCommand) Execute(m *Model) ExecuteResult {
	var insertAt int
	if c.above {
		insertAt = m.cursorRow
	} else {
		insertAt = m.cursorRow + 1
	}

	// Store the actual insertion row for undo
	c.row = insertAt

	// Insert new empty line
	newContent := make([]string, len(m.content)+1)
	copy(newContent[:insertAt], m.content[:insertAt])
	newContent[insertAt] = ""
	copy(newContent[insertAt+1:], m.content[insertAt:])
	m.content = newContent

	// Move cursor to new line
	m.cursorRow = insertAt
	m.cursorCol = 0

	return Executed
}

// Undo removes the inserted line.
func (c *InsertLineCommand) Undo(m *Model) error {
	if c.row >= len(m.content) {
		return nil // Invalid state
	}

	// Remove the inserted line
	newContent := make([]string, len(m.content)-1)
	copy(newContent[:c.row], m.content[:c.row])
	copy(newContent[c.row:], m.content[c.row+1:])
	m.content = newContent

	// Restore cursor position
	if c.above {
		m.cursorRow = c.row
	} else {
		m.cursorRow = c.row - 1
	}
	if m.cursorRow < 0 {
		m.cursorRow = 0
	}
	if m.cursorRow >= len(m.content) {
		m.cursorRow = len(m.content) - 1
	}
	m.clampCursorCol()

	return nil
}

// Keys returns the trigger keys for this command.
func (c *InsertLineCommand) Keys() []string {
	if c.above {
		return []string{"O"}
	}
	return []string{"o"}
}

// Mode returns the mode this command operates in.
func (c *InsertLineCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *InsertLineCommand) ID() string {
	if c.above {
		return "insert.line_above"
	}
	return "insert.line_below"
}

// ============================================================================
// Space Command (Insert mode)
// ============================================================================

// SpaceCommand inserts a space character.
// This wraps InsertTextCommand but is registered separately because
// Bubble Tea sends space as KeySpace, not KeyRunes.
type SpaceCommand struct {
	InsertBase
	insertCmd *InsertTextCommand
}

// Execute inserts a space character.
func (c *SpaceCommand) Execute(m *Model) ExecuteResult {
	// Check CharLimit before inserting
	if m.config.CharLimit > 0 {
		if m.totalCharCount() >= m.config.CharLimit {
			return Skipped
		}
	}

	c.insertCmd = &InsertTextCommand{
		row:  m.cursorRow,
		col:  m.cursorCol,
		text: " ",
	}
	return c.insertCmd.Execute(m)
}

// Undo removes the inserted space.
func (c *SpaceCommand) Undo(m *Model) error {
	if c.insertCmd != nil {
		return c.insertCmd.Undo(m)
	}
	return nil
}

// Keys returns the trigger keys for this command.
func (c *SpaceCommand) Keys() []string {
	return []string{"<space>"}
}

// Mode returns the mode this command operates in.
func (c *SpaceCommand) Mode() Mode {
	return ModeInsert
}

// ID returns the hierarchical identifier for this command.
func (c *SpaceCommand) ID() string {
	return "insert.space"
}
