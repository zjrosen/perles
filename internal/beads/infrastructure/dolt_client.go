package infrastructure

import (
	"database/sql"
	"fmt"
	"path/filepath"

	appbeads "github.com/zjrosen/perles/internal/beads/application"
	domain "github.com/zjrosen/perles/internal/beads/domain"
	"github.com/zjrosen/perles/internal/log"

	_ "github.com/dolthub/driver"
)

// Compile-time check that DoltClient implements required interfaces.
var (
	_ appbeads.DBClient      = (*DoltClient)(nil)
	_ appbeads.VersionReader = (*DoltClient)(nil)
	_ appbeads.CommentReader = (*DoltClient)(nil)
)

// DoltClient provides read access to the beads Dolt database.
type DoltClient struct {
	db      *sql.DB
	doltDir string // Path to .beads/dolt/{database} directory
}

// NewDoltClient creates a client connected to the embedded Dolt database.
// beadsDir should be the resolved .beads directory path.
// databaseName is typically "beads" (from metadata.json).
func NewDoltClient(beadsDir, databaseName string) (*DoltClient, error) {
	// The Dolt embedded driver expects the data root directory (.beads/dolt/)
	// as the path, with the database name specified via the &database= param.
	// Each subdirectory containing a .dolt/ folder is a separate database.
	dataDir := filepath.Join(beadsDir, "dolt")
	doltDir := filepath.Join(dataDir, databaseName)

	log.Debug(log.CatDB, "Opening Dolt database", "path", doltDir, "database", databaseName)

	dsn := fmt.Sprintf("file://%s?commitname=perles&commitemail=perles@local&database=%s",
		filepath.ToSlash(dataDir), databaseName)

	db, err := sql.Open("dolt", dsn)
	if err != nil {
		log.ErrorErr(log.CatDB, "Failed to open Dolt database", err, "path", doltDir)
		return nil, fmt.Errorf("opening dolt database: %w", err)
	}

	if err := db.Ping(); err != nil {
		log.ErrorErr(log.CatDB, "Failed to ping Dolt database", err, "path", doltDir)
		_ = db.Close()
		return nil, fmt.Errorf("pinging dolt database: %w", err)
	}

	log.Info(log.CatDB, "Connected to Dolt database", "path", doltDir, "database", databaseName)

	return &DoltClient{db: db, doltDir: doltDir}, nil
}

// Close closes the database connection.
func (c *DoltClient) Close() error {
	return c.db.Close()
}

// DBPath returns the path to the Dolt database directory for file watching.
func (c *DoltClient) DBPath() string {
	return c.doltDir
}

// DB returns the underlying database connection.
func (c *DoltClient) DB() *sql.DB {
	return c.db
}

// Dialect returns the SQL dialect (MySQL for Dolt).
func (c *DoltClient) Dialect() appbeads.SQLDialect {
	return appbeads.DialectMySQL
}

// Version returns the beads version from the database metadata table.
func (c *DoltClient) Version() (string, error) {
	var version string
	// `key` is a reserved word in MySQL, must be backtick-quoted
	err := c.db.QueryRow("SELECT `value` FROM metadata WHERE `key` = ?", "bd_version").Scan(&version)
	if err != nil {
		return "", fmt.Errorf("reading bd_version from metadata: %w", err)
	}
	return version, nil
}

// GetComments fetches comments for an issue.
func (c *DoltClient) GetComments(issueID string) ([]domain.Comment, error) {
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
