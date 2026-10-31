package outdated

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/require"
)

// testRequiredVersion is a representative minimum version string used in tests.
// This avoids importing beads domain types into the backend-agnostic outdated UI.
const testRequiredVersion = "1.0.0"

func TestOutdated_New(t *testing.T) {
	m := New("0.29.0", testRequiredVersion)

	// Verify model is created with versions but zero dimensions
	require.Equal(t, 0, m.width, "expected width to be 0")
	require.Equal(t, 0, m.height, "expected height to be 0")
	require.Equal(t, "0.29.0", m.currentVersion, "expected current version")
	require.Equal(t, testRequiredVersion, m.requiredVersion, "expected required version")
}

func TestOutdated_Init(t *testing.T) {
	m := New("0.29.0", testRequiredVersion)

	// Init should return nil (no initial command)
	cmd := m.Init()
	require.Nil(t, cmd, "expected Init to return nil")
}

func TestOutdated_SetSize(t *testing.T) {
	m := New("0.29.0", testRequiredVersion)

	// Set dimensions
	m = m.SetSize(120, 40)

	require.Equal(t, 120, m.width, "expected width to be 120")
	require.Equal(t, 40, m.height, "expected height to be 40")

	// Verify SetSize returns new model (immutability)
	m2 := m.SetSize(80, 24)
	require.Equal(t, 80, m2.width, "expected new model width to be 80")
	require.Equal(t, 24, m2.height, "expected new model height to be 24")
	require.Equal(t, 120, m.width, "expected original model width unchanged")
}

func TestOutdated_WindowSizeMsg(t *testing.T) {
	m := New("0.29.0", testRequiredVersion)

	// Send WindowSizeMsg
	msg := tea.WindowSizeMsg{Width: 80, Height: 24}
	newModel, cmd := m.Update(msg)

	// Cast back to Model to check fields
	updated := newModel.(Model)
	require.Equal(t, 80, updated.width, "expected width to be updated")
	require.Equal(t, 24, updated.height, "expected height to be updated")
	require.Nil(t, cmd, "expected no command from WindowSizeMsg")
}

func TestOutdated_QuitKeys(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyMsg
	}{
		{"q key", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}},
		{"ctrl+c", tea.KeyMsg{Type: tea.KeyCtrlC}},
		{"esc", tea.KeyMsg{Type: tea.KeyEsc}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New("0.29.0", testRequiredVersion).SetSize(80, 24)
			_, cmd := m.Update(tt.key)

			// Should return tea.Quit command
			require.NotNil(t, cmd, "expected quit command")

			// Execute the command and verify it's a quit message
			msg := cmd()
			_, isQuit := msg.(tea.QuitMsg)
			require.True(t, isQuit, "expected tea.QuitMsg")
		})
	}
}

func TestOutdated_OtherKeyMsg(t *testing.T) {
	m := New("0.29.0", testRequiredVersion).SetSize(80, 24)

	// Send a random key
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}
	_, cmd := m.Update(msg)

	// Should not return any command
	require.Nil(t, cmd, "expected no command from other keys")
}

func TestOutdated_EmptyDimensions(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		height int
	}{
		{"zero width", 0, 24},
		{"zero height", 80, 0},
		{"both zero", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New("0.29.0", testRequiredVersion).SetSize(tt.width, tt.height)
			view := m.View()

			require.Equal(t, "", view, "expected empty string for zero dimensions")
		})
	}
}

func TestOutdated_View_ContainsTitle(t *testing.T) {
	m := New("0.29.0", testRequiredVersion).SetSize(80, 24)
	view := m.View()

	require.Contains(t, view, "break in the chain", "expected view to contain title")
}

func TestOutdated_View_ContainsVersions(t *testing.T) {
	m := New("0.29.0", testRequiredVersion).SetSize(80, 24)
	view := m.View()

	require.Contains(t, view, "0.29.0", "expected view to contain current version")
	require.Contains(t, view, testRequiredVersion, "expected view to contain required version")
}

func TestOutdated_View_ContainsUpgradeInstructions(t *testing.T) {
	m := New("0.29.0", testRequiredVersion).SetSize(80, 24)
	view := m.View()

	require.Contains(t, view, "bd migrate schema", "expected view to contain the schema migration")
	require.Contains(t, view, "Upgrade bd", "expected view to ask for a bd upgrade")
	require.Contains(t, view, "docs/getting-started/upgrading.md", "expected view to point at the cross-era guide")
}

func TestOutdated_View_ContainsHint(t *testing.T) {
	m := New("0.29.0", testRequiredVersion).SetSize(80, 24)
	view := m.View()

	require.Contains(t, view, "Press q to quit", "expected view to contain quit hint")
}

func TestOutdated_View_ContainsChainArt(t *testing.T) {
	m := New("0.29.0", testRequiredVersion).SetSize(120, 40)
	view := m.View()

	// Chain links use box-drawing characters
	require.Contains(t, view, "╔═══════╗", "expected view to contain chain link top")
	require.Contains(t, view, "╚═══════╝", "expected view to contain chain link bottom")
}

func TestOutdated_View_Stability(t *testing.T) {
	m := New("0.29.0", testRequiredVersion).SetSize(80, 24)
	view1 := m.View()
	view2 := m.View()

	// Same model should produce identical output
	require.Equal(t, view1, view2, "expected stable output from same model")

	// Output should be non-empty and contain expected content
	require.NotEmpty(t, view1, "expected non-empty view")
	require.Greater(t, len(view1), 100, "expected substantial output")
}

func TestOutdated_View_VariousSizes(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		height int
	}{
		{"standard 80x24", 80, 24},
		{"large 120x40", 120, 40},
		{"wide 200x30", 200, 30},
		{"tall 80x50", 80, 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New("0.29.0", testRequiredVersion).SetSize(tt.width, tt.height)
			view := m.View()

			// All sizes should render the core content
			require.Contains(t, view, "break in the chain", "expected title")
			require.Contains(t, view, "0.29.0", "expected current version")
			require.Contains(t, view, "Press q to quit", "expected quit hint")
		})
	}
}

// TestOutdated_View_Golden_Standard uses teatest golden file comparison for 80x24
// Run with -update flag to update golden files: go test -update ./internal/ui/outdated/...
func TestOutdated_View_Golden_Standard(t *testing.T) {
	m := New("0.29.0", testRequiredVersion).SetSize(80, 24)
	view := m.View()

	// teatest's RequireEqualOutput compares against golden files in testdata/
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestOutdated_View_Golden_Large uses teatest golden file comparison for 120x40
func TestOutdated_View_Golden_Large(t *testing.T) {
	m := New("0.29.0", testRequiredVersion).SetSize(120, 40)
	view := m.View()

	teatest.RequireEqualOutput(t, []byte(view))
}
