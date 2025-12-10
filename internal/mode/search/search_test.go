package search

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"

	"perles/internal/beads"
	"perles/internal/config"
	"perles/internal/mode"
	"perles/internal/mode/shared"
	"perles/internal/ui/details"
)

// createTestModel creates a minimal Model for testing state transitions.
// It does not require a database connection.
func createTestModel() Model {
	cfg := config.Defaults()
	services := mode.Services{
		Config:    &cfg,
		Clipboard: shared.MockClipboard{},
	}

	m := New(services)
	m.width = 100
	m.height = 40
	return m
}

// createTestModelWithResults creates a Model with some test results loaded.
func createTestModelWithResults() Model {
	m := createTestModel()
	issues := []beads.Issue{
		{ID: "test-1", TitleText: "First Issue", Priority: 1, Status: beads.StatusOpen, Type: beads.TypeTask},
		{ID: "test-2", TitleText: "Second Issue", Priority: 2, Status: beads.StatusInProgress, Type: beads.TypeBug},
		{ID: "test-3", TitleText: "Third Issue", Priority: 0, Status: beads.StatusOpen, Type: beads.TypeFeature},
	}
	m, _ = m.handleSearchResults(searchResultsMsg{issues: issues, err: nil})
	return m
}

func TestSearch_New(t *testing.T) {
	m := createTestModel()

	require.Equal(t, FocusSearch, m.focus, "expected focus on search input")
	require.Equal(t, ViewSearch, m.view, "expected ViewSearch mode")
	require.False(t, m.hasDetail, "expected no detail initially")
	require.Nil(t, m.results, "expected no results initially")
}

func TestSearch_SetSize(t *testing.T) {
	m := createTestModel()

	m = m.SetSize(120, 50)

	require.Equal(t, 120, m.width, "width should be updated")
	require.Equal(t, 50, m.height, "height should be updated")
}

func TestSearch_SetSize_ZeroGuard(t *testing.T) {
	m := createTestModel()
	m.width = 100
	m.height = 40

	m = m.SetSize(0, 0)

	// Should not crash and should preserve existing values
	require.Equal(t, 0, m.width, "width should be 0")
	require.Equal(t, 0, m.height, "height should be 0")
}

func TestSearch_HandleSearchResults_Success(t *testing.T) {
	m := createTestModel()
	issues := []beads.Issue{
		{ID: "test-1", TitleText: "First", Priority: 1, Status: beads.StatusOpen},
		{ID: "test-2", TitleText: "Second", Priority: 2, Status: beads.StatusClosed},
	}

	m, _ = m.handleSearchResults(searchResultsMsg{issues: issues, err: nil})

	require.Nil(t, m.searchErr, "expected no error")
	require.Len(t, m.results, 2, "expected 2 results")
	require.Equal(t, 0, m.selectedIdx, "expected first item selected")
	require.True(t, m.hasDetail, "expected detail panel to be active")
}

func TestSearch_HandleSearchResults_Empty(t *testing.T) {
	m := createTestModel()

	m, _ = m.handleSearchResults(searchResultsMsg{issues: []beads.Issue{}, err: nil})

	require.Nil(t, m.searchErr, "expected no error")
	require.Empty(t, m.results, "expected empty results")
	require.False(t, m.hasDetail, "expected no detail panel")
}

func TestSearch_HandleSearchResults_Error(t *testing.T) {
	m := createTestModel()
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
	m := createTestModelWithResults()
	m.focus = FocusResults
	m.input.Blur()

	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

	require.Equal(t, FocusSearch, m.focus, "expected focus on search")
	require.True(t, m.input.Focused(), "expected input to be focused")
}

func TestSearch_FocusNavigation_HMovesLeft(t *testing.T) {
	m := createTestModelWithResults()
	m.focus = FocusDetails

	// h moves focus from details to results
	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})

	require.Equal(t, FocusResults, m.focus, "expected focus on results")
}

func TestSearch_FocusNavigation_LMovesRight(t *testing.T) {
	m := createTestModelWithResults()
	m.focus = FocusResults

	// l moves focus from results to details
	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})

	require.Equal(t, FocusDetails, m.focus, "expected focus on details")
}

func TestSearch_FocusNavigation_LMovesToDetailsEvenWhenEmpty(t *testing.T) {
	m := createTestModel()
	m.focus = FocusResults
	m.hasDetail = false

	// l should move to details even when detail panel is empty
	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})

	require.Equal(t, FocusDetails, m.focus, "expected focus to move to details")
}

func TestSearch_FocusNavigation_EscFromSearchExitsToKanban(t *testing.T) {
	m := createTestModel()
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
	m := createTestModelWithResults()
	m.focus = FocusResults

	m, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEscape})

	require.NotNil(t, cmd, "expected command to be returned")

	// Execute the command to get the message
	msg := cmd()
	_, ok := msg.(ExitToKanbanMsg)
	require.True(t, ok, "expected ExitToKanbanMsg")
}

func TestSearch_FocusNavigation_EscFromDetailsExitsToKanban(t *testing.T) {
	m := createTestModelWithResults()
	m.focus = FocusDetails

	m, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEscape})

	require.NotNil(t, cmd, "expected command to be returned")

	// Execute the command to get the message
	msg := cmd()
	_, ok := msg.(ExitToKanbanMsg)
	require.True(t, ok, "expected ExitToKanbanMsg")
}

func TestSearch_ResultSelection_JMovesDown(t *testing.T) {
	m := createTestModelWithResults()
	m.focus = FocusResults
	m.selectedIdx = 0

	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	require.Equal(t, 1, m.selectedIdx, "expected selectedIdx to increment")
}

func TestSearch_ResultSelection_KMovesUp(t *testing.T) {
	m := createTestModelWithResults()
	m.focus = FocusResults
	m.selectedIdx = 1

	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})

	require.Equal(t, 0, m.selectedIdx, "expected selectedIdx to decrement")
}

func TestSearch_ResultSelection_JAtEnd(t *testing.T) {
	m := createTestModelWithResults()
	m.focus = FocusResults
	m.selectedIdx = 2 // Last item

	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	require.Equal(t, 2, m.selectedIdx, "expected selectedIdx to stay at end")
}

func TestSearch_ResultSelection_KAtStart(t *testing.T) {
	m := createTestModelWithResults()
	m.focus = FocusResults
	m.selectedIdx = 0

	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})

	require.Equal(t, 0, m.selectedIdx, "expected selectedIdx to stay at start")
}

func TestSearch_HelpOverlay_QuestionOpens(t *testing.T) {
	m := createTestModel()
	m.focus = FocusResults // Must not be in search input for ? to open help

	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})

	require.Equal(t, ViewHelp, m.view, "expected help view")
}

func TestSearch_HelpOverlay_QuestionCloses(t *testing.T) {
	m := createTestModel()
	m.view = ViewHelp

	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})

	require.Equal(t, ViewSearch, m.view, "expected search view")
}

func TestSearch_HelpOverlay_EscCloses(t *testing.T) {
	m := createTestModel()
	m.view = ViewHelp

	m, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEscape})

	require.Equal(t, ViewSearch, m.view, "expected search view")
}

func TestSearch_PickerOpen_Priority(t *testing.T) {
	m := createTestModelWithResults()
	m.focus = FocusDetails

	msg := details.OpenPriorityPickerMsg{IssueID: "test-1", Current: beads.Priority(1)}
	m, _ = m.openPriorityPicker(msg)

	require.Equal(t, ViewPriorityPicker, m.view, "expected priority picker view")
	require.NotNil(t, m.selectedIssue, "expected selected issue to be set")
}

func TestSearch_PickerOpen_Status(t *testing.T) {
	m := createTestModelWithResults()
	m.focus = FocusDetails

	msg := details.OpenStatusPickerMsg{IssueID: "test-1", Current: beads.StatusOpen}
	m, _ = m.openStatusPicker(msg)

	require.Equal(t, ViewStatusPicker, m.view, "expected status picker view")
	require.NotNil(t, m.selectedIssue, "expected selected issue to be set")
}

func TestSearch_PickerCancel_Esc(t *testing.T) {
	m := createTestModelWithResults()
	m.view = ViewPriorityPicker
	m.selectedIssue = &m.results[0]

	// handlePickerKey now delegates to picker.Update which produces a command
	m, cmd := m.handlePickerKey(tea.KeyMsg{Type: tea.KeyEscape})
	require.NotNil(t, cmd, "expected command from esc key")

	// Execute the command to get the message (picker.CancelMsg for pickers without callbacks)
	msg := cmd()

	// Process the message in Update
	m, _ = m.Update(msg)

	require.Equal(t, ViewSearch, m.view, "expected search view after cancel")
	require.Nil(t, m.selectedIssue, "expected selected issue to be cleared")
}

func TestSearch_PickerCancel_Q(t *testing.T) {
	m := createTestModelWithResults()
	m.view = ViewStatusPicker
	m.selectedIssue = &m.results[0]

	// handlePickerKey now delegates to picker.Update which produces a command
	m, cmd := m.handlePickerKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	require.NotNil(t, cmd, "expected command from q key")

	// Execute the command to get the message (picker.CancelMsg for pickers without callbacks)
	msg := cmd()

	// Process the message in Update
	m, _ = m.Update(msg)

	require.Equal(t, ViewSearch, m.view, "expected search view after cancel")
	require.Nil(t, m.selectedIssue, "expected selected issue to be cleared")
}

func TestSearch_PriorityChanged_Success(t *testing.T) {
	m := createTestModelWithResults()
	m.selectedIssue = &m.results[0]

	msg := priorityChangedMsg{issueID: "test-1", priority: beads.Priority(0), err: nil}
	m, cmd := m.handlePriorityChanged(msg)

	require.Nil(t, m.selectedIssue, "expected selected issue to be cleared")
	require.NotNil(t, cmd, "expected ShowToastMsg command for success")
	// Check that results list was updated
	require.Equal(t, beads.Priority(0), m.results[0].Priority, "expected priority updated in results")
}

func TestSearch_PriorityChanged_Error(t *testing.T) {
	m := createTestModelWithResults()
	m.selectedIssue = &m.results[0]

	msg := priorityChangedMsg{issueID: "test-1", priority: beads.Priority(0), err: errors.New("db error")}
	m, cmd := m.handlePriorityChanged(msg)

	require.Nil(t, m.selectedIssue, "expected selected issue to be cleared")
	require.NotNil(t, cmd, "expected ShowToastMsg command for error")
}

func TestSearch_StatusChanged_Success(t *testing.T) {
	m := createTestModelWithResults()
	m.selectedIssue = &m.results[0]

	msg := statusChangedMsg{issueID: "test-1", status: beads.StatusClosed, err: nil}
	m, cmd := m.handleStatusChanged(msg)

	require.Nil(t, m.selectedIssue, "expected selected issue to be cleared")
	require.NotNil(t, cmd, "expected ShowToastMsg command for success")
	// Check that results list was updated
	require.Equal(t, beads.StatusClosed, m.results[0].Status, "expected status updated in results")
}

func TestSearch_StatusChanged_Error(t *testing.T) {
	m := createTestModelWithResults()
	m.selectedIssue = &m.results[0]

	msg := statusChangedMsg{issueID: "test-1", status: beads.StatusClosed, err: errors.New("db error")}
	m, cmd := m.handleStatusChanged(msg)

	require.Nil(t, m.selectedIssue, "expected selected issue to be cleared")
	require.NotNil(t, cmd, "expected ShowToastMsg command for error")
}

func TestSearch_View_NotPanics(t *testing.T) {
	// Test that View() doesn't panic in various states
	tests := []struct {
		name string
		m    Model
	}{
		{"empty", createTestModel()},
		{"with_results", createTestModelWithResults()},
		{"help_view", func() Model {
			m := createTestModel()
			m.view = ViewHelp
			return m
		}()},
		{"priority_picker", func() Model {
			m := createTestModelWithResults()
			m.view = ViewPriorityPicker
			return m
		}()},
		{"status_picker", func() Model {
			m := createTestModelWithResults()
			m.view = ViewStatusPicker
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
	issue := beads.Issue{ID: "test-1", TitleText: "My Test Issue"}
	item := issueItem{issue: issue}

	require.Equal(t, "My Test Issue", item.FilterValue())
}

func TestSearch_IssueDelegate_HeightAndSpacing(t *testing.T) {
	d := newIssueDelegate()

	require.Equal(t, 1, d.Height(), "delegate height should be 1")
	require.Equal(t, 0, d.Spacing(), "delegate spacing should be 0")
}

func TestSearch_SetQuery(t *testing.T) {
	m := createTestModel()

	m = m.SetQuery("status:open")

	// Verify query was set on input
	require.Equal(t, "status:open", m.input.Value(), "query should be set")
}

func TestSearch_SetQuery_Empty(t *testing.T) {
	m := createTestModel()

	// Set a query first
	m = m.SetQuery("priority:1")
	require.Equal(t, "priority:1", m.input.Value())

	// Set empty query
	m = m.SetQuery("")

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
	m := createTestModelWithResults()

	// Set up: results have test-1 selected, but details shows a different issue
	m.selectedIdx = 0
	require.Equal(t, "test-1", m.results[m.selectedIdx].ID, "precondition: results selection is test-1")

	// Create details view showing a DIFFERENT issue (test-999)
	differentIssue := beads.Issue{ID: "test-999", TitleText: "Different Issue"}
	m.details = details.New(differentIssue, nil, nil).SetSize(50, 30)
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
	m := createTestModelWithResults()

	// Set up: results have test-1 selected
	m.selectedIdx = 0
	require.Equal(t, "test-1", m.results[m.selectedIdx].ID, "precondition: results selection is test-1")

	// Create details view showing a DIFFERENT issue
	differentIssue := beads.Issue{ID: "test-999", TitleText: "Different Issue"}
	m.details = details.New(differentIssue, nil, nil).SetSize(50, 30)
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
