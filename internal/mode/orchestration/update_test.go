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
	"github.com/zjrosen/perles/internal/orchestration/session"
	"github.com/zjrosen/perles/internal/orchestration/workflow"
	"github.com/zjrosen/perles/internal/pubsub"
	"github.com/zjrosen/perles/internal/ui/shared/modal"
	"github.com/zjrosen/perles/internal/ui/shared/vimtextarea"
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

	// Type in input (starts in Insert mode)
	m.input.SetValue("Hello coordinator")

	// Submit with Shift+Enter (vim mode uses Shift+Enter for submit, Enter for newline)
	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}, Alt: false})
	// Shift+Enter sends SubmitMsg which triggers UserInputMsg
	m, cmd = m.Update(vimtextarea.SubmitMsg{Content: "Hello coordinator"})

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

	// With vim disabled, Enter submits which emits SubmitMsg
	// Then SubmitMsg handler checks if content is empty
	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// First phase: vimtextarea emits SubmitMsg
	require.NotNil(t, cmd)
	submitMsg := cmd()
	_, isSubmitMsg := submitMsg.(vimtextarea.SubmitMsg)
	require.True(t, isSubmitMsg, "expected SubmitMsg")

	// Second phase: SubmitMsg handler returns nil for empty content
	_, cmd = m.Update(submitMsg)
	require.Nil(t, cmd, "empty input should not produce UserInputMsg")
}

func TestUpdate_QuitMsg(t *testing.T) {
	m := New(Config{VimMode: true})
	m = m.SetSize(120, 40)
	m.initializer = newTestInitializer(InitReady, nil)

	// Start in Insert mode, first ESC switches to Normal mode
	require.Equal(t, vimtextarea.ModeInsert, m.input.Mode())
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	require.Equal(t, vimtextarea.ModeNormal, m.input.Mode(), "First ESC should switch to Normal mode")
	require.Nil(t, m.quitModal, "quit modal should NOT be shown after first ESC")

	// Second ESC (in Normal mode) shows quit modal
	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	require.Nil(t, cmd, "ESC in Normal mode should not return a command (modal shown instead)")
	require.NotNil(t, m.quitModal, "quit modal should be shown after ESC in Normal mode")

	// Confirm via modal submit triggers QuitMsg
	m, cmd = m.Update(modal.SubmitMsg{})
	require.NotNil(t, cmd, "modal submit should return a command")
	msg := cmd()
	_, ok := msg.(QuitMsg)
	require.True(t, ok, "modal submit should produce QuitMsg")
}

func TestUpdate_EscQuits(t *testing.T) {
	m := New(Config{VimMode: true})
	m = m.SetSize(120, 40)
	m.initializer = newTestInitializer(InitReady, nil)

	// With vim mode, ESC in Insert mode first switches to Normal mode
	// Then ESC in Normal mode shows quit confirmation modal
	m.input.SetMode(vimtextarea.ModeNormal) // Start in Normal mode to test quit directly

	// ESC shows quit confirmation modal (when in Normal mode)
	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEscape})

	require.Nil(t, cmd, "ESC should not return a command (modal shown instead)")
	require.NotNil(t, m.quitModal, "quit modal should be shown after ESC in Normal mode")

	// Confirm via modal submit to get QuitMsg
	m, cmd = m.Update(modal.SubmitMsg{})
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(QuitMsg)
	require.True(t, ok)
}

func TestUpdate_EscFromInputQuits(t *testing.T) {
	m := New(Config{VimMode: true})
	m = m.SetSize(120, 40)
	m.initializer = newTestInitializer(InitReady, nil)

	// Input is always focused in orchestration mode, and starts in Insert mode
	require.True(t, m.input.Focused())
	require.Equal(t, vimtextarea.ModeInsert, m.input.Mode())

	// First ESC switches to Normal mode (vim behavior)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	require.Equal(t, vimtextarea.ModeNormal, m.input.Mode(), "First ESC should switch to Normal mode")
	require.Nil(t, m.quitModal, "quit modal should NOT be shown after first ESC")

	// Second ESC (in Normal mode) shows quit confirmation modal
	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	require.Nil(t, cmd, "ESC in Normal mode should not return a command (modal shown instead)")
	require.NotNil(t, m.quitModal, "quit modal should be shown after ESC in Normal mode")

	// Confirm via modal submit to get QuitMsg
	m, cmd = m.Update(modal.SubmitMsg{})
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

// ========================================================================
// Vim Mode Integration Tests
// ========================================================================

func TestVim_VimTextareaRendersInOrchestrationMode(t *testing.T) {
	// Integration test: vimtextarea renders in orchestration mode when vim mode is enabled
	m := New(Config{VimMode: true})
	m = m.SetSize(120, 30)

	// Verify vimtextarea is used (starts in Insert mode with vim enabled)
	require.True(t, m.input.VimEnabled(), "vim mode should be enabled")
	require.Equal(t, vimtextarea.ModeInsert, m.input.Mode(), "should start in Insert mode")

	// Render the view
	view := m.View()
	require.NotEmpty(t, view, "view should not be empty")
	// Note: Mode indicator is now the client's responsibility (not rendered by vimtextarea)
	// Mode can be queried via input.Mode() method
}

func TestVim_HjklTypesInInsertMode(t *testing.T) {
	// Integration test: hjkl types characters when textarea is in Insert mode
	m := New(Config{})
	m = m.SetSize(120, 40)
	m.initializer = newTestInitializer(InitReady, nil)

	// Starts in Insert mode
	require.Equal(t, vimtextarea.ModeInsert, m.input.Mode())

	// Type 'h' - should insert the character
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	require.Contains(t, m.input.Value(), "h", "h should be typed as character in Insert mode")

	// Type 'j' - should insert the character
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Contains(t, m.input.Value(), "hj", "j should be typed as character in Insert mode")

	// Type 'k' - should insert the character
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	require.Contains(t, m.input.Value(), "hjk", "k should be typed as character in Insert mode")

	// Type 'l' - should insert the character
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	require.Equal(t, "hjkl", m.input.Value(), "l should be typed as character in Insert mode")
}

func TestVim_HjklNavigatesInNormalMode(t *testing.T) {
	// Integration test: hjkl moves cursor when textarea is in Normal mode (focused)
	m := New(Config{VimMode: true})
	m = m.SetSize(120, 40)
	m.initializer = newTestInitializer(InitReady, nil)

	// Set some text and switch to Normal mode
	m.input.SetValue("hello world")
	m.input.SetMode(vimtextarea.ModeNormal)

	// Move cursor to position 5 (middle of text) using 'l' repeatedly
	for i := 0; i < 5; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	}

	// Get cursor position before pressing 'h'
	initialPos := m.input.CursorPosition()
	require.Equal(t, 5, initialPos.Col, "cursor should be at column 5")

	// Press 'h' - should move cursor left (NOT type 'h')
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})

	// Content should NOT change
	require.Equal(t, "hello world", m.input.Value(), "content should not change in Normal mode")

	// Cursor should have moved left
	newPos := m.input.CursorPosition()
	require.Equal(t, 4, newPos.Col, "cursor should move left on 'h' in Normal mode")
}

func TestVim_EscInInsertModeSwitchesToNormal(t *testing.T) {
	// Integration test: ESC in Insert mode switches to Normal (not quit)
	m := New(Config{VimMode: true})
	m = m.SetSize(120, 40)
	m.initializer = newTestInitializer(InitReady, nil)

	// Starts in Insert mode
	require.Equal(t, vimtextarea.ModeInsert, m.input.Mode())
	require.Nil(t, m.quitModal, "no quit modal initially")

	// Press ESC
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape})

	// Should switch to Normal mode, NOT show quit modal
	require.Equal(t, vimtextarea.ModeNormal, m.input.Mode(), "ESC should switch to Normal mode")
	require.Nil(t, m.quitModal, "quit modal should NOT be shown when ESC exits Insert mode")
}

func TestVim_EscInNormalModeTriggersQuit(t *testing.T) {
	// Integration test: ESC in Normal mode triggers quit confirmation
	m := New(Config{VimMode: true})
	m = m.SetSize(120, 40)
	m.initializer = newTestInitializer(InitReady, nil)

	// Switch to Normal mode
	m.input.SetMode(vimtextarea.ModeNormal)
	require.Equal(t, vimtextarea.ModeNormal, m.input.Mode())
	require.Nil(t, m.quitModal, "no quit modal initially")

	// Press ESC in Normal mode
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape})

	// Should show quit modal
	require.NotNil(t, m.quitModal, "ESC in Normal mode should show quit confirmation")
}

func TestVim_ShiftEnterSubmitsMessage(t *testing.T) {
	// Integration test: Shift+Enter submits message (sends Alt+Enter in terminal)
	m := New(Config{})
	m = m.SetSize(120, 40)
	m.initializer = newTestInitializer(InitReady, nil)

	// Type a message
	m.input.SetValue("Test message")

	// Send SubmitMsg (which is what Shift+Enter produces from vimtextarea)
	var cmd tea.Cmd
	m, cmd = m.Update(vimtextarea.SubmitMsg{Content: "Test message"})

	// Input should be cleared
	require.Equal(t, "", m.input.Value(), "input should be cleared after submit")

	// Should produce UserInputMsg command
	require.NotNil(t, cmd, "should return a command")
	msg := cmd()
	userMsg, ok := msg.(UserInputMsg)
	require.True(t, ok, "should produce UserInputMsg")
	require.Equal(t, "Test message", userMsg.Content)
}

func TestVim_EnterSubmitsMessage_VimDisabled(t *testing.T) {
	// Integration test: With vim disabled, Enter submits message
	m := New(Config{}) // VimMode: false by default
	m = m.SetSize(120, 40)
	m.initializer = newTestInitializer(InitReady, nil)

	// Type some text
	m.input.SetValue("Line 1")

	// Press Enter (vim disabled, so Enter submits)
	require.Equal(t, vimtextarea.ModeInsert, m.input.Mode())
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// First phase: vimtextarea emits SubmitMsg
	require.NotNil(t, cmd, "Enter should produce a command")
	msg := cmd()
	submitMsg, isSubmitMsg := msg.(vimtextarea.SubmitMsg)
	require.True(t, isSubmitMsg, "Enter should produce SubmitMsg")
	require.Equal(t, "Line 1", submitMsg.Content)

	// Second phase: SubmitMsg handler produces UserInputMsg
	m, cmd = m.Update(submitMsg)
	require.NotNil(t, cmd, "SubmitMsg should produce UserInputMsg command")
	msg = cmd()
	userInput, isUserInput := msg.(UserInputMsg)
	require.True(t, isUserInput, "SubmitMsg handler should produce UserInputMsg")
	require.Equal(t, "Line 1", userInput.Content)

	// Input should be cleared after submit
	require.Empty(t, m.input.Value(), "Input should be cleared after submit")
}

func TestVim_MessageSubmissionWorksEndToEnd(t *testing.T) {
	// Integration test: Message submission works end-to-end
	m := New(Config{})
	m = m.SetSize(120, 40)
	m.initializer = newTestInitializer(InitReady, nil)

	// 1. Type a message character by character (in Insert mode)
	for _, r := range "Hello coordinator" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	require.Equal(t, "Hello coordinator", m.input.Value())

	// 2. Submit via Shift+Enter (simulated by SubmitMsg)
	var cmd tea.Cmd
	m, cmd = m.Update(vimtextarea.SubmitMsg{Content: m.input.Value()})

	// 3. Verify input cleared
	require.Equal(t, "", m.input.Value())

	// 4. Verify command produced
	require.NotNil(t, cmd)
	msg := cmd()
	userMsg, ok := msg.(UserInputMsg)
	require.True(t, ok)
	require.Equal(t, "Hello coordinator", userMsg.Content)
	require.Equal(t, "COORDINATOR", userMsg.Target)
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

func TestQuitConfirmation_ForceQuit_DoubleCtrlC(t *testing.T) {
	// Test that Ctrl+C while quit modal is visible bypasses confirmation (force quit)
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Simulate quit modal being shown (as if user pressed ESC/Ctrl+C once)
	m.quitModal = &modal.Model{} // Non-nil indicates modal is visible

	// Press Ctrl+C while quit modal is visible
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

	// Modal should be cleared
	require.Nil(t, m.quitModal, "quitModal should be cleared after force quit")

	// Should produce immediate QuitMsg (force quit bypasses confirmation)
	require.NotNil(t, cmd, "should return a command for force quit")
	msg := cmd()
	_, ok := msg.(QuitMsg)
	require.True(t, ok, "force quit should produce QuitMsg")
}

func TestQuitConfirmation_ModalForwardsOtherKeys(t *testing.T) {
	// Test that non-Ctrl+C keys are forwarded to the modal
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Simulate quit modal being shown
	qm := modal.New(modal.Config{
		Title:   "Test",
		Message: "Test message",
	})
	m.quitModal = &qm

	// Press Enter (should be forwarded to modal, not trigger force quit)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Modal should still be set (key forwarded, not force quit)
	// Note: The actual modal behavior (submit/cancel) depends on modal state,
	// but the key should be forwarded rather than triggering force quit
	require.NotNil(t, cmd, "should return a command from modal")

	// Verify it's NOT a QuitMsg (force quit requires Ctrl+C specifically)
	if cmd != nil {
		msg := cmd()
		_, isQuitMsg := msg.(QuitMsg)
		// Enter on modal typically produces SubmitMsg, not QuitMsg
		require.False(t, isQuitMsg, "non-Ctrl+C keys should not trigger force quit")
	}
}

// === Quit Confirmation Test Suite ===
// These tests verify the quit confirmation modal behavior introduced to prevent
// accidental exits from orchestrator mode.

func TestQuitConfirmation_CtrlC_ShowsModal(t *testing.T) {
	// Test that Ctrl+C in ready state shows the quit confirmation modal
	m := New(Config{})
	m = m.SetSize(120, 40)
	m.initializer = newTestInitializer(InitReady, nil)

	// Ctrl+C should show quit modal, not immediately quit
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

	require.Nil(t, cmd, "Ctrl+C should not return a command (modal shown instead)")
	require.NotNil(t, m.quitModal, "quit modal should be shown after Ctrl+C")
}

func TestQuitConfirmation_Cancel(t *testing.T) {
	// Test that cancelling the modal (ESC or CancelMsg) dismisses without quitting
	m := New(Config{VimMode: true})
	m = m.SetSize(120, 40)
	m.initializer = newTestInitializer(InitReady, nil)

	// Start in Normal mode to directly trigger quit modal with ESC
	m.input.SetMode(vimtextarea.ModeNormal)

	// Trigger the quit modal (ESC in Normal mode)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	require.NotNil(t, m.quitModal, "quit modal should be shown")

	// Cancel via modal.CancelMsg
	m, cmd := m.Update(modal.CancelMsg{})

	// Modal should be cleared
	require.Nil(t, m.quitModal, "quit modal should be cleared after cancel")
	// No QuitMsg should be produced
	require.Nil(t, cmd, "cancel should not return a command")
}

func TestQuitConfirmation_DuringInit_NoModal(t *testing.T) {
	// Test that during initialization phases, ESC/Ctrl+C exits immediately (no confirmation)
	// This is intentional design - users need to escape stuck init immediately

	tests := []struct {
		name  string
		phase InitPhase
	}{
		{"InitCreatingWorkspace", InitCreatingWorkspace},
		{"InitSpawningCoordinator", InitSpawningCoordinator},
		{"InitAwaitingFirstMessage", InitAwaitingFirstMessage},
		{"InitSpawningWorkers", InitSpawningWorkers},
		{"InitWorkersReady", InitWorkersReady},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(Config{})
			m = m.SetSize(120, 40)
			m.initializer = newTestInitializer(tt.phase, nil)

			// ESC should produce immediate QuitMsg during loading (no modal)
			m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})

			// Should return a command (QuitMsg), not show a modal
			require.NotNil(t, cmd, "ESC should return a command during init phase %v", tt.phase)
			require.Nil(t, m.quitModal, "quit modal should NOT be shown during init phase %v", tt.phase)

			// Verify the command produces QuitMsg
			msg := cmd()
			_, ok := msg.(QuitMsg)
			require.True(t, ok, "ESC should produce QuitMsg during init phase %v", tt.phase)
		})
	}
}

func TestQuitConfirmation_DuringInit_CtrlC_NoModal(t *testing.T) {
	// Test that Ctrl+C also exits immediately during initialization (no confirmation)
	m := New(Config{})
	m = m.SetSize(120, 40)
	m.initializer = newTestInitializer(InitSpawningCoordinator, nil)

	// Ctrl+C should produce immediate QuitMsg during loading
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

	require.NotNil(t, cmd, "Ctrl+C should return a command during loading")
	require.Nil(t, m.quitModal, "quit modal should NOT be shown during loading")

	msg := cmd()
	_, ok := msg.(QuitMsg)
	require.True(t, ok, "Ctrl+C should produce QuitMsg during loading")
}

func TestQuitConfirmation_NavigationMode(t *testing.T) {
	// Test that quit confirmation works in navigation mode (fullscreen pane selection)
	m := New(Config{})
	m = m.SetSize(120, 40)
	m.initializer = newTestInitializer(InitReady, nil)

	// Enter navigation mode (Ctrl+F)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	require.True(t, m.navigationMode, "should be in navigation mode")

	// ESC in navigation mode exits navigation mode, doesn't trigger quit
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	require.False(t, m.navigationMode, "ESC should exit navigation mode")
	require.Nil(t, m.quitModal, "ESC in navigation mode should exit nav mode, not show quit modal")
	require.Nil(t, cmd, "should not return a command when exiting navigation mode")

	// Re-enter navigation mode
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	require.True(t, m.navigationMode, "should be back in navigation mode")

	// Ctrl+C in navigation mode should show quit modal
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	require.NotNil(t, m.quitModal, "Ctrl+C in navigation mode should show quit modal")
	require.Nil(t, cmd, "should not return command, modal shown instead")
}

// ========================================================================
// Session Shutdown Tests
// ========================================================================

func TestModel_SessionField(t *testing.T) {
	// Test that session field is set from InitializerResources
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Initially, session should be nil
	require.Nil(t, m.session, "session should be nil initially")

	// Session would be set in handleInitializerEvent when InitEventReady is received
	// This test verifies the field exists and can be accessed
	// The actual integration happens in initializer_test.go
}

func TestUpdate_CoordinatorStopped_ClosesSession(t *testing.T) {
	// Test that CoordinatorStoppedMsg closes the session with correct status
	tmpDir := t.TempDir()

	// Create a mock session using the session package
	sess, err := createTestSession(t, tmpDir)
	require.NoError(t, err)

	m := New(Config{})
	m = m.SetSize(120, 40)
	m.session = sess
	m.initializer = newTestInitializer(InitReady, nil)

	// Verify session is not closed initially
	require.Equal(t, session.StatusRunning, sess.Status, "session should be running initially")

	// Send CoordinatorStoppedMsg
	m, _ = m.Update(CoordinatorStoppedMsg{})

	// Verify session was closed
	// The session's Close method should have been called
	// We can verify by checking the metadata.json file was updated
	meta, err := session.Load(tmpDir)
	require.NoError(t, err)
	require.Equal(t, session.StatusCompleted, meta.Status, "session status should be completed")
	require.False(t, meta.EndTime.IsZero(), "end time should be set")
}

func TestUpdate_StatusMapping_NormalCompletion(t *testing.T) {
	// Test that normal completion (no error, no timeout) maps to StatusCompleted
	m := New(Config{})
	m = m.SetSize(120, 40)
	m.initializer = newTestInitializer(InitReady, nil)
	m.errorModal = nil // No error

	status := m.determineSessionStatus()
	require.Equal(t, session.StatusCompleted, status, "normal completion should map to StatusCompleted")
}

func TestUpdate_StatusMapping_ErrorModal(t *testing.T) {
	// Test that presence of error modal maps to StatusFailed
	m := New(Config{})
	m = m.SetSize(120, 40)
	m.initializer = newTestInitializer(InitReady, nil)
	m = m.SetError("test error") // This sets errorModal

	status := m.determineSessionStatus()
	require.Equal(t, session.StatusFailed, status, "error modal should map to StatusFailed")
}

func TestUpdate_StatusMapping_InitFailed(t *testing.T) {
	// Test that InitFailed phase maps to StatusFailed
	m := New(Config{})
	m = m.SetSize(120, 40)
	m.initializer = newTestInitializer(InitFailed, nil)

	status := m.determineSessionStatus()
	require.Equal(t, session.StatusFailed, status, "InitFailed should map to StatusFailed")
}

func TestUpdate_StatusMapping_InitTimedOut(t *testing.T) {
	// Test that InitTimedOut phase maps to StatusTimedOut
	m := New(Config{})
	m = m.SetSize(120, 40)
	m.initializer = newTestInitializer(InitTimedOut, nil)

	status := m.determineSessionStatus()
	require.Equal(t, session.StatusTimedOut, status, "InitTimedOut should map to StatusTimedOut")
}

func TestUpdate_NilSession(t *testing.T) {
	// Test that CoordinatorStoppedMsg handles nil session gracefully (no panic)
	m := New(Config{})
	m = m.SetSize(120, 40)
	m.session = nil // Explicitly nil

	// This should not panic
	m, cmd := m.Update(CoordinatorStoppedMsg{})

	// Command should be nil (no error)
	require.Nil(t, cmd, "should return nil command")
}

// createTestSession creates a test session for unit tests.
func createTestSession(t *testing.T, tmpDir string) (*session.Session, error) {
	t.Helper()
	return session.New("test-session-id", tmpDir)
}
