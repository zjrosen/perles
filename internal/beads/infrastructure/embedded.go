package infrastructure

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// embeddedDoltDir is the directory under .beads that holds embedded Dolt
// databases, one subdirectory per database.
const embeddedDoltDir = "embeddeddolt"

// EmbeddedWorkspaceError reports that the workspace at BeadsDir uses embedded
// Dolt. It classifies the workspace rather than refusing it: there is no SQL
// server to connect to, so the caller must read the workspace another way.
type EmbeddedWorkspaceError struct {
	BeadsDir string
	// Database is the embedded database name as bd's configuration names it:
	// the database `bd context` reported, else what bd resolves without it
	// (BEADS_DOLT_SERVER_DATABASE, metadata.json dolt_database, then
	// "beads"). bd opens it under its sanitized name (see
	// EmbeddedManifestPath). A rejected bd context (older than 1.0.0)
	// contributes only BDVersion.
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
// the database its configuration names (workspaceConfig.database) and the
// result of queryBDContext: bdCtx when bd context answered, ctxErr otherwise.
func newEmbeddedWorkspaceError(beadsDir, database string, bdCtx *BDContext, ctxErr error) *EmbeddedWorkspaceError {
	e := &EmbeddedWorkspaceError{BeadsDir: beadsDir, Database: database, BDContextErr: ctxErr}
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
// in beadsDir, <beadsDir>/embeddeddolt/<dir>/.dolt/noms/manifest. bd rewrites
// the manifest on every command, so it is the file to watch for changes.
//
// dir is database as bd opens it: bd replaces "-" and "." with "_"
// (cmd/bd sanitizeDBName), while bd context reports the configured name
// unchanged. The database directory must be a real directory, not a symbolic
// link, as bd's own repository detection requires.
//
// It returns an error when database is empty or not a plain directory name,
// when beadsDir holds no embedded Dolt repository or no repository for
// database, or when the manifest is missing.
func EmbeddedManifestPath(beadsDir, database string) (string, error) {
	if database == "" {
		return "", fmt.Errorf("embedded Dolt workspace %s: no database name to find the manifest for", beadsDir)
	}
	dir := embeddedDatabaseDir(database)
	if database == "." || database == ".." || strings.ContainsAny(database, `/\`) || !filepath.IsLocal(dir) {
		return "", fmt.Errorf("embedded Dolt database name %q is not a directory name", database)
	}
	root := filepath.Join(beadsDir, embeddedDoltDir)
	if !hasEmbeddedDoltRepository(beadsDir) {
		return "", fmt.Errorf("%s holds no embedded Dolt repository: %w", root, fs.ErrNotExist)
	}

	dbDir := filepath.Join(root, dir)
	info, err := os.Lstat(dbDir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "", missingEmbeddedDatabaseError(root, database, dir)
	case err != nil:
		return "", fmt.Errorf("embedded Dolt database %q: %w", database, err)
	case info.Mode()&os.ModeSymlink != 0:
		return "", fmt.Errorf("embedded Dolt database %q at %s is a symbolic link, not a directory", database, dbDir)
	case !info.IsDir():
		return "", fmt.Errorf("embedded Dolt database %q at %s is not a directory", database, dbDir)
	}
	doltDir := filepath.Join(dbDir, ".dolt")
	if !isNonEmptyDir(doltDir) {
		return "", fmt.Errorf("embedded Dolt database %q has no repository at %s: %w", database, doltDir, fs.ErrNotExist)
	}
	manifest := filepath.Join(doltDir, "noms", "manifest")
	info, err = os.Stat(manifest)
	if err != nil {
		return "", fmt.Errorf("embedded Dolt manifest for database %q: %w", database, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("embedded Dolt manifest %s is not a regular file", manifest)
	}
	return manifest, nil
}

// embeddedDatabaseDir is the directory under embeddeddolt that bd opens for
// database (cmd/bd sanitizeDBName).
func embeddedDatabaseDir(database string) string {
	return strings.NewReplacer("-", "_", ".", "_").Replace(database)
}

// missingEmbeddedDatabaseError reports that root holds no directory dir for
// database. A legacy unsanitized directory is named: bd's read-only opens do
// not rename it, its next writable open does.
func missingEmbeddedDatabaseError(root, database, dir string) error {
	doltDir := filepath.Join(root, dir, ".dolt")
	if dir != database && dirExists(filepath.Join(root, database)) {
		return fmt.Errorf("embedded Dolt database %q has no repository at %s; bd opens it as %q, and renames the legacy %q directory the next time it opens the database for writing: %w",
			database, doltDir, dir, database, fs.ErrNotExist)
	}
	return fmt.Errorf("embedded Dolt database %q has no repository at %s: %w", database, doltDir, fs.ErrNotExist)
}
