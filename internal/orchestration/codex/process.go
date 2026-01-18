package codex

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/client"
)

// Process represents a headless Codex CLI process.
// Process implements client.HeadlessProcess by embedding BaseProcess.
type Process struct {
	*client.BaseProcess
}

// ErrTimeout is returned when a Codex process exceeds its configured timeout.
var ErrTimeout = fmt.Errorf("codex process timed out")

// extractSession extracts the session ID (thread_id) from an init event.
func extractSession(event client.OutputEvent, rawLine []byte) string {
	if event.Type == client.EventSystem && event.SubType == "init" && event.SessionID != "" {
		return event.SessionID
	}
	return ""
}

// Spawn creates and starts a new headless Codex process.
// Context is used for cancellation and timeout control.
func Spawn(ctx context.Context, cfg Config) (*Process, error) {
	return spawnProcess(ctx, cfg, false)
}

// Resume continues an existing Codex session.
func Resume(ctx context.Context, sessionID string, cfg Config) (*Process, error) {
	cfg.SessionID = sessionID
	return spawnProcess(ctx, cfg, true)
}

// spawnProcess is the internal implementation for both Spawn and Resume.
func spawnProcess(ctx context.Context, cfg Config, isResume bool) (*Process, error) {
	var procCtx context.Context
	var cancel context.CancelFunc
	if cfg.Timeout > 0 {
		procCtx, cancel = context.WithTimeout(ctx, cfg.Timeout)
	} else {
		procCtx, cancel = context.WithCancel(ctx)
	}

	args := buildArgs(cfg, isResume)
	log.Debug(log.CatOrch, "Spawning codex process", "subsystem", "codex", "args", strings.Join(args, " "), "workDir", cfg.WorkDir)

	// #nosec G204 -- args are built from Config struct, not user input
	cmd := exec.CommandContext(procCtx, "codex", args...)
	// Set working directory via cmd.Dir as belt-and-suspenders with -C flag
	cmd.Dir = cfg.WorkDir

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

	// Create BaseProcess with Codex-specific hooks
	bp := client.NewBaseProcess(
		procCtx,
		cancel,
		cmd,
		stdout,
		stderr,
		cfg.WorkDir,
		client.WithParseEventFunc(ParseEvent),
		client.WithSessionExtractor(extractSession),
		client.WithStderrCapture(false), // Codex logs only, doesn't capture
		client.WithProviderName("codex"),
	)

	// Set initial session ID if resuming
	if cfg.SessionID != "" {
		bp.SetSessionRef(cfg.SessionID)
	}

	p := &Process{BaseProcess: bp}

	if err := cmd.Start(); err != nil {
		cancel()
		log.Debug(log.CatOrch, "Failed to start codex process", "subsystem", "codex", "error", err)
		return nil, fmt.Errorf("failed to start codex process: %w", err)
	}

	log.Debug(log.CatOrch, "Codex process started", "subsystem", "codex", "pid", cmd.Process.Pid)
	bp.SetStatus(client.StatusRunning)

	// Start output parser goroutines
	bp.StartGoroutines()

	return p, nil
}

// Ensure Process implements client.HeadlessProcess at compile time.
var _ client.HeadlessProcess = (*Process)(nil)
