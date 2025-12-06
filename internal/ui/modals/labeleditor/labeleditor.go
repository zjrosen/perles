// Package labeleditor provides a modal for editing issue labels.
package labeleditor

import (
	"perles/internal/ui/shared/formmodal"

	tea "github.com/charmbracelet/bubbletea"
)

// Model holds the label editor state.
type Model struct {
	issueID string
	form    formmodal.Model
}

// SaveMsg is sent when the user confirms label changes.
type SaveMsg struct {
	IssueID string
	Labels  []string // Final label set to persist
}

// CancelMsg is sent when the user cancels the editor.
type CancelMsg struct{}

// New creates a new label editor.
func New(issueID string, currentLabels []string) Model {
	// Convert current labels to ListOptions (all initially enabled)
	options := make([]formmodal.ListOption, len(currentLabels))
	for i, label := range currentLabels {
		options[i] = formmodal.ListOption{
			Label:    label,
			Value:    label,
			Selected: true,
		}
	}

	m := Model{issueID: issueID}

	cfg := formmodal.FormConfig{
		Title: "Edit Labels",
		Fields: []formmodal.FieldConfig{
			{
				Key:              "labels",
				Type:             formmodal.FieldTypeEditableList,
				Label:            "Labels",
				Hint:             "Space to toggle",
				Options:          options,
				InputLabel:       "Add Label",
				InputHint:        "Enter to add",
				InputPlaceholder: "Enter label name...",
			},
		},
		SubmitLabel: "Save",
		MinWidth:    42,
		OnSubmit: func(values map[string]any) tea.Msg {
			return SaveMsg{
				IssueID: m.issueID,
				Labels:  values["labels"].([]string),
			}
		},
		OnCancel: func() tea.Msg { return CancelMsg{} },
	}

	m.form = formmodal.New(cfg)
	return m
}

// SetSize sets the viewport dimensions for overlay rendering.
func (m Model) SetSize(width, height int) Model {
	m.form = m.form.SetSize(width, height)
	return m
}

// Init initializes the model.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.form, cmd = m.form.Update(msg)
	return m, cmd
}

// View renders the label editor modal.
func (m Model) View() string {
	return m.form.View()
}

// Overlay renders the label editor on top of a background view.
func (m Model) Overlay(background string) string {
	return m.form.Overlay(background)
}
