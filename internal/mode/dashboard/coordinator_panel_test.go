package dashboard

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/orchestration/controlplane"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/ui/shared/chatrender"
)

func TestNewCoordinatorPanel(t *testing.T) {
	panel := NewCoordinatorPanel()

	require.NotNil(t, panel)
	require.False(t, panel.IsFocused(), "panel should be unfocused by default")
	require.Empty(t, panel.coordinatorMessages)
	require.True(t, panel.coordinatorDirty)
	require.Equal(t, TabCoordinator, panel.activeTab)
}

func TestCoordinatorPanel_SetWorkflow(t *testing.T) {
	panel := NewCoordinatorPanel()

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
	require.True(t, panel.coordinatorDirty, "should be dirty after setting workflow")
}

func TestCoordinatorPanel_SetWorkflow_SameWorkflowNewMessages(t *testing.T) {
	panel := NewCoordinatorPanel()

	// Set initial state
	state := &WorkflowUIState{
		CoordinatorMessages: []chatrender.Message{
			{Role: "user", Content: "Hello"},
		},
		CoordinatorStatus: events.ProcessStatusReady,
	}
	panel.SetWorkflow("wf-123", state)
	panel.coordinatorDirty = false // simulate View() was called

	// Add more messages
	state.CoordinatorMessages = append(state.CoordinatorMessages, chatrender.Message{Role: "assistant", Content: "Hi"})
	state.CoordinatorStatus = events.ProcessStatusWorking
	panel.SetWorkflow("wf-123", state)

	require.Len(t, panel.coordinatorMessages, 2)
	require.Equal(t, events.ProcessStatusWorking, panel.coordinatorStatus)
	require.True(t, panel.coordinatorDirty, "should be dirty when message count changes")
}

func TestCoordinatorPanel_Focus(t *testing.T) {
	panel := NewCoordinatorPanel()
	panel.Blur()

	require.False(t, panel.IsFocused())

	panel.Focus()

	require.True(t, panel.IsFocused())
}

func TestCoordinatorPanel_SetSize(t *testing.T) {
	panel := NewCoordinatorPanel()

	panel.SetSize(100, 50)

	require.Equal(t, 100, panel.width)
	require.Equal(t, 50, panel.height)
}

func TestCoordinatorPanel_View_EmptyMessages(t *testing.T) {
	panel := NewCoordinatorPanel()
	panel.SetSize(80, 20)
	panel.SetWorkflow("wf-123", nil)

	view := panel.View()

	require.NotEmpty(t, view)
	require.Contains(t, view, "Coord", "should show Coordinator tab label")
	require.Contains(t, view, "Msgs", "should show Messages tab label")
}

func TestRenderChatContent_EmptyMessages(t *testing.T) {
	cfg := chatrender.RenderConfig{
		AgentLabel: "Coordinator",
		AgentColor: chatrender.CoordinatorColor,
		UserLabel:  "User",
	}
	content := renderChatContent(nil, 80, cfg)

	require.Contains(t, content, "Waiting for the coordinator to initialize.")
}

func TestRenderChatContent_WithMessages(t *testing.T) {
	messages := []chatrender.Message{
		{Role: "user", Content: "Hello world"},
		{Role: "assistant", Content: "Hi there!"},
	}

	cfg := chatrender.RenderConfig{
		AgentLabel: "Coordinator",
		AgentColor: chatrender.CoordinatorColor,
		UserLabel:  "User",
	}
	content := renderChatContent(messages, 80, cfg)

	require.Contains(t, content, "User")
	require.Contains(t, content, "Hello world")
	require.Contains(t, content, "Coordinator") // Uses "Coordinator" label from RenderConfig
	require.Contains(t, content, "Hi there!")
}

func TestRenderChatContent_ToolCall(t *testing.T) {
	messages := []chatrender.Message{
		{Role: "assistant", Content: "Using a tool", IsToolCall: true},
	}

	cfg := chatrender.RenderConfig{
		AgentLabel: "Coordinator",
		AgentColor: chatrender.CoordinatorColor,
		UserLabel:  "User",
	}
	content := renderChatContent(messages, 80, cfg)

	// Tool calls use the "╰╴" prefix in shared chatrender
	require.Contains(t, content, "╰╴")
	require.Contains(t, content, "Using a tool")
}

func TestRenderChatContent_FiltersEmptyMessages(t *testing.T) {
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
	content := renderChatContent(messages, 80, cfg)

	require.Contains(t, content, "Hello")
	require.Contains(t, content, "Hi!")
	// Should not have empty lines from the filtered message
}

func TestNewCoordinatorPanel_InputStartsUnfocused(t *testing.T) {
	panel := NewCoordinatorPanel()

	// Verify the input starts unfocused (focus is given on explicit Focus() call)
	require.False(t, panel.input.Focused())
	require.False(t, panel.focused)

	// After Focus(), both should be true
	panel.Focus()
	require.True(t, panel.input.Focused())
	require.True(t, panel.focused)
}

func TestCoordinatorPanel_TabNavigation(t *testing.T) {
	panel := NewCoordinatorPanel()

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

func TestCoordinatorPanel_TabNavigationWithWorkers(t *testing.T) {
	panel := NewCoordinatorPanel()

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
	panel := NewCoordinatorPanel()

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
	panel := NewCoordinatorPanel()

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
	panel := NewCoordinatorPanel()

	require.Equal(t, "W1", panel.formatWorkerTabLabel("worker-1"))
	require.Equal(t, "W99", panel.formatWorkerTabLabel("worker-99"))
	require.Equal(t, "custom", panel.formatWorkerTabLabel("custom"))
	require.Equal(t, "longla", panel.formatWorkerTabLabel("longlabel")) // truncates to 6 chars
}
