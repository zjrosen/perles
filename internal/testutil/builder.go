package testutil

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
)

// depData holds data for a dependency to be inserted.
type depData struct {
	issueID     string
	dependsOnID string
	depType     string
}

// Builder accumulates test data and inserts it in the correct order.
type Builder struct {
	t       *testing.T
	db      *sql.DB
	issues  []issueData
	deps    []depData
	blocked []string
}

// NewBuilder creates a builder for the given test database.
func NewBuilder(t *testing.T, db *sql.DB) *Builder {
	t.Helper()
	return &Builder{t: t, db: db}
}

// WithIssue adds an issue with optional configuration.
func (b *Builder) WithIssue(id string, opts ...IssueOption) *Builder {
	issue := defaultIssue(id)
	for _, opt := range opts {
		opt(&issue)
	}
	b.issues = append(b.issues, issue)
	return b
}

// WithDependency adds a dependency relationship between issues.
func (b *Builder) WithDependency(issueID, dependsOnID, depType string) *Builder {
	b.deps = append(b.deps, depData{issueID, dependsOnID, depType})
	return b
}

// WithBlockedCache marks an issue as blocked in the cache.
func (b *Builder) WithBlockedCache(issueID string) *Builder {
	b.blocked = append(b.blocked, issueID)
	return b
}

// Build inserts all accumulated data into the database.
func (b *Builder) Build() {
	b.t.Helper()
	// Insert in dependency order: issues → labels → deps → comments → blocked
	for _, issue := range b.issues {
		b.insertIssue(issue)
		b.insertLabels(issue.id, issue.labels)
		b.insertComments(issue.id, issue.comments)
	}
	for _, dep := range b.deps {
		b.insertDependency(dep)
	}
	for _, id := range b.blocked {
		b.insertBlockedCache(id)
	}
}

func (b *Builder) insertIssue(issue issueData) {
	b.t.Helper()
	_, err := b.db.Exec(
		`INSERT INTO issues (id, title, description, status, priority, issue_type, assignee, sender, ephemeral, pinned, is_template, created_at, created_by, updated_at, closed_at, deleted_at, hook_bead, role_bead, agent_state, last_activity, role_type, rig, mol_type)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		issue.id, issue.title, issue.description, issue.status, issue.priority,
		issue.issueType, issue.assignee, issue.sender, issue.ephemeral, issue.pinned, issue.isTemplate, issue.createdAt, issue.createdBy, issue.updatedAt, issue.closedAt, issue.deletedAt,
		issue.hookBead, issue.roleBead, issue.agentState, issue.lastActivity, issue.roleType, issue.rig, issue.molType,
	)
	require.NoError(b.t, err)
}

func (b *Builder) insertLabels(issueID string, labels []string) {
	b.t.Helper()
	for _, label := range labels {
		_, err := b.db.Exec(`INSERT INTO labels (issue_id, label) VALUES (?, ?)`, issueID, label)
		require.NoError(b.t, err)
	}
}

func (b *Builder) insertComments(issueID string, comments []CommentData) {
	b.t.Helper()
	for _, c := range comments {
		_, err := b.db.Exec(
			`INSERT INTO comments (issue_id, author, text) VALUES (?, ?, ?)`,
			issueID, c.Author, c.Text,
		)
		require.NoError(b.t, err)
	}
}

func (b *Builder) insertDependency(dep depData) {
	b.t.Helper()
	_, err := b.db.Exec(
		`INSERT INTO dependencies (issue_id, depends_on_id, type) VALUES (?, ?, ?)`,
		dep.issueID, dep.dependsOnID, dep.depType,
	)
	require.NoError(b.t, err)
}

func (b *Builder) insertBlockedCache(issueID string) {
	b.t.Helper()
	_, err := b.db.Exec(`INSERT INTO blocked_issues_cache (issue_id) VALUES (?)`, issueID)
	require.NoError(b.t, err)
}
