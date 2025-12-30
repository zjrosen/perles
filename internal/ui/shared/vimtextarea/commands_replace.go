package vimtextarea

// ============================================================================
// Replace Commands
// ============================================================================

// ReplaceCharCommand replaces a single character under the cursor (r command).
// It captures the position and old character for undo.
// Note: This command is executed after the replacement character is captured
// via the pending command system.
type ReplaceCharCommand struct {
	DeleteBase
	row     int  // Row where replacement occurred
	col     int  // Column where replacement occurred
	oldChar rune // Character that was replaced (for undo)
	newChar rune // The replacement character
}

// Execute replaces the character at the current cursor position.
func (c *ReplaceCharCommand) Execute(m *Model) ExecuteResult {
	// Capture position from model
	c.row = m.cursorRow
	line := m.content[c.row]

	// If line is empty, nothing to replace
	if len(line) == 0 {
		return Skipped
	}

	// Clamp column to valid range
	c.col = min(m.cursorCol, len(line)-1)

	// Capture old character for undo
	c.oldChar = rune(line[c.col])

	// Replace character at cursor position
	m.content[c.row] = line[:c.col] + string(c.newChar) + line[c.col+1:]

	// Cursor stays in place (no movement)

	return Executed
}

// Undo restores the original character.
func (c *ReplaceCharCommand) Undo(m *Model) error {
	line := m.content[c.row]

	// Restore the original character
	m.content[c.row] = line[:c.col] + string(c.oldChar) + line[c.col+1:]

	// Restore cursor position
	m.cursorRow = c.row
	m.cursorCol = c.col

	return nil
}

// Keys returns the trigger keys for this command.
// Note: The 'r' key is handled specially by the pending command system.
func (c *ReplaceCharCommand) Keys() []string {
	return []string{"r"}
}

// Mode returns the mode this command operates in.
func (c *ReplaceCharCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *ReplaceCharCommand) ID() string {
	return "replace.char"
}

// ============================================================================
// Replace Mode Commands (R command)
// ============================================================================

// EnterReplaceModeCommand enters Replace mode from Normal mode (R command).
// In Replace mode, typed characters overwrite existing text rather than inserting.
type EnterReplaceModeCommand struct {
	ModeEntryBase
}

// Execute enters Replace mode.
func (c *EnterReplaceModeCommand) Execute(m *Model) ExecuteResult {
	m.mode = ModeReplace
	m.pendingBuilder.Clear()
	return Executed
}

// Keys returns the trigger keys for this command.
func (c *EnterReplaceModeCommand) Keys() []string {
	return []string{"R"}
}

// Mode returns the mode this command operates in.
func (c *EnterReplaceModeCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *EnterReplaceModeCommand) ID() string {
	return "mode.replace"
}

// ReplaceModeCharCommand handles character input in Replace mode.
// At end of line: inserts character (append)
// Not at end: replaces character at cursor, moves cursor right
type ReplaceModeCharCommand struct {
	InsertBase
	row      int  // Row where replacement occurred
	col      int  // Column where replacement occurred
	oldChar  rune // Character that was replaced (for undo); 0 if inserted
	newChar  rune // The typed character
	wasAtEOL bool // True if character was inserted at end of line
}

// Execute handles a character typed in Replace mode.
func (c *ReplaceModeCharCommand) Execute(m *Model) ExecuteResult {
	// Capture position from model
	c.row = m.cursorRow
	c.col = m.cursorCol
	line := m.content[c.row]

	// Check if at end of line
	if c.col >= len(line) {
		// At end of line: INSERT character (append mode)
		c.wasAtEOL = true
		c.oldChar = 0 // No character was replaced
		m.content[c.row] = line + string(c.newChar)
		m.cursorCol = len(m.content[c.row])
	} else {
		// Not at end: REPLACE character at cursor
		c.wasAtEOL = false
		c.oldChar = rune(line[c.col])
		m.content[c.row] = line[:c.col] + string(c.newChar) + line[c.col+1:]
		m.cursorCol++
	}

	return Executed
}

// Undo reverses the replace or insert operation.
func (c *ReplaceModeCharCommand) Undo(m *Model) error {
	line := m.content[c.row]

	if c.wasAtEOL {
		// Was an insert - remove the character
		if len(line) > 0 && c.col < len(line) {
			m.content[c.row] = line[:c.col]
		}
	} else {
		// Was a replace - restore the original character
		m.content[c.row] = line[:c.col] + string(c.oldChar) + line[c.col+1:]
	}

	// Restore cursor position
	m.cursorRow = c.row
	m.cursorCol = c.col

	return nil
}

// Keys returns the trigger keys for this command.
// Note: This command is created dynamically for character input in Replace mode.
func (c *ReplaceModeCharCommand) Keys() []string {
	return nil // No static key binding - handled by handleKeyMsg
}

// Mode returns the mode this command operates in.
func (c *ReplaceModeCharCommand) Mode() Mode {
	return ModeReplace
}

// ID returns the hierarchical identifier for this command.
func (c *ReplaceModeCharCommand) ID() string {
	return "replace.mode_char"
}

// ReplaceModeEscapeCommand exits Replace mode and returns to Normal mode.
// Moves cursor back one position (vim behavior).
type ReplaceModeEscapeCommand struct {
	ModeEntryBase
}

// Execute exits Replace mode and returns to Normal mode.
func (c *ReplaceModeEscapeCommand) Execute(m *Model) ExecuteResult {
	m.mode = ModeNormal
	m.pendingBuilder.Clear()

	// In vim, when exiting Replace mode, cursor moves back one position
	// if it's not at the start of the line
	if m.cursorCol > 0 {
		m.cursorCol--
	}

	return Executed
}

// Keys returns the trigger keys for this command.
func (c *ReplaceModeEscapeCommand) Keys() []string {
	return []string{"<escape>"}
}

// Mode returns the mode this command operates in.
func (c *ReplaceModeEscapeCommand) Mode() Mode {
	return ModeReplace
}

// ID returns the hierarchical identifier for this command.
func (c *ReplaceModeEscapeCommand) ID() string {
	return "mode.replace_escape"
}

// ReplaceModeBackspaceCommand handles backspace in Replace mode.
// Uses simplified delete behavior (deletes previous character) rather than
// vim's full restoration semantics.
type ReplaceModeBackspaceCommand struct {
	InsertBase
	row         int
	col         int
	deletedChar rune
	hadContent  bool // True if there was a character to delete
}

// Execute handles backspace in Replace mode.
func (c *ReplaceModeBackspaceCommand) Execute(m *Model) ExecuteResult {
	c.row = m.cursorRow
	c.col = m.cursorCol

	// If at start of line, nothing to delete (simplified behavior)
	if c.col == 0 {
		c.hadContent = false
		return Skipped
	}

	line := m.content[c.row]
	c.hadContent = true
	c.deletedChar = rune(line[c.col-1])

	// Delete character before cursor
	m.content[c.row] = line[:c.col-1] + line[c.col:]
	m.cursorCol--

	return Executed
}

// Undo restores the deleted character.
func (c *ReplaceModeBackspaceCommand) Undo(m *Model) error {
	if !c.hadContent {
		return nil
	}

	line := m.content[c.row]
	// Restore the deleted character
	m.content[c.row] = line[:c.col-1] + string(c.deletedChar) + line[c.col-1:]

	// Restore cursor position
	m.cursorRow = c.row
	m.cursorCol = c.col

	return nil
}

// Keys returns the trigger keys for this command.
func (c *ReplaceModeBackspaceCommand) Keys() []string {
	return []string{"<backspace>"}
}

// Mode returns the mode this command operates in.
func (c *ReplaceModeBackspaceCommand) Mode() Mode {
	return ModeReplace
}

// ID returns the hierarchical identifier for this command.
func (c *ReplaceModeBackspaceCommand) ID() string {
	return "replace.mode_backspace"
}

// ReplaceModeSpaceCommand handles the space key in Replace mode.
// This wraps ReplaceModeCharCommand because Bubble Tea sends space as KeySpace.
type ReplaceModeSpaceCommand struct {
	InsertBase
	charCmd *ReplaceModeCharCommand
}

// Execute overwrites or appends a space character.
func (c *ReplaceModeSpaceCommand) Execute(m *Model) ExecuteResult {
	c.charCmd = &ReplaceModeCharCommand{
		newChar: ' ',
	}
	return c.charCmd.Execute(m)
}

// Undo reverses the space operation.
func (c *ReplaceModeSpaceCommand) Undo(m *Model) error {
	if c.charCmd != nil {
		return c.charCmd.Undo(m)
	}
	return nil
}

// Keys returns the trigger keys for this command.
func (c *ReplaceModeSpaceCommand) Keys() []string {
	return []string{"<space>"}
}

// Mode returns the mode this command operates in.
func (c *ReplaceModeSpaceCommand) Mode() Mode {
	return ModeReplace
}

// ID returns the hierarchical identifier for this command.
func (c *ReplaceModeSpaceCommand) ID() string {
	return "replace.mode_space"
}
