package dashboard

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	beads "github.com/zjrosen/perles/internal/beads/domain"
	"github.com/zjrosen/perles/internal/config"
	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/mode"
	"github.com/zjrosen/perles/internal/orchestration/controlplane"
	"github.com/zjrosen/perles/internal/ui/tree"
)

// === Test Helpers ===

// createTestIssue creates a test issue with the given parameters.
func createTestIssue(id, title string, parentID string) beads.Issue {
	return beads.Issue{
		ID:        id,
		TitleText: title,
		ParentID:  parentID,
		Status:    beads.StatusOpen,
		Priority:  beads.PriorityMedium,
		Type:      beads.TypeTask,
	}
}

// createEpicTreeTestModel creates a dashboard model with mocked services for epic tree testing.
// This model has a mock client that handles GetComments calls required by details.New().
func createEpicTreeTestModel(t *testing.T) Model {
	t.Helper()

	mockCP := newMockControlPlane()
	mockCP.On("List", mock.Anything, mock.Anything).Return([]*controlplane.WorkflowInstance{}, nil).Maybe()

	eventCh := make(chan controlplane.ControlPlaneEvent)
	close(eventCh)
	mockCP.On("Subscribe", mock.Anything).Return((<-chan controlplane.ControlPlaneEvent)(eventCh), func() {}).Maybe()

	// Create mock client that handles GetComments
	mockClient := mocks.NewMockBeadsClient(t)
	mockClient.EXPECT().GetComments(mock.Anything).Return([]beads.Comment{}, nil).Maybe()

	// Create mock executor
	mockExecutor := mocks.NewMockBQLExecutor(t)
	mockExecutor.EXPECT().Execute(mock.Anything).Return([]beads.Issue{}, nil).Maybe()

	cfg := config.Defaults()

	services := mode.Services{
		Client:   mockClient,
		Executor: mockExecutor,
		Config:   &cfg,
	}

	dashCfg := Config{
		ControlPlane: mockCP,
		Services:     services,
	}

	m := New(dashCfg)
	m = m.SetSize(100, 40).(Model)

	return m
}

// === Unit Tests: loadEpicTree ===

func TestLoadEpicTreeReturnsCommand(t *testing.T) {
	// Setup mock executor
	mockExecutor := mocks.NewMockBQLExecutor(t)
	mockExecutor.EXPECT().
		Execute(mock.MatchedBy(func(query string) bool {
			return query == `id = "epic-123" expand down depth *`
		})).
		Return([]beads.Issue{createTestIssue("epic-123", "Test Epic", "")}, nil).
		Maybe()

	// Call loadEpicTree
	cmd := loadEpicTree("epic-123", mockExecutor)

	// Verify command is returned
	require.NotNil(t, cmd, "loadEpicTree should return a non-nil command")
}

func TestLoadEpicTreeReturnsNilForEmptyEpicID(t *testing.T) {
	mockExecutor := mocks.NewMockBQLExecutor(t)

	// Empty epic ID should return nil
	cmd := loadEpicTree("", mockExecutor)
	require.Nil(t, cmd, "loadEpicTree should return nil for empty epic ID")
}

func TestLoadEpicTreeReturnsNilForNilExecutor(t *testing.T) {
	// Nil executor should return nil
	cmd := loadEpicTree("epic-123", nil)
	require.Nil(t, cmd, "loadEpicTree should return nil for nil executor")
}

func TestLoadEpicTreeExecutesBQL(t *testing.T) {
	// Setup mock executor
	mockExecutor := mocks.NewMockBQLExecutor(t)
	expectedIssues := []beads.Issue{
		createTestIssue("epic-123", "Test Epic", ""),
		createTestIssue("task-1", "Task 1", "epic-123"),
	}
	mockExecutor.EXPECT().
		Execute(`id = "epic-123" expand down depth *`).
		Return(expectedIssues, nil).
		Once()

	// Call loadEpicTree and execute the command
	cmd := loadEpicTree("epic-123", mockExecutor)
	require.NotNil(t, cmd)

	// Execute the command
	msg := cmd()

	// Verify the message
	loadedMsg, ok := msg.(epicTreeLoadedMsg)
	require.True(t, ok, "command should return epicTreeLoadedMsg")
	require.Equal(t, "epic-123", loadedMsg.RootID)
	require.Len(t, loadedMsg.Issues, 2)
	require.NoError(t, loadedMsg.Err)

	mockExecutor.AssertExpectations(t)
}

func TestLoadEpicTreeReturnsErrorInMsg(t *testing.T) {
	// Setup mock executor that returns an error
	mockExecutor := mocks.NewMockBQLExecutor(t)
	expectedErr := errors.New("database error")
	mockExecutor.EXPECT().
		Execute(`id = "epic-123" expand down depth *`).
		Return(nil, expectedErr).
		Once()

	// Call loadEpicTree and execute the command
	cmd := loadEpicTree("epic-123", mockExecutor)
	require.NotNil(t, cmd)

	// Execute the command
	msg := cmd()

	// Verify the error is in the message
	loadedMsg, ok := msg.(epicTreeLoadedMsg)
	require.True(t, ok, "command should return epicTreeLoadedMsg")
	require.Equal(t, "epic-123", loadedMsg.RootID)
	require.Nil(t, loadedMsg.Issues)
	require.ErrorIs(t, loadedMsg.Err, expectedErr)

	mockExecutor.AssertExpectations(t)
}

// === Unit Tests: handleEpicTreeLoaded ===

func TestHandleEpicTreeLoadedBuildsTree(t *testing.T) {
	// Setup model with mocked services
	m := createEpicTreeTestModel(t)
	m.lastLoadedEpicID = "epic-123"

	// Create issues
	issues := []beads.Issue{
		createTestIssue("epic-123", "Test Epic", ""),
		createTestIssue("task-1", "Task 1", "epic-123"),
		createTestIssue("task-2", "Task 2", "epic-123"),
	}

	// Handle the message
	msg := epicTreeLoadedMsg{
		Issues: issues,
		RootID: "epic-123",
		Err:    nil,
	}
	result, cmd := m.handleEpicTreeLoaded(msg)
	m = result.(Model)

	// Verify tree is built
	require.NotNil(t, m.epicTree, "epic tree should be created")
	require.Nil(t, cmd, "no follow-up command expected")
}

func TestHandleEpicTreeLoadedRejectsStale(t *testing.T) {
	// Setup model with different lastLoadedEpicID
	m := createEpicTreeTestModel(t)
	m.lastLoadedEpicID = "epic-456" // Different from message

	// Create issues for a different epic
	issues := []beads.Issue{
		createTestIssue("epic-123", "Old Epic", ""),
	}

	// Handle the message
	msg := epicTreeLoadedMsg{
		Issues: issues,
		RootID: "epic-123", // Different from lastLoadedEpicID
		Err:    nil,
	}
	result, cmd := m.handleEpicTreeLoaded(msg)
	m = result.(Model)

	// Verify tree is NOT built (stale response rejected)
	require.Nil(t, m.epicTree, "epic tree should not be created for stale response")
	require.Nil(t, cmd)
}

func TestHandleEpicTreeLoadedHandlesError(t *testing.T) {
	// Setup model
	m := createEpicTreeTestModel(t)
	m.lastLoadedEpicID = "epic-123"
	// Pre-set an existing tree to verify it gets cleared
	issueMap := map[string]*beads.Issue{
		"old-epic": {ID: "old-epic", TitleText: "Old"},
	}
	m.epicTree = tree.New("old-epic", issueMap, tree.DirectionDown, tree.ModeDeps, nil)

	// Handle error message
	msg := epicTreeLoadedMsg{
		Issues: nil,
		RootID: "epic-123",
		Err:    errors.New("load failed"),
	}
	result, cmd := m.handleEpicTreeLoaded(msg)
	m = result.(Model)

	// Verify tree is cleared on error
	require.Nil(t, m.epicTree, "epic tree should be cleared on error")
	require.False(t, m.hasEpicDetail, "hasEpicDetail should be false on error")
	require.Nil(t, cmd)
}

func TestHandleEpicTreeLoadedHandlesEmptyResults(t *testing.T) {
	// Setup model
	m := createEpicTreeTestModel(t)
	m.lastLoadedEpicID = "epic-123"

	// Handle empty results
	msg := epicTreeLoadedMsg{
		Issues: []beads.Issue{}, // Empty
		RootID: "epic-123",
		Err:    nil,
	}
	result, cmd := m.handleEpicTreeLoaded(msg)
	m = result.(Model)

	// Verify tree is nil for empty results
	require.Nil(t, m.epicTree, "epic tree should be nil for empty results")
	require.False(t, m.hasEpicDetail, "hasEpicDetail should be false for empty results")
	require.Nil(t, cmd)
}

func TestHandleEpicTreeLoadedPreservesDirectionAndMode(t *testing.T) {
	// Setup model with existing tree having custom direction and mode
	m := createEpicTreeTestModel(t)
	m.lastLoadedEpicID = "epic-123"

	// Create existing tree with DirectionUp and ModeChildren
	existingIssueMap := map[string]*beads.Issue{
		"old-epic": {ID: "old-epic", TitleText: "Old"},
	}
	m.epicTree = tree.New("old-epic", existingIssueMap, tree.DirectionUp, tree.ModeChildren, nil)

	// Verify existing tree has the custom settings
	require.Equal(t, tree.DirectionUp, m.epicTree.Direction())
	require.Equal(t, tree.ModeChildren, m.epicTree.Mode())

	// Create new issues
	issues := []beads.Issue{
		createTestIssue("epic-123", "New Epic", ""),
	}

	// Handle the message
	msg := epicTreeLoadedMsg{
		Issues: issues,
		RootID: "epic-123",
		Err:    nil,
	}
	result, _ := m.handleEpicTreeLoaded(msg)
	m = result.(Model)

	// Verify new tree preserves direction and mode
	require.NotNil(t, m.epicTree)
	require.Equal(t, tree.DirectionUp, m.epicTree.Direction(), "direction should be preserved")
	require.Equal(t, tree.ModeChildren, m.epicTree.Mode(), "mode should be preserved")
}

// === Unit Tests: updateEpicDetail ===

func TestUpdateEpicDetailSyncsWithTree(t *testing.T) {
	// Setup model with mocked services
	m := createEpicTreeTestModel(t)

	// Create issue map and tree
	issueMap := map[string]*beads.Issue{
		"epic-123": {ID: "epic-123", TitleText: "Test Epic", Status: beads.StatusOpen},
		"task-1":   {ID: "task-1", TitleText: "Task 1", ParentID: "epic-123", Status: beads.StatusOpen},
	}
	m.epicTree = tree.New("epic-123", issueMap, tree.DirectionDown, tree.ModeDeps, nil)

	// Ensure tree has a selected node
	require.NotNil(t, m.epicTree.SelectedNode(), "tree should have a selected node")

	// Call updateEpicDetail
	m.updateEpicDetail()

	// Verify details panel is updated
	require.True(t, m.hasEpicDetail, "hasEpicDetail should be true after update")
}

func TestUpdateEpicDetailHandlesNilTree(t *testing.T) {
	// Setup model without a tree
	m := createEpicTreeTestModel(t)
	m.epicTree = nil
	m.hasEpicDetail = true // Pre-set to verify it gets cleared

	// Call updateEpicDetail
	m.updateEpicDetail()

	// Verify details are cleared
	require.False(t, m.hasEpicDetail, "hasEpicDetail should be false for nil tree")
}

func TestUpdateEpicDetailHandlesNilNode(t *testing.T) {
	// Setup model with an empty tree (no nodes)
	m := createEpicTreeTestModel(t)

	// Create tree with empty issue map (results in no selected node)
	emptyIssueMap := map[string]*beads.Issue{}
	m.epicTree = tree.New("nonexistent", emptyIssueMap, tree.DirectionDown, tree.ModeDeps, nil)

	// Verify tree has no selected node
	require.Nil(t, m.epicTree.SelectedNode(), "tree should have no selected node")

	m.hasEpicDetail = true // Pre-set to verify it gets cleared

	// Call updateEpicDetail
	m.updateEpicDetail()

	// Verify details are cleared
	require.False(t, m.hasEpicDetail, "hasEpicDetail should be false when no node selected")
}

// === Unit Tests: Tree loading wiring (perles-boi8.3) ===

// createEpicTreeTestModelWithWorkflows creates a test model with workflows that have EpicIDs.
func createEpicTreeTestModelWithWorkflows(t *testing.T) Model {
	t.Helper()

	mockCP := newMockControlPlane()

	// Create workflows with and without EpicIDs
	workflows := []*controlplane.WorkflowInstance{
		{ID: "wf-1", EpicID: "epic-100", State: controlplane.WorkflowRunning},
		{ID: "wf-2", EpicID: "epic-200", State: controlplane.WorkflowRunning},
		{ID: "wf-3", EpicID: "", State: controlplane.WorkflowRunning}, // No epic
	}
	mockCP.On("List", mock.Anything, mock.Anything).Return(workflows, nil).Maybe()

	eventCh := make(chan controlplane.ControlPlaneEvent)
	close(eventCh)
	mockCP.On("Subscribe", mock.Anything).Return((<-chan controlplane.ControlPlaneEvent)(eventCh), func() {}).Maybe()

	// Create mock client that handles GetComments
	mockClient := mocks.NewMockBeadsClient(t)
	mockClient.EXPECT().GetComments(mock.Anything).Return([]beads.Comment{}, nil).Maybe()

	// Create mock executor
	mockExecutor := mocks.NewMockBQLExecutor(t)
	mockExecutor.EXPECT().Execute(mock.Anything).Return([]beads.Issue{}, nil).Maybe()

	cfg := config.Defaults()

	services := mode.Services{
		Client:   mockClient,
		Executor: mockExecutor,
		Config:   &cfg,
	}

	dashCfg := Config{
		ControlPlane: mockCP,
		Services:     services,
	}

	m := New(dashCfg)
	m.workflows = workflows
	m = m.SetSize(100, 40).(Model)

	return m
}

func TestTreeLoadTriggeredOnWorkflowSelection(t *testing.T) {
	// Setup model with workflows
	m := createEpicTreeTestModelWithWorkflows(t)

	// Select first workflow (has epic-100)
	m.selectedIndex = 0
	m.lastLoadedEpicID = "" // No epic loaded yet

	// Navigate to second workflow (has epic-200)
	cmd := m.handleWorkflowSelectionChange(1)

	// Verify:
	// 1. Command is returned (immediate load initiated)
	require.NotNil(t, cmd, "should return load command when new epic selected")

	// 2. lastLoadedEpicID is updated
	require.Equal(t, "epic-200", m.lastLoadedEpicID, "lastLoadedEpicID should be updated to new epic")
}

func TestTreeLoadSkippedForEmptyEpicID(t *testing.T) {
	// Setup model
	m := createEpicTreeTestModelWithWorkflows(t)

	// Start at workflow with epic
	m.selectedIndex = 0 // wf-1 with epic-100

	// Navigate to workflow without epic (wf-3 at index 2)
	cmd := m.handleWorkflowSelectionChange(2)

	// Verify no command is returned (no epic to load)
	require.Nil(t, cmd, "should not trigger tree load when workflow has no epicID")
}

func TestTreeLoadSkippedForSameEpic(t *testing.T) {
	// Setup model
	m := createEpicTreeTestModelWithWorkflows(t)

	// First workflow selected, epic already loaded
	m.selectedIndex = 0
	m.lastLoadedEpicID = "epic-100" // Same as wf-1's epic

	// Create another workflow with the same epic
	m.workflows = append(m.workflows, &controlplane.WorkflowInstance{
		ID:     "wf-4",
		EpicID: "epic-100", // Same epic as wf-1
		State:  controlplane.WorkflowRunning,
	})

	// Navigate to wf-4 (index 3) which has the same epic
	cmd := m.handleWorkflowSelectionChange(3)

	// Verify no command is returned (same epic already loaded)
	require.Nil(t, cmd, "should not trigger tree load when same epic already loaded")
}

// === Unit Tests: Tree Navigation and Toggle Keys (perles-boi8.6) ===

// createEpicTreeTestModelWithTree creates a test model with a pre-populated epic tree for navigation tests.
func createEpicTreeTestModelWithTree(t *testing.T) Model {
	t.Helper()

	m := createEpicTreeTestModel(t)

	// Create a tree with multiple nodes for navigation
	// NOTE: The tree traverses Children arrays (DirectionDown), so we must populate Children on the parent
	issueMap := map[string]*beads.Issue{
		"epic-123": {ID: "epic-123", TitleText: "Test Epic", Status: beads.StatusOpen, Type: beads.TypeEpic, Children: []string{"task-1", "task-2", "task-3"}},
		"task-1":   {ID: "task-1", TitleText: "Task 1", ParentID: "epic-123", Status: beads.StatusOpen, Type: beads.TypeTask},
		"task-2":   {ID: "task-2", TitleText: "Task 2", ParentID: "epic-123", Status: beads.StatusOpen, Type: beads.TypeTask},
		"task-3":   {ID: "task-3", TitleText: "Task 3", ParentID: "epic-123", Status: beads.StatusOpen, Type: beads.TypeTask},
	}
	m.epicTree = tree.New("epic-123", issueMap, tree.DirectionDown, tree.ModeDeps, nil)
	m.epicTree.SetSize(80, 20)

	// Create a workflow with an epic
	m.workflows = []*controlplane.WorkflowInstance{
		{ID: "wf-1", EpicID: "epic-123", State: controlplane.WorkflowRunning},
	}

	m.focus = FocusEpicView
	m.epicViewFocus = EpicFocusTree

	return m
}

func TestTreeCursorDown(t *testing.T) {
	// Verify 'j' key moves cursor down in tree
	m := createEpicTreeTestModelWithTree(t)

	// Verify initial cursor position is at the root
	initialNode := m.epicTree.SelectedNode()
	require.NotNil(t, initialNode)
	require.Equal(t, "epic-123", initialNode.Issue.ID, "initial selection should be root")

	// Press 'j' to move down
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = result.(Model)

	// Verify cursor moved to a child node
	newNode := m.epicTree.SelectedNode()
	require.NotNil(t, newNode)
	require.NotEqual(t, "epic-123", newNode.Issue.ID, "'j' should move cursor to a child node")
}

func TestTreeCursorUp(t *testing.T) {
	// Verify 'k' key moves cursor up in tree
	m := createEpicTreeTestModelWithTree(t)

	// First move down to have room to move up
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = result.(Model)

	nodeAfterJ := m.epicTree.SelectedNode()
	require.NotNil(t, nodeAfterJ)
	require.NotEqual(t, "epic-123", nodeAfterJ.Issue.ID, "after 'j', should not be at root anymore")

	// Press 'k' to move up (back to root)
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = result.(Model)

	// Verify cursor moved back up to root
	nodeAfterK := m.epicTree.SelectedNode()
	require.NotNil(t, nodeAfterK)
	require.Equal(t, "epic-123", nodeAfterK.Issue.ID, "'k' should move cursor back to root")
}

func TestTreeToDetailsPaneSwitch(t *testing.T) {
	// Verify 'l' key switches from tree to details pane
	m := createEpicTreeTestModelWithTree(t)
	m.epicViewFocus = EpicFocusTree

	// Press 'l' to switch to details pane
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = result.(Model)

	require.Equal(t, EpicFocusDetails, m.epicViewFocus, "'l' should switch focus to details pane")
}

func TestDetailsToTreePaneSwitch(t *testing.T) {
	// Verify 'h' key switches from details to tree pane
	m := createEpicTreeTestModelWithTree(t)
	m.epicViewFocus = EpicFocusDetails

	// Press 'h' to switch to tree pane
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = result.(Model)

	require.Equal(t, EpicFocusTree, m.epicViewFocus, "'h' should switch focus to tree pane")
}

func TestTreeModeToggle(t *testing.T) {
	// Verify 'm' key toggles tree mode
	m := createEpicTreeTestModelWithTree(t)

	// Verify initial mode is deps
	require.Equal(t, tree.ModeDeps, m.epicTree.Mode(), "initial mode should be deps")

	// Press 'm' to toggle mode
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = result.(Model)

	require.Equal(t, tree.ModeChildren, m.epicTree.Mode(), "'m' should toggle mode to children")

	// Press 'm' again to toggle back
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = result.(Model)

	require.Equal(t, tree.ModeDeps, m.epicTree.Mode(), "'m' should toggle mode back to deps")
}

func TestCursorMoveTriggersDetailUpdate(t *testing.T) {
	// Verify that j/k cursor movement triggers details panel update
	m := createEpicTreeTestModelWithTree(t)
	m.hasEpicDetail = false // Start without details

	// Move cursor - should trigger updateEpicDetail
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = result.(Model)

	// updateEpicDetail should be called and set hasEpicDetail
	require.True(t, m.hasEpicDetail, "cursor movement should trigger detail update and set hasEpicDetail")
}
