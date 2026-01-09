package beads

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zjrosen/perles/internal/log"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

// Client provides access to beads data.
type Client struct {
	db     *sql.DB
	dbPath string
}

type BeadsClient interface {
	Version() (string, error)
	GetComments(issueID string) ([]Comment, error)
}

// resolveBeadsDir resolves the actual .beads directory, following redirect files
// used by git worktrees. If a redirect file exists, it contains a relative path
// to the actual .beads directory.
func resolveBeadsDir(projectPath string) string {
	beadsDir := filepath.Join(projectPath, ".beads")
	redirectPath := filepath.Join(beadsDir, "redirect")

	content, err := os.ReadFile(redirectPath) //nolint:gosec // redirect path is within .beads dir
	if err != nil {
		return beadsDir
	}

	redirectTarget := strings.TrimSpace(string(content))
	if redirectTarget == "" {
		return beadsDir
	}

	resolvedPath := filepath.Join(beadsDir, redirectTarget)
	resolvedPath = filepath.Clean(resolvedPath)

	log.Debug(log.CatDB, "Following beads redirect", "from", beadsDir, "to", resolvedPath)
	return resolvedPath
}

// NewClient creates a client connected to the beads database.
func NewClient(projectPath string) (*Client, error) {
	beadsDir := resolveBeadsDir(projectPath)
	dbPath := filepath.Join(beadsDir, "beads.db")
	log.Debug(log.CatDB, "Opening database", "path", dbPath)
	db, err := sql.Open("sqlite3", "file:"+dbPath+"?mode=ro")
	if err != nil {
		log.ErrorErr(log.CatDB, "Failed to open database", err, "path", dbPath)
		return nil, err
	}
	// Verify connection works
	if err := db.Ping(); err != nil {
		log.ErrorErr(log.CatDB, "Failed to ping database", err, "path", dbPath)
		return nil, err
	}
	log.Info(log.CatDB, "Connected to database", "path", dbPath)
	return &Client{db: db, dbPath: dbPath}, nil
}

// Close closes the database connection.
func (c *Client) Close() error {
	return c.db.Close()
}

// DBPath returns the resolved path to the beads.db file.
func (c *Client) DBPath() string {
	return c.dbPath
}

// DB returns the underlying database connection.
// Used by BQL executor to run queries directly.
func (c *Client) DB() *sql.DB {
	return c.db
}

// Version returns the beads version from the database metadata table.
func (c *Client) Version() (string, error) {
	var version string
	err := c.db.QueryRow("SELECT value FROM metadata WHERE key = 'bd_version'").Scan(&version)
	if err != nil {
		return "", fmt.Errorf("reading bd_version from metadata: %w", err)
	}
	return version, nil
}

// GetComments fetches comments for an issue.
func (c *Client) GetComments(issueID string) ([]Comment, error) {
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

	var comments []Comment
	for rows.Next() {
		var comment Comment
		if err := rows.Scan(&comment.ID, &comment.Author, &comment.Text, &comment.CreatedAt); err != nil {
			log.ErrorErr(log.CatDB, "GetComments scan failed", err, "issueID", issueID)
			return nil, err
		}
		comments = append(comments, comment)
	}
	return comments, rows.Err()
}
