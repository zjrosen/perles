package orchestration

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"

	"perles/internal/orchestration/pool"
)

func TestUpdate_WindowSize(t *testing.T) {
	m := New(Config{})

	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	require.Equal(t, 120, m.width)
	require.Equal(t, 40, m.height)
}

func TestUpdate_TabCyclesMessageTargets(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Initial: Input is focused, target is COORDINATOR
	require.True(t, m.input.Focused())
	require.Equal(t, "COORDINATOR", m.messageTarget)

	// Tab with no workers -> cycles through COORDINATOR -> BROADCAST -> COORDINATOR
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.True(t, m.input.Focused()) // Input stays focused
	require.Equal(t, "BROADCAST", m.messageTarget)

	// Tab -> back to COORDINATOR (no workers yet)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, "COORDINATOR", m.messageTarget)

	// Add workers and test full cycling
	m = m.UpdateWorker("worker-1", pool.WorkerWorking)
	m = m.UpdateWorker("worker-2", pool.WorkerWorking)

	// Tab -> BROADCAST
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, "BROADCAST", m.messageTarget)

	// Tab -> worker-1
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, "worker-1", m.messageTarget)

	// Tab -> worker-2
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, "worker-2", m.messageTarget)

	// Tab -> COORDINATOR (wrap)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, "COORDINATOR", m.messageTarget)
}

func TestUpdate_CtrlBracketsCycleWorkers(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Add workers
	m = m.UpdateWorker("worker-1", pool.WorkerWorking)
	m = m.UpdateWorker("worker-2", pool.WorkerWorking)

	// Initial: worker-1 displayed
	require.Equal(t, "worker-1", m.CurrentWorkerID())

	// ctrl+] -> worker-2
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}, Alt: false})
	// Note: ctrl+] is tricky to simulate, test CycleWorker directly instead
	m = m.CycleWorker(true)
	require.Equal(t, "worker-2", m.CurrentWorkerID())

	// ctrl+[ -> worker-1
	m = m.CycleWorker(false)
	require.Equal(t, "worker-1", m.CurrentWorkerID())
}

func TestUpdate_InputAlwaysFocused(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Input is focused by default
	require.True(t, m.input.Focused())

	// Tab doesn't unfocus input - just cycles targets
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.True(t, m.input.Focused())
}

func TestUpdate_InputSubmit(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Focus and type in input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.input.SetValue("Hello coordinator")

	// Submit with Enter
	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Input should be cleared
	require.Equal(t, "", m.input.Value())

	// Should produce UserInputMsg
	require.NotNil(t, cmd)
	msg := cmd()
	userMsg, ok := msg.(UserInputMsg)
	require.True(t, ok)
	require.Equal(t, "Hello coordinator", userMsg.Content)
}

func TestUpdate_InputEmpty_NoSubmit(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Focus input but leave empty
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, m.input.Focused())

	// Submit with empty input should not produce command
	var cmd tea.Cmd
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	require.Nil(t, cmd)
}

func TestUpdate_QuitMsg(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Esc quits (input is always focused)
	var cmd tea.Cmd
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEscape})

	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(QuitMsg)
	require.True(t, ok)
}

func TestUpdate_EscQuits(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 40)

	var cmd tea.Cmd
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEscape})

	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(QuitMsg)
	require.True(t, ok)
}

func TestUpdate_EscFromInputQuits(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Focus input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, m.input.Focused())

	// Esc should quit (not just unfocus)
	var cmd tea.Cmd
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEscape})

	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(QuitMsg)
	require.True(t, ok)
}

func TestUpdate_PauseMsg(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Ctrl+P pauses (input is always focused)
	var cmd tea.Cmd
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})

	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(PauseMsg)
	require.True(t, ok)
}

func TestUpdate_TabKeepsInputFocused(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Input is focused by default
	require.True(t, m.input.Focused())

	// Tab keeps input focused (just cycles message target)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.True(t, m.input.Focused())
}
