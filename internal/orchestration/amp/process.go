package amp

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/client"
)

// Process represents a headless Amp process.
// Process implements client.HeadlessProcess by embedding BaseProcess.
type Process struct {
	*client.BaseProcess
}

// ErrTimeout is returned when an Amp process exceeds its configured timeout.
var ErrTimeout = fmt.Errorf("amp process timed out")

// parser is the shared Amp event parser instance.
var parser = NewParser()

// Spawn creates and starts a new headless Amp process.
// Context is used for cancellation and timeout control.
func Spawn(ctx context.Context, cfg Config) (*Process, error) {
	return spawnProcess(ctx, cfg, false)
}

// Resume continues an existing Amp thread.
func Resume(ctx context.Context, threadID string, cfg Config) (*Process, error) {
	cfg.ThreadID = threadID
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
	log.Debug(log.CatOrch, "Spawning amp process", "subsystem", "amp", "args", strings.Join(args, " "), "workDir", cfg.WorkDir)

	// #nosec G204 -- args are built from Config struct, not user input
	cmd := exec.CommandContext(procCtx, "amp", args...)
	cmd.Dir = cfg.WorkDir

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
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

	// Create BaseProcess with Amp-specific hooks
	bp := client.NewBaseProcess(
		procCtx,
		cancel,
		cmd,
		stdout,
		stderr,
		cfg.WorkDir,
		client.WithEventParser(parser),
		client.WithStderrCapture(false), // Amp logs but doesn't capture stderr
		client.WithProviderName("amp"),
	)

	// Store stdin for later use
	bp.SetStdin(stdin)

	// Set initial thread ID if resuming
	if cfg.ThreadID != "" {
		bp.SetSessionRef(cfg.ThreadID)
	}

	p := &Process{BaseProcess: bp}

	if err := cmd.Start(); err != nil {
		cancel()
		log.Debug(log.CatOrch, "Failed to start amp process", "subsystem", "amp", "error", err)
		return nil, fmt.Errorf("failed to start amp process: %w", err)
	}

	log.Debug(log.CatOrch, "Amp process started", "subsystem", "amp", "pid", cmd.Process.Pid)
	bp.SetStatus(client.StatusRunning)

	// Write prompt to stdin if provided (Amp reads prompt from stdin in execute mode)
	// Keep stdin handling in provider due to complex goroutine timing requirements
	if cfg.Prompt != "" {
		go func() {
			defer func() {
				if closeErr := stdin.Close(); closeErr != nil {
					log.Debug(log.CatOrch, "Failed to close stdin", "subsystem", "amp", "error", closeErr)
				}
			}()
			_, writeErr := io.WriteString(stdin, cfg.Prompt)
			if writeErr != nil {
				log.Debug(log.CatOrch, "Failed to write prompt to stdin", "subsystem", "amp", "error", writeErr)
			}
		}()
	} else {
		if closeErr := stdin.Close(); closeErr != nil {
			log.Debug(log.CatOrch, "Failed to close stdin", "subsystem", "amp", "error", closeErr)
		}
	}

	// Start output parser goroutines
	bp.StartGoroutines()

	return p, nil
}

// ThreadID returns the thread ID (Amp's equivalent of session ID).
func (p *Process) ThreadID() string {
	return p.SessionRef()
}

// Ensure Process implements client.HeadlessProcess at compile time.
var _ client.HeadlessProcess = (*Process)(nil)
