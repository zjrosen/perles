package dashboard

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	appgit "github.com/zjrosen/perles/internal/git/application"
	domaingit "github.com/zjrosen/perles/internal/git/domain"
	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/mode"
	"github.com/zjrosen/perles/internal/orchestration/controlplane"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/message"
)

// === Test Helpers ===

// createTestWorkflow creates a test workflow instance with the given parameters.
func createTestWorkflow(id controlplane.WorkflowID, name string, state controlplane.WorkflowState) *controlplane.WorkflowInstance {
	return &controlplane.WorkflowInstance{
		ID:            id,
		Name:          name,
		State:         state,
		TemplateID:    "test-template",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		ActiveWorkers: 0,
		TokensUsed:    0,
	}
}

// createTestModel creates a dashboard model with a mock ControlPlane.
func createTestModel(t *testing.T, workflows []*controlplane.WorkflowInstance) (Model, *mockControlPlane) {
	t.Helper()

	mockCP := newMockControlPlane()
	mockCP.On("List", mock.Anything, mock.Anything).Return(workflows, nil).Maybe()

	// Setup event channel for global subscription
	eventCh := make(chan controlplane.ControlPlaneEvent)
	close(eventCh) // Close immediately for tests that don't need events
	mockCP.On("Subscribe", mock.Anything).Return((<-chan controlplane.ControlPlaneEvent)(eventCh), func() {}).Maybe()

	// Setup event channel for workflow-specific subscriptions
	wfEventCh := make(chan controlplane.ControlPlaneEvent)
	close(wfEventCh)
	mockCP.On("SubscribeWorkflow", mock.Anything, mock.Anything).Return(
		(<-chan controlplane.ControlPlaneEvent)(wfEventCh),
		func() {},
	).Maybe()

	cfg := Config{
		ControlPlane: mockCP,
		Services:     mode.Services{},
	}

	m := New(cfg)
	// Simulate workflow load
	m.workflows = workflows
	m.workflowList = m.workflowList.SetWorkflows(workflows)
	m.resourceSummary = m.resourceSummary.Update(workflows)
	m = m.SetSize(100, 40).(Model)

	return m, mockCP
}

// === Unit Tests: Model Initialization ===

func TestModel_Init_ReturnsCommands(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
		createTestWorkflow("wf-2", "Workflow 2", controlplane.WorkflowPending),
	}

	mockCP := newMockControlPlane()
	// Setup expectations for when commands are executed
	mockCP.On("List", mock.Anything, mock.Anything).Return(workflows, nil).Maybe()
	mockCP.On("Subscribe", mock.Anything).Return((<-chan controlplane.ControlPlaneEvent)(nil), func() {}).Maybe()

	cfg := Config{
		ControlPlane: mockCP,
		Services:     mode.Services{},
	}

	m := New(cfg)

	// Init should return commands for subscription and loading
	cmds := m.Init()
	require.NotNil(t, cmds)
}

func TestModel_WorkflowsLoaded_UpdatesState(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
		createTestWorkflow("wf-2", "Workflow 2", controlplane.WorkflowPending),
	}

	m, _ := createTestModel(t, []*controlplane.WorkflowInstance{})
	require.Len(t, m.workflows, 0)

	// Simulate receiving workflows from ControlPlane
	result, _ := m.Update(workflowsLoadedMsg{workflows: workflows})
	m = result.(Model)

	require.Len(t, m.workflows, 2)
	require.Equal(t, controlplane.WorkflowID("wf-1"), m.workflows[0].ID)
	require.Equal(t, controlplane.WorkflowID("wf-2"), m.workflows[1].ID)
}

// === Unit Tests: Navigation ===

func TestModel_Navigation_MoveDownIncrementsSelection(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
		createTestWorkflow("wf-2", "Workflow 2", controlplane.WorkflowPending),
		createTestWorkflow("wf-3", "Workflow 3", controlplane.WorkflowPaused),
	}

	m, _ := createTestModel(t, workflows)
	require.Equal(t, 0, m.selectedIndex)

	// Press j to move down
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = result.(Model)
	require.Equal(t, 1, m.selectedIndex)

	// Press down arrow
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = result.(Model)
	require.Equal(t, 2, m.selectedIndex)
}

func TestModel_Navigation_MoveUpDecrementsSelection(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
		createTestWorkflow("wf-2", "Workflow 2", controlplane.WorkflowPending),
		createTestWorkflow("wf-3", "Workflow 3", controlplane.WorkflowPaused),
	}

	m, _ := createTestModel(t, workflows)
	m.selectedIndex = 2

	// Press k to move up
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = result.(Model)
	require.Equal(t, 1, m.selectedIndex)

	// Press up arrow
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = result.(Model)
	require.Equal(t, 0, m.selectedIndex)
}

func TestModel_Navigation_WrapsAtBoundaries(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
		createTestWorkflow("wf-2", "Workflow 2", controlplane.WorkflowPending),
	}

	m, _ := createTestModel(t, workflows)

	// Start at last item
	m.selectedIndex = 1

	// Press j should wrap to first
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = result.(Model)
	require.Equal(t, 0, m.selectedIndex)
}

func TestModel_Navigation_DoesNotGoBelowZero(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
	}

	m, _ := createTestModel(t, workflows)
	m.selectedIndex = 0

	// Press k should stay at 0
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = result.(Model)
	require.Equal(t, 0, m.selectedIndex)
}

// === Unit Tests: Quick Actions ===

func TestModel_StopAction_CallsControlPlaneStop(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-running", "Running Workflow", controlplane.WorkflowRunning),
	}

	mockCP := newMockControlPlane()
	mockCP.On("List", mock.Anything, mock.Anything).Return(workflows, nil).Maybe()
	mockCP.On("Stop", mock.Anything, controlplane.WorkflowID("wf-running"), mock.Anything).Return(nil).Once()

	eventCh := make(chan controlplane.ControlPlaneEvent)
	close(eventCh)
	mockCP.On("Subscribe", mock.Anything).Return((<-chan controlplane.ControlPlaneEvent)(eventCh), func() {}).Maybe()

	cfg := Config{
		ControlPlane: mockCP,
		Services:     mode.Services{},
	}

	m := New(cfg)
	m.workflows = workflows
	m.selectedIndex = 0

	// Press x to stop
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	require.NotNil(t, cmd)

	// Execute the command to trigger the Stop call
	cmd()

	mockCP.AssertExpectations(t)
}

func TestModel_StopAction_DoesNotStopTerminalWorkflows(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-completed", "Completed Workflow", controlplane.WorkflowCompleted),
	}

	m, _ := createTestModel(t, workflows)

	// Press x on a completed workflow - should do nothing
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = result.(Model)
	require.Nil(t, cmd) // No command since workflow is terminal
}

// === Unit Tests: Event Handling ===

func TestModel_EventsTriggersViewRefresh(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
	}

	m, mockCP := createTestModel(t, workflows)

	// Simulate receiving a lifecycle event
	event := controlplane.ControlPlaneEvent{
		Type:       controlplane.EventWorkflowStarted,
		WorkflowID: "wf-1",
	}

	// Update with the event
	mockCP.On("List", mock.Anything, mock.Anything).Return(workflows, nil).Maybe()
	result, cmd := m.Update(event)
	m = result.(Model)

	// Should return a batch command for loading workflows and continuing to listen
	require.NotNil(t, cmd)
}

// === Unit Tests: Workflow Selection ===

func TestModel_SelectedWorkflow_ReturnsCorrectWorkflow(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
		createTestWorkflow("wf-2", "Workflow 2", controlplane.WorkflowPending),
	}

	m, _ := createTestModel(t, workflows)

	// Select first workflow
	m.selectedIndex = 0
	selected := m.SelectedWorkflow()
	require.NotNil(t, selected)
	require.Equal(t, controlplane.WorkflowID("wf-1"), selected.ID)

	// Select second workflow
	m.selectedIndex = 1
	selected = m.SelectedWorkflow()
	require.NotNil(t, selected)
	require.Equal(t, controlplane.WorkflowID("wf-2"), selected.ID)
}

func TestModel_SelectedWorkflow_ReturnsNilWhenEmpty(t *testing.T) {
	m, _ := createTestModel(t, []*controlplane.WorkflowInstance{})

	selected := m.SelectedWorkflow()
	require.Nil(t, selected)
}

func TestModel_SelectedWorkflow_ReturnsNilForInvalidIndex(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
	}

	m, _ := createTestModel(t, workflows)
	m.selectedIndex = 5 // Invalid index

	selected := m.SelectedWorkflow()
	require.Nil(t, selected)
}

// === Unit Tests: SetSize ===

func TestModel_SetSize_UpdatesDimensions(t *testing.T) {
	m, _ := createTestModel(t, []*controlplane.WorkflowInstance{})

	result := m.SetSize(120, 50)
	m = result.(Model)

	require.Equal(t, 120, m.width)
	require.Equal(t, 50, m.height)
}

// === Unit Tests: Cleanup ===

func TestModel_Cleanup_CancelsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	m := Model{
		ctx:    ctx,
		cancel: cancel,
	}

	// Verify context is not cancelled
	select {
	case <-m.ctx.Done():
		t.Fatal("context should not be cancelled before cleanup")
	default:
	}

	// Cleanup
	m.Cleanup()

	// Verify context is cancelled
	select {
	case <-ctx.Done():
		// Expected
	default:
		t.Fatal("context should be cancelled after cleanup")
	}
}

// === Unit Tests: g/G Navigation ===

func TestModel_Navigation_GGoesToFirst(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
		createTestWorkflow("wf-2", "Workflow 2", controlplane.WorkflowPending),
		createTestWorkflow("wf-3", "Workflow 3", controlplane.WorkflowPaused),
	}

	m, _ := createTestModel(t, workflows)
	m.selectedIndex = 2

	// Press g to go to first
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = result.(Model)
	require.Equal(t, 0, m.selectedIndex)
}

func TestModel_Navigation_ShiftGGoesToLast(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
		createTestWorkflow("wf-2", "Workflow 2", controlplane.WorkflowPending),
		createTestWorkflow("wf-3", "Workflow 3", controlplane.WorkflowPaused),
	}

	m, _ := createTestModel(t, workflows)
	m.selectedIndex = 0

	// Press G to go to last
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	m = result.(Model)
	require.Equal(t, 2, m.selectedIndex)
}

func TestModel_Navigation_ShiftGNoopWhenEmpty(t *testing.T) {
	m, _ := createTestModel(t, []*controlplane.WorkflowInstance{})
	m.selectedIndex = 0

	// Press G on empty list - should stay at 0
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	m = result.(Model)
	require.Equal(t, 0, m.selectedIndex)
}

// === Unit Tests: Filter ===

func TestModel_Filter_SlashActivatesFilter(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
	}

	m, _ := createTestModel(t, workflows)
	require.False(t, m.filter.IsActive())

	// Press / to activate filter
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = result.(Model)
	require.True(t, m.filter.IsActive())
	require.NotNil(t, cmd) // Should return blink command
}

func TestModel_Filter_EscClearsFilter(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Test Workflow", controlplane.WorkflowRunning),
		createTestWorkflow("wf-2", "Another Workflow", controlplane.WorkflowPending),
	}

	m, _ := createTestModel(t, workflows)
	// Set up a filter
	m.filter = m.filter.Activate()
	m.filter.textInput.SetValue("Test")
	m.filter, _ = m.filter.Update(tea.KeyMsg{Type: tea.KeyEnter})

	require.True(t, m.filter.HasFilter())

	// Press Esc to clear filter
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = result.(Model)
	require.False(t, m.filter.HasFilter())
}

func TestModel_Filter_FiltersWorkflowsByName(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Authentication System", controlplane.WorkflowRunning),
		createTestWorkflow("wf-2", "Payment Processing", controlplane.WorkflowPending),
		createTestWorkflow("wf-3", "Auth Token Refresh", controlplane.WorkflowPaused),
	}

	m, _ := createTestModel(t, workflows)
	// Set up a filter for "auth"
	m.filter = m.filter.Activate()
	m.filter.textInput.SetValue("auth")
	m.filter, _ = m.filter.Update(tea.KeyMsg{Type: tea.KeyEnter})

	filtered := m.getFilteredWorkflows()
	require.Len(t, filtered, 2) // Should match "Authentication System" and "Auth Token Refresh"
}

func TestModel_Filter_NavigationUsesFilteredList(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Authentication System", controlplane.WorkflowRunning),
		createTestWorkflow("wf-2", "Payment Processing", controlplane.WorkflowPending),
		createTestWorkflow("wf-3", "Auth Token Refresh", controlplane.WorkflowPaused),
	}

	m, _ := createTestModel(t, workflows)
	// Set up a filter for "auth"
	m.filter = m.filter.Activate()
	m.filter.textInput.SetValue("auth")
	m.filter, _ = m.filter.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Navigate with j - should only move within 2 filtered items
	m.selectedIndex = 0
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = result.(Model)
	require.Equal(t, 1, m.selectedIndex)

	// Press j again - should wrap to 0
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = result.(Model)
	require.Equal(t, 0, m.selectedIndex)
}

func TestModel_Filter_SelectedWorkflowReturnsFilteredItem(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "First Workflow", controlplane.WorkflowRunning),
		createTestWorkflow("wf-2", "Second Workflow", controlplane.WorkflowPending),
		createTestWorkflow("wf-3", "Third Workflow", controlplane.WorkflowPaused),
	}

	m, _ := createTestModel(t, workflows)
	// Set up a filter for "second"
	m.filter = m.filter.Activate()
	m.filter.textInput.SetValue("second")
	m.filter, _ = m.filter.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Selected should return the filtered item
	m.selectedIndex = 0
	selected := m.SelectedWorkflow()
	require.NotNil(t, selected)
	require.Equal(t, controlplane.WorkflowID("wf-2"), selected.ID)
}

// === Unit Tests: Help Modal ===

func TestModel_Help_QuestionMarkTogglesHelp(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
	}

	m, _ := createTestModel(t, workflows)
	require.False(t, m.showHelp)

	// Press ? to show help
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = result.(Model)
	require.True(t, m.showHelp)

	// Press ? again to hide help
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = result.(Model)
	require.False(t, m.showHelp)
}

func TestModel_Help_EscClosesHelp(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
	}

	m, _ := createTestModel(t, workflows)
	m.showHelp = true

	// Press Esc to close help
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = result.(Model)
	require.False(t, m.showHelp)
}

func TestModel_Help_OtherKeysBlockedWhenHelpShowing(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
		createTestWorkflow("wf-2", "Workflow 2", controlplane.WorkflowPending),
	}

	m, _ := createTestModel(t, workflows)
	m.showHelp = true
	m.selectedIndex = 0

	// Press j when help is showing - should not navigate
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = result.(Model)
	require.Equal(t, 0, m.selectedIndex)
	require.True(t, m.showHelp) // Help still showing
}

func TestModel_Help_ViewShowsHelpOverlay(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
	}

	m, _ := createTestModel(t, workflows)
	m.showHelp = true
	m = m.SetSize(100, 40).(Model)

	view := m.View()
	// Help overlay should contain "Dashboard Help"
	require.Contains(t, view, "Dashboard Help")
}

// (Focus and split-view tests removed - table view only now)

// === Unit Tests: WorkflowUIState Map ===

func TestModel_getOrCreateUIState_ReturnsExistingState(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
	}

	m, _ := createTestModel(t, workflows)

	// Pre-populate state
	existingState := NewWorkflowUIState()
	existingState.CoordinatorQueueCount = 42 // Add a distinguishing value
	m.workflowUIState["wf-1"] = existingState

	// getOrCreateUIState should return the existing state
	state := m.getOrCreateUIState("wf-1")
	require.Same(t, existingState, state, "should return the exact same state instance")
	require.Equal(t, 42, state.CoordinatorQueueCount, "returned state should have existing data")
}

func TestModel_getOrCreateUIState_CreatesNewStateIfMissing(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
	}

	m, _ := createTestModel(t, workflows)
	require.Len(t, m.workflowUIState, 0, "map should start empty")

	// getOrCreateUIState should create new state
	state := m.getOrCreateUIState("wf-1")
	require.NotNil(t, state, "should create a new state")
	require.True(t, state.IsEmpty(), "new state should be empty")

	// Verify it was added to the map
	require.Len(t, m.workflowUIState, 1, "map should have one entry")
	require.Same(t, state, m.workflowUIState["wf-1"], "state should be stored in map")
}

func TestModel_getOrCreateUIState_MultipleWorkflowsIsolated(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
		createTestWorkflow("wf-2", "Workflow 2", controlplane.WorkflowPending),
	}

	m, _ := createTestModel(t, workflows)

	// Create states for two different workflows
	state1 := m.getOrCreateUIState("wf-1")
	state2 := m.getOrCreateUIState("wf-2")

	// Verify they are different instances
	require.NotSame(t, state1, state2, "different workflows should have different state instances")

	// Modify state1 and verify state2 is unaffected
	state1.CoordinatorQueueCount = 100
	require.Equal(t, 0, state2.CoordinatorQueueCount, "state2 should be unaffected by state1 changes")
}

// === Unit Tests: Event Handler Cache Updates ===

func TestModel_EventCoordinatorOutput_UpdatesCache(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
	}

	m, _ := createTestModel(t, workflows)
	require.Len(t, m.workflowUIState, 0, "cache should start empty")

	// Simulate coordinator output event
	event := controlplane.ControlPlaneEvent{
		Type:       controlplane.EventCoordinatorOutput,
		WorkflowID: "wf-1",
		Payload: events.ProcessEvent{
			ProcessID: "coord-1",
			Role:      events.RoleCoordinator,
			Output:    "Hello from coordinator",
			Delta:     false,
		},
	}

	result, _ := m.Update(event)
	m = result.(Model)

	// Verify cache was updated
	require.Len(t, m.workflowUIState, 1)
	state := m.workflowUIState["wf-1"]
	require.NotNil(t, state)
	require.Len(t, state.CoordinatorMessages, 1)
	require.Equal(t, "Hello from coordinator", state.CoordinatorMessages[0].Content)
	require.Equal(t, "assistant", state.CoordinatorMessages[0].Role)
}

func TestModel_EventCoordinatorOutput_AccumulatesDeltaMessages(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
	}

	m, _ := createTestModel(t, workflows)

	// First message (non-delta)
	event1 := controlplane.ControlPlaneEvent{
		Type:       controlplane.EventCoordinatorOutput,
		WorkflowID: "wf-1",
		Payload: events.ProcessEvent{
			ProcessID: "coord-1",
			Role:      events.RoleCoordinator,
			Output:    "Hello ",
			Delta:     false,
		},
	}
	result, _ := m.Update(event1)
	m = result.(Model)

	// Second message (delta - should append)
	event2 := controlplane.ControlPlaneEvent{
		Type:       controlplane.EventCoordinatorOutput,
		WorkflowID: "wf-1",
		Payload: events.ProcessEvent{
			ProcessID: "coord-1",
			Role:      events.RoleCoordinator,
			Output:    "world!",
			Delta:     true,
		},
	}
	result, _ = m.Update(event2)
	m = result.(Model)

	// Verify delta was accumulated
	state := m.workflowUIState["wf-1"]
	require.Len(t, state.CoordinatorMessages, 1, "delta should be appended, not create new message")
	require.Equal(t, "Hello world!", state.CoordinatorMessages[0].Content)
}

func TestModel_EventWorkerOutput_UpdatesCache(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
	}

	m, _ := createTestModel(t, workflows)

	// Simulate worker output event
	event := controlplane.ControlPlaneEvent{
		Type:       controlplane.EventWorkerOutput,
		WorkflowID: "wf-1",
		Payload: events.ProcessEvent{
			ProcessID: "worker-1",
			Role:      events.RoleWorker,
			Output:    "Worker output here",
			Delta:     false,
		},
	}

	result, _ := m.Update(event)
	m = result.(Model)

	// Verify cache was updated
	state := m.workflowUIState["wf-1"]
	require.NotNil(t, state)
	require.Len(t, state.WorkerMessages["worker-1"], 1)
	require.Equal(t, "Worker output here", state.WorkerMessages["worker-1"][0].Content)
}

func TestModel_EventWorkerOutput_AccumulatesDeltaMessages(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
	}

	m, _ := createTestModel(t, workflows)

	// First worker message (non-delta)
	event1 := controlplane.ControlPlaneEvent{
		Type:       controlplane.EventWorkerOutput,
		WorkflowID: "wf-1",
		Payload: events.ProcessEvent{
			ProcessID: "worker-1",
			Role:      events.RoleWorker,
			Output:    "Starting ",
			Delta:     false,
		},
	}
	result, _ := m.Update(event1)
	m = result.(Model)

	// Second message (delta - should append)
	event2 := controlplane.ControlPlaneEvent{
		Type:       controlplane.EventWorkerOutput,
		WorkflowID: "wf-1",
		Payload: events.ProcessEvent{
			ProcessID: "worker-1",
			Role:      events.RoleWorker,
			Output:    "task...",
			Delta:     true,
		},
	}
	result, _ = m.Update(event2)
	m = result.(Model)

	// Verify delta was accumulated
	state := m.workflowUIState["wf-1"]
	require.Len(t, state.WorkerMessages["worker-1"], 1)
	require.Equal(t, "Starting task...", state.WorkerMessages["worker-1"][0].Content)
}

func TestModel_EventMessagePosted_UpdatesCache(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
	}

	m, _ := createTestModel(t, workflows)

	// Simulate message posted event
	entry := message.Entry{
		ID:      "msg-1",
		From:    "COORDINATOR",
		To:      "WORKER.1",
		Content: "Task assignment",
	}
	event := controlplane.ControlPlaneEvent{
		Type:       controlplane.EventMessagePosted,
		WorkflowID: "wf-1",
		Payload:    message.Event{Entry: entry},
	}

	result, _ := m.Update(event)
	m = result.(Model)

	// Verify cache was updated
	state := m.workflowUIState["wf-1"]
	require.NotNil(t, state)
	require.Len(t, state.MessageEntries, 1)
	require.Equal(t, "msg-1", state.MessageEntries[0].ID)
	require.Equal(t, "COORDINATOR", state.MessageEntries[0].From)
	require.Equal(t, "Task assignment", state.MessageEntries[0].Content)
}

func TestModel_EventWorkerSpawned_UpdatesCache(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
	}

	m, _ := createTestModel(t, workflows)

	// Simulate worker spawned event
	event := controlplane.ControlPlaneEvent{
		Type:       controlplane.EventWorkerSpawned,
		WorkflowID: "wf-1",
		Payload: events.ProcessEvent{
			ProcessID: "worker-1",
			Role:      events.RoleWorker,
		},
	}

	result, _ := m.Update(event)
	m = result.(Model)

	// Verify cache was updated
	state := m.workflowUIState["wf-1"]
	require.NotNil(t, state)
	require.Contains(t, state.WorkerIDs, "worker-1")
	require.Equal(t, events.ProcessStatusReady, state.WorkerStatus["worker-1"])
}

func TestModel_EventWorkerRetired_UpdatesCache(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
	}

	m, _ := createTestModel(t, workflows)

	// First spawn a worker
	spawnEvent := controlplane.ControlPlaneEvent{
		Type:       controlplane.EventWorkerSpawned,
		WorkflowID: "wf-1",
		Payload: events.ProcessEvent{
			ProcessID: "worker-1",
			Role:      events.RoleWorker,
		},
	}
	result, _ := m.Update(spawnEvent)
	m = result.(Model)

	// Verify worker was added
	state := m.workflowUIState["wf-1"]
	require.Contains(t, state.WorkerIDs, "worker-1")

	// Now retire the worker
	retireEvent := controlplane.ControlPlaneEvent{
		Type:       controlplane.EventWorkerRetired,
		WorkflowID: "wf-1",
		Payload: events.ProcessEvent{
			ProcessID: "worker-1",
			Role:      events.RoleWorker,
		},
	}
	result, _ = m.Update(retireEvent)
	m = result.(Model)

	// Verify worker was removed from IDs and marked as retired
	state = m.workflowUIState["wf-1"]
	require.NotContains(t, state.WorkerIDs, "worker-1")
	require.Equal(t, events.ProcessStatusRetired, state.WorkerStatus["worker-1"])
}

func TestModel_NonSelectedWorkflowEventsStillUpdateCache(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
		createTestWorkflow("wf-2", "Workflow 2", controlplane.WorkflowRunning),
	}

	m, _ := createTestModel(t, workflows)
	m.selectedIndex = 0 // Select wf-1

	// Simulate event for wf-2 (NOT the selected workflow)
	event := controlplane.ControlPlaneEvent{
		Type:       controlplane.EventCoordinatorOutput,
		WorkflowID: "wf-2",
		Payload: events.ProcessEvent{
			ProcessID: "coord-2",
			Role:      events.RoleCoordinator,
			Output:    "Background workflow output",
			Delta:     false,
		},
	}

	result, _ := m.Update(event)
	m = result.(Model)

	// Verify cache was updated for wf-2 even though it's not selected
	require.Contains(t, m.workflowUIState, controlplane.WorkflowID("wf-2"))
	state := m.workflowUIState["wf-2"]
	require.Len(t, state.CoordinatorMessages, 1)
	require.Equal(t, "Background workflow output", state.CoordinatorMessages[0].Content)
}

func TestModel_MultipleWorkflowsAccumulateStateIndependently(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
		createTestWorkflow("wf-2", "Workflow 2", controlplane.WorkflowRunning),
	}

	m, _ := createTestModel(t, workflows)

	// Event for wf-1
	event1 := controlplane.ControlPlaneEvent{
		Type:       controlplane.EventCoordinatorOutput,
		WorkflowID: "wf-1",
		Payload: events.ProcessEvent{
			ProcessID: "coord-1",
			Role:      events.RoleCoordinator,
			Output:    "Message for workflow 1",
			Delta:     false,
		},
	}
	result, _ := m.Update(event1)
	m = result.(Model)

	// Event for wf-2
	event2 := controlplane.ControlPlaneEvent{
		Type:       controlplane.EventCoordinatorOutput,
		WorkflowID: "wf-2",
		Payload: events.ProcessEvent{
			ProcessID: "coord-2",
			Role:      events.RoleCoordinator,
			Output:    "Message for workflow 2",
			Delta:     false,
		},
	}
	result, _ = m.Update(event2)
	m = result.(Model)

	// Verify both workflows have their own independent state
	state1 := m.workflowUIState["wf-1"]
	state2 := m.workflowUIState["wf-2"]

	require.Len(t, state1.CoordinatorMessages, 1)
	require.Equal(t, "Message for workflow 1", state1.CoordinatorMessages[0].Content)

	require.Len(t, state2.CoordinatorMessages, 1)
	require.Equal(t, "Message for workflow 2", state2.CoordinatorMessages[0].Content)
}

func TestModel_EventEmptyWorkflowID_DoesNotUpdateCache(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
	}

	m, _ := createTestModel(t, workflows)

	// Event with empty workflow ID (shouldn't happen in practice, but should be handled)
	event := controlplane.ControlPlaneEvent{
		Type:       controlplane.EventCoordinatorOutput,
		WorkflowID: "", // Empty!
		Payload: events.ProcessEvent{
			ProcessID: "coord-1",
			Role:      events.RoleCoordinator,
			Output:    "Orphan message",
			Delta:     false,
		},
	}

	result, _ := m.Update(event)
	m = result.(Model)

	// Verify no state was created
	require.Len(t, m.workflowUIState, 0, "empty workflow ID should not create cache entry")
}

func TestModel_ToolCallMessageDetected(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
	}

	m, _ := createTestModel(t, workflows)

	// Simulate tool call message (starts with 🔧)
	event := controlplane.ControlPlaneEvent{
		Type:       controlplane.EventCoordinatorOutput,
		WorkflowID: "wf-1",
		Payload: events.ProcessEvent{
			ProcessID: "coord-1",
			Role:      events.RoleCoordinator,
			Output:    "🔧 Running command...",
			Delta:     false,
		},
	}

	result, _ := m.Update(event)
	m = result.(Model)

	// Verify tool call was detected
	state := m.workflowUIState["wf-1"]
	require.Len(t, state.CoordinatorMessages, 1)
	require.True(t, state.CoordinatorMessages[0].IsToolCall)
}

func TestModel_ToolCallDeltaDoesNotAppendToToolCallMessage(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
	}

	m, _ := createTestModel(t, workflows)

	// First message is a tool call
	event1 := controlplane.ControlPlaneEvent{
		Type:       controlplane.EventCoordinatorOutput,
		WorkflowID: "wf-1",
		Payload: events.ProcessEvent{
			ProcessID: "coord-1",
			Role:      events.RoleCoordinator,
			Output:    "🔧 Running command...",
			Delta:     false,
		},
	}
	result, _ := m.Update(event1)
	m = result.(Model)

	// Second message is a delta - should NOT append to tool call
	event2 := controlplane.ControlPlaneEvent{
		Type:       controlplane.EventCoordinatorOutput,
		WorkflowID: "wf-1",
		Payload: events.ProcessEvent{
			ProcessID: "coord-1",
			Role:      events.RoleCoordinator,
			Output:    "More output",
			Delta:     true,
		},
	}
	result, _ = m.Update(event2)
	m = result.(Model)

	// Verify delta created new message (not appended to tool call)
	state := m.workflowUIState["wf-1"]
	require.Len(t, state.CoordinatorMessages, 2)
	require.Equal(t, "🔧 Running command...", state.CoordinatorMessages[0].Content)
	require.Equal(t, "More output", state.CoordinatorMessages[1].Content)
}

func TestModel_EventUpdatesLastUpdatedTimestamp(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
	}

	mockClock := &mocks.MockClock{}
	expectedTime := time.Date(2026, 1, 18, 12, 0, 0, 0, time.UTC)
	mockClock.On("Now").Return(expectedTime)

	mockCP := newMockControlPlane()
	mockCP.On("List", mock.Anything, mock.Anything).Return(workflows, nil).Maybe()

	eventCh := make(chan controlplane.ControlPlaneEvent)
	close(eventCh)
	mockCP.On("Subscribe", mock.Anything).Return((<-chan controlplane.ControlPlaneEvent)(eventCh), func() {}).Maybe()

	cfg := Config{
		ControlPlane: mockCP,
		Services: mode.Services{
			Clock: mockClock,
		},
	}

	m := New(cfg)
	m.workflows = workflows
	m = m.SetSize(100, 40).(Model)

	// Verify no state exists initially
	require.Len(t, m.workflowUIState, 0)

	// Simulate event
	event := controlplane.ControlPlaneEvent{
		Type:       controlplane.EventCoordinatorOutput,
		WorkflowID: "wf-1",
		Payload: events.ProcessEvent{
			ProcessID: "coord-1",
			Role:      events.RoleCoordinator,
			Output:    "Test output",
			Delta:     false,
		},
	}

	result, _ := m.Update(event)
	m = result.(Model)

	// Verify LastUpdated was set to the mocked time
	state := m.workflowUIState["wf-1"]
	require.NotNil(t, state)
	require.Equal(t, expectedTime, state.LastUpdated)

	mockClock.AssertExpectations(t)
}

func TestModel_EventWithNilClockDoesNotPanic(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
	}

	// Services.Clock is nil (default for test setup without mock)
	m, _ := createTestModel(t, workflows)

	// Simulate event - should not panic even without Clock
	event := controlplane.ControlPlaneEvent{
		Type:       controlplane.EventCoordinatorOutput,
		WorkflowID: "wf-1",
		Payload: events.ProcessEvent{
			ProcessID: "coord-1",
			Role:      events.RoleCoordinator,
			Output:    "Test output",
			Delta:     false,
		},
	}

	// This should not panic
	require.NotPanics(t, func() {
		m.Update(event)
	})

	// Verify state was still created (just without timestamp update)
	state := m.workflowUIState["wf-1"]
	require.NotNil(t, state)
	require.Len(t, state.CoordinatorMessages, 1)
	// LastUpdated should remain zero since Clock is nil
	require.True(t, state.LastUpdated.IsZero())
}

// === Unit Tests: LRU Eviction ===

func TestModel_evictOldestUIState_NoEvictionWhenBelowMax(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowPending),
	}

	m, _ := createTestModel(t, workflows)

	// Add 5 states (below maxCachedWorkflows)
	for i := 0; i < 5; i++ {
		id := controlplane.WorkflowID(fmt.Sprintf("wf-%d", i))
		state := NewWorkflowUIState()
		state.LastUpdated = time.Now().Add(time.Duration(-i) * time.Hour)
		m.workflowUIState[id] = state
	}

	initialCount := len(m.workflowUIState)
	m.evictOldestUIState()

	require.Equal(t, initialCount, len(m.workflowUIState), "no eviction should occur when below max")
}

func TestModel_evictOldestUIState_EvictsWhenAboveMax(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowPending),
	}

	m, _ := createTestModel(t, workflows)

	// Add 11 states (above maxCachedWorkflows)
	for i := 0; i < 11; i++ {
		id := controlplane.WorkflowID(fmt.Sprintf("wf-%d", i))
		state := NewWorkflowUIState()
		state.LastUpdated = time.Now().Add(time.Duration(-i) * time.Hour) // Older states have lower i
		m.workflowUIState[id] = state
	}

	require.Equal(t, 11, len(m.workflowUIState))

	m.evictOldestUIState()

	require.Equal(t, 10, len(m.workflowUIState), "one entry should be evicted")
}

func TestModel_evictOldestUIState_EvictsOldestByLastUpdated(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowPending),
	}

	m, _ := createTestModel(t, workflows)

	now := time.Now()

	// Add states with different timestamps
	// "wf-oldest" should be evicted
	m.workflowUIState["wf-oldest"] = &WorkflowUIState{LastUpdated: now.Add(-10 * time.Hour)}
	m.workflowUIState["wf-newer"] = &WorkflowUIState{LastUpdated: now.Add(-5 * time.Hour)}
	m.workflowUIState["wf-newest"] = &WorkflowUIState{LastUpdated: now}

	// Fill up to 11 entries
	for i := 0; i < 8; i++ {
		id := controlplane.WorkflowID(fmt.Sprintf("wf-other-%d", i))
		m.workflowUIState[id] = &WorkflowUIState{LastUpdated: now.Add(-1 * time.Hour)}
	}

	require.Equal(t, 11, len(m.workflowUIState))

	m.evictOldestUIState()

	require.Equal(t, 10, len(m.workflowUIState))
	require.NotContains(t, m.workflowUIState, controlplane.WorkflowID("wf-oldest"), "oldest should be evicted")
	require.Contains(t, m.workflowUIState, controlplane.WorkflowID("wf-newer"), "newer should remain")
	require.Contains(t, m.workflowUIState, controlplane.WorkflowID("wf-newest"), "newest should remain")
}

func TestModel_evictOldestUIState_PreservesRunningWorkflows(t *testing.T) {
	// Create workflows with different states
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-running", "Running Workflow", controlplane.WorkflowRunning),
		createTestWorkflow("wf-pending", "Pending Workflow", controlplane.WorkflowPending),
	}

	m, _ := createTestModel(t, workflows)

	now := time.Now()

	// The running workflow is oldest but should NOT be evicted
	m.workflowUIState["wf-running"] = &WorkflowUIState{LastUpdated: now.Add(-100 * time.Hour)}
	m.workflowUIState["wf-pending"] = &WorkflowUIState{LastUpdated: now.Add(-50 * time.Hour)}

	// Fill up to 11 entries with other workflows
	for i := 0; i < 9; i++ {
		id := controlplane.WorkflowID(fmt.Sprintf("wf-other-%d", i))
		m.workflowUIState[id] = &WorkflowUIState{LastUpdated: now.Add(-1 * time.Hour)}
	}

	require.Equal(t, 11, len(m.workflowUIState))

	m.evictOldestUIState()

	require.Equal(t, 10, len(m.workflowUIState))
	// Running workflow should be preserved even though it's oldest
	require.Contains(t, m.workflowUIState, controlplane.WorkflowID("wf-running"), "running workflow should not be evicted")
	// The pending workflow was the oldest evictable one
	require.NotContains(t, m.workflowUIState, controlplane.WorkflowID("wf-pending"), "pending workflow should be evicted")
}

func TestModel_evictOldestUIState_PreservesSelectedWorkflow(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-selected", "Selected Workflow", controlplane.WorkflowPending),
		createTestWorkflow("wf-other", "Other Workflow", controlplane.WorkflowPending),
	}

	m, _ := createTestModel(t, workflows)
	m.selectedIndex = 0 // Select wf-selected

	now := time.Now()

	// The selected workflow is oldest but should NOT be evicted
	m.workflowUIState["wf-selected"] = &WorkflowUIState{LastUpdated: now.Add(-100 * time.Hour)}
	m.workflowUIState["wf-other"] = &WorkflowUIState{LastUpdated: now.Add(-50 * time.Hour)}

	// Fill up to 11 entries
	for i := 0; i < 9; i++ {
		id := controlplane.WorkflowID(fmt.Sprintf("wf-other-%d", i))
		m.workflowUIState[id] = &WorkflowUIState{LastUpdated: now.Add(-1 * time.Hour)}
	}

	require.Equal(t, 11, len(m.workflowUIState))

	m.evictOldestUIState()

	require.Equal(t, 10, len(m.workflowUIState))
	// Selected workflow should be preserved
	require.Contains(t, m.workflowUIState, controlplane.WorkflowID("wf-selected"), "selected workflow should not be evicted")
	// The wf-other was the oldest evictable one
	require.NotContains(t, m.workflowUIState, controlplane.WorkflowID("wf-other"), "wf-other should be evicted")
}

func TestModel_evictOldestUIState_NoEvictionIfAllProtected(t *testing.T) {
	// Create all running workflows - none should be evicted
	var workflows []*controlplane.WorkflowInstance
	for i := 0; i < 12; i++ {
		workflows = append(workflows, createTestWorkflow(
			controlplane.WorkflowID(fmt.Sprintf("wf-%d", i)),
			fmt.Sprintf("Workflow %d", i),
			controlplane.WorkflowRunning,
		))
	}

	m, _ := createTestModel(t, workflows)

	now := time.Now()

	// Add states for all running workflows
	for i := 0; i < 12; i++ {
		id := controlplane.WorkflowID(fmt.Sprintf("wf-%d", i))
		m.workflowUIState[id] = &WorkflowUIState{LastUpdated: now.Add(time.Duration(-i) * time.Hour)}
	}

	require.Equal(t, 12, len(m.workflowUIState))

	// All workflows are running, so none should be evicted
	m.evictOldestUIState()

	require.Equal(t, 12, len(m.workflowUIState), "no eviction when all workflows are protected")
}

func TestModel_getOrCreateUIState_TriggersEviction(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowPending),
	}

	m, _ := createTestModel(t, workflows)

	now := time.Now()

	// Pre-fill cache to maxCachedWorkflows + 1 so eviction is triggered
	// Using i=0 as newest (now) and higher i values as older
	for i := 0; i <= maxCachedWorkflows; i++ {
		id := controlplane.WorkflowID(fmt.Sprintf("wf-existing-%d", i))
		m.workflowUIState[id] = &WorkflowUIState{LastUpdated: now.Add(time.Duration(-i) * time.Hour)}
	}

	require.Equal(t, maxCachedWorkflows+1, len(m.workflowUIState))

	// Trigger eviction
	m.evictOldestUIState()

	require.Equal(t, maxCachedWorkflows, len(m.workflowUIState), "one entry should be evicted")
	// The oldest one (wf-existing-10) should have been evicted
	require.NotContains(t, m.workflowUIState, controlplane.WorkflowID(fmt.Sprintf("wf-existing-%d", maxCachedWorkflows)), "oldest should be evicted")
	// Newer entries should still exist
	require.Contains(t, m.workflowUIState, controlplane.WorkflowID("wf-existing-0"), "newest should remain")
}

func TestModel_isWorkflowRunning_ReturnsTrueForRunning(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-running", "Running Workflow", controlplane.WorkflowRunning),
		createTestWorkflow("wf-pending", "Pending Workflow", controlplane.WorkflowPending),
	}

	m, _ := createTestModel(t, workflows)

	require.True(t, m.isWorkflowRunning("wf-running"))
	require.False(t, m.isWorkflowRunning("wf-pending"))
}

func TestModel_isWorkflowRunning_ReturnsFalseForUnknownID(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
	}

	m, _ := createTestModel(t, workflows)

	require.False(t, m.isWorkflowRunning("wf-unknown"))
}

// === Unit Tests: EventWorkflowStopped Cache Cleanup ===

func TestModel_EventWorkflowStopped_RemovesFromCache(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
	}

	m, _ := createTestModel(t, workflows)

	// Pre-populate cache
	m.workflowUIState["wf-1"] = NewWorkflowUIState()
	m.workflowUIState["wf-1"].CoordinatorQueueCount = 42 // Add some data
	require.Contains(t, m.workflowUIState, controlplane.WorkflowID("wf-1"))

	// Simulate EventWorkflowStopped
	event := controlplane.ControlPlaneEvent{
		Type:       controlplane.EventWorkflowStopped,
		WorkflowID: "wf-1",
	}

	result, _ := m.Update(event)
	m = result.(Model)

	// Verify cache entry was removed
	require.NotContains(t, m.workflowUIState, controlplane.WorkflowID("wf-1"), "stopped workflow should be removed from cache")
}

func TestModel_EventWorkflowStopped_OnlyRemovesMatchingWorkflow(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
		createTestWorkflow("wf-2", "Workflow 2", controlplane.WorkflowRunning),
	}

	m, _ := createTestModel(t, workflows)

	// Pre-populate cache for both workflows
	m.workflowUIState["wf-1"] = NewWorkflowUIState()
	m.workflowUIState["wf-2"] = NewWorkflowUIState()

	// Simulate EventWorkflowStopped for wf-1
	event := controlplane.ControlPlaneEvent{
		Type:       controlplane.EventWorkflowStopped,
		WorkflowID: "wf-1",
	}

	result, _ := m.Update(event)
	m = result.(Model)

	// Verify only wf-1 was removed
	require.NotContains(t, m.workflowUIState, controlplane.WorkflowID("wf-1"))
	require.Contains(t, m.workflowUIState, controlplane.WorkflowID("wf-2"), "wf-2 should remain in cache")
}

func TestModel_EventWorkflowStopped_NoopForNonexistentCache(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
	}

	m, _ := createTestModel(t, workflows)

	// Don't pre-populate cache - wf-1 has no cache entry

	// Simulate EventWorkflowStopped
	event := controlplane.ControlPlaneEvent{
		Type:       controlplane.EventWorkflowStopped,
		WorkflowID: "wf-1",
	}

	// Should not panic
	require.NotPanics(t, func() {
		m.Update(event)
	})
}

func TestModel_EventWorkflowStopped_EmptyWorkflowID_NoEffect(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
	}

	m, _ := createTestModel(t, workflows)

	// Pre-populate cache
	m.workflowUIState["wf-1"] = NewWorkflowUIState()

	// Simulate EventWorkflowStopped with empty workflow ID
	event := controlplane.ControlPlaneEvent{
		Type:       controlplane.EventWorkflowStopped,
		WorkflowID: "",
	}

	result, _ := m.Update(event)
	m = result.(Model)

	// Cache should be unchanged (delete with empty key is a no-op)
	require.Contains(t, m.workflowUIState, controlplane.WorkflowID("wf-1"))
}

// === Phase 7: Wiring and Integration Tests ===

func TestModel_Config_AcceptsGitExecutorFactory(t *testing.T) {
	mockCP := newMockControlPlane()
	mockCP.On("List", mock.Anything, mock.Anything).Return([]*controlplane.WorkflowInstance{}, nil).Maybe()

	eventCh := make(chan controlplane.ControlPlaneEvent)
	close(eventCh)
	mockCP.On("Subscribe", mock.Anything).Return((<-chan controlplane.ControlPlaneEvent)(eventCh), func() {}).Maybe()

	// Create a factory function
	factory := func(path string) appgit.GitExecutor {
		return mocks.NewMockGitExecutor(t)
	}

	cfg := Config{
		ControlPlane:       mockCP,
		Services:           mode.Services{},
		GitExecutorFactory: factory,
		WorkDir:            "/test/workdir",
	}

	m := New(cfg)

	// Verify factory and workDir are stored
	require.NotNil(t, m.gitExecutorFactory)
	require.Equal(t, "/test/workdir", m.workDir)
}

func TestModel_PassesGitExecutorToNewWorkflowModal(t *testing.T) {
	mockCP := newMockControlPlane()
	mockCP.On("List", mock.Anything, mock.Anything).Return([]*controlplane.WorkflowInstance{}, nil).Maybe()

	eventCh := make(chan controlplane.ControlPlaneEvent)
	close(eventCh)
	mockCP.On("Subscribe", mock.Anything).Return((<-chan controlplane.ControlPlaneEvent)(eventCh), func() {}).Maybe()

	// Create a mock git executor that returns branches
	mockGitExecutor := mocks.NewMockGitExecutor(t)
	mockGitExecutor.EXPECT().ListBranches().Return([]domaingit.BranchInfo{
		{Name: "main", IsCurrent: true},
		{Name: "develop", IsCurrent: false},
	}, nil).Maybe()

	factoryCalled := false
	factory := func(path string) appgit.GitExecutor {
		factoryCalled = true
		require.Equal(t, "/test/workdir", path)
		return mockGitExecutor
	}

	registryService := createTestRegistryService(t)

	cfg := Config{
		ControlPlane:       mockCP,
		Services:           mode.Services{},
		RegistryService:    registryService,
		GitExecutorFactory: factory,
		WorkDir:            "/test/workdir",
	}

	m := New(cfg)
	m = m.SetSize(100, 40).(Model)

	// Open the new workflow modal
	result, _ := m.openNewWorkflowModal()
	m = result.(Model)

	// Factory should have been called
	require.True(t, factoryCalled, "GitExecutorFactory should be called when opening modal")

	// Modal should have worktree support enabled
	require.NotNil(t, m.NewWorkflowModalRef())
	require.True(t, m.NewWorkflowModalRef().worktreeEnabled)
}

func TestModel_WorksNormallyWithNilGitExecutorFactory(t *testing.T) {
	mockCP := newMockControlPlane()
	mockCP.On("List", mock.Anything, mock.Anything).Return([]*controlplane.WorkflowInstance{}, nil).Maybe()

	eventCh := make(chan controlplane.ControlPlaneEvent)
	close(eventCh)
	mockCP.On("Subscribe", mock.Anything).Return((<-chan controlplane.ControlPlaneEvent)(eventCh), func() {}).Maybe()

	registryService := createTestRegistryService(t)

	// Create config without GitExecutorFactory
	cfg := Config{
		ControlPlane:    mockCP,
		Services:        mode.Services{},
		RegistryService: registryService,
		// GitExecutorFactory is nil
		// WorkDir is empty
	}

	m := New(cfg)
	m = m.SetSize(100, 40).(Model)

	// Open the new workflow modal - should work without crashing
	result, _ := m.openNewWorkflowModal()
	m = result.(Model)

	// Modal should exist but worktree support should be disabled
	require.NotNil(t, m.NewWorkflowModalRef())
	require.False(t, m.NewWorkflowModalRef().worktreeEnabled)
}

func TestModel_HandleStartWorkflowFailed_ErrUncommittedChanges(t *testing.T) {
	m, _ := createTestModel(t, []*controlplane.WorkflowInstance{})

	msg := StartWorkflowFailedMsg{
		WorkflowID: "wf-1",
		Err:        fmt.Errorf("cleanup failed: %w", controlplane.ErrUncommittedChanges),
	}

	result, cmd := m.Update(msg)
	m = result.(Model)

	require.NotNil(t, cmd)

	toastMsg := cmd()
	showToast, ok := toastMsg.(mode.ShowToastMsg)
	require.True(t, ok)
	require.Contains(t, showToast.Message, "uncommitted changes")
}

func TestModel_HandleStartWorkflowFailed_ErrBranchAlreadyCheckedOut(t *testing.T) {
	m, _ := createTestModel(t, []*controlplane.WorkflowInstance{})

	msg := StartWorkflowFailedMsg{
		WorkflowID: "wf-1",
		Err:        fmt.Errorf("worktree create failed: %w", domaingit.ErrBranchAlreadyCheckedOut),
	}

	result, cmd := m.Update(msg)
	m = result.(Model)

	require.NotNil(t, cmd)

	toastMsg := cmd()
	showToast, ok := toastMsg.(mode.ShowToastMsg)
	require.True(t, ok)
	require.Contains(t, showToast.Message, "already checked out")
}

func TestModel_HandleStartWorkflowFailed_ErrPathAlreadyExists(t *testing.T) {
	m, _ := createTestModel(t, []*controlplane.WorkflowInstance{})

	msg := StartWorkflowFailedMsg{
		WorkflowID: "wf-1",
		Err:        fmt.Errorf("worktree create failed: %w", domaingit.ErrPathAlreadyExists),
	}

	result, cmd := m.Update(msg)
	m = result.(Model)

	require.NotNil(t, cmd)

	toastMsg := cmd()
	showToast, ok := toastMsg.(mode.ShowToastMsg)
	require.True(t, ok)
	require.Contains(t, showToast.Message, "path already exists")
}

func TestModel_HandleStartWorkflowFailed_GenericError(t *testing.T) {
	m, _ := createTestModel(t, []*controlplane.WorkflowInstance{})

	// Use a generic error that doesn't match any specific worktree error
	msg := StartWorkflowFailedMsg{
		WorkflowID: "wf-1",
		Err:        errors.New("some generic error"),
	}

	result, cmd := m.Update(msg)
	m = result.(Model)

	require.NotNil(t, cmd)

	toastMsg := cmd()
	showToast, ok := toastMsg.(mode.ShowToastMsg)
	require.True(t, ok)
	// Should show the original error message
	require.Contains(t, showToast.Message, "some generic error")
}

func TestModel_StartWorkflow_ReturnsErrorMessage(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-pending", "Pending Workflow", controlplane.WorkflowPending),
	}

	startErr := errors.New("failed to start workflow")
	mockCP := newMockControlPlane()
	mockCP.On("List", mock.Anything, mock.Anything).Return(workflows, nil).Maybe()
	mockCP.On("Start", mock.Anything, controlplane.WorkflowID("wf-pending")).Return(startErr).Once()

	eventCh := make(chan controlplane.ControlPlaneEvent)
	close(eventCh)
	mockCP.On("Subscribe", mock.Anything).Return((<-chan controlplane.ControlPlaneEvent)(eventCh), func() {}).Maybe()

	cfg := Config{
		ControlPlane: mockCP,
		Services:     mode.Services{},
	}

	m := New(cfg)
	m.workflows = workflows
	m.selectedIndex = 0

	// Press s to start
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	require.NotNil(t, cmd)

	// Execute the command to trigger the Start call
	result := cmd()

	// Should return a StartWorkflowFailedMsg
	failedMsg, ok := result.(StartWorkflowFailedMsg)
	require.True(t, ok)
	require.Equal(t, controlplane.WorkflowID("wf-pending"), failedMsg.WorkflowID)
	require.ErrorIs(t, failedMsg.Err, startErr)

	mockCP.AssertExpectations(t)
}
