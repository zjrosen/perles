package kanban

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/zjrosen/perles/internal/config"
	"github.com/zjrosen/perles/internal/keys"
	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/mode"
	"github.com/zjrosen/perles/internal/ui/coleditor"
	"github.com/zjrosen/perles/internal/ui/shared/modal"
	"github.com/zjrosen/perles/internal/ui/shared/picker"
	"github.com/zjrosen/perles/internal/ui/shared/toaster"
)

// handleKey routes key messages to the appropriate handler based on view mode.
func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch m.view {
	case ViewBoard:
		return m.handleBoardKey(msg)
	case ViewDetails:
		return m.handleDetailsKey(msg)
	case ViewHelp:
		switch {
		case key.Matches(msg, keys.Common.Quit):
			return m, tea.Quit
		case key.Matches(msg, keys.Common.Escape), key.Matches(msg, keys.Common.Help):
			m.view = ViewBoard
			return m, nil
		}
		return m, nil
	case ViewDetailsPriorityPicker, ViewDetailsStatusPicker:
		return m.handlePickerKey(msg)
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
	case ViewDetailsEditMenu:
		return m.handleDetailsEditMenuKey(msg)
	}
	return m, nil
}

func (m Model) handleBoardKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	// Dismiss error on any key press (except quit)
	// Don't return early - let the key continue to be processed
	if m.err != nil && !key.Matches(msg, keys.Common.Quit) {
		m.err = nil
		m.errContext = ""
	}

	switch {
	case key.Matches(msg, keys.Common.Quit):
		return m, tea.Quit

	case key.Matches(msg, keys.Common.Help):
		m.view = ViewHelp
		return m, nil

	case key.Matches(msg, keys.Kanban.Refresh):
		// Save cursor before refresh to restore position after
		m.pendingCursor = m.saveCursor()
		m.loading = true
		m.manualRefreshed = true
		m.autoRefreshed = false
		// Invalidate other views so they reload when switched to
		m.board = m.board.InvalidateViews()
		return m, m.board.LoadAllColumns()

	case key.Matches(msg, keys.Kanban.Yank):
		// Yank (copy) selected issue ID to clipboard
		if issue := m.board.SelectedIssue(); issue != nil {
			if err := m.services.Clipboard.Copy(issue.ID); err != nil {
				m.err = err
				m.errContext = "copying to clipboard"
				return m, scheduleErrorClear()
			}
			return m, func() tea.Msg { return mode.ShowToastMsg{Message: "Copied: " + issue.ID, Style: toaster.StyleSuccess} }
		}
		return m, nil

	case key.Matches(msg, keys.Kanban.ToggleStatus):
		// Toggle status bar visibility
		m.showStatusBar = !m.showStatusBar
		// Recalculate board height since available space changed
		m.board = m.board.SetSize(m.width, m.boardHeight())
		return m, nil

	case key.Matches(msg, keys.Kanban.EditColumn):
		// Open column editor for focused column
		focusedCol := m.board.FocusedColumn()
		columns := m.currentViewColumns()
		if focusedCol >= 0 && focusedCol < len(columns) {
			// Pass executor for BQL preview queries and vim mode setting
			vimEnabled := m.services.Config.UI.VimMode
			m.colEditor = coleditor.New(focusedCol, columns, m.services.Executor, vimEnabled).
				SetSize(m.width, m.height)
			m.view = ViewColumnEditor
		}
		return m, nil

	case key.Matches(msg, keys.Kanban.AddColumn):
		// Open column editor in New mode (insert after focused column)
		focusedCol := m.board.FocusedColumn()
		columns := m.currentViewColumns()

		// Handle empty view - insert at position -1 (will become position 0)
		insertAfter := focusedCol
		if len(columns) == 0 {
			insertAfter = -1
		}

		// Create editor in New mode
		vimEnabled := m.services.Config.UI.VimMode
		m.colEditor = coleditor.NewForCreate(insertAfter, columns, m.services.Executor, vimEnabled).
			SetSize(m.width, m.height)
		m.view = ViewColumnEditor
		return m, nil

	case key.Matches(msg, keys.Kanban.MoveColumnLeft):
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

		// Swap columns in place and move focus
		m.board = m.board.SwapColumns(focusedCol, focusedCol-1).SetFocus(focusedCol - 1)
		return m, nil

	case key.Matches(msg, keys.Kanban.MoveColumnRight):
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

		// Swap columns in place and move focus
		m.board = m.board.SwapColumns(focusedCol, focusedCol+1).SetFocus(focusedCol + 1)
		return m, nil

	case key.Matches(msg, keys.Kanban.NextView):
		if m.board.ViewCount() > 1 {
			var cmd tea.Cmd
			m.board, cmd = m.board.CycleViewNext()

			// Show toast only if status bar is hidden
			var toastCmd tea.Cmd
			if !m.showStatusBar {
				viewName := m.board.CurrentViewName()
				viewNum := m.board.CurrentViewIndex() + 1
				viewTotal := m.board.ViewCount()
				toastCmd = func() tea.Msg {
					return mode.ShowToastMsg{
						Message: fmt.Sprintf("View: %s (%d/%d)", viewName, viewNum, viewTotal),
						Style:   toaster.StyleInfo,
					}
				}
			}

			if cmd != nil {
				m.loading = true
				if toastCmd != nil {
					return m, tea.Batch(cmd, toastCmd)
				}
				return m, cmd
			}
			return m, toastCmd
		}
		return m, nil

	case key.Matches(msg, keys.Kanban.PrevView):
		if m.board.ViewCount() > 1 {
			var cmd tea.Cmd
			m.board, cmd = m.board.CycleViewPrev()

			// Show toast only if status bar is hidden
			var toastCmd tea.Cmd
			if !m.showStatusBar {
				viewName := m.board.CurrentViewName()
				viewNum := m.board.CurrentViewIndex() + 1
				viewTotal := m.board.ViewCount()
				toastCmd = func() tea.Msg {
					return mode.ShowToastMsg{
						Message: fmt.Sprintf("View: %s (%d/%d)", viewName, viewNum, viewTotal),
						Style:   toaster.StyleInfo,
					}
				}
			}

			if cmd != nil {
				m.loading = true
				if toastCmd != nil {
					return m, tea.Batch(cmd, toastCmd)
				}
				return m, cmd
			}
			return m, toastCmd
		}
		return m, nil

	case key.Matches(msg, keys.Kanban.ViewMenu):
		m.picker = picker.NewWithConfig(picker.Config{
			Title: "View Menu",
			Options: []picker.Option{
				{Label: "Create new view", Value: "create"},
				{Label: "Delete current view", Value: "delete"},
				{Label: "Rename current view", Value: "rename"},
			},
			OnSelect: func(opt picker.Option) tea.Msg {
				switch opt.Value {
				case "create":
					return viewMenuCreateMsg{}
				case "delete":
					return viewMenuDeleteMsg{}
				case "rename":
					return viewMenuRenameMsg{}
				}
				return nil
			},
			OnCancel: func() tea.Msg { return pickerCancelledMsg{} },
		}).SetSize(m.width, m.height)
		m.view = ViewViewMenu
		return m, nil

	case key.Matches(msg, keys.Kanban.DeleteColumn):
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

	case key.Matches(msg, keys.Kanban.Enter):
		// Open search mode in tree sub-mode for the selected issue
		if issue := m.board.SelectedIssue(); issue != nil {
			return m, func() tea.Msg {
				return SwitchToSearchMsg{
					SubMode: mode.SubModeTree,
					IssueID: issue.ID,
				}
			}
		}
		return m, nil

	case key.Matches(msg, keys.Kanban.SearchFromColumn):
		// Switch to search mode in list sub-mode with current column's BQL query
		focusedCol := m.board.FocusedColumn()
		query := ""
		if focusedCol >= 0 && focusedCol < m.board.ColCount() {
			query = m.board.Column(focusedCol).Query()
		}
		return m, func() tea.Msg {
			return SwitchToSearchMsg{
				SubMode: mode.SubModeList,
				Query:   query,
			}
		}

	case key.Matches(msg, keys.Kanban.Orchestrate):
		// Start orchestration mode
		return m, func() tea.Msg {
			return SwitchToOrchestrationMsg{}
		}
	}

	// Delegate navigation to board
	var cmd tea.Cmd
	m.board, cmd = m.board.Update(msg)
	return m, cmd
}

func (m Model) handleDetailsKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Common.Quit):
		return m, tea.Quit
	case key.Matches(msg, keys.Common.Escape), key.Matches(msg, keys.Kanban.Escape):
		m.view = ViewBoard
		// Save cursor to return to the same issue after refresh
		m.pendingCursor = &cursorState{
			column:  m.board.FocusedColumn(),
			issueID: m.details.IssueID(),
		}
		// Invalidate views and reload to show any changes made in details view
		m.board = m.board.InvalidateViews()
		return m, m.board.LoadAllColumns()
	case key.Matches(msg, keys.Kanban.Yank):
		// Yank (copy) the issue ID shown in the details view
		issueID := m.details.IssueID()
		if issueID == "" {
			return m, nil
		}
		if err := m.services.Clipboard.Copy(issueID); err != nil {
			m.err = err
			m.errContext = "copying to clipboard"
			return m, scheduleErrorClear()
		}
		return m, func() tea.Msg { return mode.ShowToastMsg{Message: "Copied: " + issueID, Style: toaster.StyleSuccess} }
	}

	// Delegate to details view
	var cmd tea.Cmd
	m.details, cmd = m.details.Update(msg)
	return m, cmd
}

// handlePickerKey handles key events for all picker views.
// The picker's callbacks produce domain-specific messages.
func (m Model) handlePickerKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if key.Matches(msg, keys.Common.Quit) {
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.picker, cmd = m.picker.Update(msg)
	return m, cmd
}

func (m Model) handleColumnEditorKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if key.Matches(msg, keys.Common.Quit) {
		return m, tea.Quit
	}

	// Delegate to column editor
	var cmd tea.Cmd
	m.colEditor, cmd = m.colEditor.Update(msg)
	return m, cmd
}

func (m Model) handleNewViewModalKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if key.Matches(msg, keys.Common.Quit) {
		return m, tea.Quit
	}

	// Delegate to modal
	var cmd tea.Cmd
	m.modal, cmd = m.modal.Update(msg)
	return m, cmd
}

func (m Model) handleDeleteViewModalKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if key.Matches(msg, keys.Common.Quit) {
		return m, tea.Quit
	}

	// Delegate to modal
	var cmd tea.Cmd
	m.modal, cmd = m.modal.Update(msg)
	return m, cmd
}

func (m Model) handleDeleteConfirmKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if key.Matches(msg, keys.Common.Quit) {
		return m, tea.Quit
	}

	// Delegate to modal
	var cmd tea.Cmd
	m.modal, cmd = m.modal.Update(msg)
	return m, cmd
}

func (m Model) handleLabelEditorKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if key.Matches(msg, keys.Common.Quit) {
		return m, tea.Quit
	}

	// Delegate to label editor
	var cmd tea.Cmd
	m.labelEditor, cmd = m.labelEditor.Update(msg)
	return m, cmd
}

func (m Model) handleViewMenuKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if key.Matches(msg, keys.Common.Quit) {
		return m, tea.Quit
	}

	// Delegate to picker
	var cmd tea.Cmd
	m.picker, cmd = m.picker.Update(msg)
	return m, cmd
}

func (m Model) handleDetailsEditMenuKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if key.Matches(msg, keys.Common.Quit) {
		return m, tea.Quit
	}

	// Delegate to picker
	var cmd tea.Cmd
	m.picker, cmd = m.picker.Update(msg)
	return m, cmd
}

func (m Model) handleDeleteColumnModalKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if key.Matches(msg, keys.Common.Quit) {
		return m, tea.Quit
	}

	// Delegate to modal
	var cmd tea.Cmd
	m.modal, cmd = m.modal.Update(msg)
	return m, cmd
}

func (m Model) handleRenameViewModalKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if key.Matches(msg, keys.Common.Quit) {
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

	// SQLite queries are instant, so treat every load message as completion
	m.loading = false

	// Restore cursor if we have a pending state
	if m.pendingCursor != nil {
		m = m.restoreCursor(m.pendingCursor)
		m.pendingCursor = nil
	}
	// Auto sync is silent, manual refresh shows toaster
	m.autoRefreshed = false
	if m.manualRefreshed {
		m.manualRefreshed = false
		return m, func() tea.Msg { return mode.ShowToastMsg{Message: "refreshed issues", Style: toaster.StyleSuccess} }
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
	return m, tea.Batch(
		m.board.LoadAllColumns(),
		func() tea.Msg { return mode.ShowToastMsg{Message: "Issue deleted", Style: toaster.StyleSuccess} },
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
	return m, func() tea.Msg { return mode.ShowToastMsg{Message: "Labels updated", Style: toaster.StyleSuccess} }
}

// handleErrMsg processes error messages.
func (m Model) handleErrMsg(msg errMsg) (Model, tea.Cmd) {
	m.err = msg.err
	m.errContext = msg.context
	// Show error toaster for loading failures
	if msg.context == "loading issues" {
		return m, tea.Batch(
			scheduleErrorClear(),
			func() tea.Msg { return mode.ShowToastMsg{Message: "failed to load issues", Style: toaster.StyleError} },
		)
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
	if m.board.ViewCount() > 0 {
		return m, m.board.LoadCurrentViewCmd()
	}
	return m, m.board.LoadAllColumns()
}

// handleColEditorSave processes column editor save.
func (m Model) handleColEditorSave(msg coleditor.SaveMsg) (Model, tea.Cmd) {
	viewIndex := m.currentViewIndex()
	columns := m.currentViewColumns()
	err := config.UpdateColumnInView(m.configPath(), viewIndex, msg.ColumnIndex, msg.Config, columns, m.services.Config.Views)
	if err != nil {
		log.ErrorErr(log.CatConfig, "Failed to save column config", err,
			"viewIndex", viewIndex,
			"columnIndex", msg.ColumnIndex)
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
	cmds := []tea.Cmd{
		func() tea.Msg { return mode.ShowToastMsg{Message: "Column saved", Style: toaster.StyleSuccess} },
	}
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
		log.ErrorErr(log.CatConfig, "Failed to delete column", err,
			"viewIndex", viewIndex,
			"columnIndex", msg.ColumnIndex)
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
	cmds := []tea.Cmd{
		func() tea.Msg { return mode.ShowToastMsg{Message: "Column deleted", Style: toaster.StyleSuccess} },
	}
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
		log.ErrorErr(log.CatConfig, "Failed to add column", err,
			"viewIndex", viewIndex,
			"insertAfterIndex", msg.InsertAfterIndex)
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
	cmds := []tea.Cmd{
		func() tea.Msg { return mode.ShowToastMsg{Message: "Column added", Style: toaster.StyleSuccess} },
	}
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
			issueIDs := m.deleteIssueIDs
			m.selectedIssue = nil
			m.deleteIssueIDs = nil
			return m, deleteIssueCmd(issueIDs)
		}
		m.view = ViewBoard
		m.deleteIssueIDs = nil
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
		m.deleteIssueIDs = nil
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
