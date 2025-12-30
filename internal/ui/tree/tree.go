package tree

import (
	"fmt"
	"strings"

	"github.com/zjrosen/perles/internal/beads"
	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/mode/shared"
	"github.com/zjrosen/perles/internal/ui/shared/issuebadge"
	"github.com/zjrosen/perles/internal/ui/styles"

	"github.com/charmbracelet/lipgloss"
)

// Model holds the tree view state.
type Model struct {
	root       *TreeNode               // Current root of the tree
	nodes      []*TreeNode             // Flattened visible nodes for navigation
	cursor     int                     // Index into nodes slice
	direction  Direction               // "down" or "up"
	mode       TreeMode                // "deps" or "children"
	rootStack  []string                // Stack of previous root IDs for back navigation
	originalID string                  // Original root issue ID (for 'U' to return)
	issueMap   map[string]*beads.Issue // Cached issues for tree building
	clock      shared.Clock            // Clock for formatting relative timestamps
	width      int
	height     int
	scrollTop  int // First visible line index (for viewport scrolling)
}

// New creates a new tree model with default mode (deps).
func New(rootID string, issueMap map[string]*beads.Issue, dir Direction, mode TreeMode, clock shared.Clock) *Model {
	m := &Model{
		direction:  dir,
		mode:       mode,
		originalID: rootID,
		issueMap:   issueMap,
		clock:      clock,
		cursor:     0,
	}

	// Build the tree
	root, err := BuildTree(issueMap, rootID, dir, m.mode)
	if err != nil {
		// Return empty model on error
		return m
	}

	m.root = root
	m.nodes = root.Flatten()
	return m
}

// SetSize sets the viewport dimensions.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// MoveCursor moves the cursor by delta, respecting bounds.
func (m *Model) MoveCursor(delta int) {
	newPos := m.cursor + delta
	newPos = max(newPos, 0)
	newPos = min(newPos, len(m.nodes)-1)
	newPos = max(newPos, 0) // Handle empty nodes case
	m.cursor = newPos
	m.ensureCursorVisible()
}

// ensureCursorVisible adjusts scrollTop to keep cursor in view.
func (m *Model) ensureCursorVisible() {
	viewportHeight := m.viewportHeight()
	if viewportHeight <= 0 {
		return
	}

	// Scroll down if cursor is below viewport
	if m.cursor >= m.scrollTop+viewportHeight {
		m.scrollTop = m.cursor - viewportHeight + 1
	}

	// Scroll up if cursor is above viewport
	if m.cursor < m.scrollTop {
		m.scrollTop = m.cursor
	}

	// Clamp scrollTop
	maxScroll := max(len(m.nodes)-viewportHeight, 0)
	m.scrollTop = min(m.scrollTop, maxScroll)
	m.scrollTop = max(m.scrollTop, 0)
}

// viewportHeight returns the number of visible node rows.
func (m *Model) viewportHeight() int {
	// Reserve lines for padding only (title is now in parent container's border)
	reserved := 1
	if m.height > reserved {
		return m.height - reserved
	}
	return 1
}

// RefreshNodes rebuilds the flattened nodes list after state changes.
func (m *Model) RefreshNodes() {
	if m.root != nil {
		m.nodes = m.root.Flatten()
		// Clamp cursor to valid range
		if m.cursor >= len(m.nodes) {
			m.cursor = len(m.nodes) - 1
		}
		if m.cursor < 0 {
			m.cursor = 0
		}
	}
}

// SelectedNode returns the currently selected node.
func (m *Model) SelectedNode() *TreeNode {
	if m.cursor >= 0 && m.cursor < len(m.nodes) {
		return m.nodes[m.cursor]
	}
	return nil
}

// SelectByIssueID moves the cursor to the node with the given issue ID.
// Returns true if the issue was found and selected, false otherwise.
// Note: Does not adjust scroll position - caller should ensure size is set
// and let normal rendering handle scroll adjustment.
func (m *Model) SelectByIssueID(issueID string) bool {
	for i, node := range m.nodes {
		if node.Issue.ID == issueID {
			m.cursor = i
			return true
		}
	}
	return false
}

// Root returns the tree root.
func (m *Model) Root() *TreeNode {
	return m.root
}

// Direction returns the current traversal direction.
func (m *Model) Direction() Direction {
	return m.direction
}

// SetDirection changes the direction (requires rebuild).
func (m *Model) SetDirection(dir Direction) {
	m.direction = dir
}

// Mode returns the current tree mode.
func (m *Model) Mode() TreeMode {
	return m.mode
}

// ToggleMode switches between deps and children modes.
func (m *Model) ToggleMode() {
	if m.mode == ModeDeps {
		m.mode = ModeChildren
	} else {
		m.mode = ModeDeps
	}
}

// Rebuild reconstructs the tree from existing data with current direction/mode.
// Call this after SetDirection or ToggleMode to apply the changes.
func (m *Model) Rebuild() error {
	if m.root == nil {
		return nil
	}
	rootID := m.root.Issue.ID
	root, err := BuildTree(m.issueMap, rootID, m.direction, m.mode)
	if err != nil {
		log.ErrorErr(log.CatTree, "Failed to rebuild tree", err,
			"rootID", rootID,
			"direction", string(m.direction),
			"mode", string(m.mode))
		return err
	}
	m.root = root
	m.nodes = root.Flatten()
	// Try to preserve cursor position, clamp if needed
	if m.cursor >= len(m.nodes) {
		m.cursor = max(len(m.nodes)-1, 0)
	}
	return nil
}

// Refocus sets a new root and pushes current root to stack.
func (m *Model) Refocus(newRootID string) error {
	if m.root != nil {
		m.rootStack = append(m.rootStack, m.root.Issue.ID)
	}
	root, err := BuildTree(m.issueMap, newRootID, m.direction, m.mode)
	if err != nil {
		log.ErrorErr(log.CatTree, "Failed to refocus tree", err,
			"newRootID", newRootID,
			"direction", string(m.direction),
			"mode", string(m.mode))
		return err
	}
	m.root = root
	m.nodes = root.Flatten()
	m.cursor = 0
	return nil
}

// GoBack pops the stack and returns to previous root.
// Returns true if successful, false if a re-query is needed (parent not in cache).
func (m *Model) GoBack() (needsRequery bool, requeriedID string) {
	var prevID string

	if len(m.rootStack) > 0 {
		// Pop from stack
		prevID = m.rootStack[len(m.rootStack)-1]
		m.rootStack = m.rootStack[:len(m.rootStack)-1]
	} else if m.root != nil && m.root.Issue.ParentID != "" {
		// No stack, but root has a parent - navigate to it
		prevID = m.root.Issue.ParentID
	} else {
		return false, "" // Already at top level
	}

	// Check if parent exists in issue map
	if _, ok := m.issueMap[prevID]; !ok {
		// Parent not in loaded data - need to re-query
		return true, prevID
	}

	root, err := BuildTree(m.issueMap, prevID, m.direction, m.mode)
	if err != nil {
		log.ErrorErr(log.CatTree, "Failed to go back in tree", err,
			"prevID", prevID,
			"direction", string(m.direction),
			"mode", string(m.mode))
		return false, ""
	}
	m.root = root
	m.nodes = root.Flatten()
	m.cursor = 0
	return false, ""
}

// GoToOriginal clears stack and returns to original root.
func (m *Model) GoToOriginal() error {
	m.rootStack = nil
	root, err := BuildTree(m.issueMap, m.originalID, m.direction, m.mode)
	if err != nil {
		log.ErrorErr(log.CatTree, "Failed to go to original root", err,
			"originalID", m.originalID,
			"direction", string(m.direction),
			"mode", string(m.mode))
		return err
	}
	m.root = root
	m.nodes = root.Flatten()
	m.cursor = 0
	return nil
}

// View renders the tree.
func (m *Model) View() string {
	if m.root == nil || len(m.nodes) == 0 {
		return "No tree data"
	}

	var sb strings.Builder

	// Check if tree has only root (no children/parents in this direction)
	if len(m.nodes) == 1 && len(m.root.Children) == 0 {
		// Show the root node
		line := m.renderNode(m.root, true, true)
		sb.WriteString(line)
		sb.WriteString("\n\n")

		// Show message about no dependencies
		mutedStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)
		if m.direction == DirectionDown {
			sb.WriteString(mutedStyle.Render("No child dependencies found."))
		} else {
			sb.WriteString(mutedStyle.Render("No parent dependencies found."))
		}
		sb.WriteString("\n")
		sb.WriteString(mutedStyle.Render("Press 'd' to toggle direction and check the other direction."))
		return sb.String()
	}

	// Calculate viewport bounds
	viewportHeight := m.viewportHeight()
	endIdx := min(m.scrollTop+viewportHeight, len(m.nodes))

	// Show scroll indicator (up)
	if m.scrollTop > 0 {
		scrollStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)
		sb.WriteString(scrollStyle.Render(fmt.Sprintf("  ↑ %d more above", m.scrollTop)))
		sb.WriteString("\n")
	}

	// Render visible nodes only
	for i := m.scrollTop; i < endIdx; i++ {
		node := m.nodes[i]
		isLast := m.isLastChild(node)
		line := m.renderNode(node, isLast, i == m.cursor)
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	// Show scroll indicator (down)
	remaining := len(m.nodes) - endIdx
	if remaining > 0 {
		scrollStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)
		sb.WriteString(scrollStyle.Render(fmt.Sprintf("  ↓ %d more below", remaining)))
		sb.WriteString("\n")
	}

	return sb.String()
}

// isLastChild determines if node is the last child of its parent.
func (m *Model) isLastChild(node *TreeNode) bool {
	if node.Parent == nil {
		return true // Root is always "last"
	}
	children := node.Parent.Children
	return len(children) > 0 && children[len(children)-1] == node
}

// renderNode renders a single tree node.
func (m *Model) renderNode(node *TreeNode, isLast bool, isSelected bool) string {
	var sb strings.Builder

	// Cursor indicator
	if isSelected {
		sb.WriteString(styles.SelectionIndicatorStyle.Render(">"))
	} else {
		sb.WriteString(" ")
	}

	// Tree prefix (indentation and branch characters)
	prefix := m.buildPrefix(node, isLast)
	if isSelected && node.Depth > 0 {
		// Add horizontal guide line for selected nodes by replacing spaces with ─
		prefix = m.addSelectionGuide(prefix)
	}
	sb.WriteString(prefix)

	// Use shared issuebadge component for type/priority/id
	sb.WriteString(issuebadge.RenderBadge(node.Issue))
	sb.WriteString(" ")

	// Status indicator
	statusText := m.renderStatus(node.Issue.Status)
	statusWidth := lipgloss.Width(statusText)

	// Build right metadata: comment indicator + timestamp
	metaStyle := lipgloss.NewStyle().Foreground(styles.TextSecondaryColor)
	timestamp := shared.FormatRelativeTimeWithClock(node.Issue.CreatedAt, m.clock)
	commentInd := styles.FormatCommentIndicator(node.Issue.CommentCount)

	rightMeta := timestamp
	if commentInd != "" {
		rightMeta = commentInd + " " + timestamp
	}
	rightRendered := metaStyle.Render(rightMeta)
	rightWidth := lipgloss.Width(rightRendered)

	// Calculate available width for title
	// Format: [prefix][type][priority][id] title status    metadata
	leftWidth := lipgloss.Width(sb.String())
	minPadding := 2
	minTitleWidth := 10 // Minimum chars for a readable title

	// Check if we have enough room for metadata
	// Need: leftWidth + minTitleWidth + 1 (space) + statusWidth + minPadding + rightWidth <= m.width
	showMetadata := m.width >= leftWidth+minTitleWidth+1+statusWidth+minPadding+rightWidth

	var availableForTitle int
	if showMetadata {
		// Leave room for: title + " " + status + padding (min 2) + metadata
		availableForTitle = m.width - leftWidth - 1 - statusWidth - minPadding - rightWidth
	} else {
		// No metadata - just need room for title + " " + status
		availableForTitle = m.width - leftWidth - 1 - statusWidth
		rightWidth = 0
		rightRendered = ""
	}
	if availableForTitle < 0 {
		availableForTitle = 0
	}

	// Title (truncate if needed)
	title := node.Issue.TitleText
	if availableForTitle > 0 && lipgloss.Width(title) > availableForTitle {
		title = styles.TruncateString(title, availableForTitle)
	} else if availableForTitle <= 0 {
		title = ""
	}
	sb.WriteString(title)

	sb.WriteString(" ")
	sb.WriteString(statusText)

	// Add metadata if showing
	if showMetadata && rightWidth > 0 {
		// Calculate padding to right-align metadata
		currentWidth := lipgloss.Width(sb.String())
		paddingNeeded := max(m.width-currentWidth-rightWidth, minPadding)
		if isSelected {
			guideStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)
			sb.WriteString(guideStyle.Render(strings.Repeat("─", paddingNeeded)))
		} else {
			sb.WriteString(strings.Repeat(" ", paddingNeeded))
		}
		sb.WriteString(rightRendered)
	}

	return sb.String()
}

// buildPrefix builds the tree branch prefix for a node.
func (m *Model) buildPrefix(node *TreeNode, isLast bool) string {
	if node.Depth == 0 {
		return ""
	}

	var parts []string

	// Walk up the parent chain to build the prefix
	ancestors := m.getAncestors(node)
	for i := len(ancestors) - 1; i >= 0; i-- {
		ancestor := ancestors[i]
		if ancestor.Parent != nil {
			ancestorIsLast := m.isLastChild(ancestor)
			if ancestorIsLast {
				parts = append(parts, "    ") // No line (space)
			} else {
				parts = append(parts, "│   ") // Continuing line
			}
		}
	}

	// Add the connector for this node
	if isLast {
		parts = append(parts, "└─")
	} else {
		parts = append(parts, "├─")
	}

	return strings.Join(parts, "")
}

// getAncestors returns ancestors from immediate parent to root (not including node).
func (m *Model) getAncestors(node *TreeNode) []*TreeNode {
	var ancestors []*TreeNode
	current := node.Parent
	for current != nil {
		ancestors = append(ancestors, current)
		current = current.Parent
	}
	return ancestors
}

// addSelectionGuide replaces spaces in the prefix with horizontal lines for the selected node.
// This creates a visual guide from the cursor to the node content.
// Preserves │ characters and branch connectors (├─, └─).
func (m *Model) addSelectionGuide(prefix string) string {
	guideStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)
	var result strings.Builder

	for _, r := range prefix {
		switch r {
		case ' ':
			// Replace space with horizontal line
			result.WriteString(guideStyle.Render("─"))
		case '│', '├', '└', '─':
			// Keep tree structure characters as-is
			result.WriteRune(r)
		default:
			result.WriteRune(r)
		}
	}

	return result.String()
}

// renderStatus renders the status badge.
func (m *Model) renderStatus(status beads.Status) string {
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
