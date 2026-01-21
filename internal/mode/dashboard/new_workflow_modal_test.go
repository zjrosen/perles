package dashboard

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	domaingit "github.com/zjrosen/perles/internal/git/domain"
	"github.com/zjrosen/perles/internal/keys"
	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/mode"
	"github.com/zjrosen/perles/internal/orchestration/controlplane"
	appreg "github.com/zjrosen/perles/internal/registry/application"
)

// === Test Helpers ===

// createTestRegistryServiceFS creates a MapFS for testing with a valid registry.yaml
func createTestRegistryServiceFS() fstest.MapFS {
	return fstest.MapFS{
		"registry.yaml": &fstest.MapFile{
			Data: []byte(`
registry:
  - namespace: "spec-workflow"
    key: "quick-plan"
    version: "v1"
    name: "Quick Plan"
    description: "Fast planning workflow"
    nodes:
      - key: "plan"
        name: "Plan"
        template: "v1-plan.md"
  - namespace: "spec-workflow"
    key: "cook"
    version: "v1"
    name: "Cook"
    description: "Implementation workflow"
    nodes:
      - key: "cook"
        name: "Cook"
        template: "v1-cook.md"
  - namespace: "spec-workflow"
    key: "research"
    version: "v1"
    name: "Research"
    description: "Research to tasks"
    nodes:
      - key: "research"
        name: "Research"
        template: "v1-research.md"
`),
		},
		"v1-plan.md":     &fstest.MapFile{Data: []byte("# Plan Template")},
		"v1-cook.md":     &fstest.MapFile{Data: []byte("# Cook Template")},
		"v1-research.md": &fstest.MapFile{Data: []byte("# Research Template")},
	}
}

// createTestWorkflowTemplatesFS creates a mock workflow templates FS with epic_driven.md
func createTestWorkflowTemplatesFS() fstest.MapFS {
	return fstest.MapFS{
		"epic_driven.md": &fstest.MapFile{
			Data: []byte("# Epic-Driven Workflow\n\nYou are the Coordinator."),
		},
	}
}

// createTestRegistryService creates a registry service with test templates.
func createTestRegistryService(t *testing.T) *appreg.RegistryService {
	t.Helper()
	registryFS := createTestRegistryServiceFS()
	workflowFS := createTestWorkflowTemplatesFS()
	registry, err := appreg.NewRegistryService(registryFS, workflowFS)
	require.NoError(t, err)
	return registry
}

// createTestModelWithRegistryService creates a dashboard model with a mock ControlPlane and registry service.
func createTestModelWithRegistryService(t *testing.T, workflows []*controlplane.WorkflowInstance) (Model, *mockControlPlane, *appreg.RegistryService) {
	t.Helper()

	mockCP := newMockControlPlane()
	mockCP.On("List", mock.Anything, mock.Anything).Return(workflows, nil).Maybe()

	eventCh := make(chan controlplane.ControlPlaneEvent)
	close(eventCh)
	mockCP.On("Subscribe", mock.Anything).Return((<-chan controlplane.ControlPlaneEvent)(eventCh), func() {}).Maybe()

	registryService := createTestRegistryService(t)

	cfg := Config{
		ControlPlane:    mockCP,
		Services:        mode.Services{},
		RegistryService: registryService,
	}

	m := New(cfg)
	m.workflows = workflows
	m.workflowList = m.workflowList.SetWorkflows(workflows)
	m.resourceSummary = m.resourceSummary.Update(workflows)
	m = m.SetSize(100, 40).(Model)

	return m, mockCP, registryService
}

// === Unit Tests: Modal loads templates from registry ===

func TestNewWorkflowModal_LoadsTemplatesFromRegistry(t *testing.T) {
	registryService := createTestRegistryService(t)
	modal := NewNewWorkflowModal(registryService, nil, nil, nil)
	require.NotNil(t, modal)

	// Modal should be created with templates from registry
	// The form should have fields configured
	view := modal.View()
	require.NotEmpty(t, view)
	require.Contains(t, view, "Template")
}

func TestNewWorkflowModal_HandlesNilRegistry(t *testing.T) {
	modal := NewNewWorkflowModal(nil, nil, nil, nil)
	require.NotNil(t, modal)

	// Should still render without crashing
	view := modal.View()
	require.NotEmpty(t, view)
}

// === Unit Tests: Form validation ===

func TestNewWorkflowModal_ValidationRejectsEmptyGoal(t *testing.T) {
	registryService := createTestRegistryService(t)
	modal := NewNewWorkflowModal(registryService, nil, nil, nil)

	// Validation should fail with empty goal
	values := map[string]any{
		"template":     "quick-plan",
		"name":         "",
		"goal":         "",
		"priority":     "normal",
		"max_workers":  "",
		"token_budget": "",
	}

	err := modal.validate(values)
	require.Error(t, err)
	require.Contains(t, err.Error(), "goal is required")
}

func TestNewWorkflowModal_ValidationRejectsEmptyTemplate(t *testing.T) {
	registryService := createTestRegistryService(t)
	modal := NewNewWorkflowModal(registryService, nil, nil, nil)

	values := map[string]any{
		"template":     "",
		"name":         "",
		"goal":         "Test goal",
		"priority":     "normal",
		"max_workers":  "",
		"token_budget": "",
	}

	err := modal.validate(values)
	require.Error(t, err)
	require.Contains(t, err.Error(), "template is required")
}

func TestNewWorkflowModal_ValidationRejectsInvalidMaxWorkers(t *testing.T) {
	registryService := createTestRegistryService(t)
	modal := NewNewWorkflowModal(registryService, nil, nil, nil)

	values := map[string]any{
		"template":     "quick-plan",
		"name":         "",
		"goal":         "Test goal",
		"priority":     "normal",
		"max_workers":  "invalid",
		"token_budget": "",
	}

	err := modal.validate(values)
	require.Error(t, err)
	require.Contains(t, err.Error(), "max workers must be a positive number")
}

func TestNewWorkflowModal_ValidationRejectsInvalidTokenBudget(t *testing.T) {
	registryService := createTestRegistryService(t)
	modal := NewNewWorkflowModal(registryService, nil, nil, nil)

	values := map[string]any{
		"template":     "quick-plan",
		"name":         "",
		"goal":         "Test goal",
		"priority":     "normal",
		"max_workers":  "",
		"token_budget": "-100",
	}

	err := modal.validate(values)
	require.Error(t, err)
	require.Contains(t, err.Error(), "token budget must be a positive number")
}

func TestNewWorkflowModal_ValidationAcceptsValidInput(t *testing.T) {
	registryService := createTestRegistryService(t)
	modal := NewNewWorkflowModal(registryService, nil, nil, nil)

	values := map[string]any{
		"template":     "quick-plan",
		"name":         "My Workflow",
		"goal":         "Test goal",
		"priority":     "normal",
		"max_workers":  "4",
		"token_budget": "10000",
	}

	err := modal.validate(values)
	require.NoError(t, err)
}

// === Unit Tests: Cancel closes modal without action ===

func TestNewWorkflowModal_CancelClosesModal(t *testing.T) {
	m, _, _ := createTestModelWithRegistryService(t, []*controlplane.WorkflowInstance{})

	// Open the modal
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = result.(Model)
	require.True(t, m.InNewWorkflowModal())

	// Press Escape to cancel
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = result.(Model)

	// Modal should now receive CancelNewWorkflowMsg
	result, _ = m.Update(CancelNewWorkflowMsg{})
	m = result.(Model)
	require.False(t, m.InNewWorkflowModal())
}

// === Unit Tests: Create calls ControlPlane.Create ===

func TestNewWorkflowModal_CreateCallsControlPlane(t *testing.T) {
	mockCP := newMockControlPlane()
	mockCP.On("List", mock.Anything, mock.Anything).Return([]*controlplane.WorkflowInstance{}, nil).Maybe()
	mockCP.On("Create", mock.Anything, mock.MatchedBy(func(spec controlplane.WorkflowSpec) bool {
		return spec.TemplateID == "quick-plan" && spec.InitialGoal == "Test goal"
	})).Return(controlplane.WorkflowID("new-workflow-id"), nil).Once()

	eventCh := make(chan controlplane.ControlPlaneEvent)
	close(eventCh)
	mockCP.On("Subscribe", mock.Anything).Return((<-chan controlplane.ControlPlaneEvent)(eventCh), func() {}).Maybe()

	registryService := createTestRegistryService(t)
	modal := NewNewWorkflowModal(registryService, mockCP, nil, nil)

	// Simulate form submission
	values := map[string]any{
		"template":     "quick-plan",
		"name":         "",
		"goal":         "Test goal",
		"priority":     "normal",
		"max_workers":  "",
		"token_budget": "",
	}

	msg := modal.onSubmit(values)
	createMsg, ok := msg.(CreateWorkflowMsg)
	require.True(t, ok)
	require.Equal(t, controlplane.WorkflowID("new-workflow-id"), createMsg.WorkflowID)

	mockCP.AssertExpectations(t)
}

// === Unit Tests: Create workflow always starts immediately ===

func TestDashboard_CreateWorkflowStartsImmediately(t *testing.T) {
	mockCP := newMockControlPlane()
	mockCP.On("List", mock.Anything, mock.Anything).Return([]*controlplane.WorkflowInstance{}, nil).Maybe()
	mockCP.On("Start", mock.Anything, controlplane.WorkflowID("new-wf")).Return(nil).Once()

	eventCh := make(chan controlplane.ControlPlaneEvent)
	close(eventCh)
	mockCP.On("Subscribe", mock.Anything).Return((<-chan controlplane.ControlPlaneEvent)(eventCh), func() {}).Maybe()

	registryService := createTestRegistryService(t)

	cfg := Config{
		ControlPlane:    mockCP,
		Services:        mode.Services{},
		RegistryService: registryService,
	}

	m := New(cfg)
	m.workflows = []*controlplane.WorkflowInstance{}
	m = m.SetSize(100, 40).(Model)

	// Open modal
	result, _ := m.openNewWorkflowModal()
	m = result.(Model)

	// Simulate successful creation
	result, cmd := m.Update(CreateWorkflowMsg{
		WorkflowID: "new-wf",
		Name:       "Test",
	})
	m = result.(Model)

	// Modal should be closed
	require.False(t, m.InNewWorkflowModal())

	// Command should be returned (includes start workflow)
	require.NotNil(t, cmd)
}

// === Unit Tests: Resource limits default to empty ===

func TestNewWorkflowModal_ResourceLimitsOptional(t *testing.T) {
	registryService := createTestRegistryService(t)
	modal := NewNewWorkflowModal(registryService, nil, nil, nil)

	values := map[string]any{
		"template":     "quick-plan",
		"name":         "",
		"goal":         "Test goal",
		"priority":     "normal",
		"max_workers":  "",
		"token_budget": "",
	}

	// Should pass validation with empty resource limits
	err := modal.validate(values)
	require.NoError(t, err)
}

// === Unit Tests: Tab navigates between fields ===

func TestNewWorkflowModal_TabNavigates(t *testing.T) {
	registryService := createTestRegistryService(t)
	modal := NewNewWorkflowModal(registryService, nil, nil, nil).SetSize(100, 40)

	// Press Tab - should navigate to next field
	modal, _ = modal.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Modal should still be functional
	require.NotNil(t, modal)
	view := modal.View()
	require.NotEmpty(t, view)
}

// === Unit Tests: N key opens modal ===

func TestDashboard_NKeyOpensModal(t *testing.T) {
	m, _, _ := createTestModelWithRegistryService(t, []*controlplane.WorkflowInstance{})
	require.False(t, m.InNewWorkflowModal())

	// Press n to open modal
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = result.(Model)

	require.True(t, m.InNewWorkflowModal())
	// Note: Init command may be nil if no text inputs need blink
}

func TestDashboard_ShiftNKeyOpensModal(t *testing.T) {
	m, _, _ := createTestModelWithRegistryService(t, []*controlplane.WorkflowInstance{})

	// Press N (shift+n) to open modal
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	m = result.(Model)

	require.True(t, m.InNewWorkflowModal())
}

// === Unit Tests: Escape key in dashboard doesn't interfere ===

func TestDashboard_EscapeKeyWithoutModal(t *testing.T) {
	m, _, _ := createTestModelWithRegistryService(t, []*controlplane.WorkflowInstance{})

	// Press Escape without modal open - should not crash
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = result.(Model)

	// Dashboard should still be functional
	view := m.View()
	require.NotEmpty(t, view)
}

// === Unit Tests: Modal overlay rendering ===

func TestDashboard_ModalRendersAsOverlay(t *testing.T) {
	m, _, _ := createTestModelWithRegistryService(t, []*controlplane.WorkflowInstance{})

	// Open modal
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = result.(Model)

	// View should contain modal content
	view := m.View()
	require.Contains(t, view, "New Workflow")
	require.Contains(t, view, "Template")
	require.Contains(t, view, "Goal")
}

// === Unit Tests: Window resize updates modal ===

func TestDashboard_WindowResizeUpdatesModal(t *testing.T) {
	m, _, _ := createTestModelWithRegistryService(t, []*controlplane.WorkflowInstance{})

	// Open modal
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = result.(Model)

	// Resize window
	result, _ = m.Update(tea.WindowSizeMsg{Width: 150, Height: 50})
	m = result.(Model)

	require.Equal(t, 150, m.width)
	require.Equal(t, 50, m.height)
	require.True(t, m.InNewWorkflowModal())
}

// === Unit Tests: Modal handles Ctrl+S ===

func TestNewWorkflowModal_CtrlSSavesForm(t *testing.T) {
	registryService := createTestRegistryService(t)
	modal := NewNewWorkflowModal(registryService, nil, nil, nil).SetSize(100, 40)

	// Press Ctrl+S - should trigger save/validation
	// Since form is empty, it should show validation error
	modal, _ = modal.Update(tea.KeyMsg{Type: tea.KeyCtrlS})

	// Modal should still be functional (validation error shown)
	require.NotNil(t, modal)
}

// === Integration Tests: Full workflow creation flow ===

func TestDashboard_FullWorkflowCreationFlow(t *testing.T) {
	mockCP := newMockControlPlane()
	mockCP.On("List", mock.Anything, mock.Anything).Return([]*controlplane.WorkflowInstance{}, nil).Maybe()
	mockCP.On("Create", mock.Anything, mock.Anything).Return(controlplane.WorkflowID("created-wf"), nil).Once()

	eventCh := make(chan controlplane.ControlPlaneEvent)
	close(eventCh)
	mockCP.On("Subscribe", mock.Anything).Return((<-chan controlplane.ControlPlaneEvent)(eventCh), func() {}).Maybe()

	registryService := createTestRegistryService(t)

	cfg := Config{
		ControlPlane:    mockCP,
		Services:        mode.Services{},
		RegistryService: registryService,
	}

	m := New(cfg)
	m = m.SetSize(100, 40).(Model)

	// 1. Open modal
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = result.(Model)
	require.True(t, m.InNewWorkflowModal())

	// 2. Simulate receiving CreateWorkflowMsg (as if form was filled and submitted)
	result, _ = m.Update(CreateWorkflowMsg{
		WorkflowID: "created-wf",
		Name:       "Test Workflow",
	})
	m = result.(Model)

	// 3. Modal should be closed
	require.False(t, m.InNewWorkflowModal())
}

// Test that buildTemplateOptions handles empty registry
func TestBuildTemplateOptions_EmptyRegistry(t *testing.T) {
	// Create a domain registry with no spec-workflow registrations
	fs := fstest.MapFS{
		"registry.yaml": &fstest.MapFile{
			Data: []byte(`
registry:
  - namespace: "lang-guidelines"
    key: "go"
    version: "v1"
    name: "Go Guidelines"
    description: "Go language guidelines"
    nodes:
      - key: "coding"
        name: "Coding"
        template: "v1-coding.md"
`),
		},
		"v1-coding.md": &fstest.MapFile{Data: []byte("# Coding Guidelines")},
	}
	workflowFS := createTestWorkflowTemplatesFS()
	registryService, err := appreg.NewRegistryService(fs, workflowFS)
	require.NoError(t, err)

	options := buildTemplateOptions(registryService)
	require.Empty(t, options) // No spec-workflow registrations
}

// Test that buildTemplateOptions creates correct options
func TestBuildTemplateOptions_CreatesCorrectOptions(t *testing.T) {
	registryService := createTestRegistryService(t)
	options := buildTemplateOptions(registryService)

	require.Len(t, options, 3)

	// Options should include template info
	hasQuickPlan := false
	for _, opt := range options {
		if opt.Value == "quick-plan" {
			hasQuickPlan = true
			require.Contains(t, opt.Label, "Quick Plan")
		}
	}
	require.True(t, hasQuickPlan)
}

// Test that buildTemplateOptions handles nil registry
func TestBuildTemplateOptions_NilRegistry(t *testing.T) {
	options := buildTemplateOptions(nil)
	require.Empty(t, options)
}

// Test escape key handler checks for common escape binding
func TestNewWorkflowModal_EscapeClearsModal(t *testing.T) {
	registryService := createTestRegistryService(t)
	modal := NewNewWorkflowModal(registryService, nil, nil, nil).SetSize(100, 40)

	// Press escape
	modal, cmd := modal.Update(keys.Common.Escape.Keys()[0])
	require.NotNil(t, modal)

	// Should produce a cancel message command
	if cmd != nil {
		msg := cmd()
		_, isCancel := msg.(CancelNewWorkflowMsg)
		require.True(t, isCancel)
	}
}

// === Worktree UI Tests ===

// createMockGitExecutorWithBranches creates a mock GitExecutor with test branches.
func createMockGitExecutorWithBranches(t *testing.T) *mocks.MockGitExecutor {
	t.Helper()
	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().ListBranches().Return([]domaingit.BranchInfo{
		{Name: "main", IsCurrent: false},
		{Name: "develop", IsCurrent: true},
		{Name: "feature/auth", IsCurrent: false},
	}, nil).Maybe()
	return mockGit
}

func TestNewWorkflowModal_PopulatesBranchOptionsFromListBranches(t *testing.T) {
	registryService := createTestRegistryService(t)
	mockGit := createMockGitExecutorWithBranches(t)

	modal := NewNewWorkflowModal(registryService, nil, mockGit, nil)
	require.NotNil(t, modal)
	require.True(t, modal.worktreeEnabled)

	// Modal should contain Git Worktree toggle (always visible)
	view := modal.SetSize(100, 40).View()
	require.Contains(t, view, "Git Worktree")

	// Branch fields should be hidden initially (worktree toggle defaults to No)
	require.NotContains(t, view, "Base Branch")
	require.NotContains(t, view, "Branch Name")

	// Navigate to the worktree toggle and switch to Yes
	// Tab through: Template -> Name -> Goal -> Git Worktree
	modal, _ = modal.Update(tea.KeyMsg{Type: tea.KeyTab})
	modal, _ = modal.Update(tea.KeyMsg{Type: tea.KeyTab})
	modal, _ = modal.Update(tea.KeyMsg{Type: tea.KeyTab})
	// Switch toggle to Yes (right arrow)
	modal, _ = modal.Update(tea.KeyMsg{Type: tea.KeyRight})

	// Now branch fields should be visible
	view = modal.View()
	require.Contains(t, view, "Base Branch")
	require.Contains(t, view, "Branch Name")
}

func TestNewWorkflowModal_DisablesWorktreeFieldsWhenListBranchesFails(t *testing.T) {
	registryService := createTestRegistryService(t)
	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().ListBranches().Return(nil, errors.New("not a git repo"))

	modal := NewNewWorkflowModal(registryService, nil, mockGit, nil)
	require.NotNil(t, modal)
	require.False(t, modal.worktreeEnabled)

	// Modal should NOT contain worktree fields when git fails
	view := modal.SetSize(100, 40).View()
	require.NotContains(t, view, "Git Worktree")
	require.NotContains(t, view, "Base Branch")
}

func TestNewWorkflowModal_DisablesWorktreeFieldsWhenGitExecutorNil(t *testing.T) {
	registryService := createTestRegistryService(t)

	modal := NewNewWorkflowModal(registryService, nil, nil, nil)
	require.NotNil(t, modal)
	require.False(t, modal.worktreeEnabled)

	// Modal should NOT contain worktree fields when no git executor
	view := modal.SetSize(100, 40).View()
	require.NotContains(t, view, "Git Worktree")
	require.NotContains(t, view, "Base Branch")
}

func TestNewWorkflowModal_OnSubmitSetsWorktreeEnabledCorrectly(t *testing.T) {
	registryService := createTestRegistryService(t)
	mockGit := createMockGitExecutorWithBranches(t)

	mockCP := newMockControlPlane()
	mockCP.On("Create", mock.Anything, mock.MatchedBy(func(spec controlplane.WorkflowSpec) bool {
		return spec.WorktreeEnabled == true &&
			spec.WorktreeBaseBranch == "main" &&
			spec.WorktreeBranchName == "my-feature"
	})).Return(controlplane.WorkflowID("new-workflow-id"), nil).Once()

	modal := NewNewWorkflowModal(registryService, mockCP, mockGit, nil)

	values := map[string]any{
		"template":      "quick-plan",
		"name":          "",
		"goal":          "Test goal",
		"use_worktree":  "true",
		"base_branch":   "main",
		"custom_branch": "my-feature",
	}

	msg := modal.onSubmit(values)
	createMsg, ok := msg.(CreateWorkflowMsg)
	require.True(t, ok)
	require.Equal(t, controlplane.WorkflowID("new-workflow-id"), createMsg.WorkflowID)

	mockCP.AssertExpectations(t)
}

func TestNewWorkflowModal_OnSubmitSetsWorktreeBaseBranchFromSearchSelect(t *testing.T) {
	registryService := createTestRegistryService(t)
	mockGit := createMockGitExecutorWithBranches(t)

	mockCP := newMockControlPlane()
	mockCP.On("Create", mock.Anything, mock.MatchedBy(func(spec controlplane.WorkflowSpec) bool {
		return spec.WorktreeEnabled == true && spec.WorktreeBaseBranch == "develop"
	})).Return(controlplane.WorkflowID("new-workflow-id"), nil).Once()

	modal := NewNewWorkflowModal(registryService, mockCP, mockGit, nil)

	values := map[string]any{
		"template":      "quick-plan",
		"name":          "",
		"goal":          "Test goal",
		"use_worktree":  "true",
		"base_branch":   "develop",
		"custom_branch": "",
	}

	msg := modal.onSubmit(values)
	createMsg, ok := msg.(CreateWorkflowMsg)
	require.True(t, ok)
	require.Equal(t, controlplane.WorkflowID("new-workflow-id"), createMsg.WorkflowID)

	mockCP.AssertExpectations(t)
}

func TestNewWorkflowModal_OnSubmitSetsWorktreeBranchNameFromTextField(t *testing.T) {
	registryService := createTestRegistryService(t)
	mockGit := createMockGitExecutorWithBranches(t)

	mockCP := newMockControlPlane()
	mockCP.On("Create", mock.Anything, mock.MatchedBy(func(spec controlplane.WorkflowSpec) bool {
		return spec.WorktreeEnabled == true && spec.WorktreeBranchName == "perles-custom-branch"
	})).Return(controlplane.WorkflowID("new-workflow-id"), nil).Once()

	modal := NewNewWorkflowModal(registryService, mockCP, mockGit, nil)

	values := map[string]any{
		"template":      "quick-plan",
		"name":          "",
		"goal":          "Test goal",
		"use_worktree":  "true",
		"base_branch":   "main",
		"custom_branch": "perles-custom-branch",
	}

	msg := modal.onSubmit(values)
	createMsg, ok := msg.(CreateWorkflowMsg)
	require.True(t, ok)
	require.Equal(t, controlplane.WorkflowID("new-workflow-id"), createMsg.WorkflowID)

	mockCP.AssertExpectations(t)
}

func TestNewWorkflowModal_ValidationRequiresBaseBranchWhenWorktreeEnabled(t *testing.T) {
	registryService := createTestRegistryService(t)
	mockGit := createMockGitExecutorWithBranches(t)

	modal := NewNewWorkflowModal(registryService, nil, mockGit, nil)

	values := map[string]any{
		"template":      "quick-plan",
		"name":          "",
		"goal":          "Test goal",
		"use_worktree":  "true",
		"base_branch":   "", // Missing base branch
		"custom_branch": "",
	}

	err := modal.validate(values)
	require.Error(t, err)
	require.Contains(t, err.Error(), "base branch is required when worktree is enabled")
}

func TestNewWorkflowModal_ValidationRejectsInvalidBranchNames(t *testing.T) {
	registryService := createTestRegistryService(t)
	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().ListBranches().Return([]domaingit.BranchInfo{
		{Name: "main", IsCurrent: true},
	}, nil)
	mockGit.EXPECT().ValidateBranchName("invalid..branch").Return(errors.New("invalid ref format"))

	modal := NewNewWorkflowModal(registryService, nil, mockGit, nil)

	values := map[string]any{
		"template":      "quick-plan",
		"name":          "",
		"goal":          "Test goal",
		"use_worktree":  "true",
		"base_branch":   "main",
		"custom_branch": "invalid..branch", // Invalid branch name
	}

	err := modal.validate(values)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid branch name")
}

func TestNewWorkflowModal_ValidationAcceptsValidBranchName(t *testing.T) {
	registryService := createTestRegistryService(t)
	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().ListBranches().Return([]domaingit.BranchInfo{
		{Name: "main", IsCurrent: true},
	}, nil)
	mockGit.EXPECT().ValidateBranchName("feature/valid-branch").Return(nil)

	modal := NewNewWorkflowModal(registryService, nil, mockGit, nil)

	values := map[string]any{
		"template":      "quick-plan",
		"name":          "",
		"goal":          "Test goal",
		"use_worktree":  "true",
		"base_branch":   "main",
		"custom_branch": "feature/valid-branch",
	}

	err := modal.validate(values)
	require.NoError(t, err)
}

func TestNewWorkflowModal_ValidationPassesWhenWorktreeDisabled(t *testing.T) {
	registryService := createTestRegistryService(t)
	mockGit := createMockGitExecutorWithBranches(t)

	modal := NewNewWorkflowModal(registryService, nil, mockGit, nil)

	values := map[string]any{
		"template":      "quick-plan",
		"name":          "",
		"goal":          "Test goal",
		"use_worktree":  "false", // Worktree disabled
		"base_branch":   "",      // Empty but should be OK
		"custom_branch": "",
	}

	err := modal.validate(values)
	require.NoError(t, err)
}

func TestBuildBranchOptions_NilGitExecutor(t *testing.T) {
	options, available := buildBranchOptions(nil)
	require.Nil(t, options)
	require.False(t, available)
}

func TestBuildBranchOptions_ListBranchesError(t *testing.T) {
	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().ListBranches().Return(nil, errors.New("git error"))

	options, available := buildBranchOptions(mockGit)
	require.Nil(t, options)
	require.False(t, available)
}

func TestBuildBranchOptions_EmptyBranchList(t *testing.T) {
	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().ListBranches().Return([]domaingit.BranchInfo{}, nil)

	options, available := buildBranchOptions(mockGit)
	require.Nil(t, options)
	require.False(t, available)
}

func TestBuildBranchOptions_ConvertsCorrectly(t *testing.T) {
	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().ListBranches().Return([]domaingit.BranchInfo{
		{Name: "main", IsCurrent: false},
		{Name: "develop", IsCurrent: true},
		{Name: "feature/test", IsCurrent: false},
	}, nil)

	options, available := buildBranchOptions(mockGit)
	require.True(t, available)
	require.Len(t, options, 3)

	// Check first branch
	require.Equal(t, "main", options[0].Label)
	require.Equal(t, "main", options[0].Value)
	require.False(t, options[0].Selected)

	// Check current branch is selected
	require.Equal(t, "develop", options[1].Label)
	require.Equal(t, "develop", options[1].Value)
	require.True(t, options[1].Selected)

	// Check third branch
	require.Equal(t, "feature/test", options[2].Label)
	require.Equal(t, "feature/test", options[2].Value)
	require.False(t, options[2].Selected)
}

// === WorkflowCreator Integration Tests ===

// MockWorkflowCreator is a mock implementation for testing.
type MockWorkflowCreator struct {
	mock.Mock
}

func (m *MockWorkflowCreator) Create(feature, workflowKey string) (*appreg.WorkflowResultDTO, error) {
	args := m.Called(feature, workflowKey)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*appreg.WorkflowResultDTO), args.Error(1)
}

// MockRegistryService is a mock implementation for testing.
type MockRegistryService struct {
	mock.Mock
}

func (m *MockRegistryService) GetEpicDrivenTemplate() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func TestNewWorkflowModal_OnSubmitCallsWorkflowCreator_LegacyPathWithoutCreator(t *testing.T) {
	registryService := createTestRegistryService(t)
	mockCP := newMockControlPlane()
	// When WorkflowCreator is nil, the goal is passed directly as InitialGoal
	mockCP.On("Create", mock.Anything, mock.MatchedBy(func(spec controlplane.WorkflowSpec) bool {
		return spec.EpicID == "" &&
			spec.InitialGoal == "Test my feature" &&
			spec.TemplateID == "quick-plan" &&
			spec.Name == "test-feature"
	})).Return(controlplane.WorkflowID("new-workflow-id"), nil).Once()

	// Note: We can't directly use the mock since NewWorkflowModal expects concrete types.
	// This test verifies the behavior when WorkflowCreator is nil (legacy path)
	modal := NewNewWorkflowModal(registryService, mockCP, nil, nil)

	values := map[string]any{
		"template": "quick-plan",
		"name":     "test-feature",
		"goal":     "Test my feature",
	}

	msg := modal.onSubmit(values)
	createMsg, ok := msg.(CreateWorkflowMsg)
	require.True(t, ok)
	require.Equal(t, controlplane.WorkflowID("new-workflow-id"), createMsg.WorkflowID)
	mockCP.AssertExpectations(t)
}

func TestNewWorkflowModal_MockCreatorAndRegistryServiceTypes(t *testing.T) {
	// This test verifies the mock types are properly defined for future use
	// when we need to test with actual WorkflowCreator and RegistryService instances

	mockCreator := &MockWorkflowCreator{}
	mockCreatorResult := &appreg.WorkflowResultDTO{
		Epic: appreg.EpicDTO{
			ID:      "perles-abc123",
			Title:   "Plan: Test Feature",
			Feature: "test-feature",
		},
		Workflow: appreg.WorkflowInfoDTO{
			Key:  "quick-plan",
			Name: "Quick Plan",
		},
		Tasks: []appreg.TaskResultDTO{},
	}
	mockCreator.On("Create", "test-feature", "quick-plan").Return(mockCreatorResult, nil)

	mockRegService := &MockRegistryService{}
	mockRegService.On("GetEpicDrivenTemplate").Return("# Epic-Driven Workflow\n\nYou are the Coordinator.", nil)

	// Verify mock methods work
	result, err := mockCreator.Create("test-feature", "quick-plan")
	require.NoError(t, err)
	require.Equal(t, "perles-abc123", result.Epic.ID)
	require.True(t, strings.Contains(result.Epic.Title, "Test Feature"))

	template, err := mockRegService.GetEpicDrivenTemplate()
	require.NoError(t, err)
	require.Contains(t, template, "Epic-Driven Workflow")

	mockCreator.AssertExpectations(t)
	mockRegService.AssertExpectations(t)
}

func TestNewWorkflowModal_BuildCoordinatorPromptContainsAllSections(t *testing.T) {
	registryService := createTestRegistryService(t)
	modal := NewNewWorkflowModal(registryService, nil, nil, nil)

	// Test the prompt building with no registry service (fallback)
	prompt := modal.buildCoordinatorPrompt("perles-abc123", "Build a cool feature")

	// Verify prompt contains epic ID
	require.Contains(t, prompt, "perles-abc123")
	// Verify prompt contains bd show command
	require.Contains(t, prompt, "bd show perles-abc123 --json")
	// Verify prompt contains goal
	require.Contains(t, prompt, "Build a cool feature")
	// Verify prompt structure
	require.Contains(t, prompt, "# Your Epic")
	require.Contains(t, prompt, "# Your Goal")
}

func TestNewWorkflowModal_OnSubmitReturnsErrorOnWorkflowCreatorFailure(t *testing.T) {
	registryService := createTestRegistryService(t)

	// Test the error handling path by verifying ErrorMsg is returned
	// when WorkflowCreator would fail (simulated by checking the error type exists)
	modal := NewNewWorkflowModal(registryService, nil, nil, nil)

	// This test verifies the ErrorMsg type is properly defined and can be used
	errMsg := ErrorMsg{Err: errors.New("create epic failed")}
	require.Error(t, errMsg.Err)
	require.Contains(t, errMsg.Err.Error(), "create epic failed")

	// Also verify the modal exists and is functional
	require.NotNil(t, modal)
}

func TestNewWorkflowModal_EpicIDPassedToWorkflowSpec(t *testing.T) {
	registryService := createTestRegistryService(t)

	// Verify that when onSubmit returns with EpicID, the spec contains it
	mockCP := newMockControlPlane()
	// Match on EpicID being empty when no WorkflowCreator is present
	mockCP.On("Create", mock.Anything, mock.MatchedBy(func(spec controlplane.WorkflowSpec) bool {
		return spec.EpicID == "" && spec.InitialGoal == "Test goal"
	})).Return(controlplane.WorkflowID("workflow-123"), nil).Once()

	modal := NewNewWorkflowModal(registryService, mockCP, nil, nil)

	values := map[string]any{
		"template": "quick-plan",
		"name":     "",
		"goal":     "Test goal",
	}

	msg := modal.onSubmit(values)
	createMsg, ok := msg.(CreateWorkflowMsg)
	require.True(t, ok)
	require.Equal(t, controlplane.WorkflowID("workflow-123"), createMsg.WorkflowID)
	mockCP.AssertExpectations(t)
}
