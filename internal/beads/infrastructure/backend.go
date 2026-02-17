package infrastructure

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	appbeads "github.com/zjrosen/perles/internal/beads/application"
	"github.com/zjrosen/perles/internal/log"
)

const (
	defaultDoltServerHost = "127.0.0.1"
	defaultDoltServerPort = 3307
	defaultDoltServerUser = "root"
)

// BeadsMetadata represents the .beads/metadata.json structure.
type BeadsMetadata struct {
	Backend      string `json:"backend"`       // "sqlite" or "dolt"
	DoltDatabase string `json:"dolt_database"` // e.g., "beads"
	DoltMode     string `json:"dolt_mode"`     // "server" — set by bd in metadata.json

	// Dolt server connection fields.
	// Perles always uses server mode for Dolt (embedded takes an exclusive lock).
	DoltServerHost string `json:"dolt_server_host,omitempty"` // default: 127.0.0.1
	DoltServerPort int    `json:"dolt_server_port,omitempty"` // default: 3307
	DoltServerUser string `json:"dolt_server_user,omitempty"` // default: root
}

// IsDoltServer returns true if the backend is Dolt with server mode configured.
func (m *BeadsMetadata) IsDoltServer() bool {
	return m.Backend == "dolt" && m.DoltMode == "server"
}

// GetDoltServerHost returns the server host, with env var override.
func (m *BeadsMetadata) GetDoltServerHost() string {
	if h := os.Getenv("BEADS_DOLT_SERVER_HOST"); h != "" {
		return h
	}
	if m.DoltServerHost != "" {
		return m.DoltServerHost
	}
	return defaultDoltServerHost
}

// GetDoltServerPort returns the server port, with env var override.
func (m *BeadsMetadata) GetDoltServerPort() int {
	if p := os.Getenv("BEADS_DOLT_SERVER_PORT"); p != "" {
		if port, err := strconv.Atoi(p); err == nil && port > 0 {
			return port
		}
	}
	if m.DoltServerPort > 0 {
		return m.DoltServerPort
	}
	return defaultDoltServerPort
}

// GetDoltServerUser returns the server user, with env var override.
func (m *BeadsMetadata) GetDoltServerUser() string {
	if u := os.Getenv("BEADS_DOLT_SERVER_USER"); u != "" {
		return u
	}
	if m.DoltServerUser != "" {
		return m.DoltServerUser
	}
	return defaultDoltServerUser
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
		// Dolt always uses server mode (MySQL protocol) in Perles.
		// The embedded Dolt driver takes an exclusive lock that blocks all
		// other processes (including bd) for the entire Perles session.
		log.Info(log.CatDB, "Connecting to Dolt server",
			"host", meta.GetDoltServerHost(),
			"port", meta.GetDoltServerPort(),
			"user", meta.GetDoltServerUser(),
			"database", meta.DoltDatabase)
		return NewDoltServerClient(
			beadsDir,
			meta.DoltDatabase,
			meta.GetDoltServerHost(),
			meta.GetDoltServerPort(),
			meta.GetDoltServerUser(),
		)
	default:
		return NewSQLiteClient(beadsDir)
	}
}
