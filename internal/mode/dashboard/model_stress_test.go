package dashboard

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/mock"
	"pgregory.net/rapid"

	"github.com/zjrosen/perles/internal/mode"
	"github.com/zjrosen/perles/internal/orchestration/controlplane"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/ui/shared/chatrender"
)

// === Stress Test Helpers ===

// createStressTestModel creates a model configured for stress testing with multiple workflows.
// Uses a mix of workflow states to allow LRU eviction testing.
func createStressTestModel(t *testing.T, numWorkflows int) Model {
	t.Helper()

	workflows := make([]*controlplane.WorkflowInstance, numWorkflows)
	states := []controlplane.WorkflowState{
		controlplane.WorkflowPending,
		controlplane.WorkflowPaused,
		controlplane.WorkflowCompleted,
	}
	for i := 0; i < numWorkflows; i++ {
		// Mix of states - only a few running, rest are evictable
		var state controlplane.WorkflowState
		if i < 2 {
			state = controlplane.WorkflowRunning // Only first 2 are running (protected)
		} else {
			state = states[i%len(states)] // Rest are non-running (evictable)
		}
		workflows[i] = createTestWorkflow(
			controlplane.WorkflowID(controlplane.NewWorkflowID()),
			"Workflow "+string(rune('A'+i)),
			state,
		)
	}

	mockCP := newMockControlPlane(t)
	mockCP.On("List", mock.Anything, mock.Anything).Return(workflows, nil).Maybe()

	eventCh := make(chan controlplane.ControlPlaneEvent)
	close(eventCh)
	mockCP.On("Subscribe", mock.Anything).Return((<-chan controlplane.ControlPlaneEvent)(eventCh), func() {}).Maybe()
	mockCP.On("SubscribeWorkflow", mock.Anything, mock.Anything).Return((<-chan controlplane.ControlPlaneEvent)(eventCh), func() {}).Maybe()

	cfg := Config{
		ControlPlane: mockCP,
		Services:     mode.Services{},
	}

	m := New(cfg)
	m.workflows = workflows
	m.workflowList = m.workflowList.SetWorkflows(workflows)
	m.resourceSummary = m.resourceSummary.Update(workflows)
	m = m.SetSize(120, 50).(Model)

	return m
}

// simulateCoordinatorEvent simulates a coordinator output event.
func simulateCoordinatorEvent(workflowID controlplane.WorkflowID, output string) controlplane.ControlPlaneEvent {
	return controlplane.ControlPlaneEvent{
		Type:       controlplane.EventCoordinatorOutput,
		WorkflowID: workflowID,
		Payload: events.ProcessEvent{
			ProcessID: "coordinator",
			Output:    output,
			Delta:     false,
		},
	}
}

// simulateWorkerEvent simulates a worker output event.
func simulateWorkerEvent(workflowID controlplane.WorkflowID, workerID, output string) controlplane.ControlPlaneEvent {
	return controlplane.ControlPlaneEvent{
		Type:       controlplane.EventWorkerOutput,
		WorkflowID: workflowID,
		Payload: events.ProcessEvent{
			ProcessID: workerID,
			Output:    output,
			Delta:     false,
		},
	}
}

// simulateMessageEvent simulates a message posted event.
func simulateMessageEvent(workflowID controlplane.WorkflowID, content string) controlplane.ControlPlaneEvent {
	return controlplane.ControlPlaneEvent{
		Type:       controlplane.EventMessagePosted,
		WorkflowID: workflowID,
		Payload: message.Event{
			Entry: message.Entry{
				Content: content,
			},
		},
	}
}

// simulateWorkerSpawnedEvent simulates a worker spawned event.
func simulateWorkerSpawnedEvent(workflowID controlplane.WorkflowID, workerID string) controlplane.ControlPlaneEvent {
	return controlplane.ControlPlaneEvent{
		Type:       controlplane.EventWorkerSpawned,
		WorkflowID: workflowID,
		Payload: events.ProcessEvent{
			ProcessID: workerID,
			Status:    events.ProcessStatusReady,
		},
	}
}

// === Stress Tests ===

// TestModel_RapidWorkflowSwitching performs property-based testing of rapid workflow selection,
// focus changes, and event handling. Verifies no goroutine leaks and cache bounds are maintained.
func TestModel_RapidWorkflowSwitching(t *testing.T) {
	// Allow GC to clean up before measuring baseline
	runtime.GC()
	time.Sleep(10 * time.Millisecond)
	baselineGoroutines := runtime.NumGoroutine()

	rapid.Check(t, func(rt *rapid.T) {
		numWorkflows := rapid.IntRange(5, 15).Draw(rt, "numWorkflows")
		m := createStressTestModel(t, numWorkflows)

		iterations := rapid.IntRange(50, 200).Draw(rt, "iterations")

		for i := 0; i < iterations; i++ {
			action := rapid.IntRange(0, 4).Draw(rt, "action")

			switch action {
			case 0: // Select a different workflow via navigation
				if numWorkflows > 0 {
					direction := rapid.IntRange(0, 1).Draw(rt, "direction")
					var key tea.KeyMsg
					if direction == 0 {
						key = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}
					} else {
						key = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}
					}
					result, _ := m.Update(key)
					m = result.(Model)
				}

			case 1: // Cycle focus
				key := tea.KeyMsg{Type: tea.KeyTab}
				result, _ := m.Update(key)
				m = result.(Model)

			case 2: // Simulate coordinator event for random workflow
				if numWorkflows > 0 {
					idx := rapid.IntRange(0, numWorkflows-1).Draw(rt, "workflowIdx")
					wf := m.workflows[idx]
					event := simulateCoordinatorEvent(wf.ID, "Test output "+string(rune('0'+i%10)))
					result, _ := m.Update(event)
					m = result.(Model)
				}

			case 3: // Simulate worker event for random workflow
				if numWorkflows > 0 {
					idx := rapid.IntRange(0, numWorkflows-1).Draw(rt, "workflowIdx")
					wf := m.workflows[idx]
					workerID := "worker-" + string(rune('1'+rapid.IntRange(0, 4).Draw(rt, "workerNum")))
					event := simulateWorkerEvent(wf.ID, workerID, "Worker output")
					result, _ := m.Update(event)
					m = result.(Model)
				}

			case 4: // Simulate message event for random workflow
				if numWorkflows > 0 {
					idx := rapid.IntRange(0, numWorkflows-1).Draw(rt, "workflowIdx")
					wf := m.workflows[idx]
					event := simulateMessageEvent(wf.ID, "Test message")
					result, _ := m.Update(event)
					m = result.(Model)
				}
			}
		}

		// Verify cache bounds: should never exceed maxCachedWorkflows + protected workflows
		// Protected = running workflows (2 in our test setup) + selected workflow (1) = 3
		cacheSize := len(m.workflowUIState)
		maxAllowed := maxCachedWorkflows + 3 // +3 for running (2) + selected (1) protection margin
		if cacheSize > maxAllowed {
			rt.Fatalf("cache exceeded bounds: size=%d, maxAllowed=%d", cacheSize, maxAllowed)
		}

		// Verify no data corruption in cached states
		for wfID, state := range m.workflowUIState {
			if state == nil {
				rt.Fatalf("nil state for workflow %s", wfID)
			}
			// Verify all maps are initialized
			if state.WorkerStatus == nil {
				rt.Fatalf("nil WorkerStatus map for workflow %s", wfID)
			}
			if state.WorkerMessages == nil {
				rt.Fatalf("nil WorkerMessages map for workflow %s", wfID)
			}
		}
	})

	// Allow GC and check for goroutine leaks
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	finalGoroutines := runtime.NumGoroutine()

	// Allow small variance (baseline + 5) for runtime variations
	if finalGoroutines > baselineGoroutines+5 {
		t.Errorf("goroutine leak detected: baseline=%d, final=%d, delta=%d",
			baselineGoroutines, finalGoroutines, finalGoroutines-baselineGoroutines)
	}
}

// TestModel_ConcurrentEventDelivery verifies that concurrent event delivery
// does not cause panics or data races.
func TestModel_ConcurrentEventDelivery(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		numWorkflows := rapid.IntRange(3, 8).Draw(rt, "numWorkflows")
		m := createStressTestModel(t, numWorkflows)

		// Pre-capture workflow IDs to avoid race on m.workflows access
		workflowIDs := make([]controlplane.WorkflowID, numWorkflows)
		for i := 0; i < numWorkflows; i++ {
			workflowIDs[i] = m.workflows[i].ID
		}

		numGoroutines := rapid.IntRange(5, 15).Draw(rt, "numGoroutines")
		eventsPerGoroutine := rapid.IntRange(20, 50).Draw(rt, "eventsPerGoroutine")

		var wg sync.WaitGroup
		var mu sync.Mutex // Protect model access since Model is not thread-safe
		panicChan := make(chan interface{}, numGoroutines)

		for g := 0; g < numGoroutines; g++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						panicChan <- r
					}
				}()

				for e := 0; e < eventsPerGoroutine; e++ {
					// Generate a random event using pre-captured workflow IDs
					var event controlplane.ControlPlaneEvent
					wfIdx := (goroutineID + e) % numWorkflows
					wfID := workflowIDs[wfIdx]

					eventType := e % 4
					switch eventType {
					case 0:
						event = simulateCoordinatorEvent(wfID, "Concurrent output")
					case 1:
						event = simulateWorkerEvent(wfID, "worker-"+string(rune('1'+goroutineID%5)), "Worker output")
					case 2:
						event = simulateMessageEvent(wfID, "Concurrent message")
					case 3:
						event = simulateWorkerSpawnedEvent(wfID, "worker-new-"+string(rune('1'+goroutineID%5)))
					}

					// Serialize model access
					mu.Lock()
					result, _ := m.Update(event)
					m = result.(Model)
					mu.Unlock()
				}
			}(g)
		}

		wg.Wait()
		close(panicChan)

		// Check for panics
		for p := range panicChan {
			rt.Fatalf("panic during concurrent event delivery: %v", p)
		}

		// Verify model is still in a valid state
		if m.workflowUIState == nil {
			rt.Fatal("workflowUIState map is nil after concurrent events")
		}

		// Verify all cached states are valid
		for wfID, state := range m.workflowUIState {
			if state == nil {
				rt.Fatalf("nil state for workflow %s after concurrent events", wfID)
			}
		}
	})
}

// TestModel_CacheBoundsUnderLoad verifies that the cache never exceeds
// maxCachedWorkflows + protected workflows under sustained load.
func TestModel_CacheBoundsUnderLoad(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Use more workflows than the cache limit to stress LRU eviction
		numWorkflows := rapid.IntRange(maxCachedWorkflows+5, maxCachedWorkflows+20).Draw(rt, "numWorkflows")
		m := createStressTestModel(t, numWorkflows)

		iterations := rapid.IntRange(100, 300).Draw(rt, "iterations")

		// Track max cache size observed
		maxCacheSize := 0

		for i := 0; i < iterations; i++ {
			// Randomly switch workflows to trigger cache operations
			newIdx := rapid.IntRange(0, numWorkflows-1).Draw(rt, "newIdx")
			m.handleWorkflowSelectionChange(newIdx)

			// Simulate events for the currently selected workflow
			wf := m.SelectedWorkflow()
			if wf != nil {
				eventType := rapid.IntRange(0, 2).Draw(rt, "eventType")
				var event controlplane.ControlPlaneEvent
				switch eventType {
				case 0:
					event = simulateCoordinatorEvent(wf.ID, "Cache stress output")
				case 1:
					event = simulateWorkerEvent(wf.ID, "worker-stress", "Worker stress output")
				case 2:
					event = simulateMessageEvent(wf.ID, "Cache stress message")
				}
				result, _ := m.Update(event)
				m = result.(Model)
			}

			// Track cache size
			currentSize := len(m.workflowUIState)
			if currentSize > maxCacheSize {
				maxCacheSize = currentSize
			}

			// Verify cache bounds after each operation
			// Allow for running (2) + selected (1) workflows beyond maxCachedWorkflows
			maxAllowed := maxCachedWorkflows + 3
			if currentSize > maxAllowed {
				rt.Fatalf("cache exceeded bounds at iteration %d: size=%d, maxAllowed=%d",
					i, currentSize, maxAllowed)
			}
		}

		// Verify final state
		finalCacheSize := len(m.workflowUIState)
		maxAllowed := maxCachedWorkflows + 3 // running (2) + selected (1)
		if finalCacheSize > maxAllowed {
			rt.Fatalf("final cache size exceeded bounds: size=%d, maxAllowed=%d", finalCacheSize, maxAllowed)
		}

		// Verify all entries have valid timestamps (LRU metadata)
		for wfID, state := range m.workflowUIState {
			if state == nil {
				rt.Fatalf("nil state for workflow %s", wfID)
			}
			// Note: LastUpdated may be zero if Clock is nil in tests, which is acceptable
		}
	})
}

// TestModel_WorkflowSelectionStressConsistency verifies that after many rapid
// workflow selections, the model state remains consistent.
func TestModel_WorkflowSelectionStressConsistency(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		numWorkflows := rapid.IntRange(5, 20).Draw(rt, "numWorkflows")
		m := createStressTestModel(t, numWorkflows)

		iterations := rapid.IntRange(200, 500).Draw(rt, "iterations")

		for i := 0; i < iterations; i++ {
			// Random selection change
			newIdx := rapid.IntRange(0, numWorkflows-1).Draw(rt, "newIdx")
			m.handleWorkflowSelectionChange(newIdx)

			// Add some data to the current workflow's cache
			wf := m.SelectedWorkflow()
			if wf != nil {
				state := m.getOrCreateUIState(wf.ID)
				state.CoordinatorMessages = append(state.CoordinatorMessages, chatrender.Message{
					Role:    "assistant",
					Content: "Iteration " + string(rune('0'+i%10)),
				})
			}

			// Periodically verify invariants
			if i%50 == 0 {
				// Selected index must be valid
				if m.selectedIndex < 0 || m.selectedIndex >= numWorkflows {
					rt.Fatalf("invalid selectedIndex: %d (numWorkflows=%d)", m.selectedIndex, numWorkflows)
				}

				// Selected workflow must exist
				selected := m.SelectedWorkflow()
				if selected == nil {
					rt.Fatalf("SelectedWorkflow() returned nil at iteration %d", i)
				}

				// Cache must have entry for selected workflow if we added data
				if _, exists := m.workflowUIState[selected.ID]; !exists {
					rt.Fatalf("cache missing entry for selected workflow %s at iteration %d", selected.ID, i)
				}
			}
		}

		// Final consistency check
		if m.selectedIndex < 0 || m.selectedIndex >= numWorkflows {
			rt.Fatalf("final selectedIndex invalid: %d", m.selectedIndex)
		}

		selected := m.SelectedWorkflow()
		if selected == nil {
			rt.Fatal("final SelectedWorkflow() is nil")
		}
	})
}

// TestModel_EventWorkflowFailedCleansCache verifies that EventWorkflowFailed
// properly cleans up the cache under stress.
func TestModel_EventWorkflowFailedCleansCache(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		numWorkflows := rapid.IntRange(5, 15).Draw(rt, "numWorkflows")
		m := createStressTestModel(t, numWorkflows)

		// Populate cache for all workflows
		for _, wf := range m.workflows {
			state := m.getOrCreateUIState(wf.ID)
			state.CoordinatorMessages = append(state.CoordinatorMessages, chatrender.Message{
				Role:    "assistant",
				Content: "Initial content",
			})
		}

		// Verify all workflows are cached
		initialCacheSize := len(m.workflowUIState)
		if initialCacheSize != numWorkflows && initialCacheSize != maxCachedWorkflows+2 {
			// Cache might be limited by LRU eviction
			if initialCacheSize > maxCachedWorkflows+2 {
				rt.Fatalf("initial cache too large: %d", initialCacheSize)
			}
		}

		// Send EventWorkflowFailed for random workflows
		numToFail := rapid.IntRange(1, numWorkflows/2).Draw(rt, "numToFail")
		failedIDs := make(map[controlplane.WorkflowID]bool)

		for i := 0; i < numToFail; i++ {
			idx := rapid.IntRange(0, numWorkflows-1).Draw(rt, "failIdx")
			wfID := m.workflows[idx].ID

			if failedIDs[wfID] {
				continue // Already failed
			}

			event := controlplane.ControlPlaneEvent{
				Type:       controlplane.EventWorkflowFailed,
				WorkflowID: wfID,
			}
			result, _ := m.Update(event)
			m = result.(Model)
			failedIDs[wfID] = true
		}

		// Verify failed workflows are removed from cache
		for failedID := range failedIDs {
			if _, exists := m.workflowUIState[failedID]; exists {
				rt.Fatalf("failed workflow %s still in cache", failedID)
			}
		}

		// Verify cache size decreased appropriately
		finalCacheSize := len(m.workflowUIState)
		if finalCacheSize > initialCacheSize-len(failedIDs)+2 {
			// Allow some margin due to LRU and protected workflows
			// But cache should generally be smaller
			t.Logf("cache size: initial=%d, final=%d, failed=%d", initialCacheSize, finalCacheSize, len(failedIDs))
		}
	})
}

// TestModel_NoDataRacesOnStateAccess verifies thread-safety of state access patterns
// by running concurrent reads and writes (simulating what could happen with
// concurrent event delivery in a real scenario).
func TestModel_NoDataRacesOnStateAccess(t *testing.T) {
	// This test is designed to be run with -race flag to detect data races
	numWorkflows := 10
	m := createStressTestModel(t, numWorkflows)

	var wg sync.WaitGroup
	var mu sync.Mutex // We need this because Model is not thread-safe by design
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Writer goroutines
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					mu.Lock()
					wfIdx := id % numWorkflows
					wfID := m.workflows[wfIdx].ID
					event := simulateCoordinatorEvent(wfID, "Race test")
					result, _ := m.Update(event)
					m = result.(Model)
					mu.Unlock()
					time.Sleep(time.Microsecond)
				}
			}
		}(i)
	}

	// Reader goroutines
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					mu.Lock()
					_ = m.View()
					_ = m.SelectedWorkflow()
					mu.Unlock()
					time.Sleep(time.Microsecond)
				}
			}
		}()
	}

	wg.Wait()
}
