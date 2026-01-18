package opencode

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/client"
)

// Process represents a headless OpenCode CLI process.
// Process implements client.HeadlessProcess by embedding BaseProcess.
type Process struct {
	*client.BaseProcess
}

// ErrTimeout is returned when an OpenCode process exceeds its configured timeout.
var ErrTimeout = fmt.Errorf("opencode process timed out")

// ErrNotFound is returned when the opencode executable cannot be found.
var ErrNotFound = fmt.Errorf("opencode: executable not found - ensure 'opencode' is in PATH")

// findExecutable locates the opencode executable.
func findExecutable() (string, error) {
	// On Windows, executables need .exe extension
	execName := "opencode"
	if os.PathSeparator == '\\' {
		execName = "opencode.exe"
	}

	// Check ~/.local/bin/opencode first (common Go binary location)
	homeDir, err := os.UserHomeDir()
	if err == nil {
		localPath := filepath.Join(homeDir, ".local", "bin", execName)
		if _, err := os.Stat(localPath); err == nil {
			log.Debug(log.CatOrch, "Found opencode at local bin", "subsystem", "opencode", "path", localPath)
			return localPath, nil
		}
	}

	// Check /usr/local/bin/opencode
	localPath := "/usr/local/bin/opencode"
	if _, err := os.Stat(localPath); err == nil {
		log.Debug(log.CatOrch, "Found opencode at system bin", "subsystem", "opencode", "path", localPath)
		return localPath, nil
	}

	// Fall back to exec.LookPath
	path, err := exec.LookPath("opencode")
	if err == nil {
		log.Debug(log.CatOrch, "Found opencode via PATH", "subsystem", "opencode", "path", path)
		return path, nil
	}

	return "", ErrNotFound
}

// Spawn creates and starts a new headless OpenCode process.
// Context is used for cancellation and timeout control.
func Spawn(ctx context.Context, cfg Config) (*Process, error) {
	return spawnProcess(ctx, cfg, false)
}

// Resume continues an existing OpenCode session using --session flag.
func Resume(ctx context.Context, sessionID string, cfg Config) (*Process, error) {
	cfg.SessionID = sessionID
	return spawnProcess(ctx, cfg, true)
}

// spawnProcess is the internal implementation for both Spawn and Resume.
// Uses SpawnBuilder for clean process lifecycle management.
func spawnProcess(ctx context.Context, cfg Config, isResume bool) (*Process, error) {
	// Find the opencode executable (provider-specific)
	execPath, err := findExecutable()
	if err != nil {
		return nil, err
	}

	args := buildArgs(cfg, isResume)

	// Build environment variables for MCP config
	// OPENCODE_CONFIG_CONTENT allows per-process MCP config without file conflicts
	var env []string
	if cfg.MCPConfig != "" {
		env = []string{"OPENCODE_CONFIG_CONTENT=" + cfg.MCPConfig}
	}

	base, err := client.NewSpawnBuilder(ctx).
		WithExecutable(execPath, args).
		WithWorkDir(cfg.WorkDir).
		WithSessionRef(cfg.SessionID).
		WithTimeout(cfg.Timeout).
		WithParser(NewParser()).
		WithEnv(env).
		WithProviderName("opencode").
		WithStderrCapture(true).
		Build()
	if err != nil {
		return nil, fmt.Errorf("opencode: %w", err)
	}

	return &Process{BaseProcess: base}, nil
}

// SessionID returns the session ID (may be empty until first event with sessionID is received).
// This is a convenience method that wraps SessionRef for backwards compatibility.
func (p *Process) SessionID() string {
	return p.SessionRef()
}

// Ensure Process implements client.HeadlessProcess at compile time.
var _ client.HeadlessProcess = (*Process)(nil)
