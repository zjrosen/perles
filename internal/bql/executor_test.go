package bql

import (
	"database/sql"
	"testing"
	"time"

	"github.com/zjrosen/perles/internal/beads"
	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/testutil"

	"github.com/stretchr/testify/mock"
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

// newTestExecutor creates an executor with mock caches for testing.
// Uses testing.TB interface to work with both *testing.T and *testing.B.
func newTestExecutor(tb testing.TB, db *sql.DB) *Executor {
	bqlCache := mocks.NewMockCacheManager[string, []beads.Issue](tb)
	bqlCache.On("Get", mock.Anything, mock.Anything).Return(nil, false).Maybe()
	bqlCache.On("GetWithRefresh", mock.Anything, mock.Anything, mock.Anything).Return(nil, false).Maybe()
	bqlCache.On("Set", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return().Maybe()

	depGraphCache := mocks.NewMockCacheManager[string, *DependencyGraph](tb)
	depGraphCache.On("Get", mock.Anything, mock.Anything).Return(nil, false).Maybe()
	depGraphCache.On("GetWithRefresh", mock.Anything, mock.Anything, mock.Anything).Return(nil, false).Maybe()
	depGraphCache.On("Set", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return().Maybe()

	return NewExecutor(db, bqlCache, depGraphCache)
}

func TestExecutor_TypeFilter(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

	// Blocked issues
	issues, err := executor.Execute("blocked = true")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Equal(t, "test-3", issues[0].ID)
}

func TestExecutor_ReadyFilter(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

	// Title contains "bug"
	issues, err := executor.Execute("title ~ bug")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Equal(t, "test-1", issues[0].ID)
}

func TestExecutor_OrderBy(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

	// No P4 priority issues exist
	issues, err := executor.Execute("priority = P4")
	require.NoError(t, err)

	require.Empty(t, issues)
}

func TestExecutor_InvalidQuery(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)

	// Invalid field
	_, err := executor.Execute("foo = bar")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown field")
}

func TestExecutor_ParseError(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)

	// Invalid syntax
	_, err := executor.Execute("type = = bug")
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse error")
}

func TestExecutor_LoadsLabels(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

	// Get test-3 which is blocked by test-1
	issues, err := executor.Execute("id = test-3")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Contains(t, issues[0].BlockedBy, "test-1")
}

func TestExecutor_LoadsBlocks(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)

	// Get test-1 which blocks test-3
	issues, err := executor.Execute("id = test-1")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Contains(t, issues[0].Blocks, "test-3", "test-1 should show test-3 in Blocks")
}

func TestExecutor_LoadsChildrenForEpicWithChildren(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)

	// Get test-6 (epic) which has test-2 as child via parent-child dependency
	issues, err := executor.Execute("id = test-6")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Contains(t, issues[0].Children, "test-2", "epic should show child in Blocks")
}

func TestExecutor_LoadsRelated(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithDiscoveredFromTestData)
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

	// Get test-1 which has assignee "alice"
	issues, err := executor.Execute("id = test-1")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Equal(t, "alice", issues[0].Assignee, "assignee should be populated from database")
}

func TestExecutor_AssigneeNullIsEmptyString(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)

	// Get test-2 which has NULL assignee
	issues, err := executor.Execute("id = test-2")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Equal(t, "", issues[0].Assignee, "NULL assignee should be empty string")
}

func TestExecutor_MultipleIssuesWithDifferentAssignees(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

	// Query with no results + expand should return empty
	issues, err := executor.Execute("id = nonexistent expand down")
	require.NoError(t, err)
	require.Empty(t, issues)
}

func TestExecutor_ExpandWithOrderBy(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithHierarchyTestData)
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

	// standalone has no relationships - expand should return only the matched issue
	issues, err := executor.Execute("id = standalone expand down")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Equal(t, "standalone", issues[0].ID)
}

func TestExecutor_ExpandNoRelationshipsWithUnlimitedDepth(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithHierarchyTestData)
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

	// Query for non-existent IDs should return empty slice, no error
	issues, err := executor.Execute(`id in ("nonexistent-1", "nonexistent-2")`)
	require.NoError(t, err)
	require.Empty(t, issues)
}

func TestExecutor_IDIn_Mixed(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

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

	executor := newTestExecutor(t, db)

	issues, err := executor.Execute("pinned = false")
	require.NoError(t, err)

	// Only explicitly false, not nil
	require.Len(t, issues, 1)
	require.Equal(t, "not-pinned", issues[0].ID)
}

// =============================================================================
// IsTemplate Field Tests
// =============================================================================

func TestExecutor_IsTemplateTrue(t *testing.T) {
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("issue-1", testutil.Title("Template issue"), testutil.IsTemplate(true))
	})
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)

	issues, err := executor.Execute("id = issue-1")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.NotNil(t, issues[0].IsTemplate, "IsTemplate should not be nil")
	require.True(t, *issues[0].IsTemplate, "IsTemplate should be true")
}

func TestExecutor_IsTemplateFalse(t *testing.T) {
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("issue-1", testutil.Title("Not template issue"), testutil.IsTemplate(false))
	})
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)

	issues, err := executor.Execute("id = issue-1")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.NotNil(t, issues[0].IsTemplate, "IsTemplate should not be nil when explicitly set to false")
	require.False(t, *issues[0].IsTemplate, "IsTemplate should be false")
}

func TestExecutor_IsTemplateNilByDefault(t *testing.T) {
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("issue-1", testutil.Title("Issue without is_template"))
	})
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)

	issues, err := executor.Execute("id = issue-1")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Nil(t, issues[0].IsTemplate, "default is_template should be nil")
}

func TestExecutor_MultipleIssuesWithDifferentIsTemplate(t *testing.T) {
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("issue-1", testutil.Title("Issue 1"), testutil.IsTemplate(true)).
			WithIssue("issue-2", testutil.Title("Issue 2"), testutil.IsTemplate(false)).
			WithIssue("issue-3", testutil.Title("Issue 3")) // No is_template (nil)
	})
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)

	issues, err := executor.Execute("order by id asc")
	require.NoError(t, err)

	require.Len(t, issues, 3)

	// Build map for easier assertion
	issueMap := make(map[string]beads.Issue)
	for _, issue := range issues {
		issueMap[issue.ID] = issue
	}

	// issue-1: is_template=true
	require.NotNil(t, issueMap["issue-1"].IsTemplate)
	require.True(t, *issueMap["issue-1"].IsTemplate)

	// issue-2: is_template=false (explicitly set)
	require.NotNil(t, issueMap["issue-2"].IsTemplate)
	require.False(t, *issueMap["issue-2"].IsTemplate)

	// issue-3: is_template=nil (not set)
	require.Nil(t, issueMap["issue-3"].IsTemplate)
}

func TestExecutor_QueryByIsTemplateTrue(t *testing.T) {
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("template-1", testutil.Title("Template issue"), testutil.IsTemplate(true)).
			WithIssue("template-2", testutil.Title("Also template"), testutil.IsTemplate(true)).
			WithIssue("not-template", testutil.Title("Not template"), testutil.IsTemplate(false)).
			WithIssue("unset-template", testutil.Title("IsTemplate unset")) // nil
	})
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)

	issues, err := executor.Execute("is_template = true")
	require.NoError(t, err)

	require.Len(t, issues, 2)
	ids := make(map[string]bool)
	for _, issue := range issues {
		ids[issue.ID] = true
	}
	require.True(t, ids["template-1"])
	require.True(t, ids["template-2"])
}

func TestExecutor_QueryByIsTemplateFalse(t *testing.T) {
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("template-1", testutil.Title("Template issue"), testutil.IsTemplate(true)).
			WithIssue("not-template", testutil.Title("Not template"), testutil.IsTemplate(false)).
			WithIssue("unset-template", testutil.Title("IsTemplate unset")) // nil
	})
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)

	issues, err := executor.Execute("is_template = false")
	require.NoError(t, err)

	// Only explicitly false, not nil
	require.Len(t, issues, 1)
	require.Equal(t, "not-template", issues[0].ID)
}

// =============================================================================
// CreatedBy Field Tests
// =============================================================================

func TestExecutor_CreatedByPopulated(t *testing.T) {
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("issue-1", testutil.Title("Issue with creator"), testutil.CreatedBy("alice"))
	})
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)

	issues, err := executor.Execute("id = issue-1")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Equal(t, "alice", issues[0].CreatedBy)
}

func TestExecutor_CreatedByEmptyByDefault(t *testing.T) {
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("issue-1", testutil.Title("Issue without creator"))
	})
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)

	issues, err := executor.Execute("id = issue-1")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Equal(t, "", issues[0].CreatedBy, "default created_by should be empty string")
}

// =============================================================================
// Agent Field Tests
// =============================================================================

func TestExecutor_AgentFieldsPopulated(t *testing.T) {
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("agent-1",
				testutil.Title("Agent issue"),
				testutil.HookBead("task-123"),
				testutil.RoleBead("role-def-1"),
				testutil.AgentState("running"),
				testutil.RoleType("polecat"),
				testutil.Rig("rig-alpha"),
			)
	})
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)

	issues, err := executor.Execute("id = agent-1")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	issue := issues[0]
	require.Equal(t, "task-123", issue.HookBead)
	require.Equal(t, "role-def-1", issue.RoleBead)
	require.Equal(t, "running", issue.AgentState)
	require.Equal(t, "polecat", issue.RoleType)
	require.Equal(t, "rig-alpha", issue.Rig)
}

func TestExecutor_AgentFieldsEmptyByDefault(t *testing.T) {
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("issue-1", testutil.Title("Regular issue"))
	})
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)

	issues, err := executor.Execute("id = issue-1")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	issue := issues[0]
	require.Equal(t, "", issue.HookBead, "default hook_bead should be empty")
	require.Equal(t, "", issue.RoleBead, "default role_bead should be empty")
	require.Equal(t, "", issue.AgentState, "default agent_state should be empty")
	require.Equal(t, "", issue.RoleType, "default role_type should be empty")
	require.Equal(t, "", issue.Rig, "default rig should be empty")
	require.True(t, issue.LastActivity.IsZero(), "default last_activity should be zero time")
}

func TestExecutor_QueryByAgentState(t *testing.T) {
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("agent-running", testutil.Title("Running agent"), testutil.AgentState("running")).
			WithIssue("agent-idle", testutil.Title("Idle agent"), testutil.AgentState("idle")).
			WithIssue("regular-issue", testutil.Title("Regular issue"))
	})
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)

	issues, err := executor.Execute("agent_state = running")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Equal(t, "agent-running", issues[0].ID)
}

func TestExecutor_QueryByRoleType(t *testing.T) {
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("polecat-agent", testutil.Title("Polecat agent"), testutil.RoleType("polecat")).
			WithIssue("crew-agent", testutil.Title("Crew agent"), testutil.RoleType("crew")).
			WithIssue("regular-issue", testutil.Title("Regular issue"))
	})
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)

	issues, err := executor.Execute("role_type = polecat")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Equal(t, "polecat-agent", issues[0].ID)
}

func TestExecutor_QueryByRig(t *testing.T) {
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("alpha-agent", testutil.Title("Alpha rig agent"), testutil.Rig("rig-alpha")).
			WithIssue("beta-agent", testutil.Title("Beta rig agent"), testutil.Rig("rig-beta")).
			WithIssue("town-agent", testutil.Title("Town agent")) // empty rig
	})
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)

	issues, err := executor.Execute("rig = rig-alpha")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Equal(t, "alpha-agent", issues[0].ID)
}

// =============================================================================
// Benchmark Helper Functions
// =============================================================================

// setupLargeHierarchy creates a hierarchical issue tree for benchmarking.
// It creates a tree with the specified total count of issues across the given depth.
// The structure fans out at each level to reach the target count.
//
// Parameters:
//   - t: *testing.T for regular tests
//   - count: total number of issues to create
//   - depth: maximum depth of the hierarchy (1 = flat, 2 = one level of children, etc.)
//
// Returns the database with the hierarchy created. Root issue is "root".
func setupLargeHierarchy(t *testing.T, count, depth int) *sql.DB {
	t.Helper()
	return setupLargeHierarchyInternal(t, count, depth)
}

// setupLargeHierarchyB is the benchmark version of setupLargeHierarchy.
func setupLargeHierarchyB(b *testing.B, count, depth int) *sql.DB {
	b.Helper()
	return setupLargeHierarchyInternal(b, count, depth)
}

// setupLargeHierarchyInternal is the shared implementation for creating hierarchies.
func setupLargeHierarchyInternal(tb testing.TB, count, depth int) *sql.DB {
	tb.Helper()

	// Create db directly without testutil to avoid *testing.T requirement
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		tb.Fatalf("failed to open db: %v", err)
	}
	if _, err := db.Exec(testutil.Schema); err != nil {
		tb.Fatalf("failed to create schema: %v", err)
	}

	if count <= 0 || depth <= 0 {
		return db
	}

	// Calculate fan-out per level to achieve approximately 'count' total issues
	// For a balanced tree: count ≈ 1 + fanout + fanout^2 + ... + fanout^(depth-1)
	// Simplified: use fixed fanout based on count and depth
	fanout := 1
	if depth > 1 {
		// Estimate fanout: count ≈ fanout^depth, so fanout ≈ count^(1/depth)
		for f := 2; ; f++ {
			total := 1
			power := 1
			for d := 1; d < depth; d++ {
				power *= f
				total += power
			}
			if total >= count {
				fanout = f
				break
			}
			if f > 100 {
				fanout = f
				break
			}
		}
	}

	// Insert root issue
	now := time.Now()
	_, err = db.Exec(`INSERT INTO issues (id, title, status, priority, issue_type, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, "root", "Root Epic", "open", 2, "epic", now, now)
	if err != nil {
		tb.Fatalf("failed to insert root: %v", err)
	}

	issueCount := 1
	currentLevel := []string{"root"}

	// Build tree level by level
	for d := 1; d < depth && issueCount < count; d++ {
		nextLevel := []string{}
		for _, parentID := range currentLevel {
			for f := 0; f < fanout && issueCount < count; f++ {
				childID := parentID + "-" + string(rune('a'+f))
				_, err = db.Exec(`INSERT INTO issues (id, title, status, priority, issue_type, created_at, updated_at)
					VALUES (?, ?, ?, ?, ?, ?, ?)`,
					childID, "Issue "+childID, "open", d%5, "task", now, now)
				if err != nil {
					tb.Fatalf("failed to insert issue %s: %v", childID, err)
				}
				_, err = db.Exec(`INSERT INTO dependencies (issue_id, depends_on_id, type) VALUES (?, ?, ?)`,
					childID, parentID, "parent-child")
				if err != nil {
					tb.Fatalf("failed to insert dependency for %s: %v", childID, err)
				}
				nextLevel = append(nextLevel, childID)
				issueCount++
			}
		}
		currentLevel = nextLevel
	}

	return db
}

// setupRealisticData creates a realistic kanban-style dataset for benchmarking.
// It creates issues distributed across different statuses similar to a real project:
//   - ~10% blocked
//   - ~20% ready (open, not blocked)
//   - ~30% in_progress
//   - ~40% closed
//
// Issues have realistic dependencies, labels, and relationships.
//
// Parameters:
//   - t: *testing.T for regular tests
//   - count: total number of issues to create
//
// Returns the database with the realistic data created.
func setupRealisticData(t *testing.T, count int) *sql.DB {
	t.Helper()
	return setupRealisticDataInternal(t, count)
}

// setupRealisticDataB is the benchmark version of setupRealisticData.
func setupRealisticDataB(b *testing.B, count int) *sql.DB {
	b.Helper()
	return setupRealisticDataInternal(b, count)
}

// setupRealisticDataInternal is the shared implementation for creating realistic data.
func setupRealisticDataInternal(tb testing.TB, count int) *sql.DB {
	tb.Helper()

	// Create db directly without testutil to avoid *testing.T requirement
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		tb.Fatalf("failed to open db: %v", err)
	}
	if _, err := db.Exec(testutil.Schema); err != nil {
		tb.Fatalf("failed to create schema: %v", err)
	}

	if count <= 0 {
		return db
	}

	// Distribution: 10% blocked, 20% ready, 30% in_progress, 40% closed
	blockedCount := count / 10
	readyCount := count / 5
	inProgressCount := (count * 3) / 10
	closedCount := count - blockedCount - readyCount - inProgressCount

	now := time.Now()
	issueNum := 0
	blockerIDs := []string{}

	// Create closed issues first (40%)
	for i := 0; i < closedCount; i++ {
		id := "closed-" + formatNum(issueNum)
		insertBenchmarkIssue(tb, db, id, "Closed Issue "+formatNum(issueNum), "closed",
			issueNum%5, issueTypes[issueNum%len(issueTypes)], now)
		insertBenchmarkLabels(tb, db, id, labels[issueNum%len(labels)])
		issueNum++
	}

	// Create in_progress issues (30%)
	for i := 0; i < inProgressCount; i++ {
		id := "progress-" + formatNum(issueNum)
		insertBenchmarkIssue(tb, db, id, "In Progress Issue "+formatNum(issueNum), "in_progress",
			issueNum%5, issueTypes[issueNum%len(issueTypes)], now)
		insertBenchmarkLabels(tb, db, id, labels[issueNum%len(labels)])
		issueNum++
	}

	// Create ready issues (20%) - these will be blockers for blocked issues
	for i := 0; i < readyCount; i++ {
		id := "ready-" + formatNum(issueNum)
		insertBenchmarkIssue(tb, db, id, "Ready Issue "+formatNum(issueNum), "open",
			issueNum%5, issueTypes[issueNum%len(issueTypes)], now)
		insertBenchmarkLabels(tb, db, id, labels[issueNum%len(labels)])
		blockerIDs = append(blockerIDs, id)
		issueNum++
	}

	// Create blocked issues (10%) - blocked by ready issues
	for i := 0; i < blockedCount; i++ {
		id := "blocked-" + formatNum(issueNum)
		insertBenchmarkIssue(tb, db, id, "Blocked Issue "+formatNum(issueNum), "open",
			issueNum%5, issueTypes[issueNum%len(issueTypes)], now)
		insertBenchmarkLabels(tb, db, id, labels[issueNum%len(labels)])
		// Block by a ready issue
		if len(blockerIDs) > 0 {
			blockerID := blockerIDs[i%len(blockerIDs)]
			_, err := db.Exec(`INSERT INTO dependencies (issue_id, depends_on_id, type) VALUES (?, ?, ?)`,
				id, blockerID, "blocks")
			if err != nil {
				tb.Fatalf("failed to insert dependency: %v", err)
			}
			_, err = db.Exec(`INSERT INTO blocked_issues_cache (issue_id) VALUES (?)`, id)
			if err != nil {
				tb.Fatalf("failed to insert blocked cache: %v", err)
			}
		}
		issueNum++
	}

	return db
}

// insertBenchmarkIssue inserts an issue directly for benchmark tests.
func insertBenchmarkIssue(tb testing.TB, db *sql.DB, id, title, status string, priority int, issueType string, now time.Time) {
	var closedAt interface{}
	if status == "closed" {
		closedAt = now
	}
	_, err := db.Exec(`INSERT INTO issues (id, title, status, priority, issue_type, created_at, updated_at, closed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, title, status, priority, issueType, now, now, closedAt)
	if err != nil {
		tb.Fatalf("failed to insert issue %s: %v", id, err)
	}
}

// insertBenchmarkLabels inserts labels directly for benchmark tests.
func insertBenchmarkLabels(tb testing.TB, db *sql.DB, issueID string, labels []string) {
	for _, label := range labels {
		_, err := db.Exec(`INSERT INTO labels (issue_id, label) VALUES (?, ?)`, issueID, label)
		if err != nil {
			tb.Fatalf("failed to insert label for %s: %v", issueID, err)
		}
	}
}

// formatNum formats a number with leading zeros for consistent sorting.
func formatNum(n int) string {
	return string(rune('0'+n/100)) + string(rune('0'+(n%100)/10)) + string(rune('0'+n%10))
}

// issueTypes for realistic data distribution
var issueTypes = []string{"bug", "feature", "task", "chore"}

// labels for realistic data distribution
var labels = [][]string{
	{"urgent", "backend"},
	{"frontend"},
	{"urgent", "security"},
	{"documentation"},
	{"refactor"},
	{"performance"},
	{"api"},
	{"ui", "ux"},
}

// =============================================================================
// Benchmark Tests
// =============================================================================

// BenchmarkExecute_ExpandDownDepthUnlimited measures the performance of expanding
// a deep hierarchy with unlimited depth. This is one of the most expensive operations
// in the BQL executor and is a primary optimization target.
//
// Baseline measurements (Apple M3 Max, 2025-12-29):
//   - 100 issues, depth 5: ~7.2 ms/op, 558 KB/op, 7397 allocs/op
//
// Final results after all optimizations (Apple M3 Max, 2025-12-29):
//   - 100 issues, depth 5: ~1.18 ms/op, 584 KB/op, 9052 allocs/op
//   - Improvement: 6.1x faster (graph-based expand replaces O(D×N) SQL with O(2) queries)
//   - Note: Memory per-op slightly higher due to graph structure, but total query
//     count dramatically reduced from hundreds of SQL queries to just 2
func BenchmarkExecute_ExpandDownDepthUnlimited(b *testing.B) {
	db := setupLargeHierarchyB(b, 100, 5)
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(b, db)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := executor.Execute("id = root expand down depth *")
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkExecute_KanbanBoard measures the performance of loading a typical
// kanban board with 4 columns (blocked, ready, in_progress, closed).
// This simulates the common UI pattern of loading multiple column queries.
//
// Baseline measurements (Apple M3 Max, 2025-12-29):
//   - 500 issues, 4 columns: ~55 ms/op, 2.3 MB/op, 38765 allocs/op
//
// Final results after all optimizations (Apple M3 Max, 2025-12-29):
//   - 500 issues, 4 columns: ~5.3 ms/op, 2.65 MB/op, 39793 allocs/op
//   - Improvement: 10.4x faster (batch dependency loading reduces subqueries)
//   - With caching enabled: ~1.17 µs/op on cache hit (4700x faster)
func BenchmarkExecute_KanbanBoard(b *testing.B) {
	db := setupRealisticDataB(b, 500)
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(b, db)
	queries := []string{
		"blocked = true order by priority",
		"ready = true order by priority",
		"status = in_progress order by updated",
		"status = closed order by updated",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, q := range queries {
			_, err := executor.Execute(q)
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}

// BenchmarkExecute_BaseQueryWithDependencies measures the performance of a
// base query that returns issues with dependencies loaded. This tests the
// correlated subquery performance for loading dependency information.
//
// Baseline measurements (Apple M3 Max, 2025-12-29):
//   - 50 result rows with dependencies: ~3.2 ms/op, 185 KB/op, 3392 allocs/op
//
// Final results after all optimizations (Apple M3 Max, 2025-12-29):
//   - 50 result rows with dependencies: ~0.65 ms/op, 258 KB/op, 5578 allocs/op
//   - Improvement: 4.9x faster (batch IN-clause queries replace correlated subqueries)
func BenchmarkExecute_BaseQueryWithDependencies(b *testing.B) {
	// Create 50 issues with various dependencies
	db := setupBaseQueryBenchmarkDataB(b)
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(b, db)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := executor.Execute("status = open order by priority")
		if err != nil {
			b.Fatal(err)
		}
	}
}

// setupBaseQueryBenchmarkDataB creates 50 issues with interconnected dependencies for benchmarking.
func setupBaseQueryBenchmarkDataB(b *testing.B) *sql.DB {
	b.Helper()

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		b.Fatalf("failed to open db: %v", err)
	}
	if _, err := db.Exec(testutil.Schema); err != nil {
		b.Fatalf("failed to create schema: %v", err)
	}

	now := time.Now()

	// Create 50 issues with interconnected dependencies
	for i := 0; i < 50; i++ {
		id := "issue-" + formatNum(i)
		insertBenchmarkIssue(b, db, id, "Issue "+formatNum(i), "open",
			i%5, issueTypes[i%len(issueTypes)], now)
		insertBenchmarkLabels(b, db, id, labels[i%len(labels)])
	}

	// Add blocking dependencies (each issue blocked by previous)
	for i := 1; i < 50; i++ {
		_, err := db.Exec(`INSERT INTO dependencies (issue_id, depends_on_id, type) VALUES (?, ?, ?)`,
			"issue-"+formatNum(i), "issue-"+formatNum(i-1), "blocks")
		if err != nil {
			b.Fatalf("failed to insert dependency: %v", err)
		}
	}

	// Add parent-child dependencies (create 5 epics with 9 children each)
	for epic := 0; epic < 5; epic++ {
		epicID := "issue-" + formatNum(epic*10)
		for child := 1; child < 10; child++ {
			childID := "issue-" + formatNum(epic*10+child)
			_, err := db.Exec(`INSERT INTO dependencies (issue_id, depends_on_id, type) VALUES (?, ?, ?)`,
				childID, epicID, "parent-child")
			if err != nil {
				b.Fatalf("failed to insert dependency: %v", err)
			}
		}
	}

	return db
}

// =============================================================================
// Helper Function Unit Tests
// =============================================================================

func TestSetupLargeHierarchy_CreatesCorrectStructure(t *testing.T) {
	db := setupLargeHierarchy(t, 100, 5)
	defer func() { _ = db.Close() }()

	// Verify root exists
	var rootCount int
	err := db.QueryRow(`SELECT COUNT(*) FROM issues WHERE id = 'root'`).Scan(&rootCount)
	require.NoError(t, err)
	require.Equal(t, 1, rootCount, "root issue should exist")

	// Verify total issue count is close to target
	var totalCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM issues`).Scan(&totalCount)
	require.NoError(t, err)
	require.GreaterOrEqual(t, totalCount, 50, "should have at least 50 issues")
	require.LessOrEqual(t, totalCount, 150, "should have at most 150 issues")

	// Verify parent-child relationships exist
	var depCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM dependencies WHERE type = 'parent-child'`).Scan(&depCount)
	require.NoError(t, err)
	require.Greater(t, depCount, 0, "should have parent-child dependencies")

	// Verify hierarchy depth by checking that root has children
	var childCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM dependencies WHERE depends_on_id = 'root'`).Scan(&childCount)
	require.NoError(t, err)
	require.Greater(t, childCount, 0, "root should have children")
}

func TestSetupLargeHierarchy_HandlesEdgeCases(t *testing.T) {
	// Test with zero count
	db := setupLargeHierarchy(t, 0, 5)
	defer func() { _ = db.Close() }()
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM issues`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count, "zero count should create no issues")

	// Test with zero depth
	db2 := setupLargeHierarchy(t, 100, 0)
	defer func() { _ = db2.Close() }()
	err = db2.QueryRow(`SELECT COUNT(*) FROM issues`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count, "zero depth should create no issues")
}

func TestSetupRealisticData_CreatesExpectedDistribution(t *testing.T) {
	db := setupRealisticData(t, 100)
	defer func() { _ = db.Close() }()

	// Verify total count
	var totalCount int
	err := db.QueryRow(`SELECT COUNT(*) FROM issues`).Scan(&totalCount)
	require.NoError(t, err)
	require.Equal(t, 100, totalCount, "should have exactly 100 issues")

	// Verify status distribution (allowing for rounding)
	var closedCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM issues WHERE status = 'closed'`).Scan(&closedCount)
	require.NoError(t, err)
	require.GreaterOrEqual(t, closedCount, 35, "should have ~40% closed")
	require.LessOrEqual(t, closedCount, 45, "should have ~40% closed")

	var inProgressCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM issues WHERE status = 'in_progress'`).Scan(&inProgressCount)
	require.NoError(t, err)
	require.GreaterOrEqual(t, inProgressCount, 25, "should have ~30% in_progress")
	require.LessOrEqual(t, inProgressCount, 35, "should have ~30% in_progress")

	var openCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM issues WHERE status = 'open'`).Scan(&openCount)
	require.NoError(t, err)
	require.GreaterOrEqual(t, openCount, 25, "should have ~30% open (ready + blocked)")
	require.LessOrEqual(t, openCount, 35, "should have ~30% open (ready + blocked)")

	// Verify blocked issues have dependencies
	var blockedWithDeps int
	err = db.QueryRow(`
		SELECT COUNT(DISTINCT bc.issue_id)
		FROM blocked_issues_cache bc
		JOIN dependencies d ON d.issue_id = bc.issue_id
	`).Scan(&blockedWithDeps)
	require.NoError(t, err)
	require.Greater(t, blockedWithDeps, 0, "blocked issues should have dependencies")
}

func TestSetupRealisticData_HandlesEdgeCases(t *testing.T) {
	// Test with zero count
	db := setupRealisticData(t, 0)
	defer func() { _ = db.Close() }()
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM issues`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count, "zero count should create no issues")

	// Test with small count
	db2 := setupRealisticData(t, 10)
	defer func() { _ = db2.Close() }()
	err = db2.QueryRow(`SELECT COUNT(*) FROM issues`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 10, count, "small count should work")
}

// =============================================================================
// Graph-Based Expand Unit Tests
// =============================================================================

func TestLoadDependencyGraph_ReturnsCorrectAdjacencyLists(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithHierarchyTestData)
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)
	graph, err := executor.loadDependencyGraph()
	require.NoError(t, err)
	require.NotNil(t, graph)

	// Verify forward edges exist (child -> parent direction)
	// task-1 depends on epic-1 (parent-child)
	require.NotEmpty(t, graph.Forward["task-1"], "task-1 should have forward edges")
	foundParent := false
	for _, edge := range graph.Forward["task-1"] {
		if edge.TargetID == "epic-1" && edge.Type == "parent-child" {
			foundParent = true
			break
		}
	}
	require.True(t, foundParent, "task-1 should have edge to epic-1")

	// Verify reverse edges exist (parent -> children direction)
	require.NotEmpty(t, graph.Reverse["epic-1"], "epic-1 should have reverse edges")
	childCount := 0
	for _, edge := range graph.Reverse["epic-1"] {
		if edge.Type == "parent-child" {
			childCount++
		}
	}
	require.Equal(t, 2, childCount, "epic-1 should have 2 children (task-1 and task-2)")
}

func TestLoadDependencyGraph_ExcludesDeletedIssues(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithHierarchyTestData)
	defer func() { _ = db.Close() }()

	// Add a deleted issue with a dependency
	_, err := db.Exec(`INSERT INTO issues (id, title, status, priority, issue_type) VALUES ('deleted-1', 'Deleted', 'deleted', 1, 'task')`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO dependencies (issue_id, depends_on_id, type) VALUES ('deleted-1', 'epic-1', 'parent-child')`)
	require.NoError(t, err)

	executor := newTestExecutor(t, db)
	graph, err := executor.loadDependencyGraph()
	require.NoError(t, err)

	// Verify deleted issue is not in the graph
	require.Empty(t, graph.Forward["deleted-1"], "deleted issue should not have forward edges")

	// Verify epic-1's reverse edges don't include deleted issue
	for _, edge := range graph.Reverse["epic-1"] {
		require.NotEqual(t, "deleted-1", edge.TargetID, "deleted issue should not be in reverse edges")
	}
}

func TestTraverseGraph_BFSRespectsDepthLimit(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithHierarchyTestData)
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)
	graph, err := executor.loadDependencyGraph()
	require.NoError(t, err)

	// Traverse down from epic-1 with depth 1 (should only get children, not grandchildren)
	allIDs := executor.traverseGraph(graph, []string{"epic-1"}, ExpandDown, 1)
	idSet := make(map[string]bool)
	for _, id := range allIDs {
		idSet[id] = true
	}

	require.True(t, idSet["epic-1"], "should include starting node")
	require.True(t, idSet["task-1"], "should include child at depth 1")
	require.True(t, idSet["task-2"], "should include child at depth 1")
	require.False(t, idSet["subtask-1"], "should NOT include grandchild at depth 1")
}

func TestTraverseGraph_DFSHandlesUnlimitedDepth(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithHierarchyTestData)
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)
	graph, err := executor.loadDependencyGraph()
	require.NoError(t, err)

	// Traverse down from epic-1 with unlimited depth (should get all descendants)
	allIDs := executor.traverseGraph(graph, []string{"epic-1"}, ExpandDown, int(DepthUnlimited))
	idSet := make(map[string]bool)
	for _, id := range allIDs {
		idSet[id] = true
	}

	require.True(t, idSet["epic-1"], "should include starting node")
	require.True(t, idSet["task-1"], "should include child")
	require.True(t, idSet["task-2"], "should include child")
	require.True(t, idSet["subtask-1"], "should include grandchild with unlimited depth")
}

func TestTraverseGraph_HandlesCyclesWithoutInfiniteLoop(t *testing.T) {
	db := setupDB(t, nil)
	defer func() { _ = db.Close() }()

	// Create circular dependency: A blocks B, B blocks A
	now := time.Now()
	_, err := db.Exec(`INSERT INTO issues (id, title, status, priority, issue_type, created_at, updated_at) VALUES ('circular-a', 'Circular A', 'open', 1, 'task', ?, ?)`, now, now)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO issues (id, title, status, priority, issue_type, created_at, updated_at) VALUES ('circular-b', 'Circular B', 'open', 1, 'task', ?, ?)`, now, now)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO dependencies (issue_id, depends_on_id, type) VALUES ('circular-b', 'circular-a', 'blocks')`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO dependencies (issue_id, depends_on_id, type) VALUES ('circular-a', 'circular-b', 'blocks')`)
	require.NoError(t, err)

	executor := newTestExecutor(t, db)
	graph, err := executor.loadDependencyGraph()
	require.NoError(t, err)

	// Should complete without infinite loop
	allIDs := executor.traverseGraph(graph, []string{"circular-a"}, ExpandAll, int(DepthUnlimited))

	require.Len(t, allIDs, 2, "should return both issues without infinite loop")
	idSet := make(map[string]bool)
	for _, id := range allIDs {
		idSet[id] = true
	}
	require.True(t, idSet["circular-a"])
	require.True(t, idSet["circular-b"])
}

func TestFetchIssuesByIDs_ReturnsCorrectIssuesInBatch(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithHierarchyTestData)
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)
	issues, err := executor.fetchIssuesByIDs([]string{"task-1", "task-2", "subtask-1"})
	require.NoError(t, err)

	require.Len(t, issues, 3)
	idSet := make(map[string]bool)
	for _, issue := range issues {
		idSet[issue.ID] = true
	}
	require.True(t, idSet["task-1"])
	require.True(t, idSet["task-2"])
	require.True(t, idSet["subtask-1"])
}

func TestFetchIssuesByIDs_ExcludesDeletedIssues(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithHierarchyTestData)
	defer func() { _ = db.Close() }()

	// Add a deleted issue
	_, err := db.Exec(`INSERT INTO issues (id, title, status, priority, issue_type) VALUES ('deleted-1', 'Deleted', 'deleted', 1, 'task')`)
	require.NoError(t, err)

	executor := newTestExecutor(t, db)
	issues, err := executor.fetchIssuesByIDs([]string{"task-1", "deleted-1"})
	require.NoError(t, err)

	// Should only return task-1, not the deleted issue
	require.Len(t, issues, 1)
	require.Equal(t, "task-1", issues[0].ID)
}

func TestFetchIssuesByIDs_EmptyInput(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithHierarchyTestData)
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)
	issues, err := executor.fetchIssuesByIDs([]string{})
	require.NoError(t, err)
	require.Nil(t, issues)
}

func TestTraverseGraph_EmptyGraph(t *testing.T) {
	db := setupDB(t, nil) // Empty database
	defer func() { _ = db.Close() }()

	// Add a single issue with no dependencies
	now := time.Now()
	_, err := db.Exec(`INSERT INTO issues (id, title, status, priority, issue_type, created_at, updated_at) VALUES ('lonely', 'Lonely', 'open', 1, 'task', ?, ?)`, now, now)
	require.NoError(t, err)

	executor := newTestExecutor(t, db)
	graph, err := executor.loadDependencyGraph()
	require.NoError(t, err)

	// Graph should be empty
	require.Empty(t, graph.Forward)
	require.Empty(t, graph.Reverse)

	// Traversal should return only the starting node
	allIDs := executor.traverseGraph(graph, []string{"lonely"}, ExpandDown, int(DepthUnlimited))
	require.Len(t, allIDs, 1)
	require.Equal(t, "lonely", allIDs[0])
}

func TestTraverseGraph_SingleNodeWithNoEdges(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithHierarchyTestData)
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)
	graph, err := executor.loadDependencyGraph()
	require.NoError(t, err)

	// standalone has no dependencies
	allIDs := executor.traverseGraph(graph, []string{"standalone"}, ExpandAll, int(DepthUnlimited))
	require.Len(t, allIDs, 1)
	require.Equal(t, "standalone", allIDs[0])
}

// =============================================================================
// Batch Loading Tests (for perles-ezi8.3)
// =============================================================================

func TestLoadDependenciesForIssues_GroupsByTypeCorrectly(t *testing.T) {
	// Setup data with multiple dependency types
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			// Parent-child: epic -> task
			WithIssue("epic-1", testutil.Title("Epic"), testutil.IssueType("epic")).
			WithIssue("task-1", testutil.Title("Task 1"), testutil.IssueType("task")).
			// Blocks: blocker blocks blocked
			WithIssue("blocker", testutil.Title("Blocker"), testutil.IssueType("bug")).
			WithIssue("blocked", testutil.Title("Blocked"), testutil.IssueType("task")).
			// Discovered-from: discovered <- origin
			WithIssue("origin", testutil.Title("Origin"), testutil.IssueType("feature")).
			WithIssue("discovered", testutil.Title("Discovered"), testutil.IssueType("bug")).
			WithDependency("task-1", "epic-1", "parent-child").
			WithDependency("blocked", "blocker", "blocks").
			WithDependency("discovered", "origin", "discovered-from")
	})
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)
	deps, err := executor.loadDependenciesForIssues([]string{"epic-1", "task-1", "blocker", "blocked", "origin", "discovered"})
	require.NoError(t, err)

	// Check parent-child grouping
	require.Equal(t, "epic-1", deps["task-1"].ParentID, "task-1 should have epic-1 as parent")
	require.Contains(t, deps["epic-1"].Children, "task-1", "epic-1 should have task-1 as child")

	// Check blocks grouping
	require.Contains(t, deps["blocked"].BlockedBy, "blocker", "blocked should have blocker in BlockedBy")
	require.Contains(t, deps["blocker"].Blocks, "blocked", "blocker should have blocked in Blocks")

	// Check discovered-from grouping
	require.Contains(t, deps["discovered"].DiscoveredFrom, "origin", "discovered should have origin in DiscoveredFrom")
	require.Contains(t, deps["origin"].Discovered, "discovered", "origin should have discovered in Discovered")
}

func TestLoadDependenciesForIssues_HandlesBothDirections(t *testing.T) {
	// Test both directions of issue_id and depends_on_id mapping
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("parent", testutil.Title("Parent"), testutil.IssueType("epic")).
			WithIssue("child", testutil.Title("Child"), testutil.IssueType("task")).
			WithDependency("child", "parent", "parent-child")
	})
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)

	// When querying for parent only
	depsParent, err := executor.loadDependenciesForIssues([]string{"parent"})
	require.NoError(t, err)
	require.Contains(t, depsParent["parent"].Children, "child", "parent should see child even when child not in query set")

	// When querying for child only
	depsChild, err := executor.loadDependenciesForIssues([]string{"child"})
	require.NoError(t, err)
	require.Equal(t, "parent", depsChild["child"].ParentID, "child should see parent even when parent not in query set")

	// When querying for both
	depsBoth, err := executor.loadDependenciesForIssues([]string{"parent", "child"})
	require.NoError(t, err)
	require.Contains(t, depsBoth["parent"].Children, "child")
	require.Equal(t, "parent", depsBoth["child"].ParentID)
}

func TestLoadLabelsForIssues_ReturnsCorrectLabelSets(t *testing.T) {
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("issue-1", testutil.Title("Issue 1"), testutil.Labels("urgent", "backend", "security")).
			WithIssue("issue-2", testutil.Title("Issue 2"), testutil.Labels("frontend")).
			WithIssue("issue-3", testutil.Title("Issue 3")) // No labels
	})
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)
	labels, err := executor.loadLabelsForIssues([]string{"issue-1", "issue-2", "issue-3"})
	require.NoError(t, err)

	// Check issue-1 has all 3 labels
	require.Len(t, labels["issue-1"], 3)
	require.Contains(t, labels["issue-1"], "urgent")
	require.Contains(t, labels["issue-1"], "backend")
	require.Contains(t, labels["issue-1"], "security")

	// Check issue-2 has 1 label
	require.Len(t, labels["issue-2"], 1)
	require.Contains(t, labels["issue-2"], "frontend")

	// Check issue-3 has no labels (not in map or empty)
	require.Empty(t, labels["issue-3"])
}

func TestLoadCommentCountsForIssues_ReturnsCorrectCounts(t *testing.T) {
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("issue-1", testutil.Title("Issue 1"),
				testutil.Comments(
					testutil.Comment("alice", "First comment"),
					testutil.Comment("bob", "Second comment"),
					testutil.Comment("charlie", "Third comment"),
				)).
			WithIssue("issue-2", testutil.Title("Issue 2"),
									testutil.Comments(testutil.Comment("alice", "Single comment"))).
			WithIssue("issue-3", testutil.Title("Issue 3")) // No comments
	})
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)
	counts, err := executor.loadCommentCountsForIssues([]string{"issue-1", "issue-2", "issue-3"})
	require.NoError(t, err)

	require.Equal(t, 3, counts["issue-1"], "issue-1 should have 3 comments")
	require.Equal(t, 1, counts["issue-2"], "issue-2 should have 1 comment")
	require.Equal(t, 0, counts["issue-3"], "issue-3 should have 0 comments")
}

func TestBatchLoading_EmptyIDList(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)

	// All batch loading functions should return empty maps for empty input
	deps, err := executor.loadDependenciesForIssues([]string{})
	require.NoError(t, err)
	require.Empty(t, deps)

	labels, err := executor.loadLabelsForIssues([]string{})
	require.NoError(t, err)
	require.Empty(t, labels)

	counts, err := executor.loadCommentCountsForIssues([]string{})
	require.NoError(t, err)
	require.Empty(t, counts)
}

func TestBatchLoading_SingleID(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)

	// test-1 has labels: urgent, auth
	labels, err := executor.loadLabelsForIssues([]string{"test-1"})
	require.NoError(t, err)
	require.Len(t, labels["test-1"], 2)
	require.Contains(t, labels["test-1"], "urgent")
	require.Contains(t, labels["test-1"], "auth")

	// test-1 blocks test-3
	deps, err := executor.loadDependenciesForIssues([]string{"test-1"})
	require.NoError(t, err)
	require.Contains(t, deps["test-1"].Blocks, "test-3")
}

func TestBatchLoading_IssuesWithNoDependencies(t *testing.T) {
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("standalone-1", testutil.Title("Standalone 1")).
			WithIssue("standalone-2", testutil.Title("Standalone 2"))
	})
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)

	deps, err := executor.loadDependenciesForIssues([]string{"standalone-1", "standalone-2"})
	require.NoError(t, err)

	// Both issues should have empty or no dependencies
	require.Empty(t, deps["standalone-1"].ParentID)
	require.Empty(t, deps["standalone-1"].Children)
	require.Empty(t, deps["standalone-1"].BlockedBy)
	require.Empty(t, deps["standalone-1"].Blocks)
}

func TestBatchLoading_IssuesWithNoLabels(t *testing.T) {
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("no-labels", testutil.Title("No Labels Issue"))
	})
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)

	labels, err := executor.loadLabelsForIssues([]string{"no-labels"})
	require.NoError(t, err)
	require.Empty(t, labels["no-labels"])
}

func TestBatchLoading_IssuesWithNoComments(t *testing.T) {
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("no-comments", testutil.Title("No Comments Issue"))
	})
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)

	counts, err := executor.loadCommentCountsForIssues([]string{"no-comments"})
	require.NoError(t, err)
	require.Equal(t, 0, counts["no-comments"])
}

func TestBatchLoading_IntegrationFullQueryEquivalence(t *testing.T) {
	// This test verifies that batch loading produces identical results to
	// what was expected from the old correlated subquery approach
	db := setupDB(t, (*testutil.Builder).WithStandardTestData)
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)

	// Get test-1 which has known data: labels (urgent, auth), blocks (test-3)
	issues, err := executor.Execute("id = test-1")
	require.NoError(t, err)
	require.Len(t, issues, 1)

	issue := issues[0]
	require.Equal(t, "test-1", issue.ID)
	require.Contains(t, issue.Labels, "urgent")
	require.Contains(t, issue.Labels, "auth")
	require.Contains(t, issue.Blocks, "test-3")

	// Get test-3 which is blocked by test-1
	issues, err = executor.Execute("id = test-3")
	require.NoError(t, err)
	require.Len(t, issues, 1)

	issue = issues[0]
	require.Equal(t, "test-3", issue.ID)
	require.Contains(t, issue.BlockedBy, "test-1")

	// Get test-6 (epic) which has test-2 as child
	issues, err = executor.Execute("id = test-6")
	require.NoError(t, err)
	require.Len(t, issues, 1)

	issue = issues[0]
	require.Equal(t, "test-6", issue.ID)
	require.Contains(t, issue.Children, "test-2")

	// Get test-2 which has test-6 as parent
	issues, err = executor.Execute("id = test-2")
	require.NoError(t, err)
	require.Len(t, issues, 1)

	issue = issues[0]
	require.Equal(t, "test-2", issue.ID)
	require.Equal(t, "test-6", issue.ParentID)
}
