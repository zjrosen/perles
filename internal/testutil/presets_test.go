package testutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPreset_StandardTestData(t *testing.T) {
	db := NewTestDB(t)
	defer func() { _ = db.Close() }()

	NewBuilder(t, db).WithStandardTestData().Build()

	// Verify 6 issues
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM issues`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 6, count, "expected 6 issues")

	// Verify issue IDs
	rows, err := db.Query(`SELECT id FROM issues ORDER BY id`)
	require.NoError(t, err)
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.Equal(t, []string{"test-1", "test-2", "test-3", "test-4", "test-5", "test-6"}, ids)

	// Verify labels
	err = db.QueryRow(`SELECT COUNT(*) FROM labels`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 5, count, "expected 5 labels")

	// Verify dependencies
	err = db.QueryRow(`SELECT COUNT(*) FROM dependencies`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 2, count, "expected 2 dependencies")

	// Verify blocked cache
	err = db.QueryRow(`SELECT COUNT(*) FROM blocked_issues_cache`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count, "expected 1 blocked issue")

	// Verify specific issue attributes
	var title, status, issueType string
	var priority int
	var assignee *string
	err = db.QueryRow(`SELECT title, status, priority, issue_type, assignee FROM issues WHERE id = ?`, "test-1").
		Scan(&title, &status, &priority, &issueType, &assignee)
	require.NoError(t, err)
	require.Equal(t, "Fix login bug", title)
	require.Equal(t, "open", status)
	require.Equal(t, 0, priority)
	require.Equal(t, "bug", issueType)
	require.NotNil(t, assignee)
	require.Equal(t, "alice", *assignee)
}

func TestPreset_HierarchyTestData(t *testing.T) {
	db := NewTestDB(t)
	defer func() { _ = db.Close() }()

	NewBuilder(t, db).WithHierarchyTestData().Build()

	// Verify 8 issues
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM issues`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 8, count, "expected 8 issues")

	// Verify parent-child dependencies (3)
	err = db.QueryRow(`SELECT COUNT(*) FROM dependencies WHERE type = ?`, "parent-child").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 3, count, "expected 3 parent-child dependencies")

	// Verify blocks dependencies (2)
	err = db.QueryRow(`SELECT COUNT(*) FROM dependencies WHERE type = ?`, "blocks").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 2, count, "expected 2 blocks dependencies")

	// Verify epic-1 type
	var issueType string
	err = db.QueryRow(`SELECT issue_type FROM issues WHERE id = ?`, "epic-1").Scan(&issueType)
	require.NoError(t, err)
	require.Equal(t, "epic", issueType)

	// Verify hierarchy: task-1 is child of epic-1
	var dependsOnID string
	err = db.QueryRow(`SELECT depends_on_id FROM dependencies WHERE issue_id = ? AND type = ?`, "task-1", "parent-child").
		Scan(&dependsOnID)
	require.NoError(t, err)
	require.Equal(t, "epic-1", dependsOnID)

	// Verify hierarchy: subtask-1 is child of task-1
	err = db.QueryRow(`SELECT depends_on_id FROM dependencies WHERE issue_id = ? AND type = ?`, "subtask-1", "parent-child").
		Scan(&dependsOnID)
	require.NoError(t, err)
	require.Equal(t, "task-1", dependsOnID)
}

func TestPreset_ClientTestData(t *testing.T) {
	db := NewTestDB(t)
	defer func() { _ = db.Close() }()

	NewBuilder(t, db).WithClientTestData().Build()

	// Verify 5 issues
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM issues`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 5, count, "expected 5 issues")

	// Verify comments exist
	err = db.QueryRow(`SELECT COUNT(*) FROM comments`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 3, count, "expected 3 comments")

	// Verify issue-1 has 2 comments
	err = db.QueryRow(`SELECT COUNT(*) FROM comments WHERE issue_id = ?`, "issue-1").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 2, count, "expected 2 comments on issue-1")

	// Verify issue-2 has 1 comment
	err = db.QueryRow(`SELECT COUNT(*) FROM comments WHERE issue_id = ?`, "issue-2").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count, "expected 1 comment on issue-2")

	// Verify labels
	err = db.QueryRow(`SELECT COUNT(*) FROM labels`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 3, count, "expected 3 labels")

	// Verify dependencies
	err = db.QueryRow(`SELECT COUNT(*) FROM dependencies`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 2, count, "expected 2 dependencies")

	// Verify issue-4 has status 'deleted'
	var status string
	err = db.QueryRow(`SELECT status FROM issues WHERE id = ?`, "issue-4").Scan(&status)
	require.NoError(t, err)
	require.Equal(t, "deleted", status)
}
