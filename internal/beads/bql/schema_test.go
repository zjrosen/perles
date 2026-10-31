package bql

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	beads "github.com/zjrosen/perles/internal/beads/domain"
	"github.com/zjrosen/perles/internal/testutil"

	"github.com/stretchr/testify/require"
)

// currentDoltTestSchema mirrors a current beads Dolt schema (migrations 0046+)
// for the columns Perles reads: issues.is_blocked, the newer issues columns,
// the split dependency target columns, and the custom status/type tables.
const currentDoltTestSchema = `
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
	owner TEXT DEFAULT '',
	sender TEXT,
	ephemeral INTEGER,
	pinned INTEGER,
	is_template INTEGER,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	created_by TEXT DEFAULT '',
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	started_at DATETIME,
	closed_at DATETIME,
	due_at DATETIME,
	defer_until DATETIME,
	close_reason TEXT DEFAULT '',
	mol_type TEXT DEFAULT '',
	metadata TEXT DEFAULT '{}',
	is_blocked INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE labels (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	issue_id TEXT NOT NULL,
	label TEXT NOT NULL
);

CREATE TABLE dependencies (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	issue_id TEXT NOT NULL,
	depends_on_issue_id TEXT,
	depends_on_wisp_id TEXT,
	depends_on_external TEXT,
	type TEXT NOT NULL DEFAULT 'blocks'
);

CREATE TABLE comments (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	issue_id TEXT NOT NULL,
	author TEXT NOT NULL,
	text TEXT NOT NULL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE custom_statuses (
	name TEXT PRIMARY KEY,
	category TEXT NOT NULL DEFAULT 'unspecified'
);

CREATE TABLE custom_types (
	name TEXT PRIMARY KEY
);
`

// preIsBlockedDoltTestSchema is a beads Dolt schema between migrations 0041
// and 0045: split dependency targets and custom statuses, but no is_blocked.
var preIsBlockedDoltTestSchema = strings.Replace(currentDoltTestSchema,
	",\n\tis_blocked INTEGER NOT NULL DEFAULT 0", "", 1)

func execSQL(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	_, err := db.Exec(query, args...)
	require.NoError(t, err)
}

// insertCurrentIssue inserts an issue into a current-schema DB with optional
// column overrides (column name -> value).
func insertCurrentIssue(t *testing.T, db *sql.DB, id, status, issueType string, extra map[string]any) {
	t.Helper()
	now := time.Now().UTC()
	columns := []string{"id", "title", "status", "priority", "issue_type", "created_at", "updated_at"}
	values := []any{id, id, status, 2, issueType, now, now}
	if status == "closed" {
		columns = append(columns, "closed_at")
		values = append(values, now)
	}
	for col, val := range extra {
		columns = append(columns, col)
		values = append(values, val)
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(columns)), ", ")
	execSQL(t, db, "INSERT INTO issues ("+strings.Join(columns, ", ")+") VALUES ("+placeholders+")", values...)
}

// insertTypedDep inserts a dependency into the given typed target column.
func insertTypedDep(t *testing.T, db *sql.DB, issueID, target, column, depType string) {
	t.Helper()
	execSQL(t, db, "INSERT INTO dependencies (issue_id, "+column+", type) VALUES (?, ?, ?)", issueID, target, depType)
}

func addCustomStatus(t *testing.T, db *sql.DB, name, category string) {
	t.Helper()
	execSQL(t, db, "INSERT INTO custom_statuses (name, category) VALUES (?, ?)", name, category)
}

func executeIDs(t *testing.T, executor *Executor, query string) map[string]bool {
	t.Helper()
	issues, err := executor.Execute(query)
	require.NoError(t, err, query)
	return collectIDs(issues)
}

func TestProbeSchema(t *testing.T) {
	t.Run("testutil sqlite schema", func(t *testing.T) {
		db := testutil.NewTestDB(t)
		defer func() { _ = db.Close() }()

		s, err := probeSchema(db)
		require.NoError(t, err)
		require.Equal(t, legacyDependencyTargetColumn, s.DependencyTarget)
		require.False(t, s.HasDependsOnExternal)
		require.False(t, s.HasIsBlocked)
		require.False(t, s.HasCustomStatuses)
		require.False(t, s.HasCustomTypes)
		require.True(t, s.HasIssueColumn("started_at"))
		require.True(t, s.HasIssueColumn("deleted_at"))
		require.False(t, s.HasIssueColumn("is_blocked"))
	})

	t.Run("schema 32 era dolt schema", func(t *testing.T) {
		db := setupDoltDB(t)
		defer func() { _ = db.Close() }()

		s, err := probeSchema(db)
		require.NoError(t, err)
		require.Equal(t, issueDependencyTargetColumn, s.DependencyTarget)
		require.False(t, s.HasDependsOnExternal)
		require.False(t, s.HasIsBlocked)
		require.False(t, s.HasCustomStatuses)
		require.False(t, s.HasIssueColumn("started_at"))
		require.False(t, s.HasIssueColumn("owner"))
		require.True(t, s.HasIssueColumn("closed_at"))
	})

	t.Run("legacy depends_on_id dolt schema", func(t *testing.T) {
		db := setupLegacyDoltDB(t)
		defer func() { _ = db.Close() }()

		s, err := probeSchema(db)
		require.NoError(t, err)
		require.Equal(t, legacyDependencyTargetColumn, s.DependencyTarget)
	})

	t.Run("current dolt schema", func(t *testing.T) {
		db := setupDoltDBWithSchema(t, currentDoltTestSchema)
		defer func() { _ = db.Close() }()

		s, err := probeSchema(db)
		require.NoError(t, err)
		require.Equal(t, issueDependencyTargetColumn, s.DependencyTarget)
		require.True(t, s.HasDependsOnExternal)
		require.True(t, s.HasIsBlocked)
		require.True(t, s.HasCustomStatuses)
		require.True(t, s.HasCustomTypes)
		require.True(t, s.HasIssueColumn("started_at"))
		require.True(t, s.HasIssueColumn("due_at"))
	})

	t.Run("pre is_blocked dolt schema", func(t *testing.T) {
		db := setupDoltDBWithSchema(t, preIsBlockedDoltTestSchema)
		defer func() { _ = db.Close() }()

		s, err := probeSchema(db)
		require.NoError(t, err)
		require.True(t, s.HasDependsOnExternal)
		require.False(t, s.HasIsBlocked)
		require.True(t, s.HasCustomStatuses)
	})

	t.Run("missing dependencies target column", func(t *testing.T) {
		db := setupDoltDBWithSchema(t, strings.ReplaceAll(doltTestSchema, issueDependencyTargetColumn, "target"))
		defer func() { _ = db.Close() }()

		_, err := probeSchema(db)
		require.ErrorContains(t, err, "no supported target column")
	})
}

func TestIsMissingTableError(t *testing.T) {
	require.True(t, isMissingTableError(errors.New("sqlite3: SQL logic error: no such table: custom_statuses")))
	require.True(t, isMissingTableError(errors.New("Error 1146 (42S02): Table 'beads.custom_statuses' doesn't exist")))
	require.True(t, isMissingTableError(errors.New("Error 1146: table not found: custom_statuses")))
	require.False(t, isMissingTableError(errors.New("driver: bad connection")))
}

// TestExecutor_ReadyMatchesBdReady checks the bd ready predicates against the
// SQLite test schema (blocked_issues_cache, legacy depends_on_id).
func TestExecutor_ReadyMatchesBdReady(t *testing.T) {
	future := time.Now().Add(48 * time.Hour)
	past := time.Now().Add(-48 * time.Hour)

	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("plain").
			WithIssue("decision", testutil.IssueType("decision")).
			WithIssue("in-progress", testutil.Status("in_progress")).
			WithIssue("pinned-flag", testutil.Pinned(true)).
			WithIssue("unpinned-flag", testutil.Pinned(false)).
			WithIssue("ephemeral", testutil.Ephemeral(true)).
			WithIssue("gate", testutil.IssueType("gate")).
			WithIssue("molecule", testutil.IssueType("molecule")).
			WithIssue("message", testutil.IssueType("message")).
			WithIssue("agent", testutil.IssueType("agent")).
			WithIssue("cached-blocked").
			WithIssue("self-deferred", testutil.DeferUntil(future)).
			WithIssue("deferred-epic", testutil.IssueType("epic"), testutil.DeferUntil(future)).
			WithIssue("child-of-deferred").
			WithIssue("past-epic", testutil.IssueType("epic"), testutil.DeferUntil(past)).
			WithIssue("child-of-past").
			WithDependency("child-of-deferred", "deferred-epic", "parent-child").
			WithDependency("child-of-past", "past-epic", "parent-child").
			WithBlockedCache("cached-blocked")
	})
	defer func() { _ = db.Close() }()

	executor := newTestExecutor(t, db)
	ready := executeIDs(t, executor, "ready = true")

	for _, id := range []string{"plain", "decision", "unpinned-flag", "past-epic", "child-of-past"} {
		require.True(t, ready[id], "%s should be ready", id)
	}
	for _, id := range []string{
		"in-progress", "pinned-flag", "ephemeral", "gate", "molecule", "message", "agent",
		"cached-blocked", "self-deferred", "deferred-epic", "child-of-deferred",
	} {
		require.False(t, ready[id], "%s should not be ready", id)
	}

	notReady := executeIDs(t, executor, "ready = false")
	for id := range ready {
		require.False(t, notReady[id], "%s cannot be both ready and not ready", id)
	}
	require.Len(t, notReady, 16-len(ready), "ready = false is the complement of ready = true")
}

// TestExecutor_SQLiteIsBlockedAndCustomStatuses adds current-beads columns
// and tables to the SQLite test schema: blocked then reads is_blocked instead
// of blocked_issues_cache, and ready accepts custom active statuses.
func TestExecutor_SQLiteIsBlockedAndCustomStatuses(t *testing.T) {
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("flagged").
			WithIssue("flagged-closed", testutil.Status("closed")).
			WithIssue("flagged-pinned", testutil.Status("pinned")).
			WithIssue("cache-only").
			WithIssue("triage", testutil.Status("triage")).
			WithIssue("review", testutil.Status("review")).
			WithBlockedCache("cache-only")
	})
	defer func() { _ = db.Close() }()

	execSQL(t, db, "ALTER TABLE issues ADD COLUMN is_blocked INTEGER NOT NULL DEFAULT 0")
	execSQL(t, db, "UPDATE issues SET is_blocked = 1 WHERE id IN ('flagged', 'flagged-closed', 'flagged-pinned')")
	execSQL(t, db, "CREATE TABLE custom_statuses (name TEXT PRIMARY KEY, category TEXT NOT NULL DEFAULT 'unspecified')")
	addCustomStatus(t, db, "triage", "active")
	addCustomStatus(t, db, "review", "wip")

	executor := newTestExecutor(t, db)

	blocked := executeIDs(t, executor, "blocked = true")
	require.Equal(t, map[string]bool{"flagged": true}, blocked,
		"is_blocked is the source of truth; closed and pinned issues are never blocked")

	notBlocked := executeIDs(t, executor, "blocked = false")
	require.False(t, notBlocked["flagged"])
	require.True(t, notBlocked["flagged-closed"])
	require.True(t, notBlocked["flagged-pinned"])
	require.True(t, notBlocked["cache-only"], "blocked_issues_cache is ignored once is_blocked exists")

	ready := executeIDs(t, executor, "ready = true")
	require.True(t, ready["triage"], "custom status with category active is ready")
	require.False(t, ready["review"], "custom status with category wip is not ready")
	require.True(t, ready["cache-only"])
	require.False(t, ready["flagged"])
}

func TestExecutor_NewFields(t *testing.T) {
	now := time.Now()
	db := setupDB(t, func(b *testutil.Builder) *testutil.Builder {
		return b.
			WithIssue("started-recent", testutil.Owner("alice"), testutil.StartedAt(now.Add(-2*time.Hour)), testutil.DueAt(now.Add(72*time.Hour))).
			WithIssue("started-old", testutil.Owner("bob"), testutil.StartedAt(now.Add(-30*24*time.Hour)), testutil.DueAt(now.Add(-24*time.Hour))).
			WithIssue("closed-dup", testutil.Status("closed"), testutil.CloseReason("duplicate of x")).
			WithIssue("never-started")
	})
	defer func() { _ = db.Close() }()
	execSQL(t, db, "UPDATE issues SET acceptance_criteria = 'all tests pass' WHERE id = 'started-recent'")

	executor := newTestExecutor(t, db)

	require.Equal(t, map[string]bool{"started-recent": true}, executeIDs(t, executor, "owner = alice"))
	require.Equal(t, map[string]bool{"closed-dup": true, "never-started": true}, executeIDs(t, executor, "owner = ''"),
		"a missing owner compares as the empty string")
	require.Equal(t, map[string]bool{"started-recent": true}, executeIDs(t, executor, "started >= -1d"))
	require.Equal(t, map[string]bool{"started-old": true}, executeIDs(t, executor, "due < now"))
	require.Equal(t, map[string]bool{"closed-dup": true}, executeIDs(t, executor, "closed >= today"))
	require.Equal(t, map[string]bool{"closed-dup": true}, executeIDs(t, executor, `close_reason ~ "DUPLICATE"`))
	require.Equal(t, map[string]bool{"started-recent": true}, executeIDs(t, executor, `acceptance_criteria ~ tests`))

	issues, err := executor.Execute("owner ~ o order by due asc")
	require.NoError(t, err)
	require.Len(t, issues, 1)
	require.Equal(t, "started-old", issues[0].ID)
}

func TestExecutor_FieldUnavailable(t *testing.T) {
	t.Run("dolt schema without newer columns", func(t *testing.T) {
		db := setupDoltDB(t)
		defer func() { _ = db.Close() }()
		insertDoltIssue(t, db, "a-1", "A", "open", "task", 2)

		executor := newDoltExecutor(t, db)
		for _, tc := range []struct{ query, field string }{
			{"started > -7d", "started"},
			{"due < now", "due"},
			{"owner = alice", "owner"},
			{"type = task order by started", "started"},
			{"not (type = bug or owner ~ x)", "owner"},
			{"metadata.team = x", "metadata.team"},
		} {
			_, err := executor.Execute(tc.query)
			require.Error(t, err, tc.query)
			var unavailable *FieldUnavailableError
			require.ErrorAs(t, err, &unavailable, tc.query)
			require.Equal(t, tc.field, unavailable.Field)
			require.EqualError(t, err, `field "`+tc.field+`" is not available in this beads database (upgrade beads)`)
		}

		// Fields whose columns exist keep working.
		ids := executeIDs(t, executor, "close_reason ~ x or acceptance_criteria ~ x order by closed")
		require.Empty(t, ids)
	})

	t.Run("re-probes after the column is added", func(t *testing.T) {
		db := testutil.NewTestDB(t)
		defer func() { _ = db.Close() }()
		testutil.NewBuilder(t, db).WithIssue("a-1").Build()
		execSQL(t, db, "ALTER TABLE issues DROP COLUMN started_at")

		executor := newTestExecutor(t, db)
		_, err := executor.Execute("started > -7d")
		var unavailable *FieldUnavailableError
		require.ErrorAs(t, err, &unavailable)

		// bd migrates the database while perles is running.
		execSQL(t, db, "ALTER TABLE issues ADD COLUMN started_at DATETIME")
		execSQL(t, db, "UPDATE issues SET started_at = ? WHERE id = 'a-1'", time.Now())
		require.Equal(t, map[string]bool{"a-1": true}, executeIDs(t, executor, "started > -7d"))
	})
}

func TestDoltDialect_CurrentSchema_ExternalDependencies(t *testing.T) {
	db := setupDoltDBWithSchema(t, currentDoltTestSchema)
	defer func() { _ = db.Close() }()

	insertCurrentIssue(t, db, "pa-1", "open", "epic", nil)
	insertCurrentIssue(t, db, "pb-1", "open", "task", nil)
	insertCurrentIssue(t, db, "pa-2", "open", "task", nil)
	insertCurrentIssue(t, db, "pa-3", "open", "task", nil)
	// Cross-prefix edges to local issues are stored in depends_on_external.
	insertTypedDep(t, db, "pb-1", "pa-1", externalDependencyTargetColumn, "parent-child")
	insertTypedDep(t, db, "pa-2", "pb-1", externalDependencyTargetColumn, "blocks")
	// Same-prefix edges use depends_on_issue_id.
	insertTypedDep(t, db, "pa-3", "pa-2", issueDependencyTargetColumn, "discovered-from")
	// Truly external and wisp targets are not local issues and are skipped.
	insertTypedDep(t, db, "pa-2", "external:other:thing", externalDependencyTargetColumn, "blocks")
	insertTypedDep(t, db, "pa-3", "pa-wisp-1", "depends_on_wisp_id", "blocks")

	executor := newDoltExecutor(t, db)
	byID := func(id string) beads.Issue {
		t.Helper()
		issues, err := executor.Execute("id = " + id)
		require.NoError(t, err)
		require.Len(t, issues, 1)
		return issues[0]
	}

	require.Equal(t, []string{"pb-1"}, byID("pa-1").Children)
	pb1 := byID("pb-1")
	require.Equal(t, "pa-1", pb1.ParentID)
	require.Equal(t, []string{"pa-2"}, pb1.Blocks)
	pa2 := byID("pa-2")
	require.Equal(t, []string{"pb-1"}, pa2.BlockedBy)
	require.Equal(t, []string{"pa-3"}, pa2.Discovered)
	pa3 := byID("pa-3")
	require.Equal(t, []string{"pa-2"}, pa3.DiscoveredFrom)
	require.Empty(t, pa3.BlockedBy)

	expanded := executeIDs(t, executor, "id = pa-1 expand down depth *")
	require.Equal(t, map[string]bool{"pa-1": true, "pb-1": true, "pa-2": true, "pa-3": true}, expanded)
}

func TestDoltDialect_CurrentSchema_BlockedUsesIsBlocked(t *testing.T) {
	db := setupDoltDBWithSchema(t, currentDoltTestSchema)
	defer func() { _ = db.Close() }()

	insertCurrentIssue(t, db, "blocker", "open", "task", nil)
	insertCurrentIssue(t, db, "flagged", "open", "task", map[string]any{"is_blocked": 1})
	insertCurrentIssue(t, db, "flagged-closed", "closed", "task", map[string]any{"is_blocked": 1})
	insertCurrentIssue(t, db, "flagged-pinned", "pinned", "task", map[string]any{"is_blocked": 1})
	// A blocks edge without the flag: bd's flag is the source of truth.
	insertCurrentIssue(t, db, "edge-only", "open", "task", nil)
	insertTypedDep(t, db, "edge-only", "blocker", issueDependencyTargetColumn, "blocks")

	executor := newDoltExecutor(t, db)

	require.Equal(t, map[string]bool{"flagged": true}, executeIDs(t, executor, "blocked = true"))
	require.Equal(t, map[string]bool{
		"blocker": true, "flagged-closed": true, "flagged-pinned": true, "edge-only": true,
	}, executeIDs(t, executor, "blocked = false"))
}

func TestDoltDialect_CurrentSchema_Ready(t *testing.T) {
	db := setupDoltDBWithSchema(t, currentDoltTestSchema)
	defer func() { _ = db.Close() }()

	future := time.Now().UTC().Add(48 * time.Hour)
	past := time.Now().UTC().Add(-48 * time.Hour)

	addCustomStatus(t, db, "triage", "active")
	addCustomStatus(t, db, "review", "wip")
	insertCurrentIssue(t, db, "plain", "open", "task", nil)
	insertCurrentIssue(t, db, "decision", "open", "decision", nil)
	insertCurrentIssue(t, db, "triage", "triage", "task", nil)
	insertCurrentIssue(t, db, "review", "review", "task", nil)
	insertCurrentIssue(t, db, "hooked", "hooked", "task", nil)
	insertCurrentIssue(t, db, "flagged", "open", "task", map[string]any{"is_blocked": 1})
	insertCurrentIssue(t, db, "pinned-flag", "open", "task", map[string]any{"pinned": 1})
	insertCurrentIssue(t, db, "ephemeral", "open", "task", map[string]any{"ephemeral": 1})
	insertCurrentIssue(t, db, "gate", "open", "gate", nil)
	insertCurrentIssue(t, db, "role", "open", "role", nil)
	insertCurrentIssue(t, db, "self-deferred", "open", "task", map[string]any{"defer_until": future})
	insertCurrentIssue(t, db, "deferred-epic", "open", "epic", map[string]any{"defer_until": future})
	insertCurrentIssue(t, db, "child-of-deferred", "open", "task", nil)
	insertCurrentIssue(t, db, "xb-child-of-deferred", "open", "task", nil)
	insertCurrentIssue(t, db, "past-epic", "open", "epic", map[string]any{"defer_until": past})
	insertCurrentIssue(t, db, "child-of-past", "open", "task", nil)
	insertTypedDep(t, db, "child-of-deferred", "deferred-epic", issueDependencyTargetColumn, "parent-child")
	insertTypedDep(t, db, "xb-child-of-deferred", "deferred-epic", externalDependencyTargetColumn, "parent-child")
	insertTypedDep(t, db, "child-of-past", "past-epic", issueDependencyTargetColumn, "parent-child")

	executor := newDoltExecutor(t, db)

	require.Equal(t, map[string]bool{
		"plain": true, "decision": true, "triage": true, "past-epic": true, "child-of-past": true,
	}, executeIDs(t, executor, "ready = true"))

	notReady := executeIDs(t, executor, "ready = false")
	require.Len(t, notReady, 11)
	require.True(t, notReady["xb-child-of-deferred"], "a deferred parent reached through depends_on_external defers the child")
}

// TestDoltDialect_PreIsBlocked_BlockedFallback covers the inlined
// blocked_issues view (beads migration 0045) on a schema with custom
// statuses and depends_on_external but no is_blocked.
func TestDoltDialect_PreIsBlocked_BlockedFallback(t *testing.T) {
	db := setupDoltDBWithSchema(t, preIsBlockedDoltTestSchema)
	defer func() { _ = db.Close() }()

	addCustomStatus(t, db, "shipped", "done")
	addCustomStatus(t, db, "icebox", "frozen")
	addCustomStatus(t, db, "review", "wip")

	insertCurrentIssue(t, db, "open-blocker", "open", "task", nil)
	insertCurrentIssue(t, db, "review-blocker", "review", "task", nil)
	insertCurrentIssue(t, db, "deferred-blocker", "deferred", "task", nil)
	insertCurrentIssue(t, db, "closed-blocker", "closed", "task", nil)
	insertCurrentIssue(t, db, "pinned-blocker", "pinned", "task", nil)
	insertCurrentIssue(t, db, "shipped-blocker", "shipped", "task", nil)
	insertCurrentIssue(t, db, "icebox-blocker", "icebox", "task", nil)
	insertCurrentIssue(t, db, "xb-blocker", "open", "task", nil)

	insertCurrentIssue(t, db, "by-open", "open", "task", nil)
	insertCurrentIssue(t, db, "by-review", "in_progress", "task", nil)
	insertCurrentIssue(t, db, "by-deferred", "hooked", "task", nil)
	insertCurrentIssue(t, db, "by-closed", "open", "task", nil)
	insertCurrentIssue(t, db, "by-pinned", "open", "task", nil)
	insertCurrentIssue(t, db, "by-shipped", "open", "task", nil)
	insertCurrentIssue(t, db, "by-icebox", "open", "task", nil)
	insertCurrentIssue(t, db, "by-external", "open", "task", nil)
	insertCurrentIssue(t, db, "by-remote", "open", "task", nil)
	insertCurrentIssue(t, db, "shipped-by-open", "shipped", "task", nil)
	insertCurrentIssue(t, db, "pinned-by-open", "pinned", "task", nil)

	for issue, blocker := range map[string]string{
		"by-open":         "open-blocker",
		"by-review":       "review-blocker",
		"by-deferred":     "deferred-blocker",
		"by-closed":       "closed-blocker",
		"by-pinned":       "pinned-blocker",
		"by-shipped":      "shipped-blocker",
		"by-icebox":       "icebox-blocker",
		"shipped-by-open": "open-blocker",
		"pinned-by-open":  "open-blocker",
	} {
		insertTypedDep(t, db, issue, blocker, issueDependencyTargetColumn, "blocks")
	}
	insertTypedDep(t, db, "by-external", "xb-blocker", externalDependencyTargetColumn, "blocks")
	insertTypedDep(t, db, "by-remote", "external:other:x", externalDependencyTargetColumn, "blocks")

	executor := newDoltExecutor(t, db)

	require.Equal(t, map[string]bool{
		"by-open": true, "by-review": true, "by-deferred": true, "by-external": true,
	}, executeIDs(t, executor, "blocked = true"))

	ready := executeIDs(t, executor, "ready = true")
	require.False(t, ready["by-open"])
	require.False(t, ready["by-external"])
	require.True(t, ready["by-closed"])
	require.True(t, ready["by-shipped"])
	require.True(t, ready["by-remote"], "an edge to a non-local target never blocks")
}
