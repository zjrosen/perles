package unsupported

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/task"
)

var allKinds = []struct {
	name   string
	kind   task.UnsupportedWorkspaceKind
	detail string
}{
	{"Embedded", task.WorkspaceEmbeddedDolt, ""},
	{"ProxiedServer", task.WorkspaceProxiedServer, ""},
	{"HTTP", task.WorkspaceHTTPBackend, ""},
	{"Graph", task.WorkspaceGraph, `graph_mode "link"`},
	{"LegacySQLite", task.WorkspaceLegacySQLite, ""},
	{"UnknownBackend", task.WorkspaceUnknownBackend, "postgres"},
}

func TestUnsupported_New(t *testing.T) {
	m := New(task.WorkspaceGraph, "x")

	require.Equal(t, 0, m.width)
	require.Equal(t, 0, m.height)
	require.Equal(t, task.WorkspaceGraph, m.kind)
	require.Equal(t, "x", m.detail)
	require.Nil(t, m.Init())
}

func TestUnsupported_SetSize(t *testing.T) {
	m := New(task.WorkspaceEmbeddedDolt, "").SetSize(120, 40)
	require.Equal(t, 120, m.width)
	require.Equal(t, 40, m.height)

	m2 := m.SetSize(80, 24)
	require.Equal(t, 80, m2.width)
	require.Equal(t, 120, m.width, "SetSize must not mutate the receiver")
}

func TestUnsupported_WindowSizeMsg(t *testing.T) {
	newModel, cmd := New(task.WorkspaceEmbeddedDolt, "").Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	updated := newModel.(Model)
	require.Equal(t, 80, updated.width)
	require.Equal(t, 24, updated.height)
	require.Nil(t, cmd)
}

func TestUnsupported_QuitKeys(t *testing.T) {
	for _, k := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'q'}},
		{Type: tea.KeyCtrlC},
		{Type: tea.KeyEsc},
	} {
		t.Run(k.String(), func(t *testing.T) {
			_, cmd := New(task.WorkspaceEmbeddedDolt, "").SetSize(80, 24).Update(k)
			require.NotNil(t, cmd)
			_, isQuit := cmd().(tea.QuitMsg)
			require.True(t, isQuit)
		})
	}
}

func TestUnsupported_OtherKey(t *testing.T) {
	_, cmd := New(task.WorkspaceEmbeddedDolt, "").SetSize(80, 24).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	require.Nil(t, cmd)
}

func TestUnsupported_EmptyDimensions(t *testing.T) {
	require.Empty(t, New(task.WorkspaceEmbeddedDolt, "").SetSize(0, 24).View())
	require.Empty(t, New(task.WorkspaceEmbeddedDolt, "").SetSize(80, 0).View())
}

func TestUnsupported_View_Content(t *testing.T) {
	tests := []struct {
		kind   task.UnsupportedWorkspaceKind
		detail string
		want   []string
	}{
		{task.WorkspaceEmbeddedDolt, "", []string{
			"embedded Dolt", "no SQL server", "bd init --server",
			"bd backup init <backup-dir> && bd backup sync", "bd backup restore --force <backup-dir>",
		}},
		{task.WorkspaceProxiedServer, "", []string{
			"proxied-server", "bd dolt stop", "bd migrate from-proxied-server-to-server",
		}},
		{task.WorkspaceHTTPBackend, "", []string{"bd connect", "http backend", "bd connect --clear"}},
		{task.WorkspaceGraph, `graph_mode "link"`, []string{`graph_mode "link"`, "graph data model"}},
		{task.WorkspaceGraph, "", []string{"graph data model (graph_mode)"}},
		{task.WorkspaceLegacySQLite, "", []string{"SQLite", "docs/getting-started/upgrading.md#cross-era-upgrades"}},
		{task.WorkspaceUnknownBackend, "postgres", []string{`"postgres" storage backend`}},
		{task.WorkspaceUnknownBackend, "", []string{`"unknown" storage backend`}},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind)+"/"+tt.detail, func(t *testing.T) {
			view := New(tt.kind, tt.detail).SetSize(100, 40).View()
			require.Contains(t, view, "Looks like there's a break in the chain!")
			require.Contains(t, view, "Press q to quit")
			for _, want := range tt.want {
				require.Contains(t, view, want)
			}
		})
	}
}

// Every kind fits a standard 80x24 terminal without wrapping or overflowing.
func TestUnsupported_View_FitsStandardTerminal(t *testing.T) {
	for _, k := range allKinds {
		t.Run(k.name, func(t *testing.T) {
			view := New(k.kind, k.detail).SetSize(80, 24).View()
			require.Equal(t, 24, lipgloss.Height(view))
			require.LessOrEqual(t, lipgloss.Width(view), 80)
			require.Contains(t, view, "Press q to quit")
		})
	}
}

// A terminal too short for the chain art still shows the instructions.
func TestUnsupported_View_DropsArtWhenShort(t *testing.T) {
	view := New(task.WorkspaceEmbeddedDolt, "").SetSize(80, 18).View()
	require.NotContains(t, view, "╔═══════╗")
	require.Contains(t, view, "bd init --server")
	require.Contains(t, view, "Press q to quit")

	tall := New(task.WorkspaceEmbeddedDolt, "").SetSize(120, 40).View()
	require.Contains(t, tall, "╔═══════╗")
}

// Golden tests: go test ./internal/ui/unsupported/... -update
func TestUnsupported_View_Golden_Standard(t *testing.T) {
	for _, k := range allKinds {
		t.Run(k.name, func(t *testing.T) {
			teatest.RequireEqualOutput(t, []byte(New(k.kind, k.detail).SetSize(80, 24).View()))
		})
	}
}

func TestUnsupported_View_Golden_Large(t *testing.T) {
	teatest.RequireEqualOutput(t, []byte(New(task.WorkspaceEmbeddedDolt, "").SetSize(120, 40).View()))
}
