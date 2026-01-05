package diffviewer

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/require"
)

func TestHelp_newHelp(t *testing.T) {
	h := newHelp()

	// Default context should be FileList
	require.Equal(t, helpContextFileList, h.context, "expected default context to be FileList")
}

func TestHelp_SetContext(t *testing.T) {
	tests := []struct {
		name    string
		context helpContext
	}{
		{"FileList", helpContextFileList},
		{"Commits", helpContextCommits},
		{"CommitFiles", helpContextCommitFiles},
		{"Diff", helpContextDiff},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHelp().SetContext(tt.context)
			require.Equal(t, tt.context, h.context, "expected context to be set")
		})
	}
}

func TestHelp_SetSize(t *testing.T) {
	h := newHelp().SetSize(120, 40)

	require.Equal(t, 120, h.width, "expected width to be 120")
	require.Equal(t, 40, h.height, "expected height to be 40")
}

func TestHelp_View_ContainsTitle(t *testing.T) {
	h := newHelp().SetSize(100, 40)
	view := h.View()

	require.Contains(t, view, "Diff Viewer Help", "expected view to contain title")
}

func TestHelp_View_ContainsSections(t *testing.T) {
	h := newHelp().SetSize(100, 40)
	view := h.View()

	require.Contains(t, view, "Navigation", "expected view to contain Navigation section")
	require.Contains(t, view, "Scrolling & View", "expected view to contain Scrolling & View section")
	require.Contains(t, view, "Actions", "expected view to contain Actions section")
	require.Contains(t, view, "General", "expected view to contain General section")
}

func TestHelp_View_ContainsNavigationKeys(t *testing.T) {
	h := newHelp().SetSize(100, 40)
	view := h.View()

	require.Contains(t, view, "j/↓", "expected view to contain j/down key")
	require.Contains(t, view, "k/↑", "expected view to contain k/up key")
	require.Contains(t, view, "h/←", "expected view to contain h/left key")
	require.Contains(t, view, "l/→", "expected view to contain l/right key")
	require.Contains(t, view, "tab", "expected view to contain tab key")
}

func TestHelp_View_ContainsScrollingKeys(t *testing.T) {
	h := newHelp().SetSize(100, 40)
	view := h.View()

	require.Contains(t, view, "ctrl+u", "expected view to contain ctrl+u key")
	require.Contains(t, view, "ctrl+d", "expected view to contain ctrl+d key")
	require.Contains(t, view, "go to top", "expected view to contain go to top hint")
	require.Contains(t, view, "go to bottom", "expected view to contain go to bottom hint")
}

func TestHelp_View_ContainsContextDependentKeybinding(t *testing.T) {
	h := newHelp().SetSize(100, 40)
	view := h.View()

	// The [ / ] keys have context-dependent behavior:
	// - Navigate tabs when commits pane is focused
	// - Navigate hunks when diff pane is focused
	require.Contains(t, view, "[ / ]", "expected view to contain context-dependent keybinding for bracket keys")
	require.Contains(t, view, "tabs/hunks", "expected view to contain context-dependent description for bracket keys")
}

func TestHelp_View_ContainsGeneralKeys(t *testing.T) {
	h := newHelp().SetSize(100, 40)
	view := h.View()

	require.Contains(t, view, "?", "expected view to contain help key")
	require.Contains(t, view, "esc", "expected view to contain escape key")
}

func TestHelp_View_ContainsFooter(t *testing.T) {
	h := newHelp().SetSize(100, 40)
	view := h.View()

	require.Contains(t, view, "Press ? or Esc to close", "expected view to contain footer")
}

func TestHelp_View_ContextSpecificActions_FileList(t *testing.T) {
	h := newHelp().SetContext(helpContextFileList).SetSize(100, 40)
	view := h.View()

	require.Contains(t, view, "toggle/view file", "expected FileList actions")
}

func TestHelp_View_ContextSpecificActions_Commits(t *testing.T) {
	h := newHelp().SetContext(helpContextCommits).SetSize(100, 40)
	view := h.View()

	require.Contains(t, view, "view commit files", "expected Commits actions")
}

func TestHelp_View_ContextSpecificActions_CommitFiles(t *testing.T) {
	h := newHelp().SetContext(helpContextCommitFiles).SetSize(100, 40)
	view := h.View()

	require.Contains(t, view, "toggle/view file", "expected CommitFiles toggle action")
	require.Contains(t, view, "back to commits", "expected CommitFiles back action")
}

func TestHelp_View_ContextSpecificActions_Diff(t *testing.T) {
	h := newHelp().SetContext(helpContextDiff).SetSize(100, 40)
	view := h.View()

	require.Contains(t, view, "back to list", "expected Diff back action")
}

func TestHelp_Overlay(t *testing.T) {
	h := newHelp().SetSize(100, 40)

	// Create a simple background
	background := strings.Repeat(strings.Repeat(".", 100)+"\n", 40)

	result := h.Overlay(background)

	// Should contain help content
	require.Contains(t, result, "Navigation", "expected overlay to contain Navigation")
	require.Contains(t, result, "Diff Viewer Help", "expected overlay to contain title")

	// Should still have some background visible
	lines := strings.Split(result, "\n")
	require.NotEmpty(t, lines, "expected result to have lines")

	// First line should have background content
	require.Contains(t, lines[0], ".", "expected first line to contain background")
}

func TestHelp_Overlay_EmptyBackground(t *testing.T) {
	h := newHelp().SetSize(100, 40)

	result := h.Overlay("")
	view := h.View()

	// Both should contain the same help content
	require.Contains(t, result, "Navigation")
	require.Contains(t, view, "Navigation")
}

func TestHelp_View_Stability(t *testing.T) {
	h := newHelp().SetSize(100, 40)
	view1 := h.View()
	view2 := h.View()

	// Same model should produce identical output
	require.Equal(t, view1, view2, "expected stable output from same model")

	// Output should be non-empty
	require.NotEmpty(t, view1, "expected non-empty view")
	require.Greater(t, len(view1), 100, "expected substantial output")
}

func TestHelp_View_VariousSizes(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		height int
	}{
		{"standard 80x24", 80, 24},
		{"large 120x40", 120, 40},
		{"wide 200x30", 200, 30},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHelp().SetSize(tt.width, tt.height)
			view := h.View()

			// All sizes should render core content
			require.Contains(t, view, "Navigation", "expected Navigation section")
			require.Contains(t, view, "Scrolling & View", "expected Scrolling & View section")
			require.Contains(t, view, "Actions", "expected Actions section")
			require.Contains(t, view, "General", "expected General section")
		})
	}
}

// Golden test for help view with FileList context
func TestHelp_View_Golden_FileList(t *testing.T) {
	h := newHelp().SetContext(helpContextFileList).SetSize(100, 40)
	view := h.View()

	teatest.RequireEqualOutput(t, []byte(view))
}

// Golden test for help view with Commits context
func TestHelp_View_Golden_Commits(t *testing.T) {
	h := newHelp().SetContext(helpContextCommits).SetSize(100, 40)
	view := h.View()

	teatest.RequireEqualOutput(t, []byte(view))
}

// Golden test for help view with CommitFiles context
func TestHelp_View_Golden_CommitFiles(t *testing.T) {
	h := newHelp().SetContext(helpContextCommitFiles).SetSize(100, 40)
	view := h.View()

	teatest.RequireEqualOutput(t, []byte(view))
}

// Golden test for help view with Diff context
func TestHelp_View_Golden_Diff(t *testing.T) {
	h := newHelp().SetContext(helpContextDiff).SetSize(100, 40)
	view := h.View()

	teatest.RequireEqualOutput(t, []byte(view))
}
