package playground

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/ui/styles"
)

// createGoldenTestModel creates a Model for reproducible golden tests.
func createGoldenTestModel(t *testing.T) Model {
	m := New()
	m.width = 100
	m.height = 30
	return m
}

// updateModel is a helper to update the model and return the typed Model.
func updateModel(t *testing.T, m Model, msg tea.Msg) Model {
	result, _ := m.Update(msg)
	return result.(Model)
}

// selectComponent navigates to index and selects it via Enter.
func selectComponent(t *testing.T, m Model, index int) Model {
	for i := 0; i < index; i++ {
		m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	}
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	return m
}

// Golden tests for playground mode rendering.
// Run with -update flag to update golden files: go test -update ./internal/mode/playground/...

func TestPlayground_Golden_Sidebar(t *testing.T) {
	m := createGoldenTestModel(t)
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestPlayground_Golden_SidebarNavigation(t *testing.T) {
	m := createGoldenTestModel(t)

	// Navigate down to second item
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestPlayground_Golden_PickerDemo(t *testing.T) {
	m := createGoldenTestModel(t)

	// Select picker demo (index 0)
	m = selectComponent(t, m, 0)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestPlayground_Golden_ToasterDemo(t *testing.T) {
	m := createGoldenTestModel(t)

	// Select toaster demo (index 1)
	m = selectComponent(t, m, 1)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestPlayground_Golden_PanesDemo(t *testing.T) {
	m := createGoldenTestModel(t)

	// Select panes demo (index 2)
	m = selectComponent(t, m, 2)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestPlayground_Golden_ThemeTokens(t *testing.T) {
	m := createGoldenTestModel(t)

	// Select theme tokens demo (last item, index 12)
	m = selectComponent(t, m, 12)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestPlayground_Golden_Help(t *testing.T) {
	m := createGoldenTestModel(t)

	// Open help with "?"
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestPlayground_Golden_DemoFocused(t *testing.T) {
	m := createGoldenTestModel(t)

	// Select picker demo (index 0)
	m = selectComponent(t, m, 0)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestPlayground_Golden_ColorpickerDemo(t *testing.T) {
	m := createGoldenTestModel(t)

	// Select colorpicker demo (index 3)
	m = selectComponent(t, m, 3)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestPlayground_Golden_ModalDemo(t *testing.T) {
	m := createGoldenTestModel(t)

	// Select modal demo (index 4)
	m = selectComponent(t, m, 4)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestPlayground_Golden_VimtextareaDemo(t *testing.T) {
	m := createGoldenTestModel(t)

	// Select vimtextarea demo (index 5)
	m = selectComponent(t, m, 5)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestPlayground_Golden_FormmodalDemo(t *testing.T) {
	m := createGoldenTestModel(t)

	// Select formmodal demo (index 6)
	m = selectComponent(t, m, 6)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestPlayground_Golden_MarkdownDemo(t *testing.T) {
	m := createGoldenTestModel(t)

	// Select markdown demo (index 7)
	m = selectComponent(t, m, 7)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestPlayground_Golden_ChainartDemo(t *testing.T) {
	m := createGoldenTestModel(t)

	// Select chainart demo (index 8)
	m = selectComponent(t, m, 8)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestPlayground_Golden_ScrollpaneDemo(t *testing.T) {
	m := createGoldenTestModel(t)

	// Select scrollpane demo (index 9)
	m = selectComponent(t, m, 9)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestPlayground_Golden_LogoverlayDemo(t *testing.T) {
	m := createGoldenTestModel(t)

	// Select logoverlay demo (index 10)
	m = selectComponent(t, m, 10)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestPlayground_Golden_IssuebadgeDemo(t *testing.T) {
	m := createGoldenTestModel(t)

	// Select issuebadge demo (index 11)
	m = selectComponent(t, m, 11)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// Unit tests

func TestGetTokenColor(t *testing.T) {
	// Test that all tokens return valid hex values
	tokens := styles.AllTokens()
	for _, token := range tokens {
		hex := GetTokenColor(token)
		require.NotEmpty(t, hex, "Token %s should return a non-empty hex value", token)
		require.True(t, hex[0] == '#', "Token %s should return a hex value starting with #, got %s", token, hex)
	}
}

func TestGetTokenCategories(t *testing.T) {
	categories := GetTokenCategories()

	// Should have 13 categories
	require.Len(t, categories, 13, "Should have 13 token categories")

	// Count total tokens across categories
	totalTokens := 0
	for _, cat := range categories {
		totalTokens += len(cat.Tokens)
	}

	// Should match AllTokens count (46)
	allTokens := styles.AllTokens()
	require.Equal(t, len(allTokens), totalTokens, "Total tokens in categories should match AllTokens()")
}

func TestComponentRegistry(t *testing.T) {
	demos := GetComponentDemos()

	// Should have 13 demos (including theme tokens and issuebadge)
	require.Len(t, demos, 13, "Should have 13 component demos")

	// Each demo should have valid Create function
	for _, demo := range demos {
		require.NotEmpty(t, demo.Name, "Demo should have a name")
		require.NotEmpty(t, demo.Description, "Demo should have a description")
		require.NotNil(t, demo.Create, "Demo should have a Create function")

		// Verify Create function works
		model := demo.Create(80, 24)
		require.NotNil(t, model, "Create should return a non-nil model")

		// Verify Reset works
		resetModel := model.Reset()
		require.NotNil(t, resetModel, "Reset should return a non-nil model")
	}
}

func TestFocusTransitions(t *testing.T) {
	m := createGoldenTestModel(t)

	// Initial state
	require.Equal(t, FocusSidebar, m.focus)

	// Tab to demo area
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, FocusDemo, m.focus)

	// Tab back to sidebar
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, FocusSidebar, m.focus)

	// Select component with Enter
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	require.Equal(t, FocusDemo, m.focus)
	require.NotNil(t, m.demoModel)
}

func TestSidebarNavigation(t *testing.T) {
	m := createGoldenTestModel(t)

	// Initial selection
	require.Equal(t, 0, m.selectedIndex)

	// Navigate down through components
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, 1, m.selectedIndex)

	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, 2, m.selectedIndex)

	// Navigate up
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	require.Equal(t, 1, m.selectedIndex)

	// Wrap around: k at top goes to bottom
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	require.Equal(t, 0, m.selectedIndex)
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	require.Equal(t, len(m.demos)-1, m.selectedIndex)

	// Wrap around: j at bottom goes to top
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, 0, m.selectedIndex)
}

func TestResetComponent(t *testing.T) {
	m := createGoldenTestModel(t)

	// Select a component via Enter
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, m.demoModel)

	// Reset with Ctrl+R
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})

	// Model should still exist and last action should mention reset
	require.NotNil(t, m.demoModel)
	require.Contains(t, m.lastAction, "Reset")
}
