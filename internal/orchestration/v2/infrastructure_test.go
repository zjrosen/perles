package v2

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/handler"
	"github.com/zjrosen/perles/internal/orchestration/v2/processor"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
	"github.com/zjrosen/perles/internal/orchestration/workflow"
)

// createTestTaskExecutor creates a MockTaskExecutor for infrastructure tests.
func createTestTaskExecutor(t *testing.T) *mocks.MockTaskExecutor {
	return mocks.NewMockTaskExecutor(t)
}

// createTestAgentProvider creates an AgentProvider mock for testing.
func createTestAgentProvider(t *testing.T) client.AgentProvider {
	mockClient := mocks.NewMockHeadlessClient(t)
	mockClient.EXPECT().Type().Return(client.ClientClaude).Maybe()

	mockProvider := mocks.NewMockAgentProvider(t)
	mockProvider.EXPECT().Client().Return(mockClient, nil).Maybe()
	mockProvider.EXPECT().Extensions().Return(map[string]any{}).Maybe()
	mockProvider.EXPECT().Type().Return(client.ClientClaude).Maybe()
	return mockProvider
}

// ===========================================================================
// Config Validation Tests
// ===========================================================================

func TestInfrastructureConfig_Validate(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		cfg := InfrastructureConfig{
			Port: 8080,
			AgentProviders: client.AgentProviders{
				client.RoleCoordinator: createTestAgentProvider(t),
			},
			WorkDir: "/tmp/test",
		}
		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing port returns error", func(t *testing.T) {
		cfg := InfrastructureConfig{
			Port: 0, // Invalid: zero port
			AgentProviders: client.AgentProviders{
				client.RoleCoordinator: createTestAgentProvider(t),
			},
			WorkDir: "/tmp/test",
		}
		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "port is required")
	})

	t.Run("nil AgentProviders returns error", func(t *testing.T) {
		cfg := InfrastructureConfig{
			Port:           8080,
			AgentProviders: nil, // Invalid: nil providers
			WorkDir:        "/tmp/test",
		}
		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "AgentProviders is required")
	})

	t.Run("empty WorkDir returns error", func(t *testing.T) {
		cfg := InfrastructureConfig{
			Port: 8080,
			AgentProviders: client.AgentProviders{
				client.RoleCoordinator: createTestAgentProvider(t),
			},
			WorkDir: "", // Invalid: empty work dir
		}
		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "work directory is required")
	})
}

// ===========================================================================
// NewInfrastructure Tests
// ===========================================================================

func TestNewInfrastructure(t *testing.T) {
	t.Run("creates infrastructure with valid config", func(t *testing.T) {
		cfg := InfrastructureConfig{
			Port: 8080,
			AgentProviders: client.AgentProviders{
				client.RoleCoordinator: createTestAgentProvider(t),
			},
			WorkDir:      "/tmp/test",
			TaskExecutor: createTestTaskExecutor(t),
		}

		infra, err := NewInfrastructure(cfg)
		require.NoError(t, err)
		require.NotNil(t, infra)

		// Verify Core components are created
		assert.NotNil(t, infra.Core.Processor)
		assert.NotNil(t, infra.Core.Adapter)
		assert.NotNil(t, infra.Core.EventBus)
		assert.NotNil(t, infra.Core.CmdSubmitter)

		// Verify Repositories are created
		assert.NotNil(t, infra.Repositories.ProcessRepo)
		assert.NotNil(t, infra.Repositories.TaskRepo)
		assert.NotNil(t, infra.Repositories.QueueRepo)

		// Verify Internal components are created
		assert.NotNil(t, infra.Internal.ProcessRegistry)
	})

	t.Run("returns error for invalid config", func(t *testing.T) {
		cfg := InfrastructureConfig{} // All fields empty - invalid

		infra, err := NewInfrastructure(cfg)
		assert.Error(t, err)
		assert.Nil(t, infra)
		assert.Contains(t, err.Error(), "invalid infrastructure config")
	})

	t.Run("returns error for nil AgentProviders", func(t *testing.T) {
		cfg := InfrastructureConfig{
			Port:           8080,
			AgentProviders: nil,
			WorkDir:        "/tmp/test",
		}

		infra, err := NewInfrastructure(cfg)
		assert.Error(t, err)
		assert.Nil(t, infra)
		assert.Contains(t, err.Error(), "AgentProviders is required")
	})

	t.Run("returns error for zero Port", func(t *testing.T) {
		cfg := InfrastructureConfig{
			Port: 0,
			AgentProviders: client.AgentProviders{
				client.RoleCoordinator: createTestAgentProvider(t),
			},
			WorkDir: "/tmp/test",
		}

		infra, err := NewInfrastructure(cfg)
		assert.Error(t, err)
		assert.Nil(t, infra)
		assert.Contains(t, err.Error(), "port is required")
	})
}

// ===========================================================================
// Lifecycle Tests
// ===========================================================================

func TestInfrastructure_Start(t *testing.T) {
	t.Run("starts processor and waits for ready", func(t *testing.T) {
		cfg := InfrastructureConfig{
			Port: 8080,
			AgentProviders: client.AgentProviders{
				client.RoleCoordinator: createTestAgentProvider(t),
			},
			WorkDir:      "/tmp/test",
			TaskExecutor: createTestTaskExecutor(t),
		}

		infra, err := NewInfrastructure(cfg)
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Start should succeed and processor should be running
		err = infra.Start(ctx)
		require.NoError(t, err)

		// Processor should be running after Start returns
		assert.True(t, infra.Core.Processor.IsRunning())

		// Clean up
		infra.Shutdown()
	})

	t.Run("returns error when context is cancelled during start", func(t *testing.T) {
		cfg := InfrastructureConfig{
			Port: 8080,
			AgentProviders: client.AgentProviders{
				client.RoleCoordinator: createTestAgentProvider(t),
			},
			WorkDir:      "/tmp/test",
			TaskExecutor: createTestTaskExecutor(t),
		}

		infra, err := NewInfrastructure(cfg)
		require.NoError(t, err)

		// Create an already-cancelled context
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		// Start should fail because context is already cancelled
		err = infra.Start(ctx)
		assert.Error(t, err)
	})
}

func TestInfrastructure_Drain(t *testing.T) {
	t.Run("gracefully shuts down processor", func(t *testing.T) {
		cfg := InfrastructureConfig{
			Port: 8080,
			AgentProviders: client.AgentProviders{
				client.RoleCoordinator: createTestAgentProvider(t),
			},
			WorkDir:      "/tmp/test",
			TaskExecutor: createTestTaskExecutor(t),
		}

		infra, err := NewInfrastructure(cfg)
		require.NoError(t, err)

		ctx := context.Background()
		err = infra.Start(ctx)
		require.NoError(t, err)

		// Processor should be running
		assert.True(t, infra.Core.Processor.IsRunning())

		// Drain should stop the processor
		infra.Drain()

		// Processor should no longer be running after Drain
		assert.False(t, infra.Core.Processor.IsRunning())
	})

	t.Run("handles drain on unstarted infrastructure", func(t *testing.T) {
		cfg := InfrastructureConfig{
			Port: 8080,
			AgentProviders: client.AgentProviders{
				client.RoleCoordinator: createTestAgentProvider(t),
			},
			WorkDir:      "/tmp/test",
			TaskExecutor: createTestTaskExecutor(t),
		}

		infra, err := NewInfrastructure(cfg)
		require.NoError(t, err)

		// Drain should not panic even if Start was never called
		assert.NotPanics(t, func() {
			infra.Drain()
		})
	})
}

func TestInfrastructure_Shutdown(t *testing.T) {
	t.Run("stops all components cleanly", func(t *testing.T) {
		cfg := InfrastructureConfig{
			Port: 8080,
			AgentProviders: client.AgentProviders{
				client.RoleCoordinator: createTestAgentProvider(t),
			},
			WorkDir:      "/tmp/test",
			TaskExecutor: createTestTaskExecutor(t),
		}

		infra, err := NewInfrastructure(cfg)
		require.NoError(t, err)

		ctx := context.Background()
		err = infra.Start(ctx)
		require.NoError(t, err)

		// All components should be running
		assert.True(t, infra.Core.Processor.IsRunning())

		// Shutdown should stop everything cleanly
		assert.NotPanics(t, func() {
			infra.Shutdown()
		})

		// Processor should no longer be running
		assert.False(t, infra.Core.Processor.IsRunning())
	})

	t.Run("handles shutdown on unstarted infrastructure", func(t *testing.T) {
		cfg := InfrastructureConfig{
			Port: 8080,
			AgentProviders: client.AgentProviders{
				client.RoleCoordinator: createTestAgentProvider(t),
			},
			WorkDir:      "/tmp/test",
			TaskExecutor: createTestTaskExecutor(t),
		}

		infra, err := NewInfrastructure(cfg)
		require.NoError(t, err)

		// Shutdown should not panic even if Start was never called
		assert.NotPanics(t, func() {
			infra.Shutdown()
		})
	})

	t.Run("can be called multiple times safely", func(t *testing.T) {
		cfg := InfrastructureConfig{
			Port: 8080,
			AgentProviders: client.AgentProviders{
				client.RoleCoordinator: createTestAgentProvider(t),
			},
			WorkDir:      "/tmp/test",
			TaskExecutor: createTestTaskExecutor(t),
		}

		infra, err := NewInfrastructure(cfg)
		require.NoError(t, err)

		ctx := context.Background()
		err = infra.Start(ctx)
		require.NoError(t, err)

		// Calling Shutdown multiple times should not panic
		assert.NotPanics(t, func() {
			infra.Shutdown()
			infra.Shutdown()
		})
	})
}

// ===========================================================================
// Handler Registration Tests
// ===========================================================================

func TestAllHandlersRegistered(t *testing.T) {
	cfg := InfrastructureConfig{
		Port: 8080,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: createTestAgentProvider(t),
		},
		WorkDir:      "/tmp/test",
		TaskExecutor: createTestTaskExecutor(t),
	}

	infra, err := NewInfrastructure(cfg)
	require.NoError(t, err)

	// Start the infrastructure so we can verify handlers are properly registered
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = infra.Start(ctx)
	require.NoError(t, err)
	defer infra.Drain()

	// The processor should be running and ready to process commands
	assert.True(t, infra.Core.Processor.IsRunning())

	// Verify all repositories are properly wired
	assert.NotNil(t, infra.Repositories.ProcessRepo)
	assert.NotNil(t, infra.Repositories.TaskRepo)
	assert.NotNil(t, infra.Repositories.QueueRepo)

	// Verify process registry is created
	assert.NotNil(t, infra.Internal.ProcessRegistry)
}

func TestInfrastructure_BroadcastHandlerRegistered(t *testing.T) {
	cfg := InfrastructureConfig{
		Port: 8080,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: createTestAgentProvider(t),
		},
		WorkDir:      "/tmp/test",
		TaskExecutor: createTestTaskExecutor(t),
	}

	infra, err := NewInfrastructure(cfg)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, infra.Start(ctx))
	defer infra.Drain()

	// Two Working workers (messages are queued, not delivered) and one retired worker.
	now := time.Now()
	for _, p := range []*repository.Process{
		{ID: "worker-1", Role: repository.RoleWorker, Status: repository.StatusWorking, CreatedAt: now},
		{ID: "worker-2", Role: repository.RoleWorker, Status: repository.StatusWorking, CreatedAt: now},
		{ID: "worker-3", Role: repository.RoleWorker, Status: repository.StatusRetired, CreatedAt: now},
	} {
		require.NoError(t, infra.Repositories.ProcessRepo.Save(p))
	}

	cmd := command.NewBroadcastCommand(command.SourceUser, "hello everyone", []string{"worker-2"})
	result, err := infra.Core.Processor.SubmitAndWait(ctx, cmd)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotErrorIs(t, result.Error, processor.ErrUnknownCommandType)
	require.True(t, result.Success, "broadcast should succeed: %v", result.Error)

	broadcastResult, ok := result.Data.(*handler.BroadcastResult)
	require.True(t, ok, "expected *handler.BroadcastResult, got %T", result.Data)
	require.Equal(t, []string{"worker-1"}, broadcastResult.TargetWorkers)
	require.Equal(t, 1, broadcastResult.MessagesSent)

	// Barrier: follow-ups were enqueued before the result was returned, so a second
	// command submitted now is processed after them (FIFO). Excluding every worker
	// makes it a no-op. Once it returns, repositories are safe to read.
	barrier := command.NewBroadcastCommand(command.SourceUser, "barrier", []string{"worker-1", "worker-2", "worker-3"})
	barrierResult, err := infra.Core.Processor.SubmitAndWait(ctx, barrier)
	require.NoError(t, err)
	require.True(t, barrierResult.Success)

	// The SendToProcess follow-up queued the message for the non-excluded active worker only.
	require.Equal(t, 1, infra.Repositories.QueueRepo.Size("worker-1"))
	require.Equal(t, 0, infra.Repositories.QueueRepo.Size("worker-2"))
	require.Equal(t, 0, infra.Repositories.QueueRepo.Size("worker-3"))
}

// ===========================================================================
// Integration Tests
// ===========================================================================

func TestInfrastructure_Integration(t *testing.T) {
	t.Run("full lifecycle: create, start, drain", func(t *testing.T) {
		mockClient := mocks.NewMockHeadlessClient(t)
		mockClient.EXPECT().Type().Return(client.ClientClaude).Maybe()
		// Allow Spawn to be called if needed during tests
		mockClient.On("Spawn", mock.Anything, mock.Anything).
			Return(nil, nil).
			Maybe()

		// Create provider mock with extensions
		mockProvider := mocks.NewMockAgentProvider(t)
		mockProvider.EXPECT().Client().Return(mockClient, nil).Maybe()
		mockProvider.EXPECT().Extensions().Return(map[string]any{"model": "claude-3"}).Maybe()
		mockProvider.EXPECT().Type().Return(client.ClientClaude).Maybe()
		provider := mockProvider

		cfg := InfrastructureConfig{
			Port: 8080,
			AgentProviders: client.AgentProviders{
				client.RoleCoordinator: provider,
			},
			WorkDir:      "/tmp/test",
			TaskExecutor: createTestTaskExecutor(t),
		}

		// Create
		infra, err := NewInfrastructure(cfg)
		require.NoError(t, err)
		require.NotNil(t, infra)

		// Start
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = infra.Start(ctx)
		require.NoError(t, err)
		assert.True(t, infra.Core.Processor.IsRunning())

		// Drain
		infra.Drain()
		assert.False(t, infra.Core.Processor.IsRunning())
	})

	t.Run("creates infrastructure with WorkflowStateProvider", func(t *testing.T) {
		mockClient := mocks.NewMockHeadlessClient(t)
		mockClient.EXPECT().Type().Return(client.ClientClaude).Maybe()

		mockProvider := mocks.NewMockAgentProvider(t)
		mockProvider.EXPECT().Client().Return(mockClient, nil).Maybe()
		mockProvider.EXPECT().Extensions().Return(map[string]any{}).Maybe()
		mockProvider.EXPECT().Type().Return(client.ClientClaude).Maybe()

		// Create a mock workflow state provider
		workflowProvider := &mockWorkflowStateProvider{}

		cfg := InfrastructureConfig{
			Port: 8080,
			AgentProviders: client.AgentProviders{
				client.RoleCoordinator: mockProvider,
			},
			WorkDir:               "/tmp/test",
			TaskExecutor:          createTestTaskExecutor(t),
			WorkflowStateProvider: workflowProvider,
		}

		// Create infrastructure with WorkflowStateProvider
		infra, err := NewInfrastructure(cfg)
		require.NoError(t, err)
		require.NotNil(t, infra)

		// Start the infrastructure
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = infra.Start(ctx)
		require.NoError(t, err)
		assert.True(t, infra.Core.Processor.IsRunning())

		// Clean up
		infra.Shutdown()
	})
}

// mockWorkflowStateProvider implements handler.WorkflowStateProvider for testing.
type mockWorkflowStateProvider struct{}

func (m *mockWorkflowStateProvider) GetActiveWorkflowState() (*workflow.WorkflowState, error) {
	return nil, nil
}
