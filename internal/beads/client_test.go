package beads

import (
	"database/sql"
	"testing"

	"github.com/zjrosen/perles/internal/testutil"

	"github.com/stretchr/testify/require"
)

func TestNewClient_InvalidPath(t *testing.T) {
	_, err := NewClient("/nonexistent/path/that/does/not/exist")
	require.Error(t, err, "expected error for invalid path")
}

// setupDB creates an in-memory SQLite database, optionally configured via the builder.
func setupDB(t *testing.T, configure func(*testutil.Builder) *testutil.Builder) *sql.DB {
	db := testutil.NewTestDB(t)
	b := testutil.NewBuilder(t, db)
	if configure != nil {
		b = configure(b)
	}
	b.Build()
	return db
}

// newTestClient creates a Client using the provided test database.
func newTestClient(db *sql.DB) *Client {
	return &Client{db: db, dbPath: ":memory:"}
}

func TestGetComments_NoComments(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithClientTestData)
	defer func() { _ = db.Close() }()
	client := newTestClient(db)

	// issue-3 has no comments
	comments, err := client.GetComments("issue-3")
	require.NoError(t, err)
	require.Empty(t, comments, "issue with no comments should return empty slice")
}

func TestGetComments_SingleComment(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithClientTestData)
	defer func() { _ = db.Close() }()
	client := newTestClient(db)

	// issue-2 has one comment
	comments, err := client.GetComments("issue-2")
	require.NoError(t, err)
	require.Len(t, comments, 1)

	require.Equal(t, "charlie", comments[0].Author)
	require.Equal(t, "Only comment on issue-2", comments[0].Text)
	require.NotZero(t, comments[0].ID)
	require.False(t, comments[0].CreatedAt.IsZero())
}

func TestGetComments_MultipleComments(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithClientTestData)
	defer func() { _ = db.Close() }()
	client := newTestClient(db)

	// issue-1 has two comments
	comments, err := client.GetComments("issue-1")
	require.NoError(t, err)
	require.Len(t, comments, 2)

	// Verify both comments are present
	authors := []string{comments[0].Author, comments[1].Author}
	require.ElementsMatch(t, []string{"alice", "bob"}, authors)
}

func TestGetComments_OrderedByCreatedAt(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithClientTestData)
	defer func() { _ = db.Close() }()
	client := newTestClient(db)

	// issue-1 has two comments from alice and bob
	comments, err := client.GetComments("issue-1")
	require.NoError(t, err)
	require.Len(t, comments, 2)

	// Comments should be returned in order (alice inserted first, bob second)
	// When created_at is the same, SQLite stable sort preserves insertion order
	require.Equal(t, "alice", comments[0].Author, "first comment should be from alice")
	require.Equal(t, "bob", comments[1].Author, "second comment should be from bob")
	// Both comments have valid timestamps
	require.False(t, comments[0].CreatedAt.IsZero())
	require.False(t, comments[1].CreatedAt.IsZero())
}

func TestGetComments_NonExistentIssue(t *testing.T) {
	db := setupDB(t, (*testutil.Builder).WithClientTestData)
	defer func() { _ = db.Close() }()
	client := newTestClient(db)

	// Non-existent issue should return empty slice (not error)
	comments, err := client.GetComments("nonexistent-issue")
	require.NoError(t, err)
	require.Empty(t, comments, "non-existent issue should return empty slice")
}

func TestClient_Close(t *testing.T) {
	db := setupDB(t, nil)
	client := newTestClient(db)

	// Close should succeed
	err := client.Close()
	require.NoError(t, err, "Close should succeed")
}

func TestClient_DB(t *testing.T) {
	db := setupDB(t, nil)
	defer func() { _ = db.Close() }()
	client := newTestClient(db)

	// DB should return the underlying database
	returnedDB := client.DB()
	require.NotNil(t, returnedDB, "DB should return non-nil database")
	require.Same(t, db, returnedDB, "DB should return the same database instance")
}
