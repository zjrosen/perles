package bql

import (
	"database/sql"
	"perles/internal/beads"
	"testing"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestDB creates an in-memory SQLite database with test data.
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

		CREATE TABLE blocked_issues_cache (
			issue_id TEXT PRIMARY KEY
		);

		CREATE VIEW ready_issues AS
		SELECT i.id
		FROM issues i
		WHERE i.status IN ('open', 'in_progress')
		  AND i.id NOT IN (SELECT issue_id FROM blocked_issues_cache);
	`
	_, err = db.Exec(schema)
	require.NoError(t, err)

	// Insert test data
	now := time.Now()
	yesterday := time.Now().Add(-24 * time.Hour)
	lastWeek := time.Now().Add(-7 * 24 * time.Hour)

	testIssues := []struct {
		id, title, desc, status string
		priority                int
		issueType               string
		createdAt, updatedAt    time.Time
	}{
		{"test-1", "Fix login bug", "Login fails for users", "open", 0, "bug", lastWeek, now},
		{"test-2", "Add search feature", "Search functionality", "open", 1, "feature", yesterday, yesterday},
		{"test-3", "Refactor auth", "Clean up auth code", "in_progress", 2, "task", lastWeek, yesterday},
		{"test-4", "Update docs", "Documentation update", "closed", 3, "chore", lastWeek, lastWeek},
		{"test-5", "Critical security fix", "Urgent fix needed", "open", 0, "bug", now, now},
		{"test-6", "Epic: New dashboard", "Dashboard epic", "open", 1, "epic", lastWeek, now},
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
		{"test-1", "urgent"},
		{"test-1", "auth"},
		{"test-5", "urgent"},
		{"test-5", "security"},
		{"test-3", "auth"},
	}

	for _, l := range labels {
		_, err = db.Exec(`INSERT INTO labels (issue_id, label) VALUES (?, ?)`, l.issueID, l.label)
		require.NoError(t, err)
	}

	// Insert blocked issues
	_, err = db.Exec(`INSERT INTO blocked_issues_cache (issue_id) VALUES (?)`, "test-3")
	require.NoError(t, err)

	// Insert dependency (test-3 blocked by test-1)
	_, err = db.Exec(
		`INSERT INTO dependencies (issue_id, depends_on_id, type) VALUES (?, ?, ?)`,
		"test-3", "test-1", "blocks",
	)
	require.NoError(t, err)

	// Insert parent-child dependency (test-2 is child of test-6 epic)
	_, err = db.Exec(
		`INSERT INTO dependencies (issue_id, depends_on_id, type) VALUES (?, ?, ?)`,
		"test-2", "test-6", "parent-child",
	)
	require.NoError(t, err)

	return db
}

func TestExecutor_TypeFilter(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Query for bugs only
	issues, err := executor.Execute("type = bug")
	require.NoError(t, err)

	assert.Len(t, issues, 2)
	for _, issue := range issues {
		assert.Equal(t, beads.TypeBug, issue.Type)
	}
}

func TestExecutor_StatusFilter(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	issues, err := executor.Execute("status = open")
	require.NoError(t, err)

	assert.Len(t, issues, 4) // test-1, test-2, test-5, test-6
	for _, issue := range issues {
		assert.Equal(t, beads.StatusOpen, issue.Status)
	}
}

func TestExecutor_PriorityFilter(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// P0 issues only
	issues, err := executor.Execute("priority = P0")
	require.NoError(t, err)

	assert.Len(t, issues, 2) // test-1 and test-5
	for _, issue := range issues {
		assert.Equal(t, beads.Priority(0), issue.Priority)
	}
}

func TestExecutor_PriorityComparison(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Priority less than P2 (so P0 and P1)
	issues, err := executor.Execute("priority < P2")
	require.NoError(t, err)

	// test-1 (P0), test-5 (P0), test-2 (P1), test-6 (P1) - 4 issues with priority < 2
	assert.Len(t, issues, 4)
	for _, issue := range issues {
		assert.Less(t, int(issue.Priority), 2)
	}
}

func TestExecutor_BlockedFilter(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Blocked issues
	issues, err := executor.Execute("blocked = true")
	require.NoError(t, err)

	assert.Len(t, issues, 1)
	assert.Equal(t, "test-3", issues[0].ID)
}

func TestExecutor_ReadyFilter(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Ready issues (open/in_progress and not blocked)
	issues, err := executor.Execute("ready = true")
	require.NoError(t, err)

	// test-1, test-2, test-5, test-6 are open/in_progress and not blocked
	// test-3 is in_progress but blocked
	// test-4 is closed
	assert.Len(t, issues, 4)
	for _, issue := range issues {
		assert.NotEqual(t, "test-3", issue.ID, "blocked issue should not be ready")
		assert.NotEqual(t, "test-4", issue.ID, "closed issue should not be ready")
	}
}

func TestExecutor_LabelFilter(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Issues with urgent label
	issues, err := executor.Execute("label = urgent")
	require.NoError(t, err)

	assert.Len(t, issues, 2) // test-1 and test-5
	for _, issue := range issues {
		assert.Contains(t, []string{"test-1", "test-5"}, issue.ID)
	}
}

func TestExecutor_TitleContains(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Title contains "bug"
	issues, err := executor.Execute("title ~ bug")
	require.NoError(t, err)

	assert.Len(t, issues, 1)
	assert.Equal(t, "test-1", issues[0].ID)
}

func TestExecutor_OrderBy(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Order by priority ascending
	issues, err := executor.Execute("status = open order by priority asc")
	require.NoError(t, err)

	assert.Len(t, issues, 4)
	// First two should be P0
	assert.Equal(t, beads.Priority(0), issues[0].Priority)
	assert.Equal(t, beads.Priority(0), issues[1].Priority)
	// Next should be P1
	assert.Equal(t, beads.Priority(1), issues[2].Priority)
}

func TestExecutor_ComplexQuery(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Complex: type = bug and priority = P0
	issues, err := executor.Execute("type = bug and priority = P0")
	require.NoError(t, err)

	assert.Len(t, issues, 2) // test-1 and test-5
	for _, issue := range issues {
		assert.Equal(t, beads.TypeBug, issue.Type)
		assert.Equal(t, beads.Priority(0), issue.Priority)
	}
}

func TestExecutor_OrQuery(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// type = bug or type = feature
	issues, err := executor.Execute("type = bug or type = feature")
	require.NoError(t, err)

	assert.Len(t, issues, 3) // test-1, test-2, test-5
	for _, issue := range issues {
		assert.Contains(t, []beads.IssueType{beads.TypeBug, beads.TypeFeature}, issue.Type)
	}
}

func TestExecutor_InExpression(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// type in (bug, task)
	issues, err := executor.Execute("type in (bug, task)")
	require.NoError(t, err)

	assert.Len(t, issues, 3) // test-1, test-3, test-5
	for _, issue := range issues {
		assert.Contains(t, []beads.IssueType{beads.TypeBug, beads.TypeTask}, issue.Type)
	}
}

func TestExecutor_EmptyResult(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// No P4 priority issues exist
	issues, err := executor.Execute("priority = P4")
	require.NoError(t, err)

	assert.Empty(t, issues)
}

func TestExecutor_InvalidQuery(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Invalid field
	_, err := executor.Execute("foo = bar")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown field")
}

func TestExecutor_ParseError(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Invalid syntax
	_, err := executor.Execute("type = = bug")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse error")
}

func TestExecutor_LoadsLabels(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Get test-1 which has labels
	issues, err := executor.Execute("id = test-1")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	assert.Contains(t, issues[0].Labels, "urgent")
	assert.Contains(t, issues[0].Labels, "auth")
}

func TestExecutor_LoadsBlockedBy(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Get test-3 which is blocked by test-1
	issues, err := executor.Execute("id = test-3")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	assert.Contains(t, issues[0].BlockedBy, "test-1")
}

func TestExecutor_LoadsBlocks(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Get test-1 which blocks test-3
	issues, err := executor.Execute("id = test-1")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	assert.Contains(t, issues[0].Blocks, "test-3", "test-1 should show test-3 in Blocks")
}

func TestExecutor_LoadsBlocksForEpicWithChildren(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Get test-6 (epic) which has test-2 as child via parent-child dependency
	issues, err := executor.Execute("id = test-6")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	assert.Contains(t, issues[0].Blocks, "test-2", "epic should show child in Blocks")
}

func TestIsBQLQuery(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"type = bug", true},
		{"priority > P1", true},
		{"status in (open, closed)", true},
		{"title ~ auth", true},
		{"type = bug and priority = P0", true},
		{"order by created desc", true},
		{"hello world", false},
		{"fix login", false},
		{"search term", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := IsBQLQuery(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExecutor_OrderByOnly(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Just order by, no filter (should return all non-deleted)
	issues, err := executor.Execute("order by priority asc")
	require.NoError(t, err)

	assert.Len(t, issues, 6) // All issues except deleted
	// First should be P0
	assert.Equal(t, beads.Priority(0), issues[0].Priority)
}
