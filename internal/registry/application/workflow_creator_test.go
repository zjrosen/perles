package registry

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	beads "github.com/zjrosen/perles/internal/beads/domain"
	"github.com/zjrosen/perles/internal/config"
	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/templates"
)

func TestWorkflowCreator_Create(t *testing.T) {
	tests := []struct {
		name        string
		feature     string
		workflowKey string
		setupMock   func(*mocks.MockIssueExecutor)
		wantErr     bool
		errContains string
		wantTasks   int
		checkResult func(*testing.T, *WorkflowResultDTO, *mocks.MockIssueExecutor)
	}{
		{
			name:        "success - research-proposal creates tasks",
			feature:     "test-feature",
			workflowKey: "research-proposal",
			setupMock: func(m *mocks.MockIssueExecutor) {
				m.EXPECT().CreateEpic(
					"Research Proposal: Test Feature",
					mock.AnythingOfType("string"),
					[]string{"feature:test-feature", "workflow:research-proposal"},
				).Return(beads.CreateResult{ID: "test-epic", Title: "Research Proposal: Test Feature"}, nil)

				// Expect multiple task creations (16 nodes in research-proposal)
				m.EXPECT().CreateTask(
					mock.AnythingOfType("string"),
					mock.AnythingOfType("string"),
					"test-epic",
					mock.AnythingOfType("string"), // assignee
					[]string{"spec:plan"},
				).Return(beads.CreateResult{ID: "task-1", Title: "Task"}, nil).Times(16)

				// Expect dependency additions
				m.EXPECT().AddDependency(mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil).Maybe()
			},
			wantTasks: 16,
			checkResult: func(t *testing.T, result *WorkflowResultDTO, _ *mocks.MockIssueExecutor) {
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
			setupMock:   func(_ *mocks.MockIssueExecutor) {},
			wantErr:     true,
			errContains: "workflow not found",
		},
		{
			name:        "error - bd create epic fails",
			feature:     "test-feature",
			workflowKey: "research-proposal",
			setupMock: func(m *mocks.MockIssueExecutor) {
				m.EXPECT().CreateEpic(
					mock.AnythingOfType("string"),
					mock.AnythingOfType("string"),
					mock.AnythingOfType("[]string"),
				).Return(beads.CreateResult{}, errors.New("bd command failed: exit 1"))
			},
			wantErr:     true,
			errContains: "create epic",
		},
		{
			name:        "error - bd create task fails",
			feature:     "test-feature",
			workflowKey: "research-proposal",
			setupMock: func(m *mocks.MockIssueExecutor) {
				m.EXPECT().CreateEpic(
					mock.AnythingOfType("string"),
					mock.AnythingOfType("string"),
					mock.AnythingOfType("[]string"),
				).Return(beads.CreateResult{ID: "test-epic", Title: "Plan: Test Feature"}, nil)

				m.EXPECT().CreateTask(
					mock.AnythingOfType("string"),
					mock.AnythingOfType("string"),
					"test-epic",
					mock.AnythingOfType("string"), // assignee
					mock.AnythingOfType("[]string"),
				).Return(beads.CreateResult{}, errors.New("bd command failed: exit 1"))
			},
			wantErr:     true,
			errContains: "create task",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock
			mockExecutor := mocks.NewMockIssueExecutor(t)
			tt.setupMock(mockExecutor)

			// Create service with real registry
			registrySvc, err := NewRegistryService(templates.RegistryFS(), "")
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
	registrySvc, err := NewRegistryService(templates.RegistryFS(), "")
	require.NoError(t, err)

	mockExecutor := mocks.NewMockIssueExecutor(t)
	cfg := config.TemplatesConfig{DocumentPath: "docs/custom"}

	creator := NewWorkflowCreator(registrySvc, mockExecutor, cfg)

	require.Equal(t, cfg, creator.templatesConfig)
}

func TestWorkflowCreator_CreateWithArgs_Config(t *testing.T) {
	registrySvc, err := createRegistryServiceWithFS(createWorkflowCreatorConfigFS("Config entries: {{len .Config}}"))
	require.NoError(t, err)

	mockExecutor := mocks.NewMockIssueExecutor(t)
	mockExecutor.EXPECT().CreateEpic(
		"Config Workflow: Test Feature",
		mock.AnythingOfType("string"),
		[]string{"feature:test-feature", "workflow:config-workflow"},
	).Return(beads.CreateResult{ID: "test-epic", Title: "Config Workflow: Test Feature"}, nil)
	mockExecutor.EXPECT().CreateTask(
		"Task",
		mock.MatchedBy(func(content string) bool {
			return strings.Contains(content, "Config entries: 1")
		}),
		"test-epic",
		mock.AnythingOfType("string"),
		[]string{"spec:plan"},
	).Return(beads.CreateResult{ID: "task-1", Title: "Task"}, nil)

	creator := NewWorkflowCreator(registrySvc, mockExecutor, config.TemplatesConfig{})

	_, err = creator.CreateWithArgs("test-feature", "config-workflow", nil)
	require.NoError(t, err)
}

func TestWorkflowCreator_CreateWithArgs_ConfigDefault(t *testing.T) {
	registrySvc, err := createRegistryServiceWithFS(createWorkflowCreatorConfigFS("Config: {{.Config.document_path}}"))
	require.NoError(t, err)

	mockExecutor := mocks.NewMockIssueExecutor(t)
	mockExecutor.EXPECT().CreateEpic(
		"Config Workflow: Test Feature",
		mock.AnythingOfType("string"),
		[]string{"feature:test-feature", "workflow:config-workflow"},
	).Return(beads.CreateResult{ID: "test-epic", Title: "Config Workflow: Test Feature"}, nil)
	mockExecutor.EXPECT().CreateTask(
		"Task",
		mock.MatchedBy(func(content string) bool {
			return strings.Contains(content, "Config: docs/proposals")
		}),
		"test-epic",
		mock.AnythingOfType("string"),
		[]string{"spec:plan"},
	).Return(beads.CreateResult{ID: "task-1", Title: "Task"}, nil)

	creator := NewWorkflowCreator(registrySvc, mockExecutor, config.TemplatesConfig{})

	_, err = creator.CreateWithArgs("test-feature", "config-workflow", nil)
	require.NoError(t, err)
}

func TestWorkflowCreator_CreateWithArgs_ConfigCustom(t *testing.T) {
	registrySvc, err := createRegistryServiceWithFS(createWorkflowCreatorConfigFS("Config: {{.Config.document_path}}"))
	require.NoError(t, err)

	mockExecutor := mocks.NewMockIssueExecutor(t)
	mockExecutor.EXPECT().CreateEpic(
		"Config Workflow: Test Feature",
		mock.AnythingOfType("string"),
		[]string{"feature:test-feature", "workflow:config-workflow"},
	).Return(beads.CreateResult{ID: "test-epic", Title: "Config Workflow: Test Feature"}, nil)
	mockExecutor.EXPECT().CreateTask(
		"Task",
		mock.MatchedBy(func(content string) bool {
			return strings.Contains(content, "Config: docs/custom")
		}),
		"test-epic",
		mock.AnythingOfType("string"),
		[]string{"spec:plan"},
	).Return(beads.CreateResult{ID: "task-1", Title: "Task"}, nil)

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
