package beads

import (
	"database/sql"
	"testing"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient_InvalidPath(t *testing.T) {
	_, err := NewClient("/nonexistent/path/that/does/not/exist")
	assert.Error(t, err, "expected error for invalid path")
}

// setupTestDB creates an in-memory SQLite database with test data for client tests.
func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)

	// Create schema
	schema := `
		CREATE TABLE issues (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			description TEXT,
			status TEXT NOT NULL DEFAULT 'open',
			priority INTEGER NOT NULL DEFAULT 2,
			issue_type TEXT NOT NULL DEFAULT 'task',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE labels (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			issue_id TEXT NOT NULL,
			label TEXT NOT NULL,
			FOREIGN KEY (issue_id) REFERENCES issues(id)
		);

		CREATE TABLE dependencies (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			issue_id TEXT NOT NULL,
			depends_on_id TEXT NOT NULL,
			type TEXT NOT NULL DEFAULT 'blocks',
			FOREIGN KEY (issue_id) REFERENCES issues(id),
			FOREIGN KEY (depends_on_id) REFERENCES issues(id)
		);
	`
	_, err = db.Exec(schema)
	require.NoError(t, err)

	// Insert test data
	now := time.Now()
	yesterday := now.Add(-24 * time.Hour)

	testIssues := []struct {
		id, title, desc, status string
		priority                int
		issueType               string
		createdAt, updatedAt    time.Time
	}{
		{"issue-1", "First issue", "Description 1", "open", 0, "bug", yesterday, now},
		{"issue-2", "Second issue", "Description 2", "open", 1, "feature", yesterday, yesterday},
		{"issue-3", "Third issue", "", "in_progress", 2, "task", yesterday, yesterday},
		{"issue-4", "Deleted issue", "Should not appear", "deleted", 0, "bug", yesterday, now},
		{"epic-1", "Epic with children", "An epic", "open", 1, "epic", yesterday, now},
	}

	for _, i := range testIssues {
		_, err = db.Exec(
			`INSERT INTO issues (id, title, description, status, priority, issue_type, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			i.id, i.title, i.desc, i.status, i.priority, i.issueType, i.createdAt, i.updatedAt,
		)
		require.NoError(t, err)
	}

	// Insert labels
	labels := []struct {
		issueID, label string
	}{
		{"issue-1", "urgent"},
		{"issue-1", "backend"},
		{"issue-2", "frontend"},
	}

	for _, l := range labels {
		_, err = db.Exec(`INSERT INTO labels (issue_id, label) VALUES (?, ?)`, l.issueID, l.label)
		require.NoError(t, err)
	}

	// Insert dependencies
	// issue-3 is blocked by issue-1
	_, err = db.Exec(
		`INSERT INTO dependencies (issue_id, depends_on_id, type) VALUES (?, ?, ?)`,
		"issue-3", "issue-1", "blocks",
	)
	require.NoError(t, err)

	// issue-2 is a child of epic-1
	_, err = db.Exec(
		`INSERT INTO dependencies (issue_id, depends_on_id, type) VALUES (?, ?, ?)`,
		"issue-2", "epic-1", "parent-child",
	)
	require.NoError(t, err)

	return db
}

// newTestClient creates a Client using the provided test database.
func newTestClient(db *sql.DB) *Client {
	return &Client{db: db, dbPath: ":memory:"}
}

func TestListIssuesByIds_EmptyList(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	client := newTestClient(db)

	issues, err := client.ListIssuesByIds([]string{})
	require.NoError(t, err)
	assert.Empty(t, issues)
}

func TestListIssuesByIds_SingleID(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	client := newTestClient(db)

	issues, err := client.ListIssuesByIds([]string{"issue-1"})
	require.NoError(t, err)
	require.Len(t, issues, 1)

	assert.Equal(t, "issue-1", issues[0].ID)
	assert.Equal(t, "First issue", issues[0].TitleText)
	assert.Equal(t, "Description 1", issues[0].DescriptionText)
	assert.Equal(t, StatusOpen, issues[0].Status)
	assert.Equal(t, Priority(0), issues[0].Priority)
	assert.Equal(t, TypeBug, issues[0].Type)
}

func TestListIssuesByIds_MultipleIDs(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	client := newTestClient(db)

	issues, err := client.ListIssuesByIds([]string{"issue-1", "issue-2", "issue-3"})
	require.NoError(t, err)
	require.Len(t, issues, 3)

	// Collect IDs
	ids := make([]string, len(issues))
	for i, issue := range issues {
		ids[i] = issue.ID
	}
	assert.ElementsMatch(t, []string{"issue-1", "issue-2", "issue-3"}, ids)
}

func TestListIssuesByIds_NonExistentIDsOmitted(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	client := newTestClient(db)

	issues, err := client.ListIssuesByIds([]string{"issue-1", "nonexistent-xyz", "issue-2"})
	require.NoError(t, err)
	require.Len(t, issues, 2, "nonexistent IDs should be silently omitted")

	ids := make([]string, len(issues))
	for i, issue := range issues {
		ids[i] = issue.ID
	}
	assert.ElementsMatch(t, []string{"issue-1", "issue-2"}, ids)
}

func TestListIssuesByIds_DeletedExcluded(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	client := newTestClient(db)

	issues, err := client.ListIssuesByIds([]string{"issue-1", "issue-4"})
	require.NoError(t, err)
	require.Len(t, issues, 1, "deleted issues should be excluded")

	assert.Equal(t, "issue-1", issues[0].ID)
}

func TestListIssuesByIds_LabelsPopulated(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	client := newTestClient(db)

	issues, err := client.ListIssuesByIds([]string{"issue-1"})
	require.NoError(t, err)
	require.Len(t, issues, 1)

	assert.ElementsMatch(t, []string{"urgent", "backend"}, issues[0].Labels)
}

func TestListIssuesByIds_BlockedByPopulated(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	client := newTestClient(db)

	// issue-3 is blocked by issue-1
	issues, err := client.ListIssuesByIds([]string{"issue-3"})
	require.NoError(t, err)
	require.Len(t, issues, 1)

	assert.Contains(t, issues[0].BlockedBy, "issue-1")
}

func TestListIssuesByIds_BlocksPopulated(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	client := newTestClient(db)

	// issue-1 blocks issue-3
	issues, err := client.ListIssuesByIds([]string{"issue-1"})
	require.NoError(t, err)
	require.Len(t, issues, 1)

	assert.Contains(t, issues[0].Blocks, "issue-3")
}

func TestListIssuesByIds_EpicChildrenInBlocks(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	client := newTestClient(db)

	// epic-1 has issue-2 as a child
	issues, err := client.ListIssuesByIds([]string{"epic-1"})
	require.NoError(t, err)
	require.Len(t, issues, 1)

	assert.Contains(t, issues[0].Blocks, "issue-2", "epic should have children in Blocks field")
}

func TestListIssuesByIds_NullDescription(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	client := newTestClient(db)

	// issue-3 has empty description
	issues, err := client.ListIssuesByIds([]string{"issue-3"})
	require.NoError(t, err)
	require.Len(t, issues, 1)

	// Empty description should be handled gracefully
	assert.Equal(t, "", issues[0].DescriptionText)
}

func TestListIssuesByIds_AllNonExistent(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	client := newTestClient(db)

	issues, err := client.ListIssuesByIds([]string{"fake-1", "fake-2", "fake-3"})
	require.NoError(t, err)
	assert.Empty(t, issues, "all nonexistent IDs should return empty slice")
}
