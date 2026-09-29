package registry

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/registry/domain"
)

// UserRegistryDir returns the path to user YAML workflow registries.
// Returns ~/.perles/workflows (for backwards compatibility with markdown workflows).
// Returns empty string if home directory cannot be determined.
func UserRegistryDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".perles", "workflows")
}

// UserRegistryBaseDir returns the base directory for user registrations.
// Returns ~/.perles (root for os.DirFS).
// Returns empty string if home directory cannot be determined.
func UserRegistryBaseDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".perles")
}

// LoadUserRegistryFromDir loads YAML registrations from a user directory.
// baseDir should be the root directory (e.g., ~/.perles/) that contains a "workflows" subdirectory.
// Returns nil, nil, nil if the directory doesn't exist (graceful fallback).
// Invalid workflows are logged (with their template.yaml path and error) and skipped;
// the remaining workflows still load.
//
// Templates referenced by user workflows are resolved in the user directory first. When
// builtinFS is non-nil, templates not found there fall back to builtinFS (the embedded
// built-in templates), so shared templates such as v1-epic-instructions.md and
// v1-human-review.md don't need to be copied. The returned FS reads templates with the
// same precedence and should be used to render the returned registrations.
func LoadUserRegistryFromDir(baseDir string, builtinFS fs.FS) ([]*registry.Registration, fs.FS, error) {
	if baseDir == "" {
		return nil, nil, nil
	}

	// Check if base directory exists
	info, err := os.Stat(baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			// Directory doesn't exist - not an error, just no user workflows
			return nil, nil, nil
		}
		return nil, nil, nil // Other stat errors are treated as "no user workflows"
	}
	if !info.IsDir() {
		return nil, nil, nil
	}

	// Check if workflows subdirectory exists
	workflowsDir := filepath.Join(baseDir, "workflows")
	info, err = os.Stat(workflowsDir)
	if err != nil {
		if os.IsNotExist(err) {
			// Workflows directory doesn't exist - not an error
			return nil, nil, nil
		}
		return nil, nil, nil
	}
	if !info.IsDir() {
		return nil, nil, nil
	}

	// Create os.DirFS rooted at base directory
	// This allows the YAML loader to walk the "workflows" subdirectory
	userFS := os.DirFS(baseDir)
	templateFS := newLayeredFS(userFS, builtinFS)

	// Load registrations using the existing YAML loader with SourceUser,
	// skipping invalid workflows instead of dropping all of them
	regs, err := loadRegistryFromYAML(userFS, registry.SourceUser, loadOptions{
		templateFS:  templateFS,
		skipInvalid: true,
	})
	if err != nil {
		// Log warning but don't fail - e.g. no valid user workflows
		log.Warn(log.CatConfig, "loading user registrations", "error", err.Error(), "dir", baseDir)
		// Return the FS even if loading failed - allows caller to handle partial results
		return nil, templateFS, nil
	}

	return regs, templateFS, nil
}
