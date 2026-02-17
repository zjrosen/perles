package infrastructure

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	appbeads "github.com/zjrosen/perles/internal/beads/application"
	"github.com/zjrosen/perles/internal/log"
)

// BeadsMetadata represents the .beads/metadata.json structure.
type BeadsMetadata struct {
	Backend      string `json:"backend"`       // "sqlite" or "dolt"
	DoltDatabase string `json:"dolt_database"` // e.g., "beads"
}

// DetectBackend reads .beads/metadata.json and returns the backend type.
// Returns "sqlite" as default if the file doesn't exist (backward compatibility).
func DetectBackend(beadsDir string) (string, error) {
	meta, err := LoadMetadata(beadsDir)
	if err != nil {
		return "", err
	}
	return meta.Backend, nil
}

// LoadMetadata parses the .beads/metadata.json file.
// Returns default SQLite metadata when the file doesn't exist.
func LoadMetadata(beadsDir string) (*BeadsMetadata, error) {
	metadataPath := filepath.Join(beadsDir, "metadata.json")

	data, err := os.ReadFile(metadataPath) //nolint:gosec // metadata.json is within .beads dir
	if err != nil {
		if os.IsNotExist(err) {
			log.Debug(log.CatDB, "No metadata.json found, defaulting to sqlite", "path", metadataPath)
			return &BeadsMetadata{Backend: "sqlite"}, nil
		}
		return nil, fmt.Errorf("reading metadata.json: %w", err)
	}

	var meta BeadsMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parsing metadata.json: %w", err)
	}

	// Normalize and validate
	switch meta.Backend {
	case "dolt":
		if meta.DoltDatabase == "" {
			meta.DoltDatabase = "beads"
		}
	case "sqlite", "":
		meta.Backend = "sqlite"
	default:
		return nil, fmt.Errorf("unsupported backend type: %s", meta.Backend)
	}

	return &meta, nil
}

// NewClient creates the appropriate database client based on backend detection.
// beadsDir should be the resolved .beads directory path.
func NewClient(beadsDir string) (appbeads.DBClient, error) {
	meta, err := LoadMetadata(beadsDir)
	if err != nil {
		return nil, fmt.Errorf("detecting backend: %w", err)
	}

	log.Info(log.CatDB, "Backend detected", "backend", meta.Backend, "beadsDir", beadsDir)

	switch meta.Backend {
	case "dolt":
		return NewDoltClient(beadsDir, meta.DoltDatabase)
	default:
		return NewSQLiteClient(beadsDir)
	}
}
