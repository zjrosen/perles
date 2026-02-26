package infrastructure

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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
	// Dolt server connection fields.
	// Perles always uses server mode for Dolt (embedded takes an exclusive lock).
	DoltServerHost string `json:"dolt_server_host,omitempty"` // default: 127.0.0.1
	DoltServerPort int    `json:"dolt_server_port,omitempty"` // default: 3307
	DoltServerUser string `json:"dolt_server_user,omitempty"` // default: root
}

// IsDoltServer returns true if the backend is Dolt.
// Server mode is always used for Dolt (embedded takes an exclusive lock).
func (m *BeadsMetadata) IsDoltServer() bool {
	return m.Backend == "dolt"
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

// GetDoltServerPortWithDir returns the server port, discovering it from the
// dolt-server.port file that bd writes when starting the Dolt SQL server.
// Resolution order: env var → metadata.json → .beads/dolt-server.port → default.
func (m *BeadsMetadata) GetDoltServerPortWithDir(beadsDir string) int {
	if p := os.Getenv("BEADS_DOLT_SERVER_PORT"); p != "" {
		if port, err := strconv.Atoi(p); err == nil && port > 0 {
			return port
		}
	}
	if m.DoltServerPort > 0 {
		return m.DoltServerPort
	}
	// bd writes the actual server port to .beads/dolt-server.port at startup.
	// This is the most reliable source when metadata.json doesn't specify a port.
	portFile := filepath.Join(beadsDir, "dolt-server.port")
	if data, err := os.ReadFile(portFile); err == nil { //nolint:gosec // within .beads dir
		if port, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && port > 0 {
			log.Debug(log.CatDB, "Discovered Dolt server port from dolt-server.port", "port", port)
			return port
		}
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
		port := meta.GetDoltServerPortWithDir(beadsDir)
		log.Info(log.CatDB, "Connecting to Dolt server",
			"host", meta.GetDoltServerHost(),
			"port", port,
			"user", meta.GetDoltServerUser(),
			"database", meta.DoltDatabase)
		return NewDoltServerClient(
			beadsDir,
			meta.DoltDatabase,
			meta.GetDoltServerHost(),
			port,
			meta.GetDoltServerUser(),
		)
	default:
		return NewSQLiteClient(beadsDir)
	}
}
