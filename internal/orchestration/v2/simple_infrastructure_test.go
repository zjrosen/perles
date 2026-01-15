package v2

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/orchestration/client"
)

// createTestSimpleAgentProvider creates an AgentProvider mock for testing.
func createTestSimpleAgentProvider(t *testing.T) client.AgentProvider {
	mockClient := mocks.NewMockHeadlessClient(t)
	mockClient.EXPECT().Type().Return(client.ClientClaude).Maybe()

	mockProvider := mocks.NewMockAgentProvider(t)
	mockProvider.EXPECT().Client().Return(mockClient, nil).Maybe()
	mockProvider.EXPECT().Extensions().Return(map[string]any{}).Maybe()
	mockProvider.EXPECT().Type().Return(client.ClientClaude).Maybe()
	return mockProvider
}

// ===========================================================================
// SimpleInfrastructure Tests
// ===========================================================================

func TestSimpleInfrastructureConfig_Validate(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		cfg := SimpleInfrastructureConfig{
			AgentProvider: createTestSimpleAgentProvider(t),
			WorkDir:       "/tmp/test",
			SystemPrompt:  "You are a helpful assistant.",
		}
		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Run("nil AgentProvider returns error", func(t *testing.T) {
		cfg := SimpleInfrastructureConfig{
			AgentProvider: nil,
			WorkDir:       "/tmp/test",
		}
		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "AgentProvider is required")
	})

	t.Run("empty WorkDir returns error", func(t *testing.T) {
		cfg := SimpleInfrastructureConfig{
			AgentProvider: createTestSimpleAgentProvider(t),
			WorkDir:       "",
		}
		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "work directory is required")
	})
}

func TestNewSimpleInfrastructure(t *testing.T) {
	t.Run("creates infrastructure with valid config", func(t *testing.T) {
		cfg := SimpleInfrastructureConfig{
			AgentProvider: createTestSimpleAgentProvider(t),
			WorkDir:       "/tmp/test",
			SystemPrompt:  "You are a helpful assistant.",
		}

		infra, err := NewSimpleInfrastructure(cfg)
		require.NoError(t, err)
		require.NotNil(t, infra)

		assert.NotNil(t, infra.Processor)
		assert.NotNil(t, infra.EventBus)
		assert.NotNil(t, infra.ProcessRepo)
		assert.NotNil(t, infra.QueueRepo)
		assert.NotNil(t, infra.ProcessRegistry)
		assert.NotNil(t, infra.CmdSubmitter)
	})

	t.Run("returns error for nil AgentProvider", func(t *testing.T) {
		cfg := SimpleInfrastructureConfig{
			AgentProvider: nil,
			WorkDir:       "/tmp/test",
		}

		infra, err := NewSimpleInfrastructure(cfg)
		assert.Error(t, err)
		assert.Nil(t, infra)
		assert.Contains(t, err.Error(), "AgentProvider is required")
	})

	t.Run("returns error for empty WorkDir", func(t *testing.T) {
		cfg := SimpleInfrastructureConfig{
			AgentProvider: createTestSimpleAgentProvider(t),
			WorkDir:       "",
		}

		infra, err := NewSimpleInfrastructure(cfg)
		assert.Error(t, err)
		assert.Nil(t, infra)
		assert.Contains(t, err.Error(), "work directory is required")
	})
}

func TestSimpleInfrastructure_Lifecycle(t *testing.T) {
	t.Run("start and shutdown", func(t *testing.T) {
		cfg := SimpleInfrastructureConfig{
			AgentProvider: createTestSimpleAgentProvider(t),
			WorkDir:       "/tmp/test",
			SystemPrompt:  "You are a helpful assistant.",
		}

		infra, err := NewSimpleInfrastructure(cfg)
		require.NoError(t, err)

		err = infra.Start()
		require.NoError(t, err)

		assert.True(t, infra.Processor.IsRunning())

		infra.Shutdown()
		assert.False(t, infra.Processor.IsRunning())
	})

	t.Run("shutdown handles unstarted infrastructure", func(t *testing.T) {
		cfg := SimpleInfrastructureConfig{
			AgentProvider: createTestSimpleAgentProvider(t),
			WorkDir:       "/tmp/test",
		}

		infra, err := NewSimpleInfrastructure(cfg)
		require.NoError(t, err)

		assert.NotPanics(t, func() {
			infra.Shutdown()
		})
	})
}

func TestSimpleInfrastructure_RegistersOnlyCoreHandlers(t *testing.T) {
	cfg := SimpleInfrastructureConfig{
		AgentProvider: createTestSimpleAgentProvider(t),
		WorkDir:       "/tmp/test",
		SystemPrompt:  "You are a helpful assistant.",
	}

	infra, err := NewSimpleInfrastructure(cfg)
	require.NoError(t, err)

	err = infra.Start()
	require.NoError(t, err)
	defer infra.Shutdown()

	assert.True(t, infra.Processor.IsRunning())
	assert.NotNil(t, infra.ProcessRepo)
	assert.NotNil(t, infra.QueueRepo)
	assert.NotNil(t, infra.ProcessRegistry)
}
