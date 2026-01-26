// Package controlplane provides DurableRegistry for persisting workflow state to SQLite.
package controlplane

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"sync"

	"github.com/zjrosen/perles/internal/sessions/domain"
)

// DurableRegistry implements Registry with SQLite-backed persistence for durable fields
// and an in-memory map for runtime-only fields.
//
// Architecture:
//   - Session (persisted) = source of truth for identity, config, state, timestamps
//   - WorkflowInstance (runtime) = rehydratable handle with infra, servers, context
//
// The DurableRegistry synchronizes both layers: writes go to SQLite first,
// then update the in-memory runtime map.
type DurableRegistry struct {
	project     string
	sessionRepo domain.SessionRepository

	mu       sync.RWMutex
	runtimes map[WorkflowID]*WorkflowInstance // Active workflows with runtime resources
}

// NewDurableRegistry creates a new DurableRegistry.
func NewDurableRegistry(project string, sessionRepo domain.SessionRepository) *DurableRegistry {
	return &DurableRegistry{
		project:     project,
		sessionRepo: sessionRepo,
		runtimes:    make(map[WorkflowID]*WorkflowInstance),
	}
}

// Put stores a workflow instance. Returns an error if a workflow
// with the same ID already exists.
func (r *DurableRegistry) Put(inst *WorkflowInstance) error {
	if inst == nil {
		return fmt.Errorf("workflow instance cannot be nil")
	}
	if !inst.ID.IsValid() {
		return fmt.Errorf("workflow instance has invalid ID")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if already exists in runtime
	if _, exists := r.runtimes[inst.ID]; exists {
		return fmt.Errorf("workflow with ID %s already exists", inst.ID)
	}

	// Create session entity from workflow instance
	session := r.workflowToSession(inst)

	// Persist to SQLite
	if err := r.sessionRepo.Save(session); err != nil {
		return fmt.Errorf("failed to persist session: %w", err)
	}

	// Store in runtime map
	r.runtimes[inst.ID] = inst
	return nil
}

// Get retrieves a workflow by ID. Returns the workflow and true if found,
// or nil and false if not found.
//
// This first checks the runtime map for active workflows. If not found there,
// it checks SQLite for persisted sessions (which may be paused/completed workflows).
func (r *DurableRegistry) Get(id WorkflowID) (*WorkflowInstance, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check runtime first (active workflows with full resources)
	if inst, ok := r.runtimes[id]; ok {
		return inst, true
	}

	// Check SQLite for persisted session (may be paused/completed)
	session, err := r.sessionRepo.FindByGUID(r.project, string(id))
	if err != nil {
		return nil, false
	}

	// Reconstitute a minimal WorkflowInstance from session (no runtime resources)
	inst := r.sessionToWorkflow(session)
	return inst, true
}

// Update atomically modifies a workflow. The update function is called
// while holding an exclusive lock on the workflow. Returns an error if
// the workflow is not found.
func (r *DurableRegistry) Update(id WorkflowID, fn func(*WorkflowInstance)) error {
	if fn == nil {
		return fmt.Errorf("update function cannot be nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if workflow is in runtime (active/running)
	inst, inRuntime := r.runtimes[id]

	// If not in runtime, load from database
	if !inRuntime {
		existingSession, err := r.sessionRepo.FindByGUID(r.project, string(id))
		if err != nil {
			return fmt.Errorf("workflow with ID %s not found: %w", id, err)
		}
		inst = r.sessionToWorkflow(existingSession)
	}

	// Apply the update
	fn(inst)

	// Find existing session to get its ID for update
	existingSession, err := r.sessionRepo.FindByGUID(r.project, string(id))
	if err != nil {
		return fmt.Errorf("failed to find session for update: %w", err)
	}

	// Update existing session with new values
	r.updateSessionFromWorkflow(existingSession, inst)

	// Persist updated session
	if err := r.sessionRepo.Save(existingSession); err != nil {
		return fmt.Errorf("failed to persist session update: %w", err)
	}

	return nil
}

// List returns workflows matching the query. Results are sorted by
// creation time (newest first).
//
// This queries SQLite for all sessions matching the criteria, then merges
// with runtime data for active workflows.
//
// Ownership checking: For each workflow, if owner_current_pid refers to a
// dead process, we claim ownership by updating it to the current PID.
// If it refers to a live process that isn't us, IsLocked is set to true.
func (r *DurableRegistry) List(q ListQuery) []*WorkflowInstance {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Build domain filter - push as much filtering as possible to SQLite
	// Note: Archived sessions are excluded by default at the domain layer
	filter := domain.ListFilter{
		Limit:    q.Limit,
		OwnerPID: q.OwnerPID,
	}

	// Query SQLite for sessions
	sessions, err := r.sessionRepo.ListWithFilter(r.project, filter)
	if err != nil {
		return nil
	}

	currentPID := os.Getpid()

	// Convert sessions to workflow instances, merging with runtime data
	var results []*WorkflowInstance
	for _, session := range sessions {
		var inst *WorkflowInstance

		// Check if we have runtime data for this workflow
		if runtime, ok := r.runtimes[WorkflowID(session.GUID())]; ok {
			inst = runtime
		} else {
			inst = r.sessionToWorkflow(session)
		}

		// Check ownership and claim orphaned sessions
		if ownerPID := session.OwnerCurrentPID(); ownerPID != nil {
			if *ownerPID != currentPID {
				if isProcessAlive(*ownerPID) {
					// Another live process owns this workflow
					inst.IsLocked = true
				} else {
					// Owner process is dead - claim ownership
					session.SetOwnerCurrentPID(&currentPID)
					//nolint:errcheck // Best-effort update, don't fail List
					_ = r.sessionRepo.Save(session)
				}
			}
			// If ownerPID == currentPID, we already own it, IsLocked stays false
		}

		// Apply remaining query filters (state, labels, template)
		if !r.matchesQuery(inst, &q) {
			continue
		}

		results = append(results, inst)
	}

	// Results are already sorted by created_at desc from SQLite
	// Apply offset
	if q.Offset > 0 {
		if q.Offset >= len(results) {
			return nil
		}
		results = results[q.Offset:]
	}

	return results
}

// Remove deletes a workflow from the registry. Returns an error if
// the workflow is not found.
func (r *DurableRegistry) Remove(id WorkflowID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Remove from runtime
	if _, ok := r.runtimes[id]; !ok {
		return fmt.Errorf("workflow with ID %s not found", id)
	}
	delete(r.runtimes, id)

	// Soft delete in SQLite
	//nolint:staticcheck // SA9003: Intentionally ignoring error - session may not exist in DB
	if err := r.sessionRepo.Delete(r.project, string(id)); err != nil {
		// Log but don't fail if session doesn't exist in DB
		// (could be a workflow that was never persisted)
	}

	return nil
}

// Count returns the number of workflows in each state.
func (r *DurableRegistry) Count() map[WorkflowState]int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Query all sessions from SQLite
	sessions, err := r.sessionRepo.ListWithFilter(r.project, domain.ListFilter{})
	if err != nil {
		return make(map[WorkflowState]int)
	}

	counts := make(map[WorkflowState]int)
	for _, session := range sessions {
		state := sessionStateToWorkflowState(session.State())
		counts[state]++
	}
	return counts
}

// AttachRuntime attaches runtime resources to an existing workflow.
// This is used when resuming a paused workflow.
func (r *DurableRegistry) AttachRuntime(inst *WorkflowInstance) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.runtimes[inst.ID]; exists {
		return fmt.Errorf("workflow %s already has runtime attached", inst.ID)
	}

	r.runtimes[inst.ID] = inst
	return nil
}

// DetachRuntime removes runtime resources from a workflow without deleting it.
// This is used when pausing a workflow.
func (r *DurableRegistry) DetachRuntime(id WorkflowID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.runtimes, id)
}

// HasRuntime returns true if the workflow has runtime resources attached.
func (r *DurableRegistry) HasRuntime(id WorkflowID) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.runtimes[id]
	return ok
}

// Archive marks a workflow as archived. Archived workflows are excluded
// from List queries by default. Returns an error if the workflow is not found.
func (r *DurableRegistry) Archive(id WorkflowID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Find the session in SQLite
	session, err := r.sessionRepo.FindByGUID(r.project, string(id))
	if err != nil {
		return fmt.Errorf("workflow with ID %s not found: %w", id, err)
	}

	// Archive the session
	session.Archive()

	// Persist the update
	if err := r.sessionRepo.Save(session); err != nil {
		return fmt.Errorf("failed to persist archive: %w", err)
	}

	// Remove from runtime if present (archived workflows shouldn't be active)
	delete(r.runtimes, id)

	return nil
}

// workflowToSession converts a WorkflowInstance to a NEW Session entity.
// Used only for initial creation (Put). For updates, use updateSessionFromWorkflow.
func (r *DurableRegistry) workflowToSession(inst *WorkflowInstance) *domain.Session {
	session := domain.NewSession(string(inst.ID), r.project, workflowStateToSessionState(inst.State))

	// Set owner PIDs - the current process owns this session
	pid := os.Getpid()
	session.SetOwnerCreatedPID(&pid)
	session.SetOwnerCurrentPID(&pid)

	r.updateSessionFromWorkflow(session, inst)
	return session
}

// updateSessionFromWorkflow updates an existing Session entity with values from WorkflowInstance.
// This preserves the session's ID for proper UPDATE operations.
func (r *DurableRegistry) updateSessionFromWorkflow(session *domain.Session, inst *WorkflowInstance) {
	session.SetName(inst.Name)
	session.SetTemplateID(inst.TemplateID)
	session.SetEpicID(inst.EpicID)
	session.SetWorkDir(inst.WorkDir)
	session.SetLabels(inst.Labels)
	session.SetWorktreeEnabled(inst.WorktreeEnabled)
	session.SetWorktreeBaseBranch(inst.WorktreeBaseBranch)
	session.SetWorktreeBranchName(inst.WorktreeBranchName)
	session.SetWorktreePath(inst.WorktreePath)
	session.SetWorktreeBranch(inst.WorktreeBranch)
	session.SetSessionDir(inst.SessionDir)
	session.SetTokensUsed(inst.TokensUsed)
	session.SetActiveWorkers(inst.ActiveWorkers)

	// Handle state transitions
	switch inst.State {
	case WorkflowRunning:
		if session.State() != domain.SessionStateRunning {
			session.Start()
		}
	case WorkflowPaused:
		if session.State() != domain.SessionStatePaused {
			session.Pause()
		}
	case WorkflowCompleted:
		session.MarkCompleted()
	case WorkflowFailed:
		session.MarkFailed()
	}
}

// sessionToWorkflow converts a Session entity to a WorkflowInstance.
// Note: This creates a workflow without runtime resources (Infrastructure, HTTPServer, etc.)
func (r *DurableRegistry) sessionToWorkflow(session *domain.Session) *WorkflowInstance {
	inst := &WorkflowInstance{
		ID:                 WorkflowID(session.GUID()),
		TemplateID:         session.TemplateID(),
		Name:               session.Name(),
		WorkDir:            session.WorkDir(),
		EpicID:             session.EpicID(),
		WorktreeEnabled:    session.WorktreeEnabled(),
		WorktreeBaseBranch: session.WorktreeBaseBranch(),
		WorktreeBranchName: session.WorktreeBranchName(),
		WorktreePath:       session.WorktreePath(),
		WorktreeBranch:     session.WorktreeBranch(),
		SessionDir:         session.SessionDir(),
		State:              sessionStateToWorkflowState(session.State()),
		Labels:             session.Labels(),
		CreatedAt:          session.CreatedAt(),
		StartedAt:          session.StartedAt(),
		CompletedAt:        session.CompletedAt(),
		UpdatedAt:          session.UpdatedAt(),
		TokensUsed:         session.TokensUsed(),
		ActiveWorkers:      session.ActiveWorkers(),
	}

	// Copy labels to avoid external mutation
	if session.Labels() != nil {
		inst.Labels = make(map[string]string, len(session.Labels()))
		maps.Copy(inst.Labels, session.Labels())
	}

	// Handle PausedAt (Session uses *time.Time, WorkflowInstance uses time.Time)
	if session.PausedAt() != nil {
		inst.PausedAt = *session.PausedAt()
	}

	// Handle health timestamps
	if session.LastHeartbeatAt() != nil {
		inst.LastHeartbeatAt = *session.LastHeartbeatAt()
	}
	if session.LastProgressAt() != nil {
		inst.LastProgressAt = *session.LastProgressAt()
	}

	return inst
}

// matchesQuery checks if a workflow matches the given query filters.
func (r *DurableRegistry) matchesQuery(inst *WorkflowInstance, q *ListQuery) bool {
	// Check state filter
	if len(q.States) > 0 {
		if !slices.Contains(q.States, inst.State) {
			return false
		}
	}

	// Check labels filter (must match ALL specified labels)
	if len(q.Labels) > 0 {
		for key, value := range q.Labels {
			if inst.Labels[key] != value {
				return false
			}
		}
	}

	// Check template filter
	if q.TemplateID != "" {
		if inst.TemplateID != q.TemplateID {
			return false
		}
	}

	return true
}

// workflowStateToSessionState converts WorkflowState to SessionState.
func workflowStateToSessionState(state WorkflowState) domain.SessionState {
	switch state {
	case WorkflowPending:
		return domain.SessionStatePending
	case WorkflowRunning:
		return domain.SessionStateRunning
	case WorkflowPaused:
		return domain.SessionStatePaused
	case WorkflowCompleted:
		return domain.SessionStateCompleted
	case WorkflowFailed:
		return domain.SessionStateFailed
	default:
		return domain.SessionStatePending
	}
}

// sessionStateToWorkflowState converts SessionState to WorkflowState.
func sessionStateToWorkflowState(state domain.SessionState) WorkflowState {
	switch state {
	case domain.SessionStatePending:
		return WorkflowPending
	case domain.SessionStateRunning:
		return WorkflowRunning
	case domain.SessionStatePaused:
		return WorkflowPaused
	case domain.SessionStateCompleted:
		return WorkflowCompleted
	case domain.SessionStateFailed:
		return WorkflowFailed
	case domain.SessionStateTimedOut:
		return WorkflowFailed // Map timed_out to failed
	default:
		return WorkflowPending
	}
}

// Ensure DurableRegistry implements Registry interface.
var _ Registry = (*DurableRegistry)(nil)
