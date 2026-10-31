package infrastructure

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// embeddedDoltDir is the directory under .beads that holds embedded Dolt
// databases, one subdirectory per database.
const embeddedDoltDir = "embeddeddolt"

// EmbeddedWorkspaceError reports that the workspace at BeadsDir uses embedded
// Dolt. It classifies the workspace rather than refusing it: there is no SQL
// server to connect to, so the caller must read the workspace another way.
type EmbeddedWorkspaceError struct {
	BeadsDir string
	// Database is the embedded database name: the database `bd context`
	// reported, else metadata.json dolt_database. It is empty when neither
	// names one. A rejected bd context (older than 1.0.0) contributes only
	// BDVersion.
	Database string
	// BDVersion is the raw bd_version `bd context` printed, kept even when
	// perles rejected that bd as too old. It is empty when bd context printed
	// no version (bd missing, bd failed, or an empty bd_version).
	BDVersion string
	// BDContextErr is nil when bd context answered with a version of at least
	// 1.0.0, and otherwise the error that stopped perles from using it. When
	// bd is not on PATH it wraps exec.ErrNotFound.
	BDContextErr error
}

func (e *EmbeddedWorkspaceError) Error() string {
	return fmt.Sprintf("beads workspace %s uses embedded Dolt", e.BeadsDir)
}

// newEmbeddedWorkspaceError classifies the embedded workspace at beadsDir from
// its metadata (nil when absent) and the result of queryBDContext: bdCtx when
// bd context answered, ctxErr otherwise.
func newEmbeddedWorkspaceError(beadsDir string, meta *BeadsMetadata, bdCtx *BDContext, ctxErr error) *EmbeddedWorkspaceError {
	e := &EmbeddedWorkspaceError{BeadsDir: beadsDir, BDContextErr: ctxErr}
	if meta != nil {
		e.Database = meta.DoltDatabase
	}
	if bdCtx != nil {
		e.BDVersion = bdCtx.BdVersion
		if bdCtx.Database != "" {
			e.Database = bdCtx.Database
		}
	} else if tooOld, ok := errors.AsType[*bdContextTooOldError](ctxErr); ok {
		e.BDVersion = tooOld.Version
	}
	return e
}

// EmbeddedManifestPath returns the Dolt noms manifest of the embedded database
// in beadsDir, <beadsDir>/embeddeddolt/<database>/.dolt/noms/manifest. bd
// rewrites the manifest on every command, so it is the file to watch for
// changes. It returns an error when database is empty or not a plain
// directory name, when beadsDir holds no embedded Dolt repository or no
// repository for database, or when the manifest is missing.
func EmbeddedManifestPath(beadsDir, database string) (string, error) {
	if database == "" {
		return "", fmt.Errorf("embedded Dolt workspace %s names no database (bd context and metadata.json dolt_database are empty)", beadsDir)
	}
	if database == "." || database == ".." || filepath.Base(database) != database {
		return "", fmt.Errorf("embedded Dolt database name %q is not a directory name", database)
	}
	root := filepath.Join(beadsDir, embeddedDoltDir)
	if !hasEmbeddedDoltRepository(beadsDir) {
		return "", fmt.Errorf("%s holds no embedded Dolt repository: %w", root, fs.ErrNotExist)
	}
	doltDir := filepath.Join(root, database, ".dolt")
	if !isNonEmptyDir(doltDir) {
		return "", fmt.Errorf("embedded Dolt database %q has no repository at %s: %w", database, doltDir, fs.ErrNotExist)
	}
	manifest := filepath.Join(doltDir, "noms", "manifest")
	info, err := os.Stat(manifest)
	if err != nil {
		return "", fmt.Errorf("embedded Dolt manifest for database %q: %w", database, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("embedded Dolt manifest %s is not a regular file", manifest)
	}
	return manifest, nil
}
