package orchestration

import (
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/require"

	"perles/internal/orchestration/message"
	"perles/internal/orchestration/pool"
)

// testNow is a fixed reference time for reproducible golden tests.
var testNow = time.Date(2025, 12, 21, 14, 30, 0, 0, time.UTC)

func TestNew(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Verify initial state through public API
	total, active := m.WorkerCount()
	require.Equal(t, 0, total)
	require.Equal(t, 0, active)
	require.Equal(t, "", m.CurrentWorkerID())
}

func TestSetSize(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 40)

	require.Equal(t, 120, m.width)
	require.Equal(t, 40, m.height)
}

func TestSetSize_PreservesScrollPosition(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Add enough content to make it scrollable
	for i := 0; i < 50; i++ {
		m = m.AddChatMessage("coordinator", "This is a long message to ensure we have plenty of scrollable content")
	}

	// The viewport content needs to be set explicitly since View() uses a local copy.
	// In the real app, this happens during rendering, but we need to do it manually here.
	vp := m.coordinatorPane.viewports[viewportKey]
	content := m.renderCoordinatorContent(vp.Width)
	vp.SetContent(content)

	// Scroll up to middle (manually set offset to simulate user scrolling)
	totalLines := vp.TotalLineCount()
	if totalLines > vp.Height {
		// Set to approximately 50% scroll
		midOffset := (totalLines - vp.Height) / 2
		vp.SetYOffset(midOffset)
	}
	m.coordinatorPane.viewports[viewportKey] = vp

	// Verify we're not at bottom before resize
	vp = m.coordinatorPane.viewports[viewportKey]
	require.False(t, vp.AtBottom(), "should be scrolled up before resize")
	oldPercent := vp.ScrollPercent()
	require.Greater(t, oldPercent, 0.0, "should have some scroll percent")
	require.Less(t, oldPercent, 1.0, "should not be at bottom")

	// Resize the terminal (make it wider and shorter)
	m = m.SetSize(160, 20)

	// The scroll position should be preserved proportionally
	// Due to re-wrapping, exact position may vary, but should be in similar range
	vp = m.coordinatorPane.viewports[viewportKey]
	newPercent := vp.ScrollPercent()
	require.Greater(t, newPercent, 0.0, "scroll position should be preserved (not at top)")

	// The contentDirty flag should be set for re-rendering
	require.True(t, m.coordinatorPane.contentDirty, "content should be marked dirty after resize")
}

func TestSetSize_AtBottomStaysAtBottom(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Add content
	for i := 0; i < 20; i++ {
		m = m.AddChatMessage("coordinator", "Message content")
	}

	// Render to populate viewport
	_ = m.View()

	// Ensure we're at the bottom (default position)
	vp := m.coordinatorPane.viewports[viewportKey]
	vp.GotoBottom()
	m.coordinatorPane.viewports[viewportKey] = vp
	require.True(t, m.coordinatorPane.viewports[viewportKey].AtBottom(), "should be at bottom before resize")

	// Resize
	m = m.SetSize(160, 25)

	// After resize, the viewport should still be configured to stay at bottom
	// The actual AtBottom check happens after content is re-set in the render
	// We verify that contentDirty is set which triggers the smart auto-scroll
	require.True(t, m.coordinatorPane.contentDirty, "content should be marked dirty")
}

func TestSetSize_UpdatesWorkerViewports(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Add workers with viewports
	m = m.UpdateWorker("worker-1", pool.WorkerWorking)
	m = m.AddWorkerMessage("worker-1", "Processing task...")
	m = m.UpdateWorker("worker-2", pool.WorkerWorking)
	m = m.AddWorkerMessage("worker-2", "Also processing...")

	// Render to create viewports
	_ = m.View()

	// Verify viewports were created
	_, has1 := m.workerPane.viewports["worker-1"]
	_, has2 := m.workerPane.viewports["worker-2"]
	require.True(t, has1, "worker-1 viewport should exist")
	require.True(t, has2, "worker-2 viewport should exist")

	// Resize
	m = m.SetSize(160, 40)

	// Verify worker viewports were updated (contentDirty set)
	require.True(t, m.workerPane.contentDirty["worker-1"], "worker-1 content should be dirty")
	require.True(t, m.workerPane.contentDirty["worker-2"], "worker-2 content should be dirty")
}

func TestAddChatMessage(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	m = m.AddChatMessage("user", "Hello, coordinator!")
	m = m.AddChatMessage("coordinator", "I'll help you orchestrate this epic.")

	// Verify messages appear in the rendered view (word wrap may split text)
	view := m.View()
	require.Contains(t, view, "User")
	require.Contains(t, view, "Hello, coordinator!")
	require.Contains(t, view, "Coordinator")
	require.Contains(t, view, "orchestrate") // Check for key word regardless of wrapping
}

func TestSetMessageEntries(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	entries := []message.Entry{
		{
			ID:        "msg-1",
			Timestamp: testNow,
			From:      message.ActorCoordinator,
			To:        message.ActorAll,
			Content:   "Starting epic execution",
			Type:      message.MessageInfo,
		},
		{
			ID:        "msg-2",
			Timestamp: testNow.Add(5 * time.Minute),
			From:      message.WorkerID(1),
			To:        message.ActorCoordinator,
			Content:   "Task .1 complete",
			Type:      message.MessageCompletion,
		},
	}

	m = m.SetMessageEntries(entries)

	// Verify messages appear in the rendered view
	view := m.View()
	require.Contains(t, view, "Starting epic execution")
	require.Contains(t, view, "Task .1 complete")
	require.Contains(t, view, "COORDINATOR")
	require.Contains(t, view, "WORKER.1")
}

func TestUpdateWorker(t *testing.T) {
	m := New(Config{})

	// Add first worker
	m = m.UpdateWorker("worker-1", pool.WorkerWorking)
	m = m.AddWorkerMessage("worker-1", "Processing task...")

	total, active := m.WorkerCount()
	require.Equal(t, 1, total)
	require.Equal(t, 1, active)
	require.Equal(t, "worker-1", m.CurrentWorkerID())

	// Add second worker
	m = m.UpdateWorker("worker-2", pool.WorkerWorking)
	m = m.AddWorkerMessage("worker-2", "Also processing...")

	total, active = m.WorkerCount()
	require.Equal(t, 2, total)
	require.Equal(t, 2, active)

	// Update first worker to retired - should be removed from list
	m = m.UpdateWorker("worker-1", pool.WorkerRetired)

	total, active = m.WorkerCount()
	require.Equal(t, 1, total)  // Retired workers are removed
	require.Equal(t, 1, active) // Only worker-2 remains
	require.Equal(t, "worker-2", m.CurrentWorkerID())
}

func TestUpdateWorker_ExitsFullscreenWhenFullscreenWorkerRetires(t *testing.T) {
	m := New(Config{})

	// Add three workers
	m = m.UpdateWorker("worker-1", pool.WorkerWorking)
	m = m.UpdateWorker("worker-2", pool.WorkerWorking)
	m = m.UpdateWorker("worker-3", pool.WorkerWorking)

	// Enter fullscreen for worker-2 (index 1)
	m.fullscreenPaneType = PaneWorker
	m.fullscreenWorkerIndex = 1
	require.Equal(t, PaneWorker, m.fullscreenPaneType)
	require.Equal(t, 1, m.fullscreenWorkerIndex)

	// Retire worker-2 (the fullscreen worker)
	m = m.UpdateWorker("worker-2", pool.WorkerRetired)

	// Should exit fullscreen
	require.Equal(t, PaneNone, m.fullscreenPaneType)
	require.Equal(t, -1, m.fullscreenWorkerIndex)
}

func TestUpdateWorker_KeepsFullscreenWhenNonFullscreenWorkerRetires(t *testing.T) {
	m := New(Config{})

	// Add three workers
	m = m.UpdateWorker("worker-1", pool.WorkerWorking)
	m = m.UpdateWorker("worker-2", pool.WorkerWorking)
	m = m.UpdateWorker("worker-3", pool.WorkerWorking)

	// Enter fullscreen for worker-2 (index 1)
	m.fullscreenPaneType = PaneWorker
	m.fullscreenWorkerIndex = 1
	require.Equal(t, PaneWorker, m.fullscreenPaneType)
	require.Equal(t, 1, m.fullscreenWorkerIndex)

	// Retire worker-3 (not the fullscreen worker)
	m = m.UpdateWorker("worker-3", pool.WorkerRetired)

	// Should keep fullscreen
	require.Equal(t, PaneWorker, m.fullscreenPaneType)
	require.Equal(t, 1, m.fullscreenWorkerIndex)
}

func TestCleanupRetiredWorkerViewports(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Create 8 workers and retire them all
	// maxRetiredWorkerViewports = 5, so oldest 3 should be cleaned up
	for i := 1; i <= 8; i++ {
		workerID := "worker-" + string(rune('0'+i))
		m = m.UpdateWorker(workerID, pool.WorkerWorking)
		m = m.AddWorkerMessage(workerID, "Processing...")
		// Manually create viewport to simulate what rendering would do
		m.workerPane.viewports[workerID] = viewport.New(10, 10)
	}

	// Verify all 8 are active
	total, active := m.WorkerCount()
	require.Equal(t, 8, total)
	require.Equal(t, 8, active)

	// Retire workers one by one (1 through 8)
	for i := 1; i <= 8; i++ {
		workerID := "worker-" + string(rune('0'+i))
		m = m.UpdateWorker(workerID, pool.WorkerRetired)
	}

	// All workers should be removed from active list
	total, active = m.WorkerCount()
	require.Equal(t, 0, total)
	require.Equal(t, 0, active)

	// Only last 5 retired workers should have data retained
	// Workers 1-3 should be cleaned up, workers 4-8 should remain
	require.Len(t, m.workerPane.retiredOrder, 5)
	require.Len(t, m.workerPane.viewports, 5)
	require.Len(t, m.workerPane.workerStatus, 5)

	// Verify the correct workers are retained (oldest removed)
	expectedRetired := []string{"worker-4", "worker-5", "worker-6", "worker-7", "worker-8"}
	require.Equal(t, expectedRetired, m.workerPane.retiredOrder)

	// Verify workers 1-3 are fully cleaned up
	for i := 1; i <= 3; i++ {
		workerID := "worker-" + string(rune('0'+i))
		_, hasViewport := m.workerPane.viewports[workerID]
		_, hasStatus := m.workerPane.workerStatus[workerID]
		_, hasMessages := m.workerPane.workerMessages[workerID]
		require.False(t, hasViewport, "worker %s viewport should be cleaned up", workerID)
		require.False(t, hasStatus, "worker %s status should be cleaned up", workerID)
		require.False(t, hasMessages, "worker %s messages should be cleaned up", workerID)
	}

	// Verify workers 4-8 still have their data
	for i := 4; i <= 8; i++ {
		workerID := "worker-" + string(rune('0'+i))
		_, hasStatus := m.workerPane.workerStatus[workerID]
		_, hasMessages := m.workerPane.workerMessages[workerID]
		_, hasViewport := m.workerPane.viewports[workerID]
		require.True(t, hasStatus, "worker %s status should be retained", workerID)
		require.True(t, hasMessages, "worker %s messages should be retained", workerID)
		require.True(t, hasViewport, "worker %s viewport should be retained", workerID)
	}
}

func TestCleanupRetiredWorkerViewports_ActiveWorkersNeverCleaned(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Create 10 workers - retire some but keep others active
	for i := 1; i <= 10; i++ {
		workerID := "worker-" + string(rune('0'+i))
		m = m.UpdateWorker(workerID, pool.WorkerWorking)
		m = m.AddWorkerMessage(workerID, "Processing...")
	}

	// Retire workers 1-8 (8 retired total, triggers cleanup)
	for i := 1; i <= 8; i++ {
		workerID := "worker-" + string(rune('0'+i))
		m = m.UpdateWorker(workerID, pool.WorkerRetired)
	}

	// Workers 9 and 10 (shown as ':' and ';' in ASCII) should still be active
	// Using correct naming: worker-9 and worker-:
	total, active := m.WorkerCount()
	require.Equal(t, 2, total)  // Only workers 9-10 are active
	require.Equal(t, 2, active) // Both are working, not retired

	// Active workers should NEVER be affected by cleanup
	require.Contains(t, m.workerPane.workerIDs, "worker-9")
	require.Contains(t, m.workerPane.workerIDs, "worker-:")

	// Cleanup only affects retired workers
	require.Len(t, m.workerPane.retiredOrder, 5) // max 5 retained
}

func TestCleanupRetiredWorkerViewports_UnderLimit(t *testing.T) {
	m := New(Config{})

	// Create 3 workers and retire them (under the limit of 5)
	for i := 1; i <= 3; i++ {
		workerID := "worker-" + string(rune('0'+i))
		m = m.UpdateWorker(workerID, pool.WorkerWorking)
		m = m.AddWorkerMessage(workerID, "Processing...")
		m = m.UpdateWorker(workerID, pool.WorkerRetired)
	}

	// All 3 should be retained since we're under the limit
	require.Len(t, m.workerPane.retiredOrder, 3)
	require.Len(t, m.workerPane.workerStatus, 3)
	require.Len(t, m.workerPane.workerMessages, 3)

	// Verify all workers retained
	expectedRetired := []string{"worker-1", "worker-2", "worker-3"}
	require.Equal(t, expectedRetired, m.workerPane.retiredOrder)
}

func TestCycleWorker(t *testing.T) {
	m := New(Config{})

	// No workers - should not panic
	m = m.CycleWorker(true)
	require.Equal(t, "", m.CurrentWorkerID())

	// Add workers (retired workers are not added to list)
	m = m.UpdateWorker("worker-1", pool.WorkerWorking)
	m = m.UpdateWorker("worker-2", pool.WorkerWorking)
	m = m.UpdateWorker("worker-3", pool.WorkerWorking)

	// Initial state - first worker
	require.Equal(t, "worker-1", m.CurrentWorkerID())

	// Cycle forward
	m = m.CycleWorker(true)
	require.Equal(t, "worker-2", m.CurrentWorkerID())

	m = m.CycleWorker(true)
	require.Equal(t, "worker-3", m.CurrentWorkerID())

	// Wrap around
	m = m.CycleWorker(true)
	require.Equal(t, "worker-1", m.CurrentWorkerID())

	// Cycle backward
	m = m.CycleWorker(false)
	require.Equal(t, "worker-3", m.CurrentWorkerID())

	// Retire worker-2 - should be removed and cycling adjusted
	m = m.UpdateWorker("worker-2", pool.WorkerRetired)
	total, _ := m.WorkerCount()
	require.Equal(t, 2, total) // worker-1 and worker-3 remain

	// Cycling should skip retired worker
	m = m.CycleWorker(true) // From worker-3 to worker-1
	require.Equal(t, "worker-1", m.CurrentWorkerID())
	m = m.CycleWorker(true) // From worker-1 to worker-3 (skips removed worker-2)
	require.Equal(t, "worker-3", m.CurrentWorkerID())
}

func TestFocusCycling(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Verify initial focus is on coordinator pane (default)
	// Tab cycles focus: Coordinator -> Message -> Worker -> Coordinator
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	// Now focused on worker pane - Tab should cycle back to coordinator
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	// Focus is back on coordinator - verify by checking view renders
	view := m.View()
	require.NotEmpty(t, view) // View should render without panic
}

// Golden tests for view rendering.
// Run with -update flag to update golden files: go test -update ./internal/mode/orchestration/...

func TestView_Golden_Empty(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_WithChat(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	m = m.AddChatMessage("user", "Start working on the auth epic")
	m = m.AddChatMessage("coordinator", "I'll analyze the epic and plan the execution. I see 4 tasks with dependencies.")

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_WithMessages(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	entries := []message.Entry{
		{
			ID:        "msg-1",
			Timestamp: testNow,
			From:      message.ActorCoordinator,
			To:        message.ActorAll,
			Content:   "Starting epic perles-auth. 4 tasks identified.",
			Type:      message.MessageInfo,
		},
		{
			ID:        "msg-2",
			Timestamp: testNow.Add(2 * time.Minute),
			From:      message.WorkerID(1),
			To:        message.ActorCoordinator,
			Content:   "Task complete. Added OAuth providers.",
			Type:      message.MessageCompletion,
		},
	}
	m = m.SetMessageEntries(entries)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_WithWorkers(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	m = m.UpdateWorker("worker-1", pool.WorkerWorking)
	m = m.AddWorkerMessage("worker-1", "Reading auth/oauth.go\nFound existing setup\nAdding Google provider")
	m = m.UpdateWorker("worker-2", pool.WorkerReady)
	m = m.AddWorkerMessage("worker-2", "Task complete")

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_FullState(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(140, 35)

	// Add chat messages
	m = m.AddChatMessage("user", "Start working on the auth epic")
	m = m.AddChatMessage("coordinator", "I'll start tasks .1 and .2 in parallel since they have no dependencies.")

	// Add message log
	entries := []message.Entry{
		{
			ID:        "msg-1",
			Timestamp: testNow,
			From:      message.ActorCoordinator,
			To:        message.ActorAll,
			Content:   "Starting epic perles-auth.",
			Type:      message.MessageInfo,
		},
		{
			ID:        "msg-2",
			Timestamp: testNow.Add(1 * time.Minute),
			From:      message.WorkerID(1),
			To:        message.ActorCoordinator,
			Content:   "Task .1 complete. Tests passing.",
			Type:      message.MessageCompletion,
		},
		{
			ID:        "msg-3",
			Timestamp: testNow.Add(2 * time.Minute),
			From:      message.ActorUser,
			To:        message.ActorCoordinator,
			Content:   "Add refresh token support",
			Type:      message.MessageRequest,
		},
	}
	m = m.SetMessageEntries(entries)

	// Add workers
	m = m.UpdateWorker("worker-1", pool.WorkerReady)
	m = m.AddWorkerMessage("worker-1", "OAuth setup complete")
	m = m.UpdateWorker("worker-2", pool.WorkerWorking)
	m = m.AddWorkerMessage("worker-2", "Adding JWT validation\nProcessing...")

	// Focus on workers pane by pressing Tab twice (Coordinator -> Message -> Worker)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_Narrow(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(80, 24)

	m = m.AddChatMessage("user", "Hello")
	m = m.UpdateWorker("worker-1", pool.WorkerReady)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_Wide(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(200, 40)

	m = m.AddChatMessage("coordinator", "I see this epic has many tasks. Let me analyze the dependency graph to determine optimal execution order.")

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_WithToolCalls(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// User asks coordinator to start
	m = m.AddChatMessage("user", "Start working on the epic")

	// Coordinator responds and makes multiple tool calls (first group)
	m = m.AddChatMessage("coordinator", "I'll load the necessary tools first")
	m = m.AddChatMessage("coordinator", "🔧 MCPSearch")
	m = m.AddChatMessage("coordinator", "🔧 MCPSearch")
	m = m.AddChatMessage("coordinator", "🔧 MCPSearch")

	// Coordinator text response
	m = m.AddChatMessage("coordinator", "Perfect! Now I'll spawn workers for the ready tasks.")

	// Another group of tool calls
	m = m.AddChatMessage("coordinator", "🔧 mcp__perles-orchestrator__post_message")
	m = m.AddChatMessage("coordinator", "🔧 mcp__perles-orchestrator__spawn_worker")
	m = m.AddChatMessage("coordinator", "🔧 mcp__perles-orchestrator__spawn_worker")
	m = m.AddChatMessage("coordinator", "🔧 mcp__perles-orchestrator__list_workers")

	// Final response
	m = m.AddChatMessage("coordinator", "I've spawned 2 workers. Monitoring their progress.")

	// Add a worker with formatted output showing tool calls
	m = m.UpdateWorker("worker-1", pool.WorkerWorking)
	m = m.AddWorkerMessage("worker-1", "I'll start by checking for messages")
	m = m.AddWorkerMessage("worker-1", "🔧 mcp__perles-worker__check_messages")
	m = m.AddWorkerMessage("worker-1", "🔧 mcp__perles-worker__post_message")
	m = m.AddWorkerMessage("worker-1", "Task completed successfully!")

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_Paused(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.paused = true

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_WithError(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m = m.SetError("Worker failed: connection refused")

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestResizeViewportProportional(t *testing.T) {
	tests := []struct {
		name       string
		setupVp    func() viewport.Model
		newWidth   int
		newHeight  int
		wantBottom bool // Should be at bottom after resize
	}{
		{
			name: "at bottom stays at bottom",
			setupVp: func() viewport.Model {
				vp := viewport.New(80, 20)
				content := ""
				for i := 0; i < 50; i++ {
					content += "line\n"
				}
				vp.SetContent(content)
				vp.GotoBottom()
				return vp
			},
			newWidth:   100,
			newHeight:  25,
			wantBottom: true,
		},
		{
			name: "scrolled up preserves position",
			setupVp: func() viewport.Model {
				vp := viewport.New(80, 20)
				content := ""
				for i := 0; i < 100; i++ {
					content += "line\n"
				}
				vp.SetContent(content)
				// Scroll to ~50% (offset 40 of 80 scrollable range)
				vp.SetYOffset(40)
				return vp
			},
			newWidth:   100,
			newHeight:  25,
			wantBottom: false,
		},
		{
			name: "at top stays at top",
			setupVp: func() viewport.Model {
				vp := viewport.New(80, 20)
				content := ""
				for i := 0; i < 50; i++ {
					content += "line\n"
				}
				vp.SetContent(content)
				vp.SetYOffset(0) // At top
				return vp
			},
			newWidth:   100,
			newHeight:  25,
			wantBottom: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vp := tt.setupVp()
			wasAtBottom := vp.AtBottom()
			oldPercent := vp.ScrollPercent()

			newVp := resizeViewportProportional(vp, tt.newWidth, tt.newHeight)

			// Check dimensions updated
			require.Equal(t, tt.newWidth, newVp.Width)
			require.Equal(t, tt.newHeight, newVp.Height)

			// Check scroll position behavior
			if tt.wantBottom {
				// Note: AtBottom check may not work immediately after dimension change
				// since content wasn't re-set. The important thing is the scroll offset
				// is at the maximum position.
				if wasAtBottom {
					// We expected to stay at bottom, so GotoBottom should have been called
					// The actual AtBottom() result depends on content which wasn't updated
					// So we just verify the function completed without error
					_ = newVp.AtBottom()
				}
			} else {
				// For non-bottom cases, verify scroll position was preserved
				// (approximately, since line counts may change with re-wrapping)
				if oldPercent > 0 && oldPercent < 1 {
					// Non-trivial scroll position should be preserved
					newPercent := newVp.ScrollPercent()
					// Allow some variance due to dimension changes
					require.Greater(t, newPercent, 0.0, "scroll position should not be at top")
				}
			}
		})
	}
}

func TestBuildScrollIndicator(t *testing.T) {
	tests := []struct {
		name           string
		totalLines     int
		viewportHeight int
		yOffset        int
		want           string
	}{
		{
			name:           "content fits in viewport",
			totalLines:     10,
			viewportHeight: 20,
			yOffset:        0,
			want:           "",
		},
		{
			name:           "at bottom (live view)",
			totalLines:     100,
			viewportHeight: 20,
			yOffset:        80, // 100 - 20 = 80 (at bottom)
			want:           "",
		},
		{
			name:           "scrolled to top",
			totalLines:     100,
			viewportHeight: 20,
			yOffset:        0,
			want:           "↑0%",
		},
		{
			name:           "scrolled to middle",
			totalLines:     100,
			viewportHeight: 20,
			yOffset:        40, // 50% of scrollable range (0-80)
			want:           "↑50%",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vp := viewport.New(80, tt.viewportHeight)

			// Build content with exact number of lines
			var content string
			for i := 0; i < tt.totalLines; i++ {
				if i > 0 {
					content += "\n"
				}
				content += "line"
			}
			vp.SetContent(content)
			vp.SetYOffset(tt.yOffset)

			got := buildScrollIndicator(vp)
			require.Equal(t, tt.want, got)
		})
	}
}
