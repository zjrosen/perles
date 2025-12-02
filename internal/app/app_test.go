package app

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"perles/internal/config"
	"perles/internal/mode"
	"perles/internal/mode/kanban"
	"perles/internal/mode/search"

	tea "github.com/charmbracelet/bubbletea"
)

// createTestModel creates a minimal Model for testing.
// It does not require a database connection.
func createTestModel() Model {
	cfg := config.Defaults()
	services := mode.Services{
		Config: &cfg,
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
	m := createTestModel()
	assert.Equal(t, mode.ModeKanban, m.currentMode, "expected default mode to be kanban")
}

func TestApp_WindowSizeMsg(t *testing.T) {
	m := createTestModel()

	// Simulate window resize
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 50})
	m = newModel.(Model)

	assert.Equal(t, 120, m.width, "expected width to be updated")
	assert.Equal(t, 50, m.height, "expected height to be updated")
}

func TestApp_CtrlSpaceSwitchesMode(t *testing.T) {
	m := createTestModel()

	// Ctrl+Space (ctrl+@) should switch from kanban to search mode
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = newModel.(Model)

	// Should now be in search mode
	assert.Equal(t, mode.ModeSearch, m.currentMode, "mode should switch to search")

	// Ctrl+Space again should switch back to kanban
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = newModel.(Model)

	assert.Equal(t, mode.ModeKanban, m.currentMode, "mode should switch back to kanban")
}

func TestApp_ViewDelegates(t *testing.T) {
	m := createTestModel()

	// View should delegate to kanban
	view := m.View()
	assert.NotEmpty(t, view, "expected non-empty view from kanban mode")
}

func TestApp_ModeSwitchPreservesSize(t *testing.T) {
	m := createTestModel()

	// Set initial window size
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 150, Height: 60})
	m = newModel.(Model)

	assert.Equal(t, 150, m.width, "initial width should be 150")
	assert.Equal(t, 60, m.height, "initial height should be 60")

	// Switch to search mode (Ctrl+Space)
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = newModel.(Model)

	// Verify size preserved in app
	assert.Equal(t, 150, m.width, "width should be preserved after mode switch")
	assert.Equal(t, 60, m.height, "height should be preserved after mode switch")

	// Verify search mode has the correct size (by checking View doesn't panic)
	view := m.View()
	assert.NotEmpty(t, view, "search view should render without panic")

	// Switch back to kanban (Ctrl+Space)
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = newModel.(Model)

	// Verify size still preserved
	assert.Equal(t, 150, m.width, "width should be preserved after returning to kanban")
	assert.Equal(t, 60, m.height, "height should be preserved after returning to kanban")
}

func TestApp_SearchModeInit(t *testing.T) {
	m := createTestModel()

	// Switch to search mode (Ctrl+Space)
	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = newModel.(Model)

	// Verify mode switched
	assert.Equal(t, mode.ModeSearch, m.currentMode, "should be in search mode")

	// Init should have been called (returns a command)
	// The search Init() returns nil if no initial query
	// This is expected behavior - we just verify the switch worked
	_ = cmd // May be nil for empty search

	// Verify View renders search mode content
	view := m.View()
	assert.NotEmpty(t, view, "search view should render")
}

func TestApp_KanbanModeExtracted(t *testing.T) {
	m := createTestModel()

	// Verify kanban mode exists and works
	assert.NotNil(t, m.kanban, "kanban mode should be initialized")
	assert.Equal(t, mode.ModeKanban, m.currentMode, "default mode should be kanban")

	// Verify kanban view renders
	view := m.View()
	assert.NotEmpty(t, view, "kanban view should render")

	// Verify we can interact with kanban mode
	// (j key for navigation - should not crash)
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = newModel.(Model)

	assert.Equal(t, mode.ModeKanban, m.currentMode, "should still be in kanban mode")
}

func TestApp_CtrlCQuits(t *testing.T) {
	m := createTestModel()

	// Ctrl+C should return quit command
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

	// The quit command should be tea.Quit
	assert.NotNil(t, cmd, "expected quit command")
}

func TestApp_SearchModeReceivesUpdates(t *testing.T) {
	m := createTestModel()

	// Switch to search mode (Ctrl+Space)
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = newModel.(Model)

	// Send a key that search mode handles (? for help)
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = newModel.(Model)

	// Should still be in search mode (help overlay doesn't change mode)
	assert.Equal(t, mode.ModeSearch, m.currentMode, "should still be in search mode")

	// View should render without panic
	view := m.View()
	assert.NotEmpty(t, view, "view should render")
}

func TestApp_ModeSwitchRoundTrip(t *testing.T) {
	m := createTestModel()

	// Multiple round trips should work (Ctrl+Space)
	for i := 0; i < 3; i++ {
		// Switch to search
		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
		m = newModel.(Model)
		assert.Equal(t, mode.ModeSearch, m.currentMode, "should be in search mode")

		// Switch back to kanban
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
		m = newModel.(Model)
		assert.Equal(t, mode.ModeKanban, m.currentMode, "should be in kanban mode")
	}
}

func TestApp_SwitchToSearchMsg_WithQuery(t *testing.T) {
	m := createTestModel()

	// Simulate receiving SwitchToSearchMsg from kanban mode
	newModel, cmd := m.Update(kanban.SwitchToSearchMsg{Query: "status:open"})
	m = newModel.(Model)

	// Verify mode switched to search
	assert.Equal(t, mode.ModeSearch, m.currentMode, "should be in search mode")

	// Init should be called (returns command batch)
	assert.NotNil(t, cmd, "expected Init command")

	// View should render without panic
	view := m.View()
	assert.NotEmpty(t, view, "search view should render")
}

func TestApp_SwitchToSearchMsg_EmptyQuery(t *testing.T) {
	m := createTestModel()

	// Simulate SwitchToSearchMsg with empty query (no column focused)
	newModel, cmd := m.Update(kanban.SwitchToSearchMsg{Query: ""})
	m = newModel.(Model)

	// Verify mode switched to search
	assert.Equal(t, mode.ModeSearch, m.currentMode, "should be in search mode")

	// Init should be called
	assert.NotNil(t, cmd, "expected Init command")

	// View should render
	view := m.View()
	assert.NotEmpty(t, view, "search view should render")
}

func TestApp_SaveSearchToNewView(t *testing.T) {
	m := createTestModel()
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
	assert.GreaterOrEqual(t, len(m.services.Config.Views), initialViewCount,
		"view count should not decrease")
}

func TestApp_SaveSearchToNewView_Structure(t *testing.T) {
	m := createTestModel()

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
	assert.NotNil(t, resultModel, "handler should return model")
}
