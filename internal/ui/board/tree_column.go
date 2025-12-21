package board

import (
	"fmt"
	"strings"

	"perles/internal/beads"
	"perles/internal/bql"
	"perles/internal/mode/shared"
	"perles/internal/ui/styles"
	"perles/internal/ui/tree"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TreeColumn wraps a tree.Model and implements BoardColumn interface.
// It displays an issue dependency tree in a column format.
type TreeColumn struct {
	title       string
	columnIndex int           // position within the view for message routing
	rootID      string        // Root issue ID for the tree
	mode        tree.TreeMode // ModeDeps or ModeChildren
	color       lipgloss.TerminalColor
	executor    bql.BQLExecutor
	clock       shared.Clock // Clock for timestamp formatting
	width       int
	height      int
	focused     *bool // pointer so it survives value copies
	loadError   error
	tree        *tree.Model
}

// TreeColumnLoadedMsg is sent when a tree column finishes loading its data.
type TreeColumnLoadedMsg struct {
	ViewIndex   int                     // which view this column belongs to
	ColumnIndex int                     // which column within the view
	ColumnTitle string                  // kept for debugging/logging
	RootID      string                  // the root issue ID
	Issues      []beads.Issue           // loaded issues (nil if error)
	IssueMap    map[string]*beads.Issue // indexed issues for tree building
	Err         error                   // error if load failed
}

// NewTreeColumn creates a new tree column.
// treeMode can be "deps" (default) or "child".
func NewTreeColumn(title, rootID, treeMode string, executor bql.BQLExecutor, clock shared.Clock) TreeColumn {
	focused := new(bool)

	// Convert string mode to tree.TreeMode
	mode := tree.ModeDeps
	if treeMode == "child" {
		mode = tree.ModeChildren
	}

	return TreeColumn{
		title:    title,
		rootID:   rootID,
		mode:     mode,
		executor: executor,
		clock:    clock,
		focused:  focused,
	}
}

// Title returns the column title with mode indicator.
// Format: "Title (deps)" or "Title (child)"
func (c TreeColumn) Title() string {
	mode := "deps"
	if c.mode == tree.ModeChildren {
		mode = "child"
	}

	return fmt.Sprintf("%s (%s)", c.title, mode)
}

// RightTitle returns a progress bar showing completed/total counts.
func (c TreeColumn) RightTitle() string {
	if c.tree == nil || c.tree.Root() == nil {
		return ""
	}

	closed, total := c.tree.Root().CalculateProgress()
	if total == 0 {
		return ""
	}

	return renderCompactProgress(closed, total)
}

// renderCompactProgress renders a compact progress bar with percentage and counts.
func renderCompactProgress(closed, total int) string {
	if total == 0 {
		return ""
	}
	percent := float64(closed) / float64(total) * 100
	barWidth := 10
	filledWidth := int(float64(barWidth) * float64(closed) / float64(total))

	filledStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)
	emptyStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)

	filled := filledStyle.Render(strings.Repeat("█", filledWidth))
	empty := emptyStyle.Render(strings.Repeat("░", barWidth-filledWidth))

	return fmt.Sprintf("%s%s %.0f%% (%d/%d)", filled, empty, percent, closed, total)
}

// View renders the tree column content.
func (c TreeColumn) View() string {
	if c.loadError != nil {
		errorStyle := lipgloss.NewStyle().
			Foreground(styles.StatusErrorColor).
			Padding(1, 2)
		return errorStyle.Render(fmt.Sprintf("Error: %v", c.loadError))
	}

	if c.tree == nil {
		emptyStyle := lipgloss.NewStyle().
			Foreground(styles.TextMutedColor).
			Italic(true).
			Padding(1, 2)
		return emptyStyle.Render("No tree data")
	}

	return c.tree.View()
}

// SetSize updates column dimensions.
func (c TreeColumn) SetSize(width, height int) BoardColumn {
	c.width = width
	c.height = height

	// Size for tree (account for borders)
	treeWidth := max(width-2, 1)
	treeHeight := max(height-5, 1)

	if c.tree != nil {
		c.tree.SetSize(treeWidth, treeHeight)
	}

	return c
}

// SetFocused sets whether this column is focused.
func (c TreeColumn) SetFocused(focused bool) BoardColumn {
	*c.focused = focused
	return c
}

// Color returns the column's border/title color.
func (c TreeColumn) Color() lipgloss.TerminalColor {
	if c.color == nil {
		return styles.BorderDefaultColor
	}
	return c.color
}

// SetColor sets the column's border/title color.
func (c TreeColumn) SetColor(color lipgloss.TerminalColor) TreeColumn {
	c.color = color
	return c
}

// Width returns the column's width for rendering.
func (c TreeColumn) Width() int {
	return c.width
}

// LoadCmd returns a tea.Cmd that loads tree data asynchronously.
// viewIndex identifies which view, columnIndex identifies which column within that view.
func (c TreeColumn) LoadCmd(viewIndex, columnIndex int) tea.Cmd {
	if c.executor == nil || c.rootID == "" {
		return nil
	}

	// Capture values for closure
	executor := c.executor
	rootID := c.rootID
	title := c.title

	return func() tea.Msg {
		// Build BQL expand query - always expand down to get full tree
		// Format: id = "rootID" expand down depth *
		query := fmt.Sprintf(`id = "%s" expand down depth *`, rootID)

		issues, err := executor.Execute(query)
		if err != nil {
			return TreeColumnLoadedMsg{
				ViewIndex:   viewIndex,
				ColumnIndex: columnIndex,
				ColumnTitle: title,
				RootID:      rootID,
				Err:         err,
			}
		}

		// Build issue map for tree construction
		issueMap := make(map[string]*beads.Issue, len(issues))
		for i := range issues {
			issueMap[issues[i].ID] = &issues[i]
		}

		return TreeColumnLoadedMsg{
			ViewIndex:   viewIndex,
			ColumnIndex: columnIndex,
			ColumnTitle: title,
			RootID:      rootID,
			Issues:      issues,
			IssueMap:    issueMap,
			Err:         nil,
		}
	}
}

// HandleLoaded processes a load message and returns the updated column.
func (c TreeColumn) HandleLoaded(msg tea.Msg) BoardColumn {
	loadedMsg, ok := msg.(TreeColumnLoadedMsg)
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

	// Check if root issue exists
	if _, exists := loadedMsg.IssueMap[loadedMsg.RootID]; !exists {
		c.loadError = fmt.Errorf("root issue %s not found", loadedMsg.RootID)
		return c
	}

	// Initialize tree model with down direction and current mode
	c.tree = tree.New(loadedMsg.RootID, loadedMsg.IssueMap, tree.DirectionDown, c.mode, c.clock)

	// Apply current size to tree
	if c.width > 0 && c.height > 0 {
		treeWidth := max(c.width-2, 1)
		treeHeight := max(c.height-5, 1)
		c.tree.SetSize(treeWidth, treeHeight)
	}

	c.loadError = nil
	return c
}

// SelectedIssue returns the currently selected tree node's issue.
func (c TreeColumn) SelectedIssue() *beads.Issue {
	if c.tree == nil {
		return nil
	}

	node := c.tree.SelectedNode()
	if node == nil {
		return nil
	}

	return &node.Issue
}

// Update handles messages (j/k navigation, m toggle).
func (c TreeColumn) Update(msg tea.Msg) (BoardColumn, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "j", "down":
			if c.tree != nil {
				c.tree.MoveCursor(1)
			}
		case "k", "up":
			if c.tree != nil {
				c.tree.MoveCursor(-1)
			}
		case "m":
			// Toggle mode and rebuild tree from existing data
			if c.tree != nil {
				c.tree.ToggleMode()
				c.mode = c.tree.Mode() // Sync our mode field
				_ = c.tree.Rebuild()   // Rebuild with new mode
			}
		}
	}

	return c, nil
}

// SetShowCounts sets whether to display counts in the column title.
// For TreeColumn, counts are always shown based on tree node count.
func (c TreeColumn) SetShowCounts(show bool) BoardColumn {
	// TreeColumn always shows counts in Title() based on tree size
	// This method exists for BoardColumn interface compliance
	return c
}

// SetClock sets the clock for timestamp formatting.
func (c TreeColumn) SetClock(clock shared.Clock) BoardColumn {
	c.clock = clock
	return c
}

// IsEmpty returns true if the column has no tree data.
func (c TreeColumn) IsEmpty() bool {
	if c.tree == nil {
		return true
	}
	root := c.tree.Root()
	return root == nil
}

// SetColumnIndex sets the column's index for message routing.
func (c TreeColumn) SetColumnIndex(index int) TreeColumn {
	c.columnIndex = index
	return c
}

// ColumnIndex returns the column's index.
func (c TreeColumn) ColumnIndex() int {
	return c.columnIndex
}

// LoadError returns the error from the last load attempt, if any.
func (c TreeColumn) LoadError() error {
	return c.loadError
}

// RootID returns the root issue ID for this tree column.
func (c TreeColumn) RootID() string {
	return c.rootID
}

// Mode returns the tree mode ("deps" or "child").
func (c TreeColumn) Mode() string {
	if c.mode == tree.ModeChildren {
		return "child"
	}
	return "deps"
}
