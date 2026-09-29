// Package controlplane provides health monitoring for workflows.
package controlplane

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/pubsub"
)

// HealthMonitor monitors workflow health by tracking heartbeats and progress.
// It detects stuck workflows and can emit health events for recovery actions.
type HealthMonitor interface {
	// Start begins the health monitoring loop.
	// The monitor subscribes to the event bus and starts periodic health checks.
	Start(ctx context.Context) error

	// Stop stops the health monitoring loop.
	// It is safe to call Stop multiple times or before Start.
	Stop()

	// SetPolicy updates the health monitoring policy.
	SetPolicy(policy HealthPolicy)

	// GetStatus returns the health status for a specific workflow.
	// Returns false if the workflow is not being tracked.
	GetStatus(id WorkflowID) (HealthStatus, bool)

	// GetAllStatuses returns health status for all tracked workflows.
	GetAllStatuses() []HealthStatus

	// RecordHeartbeat records a heartbeat for the specified workflow.
	// This updates LastHeartbeatAt and marks the workflow as healthy.
	RecordHeartbeat(id WorkflowID)

	// RecordProgress records forward progress for the specified workflow.
	// This updates both LastProgressAt and LastHeartbeatAt, and resets recovery attempts.
	RecordProgress(id WorkflowID)

	// TrackWorkflow starts tracking a new workflow.
	// If the workflow is already tracked, this is a no-op.
	TrackWorkflow(id WorkflowID)

	// UntrackWorkflow stops tracking a workflow.
	UntrackWorkflow(id WorkflowID)
}

// HealthEventCallback is called when a health event is detected.
// The callback is invoked asynchronously from the check loop.
type HealthEventCallback func(event HealthEvent)

// HealthMonitorConfig configures the HealthMonitor.
type HealthMonitorConfig struct {
	// Policy defines health monitoring thresholds.
	Policy HealthPolicy

	// CheckInterval is how often the monitor runs health checks.
	// Defaults to 10 seconds if not specified.
	CheckInterval time.Duration

	// EventBus is the control plane event bus to subscribe to for process events.
	// If nil, the monitor will not auto-track heartbeats from events.
	EventBus *pubsub.Broker[ControlPlaneEvent]

	// OnHealthEvent is called when a health event is detected.
	// Can be used to emit events to an external system.
	OnHealthEvent HealthEventCallback

	// RecoveryExecutor executes recovery actions for stuck workflows.
	// If nil, no automatic recovery is performed (events still emitted).
	RecoveryExecutor RecoveryExecutor

	// Clock is used for time operations (for testing).
	// If nil, uses time.Now().
	Clock Clock
}

// Clock interface for time operations (allows testing).
type Clock interface {
	Now() time.Time
}

// realClock implements Clock using the real time.
type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// defaultHealthMonitor is the default implementation of HealthMonitor.
type defaultHealthMonitor struct {
	mu       sync.RWMutex
	policy   HealthPolicy
	statuses map[WorkflowID]*HealthStatus
	clock    Clock

	// Check loop state
	checkInterval    time.Duration
	eventBus         *pubsub.Broker[ControlPlaneEvent]
	onHealthEvent    HealthEventCallback
	recoveryExecutor RecoveryExecutor

	// recovering holds workflows with a recovery action in flight.
	// Recovery actions for the same workflow must not overlap because they
	// escalate (nudge -> replace -> pause -> fail) and mutate workflow state.
	recovering map[WorkflowID]struct{}

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	wg     sync.WaitGroup
}

// NewHealthMonitor creates a new HealthMonitor with the given configuration.
func NewHealthMonitor(cfg HealthMonitorConfig) HealthMonitor {
	clock := cfg.Clock
	if clock == nil {
		clock = realClock{}
	}

	checkInterval := cfg.CheckInterval
	if checkInterval == 0 {
		checkInterval = 10 * time.Second
	}

	return &defaultHealthMonitor{
		policy:           cfg.Policy,
		statuses:         make(map[WorkflowID]*HealthStatus),
		clock:            clock,
		checkInterval:    checkInterval,
		eventBus:         cfg.EventBus,
		onHealthEvent:    cfg.OnHealthEvent,
		recoveryExecutor: cfg.RecoveryExecutor,
		recovering:       make(map[WorkflowID]struct{}),
	}
}

// Start begins the health monitoring loop.
func (m *defaultHealthMonitor) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.done != nil {
		m.mu.Unlock()
		return nil // Already started
	}

	m.ctx, m.cancel = context.WithCancel(ctx)
	m.done = make(chan struct{})
	m.mu.Unlock()

	// Start event bus subscription if configured (with panic recovery)
	if m.eventBus != nil {
		m.wg.Add(1)
		log.SafeGo("healthmonitor.eventLoop", func() {
			defer m.wg.Done()
			m.eventLoopInner()
		})
	}

	// Start periodic check loop (with panic recovery)
	m.wg.Add(1)
	log.SafeGo("healthmonitor.checkLoop", func() {
		defer m.wg.Done()
		m.checkLoopInner()
	})

	return nil
}

// Stop stops the health monitoring loop.
func (m *defaultHealthMonitor) Stop() {
	m.mu.Lock()
	if m.cancel == nil {
		m.mu.Unlock()
		return // Not started
	}
	cancel := m.cancel
	done := m.done
	m.mu.Unlock()

	cancel()
	<-done // Wait for loops and in-flight recoveries to finish
}

// SetPolicy updates the health monitoring policy.
func (m *defaultHealthMonitor) SetPolicy(policy HealthPolicy) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.policy = policy
}

// GetStatus returns the health status for a specific workflow.
func (m *defaultHealthMonitor) GetStatus(id WorkflowID) (HealthStatus, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status, ok := m.statuses[id]
	if !ok {
		return HealthStatus{}, false
	}
	return *status, true
}

// GetAllStatuses returns health status for all tracked workflows.
func (m *defaultHealthMonitor) GetAllStatuses() []HealthStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]HealthStatus, 0, len(m.statuses))
	for _, status := range m.statuses {
		result = append(result, *status)
	}
	return result
}

// RecordHeartbeat records a heartbeat for the specified workflow.
func (m *defaultHealthMonitor) RecordHeartbeat(id WorkflowID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	status, ok := m.statuses[id]
	if !ok {
		// Auto-track if not already tracked
		status = m.createStatus(id)
		m.statuses[id] = status
	}

	status.LastHeartbeatAt = m.clock.Now()
	status.IsHealthy = true
}

// RecordProgress records forward progress for the specified workflow.
func (m *defaultHealthMonitor) RecordProgress(id WorkflowID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	status, ok := m.statuses[id]
	if !ok {
		// Auto-track if not already tracked
		status = m.createStatus(id)
		m.statuses[id] = status
	}

	now := m.clock.Now()
	status.LastProgressAt = now
	status.LastHeartbeatAt = now // Progress implies heartbeat
	status.IsHealthy = true

	// Reset recovery counter on progress (workflow is no longer stuck)
	if status.RecoveryCount > 0 {
		status.ResetRecovery()
	}
}

// TrackWorkflow starts tracking a new workflow.
func (m *defaultHealthMonitor) TrackWorkflow(id WorkflowID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.statuses[id]; ok {
		return // Already tracked
	}

	m.statuses[id] = m.createStatus(id)
}

// UntrackWorkflow stops tracking a workflow.
func (m *defaultHealthMonitor) UntrackWorkflow(id WorkflowID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.statuses, id)
}

// createStatus creates a new HealthStatus for the given workflow ID.
// Must be called with mu held.
func (m *defaultHealthMonitor) createStatus(id WorkflowID) *HealthStatus {
	now := m.clock.Now()
	return &HealthStatus{
		WorkflowID:      id,
		IsHealthy:       true,
		LastHeartbeatAt: now,
		LastProgressAt:  now,
		RecoveryCount:   0,
	}
}

// checkLoopInner runs periodic health checks. Called by the wrapped checkLoop goroutine.
func (m *defaultHealthMonitor) checkLoopInner() {
	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			m.signalDone()
			return
		case <-ticker.C:
			m.runHealthCheck()
		}
	}
}

// signalDone signals that all loops have completed.
// Only the last loop to finish should close done.
func (m *defaultHealthMonitor) signalDone() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Wait for all goroutines then close done (with panic recovery)
	log.SafeGo("healthmonitor.signalDone", func() {
		m.wg.Wait()
		close(m.done)
	})
}

// runHealthCheck checks all tracked workflows for health issues.
func (m *defaultHealthMonitor) runHealthCheck() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.clock.Now()
	policy := m.policy

	for id, status := range m.statuses {
		timeSinceHeartbeat := now.Sub(status.LastHeartbeatAt)
		timeSinceProgress := now.Sub(status.LastProgressAt)

		log.Debug(log.CatOrch, "Health check tick",
			"workflow_id", id,
			"time_since_progress", timeSinceProgress.Truncate(time.Second),
			"time_since_heartbeat", timeSinceHeartbeat.Truncate(time.Second),
			"progress_timeout", policy.ProgressTimeout,
			"recovery_count", status.RecoveryCount,
			"is_stuck", timeSinceProgress > policy.ProgressTimeout)

		// Check heartbeat timeout
		if timeSinceHeartbeat > policy.HeartbeatTimeout {
			if status.IsHealthy {
				status.IsHealthy = false
				m.emitEvent(NewHealthEvent(HealthHeartbeatMissed, id).
					WithDetails("No heartbeat for " + timeSinceHeartbeat.String()))
			}
		}

		// Check progress timeout (stuck detection)
		// Stuck is computed from LastProgressAt vs ProgressTimeout
		if timeSinceProgress > policy.ProgressTimeout {
			// Emit "stuck suspected" event only once (when RecoveryCount is 0)
			if status.RecoveryCount == 0 {
				m.emitEvent(NewHealthEvent(HealthStuckSuspected, id).
					WithDetails("No progress for " + timeSinceProgress.Truncate(time.Second).String()))
			}

			// Trigger recovery if needed
			m.triggerRecoveryIfNeeded(id, status, policy, now)
		}
	}
}

// triggerRecoveryIfNeeded checks if recovery should be triggered and executes it.
// Must be called with mu held.
func (m *defaultHealthMonitor) triggerRecoveryIfNeeded(id WorkflowID, status *HealthStatus, policy HealthPolicy, now time.Time) {
	// Skip if no recovery executor is configured
	if m.recoveryExecutor == nil {
		log.Debug(log.CatOrch, "Recovery skipped: no executor configured", "workflow_id", id)
		return
	}

	// Skip if the previous recovery for this workflow has not finished yet
	if _, inFlight := m.recovering[id]; inFlight {
		log.Debug(log.CatOrch, "Recovery skipped: previous recovery still in progress", "workflow_id", id)
		return
	}

	// Check if recovery is needed based on status and policy (using our clock)
	needsRecovery := status.NeedsRecoveryAt(policy, now)
	var timeSinceLastRecovery time.Duration
	if status.LastRecoveryAt != nil {
		timeSinceLastRecovery = now.Sub(*status.LastRecoveryAt)
	}
	log.Debug(log.CatOrch, "Recovery check",
		"workflow_id", id,
		"needs_recovery", needsRecovery,
		"recovery_count", status.RecoveryCount,
		"max_recoveries", policy.MaxRecoveries,
		"time_since_last_recovery", timeSinceLastRecovery.Truncate(time.Second),
		"recovery_backoff", policy.RecoveryBackoff)

	if !needsRecovery {
		return
	}

	// Determine the appropriate recovery action
	action := DetermineRecoveryActionAt(status, policy, now)
	log.Debug(log.CatOrch, "Recovery action determined",
		"workflow_id", id,
		"action", action,
		"action_valid", action >= 0)

	if action < 0 {
		// No recovery action available - emit "still stuck" event periodically
		// to provide visibility into limbo state (once per backoff period)
		if status.LastRecoveryAt != nil {
			timeSinceLastRecovery := now.Sub(*status.LastRecoveryAt)
			if timeSinceLastRecovery >= policy.RecoveryBackoff {
				// Update LastRecoveryAt to rate-limit the event (without incrementing count)
				lastRecovery := now
				status.LastRecoveryAt = &lastRecovery
				timeSinceProgress := now.Sub(status.LastProgressAt)
				m.emitEvent(NewHealthEvent(HealthStillStuck, id).
					WithDetails(fmt.Sprintf("No progress for %s, no recovery actions available (count=%d)",
						timeSinceProgress.Truncate(time.Second), status.RecoveryCount)))
			}
		}
		return
	}

	// Record the recovery attempt (increment count and timestamp using our clock)
	status.RecordRecoveryAttemptAt(now)
	m.recovering[id] = struct{}{}

	// Execute recovery asynchronously to avoid blocking the check loop (with panic recovery).
	// The goroutine is tracked by wg and derives its context from the monitor's,
	// so Stop cancels in-flight recoveries and waits for them to finish.
	parent := m.ctx
	m.wg.Add(1)
	log.SafeGo("healthmonitor.executeRecovery", func() {
		defer m.wg.Done()
		defer m.finishRecovery(id)

		ctx, cancel := context.WithTimeout(parent, 30*time.Second)
		defer cancel()

		// Recovery executor will emit its own events
		_ = m.recoveryExecutor.ExecuteRecovery(ctx, id, action)
	})
}

// finishRecovery marks the in-flight recovery for a workflow as done,
// allowing the next health check to escalate if the workflow is still stuck.
func (m *defaultHealthMonitor) finishRecovery(id WorkflowID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.recovering, id)
}

// emitEvent emits a health event if a callback is configured.
// Must be called with mu held.
func (m *defaultHealthMonitor) emitEvent(event HealthEvent) {
	if m.onHealthEvent != nil {
		// Emit asynchronously to avoid blocking the check loop (with panic recovery)
		log.SafeGo("healthmonitor.emitEvent", func() {
			m.onHealthEvent(event)
		})
	}
}

// eventLoopInner subscribes to the event bus and processes events. Called by the wrapped eventLoop goroutine.
func (m *defaultHealthMonitor) eventLoopInner() {
	ch := m.eventBus.Subscribe(m.ctx)

	for {
		select {
		case <-m.ctx.Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			m.processEvent(event)
		}
	}
}

// processEvent handles an event from the event bus.
func (m *defaultHealthMonitor) processEvent(event pubsub.Event[ControlPlaneEvent]) {
	cpEvent := event.Payload

	// Extract ProcessEvent from the ControlPlaneEvent payload
	processEvent, ok := cpEvent.Payload.(events.ProcessEvent)
	if !ok {
		return
	}

	// Use the WorkflowID from the ControlPlaneEvent envelope (preferred)
	// Fall back to ProcessID if WorkflowID is empty (backwards compatibility)
	workflowID := cpEvent.WorkflowID
	if workflowID == "" {
		workflowID = WorkflowID(processEvent.ProcessID)
	}

	// Stop tracking completed workflows - they don't need health monitoring
	if processEvent.Type == events.ProcessWorkflowComplete {
		m.UntrackWorkflow(workflowID)
		return
	}

	if isProgressEvent(processEvent) {
		m.RecordProgress(workflowID)
	} else {
		// Any process event is at least a heartbeat
		m.RecordHeartbeat(workflowID)
	}
}

// isProgressEvent determines if an event represents forward progress.
// Progress events indicate meaningful workflow advancement and reset the stuck timer.
// Only worker output counts as progress - coordinator output could be a nudge response.
func isProgressEvent(event events.ProcessEvent) bool {
	return event.Type == events.ProcessOutput && event.Role == events.RoleWorker
}
