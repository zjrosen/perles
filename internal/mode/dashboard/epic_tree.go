package dashboard

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	beads "github.com/zjrosen/perles/internal/beads/domain"
	"github.com/zjrosen/perles/internal/bql"
	"github.com/zjrosen/perles/internal/mode"
	"github.com/zjrosen/perles/internal/orchestration/controlplane"
	"github.com/zjrosen/perles/internal/ui/details"
	"github.com/zjrosen/perles/internal/ui/tree"
)

// loadEpicTree creates a command to load the epic tree data for the given epic ID.
// It executes a BQL query to fetch the epic and all its children using expand down depth *.
func loadEpicTree(epicID string, executor bql.BQLExecutor) tea.Cmd {
	if epicID == "" || executor == nil {
		return nil
	}

	query := fmt.Sprintf(`id = "%s" expand down depth *`, epicID)

	return func() tea.Msg {
		issues, err := executor.Execute(query)
		return epicTreeLoadedMsg{
			Issues: issues,
			RootID: epicID,
			Err:    err,
		}
	}
}

// handleEpicTreeLoaded processes the epic tree loading result and builds the tree model.
// It rejects stale responses by comparing the loaded root ID with lastLoadedEpicID.
func (m Model) handleEpicTreeLoaded(msg epicTreeLoadedMsg) (mode.Controller, tea.Cmd) {
	// Reject stale responses (user may have navigated to different workflow)
	if msg.RootID != m.lastLoadedEpicID {
		return m, nil
	}

	// Handle errors
	if msg.Err != nil {
		// Clear tree on error so UI can show appropriate empty state
		m.epicTree = nil
		m.hasEpicDetail = false
		return m, nil
	}

	// Handle empty results
	if len(msg.Issues) == 0 {
		m.epicTree = nil
		m.hasEpicDetail = false
		return m, nil
	}

	// Build issue map for tree construction
	issueMap := make(map[string]*beads.Issue, len(msg.Issues))
	for i := range msg.Issues {
		issueMap[msg.Issues[i].ID] = &msg.Issues[i]
	}

	// Determine direction and mode - check cached state first, then existing tree, then defaults
	dir := tree.DirectionDown
	treeMode := tree.ModeDeps
	var selectedID string

	// Check cached state for this workflow
	wf := m.SelectedWorkflow()
	if wf != nil {
		if state, exists := m.workflowUIState[wf.ID]; exists {
			if state.TreeDirection != "" {
				dir = state.TreeDirection
			}
			if state.TreeMode != "" {
				treeMode = state.TreeMode
			}
			selectedID = state.TreeSelectedID
		}
	}

	// Preserve existing state if tree already exists (user may have changed within session)
	if m.epicTree != nil {
		dir = m.epicTree.Direction()
		treeMode = m.epicTree.Mode()
		// Preserve current selection over cached state
		if node := m.epicTree.SelectedNode(); node != nil {
			selectedID = node.Issue.ID
		}
	}

	// Initialize tree model
	clock := m.services.Clock
	m.epicTree = tree.New(msg.RootID, issueMap, dir, treeMode, clock)
	m.epicTree.SetZonePrefix(zoneEpicIssuePrefix)

	// Restore selection if we have a saved ID
	if selectedID != "" {
		m.epicTree.SelectByIssueID(selectedID)
	}

	// Update details panel with selected node
	m.updateEpicDetail()

	return m, nil
}

// updateEpicDetail updates the epic details panel with the currently selected tree node.
// It creates a new details model based on the tree's current selection.
func (m *Model) updateEpicDetail() {
	if m.epicTree == nil {
		m.hasEpicDetail = false
		return
	}

	node := m.epicTree.SelectedNode()
	if node == nil {
		m.hasEpicDetail = false
		return
	}

	// Create new details panel for the selected issue
	// Use executor and client from services for dependency loading and comments
	m.epicDetails = details.New(node.Issue, m.services.Executor, m.services.Client).
		SetMarkdownStyle(m.services.Config.UI.MarkdownStyle).
		SetHideFooter(true)

	// Set initial size so viewport is ready for scrolling
	detailsWidth, detailsHeight := m.calculateEpicDetailsSize()
	if detailsWidth > 0 && detailsHeight > 0 {
		m.epicDetails = m.epicDetails.SetSize(detailsWidth, detailsHeight)
	}

	m.hasEpicDetail = true
}

// calculateEpicDetailsSize returns the width and height for the epic details pane.
// Returns (0, 0) if dimensions cannot be calculated (e.g., before first resize).
func (m *Model) calculateEpicDetailsSize() (int, int) {
	if m.width == 0 || m.height == 0 {
		return 0, 0
	}

	// Same logic as SetSize and renderView
	footerHeight := 3
	contentHeight := max(m.height-footerHeight, 5)

	minTableHeight := minWorkflowTableRows + 3
	tableHeight := max(contentHeight*55/100, minTableHeight)
	epicSectionHeight := contentHeight - tableHeight

	if epicSectionHeight < 5 {
		return 0, 0
	}

	epicWidth := m.width
	if m.showCoordinatorPanel && m.coordinatorPanel != nil {
		epicWidth = m.width - CoordinatorPanelWidth
	}

	// 40%/60% split for tree/details
	treeWidth := epicWidth * 40 / 100
	detailsWidth := epicWidth - treeWidth

	return detailsWidth - 2, epicSectionHeight - 2
}

// triggerEpicTreeLoad determines if an epic tree load should be triggered
// based on the current workflow selection. Loads are immediate since the
// BQL query is fast enough that debouncing is unnecessary.
//
// Skip conditions (returns nil):
// - epicID is empty (workflow has no associated epic)
// - epicID unchanged from lastLoadedEpicID (same epic already loaded)
func (m *Model) triggerEpicTreeLoad() tea.Cmd {
	// Get the selected workflow's epic ID
	wf := m.SelectedWorkflow()
	if wf == nil {
		return nil
	}
	epicID := wf.EpicID

	// Skip if no epic ID
	if epicID == "" {
		return nil
	}

	// Skip if same epic already loaded
	if epicID == m.lastLoadedEpicID {
		return nil
	}

	// Track the expected epic ID for stale response detection
	m.lastLoadedEpicID = epicID

	// Load immediately - queries are fast enough that debouncing is unnecessary
	return loadEpicTree(epicID, m.services.Executor)
}

// handleEpicTreeKeysFocusTree handles key events when the tree pane has focus within the epic view.
func (m Model) handleEpicTreeKeysFocusTree(msg tea.KeyMsg) (mode.Controller, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.epicTree != nil {
			m.epicTree.MoveCursor(1)
			m.updateEpicDetail()
		}
		return m, nil

	case "k", "up":
		if m.epicTree != nil {
			m.epicTree.MoveCursor(-1)
			m.updateEpicDetail()
		}
		return m, nil

	case "enter":
		// Refocus tree on selected node
		if m.epicTree != nil {
			node := m.epicTree.SelectedNode()
			if node != nil {
				_ = m.epicTree.Refocus(node.Issue.ID)
				m.updateEpicDetail()
			}
		}
		return m, nil

	case "m":
		// Toggle mode (deps/children)
		if m.epicTree != nil {
			m.epicTree.ToggleMode()
			_ = m.epicTree.Rebuild()
			m.updateEpicDetail()
		}
		return m, nil

	case "l", "right":
		// Switch to details pane
		m.epicViewFocus = EpicFocusDetails
		return m, nil

	case "h", "left":
		// No-op, already at leftmost pane
		return m, nil
	}

	return m, nil
}

// handleEpicTreeKeysFocusDetails handles key events when the details pane has focus within the epic view.
func (m Model) handleEpicTreeKeysFocusDetails(msg tea.KeyMsg) (mode.Controller, tea.Cmd) {
	switch msg.String() {
	case "h", "left":
		// Switch to tree pane
		m.epicViewFocus = EpicFocusTree
		return m, nil

	case "l", "right":
		// No-op, already at rightmost pane
		return m, nil

	case "j", "k", "g", "G":
		// Forward scroll keys to details panel
		if m.hasEpicDetail {
			var cmd tea.Cmd
			m.epicDetails, cmd = m.epicDetails.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	return m, nil
}

// saveEpicTreeState saves the current epic tree state to the UI state cache for the given workflow.
// This preserves tree direction, mode, and selected issue ID for restoration when returning to this workflow.
// Only stores minimal state (enums and ID string) to avoid memory pressure.
func (m *Model) saveEpicTreeState(workflowID string) {
	if workflowID == "" {
		return
	}

	state := m.getOrCreateUIState(controlplane.WorkflowID(workflowID))

	if m.epicTree != nil {
		state.TreeDirection = m.epicTree.Direction()
		state.TreeMode = m.epicTree.Mode()
		if node := m.epicTree.SelectedNode(); node != nil {
			state.TreeSelectedID = node.Issue.ID
		} else {
			state.TreeSelectedID = ""
		}
	} else {
		// Clear tree state if no tree exists
		state.TreeDirection = ""
		state.TreeMode = ""
		state.TreeSelectedID = ""
	}
}
