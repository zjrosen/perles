package infrastructure

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// embeddedMetadata is metadata.json for an embedded workspace whose database
// differs from the one the bd context fixtures report, so tests can tell the
// two sources apart.
const embeddedMetadata = `{"backend":"dolt","dolt_mode":"embedded","dolt_database":"meta_db"}`

// resolveEmbedded resolves beadsDir with runner and returns its embedded
// classification.
func resolveEmbedded(t *testing.T, beadsDir string, runner BDContextRunner) *EmbeddedWorkspaceError {
	t.Helper()
	conn, err := ResolveConnection(beadsDir, ResolveOptions{WorkDir: filepath.Dir(beadsDir), BDContext: runner})
	require.Nil(t, conn)
	ew, ok := errors.AsType[*EmbeddedWorkspaceError](err)
	require.True(t, ok, "got %v", err)
	_, isUnsupported := errors.AsType[*UnsupportedWorkspaceError](err)
	require.False(t, isUnsupported, "embedded is classified, not refused")
	require.Equal(t, beadsDir, ew.BeadsDir)
	return ew
}

func requireTooOld(t *testing.T, err error, version string) {
	t.Helper()
	tooOld, ok := errors.AsType[*bdContextTooOldError](err)
	require.True(t, ok, "want the too-old rejection, got %v", err)
	require.Equal(t, version, tooOld.Version)
}

func TestResolveConnection_EmbeddedClassification(t *testing.T) {
	errUnsafe := errors.New("bd context: exit status 1: Error: refusing to run in unsafe location /tmp/project")

	tests := []struct {
		name         string
		metadata     string // defaults to embeddedMetadata
		runner       BDContextRunner
		wantDatabase string
		wantVersion  string
		checkErr     func(t *testing.T, err error)
	}{
		{
			name:         "bd context answered",
			runner:       bdContextJSON(`{"backend":"dolt","dolt_mode":"embedded","database":"ctx_db","bd_version":"1.3.2 (6ebaacba8: main@6ebaacba8)"}`),
			wantDatabase: "ctx_db",
			wantVersion:  "1.3.2 (6ebaacba8: main@6ebaacba8)",
			checkErr:     func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name:         "bd context answered without a database",
			runner:       bdContextJSON(`{"backend":"dolt","dolt_mode":"embedded","database":"","bd_version":"1.0.5"}`),
			wantDatabase: "meta_db",
			wantVersion:  "1.0.5",
			checkErr:     func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name:         "bd older than 1.0.0 keeps its raw version",
			runner:       bdContextJSON(`{"backend":"dolt","dolt_mode":"embedded","database":"ctx_db","bd_version":"0.62.0"}`),
			wantDatabase: "meta_db",
			wantVersion:  "0.62.0",
			checkErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "bd 0.62.0 is older than 1.0.0")
				requireTooOld(t, err, "0.62.0")
			},
		},
		{
			name:         "empty bd_version",
			runner:       bdContextJSON(`{"backend":"dolt","dolt_mode":"embedded","database":"ctx_db","bd_version":""}`),
			wantDatabase: "meta_db",
			wantVersion:  "",
			checkErr:     func(t *testing.T, err error) { requireTooOld(t, err, "") },
		},
		{
			name:         "bd missing",
			runner:       noBDContext,
			wantDatabase: "meta_db",
			wantVersion:  "",
			checkErr:     func(t *testing.T, err error) { require.ErrorIs(t, err, exec.ErrNotFound) },
		},
		{
			name:         "bd context fails (unsafe location)",
			runner:       func(context.Context, string, string) ([]byte, error) { return nil, errUnsafe },
			wantDatabase: "meta_db",
			wantVersion:  "",
			checkErr: func(t *testing.T, err error) {
				require.ErrorIs(t, err, errUnsafe)
				require.NotErrorIs(t, err, exec.ErrNotFound)
			},
		},
		{
			name:         "bd context reports an error object",
			runner:       bdContextJSON(`{"error":"cannot resolve repo context","schema_version":1}`),
			wantDatabase: "meta_db",
			wantVersion:  "",
			checkErr: func(t *testing.T, err error) {
				require.EqualError(t, err, "bd context: cannot resolve repo context")
				_, tooOld := errors.AsType[*bdContextTooOldError](err)
				require.False(t, tooOld)
			},
		},
		{
			name:         "neither bd context nor metadata names a database",
			metadata:     `{"backend":"dolt","dolt_mode":"embedded"}`,
			runner:       noBDContext,
			wantDatabase: "",
			wantVersion:  "",
			checkErr:     func(t *testing.T, err error) { require.ErrorIs(t, err, exec.ErrNotFound) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateBeadsEnv(t)
			beadsDir := newBeadsDir(t)
			metadata := tt.metadata
			if metadata == "" {
				metadata = embeddedMetadata
			}
			writeMetadata(t, beadsDir, metadata)

			ew := resolveEmbedded(t, beadsDir, tt.runner)
			require.Equal(t, tt.wantDatabase, ew.Database, "Database")
			require.Equal(t, tt.wantVersion, ew.BDVersion, "BDVersion")
			tt.checkErr(t, ew.BDContextErr)
		})
	}
}

func TestResolveConnection_EmbeddedWithoutMetadata(t *testing.T) {
	isolateBeadsEnv(t)
	beadsDir := newBeadsDir(t)
	writeFile(t, beadsDir, "embeddeddolt/ctx_db/.dolt/noms/manifest", "manifest")

	ew := resolveEmbedded(t, beadsDir, bdContextJSON(`{"backend":"dolt","dolt_mode":"embedded","database":"ctx_db","bd_version":"1.3.2"}`))
	require.Equal(t, &EmbeddedWorkspaceError{BeadsDir: beadsDir, Database: "ctx_db", BDVersion: "1.3.2"}, ew)

	ew = resolveEmbedded(t, beadsDir, noBDContext)
	require.Empty(t, ew.Database, "no metadata and no bd context: the database is unknown")
	require.ErrorIs(t, ew.BDContextErr, exec.ErrNotFound)
}

func TestEmbeddedWorkspaceError_Error(t *testing.T) {
	err := &EmbeddedWorkspaceError{BeadsDir: "/p/.beads", Database: "beads"}
	require.EqualError(t, err, "beads workspace /p/.beads uses embedded Dolt")
}

func TestEmbeddedManifestPath(t *testing.T) {
	t.Run("existing repository", func(t *testing.T) {
		beadsDir := newBeadsDir(t)
		writeFile(t, beadsDir, "embeddeddolt/other/.dolt/noms/manifest", "other")
		want := writeFile(t, beadsDir, "embeddeddolt/perles/.dolt/noms/manifest", "5:__DOLT__:abc")

		got, err := EmbeddedManifestPath(beadsDir, "perles")
		require.NoError(t, err)
		require.Equal(t, want, got)
		require.Equal(t, filepath.Join(beadsDir, "embeddeddolt", "perles", ".dolt", "noms", "manifest"), got)
	})

	t.Run("classification feeds the resolver", func(t *testing.T) {
		isolateBeadsEnv(t)
		beadsDir := newBeadsDir(t)
		writeMetadata(t, beadsDir, `{"backend":"dolt","dolt_mode":"embedded","dolt_database":"perles"}`)
		want := writeFile(t, beadsDir, "embeddeddolt/perles/.dolt/noms/manifest", "5:__DOLT__:abc")

		ew := resolveEmbedded(t, beadsDir, noBDContext)
		got, err := EmbeddedManifestPath(ew.BeadsDir, ew.Database)
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	errorCases := []struct {
		name     string
		files    []string // files to create beneath .beads
		dirs     []string // empty directories to create beneath .beads
		database string
		wantErr  string
		notExist bool // the error wraps fs.ErrNotExist
	}{
		{
			name: "empty database", files: []string{"embeddeddolt/perles/.dolt/noms/manifest"},
			database: "", wantErr: "names no database",
		},
		{
			name: "database escapes embeddeddolt", files: []string{"embeddeddolt/perles/.dolt/noms/manifest"},
			database: "../embeddeddolt/perles", wantErr: "is not a directory name",
		},
		{name: "database is dot-dot", database: "..", wantErr: "is not a directory name"},
		{name: "database is dot", database: ".", wantErr: "is not a directory name"},
		{name: "no embeddeddolt directory", database: "perles", wantErr: "holds no embedded Dolt repository", notExist: true},
		{
			name: "empty .dolt directory", dirs: []string{"embeddeddolt/perles/.dolt"},
			database: "perles", wantErr: "holds no embedded Dolt repository", notExist: true,
		},
		{
			name: "repository for another database only", files: []string{"embeddeddolt/other/.dolt/noms/manifest"},
			database: "perles", wantErr: `embedded Dolt database "perles" has no repository`, notExist: true,
		},
		{
			name: "missing manifest", files: []string{"embeddeddolt/perles/.dolt/config.json"},
			database: "perles", wantErr: `embedded Dolt manifest for database "perles"`, notExist: true,
		},
		{
			name: "manifest is a directory", files: []string{"embeddeddolt/perles/.dolt/noms/manifest/x"},
			database: "perles", wantErr: "is not a regular file",
		},
	}
	for _, tt := range errorCases {
		t.Run(tt.name, func(t *testing.T) {
			beadsDir := newBeadsDir(t)
			for _, f := range tt.files {
				writeFile(t, beadsDir, f, "x")
			}
			for _, d := range tt.dirs {
				require.NoError(t, os.MkdirAll(filepath.Join(beadsDir, d), 0o750))
			}

			got, err := EmbeddedManifestPath(beadsDir, tt.database)
			require.Empty(t, got)
			require.ErrorContains(t, err, tt.wantErr)
			if tt.notExist {
				require.ErrorIs(t, err, fs.ErrNotExist)
			}
		})
	}
}
