package search

import (
	"errors"
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/mock"

	"perles/internal/beads"
	"perles/internal/config"
	"perles/internal/mocks"
	"perles/internal/mode"
	"perles/internal/ui/shared/formmodal"
)

// testNow is a fixed reference time for golden tests to ensure reproducible timestamps.
var testNow = time.Date(2025, 12, 13, 12, 0, 0, 0, time.UTC)

// createGoldenTestModel creates a Model with a mock clock for reproducible golden tests.
func createGoldenTestModel(t *testing.T) Model {
	cfg := config.Defaults()
	clipboard := mocks.NewMockClipboard(t)
	clipboard.EXPECT().Copy(mock.Anything).Return(nil).Maybe()
	clock := mocks.NewMockClock(t)
	clock.EXPECT().Now().Return(testNow).Maybe()

	mockClient := mocks.NewMockBeadsClient(t)
	mockClient.EXPECT().GetComments(mock.Anything).Return([]beads.Comment{}, nil).Maybe()

	services := mode.Services{
		Client:    mockClient,
		Config:    &cfg,
		Clipboard: clipboard,
		Clock:     clock,
	}

	m := New(services)
	m.width = 100
	m.height = 40
	return m
}

// createGoldenTestModelWithViews creates a Model with views and a mock clock.
func createGoldenTestModelWithViews(t *testing.T) Model {
	cfg := config.Defaults()
	cfg.Views = []config.ViewConfig{
		{Name: "Inbox"},
		{Name: "Critical"},
		{Name: "In Progress"},
	}
	clipboard := mocks.NewMockClipboard(t)
	clipboard.EXPECT().Copy(mock.Anything).Return(nil).Maybe()
	clock := mocks.NewMockClock(t)
	clock.EXPECT().Now().Return(testNow).Maybe()
	services := mode.Services{
		Config:    &cfg,
		Clipboard: clipboard,
		Clock:     clock,
	}

	m := New(services)
	m.width = 100
	m.height = 40
	return m
}

// Golden tests for search mode rendering.
// Run with -update flag to update golden files: go test -update ./internal/mode/search/...

func TestSearch_View_Golden_Empty(t *testing.T) {
	m := createGoldenTestModel(t)
	m = m.SetSize(100, 30)
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestSearch_View_Golden_WithResults(t *testing.T) {
	m := createGoldenTestModel(t)
	m = m.SetSize(100, 30)

	// Load some results with timestamps and comments
	issues := []beads.Issue{
		{
			ID: "bd-a1b", TitleText: "Implement webhook system", Priority: 1, Status: beads.StatusOpen, Type: beads.TypeFeature,
			CreatedAt:    testNow.Add(-10 * time.Hour), // 10h ago
			CommentCount: 3,                            // 3 comments
		},
		{
			ID: "bd-c2d", TitleText: "Fix crash on startup", Priority: 0, Status: beads.StatusInProgress, Type: beads.TypeBug,
			CreatedAt: testNow.Add(-3 * 24 * time.Hour), // 3d ago
		},
		{
			ID: "bd-e3f", TitleText: "Add unit tests", Priority: 2, Status: beads.StatusOpen, Type: beads.TypeTask,
			CreatedAt:    testNow.Add(-2 * 7 * 24 * time.Hour), // 2w ago
			CommentCount: 1,                                    // 1 comment
		},
	}
	m, _ = m.handleSearchResults(searchResultsMsg{issues: issues, err: nil})

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestSearch_View_Golden_Error(t *testing.T) {
	m := createGoldenTestModel(t)
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
	m := createGoldenTestModel(t)
	m = m.SetSize(100, 30)
	m.input.SetValue("status = closed")

	// Simulate empty result
	m, _ = m.handleSearchResults(searchResultsMsg{issues: []beads.Issue{}, err: nil})

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestSearch_View_Golden_Wide(t *testing.T) {
	m := createGoldenTestModel(t)
	m = m.SetSize(200, 40)

	// Load some results with timestamps
	issues := []beads.Issue{
		{
			ID: "bd-a1b", TitleText: "Implement webhook system", Priority: 1, Status: beads.StatusOpen, Type: beads.TypeFeature,
			CreatedAt: testNow.Add(-5 * time.Minute), // 5m ago
		},
		{
			ID: "bd-c2d", TitleText: "Fix crash on startup", Priority: 0, Status: beads.StatusInProgress, Type: beads.TypeBug,
			CreatedAt:    testNow.Add(-6 * 30 * 24 * time.Hour), // 6mo ago
			CommentCount: 2,                                     // 2 comments
		},
	}
	m, _ = m.handleSearchResults(searchResultsMsg{issues: issues, err: nil})

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestSearch_View_Golden_Narrow(t *testing.T) {
	m := createGoldenTestModel(t)
	m = m.SetSize(80, 24)

	// Load some results with timestamp (narrow width should truncate title)
	issues := []beads.Issue{
		{
			ID: "bd-a1b", TitleText: "Implement webhook system", Priority: 1, Status: beads.StatusOpen, Type: beads.TypeFeature,
			CreatedAt: testNow.Add(-1 * time.Hour), // 1h ago
		},
	}
	m, _ = m.handleSearchResults(searchResultsMsg{issues: issues, err: nil})

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestSearch_View_Golden_NewViewModal(t *testing.T) {
	m := createGoldenTestModelWithViews(t)
	m = m.SetSize(100, 30)
	m.input.SetValue("status = open")

	// Open new view modal
	m.newViewModal = formmodal.New(makeNewViewFormConfig(m.services.Config.Views, m.input.Value())).
		SetSize(m.width, m.height)
	m.view = ViewNewView

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestSearch_View_Golden_SaveColumnModal(t *testing.T) {
	m := createGoldenTestModelWithViews(t)
	m = m.SetSize(100, 30)
	m.input.SetValue("priority = 0")

	// Open save column modal
	m.viewSelector = formmodal.New(makeUpdateViewFormConfig(m.services.Config.Views, m.input.Value())).
		SetSize(m.width, m.height)
	m.view = ViewSaveColumn

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}
