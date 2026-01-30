package controlplane

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/flags"
	appgit "github.com/zjrosen/perles/internal/git/application"
	domaingit "github.com/zjrosen/perles/internal/git/domain"
	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/fabric"
	fabricpersist "github.com/zjrosen/perles/internal/orchestration/fabric/persistence"
	"github.com/zjrosen/perles/internal/orchestration/session"
	v2 "github.com/zjrosen/perles/internal/orchestration/v2"
	"github.com/zjrosen/perles/internal/orchestration/v2/adapter"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/handler"
	"github.com/zjrosen/perles/internal/orchestration/v2/processor"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
	"github.com/zjrosen/perles/internal/pubsub"
)

// === Mock Infrastructure Factory ===

type mockInfrastructureFactory struct {
	mock.Mock
}

func (m *mockInfrastructureFactory) Create(cfg v2.InfrastructureConfig) (*v2.Infrastructure, error) {
	args := m.Called(cfg)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*v2.Infrastructure), args.Error(1)
}

// === Mock Listener Factory ===

type mockListenerFactory struct {
	listener net.Listener
	err      error
}

func (m *mockListenerFactory) Create(address string) (net.Listener, error) {
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

// === Test Helper Functions ===

// newTestSupervisorConfig creates a SupervisorConfig with mocks for testing.
func newTestSupervisorConfig(t *testing.T) (SupervisorConfig, *mocks.MockAgentProvider, *mockInfrastructureFactory) {
	t.Helper()

	mockProvider := mocks.NewMockAgentProvider(t)
	mockFactory := &mockInfrastructureFactory{}
	sessionFactory := session.NewFactory(session.FactoryConfig{BaseDir: t.TempDir()})

	return SupervisorConfig{
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: mockProvider,
		},
		InfrastructureFactory: mockFactory,
		ListenerFactory:       &mockListenerFactory{},
		SessionFactory:        sessionFactory,
	}, mockProvider, mockFactory
}

// setupAgentProviderMock sets up common mock expectations for AgentProvider.
// The Supervisor.AllocateResources() method calls Client() and Extensions() when creating the MCP server.
func setupAgentProviderMock(t *testing.T, mockProvider *mocks.MockAgentProvider) {
	t.Helper()
	mockClient := mocks.NewMockHeadlessClient(t)
	mockProvider.On("Client").Return(mockClient, nil).Maybe()
	mockProvider.On("Extensions").Return(map[string]any{}).Maybe()
}

// cleanupSessionOnTestEnd registers a cleanup function to close the session
// when the test ends. This is required on Windows where files cannot be deleted
// while they are still open. Use this for tests that call Start() but not Stop().
func cleanupSessionOnTestEnd(t *testing.T, inst *WorkflowInstance) {
	t.Helper()
	t.Cleanup(func() {
		if inst.Session != nil {
			_ = inst.Session.Close(session.StatusCompleted)
		}
	})
}

// startWorkflow is a test helper that calls both AllocateResources and SpawnCoordinator.
// This simulates the full start flow that ControlPlane.Start() does.
func startWorkflow(ctx context.Context, s Supervisor, inst *WorkflowInstance) error {
	if err := s.AllocateResources(ctx, inst); err != nil {
		return err
	}
	return s.SpawnCoordinator(ctx, inst)
}

// createMinimalInfrastructure creates a minimal v2.Infrastructure suitable for testing.
// It sets up the Core.Processor, Core.EventBus, Core.Adapter, and Internal.TurnEnforcer
// so that Start/SubmitAndWait work.
func createMinimalInfrastructure(t *testing.T) *v2.Infrastructure {
	t.Helper()

	eventBus := pubsub.NewBroker[any]()
	cmdProcessor := processor.NewCommandProcessor(
		processor.WithQueueCapacity(100),
		processor.WithEventBus(eventBus),
	)

	// Register mock handlers for commands used during Start
	cmdProcessor.RegisterHandler(command.CmdSpawnProcess, &mockSpawnHandler{})
	cmdProcessor.RegisterHandler(command.CmdSendToProcess, &mockSendHandler{})

	// Create the v2 adapter for MCP server setup
	v2Adapter := adapter.NewV2Adapter(cmdProcessor)

	// Create turn enforcer for worker server cache
	turnEnforcer := handler.NewTurnCompletionTracker()

	// Create repositories for pause/resume tests
	processRepo := repository.NewMemoryProcessRepository()
	queueRepo := repository.NewMemoryQueueRepository(10)

	return &v2.Infrastructure{
		Core: v2.CoreComponents{
			Processor: cmdProcessor,
			EventBus:  eventBus,
			Adapter:   v2Adapter,
		},
		Internal: v2.InternalComponents{
			TurnEnforcer: turnEnforcer,
		},
		Repositories: v2.RepositoryComponents{
			ProcessRepo: processRepo,
			QueueRepo:   queueRepo,
		},
	}
}

// mockSpawnHandler is a simple handler that always succeeds for SpawnProcess commands.
type mockSpawnHandler struct{}

func (h *mockSpawnHandler) Handle(_ context.Context, cmd command.Command) (*command.CommandResult, error) {
	return &command.CommandResult{
		Success: true,
		Data:    &mockSpawnResult{processID: "coordinator"},
	}, nil
}

// mockSpawnResult implements the GetProcessID interface for spawn results.
type mockSpawnResult struct {
	processID string
}

func (r *mockSpawnResult) GetProcessID() string {
	return r.processID
}

// mockSendHandler is a simple handler that always succeeds for SendToProcess commands.
type mockSendHandler struct{}

func (h *mockSendHandler) Handle(_ context.Context, cmd command.Command) (*command.CommandResult, error) {
	return &command.CommandResult{Success: true}, nil
}

// === Unit Tests: NewSupervisor ===

func TestNewSupervisor_ValidConfig(t *testing.T) {
	cfg, _, _ := newTestSupervisorConfig(t)

	supervisor, err := NewSupervisor(cfg)

	require.NoError(t, err)
	require.NotNil(t, supervisor)
}

func TestNewSupervisor_MissingAgentProvider(t *testing.T) {
	cfg := SupervisorConfig{
		SessionFactory: session.NewFactory(session.FactoryConfig{BaseDir: t.TempDir()}),
		// AgentProviders is nil
	}

	supervisor, err := NewSupervisor(cfg)

	require.Error(t, err)
	require.Nil(t, supervisor)
	require.Contains(t, err.Error(), "AgentProviders is required")
}

func TestNewSupervisor_DefaultInfrastructureFactory(t *testing.T) {
	mockProvider := mocks.NewMockAgentProvider(t)

	cfg := SupervisorConfig{
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: mockProvider,
		},
		SessionFactory: session.NewFactory(session.FactoryConfig{BaseDir: t.TempDir()}),
		// InfrastructureFactory is nil - should use default
	}

	supervisor, err := NewSupervisor(cfg)

	require.NoError(t, err)
	require.NotNil(t, supervisor)
}

// === Unit Tests: AllocateResources + SpawnCoordinator ===

func TestSupervisor_FullStart_TransitionsPendingToRunning(t *testing.T) {
	cfg, mockProvider, mockFactory := newTestSupervisorConfig(t)
	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	inst := newTestInstance(t, "test-workflow")
	require.Equal(t, WorkflowPending, inst.State)
	cleanupSessionOnTestEnd(t, inst) // Close session before TempDir cleanup (Windows)

	// Setup mock infrastructure
	infra := createMinimalInfrastructure(t)
	mockFactory.On("Create", mock.AnythingOfType("v2.InfrastructureConfig")).Return(infra, nil)

	// Setup mock provider - AllocateResources() calls Client() and Extensions()
	setupAgentProviderMock(t, mockProvider)

	// Start the processor for the test
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go infra.Core.Processor.Run(ctx)
	require.NoError(t, infra.Core.Processor.WaitForReady(ctx))

	// Execute full start (AllocateResources + SpawnCoordinator)
	err = startWorkflow(ctx, supervisor, inst)

	require.NoError(t, err)
	require.Equal(t, WorkflowRunning, inst.State)
	require.NotNil(t, inst.Infrastructure)
	require.Greater(t, inst.MCPPort, 0)
	require.NotNil(t, inst.Ctx)
	require.NotNil(t, inst.Cancel)
	require.NotNil(t, inst.StartedAt)

	mockFactory.AssertExpectations(t)
}

func TestSupervisor_AllocateResources_RejectsNonPendingWorkflow(t *testing.T) {
	cfg, _, _ := newTestSupervisorConfig(t)
	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	inst := newTestInstance(t, "test-workflow")
	// Force workflow to Running state (bypass transition)
	inst.State = WorkflowRunning

	err = supervisor.AllocateResources(context.Background(), inst)

	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidState)
	require.Contains(t, err.Error(), "running")
	require.Contains(t, err.Error(), "pending")
}

func TestSupervisor_AllocateResources_RejectsCompleted(t *testing.T) {
	cfg, _, _ := newTestSupervisorConfig(t)
	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	inst := newTestInstance(t, "test-workflow")
	inst.State = WorkflowCompleted

	err = supervisor.AllocateResources(context.Background(), inst)

	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidState)
}

func TestSupervisor_AllocateResources_RejectsFailed(t *testing.T) {
	cfg, _, _ := newTestSupervisorConfig(t)
	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	inst := newTestInstance(t, "test-workflow")
	inst.State = WorkflowFailed

	err = supervisor.AllocateResources(context.Background(), inst)

	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidState)
}

func TestSupervisor_AllocateResources_AcquiresPort(t *testing.T) {
	cfg, mockProvider, mockFactory := newTestSupervisorConfig(t)
	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	inst := newTestInstance(t, "test-workflow")
	cleanupSessionOnTestEnd(t, inst) // Close session before TempDir cleanup (Windows)

	// Setup mock infrastructure
	infra := createMinimalInfrastructure(t)
	mockFactory.On("Create", mock.AnythingOfType("v2.InfrastructureConfig")).Return(infra, nil)

	// Setup mock provider - AllocateResources() calls Client() and Extensions()
	setupAgentProviderMock(t, mockProvider)

	// Start the processor
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go infra.Core.Processor.Run(ctx)
	require.NoError(t, infra.Core.Processor.WaitForReady(ctx))

	// Execute full start (AllocateResources + SpawnCoordinator)
	err = startWorkflow(ctx, supervisor, inst)

	require.NoError(t, err)
	// Port should be assigned by the OS (any valid port > 0)
	require.Greater(t, inst.MCPPort, 0)
}

func TestSupervisor_AllocateResources_CleansUpOnInfrastructureError(t *testing.T) {
	cfg, _, mockFactory := newTestSupervisorConfig(t)
	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	inst := newTestInstance(t, "test-workflow")
	cleanupSessionOnTestEnd(t, inst) // Close session before TempDir cleanup (Windows)

	// Make infrastructure creation fail
	expectedErr := errors.New("infrastructure creation failed")
	mockFactory.On("Create", mock.AnythingOfType("v2.InfrastructureConfig")).Return(nil, expectedErr)

	// Execute AllocateResources
	err = supervisor.AllocateResources(context.Background(), inst)

	require.Error(t, err)
	require.Contains(t, err.Error(), "creating infrastructure")
	// Workflow should remain in Pending state
	require.Equal(t, WorkflowPending, inst.State)

	mockFactory.AssertExpectations(t)
}

// === Unit Tests: Stop ===

func TestSupervisor_Shutdown_TransitionsRunningToFailed(t *testing.T) {
	cfg, mockProvider, mockFactory := newTestSupervisorConfig(t)
	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	// Create and start a workflow first
	inst := newTestInstance(t, "test-workflow")
	infra := createMinimalInfrastructure(t)
	mockFactory.On("Create", mock.AnythingOfType("v2.InfrastructureConfig")).Return(infra, nil)
	setupAgentProviderMock(t, mockProvider)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go infra.Core.Processor.Run(ctx)
	require.NoError(t, infra.Core.Processor.WaitForReady(ctx))

	require.NoError(t, startWorkflow(ctx, supervisor, inst))
	require.Equal(t, WorkflowRunning, inst.State)

	// Now stop it
	err = supervisor.Shutdown(ctx, inst, StopOptions{Reason: "test"})

	require.NoError(t, err)
	require.Equal(t, WorkflowFailed, inst.State)
	require.Nil(t, inst.Infrastructure)
	require.Equal(t, 0, inst.MCPPort)
	require.Nil(t, inst.Ctx)
	require.Nil(t, inst.Cancel)
}

func TestSupervisor_Shutdown_TransitionsPausedToFailed(t *testing.T) {
	cfg, _, _ := newTestSupervisorConfig(t)
	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	// Create a paused workflow
	inst := newTestInstance(t, "test-workflow")
	// Manually set to Paused (bypassing normal transition)
	inst.State = WorkflowPaused

	err = supervisor.Shutdown(context.Background(), inst, StopOptions{Reason: "test"})

	require.NoError(t, err)
	require.Equal(t, WorkflowFailed, inst.State)
}

func TestSupervisor_Shutdown_WithForce_SkipsGracefulShutdown(t *testing.T) {
	cfg, mockProvider, mockFactory := newTestSupervisorConfig(t)
	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	// Create and start a workflow
	inst := newTestInstance(t, "test-workflow")
	infra := createMinimalInfrastructure(t)
	mockFactory.On("Create", mock.AnythingOfType("v2.InfrastructureConfig")).Return(infra, nil)
	setupAgentProviderMock(t, mockProvider)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go infra.Core.Processor.Run(ctx)
	require.NoError(t, infra.Core.Processor.WaitForReady(ctx))

	require.NoError(t, startWorkflow(ctx, supervisor, inst))

	// Stop with Force=true
	err = supervisor.Shutdown(ctx, inst, StopOptions{Force: true})

	require.NoError(t, err)
	require.Equal(t, WorkflowFailed, inst.State)
}

func TestSupervisor_Shutdown_RejectsTerminalState(t *testing.T) {
	cfg, _, _ := newTestSupervisorConfig(t)
	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	testCases := []struct {
		name  string
		state WorkflowState
	}{
		{"Completed", WorkflowCompleted},
		{"Failed", WorkflowFailed},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			inst := newTestInstance(t, "test-workflow")
			inst.State = tc.state

			err := supervisor.Shutdown(context.Background(), inst, StopOptions{})

			require.Error(t, err)
			require.ErrorIs(t, err, ErrInvalidState)
		})
	}
}

func TestSupervisor_Shutdown_AllowsFromPendingState(t *testing.T) {
	cfg, _, _ := newTestSupervisorConfig(t)
	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	inst := newTestInstance(t, "test-workflow")
	// Instance is in Pending state by default
	require.Equal(t, WorkflowPending, inst.State)

	// According to the state machine, Pending -> Failed is a valid transition
	// This allows cancelling a workflow before it starts
	err = supervisor.Shutdown(context.Background(), inst, StopOptions{Reason: "cancelled before start"})

	require.NoError(t, err)
	require.Equal(t, WorkflowFailed, inst.State)
}

// === Integration-style Test ===

func TestSupervisor_StartShutdown_FullLifecycle(t *testing.T) {
	cfg, mockProvider, mockFactory := newTestSupervisorConfig(t)
	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	// Create a workflow
	inst := newTestInstance(t, "lifecycle-test")
	require.Equal(t, WorkflowPending, inst.State)

	// Setup mock infrastructure
	infra := createMinimalInfrastructure(t)
	mockFactory.On("Create", mock.AnythingOfType("v2.InfrastructureConfig")).Return(infra, nil)
	setupAgentProviderMock(t, mockProvider)

	// Start the processor
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go infra.Core.Processor.Run(ctx)
	require.NoError(t, infra.Core.Processor.WaitForReady(ctx))

	// Start the workflow
	err = startWorkflow(ctx, supervisor, inst)
	require.NoError(t, err)
	require.Equal(t, WorkflowRunning, inst.State)
	require.NotNil(t, inst.StartedAt)

	// Verify resources are allocated
	require.Greater(t, inst.MCPPort, 0)
	require.NotNil(t, inst.Infrastructure)

	// Stop the workflow
	err = supervisor.Shutdown(ctx, inst, StopOptions{Reason: "test complete"})
	require.NoError(t, err)
	require.Equal(t, WorkflowFailed, inst.State)

	// Verify resources are released
	require.Equal(t, 0, inst.MCPPort)
	require.Nil(t, inst.Infrastructure)

	mockFactory.AssertExpectations(t)
}

// === Unit Tests: Pause ===

func TestSupervisor_Pause_ReturnsErrInvalidStateIfNotRunning(t *testing.T) {
	cfg, _, _ := newTestSupervisorConfig(t)
	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	testCases := []struct {
		name  string
		state WorkflowState
	}{
		{"Pending", WorkflowPending},
		{"Paused", WorkflowPaused},
		{"Completed", WorkflowCompleted},
		{"Failed", WorkflowFailed},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			inst := newTestInstance(t, "test-workflow")
			inst.State = tc.state

			err := supervisor.Pause(context.Background(), inst)

			require.Error(t, err)
			require.ErrorIs(t, err, ErrInvalidState)
			require.Contains(t, err.Error(), tc.state.String())
		})
	}
}

func TestSupervisor_Pause_TransitionsRunningToPaused(t *testing.T) {
	cfg, _, _ := newTestSupervisorConfig(t)
	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	inst := newTestInstance(t, "test-workflow")
	inst.State = WorkflowRunning

	err = supervisor.Pause(context.Background(), inst)

	require.NoError(t, err)
	require.Equal(t, WorkflowPaused, inst.State)
}

func TestSupervisor_Pause_WithInfrastructure_DoesNotPanic(t *testing.T) {
	cfg, _, _ := newTestSupervisorConfig(t)
	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	inst := newTestInstance(t, "test-workflow")
	inst.State = WorkflowRunning

	// Create real infrastructure - verifies that Pause doesn't panic when infrastructure exists
	inst.Infrastructure = createMinimalInfrastructure(t)

	err = supervisor.Pause(context.Background(), inst)

	require.NoError(t, err)
	require.Equal(t, WorkflowPaused, inst.State)
}

func TestSupervisor_Pause_WithNilInfrastructure_DoesNotPanic(t *testing.T) {
	cfg, _, _ := newTestSupervisorConfig(t)
	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	inst := newTestInstance(t, "test-workflow")
	inst.State = WorkflowRunning
	inst.Infrastructure = nil // explicitly nil

	err = supervisor.Pause(context.Background(), inst)

	require.NoError(t, err)
	require.Equal(t, WorkflowPaused, inst.State)
}

func TestSupervisor_Pause_SetsPausedAtTimestamp(t *testing.T) {
	cfg, _, _ := newTestSupervisorConfig(t)
	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	inst := newTestInstance(t, "test-workflow")
	inst.State = WorkflowRunning
	require.True(t, inst.PausedAt.IsZero(), "PausedAt should be zero before pause")

	beforePause := time.Now()
	err = supervisor.Pause(context.Background(), inst)
	afterPause := time.Now()

	require.NoError(t, err)
	require.False(t, inst.PausedAt.IsZero(), "PausedAt should be set after pause")
	require.True(t, inst.PausedAt.After(beforePause) || inst.PausedAt.Equal(beforePause),
		"PausedAt should be at or after beforePause")
	require.True(t, inst.PausedAt.Before(afterPause) || inst.PausedAt.Equal(afterPause),
		"PausedAt should be at or before afterPause")
}

// === Unit Tests: Resume ===

func TestSupervisor_Resume_ReturnsErrInvalidStateIfNotPaused(t *testing.T) {
	cfg, _, _ := newTestSupervisorConfig(t)
	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	testCases := []struct {
		name  string
		state WorkflowState
	}{
		{"Pending", WorkflowPending},
		{"Running", WorkflowRunning},
		{"Completed", WorkflowCompleted},
		{"Failed", WorkflowFailed},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			inst := newTestInstance(t, "test-workflow")
			inst.State = tc.state

			err := supervisor.Resume(context.Background(), inst)

			require.Error(t, err)
			require.ErrorIs(t, err, ErrInvalidState)
			require.Contains(t, err.Error(), tc.state.String())
		})
	}
}

func TestSupervisor_Resume_TransitionsPausedToRunning(t *testing.T) {
	cfg, mockProvider, mockFactory := newTestSupervisorConfig(t)
	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	inst := newTestInstance(t, "test-workflow")
	inst.State = WorkflowPaused
	inst.PausedAt = time.Now().Add(-5 * time.Minute)

	// Setup infrastructure for resume command
	infra := createMinimalInfrastructure(t)
	mockFactory.On("Create", mock.AnythingOfType("v2.InfrastructureConfig")).Return(infra, nil).Maybe()
	setupAgentProviderMock(t, mockProvider)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go infra.Core.Processor.Run(ctx)
	require.NoError(t, infra.Core.Processor.WaitForReady(ctx))

	inst.Infrastructure = infra
	inst.Ctx = ctx

	// Add coordinator to process repository so resume can find it
	coord := &repository.Process{
		ID:     repository.CoordinatorID,
		Role:   repository.RoleCoordinator,
		Status: repository.StatusPaused,
	}
	require.NoError(t, infra.Repositories.ProcessRepo.Save(coord))

	// Register handlers needed for resume
	infra.Core.Processor.RegisterHandler(command.CmdResumeProcess, &handlerTracker{})

	err = supervisor.Resume(ctx, inst)

	require.NoError(t, err)
	require.Equal(t, WorkflowRunning, inst.State)
}

func TestSupervisor_Resume_CallsResumeProcess(t *testing.T) {
	cfg, mockProvider, mockFactory := newTestSupervisorConfig(t)
	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	inst := newTestInstance(t, "test-workflow")
	inst.State = WorkflowPaused
	inst.PausedAt = time.Now().Add(-5 * time.Minute)

	// Setup infrastructure
	infra := createMinimalInfrastructure(t)
	mockFactory.On("Create", mock.AnythingOfType("v2.InfrastructureConfig")).Return(infra, nil).Maybe()
	setupAgentProviderMock(t, mockProvider)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go infra.Core.Processor.Run(ctx)
	require.NoError(t, infra.Core.Processor.WaitForReady(ctx))

	inst.Infrastructure = infra
	inst.Ctx = ctx

	// Add coordinator to process repository so resume can find it
	coord := &repository.Process{
		ID:     repository.CoordinatorID,
		Role:   repository.RoleCoordinator,
		Status: repository.StatusPaused,
	}
	require.NoError(t, infra.Repositories.ProcessRepo.Save(coord))

	// Track if resume and send handlers were called
	resumeCalled := false
	sendCalled := false
	infra.Core.Processor.RegisterHandler(command.CmdResumeProcess, &handlerTracker{onHandle: func() { resumeCalled = true }})
	infra.Core.Processor.RegisterHandler(command.CmdSendToProcess, &handlerTracker{onHandle: func() { sendCalled = true }})

	err = supervisor.Resume(ctx, inst)

	require.NoError(t, err)
	require.True(t, resumeCalled, "ResumeProcess command should be submitted")
	require.True(t, sendCalled, "SendToProcess command should be submitted for resume message")
}

func TestSupervisor_Resume_RollsBackOnResumeFailure(t *testing.T) {
	cfg, mockProvider, mockFactory := newTestSupervisorConfig(t)
	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	inst := newTestInstance(t, "test-workflow")
	inst.State = WorkflowPaused
	inst.PausedAt = time.Now().Add(-5 * time.Minute)

	// Setup infrastructure with failing resume handler
	infra := createMinimalInfrastructure(t)
	mockFactory.On("Create", mock.AnythingOfType("v2.InfrastructureConfig")).Return(infra, nil).Maybe()
	setupAgentProviderMock(t, mockProvider)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go infra.Core.Processor.Run(ctx)
	require.NoError(t, infra.Core.Processor.WaitForReady(ctx))

	inst.Infrastructure = infra
	inst.Ctx = ctx

	// Add coordinator to process repository
	coord := &repository.Process{
		ID:     repository.CoordinatorID,
		Role:   repository.RoleCoordinator,
		Status: repository.StatusPaused,
	}
	require.NoError(t, infra.Repositories.ProcessRepo.Save(coord))

	// Register failing resume handler
	failingErr := errors.New("resume failed")
	infra.Core.Processor.RegisterHandler(command.CmdResumeProcess, &failingHandler{err: failingErr})

	err = supervisor.Resume(ctx, inst)

	require.Error(t, err)
	require.Contains(t, err.Error(), "resuming coordinator")
	// State should rollback to Paused
	require.Equal(t, WorkflowPaused, inst.State)
}

func TestSupervisor_Resume_WithNilNudger_DoesNotPanic(t *testing.T) {
	cfg, mockProvider, mockFactory := newTestSupervisorConfig(t)
	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	inst := newTestInstance(t, "test-workflow")
	inst.State = WorkflowPaused
	inst.PausedAt = time.Now().Add(-5 * time.Minute)

	// Setup infrastructure
	infra := createMinimalInfrastructure(t)
	mockFactory.On("Create", mock.AnythingOfType("v2.InfrastructureConfig")).Return(infra, nil).Maybe()
	setupAgentProviderMock(t, mockProvider)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go infra.Core.Processor.Run(ctx)
	require.NoError(t, infra.Core.Processor.WaitForReady(ctx))

	inst.Infrastructure = infra
	inst.Ctx = ctx

	// Add coordinator to process repository so resume can find it
	coord := &repository.Process{
		ID:     repository.CoordinatorID,
		Role:   repository.RoleCoordinator,
		Status: repository.StatusPaused,
	}
	require.NoError(t, infra.Repositories.ProcessRepo.Save(coord))

	// Register handlers needed for resume
	infra.Core.Processor.RegisterHandler(command.CmdResumeProcess, &handlerTracker{})
	infra.Core.Processor.RegisterHandler(command.CmdSendToProcess, &handlerTracker{})

	err = supervisor.Resume(ctx, inst)

	require.NoError(t, err)
	require.Equal(t, WorkflowRunning, inst.State)
}

// === Test Helpers for Pause/Resume ===

// handlerTracker is a generic handler that tracks when commands are received.
type handlerTracker struct {
	onHandle func()
}

func (h *handlerTracker) Handle(_ context.Context, _ command.Command) (*command.CommandResult, error) {
	if h.onHandle != nil {
		h.onHandle()
	}
	return &command.CommandResult{
		Success: true,
		Data:    nil,
	}, nil
}

// spawnTracker is a handler that tracks spawn commands.
type spawnTracker struct {
	onHandle func()
}

func (h *spawnTracker) Handle(_ context.Context, _ command.Command) (*command.CommandResult, error) {
	if h.onHandle != nil {
		h.onHandle()
	}
	return &command.CommandResult{
		Success: true,
		Data:    &mockSpawnResult{processID: "coordinator"},
	}, nil
}

// failingHandler is a generic handler that returns an error.
type failingHandler struct {
	err error
}

func (h *failingHandler) Handle(_ context.Context, _ command.Command) (*command.CommandResult, error) {
	return &command.CommandResult{
		Success: false,
		Error:   h.err,
	}, nil
}

// failingSpawnHandler is a handler that returns an error.
type failingSpawnHandler struct {
	err error
}

func (h *failingSpawnHandler) Handle(_ context.Context, _ command.Command) (*command.CommandResult, error) {
	return &command.CommandResult{
		Success: false,
		Error:   h.err,
	}, nil
}

// === Unit Tests: SupervisorConfig worktree fields ===

func TestSupervisor_Config_AcceptsGitExecutorFactory(t *testing.T) {
	mockProvider := mocks.NewMockAgentProvider(t)

	// Create a mock GitExecutor factory
	factoryCalled := false
	gitFactory := func(workDir string) appgit.GitExecutor {
		factoryCalled = true
		return nil
	}

	cfg := SupervisorConfig{
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: mockProvider,
		},
		SessionFactory:     session.NewFactory(session.FactoryConfig{BaseDir: t.TempDir()}),
		GitExecutorFactory: gitFactory,
	}

	supervisor, err := NewSupervisor(cfg)

	require.NoError(t, err)
	require.NotNil(t, supervisor)

	// Access the concrete type to verify the factory was stored
	ds := supervisor.(*defaultSupervisor)
	require.NotNil(t, ds.gitExecutorFactory)

	// Call the factory to verify it works
	ds.gitExecutorFactory("/tmp")
	require.True(t, factoryCalled)
}

func TestSupervisor_Config_DefaultWorktreeTimeout(t *testing.T) {
	mockProvider := mocks.NewMockAgentProvider(t)

	// Create config without specifying WorktreeTimeout
	cfg := SupervisorConfig{
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: mockProvider,
		},
		SessionFactory: session.NewFactory(session.FactoryConfig{BaseDir: t.TempDir()}),
	}

	supervisor, err := NewSupervisor(cfg)

	require.NoError(t, err)
	require.NotNil(t, supervisor)

	// Access the concrete type to verify default was applied
	ds := supervisor.(*defaultSupervisor)
	require.Equal(t, DefaultWorktreeTimeout, ds.worktreeTimeout)
	require.Equal(t, 30*time.Second, ds.worktreeTimeout)
}

func TestSupervisor_Config_CustomWorktreeTimeout(t *testing.T) {
	mockProvider := mocks.NewMockAgentProvider(t)

	customTimeout := 60 * time.Second
	cfg := SupervisorConfig{
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: mockProvider,
		},
		SessionFactory:  session.NewFactory(session.FactoryConfig{BaseDir: t.TempDir()}),
		WorktreeTimeout: customTimeout,
	}

	supervisor, err := NewSupervisor(cfg)

	require.NoError(t, err)
	require.NotNil(t, supervisor)

	ds := supervisor.(*defaultSupervisor)
	require.Equal(t, customTimeout, ds.worktreeTimeout)
}

func TestSupervisor_Config_AcceptsFlags(t *testing.T) {
	mockProvider := mocks.NewMockAgentProvider(t)

	flagsRegistry := flags.New(map[string]bool{
		flags.FlagSessionResume: true,
	})

	cfg := SupervisorConfig{
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: mockProvider,
		},
		SessionFactory: session.NewFactory(session.FactoryConfig{BaseDir: t.TempDir()}),
		Flags:          flagsRegistry,
	}

	supervisor, err := NewSupervisor(cfg)

	require.NoError(t, err)
	require.NotNil(t, supervisor)

	ds := supervisor.(*defaultSupervisor)
	require.NotNil(t, ds.flags)
	require.True(t, ds.flags.Enabled(flags.FlagSessionResume))
}

// === Unit Tests: Start() with Worktree ===

// newTestSpecWithWorktree creates a WorkflowSpec with worktree enabled for testing.
func newTestSpecWithWorktree(name, baseBranch, customBranch string) *WorkflowSpec {
	return &WorkflowSpec{
		TemplateID:         "test-template",
		InitialPrompt:      "Test goal",
		Name:               name,
		WorktreeEnabled:    true,
		WorktreeBaseBranch: baseBranch,
		WorktreeBranchName: customBranch,
	}
}

// newTestInstanceWithWorktree creates a WorkflowInstance with worktree enabled for testing.
func newTestInstanceWithWorktree(t *testing.T, name, baseBranch, customBranch string) *WorkflowInstance {
	t.Helper()
	inst, err := NewWorkflowInstance(newTestSpecWithWorktree(name, baseBranch, customBranch))
	require.NoError(t, err)
	return inst
}

func TestSupervisor_Start_CreatesWorktreeWhenEnabled(t *testing.T) {
	mockGitExecutor := mocks.NewMockGitExecutor(t)
	mockProvider := mocks.NewMockAgentProvider(t)
	mockFactory := &mockInfrastructureFactory{}

	cfg := SupervisorConfig{
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: mockProvider,
		},
		ListenerFactory:       &mockListenerFactory{},
		InfrastructureFactory: mockFactory,
		SessionFactory:        session.NewFactory(session.FactoryConfig{BaseDir: t.TempDir()}),
		GitExecutorFactory: func(workDir string) appgit.GitExecutor {
			return mockGitExecutor
		},
	}

	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	inst := newTestInstanceWithWorktree(t, "worktree-test", "main", "")
	cleanupSessionOnTestEnd(t, inst) // Close session before TempDir cleanup (Windows)
	workflowID := inst.ID.String()
	expectedPath := "/tmp/worktrees/" + workflowID
	expectedBranch := "perles-workflow-" + workflowID[:8]

	// Setup mock expectations for worktree creation
	mockGitExecutor.EXPECT().PruneWorktrees().Return(nil)
	mockGitExecutor.EXPECT().DetermineWorktreePath(workflowID).Return(expectedPath, nil)
	mockGitExecutor.EXPECT().CreateWorktreeWithContext(
		mock.Anything, expectedPath, expectedBranch, "main",
	).Return(nil)

	// Setup mock infrastructure
	infra := createMinimalInfrastructure(t)
	mockFactory.On("Create", mock.AnythingOfType("v2.InfrastructureConfig")).Return(infra, nil)
	setupAgentProviderMock(t, mockProvider)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go infra.Core.Processor.Run(ctx)
	require.NoError(t, infra.Core.Processor.WaitForReady(ctx))

	// Execute Start
	err = startWorkflow(ctx, supervisor, inst)

	require.NoError(t, err)
	require.Equal(t, WorkflowRunning, inst.State)
	require.Equal(t, expectedPath, inst.WorktreePath)
	require.Equal(t, expectedBranch, inst.WorktreeBranch)
	require.Equal(t, expectedPath, inst.WorkDir) // WorkDir should be updated to worktree path
}

func TestSupervisor_Start_SkipsWorktreeWhenDisabled(t *testing.T) {
	mockProvider := mocks.NewMockAgentProvider(t)
	mockFactory := &mockInfrastructureFactory{}

	factoryCalled := false
	cfg := SupervisorConfig{
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: mockProvider,
		},
		ListenerFactory:       &mockListenerFactory{},
		InfrastructureFactory: mockFactory,
		SessionFactory:        session.NewFactory(session.FactoryConfig{BaseDir: t.TempDir()}),
		GitExecutorFactory: func(workDir string) appgit.GitExecutor {
			factoryCalled = true
			return mocks.NewMockGitExecutor(t)
		},
	}

	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	// Instance without worktree enabled
	inst := newTestInstance(t, "no-worktree-test")
	cleanupSessionOnTestEnd(t, inst) // Close session before TempDir cleanup (Windows)
	require.False(t, inst.WorktreeEnabled)

	// Setup mock infrastructure
	infra := createMinimalInfrastructure(t)
	mockFactory.On("Create", mock.AnythingOfType("v2.InfrastructureConfig")).Return(infra, nil)
	setupAgentProviderMock(t, mockProvider)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go infra.Core.Processor.Run(ctx)
	require.NoError(t, infra.Core.Processor.WaitForReady(ctx))

	// Execute Start
	err = startWorkflow(ctx, supervisor, inst)

	require.NoError(t, err)
	require.Equal(t, WorkflowRunning, inst.State)
	require.Empty(t, inst.WorktreePath)
	require.Empty(t, inst.WorktreeBranch)
	require.False(t, factoryCalled, "GitExecutorFactory should not be called when worktree is disabled")
}

func TestSupervisor_Start_UsesCustomBranchNameWhenSet(t *testing.T) {
	mockGitExecutor := mocks.NewMockGitExecutor(t)
	mockProvider := mocks.NewMockAgentProvider(t)
	mockFactory := &mockInfrastructureFactory{}

	cfg := SupervisorConfig{
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: mockProvider,
		},
		ListenerFactory:       &mockListenerFactory{},
		InfrastructureFactory: mockFactory,
		SessionFactory:        session.NewFactory(session.FactoryConfig{BaseDir: t.TempDir()}),
		GitExecutorFactory: func(workDir string) appgit.GitExecutor {
			return mockGitExecutor
		},
	}

	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	customBranch := "my-custom-branch"
	inst := newTestInstanceWithWorktree(t, "custom-branch-test", "develop", customBranch)
	cleanupSessionOnTestEnd(t, inst) // Close session before TempDir cleanup (Windows)
	workflowID := inst.ID.String()
	expectedPath := "/tmp/worktrees/" + workflowID

	// Setup mock expectations - should use custom branch name
	mockGitExecutor.EXPECT().PruneWorktrees().Return(nil)
	mockGitExecutor.EXPECT().DetermineWorktreePath(workflowID).Return(expectedPath, nil)
	mockGitExecutor.EXPECT().CreateWorktreeWithContext(
		mock.Anything, expectedPath, customBranch, "develop",
	).Return(nil)

	// Setup mock infrastructure
	infra := createMinimalInfrastructure(t)
	mockFactory.On("Create", mock.AnythingOfType("v2.InfrastructureConfig")).Return(infra, nil)
	setupAgentProviderMock(t, mockProvider)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go infra.Core.Processor.Run(ctx)
	require.NoError(t, infra.Core.Processor.WaitForReady(ctx))

	// Execute Start
	err = startWorkflow(ctx, supervisor, inst)

	require.NoError(t, err)
	require.Equal(t, customBranch, inst.WorktreeBranch)
}

func TestSupervisor_Start_AutoGeneratesBranchNameWhenEmpty(t *testing.T) {
	mockGitExecutor := mocks.NewMockGitExecutor(t)
	mockProvider := mocks.NewMockAgentProvider(t)
	mockFactory := &mockInfrastructureFactory{}

	cfg := SupervisorConfig{
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: mockProvider,
		},
		ListenerFactory:       &mockListenerFactory{},
		InfrastructureFactory: mockFactory,
		SessionFactory:        session.NewFactory(session.FactoryConfig{BaseDir: t.TempDir()}),
		GitExecutorFactory: func(workDir string) appgit.GitExecutor {
			return mockGitExecutor
		},
	}

	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	// Empty WorktreeBranchName - should auto-generate
	inst := newTestInstanceWithWorktree(t, "auto-branch-test", "main", "")
	cleanupSessionOnTestEnd(t, inst) // Close session before TempDir cleanup (Windows)
	workflowID := inst.ID.String()
	expectedPath := "/tmp/worktrees/" + workflowID
	expectedBranch := "perles-workflow-" + workflowID[:8]

	// Setup mock expectations
	mockGitExecutor.EXPECT().PruneWorktrees().Return(nil)
	mockGitExecutor.EXPECT().DetermineWorktreePath(workflowID).Return(expectedPath, nil)
	mockGitExecutor.EXPECT().CreateWorktreeWithContext(
		mock.Anything, expectedPath, expectedBranch, "main",
	).Return(nil)

	// Setup mock infrastructure
	infra := createMinimalInfrastructure(t)
	mockFactory.On("Create", mock.AnythingOfType("v2.InfrastructureConfig")).Return(infra, nil)
	setupAgentProviderMock(t, mockProvider)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go infra.Core.Processor.Run(ctx)
	require.NoError(t, infra.Core.Processor.WaitForReady(ctx))

	// Execute Start
	err = startWorkflow(ctx, supervisor, inst)

	require.NoError(t, err)
	require.Equal(t, expectedBranch, inst.WorktreeBranch)
	require.Contains(t, inst.WorktreeBranch, "perles-workflow-")
}

func TestSupervisor_Start_SetsInstanceFieldsCorrectly(t *testing.T) {
	mockGitExecutor := mocks.NewMockGitExecutor(t)
	mockProvider := mocks.NewMockAgentProvider(t)
	mockFactory := &mockInfrastructureFactory{}

	cfg := SupervisorConfig{
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: mockProvider,
		},
		ListenerFactory:       &mockListenerFactory{},
		InfrastructureFactory: mockFactory,
		SessionFactory:        session.NewFactory(session.FactoryConfig{BaseDir: t.TempDir()}),
		GitExecutorFactory: func(workDir string) appgit.GitExecutor {
			return mockGitExecutor
		},
	}

	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	inst := newTestInstanceWithWorktree(t, "fields-test", "main", "custom-branch")
	cleanupSessionOnTestEnd(t, inst) // Close session before TempDir cleanup (Windows)
	workflowID := inst.ID.String()
	worktreePath := "/tmp/custom-worktree-path"
	branchName := "custom-branch"

	// Setup mock expectations
	mockGitExecutor.EXPECT().PruneWorktrees().Return(nil)
	mockGitExecutor.EXPECT().DetermineWorktreePath(workflowID).Return(worktreePath, nil)
	mockGitExecutor.EXPECT().CreateWorktreeWithContext(
		mock.Anything, worktreePath, branchName, "main",
	).Return(nil)

	// Setup mock infrastructure
	infra := createMinimalInfrastructure(t)
	mockFactory.On("Create", mock.AnythingOfType("v2.InfrastructureConfig")).Return(infra, nil)
	setupAgentProviderMock(t, mockProvider)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go infra.Core.Processor.Run(ctx)
	require.NoError(t, infra.Core.Processor.WaitForReady(ctx))

	// Execute Start
	err = startWorkflow(ctx, supervisor, inst)

	require.NoError(t, err)
	require.Equal(t, worktreePath, inst.WorktreePath, "WorktreePath should be set")
	require.Equal(t, branchName, inst.WorktreeBranch, "WorktreeBranch should be set")
	require.Equal(t, worktreePath, inst.WorkDir, "WorkDir should be overridden to worktree path")
}

func TestSupervisor_Start_CleansUpWorktreeOnSubsequentFailure(t *testing.T) {
	mockGitExecutor := mocks.NewMockGitExecutor(t)
	mockProvider := mocks.NewMockAgentProvider(t)
	mockFactory := &mockInfrastructureFactory{}

	cfg := SupervisorConfig{
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: mockProvider,
		},
		ListenerFactory:       &mockListenerFactory{},
		InfrastructureFactory: mockFactory,
		SessionFactory:        session.NewFactory(session.FactoryConfig{BaseDir: t.TempDir()}),
		GitExecutorFactory: func(workDir string) appgit.GitExecutor {
			return mockGitExecutor
		},
	}

	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	inst := newTestInstanceWithWorktree(t, "cleanup-test", "main", "")
	cleanupSessionOnTestEnd(t, inst) // Close session before TempDir cleanup (Windows)
	workflowID := inst.ID.String()
	worktreePath := "/tmp/worktrees/" + workflowID
	expectedBranch := "perles-workflow-" + workflowID[:8]

	// Setup mock expectations for successful worktree creation
	mockGitExecutor.EXPECT().PruneWorktrees().Return(nil)
	mockGitExecutor.EXPECT().DetermineWorktreePath(workflowID).Return(worktreePath, nil)
	mockGitExecutor.EXPECT().CreateWorktreeWithContext(
		mock.Anything, worktreePath, expectedBranch, "main",
	).Return(nil)

	// Setup RemoveWorktree expectation - should be called during cleanup
	mockGitExecutor.EXPECT().RemoveWorktree(worktreePath).Return(nil)

	// Make infrastructure creation fail to trigger cleanup
	infrastructureErr := errors.New("infrastructure creation failed")
	mockFactory.On("Create", mock.AnythingOfType("v2.InfrastructureConfig")).Return(nil, infrastructureErr)

	// Execute Start
	err = startWorkflow(context.Background(), supervisor, inst)

	require.Error(t, err)
	require.Contains(t, err.Error(), "creating infrastructure")
	require.Equal(t, WorkflowPending, inst.State) // Should stay in Pending state
}

func TestSupervisor_Start_HandlesErrBranchAlreadyCheckedOut(t *testing.T) {
	mockGitExecutor := mocks.NewMockGitExecutor(t)
	mockProvider := mocks.NewMockAgentProvider(t)

	cfg := SupervisorConfig{
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: mockProvider,
		},
		ListenerFactory: &mockListenerFactory{},
		SessionFactory:  session.NewFactory(session.FactoryConfig{BaseDir: t.TempDir()}),
		GitExecutorFactory: func(workDir string) appgit.GitExecutor {
			return mockGitExecutor
		},
	}

	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	inst := newTestInstanceWithWorktree(t, "branch-error-test", "main", "existing-branch")
	workflowID := inst.ID.String()
	worktreePath := "/tmp/worktrees/" + workflowID

	// Setup mock expectations
	mockGitExecutor.EXPECT().PruneWorktrees().Return(nil)
	mockGitExecutor.EXPECT().DetermineWorktreePath(workflowID).Return(worktreePath, nil)
	mockGitExecutor.EXPECT().CreateWorktreeWithContext(
		mock.Anything, worktreePath, "existing-branch", "main",
	).Return(fmt.Errorf("%w: already in use", domaingit.ErrBranchAlreadyCheckedOut))

	// Execute Start
	err = startWorkflow(context.Background(), supervisor, inst)

	require.Error(t, err)
	require.ErrorIs(t, err, domaingit.ErrBranchAlreadyCheckedOut)
	require.Contains(t, err.Error(), "already checked out in another worktree")
	require.Contains(t, err.Error(), "existing-branch")
	require.Equal(t, WorkflowPending, inst.State) // Should stay in Pending state
}

func TestSupervisor_Start_HandlesTimeoutCorrectly(t *testing.T) {
	mockGitExecutor := mocks.NewMockGitExecutor(t)
	mockProvider := mocks.NewMockAgentProvider(t)

	cfg := SupervisorConfig{
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: mockProvider,
		},
		ListenerFactory: &mockListenerFactory{},
		SessionFactory:  session.NewFactory(session.FactoryConfig{BaseDir: t.TempDir()}),
		WorktreeTimeout: 100 * time.Millisecond, // Short timeout for test
		GitExecutorFactory: func(workDir string) appgit.GitExecutor {
			return mockGitExecutor
		},
	}

	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	inst := newTestInstanceWithWorktree(t, "timeout-test", "main", "")
	workflowID := inst.ID.String()
	worktreePath := "/tmp/worktrees/" + workflowID
	expectedBranch := "perles-workflow-" + workflowID[:8]

	// Setup mock expectations - CreateWorktreeWithContext returns timeout error
	mockGitExecutor.EXPECT().PruneWorktrees().Return(nil)
	mockGitExecutor.EXPECT().DetermineWorktreePath(workflowID).Return(worktreePath, nil)
	mockGitExecutor.EXPECT().CreateWorktreeWithContext(
		mock.Anything, worktreePath, expectedBranch, "main",
	).Return(domaingit.ErrWorktreeTimeout)

	// Execute Start
	err = startWorkflow(context.Background(), supervisor, inst)

	require.Error(t, err)
	require.ErrorIs(t, err, domaingit.ErrWorktreeTimeout)
	require.Contains(t, err.Error(), "timed out")
	require.Equal(t, WorkflowPending, inst.State) // Should stay in Pending state
}

// === Unit Tests: Stop() with Worktree Cleanup ===

func TestSupervisor_Shutdown_ReturnsErrUncommittedChanges(t *testing.T) {
	mockGitExecutor := mocks.NewMockGitExecutor(t)
	mockProvider := mocks.NewMockAgentProvider(t)

	cfg := SupervisorConfig{
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: mockProvider,
		},
		ListenerFactory: &mockListenerFactory{},
		SessionFactory:  session.NewFactory(session.FactoryConfig{BaseDir: t.TempDir()}),
		GitExecutorFactory: func(workDir string) appgit.GitExecutor {
			return mockGitExecutor
		},
	}

	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	// Create a workflow in Running state with a worktree
	inst := newTestInstance(t, "uncommitted-test")
	inst.State = WorkflowRunning
	inst.WorktreePath = "/tmp/worktree-uncommitted"
	inst.WorktreeBranch = "test-branch"

	// Setup mock to report uncommitted changes
	mockGitExecutor.EXPECT().HasUncommittedChanges().Return(true, nil)

	// Execute Stop without Force
	err = supervisor.Shutdown(context.Background(), inst, StopOptions{
		Reason: "test stop",
		Force:  false,
	})

	require.Error(t, err)
	require.ErrorIs(t, err, ErrUncommittedChanges)
	require.Contains(t, err.Error(), "please commit, stash, or force stop")
	require.Equal(t, WorkflowRunning, inst.State) // Should NOT transition
}

func TestSupervisor_Shutdown_BypassesUncommittedCheckWhenForceTrue(t *testing.T) {
	mockGitExecutor := mocks.NewMockGitExecutor(t)
	mockProvider := mocks.NewMockAgentProvider(t)

	cfg := SupervisorConfig{
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: mockProvider,
		},
		ListenerFactory: &mockListenerFactory{},
		SessionFactory:  session.NewFactory(session.FactoryConfig{BaseDir: t.TempDir()}),
		GitExecutorFactory: func(workDir string) appgit.GitExecutor {
			return mockGitExecutor
		},
	}

	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	// Create a workflow in Running state with a worktree
	inst := newTestInstance(t, "force-stop-test")
	inst.State = WorkflowRunning
	inst.WorktreePath = "/tmp/worktree-force"
	inst.WorktreeBranch = "test-branch"

	// HasUncommittedChanges should NOT be called when Force=true
	// No mock setup for HasUncommittedChanges - it should not be called

	// Execute Stop with Force=true
	err = supervisor.Shutdown(context.Background(), inst, StopOptions{
		Reason: "force stop test",
		Force:  true,
	})

	require.NoError(t, err)
	require.Equal(t, WorkflowFailed, inst.State)
	// Verify HasUncommittedChanges was never called
	mockGitExecutor.AssertNotCalled(t, "HasUncommittedChanges")
}

// === Session Factory Integration Tests ===

func TestSupervisor_Start_CreatesSessionWhenFactoryConfigured(t *testing.T) {
	cfg, mockProvider, mockFactory := newTestSupervisorConfig(t)

	// Configure session factory with temp directory
	sessionBaseDir := t.TempDir()
	cfg.SessionFactory = session.NewFactory(session.FactoryConfig{
		BaseDir: sessionBaseDir,
	})

	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	inst := newTestInstance(t, "test-workflow-with-session")
	cleanupSessionOnTestEnd(t, inst) // Close session before TempDir cleanup (Windows)

	// Setup mock infrastructure
	infra := createMinimalInfrastructure(t)
	mockFactory.On("Create", mock.AnythingOfType("v2.InfrastructureConfig")).Return(infra, nil)
	setupAgentProviderMock(t, mockProvider)

	// Start the processor for the test
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go infra.Core.Processor.Run(ctx)
	require.NoError(t, infra.Core.Processor.WaitForReady(ctx))

	// Execute full start (AllocateResources + SpawnCoordinator)
	err = startWorkflow(ctx, supervisor, inst)

	require.NoError(t, err)
	require.Equal(t, WorkflowRunning, inst.State)

	// Verify session was created and attached to instance
	require.NotNil(t, inst.Session, "Session should be created when factory is configured")
	require.NotEmpty(t, inst.Session.Dir, "Session directory should be set")
	require.DirExists(t, inst.Session.Dir, "Session directory should exist on disk")

	// Verify session ID matches workflow ID
	require.Equal(t, inst.ID.String(), inst.Session.ID, "Session ID should match workflow ID")

	mockFactory.AssertExpectations(t)
}

func TestSupervisor_Shutdown_ClosesSession(t *testing.T) {
	cfg, mockProvider, mockFactory := newTestSupervisorConfig(t)
	// SessionFactory already provided by newTestSupervisorConfig
	sessionBaseDir := t.TempDir()
	cfg.SessionFactory = session.NewFactory(session.FactoryConfig{
		BaseDir: sessionBaseDir,
	})

	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	inst := newTestInstance(t, "test-workflow-stop-session")

	// Setup mock infrastructure
	infra := createMinimalInfrastructure(t)
	mockFactory.On("Create", mock.AnythingOfType("v2.InfrastructureConfig")).Return(infra, nil)
	setupAgentProviderMock(t, mockProvider)

	// Start the processor and workflow
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go infra.Core.Processor.Run(ctx)
	require.NoError(t, infra.Core.Processor.WaitForReady(ctx))

	err = startWorkflow(ctx, supervisor, inst)
	require.NoError(t, err)
	require.NotNil(t, inst.Session)

	sessionDir := inst.Session.Dir

	// Stop the workflow
	err = supervisor.Shutdown(ctx, inst, StopOptions{})

	require.NoError(t, err)
	require.Equal(t, WorkflowFailed, inst.State)

	// Session should be nil (closed)
	require.Nil(t, inst.Session, "Session should be set to nil after close")

	// Session directory should still exist (data preserved)
	require.DirExists(t, sessionDir, "Session directory should persist after close")

	// Verify metadata shows completed status
	meta, err := session.Load(sessionDir)
	require.NoError(t, err)
	require.Equal(t, session.StatusCompleted, meta.Status, "Session should be marked completed")
}

// === Unit Tests: Fabric Forwarder ===

func TestFabricForwarder_PublishesToEventBus(t *testing.T) {
	// Test that the fabricForwarder pattern correctly publishes fabric.Event to the event bus.
	// This tests the forwarder function logic in isolation.

	eventBus := pubsub.NewBroker[any]()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe to the event bus
	eventCh := eventBus.Subscribe(ctx)

	// Create the forwarder function (same pattern as in AllocateResources)
	fabricForwarder := func(event fabric.Event) {
		eventBus.Publish(pubsub.UpdatedEvent, event)
	}

	// Create a test fabric event
	testEvent := fabric.Event{
		Type:        fabric.EventMessagePosted,
		ChannelID:   "ch-123",
		ChannelSlug: "tasks",
		AgentID:     "COORDINATOR",
	}

	// Call the forwarder
	fabricForwarder(testEvent)

	// Verify the event was published to the bus
	select {
	case received := <-eventCh:
		require.Equal(t, pubsub.UpdatedEvent, received.Type)
		fabricEvent, ok := received.Payload.(fabric.Event)
		require.True(t, ok, "payload should be fabric.Event")
		require.Equal(t, fabric.EventMessagePosted, fabricEvent.Type)
		require.Equal(t, "ch-123", fabricEvent.ChannelID)
		require.Equal(t, "tasks", fabricEvent.ChannelSlug)
		require.Equal(t, "COORDINATOR", fabricEvent.AgentID)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for event on bus")
	}
}

func TestChainHandler_ThreeHandlers(t *testing.T) {
	// Test that ChainHandler correctly calls all three handlers in sequence.
	// This verifies the pattern used in supervisor.go works with 3 handlers.

	var calls []string

	h1 := func(e fabric.Event) { calls = append(calls, "logger:"+string(e.Type)) }
	h2 := func(e fabric.Event) { calls = append(calls, "broker:"+string(e.Type)) }
	h3 := func(e fabric.Event) { calls = append(calls, "forwarder:"+string(e.Type)) }

	// Use the same ChainHandler pattern as in supervisor.go
	chained := fabricpersist.ChainHandler(h1, h2, h3)

	event := fabric.Event{Type: fabric.EventMessagePosted}
	chained(event)

	// All three handlers should be called in order
	require.Len(t, calls, 3)
	require.Equal(t, "logger:message.posted", calls[0])
	require.Equal(t, "broker:message.posted", calls[1])
	require.Equal(t, "forwarder:message.posted", calls[2])
}

func TestFabricEventPipeline_EndToEnd(t *testing.T) {
	// Integration test: verify event flows from FabricService through ChainHandler to EventBus.
	// This simulates the complete event pipeline set up in AllocateResources.

	eventBus := pubsub.NewBroker[any]()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe to the event bus
	eventCh := eventBus.Subscribe(ctx)

	// Track what handlers were called
	var handlersCalled []string
	var mu sync.Mutex

	// Create mock handlers like those in AllocateResources
	mockLogger := func(e fabric.Event) {
		mu.Lock()
		handlersCalled = append(handlersCalled, "logger")
		mu.Unlock()
	}
	mockBroker := func(e fabric.Event) {
		mu.Lock()
		handlersCalled = append(handlersCalled, "broker")
		mu.Unlock()
	}
	fabricForwarder := func(event fabric.Event) {
		mu.Lock()
		handlersCalled = append(handlersCalled, "forwarder")
		mu.Unlock()
		eventBus.Publish(pubsub.UpdatedEvent, event)
	}

	// Create the chained handler
	chained := fabricpersist.ChainHandler(mockLogger, mockBroker, fabricForwarder)

	// Simulate FabricService emitting an event
	testEvent := fabric.Event{
		Type:        fabric.EventReplyPosted,
		ChannelID:   "ch-general",
		ChannelSlug: "general",
		AgentID:     "WORKER.1",
		ParentID:    "msg-123",
	}

	// Emit through the chain (simulates FabricService.emit())
	chained(testEvent)

	// Verify all handlers were called
	mu.Lock()
	require.Equal(t, []string{"logger", "broker", "forwarder"}, handlersCalled)
	mu.Unlock()

	// Verify the event reached the event bus
	select {
	case received := <-eventCh:
		fabricEvent, ok := received.Payload.(fabric.Event)
		require.True(t, ok)
		require.Equal(t, fabric.EventReplyPosted, fabricEvent.Type)
		require.Equal(t, "general", fabricEvent.ChannelSlug)
		require.Equal(t, "msg-123", fabricEvent.ParentID)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for event on bus")
	}
}

func TestExistingHandlers_StillCalled(t *testing.T) {
	// Verify that adding the forwarder doesn't break the existing handlers.
	// EventLogger and Broker handlers should still receive all events.

	eventBus := pubsub.NewBroker[any]()

	// Track handler invocations
	var loggerEvents []fabric.Event
	var brokerEvents []fabric.Event
	var forwarderEvents []fabric.Event
	var mu sync.Mutex

	mockLogger := func(e fabric.Event) {
		mu.Lock()
		loggerEvents = append(loggerEvents, e)
		mu.Unlock()
	}
	mockBroker := func(e fabric.Event) {
		mu.Lock()
		brokerEvents = append(brokerEvents, e)
		mu.Unlock()
	}
	fabricForwarder := func(e fabric.Event) {
		mu.Lock()
		forwarderEvents = append(forwarderEvents, e)
		mu.Unlock()
		eventBus.Publish(pubsub.UpdatedEvent, e)
	}

	chained := fabricpersist.ChainHandler(mockLogger, mockBroker, fabricForwarder)

	// Send multiple different event types
	events := []fabric.Event{
		{Type: fabric.EventMessagePosted, ChannelID: "ch-1"},
		{Type: fabric.EventReplyPosted, ChannelID: "ch-2"},
		{Type: fabric.EventSubscribed, ChannelID: "ch-3"},
		{Type: fabric.EventChannelCreated, ChannelID: "ch-4"},
	}

	for _, e := range events {
		chained(e)
	}

	mu.Lock()
	defer mu.Unlock()

	// All handlers should receive all events
	require.Len(t, loggerEvents, 4, "EventLogger should receive all events")
	require.Len(t, brokerEvents, 4, "Broker should receive all events")
	require.Len(t, forwarderEvents, 4, "Forwarder should receive all events")

	// Verify event types were preserved
	require.Equal(t, fabric.EventMessagePosted, loggerEvents[0].Type)
	require.Equal(t, fabric.EventReplyPosted, brokerEvents[1].Type)
	require.Equal(t, fabric.EventSubscribed, forwarderEvents[2].Type)
}

// === Unit Tests: SpawnObserver ===

func TestSupervisor_SpawnCoordinator_SpawnsObserverWhenEnabled(t *testing.T) {
	mockObserverProvider := mocks.NewMockAgentProvider(t)
	mockCoordinatorProvider := mocks.NewMockAgentProvider(t)
	mockFactory := &mockInfrastructureFactory{}

	cfg := SupervisorConfig{
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: mockCoordinatorProvider,
			client.RoleObserver:    mockObserverProvider, // Observer enabled
		},
		InfrastructureFactory: mockFactory,
		ListenerFactory:       &mockListenerFactory{},
		SessionFactory:        session.NewFactory(session.FactoryConfig{BaseDir: t.TempDir()}),
	}

	// Verify observer provider is set
	require.NotNil(t, cfg.AgentProviders[client.RoleObserver], "Observer provider should be set")

	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	// Verify supervisor has observer provider
	ds := supervisor.(*defaultSupervisor)
	_, hasObserver := ds.agentProviders[client.RoleObserver]
	require.True(t, hasObserver, "Supervisor should have observer provider")

	inst := newTestInstance(t, "observer-test")
	cleanupSessionOnTestEnd(t, inst)

	// Setup mock infrastructure
	infra := createMinimalInfrastructure(t)
	mockFactory.On("Create", mock.AnythingOfType("v2.InfrastructureConfig")).Return(infra, nil)
	setupAgentProviderMock(t, mockCoordinatorProvider)
	setupAgentProviderMock(t, mockObserverProvider)

	// Track spawn commands with role logging
	spawnCalls := 0
	var spawnRoles []string
	infra.Core.Processor.RegisterHandler(command.CmdSpawnProcess, &roleBasedSpawnHandler{
		onCoordinator: func() (*command.CommandResult, error) {
			spawnCalls++
			spawnRoles = append(spawnRoles, "coordinator")
			return &command.CommandResult{Success: true, Data: &mockSpawnResult{processID: "coordinator"}}, nil
		},
		onObserver: func() (*command.CommandResult, error) {
			spawnCalls++
			spawnRoles = append(spawnRoles, "observer")
			return &command.CommandResult{Success: true, Data: &mockSpawnResult{processID: "observer"}}, nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go infra.Core.Processor.Run(ctx)
	require.NoError(t, infra.Core.Processor.WaitForReady(ctx))

	// Execute full start
	err = startWorkflow(ctx, supervisor, inst)

	require.NoError(t, err)
	require.Equal(t, WorkflowRunning, inst.State)

	// Debug output
	t.Logf("Spawn calls: %d", spawnCalls)
	t.Logf("Spawn roles: %v", spawnRoles)
	t.Logf("inst.Infrastructure nil: %v", inst.Infrastructure == nil)
	if inst.Infrastructure != nil {
		t.Logf("inst.Infrastructure.Core.Processor nil: %v", inst.Infrastructure.Core.Processor == nil)
	}

	// Should have spawned both coordinator and observer
	require.Equal(t, 2, spawnCalls, "Should spawn both coordinator and observer")
}

func TestSupervisor_SpawnCoordinator_SkipsObserverWhenDisabled(t *testing.T) {
	mockCoordinatorProvider := mocks.NewMockAgentProvider(t)
	mockFactory := &mockInfrastructureFactory{}

	cfg := SupervisorConfig{
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: mockCoordinatorProvider,
			// No RoleObserver - observer disabled
		},
		InfrastructureFactory: mockFactory,
		ListenerFactory:       &mockListenerFactory{},
		SessionFactory:        session.NewFactory(session.FactoryConfig{BaseDir: t.TempDir()}),
	}

	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	inst := newTestInstance(t, "no-observer-test")
	cleanupSessionOnTestEnd(t, inst)

	// Setup mock infrastructure
	infra := createMinimalInfrastructure(t)
	mockFactory.On("Create", mock.AnythingOfType("v2.InfrastructureConfig")).Return(infra, nil)
	setupAgentProviderMock(t, mockCoordinatorProvider)

	// Track spawn commands
	spawnCalls := 0
	infra.Core.Processor.RegisterHandler(command.CmdSpawnProcess, &spawnTracker{onHandle: func() {
		spawnCalls++
	}})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go infra.Core.Processor.Run(ctx)
	require.NoError(t, infra.Core.Processor.WaitForReady(ctx))

	// Execute full start
	err = startWorkflow(ctx, supervisor, inst)

	require.NoError(t, err)
	require.Equal(t, WorkflowRunning, inst.State)
	// Should only spawn coordinator (no observer)
	require.Equal(t, 1, spawnCalls, "Should only spawn coordinator when observer disabled")
}

func TestSupervisor_SpawnObserver_FailOpenOnError(t *testing.T) {
	mockObserverProvider := mocks.NewMockAgentProvider(t)
	mockCoordinatorProvider := mocks.NewMockAgentProvider(t)
	mockFactory := &mockInfrastructureFactory{}

	cfg := SupervisorConfig{
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: mockCoordinatorProvider,
			client.RoleObserver:    mockObserverProvider,
		},
		InfrastructureFactory: mockFactory,
		ListenerFactory:       &mockListenerFactory{},
		SessionFactory:        session.NewFactory(session.FactoryConfig{BaseDir: t.TempDir()}),
	}

	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	inst := newTestInstance(t, "failopen-test")
	cleanupSessionOnTestEnd(t, inst)

	// Setup mock infrastructure
	infra := createMinimalInfrastructure(t)
	mockFactory.On("Create", mock.AnythingOfType("v2.InfrastructureConfig")).Return(infra, nil)
	setupAgentProviderMock(t, mockCoordinatorProvider)
	setupAgentProviderMock(t, mockObserverProvider)

	// Track spawn commands and fail observer spawn
	spawnCallCount := 0
	infra.Core.Processor.RegisterHandler(command.CmdSpawnProcess, &roleBasedSpawnHandler{
		onCoordinator: func() (*command.CommandResult, error) {
			spawnCallCount++
			return &command.CommandResult{Success: true, Data: &mockSpawnResult{processID: "coordinator"}}, nil
		},
		onObserver: func() (*command.CommandResult, error) {
			spawnCallCount++
			// Return error for observer spawn
			return &command.CommandResult{Success: false, Error: errors.New("observer spawn failed")}, nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go infra.Core.Processor.Run(ctx)
	require.NoError(t, infra.Core.Processor.WaitForReady(ctx))

	// Execute full start - should succeed even though observer failed
	err = startWorkflow(ctx, supervisor, inst)

	// Workflow should still succeed (fail-open behavior)
	require.NoError(t, err, "Workflow should succeed even when observer fails to spawn")
	require.Equal(t, WorkflowRunning, inst.State)
	require.Equal(t, 2, spawnCallCount, "Both coordinator and observer spawn should be attempted")
}

func TestSupervisor_SpawnObserver_AfterCoordinator(t *testing.T) {
	mockObserverProvider := mocks.NewMockAgentProvider(t)
	mockCoordinatorProvider := mocks.NewMockAgentProvider(t)
	mockFactory := &mockInfrastructureFactory{}

	cfg := SupervisorConfig{
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: mockCoordinatorProvider,
			client.RoleObserver:    mockObserverProvider,
		},
		InfrastructureFactory: mockFactory,
		ListenerFactory:       &mockListenerFactory{},
		SessionFactory:        session.NewFactory(session.FactoryConfig{BaseDir: t.TempDir()}),
	}

	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	inst := newTestInstance(t, "spawn-order-test")
	cleanupSessionOnTestEnd(t, inst)

	// Setup mock infrastructure
	infra := createMinimalInfrastructure(t)
	mockFactory.On("Create", mock.AnythingOfType("v2.InfrastructureConfig")).Return(infra, nil)
	setupAgentProviderMock(t, mockCoordinatorProvider)
	setupAgentProviderMock(t, mockObserverProvider)

	// Track spawn order
	var spawnOrder []string
	infra.Core.Processor.RegisterHandler(command.CmdSpawnProcess, &roleBasedSpawnHandler{
		onCoordinator: func() (*command.CommandResult, error) {
			spawnOrder = append(spawnOrder, "coordinator")
			return &command.CommandResult{Success: true, Data: &mockSpawnResult{processID: "coordinator"}}, nil
		},
		onObserver: func() (*command.CommandResult, error) {
			spawnOrder = append(spawnOrder, "observer")
			return &command.CommandResult{Success: true, Data: &mockSpawnResult{processID: "observer"}}, nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go infra.Core.Processor.Run(ctx)
	require.NoError(t, infra.Core.Processor.WaitForReady(ctx))

	// Execute full start
	err = startWorkflow(ctx, supervisor, inst)

	require.NoError(t, err)
	require.Equal(t, WorkflowRunning, inst.State)
	// Observer should spawn after coordinator (sequential, not parallel)
	require.Equal(t, []string{"coordinator", "observer"}, spawnOrder, "Observer should spawn after coordinator")
}

// roleBasedSpawnHandler handles spawn commands differently based on the role.
type roleBasedSpawnHandler struct {
	onCoordinator func() (*command.CommandResult, error)
	onObserver    func() (*command.CommandResult, error)
	onWorker      func() (*command.CommandResult, error)
}

func (h *roleBasedSpawnHandler) Handle(_ context.Context, cmd command.Command) (*command.CommandResult, error) {
	spawnCmd := cmd.(*command.SpawnProcessCommand)
	switch spawnCmd.Role {
	case repository.RoleCoordinator:
		if h.onCoordinator != nil {
			return h.onCoordinator()
		}
	case repository.RoleObserver:
		if h.onObserver != nil {
			return h.onObserver()
		}
	case repository.RoleWorker:
		if h.onWorker != nil {
			return h.onWorker()
		}
	}
	// Default success
	return &command.CommandResult{
		Success: true,
		Data:    &mockSpawnResult{processID: string(spawnCmd.Role)},
	}, nil
}
