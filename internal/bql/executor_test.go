package bql

import (
	"database/sql"
	"perles/internal/beads"
	"testing"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/stretchr/testify/require"
)

// strPtr returns a pointer to the given string (helper for nullable assignee).
func strPtr(s string) *string { return &s }

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
			assignee TEXT,
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
		assignee                *string // nil means NULL
		createdAt, updatedAt    time.Time
	}{
		{"test-1", "Fix login bug", "Login fails for users", "open", 0, "bug", strPtr("alice"), lastWeek, now},
		{"test-2", "Add search feature", "Search functionality", "open", 1, "feature", nil, yesterday, yesterday},
		{"test-3", "Refactor auth", "Clean up auth code", "in_progress", 2, "task", strPtr("bob"), lastWeek, yesterday},
		{"test-4", "Update docs", "Documentation update", "closed", 3, "chore", nil, lastWeek, lastWeek},
		{"test-5", "Critical security fix", "Urgent fix needed", "open", 0, "bug", strPtr("alice"), now, now},
		{"test-6", "Epic: New dashboard", "Dashboard epic", "open", 1, "epic", nil, lastWeek, now},
	}

	for _, i := range testIssues {
		_, err = db.Exec(
			`INSERT INTO issues (id, title, description, status, priority, issue_type, assignee, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			i.id, i.title, i.desc, i.status, i.priority, i.issueType, i.assignee, i.createdAt, i.updatedAt,
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

	require.Len(t, issues, 2)
	for _, issue := range issues {
		require.Equal(t, beads.TypeBug, issue.Type)
	}
}

func TestExecutor_StatusFilter(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	issues, err := executor.Execute("status = open")
	require.NoError(t, err)

	require.Len(t, issues, 4) // test-1, test-2, test-5, test-6
	for _, issue := range issues {
		require.Equal(t, beads.StatusOpen, issue.Status)
	}
}

func TestExecutor_PriorityFilter(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// P0 issues only
	issues, err := executor.Execute("priority = P0")
	require.NoError(t, err)

	require.Len(t, issues, 2) // test-1 and test-5
	for _, issue := range issues {
		require.Equal(t, beads.Priority(0), issue.Priority)
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
	require.Len(t, issues, 4)
	for _, issue := range issues {
		require.Less(t, int(issue.Priority), 2)
	}
}

func TestExecutor_BlockedFilter(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Blocked issues
	issues, err := executor.Execute("blocked = true")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Equal(t, "test-3", issues[0].ID)
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
	require.Len(t, issues, 4)
	for _, issue := range issues {
		require.NotEqual(t, "test-3", issue.ID, "blocked issue should not be ready")
		require.NotEqual(t, "test-4", issue.ID, "closed issue should not be ready")
	}
}

func TestExecutor_LabelFilter(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Issues with urgent label
	issues, err := executor.Execute("label = urgent")
	require.NoError(t, err)

	require.Len(t, issues, 2) // test-1 and test-5
	for _, issue := range issues {
		require.Contains(t, []string{"test-1", "test-5"}, issue.ID)
	}
}

func TestExecutor_TitleContains(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Title contains "bug"
	issues, err := executor.Execute("title ~ bug")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Equal(t, "test-1", issues[0].ID)
}

func TestExecutor_OrderBy(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Order by priority ascending
	issues, err := executor.Execute("status = open order by priority asc")
	require.NoError(t, err)

	require.Len(t, issues, 4)
	// First two should be P0
	require.Equal(t, beads.Priority(0), issues[0].Priority)
	require.Equal(t, beads.Priority(0), issues[1].Priority)
	// Next should be P1
	require.Equal(t, beads.Priority(1), issues[2].Priority)
}

func TestExecutor_ComplexQuery(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Complex: type = bug and priority = P0
	issues, err := executor.Execute("type = bug and priority = P0")
	require.NoError(t, err)

	require.Len(t, issues, 2) // test-1 and test-5
	for _, issue := range issues {
		require.Equal(t, beads.TypeBug, issue.Type)
		require.Equal(t, beads.Priority(0), issue.Priority)
	}
}

func TestExecutor_OrQuery(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// type = bug or type = feature
	issues, err := executor.Execute("type = bug or type = feature")
	require.NoError(t, err)

	require.Len(t, issues, 3) // test-1, test-2, test-5
	for _, issue := range issues {
		require.Contains(t, []beads.IssueType{beads.TypeBug, beads.TypeFeature}, issue.Type)
	}
}

func TestExecutor_InExpression(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// type in (bug, task)
	issues, err := executor.Execute("type in (bug, task)")
	require.NoError(t, err)

	require.Len(t, issues, 3) // test-1, test-3, test-5
	for _, issue := range issues {
		require.Contains(t, []beads.IssueType{beads.TypeBug, beads.TypeTask}, issue.Type)
	}
}

func TestExecutor_EmptyResult(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// No P4 priority issues exist
	issues, err := executor.Execute("priority = P4")
	require.NoError(t, err)

	require.Empty(t, issues)
}

func TestExecutor_InvalidQuery(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Invalid field
	_, err := executor.Execute("foo = bar")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown field")
}

func TestExecutor_ParseError(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Invalid syntax
	_, err := executor.Execute("type = = bug")
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse error")
}

func TestExecutor_LoadsLabels(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Get test-1 which has labels
	issues, err := executor.Execute("id = test-1")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Contains(t, issues[0].Labels, "urgent")
	require.Contains(t, issues[0].Labels, "auth")
}

func TestExecutor_LoadsBlockedBy(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Get test-3 which is blocked by test-1
	issues, err := executor.Execute("id = test-3")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Contains(t, issues[0].BlockedBy, "test-1")
}

func TestExecutor_LoadsBlocks(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Get test-1 which blocks test-3
	issues, err := executor.Execute("id = test-1")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Contains(t, issues[0].Blocks, "test-3", "test-1 should show test-3 in Blocks")
}

func TestExecutor_LoadsBlocksForEpicWithChildren(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Get test-6 (epic) which has test-2 as child via parent-child dependency
	issues, err := executor.Execute("id = test-6")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Contains(t, issues[0].Children, "test-2", "epic should show child in Children")
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
		// Expand keyword tests
		{"id = x expand children", true},
		{"type = epic expand all depth *", true},
		{"expand children order by priority", true},
		{"expand blockers", true},
		{"EXPAND CHILDREN", true},
		{"id = x EXPAND children DEPTH 2", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := IsBQLQuery(tt.input)
			require.Equal(t, tt.want, got)
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

	require.Len(t, issues, 6) // All issues except deleted
	// First should be P0
	require.Equal(t, beads.Priority(0), issues[0].Priority)
}

func TestExecutor_AssigneePopulated(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Get test-1 which has assignee "alice"
	issues, err := executor.Execute("id = test-1")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Equal(t, "alice", issues[0].Assignee, "assignee should be populated from database")
}

func TestExecutor_AssigneeNullIsEmptyString(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Get test-2 which has NULL assignee
	issues, err := executor.Execute("id = test-2")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Equal(t, "", issues[0].Assignee, "NULL assignee should be empty string")
}

func TestExecutor_MultipleIssuesWithDifferentAssignees(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Get multiple issues and verify each has correct assignee
	issues, err := executor.Execute("id in (test-1, test-2, test-3)")
	require.NoError(t, err)

	require.Len(t, issues, 3)

	// Build map for easier assertion
	assigneeByID := make(map[string]string)
	for _, issue := range issues {
		assigneeByID[issue.ID] = issue.Assignee
	}

	require.Equal(t, "alice", assigneeByID["test-1"])
	require.Equal(t, "", assigneeByID["test-2"])
	require.Equal(t, "bob", assigneeByID["test-3"])
}

// setupExpandTestDB creates a database with more complex dependency relationships.
func setupExpandTestDB(t *testing.T) *sql.DB {
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
			assignee TEXT,
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

	// Create a hierarchical structure:
	// epic-1 (epic)
	//   ├── task-1 (child)
	//   │     └── subtask-1 (grandchild)
	//   └── task-2 (child)
	//
	// And blocking relationships:
	// blocker-1 blocks blocked-1
	// blocked-1 blocks blocked-2 (chain: blocker-1 -> blocked-1 -> blocked-2)

	issues := []struct {
		id, title, issueType string
	}{
		{"epic-1", "Epic One", "epic"},
		{"task-1", "Task One", "task"},
		{"task-2", "Task Two", "task"},
		{"subtask-1", "Subtask One", "task"},
		{"blocker-1", "Blocker One", "bug"},
		{"blocked-1", "Blocked One", "task"},
		{"blocked-2", "Blocked Two", "task"},
		{"standalone", "Standalone Issue", "task"},
	}

	for _, i := range issues {
		_, err = db.Exec(
			`INSERT INTO issues (id, title, status, priority, issue_type) VALUES (?, ?, 'open', 1, ?)`,
			i.id, i.title, i.issueType,
		)
		require.NoError(t, err)
	}

	// Parent-child relationships
	deps := []struct {
		issueID, dependsOnID, depType string
	}{
		{"task-1", "epic-1", "parent-child"},    // task-1 is child of epic-1
		{"task-2", "epic-1", "parent-child"},    // task-2 is child of epic-1
		{"subtask-1", "task-1", "parent-child"}, // subtask-1 is child of task-1
		{"blocked-1", "blocker-1", "blocks"},    // blocked-1 is blocked by blocker-1
		{"blocked-2", "blocked-1", "blocks"},    // blocked-2 is blocked by blocked-1
	}

	for _, d := range deps {
		_, err = db.Exec(
			`INSERT INTO dependencies (issue_id, depends_on_id, type) VALUES (?, ?, ?)`,
			d.issueID, d.dependsOnID, d.depType,
		)
		require.NoError(t, err)
	}

	return db
}

func TestExecutor_ExpandChildren(t *testing.T) {
	db := setupExpandTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Get epic with direct children
	issues, err := executor.Execute("id = epic-1 expand children")
	require.NoError(t, err)

	// Should return epic-1, task-1, task-2 (3 issues)
	require.Len(t, issues, 3)

	ids := make(map[string]bool)
	for _, issue := range issues {
		ids[issue.ID] = true
	}

	require.True(t, ids["epic-1"], "should include base issue")
	require.True(t, ids["task-1"], "should include child task-1")
	require.True(t, ids["task-2"], "should include child task-2")
	require.False(t, ids["subtask-1"], "should NOT include grandchild at depth 1")
}

func TestExecutor_ExpandChildrenDepth2(t *testing.T) {
	db := setupExpandTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Get epic with children and grandchildren (depth 2)
	issues, err := executor.Execute("id = epic-1 expand children depth 2")
	require.NoError(t, err)

	// Should return epic-1, task-1, task-2, subtask-1 (4 issues)
	require.Len(t, issues, 4)

	ids := make(map[string]bool)
	for _, issue := range issues {
		ids[issue.ID] = true
	}

	require.True(t, ids["epic-1"], "should include base issue")
	require.True(t, ids["task-1"], "should include child")
	require.True(t, ids["task-2"], "should include child")
	require.True(t, ids["subtask-1"], "should include grandchild at depth 2")
}

func TestExecutor_ExpandChildrenUnlimited(t *testing.T) {
	db := setupExpandTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Get epic with all descendants
	issues, err := executor.Execute("id = epic-1 expand children depth *")
	require.NoError(t, err)

	// Should return all 4 issues in hierarchy
	require.Len(t, issues, 4)

	ids := make(map[string]bool)
	for _, issue := range issues {
		ids[issue.ID] = true
	}

	require.True(t, ids["epic-1"])
	require.True(t, ids["task-1"])
	require.True(t, ids["task-2"])
	require.True(t, ids["subtask-1"])
}

func TestExecutor_ExpandBlockers(t *testing.T) {
	db := setupExpandTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Get blocked-1 with its blockers
	issues, err := executor.Execute("id = blocked-1 expand blockers")
	require.NoError(t, err)

	// Should return blocked-1 and blocker-1
	require.Len(t, issues, 2)

	ids := make(map[string]bool)
	for _, issue := range issues {
		ids[issue.ID] = true
	}

	require.True(t, ids["blocked-1"], "should include base issue")
	require.True(t, ids["blocker-1"], "should include blocker")
}

func TestExecutor_ExpandBlocks(t *testing.T) {
	db := setupExpandTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Get blocker-1 with issues it blocks
	issues, err := executor.Execute("id = blocker-1 expand blocks")
	require.NoError(t, err)

	// Should return blocker-1 and blocked-1
	require.Len(t, issues, 2)

	ids := make(map[string]bool)
	for _, issue := range issues {
		ids[issue.ID] = true
	}

	require.True(t, ids["blocker-1"], "should include base issue")
	require.True(t, ids["blocked-1"], "should include blocked issue")
}

func TestExecutor_ExpandDeps(t *testing.T) {
	db := setupExpandTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Get blocked-1 with bidirectional blocking deps
	issues, err := executor.Execute("id = blocked-1 expand deps")
	require.NoError(t, err)

	// Should return blocked-1, blocker-1 (blocks it), blocked-2 (it blocks)
	require.Len(t, issues, 3)

	ids := make(map[string]bool)
	for _, issue := range issues {
		ids[issue.ID] = true
	}

	require.True(t, ids["blocked-1"], "should include base issue")
	require.True(t, ids["blocker-1"], "should include issue that blocks it")
	require.True(t, ids["blocked-2"], "should include issue it blocks")
}

func TestExecutor_ExpandAll(t *testing.T) {
	db := setupExpandTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Get task-1 with all relationships
	issues, err := executor.Execute("id = task-1 expand all")
	require.NoError(t, err)

	// task-1 has: epic-1 (parent) and subtask-1 (child)
	require.Len(t, issues, 3)

	ids := make(map[string]bool)
	for _, issue := range issues {
		ids[issue.ID] = true
	}

	require.True(t, ids["task-1"], "should include base issue")
	require.True(t, ids["epic-1"], "should include parent via 'all'")
	require.True(t, ids["subtask-1"], "should include child via 'all'")
}

func TestExecutor_ExpandNoDuplicates(t *testing.T) {
	db := setupExpandTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Multiple iterations should not produce duplicates
	issues, err := executor.Execute("id = epic-1 expand children depth *")
	require.NoError(t, err)

	// Check for duplicates
	seen := make(map[string]bool)
	for _, issue := range issues {
		require.False(t, seen[issue.ID], "duplicate issue found: %s", issue.ID)
		seen[issue.ID] = true
	}
}

func TestExecutor_ExpandCircularDeps(t *testing.T) {
	db := setupExpandTestDB(t)
	defer func() { _ = db.Close() }()

	// Add circular blocking dependency: A blocks B, B blocks A
	_, err := db.Exec(`INSERT INTO issues (id, title, status, priority, issue_type) VALUES ('circular-a', 'Circular A', 'open', 1, 'task')`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO issues (id, title, status, priority, issue_type) VALUES ('circular-b', 'Circular B', 'open', 1, 'task')`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO dependencies (issue_id, depends_on_id, type) VALUES ('circular-b', 'circular-a', 'blocks')`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO dependencies (issue_id, depends_on_id, type) VALUES ('circular-a', 'circular-b', 'blocks')`)
	require.NoError(t, err)

	executor := NewExecutor(db)

	// Unlimited depth with circular deps should terminate
	issues, err := executor.Execute("id = circular-a expand deps depth *")
	require.NoError(t, err)

	// Should return both issues without infinite loop
	require.Len(t, issues, 2)

	ids := make(map[string]bool)
	for _, issue := range issues {
		ids[issue.ID] = true
	}

	require.True(t, ids["circular-a"])
	require.True(t, ids["circular-b"])
}

func TestExecutor_ExpandEmptyBaseResult(t *testing.T) {
	db := setupExpandTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Query with no results + expand should return empty
	issues, err := executor.Execute("id = nonexistent expand children")
	require.NoError(t, err)
	require.Empty(t, issues)
}

func TestExecutor_ExpandWithOrderBy(t *testing.T) {
	db := setupExpandTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Expand should work with order by (order applies to base query)
	issues, err := executor.Execute("id = epic-1 expand children order by id asc")
	require.NoError(t, err)

	require.Len(t, issues, 3)
	// First should be epic-1 (base result), children added after
	require.Equal(t, "epic-1", issues[0].ID)
}

// collectIDs is a helper that returns a map of issue IDs for easier assertions.
func collectIDs(issues []beads.Issue) map[string]bool {
	ids := make(map[string]bool)
	for _, issue := range issues {
		ids[issue.ID] = true
	}
	return ids
}

// setupNestedTestDB creates a DB with nested hierarchy for depth testing.
// Structure:
//
//	epic-1 (epic)
//	  ├── task-1 (child)
//	  │     └── subtask-1 (grandchild)
//	  └── task-2 (child)
func setupNestedTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)

	schema := `
		CREATE TABLE issues (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			description TEXT,
			status TEXT NOT NULL DEFAULT 'open',
			priority INTEGER NOT NULL DEFAULT 2,
			issue_type TEXT NOT NULL DEFAULT 'task',
			assignee TEXT,
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
		CREATE TABLE blocked_issues_cache (issue_id TEXT PRIMARY KEY);
		CREATE VIEW ready_issues AS
		SELECT i.id FROM issues i
		WHERE i.status IN ('open', 'in_progress')
		  AND i.id NOT IN (SELECT issue_id FROM blocked_issues_cache);
	`
	_, err = db.Exec(schema)
	require.NoError(t, err)

	issues := []struct{ id, title, issueType string }{
		{"epic-1", "Epic One", "epic"},
		{"task-1", "Task One", "task"},
		{"task-2", "Task Two", "task"},
		{"subtask-1", "Subtask One", "task"},
	}
	for _, i := range issues {
		_, err = db.Exec(`INSERT INTO issues (id, title, status, priority, issue_type) VALUES (?, ?, 'open', 1, ?)`,
			i.id, i.title, i.issueType)
		require.NoError(t, err)
	}

	deps := []struct{ issueID, dependsOnID string }{
		{"task-1", "epic-1"},
		{"task-2", "epic-1"},
		{"subtask-1", "task-1"},
	}
	for _, d := range deps {
		_, err = db.Exec(`INSERT INTO dependencies (issue_id, depends_on_id, type) VALUES (?, ?, 'parent-child')`,
			d.issueID, d.dependsOnID)
		require.NoError(t, err)
	}

	return db
}

// setupDeepTestDB creates a DB with a 4+ level deep hierarchy.
// Structure:
//
//	level-0 (epic)
//	  └── level-1
//	        └── level-2
//	              └── level-3
//	                    └── level-4
func setupDeepTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)

	schema := `
		CREATE TABLE issues (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			description TEXT,
			status TEXT NOT NULL DEFAULT 'open',
			priority INTEGER NOT NULL DEFAULT 2,
			issue_type TEXT NOT NULL DEFAULT 'task',
			assignee TEXT,
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
		CREATE TABLE blocked_issues_cache (issue_id TEXT PRIMARY KEY);
		CREATE VIEW ready_issues AS
		SELECT i.id FROM issues i
		WHERE i.status IN ('open', 'in_progress')
		  AND i.id NOT IN (SELECT issue_id FROM blocked_issues_cache);
	`
	_, err = db.Exec(schema)
	require.NoError(t, err)

	// Create 5 levels (0-4)
	for i := 0; i <= 4; i++ {
		issueType := "task"
		if i == 0 {
			issueType = "epic"
		}
		_, err = db.Exec(`INSERT INTO issues (id, title, status, priority, issue_type) VALUES (?, ?, 'open', 1, ?)`,
			"level-"+string(rune('0'+i)), "Level "+string(rune('0'+i)), issueType)
		require.NoError(t, err)
	}

	// Create parent-child relationships (level-N is child of level-(N-1))
	for i := 1; i <= 4; i++ {
		_, err = db.Exec(`INSERT INTO dependencies (issue_id, depends_on_id, type) VALUES (?, ?, 'parent-child')`,
			"level-"+string(rune('0'+i)), "level-"+string(rune('0'+i-1)))
		require.NoError(t, err)
	}

	return db
}

// setupCircularTestDB creates a DB with circular blocking dependencies.
// Structure: circular-a <-> circular-b (each blocks the other)
func setupCircularTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)

	schema := `
		CREATE TABLE issues (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			description TEXT,
			status TEXT NOT NULL DEFAULT 'open',
			priority INTEGER NOT NULL DEFAULT 2,
			issue_type TEXT NOT NULL DEFAULT 'task',
			assignee TEXT,
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
		CREATE TABLE blocked_issues_cache (issue_id TEXT PRIMARY KEY);
		CREATE VIEW ready_issues AS
		SELECT i.id FROM issues i
		WHERE i.status IN ('open', 'in_progress')
		  AND i.id NOT IN (SELECT issue_id FROM blocked_issues_cache);
	`
	_, err = db.Exec(schema)
	require.NoError(t, err)

	// Create two issues
	_, err = db.Exec(`INSERT INTO issues (id, title, status, priority, issue_type) VALUES ('circular-a', 'Circular A', 'open', 1, 'task')`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO issues (id, title, status, priority, issue_type) VALUES ('circular-b', 'Circular B', 'open', 1, 'task')`)
	require.NoError(t, err)

	// Circular blocking: A blocks B, B blocks A
	_, err = db.Exec(`INSERT INTO dependencies (issue_id, depends_on_id, type) VALUES ('circular-b', 'circular-a', 'blocks')`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO dependencies (issue_id, depends_on_id, type) VALUES ('circular-a', 'circular-b', 'blocks')`)
	require.NoError(t, err)

	return db
}

// =============================================================================
// Phase 5: Edge Case Tests
// =============================================================================

func TestExecutor_ExpandNoRelationships(t *testing.T) {
	db := setupExpandTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// standalone has no relationships - expand should return only the matched issue
	issues, err := executor.Execute("id = standalone expand children")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Equal(t, "standalone", issues[0].ID)
}

func TestExecutor_ExpandNoRelationshipsWithUnlimitedDepth(t *testing.T) {
	db := setupExpandTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// standalone has no relationships - even with depth * should return just the issue
	issues, err := executor.Execute("id = standalone expand all depth *")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Equal(t, "standalone", issues[0].ID)
}

func TestExecutor_ExpandSelfReferentialNoDuplicates(t *testing.T) {
	db := setupExpandTestDB(t)
	defer func() { _ = db.Close() }()

	// Add self-referential dependency: issue blocks itself
	_, err := db.Exec(`INSERT INTO issues (id, title, status, priority, issue_type) VALUES ('self-ref', 'Self Referential', 'open', 1, 'task')`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO dependencies (issue_id, depends_on_id, type) VALUES ('self-ref', 'self-ref', 'blocks')`)
	require.NoError(t, err)

	executor := NewExecutor(db)

	// Expand should not duplicate the self-referential issue
	issues, err := executor.Execute("id = self-ref expand deps depth *")
	require.NoError(t, err)

	require.Len(t, issues, 1, "self-referential issue should appear exactly once")
	require.Equal(t, "self-ref", issues[0].ID)
}

func TestExecutor_ExpandMultipleMatchesOverlappingDeps(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	schema := `
		CREATE TABLE issues (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			description TEXT,
			status TEXT NOT NULL DEFAULT 'open',
			priority INTEGER NOT NULL DEFAULT 2,
			issue_type TEXT NOT NULL DEFAULT 'task',
			assignee TEXT,
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
		CREATE TABLE blocked_issues_cache (issue_id TEXT PRIMARY KEY);
		CREATE VIEW ready_issues AS
		SELECT i.id FROM issues i
		WHERE i.status IN ('open', 'in_progress')
		  AND i.id NOT IN (SELECT issue_id FROM blocked_issues_cache);
	`
	_, err = db.Exec(schema)
	require.NoError(t, err)

	// Create two epics that share a child
	// epic-a -> shared-child
	// epic-b -> shared-child
	// epic-a -> unique-a
	// epic-b -> unique-b
	issues := []struct{ id, title, issueType string }{
		{"epic-a", "Epic A", "epic"},
		{"epic-b", "Epic B", "epic"},
		{"shared-child", "Shared Child", "task"},
		{"unique-a", "Unique A", "task"},
		{"unique-b", "Unique B", "task"},
	}
	for _, i := range issues {
		_, err = db.Exec(`INSERT INTO issues (id, title, status, priority, issue_type) VALUES (?, ?, 'open', 1, ?)`,
			i.id, i.title, i.issueType)
		require.NoError(t, err)
	}

	deps := []struct{ issueID, dependsOnID string }{
		{"shared-child", "epic-a"},
		{"shared-child", "epic-b"},
		{"unique-a", "epic-a"},
		{"unique-b", "epic-b"},
	}
	for _, d := range deps {
		_, err = db.Exec(`INSERT INTO dependencies (issue_id, depends_on_id, type) VALUES (?, ?, 'parent-child')`,
			d.issueID, d.dependsOnID)
		require.NoError(t, err)
	}

	executor := NewExecutor(db)

	// Query both epics with expand children - shared-child should appear only once
	issues2, err := executor.Execute("type = epic expand children")
	require.NoError(t, err)

	ids := collectIDs(issues2)

	// Should have: epic-a, epic-b, shared-child, unique-a, unique-b (5 unique issues)
	require.Len(t, ids, 5, "should have exactly 5 unique issues")
	require.True(t, ids["epic-a"])
	require.True(t, ids["epic-b"])
	require.True(t, ids["shared-child"])
	require.True(t, ids["unique-a"])
	require.True(t, ids["unique-b"])
}

func TestExecutor_ExpandWithoutFilter(t *testing.T) {
	db := setupNestedTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Expand without filter - should expand all issues
	issues, err := executor.Execute("expand children")
	require.NoError(t, err)

	// All 4 issues should be returned: epic-1, task-1, task-2, subtask-1
	// (the expand adds children of all issues returned)
	require.Len(t, issues, 4)
}

func TestExecutor_ExpandWithoutFilterOrderBy(t *testing.T) {
	db := setupNestedTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Expand without filter + order by
	issues, err := executor.Execute("expand children order by id asc")
	require.NoError(t, err)

	// All 4 issues
	require.Len(t, issues, 4)
	// First should be alphabetically first base issue
	require.Equal(t, "epic-1", issues[0].ID)
}

func TestExecutor_ExpandDepth10Boundary(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	schema := `
		CREATE TABLE issues (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			description TEXT,
			status TEXT NOT NULL DEFAULT 'open',
			priority INTEGER NOT NULL DEFAULT 2,
			issue_type TEXT NOT NULL DEFAULT 'task',
			assignee TEXT,
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
		CREATE TABLE blocked_issues_cache (issue_id TEXT PRIMARY KEY);
		CREATE VIEW ready_issues AS
		SELECT i.id FROM issues i
		WHERE i.status IN ('open', 'in_progress')
		  AND i.id NOT IN (SELECT issue_id FROM blocked_issues_cache);
	`
	_, err = db.Exec(schema)
	require.NoError(t, err)

	// Create 12 levels (0-11) to test depth 10 boundary
	for i := 0; i <= 11; i++ {
		issueType := "task"
		if i == 0 {
			issueType = "epic"
		}
		id := "d" + string(rune('a'+i)) // da, db, dc, ... dl
		_, err = db.Exec(`INSERT INTO issues (id, title, status, priority, issue_type) VALUES (?, ?, 'open', 1, ?)`,
			id, "Level "+string(rune('0'+i)), issueType)
		require.NoError(t, err)
	}

	// Create parent-child chain
	for i := 1; i <= 11; i++ {
		child := "d" + string(rune('a'+i))
		parent := "d" + string(rune('a'+i-1))
		_, err = db.Exec(`INSERT INTO dependencies (issue_id, depends_on_id, type) VALUES (?, ?, 'parent-child')`,
			child, parent)
		require.NoError(t, err)
	}

	executor := NewExecutor(db)

	// Depth 10 should return exactly 11 issues (root + 10 levels)
	issues, err := executor.Execute("id = da expand children depth 10")
	require.NoError(t, err)

	ids := collectIDs(issues)
	require.Len(t, ids, 11, "depth 10 should return root + 10 children levels")

	// Verify last included level is dk (level 10)
	require.True(t, ids["dk"], "level 10 (dk) should be included")
	// Verify level 11 (dl) is NOT included
	require.False(t, ids["dl"], "level 11 (dl) should NOT be included at depth 10")
}

func TestExecutor_ExpandMixedTermination(t *testing.T) {
	// Tests that depth * terminates naturally when graph ends, not at safety limit
	db := setupDeepTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// depth * on a 5-level hierarchy should return all 5, not 100
	issues, err := executor.Execute("id = level-0 expand children depth *")
	require.NoError(t, err)

	ids := collectIDs(issues)
	require.Len(t, ids, 5, "should return exactly 5 levels (0-4)")

	for i := 0; i <= 4; i++ {
		id := "level-" + string(rune('0'+i))
		require.True(t, ids[id], "should include %s", id)
	}
}

func TestExecutor_ExpandCircularDepsStandalone(t *testing.T) {
	db := setupCircularTestDB(t)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Unlimited depth with circular deps should terminate
	issues, err := executor.Execute("id = circular-a expand deps depth *")
	require.NoError(t, err)

	ids := collectIDs(issues)
	require.Len(t, ids, 2, "should return exactly 2 issues")
	require.True(t, ids["circular-a"])
	require.True(t, ids["circular-b"])
}

func TestExecutor_ExpandLargeFanout(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	schema := `
		CREATE TABLE issues (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			description TEXT,
			status TEXT NOT NULL DEFAULT 'open',
			priority INTEGER NOT NULL DEFAULT 2,
			issue_type TEXT NOT NULL DEFAULT 'task',
			assignee TEXT,
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
		CREATE TABLE blocked_issues_cache (issue_id TEXT PRIMARY KEY);
		CREATE VIEW ready_issues AS
		SELECT i.id FROM issues i
		WHERE i.status IN ('open', 'in_progress')
		  AND i.id NOT IN (SELECT issue_id FROM blocked_issues_cache);
	`
	_, err = db.Exec(schema)
	require.NoError(t, err)

	// Create 1 epic with 100 children (large fan-out)
	_, err = db.Exec(`INSERT INTO issues (id, title, status, priority, issue_type) VALUES ('big-epic', 'Big Epic', 'open', 1, 'epic')`)
	require.NoError(t, err)

	for i := 0; i < 100; i++ {
		id := "child-" + string(rune('0'+i/100)) + string(rune('0'+(i%100)/10)) + string(rune('0'+i%10))
		_, err = db.Exec(`INSERT INTO issues (id, title, status, priority, issue_type) VALUES (?, ?, 'open', 2, 'task')`,
			id, "Child "+id)
		require.NoError(t, err)
		_, err = db.Exec(`INSERT INTO dependencies (issue_id, depends_on_id, type) VALUES (?, 'big-epic', 'parent-child')`, id)
		require.NoError(t, err)
	}

	executor := NewExecutor(db)

	// Measure performance - should complete in <1 second
	start := time.Now()
	issues, err := executor.Execute("id = big-epic expand children")
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.Len(t, issues, 101, "should return epic + 100 children")
	require.Less(t, elapsed, time.Second, "large fan-out should complete in <1s, took %v", elapsed)
}
