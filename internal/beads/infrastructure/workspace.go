package infrastructure

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// WorkspaceKind names a beads workspace layout perles cannot read.
type WorkspaceKind string

const (
	// WorkspaceEmbeddedDolt is Dolt in embedded mode (the database runs inside
	// bd, with no SQL server) that perles' snapshot reader cannot start on.
	// Detail says why.
	WorkspaceEmbeddedDolt WorkspaceKind = "embedded"
	// WorkspaceProxiedServer is Dolt in proxied-server mode (dolt_mode
	// "proxied-server"), which perles does not read yet.
	WorkspaceProxiedServer WorkspaceKind = "proxied-server"
	// WorkspaceHTTPBackend is a workspace attached to a remote `bd serve` with
	// `bd connect` (backend "http").
	WorkspaceHTTPBackend WorkspaceKind = "http"
	// WorkspaceGraph is a graph-model workspace (graph_mode other than
	// "dependency", or a .beads/graph-preview-format marker).
	WorkspaceGraph WorkspaceKind = "graph"
	// WorkspaceLegacySQLite is a pre-Dolt SQLite workspace, which current bd
	// refuses until it is migrated explicitly.
	WorkspaceLegacySQLite WorkspaceKind = "legacy-sqlite"
	// WorkspaceUnknownBackend is any other backend (such as the removed
	// postgres and mysql backends).
	WorkspaceUnknownBackend WorkspaceKind = "backend"
)

// UnsupportedWorkspaceError reports a beads workspace perles cannot read.
type UnsupportedWorkspaceError struct {
	Kind     WorkspaceKind
	BeadsDir string
	// Detail qualifies the kind: the backend name for WorkspaceUnknownBackend,
	// what declared the graph format for WorkspaceGraph.
	//
	// For WorkspaceEmbeddedDolt it is why the snapshot reader cannot start,
	// such as CheckEmbeddedBDVersion's "embedded workspaces need bd >= 1.0.5
	// (found X)". It carries no cause: mapping the error to
	// task.UnsupportedWorkspaceError must prefix one (see that Detail's
	// "<cause>: <text>" format). The embedded bd version gate always sets it.
	Detail string
}

func (e *UnsupportedWorkspaceError) Error() string {
	switch e.Kind {
	case WorkspaceEmbeddedDolt:
		if e.Detail != "" {
			return fmt.Sprintf("beads workspace %s uses embedded Dolt, but perles' snapshot reader cannot start: %s", e.BeadsDir, e.Detail)
		}
		return fmt.Sprintf("beads workspace %s uses embedded Dolt, and perles' snapshot reader cannot start", e.BeadsDir)
	case WorkspaceProxiedServer:
		return fmt.Sprintf("beads workspace %s uses Dolt proxied-server mode, which perles does not support", e.BeadsDir)
	case WorkspaceHTTPBackend:
		return fmt.Sprintf("beads workspace %s uses the http backend (bd connect), which perles does not support", e.BeadsDir)
	case WorkspaceGraph:
		return fmt.Sprintf("beads workspace %s uses a graph workspace format (%s), which perles does not support", e.BeadsDir, e.Detail)
	case WorkspaceLegacySQLite:
		return fmt.Sprintf("beads workspace %s is a historical SQLite workspace; explicit migration is required", e.BeadsDir)
	default:
		return fmt.Sprintf("beads workspace %s uses unsupported backend %q", e.BeadsDir, e.Detail)
	}
}

// ErrWorkspaceNotFound reports that the beads directory does not exist or
// holds no beads workspace.
var ErrWorkspaceNotFound = errors.New("no beads workspace found")

// graphFormatMarkerFile is the sentinel graph-preview bd builds write in .beads
// (configfile.GraphFormatMarkerFile).
const graphFormatMarkerFile = "graph-preview-format"

// classifyWorkspace refuses workspaces whose format or backend perles cannot
// read, in bd's admission order: graph format, historical SQLite, backend.
// meta is the workspace metadata (nil when there is none).
func classifyWorkspace(beadsDir string, meta *BeadsMetadata) error {
	if meta != nil {
		if mode := strings.TrimSpace(meta.GraphMode); mode != "" && mode != "dependency" {
			return &UnsupportedWorkspaceError{Kind: WorkspaceGraph, BeadsDir: beadsDir, Detail: fmt.Sprintf("graph_mode %q", mode)}
		}
	}
	if hasGraphFormatMarker(beadsDir) {
		return graphMarkerError(beadsDir)
	}
	if isHistoricalSQLiteWorkspace(beadsDir, meta) {
		return &UnsupportedWorkspaceError{Kind: WorkspaceLegacySQLite, BeadsDir: beadsDir}
	}
	if meta == nil {
		return nil
	}
	return checkBackend(beadsDir, meta.Backend)
}

// checkBackend refuses every backend but Dolt ("" is bd's legacy spelling of
// Dolt).
func checkBackend(beadsDir, backend string) error {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case "", "dolt":
		return nil
	case "sqlite":
		return &UnsupportedWorkspaceError{Kind: WorkspaceLegacySQLite, BeadsDir: beadsDir}
	case "http":
		return &UnsupportedWorkspaceError{Kind: WorkspaceHTTPBackend, BeadsDir: beadsDir}
	default:
		return &UnsupportedWorkspaceError{Kind: WorkspaceUnknownBackend, BeadsDir: beadsDir, Detail: backend}
	}
}

func graphMarkerError(beadsDir string) error {
	return &UnsupportedWorkspaceError{Kind: WorkspaceGraph, BeadsDir: beadsDir, Detail: ".beads/" + graphFormatMarkerFile + " marker"}
}

func hasGraphFormatMarker(beadsDir string) bool {
	_, err := os.Lstat(filepath.Join(beadsDir, graphFormatMarkerFile))
	return err == nil
}

// isHistoricalSQLiteWorkspace reports whether beadsDir is a pre-Dolt SQLite
// workspace whose SQLite database still holds its data
// (configfile.IsHistoricalSQLiteWorkspace). It never applies when an embedded
// Dolt repository exists. With metadata, the metadata must name a bare SQLite
// database file (and no Dolt configuration) that exists and is non-empty.
// Without metadata, the workspace must hold no Dolt data and exactly one
// non-empty .db file with a SQLite header; a 0-byte beads.db left beside a
// Dolt workspace does not count.
func isHistoricalSQLiteWorkspace(beadsDir string, meta *BeadsMetadata) bool {
	if hasEmbeddedDoltRepository(beadsDir) {
		return false
	}
	if meta != nil {
		return configuredSQLiteWorkspace(beadsDir, meta)
	}
	if hasDoltData(beadsDir) {
		return false
	}
	name, ok := soleSQLiteDatabase(beadsDir)
	return ok && hasSQLiteHeader(filepath.Join(beadsDir, name))
}

func configuredSQLiteWorkspace(beadsDir string, meta *BeadsMetadata) bool {
	if meta.Backend != "" && meta.Backend != "sqlite" {
		return false
	}
	if meta.DoltMode != "" || meta.DoltDatabase != "" {
		return false
	}
	name := meta.SQLitePath
	if name == "" {
		name = meta.Database
	}
	if name == "" {
		name = "beads.db"
	}
	if filepath.Base(name) != name {
		return false
	}
	return isNonEmptyRegularFile(filepath.Join(beadsDir, name))
}

// soleSQLiteDatabase returns the single non-empty regular .db file directly
// in beadsDir. Empty .db files are ignored; two candidates, or a .db symlink,
// fail closed.
func soleSQLiteDatabase(beadsDir string) (string, bool) {
	entries, err := os.ReadDir(beadsDir)
	if err != nil {
		return "", false
	}
	found := ""
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".db") {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return "", false
		}
		if !isNonEmptyRegularFile(filepath.Join(beadsDir, name)) {
			continue
		}
		if found != "" {
			return "", false
		}
		found = name
	}
	return found, found != ""
}

// hasEmbeddedDoltRepository reports whether beadsDir/embeddeddolt holds a
// database directory with a non-empty .dolt directory
// (configfile.HasEmbeddedDoltRepository).
func hasEmbeddedDoltRepository(beadsDir string) bool {
	root := filepath.Join(beadsDir, embeddedDoltDir)
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() {
		return false
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		if !isNonEmptyDir(filepath.Join(root, entry.Name(), ".dolt")) {
			continue
		}
		return true
	}
	return false
}

// hasDoltData reports whether beadsDir holds any Dolt data: an embedded
// repository or a non-empty dolt/ server data directory.
func hasDoltData(beadsDir string) bool {
	return hasEmbeddedDoltRepository(beadsDir) || isNonEmptyDir(filepath.Join(beadsDir, "dolt"))
}

// hasWorkspaceState reports whether beadsDir holds anything bd would recognize
// as a workspace: metadata, a config.yaml, or Dolt data directories.
func hasWorkspaceState(beadsDir string, meta *BeadsMetadata) bool {
	return meta != nil ||
		fileExists(filepath.Join(beadsDir, "config.yaml")) ||
		dirExists(filepath.Join(beadsDir, embeddedDoltDir)) ||
		dirExists(filepath.Join(beadsDir, "dolt"))
}

func isNonEmptyDir(path string) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() {
		return false
	}
	dir, err := os.Open(path) //nolint:gosec // bounded probe beneath the .beads dir
	if err != nil {
		return false
	}
	defer func() { _ = dir.Close() }()
	names, _ := dir.Readdirnames(1)
	return len(names) > 0
}

func isNonEmptyRegularFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}

const sqliteHeader = "SQLite format 3\x00"

func hasSQLiteHeader(path string) bool {
	f, err := os.Open(path) //nolint:gosec // bounded header read beneath the .beads dir
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	header := make([]byte, len(sqliteHeader))
	if _, err := io.ReadFull(f, header); err != nil {
		return false
	}
	return string(header) == sqliteHeader
}
