package orchestration

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"

	"perles/internal/orchestration/pool"
)

// TestToggleNavigationMode tests entering and exiting navigation mode.
func TestToggleNavigationMode(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Initially not in navigation mode, input is focused
	require.False(t, m.navigationMode)
	require.True(t, m.input.Focused())

	// Toggle to navigation mode
	m = m.toggleNavigationMode()
	require.True(t, m.navigationMode)
	require.False(t, m.input.Focused())

	// Toggle back to normal mode
	m = m.toggleNavigationMode()
	require.False(t, m.navigationMode)
	require.True(t, m.input.Focused())
}

// TestExitNavigationMode tests exiting navigation mode clears fullscreen.
func TestExitNavigationMode(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m = m.UpdateWorker("worker-1", pool.WorkerWorking)

	// Enter navigation mode and select a pane
	m = m.toggleNavigationMode()
	m = m.toggleFullscreenPane(PaneCoordinator, 0)
	require.Equal(t, PaneCoordinator, m.fullscreenPaneType)

	// Exit navigation mode
	m = m.exitNavigationMode()
	require.False(t, m.navigationMode)
	require.Equal(t, PaneNone, m.fullscreenPaneType)
	require.True(t, m.input.Focused())
}

// TestToggleFullscreenPane_Coordinator tests toggling coordinator pane fullscreen.
func TestToggleFullscreenPane_Coordinator(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Initially no fullscreen
	require.Equal(t, PaneNone, m.fullscreenPaneType)

	// Toggle coordinator fullscreen
	m = m.toggleFullscreenPane(PaneCoordinator, 0)
	require.Equal(t, PaneCoordinator, m.fullscreenPaneType)

	// Toggle again to exit
	m = m.toggleFullscreenPane(PaneCoordinator, 0)
	require.Equal(t, PaneNone, m.fullscreenPaneType)
}

// TestToggleFullscreenPane_Messages tests toggling messages pane fullscreen.
func TestToggleFullscreenPane_Messages(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Toggle messages fullscreen
	m = m.toggleFullscreenPane(PaneMessages, 0)
	require.Equal(t, PaneMessages, m.fullscreenPaneType)

	// Toggle again to exit
	m = m.toggleFullscreenPane(PaneMessages, 0)
	require.Equal(t, PaneNone, m.fullscreenPaneType)
}

// TestToggleFullscreenPane_Worker tests toggling worker pane fullscreen.
func TestToggleFullscreenPane_Worker(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Add workers
	m = m.UpdateWorker("worker-1", pool.WorkerWorking)
	m = m.UpdateWorker("worker-2", pool.WorkerWorking)

	// Toggle worker-1 fullscreen
	m = m.toggleFullscreenPane(PaneWorker, 0)
	require.Equal(t, PaneWorker, m.fullscreenPaneType)
	require.Equal(t, 0, m.fullscreenWorkerIndex)

	// Toggle worker-2 fullscreen (switches, doesn't exit)
	m = m.toggleFullscreenPane(PaneWorker, 1)
	require.Equal(t, PaneWorker, m.fullscreenPaneType)
	require.Equal(t, 1, m.fullscreenWorkerIndex)

	// Toggle worker-2 again to exit
	m = m.toggleFullscreenPane(PaneWorker, 1)
	require.Equal(t, PaneNone, m.fullscreenPaneType)
	require.Equal(t, -1, m.fullscreenWorkerIndex)
}

// TestToggleFullscreenPane_WorkerNoWorkers tests that toggling with no workers is a no-op.
func TestToggleFullscreenPane_WorkerNoWorkers(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// No workers exist
	require.Equal(t, PaneNone, m.fullscreenPaneType)

	// Toggle should be no-op
	m = m.toggleFullscreenPane(PaneWorker, 0)
	require.Equal(t, PaneNone, m.fullscreenPaneType)
}

// TestToggleFullscreenPane_SwitchBetweenPaneTypes tests switching between different pane types.
func TestToggleFullscreenPane_SwitchBetweenPaneTypes(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m = m.UpdateWorker("worker-1", pool.WorkerWorking)

	// Start with coordinator
	m = m.toggleFullscreenPane(PaneCoordinator, 0)
	require.Equal(t, PaneCoordinator, m.fullscreenPaneType)

	// Switch to messages
	m = m.toggleFullscreenPane(PaneMessages, 0)
	require.Equal(t, PaneMessages, m.fullscreenPaneType)

	// Switch to worker
	m = m.toggleFullscreenPane(PaneWorker, 0)
	require.Equal(t, PaneWorker, m.fullscreenPaneType)

	// Back to coordinator
	m = m.toggleFullscreenPane(PaneCoordinator, 0)
	require.Equal(t, PaneCoordinator, m.fullscreenPaneType)
}

// TestUpdate_CtrlF_TogglesNavigationMode tests that ctrl+f toggles navigation mode.
func TestUpdate_CtrlF_TogglesNavigationMode(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Initially not in navigation mode
	require.False(t, m.navigationMode)
	require.True(t, m.input.Focused())

	// Press ctrl+f
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	require.True(t, m.navigationMode)
	require.False(t, m.input.Focused())

	// Press ctrl+f again
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	require.False(t, m.navigationMode)
	require.True(t, m.input.Focused())
}

// TestUpdate_NavigationMode_NumberKeys tests that number keys work in navigation mode.
func TestUpdate_NavigationMode_NumberKeys(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m = m.UpdateWorker("worker-1", pool.WorkerWorking)
	m = m.UpdateWorker("worker-2", pool.WorkerWorking)

	// Enter navigation mode
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	require.True(t, m.navigationMode)

	// Press '1' for worker-1
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	require.Equal(t, PaneWorker, m.fullscreenPaneType)
	require.Equal(t, 0, m.fullscreenWorkerIndex)

	// Press '2' for worker-2
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	require.Equal(t, PaneWorker, m.fullscreenPaneType)
	require.Equal(t, 1, m.fullscreenWorkerIndex)

	// Press '5' for coordinator
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'5'}})
	require.Equal(t, PaneCoordinator, m.fullscreenPaneType)

	// Press '6' for messages
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'6'}})
	require.Equal(t, PaneMessages, m.fullscreenPaneType)
}

// TestUpdate_NavigationMode_Escape tests that escape exits navigation mode.
func TestUpdate_NavigationMode_Escape(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Enter navigation mode and select a pane
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'5'}})
	require.True(t, m.navigationMode)
	require.Equal(t, PaneCoordinator, m.fullscreenPaneType)

	// Press escape
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	require.False(t, m.navigationMode)
	require.Equal(t, PaneNone, m.fullscreenPaneType)
	require.True(t, m.input.Focused())
}

// TestUpdate_NumberKeysIgnoredWhenNotInNavigationMode tests that number keys type into input normally.
func TestUpdate_NumberKeysIgnoredWhenNotInNavigationMode(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m = m.UpdateWorker("worker-1", pool.WorkerWorking)

	// Not in navigation mode, input is focused
	require.False(t, m.navigationMode)
	require.True(t, m.input.Focused())
	require.Equal(t, PaneNone, m.fullscreenPaneType)

	// Press "1" key - should go to input, not toggle fullscreen
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	require.Equal(t, PaneNone, m.fullscreenPaneType, "should not enter fullscreen when not in navigation mode")
	require.Contains(t, m.input.Value(), "1", "number should be typed into input")
}
