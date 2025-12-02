// Package details contains the issue detail view component.
package details

import (
	"fmt"
	"perles/internal/beads"
	"perles/internal/ui/board"
	"perles/internal/ui/markdown"
	"perles/internal/ui/styles"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Layout constants for two-column view.
const (
	minTwoColumnWidth  = 80 // Below this, use single-column layout
	contentColWidth    = 80 // Preferred fixed width for content column
	metadataColWidth   = 80 // Fixed width for metadata column (allows full titles)
	metadataDividerLen = 25 // Visual divider length (keeps compact appearance)
	columnGap          = 2  // Gap between columns
)

// DependencyLoader provides the ability to load issue data for dependencies.
// This interface allows for mocking in tests.
type DependencyLoader interface {
	ListIssuesByIds(ids []string) ([]beads.Issue, error)
}

// DependencyItem holds loaded dependency data for display.
type DependencyItem struct {
	Issue    *beads.Issue // Full issue data (nil if load failed)
	ID       string       // Always available (from BlockedBy/Blocks/Related)
	Category string       // "blocked_by", "blocks", or "related"
}

// FocusPane represents which pane has focus in the details view.
type FocusPane int

const (
	FocusContent  FocusPane = iota // Left column (markdown viewport)
	FocusMetadata                  // Right column (editable fields)
)

// MetadataField represents selectable (editable) fields in the metadata column.
type MetadataField int

const (
	FieldPriority MetadataField = iota
	FieldStatus
	FieldDependency // When focused, selectedDependency index applies
)

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

// Model holds the detail view state.
type Model struct {
	issue              beads.Issue
	viewport           viewport.Model
	mdRenderer         *markdown.Renderer
	width              int
	height             int
	ready              bool
	focusPane          FocusPane     // Which pane has focus
	selectedField      MetadataField // Which metadata field is selected (when FocusMetadata)
	dependencies       []DependencyItem
	selectedDependency int // Index into dependencies slice (when FieldDependency)
	loader             DependencyLoader
}

// New creates a new detail view.
// The optional loader parameter enables loading full issue data for dependencies.
// Pass *beads.Client or any DependencyLoader implementation; nil disables loading.
func New(issue beads.Issue, loader DependencyLoader) Model {
	m := Model{
		issue:  issue,
		loader: loader,
	}
	m.loadDependencies()
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
	viewportHeight := height - headerHeight - footerHeight

	if viewportHeight < 1 {
		viewportHeight = 1
	}

	// Initialize or update markdown renderer (uses left column width)
	if m.mdRenderer == nil || m.mdRenderer.Width() != leftColWidth {
		if r, err := markdown.New(leftColWidth); err == nil {
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
		m.viewport.GotoTop()
	}

	return m
}

// useTwoColumnLayout returns true if the terminal is wide enough for two columns.
func (m Model) useTwoColumnLayout() bool {
	return m.width >= minTwoColumnWidth
}

// calculateColumnWidths returns the left and right column widths based on terminal size.
// Uses fixed widths when there's enough space, otherwise falls back to percentage-based.
func (m Model) calculateColumnWidths() (leftWidth, rightWidth int) {
	// Content width accounts for modal padding and borders
	availableWidth := m.width
	if availableWidth < 10 {
		availableWidth = 10
	}

	// Total fixed width needed for two-column layout
	totalFixedWidth := contentColWidth + metadataColWidth + columnGap

	if !m.useTwoColumnLayout() {
		// Single column - full width
		return availableWidth, 0
	}

	if availableWidth >= totalFixedWidth {
		// Enough space for fixed widths
		return contentColWidth, metadataColWidth
	}

	// Percentage-based fallback (70/30 split)
	leftWidth = (availableWidth - columnGap) * 70 / 100
	rightWidth = availableWidth - leftWidth - columnGap
	return leftWidth, rightWidth
}

// calculateHeaderHeight returns the actual height of the header based on title wrapping.
func (m Model) calculateHeaderHeight(colWidth int) int {
	if colWidth < 1 {
		colWidth = 1
	}

	// Calculate visual width of title line to match renderHeader() format:
	// [Type][Priority][ID] Title
	// Example: [T][P2][perles-abc] Some title text
	typeText := board.GetTypeIndicator(m.issue.Type) // e.g., "[T]"
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
		switch msg.String() {
		case "h":
			// Move focus left (to content pane)
			if m.focusPane == FocusMetadata {
				m.focusPane = FocusContent
			}
			return m, nil
		case "l":
			// Move focus right (to metadata pane)
			if m.focusPane == FocusContent {
				m.focusPane = FocusMetadata
			}
			return m, nil
		case "j", "down", "ctrl+n":
			if m.focusPane == FocusMetadata {
				switch m.selectedField {
				case FieldPriority:
					m.selectedField = FieldStatus
				case FieldStatus:
					if len(m.dependencies) > 0 {
						m.selectedField = FieldDependency
						m.selectedDependency = 0
					} else {
						m.selectedField = FieldPriority // wrap around
					}
				case FieldDependency:
					if m.selectedDependency < len(m.dependencies)-1 {
						m.selectedDependency++
					} else {
						m.selectedField = FieldPriority // wrap to top
						m.selectedDependency = 0
					}
				}
				return m, nil
			}
			m.viewport.ScrollDown(1)
		case "k", "up", "ctrl+p":
			if m.focusPane == FocusMetadata {
				switch m.selectedField {
				case FieldPriority:
					if len(m.dependencies) > 0 {
						m.selectedField = FieldDependency
						m.selectedDependency = len(m.dependencies) - 1 // wrap to last dep
					} else {
						m.selectedField = FieldStatus // wrap around
					}
				case FieldStatus:
					m.selectedField = FieldPriority
				case FieldDependency:
					if m.selectedDependency > 0 {
						m.selectedDependency--
					} else {
						m.selectedField = FieldStatus // back to Status
					}
				}
				return m, nil
			}
			m.viewport.ScrollUp(1)
		case "g":
			m.viewport.GotoTop()
		case "G":
			m.viewport.GotoBottom()
		case "enter":
			if m.focusPane == FocusMetadata {
				// Check if on a dependency - emit navigation message
				if m.selectedField == FieldDependency && len(m.dependencies) > 0 {
					dep := m.dependencies[m.selectedDependency]
					return m, func() tea.Msg {
						return NavigateToDependencyMsg{IssueID: dep.ID}
					}
				}
				// Otherwise open field picker for Priority/Status
				return m, m.openFieldPicker()
			}
		case "d":
			return m, func() tea.Msg {
				return DeleteIssueMsg{
					IssueID:   m.issue.ID,
					IssueType: m.issue.Type,
				}
			}
		case "L":
			// Open label editor (shift+l to avoid conflict with 'l' column navigation)
			return m, func() tea.Msg {
				return OpenLabelEditorMsg{
					IssueID: m.issue.ID,
					Labels:  m.issue.Labels,
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

// IsOnLeftEdge returns true if focus is on the leftmost column (content pane).
func (m Model) IsOnLeftEdge() bool {
	return m.focusPane == FocusContent
}

// IsOnRightEdge returns true if focus is on the rightmost column (metadata pane).
func (m Model) IsOnRightEdge() bool {
	return m.focusPane == FocusMetadata
}

// openFieldPicker returns a command to open a picker for the currently selected field.
func (m Model) openFieldPicker() tea.Cmd {
	switch m.selectedField {
	case FieldPriority:
		return func() tea.Msg {
			return OpenPriorityPickerMsg{IssueID: m.issue.ID, Current: m.issue.Priority}
		}
	case FieldStatus:
		return func() tea.Msg {
			return OpenStatusPickerMsg{IssueID: m.issue.ID, Current: m.issue.Status}
		}
	}
	return nil
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
		for i := 0; i < dividerHeight; i++ {
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
		leftPadding := (m.width - contentWidth) / 2
		if leftPadding < 0 {
			leftPadding = 0
		}

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
	typeText := board.GetTypeIndicator(issue.Type)
	typeStyle := board.GetTypeStyle(issue.Type)
	priorityText := fmt.Sprintf("[P%d]", issue.Priority)
	priorityStyle := board.GetPriorityStyle(issue.Priority)
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
	metaLine := fmt.Sprintf("Status: %s", statusStyle.Render(formatStatus(issue.Status)))

	// Labels line
	var labelsLine string
	if len(issue.Labels) > 0 {
		labelsLine = "Labels: " + strings.Join(issue.Labels, ", ")
	}

	lines := []string{titleLine, metaLine}
	if labelsLine != "" {
		lines = append(lines, labelsLine)
	}

	return strings.Join(lines, "\n")
}

// renderLeftColumn renders the left column content (description only).
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

	// Type (read-only, shown at top)
	sb.WriteString(indent)
	sb.WriteString(labelStyle.Render("Type"))
	sb.WriteString(getTypeStyle(issue.Type).Render(formatType(issue.Type)))
	sb.WriteString("\n")
	sb.WriteString(indentedDivider)
	sb.WriteString("\n")

	// Helper to get selection prefix for editable fields
	prefix := func(field MetadataField) string {
		if m.focusPane == FocusMetadata && m.selectedField == field {
			return styles.SelectionIndicatorStyle.Render(">")
		}
		return " "
	}

	// Priority (editable)
	sb.WriteString(prefix(FieldPriority))
	sb.WriteString(labelStyle.Render("Priority"))
	sb.WriteString(getPriorityStyle(issue.Priority).Render(fmt.Sprintf("P%d", issue.Priority)))
	sb.WriteString("\n")

	// Status (editable)
	sb.WriteString(prefix(FieldStatus))
	sb.WriteString(labelStyle.Render("Status"))
	sb.WriteString(getStatusStyle(issue.Status).Render(formatStatus(issue.Status)))
	sb.WriteString("\n")
	sb.WriteString(indentedDivider)
	sb.WriteString("\n")

	// Timestamps
	sb.WriteString(indent)
	sb.WriteString(labelStyle.Render("Created"))
	sb.WriteString(valueStyle.Render(issue.CreatedAt.Format("2006-01-02")))
	sb.WriteString("\n")

	if !issue.UpdatedAt.IsZero() && issue.UpdatedAt != issue.CreatedAt {
		sb.WriteString(indent)
		sb.WriteString(labelStyle.Render("Updated"))
		sb.WriteString(valueStyle.Render(issue.UpdatedAt.Format("2006-01-02")))
		sb.WriteString("\n")
	}

	// Labels section
	if len(issue.Labels) > 0 {
		sb.WriteString(indentedDivider)
		sb.WriteString("\n")
		sb.WriteString(indent)
		sb.WriteString(labelStyle.Render("Labels"))
		sb.WriteString("\n")
		for _, label := range issue.Labels {
			sb.WriteString(indent + " " + label + "\n")
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

// renderDependencyItem renders a single dependency with board-style formatting.
// Format: [T][P2][id] (compact, no title to avoid overlap)
// The selected parameter controls the ">" prefix for navigation.
func (m Model) renderDependencyItem(item DependencyItem, selected bool) string {
	// Indent like labels: "  " normally, " >" when selected
	prefix := "  "
	if selected && m.focusPane == FocusMetadata && m.selectedField == FieldDependency {
		prefix = " " + styles.SelectionIndicatorStyle.Render(">")
	}

	idStyle := lipgloss.NewStyle().Foreground(styles.TextSecondaryColor)

	if item.Issue == nil {
		// Fallback: just show ID if load failed
		return prefix + idStyle.Render(item.ID)
	}

	// Use board styling functions - compact format without title
	typeText := board.GetTypeIndicator(item.Issue.Type)
	typeStyle := board.GetTypeStyle(item.Issue.Type)
	priorityText := fmt.Sprintf("[P%d]", item.Issue.Priority)
	priorityStyle := board.GetPriorityStyle(item.Issue.Priority)

	return fmt.Sprintf("%s%s%s%s",
		prefix,
		typeStyle.Render(typeText),
		priorityStyle.Render(priorityText),
		idStyle.Render("["+item.ID+"]"),
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
	var blockedBy, blocks, related []DependencyItem
	for _, dep := range m.dependencies {
		switch dep.Category {
		case "blocked_by":
			blockedBy = append(blockedBy, dep)
		case "blocks":
			blocks = append(blocks, dep)
		case "related":
			related = append(related, dep)
		}
	}

	depIndex := 0 // Track overall index for selection

	if len(blockedBy) > 0 {
		sb.WriteString(indentedDivider)
		sb.WriteString("\n")
		blockedStyle := lipgloss.NewStyle().Foreground(styles.StatusErrorColor)
		sb.WriteString(indent)
		sb.WriteString(blockedStyle.Render("Blocked by"))
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
		blocksStyle := lipgloss.NewStyle().Foreground(styles.PriorityHighColor)
		sb.WriteString(indent)
		sb.WriteString(blocksStyle.Render("Blocks"))
		sb.WriteString("\n")
		for _, dep := range blocks {
			sb.WriteString(m.renderDependencyItem(dep, depIndex == m.selectedDependency))
			sb.WriteString("\n")
			depIndex++
		}
	}

	if len(related) > 0 {
		sb.WriteString(indentedDivider)
		sb.WriteString("\n")
		sb.WriteString(indent)
		sb.WriteString(labelStyle.Render("Related"))
		sb.WriteString("\n")
		for _, dep := range related {
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

	return footerStyle.Render("[j/k] Scroll  [Shift+L] Edit Labels  [d] Delete Issue  [Esc] Back" + scrollPercent)
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
	default:
		return string(t)
	}
}

// IssueID returns the ID of the currently displayed issue.
func (m Model) IssueID() string {
	return m.issue.ID
}

// loadDependencies populates the dependencies slice from the issue's
// BlockedBy, Blocks, and Related fields. If a client is available,
// it fetches full issue data for each dependency.
func (m *Model) loadDependencies() {
	// Collect all dependency IDs with their categories
	var items []DependencyItem
	for _, id := range m.issue.BlockedBy {
		items = append(items, DependencyItem{ID: id, Category: "blocked_by"})
	}
	for _, id := range m.issue.Blocks {
		items = append(items, DependencyItem{ID: id, Category: "blocks"})
	}
	for _, id := range m.issue.Related {
		items = append(items, DependencyItem{ID: id, Category: "related"})
	}

	if len(items) == 0 {
		m.dependencies = items
		return
	}

	// If we have a loader, fetch full issue data
	if m.loader != nil {
		ids := make([]string, len(items))
		for i, item := range items {
			ids[i] = item.ID
		}

		issues, err := m.loader.ListIssuesByIds(ids)
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
