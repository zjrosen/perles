package vimtextarea

// ExecuteResult indicates the outcome of command execution.
type ExecuteResult int

const (
	// Executed means the command was executed successfully and consumed the key.
	Executed ExecuteResult = iota
	// PassThrough means the command chose not to handle this key (let parent handle it).
	PassThrough
	// Skipped means pre-conditions weren't met (e.g., backspace at position 0,0).
	Skipped
)

// Command represents an executable, reversible text operation.
// Commands are used to implement the undo/redo system - each content-mutating
// operation (delete, insert, etc.) is represented as a Command that can be
// executed and undone.
//
// Note: Motion commands (hjkl, w, b, etc.) implement this interface but their
// Undo() is a no-op because cursor movements are not undoable in vim semantics.
type Command interface {
	// Execute applies the command to the model.
	// Returns ExecuteResult indicating whether the command was executed, skipped, or should pass through.
	Execute(m *Model) ExecuteResult

	// Undo reverses the command's effect.
	// Returns error if the command cannot be undone.
	Undo(m *Model) error

	// Keys returns the trigger key(s) that invoke this command.
	// Most commands have a single key, but some have aliases (e.g., arrow keys + hjkl).
	// For single-key commands: []string{"h"}, []string{"j"}
	// For multi-key sequences: []string{"dd"}, []string{"dw"}
	// For special keys: []string{"<backspace>"}, []string{"<enter>"}
	// For aliases: []string{"<left>", "<ctrl+b>"} (both trigger same command)
	Keys() []string

	// Mode returns which vim mode this command operates in.
	Mode() Mode

	// ID returns a hierarchical identifier for this command type.
	// Used for debugging, logging, and registry organization.
	// Examples: "delete.char", "move.down", "insert.text"
	ID() string

	// IsUndoable returns true if this command should be added to undo history.
	// Content-mutating commands return true; motion/mode commands return false.
	IsUndoable() bool

	// ChangesContent returns true if this command modifies the text content.
	// Used to determine whether to trigger onChange callbacks.
	// Note: Undo/Redo return true (they change content) but IsUndoable returns false.
	ChangesContent() bool

	// IsModeChange returns true if this command changes the vim mode.
	// Used to determine whether to emit ModeChangeMsg.
	IsModeChange() bool
}

// ============================================================================
// Base structs for reducing boilerplate in Command implementations
// ============================================================================

// MotionBase provides default implementations for motion commands.
// Motion commands are not undoable, don't change content, and don't change mode.
type MotionBase struct{}

func (MotionBase) Undo(*Model) error    { return nil }
func (MotionBase) IsUndoable() bool     { return false }
func (MotionBase) ChangesContent() bool { return false }
func (MotionBase) IsModeChange() bool   { return false }

// DeleteBase provides default implementations for delete commands.
// Delete commands are undoable, change content, but don't change mode.
type DeleteBase struct{}

func (DeleteBase) IsUndoable() bool     { return true }
func (DeleteBase) ChangesContent() bool { return true }
func (DeleteBase) IsModeChange() bool   { return false }

// ChangeBase provides default implementations for change commands.
// Change commands are undoable, change content, and change mode (to insert).
type ChangeBase struct{}

func (ChangeBase) IsUndoable() bool     { return true }
func (ChangeBase) ChangesContent() bool { return true }
func (ChangeBase) IsModeChange() bool   { return true }

// ModeEntryBase provides default implementations for mode entry commands.
// Mode entry commands are not undoable, don't change content, but do change mode.
type ModeEntryBase struct{}

func (ModeEntryBase) Undo(*Model) error    { return nil }
func (ModeEntryBase) IsUndoable() bool     { return false }
func (ModeEntryBase) ChangesContent() bool { return false }
func (ModeEntryBase) IsModeChange() bool   { return true }

// InsertBase provides default implementations for insert-mode editing commands.
// These are undoable, change content, but don't change mode (they execute within insert mode).
type InsertBase struct{}

func (InsertBase) IsUndoable() bool     { return true }
func (InsertBase) ChangesContent() bool { return true }
func (InsertBase) IsModeChange() bool   { return false }

// ============================================================================
// CommandHistory
// ============================================================================

// CommandHistory manages the command stack for undo/redo operations.
// It maintains a list of executed commands and an index pointing to the
// current position in the history. This enables navigating backward (undo)
// and forward (redo) through command history.
//
// The undoIndex works as follows:
//   - -1 means we're at the base state (nothing to undo)
//   - 0 to len(commands)-1 points to the last executed command
//   - When undoing, we call Undo on commands[undoIndex] and decrement
//   - When redoing, we increment and call Execute on commands[undoIndex]
//
// When a new command is pushed after undoing, all "future" commands
// (those after undoIndex) are discarded, as the new command creates
// a new branch in history.
type CommandHistory struct {
	commands  []Command // All executed commands
	undoIndex int       // Current position (-1 = at base state, len-1 = at latest)
}

// NewCommandHistory creates an empty command history.
func NewCommandHistory() *CommandHistory {
	return &CommandHistory{
		commands:  make([]Command, 0),
		undoIndex: -1,
	}
}

// Push adds a new command after execution.
// This should be called AFTER the command has been executed successfully.
// Clears any "future" commands (invalidates redo stack) since a new
// command creates a new branch in history.
func (h *CommandHistory) Push(cmd Command) {
	// Truncate redo history - new command invalidates future
	// Keep only commands up to and including undoIndex
	h.commands = h.commands[:h.undoIndex+1]
	h.commands = append(h.commands, cmd)
	h.undoIndex = len(h.commands) - 1
}

// Undo reverses the last command and moves back in history.
// Returns nil if there's nothing to undo (at base state).
// The command's Undo method is called to reverse its effect.
func (h *CommandHistory) Undo(m *Model) error {
	if h.undoIndex < 0 {
		return nil // Nothing to undo
	}
	err := h.commands[h.undoIndex].Undo(m)
	h.undoIndex--
	return err
}

// Redo re-executes the next command and moves forward in history.
// Returns nil if there's nothing to redo (at latest command).
// The command's Execute method is called to re-apply its effect.
func (h *CommandHistory) Redo(m *Model) error {
	if h.undoIndex >= len(h.commands)-1 {
		return nil // Nothing to redo
	}
	h.undoIndex++
	_ = h.commands[h.undoIndex].Execute(m)
	return nil
}

// CanUndo returns true if there are commands to undo.
func (h *CommandHistory) CanUndo() bool {
	return h.undoIndex >= 0
}

// CanRedo returns true if there are commands to redo.
func (h *CommandHistory) CanRedo() bool {
	return h.undoIndex < len(h.commands)-1
}

// Clear resets the command history to empty state.
// This is typically called when the content is reset or cleared.
func (h *CommandHistory) Clear() {
	h.commands = h.commands[:0]
	h.undoIndex = -1
}

// ============================================================================
// CommandRegistry
// ============================================================================

// CommandRegistry provides mode-aware, key-based command dispatch.
// Commands are registered with their Mode() and Key() used for lookup.
// Create() returns a fresh instance via reflection to ensure clean state for undo.
type CommandRegistry struct {
	// commands maps Mode -> trigger key -> prototype command
	commands map[Mode]map[string]Command
}

// NewCommandRegistry creates an empty command registry.
func NewCommandRegistry() *CommandRegistry {
	return &CommandRegistry{
		commands: make(map[Mode]map[string]Command),
	}
}

// Register adds a command to the registry using its Mode() and Keys() methods.
// Commands with multiple keys are registered under each key.
func (r *CommandRegistry) Register(cmd Command) {
	mode := cmd.Mode()
	keys := cmd.Keys()

	if r.commands[mode] == nil {
		r.commands[mode] = make(map[string]Command)
	}
	for _, key := range keys {
		r.commands[mode][key] = cmd
	}
}

// Get retrieves a command for a specific mode and key.
func (r *CommandRegistry) Get(mode Mode, key string) (Command, bool) {
	if modeMap, ok := r.commands[mode]; ok {
		if cmd, ok := modeMap[key]; ok {
			return cmd, true
		}
	}
	return nil, false
}

// registerWithModeKeys adds a command with explicit mode, registering all its Keys().
func (r *CommandRegistry) registerWithModeKeys(mode Mode, cmd Command) {
	if r.commands[mode] == nil {
		r.commands[mode] = make(map[string]Command)
	}
	for _, key := range cmd.Keys() {
		r.commands[mode][key] = cmd
	}
}

// ============================================================================
// PendingCommandRegistry - Multi-key sequence dispatch
// ============================================================================

// PendingCommandRegistry provides operator+motion command dispatch for multi-key sequences.
// Maps (operator rune, second key string) -> prototype Command.
// Examples: ('g', 'g') -> MoveToFirstLineCommand, ('d', 'd') -> DeleteLineCommand
type PendingCommandRegistry struct {
	// commands maps operator -> second key -> prototype command
	commands map[rune]map[string]Command
}

// NewPendingCommandRegistry creates an empty pending command registry.
func NewPendingCommandRegistry() *PendingCommandRegistry {
	return &PendingCommandRegistry{
		commands: make(map[rune]map[string]Command),
	}
}

// Register adds a command for a specific operator and second key.
func (r *PendingCommandRegistry) Register(operator rune, secondKey string, cmd Command) {
	if r.commands[operator] == nil {
		r.commands[operator] = make(map[string]Command)
	}
	r.commands[operator][secondKey] = cmd
}

// Get retrieves a command for a specific operator and key sequence.
func (r *PendingCommandRegistry) Get(operator rune, keySequence string) (Command, bool) {
	if opMap, ok := r.commands[operator]; ok {
		if cmd, ok := opMap[keySequence]; ok {
			return cmd, true
		}
	}
	return nil, false
}

// HasPrefix returns true if there are any commands for the operator that start with the given prefix.
// This is used to determine if we should continue buffering keys for multi-key sequences.
func (r *PendingCommandRegistry) HasPrefix(operator rune, prefix string) bool {
	opMap, ok := r.commands[operator]
	if !ok {
		return false
	}
	for key := range opMap {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// ============================================================================
// Default Registries
// ============================================================================

// DefaultPendingRegistry is the global pending command registry with all multi-key sequences registered.
var DefaultPendingRegistry = newDefaultPendingRegistry()

// newDefaultPendingRegistry creates and populates the default pending command registry.
func newDefaultPendingRegistry() *PendingCommandRegistry {
	r := NewPendingCommandRegistry()

	// g prefix commands
	r.Register('g', "g", &MoveToFirstLineCommand{})

	// d prefix commands (delete operator + motion)
	r.Register('d', "d", &DeleteLineCommand{})
	r.Register('d', "w", &DeleteWordCommand{})
	r.Register('d', "$", &DeleteToEOLCommand{})
	r.Register('d', "j", &DeleteLinesDownCommand{})
	r.Register('d', "k", &DeleteLinesUpCommand{})
	r.Register('d', "G", &DeleteToLastLineCommand{})
	r.Register('d', "gg", &DeleteToFirstLineCommand{})

	// c prefix commands (change operator + motion)
	r.Register('c', "c", &ChangeLineCommand{})
	r.Register('c', "w", &ChangeWordCommand{})
	r.Register('c', "$", &ChangeToEOLCommand{})
	r.Register('c', "0", &ChangeToLineStartCommand{})
	r.Register('c', "j", &ChangeLinesDownCommand{})
	r.Register('c', "k", &ChangeLinesUpCommand{})
	r.Register('c', "G", &ChangeToLastLineCommand{})
	r.Register('c', "gg", &ChangeToFirstLineCommand{})

	// y prefix commands (yank operator + motion)
	r.Register('y', "y", &YankLineCommand{})
	r.Register('y', "w", &YankWordCommand{})
	r.Register('y', "$", &YankToEOLCommand{})

	// Text object commands - delete (diw, daw)
	r.Register('d', "iw", &DeleteTextObjectCommand{object: 'w', inner: true})
	r.Register('d', "aw", &DeleteTextObjectCommand{object: 'w', inner: false})

	// Text object commands - change (ciw, caw)
	r.Register('c', "iw", &ChangeTextObjectCommand{object: 'w', inner: true})
	r.Register('c', "aw", &ChangeTextObjectCommand{object: 'w', inner: false})

	// Text object commands - yank (yiw, yaw)
	r.Register('y', "iw", &YankTextObjectCommand{object: 'w', inner: true})
	r.Register('y', "aw", &YankTextObjectCommand{object: 'w', inner: false})

	// Text object commands - visual select (viw, vaw)
	r.Register('v', "iw", &VisualSelectTextObjectCommand{object: 'w', inner: true})
	r.Register('v', "aw", &VisualSelectTextObjectCommand{object: 'w', inner: false})

	// WORD object commands - delete (diW, daW)
	r.Register('d', "iW", &DeleteTextObjectCommand{object: 'W', inner: true})
	r.Register('d', "aW", &DeleteTextObjectCommand{object: 'W', inner: false})

	// WORD object commands - change (ciW, caW)
	r.Register('c', "iW", &ChangeTextObjectCommand{object: 'W', inner: true})
	r.Register('c', "aW", &ChangeTextObjectCommand{object: 'W', inner: false})

	// WORD object commands - yank (yiW, yaW)
	r.Register('y', "iW", &YankTextObjectCommand{object: 'W', inner: true})
	r.Register('y', "aW", &YankTextObjectCommand{object: 'W', inner: false})

	// WORD object commands - visual select (viW, vaW)
	r.Register('v', "iW", &VisualSelectTextObjectCommand{object: 'W', inner: true})
	r.Register('v', "aW", &VisualSelectTextObjectCommand{object: 'W', inner: false})

	// Double quote object commands - delete (di", da")
	r.Register('d', "i\"", &DeleteTextObjectCommand{object: '"', inner: true})
	r.Register('d', "a\"", &DeleteTextObjectCommand{object: '"', inner: false})

	// Double quote object commands - change (ci", ca")
	r.Register('c', "i\"", &ChangeTextObjectCommand{object: '"', inner: true})
	r.Register('c', "a\"", &ChangeTextObjectCommand{object: '"', inner: false})

	// Double quote object commands - yank (yi", ya")
	r.Register('y', "i\"", &YankTextObjectCommand{object: '"', inner: true})
	r.Register('y', "a\"", &YankTextObjectCommand{object: '"', inner: false})

	// Double quote object commands - visual select (vi", va")
	r.Register('v', "i\"", &VisualSelectTextObjectCommand{object: '"', inner: true})
	r.Register('v', "a\"", &VisualSelectTextObjectCommand{object: '"', inner: false})

	// Single quote object commands - delete (di', da')
	r.Register('d', "i'", &DeleteTextObjectCommand{object: '\'', inner: true})
	r.Register('d', "a'", &DeleteTextObjectCommand{object: '\'', inner: false})

	// Single quote object commands - change (ci', ca')
	r.Register('c', "i'", &ChangeTextObjectCommand{object: '\'', inner: true})
	r.Register('c', "a'", &ChangeTextObjectCommand{object: '\'', inner: false})

	// Single quote object commands - yank (yi', ya')
	r.Register('y', "i'", &YankTextObjectCommand{object: '\'', inner: true})
	r.Register('y', "a'", &YankTextObjectCommand{object: '\'', inner: false})

	// Single quote object commands - visual select (vi', va')
	r.Register('v', "i'", &VisualSelectTextObjectCommand{object: '\'', inner: true})
	r.Register('v', "a'", &VisualSelectTextObjectCommand{object: '\'', inner: false})

	// Parentheses object commands - delete (di(, da(, di), da))
	r.Register('d', "i(", &DeleteTextObjectCommand{object: '(', inner: true})
	r.Register('d', "a(", &DeleteTextObjectCommand{object: '(', inner: false})
	r.Register('d', "i)", &DeleteTextObjectCommand{object: ')', inner: true})
	r.Register('d', "a)", &DeleteTextObjectCommand{object: ')', inner: false})

	// Parentheses object commands - change (ci(, ca(, ci), ca))
	r.Register('c', "i(", &ChangeTextObjectCommand{object: '(', inner: true})
	r.Register('c', "a(", &ChangeTextObjectCommand{object: '(', inner: false})
	r.Register('c', "i)", &ChangeTextObjectCommand{object: ')', inner: true})
	r.Register('c', "a)", &ChangeTextObjectCommand{object: ')', inner: false})

	// Parentheses object commands - yank (yi(, ya(, yi), ya))
	r.Register('y', "i(", &YankTextObjectCommand{object: '(', inner: true})
	r.Register('y', "a(", &YankTextObjectCommand{object: '(', inner: false})
	r.Register('y', "i)", &YankTextObjectCommand{object: ')', inner: true})
	r.Register('y', "a)", &YankTextObjectCommand{object: ')', inner: false})

	// Parentheses object commands - visual select (vi(, va(, vi), va))
	r.Register('v', "i(", &VisualSelectTextObjectCommand{object: '(', inner: true})
	r.Register('v', "a(", &VisualSelectTextObjectCommand{object: '(', inner: false})
	r.Register('v', "i)", &VisualSelectTextObjectCommand{object: ')', inner: true})
	r.Register('v', "a)", &VisualSelectTextObjectCommand{object: ')', inner: false})

	// Square bracket object commands - delete (di[, da[, di], da])
	r.Register('d', "i[", &DeleteTextObjectCommand{object: '[', inner: true})
	r.Register('d', "a[", &DeleteTextObjectCommand{object: '[', inner: false})
	r.Register('d', "i]", &DeleteTextObjectCommand{object: ']', inner: true})
	r.Register('d', "a]", &DeleteTextObjectCommand{object: ']', inner: false})

	// Square bracket object commands - change (ci[, ca[, ci], ca])
	r.Register('c', "i[", &ChangeTextObjectCommand{object: '[', inner: true})
	r.Register('c', "a[", &ChangeTextObjectCommand{object: '[', inner: false})
	r.Register('c', "i]", &ChangeTextObjectCommand{object: ']', inner: true})
	r.Register('c', "a]", &ChangeTextObjectCommand{object: ']', inner: false})

	// Square bracket object commands - yank (yi[, ya[, yi], ya])
	r.Register('y', "i[", &YankTextObjectCommand{object: '[', inner: true})
	r.Register('y', "a[", &YankTextObjectCommand{object: '[', inner: false})
	r.Register('y', "i]", &YankTextObjectCommand{object: ']', inner: true})
	r.Register('y', "a]", &YankTextObjectCommand{object: ']', inner: false})

	// Square bracket object commands - visual select (vi[, va[, vi], va])
	r.Register('v', "i[", &VisualSelectTextObjectCommand{object: '[', inner: true})
	r.Register('v', "a[", &VisualSelectTextObjectCommand{object: '[', inner: false})
	r.Register('v', "i]", &VisualSelectTextObjectCommand{object: ']', inner: true})
	r.Register('v', "a]", &VisualSelectTextObjectCommand{object: ']', inner: false})

	// Curly brace object commands - delete (di{, da{, di}, da})
	r.Register('d', "i{", &DeleteTextObjectCommand{object: '{', inner: true})
	r.Register('d', "a{", &DeleteTextObjectCommand{object: '{', inner: false})
	r.Register('d', "i}", &DeleteTextObjectCommand{object: '}', inner: true})
	r.Register('d', "a}", &DeleteTextObjectCommand{object: '}', inner: false})

	// Curly brace object commands - change (ci{, ca{, ci}, ca})
	r.Register('c', "i{", &ChangeTextObjectCommand{object: '{', inner: true})
	r.Register('c', "a{", &ChangeTextObjectCommand{object: '{', inner: false})
	r.Register('c', "i}", &ChangeTextObjectCommand{object: '}', inner: true})
	r.Register('c', "a}", &ChangeTextObjectCommand{object: '}', inner: false})

	// Curly brace object commands - yank (yi{, ya{, yi}, ya})
	r.Register('y', "i{", &YankTextObjectCommand{object: '{', inner: true})
	r.Register('y', "a{", &YankTextObjectCommand{object: '{', inner: false})
	r.Register('y', "i}", &YankTextObjectCommand{object: '}', inner: true})
	r.Register('y', "a}", &YankTextObjectCommand{object: '}', inner: false})

	// Curly brace object commands - visual select (vi{, va{, vi}, va})
	r.Register('v', "i{", &VisualSelectTextObjectCommand{object: '{', inner: true})
	r.Register('v', "a{", &VisualSelectTextObjectCommand{object: '{', inner: false})
	r.Register('v', "i}", &VisualSelectTextObjectCommand{object: '}', inner: true})
	r.Register('v', "a}", &VisualSelectTextObjectCommand{object: '}', inner: false})

	// Bracket text object commands - delete (dib, dab)
	// 'b' matches innermost bracket pair: (), [], or {}
	r.Register('d', "ib", &DeleteTextObjectCommand{object: 'b', inner: true})
	r.Register('d', "ab", &DeleteTextObjectCommand{object: 'b', inner: false})

	// Bracket text object commands - change (cib, cab)
	r.Register('c', "ib", &ChangeTextObjectCommand{object: 'b', inner: true})
	r.Register('c', "ab", &ChangeTextObjectCommand{object: 'b', inner: false})

	// Bracket text object commands - yank (yib, yab)
	r.Register('y', "ib", &YankTextObjectCommand{object: 'b', inner: true})
	r.Register('y', "ab", &YankTextObjectCommand{object: 'b', inner: false})

	// Bracket text object commands - visual select (vib, vab)
	r.Register('v', "ib", &VisualSelectTextObjectCommand{object: 'b', inner: true})
	r.Register('v', "ab", &VisualSelectTextObjectCommand{object: 'b', inner: false})

	return r
}

// DefaultRegistry is the global command registry with all built-in commands registered.
var DefaultRegistry = newDefaultRegistry()

// newDefaultRegistry creates and populates the default command registry.
func newDefaultRegistry() *CommandRegistry {
	r := NewCommandRegistry()

	// ============================================================================
	// Normal Mode Commands
	// ============================================================================

	// Motion commands
	r.Register(&MoveLeftCommand{})
	r.Register(&MoveRightCommand{})
	r.Register(&MoveDownCommand{})
	r.Register(&MoveUpCommand{})
	r.Register(&MoveWordForwardCommand{})
	r.Register(&MoveWordBackwardCommand{})
	r.Register(&MoveWordEndCommand{})
	r.Register(&MoveToLineStartCommand{})
	r.Register(&MoveToLineEndCommand{})
	r.Register(&MoveToFirstNonBlankCommand{})
	r.Register(&MoveToLastLineCommand{})
	r.Register(&MoveToFirstLineCommand{})

	// Delete commands
	r.Register(&DeleteCharCommand{})
	r.Register(&DeleteToEOLCommand{})
	r.Register(&DeleteLineCommand{})
	r.Register(&DeleteWordCommand{})

	// Paste commands
	r.Register(&PasteAfterCommand{})  // 'p' - paste after cursor
	r.Register(&PasteBeforeCommand{}) // 'P' - paste before cursor

	// Change commands (single-key)
	r.Register(&ChangeToEOLCommand{})

	// Mode entry commands
	r.Register(&EnterInsertModeCommand{})
	r.Register(&EnterInsertModeAfterCommand{})
	r.Register(&EnterInsertModeAtEndCommand{})
	r.Register(&EnterInsertModeAtStartCommand{})
	r.Register(&InsertLineBelowCommand{})
	r.Register(&InsertLineAboveCommand{})

	// Visual mode entry commands
	r.Register(&EnterVisualModeCommand{})     // 'v' - character-wise visual mode
	r.Register(&EnterVisualLineModeCommand{}) // 'V' - line-wise visual mode

	// Replace mode entry command
	r.Register(&EnterReplaceModeCommand{}) // 'R' - replace mode (overwrite)

	// ============================================================================
	// Replace Mode Commands
	// ============================================================================

	r.Register(&ReplaceModeEscapeCommand{})    // <escape> - exit Replace mode
	r.Register(&ReplaceModeBackspaceCommand{}) // <backspace> - delete previous char
	r.Register(&ReplaceModeSpaceCommand{})     // <space> - overwrite/append space

	// ============================================================================
	// Visual Mode Commands
	// ============================================================================

	// Escape commands for visual modes
	r.registerWithModeKeys(ModeVisual, &VisualModeEscapeCommand{mode: ModeVisual})
	r.registerWithModeKeys(ModeVisualLine, &VisualModeEscapeCommand{mode: ModeVisualLine})

	// Toggle commands: v/V behavior within visual modes
	r.Register(&VisualModeToggleVCommand{})          // 'v' in ModeVisual -> Normal (toggle off)
	r.Register(&VisualModeToggleShiftVCommand{})     // 'V' in ModeVisual -> VisualLine (switch)
	r.Register(&VisualLineModeToggleVCommand{})      // 'v' in ModeVisualLine -> Visual (switch)
	r.Register(&VisualLineModeToggleShiftVCommand{}) // 'V' in ModeVisualLine -> Normal (toggle off)

	// Visual mode operations (delete, yank, change on selection)
	r.registerWithModeKeys(ModeVisual, &VisualDeleteCommand{mode: ModeVisual})         // 'd', 'x'
	r.registerWithModeKeys(ModeVisualLine, &VisualDeleteCommand{mode: ModeVisualLine}) // 'd', 'x'
	r.registerWithModeKeys(ModeVisual, &VisualYankCommand{mode: ModeVisual})           // 'y'
	r.registerWithModeKeys(ModeVisualLine, &VisualYankCommand{mode: ModeVisualLine})   // 'y'
	r.registerWithModeKeys(ModeVisual, &VisualChangeCommand{mode: ModeVisual})         // 'c'
	r.registerWithModeKeys(ModeVisualLine, &VisualChangeCommand{mode: ModeVisualLine}) // 'c'

	// Visual mode anchor swap ('o' - move cursor to other end of selection)
	r.registerWithModeKeys(ModeVisual, &VisualSwapAnchorCommand{mode: ModeVisual})         // 'o'
	r.registerWithModeKeys(ModeVisualLine, &VisualSwapAnchorCommand{mode: ModeVisualLine}) // 'o'

	// ============================================================================
	// Visual Mode Motion Commands (reuse existing motion implementations)
	// ============================================================================
	// Motion commands work in visual mode by moving cursor while anchor stays fixed.
	// Selection is computed from anchor + cursor at render time.

	// ModeVisual: All directions, word motions, line motions, document motions
	r.registerWithModeKeys(ModeVisual, &MoveLeftCommand{})            // h
	r.registerWithModeKeys(ModeVisual, &MoveRightCommand{})           // l
	r.registerWithModeKeys(ModeVisual, &MoveUpCommand{})              // k
	r.registerWithModeKeys(ModeVisual, &MoveDownCommand{})            // j
	r.registerWithModeKeys(ModeVisual, &MoveWordForwardCommand{})     // w
	r.registerWithModeKeys(ModeVisual, &MoveWordBackwardCommand{})    // b
	r.registerWithModeKeys(ModeVisual, &MoveWordEndCommand{})         // e
	r.registerWithModeKeys(ModeVisual, &MoveToLineStartCommand{})     // 0
	r.registerWithModeKeys(ModeVisual, &MoveToLineEndCommand{})       // $
	r.registerWithModeKeys(ModeVisual, &MoveToFirstNonBlankCommand{}) // ^
	r.registerWithModeKeys(ModeVisual, &MoveToFirstLineCommand{})     // gg
	r.registerWithModeKeys(ModeVisual, &MoveToLastLineCommand{})      // G

	// ModeVisualLine: Vertical motions only (h/l don't apply in line-wise mode)
	r.registerWithModeKeys(ModeVisualLine, &MoveUpCommand{})          // k
	r.registerWithModeKeys(ModeVisualLine, &MoveDownCommand{})        // j
	r.registerWithModeKeys(ModeVisualLine, &MoveToFirstLineCommand{}) // gg
	r.registerWithModeKeys(ModeVisualLine, &MoveToLastLineCommand{})  // G

	// Special key commands
	r.Register(&UndoCommand{})
	r.Register(&ConditionalRedoCommand{})
	r.Register(&StartPendingCommand{operator: 'g'})
	r.Register(&StartPendingCommand{operator: 'd'})
	r.Register(&StartPendingCommand{operator: 'c'})
	r.Register(&StartPendingCommand{operator: 'r'})
	r.Register(&StartPendingCommand{operator: 'y'})
	r.Register(&StartPendingCommand{operator: 'v'}) // Visual mode with text object support (viw, vaw, etc.)
	r.Register(&YankToEOLCommand{})                 // Y is alias for y$
	r.Register(&NormalModeEscapeCommand{})

	// ============================================================================
	// Insert Mode Commands
	// ============================================================================

	r.Register(&EscapeCommand{})
	r.Register(&BackspaceCommand{})
	r.Register(&DeleteKeyCommand{})
	r.Register(&SplitLineCommand{})
	r.Register(&SpaceCommand{})
	r.Register(&KillToLineStartCommand{})
	r.Register(&KillToLineEndCommand{})
	r.Register(&MoveToLineStartInsertCommand{})
	r.Register(&MoveToLineEndInsertCommand{})
	r.Register(&ArrowLeftCommand{})
	r.Register(&ArrowRightCommand{})
	r.Register(&ArrowUpCommand{})
	r.Register(&ArrowDownCommand{})

	// Arrow keys and Ctrl+B/F for Normal mode (reuse Insert mode commands)
	r.registerWithModeKeys(ModeNormal, &ArrowLeftCommand{})
	r.registerWithModeKeys(ModeNormal, &ArrowRightCommand{})
	r.registerWithModeKeys(ModeNormal, &ArrowUpCommand{})
	r.registerWithModeKeys(ModeNormal, &ArrowDownCommand{})

	// Submit commands - registered for both modes
	r.registerWithModeKeys(ModeNormal, &SubmitCommand{})
	r.registerWithModeKeys(ModeInsert, &SubmitCommand{})

	return r
}

// ============================================================================
// Helper Functions
// ============================================================================

// isWhitespace returns true if the rune is a space or tab.
func isWhitespace(r rune) bool {
	return r == ' ' || r == '\t'
}
