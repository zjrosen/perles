// Package bqlinput provides a text input with BQL syntax highlighting and wrapping.
package bqlinput

import (
	"perles/internal/bql"
	"perles/internal/ui/styles"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Model is a single-line text input with BQL syntax highlighting.
type Model struct {
	value       string
	cursor      int // cursor position (0 = before first char)
	focused     bool
	width       int
	placeholder string

	// Styles
	cursorStyle      lipgloss.Style
	placeholderStyle lipgloss.Style
}

// New creates a new BQL input model.
func New() Model {
	return Model{
		value:            "",
		cursor:           0,
		focused:          false,
		width:            40,
		placeholder:      "",
		cursorStyle:      lipgloss.NewStyle().Underline(true),
		placeholderStyle: lipgloss.NewStyle().Foreground(styles.TextPlaceholderColor),
	}
}

// Value returns the current text value.
func (m Model) Value() string {
	return m.value
}

// SetValue sets the text value and clamps cursor.
func (m *Model) SetValue(v string) {
	m.value = v
	if m.cursor > len(v) {
		m.cursor = len(v)
	}
}

// Cursor returns the current cursor position.
func (m Model) Cursor() int {
	return m.cursor
}

// SetCursor sets the cursor position (clamped to valid range).
func (m *Model) SetCursor(pos int) {
	if pos < 0 {
		pos = 0
	}
	if pos > len(m.value) {
		pos = len(m.value)
	}
	m.cursor = pos
}

// Focused returns whether the input is focused.
func (m Model) Focused() bool {
	return m.focused
}

// Focus focuses the input.
func (m *Model) Focus() {
	m.focused = true
}

// Blur removes focus from the input.
func (m *Model) Blur() {
	m.focused = false
}

// SetWidth sets the display width.
func (m *Model) SetWidth(w int) {
	if w < 1 {
		w = 1
	}
	m.width = w
}

// Width returns the display width.
func (m Model) Width() int {
	return m.width
}

// Height returns the number of display lines needed for the current content.
// This accounts for text wrapping when content exceeds width.
func (m Model) Height() int {
	if m.value == "" {
		return 1
	}
	lines := m.wrapText()
	if len(lines) == 0 {
		return 1
	}
	return len(lines)
}

// SetPlaceholder sets the placeholder text.
func (m *Model) SetPlaceholder(p string) {
	m.placeholder = p
}

// Update handles key messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.focused {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyLeft:
			if msg.Alt {
				// Option+Left: move backward one word
				m.cursor = prevWordStart(m.value, m.cursor)
			} else if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyRight:
			if msg.Alt {
				// Option+Right: move forward one word
				m.cursor = nextWordEnd(m.value, m.cursor)
			} else if m.cursor < len(m.value) {
				m.cursor++
			}
		case tea.KeyCtrlF:
			// Ctrl+F: move forward one word
			m.cursor = nextWordEnd(m.value, m.cursor)
		case tea.KeyCtrlB:
			// Ctrl+B: move backward one word
			m.cursor = prevWordStart(m.value, m.cursor)
		case tea.KeyHome, tea.KeyCtrlA:
			m.cursor = 0
		case tea.KeyEnd, tea.KeyCtrlE:
			m.cursor = len(m.value)
		case tea.KeyBackspace:
			if m.cursor > 0 {
				m.value = m.value[:m.cursor-1] + m.value[m.cursor:]
				m.cursor--
			}
		case tea.KeyDelete:
			if m.cursor < len(m.value) {
				m.value = m.value[:m.cursor] + m.value[m.cursor+1:]
			}
		case tea.KeyCtrlK:
			// Kill to end of line
			m.value = m.value[:m.cursor]
		case tea.KeyCtrlU:
			// Kill to beginning of line
			m.value = m.value[m.cursor:]
			m.cursor = 0
		case tea.KeyRunes:
			// Handle Alt+f/b for word navigation (macOS option+arrow sends these)
			if msg.Alt && len(msg.Runes) == 1 {
				switch msg.Runes[0] {
				case 'f':
					// Alt+f (option+right on macOS): forward word
					m.cursor = nextWordEnd(m.value, m.cursor)
					return m, nil
				case 'b':
					// Alt+b (option+left on macOS): backward word
					m.cursor = prevWordStart(m.value, m.cursor)
					return m, nil
				}
			}
			// Insert characters at cursor
			for _, r := range msg.Runes {
				m.value = m.value[:m.cursor] + string(r) + m.value[m.cursor:]
				m.cursor++
			}
		case tea.KeySpace:
			m.value = m.value[:m.cursor] + " " + m.value[m.cursor:]
			m.cursor++
		}
	}

	return m, nil
}

// ANSI codes for cursor - only toggle reverse, don't reset other styles
const (
	cursorOn  = "\x1b[7m"  // reverse video on
	cursorOff = "\x1b[27m" // reverse video off (not full reset)
)

// View renders the input with syntax highlighting and text wrapping.
// Returns multiple lines joined by newlines when content exceeds width.
func (m Model) View() string {
	lines := m.wrapText()
	return strings.Join(lines, "\n")
}

// wrapText returns the highlighted text wrapped to fit within the configured width.
// Wrapping is ANSI-aware using lipgloss.Width for visual width measurement.
func (m Model) wrapText() []string {
	// Empty value - show placeholder or cursor
	if m.value == "" {
		if m.focused {
			return []string{cursorOn + " " + cursorOff}
		}
		if m.placeholder != "" {
			return []string{m.placeholderStyle.Render(m.placeholder)}
		}
		return []string{""}
	}

	// Highlight the full text
	highlighted := bql.Highlight(m.value)

	// Insert cursor if focused
	if m.focused {
		highlighted = insertCursor(highlighted, m.value, m.cursor)
	}

	// If text fits on one line, return as-is
	if lipgloss.Width(highlighted) <= m.width {
		return []string{highlighted}
	}

	// Word-aware wrapping that preserves all characters including spaces
	return wrapHighlightedText(highlighted, m.width)
}

// wrapHighlightedText wraps highlighted text at word boundaries, preserving all characters.
// This is ANSI-aware using lipgloss.Width for accurate visual width.
func wrapHighlightedText(text string, maxWidth int) []string {
	if maxWidth <= 0 {
		maxWidth = 40
	}

	// Word-aware wrapping that preserves all characters including spaces
	var lines []string
	var currentLine strings.Builder
	currentWidth := 0
	lastSpaceIdx := -1  // byte index in currentLine where last space was written
	lastSpaceWidth := 0 // visual width at that point

	i := 0
	for i < len(text) {
		// Handle ANSI escape sequences (don't count them in width)
		if text[i] == '\x1b' {
			start := i
			for i < len(text) && text[i] != 'm' {
				i++
			}
			if i < len(text) {
				i++ // include the 'm'
			}
			currentLine.WriteString(text[start:i])
			continue
		}

		// Check if adding this character would exceed maxWidth
		if currentWidth >= maxWidth {
			// Try to break at last space if we have one
			if lastSpaceIdx > 0 {
				// Keep everything up to and including the space on this line
				lineContent := currentLine.String()[:lastSpaceIdx+1]
				lines = append(lines, lineContent)

				// Start new line with the rest (after the space)
				remainder := currentLine.String()[lastSpaceIdx+1:]
				currentLine.Reset()
				currentLine.WriteString(remainder)
				currentWidth = currentWidth - lastSpaceWidth - 1 // -1 for the space
			} else {
				// No space to break at - hard break
				lines = append(lines, currentLine.String())
				currentLine.Reset()
				currentWidth = 0
			}
			lastSpaceIdx = -1
			lastSpaceWidth = 0
		}

		// Track space positions for word wrapping
		if text[i] == ' ' {
			lastSpaceIdx = currentLine.Len()
			lastSpaceWidth = currentWidth
		}

		currentLine.WriteByte(text[i])
		currentWidth++
		i++
	}

	// Don't forget the last line
	if currentLine.Len() > 0 {
		lines = append(lines, currentLine.String())
	}

	if len(lines) == 0 {
		return []string{text}
	}
	return lines
}

// insertCursor inserts a cursor at the given position in highlighted text.
// Uses targeted ANSI codes that don't reset surrounding styles.
func insertCursor(highlighted, original string, cursor int) string {
	// Cursor at end - append cursor block
	if cursor >= len(original) {
		return highlighted + cursorOn + " " + cursorOff
	}

	// Map cursor position from original text to highlighted text
	// by walking through both, skipping ANSI codes in highlighted
	origIdx := 0
	highIdx := 0

	for origIdx < cursor && highIdx < len(highlighted) {
		// Skip ANSI escape sequences
		if highlighted[highIdx] == '\x1b' {
			for highIdx < len(highlighted) && highlighted[highIdx] != 'm' {
				highIdx++
			}
			if highIdx < len(highlighted) {
				highIdx++ // skip 'm'
			}
			continue
		}
		origIdx++
		highIdx++
	}

	// Skip any ANSI codes at cursor position
	for highIdx < len(highlighted) && highlighted[highIdx] == '\x1b' {
		for highIdx < len(highlighted) && highlighted[highIdx] != 'm' {
			highIdx++
		}
		if highIdx < len(highlighted) {
			highIdx++
		}
	}

	// Insert cursor styling around the character
	if highIdx >= len(highlighted) {
		return highlighted + cursorOn + " " + cursorOff
	}

	charUnderCursor := string(highlighted[highIdx])
	return highlighted[:highIdx] + cursorOn + charUnderCursor + cursorOff + highlighted[highIdx+1:]
}

// nextWordEnd finds the position after the next word from pos.
// Skips non-word characters first, then skips word characters.
func nextWordEnd(s string, pos int) int {
	n := len(s)
	// Skip non-word characters first
	for pos < n && !isWordChar(rune(s[pos])) {
		pos++
	}
	// Skip word characters
	for pos < n && isWordChar(rune(s[pos])) {
		pos++
	}
	return pos
}

// prevWordStart finds the position at the start of the previous word from pos.
// Skips non-word characters backward first, then skips word characters backward.
func prevWordStart(s string, pos int) int {
	// Skip non-word characters backward
	for pos > 0 && !isWordChar(rune(s[pos-1])) {
		pos--
	}
	// Skip word characters backward
	for pos > 0 && isWordChar(rune(s[pos-1])) {
		pos--
	}
	return pos
}

// isWordChar returns true if c is a word character (alphanumeric or underscore).
func isWordChar(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}
