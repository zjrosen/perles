// Package controlplane provides the ControlPlane interface, the main entry point
// for managing multiple concurrent AI orchestration workflows.
package controlplane

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/zjrosen/perles/internal/log"
)

// ErrWorkflowNotFound is returned when a workflow is not found in the registry.
var ErrWorkflowNotFound = fmt.Errorf("workflow not found")

// ControlPlane is the main entry point for managing workflows.
// It coordinates the Registry and Supervisor to provide a unified API
// for workflow lifecycle management.
type ControlPlane interface {
	// Create creates a new workflow instance in Pending state.
	// The workflow must be started with Start() to begin execution.
	Create(ctx context.Context, spec WorkflowSpec) (WorkflowID, error)

	// Start transitions a pending workflow to running.
	// Allocates resources, creates infrastructure, and spawns the coordinator.
	Start(ctx context.Context, id WorkflowID) error

	// Pause suspends a running workflow, stopping all processes and clearing queues.
	// The infrastructure remains allocated for potential resumption.
	// Returns ErrWorkflowNotFound if the workflow does not exist.
	Pause(ctx context.Context, id WorkflowID) error

	// Resume restarts a paused workflow by respawning the coordinator.
	// Sends a system message to the coordinator with pause context.
	// Returns ErrWorkflowNotFound if the workflow does not exist.
	Resume(ctx context.Context, id WorkflowID) error

	// Complete marks a workflow as completed and persists the final state.
	// This should be called when the coordinator signals completion.
	// Returns ErrWorkflowNotFound if the workflow does not exist.
	Complete(ctx context.Context, id WorkflowID) error

	// Fail marks a workflow as failed and persists the final state.
	// This should be called when a workflow encounters an unrecoverable error.
	// Returns ErrWorkflowNotFound if the workflow does not exist.
	Fail(ctx context.Context, id WorkflowID) error

	// Get retrieves a workflow by ID.
	// Returns ErrWorkflowNotFound if the workflow does not exist.
	Get(ctx context.Context, id WorkflowID) (*WorkflowInstance, error)

	// List returns workflows matching the query.
	List(ctx context.Context, q ListQuery) ([]*WorkflowInstance, error)

	// Registry returns the underlying workflow registry.
	// This enables direct registry updates for dashboard operations.
	Registry() Registry

	// Archive marks a workflow as archived. Archived workflows are excluded
	// from List queries by default. This is only supported when session
	// persistence is enabled (DurableRegistry). Returns nil when using
	// in-memory registry.
	// Returns ErrWorkflowNotFound if the workflow does not exist.
	Archive(ctx context.Context, id WorkflowID) error

	// === Event Subscription ===

	// Subscribe returns a channel of all control plane events.
	// Events are tagged with workflow ID for filtering.
	// The returned function must be called to unsubscribe and clean up resources.
	// The channel is automatically closed when the context is cancelled.
	Subscribe(ctx context.Context) (<-chan ControlPlaneEvent, func())

	// SubscribeWorkflow returns events for a specific workflow only.
	// This is a convenience method equivalent to SubscribeFiltered with
	// a filter containing only the specified workflow ID.
	// The returned function must be called to unsubscribe and clean up resources.
	SubscribeWorkflow(ctx context.Context, id WorkflowID) (<-chan ControlPlaneEvent, func())

	// SubscribeFiltered returns events matching the specified filter criteria.
	// The filter can specify event types to include, workflow IDs to include,
	// and event types to exclude. See EventFilter for details.
	// The returned function must be called to unsubscribe and clean up resources.
	SubscribeFiltered(ctx context.Context, filter EventFilter) (<-chan ControlPlaneEvent, func())

	// === Health Monitoring ===

	// GetHealthStatus returns the health status for a specific workflow.
	// Returns false if the workflow is not being tracked by the HealthMonitor.
	GetHealthStatus(id WorkflowID) (HealthStatus, bool)

	// === Lifecycle Management ===

	// Shutdown gracefully stops all running workflows and releases all resources.
	// It respects the provided context's deadline/timeout for the shutdown process.
	// If the context is cancelled or times out, remaining workflows are force-stopped.
	// Returns an error if any workflow fails to stop cleanly (aggregated).
	Shutdown(ctx context.Context) error
}

// ControlPlaneConfig configures the ControlPlane.
type ControlPlaneConfig struct {
	// Registry stores workflow instances.
	Registry Registry
	// Supervisor manages workflow lifecycle.
	Supervisor Supervisor
	// EventBus aggregates events from all workflows (optional).
	// If provided, enables Subscribe, SubscribeWorkflow, and SubscribeFiltered.
	// If nil, a new CrossWorkflowEventBus is created automatically.
	EventBus *CrossWorkflowEventBus
	// HealthMonitor monitors workflow health (optional).
	// If provided, it will be stopped during Shutdown.
	HealthMonitor HealthMonitor
}

// Validate checks that all required fields are provided.
func (c *ControlPlaneConfig) Validate() error {
	if c.Registry == nil {
		return fmt.Errorf("Registry is required")
	}
	if c.Supervisor == nil {
		return fmt.Errorf("Supervisor is required")
	}
	return nil
}

// defaultControlPlane is the default implementation of ControlPlane.
type defaultControlPlane struct {
	registry      Registry
	supervisor    Supervisor
	eventBus      *CrossWorkflowEventBus
	healthMonitor HealthMonitor
}

// NewControlPlane creates a new ControlPlane with the given configuration.
func NewControlPlane(cfg ControlPlaneConfig) (ControlPlane, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	eventBus := cfg.EventBus
	if eventBus == nil {
		eventBus = NewCrossWorkflowEventBus()
	}

	cp := &defaultControlPlane{
		registry:      cfg.Registry,
		supervisor:    cfg.Supervisor,
		eventBus:      eventBus,
		healthMonitor: cfg.HealthMonitor,
	}

	// Set up lifecycle callback to handle workflow state transitions
	eventBus.SetLifecycleCallback(cp.handleLifecycleEvent)

	return cp, nil
}

// Create creates a new workflow instance in Pending state.
func (cp *defaultControlPlane) Create(ctx context.Context, spec WorkflowSpec) (WorkflowID, error) {
	// Validate spec
	if err := spec.Validate(); err != nil {
		return "", fmt.Errorf("invalid spec: %w", err)
	}

	// Create the workflow instance
	inst, err := NewWorkflowInstance(&spec)
	if err != nil {
		return "", fmt.Errorf("creating workflow instance: %w", err)
	}

	// Store in registry
	if err := cp.registry.Put(inst); err != nil {
		return "", fmt.Errorf("storing workflow: %w", err)
	}

	// Emit workflow created event so subscribers (e.g., dashboard) can update
	cp.eventBus.Publish(ControlPlaneEvent{
		Type:         EventWorkflowCreated,
		WorkflowID:   inst.ID,
		WorkflowName: inst.Name,
		TemplateID:   inst.TemplateID,
		State:        inst.State,
		Timestamp:    inst.CreatedAt,
	})

	return inst.ID, nil
}

// Start transitions a pending workflow to running.
func (cp *defaultControlPlane) Start(ctx context.Context, id WorkflowID) error {
	// Get workflow from registry
	inst, ok := cp.registry.Get(id)
	if !ok {
		return ErrWorkflowNotFound
	}

	// Phase 1: Allocate resources (infrastructure, MCP server, session)
	if err := cp.supervisor.AllocateResources(ctx, inst); err != nil {
		return fmt.Errorf("allocating resources: %w", err)
	}

	// Attach workflow's event bus to the cross-workflow event bus BEFORE spawning coordinator.
	// This ensures subscribers (e.g., Dashboard) receive the coordinator spawned event.
	cp.eventBus.AttachWorkflow(inst)

	// Phase 2: Spawn coordinator (now the event will be captured)
	if err := cp.supervisor.SpawnCoordinator(ctx, inst); err != nil {
		// Detach on failure to avoid dangling subscriptions
		cp.eventBus.DetachWorkflow(id)
		return fmt.Errorf("spawning coordinator: %w", err)
	}

	// Persist the running state and resource allocations to registry (for SQLite-backed registries).
	// AllocateResources modifies: WorkDir, WorktreePath, WorktreeBranch, SessionDir, MCPPort.
	// SpawnCoordinator modifies: State, StartedAt, ActiveWorkers.
	//nolint:staticcheck // SA9003: Intentionally ignoring error - in-memory state is authoritative
	if err := cp.registry.Update(id, func(w *WorkflowInstance) {
		// State transitions
		w.State = inst.State
		w.StartedAt = inst.StartedAt
		// Resource allocations from AllocateResources
		w.WorkDir = inst.WorkDir
		w.WorktreePath = inst.WorktreePath
		w.WorktreeBranch = inst.WorktreeBranch
		w.SessionDir = inst.SessionDir
		w.MCPPort = inst.MCPPort
		w.ActiveWorkers = inst.ActiveWorkers
	}); err != nil {
		// Log but don't fail - the in-memory state is already updated
	}

	return nil
}

// Pause suspends a running workflow, stopping all processes and clearing queues.
// The infrastructure remains allocated for potential resumption.
func (cp *defaultControlPlane) Pause(ctx context.Context, id WorkflowID) error {
	// Get workflow from registry
	inst, ok := cp.registry.Get(id)
	if !ok {
		return ErrWorkflowNotFound
	}

	// Delegate to supervisor
	if err := cp.supervisor.Pause(ctx, inst); err != nil {
		return err
	}

	// Persist the paused state and runtime metrics to registry (for SQLite-backed registries).
	// This captures the current state for cold resume after app restart.
	//nolint:staticcheck // SA9003: Intentionally ignoring error - in-memory state is authoritative
	if err := cp.registry.Update(id, func(w *WorkflowInstance) {
		w.State = inst.State
		w.PausedAt = inst.PausedAt
		w.TokensUsed = inst.TokensUsed
		w.ActiveWorkers = inst.ActiveWorkers
		w.LastHeartbeatAt = inst.LastHeartbeatAt
		w.LastProgressAt = inst.LastProgressAt
	}); err != nil {
		// Log but don't fail - the in-memory state is already updated
		// This handles the case where registry.Get returned a reconstituted session
		// that isn't in the runtime map
	}

	// Emit workflow paused event
	cp.eventBus.Publish(ControlPlaneEvent{
		Type:         EventWorkflowPaused,
		WorkflowID:   inst.ID,
		WorkflowName: inst.Name,
		TemplateID:   inst.TemplateID,
		State:        inst.State,
		Timestamp:    inst.PausedAt,
	})

	return nil
}

// Resume restarts a paused workflow by respawning the coordinator.
// Sends a system message to the coordinator with pause context.
//
// For "cold resume" (workflow loaded from SQLite without runtime infrastructure),
// this method first allocates resources before resuming the coordinator.
func (cp *defaultControlPlane) Resume(ctx context.Context, id WorkflowID) error {
	// Get workflow from registry
	inst, ok := cp.registry.Get(id)
	if !ok {
		return ErrWorkflowNotFound
	}

	// Cold resume detection: workflow is Paused but has no Infrastructure
	// This happens when a paused workflow is loaded from SQLite after app restart.
	if inst.Infrastructure == nil {
		log.Debug(log.CatOrch, "Cold resume detected - allocating resources", "workflowID", id)

		// Allocate resources (creates infrastructure, MCP server, reopens session)
		// AllocateResources accepts Paused state for cold resume scenarios.
		if err := cp.supervisor.AllocateResources(ctx, inst); err != nil {
			return fmt.Errorf("allocating resources for cold resume: %w", err)
		}

		// Attach workflow's event bus to the cross-workflow event bus
		cp.eventBus.AttachWorkflow(inst)

		// Attach runtime to DurableRegistry if applicable
		if dr, ok := cp.registry.(*DurableRegistry); ok {
			if err := dr.AttachRuntime(inst); err != nil {
				// Log warning but continue - workflow may already be attached
				log.Debug(log.CatOrch, "Failed to attach runtime to registry",
					"workflowID", id, "error", err)
			}
		}
	}

	// Delegate to supervisor (handles warm and cold resume identically from here)
	if err := cp.supervisor.Resume(ctx, inst); err != nil {
		return err
	}

	// Persist the resumed state to registry (for SQLite-backed registries)
	//nolint:staticcheck // SA9003: Intentionally ignoring error - in-memory state is authoritative
	if err := cp.registry.Update(id, func(w *WorkflowInstance) {
		w.State = inst.State
	}); err != nil {
		// Log but don't fail - the in-memory state is already updated
	}

	// Emit workflow resumed event
	cp.eventBus.Publish(ControlPlaneEvent{
		Type:         EventWorkflowResumed,
		WorkflowID:   inst.ID,
		WorkflowName: inst.Name,
		TemplateID:   inst.TemplateID,
		State:        inst.State,
		Timestamp:    inst.UpdatedAt,
	})

	return nil
}

// Complete marks a workflow as completed and persists the final state.
func (cp *defaultControlPlane) Complete(ctx context.Context, id WorkflowID) error {
	// Get workflow from registry
	inst, ok := cp.registry.Get(id)
	if !ok {
		return ErrWorkflowNotFound
	}

	// Transition to completed state
	now := time.Now()
	if err := inst.TransitionTo(WorkflowCompleted); err != nil {
		return fmt.Errorf("transitioning to completed: %w", err)
	}
	inst.CompletedAt = &now

	// Persist the completed state to registry (for SQLite-backed registries)
	//nolint:staticcheck // SA9003: Intentionally ignoring error - in-memory state is authoritative
	if err := cp.registry.Update(id, func(w *WorkflowInstance) {
		w.State = inst.State
		w.CompletedAt = inst.CompletedAt
		w.TokensUsed = inst.TokensUsed
		w.ActiveWorkers = inst.ActiveWorkers
	}); err != nil {
		// Log but don't fail - the in-memory state is already updated
	}

	// Emit workflow completed event
	cp.eventBus.Publish(ControlPlaneEvent{
		Type:         EventWorkflowCompleted,
		WorkflowID:   inst.ID,
		WorkflowName: inst.Name,
		TemplateID:   inst.TemplateID,
		State:        inst.State,
		Timestamp:    now,
	})

	return nil
}

// Fail marks a workflow as failed and persists the final state.
func (cp *defaultControlPlane) Fail(ctx context.Context, id WorkflowID) error {
	// Get workflow from registry
	inst, ok := cp.registry.Get(id)
	if !ok {
		return ErrWorkflowNotFound
	}

	// Transition to failed state
	now := time.Now()
	if err := inst.TransitionTo(WorkflowFailed); err != nil {
		return fmt.Errorf("transitioning to failed: %w", err)
	}
	inst.CompletedAt = &now

	// Persist the failed state to registry (for SQLite-backed registries)
	//nolint:staticcheck // SA9003: Intentionally ignoring error - in-memory state is authoritative
	if err := cp.registry.Update(id, func(w *WorkflowInstance) {
		w.State = inst.State
		w.CompletedAt = inst.CompletedAt
		w.TokensUsed = inst.TokensUsed
		w.ActiveWorkers = inst.ActiveWorkers
	}); err != nil {
		// Log but don't fail - the in-memory state is already updated
	}

	// Emit workflow failed event
	cp.eventBus.Publish(ControlPlaneEvent{
		Type:         EventWorkflowFailed,
		WorkflowID:   inst.ID,
		WorkflowName: inst.Name,
		TemplateID:   inst.TemplateID,
		State:        inst.State,
		Timestamp:    now,
	})

	return nil
}

// stopWorkflow terminates a workflow and releases all resources.
// This transitions the workflow to Failed state, which is a terminal state.
// For running workflows, it first pauses them to persist state for cold resume,
// then proceeds with full shutdown.
func (cp *defaultControlPlane) stopWorkflow(ctx context.Context, id WorkflowID, opts StopOptions) error {
	// Get workflow from registry
	inst, ok := cp.registry.Get(id)
	if !ok {
		return ErrWorkflowNotFound
	}

	// For running workflows, pause first to persist state for potential cold resume
	// This updates the registry with runtime metrics and paused state
	if inst.State == WorkflowRunning {
		if pauseErr := cp.Pause(ctx, id); pauseErr != nil {
			// Log but continue with shutdown - we still want to release resources
			log.Debug(log.CatOrch, "Failed to pause workflow before shutdown",
				"workflowID", id, "error", pauseErr)
		}
	}

	// Detach from cross-workflow event bus before stopping
	// This stops forwarding events from the workflow's infrastructure
	cp.eventBus.DetachWorkflow(id)

	// Delegate to supervisor for full resource cleanup (transitions state to Failed)
	if err := cp.supervisor.Shutdown(ctx, inst, opts); err != nil {
		return fmt.Errorf("stopping workflow: %w", err)
	}

	return nil
}

// handleLifecycleEvent handles lifecycle events from the event bus.
// This is called synchronously when a lifecycle event is detected.
func (cp *defaultControlPlane) handleLifecycleEvent(inst *WorkflowInstance, event ControlPlaneEvent) {
	switch event.Type {
	case EventWorkflowCompleted:
		// Complete the workflow via the standard Complete() method
		if inst.State.CanTransitionTo(WorkflowCompleted) {
			if err := cp.Complete(context.Background(), inst.ID); err != nil {
				log.Error(log.CatOrch, "Failed to complete workflow",
					"workflowID", inst.ID, "error", err)
			} else {
				log.Debug(log.CatOrch, "Workflow completed via signal_workflow_complete",
					"workflowID", inst.ID, "name", inst.Name)
			}
		}

	case EventWorkflowFailed:
		// Fail the workflow via the standard Fail() method
		if inst.State.CanTransitionTo(WorkflowFailed) {
			if err := cp.Fail(context.Background(), inst.ID); err != nil {
				log.Error(log.CatOrch, "Failed to fail workflow",
					"workflowID", inst.ID, "error", err)
			} else {
				log.Debug(log.CatOrch, "Workflow failed via lifecycle event",
					"workflowID", inst.ID, "name", inst.Name)
			}
		}
	}
}

// Get retrieves a workflow by ID.
func (cp *defaultControlPlane) Get(ctx context.Context, id WorkflowID) (*WorkflowInstance, error) {
	inst, ok := cp.registry.Get(id)
	if !ok {
		return nil, ErrWorkflowNotFound
	}
	return inst, nil
}

// List returns workflows matching the query.
func (cp *defaultControlPlane) List(ctx context.Context, q ListQuery) ([]*WorkflowInstance, error) {
	return cp.registry.List(q), nil
}

// Registry returns the underlying workflow registry.
func (cp *defaultControlPlane) Registry() Registry {
	return cp.registry
}

// Archive marks a workflow as archived.
func (cp *defaultControlPlane) Archive(ctx context.Context, id WorkflowID) error {
	return cp.registry.Archive(id)
}

// Subscribe returns a channel of all control plane events.
// The returned function must be called to unsubscribe and clean up resources.
// The channel is automatically closed when the context is cancelled.
func (cp *defaultControlPlane) Subscribe(ctx context.Context) (<-chan ControlPlaneEvent, func()) {
	// Create a cancellable context for this subscription
	subCtx, cancel := context.WithCancel(ctx)

	// Subscribe to the event bus broker
	brokerCh := cp.eventBus.broker.Subscribe(subCtx)

	// Create output channel
	outputCh := make(chan ControlPlaneEvent, 64)

	// Forward events from broker to output channel with panic recovery
	log.SafeGo("controlplane.Subscribe", func() {
		defer close(outputCh)
		for {
			select {
			case <-subCtx.Done():
				return
			case event, ok := <-brokerCh:
				if !ok {
					return
				}
				select {
				case outputCh <- event.Payload:
				case <-subCtx.Done():
					return
				}
			}
		}
	})

	return outputCh, cancel
}

// SubscribeWorkflow returns events for a specific workflow only.
// This is a convenience method equivalent to SubscribeFiltered with
// a filter containing only the specified workflow ID.
func (cp *defaultControlPlane) SubscribeWorkflow(ctx context.Context, id WorkflowID) (<-chan ControlPlaneEvent, func()) {
	return cp.SubscribeFiltered(ctx, EventFilter{
		WorkflowIDs: []WorkflowID{id},
	})
}

// SubscribeFiltered returns events matching the specified filter criteria.
// The filter can specify event types to include, workflow IDs to include,
// and event types to exclude. See EventFilter for details.
func (cp *defaultControlPlane) SubscribeFiltered(ctx context.Context, filter EventFilter) (<-chan ControlPlaneEvent, func()) {
	// Create a cancellable context for this subscription
	subCtx, cancel := context.WithCancel(ctx)

	// Subscribe to the event bus broker
	brokerCh := cp.eventBus.broker.Subscribe(subCtx)

	// Create output channel
	outputCh := make(chan ControlPlaneEvent, 64)

	// Forward filtered events from broker to output channel with panic recovery
	log.SafeGo("controlplane.SubscribeFiltered", func() {
		defer close(outputCh)
		for {
			select {
			case <-subCtx.Done():
				return
			case event, ok := <-brokerCh:
				if !ok {
					return
				}
				// Apply filter
				if filter.Matches(event.Payload) {
					select {
					case outputCh <- event.Payload:
					case <-subCtx.Done():
						return
					}
				}
			}
		}
	})

	return outputCh, cancel
}

// EventBus returns the underlying CrossWorkflowEventBus.
// This allows external code to attach workflows, publish events directly,
// or use pubsub.ContinuousListener for Bubble Tea integration.
func (cp *defaultControlPlane) EventBus() *CrossWorkflowEventBus {
	return cp.eventBus
}

// GetHealthStatus returns the health status for a specific workflow.
// Returns false if the workflow is not being tracked or if no HealthMonitor is configured.
func (cp *defaultControlPlane) GetHealthStatus(id WorkflowID) (HealthStatus, bool) {
	if cp.healthMonitor == nil {
		return HealthStatus{}, false
	}
	return cp.healthMonitor.GetStatus(id)
}

// Shutdown gracefully stops all running workflows and releases all resources.
// The shutdown sequence is:
// 1. Stop the HealthMonitor (if configured)
// 2. Stop all active workflows via stopWorkflow (handles pause + resource cleanup)
// 3. Close the CrossWorkflowEventBus
// 4. Release all scheduler resources (if configured)
func (cp *defaultControlPlane) Shutdown(ctx context.Context) error {
	var errs []error

	// Step 1: Stop HealthMonitor
	if cp.healthMonitor != nil {
		cp.healthMonitor.Stop()
	}

	// Step 2: Stop all active workflows owned by this process
	// - Running and Paused states need proper resource cleanup via supervisor.Shutdown
	// - Pending workflows have no resources allocated, skip them
	// - stopWorkflow handles pausing running workflows before full shutdown
	// - Only workflows owned by current PID (filtered at DB level)
	// - Archived workflows are excluded by default
	currentPID := os.Getpid()
	activeWorkflows := cp.registry.List(ListQuery{
		States:   []WorkflowState{WorkflowRunning, WorkflowPaused},
		OwnerPID: &currentPID,
	})

	for _, inst := range activeWorkflows {
		// stopWorkflow handles: pause (for running) -> detach event bus -> supervisor.Shutdown
		if stopErr := cp.stopWorkflow(ctx, inst.ID, StopOptions{}); stopErr != nil {
			errs = append(errs, fmt.Errorf("workflow %s: %w", inst.ID, stopErr))
		}
	}

	// Step 3: Close the EventBus
	if cp.eventBus != nil {
		cp.eventBus.Close()
	}

	// Return aggregated errors if any
	if len(errs) > 0 {
		return fmt.Errorf("shutdown completed with %d errors: %w", len(errs), errors.Join(errs...))
	}

	return nil
}
