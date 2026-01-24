// Package integration provides implementations that bridge v2 handlers
// to actual system components (worker sessions, BD CLI).
package integration

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/v2/handler"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
)

// Compile-time check that ProcessSessionDeliverer implements MessageDeliverer.
var _ handler.MessageDeliverer = (*ProcessSessionDeliverer)(nil)

// DefaultDeliveryTimeout is the default timeout for message delivery.
// This prevents blocking the FIFO processor on slow session resume operations.
const DefaultDeliveryTimeout = 3 * time.Second

// SessionProvider provides access to process session information and MCP config generation.
// This abstracts the session-related functionality for both coordinator and workers.
type SessionProvider interface {
	// GetProcessSessionID returns the session ID for a process (coordinator or worker).
	// Returns an error if the process doesn't exist or has no session.
	GetProcessSessionID(processID string) (string, error)

	// GenerateProcessMCPConfig generates the MCP config JSON for a process.
	// For coordinator, generates coordinator MCP config.
	// For workers, generates worker MCP config with their ID.
	GenerateProcessMCPConfig(processID string) (string, error)

	// GetWorkDir returns the working directory for processes.
	GetWorkDir() string
}

// ProcessResumer abstracts process resume functionality for message delivery.
type ProcessResumer interface {
	// ResumeProcess resumes a process (coordinator or worker) by providing a new AI process.
	ResumeProcess(processID string, proc client.HeadlessProcess) error
}

// ProcessSessionDeliverer implements the MessageDeliverer interface
// by resuming process sessions with the message content.
// Works for both coordinator and worker processes.
type ProcessSessionDeliverer struct {
	sessionProvider       SessionProvider
	coordinatorClient     client.HeadlessClient
	workerClient          client.HeadlessClient
	resumer               ProcessResumer
	timeout               time.Duration
	coordinatorExtensions map[string]any
	workerExtensions      map[string]any
	beadsDir              string
}

// ProcessSessionDelivererOption configures ProcessSessionDeliverer.
type ProcessSessionDelivererOption func(*ProcessSessionDeliverer)

// WithDeliveryTimeout sets the timeout for message delivery.
func WithDeliveryTimeout(timeout time.Duration) ProcessSessionDelivererOption {
	return func(d *ProcessSessionDeliverer) {
		d.timeout = timeout
	}
}

// WithBeadsDir sets the beads database directory for spawned processes.
func WithBeadsDir(beadsDir string) ProcessSessionDelivererOption {
	return func(d *ProcessSessionDeliverer) {
		d.beadsDir = beadsDir
	}
}

// NewProcessSessionDeliverer creates a new ProcessSessionDeliverer.
//
// Parameters:
//   - sessionProvider: provides session IDs and MCP config for processes
//   - coordinatorClient: HeadlessClient for spawning/resuming coordinator sessions
//   - workerClient: HeadlessClient for spawning/resuming worker sessions
//   - resumer: ProcessResumer for resuming processes (typically ProcessRegistry)
//   - coordinatorExtensions: provider-specific configuration for coordinator
//   - workerExtensions: provider-specific configuration for workers
//   - opts: optional configuration
func NewProcessSessionDeliverer(
	sessionProvider SessionProvider,
	coordinatorClient client.HeadlessClient,
	workerClient client.HeadlessClient,
	resumer ProcessResumer,
	coordinatorExtensions map[string]any,
	workerExtensions map[string]any,
	opts ...ProcessSessionDelivererOption,
) *ProcessSessionDeliverer {
	// Defensive shallow copies to prevent accidental mutation races
	coordExtCopy := make(map[string]any, len(coordinatorExtensions))
	maps.Copy(coordExtCopy, coordinatorExtensions)
	workerExtCopy := make(map[string]any, len(workerExtensions))
	maps.Copy(workerExtCopy, workerExtensions)

	d := &ProcessSessionDeliverer{
		sessionProvider:       sessionProvider,
		coordinatorClient:     coordinatorClient,
		workerClient:          workerClient,
		resumer:               resumer,
		timeout:               DefaultDeliveryTimeout,
		coordinatorExtensions: coordExtCopy,
		workerExtensions:      workerExtCopy,
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// Deliver sends a message to a process (coordinator or worker) by resuming their session.
// Implements the MessageDeliverer interface from handler/messaging.go.
//
// The delivery process:
// 1. Get the process's session ID
// 2. Generate MCP config for the process (coordinator or worker specific)
// 3. Spawn/resume the session with the message as prompt
// 4. Resume the process to handle events
//
// Returns an error if:
// - The process doesn't exist or has no session
// - The session spawn/resume fails
// - The process resume fails
// - The context is cancelled or times out
func (d *ProcessSessionDeliverer) Deliver(ctx context.Context, processID, content string) error {
	// Use a timeout for the spawn operation itself, but NOT for the process lifetime.
	// The claude process needs to live beyond this function call.
	// We use context.Background() for the actual spawn to avoid killing the process
	// when the parent context is cancelled.
	spawnDeadline := time.Now().Add(d.timeout)

	// 1. Get process's session ID
	sessionID, err := d.sessionProvider.GetProcessSessionID(processID)
	if err != nil {
		return fmt.Errorf("failed to get session for process %s: %w", processID, err)
	}
	if sessionID == "" {
		return fmt.Errorf("process %s has no session ID (may still be starting)", processID)
	}

	// Check if we've exceeded timeout after session lookup
	if time.Now().After(spawnDeadline) {
		return fmt.Errorf("timeout exceeded before spawn for process %s", processID)
	}

	// 2. Generate MCP config for the process (handles coordinator vs worker internally)
	mcpConfig, err := d.sessionProvider.GenerateProcessMCPConfig(processID)
	if err != nil {
		return fmt.Errorf("failed to generate MCP config for process %s: %w", processID, err)
	}

	// Check if we've exceeded timeout after config generation
	if time.Now().After(spawnDeadline) {
		return fmt.Errorf("timeout exceeded before spawn for process %s", processID)
	}

	// Check if parent context is cancelled
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// 3. Select client and extensions based on process role
	var aiClient client.HeadlessClient
	var extensions map[string]any

	if processID == repository.CoordinatorID {
		log.Error(log.CatOrch, "is coordinator process", "processId", processID, "isCoord", true)
		aiClient = d.coordinatorClient
		extensions = d.coordinatorExtensions
	} else {
		log.Error(log.CatOrch, "is coordinator process", "processId", processID, "isCoord", false)
		aiClient = d.workerClient
		extensions = d.workerExtensions
	}

	// 4. Spawn/resume the session with the message as prompt
	// IMPORTANT: Use context.Background() here because the claude process lifetime
	// is managed by the Process struct, not by this function's context.
	// If we used the parent context, the process would be killed when Deliver() returns.
	proc, err := aiClient.Spawn(context.Background(), client.Config{
		WorkDir:         d.sessionProvider.GetWorkDir(),
		BeadsDir:        d.beadsDir,
		SessionID:       sessionID,
		Prompt:          content,
		MCPConfig:       mcpConfig,
		SkipPermissions: true,
		DisallowedTools: []string{"AskUserQuestion"},
		Extensions:      extensions,
	})
	if err != nil {
		return fmt.Errorf("failed to resume session for process %s: %w", processID, err)
	}

	// 5. Resume the process to handle events
	if err := d.resumer.ResumeProcess(processID, proc); err != nil {
		// Try to cancel the process we spawned
		_ = proc.Cancel()
		return fmt.Errorf("failed to resume process %s: %w", processID, err)
	}

	return nil
}
