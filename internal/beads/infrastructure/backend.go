package infrastructure

import (
	"fmt"

	appbeads "github.com/zjrosen/perles/internal/beads/application"
	"github.com/zjrosen/perles/internal/log"
)

// NewClient connects to the beads workspace at beadsDir (the resolved .beads
// directory). Beads stores issues in Dolt, and perles reads them through the
// workspace's Dolt SQL server.
//
// It returns *EmbeddedWorkspaceError for an embedded Dolt workspace, which has
// no server to connect to and must be read another way,
// *UnsupportedWorkspaceError for workspaces perles cannot read
// (proxied-server Dolt, the http backend, graph formats, historical SQLite),
// an error wrapping ErrWorkspaceNotFound when beadsDir holds no workspace, and
// *ServerUnreachableError when the workspace is in server mode but its server
// cannot be reached.
func NewClient(beadsDir string, opts ResolveOptions) (appbeads.DBClient, error) {
	conn, err := ResolveConnection(beadsDir, opts)
	if err != nil {
		return nil, err
	}

	log.Info(log.CatDB, "Connecting to Dolt server",
		"location", conn.Location(),
		"portSource", string(conn.PortSource),
		"user", conn.User,
		"database", conn.Database,
		"tls", conn.TLS,
		"sharedServer", conn.SharedServer,
		"fromBDContext", conn.FromBDContext)

	client, err := NewDoltServerClient(*conn)
	if err != nil {
		return nil, &ServerUnreachableError{Conn: *conn, Err: err}
	}
	return client, nil
}

// ServerUnreachableError reports that a server-mode workspace's Dolt server
// could not be reached (or refused the connection).
type ServerUnreachableError struct {
	Conn Connection
	Err  error
}

func (e *ServerUnreachableError) Error() string {
	return fmt.Sprintf("dolt server unreachable at %s: %v", e.Conn.Location(), e.Err)
}

func (e *ServerUnreachableError) Unwrap() error {
	return e.Err
}
