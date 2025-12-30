// Package details contains the issue detail view component.
package details

import (
	"fmt"
	"strings"
	"time"

	"github.com/zjrosen/perles/internal/beads"
	"github.com/zjrosen/perles/internal/bql"
	"github.com/zjrosen/perles/internal/keys"
	"github.com/zjrosen/perles/internal/ui/shared/markdown"
	"github.com/zjrosen/perles/internal/ui/styles"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
)

// Layout constants for two-column view.
const (
	minTwoColumnWidth  = 100 // Below this, use single-column layout
	contentColWidth    = 80  // Preferred fixed width for content column
	metadataColWidth   = 34  // Fixed width for metadata column
	metadataDividerLen = 29  // Visual divider length (extended for full timestamps)
	columnGap          = 2   // Gap between columns
)

// DependencyItem holds loaded dependency data for display.
type DependencyItem struct {
	Issue    *beads.Issue // Full issue data (nil if load failed)
	ID       string       // Always available (from BlockedBy/Blocks/DiscoveredFrom/Discovered)
	Category string       // "blocked_by", "blocks", "children", "discovered_from", or "discovered"
}

// Messages emitted by the details view for the app to handle.

// OpenPriorityPickerMsg requests opening the priority picker.
type OpenPriorityPickerMsg struct {
	IssueID string
	Current beads.Priority
}

// OpenStatusPickerMsg requests opening the status picker.
type OpenStatusPickerMsg struct {
	IssueID string
	Current beads.Status
}

// DeleteIssueMsg requests deletion of the current issue.
type DeleteIssueMsg struct {
	IssueID   string
	IssueType beads.IssueType
}

// NavigateToDependencyMsg requests navigation to a dependency's detail view.
type NavigateToDependencyMsg struct {
	IssueID string
}

// OpenLabelEditorMsg requests opening the label editor modal.
type OpenLabelEditorMsg struct {
	IssueID string
	Labels  []string
}

// OpenEditMenuMsg requests opening the edit menu.
type OpenEditMenuMsg struct {
	IssueID  string
	Labels   []string
	Priority beads.Priority
	Status   beads.Status
}

// FocusPane represents which pane has focus in the details view.
type FocusPane int

const (
	FocusContent  FocusPane = iota // Left column (markdown viewport)
	FocusMetadata                  // Right column (dependencies)
)

// Model holds the detail view state.
type Model struct {
	issue              beads.Issue
	viewport           viewport.Model
	mdRenderer         *markdown.Renderer
	markdownStyle      string // "dark" or "light"
	width              int
	height             int
	ready              bool
	focusPane          FocusPane
	dependencies       []DependencyItem
	selectedDependency int // Index into dependencies slice
	executor           bql.BQLExecutor
	comments           []beads.Comment
	commentLoader      beads.BeadsClient
	commentsLoaded     bool
	commentsError      error
}

// New creates a new detail view.
// The optional loader parameter enables loading full issue data for dependencies.
// The optional commentLoader enables loading comments for the issue.
// Pass *beads.Client for both (it implements both interfaces); nil disables loading.
func New(issue beads.Issue, executor bql.BQLExecutor, commentLoader beads.BeadsClient) Model {
	m := Model{
		issue:         issue,
		executor:      executor,
		commentLoader: commentLoader,
		markdownStyle: "dark", // Default, will be overridden by SetMarkdownStyle
	}
	m.loadDependencies()
	m.loadComments()
	return m
}

// SetMarkdownStyle sets the markdown rendering style ("dark" or "light").
func (m Model) SetMarkdownStyle(style string) Model {
	m.markdownStyle = style
	// Clear renderer to force recreation with new style
	m.mdRenderer = nil
	return m
}

// SetSize updates dimensions and initializes viewport.
func (m Model) SetSize(width, height int) Model {
	m.width = width
	m.height = height

	// Calculate column widths first (needed for header height calculation)
	leftColWidth, _ := m.calculateColumnWidths()

	// Calculate actual header height based on title wrapping
	headerHeight := m.calculateHeaderHeight(leftColWidth)
	footerHeight := 1 // Footer is a single line
	viewportHeight := max(height-headerHeight-footerHeight, 1)

	// Initialize or update markdown renderer (uses left column width)
	if m.mdRenderer == nil || m.mdRenderer.Width() != leftColWidth {
		if r, err := markdown.New(leftColWidth, m.markdownStyle); err == nil {
			m.mdRenderer = r
		}
	}

	// Viewport width matches left column width
	viewportWidth := leftColWidth
	if !m.useTwoColumnLayout() {
		viewportWidth = m.width
	}

	if !m.ready {
		m.viewport = viewport.New(viewportWidth, viewportHeight)
		m.viewport.SetContent(m.renderLeftColumn())
		m.viewport.GotoTop()
		m.ready = true
	} else {
		m.viewport.Width = viewportWidth
		m.viewport.Height = viewportHeight
		m.viewport.SetContent(m.renderLeftColumn())
		// Scroll position preserved - GotoTop() removed to enable scroll restoration
	}

	return m
}

// useTwoColumnLayout returns true if the terminal is wide enough for two columns.
func (m Model) useTwoColumnLayout() bool {
	return m.width >= minTwoColumnWidth
}

// calculateColumnWidths returns the left and right column widths based on terminal size.
// Uses fixed widths when there's enough space, otherwise uses fixed sidebar minimum.
func (m Model) calculateColumnWidths() (leftWidth, rightWidth int) {
	availableWidth := max(m.width, 10)

	if !m.useTwoColumnLayout() {
		return availableWidth - columnGap, 0
	}

	leftWidth = availableWidth - metadataColWidth - columnGap
	return leftWidth, metadataColWidth
}

// calculateHeaderHeight returns the actual height of the header based on title wrapping.
func (m Model) calculateHeaderHeight(colWidth int) int {
	if colWidth < 1 {
		colWidth = 1
	}

	// Calculate visual width of title line to match renderHeader() format:
	// [Type][Priority][ID] Title
	// Example: [T][P2][perles-abc] Some title text
	typeText := styles.GetTypeIndicator(m.issue.Type) // e.g., "[T]"
	priorityText := fmt.Sprintf("[P%d]", m.issue.Priority)
	idText := fmt.Sprintf("[%s]", m.issue.ID)
	// Visual width = type + priority + id + space + title
	titleLineWidth := len(typeText) + len(priorityText) + len(idText) + 1 + len(m.issue.TitleText)
	titleLines := (titleLineWidth + colWidth - 1) / colWidth // Ceiling division

	if m.useTwoColumnLayout() {
		// Two-column: title + 1 blank line (\n\n = end of title + blank line)
		return titleLines + 1
	}

	// Single-column: title + meta line + optional labels line
	// Meta line is roughly fixed width, labels vary
	lines := titleLines + 1 // title + meta
	if len(m.issue.Labels) > 0 {
		lines++ // labels line
	}
	return lines
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Common.Left):
			// Move focus left (to content pane)
			if m.focusPane == FocusMetadata {
				m.focusPane = FocusContent
			}
			return m, nil
		case key.Matches(msg, keys.Common.Right):
			// Move focus right (to metadata/dependencies pane) - only if there are dependencies
			if m.focusPane == FocusContent && len(m.dependencies) > 0 {
				m.focusPane = FocusMetadata
				if m.selectedDependency < 0 {
					m.selectedDependency = 0 // Select first dependency
				}
			}
			return m, nil
		case key.Matches(msg, keys.Common.Down), key.Matches(msg, keys.Component.Next):
			if m.focusPane == FocusMetadata && len(m.dependencies) > 0 {
				// Navigate dependencies
				m.selectedDependency++
				if m.selectedDependency >= len(m.dependencies) {
					m.selectedDependency = 0 // wrap around
				}
				return m, nil
			}
			m.viewport.ScrollDown(1)
		case key.Matches(msg, keys.Common.Up), key.Matches(msg, keys.Component.Prev):
			if m.focusPane == FocusMetadata && len(m.dependencies) > 0 {
				// Navigate dependencies
				m.selectedDependency--
				if m.selectedDependency < 0 {
					m.selectedDependency = len(m.dependencies) - 1 // wrap around
				}
				return m, nil
			}
			m.viewport.ScrollUp(1)
		case key.Matches(msg, keys.Component.GotoTop):
			m.viewport.GotoTop()
		case key.Matches(msg, keys.Component.GotoBottom):
			m.viewport.GotoBottom()
		case key.Matches(msg, keys.Common.Enter):
			// Navigate to selected dependency (only when metadata focused)
			if m.focusPane == FocusMetadata && m.selectedDependency >= 0 && m.selectedDependency < len(m.dependencies) {
				dep := m.dependencies[m.selectedDependency]
				return m, func() tea.Msg {
					return NavigateToDependencyMsg{IssueID: dep.ID}
				}
			}
			return m, nil
		case key.Matches(msg, keys.Component.DelAction):
			return m, func() tea.Msg {
				return DeleteIssueMsg{
					IssueID:   m.issue.ID,
					IssueType: m.issue.Type,
				}
			}
		case key.Matches(msg, keys.Component.EditAction):
			// Open edit menu
			return m, func() tea.Msg {
				return OpenEditMenuMsg{
					IssueID:  m.issue.ID,
					Labels:   m.issue.Labels,
					Priority: m.issue.Priority,
					Status:   m.issue.Status,
				}
			}
		}
	}

	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// UpdateStatus updates the displayed status after a change.
func (m Model) UpdateStatus(status beads.Status) Model {
	m.issue.Status = status
	return m
}

// UpdateLabels updates the displayed labels after a change.
func (m Model) UpdateLabels(labels []string) Model {
	m.issue.Labels = labels
	return m
}

// UpdatePriority updates the displayed priority after a change.
func (m Model) UpdatePriority(priority beads.Priority) Model {
	m.issue.Priority = priority
	return m
}

// IsOnLeftEdge returns true if focus is on the leftmost position (content pane or no deps).
func (m Model) IsOnLeftEdge() bool {
	return m.focusPane == FocusContent || len(m.dependencies) == 0
}

// View renders the detail view.
func (m Model) View() string {
	if !m.ready || m.width == 0 {
		return "Loading..."
	}

	// Build the modal
	header := m.renderHeader()
	footer := m.renderFooter()

	var body string
	if m.useTwoColumnLayout() {
		// Two-column layout: left (header + scrollable content) + right (static metadata)
		// Include header in left column so both columns start at top
		leftCol := header + m.viewport.View()
		rightCol := m.renderMetadataColumn()

		// Get calculated column widths (fixed or percentage-based)
		leftWidth, rightWidth := m.calculateColumnWidths()

		// Style columns with calculated widths
		leftStyle := lipgloss.NewStyle().Width(leftWidth)
		rightStyle := lipgloss.NewStyle().Width(rightWidth)

		// Render left column first to get actual line count after width wrapping
		renderedLeftCol := leftStyle.Render(leftCol)
		leftLines := strings.Count(renderedLeftCol, "\n") + 1
		dividerHeight := leftLines

		// Truncate right column if it exceeds the available height
		rightLineSlice := strings.Split(rightCol, "\n")
		if len(rightLineSlice) > leftLines {
			rightLineSlice = rightLineSlice[:leftLines]
			rightCol = strings.Join(rightLineSlice, "\n")
		}

		// Render vertical divider with consistent spacing per line
		dividerStyle := lipgloss.NewStyle().Foreground(styles.BorderDefaultColor)
		var dividerLines []string
		for range dividerHeight {
			dividerLines = append(dividerLines, " │ ")
		}
		verticalDivider := dividerStyle.Render(strings.Join(dividerLines, "\n"))

		// Join columns horizontally, aligned at top
		content := lipgloss.JoinHorizontal(
			lipgloss.Top,
			renderedLeftCol,
			verticalDivider,
			rightStyle.Render(rightCol),
		)

		// Calculate left padding to center the content
		contentWidth := leftWidth + 3 + rightWidth // 3 for " │ "
		leftPadding := max((m.width-contentWidth)/2, 0)

		contentStyle := lipgloss.NewStyle().PaddingLeft(leftPadding)

		// Create footer row with divider extending down
		footerLeftStyle := lipgloss.NewStyle().Width(leftWidth)
		footerRightStyle := lipgloss.NewStyle().Width(rightWidth)
		footerDivider := dividerStyle.Render(" │ ")
		footerRow := lipgloss.JoinHorizontal(
			lipgloss.Top,
			footerLeftStyle.Render(footer),
			footerDivider,
			footerRightStyle.Render(""),
		)

		body = lipgloss.JoinVertical(lipgloss.Left, contentStyle.Render(content), contentStyle.Render(footerRow))
	} else {
		// Single-column fallback (narrow terminals)
		content := m.viewport.View()
		body = lipgloss.JoinVertical(lipgloss.Left, header, content, footer)
	}

	// Apply modal styling
	modalStyle := lipgloss.NewStyle().
		Padding(0, 1)

	return modalStyle.Render(body)
}

// renderHeader renders the issue header.
func (m Model) renderHeader() string {
	issue := m.issue

	// Title line - uses same style as column list: [Type][Priority][ID] Title
	typeText := styles.GetTypeIndicator(issue.Type)
	typeStyle := styles.GetTypeStyle(issue.Type)
	priorityText := fmt.Sprintf("[P%d]", issue.Priority)
	priorityStyle := styles.GetPriorityStyle(issue.Priority)
	idStyle := lipgloss.NewStyle().Foreground(styles.TextSecondaryColor)
	issueId := fmt.Sprintf("[%s]", issue.ID)

	titleLine := fmt.Sprintf("%s%s%s %s",
		typeStyle.Render(typeText),
		priorityStyle.Render(priorityText),
		idStyle.Render(issueId),
		issue.TitleText,
	)

	if m.useTwoColumnLayout() {
		// Two-column: just title with blank line below (metadata in right column)
		return titleLine + "\n\n"
	}

	// Single-column: include inline metadata (type and priority already in title line)
	statusStyle := getStatusStyle(issue.Status)
	metaLine := fmt.Sprintf("\nStatus: %s", statusStyle.Render(formatStatus(issue.Status)))

	// Labels line
	var labelsLine string
	if len(issue.Labels) > 0 {
		labelsLine = "Labels: " + strings.Join(issue.Labels, ", ")
	}

	lines := []string{titleLine, metaLine}
	if labelsLine != "" {
		lines = append(lines, labelsLine)
	}

	return strings.Join(lines, "\n") + "\n"
}

// renderLeftColumn renders the left column content (description + comments).
// Dependencies are now rendered in the right metadata column.
func (m Model) renderLeftColumn() string {
	issue := m.issue
	var sb strings.Builder

	// Description with markdown rendering
	if issue.DescriptionText != "" {
		rendered := m.renderDescription()
		sb.WriteString(rendered)
		sb.WriteString("\n")
	} else {
		// Empty state for no description
		emptyStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor).Italic(true)
		sb.WriteString(emptyStyle.Render("No description"))
		sb.WriteString("\n")
	}

	// Acceptance Criteria
	sb.WriteString(m.renderMarkdownSection("Acceptance Criteria", issue.AcceptanceCriteria))

	// Design
	sb.WriteString(m.renderMarkdownSection("Design", issue.Design))

	// Notes
	sb.WriteString(m.renderMarkdownSection("Notes", issue.Notes))

	// Comments error handling
	if m.commentsError != nil {
		sb.WriteString("\n")
		errorStyle := lipgloss.NewStyle().Foreground(styles.StatusErrorColor)
		sb.WriteString(errorStyle.Render("Failed to load comments"))
		sb.WriteString("\n")
	}

	// Comments section
	if len(m.comments) > 0 {
		sb.WriteString("\n")
		headerStyle := lipgloss.NewStyle().Bold(true)
		sb.WriteString(headerStyle.Render("Comments"))
		sb.WriteString("\n\n")

		// Calculate wrap width based on content column
		wrapWidth := contentColWidth - 2 // Leave some margin
		if m.width > 0 && m.width < contentColWidth {
			wrapWidth = m.width - 4
		}

		commentHeaderStyle := lipgloss.NewStyle().Foreground(styles.TextSecondaryColor)

		for _, c := range m.comments {
			// [author] timestamp - styled with secondary color
			// Use same format as metadata timestamps for consistency
			header := fmt.Sprintf("[%s] %s",
				c.Author,
				c.CreatedAt.Format("2006-01-02 15:04:05"))
			sb.WriteString(commentHeaderStyle.Render(header))
			sb.WriteString("\n")
			// Wrap comment text to fit column width
			wrappedText := wordwrap.String(c.Text, wrapWidth)
			sb.WriteString(wrappedText)
			sb.WriteString("\n\n")
		}
	}

	return sb.String()
}

// renderMetadataColumn renders the right column metadata panel.
// This will be used as the static right column in the two-column layout.
func (m Model) renderMetadataColumn() string {
	issue := m.issue
	var sb strings.Builder

	labelStyle := lipgloss.NewStyle().
		Foreground(styles.TextSecondaryColor).
		Width(10)

	dividerStyle := lipgloss.NewStyle().Foreground(styles.BorderDefaultColor)
	divider := dividerStyle.Render(strings.Repeat("─", metadataDividerLen))

	valueStyle := lipgloss.NewStyle()

	indent := " "
	indentedDivider := indent + divider

	// Type (read-only)
	sb.WriteString(indent)
	sb.WriteString(labelStyle.Render("Type"))
	sb.WriteString(getTypeStyle(issue.Type).Render(formatType(issue.Type)))
	sb.WriteString("\n")
	sb.WriteString(indentedDivider)
	sb.WriteString("\n")

	// Priority (read-only display, edit via e)
	sb.WriteString(indent)
	sb.WriteString(labelStyle.Render("Priority"))
	sb.WriteString(getPriorityStyle(issue.Priority).Render(fmt.Sprintf("P%d", issue.Priority)))
	sb.WriteString("\n")

	// Status (read-only display, edit via e)
	sb.WriteString(indent)
	sb.WriteString(labelStyle.Render("Status"))
	sb.WriteString(getStatusStyle(issue.Status).Render(formatStatus(issue.Status)))
	sb.WriteString("\n")
	sb.WriteString(indentedDivider)
	sb.WriteString("\n")

	// Assignee (only show if non-empty)
	if issue.Assignee != "" {
		sb.WriteString(indent)
		sb.WriteString(labelStyle.Render("Assignee"))
		sb.WriteString(valueStyle.Render(issue.Assignee))
		sb.WriteString("\n")
		sb.WriteString(indentedDivider)
		sb.WriteString("\n")
	}

	// MolType (if set)
	if issue.MolType != "" {
		sb.WriteString(indent)
		sb.WriteString(labelStyle.Render("Mol Type"))
		sb.WriteString(valueStyle.Render(issue.MolType))
		sb.WriteString("\n")
	}

	// MolType (if set)
	if issue.RoleType != "" {
		sb.WriteString(indent)
		sb.WriteString(labelStyle.Render("Role Type"))
		sb.WriteString(valueStyle.Render(issue.RoleType))
		sb.WriteString("\n")
	}

	if issue.MolType != "" || issue.RoleType != "" {
		sb.WriteString(indentedDivider)
		sb.WriteString("\n")
	}

	// CreatedBy (if set)
	if issue.CreatedBy != "" {
		sb.WriteString(indent)
		sb.WriteString(labelStyle.Render("Creator"))
		sb.WriteString(valueStyle.Render(issue.CreatedBy))
		sb.WriteString("\n")
	}

	// Timestamps
	sb.WriteString(indent)
	sb.WriteString(labelStyle.Render("Created"))
	sb.WriteString(valueStyle.Render(issue.CreatedAt.Format("2006-01-02 15:04:05")))
	sb.WriteString("\n")

	if !issue.UpdatedAt.IsZero() && issue.UpdatedAt != issue.CreatedAt {
		sb.WriteString(indent)
		sb.WriteString(labelStyle.Render("Updated"))
		sb.WriteString(valueStyle.Render(issue.UpdatedAt.Format("2006-01-02 15:04:05")))
		sb.WriteString("\n")
	}

	// Closed timestamp and Duration (only for closed issues)
	if !issue.ClosedAt.IsZero() {
		sb.WriteString(indent)
		sb.WriteString(labelStyle.Render("Closed"))
		sb.WriteString(valueStyle.Render(issue.ClosedAt.Format("2006-01-02 15:04:05")))
		sb.WriteString("\n")

		// Duration from Created to Closed
		duration := issue.ClosedAt.Sub(issue.CreatedAt)
		sb.WriteString(indent)
		sb.WriteString(labelStyle.Render("Duration"))
		sb.WriteString(valueStyle.Render(formatDuration(duration)))
		sb.WriteString("\n")
	}

	// Labels section
	if len(issue.Labels) > 0 {
		sb.WriteString(indentedDivider)
		sb.WriteString("\n")
		sb.WriteString(indent)
		sb.WriteString(labelStyle.Render("Labels"))
		sb.WriteString("\n")

		labelIndent := indent + " "
		maxLabelWidth := metadataColWidth - len(labelIndent) - 4
		for _, label := range issue.Labels {
			// Split long labels across multiple lines, each properly indented
			for len(label) > 0 {
				lineLen := min(len(label), maxLabelWidth)
				sb.WriteString(labelIndent + label[:lineLen] + "\n")
				label = label[lineLen:]
			}
		}
	}

	// Dependencies section (rendered with board-style formatting)
	depSection := m.renderDependenciesSection()
	if depSection != "" {
		sb.WriteString(depSection)
	}

	return sb.String()
}

// renderDescription renders the issue description with markdown styling.
func (m Model) renderDescription() string {
	if m.issue.DescriptionText == "" {
		return ""
	}

	// Try markdown rendering, fall back to plain text
	if m.mdRenderer != nil {
		if rendered, err := m.mdRenderer.Render(m.issue.DescriptionText); err == nil {
			return strings.TrimSpace(rendered)
		}
	}

	// Fallback: plain text with header
	return "Description:\n" + m.issue.DescriptionText
}

// renderMarkdownSection renders a titled markdown section.
// Returns empty string if content is empty.
func (m Model) renderMarkdownSection(title, content string) string {
	if content == "" {
		return ""
	}

	var sb strings.Builder

	// Spacing before section
	sb.WriteString("\n")

	// Header
	headerStyle := lipgloss.NewStyle().Bold(true)
	sb.WriteString(headerStyle.Render(title))
	sb.WriteString("\n\n")

	// Content
	if m.mdRenderer != nil {
		if rendered, err := m.mdRenderer.Render(content); err == nil {
			sb.WriteString(strings.TrimSpace(rendered))
			sb.WriteString("\n")
			return sb.String()
		}
	}

	// Fallback
	sb.WriteString(content)
	sb.WriteString("\n")
	return sb.String()
}

// renderDependencyItem renders a single dependency with board-style formatting.
// Format: [T][P2][id] (compact, no title to avoid overlap)
// The selected parameter controls the ">" prefix for navigation.
func (m Model) renderDependencyItem(item DependencyItem, selected bool) string {
	prefix := "  "
	if selected && m.focusPane == FocusMetadata {
		prefix = " " + styles.SelectionIndicatorStyle.Render(">")
	}
	idStyle := lipgloss.NewStyle().Foreground(styles.TextSecondaryColor)

	if item.Issue == nil {
		// Fallback: just show ID if load failed
		return prefix + idStyle.Render(item.ID)
	}

	// Use board styling functions - compact format without title
	typeText := styles.GetTypeIndicator(item.Issue.Type)
	typeStyle := styles.GetTypeStyle(item.Issue.Type)
	priorityText := fmt.Sprintf("[P%d]", item.Issue.Priority)
	priorityStyle := styles.GetPriorityStyle(item.Issue.Priority)

	return fmt.Sprintf("%s%s%s%s %s",
		prefix,
		typeStyle.Render(typeText),
		priorityStyle.Render(priorityText),
		idStyle.Render("["+item.ID+"]"),
		renderStatusIndicator(item.Issue.Status),
	)
}

// renderDependenciesSection renders the dependencies section for the metadata column.
// Groups dependencies by category (blocked_by, blocks, related).
func (m Model) renderDependenciesSection() string {
	if len(m.dependencies) == 0 {
		return ""
	}

	var sb strings.Builder

	labelStyle := lipgloss.NewStyle().
		Foreground(styles.TextSecondaryColor).
		Width(10)

	dividerStyle := lipgloss.NewStyle().Foreground(styles.BorderDefaultColor)
	divider := dividerStyle.Render(strings.Repeat("─", metadataDividerLen))

	indent := " "
	indentedDivider := indent + divider

	// Group by category
	var children, blockedBy, blocks, discoveredFrom, discovered []DependencyItem
	for _, dep := range m.dependencies {
		switch dep.Category {
		case "children":
			children = append(children, dep)
		case "blocked_by":
			blockedBy = append(blockedBy, dep)
		case "blocks":
			blocks = append(blocks, dep)
		case "discovered_from":
			discoveredFrom = append(discoveredFrom, dep)
		case "discovered":
			discovered = append(discovered, dep)
		}
	}

	depIndex := 0 // Track overall index for selection

	if len(blockedBy) > 0 {
		sb.WriteString(indentedDivider)
		sb.WriteString("\n")
		sb.WriteString(indent)
		sb.WriteString(labelStyle.Render("Blocked by"))
		sb.WriteString("\n")
		for _, dep := range blockedBy {
			sb.WriteString(m.renderDependencyItem(dep, depIndex == m.selectedDependency))
			sb.WriteString("\n")
			depIndex++
		}
	}

	if len(blocks) > 0 {
		sb.WriteString(indentedDivider)
		sb.WriteString("\n")
		sb.WriteString(indent)
		sb.WriteString(labelStyle.Render("Blocks"))
		sb.WriteString("\n")
		for _, dep := range blocks {
			sb.WriteString(m.renderDependencyItem(dep, depIndex == m.selectedDependency))
			sb.WriteString("\n")
			depIndex++
		}
	}

	if len(children) > 0 {
		sb.WriteString(indentedDivider)
		sb.WriteString("\n")
		sb.WriteString(indent)
		sb.WriteString(labelStyle.Render("Children"))
		sb.WriteString("\n")
		for _, dep := range children {
			sb.WriteString(m.renderDependencyItem(dep, depIndex == m.selectedDependency))
			sb.WriteString("\n")
			depIndex++
		}
	}

	if len(discoveredFrom) > 0 {
		sb.WriteString(indentedDivider)
		sb.WriteString("\n")
		sb.WriteString(indent)
		sb.WriteString(labelStyle.Render("Disc. from"))
		sb.WriteString("\n")
		for _, dep := range discoveredFrom {
			sb.WriteString(m.renderDependencyItem(dep, depIndex == m.selectedDependency))
			sb.WriteString("\n")
			depIndex++
		}
	}

	if len(discovered) > 0 {
		sb.WriteString(indentedDivider)
		sb.WriteString("\n")
		sb.WriteString(indent)
		sb.WriteString(labelStyle.Render("Discovered"))
		sb.WriteString("\n")
		for _, dep := range discovered {
			sb.WriteString(m.renderDependencyItem(dep, depIndex == m.selectedDependency))
			sb.WriteString("\n")
			depIndex++
		}
	}

	return sb.String()
}

// renderFooter renders the keybinding hints.
func (m Model) renderFooter() string {
	footerStyle := lipgloss.NewStyle().
		Foreground(styles.TextDescriptionColor)

	scrollPercent := ""
	if m.viewport.TotalLineCount() > m.viewport.Height {
		scrollPercent = fmt.Sprintf(" %3.0f%%", m.viewport.ScrollPercent()*100)
	}

	return footerStyle.Render("[j/k] Scroll  [e] Edit Issue  [d] Delete Issue  [Esc] Back" + scrollPercent)
}

// getTypeStyle returns the style for an issue type.
func getTypeStyle(t beads.IssueType) lipgloss.Style {
	switch t {
	case beads.TypeBug:
		return styles.TypeBugStyle
	case beads.TypeFeature:
		return styles.TypeFeatureStyle
	case beads.TypeTask:
		return styles.TypeTaskStyle
	case beads.TypeEpic:
		return styles.TypeEpicStyle
	case beads.TypeChore:
		return styles.TypeChoreStyle
	case beads.TypeMolecule:
		return styles.TypeMoleculeStyle
	default:
		return lipgloss.NewStyle()
	}
}

// getPriorityStyle returns the style for a priority level.
func getPriorityStyle(p beads.Priority) lipgloss.Style {
	switch p {
	case beads.PriorityCritical:
		return styles.PriorityCriticalStyle
	case beads.PriorityHigh:
		return styles.PriorityHighStyle
	case beads.PriorityMedium:
		return styles.PriorityMediumStyle
	case beads.PriorityLow:
		return styles.PriorityLowStyle
	case beads.PriorityBacklog:
		return styles.PriorityBacklogStyle
	default:
		return lipgloss.NewStyle()
	}
}

// getStatusStyle returns the style for a status value.
func getStatusStyle(s beads.Status) lipgloss.Style {
	switch s {
	case beads.StatusOpen:
		return lipgloss.NewStyle().Foreground(styles.StatusOpenColor)
	case beads.StatusInProgress:
		return lipgloss.NewStyle().Foreground(styles.StatusInProgressColor)
	case beads.StatusClosed:
		return lipgloss.NewStyle().Foreground(styles.StatusClosedColor)
	default:
		return lipgloss.NewStyle()
	}
}

// renderStatusIndicator renders the status badge for dependency items.
// Matches the tree view format: ○ (open), ● (in progress), ✓ (closed).
func renderStatusIndicator(status beads.Status) string {
	switch status {
	case beads.StatusClosed:
		style := lipgloss.NewStyle().Foreground(styles.StatusClosedColor)
		return style.Render("✓")
	case beads.StatusInProgress:
		style := lipgloss.NewStyle().Foreground(styles.StatusInProgressColor)
		return style.Render("●")
	default:
		style := lipgloss.NewStyle().Foreground(styles.StatusOpenColor)
		return style.Render("○")
	}
}

// formatStatus returns the display name for a status (Title Case).
func formatStatus(s beads.Status) string {
	switch s {
	case beads.StatusOpen:
		return "Open"
	case beads.StatusInProgress:
		return "In Progress"
	case beads.StatusClosed:
		return "Closed"
	default:
		return string(s)
	}
}

// formatType returns the display name for an issue type (Title Case).
func formatType(t beads.IssueType) string {
	switch t {
	case beads.TypeBug:
		return "Bug"
	case beads.TypeFeature:
		return "Feature"
	case beads.TypeTask:
		return "Task"
	case beads.TypeEpic:
		return "Epic"
	case beads.TypeChore:
		return "Chore"
	case beads.TypeMolecule:
		return "Molecule"
	default:
		return string(t)
	}
}

// IssueID returns the ID of the currently displayed issue.
func (m Model) IssueID() string {
	return m.issue.ID
}

// YOffset returns the current viewport scroll position.
func (m Model) YOffset() int {
	return m.viewport.YOffset
}

// SetYOffset restores the viewport scroll position.
// The offset is clamped to valid range by the viewport.
func (m Model) SetYOffset(offset int) Model {
	m.viewport.SetYOffset(offset)
	return m
}

// loadDependencies populates the dependencies slice from the issue's
// BlockedBy, Blocks, Children, DiscoveredFrom, and Discovered fields. If a client is available,
// it fetches full issue data for each dependency.
func (m *Model) loadDependencies() {
	// Collect all dependency IDs with their categories
	// Order must match renderDependenciesSection: blocked_by, blocks, children, discovered_from, discovered
	var items []DependencyItem
	for _, id := range m.issue.BlockedBy {
		items = append(items, DependencyItem{ID: id, Category: "blocked_by"})
	}
	for _, id := range m.issue.Blocks {
		items = append(items, DependencyItem{ID: id, Category: "blocks"})
	}
	for _, id := range m.issue.Children {
		items = append(items, DependencyItem{ID: id, Category: "children"})
	}
	for _, id := range m.issue.DiscoveredFrom {
		items = append(items, DependencyItem{ID: id, Category: "discovered_from"})
	}
	for _, id := range m.issue.Discovered {
		items = append(items, DependencyItem{ID: id, Category: "discovered"})
	}

	if len(items) == 0 {
		m.dependencies = items
		return
	}

	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}

	query := bql.BuildIDQuery(ids)
	if query != "" {
		issues, err := m.executor.Execute(query)
		if err == nil {
			// Build a lookup map
			issueMap := make(map[string]*beads.Issue)
			for i := range issues {
				issueMap[issues[i].ID] = &issues[i]
			}

			// Populate Issue field for each item
			for i := range items {
				if issue, ok := issueMap[items[i].ID]; ok {
					items[i].Issue = issue
				}
			}
		}
		// On error, we keep items with nil Issue (fallback to ID display)
	}

	m.dependencies = items
}

// loadComments fetches comments for the current issue using the comment loader.
func (m *Model) loadComments() {
	if m.commentsLoaded {
		return
	}
	comments, err := m.commentLoader.GetComments(m.issue.ID)
	m.comments = comments
	m.commentsError = err
	m.commentsLoaded = true
}

// formatDuration returns a human-readable duration string.
// Shows the two largest non-zero units (e.g., "3d 4h", "2h 15m", "45m").
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}
