package treeview

import (
	"fmt"
	"perles/internal/beads"
	"perles/internal/ui/styles"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TreeLoader fetches issues for the tree.
type TreeLoader interface {
	Execute(query string) ([]beads.Issue, error)
}

// Direction determines which way the tree grows.
type Direction int

const (
	DirectionDownstream Direction = iota // Show what this issue blocks (children/dependents)
	DirectionUpstream                    // Show what blocks this issue (parents/prerequisites)
)

// Node represents a node in the tree.
type Node struct {
	Issue    beads.Issue
	Children []*Node
	Expanded bool   // For collapsible nodes (future)
	Depth    int
	IsLast   bool   // For drawing connector lines
	Prefix   string // Tree prefix for indentation (e.g., "│   │   ")
}

// Model is the tree view model.
type Model struct {
	rootID    string
	loader    TreeLoader
	viewport  viewport.Model
	width     int
	height    int
	ready     bool
	loading   bool
	err       error
	nodes     []*Node // Linearized list of nodes for navigation
	rootNode  *Node
	selected  int // Index in nodes slice
	direction Direction
}

// New creates a new tree view.
func New(rootID string, loader TreeLoader) Model {
	return Model{
		rootID:    rootID,
		loader:    loader,
		direction: DirectionDownstream, // Default to downstream (breakdown)
		loading:   true,
	}
}

// Init loads the tree data.
func (m Model) Init() tea.Cmd {
	return m.loadTreeCmd
}

// SetSize updates dimensions.
func (m Model) SetSize(width, height int) Model {
	m.width = width
	m.height = height
	if !m.ready {
		m.viewport = viewport.New(width, height)
		m.ready = true
	} else {
		m.viewport.Width = width
		m.viewport.Height = height
	}
	m.viewport.SetContent(m.renderContent())
	return m
}

// CopyIDMsg requests copying an issue ID.
type CopyIDMsg struct {
	IssueID string
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "t", "esc":
			// Parent should handle exit
			return m, nil
		case "r":
			// Refresh/Reload
			m.loading = true
			return m, m.loadTreeCmd
		case "tab":
			// Toggle direction
			if m.direction == DirectionDownstream {
				m.direction = DirectionUpstream
			} else {
				m.direction = DirectionDownstream
			}
			m.loading = true
			return m, m.loadTreeCmd
		case "y":
			// Copy ID
			if m.selected >= 0 && m.selected < len(m.nodes) {
				return m, func() tea.Msg {
					return CopyIDMsg{IssueID: m.nodes[m.selected].Issue.ID}
				}
			}
		case "j", "down":
			if len(m.nodes) > 0 {
				m.selected++
				if m.selected >= len(m.nodes) {
					m.selected = 0
				}
				m.scrollToSelection()
			}
		case "k", "up":
			if len(m.nodes) > 0 {
				m.selected--
				if m.selected < 0 {
					m.selected = len(m.nodes) - 1
				}
				m.scrollToSelection()
			}
		case "enter":
			// Navigate to selected issue (parent handles this)
			// Return a message that the parent (details view) recognizes?
			// Or just return the model and let parent check selected?
			// We'll define a message type for this.
			if m.selected >= 0 && m.selected < len(m.nodes) {
				return m, func() tea.Msg {
					return NavigateMsg{IssueID: m.nodes[m.selected].Issue.ID}
				}
			}
		}
	case treeLoadedMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.rootNode = msg.root
			m.nodes = flattenTree(m.rootNode)
			m.selected = 0
		}
		m.viewport.SetContent(m.renderContent())
		return m, nil
	}

	return m, nil
}

// View renders the tree.
func (m Model) View() string {
	if !m.ready {
		return "Loading..."
	}
	return m.viewport.View()
}

// Internal helpers

func (m Model) loadTreeCmd() tea.Msg {
	// Construct BQL query based on direction
	// Downstream: expand downstream (all dependents/incoming links)
	// Upstream: expand upstream (all dependencies/outgoing links)
	expandType := "downstream"
	if m.direction == DirectionUpstream {
		expandType = "upstream"
	}

	// Query: id = <rootID> expand <type> depth *
	query := fmt.Sprintf("id = %s expand %s depth *", m.rootID, expandType)

	issue, err := m.loader.Execute(query)
	if err != nil {
		return treeLoadedMsg{err: err}
	}

	// Build tree
	root := buildTree(m.rootID, issue, m.direction)
	return treeLoadedMsg{root: root}
}

func (m *Model) scrollToSelection() {
	// Simple scrolling: keep selected line in view
	// Viewport tracks lines.
	// Header takes up some lines?
	// renderContent returns the full string.
	// We need to map selection index to line number.
	// Each node is one line.
	// Header is 2 lines (Title + Usage).
	headerHeight := 2
	m.viewport.SetYOffset(max(0, m.selected-m.viewport.Height/2+headerHeight))
	m.viewport.SetContent(m.renderContent())
}

func (m Model) renderContent() string {
	var sb strings.Builder

	// Header
	dirStr := "Downstream (Dependents)"
	if m.direction == DirectionUpstream {
		dirStr = "Upstream (Dependencies)"
	}
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.TextSecondaryColor)
	sb.WriteString(titleStyle.Render(fmt.Sprintf("Dependency Tree: %s", dirStr)))
	sb.WriteString("\n")

	helpStyle := lipgloss.NewStyle().Foreground(styles.TextDescriptionColor)
	sb.WriteString(helpStyle.Render("[Tab] Switch Direction  [Enter] Go to Issue  [y] Copy ID  [Esc] Back"))
	sb.WriteString("\n\n")

	if m.loading {
		sb.WriteString("Loading...")
		return sb.String()
	}

	if m.err != nil {
		errStyle := lipgloss.NewStyle().Foreground(styles.StatusErrorColor)
		sb.WriteString(errStyle.Render(fmt.Sprintf("Error loading tree: %v", m.err)))
		return sb.String()
	}

	if m.rootNode == nil {
		sb.WriteString("No data.")
		return sb.String()
	}

	// Render nodes
	for i, node := range m.nodes {
		isSelected := i == m.selected
		sb.WriteString(renderNode(node, isSelected))
		sb.WriteString("\n")
	}

	return sb.String()
}

// Tree building logic

func buildTree(rootID string, issues []beads.Issue, dir Direction) *Node {
	issueMap := make(map[string]beads.Issue)
	for _, issue := range issues {
		issueMap[issue.ID] = issue
	}

	rootIssue, ok := issueMap[rootID]
	if !ok {
		// Should not happen if query succeeded, unless root is deleted
		return nil
	}

	visited := make(map[string]bool)
	return buildNode(rootIssue, issueMap, visited, dir, 0)
}

func buildNode(issue beads.Issue, issueMap map[string]beads.Issue, visited map[string]bool, dir Direction, depth int) *Node {
	if visited[issue.ID] {
		return nil // Cycle detected or already visited
	}
	visited[issue.ID] = true
	// No backtracking: we want to show each node only once globally in the tree.
	// This prevents diamond dependencies from duplicating nodes.

	node := &Node{
		Issue: issue,
		Depth: depth,
	}

	// Determine children IDs based on direction
	var childrenIDs []string
	if dir == DirectionDownstream {
		// Downstream: dependents (what points to me)
		// Includes children (parent-child) and issues I block (blocks)
		childrenIDs = append(childrenIDs, issue.Children...)
		childrenIDs = append(childrenIDs, issue.Blocks...)
	} else {
		// Upstream: dependencies (what I point to)
		// Includes parent (parent-child) and issues blocking me (blocked_by)
		if issue.ParentID != "" {
			childrenIDs = append(childrenIDs, issue.ParentID)
		}
		childrenIDs = append(childrenIDs, issue.BlockedBy...)
	}

	for _, id := range childrenIDs {
		if childIssue, ok := issueMap[id]; ok {
			childNode := buildNode(childIssue, issueMap, visited, dir, depth+1)
			if childNode != nil {
				node.Children = append(node.Children, childNode)
			}
		}
	}

	// Mark the last actually-added child (not based on original list position)
	if len(node.Children) > 0 {
		node.Children[len(node.Children)-1].IsLast = true
	}

	return node
}

func flattenTree(root *Node) []*Node {
	if root == nil {
		return nil
	}
	return flattenTreeWithAncestry(root, nil)
}

func flattenTreeWithAncestry(node *Node, ancestry []bool) []*Node {
	// Build prefix from ancestry
	var prefix strings.Builder
	for _, ancestorWasLast := range ancestry {
		if ancestorWasLast {
			prefix.WriteString("    ") // 4 spaces (completed branch)
		} else {
			prefix.WriteString("│   ") // Continuation line
		}
	}
	node.Prefix = prefix.String()

	var nodes []*Node
	nodes = append(nodes, node)

	for _, child := range node.Children {
		// Build ancestry for children: add current node's IsLast status
		// (but skip root at depth 0 since it has no parent to show continuation for)
		var childAncestry []bool
		childAncestry = append(childAncestry, ancestry...)
		if node.Depth > 0 {
			childAncestry = append(childAncestry, node.IsLast)
		}
		nodes = append(nodes, flattenTreeWithAncestry(child, childAncestry)...)
	}
	return nodes
}

// Rendering logic

func renderNode(node *Node, selected bool) string {
	// Indentation (computed prefix with proper tree connectors)
	indent := node.Prefix

	// Tree marker
	marker := ""
	if node.Depth > 0 {
		if node.IsLast {
			marker = "└── "
		} else {
			marker = "├── "
		}
	}

	// Selection cursor
	cursor := " "
	if selected {
		cursor = styles.SelectionIndicatorStyle.Render(">")
	}

	// Issue details
	typeStyle := getIssueTypeStyle(node.Issue.Type)
	typeStr := typeStyle.Render(string(node.Issue.Type)) // e.g. "bug"

	priorityStyle := getIssuePriorityStyle(node.Issue.Priority)
	prioStr := priorityStyle.Render(fmt.Sprintf("P%d", node.Issue.Priority))

	statusStyle := getIssueStatusStyle(node.Issue.Status)
	statusStr := statusStyle.Render(string(node.Issue.Status))

	idStyle := lipgloss.NewStyle().Foreground(styles.TextSecondaryColor)
	idStr := idStyle.Render("[" + node.Issue.ID + "]")

	// Title (truncated if too long?)
	title := node.Issue.TitleText

	// Format: >  └── [bug] [P1] [ID] Title (Status)
	return fmt.Sprintf("%s %s%s%s %s %s %s (%s)",
		cursor,
		indent,
		marker,
		typeStr,
		prioStr,
		idStr,
		title,
		statusStr,
	)
}

// Styles (copied/adapted from board/styles because of import cycles if we use board?)
// Actually we can import styles package.
// We'll reuse local helpers that use styles package.

func getIssueTypeStyle(t beads.IssueType) lipgloss.Style {
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

func getIssuePriorityStyle(p beads.Priority) lipgloss.Style {
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

func getIssueStatusStyle(s beads.Status) lipgloss.Style {
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

// Messages

type treeLoadedMsg struct {
	root *Node
	err  error
}

// NavigateMsg requests navigation to an issue.
type NavigateMsg struct {
	IssueID string
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
