package board

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"perles/internal/beads"
)

func TestColumn_NewColumn(t *testing.T) {
	c := NewColumn("Test", beads.StatusOpen)
	assert.Equal(t, "Test", c.title)
	assert.Equal(t, beads.StatusOpen, c.status)
}

func TestColumn_SetItems(t *testing.T) {
	c := NewColumn("Test", beads.StatusOpen)
	issues := []beads.Issue{
		{ID: "bd-1", TitleText: "Issue 1"},
		{ID: "bd-2", TitleText: "Issue 2"},
	}
	c = c.SetItems(issues)
	assert.Len(t, c.Items(), 2)
}

func TestColumn_SetItems_Empty(t *testing.T) {
	c := NewColumn("Test", beads.StatusOpen)
	c = c.SetItems([]beads.Issue{})
	assert.Empty(t, c.Items())
}

func TestColumn_SelectedItem_Empty(t *testing.T) {
	c := NewColumn("Test", beads.StatusOpen)
	assert.Nil(t, c.SelectedItem(), "expected nil selected item on empty column")
}

func TestColumn_SelectedItem_WithItems(t *testing.T) {
	c := NewColumn("Test", beads.StatusOpen)
	issues := []beads.Issue{
		{ID: "bd-1", TitleText: "Issue 1"},
		{ID: "bd-2", TitleText: "Issue 2"},
	}
	c = c.SetItems(issues)
	selected := c.SelectedItem()
	require.NotNil(t, selected, "expected non-nil selected item")
	assert.Equal(t, "bd-1", selected.ID, "expected first item selected")
}

func TestColumn_SelectByID(t *testing.T) {
	c := NewColumn("Test", beads.StatusOpen)
	issues := []beads.Issue{
		{ID: "bd-1", TitleText: "Issue 1"},
		{ID: "bd-2", TitleText: "Issue 2"},
		{ID: "bd-3", TitleText: "Issue 3"},
	}
	c = c.SetItems(issues)

	c, found := c.SelectByID("bd-2")
	require.True(t, found, "expected to find bd-2")
	selected := c.SelectedItem()
	require.NotNil(t, selected, "expected selected item")
	assert.Equal(t, "bd-2", selected.ID, "expected bd-2 to be selected")
}

func TestColumn_SelectByID_NotFound(t *testing.T) {
	c := NewColumn("Test", beads.StatusOpen)
	issues := []beads.Issue{{ID: "bd-1", TitleText: "Issue 1"}}
	c = c.SetItems(issues)

	_, found := c.SelectByID("nonexistent")
	assert.False(t, found, "expected not to find nonexistent issue")
}

func TestColumn_SetFocused(t *testing.T) {
	c := NewColumn("Test", beads.StatusOpen)
	c = c.SetFocused(true)
	assert.True(t, *c.focused, "expected column to be focused")
	c = c.SetFocused(false)
	assert.False(t, *c.focused, "expected column to be unfocused")
}

func TestColumn_SetSize(t *testing.T) {
	c := NewColumn("Test", beads.StatusOpen)
	c = c.SetSize(50, 20)
	assert.Equal(t, 50, c.width)
	assert.Equal(t, 20, c.height)
}

func TestColumn_Title_Empty(t *testing.T) {
	c := NewColumn("Test", beads.StatusOpen)
	title := c.Title()
	assert.Equal(t, "Test (0)", title)
}

func TestColumn_View_Empty(t *testing.T) {
	c := NewColumn("Test", beads.StatusOpen)
	c = c.SetSize(30, 10)
	view := c.View()
	assert.Contains(t, view, "No issues")
}

func TestColumn_Title_WithItems(t *testing.T) {
	c := NewColumn("Ready", beads.StatusOpen)
	issues := []beads.Issue{
		{ID: "bd-1", TitleText: "Issue 1", Priority: beads.PriorityHigh, Type: beads.TypeTask},
		{ID: "bd-2", TitleText: "Issue 2", Priority: beads.PriorityMedium, Type: beads.TypeBug},
	}
	c = c.SetItems(issues)
	title := c.Title()
	assert.Equal(t, "Ready (2)", title)
}

func TestColumn_View_WithItems(t *testing.T) {
	c := NewColumn("Ready", beads.StatusOpen)
	c = c.SetSize(50, 20)
	issues := []beads.Issue{
		{ID: "bd-1", TitleText: "Issue 1", Priority: beads.PriorityHigh, Type: beads.TypeTask},
		{ID: "bd-2", TitleText: "Issue 2", Priority: beads.PriorityMedium, Type: beads.TypeBug},
	}
	c = c.SetItems(issues)
	view := c.View()
	// View now returns only content, not header
	assert.NotEmpty(t, view, "expected non-empty view")
}

func TestColumn_SetShowCounts(t *testing.T) {
	c := NewColumn("Test", beads.StatusOpen)
	c = c.SetShowCounts(false)
	require.NotNil(t, c.showCounts)
	assert.False(t, *c.showCounts, "expected showCounts to be false")
	c = c.SetShowCounts(true)
	assert.True(t, *c.showCounts, "expected showCounts to be true")
}

func TestColumn_Title_ShowCountsFalse(t *testing.T) {
	c := NewColumn("Ready", beads.StatusOpen)
	issues := []beads.Issue{
		{ID: "bd-1", TitleText: "Issue 1"},
		{ID: "bd-2", TitleText: "Issue 2"},
	}
	c = c.SetItems(issues)
	c = c.SetShowCounts(false)
	title := c.Title()
	// Should show just title without count
	assert.Equal(t, "Ready", title)
}

func TestColumn_Title_ShowCountsTrue(t *testing.T) {
	c := NewColumn("Ready", beads.StatusOpen)
	issues := []beads.Issue{
		{ID: "bd-1", TitleText: "Issue 1"},
		{ID: "bd-2", TitleText: "Issue 2"},
	}
	c = c.SetItems(issues)
	c = c.SetShowCounts(true)
	title := c.Title()
	// Should show title with count
	assert.Equal(t, "Ready (2)", title)
}

func TestColumn_Title_ShowCountsDefault(t *testing.T) {
	// When showCounts is nil (not set), should default to showing counts
	c := NewColumn("Ready", beads.StatusOpen)
	issues := []beads.Issue{
		{ID: "bd-1", TitleText: "Issue 1"},
	}
	c = c.SetItems(issues)
	// Don't call SetShowCounts - leave as nil
	title := c.Title()
	assert.Equal(t, "Ready (1)", title)
}

func TestColumn_Update_NavigateDown(t *testing.T) {
	c := NewColumn("Test", beads.StatusOpen)
	issues := []beads.Issue{
		{ID: "bd-1", TitleText: "Issue 1"},
		{ID: "bd-2", TitleText: "Issue 2"},
	}
	c = c.SetItems(issues)

	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	selected := c.SelectedItem()
	require.NotNil(t, selected)
	assert.Equal(t, "bd-2", selected.ID, "expected bd-2 after down navigation")
}

func TestColumn_Update_NavigateUp(t *testing.T) {
	c := NewColumn("Test", beads.StatusOpen)
	issues := []beads.Issue{
		{ID: "bd-1", TitleText: "Issue 1"},
		{ID: "bd-2", TitleText: "Issue 2"},
	}
	c = c.SetItems(issues)

	// Navigate down first
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	// Then up
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	selected := c.SelectedItem()
	require.NotNil(t, selected)
	assert.Equal(t, "bd-1", selected.ID, "expected bd-1 after up navigation")
}

func TestColumn_Items(t *testing.T) {
	c := NewColumn("Test", beads.StatusOpen)
	issues := []beads.Issue{
		{ID: "bd-1", TitleText: "Issue 1"},
		{ID: "bd-2", TitleText: "Issue 2"},
	}
	c = c.SetItems(issues)
	items := c.Items()
	assert.Len(t, items, 2)
	assert.Equal(t, "bd-1", items[0].ID, "expected first item bd-1")
}

// TestColumn_View_Golden uses teatest golden file comparison
// Run with -update flag to update golden files: go test -update ./internal/ui/board/...
func TestColumn_View_Golden(t *testing.T) {
	c := NewColumn("Ready", beads.StatusOpen).SetSize(30, 15)
	view := c.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestColumn_View_WithIssues_Golden tests column with sample issues
func TestColumn_View_WithIssues_Golden(t *testing.T) {
	c := NewColumn("Ready", beads.StatusOpen).SetSize(30, 15).SetFocused(true)
	issues := []beads.Issue{
		{ID: "bd-1", TitleText: "First Issue", Priority: beads.PriorityHigh, Type: beads.TypeBug},
		{ID: "bd-2", TitleText: "Second Issue", Priority: beads.PriorityMedium, Type: beads.TypeTask},
		{ID: "bd-3", TitleText: "Third Issue", Priority: beads.PriorityLow, Type: beads.TypeFeature},
	}
	c = c.SetItems(issues)
	view := c.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// Tests for BQL self-loading functionality

func TestColumn_NewColumnWithExecutor(t *testing.T) {
	// NewColumnWithExecutor should create a column with executor and query set
	// We pass nil executor for unit test (actual execution tested elsewhere)
	c := NewColumnWithExecutor("Ready", "status = open", nil)
	assert.Equal(t, "Ready", c.title)
	assert.Equal(t, "status = open", c.Query())
	assert.Nil(t, c.executor)
}

func TestColumn_SetQuery(t *testing.T) {
	c := NewColumn("Test", beads.StatusOpen)
	c = c.SetQuery("status = open and ready = true")
	assert.Equal(t, "status = open and ready = true", c.Query())
}

func TestColumn_SetExecutor(t *testing.T) {
	c := NewColumn("Test", beads.StatusOpen)
	// We can't easily create a real executor without a DB, so just test the setter
	c = c.SetExecutor(nil)
	assert.Nil(t, c.executor)
}

func TestColumn_LoadingState(t *testing.T) {
	c := NewColumn("Test", beads.StatusOpen)

	// Default should be not loading
	assert.False(t, c.IsLoading())

	// Set loading
	c = c.SetLoading(true)
	assert.True(t, c.IsLoading())

	// Clear loading
	c = c.SetLoading(false)
	assert.False(t, c.IsLoading())
}

func TestColumn_LoadError(t *testing.T) {
	c := NewColumn("Test", beads.StatusOpen)

	// Default should have no error
	assert.Nil(t, c.LoadError())
}

func TestColumn_LoadIssues_NoExecutor(t *testing.T) {
	// Without executor, LoadIssues should be a no-op
	c := NewColumn("Test", beads.StatusOpen)
	c = c.SetQuery("status = open")

	// Should return unchanged column
	c2 := c.LoadIssues()
	assert.Empty(t, c2.Items())
}

func TestColumn_LoadIssues_NoQuery(t *testing.T) {
	// Without query, LoadIssues should be a no-op
	c := NewColumn("Test", beads.StatusOpen)
	// Don't set query

	c2 := c.LoadIssues()
	assert.Empty(t, c2.Items())
}

func TestColumn_LoadIssuesCmd_NoExecutor(t *testing.T) {
	c := NewColumn("Test", beads.StatusOpen)
	c = c.SetQuery("status = open")

	// Should return nil command
	cmd := c.LoadIssuesCmd()
	assert.Nil(t, cmd)
}

func TestColumn_LoadIssuesCmd_NoQuery(t *testing.T) {
	c := NewColumn("Test", beads.StatusOpen)
	// Don't set query

	cmd := c.LoadIssuesCmd()
	assert.Nil(t, cmd)
}

func TestColumnLoadedMsg_Structure(t *testing.T) {
	// Test that ColumnLoadedMsg can be constructed correctly
	issues := []beads.Issue{{ID: "test-1", TitleText: "Test"}}
	msg := ColumnLoadedMsg{
		ColumnTitle: "Ready",
		Issues:      issues,
		Err:         nil,
	}

	assert.Equal(t, "Ready", msg.ColumnTitle)
	assert.Len(t, msg.Issues, 1)
	assert.Nil(t, msg.Err)
}

func TestColumnLoadedMsg_WithError(t *testing.T) {
	msg := ColumnLoadedMsg{
		ColumnTitle: "Ready",
		Issues:      nil,
		Err:         assert.AnError,
	}

	assert.Equal(t, "Ready", msg.ColumnTitle)
	assert.Nil(t, msg.Issues)
	assert.Error(t, msg.Err)
}
