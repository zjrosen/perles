package opencode

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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

// extractSession extracts the session ID from any OpenCode event.
// CRITICAL: OpenCode doesn't emit a system/init event, so we capture from ANY event with sessionID.
// This is called for EVERY event by BaseProcess.
func extractSession(event client.OutputEvent, _ []byte) string {
	// OpenCode includes sessionID in most events
	if event.SessionID != "" {
		return event.SessionID
	}
	return ""
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
func spawnProcess(ctx context.Context, cfg Config, isResume bool) (*Process, error) {
	// Find the opencode executable
	execPath, err := findExecutable()
	if err != nil {
		return nil, err
	}

	var procCtx context.Context
	var cancel context.CancelFunc
	if cfg.Timeout > 0 {
		procCtx, cancel = context.WithTimeout(ctx, cfg.Timeout)
	} else {
		procCtx, cancel = context.WithCancel(ctx)
	}

	args := buildArgs(cfg, isResume)
	log.Debug(log.CatOrch, "Spawning opencode process", "subsystem", "opencode", "args", strings.Join(args, " "), "workDir", cfg.WorkDir)

	// #nosec G204 -- args are built from Config struct, not user input
	cmd := exec.CommandContext(procCtx, execPath, args...)
	cmd.Dir = cfg.WorkDir

	// Pass MCP config via OPENCODE_CONFIG_CONTENT env var for process isolation.
	// Each process gets its own MCP config without file conflicts.
	if cfg.MCPConfig != "" {
		cmd.Env = append(os.Environ(), "OPENCODE_CONFIG_CONTENT="+cfg.MCPConfig)
		log.Debug(log.CatOrch, "Setting OPENCODE_CONFIG_CONTENT", "subsystem", "opencode", "config", cfg.MCPConfig)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Create BaseProcess with OpenCode-specific hooks
	bp := client.NewBaseProcess(
		procCtx,
		cancel,
		cmd,
		stdout,
		stderr,
		cfg.WorkDir,
		client.WithParseEventFunc(parseEvent),
		client.WithSessionExtractor(extractSession),
		client.WithStderrCapture(true),
		client.WithProviderName("opencode"),
	)

	p := &Process{BaseProcess: bp}

	// Set initial session ID if provided (for --resume)
	if cfg.SessionID != "" {
		bp.SetSessionRef(cfg.SessionID)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		log.Debug(log.CatOrch, "Failed to start opencode process", "subsystem", "opencode", "error", err)
		return nil, fmt.Errorf("failed to start opencode process: %w", err)
	}

	log.Debug(log.CatOrch, "OpenCode process started", "subsystem", "opencode", "pid", cmd.Process.Pid)
	bp.SetStatus(client.StatusRunning)

	// Start output parser goroutines
	bp.StartGoroutines()

	return p, nil
}

// SessionID returns the session ID (may be empty until first event with sessionID is received).
// This is a convenience method that wraps SessionRef for backwards compatibility.
func (p *Process) SessionID() string {
	return p.SessionRef()
}

// Ensure Process implements client.HeadlessProcess at compile time.
var _ client.HeadlessProcess = (*Process)(nil)
