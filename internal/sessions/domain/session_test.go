package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSessionState_String(t *testing.T) {
	tests := []struct {
		state    SessionState
		expected string
	}{
		{SessionStatePending, "pending"},
		{SessionStateRunning, "running"},
		{SessionStatePaused, "paused"},
		{SessionStateCompleted, "completed"},
		{SessionStateFailed, "failed"},
		{SessionStateTimedOut, "timed_out"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.state.String())
		})
	}
}

func TestSessionState_IsValid(t *testing.T) {
	tests := []struct {
		state   SessionState
		isValid bool
	}{
		{SessionStatePending, true},
		{SessionStateRunning, true},
		{SessionStatePaused, true},
		{SessionStateCompleted, true},
		{SessionStateFailed, true},
		{SessionStateTimedOut, true},
		{SessionState("invalid"), false},
		{SessionState(""), false},
		{SessionState("RUNNING"), false}, // case sensitive
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			require.Equal(t, tt.isValid, tt.state.IsValid())
		})
	}
}

func TestNewSession(t *testing.T) {
	before := time.Now()
	session := NewSession("test-guid-123", "my-project", SessionStateRunning)
	after := time.Now()

	// Verify all fields are set correctly
	require.Equal(t, int64(0), session.ID(), "ID should be 0 for new sessions")
	require.Equal(t, "test-guid-123", session.GUID())
	require.Equal(t, "my-project", session.Project())
	require.Equal(t, SessionStateRunning, session.State())

	// Verify timestamps are within the expected range
	require.False(t, session.CreatedAt().Before(before), "createdAt should be >= before")
	require.False(t, session.CreatedAt().After(after), "createdAt should be <= after")
	require.Equal(t, session.CreatedAt(), session.UpdatedAt(), "createdAt and updatedAt should match for new session")

	// Verify not deleted
	require.Nil(t, session.DeletedAt())
	require.False(t, session.IsDeleted())
}

func TestNewSession_DifferentStates(t *testing.T) {
	states := []SessionState{
		SessionStatePending,
		SessionStateRunning,
		SessionStatePaused,
		SessionStateCompleted,
		SessionStateFailed,
		SessionStateTimedOut,
	}

	for _, state := range states {
		t.Run(state.String(), func(t *testing.T) {
			session := NewSession("guid", "project", state)
			require.Equal(t, state, session.State())
		})
	}
}

func TestReconstituteSession(t *testing.T) {
	createdAt := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	startedAt := time.Date(2026, 1, 10, 12, 5, 0, 0, time.UTC)
	pausedAt := time.Date(2026, 1, 12, 8, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 1, 15, 14, 30, 0, 0, time.UTC)
	archivedAt := time.Date(2026, 1, 18, 10, 0, 0, 0, time.UTC)
	deletedAt := time.Date(2026, 1, 20, 9, 0, 0, 0, time.UTC)
	ownerCreatedPID := 12345
	ownerCurrentPID := 67890

	session := ReconstituteSession(
		42,
		"reconstituted-guid",
		"reconstituted-project",
		"Test Session",
		SessionStateCompleted,
		"template-abc",
		"epic-123",
		"/path/to/workdir",
		nil,
		false,
		"", "",
		"/path/to/worktree",
		"feature/branch",
		"", // sessionDir
		&ownerCreatedPID,
		&ownerCurrentPID,
		0,
		0,
		nil, nil,
		createdAt,
		&startedAt,
		&pausedAt,
		nil, // completedAt
		updatedAt,
		&archivedAt,
		&deletedAt,
	)

	// Verify all values are preserved exactly
	require.Equal(t, int64(42), session.ID())
	require.Equal(t, "reconstituted-guid", session.GUID())
	require.Equal(t, "reconstituted-project", session.Project())
	require.Equal(t, "Test Session", session.Name())
	require.Equal(t, SessionStateCompleted, session.State())
	require.Equal(t, "template-abc", session.TemplateID())
	require.Equal(t, "epic-123", session.EpicID())
	require.Equal(t, "/path/to/workdir", session.WorkDir())
	require.Equal(t, "/path/to/worktree", session.WorktreePath())
	require.Equal(t, "feature/branch", session.WorktreeBranch())
	require.NotNil(t, session.OwnerCreatedPID())
	require.Equal(t, 12345, *session.OwnerCreatedPID())
	require.NotNil(t, session.OwnerCurrentPID())
	require.Equal(t, 67890, *session.OwnerCurrentPID())
	require.Equal(t, createdAt, session.CreatedAt())
	require.NotNil(t, session.StartedAt())
	require.Equal(t, startedAt, *session.StartedAt())
	require.NotNil(t, session.PausedAt())
	require.Equal(t, pausedAt, *session.PausedAt())
	require.Equal(t, updatedAt, session.UpdatedAt())
	require.NotNil(t, session.ArchivedAt())
	require.Equal(t, archivedAt, *session.ArchivedAt())
	require.True(t, session.IsArchived())
	require.NotNil(t, session.DeletedAt())
	require.Equal(t, deletedAt, *session.DeletedAt())
	require.True(t, session.IsDeleted())
}

func TestReconstituteSession_NilDeletedAt(t *testing.T) {
	createdAt := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 1, 15, 14, 30, 0, 0, time.UTC)

	session := ReconstituteSession(
		99,
		"active-guid",
		"active-project",
		"",
		SessionStateRunning,
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
		createdAt,
		nil, nil,
		nil, // completedAt
		updatedAt,
		nil,
		nil,
	)

	require.Nil(t, session.OwnerCreatedPID())
	require.Nil(t, session.OwnerCurrentPID())
	require.Nil(t, session.ArchivedAt())
	require.False(t, session.IsArchived())
	require.Nil(t, session.DeletedAt())
	require.False(t, session.IsDeleted())
}

func TestSession_MarkCompleted(t *testing.T) {
	session := NewSession("guid", "project", SessionStateRunning)
	originalCreatedAt := session.CreatedAt()

	// Small delay to ensure updatedAt changes
	time.Sleep(time.Millisecond)

	before := time.Now()
	session.MarkCompleted()
	after := time.Now()

	require.Equal(t, SessionStateCompleted, session.State())
	require.Equal(t, originalCreatedAt, session.CreatedAt(), "createdAt should not change")
	require.False(t, session.UpdatedAt().Before(before), "updatedAt should be >= before")
	require.False(t, session.UpdatedAt().After(after), "updatedAt should be <= after")
}

func TestSession_MarkFailed(t *testing.T) {
	session := NewSession("guid", "project", SessionStateRunning)
	originalCreatedAt := session.CreatedAt()

	// Small delay to ensure updatedAt changes
	time.Sleep(time.Millisecond)

	before := time.Now()
	session.MarkFailed()
	after := time.Now()

	require.Equal(t, SessionStateFailed, session.State())
	require.Equal(t, originalCreatedAt, session.CreatedAt(), "createdAt should not change")
	require.False(t, session.UpdatedAt().Before(before), "updatedAt should be >= before")
	require.False(t, session.UpdatedAt().After(after), "updatedAt should be <= after")
}

func TestSession_MarkTimedOut(t *testing.T) {
	session := NewSession("guid", "project", SessionStateRunning)
	originalCreatedAt := session.CreatedAt()

	// Small delay to ensure updatedAt changes
	time.Sleep(time.Millisecond)

	before := time.Now()
	session.MarkTimedOut()
	after := time.Now()

	require.Equal(t, SessionStateTimedOut, session.State())
	require.Equal(t, originalCreatedAt, session.CreatedAt(), "createdAt should not change")
	require.False(t, session.UpdatedAt().Before(before), "updatedAt should be >= before")
	require.False(t, session.UpdatedAt().After(after), "updatedAt should be <= after")
}

func TestSession_SoftDelete(t *testing.T) {
	session := NewSession("guid", "project", SessionStateRunning)
	require.False(t, session.IsDeleted())
	require.Nil(t, session.DeletedAt())

	// Small delay to ensure timestamps change
	time.Sleep(time.Millisecond)

	before := time.Now()
	session.SoftDelete()
	after := time.Now()

	require.True(t, session.IsDeleted())
	require.NotNil(t, session.DeletedAt())

	// Verify deletedAt is within expected range
	require.False(t, session.DeletedAt().Before(before), "deletedAt should be >= before")
	require.False(t, session.DeletedAt().After(after), "deletedAt should be <= after")

	// Verify updatedAt was also updated
	require.False(t, session.UpdatedAt().Before(before), "updatedAt should be >= before")
	require.False(t, session.UpdatedAt().After(after), "updatedAt should be <= after")
}

func TestSession_IsDeleted(t *testing.T) {
	t.Run("not deleted when deletedAt is nil", func(t *testing.T) {
		session := NewSession("guid", "project", SessionStateRunning)
		require.False(t, session.IsDeleted())
	})

	t.Run("deleted when deletedAt is set", func(t *testing.T) {
		deletedAt := time.Now()
		session := ReconstituteSession(
			1, "guid", "project", "", SessionStateCompleted, "", "", "",
			nil, false, "", "", "", "",
			"", // sessionDir
			nil, nil, 0, 0, nil, nil,
			time.Now(), nil, nil, nil, time.Now(), nil, &deletedAt,
		)
		require.True(t, session.IsDeleted())
	})
}

func TestSession_SetID(t *testing.T) {
	session := NewSession("guid", "project", SessionStateRunning)
	require.Equal(t, int64(0), session.ID())

	session.SetID(123)
	require.Equal(t, int64(123), session.ID())

	// Can be updated again
	session.SetID(456)
	require.Equal(t, int64(456), session.ID())
}

func TestSession_Getters(t *testing.T) {
	createdAt := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 1, 15, 14, 30, 0, 0, time.UTC)

	session := ReconstituteSession(
		100,
		"test-guid",
		"test-project",
		"Test Name",
		SessionStateFailed,
		"template-xyz",
		"epic-456",
		"/work/dir",
		nil,
		false,
		"", "",
		"/worktree/path",
		"main",
		"", // sessionDir
		nil, nil,
		0,
		0,
		nil, nil,
		createdAt,
		nil, nil,
		nil, // completedAt
		updatedAt,
		nil,
		nil,
	)

	require.Equal(t, int64(100), session.ID())
	require.Equal(t, "test-guid", session.GUID())
	require.Equal(t, "test-project", session.Project())
	require.Equal(t, "Test Name", session.Name())
	require.Equal(t, SessionStateFailed, session.State())
	require.Equal(t, "template-xyz", session.TemplateID())
	require.Equal(t, "epic-456", session.EpicID())
	require.Equal(t, "/work/dir", session.WorkDir())
	require.Equal(t, "/worktree/path", session.WorktreePath())
	require.Equal(t, "main", session.WorktreeBranch())
	require.Equal(t, createdAt, session.CreatedAt())
	require.Nil(t, session.StartedAt())
	require.Nil(t, session.PausedAt())
	require.Equal(t, updatedAt, session.UpdatedAt())
	require.Nil(t, session.DeletedAt())
	require.False(t, session.IsDeleted())
}

func TestSessionState_Constants(t *testing.T) {
	// Verify the exact string values
	require.Equal(t, SessionState("pending"), SessionStatePending)
	require.Equal(t, SessionState("running"), SessionStateRunning)
	require.Equal(t, SessionState("paused"), SessionStatePaused)
	require.Equal(t, SessionState("completed"), SessionStateCompleted)
	require.Equal(t, SessionState("failed"), SessionStateFailed)
	require.Equal(t, SessionState("timed_out"), SessionStateTimedOut)
}
