package saveactionpicker

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	m := New("status:open")

	assert.Equal(t, "status:open", m.Query(), "expected query to be set")
	assert.Equal(t, 0, m.Selected(), "expected default selection at 0")
}

func TestSetSize(t *testing.T) {
	m := New("test")

	m = m.SetSize(120, 40)
	assert.Equal(t, 120, m.width, "expected width to be 120")
	assert.Equal(t, 40, m.height, "expected height to be 40")

	// Verify immutability
	m2 := m.SetSize(80, 24)
	assert.Equal(t, 80, m2.width, "expected new model width to be 80")
	assert.Equal(t, 120, m.width, "expected original model width unchanged")
}

func TestUpdate_NavigateDown(t *testing.T) {
	m := New("test")

	// Initial state: first option selected
	assert.Equal(t, 0, m.Selected())

	// Navigate down with 'j'
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Equal(t, 1, m.Selected(), "expected selection at 1 after 'j'")

	// Navigate down wraps to 0
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 0, m.Selected(), "expected selection to wrap to 0")
}

func TestUpdate_NavigateUp(t *testing.T) {
	m := New("test")

	// Initial state: first option selected
	assert.Equal(t, 0, m.Selected())

	// Navigate up wraps to 1 (from 0)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	assert.Equal(t, 1, m.Selected(), "expected selection to wrap to 1 after 'k'")

	// Navigate up from 1 goes to 0
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, 0, m.Selected(), "expected selection at 0 after up arrow")
}

func TestUpdate_SelectExistingView(t *testing.T) {
	m := New("status:open")

	// Select first option (existing view)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd, "expected command from enter")

	msg := cmd().(SelectMsg)
	assert.Equal(t, ActionExistingView, msg.Action, "expected ActionExistingView")
	assert.Equal(t, "status:open", msg.Query, "expected query to be passed")
}

func TestUpdate_SelectNewView(t *testing.T) {
	m := New("priority:high")

	// Navigate to second option
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 1, m.Selected())

	// Select second option (new view)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd, "expected command from enter")

	msg := cmd().(SelectMsg)
	assert.Equal(t, ActionNewView, msg.Action, "expected ActionNewView")
	assert.Equal(t, "priority:high", msg.Query, "expected query to be passed")
}

func TestUpdate_Cancel(t *testing.T) {
	m := New("test")

	// Press escape
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	require.NotNil(t, cmd, "expected command from esc")

	msg := cmd()
	_, ok := msg.(CancelMsg)
	assert.True(t, ok, "expected CancelMsg")
}

func TestUpdate_CtrlN(t *testing.T) {
	m := New("test")

	// ctrl+n should navigate down
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	assert.Equal(t, 1, m.Selected(), "expected selection at 1 after ctrl+n")
}

func TestUpdate_CtrlP(t *testing.T) {
	m := New("test")

	// ctrl+p from 0 should wrap to 1
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	assert.Equal(t, 1, m.Selected(), "expected selection at 1 after ctrl+p")
}

func TestView(t *testing.T) {
	m := New("test").SetSize(80, 24)
	view := m.View()

	// Should contain title
	assert.Contains(t, view, "Save search query as column:", "expected title")

	// Should contain options
	assert.Contains(t, view, "Save to existing view", "expected first option")
	assert.Contains(t, view, "Save to new view", "expected second option")

	// Should have selection indicator
	assert.Contains(t, view, ">", "expected selection indicator")
}

func TestView_Stability(t *testing.T) {
	m := New("test").SetSize(80, 24)

	view1 := m.View()
	view2 := m.View()

	assert.Equal(t, view1, view2, "expected stable output")
}

// TestView_Golden uses teatest golden file comparison.
// Run with -update flag to update golden files: go test -update ./internal/ui/saveactionpicker/...
func TestView_Golden(t *testing.T) {
	m := New("status:open").SetSize(80, 24)
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Selected_Golden(t *testing.T) {
	m := New("status:open").SetSize(80, 24)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}
