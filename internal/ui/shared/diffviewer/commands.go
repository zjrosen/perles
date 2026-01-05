// Package diffviewer provides keybinding-to-command mapping for the diff viewer.
package diffviewer

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/zjrosen/perles/internal/keys"
)

// commandID uniquely identifies commands in the diff viewer.
type commandID string

// Command IDs for diff viewer actions.
const (
	// Navigation commands
	cmdFocusFileList commandID = "focus_file_list"
	cmdFocusCommits  commandID = "focus_commits"
	cmdFocusDiff     commandID = "focus_diff"
	cmdFocusLeft     commandID = "focus_left"
	cmdFocusRight    commandID = "focus_right"
	cmdCyclePanes    commandID = "cycle_panes"
	cmdNextFile      commandID = "next_file"
	cmdPrevFile      commandID = "prev_file"
	cmdNextHunk      commandID = "next_hunk"
	cmdPrevHunk      commandID = "prev_hunk"

	// Scrolling commands
	cmdScrollUp   commandID = "scroll_up"
	cmdScrollDown commandID = "scroll_down"
	cmdGotoTop    commandID = "goto_top"
	cmdGotoBottom commandID = "goto_bottom"

	// Actions
	cmdSelectItem commandID = "select_item"
	cmdGoBack     commandID = "go_back"
	cmdCopyHunk   commandID = "copy_hunk"

	// View mode commands
	cmdToggleViewMode commandID = "toggle_view_mode"

	// Help and UI
	cmdShowHelp    commandID = "show_help"
	cmdCloseViewer commandID = "close_viewer"
)

// keyBindingToCommand maps key bindings to their corresponding command IDs.
// This mapping enables handleKeyMsg to route through executeCommand,
// eliminating duplicate navigation logic.
var keyBindingToCommand = map[*key.Binding]commandID{
	&keys.DiffViewer.NextFile:       cmdNextFile,
	&keys.DiffViewer.PrevFile:       cmdPrevFile,
	&keys.DiffViewer.FocusLeft:      cmdFocusLeft,
	&keys.DiffViewer.FocusRight:     cmdFocusRight,
	&keys.DiffViewer.Tab:            cmdCyclePanes,
	&keys.DiffViewer.Select:         cmdSelectItem,
	&keys.DiffViewer.Close:          cmdGoBack,
	&keys.DiffViewer.ScrollUp:       cmdScrollUp,
	&keys.DiffViewer.ScrollDown:     cmdScrollDown,
	&keys.DiffViewer.GotoTop:        cmdGotoTop,
	&keys.DiffViewer.GotoBottom:     cmdGotoBottom,
	&keys.DiffViewer.ToggleViewMode: cmdToggleViewMode,
	&keys.DiffViewer.Help:           cmdShowHelp,
	&keys.DiffViewer.CopyHunk:       cmdCopyHunk,
}

// keyToCommand returns the commandID for a given key message, or empty string if no match.
func keyToCommand(msg tea.KeyMsg) commandID {
	for binding, cmdID := range keyBindingToCommand {
		if key.Matches(msg, *binding) {
			return cmdID
		}
	}
	return ""
}
