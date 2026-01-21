package dashboard

import (
	"testing"
	"testing/fstest"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/mode"
	"github.com/zjrosen/perles/internal/orchestration/controlplane"
	appreg "github.com/zjrosen/perles/internal/registry/application"
)

// testNow is a fixed reference time for golden tests to ensure reproducible timestamps.
var testNow = time.Date(2025, 12, 13, 12, 0, 0, 0, time.UTC)

// fixedClock is a clock that always returns testNow for reproducible golden tests.
type fixedClock struct{}

func (fixedClock) Now() time.Time { return testNow }

// filterMockCalls removes calls matching the given method name from the list.
// Used to override default mock expectations set in newMockControlPlane.
func filterMockCalls(calls []*mock.Call, methodName string) []*mock.Call {
	var filtered []*mock.Call
	for _, call := range calls {
		if call.Method != methodName {
			filtered = append(filtered, call)
		}
	}
	return filtered
}

// createGoldenTestModel creates a Model with mocked dependencies for reproducible golden tests.
func createGoldenTestModel(t *testing.T, workflows []*controlplane.WorkflowInstance) Model {
	t.Helper()
	mockCP := newMockControlPlane()
	mockCP.On("List", mock.Anything, mock.Anything).Return(workflows, nil).Maybe()
	mockCP.On("Subscribe", mock.Anything).Return((<-chan controlplane.ControlPlaneEvent)(nil), func() {}).Maybe()
	// Override default GetHealthStatus to use testNow for consistent golden output
	mockCP.ExpectedCalls = filterMockCalls(mockCP.ExpectedCalls, "GetHealthStatus")
	mockCP.On("GetHealthStatus", mock.Anything).Return(controlplane.HealthStatus{
		IsHealthy:       true,
		LastHeartbeatAt: testNow,
		LastProgressAt:  testNow,
	}, true).Maybe()
	cfg := Config{
		ControlPlane: mockCP,
		Services: mode.Services{
			Clock: fixedClock{},
		},
	}
	m := New(cfg)
	// Pre-populate workflow list to skip async loading
	m.workflows = workflows
	m.workflowList = m.workflowList.SetWorkflows(workflows)
	m.resourceSummary = m.resourceSummary.Update(workflows)
	m.width = 100
	m.height = 40
	return m
}

// createTestWorkflowWithDetails creates a workflow with detailed resource info.
func createTestWorkflowWithDetails(
	id controlplane.WorkflowID,
	name string,
	state controlplane.WorkflowState,
	activeWorkers int,
	tokensUsed int64,
) *controlplane.WorkflowInstance {
	wf := &controlplane.WorkflowInstance{
		ID:            id,
		Name:          name,
		State:         state,
		TemplateID:    "test-template",
		CreatedAt:     testNow,
		UpdatedAt:     testNow,
		ActiveWorkers: activeWorkers,
		TokensUsed:    tokensUsed,
	}
	// Set heartbeat for active workflows so health displays correctly
	if wf.IsActive() {
		wf.LastHeartbeatAt = testNow
	}
	return wf
}

// Golden tests for dashboard mode rendering.
// Run with -update flag to update golden files: go test -update ./internal/mode/dashboard/...
func TestDashboard_View_Golden_Empty(t *testing.T) {
	m := createGoldenTestModel(t, []*controlplane.WorkflowInstance{})
	m.width = 100
	m.height = 30
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestDashboard_View_Golden_WithWorkflows(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflowWithDetails(
			"wf-001",
			"Build authentication system",
			controlplane.WorkflowRunning,
			3, 125000,
		),
		createTestWorkflowWithDetails(
			"wf-002",
			"Fix payment processing bug",
			controlplane.WorkflowPending,
			0, 0,
		),
		createTestWorkflowWithDetails(
			"wf-003",
			"Refactor database layer",
			controlplane.WorkflowPaused,
			1, 45000,
		),
		createTestWorkflowWithDetails(
			"wf-004",
			"Deploy to production",
			controlplane.WorkflowCompleted,
			0, 87500,
		),
		createTestWorkflowWithDetails(
			"wf-005",
			"Integration test failure investigation",
			controlplane.WorkflowFailed,
			0, 12300,
		),
	}
	m := createGoldenTestModel(t, workflows)
	m.width = 100
	m.height = 30
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestDashboard_View_Golden_WithSelection(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflowWithDetails(
			"wf-001",
			"First Workflow",
			controlplane.WorkflowRunning,
			2, 50000,
		),
		createTestWorkflowWithDetails(
			"wf-002",
			"Second Workflow (Selected)",
			controlplane.WorkflowPending,
			0, 0,
		),
		createTestWorkflowWithDetails(
			"wf-003",
			"Third Workflow",
			controlplane.WorkflowRunning,
			1, 25000,
		),
	}
	m := createGoldenTestModel(t, workflows)
	m.selectedIndex = 1 // Select the second workflow
	m.width = 100
	m.height = 30
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestDashboard_View_Golden_LongNames(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflowWithDetails(
			"wf-001",
			"This is a very long workflow name that should be truncated in the display",
			controlplane.WorkflowRunning,
			2, 99000,
		),
		createTestWorkflowWithDetails(
			"wf-002",
			"Another extremely long workflow name for testing truncation behavior",
			controlplane.WorkflowPending,
			0, 0,
		),
	}
	m := createGoldenTestModel(t, workflows)
	m.width = 100
	m.height = 30
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestDashboard_View_Golden_LargeTokenCounts(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflowWithDetails(
			"wf-001",
			"Small token usage",
			controlplane.WorkflowRunning,
			1, 500, // 500 tokens
		),
		createTestWorkflowWithDetails(
			"wf-002",
			"Medium token usage",
			controlplane.WorkflowRunning,
			2, 50000, // 50K tokens
		),
		createTestWorkflowWithDetails(
			"wf-003",
			"Large token usage",
			controlplane.WorkflowRunning,
			3, 2500000, // 2.5M tokens
		),
	}
	m := createGoldenTestModel(t, workflows)
	m.width = 100
	m.height = 30
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// createGoldenTestRegistryFS creates a MapFS for golden testing with a valid registry.yaml
func createGoldenTestRegistryFS() fstest.MapFS {
	return fstest.MapFS{
		"registry.yaml": &fstest.MapFile{
			Data: []byte(`
registry:
  - namespace: "spec-workflow"
    key: "quick-plan"
    version: "v1"
    name: "Quick Plan"
    description: "Fast planning workflow for rapid iteration"
    nodes:
      - key: "plan"
        name: "Plan"
        template: "v1-plan.md"
  - namespace: "spec-workflow"
    key: "cook"
    version: "v1"
    name: "Cook"
    description: "Full implementation workflow with review"
    nodes:
      - key: "cook"
        name: "Cook"
        template: "v1-cook.md"
  - namespace: "spec-workflow"
    key: "research"
    version: "v1"
    name: "Research"
    description: "Research and task discovery workflow"
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

// createGoldenTestModelWithRegistry creates a Model with mocked dependencies and a domain registry.
func createGoldenTestModelWithRegistry(t *testing.T, workflows []*controlplane.WorkflowInstance) Model {
	t.Helper()
	mockCP := newMockControlPlane()
	mockCP.On("List", mock.Anything, mock.Anything).Return(workflows, nil).Maybe()
	mockCP.On("Subscribe", mock.Anything).Return((<-chan controlplane.ControlPlaneEvent)(nil), func() {}).Maybe()
	// Override default GetHealthStatus to use testNow for consistent golden output
	mockCP.ExpectedCalls = filterMockCalls(mockCP.ExpectedCalls, "GetHealthStatus")
	mockCP.On("GetHealthStatus", mock.Anything).Return(controlplane.HealthStatus{
		IsHealthy:       true,
		LastHeartbeatAt: testNow,
		LastProgressAt:  testNow,
	}, true).Maybe()

	registryFS := createGoldenTestRegistryFS()
	workflowFS := createTestWorkflowTemplatesFS()
	registryService, err := appreg.NewRegistryService(registryFS, workflowFS)
	require.NoError(t, err)

	cfg := Config{
		ControlPlane:    mockCP,
		Services:        mode.Services{},
		RegistryService: registryService,
	}
	m := New(cfg)
	m.workflows = workflows
	m.workflowList = m.workflowList.SetWorkflows(workflows)
	m.resourceSummary = m.resourceSummary.Update(workflows)
	m.width = 100
	m.height = 40
	return m
}

func TestDashboard_View_Golden_NewWorkflowModal(t *testing.T) {
	m := createGoldenTestModelWithRegistry(t, []*controlplane.WorkflowInstance{})
	m.width = 100
	m.height = 40
	// Open the new workflow modal
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = result.(Model)
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestDashboard_View_Golden_WithFilter(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflowWithDetails(
			"wf-001",
			"Build authentication system",
			controlplane.WorkflowRunning,
			3, 125000,
		),
		createTestWorkflowWithDetails(
			"wf-002",
			"Fix payment processing bug",
			controlplane.WorkflowPending,
			0, 0,
		),
		createTestWorkflowWithDetails(
			"wf-003",
			"Auth token refresh handler",
			controlplane.WorkflowPaused,
			1, 45000,
		),
	}
	m := createGoldenTestModel(t, workflows)
	m.width = 100
	m.height = 30
	// Set up a filter for "auth" - this should filter to 2 workflows
	m.filter = m.filter.Activate()
	m.filter.textInput.SetValue("auth")
	m.filter, _ = m.filter.Update(tea.KeyMsg{Type: tea.KeyEnter})
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestDashboard_View_Golden_WithHelpModal(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflowWithDetails(
			"wf-001",
			"Build authentication system",
			controlplane.WorkflowRunning,
			2, 50000,
		),
	}
	m := createGoldenTestModel(t, workflows)
	m.width = 120
	m.height = 35
	// Open the help modal
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = result.(Model)
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestDashboard_View_Golden_ShortTerminal(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflowWithDetails(
			"wf-001",
			"First Workflow",
			controlplane.WorkflowRunning,
			2, 50000,
		),
		createTestWorkflowWithDetails(
			"wf-002",
			"Second Workflow",
			controlplane.WorkflowPending,
			0, 0,
		),
	}
	m := createGoldenTestModel(t, workflows)
	m.width = 100
	m.height = 10 // Very short terminal - should enforce minimum content height of 5
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestDashboard_View_Golden_NarrowTerminal(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflowWithDetails(
			"wf-001",
			"Build auth system",
			controlplane.WorkflowRunning,
			2, 50000,
		),
		createTestWorkflowWithDetails(
			"wf-002",
			"Fix bug",
			controlplane.WorkflowPending,
			0, 0,
		),
	}
	m := createGoldenTestModel(t, workflows)
	m.width = 80
	m.height = 24
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestDashboard_View_Golden_VeryNarrowTerminal(t *testing.T) {
	// Narrow terminal - table should still render
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflowWithDetails(
			"wf-001",
			"Workflow 1",
			controlplane.WorkflowRunning,
			2, 50000,
		),
		createTestWorkflowWithDetails(
			"wf-002",
			"Workflow 2",
			controlplane.WorkflowPending,
			0, 0,
		),
	}
	m := createGoldenTestModel(t, workflows)
	m.width = 60
	m.height = 20
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// createTestWorkflowWithStarted creates a workflow with StartedAt set for timestamp display testing.
func createTestWorkflowWithStarted(
	id controlplane.WorkflowID,
	name string,
	state controlplane.WorkflowState,
	activeWorkers int,
	tokensUsed int64,
	startedAt time.Time,
) *controlplane.WorkflowInstance {
	wf := &controlplane.WorkflowInstance{
		ID:            id,
		Name:          name,
		State:         state,
		TemplateID:    "test-template",
		CreatedAt:     testNow,
		UpdatedAt:     testNow,
		ActiveWorkers: activeWorkers,
		TokensUsed:    tokensUsed,
		StartedAt:     &startedAt,
	}
	if wf.IsActive() {
		wf.LastHeartbeatAt = testNow
	}
	return wf
}

func TestDashboard_View_Golden_WithTimestamps(t *testing.T) {
	// Test with StartedAt timestamps to verify border alignment
	startTime := testNow.Add(-5 * time.Minute) // Started 5 minutes ago
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflowWithStarted(
			"wf-001",
			"Running workflow with timestamp",
			controlplane.WorkflowRunning,
			1, 50000,
			startTime,
		),
	}
	m := createGoldenTestModel(t, workflows)
	m.width = 100
	m.height = 20
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestDashboard_View_Golden_WithTimestamps_Narrow(t *testing.T) {
	// Test with StartedAt timestamps at narrower width to check border bleeding
	startTime := testNow.Add(-5 * time.Minute)
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflowWithStarted(
			"wf-001",
			"Running workflow",
			controlplane.WorkflowRunning,
			1, 50000,
			startTime,
		),
	}
	m := createGoldenTestModel(t, workflows)
	m.width = 90
	m.height = 20
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// createTestWorkflowWithWorkDir creates a workflow with WorkDir and WorktreePath fields set.
func createTestWorkflowWithWorkDir(
	id controlplane.WorkflowID,
	name string,
	state controlplane.WorkflowState,
	activeWorkers int,
	tokensUsed int64,
	workDir string,
	worktreePath string,
) *controlplane.WorkflowInstance {
	wf := &controlplane.WorkflowInstance{
		ID:            id,
		Name:          name,
		State:         state,
		TemplateID:    "test-template",
		CreatedAt:     testNow,
		UpdatedAt:     testNow,
		ActiveWorkers: activeWorkers,
		TokensUsed:    tokensUsed,
		WorkDir:       workDir,
		WorktreePath:  worktreePath,
	}
	if wf.IsActive() {
		wf.LastHeartbeatAt = testNow
	}
	return wf
}

func TestDashboard_View_Golden_WithWorkDirColumn(t *testing.T) {
	// Test with different WorkDir/WorktreePath configurations:
	// 1. Workflow with worktree (shows 🌳 prefix)
	// 2. Workflow with custom workdir (shows directory name)
	// 3. Workflow with no workdir set (shows ·)
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflowWithWorkDir(
			"wf-001",
			"Worktree workflow",
			controlplane.WorkflowRunning,
			2, 50000,
			"", // WorkDir not used when worktree is set
			"/path/to/worktrees/feature-branch",
		),
		createTestWorkflowWithWorkDir(
			"wf-002",
			"Custom workdir workflow",
			controlplane.WorkflowPending,
			0, 0,
			"/some/other/project",
			"", // No worktree
		),
		createTestWorkflowWithWorkDir(
			"wf-003",
			"Current directory workflow",
			controlplane.WorkflowRunning,
			1, 25000,
			"", // Empty = current directory
			"",
		),
	}
	m := createGoldenTestModel(t, workflows)
	m.width = 120
	m.height = 20
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// createTestWorkflowWithEpicID creates a workflow with EpicID field set.
func createTestWorkflowWithEpicID(
	id controlplane.WorkflowID,
	name string,
	state controlplane.WorkflowState,
	activeWorkers int,
	tokensUsed int64,
	epicID string,
) *controlplane.WorkflowInstance {
	wf := &controlplane.WorkflowInstance{
		ID:            id,
		Name:          name,
		State:         state,
		TemplateID:    "test-template",
		CreatedAt:     testNow,
		UpdatedAt:     testNow,
		ActiveWorkers: activeWorkers,
		TokensUsed:    tokensUsed,
		EpicID:        epicID,
	}
	if wf.IsActive() {
		wf.LastHeartbeatAt = testNow
	}
	return wf
}

func TestDashboard_View_Golden_WithEpicIDColumn(t *testing.T) {
	// Test with different EpicID configurations:
	// 1. Workflow with EpicID set
	// 2. Workflow with no EpicID (shows dash)
	// 3. Workflow with different EpicID
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflowWithEpicID(
			"wf-001",
			"Epic-linked workflow",
			controlplane.WorkflowRunning,
			2, 50000,
			"perles-abc.1",
		),
		createTestWorkflowWithEpicID(
			"wf-002",
			"Standalone workflow",
			controlplane.WorkflowPending,
			0, 0,
			"", // No epic ID
		),
		createTestWorkflowWithEpicID(
			"wf-003",
			"Another epic workflow",
			controlplane.WorkflowRunning,
			1, 25000,
			"epic-xyz",
		),
	}
	m := createGoldenTestModel(t, workflows)
	m.width = 120
	m.height = 20
	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}
