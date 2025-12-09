package testutil

import "time"

// WithStandardTestData adds the standard test dataset.
// Mirrors executor_test.go setupTestDB.
func (b *Builder) WithStandardTestData() *Builder {
	now := time.Now()
	yesterday := now.Add(-24 * time.Hour)
	lastWeek := now.Add(-7 * 24 * time.Hour)

	return b.
		WithIssue("test-1",
			Title("Fix login bug"), Description("Login fails for users"),
			Status("open"), Priority(0), IssueType("bug"), Assignee("alice"),
			Labels("urgent", "auth"), CreatedAt(lastWeek), UpdatedAt(now)).
		WithIssue("test-2",
			Title("Add search feature"), Description("Search functionality"),
			Status("open"), Priority(1), IssueType("feature"),
			CreatedAt(yesterday), UpdatedAt(yesterday)).
		WithIssue("test-3",
			Title("Refactor auth"), Description("Clean up auth code"),
			Status("in_progress"), Priority(2), IssueType("task"), Assignee("bob"),
			Labels("auth"), CreatedAt(lastWeek), UpdatedAt(yesterday)).
		WithIssue("test-4",
			Title("Update docs"), Description("Documentation update"),
			Status("closed"), Priority(3), IssueType("chore"),
			CreatedAt(lastWeek), UpdatedAt(lastWeek)).
		WithIssue("test-5",
			Title("Critical security fix"), Description("Urgent fix needed"),
			Status("open"), Priority(0), IssueType("bug"), Assignee("alice"),
			Labels("urgent", "security"), CreatedAt(now), UpdatedAt(now)).
		WithIssue("test-6",
			Title("Epic: New dashboard"), Description("Dashboard epic"),
			Status("open"), Priority(1), IssueType("epic"),
			CreatedAt(lastWeek), UpdatedAt(now)).
		WithDependency("test-3", "test-1", "blocks").
		WithDependency("test-2", "test-6", "parent-child").
		WithBlockedCache("test-3")
}

// WithHierarchyTestData adds hierarchical test data.
// Mirrors executor_test.go setupExpandTestDB.
//
// Structure:
//
//	epic-1 (epic)
//	  ├── task-1 (child)
//	  │     └── subtask-1 (grandchild)
//	  └── task-2 (child)
//
//	blocker-1 blocks blocked-1 blocks blocked-2
func (b *Builder) WithHierarchyTestData() *Builder {
	return b.
		WithIssue("epic-1", Title("Epic One"), IssueType("epic")).
		WithIssue("task-1", Title("Task One"), IssueType("task")).
		WithIssue("task-2", Title("Task Two"), IssueType("task")).
		WithIssue("subtask-1", Title("Subtask One"), IssueType("task")).
		WithIssue("blocker-1", Title("Blocker One"), IssueType("bug")).
		WithIssue("blocked-1", Title("Blocked One"), IssueType("task")).
		WithIssue("blocked-2", Title("Blocked Two"), IssueType("task")).
		WithIssue("standalone", Title("Standalone Issue"), IssueType("task")).
		WithDependency("task-1", "epic-1", "parent-child").
		WithDependency("task-2", "epic-1", "parent-child").
		WithDependency("subtask-1", "task-1", "parent-child").
		WithDependency("blocked-1", "blocker-1", "blocks").
		WithDependency("blocked-2", "blocked-1", "blocks")
}

// WithClientTestData adds client test data.
// Mirrors beads/client_test.go setupTestDB.
func (b *Builder) WithClientTestData() *Builder {
	now := time.Now()
	yesterday := now.Add(-24 * time.Hour)

	return b.
		WithIssue("issue-1",
			Title("First issue"), Description("Description 1"),
			Status("open"), Priority(0), IssueType("bug"),
			Labels("urgent", "backend"),
			Comments(Comment("alice", "First comment on issue-1"), Comment("bob", "Second comment on issue-1")),
			CreatedAt(yesterday), UpdatedAt(now)).
		WithIssue("issue-2",
			Title("Second issue"), Description("Description 2"),
			Status("open"), Priority(1), IssueType("feature"),
			Labels("frontend"),
			Comments(Comment("charlie", "Only comment on issue-2")),
			CreatedAt(yesterday), UpdatedAt(yesterday)).
		WithIssue("issue-3",
			Title("Third issue"), Description(""),
			Status("in_progress"), Priority(2), IssueType("task"),
			CreatedAt(yesterday), UpdatedAt(yesterday)).
		WithIssue("issue-4",
			Title("Deleted issue"), Description("Should not appear"),
			Status("deleted"), Priority(0), IssueType("bug"),
			CreatedAt(yesterday), UpdatedAt(now)).
		WithIssue("epic-1",
			Title("Epic with children"), Description("An epic"),
			Status("open"), Priority(1), IssueType("epic"),
			CreatedAt(yesterday), UpdatedAt(now)).
		WithDependency("issue-3", "issue-1", "blocks").
		WithDependency("issue-2", "epic-1", "parent-child")
}
