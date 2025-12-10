package kanban

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"

	"perles/internal/beads"
	"perles/internal/config"
	"perles/internal/mode"
	"perles/internal/mode/shared"
	"perles/internal/ui/board"
	"perles/internal/ui/details"
	"perles/internal/ui/shared/modal"
)

// createTestModel creates a minimal Model for testing state transitions.
// It does not require a database connection.
func createTestModel() Model {
	cfg := config.Defaults()
	services := mode.Services{
		Config:    &cfg,
		Clipboard: shared.MockClipboard{},
	}

	return Model{
		services: services,
		width:    100,
		height:   40,
		view:     ViewBoard,
	}
}

func TestDeleteFlow_CancelReturnsToDetails(t *testing.T) {
	m := createTestModel()

	// Set up delete confirm view
	issue := beads.Issue{
		ID:        "test-123",
		TitleText: "Test Issue",
		Type:      beads.TypeTask,
	}
	m.view = ViewDeleteConfirm
	m.details = details.New(issue, nil, nil).SetSize(100, 40)
	m.selectedIssue = &issue

	// Simulate modal cancel
	m, _ = m.handleModalCancel()

	require.Equal(t, ViewDetails, m.view, "expected ViewDetails after cancel")
	require.Nil(t, m.selectedIssue, "expected selectedIssue to be cleared")
}

func TestDeleteFlow_SubmitTriggersDelete(t *testing.T) {
	m := createTestModel()

	// Set up delete confirm view with selected issue
	issue := beads.Issue{
		ID:        "test-123",
		TitleText: "Test Issue",
		Type:      beads.TypeTask,
	}
	m.view = ViewDeleteConfirm
	m.selectedIssue = &issue

	// Simulate modal submit
	m, cmd := m.handleModalSubmit(modal.SubmitMsg{})

	// Should return a delete command
	require.NotNil(t, cmd, "expected delete command")
	require.Nil(t, m.selectedIssue, "expected selectedIssue to be cleared")
}

func TestDeleteFlow_IssueDeletedMsgReturnsToBoard(t *testing.T) {
	m := createTestModel()

	// Simulate receiving success message
	msg := issueDeletedMsg{
		issueID: "test-123",
		err:     nil,
	}
	m, cmd := m.handleIssueDeleted(msg)

	require.Equal(t, ViewBoard, m.view, "expected ViewBoard after successful delete")
	// The command should include a ShowToastMsg emission (app now owns toaster)
	require.NotNil(t, cmd, "expected command for toast message")
}

func TestDeleteFlow_IssueDeletedMsgWithErrorShowsError(t *testing.T) {
	m := createTestModel()

	// Simulate receiving error message
	msg := issueDeletedMsg{
		issueID: "test-123",
		err:     errors.New("test error"),
	}
	m, _ = m.handleIssueDeleted(msg)

	require.Equal(t, ViewBoard, m.view, "expected ViewBoard after error")
	require.Error(t, m.err, "expected error to be set")
	require.Equal(t, "deleting issue", m.errContext)
}

func TestCreateDeleteModal_RegularIssue(t *testing.T) {
	m := createTestModel()

	issue := &beads.Issue{
		ID:        "test-456",
		TitleText: "Issue to Delete",
		Type:      beads.TypeTask,
	}

	modal, isCascade := shared.CreateDeleteModal(issue, m.services.Executor)

	require.NotNil(t, modal)
	require.False(t, isCascade, "expected non-cascade for regular task")
}

func TestCreateDeleteModal_EpicWithoutChildren(t *testing.T) {
	m := createTestModel()

	issue := &beads.Issue{
		ID:        "epic-1",
		TitleText: "Epic Without Children",
		Type:      beads.TypeEpic,
		Blocks:    []string{}, // No children
	}

	modal, isCascade := shared.CreateDeleteModal(issue, m.services.Executor)

	require.NotNil(t, modal)
	require.False(t, isCascade, "expected non-cascade for epic without children")
}

func TestCreateDeleteModal_EpicWithChildren(t *testing.T) {
	m := createTestModel()

	issue := &beads.Issue{
		ID:        "epic-1",
		TitleText: "Epic With Children",
		Type:      beads.TypeEpic,
		Children:  []string{"task-1", "task-2", "task-3"},
	}

	modal, isCascade := shared.CreateDeleteModal(issue, m.services.Executor)

	require.NotNil(t, modal)
	require.True(t, isCascade, "expected cascade for epic with children")
}

func TestDeleteFlow_CascadeSubmit(t *testing.T) {
	m := createTestModel()

	// Set up cascade delete scenario
	issue := beads.Issue{
		ID:        "epic-1",
		TitleText: "Epic With Children",
		Type:      beads.TypeEpic,
		Blocks:    []string{"task-1", "task-2"},
	}
	m.view = ViewDeleteConfirm
	m.selectedIssue = &issue
	m.deleteIsCascade = true

	// Simulate modal submit
	m, cmd := m.handleModalSubmit(modal.SubmitMsg{})

	// Should return a delete command
	require.NotNil(t, cmd, "expected delete command")
	require.Nil(t, m.selectedIssue, "expected selectedIssue to be cleared")
	require.False(t, m.deleteIsCascade, "expected deleteIsCascade to be cleared")
}

func TestDeleteFlow_CancelClearsCascadeFlag(t *testing.T) {
	m := createTestModel()

	issue := beads.Issue{
		ID:        "epic-1",
		TitleText: "Epic",
		Type:      beads.TypeEpic,
	}
	m.view = ViewDeleteConfirm
	m.selectedIssue = &issue
	m.deleteIsCascade = true

	// Simulate cancel
	m, _ = m.handleModalCancel()

	require.False(t, m.deleteIsCascade, "expected deleteIsCascade to be cleared on cancel")
}

func TestDeleteFlow_SubmitWithNoSelectedIssue(t *testing.T) {
	m := createTestModel()

	// Set up delete confirm view but NO selected issue
	m.view = ViewDeleteConfirm
	m.selectedIssue = nil

	// Simulate modal submit
	m, cmd := m.handleModalSubmit(modal.SubmitMsg{})

	// Should return to board, not crash
	require.Equal(t, ViewBoard, m.view, "expected ViewBoard when no issue selected")
	require.Nil(t, cmd, "expected no command when no issue selected")
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
	brd := board.NewFromConfig(boardConfigs).SetSize(100, 40)

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
	brd := board.NewFromConfig(boardConfigs).SetSize(100, 40)

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
	brd := board.NewFromConfig(boardConfigs).SetSize(100, 40)

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
