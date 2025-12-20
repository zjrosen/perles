package bql

import (
	"database/sql"
	"testing"
	"time"

	"perles/internal/beads"
	"perles/internal/testutil"

	"github.com/stretchr/testify/require"
)

// setupDB creates an in-memory SQLite database, optionally configured via the builder.
// Pass nil for an empty database, or a function to configure the builder.
func setupDB(t *testing.T, configure func(*testutil.Builder) *testutil.Builder) *sql.DB {
	db := testutil.NewTestDB(t)
	b := testutil.NewBuilder(t, db)
	if configure != nil {
		b = configure(b)
	}
	b.Build()
	return db
}

func TestExecutor_TypeFilter(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
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
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
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
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
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
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
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
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Blocked issues
	issues, err := executor.Execute("blocked = true")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Equal(t, "test-3", issues[0].ID)
}

func TestExecutor_ReadyFilter(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
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
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Issues with urgent label (exact match)
	issues, err := executor.Execute("label = urgent")
	require.NoError(t, err)

	require.Len(t, issues, 2) // test-1 and test-5
	for _, issue := range issues {
		require.Contains(t, []string{"test-1", "test-5"}, issue.ID)
	}
}

func TestExecutor_LabelContains(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Issues with labels containing "urg" (should match "urgent")
	issues, err := executor.Execute("label ~ urg")
	require.NoError(t, err)

	require.Len(t, issues, 2) // test-1 and test-5 have "urgent"
	for _, issue := range issues {
		require.Contains(t, []string{"test-1", "test-5"}, issue.ID)
	}
}

func TestExecutor_LabelNotContains(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Issues with labels NOT containing "urg" (excludes "urgent")
	issues, err := executor.Execute("label !~ urg and status = open")
	require.NoError(t, err)

	// Should exclude test-1 and test-5 which have "urgent"
	for _, issue := range issues {
		require.NotContains(t, []string{"test-1", "test-5"}, issue.ID)
	}
}

func TestExecutor_TitleContains(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Title contains "bug"
	issues, err := executor.Execute("title ~ bug")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Equal(t, "test-1", issues[0].ID)
}

func TestExecutor_OrderBy(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
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
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
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
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
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
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
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
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// No P4 priority issues exist
	issues, err := executor.Execute("priority = P4")
	require.NoError(t, err)

	require.Empty(t, issues)
}

func TestExecutor_InvalidQuery(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Invalid field
	_, err := executor.Execute("foo = bar")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown field")
}

func TestExecutor_ParseError(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Invalid syntax
	_, err := executor.Execute("type = = bug")
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse error")
}

func TestExecutor_LoadsLabels(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
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
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Get test-3 which is blocked by test-1
	issues, err := executor.Execute("id = test-3")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Contains(t, issues[0].BlockedBy, "test-1")
}

func TestExecutor_LoadsBlocks(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Get test-1 which blocks test-3
	issues, err := executor.Execute("id = test-1")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Contains(t, issues[0].Blocks, "test-3", "test-1 should show test-3 in Blocks")
}

func TestExecutor_LoadsChildrenForEpicWithChildren(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Get test-6 (epic) which has test-2 as child via parent-child dependency
	issues, err := executor.Execute("id = test-6")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Contains(t, issues[0].Children, "test-2", "epic should show child in Blocks")
}

func TestExecutor_LoadsRelated(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithDiscoveredFromTestData)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Get origin-1 which has discovered-1 as a related issue (discovered FROM origin-1)
	issues, err := executor.Execute("id = origin-1")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Contains(t, issues[0].Discovered, "discovered-1", "origin-1 should show discovered-1 in Discovered")
	require.Empty(t, issues[0].DiscoveredFrom, "origin-1 should have no DiscoveredFrom (it's the origin)")

	// Get discovered-1 which has both origin-1 (discovered from) and discovered-2 (discoverer)
	issues, err = executor.Execute("id = discovered-1")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Contains(t, issues[0].DiscoveredFrom, "origin-1", "discovered-1 should show origin-1 in DiscoveredFrom")
	require.Contains(t, issues[0].Discovered, "discovered-2", "discovered-1 should show discovered-2 in Discovered")
}

func TestExecutor_ExpandUpWithDiscoveredFrom(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithDiscoveredFromTestData)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Get discovered-1 with expand up - should include origin-1 (the issue it was discovered from)
	issues, err := executor.Execute("id = discovered-1 expand up")
	require.NoError(t, err)

	require.Len(t, issues, 2)

	ids := make(map[string]bool)
	for _, issue := range issues {
		ids[issue.ID] = true
	}

	require.True(t, ids["discovered-1"], "should include base issue")
	require.True(t, ids["origin-1"], "should include origin issue via discovered-from")
}

func TestExecutor_ExpandDownWithDiscoveredFrom(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithDiscoveredFromTestData)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Get origin-1 with expand down - should include discovered-1 (discovered from origin-1)
	issues, err := executor.Execute("id = origin-1 expand down")
	require.NoError(t, err)

	require.Len(t, issues, 2)

	ids := make(map[string]bool)
	for _, issue := range issues {
		ids[issue.ID] = true
	}

	require.True(t, ids["origin-1"], "should include base issue")
	require.True(t, ids["discovered-1"], "should include discovered issue")
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
		{"id = x expand down", true},
		{"type = epic expand all depth *", true},
		{"expand down order by priority", true},
		{"expand up", true},
		{"EXPAND DOWN", true},
		{"id = x EXPAND down DEPTH 2", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := IsBQLQuery(tt.input)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestExecutor_OrderByOnly(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
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
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Get test-1 which has assignee "alice"
	issues, err := executor.Execute("id = test-1")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Equal(t, "alice", issues[0].Assignee, "assignee should be populated from database")
}

func TestExecutor_AssigneeNullIsEmptyString(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Get test-2 which has NULL assignee
	issues, err := executor.Execute("id = test-2")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Equal(t, "", issues[0].Assignee, "NULL assignee should be empty string")
}

func TestExecutor_MultipleIssuesWithDifferentAssignees(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
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

func TestExecutor_ExpandDown(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithHierarchyTestData)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Get epic with direct children and blocked issues (expand down = children + blocks)
	issues, err := executor.Execute("id = epic-1 expand down")
	require.NoError(t, err)

	// Should return epic-1, task-1, task-2 (3 issues - children only since epic-1 doesn't block anything)
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

func TestExecutor_ExpandDownDepth2(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithHierarchyTestData)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Get epic with children and grandchildren (depth 2)
	issues, err := executor.Execute("id = epic-1 expand down depth 2")
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

func TestExecutor_ExpandDownUnlimited(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithHierarchyTestData)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Get epic with all descendants
	issues, err := executor.Execute("id = epic-1 expand down depth *")
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

func TestExecutor_ExpandUp(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithHierarchyTestData)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Get blocked-1 with its blockers (expand up = parent + blockers)
	issues, err := executor.Execute("id = blocked-1 expand up")
	require.NoError(t, err)

	// Should return blocked-1 and blocker-1 (blockers)
	require.Len(t, issues, 2)

	ids := make(map[string]bool)
	for _, issue := range issues {
		ids[issue.ID] = true
	}

	require.True(t, ids["blocked-1"], "should include base issue")
	require.True(t, ids["blocker-1"], "should include blocker")
}

func TestExecutor_ExpandDownWithBlocks(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithHierarchyTestData)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Get blocker-1 with issues it blocks (expand down includes blocks)
	issues, err := executor.Execute("id = blocker-1 expand down")
	require.NoError(t, err)

	// Should return blocker-1 and blocked-1 (blocked-1 is blocked by blocker-1)
	require.Len(t, issues, 2)

	ids := make(map[string]bool)
	for _, issue := range issues {
		ids[issue.ID] = true
	}

	require.True(t, ids["blocker-1"], "should include base issue")
	require.True(t, ids["blocked-1"], "should include blocked issue")
}

func TestExecutor_ExpandUpWithParent(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithHierarchyTestData)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Get task-1 with its parent (expand up = parent + blockers)
	issues, err := executor.Execute("id = task-1 expand up")
	require.NoError(t, err)

	// Should return task-1 and epic-1 (parent)
	require.Len(t, issues, 2)

	ids := make(map[string]bool)
	for _, issue := range issues {
		ids[issue.ID] = true
	}

	require.True(t, ids["task-1"], "should include base issue")
	require.True(t, ids["epic-1"], "should include parent")
}

func TestExecutor_ExpandAll(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithHierarchyTestData)
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
	db := setupDB(t, (*testutil.Builder).WithHierarchyTestData)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Multiple iterations should not produce duplicates
	issues, err := executor.Execute("id = epic-1 expand down depth *")
	require.NoError(t, err)

	// Check for duplicates
	seen := make(map[string]bool)
	for _, issue := range issues {
		require.False(t, seen[issue.ID], "duplicate issue found: %s", issue.ID)
		seen[issue.ID] = true
	}
}

func TestExecutor_ExpandCircularDeps(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithHierarchyTestData)
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

	// Unlimited depth with circular deps should terminate (expand all tests bidirectional)
	issues, err := executor.Execute("id = circular-a expand all depth *")
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
	db := setupDB(t, (*testutil.Builder).WithHierarchyTestData)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Query with no results + expand should return empty
	issues, err := executor.Execute("id = nonexistent expand down")
	require.NoError(t, err)
	require.Empty(t, issues)
}

func TestExecutor_ExpandWithOrderBy(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithHierarchyTestData)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Expand should work with order by (order applies to base query)
	issues, err := executor.Execute("id = epic-1 expand down order by id asc")
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

// =============================================================================
// Phase 5: Edge Case Tests
// =============================================================================

func TestExecutor_ExpandNoRelationships(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithHierarchyTestData)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// standalone has no relationships - expand should return only the matched issue
	issues, err := executor.Execute("id = standalone expand down")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Equal(t, "standalone", issues[0].ID)
}

func TestExecutor_ExpandNoRelationshipsWithUnlimitedDepth(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithHierarchyTestData)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// standalone has no relationships - even with depth * should return just the issue
	issues, err := executor.Execute("id = standalone expand all depth *")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Equal(t, "standalone", issues[0].ID)
}

func TestExecutor_ExpandSelfReferentialNoDuplicates(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithHierarchyTestData)
	defer func() { _ = db.Close() }()

	// Add self-referential dependency: issue blocks itself
	_, err := db.Exec(`INSERT INTO issues (id, title, status, priority, issue_type) VALUES ('self-ref', 'Self Referential', 'open', 1, 'task')`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO dependencies (issue_id, depends_on_id, type) VALUES ('self-ref', 'self-ref', 'blocks')`)
	require.NoError(t, err)

	executor := NewExecutor(db)

	// Expand should not duplicate the self-referential issue
	issues, err := executor.Execute("id = self-ref expand all depth *")
	require.NoError(t, err)

	require.Len(t, issues, 1, "self-referential issue should appear exactly once")
	require.Equal(t, "self-ref", issues[0].ID)
}

func TestExecutor_ExpandMultipleMatchesOverlappingDeps(t *testing.T) {
	// Create two epics that share a child
	// epic-a -> shared-child
	// epic-b -> shared-child
	// epic-a -> unique-a
	// epic-b -> unique-b
	db := testutil.NewTestDB(t)
	defer func() { _ = db.Close() }()

	testutil.NewBuilder(t, db).
		WithIssue("epic-a", testutil.Title("Epic A"), testutil.IssueType("epic")).
		WithIssue("epic-b", testutil.Title("Epic B"), testutil.IssueType("epic")).
		WithIssue("shared-child", testutil.Title("Shared Child"), testutil.IssueType("task")).
		WithIssue("unique-a", testutil.Title("Unique A"), testutil.IssueType("task")).
		WithIssue("unique-b", testutil.Title("Unique B"), testutil.IssueType("task")).
		WithDependency("shared-child", "epic-a", "parent-child").
		WithDependency("shared-child", "epic-b", "parent-child").
		WithDependency("unique-a", "epic-a", "parent-child").
		WithDependency("unique-b", "epic-b", "parent-child").
		Build()

	executor := NewExecutor(db)

	// Query both epics with expand down - shared-child should appear only once
	issues2, err := executor.Execute("type = epic expand down")
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
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.WithIssue("epic-1", testutil.Title("Epic One"), testutil.IssueType("epic")).
			WithIssue("task-1", testutil.Title("Task One"), testutil.IssueType("task")).
			WithIssue("task-2", testutil.Title("Task Two"), testutil.IssueType("task")).
			WithIssue("subtask-1", testutil.Title("Subtask One"), testutil.IssueType("task")).
			WithDependency("task-1", "epic-1", "parent-child").
			WithDependency("task-2", "epic-1", "parent-child").
			WithDependency("subtask-1", "task-1", "parent-child")
	})
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Expand without filter - should expand all issues
	issues, err := executor.Execute("expand down")
	require.NoError(t, err)

	// All 4 issues should be returned: epic-1, task-1, task-2, subtask-1
	// (the expand adds children of all issues returned)
	require.Len(t, issues, 4)
}

func TestExecutor_ExpandWithoutFilterOrderBy(t *testing.T) {
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.WithIssue("epic-1", testutil.Title("Epic One"), testutil.IssueType("epic")).
			WithIssue("task-1", testutil.Title("Task One"), testutil.IssueType("task")).
			WithIssue("task-2", testutil.Title("Task Two"), testutil.IssueType("task")).
			WithIssue("subtask-1", testutil.Title("Subtask One"), testutil.IssueType("task")).
			WithDependency("task-1", "epic-1", "parent-child").
			WithDependency("task-2", "epic-1", "parent-child").
			WithDependency("subtask-1", "task-1", "parent-child")
	})
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Expand without filter + order by
	issues, err := executor.Execute("expand down order by id asc")
	require.NoError(t, err)

	// All 4 issues
	require.Len(t, issues, 4)
	// First should be alphabetically first base issue
	require.Equal(t, "epic-1", issues[0].ID)
}

func TestExecutor_ExpandDepth10Boundary(t *testing.T) {
	// Create 12 levels (0-11) to test depth 10 boundary
	db := testutil.NewTestDB(t)
	defer func() { _ = db.Close() }()

	builder := testutil.NewBuilder(t, db)
	// Create 12 levels: da (epic), db, dc, ... dl (all tasks)
	builder.WithIssue("da", testutil.Title("Level 0"), testutil.IssueType("epic"))
	for i := 1; i <= 11; i++ {
		id := "d" + string(rune('a'+i))
		builder.WithIssue(id, testutil.Title("Level "+string(rune('0'+i))), testutil.IssueType("task"))
	}
	// Create parent-child chain
	for i := 1; i <= 11; i++ {
		child := "d" + string(rune('a'+i))
		parent := "d" + string(rune('a'+i-1))
		builder.WithDependency(child, parent, "parent-child")
	}
	builder.Build()

	executor := NewExecutor(db)

	// Depth 10 should return exactly 11 issues (root + 10 levels)
	issues, err := executor.Execute("id = da expand down depth 10")
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
	// 5-level deep hierarchy: level-0 -> level-1 -> level-2 -> level-3 -> level-4
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.WithIssue("level-0", testutil.Title("Level 0"), testutil.IssueType("epic")).
			WithIssue("level-1", testutil.Title("Level 1"), testutil.IssueType("task")).
			WithIssue("level-2", testutil.Title("Level 2"), testutil.IssueType("task")).
			WithIssue("level-3", testutil.Title("Level 3"), testutil.IssueType("task")).
			WithIssue("level-4", testutil.Title("Level 4"), testutil.IssueType("task")).
			WithDependency("level-1", "level-0", "parent-child").
			WithDependency("level-2", "level-1", "parent-child").
			WithDependency("level-3", "level-2", "parent-child").
			WithDependency("level-4", "level-3", "parent-child")
	})
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// depth * on a 5-level hierarchy should return all 5, not 100
	issues, err := executor.Execute("id = level-0 expand down depth *")
	require.NoError(t, err)

	ids := collectIDs(issues)
	require.Len(t, ids, 5, "should return exactly 5 levels (0-4)")

	for i := 0; i <= 4; i++ {
		id := "level-" + string(rune('0'+i))
		require.True(t, ids[id], "should include %s", id)
	}
}

func TestExecutor_ExpandCircularDepsStandalone(t *testing.T) {
	// Circular blocking: circular-a <-> circular-b
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.WithIssue("circular-a", testutil.Title("Circular A"), testutil.IssueType("task")).
			WithIssue("circular-b", testutil.Title("Circular B"), testutil.IssueType("task")).
			WithDependency("circular-b", "circular-a", "blocks").
			WithDependency("circular-a", "circular-b", "blocks")
	})
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Unlimited depth with circular deps should terminate
	issues, err := executor.Execute("id = circular-a expand all depth *")
	require.NoError(t, err)

	ids := collectIDs(issues)
	require.Len(t, ids, 2, "should return exactly 2 issues")
	require.True(t, ids["circular-a"])
	require.True(t, ids["circular-b"])
}

// =============================================================================
// BuildIDQuery Tests
// =============================================================================

func TestBuildIDQuery_Empty(t *testing.T) {
	// nil slice returns empty string
	require.Equal(t, "", BuildIDQuery(nil))
	// empty slice returns empty string
	require.Equal(t, "", BuildIDQuery([]string{}))
}

func TestBuildIDQuery_Single(t *testing.T) {
	result := BuildIDQuery([]string{"bd-123"})
	require.Equal(t, `id = "bd-123"`, result)
}

func TestBuildIDQuery_Multiple(t *testing.T) {
	result := BuildIDQuery([]string{"bd-1", "bd-2", "bd-3"})
	require.Equal(t, `id in ("bd-1", "bd-2", "bd-3")`, result)
}

func TestBuildIDQuery_SpecialCharacters(t *testing.T) {
	// IDs with hyphens, dots, and other characters
	result := BuildIDQuery([]string{"ms-8tn.1", "pd-j39"})
	require.Equal(t, `id in ("ms-8tn.1", "pd-j39")`, result)
}

func TestExecutor_IDIn_NonExistent(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Query for non-existent IDs should return empty slice, no error
	issues, err := executor.Execute(`id in ("nonexistent-1", "nonexistent-2")`)
	require.NoError(t, err)
	require.Empty(t, issues)
}

func TestExecutor_IDIn_Mixed(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Query with mix of existing and non-existent IDs returns only existing
	issues, err := executor.Execute(`id in ("test-1", "nonexistent", "test-3")`)
	require.NoError(t, err)
	require.Len(t, issues, 2)

	ids := make(map[string]bool)
	for _, issue := range issues {
		ids[issue.ID] = true
	}
	require.True(t, ids["test-1"])
	require.True(t, ids["test-3"])
	require.False(t, ids["nonexistent"])
}

func TestExecutor_ClosedAtPopulated(t *testing.T) {
	// Create a closed issue with closed_at timestamp
	closedAt := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	createdAt := time.Date(2024, 6, 1, 9, 0, 0, 0, time.UTC)

	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.WithIssue("closed-1",
			testutil.Title("Closed Issue"),
			testutil.Status("closed"),
			testutil.CreatedAt(createdAt),
			testutil.ClosedAt(closedAt),
		)
	})
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	issues, err := executor.Execute("id = closed-1")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Equal(t, beads.StatusClosed, issues[0].Status, "status should be closed")
	require.False(t, issues[0].ClosedAt.IsZero(), "ClosedAt should not be zero")
	require.Equal(t, closedAt.UTC(), issues[0].ClosedAt.UTC(), "ClosedAt should match the set timestamp")
}

func TestExecutor_ClosedAtNullForOpenIssue(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Get test-1 which is open
	issues, err := executor.Execute("id = test-1")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Equal(t, beads.StatusOpen, issues[0].Status, "status should be open")
	require.True(t, issues[0].ClosedAt.IsZero(), "ClosedAt should be zero for open issues")
}

func TestExecutor_ExpandLargeFanout(t *testing.T) {
	// Create 1 epic with 100 children (large fan-out)
	db := testutil.NewTestDB(t)
	defer func() { _ = db.Close() }()

	builder := testutil.NewBuilder(t, db)
	builder.WithIssue("big-epic", testutil.Title("Big Epic"), testutil.IssueType("epic"))
	for i := 0; i < 100; i++ {
		id := "child-" + string(rune('0'+i/100)) + string(rune('0'+(i%100)/10)) + string(rune('0'+i%10))
		builder.WithIssue(id, testutil.Title("Child "+id), testutil.IssueType("task"), testutil.Priority(2))
		builder.WithDependency(id, "big-epic", "parent-child")
	}
	builder.Build()

	executor := NewExecutor(db)

	// Measure performance - should complete in <1 second
	start := time.Now()
	issues, err := executor.Execute("id = big-epic expand down")
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.Len(t, issues, 101, "should return epic + 100 children")
	require.Less(t, elapsed, time.Second, "large fan-out should complete in <1s, took %v", elapsed)
}

// =============================================================================
// Soft Delete and Tombstone Tests
// =============================================================================

func TestExecutor_ExcludesTombstoneStatus(t *testing.T) {
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("open-1", testutil.Title("Open Issue"), testutil.Status("open")).
			WithIssue("tombstone-1", testutil.Title("Tombstone Issue"), testutil.Status("tombstone"))
	})
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Query all issues - tombstone should be excluded
	issues, err := executor.Execute("order by id asc")
	require.NoError(t, err)

	require.Len(t, issues, 1, "tombstone issues should be excluded")
	require.Equal(t, "open-1", issues[0].ID)
}

func TestExecutor_ExcludesDeletedAtIssues(t *testing.T) {
	deletedTime := time.Date(2024, 6, 15, 10, 0, 0, 0, time.UTC)

	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("open-1", testutil.Title("Open Issue"), testutil.Status("open")).
			WithIssue("soft-deleted-1", testutil.Title("Soft Deleted"), testutil.Status("deleted"), testutil.DeletedAt(deletedTime))
	})
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Query all issues - soft-deleted should be excluded
	issues, err := executor.Execute("order by id asc")
	require.NoError(t, err)

	require.Len(t, issues, 1, "soft-deleted issues should be excluded")
	require.Equal(t, "open-1", issues[0].ID)
}

func TestExecutor_ExcludesDeletedStatusWithDeletedAt(t *testing.T) {
	deletedTime := time.Date(2024, 6, 15, 10, 0, 0, 0, time.UTC)

	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("open-1", testutil.Title("Open Issue"), testutil.Status("open")).
			WithIssue("deleted-1", testutil.Title("Deleted Issue"), testutil.Status("deleted"), testutil.DeletedAt(deletedTime))
	})
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Query all issues - deleted should be excluded (both status and deleted_at check)
	issues, err := executor.Execute("order by id asc")
	require.NoError(t, err)

	require.Len(t, issues, 1, "deleted issues should be excluded")
	require.Equal(t, "open-1", issues[0].ID)
}

func TestExecutor_ExcludesMultipleSoftDeleteStates(t *testing.T) {
	deletedTime := time.Date(2024, 6, 15, 10, 0, 0, 0, time.UTC)

	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("open-1", testutil.Title("Open Issue")).
			WithIssue("closed-1", testutil.Title("Closed Issue"), testutil.Status("closed")).
			WithIssue("deleted-1", testutil.Title("Deleted"), testutil.Status("deleted"), testutil.DeletedAt(deletedTime)).
			WithIssue("tombstone-1", testutil.Title("Tombstone"), testutil.Status("tombstone"))
	})
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Query all issues
	issues, err := executor.Execute("order by id asc")
	require.NoError(t, err)

	require.Len(t, issues, 2, "should return only open and closed, not deleted/tombstone")

	ids := collectIDs(issues)
	require.True(t, ids["open-1"], "open issues should be included")
	require.True(t, ids["closed-1"], "closed issues should be included")
	require.False(t, ids["deleted-1"], "deleted issues should be excluded")
	require.False(t, ids["tombstone-1"], "tombstone issues should be excluded")
}

func TestExecutor_FetchByIDsExcludesTombstone(t *testing.T) {
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("epic-1", testutil.Title("Epic"), testutil.IssueType("epic")).
			WithIssue("task-1", testutil.Title("Task")).
			WithIssue("tombstone-task", testutil.Title("Tombstone Task"), testutil.Status("tombstone")).
			WithDependency("task-1", "epic-1", "parent-child").
			WithDependency("tombstone-task", "epic-1", "parent-child")
	})
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Expand down should not include tombstone task
	issues, err := executor.Execute("id = epic-1 expand down")
	require.NoError(t, err)

	ids := collectIDs(issues)
	require.Len(t, ids, 2, "should return epic + non-tombstone child")
	require.True(t, ids["epic-1"])
	require.True(t, ids["task-1"])
	require.False(t, ids["tombstone-task"], "tombstone task should be excluded from expand")
}

func TestExecutor_FetchByIDsExcludesDeletedAt(t *testing.T) {
	deletedTime := time.Date(2024, 6, 15, 10, 0, 0, 0, time.UTC)

	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("epic-1", testutil.Title("Epic"), testutil.IssueType("epic")).
			WithIssue("task-1", testutil.Title("Task")).
			WithIssue("deleted-task", testutil.Title("Deleted Task"), testutil.Status("deleted"), testutil.DeletedAt(deletedTime)).
			WithDependency("task-1", "epic-1", "parent-child").
			WithDependency("deleted-task", "epic-1", "parent-child")
	})
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	// Expand down should not include soft-deleted task
	issues, err := executor.Execute("id = epic-1 expand down")
	require.NoError(t, err)

	ids := collectIDs(issues)
	require.Len(t, ids, 2, "should return epic + non-deleted child")
	require.True(t, ids["epic-1"])
	require.True(t, ids["task-1"])
	require.False(t, ids["deleted-task"], "deleted task should be excluded from expand")
}

// =============================================================================
// Sender and Ephemeral Field Tests
// =============================================================================

func TestExecutor_SenderPopulated(t *testing.T) {
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("issue-1", testutil.Title("Issue with sender"), testutil.Sender("alice@example.com"))
	})
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	issues, err := executor.Execute("id = issue-1")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Equal(t, "alice@example.com", issues[0].Sender)
}

func TestExecutor_SenderEmptyByDefault(t *testing.T) {
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("issue-1", testutil.Title("Issue without sender"))
	})
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	issues, err := executor.Execute("id = issue-1")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Equal(t, "", issues[0].Sender, "default sender should be empty string")
}

func TestExecutor_EphemeralTrue(t *testing.T) {
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("issue-1", testutil.Title("Ephemeral issue"), testutil.Ephemeral(true))
	})
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	issues, err := executor.Execute("id = issue-1")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.True(t, issues[0].Ephemeral, "Ephemeral should be true")
}

func TestExecutor_EphemeralFalseByDefault(t *testing.T) {
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("issue-1", testutil.Title("Non-ephemeral issue"))
	})
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	issues, err := executor.Execute("id = issue-1")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.False(t, issues[0].Ephemeral, "default ephemeral should be false")
}

func TestExecutor_SenderAndEphemeralTogether(t *testing.T) {
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("issue-1", testutil.Title("Full issue"),
				testutil.Sender("bob@example.com"),
				testutil.Ephemeral(true),
			)
	})
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	issues, err := executor.Execute("id = issue-1")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Equal(t, "bob@example.com", issues[0].Sender)
	require.True(t, issues[0].Ephemeral)
}

func TestExecutor_MultipleIssuesWithDifferentSenderAndEphemeral(t *testing.T) {
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("issue-1", testutil.Title("Issue 1"), testutil.Sender("alice"), testutil.Ephemeral(true)).
			WithIssue("issue-2", testutil.Title("Issue 2"), testutil.Sender("bob")).
			WithIssue("issue-3", testutil.Title("Issue 3")) // No sender or ephemeral
	})
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	issues, err := executor.Execute("order by id asc")
	require.NoError(t, err)

	require.Len(t, issues, 3)

	// Build map for easier assertion
	issueMap := make(map[string]beads.Issue)
	for _, issue := range issues {
		issueMap[issue.ID] = issue
	}

	// issue-1: sender=alice, ephemeral=true
	require.Equal(t, "alice", issueMap["issue-1"].Sender)
	require.True(t, issueMap["issue-1"].Ephemeral)

	// issue-2: sender=bob, ephemeral=false (default)
	require.Equal(t, "bob", issueMap["issue-2"].Sender)
	require.False(t, issueMap["issue-2"].Ephemeral)

	// issue-3: no sender or ephemeral (defaults)
	require.Equal(t, "", issueMap["issue-3"].Sender)
	require.False(t, issueMap["issue-3"].Ephemeral)
}

// =============================================================================
// Pinned Field Tests
// =============================================================================

func TestExecutor_PinnedTrue(t *testing.T) {
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("issue-1", testutil.Title("Pinned issue"), testutil.Pinned(true))
	})
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	issues, err := executor.Execute("id = issue-1")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.NotNil(t, issues[0].Pinned, "Pinned should not be nil")
	require.True(t, *issues[0].Pinned, "Pinned should be true")
}

func TestExecutor_PinnedFalse(t *testing.T) {
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("issue-1", testutil.Title("Not pinned issue"), testutil.Pinned(false))
	})
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	issues, err := executor.Execute("id = issue-1")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.NotNil(t, issues[0].Pinned, "Pinned should not be nil when explicitly set to false")
	require.False(t, *issues[0].Pinned, "Pinned should be false")
}

func TestExecutor_PinnedNilByDefault(t *testing.T) {
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("issue-1", testutil.Title("Issue without pinned"))
	})
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	issues, err := executor.Execute("id = issue-1")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Nil(t, issues[0].Pinned, "default pinned should be nil")
}

func TestExecutor_MultipleIssuesWithDifferentPinned(t *testing.T) {
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("issue-1", testutil.Title("Issue 1"), testutil.Pinned(true)).
			WithIssue("issue-2", testutil.Title("Issue 2"), testutil.Pinned(false)).
			WithIssue("issue-3", testutil.Title("Issue 3")) // No pinned (nil)
	})
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	issues, err := executor.Execute("order by id asc")
	require.NoError(t, err)

	require.Len(t, issues, 3)

	// Build map for easier assertion
	issueMap := make(map[string]beads.Issue)
	for _, issue := range issues {
		issueMap[issue.ID] = issue
	}

	// issue-1: pinned=true
	require.NotNil(t, issueMap["issue-1"].Pinned)
	require.True(t, *issueMap["issue-1"].Pinned)

	// issue-2: pinned=false (explicitly set)
	require.NotNil(t, issueMap["issue-2"].Pinned)
	require.False(t, *issueMap["issue-2"].Pinned)

	// issue-3: pinned=nil (not set)
	require.Nil(t, issueMap["issue-3"].Pinned)
}

func TestExecutor_QueryByPinnedTrue(t *testing.T) {
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("pinned-1", testutil.Title("Pinned issue"), testutil.Pinned(true)).
			WithIssue("pinned-2", testutil.Title("Also pinned"), testutil.Pinned(true)).
			WithIssue("not-pinned", testutil.Title("Not pinned"), testutil.Pinned(false)).
			WithIssue("unset-pinned", testutil.Title("Pinned unset")) // nil
	})
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	issues, err := executor.Execute("pinned = true")
	require.NoError(t, err)

	require.Len(t, issues, 2)
	ids := make(map[string]bool)
	for _, issue := range issues {
		ids[issue.ID] = true
	}
	require.True(t, ids["pinned-1"])
	require.True(t, ids["pinned-2"])
}

func TestExecutor_QueryByPinnedFalse(t *testing.T) {
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("pinned-1", testutil.Title("Pinned issue"), testutil.Pinned(true)).
			WithIssue("not-pinned", testutil.Title("Not pinned"), testutil.Pinned(false)).
			WithIssue("unset-pinned", testutil.Title("Pinned unset")) // nil
	})
	defer func() { _ = db.Close() }()

	executor := NewExecutor(db)

	issues, err := executor.Execute("pinned = false")
	require.NoError(t, err)

	// Only explicitly false, not nil
	require.Len(t, issues, 1)
	require.Equal(t, "not-pinned", issues[0].ID)
}
