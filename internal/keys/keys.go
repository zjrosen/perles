// Package keys contains keybinding definitions.
package keys

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
)

// translateToTerminal converts user-friendly key names to terminal codes.
// Example: "ctrl+space" -> "ctrl+@" (terminal sends ctrl+@ for ctrl+space)
func translateToTerminal(key string) string {
	normalized := strings.ToLower(key)
	// Check for ctrl+space variants before trimming (space might be significant)
	if normalized == "ctrl+space" || normalized == "ctrl+ " {
		return "ctrl+@"
	}
	// For other keys, trim and normalize
	return strings.ToLower(strings.TrimSpace(key))
}

// translateToDisplay converts terminal codes to user-friendly display text.
// Example: "ctrl+@" -> "ctrl+space"
func translateToDisplay(key string) string {
	normalized := strings.ToLower(strings.TrimSpace(key))
	if normalized == "ctrl+@" {
		return "ctrl+space"
	}

	return normalized
}

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
	Dashboard        key.Binding // Open multi-workflow dashboard
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
	Dashboard: key.NewBinding(
		key.WithKeys("ctrl+o"),
		key.WithHelp("ctrl+o", "dashboard"),
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

// App contains keybindings for app-level actions (cross-mode).
var App = struct {
	ToggleChatPanel key.Binding
	ChatFocus       key.Binding
	ChatNextTab     key.Binding
	ChatPrevTab     key.Binding
	ChatNextSession key.Binding
	ChatPrevSession key.Binding
}{
	ToggleChatPanel: key.NewBinding(
		key.WithKeys("ctrl+w"),
		key.WithHelp("ctrl+w", "toggle chat panel"),
	),
	ChatFocus: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "switch chat/board focus"),
	),
	ChatNextTab: key.NewBinding(
		key.WithKeys("ctrl+j"),
		key.WithHelp("ctrl+j", "next chat tab"),
	),
	ChatPrevTab: key.NewBinding(
		key.WithKeys("ctrl+k"),
		key.WithHelp("ctrl+k", "prev chat tab"),
	),
	ChatNextSession: key.NewBinding(
		key.WithKeys("ctrl+n"),
		key.WithHelp("ctrl+n", "next chat session"),
	),
	ChatPrevSession: key.NewBinding(
		key.WithKeys("ctrl+p"),
		key.WithHelp("ctrl+p", "prev chat session"),
	),
}

// DiffViewer contains keybindings specific to the diff viewer overlay.
var DiffViewer = struct {
	Open           key.Binding // ctrl+g - opens diff viewer
	Close          key.Binding // esc, q - closes diff viewer
	NextFile       key.Binding // j, down - next file in list
	PrevFile       key.Binding // k, up - previous file in list
	ScrollUp       key.Binding // ctrl+u, pgup - scroll diff up
	ScrollDown     key.Binding // ctrl+d, pgdn - scroll diff down
	GotoTop        key.Binding // g - go to top of diff
	GotoBottom     key.Binding // G - go to bottom of diff
	FocusLeft      key.Binding // h - focus file list
	FocusRight     key.Binding // l - focus diff pane
	Tab            key.Binding // tab - cycle through panes
	Select         key.Binding // enter - select item (load diff for commit)
	Help           key.Binding // ? - show help overlay
	NextHunk       key.Binding // ] - jump to next hunk
	PrevHunk       key.Binding // [ - jump to previous hunk
	CopyHunk       key.Binding // y - copy current hunk to clipboard
	ToggleViewMode key.Binding // ctrl+v - toggle between unified and side-by-side
}{
	Open: key.NewBinding(
		key.WithKeys("ctrl+g"),
		key.WithHelp("ctrl+g", "git diff"),
	),
	Close: key.NewBinding(
		key.WithKeys("esc", "q"),
		key.WithHelp("esc/q", "close diff"),
	),
	NextFile: key.NewBinding(
		key.WithKeys("j", "down"),
		key.WithHelp("j/↓", "next file"),
	),
	PrevFile: key.NewBinding(
		key.WithKeys("k", "up"),
		key.WithHelp("k/↑", "prev file"),
	),
	ScrollUp: key.NewBinding(
		key.WithKeys("ctrl+u", "pgup"),
		key.WithHelp("ctrl+u", "scroll up"),
	),
	ScrollDown: key.NewBinding(
		key.WithKeys("ctrl+d", "pgdown"),
		key.WithHelp("ctrl+d", "scroll down"),
	),
	GotoTop: key.NewBinding(
		key.WithKeys("g"),
		key.WithHelp("g", "go to top"),
	),
	GotoBottom: key.NewBinding(
		key.WithKeys("G"),
		key.WithHelp("G", "go to bottom"),
	),
	FocusLeft: key.NewBinding(
		key.WithKeys("h", "left"),
		key.WithHelp("h/←", "file list"),
	),
	FocusRight: key.NewBinding(
		key.WithKeys("l", "right"),
		key.WithHelp("l/→", "diff pane"),
	),
	Tab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "cycle panes"),
	),
	Select: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "show help"),
	),
	NextHunk: key.NewBinding(
		key.WithKeys("]"),
		key.WithHelp("]", "next hunk"),
	),
	PrevHunk: key.NewBinding(
		key.WithKeys("["),
		key.WithHelp("[", "prev hunk"),
	),
	CopyHunk: key.NewBinding(
		key.WithKeys("y"),
		key.WithHelp("y", "copy hunk"),
	),
	ToggleViewMode: key.NewBinding(
		key.WithKeys("ctrl+v"),
		key.WithHelp("^v", "toggle view"),
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

// Dashboard contains keybindings specific to dashboard mode.
var Dashboard = struct {
	Up              key.Binding
	Down            key.Binding
	Left            key.Binding
	Right           key.Binding
	Tab             key.Binding
	GotoTop         key.Binding
	GotoBottom      key.Binding
	Enter           key.Binding
	Start           key.Binding
	Stop            key.Binding
	New             key.Binding
	Rename          key.Binding
	Filter          key.Binding
	ClearFilter     key.Binding
	Help            key.Binding
	Quit            key.Binding
	CoordinatorChat key.Binding
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
		key.WithHelp("h/←", "focus list"),
	),
	Right: key.NewBinding(
		key.WithKeys("l", "right"),
		key.WithHelp("l/→", "focus details"),
	),
	Tab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "cycle focus"),
	),
	GotoTop: key.NewBinding(
		key.WithKeys("g"),
		key.WithHelp("g", "go to first"),
	),
	GotoBottom: key.NewBinding(
		key.WithKeys("G"),
		key.WithHelp("G", "go to last"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "view details"),
	),
	Start: key.NewBinding(
		key.WithKeys("s"),
		key.WithHelp("s", "start/resume workflow"),
	),
	Stop: key.NewBinding(
		key.WithKeys("x"),
		key.WithHelp("x", "pause workflow"),
	),
	New: key.NewBinding(
		key.WithKeys("n", "N"),
		key.WithHelp("n", "new workflow"),
	),
	Rename: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "rename workflow"),
	),
	Filter: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "filter"),
	),
	ClearFilter: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "clear filter"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "toggle help"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q"),
		key.WithHelp("q", "quit"),
	),
	CoordinatorChat: key.NewBinding(
		key.WithKeys("ctrl+w"),
		key.WithHelp("ctrl+w", "coordinator chat"),
	),
}

// DiffViewerShortHelp returns keybindings for the short help view (diff viewer).
func DiffViewerShortHelp() []key.Binding {
	return []key.Binding{
		DiffViewer.Close,
		DiffViewer.NextFile,
		DiffViewer.PrevFile,
	}
}

// DiffViewerFullHelp returns keybindings for the full help view (diff viewer).
func DiffViewerFullHelp() [][]key.Binding {
	return [][]key.Binding{
		{DiffViewer.NextFile, DiffViewer.PrevFile, DiffViewer.FocusLeft, DiffViewer.FocusRight},
		{DiffViewer.ScrollUp, DiffViewer.ScrollDown, DiffViewer.NextHunk, DiffViewer.PrevHunk},
		{DiffViewer.CopyHunk, DiffViewer.ToggleViewMode, DiffViewer.Help, DiffViewer.Close},
	}
}

// DashboardShortHelp returns keybindings for the short help view (dashboard mode).
func DashboardShortHelp() []key.Binding {
	return []key.Binding{
		Dashboard.Help,
		Dashboard.Filter,
		Dashboard.Quit,
	}
}

// DashboardFullHelp returns keybindings for the full help view (dashboard mode).
func DashboardFullHelp() [][]key.Binding {
	return [][]key.Binding{
		{Dashboard.Up, Dashboard.Down, Dashboard.GotoTop, Dashboard.GotoBottom},
		{Dashboard.Enter, Dashboard.Stop},
		{Dashboard.New, Dashboard.Rename, Dashboard.Filter, Dashboard.ClearFilter},
		{Dashboard.Help, Dashboard.Quit},
	}
}

// ApplyConfig applies user-configured keybindings to the package-level bindings.
func ApplyConfig(searchKey, dashboardKey string) {
	if searchKey != "" {
		terminalKey := translateToTerminal(searchKey)
		displayKey := translateToDisplay(searchKey)
		Kanban.SwitchMode.SetKeys(terminalKey)
		Kanban.SwitchMode.SetHelp(displayKey, "search mode")
		Search.SwitchMode.SetKeys(terminalKey)
		Search.SwitchMode.SetHelp(displayKey, "switch mode")
	}

	if dashboardKey != "" {
		terminalKey := translateToTerminal(dashboardKey)
		displayKey := translateToDisplay(dashboardKey)
		Kanban.Dashboard.SetKeys(terminalKey)
		Kanban.Dashboard.SetHelp(displayKey, "dashboard")
	}
}

// ResetForTesting resets keybindings to defaults for testing.
// Only call from test files.
func ResetForTesting() {
	Kanban.SwitchMode.SetKeys("ctrl+@")
	Kanban.SwitchMode.SetHelp("^space", "search mode")
	Search.SwitchMode.SetKeys("ctrl+@")
	Search.SwitchMode.SetHelp("ctrl+space", "switch mode")
	Kanban.Dashboard.SetKeys("ctrl+o")
	Kanban.Dashboard.SetHelp("ctrl+o", "dashboard")
}
