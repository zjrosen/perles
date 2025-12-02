package beads

import (
	"database/sql"
	"strings"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

// Client provides access to beads data.
type Client struct {
	db     *sql.DB
	dbPath string
}

// NewClient creates a client connected to the beads database.
func NewClient(projectPath string) (*Client, error) {
	dbPath := projectPath + "/.beads/beads.db"
	db, err := sql.Open("sqlite3", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return nil, err
	}
	// Verify connection works
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return &Client{db: db, dbPath: dbPath}, nil
}

// Close closes the database connection.
func (c *Client) Close() error {
	return c.db.Close()
}

// DB returns the underlying database connection.
// Used by BQL executor to run queries directly.
func (c *Client) DB() *sql.DB {
	return c.db
}

// ListIssuesByIds fetches issues by their IDs directly from the database.
// Issues that don't exist or are deleted are silently omitted.
// Returns issues in arbitrary order (not guaranteed to match input order).
func (c *Client) ListIssuesByIds(ids []string) ([]Issue, error) {
	if len(ids) == 0 {
		return []Issue{}, nil
	}

	// Build placeholders for IN clause
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	// Query using same pattern as bql/executor.go
	query := `
		SELECT
			i.id, i.title, i.description, i.status,
			i.priority, i.issue_type, i.created_at, i.updated_at,
			COALESCE((
				SELECT GROUP_CONCAT(d.depends_on_id)
				FROM dependencies d
				JOIN issues blocker ON d.depends_on_id = blocker.id
				WHERE d.issue_id = i.id
					AND d.type = 'blocks'
					AND blocker.status IN ('open', 'in_progress', 'blocked')
			), '') as blocker_ids,
			COALESCE((
				SELECT GROUP_CONCAT(d.issue_id)
				FROM dependencies d
				JOIN issues child ON d.issue_id = child.id
				WHERE d.depends_on_id = i.id
					AND d.type IN ('blocks', 'parent-child')
					AND child.status != 'deleted'
			), '') as blocks_ids,
			COALESCE((
				SELECT GROUP_CONCAT(l.label)
				FROM labels l
				WHERE l.issue_id = i.id
			), '') as labels
		FROM issues i
		WHERE i.status != 'deleted'
			AND i.id IN (` + strings.Join(placeholders, ",") + `)`

	rows, err := c.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var issues []Issue
	for rows.Next() {
		var issue Issue
		var description sql.NullString
		var blockerIDs string
		var blocksIDs string
		var labelsStr string

		err := rows.Scan(
			&issue.ID, &issue.TitleText, &description,
			&issue.Status, &issue.Priority, &issue.Type,
			&issue.CreatedAt, &issue.UpdatedAt,
			&blockerIDs, &blocksIDs, &labelsStr,
		)
		if err != nil {
			return nil, err
		}

		if description.Valid {
			issue.DescriptionText = description.String
		}

		// Parse blocker IDs from comma-separated string
		if blockerIDs != "" {
			issue.BlockedBy = strings.Split(blockerIDs, ",")
		}

		// Parse blocks IDs from comma-separated string
		if blocksIDs != "" {
			issue.Blocks = strings.Split(blocksIDs, ",")
		}

		// Parse labels from comma-separated string
		if labelsStr != "" {
			issue.Labels = strings.Split(labelsStr, ",")
		}

		issues = append(issues, issue)
	}

	return issues, rows.Err()
}
