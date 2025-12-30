package vimtextarea

// ============================================================================
// Mode Entry Commands
// ============================================================================
//
// These commands handle entering insert mode from normal mode.

// EnterInsertModeCommand enters insert mode at the cursor position (i command).
// This is a non-undoable command - mode changes are not recorded in history.
type EnterInsertModeCommand struct {
	ModeEntryBase
	prevMode Mode
}

// Execute enters insert mode at the current cursor position.
func (c *EnterInsertModeCommand) Execute(m *Model) ExecuteResult {
	c.prevMode = m.mode
	m.mode = ModeInsert
	m.pendingBuilder.Clear()
	return Executed
}

// Keys returns the trigger keys for this command.
func (c *EnterInsertModeCommand) Keys() []string {
	return []string{"i"}
}

// Mode returns the mode this command operates in.
func (c *EnterInsertModeCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *EnterInsertModeCommand) ID() string {
	return "mode.insert"
}

// EnterInsertModeAfterCommand enters insert mode after the cursor (a command).
type EnterInsertModeAfterCommand struct {
	ModeEntryBase
	prevMode Mode
}

// Execute enters insert mode after the current cursor position.
func (c *EnterInsertModeAfterCommand) Execute(m *Model) ExecuteResult {
	c.prevMode = m.mode
	m.mode = ModeInsert
	m.pendingBuilder.Clear()

	// Move cursor right by one (if not at end of line)
	line := m.content[m.cursorRow]
	if m.cursorCol < len(line) {
		m.cursorCol++
	}
	return Executed
}

// Keys returns the trigger keys for this command.
func (c *EnterInsertModeAfterCommand) Keys() []string {
	return []string{"a"}
}

// Mode returns the mode this command operates in.
func (c *EnterInsertModeAfterCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *EnterInsertModeAfterCommand) ID() string {
	return "mode.insert_after"
}

// EnterInsertModeAtEndCommand enters insert mode at end of line (A command).
type EnterInsertModeAtEndCommand struct {
	ModeEntryBase
	prevMode Mode
}

// Execute enters insert mode at the end of the current line.
func (c *EnterInsertModeAtEndCommand) Execute(m *Model) ExecuteResult {
	c.prevMode = m.mode
	m.mode = ModeInsert
	m.pendingBuilder.Clear()

	// Move cursor to end of line
	m.cursorCol = len(m.content[m.cursorRow])
	return Executed
}

// Keys returns the trigger keys for this command.
func (c *EnterInsertModeAtEndCommand) Keys() []string {
	return []string{"A"}
}

// Mode returns the mode this command operates in.
func (c *EnterInsertModeAtEndCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *EnterInsertModeAtEndCommand) ID() string {
	return "mode.insert_at_end"
}

// EnterInsertModeAtStartCommand enters insert mode at first non-blank (I command).
type EnterInsertModeAtStartCommand struct {
	ModeEntryBase
	prevMode Mode
}

// Execute enters insert mode at the first non-blank character.
func (c *EnterInsertModeAtStartCommand) Execute(m *Model) ExecuteResult {
	c.prevMode = m.mode
	m.mode = ModeInsert
	m.pendingBuilder.Clear()

	// Find first non-blank character
	line := m.content[m.cursorRow]
	m.cursorCol = 0
	for i, ch := range line {
		if ch != ' ' && ch != '\t' {
			m.cursorCol = i
			break
		}
	}
	return Executed
}

// Keys returns the trigger keys for this command.
func (c *EnterInsertModeAtStartCommand) Keys() []string {
	return []string{"I"}
}

// Mode returns the mode this command operates in.
func (c *EnterInsertModeAtStartCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *EnterInsertModeAtStartCommand) ID() string {
	return "mode.insert_at_start"
}

// InsertLineBelowCommand inserts a new line below and enters insert mode (o command).
// This combines content mutation (undoable) with mode change.
type InsertLineBelowCommand struct {
	ChangeBase
	insertedRow int // Row where the new line was inserted
}

// Execute inserts a new line below the current line and enters insert mode.
func (c *InsertLineBelowCommand) Execute(m *Model) ExecuteResult {
	// Insert new empty line below current line
	c.insertedRow = m.cursorRow + 1
	newContent := make([]string, len(m.content)+1)
	copy(newContent[:m.cursorRow+1], m.content[:m.cursorRow+1])
	newContent[c.insertedRow] = ""
	copy(newContent[c.insertedRow+1:], m.content[m.cursorRow+1:])
	m.content = newContent

	// Move cursor to new line
	m.cursorRow = c.insertedRow
	m.cursorCol = 0

	// Enter insert mode
	m.mode = ModeInsert
	m.pendingBuilder.Clear()
	return Executed
}

// Undo removes the inserted line (but mode change is not undone).
func (c *InsertLineBelowCommand) Undo(m *Model) error {
	if c.insertedRow >= len(m.content) {
		return nil
	}

	// Remove the inserted line
	newContent := make([]string, len(m.content)-1)
	copy(newContent[:c.insertedRow], m.content[:c.insertedRow])
	copy(newContent[c.insertedRow:], m.content[c.insertedRow+1:])
	m.content = newContent

	// Restore cursor position
	m.cursorRow = max(c.insertedRow-1, 0)
	m.clampCursorCol()
	return nil
}

// Keys returns the trigger keys for this command.
func (c *InsertLineBelowCommand) Keys() []string {
	return []string{"o"}
}

// Mode returns the mode this command operates in.
func (c *InsertLineBelowCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *InsertLineBelowCommand) ID() string {
	return "mode.insert_line_below"
}

// InsertLineAboveCommand inserts a new line above and enters insert mode (O command).
// This combines content mutation (undoable) with mode change.
type InsertLineAboveCommand struct {
	ChangeBase
	insertedRow int // Row where the new line was inserted
}

// Execute inserts a new line above the current line and enters insert mode.
func (c *InsertLineAboveCommand) Execute(m *Model) ExecuteResult {
	// Insert new empty line above current line
	c.insertedRow = m.cursorRow
	newContent := make([]string, len(m.content)+1)
	copy(newContent[:c.insertedRow], m.content[:c.insertedRow])
	newContent[c.insertedRow] = ""
	copy(newContent[c.insertedRow+1:], m.content[c.insertedRow:])
	m.content = newContent

	// Cursor stays on current row (which is now the new empty line)
	m.cursorCol = 0

	// Enter insert mode
	m.mode = ModeInsert
	m.pendingBuilder.Clear()
	return Executed
}

// Undo removes the inserted line (but mode change is not undone).
func (c *InsertLineAboveCommand) Undo(m *Model) error {
	if c.insertedRow >= len(m.content) {
		return nil
	}

	// Remove the inserted line
	newContent := make([]string, len(m.content)-1)
	copy(newContent[:c.insertedRow], m.content[:c.insertedRow])
	copy(newContent[c.insertedRow:], m.content[c.insertedRow+1:])
	m.content = newContent

	// Restore cursor position
	m.cursorRow = c.insertedRow
	if m.cursorRow >= len(m.content) {
		m.cursorRow = len(m.content) - 1
	}
	m.clampCursorCol()
	return nil
}

// Keys returns the trigger keys for this command.
func (c *InsertLineAboveCommand) Keys() []string {
	return []string{"O"}
}

// Mode returns the mode this command operates in.
func (c *InsertLineAboveCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *InsertLineAboveCommand) ID() string {
	return "mode.insert_line_above"
}

// ============================================================================
// Escape Commands
// ============================================================================

// EscapeCommand exits insert mode and returns to normal mode.
// In normal mode, it clears pending commands and passes through to parent.
type EscapeCommand struct {
	ModeEntryBase
	wasInsertMode bool
	prevCol       int
}

// Execute handles the escape key.
func (c *EscapeCommand) Execute(m *Model) ExecuteResult {
	// If vim is disabled, ESC should pass through (no mode switching)
	if !m.config.VimEnabled {
		return PassThrough
	}

	c.wasInsertMode = m.mode == ModeInsert

	if m.mode == ModeInsert {
		c.prevCol = m.cursorCol
		m.mode = ModeNormal
		// In vim, when exiting insert mode, cursor moves back one position
		// if it's not at the start of the line
		if m.cursorCol > 0 {
			m.cursorCol--
		}
	}
	// Always clear pending builder
	m.pendingBuilder.Clear()
	return Executed
}

// Keys returns the trigger keys for this command.
func (c *EscapeCommand) Keys() []string {
	return []string{"<escape>", "<ctrl+c>"}
}

// Mode returns the mode this command operates in.
func (c *EscapeCommand) Mode() Mode {
	return ModeInsert // Primary mode is insert (normal mode escape passes through)
}

// ID returns the hierarchical identifier for this command.
func (c *EscapeCommand) ID() string {
	return "mode.escape"
}

// NormalModeEscapeCommand handles ESC in Normal mode.
// It clears pending commands and passes through to parent for quit handling.
type NormalModeEscapeCommand struct {
	MotionBase
}

// Execute clears pending commands and returns PassThrough.
func (c *NormalModeEscapeCommand) Execute(m *Model) ExecuteResult {
	m.pendingBuilder.Clear()
	return PassThrough
}

// Keys returns the trigger keys for this command.
func (c *NormalModeEscapeCommand) Keys() []string {
	return []string{"<escape>"}
}

// Mode returns the mode this command operates in.
func (c *NormalModeEscapeCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *NormalModeEscapeCommand) ID() string {
	return "mode.escape_normal"
}

// ============================================================================
// Visual Mode Entry Commands
// ============================================================================

// EnterVisualModeCommand enters character-wise visual mode (v command).
// This is a non-undoable command - mode changes are not recorded in history.
// Sets the visual anchor to the current cursor position.
type EnterVisualModeCommand struct {
	ModeEntryBase
}

// Execute enters character-wise visual mode at the current cursor position.
func (c *EnterVisualModeCommand) Execute(m *Model) ExecuteResult {
	m.visualAnchor = Position{Row: m.cursorRow, Col: m.cursorCol}
	m.mode = ModeVisual
	m.pendingBuilder.Clear()
	return Executed
}

// Keys returns the trigger keys for this command.
func (c *EnterVisualModeCommand) Keys() []string {
	return []string{"v"}
}

// Mode returns the mode this command operates in.
func (c *EnterVisualModeCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *EnterVisualModeCommand) ID() string {
	return "mode.visual"
}

// EnterVisualLineModeCommand enters line-wise visual mode (V command).
// This is a non-undoable command - mode changes are not recorded in history.
// Sets the visual anchor to the start of the current line (Col = 0).
type EnterVisualLineModeCommand struct {
	ModeEntryBase
}

// Execute enters line-wise visual mode with anchor at start of line.
func (c *EnterVisualLineModeCommand) Execute(m *Model) ExecuteResult {
	m.visualAnchor = Position{Row: m.cursorRow, Col: 0}
	m.mode = ModeVisualLine
	m.pendingBuilder.Clear()
	return Executed
}

// Keys returns the trigger keys for this command.
func (c *EnterVisualLineModeCommand) Keys() []string {
	return []string{"V"}
}

// Mode returns the mode this command operates in.
func (c *EnterVisualLineModeCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *EnterVisualLineModeCommand) ID() string {
	return "mode.visual_line"
}

// ============================================================================
// Visual Mode Exit Commands
// ============================================================================

// VisualModeEscapeCommand exits visual mode and returns to normal mode.
// It clears the visual anchor and pending commands.
type VisualModeEscapeCommand struct {
	ModeEntryBase
	mode Mode // Which visual mode to register for (set during registration)
}

// Execute exits visual mode, clears anchor, and returns to normal mode.
func (c *VisualModeEscapeCommand) Execute(m *Model) ExecuteResult {
	m.mode = ModeNormal
	m.visualAnchor = Position{}
	m.pendingBuilder.Clear()
	return Executed
}

// Keys returns the trigger keys for this command.
func (c *VisualModeEscapeCommand) Keys() []string {
	return []string{"<escape>", "<ctrl+c>"}
}

// Mode returns the mode this command operates in.
func (c *VisualModeEscapeCommand) Mode() Mode {
	return c.mode
}

// ID returns the hierarchical identifier for this command.
func (c *VisualModeEscapeCommand) ID() string {
	return "mode.visual_escape"
}

// ============================================================================
// Visual Mode Toggle Commands
// ============================================================================

// VisualModeToggleVCommand handles 'v' key pressed while already in ModeVisual.
// Pressing 'v' while in visual mode toggles it off (returns to Normal mode).
type VisualModeToggleVCommand struct {
	ModeEntryBase
}

// Execute exits visual mode and returns to normal mode.
func (c *VisualModeToggleVCommand) Execute(m *Model) ExecuteResult {
	m.mode = ModeNormal
	m.visualAnchor = Position{}
	m.pendingBuilder.Clear()
	return Executed
}

// Keys returns the trigger keys for this command.
func (c *VisualModeToggleVCommand) Keys() []string {
	return []string{"v"}
}

// Mode returns the mode this command operates in.
func (c *VisualModeToggleVCommand) Mode() Mode {
	return ModeVisual
}

// ID returns the hierarchical identifier for this command.
func (c *VisualModeToggleVCommand) ID() string {
	return "mode.visual_toggle_v"
}

// VisualModeToggleShiftVCommand handles 'V' key pressed while in ModeVisual.
// Pressing 'V' while in character-wise visual mode switches to line-wise visual mode.
type VisualModeToggleShiftVCommand struct {
	ModeEntryBase
}

// Execute switches from character-wise to line-wise visual mode.
// Preserves the anchor row but sets Col to 0 for line-wise selection.
func (c *VisualModeToggleShiftVCommand) Execute(m *Model) ExecuteResult {
	m.visualAnchor.Col = 0
	m.mode = ModeVisualLine
	m.pendingBuilder.Clear()
	return Executed
}

// Keys returns the trigger keys for this command.
func (c *VisualModeToggleShiftVCommand) Keys() []string {
	return []string{"V"}
}

// Mode returns the mode this command operates in.
func (c *VisualModeToggleShiftVCommand) Mode() Mode {
	return ModeVisual
}

// ID returns the hierarchical identifier for this command.
func (c *VisualModeToggleShiftVCommand) ID() string {
	return "mode.visual_toggle_shift_v"
}

// VisualLineModeToggleVCommand handles 'v' key pressed while in ModeVisualLine.
// Pressing 'v' while in line-wise visual mode switches to character-wise visual mode.
type VisualLineModeToggleVCommand struct {
	ModeEntryBase
}

// Execute switches from line-wise to character-wise visual mode.
func (c *VisualLineModeToggleVCommand) Execute(m *Model) ExecuteResult {
	m.mode = ModeVisual
	m.pendingBuilder.Clear()
	return Executed
}

// Keys returns the trigger keys for this command.
func (c *VisualLineModeToggleVCommand) Keys() []string {
	return []string{"v"}
}

// Mode returns the mode this command operates in.
func (c *VisualLineModeToggleVCommand) Mode() Mode {
	return ModeVisualLine
}

// ID returns the hierarchical identifier for this command.
func (c *VisualLineModeToggleVCommand) ID() string {
	return "mode.visual_line_toggle_v"
}

// VisualLineModeToggleShiftVCommand handles 'V' key pressed while in ModeVisualLine.
// Pressing 'V' while in line-wise visual mode toggles it off (returns to Normal mode).
type VisualLineModeToggleShiftVCommand struct {
	ModeEntryBase
}

// Execute exits visual line mode and returns to normal mode.
func (c *VisualLineModeToggleShiftVCommand) Execute(m *Model) ExecuteResult {
	m.mode = ModeNormal
	m.visualAnchor = Position{}
	m.pendingBuilder.Clear()
	return Executed
}

// Keys returns the trigger keys for this command.
func (c *VisualLineModeToggleShiftVCommand) Keys() []string {
	return []string{"V"}
}

// Mode returns the mode this command operates in.
func (c *VisualLineModeToggleShiftVCommand) Mode() Mode {
	return ModeVisualLine
}

// ID returns the hierarchical identifier for this command.
func (c *VisualLineModeToggleShiftVCommand) ID() string {
	return "mode.visual_line_toggle_shift_v"
}
