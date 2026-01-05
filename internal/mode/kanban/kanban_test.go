package kanban

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/beads"
	"github.com/zjrosen/perles/internal/config"
	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/mode"
	"github.com/zjrosen/perles/internal/mode/shared"
	"github.com/zjrosen/perles/internal/ui/board"
	"github.com/zjrosen/perles/internal/ui/modals/issueeditor"
	"github.com/zjrosen/perles/internal/ui/shared/diffviewer"
	"github.com/zjrosen/perles/internal/ui/shared/modal"
)

// createTestModel creates a minimal Model for testing state transitions.
// It does not require a database connection.
func createTestModel(t *testing.T) Model {
	cfg := config.Defaults()
	clipboard := mocks.NewMockClipboard(t)
	clipboard.EXPECT().Copy(mock.Anything).Return(nil).Maybe()

	mockExecutor := mocks.NewMockBQLExecutor(t)
	services := mode.Services{
		Config:    &cfg,
		Clipboard: clipboard,
		Executor:  mockExecutor,
	}

	return Model{
		services: services,
		width:    100,
		height:   40,
		view:     ViewBoard,
	}
}

func TestCreateDeleteModal_RegularIssue(t *testing.T) {
	mockExecutor := mocks.NewMockBQLExecutor(t)
	// No expectations needed - Execute won't be called for non-epic

	issue := &beads.Issue{
		ID:        "test-456",
		TitleText: "Issue to Delete",
		Type:      beads.TypeTask,
	}

	modal, issueIDs := shared.CreateDeleteModal(issue, mockExecutor)

	require.NotNil(t, modal)
	require.Equal(t, []string{"test-456"}, issueIDs, "expected single-element slice with issue ID")
}

func TestCreateDeleteModal_EpicWithoutChildren(t *testing.T) {
	mockExecutor := mocks.NewMockBQLExecutor(t)
	// No expectations needed - Execute won't be called for epic without children

	issue := &beads.Issue{
		ID:        "epic-1",
		TitleText: "Epic Without Children",
		Type:      beads.TypeEpic,
		Children:  []string{}, // No children
	}

	modal, issueIDs := shared.CreateDeleteModal(issue, mockExecutor)

	require.NotNil(t, modal)
	require.Equal(t, []string{"epic-1"}, issueIDs, "expected single-element slice with epic ID")
}

func TestCreateDeleteModal_EpicWithChildren(t *testing.T) {
	mockExecutor := mocks.NewMockBQLExecutor(t)
	mockExecutor.EXPECT().Execute(mock.Anything).Return([]beads.Issue{
		{ID: "epic-1", Type: beads.TypeEpic, TitleText: "Epic With Children"},
		{ID: "task-1", Type: beads.TypeTask, TitleText: "Child 1"},
		{ID: "task-2", Type: beads.TypeTask, TitleText: "Child 2"},
		{ID: "task-3", Type: beads.TypeTask, TitleText: "Child 3"},
	}, nil)

	issue := &beads.Issue{
		ID:        "epic-1",
		TitleText: "Epic With Children",
		Type:      beads.TypeEpic,
		Children:  []string{"task-1", "task-2", "task-3"},
	}

	modal, issueIDs := shared.CreateDeleteModal(issue, mockExecutor)

	require.NotNil(t, modal)
	require.Len(t, issueIDs, 4, "expected 4 IDs (epic + 3 children)")
	require.Contains(t, issueIDs, "epic-1", "expected epic ID in delete list")
	require.Contains(t, issueIDs, "task-1", "expected child task-1 in delete list")
	require.Contains(t, issueIDs, "task-2", "expected child task-2 in delete list")
	require.Contains(t, issueIDs, "task-3", "expected child task-3 in delete list")
}

// =============================================================================
// Entry Point Tests: Verify kanban keys send correct sub-mode messages
// =============================================================================

// createTestModelWithIssue creates a Model with a board that has a selected issue.
func createTestModelWithIssue(issueID string, query string) Model {
	cfg := config.Defaults()
	services := mode.Services{
		Config: &cfg,
	}

	// Create board with a column containing one issue
	boardConfigs := []config.ColumnConfig{
		{Name: "Test", Query: query, Color: "#888888"},
	}
	brd := board.NewFromViews([]config.ViewConfig{{Name: "Test", Columns: boardConfigs}}, nil, nil).SetSize(100, 40)

	// The board columns are unexported, so we use the ColumnLoadedMsg to populate
	// Since we don't have an executor, simulate the load completion
	brd, _ = brd.Update(board.ColumnLoadedMsg{
		ViewIndex:   0,
		ColumnTitle: "Test",
		Issues: []beads.Issue{
			{ID: issueID, TitleText: "Test Issue", Type: beads.TypeTask},
		},
		Err: nil,
	})

	return Model{
		services: services,
		board:    brd,
		width:    100,
		height:   40,
		view:     ViewBoard,
	}
}

func TestKanban_EnterKey_SendsSubModeTree(t *testing.T) {
	m := createTestModelWithIssue("test-123", "status = open")

	// Simulate Enter keypress
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := m.handleBoardKey(msg)

	// Execute the command to get the message
	require.NotNil(t, cmd, "expected command from Enter key")
	result := cmd()

	// Verify it's a SwitchToSearchMsg with SubModeTree
	switchMsg, ok := result.(SwitchToSearchMsg)
	require.True(t, ok, "expected SwitchToSearchMsg, got %T", result)
	require.Equal(t, mode.SubModeTree, switchMsg.SubMode, "expected SubModeTree")
	require.Equal(t, "test-123", switchMsg.IssueID, "expected IssueID to match selected issue")
}

func TestKanban_SlashKey_SendsSubModeList(t *testing.T) {
	m := createTestModelWithIssue("test-789", "priority >= 0")

	// Simulate '/' keypress
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
	_, cmd := m.handleBoardKey(msg)

	// Execute the command to get the message
	require.NotNil(t, cmd, "expected command from '/' key")
	result := cmd()

	// Verify it's a SwitchToSearchMsg with SubModeList
	switchMsg, ok := result.(SwitchToSearchMsg)
	require.True(t, ok, "expected SwitchToSearchMsg, got %T", result)
	require.Equal(t, mode.SubModeList, switchMsg.SubMode, "expected SubModeList")
	require.Equal(t, "priority >= 0", switchMsg.Query, "expected Query to match column BQL")
}

func TestKanban_EnterKey_NoIssue_NoCommand(t *testing.T) {
	// Model with empty board (no issues)
	cfg := config.Defaults()
	services := mode.Services{
		Config: &cfg,
	}

	boardConfigs := []config.ColumnConfig{
		{Name: "Empty", Query: "status = open"},
	}
	brd := board.NewFromViews([]config.ViewConfig{{Name: "Test", Columns: boardConfigs}}, nil, nil).SetSize(100, 40)

	m := Model{
		services: services,
		board:    brd,
		width:    100,
		height:   40,
		view:     ViewBoard,
	}

	// Simulate Enter keypress on empty board
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := m.handleBoardKey(msg)

	// Should return nil command when no issue is selected
	require.Nil(t, cmd, "expected nil command when no issue selected")
}

func TestKanban_TKey_NoIssue_NoCommand(t *testing.T) {
	// Model with empty board (no issues)
	cfg := config.Defaults()
	services := mode.Services{
		Config: &cfg,
	}

	boardConfigs := []config.ColumnConfig{
		{Name: "Empty", Query: "status = open"},
	}
	brd := board.NewFromViews([]config.ViewConfig{{Name: "Test", Columns: boardConfigs}}, nil, nil).SetSize(100, 40)

	m := Model{
		services: services,
		board:    brd,
		width:    100,
		height:   40,
		view:     ViewBoard,
	}

	// Simulate 't' keypress on empty board
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}}
	_, cmd := m.handleBoardKey(msg)

	// Should return nil command when no issue is selected
	require.Nil(t, cmd, "expected nil command when no issue selected")
}

// =============================================================================
// Orchestration Mode Entry Tests
// =============================================================================

func TestKanban_CtrlO_SendsOrchestrationMsg(t *testing.T) {
	m := createTestModelWithIssue("task-123", "status = open")

	// Simulate 'ctrl+o' keypress
	msg := tea.KeyMsg{Type: tea.KeyCtrlO}
	_, cmd := m.handleBoardKey(msg)

	// Execute the command to get the message
	require.NotNil(t, cmd, "expected command from 'ctrl+o' key")
	result := cmd()

	// Verify it's a SwitchToOrchestrationMsg
	_, ok := result.(SwitchToOrchestrationMsg)
	require.True(t, ok, "expected SwitchToOrchestrationMsg, got %T", result)
}

// =============================================================================
// Quit Confirmation Tests
// =============================================================================

func TestKanban_CtrlC_OpensQuitModal(t *testing.T) {
	m := createTestModel(t)
	m.view = ViewBoard

	// Simulate Ctrl+C keypress
	msg := tea.KeyMsg{Type: tea.KeyCtrlC}
	m, cmd := m.handleBoardKey(msg)

	// Should open quit modal, not quit immediately
	require.True(t, m.quitModal.IsVisible(), "expected quitModal to be visible")
	require.Nil(t, cmd, "expected no command (just showing modal)")
}

func TestKanban_QKey_DoesNotQuit(t *testing.T) {
	m := createTestModel(t)
	m.view = ViewBoard

	// Simulate 'q' keypress - should NOT quit
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	m, cmd := m.handleBoardKey(msg)

	// Should not return tea.Quit and should not open quit modal
	require.False(t, m.quitModal.IsVisible(), "expected quitModal to NOT be visible on 'q' key")
	// The command should be nil or delegate to board (not tea.Quit)
	if cmd != nil {
		result := cmd()
		_, isQuit := result.(tea.QuitMsg)
		require.False(t, isQuit, "expected 'q' key to NOT quit")
	}
}

func TestKanban_CtrlC_QuitsWhenModalOpen(t *testing.T) {
	m := createTestModel(t)
	// First, open the quit modal
	m.quitModal.Show()
	require.True(t, m.quitModal.IsVisible(), "precondition: quitModal should be visible")

	// Simulate Ctrl+C while modal is open
	msg := tea.KeyMsg{Type: tea.KeyCtrlC}
	m, cmd := m.Update(msg)

	// Should clear modal and quit
	require.False(t, m.quitModal.IsVisible(), "expected quitModal to be hidden")
	require.NotNil(t, cmd, "expected quit command")
}

func TestKanban_Enter_QuitsWhenModalOpen(t *testing.T) {
	m := createTestModel(t)
	// First, open the quit modal
	m.quitModal.Show()
	require.True(t, m.quitModal.IsVisible(), "precondition: quitModal should be visible")

	// Simulate Enter while modal is open (via modal.SubmitMsg from inner modal)
	msg := modal.SubmitMsg{}
	m, cmd := m.Update(msg)

	// Should clear modal and quit
	require.False(t, m.quitModal.IsVisible(), "expected quitModal to be hidden")
	require.NotNil(t, cmd, "expected quit command")
}

func TestKanban_Escape_DismissesQuitModal(t *testing.T) {
	m := createTestModel(t)
	// First, open the quit modal
	m.quitModal.Show()
	require.True(t, m.quitModal.IsVisible(), "precondition: quitModal should be visible")

	// Simulate Escape while modal is open (via modal.CancelMsg from inner modal)
	msg := modal.CancelMsg{}
	m, cmd := m.Update(msg)

	// Should dismiss modal, not quit
	require.False(t, m.quitModal.IsVisible(), "expected quitModal to be hidden")
	require.Nil(t, cmd, "expected no command (modal dismissed)")
}

func TestKanban_QuitModalSubmit_Quits(t *testing.T) {
	m := createTestModel(t)
	// Open the quit modal
	m.quitModal.Show()
	require.True(t, m.quitModal.IsVisible(), "precondition: quitModal should be visible")

	// Simulate modal submit
	m, cmd := m.Update(modal.SubmitMsg{})

	// Should clear modal and return tea.Quit
	require.False(t, m.quitModal.IsVisible(), "expected quitModal to be hidden")
	require.NotNil(t, cmd, "expected quit command")
	// Note: tea.Quit returns a tea.QuitMsg function, so we verify it's set
}

func TestKanban_QuitModalCancel_DismissesModal(t *testing.T) {
	m := createTestModel(t)
	// Open the quit modal
	m.quitModal.Show()
	require.True(t, m.quitModal.IsVisible(), "precondition: quitModal should be visible")

	// Simulate modal cancel (Esc)
	m, cmd := m.Update(modal.CancelMsg{})

	// Should clear modal, not quit
	require.False(t, m.quitModal.IsVisible(), "expected quitModal to be hidden")
	require.Nil(t, cmd, "expected no command")
}

func TestKanban_HelpView_CtrlC_OpensQuitModal(t *testing.T) {
	m := createTestModel(t)
	m.view = ViewHelp

	// Simulate Ctrl+C in help view
	msg := tea.KeyMsg{Type: tea.KeyCtrlC}
	m, cmd := m.handleKey(msg)

	// Should open quit modal
	require.True(t, m.quitModal.IsVisible(), "expected quitModal to be visible in help view")
	require.Nil(t, cmd, "expected no command")
}

// =============================================================================
// Ctrl+E Issue Editor from Board View Tests
// =============================================================================

func TestKanban_CtrlE_BoardView_EmitsOpenEditMenuMsg(t *testing.T) {
	m := createTestModelWithIssue("test-123", "status = open")

	// Simulate Ctrl+E keypress
	msg := tea.KeyMsg{Type: tea.KeyCtrlE}
	_, cmd := m.handleBoardKey(msg)

	// Execute the command to get the message
	require.NotNil(t, cmd, "expected command from Ctrl+E key")
	result := cmd()

	// Verify it's an OpenEditMenuMsg
	editMsg, ok := result.(OpenEditMenuMsg)
	require.True(t, ok, "expected OpenEditMenuMsg, got %T", result)
	require.Equal(t, "test-123", editMsg.Issue.ID, "expected IssueID to match selected issue")
}

func TestKanban_CtrlE_EmptyBoard_NoOp(t *testing.T) {
	// Model with empty board (no issues)
	cfg := config.Defaults()
	services := mode.Services{
		Config: &cfg,
	}

	boardConfigs := []config.ColumnConfig{
		{Name: "Empty", Query: "status = open"},
	}
	brd := board.NewFromViews([]config.ViewConfig{{Name: "Test", Columns: boardConfigs}}, nil, nil).SetSize(100, 40)

	m := Model{
		services: services,
		board:    brd,
		width:    100,
		height:   40,
		view:     ViewBoard,
	}

	// Simulate Ctrl+E keypress on empty board
	msg := tea.KeyMsg{Type: tea.KeyCtrlE}
	_, cmd := m.handleBoardKey(msg)

	// Should return nil command when no issue is selected
	require.Nil(t, cmd, "expected nil command when no issue selected")
}

func TestKanban_CtrlE_MessageContainsIssueData(t *testing.T) {
	// Create a model with an issue that has specific data
	cfg := config.Defaults()
	services := mode.Services{
		Config: &cfg,
	}

	boardConfigs := []config.ColumnConfig{
		{Name: "Test", Query: "status = open", Color: "#888888"},
	}
	brd := board.NewFromViews([]config.ViewConfig{{Name: "Test", Columns: boardConfigs}}, nil, nil).SetSize(100, 40)

	// Populate with issue that has labels, priority, and status
	brd, _ = brd.Update(board.ColumnLoadedMsg{
		ViewIndex:   0,
		ColumnTitle: "Test",
		Issues: []beads.Issue{
			{
				ID:        "issue-456",
				TitleText: "Test Issue With Data",
				Type:      beads.TypeTask,
				Labels:    []string{"bug", "urgent", "p0"},
				Priority:  beads.PriorityHigh,
				Status:    beads.StatusInProgress,
			},
		},
		Err: nil,
	})

	m := Model{
		services: services,
		board:    brd,
		width:    100,
		height:   40,
		view:     ViewBoard,
	}

	// Simulate Ctrl+E keypress
	msg := tea.KeyMsg{Type: tea.KeyCtrlE}
	_, cmd := m.handleBoardKey(msg)

	require.NotNil(t, cmd, "expected command from Ctrl+E key")
	result := cmd()

	// Verify message contains all correct issue data
	editMsg, ok := result.(OpenEditMenuMsg)
	require.True(t, ok, "expected OpenEditMenuMsg, got %T", result)
	require.Equal(t, "issue-456", editMsg.Issue.ID, "IssueID should match")
	require.Equal(t, []string{"bug", "urgent", "p0"}, editMsg.Issue.Labels, "Labels should match")
	require.Equal(t, beads.PriorityHigh, editMsg.Issue.Priority, "Priority should match")
	require.Equal(t, beads.StatusInProgress, editMsg.Issue.Status, "Status should match")
}

func TestKanban_CtrlE_SaveMsg_ReturnsToBoardView(t *testing.T) {
	m := createTestModelWithIssue("test-123", "status = open")
	require.Equal(t, ViewBoard, m.view, "precondition: should start in board view")

	// Simulate Ctrl+E keypress and process the message
	keyMsg := tea.KeyMsg{Type: tea.KeyCtrlE}
	_, cmd := m.handleBoardKey(keyMsg)
	require.NotNil(t, cmd, "expected command from Ctrl+E key")

	// Execute command to get OpenEditMenuMsg and process it
	result := cmd()
	editMsg, ok := result.(OpenEditMenuMsg)
	require.True(t, ok, "expected OpenEditMenuMsg")

	// Process OpenEditMenuMsg to open the editor
	m, _ = m.Update(editMsg)
	require.Equal(t, ViewEditIssue, m.view, "expected ViewEditIssue after opening editor")

	// Process SaveMsg
	saveMsg := issueeditor.SaveMsg{
		IssueID:  "test-123",
		Priority: beads.PriorityHigh,
		Status:   beads.StatusInProgress,
		Labels:   []string{"updated"},
	}
	m, cmd = m.Update(saveMsg)

	// Should return to board view
	require.Equal(t, ViewBoard, m.view, "expected ViewBoard after save when opened from board")
	require.NotNil(t, cmd, "expected commands for updating issue and refreshing board")
}

func TestKanban_CtrlE_CancelMsg_ReturnsToBoardView(t *testing.T) {
	m := createTestModelWithIssue("test-123", "status = open")
	require.Equal(t, ViewBoard, m.view, "precondition: should start in board view")

	// Simulate Ctrl+E keypress and process the message
	keyMsg := tea.KeyMsg{Type: tea.KeyCtrlE}
	_, cmd := m.handleBoardKey(keyMsg)
	require.NotNil(t, cmd, "expected command from Ctrl+E key")

	// Execute command to get OpenEditMenuMsg and process it
	result := cmd()
	editMsg, ok := result.(OpenEditMenuMsg)
	require.True(t, ok, "expected OpenEditMenuMsg")

	// Process OpenEditMenuMsg to open the editor
	m, _ = m.Update(editMsg)
	require.Equal(t, ViewEditIssue, m.view, "expected ViewEditIssue after opening editor")

	// Process CancelMsg
	cancelMsg := issueeditor.CancelMsg{}
	m, cmd = m.Update(cancelMsg)

	// Should return to board view
	require.Equal(t, ViewBoard, m.view, "expected ViewBoard after cancel when opened from board")
	require.Nil(t, cmd, "expected no command on cancel")
}

// =============================================================================
// Diff Viewer Tests (Ctrl+G)
// =============================================================================

func TestKanban_CtrlG_OpensDiffViewer(t *testing.T) {
	m := createTestModel(t)
	m.view = ViewBoard

	// Simulate Ctrl+G keypress
	msg := tea.KeyMsg{Type: tea.KeyCtrlG}
	_, cmd := m.handleBoardKey(msg)

	// Execute the command to get the message
	require.NotNil(t, cmd, "expected command from Ctrl+G key")
	result := cmd()

	// Verify it's a ShowDiffViewerMsg
	_, ok := result.(diffviewer.ShowDiffViewerMsg)
	require.True(t, ok, "expected diffviewer.ShowDiffViewerMsg, got %T", result)
}
