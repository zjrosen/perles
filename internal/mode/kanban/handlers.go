package kanban

import (
	"fmt"
	"time"

	"perles/internal/beads"
	"perles/internal/config"
	"perles/internal/ui/coleditor"
	"perles/internal/ui/details"
	"perles/internal/ui/modal"
	"perles/internal/ui/toaster"
	"perles/internal/ui/viewmenu"

	tea "github.com/charmbracelet/bubbletea"
)

// handleKey routes key messages to the appropriate handler based on view mode.
func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch m.view {
	case ViewBoard:
		return m.handleBoardKey(msg)
	case ViewDetails:
		return m.handleDetailsKey(msg)
	case ViewHelp:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc", "?":
			m.view = ViewBoard
			return m, nil
		}
		return m, nil
	case ViewDetailsPriorityPicker:
		return m.handleDetailsPriorityPickerKey(msg)
	case ViewDetailsStatusPicker:
		return m.handleDetailsStatusPickerKey(msg)
	case ViewColumnEditor:
		return m.handleColumnEditorKey(msg)
	case ViewNewViewModal:
		return m.handleNewViewModalKey(msg)
	case ViewDeleteViewModal:
		return m.handleDeleteViewModalKey(msg)
	case ViewDeleteConfirm:
		return m.handleDeleteConfirmKey(msg)
	case ViewLabelEditor:
		return m.handleLabelEditorKey(msg)
	case ViewViewMenu:
		return m.handleViewMenuKey(msg)
	case ViewDeleteColumnModal:
		return m.handleDeleteColumnModalKey(msg)
	case ViewRenameViewModal:
		return m.handleRenameViewModalKey(msg)
	}
	return m, nil
}

func (m Model) handleBoardKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	// Dismiss error on any key press (except quit)
	// Don't return early - let the key continue to be processed
	if m.err != nil && msg.String() != "q" && msg.String() != "ctrl+c" {
		m.err = nil
		m.errContext = ""
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "?":
		m.view = ViewHelp
		return m, nil

	case "r":
		// Save cursor before refresh to restore position after
		m.pendingCursor = m.saveCursor()
		m.loading = true
		m.manualRefreshed = true
		m.autoRefreshed = false
		// Invalidate other views so they reload when switched to
		m.board = m.board.InvalidateViews()
		cmds := []tea.Cmd{m.spinner.Tick}
		if loadCmd := m.board.LoadAllColumns(); loadCmd != nil {
			cmds = append(cmds, loadCmd)
		}
		return m, tea.Batch(cmds...)

	case "y":
		// Yank (copy) selected issue ID to clipboard
		if issue := m.board.SelectedIssue(); issue != nil {
			if err := copyToClipboard(issue.ID); err != nil {
				m.err = err
				m.errContext = "copying to clipboard"
				return m, scheduleErrorClear()
			}
			m.toaster = m.toaster.Show("Copied: "+issue.ID, toaster.StyleSuccess)
			return m, toaster.ScheduleDismiss(2 * time.Second)
		}
		return m, nil

	case "w":
		// Toggle status bar visibility
		m.showStatusBar = !m.showStatusBar
		// Recalculate board height since available space changed
		m.board = m.board.SetSize(m.width, m.boardHeight())
		return m, nil

	case "e":
		// Open column editor for focused column
		focusedCol := m.board.FocusedColumn()
		columns := m.currentViewColumns()
		if focusedCol >= 0 && focusedCol < len(columns) {
			// Pass executor for BQL preview queries
			m.colEditor = coleditor.New(focusedCol, columns, m.services.Executor).
				SetSize(m.width, m.height)
			m.view = ViewColumnEditor
		}
		return m, nil

	case "a":
		// Open column editor in New mode (insert after focused column)
		focusedCol := m.board.FocusedColumn()
		columns := m.currentViewColumns()

		// Handle empty view - insert at position -1 (will become position 0)
		insertAfter := focusedCol
		if len(columns) == 0 {
			insertAfter = -1
		}

		// Create editor in New mode
		m.colEditor = coleditor.NewForCreate(insertAfter, columns, m.services.Executor).
			SetSize(m.width, m.height)
		m.view = ViewColumnEditor
		return m, nil

	case "ctrl+h": // Move column left
		focusedCol := m.board.FocusedColumn()
		if focusedCol <= 0 {
			return m, nil // Already leftmost
		}
		viewIndex := m.currentViewIndex()
		columns := m.currentViewColumns()

		if err := config.SwapColumnsInView(m.configPath(), viewIndex, focusedCol, focusedCol-1, columns, m.services.Config.Views); err != nil {
			m.err = err
			m.errContext = "moving column"
			return m, scheduleErrorClear()
		}

		// Update in-memory config
		columns[focusedCol], columns[focusedCol-1] = columns[focusedCol-1], columns[focusedCol]
		m.services.Config.SetColumnsForView(viewIndex, columns)

		// Rebuild board and set focus to new position
		m.rebuildBoardWithFocus(focusedCol - 1)

		m.loading = true
		cmds := []tea.Cmd{m.spinner.Tick}
		if loadCmd := m.loadBoardCmd(); loadCmd != nil {
			cmds = append(cmds, loadCmd)
		}
		return m, tea.Batch(cmds...)

	case "ctrl+l": // Move column right
		focusedCol := m.board.FocusedColumn()
		viewIndex := m.currentViewIndex()
		columns := m.currentViewColumns()
		if focusedCol >= len(columns)-1 {
			return m, nil // Already rightmost
		}

		if err := config.SwapColumnsInView(m.configPath(), viewIndex, focusedCol, focusedCol+1, columns, m.services.Config.Views); err != nil {
			m.err = err
			m.errContext = "moving column"
			return m, scheduleErrorClear()
		}

		// Update in-memory config
		columns[focusedCol], columns[focusedCol+1] = columns[focusedCol+1], columns[focusedCol]
		m.services.Config.SetColumnsForView(viewIndex, columns)

		// Rebuild board and set focus to new position
		m.rebuildBoardWithFocus(focusedCol + 1)

		m.loading = true
		cmds := []tea.Cmd{m.spinner.Tick}
		if loadCmd := m.loadBoardCmd(); loadCmd != nil {
			cmds = append(cmds, loadCmd)
		}
		return m, tea.Batch(cmds...)

	case "ctrl+j", "ctrl+n": // Next view
		if m.board.ViewCount() > 1 {
			var cmd tea.Cmd
			m.board, cmd = m.board.CycleViewNext()

			// Show toast only if status bar is hidden
			var toastCmd tea.Cmd
			if !m.showStatusBar {
				viewName := m.board.CurrentViewName()
				viewNum := m.board.CurrentViewIndex() + 1
				viewTotal := m.board.ViewCount()
				m.toaster = m.toaster.Show(
					fmt.Sprintf("View: %s (%d/%d)", viewName, viewNum, viewTotal),
					toaster.StyleInfo,
				)
				toastCmd = toaster.ScheduleDismiss(2 * time.Second)
			}

			if cmd != nil {
				m.loading = true
				cmds := []tea.Cmd{cmd, m.spinner.Tick}
				if toastCmd != nil {
					cmds = append(cmds, toastCmd)
				}
				return m, tea.Batch(cmds...)
			}
			return m, toastCmd
		}
		return m, nil

	case "ctrl+k", "ctrl+p": // Previous view
		if m.board.ViewCount() > 1 {
			var cmd tea.Cmd
			m.board, cmd = m.board.CycleViewPrev()

			// Show toast only if status bar is hidden
			var toastCmd tea.Cmd
			if !m.showStatusBar {
				viewName := m.board.CurrentViewName()
				viewNum := m.board.CurrentViewIndex() + 1
				viewTotal := m.board.ViewCount()
				m.toaster = m.toaster.Show(
					fmt.Sprintf("View: %s (%d/%d)", viewName, viewNum, viewTotal),
					toaster.StyleInfo,
				)
				toastCmd = toaster.ScheduleDismiss(2 * time.Second)
			}

			if cmd != nil {
				m.loading = true
				cmds := []tea.Cmd{cmd, m.spinner.Tick}
				if toastCmd != nil {
					cmds = append(cmds, toastCmd)
				}
				return m, tea.Batch(cmds...)
			}
			return m, toastCmd
		}
		return m, nil

	case "ctrl+v": // View menu
		m.viewMenu = viewmenu.New().SetSize(m.width, m.height)
		m.view = ViewViewMenu
		return m, nil

	case "ctrl+d": // Delete column
		focusedCol := m.board.FocusedColumn()
		columns := m.currentViewColumns()
		if focusedCol < 0 || focusedCol >= len(columns) {
			return m, nil // no column focused, do nothing
		}
		colName := columns[focusedCol].Name
		m.modal = modal.New(modal.Config{
			Title:          "Delete Column",
			Message:        fmt.Sprintf("Delete column '%s'? This cannot be undone.", colName),
			ConfirmVariant: modal.ButtonDanger,
		})
		m.pendingDeleteColumn = focusedCol
		m.modal.SetSize(m.width, m.height)
		m.view = ViewDeleteColumnModal
		return m, m.modal.Init()

	case "enter":
		if issue := m.board.SelectedIssue(); issue != nil {
			m.details = details.New(*issue, m.services.Client).SetSize(m.width, m.height)
			m.view = ViewDetails
		}
		return m, nil

	case "/":
		// Switch to search with current column's BQL query
		focusedCol := m.board.FocusedColumn()
		query := ""
		if focusedCol >= 0 && focusedCol < m.board.ColCount() {
			query = m.board.Column(focusedCol).Query()
		}
		return m, func() tea.Msg { return SwitchToSearchMsg{Query: query} }
	}

	// Delegate navigation to board
	var cmd tea.Cmd
	m.board, cmd = m.board.Update(msg)
	return m, cmd
}

func (m Model) handleDetailsKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		m.view = ViewBoard
		// Save cursor to return to the same issue after refresh
		m.pendingCursor = &cursorState{
			column:  m.board.FocusedColumn(),
			issueID: m.details.IssueID(),
		}
		// Invalidate views and reload to show any changes made in details view
		m.board = m.board.InvalidateViews()
		return m, m.board.LoadAllColumns()
	}

	// Delegate to details view
	var cmd tea.Cmd
	m.details, cmd = m.details.Update(msg)
	return m, cmd
}

func (m Model) handleDetailsPriorityPickerKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		m.view = ViewDetails
		m.selectedIssue = nil
		return m, nil
	case "enter":
		if m.selectedIssue != nil {
			priority := beads.Priority(m.picker.Selected().Value[1] - '0') // Parse "P0"-"P4"
			m.view = ViewDetails
			return m, updatePriorityCmd(m.selectedIssue.ID, priority)
		}
		m.view = ViewDetails
		return m, nil
	}
	var cmd tea.Cmd
	m.picker, cmd = m.picker.Update(msg)
	return m, cmd
}

func (m Model) handleDetailsStatusPickerKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		m.view = ViewDetails
		m.selectedIssue = nil
		return m, nil
	case "enter":
		if m.selectedIssue != nil {
			status := beads.Status(m.picker.Selected().Value)
			m.view = ViewDetails
			return m, updateStatusCmd(m.selectedIssue.ID, status)
		}
		m.view = ViewDetails
		return m, nil
	}
	var cmd tea.Cmd
	m.picker, cmd = m.picker.Update(msg)
	return m, cmd
}

func (m Model) handleColumnEditorKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	}

	// Delegate to column editor
	var cmd tea.Cmd
	m.colEditor, cmd = m.colEditor.Update(msg)
	return m, cmd
}

func (m Model) handleNewViewModalKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	}

	// Delegate to modal
	var cmd tea.Cmd
	m.modal, cmd = m.modal.Update(msg)
	return m, cmd
}

func (m Model) handleDeleteViewModalKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	}

	// Delegate to modal
	var cmd tea.Cmd
	m.modal, cmd = m.modal.Update(msg)
	return m, cmd
}

func (m Model) handleDeleteConfirmKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	}

	// Delegate to modal
	var cmd tea.Cmd
	m.modal, cmd = m.modal.Update(msg)
	return m, cmd
}

func (m Model) handleLabelEditorKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	}

	// Delegate to label editor
	var cmd tea.Cmd
	m.labelEditor, cmd = m.labelEditor.Update(msg)
	return m, cmd
}

func (m Model) handleViewMenuKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	}

	// Delegate to view menu
	var cmd tea.Cmd
	m.viewMenu, cmd = m.viewMenu.Update(msg)
	return m, cmd
}

func (m Model) handleDeleteColumnModalKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	}

	// Delegate to modal
	var cmd tea.Cmd
	m.modal, cmd = m.modal.Update(msg)
	return m, cmd
}

func (m Model) handleRenameViewModalKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	}

	// Delegate to modal
	var cmd tea.Cmd
	m.modal, cmd = m.modal.Update(msg)
	return m, cmd
}

// handleColumnLoaded processes column load completion.
func (m Model) handleColumnLoaded(msg tea.Msg) (Model, tea.Cmd) {
	// Pass message to board for handling
	m.board, _ = m.board.Update(msg)

	// Check if all columns are done loading
	allLoaded := true
	for i := 0; i < m.board.ColCount(); i++ {
		if m.board.Column(i).IsLoading() {
			allLoaded = false
			break
		}
	}

	if allLoaded {
		m.loading = false

		// Restore cursor if we have a pending state
		if m.pendingCursor != nil {
			m = m.restoreCursor(m.pendingCursor)
			m.pendingCursor = nil
		}
		// Auto sync is silent, manual refresh shows toaster
		m.autoRefreshed = false
		if m.manualRefreshed {
			m.toaster = m.toaster.Show("refreshed issues", toaster.StyleSuccess)
			m.manualRefreshed = false
			return m, toaster.ScheduleDismiss(2 * time.Second)
		}
	}
	return m, nil
}

// handleStatusChanged processes status change results.
func (m Model) handleStatusChanged(msg statusChangedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.err = msg.err
		m.errContext = "updating status"
		m.view = ViewBoard
		m.selectedIssue = nil
		return m, scheduleErrorClear()
	}
	// If editing from details view, stay there and update the displayed value
	if m.view == ViewDetails {
		m.details = m.details.UpdateStatus(msg.status)
		return m, nil
	}
	m.view = ViewBoard
	// Save cursor to follow the issue after status change
	m.pendingCursor = &cursorState{
		column:  m.board.FocusedColumn(),
		issueID: msg.issueID,
	}
	m.selectedIssue = nil
	// Invalidate other views so they reload when switched to
	m.board = m.board.InvalidateViews()
	return m, m.board.LoadAllColumns()
}

// handlePriorityChanged processes priority change results.
func (m Model) handlePriorityChanged(msg priorityChangedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.err = msg.err
		m.errContext = "changing priority"
		m.view = ViewDetails
		m.selectedIssue = nil
		return m, scheduleErrorClear()
	}
	// If editing from details view, stay there and update the displayed value
	if m.view == ViewDetails {
		m.details = m.details.UpdatePriority(msg.priority)
		return m, nil
	}
	m.view = ViewBoard
	// Save cursor to stay on the same issue after priority change
	m.pendingCursor = &cursorState{
		column:  m.board.FocusedColumn(),
		issueID: msg.issueID,
	}
	m.selectedIssue = nil
	// Invalidate other views so they reload when switched to
	m.board = m.board.InvalidateViews()
	return m, m.board.LoadAllColumns()
}

// handleIssueDeleted processes issue deletion results.
func (m Model) handleIssueDeleted(msg issueDeletedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.err = msg.err
		m.errContext = "deleting issue"
		m.view = ViewBoard
		m.selectedIssue = nil
		return m, scheduleErrorClear()
	}
	// Return to board, refresh to remove deleted issue
	m.view = ViewBoard
	m.selectedIssue = nil
	m.loading = true
	m.board = m.board.InvalidateViews()
	m.toaster = m.toaster.Show("Issue deleted", toaster.StyleSuccess)
	return m, tea.Batch(
		m.board.LoadAllColumns(),
		toaster.ScheduleDismiss(2*time.Second),
	)
}

// handleLabelsChanged processes label change results.
func (m Model) handleLabelsChanged(msg labelsChangedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.err = msg.err
		m.errContext = "updating labels"
		return m, scheduleErrorClear()
	}
	// Update details view to show new labels
	m.details = m.details.UpdateLabels(msg.labels)
	m.toaster = m.toaster.Show("Labels updated", toaster.StyleSuccess)
	return m, toaster.ScheduleDismiss(2 * time.Second)
}

// handleErrMsg processes error messages.
func (m Model) handleErrMsg(msg errMsg) (Model, tea.Cmd) {
	m.err = msg.err
	m.errContext = msg.context
	// Show error toaster for loading failures
	if msg.context == "loading issues" {
		m.toaster = m.toaster.Show("failed to load issues", toaster.StyleError)
		return m, tea.Batch(scheduleErrorClear(), toaster.ScheduleDismiss(3*time.Second))
	}
	return m, scheduleErrorClear()
}

// HandleDBChanged processes database change notifications from the app.
// This is called by app.go when the centralized watcher detects changes.
// The app handles re-subscription; this method just triggers the refresh.
func (m Model) HandleDBChanged() (Model, tea.Cmd) {
	// Don't refresh if already loading or not in ViewBoard
	if m.loading || m.view != ViewBoard {
		return m, nil
	}

	// Trigger refresh
	m.pendingCursor = m.saveCursor()
	m.loading = true
	m.autoRefreshed = true
	m.manualRefreshed = false

	// Invalidate other views so they reload when switched to
	m.board = m.board.InvalidateViews()

	// Only reload current view if views are configured, otherwise load all
	var loadCmd tea.Cmd
	if m.board.ViewCount() > 0 {
		loadCmd = m.board.LoadCurrentViewCmd()
	} else {
		loadCmd = m.board.LoadAllColumns()
	}

	return m, tea.Batch(m.spinner.Tick, loadCmd)
}

// handleColEditorSave processes column editor save.
func (m Model) handleColEditorSave(msg coleditor.SaveMsg) (Model, tea.Cmd) {
	viewIndex := m.currentViewIndex()
	columns := m.currentViewColumns()
	err := config.UpdateColumnInView(m.configPath(), viewIndex, msg.ColumnIndex, msg.Config, columns, m.services.Config.Views)
	if err != nil {
		m.err = err
		m.errContext = "saving column config"
		m.view = ViewBoard
		return m, scheduleErrorClear()
	}

	// Update in-memory config
	columns[msg.ColumnIndex] = msg.Config
	m.services.Config.SetColumnsForView(viewIndex, columns)

	// Rebuild board with new config
	m.rebuildBoard()

	m.view = ViewBoard
	m.loading = true
	m.toaster = m.toaster.Show("Column saved", toaster.StyleSuccess)
	cmds := []tea.Cmd{m.spinner.Tick, toaster.ScheduleDismiss(2 * time.Second)}
	if loadCmd := m.loadBoardCmd(); loadCmd != nil {
		cmds = append(cmds, loadCmd)
	}
	return m, tea.Batch(cmds...)
}

// handleColEditorDelete processes column editor delete.
func (m Model) handleColEditorDelete(msg coleditor.DeleteMsg) (Model, tea.Cmd) {
	viewIndex := m.currentViewIndex()
	columns := m.currentViewColumns()
	err := config.DeleteColumnInView(m.configPath(), viewIndex, msg.ColumnIndex, columns, m.services.Config.Views)
	if err != nil {
		m.err = err
		m.errContext = "deleting column"
		m.view = ViewBoard
		return m, scheduleErrorClear()
	}

	// Update in-memory config (remove the column)
	newColumns := append(columns[:msg.ColumnIndex], columns[msg.ColumnIndex+1:]...)
	m.services.Config.SetColumnsForView(viewIndex, newColumns)

	// Rebuild board with new config
	m.rebuildBoard()

	m.view = ViewBoard
	m.loading = true
	m.toaster = m.toaster.Show("Column deleted", toaster.StyleSuccess)
	cmds := []tea.Cmd{m.spinner.Tick, toaster.ScheduleDismiss(2 * time.Second)}
	if loadCmd := m.loadBoardCmd(); loadCmd != nil {
		cmds = append(cmds, loadCmd)
	}
	return m, tea.Batch(cmds...)
}

// handleColEditorAdd processes column editor add.
func (m Model) handleColEditorAdd(msg coleditor.AddMsg) (Model, tea.Cmd) {
	viewIndex := m.currentViewIndex()
	columns := m.currentViewColumns()

	err := config.AddColumnInView(m.configPath(), viewIndex, msg.InsertAfterIndex, msg.Config, columns, m.services.Config.Views)
	if err != nil {
		m.err = err
		m.errContext = "adding column"
		m.view = ViewBoard
		return m, scheduleErrorClear()
	}

	// Update in-memory config (insert the column)
	insertPos := msg.InsertAfterIndex + 1
	newColumns := make([]config.ColumnConfig, 0, len(columns)+1)
	for i, col := range columns {
		if i == insertPos {
			newColumns = append(newColumns, msg.Config)
		}
		newColumns = append(newColumns, col)
	}
	if insertPos >= len(columns) {
		newColumns = append(newColumns, msg.Config)
	}
	m.services.Config.SetColumnsForView(viewIndex, newColumns)

	// Rebuild board with new config
	m.rebuildBoardWithFocus(insertPos)

	m.view = ViewBoard
	m.loading = true
	m.toaster = m.toaster.Show("Column added", toaster.StyleSuccess)
	cmds := []tea.Cmd{m.spinner.Tick, toaster.ScheduleDismiss(2 * time.Second)}
	if loadCmd := m.loadBoardCmd(); loadCmd != nil {
		cmds = append(cmds, loadCmd)
	}
	return m, tea.Batch(cmds...)
}

// handleModalSubmit processes modal submission.
func (m Model) handleModalSubmit(msg modal.SubmitMsg) (Model, tea.Cmd) {
	if m.view == ViewNewViewModal {
		return m.createNewView(msg.Values["name"])
	}
	if m.view == ViewDeleteViewModal {
		return m.deleteCurrentView()
	}
	if m.view == ViewDeleteColumnModal {
		return m.deleteColumn()
	}
	if m.view == ViewRenameViewModal {
		return m.renameCurrentView(msg.Values["name"])
	}
	if m.view == ViewDeleteConfirm {
		if m.selectedIssue != nil {
			issueID := m.selectedIssue.ID
			cascade := m.deleteIsCascade
			m.selectedIssue = nil
			m.deleteIsCascade = false
			return m, deleteIssueCmd(issueID, cascade)
		}
		m.view = ViewBoard
		m.deleteIsCascade = false
		return m, nil
	}
	// Route to column editor for delete confirmation modal
	if m.view == ViewColumnEditor {
		var cmd tea.Cmd
		m.colEditor, cmd = m.colEditor.Update(modal.SubmitMsg{})
		return m, cmd
	}
	return m, nil
}

// handleModalCancel processes modal cancellation.
func (m Model) handleModalCancel() (Model, tea.Cmd) {
	if m.view == ViewNewViewModal || m.view == ViewDeleteViewModal || m.view == ViewDeleteColumnModal || m.view == ViewRenameViewModal {
		m.view = ViewBoard
		m.pendingDeleteColumn = -1
		return m, nil
	}
	if m.view == ViewDeleteConfirm {
		m.view = ViewDetails
		m.selectedIssue = nil
		m.deleteIsCascade = false
		return m, nil
	}
	// Route to column editor for delete confirmation modal
	if m.view == ViewColumnEditor {
		var cmd tea.Cmd
		m.colEditor, cmd = m.colEditor.Update(modal.CancelMsg{})
		return m, cmd
	}
	return m, nil
}
