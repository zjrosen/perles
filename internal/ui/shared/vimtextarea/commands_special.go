package vimtextarea

// ============================================================================
// PendingCommandBuilder
// ============================================================================

// PendingCommandBuilder replaces the string-based pendingCommand with a structured builder.
// It handles multi-key sequences like 'gg', 'dd', 'dw', 'd$', 'dj', 'dk', 'dgg', 'cgg', etc.
type PendingCommandBuilder struct {
	operator  rune   // 'd', 'y', 'c', 'g', or 0 for none
	count     int    // repeat count (default 1, for future use)
	motion    rune   // motion key or 0 (legacy, kept for compatibility)
	keyBuffer string // buffered keys after operator (for multi-key sequences like "gg")
}

// NewPendingCommandBuilder creates an empty pending command builder.
func NewPendingCommandBuilder() *PendingCommandBuilder {
	return &PendingCommandBuilder{
		count: 1, // default count
	}
}

// Clear resets the builder to empty state.
func (b *PendingCommandBuilder) Clear() {
	b.operator = 0
	b.count = 1
	b.motion = 0
	b.keyBuffer = ""
}

// IsEmpty returns true if no pending command is being built.
func (b *PendingCommandBuilder) IsEmpty() bool {
	return b.operator == 0
}

// SetOperator sets the pending operator (first key of multi-key sequence).
func (b *PendingCommandBuilder) SetOperator(op rune) {
	b.operator = op
	b.motion = 0
	b.keyBuffer = ""
}

// Operator returns the current pending operator.
func (b *PendingCommandBuilder) Operator() rune {
	return b.operator
}

// AppendKey adds a key to the buffer for multi-key sequences.
func (b *PendingCommandBuilder) AppendKey(key string) {
	b.keyBuffer += key
}

// KeyBuffer returns the current buffered keys.
func (b *PendingCommandBuilder) KeyBuffer() string {
	return b.keyBuffer
}

// ============================================================================
// Undo/Redo Commands
// ============================================================================

// UndoCommand undoes the last content-mutating command.
type UndoCommand struct{}

// Execute undoes the last command in history.
func (c *UndoCommand) Execute(m *Model) ExecuteResult {
	if m.history.CanUndo() {
		_ = m.history.Undo(m)
	}
	return Executed
}

// Undo returns nil - undo itself is not undoable (use redo).
func (c *UndoCommand) Undo(m *Model) error {
	return nil
}

// Keys returns the trigger keys for this command.
func (c *UndoCommand) Keys() []string {
	return []string{"u"}
}

// Mode returns the mode this command operates in.
func (c *UndoCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *UndoCommand) ID() string {
	return "history.undo"
}

// IsUndoable returns false - undo is not added to history (it manipulates history).
func (c *UndoCommand) IsUndoable() bool { return false }

// ChangesContent returns true - undo restores previous content state.
func (c *UndoCommand) ChangesContent() bool { return true }

// IsModeChange returns false - undo doesn't change mode.
func (c *UndoCommand) IsModeChange() bool { return false }

// RedoCommand redoes the last undone command.
type RedoCommand struct{}

// Execute redoes the next command in history.
func (c *RedoCommand) Execute(m *Model) ExecuteResult {
	if m.history.CanRedo() {
		_ = m.history.Redo(m)
	}
	return Executed
}

// Undo returns nil - redo itself is not undoable (use undo).
func (c *RedoCommand) Undo(m *Model) error {
	return nil
}

// Keys returns the trigger keys for this command.
func (c *RedoCommand) Keys() []string {
	return []string{"<ctrl+r>"}
}

// Mode returns the mode this command operates in.
func (c *RedoCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *RedoCommand) ID() string {
	return "history.redo"
}

// IsUndoable returns false - redo is not added to history (it manipulates history).
func (c *RedoCommand) IsUndoable() bool { return false }

// ChangesContent returns true - redo reapplies a command that changed content.
func (c *RedoCommand) ChangesContent() bool { return true }

// IsModeChange returns false - redo doesn't change mode.
func (c *RedoCommand) IsModeChange() bool { return false }

// ConditionalRedoCommand only executes redo when redo history is available.
// If no redo is available, it returns PassThrough so parent can handle Ctrl+R.
type ConditionalRedoCommand struct{}

// Execute performs redo if available, otherwise passes through.
func (c *ConditionalRedoCommand) Execute(m *Model) ExecuteResult {
	if !m.history.CanRedo() {
		return PassThrough
	}
	_ = m.history.Redo(m)
	return Executed
}

// Undo returns nil - redo itself is not undoable (use undo).
func (c *ConditionalRedoCommand) Undo(m *Model) error {
	return nil
}

// Keys returns the trigger keys for this command.
func (c *ConditionalRedoCommand) Keys() []string {
	return []string{"<ctrl+r>"}
}

// Mode returns the mode this command operates in.
func (c *ConditionalRedoCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *ConditionalRedoCommand) ID() string {
	return "history.redo_conditional"
}

// IsUndoable returns false - redo is not added to history.
func (c *ConditionalRedoCommand) IsUndoable() bool { return false }

// ChangesContent returns true - redo reapplies a command that changed content.
func (c *ConditionalRedoCommand) ChangesContent() bool { return true }

// IsModeChange returns false - redo doesn't change mode.
func (c *ConditionalRedoCommand) IsModeChange() bool { return false }

// ============================================================================
// Pending Command
// ============================================================================

// StartPendingCommand sets a pending operator for multi-key sequences (g, d prefixes).
// This is not a content-mutating command - just sets state for the next key.
type StartPendingCommand struct {
	MotionBase
	operator rune
}

// Execute sets the pending operator.
func (c *StartPendingCommand) Execute(m *Model) ExecuteResult {
	m.pendingBuilder.SetOperator(c.operator)
	return Executed
}

// Keys returns the trigger keys for this command.
func (c *StartPendingCommand) Keys() []string {
	return []string{string(c.operator)}
}

// Mode returns the mode this command operates in.
func (c *StartPendingCommand) Mode() Mode {
	return ModeNormal
}

// ID returns the hierarchical identifier for this command.
func (c *StartPendingCommand) ID() string {
	return "pending." + string(c.operator)
}

// ============================================================================
// Submit Command (Alt+Enter, Ctrl+J)
// ============================================================================

// SubmitCommand triggers content submission.
// This is a special command that produces a SubmitMsg or calls OnSubmit callback.
// It doesn't modify content - just signals that the user wants to submit.
type SubmitCommand struct {
	MotionBase
}

// Execute produces a submit message (handled via tea.Cmd, not directly here).
// The actual submission is done via the tea.Cmd returned by executeAndRespond.
func (c *SubmitCommand) Execute(m *Model) ExecuteResult {
	// Submit always executes - the actual message is produced via tea.Cmd
	return Executed
}

// Keys returns the trigger keys for this command.
func (c *SubmitCommand) Keys() []string {
	return []string{"<enter>", "<ctrl+j>"}
}

// Mode returns the mode this command operates in.
// Submit works in all modes.
func (c *SubmitCommand) Mode() Mode {
	return ModeInsert // Registered for Insert, but works in all modes
}

// ID returns the hierarchical identifier for this command.
func (c *SubmitCommand) ID() string {
	return "submit"
}

// IsSubmit returns true - this is a submit command.
func (c *SubmitCommand) IsSubmit() bool { return true }
