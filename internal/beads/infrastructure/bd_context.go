package infrastructure

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zjrosen/perles/internal/beads/domain"
)

// BDContext is the effective workspace identity `bd context --json` reports
// (bd's cmd/bd ContextInfo). bd resolves it from config files without opening
// the database, applying its own precedence for mode, host, port and database.
type BDContext struct {
	BeadsDir   string `json:"beads_dir"`
	Backend    string `json:"backend"`
	DoltMode   string `json:"dolt_mode"`
	ServerHost string `json:"server_host,omitempty"`
	ServerPort int    `json:"server_port,omitempty"`
	ProxiedDir string `json:"proxied_dir,omitempty"`
	Database   string `json:"database"`
	DataDir    string `json:"data_dir,omitempty"`
	BdVersion  string `json:"bd_version"`
}

// minBDContextVersion is the oldest bd whose `bd context` reports the port the
// store dials. Earlier releases (bd context exists since v0.60.0) reported the
// metadata/default port and ignored .beads/dolt-server.port.
const minBDContextVersion = "1.0.0"

// bdContextTimeout bounds how long perles waits for `bd context`.
const bdContextTimeout = 5 * time.Second

// BDContextRunner runs `bd context --json` for beadsDir (from workDir when it
// is non-empty) and returns its stdout. Tests inject one so they never run bd.
type BDContextRunner func(ctx context.Context, beadsDir, workDir string) ([]byte, error)

// errBDContextDisabled is returned by the default runner inside test
// binaries, so a test that does not inject a runner never runs the real bd.
var errBDContextDisabled = errors.New("bd context is not run in tests")

// defaultBDContextRunner runs the bd on PATH.
func defaultBDContextRunner(ctx context.Context, beadsDir, workDir string) ([]byte, error) {
	if testing.Testing() {
		return nil, errBDContextDisabled
	}
	return runBDContextCommand(ctx, "bd", beadsDir, workDir)
}

// runBDContextCommand runs `<bin> context --json` with BEADS_DIR set to the
// absolute beadsDir and BD_JSON_ENVELOPE removed from the environment.
func runBDContextCommand(ctx context.Context, bin, beadsDir, workDir string) ([]byte, error) {
	absBeadsDir, err := filepath.Abs(beadsDir)
	if err != nil {
		return nil, fmt.Errorf("resolving beads dir: %w", err)
	}

	cmd := exec.CommandContext(ctx, bin, "context", "--json") //nolint:gosec // fixed arguments
	if workDir != "" {
		cmd.Dir = workDir
	}
	cmd.Env = bdContextEnv(os.Environ(), absBeadsDir)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return stdout.Bytes(), fmt.Errorf("bd context: %w: %s", err, msg)
		}
		return stdout.Bytes(), fmt.Errorf("bd context: %w", err)
	}
	return stdout.Bytes(), nil
}

// bdContextEnv returns env with BEADS_DIR replaced by beadsDir and
// BD_JSON_ENVELOPE removed.
func bdContextEnv(env []string, beadsDir string) []string {
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if name == "BEADS_DIR" || name == "BD_JSON_ENVELOPE" {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "BEADS_DIR="+beadsDir)
}

// bdContextTooOldError rejects a bd context answer whose bd_version is empty
// or older than minBDContextVersion. Version is the raw bd_version.
type bdContextTooOldError struct {
	Version string
}

func (e *bdContextTooOldError) Error() string {
	if e.Version == "" {
		return fmt.Sprintf("bd context reported no bd_version (need bd %s or later)", minBDContextVersion)
	}
	return fmt.Sprintf("bd %s is older than %s", e.Version, minBDContextVersion)
}

// queryBDContext runs runner and parses its output. It returns an error when
// bd fails (the runner's error, unchanged), reports an error, or is older than
// minBDContextVersion (a *bdContextTooOldError carrying the raw version);
// callers then resolve the connection from config files themselves.
func queryBDContext(runner BDContextRunner, beadsDir, workDir string) (*BDContext, error) {
	ctx, cancel := context.WithTimeout(context.Background(), bdContextTimeout)
	defer cancel()

	out, err := runner(ctx, beadsDir, workDir)
	if err != nil {
		return nil, err
	}
	bdCtx, err := parseBDContext(out)
	if err != nil {
		return nil, err
	}
	if v := bdCtx.BdVersion; v == "" || domain.CompareVersions(baseVersion(v), minBDContextVersion) < 0 {
		return nil, &bdContextTooOldError{Version: v}
	}
	return bdCtx, nil
}

// parseBDContext parses `bd context --json` output: a bare object (with bd's
// schema_version key), or an envelope {"schema_version": N, "data": {...}}.
// An {"error": "..."} object is returned as an error.
func parseBDContext(out []byte) (*BDContext, error) {
	out = bytes.TrimSpace(out)
	if len(out) == 0 {
		return nil, errors.New("bd context printed nothing")
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parsing bd context output: %w", err)
	}
	if data := bytes.TrimSpace(raw["data"]); len(data) > 0 && data[0] == '{' {
		out, raw = data, nil
		if err := json.Unmarshal(out, &raw); err != nil {
			return nil, fmt.Errorf("parsing bd context envelope: %w", err)
		}
	}
	if msg, ok := raw["error"]; ok {
		var s string
		if json.Unmarshal(msg, &s) != nil {
			s = string(msg)
		}
		return nil, fmt.Errorf("bd context: %s", s)
	}

	var bdCtx BDContext
	if err := json.Unmarshal(out, &bdCtx); err != nil {
		return nil, fmt.Errorf("parsing bd context output: %w", err)
	}
	return &bdCtx, nil
}

// baseVersion strips a pre-release, build or trailing annotation from a
// version string ("1.2.0-rc.1" and "1.3.2 (abc)" become "1.2.0" and "1.3.2").
func baseVersion(v string) string {
	v = strings.TrimSpace(v)
	if i := strings.IndexAny(v, "-+ "); i >= 0 {
		v = v[:i]
	}
	return v
}
