// Package controlplane provides Registry for storing and querying workflow instances.
package controlplane

import (
	"fmt"
	"slices"
	"sync"
)

// ListQuery filters workflows for listing.
type ListQuery struct {
	// States filters by workflow state(s). If empty, all states are included.
	States []WorkflowState

	// Labels filters by labels (match all). If empty, no label filtering is applied.
	Labels map[string]string

	// TemplateID filters by template. If empty, all templates are included.
	TemplateID string

	// Limit is the maximum number of results to return. 0 means no limit.
	Limit int

	// Offset is the number of results to skip. Used for pagination.
	Offset int
}

// Registry stores and queries workflow instances.
// Implementations must be thread-safe for concurrent access.
type Registry interface {
	// Put stores a workflow instance. Returns an error if a workflow
	// with the same ID already exists.
	Put(inst *WorkflowInstance) error

	// Get retrieves a workflow by ID. Returns the workflow and true if found,
	// or nil and false if not found.
	Get(id WorkflowID) (*WorkflowInstance, bool)

	// Update atomically modifies a workflow. The update function is called
	// while holding an exclusive lock on the workflow. Returns an error if
	// the workflow is not found.
	Update(id WorkflowID, fn func(*WorkflowInstance)) error

	// List returns workflows matching the query. Results are sorted by
	// creation time (newest first).
	List(q ListQuery) []*WorkflowInstance

	// Remove deletes a workflow from the registry. Returns an error if
	// the workflow is not found.
	Remove(id WorkflowID) error

	// Count returns the number of workflows in each state.
	Count() map[WorkflowState]int
}

// inMemoryRegistry is a thread-safe in-memory implementation of Registry.
type inMemoryRegistry struct {
	mu        sync.RWMutex
	workflows map[WorkflowID]*WorkflowInstance
}

// NewInMemoryRegistry creates a new in-memory Registry.
func NewInMemoryRegistry() Registry {
	return &inMemoryRegistry{
		workflows: make(map[WorkflowID]*WorkflowInstance),
	}
}

// Put stores a workflow instance.
func (r *inMemoryRegistry) Put(inst *WorkflowInstance) error {
	if inst == nil {
		return fmt.Errorf("workflow instance cannot be nil")
	}
	if !inst.ID.IsValid() {
		return fmt.Errorf("workflow instance has invalid ID")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.workflows[inst.ID]; exists {
		return fmt.Errorf("workflow with ID %s already exists", inst.ID)
	}

	r.workflows[inst.ID] = inst
	return nil
}

// Get retrieves a workflow by ID.
func (r *inMemoryRegistry) Get(id WorkflowID) (*WorkflowInstance, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	inst, ok := r.workflows[id]
	return inst, ok
}

// Update atomically modifies a workflow.
func (r *inMemoryRegistry) Update(id WorkflowID, fn func(*WorkflowInstance)) error {
	if fn == nil {
		return fmt.Errorf("update function cannot be nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	inst, ok := r.workflows[id]
	if !ok {
		return fmt.Errorf("workflow with ID %s not found", id)
	}

	fn(inst)
	return nil
}

// List returns workflows matching the query.
func (r *inMemoryRegistry) List(q ListQuery) []*WorkflowInstance {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Collect matching workflows
	var results []*WorkflowInstance
	for _, inst := range r.workflows {
		if r.matchesQuery(inst, &q) {
			results = append(results, inst)
		}
	}

	// Sort by creation time (newest first, so new workflows appear at top)
	sortByCreatedAtDesc(results)

	// Apply offset and limit
	if q.Offset > 0 {
		if q.Offset >= len(results) {
			return nil
		}
		results = results[q.Offset:]
	}
	if q.Limit > 0 && len(results) > q.Limit {
		results = results[:q.Limit]
	}

	return results
}

// Remove deletes a workflow from the registry.
func (r *inMemoryRegistry) Remove(id WorkflowID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.workflows[id]; !ok {
		return fmt.Errorf("workflow with ID %s not found", id)
	}

	delete(r.workflows, id)
	return nil
}

// Count returns the number of workflows in each state.
func (r *inMemoryRegistry) Count() map[WorkflowState]int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	counts := make(map[WorkflowState]int)
	for _, inst := range r.workflows {
		counts[inst.State]++
	}
	return counts
}

// matchesQuery checks if a workflow matches the given query filters.
func (r *inMemoryRegistry) matchesQuery(inst *WorkflowInstance, q *ListQuery) bool {
	// Check state filter
	if len(q.States) > 0 {
		if !containsState(q.States, inst.State) {
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

// containsState checks if a state is in the slice.
func containsState(states []WorkflowState, target WorkflowState) bool {
	return slices.Contains(states, target)
}

// sortByCreatedAtDesc sorts workflows by CreatedAt in descending order (newest first).
// When CreatedAt times are equal, sorts by ID descending for stable ordering.
func sortByCreatedAtDesc(workflows []*WorkflowInstance) {
	// Simple insertion sort - adequate for expected list sizes
	for i := 1; i < len(workflows); i++ {
		for j := i; j > 0 && isNewerOrSameTimeWithLargerID(workflows[j], workflows[j-1]); j-- {
			workflows[j], workflows[j-1] = workflows[j-1], workflows[j]
		}
	}
}

// isNewerOrSameTimeWithLargerID returns true if a should sort before b.
// Primary sort: CreatedAt descending (newer first).
// Secondary sort (tie-breaker): ID descending for stable ordering.
func isNewerOrSameTimeWithLargerID(a, b *WorkflowInstance) bool {
	if a.CreatedAt.After(b.CreatedAt) {
		return true
	}
	if a.CreatedAt.Equal(b.CreatedAt) {
		return string(a.ID) > string(b.ID)
	}
	return false
}
