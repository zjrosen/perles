package picker

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testOptions() []Option {
	return []Option{
		{Label: "Option 1", Value: "1"},
		{Label: "Option 2", Value: "2"},
		{Label: "Option 3", Value: "3"},
	}
}

func TestPicker_New(t *testing.T) {
	options := testOptions()
	m := New("Test Title", options)

	assert.Equal(t, "Test Title", m.title, "expected title to be set")
	assert.Len(t, m.options, 3, "expected 3 options")
	assert.Equal(t, 0, m.selected, "expected default selection at 0")
}

func TestPicker_SetSelected(t *testing.T) {
	options := testOptions()
	m := New("Test", options)

	// Set valid index
	m = m.SetSelected(2)
	assert.Equal(t, 2, m.selected, "expected selection at index 2")

	// Set invalid index (too high) - should not change
	m = m.SetSelected(10)
	assert.Equal(t, 2, m.selected, "expected selection unchanged for invalid index")

	// Set invalid index (negative) - should not change
	m = m.SetSelected(-1)
	assert.Equal(t, 2, m.selected, "expected selection unchanged for negative index")
}

func TestPicker_Selected(t *testing.T) {
	options := testOptions()
	m := New("Test", options)

	// Default selection
	selected := m.Selected()
	assert.Equal(t, "Option 1", selected.Label, "expected first option selected")
	assert.Equal(t, "1", selected.Value, "expected first option value")

	// After changing selection
	m = m.SetSelected(1)
	selected = m.Selected()
	assert.Equal(t, "Option 2", selected.Label, "expected second option selected")
	assert.Equal(t, "2", selected.Value, "expected second option value")
}

func TestPicker_Selected_Empty(t *testing.T) {
	m := New("Test", []Option{})
	selected := m.Selected()
	assert.Equal(t, Option{}, selected, "expected empty option for empty picker")
}

func TestPicker_Update_NavigateDown(t *testing.T) {
	options := testOptions()
	m := New("Test", options)

	// Navigate down with 'j'
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Equal(t, 1, m.selected, "expected selection at 1 after 'j'")

	// Navigate down with arrow key
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 2, m.selected, "expected selection at 2 after down arrow")

	// At bottom boundary - should not go past
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Equal(t, 2, m.selected, "expected selection to stay at 2 (boundary)")
}

func TestPicker_Update_NavigateUp(t *testing.T) {
	options := testOptions()
	m := New("Test", options).SetSelected(2)

	// Navigate up with 'k'
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	assert.Equal(t, 1, m.selected, "expected selection at 1 after 'k'")

	// Navigate up with arrow key
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, 0, m.selected, "expected selection at 0 after up arrow")

	// At top boundary - should not go past
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	assert.Equal(t, 0, m.selected, "expected selection to stay at 0 (boundary)")
}

func TestPicker_SetSize(t *testing.T) {
	m := New("Test", testOptions())

	m = m.SetSize(120, 40)
	assert.Equal(t, 120, m.viewportWidth, "expected viewport width to be 120")
	assert.Equal(t, 40, m.viewportHeight, "expected viewport height to be 40")

	// Verify immutability
	m2 := m.SetSize(80, 24)
	assert.Equal(t, 80, m2.viewportWidth, "expected new model width to be 80")
	assert.Equal(t, 24, m2.viewportHeight, "expected new model height to be 24")
	assert.Equal(t, 120, m.viewportWidth, "expected original model width unchanged")
}

func TestPicker_SetBoxWidth(t *testing.T) {
	m := New("Test", testOptions())

	m = m.SetBoxWidth(50)
	assert.Equal(t, 50, m.boxWidth, "expected box width to be 50")

	// Verify immutability
	m2 := m.SetBoxWidth(30)
	assert.Equal(t, 30, m2.boxWidth, "expected new model box width to be 30")
	assert.Equal(t, 50, m.boxWidth, "expected original model box width unchanged")
}

func TestPicker_FindIndexByValue(t *testing.T) {
	options := testOptions()

	// Find existing value
	index := FindIndexByValue(options, "2")
	assert.Equal(t, 1, index, "expected index 1 for value '2'")

	// Find first value
	index = FindIndexByValue(options, "1")
	assert.Equal(t, 0, index, "expected index 0 for value '1'")

	// Find last value
	index = FindIndexByValue(options, "3")
	assert.Equal(t, 2, index, "expected index 2 for value '3'")

	// Not found - returns 0
	index = FindIndexByValue(options, "nonexistent")
	assert.Equal(t, 0, index, "expected index 0 for non-existent value")
}

func TestPicker_View(t *testing.T) {
	options := testOptions()
	m := New("Select Option", options).SetSize(80, 24)
	view := m.View()

	// Should contain title
	assert.Contains(t, view, "Select Option", "expected view to contain title")

	// Should contain options
	assert.Contains(t, view, "Option 1", "expected view to contain Option 1")
	assert.Contains(t, view, "Option 2", "expected view to contain Option 2")
	assert.Contains(t, view, "Option 3", "expected view to contain Option 3")

	// Should have selection indicator on first option
	assert.Contains(t, view, ">", "expected view to contain selection indicator")
}

func TestPicker_View_WithSelection(t *testing.T) {
	options := testOptions()
	m := New("Test", options).SetSelected(1).SetSize(80, 24)
	view := m.View()

	// View should render without error
	require.NotEmpty(t, view, "expected non-empty view")
	assert.Contains(t, view, "Option 2", "expected view to contain selected option")
}

func TestPicker_View_Stability(t *testing.T) {
	options := testOptions()
	m := New("Test", options).SetSize(80, 24)

	view1 := m.View()
	view2 := m.View()

	// Same model should produce identical output
	assert.Equal(t, view1, view2, "expected stable output from same model")
}

// TestPicker_View_Golden uses teatest golden file comparison
// Run with -update flag to update golden files: go test -update perles/internal/ui/picker
func TestPicker_View_Golden(t *testing.T) {
	options := testOptions()
	m := New("Select Option", options).SetSize(80, 24)
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestPicker_View_Selected_Golden tests picker with non-default selection
func TestPicker_View_Selected_Golden(t *testing.T) {
	options := testOptions()
	m := New("Select Option", options).SetSelected(1).SetSize(80, 24)
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}
