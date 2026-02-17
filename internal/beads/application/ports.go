package application

import (
	"database/sql"

	domain "github.com/zjrosen/perles/internal/beads/domain"
)

// SQLDialect identifies the SQL variant for query generation.
type SQLDialect string

const (
	// DialectSQLite is the SQLite SQL dialect.
	DialectSQLite SQLDialect = "sqlite"
	// DialectMySQL is the MySQL SQL dialect (used by Dolt).
	DialectMySQL SQLDialect = "mysql"
)

// DBClient provides database access with backend-agnostic operations.
type DBClient interface {
	VersionReader
	CommentReader

	// DB returns the underlying *sql.DB for query execution.
	DB() *sql.DB

	// DBPath returns a path suitable for file watching.
	DBPath() string

	// Dialect returns the SQL dialect for query building.
	Dialect() SQLDialect

	// Close closes the database connection.
	Close() error
}

// VersionReader reads the beads database version.
type VersionReader interface {
	Version() (string, error)
}

// CommentReader reads comments for issues.
type CommentReader interface {
	GetComments(issueID string) ([]domain.Comment, error)
}

// IssueReader reads issue details.
type IssueReader interface {
	ShowIssue(issueID string) (*domain.Issue, error)
}

// IssueWriter provides write operations for issues.
type IssueWriter interface {
	UpdateStatus(issueID string, status domain.Status) error
	UpdatePriority(issueID string, priority domain.Priority) error
	UpdateType(issueID string, issueType domain.IssueType) error
	UpdateTitle(issueID, title string) error
	UpdateDescription(issueID, description string) error
	UpdateNotes(issueID, notes string) error
	CloseIssue(issueID, reason string) error
	ReopenIssue(issueID string) error
	SetLabels(issueID string, labels []string) error
	AddComment(issueID, author, text string) error
	CreateEpic(title, description string, labels []string) (domain.CreateResult, error)
	CreateTask(title, description, parentID, assignee string, labels []string) (domain.CreateResult, error)
	DeleteIssues(issueIDs []string) error
	AddDependency(taskID, dependsOnID string) error
	UpdateIssue(issueID string, opts domain.UpdateIssueOptions) error
}

// IssueExecutor combines read and write operations for issues.
// This is the full interface implemented by BDExecutor.
type IssueExecutor interface {
	IssueReader
	IssueWriter
}
