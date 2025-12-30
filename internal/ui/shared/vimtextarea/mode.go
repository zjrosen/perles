// Package vimtextarea provides a vim-enabled textarea component for Bubble Tea applications.
package vimtextarea

// Mode represents the current vim editing mode.
type Mode int

const (
	// ModeNormal is the default vim mode for navigation and commands.
	ModeNormal Mode = iota
	// ModeInsert is the mode for inserting text.
	ModeInsert
	// ModeVisual is the mode for visual selection (character-wise).
	ModeVisual
	// ModeVisualLine is the mode for visual line selection.
	ModeVisualLine
	// ModeReplace is the mode for replacing text (overwrite mode).
	ModeReplace
)

// String returns the string representation of the mode.
func (m Mode) String() string {
	switch m {
	case ModeNormal:
		return "NORMAL"
	case ModeInsert:
		return "INSERT"
	case ModeVisual:
		return "VISUAL"
	case ModeVisualLine:
		return "VISUAL LINE"
	case ModeReplace:
		return "REPLACE"
	default:
		return "UNKNOWN"
	}
}
