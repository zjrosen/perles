package orchestration

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/ui/shared/panes"
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
}

func TestNew_VimModeDisabledByDefault(t *testing.T) {
	// When VimMode is not set in config, it defaults to false
	m := New(Config{})

	// The input textarea should NOT have vim mode enabled
	require.False(t, m.input.VimEnabled(), "vim mode should be disabled by default")
}

func TestNew_VimModeEnabled(t *testing.T) {
	// When VimMode is explicitly set to true
	m := New(Config{VimMode: true})

	// The input textarea should have vim mode enabled
	require.True(t, m.input.VimEnabled(), "vim mode should be enabled when configured")
}

func TestNew_VimModeExplicitlyDisabled(t *testing.T) {
	// When VimMode is explicitly set to false
	m := New(Config{VimMode: false})

	// The input textarea should NOT have vim mode enabled
	require.False(t, m.input.VimEnabled(), "vim mode should be disabled when configured")
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
	m = m.UpdateWorker("worker-1", events.ProcessStatusWorking)
	m = m.AddWorkerMessage("worker-1", "Processing task...")
	m = m.UpdateWorker("worker-2", events.ProcessStatusWorking)
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
	m = m.UpdateWorker("worker-1", events.ProcessStatusWorking)
	m = m.AddWorkerMessage("worker-1", "Processing task...")

	total, active := m.WorkerCount()
	require.Equal(t, 1, total)
	require.Equal(t, 1, active)

	// Add second worker
	m = m.UpdateWorker("worker-2", events.ProcessStatusWorking)
	m = m.AddWorkerMessage("worker-2", "Also processing...")

	total, active = m.WorkerCount()
	require.Equal(t, 2, total)
	require.Equal(t, 2, active)

	// Update first worker to retired - should be removed from list
	m = m.UpdateWorker("worker-1", events.ProcessStatusRetired)

	total, active = m.WorkerCount()
	require.Equal(t, 1, total)  // Retired workers are removed
	require.Equal(t, 1, active) // Only worker-2 remains
}

func TestUpdateWorker_ExitsFullscreenWhenFullscreenWorkerRetires(t *testing.T) {
	m := New(Config{})

	// Add three workers
	m = m.UpdateWorker("worker-1", events.ProcessStatusWorking)
	m = m.UpdateWorker("worker-2", events.ProcessStatusWorking)
	m = m.UpdateWorker("worker-3", events.ProcessStatusWorking)

	// Enter fullscreen for worker-2 (index 1)
	m.fullscreenPaneType = PaneWorker
	m.fullscreenWorkerIndex = 1
	require.Equal(t, PaneWorker, m.fullscreenPaneType)
	require.Equal(t, 1, m.fullscreenWorkerIndex)

	// Retire worker-2 (the fullscreen worker)
	m = m.UpdateWorker("worker-2", events.ProcessStatusRetired)

	// Should exit fullscreen
	require.Equal(t, PaneNone, m.fullscreenPaneType)
	require.Equal(t, -1, m.fullscreenWorkerIndex)
}

func TestUpdateWorker_KeepsFullscreenWhenNonFullscreenWorkerRetires(t *testing.T) {
	m := New(Config{})

	// Add three workers
	m = m.UpdateWorker("worker-1", events.ProcessStatusWorking)
	m = m.UpdateWorker("worker-2", events.ProcessStatusWorking)
	m = m.UpdateWorker("worker-3", events.ProcessStatusWorking)

	// Enter fullscreen for worker-2 (index 1)
	m.fullscreenPaneType = PaneWorker
	m.fullscreenWorkerIndex = 1
	require.Equal(t, PaneWorker, m.fullscreenPaneType)
	require.Equal(t, 1, m.fullscreenWorkerIndex)

	// Retire worker-3 (not the fullscreen worker)
	m = m.UpdateWorker("worker-3", events.ProcessStatusRetired)

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
		m = m.UpdateWorker(workerID, events.ProcessStatusWorking)
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
		m = m.UpdateWorker(workerID, events.ProcessStatusRetired)
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
		m = m.UpdateWorker(workerID, events.ProcessStatusWorking)
		m = m.AddWorkerMessage(workerID, "Processing...")
	}

	// Retire workers 1-8 (8 retired total, triggers cleanup)
	for i := 1; i <= 8; i++ {
		workerID := "worker-" + string(rune('0'+i))
		m = m.UpdateWorker(workerID, events.ProcessStatusRetired)
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
		m = m.UpdateWorker(workerID, events.ProcessStatusWorking)
		m = m.AddWorkerMessage(workerID, "Processing...")
		m = m.UpdateWorker(workerID, events.ProcessStatusRetired)
	}

	// All 3 should be retained since we're under the limit
	require.Len(t, m.workerPane.retiredOrder, 3)
	require.Len(t, m.workerPane.workerStatus, 3)
	require.Len(t, m.workerPane.workerMessages, 3)

	// Verify all workers retained
	expectedRetired := []string{"worker-1", "worker-2", "worker-3"}
	require.Equal(t, expectedRetired, m.workerPane.retiredOrder)
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
	// Set mcpPort to demonstrate coordinator title with port
	m.mcpPort = 8467

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
	m.mcpPort = 9000

	// Add workers with task IDs and phases
	m = m.UpdateWorker("worker-1", events.ProcessStatusWorking)
	m = m.SetWorkerTask("worker-1", "perles-auth.1", events.ProcessPhaseImplementing)
	m = m.AddWorkerMessage("worker-1", "Reading auth/oauth.go\nFound existing setup\nAdding Google provider")
	m = m.UpdateWorker("worker-2", events.ProcessStatusReady)
	m = m.SetWorkerTask("worker-2", "", events.ProcessPhaseIdle) // Idle worker, no task
	m = m.AddWorkerMessage("worker-2", "Task complete")

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_FullState(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(140, 35)
	m.mcpPort = 8467

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

	// Add workers with task context
	m = m.UpdateWorker("worker-1", events.ProcessStatusReady)
	m = m.SetWorkerTask("worker-1", "perles-auth.1", events.ProcessPhaseIdle)
	m = m.AddWorkerMessage("worker-1", "OAuth setup complete")
	m = m.UpdateWorker("worker-2", events.ProcessStatusWorking)
	m = m.SetWorkerTask("worker-2", "perles-auth.2", events.ProcessPhaseReviewing)
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
	m = m.UpdateWorker("worker-1", events.ProcessStatusReady)

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

func TestView_Golden_CoordinatorStopped(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.mcpPort = 8467

	// Set coordinator to stopped status
	m.coordinatorStatus = events.ProcessStatusStopped

	m = m.AddChatMessage("user", "Please pause operations")
	m = m.AddChatMessage("coordinator", "Understood, pausing now.")

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_WorkerStopped(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.mcpPort = 9000

	// Add a working worker and a stopped worker
	m = m.UpdateWorker("worker-1", events.ProcessStatusWorking)
	m = m.SetWorkerTask("worker-1", "perles-auth.1", events.ProcessPhaseImplementing)
	m = m.AddWorkerMessage("worker-1", "Working on OAuth provider integration...")

	m = m.UpdateWorker("worker-2", events.ProcessStatusStopped)
	m = m.AddWorkerMessage("worker-2", "Stopped by user request")

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
	m = m.UpdateWorker("worker-1", events.ProcessStatusWorking)
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

func TestView_Golden_FullscreenCoordinator(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Add chat messages to the coordinator pane
	m = m.AddChatMessage("user", "Start working on the auth epic")
	m = m.AddChatMessage("coordinator", "I'll analyze the epic and plan the execution. I see 4 tasks with dependencies.")
	m = m.AddChatMessage("coordinator", "🔧 MCPSearch")
	m = m.AddChatMessage("coordinator", "🔧 MCPSearch")
	m = m.AddChatMessage("coordinator", "Now I'll spawn workers for the ready tasks.")
	m = m.AddChatMessage("coordinator", "🔧 mcp__perles-orchestrator__spawn_worker")
	m = m.AddChatMessage("coordinator", "🔧 mcp__perles-orchestrator__spawn_worker")

	// Enter navigation mode and fullscreen coordinator pane
	m.navigationMode = true
	m.fullscreenPaneType = PaneCoordinator
	m.fullscreenWorkerIndex = -1

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_FullscreenMessages(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Add message log entries
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
			Content:   "Task .1 complete. OAuth providers added successfully.",
			Type:      message.MessageCompletion,
		},
		{
			ID:        "msg-3",
			Timestamp: testNow.Add(5 * time.Minute),
			From:      message.ActorUser,
			To:        message.ActorCoordinator,
			Content:   "Add refresh token support to the implementation",
			Type:      message.MessageRequest,
		},
		{
			ID:        "msg-4",
			Timestamp: testNow.Add(7 * time.Minute),
			From:      message.WorkerID(2),
			To:        message.ActorCoordinator,
			Content:   "Task .2 in progress. Adding JWT validation.",
			Type:      message.MessageInfo,
		},
	}
	m = m.SetMessageEntries(entries)

	// Enter navigation mode and fullscreen messages pane
	m.navigationMode = true
	m.fullscreenPaneType = PaneMessages
	m.fullscreenWorkerIndex = -1

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_FullscreenWorker(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Add workers with messages, task IDs and phases
	m = m.UpdateWorker("worker-1", events.ProcessStatusWorking)
	m = m.SetWorkerTask("worker-1", "perles-auth.1", events.ProcessPhaseImplementing)
	m = m.AddWorkerMessage("worker-1", "Reading auth/oauth.go")
	m = m.AddWorkerMessage("worker-1", "Found existing OAuth setup")
	m = m.AddWorkerMessage("worker-1", "🔧 Read")
	m = m.AddWorkerMessage("worker-1", "🔧 Grep")
	m = m.AddWorkerMessage("worker-1", "Adding Google provider configuration")
	m = m.AddWorkerMessage("worker-1", "🔧 Edit")
	m = m.AddWorkerMessage("worker-1", "Running tests to verify changes")
	m = m.AddWorkerMessage("worker-1", "🔧 Bash")

	m = m.UpdateWorker("worker-2", events.ProcessStatusReady)
	m = m.SetWorkerTask("worker-2", "perles-auth.2", events.ProcessPhaseAwaitingReview)
	m = m.AddWorkerMessage("worker-2", "Task complete")

	// Enter navigation mode and fullscreen worker-1 (index 0)
	m.navigationMode = true
	m.fullscreenPaneType = PaneWorker
	m.fullscreenWorkerIndex = 0

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_FullscreenCommand(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Add command log entries
	m.commandPane.entries = []CommandLogEntry{
		{
			Timestamp:   testNow,
			CommandType: command.CmdSpawnProcess,
			CommandID:   "abc12345-6789-0123-4567-890abcdef012",
			Source:      command.SourceInternal,
			Success:     true,
			Duration:    25 * time.Millisecond,
		},
		{
			Timestamp:   testNow.Add(time.Second),
			CommandType: command.CmdAssignTask,
			CommandID:   "def12345-6789-0123-4567-890abcdef012",
			Source:      command.SourceMCPTool,
			Success:     true,
			Duration:    150 * time.Millisecond,
		},
		{
			Timestamp:   testNow.Add(2 * time.Second),
			CommandType: command.CmdSendToProcess,
			CommandID:   "ghi12345-6789-0123-4567-890abcdef012",
			Source:      command.SourceUser,
			Success:     false,
			Error:       "worker not found",
			Duration:    5 * time.Millisecond,
		},
		{
			Timestamp:   testNow.Add(3 * time.Second),
			CommandType: command.CmdReportComplete,
			CommandID:   "jkl12345-6789-0123-4567-890abcdef012",
			Source:      command.SourceCallback,
			Success:     true,
			Duration:    1200 * time.Millisecond,
		},
	}
	m.commandPane.contentDirty = true

	// Enter navigation mode and fullscreen command pane
	m.navigationMode = true
	m.fullscreenPaneType = PaneCommand

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestScrollablePane_PreservesScrollOnResize(t *testing.T) {
	// Test that ScrollablePane preserves scroll position proportionally on resize

	t.Run("at bottom stays at bottom", func(t *testing.T) {
		vp := viewport.New(80, 20)
		content := strings.Repeat("line\n", 50)
		vp.SetContent(content)
		vp.GotoBottom()
		require.True(t, vp.AtBottom(), "should start at bottom")

		// Call ScrollablePane with new dimensions
		_ = panes.ScrollablePane(102, 27, panes.ScrollableConfig{
			Viewport:     &vp,
			ContentDirty: true,
			LeftTitle:    "TEST",
			TitleColor:   CoordinatorColor,
			BorderColor:  CoordinatorColor,
		}, func(wrapWidth int) string {
			return strings.Repeat("line\n", 50)
		})

		require.True(t, vp.AtBottom(), "should stay at bottom after resize")
	})

	t.Run("scrolled up preserves position", func(t *testing.T) {
		vp := viewport.New(80, 20)
		content := strings.Repeat("line\n", 100)
		vp.SetContent(content)
		// Scroll to ~50% (offset 40 of 80 scrollable range)
		vp.SetYOffset(40)
		require.False(t, vp.AtBottom(), "should not be at bottom")
		oldPercent := vp.ScrollPercent()

		// Call ScrollablePane with new dimensions (resize)
		_ = panes.ScrollablePane(102, 27, panes.ScrollableConfig{
			Viewport:     &vp,
			ContentDirty: true,
			LeftTitle:    "TEST",
			TitleColor:   CoordinatorColor,
			BorderColor:  CoordinatorColor,
		}, func(wrapWidth int) string {
			return strings.Repeat("line\n", 100)
		})

		// Scroll position should be approximately preserved
		newPercent := vp.ScrollPercent()
		require.Greater(t, newPercent, 0.0, "should not be at top")
		// Allow variance since dimensions changed, but should be in similar range
		require.InDelta(t, oldPercent, newPercent, 0.15, "scroll position should be approximately preserved")
	})

	t.Run("at top stays at top", func(t *testing.T) {
		vp := viewport.New(80, 20)
		content := strings.Repeat("line\n", 50)
		vp.SetContent(content)
		vp.SetYOffset(0)
		require.Equal(t, 0, vp.YOffset, "should start at top")

		// Call ScrollablePane with new dimensions
		_ = panes.ScrollablePane(102, 27, panes.ScrollableConfig{
			Viewport:     &vp,
			ContentDirty: true,
			LeftTitle:    "TEST",
			TitleColor:   CoordinatorColor,
			BorderColor:  CoordinatorColor,
		}, func(wrapWidth int) string {
			return strings.Repeat("line\n", 50)
		})

		// When at top (0%), should stay at top
		require.Equal(t, 0, vp.YOffset, "should stay at top after resize")
	})
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

			got := panes.BuildScrollIndicator(vp)
			require.Equal(t, tt.want, got)
		})
	}
}

// Invariant tests for panes.ScrollablePane helper.
// These tests verify the critical invariants documented in the scrollable pane implementation.

func TestRenderScrollablePane_WasAtBottomTiming(t *testing.T) {
	// This test verifies that wasAtBottom is checked BEFORE SetContent().
	// If this invariant is violated, users would be forcibly scrolled to bottom
	// every time new content arrives, even when reading history.

	vp := viewport.New(80, 20)

	// Build content that exceeds viewport height
	var content string
	for i := 0; i < 100; i++ {
		if i > 0 {
			content += "\n"
		}
		content += "line"
	}
	vp.SetContent(content)

	// Scroll up to middle (not at bottom)
	vp.SetYOffset(40)
	require.False(t, vp.AtBottom(), "should be scrolled up initially")

	// Call panes.ScrollablePane with contentDirty=true
	// Content still exceeds viewport to properly test scroll preservation
	_ = panes.ScrollablePane(84, 22, panes.ScrollableConfig{
		Viewport:       &vp,
		ContentDirty:   true,
		HasNewContent:  false,
		MetricsDisplay: "",
		LeftTitle:      "TEST",
		TitleColor:     CoordinatorColor,
		BorderColor:    CoordinatorColor,
	}, func(wrapWidth int) string {
		// Return content that still exceeds viewport height
		// so we can properly test if scroll position is preserved
		var newContent string
		for i := 0; i < 100; i++ {
			if i > 0 {
				newContent += "\n"
			}
			newContent += "updated line"
		}
		return newContent
	})

	// If the invariant is preserved, we should NOT be at bottom
	// because we were scrolled up and wasAtBottom was captured before SetContent
	require.False(t, vp.AtBottom(), "scroll position should be preserved when scrolled up")
}

func TestRenderScrollablePane_PaddingIsPrepended(t *testing.T) {
	// This test verifies that content padding is PREPENDED (not appended).
	// If padding is appended, content would appear at the top of the viewport
	// instead of being pushed to the bottom.

	vp := viewport.New(80, 20)

	// Call with short content that needs padding
	result := panes.ScrollablePane(84, 22, panes.ScrollableConfig{
		Viewport:       &vp,
		ContentDirty:   false,
		HasNewContent:  false,
		MetricsDisplay: "",
		LeftTitle:      "TEST",
		TitleColor:     CoordinatorColor,
		BorderColor:    CoordinatorColor,
	}, func(wrapWidth int) string {
		return "short content"
	})

	// The rendered content should have the actual text at the bottom.
	// We verify by checking that the content appears in the result
	// and that empty lines come before it (prepended padding).
	require.Contains(t, result, "short content", "content should appear in result")

	// Verify viewport content has padding prepended by checking line count
	// equals viewport height (padding fills the difference)
	viewContent := vp.View()
	lines := len(strings.Split(viewContent, "\n"))
	require.Equal(t, vp.Height, lines, "viewport should be filled with padded content")

	// Verify content is at bottom by checking it's in the last lines
	viewLines := strings.Split(viewContent, "\n")
	lastLine := viewLines[len(viewLines)-1]
	require.Contains(t, lastLine, "short content", "content should be at bottom of viewport")
}

func TestRenderScrollablePane_ViewportReferenceSemantics(t *testing.T) {
	// This test verifies that viewport uses pointer semantics.
	// If value semantics were used, scroll state changes wouldn't persist
	// because Go copies structs by value.

	vp := viewport.New(80, 20)

	// Build scrollable content
	var content string
	for i := 0; i < 100; i++ {
		if i > 0 {
			content += "\n"
		}
		content += "line"
	}
	vp.SetContent(content)
	vp.GotoBottom()

	// Scroll up
	vp.SetYOffset(30)
	initialOffset := vp.YOffset

	// Call panes.ScrollablePane with contentDirty=false (shouldn't auto-scroll)
	_ = panes.ScrollablePane(84, 22, panes.ScrollableConfig{
		Viewport:       &vp, // Pointer, not value
		ContentDirty:   false,
		HasNewContent:  false,
		MetricsDisplay: "",
		LeftTitle:      "TEST",
		TitleColor:     CoordinatorColor,
		BorderColor:    CoordinatorColor,
	}, func(wrapWidth int) string {
		return content
	})

	// The viewport dimensions should be updated (proving writes persist)
	require.Equal(t, 82, vp.Width, "viewport width should be updated via pointer")
	require.Equal(t, 20, vp.Height, "viewport height should be updated via pointer")

	// Scroll position should be preserved (proving we used pointer semantics)
	// Note: The exact offset may change due to dimension changes, but it should
	// still be in a non-bottom position if pointer semantics are working
	require.Greater(t, vp.YOffset, 0, "scroll offset should be preserved")
	require.False(t, vp.AtBottom(), "should not be at bottom (scroll preserved)")
	_ = initialOffset // Avoid unused variable
}

// Boundary condition tests for renderChatContent helper.
// These tests verify the critical edge cases in tool call sequence detection.
// CRITICAL: Off-by-one errors in these conditions cause index out of bounds or wrong tree characters.

func TestRenderChatContent_SingleToolCall(t *testing.T) {
	// Single tool call: Both first AND last (should get ╰╴ character, not ├╴)
	// This tests the case where i == 0 AND i == len(messages)-1
	messages := []ChatMessage{
		{Role: "assistant", Content: "🔧 Read", IsToolCall: true},
	}

	cfg := ChatRenderConfig{
		AgentLabel: "Coordinator",
		AgentColor: CoordinatorColor,
	}

	result := renderChatContent(messages, 80, cfg)

	// Single tool call should use ╰╴ (last in sequence), not ├╴
	require.Contains(t, result, "╰╴ Read", "single tool call should use ╰╴ character")
	require.NotContains(t, result, "├╴", "single tool call should NOT use ├╴ character")

	// Should have role label before tool call
	require.Contains(t, result, "Coordinator", "should show agent role label")
}

func TestRenderChatContent_FirstMessageIsToolCall(t *testing.T) {
	// First message is tool call: Tests i == 0 boundary in isFirstToolInSequence
	// This ensures we don't try to access messages[i-1] when i == 0
	messages := []ChatMessage{
		{Role: "assistant", Content: "🔧 Glob", IsToolCall: true},
		{Role: "assistant", Content: "🔧 Read", IsToolCall: true},
		{Role: "assistant", Content: "Found the files", IsToolCall: false},
	}

	cfg := ChatRenderConfig{
		AgentLabel: "Worker",
		AgentColor: WorkerColor,
	}

	result := renderChatContent(messages, 80, cfg)

	// First tool should start sequence with role label
	require.Contains(t, result, "Worker", "should show agent role label for tool sequence")

	// First tool gets ├╴, last tool gets ╰╴
	require.Contains(t, result, "├╴ Glob", "first tool in sequence should use ├╴")
	require.Contains(t, result, "╰╴ Read", "last tool in sequence should use ╰╴")

	// Regular message at end should have its own role label
	require.Contains(t, result, "Found the files")
}

func TestRenderChatContent_LastMessageIsToolCall(t *testing.T) {
	// Last message is tool call: Tests i == len(messages)-1 boundary in isLastToolInSequence
	// This ensures we don't try to access messages[i+1] when at the last index
	messages := []ChatMessage{
		{Role: "assistant", Content: "Starting work", IsToolCall: false},
		{Role: "assistant", Content: "🔧 Edit", IsToolCall: true},
		{Role: "assistant", Content: "🔧 Bash", IsToolCall: true},
	}

	cfg := ChatRenderConfig{
		AgentLabel: "Coordinator",
		AgentColor: CoordinatorColor,
	}

	result := renderChatContent(messages, 80, cfg)

	// First message should have role label
	require.Contains(t, result, "Coordinator")
	require.Contains(t, result, "Starting work")

	// Tool sequence: Edit is first (├╴), Bash is last (╰╴)
	require.Contains(t, result, "├╴ Edit", "first tool in ending sequence should use ├╴")
	require.Contains(t, result, "╰╴ Bash", "last tool (at end of messages) should use ╰╴")
}

func TestRenderChatContent_NonToolCallSurroundedByToolCalls(t *testing.T) {
	// Non-tool call surrounded by tool calls: Tests sequence breaking
	// Tool calls should form separate sequences with the text message breaking them
	messages := []ChatMessage{
		{Role: "assistant", Content: "🔧 Read", IsToolCall: true},
		{Role: "assistant", Content: "🔧 Glob", IsToolCall: true},
		{Role: "assistant", Content: "Found what I needed", IsToolCall: false},
		{Role: "assistant", Content: "🔧 Edit", IsToolCall: true},
		{Role: "assistant", Content: "🔧 Bash", IsToolCall: true},
	}

	cfg := ChatRenderConfig{
		AgentLabel:              "Worker",
		AgentColor:              WorkerColor,
		ShowCoordinatorInWorker: true,
	}

	result := renderChatContent(messages, 80, cfg)

	// First sequence: Read (├╴) -> Glob (╰╴)
	require.Contains(t, result, "├╴ Read", "first tool in first sequence should use ├╴")
	require.Contains(t, result, "╰╴ Glob", "last tool in first sequence should use ╰╴")

	// Text message breaks the sequence
	require.Contains(t, result, "Found what I needed")

	// Second sequence: Edit (├╴) -> Bash (╰╴)
	require.Contains(t, result, "├╴ Edit", "first tool in second sequence should use ├╴")
	require.Contains(t, result, "╰╴ Bash", "last tool in second sequence should use ╰╴")
}

// ============================================================================
// Command Pane Golden Tests
// ============================================================================

func TestView_Golden_WithCommandPane(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Show the command pane
	m.showCommandPane = true

	// Add sample command log entries
	m.commandPane.entries = []CommandLogEntry{
		{
			Timestamp:   testNow,
			CommandType: command.CmdSpawnProcess,
			CommandID:   "aaaaaaaa-1234-1234-1234-123456789abc",
			Source:      command.SourceMCPTool,
			Success:     true,
			Duration:    45 * time.Millisecond,
		},
		{
			Timestamp:   testNow.Add(1 * time.Second),
			CommandType: command.CmdAssignTask,
			CommandID:   "bbbbbbbb-1234-1234-1234-123456789abc",
			Source:      command.SourceInternal,
			Success:     true,
			Duration:    120 * time.Millisecond,
		},
		{
			Timestamp:   testNow.Add(2 * time.Second),
			CommandType: command.CmdSendToProcess,
			CommandID:   "cccccccc-1234-1234-1234-123456789abc",
			Source:      command.SourceUser,
			Success:     true,
			Duration:    8 * time.Millisecond,
		},
	}
	m.commandPane.contentDirty = true

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_CommandPaneEmpty(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Show the command pane with no entries
	m.showCommandPane = true
	m.commandPane.entries = []CommandLogEntry{}
	m.commandPane.contentDirty = true

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_CommandPaneWithErrors(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Show the command pane with error entries
	m.showCommandPane = true

	// Add entries with failures (red highlighting)
	m.commandPane.entries = []CommandLogEntry{
		{
			Timestamp:   testNow,
			CommandType: command.CmdSpawnProcess,
			CommandID:   "aaaaaaaa-1234-1234-1234-123456789abc",
			Source:      command.SourceMCPTool,
			Success:     true,
			Duration:    45 * time.Millisecond,
		},
		{
			Timestamp:   testNow.Add(1 * time.Second),
			CommandType: command.CmdAssignTask,
			CommandID:   "bbbbbbbb-1234-1234-1234-123456789abc",
			Source:      command.SourceInternal,
			Success:     false,
			Error:       "worker not found",
			Duration:    5 * time.Millisecond,
		},
		{
			Timestamp:   testNow.Add(2 * time.Second),
			CommandType: command.CmdSendToProcess,
			CommandID:   "cccccccc-1234-1234-1234-123456789abc",
			Source:      command.SourceCallback,
			Success:     false,
			Error:       "process terminated unexpectedly",
			Duration:    12 * time.Millisecond,
		},
	}
	m.commandPane.contentDirty = true

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_CommandPaneHidden(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Verify command pane is hidden by default (no debug mode)
	require.False(t, m.showCommandPane)

	// Add entries anyway - they should accumulate but not be visible
	m.commandPane.entries = []CommandLogEntry{
		{
			Timestamp:   testNow,
			CommandType: command.CmdSpawnProcess,
			CommandID:   "hidden-cmd-123456789abc",
			Source:      command.SourceInternal,
			Success:     true,
			Duration:    100 * time.Millisecond,
		},
	}

	// Verify the pane is hidden
	require.False(t, m.showCommandPane, "showCommandPane should be false")

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}
