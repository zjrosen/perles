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
	Backend  string `json:"backend"`             // "sqlite" or "dolt"
	DoltMode string `json:"dolt_mode,omitempty"` // "embedded" (default) or "server"
	// Dolt database name (default: "beads").
	DoltDatabase string `json:"dolt_database,omitempty"`
	// Dolt server connection fields (used in server mode only).
	DoltServerHost string `json:"dolt_server_host,omitempty"` // default: 127.0.0.1
	DoltServerPort int    `json:"dolt_server_port,omitempty"` // default: 3307
	DoltServerUser string `json:"dolt_server_user,omitempty"` // default: root
}

// IsDoltServer returns true if the backend is Dolt in server mode.
// Server mode is required for concurrent access by perles.
func (m *BeadsMetadata) IsDoltServer() bool {
	return m.Backend == "dolt" && m.isDoltServerMode()
}

// IsDoltEmbedded returns true if the backend is Dolt in embedded mode.
// Embedded mode uses an exclusive file lock, preventing concurrent access.
func (m *BeadsMetadata) IsDoltEmbedded() bool {
	return m.Backend == "dolt" && !m.isDoltServerMode()
}

// isDoltServerMode checks whether dolt is configured for server mode.
// Checks (in priority order):
//  1. BEADS_DOLT_SERVER_MODE=1 env var
//  2. BEADS_DOLT_SHARED_SERVER env var
//  3. dolt_mode field in metadata.json
//
// Default is embedded (matching beads v0.63+ behavior).
func (m *BeadsMetadata) isDoltServerMode() bool {
	if os.Getenv("BEADS_DOLT_SERVER_MODE") == "1" {
		return true
	}
	if v := os.Getenv("BEADS_DOLT_SHARED_SERVER"); v == "1" || strings.EqualFold(v, "true") {
		return true
	}
	return strings.ToLower(m.DoltMode) == "server"
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
		// dolt_mode defaults to "embedded" when absent (beads v0.63+ behavior)
		if meta.DoltMode == "" {
			meta.DoltMode = "embedded"
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
//
// Returns an EmbeddedModeUnsupportedError error when dolt is configured in embedded mode,
// which uses an exclusive file lock incompatible with concurrent access from perles.
func NewClient(beadsDir string) (appbeads.DBClient, error) {
	meta, err := LoadMetadata(beadsDir)
	if err != nil {
		return nil, fmt.Errorf("detecting backend: %w", err)
	}

	log.Info(log.CatDB, "Backend detected", "backend", meta.Backend, "doltMode", meta.DoltMode, "beadsDir", beadsDir)

	switch {
	case meta.IsDoltServer():
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
	case meta.IsDoltEmbedded():
		return nil, &EmbeddedModeUnsupportedError{BeadsDir: beadsDir}
	default:
		return NewSQLiteClient(beadsDir)
	}
}

// EmbeddedModeUnsupportedError is returned when beads is using dolt embedded mode.
// Embedded dolt takes an exclusive file lock, so perles cannot access it concurrently.
type EmbeddedModeUnsupportedError struct {
	BeadsDir string
}

func (e *EmbeddedModeUnsupportedError) Error() string {
	return fmt.Sprintf("beads is using embedded dolt mode in %s (requires server mode for concurrent access)", e.BeadsDir)
}
