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

// ============================================================================
// Translation Function Tests
// ============================================================================

func TestTranslateToTerminal_CtrlSpace(t *testing.T) {
	result := translateToTerminal("ctrl+space")
	require.Equal(t, "ctrl+@", result, "ctrl+space should translate to ctrl+@")
}

func TestTranslateToTerminal_CtrlSpaceVariant(t *testing.T) {
	result := translateToTerminal("ctrl+ ")
	require.Equal(t, "ctrl+@", result, "ctrl+ (space) should translate to ctrl+@")
}

func TestTranslateToTerminal_Passthrough(t *testing.T) {
	result := translateToTerminal("ctrl+o")
	require.Equal(t, "ctrl+o", result, "ctrl+o should pass through unchanged")
}

func TestTranslateToTerminal_CaseNormalization(t *testing.T) {
	result := translateToTerminal("Ctrl+Space")
	require.Equal(t, "ctrl+@", result, "Ctrl+Space should normalize to ctrl+@")
}

func TestTranslateToTerminal_WhitespaceTrim(t *testing.T) {
	result := translateToTerminal(" ctrl+o ")
	require.Equal(t, "ctrl+o", result, "leading/trailing whitespace should be trimmed")
}

func TestTranslateToDisplay_CtrlAt(t *testing.T) {
	result := translateToDisplay("ctrl+@")
	require.Equal(t, "ctrl+space", result, "ctrl+@ should display as ctrl+space")
}

func TestTranslateToDisplay_Passthrough(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"f1", "f1"},
		{"alt+s", "alt+s"},
		{"enter", "enter"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := translateToDisplay(tt.input)
			require.Equal(t, tt.expected, result, "%s should pass through unchanged", tt.input)
		})
	}
}

// ============================================================================
// ApplyConfig Tests
// ============================================================================

func TestApplyConfig_ModifiesSearchBindings(t *testing.T) {
	// Reset state before test
	ResetForTesting()
	defer ResetForTesting()

	// Apply config with custom search key
	ApplyConfig("ctrl+s", "")

	// Verify Kanban.SwitchMode updated
	kanbanKeys := Kanban.SwitchMode.Keys()
	require.Equal(t, []string{"ctrl+s"}, kanbanKeys, "Kanban.SwitchMode should be bound to ctrl+s")

	// Verify Search.SwitchMode updated
	searchKeys := Search.SwitchMode.Keys()
	require.Equal(t, []string{"ctrl+s"}, searchKeys, "Search.SwitchMode should be bound to ctrl+s")
}

func TestApplyConfig_ModifiesDashboardBindings(t *testing.T) {
	// Reset state before test
	ResetForTesting()
	defer ResetForTesting()

	// Apply config with custom dashboard key
	ApplyConfig("", "ctrl+d")

	// Verify Kanban.Dashboard updated
	dashboardKeys := Kanban.Dashboard.Keys()
	require.Equal(t, []string{"ctrl+d"}, dashboardKeys, "Kanban.Dashboard should be bound to ctrl+d")
}

func TestApplyConfig_SetsHelpText(t *testing.T) {
	// Reset state before test
	ResetForTesting()
	defer ResetForTesting()

	// Apply config with ctrl+space (should translate display properly)
	ApplyConfig("ctrl+space", "ctrl+o")

	// Verify Kanban.SwitchMode help text
	kanbanHelp := Kanban.SwitchMode.Help()
	require.Equal(t, "ctrl+space", kanbanHelp.Key, "Kanban.SwitchMode help key should show ctrl+space")
	require.Equal(t, "search mode", kanbanHelp.Desc, "Kanban.SwitchMode help desc should be 'search mode'")

	// Verify Search.SwitchMode help text
	searchHelp := Search.SwitchMode.Help()
	require.Equal(t, "ctrl+space", searchHelp.Key, "Search.SwitchMode help key should show ctrl+space")
	require.Equal(t, "switch mode", searchHelp.Desc, "Search.SwitchMode help desc should be 'switch mode'")

	// Verify Dashboard help text uses ^O shorthand
	dashboardHelp := Kanban.Dashboard.Help()
	require.Equal(t, "ctrl+o", dashboardHelp.Key, "Kanban.Dashboard help key should show ctrl+o")
	require.Equal(t, "dashboard", dashboardHelp.Desc, "Kanban.Dashboard help desc should be 'dashboard'")
}

func TestApplyConfig_EmptyString_NoChange(t *testing.T) {
	// Reset state before test
	ResetForTesting()
	defer ResetForTesting()

	// Capture defaults
	originalKanbanSwitchKeys := Kanban.SwitchMode.Keys()
	originalSearchSwitchKeys := Search.SwitchMode.Keys()
	originalDashboardKeys := Kanban.Dashboard.Keys()

	// Apply config with empty strings (should not modify)
	ApplyConfig("", "")

	// Verify bindings unchanged
	require.Equal(t, originalKanbanSwitchKeys, Kanban.SwitchMode.Keys(), "Kanban.SwitchMode should be unchanged")
	require.Equal(t, originalSearchSwitchKeys, Search.SwitchMode.Keys(), "Search.SwitchMode should be unchanged")
	require.Equal(t, originalDashboardKeys, Kanban.Dashboard.Keys(), "Kanban.Dashboard should be unchanged")
}

func TestResetForTesting_RestoresDefaults(t *testing.T) {
	// First modify state
	ResetForTesting()
	ApplyConfig("ctrl+x", "ctrl+y")

	// Verify modified
	require.Equal(t, []string{"ctrl+x"}, Kanban.SwitchMode.Keys())
	require.Equal(t, []string{"ctrl+y"}, Kanban.Dashboard.Keys())

	// Reset
	ResetForTesting()

	// Verify defaults restored
	require.Equal(t, []string{"ctrl+@"}, Kanban.SwitchMode.Keys(), "Kanban.SwitchMode should be restored to ctrl+@")
	require.Equal(t, []string{"ctrl+@"}, Search.SwitchMode.Keys(), "Search.SwitchMode should be restored to ctrl+@")
	require.Equal(t, []string{"ctrl+o"}, Kanban.Dashboard.Keys(), "Kanban.Dashboard should be restored to ctrl+o")

	// Verify help text restored
	kanbanHelp := Kanban.SwitchMode.Help()
	require.Equal(t, "^space", kanbanHelp.Key, "Kanban.SwitchMode help key should be restored to ^space")

	searchHelp := Search.SwitchMode.Help()
	require.Equal(t, "ctrl+space", searchHelp.Key, "Search.SwitchMode help key should be restored to ctrl+space")

	dashboardHelp := Kanban.Dashboard.Help()
	require.Equal(t, "ctrl+o", dashboardHelp.Key, "Kanban.Dashboard help key should be restored to ctrl+o")

}
