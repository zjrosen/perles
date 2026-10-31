package infrastructure

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// sqliteFile is the start of a real SQLite database file.
const sqliteFile = "SQLite format 3\x00rest-of-page"

// addEmbeddedRepo creates a non-empty embedded Dolt repository.
func addEmbeddedRepo(t *testing.T, beadsDir string) {
	t.Helper()
	writeFile(t, beadsDir, "embeddeddolt/beads/.dolt/noms/manifest", "x")
}

func TestClassifyWorkspace(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T, beadsDir string)
		wantKind   WorkspaceKind // "" means supported
		wantDetail string
	}{
		{
			name: "dolt server workspace",
			setup: func(t *testing.T, d string) {
				writeMetadata(t, d, `{"backend":"dolt","dolt_mode":"server"}`)
			},
		},
		{
			name: "dolt server workspace with 0-byte beads.db (perles repo layout)",
			setup: func(t *testing.T, d string) {
				writeMetadata(t, d, `{"database":"dolt","backend":"dolt","dolt_mode":"server","dolt_database":"perles"}`)
				writeFile(t, d, "beads.db", "")
				writeFile(t, d, "dolt/perles/.dolt/manifest", "x")
				addEmbeddedRepo(t, d)
			},
		},
		{
			name: "graph_mode dependency is an issue workspace",
			setup: func(t *testing.T, d string) {
				writeMetadata(t, d, `{"backend":"dolt","dolt_mode":"server","graph_mode":"dependency"}`)
			},
		},
		{
			name: "graph_mode link",
			setup: func(t *testing.T, d string) {
				writeMetadata(t, d, `{"backend":"dolt","dolt_mode":"server","graph_mode":"link"}`)
			},
			wantKind: WorkspaceGraph, wantDetail: `graph_mode "link"`,
		},
		{
			name: "unknown graph_mode",
			setup: func(t *testing.T, d string) {
				writeMetadata(t, d, `{"graph_mode":"hypergraph"}`)
			},
			wantKind: WorkspaceGraph, wantDetail: `graph_mode "hypergraph"`,
		},
		{
			name: "graph-preview-format marker",
			setup: func(t *testing.T, d string) {
				writeMetadata(t, d, `{"backend":"dolt","dolt_mode":"server"}`)
				writeFile(t, d, "graph-preview-format", "")
			},
			wantKind: WorkspaceGraph, wantDetail: ".beads/graph-preview-format marker",
		},
		{
			name: "graph marker without metadata",
			setup: func(t *testing.T, d string) {
				writeFile(t, d, "graph-preview-format", "1")
			},
			wantKind: WorkspaceGraph, wantDetail: ".beads/graph-preview-format marker",
		},
		{
			name: "SQLite-era metadata with beads.db",
			setup: func(t *testing.T, d string) {
				writeMetadata(t, d, `{"database":"beads.db","jsonl_export":"issues.jsonl"}`)
				writeFile(t, d, "beads.db", sqliteFile)
			},
			wantKind: WorkspaceLegacySQLite,
		},
		{
			name: "explicit sqlite backend with custom path",
			setup: func(t *testing.T, d string) {
				writeMetadata(t, d, `{"backend":"sqlite","sqlite_path":"vc.db"}`)
				writeFile(t, d, "vc.db", sqliteFile)
			},
			wantKind: WorkspaceLegacySQLite,
		},
		{
			name: "explicit sqlite backend without a database file",
			setup: func(t *testing.T, d string) {
				writeMetadata(t, d, `{"backend":"sqlite"}`)
			},
			wantKind: WorkspaceLegacySQLite,
		},
		{
			name: "SQLite-era metadata with empty beads.db is not legacy",
			setup: func(t *testing.T, d string) {
				writeMetadata(t, d, `{"database":"beads.db"}`)
				writeFile(t, d, "beads.db", "")
			},
		},
		{
			name: "SQLite-era metadata beside an embedded repo is not legacy",
			setup: func(t *testing.T, d string) {
				writeMetadata(t, d, `{"database":"beads.db"}`)
				writeFile(t, d, "beads.db", sqliteFile)
				addEmbeddedRepo(t, d)
			},
		},
		{
			name: "no metadata, SQLite beads.db",
			setup: func(t *testing.T, d string) {
				writeFile(t, d, "beads.db", sqliteFile)
				writeFile(t, d, "beads.db-wal", "")
				writeFile(t, d, "issues.jsonl", "{}\n")
				writeFile(t, d, "config.yaml", "issue-prefix: bd\n")
			},
			wantKind: WorkspaceLegacySQLite,
		},
		{
			name: "no metadata, non-SQLite .db file",
			setup: func(t *testing.T, d string) {
				writeFile(t, d, "beads.db", "not sqlite at all")
			},
		},
		{
			name: "no metadata, 0-byte beads.db",
			setup: func(t *testing.T, d string) {
				writeFile(t, d, "beads.db", "")
			},
		},
		{
			name: "no metadata, SQLite beads.db beside Dolt server data",
			setup: func(t *testing.T, d string) {
				writeFile(t, d, "beads.db", sqliteFile)
				writeFile(t, d, "dolt/beads/.dolt/manifest", "x")
			},
		},
		{
			name: "no metadata, two SQLite databases is ambiguous",
			setup: func(t *testing.T, d string) {
				writeFile(t, d, "beads.db", sqliteFile)
				writeFile(t, d, "vc.db", sqliteFile)
			},
		},
		{
			name: "http backend",
			setup: func(t *testing.T, d string) {
				writeMetadata(t, d, `{"backend":"http"}`)
			},
			wantKind: WorkspaceHTTPBackend,
		},
		{
			name: "removed postgres backend",
			setup: func(t *testing.T, d string) {
				writeMetadata(t, d, `{"backend":"postgres"}`)
			},
			wantKind: WorkspaceUnknownBackend, wantDetail: "postgres",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			beadsDir := newBeadsDir(t)
			tt.setup(t, beadsDir)
			meta, err := LoadMetadata(beadsDir)
			require.NoError(t, err)

			err = classifyWorkspace(beadsDir, meta)
			if tt.wantKind == "" {
				require.NoError(t, err)
				return
			}
			uw, ok := err.(*UnsupportedWorkspaceError)
			require.True(t, ok, "expected *UnsupportedWorkspaceError, got %v", err)
			require.Equal(t, tt.wantKind, uw.Kind)
			require.Equal(t, tt.wantDetail, uw.Detail)
			require.Equal(t, beadsDir, uw.BeadsDir)
			require.NotEmpty(t, uw.Error())
		})
	}
}

func TestHasEmbeddedDoltRepository(t *testing.T) {
	beadsDir := newBeadsDir(t)
	require.False(t, hasEmbeddedDoltRepository(beadsDir))

	require.NoError(t, os.MkdirAll(filepath.Join(beadsDir, "embeddeddolt", "beads", ".dolt"), 0o750))
	require.False(t, hasEmbeddedDoltRepository(beadsDir), "an empty .dolt is not a repository")

	addEmbeddedRepo(t, beadsDir)
	require.True(t, hasEmbeddedDoltRepository(beadsDir))
}

func TestUnsupportedWorkspaceError_Messages(t *testing.T) {
	for _, kind := range []WorkspaceKind{
		WorkspaceEmbeddedDolt, WorkspaceProxiedServer, WorkspaceHTTPBackend,
		WorkspaceGraph, WorkspaceLegacySQLite, WorkspaceUnknownBackend,
	} {
		err := &UnsupportedWorkspaceError{Kind: kind, BeadsDir: "/p/.beads", Detail: "d"}
		require.Contains(t, err.Error(), "/p/.beads", "kind %s", kind)
	}
}
