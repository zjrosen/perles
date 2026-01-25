package keys

import (
	"testing"

	"github.com/charmbracelet/bubbles/key"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Dashboard Keybinding Tests
// ============================================================================

func TestKanban_Dashboard_KeyAssignment(t *testing.T) {
	// Test that Dashboard is bound to ctrl+o
	keys := Kanban.Dashboard.Keys()
	require.Equal(t, []string{"ctrl+o"}, keys, "Dashboard should be bound to ctrl+o")
}

func TestKanban_Dashboard_HelpText(t *testing.T) {
	// Test that Dashboard has proper help text
	help := Kanban.Dashboard.Help()
	require.NotEmpty(t, help.Key, "Dashboard key help should not be empty")
	require.NotEmpty(t, help.Desc, "Dashboard description should not be empty")
	require.Equal(t, "ctrl+o", help.Key, "Dashboard help key should be ctrl+o")
	require.Equal(t, "dashboard", help.Desc, "Dashboard help desc should be 'dashboard'")
}

func TestDiffViewer_ExportedStruct(t *testing.T) {
	// Verify DiffViewer struct is exported and accessible
	require.NotNil(t, DiffViewer.Open)
	require.NotNil(t, DiffViewer.Close)
	require.NotNil(t, DiffViewer.NextFile)
	require.NotNil(t, DiffViewer.PrevFile)
	require.NotNil(t, DiffViewer.ScrollUp)
	require.NotNil(t, DiffViewer.ScrollDown)
	require.NotNil(t, DiffViewer.FocusLeft)
	require.NotNil(t, DiffViewer.FocusRight)
}

func TestDiffViewer_KeyAssignments(t *testing.T) {
	tests := []struct {
		name     string
		binding  key.Binding
		expected []string
	}{
		{
			name:     "Open uses ctrl+g (not ctrl+d)",
			binding:  DiffViewer.Open,
			expected: []string{"ctrl+g"},
		},
		{
			name:     "Close uses esc and q",
			binding:  DiffViewer.Close,
			expected: []string{"esc", "q"},
		},
		{
			name:     "NextFile uses j and down",
			binding:  DiffViewer.NextFile,
			expected: []string{"j", "down"},
		},
		{
			name:     "PrevFile uses k and up",
			binding:  DiffViewer.PrevFile,
			expected: []string{"k", "up"},
		},
		{
			name:     "ScrollUp uses ctrl+u and pgup",
			binding:  DiffViewer.ScrollUp,
			expected: []string{"ctrl+u", "pgup"},
		},
		{
			name:     "ScrollDown uses ctrl+d and pgdown",
			binding:  DiffViewer.ScrollDown,
			expected: []string{"ctrl+d", "pgdown"},
		},
		{
			name:     "FocusLeft uses h and left arrow",
			binding:  DiffViewer.FocusLeft,
			expected: []string{"h", "left"},
		},
		{
			name:     "FocusRight uses l and right arrow",
			binding:  DiffViewer.FocusRight,
			expected: []string{"l", "right"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys := tt.binding.Keys()
			require.Equal(t, tt.expected, keys)
		})
	}
}

func TestDiffViewer_HelpTextDefined(t *testing.T) {
	bindings := []struct {
		name    string
		binding key.Binding
	}{
		{"Open", DiffViewer.Open},
		{"Close", DiffViewer.Close},
		{"NextFile", DiffViewer.NextFile},
		{"PrevFile", DiffViewer.PrevFile},
		{"ScrollUp", DiffViewer.ScrollUp},
		{"ScrollDown", DiffViewer.ScrollDown},
		{"FocusLeft", DiffViewer.FocusLeft},
		{"FocusRight", DiffViewer.FocusRight},
	}

	for _, b := range bindings {
		t.Run(b.name, func(t *testing.T) {
			help := b.binding.Help()
			require.NotEmpty(t, help.Key, "key help should not be empty")
			require.NotEmpty(t, help.Desc, "description help should not be empty")
		})
	}
}

func TestDiffViewer_OpenNotCtrlD(t *testing.T) {
	// Explicit test: ctrl+g is used for Open, NOT ctrl+d (which conflicts with Delete)
	keys := DiffViewer.Open.Keys()
	require.Contains(t, keys, "ctrl+g", "Open must use ctrl+g")
	require.NotContains(t, keys, "ctrl+d", "Open must NOT use ctrl+d (conflicts with Delete)")
}

func TestDiffViewerShortHelp(t *testing.T) {
	help := DiffViewerShortHelp()
	require.NotEmpty(t, help, "short help should not be empty")
	require.Len(t, help, 3, "short help should contain 3 bindings")
	require.Equal(t, DiffViewer.Close, help[0])
	require.Equal(t, DiffViewer.NextFile, help[1])
	require.Equal(t, DiffViewer.PrevFile, help[2])
}

func TestDiffViewerFullHelp(t *testing.T) {
	help := DiffViewerFullHelp()
	require.NotEmpty(t, help, "full help should not be empty")
	require.Len(t, help, 3, "full help should contain 3 rows")

	// First row: navigation
	require.Contains(t, help[0], DiffViewer.NextFile)
	require.Contains(t, help[0], DiffViewer.PrevFile)
	require.Contains(t, help[0], DiffViewer.FocusLeft)
	require.Contains(t, help[0], DiffViewer.FocusRight)

	// Second row: scrolling
	require.Contains(t, help[1], DiffViewer.ScrollUp)
	require.Contains(t, help[1], DiffViewer.ScrollDown)

	// Third row: close
	require.Contains(t, help[2], DiffViewer.Close)
}

// ============================================================================
// App ChatFocus Keybinding Tests
// ============================================================================

func TestApp_ChatFocus_Keys(t *testing.T) {
	keys := App.ChatFocus.Keys()
	require.Equal(t, []string{"tab"}, keys, "ChatFocus should be bound to tab")
}

func TestApp_ChatFocus_HelpKey(t *testing.T) {
	help := App.ChatFocus.Help()
	require.Equal(t, "tab", help.Key, "ChatFocus help key should be 'tab'")
}

func TestApp_ChatFocus_HelpDesc(t *testing.T) {
	help := App.ChatFocus.Help()
	require.Equal(t, "switch chat/board focus", help.Desc, "ChatFocus help desc should be 'switch chat/board focus'")
}
