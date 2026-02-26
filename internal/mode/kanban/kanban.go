// Package kanban implements the kanban board mode controller.
package kanban

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	beads "github.com/zjrosen/perles/internal/beads/domain"
	"github.com/zjrosen/perles/internal/config"
	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/mode"
	"github.com/zjrosen/perles/internal/mode/shared"
	"github.com/zjrosen/perles/internal/ui/board"
	"github.com/zjrosen/perles/internal/ui/coleditor"
	"github.com/zjrosen/perles/internal/ui/details"
	"github.com/zjrosen/perles/internal/ui/modals/help"
	"github.com/zjrosen/perles/internal/ui/modals/issueeditor"
	"github.com/zjrosen/perles/internal/ui/shared/colorpicker"
	"github.com/zjrosen/perles/internal/ui/shared/editor"
	"github.com/zjrosen/perles/internal/ui/shared/modal"
	"github.com/zjrosen/perles/internal/ui/shared/picker"
	"github.com/zjrosen/perles/internal/ui/shared/toaster"
	"github.com/zjrosen/perles/internal/ui/styles"
)

// ViewMode determines which view is active within the kanban mode.
type ViewMode int

const (
	ViewBoard ViewMode = iota
	ViewHelp
	ViewColumnEditor
	ViewNewViewModal
	ViewDeleteViewModal
	ViewViewMenu
	ViewDeleteColumnModal
	ViewRenameViewModal
	ViewEditIssue   // Unified issue editor modal
	ViewDeleteIssue // Delete issue confirmation modal
)

// cursorState tracks the current selection for restoration after refresh.
type cursorState struct {
	column  board.ColumnIndex
	issueID string
}

// Model is the kanban mode state.
type Model struct {
	services mode.Services

	board       board.Model
	help        help.Model
	picker      picker.Model
	colEditor   coleditor.Model
	modal       modal.Model
	issueEditor issueeditor.Model // Unified issue editor modal
	view        ViewMode
	width       int
	height      int
	loading     bool
	err         error
	errContext  string // Context for the error (e.g., "updating status")

	// Delete operation state
	pendingDeleteColumn int          // Index of column to delete, -1 if none
	deleteIssueIDs      []string     // IDs to delete (includes descendants for epics)
	selectedIssue       *beads.Issue // Issue being deleted

	// Edit operation state
	editingIssue *beads.Issue // Issue being edited (for title/description comparison on save)

	// Pending cursor restoration after refresh
	pendingCursor    *cursorState
	pendingLoadCount int // Remaining expected load messages for current reload cycle

	// Refresh state tracking
	autoRefreshed   bool // Set when refresh triggered by file watcher
	manualRefreshed bool // Set when refresh triggered by 'r' key

	// UI visibility toggles
	showStatusBar bool

	// User-defined actions for kanban mode (key -> action config)
	actions map[string]config.ActionConfig
}

// New creates a new kanban mode controller.
func New(services mode.Services) Model {
	// Create board from views (GetViews returns defaults if none configured)
	clock := services.Clock
	boardModel := board.NewFromViews(services.Config.GetViews(), services.Executor, clock).
		SetShowCounts(services.Config.UI.ShowCounts)

	// Get user-defined actions from config (may be nil)
	var actions map[string]config.ActionConfig
	if services.Config.UI.Actions.IssueAction != nil {
		actions = services.Config.UI.Actions.IssueAction
	}

	// Convert user actions to help.UserAction for display in help overlay
	var userActions []help.UserAction
	for _, action := range actions {
		userActions = append(userActions, help.UserAction{
			Key:         action.Key,
			Description: action.Description,
		})
	}

	return Model{
		services:            services,
		view:                ViewBoard,
		board:               boardModel,
		help:                help.New().WithFlags(services.Flags).WithUserActions(userActions),
		loading:             true,
		showStatusBar:       services.Config.UI.ShowStatusBar,
		pendingDeleteColumn: -1,
		actions:             actions,
	}
}

// Init returns initial commands for the mode.
func (m Model) Init() tea.Cmd {
	// Trigger initial column load via BQL
	return m.board.LoadAllColumns()
}

// Refresh triggers a data reload.
func (m Model) Refresh() tea.Cmd {
	// Note: m.loading is set but doesn't persist (receiver is value type)
	// The actual loading state is managed through the board's LoadAllColumns
	return m.board.InvalidateViews().LoadAllColumns()
}

// RefreshFromConfig rebuilds the board from the current config.
// Use this when columns have been added/removed externally.
func (m Model) RefreshFromConfig() (Model, tea.Cmd) {
	// Preserve current selection so returning from another mode (e.g. search)
	// keeps the cursor on the same issue after the board reloads.
	m.pendingCursor = m.saveCursor()
	m.rebuildBoard()
	m.startReloadCycle()
	return m, m.loadBoardCmd()
}

// SetSize handles terminal resize.
func (m Model) SetSize(width, height int) Model {
	m.width = width
	m.height = height
	m.board = m.board.SetSize(width, m.boardHeight())
	m.help = m.help.SetSize(width, height)
	// Update column editor if we're viewing it
	if m.view == ViewColumnEditor {
		m.colEditor = m.colEditor.SetSize(width, height)
	}
	// Update modal if we're viewing it
	if m.view == ViewNewViewModal || m.view == ViewDeleteViewModal || m.view == ViewDeleteColumnModal || m.view == ViewRenameViewModal {
		m.modal.SetSize(width, height)
	}
	// Update picker if we're viewing a menu
	if m.view == ViewViewMenu {
		m.picker = m.picker.SetSize(width, height)
	}
	return m
}

// SetBoardFocused sets whether the board has focus for column highlighting.
func (m Model) SetBoardFocused(focused bool) Model {
	m.board = m.board.SetBoardFocused(focused)
	return m
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	// Handle key messages
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		return m.handleKey(keyMsg)
	}

	switch msg := msg.(type) {
	case board.ColumnLoadedMsg:
		return m.handleColumnLoaded(msg)

	case board.TreeColumnLoadedMsg:
		// Delegate tree column load messages to board
		m.board, _ = m.board.Update(msg)
		reloadDone := m.completeLoadMsg()
		if m.pendingCursor != nil && reloadDone {
			// Tree columns don't support SelectByID restoration; clear pending at end of cycle.
			m.pendingCursor = nil
		}
		m.autoRefreshed = false
		if m.manualRefreshed && reloadDone {
			m.manualRefreshed = false
			return m, func() tea.Msg { return mode.ShowToastMsg{Message: "refreshed issues", Style: toaster.StyleSuccess} }
		}
		return m, nil

	case tea.MouseMsg:
		// Route mouse events based on current view
		switch m.view {
		case ViewBoard:
			var cmd tea.Cmd
			m.board, cmd = m.board.Update(msg)
			return m, cmd
		case ViewEditIssue:
			var cmd tea.Cmd
			m.issueEditor, cmd = m.issueEditor.Update(msg)
			return m, cmd
		}
		return m, nil

	case board.IssueClickedMsg:
		// Convert click to SwitchToSearchMsg with SubModeTree (same as Enter key)
		return m, func() tea.Msg {
			return SwitchToSearchMsg{
				SubMode: mode.SubModeTree,
				IssueID: msg.IssueID,
			}
		}

	case issueSavedMsg:
		return m.handleIssueSaved(msg)

	case pickerCancelledMsg:
		// Return to board view (used by view menu picker)
		m.view = ViewBoard
		return m, nil

	case OpenEditMenuMsg:
		issue := msg.Issue
		m.editingIssue = &issue // Store for title/description comparison on save
		m.issueEditor = issueeditor.NewWithVimMode(msg.Issue, m.services.Config.UI.VimMode).
			SetSize(m.width, m.height)
		m.view = ViewEditIssue
		return m, m.issueEditor.Init()

	case issueeditor.SaveMsg:
		m.view = ViewBoard
		m.loading = true
		opts := msg.BuildUpdateOptions(m.editingIssue)
		m.editingIssue = nil
		return m, m.saveIssueCmd(msg.IssueID, opts)

	case issueeditor.CancelMsg:
		m.view = ViewBoard
		m.editingIssue = nil // Clear on cancel too
		return m, nil

	case details.DeleteIssueMsg:
		return m.openDeleteConfirm(msg)

	case issueDeletedMsg:
		return m.handleIssueDeleted(msg)

	case shared.ActionExecutedMsg:
		return m.handleActionExecuted(msg)

	case errMsg:
		return m.handleErrMsg(msg)

	case clearErrorMsg:
		m.err = nil
		m.errContext = ""
		return m, nil

	case clearRefreshIndicatorMsg:
		m.autoRefreshed = false
		m.manualRefreshed = false
		return m, nil

	case colorpicker.SelectMsg, colorpicker.CancelMsg:
		// Route colorpicker messages to column editor when it's active
		if m.view == ViewColumnEditor {
			var cmd tea.Cmd
			m.colEditor, cmd = m.colEditor.Update(msg)
			return m, cmd
		}
		return m, nil

	case coleditor.SaveMsg:
		return m.handleColEditorSave(msg)

	case coleditor.DeleteMsg:
		return m.handleColEditorDelete(msg)

	case coleditor.AddMsg:
		return m.handleColEditorAdd(msg)

	case coleditor.CancelMsg:
		m.view = ViewBoard
		return m, nil

	case viewMenuCreateMsg:
		// Open new view modal
		m.modal = modal.New(modal.Config{
			Title:          "Create New View",
			ConfirmVariant: modal.ButtonPrimary,
			Inputs: []modal.InputConfig{
				{Key: "name", Label: "View Name", Placeholder: "Enter view name...", MaxLength: 50},
			},
		})
		m.modal.SetSize(m.width, m.height)
		m.view = ViewNewViewModal
		return m, m.modal.Init()

	case viewMenuDeleteMsg:
		// Prevent deletion of last view
		if len(m.services.Config.Views) <= 1 {
			m.view = ViewBoard
			return m, func() tea.Msg {
				return mode.ShowToastMsg{Message: "Cannot delete the only view", Style: toaster.StyleError}
			}
		}
		// Open delete view confirmation
		viewName := m.board.CurrentViewName()
		m.modal = modal.New(modal.Config{
			Title:          "Delete View",
			Message:        fmt.Sprintf("Delete view '%s'? This cannot be undone.", viewName),
			ConfirmVariant: modal.ButtonDanger,
		})
		m.modal.SetSize(m.width, m.height)
		m.view = ViewDeleteViewModal
		return m, m.modal.Init()

	case viewMenuRenameMsg:
		// Open rename modal with current view name pre-filled
		currentViewName := m.board.CurrentViewName()
		m.modal = modal.New(modal.Config{
			Title:          "Rename View",
			ConfirmVariant: modal.ButtonPrimary,
			Inputs: []modal.InputConfig{
				{Key: "name", Label: "View Name", Value: currentViewName, MaxLength: 50},
			},
		})
		m.modal.SetSize(m.width, m.height)
		m.view = ViewRenameViewModal
		return m, m.modal.Init()

	case modal.SubmitMsg:
		return m.handleModalSubmit(msg)

	case modal.CancelMsg:
		return m.handleModalCancel()

	case editor.ExecMsg:
		// Forward to issueeditor modal if open - this allows Ctrl+G external editor
		// to work from the modal's description field. Without this check, the message
		// would be intercepted here and lost.
		if m.view == ViewEditIssue {
			var cmd tea.Cmd
			m.issueEditor, cmd = m.issueEditor.Update(msg)
			return m, cmd
		}
		return m, nil

	case editor.FinishedMsg:
		// Forward to issueeditor modal if open - ensures editor results return
		// to the modal's description field when editing via Ctrl+G.
		if m.view == ViewEditIssue {
			var cmd tea.Cmd
			m.issueEditor, cmd = m.issueEditor.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	return m, nil
}

// View renders the kanban mode.
func (m Model) View() string {
	switch m.view {
	case ViewHelp:
		// Render help overlay on top of board
		bg := m.renderBoardWithStatusBar()
		return m.help.Overlay(bg)
	case ViewColumnEditor:
		// Full-screen column editor
		return m.colEditor.View()
	case ViewNewViewModal, ViewDeleteViewModal, ViewRenameViewModal:
		// Render modal overlay on top of board
		bg := m.renderBoardWithStatusBar()
		return m.modal.Overlay(bg)
	case ViewEditIssue:
		// Render issue editor overlay on top of board
		bg := m.renderBoardWithStatusBar()
		return m.issueEditor.Overlay(bg)
	case ViewViewMenu:
		// Render view menu overlay on top of board
		bg := m.renderBoardWithStatusBar()
		return m.picker.Overlay(bg)
	case ViewDeleteColumnModal, ViewDeleteIssue:
		// Render delete modal overlay on top of board
		bg := m.renderBoardWithStatusBar()
		return m.modal.Overlay(bg)
	default:
		return m.renderBoardWithStatusBar()
	}
}

// renderBoardWithStatusBar renders the board with optional status bar,
// ensuring consistent width to prevent layout shifts.
func (m Model) renderBoardWithStatusBar() string {
	// Render board and ensure it fills the full width
	// This prevents layout shifts when status bar is toggled,
	// since column widths may not sum exactly to m.width due to integer division
	boardStyle := lipgloss.NewStyle().Width(m.width)
	view := boardStyle.Render(m.board.View())

	if m.showStatusBar {
		view += "\n"
		if m.err != nil {
			view += m.renderErrorBar()
		} else {
			view += m.renderStatusBar()
		}
	}
	return view
}

// Close releases resources held by the kanban mode.
func (m *Model) Close() error {
	// No resources to clean up - app owns the watcher now
	return nil
}

// saveCursor captures the current selection state.
func (m Model) saveCursor() *cursorState {
	selected := m.board.SelectedIssue()
	if selected == nil {
		return nil
	}
	return &cursorState{
		column:  m.board.FocusedColumn(),
		issueID: selected.ID,
	}
}

// restoreCursor attempts to restore selection to the saved issue.
// Returns true when the issue was found and selected.
func (m Model) restoreCursor(state *cursorState) (Model, bool) {
	if state == nil {
		return m, false
	}

	// Keep focus anchored to the previous column while loads are still arriving.
	m.board = m.board.SetFocus(state.column)

	// Try to find the issue by ID (it may have moved columns).
	newBoard, found := m.board.SelectByID(state.issueID)
	if found {
		m.board = newBoard
		return m, true
	}
	return m, false
}

// boardHeight returns the available height for the board, accounting for status bar.
func (m Model) boardHeight() int {
	if m.showStatusBar {
		return m.height - 1 // Reserve 1 line for status bar
	}
	return m.height
}

// rebuildBoard recreates the board from the current config.
func (m *Model) rebuildBoard() {
	currentView := m.board.CurrentViewIndex()

	clock := m.services.Clock
	m.board = board.NewFromViews(m.services.Config.GetViews(), m.services.Executor, clock).
		SetShowCounts(m.services.Config.UI.ShowCounts).
		SetSize(m.width, m.boardHeight())

	// Restore view index if valid
	if currentView > 0 && currentView < m.board.ViewCount() {
		m.board, _ = m.board.SwitchToView(currentView)
	}
}

// rebuildBoardWithFocus recreates the board and sets focus to a specific column.
func (m *Model) rebuildBoardWithFocus(focusColumn int) {
	m.rebuildBoard()
	m.board = m.board.SetFocus(focusColumn)
}

// loadBoardCmd returns the appropriate command to load the board.
func (m Model) loadBoardCmd() tea.Cmd {
	if m.board.ViewCount() > 0 {
		return m.board.LoadCurrentViewCmd()
	}
	return m.board.LoadAllColumns()
}

// countPendingLoadMsgs returns the number of load messages expected for the current view.
func (m Model) countPendingLoadMsgs() int {
	count := 0
	viewIndex := m.board.CurrentViewIndex()
	for i := 0; i < m.board.ColCount(); i++ {
		col := m.board.BoardColumn(i)
		if col == nil {
			continue
		}
		if cmd := col.LoadCmd(viewIndex, i); cmd != nil {
			count++
		}
	}
	return count
}

// startReloadCycle initializes load-tracking state for a board reload operation.
func (m *Model) startReloadCycle() {
	m.pendingLoadCount = m.countPendingLoadMsgs()
	m.loading = m.pendingLoadCount > 0
}

// completeLoadMsg advances reload tracking by one message and reports whether reload is complete.
func (m *Model) completeLoadMsg() bool {
	if m.pendingLoadCount > 0 {
		m.pendingLoadCount--
	}
	m.loading = m.pendingLoadCount > 0
	return m.pendingLoadCount == 0
}

// currentViewIndex returns the current view index, or 0 if no views.
func (m Model) currentViewIndex() int {
	return m.board.CurrentViewIndex()
}

// currentViewColumns returns the columns for the current view.
func (m Model) currentViewColumns() []config.ColumnConfig {
	return m.services.Config.GetColumnsForView(m.currentViewIndex())
}

// configPath returns the config path or default.
func (m Model) configPath() string {
	if m.services.ConfigPath == "" {
		return ".perles.yaml"
	}
	return m.services.ConfigPath
}

// ShowStatusBar returns whether the status bar is currently visible.
func (m Model) ShowStatusBar() bool {
	return m.showStatusBar
}

func (m Model) renderStatusBar() string {
	// Build left section with view indicator (if multiple views)
	var content string
	if m.board.ViewCount() > 1 {
		viewName := m.board.CurrentViewName()
		viewNum := m.board.CurrentViewIndex() + 1
		viewTotal := m.board.ViewCount()
		content = fmt.Sprintf("[%s] (%d/%d)", viewName, viewNum, viewTotal)
	}

	return styles.StatusBarStyle.Width(m.width).Render(content)
}

func (m Model) renderErrorBar() string {
	msg := "Error"
	if m.errContext != "" {
		msg += " " + m.errContext
	}
	msg += ": " + m.err.Error() + "  [Press any key to dismiss]"
	return styles.ErrorStyle.Width(m.width).Render(msg)
}

// deleteColumn handles the deletion of a column after modal confirmation.
func (m Model) deleteColumn() (Model, tea.Cmd) {
	colIndex := m.pendingDeleteColumn
	m.pendingDeleteColumn = -1

	if colIndex < 0 {
		m.view = ViewBoard
		return m, nil
	}

	viewIndex := m.currentViewIndex()
	columns := m.currentViewColumns()

	if colIndex >= len(columns) {
		m.view = ViewBoard
		return m, nil
	}

	err := config.DeleteColumnInView(m.configPath(), viewIndex, colIndex, columns, m.services.Config.Views)
	if err != nil {
		log.ErrorErr(log.CatConfig, "Failed to delete column", err,
			"viewIndex", viewIndex,
			"columnIndex", colIndex)
		m.err = err
		m.errContext = "deleting column"
		m.view = ViewBoard
		return m, scheduleErrorClear()
	}

	// Update in-memory config (remove the column)
	newColumns := append(columns[:colIndex], columns[colIndex+1:]...)
	m.services.Config.SetColumnsForView(viewIndex, newColumns)

	// Rebuild board with new config
	m.rebuildBoard()

	m.view = ViewBoard
	m.loading = true
	cmds := []tea.Cmd{
		func() tea.Msg { return mode.ShowToastMsg{Message: "Column deleted", Style: toaster.StyleSuccess} },
	}
	if loadCmd := m.loadBoardCmd(); loadCmd != nil {
		cmds = append(cmds, loadCmd)
	}
	return m, tea.Batch(cmds...)
}

// createNewView handles the creation of a new view after modal submission.
func (m Model) createNewView(viewName string) (Model, tea.Cmd) {
	newView := config.ViewConfig{
		Name:    viewName,
		Columns: []config.ColumnConfig{},
	}

	err := config.AddView(m.configPath(), newView, m.services.Config.Views)
	if err != nil {
		log.ErrorErr(log.CatConfig, "Failed to create view", err,
			"viewName", viewName)
		m.err = err
		m.errContext = "creating view"
		m.view = ViewBoard
		return m, scheduleErrorClear()
	}

	// Update in-memory config
	m.services.Config.Views = append(m.services.Config.Views, newView)

	// Rebuild board and switch to new view
	m.rebuildBoard()
	newViewIndex := len(m.services.Config.Views) - 1
	m.board, _ = m.board.SwitchToView(newViewIndex)

	m.view = ViewBoard
	m.loading = true
	cmds := []tea.Cmd{
		func() tea.Msg {
			return mode.ShowToastMsg{Message: "Created view: " + viewName, Style: toaster.StyleSuccess}
		},
	}
	if loadCmd := m.board.LoadCurrentViewCmd(); loadCmd != nil {
		cmds = append(cmds, loadCmd)
	}
	return m, tea.Batch(cmds...)
}

// deleteCurrentView handles the deletion of the current view after modal confirmation.
func (m Model) deleteCurrentView() (Model, tea.Cmd) {
	viewIndex := m.board.CurrentViewIndex()
	viewName := m.board.CurrentViewName()

	err := config.DeleteView(m.configPath(), viewIndex, m.services.Config.Views)
	if err != nil {
		log.ErrorErr(log.CatConfig, "Failed to delete view", err,
			"viewIndex", viewIndex,
			"viewName", viewName)
		m.err = err
		m.errContext = "deleting view"
		m.view = ViewBoard
		return m, scheduleErrorClear()
	}

	// Update in-memory config
	m.services.Config.Views = append(m.services.Config.Views[:viewIndex], m.services.Config.Views[viewIndex+1:]...)

	// Determine which view to switch to
	newViewIndex := max(viewIndex-1, 0)

	// Rebuild board with updated views
	m.view = ViewBoard
	m.rebuildBoard()
	m.board, _ = m.board.SwitchToView(newViewIndex)

	m.loading = true
	cmds := []tea.Cmd{
		func() tea.Msg {
			return mode.ShowToastMsg{Message: "Deleted view: " + viewName, Style: toaster.StyleSuccess}
		},
	}
	if loadCmd := m.board.LoadCurrentViewCmd(); loadCmd != nil {
		cmds = append(cmds, loadCmd)
	}
	return m, tea.Batch(cmds...)
}

// renameCurrentView handles renaming the current view after modal submission.
func (m Model) renameCurrentView(newName string) (Model, tea.Cmd) {
	viewIndex := m.board.CurrentViewIndex()

	err := config.RenameView(m.configPath(), viewIndex, newName, m.services.Config.Views)
	if err != nil {
		log.ErrorErr(log.CatConfig, "Failed to rename view", err,
			"viewIndex", viewIndex,
			"newName", newName)
		m.err = err
		m.errContext = "renaming view"
		m.view = ViewBoard
		return m, scheduleErrorClear()
	}

	m.services.Config.Views[viewIndex].Name = newName
	m.board = m.board.SetCurrentViewName(newName)

	m.view = ViewBoard
	return m, func() tea.Msg {
		return mode.ShowToastMsg{Message: "Renamed view to: " + newName, Style: toaster.StyleSuccess}
	}
}

// Message types

// SwitchToSearchMsg requests switching to search mode.
type SwitchToSearchMsg struct {
	Query   string       // For list sub-mode (existing)
	SubMode mode.SubMode // Which sub-mode to enter
	IssueID string       // For tree sub-mode
}

// SwitchToDashboardMsg requests switching to the multi-workflow dashboard mode.
type SwitchToDashboardMsg struct{}

// RequestRefreshMsg requests the app flush caches and reload the kanban board.
// Emitted by the "r" key handler so the app can flush BQL/dep-graph caches
// (which the kanban model does not own) before re-querying.
type RequestRefreshMsg struct{}

// PostDeleteRefreshMsg requests the app flush caches and reload after an issue
// deletion. The kanban model does not own the BQL/dep-graph caches, so the app
// must flush them before re-querying to avoid serving stale cached results that
// still include the deleted issue.
type PostDeleteRefreshMsg struct{}

// OpenEditMenuMsg requests opening the issue editor modal.
type OpenEditMenuMsg struct {
	Issue beads.Issue
}

type errMsg struct {
	err     error
	context string
}

type clearErrorMsg struct{}

type clearRefreshIndicatorMsg struct{}

// issueSavedMsg signals completion of a consolidated issue save.
type issueSavedMsg struct {
	issueID string
	opts    beads.UpdateIssueOptions
	err     error
}

// pickerCancelledMsg is produced when any picker is cancelled.
type pickerCancelledMsg struct{}

// viewMenuCreateMsg is produced when "create view" is selected in view menu picker.
type viewMenuCreateMsg struct{}

// viewMenuDeleteMsg is produced when "delete view" is selected in view menu picker.
type viewMenuDeleteMsg struct{}

// viewMenuRenameMsg is produced when "rename view" is selected in view menu picker.
type viewMenuRenameMsg struct{}

// Async commands

func (m Model) saveIssueCmd(issueID string, opts beads.UpdateIssueOptions) tea.Cmd {
	return func() tea.Msg {
		err := m.services.BeadsExecutor.UpdateIssue(issueID, opts)
		return issueSavedMsg{issueID: issueID, opts: opts, err: err}
	}
}

func scheduleErrorClear() tea.Cmd {
	return tea.Tick(3*time.Second, func(_ time.Time) tea.Msg {
		return clearErrorMsg{}
	})
}
