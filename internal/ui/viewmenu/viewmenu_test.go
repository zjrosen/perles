package viewmenu

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestViewMenu_New(t *testing.T) {
	m := New()

	assert.Equal(t, OptionCreate, m.selected, "expected default selection at OptionCreate")
}

func TestViewMenu_SetSize(t *testing.T) {
	m := New()

	m = m.SetSize(120, 40)
	assert.Equal(t, 120, m.viewportWidth, "expected viewport width to be 120")
	assert.Equal(t, 40, m.viewportHeight, "expected viewport height to be 40")

	// Verify immutability
	m2 := m.SetSize(80, 24)
	assert.Equal(t, 80, m2.viewportWidth, "expected new model width to be 80")
	assert.Equal(t, 24, m2.viewportHeight, "expected new model height to be 24")
	assert.Equal(t, 120, m.viewportWidth, "expected original model width unchanged")
}

func TestViewMenu_Selected(t *testing.T) {
	m := New()
	assert.Equal(t, OptionCreate, m.Selected(), "expected OptionCreate selected by default")
}

func TestViewMenu_Update_NavigateDown_J(t *testing.T) {
	m := New()

	// Navigate down with 'j'
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Equal(t, OptionDelete, m.selected, "expected OptionDelete after 'j'")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Equal(t, OptionRename, m.selected, "expected OptionRename after second 'j'")

	// At bottom boundary - should not go past
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Equal(t, OptionRename, m.selected, "expected selection to stay at OptionRename (boundary)")
}

func TestViewMenu_Update_NavigateDown_Arrow(t *testing.T) {
	m := New()

	// Navigate down with arrow key
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, OptionDelete, m.selected, "expected OptionDelete after down arrow")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, OptionRename, m.selected, "expected OptionRename after second down arrow")
}

func TestViewMenu_Update_NavigateDown_CtrlN(t *testing.T) {
	m := New()

	// Navigate down with ctrl+n
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	assert.Equal(t, OptionDelete, m.selected, "expected OptionDelete after ctrl+n")
}

func TestViewMenu_Update_NavigateUp_K(t *testing.T) {
	m := New()
	// Start at bottom
	m.selected = OptionRename

	// Navigate up with 'k'
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	assert.Equal(t, OptionDelete, m.selected, "expected OptionDelete after 'k'")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	assert.Equal(t, OptionCreate, m.selected, "expected OptionCreate after second 'k'")

	// At top boundary - should not go past
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	assert.Equal(t, OptionCreate, m.selected, "expected selection to stay at OptionCreate (boundary)")
}

func TestViewMenu_Update_NavigateUp_Arrow(t *testing.T) {
	m := New()
	m.selected = OptionRename

	// Navigate up with arrow key
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, OptionDelete, m.selected, "expected OptionDelete after up arrow")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, OptionCreate, m.selected, "expected OptionCreate after second up arrow")
}

func TestViewMenu_Update_NavigateUp_CtrlP(t *testing.T) {
	m := New()
	m.selected = OptionDelete

	// Navigate up with ctrl+p
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	assert.Equal(t, OptionCreate, m.selected, "expected OptionCreate after ctrl+p")
}

func TestViewMenu_Update_Enter_EmitsSelectMsg(t *testing.T) {
	tests := []struct {
		name     string
		selected Option
	}{
		{"create", OptionCreate},
		{"delete", OptionDelete},
		{"rename", OptionRename},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New()
			m.selected = tt.selected

			_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			require.NotNil(t, cmd, "expected command from Enter")

			msg := cmd()
			selectMsg, ok := msg.(SelectMsg)
			require.True(t, ok, "expected SelectMsg")
			assert.Equal(t, tt.selected, selectMsg.Option, "expected correct option in SelectMsg")
		})
	}
}

func TestViewMenu_Update_Esc_EmitsCancelMsg(t *testing.T) {
	m := New()

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	require.NotNil(t, cmd, "expected command from Esc")

	msg := cmd()
	_, ok := msg.(CancelMsg)
	assert.True(t, ok, "expected CancelMsg from Esc")
}

func TestViewMenu_View(t *testing.T) {
	m := New().SetSize(80, 24)
	view := m.View()

	// Should contain title
	assert.Contains(t, view, "View", "expected view to contain title")

	// Should contain options
	assert.Contains(t, view, "Create new view", "expected view to contain Create option")
	assert.Contains(t, view, "Delete current view", "expected view to contain Delete option")
	assert.Contains(t, view, "Rename current view", "expected view to contain Rename option")

	// Should have selection indicator
	assert.Contains(t, view, ">", "expected view to contain selection indicator")
}

func TestViewMenu_View_Stability(t *testing.T) {
	m := New().SetSize(80, 24)

	view1 := m.View()
	view2 := m.View()

	// Same model should produce identical output
	assert.Equal(t, view1, view2, "expected stable output from same model")
}

// TestViewMenu_View_Golden uses teatest golden file comparison
// Run with -update flag to update golden files: go test -update ./internal/ui/viewmenu/...
func TestViewMenu_View_Golden(t *testing.T) {
	m := New().SetSize(80, 24)
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestViewMenu_View_DeleteSelected_Golden tests menu with delete selected
func TestViewMenu_View_DeleteSelected_Golden(t *testing.T) {
	m := New().SetSize(80, 24)
	m.selected = OptionDelete
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestViewMenu_View_RenameSelected_Golden tests menu with rename selected
func TestViewMenu_View_RenameSelected_Golden(t *testing.T) {
	m := New().SetSize(80, 24)
	m.selected = OptionRename
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}
