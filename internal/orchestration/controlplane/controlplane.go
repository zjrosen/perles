// Package controlplane provides the ControlPlane interface, the main entry point
// for managing multiple concurrent AI orchestration workflows.
package controlplane

import (
	"context"
	"errors"
	"fmt"

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

	// Stop terminates a workflow and releases all resources.
	Stop(ctx context.Context, id WorkflowID, opts StopOptions) error

	// Get retrieves a workflow by ID.
	// Returns ErrWorkflowNotFound if the workflow does not exist.
	Get(ctx context.Context, id WorkflowID) (*WorkflowInstance, error)

	// List returns workflows matching the query.
	List(ctx context.Context, q ListQuery) ([]*WorkflowInstance, error)

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

	return nil
}

// Stop terminates a workflow and releases all resources.
func (cp *defaultControlPlane) Stop(ctx context.Context, id WorkflowID, opts StopOptions) error {
	// Get workflow from registry
	inst, ok := cp.registry.Get(id)
	if !ok {
		return ErrWorkflowNotFound
	}

	// Detach from cross-workflow event bus before stopping
	// This stops forwarding events from the workflow's infrastructure
	cp.eventBus.DetachWorkflow(id)

	// Delegate to supervisor
	if err := cp.supervisor.Stop(ctx, inst, opts); err != nil {
		return fmt.Errorf("stopping workflow: %w", err)
	}

	return nil
}

// handleLifecycleEvent handles lifecycle events from the event bus.
// This is called synchronously when a lifecycle event is detected.
func (cp *defaultControlPlane) handleLifecycleEvent(inst *WorkflowInstance, event ControlPlaneEvent) {
	switch event.Type {
	case EventWorkflowCompleted:
		// Transition workflow to Completed state
		if inst.State.CanTransitionTo(WorkflowCompleted) {
			if err := inst.TransitionTo(WorkflowCompleted); err != nil {
				log.Error(log.CatOrch, "Failed to transition workflow to Completed",
					"workflowID", inst.ID, "error", err)
			} else {
				log.Debug(log.CatOrch, "Workflow completed via signal_workflow_complete",
					"workflowID", inst.ID, "name", inst.Name)
			}
		}

	case EventWorkflowFailed:
		// Transition workflow to Failed state
		if inst.State.CanTransitionTo(WorkflowFailed) {
			if err := inst.TransitionTo(WorkflowFailed); err != nil {
				log.Error(log.CatOrch, "Failed to transition workflow to Failed",
					"workflowID", inst.ID, "error", err)
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
// 2. Stop all running/paused workflows (with grace period from context)
// 3. Close the CrossWorkflowEventBus
// 4. Release all scheduler resources (if configured)
//
// If the context is cancelled or times out during graceful shutdown,
// remaining workflows are force-stopped.
func (cp *defaultControlPlane) Shutdown(ctx context.Context) error {
	var errs []error

	// Step 1: Stop HealthMonitor
	if cp.healthMonitor != nil {
		cp.healthMonitor.Stop()
	}

	// Step 2: Stop all running/paused workflows
	// Query for all non-terminal workflows
	activeWorkflows := cp.registry.List(ListQuery{
		States: []WorkflowState{WorkflowRunning, WorkflowPaused, WorkflowPending},
	})

	for _, inst := range activeWorkflows {
		// Always force stop during shutdown - we're terminating everything anyway.
		// This avoids waiting for graceful process draining which can be slow.
		stopErr := cp.Stop(ctx, inst.ID, StopOptions{
			Reason: "ControlPlane shutdown",
			Force:  true,
		})
		if stopErr != nil {
			errs = append(errs, fmt.Errorf("workflow %s: %w", inst.ID, stopErr))
		}
	}

	// Step 3: Close the EventBus
	if cp.eventBus != nil {
		cp.eventBus.Close()
	}

	// Note: Individual workflow resources are released in Stop() via ReleaseAll().
	// Scheduler doesn't have a global Close() method - no cleanup needed here.

	// Return aggregated errors if any
	if len(errs) > 0 {
		return fmt.Errorf("shutdown completed with %d errors: %w", len(errs), errors.Join(errs...))
	}

	return nil
}
