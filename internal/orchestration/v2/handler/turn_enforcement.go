// Package handler provides command handlers for the v2 orchestration architecture.
// This file contains turn completion enforcement for workers.
package handler

import (
	"fmt"
	"strings"
	"sync"

	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
)

// ===========================================================================
// Constants
// ===========================================================================

// RequiredTools defines which MCP tools satisfy turn completion for workers.
// A worker must call at least one of these tools to complete their turn without
// triggering an enforcement reminder.
var RequiredTools = []string{
	"post_message",
	"report_implementation_complete",
	"report_review_verdict",
	"signal_ready",
}

// maxEnforcementAttempts is the maximum number of enforcement reminders to send
// before allowing a turn to complete without compliance. This prevents infinite
// loops from persistent non-compliance.
const maxEnforcementAttempts = 2

// ===========================================================================
// TurnCompletionEnforcer Interface
// ===========================================================================

// TurnCompletionEnforcer provides turn completion tracking and enforcement.
// Implementations track MCP tool calls during worker turns and enforce
// compliance with turn completion requirements.
type TurnCompletionEnforcer interface {
	// RecordToolCall records that a worker called a specific tool.
	// Called from MCP tool handlers when a required tool is invoked.
	RecordToolCall(processID, toolName string)

	// ResetTurn clears tracking state for a new turn.
	// Called when a turn starts (message delivery to worker).
	// Clears the tool call set, retry count, and newly spawned flag.
	ResetTurn(processID string)

	// MarkAsNewlySpawned marks a process as newly spawned.
	// The first turn after spawn is exempt from enforcement.
	// Called from SpawnProcessHandler after successful process creation.
	MarkAsNewlySpawned(processID string)

	// CheckTurnCompletion checks if required tools were called.
	// Returns a slice of missing required tool names (empty if compliant).
	// Only enforces for workers, not coordinator.
	CheckTurnCompletion(processID string, role repository.ProcessRole) []string

	// IsNewlySpawned returns true if this is the process's first turn after spawn.
	// First turns are exempt from enforcement (workers call signal_ready).
	IsNewlySpawned(processID string) bool

	// ShouldRetry returns true if enforcement retry is allowed.
	// Returns false if max retries exceeded (prevents infinite loops).
	ShouldRetry(processID string) bool

	// IncrementRetry increments the retry counter for a process.
	// Called when sending an enforcement reminder.
	IncrementRetry(processID string)

	// GetReminderMessage generates the follow-up prompt for missing tools.
	// The message instructs the worker to call a required tool.
	GetReminderMessage(processID string, missingTools []string) string

	// OnMaxRetriesExceeded is called when enforcement attempts exhausted.
	// Implementations may log warning, notify coordinator, or emit metric.
	OnMaxRetriesExceeded(processID string, missingTools []string)

	// CleanupProcess removes all tracking state for a process.
	// Called from RetireProcessHandler to prevent memory leaks.
	CleanupProcess(processID string)
}

// ===========================================================================
// TurnCompletionTracker Implementation
// ===========================================================================

// TurnCompletionTracker tracks MCP tool calls within worker turns.
// Thread-safe for concurrent access from multiple worker goroutines.
type TurnCompletionTracker struct {
	// callsThisTurn maps processID → set of tool names called this turn.
	// The inner map uses tool names as keys with true values (set semantics).
	callsThisTurn map[string]map[string]bool

	// retryCount maps processID → number of enforcement retries this turn.
	retryCount map[string]int

	// newlySpawned maps processID → first turn flag.
	// True if this is the process's first turn after spawn.
	newlySpawned map[string]bool

	// mu protects all map operations.
	mu sync.RWMutex

	// logger is an optional callback for logging enforcement events.
	// If nil, logging is silently skipped.
	logger func(format string, args ...any)
}

// NewTurnCompletionTracker creates a new TurnCompletionTracker.
func NewTurnCompletionTracker() *TurnCompletionTracker {
	return &TurnCompletionTracker{
		callsThisTurn: make(map[string]map[string]bool),
		retryCount:    make(map[string]int),
		newlySpawned:  make(map[string]bool),
	}
}

// TurnCompletionTrackerOption configures TurnCompletionTracker.
type TurnCompletionTrackerOption func(*TurnCompletionTracker)

// WithLogger sets a logger callback for enforcement events.
func WithLogger(logger func(format string, args ...any)) TurnCompletionTrackerOption {
	return func(t *TurnCompletionTracker) {
		t.logger = logger
	}
}

// NewTurnCompletionTrackerWithOptions creates a new TurnCompletionTracker with options.
func NewTurnCompletionTrackerWithOptions(opts ...TurnCompletionTrackerOption) *TurnCompletionTracker {
	t := NewTurnCompletionTracker()
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// RecordToolCall records that a worker called a specific tool.
func (t *TurnCompletionTracker) RecordToolCall(processID, toolName string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.callsThisTurn[processID] == nil {
		t.callsThisTurn[processID] = make(map[string]bool)
	}
	t.callsThisTurn[processID][toolName] = true
}

// ResetTurn clears tracking state for a new turn.
func (t *TurnCompletionTracker) ResetTurn(processID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Clear tool calls for this turn
	delete(t.callsThisTurn, processID)

	// Reset retry count for new turn
	delete(t.retryCount, processID)

	// Clear newly spawned flag after first turn
	delete(t.newlySpawned, processID)
}

// MarkAsNewlySpawned marks a process as newly spawned.
func (t *TurnCompletionTracker) MarkAsNewlySpawned(processID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.newlySpawned[processID] = true
}

// CheckTurnCompletion checks if required tools were called.
// Returns a slice of missing required tool names (empty if compliant).
// Only enforces for workers, not coordinator.
func (t *TurnCompletionTracker) CheckTurnCompletion(processID string, role repository.ProcessRole) []string {
	// Coordinators are never subject to enforcement
	if role == repository.RoleCoordinator {
		return nil
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	calls := t.callsThisTurn[processID]

	// Check if any required tool was called
	for _, tool := range RequiredTools {
		if calls != nil && calls[tool] {
			// At least one required tool was called - compliant
			return nil
		}
	}

	// No required tool was called - return all as missing
	return RequiredTools
}

// IsNewlySpawned returns true if this is the process's first turn after spawn.
func (t *TurnCompletionTracker) IsNewlySpawned(processID string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.newlySpawned[processID]
}

// ShouldRetry returns true if enforcement retry is allowed.
func (t *TurnCompletionTracker) ShouldRetry(processID string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.retryCount[processID] < maxEnforcementAttempts
}

// IncrementRetry increments the retry counter for a process.
func (t *TurnCompletionTracker) IncrementRetry(processID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.retryCount[processID]++
}

// GetReminderMessage generates the follow-up prompt for missing tools.
func (t *TurnCompletionTracker) GetReminderMessage(processID string, missingTools []string) string {
	toolList := strings.Join(missingTools, ", ")
	return fmt.Sprintf(`[SYSTEM REMINDER] Your turn completed without calling a required communication tool.

**CRITICAL**: You MUST end your turn with a tool call to one of: %s

If you completed a bd task, call: report_implementation_complete(summary="...")
If you need to communicate with the coordinator, call: post_message(to="COORDINATOR", content="...")
If you just booted up and are ready, call: signal_ready()

Please call one of these tools now to properly complete your turn.`, toolList)
}

// OnMaxRetriesExceeded is called when enforcement attempts exhausted.
func (t *TurnCompletionTracker) OnMaxRetriesExceeded(processID string, missingTools []string) {
	if t.logger != nil {
		t.logger("[WARN] Process %s exceeded max enforcement retries without calling required tools: %v",
			processID, missingTools)
	}
}

// CleanupProcess removes all tracking state for a process.
func (t *TurnCompletionTracker) CleanupProcess(processID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.callsThisTurn, processID)
	delete(t.retryCount, processID)
	delete(t.newlySpawned, processID)
}

// Ensure TurnCompletionTracker implements TurnCompletionEnforcer.
var _ TurnCompletionEnforcer = (*TurnCompletionTracker)(nil)
