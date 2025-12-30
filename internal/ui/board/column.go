package board

import (
	"fmt"
	"io"

	"github.com/zjrosen/perles/internal/beads"
	"github.com/zjrosen/perles/internal/bql"
	"github.com/zjrosen/perles/internal/mode/shared"
	"github.com/zjrosen/perles/internal/ui/shared/issuebadge"
	"github.com/zjrosen/perles/internal/ui/styles"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// BoardColumn defines the interface for board columns (BQL or Tree).
// This enables polymorphic handling of different column types.
type BoardColumn interface {
	// Title returns the column title (with optional count).
	Title() string

	// RightTitle returns an optional right-aligned title (e.g., progress bar).
	// Return empty string if no right title is needed.
	RightTitle() string

	// View renders the column content.
	View() string

	// SetSize updates column dimensions and returns the updated column.
	SetSize(width, height int) BoardColumn

	// SetFocused sets whether this column is focused.
	SetFocused(focused bool) BoardColumn

	// Color returns the column's border/title color.
	Color() lipgloss.TerminalColor

	// Width returns the column's width for rendering.
	Width() int

	// LoadCmd returns a tea.Cmd that loads data asynchronously.
	// viewIndex identifies which view, columnIndex identifies which column within that view.
	LoadCmd(viewIndex, columnIndex int) tea.Cmd

	// HandleLoaded processes a load message and returns the updated column.
	HandleLoaded(msg tea.Msg) BoardColumn

	// SelectedIssue returns the currently selected issue, if any.
	SelectedIssue() *beads.Issue

	// Update handles messages and returns the updated column and any command.
	Update(msg tea.Msg) (BoardColumn, tea.Cmd)

	// SetShowCounts sets whether to display counts in the column title.
	SetShowCounts(show bool) BoardColumn

	// IsEmpty returns true if the column has no items.
	IsEmpty() bool

	SetClock(clock shared.Clock) BoardColumn
}

// issueDelegate is a custom delegate for rendering issues with priority colors and type indicators.
type issueDelegate struct {
	focused *bool // pointer to column's focused state
}

// newIssueDelegate creates a new issue delegate.
func newIssueDelegate(focused *bool) issueDelegate {
	return issueDelegate{
		focused: focused,
	}
}

// Height returns the height of each item.
func (d issueDelegate) Height() int {
	return 1
}

// Spacing returns the spacing between items.
func (d issueDelegate) Spacing() int {
	return 0
}

// Update handles any delegate-level updates.
func (d issueDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}

// renderIssueLine returns the rendered line for an issue (used by both Render and width calculation).
func renderIssueLine(issue beads.Issue, isSelected bool) string {
	return issuebadge.Render(issue, issuebadge.Config{
		ShowSelection: true,
		Selected:      isSelected,
	})
}

// itemRenderedLines returns how many lines an issue takes when rendered at the given width.
func itemRenderedLines(issue beads.Issue, width int) int {
	line := renderIssueLine(issue, false)
	lineWidth := lipgloss.Width(line)
	if lineWidth <= width || width <= 0 {
		return 1
	}

	return (lineWidth + width - 1) / width
}

// Render renders an issue item with priority colors and type indicator.
func (d issueDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	issue, ok := item.(beads.Issue)
	if !ok {
		return
	}

	isSelected := index == m.Index() && d.focused != nil && *d.focused
	line := renderIssueLine(issue, isSelected)
	_, _ = fmt.Fprint(w, line)
}

// Column represents a single kanban column.
type Column struct {
	title       string
	columnIndex int                    // position within the view for message routing
	color       lipgloss.TerminalColor // custom color for column border/title
	list        list.Model
	items       []beads.Issue
	width       int
	height      int
	focused     *bool // pointer so it survives value copies
	showCounts  *bool // pointer so it survives value copies (nil = default true)

	// BQL self-loading fields
	executor  bql.BQLExecutor // BQL executor for loading issues
	query     string          // BQL query for this column
	loadError error           // error from last load attempt
}

// NewColumn creates a new column.
func NewColumn(title string) Column {
	// Allocate focused state on heap so pointer survives copies
	focused := new(bool)

	// Create delegate with pointer to focused state
	delegate := newIssueDelegate(focused)

	l := list.New([]list.Item{}, delegate, 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(false)
	l.SetShowPagination(false)

	return Column{
		title:   title,
		list:    l,
		focused: focused,
	}
}

// NewColumnWithExecutor creates a column that can load its own data via BQL.
func NewColumnWithExecutor(title string, query string, executor bql.BQLExecutor) Column {
	col := NewColumn(title)
	col.executor = executor
	col.query = query
	return col
}

// ColumnLoadedMsg is sent when a column finishes loading its issues.
type ColumnLoadedMsg struct {
	ViewIndex   int           // which view this column belongs to
	ColumnIndex int           // which column within the view
	ColumnTitle string        // kept for debugging/logging
	Issues      []beads.Issue // loaded issues (nil if error)
	Err         error         // error if load failed
}

// LoadIssues executes the BQL query and returns the column with loaded issues.
// This is a synchronous operation - for async loading, use LoadIssuesCmd().
func (c Column) LoadIssues() Column {
	if c.executor == nil || c.query == "" {
		return c
	}

	issues, err := c.executor.Execute(c.query)
	if err != nil {
		c.loadError = err
		return c
	}

	c.loadError = nil
	return c.SetItems(issues)
}

// LoadIssuesCmd returns a tea.Cmd that loads issues asynchronously.
// The command sends a ColumnLoadedMsg when complete (with ViewIndex = 0, ColumnIndex = 0).
func (c Column) LoadIssuesCmd() tea.Cmd {
	return c.LoadCmd(0, 0)
}

// LoadCmd returns a tea.Cmd that loads data asynchronously.
// Implements BoardColumn interface.
// viewIndex identifies which view, columnIndex identifies which column within that view.
func (c Column) LoadCmd(viewIndex, columnIndex int) tea.Cmd {
	if c.executor == nil || c.query == "" {
		return nil
	}

	// Capture values for closure
	executor := c.executor
	query := c.query
	title := c.title

	return func() tea.Msg {
		issues, err := executor.Execute(query)
		return ColumnLoadedMsg{
			ViewIndex:   viewIndex,
			ColumnIndex: columnIndex,
			ColumnTitle: title,
			Issues:      issues,
			Err:         err,
		}
	}
}

// HandleLoaded processes a load message and returns the updated column.
// Implements BoardColumn interface.
func (c Column) HandleLoaded(msg tea.Msg) BoardColumn {
	loadedMsg, ok := msg.(ColumnLoadedMsg)
	if !ok {
		return c
	}

	// Match by column index instead of title to support duplicate column names
	if loadedMsg.ColumnIndex != c.columnIndex {
		return c
	}

	if loadedMsg.Err != nil {
		c.loadError = loadedMsg.Err
		return c
	}

	c.loadError = nil
	return c.SetItems(loadedMsg.Issues)
}

// SetColumnIndex sets the column's index for message routing.
func (c Column) SetColumnIndex(index int) Column {
	c.columnIndex = index
	return c
}

// ColumnIndex returns the column's index.
func (c Column) ColumnIndex() int {
	return c.columnIndex
}

// LoadError returns the error from the last load attempt, if any.
func (c Column) LoadError() error {
	return c.loadError
}

// Query returns the BQL query for this column.
func (c Column) Query() string {
	return c.query
}

// SetQuery sets the BQL query for this column.
func (c Column) SetQuery(query string) Column {
	c.query = query
	return c
}

// SetExecutor sets the BQL executor for this column.
func (c Column) SetExecutor(executor bql.BQLExecutor) Column {
	c.executor = executor
	return c
}

// SetSize updates column dimensions.
func (c Column) SetSize(width, height int) BoardColumn {
	c.width = width
	c.height = height

	// Size list to fit inside borders (2 chars for left/right borders)
	listWidth := max(width-2, 1)
	// RenderWithTitleBorder uses height-2 for content area (top + bottom border)
	listHeight := max(height-2, 1)
	c.list.SetSize(listWidth, listHeight)

	// Recalculate PerPage based on actual item heights (accounting for wrapping)
	c.updatePerPage()
	return c
}

// SetFocused sets whether this column is focused.
func (c Column) SetFocused(focused bool) BoardColumn {
	*c.focused = focused
	return c
}

// SetItems populates the column with issues.
func (c Column) SetItems(issues []beads.Issue) Column {
	c.items = issues
	items := make([]list.Item, len(issues))
	for i, issue := range issues {
		items[i] = issue
	}
	c.list.SetItems(items)
	c.updatePerPage()
	return c
}

// updatePerPage calculates how many items actually fit in the visible area,
// accounting for items that wrap to multiple lines.
func (c *Column) updatePerPage() {
	if c.width <= 0 || c.height <= 0 || len(c.items) == 0 {
		return
	}

	// Available height for content (border takes 2 lines)
	availableLines := c.height - 2
	if availableLines <= 0 {
		return
	}

	// Inner width for content (border takes 2 chars)
	innerWidth := c.width - 2

	// Count how many items fit
	usedLines := 0
	itemsThatFit := 0
	for _, issue := range c.items {
		lines := itemRenderedLines(issue, innerWidth)
		if usedLines+lines > availableLines {
			break
		}
		usedLines += lines
		itemsThatFit++
	}

	// Ensure at least 1 item fits
	if itemsThatFit == 0 {
		itemsThatFit = 1
	}

	c.list.Paginator.PerPage = itemsThatFit
	// Also update TotalPages to match our custom PerPage
	c.list.Paginator.SetTotalPages(len(c.items))
}

// SetShowCounts sets whether to display counts in the column title.
// Implements BoardColumn interface.
func (c Column) SetShowCounts(show bool) BoardColumn {
	if c.showCounts == nil {
		c.showCounts = new(bool)
	}
	*c.showCounts = show
	return c
}

// SelectedItem returns the currently selected issue.
func (c Column) SelectedItem() *beads.Issue {
	if item := c.list.SelectedItem(); item != nil {
		issue := item.(beads.Issue)
		return &issue
	}
	return nil
}

// SelectedIssue returns the currently selected issue.
// Implements BoardColumn interface.
func (c Column) SelectedIssue() *beads.Issue {
	return c.SelectedItem()
}

// Items returns all issues in the column.
func (c Column) Items() []beads.Issue {
	return c.items
}

// IsEmpty returns true if the column has no items.
// Implements BoardColumn interface.
func (c Column) IsEmpty() bool {
	return len(c.items) == 0
}

// SelectByID selects the issue with the given ID. Returns true if found.
func (c Column) SelectByID(id string) (Column, bool) {
	for i, issue := range c.items {
		if issue.ID == id {
			c.list.Select(i)
			return c, true
		}
	}
	return c, false
}

// Update handles messages.
func (c Column) Update(msg tea.Msg) (BoardColumn, tea.Cmd) {
	var cmd tea.Cmd
	c.list, cmd = c.list.Update(msg)
	// Re-apply our custom PerPage after list.Update() resets it via updatePagination()
	c.updatePerPage()
	return c, cmd
}

// Title returns the formatted title with optional count for border rendering.
// If showCounts is false, returns just the title without count.
func (c Column) Title() string {
	// Default to showing counts if not explicitly set
	if c.showCounts != nil && !*c.showCounts {
		return c.title
	}
	return fmt.Sprintf("%s (%d)", c.title, len(c.items))
}

// RightTitle returns an optional right-aligned title.
// BQL columns don't use a right title, so this returns empty string.
func (c Column) RightTitle() string {
	return ""
}

// View renders the column content (without border - border applied by board).
func (c Column) View() string {
	if len(c.items) == 0 {
		emptyStyle := lipgloss.NewStyle().
			Foreground(styles.TextMutedColor).
			Italic(true).
			Padding(1, 2)
		return emptyStyle.Render("No issues")
	}
	return c.list.View()
}

// SetColor sets the column's border/title color.
func (c Column) SetColor(color lipgloss.TerminalColor) Column {
	c.color = color
	return c
}

// Color returns the column's color for rendering.
func (c Column) Color() lipgloss.TerminalColor {
	if c.color == nil {
		return styles.BorderDefaultColor // Default fallback
	}
	return c.color
}

// Width returns the column's width for rendering.
// Implements BoardColumn interface.
func (c Column) Width() int {
	return c.width
}

// SetClock sets the clock for timestamp formatting. The board column does not
// show timestamps, so this is a no-op for now.
func (c Column) SetClock(_ shared.Clock) BoardColumn {
	return c
}
