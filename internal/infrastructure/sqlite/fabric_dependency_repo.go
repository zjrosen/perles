package sqlite

import (
	"database/sql"
	"fmt"

	domain "github.com/zjrosen/perles/internal/orchestration/fabric/domain"
	"github.com/zjrosen/perles/internal/orchestration/fabric/repository"
)

// fabricDependencyColumns is the ordered list of columns for fabric_dependencies queries.
const fabricDependencyColumns = `session_id, thread_id, depends_on_id, relation, created_at`

// SQLiteDependencyRepository implements DependencyRepository using SQLite storage.
// All operations are scoped to a single session to prevent cross-session leakage.
type SQLiteDependencyRepository struct {
	db        *sql.DB
	sessionID string
}

// NewSQLiteDependencyRepository creates a new SQLite-backed dependency repository
// scoped to the given session.
func NewSQLiteDependencyRepository(db *sql.DB, sessionID string) *SQLiteDependencyRepository {
	return &SQLiteDependencyRepository{db: db, sessionID: sessionID}
}

// Ensure SQLiteDependencyRepository implements DependencyRepository.
var _ repository.DependencyRepository = (*SQLiteDependencyRepository)(nil)

// scanFabricDependency scans a row into a fabricDependencyModel.
func scanFabricDependency(scanner interface{ Scan(...any) error }) (*fabricDependencyModel, error) {
	var m fabricDependencyModel
	err := scanner.Scan(
		&m.SessionID, &m.ThreadID, &m.DependsOnID, &m.Relation, &m.CreatedAt,
	)
	return &m, err
}

// Add creates a dependency edge.
// Uses INSERT OR IGNORE for idempotent behavior matching the memory implementation.
func (r *SQLiteDependencyRepository) Add(dep domain.Dependency) error {
	model := toFabricDependencyModel(r.sessionID, dep)

	_, err := r.db.Exec(
		`INSERT OR IGNORE INTO fabric_dependencies (`+fabricDependencyColumns+`)
		 VALUES (?, ?, ?, ?, ?)`,
		model.SessionID, model.ThreadID, model.DependsOnID, model.Relation, model.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to add dependency: %w", err)
	}

	return nil
}

// Remove deletes all dependency edges between two threads within this session.
// No error is returned if the dependency does not exist (matching memory behavior).
func (r *SQLiteDependencyRepository) Remove(threadID, dependsOnID string) error {
	_, err := r.db.Exec(
		`DELETE FROM fabric_dependencies WHERE session_id = ? AND thread_id = ? AND depends_on_id = ?`,
		r.sessionID, threadID, dependsOnID,
	)
	if err != nil {
		return fmt.Errorf("failed to remove dependency: %w", err)
	}

	return nil
}

// GetParents returns dependencies where threadID is the dependent (outgoing edges).
// If relation is nil, returns all parent relations.
func (r *SQLiteDependencyRepository) GetParents(threadID string, relation *domain.RelationType) ([]domain.Dependency, error) {
	query := `SELECT ` + fabricDependencyColumns + ` FROM fabric_dependencies WHERE session_id = ? AND thread_id = ?`
	args := []any{r.sessionID, threadID}

	if relation != nil {
		query += ` AND relation = ?`
		args = append(args, string(*relation))
	}

	return r.queryDependencies(query, args)
}

// GetChildren returns dependencies where threadID is depended upon (incoming edges).
// If relation is nil, returns all child relations.
func (r *SQLiteDependencyRepository) GetChildren(threadID string, relation *domain.RelationType) ([]domain.Dependency, error) {
	query := `SELECT ` + fabricDependencyColumns + ` FROM fabric_dependencies WHERE session_id = ? AND depends_on_id = ?`
	args := []any{r.sessionID, threadID}

	if relation != nil {
		query += ` AND relation = ?`
		args = append(args, string(*relation))
	}

	return r.queryDependencies(query, args)
}

// GetRoots returns thread IDs with no child_of dependency within this session.
// A root is a thread that appears in the dependency graph but has no child_of parent.
func (r *SQLiteDependencyRepository) GetRoots() ([]string, error) {
	// Collect all thread IDs that appear in the dependency graph,
	// then exclude those that have a child_of relation as the dependent.
	rows, err := r.db.Query(
		`SELECT DISTINCT id FROM (
			SELECT thread_id AS id FROM fabric_dependencies WHERE session_id = ?
			UNION
			SELECT depends_on_id AS id FROM fabric_dependencies WHERE session_id = ?
		)
		WHERE id NOT IN (
			SELECT thread_id FROM fabric_dependencies WHERE session_id = ? AND relation = 'child_of'
		)`,
		r.sessionID, r.sessionID, r.sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get roots: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var roots []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan root ID: %w", err)
		}
		roots = append(roots, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating root rows: %w", err)
	}

	return roots, nil
}

// queryDependencies executes a query and returns dependency results.
func (r *SQLiteDependencyRepository) queryDependencies(query string, args []any) ([]domain.Dependency, error) {
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query dependencies: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []domain.Dependency
	for rows.Next() {
		model, err := scanFabricDependency(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan dependency row: %w", err)
		}
		results = append(results, model.toDomain())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating dependency rows: %w", err)
	}

	return results, nil
}
