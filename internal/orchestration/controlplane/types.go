// Package controlplane provides the foundational types and state management for
// multi-workflow orchestration. It defines the core domain entities including
// WorkflowID, WorkflowState, WorkflowSpec, and WorkflowInstance that enable
// running multiple concurrent AI orchestration workflows.
package controlplane

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/zjrosen/perles/internal/orchestration/mcp"
	"github.com/zjrosen/perles/internal/orchestration/metrics"
	"github.com/zjrosen/perles/internal/orchestration/session"
	v2 "github.com/zjrosen/perles/internal/orchestration/v2"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
)

// WorkflowID uniquely identifies a workflow instance.
// It is a string-based type using UUID format for global uniqueness.
type WorkflowID string

// NewWorkflowID generates a new unique WorkflowID using UUID v4.
func NewWorkflowID() WorkflowID {
	return WorkflowID(uuid.New().String())
}

// String returns the string representation of the WorkflowID.
func (id WorkflowID) String() string {
	return string(id)
}

// IsValid returns true if the WorkflowID is a valid UUID format.
func (id WorkflowID) IsValid() bool {
	if id == "" {
		return false
	}
	_, err := uuid.Parse(string(id))
	return err == nil
}

// WorkflowState represents the lifecycle state of a workflow instance.
// Valid transitions:
//
//	Pending   -> Running, Stopped
//	Running   -> Paused, Completed, Failed, Stopped
//	Paused    -> Running, Stopped
//	Completed -> (terminal)
//	Failed    -> (terminal)
//	Stopped   -> (terminal)
type WorkflowState string

const (
	// WorkflowPending indicates the workflow is created but not yet started.
	WorkflowPending WorkflowState = "pending"
	// WorkflowRunning indicates the workflow is actively executing.
	WorkflowRunning WorkflowState = "running"
	// WorkflowPaused indicates the workflow is temporarily suspended.
	WorkflowPaused WorkflowState = "paused"
	// WorkflowCompleted indicates the workflow has successfully finished.
	WorkflowCompleted WorkflowState = "completed"
	// WorkflowFailed indicates the workflow terminated due to an error.
	WorkflowFailed WorkflowState = "failed"
	// WorkflowStopped indicates the workflow was manually stopped by the user.
	WorkflowStopped WorkflowState = "stopped"
)

// validTransitions defines the allowed state transitions for workflows.
// The key is the current state, the value is a set of valid target states.
var validTransitions = map[WorkflowState]map[WorkflowState]bool{
	WorkflowPending: {
		WorkflowRunning: true,
		WorkflowStopped: true,
	},
	WorkflowRunning: {
		WorkflowPaused:    true,
		WorkflowCompleted: true,
		WorkflowFailed:    true,
		WorkflowStopped:   true,
	},
	WorkflowPaused: {
		WorkflowRunning: true,
		WorkflowStopped: true,
		WorkflowFailed:  true, // Allow failure from paused state (recovery exhaustion)
	},
	// Terminal states have no valid transitions
	WorkflowCompleted: {},
	WorkflowFailed:    {},
	WorkflowStopped:   {},
}

// String returns the string representation of the WorkflowState.
func (s WorkflowState) String() string {
	return string(s)
}

// IsValid returns true if this is a recognized WorkflowState value.
func (s WorkflowState) IsValid() bool {
	_, ok := validTransitions[s]
	return ok
}

// IsTerminal returns true if this state is a terminal state
// (Completed, Failed, or Stopped). Terminal states cannot transition
// to any other state.
func (s WorkflowState) IsTerminal() bool {
	return s == WorkflowCompleted || s == WorkflowFailed || s == WorkflowStopped
}

// CanTransitionTo returns true if transitioning from the current state
// to the target state is valid according to the workflow state machine.
func (s WorkflowState) CanTransitionTo(target WorkflowState) bool {
	allowed, ok := validTransitions[s]
	if !ok {
		return false
	}
	return allowed[target]
}

// ValidTargets returns all states that can be transitioned to from the current state.
func (s WorkflowState) ValidTargets() []WorkflowState {
	allowed, ok := validTransitions[s]
	if !ok {
		return nil
	}
	targets := make([]WorkflowState, 0, len(allowed))
	for target := range allowed {
		targets = append(targets, target)
	}
	return targets
}

// WorkflowSpec defines parameters for creating a new workflow instance.
// It captures all the information needed to initialize and start a workflow.
type WorkflowSpec struct {
	// TemplateID is the identifier of the workflow template to use.
	// This is required and must reference a valid template.
	TemplateID string

	// InitialPrompt is the initial prompt for the coordinator.
	// This is required and provides the starting context for the workflow.
	InitialPrompt string

	// Name is the display name for the workflow.
	// If empty, defaults to the template name.
	Name string

	// WorkDir is the working directory for the workflow.
	// If empty, defaults to the current working directory.
	WorkDir string

	// Labels are arbitrary key-value pairs for filtering and organization.
	Labels map[string]string

	// EpicID is the beads epic ID associated with this workflow.
	// This links the workflow to a tracking epic in the beads issue tracker.
	// Optional - may be empty for workflows not associated with an epic.
	EpicID string

	// WorktreeEnabled indicates whether to create a git worktree for this workflow.
	// When enabled, the workflow runs in an isolated worktree directory.
	WorktreeEnabled bool

	// WorktreeBaseBranch is the branch to base the worktree on (e.g., "main", "develop").
	// Required when WorktreeEnabled is true.
	WorktreeBaseBranch string

	// WorktreeBranchName is an optional custom branch name for the worktree.
	// If empty, a branch name will be auto-generated based on workflow ID.
	WorktreeBranchName string
}

// Validate checks that the WorkflowSpec has all required fields
// and that all values are within valid ranges.
func (s *WorkflowSpec) Validate() error {
	if s.TemplateID == "" {
		return fmt.Errorf("template_id is required")
	}
	if s.InitialPrompt == "" {
		return fmt.Errorf("initial_prompt is required")
	}
	return nil
}

// WorkflowInstance represents a running orchestration workflow.
// Each instance has its own Infrastructure, Session, and resource allocations.
// This is the primary aggregate root for the control plane domain.
type WorkflowInstance struct {
	// Identity
	ID         WorkflowID
	TemplateID string
	Name       string

	// Configuration
	WorkDir       string // Working directory for the workflow
	InitialPrompt string // Initial prompt for the coordinator
	EpicID        string // Beads epic ID associated with this workflow (optional)

	// Worktree configuration (from WorkflowSpec)
	WorktreeEnabled    bool   // Whether worktree was requested
	WorktreeBaseBranch string // Branch to base worktree on
	WorktreeBranchName string // Custom branch name (may be empty)

	// Worktree state (set by Supervisor.AllocateResources() when worktree is created)
	WorktreePath   string // Path to created worktree (empty if not using worktree)
	WorktreeBranch string // Actual branch name (auto-generated or custom)

	// State
	State  WorkflowState
	Labels map[string]string

	// Timestamps
	CreatedAt time.Time
	StartedAt *time.Time
	UpdatedAt time.Time

	// Runtime (owned by this instance, set when workflow is started)
	Infrastructure *v2.Infrastructure
	Session        *session.Session
	HTTPServer     *http.Server           // MCP HTTP server for this workflow
	MCPCoordServer *mcp.CoordinatorServer // MCP coordinator server
	MessageRepo    repository.MessageRepository

	// Resource tracking
	MCPPort       int
	TokensUsed    int64
	ActiveWorkers int

	// Health tracking
	LastHeartbeatAt time.Time
	LastProgressAt  time.Time

	// Lifecycle context (for cancellation)
	Ctx    context.Context
	Cancel context.CancelFunc
}

// NewWorkflowInstance creates a new WorkflowInstance from a WorkflowSpec.
// The instance is created in Pending state and must be started via the Supervisor.
func NewWorkflowInstance(spec *WorkflowSpec) (*WorkflowInstance, error) {
	if err := spec.Validate(); err != nil {
		return nil, fmt.Errorf("invalid spec: %w", err)
	}

	now := time.Now()
	name := spec.Name
	if name == "" {
		name = spec.TemplateID
	}

	// Copy labels to avoid external mutation
	labels := make(map[string]string, len(spec.Labels))
	maps.Copy(labels, spec.Labels)

	inst := &WorkflowInstance{
		ID:            NewWorkflowID(),
		TemplateID:    spec.TemplateID,
		Name:          name,
		WorkDir:       spec.WorkDir,
		InitialPrompt: spec.InitialPrompt,
		EpicID:        spec.EpicID,
		// Worktree configuration from spec
		WorktreeEnabled:    spec.WorktreeEnabled,
		WorktreeBaseBranch: spec.WorktreeBaseBranch,
		WorktreeBranchName: spec.WorktreeBranchName,
		State:              WorkflowPending,
		Labels:             labels,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	return inst, nil
}

// TransitionTo attempts to transition the workflow to the target state.
// Returns an error if the transition is not valid from the current state.
func (w *WorkflowInstance) TransitionTo(target WorkflowState) error {
	if !w.State.CanTransitionTo(target) {
		return fmt.Errorf("invalid state transition from %s to %s", w.State, target)
	}
	w.State = target
	w.UpdatedAt = time.Now()

	// Set StartedAt when transitioning to Running for the first time
	if target == WorkflowRunning && w.StartedAt == nil {
		now := w.UpdatedAt
		w.StartedAt = &now
	}

	return nil
}

// IsTerminal returns true if the workflow is in a terminal state.
func (w *WorkflowInstance) IsTerminal() bool {
	return w.State.IsTerminal()
}

// IsActive returns true if the workflow is in an active (non-terminal) state.
func (w *WorkflowInstance) IsActive() bool {
	return !w.State.IsTerminal()
}

// IsRunning returns true if the workflow is currently running.
func (w *WorkflowInstance) IsRunning() bool {
	return w.State == WorkflowRunning
}

// IsPaused returns true if the workflow is currently paused.
func (w *WorkflowInstance) IsPaused() bool {
	return w.State == WorkflowPaused
}

// RecordHeartbeat updates the last heartbeat timestamp.
// This should be called when any activity is detected from the workflow.
func (w *WorkflowInstance) RecordHeartbeat() {
	w.LastHeartbeatAt = time.Now()
	w.UpdatedAt = w.LastHeartbeatAt
}

// RecordProgress updates the last progress timestamp.
// This should be called when meaningful forward progress is made
// (e.g., phase transition, task completion).
func (w *WorkflowInstance) RecordProgress() {
	now := time.Now()
	w.LastProgressAt = now
	w.LastHeartbeatAt = now // Progress implies heartbeat
	w.UpdatedAt = now
}

// AddTokens adds tokens to the usage counter.
func (w *WorkflowInstance) AddTokens(tokens int64) {
	w.TokensUsed += tokens
	w.UpdatedAt = time.Now()
}

// TokenMetrics returns token usage metrics for this workflow.
func (w *WorkflowInstance) TokenMetrics() *metrics.TokenMetrics {
	return &metrics.TokenMetrics{
		TotalTokens: int(w.TokensUsed),
	}
}
