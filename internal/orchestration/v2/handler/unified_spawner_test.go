package handler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/mock"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/prompt/roles"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
	"github.com/zjrosen/perles/internal/pubsub"
)

// mockCommandSubmitter implements process.CommandSubmitter for testing.
type mockCommandSubmitter struct {
	commands []command.Command
}

func (m *mockCommandSubmitter) Submit(cmd command.Command) {
	m.commands = append(m.commands, cmd)
}

func TestUnifiedProcessSpawner_SpawnProcess_Worker(t *testing.T) {
	mockClient := mock.NewClient()
	eventBus := pubsub.NewBroker[any]()
	submitter := &mockCommandSubmitter{}

	spawner := NewUnifiedProcessSpawner(UnifiedSpawnerConfig{
		Client:     mockClient,
		WorkDir:    "/test/workdir",
		Port:       8080,
		Extensions: nil,
		Submitter:  submitter,
		EventBus:   eventBus,
	})

	proc, err := spawner.SpawnProcess(context.Background(), "worker-1", repository.RoleWorker, SpawnOptions{})
	require.NoError(t, err)
	require.NotNil(t, proc)
	assert.Equal(t, "worker-1", proc.ID)
	assert.Equal(t, repository.RoleWorker, proc.Role)

	// Cleanup
	proc.Stop()
}

func TestUnifiedProcessSpawner_SpawnProcess_Coordinator(t *testing.T) {
	mockClient := mock.NewClient()
	eventBus := pubsub.NewBroker[any]()
	submitter := &mockCommandSubmitter{}

	spawner := NewUnifiedProcessSpawner(UnifiedSpawnerConfig{
		Client:     mockClient,
		WorkDir:    "/test/workdir",
		Port:       8080,
		Extensions: nil,
		Submitter:  submitter,
		EventBus:   eventBus,
	})

	proc, err := spawner.SpawnProcess(context.Background(), repository.CoordinatorID, repository.RoleCoordinator, SpawnOptions{})
	require.NoError(t, err)
	require.NotNil(t, proc)
	assert.Equal(t, repository.CoordinatorID, proc.ID)
	assert.Equal(t, repository.RoleCoordinator, proc.Role)

	// Cleanup
	proc.Stop()
}

func TestUnifiedProcessSpawner_SpawnProcess_NilClient(t *testing.T) {
	eventBus := pubsub.NewBroker[any]()
	submitter := &mockCommandSubmitter{}

	spawner := NewUnifiedProcessSpawner(UnifiedSpawnerConfig{
		Client:     nil,
		WorkDir:    "/test/workdir",
		Port:       8080,
		Extensions: nil,
		Submitter:  submitter,
		EventBus:   eventBus,
	})

	proc, err := spawner.SpawnProcess(context.Background(), "worker-1", repository.RoleWorker, SpawnOptions{})
	require.Error(t, err)
	require.Nil(t, proc)
	assert.Contains(t, err.Error(), "client is nil")
}

func TestSpawnOptions_AgentType(t *testing.T) {
	opts := SpawnOptions{
		AgentType: roles.AgentTypeImplementer,
	}
	assert.Equal(t, roles.AgentTypeImplementer, opts.AgentType)
}

func TestSpawnOptions_DefaultAgentType(t *testing.T) {
	opts := SpawnOptions{}
	// Default (zero value) should be AgentTypeGeneric (empty string)
	assert.Equal(t, roles.AgentTypeGeneric, opts.AgentType)
}

func TestUnifiedProcessSpawner_SpawnProcess_WithAgentType(t *testing.T) {
	testCases := []struct {
		name      string
		agentType roles.AgentType
	}{
		{"generic", roles.AgentTypeGeneric},
		{"implementer", roles.AgentTypeImplementer},
		{"reviewer", roles.AgentTypeReviewer},
		{"researcher", roles.AgentTypeResearcher},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockClient := mock.NewClient()
			eventBus := pubsub.NewBroker[any]()
			submitter := &mockCommandSubmitter{}

			spawner := NewUnifiedProcessSpawner(UnifiedSpawnerConfig{
				Client:     mockClient,
				WorkDir:    "/test/workdir",
				Port:       8080,
				Extensions: nil,
				Submitter:  submitter,
				EventBus:   eventBus,
			})

			opts := SpawnOptions{AgentType: tc.agentType}
			proc, err := spawner.SpawnProcess(context.Background(), "worker-1", repository.RoleWorker, opts)
			require.NoError(t, err)
			require.NotNil(t, proc)
			assert.Equal(t, "worker-1", proc.ID)
			assert.Equal(t, repository.RoleWorker, proc.Role)

			// Cleanup
			proc.Stop()
		})
	}
}

func TestUnifiedProcessSpawner_GenerateMCPConfig_HTTP(t *testing.T) {
	mockClient := mock.NewClient()
	spawner := &UnifiedProcessSpawnerImpl{
		client:  mockClient,
		port:    9999,
		workDir: "/test",
	}

	config, err := spawner.generateMCPConfig("worker-1")
	require.NoError(t, err)
	assert.Contains(t, config, "9999")
	assert.Contains(t, config, "worker-1")
}

// openCodeMockClient is a mock client that returns ClientOpenCode type.
type openCodeMockClient struct {
	*mock.Client
}

func (c *openCodeMockClient) Type() client.ClientType {
	return client.ClientOpenCode
}

func TestUnifiedProcessSpawner_GenerateMCPConfig_OpenCode(t *testing.T) {
	mockClient := &openCodeMockClient{Client: mock.NewClient()}
	spawner := &UnifiedProcessSpawnerImpl{
		client:  mockClient,
		port:    9999,
		workDir: "/test",
	}

	config, err := spawner.generateMCPConfig("worker-1")
	require.NoError(t, err)
	// OpenCode format uses {"mcp": {...}} wrapper, not {"mcpServers": {...}}
	assert.Contains(t, config, `"mcp"`)
	assert.Contains(t, config, `"perles-worker"`)
	assert.Contains(t, config, `"type":"remote"`)
	assert.Contains(t, config, "9999")
	assert.Contains(t, config, "worker-1")
	// Should NOT contain mcpServers (that's Claude format)
	assert.NotContains(t, config, "mcpServers")
}

func TestUnifiedProcessSpawner_GenerateCoordinatorMCPConfig_OpenCode(t *testing.T) {
	mockClient := &openCodeMockClient{Client: mock.NewClient()}
	spawner := &UnifiedProcessSpawnerImpl{
		client:  mockClient,
		port:    9999,
		workDir: "/test",
	}

	config, err := spawner.generateCoordinatorMCPConfig()
	require.NoError(t, err)
	// OpenCode format uses {"mcp": {...}} wrapper
	assert.Contains(t, config, `"mcp"`)
	assert.Contains(t, config, `"perles-orchestrator"`)
	assert.Contains(t, config, `"type":"remote"`)
	assert.Contains(t, config, "9999")
	// Should NOT contain mcpServers (that's Claude format)
	assert.NotContains(t, config, "mcpServers")
}
