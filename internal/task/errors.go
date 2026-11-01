package task

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// ServerDownError indicates the backend's server is not reachable.
// This is used by backends that require an external server process
// (e.g., Dolt in server mode).
type ServerDownError struct {
	Host string
	Port int // 0 when no port could be found
	// Socket is the unix socket the backend tried, if any.
	Socket string
	// ManagedByBD reports that bd starts this server itself (`bd dolt start`),
	// as opposed to an external server someone else runs.
	ManagedByBD bool
	// Err is the underlying connection error.
	Err error
}

// Address names the server: host:port, unix:<socket>, or the host alone when
// no port was found.
func (e *ServerDownError) Address() string {
	switch {
	case e.Socket != "":
		return "unix:" + e.Socket
	case e.Port > 0:
		return net.JoinHostPort(e.Host, strconv.Itoa(e.Port))
	default:
		return e.Host
	}
}

func (e *ServerDownError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("backend server unreachable at %s: %v", e.Address(), e.Err)
	}
	return fmt.Sprintf("backend server unreachable at %s", e.Address())
}

func (e *ServerDownError) Unwrap() error {
	return e.Err
}

// UnsupportedWorkspaceKind names a workspace layout perles cannot read.
type UnsupportedWorkspaceKind string

const (
	// WorkspaceEmbeddedDolt is a beads workspace using embedded Dolt that
	// perles' snapshot reader cannot start on. UnsupportedWorkspaceError.Detail
	// says why.
	WorkspaceEmbeddedDolt UnsupportedWorkspaceKind = "embedded"
	// WorkspaceProxiedServer is a beads workspace in Dolt proxied-server mode.
	WorkspaceProxiedServer UnsupportedWorkspaceKind = "proxied-server"
	// WorkspaceHTTPBackend is a beads workspace attached to a remote `bd serve`
	// with `bd connect` (backend "http").
	WorkspaceHTTPBackend UnsupportedWorkspaceKind = "http"
	// WorkspaceGraph is a graph-model beads workspace (graph_mode other than
	// "dependency", or a graph-preview-format marker).
	WorkspaceGraph UnsupportedWorkspaceKind = "graph"
	// WorkspaceLegacySQLite is a pre-Dolt SQLite beads workspace that needs an
	// explicit migration.
	WorkspaceLegacySQLite UnsupportedWorkspaceKind = "legacy-sqlite"
	// WorkspaceUnknownBackend is a workspace whose storage backend perles
	// cannot read (Detail names it).
	WorkspaceUnknownBackend UnsupportedWorkspaceKind = "backend"
)

// UnsupportedWorkspaceError indicates the backend found a workspace in a mode
// or format perles cannot read.
type UnsupportedWorkspaceError struct {
	Kind     UnsupportedWorkspaceKind
	BeadsDir string
	// Detail qualifies Kind: the backend name for WorkspaceUnknownBackend, or
	// what declared the format for WorkspaceGraph.
	//
	// For WorkspaceEmbeddedDolt it is "<cause>: <text>": cause is an
	// EmbeddedCause constant and text says what failed (ParseEmbeddedDetail
	// splits the two). An empty Detail means the workspace is embedded and
	// this perles build cannot read it.
	Detail string
}

func (e *UnsupportedWorkspaceError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("unsupported beads workspace in %s: %s (%s)", e.BeadsDir, e.Kind, e.Detail)
	}
	return fmt.Sprintf("unsupported beads workspace in %s: %s", e.BeadsDir, e.Kind)
}

// EmbeddedCause names why perles' snapshot reader cannot start on an embedded
// Dolt workspace. It is the cause in a WorkspaceEmbeddedDolt Detail,
// "<cause>: <text>".
type EmbeddedCause string

const (
	// EmbeddedCauseBDNotFound is bd missing from PATH. The text contains
	// "bd CLI not found on PATH".
	EmbeddedCauseBDNotFound EmbeddedCause = "bd-not-found"
	// EmbeddedCauseBDVersion is the embedded bd version gate. The text is
	// "embedded workspaces need bd >= 1.0.5 (found X)", or
	// "embedded workspaces need bd >= 1.0.5 (could not read the bd version:
	// <reason>)".
	EmbeddedCauseBDVersion EmbeddedCause = "bd-version"
	// EmbeddedCauseBDError is any other bd failure. The text is the bd error
	// with perles' hint, e.g. "bd export failed: beads database needs a schema
	// migration: run `bd migrate schema` — ...".
	EmbeddedCauseBDError EmbeddedCause = "bd-error"
	// EmbeddedCauseWorkspace is a failure to resolve the embedded database or
	// its manifest, or any other error. The text describes it.
	EmbeddedCauseWorkspace EmbeddedCause = "workspace"
)

// ParseEmbeddedDetail splits a WorkspaceEmbeddedDolt Detail, "<cause>: <text>",
// into its cause and text. A Detail without a recognised cause is a bd error
// whose text is the whole Detail. An empty (or blank) Detail gives an empty
// cause and text: the workspace is embedded and this perles build cannot read
// it.
func ParseEmbeddedDetail(detail string) (EmbeddedCause, string) {
	if strings.TrimSpace(detail) == "" {
		return "", ""
	}
	if prefix, text, ok := strings.Cut(detail, ": "); ok {
		switch cause := EmbeddedCause(prefix); cause {
		case EmbeddedCauseBDNotFound, EmbeddedCauseBDVersion, EmbeddedCauseBDError, EmbeddedCauseWorkspace:
			return cause, text
		}
	}
	return EmbeddedCauseBDError, detail
}

// VersionIncompatibleError indicates the backend data store version is too old
// for this version of perles.
type VersionIncompatibleError struct {
	Current  string // version found in the data store (or "unknown")
	Required string // minimum version required by this backend
}

func (e *VersionIncompatibleError) Error() string {
	return fmt.Sprintf("backend version %s is too old (requires %s)", e.Current, e.Required)
}
