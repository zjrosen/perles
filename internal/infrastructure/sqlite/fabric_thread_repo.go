package sqlite

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	domain "github.com/zjrosen/perles/internal/orchestration/fabric/domain"
	"github.com/zjrosen/perles/internal/orchestration/fabric/repository"
)

// fabricThreadColumns is the ordered list of columns for fabric_threads queries.
const fabricThreadColumns = `session_id, id, type, created_at, created_by,
	content, kind, slug, title, purpose,
	name, media_type, size_bytes, storage_uri, sha256,
	mentions_json, participants_json, meta_json,
	seq, archived_at`

// SQLiteThreadRepository implements ThreadRepository using SQLite storage.
// All operations are scoped to a single session to prevent cross-session leakage.
type SQLiteThreadRepository struct {
	db        *sql.DB
	sessionID string
}

// NewSQLiteThreadRepository creates a new SQLite-backed thread repository
// scoped to the given session.
func NewSQLiteThreadRepository(db *sql.DB, sessionID string) *SQLiteThreadRepository {
	return &SQLiteThreadRepository{db: db, sessionID: sessionID}
}

// Ensure SQLiteThreadRepository implements ThreadRepository.
var _ repository.ThreadRepository = (*SQLiteThreadRepository)(nil)

// scanFabricThread scans a row into a fabricThreadModel.
func scanFabricThread(scanner interface{ Scan(...any) error }) (*fabricThreadModel, error) {
	var m fabricThreadModel
	err := scanner.Scan(
		&m.SessionID, &m.ID, &m.Type, &m.CreatedAt, &m.CreatedBy,
		&m.Content, &m.Kind, &m.Slug, &m.Title, &m.Purpose,
		&m.Name, &m.MediaType, &m.SizeBytes, &m.StorageURI, &m.Sha256,
		&m.MentionsJSON, &m.ParticipantsJSON, &m.MetaJSON,
		&m.Seq, &m.ArchivedAt,
	)
	return &m, err
}

// Create adds a new thread to the database.
// ID and Seq are assigned automatically if empty/zero.
func (r *SQLiteThreadRepository) Create(thread domain.Thread) (*domain.Thread, error) {
	if thread.ID == "" {
		thread.ID = uuid.New().String()
	}

	if thread.CreatedAt.IsZero() {
		thread.CreatedAt = time.Now()
	}

	if thread.Seq == 0 {
		seq, err := r.nextSeq()
		if err != nil {
			return nil, fmt.Errorf("failed to assign sequence: %w", err)
		}
		thread.Seq = seq
	}

	model := toFabricThreadModel(r.sessionID, &thread)

	_, err := r.db.Exec(
		`INSERT INTO fabric_threads (`+fabricThreadColumns+`)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		model.SessionID, model.ID, model.Type, model.CreatedAt, model.CreatedBy,
		model.Content, model.Kind, model.Slug, model.Title, model.Purpose,
		model.Name, model.MediaType, model.SizeBytes, model.StorageURI, model.Sha256,
		model.MentionsJSON, model.ParticipantsJSON, model.MetaJSON,
		model.Seq, model.ArchivedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert thread: %w", err)
	}

	return &thread, nil
}

// Get retrieves a thread by ID within this session.
// Returns an error if the thread is not found (matching memory behavior).
func (r *SQLiteThreadRepository) Get(id string) (*domain.Thread, error) {
	row := r.db.QueryRow(
		`SELECT `+fabricThreadColumns+` FROM fabric_threads WHERE session_id = ? AND id = ?`,
		r.sessionID, id,
	)

	model, err := scanFabricThread(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("thread not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get thread: %w", err)
	}

	return model.toDomain(), nil
}

// GetBySlug finds a channel thread by its slug within this session.
// Returns an error if the slug is not found (matching memory behavior).
func (r *SQLiteThreadRepository) GetBySlug(slug string) (*domain.Thread, error) {
	row := r.db.QueryRow(
		`SELECT `+fabricThreadColumns+` FROM fabric_threads WHERE session_id = ? AND slug = ?`,
		r.sessionID, slug,
	)

	model, err := scanFabricThread(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("channel not found: %s", slug)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get thread by slug: %w", err)
	}

	return model.toDomain(), nil
}

// List returns threads matching the filter criteria within this session.
// Results are ordered by seq ASC to match memory implementation behavior.
func (r *SQLiteThreadRepository) List(opts repository.ListOptions) ([]domain.Thread, error) {
	query := `SELECT ` + fabricThreadColumns + ` FROM fabric_threads WHERE session_id = ?`
	args := []any{r.sessionID}

	if opts.Type != nil {
		query += ` AND type = ?`
		args = append(args, string(*opts.Type))
	}

	if opts.AfterSeq > 0 {
		query += ` AND seq > ?`
		args = append(args, opts.AfterSeq)
	}

	if opts.CreatedBy != nil {
		query += ` AND created_by = ?`
		args = append(args, *opts.CreatedBy)
	}

	if opts.HasMention != nil {
		// JSON contains check: mentions_json contains the quoted agent ID
		// Use LIKE with the JSON-encoded string value for substring match
		query += ` AND mentions_json LIKE ?`
		args = append(args, `%"`+*opts.HasMention+`"%`)
	}

	if opts.ChannelID != nil {
		// Filter to messages that are children of the given channel
		query += ` AND id IN (SELECT thread_id FROM fabric_dependencies WHERE session_id = ? AND depends_on_id = ? AND relation = 'child_of')`
		args = append(args, r.sessionID, *opts.ChannelID)
	}

	query += ` ORDER BY seq ASC`

	if opts.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, opts.Limit)
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list threads: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []domain.Thread
	for rows.Next() {
		model, err := scanFabricThread(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan thread row: %w", err)
		}
		results = append(results, *model.toDomain())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating thread rows: %w", err)
	}

	return results, nil
}

// Update modifies an existing thread in the database.
// Preserves the original Seq and CreatedAt values (matching memory behavior).
func (r *SQLiteThreadRepository) Update(thread domain.Thread) (*domain.Thread, error) {
	// First fetch the existing thread to preserve Seq and CreatedAt
	existing, err := r.Get(thread.ID)
	if err != nil {
		return nil, err
	}

	thread.Seq = existing.Seq
	thread.CreatedAt = existing.CreatedAt

	model := toFabricThreadModel(r.sessionID, &thread)

	result, err := r.db.Exec(
		`UPDATE fabric_threads SET
			type = ?, created_at = ?, created_by = ?,
			content = ?, kind = ?, slug = ?, title = ?, purpose = ?,
			name = ?, media_type = ?, size_bytes = ?, storage_uri = ?, sha256 = ?,
			mentions_json = ?, participants_json = ?, meta_json = ?,
			seq = ?, archived_at = ?
		 WHERE session_id = ? AND id = ?`,
		model.Type, model.CreatedAt, model.CreatedBy,
		model.Content, model.Kind, model.Slug, model.Title, model.Purpose,
		model.Name, model.MediaType, model.SizeBytes, model.StorageURI, model.Sha256,
		model.MentionsJSON, model.ParticipantsJSON, model.MetaJSON,
		model.Seq, model.ArchivedAt,
		r.sessionID, model.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update thread: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return nil, fmt.Errorf("thread not found: %s", thread.ID)
	}

	return &thread, nil
}

// Archive soft-deletes a thread by setting ArchivedAt to the current time.
func (r *SQLiteThreadRepository) Archive(id string) error {
	now := time.Now().Unix()
	result, err := r.db.Exec(
		`UPDATE fabric_threads SET archived_at = ? WHERE session_id = ? AND id = ?`,
		now, r.sessionID, id,
	)
	if err != nil {
		return fmt.Errorf("failed to archive thread: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("thread not found: %s", id)
	}

	return nil
}

// nextSeq returns the next sequence number for this session.
// Uses MAX(seq)+1 to ensure monotonically increasing sequences.
func (r *SQLiteThreadRepository) nextSeq() (int64, error) {
	var maxSeq sql.NullInt64
	err := r.db.QueryRow(
		`SELECT MAX(seq) FROM fabric_threads WHERE session_id = ?`,
		r.sessionID,
	).Scan(&maxSeq)
	if err != nil {
		return 0, fmt.Errorf("failed to query max seq: %w", err)
	}

	if maxSeq.Valid {
		return maxSeq.Int64 + 1, nil
	}
	return 1, nil
}
