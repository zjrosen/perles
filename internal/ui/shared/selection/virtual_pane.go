// Package selection provides text selection functionality for TUI viewports.
// This file contains the VirtualSelectablePane component for virtual scrolling.
package selection

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/zjrosen/perles/internal/mode/shared"
	"github.com/zjrosen/perles/internal/ui/shared/chatrender"
	"github.com/zjrosen/perles/internal/ui/styles"

	tea "github.com/charmbracelet/bubbletea"
)

// VirtualSelectablePane combines virtual scrolling with text selection functionality.
// It renders only visible lines + buffer zone for O(visible) per-frame rendering instead of O(n).
// This provides a ~150x reduction in per-frame memory allocation for large content.
//
// Key design decisions:
// - Manages own scrollOffset instead of delegating to viewport.Model
// - Stores full plainLines[] for text selection (O(n) storage, one-time)
// - Selection highlighting applied as post-render overlay (not part of cache key)
// - Auto-scroll behavior: if at bottom before SetMessages, stays at bottom after
type VirtualSelectablePane struct {
	// Virtual content management
	virtualContent *chatrender.ChatVirtualContent

	// Scroll state
	scrollOffset int
	height       int
	width        int
	bufferLines  int // default: 50

	// Selection state
	selection  *TextSelection
	plainLines []string // For text extraction
	topPadding int      // For bottom-alignment when content < height

	// Coordinate mapping
	screenXOffset int
	screenYOffset int

	// Dependencies
	clipboard Clipboard
	makeToast ToastFunc

	// State tracking
	focused     bool
	wasAtBottom bool // For auto-scroll behavior
}

// DefaultBufferLines is the default number of lines to pre-render above/below visible area.
const DefaultBufferLines = 50

// NewVirtualSelectablePane creates a new VirtualSelectablePane with the given configuration.
// clipboard can be nil if copy functionality is not needed.
// makeToast can be nil if toast notifications are not needed.
func NewVirtualSelectablePane(clipboard shared.Clipboard, makeToast ToastFunc) *VirtualSelectablePane {
	return &VirtualSelectablePane{
		virtualContent: nil, // Created on first SetMessages call
		scrollOffset:   0,
		height:         0,
		width:          0,
		bufferLines:    DefaultBufferLines,
		selection:      New(),
		plainLines:     make([]string, 0),
		topPadding:     0,
		screenXOffset:  0,
		screenYOffset:  0,
		clipboard:      clipboard,
		makeToast:      makeToast,
		focused:        false,
		wasAtBottom:    true, // Start at bottom for new content
	}
}

// SetMessages updates the pane content from chat messages.
// It tracks whether currently at bottom (wasAtBottom = scrollOffset >= maxScrollOffset())
// and maintains that position after content update.
// The RenderConfig provides styling configuration (agent labels, colors).
func (p *VirtualSelectablePane) SetMessages(messages []chatrender.Message, width int, cfg chatrender.RenderConfig) {
	// Track if currently at bottom before content update
	p.wasAtBottom = p.scrollOffset >= p.maxScrollOffset()

	// Create new virtual content from messages
	p.virtualContent = chatrender.NewChatVirtualContentWithMessages(messages, width, cfg)

	// Update plainLines from virtual content for text selection
	p.plainLines = p.virtualContent.PlainLines()
	p.selection.SetPlainLines(p.plainLines)

	// Update width
	p.width = width

	// If wasAtBottom, maintain bottom position after content update
	if p.wasAtBottom {
		p.scrollOffset = p.maxScrollOffset()
	}

	// Calculate topPadding for bottom-alignment if content < height
	p.calculateTopPadding()

	// Clamp scroll offset to valid range (in case content shrunk)
	p.clampScrollOffset()
}

// AppendMessage appends a single message to the pane content.
// It delegates to virtualContent.AppendMessage() and updates plainLines.
// If wasAtBottom (currently at bottom), auto-scrolls to show the new content.
// Returns the line index where the new message starts.
func (p *VirtualSelectablePane) AppendMessage(msg chatrender.Message) int {
	// Track if currently at bottom before content update
	wasAtBottom := p.scrollOffset >= p.maxScrollOffset()

	// Handle case where virtualContent doesn't exist yet
	if p.virtualContent == nil {
		// Create a new virtual content with just this message
		p.virtualContent = chatrender.NewChatVirtualContentWithMessages(
			[]chatrender.Message{msg},
			p.width,
			chatrender.RenderConfig{}, // Use default config
		)
		p.plainLines = p.virtualContent.PlainLines()
		p.selection.SetPlainLines(p.plainLines)
		p.calculateTopPadding()
		if wasAtBottom {
			p.scrollOffset = p.maxScrollOffset()
		}
		return 0
	}

	// Append message to virtual content
	startIndex := p.virtualContent.AppendMessage(msg)

	// Update plainLines from virtual content
	p.plainLines = p.virtualContent.PlainLines()
	p.selection.SetPlainLines(p.plainLines)

	// Recalculate topPadding for bottom-alignment
	p.calculateTopPadding()

	// If wasAtBottom, auto-scroll to show new content
	if wasAtBottom {
		p.scrollOffset = p.maxScrollOffset()
	}

	return startIndex
}

// SetSize updates the pane dimensions.
// If width changed, it propagates to virtual content (triggers cache clear and rebuild).
// Recalculates topPadding for bottom-alignment and clamps scroll offset.
func (p *VirtualSelectablePane) SetSize(width, height int) {
	oldWidth := p.width

	p.width = width
	p.height = height

	// If width changed and we have virtual content, update it
	// This triggers cache clear and line rebuild for new word wrapping
	if p.virtualContent != nil && width != oldWidth {
		p.virtualContent.SetWidth(width)
		// Update plainLines after rebuild
		p.plainLines = p.virtualContent.PlainLines()
		p.selection.SetPlainLines(p.plainLines)
	}

	// Recalculate topPadding for bottom-alignment
	p.calculateTopPadding()

	// Clamp scroll offset to valid range
	p.clampScrollOffset()
}

// maxScrollOffset returns the maximum valid scroll offset.
// This is max(0, totalLines - height) to ensure content fills the viewport.
func (p *VirtualSelectablePane) maxScrollOffset() int {
	if p.virtualContent == nil {
		return 0
	}
	totalLines := p.virtualContent.TotalLines()
	if totalLines <= p.height {
		return 0
	}
	return totalLines - p.height
}

// clampScrollOffset ensures scrollOffset stays within valid range [0, maxScrollOffset()].
func (p *VirtualSelectablePane) clampScrollOffset() {
	maxOffset := p.maxScrollOffset()
	if p.scrollOffset < 0 {
		p.scrollOffset = 0
	}
	if p.scrollOffset > maxOffset {
		p.scrollOffset = maxOffset
	}
}

// calculateTopPadding sets topPadding for bottom-alignment when content < height.
// This is used in coordinate mapping and View rendering.
func (p *VirtualSelectablePane) calculateTopPadding() {
	if p.virtualContent == nil {
		p.topPadding = p.height
		return
	}
	totalLines := p.virtualContent.TotalLines()
	if totalLines < p.height {
		p.topPadding = p.height - totalLines
	} else {
		p.topPadding = 0
	}
}

// TotalLines returns the total number of lines in the content.
func (p *VirtualSelectablePane) TotalLines() int {
	if p.virtualContent == nil {
		return 0
	}
	return p.virtualContent.TotalLines()
}

// ScrollOffset returns the current scroll offset.
func (p *VirtualSelectablePane) ScrollOffset() int {
	return p.scrollOffset
}

// SetScrollOffset sets the scroll offset with clamping to valid range.
func (p *VirtualSelectablePane) SetScrollOffset(offset int) {
	p.scrollOffset = offset
	p.clampScrollOffset()
}

// AtBottom returns true if scrolled to the bottom.
func (p *VirtualSelectablePane) AtBottom() bool {
	return p.scrollOffset >= p.maxScrollOffset()
}

// AtTop returns true if scrolled to the top.
func (p *VirtualSelectablePane) AtTop() bool {
	return p.scrollOffset == 0
}

// Width returns the pane width.
func (p *VirtualSelectablePane) Width() int {
	return p.width
}

// Height returns the pane height.
func (p *VirtualSelectablePane) Height() int {
	return p.height
}

// TopPadding returns the number of empty padding lines at the top.
func (p *VirtualSelectablePane) TopPadding() int {
	return p.topPadding
}

// WasAtBottom returns whether the pane was at bottom before the last SetMessages call.
// This is useful for testing auto-scroll behavior.
func (p *VirtualSelectablePane) WasAtBottom() bool {
	return p.wasAtBottom
}

// VirtualContent returns the underlying ChatVirtualContent (for testing).
func (p *VirtualSelectablePane) VirtualContent() *chatrender.ChatVirtualContent {
	return p.virtualContent
}

// PlainLines returns the plain text lines for selection.
func (p *VirtualSelectablePane) PlainLines() []string {
	return p.plainLines
}

// Selection returns the TextSelection for this pane.
func (p *VirtualSelectablePane) Selection() *TextSelection {
	return p.selection
}

// SetFocused sets the focus state of the pane.
func (p *VirtualSelectablePane) SetFocused(focused bool) {
	p.focused = focused
}

// Focused returns true if the pane is focused.
func (p *VirtualSelectablePane) Focused() bool {
	return p.focused
}

// SetScreenPosition sets the pane's screen position for coordinate mapping.
func (p *VirtualSelectablePane) SetScreenPosition(x, y int) {
	p.screenXOffset = x
	p.screenYOffset = y
}

// BufferLines returns the number of buffer lines for pre-warming.
func (p *VirtualSelectablePane) BufferLines() int {
	return p.bufferLines
}

// SetBufferLines sets the number of buffer lines for pre-warming.
func (p *VirtualSelectablePane) SetBufferLines(lines int) {
	p.bufferLines = lines
}

// selectionBgStyle is the background highlight for selected text.
// Uses the global SelectionBackgroundColor from the styles package for consistency.
var selectionBgStyle = lipgloss.NewStyle().Background(styles.SelectionBackgroundColor)

// emptyStateStyle is used to render the empty state placeholder.
var emptyStateStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#999999", Dark: "#666666"})

// View renders only the visible lines, applying selection highlighting as a post-render overlay.
// Complexity: O(visible), NOT O(totalLines).
//
// The method:
// 1. Handles empty state by returning appropriate placeholder content
// 2. Calculates visible range: [scrollOffset, scrollOffset + height]
// 3. Handles bottom-alignment with topPadding for short content
// 4. Renders visible lines and applies selection overlay
// 5. Joins lines with newlines
func (p *VirtualSelectablePane) View() string {
	// Handle empty or nil content
	if p.virtualContent == nil || p.virtualContent.TotalLines() == 0 {
		return p.renderEmptyState()
	}

	totalLines := p.virtualContent.TotalLines()

	// Get selection bounds for overlay (nil if no selection)
	var selStart, selEnd *Point
	if p.selection != nil {
		selStart, selEnd = p.selection.SelectionBounds()
	}

	// Build output by iterating only visible lines (O(visible))
	var lines []string

	// Handle bottom-alignment: if topPadding > 0, prepend empty lines
	if p.topPadding > 0 {
		for i := 0; i < p.topPadding; i++ {
			lines = append(lines, "")
		}
	}

	// Calculate visible range
	visibleEnd := min(p.scrollOffset+p.height-p.topPadding, totalLines)

	// Render only visible lines
	for i := p.scrollOffset; i < visibleEnd; i++ {
		line := p.virtualContent.RenderLine(i)
		// Apply selection overlay as post-render operation (not cached)
		line = p.applySelectionOverlay(i, line, selStart, selEnd)
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// applySelectionOverlay applies selection highlighting to a rendered line.
// This is a post-render operation that doesn't affect the cache.
//
// Key design: Selection uses plain text, not styled text, for accurate column positioning.
// The plain text is used to calculate selection bounds, then highlighting is applied
// using the plain text segments, resulting in consistent selection behavior.
func (p *VirtualSelectablePane) applySelectionOverlay(lineIndex int, rendered string, selStart, selEnd *Point) string {
	// If no selection active, return rendered as-is
	if selStart == nil || selEnd == nil {
		return rendered
	}

	// If lineIndex outside selection range, return rendered as-is
	if lineIndex < selStart.Line || lineIndex > selEnd.Line {
		return rendered
	}

	// Get plainLine for accurate column positioning
	if lineIndex >= len(p.plainLines) {
		return rendered
	}
	plainLine := p.plainLines[lineIndex]

	// Calculate selection start/end columns for this line
	startCol := 0
	endCol := stringDisplayWidth(plainLine)

	if lineIndex == selStart.Line {
		startCol = selStart.Col
	}
	if lineIndex == selEnd.Line {
		endCol = selEnd.Col
	}

	// Clamp to valid display width range
	lineWidth := stringDisplayWidth(plainLine)
	if startCol > lineWidth {
		startCol = lineWidth
	}
	if endCol > lineWidth {
		endCol = lineWidth
	}
	if startCol >= endCol {
		return rendered
	}

	// Build: before + selectionBgStyle.Render(selected) + after
	// Using plain text for accurate positioning, which matches the existing pattern
	// in renderLineWithSelection in coordinator_panel.go
	before := sliceToDisplayCol(plainLine, startCol)
	selected := sliceByDisplayCols(plainLine, startCol, endCol)
	after := sliceFromDisplayCol(plainLine, endCol)

	return before + selectionBgStyle.Render(selected) + after
}

// renderEmptyState renders appropriate placeholder for empty content.
// This is shown when there are no messages to display.
func (p *VirtualSelectablePane) renderEmptyState() string {
	// Build height-worth of empty lines for consistent sizing
	var lines []string
	for i := 0; i < p.height; i++ {
		if i == p.height/2 {
			// Show centered placeholder text on middle line
			placeholder := emptyStateStyle.Render("No messages")
			// Center the placeholder if we have width
			if p.width > 0 && stringDisplayWidth(placeholder) < p.width {
				padding := (p.width - stringDisplayWidth(placeholder)) / 2
				placeholder = strings.Repeat(" ", padding) + placeholder
			}
			lines = append(lines, placeholder)
		} else {
			lines = append(lines, "")
		}
	}
	return strings.Join(lines, "\n")
}

// Note: stringDisplayWidth is defined in selection.go and handles
// display width for wide characters like emojis.

// Default content offsets for coordinate mapping.
// These match the default values in SelectablePane for consistent behavior.
const (
	// DefaultContentStartX is the X offset where content starts (after left border/padding).
	DefaultContentStartX = 2 // 1 for border + 1 for padding

	// DefaultContentStartY is the Y offset where content starts (after top border).
	DefaultContentStartY = 1 // 1 for border
)

// screenToContentPosition converts screen coordinates to content position.
// This is the key coordinate mapping function that uses scrollOffset instead of viewport.YOffset.
//
// The mapping accounts for:
// - Screen position offsets (screenXOffset, screenYOffset)
// - Content start offsets (border + padding)
// - Top padding for bottom-alignment when content < height
// - Scroll offset for virtual scrolling
// - Clamping to valid content bounds
func (p *VirtualSelectablePane) screenToContentPosition(screenX, screenY int) Point {
	// Convert screen coordinates to content-relative coordinates
	// screenYOffset: where the pane starts on screen
	// DefaultContentStartY: offset within pane for border (typically 1)
	relativeY := screenY - p.screenYOffset - DefaultContentStartY
	relativeX := screenX - p.screenXOffset - DefaultContentStartX

	// Clamp negative coordinates to 0
	if relativeY < 0 {
		relativeY = 0
	}
	if relativeX < 0 {
		relativeX = 0
	}

	// Account for top padding when content is bottom-aligned.
	// The pane may have empty padding lines at the top that aren't in plainLines.
	relativeY -= p.topPadding
	if relativeY < 0 {
		relativeY = 0
	}

	// Add scroll offset to get the line in the full content.
	// Key difference from SelectablePane: uses scrollOffset instead of viewport.YOffset.
	contentLine := relativeY + p.scrollOffset

	// Clamp to valid line range
	maxLine := len(p.plainLines) - 1
	if maxLine < 0 {
		return Point{Line: 0, Col: 0}
	}
	if contentLine > maxLine {
		contentLine = maxLine
	}

	// Clamp column to line's display width (not byte length)
	// This is important for wide characters like emojis
	col := relativeX
	if contentLine < len(p.plainLines) {
		lineDisplayWidth := stringDisplayWidth(p.plainLines[contentLine])
		if col > lineDisplayWidth {
			col = lineDisplayWidth
		}
	}

	return Point{Line: contentLine, Col: col}
}

// isWithinBounds checks if the mouse coordinates are within the pane's screen area.
func (p *VirtualSelectablePane) isWithinBounds(x, y int) bool {
	if x < p.screenXOffset || x >= p.screenXOffset+p.width {
		return false
	}
	if y < p.screenYOffset || y >= p.screenYOffset+p.height {
		return false
	}
	return true
}

// HandleMouse processes mouse events for text selection and scrolling.
// Returns a tea.Cmd if a toast should be shown (copy success/failure).
//
// Mouse event handling:
// - Left press: starts selection if within pane bounds
// - Motion with left button: continues selection (drag)
// - Left release: ends selection, copies to clipboard if selection exists
// - Wheel up: scrolls up by 3 lines
// - Wheel down: scrolls down by 3 lines
func (p *VirtualSelectablePane) HandleMouse(msg tea.MouseMsg) tea.Cmd {
	switch {
	case msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress:
		// Only start selection if click is within pane bounds
		if !p.isWithinBounds(msg.X, msg.Y) {
			return nil
		}
		// Start selection on mouse press
		contentPos := p.screenToContentPosition(msg.X, msg.Y)
		p.selection.Start(contentPos)
		return nil

	case msg.Action == tea.MouseActionMotion && msg.Button == tea.MouseButtonLeft && p.selection.IsSelecting():
		// Update selection end while dragging (only if left button is still held)
		contentPos := p.screenToContentPosition(msg.X, msg.Y)
		p.selection.Update(contentPos)
		return nil

	case msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionRelease:
		return p.handleMouseRelease()

	case msg.Button == tea.MouseButtonWheelUp:
		p.ScrollUp(3)
		return nil

	case msg.Button == tea.MouseButtonWheelDown:
		p.ScrollDown(3)
		return nil
	}

	return nil
}

// handleMouseRelease handles mouse button release - finalizes selection and copies if applicable.
func (p *VirtualSelectablePane) handleMouseRelease() tea.Cmd {
	if !p.selection.IsSelecting() {
		return nil
	}

	// Finalize selection and extract text using full plainLines
	selectedText := p.selection.Finalize()

	// Empty selection (click without drag) - clear and return immediately
	if selectedText == "" {
		p.selection.Clear()
		return nil
	}

	// Has selected text - try to copy if clipboard available
	if p.clipboard != nil {
		if err := p.clipboard.Copy(selectedText); err != nil {
			p.selection.Clear()
			if p.makeToast != nil {
				return p.makeToast("Failed to copy: "+err.Error(), true)
			}
			return nil
		}
		p.selection.Clear()
		if p.makeToast != nil {
			return p.makeToast("Copied selection", false)
		}
		return nil
	}

	// No clipboard - just clear selection
	p.selection.Clear()
	return nil
}

// ScrollUp scrolls the viewport up by n lines.
// Triggers prewarmCache() for smooth scrolling.
func (p *VirtualSelectablePane) ScrollUp(n int) {
	p.scrollOffset -= n
	p.clampScrollOffset()
	p.prewarmCache()
}

// ScrollDown scrolls the viewport down by n lines.
// Triggers prewarmCache() for smooth scrolling.
func (p *VirtualSelectablePane) ScrollDown(n int) {
	p.scrollOffset += n
	p.clampScrollOffset()
	p.prewarmCache()
}

// ScrollToTop scrolls to the top of the content.
func (p *VirtualSelectablePane) ScrollToTop() {
	p.scrollOffset = 0
}

// ScrollToBottom scrolls to the bottom of the content.
func (p *VirtualSelectablePane) ScrollToBottom() {
	p.scrollOffset = p.maxScrollOffset()
}

// prewarmCache pre-renders lines in the buffer zone above and below the visible area.
// This populates the cache for smooth scrolling - when the user scrolls, the lines
// are already cached and render quickly.
//
// Buffer range: [scrollOffset - bufferLines, scrollOffset + height + bufferLines]
// Each line in the buffer range is rendered via virtualContent.RenderLine(i).
func (p *VirtualSelectablePane) prewarmCache() {
	if p.virtualContent == nil {
		return
	}

	totalLines := p.virtualContent.TotalLines()
	if totalLines == 0 {
		return
	}

	// Calculate buffer zone above visible area
	bufferStart := max(p.scrollOffset-p.bufferLines, 0)

	// Pre-warm lines above visible area
	for i := bufferStart; i < p.scrollOffset; i++ {
		_ = p.virtualContent.RenderLine(i)
	}

	// Calculate buffer zone below visible area
	visibleEnd := min(p.scrollOffset+p.height, totalLines)
	bufferEnd := min(visibleEnd+p.bufferLines, totalLines)

	// Pre-warm lines below visible area
	for i := visibleEnd; i < bufferEnd; i++ {
		_ = p.virtualContent.RenderLine(i)
	}
}

// SelectionBounds returns the current selection bounds for rendering.
// Returns nil, nil if no selection is active.
func (p *VirtualSelectablePane) SelectionBounds() (*Point, *Point) {
	if p.selection == nil {
		return nil, nil
	}
	return p.selection.SelectionBounds()
}

// HasSelection returns true if there is an active text selection.
func (p *VirtualSelectablePane) HasSelection() bool {
	if p.selection == nil {
		return false
	}
	return p.selection.HasSelection()
}

// ClearSelection clears the current selection.
func (p *VirtualSelectablePane) ClearSelection() {
	if p.selection != nil {
		p.selection.Clear()
	}
}

// GetSelectedText returns the currently selected text.
func (p *VirtualSelectablePane) GetSelectedText() string {
	if p.selection == nil {
		return ""
	}
	return p.selection.GetSelectedText()
}
