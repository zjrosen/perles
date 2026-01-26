package selection

import (
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/rivo/uniseg"
)

// Clipboard is the interface for copy operations.
type Clipboard interface {
	Copy(text string) error
}

// ToastFunc creates a toast command with the given message and error status.
// This allows the pane to trigger toasts without depending on specific message types.
type ToastFunc func(message string, isError bool) tea.Cmd

// PaneConfig configures a SelectablePane.
type PaneConfig struct {
	// Clipboard for copy operations. If nil, selection still works but copy is disabled.
	Clipboard Clipboard

	// MakeToast creates toast commands. If nil, no toasts are shown on copy.
	MakeToast ToastFunc

	// ContentStartX is the X offset where content starts (after left border/padding).
	// Default: 2 (1 for border + 1 for padding)
	ContentStartX int

	// ContentStartY is the Y offset where content starts (after top border).
	// Default: 1 (1 for border)
	ContentStartY int
}

// SelectablePane combines a viewport with text selection functionality.
// It handles mouse events for drag-to-select and clipboard operations.
type SelectablePane struct {
	viewport  viewport.Model
	selection *TextSelection
	clipboard Clipboard
	makeToast ToastFunc

	// Coordinate offsets for mouse → content position mapping
	screenXOffset int // Pane's X position on screen
	screenYOffset int // Pane's Y position on screen (where viewport starts)
	contentStartX int // X offset where content starts within pane
	contentStartY int // Y offset where content starts within pane

	// Top padding for bottom-aligned content (empty lines added at top)
	topPadding int

	dirty bool
}

// NewPane creates a new SelectablePane with the given configuration.
func NewPane(cfg PaneConfig) *SelectablePane {
	// Apply defaults
	contentStartX := cfg.ContentStartX
	if contentStartX == 0 {
		contentStartX = 2 // border + padding
	}
	contentStartY := cfg.ContentStartY
	if contentStartY == 0 {
		contentStartY = 1 // border
	}

	return &SelectablePane{
		viewport:      viewport.New(0, 0),
		selection:     New(),
		clipboard:     cfg.Clipboard,
		makeToast:     cfg.MakeToast,
		contentStartX: contentStartX,
		contentStartY: contentStartY,
		dirty:         true,
	}
}

// SetSize updates the viewport dimensions.
func (p *SelectablePane) SetSize(width, height int) {
	p.viewport.Width = width
	p.viewport.Height = height
}

// SetScreenXOffset sets the pane's X position on screen for coordinate mapping.
// This is needed when the pane is not at the left edge of the terminal.
func (p *SelectablePane) SetScreenXOffset(offset int) {
	p.screenXOffset = offset
}

// SetScreenYOffset sets the pane's Y position on screen for coordinate mapping.
// This is needed when the pane is not at the top of its container (e.g., below a tab bar).
func (p *SelectablePane) SetScreenYOffset(offset int) {
	p.screenYOffset = offset
}

// SetTopPadding sets the number of empty padding lines at the top of the viewport.
// This is used when content is bottom-aligned and doesn't fill the full viewport height.
// The padding count is subtracted from screen Y coordinates during selection.
func (p *SelectablePane) SetTopPadding(lines int) {
	p.topPadding = lines
}

// SetContent updates the viewport content and plain lines for selection.
// The styled content is displayed; plainLines are used for text extraction.
// If autoScroll is true and viewport was at bottom, it stays at bottom.
func (p *SelectablePane) SetContent(styled string, plainLines []string, autoScroll bool) {
	wasAtBottom := p.viewport.AtBottom()
	p.viewport.SetContent(styled)
	p.selection.SetPlainLines(plainLines)
	if autoScroll && wasAtBottom {
		p.viewport.GotoBottom()
	}
}

// isWithinBounds checks if the mouse coordinates are within the pane's screen area.
func (p *SelectablePane) isWithinBounds(x, y int) bool {
	if x < p.screenXOffset || x >= p.screenXOffset+p.viewport.Width {
		return false
	}
	if y < p.screenYOffset || y >= p.screenYOffset+p.viewport.Height {
		return false
	}
	return true
}

// HandleMouse processes mouse events for text selection.
// Returns a tea.Cmd if a toast should be shown (copy success/failure).
// The caller should check Dirty() to see if re-rendering is needed.
func (p *SelectablePane) HandleMouse(msg tea.MouseMsg) tea.Cmd {
	switch {
	case msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress:
		// Only start selection if click is within pane bounds
		if !p.isWithinBounds(msg.X, msg.Y) {
			return nil
		}
		// Start selection on mouse press
		contentPos := p.screenToContentPosition(msg.X, msg.Y)
		p.selection.Start(contentPos)
		p.dirty = true
		return nil

	case msg.Action == tea.MouseActionMotion && msg.Button == tea.MouseButtonLeft && p.selection.IsSelecting():
		// Update selection end while dragging (only if left button is still held)
		contentPos := p.screenToContentPosition(msg.X, msg.Y)
		if p.selection.Update(contentPos) {
			p.dirty = true
		}
		return nil

	case msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionRelease:
		if p.selection.IsSelecting() {
			// Finalize selection and copy if there's selected text
			selectedText := p.selection.Finalize()

			// Empty selection (click without drag) - clear and return immediately
			if selectedText == "" {
				p.selection.Clear()
				p.dirty = true
				return nil
			}

			// Has selected text - try to copy if clipboard available
			if p.clipboard != nil {
				if err := p.clipboard.Copy(selectedText); err != nil {
					p.selection.Clear()
					p.dirty = true
					if p.makeToast != nil {
						return p.makeToast("Failed to copy: "+err.Error(), true)
					}
					return nil
				}
				p.selection.Clear()
				p.dirty = true
				if p.makeToast != nil {
					return p.makeToast("Copied selection", false)
				}
				return nil
			}

			// No clipboard - just clear selection
			p.selection.Clear()
			p.dirty = true
		}
		return nil

	case msg.Button == tea.MouseButtonWheelUp:
		p.viewport.ScrollUp(1)
		return nil

	case msg.Button == tea.MouseButtonWheelDown:
		p.viewport.ScrollDown(1)
		return nil
	}

	return nil
}

// screenToContentPosition converts screen coordinates to content position.
func (p *SelectablePane) screenToContentPosition(screenX, screenY int) Point {
	// Convert screen coordinates to content-relative coordinates
	// screenYOffset: where the viewport starts on screen
	// contentStartY: offset within viewport for border (typically 1)
	relativeY := screenY - p.screenYOffset - p.contentStartY
	relativeX := screenX - p.screenXOffset - p.contentStartX

	// Clamp to valid ranges
	if relativeY < 0 {
		relativeY = 0
	}
	if relativeX < 0 {
		relativeX = 0
	}

	// Account for top padding when content is bottom-aligned.
	// The viewport may have empty padding lines at the top that aren't in plainLines.
	relativeY -= p.topPadding
	if relativeY < 0 {
		relativeY = 0
	}

	// Add viewport scroll offset to get the line in the full content
	contentLine := relativeY + p.viewport.YOffset

	// Get plain lines from selection
	plainLines := p.selection.PlainLines()

	// Clamp to valid line range
	maxLine := len(plainLines) - 1
	if maxLine < 0 {
		return Point{Line: 0, Col: 0}
	}
	if contentLine > maxLine {
		contentLine = maxLine
	}

	// Clamp column to line's display width (not byte length)
	// This is important for wide characters like emojis
	col := relativeX
	if contentLine < len(plainLines) {
		lineDisplayWidth := uniseg.StringWidth(plainLines[contentLine])
		if col > lineDisplayWidth {
			col = lineDisplayWidth
		}
	}

	return Point{Line: contentLine, Col: col}
}

// SelectionBounds returns the current selection bounds for rendering.
// Returns nil, nil if no selection is active.
func (p *SelectablePane) SelectionBounds() (*Point, *Point) {
	return p.selection.SelectionBounds()
}

// View returns the viewport's rendered view.
func (p *SelectablePane) View() string {
	return p.viewport.View()
}

// Dirty returns true if the pane needs re-rendering (selection changed).
func (p *SelectablePane) Dirty() bool {
	return p.dirty
}

// ClearDirty resets the dirty flag.
func (p *SelectablePane) ClearDirty() {
	p.dirty = false
}

// ScrollUp scrolls the viewport up by n lines.
func (p *SelectablePane) ScrollUp(n int) {
	p.viewport.ScrollUp(n)
}

// ScrollDown scrolls the viewport down by n lines.
func (p *SelectablePane) ScrollDown(n int) {
	p.viewport.ScrollDown(n)
}

// GotoBottom scrolls the viewport to the bottom.
func (p *SelectablePane) GotoBottom() {
	p.viewport.GotoBottom()
}

// AtBottom returns true if the viewport is scrolled to the bottom.
func (p *SelectablePane) AtBottom() bool {
	return p.viewport.AtBottom()
}

// YOffset returns the current vertical scroll offset.
func (p *SelectablePane) YOffset() int {
	return p.viewport.YOffset
}

// Width returns the viewport width.
func (p *SelectablePane) Width() int {
	return p.viewport.Width
}

// Height returns the viewport height.
func (p *SelectablePane) Height() int {
	return p.viewport.Height
}
