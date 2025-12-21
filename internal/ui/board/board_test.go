package board

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/require"

	"perles/internal/beads"
	"perles/internal/config"
	"perles/internal/mocks"
)

func TestBoard_New_DefaultFocus(t *testing.T) {
	m := NewFromViews(config.DefaultViews(), nil, nil)
	require.Equal(t, ColReady, m.FocusedColumn(), "expected default focus on Ready column")
}

func TestBoard_NavigateRight(t *testing.T) {
	m := NewFromViews(config.DefaultViews(), nil, nil)
	// Default focus is Ready (index 1)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	require.Equal(t, ColInProgress, m.FocusedColumn(), "expected ColInProgress after 'l'")
}

func TestBoard_NavigateLeft(t *testing.T) {
	m := NewFromViews(config.DefaultViews(), nil, nil)
	// Default focus is Ready (index 1)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	require.Equal(t, ColBlocked, m.FocusedColumn(), "expected ColBlocked after 'h'")
}

func TestBoard_NavigateRightBoundary(t *testing.T) {
	m := NewFromViews(config.DefaultViews(), nil, nil)
	m = m.SetFocus(ColClosed)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	require.Equal(t, ColClosed, m.FocusedColumn(), "expected to stay at ColClosed boundary")
}

func TestBoard_NavigateLeftBoundary(t *testing.T) {
	m := NewFromViews(config.DefaultViews(), nil, nil)
	m = m.SetFocus(ColBlocked)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	require.Equal(t, ColBlocked, m.FocusedColumn(), "expected to stay at ColBlocked boundary")
}

func TestBoard_NavigateWithArrowKeys(t *testing.T) {
	m := NewFromViews(config.DefaultViews(), nil, nil)
	// Test right arrow
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	require.Equal(t, ColInProgress, m.FocusedColumn(), "expected ColInProgress after right arrow")

	// Test left arrow
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	require.Equal(t, ColReady, m.FocusedColumn(), "expected ColReady after left arrow")
}

func TestBoard_SetFocus(t *testing.T) {
	m := NewFromViews(config.DefaultViews(), nil, nil)
	m = m.SetFocus(ColClosed)
	require.Equal(t, ColClosed, m.FocusedColumn())
}

func TestBoard_SetFocus_InvalidIndex(t *testing.T) {
	m := NewFromViews(config.DefaultViews(), nil, nil)
	original := m.FocusedColumn()
	m = m.SetFocus(ColumnIndex(100)) // Invalid
	require.Equal(t, original, m.FocusedColumn(), "expected focus to remain for invalid index")
}

func TestBoard_SelectedIssue_Empty(t *testing.T) {
	m := NewFromViews(config.DefaultViews(), nil, nil)
	require.Nil(t, m.SelectedIssue(), "expected nil selected issue on empty board")
}

func TestBoard_SelectByID_NotFound(t *testing.T) {
	m := NewFromViews(config.DefaultViews(), nil, nil)
	_, found := m.SelectByID("nonexistent")
	require.False(t, found, "expected not to find nonexistent issue")
}

func TestBoard_SetSize(t *testing.T) {
	m := NewFromViews(config.DefaultViews(), nil, nil)
	_ = m.SetSize(120, 40)
	// SetSize modifies internal dimensions
	// Verified through View output
}

func TestBoard_View(t *testing.T) {
	m := NewFromViews(config.DefaultViews(), nil, nil)
	m = m.SetSize(120, 40)
	view := m.View()
	require.NotEmpty(t, view, "expected non-empty view")
}

// TestBoard_View_Golden uses teatest golden file comparison
// Run with -update flag to update golden files: go test -update ./internal/ui/board/...
func TestBoard_View_Golden(t *testing.T) {
	m := NewFromViews(config.DefaultViews(), nil, nil).SetSize(120, 40)
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestBoard_CustomColumns(t *testing.T) {
	configs := []config.ColumnConfig{
		{Name: "Todo", Query: "status = open", Color: "#EF4444"},
		{Name: "Done", Query: "status = closed", Color: "#10B981"},
	}

	board := NewFromViews([]config.ViewConfig{{Name: "Test", Columns: configs}}, nil, nil)
	require.Equal(t, 2, board.ColCount())
	require.Equal(t, 1, board.FocusedColumn()) // Second column by default
}

func TestBoard_CustomColumns_SingleColumn(t *testing.T) {
	configs := []config.ColumnConfig{
		{Name: "All", Query: "status = open"},
	}

	board := NewFromViews([]config.ViewConfig{{Name: "Test", Columns: configs}}, nil, nil)
	require.Equal(t, 1, board.ColCount())
	require.Equal(t, 0, board.FocusedColumn()) // First and only column
}

func TestBoard_CustomColumns_Navigation(t *testing.T) {
	configs := []config.ColumnConfig{
		{Name: "Col1", Query: "status = open"},
		{Name: "Col2", Query: "status = in_progress"},
		{Name: "Col3", Query: "status = closed"},
	}

	m := NewFromViews([]config.ViewConfig{{Name: "Test", Columns: configs}}, nil, nil)
	require.Equal(t, 1, m.FocusedColumn()) // Start on second column

	// Navigate right
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	require.Equal(t, 2, m.FocusedColumn())

	// Try to go past boundary
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	require.Equal(t, 2, m.FocusedColumn(), "should stay at boundary")

	// Navigate left
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	require.Equal(t, 1, m.FocusedColumn())

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	require.Equal(t, 0, m.FocusedColumn())

	// Try to go past boundary
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	require.Equal(t, 0, m.FocusedColumn(), "should stay at boundary")
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

	m := NewFromViews(views, nil, nil)

	require.Equal(t, 2, m.ViewCount())
	require.Equal(t, 0, m.CurrentViewIndex())
	require.Equal(t, "View1", m.CurrentViewName())
	require.Equal(t, 2, m.ColCount()) // View1 has 2 columns
}

func TestBoard_CycleViewNext(t *testing.T) {
	views := []config.ViewConfig{
		{Name: "View0", Columns: []config.ColumnConfig{{Name: "C0", Query: "q"}}},
		{Name: "View1", Columns: []config.ColumnConfig{{Name: "C1", Query: "q"}}},
		{Name: "View2", Columns: []config.ColumnConfig{{Name: "C2", Query: "q"}}},
	}

	m := NewFromViews(views, nil, nil)
	require.Equal(t, 0, m.CurrentViewIndex())
	require.Equal(t, "View0", m.CurrentViewName())

	// Cycle 0 -> 1
	m, _ = m.CycleViewNext()
	require.Equal(t, 1, m.CurrentViewIndex())
	require.Equal(t, "View1", m.CurrentViewName())

	// Cycle 1 -> 2
	m, _ = m.CycleViewNext()
	require.Equal(t, 2, m.CurrentViewIndex())
	require.Equal(t, "View2", m.CurrentViewName())

	// Cycle 2 -> 0 (wraparound)
	m, _ = m.CycleViewNext()
	require.Equal(t, 0, m.CurrentViewIndex())
	require.Equal(t, "View0", m.CurrentViewName())
}

func TestBoard_CycleViewPrev(t *testing.T) {
	views := []config.ViewConfig{
		{Name: "View0", Columns: []config.ColumnConfig{{Name: "C0", Query: "q"}}},
		{Name: "View1", Columns: []config.ColumnConfig{{Name: "C1", Query: "q"}}},
		{Name: "View2", Columns: []config.ColumnConfig{{Name: "C2", Query: "q"}}},
	}

	m := NewFromViews(views, nil, nil)
	require.Equal(t, 0, m.CurrentViewIndex())

	// Cycle 0 -> 2 (wraparound backward)
	m, _ = m.CycleViewPrev()
	require.Equal(t, 2, m.CurrentViewIndex())
	require.Equal(t, "View2", m.CurrentViewName())

	// Cycle 2 -> 1
	m, _ = m.CycleViewPrev()
	require.Equal(t, 1, m.CurrentViewIndex())
	require.Equal(t, "View1", m.CurrentViewName())

	// Cycle 1 -> 0
	m, _ = m.CycleViewPrev()
	require.Equal(t, 0, m.CurrentViewIndex())
	require.Equal(t, "View0", m.CurrentViewName())
}

func TestBoard_CycleViewNext_SingleView(t *testing.T) {
	views := []config.ViewConfig{
		{Name: "OnlyView", Columns: []config.ColumnConfig{{Name: "C", Query: "q"}}},
	}

	m := NewFromViews(views, nil, nil)
	require.Equal(t, 0, m.CurrentViewIndex())

	// Cycling with single view should do nothing
	m, _ = m.CycleViewNext()
	require.Equal(t, 0, m.CurrentViewIndex())
	require.Equal(t, "OnlyView", m.CurrentViewName())
}

func TestBoard_CycleViewPrev_SingleView(t *testing.T) {
	views := []config.ViewConfig{
		{Name: "OnlyView", Columns: []config.ColumnConfig{{Name: "C", Query: "q"}}},
	}

	m := NewFromViews(views, nil, nil)

	// Cycling with single view should do nothing
	m, _ = m.CycleViewPrev()
	require.Equal(t, 0, m.CurrentViewIndex())
	require.Equal(t, "OnlyView", m.CurrentViewName())
}

func TestBoard_SetCurrentViewName(t *testing.T) {
	views := []config.ViewConfig{
		{Name: "View0", Columns: []config.ColumnConfig{{Name: "C0", Query: "q"}}},
		{Name: "View1", Columns: []config.ColumnConfig{{Name: "C1", Query: "q"}}},
	}

	m := NewFromViews(views, nil, nil)
	require.Equal(t, "View0", m.CurrentViewName())

	m = m.SetCurrentViewName("Renamed")
	require.Equal(t, "Renamed", m.CurrentViewName())
	require.Equal(t, 0, m.CurrentViewIndex())
}

func TestBoard_SetCurrentViewName_PreservesOtherViews(t *testing.T) {
	views := []config.ViewConfig{
		{Name: "View0", Columns: []config.ColumnConfig{{Name: "C0", Query: "q"}}},
		{Name: "View1", Columns: []config.ColumnConfig{{Name: "C1", Query: "q"}}},
	}

	m := NewFromViews(views, nil, nil)
	m = m.SetCurrentViewName("Renamed")

	// Switch to View1 and verify it's unchanged
	m, _ = m.CycleViewNext()
	require.Equal(t, "View1", m.CurrentViewName())

	// Switch back and verify rename persisted
	m, _ = m.CycleViewPrev()
	require.Equal(t, "Renamed", m.CurrentViewName())
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

	m := NewFromViews(views, nil, nil)
	require.Equal(t, 2, m.ColCount()) // View1 has 2 columns

	// Switch to View2
	m, _ = m.CycleViewNext()
	require.Equal(t, 3, m.ColCount()) // View2 has 3 columns
	require.Equal(t, "View2", m.CurrentViewName())
}

func TestBoard_LoadCurrentViewCmd_NoExecutor(t *testing.T) {
	views := []config.ViewConfig{
		{Name: "View", Columns: []config.ColumnConfig{{Name: "C", Query: "q"}}},
	}

	m := NewFromViews(views, nil, nil)
	cmd := m.LoadCurrentViewCmd()
	// With no executor, LoadIssuesCmdForView returns nil for each column
	// and the batch should be nil
	require.Nil(t, cmd)
}

func TestBoard_ColumnLoadedMsg_UpdatesCorrectView(t *testing.T) {
	views := []config.ViewConfig{
		{Name: "View0", Columns: []config.ColumnConfig{{Name: "Col0", Query: "q0"}}},
		{Name: "View1", Columns: []config.ColumnConfig{{Name: "Col1", Query: "q1"}}},
	}

	m := NewFromViews(views, nil, nil)
	require.Equal(t, 0, m.CurrentViewIndex())

	// Message for current view (0) should be processed
	msg := ColumnLoadedMsg{
		ViewIndex:   0,
		ColumnIndex: 0,
		ColumnTitle: "Col0",
		Issues:      nil,
		Err:         nil,
	}
	m, _ = m.Update(msg)
	// No crash means success

	// Message for other view (1) should be ignored
	msg2 := ColumnLoadedMsg{
		ViewIndex:   1,
		ColumnIndex: 0,
		ColumnTitle: "Col1",
		Issues:      nil,
		Err:         nil,
	}
	m, _ = m.Update(msg2)
	// Still on view 0
	require.Equal(t, 0, m.CurrentViewIndex())
}

func TestBoard_SwitchToView(t *testing.T) {
	views := []config.ViewConfig{
		{Name: "View0", Columns: []config.ColumnConfig{{Name: "C0", Query: "q"}}},
		{Name: "View1", Columns: []config.ColumnConfig{{Name: "C1", Query: "q"}}},
		{Name: "View2", Columns: []config.ColumnConfig{{Name: "C2", Query: "q"}}},
	}

	m := NewFromViews(views, nil, nil)
	require.Equal(t, 0, m.CurrentViewIndex())
	require.Equal(t, "View0", m.CurrentViewName())

	// Switch directly to view 2
	m, _ = m.SwitchToView(2)
	require.Equal(t, 2, m.CurrentViewIndex())
	require.Equal(t, "View2", m.CurrentViewName())

	// Switch back to view 0
	m, _ = m.SwitchToView(0)
	require.Equal(t, 0, m.CurrentViewIndex())
	require.Equal(t, "View0", m.CurrentViewName())
}

func TestBoard_SwitchToView_InvalidIndex(t *testing.T) {
	views := []config.ViewConfig{
		{Name: "OnlyView", Columns: []config.ColumnConfig{{Name: "C", Query: "q"}}},
	}

	m := NewFromViews(views, nil, nil)
	require.Equal(t, 0, m.CurrentViewIndex())

	// Invalid indices should be no-ops
	m, _ = m.SwitchToView(-1)
	require.Equal(t, 0, m.CurrentViewIndex())

	m, _ = m.SwitchToView(5)
	require.Equal(t, 0, m.CurrentViewIndex())
}

func TestBoard_InvalidateViews(t *testing.T) {
	views := []config.ViewConfig{
		{Name: "View0", Columns: []config.ColumnConfig{{Name: "A", Query: "qa"}}},
		{Name: "View1", Columns: []config.ColumnConfig{{Name: "B", Query: "qb"}}},
		{Name: "View2", Columns: []config.ColumnConfig{{Name: "C", Query: "qc"}}},
	}

	m := NewFromViews(views, nil, nil)

	// Mark all views as loaded (simulating they've been visited)
	m.views[0].loaded = true
	m.views[1].loaded = true
	m.views[2].loaded = true

	// Invalidate all views
	m = m.InvalidateViews()

	// All views should now be marked as not loaded
	require.False(t, m.views[0].loaded, "View 0 should be invalidated")
	require.False(t, m.views[1].loaded, "View 1 should be invalidated")
	require.False(t, m.views[2].loaded, "View 2 should be invalidated")
}

func TestBoard_SwitchToView_ReloadsAfterInvalidate(t *testing.T) {
	views := []config.ViewConfig{
		{Name: "View0", Columns: []config.ColumnConfig{{Name: "A", Query: "qa"}}},
		{Name: "View1", Columns: []config.ColumnConfig{{Name: "B", Query: "qb"}}},
	}

	m := NewFromViews(views, nil, nil)

	// Initially view 0 is not loaded
	require.False(t, m.views[0].loaded, "View 0 should start unloaded")

	// Mark both views as loaded (simulating they've been visited)
	m.views[0].loaded = true
	m.views[1].loaded = true

	// Switch to view 1 - view is already loaded, loaded flag stays true
	m, _ = m.SwitchToView(1)
	require.True(t, m.views[1].loaded, "View 1 should still be marked loaded")

	// Invalidate all views
	m = m.InvalidateViews()
	require.False(t, m.views[0].loaded, "View 0 should be invalidated")
	require.False(t, m.views[1].loaded, "View 1 should be invalidated")

	// After invalidation, switching to a view will attempt to reload it
	// (the loaded flag is false, so switchToView will try to load)
	// Note: without an executor, LoadCurrentViewCmd returns nil, but the
	// important thing is that the loaded flag is false, indicating a reload
	// would be attempted if we had an executor
	m, _ = m.SwitchToView(0)
	// After the switch, view should be marked loaded (even without executor)
	// because ColumnLoadedMsg would mark it loaded - but since we don't
	// process that msg here, we just verify the invalidation worked
	require.False(t, m.views[0].loaded, "View 0 should still be unloaded (no executor)")
}

func TestBoard_EmptyColumns_ShowsEmptyState(t *testing.T) {
	// Board with no columns should show empty state message
	configs := []config.ColumnConfig{}
	m := NewFromViews([]config.ViewConfig{{Name: "Test", Columns: configs}}, nil, nil).SetSize(80, 24)

	view := m.View()
	require.Contains(t, view, "No columns configured")
	require.Contains(t, view, "Press 'a' to add a column")
}

func TestBoard_EmptyView_ColCount(t *testing.T) {
	// Board with no columns should have ColCount of 0
	configs := []config.ColumnConfig{}
	m := NewFromViews([]config.ViewConfig{{Name: "Test", Columns: configs}}, nil, nil)

	require.Equal(t, 0, m.ColCount())
}

func TestBoard_NewFromViews_EmptyColumns(t *testing.T) {
	// View with empty columns array should show empty state
	views := []config.ViewConfig{
		{
			Name:    "EmptyView",
			Columns: []config.ColumnConfig{}, // Empty columns
		},
	}

	m := NewFromViews(views, nil, nil).SetSize(80, 24)

	require.Equal(t, 1, m.ViewCount())
	require.Equal(t, "EmptyView", m.CurrentViewName())
	require.Equal(t, 0, m.ColCount())

	view := m.View()
	require.Contains(t, view, "No columns configured")
}

// Mixed column type tests (BQL + Tree)

func TestBoard_NewFromViews_MixedColumnTypes(t *testing.T) {
	views := []config.ViewConfig{
		{
			Name: "MixedView",
			Columns: []config.ColumnConfig{
				{Name: "BQL Column", Type: "bql", Query: "status = open"},
				{Name: "Tree Column", Type: "tree", IssueID: "perles-123", TreeMode: "deps"},
				{Name: "Default Column", Query: "status = closed"}, // type defaults to bql
			},
		},
	}

	m := NewFromViews(views, nil, nil)

	require.Equal(t, 3, m.ColCount())
	require.Equal(t, "MixedView", m.CurrentViewName())

	// Verify column types
	col0 := m.BoardColumn(0)
	_, isBQLCol0 := col0.(Column)
	require.True(t, isBQLCol0, "First column should be a BQL Column")

	col1 := m.BoardColumn(1)
	_, isTreeCol := col1.(TreeColumn)
	require.True(t, isTreeCol, "Second column should be a TreeColumn")

	col2 := m.BoardColumn(2)
	_, isBQLCol2 := col2.(Column)
	require.True(t, isBQLCol2, "Third column (default type) should be a BQL Column")
}

func TestBoard_MixedColumnTypes_Navigation(t *testing.T) {
	views := []config.ViewConfig{
		{
			Name: "MixedView",
			Columns: []config.ColumnConfig{
				{Name: "BQL", Type: "bql", Query: "status = open"},
				{Name: "Tree", Type: "tree", IssueID: "perles-123"},
				{Name: "BQL2", Type: "bql", Query: "status = closed"},
			},
		},
	}

	m := NewFromViews(views, nil, nil)
	require.Equal(t, 1, m.FocusedColumn()) // Default focus on second column

	// Navigate left (from tree to bql)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	require.Equal(t, 0, m.FocusedColumn())

	// Navigate right (from bql to tree)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	require.Equal(t, 1, m.FocusedColumn())

	// Navigate right (from tree to bql)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	require.Equal(t, 2, m.FocusedColumn())

	// Verify boundary
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	require.Equal(t, 2, m.FocusedColumn(), "should stay at boundary")
}

func TestBoard_TreeColumn_Color(t *testing.T) {
	views := []config.ViewConfig{
		{
			Name: "ColorView",
			Columns: []config.ColumnConfig{
				{Name: "Colored Tree", Type: "tree", IssueID: "perles-123", Color: "#EF4444"},
			},
		},
	}

	m := NewFromViews(views, nil, nil)

	col := m.BoardColumn(0)
	treeCol, ok := col.(TreeColumn)
	require.True(t, ok, "Column should be a TreeColumn")
	require.NotNil(t, treeCol.Color(), "TreeColumn should have a color set")
}

func TestBoard_TreeColumnLoadedMsg_UpdatesCorrectColumn(t *testing.T) {
	views := []config.ViewConfig{
		{
			Name: "MixedView",
			Columns: []config.ColumnConfig{
				{Name: "BQL", Type: "bql", Query: "status = open"},
				{Name: "Tree", Type: "tree", IssueID: "perles-123"},
			},
		},
	}

	m := NewFromViews(views, nil, nil)

	// Simulate TreeColumnLoadedMsg for current view
	msg := TreeColumnLoadedMsg{
		ViewIndex:   0,
		ColumnIndex: 1, // Tree is at index 1
		ColumnTitle: "Tree",
		RootID:      "perles-123",
		Issues:      nil,
		Err:         nil,
	}
	m, _ = m.Update(msg)
	// No crash means success

	// Message for wrong view should be ignored
	msg2 := TreeColumnLoadedMsg{
		ViewIndex:   1,
		ColumnIndex: 1,
		ColumnTitle: "Tree",
		RootID:      "perles-123",
		Issues:      nil,
		Err:         nil,
	}
	m, _ = m.Update(msg2)
	require.Equal(t, 0, m.CurrentViewIndex())
}

func TestBoard_TreeColumn_Mode(t *testing.T) {
	tests := []struct {
		name     string
		treeMode string
		expected string
	}{
		{name: "deps mode", treeMode: "deps", expected: "deps"},
		{name: "child mode", treeMode: "child", expected: "child"},
		{name: "empty defaults to deps", treeMode: "", expected: "deps"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			views := []config.ViewConfig{
				{
					Name: "View",
					Columns: []config.ColumnConfig{
						{Name: "Tree", Type: "tree", IssueID: "perles-123", TreeMode: tt.treeMode},
					},
				},
			}

			m := NewFromViews(views, nil, nil)

			col := m.BoardColumn(0)
			treeCol, ok := col.(TreeColumn)
			require.True(t, ok, "Column should be a TreeColumn")
			require.Equal(t, tt.expected, treeCol.Mode())
		})
	}
}

// TestBoard_View_WithTreeColumn_Golden tests board rendering with mixed BQL and tree columns.
// Run with -update flag to update golden files: go test -update ./internal/ui/board/...
// SwapColumns tests

func TestSwapColumns_Basic(t *testing.T) {
	configs := []config.ColumnConfig{
		{Name: "Col0", Query: "q0"},
		{Name: "Col1", Query: "q1"},
		{Name: "Col2", Query: "q2"},
	}

	m := NewFromViews([]config.ViewConfig{{Name: "Test", Columns: configs}}, nil, nil)
	require.Equal(t, "Col0", m.configs[0].Name)
	require.Equal(t, "Col1", m.configs[1].Name)
	require.Equal(t, "Col2", m.configs[2].Name)

	// Swap columns 0 and 2
	m = m.SwapColumns(0, 2)

	// Verify configs were swapped
	require.Equal(t, "Col2", m.configs[0].Name, "configs[0] should be Col2 after swap")
	require.Equal(t, "Col1", m.configs[1].Name, "configs[1] should be unchanged")
	require.Equal(t, "Col0", m.configs[2].Name, "configs[2] should be Col0 after swap")

	// Verify columns were swapped (via title)
	col0 := m.Column(0)
	col2 := m.Column(2)
	require.Contains(t, col0.Title(), "Col2", "column 0 title should be Col2")
	require.Contains(t, col2.Title(), "Col0", "column 2 title should be Col0")
}

func TestSwapColumns_SyncsToView(t *testing.T) {
	views := []config.ViewConfig{
		{
			Name: "View0",
			Columns: []config.ColumnConfig{
				{Name: "A", Query: "qa"},
				{Name: "B", Query: "qb"},
				{Name: "C", Query: "qc"},
			},
		},
	}

	m := NewFromViews(views, nil, nil)
	require.Equal(t, "A", m.configs[0].Name)
	require.Equal(t, "B", m.configs[1].Name)
	require.Equal(t, "C", m.configs[2].Name)

	// Swap columns 0 and 1
	m = m.SwapColumns(0, 1)

	// Verify model.configs updated
	require.Equal(t, "B", m.configs[0].Name, "model.configs[0] should be B")
	require.Equal(t, "A", m.configs[1].Name, "model.configs[1] should be A")

	// Verify view.configs synced
	require.Equal(t, "B", m.views[0].configs[0].Name, "view.configs[0] should be B")
	require.Equal(t, "A", m.views[0].configs[1].Name, "view.configs[1] should be A")
}

func TestSwapColumns_InvalidIndices(t *testing.T) {
	configs := []config.ColumnConfig{
		{Name: "Col0", Query: "q0"},
		{Name: "Col1", Query: "q1"},
	}

	m := NewFromViews([]config.ViewConfig{{Name: "Test", Columns: configs}}, nil, nil)
	original0 := m.configs[0].Name
	original1 := m.configs[1].Name

	// Test negative index
	m = m.SwapColumns(-1, 0)
	require.Equal(t, original0, m.configs[0].Name, "should be unchanged for negative i")
	require.Equal(t, original1, m.configs[1].Name, "should be unchanged for negative i")

	// Test out of bounds index
	m = m.SwapColumns(0, 5)
	require.Equal(t, original0, m.configs[0].Name, "should be unchanged for out of bounds j")
	require.Equal(t, original1, m.configs[1].Name, "should be unchanged for out of bounds j")

	// Test both negative
	m = m.SwapColumns(-1, -2)
	require.Equal(t, original0, m.configs[0].Name, "should be unchanged for both negative")
	require.Equal(t, original1, m.configs[1].Name, "should be unchanged for both negative")

	// Test both out of bounds
	m = m.SwapColumns(10, 20)
	require.Equal(t, original0, m.configs[0].Name, "should be unchanged for both out of bounds")
	require.Equal(t, original1, m.configs[1].Name, "should be unchanged for both out of bounds")
}

func TestSwapColumns_UpdatesColumnIndices(t *testing.T) {
	// This test verifies that column indices are updated after swapping
	// so that message routing continues to work correctly
	views := []config.ViewConfig{
		{
			Name: "View",
			Columns: []config.ColumnConfig{
				{Name: "Col0", Query: "q0"},
				{Name: "Col1", Query: "q1"},
				{Name: "Col2", Query: "q2"},
			},
		},
	}

	m := NewFromViews(views, nil, nil)

	// Verify initial column indices
	col0 := m.columns[0].(Column)
	col1 := m.columns[1].(Column)
	col2 := m.columns[2].(Column)
	require.Equal(t, 0, col0.ColumnIndex(), "col0 should have index 0")
	require.Equal(t, 1, col1.ColumnIndex(), "col1 should have index 1")
	require.Equal(t, 2, col2.ColumnIndex(), "col2 should have index 2")

	// Swap columns 0 and 2
	m = m.SwapColumns(0, 2)

	// After swap, the columns at positions 0 and 2 should have their indices updated
	// Column at position 0 (was at 2) should now have index 0
	// Column at position 2 (was at 0) should now have index 2
	swappedCol0 := m.columns[0].(Column)
	swappedCol2 := m.columns[2].(Column)
	require.Equal(t, 0, swappedCol0.ColumnIndex(), "column at position 0 should have index 0 after swap")
	require.Equal(t, 2, swappedCol2.ColumnIndex(), "column at position 2 should have index 2 after swap")

	// Verify message routing works after swap
	// Send a message for column at index 0 - should update column at position 0
	msg := ColumnLoadedMsg{
		ViewIndex:   0,
		ColumnIndex: 0,
		ColumnTitle: "Col2", // Title is "Col2" because it moved from position 2
		Issues: []beads.Issue{
			{ID: "bd-1", TitleText: "Test Issue"},
		},
	}
	m, _ = m.Update(msg)

	// The column at position 0 should have received the issues
	updatedCol0 := m.columns[0].(Column)
	require.Len(t, updatedCol0.Items(), 1, "column at position 0 should have received the message")

	// The column at position 2 should still be empty (wasn't targeted)
	updatedCol2 := m.columns[2].(Column)
	require.Empty(t, updatedCol2.Items(), "column at position 2 should not have received the message")
}

func TestBoard_View_WithTreeColumn_Golden(t *testing.T) {
	// Create board with mixed column types: BQL columns + tree column
	views := []config.ViewConfig{
		{
			Name: "Mixed",
			Columns: []config.ColumnConfig{
				{Name: "Backlog", Query: "status = open"},
				{Name: "Dependencies", Type: "tree", IssueID: "root-1", TreeMode: "deps"},
				{Name: "Done", Query: "status = closed"},
			},
		},
	}

	// Use a fixed time for deterministic golden test output
	fixedTime := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	clock := mocks.NewMockClock(t)
	clock.EXPECT().Now().Return(fixedTime).Maybe()
	m := NewFromViews(views, nil, clock)
	m = m.SetSize(120, 40)

	// Populate BQL columns with test issues
	backlogMsg := ColumnLoadedMsg{
		ViewIndex:   0,
		ColumnIndex: 0, // Backlog is at index 0
		ColumnTitle: "Backlog",
		Issues: []beads.Issue{
			{ID: "bd-1", TitleText: "Open Task", Priority: beads.PriorityMedium, Type: beads.TypeTask, Status: beads.StatusOpen},
			{ID: "bd-2", TitleText: "Open Bug", Priority: beads.PriorityHigh, Type: beads.TypeBug, Status: beads.StatusOpen},
		},
	}
	m, _ = m.Update(backlogMsg)

	doneMsg := ColumnLoadedMsg{
		ViewIndex:   0,
		ColumnIndex: 2, // Done is at index 2
		ColumnTitle: "Done",
		Issues: []beads.Issue{
			{ID: "bd-3", TitleText: "Completed Feature", Priority: beads.PriorityLow, Type: beads.TypeFeature, Status: beads.StatusClosed},
		},
	}
	m, _ = m.Update(doneMsg)

	// Populate tree column with dependency tree data
	// Set CreatedAt to fixedTime so relative time shows "now"
	treeMsg := TreeColumnLoadedMsg{
		ViewIndex:   0,
		ColumnIndex: 1, // Dependencies is at index 1
		ColumnTitle: "Dependencies",
		RootID:      "root-1",
		IssueMap: map[string]*beads.Issue{
			"root-1":  {ID: "root-1", TitleText: "Epic: Feature X", Type: beads.TypeEpic, Priority: beads.PriorityHigh, Children: []string{"child-1", "child-2"}, CreatedAt: fixedTime},
			"child-1": {ID: "child-1", TitleText: "Task: Backend API", Type: beads.TypeTask, Priority: beads.PriorityMedium, ParentID: "root-1", CreatedAt: fixedTime},
			"child-2": {ID: "child-2", TitleText: "Task: Frontend UI", Type: beads.TypeTask, Priority: beads.PriorityMedium, ParentID: "root-1", Children: []string{"child-3"}, CreatedAt: fixedTime},
			"child-3": {ID: "child-3", TitleText: "Subtask: Button", Type: beads.TypeTask, Priority: beads.PriorityLow, ParentID: "child-2", CreatedAt: fixedTime},
		},
	}
	m, _ = m.Update(treeMsg)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}
