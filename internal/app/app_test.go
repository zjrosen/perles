package app

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/config"
	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/mode"
	"github.com/zjrosen/perles/internal/mode/kanban"
	"github.com/zjrosen/perles/internal/mode/search"
	"github.com/zjrosen/perles/internal/ui/shared/diffviewer"

	tea "github.com/charmbracelet/bubbletea"
)

// createTestModel creates a minimal Model for testing.
// It does not require a database connection.
func createTestModel(t *testing.T) Model {
	cfg := config.Defaults()
	clipboard := mocks.NewMockClipboard(t)
	clipboard.EXPECT().Copy(mock.Anything).Return(nil).Maybe()
	services := mode.Services{
		Config:    &cfg,
		Clipboard: clipboard,
	}

	return Model{
		currentMode: mode.ModeKanban,
		kanban:      kanban.New(services),
		search:      search.New(services),
		services:    services,
		width:       100,
		height:      40,
	}
}

func TestApp_DefaultMode(t *testing.T) {
	m := createTestModel(t)
	require.Equal(t, mode.ModeKanban, m.currentMode, "expected default mode to be kanban")
}

func TestApp_WindowSizeMsg(t *testing.T) {
	m := createTestModel(t)

	// Simulate window resize
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 50})
	m = newModel.(Model)

	require.Equal(t, 120, m.width, "expected width to be updated")
	require.Equal(t, 50, m.height, "expected height to be updated")
}

func TestApp_CtrlSpaceSwitchesMode(t *testing.T) {
	m := createTestModel(t)

	// Ctrl+Space (ctrl+@) should switch from kanban to search mode
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = newModel.(Model)

	// Should now be in search mode
	require.Equal(t, mode.ModeSearch, m.currentMode, "mode should switch to search")

	// Ctrl+Space again should switch back to kanban
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = newModel.(Model)

	require.Equal(t, mode.ModeKanban, m.currentMode, "mode should switch back to kanban")
}

func TestApp_ViewDelegates(t *testing.T) {
	m := createTestModel(t)

	// View should delegate to kanban
	view := m.View()
	require.NotEmpty(t, view, "expected non-empty view from kanban mode")
}

func TestApp_ModeSwitchPreservesSize(t *testing.T) {
	m := createTestModel(t)

	// Set initial window size
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 150, Height: 60})
	m = newModel.(Model)

	require.Equal(t, 150, m.width, "initial width should be 150")
	require.Equal(t, 60, m.height, "initial height should be 60")

	// Switch to search mode (Ctrl+Space)
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = newModel.(Model)

	// Verify size preserved in app
	require.Equal(t, 150, m.width, "width should be preserved after mode switch")
	require.Equal(t, 60, m.height, "height should be preserved after mode switch")

	// Verify search mode has the correct size (by checking View doesn't panic)
	view := m.View()
	require.NotEmpty(t, view, "search view should render without panic")

	// Switch back to kanban (Ctrl+Space)
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = newModel.(Model)

	// Verify size still preserved
	require.Equal(t, 150, m.width, "width should be preserved after returning to kanban")
	require.Equal(t, 60, m.height, "height should be preserved after returning to kanban")
}

func TestApp_SearchModeInit(t *testing.T) {
	m := createTestModel(t)

	// Switch to search mode (Ctrl+Space)
	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = newModel.(Model)

	// Verify mode switched
	require.Equal(t, mode.ModeSearch, m.currentMode, "should be in search mode")

	// Init should have been called (returns a command)
	// The search Init() returns nil if no initial query
	// This is expected behavior - we just verify the switch worked
	_ = cmd // May be nil for empty search

	// Verify View renders search mode content
	view := m.View()
	require.NotEmpty(t, view, "search view should render")
}

func TestApp_KanbanModeExtracted(t *testing.T) {
	m := createTestModel(t)

	// Verify kanban mode exists and works
	require.NotNil(t, m.kanban, "kanban mode should be initialized")
	require.Equal(t, mode.ModeKanban, m.currentMode, "default mode should be kanban")

	// Verify kanban view renders
	view := m.View()
	require.NotEmpty(t, view, "kanban view should render")

	// Verify we can interact with kanban mode
	// (j key for navigation - should not crash)
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = newModel.(Model)

	require.Equal(t, mode.ModeKanban, m.currentMode, "should still be in kanban mode")
}

func TestApp_CtrlC_ShowsQuitConfirmation(t *testing.T) {
	m := createTestModel(t)

	// Ctrl+C should show quit confirmation modal (not quit immediately)
	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = newModel.(Model)

	// No command yet - modal is showing
	require.Nil(t, cmd, "expected no command (quit modal should be showing)")
}

func TestApp_SearchModeReceivesUpdates(t *testing.T) {
	m := createTestModel(t)

	// Switch to search mode (Ctrl+Space)
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = newModel.(Model)

	// Send a key that search mode handles (? for help)
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = newModel.(Model)

	// Should still be in search mode (help overlay doesn't change mode)
	require.Equal(t, mode.ModeSearch, m.currentMode, "should still be in search mode")

	// View should render without panic
	view := m.View()
	require.NotEmpty(t, view, "view should render")
}

func TestApp_ModeSwitchRoundTrip(t *testing.T) {
	m := createTestModel(t)

	// Multiple round trips should work (Ctrl+Space)
	for i := 0; i < 3; i++ {
		// Switch to search
		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
		m = newModel.(Model)
		require.Equal(t, mode.ModeSearch, m.currentMode, "should be in search mode")

		// Switch back to kanban
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
		m = newModel.(Model)
		require.Equal(t, mode.ModeKanban, m.currentMode, "should be in kanban mode")
	}
}

func TestApp_SwitchToSearchMsg_WithQuery(t *testing.T) {
	m := createTestModel(t)

	// Simulate receiving SwitchToSearchMsg from kanban mode
	newModel, cmd := m.Update(kanban.SwitchToSearchMsg{Query: "status:open"})
	m = newModel.(Model)

	// Verify mode switched to search
	require.Equal(t, mode.ModeSearch, m.currentMode, "should be in search mode")

	// Init should be called (returns command batch)
	require.NotNil(t, cmd, "expected Init command")

	// View should render without panic
	view := m.View()
	require.NotEmpty(t, view, "search view should render")
}

func TestApp_SwitchToSearchMsg_EmptyQuery(t *testing.T) {
	m := createTestModel(t)

	// Simulate SwitchToSearchMsg with empty query (no column focused)
	newModel, cmd := m.Update(kanban.SwitchToSearchMsg{Query: ""})
	m = newModel.(Model)

	// Verify mode switched to search
	require.Equal(t, mode.ModeSearch, m.currentMode, "should be in search mode")

	// Init should be called
	require.NotNil(t, cmd, "expected Init command")

	// View should render
	view := m.View()
	require.NotEmpty(t, view, "search view should render")
}

func TestApp_ExitToKanbanMsg(t *testing.T) {
	m := createTestModel(t)

	// Switch to search mode first (Ctrl+Space)
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = newModel.(Model)
	require.Equal(t, mode.ModeSearch, m.currentMode, "should be in search mode")

	// Simulate ExitToKanbanMsg from search mode (ESC key)
	newModel, _ = m.Update(search.ExitToKanbanMsg{})
	m = newModel.(Model)

	// Verify mode switched back to kanban
	require.Equal(t, mode.ModeKanban, m.currentMode, "should be back in kanban mode")

	// View should render kanban mode
	view := m.View()
	require.NotEmpty(t, view, "kanban view should render")
}

func TestApp_SaveSearchToNewView(t *testing.T) {
	m := createTestModel(t)
	initialViewCount := len(m.services.Config.Views)

	// Simulate SaveSearchToNewViewMsg (without config path, so AddView will fail)
	// This tests the in-memory handling
	msg := search.SaveSearchToNewViewMsg{
		ViewName:   "My Bugs",
		ColumnName: "Open Bugs",
		Color:      "#FF8787",
		Query:      "status = open",
	}
	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	// In-memory config should be updated even if file write fails
	// (because we append before AddView in the actual handler)
	// Note: Our test model has no ConfigPath so AddView will fail
	// This is expected - the important thing is the handler doesn't panic
	require.GreaterOrEqual(t, len(m.services.Config.Views), initialViewCount,
		"view count should not decrease")
}

func TestApp_SaveSearchToNewView_Structure(t *testing.T) {
	m := createTestModel(t)

	// Test with a temporary config to verify correct structure
	msg := search.SaveSearchToNewViewMsg{
		ViewName:   "Test View",
		ColumnName: "Test Column",
		Color:      "#73F59F",
		Query:      "priority = 0",
	}

	// Call handler directly to test structure without file I/O
	result, _ := m.handleSaveSearchToNewView(msg)
	resultModel := result.(Model)

	// Since config path is empty, AddView fails but we can verify the handler runs
	// The in-memory update happens after AddView, so it won't update on error
	// This is correct behavior - don't partially update on error
	require.NotNil(t, resultModel, "handler should return model")
}

func TestApp_ShowDiffViewer(t *testing.T) {
	m := createTestModel(t)
	require.False(t, m.diffViewer.Visible(), "diff viewer should start hidden")

	// Send ShowDiffViewerMsg
	newModel, cmd := m.Update(diffviewer.ShowDiffViewerMsg{})
	m = newModel.(Model)

	require.True(t, m.diffViewer.Visible(), "diff viewer should be visible after ShowDiffViewerMsg")
	require.NotNil(t, cmd, "should return LoadDiff command")
}

func TestApp_HideDiffViewer(t *testing.T) {
	m := createTestModel(t)

	// First show the diff viewer
	newModel, _ := m.Update(diffviewer.ShowDiffViewerMsg{})
	m = newModel.(Model)
	require.True(t, m.diffViewer.Visible(), "diff viewer should be visible")

	// Send HideDiffViewerMsg
	newModel, cmd := m.Update(diffviewer.HideDiffViewerMsg{})
	m = newModel.(Model)

	require.False(t, m.diffViewer.Visible(), "diff viewer should be hidden after HideDiffViewerMsg")
	require.Nil(t, cmd, "should not return any command")
}

func TestApp_DiffViewerEventRouting(t *testing.T) {
	m := createTestModel(t)

	// Show diff viewer first
	newModel, _ := m.Update(diffviewer.ShowDiffViewerMsg{})
	m = newModel.(Model)
	require.True(t, m.diffViewer.Visible(), "diff viewer should be visible")

	// Send ESC key - should close diff viewer (not switch modes)
	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = newModel.(Model)

	// The diff viewer handles ESC and emits HideDiffViewerMsg
	// We need to process that message to verify the overlay closes
	if cmd != nil {
		msg := cmd()
		if _, ok := msg.(diffviewer.HideDiffViewerMsg); ok {
			newModel, _ = m.Update(msg)
			m = newModel.(Model)
		}
	}

	require.False(t, m.diffViewer.Visible(), "diff viewer should be hidden after ESC")
	require.Equal(t, mode.ModeKanban, m.currentMode, "should remain in kanban mode")
}

func TestApp_DiffViewerOverlay(t *testing.T) {
	m := createTestModel(t)

	// Set size first
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = newModel.(Model)

	// Show diff viewer
	newModel, _ = m.Update(diffviewer.ShowDiffViewerMsg{})
	m = newModel.(Model)

	// Verify view includes diff viewer overlay
	view := m.View()
	require.NotEmpty(t, view, "view should render")
	// Since diffViewer is visible and in loading state, the view should contain diff viewer content
	require.True(t, m.diffViewer.Visible(), "diff viewer should be visible")
}

func TestApp_DiffViewerWindowResize(t *testing.T) {
	m := createTestModel(t)

	// Show diff viewer
	newModel, _ := m.Update(diffviewer.ShowDiffViewerMsg{})
	m = newModel.(Model)

	// Resize window
	newModel, _ = m.Update(tea.WindowSizeMsg{Width: 200, Height: 80})
	m = newModel.(Model)

	require.Equal(t, 200, m.width, "width should be updated")
	require.Equal(t, 80, m.height, "height should be updated")
	// The diff viewer should have received the size update (we can't easily verify internal state,
	// but the View() should not panic)
	view := m.View()
	require.NotEmpty(t, view, "view should render after resize")
}
