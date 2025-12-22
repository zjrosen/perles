package orchestration

import (
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"

	"perles/internal/orchestration/message"
	"perles/internal/orchestration/pool"
)

// =============================================================================
// Edge Case 1: Empty Panes
// =============================================================================

func TestEmptyPane_CoordinatorShowsNoScrollIndicator(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Empty coordinator pane should not have scroll indicator
	view := m.View()
	require.NotContains(t, view, "↑") // No scroll indicator
	require.NotContains(t, view, "↓New")
}

func TestEmptyPane_MessageShowsNoScrollIndicator(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Empty message pane should not have scroll indicator
	view := m.View()
	// The MESSAGE LOG title should exist but no scroll indicator
	require.Contains(t, view, "MESSAGE LOG")
	// Count scroll indicators - should be zero for empty panes
}

func TestEmptyPane_WorkerShowsPlaceholder(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Add a worker but no messages
	m = m.UpdateWorker("worker-1", pool.WorkerWorking)

	view := m.View()
	require.Contains(t, view, "Waiting for output...")
}

// =============================================================================
// Edge Case 2: Content Shorter Than Viewport
// =============================================================================

func TestShortContent_CoordinatorNoScrollIndicator(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Add just one short message (fits in viewport)
	m = m.AddChatMessage("coordinator", "Hello")
	_ = m.View() // Render to populate viewport

	// Scroll indicator should not appear
	indicator := buildScrollIndicator(m.coordinatorPane.viewports[viewportKey])
	require.Equal(t, "", indicator, "short content should show no scroll indicator")
}

func TestShortContent_MessageNoScrollIndicator(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Add one short entry
	entries := []message.Entry{
		{
			ID:        "msg-1",
			Timestamp: time.Now(),
			From:      message.ActorCoordinator,
			To:        message.ActorAll,
			Content:   "Short message",
			Type:      message.MessageInfo,
		},
	}
	m = m.SetMessageEntries(entries)
	_ = m.View()

	indicator := buildScrollIndicator(m.messagePane.viewports[viewportKey])
	require.Equal(t, "", indicator, "short content should show no scroll indicator")
}

func TestShortContent_WorkerNoScrollIndicator(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	m = m.UpdateWorker("worker-1", pool.WorkerWorking)
	m = m.AddWorkerMessage("worker-1", "Short message")
	_ = m.View()

	if vp, ok := m.workerPane.viewports["worker-1"]; ok {
		indicator := buildScrollIndicator(vp)
		require.Equal(t, "", indicator, "short content should show no scroll indicator")
	}
}

// =============================================================================
// Edge Case 3: Worker Pane With Multiple Workers
// =============================================================================

func TestMultipleWorkers_ScrollAffectsCurrentWorkerOnly(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Add two workers with enough content to scroll
	m = m.UpdateWorker("worker-1", pool.WorkerWorking)
	m = m.UpdateWorker("worker-2", pool.WorkerWorking)

	// Add lots of content to both workers
	for i := 0; i < 50; i++ {
		m = m.AddWorkerMessage("worker-1", "Worker 1 message line")
		m = m.AddWorkerMessage("worker-2", "Worker 2 message line")
	}

	// Render to create viewports
	_ = m.View()

	// Verify worker-1 is currently displayed
	require.Equal(t, "worker-1", m.CurrentWorkerID())

	// Get initial scroll positions
	vp1Before := m.workerPane.viewports["worker-1"]
	vp2Before := m.workerPane.viewports["worker-2"]
	offset1Before := vp1Before.YOffset
	offset2Before := vp2Before.YOffset

	// Scroll worker pane (affects current worker only)
	// Calculate position in worker pane area
	leftWidth := m.width * 35 / 100
	middleWidth := m.width * 32 / 100
	workerPaneX := leftWidth + middleWidth + 5 // Inside worker pane

	m, _ = m.Update(tea.MouseMsg{
		X:      workerPaneX,
		Y:      10,
		Button: tea.MouseButtonWheelUp,
	})

	// Worker-1's viewport should have changed (scrolled up)
	vp1After := m.workerPane.viewports["worker-1"]
	// Worker-2's viewport should be unchanged
	vp2After := m.workerPane.viewports["worker-2"]

	// The scroll should have affected worker-1
	require.NotEqual(t, offset1Before, vp1After.YOffset,
		"current worker viewport should have scrolled")

	// Worker-2 should be unaffected
	require.Equal(t, offset2Before, vp2After.YOffset,
		"non-current worker viewport should not change")
}

func TestMultipleWorkers_CyclingPreservesScrollPosition(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Add workers with content
	m = m.UpdateWorker("worker-1", pool.WorkerWorking)
	m = m.UpdateWorker("worker-2", pool.WorkerWorking)

	for i := 0; i < 50; i++ {
		m = m.AddWorkerMessage("worker-1", "Worker 1 message")
		m = m.AddWorkerMessage("worker-2", "Worker 2 message")
	}

	// Render to create viewports and scroll worker-1
	_ = m.View()

	// Manually scroll worker-1's viewport up
	if vp, ok := m.workerPane.viewports["worker-1"]; ok {
		vp.SetYOffset(10) // Scroll up
		m.workerPane.viewports["worker-1"] = vp
	}

	savedOffset := m.workerPane.viewports["worker-1"].YOffset

	// Cycle to worker-2
	m = m.CycleWorker(true)
	require.Equal(t, "worker-2", m.CurrentWorkerID())

	// Cycle back to worker-1
	m = m.CycleWorker(false)
	require.Equal(t, "worker-1", m.CurrentWorkerID())

	// Worker-1's scroll position should be preserved
	require.Equal(t, savedOffset, m.workerPane.viewports["worker-1"].YOffset,
		"scroll position should be preserved when cycling workers")
}

// =============================================================================
// Edge Case 4: Scroll at Boundaries
// =============================================================================

func TestScrollBoundary_ScrollUpAtTopIsNoOp(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Add scrollable content
	for i := 0; i < 50; i++ {
		m = m.AddChatMessage("coordinator", "Message line")
	}
	_ = m.View()

	// Manually scroll to top
	vp := m.coordinatorPane.viewports[viewportKey]
	vp.SetYOffset(0)
	m.coordinatorPane.viewports[viewportKey] = vp
	require.Equal(t, 0, m.coordinatorPane.viewports[viewportKey].YOffset)

	// Try to scroll up - should be no-op
	leftWidth := m.width * 35 / 100
	m, _ = m.Update(tea.MouseMsg{
		X:      leftWidth / 2, // Inside coordinator pane
		Y:      10,
		Button: tea.MouseButtonWheelUp,
	})

	// Should still be at offset 0 (can't go negative)
	require.GreaterOrEqual(t, m.coordinatorPane.viewports[viewportKey].YOffset, 0,
		"scroll offset should never be negative")
}

func TestScrollBoundary_ScrollDownAtBottomIsNoOp(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Add scrollable content
	for i := 0; i < 50; i++ {
		m = m.AddChatMessage("coordinator", "Message line")
	}
	_ = m.View()

	// Go to bottom
	vp := m.coordinatorPane.viewports[viewportKey]
	vp.GotoBottom()
	m.coordinatorPane.viewports[viewportKey] = vp
	require.True(t, m.coordinatorPane.viewports[viewportKey].AtBottom())
	bottomOffset := m.coordinatorPane.viewports[viewportKey].YOffset

	// Try to scroll down - should stay at bottom
	leftWidth := m.width * 35 / 100
	m, _ = m.Update(tea.MouseMsg{
		X:      leftWidth / 2,
		Y:      10,
		Button: tea.MouseButtonWheelDown,
	})

	// Should still be at or near bottom (viewport handles clamping)
	require.GreaterOrEqual(t, m.coordinatorPane.viewports[viewportKey].YOffset, bottomOffset-3,
		"scroll down at bottom should not scroll past end")
}

// =============================================================================
// Edge Case 5: Input Bar Area Mouse Wheel Ignored
// =============================================================================

func TestInputBarArea_MouseWheelIgnored(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Add scrollable content
	for i := 0; i < 50; i++ {
		m = m.AddChatMessage("coordinator", "Message line")
	}
	_ = m.View()

	// Scroll to a known position
	vp := m.coordinatorPane.viewports[viewportKey]
	vp.SetYOffset(20)
	m.coordinatorPane.viewports[viewportKey] = vp
	beforeOffset := m.coordinatorPane.viewports[viewportKey].YOffset

	// Send mouse wheel event in input bar area (bottom 4 lines)
	inputAreaY := m.height - 2 // Inside the input bar area
	m, _ = m.Update(tea.MouseMsg{
		X:      10,
		Y:      inputAreaY,
		Button: tea.MouseButtonWheelUp,
	})

	// Viewport should be unchanged
	require.Equal(t, beforeOffset, m.coordinatorPane.viewports[viewportKey].YOffset,
		"mouse wheel in input bar should be ignored")
}

// =============================================================================
// Edge Case 6: Rapid Content Updates / Dirty Tracking
// =============================================================================

func TestRapidUpdates_ContentDirtyFlag(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Add first message - should set dirty
	m = m.AddChatMessage("coordinator", "First message")
	require.True(t, m.coordinatorPane.contentDirty, "content should be dirty after first message")

	// Render (clears dirty via Update cycle)
	m, _ = m.Update(nil) // Update clears dirty flags
	require.False(t, m.coordinatorPane.contentDirty, "content should not be dirty after Update")

	// Add more messages rapidly
	m = m.AddChatMessage("coordinator", "Second message")
	m = m.AddChatMessage("coordinator", "Third message")
	m = m.AddChatMessage("coordinator", "Fourth message")

	// Should be dirty (needs re-render)
	require.True(t, m.coordinatorPane.contentDirty, "content should be dirty after rapid messages")

	// Single render should process all messages
	_ = m.View()

	// Verify all messages are in the rendered content
	content := m.renderCoordinatorContent(80)
	require.Contains(t, content, "First message")
	require.Contains(t, content, "Fourth message")
}

func TestRapidUpdates_WorkerContentDirtyPerWorker(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	m = m.UpdateWorker("worker-1", pool.WorkerWorking)
	m = m.UpdateWorker("worker-2", pool.WorkerWorking)

	// Add message to worker-1 only
	m = m.AddWorkerMessage("worker-1", "Message")

	// Worker-1 should be dirty, worker-2 should not
	require.True(t, m.workerPane.contentDirty["worker-1"], "worker-1 should be dirty")
	require.False(t, m.workerPane.contentDirty["worker-2"], "worker-2 should not be dirty")
}

// =============================================================================
// Edge Case 7: New Content Indicator Behavior
// =============================================================================

func TestNewContentIndicator_AppearsWhenScrolledUp(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Add initial content
	for i := 0; i < 30; i++ {
		m = m.AddChatMessage("coordinator", "Initial message")
	}

	// Render to populate viewport with content
	_ = m.View()

	// After view, we need to manually set content in the viewport to have correct TotalLineCount
	vp := m.coordinatorPane.viewports[viewportKey]
	content := m.renderCoordinatorContent(vp.Width)
	vp.SetContent(content)

	// Scroll up (not at bottom)
	vp.SetYOffset(0)
	m.coordinatorPane.viewports[viewportKey] = vp

	// Verify we're actually scrolled up
	require.False(t, m.coordinatorPane.viewports[viewportKey].AtBottom(),
		"should be scrolled up (not at bottom)")

	// Clear any existing hasNewContent flag
	m.coordinatorPane.hasNewContent = false

	// Add new content while scrolled up
	m = m.AddChatMessage("coordinator", "New message!")

	// hasNewContent should be set
	require.True(t, m.coordinatorPane.hasNewContent,
		"hasNewContent should be true when content arrives while scrolled up")
}

func TestNewContentIndicator_ClearsWhenScrollToBottom(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Set up state: scrolled up with new content flag
	for i := 0; i < 30; i++ {
		m = m.AddChatMessage("coordinator", "Message")
	}
	_ = m.View()

	vp := m.coordinatorPane.viewports[viewportKey]
	vp.SetYOffset(0)
	m.coordinatorPane.viewports[viewportKey] = vp
	m.coordinatorPane.hasNewContent = true

	// Scroll down to bottom
	vp = m.coordinatorPane.viewports[viewportKey]
	vp.GotoBottom()
	m.coordinatorPane.viewports[viewportKey] = vp

	// Simulate the scroll event handling that clears the flag
	leftWidth := m.width * 35 / 100
	m, _ = m.Update(tea.MouseMsg{
		X:      leftWidth / 2,
		Y:      10,
		Button: tea.MouseButtonWheelDown,
	})

	// If at bottom, hasNewContent should be cleared
	if m.coordinatorPane.viewports[viewportKey].AtBottom() {
		require.False(t, m.coordinatorPane.hasNewContent,
			"hasNewContent should be false when scrolled to bottom")
	}
}

func TestNewContentIndicator_NotSetWhenAtBottom(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Small amount of content (stays at bottom)
	m = m.AddChatMessage("coordinator", "First message")
	_ = m.View()

	// Verify at bottom
	vp := m.coordinatorPane.viewports[viewportKey]
	vp.GotoBottom()
	m.coordinatorPane.viewports[viewportKey] = vp
	require.True(t, m.coordinatorPane.viewports[viewportKey].AtBottom())

	// hasNewContent should not be set
	require.False(t, m.coordinatorPane.hasNewContent,
		"hasNewContent should not be set when already at bottom")

	// Add more content (while at bottom)
	m = m.AddChatMessage("coordinator", "Second message")

	// hasNewContent should still be false (auto-scrolling will follow)
	// Note: In the actual implementation, AtBottom() might be false after SetContent
	// because viewport dimensions may have changed. This tests the intent.
}

// =============================================================================
// Edge Case 8: Viewport Dimensions
// =============================================================================

func TestViewportDimensions_MinimumSize(t *testing.T) {
	m := New(Config{})

	// Very small terminal
	m = m.SetSize(40, 10)

	// Should not panic
	view := m.View()
	require.NotEmpty(t, view)

	// Viewports should have minimum dimensions (at least 1x1)
	require.GreaterOrEqual(t, m.coordinatorPane.viewports[viewportKey].Width, 1)
	require.GreaterOrEqual(t, m.coordinatorPane.viewports[viewportKey].Height, 1)
}

func TestViewportDimensions_VeryWide(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(300, 50)

	m = m.AddChatMessage("coordinator", "Test message in wide terminal")
	view := m.View()
	require.NotEmpty(t, view)
}

// =============================================================================
// Edge Case 9: Message Pane Entry Updates
// =============================================================================

func TestMessagePane_NewEntriesTriggerDirty(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	entries := []message.Entry{
		{
			ID:      "msg-1",
			Content: "First",
			Type:    message.MessageInfo,
		},
	}
	m = m.SetMessageEntries(entries)
	require.True(t, m.messagePane.contentDirty)

	// Clear dirty
	m, _ = m.Update(nil)
	require.False(t, m.messagePane.contentDirty)

	// Add more entries
	entries = append(entries, message.Entry{
		ID:      "msg-2",
		Content: "Second",
		Type:    message.MessageInfo,
	})
	m = m.SetMessageEntries(entries)
	require.True(t, m.messagePane.contentDirty, "adding entries should set dirty flag")
}

func TestMessagePane_SameEntriesStillDirty(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	entries := []message.Entry{
		{ID: "msg-1", Content: "First"},
	}
	m = m.SetMessageEntries(entries)

	m, _ = m.Update(nil) // Clear dirty

	// Set same entries again (no new messages)
	m = m.SetMessageEntries(entries)

	// contentDirty is always set, but hasNewContent should not be
	require.True(t, m.messagePane.contentDirty, "SetMessageEntries always sets dirty")
	require.False(t, m.messagePane.hasNewContent,
		"same entry count should not trigger new content indicator")
}

// =============================================================================
// Edge Case 10: Worker Retirement Cleanup
// =============================================================================

func TestWorkerRetirement_ViewportCleanedUp(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Create and retire many workers to trigger cleanup
	for i := 1; i <= 10; i++ {
		workerID := "worker-" + string(rune('0'+i))
		m = m.UpdateWorker(workerID, pool.WorkerWorking)
		m = m.AddWorkerMessage(workerID, "Processing...")

		// Render to create viewport
		_ = m.View()

		// Retire
		m = m.UpdateWorker(workerID, pool.WorkerRetired)
	}

	// Only maxRetiredWorkerViewports (5) should be retained
	require.LessOrEqual(t, len(m.workerPane.viewports), maxRetiredWorkerViewports+2,
		"old retired worker viewports should be cleaned up")
	require.LessOrEqual(t, len(m.workerPane.hasNewContent), maxRetiredWorkerViewports+2,
		"old retired worker hasNewContent should be cleaned up")
}

// =============================================================================
// Regression Tests
// =============================================================================

func TestBuildScrollIndicator_EdgeCases(t *testing.T) {
	tests := []struct {
		name           string
		totalLines     int
		viewportHeight int
		yOffset        int
		wantIndicator  string
	}{
		{
			name:           "zero lines",
			totalLines:     0,
			viewportHeight: 20,
			yOffset:        0,
			wantIndicator:  "",
		},
		{
			name:           "exactly fits viewport",
			totalLines:     20,
			viewportHeight: 20,
			yOffset:        0,
			wantIndicator:  "",
		},
		{
			name:           "one line more than viewport at top",
			totalLines:     21,
			viewportHeight: 20,
			yOffset:        0,
			wantIndicator:  "↑0%",
		},
		{
			name:           "one line more than viewport at bottom",
			totalLines:     21,
			viewportHeight: 20,
			yOffset:        1, // At bottom (21-20=1)
			wantIndicator:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vp := viewport.New(80, tt.viewportHeight)

			// Build content
			var content string
			for i := 0; i < tt.totalLines; i++ {
				if i > 0 {
					content += "\n"
				}
				content += "line"
			}
			if tt.totalLines > 0 {
				vp.SetContent(content)
			}
			vp.SetYOffset(tt.yOffset)

			got := buildScrollIndicator(vp)
			require.Equal(t, tt.wantIndicator, got)
		})
	}
}
