// Package kanban implements the kanban board mode controller.
package kanban

import (
	"fmt"
	"os/exec"
	"perles/internal/beads"
	"perles/internal/config"
	"perles/internal/mode"
	"perles/internal/ui/board"
	"perles/internal/ui/coleditor"
	"perles/internal/ui/colorpicker"
	"perles/internal/ui/details"
	"perles/internal/ui/help"
	"perles/internal/ui/labeleditor"
	"perles/internal/ui/modal"
	"perles/internal/ui/picker"
	"perles/internal/ui/styles"
	"perles/internal/ui/toaster"
	"perles/internal/ui/viewmenu"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ViewMode determines which view is active within the kanban mode.
type ViewMode int

const (
	ViewBoard ViewMode = iota
	ViewDetails
	ViewHelp
	ViewDetailsPriorityPicker
	ViewDetailsStatusPicker
	ViewColumnEditor
	ViewNewViewModal
	ViewDeleteViewModal
	ViewDeleteConfirm
	ViewLabelEditor
	ViewViewMenu
	ViewDeleteColumnModal
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
	details     details.Model
	help        help.Model
	picker      picker.Model
	colEditor   coleditor.Model
	modal       modal.Model
	labelEditor labeleditor.Model
	viewMenu    viewmenu.Model
	spinner     spinner.Model
	view        ViewMode
	width       int
	height      int
	loading     bool
	err         error
	errContext  string // Context for the error (e.g., "updating status")

	// Currently selected issue for picker operations
	selectedIssue *beads.Issue

	// Delete operation state
	deleteIsCascade     bool // True if deleting an epic with children
	pendingDeleteColumn int  // Index of column to delete, -1 if none

	// Pending cursor restoration after refresh
	pendingCursor *cursorState

	// Toaster notification overlay
	toaster toaster.Model

	// Refresh state tracking
	autoRefreshed   bool // Set when refresh triggered by file watcher
	manualRefreshed bool // Set when refresh triggered by 'r' key

	// UI visibility toggles
	showStatusBar bool
}

// New creates a new kanban mode controller.
func New(services mode.Services) Model {
	// Apply theme colors from config
	styles.ApplyTheme(services.Config.Theme.Subtle, services.Config.Theme.Error, services.Config.Theme.Success)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(styles.SpinnerColor)

	// Create board - use views if configured, otherwise fall back to columns
	var boardModel board.Model
	if len(services.Config.Views) > 0 {
		boardModel = board.NewFromViews(services.Config.Views, services.Executor).SetShowCounts(services.Config.UI.ShowCounts)
	} else {
		columns := services.Config.GetColumns()
		boardModel = board.NewFromConfigWithExecutor(columns, services.Executor).SetShowCounts(services.Config.UI.ShowCounts)
	}

	return Model{
		services:            services,
		view:                ViewBoard,
		board:               boardModel,
		help:                help.New(),
		spinner:             s,
		loading:             true,
		showStatusBar:       services.Config.UI.ShowStatusBar,
		pendingDeleteColumn: -1,
	}
}

// Init returns initial commands for the mode.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.spinner.Tick}

	// Trigger initial column load via BQL
	if loadCmd := m.board.LoadAllColumns(); loadCmd != nil {
		cmds = append(cmds, loadCmd)
	}

	return tea.Batch(cmds...)
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
	m.rebuildBoard()
	m.loading = true
	return m, m.loadBoardCmd()
}

// SetSize handles terminal resize.
func (m Model) SetSize(width, height int) Model {
	m.width = width
	m.height = height
	m.board = m.board.SetSize(width, m.boardHeight())
	m.help = m.help.SetSize(width, height)
	// Update details if we're viewing it (handles terminal resize while in details view)
	if m.view == ViewDetails {
		m.details = m.details.SetSize(width, height)
	}
	// Update column editor if we're viewing it
	if m.view == ViewColumnEditor {
		m.colEditor = m.colEditor.SetSize(width, height)
	}
	// Update modal if we're viewing it
	if m.view == ViewNewViewModal || m.view == ViewDeleteViewModal || m.view == ViewDeleteConfirm || m.view == ViewDeleteColumnModal {
		m.modal.SetSize(width, height)
	}
	// Update view menu if we're viewing it
	if m.view == ViewViewMenu {
		m.viewMenu = m.viewMenu.SetSize(width, height)
	}
	return m
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case board.ColumnLoadedMsg:
		return m.handleColumnLoaded(msg)

	case statusChangedMsg:
		return m.handleStatusChanged(msg)

	case priorityChangedMsg:
		return m.handlePriorityChanged(msg)

	case issueDeletedMsg:
		return m.handleIssueDeleted(msg)

	// Open picker messages from details view
	case details.OpenPriorityPickerMsg:
		m.picker = picker.New("Priority", priorityOptions()).
			SetSize(m.width, m.height).
			SetSelected(int(msg.Current))
		m.selectedIssue = m.getIssueByID(msg.IssueID)
		m.view = ViewDetailsPriorityPicker
		return m, nil

	case details.OpenStatusPickerMsg:
		m.picker = picker.New("Status", statusOptions()).
			SetSize(m.width, m.height).
			SetSelected(picker.FindIndexByValue(statusOptions(), string(msg.Current)))
		m.selectedIssue = m.getIssueByID(msg.IssueID)
		m.view = ViewDetailsStatusPicker
		return m, nil

	case details.DeleteIssueMsg:
		issue := m.getIssueByID(msg.IssueID)
		if issue == nil {
			return m, nil
		}
		m.modal, m.deleteIsCascade = m.createDeleteModal(issue)
		m.modal.SetSize(m.width, m.height)
		m.selectedIssue = issue
		m.view = ViewDeleteConfirm
		return m, m.modal.Init()

	case details.NavigateToDependencyMsg:
		issue := m.getIssueByID(msg.IssueID)
		if issue == nil {
			return m, nil
		}
		// Create new details view for the dependency (pass client for nested deps)
		m.details = details.New(*issue, m.services.Client).SetSize(m.width, m.height)
		return m, nil

	case details.OpenLabelEditorMsg:
		m.labelEditor = labeleditor.New(msg.IssueID, msg.Labels).
			SetSize(m.width, m.height)
		m.view = ViewLabelEditor
		return m, m.labelEditor.Init()

	case labeleditor.SaveMsg:
		m.view = ViewDetails
		return m, setLabelsCmd(msg.IssueID, msg.Labels)

	case labeleditor.CancelMsg:
		m.view = ViewDetails
		return m, nil

	case labelsChangedMsg:
		return m.handleLabelsChanged(msg)

	case errMsg:
		return m.handleErrMsg(msg)

	case clearErrorMsg:
		m.err = nil
		m.errContext = ""
		return m, nil

	case toaster.DismissMsg:
		m.toaster = m.toaster.Hide()
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

	case viewmenu.SelectMsg:
		return m.handleViewMenuSelect(msg)

	case viewmenu.CancelMsg:
		m.view = ViewBoard
		return m, nil

	case modal.SubmitMsg:
		return m.handleModalSubmit(msg)

	case modal.CancelMsg:
		return m.handleModalCancel()
	}

	return m, nil
}

// View renders the kanban mode.
func (m Model) View() string {
	switch m.view {
	case ViewDetails:
		return m.details.View()
	case ViewDetailsPriorityPicker, ViewDetailsStatusPicker:
		// Render picker overlay on top of details view
		return m.picker.Overlay(m.details.View())
	case ViewHelp:
		// Render help overlay on top of board
		bg := m.board.View()
		if m.showStatusBar {
			bg += "\n" + m.renderStatusBar()
		}
		return m.help.Overlay(bg)
	case ViewColumnEditor:
		// Full-screen column editor
		return m.colEditor.View()
	case ViewNewViewModal, ViewDeleteViewModal:
		// Render modal overlay on top of board
		bg := m.board.View()
		if m.showStatusBar {
			bg += "\n" + m.renderStatusBar()
		}
		return m.modal.Overlay(bg)
	case ViewDeleteConfirm:
		// Render modal overlay on top of details view
		return m.modal.Overlay(m.details.View())
	case ViewLabelEditor:
		// Render label editor overlay on top of details view
		return m.labelEditor.Overlay(m.details.View())
	case ViewViewMenu:
		// Render view menu overlay on top of board
		bg := m.board.View()
		if m.showStatusBar {
			bg += "\n" + m.renderStatusBar()
		}
		return m.viewMenu.Overlay(bg)
	case ViewDeleteColumnModal:
		// Render delete column modal overlay on top of board
		bg := m.board.View()
		if m.showStatusBar {
			bg += "\n" + m.renderStatusBar()
		}
		return m.modal.Overlay(bg)
	default:
		// Add top margin for spacing
		view := m.board.View()
		if m.showStatusBar {
			view += "\n"
			if m.err != nil {
				view += m.renderErrorBar()
			} else {
				view += m.renderStatusBar()
			}
		}
		// Overlay toaster if visible (works even when status bar is hidden)
		if m.toaster.Visible() {
			view = m.toaster.Overlay(view, m.width, m.height)
		}
		return view
	}
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
func (m Model) restoreCursor(state *cursorState) Model {
	if state == nil {
		return m
	}

	// Try to find the issue by ID (may have moved columns)
	newBoard, found := m.board.SelectByID(state.issueID)
	if found {
		m.board = newBoard
	} else {
		// Issue not found, stay in same column
		m.board = m.board.SetFocus(state.column)
	}
	return m
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

	if len(m.services.Config.Views) > 0 {
		m.board = board.NewFromViews(m.services.Config.Views, m.services.Executor).
			SetShowCounts(m.services.Config.UI.ShowCounts).
			SetSize(m.width, m.boardHeight())
		// Restore view index if valid
		if currentView > 0 && currentView < m.board.ViewCount() {
			m.board, _ = m.board.SwitchToView(currentView)
		}
	} else {
		m.board = board.NewFromConfigWithExecutor(m.services.Config.GetColumns(), m.services.Executor).
			SetShowCounts(m.services.Config.UI.ShowCounts).
			SetSize(m.width, m.boardHeight())
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

// findIssueByIDFromColumns searches loaded columns for an issue by ID.
func (m Model) findIssueByIDFromColumns(id string) *beads.Issue {
	for i := 0; i < m.board.ColCount(); i++ {
		col := m.board.Column(i)
		for _, issue := range col.Items() {
			if issue.ID == id {
				return &issue
			}
		}
	}
	return nil
}

// getIssueByID fetches an issue from the existing columns first.
func (m Model) getIssueByID(id string) *beads.Issue {
	issue := m.findIssueByIDFromColumns(id)
	if issue != nil {
		return issue
	}

	if m.services.Client != nil {
		issues, err := m.services.Client.ListIssuesByIds([]string{id})
		if err == nil && len(issues) == 1 {
			return &issues[0]
		}
	}

	return nil
}

// getIssuesByIds fetches multiple issues in a single query.
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

// createDeleteModal creates a confirmation modal for issue deletion.
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
	m.toaster = m.toaster.Show("Column deleted", toaster.StyleSuccess)
	cmds := []tea.Cmd{m.spinner.Tick, toaster.ScheduleDismiss(2 * time.Second)}
	if loadCmd := m.loadBoardCmd(); loadCmd != nil {
		cmds = append(cmds, loadCmd)
	}
	return m, tea.Batch(cmds...)
}

// handleViewMenuSelect routes view menu selections to appropriate actions.
func (m Model) handleViewMenuSelect(msg viewmenu.SelectMsg) (Model, tea.Cmd) {
	switch msg.Option {
	case viewmenu.OptionCreate:
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

	case viewmenu.OptionDelete:
		// Prevent deletion of last view
		if len(m.services.Config.Views) <= 1 {
			m.toaster = m.toaster.Show("Cannot delete the only view", toaster.StyleError)
			m.view = ViewBoard
			return m, toaster.ScheduleDismiss(3 * time.Second)
		}

		viewName := m.board.CurrentViewName()
		m.modal = modal.New(modal.Config{
			Title:          "Delete View",
			Message:        fmt.Sprintf("Delete view '%s'? This cannot be undone.", viewName),
			ConfirmVariant: modal.ButtonDanger,
		})
		m.modal.SetSize(m.width, m.height)
		m.view = ViewDeleteViewModal
		return m, m.modal.Init()

	case viewmenu.OptionRename:
		// Rename flow will be implemented in Phase 3
		m.toaster = m.toaster.Show("Rename not yet implemented", toaster.StyleInfo)
		m.view = ViewBoard
		return m, toaster.ScheduleDismiss(2 * time.Second)
	}

	m.view = ViewBoard
	return m, nil
}

// createNewView handles the creation of a new view after modal submission.
func (m Model) createNewView(viewName string) (Model, tea.Cmd) {
	newView := config.ViewConfig{
		Name:    viewName,
		Columns: []config.ColumnConfig{},
	}

	err := config.AddView(m.configPath(), newView, m.services.Config.Views)
	if err != nil {
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
	m.toaster = m.toaster.Show("Created view: "+viewName, toaster.StyleSuccess)
	cmds := []tea.Cmd{m.spinner.Tick, toaster.ScheduleDismiss(2 * time.Second)}
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
		m.err = err
		m.errContext = "deleting view"
		m.view = ViewBoard
		return m, scheduleErrorClear()
	}

	// Update in-memory config
	m.services.Config.Views = append(m.services.Config.Views[:viewIndex], m.services.Config.Views[viewIndex+1:]...)

	// Determine which view to switch to
	newViewIndex := viewIndex - 1
	if newViewIndex < 0 {
		newViewIndex = 0
	}

	// Rebuild board with updated views
	m.view = ViewBoard
	m.rebuildBoard()
	m.board, _ = m.board.SwitchToView(newViewIndex)

	m.loading = true
	m.toaster = m.toaster.Show("Deleted view: "+viewName, toaster.StyleSuccess)
	cmds := []tea.Cmd{m.spinner.Tick, toaster.ScheduleDismiss(2 * time.Second)}
	if loadCmd := m.board.LoadCurrentViewCmd(); loadCmd != nil {
		cmds = append(cmds, loadCmd)
	}
	return m, tea.Batch(cmds...)
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

// Message types

// SwitchToSearchMsg requests switching to search mode with an initial query.
type SwitchToSearchMsg struct {
	Query string
}

type errMsg struct {
	err     error
	context string
}

type clearErrorMsg struct{}

type clearRefreshIndicatorMsg struct{}

type statusChangedMsg struct {
	issueID string
	status  beads.Status
	err     error
}

type priorityChangedMsg struct {
	issueID  string
	priority beads.Priority
	err      error
}

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

func updateStatusCmd(issueID string, status beads.Status) tea.Cmd {
	return func() tea.Msg {
		err := beads.UpdateStatus(issueID, status)
		return statusChangedMsg{issueID, status, err}
	}
}

func updatePriorityCmd(issueID string, priority beads.Priority) tea.Cmd {
	return func() tea.Msg {
		err := beads.UpdatePriority(issueID, priority)
		return priorityChangedMsg{issueID, priority, err}
	}
}

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

func scheduleErrorClear() tea.Cmd {
	return tea.Tick(3*time.Second, func(_ time.Time) tea.Msg {
		return clearErrorMsg{}
	})
}
