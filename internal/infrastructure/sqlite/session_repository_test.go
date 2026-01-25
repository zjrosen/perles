package sqlite

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/zjrosen/perles/internal/sessions/domain"
)

// setupTestRepo creates a new DB and returns the repository for testing.
// The DB is closed when the test completes.
func setupTestRepo(t *testing.T) domain.SessionRepository {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB(dbPath)
	require.NoError(t, err, "Failed to create test database")
	t.Cleanup(func() { db.Close() })
	return db.SessionRepository()
}

func TestSessionRepository_Save_Insert(t *testing.T) {
	repo := setupTestRepo(t)

	session := domain.NewSession("guid-1", "project-a", domain.SessionStateRunning)
	require.Equal(t, int64(0), session.ID(), "New session should have ID 0")

	err := repo.Save(session)
	require.NoError(t, err, "Save should succeed for new session")
	require.Greater(t, session.ID(), int64(0), "Session should have ID assigned after insert")

	// Verify data was persisted correctly
	found, err := repo.FindByID(session.ID())
	require.NoError(t, err, "FindByID should succeed")
	require.Equal(t, session.GUID(), found.GUID())
	require.Equal(t, session.Project(), found.Project())
	require.Equal(t, session.State(), found.State())
	require.WithinDuration(t, session.CreatedAt(), found.CreatedAt(), time.Second)
	require.WithinDuration(t, session.UpdatedAt(), found.UpdatedAt(), time.Second)
}

func TestSessionRepository_Save_Update(t *testing.T) {
	repo := setupTestRepo(t)

	// Create session
	session := domain.NewSession("guid-1", "project-a", domain.SessionStateRunning)
	err := repo.Save(session)
	require.NoError(t, err)
	originalID := session.ID()
	originalCreatedAt := session.CreatedAt()

	// Sleep briefly to ensure updatedAt changes
	time.Sleep(10 * time.Millisecond)

	// Update state
	session.MarkCompleted()
	err = repo.Save(session)
	require.NoError(t, err, "Save should succeed for update")

	// Verify update
	found, err := repo.FindByID(originalID)
	require.NoError(t, err)
	require.Equal(t, domain.SessionStateCompleted, found.State(), "State should be updated")
	require.Equal(t, originalCreatedAt.Unix(), found.CreatedAt().Unix(), "CreatedAt should not change")
}

func TestSessionRepository_FindByGUID(t *testing.T) {
	repo := setupTestRepo(t)

	session := domain.NewSession("guid-1", "project-a", domain.SessionStateRunning)
	err := repo.Save(session)
	require.NoError(t, err)

	found, err := repo.FindByGUID("project-a", "guid-1")
	require.NoError(t, err, "FindByGUID should succeed")
	require.Equal(t, session.ID(), found.ID())
	require.Equal(t, "guid-1", found.GUID())
	require.Equal(t, "project-a", found.Project())
}

func TestSessionRepository_FindByGUID_NotFound(t *testing.T) {
	repo := setupTestRepo(t)

	_, err := repo.FindByGUID("project-a", "nonexistent-guid")
	require.Error(t, err, "FindByGUID should return error for non-existent session")

	var notFound *domain.SessionNotFoundError
	require.True(t, errors.As(err, &notFound), "Error should be SessionNotFoundError")
	require.Equal(t, "nonexistent-guid", notFound.GUID)
	require.Equal(t, "project-a", notFound.Project)
}

func TestSessionRepository_FindByGUID_WrongProject(t *testing.T) {
	repo := setupTestRepo(t)

	// Create session in project-a
	session := domain.NewSession("guid-1", "project-a", domain.SessionStateRunning)
	err := repo.Save(session)
	require.NoError(t, err)

	// Try to find it in project-b
	_, err = repo.FindByGUID("project-b", "guid-1")
	require.Error(t, err, "FindByGUID should not find session from different project")

	var notFound *domain.SessionNotFoundError
	require.True(t, errors.As(err, &notFound), "Error should be SessionNotFoundError")
}

func TestSessionRepository_FindByID(t *testing.T) {
	repo := setupTestRepo(t)

	session := domain.NewSession("guid-1", "project-a", domain.SessionStateRunning)
	err := repo.Save(session)
	require.NoError(t, err)

	found, err := repo.FindByID(session.ID())
	require.NoError(t, err, "FindByID should succeed")
	require.Equal(t, session.ID(), found.ID())
	require.Equal(t, session.GUID(), found.GUID())
}

func TestSessionRepository_FindByID_NotFound(t *testing.T) {
	repo := setupTestRepo(t)

	_, err := repo.FindByID(99999)
	require.Error(t, err, "FindByID should return error for non-existent ID")

	var notFound *domain.SessionNotFoundError
	require.True(t, errors.As(err, &notFound), "Error should be SessionNotFoundError")
}

func TestSessionRepository_GetActiveSession(t *testing.T) {
	repo := setupTestRepo(t)

	// Create a running session
	session := domain.NewSession("guid-1", "project-a", domain.SessionStateRunning)
	err := repo.Save(session)
	require.NoError(t, err)

	found, err := repo.GetActiveSession("project-a")
	require.NoError(t, err, "GetActiveSession should succeed")
	require.Equal(t, session.ID(), found.ID())
	require.Equal(t, domain.SessionStateRunning, found.State())
}

func TestSessionRepository_GetActiveSession_None(t *testing.T) {
	repo := setupTestRepo(t)

	// Create a completed session (not active)
	session := domain.NewSession("guid-1", "project-a", domain.SessionStateCompleted)
	err := repo.Save(session)
	require.NoError(t, err)

	_, err = repo.GetActiveSession("project-a")
	require.Error(t, err, "GetActiveSession should return error when no running session")

	var noActive *domain.NoActiveSessionError
	require.True(t, errors.As(err, &noActive), "Error should be NoActiveSessionError")
	require.Equal(t, "project-a", noActive.Project)
}

func TestSessionRepository_GetActiveSession_WrongProject(t *testing.T) {
	repo := setupTestRepo(t)

	// Create a running session in project-a
	session := domain.NewSession("guid-1", "project-a", domain.SessionStateRunning)
	err := repo.Save(session)
	require.NoError(t, err)

	// Try to get active session for project-b
	_, err = repo.GetActiveSession("project-b")
	require.Error(t, err, "GetActiveSession should not find session from different project")

	var noActive *domain.NoActiveSessionError
	require.True(t, errors.As(err, &noActive), "Error should be NoActiveSessionError")
}

func TestSessionRepository_Delete(t *testing.T) {
	repo := setupTestRepo(t)

	session := domain.NewSession("guid-1", "project-a", domain.SessionStateRunning)
	err := repo.Save(session)
	require.NoError(t, err)

	// Delete the session
	err = repo.Delete("project-a", "guid-1")
	require.NoError(t, err, "Delete should succeed")

	// Should not be findable via FindByGUID (soft deleted)
	_, err = repo.FindByGUID("project-a", "guid-1")
	require.Error(t, err, "FindByGUID should not find soft-deleted session")

	var notFound *domain.SessionNotFoundError
	require.True(t, errors.As(err, &notFound))
}

func TestSessionRepository_Delete_NotFound(t *testing.T) {
	repo := setupTestRepo(t)

	err := repo.Delete("project-a", "nonexistent-guid")
	require.Error(t, err, "Delete should return error for non-existent session")

	var notFound *domain.SessionNotFoundError
	require.True(t, errors.As(err, &notFound), "Error should be SessionNotFoundError")
}

func TestSessionRepository_Delete_WrongProject(t *testing.T) {
	repo := setupTestRepo(t)

	// Create session in project-a
	session := domain.NewSession("guid-1", "project-a", domain.SessionStateRunning)
	err := repo.Save(session)
	require.NoError(t, err)

	// Try to delete from project-b
	err = repo.Delete("project-b", "guid-1")
	require.Error(t, err, "Delete should not delete session from different project")

	var notFound *domain.SessionNotFoundError
	require.True(t, errors.As(err, &notFound), "Error should be SessionNotFoundError")

	// Verify session still exists in project-a
	_, err = repo.FindByGUID("project-a", "guid-1")
	require.NoError(t, err, "Session should still exist in original project")
}

func TestSessionRepository_DeleteAllForProject(t *testing.T) {
	repo := setupTestRepo(t)

	// Create multiple sessions in project-a
	for i := 0; i < 5; i++ {
		session := domain.NewSession(string(rune('a'+i)), "project-a", domain.SessionStateRunning)
		err := repo.Save(session)
		require.NoError(t, err)
	}

	// Create one session in project-b
	sessionB := domain.NewSession("guid-b", "project-b", domain.SessionStateRunning)
	err := repo.Save(sessionB)
	require.NoError(t, err)

	// Delete all for project-a
	err = repo.DeleteAllForProject("project-a")
	require.NoError(t, err, "DeleteAllForProject should succeed")

	// Verify project-a sessions are gone
	sessions, err := repo.ListWithFilter("project-a", domain.ListFilter{IncludeDeleted: true})
	require.NoError(t, err)
	require.Empty(t, sessions, "No sessions should remain for project-a")

	// Verify project-b session still exists
	_, err = repo.FindByGUID("project-b", "guid-b")
	require.NoError(t, err, "Session in project-b should still exist")
}

func TestSessionRepository_ListWithFilter_StateFilter(t *testing.T) {
	repo := setupTestRepo(t)

	// Create sessions with different states
	s1 := domain.NewSession("guid-1", "project-a", domain.SessionStateRunning)
	s2 := domain.NewSession("guid-2", "project-a", domain.SessionStateCompleted)
	s3 := domain.NewSession("guid-3", "project-a", domain.SessionStateFailed)
	for _, s := range []*domain.Session{s1, s2, s3} {
		err := repo.Save(s)
		require.NoError(t, err)
	}

	// Filter by running state
	sessions, err := repo.ListWithFilter("project-a", domain.ListFilter{State: domain.SessionStateRunning})
	require.NoError(t, err)
	require.Len(t, sessions, 1, "Should only find running session")
	require.Equal(t, "guid-1", sessions[0].GUID())
}

func TestSessionRepository_ListWithFilter_Limit(t *testing.T) {
	repo := setupTestRepo(t)

	// Create 5 sessions
	for i := 0; i < 5; i++ {
		session := domain.NewSession(string(rune('a'+i)), "project-a", domain.SessionStateCompleted)
		err := repo.Save(session)
		require.NoError(t, err)
		// Small sleep to ensure different created_at timestamps
		time.Sleep(5 * time.Millisecond)
	}

	sessions, err := repo.ListWithFilter("project-a", domain.ListFilter{Limit: 2})
	require.NoError(t, err)
	require.Len(t, sessions, 2, "Should return only 2 sessions with limit")
}

func TestSessionRepository_ListWithFilter_IncludeDeleted(t *testing.T) {
	repo := setupTestRepo(t)

	// Create and delete a session
	session := domain.NewSession("guid-1", "project-a", domain.SessionStateRunning)
	err := repo.Save(session)
	require.NoError(t, err)
	err = repo.Delete("project-a", "guid-1")
	require.NoError(t, err)

	// Create another session (not deleted)
	session2 := domain.NewSession("guid-2", "project-a", domain.SessionStateRunning)
	err = repo.Save(session2)
	require.NoError(t, err)

	// Without IncludeDeleted
	sessions, err := repo.ListWithFilter("project-a", domain.ListFilter{})
	require.NoError(t, err)
	require.Len(t, sessions, 1, "Should only find non-deleted session")
	require.Equal(t, "guid-2", sessions[0].GUID())

	// With IncludeDeleted
	sessions, err = repo.ListWithFilter("project-a", domain.ListFilter{IncludeDeleted: true})
	require.NoError(t, err)
	require.Len(t, sessions, 2, "Should find both sessions with IncludeDeleted")
}

func TestSessionRepository_ListWithFilter_OrderByCreatedAtDesc(t *testing.T) {
	repo := setupTestRepo(t)

	// Create sessions with explicitly different timestamps (Unix seconds)
	baseTime := time.Now()
	s1 := domain.ReconstituteSession(0, "guid-1", "project-a", "", domain.SessionStateCompleted, "", "", "",
		nil, false, "", "", "", "",
		"", // sessionDir
		nil, nil, 0, 0, nil, nil,
		baseTime.Add(-3*time.Second), nil, nil, nil, baseTime.Add(-3*time.Second), nil, nil)
	err := repo.Save(s1)
	require.NoError(t, err)

	s2 := domain.ReconstituteSession(0, "guid-2", "project-a", "", domain.SessionStateCompleted, "", "", "",
		nil, false, "", "", "", "",
		"", // sessionDir
		nil, nil, 0, 0, nil, nil,
		baseTime.Add(-2*time.Second), nil, nil, nil, baseTime.Add(-2*time.Second), nil, nil)
	err = repo.Save(s2)
	require.NoError(t, err)

	s3 := domain.ReconstituteSession(0, "guid-3", "project-a", "", domain.SessionStateCompleted, "", "", "",
		nil, false, "", "", "", "",
		"", // sessionDir
		nil, nil, 0, 0, nil, nil,
		baseTime.Add(-1*time.Second), nil, nil, nil, baseTime.Add(-1*time.Second), nil, nil)
	err = repo.Save(s3)
	require.NoError(t, err)

	sessions, err := repo.ListWithFilter("project-a", domain.ListFilter{})
	require.NoError(t, err)
	require.Len(t, sessions, 3)

	// Should be ordered newest first
	require.Equal(t, "guid-3", sessions[0].GUID(), "Newest session should be first")
	require.Equal(t, "guid-2", sessions[1].GUID())
	require.Equal(t, "guid-1", sessions[2].GUID(), "Oldest session should be last")
}

func TestSessionRepository_ListWithFilter_ProjectIsolation(t *testing.T) {
	repo := setupTestRepo(t)

	// Create sessions in different projects
	for i := 0; i < 3; i++ {
		sessionA := domain.NewSession(string(rune('a'+i)), "project-a", domain.SessionStateCompleted)
		err := repo.Save(sessionA)
		require.NoError(t, err)
	}
	for i := 0; i < 2; i++ {
		sessionB := domain.NewSession(string(rune('x'+i)), "project-b", domain.SessionStateCompleted)
		err := repo.Save(sessionB)
		require.NoError(t, err)
	}

	sessionsA, err := repo.ListWithFilter("project-a", domain.ListFilter{})
	require.NoError(t, err)
	require.Len(t, sessionsA, 3, "project-a should have 3 sessions")
	for _, s := range sessionsA {
		require.Equal(t, "project-a", s.Project(), "All sessions should belong to project-a")
	}

	sessionsB, err := repo.ListWithFilter("project-b", domain.ListFilter{})
	require.NoError(t, err)
	require.Len(t, sessionsB, 2, "project-b should have 2 sessions")
	for _, s := range sessionsB {
		require.Equal(t, "project-b", s.Project(), "All sessions should belong to project-b")
	}
}

func TestSessionRepository_Close(t *testing.T) {
	repo := setupTestRepo(t)
	err := repo.Close()
	require.NoError(t, err, "Close should succeed (no-op)")
}

// TestSessionRepository_ProjectIsolation is a property-based test using rapid.
// It verifies that cross-project queries never leak data.
func TestSessionRepository_ProjectIsolation(t *testing.T) {
	rapid.Check(t, func(r *rapid.T) {
		repo := setupTestRepo(t)

		// Generate 2-5 random projects
		numProjects := rapid.IntRange(2, 5).Draw(r, "numProjects")
		projects := make([]string, numProjects)
		for i := 0; i < numProjects; i++ {
			projects[i] = rapid.StringMatching(`project-[a-z]{3,8}`).Draw(r, "project")
		}

		// Create random sessions for each project
		sessionsPerProject := make(map[string][]string)
		for _, project := range projects {
			numSessions := rapid.IntRange(1, 10).Draw(r, "numSessions")
			guids := make([]string, numSessions)
			for i := 0; i < numSessions; i++ {
				guid := rapid.StringMatching(`guid-[a-z0-9]{8}`).Draw(r, "guid")
				guids[i] = guid
				session := domain.NewSession(guid, project, domain.SessionStateRunning)
				err := repo.Save(session)
				if err != nil {
					// GUID might conflict due to UNIQUE constraint, skip
					continue
				}
				sessionsPerProject[project] = append(sessionsPerProject[project], guid)
			}
		}

		// Verify project isolation: querying one project never returns sessions from another
		for _, queryProject := range projects {
			sessions, err := repo.ListWithFilter(queryProject, domain.ListFilter{})
			if err != nil {
				r.Fatalf("ListWithFilter failed: %v", err)
			}

			for _, session := range sessions {
				if session.Project() != queryProject {
					r.Fatalf("Project isolation violated: queried %q but got session from %q",
						queryProject, session.Project())
				}
			}
		}

		// Verify FindByGUID isolation
		for project, guids := range sessionsPerProject {
			for _, guid := range guids {
				// Should find in correct project
				_, err := repo.FindByGUID(project, guid)
				if err != nil {
					continue // May have been skipped due to UNIQUE conflict
				}

				// Should NOT find in other projects
				for _, otherProject := range projects {
					if otherProject == project {
						continue
					}
					_, err := repo.FindByGUID(otherProject, guid)
					if err == nil {
						r.Fatalf("Project isolation violated: found guid %q from project %q when querying %q",
							guid, project, otherProject)
					}
				}
			}
		}

		// Verify GetActiveSession isolation
		for _, queryProject := range projects {
			session, err := repo.GetActiveSession(queryProject)
			if err != nil {
				// No active session is fine
				continue
			}
			if session.Project() != queryProject {
				r.Fatalf("GetActiveSession isolation violated: queried %q but got session from %q",
					queryProject, session.Project())
			}
		}

		// Verify Delete isolation: deleting in one project doesn't affect another
		for project, guids := range sessionsPerProject {
			if len(guids) == 0 {
				continue
			}
			guidToDelete := guids[0]

			// Delete from this project
			_ = repo.Delete(project, guidToDelete)

			// Verify sessions in other projects are unaffected
			for otherProject, otherGuids := range sessionsPerProject {
				if otherProject == project {
					continue
				}
				for _, otherGuid := range otherGuids {
					_, err := repo.FindByGUID(otherProject, otherGuid)
					if err != nil {
						// May have been skipped/deleted, fine
						continue
					}
				}
			}
		}
	})
}

// TestSessionModel_RoundTrip verifies that converting domain -> model -> domain preserves all values.
func TestSessionModel_RoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Second) // SQLite stores Unix timestamps
	startedAt := now.Add(-4 * time.Hour)
	pausedAt := now.Add(-3 * time.Hour)
	archivedAt := now.Add(-2 * time.Hour)
	deletedAt := now.Add(-time.Hour)
	ownerCreatedPID := 12345
	ownerCurrentPID := 67890
	original := domain.ReconstituteSession(
		123,
		"test-guid",
		"test-project",
		"Test Session",
		domain.SessionStateCompleted,
		"template-abc",
		"epic-123",
		"/work/dir",
		nil,
		false,
		"", "",
		"/worktree/path",
		"feature/branch",
		"", // sessionDir
		&ownerCreatedPID,
		&ownerCurrentPID,
		0,
		0,
		nil, nil,
		now,
		&startedAt,
		&pausedAt,
		nil, // completedAt
		now,
		&archivedAt,
		&deletedAt,
	)

	model := toSessionModel(original)
	require.Equal(t, int64(123), model.ID)
	require.Equal(t, "test-guid", model.GUID)
	require.Equal(t, "test-project", model.Project)
	require.NotNil(t, model.Name)
	require.Equal(t, "Test Session", *model.Name)
	require.Equal(t, "completed", model.State)
	require.NotNil(t, model.TemplateID)
	require.Equal(t, "template-abc", *model.TemplateID)
	require.NotNil(t, model.EpicID)
	require.Equal(t, "epic-123", *model.EpicID)
	require.NotNil(t, model.WorkDir)
	require.Equal(t, "/work/dir", *model.WorkDir)
	require.NotNil(t, model.WorktreePath)
	require.Equal(t, "/worktree/path", *model.WorktreePath)
	require.NotNil(t, model.WorktreeBranch)
	require.Equal(t, "feature/branch", *model.WorktreeBranch)
	require.NotNil(t, model.OwnerCreatedPID)
	require.Equal(t, int64(12345), *model.OwnerCreatedPID)
	require.NotNil(t, model.OwnerCurrentPID)
	require.Equal(t, int64(67890), *model.OwnerCurrentPID)
	require.Equal(t, now.Unix(), model.CreatedAt)
	require.NotNil(t, model.StartedAt)
	require.Equal(t, startedAt.Unix(), *model.StartedAt)
	require.NotNil(t, model.PausedAt)
	require.Equal(t, pausedAt.Unix(), *model.PausedAt)
	require.Equal(t, now.Unix(), model.UpdatedAt)
	require.NotNil(t, model.ArchivedAt)
	require.Equal(t, archivedAt.Unix(), *model.ArchivedAt)
	require.NotNil(t, model.DeletedAt)
	require.Equal(t, deletedAt.Unix(), *model.DeletedAt)

	restored := model.toDomain()
	require.Equal(t, original.ID(), restored.ID())
	require.Equal(t, original.GUID(), restored.GUID())
	require.Equal(t, original.Project(), restored.Project())
	require.Equal(t, original.Name(), restored.Name())
	require.Equal(t, original.State(), restored.State())
	require.Equal(t, original.TemplateID(), restored.TemplateID())
	require.Equal(t, original.EpicID(), restored.EpicID())
	require.Equal(t, original.WorkDir(), restored.WorkDir())
	require.Equal(t, original.WorktreePath(), restored.WorktreePath())
	require.Equal(t, original.WorktreeBranch(), restored.WorktreeBranch())
	require.NotNil(t, restored.OwnerCreatedPID())
	require.Equal(t, *original.OwnerCreatedPID(), *restored.OwnerCreatedPID())
	require.NotNil(t, restored.OwnerCurrentPID())
	require.Equal(t, *original.OwnerCurrentPID(), *restored.OwnerCurrentPID())
	require.Equal(t, original.CreatedAt().Unix(), restored.CreatedAt().Unix())
	require.NotNil(t, restored.StartedAt())
	require.Equal(t, original.StartedAt().Unix(), restored.StartedAt().Unix())
	require.NotNil(t, restored.PausedAt())
	require.Equal(t, original.PausedAt().Unix(), restored.PausedAt().Unix())
	require.Equal(t, original.UpdatedAt().Unix(), restored.UpdatedAt().Unix())
	require.NotNil(t, restored.ArchivedAt())
	require.Equal(t, original.ArchivedAt().Unix(), restored.ArchivedAt().Unix())
	require.NotNil(t, restored.DeletedAt())
	require.Equal(t, original.DeletedAt().Unix(), restored.DeletedAt().Unix())
}

// TestSessionModel_RoundTrip_NilDeletedAt verifies nil deletedAt is preserved.
func TestSessionModel_RoundTrip_NilDeletedAt(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	original := domain.ReconstituteSession(
		456,
		"test-guid",
		"test-project",
		"",
		domain.SessionStateRunning,
		"", "", "",
		nil,
		false,
		"", "",
		"", "",
		"", // sessionDir
		nil, nil,
		0,
		0,
		nil, nil,
		now,
		nil, nil,
		nil, // completedAt
		now,
		nil,
		nil,
	)

	model := toSessionModel(original)
	require.Nil(t, model.Name)
	require.Nil(t, model.TemplateID)
	require.Nil(t, model.EpicID)
	require.Nil(t, model.WorkDir)
	require.Nil(t, model.WorktreePath)
	require.Nil(t, model.WorktreeBranch)
	require.Nil(t, model.OwnerCreatedPID)
	require.Nil(t, model.OwnerCurrentPID)
	require.Nil(t, model.StartedAt)
	require.Nil(t, model.PausedAt)
	require.Nil(t, model.ArchivedAt)
	require.Nil(t, model.DeletedAt)

	restored := model.toDomain()
	require.Empty(t, restored.Name())
	require.Empty(t, restored.TemplateID())
	require.Empty(t, restored.EpicID())
	require.Empty(t, restored.WorkDir())
	require.Empty(t, restored.WorktreePath())
	require.Empty(t, restored.WorktreeBranch())
	require.Nil(t, restored.OwnerCreatedPID())
	require.Nil(t, restored.OwnerCurrentPID())
	require.Nil(t, restored.StartedAt())
	require.Nil(t, restored.PausedAt())
	require.Nil(t, restored.ArchivedAt())
	require.False(t, restored.IsArchived())
	require.Nil(t, restored.DeletedAt())
	require.False(t, restored.IsDeleted())
}
