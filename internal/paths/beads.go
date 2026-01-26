// Package paths provides path resolution utilities.
package paths

import (
	"os"
	"path/filepath"
	"strings"
)

// ResolveBeadsDir resolves the .beads directory path from user input.
// It normalizes the input (accepting either project dir or .beads dir),
// appends .beads if needed, and follows redirect files for git worktrees.
//
// Input normalization:
//   - "/path/to/project" -> "/path/to/project/.beads"
//   - "/path/to/project/.beads" -> "/path/to/project/.beads"
//   - "/path/to/beads-data" (containing beads.db) -> "/path/to/beads-data"
//   - "" -> "./.beads"
//
// Redirect handling:
//   - If .beads/redirect exists, follows it to the actual .beads location
//   - This supports git worktrees where .beads contains a redirect to main worktree
//
// Returns the resolved .beads directory path (ready to use with bd CLI).
func ResolveBeadsDir(path string) string {
	if path == "" {
		path = "."
	}
	path = filepath.Clean(path)

	// If path already ends with .beads, use it directly
	if filepath.Base(path) == ".beads" {
		return followRedirect(path)
	}

	// If path contains beads.db directly, use it as the beads directory
	// This supports BEADS_DIR pointing directly to a beads data directory
	dbPath := filepath.Join(path, "beads.db")
	if _, err := os.Stat(dbPath); err == nil {
		return followRedirect(path)
	}

	// Otherwise, append .beads to the path
	beadsDir := filepath.Join(path, ".beads")

	// Follow redirect if present (for git worktrees)
	return followRedirect(beadsDir)
}

// followRedirect checks for a redirect file and follows it if present.
// Redirect files are used by git worktrees to point to the main worktree's .beads.
func followRedirect(beadsDir string) string {
	redirectPath := filepath.Join(beadsDir, "redirect")

	content, err := os.ReadFile(redirectPath) //nolint:gosec // redirect path is within .beads dir
	if err != nil {
		return beadsDir
	}

	redirectTarget := strings.TrimSpace(string(content))
	if redirectTarget == "" {
		return beadsDir
	}

	resolvedPath := filepath.Join(beadsDir, redirectTarget)
	return filepath.Clean(resolvedPath)
}
