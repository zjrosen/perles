package infrastructure

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zjrosen/perles/internal/log"
)

// BeadsMetadata is the part of .beads/metadata.json that perles reads to find
// and classify a workspace. Field names and meanings follow bd's
// internal/configfile.Config.
type BeadsMetadata struct {
	// Database is the legacy database name or path ("beads.db" in SQLite-era
	// metadata, "dolt" in current metadata).
	Database string `json:"database,omitempty"`
	// Backend is the storage backend: "" or "dolt" for Dolt, "http" for a
	// workspace attached with `bd connect`, or a removed backend such as
	// "sqlite".
	Backend string `json:"backend,omitempty"`
	// GraphMode is the workspace data model; only "" and "dependency" are
	// issue databases.
	GraphMode string `json:"graph_mode,omitempty"`

	// DoltMode is "embedded", "server" or "proxied-server" ("" when unset).
	DoltMode         string `json:"dolt_mode,omitempty"`
	DoltDatabase     string `json:"dolt_database,omitempty"`
	DoltServerHost   string `json:"dolt_server_host,omitempty"`
	DoltServerPort   int    `json:"dolt_server_port,omitempty"` // deprecated in bd
	DoltServerSocket string `json:"dolt_server_socket,omitempty"`
	DoltServerUser   string `json:"dolt_server_user,omitempty"`
	DoltServerTLS    bool   `json:"dolt_server_tls,omitempty"`
	DoltDataDir      string `json:"dolt_data_dir,omitempty"`

	// SQLitePath is the database file of a removed SQLite backend.
	SQLitePath string `json:"sqlite_path,omitempty"`
}

const (
	metadataFileName       = "metadata.json"
	legacyMetadataFileName = "config.json"
)

// LoadMetadata reads beadsDir's metadata.json, or the legacy config.json when
// metadata.json is absent, without migrating or rewriting anything. It
// returns (nil, nil) when the workspace has neither file.
func LoadMetadata(beadsDir string) (*BeadsMetadata, error) {
	path := filepath.Join(beadsDir, metadataFileName)
	data, err := os.ReadFile(path) //nolint:gosec // metadata.json is within the .beads dir
	if errors.Is(err, os.ErrNotExist) {
		path = filepath.Join(beadsDir, legacyMetadataFileName)
		data, err = os.ReadFile(path) //nolint:gosec // legacy config.json is within the .beads dir
		if errors.Is(err, os.ErrNotExist) {
			log.Debug(log.CatDB, "No metadata.json found", "beadsDir", beadsDir)
			return nil, nil
		}
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", filepath.Base(path), err)
	}

	var meta BeadsMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", filepath.Base(path), err)
	}
	return &meta, nil
}

// beadsYAML is bd's merged config.yaml view for one workspace: BD_* environment
// variables over the workspace's config.local.yaml and config.yaml over the
// user-level config files, matching bd's config.Initialize when BEADS_DIR names
// the workspace. Keys may be nested (dolt: {mode: server}) or flat
// (dolt.mode: server); both spellings resolve.
type beadsYAML struct {
	layers []map[string]any // highest priority first
}

// loadBeadsYAML loads the config.yaml layers bd reads for beadsDir. Files that
// are missing or do not parse are skipped.
func loadBeadsYAML(beadsDir string) beadsYAML {
	var paths []string
	workspaceConfig := filepath.Join(beadsDir, "config.yaml")
	if fileExists(workspaceConfig) {
		// bd merges config.local.yaml only when a config.yaml sits beside it.
		paths = append(paths, filepath.Join(beadsDir, "config.local.yaml"), workspaceConfig)
	}
	paths = append(paths, userConfigYAMLPaths()...)

	var y beadsYAML
	seen := make(map[string]bool, len(paths))
	for _, p := range paths {
		p = filepath.Clean(p)
		if seen[p] {
			continue
		}
		seen[p] = true
		if m := readYAMLFile(p); m != nil {
			y.layers = append(y.layers, m)
		}
	}
	return y
}

// userConfigYAMLPaths returns bd's user-level config.yaml files, highest
// priority first: ~/.config/bd, the native user config dir, then the legacy
// ~/.beads.
func userConfigYAMLPaths() []string {
	var paths []string
	home, homeErr := os.UserHomeDir()
	if homeErr == nil && filepath.IsAbs(home) {
		paths = append(paths, filepath.Join(home, ".config", "bd", "config.yaml"))
	}
	if native, err := os.UserConfigDir(); err == nil && filepath.IsAbs(native) {
		paths = append(paths, filepath.Join(native, "bd", "config.yaml"))
	}
	if homeErr == nil && filepath.IsAbs(home) {
		paths = append(paths, filepath.Join(home, ".beads", "config.yaml"))
	}
	return paths
}

func readYAMLFile(path string) map[string]any {
	data, err := os.ReadFile(path) //nolint:gosec // bd config files at fixed locations
	if err != nil {
		return nil
	}
	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		log.Debug(log.CatDB, "Skipping unparseable beads config.yaml", "path", path, "error", err)
		return nil
	}
	return lowerKeys(m)
}

// lowerKeys lower-cases map keys recursively; bd (viper) reads keys
// case-insensitively.
func lowerKeys(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if sub, ok := v.(map[string]any); ok {
			v = lowerKeys(sub)
		}
		out[strings.ToLower(k)] = v
	}
	return out
}

// get returns the value of key (such as "dolt.mode") from the highest-priority
// source that sets it: the BD_* environment variable (BD_DOLT_MODE), then each
// config file in turn. Empty values count as unset, as they do for viper.
func (y beadsYAML) get(key string) string {
	key = strings.ToLower(key)
	if v := os.Getenv(bdEnvKey(key)); v != "" {
		return v
	}
	for _, layer := range y.layers {
		if v, ok := lookupYAMLKey(layer, key); ok {
			if s := strings.TrimSpace(yamlScalarString(v)); s != "" {
				return s
			}
		}
	}
	return ""
}

// getBool parses key as a boolean (strconv.ParseBool), reporting false when it
// is unset or does not parse.
func (y beadsYAML) getBool(key string) bool {
	b, err := strconv.ParseBool(y.get(key))
	return err == nil && b
}

// bdEnvKey is viper's AutomaticEnv name for key under bd's "BD" prefix:
// dolt.shared-server becomes BD_DOLT_SHARED_SERVER.
func bdEnvKey(key string) string {
	return "BD_" + strings.ToUpper(strings.NewReplacer(".", "_", "-", "_").Replace(key))
}

// lookupYAMLKey finds a dotted key in a decoded YAML map, accepting nested
// maps, flat dotted keys, and mixtures of both.
func lookupYAMLKey(m map[string]any, key string) (any, bool) {
	if v, ok := m[key]; ok {
		return v, true
	}
	for i := range len(key) {
		if key[i] != '.' {
			continue
		}
		sub, ok := m[key[:i]].(map[string]any)
		if !ok {
			continue
		}
		if v, ok := lookupYAMLKey(sub, key[i+1:]); ok {
			return v, true
		}
	}
	return nil, false
}

func yamlScalarString(v any) string {
	switch val := v.(type) {
	case nil:
		return ""
	case string:
		return val
	case map[string]any, []any:
		return ""
	default:
		return fmt.Sprint(val)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
