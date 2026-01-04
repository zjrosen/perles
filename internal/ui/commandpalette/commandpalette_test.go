package commandpalette

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/require"
)

func testItems() []Item {
	return []Item{
		{ID: "debate", Name: "Technical Debate", Description: "Structured multi-perspective debate"},
		{ID: "research", Name: "Research Proposal", Description: "Collaborative research with synthesis"},
		{ID: "review", Name: "Code Review", Description: "Multi-perspective code review"},
	}
}

func TestCommandPalette_New(t *testing.T) {
	items := testItems()
	m := New(Config{
		Title:       "Select Workflow",
		Placeholder: "Search workflows...",
		Items:       items,
	})

	require.Equal(t, "Select Workflow", m.config.Title)
	require.Len(t, m.config.Items, 3)
	require.Equal(t, 0, m.cursor)
	require.Len(t, m.filtered, 3)
}

func TestCommandPalette_New_DefaultPlaceholder(t *testing.T) {
	m := New(Config{
		Items: testItems(),
	})

	require.Equal(t, "Search...", m.textInput.Placeholder)
}

func TestCommandPalette_Update_NavigateDown(t *testing.T) {
	m := New(Config{Items: testItems()})

	// Navigate down with arrow key
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, 1, m.cursor)

	// Navigate down again
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, 2, m.cursor)

	// At bottom boundary - should not go past
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, 2, m.cursor)
}

func TestCommandPalette_Update_NavigateUp(t *testing.T) {
	m := New(Config{Items: testItems()})
	m.cursor = 2

	// Navigate up with arrow key
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	require.Equal(t, 1, m.cursor)

	// Navigate up again
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	require.Equal(t, 0, m.cursor)

	// At top boundary - should not go past
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	require.Equal(t, 0, m.cursor)
}

func TestCommandPalette_Update_CtrlN_CtrlP(t *testing.T) {
	m := New(Config{Items: testItems()})

	// Navigate down with Ctrl+N
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	require.Equal(t, 1, m.cursor)

	// Navigate up with Ctrl+P
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	require.Equal(t, 0, m.cursor)
}

func TestCommandPalette_Selected(t *testing.T) {
	items := testItems()
	m := New(Config{Items: items})

	// Default selection
	selected, ok := m.Selected()
	require.True(t, ok)
	require.Equal(t, "debate", selected.ID)
	require.Equal(t, "Technical Debate", selected.Name)

	// After navigation
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	selected, ok = m.Selected()
	require.True(t, ok)
	require.Equal(t, "research", selected.ID)
}

func TestCommandPalette_Selected_Empty(t *testing.T) {
	m := New(Config{Items: []Item{}})

	selected, ok := m.Selected()
	require.False(t, ok)
	require.Equal(t, Item{}, selected)
}

func TestCommandPalette_Filter_ByName(t *testing.T) {
	m := New(Config{Items: testItems()})

	// Type "deb" to filter
	m.textInput.SetValue("deb")
	m = m.updateFilter()

	require.Len(t, m.filtered, 1)
	require.Equal(t, "debate", m.filtered[0].ID)
}

func TestCommandPalette_Filter_ByDescription(t *testing.T) {
	m := New(Config{Items: testItems()})

	// Type "synthesis" (only in Research description)
	m.textInput.SetValue("synthesis")
	m = m.updateFilter()

	require.Len(t, m.filtered, 1)
	require.Equal(t, "research", m.filtered[0].ID)
}

func TestCommandPalette_Filter_CaseInsensitive(t *testing.T) {
	m := New(Config{Items: testItems()})

	// Mixed case
	m.textInput.SetValue("DEBATE")
	m = m.updateFilter()

	require.Len(t, m.filtered, 1)
	require.Equal(t, "debate", m.filtered[0].ID)
}

func TestCommandPalette_Filter_NameMatchesFirst(t *testing.T) {
	items := []Item{
		{ID: "a", Name: "Alpha", Description: "Contains beta word"},
		{ID: "b", Name: "Beta", Description: "Something else"},
	}
	m := New(Config{Items: items})

	// "beta" matches both name and description
	m.textInput.SetValue("beta")
	m = m.updateFilter()

	require.Len(t, m.filtered, 2)
	// Name match should come first
	require.Equal(t, "b", m.filtered[0].ID)
	require.Equal(t, "a", m.filtered[1].ID)
}

func TestCommandPalette_Filter_NoMatches(t *testing.T) {
	m := New(Config{Items: testItems()})

	m.textInput.SetValue("nonexistent")
	m = m.updateFilter()

	require.Len(t, m.filtered, 0)
}

func TestCommandPalette_Filter_CursorReset(t *testing.T) {
	m := New(Config{Items: testItems()})

	// Move cursor to position 2
	m.cursor = 2

	// Filter to single item
	m.textInput.SetValue("debate")
	m = m.updateFilter()

	// Cursor should reset to 0 since position 2 is out of bounds
	require.Equal(t, 0, m.cursor)
}

func TestCommandPalette_ClearSearch(t *testing.T) {
	m := New(Config{Items: testItems()})

	// Type to filter
	m.textInput.SetValue("debate")
	m = m.updateFilter()
	require.Len(t, m.filtered, 1)

	// Ctrl+U clears search
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	require.Equal(t, "", m.textInput.Value())
	require.Len(t, m.filtered, 3)
}

func TestCommandPalette_Select_DefaultMsg(t *testing.T) {
	m := New(Config{Items: testItems()})

	// Press Enter
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	require.IsType(t, SelectMsg{}, msg)
	selectMsg := msg.(SelectMsg)
	require.Equal(t, "debate", selectMsg.Item.ID)
}

func TestCommandPalette_Select_CustomCallback(t *testing.T) {
	type myMsg struct{ id string }

	m := New(Config{
		Items: testItems(),
		OnSelect: func(item Item) tea.Msg {
			return myMsg{id: item.ID}
		},
	})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	require.IsType(t, myMsg{}, msg)
	require.Equal(t, "debate", msg.(myMsg).id)
}

func TestCommandPalette_Select_NoItems(t *testing.T) {
	m := New(Config{Items: testItems()})

	// Filter to empty
	m.textInput.SetValue("nonexistent")
	m = m.updateFilter()

	// Press Enter with no items
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.Nil(t, cmd)
}

func TestCommandPalette_Cancel_DefaultMsg(t *testing.T) {
	m := New(Config{Items: testItems()})

	// Press Escape
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	require.NotNil(t, cmd)

	msg := cmd()
	require.IsType(t, CancelMsg{}, msg)
}

func TestCommandPalette_Cancel_CustomCallback(t *testing.T) {
	type cancelledMsg struct{}

	m := New(Config{
		Items:    testItems(),
		OnCancel: func() tea.Msg { return cancelledMsg{} },
	})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	require.NotNil(t, cmd)

	msg := cmd()
	require.IsType(t, cancelledMsg{}, msg)
}

func TestCommandPalette_SetSize(t *testing.T) {
	m := New(Config{Items: testItems()})

	m = m.SetSize(120, 40)
	require.Equal(t, 120, m.viewportWidth)
	require.Equal(t, 40, m.viewportHeight)

	// Verify immutability
	m2 := m.SetSize(80, 24)
	require.Equal(t, 80, m2.viewportWidth)
	require.Equal(t, 120, m.viewportWidth)
}

func TestCommandPalette_Accessors(t *testing.T) {
	m := New(Config{Items: testItems()})

	require.Equal(t, 0, m.Cursor())
	require.Len(t, m.FilteredItems(), 3)
	require.Equal(t, "", m.SearchText())

	m.textInput.SetValue("test")
	require.Equal(t, "test", m.SearchText())
}

func TestCommandPalette_View_ContainsElements(t *testing.T) {
	m := New(Config{
		Title:       "Select Workflow",
		Placeholder: "Search workflows...",
		Items:       testItems(),
	}).SetSize(80, 24)

	view := m.View()

	// Should contain title
	require.Contains(t, view, "Select Workflow")

	// Should contain items
	require.Contains(t, view, "Technical Debate")
	require.Contains(t, view, "Research Proposal")
	require.Contains(t, view, "Code Review")

	// Badge values are now used for coloring, not displayed
	// Check that item names are visible instead
	require.Contains(t, view, "Technical Debate")
	require.Contains(t, view, "Code Review")

	// Should contain descriptions
	require.Contains(t, view, "Structured multi-perspective debate")

	// Should contain hints in header
	require.Contains(t, view, "↑/↓")
}

func TestCommandPalette_View_NoResults(t *testing.T) {
	m := New(Config{Items: testItems()}).SetSize(80, 24)

	m.textInput.SetValue("nonexistent")
	m = m.updateFilter()

	view := m.View()
	require.Contains(t, view, "No matching items")
}

func TestCommandPalette_View_Stability(t *testing.T) {
	m := New(Config{Items: testItems()}).SetSize(80, 24)

	view1 := m.View()
	view2 := m.View()

	require.Equal(t, view1, view2)
}

// Golden tests

func TestCommandPalette_View_Golden_Default(t *testing.T) {
	m := New(Config{
		Title:       "Select Workflow",
		Placeholder: "Search workflows...",
		Items:       testItems(),
	}).SetSize(80, 24)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestCommandPalette_View_Golden_Selected(t *testing.T) {
	m := New(Config{
		Title:       "Select Workflow",
		Placeholder: "Search workflows...",
		Items:       testItems(),
	}).SetSize(80, 24)

	// Move selection to second item
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestCommandPalette_View_Golden_Filtered(t *testing.T) {
	m := New(Config{
		Title:       "Select Workflow",
		Placeholder: "Search workflows...",
		Items:       testItems(),
	}).SetSize(80, 24)

	// Filter to "research"
	m.textInput.SetValue("research")
	m = m.updateFilter()

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestCommandPalette_View_Golden_NoResults(t *testing.T) {
	m := New(Config{
		Title:       "Select Workflow",
		Placeholder: "Search workflows...",
		Items:       testItems(),
	}).SetSize(80, 24)

	// Filter to non-existent
	m.textInput.SetValue("xyz")
	m = m.updateFilter()

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestCommandPalette_View_Golden_NoTitle(t *testing.T) {
	m := New(Config{
		Placeholder: "Search...",
		Items:       testItems(),
	}).SetSize(80, 24)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestCommandPalette_View_Golden_WithScrollIndicator(t *testing.T) {
	// More items than maxVisible (5) to trigger scroll indicator
	items := []Item{
		{ID: "1", Name: "Item One", Description: "First item"},
		{ID: "2", Name: "Item Two", Description: "Second item"},
		{ID: "3", Name: "Item Three", Description: "Third item"},
		{ID: "4", Name: "Item Four", Description: "Fourth item"},
		{ID: "5", Name: "Item Five", Description: "Fifth item"},
		{ID: "6", Name: "Item Six", Description: "Sixth item"},
		{ID: "7", Name: "Item Seven", Description: "Seventh item"},
	}
	m := New(Config{
		Title:       "Select Item",
		Placeholder: "Search...",
		Items:       items,
	}).SetSize(80, 24)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// Mouse scroll tests

func manyItems() []Item {
	return []Item{
		{ID: "1", Name: "Item One", Description: "First item"},
		{ID: "2", Name: "Item Two", Description: "Second item"},
		{ID: "3", Name: "Item Three", Description: "Third item"},
		{ID: "4", Name: "Item Four", Description: "Fourth item"},
		{ID: "5", Name: "Item Five", Description: "Fifth item"},
		{ID: "6", Name: "Item Six", Description: "Sixth item"},
		{ID: "7", Name: "Item Seven", Description: "Seventh item"},
	}
}

func TestCommandPalette_Update_MouseScrollDown(t *testing.T) {
	m := New(Config{Items: manyItems()}).SetSize(80, 24)

	// Initial scroll offset should be 0
	require.Equal(t, 0, m.scrollOffset)

	// Scroll down with mouse wheel
	m, _ = m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	require.Equal(t, 1, m.scrollOffset)

	// Scroll down again
	m, _ = m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	require.Equal(t, 2, m.scrollOffset)
}

func TestCommandPalette_Update_MouseScrollUp(t *testing.T) {
	m := New(Config{Items: manyItems()}).SetSize(80, 24)

	// Set initial scroll offset
	m.scrollOffset = 2

	// Scroll up with mouse wheel
	m, _ = m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp})
	require.Equal(t, 1, m.scrollOffset)

	// Scroll up again
	m, _ = m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp})
	require.Equal(t, 0, m.scrollOffset)
}

func TestCommandPalette_Update_MouseScrollBoundaryTop(t *testing.T) {
	m := New(Config{Items: manyItems()}).SetSize(80, 24)

	// Already at top (scrollOffset = 0)
	require.Equal(t, 0, m.scrollOffset)

	// Try to scroll up - should stay at 0
	m, _ = m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp})
	require.Equal(t, 0, m.scrollOffset)

	// Multiple attempts should still stay at 0
	m, _ = m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp})
	require.Equal(t, 0, m.scrollOffset)
}

func TestCommandPalette_Update_MouseScrollBoundaryBottom(t *testing.T) {
	m := New(Config{Items: manyItems()}).SetSize(80, 24)

	// 7 items, maxVisible = 5, so maxOffset = 2
	// Scroll to bottom
	m, _ = m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	m, _ = m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	require.Equal(t, 2, m.scrollOffset)

	// Try to scroll past bottom - should stay at maxOffset (2)
	m, _ = m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	require.Equal(t, 2, m.scrollOffset)

	// Multiple attempts should still stay at 2
	m, _ = m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	require.Equal(t, 2, m.scrollOffset)
}

func TestCommandPalette_Update_MouseIgnoresNonWheelEvents(t *testing.T) {
	m := New(Config{Items: manyItems()}).SetSize(80, 24)

	// Set initial state
	initialCursor := m.cursor
	initialOffset := m.scrollOffset

	// Left click should be ignored
	m, cmd := m.Update(tea.MouseMsg{Button: tea.MouseButtonLeft, X: 10, Y: 10})
	require.Nil(t, cmd)
	require.Equal(t, initialCursor, m.cursor)
	require.Equal(t, initialOffset, m.scrollOffset)

	// Right click should be ignored
	m, cmd = m.Update(tea.MouseMsg{Button: tea.MouseButtonRight, X: 10, Y: 10})
	require.Nil(t, cmd)
	require.Equal(t, initialCursor, m.cursor)
	require.Equal(t, initialOffset, m.scrollOffset)

	// Middle click should be ignored
	m, cmd = m.Update(tea.MouseMsg{Button: tea.MouseButtonMiddle, X: 10, Y: 10})
	require.Nil(t, cmd)
	require.Equal(t, initialCursor, m.cursor)
	require.Equal(t, initialOffset, m.scrollOffset)

	// Mouse motion should be ignored
	m, cmd = m.Update(tea.MouseMsg{Button: tea.MouseButtonNone, X: 20, Y: 20})
	require.Nil(t, cmd)
	require.Equal(t, initialCursor, m.cursor)
	require.Equal(t, initialOffset, m.scrollOffset)
}
