package vimtextarea

import (
	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"
)

// mockCommand is a test implementation of the Command interface.
// It tracks execution state and can be configured to fail.
type mockCommand struct {
	executed    bool
	undone      bool
	key         string
	failExecute bool
	failUndo    bool
}

func (c *mockCommand) Execute(m *Model) ExecuteResult {
	if c.failExecute {
		return Skipped
	}
	c.executed = true
	c.undone = false
	return Executed
}

func (c *mockCommand) Undo(m *Model) error {
	if c.failUndo {
		return assert.AnError
	}
	c.undone = true
	c.executed = false
	return nil
}

func (c *mockCommand) Keys() []string {
	return []string{c.key}
}

func (c *mockCommand) Mode() Mode {
	return ModeNormal
}

func (c *mockCommand) ID() string {
	return "mock." + c.key
}

func (c *mockCommand) IsUndoable() bool {
	return true // mock commands are undoable by default
}

func (c *mockCommand) ChangesContent() bool {
	return true // mock commands change content by default
}

func (c *mockCommand) IsModeChange() bool {
	return false // mock commands don't change mode by default
}

func newMockCommand(key string) *mockCommand {
	return &mockCommand{key: key}
}

// newTestModelWithContent creates a test model with the given content lines.
func newTestModelWithContent(lines ...string) *Model {
	if len(lines) == 0 {
		lines = []string{""}
	}
	return &Model{
		content:        lines,
		cursorRow:      0,
		cursorCol:      0,
		mode:           ModeNormal,
		history:        NewCommandHistory(),
		pendingBuilder: NewPendingCommandBuilder(),
	}
}

// enterVisualMode enters visual mode for tests by directly executing EnterVisualModeCommand.
// This bypasses the pending state that 'v' now enters (for text object support like viw/vaw).
func enterVisualMode(m *Model) {
	cmd := &EnterVisualModeCommand{}
	cmd.Execute(m)
}

// enterVisualLineMode enters visual line mode for tests by directly executing EnterVisualLineModeCommand.
func enterVisualLineMode(m *Model) {
	cmd := &EnterVisualLineModeCommand{}
	cmd.Execute(m)
}

// generateContentMutatingCommand generates a random content-mutating command
// that can be executed on the given model.
func generateContentMutatingCommand(t *rapid.T, m *Model) Command {
	cmdType := rapid.IntRange(0, 9).Draw(t, "cmdType")

	switch cmdType {
	case 0:
		// DeleteCharCommand - only if content is not empty
		if len(m.content) > 0 && len(m.content[m.cursorRow]) > 0 {
			return &DeleteCharCommand{row: m.cursorRow, col: m.cursorCol}
		}
		// Fall through to InsertTextCommand if we can't delete
		fallthrough
	case 1:
		// InsertTextCommand
		text := rapid.StringMatching(`[a-zA-Z0-9 ]{1,10}`).Draw(t, "text")
		return &InsertTextCommand{row: m.cursorRow, col: m.cursorCol, text: text}
	case 2:
		// DeleteLineCommand - only if we have lines
		if len(m.content) > 0 {
			return &DeleteLineCommand{}
		}
		fallthrough
	case 3:
		// DeleteToEOLCommand - only if not at end of line
		if m.cursorCol < len(m.content[m.cursorRow]) {
			return &DeleteToEOLCommand{row: m.cursorRow, col: m.cursorCol}
		}
		fallthrough
	case 4:
		// DeleteWordCommand - only if not at end of line
		if len(m.content[m.cursorRow]) > 0 && m.cursorCol < len(m.content[m.cursorRow]) {
			return &DeleteWordCommand{row: m.cursorRow, col: m.cursorCol}
		}
		fallthrough
	case 5:
		// SplitLineCommand
		return &SplitLineCommand{row: m.cursorRow, col: m.cursorCol}
	case 6:
		// BackspaceCommand - only if not at beginning
		if m.cursorRow > 0 || m.cursorCol > 0 {
			return &BackspaceCommand{}
		}
		fallthrough
	case 7:
		// DeleteKeyCommand - only if not at end
		if m.cursorCol < len(m.content[m.cursorRow]) || m.cursorRow < len(m.content)-1 {
			return &DeleteKeyCommand{}
		}
		fallthrough
	case 8:
		// InsertLineCommand (below)
		return &InsertLineCommand{above: false}
	case 9:
		// InsertLineCommand (above)
		return &InsertLineCommand{above: true}
	default:
		// Default to InsertTextCommand
		text := rapid.StringMatching(`[a-zA-Z0-9]{1,5}`).Draw(t, "defaultText")
		return &InsertTextCommand{row: m.cursorRow, col: m.cursorCol, text: text}
	}
}
