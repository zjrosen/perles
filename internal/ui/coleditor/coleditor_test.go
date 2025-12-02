package coleditor

import (
	"perles/internal/beads"
	"perles/internal/config"
	"perles/internal/ui/colorpicker"
	"perles/internal/ui/modal"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockExecutor is a test double for QueryExecutor.
type mockExecutor struct {
	issues []beads.Issue
}

func (m *mockExecutor) Execute(query string) ([]beads.Issue, error) {
	// Simple filter: return issues matching the query's status
	// For testing, just return all issues (preview behavior)
	return m.issues, nil
}

func TestNew_InitialState(t *testing.T) {
	cols := []config.ColumnConfig{
		{Name: "Blocked", Query: "status = open and blocked = true", Color: "#FF0000"},
	}
	ed := New(0, cols, nil)

	assert.Equal(t, ModeEdit, ed.Mode())
	assert.Equal(t, FieldName, ed.Focused())
	assert.Equal(t, "Blocked", ed.NameInput().Value())
	assert.Equal(t, "status = open and blocked = true", ed.QueryInput().Value())
}

func TestNewForCreate_DefaultValues(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Existing", Query: "status = open"}}
	ed := NewForCreate(0, columns, nil)

	assert.Equal(t, ModeNew, ed.Mode())
	assert.Equal(t, 0, ed.InsertAfter())
	assert.Equal(t, "", ed.NameInput().Value())
	assert.Equal(t, "status = open", ed.QueryInput().Value()) // Default query
	assert.Equal(t, "#AABBCC", ed.CurrentConfig().Color)
}

func TestNewForCreate_FocusOnName(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Existing", Query: "status = open"}}
	ed := NewForCreate(0, columns, nil)

	assert.Equal(t, FieldName, ed.Focused())
}

func TestNew_SetsModeEdit(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open"}}
	ed := New(0, columns, nil)

	assert.Equal(t, ModeEdit, ed.Mode())
	assert.Equal(t, -1, ed.InsertAfter()) // Not used in edit mode
}

func TestNavigation_DownMoves(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open"}}
	ed := New(0, columns, nil)

	// Initial focus on name field
	assert.Equal(t, FieldName, ed.Focused())

	// Down: Name → Color → Query → Save → Delete (matches visual layout)
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, FieldColor, ed.Focused())

	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, FieldQuery, ed.Focused())

	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, FieldSave, ed.Focused())
}

func TestNavigation_UpMoves(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open"}}
	ed := New(0, columns, nil)

	// Move down first: Name → Color → Query
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, FieldQuery, ed.Focused())

	// Press up to move back: Query → Color
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, FieldColor, ed.Focused())

	// Press shift+tab to move back: Color → Name
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	assert.Equal(t, FieldName, ed.Focused())
}

func TestNavigation_Wraps(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open"}}
	ed := New(0, columns, nil)

	// At FieldName, press up should wrap to FieldDelete
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, FieldDelete, ed.Focused())

	// Press down should wrap back to FieldName
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, FieldName, ed.Focused())
}

func TestNavigation_CtrlN(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open"}}
	ed := New(0, columns, nil)

	// Initial focus on name field
	assert.Equal(t, FieldName, ed.Focused())

	// ctrl+n should move down: Name → Color → Query
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	assert.Equal(t, FieldColor, ed.Focused())

	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	assert.Equal(t, FieldQuery, ed.Focused())

	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	assert.Equal(t, FieldSave, ed.Focused())
}

func TestNavigation_CtrlP(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open"}}
	ed := New(0, columns, nil)

	// Move down first: Name → Color → Query
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, FieldQuery, ed.Focused())

	// ctrl+p should move up: Query → Color → Name
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	assert.Equal(t, FieldColor, ed.Focused())

	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	assert.Equal(t, FieldName, ed.Focused())
}

func TestNewForCreate_NavigationSkipsDelete(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Existing", Query: "status = open"}}
	ed := NewForCreate(0, columns, nil)

	// Navigate through all fields - Delete should never be focused
	visited := make(map[Field]bool)
	for i := 0; i < 10; i++ {
		visited[ed.Focused()] = true
		ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	}

	assert.False(t, visited[FieldDelete], "Delete field should not be visited in New mode")
	assert.True(t, visited[FieldName], "Name field should be visited")
	assert.True(t, visited[FieldSave], "Save field should be visited")
}

func TestCurrentConfig_BuildsCorrectly(t *testing.T) {
	columns := []config.ColumnConfig{
		{Name: "Test", Query: "status = open and ready = true", Color: "#FF0000"},
	}
	ed := New(0, columns, nil)

	cfg := ed.CurrentConfig()

	assert.Equal(t, "Test", cfg.Name)
	assert.Equal(t, "status = open and ready = true", cfg.Query)
	assert.Equal(t, "#FF0000", cfg.Color)
}

func TestLivePreview_FiltersOnQuery(t *testing.T) {
	columns := []config.ColumnConfig{
		{Name: "Open", Query: "status = open", Color: "#FF0000"},
	}
	// Mock executor returns what the BQL query would return
	executor := &mockExecutor{
		issues: []beads.Issue{
			{ID: "1", Status: beads.StatusOpen},
		},
	}

	ed := New(0, columns, executor)

	// Preview should show what the executor returned
	require.Len(t, ed.PreviewIssues(), 1)
	assert.Equal(t, "1", ed.PreviewIssues()[0].ID)
}

func TestDelete_OpensModal(t *testing.T) {
	columns := []config.ColumnConfig{
		{Name: "Col1", Query: "status = open"},
		{Name: "Col2", Query: "status = closed"},
	}

	ed := New(0, columns, nil)

	// Navigate to delete field
	for ed.Focused() != FieldDelete {
		ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	}

	// Press enter to delete - should open modal, not delete directly
	ed, cmd := ed.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should show delete modal
	assert.True(t, ed.ShowDeleteModal())

	// Should NOT have sent DeleteMsg yet (only modal init command)
	if cmd != nil {
		msg := cmd()
		_, isDelete := msg.(DeleteMsg)
		assert.False(t, isDelete, "Should not send DeleteMsg when opening modal")
	}
}

func TestDeleteModal_ConfirmSendsDeleteMsg(t *testing.T) {
	columns := []config.ColumnConfig{
		{Name: "Col1", Query: "status = open"},
		{Name: "Col2", Query: "status = closed"},
	}

	ed := New(0, columns, nil)

	// Navigate to delete field and open modal
	for ed.Focused() != FieldDelete {
		ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, ed.ShowDeleteModal())

	// Send modal.SubmitMsg (user confirmed)
	ed, cmd := ed.Update(modal.SubmitMsg{})

	// Modal should be closed
	assert.False(t, ed.ShowDeleteModal())

	// Should return DeleteMsg with correct index
	require.NotNil(t, cmd)
	msg := cmd()
	deleteMsg, ok := msg.(DeleteMsg)
	require.True(t, ok, "Expected DeleteMsg after confirm")
	assert.Equal(t, 0, deleteMsg.ColumnIndex)
}

func TestDeleteModal_CancelReturnsToEditor(t *testing.T) {
	columns := []config.ColumnConfig{
		{Name: "Col1", Query: "status = open"},
		{Name: "Col2", Query: "status = closed"},
	}

	ed := New(0, columns, nil)

	// Navigate to delete field and open modal
	for ed.Focused() != FieldDelete {
		ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, ed.ShowDeleteModal())

	// Send modal.CancelMsg (user cancelled)
	ed, cmd := ed.Update(modal.CancelMsg{})

	// Modal should be closed
	assert.False(t, ed.ShowDeleteModal())

	// Should NOT send DeleteMsg
	assert.Nil(t, cmd, "Should not return any command on cancel")

	// Should still be on delete field (still in editor)
	assert.Equal(t, FieldDelete, ed.Focused())
}

func TestDelete_AllowedForLastColumn(t *testing.T) {
	// Deleting last column is now allowed - returns to empty state
	columns := []config.ColumnConfig{{Name: "Only", Query: "status = open"}}

	ed := New(0, columns, nil)

	// Navigate to delete field
	for ed.Focused() != FieldDelete {
		ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	}

	// Press enter - should open delete modal (even for only column)
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should show delete modal
	assert.True(t, ed.ShowDeleteModal(), "Delete modal should open for last column")

	// Confirm deletion
	_, cmd := ed.Update(modal.SubmitMsg{})

	// Should return DeleteMsg
	require.NotNil(t, cmd)
	msg := cmd()
	deleteMsg, ok := msg.(DeleteMsg)
	require.True(t, ok, "Expected DeleteMsg after confirm")
	assert.Equal(t, 0, deleteMsg.ColumnIndex)
}

func TestEnter_ReturnsSaveMsg(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open"}}
	ed := New(0, columns, nil)

	// Navigate to save field
	for ed.Focused() != FieldSave {
		ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	}

	_, cmd := ed.Update(tea.KeyMsg{Type: tea.KeyEnter})

	require.NotNil(t, cmd)
	msg := cmd()
	saveMsg, ok := msg.(SaveMsg)
	require.True(t, ok)
	assert.Equal(t, 0, saveMsg.ColumnIndex)
	assert.Equal(t, "Test", saveMsg.Config.Name)
}

func TestEsc_ReturnsCancelMsg(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open"}}
	ed := New(0, columns, nil)

	_, cmd := ed.Update(tea.KeyMsg{Type: tea.KeyEscape})

	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(CancelMsg)
	require.True(t, ok)
}

func TestNewForCreate_ReturnsAddMsg(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Existing", Query: "status = open"}}
	ed := NewForCreate(2, columns, nil) // Insert after index 2

	// Type a name
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Test Column")})

	// Navigate to Save and press Enter
	for ed.Focused() != FieldSave {
		ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	_, cmd := ed.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should return AddMsg, not SaveMsg
	require.NotNil(t, cmd)
	msg := cmd()
	addMsg, ok := msg.(AddMsg)
	require.True(t, ok, "Expected AddMsg but got %T", msg)
	assert.Equal(t, 2, addMsg.InsertAfterIndex)
	assert.Equal(t, "Test Column", addMsg.Config.Name)
}

func TestSetSize(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open"}}
	ed := New(0, columns, nil)

	ed = ed.SetSize(100, 50)

	// View should render without panic
	view := ed.View()
	assert.NotEmpty(t, view)
}

func TestView_ContainsFormElements(t *testing.T) {
	columns := []config.ColumnConfig{
		{Name: "Ready", Query: "status = open and ready = true", Color: "#73F59F"},
	}
	ed := New(0, columns, nil)
	ed = ed.SetSize(100, 40)

	view := ed.View()

	// Check header
	assert.Contains(t, view, "Edit Column")
	assert.Contains(t, view, "Ready")

	// Check form sections (labels are now section headers, not field labels)
	assert.Contains(t, view, "─ Name ")
	assert.Contains(t, view, "─ Color ")
	assert.Contains(t, view, "─ BQL Query ")

	// Check preview section
	assert.Contains(t, view, "Live Preview")
}

func TestValidation_EmptyNameBlocksSave(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "", Query: "status = open"}}
	ed := New(0, columns, nil)

	// Navigate to save field
	for ed.Focused() != FieldSave {
		ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	}

	// Try to save with empty name
	ed, cmd := ed.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should NOT return a save command
	assert.Nil(t, cmd)

	// Should have validation error
	assert.Equal(t, "Column name is required", ed.ValidationError())
}

func TestValidation_EmptyQueryBlocksSave(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: ""}}
	ed := New(0, columns, nil)

	// Navigate to save field
	for ed.Focused() != FieldSave {
		ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	}

	// Try to save with empty query
	ed, cmd := ed.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should NOT return a save command
	assert.Nil(t, cmd)

	// Should have validation error
	assert.Contains(t, ed.ValidationError(), "query is required")
}

func TestColorWarning_InvalidFormat(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open", Color: "invalid"}}
	ed := New(0, columns, nil)

	assert.True(t, ed.HasColorWarning())
}

func TestColorWarning_ValidHex(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open", Color: "#FF0000"}}
	ed := New(0, columns, nil)

	assert.False(t, ed.HasColorWarning())
}

func TestColorWarning_EmptyIsOk(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open", Color: ""}}
	ed := New(0, columns, nil)

	assert.False(t, ed.HasColorWarning())
}

func TestHorizontalNavigation(t *testing.T) {
	columns := []config.ColumnConfig{
		{Name: "Test1", Query: "status = open"},
		{Name: "Test2", Query: "status = closed"},
	}
	ed := New(0, columns, nil)

	// Navigate to Save
	for ed.Focused() != FieldSave {
		ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	assert.Equal(t, FieldSave, ed.Focused())

	// Right should go to Delete in Edit mode
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyRight})
	assert.Equal(t, FieldDelete, ed.Focused())

	// Left should go back to Save
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyLeft})
	assert.Equal(t, FieldSave, ed.Focused())
}

// TestColEditor_View_Golden uses teatest golden file comparison
// Run with -update flag to update golden files: go test -update ./internal/ui/coleditor/...
func TestColEditor_View_Golden(t *testing.T) {
	columns := []config.ColumnConfig{
		{Name: "Ready", Query: "status = open and ready = true", Color: "#73F59F"},
	}
	executor := &mockExecutor{
		issues: []beads.Issue{
			{ID: "bd-1", TitleText: "First task", Status: beads.StatusOpen, Priority: beads.PriorityHigh},
		},
	}

	ed := New(0, columns, executor)
	ed = ed.SetSize(130, 30)

	view := ed.View()

	teatest.RequireEqualOutput(t, []byte(view))
}

func TestColorPicker_EnterOpensOverlay(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open", Color: "#FF0000"}}
	ed := New(0, columns, nil)

	// Navigate to FieldColor
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, FieldColor, ed.Focused())

	// Initially picker should be closed
	assert.False(t, ed.ShowColorPicker())

	// Press Enter to open picker
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, ed.ShowColorPicker())
}

func TestColorPicker_SelectMsgUpdatesColor(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open", Color: "#FF0000"}}
	ed := New(0, columns, nil)
	ed = ed.SetSize(80, 40)

	// Navigate to FieldColor and open picker
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, ed.ShowColorPicker())

	// Simulate selecting a color (send SelectMsg directly)
	ed, _ = ed.Update(colorpicker.SelectMsg{Hex: "#73F59F"})

	// Picker should close and color should update
	assert.False(t, ed.ShowColorPicker())
	assert.Equal(t, "#73F59F", ed.ColorValue())
	assert.Equal(t, "#73F59F", ed.CurrentConfig().Color)
}

func TestColorPicker_CancelMsgClosesWithoutChange(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open", Color: "#FF0000"}}
	ed := New(0, columns, nil)
	ed = ed.SetSize(80, 40)

	// Navigate to FieldColor and open picker
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, ed.ShowColorPicker())

	// Simulate cancel
	ed, _ = ed.Update(colorpicker.CancelMsg{})

	// Picker should close but color should remain unchanged
	assert.False(t, ed.ShowColorPicker())
	assert.Equal(t, "#FF0000", ed.ColorValue())
}

func TestColorPicker_NavigationWhenOpen(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open", Color: "#FF0000"}}
	ed := New(0, columns, nil)
	ed = ed.SetSize(80, 40)

	// Navigate to FieldColor and open picker
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, ed.ShowColorPicker())

	// Key presses should go to colorpicker, not form
	// Focus should remain on FieldColor (not navigate away)
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.True(t, ed.ShowColorPicker()) // Still open
	assert.Equal(t, FieldColor, ed.Focused())
}

func TestColorPicker_PreviewUpdatesAfterSelection(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open", Color: "#FF0000"}}
	executor := &mockExecutor{
		issues: []beads.Issue{{ID: "1", Status: beads.StatusOpen}},
	}
	ed := New(0, columns, executor)
	ed = ed.SetSize(80, 40)

	// Navigate to FieldColor and open picker
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Select a color
	ed, _ = ed.Update(colorpicker.SelectMsg{Hex: "#73F59F"})

	// Preview should still have issues (updatePreview called)
	assert.Len(t, ed.PreviewIssues(), 1)
}

func TestNewForCreate_EmptyColumns(t *testing.T) {
	// When creating a column on an empty view, insertAfterIndex is -1
	columns := []config.ColumnConfig{} // Empty columns
	ed := NewForCreate(-1, columns, nil)

	assert.Equal(t, ModeNew, ed.Mode())
	assert.Equal(t, -1, ed.InsertAfter())
	assert.Equal(t, "", ed.NameInput().Value())
	assert.Equal(t, "status = open", ed.QueryInput().Value())
	assert.Empty(t, ed.AllColumns())
}
