package sqlite

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domain "github.com/zjrosen/perles/internal/orchestration/fabric/domain"
	"github.com/zjrosen/perles/internal/orchestration/fabric/repository"
)

// setupThreadRepo creates a new DB and returns a SQLiteThreadRepository for testing.
func setupThreadRepo(t *testing.T, sessionID string) *SQLiteThreadRepository {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB(dbPath)
	require.NoError(t, err, "Failed to create test database")
	t.Cleanup(func() { db.Close() })
	return NewSQLiteThreadRepository(db.Connection(), sessionID)
}

func TestSQLiteThreadRepo_Create_AutoGeneratesID(t *testing.T) {
	repo := setupThreadRepo(t, "session-1")

	thread := domain.Thread{
		Type:      domain.ThreadMessage,
		CreatedBy: "worker-1",
		Content:   "hello",
	}

	created, err := repo.Create(thread)
	require.NoError(t, err)
	require.NotEmpty(t, created.ID, "ID should be auto-generated")
	require.NotZero(t, created.Seq, "Seq should be auto-assigned")
	require.False(t, created.CreatedAt.IsZero(), "CreatedAt should be set")
}

func TestSQLiteThreadRepo_Create_AutoAssignsIncrementingSeq(t *testing.T) {
	repo := setupThreadRepo(t, "session-1")

	t1, err := repo.Create(domain.Thread{
		Type:      domain.ThreadMessage,
		CreatedBy: "worker-1",
	})
	require.NoError(t, err)

	t2, err := repo.Create(domain.Thread{
		Type:      domain.ThreadMessage,
		CreatedBy: "worker-1",
	})
	require.NoError(t, err)

	t3, err := repo.Create(domain.Thread{
		Type:      domain.ThreadMessage,
		CreatedBy: "worker-1",
	})
	require.NoError(t, err)

	require.Equal(t, int64(1), t1.Seq)
	require.Equal(t, int64(2), t2.Seq)
	require.Equal(t, int64(3), t3.Seq)
}

func TestSQLiteThreadRepo_Create_ThenGet_Identical(t *testing.T) {
	repo := setupThreadRepo(t, "session-1")

	archivedAt := time.Date(2026, 2, 9, 12, 0, 0, 0, time.UTC)
	thread := domain.Thread{
		ID:           "msg-001",
		Type:         domain.ThreadMessage,
		CreatedAt:    time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
		CreatedBy:    "worker-1",
		Content:      "Hello, world!",
		Kind:         "info",
		Slug:         "general",
		Title:        "Test Thread",
		Purpose:      "For testing",
		Name:         "artifact.txt",
		MediaType:    "text/plain",
		SizeBytes:    1024,
		StorageURI:   "file:///tmp/test.txt",
		Sha256:       "abc123",
		Mentions:     []string{"worker-2", "coordinator"},
		Participants: []string{"worker-1", "worker-2"},
		Meta:         map[string]string{"task": "bd-42"},
		Seq:          5,
		ArchivedAt:   &archivedAt,
	}

	created, err := repo.Create(thread)
	require.NoError(t, err)

	got, err := repo.Get(created.ID)
	require.NoError(t, err)

	require.Equal(t, created.ID, got.ID)
	require.Equal(t, created.Type, got.Type)
	require.Equal(t, created.CreatedAt.Unix(), got.CreatedAt.Unix())
	require.Equal(t, created.CreatedBy, got.CreatedBy)
	require.Equal(t, created.Content, got.Content)
	require.Equal(t, created.Kind, got.Kind)
	require.Equal(t, created.Slug, got.Slug)
	require.Equal(t, created.Title, got.Title)
	require.Equal(t, created.Purpose, got.Purpose)
	require.Equal(t, created.Name, got.Name)
	require.Equal(t, created.MediaType, got.MediaType)
	require.Equal(t, created.SizeBytes, got.SizeBytes)
	require.Equal(t, created.StorageURI, got.StorageURI)
	require.Equal(t, created.Sha256, got.Sha256)
	require.Equal(t, created.Mentions, got.Mentions)
	require.Equal(t, created.Participants, got.Participants)
	require.Equal(t, created.Meta, got.Meta)
	require.Equal(t, created.Seq, got.Seq)
	require.NotNil(t, got.ArchivedAt)
	require.Equal(t, created.ArchivedAt.Unix(), got.ArchivedAt.Unix())
}

func TestSQLiteThreadRepo_Get_NonExistent_ReturnsError(t *testing.T) {
	repo := setupThreadRepo(t, "session-1")

	got, err := repo.Get("nonexistent-id")
	require.Error(t, err)
	require.Nil(t, got)
	require.Contains(t, err.Error(), "thread not found")
}

func TestSQLiteThreadRepo_GetBySlug_FindsChannel(t *testing.T) {
	repo := setupThreadRepo(t, "session-1")

	_, err := repo.Create(domain.Thread{
		ID:        "ch-tasks",
		Type:      domain.ThreadChannel,
		CreatedBy: "system",
		Slug:      "tasks",
		Title:     "Tasks Channel",
	})
	require.NoError(t, err)

	got, err := repo.GetBySlug("tasks")
	require.NoError(t, err)
	require.Equal(t, "ch-tasks", got.ID)
	require.Equal(t, "tasks", got.Slug)
	require.Equal(t, "Tasks Channel", got.Title)
}

func TestSQLiteThreadRepo_GetBySlug_NonExistent_ReturnsError(t *testing.T) {
	repo := setupThreadRepo(t, "session-1")

	got, err := repo.GetBySlug("nonexistent-slug")
	require.Error(t, err)
	require.Nil(t, got)
	require.Contains(t, err.Error(), "channel not found")
}

func TestSQLiteThreadRepo_List_TypeFilter(t *testing.T) {
	repo := setupThreadRepo(t, "session-1")

	_, err := repo.Create(domain.Thread{ID: "ch-1", Type: domain.ThreadChannel, CreatedBy: "system"})
	require.NoError(t, err)
	_, err = repo.Create(domain.Thread{ID: "msg-1", Type: domain.ThreadMessage, CreatedBy: "worker-1"})
	require.NoError(t, err)
	_, err = repo.Create(domain.Thread{ID: "art-1", Type: domain.ThreadArtifact, CreatedBy: "worker-1"})
	require.NoError(t, err)
	_, err = repo.Create(domain.Thread{ID: "msg-2", Type: domain.ThreadMessage, CreatedBy: "worker-2"})
	require.NoError(t, err)

	msgType := domain.ThreadMessage
	results, err := repo.List(repository.ListOptions{Type: &msgType})
	require.NoError(t, err)
	require.Len(t, results, 2)
	for _, r := range results {
		require.Equal(t, domain.ThreadMessage, r.Type)
	}
}

func TestSQLiteThreadRepo_List_AfterSeqFilter(t *testing.T) {
	repo := setupThreadRepo(t, "session-1")

	// Create 5 threads with seq 1..5
	for i := 0; i < 5; i++ {
		_, err := repo.Create(domain.Thread{
			Type:      domain.ThreadMessage,
			CreatedBy: "worker-1",
		})
		require.NoError(t, err)
	}

	results, err := repo.List(repository.ListOptions{AfterSeq: 3})
	require.NoError(t, err)
	require.Len(t, results, 2)
	require.Equal(t, int64(4), results[0].Seq)
	require.Equal(t, int64(5), results[1].Seq)
}

func TestSQLiteThreadRepo_List_LimitCapsResults(t *testing.T) {
	repo := setupThreadRepo(t, "session-1")

	for i := 0; i < 10; i++ {
		_, err := repo.Create(domain.Thread{
			Type:      domain.ThreadMessage,
			CreatedBy: "worker-1",
		})
		require.NoError(t, err)
	}

	results, err := repo.List(repository.ListOptions{Limit: 3})
	require.NoError(t, err)
	require.Len(t, results, 3)
}

func TestSQLiteThreadRepo_List_CreatedByFilter(t *testing.T) {
	repo := setupThreadRepo(t, "session-1")

	_, err := repo.Create(domain.Thread{ID: "m1", Type: domain.ThreadMessage, CreatedBy: "worker-1"})
	require.NoError(t, err)
	_, err = repo.Create(domain.Thread{ID: "m2", Type: domain.ThreadMessage, CreatedBy: "worker-2"})
	require.NoError(t, err)
	_, err = repo.Create(domain.Thread{ID: "m3", Type: domain.ThreadMessage, CreatedBy: "worker-1"})
	require.NoError(t, err)

	creator := "worker-1"
	results, err := repo.List(repository.ListOptions{CreatedBy: &creator})
	require.NoError(t, err)
	require.Len(t, results, 2)
	for _, r := range results {
		require.Equal(t, "worker-1", r.CreatedBy)
	}
}

func TestSQLiteThreadRepo_List_HasMentionFilter(t *testing.T) {
	repo := setupThreadRepo(t, "session-1")

	_, err := repo.Create(domain.Thread{
		ID:        "m1",
		Type:      domain.ThreadMessage,
		CreatedBy: "worker-1",
		Mentions:  []string{"worker-2", "coordinator"},
	})
	require.NoError(t, err)
	_, err = repo.Create(domain.Thread{
		ID:        "m2",
		Type:      domain.ThreadMessage,
		CreatedBy: "worker-2",
		Mentions:  []string{"worker-1"},
	})
	require.NoError(t, err)
	_, err = repo.Create(domain.Thread{
		ID:        "m3",
		Type:      domain.ThreadMessage,
		CreatedBy: "worker-3",
	})
	require.NoError(t, err)

	mention := "worker-2"
	results, err := repo.List(repository.ListOptions{HasMention: &mention})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "m1", results[0].ID)
}

func TestSQLiteThreadRepo_List_OrderBySeqASC(t *testing.T) {
	repo := setupThreadRepo(t, "session-1")

	// Create threads - they should get seq 1, 2, 3
	_, err := repo.Create(domain.Thread{ID: "third", Type: domain.ThreadMessage, CreatedBy: "worker-1"})
	require.NoError(t, err)
	_, err = repo.Create(domain.Thread{ID: "first", Type: domain.ThreadMessage, CreatedBy: "worker-1"})
	require.NoError(t, err)
	_, err = repo.Create(domain.Thread{ID: "second", Type: domain.ThreadMessage, CreatedBy: "worker-1"})
	require.NoError(t, err)

	results, err := repo.List(repository.ListOptions{})
	require.NoError(t, err)
	require.Len(t, results, 3)

	// Verify ordering is by seq ASC (creation order)
	require.Equal(t, "third", results[0].ID)
	require.Equal(t, "first", results[1].ID)
	require.Equal(t, "second", results[2].ID)

	require.True(t, results[0].Seq < results[1].Seq)
	require.True(t, results[1].Seq < results[2].Seq)
}

func TestSQLiteThreadRepo_Update_ModifiesFields(t *testing.T) {
	repo := setupThreadRepo(t, "session-1")

	created, err := repo.Create(domain.Thread{
		ID:        "msg-1",
		Type:      domain.ThreadMessage,
		CreatedBy: "worker-1",
		Content:   "original content",
		Kind:      "info",
	})
	require.NoError(t, err)
	originalSeq := created.Seq
	originalCreatedAt := created.CreatedAt

	// Update the thread
	updated, err := repo.Update(domain.Thread{
		ID:        "msg-1",
		Type:      domain.ThreadMessage,
		CreatedBy: "worker-1",
		Content:   "updated content",
		Kind:      "response",
		Title:     "New Title",
	})
	require.NoError(t, err)

	// Verify updated fields
	require.Equal(t, "updated content", updated.Content)
	require.Equal(t, "response", updated.Kind)
	require.Equal(t, "New Title", updated.Title)

	// Verify Seq and CreatedAt are preserved
	require.Equal(t, originalSeq, updated.Seq)
	require.Equal(t, originalCreatedAt.Unix(), updated.CreatedAt.Unix())

	// Verify via Get
	got, err := repo.Get("msg-1")
	require.NoError(t, err)
	require.Equal(t, "updated content", got.Content)
	require.Equal(t, "response", got.Kind)
	require.Equal(t, "New Title", got.Title)
	require.Equal(t, originalSeq, got.Seq)
}

func TestSQLiteThreadRepo_Update_NonExistent_ReturnsError(t *testing.T) {
	repo := setupThreadRepo(t, "session-1")

	_, err := repo.Update(domain.Thread{
		ID:        "nonexistent",
		Type:      domain.ThreadMessage,
		CreatedBy: "worker-1",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "thread not found")
}

func TestSQLiteThreadRepo_Archive_SetsArchivedAt(t *testing.T) {
	repo := setupThreadRepo(t, "session-1")

	_, err := repo.Create(domain.Thread{
		ID:        "msg-1",
		Type:      domain.ThreadMessage,
		CreatedBy: "worker-1",
	})
	require.NoError(t, err)

	before := time.Now()
	err = repo.Archive("msg-1")
	require.NoError(t, err)
	after := time.Now()

	got, err := repo.Get("msg-1")
	require.NoError(t, err)
	require.NotNil(t, got.ArchivedAt)
	require.True(t, got.ArchivedAt.Unix() >= before.Unix())
	require.True(t, got.ArchivedAt.Unix() <= after.Unix())
}

func TestSQLiteThreadRepo_Archive_NonExistent_ReturnsError(t *testing.T) {
	repo := setupThreadRepo(t, "session-1")

	err := repo.Archive("nonexistent")
	require.Error(t, err)
	require.Contains(t, err.Error(), "thread not found")
}

func TestSQLiteThreadRepo_SessionIsolation(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	repo1 := NewSQLiteThreadRepository(db.Connection(), "session-1")
	repo2 := NewSQLiteThreadRepository(db.Connection(), "session-2")

	// Create threads in session-1
	_, err = repo1.Create(domain.Thread{
		ID:        "msg-1",
		Type:      domain.ThreadMessage,
		CreatedBy: "worker-1",
		Content:   "session 1 message",
	})
	require.NoError(t, err)

	// Create threads in session-2
	_, err = repo2.Create(domain.Thread{
		ID:        "msg-1", // Same ID, different session
		Type:      domain.ThreadMessage,
		CreatedBy: "worker-2",
		Content:   "session 2 message",
	})
	require.NoError(t, err)

	// Get from session-1
	got1, err := repo1.Get("msg-1")
	require.NoError(t, err)
	require.Equal(t, "session 1 message", got1.Content)

	// Get from session-2
	got2, err := repo2.Get("msg-1")
	require.NoError(t, err)
	require.Equal(t, "session 2 message", got2.Content)

	// List from session-1 sees only its threads
	list1, err := repo1.List(repository.ListOptions{})
	require.NoError(t, err)
	require.Len(t, list1, 1)
	require.Equal(t, "session 1 message", list1[0].Content)

	// List from session-2 sees only its threads
	list2, err := repo2.List(repository.ListOptions{})
	require.NoError(t, err)
	require.Len(t, list2, 1)
	require.Equal(t, "session 2 message", list2[0].Content)
}

func TestSQLiteThreadRepo_Parity_WithMemory(t *testing.T) {
	// Run the same sequence of operations against both SQLite and memory repos
	// and verify identical behavior

	memRepo := repository.NewMemoryThreadRepository()
	sqlRepo := setupThreadRepo(t, "parity-session")

	// 1. Create with empty ID
	memThread, err := memRepo.Create(domain.Thread{
		Type:      domain.ThreadChannel,
		CreatedBy: "system",
		Slug:      "tasks",
		Title:     "Tasks",
	})
	require.NoError(t, err)

	sqlThread, err := sqlRepo.Create(domain.Thread{
		ID:        memThread.ID, // Use the same ID for comparison
		Type:      domain.ThreadChannel,
		CreatedBy: "system",
		Slug:      "tasks",
		Title:     "Tasks",
	})
	require.NoError(t, err)

	require.Equal(t, memThread.Type, sqlThread.Type)
	require.Equal(t, memThread.Slug, sqlThread.Slug)
	require.Equal(t, memThread.Title, sqlThread.Title)

	// 2. Create messages
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("msg-%d", i)
		_, err = memRepo.Create(domain.Thread{
			ID:        id,
			Type:      domain.ThreadMessage,
			CreatedBy: "worker-1",
			Content:   fmt.Sprintf("message %d", i),
		})
		require.NoError(t, err)

		_, err = sqlRepo.Create(domain.Thread{
			ID:        id,
			Type:      domain.ThreadMessage,
			CreatedBy: "worker-1",
			Content:   fmt.Sprintf("message %d", i),
		})
		require.NoError(t, err)
	}

	// 3. Get returns same data
	memGet, err := memRepo.Get("msg-1")
	require.NoError(t, err)
	sqlGet, err := sqlRepo.Get("msg-1")
	require.NoError(t, err)
	require.Equal(t, memGet.ID, sqlGet.ID)
	require.Equal(t, memGet.Content, sqlGet.Content)
	require.Equal(t, memGet.Type, sqlGet.Type)

	// 4. GetBySlug returns same data
	memSlug, err := memRepo.GetBySlug("tasks")
	require.NoError(t, err)
	sqlSlug, err := sqlRepo.GetBySlug("tasks")
	require.NoError(t, err)
	require.Equal(t, memSlug.ID, sqlSlug.ID)
	require.Equal(t, memSlug.Slug, sqlSlug.Slug)

	// 5. List ordering is same
	msgType := domain.ThreadMessage
	memList, err := memRepo.List(repository.ListOptions{Type: &msgType})
	require.NoError(t, err)
	sqlList, err := sqlRepo.List(repository.ListOptions{Type: &msgType})
	require.NoError(t, err)
	require.Len(t, sqlList, len(memList))
	for i := range memList {
		require.Equal(t, memList[i].ID, sqlList[i].ID)
	}

	// 6. Update preserves Seq/CreatedAt
	memUpdated, err := memRepo.Update(domain.Thread{
		ID:        "msg-0",
		Type:      domain.ThreadMessage,
		CreatedBy: "worker-1",
		Content:   "updated message",
	})
	require.NoError(t, err)
	sqlUpdated, err := sqlRepo.Update(domain.Thread{
		ID:        "msg-0",
		Type:      domain.ThreadMessage,
		CreatedBy: "worker-1",
		Content:   "updated message",
	})
	require.NoError(t, err)
	require.Equal(t, memUpdated.Seq, sqlUpdated.Seq)
	require.Equal(t, memUpdated.Content, sqlUpdated.Content)

	// 7. Get non-existent returns error for both
	_, memErr := memRepo.Get("nonexistent")
	_, sqlErr := sqlRepo.Get("nonexistent")
	require.Error(t, memErr)
	require.Error(t, sqlErr)

	// 8. GetBySlug non-existent returns error for both
	_, memErr = memRepo.GetBySlug("nonexistent")
	_, sqlErr = sqlRepo.GetBySlug("nonexistent")
	require.Error(t, memErr)
	require.Error(t, sqlErr)

	// 9. Archive
	err = memRepo.Archive("msg-2")
	require.NoError(t, err)
	err = sqlRepo.Archive("msg-2")
	require.NoError(t, err)

	memArchived, err := memRepo.Get("msg-2")
	require.NoError(t, err)
	sqlArchived, err := sqlRepo.Get("msg-2")
	require.NoError(t, err)
	require.NotNil(t, memArchived.ArchivedAt)
	require.NotNil(t, sqlArchived.ArchivedAt)
}

func TestSQLiteThreadRepo_Create_PreservesExplicitSeq(t *testing.T) {
	repo := setupThreadRepo(t, "session-1")

	// When Seq is explicitly set, it should be preserved
	created, err := repo.Create(domain.Thread{
		ID:        "msg-1",
		Type:      domain.ThreadMessage,
		CreatedBy: "worker-1",
		Seq:       42,
	})
	require.NoError(t, err)
	require.Equal(t, int64(42), created.Seq)
}

func TestSQLiteThreadRepo_List_EmptyResults(t *testing.T) {
	repo := setupThreadRepo(t, "session-1")

	results, err := repo.List(repository.ListOptions{})
	require.NoError(t, err)
	// Returns nil slice (no results), which is expected Go behavior
	require.Empty(t, results)
}

func TestSQLiteThreadRepo_Create_DuplicateID_ReturnsError(t *testing.T) {
	repo := setupThreadRepo(t, "session-1")

	_, err := repo.Create(domain.Thread{
		ID:        "msg-1",
		Type:      domain.ThreadMessage,
		CreatedBy: "worker-1",
	})
	require.NoError(t, err)

	_, err = repo.Create(domain.Thread{
		ID:        "msg-1",
		Type:      domain.ThreadMessage,
		CreatedBy: "worker-2",
	})
	require.Error(t, err, "Duplicate ID within session should fail")
}

func TestSQLiteThreadRepo_Create_MinimalFields_GetsNonNilSlices(t *testing.T) {
	repo := setupThreadRepo(t, "session-1")

	created, err := repo.Create(domain.Thread{
		ID:        "msg-1",
		Type:      domain.ThreadMessage,
		CreatedBy: "worker-1",
	})
	require.NoError(t, err)

	got, err := repo.Get(created.ID)
	require.NoError(t, err)

	// JSON fields should come back as empty slices/maps, not nil
	require.NotNil(t, got.Mentions)
	require.Empty(t, got.Mentions)
	require.NotNil(t, got.Participants)
	require.Empty(t, got.Participants)
	require.NotNil(t, got.Meta)
	require.Empty(t, got.Meta)
}
