package viewselector

import (
	"testing"

	"perles/internal/config"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testViews provides standard test fixtures.
var testViews = []config.ViewConfig{
	{Name: "Default"},
	{Name: "By Priority"},
	{Name: "Archived"},
}

// TestNew_NoViewsPreselected verifies no views are selected by default.
func TestNew_NoViewsPreselected(t *testing.T) {
	m := New("status = open", testViews)

	assert.Len(t, m.views, 3)
	for _, v := range m.views {
		assert.False(t, v.selected, "view %q should not be pre-selected", v.name)
	}
}

// TestSave_RequiresColumnName verifies save fails with empty column name.
func TestSave_RequiresColumnName(t *testing.T) {
	m := New("status = open", testViews)
	m.columnName.SetValue("") // Empty name

	_, cmd := m.save()
	assert.Nil(t, cmd, "should not save with empty name")
}

// TestSave_RequiresAtLeastOneView verifies save fails when no views selected.
func TestSave_RequiresAtLeastOneView(t *testing.T) {
	m := New("status = open", testViews)
	m.columnName.SetValue("Test Column")
	// Deselect all views
	for i := range m.views {
		m.views[i].selected = false
	}

	_, cmd := m.save()
	assert.Nil(t, cmd, "should not save with no views selected")
}

// TestToggle_SpaceTogglesSelection verifies space key toggles view selection.
func TestToggle_SpaceTogglesSelection(t *testing.T) {
	m := New("status = open", testViews)
	m.focusedField = FieldViewList
	m.selectedView = 0

	// All views start unselected
	assert.False(t, m.views[0].selected)

	// Space toggles on
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	assert.True(t, m.views[0].selected)

	// Space toggles back off
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	assert.False(t, m.views[0].selected)
}

// TestCycleField_TabCyclesForward verifies tab cycles through all 4 fields.
func TestCycleField_TabCyclesForward(t *testing.T) {
	m := New("status = open", testViews)
	assert.Equal(t, FieldColumnName, m.focusedField)

	// Tab cycles: ColumnName -> Color -> ViewList -> Save -> ColumnName
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, FieldColor, m.focusedField)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, FieldViewList, m.focusedField)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, FieldSave, m.focusedField)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, FieldColumnName, m.focusedField)
}

// TestCycleField_ShiftTabCyclesBackward verifies shift+tab cycles backward.
func TestCycleField_ShiftTabCyclesBackward(t *testing.T) {
	m := New("status = open", testViews)
	assert.Equal(t, FieldColumnName, m.focusedField)

	// Shift+Tab cycles: ColumnName -> Save -> ViewList -> Color -> ColumnName
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	assert.Equal(t, FieldSave, m.focusedField)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	assert.Equal(t, FieldViewList, m.focusedField)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	assert.Equal(t, FieldColor, m.focusedField)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	assert.Equal(t, FieldColumnName, m.focusedField)
}

// TestColorPicker_EnterOpensOverlay verifies enter on color field opens picker.
func TestColorPicker_EnterOpensOverlay(t *testing.T) {
	m := New("status = open", testViews)
	m.focusedField = FieldColor

	assert.False(t, m.showColorPicker)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, m.showColorPicker)
}

// TestEsc_ReturnsCancelMsg verifies escape returns cancel message.
func TestEsc_ReturnsCancelMsg(t *testing.T) {
	m := New("status = open", testViews)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(CancelMsg)
	assert.True(t, ok, "expected CancelMsg, got %T", msg)
}

// TestSave_ReturnsSaveMsgWithCorrectData verifies save returns message with correct data.
func TestSave_ReturnsSaveMsgWithCorrectData(t *testing.T) {
	m := New("status = open and label in (urgent)", testViews)
	m.columnName.SetValue("Urgent Issues")
	// Select views 0 and 2
	m.views[0].selected = true
	m.views[2].selected = true
	m.focusedField = FieldSave

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	saveMsg, ok := msg.(SaveMsg)
	require.True(t, ok, "expected SaveMsg, got %T", msg)

	assert.Equal(t, "Urgent Issues", saveMsg.ColumnName)
	assert.Equal(t, "#73F59F", saveMsg.Color) // Default green color
	assert.Equal(t, "status = open and label in (urgent)", saveMsg.Query)
	assert.Equal(t, []int{0, 2}, saveMsg.ViewIndices)
}

// TestNew_ZeroViews verifies empty view slice is handled gracefully.
func TestNew_ZeroViews(t *testing.T) {
	m := New("status = open", []config.ViewConfig{})

	assert.Empty(t, m.views)
	// Should still be able to render without panic
	view := m.View()
	assert.Contains(t, view, "(no views)")
}

// TestNew_SingleView verifies single view works and is not pre-selected.
func TestNew_SingleView(t *testing.T) {
	singleView := []config.ViewConfig{{Name: "Default"}}
	m := New("status = open", singleView)

	assert.Len(t, m.views, 1)
	assert.False(t, m.views[0].selected, "single view should not be pre-selected")
	assert.Equal(t, "Default", m.views[0].name)
}

// TestSingleView_SaveWorks verifies save works with exactly one view.
func TestSingleView_SaveWorks(t *testing.T) {
	singleView := []config.ViewConfig{{Name: "Default"}}
	m := New("status = open", singleView)
	m.columnName.SetValue("My Column")
	m.views[0].selected = true // Select the view
	m.focusedField = FieldSave

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	saveMsg, ok := msg.(SaveMsg)
	require.True(t, ok, "expected SaveMsg")
	assert.Equal(t, []int{0}, saveMsg.ViewIndices)
}

// TestViewNavigation_EmptyViewsNoOp verifies navigation keys don't panic with empty views.
func TestViewNavigation_EmptyViewsNoOp(t *testing.T) {
	m := New("status = open", []config.ViewConfig{})
	m.focusedField = FieldViewList

	// j/k/down/up should be no-ops, not panic
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})

	// Space toggle should also be no-op with empty views
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})

	// Just verify we didn't panic
	assert.Empty(t, m.views)
}

// Golden tests - run with -update flag to update: go test -update ./internal/ui/viewselector/...

// TestViewSelector_View_Golden tests the default view with no selections.
func TestViewSelector_View_Golden(t *testing.T) {
	m := New("status = open", testViews).SetSize(80, 24)
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestViewSelector_View_WithSelections_Golden tests the view with some views selected.
func TestViewSelector_View_WithSelections_Golden(t *testing.T) {
	m := New("status = open", testViews).SetSize(80, 24)
	m.views[0].selected = true
	m.views[2].selected = true
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestViewSelector_View_WithError_Golden tests the view with a validation error.
func TestViewSelector_View_WithError_Golden(t *testing.T) {
	m := New("status = open", testViews).SetSize(80, 24)
	m.columnName.SetValue("Test Column")
	m.focusedField = FieldSave
	// Trigger error by trying to save with no views selected
	m, _ = m.save()
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestViewSelector_View_NoViews_Golden tests the view with no views configured.
func TestViewSelector_View_NoViews_Golden(t *testing.T) {
	m := New("status = open", []config.ViewConfig{}).SetSize(80, 24)
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}
