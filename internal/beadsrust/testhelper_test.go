package beadsrust

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/beads/bql"
	"github.com/zjrosen/perles/internal/cachemanager"
	"github.com/zjrosen/perles/internal/task"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

// brSchema is the canonical beads_rust schema (deduplicated from the br CLI's output).
const brSchema = `
CREATE TABLE IF NOT EXISTS "issues" (
	"id" TEXT UNIQUE,
	"content_hash" TEXT,
	"title" TEXT NOT NULL,
	"description" TEXT NOT NULL DEFAULT '',
	"design" TEXT NOT NULL DEFAULT '',
	"acceptance_criteria" TEXT NOT NULL DEFAULT '',
	"notes" TEXT NOT NULL DEFAULT '',
	"status" TEXT NOT NULL DEFAULT 'open',
	"priority" INTEGER NOT NULL DEFAULT 2,
	"issue_type" TEXT NOT NULL DEFAULT 'task',
	"assignee" TEXT,
	"owner" TEXT DEFAULT '',
	"estimated_minutes" INTEGER,
	"created_at" DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	"created_by" TEXT DEFAULT '',
	"updated_at" DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	"closed_at" DATETIME,
	"close_reason" TEXT DEFAULT '',
	"closed_by_session" TEXT DEFAULT '',
	"due_at" DATETIME,
	"defer_until" DATETIME,
	"external_ref" TEXT,
	"source_system" TEXT DEFAULT '',
	"source_repo" TEXT NOT NULL DEFAULT '.',
	"deleted_at" DATETIME,
	"deleted_by" TEXT DEFAULT '',
	"delete_reason" TEXT DEFAULT '',
	"original_type" TEXT DEFAULT '',
	"compaction_level" INTEGER DEFAULT 0,
	"compacted_at" DATETIME,
	"compacted_at_commit" TEXT,
	"original_size" INTEGER,
	"sender" TEXT DEFAULT '',
	"ephemeral" INTEGER DEFAULT 0,
	"pinned" INTEGER DEFAULT 0,
	"is_template" INTEGER DEFAULT 0
);

CREATE TABLE IF NOT EXISTS "dependencies" (
	"issue_id" TEXT NOT NULL,
	"depends_on_id" TEXT NOT NULL,
	"type" TEXT NOT NULL DEFAULT 'blocks',
	"created_at" DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	"created_by" TEXT NOT NULL DEFAULT '',
	"metadata" TEXT DEFAULT '{}',
	"thread_id" TEXT DEFAULT '',
	FOREIGN KEY("issue_id") REFERENCES "issues"("id") ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS "comments" (
	"id" TEXT PRIMARY KEY,
	"issue_id" TEXT NOT NULL,
	"author" TEXT NOT NULL,
	"text" TEXT NOT NULL,
	"created_at" DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY("issue_id") REFERENCES "issues"("id") ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS "labels" (
	"issue_id" TEXT NOT NULL,
	"label" TEXT NOT NULL,
	FOREIGN KEY("issue_id") REFERENCES "issues"("id") ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS "blocked_issues_cache" (
	"issue_id" TEXT UNIQUE,
	"blocked_by" TEXT NOT NULL,
	"blocked_at" DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY("issue_id") REFERENCES "issues"("id") ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS "config" (
	"key" TEXT NOT NULL,
	"value" TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS "metadata" (
	"key" TEXT NOT NULL,
	"value" TEXT NOT NULL
);
`

// brSeedData populates the test database with a realistic set of issues.
const brSeedData = `
INSERT INTO issues (id, title, status, priority, issue_type, assignee, owner, created_at, updated_at, sender, description, design, notes)
VALUES
	('TEST-1', 'Open bug', 'open', 0, 'bug', 'alice', 'bob', '2026-03-01T10:00:00Z', '2026-03-05T12:00:00Z', '', 'A critical bug', '', ''),
	('TEST-2', 'In-progress feature', 'in_progress', 1, 'feature', 'bob', '', '2026-03-02T10:00:00Z', '2026-03-04T12:00:00Z', 'carol', 'New auth feature', 'Auth design doc', 'Some notes'),
	('TEST-3', 'Blocked task', 'open', 2, 'task', '', '', '2026-03-01T08:00:00Z', '2026-03-03T12:00:00Z', '', 'Blocked by TEST-1', '', ''),
	('TEST-4', 'Closed chore', 'closed', 3, 'chore', 'alice', '', '2026-02-15T10:00:00Z', '2026-03-01T12:00:00Z', '', 'Old cleanup', '', ''),
	('TEST-5', 'Epic with children', 'open', 1, 'epic', 'bob', 'bob', '2026-03-01T10:00:00Z', '2026-03-05T14:00:00Z', '', 'Epic description', '', ''),
	('TEST-6', 'Child of epic', 'open', 2, 'task', '', '', '2026-03-02T10:00:00Z', '2026-03-05T15:00:00Z', '', 'Subtask', '', ''),
	('TEST-7', 'Pinned template', 'open', 2, 'task', '', '', '2026-03-03T10:00:00Z', '2026-03-05T16:00:00Z', '', 'Template issue', '', ''),
	('TEST-8', 'Deleted issue', 'deleted', 2, 'task', '', '', '2026-03-01T10:00:00Z', '2026-03-01T10:00:00Z', '', '', '', '');

-- TEST-4 is closed
UPDATE issues SET closed_at = '2026-03-01T12:00:00Z', close_reason = 'done' WHERE id = 'TEST-4';

-- TEST-7 is pinned
UPDATE issues SET pinned = 1 WHERE id = 'TEST-7';

-- TEST-8 is soft-deleted
UPDATE issues SET deleted_at = '2026-03-01T10:00:00Z' WHERE id = 'TEST-8';

-- Labels
INSERT INTO labels (issue_id, label) VALUES
	('TEST-1', 'bug'),
	('TEST-1', 'urgent'),
	('TEST-2', 'feature'),
	('TEST-5', 'epic'),
	('TEST-5', 'planning');

-- Dependencies: TEST-3 is blocked by TEST-1
INSERT INTO dependencies (issue_id, depends_on_id, type) VALUES
	('TEST-3', 'TEST-1', 'blocks');

-- Dependencies: TEST-6 is child of TEST-5
INSERT INTO dependencies (issue_id, depends_on_id, type) VALUES
	('TEST-6', 'TEST-5', 'parent-child');

-- Blocked issues cache: TEST-3 is blocked
INSERT INTO blocked_issues_cache (issue_id, blocked_by) VALUES
	('TEST-3', 'TEST-1');

-- Comments
INSERT INTO comments (id, issue_id, author, text, created_at) VALUES
	('comment-1', 'TEST-1', 'alice', 'Investigating this bug', '2026-03-02T10:00:00Z'),
	('comment-2', 'TEST-1', 'bob', 'Found the root cause', '2026-03-03T10:00:00Z'),
	('comment-3', 'TEST-2', 'carol', 'Design review done', '2026-03-03T10:00:00Z');
`

// newTestDB creates a temporary beads_rust database with schema and seed data.
// Returns the data directory path. The DB and directory are cleaned up when
// the test finishes.
func newTestDB(t *testing.T) string {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "beadsrust-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	dbPath := filepath.Join(tmpDir, "beads.db")
	db, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	// Enable WAL mode to mirror production behavior (br always uses WAL).
	_, err = db.Exec("PRAGMA journal_mode=WAL")
	require.NoError(t, err)

	_, err = db.Exec(brSchema)
	require.NoError(t, err)

	_, err = db.Exec(brSeedData)
	require.NoError(t, err)

	return tmpDir
}

// newTestReader returns a SQLiteReader connected to a fresh test database.
func newTestReader(t *testing.T) *SQLiteReader {
	t.Helper()
	dataDir := newTestDB(t)
	reader, err := NewSQLiteReader(dataDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = reader.Close() })
	return reader
}

// newTestBQLExecutor returns a BQL executor connected to a fresh test database.
func newTestBQLExecutor(t *testing.T) *BQLExecutor {
	t.Helper()
	return newTestBQLExecutorWithExtraSeed(t, "")
}

func newTestBQLExecutorWithExtraSeed(t *testing.T, seedSQL string) *BQLExecutor {
	t.Helper()

	dataDir := newTestDB(t)
	if seedSQL != "" {
		db, err := sql.Open("sqlite3", filepath.Join(dataDir, "beads.db"))
		require.NoError(t, err)

		_, err = db.Exec(seedSQL)
		require.NoError(t, err)
		require.NoError(t, db.Close())
	}

	reader, err := NewSQLiteReader(dataDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = reader.Close() })

	bqlCache := cachemanager.NewInMemoryCacheManager[string, []task.Issue](
		"test-bql-cache",
		cachemanager.DefaultExpiration,
		cachemanager.DefaultCleanupInterval,
	)
	depGraphCache := cachemanager.NewInMemoryCacheManager[string, *bql.DependencyGraph](
		"test-dep-cache",
		cachemanager.DefaultExpiration,
		cachemanager.DefaultCleanupInterval,
	)

	return NewBQLExecutor(reader.DB(), bqlCache, depGraphCache)
}

// newTestBackend returns a full BeadsRustBackend wired to a fresh test database.
func newTestBackend(t *testing.T) *BeadsRustBackend {
	t.Helper()
	dataDir := newTestDB(t)

	backend, err := NewBeadsRustBackend(dataDir, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = backend.Close() })
	return backend
}
