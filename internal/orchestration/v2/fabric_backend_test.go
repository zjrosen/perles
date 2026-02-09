package v2

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fabricsqlite "github.com/zjrosen/perles/internal/infrastructure/sqlite"
	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/orchestration/client"
	domain "github.com/zjrosen/perles/internal/orchestration/fabric/domain"
	fabricrepo "github.com/zjrosen/perles/internal/orchestration/fabric/repository"
)

// setupTestDB creates a SQLite DB with Fabric migrations applied for testing.
func setupTestDB(t *testing.T) *fabricsqlite.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := fabricsqlite.NewDB(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

// createFabricTestAgentProvider creates a minimal AgentProvider mock for fabric backend tests.
func createFabricTestAgentProvider(t *testing.T) client.AgentProvider {
	t.Helper()
	mockClient := mocks.NewMockHeadlessClient(t)
	mockClient.EXPECT().Type().Return(client.ClientClaude).Maybe()

	mockProvider := mocks.NewMockAgentProvider(t)
	mockProvider.EXPECT().Client().Return(mockClient, nil).Maybe()
	mockProvider.EXPECT().Extensions().Return(map[string]any{}).Maybe()
	mockProvider.EXPECT().Type().Return(client.ClientClaude).Maybe()
	return mockProvider
}

// ===========================================================================
// createFabricRepositories Tests
// ===========================================================================

func TestCreateFabricRepositories_NilConfig_ReturnsMemoryRepos(t *testing.T) {
	threads, deps := createFabricRepositories(nil)

	require.NotNil(t, threads)
	require.NotNil(t, deps)

	// Memory repos: verify we can create and retrieve a thread (no DB needed)
	created, err := threads.Create(domain.Thread{
		Type:      domain.ThreadChannel,
		CreatedBy: "test",
		Slug:      "general",
	})
	require.NoError(t, err)

	got, err := threads.Get(created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)
}

func TestCreateFabricRepositories_NilDB_ReturnsMemoryRepos(t *testing.T) {
	cfg := &FabricBackendConfig{
		DB:        nil, // nil DB = memory only
		SessionID: "session-1",
	}

	threads, deps := createFabricRepositories(cfg)

	require.NotNil(t, threads)
	require.NotNil(t, deps)

	// Should work without a database
	created, err := threads.Create(domain.Thread{
		Type:      domain.ThreadMessage,
		CreatedBy: "test",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, created.ID)

	err = deps.Add(domain.Dependency{
		ThreadID:    "a",
		DependsOnID: "b",
		Relation:    domain.RelationChildOf,
	})
	// Memory dep repo doesn't enforce FK, so this should succeed
	require.NoError(t, err)
}

func TestCreateFabricRepositories_SQLiteBackend_WithSQLiteReadEnabled(t *testing.T) {
	db := setupTestDB(t)

	cfg := &FabricBackendConfig{
		DB:                db.Connection(),
		SessionID:         "session-1",
		SQLiteReadEnabled: true,
	}

	threads, deps := createFabricRepositories(cfg)
	require.NotNil(t, threads)
	require.NotNil(t, deps)

	// Create via the returned repo (should use SQLite)
	created, err := threads.Create(domain.Thread{
		Type:      domain.ThreadChannel,
		CreatedBy: "test",
		Slug:      "tasks",
	})
	require.NoError(t, err)
	require.NotEmpty(t, created.ID)

	// Verify read works through SQLite
	got, err := threads.GetBySlug("tasks")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, created.ID, got.ID)
}

func TestCreateFabricRepositories_SQLiteReadDisabled_UsesMemoryForReads(t *testing.T) {
	db := setupTestDB(t)

	cfg := &FabricBackendConfig{
		DB:                db.Connection(),
		SessionID:         "session-1",
		SQLiteReadEnabled: false, // Memory reads
		DualWriteEnabled:  false,
	}

	threads, deps := createFabricRepositories(cfg)
	require.NotNil(t, threads)
	require.NotNil(t, deps)

	// With SQLiteReadEnabled=false and no dual-write, we get memory repos
	created, err := threads.Create(domain.Thread{
		Type:      domain.ThreadMessage,
		CreatedBy: "test",
		Content:   "memory only",
	})
	require.NoError(t, err)
	require.NotEmpty(t, created.ID)

	// Memory repo should find it
	got, err := threads.Get(created.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "memory only", got.Content)
}

func TestCreateFabricRepositories_SubscriptionsAcksParticipants_AlwaysMemory(t *testing.T) {
	// This test verifies the architectural constraint that subs/acks/participants
	// are always in-memory regardless of FabricBackendConfig.
	// We test this indirectly by creating full infrastructure and verifying
	// that these repos work without a DB.
	db := setupTestDB(t)

	cfg := InfrastructureConfig{
		Port: 8080,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: createFabricTestAgentProvider(t),
		},
		WorkDir: "/tmp/test",
		FabricBackend: &FabricBackendConfig{
			DB:                db.Connection(),
			SessionID:         "session-1",
			SQLiteReadEnabled: true,
		},
	}

	infra, err := NewInfrastructure(cfg)
	require.NoError(t, err)

	// FabricService should be created and functional
	require.NotNil(t, infra.Core.FabricService)
}

func TestCreateFabricRepositories_AckRepo_WiredWithChosenRepos(t *testing.T) {
	// Verify AckRepository is wired with the chosen thread/dep repos
	// by creating infrastructure with SQLite backend and running a lifecycle
	db := setupTestDB(t)

	cfg := InfrastructureConfig{
		Port: 8080,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: createFabricTestAgentProvider(t),
		},
		WorkDir: "/tmp/test",
		FabricBackend: &FabricBackendConfig{
			DB:                db.Connection(),
			SessionID:         "session-1",
			SQLiteReadEnabled: true,
		},
	}

	infra, err := NewInfrastructure(cfg)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = infra.Start(ctx)
	require.NoError(t, err)
	defer infra.Shutdown()

	// FabricService should be initialized with channels
	require.NotNil(t, infra.Core.FabricService)
	assert.True(t, infra.Core.Processor.IsRunning())
}

func TestCreateFabricRepositories_DualWrite_PropagatesBothBackends(t *testing.T) {
	db := setupTestDB(t)
	sessionID := "session-dual"

	cfg := &FabricBackendConfig{
		DB:               db.Connection(),
		SessionID:        sessionID,
		DualWriteEnabled: true,
		// Reads from memory (default), writes to both
		SQLiteReadEnabled: false,
	}

	threads, deps := createFabricRepositories(cfg)
	require.NotNil(t, threads)
	require.NotNil(t, deps)

	// Create a thread via the dual-write wrapper
	created, err := threads.Create(domain.Thread{
		Type:      domain.ThreadChannel,
		CreatedBy: "coordinator",
		Slug:      "general",
		Title:     "General",
	})
	require.NoError(t, err)
	require.NotEmpty(t, created.ID)

	// Verify the write landed in the primary (memory) by reading through wrapper
	got, err := threads.Get(created.ID)
	require.NoError(t, err)
	require.NotNil(t, got, "Thread should be readable from primary (memory) backend")
	assert.Equal(t, "General", got.Title)

	// Verify the write ALSO landed in SQLite independently
	sqliteThreads := fabricsqlite.NewSQLiteThreadRepository(db.Connection(), sessionID)
	sqliteGot, err := sqliteThreads.Get(created.ID)
	require.NoError(t, err)
	require.NotNil(t, sqliteGot, "Thread should ALSO exist in SQLite (secondary) backend")
	assert.Equal(t, "General", sqliteGot.Title)

	// Test dual-write for dependencies
	// First create a second thread so we have valid thread IDs for dependency
	msg, err := threads.Create(domain.Thread{
		Type:      domain.ThreadMessage,
		CreatedBy: "worker-1",
		Content:   "Hello!",
	})
	require.NoError(t, err)

	err = deps.Add(domain.Dependency{
		ThreadID:    msg.ID,
		DependsOnID: created.ID,
		Relation:    domain.RelationChildOf,
	})
	require.NoError(t, err)

	// Verify dependency in primary (memory)
	parents, err := deps.GetParents(msg.ID, nil)
	require.NoError(t, err)
	require.Len(t, parents, 1)
	assert.Equal(t, created.ID, parents[0].DependsOnID)

	// Verify dependency ALSO in SQLite independently
	sqliteDeps := fabricsqlite.NewSQLiteDependencyRepository(db.Connection(), sessionID)
	sqliteParents, err := sqliteDeps.GetParents(msg.ID, nil)
	require.NoError(t, err)
	require.Len(t, sqliteParents, 1, "Dependency should ALSO exist in SQLite backend")
	assert.Equal(t, created.ID, sqliteParents[0].DependsOnID)
}

func TestInfrastructure_Start_WithSQLiteBackend(t *testing.T) {
	db := setupTestDB(t)

	cfg := InfrastructureConfig{
		Port: 8080,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: createFabricTestAgentProvider(t),
		},
		WorkDir: "/tmp/test",
		FabricBackend: &FabricBackendConfig{
			DB:                db.Connection(),
			SessionID:         "session-1",
			SQLiteReadEnabled: true,
		},
	}

	infra, err := NewInfrastructure(cfg)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = infra.Start(ctx)
	require.NoError(t, err)
	assert.True(t, infra.Core.Processor.IsRunning())
	assert.NotNil(t, infra.Core.FabricService)

	infra.Shutdown()
	assert.False(t, infra.Core.Processor.IsRunning())
}

func TestInfrastructure_FullLifecycle_SQLiteBackend(t *testing.T) {
	db := setupTestDB(t)

	cfg := InfrastructureConfig{
		Port: 8080,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: createFabricTestAgentProvider(t),
		},
		WorkDir: "/tmp/test",
		FabricBackend: &FabricBackendConfig{
			DB:                db.Connection(),
			SessionID:         "session-lifecycle",
			DualWriteEnabled:  true,
			SQLiteReadEnabled: false,
		},
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

	// Verify FabricService is functional
	assert.NotNil(t, infra.Core.FabricService)

	// Verify core components
	assert.NotNil(t, infra.Core.Adapter)
	assert.NotNil(t, infra.Core.EventBus)
	assert.NotNil(t, infra.Core.CmdSubmitter)
	assert.NotNil(t, infra.Repositories.ProcessRepo)
	assert.NotNil(t, infra.Repositories.TaskRepo)
	assert.NotNil(t, infra.Repositories.QueueRepo)
	assert.NotNil(t, infra.Internal.ProcessRegistry)

	// Shutdown
	infra.Shutdown()
	assert.False(t, infra.Core.Processor.IsRunning())
}

// ===========================================================================
// Interface compliance verification
// ===========================================================================

// Verify dual-write wrappers satisfy the interfaces at compile time.
var _ fabricrepo.ThreadRepository = (*dualWriteThreadRepository)(nil)
var _ fabricrepo.DependencyRepository = (*dualWriteDependencyRepository)(nil)
