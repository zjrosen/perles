package newviewmodal

import (
	"testing"

	"perles/internal/config"
	"perles/internal/ui/colorpicker"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	m := New("status:open")

	assert.Equal(t, "status:open", m.query, "expected query to be set")
	assert.Equal(t, FieldViewName, m.Focused(), "expected default focus on view name")
	assert.Equal(t, "#73F59F", m.SelectedColor().Hex, "expected default color to be green")
}

func TestSetSize(t *testing.T) {
	m := New("test")

	m = m.SetSize(120, 40)
	assert.Equal(t, 120, m.width, "expected width to be 120")
	assert.Equal(t, 40, m.height, "expected height to be 40")

	// Verify immutability
	m2 := m.SetSize(80, 24)
	assert.Equal(t, 80, m2.width, "expected new model width to be 80")
	assert.Equal(t, 120, m.width, "expected original model width unchanged")
}

func TestFieldNavigation_Forward(t *testing.T) {
	m := New("test")

	// Start at view name
	assert.Equal(t, FieldViewName, m.Focused())

	// Tab to column name
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, FieldColumnName, m.Focused())

	// Tab to color
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, FieldColor, m.Focused())

	// Tab to save
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, FieldSave, m.Focused())

	// Tab wraps to view name
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, FieldViewName, m.Focused())
}

func TestFieldNavigation_Backward(t *testing.T) {
	m := New("test")

	// Start at view name
	assert.Equal(t, FieldViewName, m.Focused())

	// Shift+Tab wraps to save
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	assert.Equal(t, FieldSave, m.Focused())

	// Shift+Tab to color
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	assert.Equal(t, FieldColor, m.Focused())

	// Shift+Tab to column name
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	assert.Equal(t, FieldColumnName, m.Focused())

	// Shift+Tab to view name
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	assert.Equal(t, FieldViewName, m.Focused())
}

func TestFieldNavigation_CtrlN(t *testing.T) {
	m := New("test")

	// ctrl+n should navigate forward
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	assert.Equal(t, FieldColumnName, m.Focused())
}

func TestFieldNavigation_CtrlP(t *testing.T) {
	m := New("test")

	// ctrl+p should wrap to save (from view name)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	assert.Equal(t, FieldSave, m.Focused())
}

func TestValidation_EmptyViewName(t *testing.T) {
	m := New("status:open")

	// Navigate to save button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // column
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // color
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // save
	assert.Equal(t, FieldSave, m.Focused())

	// Try to save with empty view name
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should not produce SaveMsg
	assert.Nil(t, cmd, "expected no command with empty view name")
	assert.True(t, m.HasError(), "expected error for empty view name")
}

func TestValidation_DuplicateViewName(t *testing.T) {
	existingViews := []config.ViewConfig{
		{Name: "My Bugs", Columns: nil},
		{Name: "Sprint Board", Columns: nil},
	}

	m := New("status:open").SetExistingViews(existingViews)

	// Type a view name that already exists
	for _, r := range "My Bugs" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Navigate to save button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // column
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // color
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // save
	assert.Equal(t, FieldSave, m.Focused())

	// Try to save with duplicate view name
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should not produce SaveMsg
	assert.Nil(t, cmd, "expected no command with duplicate view name")
	assert.True(t, m.HasError(), "expected error for duplicate view name")
	assert.Contains(t, m.saveError, "already exists", "expected error message about duplicate")
}

func TestValidation_DuplicateViewName_CaseInsensitive(t *testing.T) {
	existingViews := []config.ViewConfig{
		{Name: "My Bugs", Columns: nil},
	}

	m := New("status:open").SetExistingViews(existingViews)

	// Type a view name that differs only in case
	for _, r := range "my bugs" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Navigate to save button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // column
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // color
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // save

	// Try to save
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should not produce SaveMsg (case-insensitive match)
	assert.Nil(t, cmd, "expected no command with case-insensitive duplicate")
	assert.True(t, m.HasError(), "expected error for case-insensitive duplicate")
}

func TestValidation_ValidViewName(t *testing.T) {
	m := New("status:open")

	// Type view name
	for _, r := range "My View" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Navigate to save button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // column
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // color
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // save
	assert.Equal(t, FieldSave, m.Focused())

	// Save
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd, "expected command from enter")

	msg := cmd().(SaveMsg)
	assert.Equal(t, "My View", msg.ViewName)
	assert.Equal(t, "status:open", msg.Query)
}

func TestColumnName_DefaultsToViewName(t *testing.T) {
	m := New("status:open")

	// Type view name
	for _, r := range "My Bugs" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Navigate to save button (skip column name, leaving it empty)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // column
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // color
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // save

	// Save
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd, "expected command from enter")

	msg := cmd().(SaveMsg)
	assert.Equal(t, "My Bugs", msg.ViewName)
	assert.Equal(t, "My Bugs", msg.ColumnName, "expected column name to default to view name")
}

func TestColumnName_ExplicitValue(t *testing.T) {
	m := New("status:open")

	// Type view name
	for _, r := range "My View" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Tab to column name
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Type column name
	for _, r := range "Open Issues" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Navigate to save
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // color
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // save

	// Save
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd, "expected command from enter")

	msg := cmd().(SaveMsg)
	assert.Equal(t, "My View", msg.ViewName)
	assert.Equal(t, "Open Issues", msg.ColumnName)
}

func TestColorPicker_OpensAndCloses(t *testing.T) {
	m := New("test")

	// Navigate to color field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // column
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // color
	assert.Equal(t, FieldColor, m.Focused())

	// Enter opens color picker
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, m.showColorPicker, "expected color picker to be open")

	// CancelMsg from colorpicker closes it
	m, _ = m.Update(colorpicker.CancelMsg{})
	assert.False(t, m.showColorPicker, "expected color picker to be closed")
}

func TestColorPicker_Selection(t *testing.T) {
	m := New("test").SetSize(80, 24)

	// Navigate to color field and open picker
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // column
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // color
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, m.showColorPicker)

	// Send a SelectMsg as if user selected a color
	m, _ = m.Update(colorpicker.SelectMsg{Hex: "#FF8787"})
	assert.False(t, m.showColorPicker, "expected color picker to close")
	assert.Equal(t, "#FF8787", m.SelectedColor().Hex, "expected selected color to update")
}

func TestCancel(t *testing.T) {
	m := New("test")

	// Press escape
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	require.NotNil(t, cmd, "expected command from esc")

	msg := cmd()
	_, ok := msg.(CancelMsg)
	assert.True(t, ok, "expected CancelMsg")
}

func TestEnterNavigatesForward_FromViewName(t *testing.T) {
	m := New("test")
	assert.Equal(t, FieldViewName, m.Focused())

	// Enter should move to next field when not on save
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.Equal(t, FieldColumnName, m.Focused())
}

func TestEnterNavigatesForward_FromColumnName(t *testing.T) {
	m := New("test")

	// Navigate to column name
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, FieldColumnName, m.Focused())

	// Enter should move to color
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.Equal(t, FieldColor, m.Focused())
}

func TestView(t *testing.T) {
	m := New("test").SetSize(80, 24)
	view := m.View()

	// Should contain title
	assert.Contains(t, view, "Create New View", "expected title")

	// Should contain field labels
	assert.Contains(t, view, "View Name", "expected view name label")
	assert.Contains(t, view, "Column Name", "expected column name label")
	assert.Contains(t, view, "Color", "expected color label")

	// Should have save button
	assert.Contains(t, view, "Save", "expected save button")
}

func TestView_Stability(t *testing.T) {
	m := New("test").SetSize(80, 24)

	view1 := m.View()
	view2 := m.View()

	assert.Equal(t, view1, view2, "expected stable output")
}

// TestView_Golden uses teatest golden file comparison.
// Run with -update flag to update golden files: go test -update ./internal/ui/newviewmodal/...
func TestView_Golden(t *testing.T) {
	m := New("status:open").SetSize(80, 24)
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_ColumnNameFocused_Golden(t *testing.T) {
	m := New("status:open").SetSize(80, 24)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // focus column name
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_ColorFocused_Golden(t *testing.T) {
	m := New("status:open").SetSize(80, 24)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // column
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // color
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_SaveFocused_Golden(t *testing.T) {
	m := New("status:open").SetSize(80, 24)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // column
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // color
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // save
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_WithError_Golden(t *testing.T) {
	m := New("status:open").SetSize(80, 24)
	// Navigate to save with empty view name
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})   // column
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})   // color
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})   // save
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // trigger error
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}
