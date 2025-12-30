// Package help contains the help overlay component.
package help

import (
	"strings"

	"github.com/zjrosen/perles/internal/keys"
	"github.com/zjrosen/perles/internal/ui/shared/overlay"
	"github.com/zjrosen/perles/internal/ui/styles"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
)

// BQLField represents a BQL field with its name and valid values.
type BQLField struct {
	Name   string
	Values string
}

// BQLOperator represents a BQL operator with its symbol and description.
type BQLOperator struct {
	Symbol string
	Desc   string
}

// BQLFields returns the list of BQL fields for help text.
func BQLFields() []BQLField {
	return []BQLField{
		{Name: "status", Values: "open, in_progress, closed"},
		{Name: "type", Values: "bug, feature, task, epic, chore"},
		{Name: "priority", Values: "p0, p1, p2, p3, p4"},
		{Name: "blocked", Values: "true, false"},
		{Name: "ready", Values: "true, false"},
		{Name: "pinned", Values: "true, false"},
		{Name: "is_template", Values: "true, false"},
		{Name: "label", Values: "string (use ~ for contains)"},
		{Name: "title", Values: "string (use ~ for contains)"},
		{Name: "description", Values: "string (use ~ for contains)"},
		{Name: "design", Values: "string (use ~ for contains)"},
		{Name: "notes", Values: "string (use ~ for contains)"},
		{Name: "id", Values: "string"},
		{Name: "assignee", Values: "string"},
		{Name: "sender", Values: "string"},
		{Name: "created_by", Values: "string"},
		{Name: "agent_state", Values: "idle, running, stuck, stopped"},
		{Name: "role_type", Values: "polecat, crew, witness, etc."},
		{Name: "rig", Values: "string"},
		{Name: "mol_type", Values: "string"},
		{Name: "created", Values: "date (today, yesterday, -7d)"},
		{Name: "updated", Values: "date (today, yesterday, -7d)"},
	}
}

// BQLOperators returns the list of BQL operators for help text.
func BQLOperators() []BQLOperator {
	return []BQLOperator{
		{Symbol: "=  !=", Desc: "equality"},
		{Symbol: "<  >  <=  >=", Desc: "comparison (priority, dates)"},
		{Symbol: "~  !~", Desc: "contains / not contains (strings)"},
		{Symbol: "in (a, b, c)", Desc: "match any value"},
		{Symbol: "and  or  not", Desc: "logical"},
		{Symbol: "expand", Desc: "include related: up, down, all"},
		{Symbol: "depth", Desc: "expansion levels: 1-10, or * (unlimited)"},
	}
}

// BQLExamples returns example BQL queries for help text.
func BQLExamples() []string {
	return []string{
		"status = open",
		"status = open and ready = true",
		"type = bug and priority <= p1",
		"type in (bug, task) and status != closed",
		`title ~ "auth" or label ~ "security"`,
		"created >= -7d order by priority",
		"not blocked and priority = p0",
		"type = epic expand down",
		"type = epic expand down depth *",
		"id = x expand up",
	}
}

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(styles.OverlayTitleColor).
			PaddingLeft(2)

	dividerStyle = lipgloss.NewStyle().
			Foreground(styles.OverlayBorderColor)

	sectionStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(styles.OverlayTitleColor).
			MarginTop(1)

	keyStyle = lipgloss.NewStyle().
			Foreground(styles.TextSecondaryColor).
			Width(11)

	descStyle = lipgloss.NewStyle().
			Foreground(styles.TextDescriptionColor)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(styles.OverlayBorderColor)

	contentStyle = lipgloss.NewStyle().
			Padding(0, 2)

	footerStyle = lipgloss.NewStyle().
			Foreground(styles.TextMutedColor).
			MarginTop(1)
)

// HelpMode indicates which mode's help to display.
type HelpMode int

const (
	ModeKanban HelpMode = iota
	ModeSearch
	ModeSearchTree // Tree sub-mode within search
)

// Model holds the help view state.
type Model struct {
	mode   HelpMode
	width  int
	height int
}

// New creates a new help view for kanban mode.
func New() Model {
	return Model{
		mode: ModeKanban,
	}
}

// NewSearch creates a new help view for search mode.
func NewSearch() Model {
	return Model{
		mode: ModeSearch,
	}
}

// SetMode changes the help mode (for sub-mode switching).
func (m Model) SetMode(mode HelpMode) Model {
	m.mode = mode
	return m
}

// SetSize updates dimensions.
func (m Model) SetSize(width, height int) Model {
	m.width = width
	m.height = height
	return m
}

// View renders the help overlay (standalone, no background).
func (m Model) View() string {
	return m.Overlay("")
}

// Overlay renders the help box on top of a background view.
func (m Model) Overlay(background string) string {
	helpBox := m.renderContent()

	if background == "" {
		return lipgloss.Place(
			m.width, m.height,
			lipgloss.Center, lipgloss.Center,
			helpBox,
		)
	}

	// Use shared overlay package
	return overlay.Place(overlay.Config{
		Width:    m.width,
		Height:   m.height,
		Position: overlay.Center,
	}, helpBox, background)
}

// renderContent builds the help box content.
func (m Model) renderContent() string {
	switch m.mode {
	case ModeSearchTree:
		return m.renderTreeContent()
	case ModeSearch:
		return m.renderSearchContent()
	default:
		return m.renderKanbanContent()
	}
}

// renderKanbanContent renders the kanban mode help.
func (m Model) renderKanbanContent() string {
	// Column style with right margin for spacing
	columnStyle := lipgloss.NewStyle().MarginRight(4)

	// Navigation column
	var navCol strings.Builder
	navCol.WriteString(sectionStyle.Render("Navigation"))
	navCol.WriteString("\n")
	navCol.WriteString(renderKeyDesc("h/l", "left/right"))
	navCol.WriteString(renderKeyDesc("j/k", "up/down"))
	navCol.WriteString(renderBinding(keys.Kanban.SwitchMode))

	// Actions column
	var actionsCol strings.Builder
	actionsCol.WriteString(sectionStyle.Render("Actions"))
	actionsCol.WriteString("\n")
	actionsCol.WriteString(renderBinding(keys.Kanban.Enter))
	actionsCol.WriteString(renderBinding(keys.Kanban.Refresh))
	actionsCol.WriteString(renderBinding(keys.Kanban.Yank))
	actionsCol.WriteString(renderBinding(keys.Kanban.AddColumn))
	actionsCol.WriteString(renderBinding(keys.Kanban.EditColumn))
	actionsCol.WriteString(renderBinding(keys.Kanban.MoveColumnLeft))
	actionsCol.WriteString(renderBinding(keys.Kanban.MoveColumnRight))

	// Views column
	var viewsCol strings.Builder
	viewsCol.WriteString(sectionStyle.Render("Views"))
	viewsCol.WriteString("\n")
	viewsCol.WriteString(renderBinding(keys.Kanban.NextView))
	viewsCol.WriteString(renderBinding(keys.Kanban.PrevView))
	viewsCol.WriteString(renderBinding(keys.Kanban.ViewMenu))
	viewsCol.WriteString(renderBinding(keys.Kanban.DeleteColumn))
	viewsCol.WriteString(renderBinding(keys.Kanban.SearchFromColumn))

	// General column
	var generalCol strings.Builder
	generalCol.WriteString(sectionStyle.Render("General"))
	generalCol.WriteString("\n")
	generalCol.WriteString(renderBinding(keys.Common.Help))
	generalCol.WriteString(renderBinding(keys.Kanban.ToggleStatus))
	generalCol.WriteString(renderBinding(keys.Kanban.Escape))
	generalCol.WriteString(renderBinding(keys.Common.Quit))

	// Join columns horizontally, aligned at top
	columns := lipgloss.JoinHorizontal(
		lipgloss.Top,
		columnStyle.Render(actionsCol.String()),
		columnStyle.Render(viewsCol.String()),
		columnStyle.Render(navCol.String()),
		generalCol.String(), // Last column doesn't need right margin
	)

	// Calculate box width based on columns content
	columnsWidth := lipgloss.Width(columns)
	boxWidth := columnsWidth + 4 // Add horizontal padding (2 each side)

	// Build body content with padding
	body := contentStyle.Render(columns + "\n" + footerStyle.Render("Press ? or Esc to close"))

	// Divider spans full box width
	divider := dividerStyle.Render(strings.Repeat("─", boxWidth))

	// Build final content: title, divider, body
	var content strings.Builder
	content.WriteString(titleStyle.Render("Keybindings"))
	content.WriteString("\n")
	content.WriteString(divider)
	content.WriteString("\n")
	content.WriteString(body)

	return boxStyle.Width(boxWidth).Render(content.String())
}

func renderBinding(b key.Binding) string {
	help := b.Help()
	return renderKeyDesc(help.Key, help.Desc)
}

func renderKeyDesc(key, desc string) string {
	return keyStyle.Render(key) + descStyle.Render(desc) + "\n"
}

// renderSearchContent renders the search mode help.
func (m Model) renderSearchContent() string {
	// Column style with right margin for spacing
	columnStyle := lipgloss.NewStyle().MarginRight(4)

	// Navigation column
	var navCol strings.Builder
	navCol.WriteString(sectionStyle.Render("Navigation"))
	navCol.WriteString("\n")
	navCol.WriteString(renderBinding(keys.Search.Left))
	navCol.WriteString(renderBinding(keys.Search.Right))
	navCol.WriteString(renderBinding(keys.Search.Up))
	navCol.WriteString(renderBinding(keys.Search.Down))
	navCol.WriteString(renderBinding(keys.Search.FocusSearch))
	navCol.WriteString(renderBinding(keys.Search.Blur))

	// Actions column
	var actionsCol strings.Builder
	actionsCol.WriteString(sectionStyle.Render("Actions"))
	actionsCol.WriteString("\n")
	actionsCol.WriteString(renderBinding(keys.Search.OpenTree))
	actionsCol.WriteString(renderBinding(keys.Search.Yank))
	actionsCol.WriteString(renderBinding(keys.Search.SaveColumn))

	// General column
	var generalCol strings.Builder
	generalCol.WriteString(sectionStyle.Render("General"))
	generalCol.WriteString("\n")
	generalCol.WriteString(renderBinding(keys.Search.SwitchMode))
	generalCol.WriteString(renderBinding(keys.Search.Help))
	generalCol.WriteString(renderBinding(keys.Search.Quit))

	// Join columns horizontally, aligned at top
	keybindingColumns := lipgloss.JoinHorizontal(
		lipgloss.Top,
		columnStyle.Render(navCol.String()),
		columnStyle.Render(actionsCol.String()),
		generalCol.String(),
	)

	// BQL Syntax section - two columns for fields/operators
	bqlStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)
	bqlLabelStyle := lipgloss.NewStyle().Foreground(styles.TextSecondaryColor).Width(12)
	bqlValueStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)

	// Fields column - use shared BQL data
	var fieldsCol strings.Builder
	fieldsCol.WriteString(sectionStyle.Render("BQL Fields"))
	fieldsCol.WriteString("\n")
	for _, f := range BQLFields() {
		// For compact display in overlay, use shorter values for some fields
		values := f.Values
		switch f.Name {
		case "label", "title":
			values = "string (~ for contains)"
		case "created", "updated":
			values = "today, -7d"
		}
		fieldsCol.WriteString(bqlLabelStyle.Render(f.Name) + bqlValueStyle.Render(values) + "\n")
	}

	// Operators column - use shared BQL data
	var opsCol strings.Builder
	opsCol.WriteString(sectionStyle.Render("BQL Operators"))
	opsCol.WriteString("\n")
	for _, op := range BQLOperators() {
		// For compact display, use shorter descriptions
		symbol := op.Symbol
		desc := op.Desc
		switch symbol {
		case "=  !=":
			// keep as is
		case "<  >  <=  >=":
			symbol = "<  >"
			desc = "comparison"
		case "~  !~":
			desc = "contains"
		case "in (a, b, c)":
			symbol = "in"
			desc = "match any: in (a, b)"
		case "and  or  not":
			symbol = "and or"
			desc = "logical"
		case "expand":
			desc = "up, down, all"
		case "depth":
			desc = "1-10 or * (unlimited)"
		}
		opsCol.WriteString(bqlLabelStyle.Render(symbol) + bqlValueStyle.Render(desc) + "\n")
	}
	// Add "not" separately for compact view
	opsCol.WriteString(bqlLabelStyle.Render("not") + bqlValueStyle.Render("negation") + "\n")

	// Join BQL columns
	bqlColumns := lipgloss.JoinHorizontal(
		lipgloss.Top,
		columnStyle.Render(fieldsCol.String()),
		opsCol.String(),
	)

	// Examples section - use shared BQL data (subset for compact view)
	var examplesCol strings.Builder
	examplesCol.WriteString(sectionStyle.Render("Examples"))
	examplesCol.WriteString("\n")
	examples := BQLExamples()
	// Show only a few examples in compact overlay (skip simple ones)
	compactExamples := []string{
		examples[1], // "status = open and ready = true"
		examples[3], // "type in (bug, task) and status != closed"
		examples[5], // "created >= -7d order by priority"
		examples[7], // "type = epic expand down"
	}
	for _, ex := range compactExamples {
		examplesCol.WriteString(bqlStyle.Render(ex) + "\n")
	}

	// Calculate box width based on widest section
	keybindingsWidth := lipgloss.Width(keybindingColumns)
	bqlWidth := lipgloss.Width(bqlColumns)
	examplesWidth := lipgloss.Width(examplesCol.String())
	columnsWidth := max(keybindingsWidth, bqlWidth, examplesWidth)
	boxWidth := columnsWidth + 4 // Add horizontal padding (2 each side)

	// Build body content with padding
	allContent := keybindingColumns + "\n" + bqlColumns + "\n" + examplesCol.String() + "\n" + footerStyle.Render("Press ? or Esc to close")
	body := contentStyle.Render(allContent)

	// Divider spans full box width
	divider := dividerStyle.Render(strings.Repeat("─", boxWidth))

	// Build final content: title, divider, body
	var content strings.Builder
	content.WriteString(titleStyle.Render("Search Mode Help"))
	content.WriteString("\n")
	content.WriteString(divider)
	content.WriteString("\n")
	content.WriteString(body)

	return boxStyle.Width(boxWidth).Render(content.String())
}

// renderTreeContent renders the tree sub-mode help.
func (m Model) renderTreeContent() string {
	// Column style with right margin for spacing
	columnStyle := lipgloss.NewStyle().MarginRight(4)

	// Navigation column - tree-specific navigation
	var navCol strings.Builder
	navCol.WriteString(sectionStyle.Render("Navigation"))
	navCol.WriteString("\n")
	navCol.WriteString(renderKeyDesc("j/↓", "move cursor down"))
	navCol.WriteString(renderKeyDesc("k/↑", "move cursor up"))
	navCol.WriteString(renderKeyDesc("l/Tab", "focus details panel"))
	navCol.WriteString(renderKeyDesc("h", "focus tree panel"))

	// Tree Actions column
	var actionsCol strings.Builder
	actionsCol.WriteString(sectionStyle.Render("Tree Actions"))
	actionsCol.WriteString("\n")
	actionsCol.WriteString(renderKeyDesc("Enter", "refocus on node"))
	actionsCol.WriteString(renderKeyDesc("u", "go back (prev root)"))
	actionsCol.WriteString(renderKeyDesc("U", "go to original root"))
	actionsCol.WriteString(renderKeyDesc("d", "toggle direction"))
	actionsCol.WriteString(renderKeyDesc("m", "toggle mode (deps/children)"))
	actionsCol.WriteString(renderKeyDesc("y", "copy issue ID"))

	// General column
	var generalCol strings.Builder
	generalCol.WriteString(sectionStyle.Render("General"))
	generalCol.WriteString("\n")
	generalCol.WriteString(renderKeyDesc("/", "switch to list mode"))
	generalCol.WriteString(renderKeyDesc("Ctrl+Space", "switch mode"))
	generalCol.WriteString(renderKeyDesc("Esc", "return to kanban"))
	generalCol.WriteString(renderKeyDesc("?", "toggle this help"))

	// Join columns horizontally, aligned at top
	columns := lipgloss.JoinHorizontal(
		lipgloss.Top,
		columnStyle.Render(navCol.String()),
		columnStyle.Render(actionsCol.String()),
		generalCol.String(),
	)

	// Calculate box width based on columns content
	columnsWidth := lipgloss.Width(columns)
	boxWidth := columnsWidth + 4 // Add horizontal padding (2 each side)

	// Build body content with padding
	body := contentStyle.Render(columns + "\n" + footerStyle.Render("Press ? or Esc to close"))

	// Divider spans full box width
	divider := dividerStyle.Render(strings.Repeat("─", boxWidth))

	// Build final content: title, divider, body
	var content strings.Builder
	content.WriteString(titleStyle.Render("Tree Mode Help"))
	content.WriteString("\n")
	content.WriteString(divider)
	content.WriteString("\n")
	content.WriteString(body)

	return boxStyle.Width(boxWidth).Render(content.String())
}
