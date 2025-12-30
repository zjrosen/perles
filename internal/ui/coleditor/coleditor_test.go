package coleditor

import (
	"testing"

	"github.com/zjrosen/perles/internal/beads"
	"github.com/zjrosen/perles/internal/config"
	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/ui/shared/colorpicker"
	"github.com/zjrosen/perles/internal/ui/shared/modal"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestNew_InitialState(t *testing.T) {
	cols := []config.ColumnConfig{
		{Name: "Blocked", Query: "status = open and blocked = true", Color: "#FF0000"},
	}
	ed := New(0, cols, nil, false)

	require.Equal(t, ModeEdit, ed.Mode())
	require.Equal(t, FieldName, ed.Focused())
	require.Equal(t, "Blocked", ed.NameInput().Value())
	require.Equal(t, "status = open and blocked = true", ed.QueryInput().Value())
}

func TestNewForCreate_DefaultValues(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Existing", Query: "status = open"}}
	ed := NewForCreate(0, columns, nil, false)

	require.Equal(t, ModeNew, ed.Mode())
	require.Equal(t, 0, ed.InsertAfter())
	require.Equal(t, "", ed.NameInput().Value())
	require.Equal(t, "status = open", ed.QueryInput().Value()) // Default query
	require.Equal(t, "#AABBCC", ed.CurrentConfig().Color)
}

func TestNewForCreate_FocusOnName(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Existing", Query: "status = open"}}
	ed := NewForCreate(0, columns, nil, false)

	require.Equal(t, FieldName, ed.Focused())
}

func TestNew_SetsModeEdit(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open"}}
	ed := New(0, columns, nil, false)

	require.Equal(t, ModeEdit, ed.Mode())
	require.Equal(t, -1, ed.InsertAfter()) // Not used in edit mode
}

func TestNavigation_DownMoves(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open"}}
	ed := New(0, columns, nil, false)

	// Initial focus on name field
	require.Equal(t, FieldName, ed.Focused())

	// Down: Name → Type → Color → Query → Save → Delete (matches visual layout)
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, FieldType, ed.Focused())

	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, FieldColor, ed.Focused())

	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, FieldQuery, ed.Focused())

	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, FieldSave, ed.Focused())
}

func TestNavigation_UpMoves(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open"}}
	ed := New(0, columns, nil, false)

	// Move down first: Name → Type → Color → Query
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown}) // Name -> Type
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown}) // Type -> Color
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown}) // Color -> Query
	require.Equal(t, FieldQuery, ed.Focused())

	// Press up to move back: Query → Color
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyUp})
	require.Equal(t, FieldColor, ed.Focused())

	// Press shift+tab to move back: Color → Type
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	require.Equal(t, FieldType, ed.Focused())

	// Press shift+tab to move back: Type → Name
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	require.Equal(t, FieldName, ed.Focused())
}

func TestNavigation_Wraps(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open"}}
	ed := New(0, columns, nil, false)

	// At FieldName, press up should wrap to FieldDelete
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyUp})
	require.Equal(t, FieldDelete, ed.Focused())

	// Press down should wrap back to FieldName
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, FieldName, ed.Focused())
}

func TestNavigation_JK_OnNonTextFields(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open"}}
	ed := New(0, columns, nil, false)

	// Move to Type field (not a text input)
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, FieldType, ed.Focused())

	// 'j' should navigate down when on non-text field
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, FieldColor, ed.Focused(), "j should navigate to Color from Type")

	// 'k' should navigate up
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	require.Equal(t, FieldType, ed.Focused(), "k should navigate to Type from Color")
}

func TestNavigation_JK_OnNameField_ShouldType(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open"}}
	ed := New(0, columns, nil, false)

	// Start on FieldName
	require.Equal(t, FieldName, ed.Focused())

	// 'j' should type 'j' in the name field, not navigate
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, FieldName, ed.Focused(), "should stay on FieldName")
	require.Contains(t, ed.NameInput().Value(), "j", "should have typed 'j'")
}

func TestNavigation_CtrlN(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open"}}
	ed := New(0, columns, nil, false)

	// Initial focus on name field
	require.Equal(t, FieldName, ed.Focused())

	// ctrl+n should move down: Name → Type → Color → Query → Save
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	require.Equal(t, FieldType, ed.Focused())

	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	require.Equal(t, FieldColor, ed.Focused())

	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	require.Equal(t, FieldQuery, ed.Focused())

	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	require.Equal(t, FieldSave, ed.Focused())
}

func TestNavigation_CtrlP(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open"}}
	ed := New(0, columns, nil, false)

	// Move down first: Name → Type → Color → Query
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown}) // Name -> Type
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown}) // Type -> Color
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown}) // Color -> Query
	require.Equal(t, FieldQuery, ed.Focused())

	// ctrl+p should move up: Query → Color → Type → Name
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	require.Equal(t, FieldColor, ed.Focused())

	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	require.Equal(t, FieldType, ed.Focused())

	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	require.Equal(t, FieldName, ed.Focused())
}

func TestNewForCreate_NavigationSkipsDelete(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Existing", Query: "status = open"}}
	ed := NewForCreate(0, columns, nil, false)

	// Navigate through all fields - Delete should never be focused
	visited := make(map[Field]bool)
	for i := 0; i < 10; i++ {
		visited[ed.Focused()] = true
		ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	}

	require.False(t, visited[FieldDelete], "Delete field should not be visited in New mode")
	require.True(t, visited[FieldName], "Name field should be visited")
	require.True(t, visited[FieldSave], "Save field should be visited")
}

func TestCurrentConfig_BuildsCorrectly(t *testing.T) {
	columns := []config.ColumnConfig{
		{Name: "Test", Query: "status = open and ready = true", Color: "#FF0000"},
	}
	ed := New(0, columns, nil, false)

	cfg := ed.CurrentConfig()

	require.Equal(t, "Test", cfg.Name)
	require.Equal(t, "status = open and ready = true", cfg.Query)
	require.Equal(t, "#FF0000", cfg.Color)
}

func TestLivePreview_FiltersOnQuery(t *testing.T) {
	columns := []config.ColumnConfig{
		{Name: "Open", Query: "status = open", Color: "#FF0000"},
	}
	// Mock executor returns what the BQL query would return
	executor := mocks.NewMockBQLExecutor(t)
	executor.EXPECT().Execute(mock.Anything).Return([]beads.Issue{
		{ID: "1", Status: beads.StatusOpen},
	}, nil)

	ed := New(0, columns, executor, false)

	// Preview should show what the executor returned
	require.Len(t, ed.PreviewIssues(), 1)
	require.Equal(t, "1", ed.PreviewIssues()[0].ID)
}

func TestDelete_OpensModal(t *testing.T) {
	columns := []config.ColumnConfig{
		{Name: "Col1", Query: "status = open"},
		{Name: "Col2", Query: "status = closed"},
	}

	ed := New(0, columns, nil, false)

	// Navigate to delete field
	for ed.Focused() != FieldDelete {
		ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	}

	// Press enter to delete - should open modal, not delete directly
	ed, cmd := ed.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should show delete modal
	require.True(t, ed.ShowDeleteModal())

	// Should NOT have sent DeleteMsg yet (only modal init command)
	if cmd != nil {
		msg := cmd()
		_, isDelete := msg.(DeleteMsg)
		require.False(t, isDelete, "Should not send DeleteMsg when opening modal")
	}
}

func TestDeleteModal_ConfirmSendsDeleteMsg(t *testing.T) {
	columns := []config.ColumnConfig{
		{Name: "Col1", Query: "status = open"},
		{Name: "Col2", Query: "status = closed"},
	}

	ed := New(0, columns, nil, false)

	// Navigate to delete field and open modal
	for ed.Focused() != FieldDelete {
		ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, ed.ShowDeleteModal())

	// Send modal.SubmitMsg (user confirmed)
	ed, cmd := ed.Update(modal.SubmitMsg{})

	// Modal should be closed
	require.False(t, ed.ShowDeleteModal())

	// Should return DeleteMsg with correct index
	require.NotNil(t, cmd)
	msg := cmd()
	deleteMsg, ok := msg.(DeleteMsg)
	require.True(t, ok, "Expected DeleteMsg after confirm")
	require.Equal(t, 0, deleteMsg.ColumnIndex)
}

func TestDeleteModal_CancelReturnsToEditor(t *testing.T) {
	columns := []config.ColumnConfig{
		{Name: "Col1", Query: "status = open"},
		{Name: "Col2", Query: "status = closed"},
	}

	ed := New(0, columns, nil, false)

	// Navigate to delete field and open modal
	for ed.Focused() != FieldDelete {
		ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, ed.ShowDeleteModal())

	// Send modal.CancelMsg (user cancelled)
	ed, cmd := ed.Update(modal.CancelMsg{})

	// Modal should be closed
	require.False(t, ed.ShowDeleteModal())

	// Should NOT send DeleteMsg
	require.Nil(t, cmd, "Should not return any command on cancel")

	// Should still be on delete field (still in editor)
	require.Equal(t, FieldDelete, ed.Focused())
}

func TestDelete_AllowedForLastColumn(t *testing.T) {
	// Deleting last column is now allowed - returns to empty state
	columns := []config.ColumnConfig{{Name: "Only", Query: "status = open"}}

	ed := New(0, columns, nil, false)

	// Navigate to delete field
	for ed.Focused() != FieldDelete {
		ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	}

	// Press enter - should open delete modal (even for only column)
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should show delete modal
	require.True(t, ed.ShowDeleteModal(), "Delete modal should open for last column")

	// Confirm deletion
	_, cmd := ed.Update(modal.SubmitMsg{})

	// Should return DeleteMsg
	require.NotNil(t, cmd)
	msg := cmd()
	deleteMsg, ok := msg.(DeleteMsg)
	require.True(t, ok, "Expected DeleteMsg after confirm")
	require.Equal(t, 0, deleteMsg.ColumnIndex)
}

func TestEnter_ReturnsSaveMsg(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open"}}
	ed := New(0, columns, nil, false)

	// Navigate to save field
	for ed.Focused() != FieldSave {
		ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	}

	_, cmd := ed.Update(tea.KeyMsg{Type: tea.KeyEnter})

	require.NotNil(t, cmd)
	msg := cmd()
	saveMsg, ok := msg.(SaveMsg)
	require.True(t, ok)
	require.Equal(t, 0, saveMsg.ColumnIndex)
	require.Equal(t, "Test", saveMsg.Config.Name)
}

func TestEsc_ReturnsCancelMsg(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open"}}
	ed := New(0, columns, nil, false)

	_, cmd := ed.Update(tea.KeyMsg{Type: tea.KeyEscape})

	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(CancelMsg)
	require.True(t, ok)
}

func TestNewForCreate_ReturnsAddMsg(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Existing", Query: "status = open"}}
	ed := NewForCreate(2, columns, nil, false) // Insert after index 2

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
	require.Equal(t, 2, addMsg.InsertAfterIndex)
	require.Equal(t, "Test Column", addMsg.Config.Name)
}

func TestSetSize(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open"}}
	ed := New(0, columns, nil, false)

	ed = ed.SetSize(100, 50)

	// View should render without panic
	view := ed.View()
	require.NotEmpty(t, view)
}

func TestView_ContainsFormElements(t *testing.T) {
	columns := []config.ColumnConfig{
		{Name: "Ready", Query: "status = open and ready = true", Color: "#73F59F"},
	}
	ed := New(0, columns, nil, false)
	ed = ed.SetSize(100, 40)

	view := ed.View()

	// Check header
	require.Contains(t, view, "Edit Column")
	require.Contains(t, view, "Ready")

	// Check form sections (labels are now section headers, not field labels)
	require.Contains(t, view, "─ Name ")
	require.Contains(t, view, "─ Color ")
	require.Contains(t, view, "─ BQL Query ")

	// Check preview section
	require.Contains(t, view, "Live Preview")
}

func TestValidation_EmptyNameBlocksSave(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "", Query: "status = open"}}
	ed := New(0, columns, nil, false)

	// Navigate to save field
	for ed.Focused() != FieldSave {
		ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	}

	// Try to save with empty name
	ed, cmd := ed.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should NOT return a save command
	require.Nil(t, cmd)

	// Should have validation error
	require.Equal(t, "Column name is required", ed.ValidationError())
}

func TestValidation_EmptyQueryBlocksSave(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: ""}}
	ed := New(0, columns, nil, false)

	// Navigate to save field
	for ed.Focused() != FieldSave {
		ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	}

	// Try to save with empty query
	ed, cmd := ed.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should NOT return a save command
	require.Nil(t, cmd)

	// Should have validation error
	require.Contains(t, ed.ValidationError(), "query is required")
}

func TestColorWarning_InvalidFormat(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open", Color: "invalid"}}
	ed := New(0, columns, nil, false)

	require.True(t, ed.HasColorWarning())
}

func TestColorWarning_ValidHex(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open", Color: "#FF0000"}}
	ed := New(0, columns, nil, false)

	require.False(t, ed.HasColorWarning())
}

func TestColorWarning_EmptyIsOk(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open", Color: ""}}
	ed := New(0, columns, nil, false)

	require.False(t, ed.HasColorWarning())
}

func TestHorizontalNavigation(t *testing.T) {
	columns := []config.ColumnConfig{
		{Name: "Test1", Query: "status = open"},
		{Name: "Test2", Query: "status = closed"},
	}
	ed := New(0, columns, nil, false)

	// Navigate to Save
	for ed.Focused() != FieldSave {
		ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	require.Equal(t, FieldSave, ed.Focused())

	// Right should go to Delete in Edit mode
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyRight})
	require.Equal(t, FieldDelete, ed.Focused())

	// Left should go back to Save
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyLeft})
	require.Equal(t, FieldSave, ed.Focused())
}

// TestColEditor_View_Golden uses teatest golden file comparison
// Run with -update flag to update golden files: go test -update ./internal/ui/coleditor/...
func TestColEditor_View_Golden(t *testing.T) {
	columns := []config.ColumnConfig{
		{Name: "Ready", Query: "status = open and ready = true", Color: "#73F59F"},
	}
	executor := mocks.NewMockBQLExecutor(t)
	executor.EXPECT().Execute(mock.Anything).Return([]beads.Issue{
		{ID: "bd-1", TitleText: "First task", Status: beads.StatusOpen, Priority: beads.PriorityHigh},
	}, nil)

	ed := New(0, columns, executor, false)
	ed = ed.SetSize(130, 30)

	view := ed.View()

	teatest.RequireEqualOutput(t, []byte(view))
}

func TestColEditor_View_Golden_Tall(t *testing.T) {
	columns := []config.ColumnConfig{
		{Name: "Ready", Query: "status = open and ready = true", Color: "#73F59F"},
	}
	executor := mocks.NewMockBQLExecutor(t)
	executor.EXPECT().Execute(mock.Anything).Return([]beads.Issue{
		{ID: "bd-1", TitleText: "First task", Status: beads.StatusOpen, Priority: beads.PriorityHigh},
	}, nil)

	ed := New(0, columns, executor, false)
	ed = ed.SetSize(130, 51)

	view := ed.View()

	teatest.RequireEqualOutput(t, []byte(view))
}

func TestColEditor_View_Golden_TreePreview(t *testing.T) {
	// Tree column with tree data loaded
	columns := []config.ColumnConfig{
		{Name: "Dependencies", Type: "tree", IssueID: "epic-1", Color: "#54A0FF"},
	}

	// Create mock executor with tree data (epic with child tasks)
	executor := mocks.NewMockBQLExecutor(t)
	executor.EXPECT().Execute(mock.Anything).Return([]beads.Issue{
		{
			ID:        "epic-1",
			TitleText: "Epic: Implement tree columns",
			Status:    beads.StatusInProgress,
			Priority:  beads.PriorityHigh,
			Type:      beads.TypeEpic,
			Blocks:    []string{"task-1", "task-2"},
		},
		{
			ID:        "task-1",
			TitleText: "Add tree column to board",
			Status:    beads.StatusClosed,
			Priority:  beads.PriorityMedium,
			Type:      beads.TypeTask,
			BlockedBy: []string{"epic-1"},
		},
		{
			ID:        "task-2",
			TitleText: "Fix tree width calculation",
			Status:    beads.StatusOpen,
			Priority:  beads.PriorityMedium,
			Type:      beads.TypeTask,
			BlockedBy: []string{"epic-1"},
		},
	}, nil)

	ed := New(0, columns, executor, false)
	ed = ed.SetSize(130, 40)

	view := ed.View()

	teatest.RequireEqualOutput(t, []byte(view))
}

func TestColorPicker_EnterOpensOverlay(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open", Color: "#FF0000"}}
	ed := New(0, columns, nil, false)

	// Navigate to FieldColor (Name -> Type -> Color)
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown}) // Name -> Type
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown}) // Type -> Color
	require.Equal(t, FieldColor, ed.Focused())

	// Initially picker should be closed
	require.False(t, ed.ShowColorPicker())

	// Press Enter to open picker
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, ed.ShowColorPicker())
}

func TestColorPicker_SelectMsgUpdatesColor(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open", Color: "#FF0000"}}
	ed := New(0, columns, nil, false)
	ed = ed.SetSize(80, 40)

	// Navigate to FieldColor (Name -> Type -> Color) and open picker
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown}) // Name -> Type
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown}) // Type -> Color
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, ed.ShowColorPicker())

	// Simulate selecting a color (send SelectMsg directly)
	ed, _ = ed.Update(colorpicker.SelectMsg{Hex: "#73F59F"})

	// Picker should close and color should update
	require.False(t, ed.ShowColorPicker())
	require.Equal(t, "#73F59F", ed.ColorValue())
	require.Equal(t, "#73F59F", ed.CurrentConfig().Color)
}

func TestColorPicker_CancelMsgClosesWithoutChange(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open", Color: "#FF0000"}}
	ed := New(0, columns, nil, false)
	ed = ed.SetSize(80, 40)

	// Navigate to FieldColor (Name -> Type -> Color) and open picker
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown}) // Name -> Type
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown}) // Type -> Color
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, ed.ShowColorPicker())

	// Simulate cancel
	ed, _ = ed.Update(colorpicker.CancelMsg{})

	// Picker should close but color should remain unchanged
	require.False(t, ed.ShowColorPicker())
	require.Equal(t, "#FF0000", ed.ColorValue())
}

func TestColorPicker_NavigationWhenOpen(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open", Color: "#FF0000"}}
	ed := New(0, columns, nil, false)
	ed = ed.SetSize(80, 40)

	// Navigate to FieldColor (Name -> Type -> Color) and open picker
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown}) // Name -> Type
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown}) // Type -> Color
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, ed.ShowColorPicker())

	// Key presses should go to colorpicker, not form
	// Focus should remain on FieldColor (not navigate away)
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.True(t, ed.ShowColorPicker()) // Still open
	require.Equal(t, FieldColor, ed.Focused())
}

func TestColorPicker_PreviewUpdatesAfterSelection(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open", Color: "#FF0000"}}
	executor := mocks.NewMockBQLExecutor(t)
	executor.EXPECT().Execute(mock.Anything).Return([]beads.Issue{
		{ID: "1", Status: beads.StatusOpen},
	}, nil).Maybe()
	ed := New(0, columns, executor, false)
	ed = ed.SetSize(80, 40)

	// Navigate to FieldColor and open picker
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Select a color
	ed, _ = ed.Update(colorpicker.SelectMsg{Hex: "#73F59F"})

	// Preview should still have issues (updatePreview called)
	require.Len(t, ed.PreviewIssues(), 1)
}

func TestNewForCreate_EmptyColumns(t *testing.T) {
	// When creating a column on an empty view, insertAfterIndex is -1
	columns := []config.ColumnConfig{} // Empty columns
	ed := NewForCreate(-1, columns, nil, false)

	require.Equal(t, ModeNew, ed.Mode())
	require.Equal(t, -1, ed.InsertAfter())
	require.Equal(t, "", ed.NameInput().Value())
	require.Equal(t, "status = open", ed.QueryInput().Value())
	require.Empty(t, ed.AllColumns())
}

// Type selector tests

func TestTypeSelector_DefaultIsBQL(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open"}}
	ed := New(0, columns, nil, false)

	require.Equal(t, "bql", ed.ColumnType())
}

func TestTypeSelector_LoadsFromConfig(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Tree", Type: "tree", IssueID: "perles-123"}}
	ed := New(0, columns, nil, false)

	require.Equal(t, "tree", ed.ColumnType())
	require.Equal(t, "perles-123", ed.IssueIDInput().Value())
}

func TestTypeSelector_ToggleWithRightArrow(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open"}}
	ed := New(0, columns, nil, false)

	// Navigate to FieldType
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, FieldType, ed.Focused())

	// Initially BQL
	require.Equal(t, "bql", ed.ColumnType())

	// Press right to toggle to tree
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyRight})
	require.Equal(t, "tree", ed.ColumnType())

	// Press right again should stay at tree (can't go past)
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyRight})
	require.Equal(t, "tree", ed.ColumnType())
}

func TestTypeSelector_ToggleWithLeftArrow(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Tree", Type: "tree", IssueID: "perles-123"}}
	ed := New(0, columns, nil, false)

	// Navigate to FieldType
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, FieldType, ed.Focused())

	// Initially tree
	require.Equal(t, "tree", ed.ColumnType())

	// Press left to toggle to bql
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyLeft})
	require.Equal(t, "bql", ed.ColumnType())

	// Press left again should stay at bql (can't go past)
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyLeft})
	require.Equal(t, "bql", ed.ColumnType())
}

func TestTypeSelector_NavigationSkipsHiddenFields(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Tree", Type: "tree", IssueID: "perles-123"}}
	ed := New(0, columns, nil, false)

	// For tree type: Name -> Type -> Color -> IssueID -> TreeMode -> Save (skips Query)
	require.Equal(t, FieldName, ed.Focused())

	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, FieldType, ed.Focused())

	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, FieldColor, ed.Focused())

	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, FieldIssueID, ed.Focused()) // Skips Query, goes to IssueID

	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, FieldTreeMode, ed.Focused()) // IssueID -> TreeMode

	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, FieldSave, ed.Focused())
}

func TestTypeSelector_BQLNavigationSkipsIssueID(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open"}}
	ed := New(0, columns, nil, false)

	// For BQL type: Name -> Type -> Color -> Query -> Save (skips IssueID)
	require.Equal(t, FieldName, ed.Focused())

	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, FieldType, ed.Focused())

	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, FieldColor, ed.Focused())

	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, FieldQuery, ed.Focused()) // Goes to Query

	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, FieldSave, ed.Focused()) // Skips IssueID
}

func TestTypeSelector_CurrentConfigIncludesType(t *testing.T) {
	// Test BQL config
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open"}}
	ed := New(0, columns, nil, false)

	cfg := ed.CurrentConfig()
	require.Equal(t, "bql", cfg.Type)
	require.Equal(t, "status = open", cfg.Query)
	require.Empty(t, cfg.IssueID)

	// Test tree config with IssueID set
	treeColumns := []config.ColumnConfig{{Name: "Tree", Type: "tree", IssueID: "perles-456"}}
	treeEd := New(0, treeColumns, nil, false)

	cfg = treeEd.CurrentConfig()
	require.Equal(t, "tree", cfg.Type)
	require.Equal(t, "perles-456", cfg.IssueID)
	require.Empty(t, cfg.Query) // Query is not set for tree type
}

func TestTypeSelector_ValidationRequiresIssueIDForTree(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Type: "tree"}}
	ed := New(0, columns, nil, false)
	ed.nameInput.SetValue("Tree Column")

	// IssueID is empty - should fail validation
	// Navigate to Save and press Enter
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown}) // Name -> Type
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown}) // Type -> Color
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown}) // Color -> IssueID
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown}) // IssueID -> TreeMode
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown}) // TreeMode -> Save
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyEnter})

	require.NotEmpty(t, ed.ValidationError())
	require.Contains(t, ed.ValidationError(), "Issue ID")
}

func TestTypeSelector_ViewRendersSelector(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Test", Query: "status = open"}}
	ed := New(0, columns, nil, false).SetSize(80, 40)

	view := ed.View()
	require.Contains(t, view, "BQL Query")
	require.Contains(t, view, "Tree View")
	require.Contains(t, view, "←/→ to switch")
}

// Integration test: Column editor creates tree column
func TestNewForCreate_TreeColumn_ReturnsAddMsg(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Existing", Query: "status = open"}}
	ed := NewForCreate(0, columns, nil, false)

	// Type a name
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Dependencies")})

	// Navigate to Type field and switch to tree
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown}) // Name -> Type
	require.Equal(t, FieldType, ed.Focused())
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyRight}) // Toggle to tree
	require.Equal(t, "tree", ed.ColumnType())

	// Navigate to IssueID and type an ID
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown}) // Type -> Color
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown}) // Color -> IssueID
	require.Equal(t, FieldIssueID, ed.Focused())
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("perles-123")})

	// Navigate to Save and press Enter
	for ed.Focused() != FieldSave {
		ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	_, cmd := ed.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should return AddMsg with tree type
	require.NotNil(t, cmd)
	msg := cmd()
	addMsg, ok := msg.(AddMsg)
	require.True(t, ok, "Expected AddMsg but got %T", msg)
	require.Equal(t, "Dependencies", addMsg.Config.Name)
	require.Equal(t, "tree", addMsg.Config.Type)
	require.Equal(t, "perles-123", addMsg.Config.IssueID)
	require.Empty(t, addMsg.Config.Query, "Query should be empty for tree columns")
}

// Integration test: Saved config has correct type and issue_id
func TestEdit_TreeColumn_ReturnsSaveMsgWithType(t *testing.T) {
	columns := []config.ColumnConfig{
		{Name: "Tree Col", Type: "tree", IssueID: "perles-old"},
	}
	ed := New(0, columns, nil, false)

	// Edit the issue ID
	// Navigate to IssueID field: Name -> Type -> Color -> IssueID
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown}) // Name -> Type
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown}) // Type -> Color
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown}) // Color -> IssueID
	require.Equal(t, FieldIssueID, ed.Focused())

	// Clear and type new ID
	for i := 0; i < 10; i++ { // Clear "perles-old"
		ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("perles-new")})

	// Navigate to Save and press Enter
	for ed.Focused() != FieldSave {
		ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	_, cmd := ed.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should return SaveMsg with tree type and new issue_id
	require.NotNil(t, cmd)
	msg := cmd()
	saveMsg, ok := msg.(SaveMsg)
	require.True(t, ok, "Expected SaveMsg but got %T", msg)
	require.Equal(t, "Tree Col", saveMsg.Config.Name)
	require.Equal(t, "tree", saveMsg.Config.Type)
	require.Equal(t, "perles-new", saveMsg.Config.IssueID)
}

// Tests for tree mode selector

func TestTreeModeSelector_DefaultIsDeps(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Tree", Type: "tree", IssueID: "perles-123"}}
	ed := New(0, columns, nil, false)

	require.Equal(t, "deps", ed.TreeMode())
}

func TestTreeModeSelector_LoadsFromConfig(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Tree", Type: "tree", IssueID: "perles-123", TreeMode: "child"}}
	ed := New(0, columns, nil, false)

	require.Equal(t, "child", ed.TreeMode())
}

func TestTreeModeSelector_ToggleWithRightArrow(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Tree", Type: "tree", IssueID: "perles-123"}}
	ed := New(0, columns, nil, false)

	// Navigate to TreeMode field: Name -> Type -> Color -> IssueID -> TreeMode
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown}) // Name -> Type
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown}) // Type -> Color
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown}) // Color -> IssueID
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown}) // IssueID -> TreeMode
	require.Equal(t, FieldTreeMode, ed.Focused())

	// Initially deps
	require.Equal(t, "deps", ed.TreeMode())

	// Press right to toggle to child
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyRight})
	require.Equal(t, "child", ed.TreeMode())

	// Press right again should stay at child (can't go past)
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyRight})
	require.Equal(t, "child", ed.TreeMode())
}

func TestTreeModeSelector_ToggleWithLeftArrow(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Tree", Type: "tree", IssueID: "perles-123", TreeMode: "child"}}
	ed := New(0, columns, nil, false)

	// Navigate to TreeMode field
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown}) // Name -> Type
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown}) // Type -> Color
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown}) // Color -> IssueID
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown}) // IssueID -> TreeMode
	require.Equal(t, FieldTreeMode, ed.Focused())

	// Initially child
	require.Equal(t, "child", ed.TreeMode())

	// Press left to toggle to deps
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyLeft})
	require.Equal(t, "deps", ed.TreeMode())

	// Press left again should stay at deps (can't go past)
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyLeft})
	require.Equal(t, "deps", ed.TreeMode())
}

func TestTreeModeSelector_CurrentConfigIncludesTreeMode(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Tree", Type: "tree", IssueID: "perles-123", TreeMode: "child"}}
	ed := New(0, columns, nil, false)

	cfg := ed.CurrentConfig()
	require.Equal(t, "child", cfg.TreeMode)
}

func TestTreeModeSelector_ViewRendersSelector(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Tree", Type: "tree", IssueID: "perles-123"}}
	ed := New(0, columns, nil, false).SetSize(80, 40)

	view := ed.View()
	require.Contains(t, view, "Dependencies")
	require.Contains(t, view, "Parent-Child")
	require.Contains(t, view, "Tree Mode")
}

func TestTreeModeSelector_SaveIncludesTreeMode(t *testing.T) {
	columns := []config.ColumnConfig{{Name: "Tree", Type: "tree", IssueID: "perles-123"}}
	ed := New(0, columns, nil, false)

	// Navigate to TreeMode and toggle to children
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})  // Name -> Type
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})  // Type -> Color
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})  // Color -> IssueID
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown})  // IssueID -> TreeMode
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyRight}) // Toggle to child

	// Navigate to Save and press Enter
	ed, _ = ed.Update(tea.KeyMsg{Type: tea.KeyDown}) // TreeMode -> Save
	_, cmd := ed.Update(tea.KeyMsg{Type: tea.KeyEnter})

	require.NotNil(t, cmd)
	msg := cmd()
	saveMsg, ok := msg.(SaveMsg)
	require.True(t, ok, "Expected SaveMsg but got %T", msg)
	require.Equal(t, "child", saveMsg.Config.TreeMode)
}
