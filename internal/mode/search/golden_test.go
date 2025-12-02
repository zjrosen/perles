package search

import (
	"errors"
	"testing"

	"github.com/charmbracelet/x/exp/teatest"

	"perles/internal/beads"
)

// Golden tests for search mode rendering.
// Run with -update flag to update golden files: go test -update ./internal/mode/search/...

func TestSearch_View_Golden_Empty(t *testing.T) {
	m := createTestModel()
	m = m.SetSize(100, 30)
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestSearch_View_Golden_WithResults(t *testing.T) {
	m := createTestModel()
	m = m.SetSize(100, 30)

	// Load some results
	issues := []beads.Issue{
		{ID: "bd-a1b", TitleText: "Implement webhook system", Priority: 1, Status: beads.StatusOpen, Type: beads.TypeFeature},
		{ID: "bd-c2d", TitleText: "Fix crash on startup", Priority: 0, Status: beads.StatusInProgress, Type: beads.TypeBug},
		{ID: "bd-e3f", TitleText: "Add unit tests", Priority: 2, Status: beads.StatusOpen, Type: beads.TypeTask},
	}
	m, _ = m.handleSearchResults(searchResultsMsg{issues: issues, err: nil})

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestSearch_View_Golden_Error(t *testing.T) {
	m := createTestModel()
	m = m.SetSize(100, 30)
	m.input.SetValue("invalid query syntax ===")

	// Simulate error result
	m, _ = m.handleSearchResults(searchResultsMsg{issues: nil, err: errors.New("syntax error: unexpected token")})
	// Set showSearchErr to true (simulates blur from search input)
	m.showSearchErr = true

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestSearch_View_Golden_NoResults(t *testing.T) {
	m := createTestModel()
	m = m.SetSize(100, 30)
	m.input.SetValue("status = closed")

	// Simulate empty result
	m, _ = m.handleSearchResults(searchResultsMsg{issues: []beads.Issue{}, err: nil})

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestSearch_View_Golden_Wide(t *testing.T) {
	m := createTestModel()
	m = m.SetSize(200, 40)

	// Load some results
	issues := []beads.Issue{
		{ID: "bd-a1b", TitleText: "Implement webhook system", Priority: 1, Status: beads.StatusOpen, Type: beads.TypeFeature},
		{ID: "bd-c2d", TitleText: "Fix crash on startup", Priority: 0, Status: beads.StatusInProgress, Type: beads.TypeBug},
	}
	m, _ = m.handleSearchResults(searchResultsMsg{issues: issues, err: nil})

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestSearch_View_Golden_Narrow(t *testing.T) {
	m := createTestModel()
	m = m.SetSize(80, 24)

	// Load some results
	issues := []beads.Issue{
		{ID: "bd-a1b", TitleText: "Implement webhook system", Priority: 1, Status: beads.StatusOpen, Type: beads.TypeFeature},
	}
	m, _ = m.handleSearchResults(searchResultsMsg{issues: issues, err: nil})

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}
