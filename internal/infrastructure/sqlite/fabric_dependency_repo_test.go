package sqlite

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	domain "github.com/zjrosen/perles/internal/orchestration/fabric/domain"
	"github.com/zjrosen/perles/internal/orchestration/fabric/repository"
)

// setupDepRepo creates a new DB and returns both a thread and dependency repo for testing.
// Thread repo is needed to create threads referenced by foreign keys.
func setupDepRepo(t *testing.T, sessionID string) (*SQLiteDependencyRepository, *SQLiteThreadRepository) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB(dbPath)
	require.NoError(t, err, "Failed to create test database")
	t.Cleanup(func() { db.Close() })
	depRepo := NewSQLiteDependencyRepository(db.Connection(), sessionID)
	threadRepo := NewSQLiteThreadRepository(db.Connection(), sessionID)
	return depRepo, threadRepo
}

// createThread is a helper to create a thread with minimal fields for dependency tests.
func createThread(t *testing.T, repo *SQLiteThreadRepository, id string) {
	t.Helper()
	_, err := repo.Create(domain.Thread{
		ID:        id,
		Type:      domain.ThreadChannel,
		CreatedBy: "system",
	})
	require.NoError(t, err)
}

func TestSQLiteDepRepo_Add_CreatesDependencyEdge(t *testing.T) {
	depRepo, threadRepo := setupDepRepo(t, "session-1")

	createThread(t, threadRepo, "msg-1")
	createThread(t, threadRepo, "channel-1")

	dep := domain.NewDependency("msg-1", "channel-1", domain.RelationChildOf)
	err := depRepo.Add(dep)
	require.NoError(t, err)

	parents, err := depRepo.GetParents("msg-1", nil)
	require.NoError(t, err)
	require.Len(t, parents, 1)
	require.Equal(t, "msg-1", parents[0].ThreadID)
	require.Equal(t, "channel-1", parents[0].DependsOnID)
	require.Equal(t, domain.RelationChildOf, parents[0].Relation)
}

func TestSQLiteDepRepo_Add_Idempotent(t *testing.T) {
	depRepo, threadRepo := setupDepRepo(t, "session-1")

	createThread(t, threadRepo, "msg-1")
	createThread(t, threadRepo, "channel-1")

	dep := domain.NewDependency("msg-1", "channel-1", domain.RelationChildOf)

	err := depRepo.Add(dep)
	require.NoError(t, err)

	// Adding again should not error (INSERT OR IGNORE)
	err = depRepo.Add(dep)
	require.NoError(t, err)

	// Should still have exactly one dependency
	parents, err := depRepo.GetParents("msg-1", nil)
	require.NoError(t, err)
	require.Len(t, parents, 1)
}

func TestSQLiteDepRepo_Remove_DeletesAllRelations(t *testing.T) {
	depRepo, threadRepo := setupDepRepo(t, "session-1")

	createThread(t, threadRepo, "msg-1")
	createThread(t, threadRepo, "channel-1")

	// Add two different relation types between the same threads
	err := depRepo.Add(domain.NewDependency("msg-1", "channel-1", domain.RelationChildOf))
	require.NoError(t, err)
	err = depRepo.Add(domain.NewDependency("msg-1", "channel-1", domain.RelationReplyTo))
	require.NoError(t, err)

	parents, err := depRepo.GetParents("msg-1", nil)
	require.NoError(t, err)
	require.Len(t, parents, 2)

	// Remove should delete all relations between these two threads
	err = depRepo.Remove("msg-1", "channel-1")
	require.NoError(t, err)

	parents, err = depRepo.GetParents("msg-1", nil)
	require.NoError(t, err)
	require.Len(t, parents, 0)
}

func TestSQLiteDepRepo_Remove_NonExistent_NoError(t *testing.T) {
	depRepo, _ := setupDepRepo(t, "session-1")

	// Removing a non-existent dependency should not error
	err := depRepo.Remove("nonexistent", "also-nonexistent")
	require.NoError(t, err)
}

func TestSQLiteDepRepo_GetParents_WithRelation_ReturnsOnlyMatching(t *testing.T) {
	depRepo, threadRepo := setupDepRepo(t, "session-1")

	createThread(t, threadRepo, "msg-2")
	createThread(t, threadRepo, "msg-1")
	createThread(t, threadRepo, "channel-1")

	// msg-2 has multiple parent relations
	err := depRepo.Add(domain.NewDependency("msg-2", "channel-1", domain.RelationChildOf))
	require.NoError(t, err)
	err = depRepo.Add(domain.NewDependency("msg-2", "msg-1", domain.RelationReplyTo))
	require.NoError(t, err)

	// Filter by child_of
	childOf := domain.RelationChildOf
	parents, err := depRepo.GetParents("msg-2", &childOf)
	require.NoError(t, err)
	require.Len(t, parents, 1)
	require.Equal(t, "channel-1", parents[0].DependsOnID)
	require.Equal(t, domain.RelationChildOf, parents[0].Relation)

	// Filter by reply_to
	replyTo := domain.RelationReplyTo
	parents, err = depRepo.GetParents("msg-2", &replyTo)
	require.NoError(t, err)
	require.Len(t, parents, 1)
	require.Equal(t, "msg-1", parents[0].DependsOnID)
	require.Equal(t, domain.RelationReplyTo, parents[0].Relation)
}

func TestSQLiteDepRepo_GetParents_NilRelation_ReturnsAll(t *testing.T) {
	depRepo, threadRepo := setupDepRepo(t, "session-1")

	createThread(t, threadRepo, "msg-2")
	createThread(t, threadRepo, "msg-1")
	createThread(t, threadRepo, "channel-1")
	createThread(t, threadRepo, "artifact-1")

	// msg-2 has three different relation types
	err := depRepo.Add(domain.NewDependency("msg-2", "channel-1", domain.RelationChildOf))
	require.NoError(t, err)
	err = depRepo.Add(domain.NewDependency("msg-2", "msg-1", domain.RelationReplyTo))
	require.NoError(t, err)
	err = depRepo.Add(domain.NewDependency("msg-2", "artifact-1", domain.RelationReferences))
	require.NoError(t, err)

	parents, err := depRepo.GetParents("msg-2", nil)
	require.NoError(t, err)
	require.Len(t, parents, 3)
}

func TestSQLiteDepRepo_GetChildren_WithRelation_ReturnsOnlyMatching(t *testing.T) {
	depRepo, threadRepo := setupDepRepo(t, "session-1")

	createThread(t, threadRepo, "channel-1")
	createThread(t, threadRepo, "msg-1")
	createThread(t, threadRepo, "msg-2")
	createThread(t, threadRepo, "artifact-1")

	// channel-1 has child_of children and a references child
	err := depRepo.Add(domain.NewDependency("msg-1", "channel-1", domain.RelationChildOf))
	require.NoError(t, err)
	err = depRepo.Add(domain.NewDependency("msg-2", "channel-1", domain.RelationChildOf))
	require.NoError(t, err)
	err = depRepo.Add(domain.NewDependency("artifact-1", "channel-1", domain.RelationReferences))
	require.NoError(t, err)

	// Filter by child_of
	childOf := domain.RelationChildOf
	children, err := depRepo.GetChildren("channel-1", &childOf)
	require.NoError(t, err)
	require.Len(t, children, 2)
	for _, child := range children {
		require.Equal(t, domain.RelationChildOf, child.Relation)
	}

	// Filter by references
	refs := domain.RelationReferences
	children, err = depRepo.GetChildren("channel-1", &refs)
	require.NoError(t, err)
	require.Len(t, children, 1)
	require.Equal(t, "artifact-1", children[0].ThreadID)
}

func TestSQLiteDepRepo_GetChildren_NilRelation_ReturnsAll(t *testing.T) {
	depRepo, threadRepo := setupDepRepo(t, "session-1")

	createThread(t, threadRepo, "channel-1")
	createThread(t, threadRepo, "msg-1")
	createThread(t, threadRepo, "msg-2")
	createThread(t, threadRepo, "artifact-1")

	err := depRepo.Add(domain.NewDependency("msg-1", "channel-1", domain.RelationChildOf))
	require.NoError(t, err)
	err = depRepo.Add(domain.NewDependency("msg-2", "channel-1", domain.RelationChildOf))
	require.NoError(t, err)
	err = depRepo.Add(domain.NewDependency("artifact-1", "channel-1", domain.RelationReferences))
	require.NoError(t, err)

	children, err := depRepo.GetChildren("channel-1", nil)
	require.NoError(t, err)
	require.Len(t, children, 3)
}

func TestSQLiteDepRepo_GetParentsChildren_ReturnDependencyType(t *testing.T) {
	depRepo, threadRepo := setupDepRepo(t, "session-1")

	createThread(t, threadRepo, "msg-1")
	createThread(t, threadRepo, "channel-1")

	err := depRepo.Add(domain.NewDependency("msg-1", "channel-1", domain.RelationChildOf))
	require.NoError(t, err)

	// GetParents returns []domain.Dependency
	parents, err := depRepo.GetParents("msg-1", nil)
	require.NoError(t, err)
	require.Len(t, parents, 1)
	require.IsType(t, domain.Dependency{}, parents[0])
	require.Equal(t, "msg-1", parents[0].ThreadID)
	require.Equal(t, "channel-1", parents[0].DependsOnID)

	// GetChildren returns []domain.Dependency
	children, err := depRepo.GetChildren("channel-1", nil)
	require.NoError(t, err)
	require.Len(t, children, 1)
	require.IsType(t, domain.Dependency{}, children[0])
	require.Equal(t, "msg-1", children[0].ThreadID)
	require.Equal(t, "channel-1", children[0].DependsOnID)
}

func TestSQLiteDepRepo_GetRoots_ReturnsThreadsWithNoChildOfParent(t *testing.T) {
	depRepo, threadRepo := setupDepRepo(t, "session-1")

	createThread(t, threadRepo, "root-channel")
	createThread(t, threadRepo, "channel-1")
	createThread(t, threadRepo, "msg-1")

	// channel-1 is child_of root-channel
	err := depRepo.Add(domain.NewDependency("channel-1", "root-channel", domain.RelationChildOf))
	require.NoError(t, err)
	// msg-1 is child_of channel-1
	err = depRepo.Add(domain.NewDependency("msg-1", "channel-1", domain.RelationChildOf))
	require.NoError(t, err)

	roots, err := depRepo.GetRoots()
	require.NoError(t, err)
	require.Len(t, roots, 1)
	require.Equal(t, "root-channel", roots[0])
}

func TestSQLiteDepRepo_GetRoots_ExcludesThreadsWithChildOf(t *testing.T) {
	depRepo, threadRepo := setupDepRepo(t, "session-1")

	createThread(t, threadRepo, "root")
	createThread(t, threadRepo, "child")
	createThread(t, threadRepo, "grandchild")

	err := depRepo.Add(domain.NewDependency("child", "root", domain.RelationChildOf))
	require.NoError(t, err)
	err = depRepo.Add(domain.NewDependency("grandchild", "child", domain.RelationChildOf))
	require.NoError(t, err)

	roots, err := depRepo.GetRoots()
	require.NoError(t, err)
	require.Len(t, roots, 1)
	require.Equal(t, "root", roots[0])
}

func TestSQLiteDepRepo_SessionIsolation(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	threadRepo1 := NewSQLiteThreadRepository(db.Connection(), "session-1")
	depRepo1 := NewSQLiteDependencyRepository(db.Connection(), "session-1")

	threadRepo2 := NewSQLiteThreadRepository(db.Connection(), "session-2")
	depRepo2 := NewSQLiteDependencyRepository(db.Connection(), "session-2")

	// Create threads in session-1
	createThread(t, threadRepo1, "msg-1")
	createThread(t, threadRepo1, "channel-1")

	// Create threads in session-2 with same IDs
	createThread(t, threadRepo2, "msg-1")
	createThread(t, threadRepo2, "channel-1")

	// Add dependency in session-1
	err = depRepo1.Add(domain.NewDependency("msg-1", "channel-1", domain.RelationChildOf))
	require.NoError(t, err)

	// Session-1 sees the dependency
	parents1, err := depRepo1.GetParents("msg-1", nil)
	require.NoError(t, err)
	require.Len(t, parents1, 1)

	// Session-2 does NOT see it
	parents2, err := depRepo2.GetParents("msg-1", nil)
	require.NoError(t, err)
	require.Len(t, parents2, 0)

	// Add a different dependency in session-2
	err = depRepo2.Add(domain.NewDependency("msg-1", "channel-1", domain.RelationReplyTo))
	require.NoError(t, err)

	// Session-1 still has only child_of
	parents1, err = depRepo1.GetParents("msg-1", nil)
	require.NoError(t, err)
	require.Len(t, parents1, 1)
	require.Equal(t, domain.RelationChildOf, parents1[0].Relation)

	// Session-2 has only reply_to
	parents2, err = depRepo2.GetParents("msg-1", nil)
	require.NoError(t, err)
	require.Len(t, parents2, 1)
	require.Equal(t, domain.RelationReplyTo, parents2[0].Relation)
}

func TestSQLiteDepRepo_ForeignKeyEnforcement(t *testing.T) {
	depRepo, _ := setupDepRepo(t, "session-1")

	// Adding a dependency referencing non-existent threads should fail due to FK constraint
	dep := domain.NewDependency("nonexistent-msg", "nonexistent-channel", domain.RelationChildOf)
	err := depRepo.Add(dep)
	require.Error(t, err, "Foreign key constraint should prevent adding dependency for non-existent threads")
}

func TestSQLiteDepRepo_Parity_WithMemory(t *testing.T) {
	memRepo := repository.NewMemoryDependencyRepository()
	depRepo, threadRepo := setupDepRepo(t, "parity-session")

	// Create threads for FK satisfaction
	createThread(t, threadRepo, "channel-1")
	createThread(t, threadRepo, "msg-1")
	createThread(t, threadRepo, "msg-2")
	createThread(t, threadRepo, "artifact-1")

	// 1. Add dependencies (same ops on both)
	deps := []domain.Dependency{
		domain.NewDependency("msg-1", "channel-1", domain.RelationChildOf),
		domain.NewDependency("msg-2", "channel-1", domain.RelationChildOf),
		domain.NewDependency("msg-2", "msg-1", domain.RelationReplyTo),
		domain.NewDependency("artifact-1", "msg-1", domain.RelationReferences),
	}
	for _, dep := range deps {
		err := memRepo.Add(dep)
		require.NoError(t, err)
		err = depRepo.Add(dep)
		require.NoError(t, err)
	}

	// 2. Idempotent add (should not error for both)
	err := memRepo.Add(deps[0])
	require.NoError(t, err)
	err = depRepo.Add(deps[0])
	require.NoError(t, err)

	// 3. GetParents with nil relation returns same count
	memParents, err := memRepo.GetParents("msg-2", nil)
	require.NoError(t, err)
	sqlParents, err := depRepo.GetParents("msg-2", nil)
	require.NoError(t, err)
	require.Equal(t, len(memParents), len(sqlParents))

	// 4. GetParents with specific relation returns same count
	childOf := domain.RelationChildOf
	memParents, err = memRepo.GetParents("msg-2", &childOf)
	require.NoError(t, err)
	sqlParents, err = depRepo.GetParents("msg-2", &childOf)
	require.NoError(t, err)
	require.Equal(t, len(memParents), len(sqlParents))
	require.Equal(t, memParents[0].DependsOnID, sqlParents[0].DependsOnID)

	replyTo := domain.RelationReplyTo
	memParents, err = memRepo.GetParents("msg-2", &replyTo)
	require.NoError(t, err)
	sqlParents, err = depRepo.GetParents("msg-2", &replyTo)
	require.NoError(t, err)
	require.Equal(t, len(memParents), len(sqlParents))
	require.Equal(t, memParents[0].DependsOnID, sqlParents[0].DependsOnID)

	// 5. GetChildren with nil relation returns same count
	memChildren, err := memRepo.GetChildren("channel-1", nil)
	require.NoError(t, err)
	sqlChildren, err := depRepo.GetChildren("channel-1", nil)
	require.NoError(t, err)
	require.Equal(t, len(memChildren), len(sqlChildren))

	// 6. GetChildren with specific relation returns same count
	refs := domain.RelationReferences
	memChildren, err = memRepo.GetChildren("msg-1", &refs)
	require.NoError(t, err)
	sqlChildren, err = depRepo.GetChildren("msg-1", &refs)
	require.NoError(t, err)
	require.Equal(t, len(memChildren), len(sqlChildren))
	require.Equal(t, memChildren[0].ThreadID, sqlChildren[0].ThreadID)

	// 7. GetRoots returns same results
	memRoots, err := memRepo.GetRoots()
	require.NoError(t, err)
	sqlRoots, err := depRepo.GetRoots()
	require.NoError(t, err)
	require.Equal(t, len(memRoots), len(sqlRoots))

	// 8. Remove and verify parity
	err = memRepo.Remove("msg-2", "channel-1")
	require.NoError(t, err)
	err = depRepo.Remove("msg-2", "channel-1")
	require.NoError(t, err)

	memParents, err = memRepo.GetParents("msg-2", nil)
	require.NoError(t, err)
	sqlParents, err = depRepo.GetParents("msg-2", nil)
	require.NoError(t, err)
	require.Equal(t, len(memParents), len(sqlParents))

	// 9. Remove non-existent is no-op for both
	err = memRepo.Remove("nonexistent", "also-nonexistent")
	require.NoError(t, err)
	err = depRepo.Remove("nonexistent", "also-nonexistent")
	require.NoError(t, err)

	// 10. GetParents/GetChildren for non-existent thread returns empty for both
	memParents, err = memRepo.GetParents("nonexistent", nil)
	require.NoError(t, err)
	sqlParents, err = depRepo.GetParents("nonexistent", nil)
	require.NoError(t, err)
	require.Equal(t, len(memParents), len(sqlParents))
	require.Empty(t, memParents)
	require.Empty(t, sqlParents)
}

func TestSQLiteDepRepo_GetRoots_EmptyGraph(t *testing.T) {
	depRepo, _ := setupDepRepo(t, "session-1")

	roots, err := depRepo.GetRoots()
	require.NoError(t, err)
	require.Empty(t, roots)
}

func TestSQLiteDepRepo_GetParents_EmptyResults(t *testing.T) {
	depRepo, _ := setupDepRepo(t, "session-1")

	parents, err := depRepo.GetParents("nonexistent", nil)
	require.NoError(t, err)
	require.Empty(t, parents)
}

func TestSQLiteDepRepo_GetChildren_EmptyResults(t *testing.T) {
	depRepo, _ := setupDepRepo(t, "session-1")

	children, err := depRepo.GetChildren("nonexistent", nil)
	require.NoError(t, err)
	require.Empty(t, children)
}

func TestSQLiteDepRepo_GetRoots_NonChildOfRelationDoesntExclude(t *testing.T) {
	depRepo, threadRepo := setupDepRepo(t, "session-1")

	createThread(t, threadRepo, "channel-1")
	createThread(t, threadRepo, "msg-1")

	// msg-1 only has a reply_to relation, NOT child_of
	// So msg-1 should still be considered a root
	err := depRepo.Add(domain.NewDependency("msg-1", "channel-1", domain.RelationReplyTo))
	require.NoError(t, err)

	roots, err := depRepo.GetRoots()
	require.NoError(t, err)
	// Both channel-1 and msg-1 should be roots (neither has child_of)
	require.Len(t, roots, 2)

	rootSet := make(map[string]bool)
	for _, r := range roots {
		rootSet[r] = true
	}
	require.True(t, rootSet["channel-1"])
	require.True(t, rootSet["msg-1"])
}
