package beadsrust

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/task"
)

const deferredIssueSeed = `
		INSERT INTO issues (id, title, status, priority, issue_type, defer_until, created_at, updated_at)
		VALUES
			('TEST-9', 'Future deferred task', 'open', 2, 'task', '2099-01-01T00:00:00Z', '2026-03-01T10:00:00Z', '2026-03-05T12:00:00Z'),
			('TEST-10', 'Past deferred task', 'open', 2, 'task', '2000-01-01T00:00:00Z', '2026-03-01T10:00:00Z', '2026-03-05T12:00:00Z')
`

func TestBQLExecutor_EmptyQuery(t *testing.T) {
	exec := newTestBQLExecutor(t)

	// An empty input produces a Query with nil Filter and no ORDER BY,
	// which is valid and returns all non-deleted issues.
	issues, err := exec.Execute("")
	require.NoError(t, err)
	require.NotNil(t, issues)

	// Seed data: 7 non-deleted issues (TEST-1 through TEST-7; TEST-8 is deleted).
	require.Len(t, issues, 7, "should return all non-deleted issues")
}

func TestBQLExecutor_StatusFilter_Open(t *testing.T) {
	exec := newTestBQLExecutor(t)

	issues, err := exec.Execute("status = open")
	require.NoError(t, err)

	// Open issues: TEST-1, TEST-3, TEST-5, TEST-6, TEST-7.
	require.Len(t, issues, 5)
	for _, issue := range issues {
		require.Equal(t, task.Status("open"), issue.Status)
	}
}

func TestBQLExecutor_StatusFilter_InProgress(t *testing.T) {
	exec := newTestBQLExecutor(t)

	issues, err := exec.Execute("status = in_progress")
	require.NoError(t, err)

	// Only TEST-2 is in_progress.
	require.Len(t, issues, 1)
	require.Equal(t, "TEST-2", issues[0].ID)
}

func TestBQLExecutor_TypeFilter(t *testing.T) {
	exec := newTestBQLExecutor(t)

	issues, err := exec.Execute("type = task")
	require.NoError(t, err)

	// Tasks: TEST-3, TEST-6, TEST-7 (TEST-8 is deleted, excluded).
	require.Len(t, issues, 3)
	for _, issue := range issues {
		require.Equal(t, task.IssueType("task"), issue.Type)
	}
}

func TestBQLExecutor_PriorityFilter(t *testing.T) {
	exec := newTestBQLExecutor(t)

	issues, err := exec.Execute("priority = P2")
	require.NoError(t, err)

	// P2 issues (non-deleted): TEST-3, TEST-6, TEST-7.
	require.Len(t, issues, 3)
	for _, issue := range issues {
		require.Equal(t, task.Priority(2), issue.Priority)
	}
}

func TestBQLExecutor_TitleContains(t *testing.T) {
	exec := newTestBQLExecutor(t)

	issues, err := exec.Execute("title ~ bug")
	require.NoError(t, err)

	// TEST-1 has title "Open bug".
	require.Len(t, issues, 1)
	require.Equal(t, "TEST-1", issues[0].ID)
}

func TestBQLExecutor_TitleContains_NoResults(t *testing.T) {
	exec := newTestBQLExecutor(t)

	issues, err := exec.Execute("title ~ nonexistentxyz123")
	require.NoError(t, err)
	require.Empty(t, issues)
}

func TestBQLExecutor_CompoundQuery(t *testing.T) {
	exec := newTestBQLExecutor(t)

	issues, err := exec.Execute("type = task and status != closed")
	require.NoError(t, err)

	// Tasks that are not closed (TEST-3, TEST-6, TEST-7).
	require.Len(t, issues, 3)
	for _, issue := range issues {
		require.Equal(t, task.IssueType("task"), issue.Type)
		require.NotEqual(t, task.Status("closed"), issue.Status)
	}
}

func TestBQLExecutor_OrderBy(t *testing.T) {
	exec := newTestBQLExecutor(t)

	issues, err := exec.Execute("status = open order by priority asc")
	require.NoError(t, err)

	// Open issues sorted by priority ascending.
	require.NotEmpty(t, issues)
	for i := 1; i < len(issues); i++ {
		require.LessOrEqual(t, int(issues[i-1].Priority), int(issues[i].Priority))
	}
}

func TestBQLExecutor_InvalidField(t *testing.T) {
	exec := newTestBQLExecutor(t)

	// Beads agent fields should be rejected.
	_, err := exec.Execute("hook_bead = test")
	require.Error(t, err)
	require.Contains(t, err.Error(), "validation error")
	require.Contains(t, err.Error(), "unknown field")
}

func TestBQLExecutor_ReadyFilter_True(t *testing.T) {
	exec := newTestBQLExecutorWithExtraSeed(t, deferredIssueSeed)

	issues, err := exec.Execute("ready = true")
	require.NoError(t, err)

	// Ready = open AND not blocked.
	// Open: TEST-1, TEST-3, TEST-5, TEST-6, TEST-7
	// Blocked: TEST-3 (in blocked_issues_cache)
	// Ready: TEST-1, TEST-5, TEST-6, TEST-7
	// Note: TEST-2 is in_progress, not open, so not "ready".
	require.Len(t, issues, 5, "expected 5 ready issues")

	ids := make(map[string]bool)
	for _, issue := range issues {
		ids[issue.ID] = true
		require.Equal(t, task.Status("open"), issue.Status,
			"ready issue %s should have status=open", issue.ID)
	}
	require.True(t, ids["TEST-1"])
	require.True(t, ids["TEST-5"])
	require.True(t, ids["TEST-6"])
	require.True(t, ids["TEST-7"])
	require.True(t, ids["TEST-10"], "past-deferred issue should be ready again")
	require.False(t, ids["TEST-3"], "TEST-3 is blocked, should not be ready")
	require.False(t, ids["TEST-9"], "future-deferred issue should not be ready")
}

func TestBQLExecutor_ReadyFilter_False(t *testing.T) {
	exec := newTestBQLExecutorWithExtraSeed(t, deferredIssueSeed)

	issues, err := exec.Execute("ready = false")
	require.NoError(t, err)

	// Not-ready = NOT (open AND not-blocked).
	// This includes: closed, in_progress, and blocked issues.
	// TEST-2 (in_progress), TEST-3 (blocked), TEST-4 (closed), TEST-9 (future-deferred).
	require.Len(t, issues, 4)

	ids := make(map[string]bool)
	for _, issue := range issues {
		ids[issue.ID] = true
	}
	require.True(t, ids["TEST-9"], "future-deferred issue should be not-ready")
	require.False(t, ids["TEST-10"], "past-deferred issue should be ready again")
}

func TestBQLExecutor_DeferUntilFilter(t *testing.T) {
	exec := newTestBQLExecutorWithExtraSeed(t, deferredIssueSeed)

	issues, err := exec.Execute("status = open and defer_until > now")
	require.NoError(t, err)
	require.Len(t, issues, 1)
	require.Equal(t, "TEST-9", issues[0].ID)
}

func TestBQLExecutor_BlockedFilter(t *testing.T) {
	exec := newTestBQLExecutor(t)

	issues, err := exec.Execute("blocked = true")
	require.NoError(t, err)

	// Only TEST-3 is in blocked_issues_cache.
	require.Len(t, issues, 1)
	require.Equal(t, "TEST-3", issues[0].ID)
}

func TestBQLExecutor_BlockedFilter_False(t *testing.T) {
	exec := newTestBQLExecutor(t)

	issues, err := exec.Execute("blocked = false")
	require.NoError(t, err)

	// All non-deleted except TEST-3 (7 total - 1 blocked = 6).
	require.Len(t, issues, 6)
	for _, issue := range issues {
		require.NotEqual(t, "TEST-3", issue.ID)
	}
}

func TestBQLExecutor_LabelFilter(t *testing.T) {
	exec := newTestBQLExecutor(t)

	issues, err := exec.Execute("label = bug")
	require.NoError(t, err)

	// TEST-1 has label "bug".
	require.Len(t, issues, 1)
	require.Equal(t, "TEST-1", issues[0].ID)
}

func TestBQLExecutor_LabelFilter_NoResults(t *testing.T) {
	exec := newTestBQLExecutor(t)

	issues, err := exec.Execute("label = nonexistent_label_xyz")
	require.NoError(t, err)
	require.Empty(t, issues)
}

func TestBQLExecutor_OwnerFilter(t *testing.T) {
	exec := newTestBQLExecutor(t)

	// TEST-1 has owner "bob", TEST-5 has owner "bob".
	issues, err := exec.Execute("owner = bob")
	require.NoError(t, err)
	require.Len(t, issues, 2)

	ids := make(map[string]bool)
	for _, issue := range issues {
		ids[issue.ID] = true
	}
	require.True(t, ids["TEST-1"])
	require.True(t, ids["TEST-5"])
}

func TestBQLExecutor_SenderFilter(t *testing.T) {
	exec := newTestBQLExecutor(t)

	// TEST-2 has sender "carol".
	issues, err := exec.Execute("sender = carol")
	require.NoError(t, err)
	require.Len(t, issues, 1)
	require.Equal(t, "TEST-2", issues[0].ID)
}

func TestBQLExecutor_DateFilter(t *testing.T) {
	exec := newTestBQLExecutor(t)

	// All test issues are created within the last year.
	issues, err := exec.Execute("created > -365d")
	require.NoError(t, err)
	require.NotEmpty(t, issues)
}

func TestBQLExecutor_ExpandDown(t *testing.T) {
	exec := newTestBQLExecutor(t)

	// TEST-5 is an epic, TEST-6 is its child (parent-child dep).
	issues, err := exec.Execute("type = epic expand down depth 2")
	require.NoError(t, err)

	ids := make(map[string]bool)
	for _, issue := range issues {
		ids[issue.ID] = true
	}
	require.True(t, ids["TEST-5"], "base issue TEST-5 should be present")
	require.True(t, ids["TEST-6"], "child TEST-6 should be expanded")
}

func TestBQLExecutor_BatchLoadLabels(t *testing.T) {
	exec := newTestBQLExecutor(t)

	issues, err := exec.Execute("status != deleted")
	require.NoError(t, err)
	require.NotEmpty(t, issues)

	// Build map by ID for easy lookup.
	issueMap := make(map[string]task.Issue)
	for _, issue := range issues {
		issueMap[issue.ID] = issue
	}

	// TEST-1 should have labels "bug" and "urgent".
	test1 := issueMap["TEST-1"]
	require.Contains(t, test1.Labels, "bug")
	require.Contains(t, test1.Labels, "urgent")

	// TEST-2 should have label "feature".
	test2 := issueMap["TEST-2"]
	require.Contains(t, test2.Labels, "feature")

	// TEST-5 should have labels "epic" and "planning".
	test5 := issueMap["TEST-5"]
	require.Contains(t, test5.Labels, "epic")
	require.Contains(t, test5.Labels, "planning")
}

func TestBQLExecutor_BatchLoadDeps(t *testing.T) {
	exec := newTestBQLExecutor(t)

	issues, err := exec.Execute("status != deleted")
	require.NoError(t, err)

	issueMap := make(map[string]task.Issue)
	for _, issue := range issues {
		issueMap[issue.ID] = issue
	}

	// TEST-3 is blocked by TEST-1.
	test3 := issueMap["TEST-3"]
	require.Contains(t, test3.BlockedBy, "TEST-1")

	// TEST-1 should block TEST-3.
	test1 := issueMap["TEST-1"]
	require.Contains(t, test1.Blocks, "TEST-3")

	// TEST-6 has parent TEST-5.
	test6 := issueMap["TEST-6"]
	require.Equal(t, "TEST-5", test6.ParentID)

	// TEST-5 has child TEST-6.
	test5 := issueMap["TEST-5"]
	require.Contains(t, test5.Children, "TEST-6")
}

func TestBQLExecutor_BatchLoadCommentCounts(t *testing.T) {
	exec := newTestBQLExecutor(t)

	issues, err := exec.Execute("status != deleted")
	require.NoError(t, err)

	issueMap := make(map[string]task.Issue)
	for _, issue := range issues {
		issueMap[issue.ID] = issue
	}

	// TEST-1 has 2 comments.
	require.Equal(t, 2, issueMap["TEST-1"].CommentCount)

	// TEST-2 has 1 comment.
	require.Equal(t, 1, issueMap["TEST-2"].CommentCount)

	// TEST-3 has 0 comments.
	require.Equal(t, 0, issueMap["TEST-3"].CommentCount)
}

func TestBQLExecutor_Caching(t *testing.T) {
	exec := newTestBQLExecutor(t)

	// First call populates cache.
	issues1, err := exec.Execute("status = open")
	require.NoError(t, err)

	// Second call should return cached results (same content).
	issues2, err := exec.Execute("status = open")
	require.NoError(t, err)

	require.Equal(t, len(issues1), len(issues2))
}

func TestBQLExecutor_PinnedFilter(t *testing.T) {
	exec := newTestBQLExecutor(t)

	issues, err := exec.Execute("pinned = true")
	require.NoError(t, err)

	// TEST-7 is pinned.
	require.Len(t, issues, 1)
	require.Equal(t, "TEST-7", issues[0].ID)
}

func TestBQLExecutor_AssigneeFilter(t *testing.T) {
	exec := newTestBQLExecutor(t)

	issues, err := exec.Execute("assignee = alice")
	require.NoError(t, err)

	// TEST-1 and TEST-4 have assignee "alice".
	require.Len(t, issues, 2)
	ids := make(map[string]bool)
	for _, issue := range issues {
		ids[issue.ID] = true
	}
	require.True(t, ids["TEST-1"])
	require.True(t, ids["TEST-4"])
}

func TestBQLExecutor_IDFilter(t *testing.T) {
	exec := newTestBQLExecutor(t)

	issues, err := exec.Execute("id = TEST-1")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	require.Equal(t, "TEST-1", issues[0].ID)
	require.Equal(t, "Open bug", issues[0].TitleText)
}

func TestBQLExecutor_InFilter(t *testing.T) {
	exec := newTestBQLExecutor(t)

	issues, err := exec.Execute("id in (TEST-1, TEST-5)")
	require.NoError(t, err)

	require.Len(t, issues, 2)
	ids := make(map[string]bool)
	for _, issue := range issues {
		ids[issue.ID] = true
	}
	require.True(t, ids["TEST-1"])
	require.True(t, ids["TEST-5"])
}

func TestBQLExecutor_Extensions(t *testing.T) {
	exec := newTestBQLExecutor(t)

	issues, err := exec.Execute("id = TEST-7")
	require.NoError(t, err)

	require.Len(t, issues, 1)
	// TEST-7 has pinned=1, should be in extensions.
	require.NotNil(t, issues[0].Extensions)
	require.Equal(t, true, issues[0].Extensions["pinned"])
}

func TestBRQueryExecutor_ViaBQL(t *testing.T) {
	backend := newTestBackend(t)
	services := backend.Services()

	// Run a BQL query through the full stack.
	issues, err := services.QueryExecutor.Execute("status = open")
	require.NoError(t, err)

	require.Len(t, issues, 5)
	for _, issue := range issues {
		require.Equal(t, task.Status("open"), issue.Status)
	}
}
