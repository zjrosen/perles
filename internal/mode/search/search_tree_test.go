package search

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/assert"

	"perles/internal/beads"
	"perles/internal/mocks"
	"perles/internal/mode"
	"perles/internal/mode/shared"
	"perles/internal/ui/tree"
)

// testClockTime for deterministic timestamps in tests.
var testClockTime = time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)

// testCreatedAt is 2 days before testClockTime for "2d ago" display
var testCreatedAt = time.Date(2025, 1, 13, 12, 0, 0, 0, time.UTC)

// newTestClock creates a MockClock that always returns testClockTime.
func newTestClock(t *testing.T) shared.Clock {
	clock := mocks.NewMockClock(t)
	clock.EXPECT().Now().Return(testClockTime).Maybe()
	return clock
}

// Golden tests for tree sub-mode rendering.
// Run with -update flag to update golden files: go test -update ./internal/mode/search/...

// createTestModelInTreeMode creates a model in tree sub-mode for testing.
func createTestModelInTreeMode(t *testing.T) Model {
	m := createTestModel(t)
	m.subMode = mode.SubModeTree
	m.focus = FocusResults
	return m
}

// buildIssueMap creates a map from issues slice for tree.New.
func buildIssueMap(issues []beads.Issue) map[string]*beads.Issue {
	m := make(map[string]*beads.Issue)
	for i := range issues {
		m[issues[i].ID] = &issues[i]
	}
	return m
}

// createTestModelWithTree creates a model with a loaded tree.
func createTestModelWithTree(t *testing.T, rootIssue beads.Issue, issues []beads.Issue) Model {
	m := createTestModelInTreeMode(t)
	m.treeRoot = &rootIssue

	// Build tree
	issueMap := buildIssueMap(issues)
	m.tree = tree.New(rootIssue.ID, issueMap, tree.DirectionDown, tree.ModeDeps, newTestClock(t))
	return m
}

func TestSearch_TreeView_Golden_Loading(t *testing.T) {
	m := createTestModelInTreeMode(t)
	m = m.SetSize(160, 30)
	// No tree loaded yet - should show loading state
	m.treeRoot = &beads.Issue{ID: "test-root"}

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestSearch_TreeView_Golden_WithTree(t *testing.T) {
	rootIssue := beads.Issue{
		ID:        "epic-1",
		TitleText: "Epic: Implement new feature",
		Type:      beads.TypeEpic,
		Status:    beads.StatusOpen,
		Priority:  1,
		Children:  []string{"task-1", "task-2", "task-3"},
	}
	issues := []beads.Issue{
		rootIssue,
		{ID: "task-1", TitleText: "Design API", Type: beads.TypeTask, Status: beads.StatusClosed, Priority: 1, ParentID: "epic-1"},
		{ID: "task-2", TitleText: "Implement backend", Type: beads.TypeTask, Status: beads.StatusInProgress, Priority: 1, ParentID: "epic-1"},
		{ID: "task-3", TitleText: "Add tests", Type: beads.TypeTask, Status: beads.StatusOpen, Priority: 2, ParentID: "epic-1"},
	}

	m := createTestModelWithTree(t, rootIssue, issues)
	m = m.SetSize(160, 30)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestSearch_TreeView_Golden_DownDirection(t *testing.T) {
	rootIssue := beads.Issue{
		ID:        "parent-1",
		TitleText: "Parent Issue",
		Type:      beads.TypeTask,
		Status:    beads.StatusOpen,
		Children:  []string{"child-1", "child-2"},
	}
	issues := []beads.Issue{
		rootIssue,
		{ID: "child-1", TitleText: "Child A", Type: beads.TypeTask, Status: beads.StatusClosed, ParentID: "parent-1"},
		{ID: "child-2", TitleText: "Child B", Type: beads.TypeTask, Status: beads.StatusOpen, ParentID: "parent-1"},
	}

	m := createTestModelWithTree(t, rootIssue, issues)
	issueMap := buildIssueMap(issues)
	m.tree = tree.New(rootIssue.ID, issueMap, tree.DirectionDown, tree.ModeDeps, newTestClock(t))
	m = m.SetSize(160, 30)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestSearch_TreeView_Golden_UpDirection(t *testing.T) {
	rootIssue := beads.Issue{
		ID:        "child-1",
		TitleText: "Child Issue",
		Type:      beads.TypeTask,
		Status:    beads.StatusOpen,
		ParentID:  "parent-1",
	}
	parentIssue := beads.Issue{
		ID:        "parent-1",
		TitleText: "Parent Issue",
		Type:      beads.TypeEpic,
		Status:    beads.StatusOpen,
		Children:  []string{"child-1"},
	}
	issues := []beads.Issue{parentIssue, rootIssue}

	m := createTestModelWithTree(t, rootIssue, issues)
	issueMap := buildIssueMap(issues)
	m.tree = tree.New(rootIssue.ID, issueMap, tree.DirectionUp, tree.ModeDeps, newTestClock(t))
	m = m.SetSize(160, 30)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestSearch_TreeView_Golden_ChildrenMode(t *testing.T) {
	// Create an epic with children and also a dependency
	rootIssue := beads.Issue{
		ID:        "epic-1",
		TitleText: "Epic: Implement new feature",
		Type:      beads.TypeEpic,
		Priority:  1,
		Status:    beads.StatusOpen,
		Children:  []string{"task-1", "task-2"},
		Blocks:    []string{"task-3"}, // This should NOT appear in children mode
	}
	issues := []beads.Issue{
		rootIssue,
		{ID: "task-1", TitleText: "Design API", Type: beads.TypeTask, Priority: 1, Status: beads.StatusClosed, ParentID: "epic-1"},
		{ID: "task-2", TitleText: "Implement backend", Type: beads.TypeTask, Priority: 1, Status: beads.StatusInProgress, ParentID: "epic-1"},
		{ID: "task-3", TitleText: "Blocked task (dependency)", Type: beads.TypeTask, Priority: 2, Status: beads.StatusOpen}, // No ParentID - pure dependency
	}

	m := createTestModelWithTree(t, rootIssue, issues)
	issueMap := buildIssueMap(issues)
	// Use children mode - should only show task-1 and task-2, NOT task-3
	m.tree = tree.New(rootIssue.ID, issueMap, tree.DirectionDown, tree.ModeChildren, newTestClock(t))
	m = m.SetSize(160, 30)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// Unit tests for renderCompactProgress
func TestRenderCompactProgress(t *testing.T) {
	tests := []struct {
		name           string
		closed         int
		total          int
		expectedSuffix string // The percentage and count suffix
		expectedEmpty  bool   // Whether result should be empty
	}{
		{"EmptyTotal", 0, 0, "", true},
		{"NoneClosedOf5", 0, 5, " 0% (0/5)", false},
		{"OneOfFive", 1, 5, " 20% (1/5)", false},
		{"HalfClosedOf4", 2, 4, " 50% (2/4)", false},
		{"ThreeOfFour", 3, 4, " 75% (3/4)", false},
		{"AllClosedOf5", 5, 5, " 100% (5/5)", false},
		{"OneOfTwo", 1, 2, " 50% (1/2)", false},
		{"AllClosedOf1", 1, 1, " 100% (1/1)", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := renderCompactProgress(tc.closed, tc.total)
			if tc.expectedEmpty {
				assert.Empty(t, result)
			} else {
				// Check suffix (percentage and counts)
				assert.Contains(t, result, tc.expectedSuffix)
				// Check that progress bar characters are present (either filled or empty)
				hasBar := strings.Contains(result, "█") || strings.Contains(result, "░")
				assert.True(t, hasBar, "progress bar should contain bar characters")
			}
		})
	}
}

func TestSearch_TreeSubMode_Initialization(t *testing.T) {
	m := createTestModel(t)

	// Enter tree sub-mode via EnterMsg
	m, _ = m.Update(EnterMsg{SubMode: mode.SubModeTree, IssueID: "test-123"})

	assert.Equal(t, mode.SubModeTree, m.subMode)
	assert.Equal(t, FocusResults, m.focus, "should focus tree panel when entering tree mode from kanban")
	assert.NotNil(t, m.treeRoot)
	assert.Equal(t, "test-123", m.treeRoot.ID)
}

func TestSearch_TreeSubMode_EnterListClearsTreeState(t *testing.T) {
	rootIssue := beads.Issue{ID: "root", TitleText: "Root"}
	m := createTestModelWithTree(t, rootIssue, []beads.Issue{rootIssue})

	// Verify tree state is set
	assert.Equal(t, mode.SubModeTree, m.subMode)
	assert.NotNil(t, m.tree)
	assert.NotNil(t, m.treeRoot)

	// EnterMsg with list mode should clear tree state
	m, _ = m.Update(EnterMsg{SubMode: mode.SubModeList, Query: "status = open"})

	assert.Equal(t, mode.SubModeList, m.subMode)
	assert.Equal(t, FocusSearch, m.focus)
	assert.Nil(t, m.tree)
	assert.Nil(t, m.treeRoot)
}

func TestSearch_EnterTreeMode_ClearsOldTreeState(t *testing.T) {
	// Bug scenario: User views Task A in tree mode, returns to kanban,
	// then opens Epic B. Without clearing m.tree, handleTreeLoaded()
	// would restore selection to Task A (if it's a child of Epic B).

	rootIssue := beads.Issue{ID: "task-1", TitleText: "Task A"}
	m := createTestModelWithTree(t, rootIssue, []beads.Issue{rootIssue})

	// Verify tree state exists (simulating previous tree session)
	assert.NotNil(t, m.tree, "precondition: tree should be set")
	assert.Equal(t, "task-1", m.treeRoot.ID)

	// User enters tree mode for a DIFFERENT issue (Epic B)
	m, _ = m.Update(EnterMsg{SubMode: mode.SubModeTree, IssueID: "epic-1"})

	// m.tree should be nil to prevent handleTreeLoaded from restoring stale selection
	assert.Nil(t, m.tree, "EnterMsg should clear tree state")
	assert.Equal(t, "epic-1", m.treeRoot.ID, "treeRoot should be set to new issue")
	assert.Equal(t, mode.SubModeTree, m.subMode, "should remain in tree mode")
	assert.Equal(t, FocusResults, m.focus, "focus should be on results")
}

// Key handling tests for tree sub-mode

// createTreeTestModel creates a model in tree sub-mode with multiple children for key testing.
func createTreeTestModel(t *testing.T) Model {
	rootIssue := beads.Issue{
		ID:        "root-1",
		TitleText: "Root Issue",
		Type:      beads.TypeEpic,
		Status:    beads.StatusOpen,
		Children:  []string{"child-1", "child-2", "child-3"},
	}
	issues := []beads.Issue{
		rootIssue,
		{ID: "child-1", TitleText: "First Child", Type: beads.TypeTask, Status: beads.StatusClosed, ParentID: "root-1"},
		{ID: "child-2", TitleText: "Second Child", Type: beads.TypeTask, Status: beads.StatusInProgress, ParentID: "root-1"},
		{ID: "child-3", TitleText: "Third Child", Type: beads.TypeTask, Status: beads.StatusOpen, ParentID: "root-1"},
	}

	m := createTestModelWithTree(t, rootIssue, issues)
	m = m.SetSize(100, 30)
	return m
}

func TestTreeSubMode_JKey_MovesCursorDown(t *testing.T) {
	m := createTreeTestModel(t)
	initialID := m.tree.SelectedNode().Issue.ID

	// Press j to move down
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	// Selected node should have changed (moved to first child)
	assert.NotEqual(t, initialID, m.tree.SelectedNode().Issue.ID, "j should move cursor down")
}

func TestTreeSubMode_KKey_MovesCursorUp(t *testing.T) {
	m := createTreeTestModel(t)

	// First move down, then test k
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	idAfterJ := m.tree.SelectedNode().Issue.ID

	// Press k to move up
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})

	// Should be back at root
	assert.NotEqual(t, idAfterJ, m.tree.SelectedNode().Issue.ID, "k should move cursor up")
	assert.Equal(t, "root-1", m.tree.SelectedNode().Issue.ID, "should be back at root")
}

func TestTreeSubMode_DownArrow_MovesCursorDown(t *testing.T) {
	m := createTreeTestModel(t)
	initialID := m.tree.SelectedNode().Issue.ID

	// Press down arrow
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})

	assert.NotEqual(t, initialID, m.tree.SelectedNode().Issue.ID, "down arrow should move cursor down")
}

func TestTreeSubMode_UpArrow_MovesCursorUp(t *testing.T) {
	m := createTreeTestModel(t)

	// First move down
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})

	// Press up arrow
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})

	assert.Equal(t, "root-1", m.tree.SelectedNode().Issue.ID, "up arrow should move cursor up to root")
}

func TestTreeSubMode_SlashKey_SwitchesToListMode(t *testing.T) {
	m := createTreeTestModel(t)

	// Press / to switch to list mode
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

	assert.Equal(t, mode.SubModeList, m.subMode, "/ should switch to list sub-mode")
	assert.Equal(t, FocusSearch, m.focus, "/ should focus search input")
}

func TestTreeSubMode_DKey_TogglesDirection(t *testing.T) {
	m := createTreeTestModel(t)
	initialDirection := m.tree.Direction()

	// Press d to toggle direction
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	// Direction should have changed
	assert.NotEqual(t, initialDirection, m.tree.Direction(), "d should toggle direction")
}

func TestTreeSubMode_MKey_TogglesMode(t *testing.T) {
	m := createTreeTestModel(t)
	initialMode := m.tree.Mode()
	assert.Equal(t, tree.ModeDeps, initialMode, "should start in deps mode")

	// Press m to toggle mode
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})

	// Mode should have changed to children
	assert.Equal(t, tree.ModeChildren, m.tree.Mode(), "m should toggle to children mode")

	// Press m again to toggle back
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})

	// Mode should have changed back to deps
	assert.Equal(t, tree.ModeDeps, m.tree.Mode(), "m should toggle back to deps mode")
}

func TestTreeSubMode_LKey_FocusesDetails(t *testing.T) {
	m := createTreeTestModel(t)
	assert.Equal(t, FocusResults, m.focus, "should start with focus on results")

	// Press l to move focus to details
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})

	assert.Equal(t, FocusDetails, m.focus, "l should move focus to details")
}

func TestTreeSubMode_RightArrow_FocusesDetails(t *testing.T) {
	m := createTreeTestModel(t)

	// Press right arrow
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})

	assert.Equal(t, FocusDetails, m.focus, "right arrow should move focus to details")
}

func TestTreeSubMode_TabKey_FocusesDetails(t *testing.T) {
	m := createTreeTestModel(t)

	// Press tab
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	assert.Equal(t, FocusDetails, m.focus, "tab should move focus to details")
}

func TestTreeSubMode_TabKey_CyclesBetweenTreeAndDetails(t *testing.T) {
	m := createTreeTestModel(t)

	// Start on tree (FocusResults)
	assert.Equal(t, FocusResults, m.focus, "should start on tree")

	// Tab to details
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, FocusDetails, m.focus, "tab should move focus to details")

	// Tab back to tree (not search input, since tree mode has no search input)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, FocusResults, m.focus, "tab should cycle back to tree, not search input")
}

func TestTreeSubMode_EscKey_ReturnsToKanban(t *testing.T) {
	m := createTreeTestModel(t)

	// Press esc - should return command that sends ExitToKanbanMsg
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	// Execute the command to get the message
	if cmd != nil {
		msg := cmd()
		_, ok := msg.(ExitToKanbanMsg)
		assert.True(t, ok, "esc should return ExitToKanbanMsg")
	} else {
		t.Error("esc should return a command")
	}
}

func TestTreeSubMode_HelpKey_ShowsHelp(t *testing.T) {
	m := createTreeTestModel(t)

	// Press ? to show help
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})

	assert.Equal(t, ViewHelp, m.view, "? should switch to help view")
}

func TestTreeSubMode_CtrlC_Quits(t *testing.T) {
	m := createTreeTestModel(t)

	// Press ctrl+c - should return quit command
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

	assert.NotNil(t, cmd, "ctrl+c should return quit command")
}

func TestTreeSubMode_NotFocused_KeysPassThrough(t *testing.T) {
	m := createTreeTestModel(t)
	m.focus = FocusDetails // Not focused on tree

	initialID := m.tree.SelectedNode().Issue.ID

	// Press j while not focused on tree
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	// Cursor should not move since tree isn't focused
	assert.Equal(t, initialID, m.tree.SelectedNode().Issue.ID, "j should not move cursor when tree not focused")
}

func TestTreeSubMode_EnterKey_RefocusesTree(t *testing.T) {
	m := createTreeTestModel(t)
	originalRootID := m.tree.Root().Issue.ID

	// Move cursor to first child
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	selectedBeforeEnter := m.tree.SelectedNode().Issue.ID
	assert.NotEqual(t, originalRootID, selectedBeforeEnter, "should have moved to child")

	// Press Enter to refocus tree on selected node
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// The tree should now be focused on the child
	// After refocus, the root becomes the previously selected node
	newRootID := m.tree.Root().Issue.ID
	assert.Equal(t, selectedBeforeEnter, newRootID, "enter should refocus tree on selected node")
	// treeRoot should also be updated
	assert.Equal(t, selectedBeforeEnter, m.treeRoot.ID, "treeRoot should match new root")
}

func TestTreeSubMode_UKey_GoesBack(t *testing.T) {
	m := createTreeTestModel(t)
	originalRootID := m.tree.Root().Issue.ID

	// First refocus to a child (Enter on child)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	newRootID := m.tree.Root().Issue.ID
	assert.NotEqual(t, originalRootID, newRootID, "should have refocused to child")

	// Press u to go back
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})

	// Should be back at original root
	assert.Equal(t, originalRootID, m.tree.Root().Issue.ID, "u should go back to previous root")
	assert.Equal(t, originalRootID, m.treeRoot.ID, "treeRoot should be updated")
}

func TestTreeSubMode_UCapitalKey_GoesToOriginal(t *testing.T) {
	m := createTreeTestModel(t)
	originalRootID := m.tree.Root().Issue.ID

	// First refocus to a child
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Refocus again to grandchild (simulate deep navigation)
	// Note: In our test data, children don't have grandchildren,
	// so we just verify that U returns to original after one level
	currentRootID := m.tree.Root().Issue.ID
	assert.NotEqual(t, originalRootID, currentRootID, "should be at child level")

	// Press U to go to original root
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})

	// Should be back at original root
	assert.Equal(t, originalRootID, m.tree.Root().Issue.ID, "U should go to original root")
	assert.Equal(t, originalRootID, m.treeRoot.ID, "treeRoot should be updated")
}

// Tests for handleIssueDeleted tree-aware deletion handling

func TestHandleIssueDeleted_TreeMode_NonRootDeletion(t *testing.T) {
	// Setup: Tree mode with a root and children
	m := createTreeTestModel(t)
	assert.Equal(t, mode.SubModeTree, m.subMode)
	assert.NotNil(t, m.treeRoot)
	rootID := m.treeRoot.ID

	// Delete a non-root issue (wasTreeRoot=false)
	msg := issueDeletedMsg{
		issueID:     "child-1",
		parentID:    "root-1",
		wasTreeRoot: false,
		err:         nil,
	}
	m, cmd := m.handleIssueDeleted(msg)

	// Verify state after deletion
	assert.Equal(t, ViewSearch, m.view, "view should return to search")
	assert.Nil(t, m.selectedIssue, "selectedIssue should be cleared")

	// Verify command is returned (loadTree with same root + toast)
	assert.NotNil(t, cmd, "should return command")

	// The tree root should still be the same (refreshed with same root)
	assert.Equal(t, rootID, m.treeRoot.ID, "treeRoot should remain unchanged for non-root deletion")
}

func TestHandleIssueDeleted_TreeMode_RootDeletionWithParent(t *testing.T) {
	// Setup: Tree mode where root has a parent
	rootIssue := beads.Issue{
		ID:        "child-root",
		TitleText: "Child as Root",
		Type:      beads.TypeTask,
		Status:    beads.StatusOpen,
		ParentID:  "parent-1", // Has a parent
	}
	issues := []beads.Issue{rootIssue}

	m := createTestModelWithTree(t, rootIssue, issues)
	assert.Equal(t, mode.SubModeTree, m.subMode)

	// Delete the root issue (wasTreeRoot=true, has parent)
	msg := issueDeletedMsg{
		issueID:     "child-root",
		parentID:    "parent-1",
		wasTreeRoot: true,
		err:         nil,
	}
	m, cmd := m.handleIssueDeleted(msg)

	// Verify state
	assert.Equal(t, ViewSearch, m.view, "view should return to search")
	assert.Nil(t, m.selectedIssue, "selectedIssue should be cleared")

	// Should return loadTree command with parentID as new root
	assert.NotNil(t, cmd, "should return command for re-rooting to parent")
}

func TestHandleIssueDeleted_TreeMode_RootDeletionWithoutParent(t *testing.T) {
	// Setup: Tree mode where root has no parent (orphan root)
	rootIssue := beads.Issue{
		ID:        "orphan-root",
		TitleText: "Orphan Root",
		Type:      beads.TypeTask,
		Status:    beads.StatusOpen,
		ParentID:  "", // No parent
	}
	issues := []beads.Issue{rootIssue}

	m := createTestModelWithTree(t, rootIssue, issues)
	assert.Equal(t, mode.SubModeTree, m.subMode)

	// Delete the root issue (wasTreeRoot=true, no parent)
	msg := issueDeletedMsg{
		issueID:     "orphan-root",
		parentID:    "",
		wasTreeRoot: true,
		err:         nil,
	}
	m, cmd := m.handleIssueDeleted(msg)

	// Verify state
	assert.Equal(t, ViewSearch, m.view, "view should return to search")
	assert.Nil(t, m.selectedIssue, "selectedIssue should be cleared")

	// Should return ExitToKanbanMsg command
	assert.NotNil(t, cmd, "should return command")

	// Execute the batch command - one of them should be ExitToKanbanMsg
	// Note: tea.Batch returns multiple commands, we check that it exists
}

func TestHandleIssueDeleted_ListMode_Deletion(t *testing.T) {
	// Setup: List mode (default)
	m := createTestModelWithResults(t)
	assert.Equal(t, mode.SubModeList, m.subMode)

	// Delete an issue in list mode
	msg := issueDeletedMsg{
		issueID:     "test-1",
		parentID:    "",
		wasTreeRoot: false,
		err:         nil,
	}
	m, cmd := m.handleIssueDeleted(msg)

	// Verify state
	assert.Equal(t, ViewSearch, m.view, "view should return to search")
	assert.Nil(t, m.selectedIssue, "selectedIssue should be cleared")

	// Should return executeSearch command + toast
	assert.NotNil(t, cmd, "should return command for list refresh")
}

func TestHandleIssueDeleted_Error(t *testing.T) {
	// Setup: Any mode
	m := createTreeTestModel(t)

	// Delete fails with error
	msg := issueDeletedMsg{
		issueID:     "any-issue",
		parentID:    "",
		wasTreeRoot: false,
		err:         assert.AnError,
	}
	m, cmd := m.handleIssueDeleted(msg)

	// Verify state
	assert.Equal(t, ViewSearch, m.view, "view should return to search")
	assert.Nil(t, m.selectedIssue, "selectedIssue should be cleared")

	// Should return error toast command
	assert.NotNil(t, cmd, "should return command for error toast")
}
