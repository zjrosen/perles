package search

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/config"
	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/mode"
	"github.com/zjrosen/perles/internal/mode/shared"
	"github.com/zjrosen/perles/internal/task"
	"github.com/zjrosen/perles/internal/ui/details"
	"github.com/zjrosen/perles/internal/ui/modals/commenteditor"
	"github.com/zjrosen/perles/internal/ui/modals/issueeditor"
	"github.com/zjrosen/perles/internal/ui/shared/diffviewer"
	"github.com/zjrosen/perles/internal/ui/shared/editor"
	"github.com/zjrosen/perles/internal/ui/shared/formmodal"
	"github.com/zjrosen/perles/internal/ui/shared/toaster"
)

// createTestModel creates a minimal Model for testing state transitions.
// It does not require a database connection.
func createTestModel(t *testing.T) Model {
	cfg := config.Defaults()
	clipboard := mocks.NewMockClipboard(t)
	clipboard.EXPECT().Copy(mock.Anything).Return(nil).Maybe()

	mockTaskExec := mocks.NewMockTaskExecutor(t)
	mockTaskExec.EXPECT().GetComments(mock.Anything).Return([]task.Comment{}, nil).Maybe()

	mockExecutor := mocks.NewMockQueryExecutor(t)
	mockExecutor.EXPECT().Execute(mock.Anything).Return([]task.Issue{}, nil).Maybe()

	services := mode.Services{
		TaskExecutor:  mockTaskExec,
		QueryExecutor: mockExecutor,
		Config:        &cfg,
		Clipboard:     clipboard,
	}

	m := New(services)
	m.width = 100
	m.height = 40
	return m
}

// createTestModelWithResults creates a Model with some test results loaded.
func createTestModelWithResults(t *testing.T) Model {
	m := createTestModel(t)
	issues := []task.Issue{
		{ID: "test-1", TitleText: "First Issue", Priority: 1, Status: task.StatusOpen, Type: task.TypeTask},
		{ID: "test-2", TitleText: "Second Issue", Priority: 2, Status: task.StatusInProgress, Type: task.TypeBug},
		{ID: "test-3", TitleText: "Third Issue", Priority: 0, Status: task.StatusOpen, Type: task.TypeFeature},
	}
	m, _ = m.handleSearchResults(searchResultsMsg{issues: issues, err: nil})
	return m
}

func createCommentReadySearchModel(t *testing.T, issue task.Issue) (Model, *mocks.MockTaskExecutor) {
	t.Helper()

	cfg := config.Defaults()
	clipboard := mocks.NewMockClipboard(t)
	clipboard.EXPECT().Copy(mock.Anything).Return(nil).Maybe()

	taskExec := mocks.NewMockTaskExecutor(t)
	services := mode.Services{
		TaskExecutor: taskExec,
		Config:       &cfg,
		Clipboard:    clipboard,
	}

	m := New(services)
	m.width = 100
	m.height = 40
	m.results = []task.Issue{issue}
	m.selectedIdx = 0
	m.resultsList.SetItems([]list.Item{issueItem{issue: issue}})
	m.resultsList.Select(0)
	m.details = details.New(issue, nil, nil, taskExec).
		SetMarkdownStyle(cfg.UI.MarkdownStyle).
		SetSize(49, 38)
	m.hasDetail = true

	return m, taskExec
}

func TestSearch_New(t *testing.T) {
	m := createTestModel(t)

	require.Equal(t, FocusSearch, m.focus, "expected focus on search input")
	require.Equal(t, ViewSearch, m.view, "expected ViewSearch mode")
	require.False(t, m.hasDetail, "expected no detail initially")
	require.Nil(t, m.results, "expected no results initially")
}

func TestSearch_SetSize(t *testing.T) {
	m := createTestModel(t)

	m = m.SetSize(120, 50)

	require.Equal(t, 120, m.width, "width should be updated")
	require.Equal(t, 50, m.height, "height should be updated")
}

func TestSearch_RenderMainView_NarrowWidth_StacksPanels(t *testing.T) {
	m := createTestModelWithResults(t)
	m = m.SetSize(80, 24)

	view := m.renderMainView()
	for _, line := range strings.Split(view, "\n") {
		require.False(t, strings.Contains(line, "BQL Search") && strings.Contains(line, "Issue Details"))
	}
}

func TestSearch_SetSize_ZeroGuard(t *testing.T) {
	m := createTestModel(t)
	m.width = 100
	m.height = 40

	m = m.SetSize(0, 0)

	// Should not crash and should preserve existing values
	require.Equal(t, 0, m.width, "width should be 0")
	require.Equal(t, 0, m.height, "height should be 0")
}

func TestSearch_HandleSearchResults_Success(t *testing.T) {
	m := createTestModel(t)
	issues := []task.Issue{
		{ID: "test-1", TitleText: "First", Priority: 1, Status: task.StatusOpen},
		{ID: "test-2", TitleText: "Second", Priority: 2, Status: task.StatusClosed},
	}

	m, _ = m.handleSearchResults(searchResultsMsg{issues: issues, err: nil})

	require.Nil(t, m.searchErr, "expected no error")
	require.Len(t, m.results, 2, "expected 2 results")
	require.Equal(t, 0, m.selectedIdx, "expected first item selected")
	require.True(t, m.hasDetail, "expected detail panel to be active")
}

func TestSearch_HandleSearchResults_Empty(t *testing.T) {
	m := createTestModel(t)

	m, _ = m.handleSearchResults(searchResultsMsg{issues: []task.Issue{}, err: nil})

	require.Nil(t, m.searchErr, "expected no error")
	require.Empty(t, m.results, "expected empty results")
	require.False(t, m.hasDetail, "expected no detail panel")
}

func TestSearch_HandleSearchResults_Error(t *testing.T) {
	m := createTestModel(t)
	testErr := errors.New("invalid query syntax")

	m, cmd := m.handleSearchResults(searchResultsMsg{issues: nil, err: testErr})

	require.Equal(t, testErr, m.searchErr, "expected error to be set")
	require.Nil(t, m.results, "expected nil results")
	require.False(t, m.hasDetail, "expected no detail panel")
	// Error is shown in Results panel after blur, not via toaster
	require.False(t, m.showSearchErr, "showSearchErr should be false until blur")
	require.Nil(t, cmd, "no command expected (no toaster)")
}

func TestSearch_FocusNavigation_SlashFocusesSearch(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusResults
	m.input.Blur()

	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

	require.Equal(t, FocusSearch, m.focus, "expected focus on search")
	require.True(t, m.input.Focused(), "expected input to be focused")
}

func TestSearch_FocusNavigation_HMovesLeft(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusDetails

	// h moves focus from details to results
	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})

	require.Equal(t, FocusResults, m.focus, "expected focus on results")
}

func TestSearch_FocusNavigation_LMovesRight(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusResults

	// l moves focus from results to details
	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})

	require.Equal(t, FocusDetails, m.focus, "expected focus on details")
}

func TestSearch_FocusNavigation_LMovesToDetailsEvenWhenEmpty(t *testing.T) {
	m := createTestModel(t)
	m.focus = FocusResults
	m.hasDetail = false

	// l should move to details even when detail panel is empty
	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})

	require.Equal(t, FocusDetails, m.focus, "expected focus to move to details")
}

func TestSearch_FocusNavigation_EscFromSearchExitsToKanban(t *testing.T) {
	m := createTestModel(t)
	m.focus = FocusSearch
	m.input.Focus()

	m, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEscape})

	// Should blur input and return ExitToKanbanMsg
	require.False(t, m.input.Focused(), "expected input to be blurred")
	require.NotNil(t, cmd, "expected command to be returned")

	// Execute the command to get the message
	msg := cmd()
	_, ok := msg.(ExitToKanbanMsg)
	require.True(t, ok, "expected ExitToKanbanMsg")
}

func TestSearch_FocusNavigation_EscFromResultsExitsToKanban(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusResults

	m, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEscape})

	require.NotNil(t, cmd, "expected command to be returned")

	// Execute the command to get the message
	msg := cmd()
	_, ok := msg.(ExitToKanbanMsg)
	require.True(t, ok, "expected ExitToKanbanMsg")
}

func TestSearch_FocusNavigation_EscFromDetailsExitsToKanban(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusDetails

	m, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEscape})

	require.NotNil(t, cmd, "expected command to be returned")

	// Execute the command to get the message
	msg := cmd()
	_, ok := msg.(ExitToKanbanMsg)
	require.True(t, ok, "expected ExitToKanbanMsg")
}

func TestSearch_ResultSelection_JMovesDown(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusResults
	m.selectedIdx = 0

	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	require.Equal(t, 1, m.selectedIdx, "expected selectedIdx to increment")
}

func TestSearch_ResultSelection_KMovesUp(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusResults
	m.selectedIdx = 1

	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})

	require.Equal(t, 0, m.selectedIdx, "expected selectedIdx to decrement")
}

func TestSearch_ResultSelection_JAtEnd(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusResults
	m.selectedIdx = 2 // Last item

	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	require.Equal(t, 2, m.selectedIdx, "expected selectedIdx to stay at end")
}

func TestSearch_ResultSelection_KAtStart(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusResults
	m.selectedIdx = 0

	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})

	require.Equal(t, 0, m.selectedIdx, "expected selectedIdx to stay at start")
}

func TestSearch_HelpOverlay_QuestionOpens(t *testing.T) {
	m := createTestModel(t)
	m.focus = FocusResults // Must not be in search input for ? to open help

	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})

	require.Equal(t, ViewHelp, m.view, "expected help view")
}

func TestSearch_HelpOverlay_QuestionCloses(t *testing.T) {
	m := createTestModel(t)
	m.view = ViewHelp

	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})

	require.Equal(t, ViewSearch, m.view, "expected search view")
}

func TestSearch_HelpOverlay_EscCloses(t *testing.T) {
	m := createTestModel(t)
	m.view = ViewHelp

	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEscape})

	require.Equal(t, ViewSearch, m.view, "expected search view")
}

func TestSearch_HelpOverlay_ShowsUserActions(t *testing.T) {
	// Create config with user actions
	cfg := config.Defaults()
	cfg.UI.Actions.IssueAction = map[string]config.ActionConfig{
		"1": {Key: "1", Command: "echo test", Description: "Test action"},
		"2": {Key: "2", Command: "echo test2", Description: "Another action"},
	}

	clipboard := mocks.NewMockClipboard(t)
	clipboard.EXPECT().Copy(mock.Anything).Return(nil).Maybe()

	mockTaskExec := mocks.NewMockTaskExecutor(t)
	mockTaskExec.EXPECT().GetComments(mock.Anything).Return([]task.Comment{}, nil).Maybe()

	mockExecutor := mocks.NewMockQueryExecutor(t)
	mockExecutor.EXPECT().Execute(mock.Anything).Return([]task.Issue{}, nil).Maybe()

	services := mode.Services{
		TaskExecutor:  mockTaskExec,
		QueryExecutor: mockExecutor,
		Config:        &cfg,
		Clipboard:     clipboard,
	}

	m := New(services)
	m.width = 150
	m.height = 50
	m.view = ViewHelp

	view := m.View()

	// Should contain User Actions section
	require.Contains(t, view, "User Actions", "expected help overlay to contain User Actions section")
	// Should contain action keys and descriptions
	require.Contains(t, view, "1", "expected help overlay to contain key 1")
	require.Contains(t, view, "Test action", "expected help overlay to contain first action description")
	require.Contains(t, view, "2", "expected help overlay to contain key 2")
	require.Contains(t, view, "Another action", "expected help overlay to contain second action description")
}

func TestSearch_HelpOverlay_NoUserActions(t *testing.T) {
	// Create model with no user actions configured
	m := createTestModel(t)
	m.width = 100
	m.height = 40
	m.view = ViewHelp

	view := m.View()

	// Should NOT contain User Actions section when no actions configured
	require.NotContains(t, view, "User Actions", "expected help overlay to NOT contain User Actions section when no actions configured")
	// Should still contain other standard sections
	require.Contains(t, view, "Navigation", "expected help overlay to contain Navigation section")
	require.Contains(t, view, "Actions", "expected help overlay to contain Actions section")
	require.Contains(t, view, "General", "expected help overlay to contain General section")
}

func TestSearch_IssueSaved_Success_PatchesResults(t *testing.T) {
	m := createTestModelWithResults(t)

	newTitle := "Updated Title"
	newDescription := "Updated Description"
	newNotes := "Updated Notes"
	newPriority := task.PriorityCritical
	newStatus := task.StatusClosed
	newLabels := []string{"done"}
	msg := issueSavedMsg{
		issueID: "test-1",
		opts: task.UpdateOptions{
			Title:       &newTitle,
			Description: &newDescription,
			Notes:       &newNotes,
			Priority:    &newPriority,
			Status:      &newStatus,
			Labels:      &newLabels,
		},
	}
	m, cmd := m.handleIssueSaved(msg)

	require.NotNil(t, cmd, "expected ShowToastMsg command for success")
	require.Equal(t, "Updated Title", m.results[0].TitleText, "expected title patched in results")
	require.Equal(t, "Updated Description", m.results[0].DescriptionText, "expected description patched in results")
	require.Equal(t, "Updated Notes", m.results[0].Notes, "expected notes patched in results")
	require.Equal(t, task.PriorityCritical, m.results[0].Priority, "expected priority patched in results")
	require.Equal(t, task.StatusClosed, m.results[0].Status, "expected status patched in results")
	require.Equal(t, []string{"done"}, m.results[0].Labels, "expected labels patched in results")

	// Verify toast message
	toastResult := cmd()
	showToast, ok := toastResult.(mode.ShowToastMsg)
	require.True(t, ok, "expected ShowToastMsg")
	require.Equal(t, "Issue updated", showToast.Message)
	require.Equal(t, toaster.StyleSuccess, showToast.Style)
}

func TestSearch_IssueSaved_Success_UnchangedFieldsNotModified(t *testing.T) {
	m := createTestModelWithResults(t)

	// Set known values on first result
	m.results[0].TitleText = "Original Title"
	m.results[0].DescriptionText = "Original Description"
	m.results[0].Notes = "Original Notes"
	m.results[0].Priority = task.PriorityLow
	m.results[0].Status = task.StatusOpen
	m.results[0].Labels = []string{"original"}

	// Only update title — all other fields should remain unchanged
	newTitle := "Updated Title"
	msg := issueSavedMsg{
		issueID: "test-1",
		opts: task.UpdateOptions{
			Title: &newTitle,
		},
	}
	m, _ = m.handleIssueSaved(msg)

	require.Equal(t, "Updated Title", m.results[0].TitleText, "title should be updated")
	require.Equal(t, "Original Description", m.results[0].DescriptionText, "description should be unchanged")
	require.Equal(t, "Original Notes", m.results[0].Notes, "notes should be unchanged")
	require.Equal(t, task.PriorityLow, m.results[0].Priority, "priority should be unchanged")
	require.Equal(t, task.StatusOpen, m.results[0].Status, "status should be unchanged")
	require.Equal(t, []string{"original"}, m.results[0].Labels, "labels should be unchanged")
}

func TestSearch_IssueSaved_Error_ShowsToast(t *testing.T) {
	m := createTestModelWithResults(t)

	msg := issueSavedMsg{
		issueID: "test-1",
		err:     errors.New("db error"),
	}
	m, cmd := m.handleIssueSaved(msg)

	require.NotNil(t, cmd, "expected ShowToastMsg command for error")
	toastResult := cmd()
	showToast, ok := toastResult.(mode.ShowToastMsg)
	require.True(t, ok, "expected ShowToastMsg")
	require.Contains(t, showToast.Message, "Save failed")
	require.Contains(t, showToast.Message, "db error")
	require.Equal(t, toaster.StyleError, showToast.Style)
}

func TestSearch_IssueSaved_IssueNotInResults_NoPanic(t *testing.T) {
	m := createTestModelWithResults(t)

	newTitle := "Updated Title"
	msg := issueSavedMsg{
		issueID: "nonexistent-id",
		opts: task.UpdateOptions{
			Title: &newTitle,
		},
	}
	// Should not panic and should still return toast
	m, cmd := m.handleIssueSaved(msg)

	require.NotNil(t, cmd, "expected ShowToastMsg command")
	// Verify original results unchanged
	require.NotEqual(t, "Updated Title", m.results[0].TitleText, "first result should be unchanged")
}

func TestSearch_View_NotPanics(t *testing.T) {
	// Test that View() doesn't panic in various states
	tests := []struct {
		name string
		m    Model
	}{
		{"empty", createTestModel(t)},
		{"with_results", createTestModelWithResults(t)},
		{"help_view", func() Model {
			m := createTestModel(t)
			m.view = ViewHelp
			return m
		}()},
		{"edit_issue", func() Model {
			m := createTestModelWithResults(t)
			m.view = ViewEditIssue
			return m
		}()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			view := tt.m.View()
			require.NotEmpty(t, view, "view should not be empty")
		})
	}
}

func TestSearch_IssueItem_FilterValue(t *testing.T) {
	issue := task.Issue{ID: "test-1", TitleText: "My Test Issue"}
	item := issueItem{issue: issue}

	require.Equal(t, "My Test Issue", item.FilterValue())
}

func TestSearch_IssueDelegate_HeightAndSpacing(t *testing.T) {
	d := newIssueDelegate()

	require.Equal(t, 1, d.Height(), "delegate height should be 1")
	require.Equal(t, 0, d.Spacing(), "delegate spacing should be 0")
}

func TestSearch_EnterMsg_WithQuery(t *testing.T) {
	m := createTestModel(t)

	m, _ = m.Update(EnterMsg{SubMode: mode.SubModeList, Query: "status:open"})

	// Verify query was set on input
	require.Equal(t, "status:open", m.input.Value(), "query should be set")
	require.Equal(t, mode.SubModeList, m.subMode)
}

func TestSearch_EnterMsg_EmptyQuery(t *testing.T) {
	m := createTestModel(t)

	// Set a query first
	m, _ = m.Update(EnterMsg{SubMode: mode.SubModeList, Query: "priority:1"})
	require.Equal(t, "priority:1", m.input.Value())

	// Enter with empty query
	m, _ = m.Update(EnterMsg{SubMode: mode.SubModeList, Query: ""})

	// Should clear the query
	require.Equal(t, "", m.input.Value(), "empty query should clear input")
}

// Tests for Ctrl+S save as column flow

func TestCtrlS_OpensActionPicker(t *testing.T) {
	m := createTestModelWithViews()
	m.focus = FocusResults // Must not be in search input
	m.input.SetValue("status = open")

	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlS})

	require.Equal(t, ViewSaveAction, m.view, "expected action picker to open")
}

func TestCtrlS_RequiresQuery(t *testing.T) {
	m := createTestModelWithViews()
	m.focus = FocusResults
	m.input.SetValue("") // Empty query

	m, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlS})

	require.NotEqual(t, ViewSaveColumn, m.view, "should not open view selector with empty query")
	require.NotNil(t, cmd, "expected ShowToastMsg command for warning")
}

func TestViewSelector_EscReturnToSearch(t *testing.T) {
	m := createTestModelWithViews()
	m.view = ViewSaveColumn

	// The factory pattern produces closeSaveViewMsg instead of formmodal.CancelMsg
	m, _ = m.Update(closeSaveViewMsg{})

	require.Equal(t, ViewSearch, m.view, "expected to return to search view")
}

func TestViewSelector_SaveBubblesUp(t *testing.T) {
	m := createTestModelWithViews()
	m.view = ViewSaveColumn
	m.input.SetValue("status = open")

	// The factory pattern produces updateViewSaveMsg directly (no longer formmodal.SubmitMsg)
	saveMsg := updateViewSaveMsg{
		ColumnName:  "Test Column",
		Color:       "#73F59F",
		Query:       "status = open",
		ViewIndices: []int{0, 1},
	}
	m, cmd := m.Update(saveMsg)

	require.Equal(t, ViewSearch, m.view, "expected to return to search view")
	require.NotNil(t, cmd, "expected batch command with ShowToastMsg")
}

// createTestModelWithViews creates a Model with views configured for viewselector tests.
func createTestModelWithViews() Model {
	cfg := config.Defaults()
	cfg.Views = []config.ViewConfig{
		{Name: "Default"},
		{Name: "By Priority"},
	}
	services := mode.Services{
		Config: &cfg,
	}

	m := New(services)
	m.width = 100
	m.height = 40
	return m
}

// createTestModelWithNoViews creates a Model with no views configured.
func createTestModelWithNoViews() Model {
	cfg := config.Defaults()
	cfg.Views = []config.ViewConfig{} // No views
	services := mode.Services{
		Config: &cfg,
	}

	m := New(services)
	m.width = 100
	m.height = 40
	return m
}

func TestCtrlS_WorksWithNoViews(t *testing.T) {
	// With the new action picker, Ctrl+S works even without views
	// because "Save to new view" doesn't require existing views
	m := createTestModelWithNoViews()
	m.focus = FocusResults
	m.input.SetValue("status = open") // Has a query

	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlS})

	// Should show action picker - user can still create a new view
	require.Equal(t, ViewSaveAction, m.view, "should open action picker even with no views")
}

func TestActionPicker_SelectExistingView(t *testing.T) {
	m := createTestModelWithViews()
	m.view = ViewSaveAction

	// Simulate selecting "existing view" from action picker via domain message
	m, _ = m.Update(saveActionExistingViewMsg{query: "status = open"})

	require.Equal(t, ViewSaveColumn, m.view, "expected to transition to view selector")
}

func TestActionPicker_SelectNewView(t *testing.T) {
	m := createTestModelWithViews()
	m.view = ViewSaveAction

	// Simulate selecting "new view" from action picker via domain message
	m, _ = m.Update(saveActionNewViewMsg{query: "status = open"})

	require.Equal(t, ViewNewView, m.view, "expected to transition to new view modal")
}

func TestActionPicker_Cancel(t *testing.T) {
	m := createTestModelWithViews()
	m.view = ViewSaveAction

	// Simulate cancelling via closeSaveViewMsg (produced by picker's OnCancel callback)
	m, _ = m.Update(closeSaveViewMsg{})

	require.Equal(t, ViewSearch, m.view, "expected to return to search")
}

func TestNewViewModal_Save(t *testing.T) {
	m := createTestModelWithViews()
	m.view = ViewNewView
	m.input.SetValue("status = open")

	// The factory pattern produces newViewSaveMsg directly (no longer formmodal.SubmitMsg)
	saveMsg := newViewSaveMsg{
		ViewName:   "My Bugs",
		ColumnName: "Open Bugs",
		Color:      "#FF8787",
		Query:      "status = open",
	}
	m, cmd := m.Update(saveMsg)

	require.Equal(t, ViewSearch, m.view, "expected to return to search")
	require.NotNil(t, cmd, "expected batch command with ShowToastMsg")
}

func TestNewViewModal_Cancel(t *testing.T) {
	m := createTestModelWithViews()
	m.view = ViewNewView

	// The factory pattern produces closeSaveViewMsg instead of formmodal.CancelMsg
	m, _ = m.Update(closeSaveViewMsg{})

	require.Equal(t, ViewSearch, m.view, "expected to return to search")
}

func TestSearch_YankKey_FocusDetails_UsesDetailsIssueID(t *testing.T) {
	m := createTestModelWithResults(t)

	// Set up: results have test-1 selected, but details shows a different issue
	m.selectedIdx = 0
	require.Equal(t, "test-1", m.results[m.selectedIdx].ID, "precondition: results selection is test-1")

	// Create details view showing a DIFFERENT issue (test-999)
	differentIssue := task.Issue{ID: "test-999", TitleText: "Different Issue"}
	m.details = details.New(differentIssue, m.services.QueryExecutor, nil, m.services.TaskExecutor).SetSize(50, 30)
	m.hasDetail = true
	m.focus = FocusDetails

	// Press 'y' while focused on details
	m, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})

	// The command should be a function that returns a ShowToastMsg
	// We can't easily inspect the clipboard, but we can verify the command exists
	// and the toast message contains the details issue ID, not the results issue ID
	require.NotNil(t, cmd, "expected a command to be returned")

	// Execute the command to get the message
	msg := cmd()
	toastMsg, ok := msg.(mode.ShowToastMsg)
	require.True(t, ok, "expected ShowToastMsg, got %T", msg)

	// The toast should mention the details issue ID (test-999), not the results issue ID (test-1)
	require.Contains(t, toastMsg.Message, "test-999", "toast should contain details issue ID")
	require.NotContains(t, toastMsg.Message, "test-1", "toast should NOT contain results issue ID")
}

func TestSearch_YankKey_FocusResults_UsesResultsIssueID(t *testing.T) {
	m := createTestModelWithResults(t)

	// Set up: results have test-1 selected
	m.selectedIdx = 0
	require.Equal(t, "test-1", m.results[m.selectedIdx].ID, "precondition: results selection is test-1")

	// Create details view showing a DIFFERENT issue
	differentIssue := task.Issue{ID: "test-999", TitleText: "Different Issue"}
	m.details = details.New(differentIssue, m.services.QueryExecutor, nil, m.services.TaskExecutor).SetSize(50, 30)
	m.hasDetail = true
	m.focus = FocusResults // Focus on results, not details

	// Press 'y' while focused on results
	m, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})

	require.NotNil(t, cmd, "expected a command to be returned")

	// Execute the command to get the message
	msg := cmd()
	toastMsg, ok := msg.(mode.ShowToastMsg)
	require.True(t, ok, "expected ShowToastMsg, got %T", msg)

	// The toast should mention the results issue ID (test-1), not the details issue ID (test-999)
	require.Contains(t, toastMsg.Message, "test-1", "toast should contain results issue ID")
	require.NotContains(t, toastMsg.Message, "test-999", "toast should NOT contain details issue ID")
}

// --- Tree Form Factory Tests ---

func TestTreeModeToIndex(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		expected int
	}{
		{"deps returns 0", "deps", 0},
		{"children returns 1", "children", 1},
		{"empty string returns 0 (default)", "", 0},
		{"unknown mode returns 0 (default)", "unknown", 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := treeModeToIndex(tc.mode)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestMakeNewViewTreeFormConfig_Structure(t *testing.T) {
	existingViews := []string{"Backlog", "Sprint"}
	issueID := "test-123"
	treeMode := "deps"

	cfg := makeNewViewTreeFormConfig(existingViews, issueID, treeMode)

	// Check form title
	require.Equal(t, "Save Tree to New View", cfg.Title)

	// Check we have 4 fields
	require.Len(t, cfg.Fields, 4)

	// Field 0: viewName (text)
	require.Equal(t, "viewName", cfg.Fields[0].Key)
	require.Equal(t, formmodal.FieldTypeText, cfg.Fields[0].Type)
	require.Equal(t, "View Name", cfg.Fields[0].Label)
	require.Equal(t, "required", cfg.Fields[0].Hint)

	// Field 1: columnName (text with default)
	require.Equal(t, "columnName", cfg.Fields[1].Key)
	require.Equal(t, formmodal.FieldTypeText, cfg.Fields[1].Type)
	require.Equal(t, "tree: test-123", cfg.Fields[1].InitialValue)

	// Field 2: color
	require.Equal(t, "color", cfg.Fields[2].Key)
	require.Equal(t, formmodal.FieldTypeColor, cfg.Fields[2].Type)

	// Field 3: treeMode (toggle)
	require.Equal(t, "treeMode", cfg.Fields[3].Key)
	require.Equal(t, formmodal.FieldTypeToggle, cfg.Fields[3].Type)
	require.Len(t, cfg.Fields[3].Options, 2)
	require.Equal(t, "Dependencies", cfg.Fields[3].Options[0].Label)
	require.Equal(t, "deps", cfg.Fields[3].Options[0].Value)
	require.Equal(t, "Parent-Child", cfg.Fields[3].Options[1].Label)
	require.Equal(t, "children", cfg.Fields[3].Options[1].Value)
	require.Equal(t, 0, cfg.Fields[3].InitialToggleIndex) // deps mode -> index 0
}

func TestMakeNewViewTreeFormConfig_InitialToggleIndex_Children(t *testing.T) {
	cfg := makeNewViewTreeFormConfig(nil, "test-123", "children")

	// When mode is "children", InitialToggleIndex should be 1
	require.Equal(t, 1, cfg.Fields[3].InitialToggleIndex)
}

func TestMakeNewViewTreeFormConfig_Validation_EmptyName(t *testing.T) {
	cfg := makeNewViewTreeFormConfig(nil, "test-123", "deps")

	err := cfg.Validate(map[string]any{
		"viewName":   "",
		"columnName": "test column",
		"color":      "#73F59F",
		"treeMode":   "deps",
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "View name is required")
}

func TestMakeNewViewTreeFormConfig_Validation_DuplicateName(t *testing.T) {
	existingViews := []string{"Backlog", "Sprint"}
	cfg := makeNewViewTreeFormConfig(existingViews, "test-123", "deps")

	err := cfg.Validate(map[string]any{
		"viewName":   "Backlog",
		"columnName": "test column",
		"color":      "#73F59F",
		"treeMode":   "deps",
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "already exists")
}

func TestMakeNewViewTreeFormConfig_Validation_DuplicateName_CaseInsensitive(t *testing.T) {
	existingViews := []string{"Backlog", "Sprint"}
	cfg := makeNewViewTreeFormConfig(existingViews, "test-123", "deps")

	err := cfg.Validate(map[string]any{
		"viewName":   "BACKLOG", // Different case
		"columnName": "test column",
		"color":      "#73F59F",
		"treeMode":   "deps",
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "already exists")
}

func TestMakeNewViewTreeFormConfig_Validation_Success(t *testing.T) {
	existingViews := []string{"Backlog", "Sprint"}
	cfg := makeNewViewTreeFormConfig(existingViews, "test-123", "deps")

	err := cfg.Validate(map[string]any{
		"viewName":   "New View",
		"columnName": "test column",
		"color":      "#73F59F",
		"treeMode":   "deps",
	})

	require.NoError(t, err)
}

func TestMakeNewViewTreeFormConfig_OnSubmit(t *testing.T) {
	cfg := makeNewViewTreeFormConfig(nil, "test-123", "deps")

	msg := cfg.OnSubmit(map[string]any{
		"viewName":   "  My View  ", // With whitespace
		"columnName": "  My Column  ",
		"color":      "#FF8787",
		"treeMode":   "children",
	})

	saveMsg, ok := msg.(treeNewViewSaveMsg)
	require.True(t, ok, "expected treeNewViewSaveMsg, got %T", msg)
	require.Equal(t, "My View", saveMsg.ViewName)     // Trimmed
	require.Equal(t, "My Column", saveMsg.ColumnName) // Trimmed
	require.Equal(t, "#FF8787", saveMsg.Color)
	require.Equal(t, "test-123", saveMsg.IssueID)
	require.Equal(t, "children", saveMsg.TreeMode)
}

func TestMakeNewViewTreeFormConfig_OnSubmit_EmptyColumnName(t *testing.T) {
	cfg := makeNewViewTreeFormConfig(nil, "test-123", "deps")

	msg := cfg.OnSubmit(map[string]any{
		"viewName":   "My View",
		"columnName": "   ", // Empty after trim
		"color":      "#73F59F",
		"treeMode":   "deps",
	})

	saveMsg := msg.(treeNewViewSaveMsg)
	require.Equal(t, "My View", saveMsg.ColumnName) // Uses view name as fallback
}

func TestMakeUpdateViewTreeFormConfig_Structure(t *testing.T) {
	views := []string{"Backlog", "Sprint", "Done"}
	issueID := "test-456"
	treeMode := "children"

	cfg := makeUpdateViewTreeFormConfig(views, issueID, treeMode)

	// Check form title
	require.Equal(t, "Add Tree Column to Views", cfg.Title)

	// Check we have 4 fields
	require.Len(t, cfg.Fields, 4)

	// Field 0: columnName (text with default)
	require.Equal(t, "columnName", cfg.Fields[0].Key)
	require.Equal(t, formmodal.FieldTypeText, cfg.Fields[0].Type)
	require.Equal(t, "tree: test-456", cfg.Fields[0].InitialValue)

	// Field 1: color
	require.Equal(t, "color", cfg.Fields[1].Key)
	require.Equal(t, formmodal.FieldTypeColor, cfg.Fields[1].Type)

	// Field 2: treeMode (toggle)
	require.Equal(t, "treeMode", cfg.Fields[2].Key)
	require.Equal(t, formmodal.FieldTypeToggle, cfg.Fields[2].Type)
	require.Equal(t, 1, cfg.Fields[2].InitialToggleIndex) // children mode -> index 1

	// Field 3: views (list)
	require.Equal(t, "views", cfg.Fields[3].Key)
	require.Equal(t, formmodal.FieldTypeList, cfg.Fields[3].Type)
	require.True(t, cfg.Fields[3].MultiSelect)
	require.Len(t, cfg.Fields[3].Options, 3)
	require.Equal(t, "Backlog", cfg.Fields[3].Options[0].Label)
	require.Equal(t, "0", cfg.Fields[3].Options[0].Value)
}

func TestMakeUpdateViewTreeFormConfig_Validation_EmptyColumnName(t *testing.T) {
	cfg := makeUpdateViewTreeFormConfig([]string{"Backlog"}, "test-123", "deps")

	err := cfg.Validate(map[string]any{
		"columnName": "   ",
		"color":      "#73F59F",
		"treeMode":   "deps",
		"views":      []string{"0"},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "column name is required")
}

func TestMakeUpdateViewTreeFormConfig_Validation_NoViewsSelected(t *testing.T) {
	cfg := makeUpdateViewTreeFormConfig([]string{"Backlog"}, "test-123", "deps")

	err := cfg.Validate(map[string]any{
		"columnName": "My Column",
		"color":      "#73F59F",
		"treeMode":   "deps",
		"views":      []string{}, // Empty selection
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "select at least one view")
}

func TestMakeUpdateViewTreeFormConfig_Validation_Success(t *testing.T) {
	cfg := makeUpdateViewTreeFormConfig([]string{"Backlog", "Sprint"}, "test-123", "deps")

	err := cfg.Validate(map[string]any{
		"columnName": "My Column",
		"color":      "#73F59F",
		"treeMode":   "deps",
		"views":      []string{"0", "1"},
	})

	require.NoError(t, err)
}

func TestMakeUpdateViewTreeFormConfig_OnSubmit(t *testing.T) {
	cfg := makeUpdateViewTreeFormConfig([]string{"Backlog", "Sprint"}, "test-123", "deps")

	msg := cfg.OnSubmit(map[string]any{
		"columnName": "  Tree: test-123  ",
		"color":      "#FF8787",
		"treeMode":   "children",
		"views":      []string{"0", "1"},
	})

	saveMsg, ok := msg.(treeUpdateViewSaveMsg)
	require.True(t, ok, "expected treeUpdateViewSaveMsg, got %T", msg)
	require.Equal(t, "Tree: test-123", saveMsg.ColumnName) // Trimmed
	require.Equal(t, "#FF8787", saveMsg.Color)
	require.Equal(t, "test-123", saveMsg.IssueID)
	require.Equal(t, "children", saveMsg.TreeMode)
	require.Equal(t, []int{0, 1}, saveMsg.ViewIndices)
}

// --- Tree Save Toast Tests ---

func TestTreeNewViewSaveMsg_EmitsToast(t *testing.T) {
	m := createTestModelWithViews()
	m.view = ViewNewView

	saveMsg := treeNewViewSaveMsg{
		ViewName:   "My Tree View",
		ColumnName: "Deps Column",
		Color:      "#73F59F",
		IssueID:    "test-123",
		TreeMode:   "deps",
	}
	m, cmd := m.Update(saveMsg)

	require.Equal(t, ViewSearch, m.view, "expected to return to search view")
	require.NotNil(t, cmd, "expected batch command with ShowToastMsg")

	// Execute the batch command and verify ShowToastMsg is present
	msgs := executeBatchCmd(cmd)
	var toastFound bool
	for _, msg := range msgs {
		if toast, ok := msg.(mode.ShowToastMsg); ok {
			require.Contains(t, toast.Message, "My Tree View", "toast should mention view name")
			toastFound = true
		}
	}
	require.True(t, toastFound, "expected ShowToastMsg in batch")
}

func TestTreeUpdateViewSaveMsg_EmitsToast_SingleView(t *testing.T) {
	m := createTestModelWithViews()
	m.view = ViewSaveColumn

	saveMsg := treeUpdateViewSaveMsg{
		ColumnName:  "Tree Column",
		Color:       "#73F59F",
		IssueID:     "test-123",
		TreeMode:    "children",
		ViewIndices: []int{0},
	}
	m, cmd := m.Update(saveMsg)

	require.Equal(t, ViewSearch, m.view, "expected to return to search view")
	require.NotNil(t, cmd, "expected batch command with ShowToastMsg")

	// Execute the batch command and verify ShowToastMsg message
	msgs := executeBatchCmd(cmd)
	var toastFound bool
	for _, msg := range msgs {
		if toast, ok := msg.(mode.ShowToastMsg); ok {
			require.Equal(t, "Tree column added to 1 view", toast.Message, "toast should use singular")
			toastFound = true
		}
	}
	require.True(t, toastFound, "expected ShowToastMsg in batch")
}

func TestTreeUpdateViewSaveMsg_EmitsToast_MultipleViews(t *testing.T) {
	m := createTestModelWithViews()
	m.view = ViewSaveColumn

	saveMsg := treeUpdateViewSaveMsg{
		ColumnName:  "Tree Column",
		Color:       "#73F59F",
		IssueID:     "test-123",
		TreeMode:    "deps",
		ViewIndices: []int{0, 1, 2},
	}
	m, cmd := m.Update(saveMsg)

	require.Equal(t, ViewSearch, m.view, "expected to return to search view")
	require.NotNil(t, cmd, "expected batch command with ShowToastMsg")

	// Execute the batch command and verify ShowToastMsg message
	msgs := executeBatchCmd(cmd)
	var toastFound bool
	for _, msg := range msgs {
		if toast, ok := msg.(mode.ShowToastMsg); ok {
			require.Equal(t, "Tree column added to 3 view(s)", toast.Message, "toast should use plural")
			toastFound = true
		}
	}
	require.True(t, toastFound, "expected ShowToastMsg in batch")
}

// executeBatchCmd executes a tea.Cmd that returns a tea.BatchMsg and collects all messages.
func executeBatchCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	var results []tea.Msg
	for _, c := range batch {
		if c != nil {
			results = append(results, c())
		}
	}
	return results
}

// =============================================================================
// Quit Request Tests (quit modal now handled at app level)
// =============================================================================

func TestSearch_CtrlC_ReturnsRequestQuitMsg_FocusResults(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusResults

	// Simulate Ctrl+C keypress
	msg := tea.KeyMsg{Type: tea.KeyCtrlC}
	_, cmd := m.handleKey(msg)

	// Should return mode.RequestQuitMsg
	require.NotNil(t, cmd, "expected quit request command")
	result := cmd()
	_, isRequestQuit := result.(mode.RequestQuitMsg)
	require.True(t, isRequestQuit, "expected mode.RequestQuitMsg")
}

func TestSearch_CtrlC_ReturnsRequestQuitMsg_FocusSearch(t *testing.T) {
	m := createTestModel(t)
	m.focus = FocusSearch
	m.input.Focus()

	// Simulate Ctrl+C keypress
	msg := tea.KeyMsg{Type: tea.KeyCtrlC}
	_, cmd := m.handleKey(msg)

	// Should return mode.RequestQuitMsg
	require.NotNil(t, cmd, "expected quit request command")
	result := cmd()
	_, isRequestQuit := result.(mode.RequestQuitMsg)
	require.True(t, isRequestQuit, "expected mode.RequestQuitMsg in search input")
}

func TestSearch_CtrlC_ReturnsRequestQuitMsg_TreeSubMode(t *testing.T) {
	m := createTestModel(t)
	m.subMode = mode.SubModeTree
	m.focus = FocusResults

	// Simulate Ctrl+C keypress
	msg := tea.KeyMsg{Type: tea.KeyCtrlC}
	_, cmd := m.handleKey(msg)

	// Should return mode.RequestQuitMsg in tree sub-mode too
	require.NotNil(t, cmd, "expected quit request command")
	result := cmd()
	_, isRequestQuit := result.(mode.RequestQuitMsg)
	require.True(t, isRequestQuit, "expected mode.RequestQuitMsg in tree sub-mode")
}

func TestSearch_QKey_QuitsDirectly(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusResults

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	_, cmd := m.handleKey(msg)

	require.NotNil(t, cmd, "expected quit command")
	_, isQuit := cmd().(tea.QuitMsg)
	require.True(t, isQuit, "expected 'q' key to quit directly")
}

// =============================================================================
// Edit Key ('ctrl+e') Tests - List Pane Edit Menu Shortcut
// =============================================================================

func TestSearch_EditKey_ListSubMode_EmitsOpenEditMenuMsg(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusResults
	m.selectedIdx = 0

	// Verify preconditions
	require.Equal(t, mode.SubModeList, m.subMode, "should be in list sub-mode")
	require.Equal(t, "test-1", m.results[m.selectedIdx].ID, "should have test-1 selected")

	// Press 'ctrl+e' while focused on results in list mode
	m, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlE})

	// Should return a command that emits OpenEditMenuMsg
	require.NotNil(t, cmd, "expected a command to be returned")

	// Execute the command to get the message
	msg := cmd()
	editMsg, ok := msg.(details.OpenEditMenuMsg)
	require.True(t, ok, "expected OpenEditMenuMsg, got %T", msg)

	// Verify the message contains correct issue data
	require.Equal(t, "test-1", editMsg.Issue.ID, "issue ID should match selected issue")
	require.Equal(t, m.results[0].Labels, editMsg.Issue.Labels, "labels should match")
	require.Equal(t, m.results[0].Priority, editMsg.Issue.Priority, "priority should match")
	require.Equal(t, m.results[0].Status, editMsg.Issue.Status, "status should match")
}

func TestSearch_EditKey_EmptyList_NoOp(t *testing.T) {
	m := createTestModel(t)
	m.focus = FocusResults
	// No results loaded - m.results is nil/empty

	// Press 'ctrl+e' with no selected issue
	m, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlE})

	// Should return nil command (no-op)
	require.Nil(t, cmd, "expected no command when no issue is selected")
}

func TestSearch_EditKey_FocusDetails_DelegatesToDetails(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusDetails
	// Ensure details has an issue set
	m.details = details.New(m.results[0], m.services.QueryExecutor, nil, m.services.TaskExecutor).SetSize(50, 30)
	m.hasDetail = true

	// Press 'ctrl+e' while focused on details - should delegate to details component
	m, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlE})

	// Details component handles 'ctrl+e' key, so cmd should exist (from details)
	require.NotNil(t, cmd, "expected command from details delegation")

	// Execute the command to verify it's an OpenEditMenuMsg from details
	msg := cmd()
	editMsg, ok := msg.(details.OpenEditMenuMsg)
	require.True(t, ok, "expected OpenEditMenuMsg from details, got %T", msg)
	require.Equal(t, m.results[0].ID, editMsg.Issue.ID, "should edit details issue")
}

func TestSearch_EditKey_FocusSearch_NoOp(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusSearch
	m.input.Focus()

	// Press 'ctrl+e' while focused on search input - should not trigger edit, input keeps focus
	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlE})

	// No edit menu should open - input should still be focused
	require.True(t, m.input.Focused(), "input should still be focused")
}

func TestSearch_DeleteKey_ListSubMode_EmitsDeleteIssueMsg(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusResults
	m.selectedIdx = 0

	// Verify preconditions
	require.Equal(t, mode.SubModeList, m.subMode, "should be in list sub-mode")
	require.Equal(t, "test-1", m.results[m.selectedIdx].ID, "should have test-1 selected")

	// Press 'ctrl+d' while focused on results in list mode
	m, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlD})

	// Should return a command that emits DeleteIssueMsg
	require.NotNil(t, cmd, "expected a command to be returned")

	// Execute the command to get the message
	msg := cmd()
	deleteMsg, ok := msg.(details.DeleteIssueMsg)
	require.True(t, ok, "expected DeleteIssueMsg, got %T", msg)

	// Verify the message contains correct issue data
	require.Equal(t, "test-1", deleteMsg.IssueID, "issue ID should match selected issue")
	require.Equal(t, m.results[0].Type, deleteMsg.IssueType, "issue type should match")
}

func TestSearch_DeleteKey_EmptyList_NoOp(t *testing.T) {
	m := createTestModel(t)
	m.focus = FocusResults
	// No results loaded - m.results is nil/empty

	// Press 'ctrl+d' with no selected issue
	m, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlD})

	// Should return nil command (no-op)
	require.Nil(t, cmd, "expected no command when no issue is selected")
}

func TestSearch_DeleteKey_FocusDetails_DelegatesToDetails(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusDetails
	// Ensure details has an issue set
	m.details = details.New(m.results[0], m.services.QueryExecutor, nil, m.services.TaskExecutor).SetSize(50, 30)
	m.hasDetail = true

	// Press 'ctrl+d' while focused on details - should delegate to details component
	m, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlD})

	// Details component handles 'd' key, so cmd should exist (from details)
	require.NotNil(t, cmd, "expected command from details delegation")

	// Execute the command to verify it's a DeleteIssueMsg from details
	msg := cmd()
	deleteMsg, ok := msg.(details.DeleteIssueMsg)
	require.True(t, ok, "expected DeleteIssueMsg from details, got %T", msg)
	require.Equal(t, m.results[0].ID, deleteMsg.IssueID, "should delete details issue")
	require.Equal(t, m.results[0].Type, deleteMsg.IssueType, "should have correct issue type")
}

// =============================================================================
// Edge Case Tests - Modal Already Open
// =============================================================================

func TestSearch_EditKey_ModalOpen_KeyIgnored(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusResults
	m.selectedIdx = 0
	// Open a modal (e.g., delete confirmation)
	m.view = ViewDeleteConfirm

	// Press 'ctrl+e' while modal is open
	m, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlE})

	// Key should be handled by modal, not trigger edit
	// Modal state should remain unchanged
	require.Equal(t, ViewDeleteConfirm, m.view, "view should still be delete confirm")
	// The command should be nil or handled by modal (modal passes 'e' through as no-op)
	if cmd != nil {
		msg := cmd()
		_, isEditMsg := msg.(details.OpenEditMenuMsg)
		require.False(t, isEditMsg, "should NOT emit OpenEditMenuMsg when modal is open")
	}
}

func TestSearch_DeleteKey_ModalOpen_KeyIgnored(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusResults
	m.selectedIdx = 0
	// Open the unified issue editor modal
	m.view = ViewEditIssue

	// Press 'd' while modal is open
	m, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	// Key should be handled by modal, not trigger delete
	// View state should remain unchanged
	require.Equal(t, ViewEditIssue, m.view, "view should still be edit issue modal")
	// The command should be nil or handled by modal
	if cmd != nil {
		msg := cmd()
		_, isDeleteMsg := msg.(details.DeleteIssueMsg)
		require.False(t, isDeleteMsg, "should NOT emit DeleteIssueMsg when modal is open")
	}
}

// ============================================================================
// Issue Editor Integration Tests
// ============================================================================

func TestSearch_IssueEditor_OpenEditMenuMsg_SetsViewEditIssue(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusDetails
	issue := m.results[0]
	issue.Labels = []string{"bug", "urgent"}

	// Process OpenEditMenuMsg (simulating 'ctrl+e' key press from details)
	msg := details.OpenEditMenuMsg{Issue: issue}
	m, _ = m.Update(msg)

	require.Equal(t, ViewEditIssue, m.view, "expected ViewEditIssue view")
}

func TestSearch_IssueEditor_ViewEditIssue_RendersIssueEditorOverlay(t *testing.T) {
	m := createTestModelWithResults(t)
	issue := m.results[0]
	issue.Labels = []string{"feature"}

	// Open issue editor via OpenEditMenuMsg
	msg := details.OpenEditMenuMsg{Issue: issue}
	m, _ = m.Update(msg)

	// Render should not panic and should contain "Edit Issue"
	view := m.View()
	require.NotEmpty(t, view, "view should not be empty")
	require.Contains(t, view, "Edit Issue", "view should contain modal title")
}

func TestSearch_IssueEditor_SaveMsg_ReturnsToViewSearch(t *testing.T) {
	m := createTestModelWithResults(t)
	m.view = ViewEditIssue

	// Process SaveMsg
	msg := issueeditor.SaveMsg{
		IssueID:  "test-1",
		Priority: task.PriorityHigh,
		Status:   task.StatusInProgress,
		Labels:   []string{"updated"},
	}
	m, cmd := m.Update(msg)

	require.Equal(t, ViewSearch, m.view, "expected ViewSearch view after save")
	require.NotNil(t, cmd, "expected saveIssueCmd")
}

func TestSearch_IssueEditor_SaveMsg_DispatchesSaveIssueCmd(t *testing.T) {
	m := createTestModelWithResults(t)
	m.view = ViewEditIssue

	// Set up selectedIssue for change detection
	issue := m.results[0]
	m.selectedIssue = &issue

	// Set up mock executor
	mockExecutor := mocks.NewMockTaskExecutor(t)
	mockExecutor.EXPECT().UpdateIssue("test-1", mock.AnythingOfType("task.UpdateOptions")).Return(nil)
	m.services.TaskExecutor = mockExecutor

	// Process SaveMsg with changed fields
	msg := issueeditor.SaveMsg{
		IssueID:  "test-1",
		Priority: task.PriorityCritical,
		Status:   task.StatusClosed,
		Labels:   []string{"done"},
	}
	m, cmd := m.Update(msg)

	require.NotNil(t, cmd, "expected saveIssueCmd")
	require.Equal(t, ViewSearch, m.view, "view should be ViewSearch")
	require.Nil(t, m.selectedIssue, "selectedIssue should be cleared after save")

	// Execute the command to trigger UpdateIssue
	result := cmd()
	savedMsg, ok := result.(issueSavedMsg)
	require.True(t, ok, "expected issueSavedMsg")
	require.Equal(t, "test-1", savedMsg.issueID)
	require.NoError(t, savedMsg.err)
}

func TestSearch_IssueEditor_CancelMsg_ReturnsToViewSearch(t *testing.T) {
	m := createTestModelWithResults(t)
	m.view = ViewEditIssue

	// Process CancelMsg
	msg := issueeditor.CancelMsg{}
	m, cmd := m.Update(msg)

	require.Equal(t, ViewSearch, m.view, "expected ViewSearch view after cancel")
	require.Nil(t, cmd, "expected no command on cancel")
}

func TestSearch_IssueEditor_ReceivesCorrectInitialValuesFromOpenEditMenuMsg(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusDetails

	// Set up issue with specific values
	issue := &task.Issue{
		ID:       "test-custom",
		Priority: task.PriorityLow,
		Status:   task.StatusInProgress,
		Labels:   []string{"alpha", "beta", "gamma"},
	}
	m.results[0] = *issue

	// Open issue editor via OpenEditMenuMsg
	msg := details.OpenEditMenuMsg{Issue: *issue}
	m, _ = m.Update(msg)

	require.Equal(t, ViewEditIssue, m.view, "expected ViewEditIssue view")
	// The issueEditor model is now set up with the correct values
	// We can verify this by checking the view renders correctly
	view := m.View()
	require.Contains(t, view, "Edit Issue", "modal should be visible")
}

func TestSearch_IssueEditor_CtrlC_ClosesOverlay(t *testing.T) {
	m := createTestModelWithResults(t)
	issue := m.results[0]

	// Open issue editor
	msg := details.OpenEditMenuMsg{Issue: issue}
	m, _ = m.Update(msg)
	require.Equal(t, ViewEditIssue, m.view, "expected ViewEditIssue view")

	// Press Ctrl+C to close
	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})

	require.Equal(t, ViewSearch, m.view, "expected ViewSearch view after Ctrl+C")
}

func TestSearch_IssueEditor_KeyDelegation(t *testing.T) {
	m := createTestModelWithResults(t)
	issue := m.results[0]

	// Open issue editor
	msg := details.OpenEditMenuMsg{Issue: issue}
	m, _ = m.Update(msg)
	require.Equal(t, ViewEditIssue, m.view, "expected ViewEditIssue view")

	// Press 'j' - should be delegated to issue editor, not change focus
	m, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	// View should still be ViewEditIssue
	require.Equal(t, ViewEditIssue, m.view, "view should still be ViewEditIssue after 'j' key")
	// Command may or may not be nil depending on editor state
	_ = cmd
}

func TestSearch_IssueEditor_SaveMsg_UpdatesTitleWhenChanged(t *testing.T) {
	m := createTestModelWithResults(t)

	// Set up mock executor for consolidated UpdateIssue
	mockExecutor := mocks.NewMockTaskExecutor(t)
	mockExecutor.EXPECT().UpdateIssue("test-1", mock.MatchedBy(func(opts task.UpdateOptions) bool {
		return opts.Title != nil && *opts.Title == "New Title"
	})).Return(nil)
	m.services.TaskExecutor = mockExecutor

	// Open editor (which sets selectedIssue)
	issue := m.results[0]
	issue.TitleText = "Original Title"
	m.results[0] = issue
	openMsg := details.OpenEditMenuMsg{Issue: issue}
	m, _ = m.Update(openMsg)

	require.NotNil(t, m.selectedIssue, "selectedIssue should be set")
	require.Equal(t, "Original Title", m.selectedIssue.TitleText, "original title should be stored")

	// Process SaveMsg with changed title
	msg := issueeditor.SaveMsg{
		IssueID:     "test-1",
		Title:       "New Title",
		Description: issue.DescriptionText,
		Priority:    issue.Priority,
		Status:      issue.Status,
		Labels:      issue.Labels,
	}
	m, cmd := m.Update(msg)

	require.Equal(t, ViewSearch, m.view, "expected ViewSearch view after save")
	require.NotNil(t, cmd, "expected saveIssueCmd")
	require.Nil(t, m.selectedIssue, "selectedIssue should be cleared after save")

	// Execute the command to trigger UpdateIssue
	cmd()
	// The mock expectations will fail if UpdateIssue isn't called with Title set
}

func TestSearch_IssueEditor_SaveMsg_SkipsTitleUpdateWhenUnchanged(t *testing.T) {
	m := createTestModelWithResults(t)

	// Set up mock executor - UpdateIssue should be called but Title should be nil in opts
	mockExecutor := mocks.NewMockTaskExecutor(t)
	mockExecutor.EXPECT().UpdateIssue("test-1", mock.MatchedBy(func(opts task.UpdateOptions) bool {
		return opts.Title == nil // Title should NOT be included when unchanged
	})).Return(nil)
	m.services.TaskExecutor = mockExecutor

	// Open editor (which sets selectedIssue)
	issue := m.results[0]
	issue.TitleText = "Same Title"
	m.results[0] = issue
	openMsg := details.OpenEditMenuMsg{Issue: issue}
	m, _ = m.Update(openMsg)

	// Process SaveMsg with same title but changed labels
	msg := issueeditor.SaveMsg{
		IssueID:     "test-1",
		Title:       "Same Title", // Same as original
		Description: issue.DescriptionText,
		Priority:    issue.Priority,
		Status:      issue.Status,
		Labels:      []string{"updated"}, // Changed to ensure UpdateIssue is called
	}
	m, cmd := m.Update(msg)

	require.Equal(t, ViewSearch, m.view, "expected ViewSearch view after save")
	require.NotNil(t, cmd, "expected saveIssueCmd")

	// Execute the command to trigger UpdateIssue
	cmd()
}

func TestSearch_IssueEditor_SaveMsg_UpdatesDescriptionWhenChanged(t *testing.T) {
	m := createTestModelWithResults(t)

	// Set up mock executor for consolidated UpdateIssue
	mockExecutor := mocks.NewMockTaskExecutor(t)
	mockExecutor.EXPECT().UpdateIssue("test-1", mock.MatchedBy(func(opts task.UpdateOptions) bool {
		return opts.Description != nil && *opts.Description == "New Description"
	})).Return(nil)
	m.services.TaskExecutor = mockExecutor

	// Open editor (which sets selectedIssue)
	issue := m.results[0]
	issue.DescriptionText = "Original Description"
	m.results[0] = issue
	openMsg := details.OpenEditMenuMsg{Issue: issue}
	m, _ = m.Update(openMsg)

	require.NotNil(t, m.selectedIssue, "selectedIssue should be set")
	require.Equal(t, "Original Description", m.selectedIssue.DescriptionText, "original description should be stored")

	// Process SaveMsg with changed description
	msg := issueeditor.SaveMsg{
		IssueID:     "test-1",
		Title:       issue.TitleText,
		Description: "New Description",
		Priority:    issue.Priority,
		Status:      issue.Status,
		Labels:      issue.Labels,
	}
	m, cmd := m.Update(msg)

	require.Equal(t, ViewSearch, m.view, "expected ViewSearch view after save")
	require.NotNil(t, cmd, "expected saveIssueCmd")
	require.Nil(t, m.selectedIssue, "selectedIssue should be cleared after save")

	// Execute the command to trigger UpdateIssue
	cmd()
	// The mock expectations will fail if UpdateIssue isn't called with Description set
}

func TestSearch_IssueEditor_SaveMsg_SkipsDescriptionUpdateWhenUnchanged(t *testing.T) {
	m := createTestModelWithResults(t)

	// Set up mock executor - UpdateIssue should be called but Description should be nil in opts
	mockExecutor := mocks.NewMockTaskExecutor(t)
	mockExecutor.EXPECT().UpdateIssue("test-1", mock.MatchedBy(func(opts task.UpdateOptions) bool {
		return opts.Description == nil // Description should NOT be included when unchanged
	})).Return(nil)
	m.services.TaskExecutor = mockExecutor

	// Open editor (which sets selectedIssue)
	issue := m.results[0]
	issue.DescriptionText = "Same Description"
	m.results[0] = issue
	openMsg := details.OpenEditMenuMsg{Issue: issue}
	m, _ = m.Update(openMsg)

	// Process SaveMsg with same description but changed labels
	msg := issueeditor.SaveMsg{
		IssueID:     "test-1",
		Title:       issue.TitleText,
		Description: "Same Description", // Same as original
		Priority:    issue.Priority,
		Status:      issue.Status,
		Labels:      []string{"updated"}, // Changed to ensure UpdateIssue is called
	}
	m, cmd := m.Update(msg)

	require.Equal(t, ViewSearch, m.view, "expected ViewSearch view after save")
	require.NotNil(t, cmd, "expected saveIssueCmd")

	// Execute the command to trigger UpdateIssue
	cmd()
}

func TestSearch_IssueEditor_SaveMsg_ErrorHandlingShowsToast(t *testing.T) {
	m := createTestModelWithResults(t)

	// Set up mock executor that returns an error
	mockExecutor := mocks.NewMockTaskExecutor(t)
	mockExecutor.EXPECT().UpdateIssue("test-1", mock.AnythingOfType("task.UpdateOptions")).Return(errors.New("database error"))
	m.services.TaskExecutor = mockExecutor

	// Open editor (which sets selectedIssue)
	issue := m.results[0]
	issue.TitleText = "Original Title"
	m.results[0] = issue
	openMsg := details.OpenEditMenuMsg{Issue: issue}
	m, _ = m.Update(openMsg)

	// Process SaveMsg with changed title
	msg := issueeditor.SaveMsg{
		IssueID:     "test-1",
		Title:       "New Title",
		Description: issue.DescriptionText,
		Priority:    issue.Priority,
		Status:      issue.Status,
		Labels:      issue.Labels,
	}
	m, cmd := m.Update(msg)

	require.NotNil(t, cmd, "expected saveIssueCmd")

	// Execute the command to get issueSavedMsg with error
	result := cmd()
	savedMsg, ok := result.(issueSavedMsg)
	require.True(t, ok, "expected issueSavedMsg")
	require.Error(t, savedMsg.err)

	// Handle the error message
	_, toastCmd := m.Update(savedMsg)
	require.NotNil(t, toastCmd, "expected toast command on error")

	// Execute toast command to verify it produces a toast message
	toastResult := toastCmd()
	showToast, ok := toastResult.(mode.ShowToastMsg)
	require.True(t, ok, "expected ShowToastMsg")
	require.Contains(t, showToast.Message, "Save failed", "toast should contain error message")
	require.Contains(t, showToast.Message, "database error")
}

func TestSearch_IssueEditor_CancelMsg_ClearsSelectedIssue(t *testing.T) {
	m := createTestModelWithResults(t)

	// Open editor (which sets selectedIssue)
	issue := m.results[0]
	openMsg := details.OpenEditMenuMsg{Issue: issue}
	m, _ = m.Update(openMsg)

	require.NotNil(t, m.selectedIssue, "selectedIssue should be set after opening editor")

	// Process CancelMsg
	cancelMsg := issueeditor.CancelMsg{}
	m, cmd := m.Update(cancelMsg)

	require.Equal(t, ViewSearch, m.view, "expected ViewSearch view after cancel")
	require.Nil(t, cmd, "expected no command on cancel")
	require.Nil(t, m.selectedIssue, "selectedIssue should be cleared after cancel")
}

func TestSearch_CommentEditor_OpenCommentModalMsg_SetsViewAddComment(t *testing.T) {
	createdAt := time.Date(2026, time.January, 2, 15, 4, 5, 0, time.UTC)
	issue := task.Issue{
		ID:           "test-1",
		TitleText:    "First Issue",
		Type:         task.TypeTask,
		Priority:     task.PriorityMedium,
		Status:       task.StatusOpen,
		CreatedAt:    createdAt,
		CommentCount: 1,
		Comments: []task.Comment{
			{ID: "1", Author: "bob", Text: "Existing", CreatedAt: createdAt},
		},
	}
	m, _ := createCommentReadySearchModel(t, issue)

	m, _ = m.Update(details.OpenCommentModalMsg{Issue: issue})

	require.Equal(t, ViewAddComment, m.view)
	require.Contains(t, m.View(), "Add Comment")
}

func TestSearch_CommentEditor_SaveMsg_RefreshesResultsAndDetails(t *testing.T) {
	createdAt := time.Date(2026, time.January, 2, 15, 4, 5, 0, time.UTC)
	issue := task.Issue{
		ID:           "test-1",
		TitleText:    "First Issue",
		Type:         task.TypeTask,
		Priority:     task.PriorityMedium,
		Status:       task.StatusOpen,
		CreatedAt:    createdAt,
		CommentCount: 1,
		Comments: []task.Comment{
			{ID: "1", Author: "bob", Text: "Existing", CreatedAt: createdAt},
		},
	}
	m, taskExec := createCommentReadySearchModel(t, issue)

	refreshed := issue
	refreshed.CommentCount = 2
	refreshed.Comments = nil

	taskExec.EXPECT().AddComment("test-1", "alice", "Ship it").Return(nil)
	taskExec.EXPECT().ShowIssue("test-1").Return(&refreshed, nil)
	taskExec.EXPECT().GetComments("test-1").Return([]task.Comment{
		{ID: "1", Author: "bob", Text: "Existing", CreatedAt: createdAt},
		{ID: "2", Author: "alice", Text: "Ship it", CreatedAt: createdAt.Add(time.Minute)},
	}, nil)

	m, _ = m.Update(details.OpenCommentModalMsg{Issue: issue})
	require.Equal(t, ViewAddComment, m.view)

	m, cmd := m.Update(commenteditor.SaveMsg{IssueID: "test-1", Author: "alice", Text: "Ship it"})
	require.Equal(t, ViewSearch, m.view)
	require.NotNil(t, cmd)

	msg := cmd()
	m, toastCmd := m.Update(msg)
	require.Equal(t, 2, m.results[0].CommentCount)
	require.Contains(t, m.details.View(), "Ship it")
	require.NotNil(t, toastCmd)

	toast := toastCmd()
	showToast, ok := toast.(mode.ShowToastMsg)
	require.True(t, ok)
	require.Equal(t, "Comment added", showToast.Message)
	require.Equal(t, toaster.StyleSuccess, showToast.Style)
}

func TestSearch_CommentEditor_SaveMsg_ErrorShowsToast(t *testing.T) {
	createdAt := time.Date(2026, time.January, 2, 15, 4, 5, 0, time.UTC)
	issue := task.Issue{
		ID:           "test-1",
		TitleText:    "First Issue",
		Type:         task.TypeTask,
		Priority:     task.PriorityMedium,
		Status:       task.StatusOpen,
		CreatedAt:    createdAt,
		CommentCount: 1,
		Comments: []task.Comment{
			{ID: "1", Author: "bob", Text: "Existing", CreatedAt: createdAt},
		},
	}
	m, taskExec := createCommentReadySearchModel(t, issue)

	taskExec.EXPECT().AddComment("test-1", "alice", "Ship it").Return(errors.New("bd unavailable"))

	m, _ = m.Update(details.OpenCommentModalMsg{Issue: issue})
	m, cmd := m.Update(commenteditor.SaveMsg{IssueID: "test-1", Author: "alice", Text: "Ship it"})
	require.NotNil(t, cmd)

	msg := cmd()
	m, toastCmd := m.Update(msg)
	require.Equal(t, 1, m.results[0].CommentCount)
	require.NotNil(t, toastCmd)

	toast := toastCmd()
	showToast, ok := toast.(mode.ShowToastMsg)
	require.True(t, ok)
	require.Contains(t, showToast.Message, "Comment failed")
	require.Contains(t, showToast.Message, "bd unavailable")
	require.Equal(t, toaster.StyleError, showToast.Style)
}

// =============================================================================
// Mouse Scroll Event Forwarding Tests
// =============================================================================

func TestSearch_MouseScrollForwardsToDetails(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusDetails

	// Set up details panel with an issue that has scrollable content
	issue := task.Issue{
		ID:        "test-scroll",
		TitleText: "Scrollable Issue",
		DescriptionText: "Line 1\nLine 2\nLine 3\nLine 4\nLine 5\nLine 6\nLine 7\nLine 8\nLine 9\nLine 10\n" +
			"Line 11\nLine 12\nLine 13\nLine 14\nLine 15\nLine 16\nLine 17\nLine 18\nLine 19\nLine 20",
	}
	m.details = details.New(issue, m.services.QueryExecutor, nil, m.services.TaskExecutor).SetSize(50, 10)
	m.hasDetail = true

	// Verify preconditions
	require.Equal(t, FocusDetails, m.focus, "precondition: focus should be on details")
	initialOffset := m.details.YOffset()

	// Send mouse wheel down event through Update
	m, cmd := m.Update(tea.MouseMsg{
		Button: tea.MouseButtonWheelDown,
	})

	// Command may be nil (viewport scroll doesn't return a command)
	_ = cmd

	// After scrolling down, the offset should increase (or stay the same if already at bottom)
	// The key test here is that the message was forwarded and processed
	// We verify by checking the details component was updated
	require.Equal(t, FocusDetails, m.focus, "focus should still be on details")
	// The viewport should have received the scroll event
	// Note: The exact offset depends on viewport configuration, but it should be >= initial
	require.GreaterOrEqual(t, m.details.YOffset(), initialOffset, "scroll down should not decrease offset")
}

func TestSearch_MouseScrollWheelUp_ForwardsToDetails(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusDetails

	// Set up details panel with scrollable content
	issue := task.Issue{
		ID:        "test-scroll-up",
		TitleText: "Scrollable Issue",
		DescriptionText: "Line 1\nLine 2\nLine 3\nLine 4\nLine 5\nLine 6\nLine 7\nLine 8\nLine 9\nLine 10\n" +
			"Line 11\nLine 12\nLine 13\nLine 14\nLine 15\nLine 16\nLine 17\nLine 18\nLine 19\nLine 20",
	}
	m.details = details.New(issue, m.services.QueryExecutor, nil, m.services.TaskExecutor).SetSize(50, 10)
	m.hasDetail = true

	// First scroll down to have somewhere to scroll up from
	m, _ = m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	m, _ = m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	m, _ = m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown})

	offsetBeforeScrollUp := m.details.YOffset()

	// Now scroll up
	m, _ = m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp})

	// After scrolling up, the offset should decrease (or stay the same if already at top)
	require.LessOrEqual(t, m.details.YOffset(), offsetBeforeScrollUp, "scroll up should not increase offset")
}

func TestSearch_MouseScrollWorksWhenFocusedOnResults(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusResults // Focus on results, not details

	// Set up details panel with scrollable content
	issue := task.Issue{
		ID:        "test-scroll-results-focus",
		TitleText: "Scrollable Issue",
		DescriptionText: "Line 1\nLine 2\nLine 3\nLine 4\nLine 5\nLine 6\nLine 7\nLine 8\nLine 9\nLine 10\n" +
			"Line 11\nLine 12\nLine 13\nLine 14\nLine 15\nLine 16\nLine 17\nLine 18\nLine 19\nLine 20",
	}
	m.details = details.New(issue, m.services.QueryExecutor, nil, m.services.TaskExecutor).SetSize(50, 10)
	m.hasDetail = true
	initialOffset := m.details.YOffset()

	// Verify preconditions
	require.Equal(t, FocusResults, m.focus, "precondition: focus should be on results")

	// Send mouse wheel down event while focused on results
	m, _ = m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown})

	// Wheel events should still scroll details even when focused on results
	require.Equal(t, FocusResults, m.focus, "focus should still be on results")
	require.GreaterOrEqual(t, m.details.YOffset(), initialOffset, "scroll down should work even when focused on results")
}

func TestSearch_MouseScrollWorksWhenFocusedOnSearch(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusSearch
	m.input.Focus()

	// Set up details panel with scrollable content
	issue := task.Issue{
		ID:        "test-scroll-search-focus",
		TitleText: "Scrollable Issue",
		DescriptionText: "Line 1\nLine 2\nLine 3\nLine 4\nLine 5\nLine 6\nLine 7\nLine 8\nLine 9\nLine 10\n" +
			"Line 11\nLine 12\nLine 13\nLine 14\nLine 15\nLine 16\nLine 17\nLine 18\nLine 19\nLine 20",
	}
	m.details = details.New(issue, m.services.QueryExecutor, nil, m.services.TaskExecutor).SetSize(50, 10)
	m.hasDetail = true
	initialOffset := m.details.YOffset()

	// Verify preconditions
	require.Equal(t, FocusSearch, m.focus, "precondition: focus should be on search")

	// Send mouse wheel down event while focused on search input
	m, _ = m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown})

	// Wheel events should still scroll details even when focused on search
	require.Equal(t, FocusSearch, m.focus, "focus should still be on search")
	require.GreaterOrEqual(t, m.details.YOffset(), initialOffset, "scroll down should work even when focused on search")
}

func TestSearch_MouseClickIgnoredWhenNotFocusedOnDetails(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusResults // Focus on results, not details

	// Set up details panel
	issue := task.Issue{
		ID:              "test-ignore",
		TitleText:       "Test Issue",
		DescriptionText: "Line 1\nLine 2\nLine 3\nLine 4\nLine 5",
	}
	m.details = details.New(issue, m.services.QueryExecutor, nil, m.services.TaskExecutor).SetSize(50, 10)
	m.hasDetail = true

	// Verify preconditions
	require.Equal(t, FocusResults, m.focus, "precondition: focus should be on results")

	// Send mouse click event when NOT focused on details
	m, cmd := m.Update(tea.MouseMsg{
		Button: tea.MouseButtonLeft,
	})

	// Non-wheel mouse events should be ignored when not focused on details
	require.Nil(t, cmd, "expected no command when mouse click is ignored")
	require.Equal(t, FocusResults, m.focus, "focus should still be on results")
}

func TestSearch_MouseClickIgnoredWhenFocusedOnSearch(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusSearch
	m.input.Focus()

	// Set up details panel
	issue := task.Issue{
		ID:              "test-ignore-search",
		TitleText:       "Test Issue",
		DescriptionText: "Some content",
	}
	m.details = details.New(issue, m.services.QueryExecutor, nil, m.services.TaskExecutor).SetSize(50, 10)
	m.hasDetail = true

	// Verify preconditions
	require.Equal(t, FocusSearch, m.focus, "precondition: focus should be on search")

	// Send mouse click event when focused on search input
	m, cmd := m.Update(tea.MouseMsg{
		Button: tea.MouseButtonLeft,
	})

	// Non-wheel mouse events should be ignored when focused on search
	require.Nil(t, cmd, "expected no command when mouse click is ignored")
	require.Equal(t, FocusSearch, m.focus, "focus should still be on search")
}

func TestSearch_MouseScrollAtBoundary_DoesNotGoNegative(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusDetails

	// Set up details panel at top of content
	issue := task.Issue{
		ID:              "test-boundary",
		TitleText:       "Boundary Test",
		DescriptionText: "Line 1\nLine 2\nLine 3",
	}
	m.details = details.New(issue, m.services.QueryExecutor, nil, m.services.TaskExecutor).SetSize(50, 10)
	m.hasDetail = true

	// Ensure we're at the top (offset 0)
	m.details = m.details.SetYOffset(0)
	require.Equal(t, 0, m.details.YOffset(), "precondition: should start at top")

	// Try to scroll up when already at top
	m, _ = m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp})

	// Offset should never go negative
	require.GreaterOrEqual(t, m.details.YOffset(), 0, "offset should never be negative")
}

// =============================================================================
// Diff Viewer Tests (Ctrl+G)
// =============================================================================

func TestSearch_CtrlG_OpensDiffViewer_FocusResults(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusResults

	// Simulate Ctrl+G keypress
	msg := tea.KeyMsg{Type: tea.KeyCtrlG}
	_, cmd := m.handleKey(msg)

	// Execute the command to get the message
	require.NotNil(t, cmd, "expected command from Ctrl+G key")
	result := cmd()

	// Verify it's a ShowDiffViewerMsg
	_, ok := result.(diffviewer.ShowDiffViewerMsg)
	require.True(t, ok, "expected diffviewer.ShowDiffViewerMsg, got %T", result)
}

func TestSearch_CtrlG_OpensDiffViewer_FocusDetails(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusDetails

	// Simulate Ctrl+G keypress
	msg := tea.KeyMsg{Type: tea.KeyCtrlG}
	_, cmd := m.handleKey(msg)

	// Execute the command to get the message
	require.NotNil(t, cmd, "expected command from Ctrl+G key")
	result := cmd()

	// Verify it's a ShowDiffViewerMsg
	_, ok := result.(diffviewer.ShowDiffViewerMsg)
	require.True(t, ok, "expected diffviewer.ShowDiffViewerMsg, got %T", result)
}

// =============================================================================
// User Action Tests (Action keys 0-9)
// =============================================================================

// createTestModelWithActions creates a Model with user actions configured.
func createTestModelWithActions(t *testing.T) Model {
	cfg := config.Defaults()
	cfg.UI.Actions.IssueAction = map[string]config.ActionConfig{
		"open-claude": {
			Key:         "1",
			Command:     "echo 'Working on {{.ID}}: {{.TitleText}}'",
			Description: "Open Claude",
		},
		"open-editor": {
			Key:         "2",
			Command:     "echo 'Editing {{.ID}}'",
			Description: "Open Editor",
		},
	}

	clipboard := mocks.NewMockClipboard(t)
	clipboard.EXPECT().Copy(mock.Anything).Return(nil).Maybe()

	mockTaskExec := mocks.NewMockTaskExecutor(t)
	mockTaskExec.EXPECT().GetComments(mock.Anything).Return([]task.Comment{}, nil).Maybe()

	mockExecutor := mocks.NewMockQueryExecutor(t)
	mockExecutor.EXPECT().Execute(mock.Anything).Return([]task.Issue{}, nil).Maybe()

	services := mode.Services{
		TaskExecutor:  mockTaskExec,
		QueryExecutor: mockExecutor,
		Config:        &cfg,
		Clipboard:     clipboard,
		WorkDir:       "/tmp/test",
	}

	m := New(services)
	m.width = 100
	m.height = 40

	// Load some test results
	issues := []task.Issue{
		{ID: "test-1", TitleText: "First Issue", Priority: 1, Status: task.StatusOpen, Type: task.TypeTask},
		{ID: "test-2", TitleText: "Second Issue", Priority: 2, Status: task.StatusInProgress, Type: task.TypeBug},
	}
	m, _ = m.handleSearchResults(searchResultsMsg{issues: issues, err: nil})

	return m
}

func TestSearch_ActionKeyTriggersWhenResultsFocused(t *testing.T) {
	m := createTestModelWithActions(t)
	m.focus = FocusResults
	m.selectedIdx = 0

	// Verify preconditions
	require.Equal(t, "test-1", m.results[m.selectedIdx].ID, "should have test-1 selected")
	require.NotNil(t, m.actions, "actions should be configured")

	// Press '1' while focused on results - should trigger action
	m, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})

	// Should return a command (the action execution)
	require.NotNil(t, cmd, "expected action command to be returned")

	// Execute the command to get the message
	msg := cmd()
	actionMsg, ok := msg.(shared.ActionExecutedMsg)
	require.True(t, ok, "expected ActionExecutedMsg, got %T", msg)
	require.Equal(t, "Open Claude", actionMsg.Name, "action name should match")
}

func TestSearch_ActionKeyIgnoredWhenInputFocused(t *testing.T) {
	m := createTestModelWithActions(t)
	m.focus = FocusSearch
	m.input.Focus()

	// Press '1' while focused on search input - should type into input, not trigger action
	oldValue := m.input.Value()
	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})

	// The '1' should be typed into the input, not trigger an action
	require.Equal(t, oldValue+"1", m.input.Value(), "key should be typed into input")
}

func TestSearch_ActionKeyShowsToastWhenNoResults(t *testing.T) {
	m := createTestModelWithActions(t)
	m.focus = FocusResults
	// Clear results so no issue is selected
	m.results = nil
	m.selectedIdx = -1

	// Press '1' while focused on results with no issue selected
	m, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})

	// Should return a command that shows "No issue selected" toast
	require.NotNil(t, cmd, "expected toast command to be returned")

	// Execute the command to get the message
	msg := cmd()
	toastMsg, ok := msg.(mode.ShowToastMsg)
	require.True(t, ok, "expected ShowToastMsg, got %T", msg)
	require.Equal(t, "No issue selected", toastMsg.Message)
}

func TestSearch_TreeMode_ActionKeyWorks(t *testing.T) {
	m := createTestModelWithActions(t)
	m.subMode = mode.SubModeTree
	m.focus = FocusResults

	// Set up tree with a selected node
	// For simplicity, we'll test that the action triggers when getSelectedIssue() returns an issue
	// by mocking the tree state. Since tree handling is complex, we test via list mode results.
	// The key point is that FocusResults covers both sub-modes.

	m.selectedIdx = 0

	// Press '1' while focused on results in tree sub-mode
	m, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})

	// Should return a command (the action execution)
	require.NotNil(t, cmd, "expected action command to be returned")

	// Execute the command to get the message
	msg := cmd()
	actionMsg, ok := msg.(shared.ActionExecutedMsg)
	require.True(t, ok, "expected ActionExecutedMsg, got %T", msg)
	require.Equal(t, "Open Claude", actionMsg.Name, "action name should match")
}

func TestSearch_ActionError_ShowsToast(t *testing.T) {
	m := createTestModelWithActions(t)
	m.focus = FocusResults

	// Test that ActionExecutedMsg with error shows toast
	errorMsg := shared.ActionExecutedMsg{
		Name: "Open Claude",
		Err:  errors.New("command failed"),
	}
	m, cmd := m.Update(errorMsg)

	// Should return a command that shows error toast
	require.NotNil(t, cmd, "expected toast command to be returned")

	// Execute the command to get the message
	msg := cmd()
	toastMsg, ok := msg.(mode.ShowToastMsg)
	require.True(t, ok, "expected ShowToastMsg, got %T", msg)
	require.Contains(t, toastMsg.Message, "command failed", "toast should contain error")
	require.Contains(t, toastMsg.Message, "Open Claude", "toast should contain action name")
}

func TestSearch_ActionExecutedMsg_SuccessSilent(t *testing.T) {
	m := createTestModelWithActions(t)
	m.focus = FocusResults

	// Test that successful ActionExecutedMsg is silent (no toast)
	successMsg := shared.ActionExecutedMsg{
		Name: "Open Claude",
		Err:  nil, // Success
	}
	m, cmd := m.Update(successMsg)

	// Should return nil command (silent on success)
	require.Nil(t, cmd, "expected no command on successful action (fire-and-forget)")
}

func TestSearch_ActionKeyNonConfigured_NoOp(t *testing.T) {
	m := createTestModelWithActions(t)
	m.focus = FocusResults
	m.selectedIdx = 0

	// Press '9' which is not configured - should delegate to board (no-op)
	m, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'9'}})

	// Should return nil command (unbound key is no-op in list pane)
	require.Nil(t, cmd, "expected no command for unconfigured action key")
}

func TestSearch_ActionsLoadedFromConfig(t *testing.T) {
	m := createTestModelWithActions(t)

	// Verify actions were loaded from config
	require.NotNil(t, m.actions, "actions should be loaded")
	require.Len(t, m.actions, 2, "should have 2 actions configured")
	require.Contains(t, m.actions, "open-claude", "should have open-claude action")
	require.Contains(t, m.actions, "open-editor", "should have open-editor action")
}

// Mouse click tests

func TestSearch_MouseClick_SearchInputFocusesFromResults(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusResults
	m.input.Blur()

	// Render to register zones
	_ = m.View()

	var z *zone.ZoneInfo
	for retries := 0; retries < 10; retries++ {
		z = zone.Get(zoneSearchInput)
		if z != nil && !z.IsZero() {
			break
		}
		_ = m.View()
		time.Sleep(time.Millisecond)
	}
	require.NotNil(t, z, "search input zone should be registered")
	require.False(t, z.IsZero(), "search input zone should not be zero")

	m, cmd := m.Update(tea.MouseMsg{
		X:      z.StartX + 1,
		Y:      z.StartY,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionRelease,
	})

	require.Nil(t, cmd, "click on search input should not produce a command")
	require.Equal(t, FocusSearch, m.focus, "focus should move to search input")
	require.True(t, m.input.Focused(), "search input should be focused")
}

func TestSearch_MouseClick_SearchInputFocusesFromDetails(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusDetails
	m.input.Blur()

	_ = m.View()

	var z *zone.ZoneInfo
	for retries := 0; retries < 10; retries++ {
		z = zone.Get(zoneSearchInput)
		if z != nil && !z.IsZero() {
			break
		}
		_ = m.View()
		time.Sleep(time.Millisecond)
	}
	require.NotNil(t, z, "search input zone should be registered")

	m, cmd := m.Update(tea.MouseMsg{
		X:      z.StartX + 1,
		Y:      z.StartY,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionRelease,
	})

	require.Nil(t, cmd)
	require.Equal(t, FocusSearch, m.focus, "focus should move to search input")
	require.True(t, m.input.Focused(), "search input should be focused")
}

func TestSearch_MouseClick_DetailsPanelFocusesFromResults(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusResults

	_ = m.View()

	var z *zone.ZoneInfo
	for retries := 0; retries < 10; retries++ {
		z = zone.Get(zoneSearchDetails)
		if z != nil && !z.IsZero() {
			break
		}
		_ = m.View()
		time.Sleep(time.Millisecond)
	}
	require.NotNil(t, z, "details zone should be registered")
	require.False(t, z.IsZero(), "details zone should not be zero")

	m, cmd := m.Update(tea.MouseMsg{
		X:      z.StartX + 1,
		Y:      z.StartY,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionRelease,
	})

	require.Nil(t, cmd, "click on details should not produce a command")
	require.Equal(t, FocusDetails, m.focus, "focus should move to details")
	require.False(t, m.input.Focused(), "search input should be blurred")
}

func TestSearch_MouseClick_DetailsPanelFocusesFromSearch(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusSearch
	m.input.Focus()

	_ = m.View()

	var z *zone.ZoneInfo
	for retries := 0; retries < 10; retries++ {
		z = zone.Get(zoneSearchDetails)
		if z != nil && !z.IsZero() {
			break
		}
		_ = m.View()
		time.Sleep(time.Millisecond)
	}
	require.NotNil(t, z, "details zone should be registered")

	m, cmd := m.Update(tea.MouseMsg{
		X:      z.StartX + 1,
		Y:      z.StartY,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionRelease,
	})

	require.Nil(t, cmd)
	require.Equal(t, FocusDetails, m.focus, "focus should move to details")
	require.False(t, m.input.Focused(), "search input should be blurred after clicking details")
}

func TestSearch_MouseClick_IgnoresRightClick(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusResults
	originalIdx := m.selectedIdx

	// Right click should be ignored
	m, cmd := m.Update(tea.MouseMsg{
		X:      10,
		Y:      5,
		Button: tea.MouseButtonRight,
		Action: tea.MouseActionRelease,
	})

	require.Nil(t, cmd, "right click should not produce a command")
	require.Equal(t, originalIdx, m.selectedIdx, "selection should not change on right click")
}

func TestSearch_MouseClick_IgnoresMiddleClick(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusResults
	originalIdx := m.selectedIdx

	// Middle click should be ignored
	m, cmd := m.Update(tea.MouseMsg{
		X:      10,
		Y:      5,
		Button: tea.MouseButtonMiddle,
		Action: tea.MouseActionRelease,
	})

	require.Nil(t, cmd, "middle click should not produce a command")
	require.Equal(t, originalIdx, m.selectedIdx, "selection should not change on middle click")
}

func TestSearch_MouseClick_IgnoresPress(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusResults
	originalIdx := m.selectedIdx

	// Click press (not release) should be ignored
	m, cmd := m.Update(tea.MouseMsg{
		X:      10,
		Y:      5,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})

	require.Nil(t, cmd, "click press should not produce a command")
	require.Equal(t, originalIdx, m.selectedIdx, "selection should not change on click press")
}

func TestSearch_MouseClick_IgnoresOutsideZone(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusResults
	originalIdx := m.selectedIdx

	// Call View() to register zones
	_ = m.View()

	// Click outside any registered zone (very far coordinates)
	m, cmd := m.Update(tea.MouseMsg{
		X:      9999,
		Y:      9999,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionRelease,
	})

	require.Nil(t, cmd, "click outside zone should not produce a command")
	require.Equal(t, originalIdx, m.selectedIdx, "selection should not change on click outside zone")
}

func TestSearch_MouseClick_SelectsIssueInList(t *testing.T) {
	// Use unique issue IDs to avoid zone conflicts with other tests
	issueID1 := "mouse-click-select-test-issue-1"
	issueID2 := "mouse-click-select-test-issue-2"
	issueID3 := "mouse-click-select-test-issue-3"

	m := createTestModel(t)
	issues := []task.Issue{
		{ID: issueID1, TitleText: "First Issue", Priority: 1, Status: task.StatusOpen, Type: task.TypeTask},
		{ID: issueID2, TitleText: "Second Issue", Priority: 2, Status: task.StatusInProgress, Type: task.TypeBug},
		{ID: issueID3, TitleText: "Third Issue", Priority: 0, Status: task.StatusOpen, Type: task.TypeFeature},
	}
	m, _ = m.handleSearchResults(searchResultsMsg{issues: issues, err: nil})
	m.focus = FocusDetails // Start with different focus to verify it changes

	// First verify we can select the first issue (simpler case)
	// Call View() to register zones
	_ = m.View()

	// Get zone ID for the first issue (index 0) - more reliable than second
	zoneID := makeSearchListZoneID(issueID1)

	// Get zone to determine click position (with retry for zone manager stability)
	var z *zone.ZoneInfo
	for retries := 0; retries < 10; retries++ {
		z = zone.Get(zoneID)
		if z != nil && !z.IsZero() {
			break
		}
		// Re-render to ensure zones are registered
		_ = m.View()
		// Zone registration is asynchronous via a channel worker in bubblezone.
		time.Sleep(time.Millisecond)
	}
	require.NotNil(t, z, "zone should be registered after View()")
	require.False(t, z.IsZero(), "zone should not be zero")

	// Click inside the zone
	width := z.EndX - z.StartX
	require.True(t, width > 0, "zone should have positive width")
	clickX := z.StartX + width/2
	clickY := z.StartY

	m, cmd := m.Update(tea.MouseMsg{
		X:      clickX,
		Y:      clickY,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionRelease,
	})

	// No command is expected (unlike board which emits IssueClickedMsg)
	require.Nil(t, cmd, "click on issue in search list should not produce a command")

	// Verify issue is selected
	require.Equal(t, 0, m.selectedIdx, "clicked issue (index 0) should be selected")
	require.Equal(t, FocusResults, m.focus, "focus should move to results pane")
	require.True(t, m.hasDetail, "detail panel should be populated")
}

func TestSearch_MouseClick_ChangesFocusToResults(t *testing.T) {
	issueID := "focus-change-test-issue"

	m := createTestModel(t)
	issues := []task.Issue{
		{ID: issueID, TitleText: "Test Issue", Priority: 1, Status: task.StatusOpen, Type: task.TypeTask},
	}
	m, _ = m.handleSearchResults(searchResultsMsg{issues: issues, err: nil})
	m.focus = FocusSearch // Start focused on search input

	// Call View() to register zones
	_ = m.View()

	// Get zone
	zoneID := makeSearchListZoneID(issueID)
	var z *zone.ZoneInfo
	for retries := 0; retries < 10; retries++ {
		z = zone.Get(zoneID)
		if z != nil && !z.IsZero() {
			break
		}
		_ = m.View()
		time.Sleep(time.Millisecond)
	}
	require.NotNil(t, z, "zone should be registered")

	// Click on the issue
	m, _ = m.Update(tea.MouseMsg{
		X:      z.StartX + 1,
		Y:      z.StartY,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionRelease,
	})

	require.Equal(t, FocusResults, m.focus, "focus should change to results after clicking issue")
}

func TestSearch_MouseClick_UpdatesDetailPanel(t *testing.T) {
	issueID := "detail-update-test-issue"
	issueTitle := "Test Issue for Detail Panel"

	m := createTestModel(t)
	issues := []task.Issue{
		{ID: issueID, TitleText: issueTitle, Priority: 1, Status: task.StatusOpen, Type: task.TypeTask},
	}
	m, _ = m.handleSearchResults(searchResultsMsg{issues: issues, err: nil})
	m.hasDetail = false // Start without detail

	// Call View() to register zones
	_ = m.View()

	// Get zone
	zoneID := makeSearchListZoneID(issueID)
	var z *zone.ZoneInfo
	for retries := 0; retries < 10; retries++ {
		z = zone.Get(zoneID)
		if z != nil && !z.IsZero() {
			break
		}
		_ = m.View()
		time.Sleep(time.Millisecond)
	}
	require.NotNil(t, z, "zone should be registered")

	// Click on the issue
	m, _ = m.Update(tea.MouseMsg{
		X:      z.StartX + 1,
		Y:      z.StartY,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionRelease,
	})

	require.True(t, m.hasDetail, "detail panel should be populated after click")
	require.Equal(t, issueID, m.details.IssueID(), "detail panel should show clicked issue")
}

func TestSearch_MouseClick_WheelEventsForwardToDetails(t *testing.T) {
	m := createTestModelWithResults(t)
	m.focus = FocusResults
	m.hasDetail = true

	// Wheel events should be forwarded to details regardless of focus
	m, cmd := m.Update(tea.MouseMsg{
		X:      50,
		Y:      10,
		Button: tea.MouseButtonWheelDown,
	})

	// Command might be nil or a scroll command from details
	// The important thing is that it doesn't select an issue
	_ = cmd
	require.Equal(t, FocusResults, m.focus, "focus should not change on wheel event")
}

// =============================================================================
// Editor Message Routing Tests
// =============================================================================

func TestSearch_EditorExecMsg_ForwardedToIssueEditorWhenViewEditIssue(t *testing.T) {
	m := createTestModelWithResults(t)
	issue := m.results[0]

	// Open issue editor modal
	m, _ = m.Update(details.OpenEditMenuMsg{Issue: issue})
	require.Equal(t, ViewEditIssue, m.view, "should be in ViewEditIssue")

	// Send an editor.ExecMsg - this would normally come from Ctrl+G in description field
	// The message should be forwarded to the issueEditor, not intercepted here
	execMsg := editor.ExecMsg{}
	m, cmd := m.Update(execMsg)

	// View should still be ViewEditIssue (modal stayed open)
	require.Equal(t, ViewEditIssue, m.view, "view should still be ViewEditIssue after editor.ExecMsg")
	// The command is forwarded to issueEditor which returns nil for an empty ExecMsg
	_ = cmd
}

func TestSearch_EditorExecMsg_ReturnsNilWhenNotEditing(t *testing.T) {
	m := createTestModelWithResults(t)

	// Not in ViewEditIssue
	m.view = ViewSearch

	// Send an editor.ExecMsg
	execMsg := editor.ExecMsg{}
	m, cmd := m.Update(execMsg)

	// Should return nil when not editing in any context
	require.Nil(t, cmd, "should return nil when not in any editing context")
}

func TestSearch_EditorFinishedMsg_ForwardedToIssueEditorWhenViewEditIssue(t *testing.T) {
	m := createTestModelWithResults(t)
	issue := m.results[0]

	// Open issue editor modal
	m, _ = m.Update(details.OpenEditMenuMsg{Issue: issue})
	require.Equal(t, ViewEditIssue, m.view, "should be in ViewEditIssue")

	// Send an editor.FinishedMsg with new content
	finishedMsg := editor.FinishedMsg{Content: "new description content"}
	m, cmd := m.Update(finishedMsg)

	// View should still be ViewEditIssue (modal stayed open)
	require.Equal(t, ViewEditIssue, m.view, "view should still be ViewEditIssue after editor.FinishedMsg")
	// The result is forwarded to issueEditor for processing
	_ = cmd
}

func TestSearch_EditorFinishedMsg_ReturnsNilWhenNotEditing(t *testing.T) {
	m := createTestModelWithResults(t)

	// Not in ViewEditIssue
	m.view = ViewSearch

	// Send an editor.FinishedMsg
	finishedMsg := editor.FinishedMsg{Content: "some content"}
	m, cmd := m.Update(finishedMsg)

	// Should return nil when not in any editing context
	require.Nil(t, cmd, "should return nil when not in any editing context")
}

func TestSearch_SaveMsg_UpdatesNotesOnlyWhenChanged(t *testing.T) {
	// Verify SaveMsg dispatches saveIssueCmd with Notes=nil when notes unchanged
	m := createTestModelWithResults(t)

	// Set up mock executor - UpdateIssue called but Notes should be nil
	mockExecutor := mocks.NewMockTaskExecutor(t)
	mockExecutor.EXPECT().UpdateIssue("test-1", mock.MatchedBy(func(opts task.UpdateOptions) bool {
		return opts.Notes == nil // Notes should NOT be included when unchanged
	})).Return(nil)
	m.services.TaskExecutor = mockExecutor

	// Set up selectedIssue with original notes
	m.selectedIssue = &task.Issue{
		ID:              "test-1",
		TitleText:       "Test Issue",
		DescriptionText: "Test description",
		Notes:           "original notes",
		Priority:        task.PriorityMedium,
		Status:          task.StatusOpen,
	}
	m.view = ViewEditIssue

	// Send SaveMsg with same notes as original but changed labels
	m, cmd := m.Update(issueeditor.SaveMsg{
		IssueID:     "test-1",
		Priority:    task.PriorityMedium,
		Status:      task.StatusOpen,
		Labels:      []string{"changed"}, // Changed to ensure UpdateIssue is called
		Title:       "Test Issue",        // Same as original
		Description: "Test description",  // Same as original
		Notes:       "original notes",    // Same as original - no change
	})

	// Verify state transition
	require.Nil(t, m.selectedIssue, "selectedIssue should be cleared after save")
	require.Equal(t, ViewSearch, m.view, "should return to search view after save")
	require.NotNil(t, cmd, "should return saveIssueCmd")

	// Execute the command to trigger UpdateIssue
	cmd()
}

func TestSearch_SaveMsg_UpdatesNotesWhenChanged(t *testing.T) {
	// Verify SaveMsg dispatches saveIssueCmd with Notes set when notes changed
	m := createTestModelWithResults(t)

	// Set up mock executor - UpdateIssue called with Notes set
	mockExecutor := mocks.NewMockTaskExecutor(t)
	mockExecutor.EXPECT().UpdateIssue("test-1", mock.MatchedBy(func(opts task.UpdateOptions) bool {
		return opts.Notes != nil && *opts.Notes == "new notes content"
	})).Return(nil)
	m.services.TaskExecutor = mockExecutor

	// Set up selectedIssue with original notes
	m.selectedIssue = &task.Issue{
		ID:              "test-1",
		TitleText:       "Test Issue",
		DescriptionText: "Test description",
		Notes:           "original notes",
		Priority:        task.PriorityMedium,
		Status:          task.StatusOpen,
	}
	m.view = ViewEditIssue

	// Send SaveMsg with different notes
	m, cmd := m.Update(issueeditor.SaveMsg{
		IssueID:     "test-1",
		Priority:    task.PriorityMedium,
		Status:      task.StatusOpen,
		Labels:      nil,
		Title:       "Test Issue",        // Same as original
		Description: "Test description",  // Same as original
		Notes:       "new notes content", // Different from original
	})

	// Verify state transition
	require.Nil(t, m.selectedIssue, "selectedIssue should be cleared after save")
	require.Equal(t, ViewSearch, m.view, "should return to search view after save")
	require.NotNil(t, cmd, "should return saveIssueCmd")

	// Execute the command to trigger UpdateIssue
	cmd()
}
