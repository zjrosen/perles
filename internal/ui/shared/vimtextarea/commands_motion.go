package vimtextarea

// ============================================================================
// Motion Commands
// ============================================================================
//
// Motion commands implement the Command interface but their Undo is a no-op
// since cursor movements are not undoable in vim semantics.
// These commands are NOT pushed to CommandHistory - they're executed directly.

// MoveLeftCommand moves cursor one character left (h motion).
type MoveLeftCommand struct {
	MotionBase
	prevCol int // Previous column position (for undo, though motions are non-undoable)
}

// Execute moves the cursor one character to the left.
func (c *MoveLeftCommand) Execute(m *Model) ExecuteResult {
	c.prevCol = m.cursorCol
	if m.cursorCol > 0 {
		m.cursorCol--
	}
	// Update preferred column for horizontal movement
	m.preferredCol = m.cursorCol
	return Executed
}

// Keys returns the trigger keys for this command.
func (c *MoveLeftCommand) Keys() []string {
	return []string{"h"}
}

// Mode returns the mode this command operates in.
func (c *MoveLeftCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *MoveLeftCommand) ID() string {
	return "move.left"
}

// MoveRightCommand moves cursor one character right (l motion).
type MoveRightCommand struct {
	MotionBase
	prevCol int
}

// Execute moves the cursor one character to the right.
func (c *MoveRightCommand) Execute(m *Model) ExecuteResult {
	c.prevCol = m.cursorCol
	line := m.content[m.cursorRow]
	// In Normal mode, cursor can go up to GraphemeCount(line)-1 (on the last grapheme)
	maxCol := max(GraphemeCount(line)-1, 0)
	if m.cursorCol < maxCol {
		m.cursorCol++
	}
	// Update preferred column for horizontal movement
	m.preferredCol = m.cursorCol
	return Executed
}

// Keys returns the trigger keys for this command.
func (c *MoveRightCommand) Keys() []string {
	return []string{"l"}
}

// Mode returns the mode this command operates in.
func (c *MoveRightCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *MoveRightCommand) ID() string {
	return "move.right"
}

// MoveDownCommand moves cursor one display line down (j motion).
// Handles soft-wrap by moving within the same logical line.
type MoveDownCommand struct {
	MotionBase
	prevRow int
	prevCol int
}

// Execute moves the cursor one display line down.
func (c *MoveDownCommand) Execute(m *Model) ExecuteResult {
	c.prevRow = m.cursorRow
	c.prevCol = m.cursorCol

	// Move down by one display line (like vim's gj)
	if m.width <= 0 {
		// No width set, fall back to logical line movement
		if m.cursorRow < len(m.content)-1 {
			m.cursorRow++
			m.cursorCol = m.preferredCol
			m.clampCursorCol()
		}
		return Executed
	}

	line := m.content[m.cursorRow]
	graphemeCount := GraphemeCount(line)
	currentWrapLine := m.cursorWrapLine()
	totalWrapLines := m.displayLinesForLine(line)
	colInWrap := m.cursorCol % m.width

	// If we can move down within the same logical line (more wrapped segments)
	if currentWrapLine < totalWrapLines-1 {
		// Move to next wrapped segment, maintaining column within wrap
		// Note: This uses grapheme count; full soft-wrap fix is in Phase 5 (Rendering)
		m.cursorCol = min((currentWrapLine+1)*m.width+colInWrap, graphemeCount)
		// In Normal mode, can't be past last grapheme
		if m.mode == ModeNormal && m.cursorCol > 0 && m.cursorCol >= graphemeCount {
			m.cursorCol = graphemeCount - 1
		}
	} else if m.cursorRow < len(m.content)-1 {
		// Move to next logical line
		m.cursorRow++
		// Position at same column within first wrap segment
		m.cursorCol = colInWrap
		m.clampCursorCol()
	}
	return Executed
}

// Keys returns the trigger keys for this command.
func (c *MoveDownCommand) Keys() []string {
	return []string{"j"}
}

// Mode returns the mode this command operates in.
func (c *MoveDownCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *MoveDownCommand) ID() string {
	return "move.down"
}

// MoveUpCommand moves cursor one display line up (k motion).
// Handles soft-wrap by moving within the same logical line.
type MoveUpCommand struct {
	MotionBase
	prevRow int
	prevCol int
}

// Execute moves the cursor one display line up.
func (c *MoveUpCommand) Execute(m *Model) ExecuteResult {
	c.prevRow = m.cursorRow
	c.prevCol = m.cursorCol

	if m.width <= 0 {
		// No width set, fall back to logical line movement
		if m.cursorRow > 0 {
			m.cursorRow--
			m.cursorCol = m.preferredCol
			m.clampCursorCol()
		}
		return Executed
	}

	currentWrapLine := m.cursorWrapLine()
	colInWrap := m.cursorCol % m.width

	// If we can move up within the same logical line (on a wrapped segment)
	if currentWrapLine > 0 {
		// Move to previous wrapped segment, maintaining column within wrap
		m.cursorCol = (currentWrapLine-1)*m.width + colInWrap
	} else if m.cursorRow > 0 {
		// Move to previous logical line
		m.cursorRow--
		prevLine := m.content[m.cursorRow]
		prevGraphemeCount := GraphemeCount(prevLine)
		prevWrapLines := m.displayLinesForLine(prevLine)
		// Position at same column within last wrap segment of previous line, clamped to grapheme count
		// Note: Full soft-wrap fix is in Phase 5 (Rendering)
		m.cursorCol = min((prevWrapLines-1)*m.width+colInWrap, prevGraphemeCount)
		// In Normal mode, can't be past last grapheme
		if m.mode == ModeNormal && m.cursorCol > 0 && m.cursorCol >= prevGraphemeCount {
			m.cursorCol = prevGraphemeCount - 1
		}
	}
	return Executed
}

// Keys returns the trigger keys for this command.
func (c *MoveUpCommand) Keys() []string {
	return []string{"k"}
}

// Mode returns the mode this command operates in.
func (c *MoveUpCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *MoveUpCommand) ID() string {
	return "move.up"
}

// MoveWordForwardCommand moves cursor to start of next word (w motion).
type MoveWordForwardCommand struct {
	MotionBase
	prevRow int
	prevCol int
}

// Execute moves the cursor to the start of the next word.
func (c *MoveWordForwardCommand) Execute(m *Model) ExecuteResult {
	c.prevRow = m.cursorRow
	c.prevCol = m.cursorCol

	line := m.content[m.cursorRow]
	graphemeCount := GraphemeCount(line)

	// If we're on the current line, try to find next word on this line
	if m.cursorCol < graphemeCount {
		nextWordCol := m.findNextWordStart(line, m.cursorCol)
		// nextWordCol > cursorCol AND nextWordCol < graphemeCount means we found a word on this line
		if nextWordCol > m.cursorCol && nextWordCol < graphemeCount {
			m.cursorCol = nextWordCol
			m.preferredCol = m.cursorCol
			return Executed
		}
	}

	// Move to next line if we couldn't find a word on current line
	if m.cursorRow < len(m.content)-1 {
		m.cursorRow++
		line = m.content[m.cursorRow]
		graphemeCount = GraphemeCount(line)
		// Find first word on the new line
		m.cursorCol = m.findFirstWordStart(line)
		// Handle empty lines - cursor stays at 0
		if m.cursorCol >= graphemeCount {
			m.cursorCol = 0
		}
		m.preferredCol = m.cursorCol
	} else if graphemeCount > 0 {
		// At last line with no next word - move to end of line (last grapheme in Normal mode)
		m.cursorCol = graphemeCount - 1
		m.preferredCol = m.cursorCol
	}

	return Executed
}

// Keys returns the trigger keys for this command.
func (c *MoveWordForwardCommand) Keys() []string {
	return []string{"w"}
}

// Mode returns the mode this command operates in.
func (c *MoveWordForwardCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *MoveWordForwardCommand) ID() string {
	return "move.word_forward"
}

// MoveWordBackwardCommand moves cursor to start of previous word (b motion).
type MoveWordBackwardCommand struct {
	MotionBase
	prevRow int
	prevCol int
}

// Execute moves the cursor to the start of the previous word.
func (c *MoveWordBackwardCommand) Execute(m *Model) ExecuteResult {
	c.prevRow = m.cursorRow
	c.prevCol = m.cursorCol

	line := m.content[m.cursorRow]

	// If we're on the current line, try to find previous word on this line
	if m.cursorCol > 0 {
		prevWordCol := m.findPrevWordStart(line, m.cursorCol)
		if prevWordCol < m.cursorCol {
			m.cursorCol = prevWordCol
			m.preferredCol = m.cursorCol
			return Executed
		}
	}

	// Move to previous line if we couldn't find a word on current line
	if m.cursorRow > 0 {
		m.cursorRow--
		line = m.content[m.cursorRow]
		// Find last word start on the new line
		m.cursorCol = m.findLastWordStart(line)
		m.preferredCol = m.cursorCol
	}
	// If at first line and start of content, stay at current position

	return Executed
}

// Keys returns the trigger keys for this command.
func (c *MoveWordBackwardCommand) Keys() []string {
	return []string{"b"}
}

// Mode returns the mode this command operates in.
func (c *MoveWordBackwardCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *MoveWordBackwardCommand) ID() string {
	return "move.word_backward"
}

// MoveWordEndCommand moves cursor to end of current/next word (e motion).
type MoveWordEndCommand struct {
	MotionBase
	prevRow int
	prevCol int
}

// Execute moves the cursor to the end of the current/next word.
func (c *MoveWordEndCommand) Execute(m *Model) ExecuteResult {
	c.prevRow = m.cursorRow
	c.prevCol = m.cursorCol

	line := m.content[m.cursorRow]
	graphemeCount := GraphemeCount(line)

	// Try to find word end on current line
	if m.cursorCol < graphemeCount {
		nextEndCol := m.findWordEnd(line, m.cursorCol)
		// -1 means no word end found on this line
		if nextEndCol >= 0 && nextEndCol > m.cursorCol {
			m.cursorCol = nextEndCol
			m.preferredCol = m.cursorCol
			return Executed
		}
	}

	// Move to next line if we couldn't find a word end on current line
	for m.cursorRow < len(m.content)-1 {
		m.cursorRow++
		line = m.content[m.cursorRow]
		graphemeCount = GraphemeCount(line)

		// Check if this line is empty
		if graphemeCount == 0 {
			// Empty line - position at col 0 and return
			m.cursorCol = 0
			m.preferredCol = 0
			return Executed
		}

		// Find end of first word on the new line
		wordEnd := m.findFirstWordEnd(line)
		if wordEnd >= 0 {
			// Found a word on this line
			m.cursorCol = wordEnd
			m.preferredCol = m.cursorCol
			return Executed
		}

		// Line is all whitespace, continue to next line
		m.cursorCol = 0
		m.preferredCol = 0
	}
	// If at last line and end of content, stay at current position

	return Executed
}

// Keys returns the trigger keys for this command.
func (c *MoveWordEndCommand) Keys() []string {
	return []string{"e"}
}

// Mode returns the mode this command operates in.
func (c *MoveWordEndCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *MoveWordEndCommand) ID() string {
	return "move.word_end"
}

// MoveToLineStartCommand moves cursor to first column (0 motion).
type MoveToLineStartCommand struct {
	MotionBase
	prevCol int
}

// Execute moves the cursor to the first column.
func (c *MoveToLineStartCommand) Execute(m *Model) ExecuteResult {
	c.prevCol = m.cursorCol
	m.cursorCol = 0
	m.preferredCol = 0
	return Executed
}

// Keys returns the trigger keys for this command.
func (c *MoveToLineStartCommand) Keys() []string {
	return []string{"0"}
}

// Mode returns the mode this command operates in.
func (c *MoveToLineStartCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *MoveToLineStartCommand) ID() string {
	return "move.line_start"
}

// MoveToLineEndCommand moves cursor to last column ($ motion).
type MoveToLineEndCommand struct {
	MotionBase
	prevCol int
}

// Execute moves the cursor to the last column.
func (c *MoveToLineEndCommand) Execute(m *Model) ExecuteResult {
	c.prevCol = m.cursorCol
	line := m.content[m.cursorRow]
	graphemeCount := GraphemeCount(line)
	if graphemeCount > 0 {
		m.cursorCol = graphemeCount - 1
	} else {
		m.cursorCol = 0
	}
	m.preferredCol = m.cursorCol
	return Executed
}

// Keys returns the trigger keys for this command.
func (c *MoveToLineEndCommand) Keys() []string {
	return []string{"$"}
}

// Mode returns the mode this command operates in.
func (c *MoveToLineEndCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *MoveToLineEndCommand) ID() string {
	return "move.line_end"
}

// MoveToFirstNonBlankCommand moves cursor to first non-blank character (^ motion).
type MoveToFirstNonBlankCommand struct {
	MotionBase
	prevCol int
}

// Execute moves the cursor to the first non-blank character.
func (c *MoveToFirstNonBlankCommand) Execute(m *Model) ExecuteResult {
	c.prevCol = m.cursorCol
	line := m.content[m.cursorRow]

	// Iterate through graphemes to find first non-blank
	iter := NewGraphemeIterator(line)
	for iter.Next() {
		cluster := iter.Cluster()
		// Check if this grapheme is whitespace
		if graphemeType(cluster) != graphemeWhitespace {
			m.cursorCol = iter.Index()
			m.preferredCol = m.cursorCol
			return Executed
		}
	}
	// No non-blank found, go to column 0
	m.cursorCol = 0
	m.preferredCol = 0
	return Executed
}

// Keys returns the trigger keys for this command.
func (c *MoveToFirstNonBlankCommand) Keys() []string {
	return []string{"^"}
}

// Mode returns the mode this command operates in.
func (c *MoveToFirstNonBlankCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *MoveToFirstNonBlankCommand) ID() string {
	return "move.first_non_blank"
}

// MoveToFirstLineCommand moves cursor to first line (gg motion).
type MoveToFirstLineCommand struct {
	MotionBase
	prevRow int
	prevCol int
}

// Execute moves the cursor to the first line.
func (c *MoveToFirstLineCommand) Execute(m *Model) ExecuteResult {
	c.prevRow = m.cursorRow
	c.prevCol = m.cursorCol
	m.cursorRow = 0
	m.clampCursorCol()
	return Executed
}

// Keys returns the trigger keys for this command.
func (c *MoveToFirstLineCommand) Keys() []string {
	return []string{"gg"}
}

// Mode returns the mode this command operates in.
func (c *MoveToFirstLineCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *MoveToFirstLineCommand) ID() string {
	return "move.first_line"
}

// MoveToLastLineCommand moves cursor to last line (G motion).
type MoveToLastLineCommand struct {
	MotionBase
	prevRow int
	prevCol int
}

// Execute moves the cursor to the last line.
func (c *MoveToLastLineCommand) Execute(m *Model) ExecuteResult {
	c.prevRow = m.cursorRow
	c.prevCol = m.cursorCol
	m.cursorRow = len(m.content) - 1
	m.clampCursorCol()
	return Executed
}

// Keys returns the trigger keys for this command.
func (c *MoveToLastLineCommand) Keys() []string {
	return []string{"G"}
}

// Mode returns the mode this command operates in.
func (c *MoveToLastLineCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *MoveToLastLineCommand) ID() string {
	return "move.last_line"
}

// ============================================================================
// Readline-style Cursor Movement Commands (Insert mode)
// ============================================================================

// MoveToLineStartInsertCommand moves cursor to start of line (Ctrl+A).
// Standard readline/emacs binding that works in Insert mode.
type MoveToLineStartInsertCommand struct {
	MotionBase
}

// Execute moves cursor to start of line.
func (c *MoveToLineStartInsertCommand) Execute(m *Model) ExecuteResult {
	m.cursorCol = 0
	return Executed
}

// Keys returns the trigger keys for this command.
func (c *MoveToLineStartInsertCommand) Keys() []string { return []string{"<ctrl+a>"} }

// Mode returns the mode this command operates in.
func (c *MoveToLineStartInsertCommand) Mode() Mode { return ModeInsert }

// ID returns the hierarchical identifier for this command.
func (c *MoveToLineStartInsertCommand) ID() string { return "move.line_start_insert" }

// MoveToLineEndInsertCommand moves cursor to end of line (Ctrl+E).
// Standard readline/emacs binding that works in Insert mode.
type MoveToLineEndInsertCommand struct {
	MotionBase
}

// Execute moves cursor to end of line.
func (c *MoveToLineEndInsertCommand) Execute(m *Model) ExecuteResult {
	// In Insert mode, cursor can be past the last grapheme (for insertion)
	m.cursorCol = GraphemeCount(m.content[m.cursorRow])
	return Executed
}

// Keys returns the trigger keys for this command.
func (c *MoveToLineEndInsertCommand) Keys() []string { return []string{"<ctrl+e>"} }

// Mode returns the mode this command operates in.
func (c *MoveToLineEndInsertCommand) Mode() Mode { return ModeInsert }

// ID returns the hierarchical identifier for this command.
func (c *MoveToLineEndInsertCommand) ID() string { return "move.line_end_insert" }

// ============================================================================
// Arrow Key Commands (work in both Normal and Insert modes)
// ============================================================================

// ArrowLeftCommand moves cursor left (left arrow key).
// Works in Insert mode - cursor movement only.
type ArrowLeftCommand struct {
	MotionBase
}

// Execute moves cursor left.
func (c *ArrowLeftCommand) Execute(m *Model) ExecuteResult {
	if m.cursorCol > 0 {
		m.cursorCol--
	}
	m.preferredCol = m.cursorCol
	return Executed
}

// Keys returns the trigger keys for this command.
func (c *ArrowLeftCommand) Keys() []string { return []string{"<left>", "<ctrl+b>"} }

// Mode returns the mode this command operates in.
func (c *ArrowLeftCommand) Mode() Mode { return ModeInsert }

// ID returns the hierarchical identifier for this command.
func (c *ArrowLeftCommand) ID() string { return "arrow.left" }

// ArrowRightCommand moves cursor right (right arrow key).
// Works in Insert mode - allows cursor past last char for insertion.
type ArrowRightCommand struct {
	MotionBase
}

// Execute moves cursor right.
func (c *ArrowRightCommand) Execute(m *Model) ExecuteResult {
	line := m.content[m.cursorRow]
	// In Insert mode, cursor can go up to GraphemeCount(line) (past last grapheme)
	if m.cursorCol < GraphemeCount(line) {
		m.cursorCol++
	}
	m.preferredCol = m.cursorCol
	return Executed
}

// Keys returns the trigger keys for this command.
func (c *ArrowRightCommand) Keys() []string { return []string{"<right>", "<ctrl+f>"} }

// Mode returns the mode this command operates in.
func (c *ArrowRightCommand) Mode() Mode { return ModeInsert }

// ID returns the hierarchical identifier for this command.
func (c *ArrowRightCommand) ID() string { return "arrow.right" }

// ArrowUpCommand moves cursor up (up arrow key).
// Works in Insert mode.
type ArrowUpCommand struct {
	MotionBase
}

// Execute moves cursor up one line.
func (c *ArrowUpCommand) Execute(m *Model) ExecuteResult {
	if m.cursorRow > 0 {
		m.cursorRow--
		// Maintain preferred column or clamp to grapheme count
		line := m.content[m.cursorRow]
		graphemeCount := GraphemeCount(line)
		if m.cursorCol > graphemeCount {
			m.cursorCol = graphemeCount
		}
	}
	return Executed
}

// Keys returns the trigger keys for this command.
func (c *ArrowUpCommand) Keys() []string { return []string{"<up>"} }

// Mode returns the mode this command operates in.
func (c *ArrowUpCommand) Mode() Mode { return ModeInsert }

// ID returns the hierarchical identifier for this command.
func (c *ArrowUpCommand) ID() string { return "arrow.up" }

// ArrowDownCommand moves cursor down (down arrow key).
// Works in Insert mode.
type ArrowDownCommand struct {
	MotionBase
}

// Execute moves cursor down one line.
func (c *ArrowDownCommand) Execute(m *Model) ExecuteResult {
	if m.cursorRow < len(m.content)-1 {
		m.cursorRow++
		// Maintain preferred column or clamp to grapheme count
		line := m.content[m.cursorRow]
		graphemeCount := GraphemeCount(line)
		if m.cursorCol > graphemeCount {
			m.cursorCol = graphemeCount
		}
	}
	return Executed
}

// Keys returns the trigger keys for this command.
func (c *ArrowDownCommand) Keys() []string { return []string{"<down>"} }

// Mode returns the mode this command operates in.
func (c *ArrowDownCommand) Mode() Mode { return ModeInsert }

// ID returns the hierarchical identifier for this command.
func (c *ArrowDownCommand) ID() string { return "arrow.down" }

// IsUndoable returns false.
func (c *ArrowDownCommand) IsUndoable() bool { return false }

// ChangesContent returns false.
func (c *ArrowDownCommand) ChangesContent() bool { return false }

// IsModeChange returns false.
func (c *ArrowDownCommand) IsModeChange() bool { return false }

// ============================================================================
// Word Motion Helpers
// ============================================================================
//
// All positions (pos parameter and return values) are GRAPHEME INDICES,
// not byte offsets. This ensures correct navigation with emoji and
// multi-byte Unicode characters.

// findNextWordStart finds the start of the next word from grapheme position pos.
// Returns the grapheme index, or grapheme count if no word found.
func (m Model) findNextWordStart(line string, pos int) int {
	n := GraphemeCount(line)
	if pos >= n {
		return pos
	}

	// Build array of graphemes and their types for easier bidirectional access
	graphemes := GraphemesInRange(line, 0, n)
	if len(graphemes) == 0 {
		return pos
	}

	// First, skip the current word or whitespace we're on
	if pos < n {
		currentType := graphemeType(graphemes[pos])
		// Skip current word characters
		for pos < n && graphemeType(graphemes[pos]) == currentType && currentType != graphemeWhitespace {
			pos++
		}
	}

	// Then skip any whitespace
	for pos < n && graphemeType(graphemes[pos]) == graphemeWhitespace {
		pos++
	}

	return pos
}

// findPrevWordStart finds the start of the previous word from grapheme position pos.
// Returns the grapheme index, or 0 if no word found.
func (m Model) findPrevWordStart(line string, pos int) int {
	if pos <= 0 {
		return 0
	}

	n := GraphemeCount(line)
	if n == 0 {
		return 0
	}

	// Build array of graphemes for easier bidirectional access
	graphemes := GraphemesInRange(line, 0, n)

	// Move back one to start looking
	pos--

	// Clamp pos to valid range after decrementing
	if pos >= n {
		pos = n - 1
	}
	if pos < 0 {
		return 0
	}

	// Skip whitespace backward
	for pos > 0 && graphemeType(graphemes[pos]) == graphemeWhitespace {
		pos--
	}

	if pos <= 0 {
		// Check if position 0 is whitespace
		if graphemeType(graphemes[0]) == graphemeWhitespace {
			return 0
		}
		return 0
	}

	// Find the type of the word we're in
	wordType := graphemeType(graphemes[pos])

	// Skip backward through the word
	for pos > 0 && graphemeType(graphemes[pos-1]) == wordType {
		pos--
	}

	return pos
}

// findFirstWordStart finds the start of the first word on a line.
// Returns 0 if line is empty or has no words.
func (m Model) findFirstWordStart(line string) int {
	n := GraphemeCount(line)
	if n == 0 {
		return 0
	}

	// Build array of graphemes
	graphemes := GraphemesInRange(line, 0, n)

	// Skip leading whitespace
	pos := 0
	for pos < n && graphemeType(graphemes[pos]) == graphemeWhitespace {
		pos++
	}
	return pos
}

// findLastWordStart finds the start of the last word on a line.
// Returns grapheme count - 1 or 0 for empty lines.
func (m Model) findLastWordStart(line string) int {
	n := GraphemeCount(line)
	if n == 0 {
		return 0
	}

	// Build array of graphemes
	graphemes := GraphemesInRange(line, 0, n)

	pos := n - 1

	// Skip trailing whitespace
	for pos > 0 && graphemeType(graphemes[pos]) == graphemeWhitespace {
		pos--
	}

	if pos <= 0 {
		return 0
	}

	// Find the type of the word we're in
	wordType := graphemeType(graphemes[pos])

	// Skip backward through the word to find its start
	for pos > 0 && graphemeType(graphemes[pos-1]) == wordType {
		pos--
	}

	return pos
}

// findWordEnd finds the end of the current/next word from grapheme position pos.
// Returns the grapheme index of the last grapheme of the word (not after it).
// Returns -1 if no word end found on this line (caller should try next line).
// If we're not at the end of a word, we move to the end of the current word.
// If we're already at the end of a word, we move to the end of the next word.
func (m Model) findWordEnd(line string, pos int) int {
	n := GraphemeCount(line)
	if pos >= n {
		return -1 // No word to find
	}

	// Build array of graphemes
	graphemes := GraphemesInRange(line, 0, n)
	if len(graphemes) == 0 {
		return -1
	}

	// Get the type of grapheme we're on
	currentType := graphemeType(graphemes[pos])

	// Check if we're on whitespace - skip to next word
	if currentType == graphemeWhitespace {
		for pos < n && graphemeType(graphemes[pos]) == graphemeWhitespace {
			pos++
		}
		if pos >= n {
			return -1 // No word found, only whitespace to end of line
		}
		// Now find end of this word
		wordType := graphemeType(graphemes[pos])
		for pos+1 < n && graphemeType(graphemes[pos+1]) == wordType {
			pos++
		}
		return pos
	}

	// We're on a word character. Check if we're NOT at the end of the word.
	// If there's a next grapheme of the same type, we're in the middle - go to end.
	if pos+1 < n && graphemeType(graphemes[pos+1]) == currentType {
		// Move to end of current word
		for pos+1 < n && graphemeType(graphemes[pos+1]) == currentType {
			pos++
		}
		return pos
	}

	// We're at the end of a word (single-grapheme word or last grapheme of word).
	// Move past this word and find the next word.
	pos++

	// Skip any whitespace
	for pos < n && graphemeType(graphemes[pos]) == graphemeWhitespace {
		pos++
	}

	if pos >= n {
		// No next word on this line
		return -1
	}

	// Find end of next word
	wordType := graphemeType(graphemes[pos])
	for pos+1 < n && graphemeType(graphemes[pos+1]) == wordType {
		pos++
	}

	return pos
}

// findFirstWordEnd finds the end of the first word on a line.
// Returns the grapheme index of the last grapheme of the first word.
func (m Model) findFirstWordEnd(line string) int {
	n := GraphemeCount(line)
	if n == 0 {
		return 0
	}

	// Build array of graphemes
	graphemes := GraphemesInRange(line, 0, n)

	// Skip leading whitespace
	pos := 0
	for pos < n && graphemeType(graphemes[pos]) == graphemeWhitespace {
		pos++
	}

	if pos >= n {
		return 0
	}

	// Find the type of the word
	wordType := graphemeType(graphemes[pos])

	// Move to end of this word
	for pos+1 < n && graphemeType(graphemes[pos+1]) == wordType {
		pos++
	}

	return pos
}
