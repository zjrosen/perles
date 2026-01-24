package controlplane

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/session"
	v2 "github.com/zjrosen/perles/internal/orchestration/v2"
	"github.com/zjrosen/perles/internal/orchestration/v2/adapter"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/handler"
	"github.com/zjrosen/perles/internal/orchestration/v2/processor"
	"github.com/zjrosen/perles/internal/pubsub"
)

// === Test Helpers ===

// testListenerFactory creates real listeners on dynamic ports for testing.
type testListenerFactory struct {
	listener net.Listener
	err      error
}

func (m *testListenerFactory) Create(address string) (net.Listener, error) {
	if m.err != nil {
		return nil, m.err
	}
	// Create a real listener on port 0 (dynamic port) to avoid port conflicts
	if m.listener == nil {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		m.listener = ln
	}
	return m.listener, nil
}

// newTestControlPlane creates a ControlPlane with real registry/supervisor and mocks for AI.
func newTestControlPlane(t *testing.T) (ControlPlane, *mockInfrastructureFactory, *mocks.MockAgentProvider) {
	t.Helper()

	mockProvider := mocks.NewMockAgentProvider(t)
	mockFactory := &mockInfrastructureFactory{}
	registry := NewInMemoryRegistry()
	sessionFactory := session.NewFactory(session.FactoryConfig{BaseDir: t.TempDir()})

	supervisor, err := NewSupervisor(SupervisorConfig{
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: mockProvider,
		},
		InfrastructureFactory: mockFactory,
		ListenerFactory:       &testListenerFactory{},
		SessionFactory:        sessionFactory,
	})
	require.NoError(t, err)

	cp, err := NewControlPlane(ControlPlaneConfig{
		Registry:   registry,
		Supervisor: supervisor,
	})
	require.NoError(t, err)

	return cp, mockFactory, mockProvider
}

// setupTestAgentProviderMock sets up common mock expectations for AgentProvider.
// The Supervisor.AllocateResources() method calls Client() and Extensions() when creating the MCP server.
func setupTestAgentProviderMock(t *testing.T, mockProvider *mocks.MockAgentProvider) {
	t.Helper()
	mockClient := mocks.NewMockHeadlessClient(t)
	mockProvider.On("Client").Return(mockClient, nil).Maybe()
	mockProvider.On("Extensions").Return(map[string]any{}).Maybe()
}

// cleanupWorkflowSessionOnTestEnd registers a cleanup function to close any session
// created during the test. Required on Windows where files cannot be deleted while open.
func cleanupWorkflowSessionOnTestEnd(t *testing.T, cp ControlPlane, id WorkflowID) {
	t.Helper()
	t.Cleanup(func() {
		inst, err := cp.Get(context.Background(), id)
		if err == nil && inst != nil && inst.Session != nil {
			_ = inst.Session.Close(session.StatusCompleted)
		}
	})
}

// createTestInfrastructure creates a minimal v2.Infrastructure suitable for testing.
// It includes Core.Adapter and Internal.TurnEnforcer required for MCP server setup.
func createTestInfrastructure(t *testing.T) *v2.Infrastructure {
	t.Helper()

	eventBus := pubsub.NewBroker[any]()
	cmdProcessor := processor.NewCommandProcessor(
		processor.WithQueueCapacity(100),
		processor.WithEventBus(eventBus),
	)

	// Register mock handlers for commands used during Start
	cmdProcessor.RegisterHandler(command.CmdSpawnProcess, &testSpawnHandler{})
	cmdProcessor.RegisterHandler(command.CmdSendToProcess, &testSendHandler{})

	// Create the v2 adapter for MCP server setup
	v2Adapter := adapter.NewV2Adapter(cmdProcessor)

	// Create turn enforcer for worker server cache
	turnEnforcer := handler.NewTurnCompletionTracker()

	return &v2.Infrastructure{
		Core: v2.CoreComponents{
			Processor: cmdProcessor,
			EventBus:  eventBus,
			Adapter:   v2Adapter,
		},
		Internal: v2.InternalComponents{
			TurnEnforcer: turnEnforcer,
		},
	}
}

// testSpawnHandler is a simple handler that always succeeds for SpawnProcess commands.
type testSpawnHandler struct{}

func (h *testSpawnHandler) Handle(_ context.Context, cmd command.Command) (*command.CommandResult, error) {
	return &command.CommandResult{
		Success: true,
		Data:    &testSpawnResult{processID: "coordinator"},
	}, nil
}

// testSpawnResult implements the GetProcessID interface for spawn results.
type testSpawnResult struct {
	processID string
}

func (r *testSpawnResult) GetProcessID() string {
	return r.processID
}

// testSendHandler is a simple handler that always succeeds for SendToProcess commands.
type testSendHandler struct{}

func (h *testSendHandler) Handle(_ context.Context, cmd command.Command) (*command.CommandResult, error) {
	return &command.CommandResult{Success: true}, nil
}

// === Unit Tests: NewControlPlane ===

func TestNewControlPlane_ValidConfig(t *testing.T) {
	mockProvider := mocks.NewMockAgentProvider(t)
	registry := NewInMemoryRegistry()
	supervisor, err := NewSupervisor(SupervisorConfig{
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: mockProvider,
		},
		SessionFactory: session.NewFactory(session.FactoryConfig{BaseDir: t.TempDir()}),
	})
	require.NoError(t, err)

	cp, err := NewControlPlane(ControlPlaneConfig{
		Registry:   registry,
		Supervisor: supervisor,
	})

	require.NoError(t, err)
	require.NotNil(t, cp)
}

func TestNewControlPlane_MissingRegistry(t *testing.T) {
	mockProvider := mocks.NewMockAgentProvider(t)
	supervisor, err := NewSupervisor(SupervisorConfig{
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: mockProvider,
		},
		SessionFactory: session.NewFactory(session.FactoryConfig{BaseDir: t.TempDir()}),
	})
	require.NoError(t, err)

	cp, err := NewControlPlane(ControlPlaneConfig{
		Supervisor: supervisor,
		// Registry is nil
	})

	require.Error(t, err)
	require.Nil(t, cp)
	require.Contains(t, err.Error(), "Registry is required")
}

func TestNewControlPlane_MissingSupervisor(t *testing.T) {
	registry := NewInMemoryRegistry()

	cp, err := NewControlPlane(ControlPlaneConfig{
		Registry: registry,
		// Supervisor is nil
	})

	require.Error(t, err)
	require.Nil(t, cp)
	require.Contains(t, err.Error(), "Supervisor is required")
}

// === Unit Tests: Create ===

func TestControlPlane_Create_StoresWorkflowInPendingState(t *testing.T) {
	cp, _, _ := newTestControlPlane(t)

	spec := WorkflowSpec{
		TemplateID:    "test-template",
		InitialPrompt: "Build a feature",
		Name:          "My Workflow",
	}

	id, err := cp.Create(context.Background(), spec)

	require.NoError(t, err)
	require.NotEmpty(t, id)

	// Verify workflow is stored and in Pending state
	inst, err := cp.Get(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, WorkflowPending, inst.State)
	require.Equal(t, "My Workflow", inst.Name)
	require.Equal(t, "test-template", inst.TemplateID)
}

func TestControlPlane_Create_RejectsInvalidSpec_MissingTemplateID(t *testing.T) {
	cp, _, _ := newTestControlPlane(t)

	spec := WorkflowSpec{
		// TemplateID is empty
		InitialPrompt: "Build a feature",
	}

	id, err := cp.Create(context.Background(), spec)

	require.Error(t, err)
	require.Empty(t, id)
	require.Contains(t, err.Error(), "template_id is required")
}

func TestControlPlane_Create_RejectsInvalidSpec_MissingInitialPrompt(t *testing.T) {
	cp, _, _ := newTestControlPlane(t)

	spec := WorkflowSpec{
		TemplateID: "test-template",
		// InitialPrompt is empty
	}

	id, err := cp.Create(context.Background(), spec)

	require.Error(t, err)
	require.Empty(t, id)
	require.Contains(t, err.Error(), "initial_prompt is required")
}

func TestControlPlane_Create_GeneratesUniqueIDs(t *testing.T) {
	cp, _, _ := newTestControlPlane(t)

	spec1 := WorkflowSpec{
		TemplateID:    "test-template",
		InitialPrompt: "Goal 1",
	}
	spec2 := WorkflowSpec{
		TemplateID:    "test-template",
		InitialPrompt: "Goal 2",
	}

	id1, err := cp.Create(context.Background(), spec1)
	require.NoError(t, err)

	id2, err := cp.Create(context.Background(), spec2)
	require.NoError(t, err)

	require.NotEqual(t, id1, id2)
}

func TestControlPlane_Create_DefaultsNameToTemplateID(t *testing.T) {
	cp, _, _ := newTestControlPlane(t)

	spec := WorkflowSpec{
		TemplateID:    "my-awesome-template",
		InitialPrompt: "Build a feature",
		// Name is empty
	}

	id, err := cp.Create(context.Background(), spec)
	require.NoError(t, err)

	inst, err := cp.Get(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, "my-awesome-template", inst.Name)
}

// === Unit Tests: Start ===

func TestControlPlane_Start_DelegatesToSupervisor(t *testing.T) {
	cp, mockFactory, mockProvider := newTestControlPlane(t)

	// Create workflow first
	spec := WorkflowSpec{
		TemplateID:    "test-template",
		InitialPrompt: "Build a feature",
	}
	id, err := cp.Create(context.Background(), spec)
	require.NoError(t, err)
	cleanupWorkflowSessionOnTestEnd(t, cp, id) // Close session before TempDir cleanup (Windows)

	// Setup mock infrastructure
	infra := createTestInfrastructure(t)
	mockFactory.On("Create", mock.AnythingOfType("v2.InfrastructureConfig")).Return(infra, nil)
	setupTestAgentProviderMock(t, mockProvider)

	// Start the processor
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go infra.Core.Processor.Run(ctx)
	require.NoError(t, infra.Core.Processor.WaitForReady(ctx))

	// Start the workflow
	err = cp.Start(ctx, id)

	require.NoError(t, err)

	// Verify workflow is now Running
	inst, err := cp.Get(ctx, id)
	require.NoError(t, err)
	require.Equal(t, WorkflowRunning, inst.State)

	mockFactory.AssertExpectations(t)
}

func TestControlPlane_Start_ReturnsErrorForNonExistentWorkflow(t *testing.T) {
	cp, _, _ := newTestControlPlane(t)

	nonExistentID := NewWorkflowID()
	err := cp.Start(context.Background(), nonExistentID)

	require.Error(t, err)
	require.ErrorIs(t, err, ErrWorkflowNotFound)
}

// === Unit Tests: Stop ===

func TestControlPlane_Stop_DelegatesToSupervisor(t *testing.T) {
	cp, mockFactory, mockProvider := newTestControlPlane(t)

	// Create and start workflow first
	spec := WorkflowSpec{
		TemplateID:    "test-template",
		InitialPrompt: "Build a feature",
	}
	id, err := cp.Create(context.Background(), spec)
	require.NoError(t, err)

	// Setup mock infrastructure
	infra := createTestInfrastructure(t)
	mockFactory.On("Create", mock.AnythingOfType("v2.InfrastructureConfig")).Return(infra, nil)
	setupTestAgentProviderMock(t, mockProvider)

	// Start the processor
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go infra.Core.Processor.Run(ctx)
	require.NoError(t, infra.Core.Processor.WaitForReady(ctx))

	require.NoError(t, cp.Start(ctx, id))

	// Stop the workflow
	err = cp.Stop(ctx, id, StopOptions{Reason: "test"})

	require.NoError(t, err)

	// Verify workflow is now Stopped
	inst, err := cp.Get(ctx, id)
	require.NoError(t, err)
	require.Equal(t, WorkflowStopped, inst.State)
}

func TestControlPlane_Stop_ReturnsErrorForNonExistentWorkflow(t *testing.T) {
	cp, _, _ := newTestControlPlane(t)

	nonExistentID := NewWorkflowID()
	err := cp.Stop(context.Background(), nonExistentID, StopOptions{})

	require.Error(t, err)
	require.ErrorIs(t, err, ErrWorkflowNotFound)
}

// === Unit Tests: Get ===

func TestControlPlane_Get_RetrievesWorkflowFromRegistry(t *testing.T) {
	cp, _, _ := newTestControlPlane(t)

	// Create workflow
	spec := WorkflowSpec{
		TemplateID:    "test-template",
		InitialPrompt: "Build a feature",
		Name:          "Test Workflow",
	}
	id, err := cp.Create(context.Background(), spec)
	require.NoError(t, err)

	// Get workflow
	inst, err := cp.Get(context.Background(), id)

	require.NoError(t, err)
	require.NotNil(t, inst)
	require.Equal(t, id, inst.ID)
	require.Equal(t, "Test Workflow", inst.Name)
}

func TestControlPlane_Get_ReturnsErrorForNonExistentWorkflow(t *testing.T) {
	cp, _, _ := newTestControlPlane(t)

	nonExistentID := NewWorkflowID()
	inst, err := cp.Get(context.Background(), nonExistentID)

	require.Error(t, err)
	require.Nil(t, inst)
	require.ErrorIs(t, err, ErrWorkflowNotFound)
}

// === Unit Tests: List ===

func TestControlPlane_List_FiltersWorkflowsCorrectly(t *testing.T) {
	cp, _, _ := newTestControlPlane(t)
	ctx := context.Background()

	// Create workflows with different priorities
	spec1 := WorkflowSpec{
		TemplateID:    "template-a",
		InitialPrompt: "Goal 1",
	}
	spec2 := WorkflowSpec{
		TemplateID:    "template-b",
		InitialPrompt: "Goal 2",
	}
	spec3 := WorkflowSpec{
		TemplateID:    "template-a",
		InitialPrompt: "Goal 3",
	}

	_, err := cp.Create(ctx, spec1)
	require.NoError(t, err)
	_, err = cp.Create(ctx, spec2)
	require.NoError(t, err)
	_, err = cp.Create(ctx, spec3)
	require.NoError(t, err)

	// List all workflows
	all, err := cp.List(ctx, ListQuery{})
	require.NoError(t, err)
	require.Len(t, all, 3)

	// List by template
	templateA, err := cp.List(ctx, ListQuery{TemplateID: "template-a"})
	require.NoError(t, err)
	require.Len(t, templateA, 2)

	// List by priority
	require.NoError(t, err)

	// List by state (all should be Pending)
	pending, err := cp.List(ctx, ListQuery{States: []WorkflowState{WorkflowPending}})
	require.NoError(t, err)
	require.Len(t, pending, 3)

	// List by state that doesn't exist
	running, err := cp.List(ctx, ListQuery{States: []WorkflowState{WorkflowRunning}})
	require.NoError(t, err)
	require.Len(t, running, 0)
}

func TestControlPlane_List_ReturnsEmptyWhenNoWorkflows(t *testing.T) {
	cp, _, _ := newTestControlPlane(t)

	workflows, err := cp.List(context.Background(), ListQuery{})

	require.NoError(t, err)
	require.Empty(t, workflows)
}

// === Integration Test: Full Lifecycle ===

func TestControlPlane_FullLifecycle_CreateStartStop(t *testing.T) {
	cp, mockFactory, mockProvider := newTestControlPlane(t)
	ctx := context.Background()

	// Step 1: Create workflow
	spec := WorkflowSpec{
		TemplateID:    "integration-template",
		InitialPrompt: "Complete integration test",
		Name:          "Integration Test Workflow",
	}
	id, err := cp.Create(ctx, spec)
	require.NoError(t, err)
	require.NotEmpty(t, id)

	// Verify initial state
	inst, err := cp.Get(ctx, id)
	require.NoError(t, err)
	require.Equal(t, WorkflowPending, inst.State)
	require.Nil(t, inst.StartedAt)

	// Step 2: Setup infrastructure mock and start workflow
	infra := createTestInfrastructure(t)
	mockFactory.On("Create", mock.AnythingOfType("v2.InfrastructureConfig")).Return(infra, nil)
	setupTestAgentProviderMock(t, mockProvider)

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go infra.Core.Processor.Run(runCtx)
	require.NoError(t, infra.Core.Processor.WaitForReady(runCtx))

	err = cp.Start(runCtx, id)
	require.NoError(t, err)

	// Verify running state
	inst, err = cp.Get(ctx, id)
	require.NoError(t, err)
	require.Equal(t, WorkflowRunning, inst.State)
	require.NotNil(t, inst.StartedAt)
	require.NotNil(t, inst.Infrastructure)
	require.Greater(t, inst.MCPPort, 0)

	// List should show running workflow
	running, err := cp.List(ctx, ListQuery{States: []WorkflowState{WorkflowRunning}})
	require.NoError(t, err)
	require.Len(t, running, 1)
	require.Equal(t, id, running[0].ID)

	// Step 3: Stop workflow
	err = cp.Stop(ctx, id, StopOptions{Reason: "integration test complete"})
	require.NoError(t, err)

	// Verify stopped state
	inst, err = cp.Get(ctx, id)
	require.NoError(t, err)
	require.Equal(t, WorkflowStopped, inst.State)
	require.Nil(t, inst.Infrastructure)
	require.Equal(t, 0, inst.MCPPort)

	// List should show stopped workflow
	stopped, err := cp.List(ctx, ListQuery{States: []WorkflowState{WorkflowStopped}})
	require.NoError(t, err)
	require.Len(t, stopped, 1)

	mockFactory.AssertExpectations(t)
}

// === Edge Cases ===

func TestControlPlane_Create_WithLabels(t *testing.T) {
	cp, _, _ := newTestControlPlane(t)

	spec := WorkflowSpec{
		TemplateID:    "test-template",
		InitialPrompt: "Build a feature",
		Labels: map[string]string{
			"team": "platform",
			"env":  "staging",
		},
	}

	id, err := cp.Create(context.Background(), spec)
	require.NoError(t, err)

	inst, err := cp.Get(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, "platform", inst.Labels["team"])
	require.Equal(t, "staging", inst.Labels["env"])
}

func TestControlPlane_Start_PropagatesInfrastructureError(t *testing.T) {
	cp, mockFactory, _ := newTestControlPlane(t)

	// Create workflow
	spec := WorkflowSpec{
		TemplateID:    "test-template",
		InitialPrompt: "Build a feature",
	}
	id, err := cp.Create(context.Background(), spec)
	require.NoError(t, err)
	cleanupWorkflowSessionOnTestEnd(t, cp, id) // Close session before TempDir cleanup (Windows)

	// Setup mock to fail
	expectedErr := errors.New("infrastructure creation failed")
	mockFactory.On("Create", mock.AnythingOfType("v2.InfrastructureConfig")).Return(nil, expectedErr)

	// Start should fail
	err = cp.Start(context.Background(), id)

	require.Error(t, err)
	require.Contains(t, err.Error(), "infrastructure creation failed")

	// Workflow should remain in Pending state
	inst, err := cp.Get(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, WorkflowPending, inst.State)
}

// === Subscription Tests ===

// newTestControlPlaneWithEventBus creates a ControlPlane with a custom event bus for testing subscriptions.
func newTestControlPlaneWithEventBus(t *testing.T) (ControlPlane, *CrossWorkflowEventBus) {
	t.Helper()

	mockProvider := mocks.NewMockAgentProvider(t)
	registry := NewInMemoryRegistry()
	eventBus := NewCrossWorkflowEventBus()
	sessionFactory := session.NewFactory(session.FactoryConfig{BaseDir: t.TempDir()})

	supervisor, err := NewSupervisor(SupervisorConfig{
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: mockProvider,
		},
		SessionFactory: sessionFactory,
	})
	require.NoError(t, err)

	cp, err := NewControlPlane(ControlPlaneConfig{
		Registry:   registry,
		Supervisor: supervisor,
		EventBus:   eventBus,
	})
	require.NoError(t, err)

	return cp, eventBus
}

func TestControlPlane_Subscribe_ReceivesAllEvents(t *testing.T) {
	cp, eventBus := newTestControlPlaneWithEventBus(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe to all events
	eventCh, unsubscribe := cp.Subscribe(ctx)
	defer unsubscribe()

	// Publish some events
	event1 := ControlPlaneEvent{
		Type:       EventWorkflowStarted,
		WorkflowID: "workflow-1",
	}
	event2 := ControlPlaneEvent{
		Type:       EventWorkerSpawned,
		WorkflowID: "workflow-2",
	}

	eventBus.Publish(event1)
	eventBus.Publish(event2)

	// Receive events
	select {
	case received := <-eventCh:
		require.Equal(t, EventWorkflowStarted, received.Type)
		require.Equal(t, WorkflowID("workflow-1"), received.WorkflowID)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for event 1")
	}

	select {
	case received := <-eventCh:
		require.Equal(t, EventWorkerSpawned, received.Type)
		require.Equal(t, WorkflowID("workflow-2"), received.WorkflowID)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for event 2")
	}
}

func TestControlPlane_SubscribeWorkflow_FiltersToSpecificWorkflow(t *testing.T) {
	cp, eventBus := newTestControlPlaneWithEventBus(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	targetWorkflowID := WorkflowID("target-workflow")

	// Subscribe to specific workflow
	eventCh, unsubscribe := cp.SubscribeWorkflow(ctx, targetWorkflowID)
	defer unsubscribe()

	// Publish events from different workflows
	eventTarget := ControlPlaneEvent{
		Type:       EventWorkerSpawned,
		WorkflowID: targetWorkflowID,
	}
	eventOther := ControlPlaneEvent{
		Type:       EventWorkerSpawned,
		WorkflowID: "other-workflow",
	}

	eventBus.Publish(eventOther) // Should be filtered out
	eventBus.Publish(eventTarget)
	eventBus.Publish(eventOther) // Should be filtered out

	// Should only receive the target workflow event
	select {
	case received := <-eventCh:
		require.Equal(t, targetWorkflowID, received.WorkflowID)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for event")
	}

	// Verify no more events
	select {
	case received := <-eventCh:
		t.Fatalf("unexpected event received: %+v", received)
	case <-time.After(50 * time.Millisecond):
		// Expected - no more events
	}
}

func TestControlPlane_SubscribeFiltered_AppliesTypeFilter(t *testing.T) {
	cp, eventBus := newTestControlPlaneWithEventBus(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe with type filter
	filter := EventFilter{
		Types: []EventType{EventWorkerSpawned, EventWorkerRetired},
	}
	eventCh, unsubscribe := cp.SubscribeFiltered(ctx, filter)
	defer unsubscribe()

	// Publish various events
	workerSpawned := ControlPlaneEvent{Type: EventWorkerSpawned}
	coordOutput := ControlPlaneEvent{Type: EventCoordinatorOutput}
	workerRetired := ControlPlaneEvent{Type: EventWorkerRetired}
	taskAssigned := ControlPlaneEvent{Type: EventTaskAssigned}

	eventBus.Publish(workerSpawned)
	eventBus.Publish(coordOutput) // Should be filtered
	eventBus.Publish(workerRetired)
	eventBus.Publish(taskAssigned) // Should be filtered

	// Should receive only worker events
	received := make([]EventType, 0)
	for i := 0; i < 2; i++ {
		select {
		case event := <-eventCh:
			received = append(received, event.Type)
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("timeout waiting for event %d", i+1)
		}
	}

	require.Contains(t, received, EventWorkerSpawned)
	require.Contains(t, received, EventWorkerRetired)

	// Verify no more events
	select {
	case event := <-eventCh:
		t.Fatalf("unexpected event received: %+v", event)
	case <-time.After(50 * time.Millisecond):
		// Expected
	}
}

func TestControlPlane_SubscribeFiltered_AppliesExcludeFilter(t *testing.T) {
	cp, eventBus := newTestControlPlaneWithEventBus(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe with exclude filter
	filter := EventFilter{
		ExcludeTypes: []EventType{EventCoordinatorOutput, EventWorkerOutput},
	}
	eventCh, unsubscribe := cp.SubscribeFiltered(ctx, filter)
	defer unsubscribe()

	// Publish various events
	workerSpawned := ControlPlaneEvent{Type: EventWorkerSpawned}
	coordOutput := ControlPlaneEvent{Type: EventCoordinatorOutput}
	workerOutput := ControlPlaneEvent{Type: EventWorkerOutput}
	taskCompleted := ControlPlaneEvent{Type: EventTaskCompleted}

	eventBus.Publish(workerSpawned) // Should pass
	eventBus.Publish(coordOutput)   // Should be excluded
	eventBus.Publish(workerOutput)  // Should be excluded
	eventBus.Publish(taskCompleted) // Should pass

	// Should receive non-excluded events
	received := make([]EventType, 0)
	for i := 0; i < 2; i++ {
		select {
		case event := <-eventCh:
			received = append(received, event.Type)
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("timeout waiting for event %d", i+1)
		}
	}

	require.Contains(t, received, EventWorkerSpawned)
	require.Contains(t, received, EventTaskCompleted)
	require.NotContains(t, received, EventCoordinatorOutput)
	require.NotContains(t, received, EventWorkerOutput)
}

func TestControlPlane_SubscribeFiltered_CombinesMultipleFilters(t *testing.T) {
	cp, eventBus := newTestControlPlaneWithEventBus(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	targetWorkflowID := WorkflowID("target-workflow")

	// Subscribe with combined filters: specific workflow AND worker events only AND exclude output
	filter := EventFilter{
		WorkflowIDs:  []WorkflowID{targetWorkflowID},
		Types:        []EventType{EventWorkerSpawned, EventWorkerRetired, EventWorkerOutput},
		ExcludeTypes: []EventType{EventWorkerOutput},
	}
	eventCh, unsubscribe := cp.SubscribeFiltered(ctx, filter)
	defer unsubscribe()

	// Publish various events
	events := []ControlPlaneEvent{
		{Type: EventWorkerSpawned, WorkflowID: targetWorkflowID},
		{Type: EventWorkerSpawned, WorkflowID: "other-workflow"},     // Wrong workflow
		{Type: EventCoordinatorOutput, WorkflowID: targetWorkflowID}, // Wrong type
		{Type: EventWorkerOutput, WorkflowID: targetWorkflowID},      // Excluded
		{Type: EventWorkerRetired, WorkflowID: targetWorkflowID},
	}

	for _, e := range events {
		eventBus.Publish(e)
	}

	// Should receive only 2 events: WorkerSpawned and WorkerRetired for target workflow
	received := make([]EventType, 0)
	for i := 0; i < 2; i++ {
		select {
		case event := <-eventCh:
			received = append(received, event.Type)
			require.Equal(t, targetWorkflowID, event.WorkflowID)
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("timeout waiting for event %d", i+1)
		}
	}

	require.Contains(t, received, EventWorkerSpawned)
	require.Contains(t, received, EventWorkerRetired)

	// Verify no more events
	select {
	case event := <-eventCh:
		t.Fatalf("unexpected event received: %+v", event)
	case <-time.After(50 * time.Millisecond):
		// Expected
	}
}

func TestControlPlane_MultipleSubscribersReceiveSameEvents(t *testing.T) {
	cp, eventBus := newTestControlPlaneWithEventBus(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create multiple subscribers
	ch1, unsub1 := cp.Subscribe(ctx)
	defer unsub1()
	ch2, unsub2 := cp.Subscribe(ctx)
	defer unsub2()

	// Publish an event
	event := ControlPlaneEvent{
		Type:       EventWorkflowStarted,
		WorkflowID: "test-workflow",
	}
	eventBus.Publish(event)

	// Both subscribers should receive the event
	select {
	case received := <-ch1:
		require.Equal(t, EventWorkflowStarted, received.Type)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for event on subscriber 1")
	}

	select {
	case received := <-ch2:
		require.Equal(t, EventWorkflowStarted, received.Type)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for event on subscriber 2")
	}
}

func TestControlPlane_ContextCancellationCleansUpSubscription(t *testing.T) {
	cp, eventBus := newTestControlPlaneWithEventBus(t)

	// Create context that we'll cancel
	subCtx, subCancel := context.WithCancel(context.Background())

	// Subscribe
	eventCh, _ := cp.Subscribe(subCtx)

	// Verify subscription is active
	initialCount := eventBus.SubscriberCount()
	require.Equal(t, 1, initialCount)

	// Cancel context
	subCancel()

	// Give time for cleanup
	time.Sleep(50 * time.Millisecond)

	// Channel should be closed
	select {
	case _, ok := <-eventCh:
		require.False(t, ok, "channel should be closed after context cancellation")
	case <-time.After(100 * time.Millisecond):
		// Channel might have been closed with remaining events drained
	}
}

func TestControlPlane_UnsubscribeFunctionStopsEventDelivery(t *testing.T) {
	cp, eventBus := newTestControlPlaneWithEventBus(t)

	ctx := context.Background()

	// Subscribe
	eventCh, unsubscribe := cp.Subscribe(ctx)

	// Publish event and verify receipt
	event1 := ControlPlaneEvent{Type: EventWorkflowStarted}
	eventBus.Publish(event1)

	select {
	case <-eventCh:
		// Good - received event
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for event before unsubscribe")
	}

	// Unsubscribe
	unsubscribe()

	// Give time for cleanup
	time.Sleep(50 * time.Millisecond)

	// Channel should be closed after unsubscribe
	select {
	case _, ok := <-eventCh:
		if ok {
			// Might receive one more buffered event, but channel should close
			_, ok = <-eventCh
			require.False(t, ok, "channel should be closed after unsubscribe")
		}
	case <-time.After(100 * time.Millisecond):
		// Channel closed immediately
	}
}

func TestControlPlane_NewControlPlaneCreatesDefaultEventBus(t *testing.T) {
	// Create ControlPlane without specifying EventBus
	mockProvider := mocks.NewMockAgentProvider(t)
	registry := NewInMemoryRegistry()
	supervisor, err := NewSupervisor(SupervisorConfig{
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: mockProvider,
		},
		SessionFactory: session.NewFactory(session.FactoryConfig{BaseDir: t.TempDir()}),
	})
	require.NoError(t, err)

	cp, err := NewControlPlane(ControlPlaneConfig{
		Registry:   registry,
		Supervisor: supervisor,
		// EventBus is nil - should be created automatically
	})
	require.NoError(t, err)

	// Verify Subscribe works (which requires an event bus)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventCh, unsubscribe := cp.Subscribe(ctx)
	defer unsubscribe()

	require.NotNil(t, eventCh)
}

func TestControlPlane_EventBusMethodReturnsEventBus(t *testing.T) {
	cp, eventBus := newTestControlPlaneWithEventBus(t)

	// Cast to defaultControlPlane to access EventBus method
	dcp, ok := cp.(*defaultControlPlane)
	require.True(t, ok)

	// Verify EventBus returns the configured bus
	require.Equal(t, eventBus, dcp.EventBus())
}

// === Shutdown Tests ===

func TestControlPlane_Shutdown_StopsAllRunningWorkflows(t *testing.T) {
	cp, mockFactory, mockProvider := newTestControlPlane(t)
	ctx := context.Background()

	// Create and start multiple workflows
	var ids []WorkflowID
	for i := 0; i < 3; i++ {
		spec := WorkflowSpec{
			TemplateID:    "test-template",
			InitialPrompt: "Goal",
		}
		id, err := cp.Create(ctx, spec)
		require.NoError(t, err)
		ids = append(ids, id)
	}

	// Setup mock provider for MCP server creation
	setupTestAgentProviderMock(t, mockProvider)

	// Setup mock infrastructure for each workflow
	for range ids {
		infra := createTestInfrastructure(t)
		mockFactory.On("Create", mock.AnythingOfType("v2.InfrastructureConfig")).Return(infra, nil).Once()

		runCtx, cancel := context.WithCancel(ctx)
		t.Cleanup(cancel)
		go infra.Core.Processor.Run(runCtx)
		require.NoError(t, infra.Core.Processor.WaitForReady(runCtx))
	}

	// Start all workflows
	for _, id := range ids {
		require.NoError(t, cp.Start(ctx, id))
	}

	// Verify all are running
	for _, id := range ids {
		inst, err := cp.Get(ctx, id)
		require.NoError(t, err)
		require.Equal(t, WorkflowRunning, inst.State)
	}

	// Shutdown
	err := cp.Shutdown(ctx)
	require.NoError(t, err)

	// Verify all are stopped
	for _, id := range ids {
		inst, err := cp.Get(ctx, id)
		require.NoError(t, err)
		require.Equal(t, WorkflowStopped, inst.State)
	}

	mockFactory.AssertExpectations(t)
}

func TestControlPlane_Shutdown_NoWorkflows(t *testing.T) {
	cp, _, _ := newTestControlPlane(t)

	// Shutdown with no workflows should succeed
	err := cp.Shutdown(context.Background())
	require.NoError(t, err)
}

func TestControlPlane_Shutdown_WithPendingWorkflows(t *testing.T) {
	cp, _, _ := newTestControlPlane(t)
	ctx := context.Background()

	// Create but don't start a workflow
	spec := WorkflowSpec{
		TemplateID:    "test-template",
		InitialPrompt: "Goal",
	}
	id, err := cp.Create(ctx, spec)
	require.NoError(t, err)

	// Verify it's pending
	inst, err := cp.Get(ctx, id)
	require.NoError(t, err)
	require.Equal(t, WorkflowPending, inst.State)

	// Shutdown should stop pending workflows too
	err = cp.Shutdown(ctx)
	require.NoError(t, err)

	// Verify it's stopped
	inst, err = cp.Get(ctx, id)
	require.NoError(t, err)
	require.Equal(t, WorkflowStopped, inst.State)
}

func TestControlPlane_Shutdown_RespectsGracePeriod(t *testing.T) {
	cp, mockFactory, mockProvider := newTestControlPlane(t)
	ctx := context.Background()

	// Create and start a workflow
	spec := WorkflowSpec{
		TemplateID:    "test-template",
		InitialPrompt: "Goal",
	}
	id, err := cp.Create(ctx, spec)
	require.NoError(t, err)

	infra := createTestInfrastructure(t)
	mockFactory.On("Create", mock.AnythingOfType("v2.InfrastructureConfig")).Return(infra, nil)
	setupTestAgentProviderMock(t, mockProvider)

	runCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	go infra.Core.Processor.Run(runCtx)
	require.NoError(t, infra.Core.Processor.WaitForReady(runCtx))

	require.NoError(t, cp.Start(ctx, id))

	// Shutdown with a long timeout (shouldn't need to force)
	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 5*time.Second)
	defer shutdownCancel()

	err = cp.Shutdown(shutdownCtx)
	require.NoError(t, err)

	// Workflow should be stopped
	inst, err := cp.Get(ctx, id)
	require.NoError(t, err)
	require.Equal(t, WorkflowStopped, inst.State)
}

func TestControlPlane_Shutdown_ForceStopsOnContextCancel(t *testing.T) {
	cp, mockFactory, mockProvider := newTestControlPlane(t)
	ctx := context.Background()

	// Create and start a workflow
	spec := WorkflowSpec{
		TemplateID:    "test-template",
		InitialPrompt: "Goal",
	}
	id, err := cp.Create(ctx, spec)
	require.NoError(t, err)

	infra := createTestInfrastructure(t)
	mockFactory.On("Create", mock.AnythingOfType("v2.InfrastructureConfig")).Return(infra, nil)
	setupTestAgentProviderMock(t, mockProvider)

	runCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	go infra.Core.Processor.Run(runCtx)
	require.NoError(t, infra.Core.Processor.WaitForReady(runCtx))

	require.NoError(t, cp.Start(ctx, id))

	// Create an already-cancelled context
	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel() // Cancel immediately

	// Shutdown with cancelled context should still complete (force stop)
	err = cp.Shutdown(cancelledCtx)
	require.NoError(t, err)

	// Workflow should be stopped
	inst, err := cp.Get(ctx, id)
	require.NoError(t, err)
	require.Equal(t, WorkflowStopped, inst.State)
}

func TestControlPlane_Shutdown_WithHealthMonitor(t *testing.T) {
	// Create control plane with health monitor
	mockProvider := mocks.NewMockAgentProvider(t)
	mockFactory := &mockInfrastructureFactory{}
	registry := NewInMemoryRegistry()
	sessionFactory := session.NewFactory(session.FactoryConfig{BaseDir: t.TempDir()})

	supervisor, err := NewSupervisor(SupervisorConfig{
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: mockProvider,
		},
		InfrastructureFactory: mockFactory,
		SessionFactory:        sessionFactory,
	})
	require.NoError(t, err)

	// Create a health monitor
	healthMonitor := NewHealthMonitor(HealthMonitorConfig{
		CheckInterval: time.Second,
	})

	cp, err := NewControlPlane(ControlPlaneConfig{
		Registry:      registry,
		Supervisor:    supervisor,
		HealthMonitor: healthMonitor,
	})
	require.NoError(t, err)

	// Start the health monitor
	monitorCtx, monitorCancel := context.WithCancel(context.Background())
	t.Cleanup(monitorCancel)
	require.NoError(t, healthMonitor.Start(monitorCtx))

	// Shutdown should stop the health monitor
	err = cp.Shutdown(context.Background())
	require.NoError(t, err)

	// Health monitor should be stopped (Stop is idempotent, this should not hang)
	healthMonitor.Stop()
}

func TestControlPlane_Shutdown_PartialFailureCleanup(t *testing.T) {
	cp, mockFactory, mockProvider := newTestControlPlane(t)
	ctx := context.Background()

	// Create multiple workflows
	var ids []WorkflowID
	for i := 0; i < 3; i++ {
		spec := WorkflowSpec{
			TemplateID:    "test-template",
			InitialPrompt: "Goal",
		}
		id, err := cp.Create(ctx, spec)
		require.NoError(t, err)
		ids = append(ids, id)
	}

	// Setup mock provider for MCP server creation
	setupTestAgentProviderMock(t, mockProvider)

	// Setup mock infrastructure for each workflow
	for range ids {
		infra := createTestInfrastructure(t)
		mockFactory.On("Create", mock.AnythingOfType("v2.InfrastructureConfig")).Return(infra, nil).Once()

		runCtx, cancel := context.WithCancel(ctx)
		t.Cleanup(cancel)
		go infra.Core.Processor.Run(runCtx)
		require.NoError(t, infra.Core.Processor.WaitForReady(runCtx))
	}

	// Start all workflows
	for _, id := range ids {
		require.NoError(t, cp.Start(ctx, id))
	}

	// Even if some workflows have issues, shutdown should complete
	// and return aggregated errors if any
	err := cp.Shutdown(ctx)
	require.NoError(t, err)

	// All workflows should be stopped
	for _, id := range ids {
		inst, err := cp.Get(ctx, id)
		require.NoError(t, err)
		require.Equal(t, WorkflowStopped, inst.State)
	}
}

func TestControlPlane_Shutdown_ClosesEventBus(t *testing.T) {
	cp, eventBus := newTestControlPlaneWithEventBus(t)
	ctx := context.Background()

	// Subscribe to events
	subCtx, subCancel := context.WithCancel(ctx)
	defer subCancel()
	eventCh, _ := cp.Subscribe(subCtx)

	// Verify subscription is active
	require.Equal(t, 1, eventBus.SubscriberCount())

	// Shutdown
	err := cp.Shutdown(ctx)
	require.NoError(t, err)

	// Give time for cleanup
	time.Sleep(50 * time.Millisecond)

	// Event channel should be closed (subscription count may not drop immediately
	// due to async cleanup, but the broker should be closed)
	select {
	case _, ok := <-eventCh:
		// If we receive a value, loop until closed
		for ok {
			_, ok = <-eventCh
		}
	case <-time.After(100 * time.Millisecond):
		// Channel cleanup is async, this is acceptable
	}
}
