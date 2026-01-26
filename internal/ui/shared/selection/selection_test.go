package selection

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	s := New()
	require.NotNil(t, s)
	assert.False(t, s.IsSelecting())
	assert.False(t, s.HasSelection())
}

func TestSelection_StartAndUpdate(t *testing.T) {
	s := New()

	s.Start(Point{Line: 0, Col: 5})
	assert.True(t, s.IsSelecting())
	assert.False(t, s.HasSelection()) // start == end initially

	changed := s.Update(Point{Line: 0, Col: 10})
	assert.True(t, changed)
	assert.True(t, s.HasSelection())

	// Update to same position should return false
	changed = s.Update(Point{Line: 0, Col: 10})
	assert.False(t, changed)
}

func TestSelection_Finalize(t *testing.T) {
	s := New()
	s.SetPlainLines([]string{"Hello World"})

	s.Start(Point{Line: 0, Col: 0})
	s.Update(Point{Line: 0, Col: 5})

	text := s.Finalize()
	assert.Equal(t, "Hello", text)
	assert.False(t, s.IsSelecting())
}

func TestSelection_GetSelectedText_SingleLine(t *testing.T) {
	s := New()
	s.SetPlainLines([]string{"Hello World"})

	s.Start(Point{Line: 0, Col: 0})
	s.Update(Point{Line: 0, Col: 5})

	text := s.GetSelectedText()
	assert.Equal(t, "Hello", text)
}

func TestSelection_GetSelectedText_MultiLine(t *testing.T) {
	s := New()
	s.SetPlainLines([]string{
		"First line",
		"Second line",
		"Third line",
	})

	s.Start(Point{Line: 0, Col: 6})
	s.Update(Point{Line: 2, Col: 5})

	text := s.GetSelectedText()
	assert.Equal(t, "line\nSecond line\nThird", text)
}

func TestSelection_GetSelectedText_ReverseSelection(t *testing.T) {
	s := New()
	s.SetPlainLines([]string{"Hello World"})

	// Start at end, drag to beginning
	s.Start(Point{Line: 0, Col: 11})
	s.Update(Point{Line: 0, Col: 6})

	text := s.GetSelectedText()
	assert.Equal(t, "World", text)
}

func TestSelection_Normalized(t *testing.T) {
	s := New()

	// Normal order
	s.Start(Point{Line: 0, Col: 0})
	s.Update(Point{Line: 1, Col: 5})

	start, end := s.Normalized()
	assert.Equal(t, Point{Line: 0, Col: 0}, start)
	assert.Equal(t, Point{Line: 1, Col: 5}, end)

	// Reverse order
	s.Start(Point{Line: 1, Col: 5})
	s.Update(Point{Line: 0, Col: 0})

	start, end = s.Normalized()
	assert.Equal(t, Point{Line: 0, Col: 0}, start)
	assert.Equal(t, Point{Line: 1, Col: 5}, end)
}

func TestSelection_Clear(t *testing.T) {
	s := New()
	s.SetPlainLines([]string{"Hello"})
	s.Start(Point{Line: 0, Col: 0})
	s.Update(Point{Line: 0, Col: 5})

	assert.True(t, s.HasSelection())

	s.Clear()
	assert.False(t, s.HasSelection())
	assert.False(t, s.IsSelecting())
}

func TestSelection_SelectionBounds(t *testing.T) {
	s := New()

	// No selection
	start, end := s.SelectionBounds()
	assert.Nil(t, start)
	assert.Nil(t, end)

	// With selection
	s.Start(Point{Line: 0, Col: 0})
	s.Update(Point{Line: 0, Col: 5})

	start, end = s.SelectionBounds()
	require.NotNil(t, start)
	require.NotNil(t, end)
	assert.Equal(t, Point{Line: 0, Col: 0}, *start)
	assert.Equal(t, Point{Line: 0, Col: 5}, *end)
}

func TestSelection_EmptyPlainLines(t *testing.T) {
	s := New()
	// No plain lines set

	s.Start(Point{Line: 0, Col: 0})
	s.Update(Point{Line: 0, Col: 5})

	text := s.GetSelectedText()
	assert.Equal(t, "", text)
}

func TestSelection_ClampToValidRange(t *testing.T) {
	s := New()
	s.SetPlainLines([]string{"Short"})

	// Selection extends beyond line length
	s.Start(Point{Line: 0, Col: 0})
	s.Update(Point{Line: 0, Col: 100}) // Way beyond "Short"

	text := s.GetSelectedText()
	assert.Equal(t, "Short", text)
}

func TestSelection_Dirty(t *testing.T) {
	s := New()
	assert.False(t, s.Dirty())

	s.Start(Point{Line: 0, Col: 0})
	assert.True(t, s.Dirty())

	s.ClearDirty()
	assert.False(t, s.Dirty())

	s.Update(Point{Line: 0, Col: 5})
	assert.True(t, s.Dirty())
}

func TestSelection_GetSelectedText_WithEmoji(t *testing.T) {
	s := New()

	// "hi😀world" - 'h'=col0, 'i'=col1, '😀'=col2-3, 'w'=col4, 'o'=col5, ...
	s.SetPlainLines([]string{"hi😀world"})

	// Select "😀w" - display columns 2-5 (emoji at col 2-3, 'w' at col 4)
	s.Start(Point{Line: 0, Col: 2})
	s.Update(Point{Line: 0, Col: 5})

	text := s.GetSelectedText()
	assert.Equal(t, "😀w", text)
}

func TestSelection_GetSelectedText_WithMultipleEmoji(t *testing.T) {
	s := New()

	// "a🎉b🔥c" - 'a'=1col, '🎉'=2cols, 'b'=1col, '🔥'=2cols, 'c'=1col
	// Columns: a=0, 🎉=1-2, b=3, 🔥=4-5, c=6
	s.SetPlainLines([]string{"a🎉b🔥c"})

	// Select "🎉b🔥" - columns 1-6
	s.Start(Point{Line: 0, Col: 1})
	s.Update(Point{Line: 0, Col: 6})

	text := s.GetSelectedText()
	assert.Equal(t, "🎉b🔥", text)
}

func TestSelection_GetSelectedText_EmojiAtStart(t *testing.T) {
	s := New()

	// "😀hello" - emoji at start
	s.SetPlainLines([]string{"😀hello"})

	// Select just the emoji (columns 0-2)
	s.Start(Point{Line: 0, Col: 0})
	s.Update(Point{Line: 0, Col: 2})

	text := s.GetSelectedText()
	assert.Equal(t, "😀", text)
}

func TestSelection_GetSelectedText_TextAfterEmoji(t *testing.T) {
	s := New()

	// "😀hello" - select "hello" after emoji
	s.SetPlainLines([]string{"😀hello"})

	// Select "hello" (columns 2-7)
	s.Start(Point{Line: 0, Col: 2})
	s.Update(Point{Line: 0, Col: 7})

	text := s.GetSelectedText()
	assert.Equal(t, "hello", text)
}
