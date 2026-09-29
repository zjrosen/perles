package board

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/task"
	"github.com/zjrosen/perles/internal/ui/tree"
)

func TestNewTreeColumn(t *testing.T) {
	tc := NewTreeColumn("Deps", "bd-123", "deps", nil, nil)

	require.Equal(t, "Deps", tc.title)
	require.Equal(t, "bd-123", tc.rootID)
	require.Equal(t, tree.ModeDeps, tc.mode)
	require.NotNil(t, tc.focused)
}

func TestTreeColumn_ModeDefault(t *testing.T) {
	// When mode is empty, should default to "deps"
	tc := NewTreeColumn("Deps", "bd-123", "", nil, nil)
	require.Equal(t, tree.ModeDeps, tc.mode)
}

func TestTreeColumn_Title_WithMode(t *testing.T) {
	tc := NewTreeColumn("Deps", "bd-123", "deps", nil, nil)
	// Title now includes mode indicator
	require.Equal(t, "Deps (deps)", tc.Title())

	// Test with child mode
	tc = NewTreeColumn("Deps", "bd-123", "child", nil, nil)
	require.Equal(t, "Deps (child)", tc.Title())
}

func TestTreeColumn_LoadCmd_NoExecutor(t *testing.T) {
	tc := NewTreeColumn("Deps", "bd-123", "deps", nil, nil)
	cmd := tc.LoadCmd(0, 0)
	require.Nil(t, cmd, "LoadCmd should return nil without executor")
}

func TestTreeColumn_LoadCmd_NoRootID(t *testing.T) {
	tc := NewTreeColumn("Deps", "", "deps", nil, nil)
	cmd := tc.LoadCmd(0, 0)
	require.Nil(t, cmd, "LoadCmd should return nil without rootID")
}

func TestTreeColumn_RootID_Method(t *testing.T) {
	tc := NewTreeColumn("Deps", "bd-123", "deps", nil, nil)
	require.Equal(t, "bd-123", tc.RootID())
}

func TestTreeColumn_Mode_Method(t *testing.T) {
	tc := NewTreeColumn("Deps", "bd-123", "deps", nil, nil)
	require.Equal(t, "deps", tc.Mode())

	tc = NewTreeColumn("Children", "bd-456", "child", nil, nil)
	require.Equal(t, "child", tc.Mode())
}

func TestTreeColumn_ChildrenAliasForChildMode(t *testing.T) {
	// "children" is a backward-compatible alias for "child" (older search-mode
	// tree saves wrote tree_mode: children).
	tc := NewTreeColumn("Children", "bd-456", "children", nil, nil)
	require.Equal(t, tree.ModeChildren, tc.mode)
	require.Equal(t, "child", tc.Mode(), "alias should normalize to the canonical config value")
	require.Equal(t, "Children (child)", tc.Title())
}

func TestTreeColumn_UnknownModeDefaultsToDeps(t *testing.T) {
	tc := NewTreeColumn("Deps", "bd-123", "bogus", nil, nil)
	require.Equal(t, tree.ModeDeps, tc.mode)
}

func TestTreeColumn_HandleLoaded_WrongMessageType(t *testing.T) {
	tc := NewTreeColumn("Deps", "bd-123", "deps", nil, nil)
	// Send a ColumnLoadedMsg instead of TreeColumnLoadedMsg
	result := tc.HandleLoaded(ColumnLoadedMsg{ColumnTitle: "Deps"})
	// Should return unchanged
	resultTC := result.(TreeColumn)
	require.Nil(t, resultTC.tree)
}

func TestTreeColumn_HandleLoaded_WrongColumnIndex(t *testing.T) {
	tc := NewTreeColumn("Deps", "bd-123", "deps", nil, nil)
	tc = tc.SetColumnIndex(0)
	issueMap := map[string]*task.Issue{
		"bd-123": {ID: "bd-123", TitleText: "Root"},
	}
	msg := TreeColumnLoadedMsg{
		ColumnIndex: 1, // Different from tc.columnIndex (0)
		ColumnTitle: "Deps",
		RootID:      "bd-123",
		IssueMap:    issueMap,
	}
	result := tc.HandleLoaded(msg)
	// Should return unchanged because column index doesn't match
	resultTC := result.(TreeColumn)
	require.Nil(t, resultTC.tree)
}

func TestTreeColumn_HandleLoaded_Success(t *testing.T) {
	tc := NewTreeColumn("Deps", "bd-123", "deps", nil, nil)
	tc = tc.SetColumnIndex(0)

	issueMap := map[string]*task.Issue{
		"bd-123": {ID: "bd-123", TitleText: "Root Issue", Children: []string{"bd-124"}},
		"bd-124": {ID: "bd-124", TitleText: "Child Issue", ParentID: "bd-123"},
	}

	msg := TreeColumnLoadedMsg{
		ColumnIndex: 0, // Match tc.columnIndex
		ColumnTitle: "Deps",
		RootID:      "bd-123",
		IssueMap:    issueMap,
	}

	result := tc.HandleLoaded(msg)
	resultTC := result.(TreeColumn)

	require.NotNil(t, resultTC.tree, "tree should be initialized")
	require.Nil(t, resultTC.loadError)
}

func TestTreeColumn_LeafRoot_NoDirectionToggleHint(t *testing.T) {
	// Board tree columns are down-only and 'd' deletes the column in kanban,
	// so the root-only empty state must not suggest pressing 'd'.
	tc := NewTreeColumn("Deps", "bd-123", "deps", nil, nil)
	tc = tc.SetColumnIndex(0)

	msg := TreeColumnLoadedMsg{
		ColumnIndex: 0,
		ColumnTitle: "Deps",
		RootID:      "bd-123",
		IssueMap: map[string]*task.Issue{
			"bd-123": {ID: "bd-123", TitleText: "Leaf Root"},
		},
	}

	resultTC := tc.HandleLoaded(msg).SetSize(80, 20).(TreeColumn)
	view := resultTC.View()

	require.Contains(t, view, "No child dependencies found.")
	require.NotContains(t, view, "toggle direction")
}

func TestTreeColumn_HandleLoaded_RootNotFound(t *testing.T) {
	tc := NewTreeColumn("Deps", "bd-123", "deps", nil, nil)
	tc = tc.SetColumnIndex(0)

	// Empty issue map - root not found
	msg := TreeColumnLoadedMsg{
		ColumnIndex: 0,
		ColumnTitle: "Deps",
		RootID:      "bd-123",
		IssueMap:    map[string]*task.Issue{},
	}

	result := tc.HandleLoaded(msg)
	resultTC := result.(TreeColumn)

	require.Nil(t, resultTC.tree)
	require.NotNil(t, resultTC.loadError)
	require.Contains(t, resultTC.loadError.Error(), "not found")
}

func TestTreeColumn_HandleLoaded_Error(t *testing.T) {
	tc := NewTreeColumn("Deps", "bd-123", "deps", nil, nil)
	tc = tc.SetColumnIndex(0)

	msg := TreeColumnLoadedMsg{
		ColumnIndex: 0,
		ColumnTitle: "Deps",
		RootID:      "bd-123",
		Err:         errTest,
	}

	result := tc.HandleLoaded(msg)
	resultTC := result.(TreeColumn)

	require.Nil(t, resultTC.tree)
	require.NotNil(t, resultTC.loadError)
	require.Equal(t, errTest, resultTC.loadError)
}

var errTest = &testError{}

type testError struct{}

func (e *testError) Error() string { return "test error" }

func TestTreeColumn_View_Error(t *testing.T) {
	tc := NewTreeColumn("Deps", "bd-123", "deps", nil, nil)
	tc.loadError = errTest

	view := tc.View()
	require.Contains(t, view, "Error")
}

func TestTreeColumn_View_NoData(t *testing.T) {
	tc := NewTreeColumn("Deps", "bd-123", "deps", nil, nil)

	view := tc.View()
	require.Contains(t, view, "No tree data")
}

func TestTreeColumn_SetSize(t *testing.T) {
	tc := NewTreeColumn("Deps", "bd-123", "deps", nil, nil)
	result := tc.SetSize(100, 50)
	resultTC := result.(TreeColumn)

	require.Equal(t, 100, resultTC.width)
	require.Equal(t, 50, resultTC.height)
}

func TestTreeColumn_SetFocused(t *testing.T) {
	tc := NewTreeColumn("Deps", "bd-123", "deps", nil, nil)
	result := tc.SetFocused(true)
	resultTC := result.(TreeColumn)

	require.True(t, *resultTC.focused)

	result = resultTC.SetFocused(false)
	resultTC = result.(TreeColumn)
	require.False(t, *resultTC.focused)
}

func TestTreeColumn_Color(t *testing.T) {
	tc := NewTreeColumn("Deps", "bd-123", "deps", nil, nil)

	// Default color
	require.NotNil(t, tc.Color())

	// Custom color
	tc = tc.SetColor(lipgloss.Color("#FF0000"))
	require.Equal(t, lipgloss.Color("#FF0000"), tc.Color())
}

func TestTreeColumn_Width(t *testing.T) {
	tc := NewTreeColumn("Deps", "bd-123", "deps", nil, nil)
	tc = tc.SetSize(100, 50).(TreeColumn)
	require.Equal(t, 100, tc.Width())
}

func TestTreeColumn_SetShowCounts(t *testing.T) {
	tc := NewTreeColumn("Deps", "bd-123", "deps", nil, nil)
	// SetShowCounts is a no-op for TreeColumn (counts always shown)
	result := tc.SetShowCounts(false)
	require.NotNil(t, result)
}

func TestTreeColumn_IsEmpty(t *testing.T) {
	tc := NewTreeColumn("Deps", "bd-123", "deps", nil, nil)
	tc = tc.SetColumnIndex(0)
	require.True(t, tc.IsEmpty(), "should be empty without tree")

	// Initialize with tree data
	issueMap := map[string]*task.Issue{
		"bd-123": {ID: "bd-123", TitleText: "Root"},
	}
	msg := TreeColumnLoadedMsg{
		ColumnIndex: 0,
		ColumnTitle: "Deps",
		RootID:      "bd-123",
		IssueMap:    issueMap,
	}
	result := tc.HandleLoaded(msg)
	resultTC := result.(TreeColumn)

	require.False(t, resultTC.IsEmpty(), "should not be empty with tree data")
}

func TestTreeColumn_SelectedIssue_NoTree(t *testing.T) {
	tc := NewTreeColumn("Deps", "bd-123", "deps", nil, nil)
	require.Nil(t, tc.SelectedIssue())
}

func TestTreeColumn_SelectedIssue_WithTree(t *testing.T) {
	tc := NewTreeColumn("Deps", "bd-123", "deps", nil, nil)
	tc = tc.SetColumnIndex(0)

	issueMap := map[string]*task.Issue{
		"bd-123": {ID: "bd-123", TitleText: "Root Issue"},
	}
	msg := TreeColumnLoadedMsg{
		ColumnIndex: 0,
		ColumnTitle: "Deps",
		RootID:      "bd-123",
		IssueMap:    issueMap,
	}
	result := tc.HandleLoaded(msg)
	resultTC := result.(TreeColumn)

	selected := resultTC.SelectedIssue()
	require.NotNil(t, selected)
	require.Equal(t, "bd-123", selected.ID)
}

func TestTreeColumn_Update_NoTree(t *testing.T) {
	tc := NewTreeColumn("Deps", "bd-123", "deps", nil, nil)
	result, cmd := tc.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Nil(t, cmd)
	require.NotNil(t, result)
}

func TestTreeColumn_Update_Navigation(t *testing.T) {
	tc := NewTreeColumn("Deps", "bd-123", "deps", nil, nil)
	tc = tc.SetColumnIndex(0)

	// Initialize with tree data with multiple nodes
	issueMap := map[string]*task.Issue{
		"bd-123": {ID: "bd-123", TitleText: "Root", Children: []string{"bd-124"}},
		"bd-124": {ID: "bd-124", TitleText: "Child", ParentID: "bd-123"},
	}
	msg := TreeColumnLoadedMsg{
		ColumnIndex: 0,
		ColumnTitle: "Deps",
		RootID:      "bd-123",
		IssueMap:    issueMap,
	}
	result := tc.HandleLoaded(msg)
	resultTC := result.(TreeColumn)

	// Navigate down
	result, _ = resultTC.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	resultTC = result.(TreeColumn)

	selected := resultTC.SelectedIssue()
	require.NotNil(t, selected)
	// After pressing 'j', should move to child
	require.Equal(t, "bd-124", selected.ID)

	// Navigate up
	result, _ = resultTC.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	resultTC = result.(TreeColumn)

	selected = resultTC.SelectedIssue()
	require.NotNil(t, selected)
	// After pressing 'k', should be back at root
	require.Equal(t, "bd-123", selected.ID)
}

func TestTreeColumnLoadedMsg_Structure(t *testing.T) {
	issues := []task.Issue{{ID: "bd-1", TitleText: "Test"}}
	issueMap := map[string]*task.Issue{"bd-1": &issues[0]}

	msg := TreeColumnLoadedMsg{
		ViewIndex:   0,
		ColumnTitle: "Deps",
		RootID:      "bd-123",
		Issues:      issues,
		IssueMap:    issueMap,
		Err:         nil,
	}

	require.Equal(t, 0, msg.ViewIndex)
	require.Equal(t, "Deps", msg.ColumnTitle)
	require.Equal(t, "bd-123", msg.RootID)
	require.Len(t, msg.Issues, 1)
	require.Len(t, msg.IssueMap, 1)
	require.Nil(t, msg.Err)
}

func TestTreeColumn_Update_ToggleMode(t *testing.T) {
	tc := NewTreeColumn("Deps", "bd-123", "deps", nil, nil)
	tc = tc.SetColumnIndex(0)
	require.Equal(t, tree.ModeDeps, tc.mode)

	// Load tree data first so toggle has something to rebuild
	issueMap := map[string]*task.Issue{
		"bd-123": {ID: "bd-123", TitleText: "Root Issue"},
	}
	msg := TreeColumnLoadedMsg{
		ColumnIndex: 0,
		ColumnTitle: "Deps",
		RootID:      "bd-123",
		IssueMap:    issueMap,
	}
	result := tc.HandleLoaded(msg)
	tc = result.(TreeColumn)
	require.NotNil(t, tc.tree)

	// Press 'm' to toggle mode - rebuilds tree in place
	result, cmd := tc.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	resultTC := result.(TreeColumn)

	require.Equal(t, tree.ModeChildren, resultTC.mode)
	require.Nil(t, cmd, "should not return command (rebuild is synchronous)")

	// Press 'm' again to toggle back
	result, _ = resultTC.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	resultTC = result.(TreeColumn)
	require.Equal(t, tree.ModeDeps, resultTC.mode)
}

func TestTreeColumn_Update_ToggleMode_NoTree(t *testing.T) {
	// Toggle should be no-op without tree data
	tc := NewTreeColumn("Deps", "bd-123", "deps", nil, nil)
	result, cmd := tc.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	resultTC := result.(TreeColumn)

	// Mode field unchanged since no tree to rebuild
	require.Equal(t, tree.ModeDeps, resultTC.mode)
	require.Nil(t, cmd)
}

func TestTreeColumn_RightTitle_Empty(t *testing.T) {
	tc := NewTreeColumn("Deps", "bd-123", "deps", nil, nil)
	// Without tree data, RightTitle returns empty
	require.Equal(t, "", tc.RightTitle())
}
