package selection

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockClipboard is a test clipboard that records copied text.
type mockClipboard struct {
	lastCopied string
	err        error
}

func (m *mockClipboard) Copy(text string) error {
	if m.err != nil {
		return m.err
	}
	m.lastCopied = text
	return nil
}

// testToast records toast calls for testing.
type testToast struct {
	message string
	isError bool
	called  bool
}

func (t *testToast) MakeToast(message string, isError bool) tea.Cmd {
	t.message = message
	t.isError = isError
	t.called = true
	return func() tea.Msg { return nil }
}

func TestNewPane(t *testing.T) {
	p := NewPane(PaneConfig{})
	require.NotNil(t, p)
	assert.NotNil(t, p.selection)
	assert.Equal(t, 2, p.contentStartX, "default contentStartX")
	assert.Equal(t, 1, p.contentStartY, "default contentStartY")
	assert.True(t, p.Dirty(), "starts dirty")
}

func TestNewPane_CustomOffsets(t *testing.T) {
	p := NewPane(PaneConfig{
		ContentStartX: 5,
		ContentStartY: 3,
	})
	assert.Equal(t, 5, p.contentStartX)
	assert.Equal(t, 3, p.contentStartY)
}

func TestPane_SetSize(t *testing.T) {
	p := NewPane(PaneConfig{})
	p.SetSize(80, 24)
	assert.Equal(t, 80, p.Width())
	assert.Equal(t, 24, p.Height())
}

func TestPane_SetScreenXOffset(t *testing.T) {
	p := NewPane(PaneConfig{})
	p.SetScreenXOffset(100)
	assert.Equal(t, 100, p.screenXOffset)
}

func TestPane_SetContent(t *testing.T) {
	p := NewPane(PaneConfig{})
	p.SetSize(80, 10)

	plainLines := []string{"Line 1", "Line 2", "Line 3"}
	p.SetContent("styled content", plainLines, false)

	// Verify plain lines are stored for selection
	start, end := p.SelectionBounds()
	assert.Nil(t, start)
	assert.Nil(t, end)
}

func TestPane_HandleMouse_SelectionFlow(t *testing.T) {
	clipboard := &mockClipboard{}
	toast := &testToast{}

	p := NewPane(PaneConfig{
		Clipboard: clipboard,
		MakeToast: toast.MakeToast,
	})
	p.SetSize(80, 10)
	p.SetContent("Hello World", []string{"Hello World"}, false)
	p.ClearDirty()

	// Press to start selection
	cmd := p.HandleMouse(tea.MouseMsg{
		X:      2, // contentStartX
		Y:      1, // contentStartY
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	assert.Nil(t, cmd)
	assert.True(t, p.Dirty())
	p.ClearDirty()

	// Drag to extend selection
	cmd = p.HandleMouse(tea.MouseMsg{
		X:      7, // 5 characters selected
		Y:      1,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionMotion,
	})
	assert.Nil(t, cmd)
	assert.True(t, p.Dirty())

	// Verify selection bounds are set
	start, end := p.SelectionBounds()
	require.NotNil(t, start)
	require.NotNil(t, end)

	p.ClearDirty()

	// Release to finalize and copy
	cmd = p.HandleMouse(tea.MouseMsg{
		X:      7,
		Y:      1,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionRelease,
	})
	require.NotNil(t, cmd)
	assert.True(t, toast.called)
	assert.Equal(t, "Copied selection", toast.message)
	assert.False(t, toast.isError)
	assert.Equal(t, "Hello", clipboard.lastCopied)
	assert.True(t, p.Dirty())

	// Selection should be cleared after copy
	start, end = p.SelectionBounds()
	assert.Nil(t, start)
	assert.Nil(t, end)
}

func TestPane_HandleMouse_CopyError(t *testing.T) {
	clipboard := &mockClipboard{err: errors.New("clipboard error")}
	toast := &testToast{}

	p := NewPane(PaneConfig{
		Clipboard: clipboard,
		MakeToast: toast.MakeToast,
	})
	p.SetSize(80, 10)
	p.SetContent("Hello World", []string{"Hello World"}, false)

	// Select some text
	p.HandleMouse(tea.MouseMsg{X: 2, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	p.HandleMouse(tea.MouseMsg{X: 7, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})

	// Release should trigger error toast
	cmd := p.HandleMouse(tea.MouseMsg{X: 7, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
	require.NotNil(t, cmd)
	assert.True(t, toast.called)
	assert.Contains(t, toast.message, "Failed to copy")
	assert.True(t, toast.isError)
}

func TestPane_HandleMouse_NoClipboard(t *testing.T) {
	p := NewPane(PaneConfig{}) // No clipboard
	p.SetSize(80, 10)
	p.SetContent("Hello World", []string{"Hello World"}, false)

	// Select some text
	p.HandleMouse(tea.MouseMsg{X: 2, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	p.HandleMouse(tea.MouseMsg{X: 7, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})

	// Release should not crash, just clear selection
	cmd := p.HandleMouse(tea.MouseMsg{X: 7, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
	assert.Nil(t, cmd)
}

func TestPane_HandleMouse_EmptySelection(t *testing.T) {
	clipboard := &mockClipboard{}
	toast := &testToast{}

	p := NewPane(PaneConfig{
		Clipboard: clipboard,
		MakeToast: toast.MakeToast,
	})
	p.SetSize(80, 10)
	p.SetContent("Hello World", []string{"Hello World"}, false)

	// Click without drag (empty selection)
	p.HandleMouse(tea.MouseMsg{X: 5, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	cmd := p.HandleMouse(tea.MouseMsg{X: 5, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})

	// No copy should happen
	assert.Nil(t, cmd)
	assert.False(t, toast.called)
	assert.Empty(t, clipboard.lastCopied)

	// Selection should be fully cleared (not just finalized)
	start, end := p.SelectionBounds()
	assert.Nil(t, start, "selection start should be nil after click without drag")
	assert.Nil(t, end, "selection end should be nil after click without drag")
}

func TestPane_HandleMouse_Scroll(t *testing.T) {
	p := NewPane(PaneConfig{})
	p.SetSize(80, 5)

	// Set content taller than viewport
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = "Line"
	}
	p.SetContent("Line\nLine\nLine\nLine\nLine\nLine\nLine\nLine\nLine\nLine\nLine\nLine\nLine\nLine\nLine\nLine\nLine\nLine\nLine\nLine", lines, false)

	assert.Equal(t, 0, p.YOffset())

	// Scroll down
	p.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	assert.Greater(t, p.YOffset(), 0)

	// Scroll up
	p.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelUp})
	assert.Equal(t, 0, p.YOffset())
}

func TestPane_ScreenToContentPosition(t *testing.T) {
	p := NewPane(PaneConfig{
		ContentStartX: 2,
		ContentStartY: 1,
	})
	p.SetSize(80, 10)
	p.SetContent("Line 1\nLine 2\nLine 3", []string{"Line 1", "Line 2", "Line 3"}, false)

	// Click at content start
	pos := p.screenToContentPosition(2, 1)
	assert.Equal(t, Point{Line: 0, Col: 0}, pos)

	// Click further right
	pos = p.screenToContentPosition(7, 1)
	assert.Equal(t, Point{Line: 0, Col: 5}, pos)

	// Click on second line
	pos = p.screenToContentPosition(4, 2)
	assert.Equal(t, Point{Line: 1, Col: 2}, pos)
}

func TestPane_ScreenToContentPosition_WithXOffset(t *testing.T) {
	p := NewPane(PaneConfig{
		ContentStartX: 2,
		ContentStartY: 1,
	})
	p.SetSize(80, 10)
	p.SetScreenXOffset(100) // Panel is 100 pixels from left edge
	p.SetContent("Hello", []string{"Hello"}, false)

	// Screen X=102 should map to content column 0
	pos := p.screenToContentPosition(102, 1)
	assert.Equal(t, Point{Line: 0, Col: 0}, pos)

	// Screen X=107 should map to content column 5
	pos = p.screenToContentPosition(107, 1)
	assert.Equal(t, Point{Line: 0, Col: 5}, pos)
}

func TestPane_ScreenToContentPosition_WithScroll(t *testing.T) {
	p := NewPane(PaneConfig{})
	p.SetSize(80, 5)

	// Create content with 20 lines (must have newlines for viewport to be scrollable)
	var content string
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = "Line"
		if i > 0 {
			content += "\n"
		}
		content += "Line"
	}
	p.SetContent(content, lines, false)

	// Scroll down - go to bottom first to ensure content is recognized as scrollable
	p.GotoBottom()
	p.ScrollUp(10) // Scroll back up to somewhere in the middle

	// Verify we're scrolled
	offset := p.YOffset()
	require.Greater(t, offset, 0, "should be scrolled down")

	// Screen Y=1 should now map to content line = offset (0 + scroll offset)
	pos := p.screenToContentPosition(2, 1)
	assert.Equal(t, offset, pos.Line)
}

func TestPane_Dirty(t *testing.T) {
	p := NewPane(PaneConfig{})
	p.SetSize(80, 10)
	p.SetContent("Hello World", []string{"Hello World"}, false)
	assert.True(t, p.Dirty(), "starts dirty")

	p.ClearDirty()
	assert.False(t, p.Dirty())

	// Mouse press within bounds should mark dirty
	p.HandleMouse(tea.MouseMsg{X: 5, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	assert.True(t, p.Dirty())
}

func TestPane_HandleMouse_IgnoresClicksOutsideBounds(t *testing.T) {
	p := NewPane(PaneConfig{})
	p.SetSize(80, 10)
	p.SetScreenXOffset(100) // Panel starts at X=100
	p.SetScreenYOffset(50)  // Panel starts at Y=50
	p.SetContent("Hello World", []string{"Hello World"}, false)
	p.ClearDirty()

	// Click outside X bounds (before X offset)
	p.HandleMouse(tea.MouseMsg{X: 50, Y: 55, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	assert.False(t, p.Dirty(), "click before X offset should be ignored")

	// Click outside X bounds (after X offset + width)
	p.HandleMouse(tea.MouseMsg{X: 200, Y: 55, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	assert.False(t, p.Dirty(), "click after X bounds should be ignored")

	// Click outside Y bounds (before Y offset)
	p.HandleMouse(tea.MouseMsg{X: 105, Y: 40, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	assert.False(t, p.Dirty(), "click before Y offset should be ignored")

	// Click outside Y bounds (after Y offset + height)
	p.HandleMouse(tea.MouseMsg{X: 105, Y: 70, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	assert.False(t, p.Dirty(), "click after Y bounds should be ignored")

	// Click inside bounds
	p.HandleMouse(tea.MouseMsg{X: 105, Y: 55, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	assert.True(t, p.Dirty(), "click inside bounds should be handled")
}

func TestPane_ScrollMethods(t *testing.T) {
	p := NewPane(PaneConfig{})
	p.SetSize(80, 5)
	p.SetContent("1\n2\n3\n4\n5\n6\n7\n8\n9\n10", []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}, false)

	assert.Equal(t, 0, p.YOffset())
	assert.False(t, p.AtBottom())

	p.GotoBottom()
	assert.True(t, p.AtBottom())

	p.ScrollUp(5)
	assert.False(t, p.AtBottom())
}

func TestPane_SetContent_AutoScroll(t *testing.T) {
	p := NewPane(PaneConfig{})
	p.SetSize(80, 5)

	// Initial content
	p.SetContent("1\n2\n3\n4\n5", []string{"1", "2", "3", "4", "5"}, false)
	p.GotoBottom()
	assert.True(t, p.AtBottom())

	// Add more content with autoScroll=true
	p.SetContent("1\n2\n3\n4\n5\n6\n7\n8\n9\n10", []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}, true)
	assert.True(t, p.AtBottom(), "should stay at bottom with autoScroll")
}

func TestPane_TopPadding_CoordinateMapping(t *testing.T) {
	p := NewPane(PaneConfig{
		ContentStartX: 2,
		ContentStartY: 1,
	})
	p.SetSize(80, 10)

	// Content has only 3 lines, but viewport is 10 lines tall
	// Visual layout with bottom-alignment:
	//   Line 0-6: empty padding (7 lines)
	//   Line 7: "Line 1"
	//   Line 8: "Line 2"
	//   Line 9: "Line 3"
	p.SetTopPadding(7) // 10 - 3 = 7 padding lines
	p.SetContent("Line 1\nLine 2\nLine 3", []string{"Line 1", "Line 2", "Line 3"}, false)

	// Click on visual row 7 (where "Line 1" appears after padding)
	// Screen Y = screenYOffset + contentStartY + 7 = 0 + 1 + 7 = 8
	pos := p.screenToContentPosition(2, 8) // contentStartX=2, screenY=8
	assert.Equal(t, Point{Line: 0, Col: 0}, pos, "should map to first content line")

	// Click on visual row 9 (where "Line 3" appears)
	pos = p.screenToContentPosition(2, 10)
	assert.Equal(t, Point{Line: 2, Col: 0}, pos, "should map to third content line")

	// Click in padding area (visual row 3)
	pos = p.screenToContentPosition(2, 4)
	assert.Equal(t, Point{Line: 0, Col: 0}, pos, "click in padding should clamp to first line")
}

func TestPane_TopPadding_SelectionInPaddedContent(t *testing.T) {
	clipboard := &mockClipboard{}
	toast := &testToast{}

	p := NewPane(PaneConfig{
		Clipboard:     clipboard,
		MakeToast:     toast.MakeToast,
		ContentStartX: 2,
		ContentStartY: 1,
	})
	p.SetSize(80, 10)

	// Content has 2 lines with 8 lines of top padding
	p.SetTopPadding(8)
	p.SetContent("Hello World\nSecond Line", []string{"Hello World", "Second Line"}, false)
	p.ClearDirty()

	// Select "Hello" on the first content line (visual row 8 after padding)
	// Screen Y = 0 + 1 + 8 = 9
	p.HandleMouse(tea.MouseMsg{X: 2, Y: 9, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	p.HandleMouse(tea.MouseMsg{X: 7, Y: 9, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
	p.HandleMouse(tea.MouseMsg{X: 7, Y: 9, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})

	assert.Equal(t, "Hello", clipboard.lastCopied, "should copy text from padded content area")
}
