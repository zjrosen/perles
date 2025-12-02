package help

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHelp_New(t *testing.T) {
	m := New()

	// Verify model is created with keys populated
	assert.NotEmpty(t, m.keys.Up.Keys(), "expected Up keys to be set")
	assert.NotEmpty(t, m.keys.Down.Keys(), "expected Down keys to be set")
	assert.NotEmpty(t, m.keys.Left.Keys(), "expected Left keys to be set")
	assert.NotEmpty(t, m.keys.Right.Keys(), "expected Right keys to be set")
	assert.NotEmpty(t, m.keys.Help.Keys(), "expected Help keys to be set")
	assert.NotEmpty(t, m.keys.Quit.Keys(), "expected Quit keys to be set")
}

func TestHelp_SetSize(t *testing.T) {
	m := New()

	// Set dimensions
	m = m.SetSize(120, 40)

	assert.Equal(t, 120, m.width, "expected width to be 120")
	assert.Equal(t, 40, m.height, "expected height to be 40")

	// Verify SetSize returns new model (immutability)
	m2 := m.SetSize(80, 24)
	assert.Equal(t, 80, m2.width, "expected new model width to be 80")
	assert.Equal(t, 24, m2.height, "expected new model height to be 24")
	assert.Equal(t, 120, m.width, "expected original model width unchanged")
}

func TestHelp_View_ContainsSections(t *testing.T) {
	m := New().SetSize(80, 24)
	view := m.View()

	assert.Contains(t, view, "Navigation", "expected view to contain Navigation section")
	assert.Contains(t, view, "Actions", "expected view to contain Actions section")
	assert.Contains(t, view, "General", "expected view to contain General section")
}

func TestHelp_View_ContainsKeybindings(t *testing.T) {
	m := New().SetSize(80, 24)
	view := m.View()

	// Navigation keys (combined)
	assert.Contains(t, view, "h/l", "expected view to contain h/l keys")
	assert.Contains(t, view, "j/k", "expected view to contain j/k keys")
	assert.Contains(t, view, "left/right", "expected view to contain left/right description")
	assert.Contains(t, view, "up/down", "expected view to contain up/down description")

	// Action keys
	assert.Contains(t, view, "enter", "expected view to contain enter key")
	assert.Contains(t, view, "r", "expected view to contain refresh key")
	assert.Contains(t, view, "/", "expected view to contain search key")

	// General keys
	assert.Contains(t, view, "?", "expected view to contain help key")
	assert.Contains(t, view, "q", "expected view to contain quit key")
	assert.Contains(t, view, "esc", "expected view to contain escape key")
}

func TestHelp_View_ContainsFooter(t *testing.T) {
	m := New().SetSize(80, 24)
	view := m.View()

	assert.Contains(t, view, "Press ? or Esc to close", "expected view to contain footer")
}

func TestHelp_View_ContainsTitle(t *testing.T) {
	m := New().SetSize(80, 24)
	view := m.View()

	assert.Contains(t, view, "Keybindings", "expected view to contain title")
}

func TestHelp_Overlay(t *testing.T) {
	m := New().SetSize(80, 24)

	// Create a simple background
	background := strings.Repeat(strings.Repeat(".", 80)+"\n", 24)

	result := m.Overlay(background)

	// Should contain help content
	assert.Contains(t, result, "Navigation", "expected overlay to contain Navigation")
	assert.Contains(t, result, "Keybindings", "expected overlay to contain title")

	// Should still have some background visible (dots at edges)
	// The overlay is centered, so edges should have background content
	lines := strings.Split(result, "\n")
	require.NotEmpty(t, lines, "expected result to have lines")

	// First line should have background content (dots)
	assert.Contains(t, lines[0], ".", "expected first line to contain background")
}

func TestHelp_Overlay_EmptyBackground(t *testing.T) {
	m := New().SetSize(80, 24)

	// Empty background should render like View()
	result := m.Overlay("")
	view := m.View()

	// Both should contain the same help content
	assert.Contains(t, result, "Navigation")
	assert.Contains(t, view, "Navigation")
}

func TestHelp_View_VariousSizes(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		height int
	}{
		{"standard 80x24", 80, 24},
		{"large 120x40", 120, 40},
		{"narrow 60x20", 60, 20},
		{"wide 200x30", 200, 30},
		{"tall 80x50", 80, 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New().SetSize(tt.width, tt.height)
			view := m.View()

			// All sizes should render the core content
			assert.Contains(t, view, "Navigation", "expected Navigation section")
			assert.Contains(t, view, "Actions", "expected Actions section")
			assert.Contains(t, view, "General", "expected General section")
			assert.Contains(t, view, "Keybindings", "expected title")
			assert.Contains(t, view, "Press ? or Esc to close", "expected footer")
		})
	}
}

func TestHelp_Overlay_Centering(t *testing.T) {
	m := New().SetSize(80, 24)

	// Create background of known size
	bg := strings.Repeat(strings.Repeat(".", 80)+"\n", 24)
	bg = strings.TrimSuffix(bg, "\n")

	result := m.Overlay(bg)
	lines := strings.Split(result, "\n")

	// Help content should be centered in the overlay
	require.GreaterOrEqual(t, len(lines), 10, "expected at least 10 lines")

	// Help content should appear somewhere in the middle
	foundOverlay := false
	for _, line := range lines {
		if strings.Contains(line, "Keybindings") {
			foundOverlay = true
			break
		}
	}
	assert.True(t, foundOverlay, "expected to find overlay content in result")
}

func TestHelp_Overlay_BackgroundPreservation(t *testing.T) {
	m := New().SetSize(80, 24)

	// Create background
	bg := strings.Repeat(strings.Repeat(".", 80)+"\n", 24)
	bg = strings.TrimSuffix(bg, "\n")

	result := m.Overlay(bg)

	// Background dots should be preserved around the help content
	dotCount := strings.Count(result, ".")
	// Should have some dots preserved (not all replaced by help content)
	assert.Greater(t, dotCount, 100, "expected background dots to be preserved around help")
}

func TestHelp_renderBinding(t *testing.T) {
	m := New()

	// Test rendering a binding
	output := m.renderBinding(m.keys.Quit)

	assert.Contains(t, output, "q", "expected binding to contain key")
	assert.Contains(t, output, "quit", "expected binding to contain description")
}

// teatest integration - verify View() produces valid output
func TestHelp_View_Stability(t *testing.T) {
	m := New().SetSize(80, 24)
	view1 := m.View()
	view2 := m.View()

	// Same model should produce identical output
	assert.Equal(t, view1, view2, "expected stable output from same model")

	// Output should be non-empty and contain expected content
	assert.NotEmpty(t, view1, "expected non-empty view")
	assert.Greater(t, len(view1), 100, "expected substantial output")
}

// Search mode tests
func TestHelp_NewSearch(t *testing.T) {
	m := NewSearch()

	// Verify model is created with search keys populated
	assert.NotEmpty(t, m.searchKeys.Up.Keys(), "expected Up keys to be set")
	assert.NotEmpty(t, m.searchKeys.Down.Keys(), "expected Down keys to be set")
	assert.NotEmpty(t, m.searchKeys.Left.Keys(), "expected Left keys to be set")
	assert.NotEmpty(t, m.searchKeys.Right.Keys(), "expected Right keys to be set")
	assert.NotEmpty(t, m.searchKeys.Help.Keys(), "expected Help keys to be set")
	assert.NotEmpty(t, m.searchKeys.Quit.Keys(), "expected Quit keys to be set")
	assert.Equal(t, ModeSearch, m.mode, "expected mode to be ModeSearch")
}

func TestHelp_SearchView_ContainsSections(t *testing.T) {
	m := NewSearch().SetSize(100, 40)
	view := m.View()

	assert.Contains(t, view, "Navigation", "expected view to contain Navigation section")
	assert.Contains(t, view, "Actions", "expected view to contain Actions section")
	assert.Contains(t, view, "General", "expected view to contain General section")
}

func TestHelp_SearchView_ContainsBQLFields(t *testing.T) {
	m := NewSearch().SetSize(100, 40)
	view := m.View()

	// BQL Fields section
	assert.Contains(t, view, "BQL Fields", "expected view to contain BQL Fields section")
	assert.Contains(t, view, "status", "expected view to contain status field")
	assert.Contains(t, view, "type", "expected view to contain type field")
	assert.Contains(t, view, "priority", "expected view to contain priority field")
	assert.Contains(t, view, "blocked", "expected view to contain blocked field")
	assert.Contains(t, view, "ready", "expected view to contain ready field")
	assert.Contains(t, view, "label", "expected view to contain label field")
	assert.Contains(t, view, "title", "expected view to contain title field")
	assert.Contains(t, view, "created", "expected view to contain created field")
}

func TestHelp_SearchView_ContainsBQLOperators(t *testing.T) {
	m := NewSearch().SetSize(100, 40)
	view := m.View()

	// BQL Operators section
	assert.Contains(t, view, "BQL Operators", "expected view to contain BQL Operators section")
	assert.Contains(t, view, "equality", "expected view to contain equality operator")
	assert.Contains(t, view, "comparison", "expected view to contain comparison operator")
	assert.Contains(t, view, "contains", "expected view to contain contains operator")
	assert.Contains(t, view, "logical", "expected view to contain logical operators")
}

func TestHelp_SearchView_ContainsKeybindings(t *testing.T) {
	m := NewSearch().SetSize(100, 40)
	view := m.View()

	// Navigation - h/l for pane focus, / and esc for search input
	assert.Contains(t, view, "focus results", "expected h for focus results")
	assert.Contains(t, view, "focus details", "expected l for focus details")
	assert.Contains(t, view, "/", "expected / for focus search")
	assert.Contains(t, view, "esc", "expected esc for blur")

	// Actions
	assert.Contains(t, view, "y", "expected y for copy issue ID")
	assert.Contains(t, view, "ctrl+s", "expected ctrl+s for save as column")
}

func TestHelp_SearchView_ContainsExamples(t *testing.T) {
	m := NewSearch().SetSize(100, 40)
	view := m.View()

	// Examples section
	assert.Contains(t, view, "Examples", "expected view to contain Examples section")
	assert.Contains(t, view, "status = open", "expected view to contain status example")
	assert.Contains(t, view, "type in (bug, task)", "expected view to contain type example")
	assert.Contains(t, view, "title ~", "expected view to contain title example")
	assert.Contains(t, view, "created >", "expected view to contain created example")
}

func TestHelp_SearchView_ContainsTitle(t *testing.T) {
	m := NewSearch().SetSize(100, 40)
	view := m.View()

	assert.Contains(t, view, "Search Mode Help", "expected view to contain title")
}

func TestHelp_SearchView_ContainsFooter(t *testing.T) {
	m := NewSearch().SetSize(100, 40)
	view := m.View()

	assert.Contains(t, view, "Press ? or Esc to close", "expected view to contain footer")
}

// TestHelp_View_Golden uses teatest golden file comparison
// Run with -update flag to update golden files: go test -update ./internal/ui/help/...
func TestHelp_View_Golden(t *testing.T) {
	m := New().SetSize(80, 24)
	view := m.View()

	// teatest's RequireEqualOutput compares against golden files in testdata/
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestHelp_SearchView_Golden uses teatest golden file comparison for search mode
func TestHelp_SearchView_Golden(t *testing.T) {
	m := NewSearch().SetSize(100, 40)
	view := m.View()

	teatest.RequireEqualOutput(t, []byte(view))
}
