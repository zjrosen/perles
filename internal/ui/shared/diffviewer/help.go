// Package diffviewer provides a context-sensitive help overlay for the diff viewer.
package diffviewer

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"

	"github.com/zjrosen/perles/internal/keys"
	"github.com/zjrosen/perles/internal/ui/shared/overlay"
	"github.com/zjrosen/perles/internal/ui/styles"
)

// helpContext indicates which context the help overlay should display.
type helpContext int

const (
	// helpContextFileList shows help for the file list pane.
	helpContextFileList helpContext = iota
	// helpContextCommits shows help for the commits pane (list mode).
	helpContextCommits
	// helpContextCommitFiles shows help for the commit files pane (files mode).
	helpContextCommitFiles
	// helpContextDiff shows help for the diff pane.
	helpContextDiff
)

// Help styles (package-level to avoid recreating each render).
var (
	helpTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(styles.OverlayTitleColor).
			PaddingLeft(2)

	helpDividerStyle = lipgloss.NewStyle().
				Foreground(styles.OverlayBorderColor)

	helpSectionStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(styles.OverlayTitleColor).
				MarginTop(1)

	helpKeyStyle = lipgloss.NewStyle().
			Foreground(styles.TextSecondaryColor).
			Width(11)

	helpDescStyle = lipgloss.NewStyle().
			Foreground(styles.TextDescriptionColor)

	helpBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(styles.OverlayBorderColor)

	helpContentStyle = lipgloss.NewStyle().
				Padding(0, 2)

	helpFooterStyle = lipgloss.NewStyle().
			Foreground(styles.TextMutedColor).
			MarginTop(1)
)

// helpModel holds the help overlay state.
type helpModel struct {
	context helpContext
	width   int
	height  int
}

// newHelp creates a new help overlay model.
func newHelp() helpModel {
	return helpModel{
		context: helpContextFileList,
	}
}

// SetContext changes which context-specific help to display.
func (h helpModel) SetContext(ctx helpContext) helpModel {
	h.context = ctx
	return h
}

// SetSize updates the overlay dimensions.
func (h helpModel) SetSize(width, height int) helpModel {
	h.width = width
	h.height = height
	return h
}

// View renders the help overlay (standalone, no background).
func (h helpModel) View() string {
	return h.Overlay("")
}

// Overlay renders the help box on top of a background view.
func (h helpModel) Overlay(background string) string {
	helpBox := h.renderContent()

	if background == "" {
		return lipgloss.Place(
			h.width, h.height,
			lipgloss.Center, lipgloss.Center,
			helpBox,
		)
	}

	return overlay.Place(overlay.Config{
		Width:    h.width,
		Height:   h.height,
		Position: overlay.Center,
	}, helpBox, background)
}

// renderContent builds the help box content based on current context.
func (h helpModel) renderContent() string {
	columnStyle := lipgloss.NewStyle().MarginRight(4)

	// Common Navigation
	var navCol strings.Builder
	navCol.WriteString(helpSectionStyle.Render("Navigation"))
	navCol.WriteString("\n")
	navCol.WriteString(renderHelpBinding(keys.DiffViewer.NextFile))
	navCol.WriteString(renderHelpBinding(keys.DiffViewer.PrevFile))
	navCol.WriteString(renderHelpBinding(keys.DiffViewer.FocusLeft))
	navCol.WriteString(renderHelpBinding(keys.DiffViewer.FocusRight))
	navCol.WriteString(renderHelpBinding(keys.DiffViewer.Tab))

	// Scrolling & View
	var scrollCol strings.Builder
	scrollCol.WriteString(helpSectionStyle.Render("Scrolling & View"))
	scrollCol.WriteString("\n")
	scrollCol.WriteString(renderHelpBinding(keys.DiffViewer.ScrollUp))
	scrollCol.WriteString(renderHelpBinding(keys.DiffViewer.ScrollDown))
	scrollCol.WriteString(renderHelpBinding(keys.DiffViewer.GotoTop))
	scrollCol.WriteString(renderHelpBinding(keys.DiffViewer.GotoBottom))
	// Context-dependent keybinding: tabs when commits pane focused, hunks when diff pane focused
	scrollCol.WriteString(renderHelpKeyDesc("[ / ]", "tabs/hunks"))
	scrollCol.WriteString(renderHelpBinding(keys.DiffViewer.ToggleViewMode))

	// Context-specific actions
	var actionsCol strings.Builder
	actionsCol.WriteString(helpSectionStyle.Render("Actions"))
	actionsCol.WriteString("\n")

	switch h.context {
	case helpContextFileList:
		actionsCol.WriteString(renderHelpKeyDesc("Enter", "toggle/view file"))
	case helpContextCommits:
		actionsCol.WriteString(renderHelpKeyDesc("Enter", "view commit files"))
	case helpContextCommitFiles:
		actionsCol.WriteString(renderHelpKeyDesc("Enter", "toggle/view file"))
		actionsCol.WriteString(renderHelpKeyDesc("Esc", "back to commits"))
	case helpContextDiff:
		actionsCol.WriteString(renderHelpKeyDesc("h", "back to list"))
	}
	actionsCol.WriteString(renderHelpBinding(keys.DiffViewer.CopyHunk))

	// General
	var generalCol strings.Builder
	generalCol.WriteString(helpSectionStyle.Render("General"))
	generalCol.WriteString("\n")
	generalCol.WriteString(renderHelpBinding(keys.DiffViewer.Help))
	generalCol.WriteString(renderHelpBinding(keys.DiffViewer.Close))

	// Join columns horizontally
	columns := lipgloss.JoinHorizontal(
		lipgloss.Top,
		columnStyle.Render(navCol.String()),
		columnStyle.Render(scrollCol.String()),
		columnStyle.Render(actionsCol.String()),
		generalCol.String(),
	)

	// Calculate box width
	columnsWidth := lipgloss.Width(columns)
	boxWidth := columnsWidth + 4

	// Footer with context-aware hint
	footer := helpFooterStyle.Render("Press ? or Esc to close")

	// Build body
	var body strings.Builder
	body.WriteString(columns)
	body.WriteString("\n")
	body.WriteString(footer)

	bodyContent := helpContentStyle.Render(body.String())
	divider := helpDividerStyle.Render(strings.Repeat("─", boxWidth))

	var content strings.Builder
	content.WriteString(helpTitleStyle.Render("Diff Viewer Help"))
	content.WriteString("\n")
	content.WriteString(divider)
	content.WriteString("\n")
	content.WriteString(bodyContent)

	return helpBoxStyle.Width(boxWidth).Render(content.String())
}

// renderHelpBinding renders a key.Binding as "key  description\n".
func renderHelpBinding(b key.Binding) string {
	help := b.Help()
	return renderHelpKeyDesc(help.Key, help.Desc)
}

// renderHelpKeyDesc renders "key  description\n" with styling.
func renderHelpKeyDesc(k, desc string) string {
	return helpKeyStyle.Render(k) + helpDescStyle.Render(desc) + "\n"
}
