package vimtextarea

import "strings"

// ============================================================================
// Paste Commands
// ============================================================================

// PasteAfterCommand pastes text after cursor (p command).
// Behavior differs based on lastYankWasLinewise:
// - Character-wise: insert after cursor position on same line
// - Line-wise: insert new line below current line
type PasteAfterCommand struct {
	InsertBase
	row         int    // Row where paste occurred
	col         int    // Column where paste started
	pastedText  string // Text that was pasted
	wasLinewise bool   // Whether the paste was line-wise
	linesAdded  int    // Number of lines added (for undo)
	originalRow int    // Original cursor row (for undo)
	originalCol int    // Original cursor col (for undo)
	endRow      int    // Cursor row after paste
	endCol      int    // Cursor col after paste
	insertedAt  int    // Row where line-wise text was inserted
}

// Execute pastes text after cursor.
func (c *PasteAfterCommand) Execute(m *Model) ExecuteResult {
	// Skip if nothing to paste
	if m.lastYankedText == "" {
		return Skipped
	}

	// Capture state for undo
	c.pastedText = m.lastYankedText
	c.wasLinewise = m.lastYankWasLinewise
	c.originalRow = m.cursorRow
	c.originalCol = m.cursorCol

	if c.wasLinewise {
		return c.executeLinewise(m)
	}
	return c.executeCharacterwise(m)
}

// executeLinewise inserts a new line below current line with pasted text.
func (c *PasteAfterCommand) executeLinewise(m *Model) ExecuteResult {
	// Insert new line below current line
	insertAt := m.cursorRow + 1
	c.insertedAt = insertAt

	// Handle multi-line yanked text
	lines := strings.Split(c.pastedText, "\n")
	c.linesAdded = len(lines)

	// Create new content with inserted lines
	newContent := make([]string, len(m.content)+c.linesAdded)
	copy(newContent[:insertAt], m.content[:insertAt])
	for i, line := range lines {
		newContent[insertAt+i] = line
	}
	copy(newContent[insertAt+c.linesAdded:], m.content[insertAt:])
	m.content = newContent

	// Move cursor to first non-blank of new line
	m.cursorRow = insertAt
	m.cursorCol = findFirstNonBlank(m.content[insertAt])
	c.endRow = m.cursorRow
	c.endCol = m.cursorCol

	return Executed
}

// executeCharacterwise inserts text after cursor position.
func (c *PasteAfterCommand) executeCharacterwise(m *Model) ExecuteResult {
	c.row = m.cursorRow
	line := m.content[c.row]

	// Insert after cursor position (at cursorCol + 1 if not at end of line)
	// cursorCol is a grapheme index, so compare with grapheme count
	insertCol := m.cursorCol
	lineGraphemeCount := GraphemeCount(line)
	if lineGraphemeCount > 0 && m.cursorCol < lineGraphemeCount {
		insertCol = m.cursorCol + 1
	}
	c.col = insertCol

	// Check if pasted text contains newlines
	if !strings.Contains(c.pastedText, "\n") {
		// Simple single-line paste
		// Use grapheme-aware slicing to prevent UTF-8 corruption
		beforePaste := SliceByGraphemes(line, 0, insertCol)
		afterPaste := SliceByGraphemes(line, insertCol, lineGraphemeCount)
		m.content[c.row] = beforePaste + c.pastedText + afterPaste

		// Cursor on last pasted character (grapheme count)
		pastedGraphemeCount := GraphemeCount(c.pastedText)
		m.cursorCol = max(insertCol+pastedGraphemeCount-1, 0)
		c.linesAdded = 0
		c.endRow = c.row
		c.endCol = m.cursorCol
	} else {
		// Multi-line character-wise paste
		lines := strings.Split(c.pastedText, "\n")
		// Use grapheme-aware slicing
		beforeCursor := SliceByGraphemes(line, 0, insertCol)
		afterCursor := SliceByGraphemes(line, insertCol, lineGraphemeCount)

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
		// Cursor at end of last pasted content (before afterCursor) - use grapheme count
		lastLineGraphemeCount := GraphemeCount(lines[len(lines)-1])
		m.cursorCol = max(lastLineGraphemeCount-1, 0)
		c.endRow = m.cursorRow
		c.endCol = m.cursorCol
	}

	return Executed
}

// Undo removes the pasted text.
func (c *PasteAfterCommand) Undo(m *Model) error {
	if c.wasLinewise {
		return c.undoLinewise(m)
	}
	return c.undoCharacterwise(m)
}

// undoLinewise removes the inserted lines.
func (c *PasteAfterCommand) undoLinewise(m *Model) error {
	// Remove the inserted lines
	newContent := make([]string, len(m.content)-c.linesAdded)
	copy(newContent[:c.insertedAt], m.content[:c.insertedAt])
	copy(newContent[c.insertedAt:], m.content[c.insertedAt+c.linesAdded:])
	m.content = newContent

	// Restore cursor position
	m.cursorRow = c.originalRow
	m.cursorCol = c.originalCol

	return nil
}

// undoCharacterwise removes the pasted text from the line.
func (c *PasteAfterCommand) undoCharacterwise(m *Model) error {
	if c.linesAdded == 0 {
		// Simple single-line undo
		line := m.content[c.row]
		lineGraphemeCount := GraphemeCount(line)
		pastedGraphemeCount := GraphemeCount(c.pastedText)
		// Use grapheme-aware slicing
		if c.col+pastedGraphemeCount <= lineGraphemeCount {
			beforePaste := SliceByGraphemes(line, 0, c.col)
			afterPaste := SliceByGraphemes(line, c.col+pastedGraphemeCount, lineGraphemeCount)
			m.content[c.row] = beforePaste + afterPaste
		}
	} else {
		// Multi-line undo: reconstruct original single line
		lines := strings.Split(c.pastedText, "\n")
		firstLine := m.content[c.row]
		// Use grapheme-aware slicing
		beforeCursor := SliceByGraphemes(firstLine, 0, c.col)

		// Get text after pasted content (from last line)
		lastLine := m.content[c.row+c.linesAdded]
		lastPastedLineGraphemeCount := GraphemeCount(lines[len(lines)-1])
		lastLineGraphemeCount := GraphemeCount(lastLine)
		afterCursor := SliceByGraphemes(lastLine, lastPastedLineGraphemeCount, lastLineGraphemeCount)

		// Reconstruct original line
		originalLine := beforeCursor + afterCursor

		// Remove pasted lines and restore original
		newContent := make([]string, len(m.content)-c.linesAdded)
		copy(newContent[:c.row], m.content[:c.row])
		newContent[c.row] = originalLine
		copy(newContent[c.row+1:], m.content[c.row+c.linesAdded+1:])
		m.content = newContent
	}

	// Restore cursor position
	m.cursorRow = c.originalRow
	m.cursorCol = c.originalCol

	return nil
}

// Keys returns the trigger keys for this command.
func (c *PasteAfterCommand) Keys() []string {
	return []string{"p"}
}

// Mode returns the mode this command operates in.
func (c *PasteAfterCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *PasteAfterCommand) ID() string {
	return "paste.after"
}

// ============================================================================
// PasteBeforeCommand
// ============================================================================

// PasteBeforeCommand pastes text before cursor (P command).
// Behavior differs based on lastYankWasLinewise:
// - Character-wise: insert before cursor position on same line
// - Line-wise: insert new line above current line
type PasteBeforeCommand struct {
	InsertBase
	row         int    // Row where paste occurred
	col         int    // Column where paste started
	pastedText  string // Text that was pasted
	wasLinewise bool   // Whether the paste was line-wise
	linesAdded  int    // Number of lines added (for undo)
	originalRow int    // Original cursor row (for undo)
	originalCol int    // Original cursor col (for undo)
	endRow      int    // Cursor row after paste
	endCol      int    // Cursor col after paste
	insertedAt  int    // Row where line-wise text was inserted
}

// Execute pastes text before cursor.
func (c *PasteBeforeCommand) Execute(m *Model) ExecuteResult {
	// Skip if nothing to paste
	if m.lastYankedText == "" {
		return Skipped
	}

	// Capture state for undo
	c.pastedText = m.lastYankedText
	c.wasLinewise = m.lastYankWasLinewise
	c.originalRow = m.cursorRow
	c.originalCol = m.cursorCol

	if c.wasLinewise {
		return c.executeLinewise(m)
	}
	return c.executeCharacterwise(m)
}

// executeLinewise inserts a new line above current line with pasted text.
func (c *PasteBeforeCommand) executeLinewise(m *Model) ExecuteResult {
	// Insert new line above current line
	insertAt := m.cursorRow
	c.insertedAt = insertAt

	// Handle multi-line yanked text
	lines := strings.Split(c.pastedText, "\n")
	c.linesAdded = len(lines)

	// Create new content with inserted lines
	newContent := make([]string, len(m.content)+c.linesAdded)
	copy(newContent[:insertAt], m.content[:insertAt])
	for i, line := range lines {
		newContent[insertAt+i] = line
	}
	copy(newContent[insertAt+c.linesAdded:], m.content[insertAt:])
	m.content = newContent

	// Move cursor to first non-blank of inserted line
	m.cursorRow = insertAt
	m.cursorCol = findFirstNonBlank(m.content[insertAt])
	c.endRow = m.cursorRow
	c.endCol = m.cursorCol

	return Executed
}

// executeCharacterwise inserts text before cursor position.
func (c *PasteBeforeCommand) executeCharacterwise(m *Model) ExecuteResult {
	c.row = m.cursorRow
	line := m.content[c.row]

	// Insert at cursor position (before current character)
	// cursorCol is a grapheme index
	insertCol := m.cursorCol
	c.col = insertCol

	lineGraphemeCount := GraphemeCount(line)

	// Check if pasted text contains newlines
	if !strings.Contains(c.pastedText, "\n") {
		// Simple single-line paste
		// Use grapheme-aware slicing to prevent UTF-8 corruption
		beforePaste := SliceByGraphemes(line, 0, insertCol)
		afterPaste := SliceByGraphemes(line, insertCol, lineGraphemeCount)
		m.content[c.row] = beforePaste + c.pastedText + afterPaste

		// Cursor at end of pasted text (on last pasted char) - use grapheme count
		pastedGraphemeCount := GraphemeCount(c.pastedText)
		m.cursorCol = max(insertCol+pastedGraphemeCount-1, 0)
		c.linesAdded = 0
		c.endRow = c.row
		c.endCol = m.cursorCol
	} else {
		// Multi-line character-wise paste
		lines := strings.Split(c.pastedText, "\n")
		// Use grapheme-aware slicing
		beforeCursor := SliceByGraphemes(line, 0, insertCol)
		afterCursor := SliceByGraphemes(line, insertCol, lineGraphemeCount)

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
		// Cursor at end of last pasted content (before afterCursor) - use grapheme count
		lastLineGraphemeCount := GraphemeCount(lines[len(lines)-1])
		m.cursorCol = max(lastLineGraphemeCount-1, 0)
		c.endRow = m.cursorRow
		c.endCol = m.cursorCol
	}

	return Executed
}

// Undo removes the pasted text.
func (c *PasteBeforeCommand) Undo(m *Model) error {
	if c.wasLinewise {
		return c.undoLinewise(m)
	}
	return c.undoCharacterwise(m)
}

// undoLinewise removes the inserted lines.
func (c *PasteBeforeCommand) undoLinewise(m *Model) error {
	// Remove the inserted lines
	newContent := make([]string, len(m.content)-c.linesAdded)
	copy(newContent[:c.insertedAt], m.content[:c.insertedAt])
	copy(newContent[c.insertedAt:], m.content[c.insertedAt+c.linesAdded:])
	m.content = newContent

	// Restore cursor position - need to adjust since lines were inserted above
	m.cursorRow = c.originalRow
	if m.cursorRow >= len(m.content) {
		m.cursorRow = len(m.content) - 1
	}
	m.cursorCol = c.originalCol

	return nil
}

// undoCharacterwise removes the pasted text from the line.
func (c *PasteBeforeCommand) undoCharacterwise(m *Model) error {
	if c.linesAdded == 0 {
		// Simple single-line undo
		line := m.content[c.row]
		lineGraphemeCount := GraphemeCount(line)
		pastedGraphemeCount := GraphemeCount(c.pastedText)
		// Use grapheme-aware slicing
		if c.col+pastedGraphemeCount <= lineGraphemeCount {
			beforePaste := SliceByGraphemes(line, 0, c.col)
			afterPaste := SliceByGraphemes(line, c.col+pastedGraphemeCount, lineGraphemeCount)
			m.content[c.row] = beforePaste + afterPaste
		}
	} else {
		// Multi-line undo: reconstruct original single line
		lines := strings.Split(c.pastedText, "\n")
		firstLine := m.content[c.row]
		// Use grapheme-aware slicing
		beforeCursor := SliceByGraphemes(firstLine, 0, c.col)

		// Get text after pasted content (from last line)
		lastLine := m.content[c.row+c.linesAdded]
		lastPastedLineGraphemeCount := GraphemeCount(lines[len(lines)-1])
		lastLineGraphemeCount := GraphemeCount(lastLine)
		afterCursor := SliceByGraphemes(lastLine, lastPastedLineGraphemeCount, lastLineGraphemeCount)

		// Reconstruct original line
		originalLine := beforeCursor + afterCursor

		// Remove pasted lines and restore original
		newContent := make([]string, len(m.content)-c.linesAdded)
		copy(newContent[:c.row], m.content[:c.row])
		newContent[c.row] = originalLine
		copy(newContent[c.row+1:], m.content[c.row+c.linesAdded+1:])
		m.content = newContent
	}

	// Restore cursor position
	m.cursorRow = c.originalRow
	m.cursorCol = c.originalCol

	return nil
}

// Keys returns the trigger keys for this command.
func (c *PasteBeforeCommand) Keys() []string {
	return []string{"P"}
}

// Mode returns the mode this command operates in.
func (c *PasteBeforeCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *PasteBeforeCommand) ID() string {
	return "paste.before"
}

// ============================================================================
// Helper Functions
// ============================================================================

// findFirstNonBlank returns the column of the first non-blank character in line.
// Returns 0 if line is empty or all blanks.
func findFirstNonBlank(line string) int {
	for i, ch := range line {
		if ch != ' ' && ch != '\t' {
			return i
		}
	}
	return 0
}
