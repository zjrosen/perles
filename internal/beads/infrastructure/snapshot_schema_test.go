package infrastructure

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ncruces/go-sqlite3/vfs/memdb"
	"github.com/stretchr/testify/require"

	appbeads "github.com/zjrosen/perles/internal/beads/application"
	"github.com/zjrosen/perles/internal/beads/bql"
	domain "github.com/zjrosen/perles/internal/beads/domain"
	"github.com/zjrosen/perles/internal/cachemanager"
)

// exportFixture is `bd --readonly export --all` from bd 1.3.2 (6ebaacba8) on
// an embedded workspace, with the author anonymised, followed by hand-edited
// records: one without _type, one with every time field at non-UTC offsets, a
// wisp_plane wisp, a promoted no-history issue, a pinned template and an
// unknown record type.
//
// bd's answers on that workspace (custom statuses in_review:active and
// shipped:done): stored is_blocked set for fx-bjw and fx-1w6 (bd blocked
// --flags --raw), and bd ready = fx-0ur, fx-aww, fx-aww.1, fx-etq, fx-wak.
const exportFixture = "testdata/bd_export_v1.3.2.jsonl"

// fixtureCaptureTime is just after the bd 1.3.2 part of the fixture was
// captured. The parity suite shifts every fixture time by now minus this, so
// relative-date queries see the same ages whenever the test runs.
var fixtureCaptureTime = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

// fixtureIssueIDs are the issues the fixture loads, in file order: every
// record except the two wisps, the memory and the config record.
var fixtureIssueIDs = []string{
	"fx-etq", "fx-bjw", "fx-aww", "fx-yjy", "fx-3l4.1", "fx-1l7", "fx-3l4", "fx-wak", "fx-0ur",
	"fx-mwt", "fx-aww.1", "fx-1w6", "fx-pcw", "fx-hjd", "fx-legacy", "fx-old", "fx-promoted", "fx-tmpl",
}

// fixtureBlockedIDs are bd's stored flags plus IDs that must not load: a wisp
// and an unknown issue.
func fixtureBlockedIDs() map[string]bool {
	return map[string]bool{"fx-bjw": true, "fx-1w6": true, "fx-wisp-ple": true, "fx-missing": true}
}

var fixtureCustomStatuses = []CustomStatus{{Name: "in_review", Category: "active"}, {Name: "shipped", Category: "done"}}

func decodeFixture(t *testing.T) []ExportIssue {
	t.Helper()
	f, err := os.Open(exportFixture)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	issues, err := DecodeExport(f)
	require.NoError(t, err)
	return issues
}

func fixtureData(t *testing.T) SnapshotData {
	t.Helper()
	return SnapshotData{
		Issues:         decodeFixture(t),
		BlockedIDs:     fixtureBlockedIDs(),
		CustomStatuses: fixtureCustomStatuses,
		BDVersion:      "1.3.2",
	}
}

// snapshotName returns a unique memdb name for the test and deletes it when
// the test ends, as the snapshot backend will.
func snapshotName(t *testing.T) string {
	t.Helper()
	name := strings.ReplaceAll(t.Name(), "/", "_") + "_" + rand.Text()
	t.Cleanup(func() { memdb.Delete(name) })
	return name
}

// snapshotDSN is the DSN LoadSnapshot opens for name.
func snapshotDSN(name string) string {
	return (&url.URL{Scheme: "file", OmitHost: true, Path: "/" + name, RawQuery: "vfs=memdb"}).String()
}

func loadTestSnapshot(t *testing.T, data SnapshotData) *sql.DB {
	t.Helper()
	db, err := LoadSnapshot(context.Background(), snapshotName(t), data)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func issueIDs(issues []ExportIssue) []string {
	ids := make([]string, len(issues))
	for i, issue := range issues {
		ids[i] = issue.ID
	}
	return ids
}

func findExportIssue(t *testing.T, issues []ExportIssue, id string) ExportIssue {
	t.Helper()
	for _, issue := range issues {
		if issue.ID == id {
			return issue
		}
	}
	t.Fatalf("issue %s not decoded", id)
	return ExportIssue{}
}

func queryStrings(t *testing.T, db *sql.DB, query string, args ...any) []string {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), query, args...)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var s string
		require.NoError(t, rows.Scan(&s))
		out = append(out, s)
	}
	require.NoError(t, rows.Err())
	return out
}

func TestDecodeExport_Fixture(t *testing.T) {
	issues := decodeFixture(t)

	// Wisps (ephemeral and wisp_plane), the memory and the config record are
	// skipped; the record without _type and the promoted no-history issue load.
	require.Equal(t, fixtureIssueIDs, issueIDs(issues))

	etq := findExportIssue(t, issues, "fx-etq")
	require.Equal(t, "Fix token refresh", etq.Title)
	require.Equal(t, "bug", etq.IssueType)
	require.Equal(t, 0, etq.Priority)
	require.Equal(t, "Design text", etq.Design)
	require.Equal(t, "AC text", etq.AcceptanceCriteria)
	require.Equal(t, "Notes text", etq.Notes)
	require.Equal(t, "author@example.com", etq.Owner)
	require.Equal(t, "Fixture Author", etq.CreatedBy)
	require.Equal(t, []string{"backend"}, etq.Labels)
	require.Equal(t, "{}", etq.Metadata)
	require.Equal(t, []ExportComment{
		{ID: "f63a9b1a-c63a-548d-ba7b-ae4d60895983", Author: "Fixture Author", Text: "First comment",
			CreatedAt: time.Date(2026, 10, 31, 23, 44, 21, 0, time.UTC)},
		{ID: "5b60a8c8-a57c-565a-8051-79f4644f5786", Author: "Fixture Author", Text: "Second comment",
			CreatedAt: time.Date(2026, 10, 31, 23, 44, 22, 0, time.UTC)},
	}, etq.Comments)

	aww := findExportIssue(t, issues, "fx-aww")
	require.Equal(t, `{"team":"backend","flaky":true,"points":5}`, aww.Metadata)

	// Outgoing edges keep bd's targets as-is: wisp and external targets too.
	require.Equal(t, []ExportDependency{
		{DependsOnID: "fx-etq", Type: "discovered-from"},
		{DependsOnID: "fx-wisp-ple", Type: "blocks"},
	}, findExportIssue(t, issues, "fx-1w6").Dependencies)
	require.Equal(t, []ExportDependency{{DependsOnID: "external:otherproj:somecap", Type: "blocks"}},
		findExportIssue(t, issues, "fx-mwt").Dependencies)
	require.Equal(t, []ExportDependency{{DependsOnID: "fx-aww", Type: "parent-child"}},
		findExportIssue(t, issues, "fx-aww.1").Dependencies)

	statuses := map[string]string{}
	for _, issue := range issues {
		statuses[issue.ID] = issue.Status
	}
	require.Equal(t, "in_review", statuses["fx-wak"])
	require.Equal(t, "shipped", statuses["fx-1l7"])
	require.Equal(t, "pinned", statuses["fx-hjd"])

	// The legacy record: no _type, null metadata, dependency metadata as a
	// string and as an object, an unknown field.
	legacy := findExportIssue(t, issues, "fx-legacy")
	require.Equal(t, "{}", legacy.Metadata)
	require.Equal(t, "mailer", legacy.Sender)
	require.Equal(t, "swarm", legacy.MolType)
	require.Equal(t, []ExportDependency{
		{DependsOnID: "fx-old", Type: "discovered-from"},
		{DependsOnID: "external:otherproj:legacycap", Type: "related"},
	}, legacy.Dependencies)
	require.True(t, legacy.CreatedAt.Equal(time.Date(2026, 9, 1, 12, 30, 0, 0, time.UTC)))

	tmpl := findExportIssue(t, issues, "fx-tmpl")
	require.True(t, tmpl.Pinned)
	require.True(t, tmpl.IsTemplate)
}

func TestDecodeExport_Records(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantIDs []string
		check   func(t *testing.T, issues []ExportIssue)
	}{
		{
			name:    "missing _type is an issue",
			input:   `{"id":"a-1","title":"x","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`,
			wantIDs: []string{"a-1"},
		},
		{
			name:    "null _type is an issue",
			input:   `{"_type":null,"id":"a-1"}`,
			wantIDs: []string{"a-1"},
		},
		{
			name: "memories and unknown record types are skipped",
			input: `{"_type":"memory","key":"k","value":"v"}
{"_type":"issue","id":"a-1"}
{"_type":"agent_state","id":"a-2"}`,
			wantIDs: []string{"a-1"},
		},
		{
			name: "non-issue records are not decoded as issues",
			input: `{"_type":"memory","key":"k","priority":"high","created_at":"yesterday","labels":"x","metadata":5,"comments":{"a":1}}
{"_type":"issue","id":"a-1","priority":2}
{"_type":"agent_state","id":7,"ephemeral":"yes","dependencies":"none"}`,
			wantIDs: []string{"a-1"},
			check: func(t *testing.T, issues []ExportIssue) {
				require.Equal(t, 2, issues[0].Priority)
			},
		},
		{
			name: "wisps are skipped",
			input: `{"_type":"issue","id":"w-1","ephemeral":true}
{"_type":"issue","id":"w-2","no_history":true,"wisp_plane":true}
{"_type":"issue","id":"a-1","ephemeral":false,"wisp_plane":false}
{"_type":"issue","id":"a-2","no_history":true}`,
			wantIDs: []string{"a-1", "a-2"},
		},
		{
			name:    "blank lines are skipped",
			input:   "\n{\"id\":\"a-1\"}\n   \n\n{\"id\":\"a-2\"}\n",
			wantIDs: []string{"a-1", "a-2"},
		},
		{
			name:    "empty input",
			input:   "",
			wantIDs: nil,
		},
		{
			name:    "metadata absent, null and object",
			input:   "{\"id\":\"a-1\"}\n{\"id\":\"a-2\",\"metadata\":null}\n{\"id\":\"a-3\",\"metadata\":{ \"b\" : [1, 2], \"a\" : {\"c\": null} }}",
			wantIDs: []string{"a-1", "a-2", "a-3"},
			check: func(t *testing.T, issues []ExportIssue) {
				require.Equal(t, "{}", issues[0].Metadata)
				require.Equal(t, "{}", issues[1].Metadata)
				require.Equal(t, `{"b":[1,2],"a":{"c":null}}`, issues[2].Metadata)
			},
		},
		{
			name:    "omitted status and type take bd's defaults",
			input:   `{"id":"a-1"}`,
			wantIDs: []string{"a-1"},
			check: func(t *testing.T, issues []ExportIssue) {
				require.Equal(t, "open", issues[0].Status)
				require.Equal(t, "task", issues[0].IssueType)
			},
		},
		{
			name:    "dependency metadata of any shape and empty targets",
			input:   `{"id":"a-1","dependencies":[{"depends_on_id":"a-2","type":"blocks","metadata":"{\"x\":1}"},{"depends_on_id":"a-3","type":"related","metadata":{"x":1}},{"depends_on_id":"","type":"blocks"}]}`,
			wantIDs: []string{"a-1"},
			check: func(t *testing.T, issues []ExportIssue) {
				require.Equal(t, []ExportDependency{{DependsOnID: "a-2", Type: "blocks"}, {DependsOnID: "a-3", Type: "related"}},
					issues[0].Dependencies)
			},
		},
		{
			name:    "comment ids as UUID strings and legacy numbers",
			input:   `{"id":"a-1","comments":[{"id":"0b7e1f6c-1111-5222-8333-444455556666","author":"me","text":"t","created_at":"2026-01-01T10:00:00+02:00"},{"id":7,"text":"old"}]}`,
			wantIDs: []string{"a-1"},
			check: func(t *testing.T, issues []ExportIssue) {
				require.Len(t, issues[0].Comments, 2)
				require.Equal(t, "0b7e1f6c-1111-5222-8333-444455556666", issues[0].Comments[0].ID)
				require.True(t, issues[0].Comments[0].CreatedAt.Equal(time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)))
				require.Equal(t, "7", issues[0].Comments[1].ID)
			},
		},
		{
			name:    "unknown fields are ignored",
			input:   `{"id":"a-1","is_blocked":true,"spec_id":"s","future":{"deep":[{"x":1}]},"dependency_count":3}`,
			wantIDs: []string{"a-1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues, err := DecodeExport(strings.NewReader(tt.input))
			require.NoError(t, err)
			require.Equal(t, tt.wantIDs, func() []string {
				if len(issues) == 0 {
					return nil
				}
				return issueIDs(issues)
			}())
			if tt.check != nil {
				tt.check(t, issues)
			}
		})
	}
}

func TestDecodeExport_Errors(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name:    "malformed line names its line number",
			input:   "{\"id\":\"a-1\"}\n\n{\"id\":\"a-2\",\n{\"id\":\"a-3\"}",
			wantErr: "bd export line 3:",
		},
		{
			name:    "wrong field type",
			input:   `{"id":"a-1","priority":"high"}`,
			wantErr: "bd export line 1:",
		},
		{
			name:    "wrong field type in an issue record",
			input:   "{\"_type\":\"memory\",\"priority\":\"high\"}\n{\"_type\":\"issue\",\"id\":\"a-1\",\"priority\":\"high\"}",
			wantErr: "bd export line 2:",
		},
		{
			name:    "_type that is not a string",
			input:   `{"_type":5,"id":"a-1"}`,
			wantErr: "bd export line 1:",
		},
		{
			name:    "line that is not an object",
			input:   `["_type","memory"]`,
			wantErr: "bd export line 1:",
		},
		{
			name:    "issue without id",
			input:   "{\"_type\":\"memory\"}\n{\"_type\":\"issue\",\"title\":\"no id\"}",
			wantErr: "bd export line 2: issue record has no id",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues, err := DecodeExport(strings.NewReader(tt.input))
			require.ErrorContains(t, err, tt.wantErr)
			require.Nil(t, issues)
		})
	}
}

func TestDecodeExport_MultiMegabyteLine(t *testing.T) {
	description := strings.Repeat("x", 5<<20)
	input := `{"id":"a-1"}` + "\n" + `{"id":"a-2","description":"` + description + `"}` + "\n" + `{"id":"a-3"}`

	issues, err := DecodeExport(strings.NewReader(input))
	require.NoError(t, err)
	require.Equal(t, []string{"a-1", "a-2", "a-3"}, issueIDs(issues))
	require.Len(t, issues[1].Description, 5<<20)
}

func TestLoadSnapshot_Rows(t *testing.T) {
	db := loadTestSnapshot(t, fixtureData(t))

	require.ElementsMatch(t, fixtureIssueIDs, queryStrings(t, db, `SELECT id FROM issues`))
	require.Empty(t, queryStrings(t, db, `SELECT id FROM issues WHERE id IN ('fx-wisp-ple', 'fx-wisp-np1')`))

	// is_blocked is set exactly for the given IDs that loaded.
	require.ElementsMatch(t, []string{"fx-bjw", "fx-1w6"}, queryStrings(t, db, `SELECT id FROM issues WHERE is_blocked = 1`))
	require.Empty(t, queryStrings(t, db, `SELECT id FROM issues WHERE is_blocked NOT IN (0, 1)`))
	// ephemeral is always 0 and deleted_at always NULL.
	require.Empty(t, queryStrings(t, db, `SELECT id FROM issues WHERE ephemeral != 0 OR deleted_at IS NOT NULL`))
	require.Equal(t, []string{"fx-tmpl"}, queryStrings(t, db, `SELECT id FROM issues WHERE pinned = 1 AND is_template = 1`))
	require.Empty(t, queryStrings(t, db, `SELECT id FROM issues WHERE (pinned = 1 OR is_template = 1) AND id != 'fx-tmpl'`))

	// assignee is NULL when absent.
	require.Equal(t, []string{"fx-yjy"}, queryStrings(t, db, `SELECT id FROM issues WHERE assignee IS NOT NULL`))
	require.Equal(t, []string{"alice"}, queryStrings(t, db, `SELECT assignee FROM issues WHERE id = 'fx-yjy'`))

	// Times are UTC 'YYYY-MM-DD HH:MM:SS' text, whatever the export offset.
	type timeRow struct {
		created, updated                         string
		started, closed, due, deferUntil, reason sql.NullString
		metadata, owner, sender, molType, status string
	}
	readTimes := func(id string) timeRow {
		var r timeRow
		require.NoError(t, db.QueryRowContext(context.Background(), `
			SELECT CAST(created_at AS TEXT), CAST(updated_at AS TEXT), CAST(started_at AS TEXT),
			       CAST(closed_at AS TEXT), CAST(due_at AS TEXT), CAST(defer_until AS TEXT), close_reason,
			       metadata, owner, sender, mol_type, status
			FROM issues WHERE id = ?`, id).Scan(
			&r.created, &r.updated, &r.started, &r.closed, &r.due, &r.deferUntil, &r.reason,
			&r.metadata, &r.owner, &r.sender, &r.molType, &r.status))
		return r
	}
	valid := func(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }

	old := readTimes("fx-old")
	require.Equal(t, "2025-03-10 03:45:30", old.created)           // +05:30
	require.Equal(t, "2025-06-02 00:00:00", old.updated)           // -07:00, crosses midnight
	require.Equal(t, valid("2025-03-12 01:30:00"), old.started)    // -02:00, crosses midnight
	require.Equal(t, valid("2025-06-01 23:59:59"), old.closed)     // -07:00, fractional second dropped
	require.Equal(t, valid("2025-06-01 00:00:00"), old.due)        // -04:00
	require.Equal(t, valid("2025-03-31 23:00:00"), old.deferUntil) // +01:00
	require.Equal(t, valid("Obsolete"), old.reason)
	require.Equal(t, "{}", old.metadata) // metadata null
	require.Equal(t, "closed", old.status)

	legacy := readTimes("fx-legacy")
	require.Equal(t, "2026-09-01 12:30:00", legacy.created)
	require.Equal(t, "2026-09-02 08:00:00", legacy.updated) // +02:00 with fractional seconds
	require.False(t, legacy.started.Valid)
	require.False(t, legacy.closed.Valid)
	require.Equal(t, "{}", legacy.metadata)
	require.Equal(t, "legacy@example.com", legacy.owner)
	require.Equal(t, "mailer", legacy.sender)
	require.Equal(t, "swarm", legacy.molType)

	require.Equal(t, `{"team":"backend","flaky":true,"points":5}`, readTimes("fx-aww").metadata)
	require.Equal(t, valid("2026-12-01 05:00:00"), readTimes("fx-aww.1").due)
	require.Equal(t, valid("2099-01-01 05:00:00"), readTimes("fx-3l4").deferUntil)
	require.Equal(t, valid("2026-10-31 23:44:20"), readTimes("fx-yjy").started)
	require.Equal(t, valid("2026-10-31 23:44:15"), readTimes("fx-pcw").closed)
	require.Equal(t, "2026-10-31 22:00:00", readTimes("fx-tmpl").created) // -04:00

	// Every time value in the snapshot has the normalized shape.
	timeShape := regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$`)
	for _, col := range []string{"created_at", "updated_at", "started_at", "closed_at", "due_at", "defer_until"} {
		for _, v := range queryStrings(t, db, `SELECT CAST(`+col+` AS TEXT) FROM issues WHERE `+col+` IS NOT NULL`) {
			require.Regexp(t, timeShape, v, "issues.%s", col)
		}
	}
	for _, v := range queryStrings(t, db, `SELECT CAST(created_at AS TEXT) FROM comments`) {
		require.Regexp(t, timeShape, v, "comments.created_at")
	}

	// Labels, dependencies (targets as-is) and comments.
	require.ElementsMatch(t, []string{
		"fx-etq:backend", "fx-aww:backend", "fx-aww:security", "fx-aww.1:frontend", "fx-legacy:legacy",
	}, queryStrings(t, db, `SELECT issue_id || ':' || label FROM labels`))
	require.ElementsMatch(t, []string{
		"fx-bjw>fx-etq:blocks",
		"fx-3l4.1>fx-3l4:parent-child",
		"fx-0ur>fx-pcw:blocks",
		"fx-mwt>external:otherproj:somecap:blocks",
		"fx-aww.1>fx-aww:parent-child",
		"fx-1w6>fx-etq:discovered-from",
		"fx-1w6>fx-wisp-ple:blocks",
		"fx-legacy>fx-old:discovered-from",
		"fx-legacy>external:otherproj:legacycap:related",
	}, queryStrings(t, db, `SELECT issue_id || '>' || depends_on_id || ':' || type FROM dependencies`))
	require.ElementsMatch(t, []string{
		"f63a9b1a-c63a-548d-ba7b-ae4d60895983|fx-etq|First comment|2026-10-31 23:44:21",
		"5b60a8c8-a57c-565a-8051-79f4644f5786|fx-etq|Second comment|2026-10-31 23:44:22",
		"acaed66b-39c2-5b39-9424-96c76cff6040|fx-aww.1|Child comment|2026-10-31 23:44:22",
		"0d9a3c1e-7b42-5f4e-9a1d-2c6b8e0f1a23|fx-old|Comment with offset|2025-03-10 04:30:00",
	}, queryStrings(t, db, `SELECT id || '|' || issue_id || '|' || text || '|' || CAST(created_at AS TEXT) FROM comments`))

	require.ElementsMatch(t, []string{"in_review:active", "shipped:done"},
		queryStrings(t, db, `SELECT name || ':' || category FROM custom_statuses`))
	require.Equal(t, []string{"1.3.2"}, queryStrings(t, db, `SELECT value FROM local_metadata WHERE key = 'bd_version'`))
}

func TestLoadSnapshot_Schema(t *testing.T) {
	// No custom statuses: custom_statuses still exists, empty.
	db := loadTestSnapshot(t, SnapshotData{Issues: decodeFixture(t), BDVersion: "1.3.2"})

	tables := queryStrings(t, db, `SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name`)
	require.Equal(t, []string{"comments", "custom_statuses", "dependencies", "issues", "labels", "local_metadata"}, tables)
	require.Empty(t, queryStrings(t, db, `SELECT name FROM custom_statuses`))

	// No CHECK constraints and no foreign keys.
	for _, ddl := range queryStrings(t, db, `SELECT sql FROM sqlite_master WHERE sql IS NOT NULL`) {
		upper := strings.ToUpper(ddl)
		require.NotContains(t, upper, "CHECK", ddl)
		require.NotContains(t, upper, "REFERENCES", ddl)
	}
	for _, table := range tables {
		require.Empty(t, queryStrings(t, db, `SELECT "table" FROM pragma_foreign_key_list(?)`, table), table)
	}

	// Comment IDs are TEXT, for bd's UUIDs.
	require.Equal(t, []string{"TEXT"}, queryStrings(t, db, `SELECT type FROM pragma_table_info('comments') WHERE name = 'id'`))

	// Indexes on the join keys.
	require.ElementsMatch(t, []string{
		"idx_comments_issue", "idx_deps_issue", "idx_deps_target", "idx_labels_issue", "idx_labels_label",
	}, queryStrings(t, db, `SELECT name FROM sqlite_master WHERE type = 'index' AND name LIKE 'idx_%'`))
}

func newSnapshotExecutor(db *sql.DB) *bql.Executor {
	return bql.NewExecutor(db, appbeads.DialectSQLite,
		cachemanager.NewInMemoryCacheManager[string, []domain.Issue]("snapshot-test", time.Minute, time.Minute),
		cachemanager.NewInMemoryCacheManager[string, *bql.DependencyGraph]("snapshot-test-deps", time.Minute, time.Minute))
}

// TestLoadSnapshot_CoversEveryBQLField runs a query on every BQL field and on
// metadata through the real executor, so a field whose column or table the
// snapshot lacks fails here.
func TestLoadSnapshot_CoversEveryBQLField(t *testing.T) {
	executor := newSnapshotExecutor(loadTestSnapshot(t, fixtureData(t)))

	queries := []string{`metadata.team = "backend"`, `metadata.team = nil`, `metadata.team != nil`, `metadata.team ~ back`}
	for field, fieldType := range bql.ValidFields {
		switch fieldType {
		case bql.FieldString:
			queries = append(queries, field+` = "x"`, field+` ~ x`)
		case bql.FieldEnum:
			queries = append(queries, field+` = open`, field+` in (open, closed)`)
		case bql.FieldPriority:
			queries = append(queries, field+` = P1`, field+` <= 2`)
		case bql.FieldBool:
			queries = append(queries, field+` = true`, field+` = false`)
		case bql.FieldDate:
			queries = append(queries, field+` > -7d`, field+` < now`)
		default:
			t.Fatalf("field %s has an unhandled type %v", field, fieldType)
		}
		if field != "blocked" && field != "ready" && field != "label" {
			queries = append(queries, `id != "" order by `+field+` desc`)
		}
	}
	for _, query := range queries {
		_, err := executor.Execute(query)
		require.NoError(t, err, query)
	}
}

func TestLoadSnapshot_SharedByName(t *testing.T) {
	name := snapshotName(t)
	db, err := LoadSnapshot(context.Background(), name, fixtureData(t))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	// With every connection of the loading pool closed, only the memdb.Create
	// reference keeps the database alive. A second pool on the same DSN still
	// sees it, so the name LoadSnapshot registered is the one
	// file:/<name>?vfs=memdb opens.
	db.SetMaxIdleConns(0)
	require.Equal(t, 0, db.Stats().OpenConnections)
	other, err := sql.Open("sqlite3", snapshotDSN(name))
	require.NoError(t, err)
	defer func() { _ = other.Close() }()
	require.Equal(t, []string{fmt.Sprint(len(fixtureIssueIDs))}, queryStrings(t, other, `SELECT CAST(COUNT(*) AS TEXT) FROM issues`))
}

func TestLoadSnapshot_SurvivesIdlePool(t *testing.T) {
	db := loadTestSnapshot(t, fixtureData(t))
	count := func() string {
		return queryStrings(t, db, `SELECT CAST(COUNT(*) AS TEXT) FROM issues`)[0]
	}
	want := fmt.Sprint(len(fixtureIssueIDs))

	// LoadSnapshot keeps idle connections.
	require.Equal(t, want, count())
	require.GreaterOrEqual(t, db.Stats().Idle, 1)

	// Several connections at once, then only one may stay idle.
	db.SetMaxIdleConns(1)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Go(func() {
			var n int
			errs <- db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM issues`).Scan(&n)
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.LessOrEqual(t, db.Stats().Idle, 1)
	require.Equal(t, want, count())

	// Every connection closed: the named memdb database lives on.
	db.SetMaxIdleConns(0)
	require.Equal(t, 0, db.Stats().OpenConnections)
	require.Equal(t, want, count())

	db.SetMaxIdleConns(1)
	require.Equal(t, want, count())
	require.Equal(t, []string{"in_review", "shipped"}, queryStrings(t, db, `SELECT name FROM custom_statuses ORDER BY name`))
}

func TestLoadSnapshot_InvalidName(t *testing.T) {
	for _, name := range []string{"", "/snapshot-1"} {
		db, err := LoadSnapshot(context.Background(), name, fixtureData(t))
		require.ErrorContains(t, err, "invalid snapshot database name")
		require.Nil(t, db)
	}
}

func TestLoadSnapshot_FailureReturnsNoDB(t *testing.T) {
	issue := func(id string, comments ...ExportComment) ExportIssue {
		now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
		return ExportIssue{ID: id, Status: "open", IssueType: "task", CreatedAt: now, UpdatedAt: now,
			Metadata: "{}", Comments: comments}
	}
	comment := ExportComment{ID: "c-1", CreatedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}

	tests := []struct {
		name    string
		data    SnapshotData
		wantErr string
	}{
		{
			name:    "duplicate issue id",
			data:    SnapshotData{Issues: []ExportIssue{issue("fx-1"), issue("fx-dup"), issue("fx-dup")}},
			wantErr: "insert issue fx-dup",
		},
		{
			name:    "duplicate comment id",
			data:    SnapshotData{Issues: []ExportIssue{issue("fx-1", comment), issue("fx-2", comment)}},
			wantErr: "insert comment c-1 of issue fx-2",
		},
		{
			name: "duplicate custom status",
			data: SnapshotData{Issues: []ExportIssue{issue("fx-1")},
				CustomStatuses: []CustomStatus{{Name: "review", Category: "active"}, {Name: "review", Category: "wip"}}},
			wantErr: "insert custom status review",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := snapshotName(t)
			db, err := LoadSnapshot(context.Background(), name, tt.data)
			require.ErrorContains(t, err, tt.wantErr)
			require.ErrorContains(t, err, "UNIQUE constraint failed")
			require.Nil(t, db)

			// The load rolled back: the named database is still registered
			// until the caller deletes it, and it is empty.
			other, err := sql.Open("sqlite3", snapshotDSN(name))
			require.NoError(t, err)
			defer func() { _ = other.Close() }()
			require.Empty(t, queryStrings(t, other, `SELECT name FROM sqlite_master`))
		})
	}
}

// captureSnapshotDBs makes LoadSnapshot's pools observable: it wraps
// openSnapshotDB for the test and returns the pools it opened.
func captureSnapshotDBs(t *testing.T) *[]*sql.DB {
	t.Helper()
	var opened []*sql.DB
	orig := openSnapshotDB
	openSnapshotDB = func(driverName, dsn string) (*sql.DB, error) {
		db, err := orig(driverName, dsn)
		if err == nil {
			opened = append(opened, db)
		}
		return db, err
	}
	t.Cleanup(func() { openSnapshotDB = orig })
	return &opened
}

// TestLoadSnapshot_FailureClosesDB: on a failed load LoadSnapshot closes the
// pool it opened, so the caller's memdb.Delete frees the database.
func TestLoadSnapshot_FailureClosesDB(t *testing.T) {
	opened := captureSnapshotDBs(t)
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	dup := ExportIssue{ID: "fx-dup", Status: "open", IssueType: "task", CreatedAt: now, UpdatedAt: now, Metadata: "{}"}

	db, err := LoadSnapshot(context.Background(), snapshotName(t), SnapshotData{Issues: []ExportIssue{dup, dup}})
	require.ErrorContains(t, err, "insert issue fx-dup")
	require.Nil(t, db)

	require.Len(t, *opened, 1)
	require.EqualError(t, (*opened)[0].PingContext(context.Background()), "sql: database is closed")
}

// TestLoadSnapshot_SuccessKeepsDBOpen: the pool LoadSnapshot returns is the
// one it opened, still open.
func TestLoadSnapshot_SuccessKeepsDBOpen(t *testing.T) {
	opened := captureSnapshotDBs(t)
	db := loadTestSnapshot(t, fixtureData(t))
	require.Equal(t, []*sql.DB{db}, *opened)
	require.NoError(t, db.PingContext(context.Background()))
}

// TestRollbackSnapshotLoad: a transaction that has already ended (its Commit
// failed or its context was cancelled) adds nothing to the load's error, and
// a live one is rolled back.
func TestRollbackSnapshotLoad(t *testing.T) {
	db, err := sql.Open("sqlite3", snapshotDSN(snapshotName(t)))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	loadErr := errors.New("insert issue fx-1: boom")

	t.Run("ended transaction", func(t *testing.T) {
		tx, err := db.BeginTx(context.Background(), nil)
		require.NoError(t, err)
		require.NoError(t, tx.Commit())

		err = rollbackSnapshotLoad(tx, loadErr)
		require.Same(t, loadErr, err, "sql.ErrTxDone must not be joined onto the load error")
	})

	t.Run("live transaction", func(t *testing.T) {
		tx, err := db.BeginTx(context.Background(), nil)
		require.NoError(t, err)
		_, err = tx.ExecContext(context.Background(), `CREATE TABLE rolled_back (id TEXT)`)
		require.NoError(t, err)

		require.Same(t, loadErr, rollbackSnapshotLoad(tx, loadErr))
		require.ErrorIs(t, tx.Commit(), sql.ErrTxDone, "the transaction was rolled back")
		require.Empty(t, queryStrings(t, db, `SELECT name FROM sqlite_master`))
	})
}

func TestLoadSnapshot_CanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	db, err := LoadSnapshot(ctx, snapshotName(t), fixtureData(t))
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, db)
}

// shiftExportTimes moves every time in issues by d.
func shiftExportTimes(issues []ExportIssue, d time.Duration) {
	shiftPtr := func(t *time.Time) *time.Time {
		if t == nil {
			return nil
		}
		shifted := t.Add(d)
		return &shifted
	}
	for i := range issues {
		issue := &issues[i]
		issue.CreatedAt = issue.CreatedAt.Add(d)
		issue.UpdatedAt = issue.UpdatedAt.Add(d)
		issue.StartedAt = shiftPtr(issue.StartedAt)
		issue.ClosedAt = shiftPtr(issue.ClosedAt)
		issue.DueAt = shiftPtr(issue.DueAt)
		issue.DeferUntil = shiftPtr(issue.DeferUntil)
		for j := range issue.Comments {
			issue.Comments[j].CreatedAt = issue.Comments[j].CreatedAt.Add(d)
		}
	}
}

// parityData is the fixture with its times shifted to the present.
func parityData(t *testing.T) SnapshotData {
	t.Helper()
	data := fixtureData(t)
	shiftExportTimes(data.Issues, time.Now().UTC().Sub(fixtureCaptureTime))
	return data
}

func resultIDs(issues []domain.Issue) []string {
	ids := make([]string, len(issues))
	for i, issue := range issues {
		ids[i] = issue.ID
	}
	return ids
}

func resultByID(t *testing.T, issues []domain.Issue, id string) domain.Issue {
	t.Helper()
	for _, issue := range issues {
		if issue.ID == id {
			return issue
		}
	}
	t.Fatalf("issue %s not in result %v", id, resultIDs(issues))
	return domain.Issue{}
}

// TestSnapshotBQLParity runs the real BQL executor, in its SQLite dialect, on
// a snapshot loaded from the fixture, for every query shape the UI hard-codes
// and every field family.
func TestSnapshotBQLParity(t *testing.T) {
	db := loadTestSnapshot(t, parityData(t))
	executor := newSnapshotExecutor(db)

	// bd ready on the fixture workspace, plus the external-only issue (bd's
	// external-dependency policy keeps it out of bd ready; the stored flag
	// is 0, as in server mode) and the hand-edited ready issues.
	bdReady := []string{"fx-0ur", "fx-aww", "fx-aww.1", "fx-etq", "fx-wak"}
	wantReady := append(slices.Clone(bdReady), "fx-mwt", "fx-legacy", "fx-promoted")

	unordered := []struct {
		query string
		want  []string
	}{
		// UI shapes: issueeditor.go, kanban/handlers.go, tree_column.go, search.go, epic_tree.go.
		{`id = "fx-etq"`, []string{"fx-etq"}},
		{`id = "fx-aww" expand down depth *`, []string{"fx-aww", "fx-aww.1"}},
		{`id = "fx-aww.1" expand up depth *`, []string{"fx-aww.1", "fx-aww"}},
		{`id = "fx-etq" expand down depth *`, []string{"fx-etq", "fx-bjw", "fx-1w6"}},
		{`id = "fx-3l4" expand down depth *`, []string{"fx-3l4", "fx-3l4.1"}},
		// The wisp target of fx-1w6 and the external target of fx-mwt are
		// dropped by the join.
		{`id = "fx-1w6" expand up depth *`, []string{"fx-1w6", "fx-etq"}},
		{`id = "fx-mwt" expand up depth *`, []string{"fx-mwt"}},
		{`id = "fx-legacy" expand all depth *`, []string{"fx-legacy", "fx-old"}},

		{`status = open and type = epic`, []string{"fx-aww"}},
		{`status = in_progress`, []string{"fx-yjy"}},
		{`status in (in_review, shipped)`, []string{"fx-wak", "fx-1l7"}},
		{`status not in (open, closed, pinned, deferred, in_progress, in_review, shipped)`, nil},
		{`type = bug`, []string{"fx-etq", "fx-bjw"}},
		{`type = bug or type = chore`, []string{"fx-etq", "fx-bjw", "fx-pcw"}},
		{`priority = P0`, []string{"fx-etq"}},
		{`priority <= 1`, []string{"fx-etq", "fx-bjw", "fx-aww"}},
		{`label = backend`, []string{"fx-etq", "fx-aww"}},
		{`label in (frontend, security)`, []string{"fx-aww.1", "fx-aww"}},
		{`label ~ end`, []string{"fx-etq", "fx-aww", "fx-aww.1"}},
		{`label != backend and type = epic`, []string{"fx-3l4"}},
		{`id in ("fx-etq", "fx-bjw", "fx-wisp-ple")`, []string{"fx-etq", "fx-bjw"}},
		{`assignee = alice`, []string{"fx-yjy"}},
		{`sender = mailer`, []string{"fx-legacy"}},
		{`mol_type = swarm`, []string{"fx-legacy"}},
		{`owner = "legacy@example.com"`, []string{"fx-legacy"}},
		{`created_by = "Legacy Author"`, []string{"fx-legacy"}},
		{`close_reason ~ done`, []string{"fx-pcw"}},
		{`description ~ Epic`, []string{"fx-aww"}},
		{`design ~ design`, []string{"fx-etq"}},
		{`acceptance_criteria = "AC text"`, []string{"fx-etq"}},
		{`notes ~ Notes`, []string{"fx-etq"}},
		{`pinned = true`, []string{"fx-tmpl"}},
		{`is_template = true`, []string{"fx-tmpl"}},

		{`metadata.team = "backend"`, []string{"fx-aww"}},
		{`metadata.points = "5"`, []string{"fx-aww"}},
		{`metadata.flaky = "true"`, []string{"fx-aww"}},
		{`metadata.team != nil`, []string{"fx-aww", "fx-aww.1"}},
		{`metadata.team = nil and type = feature`, []string{"fx-legacy"}},

		{`updated > -7d`, slices.DeleteFunc(slices.Clone(fixtureIssueIDs), func(id string) bool {
			return id == "fx-legacy" || id == "fx-old"
		})},
		{`created > -24h`, slices.DeleteFunc(slices.Clone(fixtureIssueIDs), func(id string) bool {
			return id == "fx-legacy" || id == "fx-old" || id == "fx-promoted"
		})},
		{`closed > -365d`, []string{"fx-pcw"}},
		{`started > -7d`, []string{"fx-yjy"}},
		{`due > now`, []string{"fx-aww.1"}},
		{`defer_until > now`, []string{"fx-3l4"}},

		{`title ~ "token"`, []string{"fx-etq"}},
		{`title ~ TOKEN`, []string{"fx-etq"}},
		{`title !~ e`, []string{"fx-aww.1"}},

		// blocked is bd's flag; external, wisp and closed blockers do not add to it.
		{`blocked = true`, []string{"fx-bjw", "fx-1w6"}},
		{`ready = true`, wantReady},
	}
	for _, tt := range unordered {
		t.Run(tt.query, func(t *testing.T) {
			issues, err := executor.Execute(tt.query)
			require.NoError(t, err)
			require.ElementsMatch(t, tt.want, resultIDs(issues))
		})
	}

	ordered := []struct {
		query string
		want  []string
	}{
		{`type = bug order by priority asc`, []string{"fx-etq", "fx-bjw"}},
		{`label = backend order by title asc`, []string{"fx-aww", "fx-etq"}},
		{`priority >= 3 order by priority desc, title asc`, []string{"fx-hjd", "fx-1w6", "fx-pcw", "fx-old"}},
		{`type = task and status = closed or id = "fx-legacy" order by created asc`, []string{"fx-old", "fx-legacy"}},
	}
	for _, tt := range ordered {
		t.Run(tt.query, func(t *testing.T) {
			issues, err := executor.Execute(tt.query)
			require.NoError(t, err)
			require.Equal(t, tt.want, resultIDs(issues))
		})
	}

	t.Run("fields relationships and comment counts", func(t *testing.T) {
		issues, err := executor.Execute(`status != tombstone`)
		require.NoError(t, err)
		require.ElementsMatch(t, fixtureIssueIDs, resultIDs(issues))

		etq := resultByID(t, issues, "fx-etq")
		require.Equal(t, "Fix token refresh", etq.TitleText)
		require.Equal(t, domain.TypeBug, etq.Type)
		require.Equal(t, domain.Priority(0), etq.Priority)
		require.Equal(t, domain.StatusOpen, etq.Status)
		require.Equal(t, "Design text", etq.Design)
		require.Equal(t, "AC text", etq.AcceptanceCriteria)
		require.Equal(t, "Notes text", etq.Notes)
		require.Equal(t, "Fixture Author", etq.CreatedBy)
		require.Equal(t, []string{"backend"}, etq.Labels)
		require.Equal(t, 2, etq.CommentCount)
		require.Equal(t, []string{"fx-bjw"}, etq.Blocks)
		require.Equal(t, []string{"fx-1w6"}, etq.Discovered)
		require.Empty(t, etq.BlockedBy)
		require.Empty(t, etq.ParentID)
		require.Equal(t, time.UTC, etq.CreatedAt.Location())
		require.WithinDuration(t, time.Now(), etq.CreatedAt, time.Hour)

		require.Equal(t, []string{"fx-etq"}, resultByID(t, issues, "fx-bjw").BlockedBy)
		require.Equal(t, []string{"fx-aww.1"}, resultByID(t, issues, "fx-aww").Children)
		require.ElementsMatch(t, []string{"backend", "security"}, resultByID(t, issues, "fx-aww").Labels)

		child := resultByID(t, issues, "fx-aww.1")
		require.Equal(t, "fx-aww", child.ParentID)
		require.Equal(t, 1, child.CommentCount)

		require.Equal(t, "fx-3l4", resultByID(t, issues, "fx-3l4.1").ParentID)
		require.Equal(t, []string{"fx-3l4.1"}, resultByID(t, issues, "fx-3l4").Children)
		require.Equal(t, []string{"fx-pcw"}, resultByID(t, issues, "fx-0ur").BlockedBy)
		require.Equal(t, []string{"fx-0ur"}, resultByID(t, issues, "fx-pcw").Blocks)

		// External and wisp targets are dropped by the join, as in server mode.
		require.Empty(t, resultByID(t, issues, "fx-mwt").BlockedBy)
		disc := resultByID(t, issues, "fx-1w6")
		require.Empty(t, disc.BlockedBy)
		require.Equal(t, []string{"fx-etq"}, disc.DiscoveredFrom)

		legacy := resultByID(t, issues, "fx-legacy")
		require.Equal(t, []string{"fx-old"}, legacy.DiscoveredFrom)
		require.Equal(t, "mailer", legacy.Sender)
		require.Equal(t, "swarm", legacy.MolType)
		require.Equal(t, []string{"fx-legacy"}, resultByID(t, issues, "fx-old").Discovered)

		old := resultByID(t, issues, "fx-old")
		require.Equal(t, "Obsolete", old.CloseReason)
		require.Equal(t, 1, old.CommentCount)
		require.False(t, old.ClosedAt.IsZero())
		require.False(t, old.DeferUntil.IsZero())

		yjy := resultByID(t, issues, "fx-yjy")
		require.Equal(t, "alice", yjy.Assignee)

		tmpl := resultByID(t, issues, "fx-tmpl")
		require.NotNil(t, tmpl.Pinned)
		require.True(t, *tmpl.Pinned)
		require.NotNil(t, tmpl.IsTemplate)
		require.True(t, *tmpl.IsTemplate)
		require.False(t, tmpl.Ephemeral)

		for _, issue := range issues {
			switch issue.ID {
			case "fx-etq", "fx-aww.1", "fx-old":
			default:
				require.Zero(t, issue.CommentCount, issue.ID)
			}
		}
	})

	t.Run("comments read UUID ids", func(t *testing.T) {
		// The comment query server mode runs (DoltClient.GetComments).
		rows, err := db.QueryContext(context.Background(),
			`SELECT id, author, text, created_at FROM comments WHERE issue_id = ? ORDER BY created_at ASC`, "fx-etq")
		require.NoError(t, err)
		defer func() { _ = rows.Close() }()
		var comments []domain.Comment
		for rows.Next() {
			var c domain.Comment
			require.NoError(t, rows.Scan(&c.ID, &c.Author, &c.Text, &c.CreatedAt))
			comments = append(comments, c)
		}
		require.NoError(t, rows.Err())
		require.Len(t, comments, 2)
		require.Equal(t, "f63a9b1a-c63a-548d-ba7b-ae4d60895983", comments[0].ID)
		require.Equal(t, "First comment", comments[0].Text)
		require.Equal(t, "5b60a8c8-a57c-565a-8051-79f4644f5786", comments[1].ID)
		require.Equal(t, time.Second, comments[1].CreatedAt.Sub(comments[0].CreatedAt))
	})
}

// TestSnapshotBQLParity_ReadyNeedsCustomStatuses shows why custom_statuses is
// filled when custom statuses are in use: without it, an issue in a custom
// active status is never ready.
func TestSnapshotBQLParity_ReadyNeedsCustomStatuses(t *testing.T) {
	data := parityData(t)
	data.CustomStatuses = nil
	executor := newSnapshotExecutor(loadTestSnapshot(t, data))

	issues, err := executor.Execute(`ready = true`)
	require.NoError(t, err)
	ids := resultIDs(issues)
	require.NotContains(t, ids, "fx-wak")
	require.ElementsMatch(t, []string{"fx-0ur", "fx-aww", "fx-aww.1", "fx-etq", "fx-mwt", "fx-legacy", "fx-promoted"}, ids)

	// The child of a deferred parent is not ready either way.
	require.NotContains(t, ids, "fx-3l4.1")
}
