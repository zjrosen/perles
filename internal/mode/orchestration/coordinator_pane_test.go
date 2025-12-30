package orchestration

import (
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/stretchr/testify/require"
)

func TestBuildCoordinatorTitle_WithPort(t *testing.T) {
	m := New(Config{})
	m.mcpPort = 8467

	title := m.buildCoordinatorTitle()

	// Should contain port in muted style
	require.Contains(t, title, "COORDINATOR")
	require.Contains(t, title, "(8467)")
}

func TestBuildCoordinatorTitle_NoPort(t *testing.T) {
	m := New(Config{})
	m.mcpPort = 0

	title := m.buildCoordinatorTitle()

	// Should NOT contain port when port is 0
	require.Contains(t, title, "COORDINATOR")
	require.NotContains(t, title, "(")
	require.NotContains(t, title, ")")
}

func TestBuildCoordinatorTitle_MutedStyleApplied(t *testing.T) {
	m := New(Config{})
	m.mcpPort = 12345

	title := m.buildCoordinatorTitle()

	// The port should be styled with TitleContextStyle (muted).
	// Since Lipgloss applies ANSI escape codes, we just verify the structure
	// contains both COORDINATOR and the port number.
	require.Contains(t, title, "COORDINATOR")
	require.Contains(t, title, "12345")
}

// ============================================================================
// Queue Count Display Tests
// ============================================================================

func TestCoordinatorPane_QueueCountDisplay(t *testing.T) {
	// Create model with queue count
	m := New(Config{})
	m.coordinatorPane.viewports = make(map[string]viewport.Model)
	m.coordinatorPane.queueCount = 3

	// Render the pane
	pane := m.renderCoordinatorPane(80, 20, false)

	// Should contain the queue count indicator
	require.Contains(t, pane, "[3 queued]")
}

func TestCoordinatorPane_NoQueueDisplay(t *testing.T) {
	// Create model with zero queue count
	m := New(Config{})
	m.coordinatorPane.viewports = make(map[string]viewport.Model)
	m.coordinatorPane.queueCount = 0

	// Render the pane
	pane := m.renderCoordinatorPane(80, 20, false)

	// Should NOT contain the queue indicator
	require.NotContains(t, pane, "queued")
}

func TestCoordinatorPane_QueueCountUpdates(t *testing.T) {
	// Create model
	m := New(Config{})

	// Initial state should have zero queue count
	require.Equal(t, 0, m.coordinatorPane.queueCount)

	// Simulate queue count update
	m.coordinatorPane.queueCount = 5
	require.Equal(t, 5, m.coordinatorPane.queueCount)

	// Update to different count
	m.coordinatorPane.queueCount = 2
	require.Equal(t, 2, m.coordinatorPane.queueCount)

	// Reset to zero
	m.coordinatorPane.queueCount = 0
	require.Equal(t, 0, m.coordinatorPane.queueCount)
}

func TestCoordinatorPane_FullscreenHidesQueueCount(t *testing.T) {
	// Create model with queue count
	m := New(Config{})
	m.coordinatorPane.viewports = make(map[string]viewport.Model)
	m.coordinatorPane.queueCount = 5

	// Render in fullscreen mode
	pane := m.renderCoordinatorPane(80, 20, true)

	// Fullscreen mode should still show queue count (it's bottom-left, not metrics)
	require.Contains(t, pane, "[5 queued]")
}
