package serverdown

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/require"
)

const refused = "dial tcp 127.0.0.1:50362: connect: connection refused"

func TestServerDown_NewUnreachable(t *testing.T) {
	m := NewUnreachable("127.0.0.1:50362", refused, true)

	require.Equal(t, "127.0.0.1:50362", m.address)
	require.Equal(t, refused, m.reason)
	require.True(t, m.managed)
	require.Nil(t, m.Init())
}

func TestServerDown_SetSizeAndWindowSize(t *testing.T) {
	m := NewUnreachable("127.0.0.1:3307", "", true).SetSize(120, 40)
	require.Equal(t, 120, m.width)
	require.Equal(t, 40, m.height)

	newModel, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	require.Nil(t, cmd)
	require.Equal(t, 80, newModel.(Model).width)
	require.Equal(t, 120, m.width, "Update must not mutate the receiver")
}

func TestServerDown_QuitKeys(t *testing.T) {
	for _, k := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'q'}},
		{Type: tea.KeyCtrlC},
		{Type: tea.KeyEsc},
	} {
		t.Run(k.String(), func(t *testing.T) {
			_, cmd := NewUnreachable("127.0.0.1:3307", "", true).SetSize(80, 24).Update(k)
			require.NotNil(t, cmd)
			_, isQuit := cmd().(tea.QuitMsg)
			require.True(t, isQuit)
		})
	}

	_, cmd := NewUnreachable("127.0.0.1:3307", "", true).SetSize(80, 24).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	require.Nil(t, cmd)
}

func TestServerDown_EmptyDimensions(t *testing.T) {
	require.Empty(t, NewUnreachable("a", "", true).SetSize(0, 24).View())
	require.Empty(t, NewUnreachable("a", "", true).SetSize(80, 0).View())
}

func TestServerDown_View_Managed(t *testing.T) {
	view := NewUnreachable("127.0.0.1:50362", refused, true).SetSize(100, 30).View()

	require.Contains(t, view, "Looks like there's a break in the chain!")
	require.Contains(t, view, "127.0.0.1:50362")
	require.Contains(t, view, "connection refused")
	require.Contains(t, view, "bd dolt start")
	require.Contains(t, view, "Press q to quit")
}

func TestServerDown_View_External(t *testing.T) {
	view := NewUnreachable("db.example.com:3307", refused, false).SetSize(100, 30).View()

	require.Contains(t, view, "db.example.com:3307")
	require.Contains(t, view, "bd doesn't start this server")
	require.NotContains(t, view, "bd dolt start")
}

func TestServerDown_View_LongReasonTruncated(t *testing.T) {
	reason := strings.Repeat("very long error ", 20)
	view := NewUnreachable("127.0.0.1:3307", reason, true).SetSize(80, 24).View()

	require.Contains(t, view, "…")
	require.LessOrEqual(t, lipgloss.Width(view), 80)
	require.Equal(t, 24, lipgloss.Height(view))
}

// Golden tests: go test ./internal/ui/serverdown/... -update
func TestServerDown_View_Golden_Standard(t *testing.T) {
	teatest.RequireEqualOutput(t, []byte(NewUnreachable("127.0.0.1:50362", refused, true).SetSize(80, 24).View()))
}

func TestServerDown_View_Golden_External(t *testing.T) {
	teatest.RequireEqualOutput(t, []byte(NewUnreachable("unix:/var/run/dolt.sock",
		"dial unix /var/run/dolt.sock: connect: no such file or directory", false).SetSize(80, 24).View()))
}

func TestServerDown_View_Golden_Large(t *testing.T) {
	teatest.RequireEqualOutput(t, []byte(NewUnreachable("127.0.0.1 (no port found)",
		"no Dolt server port found (.beads/dolt-server.port is missing)", true).SetSize(120, 40).View()))
}
