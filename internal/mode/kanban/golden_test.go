package kanban

import (
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/mock"

	"github.com/zjrosen/perles/internal/beads"
	"github.com/zjrosen/perles/internal/config"
	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/mode"
	"github.com/zjrosen/perles/internal/mode/shared"
	"github.com/zjrosen/perles/internal/ui/board"
)

// testNow is a fixed reference time for golden tests to ensure reproducible timestamps.
var testNow = time.Date(2025, 12, 13, 12, 0, 0, 0, time.UTC)

// createGoldenTestModel creates a Model with a mock clock for reproducible golden tests.
func createGoldenTestModel(t *testing.T) Model {
	cfg := config.Defaults()
	clipboard := mocks.NewMockClipboard(t)
	clipboard.EXPECT().Copy(mock.Anything).Return(nil).Maybe()
	clock := mocks.NewMockClock(t)
	clock.EXPECT().Now().Return(testNow).Maybe()

	mockExecutor := mocks.NewMockBQLExecutor(t)

	services := mode.Services{
		Config:    &cfg,
		Clipboard: clipboard,
		Clock:     clock,
		Executor:  mockExecutor,
	}

	m := Model{
		services:            services,
		view:                ViewBoard,
		width:               100,
		height:              40,
		pendingDeleteColumn: -1,
	}

	return m
}

// createGoldenTestModelWithBoard creates a Model with a populated board for golden tests.
func createGoldenTestModelWithBoard(t *testing.T) (Model, *mocks.MockBQLExecutor) {
	cfg := config.Defaults()
	clipboard := mocks.NewMockClipboard(t)
	clipboard.EXPECT().Copy(mock.Anything).Return(nil).Maybe()
	clock := mocks.NewMockClock(t)
	clock.EXPECT().Now().Return(testNow).Maybe()

	mockExecutor := mocks.NewMockBQLExecutor(t)

	services := mode.Services{
		Config:    &cfg,
		Clipboard: clipboard,
		Clock:     clock,
		Executor:  mockExecutor,
	}

	// Create board with a column containing issues
	boardConfigs := []config.ColumnConfig{
		{Name: "Open", Query: "status = open", Color: "#888888"},
	}
	brd := board.NewFromViews([]config.ViewConfig{{Name: "Test", Columns: boardConfigs}}, nil, clock).SetSize(100, 38)

	// Populate with test issues
	brd, _ = brd.Update(board.ColumnLoadedMsg{
		ViewIndex:   0,
		ColumnTitle: "Open",
		Issues: []beads.Issue{
			{ID: "task-123", TitleText: "Regular task to delete", Type: beads.TypeTask, Priority: 2, Status: beads.StatusOpen},
		},
		Err: nil,
	})

	m := Model{
		services:            services,
		board:               brd,
		view:                ViewBoard,
		width:               100,
		height:              40,
		pendingDeleteColumn: -1,
	}

	return m, mockExecutor
}

// =============================================================================
// Golden Tests: Delete Modal Views
// =============================================================================

func TestKanban_Golden_DeleteModal_RegularIssue(t *testing.T) {
	m := createGoldenTestModel(t)
	m = m.SetSize(100, 30)

	// Create a regular task issue
	issue := &beads.Issue{
		ID:        "task-456",
		TitleText: "Bug fix: handle null pointer",
		Type:      beads.TypeTask,
		Priority:  1,
		Status:    beads.StatusOpen,
	}

	// Create delete modal for regular issue (no executor needed for non-epic)
	mockExecutor := mocks.NewMockBQLExecutor(t)
	m.modal, m.deleteIssueIDs = shared.CreateDeleteModal(issue, mockExecutor)
	m.modal.SetSize(m.width, m.height)
	m.selectedIssue = issue
	m.view = ViewDeleteIssue

	// Create a board background for the overlay
	cfg := config.Defaults()
	clock := mocks.NewMockClock(t)
	clock.EXPECT().Now().Return(testNow).Maybe()
	boardConfigs := []config.ColumnConfig{
		{Name: "Open", Query: "status = open", Color: "#888888"},
		{Name: "In Progress", Query: "status = in_progress", Color: "#4488FF"},
	}
	m.board = board.NewFromViews([]config.ViewConfig{{Name: "Test", Columns: boardConfigs}}, nil, clock).SetSize(100, 28)
	m.services.Config = &cfg

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestKanban_Golden_DeleteModal_EpicWithDescendants(t *testing.T) {
	m := createGoldenTestModel(t)
	m = m.SetSize(100, 40)

	// Create an epic with children
	issue := &beads.Issue{
		ID:        "epic-1",
		TitleText: "Major refactoring project",
		Type:      beads.TypeEpic,
		Priority:  1,
		Status:    beads.StatusInProgress,
		Children:  []string{"task-1", "task-2", "task-3"},
	}

	// Mock executor to return descendants when expanding epic
	mockExecutor := mocks.NewMockBQLExecutor(t)
	mockExecutor.EXPECT().Execute(mock.Anything).Return([]beads.Issue{
		{ID: "epic-1", Type: beads.TypeEpic, TitleText: "Major refactoring project", Priority: 1},
		{ID: "task-1", Type: beads.TypeTask, TitleText: "Refactor database layer", Priority: 2},
		{ID: "task-2", Type: beads.TypeBug, TitleText: "Fix migration script", Priority: 0},
		{ID: "task-3", Type: beads.TypeFeature, TitleText: "Add new API endpoint", Priority: 1},
	}, nil)

	// Create delete modal for epic (shows descendants)
	m.modal, m.deleteIssueIDs = shared.CreateDeleteModal(issue, mockExecutor)
	m.modal.SetSize(m.width, m.height)
	m.selectedIssue = issue
	m.view = ViewDeleteIssue

	// Create a board background for the overlay
	cfg := config.Defaults()
	clock := mocks.NewMockClock(t)
	clock.EXPECT().Now().Return(testNow).Maybe()
	boardConfigs := []config.ColumnConfig{
		{Name: "Backlog", Query: "status = open", Color: "#888888"},
		{Name: "Active", Query: "status = in_progress", Color: "#44FF88"},
	}
	m.board = board.NewFromViews([]config.ViewConfig{{Name: "Dev", Columns: boardConfigs}}, nil, clock).SetSize(100, 38)
	m.services.Config = &cfg

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}
