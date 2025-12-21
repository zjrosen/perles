// Package coleditor provides a full-screen column configuration editor with live preview.
package coleditor

import (
	"fmt"
	"perles/internal/beads"
	"perles/internal/bql"
	"perles/internal/config"
	"perles/internal/mode/shared"
	"perles/internal/ui/board"
	"perles/internal/ui/forms/bqlinput"
	"perles/internal/ui/modals/help"
	"perles/internal/ui/shared/colorpicker"
	"perles/internal/ui/shared/modal"
	"perles/internal/ui/styles"
	"perles/internal/ui/tree"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Validation regex for hex color format
var hexColorRegex = regexp.MustCompile(`^#([0-9A-Fa-f]{3}|[0-9A-Fa-f]{6})$`)

// Mode indicates whether the editor is creating or editing a column.
type Mode int

const (
	ModeEdit Mode = iota
	ModeNew
)

// Field identifies which form field is focused.
type Field int

const (
	FieldName Field = iota
	FieldType       // Type selector: BQL Query or Tree View
	FieldColor
	FieldQuery    // BQL query input (shown when type=bql)
	FieldIssueID  // Issue ID input (shown when type=tree)
	FieldTreeMode // Tree mode selector (shown when type=tree)
	FieldSave
	FieldDelete
	fieldCount
)

// SaveMsg is sent when the user saves the editor.
type SaveMsg struct {
	ColumnIndex int
	Config      config.ColumnConfig
}

// CancelMsg is sent when the user cancels the editor.
type CancelMsg struct{}

// DeleteMsg is sent when the user deletes the column.
type DeleteMsg struct {
	ColumnIndex int
}

// AddMsg is sent when the user saves a new column.
type AddMsg struct {
	InsertAfterIndex int
	Config           config.ColumnConfig
}

// Model holds the editor state.
type Model struct {
	mode            Mode                  // Edit or New mode
	columnIndex     int                   // Index of column being edited (or preview index for new)
	insertAfter     int                   // Index to insert after (for ModeNew)
	original        config.ColumnConfig   // Original config (for cancel in edit mode)
	allColumns      []config.ColumnConfig // All columns (for context)
	isBlockedColumn bool                  // Whether this column uses blocked logic

	// Form inputs
	nameInput    textinput.Model
	columnType   string // "bql" (default) or "tree"
	queryInput   bqlinput.Model
	issueIDInput textinput.Model
	treeMode     string // "deps" (default) or "children" - for tree columns
	colorValue   string // Stores the current color hex value

	// Color picker overlay
	showColorPicker bool
	colorPicker     colorpicker.Model

	// Delete confirmation modal
	showDeleteModal bool
	deleteModal     modal.Model

	// Current field focus
	focused Field

	// Viewport dimensions
	width  int
	height int

	// Preview data - actual issues from the column for realistic preview
	previewIssues []beads.Issue
	executor      bql.BQLExecutor // BQL executor for preview queries

	// Tree preview for tree column type
	treeIssueMap map[string]*beads.Issue // Loaded issues for tree preview
	treeRootID   string                  // Root issue ID for tree preview

	// Validation error message (shown when save attempted with invalid state)
	validationError string
}

// New creates a new editor for the given column.
// executor is used to run BQL queries for live preview.
func New(columnIndex int, allColumns []config.ColumnConfig, executor bql.BQLExecutor) Model {
	cfg := allColumns[columnIndex]

	nameInput := textinput.New()
	nameInput.SetValue(cfg.Name)
	nameInput.CharLimit = 30
	nameInput.Prompt = ""
	nameInput.Focus()

	// Determine column type from config
	columnType := cfg.Type
	if columnType == "" {
		columnType = "bql" // Default to BQL
	}

	// Determine tree mode from config
	treeMode := cfg.TreeMode
	if treeMode == "" {
		treeMode = "deps" // Default to deps mode
	}

	queryInput := bqlinput.New()
	queryInput.SetValue(cfg.Query)
	queryInput.SetWidth(49)
	queryInput.SetPlaceholder("status = open and ready = true")

	issueIDInput := textinput.New()
	issueIDInput.SetValue(cfg.IssueID)
	issueIDInput.CharLimit = 50
	issueIDInput.Prompt = ""
	issueIDInput.Placeholder = "perles-123"

	// Initialize color picker
	picker := colorpicker.New().SetSelected(cfg.Color)

	// Determine if this is the "blocked" column based on query
	isBlocked := columnIndex == 0 && strings.Contains(cfg.Query, "blocked = true")

	m := Model{
		mode:            ModeEdit,
		columnIndex:     columnIndex,
		insertAfter:     -1, // Not used in edit mode
		original:        cfg,
		allColumns:      allColumns,
		isBlockedColumn: isBlocked,
		nameInput:       nameInput,
		columnType:      columnType,
		queryInput:      queryInput,
		issueIDInput:    issueIDInput,
		treeMode:        treeMode,
		colorValue:      cfg.Color,
		colorPicker:     picker,
		focused:         FieldName,
		executor:        executor,
	}

	// Initialize preview with current config
	m = m.updatePreview()
	return m
}

// NewForCreate creates an editor for adding a new column.
// insertAfterIndex specifies where to insert the new column (to the right of this index).
func NewForCreate(insertAfterIndex int, allColumns []config.ColumnConfig, executor bql.BQLExecutor) Model {
	// Default config for new column
	cfg := config.ColumnConfig{
		Name:  "",
		Type:  "bql", // Default to BQL
		Query: "status = open",
		Color: "#AABBCC", // Neutral default color
	}

	nameInput := textinput.New()
	nameInput.SetValue("")
	nameInput.CharLimit = 30
	nameInput.Prompt = ""
	nameInput.Focus()
	nameInput.Placeholder = "Column Name"

	queryInput := bqlinput.New()
	queryInput.SetValue(cfg.Query)
	queryInput.SetWidth(49)
	queryInput.SetPlaceholder("status = open and ready = true")

	issueIDInput := textinput.New()
	issueIDInput.SetValue("")
	issueIDInput.CharLimit = 50
	issueIDInput.Prompt = ""
	issueIDInput.Placeholder = "perles-123"

	// Initialize color picker with default color
	picker := colorpicker.New().SetSelected(cfg.Color)

	m := Model{
		mode:            ModeNew,
		insertAfter:     insertAfterIndex,
		columnIndex:     insertAfterIndex + 1, // Preview index
		original:        cfg,
		allColumns:      allColumns,
		isBlockedColumn: false, // New columns are never the blocked column
		nameInput:       nameInput,
		columnType:      "bql",
		queryInput:      queryInput,
		issueIDInput:    issueIDInput,
		treeMode:        "deps", // Default tree mode
		colorValue:      cfg.Color,
		colorPicker:     picker,
		focused:         FieldName,
		executor:        executor,
	}

	m = m.updatePreview()
	return m
}

// SetSize sets the viewport dimensions.
func (m Model) SetSize(width, height int) Model {
	m.width = width
	m.height = height

	// Calculate input width based on fixed left panel width
	// leftPanelWidth=75, sectionWidth=75-2=73, innerWidth=73-4 (borders)=69
	const leftPanelWidth = 75
	inputWidth := max(leftPanelWidth-6, 20) // Account for borders and padding
	m.nameInput.Width = inputWidth
	m.queryInput.SetWidth(inputWidth)
	m.issueIDInput.Width = inputWidth

	return m
}

// CurrentConfig builds a ColumnConfig from current form state.
func (m Model) CurrentConfig() config.ColumnConfig {
	cfg := config.ColumnConfig{
		Name:  m.nameInput.Value(),
		Type:  m.columnType,
		Color: m.colorValue,
	}

	if m.columnType == "tree" {
		cfg.IssueID = m.issueIDInput.Value()
		cfg.TreeMode = m.treeMode
	} else {
		cfg.Query = m.queryInput.Value()
	}

	return cfg
}

// updatePreview executes the BQL query and updates preview issues.
func (m Model) updatePreview() Model {
	cfg := m.CurrentConfig()

	if m.executor == nil {
		m.previewIssues = nil
		m.treeIssueMap = nil
		m.treeRootID = ""
		return m
	}

	if m.columnType == "tree" {
		// For tree columns, load the issue tree
		issueID := cfg.IssueID
		if issueID == "" {
			m.treeIssueMap = nil
			m.treeRootID = ""
			return m
		}

		// Execute expand query to get the tree
		query := fmt.Sprintf(`id = "%s" expand down depth *`, issueID)
		issues, err := m.executor.Execute(query)
		if err != nil {
			m.treeIssueMap = nil
			m.treeRootID = ""
			return m
		}

		// Build issue map for tree
		issueMap := make(map[string]*beads.Issue, len(issues))
		for i := range issues {
			issueMap[issues[i].ID] = &issues[i]
		}
		m.treeIssueMap = issueMap
		m.treeRootID = issueID
		m.previewIssues = nil // Clear BQL preview
		return m
	}

	// For BQL columns, execute the query directly
	issues, err := m.executor.Execute(cfg.Query)
	if err != nil {
		// Query invalid/incomplete - show empty preview
		m.previewIssues = nil
		return m
	}

	m.previewIssues = issues
	m.treeIssueMap = nil // Clear tree preview
	m.treeRootID = ""
	return m
}

// Update handles input messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	// Handle delete modal messages when modal is open
	if m.showDeleteModal {
		switch msg := msg.(type) {
		case modal.SubmitMsg:
			// User confirmed - proceed with deletion
			m.showDeleteModal = false
			return m, deleteCmd(m.columnIndex)
		case modal.CancelMsg:
			// User cancelled - return to editor
			m.showDeleteModal = false
			return m, nil
		case tea.KeyMsg:
			var cmd tea.Cmd
			m.deleteModal, cmd = m.deleteModal.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	// Handle colorpicker messages when picker is open
	if m.showColorPicker {
		switch msg := msg.(type) {
		case colorpicker.SelectMsg:
			m.colorValue = msg.Hex
			m.showColorPicker = false
			m = m.updatePreview()
			return m, nil
		case colorpicker.CancelMsg:
			m.showColorPicker = false
			return m, nil
		case tea.KeyMsg:
			var cmd tea.Cmd
			m.colorPicker, cmd = m.colorPicker.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "down":
			m.focused = m.nextField()
			m = m.updateFocus()
			return m, nil

		case "shift+tab", "up":
			m.focused = m.prevField()
			m = m.updateFocus()
			return m, nil

		case "ctrl+n":
			m.focused = m.nextField()
			m = m.updateFocus()
			return m, nil

		case "ctrl+p":
			m.focused = m.prevField()
			m = m.updateFocus()
			return m, nil

		case "j":
			// j only navigates when not in a text input field
			if !m.isTextInputField() {
				m.focused = m.nextField()
				m = m.updateFocus()
				return m, nil
			}

		case "k":
			// k only navigates when not in a text input field
			if !m.isTextInputField() {
				m.focused = m.prevField()
				m = m.updateFocus()
				return m, nil
			}

		case "left", "h":
			// Type selector toggle
			if m.focused == FieldType {
				if m.columnType == "tree" {
					m.columnType = "bql"
					m = m.updatePreview()
				}
				return m, nil
			}
			// Tree mode selector toggle
			if m.focused == FieldTreeMode {
				if m.treeMode == "child" {
					m.treeMode = "deps"
					m = m.updatePreview()
				}
				return m, nil
			}
			// Horizontal navigation between Save and Delete
			if m.focused == FieldDelete {
				m.focused = FieldSave
				return m, nil
			}

		case "right", "l":
			// Type selector toggle
			if m.focused == FieldType {
				if m.columnType == "bql" {
					m.columnType = "tree"
					m = m.updatePreview()
				}
				return m, nil
			}
			// Tree mode selector toggle
			if m.focused == FieldTreeMode {
				if m.treeMode == "deps" {
					m.treeMode = "child"
					m = m.updatePreview()
				}
				return m, nil
			}
			// Horizontal navigation between Save and Delete (Edit mode only)
			if m.focused == FieldSave && m.mode == ModeEdit {
				m.focused = FieldDelete
				return m, nil
			}

		case "enter":
			// Handle enter based on focused field
			switch m.focused {
			case FieldColor:
				// Open color picker overlay
				m.colorPicker = m.colorPicker.SetSelected(m.colorValue).SetSize(m.width, m.height)
				m.showColorPicker = true
				return m, nil
			case FieldSave:
				// Validate before save
				if err := m.validate(); err != "" {
					m.validationError = err
					return m, nil
				}
				// Branch based on mode
				if m.mode == ModeNew {
					return m, addCmd(m.insertAfter, m.CurrentConfig())
				}
				return m, saveCmd(m.columnIndex, m.CurrentConfig())
			case FieldDelete:
				// Delete column (only in edit mode)
				// Deleting last column is allowed - returns to empty state
				if m.mode == ModeEdit {
					// Open confirmation modal instead of deleting immediately
					columnName := m.nameInput.Value()
					if columnName == "" {
						columnName = m.original.Name
					}
					m.deleteModal = modal.New(modal.Config{
						Title:          "Delete Column",
						Message:        fmt.Sprintf("Delete column '%s'? This cannot be undone.", columnName),
						ConfirmVariant: modal.ButtonDanger,
					})
					m.deleteModal.SetSize(m.width, m.height)
					m.showDeleteModal = true
					return m, m.deleteModal.Init()
				}
			}
			return m, nil

		case "esc":
			return m, cancelCmd()
		}

		// Delegate to focused text input and update preview
		m = m.updateTextInput(msg)
		m = m.updatePreview() // Live preview update on every keystroke
		return m, nil
	}
	return m, nil
}

func (m Model) isTextInputField() bool {
	switch m.focused {
	case FieldName, FieldQuery, FieldIssueID:
		return true
	}
	return false
}

// nextField returns the next field in navigation order.
func (m Model) nextField() Field {
	next := m.focused + 1

	// Skip hidden fields based on column type
	if m.columnType == "tree" && next == FieldQuery {
		// For tree columns: skip Query, go to IssueID
		next = FieldIssueID
	} else if m.columnType != "tree" && next == FieldIssueID {
		// For BQL columns: skip IssueID and TreeMode, go to Save
		next = FieldSave
	} else if m.columnType != "tree" && next == FieldTreeMode {
		// For BQL columns: skip TreeMode, go to Save
		next = FieldSave
	}

	// Skip Delete field in New mode
	if m.mode == ModeNew && next == FieldDelete {
		return FieldName // Wrap around to top
	}

	if next > FieldDelete {
		return FieldName
	}
	return next
}

// prevField returns the previous field in navigation order.
func (m Model) prevField() Field {
	if m.focused == FieldName {
		// In New mode, wrap to Save (skip Delete)
		if m.mode == ModeNew {
			return FieldSave
		}
		return FieldDelete
	}

	prev := m.focused - 1

	// Skip hidden fields based on column type
	if m.columnType == "tree" && prev == FieldQuery {
		// For tree columns: skip Query, go to Color
		prev = FieldColor
	} else if m.columnType != "tree" && prev == FieldTreeMode {
		// For BQL columns: skip TreeMode and IssueID, go to Query
		prev = FieldQuery
	} else if m.columnType != "tree" && prev == FieldIssueID {
		// For BQL columns: skip IssueID, go to Query
		prev = FieldQuery
	}

	return prev
}

func (m Model) updateFocus() Model {
	// Blur all text inputs
	m.nameInput.Blur()
	m.queryInput.Blur()
	m.issueIDInput.Blur()

	// Focus the appropriate input
	switch m.focused {
	case FieldName:
		m.nameInput.Focus()
	case FieldQuery:
		m.queryInput.Focus()
	case FieldIssueID:
		m.issueIDInput.Focus()
	}
	return m
}

func (m Model) updateTextInput(msg tea.KeyMsg) Model {
	switch m.focused {
	case FieldName:
		m.nameInput, _ = m.nameInput.Update(msg)
		// Clear validation error when user types in name field
		m.validationError = ""
	case FieldQuery:
		m.queryInput, _ = m.queryInput.Update(msg)
	case FieldIssueID:
		m.issueIDInput, _ = m.issueIDInput.Update(msg)
	}
	return m
}

// validate checks the form for errors and returns an error message if invalid.
// Returns empty string if valid.
func (m Model) validate() string {
	// Name is required
	name := strings.TrimSpace(m.nameInput.Value())
	if name == "" {
		return "Column name is required"
	}

	if m.columnType == "tree" {
		// Issue ID is required for tree columns
		issueID := strings.TrimSpace(m.issueIDInput.Value())
		if issueID == "" {
			return "Issue ID is required for tree columns"
		}
	} else {
		// Query is required for BQL columns
		query := strings.TrimSpace(m.queryInput.Value())
		if query == "" {
			return "BQL query is required"
		}
	}

	return ""
}

// hasColorWarning returns true if color format is invalid but non-empty.
func (m Model) hasColorWarning() bool {
	if m.colorValue == "" {
		return false
	}
	return !hexColorRegex.MatchString(m.colorValue)
}

// View renders the full-screen editor with split layout.
func (m Model) View() string {
	// Fixed panel widths for consistent layout
	const (
		rightPanelWidth = 55 // Preview panel - fixed width
		leftPanelWidth  = 75 // Form panel - fixed width
	)

	// Total content width
	contentWidth := leftPanelWidth + 1 + rightPanelWidth // +1 for divider
	leftWidth := leftPanelWidth
	rightWidth := rightPanelWidth

	// Render both panels
	leftPanel := m.renderConfigForm(leftWidth)
	rightPanel := m.renderPreview(rightWidth)

	// Create vertical divider
	dividerStyle := lipgloss.NewStyle().
		Foreground(styles.BorderDefaultColor)
	dividerHeight := max(m.height-3, 1) // Account for header
	divider := dividerStyle.Render(strings.Repeat("│\n", dividerHeight))

	// Join panels horizontally
	content := lipgloss.JoinHorizontal(
		lipgloss.Top,
		leftPanel,
		divider,
		rightPanel,
	)

	// Header matches content width
	header := m.renderHeader(contentWidth)

	// Center content if terminal is wider than content
	result := lipgloss.JoinVertical(lipgloss.Left, header, content)
	if m.width > contentWidth {
		result = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Top, result)
	}

	// Render delete confirmation modal if open
	if m.showDeleteModal {
		return m.deleteModal.Overlay(result)
	}

	// Render colorpicker overlay if open
	if m.showColorPicker {
		return m.colorPicker.Overlay(result)
	}

	return result
}

func (m Model) renderHeader(width int) string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.TextPrimaryColor)

	helpStyle := lipgloss.NewStyle().
		Foreground(styles.TextMutedColor)

	var title string
	if m.mode == ModeNew {
		title = titleStyle.Render("New Column")
	} else {
		title = titleStyle.Render("Edit Column: " + m.original.Name)
	}
	help := helpStyle.Render("[Esc] Cancel  [↑/↓] Navigate")

	// Spread title and help across width
	gap := max(width-lipgloss.Width(title)-lipgloss.Width(help)-4, 1)

	headerStyle := lipgloss.NewStyle().
		BorderBottom(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(styles.BorderDefaultColor).
		Width(width).
		Padding(0, 1)

	return headerStyle.Render(title + strings.Repeat(" ", gap) + help)
}

func (m Model) renderConfigForm(width int) string {
	formStyle := lipgloss.NewStyle().
		Width(width).
		PaddingTop(1)

	sectionWidth := width - 2 // Account for form padding

	var sections []string

	// Section: Name (focus indicated by border highlight, no prefix needed)
	nameRows := []string{m.nameInput.View()}
	sections = append(sections, styles.RenderFormSection(nameRows, "Name", "", sectionWidth, m.focused == FieldName, styles.BorderHighlightFocusColor))

	// Section: Type selector (BQL Query or Tree View)
	typeRows := []string{m.renderTypeSelector()}
	sections = append(sections, styles.RenderFormSection(typeRows, "Type", "", sectionWidth, m.focused == FieldType, styles.BorderHighlightFocusColor))

	// Section: Color (shows swatch and hex, press Enter to open picker)
	colorSwatch := "   "
	if m.colorValue != "" {
		colorSwatch = lipgloss.NewStyle().
			Background(lipgloss.Color(m.colorValue)).
			Render("   ")
	}
	colorHint := lipgloss.NewStyle().Foreground(styles.TextMutedColor).Render(" [Enter to change]")
	colorRows := []string{colorSwatch + " " + m.colorValue + colorHint}
	sections = append(sections, styles.RenderFormSection(colorRows, "Color", "", sectionWidth, m.focused == FieldColor, styles.BorderHighlightFocusColor))

	// Section: BQL Query or Issue ID + Tree Mode (conditional based on type)
	if m.columnType == "tree" {
		// Show Issue ID input for tree columns
		issueIDRows := []string{m.issueIDInput.View()}
		sections = append(sections, styles.RenderFormSection(issueIDRows, "Root Issue ID", "", sectionWidth, m.focused == FieldIssueID, styles.BorderHighlightFocusColor))

		// Show Tree Mode selector for tree columns
		treeModeRows := []string{m.renderTreeModeSelector()}
		sections = append(sections, styles.RenderFormSection(treeModeRows, "Tree Mode", "", sectionWidth, m.focused == FieldTreeMode, styles.BorderHighlightFocusColor))
	} else {
		// Show BQL Query input for BQL columns
		queryRows := m.renderQuerySection(sectionWidth - 4) // Account for borders
		sections = append(sections, styles.RenderFormSection(queryRows, "BQL Query", "", sectionWidth, m.focused == FieldQuery, styles.BorderHighlightFocusColor))
	}

	// Actions (Save and Delete on same line) - no border, standalone buttons
	// Delete is always enabled in edit mode - last column deletion returns to empty state
	deleteEnabled := true
	actionRow := m.renderActionRowHorizontal(deleteEnabled)
	sections = append(sections, actionRow)

	// Section: Query Help (expanded with all fields and operators)
	// Hide help section on small screens (threshold ~30 lines)
	// No border - just inline help text with divider and title
	const helpHeightThreshold = 50
	if m.height >= helpHeightThreshold {
		// Divider line
		dividerStyle := lipgloss.NewStyle().Foreground(styles.BorderDefaultColor)
		divider := dividerStyle.Render(strings.Repeat("─", sectionWidth))

		// Title
		titleStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.TextSecondaryColor)
		title := titleStyle.Render("BQL Query Help")

		helpRows := m.buildBQLHelpRows()
		helpContent := divider + "\n\n" + title + "\n\n" + strings.Join(helpRows, "\n")
		sections = append(sections, helpContent)
	}

	// Warnings at the bottom (no border, just inline)
	if m.hasColorWarning() || m.validationError != "" {
		var warnings []string
		warningStyle := lipgloss.NewStyle().Foreground(styles.StatusErrorColor)
		if m.validationError != "" {
			warnings = append(warnings, warningStyle.Bold(true).Render("⚠ "+m.validationError))
		}
		if m.hasColorWarning() {
			warnings = append(warnings, warningStyle.Render("⚠ Invalid color format (expected #RGB or #RRGGBB)"))
		}
		sections = append(sections, strings.Join(warnings, "\n"))
	}

	return formStyle.Render(strings.Join(sections, "\n\n"))
}

// renderQuerySection renders the BQL query input with syntax highlighting.
// The bqlinput component handles highlighting, cursor, and wrapping internally.
// Focus is indicated by border highlight, no prefix needed.
func (m Model) renderQuerySection(maxWidth int) []string {
	// Set width for wrapping
	m.queryInput.SetWidth(maxWidth)

	// Get wrapped view from bqlinput (handles highlighting, cursor, and wrapping)
	wrapped := m.queryInput.View()
	lines := strings.Split(wrapped, "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		return []string{""}
	}

	return lines
}

// renderTypeSelector renders the type toggle selector.
func (m Model) renderTypeSelector() string {
	selectedStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.TextPrimaryColor)
	unselectedStyle := lipgloss.NewStyle().
		Foreground(styles.TextMutedColor)
	hintStyle := lipgloss.NewStyle().
		Foreground(styles.TextMutedColor)

	var bqlLabel, treeLabel string
	if m.columnType == "bql" {
		bqlLabel = selectedStyle.Render("● BQL Query")
		treeLabel = unselectedStyle.Render("○ Tree View")
	} else {
		bqlLabel = unselectedStyle.Render("○ BQL Query")
		treeLabel = selectedStyle.Render("● Tree View")
	}

	hint := hintStyle.Render(" [←/→ to switch]")
	return bqlLabel + "    " + treeLabel + hint
}

// renderTreeModeSelector renders the tree mode toggle selector.
func (m Model) renderTreeModeSelector() string {
	selectedStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.TextPrimaryColor)
	unselectedStyle := lipgloss.NewStyle().
		Foreground(styles.TextMutedColor)
	hintStyle := lipgloss.NewStyle().
		Foreground(styles.TextMutedColor)

	var depsLabel, childLabel string
	if m.treeMode == "deps" || m.treeMode == "" {
		depsLabel = selectedStyle.Render("● Dependencies")
		childLabel = unselectedStyle.Render("○ Parent-Child")
	} else {
		depsLabel = unselectedStyle.Render("○ Dependencies")
		childLabel = selectedStyle.Render("● Parent-Child")
	}

	hint := hintStyle.Render(" [←/→ to switch]")
	return depsLabel + "    " + childLabel + hint
}

func (m Model) renderActionRowHorizontal(deleteEnabled bool) string {
	// Base button style with padding
	buttonStyle := lipgloss.NewStyle().
		Padding(0, 2).
		Bold(true)

	// Save button - dark blue, lighter when focused
	saveStyle := buttonStyle.
		Foreground(styles.ButtonTextColor).
		Background(styles.ButtonPrimaryBgColor)
	if m.focused == FieldSave {
		saveStyle = saveStyle.
			Background(styles.ButtonPrimaryFocusBgColor).
			Underline(true).
			UnderlineSpaces(true)
	}
	saveBtn := saveStyle.Render("Save")

	// Hide delete button in New mode
	if m.mode == ModeNew {
		return saveBtn
	}

	// Delete button (only shown in Edit mode)
	var deleteBtn string
	if !deleteEnabled {
		deleteStyle := buttonStyle.
			Foreground(styles.TextMutedColor).
			Background(styles.ButtonDisabledBgColor)
		deleteBtn = deleteStyle.Render("Delete")
	} else {
		deleteStyle := buttonStyle.
			Foreground(styles.ButtonTextColor).
			Background(styles.ButtonDangerBgColor)
		if m.focused == FieldDelete {
			deleteStyle = deleteStyle.
				Background(styles.ButtonDangerFocusBgColor).
				Underline(true).
				UnderlineSpaces(true)
		}
		deleteBtn = deleteStyle.Render("Delete")
	}

	return saveBtn + "  " + deleteBtn
}

func (m Model) renderPreview(width int) string {
	previewStyle := lipgloss.NewStyle().
		Width(width).
		Padding(1, 2)

	// Header
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.TextMutedColor)

	countStyle := lipgloss.NewStyle().
		Foreground(styles.TextMutedColor).
		Italic(true)

	header := headerStyle.Render("Live Preview")

	// Calculate column size
	colWidth := width - 4
	colHeight := m.height - 10
	if colWidth < 10 {
		colWidth = 10
	}
	if colHeight < 5 {
		colHeight = 5
	}
	if colHeight > 20 {
		colHeight = 20
	}

	cfg := m.CurrentConfig()
	var columnView string
	var countInfo string

	if m.columnType == "tree" {
		// Render tree preview
		if m.treeIssueMap != nil && m.treeRootID != "" {
			// Convert tree mode string to tree.TreeMode
			// Config uses "child" but tree model uses ModeChildren
			treeMode := tree.ModeDeps
			if m.treeMode == "child" {
				treeMode = tree.ModeChildren
			}
			treeModel := tree.New(m.treeRootID, m.treeIssueMap, tree.DirectionDown, treeMode, shared.RealClock{})
			// Account for border wrapper (2 chars for left/right border, 2 for top/bottom)
			treeModel.SetSize(colWidth-2, colHeight-2)
			countInfo = countStyle.Render(fmt.Sprintf("Tree with %d nodes", len(m.treeIssueMap)))
			columnView = styles.RenderWithTitleBorder(
				treeModel.View(),
				cfg.Name,
				"",
				colWidth,
				colHeight,
				true,
				lipgloss.Color(cfg.Color),
				lipgloss.Color(cfg.Color),
			)
		} else {
			// No tree data yet
			emptyMsg := lipgloss.NewStyle().
				Foreground(styles.TextMutedColor).
				Italic(true).
				Render("Enter an issue ID to preview the tree")
			countInfo = countStyle.Render("No tree data")
			columnView = styles.RenderWithTitleBorder(
				emptyMsg,
				cfg.Name,
				"",
				colWidth,
				colHeight,
				true,
				lipgloss.Color(cfg.Color),
				lipgloss.Color(cfg.Color),
			)
		}
	} else {
		// Render BQL column preview (existing logic)
		countInfo = countStyle.Render(fmt.Sprintf("%d issues match current filters", len(m.previewIssues)))

		// Create a preview column using actual Column component
		previewCol := board.NewColumn(cfg.Name)
		if cfg.Color != "" {
			previewCol = previewCol.SetColor(lipgloss.Color(cfg.Color))
		}
		previewCol = previewCol.SetItems(m.previewIssues)
		previewCol = previewCol.SetFocused(true).(board.Column)
		previewCol = previewCol.SetSize(colWidth, colHeight).(board.Column)

		columnView = styles.RenderWithTitleBorder(
			previewCol.View(),
			previewCol.Title(),
			"",
			colWidth,
			colHeight,
			true, // focused
			previewCol.Color(),
			previewCol.Color(),
		)
	}

	return previewStyle.Render(header + "\n" + countInfo + "\n\n" + columnView)
}

// Command functions
func saveCmd(index int, cfg config.ColumnConfig) tea.Cmd {
	return func() tea.Msg {
		return SaveMsg{ColumnIndex: index, Config: cfg}
	}
}

func cancelCmd() tea.Cmd {
	return func() tea.Msg {
		return CancelMsg{}
	}
}

func deleteCmd(index int) tea.Cmd {
	return func() tea.Msg {
		return DeleteMsg{ColumnIndex: index}
	}
}

func addCmd(insertAfter int, cfg config.ColumnConfig) tea.Cmd {
	return func() tea.Msg {
		return AddMsg{InsertAfterIndex: insertAfter, Config: cfg}
	}
}

// Focused returns the currently focused field (for testing).
func (m Model) Focused() Field {
	return m.focused
}

// PreviewIssues returns the preview issues (for testing).
func (m Model) PreviewIssues() []beads.Issue {
	return m.previewIssues
}

// AllColumns returns all columns (for testing delete disabled check).
func (m Model) AllColumns() []config.ColumnConfig {
	return m.allColumns
}

// UpdatePreview updates the preview (exported for testing).
func (m Model) UpdatePreview() Model {
	return m.updatePreview()
}

// ValidationError returns the current validation error (for testing).
func (m Model) ValidationError() string {
	return m.validationError
}

// HasColorWarning returns true if color format is invalid (exported for testing).
func (m Model) HasColorWarning() bool {
	return m.hasColorWarning()
}

// Mode returns the editor mode (for testing).
func (m Model) Mode() Mode {
	return m.mode
}

// InsertAfter returns the insert-after index (for testing).
func (m Model) InsertAfter() int {
	return m.insertAfter
}

// NameInput returns the name input (for testing).
func (m Model) NameInput() textinput.Model {
	return m.nameInput
}

// QueryInput returns the query input (for testing).
func (m Model) QueryInput() bqlinput.Model {
	return m.queryInput
}

// ColumnType returns the current column type (for testing).
func (m Model) ColumnType() string {
	return m.columnType
}

// IssueIDInput returns the issue ID input (for testing).
func (m Model) IssueIDInput() textinput.Model {
	return m.issueIDInput
}

// TreeMode returns the current tree mode (for testing).
func (m Model) TreeMode() string {
	return m.treeMode
}

// ShowColorPicker returns whether the color picker is visible (for testing).
func (m Model) ShowColorPicker() bool {
	return m.showColorPicker
}

// ColorValue returns the current color value (for testing).
func (m Model) ColorValue() string {
	return m.colorValue
}

// ShowDeleteModal returns whether the delete modal is visible (for testing).
func (m Model) ShowDeleteModal() bool {
	return m.showDeleteModal
}

// buildBQLHelpRows builds the BQL syntax help rows using shared help data.
// Returns a two-column layout (Fields | Operators) with Examples below.
func (m Model) buildBQLHelpRows() []string {
	labelStyle := lipgloss.NewStyle().Foreground(styles.TextSecondaryColor).Width(10)
	valueStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.TextSecondaryColor)
	columnStyle := lipgloss.NewStyle().MarginRight(4)

	// Operator label needs more width for symbols like "in (a,b,c)"
	opLabelStyle := lipgloss.NewStyle().Foreground(styles.TextSecondaryColor).Width(8)

	// Fields column
	var fieldsCol strings.Builder
	fieldsCol.WriteString(headerStyle.Render("BQL Fields"))
	fieldsCol.WriteString("\n")
	for _, f := range help.BQLFields() {
		fieldsCol.WriteString(labelStyle.Render(f.Name) + valueStyle.Render(f.Values) + "\n")
	}

	// Operators column - use compact symbols for better layout
	var opsCol strings.Builder
	opsCol.WriteString(headerStyle.Render("BQL Operators"))
	opsCol.WriteString("\n")
	// Use compact operator display to fit in two-column layout
	compactOps := []struct{ symbol, desc string }{
		{"=  !=", "equality"},
		{"<  >", "comparison"},
		{"~  !~", "contains"},
		{"in", "match any"},
		{"and or", "logical"},
		{"not", "negation"},
	}
	for _, op := range compactOps {
		opsCol.WriteString(opLabelStyle.Render(op.symbol) + valueStyle.Render(op.desc) + "\n")
	}

	// Join columns horizontally
	columns := lipgloss.JoinHorizontal(
		lipgloss.Top,
		columnStyle.Render(fieldsCol.String()),
		opsCol.String(),
	)

	// Examples section below columns
	var examplesCol strings.Builder
	examplesCol.WriteString("\n")
	examplesCol.WriteString(headerStyle.Render("Examples"))
	examplesCol.WriteString("\n")
	for _, ex := range help.BQLExamples() {
		examplesCol.WriteString(valueStyle.Render(ex) + "\n")
	}

	// Combine columns + examples into single string, then split into rows
	combined := columns + examplesCol.String()
	return strings.Split(combined, "\n")
}
