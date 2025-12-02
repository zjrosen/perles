// Package search implements the search mode controller for BQL-powered issue search.
package search

import (
	"fmt"
	"io"
	"os/exec"
	"perles/internal/beads"
	"perles/internal/mode"
	"perles/internal/ui/board"
	"perles/internal/ui/bqlinput"
	"perles/internal/ui/colorpicker"
	"perles/internal/ui/details"
	"perles/internal/ui/help"
	"perles/internal/ui/labeleditor"
	"perles/internal/ui/modal"
	"perles/internal/ui/newviewmodal"
	"perles/internal/ui/picker"
	"perles/internal/ui/saveactionpicker"
	"perles/internal/ui/styles"
	"perles/internal/ui/toaster"
	"perles/internal/ui/viewselector"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	ViewSaveAction    // Action picker: existing vs new view
	ViewNewView       // New view modal
	ViewDeleteConfirm // Delete issue confirmation modal
	ViewLabelEditor   // Label editor modal
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
	toaster       toaster.Model
	selectedIssue *beads.Issue // Issue being edited in picker
	viewSelector  viewselector.Model
	actionPicker  saveactionpicker.Model
	newViewModal  newviewmodal.Model
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
		toaster:     toaster.New(),
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
	inputWidth := leftWidth - 4 // Padding
	if inputWidth < 1 {
		inputWidth = 1
	}
	m.input.SetWidth(inputWidth)

	// Update results list
	listHeight := height - 5 // Input row + header + status + borders
	if listHeight < 1 {
		listHeight = 1
	}
	listWidth := leftWidth - 2
	if listWidth < 1 {
		listWidth = 1
	}
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

	case toaster.DismissMsg:
		m.toaster = m.toaster.Hide()
		return m, nil

	case viewselector.CancelMsg:
		m.view = ViewSearch
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

	case viewselector.SaveMsg:
		m.view = ViewSearch
		// Show success toast
		count := len(msg.ViewIndices)
		toastMsg := fmt.Sprintf("Column added to %d view(s)", count)
		if count == 1 {
			toastMsg = "Column added to 1 view"
		}
		m.toaster = m.toaster.Show(toastMsg, toaster.StyleSuccess)
		return m, tea.Batch(
			func() tea.Msg {
				return SaveSearchAsColumnMsg{
					ColumnName:  msg.ColumnName,
					Color:       msg.Color,
					Query:       msg.Query,
					ViewIndices: msg.ViewIndices,
				}
			},
			toaster.ScheduleDismiss(3*time.Second),
		)

	case saveactionpicker.SelectMsg:
		switch msg.Action {
		case saveactionpicker.ActionExistingView:
			// Transition to existing view selector
			m.viewSelector = viewselector.New(msg.Query, m.services.Config.Views)
			m.viewSelector = m.viewSelector.SetSize(m.width, m.height)
			m.view = ViewSaveColumn
		case saveactionpicker.ActionNewView:
			// Transition to new view modal
			m.newViewModal = newviewmodal.New(msg.Query).
				SetSize(m.width, m.height).
				SetExistingViews(m.services.Config.Views)
			m.view = ViewNewView
		}
		return m, nil

	case saveactionpicker.CancelMsg:
		m.view = ViewSearch
		return m, nil

	case newviewmodal.SaveMsg:
		m.view = ViewSearch
		m.toaster = m.toaster.Show(fmt.Sprintf("Created view '%s'", msg.ViewName), toaster.StyleSuccess)
		return m, tea.Batch(
			func() tea.Msg {
				return SaveSearchToNewViewMsg{
					ViewName:   msg.ViewName,
					ColumnName: msg.ColumnName,
					Color:      msg.Color,
					Query:      msg.Query,
				}
			},
			toaster.ScheduleDismiss(3*time.Second),
		)

	case newviewmodal.CancelMsg:
		m.view = ViewSearch
		return m, nil

	case details.DeleteIssueMsg:
		return m.openDeleteConfirm(msg)

	case details.OpenLabelEditorMsg:
		m.labelEditor = labeleditor.New(msg.IssueID, msg.Labels).
			SetSize(m.width, m.height)
		m.view = ViewLabelEditor
		return m, m.labelEditor.Init()

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
	case ViewPriorityPicker, ViewStatusPicker:
		return m.picker.Overlay(m.renderMainView())
	case ViewSaveColumn:
		return m.viewSelector.Overlay(m.renderMainView())
	case ViewSaveAction:
		return m.actionPicker.Overlay(m.renderMainView())
	case ViewNewView:
		return m.newViewModal.Overlay(m.renderMainView())
	case ViewDeleteConfirm:
		return m.modal.Overlay(m.renderMainView())
	case ViewLabelEditor:
		return m.labelEditor.Overlay(m.renderMainView())
	}

	// Main view with potential toaster
	view := m.renderMainView()
	if m.toaster.Visible() {
		view = m.toaster.Overlay(view, m.width, m.height)
	}
	return view
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

	case ViewSaveAction:
		var cmd tea.Cmd
		m.actionPicker, cmd = m.actionPicker.Update(msg)
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
		case "esc", "tab", "ctrl+n":
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
				m.toaster = m.toaster.Show("Enter a query first", toaster.StyleWarn)
				return m, toaster.ScheduleDismiss(2 * time.Second)
			}
			// Show action picker to choose between existing view or new view
			m.actionPicker = saveactionpicker.New(query)
			m.actionPicker = m.actionPicker.SetSize(m.width, m.height)
			m.view = ViewSaveAction
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
			m.toaster = m.toaster.Show("Enter a query first", toaster.StyleWarn)
			return m, toaster.ScheduleDismiss(2 * time.Second)
		}
		// Show action picker to choose between existing view or new view
		m.actionPicker = saveactionpicker.New(query)
		m.actionPicker = m.actionPicker.SetSize(m.width, m.height)
		m.view = ViewSaveAction
		return m, nil

	case "h":
		// Move focus left
		if m.focus == FocusDetails {
			if m.details.IsOnLeftEdge() {
				// Already on left edge of details, move to results
				m.focus = FocusResults
			} else {
				// Delegate to details (move from metadata to content)
				var cmd tea.Cmd
				m.details, cmd = m.details.Update(msg)
				return m, cmd
			}
		}
		return m, nil

	case "l":
		// Move focus right
		switch m.focus {
		case FocusResults:
			m.focus = FocusDetails
		case FocusDetails:
			// Delegate to details (move from content to metadata)
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
		return m.handleNavUp()

	case "y":
		// Yank (copy) issue ID to clipboard
		if m.focus == FocusResults || m.focus == FocusDetails {
			return m.yankIssueID()
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

// handlePickerKey processes keyboard input when a picker is active.
func (m Model) handlePickerKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.view = ViewSearch
		m.selectedIssue = nil
		return m, nil

	case "enter":
		// Confirm selection and update
		if m.selectedIssue == nil {
			m.view = ViewSearch
			return m, nil
		}

		selected := m.picker.Selected()
		if m.view == ViewPriorityPicker {
			// Parse priority from "P0"-"P4"
			priority := beads.Priority(selected.Value[1] - '0')
			m.view = ViewSearch
			return m, updatePriorityCmd(m.selectedIssue.ID, priority)
		}
		if m.view == ViewStatusPicker {
			status := beads.Status(selected.Value)
			m.view = ViewSearch
			return m, updateStatusCmd(m.selectedIssue.ID, status)
		}

		m.view = ViewSearch
		return m, nil
	}

	// Delegate navigation to picker's Update method
	var cmd tea.Cmd
	m.picker, cmd = m.picker.Update(msg)
	return m, cmd
}

// openPriorityPicker opens the priority picker overlay.
func (m Model) openPriorityPicker(msg details.OpenPriorityPickerMsg) (Model, tea.Cmd) {
	m.picker = picker.New("Priority", priorityOptions()).
		SetSize(m.width, m.height).
		SetSelected(int(msg.Current))
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
	m.picker = picker.New("Status", statusOptions()).
		SetSize(m.width, m.height).
		SetSelected(picker.FindIndexByValue(statusOptions(), string(msg.Current)))
	// Store the issue being edited
	if m.selectedIdx >= 0 && m.selectedIdx < len(m.results) {
		issue := m.results[m.selectedIdx]
		m.selectedIssue = &issue
	}
	m.view = ViewStatusPicker
	return m, nil
}

// priorityOptions returns picker options for priority levels.
func priorityOptions() []picker.Option {
	return []picker.Option{
		{Label: "P0 - Critical", Value: "P0", Color: styles.PriorityCriticalColor},
		{Label: "P1 - High", Value: "P1", Color: styles.PriorityHighColor},
		{Label: "P2 - Medium", Value: "P2", Color: styles.PriorityMediumColor},
		{Label: "P3 - Low", Value: "P3", Color: styles.PriorityLowColor},
		{Label: "P4 - Backlog", Value: "P4", Color: styles.PriorityBacklogColor},
	}
}

// statusOptions returns picker options for status values.
func statusOptions() []picker.Option {
	return []picker.Option{
		{Label: "Open", Value: string(beads.StatusOpen), Color: styles.StatusOpenColor},
		{Label: "In Progress", Value: string(beads.StatusInProgress), Color: styles.StatusInProgressColor},
		{Label: "Closed", Value: string(beads.StatusClosed), Color: styles.StatusClosedColor},
	}
}

// updateDetailPanel updates the detail panel with the currently selected issue.
func (m *Model) updateDetailPanel() {
	if m.selectedIdx >= 0 && m.selectedIdx < len(m.results) {
		issue := m.results[m.selectedIdx]
		rightWidth := m.width - (m.width / 2) - 1
		// rightWidth-2 for left/right border, height-2 for top/bottom border
		m.details = details.New(issue, m.services.Client).SetSize(rightWidth-2, m.height-2)
		m.hasDetail = true
	}
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
		m.toaster = m.toaster.Show("Issue not found: "+issueID, toaster.StyleError)
		return m, toaster.ScheduleDismiss(2 * time.Second)
	}

	issue := issues[0]

	// Update the details panel with this issue
	rightWidth := m.width - (m.width / 2) - 1
	// rightWidth-2 for left/right border, height-2 for top/bottom border
	m.details = details.New(issue, m.services.Client).SetSize(rightWidth-2, m.height-2)
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
		m.toaster = m.toaster.Show("Error: "+msg.err.Error(), toaster.StyleError)
		return m, toaster.ScheduleDismiss(3 * time.Second)
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

	m.toaster = m.toaster.Show("Priority updated", toaster.StyleSuccess)
	return m, toaster.ScheduleDismiss(2 * time.Second)
}

// handleStatusChanged processes status change results.
func (m Model) handleStatusChanged(msg statusChangedMsg) (Model, tea.Cmd) {
	m.selectedIssue = nil
	if msg.err != nil {
		m.toaster = m.toaster.Show("Error: "+msg.err.Error(), toaster.StyleError)
		return m, toaster.ScheduleDismiss(3 * time.Second)
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

	m.toaster = m.toaster.Show("Status updated", toaster.StyleSuccess)
	return m, toaster.ScheduleDismiss(2 * time.Second)
}

// yankIssueID copies the selected issue ID to clipboard.
func (m Model) yankIssueID() (Model, tea.Cmd) {
	if m.selectedIdx < 0 || m.selectedIdx >= len(m.results) {
		m.toaster = m.toaster.Show("No issue selected", toaster.StyleError)
		return m, toaster.ScheduleDismiss(2 * time.Second)
	}

	issue := m.results[m.selectedIdx]
	if err := copyToClipboard(issue.ID); err != nil {
		m.toaster = m.toaster.Show("Clipboard error: "+err.Error(), toaster.StyleError)
		return m, toaster.ScheduleDismiss(3 * time.Second)
	}

	m.toaster = m.toaster.Show("Copied: "+issue.ID, toaster.StyleSuccess)
	return m, toaster.ScheduleDismiss(2 * time.Second)
}

// copyToClipboard copies text to the system clipboard.
func copyToClipboard(text string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		cmd = exec.Command("xclip", "-selection", "clipboard")
	default:
		cmd = exec.Command("xclip", "-selection", "clipboard")
	}

	pipe, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	if _, err := pipe.Write([]byte(text)); err != nil {
		return err
	}

	if err := pipe.Close(); err != nil {
		return err
	}

	return cmd.Wait()
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

	m.modal, m.deleteIsCascade = m.createDeleteModal(issue)
	m.modal.SetSize(m.width, m.height)
	m.selectedIssue = issue
	m.view = ViewDeleteConfirm
	return m, m.modal.Init()
}

// createDeleteModal creates a delete confirmation modal for the given issue.
func (m Model) createDeleteModal(issue *beads.Issue) (modal.Model, bool) {
	// Check if this is an epic with child issues
	hasChildren := issue.Type == beads.TypeEpic && len(issue.Blocks) > 0

	if hasChildren {
		// Build list of child issues for the modal message
		childIssues := m.getIssuesByIds(issue.Blocks)
		var childList strings.Builder
		issueIdStyle := lipgloss.NewStyle().Foreground(styles.TextSecondaryColor)
		for _, childID := range issue.Blocks {
			if child, ok := childIssues[childID]; ok {
				typeText := board.GetTypeIndicator(child.Type)
				typeStyle := board.GetTypeStyle(child.Type)
				priorityText := fmt.Sprintf("[P%d]", child.Priority)
				priorityStyle := board.GetPriorityStyle(child.Priority)
				idText := fmt.Sprintf("[%s]", childID)

				line := fmt.Sprintf("  %s%s%s %s\n",
					typeStyle.Render(typeText),
					priorityStyle.Render(priorityText),
					issueIdStyle.Render(idText),
					child.TitleText)
				childList.WriteString(line)
			} else {
				childList.WriteString(fmt.Sprintf("  - %s\n", childID))
			}
		}

		message := fmt.Sprintf("Delete epic \"%s: %s\"?\n\nThis will also delete %d child issue(s):\n%s\nThis action cannot be undone.",
			issue.ID, issue.TitleText, len(issue.Blocks), childList.String())

		return modal.New(modal.Config{
			Title:          "Delete Epic",
			Message:        message,
			ConfirmVariant: modal.ButtonDanger,
			MinWidth:       60,
		}), true
	}

	// Regular issue deletion
	message := fmt.Sprintf("Delete \"%s: %s\"?\n\nThis action cannot be undone.", issue.ID, issue.TitleText)
	return modal.New(modal.Config{
		Title:          "Delete Issue",
		Message:        message,
		ConfirmVariant: modal.ButtonDanger,
	}), false
}

// getIssuesByIds fetches issues by their IDs using the client.
func (m Model) getIssuesByIds(ids []string) map[string]*beads.Issue {
	result := make(map[string]*beads.Issue)
	if len(ids) == 0 || m.services.Client == nil {
		return result
	}

	issues, err := m.services.Client.ListIssuesByIds(ids)
	if err != nil {
		return result
	}

	for i := range issues {
		result[issues[i].ID] = &issues[i]
	}
	return result
}

// handleModalSubmit processes modal confirmation.
func (m Model) handleModalSubmit(msg modal.SubmitMsg) (Model, tea.Cmd) {
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
		m.toaster = m.toaster.Show("Error: "+msg.err.Error(), toaster.StyleError)
		m.view = ViewSearch
		m.selectedIssue = nil
		return m, toaster.ScheduleDismiss(3 * time.Second)
	}

	// Return to search, refresh results to remove deleted issue
	m.view = ViewSearch
	m.selectedIssue = nil
	m.toaster = m.toaster.Show("Issue deleted", toaster.StyleSuccess)
	return m, tea.Batch(
		m.executeSearch(),
		toaster.ScheduleDismiss(2*time.Second),
	)
}

// handleLabelsChanged processes label change results.
func (m Model) handleLabelsChanged(msg labelsChangedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.toaster = m.toaster.Show("Error: "+msg.err.Error(), toaster.StyleError)
		return m, toaster.ScheduleDismiss(3 * time.Second)
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

	m.toaster = m.toaster.Show("Labels updated", toaster.StyleSuccess)
	return m, toaster.ScheduleDismiss(2 * time.Second)
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
