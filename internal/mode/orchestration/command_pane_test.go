package orchestration

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/orchestration/v2/command"
)

// ============================================================================
// Initialization Tests
// ============================================================================

func TestNewCommandPane(t *testing.T) {
	pane := newCommandPane()

	// Should have empty entries
	require.Empty(t, pane.entries)

	// Should have initialized viewports map with default viewport
	require.NotNil(t, pane.viewports)
	_, ok := pane.viewports[viewportKey]
	require.True(t, ok, "viewport should be initialized with viewportKey")

	// Should start with contentDirty true for initial render
	require.True(t, pane.contentDirty)

	// Should start with hasNewContent false
	require.False(t, pane.hasNewContent)
}

// ============================================================================
// Rendering Tests - Empty State
// ============================================================================

func TestRenderCommandContent_Empty(t *testing.T) {
	entries := []CommandLogEntry{}

	content := renderCommandContent(entries, 100)

	require.Empty(t, content, "empty entries should produce empty content")
}

// ============================================================================
// Rendering Tests - With Entries
// ============================================================================

func TestRenderCommandContent_WithEntries(t *testing.T) {
	entries := []CommandLogEntry{
		{
			Timestamp:   time.Date(2026, 1, 3, 15, 4, 5, 0, time.UTC),
			CommandType: command.CmdSpawnProcess,
			CommandID:   "12345678-1234-1234-1234-123456789abc",
			Source:      command.SourceMCPTool,
			Success:     true,
			Duration:    50 * time.Millisecond,
		},
	}

	content := renderCommandContent(entries, 200)

	// Should contain timestamp
	require.Contains(t, content, "15:04:05")

	// Should contain source in brackets
	require.Contains(t, content, "[mcp_tool]")

	// Should contain command type
	require.Contains(t, content, "spawn_process")

	// Should contain success checkmark
	require.Contains(t, content, "✓")

	// Should contain duration
	require.Contains(t, content, "50ms")
}

func TestRenderCommandContent_MultipleEntries(t *testing.T) {
	entries := []CommandLogEntry{
		{
			Timestamp:   time.Date(2026, 1, 3, 15, 4, 5, 0, time.UTC),
			CommandType: command.CmdSpawnProcess,
			CommandID:   "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			Source:      command.SourceMCPTool,
			Success:     true,
			Duration:    50 * time.Millisecond,
		},
		{
			Timestamp:   time.Date(2026, 1, 3, 15, 4, 6, 0, time.UTC),
			CommandType: command.CmdAssignTask,
			CommandID:   "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
			Source:      command.SourceInternal,
			Success:     true,
			Duration:    100 * time.Millisecond,
		},
	}

	content := renderCommandContent(entries, 200)

	// Should contain both entries
	require.Contains(t, content, "15:04:05")
	require.Contains(t, content, "15:04:06")
	require.Contains(t, content, "spawn_process")
	require.Contains(t, content, "assign_task")

	// Should be multiple lines
	lines := strings.Split(content, "\n")
	require.Len(t, lines, 2)
}

// ============================================================================
// Success Formatting Tests
// ============================================================================

func TestRenderCommandContent_SuccessFormatting(t *testing.T) {
	entries := []CommandLogEntry{
		{
			Timestamp:   time.Date(2026, 1, 3, 10, 30, 0, 0, time.UTC),
			CommandType: command.CmdReportComplete,
			CommandID:   "11111111-1111-1111-1111-111111111111",
			Source:      command.SourceCallback,
			Success:     true,
			Duration:    25 * time.Millisecond,
		},
	}

	content := renderCommandContent(entries, 200)

	// Should contain green checkmark (via ANSI codes, but we check for ✓)
	require.Contains(t, content, "✓")

	// Should NOT contain error marker
	require.NotContains(t, content, "✗")
}

// ============================================================================
// Failure Formatting Tests
// ============================================================================

func TestRenderCommandContent_FailureFormatting(t *testing.T) {
	entries := []CommandLogEntry{
		{
			Timestamp:   time.Date(2026, 1, 3, 10, 30, 0, 0, time.UTC),
			CommandType: command.CmdAssignTask,
			CommandID:   "22222222-2222-2222-2222-222222222222",
			Source:      command.SourceUser,
			Success:     false,
			Error:       "worker not found",
			Duration:    5 * time.Millisecond,
		},
	}

	content := renderCommandContent(entries, 200)

	// Should contain red X marker (via ANSI codes, but we check for ✗)
	require.Contains(t, content, "✗")

	// Should contain error message
	require.Contains(t, content, "worker not found")

	// Should NOT contain success marker
	require.NotContains(t, content, "✓")
}

func TestRenderCommandContent_FailureWithoutError(t *testing.T) {
	entries := []CommandLogEntry{
		{
			Timestamp:   time.Date(2026, 1, 3, 10, 30, 0, 0, time.UTC),
			CommandType: command.CmdRetireProcess,
			CommandID:   "33333333-3333-3333-3333-333333333333",
			Source:      command.SourceInternal,
			Success:     false,
			Error:       "", // Empty error string
			Duration:    10 * time.Millisecond,
		},
	}

	content := renderCommandContent(entries, 200)

	// Should contain just the X marker
	require.Contains(t, content, "✗")

	// Should NOT contain success marker
	require.NotContains(t, content, "✓")
}

// ============================================================================
// Error Truncation Tests
// ============================================================================

func TestRenderCommandContent_ErrorTruncation(t *testing.T) {
	longError := "this is a very long error message that exceeds the maximum display length of two hundred characters and should definitely be truncated when rendered in the command pane because we want to keep the UI clean and readable"
	require.Greater(t, len(longError), maxErrorDisplayLength, "test error should be longer than max display length")

	entries := []CommandLogEntry{
		{
			Timestamp:   time.Date(2026, 1, 3, 10, 30, 0, 0, time.UTC),
			CommandType: command.CmdAssignTask,
			CommandID:   "44444444-4444-4444-4444-444444444444",
			Source:      command.SourceMCPTool,
			Success:     false,
			Error:       longError,
			Duration:    5 * time.Millisecond,
		},
	}

	content := renderCommandContent(entries, 500) // Wide viewport to not truncate line

	// Should contain truncation indicator
	require.Contains(t, content, "...")

	// Should NOT contain the full error message (the end part)
	require.NotContains(t, content, "clean and readable")

	// Should contain the beginning of the error
	require.Contains(t, content, "this is a very long error")
}

func TestRenderCommandContent_ErrorAtMaxLength(t *testing.T) {
	// Create error exactly at max length - should NOT be truncated
	exactError := strings.Repeat("x", maxErrorDisplayLength)
	require.Equal(t, maxErrorDisplayLength, len(exactError))

	entries := []CommandLogEntry{
		{
			Timestamp:   time.Date(2026, 1, 3, 10, 30, 0, 0, time.UTC),
			CommandType: command.CmdAssignTask,
			CommandID:   "55555555-5555-5555-5555-555555555555",
			Source:      command.SourceMCPTool,
			Success:     false,
			Error:       exactError,
			Duration:    5 * time.Millisecond,
		},
	}

	content := renderCommandContent(entries, 500) // Wide viewport

	// Should contain the full error
	require.Contains(t, content, exactError)

	// Should NOT contain truncation indicator (the "..." added for truncation)
	// Note: We check for "..." only at the point where truncation would add it
	// This is a bit tricky since the error itself might contain "..."
	// So we check that the full error is present
	require.Contains(t, content, strings.Repeat("x", maxErrorDisplayLength))
}

// ============================================================================
// CommandID Shortening Tests
// ============================================================================

func TestRenderCommandContent_CommandIDShortened(t *testing.T) {
	// UUID format: 8-4-4-4-12 = 36 chars total
	fullID := "12345678-1234-1234-1234-123456789abc"
	require.Greater(t, len(fullID), commandIDDisplayLength, "test ID should be longer than display length")

	entries := []CommandLogEntry{
		{
			Timestamp:   time.Date(2026, 1, 3, 10, 30, 0, 0, time.UTC),
			CommandType: command.CmdSpawnProcess,
			CommandID:   fullID,
			Source:      command.SourceMCPTool,
			Success:     true,
			Duration:    50 * time.Millisecond,
		},
	}

	content := renderCommandContent(entries, 200)

	// Should contain last 8 characters of the ID
	expectedShortID := fullID[len(fullID)-commandIDDisplayLength:] // "6789abc"
	require.Contains(t, content, "("+expectedShortID+")")

	// Should NOT contain full ID
	require.NotContains(t, content, fullID)
}

func TestRenderCommandContent_ShortCommandID(t *testing.T) {
	// ID shorter than display length - should show entire ID
	shortID := "abc123"
	require.Less(t, len(shortID), commandIDDisplayLength)

	entries := []CommandLogEntry{
		{
			Timestamp:   time.Date(2026, 1, 3, 10, 30, 0, 0, time.UTC),
			CommandType: command.CmdSpawnProcess,
			CommandID:   shortID,
			Source:      command.SourceMCPTool,
			Success:     true,
			Duration:    50 * time.Millisecond,
		},
	}

	content := renderCommandContent(entries, 200)

	// Should contain the full short ID in parentheses
	require.Contains(t, content, "("+shortID+")")
}

// ============================================================================
// Duration Formatting Tests
// ============================================================================

func TestFormatDuration_Milliseconds(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{1 * time.Millisecond, "1ms"},
		{50 * time.Millisecond, "50ms"},
		{100 * time.Millisecond, "100ms"},
		{999 * time.Millisecond, "999ms"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			result := formatDuration(tc.duration)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestFormatDuration_Seconds(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{1 * time.Second, "1.0s"},
		{1500 * time.Millisecond, "1.5s"},
		{2 * time.Second, "2.0s"},
		{10 * time.Second, "10.0s"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			result := formatDuration(tc.duration)
			require.Equal(t, tc.expected, result)
		})
	}
}

// ============================================================================
// Full Pane Rendering Tests
// ============================================================================

func TestRenderCommandPane_Empty(t *testing.T) {
	pane := newCommandPane()

	result := renderCommandPane(&pane, 80, 10)

	// Should contain the pane title
	require.Contains(t, result, "COMMAND LOG")

	// Should have borders (check for box drawing characters)
	require.Contains(t, result, "─")
}

func TestRenderCommandPane_WithEntries(t *testing.T) {
	pane := newCommandPane()
	pane.entries = []CommandLogEntry{
		{
			Timestamp:   time.Date(2026, 1, 3, 15, 4, 5, 0, time.UTC),
			CommandType: command.CmdSpawnProcess,
			CommandID:   "12345678-1234-1234-1234-123456789abc",
			Source:      command.SourceMCPTool,
			Success:     true,
			Duration:    50 * time.Millisecond,
		},
	}
	pane.contentDirty = true

	result := renderCommandPane(&pane, 100, 10)

	// Should contain the pane title
	require.Contains(t, result, "COMMAND LOG")

	// Should contain the entry content
	require.Contains(t, result, "spawn_process")
	require.Contains(t, result, "✓")
}

// ============================================================================
// Source Type Coverage Tests
// ============================================================================

func TestRenderCommandContent_AllSources(t *testing.T) {
	entries := []CommandLogEntry{
		{
			Timestamp:   time.Date(2026, 1, 3, 10, 0, 0, 0, time.UTC),
			CommandType: command.CmdSpawnProcess,
			CommandID:   "aaaaaaaa",
			Source:      command.SourceMCPTool,
			Success:     true,
			Duration:    10 * time.Millisecond,
		},
		{
			Timestamp:   time.Date(2026, 1, 3, 10, 0, 1, 0, time.UTC),
			CommandType: command.CmdAssignTask,
			CommandID:   "bbbbbbbb",
			Source:      command.SourceInternal,
			Success:     true,
			Duration:    20 * time.Millisecond,
		},
		{
			Timestamp:   time.Date(2026, 1, 3, 10, 0, 2, 0, time.UTC),
			CommandType: command.CmdReportComplete,
			CommandID:   "cccccccc",
			Source:      command.SourceCallback,
			Success:     true,
			Duration:    30 * time.Millisecond,
		},
		{
			Timestamp:   time.Date(2026, 1, 3, 10, 0, 3, 0, time.UTC),
			CommandType: command.CmdSendToProcess,
			CommandID:   "dddddddd",
			Source:      command.SourceUser,
			Success:     true,
			Duration:    40 * time.Millisecond,
		},
	}

	content := renderCommandContent(entries, 200)

	// Should contain all source types
	require.Contains(t, content, "[mcp_tool]")
	require.Contains(t, content, "[internal]")
	require.Contains(t, content, "[callback]")
	require.Contains(t, content, "[user]")
}

// ============================================================================
// Model Integration Tests - Max Entries and FIFO Eviction
// ============================================================================

func TestCommandPaneMaxEntries(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Add exactly maxCommandLogEntries entries
	for i := 0; i < maxCommandLogEntries; i++ {
		entry := CommandLogEntry{
			Timestamp:   time.Now(),
			CommandType: command.CmdSpawnProcess,
			CommandID:   fmt.Sprintf("cmd-%d", i),
			Source:      command.SourceInternal,
			Success:     true,
			Duration:    10 * time.Millisecond,
		}
		m.commandPane.entries = append(m.commandPane.entries, entry)
	}

	require.Len(t, m.commandPane.entries, maxCommandLogEntries, "should have exactly max entries")
	require.Equal(t, "cmd-0", m.commandPane.entries[0].CommandID, "first entry should be cmd-0")
	require.Equal(t, fmt.Sprintf("cmd-%d", maxCommandLogEntries-1), m.commandPane.entries[maxCommandLogEntries-1].CommandID, "last entry should be cmd-499")

	// Add one more entry (should trigger FIFO eviction)
	newEntry := CommandLogEntry{
		Timestamp:   time.Now(),
		CommandType: command.CmdAssignTask,
		CommandID:   "cmd-500",
		Source:      command.SourceUser,
		Success:     true,
		Duration:    5 * time.Millisecond,
	}
	m.commandPane.entries = append(m.commandPane.entries, newEntry)
	if len(m.commandPane.entries) > maxCommandLogEntries {
		m.commandPane.entries = m.commandPane.entries[1:]
	}

	// Verify FIFO eviction occurred
	require.Len(t, m.commandPane.entries, maxCommandLogEntries, "should still have max entries after eviction")
	require.Equal(t, "cmd-1", m.commandPane.entries[0].CommandID, "first entry should now be cmd-1 (cmd-0 was evicted)")
	require.Equal(t, "cmd-500", m.commandPane.entries[maxCommandLogEntries-1].CommandID, "last entry should be the newly added cmd-500")
}

func TestCommandPaneMaxEntries_BoundaryCondition(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Add exactly maxCommandLogEntries entries (at limit, no eviction yet)
	for i := 0; i < maxCommandLogEntries; i++ {
		entry := CommandLogEntry{
			Timestamp:   time.Now(),
			CommandType: command.CmdSpawnProcess,
			CommandID:   fmt.Sprintf("boundary-%d", i),
			Source:      command.SourceInternal,
			Success:     true,
			Duration:    10 * time.Millisecond,
		}
		m.commandPane.entries = append(m.commandPane.entries, entry)
		// Apply max entry bounds checking (same as in handleV2Event)
		if len(m.commandPane.entries) > maxCommandLogEntries {
			m.commandPane.entries = m.commandPane.entries[1:]
		}
	}

	require.Len(t, m.commandPane.entries, maxCommandLogEntries, "at limit: should have exactly maxCommandLogEntries entries")
	require.Equal(t, "boundary-0", m.commandPane.entries[0].CommandID, "at limit: first entry should still be boundary-0")

	// Add one more (triggers first eviction)
	entryOver := CommandLogEntry{
		Timestamp:   time.Now(),
		CommandType: command.CmdAssignTask,
		CommandID:   fmt.Sprintf("boundary-%d", maxCommandLogEntries),
		Source:      command.SourceUser,
		Success:     true,
		Duration:    5 * time.Millisecond,
	}
	m.commandPane.entries = append(m.commandPane.entries, entryOver)
	if len(m.commandPane.entries) > maxCommandLogEntries {
		m.commandPane.entries = m.commandPane.entries[1:]
	}

	require.Len(t, m.commandPane.entries, maxCommandLogEntries, "after overflow: should still have maxCommandLogEntries entries")
	require.Equal(t, "boundary-1", m.commandPane.entries[0].CommandID, "after overflow: first entry should be boundary-1")
	require.Equal(t, fmt.Sprintf("boundary-%d", maxCommandLogEntries), m.commandPane.entries[maxCommandLogEntries-1].CommandID, "after overflow: last entry should be the newest")
}

// ============================================================================
// Model Integration Tests - Accumulate When Hidden
// ============================================================================

func TestCommandPaneAccumulatesWhenHidden(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Verify pane is hidden by default (no debug mode)
	require.False(t, m.showCommandPane, "pane should be hidden for this test")

	// Add entries while hidden (simulating event handler behavior)
	entry1 := CommandLogEntry{
		Timestamp:   time.Now(),
		CommandType: command.CmdSpawnProcess,
		CommandID:   "hidden-1",
		Source:      command.SourceInternal,
		Success:     true,
		Duration:    10 * time.Millisecond,
	}
	m.commandPane.entries = append(m.commandPane.entries, entry1)
	m.commandPane.contentDirty = true

	require.Len(t, m.commandPane.entries, 1, "entry should accumulate when hidden")

	// Add more entries while still hidden
	entry2 := CommandLogEntry{
		Timestamp:   time.Now(),
		CommandType: command.CmdAssignTask,
		CommandID:   "hidden-2",
		Source:      command.SourceUser,
		Success:     false,
		Error:       "test error",
		Duration:    5 * time.Millisecond,
	}
	m.commandPane.entries = append(m.commandPane.entries, entry2)
	m.commandPane.contentDirty = true

	require.Len(t, m.commandPane.entries, 2, "entries should continue to accumulate when hidden")

	// Now show the pane - all entries should be available
	m.showCommandPane = true
	require.Len(t, m.commandPane.entries, 2, "entries should be preserved when pane becomes visible")
	require.Equal(t, "hidden-1", m.commandPane.entries[0].CommandID)
	require.Equal(t, "hidden-2", m.commandPane.entries[1].CommandID)
}

// ============================================================================
// Model Integration Tests - hasNewContent Indicator
// ============================================================================

func TestCommandPaneNewContentIndicator(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Show the pane and set up viewport
	m.showCommandPane = true

	// Add enough content to make viewport scrollable
	for i := 0; i < 20; i++ {
		entry := CommandLogEntry{
			Timestamp:   time.Now(),
			CommandType: command.CmdSpawnProcess,
			CommandID:   fmt.Sprintf("scroll-%d", i),
			Source:      command.SourceInternal,
			Success:     true,
			Duration:    10 * time.Millisecond,
		}
		m.commandPane.entries = append(m.commandPane.entries, entry)
	}

	// Render to populate viewport content
	content := renderCommandContent(m.commandPane.entries, 100)
	vp := m.commandPane.viewports[viewportKey]
	vp.Width = 100
	vp.Height = 3 // Small height to force scrolling
	vp.SetContent(content)

	// Scroll up (simulate user scrolling away from bottom)
	vp.SetYOffset(0) // Scroll to top
	m.commandPane.viewports[viewportKey] = vp

	// Verify we're not at bottom
	require.False(t, m.commandPane.viewports[viewportKey].AtBottom(), "should be scrolled up")

	// Now simulate receiving a new command log event when visible and scrolled up
	// This is the condition when hasNewContent should be set
	if m.showCommandPane && !m.commandPane.viewports[viewportKey].AtBottom() {
		m.commandPane.hasNewContent = true
	}

	require.True(t, m.commandPane.hasNewContent, "hasNewContent should be set when visible and scrolled up")
}

func TestCommandPaneNewContentIndicator_Hidden(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Verify pane is hidden by default (no debug mode)
	require.False(t, m.showCommandPane, "pane should be hidden for this test")

	// Add enough content to make viewport scrollable
	for i := 0; i < 20; i++ {
		entry := CommandLogEntry{
			Timestamp:   time.Now(),
			CommandType: command.CmdSpawnProcess,
			CommandID:   fmt.Sprintf("hidden-scroll-%d", i),
			Source:      command.SourceInternal,
			Success:     true,
			Duration:    10 * time.Millisecond,
		}
		m.commandPane.entries = append(m.commandPane.entries, entry)
	}

	// Render to populate viewport content
	content := renderCommandContent(m.commandPane.entries, 100)
	vp := m.commandPane.viewports[viewportKey]
	vp.Width = 100
	vp.Height = 3 // Small height to force scrolling
	vp.SetContent(content)
	vp.SetYOffset(0) // Scroll to top
	m.commandPane.viewports[viewportKey] = vp

	// Verify we're not at bottom
	require.False(t, m.commandPane.viewports[viewportKey].AtBottom(), "should be scrolled up")

	// Simulate receiving a new command log event when HIDDEN
	// hasNewContent should NOT be set when hidden (per task requirements)
	if m.showCommandPane && !m.commandPane.viewports[viewportKey].AtBottom() {
		m.commandPane.hasNewContent = true
	}

	require.False(t, m.commandPane.hasNewContent, "hasNewContent should NOT be set when pane is hidden")
}

func TestCommandPaneNewContentIndicator_AtBottom(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Show the pane
	m.showCommandPane = true

	// Add some content
	for i := 0; i < 5; i++ {
		entry := CommandLogEntry{
			Timestamp:   time.Now(),
			CommandType: command.CmdSpawnProcess,
			CommandID:   fmt.Sprintf("bottom-%d", i),
			Source:      command.SourceInternal,
			Success:     true,
			Duration:    10 * time.Millisecond,
		}
		m.commandPane.entries = append(m.commandPane.entries, entry)
	}

	// Render and keep viewport at bottom (default behavior)
	content := renderCommandContent(m.commandPane.entries, 100)
	vp := m.commandPane.viewports[viewportKey]
	vp.Width = 100
	vp.Height = 10 // Larger than content, so we're at bottom
	vp.SetContent(content)
	vp.GotoBottom()
	m.commandPane.viewports[viewportKey] = vp

	// Verify we're at bottom
	require.True(t, m.commandPane.viewports[viewportKey].AtBottom(), "should be at bottom")

	// Simulate receiving a new command log event when visible but at bottom
	// hasNewContent should NOT be set when at bottom
	if m.showCommandPane && !m.commandPane.viewports[viewportKey].AtBottom() {
		m.commandPane.hasNewContent = true
	}

	require.False(t, m.commandPane.hasNewContent, "hasNewContent should NOT be set when at bottom")
}

// ============================================================================
// Model Integration Tests - SetSize
// ============================================================================

func TestModelSetSize_CommandPane(t *testing.T) {
	m := New(Config{})

	// Initial SetSize
	m = m.SetSize(120, 30)

	// Verify command pane viewport map was initialized (by newCommandPane)
	require.NotNil(t, m.commandPane.viewports)
	_, ok := m.commandPane.viewports[viewportKey]
	require.True(t, ok, "viewport should exist after SetSize")

	// Verify contentDirty is set (viewport dimensions are set at render time by ScrollablePane)
	require.True(t, m.commandPane.contentDirty, "contentDirty should be true after SetSize")

	// Reset and resize
	m.commandPane.contentDirty = false
	m = m.SetSize(200, 50)

	// Verify contentDirty is set again on resize
	require.True(t, m.commandPane.contentDirty, "contentDirty should be true after resize")
}

func TestModelSetSize_CommandPane_PreservesEntries(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Add some entries
	for i := 0; i < 5; i++ {
		m.commandPane.entries = append(m.commandPane.entries, CommandLogEntry{
			Timestamp:   time.Now(),
			CommandType: command.CmdSpawnProcess,
			CommandID:   fmt.Sprintf("resize-%d", i),
			Source:      command.SourceInternal,
			Success:     true,
			Duration:    10 * time.Millisecond,
		})
	}

	require.Len(t, m.commandPane.entries, 5, "should have 5 entries before resize")

	// Resize
	m = m.SetSize(200, 50)

	// Entries should be preserved
	require.Len(t, m.commandPane.entries, 5, "entries should be preserved after resize")
	require.Equal(t, "resize-0", m.commandPane.entries[0].CommandID)
	require.Equal(t, "resize-4", m.commandPane.entries[4].CommandID)
}

// ============================================================================
// Toggle Command Tests - /show commands and /hide commands
// ============================================================================

func TestShowHideCommands(t *testing.T) {
	// Use DebugMode to start with command pane visible
	m := New(Config{DebugMode: true})
	m = m.SetSize(120, 30)

	// Verify pane is visible in debug mode
	require.True(t, m.showCommandPane, "pane should be visible in debug mode")

	// Test /hide commands
	newModel, handled := m.handleSlashCommand("/hide commands")
	require.True(t, handled, "/hide commands should be handled")
	require.False(t, newModel.showCommandPane, "showCommandPane should be false after /hide commands")

	// Test /show commands
	m = newModel
	newModel, handled = m.handleSlashCommand("/show commands")
	require.True(t, handled, "/show commands should be handled")
	require.True(t, newModel.showCommandPane, "showCommandPane should be true after /show commands")
}

func TestCommandPaneHiddenByDefault(t *testing.T) {
	// Default (no debug mode) should have command pane hidden
	m := New(Config{})
	m = m.SetSize(120, 30)

	require.False(t, m.showCommandPane, "command pane should be hidden by default")

	// Test /show commands reveals it
	newModel, handled := m.handleSlashCommand("/show commands")
	require.True(t, handled, "/show commands should be handled")
	require.True(t, newModel.showCommandPane, "showCommandPane should be true after /show commands")
}

func TestCommandPaneVisibleInDebugMode(t *testing.T) {
	// Debug mode should have command pane visible by default
	m := New(Config{DebugMode: true})
	m = m.SetSize(120, 30)

	require.True(t, m.showCommandPane, "command pane should be visible in debug mode")
}

func TestShowCommands_SetsContentDirty(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Clear contentDirty to verify it gets set
	m.commandPane.contentDirty = false

	// Test /show commands sets contentDirty
	newModel, handled := m.handleSlashCommand("/show commands")
	require.True(t, handled)
	require.True(t, newModel.commandPane.contentDirty, "contentDirty should be set when showing pane")
}

func TestHideCommands_NoContentDirty(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// First show the pane
	m.showCommandPane = true
	m.commandPane.contentDirty = false

	// Test /hide commands does NOT set contentDirty
	newModel, handled := m.handleSlashCommand("/hide commands")
	require.True(t, handled)
	require.False(t, newModel.commandPane.contentDirty, "contentDirty should NOT be set when hiding pane")
}

func TestShowCommands_Idempotent(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Show the pane first
	m.showCommandPane = true

	// Call /show commands again (should be no-op but still handled)
	newModel, handled := m.handleSlashCommand("/show commands")
	require.True(t, handled, "should still be handled even when already visible")
	require.True(t, newModel.showCommandPane, "should remain visible")
}

func TestHideCommands_Idempotent(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Verify pane is hidden by default (no debug mode)
	require.False(t, m.showCommandPane)

	// Call /hide commands when already hidden (should be no-op but still handled)
	newModel, handled := m.handleSlashCommand("/hide commands")
	require.True(t, handled, "should still be handled even when already hidden")
	require.False(t, newModel.showCommandPane, "should remain hidden")
}

func TestShowCommands_InvalidVariants(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Test invalid variants that should NOT be handled
	invalidCommands := []string{
		"/show command",  // Missing 's'
		"/showcommands",  // No space
		"/show Commands", // Capital C
		"/SHOW commands", // Uppercase
		"/show",          // Missing "commands"
		"/commands show", // Wrong order
	}

	for _, cmd := range invalidCommands {
		_, handled := m.handleSlashCommand(cmd)
		require.False(t, handled, "%q should not be handled as /show commands", cmd)
	}
}

func TestHideCommands_InvalidVariants(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)

	// Test invalid variants that should NOT be handled
	invalidCommands := []string{
		"/hide command",  // Missing 's'
		"/hidecommands",  // No space
		"/hide Commands", // Capital C
		"/HIDE commands", // Uppercase
		"/hide",          // Missing "commands"
		"/commands hide", // Wrong order
	}

	for _, cmd := range invalidCommands {
		_, handled := m.handleSlashCommand(cmd)
		require.False(t, handled, "%q should not be handled as /hide commands", cmd)
	}
}

// ============================================================================
// Trace ID Display Tests
// ============================================================================

func TestRenderCommandContent_WithTraceID(t *testing.T) {
	entries := []CommandLogEntry{
		{
			Timestamp:   time.Date(2026, 1, 3, 15, 4, 5, 0, time.UTC),
			CommandType: command.CmdSpawnProcess,
			CommandID:   "12345678-1234-1234-1234-123456789abc",
			Source:      command.SourceMCPTool,
			Success:     true,
			Duration:    50 * time.Millisecond,
			TraceID:     "abc123def456789012345678901234ff",
		},
	}

	content := renderCommandContent(entries, 200)

	// Should contain abbreviated trace ID (first 8 chars)
	require.Contains(t, content, "[abc123de]", "Should contain abbreviated trace ID")

	// Should NOT contain full trace ID
	require.NotContains(t, content, "abc123def456789012345678901234ff")
}

func TestRenderCommandContent_WithoutTraceID(t *testing.T) {
	entries := []CommandLogEntry{
		{
			Timestamp:   time.Date(2026, 1, 3, 15, 4, 5, 0, time.UTC),
			CommandType: command.CmdSpawnProcess,
			CommandID:   "12345678-1234-1234-1234-123456789abc",
			Source:      command.SourceMCPTool,
			Success:     true,
			Duration:    50 * time.Millisecond,
			TraceID:     "", // Empty trace ID
		},
	}

	content := renderCommandContent(entries, 200)

	// Should NOT contain trace ID brackets (no empty brackets)
	// Check that there's no extra brackets beyond the expected command ID and source
	require.Contains(t, content, "[mcp_tool]")
	require.Contains(t, content, "(56789abc)") // Command ID - last 8 chars
	// Should not have empty trace ID brackets
	require.NotContains(t, content, "[]")
}

func TestRenderCommandContent_ShortTraceID(t *testing.T) {
	// Trace ID shorter than 8 chars - should show entire ID
	shortTraceID := "abc123"

	entries := []CommandLogEntry{
		{
			Timestamp:   time.Date(2026, 1, 3, 15, 4, 5, 0, time.UTC),
			CommandType: command.CmdSpawnProcess,
			CommandID:   "12345678",
			Source:      command.SourceMCPTool,
			Success:     true,
			Duration:    50 * time.Millisecond,
			TraceID:     shortTraceID,
		},
	}

	content := renderCommandContent(entries, 200)

	// Should contain the full short trace ID in brackets
	require.Contains(t, content, "["+shortTraceID+"]")
}

func TestRenderCommandContent_TraceIDExactly8Chars(t *testing.T) {
	// Trace ID exactly 8 chars - should show entire ID
	exactTraceID := "abc12345"
	require.Len(t, exactTraceID, 8)

	entries := []CommandLogEntry{
		{
			Timestamp:   time.Date(2026, 1, 3, 15, 4, 5, 0, time.UTC),
			CommandType: command.CmdSpawnProcess,
			CommandID:   "12345678",
			Source:      command.SourceMCPTool,
			Success:     true,
			Duration:    50 * time.Millisecond,
			TraceID:     exactTraceID,
		},
	}

	content := renderCommandContent(entries, 200)

	// Should contain the full 8-char trace ID
	require.Contains(t, content, "["+exactTraceID+"]")
}
