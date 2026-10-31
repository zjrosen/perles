// Package serverdown provides a view shown when the Dolt server is unreachable.
package serverdown

import (
	"strings"

	"github.com/zjrosen/perles/internal/keys"
	"github.com/zjrosen/perles/internal/ui/shared/chainart"
	"github.com/zjrosen/perles/internal/ui/styles"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Model holds the server-down view state.
type Model struct {
	width   int
	height  int
	address string
	reason  string
	managed bool
}

// NewUnreachable creates a view for a Dolt server that could not be reached.
// address names the server (host:port or unix:<socket>), reason is the
// connection error (may be empty), and managed reports whether bd starts this
// server itself, in which case the view suggests `bd dolt start`.
func NewUnreachable(address, reason string, managed bool) Model {
	return Model{
		address: address,
		reason:  reason,
		managed: managed,
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

// View renders the server-down state.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.TextPrimaryColor)

	messageStyle := lipgloss.NewStyle().
		Foreground(styles.TextDescriptionColor)

	addressStyle := lipgloss.NewStyle().
		Foreground(styles.TextPrimaryColor).
		Bold(true)

	cmdStyle := lipgloss.NewStyle().
		Foreground(styles.StatusSuccessColor).
		Bold(true)

	noteStyle := lipgloss.NewStyle().
		Foreground(styles.TextMutedColor)

	hintStyle := lipgloss.NewStyle().
		Foreground(styles.TextMutedColor).
		Italic(true)

	text := []string{
		titleStyle.Render("Oh no! Looks like there's a break in the chain!"),
		"",
		messageStyle.Render("Could not connect to the Dolt server at ") +
			addressStyle.Render(m.address) + messageStyle.Render("."),
	}
	if reason := m.reasonLine(); reason != "" {
		text = append(text, noteStyle.Render(reason))
	}
	text = append(text, "")

	if m.managed {
		text = append(text,
			messageStyle.Render("Start the server for this project with:"),
			"",
			cmdStyle.Render("bd dolt start"),
			"",
			noteStyle.Render("A remote or externally managed server must be started where it runs."),
		)
	} else {
		text = append(text,
			messageStyle.Render("bd doesn't start this server. Make sure it is running and reachable,"),
			messageStyle.Render("and check BEADS_DOLT_SERVER_* or .beads/metadata.json if it moved."),
		)
	}
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

// reasonLine is the connection error on one line, truncated to the view width.
func (m Model) reasonLine() string {
	reason := strings.Join(strings.Fields(m.reason), " ")
	if reason == "" {
		return ""
	}
	return ansi.Truncate(reason, max(m.width-4, 10), "…")
}

// SetSize updates the view dimensions.
func (m Model) SetSize(width, height int) Model {
	m.width = width
	m.height = height
	return m
}
