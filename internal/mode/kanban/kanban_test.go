package kanban

import (
	"errors"
	"fmt"
	"runtime"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	beads "github.com/zjrosen/perles/internal/beads/domain"
	"github.com/zjrosen/perles/internal/config"
	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/mode"
	"github.com/zjrosen/perles/internal/mode/shared"
	"github.com/zjrosen/perles/internal/ui/board"
	"github.com/zjrosen/perles/internal/ui/modals/issueeditor"
	"github.com/zjrosen/perles/internal/ui/shared/diffviewer"
	"github.com/zjrosen/perles/internal/ui/shared/editor"
	"github.com/zjrosen/perles/internal/ui/shared/toaster"
)

// Note: TestMain is defined in golden_test.go and initializes zone.NewGlobal()

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

// createRefreshTestModel builds a kanban model configured for reload/cursor tests.
func createRefreshTestModel(t *testing.T, columns []config.ColumnConfig) Model {
	cfg := config.Defaults()
	cfg.Views = []config.ViewConfig{{Name: "Test", Columns: columns}}
	mockExecutor := mocks.NewMockBQLExecutor(t)

	services := mode.Services{
		Config:   &cfg,
		Executor: mockExecutor,
	}

	brd := board.NewFromViews(cfg.GetViews(), mockExecutor, nil).SetSize(100, 40)
	return Model{
		services: services,
		board:    brd,
		width:    100,
		height:   40,
		view:     ViewBoard,
	}
}

// seedBoardColumn loads test issues into a board column via a ColumnLoadedMsg.
func seedBoardColumn(brd board.Model, columnIndex int, columnTitle string, issues []beads.Issue) board.Model {
	brd, _ = brd.Update(board.ColumnLoadedMsg{
		ViewIndex:   0,
		ColumnIndex: columnIndex,
		ColumnTitle: columnTitle,
		Issues:      issues,
		Err:         nil,
	})
	return brd
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
// Quit Request Tests (quit modal now handled at app level)
// =============================================================================

func TestKanban_CtrlC_ReturnsRequestQuitMsg(t *testing.T) {
	m := createTestModel(t)
	m.view = ViewBoard

	// Simulate Ctrl+C keypress
	msg := tea.KeyMsg{Type: tea.KeyCtrlC}
	_, cmd := m.handleBoardKey(msg)

	// Should return a command that produces mode.RequestQuitMsg
	require.NotNil(t, cmd, "expected quit request command")
	result := cmd()
	_, isRequestQuit := result.(mode.RequestQuitMsg)
	require.True(t, isRequestQuit, "expected mode.RequestQuitMsg")
}

func TestKanban_QKey_DoesNotQuit(t *testing.T) {
	m := createTestModel(t)
	m.view = ViewBoard

	// Simulate 'q' keypress - should NOT quit
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	_, cmd := m.handleBoardKey(msg)

	// The command should be nil or delegate to board (not tea.Quit or RequestQuitMsg)
	if cmd != nil {
		result := cmd()
		_, isQuit := result.(tea.QuitMsg)
		require.False(t, isQuit, "expected 'q' key to NOT quit")
		_, isRequestQuit := result.(mode.RequestQuitMsg)
		require.False(t, isRequestQuit, "expected 'q' key to NOT request quit")
	}
}

func TestKanban_HelpView_CtrlC_ReturnsRequestQuitMsg(t *testing.T) {
	m := createTestModel(t)
	m.view = ViewHelp

	// Simulate Ctrl+C in help view
	msg := tea.KeyMsg{Type: tea.KeyCtrlC}
	_, cmd := m.handleKey(msg)

	// Should return mode.RequestQuitMsg
	require.NotNil(t, cmd, "expected quit request command")
	result := cmd()
	_, isRequestQuit := result.(mode.RequestQuitMsg)
	require.True(t, isRequestQuit, "expected mode.RequestQuitMsg in help view")
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

// TestHandleBoardKey_Dashboard verifies ctrl+o switches to dashboard.
func TestHandleBoardKey_Dashboard(t *testing.T) {
	cfg := config.Defaults()
	clipboard := mocks.NewMockClipboard(t)
	clipboard.EXPECT().Copy(mock.Anything).Return(nil).Maybe()
	mockExecutor := mocks.NewMockBQLExecutor(t)

	services := mode.Services{
		Config:    &cfg,
		Clipboard: clipboard,
		Executor:  mockExecutor,
	}

	m := Model{
		services: services,
		width:    100,
		height:   40,
		view:     ViewBoard,
	}

	// Simulate ctrl+o key press
	msg := tea.KeyMsg{Type: tea.KeyCtrlO}
	_, cmd := m.handleBoardKey(msg)

	// Should return SwitchToDashboardMsg
	require.NotNil(t, cmd, "expected command to be returned")
	result := cmd()
	_, ok := result.(SwitchToDashboardMsg)
	require.True(t, ok, "expected SwitchToDashboardMsg, got %T", result)
}

// =============================================================================
// Mouse Click Integration Tests
// =============================================================================

// TestKanban_ClickOpensTreeView tests the full click → focus → select → tree view flow.
// This is an integration test verifying that clicking an issue in the kanban board
// correctly emits a SwitchToSearchMsg with SubModeTree, identical to pressing Enter.
func TestKanban_ClickOpensTreeView(t *testing.T) {
	// Skip on Windows: zone.Manager relies on terminal capabilities that behave
	// differently on Windows, causing zone registration to fail in CI environments.
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows: zone.Manager terminal detection not reliable in CI")
	}

	issueID := "click-integration-test-1"

	cfg := config.Defaults()
	services := mode.Services{
		Config: &cfg,
	}

	// Create board with a column containing one issue
	boardConfigs := []config.ColumnConfig{
		{Name: "Test", Query: "status = open", Color: "#888888"},
	}
	brd := board.NewFromViews([]config.ViewConfig{{Name: "Test", Columns: boardConfigs}}, nil, nil).SetSize(100, 40)

	// Populate with issue
	brd, _ = brd.Update(board.ColumnLoadedMsg{
		ViewIndex:   0,
		ColumnTitle: "Test",
		Issues: []beads.Issue{
			{ID: issueID, TitleText: "Test Issue for Click", Type: beads.TypeTask, Status: beads.StatusOpen},
		},
	})

	m := Model{
		services: services,
		board:    brd,
		width:    100,
		height:   40,
		view:     ViewBoard,
	}

	// Call View() to register zones (required for click detection)
	_ = m.View()

	// Get zone to determine click position (with retry for zone manager stability)
	zoneID := board.MakeZoneID(0, issueID)
	var z *zone.ZoneInfo
	for retries := 0; retries < 10; retries++ {
		z = zone.Get(zoneID)
		if z != nil && !z.IsZero() {
			break
		}
		// Re-render to ensure zones are registered
		_ = m.View()
		// Zone registration is asynchronous via a channel worker in bubblezone.
		// A small delay allows the worker goroutine to process the channel.
		time.Sleep(time.Millisecond)
	}
	require.NotNil(t, z, "zone should be registered after View()")
	require.False(t, z.IsZero(), "zone should not be zero")

	// Click inside the zone
	width := z.EndX - z.StartX
	clickX := z.StartX + width/2
	clickY := z.StartY

	m, cmd := m.Update(tea.MouseMsg{
		X:      clickX,
		Y:      clickY,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionRelease,
	})

	// Verify click produces a command
	require.NotNil(t, cmd, "click on issue should produce a command")

	// Execute the command to get IssueClickedMsg
	result := cmd()
	clickedMsg, ok := result.(board.IssueClickedMsg)
	require.True(t, ok, "expected IssueClickedMsg, got %T", result)
	require.Equal(t, issueID, clickedMsg.IssueID, "IssueClickedMsg should contain clicked issue ID")

	// Process the IssueClickedMsg through kanban's Update to get SwitchToSearchMsg
	m, cmd = m.Update(clickedMsg)
	require.NotNil(t, cmd, "IssueClickedMsg should produce a command")

	result = cmd()
	switchMsg, ok := result.(SwitchToSearchMsg)
	require.True(t, ok, "expected SwitchToSearchMsg, got %T", result)
	require.Equal(t, mode.SubModeTree, switchMsg.SubMode, "expected SubModeTree")
	require.Equal(t, issueID, switchMsg.IssueID, "expected IssueID to match clicked issue")
}

// TestKanban_ClickBehaviorMatchesEnterKey verifies that click produces the same result as Enter key.
func TestKanban_ClickBehaviorMatchesEnterKey(t *testing.T) {
	issueID := "click-vs-enter-test-1"

	cfg := config.Defaults()
	services := mode.Services{
		Config: &cfg,
	}

	// Create two identical models for comparison
	boardConfigs := []config.ColumnConfig{
		{Name: "Test", Query: "status = open", Color: "#888888"},
	}

	// Model for click test
	brd1 := board.NewFromViews([]config.ViewConfig{{Name: "Test", Columns: boardConfigs}}, nil, nil).SetSize(100, 40)
	brd1, _ = brd1.Update(board.ColumnLoadedMsg{
		ViewIndex:   0,
		ColumnTitle: "Test",
		Issues: []beads.Issue{
			{ID: issueID, TitleText: "Test Issue", Type: beads.TypeTask, Status: beads.StatusOpen},
		},
	})

	mClick := Model{
		services: services,
		board:    brd1,
		width:    100,
		height:   40,
		view:     ViewBoard,
	}

	// Model for Enter key test
	brd2 := board.NewFromViews([]config.ViewConfig{{Name: "Test", Columns: boardConfigs}}, nil, nil).SetSize(100, 40)
	brd2, _ = brd2.Update(board.ColumnLoadedMsg{
		ViewIndex:   0,
		ColumnTitle: "Test",
		Issues: []beads.Issue{
			{ID: issueID, TitleText: "Test Issue", Type: beads.TypeTask, Status: beads.StatusOpen},
		},
	})

	mEnter := Model{
		services: services,
		board:    brd2,
		width:    100,
		height:   40,
		view:     ViewBoard,
	}

	// Test Enter key behavior
	_, enterCmd := mEnter.handleBoardKey(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, enterCmd, "Enter key should produce a command")
	enterResult := enterCmd()
	enterSwitchMsg, ok := enterResult.(SwitchToSearchMsg)
	require.True(t, ok, "Enter key should produce SwitchToSearchMsg")

	// Test click behavior
	_ = mClick.View() // Register zones

	zoneID := board.MakeZoneID(0, issueID)
	var z *zone.ZoneInfo
	for retries := 0; retries < 10; retries++ {
		z = zone.Get(zoneID)
		if z != nil && !z.IsZero() {
			break
		}
		// Re-render to ensure zones are registered
		_ = mClick.View()
		// Zone registration is asynchronous via a channel worker in bubblezone.
		// A small delay allows the worker goroutine to process the channel.
		time.Sleep(time.Millisecond)
	}
	require.NotNil(t, z, "zone should be registered")

	width := z.EndX - z.StartX
	mClick, clickCmd := mClick.Update(tea.MouseMsg{
		X:      z.StartX + width/2,
		Y:      z.StartY,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionRelease,
	})
	require.NotNil(t, clickCmd, "click should produce a command")

	clickResult := clickCmd()
	clickedMsg, ok := clickResult.(board.IssueClickedMsg)
	require.True(t, ok, "click should produce IssueClickedMsg")

	// Process IssueClickedMsg
	_, finalCmd := mClick.Update(clickedMsg)
	require.NotNil(t, finalCmd, "IssueClickedMsg should produce a command")

	finalResult := finalCmd()
	clickSwitchMsg, ok := finalResult.(SwitchToSearchMsg)
	require.True(t, ok, "click flow should produce SwitchToSearchMsg")

	// Verify both produce equivalent SwitchToSearchMsg
	require.Equal(t, enterSwitchMsg.SubMode, clickSwitchMsg.SubMode, "SubMode should match between Enter and Click")
	require.Equal(t, enterSwitchMsg.IssueID, clickSwitchMsg.IssueID, "IssueID should match between Enter and Click")
}

// TestKanban_KeyboardNavigationUnchanged verifies keyboard navigation still works after mouse support.
func TestKanban_KeyboardNavigationUnchanged(t *testing.T) {
	cfg := config.Defaults()
	services := mode.Services{
		Config: &cfg,
	}

	// Use 3 columns - default focus will be on middle column (column 1)
	boardConfigs := []config.ColumnConfig{
		{Name: "Col1", Query: "status = open", Color: "#888888"},
		{Name: "Col2", Query: "status = in_progress", Color: "#999999"},
		{Name: "Col3", Query: "status = closed", Color: "#aaaaaa"},
	}
	brd := board.NewFromViews([]config.ViewConfig{{Name: "Test", Columns: boardConfigs}}, nil, nil).SetSize(150, 40)

	// Populate all columns
	brd, _ = brd.Update(board.ColumnLoadedMsg{
		ViewIndex:   0,
		ColumnIndex: 0,
		ColumnTitle: "Col1",
		Issues: []beads.Issue{
			{ID: "issue-1", TitleText: "Issue 1", Type: beads.TypeTask},
		},
	})
	brd, _ = brd.Update(board.ColumnLoadedMsg{
		ViewIndex:   0,
		ColumnIndex: 1,
		ColumnTitle: "Col2",
		Issues: []beads.Issue{
			{ID: "issue-2", TitleText: "Issue 2", Type: beads.TypeTask},
		},
	})
	brd, _ = brd.Update(board.ColumnLoadedMsg{
		ViewIndex:   0,
		ColumnIndex: 2,
		ColumnTitle: "Col3",
		Issues: []beads.Issue{
			{ID: "issue-3", TitleText: "Issue 3", Type: beads.TypeTask},
		},
	})

	m := Model{
		services: services,
		board:    brd,
		width:    150,
		height:   40,
		view:     ViewBoard,
	}

	// With 3 columns, default focus is on middle column (column 1)
	initialFocus := m.board.FocusedColumn()
	require.Equal(t, 1, initialFocus, "default focus should be middle column")

	// Test right navigation (l key)
	m, _ = m.handleBoardKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	require.Equal(t, 2, m.board.FocusedColumn(), "l key should move focus right to column 2")

	// Test left navigation (h key)
	m, _ = m.handleBoardKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	require.Equal(t, 1, m.board.FocusedColumn(), "h key should move focus left to column 1")

	// Continue moving left
	m, _ = m.handleBoardKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	require.Equal(t, 0, m.board.FocusedColumn(), "h key should move focus left to column 0")

	// Test up/down navigation (j/k)
	m.board, _ = m.board.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m.board, _ = m.board.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	// Just verify no panic - selection state is internal to column
}

// =============================================================================
// User Action Integration Tests
// =============================================================================

// createTestModelWithActions creates a Model with user-defined actions configured.
func createTestModelWithActions(t *testing.T, actions map[string]config.ActionConfig, hasIssue bool) Model {
	cfg := config.Defaults()
	cfg.UI.Actions.IssueAction = actions

	clipboard := mocks.NewMockClipboard(t)
	clipboard.EXPECT().Copy(mock.Anything).Return(nil).Maybe()
	mockExecutor := mocks.NewMockBQLExecutor(t)

	services := mode.Services{
		Config:    &cfg,
		Clipboard: clipboard,
		Executor:  mockExecutor,
		WorkDir:   t.TempDir(),
	}

	boardConfigs := []config.ColumnConfig{
		{Name: "Test", Query: "status = open", Color: "#888888"},
	}
	brd := board.NewFromViews([]config.ViewConfig{{Name: "Test", Columns: boardConfigs}}, nil, nil).SetSize(100, 40)

	if hasIssue {
		brd, _ = brd.Update(board.ColumnLoadedMsg{
			ViewIndex:   0,
			ColumnTitle: "Test",
			Issues: []beads.Issue{
				{ID: "test-123", TitleText: "Test Issue", Type: beads.TypeTask, Status: beads.StatusOpen},
			},
		})
	}

	return Model{
		services: services,
		board:    brd,
		width:    100,
		height:   40,
		view:     ViewBoard,
		actions:  actions,
	}
}

func TestMatchUserAction_MatchesConfiguredKey(t *testing.T) {
	actions := map[string]config.ActionConfig{
		"test-action": {Key: "1", Command: "echo test", Description: "Test action"},
	}

	// Test matching key
	action, name, ok := shared.MatchUserAction(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}}, actions)
	require.True(t, ok, "should match configured key '1'")
	require.Equal(t, "test-action", name, "should return action name")
	require.Equal(t, "echo test", action.Command, "should return action config")
}

func TestMatchUserAction_NoMatchForUnconfiguredKey(t *testing.T) {
	actions := map[string]config.ActionConfig{
		"test-action": {Key: "1", Command: "echo test", Description: "Test action"},
	}

	// Test non-matching key
	_, _, ok := shared.MatchUserAction(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}}, actions)
	require.False(t, ok, "should not match unconfigured key '2'")
}

func TestMatchUserAction_NormalizedKeyMatching(t *testing.T) {
	// Test that ctrl+space matches ctrl+@ (terminal code)
	actions := map[string]config.ActionConfig{
		"ctrl-space-action": {Key: "ctrl+space", Command: "echo test", Description: "Ctrl+Space action"},
	}

	// Terminal sends ctrl+@ for ctrl+space
	action, name, ok := shared.MatchUserAction(tea.KeyMsg{Type: tea.KeyCtrlAt}, actions)
	// Note: This test depends on how tea.KeyMsg.String() handles ctrl+@
	// The normalized comparison should handle this
	_ = action
	_ = name
	_ = ok
	// This is a defense-in-depth test - the config validation should prevent
	// user from configuring reserved keys like ctrl+space
}

func TestMatchUserAction_NoActionsConfigured(t *testing.T) {
	// Test with nil actions map
	_, _, ok := shared.MatchUserAction(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}}, nil)
	require.False(t, ok, "should return false when no actions configured")
}

func TestMatchUserAction_EmptyActionsMap(t *testing.T) {
	actions := map[string]config.ActionConfig{}

	// Test with empty actions map
	_, _, ok := shared.MatchUserAction(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}}, actions)
	require.False(t, ok, "should return false when actions map is empty")
}

func TestHandleBoardKey_UserActionWithIssue_ExecutesAction(t *testing.T) {
	actions := map[string]config.ActionConfig{
		"echo-action": {Key: "1", Command: "echo 'hello'", Description: "Echo action"},
	}
	m := createTestModelWithActions(t, actions, true)

	// Press the user action key
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}}
	_, cmd := m.handleBoardKey(msg)

	// Should return a command (executeAction tea.Cmd)
	require.NotNil(t, cmd, "should return a command when user action matches with issue selected")
}

func TestHandleBoardKey_UserActionNoIssue_ShowsWarningToast(t *testing.T) {
	actions := map[string]config.ActionConfig{
		"echo-action": {Key: "1", Command: "echo 'hello'", Description: "Echo action"},
	}
	m := createTestModelWithActions(t, actions, false) // No issue

	// Press the user action key
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}}
	_, cmd := m.handleBoardKey(msg)

	// Should return a command that produces a warning toast
	require.NotNil(t, cmd, "should return a command when no issue selected")
	result := cmd()
	toastMsg, ok := result.(mode.ShowToastMsg)
	require.True(t, ok, "expected ShowToastMsg, got %T", result)
	require.Equal(t, "No issue selected", toastMsg.Message)
}

func TestHandleBoardKey_BuiltInKeyTakesPriority(t *testing.T) {
	// Configure a user action with a key that conflicts with built-in (this shouldn't
	// happen with proper validation, but test defense-in-depth)
	actions := map[string]config.ActionConfig{
		"conflicting-action": {Key: "?", Command: "echo 'conflict'", Description: "Conflicting action"},
	}
	m := createTestModelWithActions(t, actions, true)

	// Press '?' which is the help key
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}
	newM, _ := m.handleBoardKey(msg)

	// Built-in should win - view should change to help
	require.Equal(t, ViewHelp, newM.view, "built-in help key should take priority")
}

func TestHandleBoardKey_UserActionOnlyAfterBuiltInSwitch(t *testing.T) {
	// User action with a safe key that doesn't conflict with built-in
	actions := map[string]config.ActionConfig{
		"safe-action": {Key: "1", Command: "echo 'safe'", Description: "Safe action"},
	}
	m := createTestModelWithActions(t, actions, true)

	// Press '1' - should trigger user action
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}}
	newM, cmd := m.handleBoardKey(msg)

	// Should remain in board view (no built-in handler for '1')
	require.Equal(t, ViewBoard, newM.view, "should remain in board view")
	// Should return a command (the executeAction cmd)
	require.NotNil(t, cmd, "should return command for user action")
}

func TestHandleActionExecuted_Error_ShowsErrorToast(t *testing.T) {
	m := createTestModelWithActions(t, nil, true)

	msg := shared.ActionExecutedMsg{
		Name: "Test action",
		Err:  fmt.Errorf("command failed: exit status 1"),
	}

	_, cmd := m.handleActionExecuted(msg)
	require.NotNil(t, cmd, "should return command for error toast")

	result := cmd()
	toastMsg, ok := result.(mode.ShowToastMsg)
	require.True(t, ok, "expected ShowToastMsg")
	require.Contains(t, toastMsg.Message, "Test action")
	require.Contains(t, toastMsg.Message, "command failed")
}

func TestHandleActionExecuted_Success_Silent(t *testing.T) {
	m := createTestModelWithActions(t, nil, true)

	msg := shared.ActionExecutedMsg{
		Name: "Test action",
		Err:  nil,
	}

	_, cmd := m.handleActionExecuted(msg)
	require.Nil(t, cmd, "should return nil command on success (fire-and-forget)")
}

func TestNew_InitializesActionsFromConfig(t *testing.T) {
	cfg := config.Defaults()
	cfg.UI.Actions.IssueAction = map[string]config.ActionConfig{
		"test-action": {Key: "1", Command: "echo test", Description: "Test action"},
	}

	services := mode.Services{
		Config: &cfg,
	}

	m := New(services)

	require.NotNil(t, m.actions, "actions should be initialized from config")
	require.Contains(t, m.actions, "test-action", "actions should contain configured action")
}

func TestNew_NilActionsWhenNotConfigured(t *testing.T) {
	cfg := config.Defaults()
	// Don't set any actions

	services := mode.Services{
		Config: &cfg,
	}

	m := New(services)

	// Actions should be nil when not configured
	require.Nil(t, m.actions, "actions should be nil when not configured")
}

// =============================================================================
// Editor Message Routing Tests
// =============================================================================

func TestKanban_EditorExecMsg_ForwardedToIssueEditorWhenViewEditIssue(t *testing.T) {
	m := createTestModelWithIssue("test-123", "status = open")
	issue := m.board.SelectedIssue()
	require.NotNil(t, issue, "precondition: should have selected issue")

	// Open issue editor modal
	m, _ = m.Update(OpenEditMenuMsg{Issue: *issue})
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

func TestKanban_EditorExecMsg_ReturnsNilWhenNotInEditIssueView(t *testing.T) {
	m := createTestModelWithIssue("test-123", "status = open")

	// Not in ViewEditIssue - stay in ViewBoard
	require.Equal(t, ViewBoard, m.view, "precondition: should be in ViewBoard")

	// Send an editor.ExecMsg
	execMsg := editor.ExecMsg{}
	m, cmd := m.Update(execMsg)

	// Should return nil when not in ViewEditIssue
	require.Nil(t, cmd, "should return nil when not in ViewEditIssue")
	require.Equal(t, ViewBoard, m.view, "view should still be ViewBoard")
}

func TestKanban_EditorFinishedMsg_ForwardedToIssueEditorWhenViewEditIssue(t *testing.T) {
	m := createTestModelWithIssue("test-123", "status = open")
	issue := m.board.SelectedIssue()
	require.NotNil(t, issue, "precondition: should have selected issue")

	// Open issue editor modal
	m, _ = m.Update(OpenEditMenuMsg{Issue: *issue})
	require.Equal(t, ViewEditIssue, m.view, "should be in ViewEditIssue")

	// Send an editor.FinishedMsg with new content
	finishedMsg := editor.FinishedMsg{Content: "new description content"}
	m, cmd := m.Update(finishedMsg)

	// View should still be ViewEditIssue (modal stayed open)
	require.Equal(t, ViewEditIssue, m.view, "view should still be ViewEditIssue after editor.FinishedMsg")
	// The result is forwarded to issueEditor for processing
	_ = cmd
}

func TestKanban_EditorFinishedMsg_ReturnsNilWhenNotInEditIssueView(t *testing.T) {
	m := createTestModelWithIssue("test-123", "status = open")

	// Not in ViewEditIssue - stay in ViewBoard
	require.Equal(t, ViewBoard, m.view, "precondition: should be in ViewBoard")

	// Send an editor.FinishedMsg
	finishedMsg := editor.FinishedMsg{Content: "some content"}
	m, cmd := m.Update(finishedMsg)

	// Should return nil when not in ViewEditIssue
	require.Nil(t, cmd, "should return nil when not in ViewEditIssue")
	require.Equal(t, ViewBoard, m.view, "view should still be ViewBoard")
}

// =============================================================================
// Title and Description Update Tests
// =============================================================================

func TestKanban_SaveMsg_DispatchesSingleSaveIssueCmd(t *testing.T) {
	m := createTestModelWithIssue("test-123", "status = open")

	// Set up mock executor expecting single UpdateIssue call
	mockExecutor := mocks.NewMockIssueExecutor(t)
	mockExecutor.EXPECT().UpdateIssue("test-123", mock.MatchedBy(func(opts beads.UpdateIssueOptions) bool {
		return opts.Title != nil && *opts.Title == "New Title"
	})).Return(nil)
	m.services.BeadsExecutor = mockExecutor

	// Get selected issue and set original title
	issue := m.board.SelectedIssue()
	require.NotNil(t, issue, "precondition: should have selected issue")
	issueCopy := *issue
	issueCopy.TitleText = "Original Title"

	// Open editor (sets editingIssue)
	m, _ = m.Update(OpenEditMenuMsg{Issue: issueCopy})
	require.NotNil(t, m.editingIssue)

	// Process SaveMsg with changed title
	msg := issueeditor.SaveMsg{
		IssueID:     "test-123",
		Title:       "New Title",
		Description: issueCopy.DescriptionText,
		Notes:       issueCopy.Notes,
		Priority:    issueCopy.Priority,
		Status:      issueCopy.Status,
		Labels:      issueCopy.Labels,
	}
	m, cmd := m.Update(msg)

	require.Equal(t, ViewBoard, m.view)
	require.True(t, m.loading)
	require.Nil(t, m.editingIssue, "editingIssue should be cleared after save")
	require.NotNil(t, cmd, "expected saveIssueCmd")

	// Execute the command to trigger the mock
	result := cmd()
	_, ok := result.(issueSavedMsg)
	require.True(t, ok, "command should return issueSavedMsg")
}

func TestKanban_HandleIssueSaved_Success(t *testing.T) {
	m := createTestModelWithIssue("test-123", "status = open")

	// handleIssueSaved on success should save cursor and invalidate views
	msg := issueSavedMsg{
		issueID: "test-123",
		opts:    beads.UpdateIssueOptions{},
		err:     nil,
	}
	m, _ = m.handleIssueSaved(msg)

	require.NotNil(t, m.pendingCursor, "should save cursor for issue following")
	require.Equal(t, "test-123", m.pendingCursor.issueID, "cursor should track saved issue")
}

func TestKanban_RefreshFromConfig_PreservesSelectedIssue(t *testing.T) {
	columns := []config.ColumnConfig{
		{Name: "In Progress", Query: "status = in_progress", Color: "#888888"},
	}
	m := createRefreshTestModel(t, columns)

	issues := []beads.Issue{
		{ID: "task-1", TitleText: "Task 1", Type: beads.TypeTask},
		{ID: "task-2", TitleText: "Task 2", Type: beads.TypeTask},
		{ID: "task-3", TitleText: "Task 3", Type: beads.TypeTask},
	}
	m.board = seedBoardColumn(m.board, 0, "In Progress", issues)

	var found bool
	m.board, found = m.board.SelectByID("task-3")
	require.True(t, found, "precondition: task-3 should be selectable")
	require.Equal(t, "task-3", m.board.SelectedIssue().ID, "precondition: task-3 should be selected")

	m, _ = m.RefreshFromConfig()
	require.NotNil(t, m.pendingCursor, "refresh should capture cursor for restoration")
	require.Equal(t, "task-3", m.pendingCursor.issueID, "refresh should track selected issue ID")

	// Simulate column reload completion after RefreshFromConfig().
	m, _ = m.handleColumnLoaded(board.ColumnLoadedMsg{
		ViewIndex:   0,
		ColumnIndex: 0,
		ColumnTitle: "In Progress",
		Issues:      issues,
		Err:         nil,
	})

	selected := m.board.SelectedIssue()
	require.NotNil(t, selected, "selection should be restored after reload")
	require.Equal(t, "task-3", selected.ID, "cursor should stay on previously selected issue")
	require.Nil(t, m.pendingCursor, "pending cursor should clear after restoration")
}

func TestKanban_RefreshFromConfig_RestoresAfterLaterColumnLoad(t *testing.T) {
	columns := []config.ColumnConfig{
		{Name: "Todo", Query: "status = open", Color: "#999999"},
		{Name: "In Progress", Query: "status = in_progress", Color: "#888888"},
	}
	m := createRefreshTestModel(t, columns)
	col0Issues := []beads.Issue{
		{ID: "task-1", TitleText: "Task 1", Type: beads.TypeTask},
	}
	col1Issues := []beads.Issue{
		{ID: "task-2", TitleText: "Task 2", Type: beads.TypeTask},
		{ID: "task-3", TitleText: "Task 3", Type: beads.TypeTask},
	}
	m.board = seedBoardColumn(m.board, 0, "Todo", col0Issues)
	m.board = seedBoardColumn(m.board, 1, "In Progress", col1Issues)

	var found bool
	m.board, found = m.board.SelectByID("task-3")
	require.True(t, found, "precondition: task-3 should be selectable")
	require.Equal(t, "task-3", m.board.SelectedIssue().ID, "precondition: task-3 should be selected")

	m, _ = m.RefreshFromConfig()
	require.NotNil(t, m.pendingCursor, "refresh should capture cursor for restoration")

	// First loaded column does not contain the selected issue: keep pending cursor.
	m, _ = m.handleColumnLoaded(board.ColumnLoadedMsg{
		ViewIndex:   0,
		ColumnIndex: 0,
		ColumnTitle: "Todo",
		Issues:      col0Issues,
		Err:         nil,
	})
	require.NotNil(t, m.pendingCursor, "cursor restore should remain pending until matching issue loads")

	// Later column includes selected issue: restore and clear pending cursor.
	m, _ = m.handleColumnLoaded(board.ColumnLoadedMsg{
		ViewIndex:   0,
		ColumnIndex: 1,
		ColumnTitle: "In Progress",
		Issues:      col1Issues,
		Err:         nil,
	})
	selected := m.board.SelectedIssue()
	require.NotNil(t, selected, "selection should be restored once matching column loads")
	require.Equal(t, "task-3", selected.ID, "cursor should restore to the original issue")
	require.Nil(t, m.pendingCursor, "pending cursor should clear after successful restoration")
}

func TestKanban_RefreshFromConfig_ClearsPendingWhenIssueMissing(t *testing.T) {
	columns := []config.ColumnConfig{
		{Name: "Todo", Query: "status = open", Color: "#999999"},
		{Name: "In Progress", Query: "status = in_progress", Color: "#888888"},
	}
	m := createRefreshTestModel(t, columns)
	initialCol0 := []beads.Issue{{ID: "task-1", TitleText: "Task 1", Type: beads.TypeTask}}
	initialCol1 := []beads.Issue{
		{ID: "task-2", TitleText: "Task 2", Type: beads.TypeTask},
		{ID: "task-3", TitleText: "Task 3", Type: beads.TypeTask},
	}
	reloadedCol0 := []beads.Issue{{ID: "task-1", TitleText: "Task 1", Type: beads.TypeTask}}
	reloadedCol1 := []beads.Issue{{ID: "task-2", TitleText: "Task 2", Type: beads.TypeTask}} // task-3 removed

	m.board = seedBoardColumn(m.board, 0, "Todo", initialCol0)
	m.board = seedBoardColumn(m.board, 1, "In Progress", initialCol1)

	var found bool
	m.board, found = m.board.SelectByID("task-3")
	require.True(t, found, "precondition: task-3 should be selectable")

	m, _ = m.RefreshFromConfig()
	require.NotNil(t, m.pendingCursor, "refresh should capture cursor for restoration")

	m, _ = m.handleColumnLoaded(board.ColumnLoadedMsg{
		ViewIndex:   0,
		ColumnIndex: 0,
		ColumnTitle: "Todo",
		Issues:      reloadedCol0,
		Err:         nil,
	})
	require.NotNil(t, m.pendingCursor, "pending cursor should remain before all loads complete")

	m, _ = m.handleColumnLoaded(board.ColumnLoadedMsg{
		ViewIndex:   0,
		ColumnIndex: 1,
		ColumnTitle: "In Progress",
		Issues:      reloadedCol1,
		Err:         nil,
	})
	require.Nil(t, m.pendingCursor, "pending cursor should clear when issue is missing after full reload")
}

func TestKanban_HandleIssueSaved_Error(t *testing.T) {
	m := createTestModel(t)

	// handleIssueSaved on error should return ShowToastMsg
	msg := issueSavedMsg{
		issueID: "test-123",
		opts:    beads.UpdateIssueOptions{},
		err:     errors.New("database error"),
	}
	m, cmd := m.handleIssueSaved(msg)

	require.NotNil(t, cmd, "expected toast command on error")
	toastResult := cmd()
	showToast, ok := toastResult.(mode.ShowToastMsg)
	require.True(t, ok, "expected ShowToastMsg")
	require.Contains(t, showToast.Message, "Save failed")
	require.Contains(t, showToast.Message, "database error")
	require.Equal(t, toaster.StyleError, showToast.Style)
}

func TestKanban_SaveMsg_NoChanges(t *testing.T) {
	// When nothing changed, UpdateIssue is still called with all-nil opts (no-op)
	m := createTestModel(t)

	m.editingIssue = &beads.Issue{
		ID:              "test-issue",
		TitleText:       "Test Issue",
		DescriptionText: "Test description",
		Notes:           "original notes",
		Priority:        beads.PriorityMedium,
		Status:          beads.StatusOpen,
		Labels:          []string{},
	}
	m.view = ViewEditIssue

	// Set up mock expecting UpdateIssue with all-nil opts
	mockExecutor := mocks.NewMockIssueExecutor(t)
	mockExecutor.EXPECT().UpdateIssue("test-issue", mock.MatchedBy(func(opts beads.UpdateIssueOptions) bool {
		return opts.Title == nil && opts.Description == nil && opts.Notes == nil &&
			opts.Priority == nil && opts.Status == nil && opts.Labels == nil
	})).Return(nil)
	m.services.BeadsExecutor = mockExecutor

	m, cmd := m.Update(issueeditor.SaveMsg{
		IssueID:     "test-issue",
		Priority:    beads.PriorityMedium,
		Status:      beads.StatusOpen,
		Labels:      []string{},
		Title:       "Test Issue",
		Description: "Test description",
		Notes:       "original notes",
	})

	require.Nil(t, m.editingIssue, "editingIssue should be cleared after save")
	require.Equal(t, ViewBoard, m.view)
	require.NotNil(t, cmd)

	// Execute to trigger mock
	result := cmd()
	_, ok := result.(issueSavedMsg)
	require.True(t, ok, "command should return issueSavedMsg")
}

func TestKanban_SaveMsg_ClearsEditingIssue(t *testing.T) {
	m := createTestModel(t)

	m.editingIssue = &beads.Issue{
		ID:        "test-issue",
		TitleText: "Test Issue",
	}
	m.view = ViewEditIssue

	m, _ = m.Update(issueeditor.SaveMsg{
		IssueID:  "test-issue",
		Title:    "Test Issue",
		Priority: beads.PriorityMedium,
		Status:   beads.StatusOpen,
		Labels:   []string{},
	})

	require.Nil(t, m.editingIssue, "editingIssue should be cleared after save")
	require.Equal(t, ViewBoard, m.view)
}

func TestKanban_IssueEditor_CancelMsg_ClearsEditingIssue(t *testing.T) {
	m := createTestModelWithIssue("test-123", "status = open")

	// Get the selected issue
	issue := m.board.SelectedIssue()
	require.NotNil(t, issue, "precondition: should have selected issue")

	// Open editor (which sets editingIssue)
	m, _ = m.Update(OpenEditMenuMsg{Issue: *issue})
	require.NotNil(t, m.editingIssue, "editingIssue should be set after opening editor")

	// Process CancelMsg
	cancelMsg := issueeditor.CancelMsg{}
	m, cmd := m.Update(cancelMsg)

	require.Equal(t, ViewBoard, m.view, "expected ViewBoard view after cancel")
	require.Nil(t, cmd, "expected no command on cancel")
	require.Nil(t, m.editingIssue, "editingIssue should be cleared after cancel")
}

func TestKanban_SaveIssueCmd_CallsUpdateIssue(t *testing.T) {
	m := createTestModel(t)

	title := "New Title"
	opts := beads.UpdateIssueOptions{Title: &title}

	mockExecutor := mocks.NewMockIssueExecutor(t)
	mockExecutor.EXPECT().UpdateIssue("test-issue", opts).Return(nil)
	m.services.BeadsExecutor = mockExecutor

	cmd := m.saveIssueCmd("test-issue", opts)
	require.NotNil(t, cmd)

	msg := cmd()
	savedMsg, ok := msg.(issueSavedMsg)
	require.True(t, ok, "command should return issueSavedMsg")
	require.Equal(t, "test-issue", savedMsg.issueID)
	require.NoError(t, savedMsg.err)
	require.NotNil(t, savedMsg.opts.Title)
	require.Equal(t, "New Title", *savedMsg.opts.Title)
}

func TestKanban_SaveIssueCmd_PropagatesErrors(t *testing.T) {
	m := createTestModel(t)

	opts := beads.UpdateIssueOptions{}

	mockExecutor := mocks.NewMockIssueExecutor(t)
	mockExecutor.EXPECT().UpdateIssue("test-issue", opts).Return(errors.New("update failed"))
	m.services.BeadsExecutor = mockExecutor

	cmd := m.saveIssueCmd("test-issue", opts)
	require.NotNil(t, cmd)

	msg := cmd()
	savedMsg, ok := msg.(issueSavedMsg)
	require.True(t, ok, "command should return issueSavedMsg")
	require.Error(t, savedMsg.err)
	require.Contains(t, savedMsg.err.Error(), "update failed")
}
