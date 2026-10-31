// Package unsupported provides the view shown when the beads workspace is in a
// mode or format perles cannot read: embedded or proxied-server Dolt, the http
// backend, a graph workspace, a historical SQLite workspace, or an unknown
// storage backend. Each kind explains why and what to run instead.
package unsupported

import (
	"fmt"

	"github.com/zjrosen/perles/internal/keys"
	"github.com/zjrosen/perles/internal/task"
	"github.com/zjrosen/perles/internal/ui/shared/chainart"
	"github.com/zjrosen/perles/internal/ui/styles"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Model holds the unsupported-workspace view state.
type Model struct {
	width  int
	height int
	kind   task.UnsupportedWorkspaceKind
	detail string
}

// New creates a view for a workspace of the given kind. detail qualifies the
// kind (see task.UnsupportedWorkspaceError.Detail).
func New(kind task.UnsupportedWorkspaceKind, detail string) Model {
	return Model{kind: kind, detail: detail}
}

// Init returns the initial command.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Common.Quit), key.Matches(msg, keys.Common.Escape):
			return m, tea.Quit
		}
	}
	return m, nil
}

// SetSize updates the view dimensions.
func (m Model) SetSize(width, height int) Model {
	m.width = width
	m.height = height
	return m
}

// screenStyles are the text styles the view composes.
type screenStyles struct {
	message lipgloss.Style
	command lipgloss.Style
	note    lipgloss.Style
}

// View renders the unsupported-workspace state.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	st := screenStyles{
		message: lipgloss.NewStyle().Foreground(styles.TextDescriptionColor),
		command: lipgloss.NewStyle().Foreground(styles.StatusSuccessColor).Bold(true),
		note:    lipgloss.NewStyle().Foreground(styles.TextMutedColor),
	}
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.TextPrimaryColor)
	hintStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor).Italic(true)

	text := []string{
		titleStyle.Render("Oh no! Looks like there's a break in the chain!"),
		"",
	}
	text = append(text, m.body(st)...)
	text = append(text, "\n", hintStyle.Render("Press q to quit"))

	// Drop the chain art when the full screen would not fit.
	content := lipgloss.JoinVertical(lipgloss.Center, append([]string{chainart.BuildChainArt(), "\n"}, text...)...)
	if lipgloss.Height(content) > m.height {
		content = lipgloss.JoinVertical(lipgloss.Center, text...)
	}

	containerStyle := lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Align(lipgloss.Center, lipgloss.Center)

	return containerStyle.Render(content)
}

// body returns the explanation and remedy lines for the workspace kind.
func (m Model) body(st screenStyles) []string {
	msg := func(s string) string { return st.message.Render(s) }
	cmd := func(s string) string { return st.command.Render(s) }
	note := func(s string) string { return st.note.Render(s) }

	switch m.kind {
	case task.WorkspaceEmbeddedDolt:
		return []string{
			msg("This project keeps its beads in embedded Dolt, which runs inside bd"),
			msg("and has no SQL server for perles to connect to. Perles needs server mode."),
			"",
			msg("New project with no issues yet:"),
			cmd("bd init --server"),
			"",
			msg("Existing project (bd init --server refuses one that has data):"),
			steps(st,
				step{"1. in this project:", "bd backup init <backup-dir> && bd backup sync"},
				step{"2. in a new directory:", "bd init --server"},
				step{"3. in that directory:", "bd backup restore --force <backup-dir>"},
			),
		}

	case task.WorkspaceProxiedServer:
		return []string{
			msg("This project uses Dolt proxied-server mode, which perles can't read yet."),
			"",
			msg("Switch it to server mode (both modes keep their data in .beads/dolt):"),
			cmd("bd dolt stop"),
			cmd("bd migrate from-proxied-server-to-server"),
			"",
			note("The migration is experimental in bd. It rewrites dolt_mode in the"),
			note("committed .beads/metadata.json, so other clones switch when they pull."),
		}

	case task.WorkspaceHTTPBackend:
		return []string{
			msg("This project is attached to a remote bd serve over HTTP (bd connect)."),
			msg("Perles reads beads from a Dolt SQL server and can't use the http backend."),
			"",
			msg("Run perles in the workspace that bd serve is serving, or detach this"),
			msg("one and restore its previous backend:"),
			cmd("bd connect --clear"),
		}

	case task.WorkspaceGraph:
		detail := m.detail
		if detail == "" {
			detail = "graph_mode"
		}
		return []string{
			msg(fmt.Sprintf("This workspace uses a graph data model (%s),", detail)),
			msg("which perles can't read as an issue database."),
			"",
			msg("Open it with the bd build that created it, and leave graph_mode,"),
			msg("the graph_* fields and the .beads directory unchanged."),
		}

	case task.WorkspaceLegacySQLite:
		return []string{
			msg("This project still keeps its beads in SQLite. Beads moved to Dolt in"),
			msg("v0.58, and current bd refuses this workspace until it is migrated."),
			"",
			msg("Keep .beads unchanged and follow the sealed SQLite bridge in"),
			msg("the cross-era upgrade guide of the beads docs:"),
			cmd("docs/getting-started/upgrading.md#cross-era-upgrades"),
			"",
			note("Perles can read the migrated project once it runs in Dolt server mode."),
		}

	default:
		backend := m.detail
		if backend == "" {
			backend = "unknown"
		}
		return []string{
			msg(fmt.Sprintf("This project uses the %q storage backend, which perles can't read.", backend)),
			msg("Perles reads beads projects that use Dolt in server mode."),
			"",
			msg("Check \"backend\" in .beads/metadata.json, or see the beads upgrade guide:"),
			cmd("docs/getting-started/upgrading.md"),
		}
	}
}

// step is one numbered instruction: a label and the command to run.
type step struct {
	label   string
	command string
}

// steps renders numbered instructions as a left-aligned block with the
// commands in one column; the caller centers the block as a whole.
func steps(st screenStyles, items ...step) string {
	labelWidth := 0
	for _, it := range items {
		labelWidth = max(labelWidth, lipgloss.Width(it.label))
	}
	labelStyle := st.message.Width(labelWidth + 2)

	lines := make([]string, 0, len(items))
	for _, it := range items {
		lines = append(lines, labelStyle.Render(it.label)+st.command.Render(it.command))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}
