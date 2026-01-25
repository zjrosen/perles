package domain

// ListFilter provides filtering options for listing sessions.
type ListFilter struct {
	// State filters sessions by their current state.
	// If empty, all states are included.
	State SessionState

	// OwnerPID filters to sessions owned by a specific process ID.
	// If nil, no PID filtering is applied.
	OwnerPID *int

	// Limit restricts the number of sessions returned.
	// If 0, no limit is applied.
	Limit int

	// IncludeDeleted includes soft-deleted sessions in results.
	// By default, deleted sessions are excluded.
	IncludeDeleted bool

	// IncludeArchived includes archived sessions in results.
	// By default, archived sessions are excluded.
	IncludeArchived bool
}

// SessionRepository defines the persistence interface for Session entities.
// Implementations may use SQLite, in-memory storage, or other backends.
type SessionRepository interface {
	// Save persists a session to the repository.
	// For new sessions (ID == 0), this creates a new record and sets the ID.
	// For existing sessions (ID > 0), this updates the existing record.
	Save(session *Session) error

	// FindByGUID retrieves a session by its GUID within a specific project.
	// Returns SessionNotFoundError if no matching session exists.
	// Soft-deleted sessions are not returned.
	FindByGUID(project, guid string) (*Session, error)

	// FindByID retrieves a session by its internal database ID.
	// Returns SessionNotFoundError if no matching session exists.
	// Soft-deleted sessions are not returned.
	FindByID(id int64) (*Session, error)

	// GetActiveSession retrieves the currently running session for a project.
	// Returns NoActiveSessionError if no session is in the running state.
	GetActiveSession(project string) (*Session, error)

	// Delete performs a soft delete on a session by setting its deletedAt timestamp.
	// Returns SessionNotFoundError if no matching session exists.
	Delete(project, guid string) error

	// DeleteAllForProject performs a hard delete of all sessions for a project.
	// This permanently removes all session records for the specified project.
	DeleteAllForProject(project string) error

	// ListWithFilter retrieves sessions for a project matching the given filter criteria.
	// Results are ordered by created_at descending (newest first).
	ListWithFilter(project string, filter ListFilter) ([]*Session, error)

	// Close releases any resources held by the repository.
	Close() error
}
