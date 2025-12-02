package board

import (
	"fmt"
	"io"
	"perles/internal/beads"
	"perles/internal/bql"
	"perles/internal/ui/styles"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

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

// Render renders an issue item with priority colors and type indicator.
func (d issueDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	issue, ok := item.(beads.Issue)
	if !ok {
		return
	}

	isSelected := index == m.Index() && d.focused != nil && *d.focused

	// Text content
	priorityText := fmt.Sprintf("[P%d]", issue.Priority)
	typeText := GetTypeIndicator(issue.Type)
	issueId := fmt.Sprintf("[%s]", issue.ID)
	issueTitle := issue.TitleText

	// Component styles
	priorityStyle := GetPriorityStyle(issue.Priority)
	typeStyle := GetTypeStyle(issue.Type)
	issueIdStyle := lipgloss.NewStyle().Foreground(styles.TextSecondaryColor)

	var line string

	lineParts := []string{
		typeStyle.Render(typeText),
		priorityStyle.Render(priorityText),
		issueIdStyle.Render(issueId),
		fmt.Sprintf(" %s", issueTitle),
	}
	line = strings.Join(lineParts, "")

	if isSelected {
		line = styles.SelectionIndicatorStyle.Render(">") + line
	} else {
		line = " " + line
	}

	_, _ = fmt.Fprint(w, line)
}

// GetTypeIndicator returns the letter indicator for an issue type.
func GetTypeIndicator(t beads.IssueType) string {
	switch t {
	case beads.TypeBug:
		return "[B]"
	case beads.TypeFeature:
		return "[F]"
	case beads.TypeTask:
		return "[T]"
	case beads.TypeEpic:
		return "[E]"
	case beads.TypeChore:
		return "[C]"
	default:
		return "[?]"
	}
}

// GetTypeStyle returns the style for an issue type.
func GetTypeStyle(t beads.IssueType) lipgloss.Style {
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

// GetPriorityStyle returns the style for a priority level.
func GetPriorityStyle(p beads.Priority) lipgloss.Style {
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

// Column represents a single kanban column.
type Column struct {
	title      string
	status     beads.Status
	color      lipgloss.TerminalColor // custom color for column border/title
	list       list.Model
	items      []beads.Issue
	width      int
	height     int
	focused    *bool // pointer so it survives value copies
	showCounts *bool // pointer so it survives value copies (nil = default true)

	// BQL self-loading fields
	executor  *bql.Executor // BQL executor for loading issues
	query     string        // BQL query for this column
	loading   bool          // true while loading issues
	loadError error         // error from last load attempt
}

// NewColumn creates a new column.
func NewColumn(title string, status beads.Status) Column {
	// Allocate focused state on heap so pointer survives copies
	focused := new(bool)

	// Create delegate with pointer to focused state
	delegate := newIssueDelegate(focused)

	l := list.New([]list.Item{}, delegate, 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(false)

	return Column{
		title:   title,
		status:  status,
		list:    l,
		focused: focused,
	}
}

// NewColumnWithExecutor creates a column that can load its own data via BQL.
func NewColumnWithExecutor(title string, query string, executor *bql.Executor) Column {
	col := NewColumn(title, "")
	col.executor = executor
	col.query = query
	return col
}

// ColumnLoadedMsg is sent when a column finishes loading its issues.
type ColumnLoadedMsg struct {
	ViewIndex   int           // which view this column belongs to
	ColumnTitle string        // identifies which column loaded
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
		c.loading = false
		return c
	}

	c.loadError = nil
	c.loading = false
	return c.SetItems(issues)
}

// LoadIssuesCmd returns a tea.Cmd that loads issues asynchronously.
// The command sends a ColumnLoadedMsg when complete (with ViewIndex = 0).
func (c Column) LoadIssuesCmd() tea.Cmd {
	return c.LoadIssuesCmdForView(0)
}

// LoadIssuesCmdForView loads issues and includes view index in the message.
func (c Column) LoadIssuesCmdForView(viewIndex int) tea.Cmd {
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
			ColumnTitle: title,
			Issues:      issues,
			Err:         err,
		}
	}
}

// SetLoading sets the loading state of the column.
func (c Column) SetLoading(loading bool) Column {
	c.loading = loading
	return c
}

// IsLoading returns true if the column is currently loading.
func (c Column) IsLoading() bool {
	return c.loading
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
func (c Column) SetExecutor(executor *bql.Executor) Column {
	c.executor = executor
	return c
}

// SetSize updates column dimensions.
func (c Column) SetSize(width, height int) Column {
	c.width = width
	c.height = height

	// Size list to fit inside borders (2 chars for left/right borders)
	listWidth := width - 2
	if listWidth < 1 {
		listWidth = 1
	}
	// Account for top/bottom borders and bubbles list internal chrome
	listHeight := height - 5
	if listHeight < 1 {
		listHeight = 1
	}
	c.list.SetSize(listWidth, listHeight)
	return c
}

// SetFocused sets whether this column is focused.
func (c Column) SetFocused(focused bool) Column {
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
	return c
}

// SetShowCounts sets whether to display counts in the column title.
func (c Column) SetShowCounts(show bool) Column {
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

// Items returns all issues in the column.
func (c Column) Items() []beads.Issue {
	return c.items
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
func (c Column) Update(msg tea.Msg) (Column, tea.Cmd) {
	var cmd tea.Cmd
	c.list, cmd = c.list.Update(msg)
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
