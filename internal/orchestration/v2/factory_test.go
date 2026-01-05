package v2

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
)

// ===========================================================================
// Config Validation Tests
// ===========================================================================

func TestInfrastructureConfig_Validate(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		mockClient := mocks.NewMockHeadlessClient(t)
		mockClient.EXPECT().Type().Return("claude").Maybe()

		cfg := InfrastructureConfig{
			Port:        8080,
			AIClient:    mockClient,
			WorkDir:     "/tmp/test",
			MessageRepo: repository.NewMemoryMessageRepository(),
		}
		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing port returns error", func(t *testing.T) {
		mockClient := mocks.NewMockHeadlessClient(t)
		cfg := InfrastructureConfig{
			Port:        0, // Invalid: zero port
			AIClient:    mockClient,
			WorkDir:     "/tmp/test",
			MessageRepo: repository.NewMemoryMessageRepository(),
		}
		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "port is required")
	})

	t.Run("nil AIClient returns error", func(t *testing.T) {
		cfg := InfrastructureConfig{
			Port:        8080,
			AIClient:    nil, // Invalid: nil client
			WorkDir:     "/tmp/test",
			MessageRepo: repository.NewMemoryMessageRepository(),
		}
		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "AI client is required")
	})

	t.Run("nil MessageRepo returns error", func(t *testing.T) {
		mockClient := mocks.NewMockHeadlessClient(t)
		cfg := InfrastructureConfig{
			Port:        8080,
			AIClient:    mockClient,
			WorkDir:     "/tmp/test",
			MessageRepo: nil, // Invalid: nil message repo
		}
		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "message repository is required")
	})

	t.Run("empty WorkDir returns error", func(t *testing.T) {
		mockClient := mocks.NewMockHeadlessClient(t)
		cfg := InfrastructureConfig{
			Port:        8080,
			AIClient:    mockClient,
			WorkDir:     "", // Invalid: empty work dir
			MessageRepo: repository.NewMemoryMessageRepository(),
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
		mockClient := mocks.NewMockHeadlessClient(t)
		mockClient.EXPECT().Type().Return("claude").Maybe()

		cfg := InfrastructureConfig{
			Port:        8080,
			AIClient:    mockClient,
			WorkDir:     "/tmp/test",
			MessageRepo: repository.NewMemoryMessageRepository(),
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

	t.Run("returns error for nil AIClient", func(t *testing.T) {
		cfg := InfrastructureConfig{
			Port:        8080,
			AIClient:    nil,
			WorkDir:     "/tmp/test",
			MessageRepo: repository.NewMemoryMessageRepository(),
		}

		infra, err := NewInfrastructure(cfg)
		assert.Error(t, err)
		assert.Nil(t, infra)
		assert.Contains(t, err.Error(), "AI client is required")
	})

	t.Run("returns error for nil MessageRepo", func(t *testing.T) {
		mockClient := mocks.NewMockHeadlessClient(t)
		cfg := InfrastructureConfig{
			Port:        8080,
			AIClient:    mockClient,
			WorkDir:     "/tmp/test",
			MessageRepo: nil,
		}

		infra, err := NewInfrastructure(cfg)
		assert.Error(t, err)
		assert.Nil(t, infra)
		assert.Contains(t, err.Error(), "message repository is required")
	})

	t.Run("returns error for zero Port", func(t *testing.T) {
		mockClient := mocks.NewMockHeadlessClient(t)
		cfg := InfrastructureConfig{
			Port:        0,
			AIClient:    mockClient,
			WorkDir:     "/tmp/test",
			MessageRepo: repository.NewMemoryMessageRepository(),
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
		mockClient := mocks.NewMockHeadlessClient(t)
		mockClient.EXPECT().Type().Return("claude").Maybe()

		cfg := InfrastructureConfig{
			Port:        8080,
			AIClient:    mockClient,
			WorkDir:     "/tmp/test",
			MessageRepo: repository.NewMemoryMessageRepository(),
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
		infra.Drain()
	})

	t.Run("returns error when context is cancelled during start", func(t *testing.T) {
		mockClient := mocks.NewMockHeadlessClient(t)
		mockClient.EXPECT().Type().Return("claude").Maybe()

		cfg := InfrastructureConfig{
			Port:        8080,
			AIClient:    mockClient,
			WorkDir:     "/tmp/test",
			MessageRepo: repository.NewMemoryMessageRepository(),
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
		mockClient := mocks.NewMockHeadlessClient(t)
		mockClient.EXPECT().Type().Return("claude").Maybe()

		cfg := InfrastructureConfig{
			Port:        8080,
			AIClient:    mockClient,
			WorkDir:     "/tmp/test",
			MessageRepo: repository.NewMemoryMessageRepository(),
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
		mockClient := mocks.NewMockHeadlessClient(t)
		mockClient.EXPECT().Type().Return("claude").Maybe()

		cfg := InfrastructureConfig{
			Port:        8080,
			AIClient:    mockClient,
			WorkDir:     "/tmp/test",
			MessageRepo: repository.NewMemoryMessageRepository(),
		}

		infra, err := NewInfrastructure(cfg)
		require.NoError(t, err)

		// Drain should not panic even if Start was never called
		assert.NotPanics(t, func() {
			infra.Drain()
		})
	})
}

// ===========================================================================
// Handler Registration Tests
// ===========================================================================

func TestAllHandlersRegistered(t *testing.T) {
	mockClient := mocks.NewMockHeadlessClient(t)
	mockClient.EXPECT().Type().Return("claude").Maybe()

	cfg := InfrastructureConfig{
		Port:        8080,
		AIClient:    mockClient,
		WorkDir:     "/tmp/test",
		MessageRepo: repository.NewMemoryMessageRepository(),
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

// ===========================================================================
// Integration Tests
// ===========================================================================

func TestInfrastructure_Integration(t *testing.T) {
	t.Run("full lifecycle: create, start, drain", func(t *testing.T) {
		mockClient := mocks.NewMockHeadlessClient(t)
		mockClient.EXPECT().Type().Return("claude").Maybe()
		// Allow Spawn to be called if needed during tests
		mockClient.On("Spawn", mock.Anything, mock.Anything).
			Return(nil, nil).
			Maybe()

		cfg := InfrastructureConfig{
			Port:        8080,
			AIClient:    mockClient,
			WorkDir:     "/tmp/test",
			MessageRepo: repository.NewMemoryMessageRepository(),
			Extensions:  map[string]any{"model": "claude-3"},
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
}
