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
// Invalid template.yaml files are logged and skipped.
func LoadUserRegistryFromDir(baseDir string) ([]*registry.Registration, fs.FS, error) {
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
	// This allows LoadRegistryFromYAML to walk "workflows" subdirectory
	userFS := os.DirFS(baseDir)

	// Load registrations using the existing YAML loader with SourceUser
	regs, err := LoadRegistryFromYAMLWithSource(userFS, registry.SourceUser)
	if err != nil {
		// Log warning but don't fail - user may have partial/invalid workflows
		log.Warn(log.CatConfig, "loading user registrations", "error", err.Error(), "dir", baseDir)
		// Return the FS even if loading failed - allows caller to handle partial results
		return nil, userFS, nil
	}

	return regs, userFS, nil
}
