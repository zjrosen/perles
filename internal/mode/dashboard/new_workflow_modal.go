// Package dashboard implements the multi-workflow dashboard TUI mode.
package dashboard

import (
	"context"
	"errors"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	appgit "github.com/zjrosen/perles/internal/git/application"
	"github.com/zjrosen/perles/internal/orchestration/controlplane"
	appreg "github.com/zjrosen/perles/internal/registry/application"
	"github.com/zjrosen/perles/internal/ui/shared/formmodal"
)

// NewWorkflowModal holds the state for the new workflow creation modal.
type NewWorkflowModal struct {
	form            formmodal.Model
	registryService *appreg.RegistryService // Registry for template listing, validation, and epic_driven.md access
	controlPlane    controlplane.ControlPlane
	gitExecutor     appgit.GitExecutor
	workflowCreator *appreg.WorkflowCreator
	worktreeEnabled bool // track if worktree options are available
}

// CreateWorkflowMsg is sent when a workflow is created successfully.
type CreateWorkflowMsg struct {
	WorkflowID controlplane.WorkflowID
	Name       string
}

// CancelNewWorkflowMsg is sent when the modal is cancelled.
type CancelNewWorkflowMsg struct{}

// NewNewWorkflowModal creates a new workflow creation modal.
// gitExecutor is optional - if nil or if ListBranches() fails, worktree options are disabled.
// workflowCreator is optional - if nil, epic creation is skipped.
// registryService is optional - if nil, template listing returns empty options.
func NewNewWorkflowModal(
	registryService *appreg.RegistryService,
	cp controlplane.ControlPlane,
	gitExecutor appgit.GitExecutor,
	workflowCreator *appreg.WorkflowCreator,
) *NewWorkflowModal {
	m := &NewWorkflowModal{
		registryService: registryService,
		controlPlane:    cp,
		gitExecutor:     gitExecutor,
		workflowCreator: workflowCreator,
	}

	// Build template options from registry service
	templateOptions := buildTemplateOptions(registryService)

	// Build branch options from git executor (if available)
	branchOptions, worktreeAvailable := buildBranchOptions(gitExecutor)
	m.worktreeEnabled = worktreeAvailable

	// Build form fields
	fields := []formmodal.FieldConfig{
		{
			Key:               "template",
			Type:              formmodal.FieldTypeSearchSelect,
			Label:             "Template",
			Hint:              "required",
			Options:           templateOptions,
			SearchPlaceholder: "Search templates...",
			MaxVisibleItems:   5,
		},
		{
			Key:         "name",
			Type:        formmodal.FieldTypeText,
			Label:       "Name",
			Hint:        "optional",
			Placeholder: "Workflow name (defaults to template name)",
		},
		{
			Key:         "goal",
			Type:        formmodal.FieldTypeTextArea,
			Label:       "Goal",
			Hint:        "required",
			Placeholder: "What should this workflow accomplish?",
			MaxHeight:   5,
			VimEnabled:  true,
		},
	}

	// Add worktree fields if git support is available
	if worktreeAvailable {
		// Helper to check if worktree is enabled
		worktreeEnabled := func(values map[string]any) bool {
			v, _ := values["use_worktree"].(string)
			return v == "true"
		}

		worktreeFields := []formmodal.FieldConfig{
			{
				Key:   "use_worktree",
				Type:  formmodal.FieldTypeToggle,
				Label: "Git Worktree",
				Hint:  "optional",
				Options: []formmodal.ListOption{
					{Label: "No", Value: "false", Selected: true},
					{Label: "Yes", Value: "true"},
				},
			},
			{
				Key:               "base_branch",
				Type:              formmodal.FieldTypeSearchSelect,
				Label:             "Base Branch",
				Hint:              "required",
				Options:           branchOptions,
				SearchPlaceholder: "Search branches...",
				MaxVisibleItems:   5,
				VisibleWhen:       worktreeEnabled,
			},
			{
				Key:         "custom_branch",
				Type:        formmodal.FieldTypeText,
				Label:       "Branch Name",
				Hint:        "optional - auto-generated if empty",
				Placeholder: "perles-workflow-abc123",
				VisibleWhen: worktreeEnabled,
			},
		}
		fields = append(fields, worktreeFields...)
	}

	cfg := formmodal.FormConfig{
		Title:       "New Workflow",
		Fields:      fields,
		SubmitLabel: "Create",
		MinWidth:    65,
		Validate:    m.validate,
		OnSubmit:    m.onSubmit,
		OnCancel:    func() tea.Msg { return CancelNewWorkflowMsg{} },
	}

	m.form = formmodal.New(cfg)
	return m
}

// buildBranchOptions converts git branches to list options.
// Returns the options and a boolean indicating if worktree support is available.
func buildBranchOptions(gitExecutor appgit.GitExecutor) ([]formmodal.ListOption, bool) {
	if gitExecutor == nil {
		return nil, false
	}

	branches, err := gitExecutor.ListBranches()
	if err != nil {
		return nil, false
	}

	if len(branches) == 0 {
		return nil, false
	}

	options := make([]formmodal.ListOption, len(branches))
	for i, branch := range branches {
		options[i] = formmodal.ListOption{
			Label:    branch.Name,
			Value:    branch.Name,
			Selected: branch.IsCurrent, // Select current branch by default
		}
	}

	return options, true
}

// buildTemplateOptions converts domain registry registrations to list options.
// Uses GetByNamespace("spec-workflow") to get only workflow templates (not language guidelines).
func buildTemplateOptions(registryService *appreg.RegistryService) []formmodal.ListOption {
	if registryService == nil {
		return []formmodal.ListOption{}
	}

	// Get spec-workflow registrations (workflow templates, not language guidelines)
	registrations := registryService.GetByNamespace("spec-workflow")

	options := make([]formmodal.ListOption, len(registrations))
	for i, reg := range registrations {
		options[i] = formmodal.ListOption{
			Label:    reg.Name(),
			Subtext:  reg.Description(),
			Value:    reg.Key(), // Use key for WorkflowCreator.Create()
			Selected: i == 0,    // Select first template by default
		}
	}

	return options
}

// validate checks form values before submission.
func (m *NewWorkflowModal) validate(values map[string]any) error {
	// Template is required
	templateKey, ok := values["template"].(string)
	if !ok || templateKey == "" {
		return errors.New("template is required")
	}

	// Verify template exists in domain registry
	if m.registryService != nil {
		if _, err := m.registryService.GetByKey("spec-workflow", templateKey); err != nil {
			return errors.New("selected template not found")
		}
	}

	// Goal is required
	goal, ok := values["goal"].(string)
	if !ok || goal == "" {
		return errors.New("goal is required")
	}

	// Validate worktree fields if worktree is enabled
	if m.worktreeEnabled {
		useWorktree, _ := values["use_worktree"].(string)
		if useWorktree == "true" {
			// Base branch is required when worktree is enabled
			baseBranch, _ := values["base_branch"].(string)
			if baseBranch == "" {
				return errors.New("base branch is required when worktree is enabled")
			}

			// Validate custom branch name if provided
			customBranch, _ := values["custom_branch"].(string)
			if customBranch != "" && m.gitExecutor != nil {
				if err := m.gitExecutor.ValidateBranchName(customBranch); err != nil {
					return errors.New("invalid branch name: " + err.Error())
				}
			}
		}
	}

	return nil
}

// ErrorMsg is sent when workflow creation fails.
type ErrorMsg struct {
	Err error
}

// onSubmit creates the workflow from form values.
// If WorkflowCreator is available, it first creates an epic and tasks in beads,
// then builds the coordinator prompt from epic_driven.md + epic ID + user goal.
func (m *NewWorkflowModal) onSubmit(values map[string]any) tea.Msg {
	templateID := values["template"].(string)
	name := values["name"].(string)
	goal := values["goal"].(string)

	var epicID string
	var initialPrompt string

	// If WorkflowCreator is available, create epic + tasks in beads first
	if m.workflowCreator != nil {
		// Use name as feature slug, or derive from templateID if empty
		feature := name
		if feature == "" {
			feature = templateID
		}

		result, err := m.workflowCreator.Create(feature, templateID)
		if err != nil {
			return ErrorMsg{Err: fmt.Errorf("create epic: %w", err)}
		}

		epicID = result.Epic.ID

		// Build coordinator prompt: epic_driven.md + epic ID section + user goal
		initialPrompt = m.buildCoordinatorPrompt(epicID, goal)
	} else {
		// No WorkflowCreator, use goal directly as InitialGoal
		initialPrompt = goal
	}

	// Build WorkflowSpec
	spec := controlplane.WorkflowSpec{
		TemplateID:  templateID,
		InitialGoal: initialPrompt,
		Name:        name,
		EpicID:      epicID,
	}

	// Set worktree fields if worktree options are available
	if m.worktreeEnabled {
		useWorktree, _ := values["use_worktree"].(string)
		if useWorktree == "true" {
			spec.WorktreeEnabled = true
			spec.WorktreeBaseBranch, _ = values["base_branch"].(string)
			spec.WorktreeBranchName, _ = values["custom_branch"].(string)
		}
	}

	// Create the workflow
	if m.controlPlane == nil {
		return CreateWorkflowMsg{Name: spec.Name}
	}

	workflowID, err := m.controlPlane.Create(context.Background(), spec)
	if err != nil {
		// Return error as message (modal will display validation error)
		return CreateWorkflowMsg{Name: spec.Name}
	}

	return CreateWorkflowMsg{
		WorkflowID: workflowID,
		Name:       spec.Name,
	}
}

// buildCoordinatorPrompt assembles the coordinator prompt from:
// 1. epic_driven.md content (generic coordinator instructions)
// 2. Epic ID section (so coordinator can read detailed instructions via bd show)
// 3. User's goal
func (m *NewWorkflowModal) buildCoordinatorPrompt(epicID, goal string) string {
	// Load epic_driven.md template if registry service is available
	var epicDrivenContent string
	if m.registryService != nil {
		content, err := m.registryService.GetEpicDrivenTemplate()
		if err == nil {
			epicDrivenContent = content
		}
		// If error loading template, continue without it
	}

	// Build the full prompt
	if epicDrivenContent != "" {
		return fmt.Sprintf(`%s

---

# Your Epic

Epic ID: %s

Use `+"`bd show %s --json`"+` to read your detailed workflow instructions.

# Your Goal

%s
`, epicDrivenContent, epicID, epicID, goal)
	}

	// Fallback if no epic_driven.md available
	return fmt.Sprintf(`# Your Epic

Epic ID: %s

Use `+"`bd show %s --json`"+` to read your detailed workflow instructions.

# Your Goal

%s
`, epicID, epicID, goal)
}

// SetSize sets the modal dimensions.
func (m *NewWorkflowModal) SetSize(width, height int) *NewWorkflowModal {
	m.form = m.form.SetSize(width, height)
	return m
}

// Init initializes the modal.
func (m *NewWorkflowModal) Init() tea.Cmd {
	return m.form.Init()
}

// Update handles messages for the modal.
func (m *NewWorkflowModal) Update(msg tea.Msg) (*NewWorkflowModal, tea.Cmd) {
	var cmd tea.Cmd
	m.form, cmd = m.form.Update(msg)
	return m, cmd
}

// View renders the modal.
func (m *NewWorkflowModal) View() string {
	return m.form.View()
}

// Overlay renders the modal on top of a background view.
func (m *NewWorkflowModal) Overlay(background string) string {
	return m.form.Overlay(background)
}
