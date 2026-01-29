package dashboard

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/mode"
	"github.com/zjrosen/perles/internal/orchestration/controlplane"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/fabric"
	fabricDomain "github.com/zjrosen/perles/internal/orchestration/fabric/domain"
	"github.com/zjrosen/perles/internal/orchestration/metrics"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
	"github.com/zjrosen/perles/internal/ui/shared/chatrender"
	"github.com/zjrosen/perles/internal/ui/shared/toaster"
)

func TestNewCoordinatorPanel(t *testing.T) {
	panel := NewCoordinatorPanel(false, false, nil)

	require.NotNil(t, panel)
	require.False(t, panel.IsFocused(), "panel should be unfocused by default")
	require.Empty(t, panel.coordinatorMessages)
	require.Equal(t, TabCoordinator, panel.activeTab)
}

func TestCoordinatorPanel_SetWorkflow(t *testing.T) {
	panel := NewCoordinatorPanel(false, false, nil)

	state := &WorkflowUIState{
		CoordinatorMessages: []chatrender.Message{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi there"},
		},
		CoordinatorStatus:     events.ProcessStatusWorking,
		CoordinatorQueueCount: 1,
	}

	panel.SetWorkflow("wf-123", state)

	require.Equal(t, controlplane.WorkflowID("wf-123"), panel.workflowID)
	require.Len(t, panel.coordinatorMessages, 2)
	require.Equal(t, events.ProcessStatusWorking, panel.coordinatorStatus)
	require.Equal(t, 1, panel.coordinatorQueue)
}

func TestCoordinatorPanel_SetWorkflow_SameWorkflowNewMessages(t *testing.T) {
	panel := NewCoordinatorPanel(false, false, nil)

	// Set initial state
	state := &WorkflowUIState{
		CoordinatorMessages: []chatrender.Message{
			{Role: "user", Content: "Hello"},
		},
		CoordinatorStatus: events.ProcessStatusReady,
	}
	panel.SetWorkflow("wf-123", state)

	// Add more messages
	state.CoordinatorMessages = append(state.CoordinatorMessages, chatrender.Message{Role: "assistant", Content: "Hi"})
	state.CoordinatorStatus = events.ProcessStatusWorking
	panel.SetWorkflow("wf-123", state)

	require.Len(t, panel.coordinatorMessages, 2)
	require.Equal(t, events.ProcessStatusWorking, panel.coordinatorStatus)
}

func TestCoordinatorPanel_Focus(t *testing.T) {
	panel := NewCoordinatorPanel(false, false, nil)
	panel.Blur()

	require.False(t, panel.IsFocused())

	panel.Focus()

	require.True(t, panel.IsFocused())
}

func TestCoordinatorPanel_SetSize(t *testing.T) {
	panel := NewCoordinatorPanel(false, false, nil)

	panel.SetSize(100, 50)

	require.Equal(t, 100, panel.width)
	require.Equal(t, 50, panel.height)
}

func TestCoordinatorPanel_View_EmptyMessages(t *testing.T) {
	panel := NewCoordinatorPanel(false, false, nil)
	panel.SetSize(80, 20)
	panel.SetWorkflow("wf-123", nil)

	view := panel.View()

	require.NotEmpty(t, view)
	require.Contains(t, view, "Coord", "should show Coordinator tab label")
	require.Contains(t, view, "Msgs", "should show Messages tab label")
}

func TestRenderChatContentWithSelection_EmptyMessages(t *testing.T) {
	cfg := chatrender.RenderConfig{
		AgentLabel: "Coordinator",
		AgentColor: chatrender.CoordinatorColor,
		UserLabel:  "User",
	}
	content, plainLines := renderChatContentWithSelection(nil, 80, cfg, nil, nil)

	require.Contains(t, content, "Waiting for the coordinator to initialize.")
	require.Nil(t, plainLines)
}

func TestRenderChatContentWithSelection_WithMessages(t *testing.T) {
	messages := []chatrender.Message{
		{Role: "user", Content: "Hello world"},
		{Role: "assistant", Content: "Hi there!"},
	}

	cfg := chatrender.RenderConfig{
		AgentLabel: "Coordinator",
		AgentColor: chatrender.CoordinatorColor,
		UserLabel:  "User",
	}
	content, plainLines := renderChatContentWithSelection(messages, 80, cfg, nil, nil)

	require.Contains(t, content, "User")
	require.Contains(t, content, "Hello world")
	require.Contains(t, content, "Coordinator") // Uses "Coordinator" label from RenderConfig
	require.Contains(t, content, "Hi there!")

	// Verify plain lines are returned for selection
	require.NotNil(t, plainLines)
	require.Contains(t, plainLines, "User")
	require.Contains(t, plainLines, "Hello world")
}

func TestRenderChatContentWithSelection_RendersToolCalls(t *testing.T) {
	messages := []chatrender.Message{
		{Role: "assistant", Content: "🔧 Read: file.go", IsToolCall: true},
		{Role: "assistant", Content: "🔧 Edit: config.yaml", IsToolCall: true},
	}

	cfg := chatrender.RenderConfig{
		AgentLabel: "Coordinator",
		AgentColor: chatrender.CoordinatorColor,
		UserLabel:  "User",
	}
	content, plainLines := renderChatContentWithSelection(messages, 80, cfg, nil, nil)

	// Tool calls should be rendered with tree-like prefixes
	require.Contains(t, content, "Coordinator") // Role label before first tool
	require.Contains(t, content, "├╴ Read: file.go")
	require.Contains(t, content, "╰╴ Edit: config.yaml") // Last tool uses end prefix
	require.NotNil(t, plainLines)
	require.Contains(t, plainLines, "├╴ Read: file.go")
	require.Contains(t, plainLines, "╰╴ Edit: config.yaml")
}

func TestRenderChatContentWithSelection_FiltersEmptyMessages(t *testing.T) {
	messages := []chatrender.Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: ""},    // Empty - should be filtered
		{Role: "assistant", Content: "Hi!"}, // Non-empty - should appear
	}

	cfg := chatrender.RenderConfig{
		AgentLabel: "Coordinator",
		AgentColor: chatrender.CoordinatorColor,
		UserLabel:  "User",
	}
	content, _ := renderChatContentWithSelection(messages, 80, cfg, nil, nil)

	require.Contains(t, content, "Hello")
	require.Contains(t, content, "Hi!")
	// Should not have empty lines from the filtered message
}

func TestNewCoordinatorPanel_InputStartsUnfocused(t *testing.T) {
	panel := NewCoordinatorPanel(false, false, nil)

	// Verify the input starts unfocused (focus is given on explicit Focus() call)
	require.False(t, panel.input.Focused())
	require.False(t, panel.focused)

	// After Focus(), both should be true
	panel.Focus()
	require.True(t, panel.input.Focused())
	require.True(t, panel.focused)
}

func TestCoordinatorPanel_TabNavigation(t *testing.T) {
	panel := NewCoordinatorPanel(false, false, nil)

	// Initially on TabCoordinator
	require.Equal(t, TabCoordinator, panel.ActiveTab())

	// Tab forward
	panel.NextTab()
	require.Equal(t, TabMessages, panel.ActiveTab())

	// Tab backward
	panel.PrevTab()
	require.Equal(t, TabCoordinator, panel.ActiveTab())

	// Tab wraps around
	panel.PrevTab()
	require.Equal(t, TabMessages, panel.ActiveTab(), "should wrap to last tab")
}

func TestCoordinatorPanel_TabNavigationDebugMode(t *testing.T) {
	panel := NewCoordinatorPanel(true, false, nil) // debug mode, no vim

	// Initially on TabCoordinator
	require.Equal(t, TabCoordinator, panel.ActiveTab())

	// Tab forward through: Coordinator -> Messages -> CmdLog
	panel.NextTab()
	require.Equal(t, TabMessages, panel.ActiveTab())
	panel.NextTab()
	require.Equal(t, 2, panel.ActiveTab(), "should be on command log tab")

	// Tab wraps back to coordinator
	panel.NextTab()
	require.Equal(t, TabCoordinator, panel.ActiveTab())

	// Tab backward from coordinator wraps to command log
	panel.PrevTab()
	require.Equal(t, 2, panel.ActiveTab(), "should wrap to command log tab")
}

func TestCoordinatorPanel_TabNavigationWithWorkers(t *testing.T) {
	panel := NewCoordinatorPanel(false, false, nil)

	// Set workflow with workers
	state := &WorkflowUIState{
		WorkerIDs:         []string{"worker-1", "worker-2"},
		WorkerStatus:      make(map[string]events.ProcessStatus),
		WorkerPhases:      make(map[string]events.ProcessPhase),
		WorkerMessages:    make(map[string][]chatrender.Message),
		WorkerQueueCounts: make(map[string]int),
	}
	panel.SetWorkflow("wf-123", state)

	// Should now have 4 tabs: Coord, Msgs, W1, W2
	require.Equal(t, 4, panel.tabCount())

	// Navigate through all tabs
	require.Equal(t, TabCoordinator, panel.ActiveTab())
	panel.NextTab()
	require.Equal(t, TabMessages, panel.ActiveTab())
	panel.NextTab()
	require.Equal(t, TabFirstWorker, panel.ActiveTab()) // worker-1
	panel.NextTab()
	require.Equal(t, TabFirstWorker+1, panel.ActiveTab()) // worker-2
	panel.NextTab()
	require.Equal(t, TabCoordinator, panel.ActiveTab(), "should wrap back to coordinator")
}

func TestCoordinatorPanel_SetWorkflow_SyncsWorkerData(t *testing.T) {
	panel := NewCoordinatorPanel(false, false, nil)

	state := &WorkflowUIState{
		WorkerIDs: []string{"worker-1", "worker-2"},
		WorkerStatus: map[string]events.ProcessStatus{
			"worker-1": events.ProcessStatusWorking,
			"worker-2": events.ProcessStatusReady,
		},
		WorkerPhases: map[string]events.ProcessPhase{
			"worker-1": events.ProcessPhaseImplementing,
		},
		WorkerMessages: map[string][]chatrender.Message{
			"worker-1": {{Role: "assistant", Content: "Hello from worker"}},
		},
		WorkerQueueCounts: map[string]int{
			"worker-1": 2,
		},
	}
	panel.SetWorkflow("wf-123", state)

	require.Len(t, panel.workerIDs, 2)
	require.Equal(t, events.ProcessStatusWorking, panel.workerStatus["worker-1"])
	require.Equal(t, events.ProcessPhaseImplementing, panel.workerPhases["worker-1"])
	require.Len(t, panel.workerMessages["worker-1"], 1)
	require.Equal(t, 2, panel.workerQueues["worker-1"])
}

func TestCoordinatorPanel_SetWorkflow_ResetsTabWhenWorkerRemoved(t *testing.T) {
	panel := NewCoordinatorPanel(false, false, nil)

	// Initial state with workers
	state := &WorkflowUIState{
		WorkerIDs:         []string{"worker-1", "worker-2"},
		WorkerStatus:      make(map[string]events.ProcessStatus),
		WorkerPhases:      make(map[string]events.ProcessPhase),
		WorkerMessages:    make(map[string][]chatrender.Message),
		WorkerQueueCounts: make(map[string]int),
	}
	panel.SetWorkflow("wf-123", state)

	// Navigate to worker-2 tab
	panel.activeTab = TabFirstWorker + 1 // worker-2

	// Remove workers
	state.WorkerIDs = nil
	panel.SetWorkflow("wf-123", state)

	// Should reset to coordinator since worker tab no longer exists
	require.Equal(t, TabCoordinator, panel.activeTab)
}

func TestCoordinatorPanel_FormatWorkerTabLabel(t *testing.T) {
	panel := NewCoordinatorPanel(false, false, nil)

	require.Equal(t, "W1", panel.formatWorkerTabLabel("worker-1"))
	require.Equal(t, "W99", panel.formatWorkerTabLabel("worker-99"))
	require.Equal(t, "custom", panel.formatWorkerTabLabel("custom"))
	require.Equal(t, "longla", panel.formatWorkerTabLabel("longlabel")) // truncates to 6 chars
}

func TestSetWorkflow_SyncsMetrics(t *testing.T) {
	panel := NewCoordinatorPanel(false, false, nil)

	coordinatorMetrics := &metrics.TokenMetrics{
		TokensUsed:  27000,
		TotalTokens: 200000,
	}
	workerMetrics := map[string]*metrics.TokenMetrics{
		"worker-1": {TokensUsed: 15000, TotalTokens: 200000},
		"worker-2": {TokensUsed: 8000, TotalTokens: 200000},
	}

	state := &WorkflowUIState{
		CoordinatorMetrics: coordinatorMetrics,
		WorkerIDs:          []string{"worker-1", "worker-2"},
		WorkerStatus:       make(map[string]events.ProcessStatus),
		WorkerPhases:       make(map[string]events.ProcessPhase),
		WorkerMessages:     make(map[string][]chatrender.Message),
		WorkerMetrics:      workerMetrics,
		WorkerQueueCounts:  make(map[string]int),
	}

	panel.SetWorkflow("wf-123", state)

	// Verify coordinator metrics synced
	require.Equal(t, coordinatorMetrics, panel.coordinatorMetrics)
	require.Equal(t, 27000, panel.coordinatorMetrics.TokensUsed)
	require.Equal(t, 200000, panel.coordinatorMetrics.TotalTokens)

	// Verify worker metrics synced
	require.Len(t, panel.workerMetrics, 2)
	require.Equal(t, 15000, panel.workerMetrics["worker-1"].TokensUsed)
	require.Equal(t, 8000, panel.workerMetrics["worker-2"].TokensUsed)
}

func TestSetWorkflow_ClearsStaleMetrics(t *testing.T) {
	panel := NewCoordinatorPanel(false, false, nil)

	// First workflow with worker-1 and worker-2
	state1 := &WorkflowUIState{
		CoordinatorMetrics: &metrics.TokenMetrics{TokensUsed: 10000, TotalTokens: 200000},
		WorkerIDs:          []string{"worker-1", "worker-2"},
		WorkerStatus:       make(map[string]events.ProcessStatus),
		WorkerPhases:       make(map[string]events.ProcessPhase),
		WorkerMessages:     make(map[string][]chatrender.Message),
		WorkerMetrics: map[string]*metrics.TokenMetrics{
			"worker-1": {TokensUsed: 5000, TotalTokens: 200000},
			"worker-2": {TokensUsed: 3000, TotalTokens: 200000},
		},
		WorkerQueueCounts: make(map[string]int),
	}
	panel.SetWorkflow("wf-1", state1)

	// Verify first workflow metrics
	require.Len(t, panel.workerMetrics, 2)
	require.NotNil(t, panel.workerMetrics["worker-1"])
	require.NotNil(t, panel.workerMetrics["worker-2"])

	// Second workflow with only worker-3 (different set of workers)
	state2 := &WorkflowUIState{
		CoordinatorMetrics: &metrics.TokenMetrics{TokensUsed: 20000, TotalTokens: 200000},
		WorkerIDs:          []string{"worker-3"},
		WorkerStatus:       make(map[string]events.ProcessStatus),
		WorkerPhases:       make(map[string]events.ProcessPhase),
		WorkerMessages:     make(map[string][]chatrender.Message),
		WorkerMetrics: map[string]*metrics.TokenMetrics{
			"worker-3": {TokensUsed: 7000, TotalTokens: 200000},
		},
		WorkerQueueCounts: make(map[string]int),
	}
	panel.SetWorkflow("wf-2", state2)

	// Verify old workers' metrics are cleared and new worker metrics are set
	require.Len(t, panel.workerMetrics, 1, "should only have 1 worker metrics after switching workflows")
	require.Nil(t, panel.workerMetrics["worker-1"], "worker-1 metrics should be cleared")
	require.Nil(t, panel.workerMetrics["worker-2"], "worker-2 metrics should be cleared")
	require.NotNil(t, panel.workerMetrics["worker-3"], "worker-3 metrics should be set")
	require.Equal(t, 7000, panel.workerMetrics["worker-3"].TokensUsed)

	// Verify coordinator metrics updated
	require.Equal(t, 20000, panel.coordinatorMetrics.TokensUsed)
}

func TestGetActiveMetricsDisplay_Coordinator(t *testing.T) {
	panel := NewCoordinatorPanel(false, false, nil)

	// Set up coordinator with metrics
	state := &WorkflowUIState{
		CoordinatorMetrics: &metrics.TokenMetrics{
			TokensUsed:  27000,
			TotalTokens: 200000,
		},
	}
	panel.SetWorkflow("wf-123", state)
	panel.activeTab = TabCoordinator

	result := panel.getActiveMetricsDisplay()

	// FormatMetricsDisplay returns formatted string like "27k/200k"
	require.NotEmpty(t, result)
	require.Contains(t, result, "27k")
	require.Contains(t, result, "200k")
}

func TestGetActiveMetricsDisplay_Worker(t *testing.T) {
	panel := NewCoordinatorPanel(false, false, nil)

	// Set up with workers and metrics
	state := &WorkflowUIState{
		WorkerIDs:    []string{"worker-1", "worker-2"},
		WorkerStatus: make(map[string]events.ProcessStatus),
		WorkerPhases: make(map[string]events.ProcessPhase),
		WorkerMetrics: map[string]*metrics.TokenMetrics{
			"worker-1": {TokensUsed: 15000, TotalTokens: 200000},
			"worker-2": {TokensUsed: 8000, TotalTokens: 200000},
		},
		WorkerMessages:    make(map[string][]chatrender.Message),
		WorkerQueueCounts: make(map[string]int),
	}
	panel.SetWorkflow("wf-123", state)

	// Select worker-1 tab (TabFirstWorker + 0)
	panel.activeTab = TabFirstWorker

	result := panel.getActiveMetricsDisplay()

	// Should show worker-1's metrics (15k/200k)
	require.NotEmpty(t, result)
	require.Contains(t, result, "15k")
	require.Contains(t, result, "200k")

	// Select worker-2 tab (TabFirstWorker + 1)
	panel.activeTab = TabFirstWorker + 1

	result = panel.getActiveMetricsDisplay()

	// Should show worker-2's metrics (8k/200k)
	require.NotEmpty(t, result)
	require.Contains(t, result, "8k")
	require.Contains(t, result, "200k")
}

func TestGetActiveMetricsDisplay_Messages(t *testing.T) {
	panel := NewCoordinatorPanel(false, false, nil)

	// Set up with coordinator metrics
	state := &WorkflowUIState{
		CoordinatorMetrics: &metrics.TokenMetrics{
			TokensUsed:  27000,
			TotalTokens: 200000,
		},
	}
	panel.SetWorkflow("wf-123", state)

	// Select messages tab
	panel.activeTab = TabMessages

	result := panel.getActiveMetricsDisplay()

	// Should return empty string for message log tab
	require.Empty(t, result)
}

func TestGetActiveMetricsDisplay_NilMetrics(t *testing.T) {
	panel := NewCoordinatorPanel(false, false, nil)

	// Set up without any metrics (nil)
	state := &WorkflowUIState{
		CoordinatorMetrics: nil,
		WorkerIDs:          []string{"worker-1"},
		WorkerStatus:       make(map[string]events.ProcessStatus),
		WorkerPhases:       make(map[string]events.ProcessPhase),
		WorkerMessages:     make(map[string][]chatrender.Message),
		WorkerMetrics:      nil, // nil map
		WorkerQueueCounts:  make(map[string]int),
	}
	panel.SetWorkflow("wf-123", state)

	// Coordinator tab with nil metrics
	panel.activeTab = TabCoordinator
	result := panel.getActiveMetricsDisplay()
	require.Empty(t, result, "should return empty string for nil coordinator metrics")

	// Worker tab with nil metrics map
	panel.activeTab = TabFirstWorker
	result = panel.getActiveMetricsDisplay()
	require.Empty(t, result, "should return empty string for nil worker metrics")
}

func TestGetActiveMetricsDisplay_InvalidWorkerTab(t *testing.T) {
	panel := NewCoordinatorPanel(false, false, nil)

	// Set up with only one worker
	state := &WorkflowUIState{
		WorkerIDs:    []string{"worker-1"},
		WorkerStatus: make(map[string]events.ProcessStatus),
		WorkerPhases: make(map[string]events.ProcessPhase),
		WorkerMetrics: map[string]*metrics.TokenMetrics{
			"worker-1": {TokensUsed: 15000, TotalTokens: 200000},
		},
		WorkerMessages:    make(map[string][]chatrender.Message),
		WorkerQueueCounts: make(map[string]int),
	}
	panel.SetWorkflow("wf-123", state)

	// Try to access worker tab index that doesn't exist (worker-2 at index 1)
	panel.activeTab = TabFirstWorker + 5 // Invalid index

	result := panel.getActiveMetricsDisplay()

	// Should return empty string for invalid worker index (no panic)
	require.Empty(t, result)
}

func TestView_ShowsMetricsInBottomRight(t *testing.T) {
	panel := NewCoordinatorPanel(false, false, nil)
	panel.SetSize(60, 20)

	// Set up coordinator with metrics
	state := &WorkflowUIState{
		CoordinatorMetrics: &metrics.TokenMetrics{
			TokensUsed:  27000,
			TotalTokens: 200000,
		},
		CoordinatorStatus: events.ProcessStatusWorking,
	}
	panel.SetWorkflow("wf-123", state)
	panel.activeTab = TabCoordinator

	view := panel.View()

	// Verify the metrics string appears in the rendered output
	// FormatMetricsDisplay returns "27k/200k" for these values
	require.Contains(t, view, "27k/200k", "metrics should appear in View() output")
}

func TestView_MetricsFitInPanelWidth(t *testing.T) {
	panel := NewCoordinatorPanel(false, false, nil)

	// Use exactly 60-char width as specified in task
	panel.SetSize(60, 20)

	// Set up with both queue count (BottomLeft) and metrics (BottomRight)
	state := &WorkflowUIState{
		CoordinatorMetrics: &metrics.TokenMetrics{
			TokensUsed:  27000,
			TotalTokens: 200000,
		},
		CoordinatorStatus:     events.ProcessStatusWorking,
		CoordinatorQueueCount: 3, // Will show "[3 queued]" in BottomLeft
	}
	panel.SetWorkflow("wf-123", state)
	panel.activeTab = TabCoordinator

	view := panel.View()

	// Verify both queue count and metrics appear without truncation
	// FormatQueueCount returns "[N queued]" format
	require.Contains(t, view, "[3 queued]", "queue count should appear in BottomLeft")
	require.Contains(t, view, "27k/200k", "metrics should appear in BottomRight")

	// Verify no line exceeds panel width (basic overflow check)
	lines := splitLines(view)
	for _, line := range lines {
		// Use visual width (ANSI codes don't count toward visual width)
		// We just verify no obvious overflow - actual visual rendering is what matters
		require.LessOrEqual(t, visualWidth(line), 60,
			"no line should exceed panel width of 60 characters")
	}
}

// splitLines splits a string into lines
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// visualWidth calculates the visual width of a string, ignoring ANSI escape codes
func visualWidth(s string) int {
	width := 0
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		width++
	}
	return width
}

// ============================================================================
// Slash Command Tests
// ============================================================================

func TestHandleSlashCommand_Stop_Valid(t *testing.T) {
	m := Model{}
	workflowID := controlplane.WorkflowID("wf-123")

	newM, cmd := m.handleSlashCommand(workflowID, "/stop worker-1")

	require.NotNil(t, newM)
	require.NotNil(t, cmd)
}

func TestHandleSlashCommand_Stop_WithForce(t *testing.T) {
	m := Model{}
	workflowID := controlplane.WorkflowID("wf-123")

	newM, cmd := m.handleSlashCommand(workflowID, "/stop coordinator --force")

	require.NotNil(t, newM)
	require.NotNil(t, cmd)
}

func TestHandleSlashCommand_Stop_MissingProcessID(t *testing.T) {
	m := Model{}
	workflowID := controlplane.WorkflowID("wf-123")

	newM, cmd := m.handleSlashCommand(workflowID, "/stop")

	require.NotNil(t, newM)
	require.NotNil(t, cmd)
	// Should return a warning toast
	msg := cmd()
	toastMsg, ok := msg.(mode.ShowToastMsg)
	require.True(t, ok, "expected ShowToastMsg, got %T", msg)
	require.Contains(t, toastMsg.Message, "Usage:")
}

func TestHandleSlashCommand_Spawn(t *testing.T) {
	m := Model{}
	workflowID := controlplane.WorkflowID("wf-123")

	newM, cmd := m.handleSlashCommand(workflowID, "/spawn")

	require.NotNil(t, newM)
	require.NotNil(t, cmd)
}

func TestHandleSlashCommand_Retire_Valid(t *testing.T) {
	m := Model{}
	workflowID := controlplane.WorkflowID("wf-123")

	newM, cmd := m.handleSlashCommand(workflowID, "/retire worker-1")

	require.NotNil(t, newM)
	require.NotNil(t, cmd)
}

func TestHandleSlashCommand_Retire_WithReason(t *testing.T) {
	m := Model{}
	workflowID := controlplane.WorkflowID("wf-123")

	newM, cmd := m.handleSlashCommand(workflowID, "/retire worker-1 task completed")

	require.NotNil(t, newM)
	require.NotNil(t, cmd)
}

func TestHandleSlashCommand_Retire_MissingWorkerID(t *testing.T) {
	m := Model{}
	workflowID := controlplane.WorkflowID("wf-123")

	newM, cmd := m.handleSlashCommand(workflowID, "/retire")

	require.NotNil(t, newM)
	require.NotNil(t, cmd)
	// Should return a warning toast
	msg := cmd()
	toastMsg, ok := msg.(mode.ShowToastMsg)
	require.True(t, ok, "expected ShowToastMsg, got %T", msg)
	require.Contains(t, toastMsg.Message, "Usage:")
}

func TestHandleSlashCommand_Retire_CannotRetireCoordinator(t *testing.T) {
	m := Model{}
	workflowID := controlplane.WorkflowID("wf-123")

	newM, cmd := m.handleSlashCommand(workflowID, "/retire coordinator")

	require.NotNil(t, newM)
	require.NotNil(t, cmd)
	// Should return a warning toast about not retiring coordinator
	msg := cmd()
	toastMsg, ok := msg.(mode.ShowToastMsg)
	require.True(t, ok, "expected ShowToastMsg, got %T", msg)
	require.Contains(t, toastMsg.Message, "Cannot retire coordinator")
}

func TestHandleSlashCommand_Replace_Valid(t *testing.T) {
	m := Model{}
	workflowID := controlplane.WorkflowID("wf-123")

	newM, cmd := m.handleSlashCommand(workflowID, "/replace worker-1")

	require.NotNil(t, newM)
	require.NotNil(t, cmd)
}

func TestHandleSlashCommand_Replace_Coordinator(t *testing.T) {
	m := Model{}
	workflowID := controlplane.WorkflowID("wf-123")

	// Unlike /retire, /replace coordinator IS allowed
	newM, cmd := m.handleSlashCommand(workflowID, "/replace coordinator")

	require.NotNil(t, newM)
	require.NotNil(t, cmd)
}

func TestHandleSlashCommand_Replace_WithReason(t *testing.T) {
	m := Model{}
	workflowID := controlplane.WorkflowID("wf-123")

	newM, cmd := m.handleSlashCommand(workflowID, "/replace worker-1 needs fresh context")

	require.NotNil(t, newM)
	require.NotNil(t, cmd)
}

func TestHandleSlashCommand_Replace_MissingProcessID(t *testing.T) {
	m := Model{}
	workflowID := controlplane.WorkflowID("wf-123")

	newM, cmd := m.handleSlashCommand(workflowID, "/replace")

	require.NotNil(t, newM)
	require.NotNil(t, cmd)
	// Should return a warning toast
	msg := cmd()
	toastMsg, ok := msg.(mode.ShowToastMsg)
	require.True(t, ok, "expected ShowToastMsg, got %T", msg)
	require.Contains(t, toastMsg.Message, "Usage:")
}

func TestHandleSlashCommand_UnknownCommand_PassedToCoordinator(t *testing.T) {
	m := Model{}
	workflowID := controlplane.WorkflowID("wf-123")

	// Unknown slash commands should be passed through to coordinator
	newM, cmd := m.handleSlashCommand(workflowID, "/unknown command")

	require.NotNil(t, newM)
	// Should return sendToCoordinator command (not a toast)
	require.NotNil(t, cmd)
}

func TestHandleSlashCommand_EmptyContent(t *testing.T) {
	m := Model{}
	workflowID := controlplane.WorkflowID("wf-123")

	newM, cmd := m.handleSlashCommand(workflowID, "")

	require.NotNil(t, newM)
	require.Nil(t, cmd)
}

func TestShowWarning_ReturnsToastMsg(t *testing.T) {
	cmd := showWarning("test warning message")

	require.NotNil(t, cmd)
	msg := cmd()
	toastMsg, ok := msg.(mode.ShowToastMsg)
	require.True(t, ok, "expected ShowToastMsg, got %T", msg)
	require.Equal(t, "test warning message", toastMsg.Message)
	require.Equal(t, toaster.StyleWarn, toastMsg.Style)
}

// ============================================================================
// Slash Command - Command Content Verification Tests
// ============================================================================

// captureSubmitter is a test helper that captures commands submitted to it.
type captureSubmitter struct {
	commands []command.Command
}

func (c *captureSubmitter) Submit(cmd command.Command) {
	c.commands = append(c.commands, cmd)
}

func TestStopCommand_CreatesCorrectCommand(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantProcessID string
		wantForce     bool
	}{
		{
			name:          "stop worker",
			input:         "/stop worker-1",
			wantProcessID: "worker-1",
			wantForce:     false,
		},
		{
			name:          "stop coordinator",
			input:         "/stop coordinator",
			wantProcessID: "coordinator",
			wantForce:     false,
		},
		{
			name:          "stop with force",
			input:         "/stop worker-1 --force",
			wantProcessID: "worker-1",
			wantForce:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capture := &captureSubmitter{}
			parts := strings.Fields(tt.input)

			processID := parts[1]
			force := len(parts) > 2 && parts[2] == "--force"

			cmd := command.NewStopProcessCommand(command.SourceUser, processID, force, "user_requested")
			capture.Submit(cmd)

			require.Len(t, capture.commands, 1)
			stopCmd, ok := capture.commands[0].(*command.StopProcessCommand)
			require.True(t, ok)
			require.Equal(t, tt.wantProcessID, stopCmd.ProcessID)
			require.Equal(t, tt.wantForce, stopCmd.Force)
			require.Equal(t, "user_requested", stopCmd.Reason)
			require.Equal(t, command.SourceUser, stopCmd.Source())
		})
	}
}

func TestSpawnCommand_CreatesCorrectCommand(t *testing.T) {
	capture := &captureSubmitter{}

	cmd := command.NewSpawnProcessCommand(command.SourceUser, repository.RoleWorker)
	capture.Submit(cmd)

	require.Len(t, capture.commands, 1)
	spawnCmd, ok := capture.commands[0].(*command.SpawnProcessCommand)
	require.True(t, ok)
	require.Equal(t, repository.RoleWorker, spawnCmd.Role)
	require.Equal(t, command.SourceUser, spawnCmd.Source())
}

func TestRetireCommand_CreatesCorrectCommand(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantWorkerID string
		wantReason   string
	}{
		{
			name:         "retire with default reason",
			input:        "/retire worker-1",
			wantWorkerID: "worker-1",
			wantReason:   "user_requested",
		},
		{
			name:         "retire with custom reason",
			input:        "/retire worker-2 task completed",
			wantWorkerID: "worker-2",
			wantReason:   "task completed",
		},
		{
			name:         "retire with multi-word reason",
			input:        "/retire worker-3 finished all assigned tasks",
			wantWorkerID: "worker-3",
			wantReason:   "finished all assigned tasks",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capture := &captureSubmitter{}
			parts := strings.Fields(tt.input)

			workerID := parts[1]
			reason := "user_requested"
			if len(parts) > 2 {
				reason = strings.Join(parts[2:], " ")
			}

			cmd := command.NewRetireProcessCommand(command.SourceUser, workerID, reason)
			capture.Submit(cmd)

			require.Len(t, capture.commands, 1)
			retireCmd, ok := capture.commands[0].(*command.RetireProcessCommand)
			require.True(t, ok)
			require.Equal(t, tt.wantWorkerID, retireCmd.ProcessID)
			require.Equal(t, tt.wantReason, retireCmd.Reason)
			require.Equal(t, command.SourceUser, retireCmd.Source())
		})
	}
}

func TestReplaceCommand_CreatesCorrectCommand(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantProcessID string
		wantReason    string
	}{
		{
			name:          "replace worker",
			input:         "/replace worker-1",
			wantProcessID: "worker-1",
			wantReason:    "user_requested",
		},
		{
			name:          "replace coordinator",
			input:         "/replace coordinator",
			wantProcessID: "coordinator",
			wantReason:    "user_requested",
		},
		{
			name:          "replace with reason",
			input:         "/replace worker-1 needs fresh context",
			wantProcessID: "worker-1",
			wantReason:    "needs fresh context",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capture := &captureSubmitter{}
			parts := strings.Fields(tt.input)

			processID := parts[1]
			reason := "user_requested"
			if len(parts) > 2 {
				reason = strings.Join(parts[2:], " ")
			}

			cmd := command.NewReplaceProcessCommand(command.SourceUser, processID, reason)
			capture.Submit(cmd)

			require.Len(t, capture.commands, 1)
			replaceCmd, ok := capture.commands[0].(*command.ReplaceProcessCommand)
			require.True(t, ok)
			require.Equal(t, tt.wantProcessID, replaceCmd.ProcessID)
			require.Equal(t, tt.wantReason, replaceCmd.Reason)
			require.Equal(t, command.SourceUser, replaceCmd.Source())
		})
	}
}

// ============================================================================
// Clipboard Wiring Tests
// ============================================================================

func TestCoordinatorPanel_HasClipboardField(t *testing.T) {
	panel := NewCoordinatorPanel(false, false, nil)

	// Verify clipboard field exists and is nil by default (not set by constructor)
	require.Nil(t, panel.clipboard, "clipboard should be nil by default (set by parent)")
}

func TestCoordinatorPanel_ClipboardCanBeSet(t *testing.T) {
	panel := NewCoordinatorPanel(false, false, nil)

	// Create a mock clipboard
	mockClipboard := &mockClipboardForTest{}

	// Set the clipboard
	panel.clipboard = mockClipboard

	// Verify it's set
	require.NotNil(t, panel.clipboard, "clipboard should be set after assignment")
	require.Same(t, mockClipboard, panel.clipboard, "clipboard should be the same instance")
}

// mockClipboardForTest is a simple mock clipboard for testing.
type mockClipboardForTest struct {
	lastCopiedText string
}

func (m *mockClipboardForTest) Copy(text string) error {
	m.lastCopiedText = text
	return nil
}

// mockClipboardWithError is a mock clipboard that returns an error on Copy.
type mockClipboardWithError struct {
	err error
}

func (m *mockClipboardWithError) Copy(_ string) error {
	return m.err
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestCoordinatorPanel_SelectionNilClipboard(t *testing.T) {
	// Test that selection with nil clipboard does not crash
	panel := NewCoordinatorPanel(false, false, nil)
	// clipboard is nil by default
	panel.SetSize(100, 30)
	panel.SetScreenXOffset(0)

	state := &WorkflowUIState{
		CoordinatorMessages: []chatrender.Message{
			{Role: "user", Content: "Test message"},
		},
		CoordinatorStatus: events.ProcessStatusReady,
	}
	panel.SetWorkflow("wf-123", state)
	panel.activeTab = TabCoordinator

	// Render to populate plain lines
	_ = panel.View()

	// Simulate selection
	pressMsg := tea.MouseMsg{X: 10, Y: 5, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	panel.Update(pressMsg)

	releaseMsg := tea.MouseMsg{X: 20, Y: 5, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease}
	_, cmd := panel.Update(releaseMsg)

	// No toast should be returned when clipboard is nil
	require.Nil(t, cmd, "should not return toast when clipboard is nil")
}

func TestCoordinatorPanel_ScrollAfterSelection(t *testing.T) {
	// Verify scroll behavior works after selection
	panel := NewCoordinatorPanel(false, false, nil)
	mockClip := &mockClipboardForTest{}
	panel.clipboard = mockClip
	panel.SetSize(100, 30)

	// Set up messages with enough content to scroll
	var messages []chatrender.Message
	for i := 0; i < 20; i++ {
		messages = append(messages, chatrender.Message{
			Role:    "assistant",
			Content: fmt.Sprintf("Message number %d with some content", i),
		})
	}

	state := &WorkflowUIState{
		CoordinatorMessages: messages,
		CoordinatorStatus:   events.ProcessStatusReady,
	}
	panel.SetWorkflow("wf-123", state)
	panel.activeTab = TabCoordinator

	// Render to initialize viewport
	_ = panel.View()

	// Viewport starts at bottom due to padContentToBottom, so scroll up first
	scrollUp := tea.MouseMsg{
		Button: tea.MouseButtonWheelUp,
		Action: tea.MouseActionPress,
	}
	panel.Update(scrollUp)

	// Record position after scroll up
	offsetAfterScrollUp := panel.coordinatorPane.YOffset()

	// Now scroll down - should still work
	scrollDown := tea.MouseMsg{
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
	}
	panel.Update(scrollDown)

	// Viewport should scroll down
	require.Greater(t, panel.coordinatorPane.YOffset(), offsetAfterScrollUp,
		"scroll down should increase YOffset")

	// Scroll back up - should still work
	panel.Update(scrollUp)

	// Viewport should scroll up
	require.Equal(t, offsetAfterScrollUp, panel.coordinatorPane.YOffset(),
		"scroll up should decrease YOffset back to previous position")
}

func TestCoordinatorPanel_TabSwitchAfterSelection(t *testing.T) {
	// Verify tab switching still works after selection
	panel := NewCoordinatorPanel(false, false, nil)
	mockClip := &mockClipboardForTest{}
	panel.clipboard = mockClip
	panel.SetSize(100, 30)

	state := &WorkflowUIState{
		CoordinatorMessages: []chatrender.Message{
			{Role: "user", Content: "Test message"},
		},
		CoordinatorStatus: events.ProcessStatusReady,
	}
	panel.SetWorkflow("wf-123", state)

	// Verify initial state
	require.Equal(t, TabCoordinator, panel.ActiveTab())

	// Switch tab using ctrl+j key
	tabNext := tea.KeyMsg{Type: tea.KeyCtrlJ}
	panel.Update(tabNext)

	// Tab should switch to messages
	require.Equal(t, TabMessages, panel.ActiveTab(),
		"tab should switch")

	// Switch back using ctrl+k key
	tabPrev := tea.KeyMsg{Type: tea.KeyCtrlK}
	panel.Update(tabPrev)

	// Tab should switch back to coordinator
	require.Equal(t, TabCoordinator, panel.ActiveTab(),
		"tab should switch back")
}

// ============================================================================
// Fabric Events Tests (Task .6)
// ============================================================================

func TestCoordinatorPanel_HasFabricEvents(t *testing.T) {
	// Verify struct has fabricEvents field with correct type
	panel := NewCoordinatorPanel(false, false, nil)

	// fabricEvents should exist and be initialized as empty slice
	require.NotNil(t, panel.fabricEvents, "fabricEvents field should exist")
	require.Empty(t, panel.fabricEvents, "fabricEvents should start empty")

	// Should be a slice of fabric.Event that can be appended
	panel.fabricEvents = append(panel.fabricEvents, fabric.Event{
		Type:        fabric.EventMessagePosted,
		ChannelSlug: "tasks",
	})
	require.Len(t, panel.fabricEvents, 1)
	require.Equal(t, fabric.EventMessagePosted, panel.fabricEvents[0].Type)
	require.Equal(t, "tasks", panel.fabricEvents[0].ChannelSlug)
}

func TestSetWorkflow_SyncsFabricEvents(t *testing.T) {
	// Verify fabricEvents synced from WorkflowUIState
	panel := NewCoordinatorPanel(false, false, nil)

	// Create state with fabric events
	state := &WorkflowUIState{
		FabricEvents: []fabric.Event{
			{Type: fabric.EventMessagePosted, ChannelSlug: "tasks", AgentID: "coordinator"},
			{Type: fabric.EventReplyPosted, ChannelSlug: "tasks", AgentID: "worker-1"},
		},
	}

	panel.SetWorkflow("wf-123", state)

	// Verify fabricEvents were synced
	require.Len(t, panel.fabricEvents, 2, "fabricEvents should be synced from state")
	require.Equal(t, fabric.EventMessagePosted, panel.fabricEvents[0].Type)
	require.Equal(t, fabric.EventReplyPosted, panel.fabricEvents[1].Type)
	require.Equal(t, "coordinator", panel.fabricEvents[0].AgentID)
	require.Equal(t, "worker-1", panel.fabricEvents[1].AgentID)
}

func TestWorkflowSwitch_PreservesFabricEvents(t *testing.T) {
	// Verify events are preserved when switching workflows
	panel := NewCoordinatorPanel(false, false, nil)

	// Set workflow 1 with some events
	state1 := &WorkflowUIState{
		FabricEvents: []fabric.Event{
			{Type: fabric.EventMessagePosted, ChannelSlug: "tasks", AgentID: "coordinator"},
		},
	}
	panel.SetWorkflow("wf-1", state1)
	require.Len(t, panel.fabricEvents, 1, "wf-1 events should be synced")

	// Switch to workflow 2 with different events
	state2 := &WorkflowUIState{
		FabricEvents: []fabric.Event{
			{Type: fabric.EventMessagePosted, ChannelSlug: "general", AgentID: "worker-1"},
			{Type: fabric.EventReplyPosted, ChannelSlug: "general", AgentID: "coordinator"},
			{Type: fabric.EventMessagePosted, ChannelSlug: "planning", AgentID: "worker-2"},
		},
	}
	panel.SetWorkflow("wf-2", state2)

	// Verify wf-2 events are now active
	require.Len(t, panel.fabricEvents, 3, "wf-2 events should be synced")
	require.Equal(t, "general", panel.fabricEvents[0].ChannelSlug)
	require.Equal(t, "worker-1", panel.fabricEvents[0].AgentID)

	// Switch back to workflow 1 (simulating preserved UI state)
	panel.SetWorkflow("wf-1", state1)

	// Verify wf-1 events are restored
	require.Len(t, panel.fabricEvents, 1, "wf-1 events should be restored after switch")
	require.Equal(t, "tasks", panel.fabricEvents[0].ChannelSlug)
	require.Equal(t, "coordinator", panel.fabricEvents[0].AgentID)
}

// ============================================================================
// Fabric Events Rendering Tests (Task .7)
// ============================================================================

func TestRenderFabricEvents_MessagePosted(t *testing.T) {
	// Verify format: timestamp, channel, sender, content
	panel := NewCoordinatorPanel(false, false, nil)
	panel.SetSize(80, 20)

	// Create test event with specific timestamp for verification
	testTime := time.Date(2025, 1, 15, 14, 30, 0, 0, time.UTC)
	state := &WorkflowUIState{
		FabricEvents: []fabric.Event{
			{
				Type:        fabric.EventMessagePosted,
				Timestamp:   testTime,
				ChannelSlug: "tasks",
				Thread: &fabricDomain.Thread{
					CreatedBy: "coordinator",
					Content:   "Task assignment for worker-1",
				},
			},
		},
	}
	panel.SetWorkflow("wf-123", state)

	// Render the fabric events
	content, plainLines := panel.renderFabricEventsWithSelection(80, nil, nil)

	// Verify timestamp format (HH:MM)
	require.Contains(t, content, "14:30", "should show timestamp in HH:MM format")

	// Verify channel format [#channelslug]
	require.Contains(t, content, "[#tasks]", "should show channel as [#channelslug]")

	// Verify sender displayed
	require.Contains(t, content, "coordinator", "should show sender")

	// Verify content displayed
	require.Contains(t, content, "Task assignment for worker-1", "should show message content")

	// Verify plain lines for selection extraction
	require.NotNil(t, plainLines, "should return plain lines for selection")
	foundHeader := false
	for _, line := range plainLines {
		if strings.Contains(line, "14:30") && strings.Contains(line, "[#tasks]") && strings.Contains(line, "coordinator") {
			foundHeader = true
			break
		}
	}
	require.True(t, foundHeader, "plain lines should contain header with timestamp, channel, and sender")
}

func TestRenderFabricEvents_ReplyPosted(t *testing.T) {
	// Verify "↳ reply:" prefix shown for reply.posted events
	panel := NewCoordinatorPanel(false, false, nil)
	panel.SetSize(80, 20)

	testTime := time.Date(2025, 1, 15, 15, 45, 0, 0, time.UTC)
	state := &WorkflowUIState{
		FabricEvents: []fabric.Event{
			{
				Type:        fabric.EventReplyPosted,
				Timestamp:   testTime,
				ChannelSlug: "tasks",
				Thread: &fabricDomain.Thread{
					CreatedBy: "worker-1",
					Content:   "Implementation complete",
				},
			},
		},
	}
	panel.SetWorkflow("wf-123", state)

	// Render the fabric events
	content, plainLines := panel.renderFabricEventsWithSelection(80, nil, nil)

	// Verify reply indicator present
	require.Contains(t, content, "↳ reply:", "should show reply indicator prefix")

	// Verify content follows reply indicator
	require.Contains(t, content, "Implementation complete", "should show reply content")

	// Verify plain lines contain reply indicator
	foundReply := false
	for _, line := range plainLines {
		if strings.Contains(line, "↳ reply:") {
			foundReply = true
			break
		}
	}
	require.True(t, foundReply, "plain lines should contain reply indicator")
}

func TestRenderFabricEvents_EmptyList(t *testing.T) {
	// Verify "No inter-agent messages yet." shown for empty state
	panel := NewCoordinatorPanel(false, false, nil)
	panel.SetSize(80, 20)

	// Empty fabric events
	state := &WorkflowUIState{
		FabricEvents: []fabric.Event{},
	}
	panel.SetWorkflow("wf-123", state)

	// Render the fabric events
	content, plainLines := panel.renderFabricEventsWithSelection(80, nil, nil)

	// Verify empty state message
	require.Contains(t, content, "No inter-agent messages yet.", "should show empty state message")

	// Verify no plain lines for empty state
	require.Nil(t, plainLines, "plain lines should be nil for empty state")
}

func TestRenderFabricEvents_CoordinatorStyle(t *testing.T) {
	// Verify coordinator messages have correct format and use coordinator styling path
	panel := NewCoordinatorPanel(false, false, nil)
	panel.SetSize(80, 20)

	testTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	state := &WorkflowUIState{
		FabricEvents: []fabric.Event{
			{
				Type:        fabric.EventMessagePosted,
				Timestamp:   testTime,
				ChannelSlug: "planning",
				Thread: &fabricDomain.Thread{
					CreatedBy: "coordinator",
					Content:   "Planning discussion",
				},
			},
		},
	}
	panel.SetWorkflow("wf-123", state)

	// Render the fabric events
	content, plainLines := panel.renderFabricEventsWithSelection(80, nil, nil)

	// Verify the content contains the coordinator name
	require.Contains(t, content, "coordinator", "should show coordinator name")

	// Verify content structure: timestamp, channel, sender
	require.Contains(t, content, "10:00", "should show timestamp")
	require.Contains(t, content, "[#planning]", "should show channel")
	require.Contains(t, content, "Planning discussion", "should show content")

	// Verify left border uses coordinator color (verified by having the "│" prefix)
	require.Contains(t, content, "│", "should have left border for coordinator")

	// Verify plain lines have correct header format
	require.NotNil(t, plainLines)
	require.Contains(t, plainLines[0], "coordinator", "plain header should contain sender")
}

func TestRenderFabricEvents_WorkerStyle(t *testing.T) {
	// Verify worker messages have correct format and use worker styling path
	panel := NewCoordinatorPanel(false, false, nil)
	panel.SetSize(80, 20)

	testTime := time.Date(2025, 1, 15, 11, 30, 0, 0, time.UTC)
	state := &WorkflowUIState{
		FabricEvents: []fabric.Event{
			{
				Type:        fabric.EventMessagePosted,
				Timestamp:   testTime,
				ChannelSlug: "tasks",
				Thread: &fabricDomain.Thread{
					CreatedBy: "worker-1",
					Content:   "Task in progress",
				},
			},
		},
	}
	panel.SetWorkflow("wf-123", state)

	// Render the fabric events
	content, plainLines := panel.renderFabricEventsWithSelection(80, nil, nil)

	// Verify the content contains the worker name
	require.Contains(t, content, "worker-1", "should show worker name")

	// Verify content structure: timestamp, channel, sender
	require.Contains(t, content, "11:30", "should show timestamp")
	require.Contains(t, content, "[#tasks]", "should show channel")
	require.Contains(t, content, "Task in progress", "should show content")

	// Verify left border uses worker color (verified by having the "│" prefix)
	require.Contains(t, content, "│", "should have left border for worker")

	// Verify plain lines have correct header format
	require.NotNil(t, plainLines)
	require.Contains(t, plainLines[0], "worker-1", "plain header should contain sender")
}
