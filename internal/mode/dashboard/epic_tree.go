package dashboard

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/zjrosen/perles/internal/keys"
	"github.com/zjrosen/perles/internal/mode"
	"github.com/zjrosen/perles/internal/orchestration/controlplane"
	"github.com/zjrosen/perles/internal/task"
	"github.com/zjrosen/perles/internal/ui/details"
	"github.com/zjrosen/perles/internal/ui/modals/commenteditor"
	"github.com/zjrosen/perles/internal/ui/modals/issueeditor"
	"github.com/zjrosen/perles/internal/ui/shared/toaster"
	"github.com/zjrosen/perles/internal/ui/tree"
)

// loadEpicTree creates a command to load the epic tree data for the given epic ID.
// It executes a BQL query to fetch the epic and all its children using expand down depth *.
func loadEpicTree(epicID string, executor task.QueryExecutor) tea.Cmd {
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
	issueMap := make(map[string]*task.Issue, len(msg.Issues))
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
	m.epicDetails = details.New(node.Issue, m.services.QueryExecutor, m.services.QueryHelpers, m.services.TaskExecutor).
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
	if m.maximizedPane == dashboardPaneEpicDetails {
		return max(m.width-2, 1), max(m.height-2, 1)
	}

	// Same logic as SetSize and renderView
	contentHeight := m.contentHeight()

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

	layout := calculateEpicTreeLayout(epicWidth)
	return layout.detailsWidth - 2, epicSectionHeight - 2
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
	return loadEpicTree(epicID, m.services.QueryExecutor)
}

// handleEpicTreeKeysFocusTree handles key events when the tree pane has focus within the epic view.
func (m Model) handleEpicTreeKeysFocusTree(msg tea.KeyMsg) (mode.Controller, tea.Cmd) {
	switch msg.String() {
	case "y": // Yank (copy) issue ID to clipboard
		return m.yankTreeIssueID()

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

	// Handle key bindings that require key.Matches
	if key.Matches(msg, keys.Component.EditAction) {
		if m.epicTree != nil {
			if node := m.epicTree.SelectedNode(); node != nil {
				issue := node.Issue
				m.editingIssue = &issue // Store for comparison on save
				editor := issueeditor.NewWithExecutorAndVimMode(issue, m.services.QueryExecutor, m.vimMode).SetSize(m.width, m.height)
				m.issueEditor = &editor
				return m, m.issueEditor.Init()
			}
		}
		return m, nil
	}

	return m, nil
}

// handleEpicTreeKeysFocusDetails handles key events when the details pane has focus within the epic view.
func (m Model) handleEpicTreeKeysFocusDetails(msg tea.KeyMsg) (mode.Controller, tea.Cmd) {
	switch msg.String() {
	case "y": // Yank (copy) issue description to clipboard
		return m.yankIssueDescription()

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

	// Handle key bindings that require key.Matches
	if key.Matches(msg, keys.Component.EditAction) {
		// Details panel shows the tree's selected issue, so use same source
		if m.epicTree != nil {
			if node := m.epicTree.SelectedNode(); node != nil {
				issue := node.Issue
				m.editingIssue = &issue // Store for comparison on save
				editor := issueeditor.NewWithExecutorAndVimMode(issue, m.services.QueryExecutor, m.vimMode).SetSize(m.width, m.height)
				m.issueEditor = &editor
				return m, m.issueEditor.Init()
			}
		}
		return m, nil
	}

	if key.Matches(msg, keys.Component.CommentAction) {
		if m.hasEpicDetail {
			issue := m.epicDetails.Issue()
			editor := commenteditor.NewWithVimMode(issue, m.vimMode).SetSize(m.width, m.height)
			m.commentEditor = &editor
			return m, m.commentEditor.Init()
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

// yankTreeIssueID copies the selected tree node's issue ID to clipboard.
func (m Model) yankTreeIssueID() (mode.Controller, tea.Cmd) {
	if m.epicTree == nil {
		return m, func() tea.Msg {
			return mode.ShowToastMsg{Message: "No tree loaded", Style: toaster.StyleError}
		}
	}

	node := m.epicTree.SelectedNode()
	if node == nil {
		return m, func() tea.Msg {
			return mode.ShowToastMsg{Message: "No issue selected", Style: toaster.StyleError}
		}
	}

	if m.services.Clipboard == nil {
		return m, func() tea.Msg {
			return mode.ShowToastMsg{Message: "Clipboard unavailable", Style: toaster.StyleError}
		}
	}

	if err := m.services.Clipboard.Copy(node.Issue.ID); err != nil {
		return m, func() tea.Msg {
			return mode.ShowToastMsg{Message: "Clipboard error: " + err.Error(), Style: toaster.StyleError}
		}
	}

	return m, func() tea.Msg {
		return mode.ShowToastMsg{Message: "Copied: " + node.Issue.ID, Style: toaster.StyleSuccess}
	}
}

// yankIssueDescription copies the selected issue's description to clipboard.
func (m Model) yankIssueDescription() (mode.Controller, tea.Cmd) {
	if m.epicTree == nil {
		return m, func() tea.Msg {
			return mode.ShowToastMsg{Message: "No tree loaded", Style: toaster.StyleError}
		}
	}

	node := m.epicTree.SelectedNode()
	if node == nil {
		return m, func() tea.Msg {
			return mode.ShowToastMsg{Message: "No issue selected", Style: toaster.StyleError}
		}
	}

	if m.services.Clipboard == nil {
		return m, func() tea.Msg {
			return mode.ShowToastMsg{Message: "Clipboard unavailable", Style: toaster.StyleError}
		}
	}

	description := node.Issue.DescriptionText
	if description == "" {
		return m, func() tea.Msg {
			return mode.ShowToastMsg{Message: "Issue has no description", Style: toaster.StyleWarn}
		}
	}

	if err := m.services.Clipboard.Copy(description); err != nil {
		return m, func() tea.Msg {
			return mode.ShowToastMsg{Message: "Clipboard error: " + err.Error(), Style: toaster.StyleError}
		}
	}

	return m, func() tea.Msg {
		return mode.ShowToastMsg{Message: "Copied issue description", Style: toaster.StyleSuccess}
	}
}
