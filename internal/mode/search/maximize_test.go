package search

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/task"
	"github.com/zjrosen/perles/internal/testutil/zonetest"
)

// searchActionZoneIDs are the pane header action ([+]/[-]) zones that
// handleMouseClick hit-tests. Maximizing a pane stops rendering the others.
var searchActionZoneIDs = []string{
	makeSearchPaneActionZoneID(searchPaneInput),
	makeSearchPaneActionZoneID(searchPaneResults),
	makeSearchPaneActionZoneID(searchPaneTree),
	makeSearchPaneActionZoneID(searchPaneDetails),
}

// requireSearchZoneInfo renders m, waits until bubblezone has stored the
// zones of that render, and returns the bounds of zoneID in it.
func requireSearchZoneInfo(t *testing.T, m Model, zoneID string) *zone.ZoneInfo {
	t.Helper()

	// With no overlay open, View() is zone.Scan(m.renderMainView()). Scanning
	// that render through zonetest waits for its zones to be stored, so the
	// bounds read here and by the click handler come from this layout, not the
	// previous one. Action zones from an earlier layout (or test) that this
	// render doesn't draw are cleared first, so the click can't match one of
	// them at the same position.
	zonetest.ScanAndWait(t, m.renderMainView(), searchActionZoneIDs...)

	z := zone.Get(zoneID)
	require.NotNil(t, z, "zone %q should be registered", zoneID)
	require.False(t, z.IsZero(), "zone %q should not be zero", zoneID)
	return z
}

func TestSearch_MouseClick_ListPaneHeaderActionsToggleMaximize(t *testing.T) {
	tests := []struct {
		name          string
		pane          searchPane
		expectedFocus FocusPane
		expectInput   bool
	}{
		{
			name:          "input",
			pane:          searchPaneInput,
			expectedFocus: FocusSearch,
			expectInput:   true,
		},
		{
			name:          "results",
			pane:          searchPaneResults,
			expectedFocus: FocusResults,
		},
		{
			name:          "details",
			pane:          searchPaneDetails,
			expectedFocus: FocusDetails,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := createTestModelWithResults(t).SetSize(100, 30)
			m.focus = FocusResults
			m.input.Blur()

			actionZoneID := makeSearchPaneActionZoneID(tt.pane)
			z := requireSearchZoneInfo(t, m, actionZoneID)

			m, cmd := m.Update(tea.MouseMsg{
				X:      z.StartX + 1,
				Y:      z.StartY,
				Button: tea.MouseButtonLeft,
				Action: tea.MouseActionRelease,
			})

			require.Nil(t, cmd, "header action click should not emit a command")
			require.Equal(t, tt.pane, m.effectiveMaximizedPane(), "clicked pane should become fullscreen")
			require.Equal(t, tt.expectedFocus, m.focus, "fullscreen pane should take focus")
			require.Equal(t, tt.expectInput, m.input.Focused(), "input focus should follow the active pane")

			z = requireSearchZoneInfo(t, m, actionZoneID)

			m, cmd = m.Update(tea.MouseMsg{
				X:      z.StartX + 1,
				Y:      z.StartY,
				Button: tea.MouseButtonLeft,
				Action: tea.MouseActionRelease,
			})

			require.Nil(t, cmd, "restore click should not emit a command")
			require.Equal(t, searchPaneNone, m.effectiveMaximizedPane(), "second click should restore the normal layout")
		})
	}
}

func TestSearch_MouseClick_TreeHeaderActionTogglesMaximize(t *testing.T) {
	rootIssue := task.Issue{
		ID:        "epic-1",
		TitleText: "Epic: Implement new feature",
		Type:      task.TypeEpic,
		Status:    task.StatusOpen,
		Priority:  1,
		Children:  []string{"task-1", "task-2"},
	}
	issues := []task.Issue{
		rootIssue,
		{ID: "task-1", TitleText: "Design API", Type: task.TypeTask, Status: task.StatusClosed, Priority: 1, ParentID: "epic-1"},
		{ID: "task-2", TitleText: "Implement backend", Type: task.TypeTask, Status: task.StatusInProgress, Priority: 1, ParentID: "epic-1"},
	}

	m := createTestModelWithTree(t, rootIssue, issues).SetSize(120, 30)
	m.focus = FocusDetails

	actionZoneID := makeSearchPaneActionZoneID(searchPaneTree)
	z := requireSearchZoneInfo(t, m, actionZoneID)

	m, cmd := m.Update(tea.MouseMsg{
		X:      z.StartX + 1,
		Y:      z.StartY,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionRelease,
	})

	require.Nil(t, cmd, "header action click should not emit a command")
	require.Equal(t, searchPaneTree, m.effectiveMaximizedPane(), "tree pane should become fullscreen")
	require.Equal(t, FocusResults, m.focus, "tree pane uses results focus")
	require.False(t, m.input.Focused(), "tree maximize should blur the search input")

	z = requireSearchZoneInfo(t, m, actionZoneID)

	m, cmd = m.Update(tea.MouseMsg{
		X:      z.StartX + 1,
		Y:      z.StartY,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionRelease,
	})

	require.Nil(t, cmd, "restore click should not emit a command")
	require.Equal(t, searchPaneNone, m.effectiveMaximizedPane(), "second click should restore the normal layout")
}
