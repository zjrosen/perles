// Package issueeditor provides a unified modal for editing issue properties.
//
// This modal combines priority, status, and labels editing into a single form,
// replacing the previous three-modal architecture with a streamlined interface.
package issueeditor

import (
	"slices"
	"strconv"

	"github.com/zjrosen/perles/internal/mode/shared"
	"github.com/zjrosen/perles/internal/task"
	"github.com/zjrosen/perles/internal/ui/shared/formmodal"
	"github.com/zjrosen/perles/internal/ui/shared/issuebadge"

	tea "github.com/charmbracelet/bubbletea"
)

// Model holds the issue editor state.
type Model struct {
	issue      task.Issue
	form       formmodal.Model
	createMode bool
}

// SaveMsg is sent when the user confirms issue changes.
type SaveMsg struct {
	IssueID     string
	IssueType   task.IssueType
	ParentID    string
	Title       string
	Description string
	Notes       string
	Priority    task.Priority
	Status      task.Status
	Labels      []string
}

// CancelMsg is sent when the user cancels the editor.
type CancelMsg struct{}

// BuildUpdateOptions compares the SaveMsg fields against the original issue
// snapshot and returns an UpdateIssueOptions with only changed fields set (non-nil).
// If original is nil (safety fallback), all fields are populated from the SaveMsg.
func (m SaveMsg) BuildUpdateOptions(original *task.Issue) task.UpdateOptions {
	var opts task.UpdateOptions
	if original == nil {
		opts.Title = &m.Title
		opts.Description = &m.Description
		opts.Notes = &m.Notes
		p := m.Priority
		opts.Priority = &p
		s := m.Status
		opts.Status = &s
		labels := cloneLabels(m.Labels)
		opts.Labels = &labels
		return opts
	}
	if m.Title != original.TitleText {
		opts.Title = &m.Title
	}
	if m.Description != original.DescriptionText {
		opts.Description = &m.Description
	}
	if m.Notes != original.Notes {
		opts.Notes = &m.Notes
	}
	if m.Priority != original.Priority {
		p := m.Priority
		opts.Priority = &p
	}
	if m.Status != original.Status {
		s := m.Status
		opts.Status = &s
	}
	if !slices.Equal(m.Labels, original.Labels) {
		labels := cloneLabels(m.Labels)
		opts.Labels = &labels
	}
	return opts
}

func cloneLabels(labels []string) []string {
	if labels == nil {
		return nil
	}
	return append([]string{}, labels...)
}

// New creates a new issue editor with vim mode disabled by default.
// Use NewWithVimMode to control vim behavior from user configuration.
func New(issue task.Issue) Model {
	return NewWithVimMode(issue, false)
}

// NewWithVimMode creates a new issue editor with the given issue and vim mode setting.
func NewWithVimMode(issue task.Issue, vimEnabled bool) Model {
	return newModel(issue, nil, vimEnabled, false)
}

// NewForCreate creates a new issue editor in create mode with vim mode disabled.
func NewForCreate() Model {
	return NewForCreateWithExecutorAndVimMode(nil, false)
}

// NewForCreateWithVimMode creates a new issue editor in create mode.
func NewForCreateWithVimMode(vimEnabled bool) Model {
	return NewForCreateWithExecutorAndVimMode(nil, vimEnabled)
}

// NewForCreateWithExecutorAndVimMode creates a new issue editor in create mode.
func NewForCreateWithExecutorAndVimMode(epicSearchExecutor task.QueryExecutor, vimEnabled bool) Model {
	issue := task.Issue{
		Type:     task.TypeTask,
		Priority: task.PriorityMedium,
		Status:   task.StatusOpen,
		Labels:   []string{},
	}
	return newModel(issue, epicSearchExecutor, vimEnabled, true)
}

func newModel(issue task.Issue, epicSearchExecutor task.QueryExecutor, vimEnabled, createMode bool) Model {
	m := Model{issue: issue, createMode: createMode}
	cfg := formmodal.FormConfig{
		Title:        m.title(),
		TitleContent: m.titleContent(),
		Columns:      []formmodal.ColumnConfig{{}, {}},
		Fields:       issueFields(issue, epicSearchExecutor, vimEnabled, createMode),
		SubmitLabel:  m.submitLabel(),
		MinWidth:     52,
		OnSubmit: func(values map[string]any) tea.Msg {
			return saveMsgFromValues(issue, values, createMode)
		},
		OnCancel: func() tea.Msg { return CancelMsg{} },
	}
	m.form = formmodal.New(cfg)
	return m
}

func issueFields(issue task.Issue, epicSearchExecutor task.QueryExecutor, vimEnabled, createMode bool) []formmodal.FieldConfig {
	fields := []formmodal.FieldConfig{
		{
			Key:          "title",
			Type:         formmodal.FieldTypeTextArea,
			Label:        "Title",
			Placeholder:  "Issue title...",
			InitialValue: issue.TitleText,
			MaxLength:    200,
			MaxHeight:    3,
			VimEnabled:   vimEnabled,
			Column:       0,
		},
	}

	if createMode {
		fields = append(fields, formmodal.FieldConfig{
			Key:     "type",
			Type:    formmodal.FieldTypeSelect,
			Label:   "Type",
			Hint:    "Space to toggle",
			Options: issueTypeListOptions(issue.Type),
			Column:  0,
		})
	}

	fields = append(fields, formmodal.FieldConfig{
		Key:     "priority",
		Type:    formmodal.FieldTypeSelect,
		Label:   "Priority",
		Hint:    "Space to toggle",
		Options: priorityListOptions(issue.Priority),
		Column:  0,
	})

	if createMode {
		fields = append(fields, formmodal.FieldConfig{
			Key:                "parent_id",
			Type:               formmodal.FieldTypeEpicSearch,
			Label:              "Parent Epic",
			Hint:               "Enter to search",
			SearchPlaceholder:  "Search epics...",
			InitialValue:       issue.ParentID,
			EpicSearchExecutor: epicSearchExecutor,
			DebounceMs:         200,
			Column:             0,
			VisibleWhen: func(values map[string]any) bool {
				issueType, _ := values["type"].(string)
				return issueType != string(task.TypeEpic)
			},
		})
	}

	fields = append(fields,
		formmodal.FieldConfig{
			Key:     "status",
			Type:    formmodal.FieldTypeSelect,
			Label:   "Status",
			Hint:    "Space to toggle",
			Options: statusListOptions(issue.Status),
			Column:  0,
		},
		formmodal.FieldConfig{
			Key:              "labels",
			Type:             formmodal.FieldTypeEditableList,
			Label:            "Labels",
			Hint:             "Space to toggle",
			Options:          labelsListOptions(issue.Labels),
			InputLabel:       "Add Label",
			InputHint:        "Enter to add",
			InputPlaceholder: "Enter label name...",
			Column:           0,
		},
		formmodal.FieldConfig{
			Key:          "description",
			Type:         formmodal.FieldTypeTextArea,
			Label:        "Description",
			Hint:         "Ctrl+G for editor",
			Placeholder:  "Issue description...",
			InitialValue: issue.DescriptionText,
			VimEnabled:   vimEnabled,
			MaxHeight:    8,
			Column:       1,
		},
		formmodal.FieldConfig{
			Key:          "notes",
			Type:         formmodal.FieldTypeTextArea,
			Label:        "Notes",
			Hint:         "Ctrl+G for editor",
			Placeholder:  "Issue notes...",
			InitialValue: issue.Notes,
			VimEnabled:   vimEnabled,
			MaxHeight:    8,
			Column:       1,
		},
	)

	return fields
}

func saveMsgFromValues(issue task.Issue, values map[string]any, createMode bool) SaveMsg {
	issueType := issue.Type
	if createMode {
		issueType = task.IssueType(values["type"].(string))
	}
	parentID, _ := values["parent_id"].(string)
	if issueType == task.TypeEpic {
		parentID = ""
	}

	return SaveMsg{
		IssueID:     issue.ID,
		IssueType:   issueType,
		ParentID:    parentID,
		Title:       values["title"].(string),
		Description: values["description"].(string),
		Notes:       values["notes"].(string),
		Priority:    parsePriority(values["priority"].(string)),
		Status:      task.Status(values["status"].(string)),
		Labels:      append([]string(nil), values["labels"].([]string)...),
	}
}

func issueTypeListOptions(current task.IssueType) []formmodal.ListOption {
	if current == "" {
		current = task.TypeTask
	}

	types := []task.IssueType{
		task.TypeTask,
		task.TypeBug,
		task.TypeFeature,
		task.TypeEpic,
		task.TypeChore,
		task.TypeStory,
		task.TypeSpike,
		task.TypeMilestone,
	}

	result := make([]formmodal.ListOption, len(types))
	for i, issueType := range types {
		result[i] = formmodal.ListOption{
			Label:    string(issueType),
			Value:    string(issueType),
			Selected: issueType == current,
		}
	}
	return result
}

func (m Model) title() string {
	if m.createMode {
		return "New Issue"
	}
	return "Edit Issue"
}

func (m Model) submitLabel() string {
	if m.createMode {
		return "Create"
	}
	return "Save"
}

func (m Model) titleContent() func(width int) string {
	if m.createMode {
		return nil
	}
	return func(width int) string {
		return issuebadge.RenderBadge(m.issue)
	}
}

func (m Model) IsCreateMode() bool {
	return m.createMode
}

// priorityListOptions converts shared.PriorityOptions to formmodal.ListOption
// with the current priority pre-selected, preserving colors.
func priorityListOptions(current task.Priority) []formmodal.ListOption {
	opts := shared.PriorityOptions()
	result := make([]formmodal.ListOption, len(opts))
	for i, opt := range opts {
		result[i] = formmodal.ListOption{
			Label:    opt.Label,
			Value:    opt.Value,
			Selected: i == int(current),
			Color:    opt.Color,
		}
	}
	return result
}

// statusListOptions converts shared.StatusOptions to formmodal.ListOption
// with the current status pre-selected, preserving colors.
func statusListOptions(current task.Status) []formmodal.ListOption {
	opts := shared.StatusOptions()
	result := make([]formmodal.ListOption, len(opts))
	for i, opt := range opts {
		result[i] = formmodal.ListOption{
			Label:    opt.Label,
			Value:    opt.Value,
			Selected: opt.Value == string(current),
			Color:    opt.Color,
		}
	}
	return result
}

// labelsListOptions converts a slice of labels to formmodal.ListOption
// with all labels initially selected.
func labelsListOptions(labels []string) []formmodal.ListOption {
	result := make([]formmodal.ListOption, len(labels))
	for i, label := range labels {
		result[i] = formmodal.ListOption{
			Label:    label,
			Value:    label,
			Selected: true,
		}
	}
	return result
}

// parsePriority parses a priority string value (e.g., "P0") to task.Priority.
func parsePriority(value string) task.Priority {
	if len(value) >= 2 && value[0] == 'P' {
		if p, err := strconv.Atoi(value[1:]); err == nil && p >= 0 && p <= 4 {
			return task.Priority(p)
		}
	}
	return task.PriorityMedium // default to medium if parsing fails
}

// SetSize sets the viewport dimensions for overlay rendering.
func (m Model) SetSize(width, height int) Model {
	m.form = m.form.SetSize(width, height)
	return m
}

// Init initializes the model.
func (m Model) Init() tea.Cmd {
	return m.form.Init()
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.form, cmd = m.form.Update(msg)
	return m, cmd
}

// View renders the issue editor modal.
func (m Model) View() string {
	return m.form.View()
}

// Overlay renders the issue editor over a background view.
func (m Model) Overlay(background string) string {
	return m.form.Overlay(background)
}
