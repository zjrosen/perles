package controlplane

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/flags"
	appgit "github.com/zjrosen/perles/internal/git/application"
	domaingit "github.com/zjrosen/perles/internal/git/domain"
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

func TestSupervisor_AllocateResources_RejectsStopped(t *testing.T) {
	cfg, _, _ := newTestSupervisorConfig(t)
	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	inst := newTestInstance(t, "test-workflow")
	inst.State = WorkflowStopped

	err = supervisor.AllocateResources(context.Background(), inst)

	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidState)
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

func TestSupervisor_Stop_TransitionsRunningToStopped(t *testing.T) {
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
	err = supervisor.Stop(ctx, inst, StopOptions{Reason: "test"})

	require.NoError(t, err)
	require.Equal(t, WorkflowStopped, inst.State)
	require.Nil(t, inst.Infrastructure)
	require.Equal(t, 0, inst.MCPPort)
	require.Nil(t, inst.Ctx)
	require.Nil(t, inst.Cancel)
}

func TestSupervisor_Stop_TransitionsPausedToStopped(t *testing.T) {
	cfg, _, _ := newTestSupervisorConfig(t)
	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	// Create a paused workflow
	inst := newTestInstance(t, "test-workflow")
	// Manually set to Paused (bypassing normal transition)
	inst.State = WorkflowPaused

	err = supervisor.Stop(context.Background(), inst, StopOptions{Reason: "test"})

	require.NoError(t, err)
	require.Equal(t, WorkflowStopped, inst.State)
}

func TestSupervisor_Stop_WithForce_SkipsGracefulShutdown(t *testing.T) {
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
	err = supervisor.Stop(ctx, inst, StopOptions{Force: true})

	require.NoError(t, err)
	require.Equal(t, WorkflowStopped, inst.State)
}

func TestSupervisor_Stop_RejectsTerminalState(t *testing.T) {
	cfg, _, _ := newTestSupervisorConfig(t)
	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	testCases := []struct {
		name  string
		state WorkflowState
	}{
		{"Stopped", WorkflowStopped},
		{"Completed", WorkflowCompleted},
		{"Failed", WorkflowFailed},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			inst := newTestInstance(t, "test-workflow")
			inst.State = tc.state

			err := supervisor.Stop(context.Background(), inst, StopOptions{})

			require.Error(t, err)
			require.ErrorIs(t, err, ErrInvalidState)
		})
	}
}

func TestSupervisor_Stop_AllowsFromPendingState(t *testing.T) {
	cfg, _, _ := newTestSupervisorConfig(t)
	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	inst := newTestInstance(t, "test-workflow")
	// Instance is in Pending state by default
	require.Equal(t, WorkflowPending, inst.State)

	// According to the state machine, Pending -> Stopped is a valid transition
	// This allows cancelling a workflow before it starts
	err = supervisor.Stop(context.Background(), inst, StopOptions{Reason: "cancelled before start"})

	require.NoError(t, err)
	require.Equal(t, WorkflowStopped, inst.State)
}

// === Integration-style Test ===

func TestSupervisor_StartStop_FullLifecycle(t *testing.T) {
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
	err = supervisor.Stop(ctx, inst, StopOptions{Reason: "test complete"})
	require.NoError(t, err)
	require.Equal(t, WorkflowStopped, inst.State)

	// Verify resources are released
	require.Equal(t, 0, inst.MCPPort)
	require.Nil(t, inst.Infrastructure)

	mockFactory.AssertExpectations(t)
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
		flags.FlagRemoveWorktree: true,
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
	require.True(t, ds.flags.Enabled(flags.FlagRemoveWorktree))
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
	// RemoveWorktree should have been called (verified by mock expectations)
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

func TestSupervisor_Stop_ReturnsErrUncommittedChanges(t *testing.T) {
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
	err = supervisor.Stop(context.Background(), inst, StopOptions{
		Reason: "test stop",
		Force:  false,
	})

	require.Error(t, err)
	require.ErrorIs(t, err, ErrUncommittedChanges)
	require.Contains(t, err.Error(), "please commit, stash, or force stop")
	require.Equal(t, WorkflowRunning, inst.State) // Should NOT transition
}

func TestSupervisor_Stop_BypassesUncommittedCheckWhenForceTrue(t *testing.T) {
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
	err = supervisor.Stop(context.Background(), inst, StopOptions{
		Reason: "force stop test",
		Force:  true,
	})

	require.NoError(t, err)
	require.Equal(t, WorkflowStopped, inst.State)
	// Verify HasUncommittedChanges was never called
	mockGitExecutor.AssertNotCalled(t, "HasUncommittedChanges")
}

func TestSupervisor_Stop_RemovesWorktreeWhenFlagEnabled(t *testing.T) {
	mockGitExecutor := mocks.NewMockGitExecutor(t)
	mockProvider := mocks.NewMockAgentProvider(t)

	flagsRegistry := flags.New(map[string]bool{
		flags.FlagRemoveWorktree: true,
	})

	cfg := SupervisorConfig{
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: mockProvider,
		},
		ListenerFactory: &mockListenerFactory{},
		Flags:           flagsRegistry,
		SessionFactory:  session.NewFactory(session.FactoryConfig{BaseDir: t.TempDir()}),
		GitExecutorFactory: func(workDir string) appgit.GitExecutor {
			return mockGitExecutor
		},
	}

	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	// Create a workflow in Running state with a worktree
	inst := newTestInstance(t, "remove-worktree-test")
	inst.State = WorkflowRunning
	inst.WorktreePath = "/tmp/worktree-remove"
	inst.WorktreeBranch = "test-branch"

	// Setup mock expectations
	mockGitExecutor.EXPECT().HasUncommittedChanges().Return(false, nil)
	mockGitExecutor.EXPECT().RemoveWorktree("/tmp/worktree-remove").Return(nil)

	// Execute Stop
	err = supervisor.Stop(context.Background(), inst, StopOptions{
		Reason: "test stop",
		Force:  false,
	})

	require.NoError(t, err)
	require.Equal(t, WorkflowStopped, inst.State)
}

func TestSupervisor_Stop_PreservesWorktreeWhenFlagDisabled(t *testing.T) {
	mockGitExecutor := mocks.NewMockGitExecutor(t)
	mockProvider := mocks.NewMockAgentProvider(t)

	// FlagRemoveWorktree is disabled (false)
	flagsRegistry := flags.New(map[string]bool{
		flags.FlagRemoveWorktree: false,
	})

	cfg := SupervisorConfig{
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: mockProvider,
		},
		ListenerFactory: &mockListenerFactory{},
		Flags:           flagsRegistry,
		SessionFactory:  session.NewFactory(session.FactoryConfig{BaseDir: t.TempDir()}),
		GitExecutorFactory: func(workDir string) appgit.GitExecutor {
			return mockGitExecutor
		},
	}

	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	// Create a workflow in Running state with a worktree
	inst := newTestInstance(t, "preserve-worktree-test")
	inst.State = WorkflowRunning
	inst.WorktreePath = "/tmp/worktree-preserve"
	inst.WorktreeBranch = "test-branch"

	// Setup mock - only HasUncommittedChanges should be called, NOT RemoveWorktree
	mockGitExecutor.EXPECT().HasUncommittedChanges().Return(false, nil)
	// RemoveWorktree should NOT be called - no mock setup for it

	// Execute Stop
	err = supervisor.Stop(context.Background(), inst, StopOptions{
		Reason: "test stop",
		Force:  false,
	})

	require.NoError(t, err)
	require.Equal(t, WorkflowStopped, inst.State)
	// Verify RemoveWorktree was never called
	mockGitExecutor.AssertNotCalled(t, "RemoveWorktree", mock.Anything)
}

func TestSupervisor_Stop_HandlesRemoveWorktreeErrorsGracefully(t *testing.T) {
	mockGitExecutor := mocks.NewMockGitExecutor(t)
	mockProvider := mocks.NewMockAgentProvider(t)

	flagsRegistry := flags.New(map[string]bool{
		flags.FlagRemoveWorktree: true,
	})

	cfg := SupervisorConfig{
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: mockProvider,
		},
		ListenerFactory: &mockListenerFactory{},
		Flags:           flagsRegistry,
		SessionFactory:  session.NewFactory(session.FactoryConfig{BaseDir: t.TempDir()}),
		GitExecutorFactory: func(workDir string) appgit.GitExecutor {
			return mockGitExecutor
		},
	}

	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	// Create a workflow in Running state with a worktree
	inst := newTestInstance(t, "worktree-error-test")
	inst.State = WorkflowRunning
	inst.WorktreePath = "/tmp/worktree-error"
	inst.WorktreeBranch = "test-branch"

	// Setup mock - RemoveWorktree returns an error
	mockGitExecutor.EXPECT().HasUncommittedChanges().Return(false, nil)
	mockGitExecutor.EXPECT().RemoveWorktree("/tmp/worktree-error").Return(errors.New("worktree removal failed"))

	// Execute Stop - should succeed despite RemoveWorktree error
	err = supervisor.Stop(context.Background(), inst, StopOptions{
		Reason: "test stop",
		Force:  false,
	})

	require.NoError(t, err) // Stop should succeed even with RemoveWorktree error
	require.Equal(t, WorkflowStopped, inst.State)
}

func TestSupervisor_Stop_WorksNormallyWhenWorktreePathEmpty(t *testing.T) {
	mockProvider := mocks.NewMockAgentProvider(t)

	factoryCalled := false
	cfg := SupervisorConfig{
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: mockProvider,
		},
		ListenerFactory: &mockListenerFactory{},
		Flags:           flags.New(map[string]bool{flags.FlagRemoveWorktree: true}),
		SessionFactory:  session.NewFactory(session.FactoryConfig{BaseDir: t.TempDir()}),
		GitExecutorFactory: func(workDir string) appgit.GitExecutor {
			factoryCalled = true
			return mocks.NewMockGitExecutor(t)
		},
	}

	supervisor, err := NewSupervisor(cfg)
	require.NoError(t, err)

	// Create a workflow in Running state WITHOUT a worktree
	inst := newTestInstance(t, "no-worktree-test")
	inst.State = WorkflowRunning
	inst.WorktreePath = "" // No worktree
	inst.WorktreeBranch = ""

	// Execute Stop
	err = supervisor.Stop(context.Background(), inst, StopOptions{
		Reason: "test stop",
		Force:  false,
	})

	require.NoError(t, err)
	require.Equal(t, WorkflowStopped, inst.State)
	// GitExecutorFactory should NOT be called when WorktreePath is empty
	require.False(t, factoryCalled, "GitExecutorFactory should not be called when WorktreePath is empty")
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

func TestSupervisor_Stop_ClosesSession(t *testing.T) {
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
	err = supervisor.Stop(ctx, inst, StopOptions{})

	require.NoError(t, err)
	require.Equal(t, WorkflowStopped, inst.State)

	// Session should be nil (closed)
	require.Nil(t, inst.Session, "Session should be set to nil after close")

	// Session directory should still exist (data preserved)
	require.DirExists(t, sessionDir, "Session directory should persist after close")

	// Verify metadata shows completed status
	meta, err := session.Load(sessionDir)
	require.NoError(t, err)
	require.Equal(t, session.StatusCompleted, meta.Status, "Session should be marked completed")
}
