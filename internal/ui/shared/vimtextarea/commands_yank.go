package vimtextarea

// ============================================================================
// Yank Commands
// ============================================================================

// YankLineCommand yanks the entire current line (yy command).
// Sets lastYankWasLinewise = true for proper paste behavior.
type YankLineCommand struct {
	MotionBase
	// Capture position for highlight after execute
	highlightRow int
	highlightCol int
}

// Execute yanks the entire current line.
func (c *YankLineCommand) Execute(m *Model) ExecuteResult {
	// Capture position for highlight
	c.highlightRow = m.cursorRow
	c.highlightCol = len(m.content[m.cursorRow])

	// Store current line in yank register
	m.lastYankedText = m.content[m.cursorRow]
	m.lastYankWasLinewise = true

	// Cursor stays in place - yank doesn't move cursor
	return Executed
}

// YankHighlightRegion returns the region to highlight after yank.
func (c *YankLineCommand) YankHighlightRegion() (start, end Position, linewise bool, show bool) {
	return Position{Row: c.highlightRow, Col: 0},
		Position{Row: c.highlightRow, Col: c.highlightCol},
		true, true
}

// Keys returns the trigger keys for this command.
func (c *YankLineCommand) Keys() []string {
	return []string{"yy"}
}

// Mode returns the mode this command operates in.
func (c *YankLineCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *YankLineCommand) ID() string {
	return "yank.line"
}

// YankWordCommand yanks from cursor to start of next word (yw command).
// Sets lastYankWasLinewise = false for proper paste behavior.
type YankWordCommand struct {
	MotionBase
	// Capture positions for highlight after execute
	highlightRow      int
	highlightStartCol int
	highlightEndCol   int
	showHighlight     bool
}

// Execute yanks from cursor to start of next word.
func (c *YankWordCommand) Execute(m *Model) ExecuteResult {
	line := m.content[m.cursorRow]

	// If at end of line or empty line, yank remaining text (may be empty)
	if len(line) == 0 || m.cursorCol >= len(line) {
		m.lastYankedText = ""
		m.lastYankWasLinewise = false
		c.showHighlight = false
		return Executed
	}

	// Find where the next word starts (this is where we yank to)
	endCol := m.findNextWordStart(line, m.cursorCol)

	// Capture for highlight
	c.highlightRow = m.cursorRow
	c.highlightStartCol = m.cursorCol

	// Yank from cursor to end position
	if endCol >= len(line) {
		m.lastYankedText = line[m.cursorCol:]
		c.highlightEndCol = len(line) - 1
	} else {
		m.lastYankedText = line[m.cursorCol:endCol]
		c.highlightEndCol = endCol - 1
	}
	m.lastYankWasLinewise = false
	c.showHighlight = len(m.lastYankedText) > 0

	// Cursor stays in place - yank doesn't move cursor
	return Executed
}

// YankHighlightRegion returns the region to highlight after yank.
func (c *YankWordCommand) YankHighlightRegion() (start, end Position, linewise bool, show bool) {
	return Position{Row: c.highlightRow, Col: c.highlightStartCol},
		Position{Row: c.highlightRow, Col: c.highlightEndCol},
		false, c.showHighlight
}

// Keys returns the trigger keys for this command.
func (c *YankWordCommand) Keys() []string {
	return []string{"yw"}
}

// Mode returns the mode this command operates in.
func (c *YankWordCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *YankWordCommand) ID() string {
	return "yank.word"
}

// YankToEOLCommand yanks from cursor to end of line (y$ or Y command).
// Sets lastYankWasLinewise = false for proper paste behavior.
type YankToEOLCommand struct {
	MotionBase
	// Capture positions for highlight after execute
	highlightRow      int
	highlightStartCol int
	highlightEndCol   int
	showHighlight     bool
}

// Execute yanks from cursor to end of line.
func (c *YankToEOLCommand) Execute(m *Model) ExecuteResult {
	line := m.content[m.cursorRow]

	// If cursor is at or past end, yank empty string
	if m.cursorCol >= len(line) {
		m.lastYankedText = ""
		m.lastYankWasLinewise = false
		c.showHighlight = false
		return Executed
	}

	// Capture for highlight
	c.highlightRow = m.cursorRow
	c.highlightStartCol = m.cursorCol
	c.highlightEndCol = len(line) - 1

	// Yank from cursor to end of line
	m.lastYankedText = line[m.cursorCol:]
	m.lastYankWasLinewise = false
	c.showHighlight = len(m.lastYankedText) > 0

	// Cursor stays in place - yank doesn't move cursor
	return Executed
}

// YankHighlightRegion returns the region to highlight after yank.
func (c *YankToEOLCommand) YankHighlightRegion() (start, end Position, linewise bool, show bool) {
	return Position{Row: c.highlightRow, Col: c.highlightStartCol},
		Position{Row: c.highlightRow, Col: c.highlightEndCol},
		false, c.showHighlight
}

// Keys returns the trigger keys for this command.
// Y is an alias for y$ in Normal mode.
func (c *YankToEOLCommand) Keys() []string {
	return []string{"y$", "Y"}
}

// Mode returns the mode this command operates in.
func (c *YankToEOLCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *YankToEOLCommand) ID() string {
	return "yank.to_eol"
}
