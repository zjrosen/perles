package registry

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/config"
	"github.com/zjrosen/perles/internal/mocks"
	taskpkg "github.com/zjrosen/perles/internal/task"
	"github.com/zjrosen/perles/internal/templates"
)

func TestWorkflowCreator_Create(t *testing.T) {
	tests := []struct {
		name        string
		feature     string
		workflowKey string
		setupMock   func(*mocks.MockTaskExecutor)
		wantErr     bool
		errContains string
		wantTasks   int
		checkResult func(*testing.T, *WorkflowResultDTO, *mocks.MockTaskExecutor)
	}{
		{
			name:        "success - research-proposal creates tasks",
			feature:     "test-feature",
			workflowKey: "research-proposal",
			setupMock: func(m *mocks.MockTaskExecutor) {
				m.EXPECT().CreateEpic(
					"Research Proposal: Test Feature",
					mock.AnythingOfType("string"),
					[]string{"feature:test-feature", "workflow:research-proposal"},
				).Return(taskpkg.CreateResult{ID: "test-epic", Title: "Research Proposal: Test Feature"}, nil)

				// Expect multiple task creations (16 nodes in research-proposal)
				m.EXPECT().CreateTask(
					mock.AnythingOfType("string"),
					mock.AnythingOfType("string"),
					"test-epic",
					mock.AnythingOfType("string"), // assignee
					[]string{"spec:plan"},
				).Return(taskpkg.CreateResult{ID: "task-1", Title: "Task"}, nil).Times(16)

				// Expect dependency additions
				m.EXPECT().AddDependency(mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil).Maybe()
			},
			wantTasks: 16,
			checkResult: func(t *testing.T, result *WorkflowResultDTO, _ *mocks.MockTaskExecutor) {
				require.Equal(t, "test-epic", result.Epic.ID)
				require.Equal(t, "Research Proposal: Test Feature", result.Epic.Title)
				require.Equal(t, "test-feature", result.Epic.Feature)
				require.Equal(t, "research-proposal", result.Workflow.Key)
			},
		},
		{
			name:        "error - workflow not found",
			feature:     "test-feature",
			workflowKey: "nonexistent",
			setupMock:   func(_ *mocks.MockTaskExecutor) {},
			wantErr:     true,
			errContains: "workflow not found",
		},
		{
			name:        "error - bd create epic fails",
			feature:     "test-feature",
			workflowKey: "research-proposal",
			setupMock: func(m *mocks.MockTaskExecutor) {
				m.EXPECT().CreateEpic(
					mock.AnythingOfType("string"),
					mock.AnythingOfType("string"),
					mock.AnythingOfType("[]string"),
				).Return(taskpkg.CreateResult{}, errors.New("bd command failed: exit 1"))
			},
			wantErr:     true,
			errContains: "create epic",
		},
		{
			name:        "error - bd create task fails",
			feature:     "test-feature",
			workflowKey: "research-proposal",
			setupMock: func(m *mocks.MockTaskExecutor) {
				m.EXPECT().CreateEpic(
					mock.AnythingOfType("string"),
					mock.AnythingOfType("string"),
					mock.AnythingOfType("[]string"),
				).Return(taskpkg.CreateResult{ID: "test-epic", Title: "Plan: Test Feature"}, nil)

				m.EXPECT().CreateTask(
					mock.AnythingOfType("string"),
					mock.AnythingOfType("string"),
					"test-epic",
					mock.AnythingOfType("string"), // assignee
					mock.AnythingOfType("[]string"),
				).Return(taskpkg.CreateResult{}, errors.New("bd command failed: exit 1"))
			},
			wantErr:     true,
			errContains: "create task",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock
			mockExecutor := mocks.NewMockTaskExecutor(t)
			tt.setupMock(mockExecutor)

			// Create service with real registry
			registrySvc, err := NewRegistryService(templates.RegistryFS(), nil, "")
			require.NoError(t, err)

			creator := NewWorkflowCreator(registrySvc, mockExecutor, config.TemplatesConfig{})

			// Execute
			result, err := creator.Create(tt.feature, tt.workflowKey)

			// Verify
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					require.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)

			if tt.wantTasks > 0 {
				require.Len(t, result.Tasks, tt.wantTasks)
			}

			if tt.checkResult != nil {
				tt.checkResult(t, result, mockExecutor)
			}
		})
	}
}

func TestWorkflowCreator_NewWithConfig(t *testing.T) {
	registrySvc, err := NewRegistryService(templates.RegistryFS(), nil, "")
	require.NoError(t, err)

	mockExecutor := mocks.NewMockTaskExecutor(t)
	cfg := config.TemplatesConfig{DocumentPath: "docs/custom"}

	creator := NewWorkflowCreator(registrySvc, mockExecutor, cfg)

	require.Equal(t, cfg, creator.templatesConfig)
}

func TestWorkflowCreator_CreateWithArgs_Config(t *testing.T) {
	registrySvc, err := createRegistryServiceWithFS(createWorkflowCreatorConfigFS("Config entries: {{len .Config}}"))
	require.NoError(t, err)

	mockExecutor := mocks.NewMockTaskExecutor(t)
	mockExecutor.EXPECT().CreateEpic(
		"Config Workflow: Test Feature",
		mock.AnythingOfType("string"),
		[]string{"feature:test-feature", "workflow:config-workflow"},
	).Return(taskpkg.CreateResult{ID: "test-epic", Title: "Config Workflow: Test Feature"}, nil)
	mockExecutor.EXPECT().CreateTask(
		"Task",
		mock.MatchedBy(func(content string) bool {
			return strings.Contains(content, "Config entries: 1")
		}),
		"test-epic",
		mock.AnythingOfType("string"),
		[]string{"spec:plan"},
	).Return(taskpkg.CreateResult{ID: "task-1", Title: "Task"}, nil)

	creator := NewWorkflowCreator(registrySvc, mockExecutor, config.TemplatesConfig{})

	_, err = creator.CreateWithArgs("test-feature", "config-workflow", nil)
	require.NoError(t, err)
}

func TestWorkflowCreator_CreateWithArgs_ConfigDefault(t *testing.T) {
	registrySvc, err := createRegistryServiceWithFS(createWorkflowCreatorConfigFS("Config: {{.Config.document_path}}"))
	require.NoError(t, err)

	mockExecutor := mocks.NewMockTaskExecutor(t)
	mockExecutor.EXPECT().CreateEpic(
		"Config Workflow: Test Feature",
		mock.AnythingOfType("string"),
		[]string{"feature:test-feature", "workflow:config-workflow"},
	).Return(taskpkg.CreateResult{ID: "test-epic", Title: "Config Workflow: Test Feature"}, nil)
	mockExecutor.EXPECT().CreateTask(
		"Task",
		mock.MatchedBy(func(content string) bool {
			return strings.Contains(content, "Config: docs/proposals")
		}),
		"test-epic",
		mock.AnythingOfType("string"),
		[]string{"spec:plan"},
	).Return(taskpkg.CreateResult{ID: "task-1", Title: "Task"}, nil)

	creator := NewWorkflowCreator(registrySvc, mockExecutor, config.TemplatesConfig{})

	_, err = creator.CreateWithArgs("test-feature", "config-workflow", nil)
	require.NoError(t, err)
}

func TestWorkflowCreator_CreateWithArgs_ConfigCustom(t *testing.T) {
	registrySvc, err := createRegistryServiceWithFS(createWorkflowCreatorConfigFS("Config: {{.Config.document_path}}"))
	require.NoError(t, err)

	mockExecutor := mocks.NewMockTaskExecutor(t)
	mockExecutor.EXPECT().CreateEpic(
		"Config Workflow: Test Feature",
		mock.AnythingOfType("string"),
		[]string{"feature:test-feature", "workflow:config-workflow"},
	).Return(taskpkg.CreateResult{ID: "test-epic", Title: "Config Workflow: Test Feature"}, nil)
	mockExecutor.EXPECT().CreateTask(
		"Task",
		mock.MatchedBy(func(content string) bool {
			return strings.Contains(content, "Config: docs/custom")
		}),
		"test-epic",
		mock.AnythingOfType("string"),
		[]string{"spec:plan"},
	).Return(taskpkg.CreateResult{ID: "task-1", Title: "Task"}, nil)

	creator := NewWorkflowCreator(registrySvc, mockExecutor, config.TemplatesConfig{DocumentPath: "docs/custom"})

	_, err = creator.CreateWithArgs("test-feature", "config-workflow", nil)
	require.NoError(t, err)
}

func TestToTitleCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"test-feature", "Test Feature"},
		{"test-standardization-testify-require", "Test Standardization Testify Require"},
		{"simple", "Simple"},
		{"", ""},
		{"a-b-c", "A B C"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := toTitleCase(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

// --- Tests for collaborative/zero-node workflow path (nil DAG) ---

func createCollaborativeWorkflowFS(epicContent string) fstest.MapFS {
	return fstest.MapFS{
		"workflows/collab/template.yaml": &fstest.MapFile{
			Data: []byte(`
registry:
  - namespace: "workflow"
    key: "collab-planning"
    version: "v1"
    name: "Collaborative Planning"
    description: "Multi-agent planning via Fabric channels"
    epic_template: "epic.md"
    system_prompt: "instructions.md"
    labels:
      - "category:planning"
    arguments:
      - key: "goal"
        label: "Planning Goal"
        description: "What to plan"
        type: "textarea"
        required: true
`),
		},
		"workflows/collab/epic.md": &fstest.MapFile{
			Data: []byte(epicContent),
		},
		"workflows/collab/instructions.md": &fstest.MapFile{
			Data: []byte("# Coordinator instructions"),
		},
	}
}

func createEpicDrivenWorkflowFS() fstest.MapFS {
	return fstest.MapFS{
		"workflows/epic-driven/template.yaml": &fstest.MapFile{
			Data: []byte(`
registry:
  - namespace: "workflow"
    key: "epic-driven"
    version: "v1"
    name: "Epic Driven"
    description: "Uses an existing epic"
    system_prompt: "instructions.md"
    arguments:
      - key: "epic_id"
        label: "Epic ID"
        description: "Existing epic to work on"
        type: "text"
        required: true
`),
		},
		"workflows/epic-driven/instructions.md": &fstest.MapFile{
			Data: []byte("# Epic-driven instructions"),
		},
	}
}

func TestWorkflowCreator_CreateWithArgs_CollaborativeWorkflow(t *testing.T) {
	// Collaborative workflow: has epic_template + system_prompt, zero nodes.
	// Should create an epic but return zero tasks (nil DAG path).
	registrySvc, err := createRegistryServiceWithFS(createCollaborativeWorkflowFS(
		"# Planning: {{.Args.goal}}\nSlug: {{.Slug}}",
	))
	require.NoError(t, err)

	mockExecutor := mocks.NewMockTaskExecutor(t)
	mockExecutor.EXPECT().CreateEpic(
		"Collaborative Planning: Test Feature",
		mock.MatchedBy(func(body string) bool {
			return strings.Contains(body, "Planning: Build a widget") &&
				strings.Contains(body, "Slug: test-feature")
		}),
		[]string{"feature:test-feature", "workflow:collab-planning"},
	).Return(taskpkg.CreateResult{ID: "epic-123", Title: "Collaborative Planning: Test Feature"}, nil)

	// No CreateTask or AddDependency calls expected

	creator := NewWorkflowCreator(registrySvc, mockExecutor, config.TemplatesConfig{})

	result, err := creator.CreateWithArgs("test-feature", "collab-planning", map[string]string{
		"goal": "Build a widget",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify epic
	require.Equal(t, "epic-123", result.Epic.ID)
	require.Equal(t, "Collaborative Planning: Test Feature", result.Epic.Title)
	require.Equal(t, "test-feature", result.Epic.Feature)

	// Verify workflow info
	require.Equal(t, "collab-planning", result.Workflow.Key)
	require.Equal(t, "Collaborative Planning", result.Workflow.Name)

	// Critical: zero tasks for collaborative workflow
	require.Nil(t, result.Tasks, "collaborative workflow should have nil tasks (no pre-created tasks)")
}

func TestWorkflowCreator_CreateWithArgs_CollaborativeWorkflow_EpicCreationFails(t *testing.T) {
	registrySvc, err := createRegistryServiceWithFS(createCollaborativeWorkflowFS("# Epic"))
	require.NoError(t, err)

	mockExecutor := mocks.NewMockTaskExecutor(t)
	mockExecutor.EXPECT().CreateEpic(
		mock.AnythingOfType("string"),
		mock.AnythingOfType("string"),
		mock.AnythingOfType("[]string"),
	).Return(taskpkg.CreateResult{}, errors.New("bd command failed"))

	creator := NewWorkflowCreator(registrySvc, mockExecutor, config.TemplatesConfig{})

	_, err = creator.CreateWithArgs("test-feature", "collab-planning", map[string]string{
		"goal": "Build a widget",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "create epic")
}

func TestWorkflowCreator_CreateWithArgs_CollaborativeWorkflow_NoEpicTemplate(t *testing.T) {
	// A collaborative workflow without an epic_template should use the default description
	fsys := fstest.MapFS{
		"workflows/no-epic-tmpl/template.yaml": &fstest.MapFile{
			Data: []byte(`
registry:
  - namespace: "workflow"
    key: "no-epic-tmpl"
    version: "v1"
    name: "No Epic Template"
    description: "Workflow without epic template"
    system_prompt: "instructions.md"
    arguments:
      - key: "goal"
        label: "Goal"
        description: "The goal"
        type: "textarea"
        required: true
`),
		},
		"workflows/no-epic-tmpl/instructions.md": &fstest.MapFile{
			Data: []byte("# Instructions"),
		},
	}

	registrySvc, err := createRegistryServiceWithFS(fsys)
	require.NoError(t, err)

	mockExecutor := mocks.NewMockTaskExecutor(t)
	mockExecutor.EXPECT().CreateEpic(
		"No Epic Template: Test Feature",
		mock.MatchedBy(func(body string) bool {
			// Should use default description format when no epic_template
			return strings.Contains(body, "Workflow: no-epic-tmpl") &&
				strings.Contains(body, "Feature: test-feature")
		}),
		[]string{"feature:test-feature", "workflow:no-epic-tmpl"},
	).Return(taskpkg.CreateResult{ID: "epic-456", Title: "No Epic Template: Test Feature"}, nil)

	creator := NewWorkflowCreator(registrySvc, mockExecutor, config.TemplatesConfig{})

	result, err := creator.CreateWithArgs("test-feature", "no-epic-tmpl", map[string]string{
		"goal": "Something",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "epic-456", result.Epic.ID)
	require.Nil(t, result.Tasks)
}

func TestWorkflowCreator_CreateWithArgs_EpicDrivenWorkflow(t *testing.T) {
	// Epic-driven workflow: single epic_id argument, no nodes, nil DAG.
	// Should also return zero tasks via the same nil DAG path.
	registrySvc, err := createRegistryServiceWithFS(createEpicDrivenWorkflowFS())
	require.NoError(t, err)

	mockExecutor := mocks.NewMockTaskExecutor(t)
	mockExecutor.EXPECT().CreateEpic(
		"Epic Driven: My Epic",
		mock.AnythingOfType("string"), // default description (no epic_template)
		[]string{"feature:my-epic", "workflow:epic-driven"},
	).Return(taskpkg.CreateResult{ID: "epic-789", Title: "Epic Driven: My Epic"}, nil)

	creator := NewWorkflowCreator(registrySvc, mockExecutor, config.TemplatesConfig{})

	result, err := creator.CreateWithArgs("my-epic", "epic-driven", map[string]string{
		"epic_id": "existing-epic-42",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Equal(t, "epic-789", result.Epic.ID)
	require.Equal(t, "epic-driven", result.Workflow.Key)
	require.Equal(t, "Epic Driven", result.Workflow.Name)
	require.Nil(t, result.Tasks, "epic-driven workflow should have nil tasks")
}

func createWorkflowCreatorConfigFS(templateContent string) fstest.MapFS {
	return fstest.MapFS{
		"workflows/config-workflow/template.yaml": &fstest.MapFile{
			Data: []byte(`
registry:
  - namespace: "workflow"
    key: "config-workflow"
    version: "v1"
    name: "Config Workflow"
    description: "Config test workflow"
    nodes:
      - key: "task"
        name: "Task"
        template: "task.md"
`),
		},
		"workflows/config-workflow/task.md": &fstest.MapFile{
			Data: []byte(templateContent),
		},
	}
}
