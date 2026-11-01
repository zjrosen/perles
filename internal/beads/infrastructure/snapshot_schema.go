package infrastructure

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/zjrosen/perles/internal/log"

	_ "github.com/ncruces/go-sqlite3/driver" // registers the "sqlite3" driver
	"github.com/ncruces/go-sqlite3/vfs/memdb"
)

// snapshotSchema is the DDL of an embedded-workspace snapshot, the in-memory
// SQLite database that the BQL executor queries in its SQLite dialect.
//
// It covers every BQL field (bql.ValidFields) plus metadata.*. issues has
// is_blocked, so blocked and ready read bd's stored flag, and deleted_at,
// which the SQLite soft-delete filter requires (always NULL). There are no
// CHECK constraints and no foreign keys: dependencies keep bd's export target
// as-is (a local issue, a wisp or an external: ref), and the executor's joins
// to issues drop targets that are not loaded issues, as in server mode.
// Comment IDs are TEXT because bd's are UUIDs. custom_statuses always exists,
// so the executor's schema probe sees a stable schema, and is filled only
// when the workspace uses custom statuses. There is no custom_types table:
// BQL only probes for it.
//
// Every time column holds UTC 'YYYY-MM-DD HH:MM:SS' text, which compares
// lexically with SQLite's datetime('now', ...) and scans into time.Time.
const snapshotSchema = `
CREATE TABLE issues (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  design TEXT NOT NULL DEFAULT '',
  acceptance_criteria TEXT NOT NULL DEFAULT '',
  notes TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'open',
  priority INTEGER NOT NULL DEFAULT 2,
  issue_type TEXT NOT NULL DEFAULT 'task',
  assignee TEXT,
  owner TEXT NOT NULL DEFAULT '',
  sender TEXT NOT NULL DEFAULT '',
  ephemeral INTEGER NOT NULL DEFAULT 0,
  pinned INTEGER NOT NULL DEFAULT 0,
  is_template INTEGER NOT NULL DEFAULT 0,
  is_blocked INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  created_by TEXT NOT NULL DEFAULT '',
  updated_at DATETIME NOT NULL,
  started_at DATETIME,
  closed_at DATETIME,
  due_at DATETIME,
  defer_until DATETIME,
  close_reason TEXT NOT NULL DEFAULT '',
  mol_type TEXT NOT NULL DEFAULT '',
  metadata TEXT NOT NULL DEFAULT '{}',
  deleted_at DATETIME
);
CREATE TABLE labels (issue_id TEXT NOT NULL, label TEXT NOT NULL);
CREATE INDEX idx_labels_issue ON labels(issue_id);
CREATE INDEX idx_labels_label ON labels(label);
CREATE TABLE dependencies (
  issue_id TEXT NOT NULL,
  depends_on_id TEXT NOT NULL,
  type TEXT NOT NULL DEFAULT 'blocks'
);
CREATE INDEX idx_deps_issue ON dependencies(issue_id);
CREATE INDEX idx_deps_target ON dependencies(depends_on_id);
CREATE TABLE comments (
  id TEXT PRIMARY KEY,
  issue_id TEXT NOT NULL,
  author TEXT NOT NULL DEFAULT '',
  text TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL
);
CREATE INDEX idx_comments_issue ON comments(issue_id);
CREATE TABLE custom_statuses (name TEXT PRIMARY KEY, category TEXT NOT NULL DEFAULT '');
CREATE TABLE local_metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL);
`

// snapshotTimeLayout is the UTC text layout of every snapshot time column.
const snapshotTimeLayout = "2006-01-02 15:04:05"

// maxExportLineBytes bounds one line of bd export output. An issue with a
// long description, many comments or large metadata is a single multi-MB
// line.
const maxExportLineBytes = 64 << 20

// snapshotMaxIdleConns keeps idle connections to a snapshot open, so queries
// reuse them instead of opening a new SQLite connection each time. The named
// memdb database stays alive without them, but it must be at least 1.
const snapshotMaxIdleConns = 4

// ExportIssue is one issues-table record of `bd export --all`, with its
// labels, outgoing dependency edges and comments.
type ExportIssue struct {
	ID                 string
	Title              string
	Description        string
	Design             string
	AcceptanceCriteria string
	Notes              string
	// Status is bd's status, "open" when the record omits it.
	Status   string
	Priority int
	// IssueType is bd's issue type, "task" when the record omits it.
	IssueType   string
	Assignee    string
	Owner       string
	Sender      string
	CreatedBy   string
	CloseReason string
	MolType     string
	Pinned      bool
	IsTemplate  bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
	StartedAt   *time.Time
	ClosedAt    *time.Time
	DueAt       *time.Time
	DeferUntil  *time.Time
	// Metadata is the issue's metadata as compact JSON text, "{}" when the
	// record has none or null.
	Metadata     string
	Labels       []string
	Dependencies []ExportDependency
	Comments     []ExportComment
}

// ExportDependency is an outgoing dependency edge of an ExportIssue: the
// issue depends on DependsOnID with the given type (blocks, parent-child,
// waits-for, ...). The target is bd's export target as-is: a local issue, a
// wisp or an external: ref.
type ExportDependency struct {
	DependsOnID string
	Type        string
}

// ExportComment is a comment of an ExportIssue. bd comment IDs are UUIDs.
type ExportComment struct {
	ID        string
	Author    string
	Text      string
	CreatedAt time.Time
}

// exportRecord is one line of `bd export --all` as bd writes it. Unknown
// fields are ignored.
type exportRecord struct {
	// RecordType is "issue" for issues and e.g. "memory" for memories. bd
	// older than 1.0.3 writes no _type, and such a record is an issue.
	RecordType string `json:"_type"` //nolint:tagliatelle // bd's record discriminator key
	// Ephemeral marks a wisp, which lives in bd's wisps table.
	Ephemeral bool `json:"ephemeral"`
	// WispPlane marks a wisps-table row whose flags alone do not say so (a
	// no-history wisp).
	WispPlane bool `json:"wisp_plane"`

	ID                 string                 `json:"id"`
	Title              string                 `json:"title"`
	Description        string                 `json:"description"`
	Design             string                 `json:"design"`
	AcceptanceCriteria string                 `json:"acceptance_criteria"`
	Notes              string                 `json:"notes"`
	Status             string                 `json:"status"`
	Priority           int                    `json:"priority"`
	IssueType          string                 `json:"issue_type"`
	Assignee           string                 `json:"assignee"`
	Owner              string                 `json:"owner"`
	Sender             string                 `json:"sender"`
	CreatedBy          string                 `json:"created_by"`
	CloseReason        string                 `json:"close_reason"`
	MolType            string                 `json:"mol_type"`
	Pinned             bool                   `json:"pinned"`
	IsTemplate         bool                   `json:"is_template"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`
	StartedAt          *time.Time             `json:"started_at"`
	ClosedAt           *time.Time             `json:"closed_at"`
	DueAt              *time.Time             `json:"due_at"`
	DeferUntil         *time.Time             `json:"defer_until"`
	Metadata           json.RawMessage        `json:"metadata"`
	Labels             []string               `json:"labels"`
	Dependencies       []exportDependencyJSON `json:"dependencies"`
	Comments           []exportCommentJSON    `json:"comments"`
}

// exportDependencyJSON is a dependency record of bd export. Its metadata (a
// JSON string in bd) is not decoded, so any shape is accepted.
type exportDependencyJSON struct {
	DependsOnID string `json:"depends_on_id"`
	Type        string `json:"type"`
}

// exportCommentJSON is a comment record of bd export.
type exportCommentJSON struct {
	ID        bdFlexibleID `json:"id"`
	Author    string       `json:"author"`
	Text      string       `json:"text"`
	CreatedAt time.Time    `json:"created_at"`
}

// isIssue reports whether the record is an issues-table issue: an issue
// record that is not a wisp.
func (r *exportRecord) isIssue() bool {
	if r.RecordType != "" && r.RecordType != "issue" {
		return false
	}
	return !r.Ephemeral && !r.WispPlane
}

func (r *exportRecord) toIssue() (ExportIssue, error) {
	metadata, err := normalizeExportMetadata(r.Metadata)
	if err != nil {
		return ExportIssue{}, fmt.Errorf("issue %s metadata: %w", r.ID, err)
	}
	issue := ExportIssue{
		ID:                 r.ID,
		Title:              r.Title,
		Description:        r.Description,
		Design:             r.Design,
		AcceptanceCriteria: r.AcceptanceCriteria,
		Notes:              r.Notes,
		Status:             firstNonEmpty(r.Status, "open"),
		Priority:           r.Priority,
		IssueType:          firstNonEmpty(r.IssueType, "task"),
		Assignee:           r.Assignee,
		Owner:              r.Owner,
		Sender:             r.Sender,
		CreatedBy:          r.CreatedBy,
		CloseReason:        r.CloseReason,
		MolType:            r.MolType,
		Pinned:             r.Pinned,
		IsTemplate:         r.IsTemplate,
		CreatedAt:          r.CreatedAt,
		UpdatedAt:          r.UpdatedAt,
		StartedAt:          r.StartedAt,
		ClosedAt:           r.ClosedAt,
		DueAt:              r.DueAt,
		DeferUntil:         r.DeferUntil,
		Metadata:           metadata,
		Labels:             r.Labels,
	}
	for _, d := range r.Dependencies {
		if d.DependsOnID == "" {
			continue
		}
		issue.Dependencies = append(issue.Dependencies, ExportDependency(d))
	}
	for _, c := range r.Comments {
		issue.Comments = append(issue.Comments, ExportComment{
			ID:        string(c.ID),
			Author:    c.Author,
			Text:      c.Text,
			CreatedAt: c.CreatedAt,
		})
	}
	return issue, nil
}

// normalizeExportMetadata re-marshals issue metadata to compact JSON text,
// "{}" when it is absent or null.
func normalizeExportMetadata(raw json.RawMessage) (string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return "{}", nil
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, trimmed); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// DecodeExport decodes `bd export --all` JSONL into the issues a snapshot
// loads. It keeps issue records, including records without _type (bd older
// than 1.0.3), and skips wisps (ephemeral or wisp_plane), memories and every
// other record type. Blank lines are skipped. A malformed line is an error
// that names its line number.
func DecodeExport(r io.Reader) ([]ExportIssue, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxExportLineBytes)

	var issues []ExportIssue
	skipped := 0
	line := 0
	for scanner.Scan() {
		line++
		text := bytes.TrimSpace(scanner.Bytes())
		if len(text) == 0 {
			continue
		}
		var rec exportRecord
		if err := json.Unmarshal(text, &rec); err != nil {
			return nil, fmt.Errorf("bd export line %d: %w", line, err)
		}
		if !rec.isIssue() {
			skipped++
			continue
		}
		if rec.ID == "" {
			return nil, fmt.Errorf("bd export line %d: issue record has no id", line)
		}
		issue, err := rec.toIssue()
		if err != nil {
			return nil, fmt.Errorf("bd export line %d: %w", line, err)
		}
		issues = append(issues, issue)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("bd export line %d: %w", line+1, err)
	}
	log.Debug(log.CatBeads, "Decoded bd export", "issues", len(issues), "skipped", skipped)
	return issues, nil
}

// CustomStatus is a custom status of the workspace (bd statuses --json
// custom_statuses), with its category: active, wip, done or frozen.
type CustomStatus struct {
	Name     string
	Category string
}

// SnapshotData is everything one snapshot generation loads.
type SnapshotData struct {
	// Issues are the decoded export records (DecodeExport).
	Issues []ExportIssue
	// BlockedIDs are the issues whose stored is_blocked flag bd reports set.
	// is_blocked is 1 exactly for loaded issues in this set; IDs that are not
	// loaded are ignored.
	BlockedIDs map[string]bool
	// CustomStatuses fill custom_statuses. Empty when the workspace uses no
	// custom status.
	CustomStatuses []CustomStatus
	// BDVersion is stored as local_metadata bd_version.
	BDVersion string
}

// LoadSnapshot creates the shared in-memory database name with memdb.Create,
// opens it and loads data into it in one transaction.
//
// name must be unique per snapshot generation and has no leading slash; the
// database is opened with the DSN file:/<name>?vfs=memdb, which memdb looks
// up without the slash. The memdb reference keeps the database alive while
// the pool's connections come and go, until the caller calls
// memdb.Delete(name). The caller owns that call, after closing the returned
// DB or after LoadSnapshot fails: on any failure after memdb.Create, the DB
// is closed before the error is returned, so the Delete frees it.
func LoadSnapshot(ctx context.Context, name string, data SnapshotData) (*sql.DB, error) {
	if name == "" || strings.HasPrefix(name, "/") {
		return nil, fmt.Errorf("invalid snapshot database name %q: must be non-empty without a leading slash", name)
	}
	start := time.Now()

	memdb.Create(name, nil)
	dsn := (&url.URL{
		Scheme:   "file",
		OmitHost: true,
		Path:     "/" + name,
		RawQuery: url.Values{"vfs": {"memdb"}}.Encode(),
	}).String()
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open snapshot database: %w", err)
	}
	db.SetMaxIdleConns(snapshotMaxIdleConns)

	if err := loadSnapshot(ctx, db, data); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close snapshot database: %w", closeErr))
		}
		return nil, err
	}
	log.Debug(log.CatBeads, "Loaded snapshot", "name", name, "issues", len(data.Issues),
		"blocked", len(data.BlockedIDs), "customStatuses", len(data.CustomStatuses),
		"duration", time.Since(start))
	return db, nil
}

// loadSnapshot creates the schema and inserts data in one transaction.
func loadSnapshot(ctx context.Context, db *sql.DB, data SnapshotData) (err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin snapshot load: %w", err)
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				err = errors.Join(err, fmt.Errorf("roll back snapshot load: %w", rbErr))
			}
		}
	}()

	if _, err := tx.ExecContext(ctx, snapshotSchema); err != nil {
		return fmt.Errorf("create snapshot schema: %w", err)
	}

	l, err := newSnapshotLoader(ctx, tx)
	if err != nil {
		return err
	}
	defer l.close()

	for i := range data.Issues {
		if err := l.insertIssue(ctx, &data.Issues[i], data.BlockedIDs[data.Issues[i].ID]); err != nil {
			return err
		}
	}
	for _, s := range data.CustomStatuses {
		if _, err := tx.ExecContext(ctx, `INSERT INTO custom_statuses (name, category) VALUES (?, ?)`, s.Name, s.Category); err != nil {
			return fmt.Errorf("insert custom status %s: %w", s.Name, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO local_metadata (key, value) VALUES ('bd_version', ?)`, data.BDVersion); err != nil {
		return fmt.Errorf("insert bd_version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit snapshot load: %w", err)
	}
	return nil
}

// snapshotLoader holds the prepared insert statements of one load.
type snapshotLoader struct {
	issue, label, dependency, comment *sql.Stmt
}

func newSnapshotLoader(ctx context.Context, tx *sql.Tx) (*snapshotLoader, error) {
	l := &snapshotLoader{}
	stmts := []struct {
		dst   **sql.Stmt
		query string
	}{
		{&l.issue, `INSERT INTO issues (
			id, title, description, design, acceptance_criteria, notes, status, priority,
			issue_type, assignee, owner, sender, ephemeral, pinned, is_template, is_blocked,
			created_at, created_by, updated_at, started_at, closed_at, due_at, defer_until,
			close_reason, mol_type, metadata, deleted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`},
		{&l.label, `INSERT INTO labels (issue_id, label) VALUES (?, ?)`},
		{&l.dependency, `INSERT INTO dependencies (issue_id, depends_on_id, type) VALUES (?, ?, ?)`},
		{&l.comment, `INSERT INTO comments (id, issue_id, author, text, created_at) VALUES (?, ?, ?, ?, ?)`},
	}
	for _, s := range stmts {
		stmt, err := tx.PrepareContext(ctx, s.query)
		if err != nil {
			l.close()
			return nil, fmt.Errorf("prepare snapshot insert: %w", err)
		}
		*s.dst = stmt
	}
	return l, nil
}

// close closes the prepared statements. Their errors are irrelevant: the
// transaction that owns them is committed or rolled back either way.
func (l *snapshotLoader) close() {
	for _, stmt := range []*sql.Stmt{l.issue, l.label, l.dependency, l.comment} {
		if stmt != nil {
			_ = stmt.Close()
		}
	}
}

func (l *snapshotLoader) insertIssue(ctx context.Context, issue *ExportIssue, blocked bool) error {
	var assignee any
	if issue.Assignee != "" {
		assignee = issue.Assignee
	}
	if _, err := l.issue.ExecContext(ctx,
		issue.ID, issue.Title, issue.Description, issue.Design, issue.AcceptanceCriteria, issue.Notes,
		issue.Status, issue.Priority, issue.IssueType, assignee, issue.Owner, issue.Sender,
		issue.Pinned, issue.IsTemplate, blocked,
		snapshotTime(issue.CreatedAt), issue.CreatedBy, snapshotTime(issue.UpdatedAt),
		snapshotTimePtr(issue.StartedAt), snapshotTimePtr(issue.ClosedAt),
		snapshotTimePtr(issue.DueAt), snapshotTimePtr(issue.DeferUntil),
		issue.CloseReason, issue.MolType, issue.Metadata,
	); err != nil {
		return fmt.Errorf("insert issue %s: %w", issue.ID, err)
	}
	for _, label := range issue.Labels {
		if _, err := l.label.ExecContext(ctx, issue.ID, label); err != nil {
			return fmt.Errorf("insert label %q of issue %s: %w", label, issue.ID, err)
		}
	}
	for _, dep := range issue.Dependencies {
		if _, err := l.dependency.ExecContext(ctx, issue.ID, dep.DependsOnID, dep.Type); err != nil {
			return fmt.Errorf("insert dependency %s -> %s: %w", issue.ID, dep.DependsOnID, err)
		}
	}
	for _, c := range issue.Comments {
		if _, err := l.comment.ExecContext(ctx, c.ID, issue.ID, c.Author, c.Text, snapshotTime(c.CreatedAt)); err != nil {
			return fmt.Errorf("insert comment %s of issue %s: %w", c.ID, issue.ID, err)
		}
	}
	return nil
}

// snapshotTime formats t as UTC 'YYYY-MM-DD HH:MM:SS' text.
func snapshotTime(t time.Time) string {
	return t.UTC().Format(snapshotTimeLayout)
}

// snapshotTimePtr is snapshotTime for an optional time; nil is NULL.
func snapshotTimePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return snapshotTime(*t)
}
