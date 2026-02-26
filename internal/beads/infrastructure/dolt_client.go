package infrastructure

import (
	"database/sql"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/go-sql-driver/mysql"

	appbeads "github.com/zjrosen/perles/internal/beads/application"
	"github.com/zjrosen/perles/internal/beads/domain"
	"github.com/zjrosen/perles/internal/log"
)

// mysqlLogger routes the MySQL driver's internal log output through the
// perles debug logger instead of writing directly to stderr.
type mysqlLogger struct{}

func (mysqlLogger) Print(v ...any) {
	log.Debug(log.CatDB, fmt.Sprint(v...))
}

func init() {
	// Redirect the MySQL driver's built-in logger through our debug logger
	// to prevent connection pool messages (e.g. "closing bad idle connection: EOF")
	// from writing to stderr and corrupting the TUI.
	_ = mysql.SetLogger(mysqlLogger{})
}

// Compile-time check that DoltClient implements required interfaces.
var (
	_ appbeads.DBClient      = (*DoltClient)(nil)
	_ appbeads.VersionReader = (*DoltClient)(nil)
	_ appbeads.CommentReader = (*DoltClient)(nil)
)

// DoltClient provides read access to the beads Dolt database.
type DoltClient struct {
	db      *sql.DB
	doltDir string // Path to .beads/dolt/ directory
}

// NewDoltServerClient creates a client connected to a running dolt sql-server via MySQL protocol.
func NewDoltServerClient(beadsDir, databaseName, host string, port int, user string) (*DoltClient, error) {
	doltDir := filepath.Join(beadsDir, "dolt")
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))

	log.Debug(log.CatDB, "Connecting to Dolt server", "addr", addr, "database", databaseName)

	// Fail-fast TCP check before MySQL protocol handshake
	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		return nil, fmt.Errorf("dolt server unreachable at %s: %w\n\n"+
			"The Dolt server may not be running. Start it with:\n"+
			"bd dolt start",
			addr, err)
	}
	_ = conn.Close()

	// Build MySQL DSN
	password := ""
	if p := lookupEnv("BEADS_DOLT_PASSWORD"); p != "" {
		password = ":" + p
	}
	dsn := fmt.Sprintf("%s%s@tcp(%s)/%s?parseTime=true",
		user, password, addr, databaseName)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening dolt server connection: %w", err)
	}

	// Server mode: allow concurrent connections
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("pinging dolt server at %s: %w", addr, err)
	}

	log.Info(log.CatDB, "Connected to Dolt server",
		"addr", addr, "database", databaseName)

	return &DoltClient{db: db, doltDir: doltDir}, nil
}

// lookupEnv returns the value of an environment variable, or empty string if not set.
func lookupEnv(key string) string {
	v, _ := os.LookupEnv(key)
	return v
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
