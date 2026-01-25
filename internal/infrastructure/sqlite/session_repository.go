package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/zjrosen/perles/internal/sessions/domain"
)

// sessionColumns is the list of columns to select for session queries.
const sessionColumns = `id, guid, project, name, state, template_id, epic_id, work_dir, labels, 
	worktree_enabled, worktree_base_branch, worktree_branch_name, worktree_path, worktree_branch, session_dir,
	owner_created_pid, owner_current_pid, tokens_used, active_workers, last_heartbeat_at, last_progress_at,
	created_at, started_at, paused_at, completed_at, updated_at, archived_at, deleted_at`

// sessionRepository implements domain.SessionRepository using SQLite.
type sessionRepository struct {
	db *sql.DB
}

// newSessionRepository creates a new sessionRepository instance.
func newSessionRepository(db *sql.DB) *sessionRepository {
	return &sessionRepository{db: db}
}

// Ensure sessionRepository implements domain.SessionRepository.
var _ domain.SessionRepository = (*sessionRepository)(nil)

// scanSession scans a row into a SessionModel.
func scanSession(scanner interface{ Scan(...any) error }) (*SessionModel, error) {
	var model SessionModel
	err := scanner.Scan(
		&model.ID, &model.GUID, &model.Project, &model.Name, &model.State,
		&model.TemplateID, &model.EpicID, &model.WorkDir, &model.Labels,
		&model.WorktreeEnabled, &model.WorktreeBaseBranch, &model.WorktreeBranchName,
		&model.WorktreePath, &model.WorktreeBranch, &model.SessionDir,
		&model.OwnerCreatedPID, &model.OwnerCurrentPID,
		&model.TokensUsed, &model.ActiveWorkers,
		&model.LastHeartbeatAt, &model.LastProgressAt,
		&model.CreatedAt, &model.StartedAt, &model.PausedAt, &model.CompletedAt, &model.UpdatedAt,
		&model.ArchivedAt, &model.DeletedAt,
	)
	return &model, err
}

// Save persists a session to the database.
// For new sessions (ID == 0), inserts a new row and sets the session ID.
// For existing sessions (ID > 0), updates the existing row.
func (r *sessionRepository) Save(session *domain.Session) error {
	model := toSessionModel(session)

	if session.ID() == 0 {
		// Insert new session
		result, err := r.db.Exec(
			`INSERT INTO sessions (
				guid, project, name, state, template_id, epic_id, work_dir, labels,
				worktree_enabled, worktree_base_branch, worktree_branch_name, worktree_path, worktree_branch, session_dir,
				owner_created_pid, owner_current_pid, tokens_used, active_workers, last_heartbeat_at, last_progress_at,
				created_at, started_at, paused_at, completed_at, updated_at, archived_at, deleted_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			model.GUID, model.Project, model.Name, model.State, model.TemplateID, model.EpicID,
			model.WorkDir, model.Labels,
			model.WorktreeEnabled, model.WorktreeBaseBranch, model.WorktreeBranchName,
			model.WorktreePath, model.WorktreeBranch, model.SessionDir,
			model.OwnerCreatedPID, model.OwnerCurrentPID,
			model.TokensUsed, model.ActiveWorkers, model.LastHeartbeatAt, model.LastProgressAt,
			model.CreatedAt, model.StartedAt, model.PausedAt, model.CompletedAt, model.UpdatedAt, model.ArchivedAt, model.DeletedAt,
		)
		if err != nil {
			return fmt.Errorf("failed to insert session: %w", err)
		}
		id, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("failed to get last insert id: %w", err)
		}
		session.SetID(id)
		return nil
	}

	// Update existing session
	_, err := r.db.Exec(
		`UPDATE sessions SET 
			name = ?, state = ?, template_id = ?, epic_id = ?, work_dir = ?, labels = ?,
			worktree_enabled = ?, worktree_base_branch = ?, worktree_branch_name = ?, worktree_path = ?, worktree_branch = ?, session_dir = ?,
			owner_created_pid = ?, owner_current_pid = ?, tokens_used = ?, active_workers = ?, 
			last_heartbeat_at = ?, last_progress_at = ?,
			started_at = ?, paused_at = ?, completed_at = ?, updated_at = ?, archived_at = ?, deleted_at = ?
		WHERE id = ?`,
		model.Name, model.State, model.TemplateID, model.EpicID, model.WorkDir, model.Labels,
		model.WorktreeEnabled, model.WorktreeBaseBranch, model.WorktreeBranchName, model.WorktreePath, model.WorktreeBranch, model.SessionDir,
		model.OwnerCreatedPID, model.OwnerCurrentPID, model.TokensUsed, model.ActiveWorkers,
		model.LastHeartbeatAt, model.LastProgressAt,
		model.StartedAt, model.PausedAt, model.CompletedAt, model.UpdatedAt, model.ArchivedAt, model.DeletedAt,
		model.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}
	return nil
}

// FindByGUID retrieves a session by its GUID within a specific project.
// Returns SessionNotFoundError if no matching session exists.
// Soft-deleted sessions are not returned.
func (r *sessionRepository) FindByGUID(project, guid string) (*domain.Session, error) {
	row := r.db.QueryRow(
		`SELECT `+sessionColumns+` FROM sessions WHERE project = ? AND guid = ? AND deleted_at IS NULL`,
		project, guid,
	)
	model, err := scanSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &domain.SessionNotFoundError{GUID: guid, Project: project}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find session by guid: %w", err)
	}
	return model.toDomain(), nil
}

// FindByID retrieves a session by its internal database ID.
// Returns SessionNotFoundError if no matching session exists.
// Soft-deleted sessions are not returned.
// Note: This method does not filter by project as it's used for internal lookups.
func (r *sessionRepository) FindByID(id int64) (*domain.Session, error) {
	row := r.db.QueryRow(
		`SELECT `+sessionColumns+` FROM sessions WHERE id = ? AND deleted_at IS NULL`,
		id,
	)
	model, err := scanSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &domain.SessionNotFoundError{GUID: "", Project: ""}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find session by id: %w", err)
	}
	return model.toDomain(), nil
}

// GetActiveSession retrieves the currently running session for a project.
// Returns NoActiveSessionError if no session is in the running state.
func (r *sessionRepository) GetActiveSession(project string) (*domain.Session, error) {
	row := r.db.QueryRow(
		`SELECT `+sessionColumns+` FROM sessions WHERE project = ? AND state = 'running' AND deleted_at IS NULL`,
		project,
	)
	model, err := scanSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &domain.NoActiveSessionError{Project: project}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get active session: %w", err)
	}
	return model.toDomain(), nil
}

// Delete performs a soft delete on a session by setting its deletedAt timestamp.
// Returns SessionNotFoundError if no matching session exists.
func (r *sessionRepository) Delete(project, guid string) error {
	now := time.Now().Unix()
	result, err := r.db.Exec(
		`UPDATE sessions SET deleted_at = ?, updated_at = ?
		 WHERE project = ? AND guid = ? AND deleted_at IS NULL`,
		now, now, project, guid,
	)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return &domain.SessionNotFoundError{GUID: guid, Project: project}
	}
	return nil
}

// DeleteAllForProject performs a hard delete of all sessions for a project.
// This permanently removes all session records for the specified project.
func (r *sessionRepository) DeleteAllForProject(project string) error {
	_, err := r.db.Exec(
		`DELETE FROM sessions WHERE project = ?`,
		project,
	)
	if err != nil {
		return fmt.Errorf("failed to delete all sessions for project: %w", err)
	}
	return nil
}

// ListWithFilter retrieves sessions for a project matching the given filter criteria.
// Results are ordered by created_at descending (newest first).
func (r *sessionRepository) ListWithFilter(project string, filter domain.ListFilter) ([]*domain.Session, error) {
	query := `SELECT ` + sessionColumns + ` FROM sessions WHERE project = ?`
	args := []any{project}

	// Add state filter if specified
	if filter.State != "" {
		query += ` AND state = ?`
		args = append(args, string(filter.State))
	}

	// Add owner PID filter if specified
	if filter.OwnerPID != nil {
		query += ` AND owner_current_pid = ?`
		args = append(args, *filter.OwnerPID)
	}

	// Filter out deleted unless IncludeDeleted is true
	if !filter.IncludeDeleted {
		query += ` AND deleted_at IS NULL`
	}

	// Filter out archived unless IncludeArchived is true
	if !filter.IncludeArchived {
		query += ` AND archived_at IS NULL`
	}

	// Order by created_at descending (newest first)
	query += ` ORDER BY created_at DESC`

	// Add limit if specified
	if filter.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, filter.Limit)
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var sessions []*domain.Session
	for rows.Next() {
		model, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan session row: %w", err)
		}
		sessions = append(sessions, model.toDomain())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating session rows: %w", err)
	}

	return sessions, nil
}

// Close releases any resources held by the repository.
// This is a no-op because the connection is owned by the DB struct.
func (r *sessionRepository) Close() error {
	// No-op: connection is owned by DB struct
	return nil
}
