package board

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/assert"

	"perles/internal/config"
)

func TestBoard_New_DefaultFocus(t *testing.T) {
	m := New()
	assert.Equal(t, ColReady, m.FocusedColumn(), "expected default focus on Ready column")
}

func TestBoard_NavigateRight(t *testing.T) {
	m := New()
	// Default focus is Ready (index 1)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	assert.Equal(t, ColInProgress, m.FocusedColumn(), "expected ColInProgress after 'l'")
}

func TestBoard_NavigateLeft(t *testing.T) {
	m := New()
	// Default focus is Ready (index 1)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	assert.Equal(t, ColBlocked, m.FocusedColumn(), "expected ColBlocked after 'h'")
}

func TestBoard_NavigateRightBoundary(t *testing.T) {
	m := New()
	m = m.SetFocus(ColClosed)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	assert.Equal(t, ColClosed, m.FocusedColumn(), "expected to stay at ColClosed boundary")
}

func TestBoard_NavigateLeftBoundary(t *testing.T) {
	m := New()
	m = m.SetFocus(ColBlocked)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	assert.Equal(t, ColBlocked, m.FocusedColumn(), "expected to stay at ColBlocked boundary")
}

func TestBoard_NavigateWithArrowKeys(t *testing.T) {
	m := New()
	// Test right arrow
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	assert.Equal(t, ColInProgress, m.FocusedColumn(), "expected ColInProgress after right arrow")

	// Test left arrow
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	assert.Equal(t, ColReady, m.FocusedColumn(), "expected ColReady after left arrow")
}

func TestBoard_SetFocus(t *testing.T) {
	m := New()
	m = m.SetFocus(ColClosed)
	assert.Equal(t, ColClosed, m.FocusedColumn())
}

func TestBoard_SetFocus_InvalidIndex(t *testing.T) {
	m := New()
	original := m.FocusedColumn()
	m = m.SetFocus(ColumnIndex(100)) // Invalid
	assert.Equal(t, original, m.FocusedColumn(), "expected focus to remain for invalid index")
}

func TestBoard_SelectedIssue_Empty(t *testing.T) {
	m := New()
	assert.Nil(t, m.SelectedIssue(), "expected nil selected issue on empty board")
}

func TestBoard_SelectByID_NotFound(t *testing.T) {
	m := New()
	_, found := m.SelectByID("nonexistent")
	assert.False(t, found, "expected not to find nonexistent issue")
}

func TestBoard_SetSize(t *testing.T) {
	m := New()
	_ = m.SetSize(120, 40)
	// SetSize modifies internal dimensions
	// Verified through View output
}

func TestBoard_View(t *testing.T) {
	m := New()
	m = m.SetSize(120, 40)
	view := m.View()
	assert.NotEmpty(t, view, "expected non-empty view")
}

// TestBoard_View_Golden uses teatest golden file comparison
// Run with -update flag to update golden files: go test -update ./internal/ui/board/...
func TestBoard_View_Golden(t *testing.T) {
	m := New().SetSize(120, 40)
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestBoard_CustomColumns(t *testing.T) {
	configs := []config.ColumnConfig{
		{Name: "Todo", Query: "status = open", Color: "#EF4444"},
		{Name: "Done", Query: "status = closed", Color: "#10B981"},
	}

	board := NewFromConfig(configs)
	assert.Equal(t, 2, board.ColCount())
	assert.Equal(t, 1, board.FocusedColumn()) // Second column by default
}

func TestBoard_CustomColumns_SingleColumn(t *testing.T) {
	configs := []config.ColumnConfig{
		{Name: "All", Query: "status = open"},
	}

	board := NewFromConfig(configs)
	assert.Equal(t, 1, board.ColCount())
	assert.Equal(t, 0, board.FocusedColumn()) // First and only column
}

func TestBoard_CustomColumns_Navigation(t *testing.T) {
	configs := []config.ColumnConfig{
		{Name: "Col1", Query: "status = open"},
		{Name: "Col2", Query: "status = in_progress"},
		{Name: "Col3", Query: "status = closed"},
	}

	m := NewFromConfig(configs)
	assert.Equal(t, 1, m.FocusedColumn()) // Start on second column

	// Navigate right
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	assert.Equal(t, 2, m.FocusedColumn())

	// Try to go past boundary
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	assert.Equal(t, 2, m.FocusedColumn(), "should stay at boundary")

	// Navigate left
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	assert.Equal(t, 1, m.FocusedColumn())

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	assert.Equal(t, 0, m.FocusedColumn())

	// Try to go past boundary
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	assert.Equal(t, 0, m.FocusedColumn(), "should stay at boundary")
}

// Multi-view tests

func TestBoard_NewFromViews(t *testing.T) {
	views := []config.ViewConfig{
		{
			Name: "View1",
			Columns: []config.ColumnConfig{
				{Name: "Open", Query: "status = open"},
				{Name: "Closed", Query: "status = closed"},
			},
		},
		{
			Name: "View2",
			Columns: []config.ColumnConfig{
				{Name: "InProgress", Query: "status = in_progress"},
			},
		},
	}

	m := NewFromViews(views, nil)

	assert.Equal(t, 2, m.ViewCount())
	assert.Equal(t, 0, m.CurrentViewIndex())
	assert.Equal(t, "View1", m.CurrentViewName())
	assert.Equal(t, 2, m.ColCount()) // View1 has 2 columns
}

func TestBoard_CycleViewNext(t *testing.T) {
	views := []config.ViewConfig{
		{Name: "View0", Columns: []config.ColumnConfig{{Name: "C0", Query: "q"}}},
		{Name: "View1", Columns: []config.ColumnConfig{{Name: "C1", Query: "q"}}},
		{Name: "View2", Columns: []config.ColumnConfig{{Name: "C2", Query: "q"}}},
	}

	m := NewFromViews(views, nil)
	assert.Equal(t, 0, m.CurrentViewIndex())
	assert.Equal(t, "View0", m.CurrentViewName())

	// Cycle 0 -> 1
	m, _ = m.CycleViewNext()
	assert.Equal(t, 1, m.CurrentViewIndex())
	assert.Equal(t, "View1", m.CurrentViewName())

	// Cycle 1 -> 2
	m, _ = m.CycleViewNext()
	assert.Equal(t, 2, m.CurrentViewIndex())
	assert.Equal(t, "View2", m.CurrentViewName())

	// Cycle 2 -> 0 (wraparound)
	m, _ = m.CycleViewNext()
	assert.Equal(t, 0, m.CurrentViewIndex())
	assert.Equal(t, "View0", m.CurrentViewName())
}

func TestBoard_CycleViewPrev(t *testing.T) {
	views := []config.ViewConfig{
		{Name: "View0", Columns: []config.ColumnConfig{{Name: "C0", Query: "q"}}},
		{Name: "View1", Columns: []config.ColumnConfig{{Name: "C1", Query: "q"}}},
		{Name: "View2", Columns: []config.ColumnConfig{{Name: "C2", Query: "q"}}},
	}

	m := NewFromViews(views, nil)
	assert.Equal(t, 0, m.CurrentViewIndex())

	// Cycle 0 -> 2 (wraparound backward)
	m, _ = m.CycleViewPrev()
	assert.Equal(t, 2, m.CurrentViewIndex())
	assert.Equal(t, "View2", m.CurrentViewName())

	// Cycle 2 -> 1
	m, _ = m.CycleViewPrev()
	assert.Equal(t, 1, m.CurrentViewIndex())
	assert.Equal(t, "View1", m.CurrentViewName())

	// Cycle 1 -> 0
	m, _ = m.CycleViewPrev()
	assert.Equal(t, 0, m.CurrentViewIndex())
	assert.Equal(t, "View0", m.CurrentViewName())
}

func TestBoard_CycleViewNext_SingleView(t *testing.T) {
	views := []config.ViewConfig{
		{Name: "OnlyView", Columns: []config.ColumnConfig{{Name: "C", Query: "q"}}},
	}

	m := NewFromViews(views, nil)
	assert.Equal(t, 0, m.CurrentViewIndex())

	// Cycling with single view should do nothing
	m, _ = m.CycleViewNext()
	assert.Equal(t, 0, m.CurrentViewIndex())
	assert.Equal(t, "OnlyView", m.CurrentViewName())
}

func TestBoard_CycleViewPrev_SingleView(t *testing.T) {
	views := []config.ViewConfig{
		{Name: "OnlyView", Columns: []config.ColumnConfig{{Name: "C", Query: "q"}}},
	}

	m := NewFromViews(views, nil)

	// Cycling with single view should do nothing
	m, _ = m.CycleViewPrev()
	assert.Equal(t, 0, m.CurrentViewIndex())
	assert.Equal(t, "OnlyView", m.CurrentViewName())
}

func TestBoard_ViewSwitchChangesColumns(t *testing.T) {
	views := []config.ViewConfig{
		{
			Name: "View1",
			Columns: []config.ColumnConfig{
				{Name: "A", Query: "qa"},
				{Name: "B", Query: "qb"},
			},
		},
		{
			Name: "View2",
			Columns: []config.ColumnConfig{
				{Name: "X", Query: "qx"},
				{Name: "Y", Query: "qy"},
				{Name: "Z", Query: "qz"},
			},
		},
	}

	m := NewFromViews(views, nil)
	assert.Equal(t, 2, m.ColCount()) // View1 has 2 columns

	// Switch to View2
	m, _ = m.CycleViewNext()
	assert.Equal(t, 3, m.ColCount()) // View2 has 3 columns
	assert.Equal(t, "View2", m.CurrentViewName())
}

func TestBoard_LoadCurrentViewCmd_NoExecutor(t *testing.T) {
	views := []config.ViewConfig{
		{Name: "View", Columns: []config.ColumnConfig{{Name: "C", Query: "q"}}},
	}

	m := NewFromViews(views, nil)
	cmd := m.LoadCurrentViewCmd()
	// With no executor, LoadIssuesCmdForView returns nil for each column
	// and the batch should be nil
	assert.Nil(t, cmd)
}

func TestBoard_ColumnLoadedMsg_UpdatesCorrectView(t *testing.T) {
	views := []config.ViewConfig{
		{Name: "View0", Columns: []config.ColumnConfig{{Name: "Col0", Query: "q0"}}},
		{Name: "View1", Columns: []config.ColumnConfig{{Name: "Col1", Query: "q1"}}},
	}

	m := NewFromViews(views, nil)
	assert.Equal(t, 0, m.CurrentViewIndex())

	// Message for current view (0) should be processed
	msg := ColumnLoadedMsg{
		ViewIndex:   0,
		ColumnTitle: "Col0",
		Issues:      nil,
		Err:         nil,
	}
	m, _ = m.Update(msg)
	// No crash means success

	// Message for other view (1) should be ignored
	msg2 := ColumnLoadedMsg{
		ViewIndex:   1,
		ColumnTitle: "Col1",
		Issues:      nil,
		Err:         nil,
	}
	m, _ = m.Update(msg2)
	// Still on view 0
	assert.Equal(t, 0, m.CurrentViewIndex())
}

func TestBoard_SwitchToView(t *testing.T) {
	views := []config.ViewConfig{
		{Name: "View0", Columns: []config.ColumnConfig{{Name: "C0", Query: "q"}}},
		{Name: "View1", Columns: []config.ColumnConfig{{Name: "C1", Query: "q"}}},
		{Name: "View2", Columns: []config.ColumnConfig{{Name: "C2", Query: "q"}}},
	}

	m := NewFromViews(views, nil)
	assert.Equal(t, 0, m.CurrentViewIndex())
	assert.Equal(t, "View0", m.CurrentViewName())

	// Switch directly to view 2
	m, _ = m.SwitchToView(2)
	assert.Equal(t, 2, m.CurrentViewIndex())
	assert.Equal(t, "View2", m.CurrentViewName())

	// Switch back to view 0
	m, _ = m.SwitchToView(0)
	assert.Equal(t, 0, m.CurrentViewIndex())
	assert.Equal(t, "View0", m.CurrentViewName())
}

func TestBoard_SwitchToView_InvalidIndex(t *testing.T) {
	views := []config.ViewConfig{
		{Name: "OnlyView", Columns: []config.ColumnConfig{{Name: "C", Query: "q"}}},
	}

	m := NewFromViews(views, nil)
	assert.Equal(t, 0, m.CurrentViewIndex())

	// Invalid indices should be no-ops
	m, _ = m.SwitchToView(-1)
	assert.Equal(t, 0, m.CurrentViewIndex())

	m, _ = m.SwitchToView(5)
	assert.Equal(t, 0, m.CurrentViewIndex())
}

func TestBoard_InvalidateViews(t *testing.T) {
	views := []config.ViewConfig{
		{Name: "View0", Columns: []config.ColumnConfig{{Name: "A", Query: "qa"}}},
		{Name: "View1", Columns: []config.ColumnConfig{{Name: "B", Query: "qb"}}},
		{Name: "View2", Columns: []config.ColumnConfig{{Name: "C", Query: "qc"}}},
	}

	m := NewFromViews(views, nil)

	// Mark all views as loaded (simulating they've been visited)
	m.views[0].loaded = true
	m.views[1].loaded = true
	m.views[2].loaded = true

	// Invalidate all views
	m = m.InvalidateViews()

	// All views should now be marked as not loaded
	assert.False(t, m.views[0].loaded, "View 0 should be invalidated")
	assert.False(t, m.views[1].loaded, "View 1 should be invalidated")
	assert.False(t, m.views[2].loaded, "View 2 should be invalidated")
}

func TestBoard_SwitchToView_ReloadsAfterInvalidate(t *testing.T) {
	views := []config.ViewConfig{
		{Name: "View0", Columns: []config.ColumnConfig{{Name: "A", Query: "qa"}}},
		{Name: "View1", Columns: []config.ColumnConfig{{Name: "B", Query: "qb"}}},
	}

	m := NewFromViews(views, nil)

	// Initially view 0 is not loaded
	assert.False(t, m.views[0].loaded, "View 0 should start unloaded")

	// Mark both views as loaded (simulating they've been visited)
	m.views[0].loaded = true
	m.views[1].loaded = true

	// Switch to view 1 - view is already loaded, loaded flag stays true
	m, _ = m.SwitchToView(1)
	assert.True(t, m.views[1].loaded, "View 1 should still be marked loaded")

	// Invalidate all views
	m = m.InvalidateViews()
	assert.False(t, m.views[0].loaded, "View 0 should be invalidated")
	assert.False(t, m.views[1].loaded, "View 1 should be invalidated")

	// After invalidation, switching to a view will attempt to reload it
	// (the loaded flag is false, so switchToView will try to load)
	// Note: without an executor, LoadCurrentViewCmd returns nil, but the
	// important thing is that the loaded flag is false, indicating a reload
	// would be attempted if we had an executor
	m, _ = m.SwitchToView(0)
	// After the switch, view should be marked loaded (even without executor)
	// because ColumnLoadedMsg would mark it loaded - but since we don't
	// process that msg here, we just verify the invalidation worked
	assert.False(t, m.views[0].loaded, "View 0 should still be unloaded (no executor)")
}

func TestBoard_EmptyColumns_ShowsEmptyState(t *testing.T) {
	// Board with no columns should show empty state message
	configs := []config.ColumnConfig{}
	m := NewFromConfig(configs).SetSize(80, 24)

	view := m.View()
	assert.Contains(t, view, "No columns configured")
	assert.Contains(t, view, "Press 'a' to add a column")
}

func TestBoard_EmptyView_ColCount(t *testing.T) {
	// Board with no columns should have ColCount of 0
	configs := []config.ColumnConfig{}
	m := NewFromConfig(configs)

	assert.Equal(t, 0, m.ColCount())
}

func TestBoard_NewFromViews_EmptyColumns(t *testing.T) {
	// View with empty columns array should show empty state
	views := []config.ViewConfig{
		{
			Name:    "EmptyView",
			Columns: []config.ColumnConfig{}, // Empty columns
		},
	}

	m := NewFromViews(views, nil).SetSize(80, 24)

	assert.Equal(t, 1, m.ViewCount())
	assert.Equal(t, "EmptyView", m.CurrentViewName())
	assert.Equal(t, 0, m.ColCount())

	view := m.View()
	assert.Contains(t, view, "No columns configured")
}
