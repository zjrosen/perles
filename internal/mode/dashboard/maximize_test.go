package dashboard

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/orchestration/controlplane"
	"github.com/zjrosen/perles/internal/testutil/zonetest"
)

// dashboardActionZoneIDs are the pane header action ([+]/[-]) zones that
// handleMouseMsg hit-tests. Maximizing a pane stops rendering the others.
var dashboardActionZoneIDs = []string{
	zoneWorkflowAction,
	zoneEpicTreeAction,
	zoneEpicDetailsAction,
	zoneCoordinatorContentAction,
	zoneCoordinatorInputAction,
}

// requireDashboardActionZoneInfo renders m, waits until bubblezone has stored
// the zones of that render, and returns the bounds of zoneID in it.
func requireDashboardActionZoneInfo(t *testing.T, m Model, zoneID string) *zone.ZoneInfo {
	t.Helper()

	// With no modal open, View() is zone.Scan(m.renderView()). Scanning that
	// render through zonetest waits for its zones to be stored, so the bounds
	// read here and by the click handler come from this layout, not the
	// previous one. Action zones from an earlier layout (or subtest) that this
	// render doesn't draw are cleared first, so the click can't match one of
	// them at the same position.
	zonetest.ScanAndWait(t, m.renderView(), dashboardActionZoneIDs...)

	z := zone.Get(zoneID)
	require.NotNil(t, z, "zone %q should be registered", zoneID)
	require.False(t, z.IsZero(), "zone %q should not be zero", zoneID)
	return z
}

func createDashboardCoordinatorTestModel(t *testing.T) Model {
	t.Helper()

	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
	}

	m, _ := createTestModel(t, workflows)
	m = m.SetSize(120, 40).(Model)

	panel := NewCoordinatorPanel(false, true, true, nil)
	panel.SetWorkflow(workflows[0].ID, nil)
	m.coordinatorPanel = panel
	m.showCoordinatorPanel = true
	m.focus = FocusTable
	m.updateComponentFocusStates()

	return m
}

func TestDashboard_MouseClick_HeaderActionsMaximizeAndRestorePanes(t *testing.T) {
	tests := []struct {
		name       string
		build      func(t *testing.T) Model
		zoneID     string
		pane       dashboardPane
		assertPane func(t *testing.T, m Model)
	}{
		{
			name: "workflow table",
			build: func(t *testing.T) Model {
				m, _ := createTestModel(t, []*controlplane.WorkflowInstance{
					createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
				})
				return m
			},
			zoneID: zoneWorkflowAction,
			pane:   dashboardPaneWorkflowTable,
			assertPane: func(t *testing.T, m Model) {
				require.Equal(t, FocusTable, m.focus, "workflow table should take focus")
			},
		},
		{
			name: "epic tree",
			build: func(t *testing.T) Model {
				m := createEpicTreeTestModelWithTree(t)
				m.focus = FocusTable
				m.epicViewFocus = EpicFocusDetails
				m.updateComponentFocusStates()
				return m
			},
			zoneID: zoneEpicTreeAction,
			pane:   dashboardPaneEpicTree,
			assertPane: func(t *testing.T, m Model) {
				require.Equal(t, FocusEpicView, m.focus, "epic tree should take epic focus")
				require.Equal(t, EpicFocusTree, m.epicViewFocus, "epic tree sub-focus should be selected")
			},
		},
		{
			name: "epic details",
			build: func(t *testing.T) Model {
				m := createEpicTreeTestModelWithTree(t)
				m.focus = FocusTable
				m.epicViewFocus = EpicFocusTree
				m.updateComponentFocusStates()
				return m
			},
			zoneID: zoneEpicDetailsAction,
			pane:   dashboardPaneEpicDetails,
			assertPane: func(t *testing.T, m Model) {
				require.Equal(t, FocusEpicView, m.focus, "epic details should take epic focus")
				require.Equal(t, EpicFocusDetails, m.epicViewFocus, "epic details sub-focus should be selected")
			},
		},
		{
			name:   "coordinator content",
			build:  createDashboardCoordinatorTestModel,
			zoneID: zoneCoordinatorContentAction,
			pane:   dashboardPaneCoordinatorContent,
			assertPane: func(t *testing.T, m Model) {
				require.Equal(t, FocusCoordinator, m.focus, "coordinator content should take coordinator focus")
				require.NotNil(t, m.coordinatorPanel, "coordinator panel should remain available")
				require.True(t, m.coordinatorPanel.IsFocused(), "coordinator panel should be focused")
			},
		},
		{
			name:   "coordinator input",
			build:  createDashboardCoordinatorTestModel,
			zoneID: zoneCoordinatorInputAction,
			pane:   dashboardPaneCoordinatorInput,
			assertPane: func(t *testing.T, m Model) {
				require.Equal(t, FocusCoordinator, m.focus, "coordinator input should take coordinator focus")
				require.NotNil(t, m.coordinatorPanel, "coordinator panel should remain available")
				require.True(t, m.coordinatorPanel.IsFocused(), "coordinator panel should be focused")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.build(t)

			z := requireDashboardActionZoneInfo(t, m, tt.zoneID)

			controller, cmd := m.Update(tea.MouseMsg{
				X:      z.StartX + 1,
				Y:      z.StartY,
				Button: tea.MouseButtonLeft,
				Action: tea.MouseActionRelease,
			})

			require.Nil(t, cmd, "header action click should not emit a command")
			m = controller.(Model)
			require.Equal(t, tt.pane, m.maximizedPane, "clicked pane should become fullscreen")
			tt.assertPane(t, m)

			z = requireDashboardActionZoneInfo(t, m, tt.zoneID)

			controller, cmd = m.Update(tea.MouseMsg{
				X:      z.StartX + 1,
				Y:      z.StartY,
				Button: tea.MouseButtonLeft,
				Action: tea.MouseActionRelease,
			})

			require.Nil(t, cmd, "restore click should not emit a command")
			m = controller.(Model)
			require.Equal(t, dashboardPaneNone, m.maximizedPane, "second click should restore the normal layout")
		})
	}
}
