// Package dashboard implements the multi-workflow dashboard TUI mode.
package dashboard

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	appgit "github.com/zjrosen/perles/internal/git/application"
	"github.com/zjrosen/perles/internal/orchestration/controlplane"
	appreg "github.com/zjrosen/perles/internal/registry/application"
	"github.com/zjrosen/perles/internal/registry/domain"
	"github.com/zjrosen/perles/internal/ui/shared/formmodal"
)

// argFieldPrefix is the prefix for argument field keys in form values.
const argFieldPrefix = "arg_"

// NewWorkflowModal holds the state for the new workflow creation modal.
type NewWorkflowModal struct {
	form            formmodal.Model
	registryService *appreg.RegistryService // Registry for template listing, validation, and epic_driven.md access
	controlPlane    controlplane.ControlPlane
	gitExecutor     appgit.GitExecutor
	workflowCreator *appreg.WorkflowCreator
	worktreeEnabled bool // track if worktree options are available

	// templateArgs maps template key → slice of arguments for that template.
	// Used to validate required arguments and build TemplateContext.Args on submit.
	templateArgs map[string][]*registry.Argument

	// Spinner animation state (for loading indicator)
	spinnerFrame int
}

// spinnerFrames defines the braille spinner animation sequence.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// spinnerTickMsg advances the spinner frame during submission.
type spinnerTickMsg struct{}

// spinnerTick returns a command that sends spinnerTickMsg after 80ms.
func spinnerTick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

// startSubmitMsg signals the modal to begin async workflow creation.
type startSubmitMsg struct {
	values map[string]any
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
		templateArgs:    make(map[string][]*registry.Argument),
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
	}

	// Add argument fields for all templates (visibility controlled by VisibleWhen)
	argFields := m.buildArgumentFields(registryService)
	fields = append(fields, argFields...)

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

// buildArgumentFields creates form fields for all template arguments.
// Each field uses VisibleWhen to only show when its template is selected.
// Also populates m.templateArgs for validation and submission.
func (m *NewWorkflowModal) buildArgumentFields(registryService *appreg.RegistryService) []formmodal.FieldConfig {
	if registryService == nil {
		return nil
	}

	var fields []formmodal.FieldConfig
	registrations := registryService.GetByNamespace("workflow")

	for _, reg := range registrations {
		args := reg.Arguments()
		if len(args) == 0 {
			continue
		}

		// Store arguments for this template (used in validate and submit)
		m.templateArgs[reg.Key()] = args

		// Create a field for each argument
		for _, arg := range args {
			// Capture templateKey for closure
			templateKey := reg.Key()

			// Build visibility function: show only when this template is selected
			visibleWhen := func(values map[string]any) bool {
				selected, _ := values["template"].(string)
				return selected == templateKey
			}

			// Map ArgumentType to formmodal.FieldType
			field := formmodal.FieldConfig{
				Key:          argFieldPrefix + arg.Key(),
				Label:        arg.Label(),
				Placeholder:  arg.Description(),
				InitialValue: arg.DefaultValue(),
				VisibleWhen:  visibleWhen,
			}

			// Set hint based on required status
			if arg.Required() {
				field.Hint = "required"
			} else {
				field.Hint = "optional"
			}

			// Map argument type to field type
			switch arg.Type() {
			case registry.ArgumentTypeText, registry.ArgumentTypeNumber:
				field.Type = formmodal.FieldTypeText
			case registry.ArgumentTypeTextarea:
				field.Type = formmodal.FieldTypeTextArea
				field.MaxHeight = 4
				field.VimEnabled = true
			case registry.ArgumentTypeSelect:
				field.Type = formmodal.FieldTypeSelect
				field.Options = buildSelectOptions(arg.Options(), arg.DefaultValue())
			case registry.ArgumentTypeMultiSelect:
				field.Type = formmodal.FieldTypeList
				field.MultiSelect = true
				field.Options = buildSelectOptions(arg.Options(), arg.DefaultValue())
			default:
				field.Type = formmodal.FieldTypeText
			}

			fields = append(fields, field)
		}
	}

	return fields
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

// buildSelectOptions converts argument options to formmodal ListOptions.
// If defaultValue matches an option, that option is marked as selected.
func buildSelectOptions(options []string, defaultValue string) []formmodal.ListOption {
	listOptions := make([]formmodal.ListOption, len(options))
	for i, opt := range options {
		listOptions[i] = formmodal.ListOption{
			Label:    opt,
			Value:    opt,
			Selected: opt == defaultValue || (defaultValue == "" && i == 0),
		}
	}
	return listOptions
}

// buildTemplateOptions converts domain registry registrations to list options.
// Uses GetByNamespace("workflow") to get only workflow templates (not language guidelines).
func buildTemplateOptions(registryService *appreg.RegistryService) []formmodal.ListOption {
	if registryService == nil {
		return []formmodal.ListOption{}
	}

	// Get workflow registrations (workflow templates, not language guidelines)
	registrations := registryService.GetByNamespace("workflow")

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
		if _, err := m.registryService.GetByKey("workflow", templateKey); err != nil {
			return errors.New("selected template not found")
		}
	}

	// Validate required arguments for the selected template
	if args, hasArgs := m.templateArgs[templateKey]; hasArgs {
		for _, arg := range args {
			if arg.Required() {
				fieldKey := argFieldPrefix + arg.Key()
				// Handle both string (text/select) and []string (multi-select) values
				switch v := values[fieldKey].(type) {
				case string:
					if strings.TrimSpace(v) == "" {
						return fmt.Errorf("%s is required", arg.Label())
					}
				case []string:
					if len(v) == 0 {
						return fmt.Errorf("%s is required", arg.Label())
					}
				default:
					return fmt.Errorf("%s is required", arg.Label())
				}
			}
		}
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

// onSubmit is called when the form is validated and ready for submission.
// Returns a message to trigger async workflow creation (to avoid blocking UI).
func (m *NewWorkflowModal) onSubmit(values map[string]any) tea.Msg {
	// Return a message that will trigger async creation
	return startSubmitMsg{values: values}
}

// createWorkflowAsync performs the actual workflow creation.
// This runs as a tea.Cmd to avoid blocking the UI.
func (m *NewWorkflowModal) createWorkflowAsync(values map[string]any) tea.Cmd {
	return func() tea.Msg {
		templateID := values["template"].(string)
		name := values["name"].(string)

		// Extract argument values for the selected template
		args := m.extractArgumentValues(templateID, values)

		var epicID string
		var initialPrompt string

		// If WorkflowCreator is available, create epic + tasks in beads first
		if m.workflowCreator != nil {
			// Use name as feature slug, or derive from templateID if empty
			feature := name
			if feature == "" {
				feature = templateID
			}

			result, err := m.workflowCreator.CreateWithArgs(feature, templateID, args)
			if err != nil {
				return ErrorMsg{Err: fmt.Errorf("create epic: %w", err)}
			}

			epicID = result.Epic.ID

			// Build coordinator prompt: instructions template + epic ID section + args
			initialPrompt = m.buildCoordinatorPrompt(templateID, epicID, args)
		} else {
			// No WorkflowCreator, use goal from args if present, otherwise empty
			initialPrompt = args["goal"]
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
			return ErrorMsg{Err: fmt.Errorf("create workflow: %w", err)}
		}

		return CreateWorkflowMsg{
			WorkflowID: workflowID,
			Name:       spec.Name,
		}
	}
}

// extractArgumentValues extracts argument values from form values for the selected template.
// Returns a map of argument key → value (without the argFieldPrefix).
func (m *NewWorkflowModal) extractArgumentValues(templateKey string, values map[string]any) map[string]string {
	args := make(map[string]string)

	// Get arguments for this template
	templateArgs, hasArgs := m.templateArgs[templateKey]
	if !hasArgs {
		return args
	}

	// Extract values for each argument
	for _, arg := range templateArgs {
		fieldKey := argFieldPrefix + arg.Key()
		// Handle both string (text/select) and []string (multi-select) values
		switch v := values[fieldKey].(type) {
		case string:
			if v != "" {
				args[arg.Key()] = v
			} else if arg.DefaultValue() != "" {
				args[arg.Key()] = arg.DefaultValue()
			}
		case []string:
			if len(v) > 0 {
				args[arg.Key()] = strings.Join(v, ", ")
			} else if arg.DefaultValue() != "" {
				args[arg.Key()] = arg.DefaultValue()
			}
		default:
			if arg.DefaultValue() != "" {
				args[arg.Key()] = arg.DefaultValue()
			}
		}
	}

	return args
}

// buildCoordinatorPrompt assembles the coordinator prompt from:
// 1. Instructions template content (from registration's instructions field)
// 2. Epic ID section (so coordinator can read detailed instructions via bd show)
// 3. Argument values (includes goal if defined as a template argument)
func (m *NewWorkflowModal) buildCoordinatorPrompt(templateID, epicID string, args map[string]string) string {
	// Load instructions template if registry service is available
	var instructionsContent string
	if m.registryService != nil {
		// Get the registration for this template
		reg, err := m.registryService.GetByKey("workflow", templateID)
		if err == nil {
			content, err := m.registryService.GetInstructionsTemplate(reg)
			if err == nil {
				instructionsContent = content
			}
		}
		// If error loading template, continue without it
	}

	// Build argument section if any args are present
	var argsSection string
	if len(args) > 0 {
		var sb strings.Builder
		sb.WriteString("\n# Arguments\n\n")
		for key, val := range args {
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", key, val))
		}
		argsSection = sb.String()
	}

	// Build the full prompt
	if instructionsContent != "" {
		return fmt.Sprintf(`%s

---

# Your Epic

Epic ID: %s

Use `+"`bd show %s --json`"+` to read your detailed workflow instructions.
%s`, instructionsContent, epicID, epicID, argsSection)
	}

	// Fallback if no instructions template available
	return fmt.Sprintf(`# Your Epic

Epic ID: %s

Use `+"`bd show %s --json`"+` to read your detailed workflow instructions.
%s`, epicID, epicID, argsSection)
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
	switch msg := msg.(type) {
	case startSubmitMsg:
		// Start async workflow creation with loading indicator
		m.spinnerFrame = 0
		m.form = m.form.SetLoading(spinnerFrames[0] + " Creating workflow...")
		return m, tea.Batch(spinnerTick(), m.createWorkflowAsync(msg.values))

	case spinnerTickMsg:
		// Advance spinner animation while loading
		if m.form.IsLoading() {
			m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerFrames)
			m.form = m.form.SetLoading(spinnerFrames[m.spinnerFrame] + " Creating workflow...")
			return m, spinnerTick()
		}
		return m, nil

	case ErrorMsg:
		// Clear loading state and show error in the form
		m.form = m.form.SetLoading("").SetError(msg.Err.Error())
		return m, nil

	case CreateWorkflowMsg:
		// Clear loading state on success (message will bubble up)
		m.form = m.form.SetLoading("")
		return m, nil
	}

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
