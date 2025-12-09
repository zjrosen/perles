package testutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuilder_WithIssue(t *testing.T) {
	db := NewTestDB(t)
	defer func() { _ = db.Close() }()

	NewBuilder(t, db).
		WithIssue("issue-1").
		Build()

	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM issues`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	var id, title, status string
	var priority int
	err = db.QueryRow(`SELECT id, title, status, priority FROM issues WHERE id = ?`, "issue-1").
		Scan(&id, &title, &status, &priority)
	require.NoError(t, err)
	require.Equal(t, "issue-1", id)
	require.Equal(t, "issue-1", title) // default title is ID
	require.Equal(t, "open", status)
	require.Equal(t, 2, priority)
}

func TestBuilder_WithIssue_AllOptions(t *testing.T) {
	db := NewTestDB(t)
	defer func() { _ = db.Close() }()

	now := time.Now().Truncate(time.Second)

	NewBuilder(t, db).
		WithIssue("issue-1",
			Title("My Title"),
			Description("My Description"),
			Status("in_progress"),
			Priority(0),
			IssueType("bug"),
			Assignee("alice"),
			CreatedAt(now),
			UpdatedAt(now),
		).
		Build()

	var id, title, desc, status, issueType string
	var priority int
	var assignee *string
	err := db.QueryRow(`SELECT id, title, description, status, priority, issue_type, assignee FROM issues WHERE id = ?`, "issue-1").
		Scan(&id, &title, &desc, &status, &priority, &issueType, &assignee)
	require.NoError(t, err)
	require.Equal(t, "My Title", title)
	require.Equal(t, "My Description", desc)
	require.Equal(t, "in_progress", status)
	require.Equal(t, 0, priority)
	require.Equal(t, "bug", issueType)
	require.NotNil(t, assignee)
	require.Equal(t, "alice", *assignee)
}

func TestBuilder_WithLabels(t *testing.T) {
	db := NewTestDB(t)
	defer func() { _ = db.Close() }()

	NewBuilder(t, db).
		WithIssue("issue-1", Labels("urgent", "backend")).
		Build()

	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM labels WHERE issue_id = ?`, "issue-1").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 2, count)

	rows, err := db.Query(`SELECT label FROM labels WHERE issue_id = ? ORDER BY label`, "issue-1")
	require.NoError(t, err)
	defer rows.Close()

	var labels []string
	for rows.Next() {
		var label string
		require.NoError(t, rows.Scan(&label))
		labels = append(labels, label)
	}
	require.Equal(t, []string{"backend", "urgent"}, labels)
}

func TestBuilder_WithComments(t *testing.T) {
	db := NewTestDB(t)
	defer func() { _ = db.Close() }()

	NewBuilder(t, db).
		WithIssue("issue-1", Comments(Comment("alice", "First comment"), Comment("bob", "Second comment"))).
		Build()

	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM comments WHERE issue_id = ?`, "issue-1").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 2, count)

	rows, err := db.Query(`SELECT author, text FROM comments WHERE issue_id = ? ORDER BY id`, "issue-1")
	require.NoError(t, err)
	defer rows.Close()

	var comments []struct{ author, text string }
	for rows.Next() {
		var c struct{ author, text string }
		require.NoError(t, rows.Scan(&c.author, &c.text))
		comments = append(comments, c)
	}
	require.Len(t, comments, 2)
	require.Equal(t, "alice", comments[0].author)
	require.Equal(t, "First comment", comments[0].text)
	require.Equal(t, "bob", comments[1].author)
	require.Equal(t, "Second comment", comments[1].text)
}

func TestBuilder_WithDependency(t *testing.T) {
	db := NewTestDB(t)
	defer func() { _ = db.Close() }()

	NewBuilder(t, db).
		WithIssue("issue-1").
		WithIssue("issue-2").
		WithDependency("issue-2", "issue-1", "blocks").
		Build()

	var issueID, dependsOnID, depType string
	err := db.QueryRow(`SELECT issue_id, depends_on_id, type FROM dependencies`).
		Scan(&issueID, &dependsOnID, &depType)
	require.NoError(t, err)
	require.Equal(t, "issue-2", issueID)
	require.Equal(t, "issue-1", dependsOnID)
	require.Equal(t, "blocks", depType)
}

func TestBuilder_WithBlockedCache(t *testing.T) {
	db := NewTestDB(t)
	defer func() { _ = db.Close() }()

	NewBuilder(t, db).
		WithIssue("issue-1").
		WithBlockedCache("issue-1").
		Build()

	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM blocked_issues_cache WHERE issue_id = ?`, "issue-1").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestBuilder_InsertOrder(t *testing.T) {
	db := NewTestDB(t)
	defer func() { _ = db.Close() }()

	// This test verifies that issues are inserted before dependencies
	// (foreign key constraint would fail otherwise)
	NewBuilder(t, db).
		WithIssue("epic-1", IssueType("epic")).
		WithIssue("task-1").
		WithDependency("task-1", "epic-1", "parent-child").
		Build()

	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM dependencies`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestBuilder_ChainMethods(t *testing.T) {
	db := NewTestDB(t)
	defer func() { _ = db.Close() }()

	// Verify method chaining returns *Builder and works correctly
	builder := NewBuilder(t, db)
	result := builder.
		WithIssue("issue-1").
		WithIssue("issue-2").
		WithIssue("issue-3").
		WithDependency("issue-2", "issue-1", "blocks").
		WithBlockedCache("issue-2")

	require.Same(t, builder, result, "chained methods should return same builder")

	result.Build()

	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM issues`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 3, count)
}

func TestBuilder_MultipleIssuesWithLabels(t *testing.T) {
	db := NewTestDB(t)
	defer func() { _ = db.Close() }()

	NewBuilder(t, db).
		WithIssue("issue-1", Labels("urgent")).
		WithIssue("issue-2", Labels("backend", "api")).
		Build()

	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM labels`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 3, count) // 1 + 2 labels
}
