// Package keys contains keybinding definitions.
package keys

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines the keybindings for the application.
type KeyMap struct {
	// Navigation
	Up    key.Binding
	Down  key.Binding
	Left  key.Binding
	Right key.Binding

	// Actions
	Enter           key.Binding
	Refresh         key.Binding
	Yank            key.Binding
	Status          key.Binding
	Priority        key.Binding
	AddColumn       key.Binding
	EditColumn      key.Binding
	MoveColumnLeft  key.Binding
	MoveColumnRight key.Binding

	// View cycling
	NextView         key.Binding
	PrevView         key.Binding
	ViewMenu         key.Binding
	DeleteColumn     key.Binding
	SearchFromColumn key.Binding

	// General
	SwitchMode   key.Binding
	Help         key.Binding
	Escape       key.Binding
	Quit         key.Binding
	ToggleStatus key.Binding
}

// DefaultKeyMap returns the default keybindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		// Navigation
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

		// Actions
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "view details"),
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

		// View cycling
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
			key.WithKeys("ctrl+d"),
			key.WithHelp("ctrl+d", "delete column"),
		),
		SearchFromColumn: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search column"),
		),

		// General
		SwitchMode: key.NewBinding(
			key.WithKeys("ctrl+@"),
			key.WithHelp("^space", "search mode"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "toggle help"),
		),
		Escape: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "go back"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		ToggleStatus: key.NewBinding(
			key.WithKeys("w"),
			key.WithHelp("w", "toggle status bar"),
		),
	}
}

// ShortHelp returns keybindings for the short help view.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Help, k.Quit}
}

// FullHelp returns keybindings for the full help view.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right}, // Navigation
		{k.Enter, k.Refresh, k.Yank, k.Status, k.Priority, k.AddColumn, k.EditColumn, k.MoveColumnLeft, k.MoveColumnRight}, // Actions
		{k.NextView, k.PrevView, k.ViewMenu, k.DeleteColumn},                                                               // Views
		{k.Help, k.ToggleStatus, k.Escape, k.Quit},                                                                         // General
	}
}

// SearchKeyMap defines the keybindings for search mode.
type SearchKeyMap struct {
	// Navigation
	Up    key.Binding
	Down  key.Binding
	Left  key.Binding
	Right key.Binding

	// Search
	FocusSearch key.Binding
	Execute     key.Binding
	Blur        key.Binding

	// Editing
	Edit       key.Binding
	Priority   key.Binding
	Status     key.Binding
	Yank       key.Binding
	SaveColumn key.Binding

	// General
	SwitchMode key.Binding
	Help       key.Binding
	Quit       key.Binding
}

// DefaultSearchKeyMap returns the keybindings for search mode.
func DefaultSearchKeyMap() SearchKeyMap {
	return SearchKeyMap{
		// Navigation
		Up: key.NewBinding(
			key.WithKeys("k", "up"),
			key.WithHelp("k/↑", "move up"),
		),
		Down: key.NewBinding(
			key.WithKeys("j", "down"),
			key.WithHelp("j/↓", "move down"),
		),
		Left: key.NewBinding(
			key.WithKeys("h"),
			key.WithHelp("h", "focus results"),
		),
		Right: key.NewBinding(
			key.WithKeys("l"),
			key.WithHelp("l", "focus details"),
		),

		// Search
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
			key.WithHelp("esc", "blur input"),
		),

		// Editing
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

		// General
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
	}
}
