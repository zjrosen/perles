// Package testutil provides test utilities for database setup.
package testutil

import (
	"database/sql"
	"testing"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/stretchr/testify/require"
)

// Schema contains the unified test database schema combining all tables
// from executor_test.go and client_test.go.
const Schema = `
CREATE TABLE issues (
	id TEXT PRIMARY KEY,
	title TEXT NOT NULL,
	description TEXT,
	design TEXT NOT NULL DEFAULT '',
	acceptance_criteria TEXT NOT NULL DEFAULT '',
	notes TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'open',
	priority INTEGER NOT NULL DEFAULT 2,
	issue_type TEXT NOT NULL DEFAULT 'task',
	assignee TEXT,
	sender TEXT,
	ephemeral INTEGER,
	pinned INTEGER,
	is_template INTEGER,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	created_by TEXT DEFAULT '',
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	closed_at DATETIME,
	deleted_at DATETIME,
	hook_bead TEXT DEFAULT '',
	role_bead TEXT DEFAULT '',
	agent_state TEXT DEFAULT '',
	last_activity DATETIME,
	role_type TEXT DEFAULT '',
	rig TEXT DEFAULT '',
	mol_type TEXT DEFAULT '',
	CHECK ((status = 'closed') = (closed_at IS NOT NULL) OR status IN ('deleted', 'tombstone'))
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

CREATE TABLE comments (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	issue_id TEXT NOT NULL,
	author TEXT NOT NULL,
	text TEXT NOT NULL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (issue_id) REFERENCES issues(id)
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

// NewTestDB creates an in-memory SQLite database with the full test schema.
// The caller is responsible for closing the database.
func NewTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	_, err = db.Exec(Schema)
	require.NoError(t, err)
	return db
}
