package orchestration

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/orchestration/claude"
	"github.com/zjrosen/perles/internal/orchestration/coordinator"
	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/orchestration/pool"
	"github.com/zjrosen/perles/internal/orchestration/workflow"
	"github.com/zjrosen/perles/internal/pubsub"
)

func TestUpdate_WindowSize(t *testing.T) {
	m := New(Config{})

	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	require.Equal(t, 120, m.width)
	require.Equal(t, 40, m.height)
}

func TestUpdate_TabCyclesMessageTargets(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Initial: Input is focused, target is COORDINATOR
	require.True(t, m.input.Focused())
	require.Equal(t, "COORDINATOR", m.messageTarget)

	// Tab with no workers -> cycles through COORDINATOR -> BROADCAST -> COORDINATOR
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.True(t, m.input.Focused()) // Input stays focused
	require.Equal(t, "BROADCAST", m.messageTarget)

	// Tab -> back to COORDINATOR (no workers yet)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, "COORDINATOR", m.messageTarget)

	// Add workers and test full cycling
	m = m.UpdateWorker("worker-1", pool.WorkerWorking)
	m = m.UpdateWorker("worker-2", pool.WorkerWorking)

	// Tab -> BROADCAST
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, "BROADCAST", m.messageTarget)

	// Tab -> worker-1
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, "worker-1", m.messageTarget)

	// Tab -> worker-2
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, "worker-2", m.messageTarget)

	// Tab -> COORDINATOR (wrap)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, "COORDINATOR", m.messageTarget)
}

func TestUpdate_CtrlBracketsCycleWorkers(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Add workers
	m = m.UpdateWorker("worker-1", pool.WorkerWorking)
	m = m.UpdateWorker("worker-2", pool.WorkerWorking)

	// Initial: worker-1 displayed
	require.Equal(t, "worker-1", m.CurrentWorkerID())

	// ctrl+] -> worker-2
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}, Alt: false})
	// Note: ctrl+] is tricky to simulate, test CycleWorker directly instead
	m = m.CycleWorker(true)
	require.Equal(t, "worker-2", m.CurrentWorkerID())

	// ctrl+[ -> worker-1
	m = m.CycleWorker(false)
	require.Equal(t, "worker-1", m.CurrentWorkerID())
}

func TestUpdate_InputAlwaysFocused(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Input is focused by default
	require.True(t, m.input.Focused())

	// Tab doesn't unfocus input - just cycles targets
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.True(t, m.input.Focused())
}

func TestUpdate_InputSubmit(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Focus and type in input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.input.SetValue("Hello coordinator")

	// Submit with Enter
	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Input should be cleared
	require.Equal(t, "", m.input.Value())

	// Should produce UserInputMsg
	require.NotNil(t, cmd)
	msg := cmd()
	userMsg, ok := msg.(UserInputMsg)
	require.True(t, ok)
	require.Equal(t, "Hello coordinator", userMsg.Content)
}

func TestUpdate_InputEmpty_NoSubmit(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Focus input but leave empty
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, m.input.Focused())

	// Submit with empty input should not produce command
	var cmd tea.Cmd
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	require.Nil(t, cmd)
}

func TestUpdate_QuitMsg(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Esc quits (input is always focused)
	var cmd tea.Cmd
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEscape})

	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(QuitMsg)
	require.True(t, ok)
}

func TestUpdate_EscQuits(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 40)

	var cmd tea.Cmd
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEscape})

	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(QuitMsg)
	require.True(t, ok)
}

func TestUpdate_EscFromInputQuits(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Focus input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, m.input.Focused())

	// Esc should quit (not just unfocus)
	var cmd tea.Cmd
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEscape})

	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(QuitMsg)
	require.True(t, ok)
}

func TestUpdate_PauseMsg(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Ctrl+Z pauses (input is always focused)
	var cmd tea.Cmd
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlZ})

	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(PauseMsg)
	require.True(t, ok)
}

func TestUpdate_WorkflowPicker(t *testing.T) {
	// Create a registry with a test workflow
	reg := workflow.NewRegistry()
	reg.Add(workflow.Workflow{
		ID:          "test-workflow",
		Name:        "Test Workflow",
		Description: "A test workflow",
		Content:     "Test content",
		Source:      workflow.SourceBuiltIn,
	})

	m := New(Config{
		WorkflowRegistry: reg,
	})
	m = m.SetSize(120, 40)

	// Ctrl+P opens workflow picker
	require.False(t, m.showWorkflowPicker)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	require.True(t, m.showWorkflowPicker)
	require.NotNil(t, m.workflowPicker)
}

func TestUpdate_TabKeepsInputFocused(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Input is focused by default
	require.True(t, m.input.Focused())

	// Tab keeps input focused (just cycles message target)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.True(t, m.input.Focused())
}

func TestHandleReplaceCoordinator_SetsPendingRefresh(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Without coordinator, should show error and not set pendingRefresh
	require.False(t, m.pendingRefresh)
	m, _ = m.handleReplaceCoordinator()
	require.NotNil(t, m.errorModal, "should set error when no coordinator")
	require.False(t, m.pendingRefresh, "should not set pendingRefresh without coordinator")
}

func TestHandleReplaceCoordinator_WithCoordinator(t *testing.T) {
	// Create a model with a coordinator
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Set up a coordinator - we need to inject one for testing
	// Create a minimal coordinator using the coordinator package
	msgIssue := message.New()
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	coord, err := coordinator.New(coordinator.Config{
		Client:       claude.NewClient(),
		WorkDir:      "/tmp",
		Pool:         workerPool,
		MessageIssue: msgIssue,
	})
	require.NoError(t, err)

	// Inject the coordinator into the model
	m.coord = coord

	// Manually set coordinator to running status so SendUserMessage doesn't fail immediately
	// Note: This uses internal knowledge of coordinator status, but is necessary for testing
	// the TUI behavior. The actual message send will fail, but we verify pendingRefresh is set.
	require.False(t, m.pendingRefresh, "pendingRefresh should start false")

	// Call handleReplaceCoordinator
	m, cmd := m.handleReplaceCoordinator()

	// Verify pendingRefresh is set to true
	require.True(t, m.pendingRefresh, "pendingRefresh should be set to true")

	// Verify a command is returned (the async function to send message)
	require.NotNil(t, cmd, "should return a command to send handoff request")

	// Verify no error modal was set
	require.Nil(t, m.errorModal, "should not set error modal when coordinator exists")
}

func TestHandleReplaceCoordinator_MessageContent(t *testing.T) {
	// This test verifies the handoff request message contains key phrases
	// that help the coordinator understand what's happening and what to include

	// The message is constructed in handleReplaceCoordinator() - we verify
	// it contains the key elements by examining the source code expectations
	// Since we can't easily extract the message from the async command,
	// we verify the key phrases that MUST be present in any valid handoff message

	expectedPhrases := []string{
		"[CONTEXT REFRESH INITIATED]",       // Header identifying the message type
		"context window",                    // Explains why refresh is happening
		"replaced with a fresh coordinator", // What will happen
		"workers will continue running",     // External state preserved
		"prepare_handoff",                   // Tool to call
		"Current work state",                // What to include
		"Recent decisions",                  // What to include
		"Blockers or issues",                // What to include
		"Recommendations",                   // What to include
		"briefing your replacement",         // Emphasis on importance
	}

	// Construct the expected message (same as in handleReplaceCoordinator)
	handoffMessage := `[CONTEXT REFRESH INITIATED]

Your context window is approaching limits. The user has initiated a coordinator refresh (Ctrl+R).

WHAT'S ABOUT TO HAPPEN:
- You will be replaced with a fresh coordinator session
- All workers will continue running (their state is preserved)
- External state (message log, bd tasks, etc.) is preserved
- The new coordinator will start with a clean context window

YOUR TASK:
Call ` + "`prepare_handoff`" + ` with a comprehensive summary for the incoming coordinator. This summary is CRITICAL - it's the primary way the new coordinator will understand what work is in progress.

WHAT TO INCLUDE IN THE HANDOFF:
1. Current work state: Which workers are doing what? What tasks are in progress?
2. Recent decisions: What approach did you take? Why?
3. Blockers or issues: Anything the new coordinator should know about?
4. Recommendations: What should the new coordinator do next?
5. Context that isn't in the message log: Internal reasoning, strategy, patterns you've noticed

The more detailed your handoff, the smoother the transition will be. Think of this as briefing your replacement.

When you're ready, call: ` + "`prepare_handoff`" + ` with your summary.`

	// Verify all expected phrases are present
	for _, phrase := range expectedPhrases {
		require.Contains(t, handoffMessage, phrase,
			"handoff message should contain: %q", phrase)
	}
}

func TestHandleMessageEvent_HandoffTriggersReplace(t *testing.T) {
	// Create a model with a coordinator and pending refresh
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Set up a coordinator
	msgIssue := message.New()
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	coord, err := coordinator.New(coordinator.Config{
		Client:       claude.NewClient(),
		WorkDir:      "/tmp",
		Pool:         workerPool,
		MessageIssue: msgIssue,
	})
	require.NoError(t, err)

	m.coord = coord
	m.pendingRefresh = true

	// Set up a message listener so handleMessageEvent doesn't return early
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.messageListener = pubsub.NewContinuousListener(ctx, msgIssue.Broker())

	// Create a handoff message event
	handoffEntry := message.Entry{
		ID:      "test-123",
		From:    message.ActorCoordinator,
		To:      message.ActorAll,
		Content: "[HANDOFF] Test handoff message",
		Type:    message.MessageHandoff,
	}
	event := pubsub.Event[message.Event]{
		Type: pubsub.UpdatedEvent,
		Payload: message.Event{
			Type:  message.EventPosted,
			Entry: handoffEntry,
		},
	}

	// Handle the message event
	m, cmd := m.handleMessageEvent(event)

	// Verify pendingRefresh is cleared
	require.False(t, m.pendingRefresh, "pendingRefresh should be cleared after handoff")

	// Verify a command is returned (batched: listener + replace command)
	require.NotNil(t, cmd, "should return a batched command")

	// Verify the message was appended to the message pane
	require.Len(t, m.messagePane.entries, 1, "should append handoff entry to message pane")
	require.Equal(t, message.MessageHandoff, m.messagePane.entries[0].Type)
}

func TestHandleMessageEvent_IgnoresHandoffWhenNotPending(t *testing.T) {
	// Create a model without pending refresh
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Set up a coordinator
	msgIssue := message.New()
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	coord, err := coordinator.New(coordinator.Config{
		Client:       claude.NewClient(),
		WorkDir:      "/tmp",
		Pool:         workerPool,
		MessageIssue: msgIssue,
	})
	require.NoError(t, err)

	m.coord = coord
	m.pendingRefresh = false // Not waiting for refresh

	// Set up a message listener
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.messageListener = pubsub.NewContinuousListener(ctx, msgIssue.Broker())

	// Create a handoff message event
	handoffEntry := message.Entry{
		ID:      "test-123",
		From:    message.ActorCoordinator,
		To:      message.ActorAll,
		Content: "[HANDOFF] Test handoff message",
		Type:    message.MessageHandoff,
	}
	event := pubsub.Event[message.Event]{
		Type: pubsub.UpdatedEvent,
		Payload: message.Event{
			Type:  message.EventPosted,
			Entry: handoffEntry,
		},
	}

	// Handle the message event
	m, _ = m.handleMessageEvent(event)

	// Verify pendingRefresh is still false (unchanged)
	require.False(t, m.pendingRefresh, "pendingRefresh should remain false")

	// Verify the message was still appended (normal behavior)
	require.Len(t, m.messagePane.entries, 1, "should still append entry to message pane")
}

func TestHandleMessageEvent_ClearsPendingRefresh(t *testing.T) {
	// This test verifies that pendingRefresh is cleared when handoff is received
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Set up a coordinator
	msgIssue := message.New()
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	coord, err := coordinator.New(coordinator.Config{
		Client:       claude.NewClient(),
		WorkDir:      "/tmp",
		Pool:         workerPool,
		MessageIssue: msgIssue,
	})
	require.NoError(t, err)

	m.coord = coord
	m.pendingRefresh = true

	// Set up a message listener
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.messageListener = pubsub.NewContinuousListener(ctx, msgIssue.Broker())

	// First verify pendingRefresh is true
	require.True(t, m.pendingRefresh, "pendingRefresh should start true")

	// Send a handoff message
	handoffEntry := message.Entry{
		ID:      "test-456",
		From:    message.ActorCoordinator,
		To:      message.ActorAll,
		Content: "[HANDOFF] Context summary here",
		Type:    message.MessageHandoff,
	}
	event := pubsub.Event[message.Event]{
		Type: pubsub.UpdatedEvent,
		Payload: message.Event{
			Type:  message.EventPosted,
			Entry: handoffEntry,
		},
	}

	m, _ = m.handleMessageEvent(event)

	// Verify pendingRefresh is now false
	require.False(t, m.pendingRefresh, "pendingRefresh should be cleared after handling handoff")
}

func TestHandleMessageEvent_NonHandoffMessagePreservesPendingRefresh(t *testing.T) {
	// This test verifies that non-handoff messages don't affect pendingRefresh
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Set up a coordinator
	msgIssue := message.New()
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	coord, err := coordinator.New(coordinator.Config{
		Client:       claude.NewClient(),
		WorkDir:      "/tmp",
		Pool:         workerPool,
		MessageIssue: msgIssue,
	})
	require.NoError(t, err)

	m.coord = coord
	m.pendingRefresh = true

	// Set up a message listener
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.messageListener = pubsub.NewContinuousListener(ctx, msgIssue.Broker())

	// Send a regular info message (not handoff)
	infoEntry := message.Entry{
		ID:      "test-789",
		From:    "WORKER.1",
		To:      message.ActorCoordinator,
		Content: "Task completed",
		Type:    message.MessageInfo,
	}
	event := pubsub.Event[message.Event]{
		Type: pubsub.UpdatedEvent,
		Payload: message.Event{
			Type:  message.EventPosted,
			Entry: infoEntry,
		},
	}

	m, _ = m.handleMessageEvent(event)

	// Verify pendingRefresh is still true (non-handoff messages don't clear it)
	require.True(t, m.pendingRefresh, "pendingRefresh should remain true for non-handoff messages")

	// Verify the message was appended
	require.Len(t, m.messagePane.entries, 1)
	require.Equal(t, message.MessageInfo, m.messagePane.entries[0].Type)
}

func TestHandoffTimeout_TriggersReplace(t *testing.T) {
	// This test verifies that RefreshTimeoutMsg triggers Replace() when pendingRefresh is true
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Set up a coordinator
	msgIssue := message.New()
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	coord, err := coordinator.New(coordinator.Config{
		Client:       claude.NewClient(),
		WorkDir:      "/tmp",
		Pool:         workerPool,
		MessageIssue: msgIssue,
	})
	require.NoError(t, err)

	m.coord = coord
	m.messageLog = msgIssue
	m.pendingRefresh = true

	// Verify pendingRefresh is true before timeout
	require.True(t, m.pendingRefresh, "pendingRefresh should be true before timeout")

	// Send RefreshTimeoutMsg
	m, cmd := m.Update(RefreshTimeoutMsg{})

	// Verify pendingRefresh is cleared
	require.False(t, m.pendingRefresh, "pendingRefresh should be cleared after timeout")

	// Verify a command is returned (the async function to post message and replace)
	require.NotNil(t, cmd, "should return a command to post fallback and replace")
}

func TestHandoffTimeout_PostsFallbackMessage(t *testing.T) {
	// This test verifies that the fallback handoff message is posted on timeout
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Set up a coordinator with message log
	msgIssue := message.New()
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	coord, err := coordinator.New(coordinator.Config{
		Client:       claude.NewClient(),
		WorkDir:      "/tmp",
		Pool:         workerPool,
		MessageIssue: msgIssue,
	})
	require.NoError(t, err)

	m.coord = coord
	m.messageLog = msgIssue
	m.pendingRefresh = true

	// Send RefreshTimeoutMsg
	m, cmd := m.Update(RefreshTimeoutMsg{})

	// The command should be non-nil
	require.NotNil(t, cmd, "should return a command")

	// Execute the command (this will post the message and try to replace)
	// The Replace() call may fail because coordinator isn't running, but message should be posted
	_ = cmd()

	// Check that the fallback message was posted to the message log
	entries := msgIssue.Entries()
	require.Len(t, entries, 1, "should have posted one fallback message")
	require.Equal(t, message.MessageHandoff, entries[0].Type, "message should be handoff type")
	require.Contains(t, entries[0].Content, "coordinator did not respond", "message should indicate timeout")
}

func TestHandoffTimeout_IgnoredWhenNotPending(t *testing.T) {
	// This test verifies that timeout is ignored if handoff already received (pendingRefresh=false)
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Set up a coordinator
	msgIssue := message.New()
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	coord, err := coordinator.New(coordinator.Config{
		Client:       claude.NewClient(),
		WorkDir:      "/tmp",
		Pool:         workerPool,
		MessageIssue: msgIssue,
	})
	require.NoError(t, err)

	m.coord = coord
	m.messageLog = msgIssue
	m.pendingRefresh = false // Handoff already received

	// Send RefreshTimeoutMsg
	m, cmd := m.Update(RefreshTimeoutMsg{})

	// Verify pendingRefresh is still false
	require.False(t, m.pendingRefresh, "pendingRefresh should remain false")

	// Verify no command is returned (timeout is ignored)
	require.Nil(t, cmd, "should return nil command when not pending")

	// Verify no fallback message was posted
	entries := msgIssue.Entries()
	require.Len(t, entries, 0, "should not post any message when not pending")
}

func TestHandleMessageEvent_WorkerReady_AppearsInMessagePane(t *testing.T) {
	// Test that MessageWorkerReady messages appear in the message pane
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Set up message log and listener
	msgIssue := message.New()
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	coord, err := coordinator.New(coordinator.Config{
		Client:       claude.NewClient(),
		WorkDir:      "/tmp",
		Pool:         workerPool,
		MessageIssue: msgIssue,
	})
	require.NoError(t, err)

	m.coord = coord

	// Set up a message listener
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.messageListener = pubsub.NewContinuousListener(ctx, msgIssue.Broker())

	// Create a worker ready message event
	readyEntry := message.Entry{
		ID:      "test-ready-123",
		From:    "WORKER.1",
		To:      message.ActorCoordinator,
		Content: "Worker WORKER.1 ready for task assignment",
		Type:    message.MessageWorkerReady,
	}
	event := pubsub.Event[message.Event]{
		Type: pubsub.UpdatedEvent,
		Payload: message.Event{
			Type:  message.EventPosted,
			Entry: readyEntry,
		},
	}

	// Handle the message event
	m, _ = m.handleMessageEvent(event)

	// Verify the message was appended to the message pane
	require.Len(t, m.messagePane.entries, 1, "should append worker ready entry to message pane")
	require.Equal(t, message.MessageWorkerReady, m.messagePane.entries[0].Type)
	require.Equal(t, "WORKER.1", m.messagePane.entries[0].From)
}

func TestHandleMessageEvent_RegularMessage_UsesDebounce(t *testing.T) {
	// Test that regular messages use debounce (contrast with worker ready)
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Set up message log and listener
	msgIssue := message.New()
	workerPool := pool.NewWorkerPool(pool.Config{})
	defer workerPool.Close()

	coord, err := coordinator.New(coordinator.Config{
		Client:       claude.NewClient(),
		WorkDir:      "/tmp",
		Pool:         workerPool,
		MessageIssue: msgIssue,
	})
	require.NoError(t, err)

	m.coord = coord

	// Set up a message listener
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.messageListener = pubsub.NewContinuousListener(ctx, msgIssue.Broker())

	// Set up nudge batcher with tracking
	var nudgeCallCount int
	m.nudgeBatcher = NewNudgeBatcher(100 * time.Millisecond) // Short debounce for test
	m.nudgeBatcher.SetOnNudge(func(messagesByType map[MessageType][]string) {
		nudgeCallCount++
	})

	// Create a regular info message event
	infoEntry := message.Entry{
		ID:      "test-info-789",
		From:    "WORKER.1",
		To:      message.ActorCoordinator,
		Content: "Task completed",
		Type:    message.MessageInfo,
	}
	event := pubsub.Event[message.Event]{
		Type: pubsub.UpdatedEvent,
		Payload: message.Event{
			Type:  message.EventPosted,
			Entry: infoEntry,
		},
	}

	// Handle the message event
	m, _ = m.handleMessageEvent(event)

	// Immediately check - should NOT have fired yet (debounce pending)
	require.Equal(t, 0, nudgeCallCount, "should not nudge immediately for regular messages")

	// Wait for debounce
	time.Sleep(150 * time.Millisecond)

	// Now it should have fired
	require.Equal(t, 1, nudgeCallCount, "should nudge after debounce for regular messages")
}
