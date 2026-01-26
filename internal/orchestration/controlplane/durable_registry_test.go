package controlplane

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/infrastructure/sqlite"
)

func TestDurableRegistry_Put_Get(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	registry := NewDurableRegistry("test-project", db.SessionRepository())

	// Create a workflow
	spec := &WorkflowSpec{
		TemplateID:    "test-template",
		InitialPrompt: "Test prompt",
		Name:          "Test Workflow",
		WorkDir:       "/tmp/test",
		Labels:        map[string]string{"env": "test"},
		EpicID:        "EPIC-123",
	}
	inst, err := NewWorkflowInstance(spec)
	require.NoError(t, err)

	// Put workflow
	err = registry.Put(inst)
	require.NoError(t, err)

	// Get workflow
	retrieved, found := registry.Get(inst.ID)
	require.True(t, found)
	require.Equal(t, inst.ID, retrieved.ID)
	require.Equal(t, "Test Workflow", retrieved.Name)
	require.Equal(t, "test-template", retrieved.TemplateID)
	require.Equal(t, "EPIC-123", retrieved.EpicID)
}

func TestDurableRegistry_Put_Duplicate(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	registry := NewDurableRegistry("test-project", db.SessionRepository())

	spec := &WorkflowSpec{
		TemplateID:    "test-template",
		InitialPrompt: "Test prompt",
	}
	inst, err := NewWorkflowInstance(spec)
	require.NoError(t, err)

	// First put should succeed
	err = registry.Put(inst)
	require.NoError(t, err)

	// Second put should fail
	err = registry.Put(inst)
	require.Error(t, err)
	require.Contains(t, err.Error(), "already exists")
}

func TestDurableRegistry_Update(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	registry := NewDurableRegistry("test-project", db.SessionRepository())

	spec := &WorkflowSpec{
		TemplateID:    "test-template",
		InitialPrompt: "Test prompt",
		Name:          "Original Name",
	}
	inst, err := NewWorkflowInstance(spec)
	require.NoError(t, err)

	err = registry.Put(inst)
	require.NoError(t, err)

	// Update the workflow
	err = registry.Update(inst.ID, func(w *WorkflowInstance) {
		w.Name = "Updated Name"
		w.TokensUsed = 1000
	})
	require.NoError(t, err)

	// Verify update
	retrieved, found := registry.Get(inst.ID)
	require.True(t, found)
	require.Equal(t, "Updated Name", retrieved.Name)
	require.Equal(t, int64(1000), retrieved.TokensUsed)
}

func TestDurableRegistry_Update_NotInRuntime(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	registry := NewDurableRegistry("test-project", db.SessionRepository())

	spec := &WorkflowSpec{
		TemplateID:    "test-template",
		InitialPrompt: "Test prompt",
		Name:          "Original Name",
	}
	inst, err := NewWorkflowInstance(spec)
	require.NoError(t, err)

	err = registry.Put(inst)
	require.NoError(t, err)

	// Simulate app restart by creating a new registry instance
	// pointing to the same database - workflow won't be in runtimes
	registry2 := NewDurableRegistry("test-project", db.SessionRepository())

	// Update the workflow (not in runtimes, only in database)
	err = registry2.Update(inst.ID, func(w *WorkflowInstance) {
		w.Name = "Renamed After Restart"
	})
	require.NoError(t, err)

	// Verify update persisted
	retrieved, found := registry2.Get(inst.ID)
	require.True(t, found)
	require.Equal(t, "Renamed After Restart", retrieved.Name)
}

func TestDurableRegistry_List(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	registry := NewDurableRegistry("test-project", db.SessionRepository())

	// Create multiple workflows
	for i := 0; i < 3; i++ {
		spec := &WorkflowSpec{
			TemplateID:    "test-template",
			InitialPrompt: "Test prompt",
		}
		inst, err := NewWorkflowInstance(spec)
		require.NoError(t, err)
		err = registry.Put(inst)
		require.NoError(t, err)
	}

	// List all
	results := registry.List(ListQuery{})
	require.Len(t, results, 3)
}

func TestDurableRegistry_List_StateFilter(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	registry := NewDurableRegistry("test-project", db.SessionRepository())

	// Create workflows in different states
	spec := &WorkflowSpec{
		TemplateID:    "test-template",
		InitialPrompt: "Test prompt",
	}

	// Pending workflow
	pending, err := NewWorkflowInstance(spec)
	require.NoError(t, err)
	err = registry.Put(pending)
	require.NoError(t, err)

	// Running workflow
	running, err := NewWorkflowInstance(spec)
	require.NoError(t, err)
	err = running.TransitionTo(WorkflowRunning)
	require.NoError(t, err)
	err = registry.Put(running)
	require.NoError(t, err)

	// Filter by running state
	results := registry.List(ListQuery{States: []WorkflowState{WorkflowRunning}})
	require.Len(t, results, 1)
	require.Equal(t, running.ID, results[0].ID)
}

func TestDurableRegistry_Remove(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	registry := NewDurableRegistry("test-project", db.SessionRepository())

	spec := &WorkflowSpec{
		TemplateID:    "test-template",
		InitialPrompt: "Test prompt",
	}
	inst, err := NewWorkflowInstance(spec)
	require.NoError(t, err)

	err = registry.Put(inst)
	require.NoError(t, err)

	// Remove workflow
	err = registry.Remove(inst.ID)
	require.NoError(t, err)

	// Should not be found in runtime
	require.False(t, registry.HasRuntime(inst.ID))
}

func TestDurableRegistry_Count(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	registry := NewDurableRegistry("test-project", db.SessionRepository())

	spec := &WorkflowSpec{
		TemplateID:    "test-template",
		InitialPrompt: "Test prompt",
	}

	// Create 2 pending, 1 running
	for i := 0; i < 2; i++ {
		inst, err := NewWorkflowInstance(spec)
		require.NoError(t, err)
		err = registry.Put(inst)
		require.NoError(t, err)
	}

	running, err := NewWorkflowInstance(spec)
	require.NoError(t, err)
	err = running.TransitionTo(WorkflowRunning)
	require.NoError(t, err)
	err = registry.Put(running)
	require.NoError(t, err)

	counts := registry.Count()
	require.Equal(t, 2, counts[WorkflowPending])
	require.Equal(t, 1, counts[WorkflowRunning])
}

func TestDurableRegistry_AttachDetachRuntime(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	registry := NewDurableRegistry("test-project", db.SessionRepository())

	spec := &WorkflowSpec{
		TemplateID:    "test-template",
		InitialPrompt: "Test prompt",
	}
	inst, err := NewWorkflowInstance(spec)
	require.NoError(t, err)

	// Put creates runtime
	err = registry.Put(inst)
	require.NoError(t, err)
	require.True(t, registry.HasRuntime(inst.ID))

	// Detach runtime (like pausing)
	registry.DetachRuntime(inst.ID)
	require.False(t, registry.HasRuntime(inst.ID))

	// Workflow still exists in SQLite (Get should find it)
	retrieved, found := registry.Get(inst.ID)
	require.True(t, found)
	require.Equal(t, inst.ID, retrieved.ID)

	// Attach runtime again (like resuming)
	err = registry.AttachRuntime(inst)
	require.NoError(t, err)
	require.True(t, registry.HasRuntime(inst.ID))
}

func TestDurableRegistry_ProjectIsolation(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	registry1 := NewDurableRegistry("project-1", db.SessionRepository())
	registry2 := NewDurableRegistry("project-2", db.SessionRepository())

	spec := &WorkflowSpec{
		TemplateID:    "test-template",
		InitialPrompt: "Test prompt",
	}

	// Create workflow in project-1
	inst, err := NewWorkflowInstance(spec)
	require.NoError(t, err)
	err = registry1.Put(inst)
	require.NoError(t, err)

	// Should be found in project-1
	_, found := registry1.Get(inst.ID)
	require.True(t, found)

	// Should NOT be found in project-2 (different project isolation)
	// Note: runtime is project-specific, but SQLite lookup uses project filter
	results := registry2.List(ListQuery{})
	require.Len(t, results, 0)
}

func TestDurableRegistry_Put_SetsOwnerPIDs(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	registry := NewDurableRegistry("test-project", db.SessionRepository())

	spec := &WorkflowSpec{
		TemplateID:    "test-template",
		InitialPrompt: "Test prompt",
		Name:          "Test Workflow",
	}
	inst, err := NewWorkflowInstance(spec)
	require.NoError(t, err)

	// Put workflow
	err = registry.Put(inst)
	require.NoError(t, err)

	// Retrieve the session directly from the repository to verify PIDs
	session, err := db.SessionRepository().FindByGUID("test-project", string(inst.ID))
	require.NoError(t, err)

	// Verify owner PIDs are set to current process
	currentPID := os.Getpid()
	require.NotNil(t, session.OwnerCreatedPID(), "owner_created_pid should be set")
	require.NotNil(t, session.OwnerCurrentPID(), "owner_current_pid should be set")
	require.Equal(t, currentPID, *session.OwnerCreatedPID(), "owner_created_pid should match current process")
	require.Equal(t, currentPID, *session.OwnerCurrentPID(), "owner_current_pid should match current process")
}

func TestDurableRegistry_List_OwnedByCurrentProcess_NotLocked(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	registry := NewDurableRegistry("test-project", db.SessionRepository())

	spec := &WorkflowSpec{
		TemplateID:    "test-template",
		InitialPrompt: "Test prompt",
		Name:          "My Workflow",
	}
	inst, err := NewWorkflowInstance(spec)
	require.NoError(t, err)
	require.NoError(t, registry.Put(inst))

	// List should return workflow as not locked (we own it)
	results := registry.List(ListQuery{})
	require.Len(t, results, 1)
	require.False(t, results[0].IsLocked, "workflow owned by current process should not be locked")
}

func TestDurableRegistry_List_OwnedByDeadProcess_ClaimsOwnership(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	registry := NewDurableRegistry("test-project", db.SessionRepository())

	spec := &WorkflowSpec{
		TemplateID:    "test-template",
		InitialPrompt: "Test prompt",
		Name:          "Orphaned Workflow",
	}
	inst, err := NewWorkflowInstance(spec)
	require.NoError(t, err)
	require.NoError(t, registry.Put(inst))

	// Simulate the workflow being owned by a dead process (PID that doesn't exist)
	// Use a very high PID that's unlikely to exist
	deadPID := 999999999
	session, err := db.SessionRepository().FindByGUID("test-project", string(inst.ID))
	require.NoError(t, err)
	session.SetOwnerCurrentPID(&deadPID)
	require.NoError(t, db.SessionRepository().Save(session))

	// List should claim ownership and not mark as locked
	results := registry.List(ListQuery{})
	require.Len(t, results, 1)
	require.False(t, results[0].IsLocked, "workflow with dead owner should not be locked")

	// Verify ownership was claimed (owner_current_pid updated to current process)
	session, err = db.SessionRepository().FindByGUID("test-project", string(inst.ID))
	require.NoError(t, err)
	require.NotNil(t, session.OwnerCurrentPID())
	require.Equal(t, os.Getpid(), *session.OwnerCurrentPID(), "should have claimed ownership")
}

func TestDurableRegistry_List_OwnedByLiveProcess_IsLocked(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	registry := NewDurableRegistry("test-project", db.SessionRepository())

	spec := &WorkflowSpec{
		TemplateID:    "test-template",
		InitialPrompt: "Test prompt",
		Name:          "Locked Workflow",
	}
	inst, err := NewWorkflowInstance(spec)
	require.NoError(t, err)
	require.NoError(t, registry.Put(inst))

	// Simulate the workflow being owned by another live process.
	// Use current process PID (we know it's alive) but trick the test
	// by using os.Getppid() - the parent process which is always alive.
	parentPID := os.Getppid()
	session, err := db.SessionRepository().FindByGUID("test-project", string(inst.ID))
	require.NoError(t, err)
	session.SetOwnerCurrentPID(&parentPID)
	require.NoError(t, db.SessionRepository().Save(session))

	// List should mark as locked (owned by another live process)
	results := registry.List(ListQuery{})
	require.Len(t, results, 1)
	require.True(t, results[0].IsLocked, "workflow owned by live process should be locked")

	// Verify ownership was NOT claimed (still owned by parent PID)
	session, err = db.SessionRepository().FindByGUID("test-project", string(inst.ID))
	require.NoError(t, err)
	require.NotNil(t, session.OwnerCurrentPID())
	require.Equal(t, parentPID, *session.OwnerCurrentPID(), "should not have claimed ownership")
}

func TestDurableRegistry_Archive(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	registry := NewDurableRegistry("test-project", db.SessionRepository())

	spec := &WorkflowSpec{
		TemplateID:    "test-template",
		InitialPrompt: "Test prompt",
		Name:          "Test Workflow",
	}
	inst, err := NewWorkflowInstance(spec)
	require.NoError(t, err)

	// Put workflow
	err = registry.Put(inst)
	require.NoError(t, err)

	// Archive workflow
	err = registry.Archive(inst.ID)
	require.NoError(t, err)

	// Workflow should no longer appear in List (archived sessions are excluded)
	results := registry.List(ListQuery{})
	require.Len(t, results, 0, "archived workflow should not appear in list")

	// Workflow should still be retrievable via Get (for viewing details)
	retrieved, found := registry.Get(inst.ID)
	require.True(t, found, "archived workflow should still be findable via Get")
	require.Equal(t, inst.ID, retrieved.ID)

	// Verify session is marked as archived in database
	session, err := db.SessionRepository().FindByGUID("test-project", string(inst.ID))
	require.NoError(t, err)
	require.True(t, session.IsArchived(), "session should be marked as archived")

	// Runtime should be removed
	require.False(t, registry.HasRuntime(inst.ID), "runtime should be removed after archive")
}

func TestDurableRegistry_Archive_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	registry := NewDurableRegistry("test-project", db.SessionRepository())

	// Try to archive non-existent workflow
	err := registry.Archive("non-existent-id")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

// setupTestDB creates a test database with migrations applied.
func setupTestDB(t *testing.T) (*sqlite.DB, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	db, err := sqlite.NewDB(dbPath)
	require.NoError(t, err)

	cleanup := func() {
		_ = db.Close()
	}

	return db, cleanup
}
