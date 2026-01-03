// Package keys contains keybinding definitions.
package keys

import "github.com/charmbracelet/bubbles/key"

// Common contains keybindings shared across all modes.
var Common = struct {
	Up     key.Binding
	Down   key.Binding
	Left   key.Binding
	Right  key.Binding
	Enter  key.Binding
	Escape key.Binding
	Quit   key.Binding
	Help   key.Binding
}{
	Up: key.NewBinding(
		key.WithKeys("k", "up"),
		key.WithHelp("k/↑", "move up"),
	),
	Down: key.NewBinding(
		key.WithKeys("j", "down"),
		key.WithHelp("j/↓", "move down"),
	),
	Left: key.NewBinding(
		key.WithKeys("h", "left"),
		key.WithHelp("h/←", "move left"),
	),
	Right: key.NewBinding(
		key.WithKeys("l", "right"),
		key.WithHelp("l/→", "move right"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "confirm"),
	),
	Escape: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "cancel"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "toggle help"),
	),
}

// Kanban contains keybindings specific to kanban mode.
var Kanban = struct {
	Enter            key.Binding // Kanban-specific enter (open tree view)
	Escape           key.Binding // Kanban-specific escape (go back)
	Refresh          key.Binding
	Yank             key.Binding
	Status           key.Binding
	Priority         key.Binding
	AddColumn        key.Binding
	EditColumn       key.Binding
	MoveColumnLeft   key.Binding
	MoveColumnRight  key.Binding
	NextView         key.Binding
	PrevView         key.Binding
	ViewMenu         key.Binding
	DeleteColumn     key.Binding
	SearchFromColumn key.Binding
	SwitchMode       key.Binding
	ToggleStatus     key.Binding
	Orchestrate      key.Binding // Start orchestration mode
	QuitConfirm      key.Binding // Ctrl+C quit with confirmation (kanban-specific)
}{
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "open tree view"),
	),
	Escape: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "go back"),
	),
	Refresh: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "refresh issues"),
	),
	Yank: key.NewBinding(
		key.WithKeys("y"),
		key.WithHelp("y", "copy issue ID"),
	),
	Status: key.NewBinding(
		key.WithKeys("s"),
		key.WithHelp("s", "change status"),
	),
	Priority: key.NewBinding(
		key.WithKeys("p"),
		key.WithHelp("p", "change priority"),
	),
	AddColumn: key.NewBinding(
		key.WithKeys("a"),
		key.WithHelp("a", "add column"),
	),
	EditColumn: key.NewBinding(
		key.WithKeys("e"),
		key.WithHelp("e", "edit column"),
	),
	MoveColumnLeft: key.NewBinding(
		key.WithKeys("ctrl+h"),
		key.WithHelp("ctrl+h", "move column left"),
	),
	MoveColumnRight: key.NewBinding(
		key.WithKeys("ctrl+l"),
		key.WithHelp("ctrl+l", "move column right"),
	),
	NextView: key.NewBinding(
		key.WithKeys("ctrl+j", "ctrl+n"),
		key.WithHelp("ctrl+j/n", "next view"),
	),
	PrevView: key.NewBinding(
		key.WithKeys("ctrl+k", "ctrl+p"),
		key.WithHelp("ctrl+k/p", "previous view"),
	),
	ViewMenu: key.NewBinding(
		key.WithKeys("ctrl+v"),
		key.WithHelp("ctrl+v", "view menu"),
	),
	DeleteColumn: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "delete column"),
	),
	SearchFromColumn: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "search column"),
	),
	SwitchMode: key.NewBinding(
		key.WithKeys("ctrl+@"),
		key.WithHelp("^space", "search mode"),
	),
	ToggleStatus: key.NewBinding(
		key.WithKeys("w"),
		key.WithHelp("w", "toggle status bar"),
	),
	Orchestrate: key.NewBinding(
		key.WithKeys("ctrl+o"),
		key.WithHelp("ctrl+o", "orchestration mode"),
	),
	QuitConfirm: key.NewBinding(
		key.WithKeys("ctrl+c"),
		key.WithHelp("ctrl+c", "quit"),
	),
}

// Search contains keybindings specific to search mode.
var Search = struct {
	Up          key.Binding
	Down        key.Binding
	Left        key.Binding
	Right       key.Binding
	FocusSearch key.Binding
	Execute     key.Binding
	Blur        key.Binding
	OpenTree    key.Binding
	Edit        key.Binding
	Priority    key.Binding
	Status      key.Binding
	Yank        key.Binding
	SaveColumn  key.Binding
	SwitchMode  key.Binding
	Help        key.Binding
	Quit        key.Binding
	QuitConfirm key.Binding // Ctrl+C quit with confirmation (search-specific)
}{
	Up: key.NewBinding(
		key.WithKeys("k", "up"),
		key.WithHelp("k/↑", "move up"),
	),
	Down: key.NewBinding(
		key.WithKeys("j", "down"),
		key.WithHelp("j/↓", "move down"),
	),
	Left: key.NewBinding(
		key.WithKeys("h", "left"),
		key.WithHelp("h", "focus results"),
	),
	Right: key.NewBinding(
		key.WithKeys("l", "right"),
		key.WithHelp("l", "focus details"),
	),
	FocusSearch: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "focus search"),
	),
	Execute: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "execute query"),
	),
	Blur: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "exit to kanban"),
	),
	OpenTree: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "open tree view"),
	),
	Edit: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "edit field"),
	),
	Priority: key.NewBinding(
		key.WithKeys("p"),
		key.WithHelp("p", "change priority"),
	),
	Status: key.NewBinding(
		key.WithKeys("s"),
		key.WithHelp("s", "change status"),
	),
	Yank: key.NewBinding(
		key.WithKeys("y"),
		key.WithHelp("y", "copy issue ID"),
	),
	SaveColumn: key.NewBinding(
		key.WithKeys("ctrl+s"),
		key.WithHelp("ctrl+s", "save to view"),
	),
	SwitchMode: key.NewBinding(
		key.WithKeys("ctrl+@"),
		key.WithHelp("ctrl+space", "switch mode"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "toggle help"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
	QuitConfirm: key.NewBinding(
		key.WithKeys("ctrl+c"),
		key.WithHelp("ctrl+c", "quit"),
	),
}

// Component contains keybindings shared across UI components.
var Component = struct {
	Confirm    key.Binding
	Cancel     key.Binding
	Tab        key.Binding
	ShiftTab   key.Binding
	Delete     key.Binding
	Next       key.Binding // Alternative navigation (ctrl+n)
	Prev       key.Binding // Alternative navigation (ctrl+p)
	GotoTop    key.Binding // Navigate to top (g)
	GotoBottom key.Binding // Navigate to bottom (G)
	EditAction key.Binding // Edit action (ctrl+e) - opens edit menu
	DelAction  key.Binding // Delete action (d) - triggers delete
	Clear      key.Binding // Clear action (c) - clears content
	Toggle     key.Binding // Toggle action (space)
	ModeToggle key.Binding // Mode toggle (m)
	Close      key.Binding // Close overlay (ctrl+x)
	Save       key.Binding // Save action (ctrl+s)
}{
	Confirm: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "confirm"),
	),
	Cancel: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "cancel"),
	),
	Tab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "next field"),
	),
	ShiftTab: key.NewBinding(
		key.WithKeys("shift+tab"),
		key.WithHelp("shift+tab", "prev field"),
	),
	Delete: key.NewBinding(
		key.WithKeys("ctrl+d", "backspace"),
		key.WithHelp("ctrl+d", "delete"),
	),
	Next: key.NewBinding(
		key.WithKeys("ctrl+n"),
		key.WithHelp("ctrl+n", "next"),
	),
	Prev: key.NewBinding(
		key.WithKeys("ctrl+p"),
		key.WithHelp("ctrl+p", "previous"),
	),
	GotoTop: key.NewBinding(
		key.WithKeys("g"),
		key.WithHelp("g", "go to top"),
	),
	GotoBottom: key.NewBinding(
		key.WithKeys("G"),
		key.WithHelp("G", "go to bottom"),
	),
	EditAction: key.NewBinding(
		key.WithKeys("ctrl+e"),
		key.WithHelp("ctrl+e", "edit issue"),
	),
	DelAction: key.NewBinding(
		key.WithKeys("ctrl+d"),
		key.WithHelp("ctrl+d", "delete issue"),
	),
	Clear: key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "clear"),
	),
	Toggle: key.NewBinding(
		key.WithKeys(" "),
		key.WithHelp("space", "toggle"),
	),
	ModeToggle: key.NewBinding(
		key.WithKeys("m"),
		key.WithHelp("m", "toggle mode"),
	),
	Close: key.NewBinding(
		key.WithKeys("ctrl+x"),
		key.WithHelp("ctrl+x", "close"),
	),
	Save: key.NewBinding(
		key.WithKeys("ctrl+s"),
		key.WithHelp("ctrl+s", "save"),
	),
}

// LogOverlay contains keybindings specific to the log overlay.
var LogOverlay = struct {
	FilterDebug key.Binding
	FilterInfo  key.Binding
	FilterWarn  key.Binding
	FilterError key.Binding
}{
	FilterDebug: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "debug level"),
	),
	FilterInfo: key.NewBinding(
		key.WithKeys("i"),
		key.WithHelp("i", "info level"),
	),
	FilterWarn: key.NewBinding(
		key.WithKeys("w"),
		key.WithHelp("w", "warn level"),
	),
	FilterError: key.NewBinding(
		key.WithKeys("e"),
		key.WithHelp("e", "error level"),
	),
}

// ShortHelp returns keybindings for the short help view (kanban mode).
func ShortHelp() []key.Binding {
	return []key.Binding{Common.Help, Kanban.QuitConfirm}
}

// FullHelp returns keybindings for the full help view (kanban mode).
func FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{Common.Up, Common.Down, Common.Left, Common.Right},
		{Common.Enter, Kanban.Refresh, Kanban.Yank, Kanban.Status, Kanban.Priority, Kanban.AddColumn, Kanban.EditColumn, Kanban.MoveColumnLeft, Kanban.MoveColumnRight},
		{Kanban.NextView, Kanban.PrevView, Kanban.ViewMenu, Kanban.DeleteColumn},
		{Common.Help, Kanban.ToggleStatus, Common.Escape, Kanban.QuitConfirm},
	}
}
