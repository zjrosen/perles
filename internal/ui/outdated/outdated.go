// Package outdated provides the view shown when beads version is too old.
package outdated

import (
	"github.com/zjrosen/perles/internal/keys"
	"github.com/zjrosen/perles/internal/ui/shared/chainart"
	"github.com/zjrosen/perles/internal/ui/styles"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Model holds the outdated view state.
type Model struct {
	width           int
	height          int
	currentVersion  string
	requiredVersion string
}

// New creates a new outdated view with version information.
func New(currentVersion, requiredVersion string) Model {
	return Model{
		currentVersion:  currentVersion,
		requiredVersion: requiredVersion,
	}
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

// View renders the outdated state.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	art := chainart.BuildChainArt()

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.TextPrimaryColor)

	messageStyle := lipgloss.NewStyle().
		Foreground(styles.TextDescriptionColor)

	versionStyle := lipgloss.NewStyle().
		Foreground(styles.StatusSuccessColor).
		Bold(true)

	cmdStyle := lipgloss.NewStyle().
		Foreground(styles.StatusSuccessColor).
		Bold(true)

	noteStyle := lipgloss.NewStyle().
		Foreground(styles.TextMutedColor)

	hintStyle := lipgloss.NewStyle().
		Foreground(styles.TextMutedColor).
		Italic(true)

	// Build the version message with proper styling (not nested)
	versionMsg := messageStyle.Render("Your beads database version (") +
		versionStyle.Render(m.currentVersion) +
		messageStyle.Render(") is older than required version (") +
		versionStyle.Render(m.requiredVersion) +
		messageStyle.Render(").")

	// Build content with vertical join for proper centering
	content := lipgloss.JoinVertical(
		lipgloss.Center,
		art,
		"\n",
		titleStyle.Render("Oh no! Looks like there's a break in the chain!"),
		"",
		versionMsg,
		"",
		messageStyle.Render("Upgrade bd, then update the database:"),
		"",
		cmdStyle.Render("bd migrate schema"),
		cmdStyle.Render("bd migrate"),
		"",
		noteStyle.Render("If bd says explicit migration is required, follow the cross-era"),
		noteStyle.Render("upgrade guide: docs/getting-started/upgrading.md in the beads docs."),
		"\n",
		hintStyle.Render("Press q to quit"),
	)

	// Center the content in the viewport
	containerStyle := lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Align(lipgloss.Center, lipgloss.Center)

	return containerStyle.Render(content)
}

// SetSize updates the view dimensions.
func (m Model) SetSize(width, height int) Model {
	m.width = width
	m.height = height
	return m
}
