package unsupported

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/task"
)

// Embedded Details in task.ParseEmbeddedDetail's "<cause>: <text>" format.
const (
	detailBDNotFound = "bd-not-found: bd export failed: bd CLI not found on PATH"
	detailBDVersion  = "bd-version: embedded workspaces need bd >= 1.0.5 (found 1.0.4)"
	detailBDError    = "bd-error: bd export failed: beads database needs a schema migration: run `bd migrate schema` — Error: schema 45 is behind bd's 47"
	detailWorkspace  = "workspace: /p/.beads/embeddeddolt holds no embedded Dolt repository"
)

var allKinds = []struct {
	name   string
	kind   task.UnsupportedWorkspaceKind
	detail string
}{
	{"Embedded", task.WorkspaceEmbeddedDolt, ""},
	{"EmbeddedBDNotFound", task.WorkspaceEmbeddedDolt, detailBDNotFound},
	{"EmbeddedBDVersion", task.WorkspaceEmbeddedDolt, detailBDVersion},
	{"EmbeddedBDError", task.WorkspaceEmbeddedDolt, detailBDError},
	{"EmbeddedWorkspace", task.WorkspaceEmbeddedDolt, detailWorkspace},
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
	// serverMode is the server-mode alternative every embedded body ends with.
	serverMode := []string{
		"New project with no issues yet:", "bd init --server",
		"bd backup init <backup-dir> && bd backup sync", "bd backup restore --force <backup-dir>",
	}
	// embedded is a cause body: its own lines, then the server-mode
	// alternative, without the empty-Detail body's "needs server mode".
	embedded := func(lines ...string) []string {
		return append(append(lines, "Or switch the project to Dolt server mode"), serverMode...)
	}
	notCause := []string{"Perles needs server mode", "no SQL server"}

	tests := []struct {
		kind    task.UnsupportedWorkspaceKind
		detail  string
		want    []string
		notWant []string
	}{
		{task.WorkspaceEmbeddedDolt, "", append([]string{"embedded Dolt", "no SQL server", "Perles needs server mode"}, serverMode...),
			[]string{"Or switch", "through bd"}},
		{task.WorkspaceEmbeddedDolt, " ", []string{"Perles needs server mode"}, []string{"Or switch"}},
		{task.WorkspaceEmbeddedDolt, detailBDNotFound, embedded(
			"embedded Dolt project through bd, but can't find bd:",
			"bd export failed: bd CLI not found on PATH",
			"Install bd 1.0.5 or later, put it on PATH and start perles again.",
		), append(notCause, "bd-not-found")},
		{task.WorkspaceEmbeddedDolt, detailBDVersion, embedded(
			"embedded Dolt project through bd, but can't use this bd:",
			"embedded workspaces need bd >= 1.0.5 (found 1.0.4)",
			"Install or upgrade bd to 1.0.5 or later and start perles again.",
		), append(notCause, "bd-version")},
		{task.WorkspaceEmbeddedDolt, "bd-version: embedded workspaces need bd >= 1.0.5 (could not read the bd version: bd context failed)", embedded(
			"but can't use this bd:",
			"(could not read the bd version:",
			"Install or upgrade bd to 1.0.5 or later",
		), notCause},
		{task.WorkspaceEmbeddedDolt, detailBDError, embedded(
			"embedded Dolt project through bd, but bd failed:",
			"bd export failed: beads database needs a schema migration: run `bd migrate",
			"schema` — Error: schema 45 is behind bd's 47",
			"Fix the problem above and start perles again.",
		), append(notCause, "bd-error")},
		{task.WorkspaceEmbeddedDolt, detailWorkspace, embedded(
			"embedded Dolt project through bd, but can't open it:",
			"/p/.beads/embeddeddolt holds no embedded Dolt repository",
			"Fix the problem above and start perles again.",
		), append(notCause, "workspace:")},
		// A Detail without a recognised cause is a bd error.
		{task.WorkspaceEmbeddedDolt, "bd export failed: boom", embedded(
			"but bd failed:", "bd export failed: boom",
		), notCause},
		{task.WorkspaceProxiedServer, "", []string{
			"proxied-server", "bd dolt stop", "bd migrate from-proxied-server-to-server",
		}, nil},
		{task.WorkspaceHTTPBackend, "", []string{"bd connect", "http backend", "bd connect --clear"}, nil},
		{task.WorkspaceGraph, `graph_mode "link"`, []string{`graph_mode "link"`, "graph data model"}, nil},
		{task.WorkspaceGraph, "", []string{"graph data model (graph_mode)"}, nil},
		{task.WorkspaceLegacySQLite, "", []string{"SQLite", "docs/getting-started/upgrading.md#cross-era-upgrades"}, nil},
		{task.WorkspaceUnknownBackend, "postgres", []string{`"postgres" storage backend`}, nil},
		{task.WorkspaceUnknownBackend, "", []string{`"unknown" storage backend`}, nil},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind)+"/"+tt.detail, func(t *testing.T) {
			view := New(tt.kind, tt.detail).SetSize(100, 40).View()
			require.Contains(t, view, "Looks like there's a break in the chain!")
			require.Contains(t, view, "Press q to quit")
			for _, want := range tt.want {
				require.Contains(t, view, want)
			}
			for _, notWant := range tt.notWant {
				require.NotContains(t, view, notWant)
			}
		})
	}
}

// A long bd error is word-wrapped and cut to maxReasonLines lines, so the
// remedy, the server-mode alternative and the quit hint still fit 80x24.
func TestUnsupported_View_LongReasonIsCapped(t *testing.T) {
	long := "bd-error: bd export failed: " + strings.Repeat("the storage layer reported a failure ", 20) + "THE-END"
	m := New(task.WorkspaceEmbeddedDolt, long).SetSize(80, 24)

	reason := m.reasonLines(screenStyles{}, strings.TrimPrefix(long, "bd-error: "))
	require.Len(t, reason, maxReasonLines)
	for _, line := range reason {
		require.LessOrEqual(t, lipgloss.Width(line), maxReasonWidth)
	}
	require.True(t, strings.HasSuffix(reason[maxReasonLines-1], "…"))

	view := m.View()
	require.Equal(t, 24, lipgloss.Height(view))
	require.LessOrEqual(t, lipgloss.Width(view), 80)
	require.Contains(t, view, "bd export failed: the storage layer reported a failure")
	require.NotContains(t, view, "THE-END")
	require.Contains(t, view, "Fix the problem above and start perles again.")
	require.Contains(t, view, "bd backup restore --force <backup-dir>")
	require.Contains(t, view, "Press q to quit")
}

// The reason wraps to the view on terminals narrower than maxReasonWidth.
func TestUnsupported_ReasonLines_WrapToView(t *testing.T) {
	text := strings.Repeat("word ", 30)
	m := New(task.WorkspaceEmbeddedDolt, "").SetSize(50, 24)
	lines := m.reasonLines(screenStyles{}, text)
	require.Greater(t, len(lines), 1)
	for _, line := range lines {
		require.LessOrEqual(t, lipgloss.Width(line), 46)
	}

	require.Empty(t, m.reasonLines(screenStyles{}, " \n\t "))
	require.Equal(t, []string{"a b"}, m.reasonLines(screenStyles{}, "a\n  b"))
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
