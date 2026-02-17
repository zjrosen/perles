package infrastructure

import (
	"database/sql"
	"fmt"
	"path/filepath"

	appbeads "github.com/zjrosen/perles/internal/beads/application"
	domain "github.com/zjrosen/perles/internal/beads/domain"
	"github.com/zjrosen/perles/internal/log"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

// Compile-time check that SQLiteClient implements required interfaces.
var (
	_ appbeads.DBClient      = (*SQLiteClient)(nil)
	_ appbeads.VersionReader = (*SQLiteClient)(nil)
	_ appbeads.CommentReader = (*SQLiteClient)(nil)
)

// SQLiteClient provides read access to the beads SQLite database.
type SQLiteClient struct {
	db     *sql.DB
	dbPath string
}

// NewSQLiteClient creates a client connected to the beads database.
// beadsDir should be the resolved .beads directory path.
func NewSQLiteClient(beadsDir string) (*SQLiteClient, error) {
	dbPath := filepath.Join(beadsDir, "beads.db")
	log.Debug(log.CatDB, "Opening database", "path", dbPath)
	db, err := sql.Open("sqlite3", "file:"+dbPath+"?mode=ro")
	if err != nil {
		log.ErrorErr(log.CatDB, "Failed to open database", err, "path", dbPath)
		return nil, err
	}
	if err := db.Ping(); err != nil {
		log.ErrorErr(log.CatDB, "Failed to ping database", err, "path", dbPath)
		return nil, err
	}
	log.Info(log.CatDB, "Connected to database", "path", dbPath)
	return &SQLiteClient{db: db, dbPath: dbPath}, nil
}

// Close closes the database connection.
func (c *SQLiteClient) Close() error {
	return c.db.Close()
}

// DBPath returns the resolved path to the beads.db file.
func (c *SQLiteClient) DBPath() string {
	return c.dbPath
}

// DB returns the underlying database connection.
// Used by BQL executor to run queries directly.
func (c *SQLiteClient) DB() *sql.DB {
	return c.db
}

// Dialect returns the SQL dialect (SQLite).
func (c *SQLiteClient) Dialect() appbeads.SQLDialect {
	return appbeads.DialectSQLite
}

// Version returns the beads version from the database metadata table.
func (c *SQLiteClient) Version() (string, error) {
	var version string
	err := c.db.QueryRow("SELECT value FROM metadata WHERE key = 'bd_version'").Scan(&version)
	if err != nil {
		return "", fmt.Errorf("reading bd_version from metadata: %w", err)
	}
	return version, nil
}

// GetComments fetches comments for an issue.
func (c *SQLiteClient) GetComments(issueID string) ([]domain.Comment, error) {
	query := `
		SELECT id, author, text, created_at
		FROM comments
		WHERE issue_id = ?
		ORDER BY created_at ASC
	`
	rows, err := c.db.Query(query, issueID)
	if err != nil {
		log.ErrorErr(log.CatDB, "GetComments query failed", err, "issueID", issueID)
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var comments []domain.Comment
	for rows.Next() {
		var comment domain.Comment
		if err := rows.Scan(&comment.ID, &comment.Author, &comment.Text, &comment.CreatedAt); err != nil {
			log.ErrorErr(log.CatDB, "GetComments scan failed", err, "issueID", issueID)
			return nil, err
		}
		comments = append(comments, comment)
	}
	return comments, rows.Err()
}
