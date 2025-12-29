// Package playground provides a component showcase and theme token viewer.
package playground

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zjrosen/perles/internal/beads"
	"github.com/zjrosen/perles/internal/bql"
	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/pubsub"
	"github.com/zjrosen/perles/internal/ui/shared/chainart"
	"github.com/zjrosen/perles/internal/ui/shared/colorpicker"
	"github.com/zjrosen/perles/internal/ui/shared/formmodal"
	"github.com/zjrosen/perles/internal/ui/shared/issuebadge"
	"github.com/zjrosen/perles/internal/ui/shared/logoverlay"
	"github.com/zjrosen/perles/internal/ui/shared/markdown"
	"github.com/zjrosen/perles/internal/ui/shared/modal"
	"github.com/zjrosen/perles/internal/ui/shared/panes"
	"github.com/zjrosen/perles/internal/ui/shared/picker"
	"github.com/zjrosen/perles/internal/ui/shared/toaster"
	"github.com/zjrosen/perles/internal/ui/shared/vimtextarea"
	"github.com/zjrosen/perles/internal/ui/styles"
)

// ComponentDemo represents a demo-able component in the playground.
type ComponentDemo struct {
	Name        string
	Description string
	Create      func(width, height int) DemoModel
}

// DemoModel is the interface that all demo models must implement.
type DemoModel interface {
	Update(msg tea.Msg) (DemoModel, tea.Cmd, string) // Returns model, cmd, and last action string
	View() string
	SetSize(width, height int) DemoModel
	Reset() DemoModel
	NeedsEscKey() bool // Returns true if the demo needs the Esc key (e.g., vim mode switching)
}

// GetComponentDemos returns the registry of all component demos.
func GetComponentDemos() []ComponentDemo {
	return []ComponentDemo{
		{
			Name:        "vimtextarea",
			Description: "Vim-enabled text input with undo/redo",
			Create:      createVimtextareaDemo,
		},
		{
			Name:        "modal",
			Description: "Confirmation and input dialogs",
			Create:      createModalDemo,
		},
		{
			Name:        "formmodal",
			Description: "Multi-field form modal",
			Create:      createFormmodalDemo,
		},
		{
			Name:        "panes",
			Description: "Bordered pane component",
			Create:      createPanesDemo,
		},
		{
			Name:        "scrollpane",
			Description: "Scrollable pane with viewport",
			Create:      createScrollablePaneDemo,
		},
		{
			Name:        "colorpicker",
			Description: "Visual color selection with presets",
			Create:      createColorpickerDemo,
		},
		{
			Name:        "picker",
			Description: "Generic option picker component",
			Create:      createPickerDemo,
		},
		{
			Name:        "toaster",
			Description: "Toast notification component",
			Create:      createToasterDemo,
		},
		{
			Name:        "logoverlay",
			Description: "Log viewer with level filtering",
			Create:      createLogOverlayDemo,
		},
		{
			Name:        "markdown",
			Description: "Glamour markdown rendering",
			Create:      createMarkdownDemo,
		},
		{
			Name:        "chainart",
			Description: "ASCII chain progress art",
			Create:      createChainartDemo,
		},
		{
			Name:        "issuebadge",
			Description: "Issue type/priority/id badge component",
			Create:      createIssueBadgeDemo,
		},
		{
			Name:        "Theme Tokens",
			Description: "All theme color tokens",
			Create:      createThemeTokensDemo,
		},
	}
}

// PickerDemoModel wraps the picker component for demonstration.
type PickerDemoModel struct {
	picker     picker.Model
	lastAction string
	width      int
	height     int
}

func createPickerDemo(width, height int) DemoModel {
	options := []picker.Option{
		{Label: "Option 1", Value: "opt1"},
		{Label: "Option 2", Value: "opt2"},
		{Label: "Option 3", Value: "opt3"},
		{Label: "Option 4", Value: "opt4"},
		{Label: "Option 5", Value: "opt5"},
	}
	p := picker.NewWithConfig(picker.Config{
		Title:   "Select an Option",
		Options: options,
	}).SetSize(width, height)

	return &PickerDemoModel{
		picker: p,
		width:  width,
		height: height,
	}
}

func (m *PickerDemoModel) Update(msg tea.Msg) (DemoModel, tea.Cmd, string) {
	// Handle picker result messages
	switch result := msg.(type) {
	case picker.SelectMsg:
		m.lastAction = "Selected: " + result.Option.Label
		m.picker = m.picker.SetSelected(0)
		return m, nil, m.lastAction
	case picker.CancelMsg:
		m.lastAction = "Cancelled"
		return m, nil, m.lastAction
	}

	var cmd tea.Cmd
	m.picker, cmd = m.picker.Update(msg)
	return m, cmd, ""
}

func (m *PickerDemoModel) View() string {
	return m.picker.View()
}

func (m *PickerDemoModel) SetSize(width, height int) DemoModel {
	m.width = width
	m.height = height
	m.picker = m.picker.SetSize(width, height)
	return m
}

func (m *PickerDemoModel) Reset() DemoModel {
	return createPickerDemo(m.width, m.height)
}

func (m *PickerDemoModel) NeedsEscKey() bool { return false }

// ToasterDemoModel wraps the toaster component for demonstration.
type ToasterDemoModel struct {
	toaster toaster.Model
	width   int
	height  int
}

func createToasterDemo(width, height int) DemoModel {
	t := toaster.New().SetSize(width, height)
	return &ToasterDemoModel{
		toaster: t,
		width:   width,
		height:  height,
	}
}

func (m *ToasterDemoModel) Update(msg tea.Msg) (DemoModel, tea.Cmd, string) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "s":
			m.toaster = m.toaster.Show("Success message!", toaster.StyleSuccess)
			return m, nil, ""
		case "e":
			m.toaster = m.toaster.Show("Error occurred!", toaster.StyleError)
			return m, nil, ""
		case "i":
			m.toaster = m.toaster.Show("Information message", toaster.StyleInfo)
			return m, nil, ""
		case "w":
			m.toaster = m.toaster.Show("Warning message!", toaster.StyleWarn)
			return m, nil, ""
		case "d":
			m.toaster = m.toaster.Hide()
			return m, nil, ""
		}
	case toaster.DismissMsg:
		m.toaster = m.toaster.Hide()
		return m, nil, ""
	}
	return m, nil, ""
}

func (m *ToasterDemoModel) View() string {
	var sb strings.Builder

	instructionStyle := lipgloss.NewStyle().Foreground(styles.TextSecondaryColor)
	sb.WriteString(instructionStyle.Render("Press keys to trigger toasts:"))
	sb.WriteString("\n\n")

	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.StatusInProgressColor)
	descStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)

	sb.WriteString("  " + keyStyle.Render("s") + " " + descStyle.Render("Success toast"))
	sb.WriteString("\n")
	sb.WriteString("  " + keyStyle.Render("e") + " " + descStyle.Render("Error toast"))
	sb.WriteString("\n")
	sb.WriteString("  " + keyStyle.Render("i") + " " + descStyle.Render("Info toast"))
	sb.WriteString("\n")
	sb.WriteString("  " + keyStyle.Render("w") + " " + descStyle.Render("Warning toast"))
	sb.WriteString("\n")
	sb.WriteString("  " + keyStyle.Render("d") + " " + descStyle.Render("Dismiss toast"))

	// If toast is visible, show it at bottom
	if m.toaster.Visible() {
		// Create a frame with enough height to position toast at bottom
		content := sb.String()
		return m.toaster.Overlay(content, m.width, m.height)
	}

	return sb.String()
}

func (m *ToasterDemoModel) SetSize(width, height int) DemoModel {
	m.width = width
	m.height = height
	m.toaster = m.toaster.SetSize(width, height)
	return m
}

func (m *ToasterDemoModel) Reset() DemoModel {
	return createToasterDemo(m.width, m.height)
}

func (m *ToasterDemoModel) NeedsEscKey() bool { return false }

// PanesDemoModel wraps the panes component for demonstration.
type PanesDemoModel struct {
	focused    bool
	lastAction string
	width      int
	height     int
}

func createPanesDemo(width, height int) DemoModel {
	return &PanesDemoModel{
		focused:    true,
		lastAction: "Press f to toggle focus",
		width:      width,
		height:     height,
	}
}

func (m *PanesDemoModel) Update(msg tea.Msg) (DemoModel, tea.Cmd, string) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "f":
			m.focused = !m.focused
			if m.focused {
				m.lastAction = "Pane focused"
			} else {
				m.lastAction = "Pane unfocused"
			}
			return m, nil, m.lastAction
		}
	}
	return m, nil, ""
}

func (m *PanesDemoModel) View() string {
	// Sample content for the pane
	content := lipgloss.NewStyle().Foreground(styles.TextPrimaryColor).Render("Sample pane content") + "\n" +
		lipgloss.NewStyle().Foreground(styles.TextSecondaryColor).Render("This demonstrates the BorderedPane component.") + "\n\n" +
		lipgloss.NewStyle().Foreground(styles.TextMutedColor).Render("Press 'f' to toggle focus state.")

	// Calculate pane dimensions (leave some margin)
	paneWidth := min(m.width-4, 60)
	paneHeight := min(m.height-4, 12)

	return panes.BorderedPane(panes.BorderConfig{
		Content:            content,
		Width:              paneWidth,
		Height:             paneHeight,
		TopLeft:            "Top Left",
		TopRight:           "Top Right",
		BottomLeft:         "Bottom Left",
		BottomRight:        "Bottom Right",
		Focused:            m.focused,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
	})
}

func (m *PanesDemoModel) SetSize(width, height int) DemoModel {
	m.width = width
	m.height = height
	return m
}

func (m *PanesDemoModel) Reset() DemoModel {
	return createPanesDemo(m.width, m.height)
}

func (m *PanesDemoModel) NeedsEscKey() bool { return false }

// ColorpickerDemoModel wraps the colorpicker component for demonstration.
type ColorpickerDemoModel struct {
	colorpicker colorpicker.Model
	lastAction  string
	width       int
	height      int
}

func createColorpickerDemo(width, height int) DemoModel {
	cp := colorpicker.New().SetSize(width, height)
	return &ColorpickerDemoModel{
		colorpicker: cp,
		lastAction:  "Navigate with h/j/k/l, Enter to select",
		width:       width,
		height:      height,
	}
}

func (m *ColorpickerDemoModel) Update(msg tea.Msg) (DemoModel, tea.Cmd, string) {
	// Handle colorpicker result messages
	switch result := msg.(type) {
	case colorpicker.SelectMsg:
		m.lastAction = "Selected color: " + result.Hex
		m.colorpicker = m.colorpicker.SetSelected(result.Hex)
		return m, nil, m.lastAction
	case colorpicker.CancelMsg:
		m.lastAction = "Cancelled"
		return m, nil, m.lastAction
	}

	var cmd tea.Cmd
	m.colorpicker, cmd = m.colorpicker.Update(msg)
	return m, cmd, ""
}

func (m *ColorpickerDemoModel) View() string {
	return m.colorpicker.View()
}

func (m *ColorpickerDemoModel) SetSize(width, height int) DemoModel {
	m.width = width
	m.height = height
	m.colorpicker = m.colorpicker.SetSize(width, height)
	return m
}

func (m *ColorpickerDemoModel) Reset() DemoModel {
	return createColorpickerDemo(m.width, m.height)
}

func (m *ColorpickerDemoModel) NeedsEscKey() bool { return false }

// ModalDemoModel wraps the modal component for demonstration.
type ModalDemoModel struct {
	modal       *modal.Model
	modalType   string // "confirm", "danger", "input"
	lastAction  string
	width       int
	height      int
	showingMenu bool
}

func createModalDemo(width, height int) DemoModel {
	return &ModalDemoModel{
		modal:       nil,
		lastAction:  "Press c/d/i to show different modals",
		width:       width,
		height:      height,
		showingMenu: true,
	}
}

func (m *ModalDemoModel) Update(msg tea.Msg) (DemoModel, tea.Cmd, string) {
	// Handle modal result messages
	switch result := msg.(type) {
	case modal.SubmitMsg:
		if len(result.Values) > 0 {
			if name, ok := result.Values["name"]; ok {
				m.lastAction = "Submitted: " + name
			} else {
				m.lastAction = "Submitted with values"
			}
		} else {
			m.lastAction = "Confirmed"
		}
		m.modal = nil
		m.showingMenu = true
		return m, nil, m.lastAction
	case modal.CancelMsg:
		m.lastAction = "Cancelled"
		m.modal = nil
		m.showingMenu = true
		return m, nil, m.lastAction
	}

	// If showing the menu, handle menu keys
	if m.showingMenu {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "c":
				// Show confirmation modal
				mdl := modal.New(modal.Config{
					Title:   "Confirm Action",
					Message: "Are you sure you want to proceed with this action?",
				})
				mdl.SetSize(m.width, m.height)
				m.modal = &mdl
				m.modalType = "confirm"
				m.showingMenu = false
				m.lastAction = "Showing: Confirmation modal"
				return m, mdl.Init(), m.lastAction
			case "d":
				// Show danger modal
				mdl := modal.New(modal.Config{
					Title:          "Delete Item",
					Message:        "This action cannot be undone. Delete this item?",
					ConfirmVariant: modal.ButtonDanger,
				})
				mdl.SetSize(m.width, m.height)
				m.modal = &mdl
				m.modalType = "danger"
				m.showingMenu = false
				m.lastAction = "Showing: Danger modal"
				return m, mdl.Init(), m.lastAction
			case "i":
				// Show input modal
				mdl := modal.New(modal.Config{
					Title:   "Create New Item",
					Message: "Enter a name for the new item:",
					Inputs: []modal.InputConfig{
						{Key: "name", Label: "Name", Placeholder: "Enter name..."},
					},
				})
				mdl.SetSize(m.width, m.height)
				m.modal = &mdl
				m.modalType = "input"
				m.showingMenu = false
				m.lastAction = "Showing: Input modal"
				return m, mdl.Init(), m.lastAction
			}
		}
		return m, nil, ""
	}

	// Modal is showing, forward messages
	if m.modal != nil {
		var cmd tea.Cmd
		newModal, cmd := m.modal.Update(msg)
		m.modal = &newModal
		return m, cmd, ""
	}

	return m, nil, ""
}

func (m *ModalDemoModel) View() string {
	var sb strings.Builder

	instructionStyle := lipgloss.NewStyle().Foreground(styles.TextSecondaryColor)
	sb.WriteString(instructionStyle.Render("Press keys to trigger different modal types:"))
	sb.WriteString("\n\n")

	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.StatusInProgressColor)
	descStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)

	sb.WriteString("  " + keyStyle.Render("c") + " " + descStyle.Render("Confirmation modal (primary)"))
	sb.WriteString("\n")
	sb.WriteString("  " + keyStyle.Render("d") + " " + descStyle.Render("Danger modal (destructive)"))
	sb.WriteString("\n")
	sb.WriteString("  " + keyStyle.Render("i") + " " + descStyle.Render("Input modal (with text field)"))

	menuContent := sb.String()

	// If modal is showing, overlay it
	if m.modal != nil {
		return m.modal.Overlay(menuContent)
	}

	return menuContent
}

func (m *ModalDemoModel) SetSize(width, height int) DemoModel {
	m.width = width
	m.height = height
	if m.modal != nil {
		m.modal.SetSize(width, height)
	}
	return m
}

func (m *ModalDemoModel) Reset() DemoModel {
	return createModalDemo(m.width, m.height)
}

func (m *ModalDemoModel) NeedsEscKey() bool { return false }

// renderDemoArea renders the main demo area with the selected component.
// The width and height parameters are reserved for future responsive layout.
func renderDemoArea(demo DemoModel, lastAction string, _, _ int) string {
	var sb strings.Builder

	// Demo content with 1 char padding
	if demo != nil {
		// Add left padding to each line
		lines := strings.Split(demo.View(), "\n")
		for i, line := range lines {
			if i > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(" " + line)
		}
	}

	// Last action indicator
	if lastAction != "" {
		sb.WriteString("\n\n")
		actionStyle := lipgloss.NewStyle().
			Foreground(styles.TextMutedColor).
			Italic(true)
		sb.WriteString(" " + actionStyle.Render("Last action: "+lastAction))
	}

	return sb.String()
}

// =============================================================================
// Phase 3: VimTextarea Demo
// =============================================================================

// VimtextareaDemoModel wraps the vimtextarea component for demonstration.
type VimtextareaDemoModel struct {
	textarea   vimtextarea.Model
	lastAction string
	width      int
	height     int
}

func createVimtextareaDemo(width, height int) DemoModel {
	ta := vimtextarea.New(vimtextarea.Config{
		VimEnabled:  true,
		DefaultMode: vimtextarea.ModeNormal,
		Placeholder: "Type here with vim keybindings...",
		MaxHeight:   min(height-4, 15),
	})
	ta.SetLexer(bql.NewSyntaxLexer())
	ta.SetSize(min(width-4, 60), min(height-4, 15))
	ta.Focus()

	return &VimtextareaDemoModel{
		textarea:   ta,
		lastAction: "Press i to enter Insert mode",
		width:      width,
		height:     height,
	}
}

func (m *VimtextareaDemoModel) Update(msg tea.Msg) (DemoModel, tea.Cmd, string) {
	// Handle vimtextarea result messages
	switch result := msg.(type) {
	case vimtextarea.ModeChangeMsg:
		m.lastAction = fmt.Sprintf("Mode: %s → %s", result.Previous, result.Mode)
		return m, nil, m.lastAction
	case vimtextarea.SubmitMsg:
		m.lastAction = "Submitted content"
		return m, nil, m.lastAction
	}

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd, ""
}

func (m *VimtextareaDemoModel) View() string {
	// Wrap textarea in a bordered pane with mode indicator in bottom left
	return panes.BorderedPane(panes.BorderConfig{
		Content:     m.textarea.View(),
		Width:       m.width,
		Height:      m.height,
		BottomLeft:  m.textarea.ModeIndicator(),
		Focused:     true,
		BorderColor: styles.BorderDefaultColor,
	})
}

func (m *VimtextareaDemoModel) SetSize(width, height int) DemoModel {
	m.width = width
	m.height = height
	m.textarea.SetSize(min(width-4, 60), min(height-4, 15))
	return m
}

func (m *VimtextareaDemoModel) Reset() DemoModel {
	return createVimtextareaDemo(m.width, m.height)
}

func (m *VimtextareaDemoModel) NeedsEscKey() bool {
	return true // vimtextarea needs Esc for mode switching
}

// =============================================================================
// Phase 3: Formmodal Demo
// =============================================================================

// FormmodalDemoModel wraps the formmodal component for demonstration.
type FormmodalDemoModel struct {
	formmodal    *formmodal.Model
	width        int
	height       int
	showingMenu  bool
	lastResult   string // Stores the last form submission result
	wasCancelled bool   // Whether the last action was a cancel
}

func createFormmodalDemo(width, height int) DemoModel {
	return &FormmodalDemoModel{
		formmodal:   nil,
		width:       width,
		height:      height,
		showingMenu: true,
	}
}

func (m *FormmodalDemoModel) Update(msg tea.Msg) (DemoModel, tea.Cmd, string) {
	// Handle formmodal result messages
	switch result := msg.(type) {
	case formmodal.SubmitMsg:
		// Format field values for display
		var parts []string
		if name, ok := result.Values["name"].(string); ok && name != "" {
			parts = append(parts, "Name: "+name)
		}
		if color, ok := result.Values["color"].(string); ok {
			parts = append(parts, "Color: "+color)
		}
		if priorities, ok := result.Values["priority"].([]string); ok && len(priorities) > 0 {
			parts = append(parts, "Priority: "+strings.Join(priorities, ", "))
		}
		if category, ok := result.Values["category"].(string); ok {
			parts = append(parts, "Category: "+category)
		}
		if enabled, ok := result.Values["enabled"].(string); ok {
			parts = append(parts, "Status: "+enabled)
		}
		m.lastResult = strings.Join(parts, "\n")
		m.wasCancelled = false
		m.formmodal = nil
		m.showingMenu = true
		return m, nil, "Created item"
	case formmodal.CancelMsg:
		m.lastResult = ""
		m.wasCancelled = true
		m.formmodal = nil
		m.showingMenu = true
		return m, nil, "Cancelled"
	}

	// If showing the menu, handle menu keys
	if m.showingMenu {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			if msg.String() == "enter" {
				// Show form modal with all field types
				fm := formmodal.New(formmodal.FormConfig{
					Title: "Create Item",
					Fields: []formmodal.FieldConfig{
						{
							Key:         "name",
							Type:        formmodal.FieldTypeText,
							Label:       "Name",
							Hint:        "required",
							Placeholder: "Enter item name...",
						},
						{
							Key:          "color",
							Type:         formmodal.FieldTypeColor,
							Label:        "Accent Color",
							InitialColor: "#73F59F",
						},
						{
							Key:         "priority",
							Type:        formmodal.FieldTypeList,
							Label:       "Priority (multi)",
							MultiSelect: true,
							Options: []formmodal.ListOption{
								{Label: "Critical", Value: "critical"},
								{Label: "High", Value: "high", Selected: true},
								{Label: "Medium", Value: "medium"},
								{Label: "Low", Value: "low"},
							},
						},
						{
							Key:   "category",
							Type:  formmodal.FieldTypeSelect,
							Label: "Category (single)",
							Options: []formmodal.ListOption{
								{Label: "Bug", Value: "bug"},
								{Label: "Feature", Value: "feature", Selected: true},
								{Label: "Task", Value: "task"},
								{Label: "Chore", Value: "chore"},
							},
						},
						{
							Key:   "enabled",
							Type:  formmodal.FieldTypeToggle,
							Label: "Status",
							Options: []formmodal.ListOption{
								{Label: "Enabled", Value: "true"},
								{Label: "Disabled", Value: "false"},
							},
							InitialToggleIndex: 0,
						},
					},
					SubmitLabel: "Create",
				}).SetSize(m.width, m.height)
				m.formmodal = &fm
				m.showingMenu = false
				return m, fm.Init(), ""
			}
		}
		return m, nil, ""
	}

	// Formmodal is showing, forward messages
	if m.formmodal != nil {
		var cmd tea.Cmd
		newFormmodal, cmd := m.formmodal.Update(msg)
		m.formmodal = &newFormmodal
		return m, cmd, ""
	}

	return m, nil, ""
}

func (m *FormmodalDemoModel) View() string {
	var sb strings.Builder

	// Show last result if available
	if m.lastResult != "" {
		headerStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.StatusSuccessColor)
		sb.WriteString(headerStyle.Render("✓ Item Created"))
		sb.WriteString("\n\n")

		resultStyle := lipgloss.NewStyle().Foreground(styles.TextSecondaryColor)
		sb.WriteString(resultStyle.Render(m.lastResult))
		sb.WriteString("\n\n")

		keyStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.StatusInProgressColor)
		sb.WriteString(keyStyle.Render("Press Enter to create another"))
	} else if m.wasCancelled {
		headerStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)
		sb.WriteString(headerStyle.Render("Form cancelled"))
		sb.WriteString("\n\n")

		keyStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.StatusInProgressColor)
		sb.WriteString(keyStyle.Render("Press Enter to try again"))
	} else {
		instructionStyle := lipgloss.NewStyle().Foreground(styles.TextSecondaryColor)
		sb.WriteString(instructionStyle.Render("Multi-field form modal demo:"))
		sb.WriteString("\n\n")

		descStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)
		sb.WriteString(descStyle.Render("  • Text field: Name input"))
		sb.WriteString("\n")
		sb.WriteString(descStyle.Render("  • Color field: Color picker"))
		sb.WriteString("\n")
		sb.WriteString(descStyle.Render("  • Select field: Priority"))
		sb.WriteString("\n")
		sb.WriteString(descStyle.Render("  • Toggle field: Status"))
		sb.WriteString("\n\n")

		keyStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.StatusInProgressColor)
		sb.WriteString(keyStyle.Render("Press Enter to open form"))
	}

	menuContent := sb.String()

	// If formmodal is showing, overlay it
	if m.formmodal != nil {
		return m.formmodal.Overlay(menuContent)
	}

	return menuContent
}

func (m *FormmodalDemoModel) SetSize(width, height int) DemoModel {
	m.width = width
	m.height = height
	if m.formmodal != nil {
		fm := m.formmodal.SetSize(width, height)
		m.formmodal = &fm
	}
	return m
}

func (m *FormmodalDemoModel) Reset() DemoModel {
	return createFormmodalDemo(m.width, m.height)
}

func (m *FormmodalDemoModel) NeedsEscKey() bool { return false }

// =============================================================================
// Phase 3: Markdown Demo
// =============================================================================

// MarkdownDemoModel wraps the markdown component for demonstration.
type MarkdownDemoModel struct {
	rendered   string
	lastAction string
	width      int
	height     int
}

const sampleMarkdown = `# Glamour Markdown Demo

This demonstrates the **glamour** markdown renderer.

## Features

- **Bold text** and *italic text*
- Code blocks with syntax highlighting
- Lists (ordered and unordered)
- Headers at multiple levels

## Code Example

` + "```go" + `
func Hello() string {
    return "Hello, World!"
}
` + "```" + `

## Lists

1. First item
2. Second item
3. Third item

> This is a blockquote that demonstrates
> multi-line quoted text.
`

func createMarkdownDemo(width, height int) DemoModel {
	renderWidth := min(width-4, 70)
	renderer, err := markdown.New(renderWidth)
	var rendered string
	if err != nil {
		rendered = "Error creating markdown renderer: " + err.Error()
	} else {
		rendered, err = renderer.Render(sampleMarkdown)
		if err != nil {
			rendered = "Error rendering markdown: " + err.Error()
		}
	}

	return &MarkdownDemoModel{
		rendered:   rendered,
		lastAction: "Displaying sample markdown",
		width:      width,
		height:     height,
	}
}

func (m *MarkdownDemoModel) Update(msg tea.Msg) (DemoModel, tea.Cmd, string) {
	// Markdown is stateless/read-only, no updates needed
	return m, nil, ""
}

func (m *MarkdownDemoModel) View() string {
	return m.rendered
}

func (m *MarkdownDemoModel) SetSize(width, height int) DemoModel {
	m.width = width
	m.height = height
	// Re-render with new width
	renderWidth := min(width-4, 70)
	renderer, err := markdown.New(renderWidth)
	if err == nil {
		m.rendered, _ = renderer.Render(sampleMarkdown)
	}
	return m
}

func (m *MarkdownDemoModel) Reset() DemoModel {
	return createMarkdownDemo(m.width, m.height)
}

func (m *MarkdownDemoModel) NeedsEscKey() bool { return false }

// =============================================================================
// Phase 3: Chainart Demo
// =============================================================================

// ChainartDemoModel demonstrates all chain art states.
type ChainartDemoModel struct {
	state      int // 0=broken, 1=intact, 2=progress0, 3=progress2, 4=progress4, 5=failed
	lastAction string
	width      int
	height     int
}

func createChainartDemo(width, height int) DemoModel {
	return &ChainartDemoModel{
		state:      0,
		lastAction: "Press n/p to cycle through states",
		width:      width,
		height:     height,
	}
}

func (m *ChainartDemoModel) Update(msg tea.Msg) (DemoModel, tea.Cmd, string) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "n", "j":
			m.state = (m.state + 1) % 6
			m.lastAction = m.getStateName()
			return m, nil, m.lastAction
		case "p", "k":
			m.state = (m.state + 5) % 6 // +5 is same as -1 mod 6
			m.lastAction = m.getStateName()
			return m, nil, m.lastAction
		}
	}
	return m, nil, ""
}

func (m *ChainartDemoModel) getStateName() string {
	switch m.state {
	case 0:
		return "Broken chain (default)"
	case 1:
		return "Intact chain (loading)"
	case 2:
		return "Progress: 0 phases complete"
	case 3:
		return "Progress: 2 phases complete"
	case 4:
		return "Progress: 4 phases complete"
	case 5:
		return "Progress: Phase 2 failed"
	default:
		return "Unknown state"
	}
}

func (m *ChainartDemoModel) View() string {
	var sb strings.Builder

	// Show current state name
	stateStyle := lipgloss.NewStyle().Foreground(styles.TextSecondaryColor)
	sb.WriteString(stateStyle.Render("State: " + m.getStateName()))
	sb.WriteString("\n\n")

	// Render the appropriate chain art
	var art string
	switch m.state {
	case 0:
		art = chainart.BuildChainArt() // Broken chain
	case 1:
		art = chainart.BuildIntactChainArt() // Intact chain
	case 2:
		art = chainart.BuildProgressChainArt(0, -1) // 0 complete, no failure
	case 3:
		art = chainart.BuildProgressChainArt(2, -1) // 2 complete, no failure
	case 4:
		art = chainart.BuildProgressChainArt(4, -1) // 4 complete, no failure
	case 5:
		art = chainart.BuildProgressChainArt(2, 2) // 2 complete, failed at phase 2
	}

	sb.WriteString(art)
	return sb.String()
}

func (m *ChainartDemoModel) SetSize(width, height int) DemoModel {
	m.width = width
	m.height = height
	return m
}

func (m *ChainartDemoModel) Reset() DemoModel {
	return createChainartDemo(m.width, m.height)
}

func (m *ChainartDemoModel) NeedsEscKey() bool { return false }

// =============================================================================
// Phase 3: ScrollablePane Demo
// =============================================================================

// ScrollablePaneDemoModel demonstrates the scrollable pane component.
type ScrollablePaneDemoModel struct {
	viewport     viewport.Model
	contentDirty bool
	lastAction   string
	width        int
	height       int
}

func createScrollablePaneDemo(width, height int) DemoModel {
	// Create viewport with initial dimensions
	vp := viewport.New(max(width-8, 20), max(height-8, 10))

	// Generate long content to demonstrate scrolling
	var content strings.Builder
	for i := 1; i <= 50; i++ {
		content.WriteString(fmt.Sprintf("Line %2d: This is sample content for the scrollable pane demo.\n", i))
	}
	vp.SetContent(content.String())

	return &ScrollablePaneDemoModel{
		viewport:     vp,
		contentDirty: true,
		lastAction:   "Use j/k or arrow keys to scroll",
		width:        width,
		height:       height,
	}
}

func (m *ScrollablePaneDemoModel) Update(msg tea.Msg) (DemoModel, tea.Cmd, string) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			m.viewport.ScrollDown(1)
			m.lastAction = fmt.Sprintf("Scrolled down (%.0f%%)", m.viewport.ScrollPercent()*100)
			return m, nil, m.lastAction
		case "k", "up":
			m.viewport.ScrollUp(1)
			m.lastAction = fmt.Sprintf("Scrolled up (%.0f%%)", m.viewport.ScrollPercent()*100)
			return m, nil, m.lastAction
		case "g":
			m.viewport.GotoTop()
			m.lastAction = "Scrolled to top"
			return m, nil, m.lastAction
		case "G":
			m.viewport.GotoBottom()
			m.lastAction = "Scrolled to bottom"
			return m, nil, m.lastAction
		case "d":
			m.viewport.ScrollDown(m.viewport.Height / 2)
			m.lastAction = fmt.Sprintf("Page down (%.0f%%)", m.viewport.ScrollPercent()*100)
			return m, nil, m.lastAction
		case "u":
			m.viewport.ScrollUp(m.viewport.Height / 2)
			m.lastAction = fmt.Sprintf("Page up (%.0f%%)", m.viewport.ScrollPercent()*100)
			return m, nil, m.lastAction
		}
	case tea.MouseMsg:
		// Forward mouse events to viewport for scroll support
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		m.lastAction = fmt.Sprintf("Scrolled (%.0f%%)", m.viewport.ScrollPercent()*100)
		return m, cmd, m.lastAction
	}
	return m, nil, ""
}

func (m *ScrollablePaneDemoModel) View() string {
	paneWidth := min(m.width-4, 70)
	paneHeight := min(m.height-4, 20)

	// Use ScrollablePane to render with proper scroll indicators
	return panes.ScrollablePane(
		paneWidth, paneHeight,
		panes.ScrollableConfig{
			Viewport:      &m.viewport,
			ContentDirty:  m.contentDirty,
			HasNewContent: false,
			LeftTitle:     "Scrollable Content",
			TitleColor:    styles.TextPrimaryColor,
			BorderColor:   styles.BorderDefaultColor,
			Focused:       true,
		},
		func(wrapWidth int) string {
			// Generate content
			var content strings.Builder
			for i := 1; i <= 50; i++ {
				content.WriteString(fmt.Sprintf("Line %2d: Sample scrollable content demonstrating viewport.\n", i))
			}
			return content.String()
		},
	)
}

func (m *ScrollablePaneDemoModel) SetSize(width, height int) DemoModel {
	m.width = width
	m.height = height
	m.viewport.Width = max(width-12, 20)
	m.viewport.Height = max(height-12, 10)
	return m
}

func (m *ScrollablePaneDemoModel) Reset() DemoModel {
	return createScrollablePaneDemo(m.width, m.height)
}

func (m *ScrollablePaneDemoModel) NeedsEscKey() bool { return false }

// =============================================================================
// Phase 3: Logoverlay Demo
// =============================================================================

// LogoverlayDemoModel wraps the logoverlay component for demonstration.
type LogoverlayDemoModel struct {
	logoverlay logoverlay.Model
	lastAction string
	width      int
	height     int
	logIndex   int  // Counter for generating unique log entries
	generating bool // Whether auto-generation is active
}

func createLogOverlayDemo(width, height int) DemoModel {
	lo := logoverlay.NewWithSize(width, height)
	lo.Show()

	m := &LogoverlayDemoModel{
		logoverlay: lo,
		lastAction: "Press a to auto-generate logs, D/I/W/E for specific levels",
		width:      width,
		height:     height,
		logIndex:   0,
		generating: false,
	}

	// Add initial sample logs using log.LogEvent
	m.addMockLog("[DEBUG] Log overlay initialized")
	m.addMockLog("[INFO] Playground demo started")
	m.addMockLog("[WARN] This is a sample warning message")
	m.addMockLog("[ERROR] Example error for demonstration")
	m.addMockLog("[INFO] Use d/i/w/e keys to filter by level")

	return m
}

// addMockLog adds a log entry to the overlay using log.LogEvent.
// Uses fixed timestamp "00:00:00" for deterministic golden tests.
func (m *LogoverlayDemoModel) addMockLog(entry string) {
	m.logIndex++
	// Use fixed timestamp for golden test stability
	fullEntry := fmt.Sprintf("00:00:00 %s", entry)
	// Send as log.LogEvent which logoverlay's Update handles
	event := log.LogEvent{
		Type:      pubsub.CreatedEvent,
		Payload:   fullEntry,
		Timestamp: time.Now(),
	}
	m.logoverlay, _ = m.logoverlay.Update(event)
}

// generateMockLogCmd returns a command that generates a mock log after a delay.
func generateMockLogCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		levels := []string{"[DEBUG]", "[INFO]", "[INFO]", "[WARN]", "[ERROR]"}
		messages := []string{
			"Processing request",
			"Database query executed",
			"Cache hit for key",
			"Connection pool low",
			"Failed to parse config",
			"User authenticated",
			"API call completed",
			"Memory usage high",
			"Retry attempt failed",
			"Task completed successfully",
		}
		level := levels[int(t.UnixNano())%len(levels)]
		msg := messages[int(t.UnixNano()/1000)%len(messages)]
		return log.LogEvent{
			Type:      pubsub.CreatedEvent,
			Payload:   fmt.Sprintf("%s %s %s", t.Format("15:04:05"), level, msg),
			Timestamp: t,
		}
	})
}

func (m *LogoverlayDemoModel) Update(msg tea.Msg) (DemoModel, tea.Cmd, string) {
	switch msg := msg.(type) {
	case log.LogEvent:
		// Forward log event to logoverlay
		m.logoverlay, _ = m.logoverlay.Update(msg)
		m.logIndex++
		m.lastAction = fmt.Sprintf("Log #%d added", m.logIndex)

		// Continue generating if active
		if m.generating {
			return m, generateMockLogCmd(), m.lastAction
		}
		return m, nil, m.lastAction

	case logoverlay.CloseMsg:
		// Re-show the overlay for demo purposes
		m.logoverlay.Show()
		m.lastAction = "Overlay re-opened (press Esc to close)"
		return m, nil, m.lastAction

	case tea.KeyMsg:
		// First forward to logoverlay for its key handling
		var cmd tea.Cmd
		m.logoverlay, cmd = m.logoverlay.Update(msg)

		switch msg.String() {
		case "a":
			// Toggle auto-generation
			m.generating = !m.generating
			if m.generating {
				m.lastAction = "Auto-generating logs..."
				return m, generateMockLogCmd(), m.lastAction
			}
			m.lastAction = "Auto-generation stopped"
			return m, nil, m.lastAction
		case "D":
			// Generate debug log
			m.addMockLog("[DEBUG] Manual debug message")
			m.lastAction = "Added DEBUG log"
			return m, nil, m.lastAction
		case "I":
			// Generate info log
			m.addMockLog("[INFO] Manual info message")
			m.lastAction = "Added INFO log"
			return m, nil, m.lastAction
		case "W":
			// Generate warn log
			m.addMockLog("[WARN] Manual warning message")
			m.lastAction = "Added WARN log"
			return m, nil, m.lastAction
		case "E":
			// Generate error log
			m.addMockLog("[ERROR] Manual error message")
			m.lastAction = "Added ERROR log"
			return m, nil, m.lastAction
		}

		return m, cmd, ""
	}

	// Forward other messages to logoverlay
	var cmd tea.Cmd
	m.logoverlay, cmd = m.logoverlay.Update(msg)
	return m, cmd, ""
}

func (m *LogoverlayDemoModel) View() string {
	var sb strings.Builder

	instructionStyle := lipgloss.NewStyle().Foreground(styles.TextSecondaryColor)
	sb.WriteString(instructionStyle.Render("Log viewer with mock log generation:"))
	sb.WriteString("\n\n")

	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.StatusInProgressColor)
	descStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)

	sb.WriteString("  " + keyStyle.Render("g") + " " + descStyle.Render("Toggle auto-generate logs"))
	sb.WriteString("\n")
	sb.WriteString("  " + keyStyle.Render("D") + " " + descStyle.Render("Add DEBUG log"))
	sb.WriteString("\n")
	sb.WriteString("  " + keyStyle.Render("I") + " " + descStyle.Render("Add INFO log"))
	sb.WriteString("\n")
	sb.WriteString("  " + keyStyle.Render("W") + " " + descStyle.Render("Add WARN log"))
	sb.WriteString("\n")
	sb.WriteString("  " + keyStyle.Render("E") + " " + descStyle.Render("Add ERROR log"))
	sb.WriteString("\n\n")

	sb.WriteString(descStyle.Render("Inside overlay:"))
	sb.WriteString("\n")
	sb.WriteString("  " + keyStyle.Render("d/i/w/e") + " " + descStyle.Render("Filter by log level"))
	sb.WriteString("\n")
	sb.WriteString("  " + keyStyle.Render("c") + " " + descStyle.Render("Clear logs"))
	sb.WriteString("\n")
	sb.WriteString("  " + keyStyle.Render("j/k") + " " + descStyle.Render("Scroll up/down"))

	if m.generating {
		sb.WriteString("\n\n")
		statusStyle := lipgloss.NewStyle().Foreground(styles.StatusSuccessColor)
		sb.WriteString(statusStyle.Render("● Auto-generating logs..."))
	}

	bgContent := sb.String()

	// Overlay the logoverlay on top
	if m.logoverlay.Visible() {
		return m.logoverlay.Overlay(bgContent)
	}
	return bgContent
}

func (m *LogoverlayDemoModel) SetSize(width, height int) DemoModel {
	m.width = width
	m.height = height
	m.logoverlay.SetSize(width, height)
	return m
}

func (m *LogoverlayDemoModel) Reset() DemoModel {
	return createLogOverlayDemo(m.width, m.height)
}

func (m *LogoverlayDemoModel) NeedsEscKey() bool { return false }

// =============================================================================
// IssueBadge Demo
// =============================================================================

// IssueBadgeDemoModel demonstrates the issuebadge component.
type IssueBadgeDemoModel struct {
	selectedIdx int // Currently selected example
	width       int
	height      int
}

// ptrBool returns a pointer to a bool value.
func ptrBool(b bool) *bool {
	return &b
}

// sampleIssues provides examples for the demo, showcasing all issue types and priorities.
var sampleIssues = []beads.Issue{
	// All 5 issue types
	{ID: "demo-e01", Type: beads.TypeEpic, Priority: 1, TitleText: "Epic: User Authentication System"},
	{ID: "demo-t02", Type: beads.TypeTask, Priority: 2, TitleText: "Task: Implement login form validation"},
	{ID: "demo-f03", Type: beads.TypeFeature, Priority: 1, TitleText: "Feature: Add dark mode support"},
	{ID: "demo-b04", Type: beads.TypeBug, Priority: 0, TitleText: "Bug: Fix memory leak in cache"},
	{ID: "demo-c05", Type: beads.TypeChore, Priority: 3, TitleText: "Chore: Update dependencies"},
	// All 5 priority levels
	{ID: "demo-p0", Type: beads.TypeBug, Priority: 0, TitleText: "P0 Critical: Security vulnerability"},
	{ID: "demo-p1", Type: beads.TypeFeature, Priority: 1, TitleText: "P1 High: Core feature request"},
	{ID: "demo-p2", Type: beads.TypeTask, Priority: 2, TitleText: "P2 Medium: Standard work item"},
	{ID: "demo-p3", Type: beads.TypeChore, Priority: 3, TitleText: "P3 Low: Nice to have improvement"},
	{ID: "demo-p4", Type: beads.TypeTask, Priority: 4, TitleText: "P4 Backlog: Future consideration"},
	// Long title for truncation demo
	{ID: "demo-long", Type: beads.TypeFeature, Priority: 2, TitleText: "This is a very long title that will be truncated to demonstrate the MaxWidth configuration option in action"},
	// Pinned issue demo
	{ID: "demo-pin", Type: beads.TypeTask, Priority: 1, TitleText: "Pinned: Important task always visible", Pinned: ptrBool(true)},
}

func createIssueBadgeDemo(width, height int) DemoModel {
	return &IssueBadgeDemoModel{
		selectedIdx: 0,
		width:       width,
		height:      height,
	}
}

func (m *IssueBadgeDemoModel) Update(msg tea.Msg) (DemoModel, tea.Cmd, string) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if m.selectedIdx < len(sampleIssues)-1 {
				m.selectedIdx++
			}
			return m, nil, fmt.Sprintf("Selected: %s", sampleIssues[m.selectedIdx].ID)
		case "k", "up":
			if m.selectedIdx > 0 {
				m.selectedIdx--
			}
			return m, nil, fmt.Sprintf("Selected: %s", sampleIssues[m.selectedIdx].ID)
		}
	}
	return m, nil, ""
}

func (m *IssueBadgeDemoModel) View() string {
	var sb strings.Builder

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.TextPrimaryColor)
	sectionStyle := lipgloss.NewStyle().Foreground(styles.TextSecondaryColor).MarginTop(1)
	instructionStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor).Italic(true)

	// Header
	sb.WriteString(headerStyle.Render("Issue Badge Component Demo"))
	sb.WriteString("\n")
	sb.WriteString(instructionStyle.Render("Use j/k to move selection"))
	sb.WriteString("\n\n")

	// Section 1: Issue Types
	sb.WriteString(sectionStyle.Render("Issue Types (E/T/F/B/C):"))
	sb.WriteString("\n")
	for i := range 5 {
		issue := sampleIssues[i]
		line := issuebadge.Render(issue, issuebadge.Config{
			ShowSelection: true,
			Selected:      i == m.selectedIdx,
		})
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	// Section 2: Priority Levels
	sb.WriteString("\n")
	sb.WriteString(sectionStyle.Render("Priority Levels (P0-P4):"))
	sb.WriteString("\n")
	for i := 5; i < 10; i++ {
		issue := sampleIssues[i]
		line := issuebadge.Render(issue, issuebadge.Config{
			ShowSelection: true,
			Selected:      i == m.selectedIdx,
		})
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	// Section 3: Title Truncation
	sb.WriteString("\n")
	sb.WriteString(sectionStyle.Render("Title Truncation (MaxWidth=60):"))
	sb.WriteString("\n")
	truncatedIssue := sampleIssues[10]
	truncatedLine := issuebadge.Render(truncatedIssue, issuebadge.Config{
		ShowSelection: true,
		Selected:      m.selectedIdx == 10,
		MaxWidth:      60,
	})
	sb.WriteString(truncatedLine)
	sb.WriteString("\n")

	// Section 4: Pinned Issue
	sb.WriteString("\n")
	sb.WriteString(sectionStyle.Render("Pinned Issue (📌 indicator):"))
	sb.WriteString("\n")
	pinnedIssue := sampleIssues[11]
	pinnedLine := issuebadge.Render(pinnedIssue, issuebadge.Config{
		ShowSelection: true,
		Selected:      m.selectedIdx == 11,
	})
	sb.WriteString(pinnedLine)
	sb.WriteString("\n")

	// Section 5: Badge Only (no title)
	sb.WriteString("\n")
	sb.WriteString(sectionStyle.Render("Badge Only (RenderBadge):"))
	sb.WriteString("\n")
	for i := range 5 {
		badge := issuebadge.RenderBadge(sampleIssues[i])
		sb.WriteString(badge)
		sb.WriteString(" ")
	}
	// Also show pinned badge
	sb.WriteString(issuebadge.RenderBadge(sampleIssues[11]))
	sb.WriteString("\n")

	return sb.String()
}

func (m *IssueBadgeDemoModel) SetSize(width, height int) DemoModel {
	m.width = width
	m.height = height
	return m
}

func (m *IssueBadgeDemoModel) Reset() DemoModel {
	return createIssueBadgeDemo(m.width, m.height)
}

func (m *IssueBadgeDemoModel) NeedsEscKey() bool { return false }

// =============================================================================
// Theme Tokens Demo
// =============================================================================

// ThemeTokensDemoModel wraps the theme token viewer for demonstration.
type ThemeTokensDemoModel struct {
	viewport     viewport.Model
	contentDirty bool
	width        int
	height       int
}

func createThemeTokensDemo(width, height int) DemoModel {
	vp := viewport.New(max(width, 20), max(height, 10))
	vp.SetContent(renderTokenContent())
	return &ThemeTokensDemoModel{
		viewport:     vp,
		contentDirty: false,
		width:        width,
		height:       height,
	}
}

func (m *ThemeTokensDemoModel) Update(msg tea.Msg) (DemoModel, tea.Cmd, string) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "g":
			m.viewport.GotoTop()
			return m, nil, ""
		case "G":
			m.viewport.GotoBottom()
			return m, nil, ""
		}
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd, ""
}

func (m *ThemeTokensDemoModel) View() string {
	return m.viewport.View()
}

func (m *ThemeTokensDemoModel) SetSize(width, height int) DemoModel {
	m.width = width
	m.height = height
	m.viewport.Width = max(width, 20)
	m.viewport.Height = max(height, 10)
	return m
}

func (m *ThemeTokensDemoModel) Reset() DemoModel {
	return createThemeTokensDemo(m.width, m.height)
}

func (m *ThemeTokensDemoModel) NeedsEscKey() bool { return false }

// renderTokenContent generates the theme token content without a title header.
func renderTokenContent() string {
	var sb strings.Builder

	categories := GetTokenCategories()

	categoryStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.StatusInProgressColor)
	tokenNameStyle := lipgloss.NewStyle().Foreground(styles.TextSecondaryColor)
	hexStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)

	for i, cat := range categories {
		if i > 0 {
			sb.WriteString("\n") // Add spacing between categories, but not before first
		}
		sb.WriteString(categoryStyle.Render(cat.Name))
		sb.WriteString("\n")

		for _, token := range cat.Tokens {
			hex := GetTokenColor(token)
			swatch := lipgloss.NewStyle().
				Background(lipgloss.Color(hex)).
				Render("  ")

			tokenName := string(token)
			line := swatch + " " + tokenNameStyle.Render(tokenName) + " " + hexStyle.Render(hex)
			sb.WriteString(line)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}
