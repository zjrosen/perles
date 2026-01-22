package registry

import (
	"fmt"
	"strings"
	"time"

	beads "github.com/zjrosen/perles/internal/beads/application"
	"github.com/zjrosen/perles/internal/registry/domain"
)

// WorkflowResultDTO is the final output of workflow creation.
type WorkflowResultDTO struct {
	Epic     EpicDTO         `json:"epic"`
	Workflow WorkflowInfoDTO `json:"workflow"`
	Tasks    []TaskResultDTO `json:"tasks"`
}

// EpicDTO represents the created epic.
type EpicDTO struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Feature string `json:"feature"`
}

// WorkflowInfoDTO identifies the workflow used.
type WorkflowInfoDTO struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// TaskResultDTO represents a created task.
type TaskResultDTO struct {
	ID        string   `json:"id"`
	Key       string   `json:"key"`
	Name      string   `json:"name"`
	DependsOn []string `json:"depends_on"`
}

// WorkflowCreator handles epic/task creation from workflow DAGs.
type WorkflowCreator struct {
	registry *RegistryService
	executor beads.IssueExecutor
}

// NewWorkflowCreator creates a new WorkflowCreator with dependency injection.
func NewWorkflowCreator(registry *RegistryService, executor beads.IssueExecutor) *WorkflowCreator {
	return &WorkflowCreator{
		registry: registry,
		executor: executor,
	}
}

// Create creates an epic with all workflow tasks and wires up dependencies.
// Returns a WorkflowResultDTO with the created epic, tasks, and resolved dependency IDs.
// This is a convenience wrapper around CreateWithArgs with no arguments.
func (c *WorkflowCreator) Create(feature, workflowKey string) (*WorkflowResultDTO, error) {
	return c.CreateWithArgs(feature, workflowKey, nil)
}

// CreateWithArgs creates an epic with all workflow tasks and wires up dependencies.
// The args parameter contains user-provided argument values for template rendering.
// Returns a WorkflowResultDTO with the created epic, tasks, and resolved dependency IDs.
func (c *WorkflowCreator) CreateWithArgs(feature, workflowKey string, args map[string]string) (*WorkflowResultDTO, error) {
	// 1. Get workflow registration
	reg, err := c.registry.GetByKey("workflow", workflowKey)
	if err != nil {
		return nil, fmt.Errorf("workflow not found: %s: %w", workflowKey, err)
	}

	// 2. Build template context with arguments
	baseCtx := TemplateContext{
		Slug: feature,
		Name: feature,
		Date: time.Now().Format("2006-01-02"),
		Args: args, // User-provided argument values
	}

	// 3. Render epic description from template (or use default)
	epicLabels := []string{
		fmt.Sprintf("feature:%s", feature),
		fmt.Sprintf("workflow:%s", workflowKey),
	}
	epicTitle := fmt.Sprintf("%s: %s", reg.Name(), toTitleCase(feature))

	var epicDescription string
	if reg.Template() != "" {
		epicDescription, err = c.registry.RenderEpicTemplate(reg, baseCtx)
		if err != nil {
			return nil, fmt.Errorf("render epic template: %w", err)
		}
	} else {
		epicDescription = fmt.Sprintf("Workflow: %s\nFeature: %s", workflowKey, feature)
	}

	epicResult, err := c.executor.CreateEpic(epicTitle, epicDescription, epicLabels)
	if err != nil {
		return nil, fmt.Errorf("create epic: %w", err)
	}

	// 4. Create tasks and build ID mapping
	dag := reg.DAG()
	if dag == nil {
		return nil, fmt.Errorf("workflow %s has no DAG", workflowKey)
	}

	keyToID := make(map[string]string)
	var tasks []TaskResultDTO

	for _, node := range dag.Nodes() {
		// Render template with context (reuse same context with args)
		identifier := registry.BuildIdentifier(reg.Namespace(), reg.Key(), reg.Version(), node.Key())
		content, err := c.registry.RenderTemplate(identifier, baseCtx)
		if err != nil {
			return nil, fmt.Errorf("render template %s: %w", node.Key(), err)
		}

		// Create task as child of epic
		taskResult, err := c.executor.CreateTask(
			node.Name(),
			content,
			epicResult.ID,
			node.Assignee(),
			[]string{"spec:plan"},
		)
		if err != nil {
			return nil, fmt.Errorf("create task %s: %w", node.Key(), err)
		}

		keyToID[node.Key()] = taskResult.ID
		tasks = append(tasks, TaskResultDTO{
			ID:   taskResult.ID,
			Key:  node.Key(),
			Name: node.Name(),
		})
	}

	// 5. Wire up dependencies and populate DependsOn
	for i, node := range dag.Nodes() {
		deps := dag.DependenciesOf(node.Key())
		resolvedDeps := make([]string, 0, len(deps))
		for _, dep := range deps {
			depID := keyToID[dep.Key()]
			resolvedDeps = append(resolvedDeps, depID)
			if err := c.executor.AddDependency(keyToID[node.Key()], depID); err != nil {
				return nil, fmt.Errorf("add dependency %s -> %s: %w", node.Key(), dep.Key(), err)
			}
		}
		tasks[i].DependsOn = resolvedDeps
	}

	return &WorkflowResultDTO{
		Epic: EpicDTO{
			ID:      epicResult.ID,
			Title:   epicTitle,
			Feature: feature,
		},
		Workflow: WorkflowInfoDTO{
			Key:  reg.Key(),
			Name: reg.Name(),
		},
		Tasks: tasks,
	}, nil
}

// toTitleCase converts dash-case to Title Case.
// e.g., "test-standardization-testify-require" → "Test Standardization Testify Require"
func toTitleCase(slug string) string {
	words := strings.Split(slug, "-")
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + word[1:]
		}
	}
	return strings.Join(words, " ")
}
