package task

import (
	"fmt"
	"net"
	"strconv"
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
	// WorkspaceEmbeddedDolt is a beads workspace using embedded Dolt, which has
	// no SQL server for perles to connect to.
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
	Detail string
}

func (e *UnsupportedWorkspaceError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("unsupported beads workspace in %s: %s (%s)", e.BeadsDir, e.Kind, e.Detail)
	}
	return fmt.Sprintf("unsupported beads workspace in %s: %s", e.BeadsDir, e.Kind)
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
