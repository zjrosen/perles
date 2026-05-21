package beadsrust

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/task"

	_ "github.com/ncruces/go-sqlite3/driver"
)

// issueColumns is the SELECT column list for queries against the issues table.
const issueColumns = `
	i.id, i.title, i.description, i.design, i.acceptance_criteria, i.notes,
	i.status, i.priority, i.issue_type, i.assignee, i.owner,
	i.created_at, i.created_by, i.updated_at, i.closed_at, i.close_reason,
	i.defer_until, i.source_repo, i.external_ref, i.pinned, i.ephemeral, i.is_template
`

// softDeleteFilter excludes tombstoned and soft-deleted issues.
const softDeleteFilter = "i.status NOT IN ('deleted', 'tombstone') AND i.deleted_at IS NULL"

// SQLiteReader provides read-only access to the beads_rust SQLite database.
// All read operations (ShowIssue, GetComments, queries) go through direct SQL
// for performance. Write operations remain on the br CLI executor.
type SQLiteReader struct {
	db     *sql.DB
	dbPath string
}

// NewSQLiteReader opens the beads_rust database in read-only mode.
func NewSQLiteReader(dataDir string) (*SQLiteReader, error) {
	dbPath := filepath.Join(dataDir, "beads.db")
	log.Debug(log.CatDB, "Opening beads_rust database", "path", dbPath)

	// Read-only mode: all writes go through the br CLI.
	// WAL mode is auto-detected from the file header (set by br).
	// No _pragma needed: the ncruces driver applies a default 1-minute
	// busy_timeout when no _pragma parameters are present.
	db, err := sql.Open("sqlite3", "file:"+dbPath+"?mode=ro")
	if err != nil {
		log.ErrorErr(log.CatDB, "Failed to open beads_rust database", err, "path", dbPath)
		return nil, fmt.Errorf("open beads_rust db: %w", err)
	}
	if err := db.PingContext(context.Background()); err != nil {
		log.ErrorErr(log.CatDB, "Failed to ping beads_rust database", err, "path", dbPath)
		return nil, fmt.Errorf("ping beads_rust db: %w", err)
	}

	// Disable connection pooling so that every query opens a fresh connection
	// with an up-to-date WAL snapshot. The br CLI writes via WAL mode, and
	// pooled connections cache stale WAL index state, missing recent writes.
	// Note: SetConnMaxIdleTime(0) is a no-op (disables idle cleanup);
	// SetMaxIdleConns(0) is what actually prevents connection reuse.
	db.SetMaxIdleConns(0)

	log.Info(log.CatDB, "Connected to beads_rust database", "path", dbPath)
	return &SQLiteReader{db: db, dbPath: dbPath}, nil
}

// Close closes the database connection.
func (r *SQLiteReader) Close() error {
	return r.db.Close()
}

// DB returns the underlying database connection for use by the BQL executor.
func (r *SQLiteReader) DB() *sql.DB {
	return r.db
}

// DBPath returns the path to the beads.db file.
func (r *SQLiteReader) DBPath() string {
	return r.dbPath
}

// ShowIssue loads a single issue by ID with all related data (labels,
// dependencies, comments).
func (r *SQLiteReader) ShowIssue(issueID string) (*task.Issue, error) {
	start := time.Now()
	defer func() {
		log.Debug(log.CatDB, "beads_rust ShowIssue completed", "issueID", issueID, "duration", time.Since(start))
	}()

	// NOT INDEXED: bypass corrupt sqlite_autoindex_issues_1 created by frankensqlite.
	// Forces a table scan instead of using the broken autoindex on issues.id.
	query := "SELECT" + issueColumns + "FROM issues i NOT INDEXED WHERE i.id = ? AND " + softDeleteFilter
	row := r.db.QueryRowContext(context.Background(), query, issueID)

	issue, err := scanIssue(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("issue not found: %s", issueID)
		}
		return nil, fmt.Errorf("query issue %s: %w", issueID, err)
	}

	// Batch load related data for single issue.
	ids := []string{issue.ID}
	if err := r.loadLabels(ids, map[string]*task.Issue{issue.ID: issue}); err != nil {
		return nil, err
	}
	if err := r.loadDependencies(ids, map[string]*task.Issue{issue.ID: issue}); err != nil {
		return nil, err
	}
	if err := r.loadComments(ids, map[string]*task.Issue{issue.ID: issue}); err != nil {
		return nil, err
	}

	return issue, nil
}

// GetComments returns all comments for an issue.
func (r *SQLiteReader) GetComments(issueID string) ([]task.Comment, error) {
	start := time.Now()
	defer func() {
		log.Debug(log.CatDB, "beads_rust GetComments completed", "issueID", issueID, "duration", time.Since(start))
	}()

	rows, err := r.db.QueryContext(context.Background(),
		"SELECT id, author, text, created_at FROM comments WHERE issue_id = ? ORDER BY created_at ASC",
		issueID,
	)
	if err != nil {
		return nil, fmt.Errorf("query comments for %s: %w", issueID, err)
	}
	defer func() { _ = rows.Close() }()

	var comments []task.Comment
	for rows.Next() {
		var c task.Comment
		var createdAt string
		if err := rows.Scan(&c.ID, &c.Author, &c.Text, &createdAt); err != nil {
			return nil, fmt.Errorf("scan comment: %w", err)
		}
		c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		if c.CreatedAt.IsZero() {
			c.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		}
		comments = append(comments, c)
	}
	return comments, rows.Err()
}

// scanIssue scans a single issue from a *sql.Row.
func scanIssue(row *sql.Row) (*task.Issue, error) {
	var issue task.Issue
	var (
		status, issueType, assignee, owner         string
		createdBy, closeReason, sourceRepo         string
		externalRef                                string
		priority, pinned, ephemeral, isTemplate    int
		createdAt, updatedAt, closedAtStr          string
		description, design, acceptCriteria, notes sql.NullString
		assigneeN, ownerN                          sql.NullString
		createdByN, closeReasonN                   sql.NullString
		sourceRepoN, externalRefN                  sql.NullString
		closedAtN, deferUntilN                     sql.NullString
	)

	err := row.Scan(
		&issue.ID, &issue.TitleText, &description, &design, &acceptCriteria, &notes,
		&status, &priority, &issueType, &assigneeN, &ownerN,
		&createdAt, &createdByN, &updatedAt, &closedAtN, &closeReasonN,
		&deferUntilN, &sourceRepoN, &externalRefN, &pinned, &ephemeral, &isTemplate,
	)
	if err != nil {
		return nil, err
	}

	issue.DescriptionText = nullStr(description)
	issue.Design = nullStr(design)
	issue.AcceptanceCriteria = nullStr(acceptCriteria)
	issue.Notes = nullStr(notes)
	assignee = nullStr(assigneeN)
	owner = nullStr(ownerN)
	createdBy = nullStr(createdByN)
	closeReason = nullStr(closeReasonN)
	sourceRepo = nullStr(sourceRepoN)
	externalRef = nullStr(externalRefN)
	closedAtStr = nullStr(closedAtN)

	issue.Status = task.Status(status)
	issue.Priority = task.Priority(priority)
	issue.Type = task.IssueType(issueType)
	issue.Assignee = assignee
	issue.CloseReason = closeReason
	issue.CreatedAt = parseTime(createdAt)
	issue.UpdatedAt = parseTime(updatedAt)
	if closedAtStr != "" {
		issue.ClosedAt = parseTime(closedAtStr)
	}
	if deferUntilStr := nullStr(deferUntilN); deferUntilStr != "" {
		issue.DeferUntil = parseTime(deferUntilStr)
	}

	// Extensions for beads_rust-specific fields.
	ext := make(map[string]any)
	if owner != "" {
		ext["owner"] = owner
	}
	if createdBy != "" {
		ext["created_by"] = createdBy
	}
	if sourceRepo != "" && sourceRepo != "." {
		ext["source_repo"] = sourceRepo
	}
	if externalRef != "" {
		ext["external_ref"] = externalRef
	}
	if pinned != 0 {
		ext["pinned"] = true
	}
	if ephemeral != 0 {
		ext["ephemeral"] = true
	}
	if isTemplate != 0 {
		ext["is_template"] = true
	}
	if len(ext) > 0 {
		issue.Extensions = ext
	}

	return &issue, nil
}

// loadLabels batch-loads labels for the given issue IDs.
func (r *SQLiteReader) loadLabels(ids []string, issueMap map[string]*task.Issue) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders, args := inClause(ids)
	// NOT INDEXED: bypass corrupt sqlite_autoindex_labels_1 created by frankensqlite.
	//nolint:gosec // G202: placeholders contains only "?" markers from inClause, not user input
	rows, err := r.db.QueryContext(context.Background(),
		"SELECT issue_id, label FROM labels NOT INDEXED WHERE issue_id IN ("+placeholders+") ORDER BY issue_id, label",
		args...,
	)
	if err != nil {
		return fmt.Errorf("load labels: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var issueID, label string
		if err := rows.Scan(&issueID, &label); err != nil {
			return fmt.Errorf("scan label: %w", err)
		}
		if issue, ok := issueMap[issueID]; ok {
			issue.Labels = append(issue.Labels, label)
		}
	}
	return rows.Err()
}

// loadDependencies batch-loads dependencies for the given issue IDs.
// It queries both directions (forward and reverse) of the dependencies table.
func (r *SQLiteReader) loadDependencies(ids []string, issueMap map[string]*task.Issue) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders, args := inClause(ids)

	// NOT INDEXED: bypass corrupt sqlite_autoindex_dependencies_1 from frankensqlite.
	// Forward: issues in our set depend ON other issues.
	// Reverse: other issues depend ON issues in our set.
	//nolint:gosec // G202: placeholders contains only "?" markers from inClause, not user input
	query := `
		SELECT d.issue_id, d.depends_on_id, d.type, 'forward' AS dir
		FROM dependencies d NOT INDEXED
		WHERE d.issue_id IN (` + placeholders + `)
		UNION ALL
		SELECT d.issue_id, d.depends_on_id, d.type, 'reverse' AS dir
		FROM dependencies d NOT INDEXED
		WHERE d.depends_on_id IN (` + placeholders + `)
		ORDER BY issue_id, depends_on_id`

	// Double the args for both halves of the UNION.
	allArgs := make([]any, 0, len(args)*2)
	allArgs = append(allArgs, args...)
	allArgs = append(allArgs, args...)

	rows, err := r.db.QueryContext(context.Background(), query, allArgs...)
	if err != nil {
		return fmt.Errorf("load dependencies: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var issueID, dependsOnID, depType, dir string
		if err := rows.Scan(&issueID, &dependsOnID, &depType, &dir); err != nil {
			return fmt.Errorf("scan dependency: %w", err)
		}

		if dir == "forward" {
			issue, ok := issueMap[issueID]
			if !ok {
				continue
			}
			switch depType {
			case "blocks", "conditional-blocks", "waits-for":
				issue.BlockedBy = append(issue.BlockedBy, dependsOnID)
			case "parent-child":
				issue.ParentID = dependsOnID
			case "discovered-from":
				issue.DiscoveredFrom = append(issue.DiscoveredFrom, dependsOnID)
			}
		} else {
			// Reverse: dependsOnID is in our set.
			issue, ok := issueMap[dependsOnID]
			if !ok {
				continue
			}
			switch depType {
			case "blocks", "conditional-blocks", "waits-for":
				issue.Blocks = append(issue.Blocks, issueID)
			case "parent-child":
				issue.Children = append(issue.Children, issueID)
			case "discovered-from":
				issue.Discovered = append(issue.Discovered, issueID)
			}
		}
	}
	return rows.Err()
}

// loadComments batch-loads full comments for the given issue IDs.
// Used by ShowIssue which needs full comment data.
func (r *SQLiteReader) loadComments(ids []string, issueMap map[string]*task.Issue) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders, args := inClause(ids)
	//nolint:gosec // G202: placeholders contains only "?" markers from inClause, not user input
	rows, err := r.db.QueryContext(context.Background(),
		"SELECT id, issue_id, author, text, created_at FROM comments WHERE issue_id IN ("+placeholders+") ORDER BY created_at ASC",
		args...,
	)
	if err != nil {
		return fmt.Errorf("load comments: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var c task.Comment
		var issueID, createdAt string
		if err := rows.Scan(&c.ID, &issueID, &c.Author, &c.Text, &createdAt); err != nil {
			return fmt.Errorf("scan comment: %w", err)
		}
		c.CreatedAt = parseTime(createdAt)
		if issue, ok := issueMap[issueID]; ok {
			issue.Comments = append(issue.Comments, c)
			issue.CommentCount = len(issue.Comments)
		}
	}
	return rows.Err()
}

// --- Helpers ---

// inClause builds a SQL IN clause with placeholders and args.
func inClause(ids []string) (string, []any) {
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	return strings.Join(placeholders, ","), args
}

// nullStr extracts a string from sql.NullString.
func nullStr(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

// parseTime attempts to parse a time string in common SQLite formats.
func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	// Try RFC3339 first (most common from br).
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	// Try RFC3339Nano.
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	// Try SQLite default format.
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t
	}
	// Try SQLite datetime with fractional seconds.
	if t, err := time.Parse("2006-01-02T15:04:05.999999Z", s); err == nil {
		return t
	}
	// Try with timezone offset.
	if t, err := time.Parse("2006-01-02T15:04:05.999999-07:00", s); err == nil {
		return t
	}
	return time.Time{}
}
