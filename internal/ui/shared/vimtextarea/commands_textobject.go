package vimtextarea

import "fmt"

// ============================================================================
// Text Object Commands
// ============================================================================

// DeleteTextObjectCommand deletes a text object (diw, daw, etc.).
// It uses the TextObjectFinder interface to locate text object bounds.
type DeleteTextObjectCommand struct {
	DeleteBase
	object      rune     // Text object type ('w', 'W', '"', etc.)
	inner       bool     // true for 'inner' (i), false for 'around' (a)
	row         int      // Row where deletion occurred (for undo)
	col         int      // Original cursor column (for undo)
	deletedText string   // Text that was deleted (for undo)
	startPos    Position // Start position of deleted region
	endPos      Position // End position of deleted region
}

// Execute deletes the text object at the cursor position.
func (c *DeleteTextObjectCommand) Execute(m *Model) ExecuteResult {
	// Look up the text object finder
	finder, ok := textObjectRegistry[c.object]
	if !ok {
		return Skipped
	}

	// Find the text object bounds
	start, end, found := finder.FindBounds(m, c.inner)
	if !found {
		return Skipped
	}

	// Capture state for undo
	c.row = m.cursorRow
	c.col = m.cursorCol
	c.startPos = start
	c.endPos = end
	c.deletedText = extractText(m.content, start, end)

	// Delete the text object
	line := m.content[start.Row]
	// end.Col is inclusive, so we delete up to and including end.Col
	newLine := line[:start.Col] + line[end.Col+1:]
	m.content[start.Row] = newLine

	// Update yank register (vim behavior: deletes also yank)
	m.lastYankedText = c.deletedText
	m.lastYankWasLinewise = false

	// Position cursor at deletion point
	m.cursorRow = start.Row
	m.cursorCol = start.Col

	// Clamp cursor if it's past end of line
	if m.cursorCol > 0 && m.cursorCol >= len(m.content[m.cursorRow]) {
		m.cursorCol = len(m.content[m.cursorRow]) - 1
	}
	if m.cursorCol < 0 {
		m.cursorCol = 0
	}

	return Executed
}

// Undo restores the deleted text.
func (c *DeleteTextObjectCommand) Undo(m *Model) error {
	line := m.content[c.startPos.Row]

	// Restore deleted text at the original position
	m.content[c.startPos.Row] = line[:c.startPos.Col] + c.deletedText + line[c.startPos.Col:]

	// Restore cursor position
	m.cursorRow = c.row
	m.cursorCol = c.col

	return nil
}

// Keys returns the trigger keys for this command.
func (c *DeleteTextObjectCommand) Keys() []string {
	modifier := "i"
	if !c.inner {
		modifier = "a"
	}
	return []string{fmt.Sprintf("d%s%c", modifier, c.object)}
}

// Mode returns the mode this command operates in.
func (c *DeleteTextObjectCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *DeleteTextObjectCommand) ID() string {
	modifier := "inner"
	if !c.inner {
		modifier = "around"
	}
	return fmt.Sprintf("delete.textobject.%s_%c", modifier, c.object)
}

// ChangeTextObjectCommand deletes a text object and enters insert mode (ciw, caw, etc.).
// It uses the TextObjectFinder interface to locate text object bounds.
type ChangeTextObjectCommand struct {
	ChangeBase
	object      rune     // Text object type ('w', 'W', '"', etc.)
	inner       bool     // true for 'inner' (i), false for 'around' (a)
	row         int      // Row where deletion occurred (for undo)
	col         int      // Original cursor column (for undo)
	deletedText string   // Text that was deleted (for undo)
	startPos    Position // Start position of deleted region
	endPos      Position // End position of deleted region
}

// Execute deletes the text object and enters insert mode.
func (c *ChangeTextObjectCommand) Execute(m *Model) ExecuteResult {
	// Look up the text object finder
	finder, ok := textObjectRegistry[c.object]
	if !ok {
		return Skipped
	}

	// Find the text object bounds
	start, end, found := finder.FindBounds(m, c.inner)
	if !found {
		return Skipped
	}

	// Capture state for undo
	c.row = m.cursorRow
	c.col = m.cursorCol
	c.startPos = start
	c.endPos = end
	c.deletedText = extractText(m.content, start, end)

	// Delete the text object
	line := m.content[start.Row]
	// end.Col is inclusive, so we delete up to and including end.Col
	newLine := line[:start.Col] + line[end.Col+1:]
	m.content[start.Row] = newLine

	// Position cursor at deletion point
	m.cursorRow = start.Row
	m.cursorCol = start.Col

	// Enter insert mode
	m.mode = ModeInsert

	return Executed
}

// Undo restores the deleted text and returns to normal mode.
func (c *ChangeTextObjectCommand) Undo(m *Model) error {
	line := m.content[c.startPos.Row]

	// Restore deleted text at the original position
	m.content[c.startPos.Row] = line[:c.startPos.Col] + c.deletedText + line[c.startPos.Col:]

	// Restore cursor position and mode
	m.cursorRow = c.row
	m.cursorCol = c.col
	m.mode = ModeNormal

	return nil
}

// Keys returns the trigger keys for this command.
func (c *ChangeTextObjectCommand) Keys() []string {
	modifier := "i"
	if !c.inner {
		modifier = "a"
	}
	return []string{fmt.Sprintf("c%s%c", modifier, c.object)}
}

// Mode returns the mode this command operates in.
func (c *ChangeTextObjectCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *ChangeTextObjectCommand) ID() string {
	modifier := "inner"
	if !c.inner {
		modifier = "around"
	}
	return fmt.Sprintf("change.textobject.%s_%c", modifier, c.object)
}

// YankTextObjectCommand yanks a text object to the register without modifying content (yiw, yaw, etc.).
// It uses the TextObjectFinder interface to locate text object bounds.
type YankTextObjectCommand struct {
	MotionBase
	object rune // Text object type ('w', 'W', '"', etc.)
	inner  bool // true for 'inner' (i), false for 'around' (a)
	// Capture positions for highlight after execute
	highlightStart Position
	highlightEnd   Position
	showHighlight  bool
}

// Execute yanks the text object at the cursor position to the register.
func (c *YankTextObjectCommand) Execute(m *Model) ExecuteResult {
	// Look up the text object finder
	finder, ok := textObjectRegistry[c.object]
	if !ok {
		c.showHighlight = false
		return Skipped
	}

	// Find the text object bounds
	start, end, found := finder.FindBounds(m, c.inner)
	if !found {
		c.showHighlight = false
		return Skipped
	}

	// Capture positions for highlight
	c.highlightStart = start
	c.highlightEnd = end

	// Yank the text without modifying content
	m.lastYankedText = extractText(m.content, start, end)
	m.lastYankWasLinewise = false
	c.showHighlight = len(m.lastYankedText) > 0

	return Executed
}

// YankHighlightRegion returns the region to highlight after yank.
func (c *YankTextObjectCommand) YankHighlightRegion() (start, end Position, linewise bool, show bool) {
	return c.highlightStart, c.highlightEnd, false, c.showHighlight
}

// Keys returns the trigger keys for this command.
func (c *YankTextObjectCommand) Keys() []string {
	modifier := "i"
	if !c.inner {
		modifier = "a"
	}
	return []string{fmt.Sprintf("y%s%c", modifier, c.object)}
}

// Mode returns the mode this command operates in.
func (c *YankTextObjectCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *YankTextObjectCommand) ID() string {
	modifier := "inner"
	if !c.inner {
		modifier = "around"
	}
	return fmt.Sprintf("yank.textobject.%s_%c", modifier, c.object)
}

// VisualSelectTextObjectCommand enters visual mode with a text object selected (viw, vaw, etc.).
// It uses the TextObjectFinder interface to locate text object bounds.
type VisualSelectTextObjectCommand struct {
	ModeEntryBase
	object rune // Text object type ('w', 'W', '"', etc.)
	inner  bool // true for 'inner' (i), false for 'around' (a)
}

// Execute enters visual mode with the text object selected.
// Sets the visual anchor to the start of the text object and cursor to the end.
func (c *VisualSelectTextObjectCommand) Execute(m *Model) ExecuteResult {
	// Look up the text object finder
	finder, ok := textObjectRegistry[c.object]
	if !ok {
		return Skipped
	}

	// Find the text object bounds
	start, end, found := finder.FindBounds(m, c.inner)
	if !found {
		return Skipped
	}

	// Set visual anchor to start of text object
	m.visualAnchor = start

	// Set cursor to end of text object (inclusive)
	m.cursorRow = end.Row
	m.cursorCol = end.Col

	// Enter visual mode
	m.mode = ModeVisual
	m.pendingBuilder.Clear()

	return Executed
}

// Keys returns the trigger keys for this command.
func (c *VisualSelectTextObjectCommand) Keys() []string {
	modifier := "i"
	if !c.inner {
		modifier = "a"
	}
	return []string{fmt.Sprintf("v%s%c", modifier, c.object)}
}

// Mode returns the mode this command operates in.
func (c *VisualSelectTextObjectCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *VisualSelectTextObjectCommand) ID() string {
	modifier := "inner"
	if !c.inner {
		modifier = "around"
	}
	return fmt.Sprintf("visual.textobject.%s_%c", modifier, c.object)
}
