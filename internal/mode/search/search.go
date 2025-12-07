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
	"perles/internal/config"
	"perles/internal/mode"
	"perles/internal/mode/shared"
	"perles/internal/ui/board"
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

	// Search state
	input         bqlinput.Model
	results       []beads.Issue
	resultsList   list.Model
	selectedIdx   int
	searchErr     error
	showSearchErr bool // Only show error after blur, not during typing
	searchVersion int  // Incremented on each input change for debounce

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
	// Execute initial search
	return m.executeSearch()
}

// SetQuery sets the initial search query and returns the modified model.
func (m Model) SetQuery(query string) Model {
	m.input.SetValue(query)
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

	return m
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case searchResultsMsg:
		return m.handleSearchResults(msg)

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

	case details.CopyIssueIDMsg:
		err := shared.CopyToClipboard(msg.IssueID)
		if err != nil {
			return m, func() tea.Msg {
				return mode.ShowToastMsg{Message: "Clipboard error: " + err.Error(), Style: toaster.StyleError}
			}
		}
		return m, func() tea.Msg { return mode.ShowToastMsg{Message: "Copied: " + msg.IssueID, Style: toaster.StyleSuccess} }

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

	// Delegate to details view if focused and message wasn't handled above
	if m.focus == FocusDetails {
		var cmd tea.Cmd
		m.details, cmd = m.details.Update(msg)
		return m, cmd
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

	// Not in search input - handle navigation and global keys
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit

	case "esc":
		// Exit search mode back to kanban
		return m, func() tea.Msg { return ExitToKanbanMsg{} }

	case "?":
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
		// Cycle focus forward: Search -> Results -> Details -> Search
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
		// Cycle focus backward: Details -> Results -> Search -> Details
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
		// Cycle focus forward: Search -> Results -> Details -> Search
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

	case "t":
		if m.focus == FocusDetails {
			issue := m.getSelectedIssue()
			if issue == nil {
				return m, nil
			}
			// Set up loaders
			var depLoader details.DependencyLoader
			var commentLoader details.CommentLoader
			if m.services.Client != nil {
				depLoader = m.services.Client
				commentLoader = m.services.Client
			}
			// Create new details model for the selected issue
			m.details = details.New(*issue, depLoader, commentLoader, m.services.Executor).SetSize(m.width-(m.width/2)-1-2, m.height-2)
			var cmd tea.Cmd
			m.details, cmd = m.details.ShowTree() // Activate tree view immediately
			m.view = ViewSearch                    // Remain in search view
			return m, cmd
		}
		return m, nil

	case "enter":
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
		// Pass nil for loaders if Client is nil to avoid interface nil vs typed-nil issues
		var depLoader details.DependencyLoader
		var commentLoader details.CommentLoader
		if m.services.Client != nil {
			depLoader = m.services.Client
			commentLoader = m.services.Client
		}
		m.details = details.New(issue, depLoader, commentLoader, m.services.Executor).SetSize(rightWidth-2, m.height-2)
		m.hasDetail = true
	}
}

// getSelectedIssue returns a pointer to the currently selected issue, or nil if none.
func (m Model) getSelectedIssue() *beads.Issue {
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

// navigateToDependency loads and displays a dependency issue in the details panel.
func (m Model) navigateToDependency(issueID string) (Model, tea.Cmd) {
	if m.services.Client == nil {
		return m, nil
	}

	// Load the issue by ID
	issues, err := m.services.Client.ListIssuesByIds([]string{issueID})
	if err != nil || len(issues) == 0 {
		return m, func() tea.Msg {
			return mode.ShowToastMsg{Message: "Issue not found: " + issueID, Style: toaster.StyleError}
		}
	}

	issue := issues[0]

	// Update the details panel with this issue
	rightWidth := m.width - (m.width / 2) - 1
	// rightWidth-2 for left/right border, height-2 for top/bottom border
	// Pass nil for loaders if Client is nil to avoid interface nil vs typed-nil issues
	var depLoader details.DependencyLoader
	var commentLoader details.CommentLoader
	if m.services.Client != nil {
		depLoader = m.services.Client
		commentLoader = m.services.Client
	}
	m.details = details.New(issue, depLoader, commentLoader, m.services.Executor).SetSize(rightWidth-2, m.height-2)
	m.hasDetail = true

	// Try to find and select this issue in the results list
	for i, result := range m.results {
		if result.ID == issueID {
			m.selectedIdx = i
			m.resultsList.Select(i)
			break
		}
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

// renderLeftPanel renders the left panel with search input and results.
func (m Model) renderLeftPanel(width int) string {
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
		width,
		panelHeight,
		m.focus == FocusDetails,
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
	// Re-execute current search
	return m, m.executeSearch()
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

	typeText := board.GetTypeIndicator(issue.Type)
	typeStyle := board.GetTypeStyle(issue.Type)

	priorityText := fmt.Sprintf("[P%d]", issue.Priority)
	priorityStyle := board.GetPriorityStyle(issue.Priority)

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
		// Try loading from client
		if m.services.Client != nil {
			issues, err := m.services.Client.ListIssuesByIds([]string{msg.IssueID})
			if err == nil && len(issues) == 1 {
				issue = &issues[0]
			}
		}
	}
	if issue == nil {
		return m, nil
	}

	m.modal, m.deleteIsCascade = shared.CreateDeleteModal(issue, m.services.Client)
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
			cascade := m.deleteIsCascade
			m.selectedIssue = nil
			m.deleteIsCascade = false
			return m, deleteIssueCmd(issueID, cascade)
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

	// Return to search, refresh results to remove deleted issue
	m.view = ViewSearch
	m.selectedIssue = nil
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
	issueID string
	err     error
}

type labelsChangedMsg struct {
	issueID string
	labels  []string
	err     error
}

// Async commands

func deleteIssueCmd(issueID string, cascade bool) tea.Cmd {
	return func() tea.Msg {
		var err error
		if cascade {
			err = beads.DeleteIssueCascade(issueID)
		} else {
			err = beads.DeleteIssue(issueID)
		}
		return issueDeletedMsg{issueID: issueID, err: err}
	}
}

func setLabelsCmd(issueID string, labels []string) tea.Cmd {
	return func() tea.Msg {
		err := beads.SetLabels(issueID, labels)
		return labelsChangedMsg{issueID: issueID, labels: labels, err: err}
	}
}
