package codex

import (
	"context"
	"fmt"

	"github.com/zjrosen/perles/internal/orchestration/client"
)

// Process represents a headless Codex CLI process.
// Process implements client.HeadlessProcess by embedding BaseProcess.
type Process struct {
	*client.BaseProcess
}

// ErrTimeout is returned when a Codex process exceeds its configured timeout.
var ErrTimeout = fmt.Errorf("codex process timed out")

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
// Uses SpawnBuilder for clean process lifecycle management.
func spawnProcess(ctx context.Context, cfg Config, isResume bool) (*Process, error) {
	args := buildArgs(cfg, isResume)

	base, err := client.NewSpawnBuilder(ctx).
		WithExecutable("codex", args).
		WithWorkDir(cfg.WorkDir).
		WithSessionRef(cfg.SessionID).
		WithTimeout(cfg.Timeout).
		WithParser(NewParser()).
		WithProviderName("codex").
		Build()
	if err != nil {
		return nil, fmt.Errorf("codex: %w", err)
	}

	return &Process{BaseProcess: base}, nil
}

// Ensure Process implements client.HeadlessProcess at compile time.
var _ client.HeadlessProcess = (*Process)(nil)
