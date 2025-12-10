// Package search implements the search mode controller for BQL-powered issue search.
package search

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"perles/internal/beads"
	"perles/internal/bql"
	"perles/internal/config"
	"perles/internal/mode"
	"perles/internal/mode/shared"
	"perles/internal/ui/details"
	"perles/internal/ui/forms/bqlinput"
	"perles/internal/ui/modals/help"
	"perles/internal/ui/modals/labeleditor"
	"perles/internal/ui/shared/colorpicker"
	"perles/internal/ui/shared/formmodal"
	"perles/internal/ui/shared/modal"
	"perles/internal/ui/shared/picker"
	"perles/internal/ui/shared/toaster"
	"perles/internal/ui/styles"
	"perles/internal/ui/tree"
)

// FocusPane represents which pane has focus in the search mode.
type FocusPane int

const (
	FocusSearch  FocusPane = iota // Left: search input
	FocusResults                  // Left: results list
	FocusDetails                  // Right: detail view
)

// ViewMode represents overlay states within search mode.
type ViewMode int

const (
	ViewSearch ViewMode = iota
	ViewHelp
	ViewPriorityPicker
	ViewStatusPicker
	ViewSaveColumn
	ViewSaveAction      // Action picker: existing vs new view
	ViewNewView         // New view modal
	ViewDeleteConfirm   // Delete issue confirmation modal
	ViewLabelEditor     // Label editor modal
	ViewDetailsEditMenu // Edit menu overlay on details
)

// Model holds the search mode state.
type Model struct {
	services mode.Services

	// Sub-mode state
	subMode mode.SubMode

	// List sub-mode (BQL search with flat results)
	input         bqlinput.Model
	results       []beads.Issue
	resultsList   list.Model
	selectedIdx   int
	searchErr     error
	showSearchErr bool // Only show error after blur, not during typing
	searchVersion int  // Incremented on each input change for debounce

	// Tree sub-mode (issue ID with tree rendering)
	tree     *tree.Model  // Tree rendering model (from internal/ui/tree)
	treeRoot *beads.Issue // Root issue for header display

	// Detail panel
	details   details.Model
	hasDetail bool // True when an issue is selected

	// Overlays
	view          ViewMode
	help          help.Model
	picker        picker.Model
	selectedIssue *beads.Issue // Issue being edited in picker
	viewSelector  formmodal.Model
	newViewModal  formmodal.Model
	modal         modal.Model
	labelEditor   labeleditor.Model

	// Delete operation state
	deleteIsCascade bool // True if deleting an epic with children

	// Focus management
	focus FocusPane

	// Layout
	width  int
	height int
}

// newViewSaveMsg is sent when creating a new view from search.
type newViewSaveMsg struct {
	ViewName   string
	ColumnName string
	Color      string
	Query      string
}

// updateViewSaveMsg is sent when adding a column to existing views.
type updateViewSaveMsg struct {
	ColumnName  string
	Color       string
	Query       string
	ViewIndices []int
}

// closeSaveViewMsg closes any save view modal and returns to search.
type closeSaveViewMsg struct{}

// makeNewViewFormConfig creates the formmodal config for creating a new view.
func makeNewViewFormConfig(existingViews []config.ViewConfig, currentQuery string) formmodal.FormConfig {
	return formmodal.FormConfig{
		Title: "Create New View",
		Fields: []formmodal.FieldConfig{
			{
				Key:         "viewName",
				Type:        formmodal.FieldTypeText,
				Label:       "View Name",
				Hint:        "required",
				Placeholder: "View name",
				MaxLength:   50,
			},
			{
				Key:         "columnName",
				Type:        formmodal.FieldTypeText,
				Label:       "Column Name",
				Hint:        "optional",
				Placeholder: "defaults to view name",
				MaxLength:   30,
			},
			{
				Key:          "color",
				Type:         formmodal.FieldTypeColor,
				Label:        "Color",
				Hint:         "Enter to change",
				InitialColor: "#73F59F",
			},
		},
		SubmitLabel: " Save ",
		MinWidth:    50,
		Validate: func(values map[string]any) error {
			viewName := strings.TrimSpace(values["viewName"].(string))
			if viewName == "" {
				return fmt.Errorf("View name is required")
			}
			for _, v := range existingViews {
				if strings.EqualFold(v.Name, viewName) {
					return fmt.Errorf("View '%s' already exists", v.Name)
				}
			}
			return nil
		},
		OnSubmit: func(values map[string]any) tea.Msg {
			viewName := strings.TrimSpace(values["viewName"].(string))
			columnName := strings.TrimSpace(values["columnName"].(string))
			if columnName == "" {
				columnName = viewName
			}
			return newViewSaveMsg{
				ViewName:   viewName,
				ColumnName: columnName,
				Color:      values["color"].(string),
				Query:      currentQuery,
			}
		},
		OnCancel: func() tea.Msg { return closeSaveViewMsg{} },
	}
}

// makeUpdateViewFormConfig creates the formmodal config for adding a column to existing views.
func makeUpdateViewFormConfig(views []config.ViewConfig, currentQuery string) formmodal.FormConfig {
	options := make([]formmodal.ListOption, len(views))
	for i, v := range views {
		options[i] = formmodal.ListOption{
			Label:    v.Name,
			Value:    fmt.Sprintf("%d", i),
			Selected: false,
		}
	}

	return formmodal.FormConfig{
		Title: "Save as Column",
		Fields: []formmodal.FieldConfig{
			{
				Key:         "columnName",
				Type:        formmodal.FieldTypeText,
				Label:       "Column Name",
				Hint:        "required",
				Placeholder: "Enter column name...",
			},
			{
				Key:          "color",
				Type:         formmodal.FieldTypeColor,
				Label:        "Color",
				Hint:         "Enter to change",
				InitialColor: "#73F59F",
			},
			{
				Key:         "views",
				Type:        formmodal.FieldTypeList,
				Label:       "Add to Views",
				Hint:        "Space to toggle",
				MultiSelect: true,
				Options:     options,
			},
		},
		SubmitLabel: " Save ",
		MinWidth:    50,
		Validate: func(values map[string]any) error {
			name := strings.TrimSpace(values["columnName"].(string))
			if name == "" {
				return fmt.Errorf("column name is required")
			}
			selectedViews := values["views"].([]string)
			if len(selectedViews) == 0 {
				return fmt.Errorf("select at least one view")
			}
			return nil
		},
		OnSubmit: func(values map[string]any) tea.Msg {
			columnName := strings.TrimSpace(values["columnName"].(string))
			selectedViews := values["views"].([]string)

			indices := make([]int, 0, len(selectedViews))
			for _, s := range selectedViews {
				if idx, err := strconv.Atoi(s); err == nil {
					indices = append(indices, idx)
				}
			}

			return updateViewSaveMsg{
				ColumnName:  columnName,
				Color:       values["color"].(string),
				Query:       currentQuery,
				ViewIndices: indices,
			}
		},
		OnCancel: func() tea.Msg { return closeSaveViewMsg{} },
	}
}

// New creates a new search mode controller.
func New(services mode.Services) Model {
	input := bqlinput.New()
	input.SetPlaceholder("Enter BQL query ex: status in (open,in_progress) and label not in (backlog) order by priority,created desc")
	input.Focus()

	// Configure results list with custom delegate
	delegate := newIssueDelegate()
	resultsList := list.New([]list.Item{}, delegate, 0, 0)
	resultsList.SetShowTitle(false)
	resultsList.SetShowStatusBar(false)
	resultsList.SetShowHelp(false)
	resultsList.SetFilteringEnabled(false)

	return Model{
		services:    services,
		input:       input,
		resultsList: resultsList,
		focus:       FocusSearch,
		view:        ViewSearch,
		help:        help.NewSearch(),
	}
}

// Init returns initial commands for the mode.
func (m Model) Init() tea.Cmd {
	// If entering tree sub-mode, load the tree instead of executing search
	if m.subMode == mode.SubModeTree && m.treeRoot != nil {
		return m.loadTree(m.treeRoot.ID)
	}
	// Execute initial search for list sub-mode
	return m.executeSearch()
}

// SetQuery sets the initial search query and returns the modified model.
// This also ensures the model is in list sub-mode.
func (m Model) SetQuery(query string) Model {
	m.subMode = mode.SubModeList
	m.focus = FocusSearch // Focus search input
	m.input.SetValue(query)
	// Clear tree state from any previous tree sub-mode usage
	m.tree = nil
	m.treeRoot = nil
	return m
}

// SetTreeRootIssueId configures the model to enter tree sub-mode for an issue.
// The tree will be loaded when Init() is called.
func (m Model) SetTreeRootIssueId(issueID string) Model {
	m.subMode = mode.SubModeTree
	m.focus = FocusResults // Focus tree panel
	// Store issueID in treeRoot temporarily - we'll load full issue in Init
	m.treeRoot = &beads.Issue{ID: issueID}
	return m
}

// SetSize handles terminal resize.
func (m Model) SetSize(width, height int) Model {
	m.width = width
	m.height = height

	// Guard against zero dimensions
	if width == 0 || height == 0 {
		return m
	}

	// Calculate 50/50 split
	leftWidth := width / 2
	rightWidth := width - leftWidth - 1 // -1 for divider

	// Update input width
	inputWidth := max(leftWidth-4, 1) // Padding
	m.input.SetWidth(inputWidth)

	// Update results list
	listHeight := max(height-5, 1) // Input row + header + status + borders
	listWidth := max(leftWidth-2, 1)
	m.resultsList.SetSize(listWidth, listHeight)

	// Update details panel (height-2 accounts for top/bottom border)
	if m.hasDetail {
		m.details = m.details.SetSize(rightWidth-2, height-2)
	}

	// Update help
	m.help = m.help.SetSize(width, height)

	// Update tree model if present (tree sub-mode)
	if m.tree != nil {
		// Tree height accounts for: borders (2)
		treeHeight := max(height-2, 1)
		m.tree.SetSize(leftWidth-2, treeHeight) // -2 for left/right border
	}

	return m
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case searchResultsMsg:
		return m.handleSearchResults(msg)

	case treeLoadedMsg:
		return m.handleTreeLoaded(msg)

	case details.OpenPriorityPickerMsg:
		return m.openPriorityPicker(msg)

	case details.OpenStatusPickerMsg:
		return m.openStatusPicker(msg)

	case details.NavigateToDependencyMsg:
		return m.navigateToDependency(msg.IssueID)

	// Picker callback messages
	case prioritySelectedMsg:
		m.view = ViewSearch
		m.selectedIssue = nil
		return m, updatePriorityCmd(msg.issueID, msg.priority)

	case statusSelectedMsg:
		m.view = ViewSearch
		m.selectedIssue = nil
		return m, updateStatusCmd(msg.issueID, msg.status)

	case pickerCancelledMsg:
		m.view = ViewSearch
		m.selectedIssue = nil
		return m, nil

	// Fallback: handle default picker messages if callbacks not set
	case picker.CancelMsg:
		m.view = ViewSearch
		m.selectedIssue = nil
		return m, nil

	case priorityChangedMsg:
		return m.handlePriorityChanged(msg)

	case statusChangedMsg:
		return m.handleStatusChanged(msg)

	case debounceSearchMsg:
		// Only execute if version matches (not stale)
		if msg.version == m.searchVersion {
			return m, m.executeSearch()
		}
		return m, nil

	case colorpicker.SelectMsg, colorpicker.CancelMsg:
		// Route colorpicker messages to the appropriate component
		switch m.view {
		case ViewSaveColumn:
			var cmd tea.Cmd
			m.viewSelector, cmd = m.viewSelector.Update(msg)
			return m, cmd
		case ViewNewView:
			var cmd tea.Cmd
			m.newViewModal, cmd = m.newViewModal.Update(msg)
			return m, cmd
		}
		return m, nil

	case closeSaveViewMsg:
		m.view = ViewSearch
		return m, nil

	case saveActionExistingViewMsg:
		// Transition to existing view selector
		m.viewSelector = formmodal.New(makeUpdateViewFormConfig(m.services.Config.Views, msg.query)).
			SetSize(m.width, m.height)
		m.view = ViewSaveColumn
		return m, nil

	case saveActionNewViewMsg:
		// Transition to new view modal
		m.newViewModal = formmodal.New(makeNewViewFormConfig(m.services.Config.Views, msg.query)).
			SetSize(m.width, m.height)
		m.view = ViewNewView
		return m, nil

	case newViewSaveMsg:
		m.view = ViewSearch
		return m, tea.Batch(
			func() tea.Msg {
				return SaveSearchToNewViewMsg(msg)
			},
			func() tea.Msg {
				return mode.ShowToastMsg{Message: fmt.Sprintf("Created view '%s'", msg.ViewName), Style: toaster.StyleSuccess}
			},
		)

	case updateViewSaveMsg:
		m.view = ViewSearch
		count := len(msg.ViewIndices)
		toastMsg := fmt.Sprintf("Column added to %d view(s)", count)
		if count == 1 {
			toastMsg = "Column added to 1 view"
		}
		return m, tea.Batch(
			func() tea.Msg {
				return SaveSearchAsColumnMsg(msg)
			},
			func() tea.Msg { return mode.ShowToastMsg{Message: toastMsg, Style: toaster.StyleSuccess} },
		)

	case details.DeleteIssueMsg:
		return m.openDeleteConfirm(msg)

	case details.OpenLabelEditorMsg:
		m.labelEditor = labeleditor.New(msg.IssueID, msg.Labels).
			SetSize(m.width, m.height)
		m.view = ViewLabelEditor
		return m, m.labelEditor.Init()

	case details.OpenEditMenuMsg:
		m.selectedIssue = m.getSelectedIssue()
		m.picker = picker.NewWithConfig(picker.Config{
			Title: "Edit Issue",
			Options: []picker.Option{
				{Label: "Edit labels", Value: "labels"},
				{Label: "Change priority", Value: "priority"},
				{Label: "Change status", Value: "status"},
			},
			OnSelect: func(opt picker.Option) tea.Msg {
				switch opt.Value {
				case "labels":
					return editMenuLabelsMsg{}
				case "priority":
					return editMenuPriorityMsg{}
				case "status":
					return editMenuStatusMsg{}
				}
				return nil
			},
			OnCancel: func() tea.Msg { return pickerCancelledMsg{} },
		}).SetSize(m.width, m.height)
		m.view = ViewDetailsEditMenu
		return m, nil

	case editMenuLabelsMsg:
		if m.selectedIssue == nil {
			m.view = ViewSearch
			return m, nil
		}
		m.labelEditor = labeleditor.New(m.selectedIssue.ID, m.selectedIssue.Labels).
			SetSize(m.width, m.height)
		m.view = ViewLabelEditor
		return m, m.labelEditor.Init()

	case editMenuPriorityMsg:
		if m.selectedIssue == nil {
			m.view = ViewSearch
			return m, nil
		}
		issueID := m.selectedIssue.ID
		m.picker = picker.NewWithConfig(picker.Config{
			Title:    "Priority",
			Options:  shared.PriorityOptions(),
			Selected: int(m.selectedIssue.Priority),
			OnSelect: func(opt picker.Option) tea.Msg {
				priority := beads.Priority(opt.Value[1] - '0') // Parse "P0"-"P4"
				return prioritySelectedMsg{issueID: issueID, priority: priority}
			},
			OnCancel: func() tea.Msg {
				return pickerCancelledMsg{}
			},
		}).SetSize(m.width, m.height)
		m.view = ViewPriorityPicker
		return m, nil

	case editMenuStatusMsg:
		if m.selectedIssue == nil {
			m.view = ViewSearch
			return m, nil
		}
		issueID := m.selectedIssue.ID
		m.picker = picker.NewWithConfig(picker.Config{
			Title:    "Status",
			Options:  shared.StatusOptions(),
			Selected: picker.FindIndexByValue(shared.StatusOptions(), string(m.selectedIssue.Status)),
			OnSelect: func(opt picker.Option) tea.Msg {
				status := beads.Status(opt.Value)
				return statusSelectedMsg{issueID: issueID, status: status}
			},
			OnCancel: func() tea.Msg {
				return pickerCancelledMsg{}
			},
		}).SetSize(m.width, m.height)
		m.view = ViewStatusPicker
		return m, nil

	case modal.SubmitMsg:
		return m.handleModalSubmit(msg)

	case modal.CancelMsg:
		return m.handleModalCancel()

	case labeleditor.SaveMsg:
		m.view = ViewSearch
		return m, setLabelsCmd(msg.IssueID, msg.Labels)

	case labeleditor.CancelMsg:
		m.view = ViewSearch
		return m, nil

	case issueDeletedMsg:
		return m.handleIssueDeleted(msg)

	case labelsChangedMsg:
		return m.handleLabelsChanged(msg)
	}

	return m, nil
}

// View renders the search mode.
func (m Model) View() string {
	// Handle overlays
	switch m.view {
	case ViewHelp:
		return m.help.Overlay(m.renderMainView())
	case ViewPriorityPicker, ViewStatusPicker, ViewSaveAction, ViewDetailsEditMenu:
		return m.picker.Overlay(m.renderMainView())
	case ViewSaveColumn:
		return m.viewSelector.Overlay(m.renderMainView())
	case ViewNewView:
		return m.newViewModal.Overlay(m.renderMainView())
	case ViewDeleteConfirm:
		return m.modal.Overlay(m.renderMainView())
	case ViewLabelEditor:
		return m.labelEditor.Overlay(m.renderMainView())
	}

	return m.renderMainView()
}

// handleKey processes keyboard input.
func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	// Handle overlays first
	switch m.view {
	case ViewHelp:
		if msg.String() == "esc" || msg.String() == "?" {
			m.view = ViewSearch
		}
		return m, nil

	case ViewPriorityPicker, ViewStatusPicker:
		return m.handlePickerKey(msg)

	case ViewSaveColumn:
		// Handle key messages
		var cmd tea.Cmd
		m.viewSelector, cmd = m.viewSelector.Update(msg)
		return m, cmd

	case ViewSaveAction, ViewDetailsEditMenu:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		// Delegate to picker
		var cmd tea.Cmd
		m.picker, cmd = m.picker.Update(msg)
		return m, cmd

	case ViewNewView:
		var cmd tea.Cmd
		m.newViewModal, cmd = m.newViewModal.Update(msg)
		return m, cmd

	case ViewDeleteConfirm:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		// Delegate to modal
		var cmd tea.Cmd
		m.modal, cmd = m.modal.Update(msg)
		return m, cmd

	case ViewLabelEditor:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		// Delegate to label editor
		var cmd tea.Cmd
		m.labelEditor, cmd = m.labelEditor.Update(msg)
		return m, cmd
	}

	// When focused on search input, only intercept specific keys
	// All other keys (including j/k/h/l) go to the input
	if m.focus == FocusSearch {
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			// Exit search mode back to kanban
			m.input.Blur()
			return m, func() tea.Msg { return ExitToKanbanMsg{} }
		case "tab", "ctrl+n":
			// Exit search input, move to results
			m.input.Blur()
			m.focus = FocusResults
			m.showSearchErr = true // Show any pending error now
			return m, nil
		case "ctrl+p":
			// Cycle backward from search to details
			m.input.Blur()
			m.showSearchErr = true
			m.focus = FocusDetails
			return m, nil
		case "enter":
			m.input.Blur()
			m.focus = FocusResults
			m.showSearchErr = true // Show any pending error now
			return m, m.executeSearch()
		case "ctrl+@":
			// Let app handle mode switch (ctrl+space)
			return m, nil
		case "ctrl+s":
			// Save current query as column (works even while typing)
			query := m.input.Value()
			if query == "" {
				return m, func() tea.Msg { return mode.ShowToastMsg{Message: "Enter a query first", Style: toaster.StyleWarn} }
			}
			// Show action picker to choose between existing view or new view
			m.picker = picker.NewWithConfig(picker.Config{
				Title: "Save search query as column:",
				Options: []picker.Option{
					{Label: "Save to existing view", Value: "existing"},
					{Label: "Save to new view", Value: "new"},
				},
				OnSelect: func(opt picker.Option) tea.Msg {
					if opt.Value == "new" {
						return saveActionNewViewMsg{query: query}
					}
					return saveActionExistingViewMsg{query: query}
				},
				OnCancel: func() tea.Msg { return closeSaveViewMsg{} },
			}).SetSize(m.width, m.height).SetBoxWidth(30)
			m.view = ViewSaveAction
			return m, nil
		case "down":
			// Down arrow always moves to results
			m.input.Blur()
			m.focus = FocusResults
			m.showSearchErr = true
			return m, nil
		default:
			// Pass all other keys to input (including j/k/h/l)
			oldValue := m.input.Value()
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)

			// If value changed, trigger debounced search
			if m.input.Value() != oldValue {
				m.searchVersion++
				debounceCmd := debounceSearch(m.searchVersion, 300*time.Millisecond)
				return m, tea.Batch(cmd, debounceCmd)
			}
			return m, cmd
		}
	}

	// Tree sub-mode specific handling (when focused on tree panel)
	if m.subMode == mode.SubModeTree && m.focus == FocusResults {
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			return m, func() tea.Msg { return ExitToKanbanMsg{} }
		case "?":
			m.help = m.help.SetMode(help.ModeSearchTree)
			m.view = ViewHelp
			return m, nil
		case "/":
			// Switch from tree to list sub-mode
			m.subMode = mode.SubModeList
			m.focus = FocusSearch
			m.input.Focus()
			m.showSearchErr = false
			return m, nil
		case "j", "down":
			if m.tree != nil {
				m.tree.MoveCursor(1)
				m.updateDetailFromTree()
			}
			return m, nil
		case "k", "up":
			if m.tree != nil {
				m.tree.MoveCursor(-1)
				m.updateDetailFromTree()
			}
			return m, nil
		case "enter":
			return m.refocusTree()
		case "u":
			return m.treeGoBack()
		case "U":
			return m.treeGoToOriginal()
		case "d":
			return m.toggleTreeDirection()
		case "m":
			return m.toggleTreeMode()
		case "l", "right":
			// Move focus to details panel
			m.focus = FocusDetails
			return m, nil
		case "y":
			// Yank (copy) issue ID to clipboard
			return m.yankTreeIssueID()
		case "tab", "ctrl+n":
			m.focus = FocusDetails
			return m, nil
		case "ctrl+p":
			m.focus = FocusDetails
			return m, nil
		}
	}

	// Not in search input - handle navigation and global keys
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit

	case "esc":
		// Exit search mode back to kanban
		return m, func() tea.Msg { return ExitToKanbanMsg{} }

	case "?":
		m.help = m.help.SetMode(help.ModeSearch)
		m.view = ViewHelp
		return m, nil

	case "/":
		// Focus search input
		m.focus = FocusSearch
		m.input.Focus()
		m.showSearchErr = false // Hide error while typing
		return m, nil

	case "ctrl+@":
		// Let app handle mode switch (ctrl+space)
		return m, nil

	case "ctrl+s":
		// Save current query as column
		query := m.input.Value()
		if query == "" {
			return m, func() tea.Msg { return mode.ShowToastMsg{Message: "Enter a query first", Style: toaster.StyleWarn} }
		}
		// Show action picker to choose between existing view or new view
		m.picker = picker.NewWithConfig(picker.Config{
			Title: "Save search query as column:",
			Options: []picker.Option{
				{Label: "Save to existing view", Value: "existing"},
				{Label: "Save to new view", Value: "new"},
			},
			OnSelect: func(opt picker.Option) tea.Msg {
				if opt.Value == "new" {
					return saveActionNewViewMsg{query: query}
				}
				return saveActionExistingViewMsg{query: query}
			},
			OnCancel: func() tea.Msg { return closeSaveViewMsg{} },
		}).SetSize(m.width, m.height).SetBoxWidth(30)
		m.view = ViewSaveAction
		return m, nil

	case "h", "left":
		// Move focus left
		if m.focus == FocusDetails {
			if m.details.IsOnLeftEdge() {
				// Already on left edge of details, move to results
				m.focus = FocusResults
			} else {
				// Delegate to details (move from deps pane to content)
				var cmd tea.Cmd
				m.details, cmd = m.details.Update(msg)
				return m, cmd
			}
		}
		return m, nil

	case "l", "right":
		// Move focus right
		switch m.focus {
		case FocusResults:
			m.focus = FocusDetails
		case FocusDetails:
			// Delegate to details (move from content to deps pane)
			var cmd tea.Cmd
			m.details, cmd = m.details.Update(msg)
			return m, cmd
		}
		return m, nil

	case "ctrl+n":
		// In tree sub-mode: cycle Results <-> Details (no search input)
		if m.subMode == mode.SubModeTree {
			switch m.focus {
			case FocusResults:
				m.focus = FocusDetails
			case FocusDetails:
				m.focus = FocusResults
			}
			return m, nil
		}
		// List sub-mode: cycle Search -> Results -> Details -> Search
		switch m.focus {
		case FocusSearch:
			m.input.Blur()
			m.focus = FocusResults
			m.showSearchErr = true
		case FocusResults:
			m.focus = FocusDetails
		case FocusDetails:
			m.focus = FocusSearch
			m.input.Focus()
			m.showSearchErr = false
		}
		return m, nil

	case "ctrl+p":
		// In tree sub-mode: cycle Details <-> Results (no search input)
		if m.subMode == mode.SubModeTree {
			switch m.focus {
			case FocusResults:
				m.focus = FocusDetails
			case FocusDetails:
				m.focus = FocusResults
			}
			return m, nil
		}
		// List sub-mode: cycle Details -> Results -> Search -> Details
		switch m.focus {
		case FocusSearch:
			m.input.Blur()
			m.showSearchErr = true
			m.focus = FocusDetails
		case FocusDetails:
			m.focus = FocusResults
		case FocusResults:
			m.focus = FocusSearch
			m.input.Focus()
			m.showSearchErr = false
		}
		return m, nil

	case "tab":
		// In tree sub-mode: cycle Results <-> Details (no search input)
		if m.subMode == mode.SubModeTree {
			switch m.focus {
			case FocusResults:
				m.focus = FocusDetails
			case FocusDetails:
				m.focus = FocusResults
			}
			return m, nil
		}
		// List sub-mode: cycle Search -> Results -> Details -> Search
		switch m.focus {
		case FocusSearch:
			m.input.Blur()
			m.focus = FocusResults
			m.showSearchErr = true
		case FocusResults:
			m.focus = FocusDetails
		case FocusDetails:
			m.focus = FocusSearch
			m.input.Focus()
			m.showSearchErr = false
		}
		return m, nil

	case "j", "down":
		return m.handleNavDown()

	case "k", "up":
		// On first result item, move to search input
		if m.focus == FocusResults && m.selectedIdx == 0 {
			m.focus = FocusSearch
			m.input.Focus()
			m.showSearchErr = false
			return m, nil
		}
		return m.handleNavUp()

	case "y":
		// Yank (copy) issue ID to clipboard
		if m.focus == FocusResults || m.focus == FocusDetails {
			return m.yankIssueID()
		}
		return m, nil

	case "enter":
		if m.focus == FocusResults {
			// Switch to tree sub-mode for selected issue
			return m.switchToTreeSubMode()
		}
		if m.focus == FocusDetails {
			// Open picker for selected field
			var cmd tea.Cmd
			m.details, cmd = m.details.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	// Delegate remaining keys to details if focused there
	if m.focus == FocusDetails {
		var cmd tea.Cmd
		m.details, cmd = m.details.Update(msg)
		return m, cmd
	}

	return m, nil
}

// handleNavDown processes downward navigation.
func (m Model) handleNavDown() (Model, tea.Cmd) {
	if m.focus == FocusResults && len(m.results) > 0 {
		if m.selectedIdx < len(m.results)-1 {
			m.selectedIdx++
			m.resultsList.Select(m.selectedIdx)
			m.updateDetailPanel()
		}
	} else if m.focus == FocusDetails {
		var cmd tea.Cmd
		m.details, cmd = m.details.Update(tea.KeyMsg{Type: tea.KeyDown})
		return m, cmd
	}
	return m, nil
}

// handleNavUp processes upward navigation.
func (m Model) handleNavUp() (Model, tea.Cmd) {
	if m.focus == FocusResults && len(m.results) > 0 {
		if m.selectedIdx > 0 {
			m.selectedIdx--
			m.resultsList.Select(m.selectedIdx)
			m.updateDetailPanel()
		}
	} else if m.focus == FocusDetails {
		var cmd tea.Cmd
		m.details, cmd = m.details.Update(tea.KeyMsg{Type: tea.KeyUp})
		return m, cmd
	}
	return m, nil
}

// handlePickerKey handles key events for all picker views.
// The picker's callbacks produce domain-specific messages.
func (m Model) handlePickerKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.picker, cmd = m.picker.Update(msg)
	return m, cmd
}

// openPriorityPicker opens the priority picker overlay.
func (m Model) openPriorityPicker(msg details.OpenPriorityPickerMsg) (Model, tea.Cmd) {
	issueID := msg.IssueID
	m.picker = picker.NewWithConfig(picker.Config{
		Title:    "Priority",
		Options:  shared.PriorityOptions(),
		Selected: int(msg.Current),
		OnSelect: func(opt picker.Option) tea.Msg {
			priority := beads.Priority(opt.Value[1] - '0') // Parse "P0"-"P4"
			return prioritySelectedMsg{issueID: issueID, priority: priority}
		},
		OnCancel: func() tea.Msg {
			return pickerCancelledMsg{}
		},
	}).SetSize(m.width, m.height)
	// Store the issue being edited
	if m.selectedIdx >= 0 && m.selectedIdx < len(m.results) {
		issue := m.results[m.selectedIdx]
		m.selectedIssue = &issue
	}
	m.view = ViewPriorityPicker
	return m, nil
}

// openStatusPicker opens the status picker overlay.
func (m Model) openStatusPicker(msg details.OpenStatusPickerMsg) (Model, tea.Cmd) {
	issueID := msg.IssueID
	m.picker = picker.NewWithConfig(picker.Config{
		Title:    "Status",
		Options:  shared.StatusOptions(),
		Selected: picker.FindIndexByValue(shared.StatusOptions(), string(msg.Current)),
		OnSelect: func(opt picker.Option) tea.Msg {
			status := beads.Status(opt.Value)
			return statusSelectedMsg{issueID: issueID, status: status}
		},
		OnCancel: func() tea.Msg {
			return pickerCancelledMsg{}
		},
	}).SetSize(m.width, m.height)
	// Store the issue being edited
	if m.selectedIdx >= 0 && m.selectedIdx < len(m.results) {
		issue := m.results[m.selectedIdx]
		m.selectedIssue = &issue
	}
	m.view = ViewStatusPicker
	return m, nil
}

// updateDetailPanel updates the detail panel with the currently selected issue.
func (m *Model) updateDetailPanel() {
	if m.selectedIdx >= 0 && m.selectedIdx < len(m.results) {
		issue := m.results[m.selectedIdx]
		rightWidth := m.width - (m.width / 2) - 1
		// rightWidth-2 for left/right border, height-2 for top/bottom border
		// Pass nil for loaders if Executor/Client is nil to avoid interface nil vs typed-nil issues
		var depLoader details.DependencyLoader
		var commentLoader details.CommentLoader
		if m.services.Executor != nil {
			depLoader = m.services.Executor
		}
		if m.services.Client != nil {
			commentLoader = m.services.Client
		}
		m.details = details.New(issue, depLoader, commentLoader).SetSize(rightWidth-2, m.height-2)
		m.hasDetail = true
	}
}

// updateDetailFromTree updates the detail panel with the currently selected tree node.
func (m *Model) updateDetailFromTree() {
	if m.tree == nil {
		return
	}
	node := m.tree.SelectedNode()
	if node == nil {
		return
	}
	rightWidth := m.width - (m.width / 2) - 1
	// rightWidth-2 for left/right border, height-2 for top/bottom border
	// Pass nil for loaders if Executor/Client is nil to avoid interface nil vs typed-nil issues
	var depLoader details.DependencyLoader
	var commentLoader details.CommentLoader
	if m.services.Executor != nil {
		depLoader = m.services.Executor
	}
	if m.services.Client != nil {
		commentLoader = m.services.Client
	}
	m.details = details.New(node.Issue, depLoader, commentLoader).SetSize(rightWidth-2, m.height-2)
	m.hasDetail = true
}

// refocusTree refocuses the tree on the currently selected node.
func (m Model) refocusTree() (Model, tea.Cmd) {
	if m.tree == nil {
		return m, nil
	}
	node := m.tree.SelectedNode()
	if node == nil || m.treeRoot == nil || node.Issue.ID == m.treeRoot.ID {
		return m, nil
	}

	// Update root for header display
	m.treeRoot = &node.Issue

	// Refocus tree (pushes old root to stack)
	if err := m.tree.Refocus(node.Issue.ID); err != nil {
		return m, func() tea.Msg {
			return mode.ShowToastMsg{Message: "Failed to refocus tree: " + err.Error(), Style: toaster.StyleError}
		}
	}

	// Update detail panel with new root
	m.updateDetailFromTree()

	return m, nil
}

// switchToTreeSubMode switches from list sub-mode to tree sub-mode for the selected issue.
func (m Model) switchToTreeSubMode() (Model, tea.Cmd) {
	issue := m.getSelectedIssue()
	if issue == nil {
		return m, nil
	}

	// Switch to tree sub-mode
	m.subMode = mode.SubModeTree
	m.treeRoot = issue

	// Load the tree for this issue
	return m, m.loadTree(issue.ID)
}

// treeGoBack navigates to the previous root in the tree history.
func (m Model) treeGoBack() (Model, tea.Cmd) {
	if m.tree == nil {
		return m, nil
	}

	needsRequery, requeryID := m.tree.GoBack()
	if needsRequery && requeryID != "" {
		// Parent not in cache, need to reload tree
		return m, m.loadTree(requeryID)
	}

	// Update header with new root
	if root := m.tree.Root(); root != nil {
		m.treeRoot = &root.Issue
	}

	// Update detail panel
	m.updateDetailFromTree()

	return m, nil
}

// treeGoToOriginal navigates to the original root of the tree.
func (m Model) treeGoToOriginal() (Model, tea.Cmd) {
	if m.tree == nil {
		return m, nil
	}

	if err := m.tree.GoToOriginal(); err != nil {
		return m, func() tea.Msg {
			return mode.ShowToastMsg{Message: "Failed to return to original: " + err.Error(), Style: toaster.StyleError}
		}
	}

	// Update header with original root
	if root := m.tree.Root(); root != nil {
		m.treeRoot = &root.Issue
	}

	// Update detail panel
	m.updateDetailFromTree()

	return m, nil
}

// toggleTreeDirection toggles the tree direction and reloads.
func (m Model) toggleTreeDirection() (Model, tea.Cmd) {
	if m.tree == nil || m.treeRoot == nil {
		return m, nil
	}

	// Toggle direction
	newDir := tree.DirectionDown
	if m.tree.Direction() == tree.DirectionDown {
		newDir = tree.DirectionUp
	}
	m.tree.SetDirection(newDir)

	// Reload tree with new direction
	return m, m.loadTree(m.treeRoot.ID)
}

// toggleTreeMode toggles between deps and children modes and reloads.
func (m Model) toggleTreeMode() (Model, tea.Cmd) {
	if m.tree == nil || m.treeRoot == nil {
		return m, nil
	}

	// Toggle mode
	m.tree.ToggleMode()

	// Reload tree with new mode
	return m, m.loadTree(m.treeRoot.ID)
}

// yankTreeIssueID copies the selected tree node's issue ID to clipboard.
func (m Model) yankTreeIssueID() (Model, tea.Cmd) {
	if m.tree == nil {
		return m, func() tea.Msg { return mode.ShowToastMsg{Message: "No tree loaded", Style: toaster.StyleError} }
	}

	node := m.tree.SelectedNode()
	if node == nil {
		return m, func() tea.Msg { return mode.ShowToastMsg{Message: "No issue selected", Style: toaster.StyleError} }
	}

	if err := shared.CopyToClipboard(node.Issue.ID); err != nil {
		return m, func() tea.Msg {
			return mode.ShowToastMsg{Message: "Clipboard error: " + err.Error(), Style: toaster.StyleError}
		}
	}

	return m, func() tea.Msg {
		return mode.ShowToastMsg{Message: "Copied: " + node.Issue.ID, Style: toaster.StyleSuccess}
	}
}

// getSelectedIssue returns a pointer to the currently selected issue, or nil if none.
func (m Model) getSelectedIssue() *beads.Issue {
	// Tree sub-mode: get selected node's issue
	if m.subMode == mode.SubModeTree && m.tree != nil {
		if node := m.tree.SelectedNode(); node != nil {
			return &node.Issue
		}
		return nil
	}

	// List sub-mode: get from results
	if m.selectedIdx >= 0 && m.selectedIdx < len(m.results) {
		issue := m.results[m.selectedIdx]
		return &issue
	}
	return nil
}

// executeSearch runs the BQL query and returns results.
func (m Model) executeSearch() tea.Cmd {
	query := m.input.Value()
	executor := m.services.Executor

	return func() tea.Msg {
		issues, err := executor.Execute(query)
		return searchResultsMsg{issues: issues, err: err}
	}
}

// loadTree creates a command to load tree data for an issue.
func (m Model) loadTree(rootID string) tea.Cmd {
	executor := m.services.Executor
	dir := tree.DirectionDown
	if m.tree != nil {
		dir = m.tree.Direction()
	}

	expandDir := "down"
	if dir == tree.DirectionUp {
		expandDir = "up"
	}
	query := fmt.Sprintf(`id = "%s" expand %s depth *`, rootID, expandDir)

	return func() tea.Msg {
		issues, err := executor.Execute(query)
		return treeLoadedMsg{
			Issues: issues,
			RootID: rootID,
			Err:    err,
		}
	}
}

// handleSearchResults processes the search results message.
func (m Model) handleSearchResults(msg searchResultsMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.searchErr = msg.err
		// Clear results so stale data doesn't show, but keep detail panel
		// so user can still navigate to it with 'l'
		m.results = nil
		m.resultsList.SetItems([]list.Item{})
		return m, nil
	}

	m.searchErr = nil
	m.showSearchErr = false // Clear error display on successful search
	m.results = msg.issues

	// Convert to list items
	items := make([]list.Item, len(msg.issues))
	for i, issue := range msg.issues {
		items[i] = issueItem{issue: issue}
	}
	m.resultsList.SetItems(items)

	// Select first item and show details
	if len(msg.issues) > 0 {
		m.selectedIdx = 0
		m.resultsList.Select(0)
		m.updateDetailPanel()
	} else {
		m.hasDetail = false
	}

	return m, nil
}

// handleTreeLoaded processes tree loading results and initializes the tree model.
func (m Model) handleTreeLoaded(msg treeLoadedMsg) (Model, tea.Cmd) {
	if msg.Err != nil {
		return m, func() tea.Msg {
			return mode.ShowToastMsg{Message: "Error loading tree: " + msg.Err.Error(), Style: toaster.StyleError}
		}
	}

	// Find root issue
	var root *beads.Issue
	for i := range msg.Issues {
		if msg.Issues[i].ID == msg.RootID {
			root = &msg.Issues[i]
			break
		}
	}

	if root == nil {
		return m, func() tea.Msg {
			return mode.ShowToastMsg{Message: "Root issue not found: " + msg.RootID, Style: toaster.StyleError}
		}
	}

	// Build issue map for tree
	issueMap := make(map[string]*beads.Issue, len(msg.Issues))
	for i := range msg.Issues {
		issueMap[msg.Issues[i].ID] = &msg.Issues[i]
	}

	// Determine direction and mode (preserve existing if tree exists, else defaults)
	dir := tree.DirectionDown
	treeMode := tree.ModeDeps
	var previousSelectedID string
	if m.tree != nil {
		dir = m.tree.Direction()
		treeMode = m.tree.Mode()
		// Save selected issue ID to restore cursor position after rebuild
		if node := m.tree.SelectedNode(); node != nil {
			previousSelectedID = node.Issue.ID
		}
	}

	// Initialize tree model
	m.treeRoot = root
	m.tree = tree.New(msg.RootID, issueMap, dir, treeMode)

	// Set tree size based on available space (must be done before restoring cursor)
	leftWidth := m.width / 2
	// Tree height accounts for: borders (2)
	treeHeight := max(m.height-2, 1)
	m.tree.SetSize(leftWidth-2, treeHeight) // -2 for left/right border

	// Restore cursor to previously selected issue if it exists in new tree
	if previousSelectedID != "" {
		m.tree.SelectByIssueID(previousSelectedID)
	}

	// Update details panel with selected node
	m.updateDetailFromTree()

	return m, nil
}

// navigateToDependency loads and displays a dependency issue in the details panel.
func (m Model) navigateToDependency(issueID string) (Model, tea.Cmd) {
	if m.services.Executor == nil {
		return m, nil
	}

	// Load the issue by ID using BQL executor
	query := bql.BuildIDQuery([]string{issueID})
	issues, err := m.services.Executor.Execute(query)
	if err != nil || len(issues) == 0 {
		return m, func() tea.Msg {
			return mode.ShowToastMsg{Message: "Issue not found: " + issueID, Style: toaster.StyleError}
		}
	}

	issue := issues[0]

	// Update the details panel with this issue
	rightWidth := m.width - (m.width / 2) - 1
	// rightWidth-2 for left/right border, height-2 for top/bottom border
	// Pass nil for loaders if Executor/Client is nil to avoid interface nil vs typed-nil issues
	var depLoader details.DependencyLoader
	var commentLoader details.CommentLoader
	if m.services.Executor != nil {
		depLoader = m.services.Executor
	}
	if m.services.Client != nil {
		commentLoader = m.services.Client
	}
	m.details = details.New(issue, depLoader, commentLoader).SetSize(rightWidth-2, m.height-2)
	m.hasDetail = true

	// Try to find and select this issue in the results list
	if m.subMode == mode.SubModeList {
		for i, result := range m.results {
			if result.ID == issueID {
				m.selectedIdx = i
				m.resultsList.Select(i)
				break
			}
		}
	}

	// If in tree mode, select the node in the tree
	if m.subMode == mode.SubModeTree && m.tree != nil {
		m.tree.SelectByIssueID(issueID)
	}

	return m, nil
}

// renderMainView renders the 50/50 split layout.
func (m Model) renderMainView() string {
	// Calculate widths (small gap between panels)
	gap := 1
	leftWidth := m.width / 2
	rightWidth := m.width - leftWidth - gap

	// Left panel: search + results
	leftPanel := m.renderLeftPanel(leftWidth)

	// Right panel: details
	rightPanel := m.renderRightPanel(rightWidth)

	// Join horizontally with gap
	content := lipgloss.JoinHorizontal(
		lipgloss.Top,
		leftPanel,
		strings.Repeat(" ", gap),
		rightPanel,
	)

	return content
}

// renderLeftPanel renders the left panel, switching between list and tree sub-modes.
func (m Model) renderLeftPanel(width int) string {
	switch m.subMode {
	case mode.SubModeTree:
		return m.renderTreeLeftPanel(width)
	default:
		return m.renderListLeftPanel(width)
	}
}

// renderListLeftPanel renders the left panel with search input and results (list sub-mode).
func (m Model) renderListLeftPanel(width int) string {
	var sb strings.Builder

	// Calculate heights dynamically based on input content
	inputContentHeight := m.input.Height()  // lines of wrapped text
	inputHeight := inputContentHeight + 2   // add 2 for borders
	resultsHeight := m.height - inputHeight // fills remaining space

	// BQL Search input with titled border
	inputContent := m.input.View()
	inputBorder := styles.RenderWithTitleBorder(
		inputContent,
		"BQL Search",
		"",
		width,
		inputHeight,
		m.focus == FocusSearch,
		styles.OverlayTitleColor,
		styles.BorderHighlightFocusColor,
	)
	sb.WriteString(inputBorder)
	sb.WriteString("\n")

	// Build results content
	var resultsContent string
	if m.searchErr != nil && m.showSearchErr {
		// Only show error after blur, not while typing
		errStyle := lipgloss.NewStyle().
			Foreground(styles.StatusErrorColor).
			Padding(1, 2)
		resultsContent = errStyle.Render("Error: " + m.searchErr.Error())
	} else if len(m.results) == 0 && m.input.Value() != "" {
		emptyStyle := lipgloss.NewStyle().
			Foreground(styles.TextSecondaryColor).
			Italic(true).
			Padding(1, 2)
		resultsContent = emptyStyle.Render("No results found")
	} else if len(m.results) > 0 {
		resultsContent = m.resultsList.View()
	} else {
		emptyStyle := lipgloss.NewStyle().
			Foreground(styles.TextSecondaryColor).
			Italic(true).
			Padding(1, 2)
		resultsContent = emptyStyle.Render("Enter a BQL query to search")
	}

	// Results title with count if we have results
	resultsTitle := "Results"
	if len(m.results) > 0 {
		resultsTitle = fmt.Sprintf("Results (%d)", len(m.results))
	}

	// Results with titled border
	resultsBorder := styles.RenderWithTitleBorder(
		resultsContent,
		resultsTitle,
		"",
		width,
		resultsHeight,
		m.focus == FocusResults,
		styles.OverlayTitleColor,
		styles.BorderHighlightFocusColor,
	)
	sb.WriteString(resultsBorder)

	return sb.String()
}

// renderRightPanel renders the right panel with issue details.
func (m Model) renderRightPanel(width int) string {
	panelHeight := m.height

	var content string
	if !m.hasDetail {
		// Empty state
		emptyStyle := lipgloss.NewStyle().
			Foreground(styles.TextSecondaryColor).
			Padding(1, 2)
		content = emptyStyle.Render("Select an issue to view details")
	} else {
		content = m.details.View()
	}

	// Wrap in titled border
	return styles.RenderWithTitleBorder(
		content,
		"Issue Details",
		"",
		width,
		panelHeight,
		m.focus == FocusDetails,
		styles.OverlayTitleColor,
		styles.BorderHighlightFocusColor,
	)
}

// renderCompactProgress renders a compact progress bar with percentage and counts.
func renderCompactProgress(closed, total int) string {
	if total == 0 {
		return ""
	}
	percent := float64(closed) / float64(total) * 100
	barWidth := 15
	filledWidth := int(float64(barWidth) * float64(closed) / float64(total))

	filledStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)
	emptyStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)

	filled := filledStyle.Render(strings.Repeat("█", filledWidth))
	empty := emptyStyle.Render(strings.Repeat("░", barWidth-filledWidth))

	return fmt.Sprintf("%s%s %.0f%% (%d/%d)", filled, empty, percent, closed, total)
}

// renderTreeLeftPanel renders the left panel with tree content (tree sub-mode).
func (m Model) renderTreeLeftPanel(width int) string {
	// Tree content with left padding for breathing room
	var content string
	if m.tree != nil {
		content = lipgloss.NewStyle().PaddingLeft(1).Render(m.tree.View())
	} else {
		emptyStyle := lipgloss.NewStyle().
			Foreground(styles.TextSecondaryColor).
			Italic(true).
			PaddingLeft(1)
		content = emptyStyle.Render("Loading tree...")
	}

	// Left title: direction and mode indicators
	dir := "↓ down"
	if m.tree != nil && m.tree.Direction() == tree.DirectionUp {
		dir = "↑ up"
	}
	mode := "deps"
	if m.tree != nil && m.tree.Mode() == tree.ModeChildren {
		mode = "children"
	}
	leftTitle := fmt.Sprintf("Tree (%s) (%s)", dir, mode)

	// Right title: progress bar
	var rightTitle string
	if m.tree != nil && m.tree.Root() != nil {
		closed, total := m.tree.Root().CalculateProgress()
		rightTitle = renderCompactProgress(closed, total)
	}

	return styles.RenderWithTitleBorder(
		content,
		leftTitle,
		rightTitle,
		width,
		m.height,
		m.focus == FocusResults, // Tree panel uses "results" focus
		styles.OverlayTitleColor,
		styles.BorderHighlightFocusColor,
	)
}

// Message types

// searchResultsMsg carries the results of a BQL query.
type searchResultsMsg struct {
	issues []beads.Issue
	err    error
}

// treeLoadedMsg carries the results of loading a tree for an issue.
type treeLoadedMsg struct {
	Issues []beads.Issue
	RootID string
	Err    error
}

// priorityChangedMsg signals completion of a priority update.
type priorityChangedMsg struct {
	issueID  string
	priority beads.Priority
	err      error
}

// statusChangedMsg signals completion of a status update.
type statusChangedMsg struct {
	issueID string
	status  beads.Status
	err     error
}

// Picker callback messages (produced by picker OnSelect/OnCancel callbacks)

// prioritySelectedMsg is produced when a priority is selected in the picker.
type prioritySelectedMsg struct {
	issueID  string
	priority beads.Priority
}

// statusSelectedMsg is produced when a status is selected in the picker.
type statusSelectedMsg struct {
	issueID string
	status  beads.Status
}

// pickerCancelledMsg is produced when any picker is cancelled.
type pickerCancelledMsg struct{}

// saveActionExistingViewMsg is produced when "existing view" is selected in save action picker.
type saveActionExistingViewMsg struct {
	query string
}

// saveActionNewViewMsg is produced when "new view" is selected in save action picker.
type saveActionNewViewMsg struct {
	query string
}

// editMenuLabelsMsg is produced when "labels" is selected in edit menu picker.
type editMenuLabelsMsg struct{}

// editMenuPriorityMsg is produced when "priority" is selected in edit menu picker.
type editMenuPriorityMsg struct{}

// editMenuStatusMsg is produced when "status" is selected in edit menu picker.
type editMenuStatusMsg struct{}

// debounceSearchMsg triggers a search after debounce delay.
type debounceSearchMsg struct {
	version int // Only execute if this matches current searchVersion
}

// SaveSearchAsColumnMsg is sent when user saves a search as a column.
// This bubbles up to the app level for config persistence.
type SaveSearchAsColumnMsg struct {
	ColumnName  string
	Color       string
	Query       string
	ViewIndices []int
}

// SaveSearchToNewViewMsg is sent when user creates a new view from search.
// This bubbles up to the app level for config persistence.
type SaveSearchToNewViewMsg struct {
	ViewName   string
	ColumnName string
	Color      string
	Query      string
}

// ExitToKanbanMsg requests switching back to kanban mode.
// This is sent when the user presses ESC in the main search view (not in an overlay).
type ExitToKanbanMsg struct{}

// Async commands

// debounceSearch creates a command that waits then triggers a search.
func debounceSearch(version int, delay time.Duration) tea.Cmd {
	return tea.Tick(delay, func(t time.Time) tea.Msg {
		return debounceSearchMsg{version: version}
	})
}

// updatePriorityCmd creates a command to update an issue's priority.
func updatePriorityCmd(issueID string, priority beads.Priority) tea.Cmd {
	return func() tea.Msg {
		err := beads.UpdatePriority(issueID, priority)
		return priorityChangedMsg{issueID: issueID, priority: priority, err: err}
	}
}

// updateStatusCmd creates a command to update an issue's status.
func updateStatusCmd(issueID string, status beads.Status) tea.Cmd {
	return func() tea.Msg {
		err := beads.UpdateStatus(issueID, status)
		return statusChangedMsg{issueID: issueID, status: status, err: err}
	}
}

// HandleDBChanged processes database change notifications from the app.
// This is called by app.go when the centralized watcher detects changes.
// The app handles re-subscription; this method just triggers the refresh.
func (m Model) HandleDBChanged() (Model, tea.Cmd) {
	switch m.subMode {
	case mode.SubModeTree:
		// Reload tree with current root
		if m.treeRoot != nil {
			return m, m.loadTree(m.treeRoot.ID)
		}
		return m, nil
	default:
		// Re-execute current search for list sub-mode
		return m, m.executeSearch()
	}
}

// Message handlers

// handlePriorityChanged processes priority change results.
func (m Model) handlePriorityChanged(msg priorityChangedMsg) (Model, tea.Cmd) {
	m.selectedIssue = nil
	if msg.err != nil {
		return m, func() tea.Msg {
			return mode.ShowToastMsg{Message: "Error: " + msg.err.Error(), Style: toaster.StyleError}
		}
	}

	// Update the details panel to show new priority
	m.details = m.details.UpdatePriority(msg.priority)

	// Update the issue in our results list
	for i := range m.results {
		if m.results[i].ID == msg.issueID {
			m.results[i].Priority = msg.priority
			break
		}
	}
	// Refresh list items
	items := make([]list.Item, len(m.results))
	for i, issue := range m.results {
		items[i] = issueItem{issue: issue}
	}
	m.resultsList.SetItems(items)

	return m, func() tea.Msg { return mode.ShowToastMsg{Message: "Priority updated", Style: toaster.StyleSuccess} }
}

// handleStatusChanged processes status change results.
func (m Model) handleStatusChanged(msg statusChangedMsg) (Model, tea.Cmd) {
	m.selectedIssue = nil
	if msg.err != nil {
		return m, func() tea.Msg {
			return mode.ShowToastMsg{Message: "Error: " + msg.err.Error(), Style: toaster.StyleError}
		}
	}

	// Update the details panel to show new status
	m.details = m.details.UpdateStatus(msg.status)

	// Update the issue in our results list
	for i := range m.results {
		if m.results[i].ID == msg.issueID {
			m.results[i].Status = msg.status
			break
		}
	}
	// Refresh list items
	items := make([]list.Item, len(m.results))
	for i, issue := range m.results {
		items[i] = issueItem{issue: issue}
	}
	m.resultsList.SetItems(items)

	return m, func() tea.Msg { return mode.ShowToastMsg{Message: "Status updated", Style: toaster.StyleSuccess} }
}

// yankIssueID copies the selected issue ID to clipboard.
func (m Model) yankIssueID() (Model, tea.Cmd) {
	if m.selectedIdx < 0 || m.selectedIdx >= len(m.results) {
		return m, func() tea.Msg { return mode.ShowToastMsg{Message: "No issue selected", Style: toaster.StyleError} }
	}

	issue := m.results[m.selectedIdx]
	if err := shared.CopyToClipboard(issue.ID); err != nil {
		return m, func() tea.Msg {
			return mode.ShowToastMsg{Message: "Clipboard error: " + err.Error(), Style: toaster.StyleError}
		}
	}

	return m, func() tea.Msg { return mode.ShowToastMsg{Message: "Copied: " + issue.ID, Style: toaster.StyleSuccess} }
}

// issueItem wraps beads.Issue for the list component.
type issueItem struct {
	issue beads.Issue
}

// FilterValue implements list.Item interface.
func (i issueItem) FilterValue() string { return i.issue.TitleText }

// issueDelegate renders issues in board style.
type issueDelegate struct{}

func newIssueDelegate() issueDelegate {
	return issueDelegate{}
}

// Height returns the height of a single list item.
func (d issueDelegate) Height() int { return 1 }

// Spacing returns the spacing between list items.
func (d issueDelegate) Spacing() int { return 0 }

// Update handles updates for list items (no-op for read-only display).
func (d issueDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

// Render renders a single list item.
func (d issueDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	issue := item.(issueItem).issue

	// Format: > [T][P2][id] Title...
	selected := index == m.Index()

	prefix := " "
	if selected {
		prefix = styles.SelectionIndicatorStyle.Render(">")
	}

	typeText := styles.GetTypeIndicator(issue.Type)
	typeStyle := styles.GetTypeStyle(issue.Type)

	priorityText := fmt.Sprintf("[P%d]", issue.Priority)
	priorityStyle := styles.GetPriorityStyle(issue.Priority)

	idStyle := lipgloss.NewStyle().Foreground(styles.TextSecondaryColor)
	idText := fmt.Sprintf("[%s]", issue.ID)

	line := fmt.Sprintf("%s%s%s%s %s",
		prefix,
		typeStyle.Render(typeText),
		priorityStyle.Render(priorityText),
		idStyle.Render(idText),
		issue.TitleText,
	)

	_, _ = fmt.Fprint(w, line)
}

// openDeleteConfirm opens the delete confirmation modal.
func (m Model) openDeleteConfirm(msg details.DeleteIssueMsg) (Model, tea.Cmd) {
	// Find the issue to delete
	var issue *beads.Issue
	for i := range m.results {
		if m.results[i].ID == msg.IssueID {
			issue = &m.results[i]
			break
		}
	}
	if issue == nil {
		// Try loading from executor
		if m.services.Executor != nil {
			query := bql.BuildIDQuery([]string{msg.IssueID})
			issues, err := m.services.Executor.Execute(query)
			if err == nil && len(issues) == 1 {
				issue = &issues[0]
			}
		}
	}
	if issue == nil {
		return m, nil
	}

	m.modal, m.deleteIsCascade = shared.CreateDeleteModal(issue, m.services.Executor)
	m.modal.SetSize(m.width, m.height)
	m.selectedIssue = issue
	m.view = ViewDeleteConfirm
	return m, m.modal.Init()
}

// handleModalSubmit processes modal confirmation.
func (m Model) handleModalSubmit(_ modal.SubmitMsg) (Model, tea.Cmd) {
	if m.view == ViewDeleteConfirm {
		if m.selectedIssue != nil {
			issueID := m.selectedIssue.ID
			parentID := m.selectedIssue.ParentID
			cascade := m.deleteIsCascade
			// Determine if this is the tree root being deleted
			wasTreeRoot := m.subMode == mode.SubModeTree &&
				m.treeRoot != nil &&
				m.selectedIssue.ID == m.treeRoot.ID
			m.selectedIssue = nil
			m.deleteIsCascade = false
			return m, deleteIssueCmd(issueID, parentID, wasTreeRoot, cascade)
		}
		m.view = ViewSearch
		m.deleteIsCascade = false
		return m, nil
	}
	return m, nil
}

// handleModalCancel processes modal cancellation.
func (m Model) handleModalCancel() (Model, tea.Cmd) {
	if m.view == ViewDeleteConfirm {
		m.view = ViewSearch
		m.selectedIssue = nil
		m.deleteIsCascade = false
		return m, nil
	}
	return m, nil
}

// handleIssueDeleted processes issue deletion results.
func (m Model) handleIssueDeleted(msg issueDeletedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.view = ViewSearch
		m.selectedIssue = nil
		return m, func() tea.Msg {
			return mode.ShowToastMsg{Message: "Error: " + msg.err.Error(), Style: toaster.StyleError}
		}
	}

	m.view = ViewSearch
	m.selectedIssue = nil

	// Tree sub-mode: refresh tree or exit to kanban
	if m.subMode == mode.SubModeTree {
		if msg.wasTreeRoot {
			if msg.parentID != "" {
				// Re-root tree to parent
				return m, tea.Batch(
					m.loadTree(msg.parentID),
					func() tea.Msg { return mode.ShowToastMsg{Message: "Issue deleted", Style: toaster.StyleSuccess} },
				)
			}
			// No parent - exit to kanban
			return m, tea.Batch(
				func() tea.Msg { return ExitToKanbanMsg{} },
				func() tea.Msg { return mode.ShowToastMsg{Message: "Issue deleted", Style: toaster.StyleSuccess} },
			)
		}
		// Non-root deleted - refresh with same root
		return m, tea.Batch(
			m.loadTree(m.treeRoot.ID),
			func() tea.Msg { return mode.ShowToastMsg{Message: "Issue deleted", Style: toaster.StyleSuccess} },
		)
	}

	// List sub-mode: existing behavior
	return m, tea.Batch(
		m.executeSearch(),
		func() tea.Msg { return mode.ShowToastMsg{Message: "Issue deleted", Style: toaster.StyleSuccess} },
	)
}

// handleLabelsChanged processes label change results.
func (m Model) handleLabelsChanged(msg labelsChangedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		return m, func() tea.Msg {
			return mode.ShowToastMsg{Message: "Error: " + msg.err.Error(), Style: toaster.StyleError}
		}
	}

	// Update details view to show new labels
	m.details = m.details.UpdateLabels(msg.labels)

	// Update the issue in our results list
	for i := range m.results {
		if m.results[i].ID == msg.issueID {
			m.results[i].Labels = msg.labels
			break
		}
	}

	return m, func() tea.Msg { return mode.ShowToastMsg{Message: "Labels updated", Style: toaster.StyleSuccess} }
}

// Message types for delete and label operations

type issueDeletedMsg struct {
	issueID     string
	parentID    string // Parent of deleted issue (for re-rooting tree)
	wasTreeRoot bool   // True if deleted issue was tree root
	err         error
}

type labelsChangedMsg struct {
	issueID string
	labels  []string
	err     error
}

// Async commands

func deleteIssueCmd(issueID, parentID string, wasTreeRoot, cascade bool) tea.Cmd {
	return func() tea.Msg {
		var err error
		if cascade {
			err = beads.DeleteIssueCascade(issueID)
		} else {
			err = beads.DeleteIssue(issueID)
		}
		return issueDeletedMsg{
			issueID:     issueID,
			parentID:    parentID,
			wasTreeRoot: wasTreeRoot,
			err:         err,
		}
	}
}

func setLabelsCmd(issueID string, labels []string) tea.Cmd {
	return func() tea.Msg {
		err := beads.SetLabels(issueID, labels)
		return labelsChangedMsg{issueID: issueID, labels: labels, err: err}
	}
}
