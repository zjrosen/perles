// Package serverdown provides views shown when the Dolt backend is misconfigured or unreachable.
package serverdown

import (
	"github.com/zjrosen/perles/internal/keys"
	"github.com/zjrosen/perles/internal/ui/shared/chainart"
	"github.com/zjrosen/perles/internal/ui/styles"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Model holds the server-down view state.
type Model struct {
	width      int
	height     int
	host       string
	port       int
	configured bool // true = server mode configured but unreachable; false = not configured
}

// NewUnreachable creates a view for when the server is configured but not running.
func NewUnreachable(host string, port int) Model {
	return Model{
		host:       host,
		port:       port,
		configured: true,
	}
}

// NewNotConfigured creates a view for when dolt_mode is not set to "server".
func NewNotConfigured() Model {
	return Model{configured: false}
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

// View renders the server-down state.
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

	cmdStyle := lipgloss.NewStyle().
		Foreground(styles.StatusSuccessColor).
		Bold(true)

	hintStyle := lipgloss.NewStyle().
		Foreground(styles.TextMutedColor).
		Italic(true)

	var content string
	if m.configured {
		content = lipgloss.JoinVertical(
			lipgloss.Center,
			art,
			"\n",
			titleStyle.Render("Oh no! Looks like there's a break in the chain!"),
			"",
			messageStyle.Render("Could not connect to the Dolt server. Run the following command to start the Dolt server:"),
			"",
			"  "+cmdStyle.Render("bd dolt start"),
			"\n",
			hintStyle.Render("Press q to quit"),
		)
	} else {
		content = lipgloss.JoinVertical(
			lipgloss.Center,
			art,
			"\n",
			titleStyle.Render("Oh no! Looks like there's a break in the chain!"),
			"",
			messageStyle.Render("The Dolt backend requires server mode, but it is not configured."),
			"",
			messageStyle.Render("Update your .beads/metadata.json to include:"),
			"",
			"  "+cmdStyle.Render(`"dolt_mode": "server"`),
			"\n",
			hintStyle.Render("Press q to quit"),
		)
	}

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
