// Package unsupported provides the view shown when the beads workspace is in a
// mode or format perles cannot read: embedded Dolt the snapshot reader cannot
// start on, proxied-server Dolt, the http backend, a graph workspace, a
// historical SQLite workspace, or an unknown storage backend. Each kind
// explains why and what to run instead.
package unsupported

import (
	"fmt"
	"strings"

	"github.com/zjrosen/perles/internal/keys"
	"github.com/zjrosen/perles/internal/task"
	"github.com/zjrosen/perles/internal/ui/shared/chainart"
	"github.com/zjrosen/perles/internal/ui/styles"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Model holds the unsupported-workspace view state.
type Model struct {
	width  int
	height int
	kind   task.UnsupportedWorkspaceKind
	detail string
	// minEmbeddedBDVersion is the oldest bd that can read an embedded
	// workspace, named in the embedded remedies.
	minEmbeddedBDVersion string
}

// New creates a view for a workspace of the given kind. detail qualifies the
// kind (see task.UnsupportedWorkspaceError.Detail). minEmbeddedBDVersion is
// the oldest bd perles' snapshot reader accepts for an embedded workspace.
func New(kind task.UnsupportedWorkspaceKind, detail, minEmbeddedBDVersion string) Model {
	return Model{kind: kind, detail: detail, minEmbeddedBDVersion: minEmbeddedBDVersion}
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
	reason  lipgloss.Style
}

// Embedded failure reasons wrap at maxReasonWidth columns and show at most
// maxReasonLines lines, so the screen fits an 80x24 terminal. A bd error
// message alone can run to 500 characters.
const (
	maxReasonWidth = 76
	maxReasonLines = 6
)

// viewChromeLines is how many lines View adds around the body: the title and
// the blank line under it, then the two-line gap and the quit hint.
const viewChromeLines = 5

// View renders the unsupported-workspace state.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	st := screenStyles{
		message: lipgloss.NewStyle().Foreground(styles.TextDescriptionColor),
		command: lipgloss.NewStyle().Foreground(styles.StatusSuccessColor).Bold(true),
		note:    lipgloss.NewStyle().Foreground(styles.TextMutedColor),
		reason:  lipgloss.NewStyle().Foreground(styles.TextPrimaryColor),
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
		cause, text := task.ParseEmbeddedDetail(m.detail)
		return m.embeddedBody(st, cause, text)

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
			msg("Perles reads Dolt server and embedded workspaces, not the http backend."),
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
			note("Perles can read the project once it is migrated to Dolt."),
		}

	default:
		backend := m.detail
		if backend == "" {
			backend = "unknown"
		}
		return []string{
			msg(fmt.Sprintf("This project uses the %q storage backend, which perles can't read.", backend)),
			msg("Perles reads beads projects that use Dolt, in server or embedded mode."),
			"",
			msg("Check \"backend\" in .beads/metadata.json, or see the beads upgrade guide:"),
			cmd("docs/getting-started/upgrading.md"),
		}
	}
}

// embeddedBody explains why perles' snapshot reader cannot start on an
// embedded workspace (see task.ParseEmbeddedDetail): the cause, bd's or the
// backend's text, the remedy, then switching to server mode as the
// alternative. An empty cause is a Detail that said nothing, which no perles
// path builds; its body names no reason.
//
// On a terminal too short for the whole body, the reason is cut to the lines
// left over, and once not even one reason line would fit, the server-mode
// alternative is dropped.
func (m Model) embeddedBody(st screenStyles, cause task.EmbeddedCause, text string) []string {
	msg := func(s string) string { return st.message.Render(s) }

	const intro = "Perles reads this embedded Dolt project through bd, "
	var problem, remedy string
	switch cause {
	case "":
		problem = "but could not start."
		remedy = fmt.Sprintf("Check that bd %s or later is on PATH and start perles again.", m.minEmbeddedBDVersion)
	case task.EmbeddedCauseBDNotFound:
		problem = "but can't find bd:"
		remedy = fmt.Sprintf("Install bd %s or later, put it on PATH and start perles again.", m.minEmbeddedBDVersion)
	case task.EmbeddedCauseBDVersion:
		problem = "but can't use this bd:"
		remedy = fmt.Sprintf("Install or upgrade bd to %s or later and start perles again.", m.minEmbeddedBDVersion)
	case task.EmbeddedCauseWorkspace:
		problem = "but can't open it:"
		remedy = "Fix the problem above and start perles again."
	default:
		problem = "but bd failed:"
		remedy = "Fix the problem above and start perles again."
	}

	head := []string{msg(intro + problem)}
	tail := []string{"", msg(remedy)}
	alternative := append([]string{
		"",
		msg("Or switch the project to Dolt server mode, which perles reads directly."),
	}, serverModeSteps(st)...)

	room := m.height - viewChromeLines - lineCount(head) - lineCount(tail)
	if strings.TrimSpace(text) != "" && room-lineCount(alternative) < 1 {
		alternative = nil
	}
	reasons := m.reasonLines(st, text, max(room-lineCount(alternative), 1))

	lines := append(head, reasons...)
	lines = append(lines, tail...)
	return append(lines, alternative...)
}

// lineCount is how many lines parts take up when joined vertically.
func lineCount(parts []string) int {
	n := 0
	for _, p := range parts {
		n += lipgloss.Height(p)
	}
	return n
}

// reasonLines renders text word-wrapped to the view, at most
// min(limit, maxReasonLines) lines; a longer text ends in an ellipsis.
func (m Model) reasonLines(st screenStyles, text string, limit int) []string {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return nil
	}
	limit = min(max(limit, 1), maxReasonLines)
	width := max(min(maxReasonWidth, m.width-4), 20)
	lines := strings.Split(ansi.Wrap(text, width, ""), "\n")
	if len(lines) > limit {
		lines = lines[:limit]
		lines[limit-1] = ansi.Truncate(lines[limit-1], width-1, "") + "…"
	}
	for i, line := range lines {
		lines[i] = st.reason.Render(line)
	}
	return lines
}

// serverModeSteps is how to move a project to Dolt server mode: a new
// project can be re-initialized, an existing one goes through a backup.
func serverModeSteps(st screenStyles) []string {
	return []string{
		st.message.Render("New project with no issues yet:"),
		st.command.Render("bd init --server"),
		"",
		st.message.Render("Existing project (bd init --server refuses one that has data):"),
		steps(st,
			step{"1. in this project:", "bd backup init <backup-dir> && bd backup sync"},
			step{"2. in a new directory:", "bd init --server"},
			step{"3. in that directory:", "bd backup restore --force <backup-dir>"},
		),
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
