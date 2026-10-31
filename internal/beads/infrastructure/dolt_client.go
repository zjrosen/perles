package infrastructure

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
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

// errNoServerPort is returned for a server-mode workspace whose server port
// could not be found: bd writes .beads/dolt-server.port when it starts the
// server, so a missing port file means the server is not running.
var errNoServerPort = errors.New("no Dolt server port found (.beads/dolt-server.port is missing)")

// minDoltServerVersion is the oldest dolt sql-server bd accepts in server mode
// (bd's doltversion.RequiredMin).
const minDoltServerVersion = "2.6.0"

// probeTimeout bounds the fail-fast reachability probes made before the MySQL
// handshake.
const probeTimeout = 500 * time.Millisecond

// dialProbe reports whether network/addr accepts a connection. A variable so
// tests can stub connectivity.
var dialProbe = func(network, addr string, timeout time.Duration) error {
	conn, err := (&net.Dialer{Timeout: timeout}).DialContext(context.Background(), network, addr)
	if err != nil {
		return err
	}
	return conn.Close()
}

// NewDoltServerClient connects to the dolt sql-server described by conn over
// the MySQL protocol.
func NewDoltServerClient(conn Connection) (*DoltClient, error) {
	if !conn.Dialable() {
		return nil, errNoServerPort
	}

	socket := socketTransport(conn)
	if socket == "" {
		// Fail fast before the MySQL handshake when nothing is listening.
		if err := dialProbe("tcp", conn.Addr(), probeTimeout); err != nil {
			return nil, err
		}
	}

	db, err := sql.Open("mysql", conn.dsn(socket))
	if err != nil {
		return nil, fmt.Errorf("opening dolt server connection: %w", err)
	}

	// Server mode: allow concurrent connections
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	log.Info(log.CatDB, "Connected to Dolt server", "location", conn.Location(), "database", conn.Database)
	warnIfOldDoltServer(ctx, db, conn.Location())

	return &DoltClient{db: db, doltDir: filepath.Join(conn.BeadsDir, "dolt")}, nil
}

// socketTransport is bd's socket-first, TCP-fallback policy: the unix socket
// to dial, or "" for TCP. A configured socket is used when it accepts
// connections; when it does not but TCP does, TCP; when neither does, the
// socket, so the error names what the workspace configured.
func socketTransport(conn Connection) string {
	if conn.Socket == "" {
		return ""
	}
	if dialProbe("unix", conn.Socket, probeTimeout) == nil {
		return conn.Socket
	}
	if conn.Port > 0 && dialProbe("tcp", conn.Addr(), probeTimeout) == nil {
		log.Debug(log.CatDB, "Dolt socket unreachable, falling back to TCP", "socket", conn.Socket, "addr", conn.Addr())
		return ""
	}
	return conn.Socket
}

// warnIfOldDoltServer logs a warning when the server runs a Dolt older than bd
// requires in server mode. A version that cannot be read is not reported.
func warnIfOldDoltServer(ctx context.Context, db *sql.DB, location string) {
	var version string
	if err := db.QueryRowContext(ctx, "SELECT dolt_version()").Scan(&version); err != nil {
		log.Debug(log.CatDB, "Could not read the Dolt server version", "location", location, "error", err)
		return
	}
	if domain.CompareVersions(baseVersion(version), minDoltServerVersion) < 0 {
		log.Warn(log.CatDB, "Dolt server is older than bd supports in server mode",
			"location", location, "version", version, "required", minDoltServerVersion)
	}
}

// Close closes the database connection.
func (c *DoltClient) Close() error {
	return c.db.Close()
}

// DBPath returns the path to the Dolt database directory. The file watcher
// watches its parent, the .beads directory, for bd's last-touched sentinel.
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

// Version returns the beads version from the database.
// It first tries local_metadata (new location), then falls back to metadata (legacy).
func (c *DoltClient) Version() (string, error) {
	var version string

	err := c.db.QueryRowContext(context.Background(), "SELECT `value` FROM local_metadata WHERE `key` = ?", "bd_version").Scan(&version)
	if err == nil {
		return version, nil
	}

	err = c.db.QueryRowContext(context.Background(), "SELECT `value` FROM metadata WHERE `key` = ?", "bd_version").Scan(&version)
	if err != nil {
		return "", fmt.Errorf("reading bd_version: %w", err)
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
	rows, err := c.db.QueryContext(context.Background(), query, issueID)
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
