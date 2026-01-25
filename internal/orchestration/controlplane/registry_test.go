package controlplane

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// === Helper Functions ===

// newTestSpec creates a valid WorkflowSpec for testing.
func newTestSpec(name string) *WorkflowSpec {
	return &WorkflowSpec{
		TemplateID:    "test-template",
		InitialPrompt: "Test goal",
		Name:          name,
	}
}

// newTestInstance creates a WorkflowInstance for testing.
func newTestInstance(t *testing.T, name string) *WorkflowInstance {
	t.Helper()
	inst, err := NewWorkflowInstance(newTestSpec(name))
	require.NoError(t, err)
	return inst
}

// === Unit Tests: Put ===

func TestRegistry_Put_StoresWorkflow(t *testing.T) {
	registry := NewInMemoryRegistry()
	inst := newTestInstance(t, "test-workflow")

	err := registry.Put(inst)
	require.NoError(t, err)

	// Verify workflow can be retrieved
	retrieved, found := registry.Get(inst.ID)
	require.True(t, found)
	require.Equal(t, inst.ID, retrieved.ID)
	require.Equal(t, inst.Name, retrieved.Name)
}

func TestRegistry_Put_RejectsDuplicate(t *testing.T) {
	registry := NewInMemoryRegistry()
	inst := newTestInstance(t, "test-workflow")

	err := registry.Put(inst)
	require.NoError(t, err)

	// Try to put the same workflow again
	err = registry.Put(inst)
	require.Error(t, err)
	require.Contains(t, err.Error(), "already exists")
}

func TestRegistry_Put_RejectsNil(t *testing.T) {
	registry := NewInMemoryRegistry()

	err := registry.Put(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot be nil")
}

func TestRegistry_Put_RejectsInvalidID(t *testing.T) {
	registry := NewInMemoryRegistry()
	inst := newTestInstance(t, "test-workflow")
	inst.ID = WorkflowID("invalid-id") // Invalid UUID format

	err := registry.Put(inst)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid ID")
}

// === Unit Tests: Get ===

func TestRegistry_Get_RetrievesWorkflow(t *testing.T) {
	registry := NewInMemoryRegistry()
	inst := newTestInstance(t, "test-workflow")

	require.NoError(t, registry.Put(inst))

	retrieved, found := registry.Get(inst.ID)
	require.True(t, found)
	require.Equal(t, inst, retrieved)
}

func TestRegistry_Get_ReturnsFalseForMissing(t *testing.T) {
	registry := NewInMemoryRegistry()

	retrieved, found := registry.Get(NewWorkflowID())
	require.False(t, found)
	require.Nil(t, retrieved)
}

// === Unit Tests: Update ===

func TestRegistry_Update_ModifiesWorkflow(t *testing.T) {
	registry := NewInMemoryRegistry()
	inst := newTestInstance(t, "test-workflow")
	require.NoError(t, registry.Put(inst))

	err := registry.Update(inst.ID, func(w *WorkflowInstance) {
		w.Name = "updated-name"
		w.State = WorkflowPaused
	})
	require.NoError(t, err)

	// Verify changes persisted
	retrieved, found := registry.Get(inst.ID)
	require.True(t, found)
	require.Equal(t, "updated-name", retrieved.Name)
	require.Equal(t, WorkflowPaused, retrieved.State)
}

func TestRegistry_Update_ReturnsErrorForMissing(t *testing.T) {
	registry := NewInMemoryRegistry()

	err := registry.Update(NewWorkflowID(), func(w *WorkflowInstance) {
		w.Name = "updated"
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

func TestRegistry_Update_RejectsNilFunction(t *testing.T) {
	registry := NewInMemoryRegistry()
	inst := newTestInstance(t, "test-workflow")
	require.NoError(t, registry.Put(inst))

	err := registry.Update(inst.ID, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot be nil")
}

// === Unit Tests: Remove ===

func TestRegistry_Remove_DeletesWorkflow(t *testing.T) {
	registry := NewInMemoryRegistry()
	inst := newTestInstance(t, "test-workflow")
	require.NoError(t, registry.Put(inst))

	err := registry.Remove(inst.ID)
	require.NoError(t, err)

	// Verify workflow is gone
	_, found := registry.Get(inst.ID)
	require.False(t, found)
}

func TestRegistry_Remove_ReturnsErrorForMissing(t *testing.T) {
	registry := NewInMemoryRegistry()

	err := registry.Remove(NewWorkflowID())
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

// === Unit Tests: List ===

func TestRegistry_List_ReturnsAllWithEmptyQuery(t *testing.T) {
	registry := NewInMemoryRegistry()

	inst1 := newTestInstance(t, "workflow-1")
	inst2 := newTestInstance(t, "workflow-2")
	inst3 := newTestInstance(t, "workflow-3")

	require.NoError(t, registry.Put(inst1))
	require.NoError(t, registry.Put(inst2))
	require.NoError(t, registry.Put(inst3))

	results := registry.List(ListQuery{})
	require.Len(t, results, 3)

	// Verify all workflows are returned (order is deterministic but depends on
	// CreatedAt and ID; we just verify all three are present and order is stable)
	ids := make(map[WorkflowID]bool)
	for _, r := range results {
		ids[r.ID] = true
	}
	require.True(t, ids[inst1.ID])
	require.True(t, ids[inst2.ID])
	require.True(t, ids[inst3.ID])

	// Verify ordering is stable (calling List again returns same order)
	results2 := registry.List(ListQuery{})
	require.Equal(t, results[0].ID, results2[0].ID)
	require.Equal(t, results[1].ID, results2[1].ID)
	require.Equal(t, results[2].ID, results2[2].ID)
}

func TestRegistry_List_FiltersByState(t *testing.T) {
	registry := NewInMemoryRegistry()

	running := newTestInstance(t, "running")
	require.NoError(t, running.TransitionTo(WorkflowRunning))

	paused := newTestInstance(t, "paused")
	require.NoError(t, paused.TransitionTo(WorkflowRunning))
	require.NoError(t, paused.TransitionTo(WorkflowPaused))

	pending := newTestInstance(t, "pending")

	require.NoError(t, registry.Put(running))
	require.NoError(t, registry.Put(paused))
	require.NoError(t, registry.Put(pending))

	// Filter by Running state only
	results := registry.List(ListQuery{States: []WorkflowState{WorkflowRunning}})
	require.Len(t, results, 1)
	require.Equal(t, running.ID, results[0].ID)

	// Filter by multiple states
	results = registry.List(ListQuery{States: []WorkflowState{WorkflowRunning, WorkflowPaused}})
	require.Len(t, results, 2)
}

func TestRegistry_List_FiltersByLabels(t *testing.T) {
	registry := NewInMemoryRegistry()

	inst1 := newTestInstance(t, "inst-1")
	inst1.Labels = map[string]string{"team": "platform", "env": "prod"}

	inst2 := newTestInstance(t, "inst-2")
	inst2.Labels = map[string]string{"team": "platform", "env": "dev"}

	inst3 := newTestInstance(t, "inst-3")
	inst3.Labels = map[string]string{"team": "frontend"}

	require.NoError(t, registry.Put(inst1))
	require.NoError(t, registry.Put(inst2))
	require.NoError(t, registry.Put(inst3))

	// Filter by team=platform
	results := registry.List(ListQuery{Labels: map[string]string{"team": "platform"}})
	require.Len(t, results, 2)

	// Filter by team=platform AND env=prod (must match all)
	results = registry.List(ListQuery{Labels: map[string]string{"team": "platform", "env": "prod"}})
	require.Len(t, results, 1)
	require.Equal(t, inst1.ID, results[0].ID)

	// Filter by non-existent label value
	results = registry.List(ListQuery{Labels: map[string]string{"team": "backend"}})
	require.Len(t, results, 0)
}

func TestRegistry_List_FiltersByTemplateID(t *testing.T) {
	registry := NewInMemoryRegistry()

	inst1 := newTestInstance(t, "inst-1")
	inst1.TemplateID = "cook.md"

	inst2 := newTestInstance(t, "inst-2")
	inst2.TemplateID = "research.md"

	inst3 := newTestInstance(t, "inst-3")
	inst3.TemplateID = "cook.md"

	require.NoError(t, registry.Put(inst1))
	require.NoError(t, registry.Put(inst2))
	require.NoError(t, registry.Put(inst3))

	results := registry.List(ListQuery{TemplateID: "cook.md"})
	require.Len(t, results, 2)

	for _, r := range results {
		require.Equal(t, "cook.md", r.TemplateID)
	}
}

func TestRegistry_List_PaginationWithLimitOffset(t *testing.T) {
	registry := NewInMemoryRegistry()

	// Create 10 workflows
	var workflows []*WorkflowInstance
	for i := 0; i < 10; i++ {
		inst := newTestInstance(t, fmt.Sprintf("workflow-%d", i))
		workflows = append(workflows, inst)
		require.NoError(t, registry.Put(inst))
		time.Sleep(time.Millisecond) // Ensure different CreatedAt
	}

	// Test Limit
	results := registry.List(ListQuery{Limit: 3})
	require.Len(t, results, 3)

	// Test Offset
	results = registry.List(ListQuery{Offset: 5})
	require.Len(t, results, 5) // 10 - 5 offset = 5 remaining

	// Test Limit + Offset
	results = registry.List(ListQuery{Limit: 3, Offset: 2})
	require.Len(t, results, 3)

	// Test offset beyond results
	results = registry.List(ListQuery{Offset: 100})
	require.Len(t, results, 0)

	// Test limit larger than results
	results = registry.List(ListQuery{Limit: 100})
	require.Len(t, results, 10)
}

func TestRegistry_List_SortsNewestFirst(t *testing.T) {
	registry := NewInMemoryRegistry()

	// Create workflows with explicit CreatedAt times
	now := time.Now()
	oldest := newTestInstance(t, "oldest")
	oldest.CreatedAt = now.Add(-2 * time.Hour)

	middle := newTestInstance(t, "middle")
	middle.CreatedAt = now.Add(-1 * time.Hour)

	newest := newTestInstance(t, "newest")
	newest.CreatedAt = now

	// Add in random order
	require.NoError(t, registry.Put(middle))
	require.NoError(t, registry.Put(oldest))
	require.NoError(t, registry.Put(newest))

	// List should return newest first
	results := registry.List(ListQuery{})
	require.Len(t, results, 3)
	require.Equal(t, "newest", results[0].Name, "newest workflow should be first")
	require.Equal(t, "middle", results[1].Name, "middle workflow should be second")
	require.Equal(t, "oldest", results[2].Name, "oldest workflow should be last")
}

func TestRegistry_List_CombinedFilters(t *testing.T) {
	registry := NewInMemoryRegistry()

	// Create workflows with various attributes
	for i := 0; i < 10; i++ {
		inst := newTestInstance(t, fmt.Sprintf("workflow-%d", i))
		inst.TemplateID = "cook.md"
		if i < 5 {
			require.NoError(t, inst.TransitionTo(WorkflowRunning))
		}
		inst.Labels = map[string]string{"index": fmt.Sprintf("%d", i)}
		require.NoError(t, registry.Put(inst))
	}

	// Filter: Running workflows
	results := registry.List(ListQuery{
		States: []WorkflowState{WorkflowRunning},
	})

	// Running workflows: 0-4 (5 workflows)
	require.Len(t, results, 5)
	for _, r := range results {
		require.Equal(t, WorkflowRunning, r.State)
	}
}

// === Unit Tests: Count ===

func TestRegistry_Count_ReturnsCorrectCounts(t *testing.T) {
	registry := NewInMemoryRegistry()

	// Create workflows in various states
	pending := newTestInstance(t, "pending")

	running1 := newTestInstance(t, "running-1")
	require.NoError(t, running1.TransitionTo(WorkflowRunning))

	running2 := newTestInstance(t, "running-2")
	require.NoError(t, running2.TransitionTo(WorkflowRunning))

	paused := newTestInstance(t, "paused")
	require.NoError(t, paused.TransitionTo(WorkflowRunning))
	require.NoError(t, paused.TransitionTo(WorkflowPaused))

	completed := newTestInstance(t, "completed")
	require.NoError(t, completed.TransitionTo(WorkflowRunning))
	require.NoError(t, completed.TransitionTo(WorkflowCompleted))

	require.NoError(t, registry.Put(pending))
	require.NoError(t, registry.Put(running1))
	require.NoError(t, registry.Put(running2))
	require.NoError(t, registry.Put(paused))
	require.NoError(t, registry.Put(completed))

	counts := registry.Count()

	require.Equal(t, 1, counts[WorkflowPending])
	require.Equal(t, 2, counts[WorkflowRunning])
	require.Equal(t, 1, counts[WorkflowPaused])
	require.Equal(t, 1, counts[WorkflowCompleted])
	require.Equal(t, 0, counts[WorkflowFailed])
}

func TestRegistry_Count_EmptyRegistry(t *testing.T) {
	registry := NewInMemoryRegistry()

	counts := registry.Count()
	require.Empty(t, counts)
}

// === Concurrency Tests ===

func TestRegistry_Concurrent_PutGetUpdate(t *testing.T) {
	registry := NewInMemoryRegistry()
	const numGoroutines = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 3) // Put + Get + Update operations

	// Store workflow IDs for concurrent access
	ids := make([]WorkflowID, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		inst := newTestInstance(t, fmt.Sprintf("workflow-%d", i))
		ids[i] = inst.ID
		require.NoError(t, registry.Put(inst))
	}

	// Concurrent Gets
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			_, found := registry.Get(ids[idx])
			require.True(t, found)
		}(i)
	}

	// Concurrent Updates
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			err := registry.Update(ids[idx], func(w *WorkflowInstance) {
				w.ActiveWorkers++
			})
			require.NoError(t, err)
		}(i)
	}

	// Concurrent List operations
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			results := registry.List(ListQuery{})
			require.Len(t, results, numGoroutines)
		}()
	}

	wg.Wait()
}

func TestRegistry_Concurrent_PutRemove(t *testing.T) {
	registry := NewInMemoryRegistry()
	const numIterations = 50

	var wg sync.WaitGroup
	errors := make(chan error, numIterations*2)

	// Half the goroutines put, half remove
	for i := 0; i < numIterations; i++ {
		wg.Add(2)

		inst := newTestInstance(t, fmt.Sprintf("workflow-%d", i))
		id := inst.ID

		// Put goroutine
		go func() {
			defer wg.Done()
			if err := registry.Put(inst); err != nil {
				errors <- err
			}
		}()

		// Remove goroutine (may run before or after Put)
		go func() {
			defer wg.Done()
			// This may fail if Put hasn't happened yet, which is expected
			_ = registry.Remove(id)
		}()
	}

	wg.Wait()
	close(errors)

	// Some puts may fail due to race with remove, but no panics should occur
	// The important thing is thread safety, not the specific outcomes
}

func TestRegistry_Concurrent_CountDuringMutations(t *testing.T) {
	registry := NewInMemoryRegistry()
	const numGoroutines = 50

	// Pre-populate some data
	for i := 0; i < 20; i++ {
		inst := newTestInstance(t, fmt.Sprintf("initial-%d", i))
		require.NoError(t, registry.Put(inst))
	}

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2)

	// Concurrent mutations
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			inst := newTestInstance(t, fmt.Sprintf("concurrent-%d", idx))
			_ = registry.Put(inst)
		}(i)
	}

	// Concurrent Count operations
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			counts := registry.Count()
			// Should not panic and should return valid counts
			total := 0
			for _, c := range counts {
				require.GreaterOrEqual(t, c, 0)
				total += c
			}
			require.GreaterOrEqual(t, total, 0)
		}()
	}

	wg.Wait()
}

// === Property-Based Tests ===

func TestRegistry_PropertyBased_CRUDConsistency(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		registry := NewInMemoryRegistry()

		// Track what we've done for verification
		added := make(map[WorkflowID]bool)
		removed := make(map[WorkflowID]bool)

		// Generate a sequence of operations
		numOps := rapid.IntRange(1, 100).Draw(t, "numOps")

		for i := 0; i < numOps; i++ {
			op := rapid.IntRange(0, 2).Draw(t, "op")

			switch op {
			case 0: // Put
				spec := &WorkflowSpec{
					TemplateID:    rapid.StringMatching(`[a-z]{3,10}\.md`).Draw(t, "templateID"),
					InitialPrompt: rapid.StringMatching(`[a-zA-Z ]{5,50}`).Draw(t, "goal"),
				}
				inst, err := NewWorkflowInstance(spec)
				if err != nil {
					continue
				}

				err = registry.Put(inst)
				if err == nil {
					added[inst.ID] = true
				}

			case 1: // Remove
				// Pick a random ID from added ones
				for id := range added {
					if !removed[id] {
						err := registry.Remove(id)
						if err == nil {
							removed[id] = true
						}
						break
					}
				}

			case 2: // List and verify
				results := registry.List(ListQuery{})

				// Count should match non-removed workflows
				expectedCount := 0
				for id := range added {
					if !removed[id] {
						expectedCount++
					}
				}

				if len(results) != expectedCount {
					t.Fatalf("expected %d workflows, got %d", expectedCount, len(results))
				}
			}
		}

		// Final verification: all non-removed workflows should be gettable
		for id := range added {
			if !removed[id] {
				_, found := registry.Get(id)
				if !found {
					t.Fatalf("workflow %s should exist but was not found", id)
				}
			}
		}

		// All removed workflows should not be gettable
		for id := range removed {
			_, found := registry.Get(id)
			if found {
				t.Fatalf("workflow %s should not exist but was found", id)
			}
		}
	})
}

func TestRegistry_PropertyBased_UpdateNeverLosesWorkflow(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		registry := NewInMemoryRegistry()

		// Add some workflows
		numWorkflows := rapid.IntRange(1, 20).Draw(t, "numWorkflows")
		ids := make([]WorkflowID, numWorkflows)

		for i := 0; i < numWorkflows; i++ {
			spec := &WorkflowSpec{
				TemplateID:    "test.md",
				InitialPrompt: "goal",
			}
			inst, err := NewWorkflowInstance(spec)
			if err != nil {
				t.Fatal(err)
			}
			ids[i] = inst.ID
			if err := registry.Put(inst); err != nil {
				t.Fatal(err)
			}
		}

		// Perform many updates
		numUpdates := rapid.IntRange(10, 100).Draw(t, "numUpdates")
		for i := 0; i < numUpdates; i++ {
			idx := rapid.IntRange(0, numWorkflows-1).Draw(t, "idx")
			err := registry.Update(ids[idx], func(w *WorkflowInstance) {
				w.ActiveWorkers++
			})
			if err != nil {
				t.Fatalf("update failed: %v", err)
			}
		}

		// Verify all workflows still exist
		for _, id := range ids {
			_, found := registry.Get(id)
			if !found {
				t.Fatalf("workflow %s lost after updates", id)
			}
		}

		// Verify count is correct
		results := registry.List(ListQuery{})
		if len(results) != numWorkflows {
			t.Fatalf("expected %d workflows, got %d", numWorkflows, len(results))
		}
	})
}

func TestRegistry_PropertyBased_FilteringIsComplete(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		registry := NewInMemoryRegistry()

		// Create workflows with random states
		numWorkflows := rapid.IntRange(5, 30).Draw(t, "numWorkflows")
		stateCount := make(map[WorkflowState]int)

		states := []WorkflowState{WorkflowPending, WorkflowRunning, WorkflowPaused, WorkflowCompleted}

		for i := 0; i < numWorkflows; i++ {
			spec := &WorkflowSpec{
				TemplateID:    "test.md",
				InitialPrompt: "goal",
			}
			inst, err := NewWorkflowInstance(spec)
			if err != nil {
				t.Fatal(err)
			}

			// Randomly transition to a state
			targetState := states[rapid.IntRange(0, len(states)-1).Draw(t, "state")]
			switch targetState {
			case WorkflowRunning:
				_ = inst.TransitionTo(WorkflowRunning)
			case WorkflowPaused:
				_ = inst.TransitionTo(WorkflowRunning)
				_ = inst.TransitionTo(WorkflowPaused)
			case WorkflowCompleted:
				_ = inst.TransitionTo(WorkflowRunning)
				_ = inst.TransitionTo(WorkflowCompleted)
			}

			stateCount[inst.State]++
			if err := registry.Put(inst); err != nil {
				t.Fatal(err)
			}
		}

		// Verify filtering by each state returns correct count
		for state, expectedCount := range stateCount {
			results := registry.List(ListQuery{States: []WorkflowState{state}})
			if len(results) != expectedCount {
				t.Fatalf("expected %d workflows in state %s, got %d", expectedCount, state, len(results))
			}
		}

		// Verify Count() matches
		counts := registry.Count()
		for state, expectedCount := range stateCount {
			if counts[state] != expectedCount {
				t.Fatalf("Count() reports %d for state %s, expected %d", counts[state], state, expectedCount)
			}
		}
	})
}
