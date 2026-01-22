package kanban

import (
	"runtime"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	beads "github.com/zjrosen/perles/internal/beads/domain"
	"github.com/zjrosen/perles/internal/config"
	"github.com/zjrosen/perles/internal/flags"
	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/mode"
	"github.com/zjrosen/perles/internal/mode/shared"
	"github.com/zjrosen/perles/internal/orchestration/session"
	"github.com/zjrosen/perles/internal/ui/board"
	"github.com/zjrosen/perles/internal/ui/commandpalette"
	"github.com/zjrosen/perles/internal/ui/modals/issueeditor"
	"github.com/zjrosen/perles/internal/ui/shared/diffviewer"
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

// =============================================================================
// Session Picker Integration Tests (Phase 5c)
// =============================================================================

func TestResumeKeybinding_OpensPicker(t *testing.T) {
	// Create model with session storage config pointing to temp directory with sessions
	tempDir := t.TempDir()
	now := time.Now()

	// Create test session
	pathBuilder := session.NewSessionPathBuilder(tempDir, "testapp")
	createResumableTestSession(t, pathBuilder, "test-session-1", now.Add(-1*time.Hour), 2)

	clock := mocks.NewMockClock(t)
	clock.EXPECT().Now().Return(now).Maybe()

	cfg := config.Defaults()
	cfg.Orchestration.SessionStorage = config.SessionStorageConfig{
		BaseDir:         tempDir,
		ApplicationName: "testapp",
	}

	services := createTestServices(t, &cfg, clock, nil)

	m := Model{
		services: services,
		width:    100,
		height:   40,
		view:     ViewBoard,
	}

	// Call openSessionPicker directly since that's what the keybinding handler does
	// (Testing the keybinding mapping itself would require Bubble Tea integration tests)
	m, cmd := m.openSessionPicker()

	// Should show picker with sessions
	require.True(t, m.showSessionPicker, "expected showSessionPicker to be true")
	require.NotNil(t, m.sessionPicker, "expected sessionPicker to not be nil")
	require.Nil(t, cmd, "expected nil command when sessions exist")
}

func TestPickerForwarding_WhenVisible(t *testing.T) {
	// Create model with picker visible
	pickerModel := commandpalette.New(commandpalette.Config{
		Title: "Resume Session",
		Items: []commandpalette.Item{
			{ID: "/path/to/session1", Name: "Session 1", Description: "1 hour ago"},
			{ID: "/path/to/session2", Name: "Session 2", Description: "2 hours ago"},
		},
	})
	pickerModel = pickerModel.SetSize(100, 40)

	cfg := config.Defaults()
	services := mode.Services{
		Config: &cfg,
	}

	m := Model{
		services:          services,
		width:             100,
		height:            40,
		view:              ViewBoard,
		sessionPicker:     &pickerModel,
		showSessionPicker: true,
	}

	// Initial cursor should be at 0
	require.Equal(t, 0, m.sessionPicker.Cursor(), "precondition: cursor should be at 0")

	// Simulate Down arrow keypress
	msg := tea.KeyMsg{Type: tea.KeyDown}
	m, _ = m.handleBoardKey(msg)

	// Event should be forwarded to picker - cursor should move
	require.NotNil(t, m.sessionPicker, "picker should still exist")
	require.Equal(t, 1, m.sessionPicker.Cursor(), "expected cursor to move to 1 after down key")
}

func TestSessionPickerSelection_TriggersModeSwitch(t *testing.T) {
	// Create model with picker visible
	pickerModel := commandpalette.New(commandpalette.Config{
		Title: "Resume Session",
		Items: []commandpalette.Item{
			{ID: "/path/to/session1", Name: "Session 1", Description: "1 hour ago"},
		},
	})
	pickerModel = pickerModel.SetSize(100, 40)

	cfg := config.Defaults()
	services := mode.Services{
		Config: &cfg,
	}

	m := Model{
		services:          services,
		width:             100,
		height:            40,
		view:              ViewBoard,
		sessionPicker:     &pickerModel,
		showSessionPicker: true,
	}

	// Handle selection message
	selectMsg := commandpalette.SelectMsg{
		Item: commandpalette.Item{
			ID:          "/path/to/session1",
			Name:        "Session 1",
			Description: "1 hour ago",
		},
	}
	m, cmd := m.Update(selectMsg)

	// Should hide picker
	require.False(t, m.showSessionPicker, "expected showSessionPicker to be false")
	require.Nil(t, m.sessionPicker, "expected sessionPicker to be nil")

	// Should return command that produces SwitchToOrchestrationMsg with ResumeSessionDir
	require.NotNil(t, cmd, "expected command from selection")
	result := cmd()
	switchMsg, ok := result.(SwitchToOrchestrationMsg)
	require.True(t, ok, "expected SwitchToOrchestrationMsg, got %T", result)
	require.Equal(t, "/path/to/session1", switchMsg.ResumeSessionDir, "expected ResumeSessionDir to match selected item ID")
}

func TestSessionPickerCancel_HidesPicker(t *testing.T) {
	// Create model with picker visible
	pickerModel := commandpalette.New(commandpalette.Config{
		Title: "Resume Session",
		Items: []commandpalette.Item{
			{ID: "/path/to/session1", Name: "Session 1", Description: "1 hour ago"},
		},
	})
	pickerModel = pickerModel.SetSize(100, 40)

	cfg := config.Defaults()
	services := mode.Services{
		Config: &cfg,
	}

	m := Model{
		services:          services,
		width:             100,
		height:            40,
		view:              ViewBoard,
		sessionPicker:     &pickerModel,
		showSessionPicker: true,
	}

	// Handle cancel message
	cancelMsg := commandpalette.CancelMsg{}
	m, cmd := m.Update(cancelMsg)

	// Should hide picker
	require.False(t, m.showSessionPicker, "expected showSessionPicker to be false")
	require.Nil(t, m.sessionPicker, "expected sessionPicker to be nil")

	// Should return nil command (stay in Kanban)
	require.Nil(t, cmd, "expected nil command on cancel")
	require.Equal(t, ViewBoard, m.view, "expected to stay in ViewBoard")
}

func TestPickerOverlay_RenderedWhenVisible(t *testing.T) {
	// Create model with picker visible
	pickerModel := commandpalette.New(commandpalette.Config{
		Title: "Resume Session",
		Items: []commandpalette.Item{
			{ID: "/path/to/session1", Name: "Session 1", Description: "1 hour ago"},
		},
	})
	pickerModel = pickerModel.SetSize(100, 40)

	cfg := config.Defaults()
	services := mode.Services{
		Config: &cfg,
	}

	m := Model{
		services:          services,
		width:             100,
		height:            40,
		view:              ViewBoard,
		sessionPicker:     &pickerModel,
		showSessionPicker: true,
	}

	// Render the view
	view := m.View()

	// View should contain session picker content
	require.Contains(t, view, "Resume Session", "expected view to contain picker title")
	require.Contains(t, view, "Session 1", "expected view to contain session name")
}

func TestPickerOverlay_NotRenderedWhenHidden(t *testing.T) {
	// Create model without picker visible
	cfg := config.Defaults()
	services := mode.Services{
		Config: &cfg,
	}

	m := Model{
		services:          services,
		width:             100,
		height:            40,
		view:              ViewBoard,
		showSessionPicker: false,
		sessionPicker:     nil,
	}

	// Render the view
	view := m.View()

	// View should NOT contain session picker content
	require.NotContains(t, view, "Resume Session", "expected view to NOT contain picker title")
}

// TestHandleBoardKey_Dashboard_FlagDisabled verifies ctrl+t does nothing when flag is disabled.
func TestHandleBoardKey_Dashboard_FlagDisabled(t *testing.T) {
	tests := []struct {
		name  string
		flags *flags.Registry
	}{
		{
			name:  "nil flags registry returns nil",
			flags: nil,
		},
		{
			name:  "flag explicitly false returns nil",
			flags: flags.New(map[string]bool{flags.FlagControlPlane: false}),
		},
		{
			name:  "unrelated flags only returns nil",
			flags: flags.New(map[string]bool{"other-flag": true}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Defaults()
			clipboard := mocks.NewMockClipboard(t)
			clipboard.EXPECT().Copy(mock.Anything).Return(nil).Maybe()
			mockExecutor := mocks.NewMockBQLExecutor(t)

			services := mode.Services{
				Config:    &cfg,
				Clipboard: clipboard,
				Executor:  mockExecutor,
				Flags:     tt.flags,
			}

			m := Model{
				services: services,
				width:    100,
				height:   40,
				view:     ViewBoard,
			}

			// Simulate ctrl+t key press
			msg := tea.KeyMsg{Type: tea.KeyCtrlT}
			_, cmd := m.handleBoardKey(msg)

			// Should return nil (no-op) when flag is disabled
			require.Nil(t, cmd, "expected nil command when flag disabled")
		})
	}
}

// TestHandleBoardKey_Dashboard_FlagEnabled verifies ctrl+t switches to dashboard when flag is enabled.
func TestHandleBoardKey_Dashboard_FlagEnabled(t *testing.T) {
	cfg := config.Defaults()
	clipboard := mocks.NewMockClipboard(t)
	clipboard.EXPECT().Copy(mock.Anything).Return(nil).Maybe()
	mockExecutor := mocks.NewMockBQLExecutor(t)

	// Enable the control-plane flag
	flagRegistry := flags.New(map[string]bool{flags.FlagControlPlane: true})

	services := mode.Services{
		Config:    &cfg,
		Clipboard: clipboard,
		Executor:  mockExecutor,
		Flags:     flagRegistry,
	}

	m := Model{
		services: services,
		width:    100,
		height:   40,
		view:     ViewBoard,
	}

	// Simulate ctrl+t key press
	msg := tea.KeyMsg{Type: tea.KeyCtrlT}
	_, cmd := m.handleBoardKey(msg)

	// With flag enabled, should return SwitchToDashboardMsg
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
	for retries := 0; retries < 3; retries++ {
		z = zone.Get(zoneID)
		if z != nil && !z.IsZero() {
			break
		}
		_ = m.View()
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
	for retries := 0; retries < 3; retries++ {
		z = zone.Get(zoneID)
		if z != nil && !z.IsZero() {
			break
		}
		_ = mClick.View()
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
