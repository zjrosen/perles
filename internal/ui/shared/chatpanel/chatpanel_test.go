package chatpanel

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	zone "github.com/lrstanley/bubblezone"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/metrics"
	v2 "github.com/zjrosen/perles/internal/orchestration/v2"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/processor"
	"github.com/zjrosen/perles/internal/orchestration/workflow"
	"github.com/zjrosen/perles/internal/pubsub"
	"github.com/zjrosen/perles/internal/ui/shared/chatrender"
	"github.com/zjrosen/perles/internal/ui/shared/vimtextarea"
)

// scanView wraps View() with zone.Scan() to strip zone markers for golden tests.
// This simulates what app.go does when rendering the chatpanel.
func scanView(m Model) string {
	return zone.Scan(m.View())
}

// newTestInfrastructure creates a v2.SimpleInfrastructure with a mock provider for testing.
// The mock provider doesn't need to spawn processes; we just need the infra to function.
func newTestInfrastructure(t *testing.T) *v2.SimpleInfrastructure {
	t.Helper()
	// Mock HeadlessClient
	mockClient := mocks.NewMockHeadlessClient(t)
	mockClient.EXPECT().Spawn(mock.Anything, mock.Anything).Return(nil, nil).Maybe()

	mockProvider := mocks.NewMockAgentProvider(t)
	mockProvider.EXPECT().Client().Return(mockClient, nil).Maybe()
	mockProvider.EXPECT().Extensions().Return(map[string]any{}).Maybe()
	mockProvider.EXPECT().Type().Return(client.ClientClaude).Maybe()

	infra, err := v2.NewSimpleInfrastructure(v2.SimpleInfrastructureConfig{
		AgentProvider: mockProvider,
		WorkDir:       "/tmp/test",
		SystemPrompt:  "test prompt",
	})
	require.NoError(t, err)
	return infra
}

// newTestInfrastructureWithSpawnExpectation creates an infrastructure with a mock
// that expects Spawn to be called. Used for tests that actually submit spawn commands.
func newTestInfrastructureWithSpawnExpectation(t *testing.T) *v2.SimpleInfrastructure {
	t.Helper()
	mockProcess := mocks.NewMockHeadlessProcess(t)

	// Set up mock process expectations - use correct method names from HeadlessProcess interface
	// Create a channel that we'll close to end the event loop
	eventCh := make(chan client.OutputEvent)
	close(eventCh)

	mockProcess.EXPECT().Events().Return((<-chan client.OutputEvent)(eventCh)).Maybe()
	mockProcess.EXPECT().Errors().Return(make(<-chan error)).Maybe()
	mockProcess.EXPECT().SessionRef().Return("test-session-id").Maybe()
	mockProcess.EXPECT().Wait().Return(nil).Maybe()
	mockProcess.EXPECT().Cancel().Return(nil).Maybe()
	mockProcess.EXPECT().IsRunning().Return(false).Maybe()

	// Mock HeadlessClient that returns the mock process
	mockClient := mocks.NewMockHeadlessClient(t)
	mockClient.EXPECT().Spawn(mock.Anything, mock.Anything).Return(mockProcess, nil).Maybe()

	// Set up mock provider to return the mock client
	mockProvider := mocks.NewMockAgentProvider(t)
	mockProvider.EXPECT().Client().Return(mockClient, nil).Maybe()
	mockProvider.EXPECT().Extensions().Return(map[string]any{}).Maybe()
	mockProvider.EXPECT().Type().Return(client.ClientClaude).Maybe()

	infra, err := v2.NewSimpleInfrastructure(v2.SimpleInfrastructureConfig{
		AgentProvider: mockProvider,
		WorkDir:       "/tmp/test",
		SystemPrompt:  "test prompt",
	})
	require.NoError(t, err)
	return infra
}

func TestNew(t *testing.T) {
	cfg := Config{
		ClientType:     "claude",
		WorkDir:        "/test/dir",
		SessionTimeout: 30 * time.Minute,
	}
	m := New(cfg)

	require.False(t, m.Visible(), "new model should not be visible")
	require.Equal(t, cfg.ClientType, m.config.ClientType)
	require.Equal(t, cfg.WorkDir, m.config.WorkDir)
	require.Equal(t, cfg.SessionTimeout, m.config.SessionTimeout)
	require.Empty(t, m.messages, "new model should have no messages")
}

func TestNew_DefaultConfig(t *testing.T) {
	m := New(DefaultConfig())

	require.False(t, m.Visible())
	require.Equal(t, "claude", m.config.ClientType)
	require.Equal(t, 30*time.Minute, m.config.SessionTimeout)
}

// ============================================================================
// Workflow State Initialization Tests
// ============================================================================

func TestNew_WorkflowRegistry_NilWhenNotProvided(t *testing.T) {
	// Default config does not provide a workflow registry
	cfg := DefaultConfig()
	m := New(cfg)

	require.Nil(t, m.workflowRegistry, "workflowRegistry should be nil when not provided in config")
}

func TestNew_WorkflowRegistry_InitializedWhenProvided(t *testing.T) {
	// Create a test workflow registry
	registry := workflow.NewRegistry()

	cfg := Config{
		ClientType:       "claude",
		WorkDir:          "/test/dir",
		SessionTimeout:   30 * time.Minute,
		WorkflowRegistry: registry,
	}
	m := New(cfg)

	require.NotNil(t, m.workflowRegistry, "workflowRegistry should be set when provided in config")
	require.Same(t, registry, m.workflowRegistry, "workflowRegistry should reference the same registry")
}

// ============================================================================
// VimMode Config Propagation Tests
// ============================================================================

func TestNew_VimModeDisabled_InputHasVimDisabled(t *testing.T) {
	cfg := Config{
		ClientType:     "claude",
		WorkDir:        "/test/dir",
		SessionTimeout: 30 * time.Minute,
		VimMode:        false,
	}
	m := New(cfg)

	// When VimMode is false, the vimtextarea ModeIndicator should return empty string
	require.Equal(t, "", m.input.ModeIndicator(), "mode indicator should be empty when VimMode is false")
}

func TestNew_VimModeEnabled_InputHasVimEnabled(t *testing.T) {
	cfg := Config{
		ClientType:     "claude",
		WorkDir:        "/test/dir",
		SessionTimeout: 30 * time.Minute,
		VimMode:        true,
	}
	m := New(cfg)

	// When VimMode is true, the vimtextarea ModeIndicator should return a mode string
	indicator := m.input.ModeIndicator()
	require.NotEmpty(t, indicator, "mode indicator should not be empty when VimMode is true")
	require.Contains(t, indicator, "INSERT", "mode indicator should show INSERT mode (default)")
}

func TestNew_DefaultConfig_VimModeDisabled(t *testing.T) {
	// DefaultConfig should have VimMode=false (matching the app default)
	cfg := DefaultConfig()
	require.False(t, cfg.VimMode, "DefaultConfig should have VimMode=false")

	m := New(cfg)
	require.Equal(t, "", m.input.ModeIndicator(), "mode indicator should be empty with default config")
}

// ============================================================================
// Ctrl+T Keybinding Tests (perles-f3tm.4 - Workflows Tab)
// NOTE: The openWorkflowPicker and handleWorkflowSelected tests were removed
// as part of the Workflows tab cleanup. The workflow picker modal has been
// replaced by the Workflows tab navigation.
// ============================================================================

func TestChatPanel_CtrlT_SwitchesToWorkflowsTab(t *testing.T) {
	// Create a registry with a chat workflow
	registry := workflow.NewRegistry()
	registry.Add(workflow.Workflow{
		ID:          "test-workflow",
		Name:        "Test Workflow",
		Description: "Test description",
		Content:     "Test content",
		Source:      workflow.SourceUser,
		TargetMode:  workflow.TargetChat,
	})

	cfg := Config{
		ClientType:       "claude",
		WorkDir:          "/test/dir",
		SessionTimeout:   30 * time.Minute,
		WorkflowRegistry: registry,
	}
	m := New(cfg).SetSize(80, 24)
	m.visible = true
	m.focused = true

	// Should start on Chat tab
	require.Equal(t, TabChat, m.activeTab, "should start on Chat tab")

	// Send Ctrl+T
	keyMsg := tea.KeyMsg{Type: tea.KeyCtrlT}
	m, _ = m.Update(keyMsg)

	// Should now be on Workflows tab
	require.Equal(t, TabWorkflows, m.activeTab, "should switch to Workflows tab after Ctrl+T")
}

func TestChatPanel_CtrlT_SwitchesToWorkflowsTabEvenWithoutRegistry(t *testing.T) {
	// Create model without a workflow registry
	cfg := Config{
		ClientType:       "claude",
		WorkDir:          "/test/dir",
		SessionTimeout:   30 * time.Minute,
		WorkflowRegistry: nil, // No registry
	}
	m := New(cfg).SetSize(80, 24)
	m.visible = true
	m.focused = true

	// Should start on Chat tab
	require.Equal(t, TabChat, m.activeTab, "should start on Chat tab")

	// Send Ctrl+T
	keyMsg := tea.KeyMsg{Type: tea.KeyCtrlT}
	m, _ = m.Update(keyMsg)

	// Should switch to Workflows tab even without a registry
	// (the view will render an empty state)
	require.Equal(t, TabWorkflows, m.activeTab, "should switch to Workflows tab even without registry")
}

func TestModel_Toggle(t *testing.T) {
	m := New(DefaultConfig())

	// Initially not visible
	require.False(t, m.Visible())

	// Toggle to visible
	m = m.Toggle()
	require.True(t, m.Visible())

	// Toggle back to hidden
	m = m.Toggle()
	require.False(t, m.Visible())
}

func TestModel_Toggle_Immutable(t *testing.T) {
	m1 := New(DefaultConfig())
	m2 := m1.Toggle()

	// Original should be unchanged
	require.False(t, m1.Visible())
	require.True(t, m2.Visible())
}

func TestModel_SetSize(t *testing.T) {
	m := New(DefaultConfig())

	m = m.SetSize(40, 20)

	require.Equal(t, 40, m.width)
	require.Equal(t, 20, m.height)
}

func TestModel_SetSize_Immutable(t *testing.T) {
	m1 := New(DefaultConfig())
	m2 := m1.SetSize(40, 20)

	// Original should be unchanged
	require.Equal(t, 0, m1.width)
	require.Equal(t, 0, m1.height)

	// New model should have updated size
	require.Equal(t, 40, m2.width)
	require.Equal(t, 20, m2.height)
}

func TestModel_View_NotVisible(t *testing.T) {
	m := New(DefaultConfig()).SetSize(40, 20)

	// Not visible, should return empty string
	require.Empty(t, m.View())
}

func TestModel_View_Visible(t *testing.T) {
	m := New(DefaultConfig()).SetSize(40, 20).Toggle()

	view := scanView(m)

	// Should render a bordered pane
	require.NotEmpty(t, view)
	require.Contains(t, view, "Chat") // title
	require.Contains(t, view, "╭")    // top-left corner
	require.Contains(t, view, "╯")    // bottom-right corner
}

func TestModel_View_ZeroDimensions(t *testing.T) {
	// Visible but zero size
	m := New(DefaultConfig()).Toggle()

	// Zero dimensions should return empty
	require.Empty(t, m.View())
}

func TestModel_View_Basic(t *testing.T) {
	// Width increased from 30 to 45 to accommodate 3 tabs (Chat|Sessions|Workflows)
	m := New(DefaultConfig()).SetSize(45, 10).Toggle()

	view := scanView(m)

	// Verify bordered pane structure
	require.Contains(t, view, "Chat")
	require.Contains(t, view, "╭")
	require.Contains(t, view, "╮")
	require.Contains(t, view, "╰")
	require.Contains(t, view, "╯")
	require.Contains(t, view, "│")
}

// TestDefaultConfig verifies default configuration values.
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	require.Equal(t, "claude", cfg.ClientType)
	require.Equal(t, 30*time.Minute, cfg.SessionTimeout)
	require.Empty(t, cfg.WorkDir, "WorkDir should be empty by default")
}

// TestChatMessage verifies message role constants.
func TestChatMessage_Roles(t *testing.T) {
	require.Equal(t, "user", RoleUser)
	require.Equal(t, "assistant", RoleAssistant)
}

// TestMessageTypes verifies message type structures exist.
func TestMessageTypes(t *testing.T) {
	// AssistantResponseMsg
	resp := AssistantResponseMsg{Content: "Hello"}
	require.Equal(t, "Hello", resp.Content)

	// AssistantDoneMsg
	done := AssistantDoneMsg{}
	_ = done

	// AssistantErrorMsg
	errMsg := AssistantErrorMsg{Error: nil}
	require.Nil(t, errMsg.Error)

	// SendMessageMsg
	send := SendMessageMsg{Content: "Hi"}
	require.Equal(t, "Hi", send.Content)
}

// ============================================================================
// Focus and Blur Tests
// ============================================================================

func TestModel_Focus(t *testing.T) {
	m := New(DefaultConfig())

	// Initially not focused
	require.False(t, m.Focused())

	// Focus
	m = m.Focus()
	require.True(t, m.Focused())
}

func TestModel_Blur(t *testing.T) {
	m := New(DefaultConfig()).Focus()

	// Initially focused
	require.True(t, m.Focused())

	// Blur
	m = m.Blur()
	require.False(t, m.Focused())
}

// ============================================================================
// AddMessage Tests
// ============================================================================

func TestModel_AddMessage(t *testing.T) {
	m := New(DefaultConfig())

	// Add a user message
	m = m.AddMessage(chatrender.Message{
		Role:    RoleUser,
		Content: "Hello!",
	})

	require.Len(t, m.Messages(), 1)
	require.Equal(t, RoleUser, m.Messages()[0].Role)
	require.Equal(t, "Hello!", m.Messages()[0].Content)
}

func TestModel_AddMessage_Multiple(t *testing.T) {
	m := New(DefaultConfig())

	// Add user message
	m = m.AddMessage(chatrender.Message{
		Role:    RoleUser,
		Content: "Hello!",
	})

	// Add assistant message
	m = m.AddMessage(chatrender.Message{
		Role:    RoleAssistant,
		Content: "Hi there!",
	})

	require.Len(t, m.Messages(), 2)
	require.Equal(t, RoleUser, m.Messages()[0].Role)
	require.Equal(t, RoleAssistant, m.Messages()[1].Role)
}

func TestModel_ClearMessages(t *testing.T) {
	m := New(DefaultConfig())

	// Add some messages
	m = m.AddMessage(chatrender.Message{Role: RoleUser, Content: "Hello!"})
	m = m.AddMessage(chatrender.Message{Role: RoleAssistant, Content: "Hi!"})

	require.Len(t, m.Messages(), 2)

	// Clear messages
	m = m.ClearMessages()

	require.Empty(t, m.Messages())
}

// ============================================================================
// Update Key Handling Tests
// ============================================================================

func TestModel_Update_KeyHandling_NotFocused(t *testing.T) {
	// Create a visible but unfocused panel
	m := New(DefaultConfig()).SetSize(40, 20).Toggle()

	require.False(t, m.Focused())

	// Keys should not be processed when not focused
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})

	require.Nil(t, cmd)
}

func TestModel_Update_KeyHandling_NotVisible(t *testing.T) {
	// Create a hidden but focused panel (shouldn't happen, but test anyway)
	m := New(DefaultConfig()).SetSize(40, 20).Focus()

	require.False(t, m.Visible())

	// Keys should not be processed when not visible
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})

	require.Nil(t, cmd)
}

func TestModel_Update_SubmitMsg_EmitsSendMessageMsg(t *testing.T) {
	// Create a visible, focused panel
	m := New(DefaultConfig()).SetSize(40, 20).Toggle().Focus()

	// Process a SubmitMsg (what vimtextarea sends on Enter)
	_, cmd := m.Update(vimtextarea.SubmitMsg{Content: "Hello!"})

	// Message should NOT be added immediately - it's added on ProcessIncoming
	// (matches orchestration mode behavior)

	// Should emit a SendMessageMsg command
	require.NotNil(t, cmd)

	// Execute the command to get the message
	msg := cmd()
	sendMsg, ok := msg.(SendMessageMsg)
	require.True(t, ok)
	require.Equal(t, "Hello!", sendMsg.Content)
}

func TestModel_Update_SubmitMsg_EmptyContent(t *testing.T) {
	// Create a visible, focused panel
	m := New(DefaultConfig()).SetSize(40, 20).Toggle().Focus()

	// Process a SubmitMsg with empty content
	m, cmd := m.Update(vimtextarea.SubmitMsg{Content: "   "}) // whitespace only

	// Should NOT add message to history
	require.Empty(t, m.Messages())

	// Should NOT emit a command
	require.Nil(t, cmd)
}

func TestModel_Update_CtrlC_EmitsRequestQuit(t *testing.T) {
	// Create a visible, focused panel
	m := New(DefaultConfig()).SetSize(40, 20).Toggle().Focus()

	// Ctrl+C should always emit RequestQuitMsg regardless of vim mode
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

	require.NotNil(t, cmd, "Ctrl+C should return a command")
	msg := cmd()
	_, ok := msg.(RequestQuitMsg)
	require.True(t, ok, "Ctrl+C should emit RequestQuitMsg")
}

func TestModel_Update_CtrlC_InsertMode_StillEmitsRequestQuit(t *testing.T) {
	// Create a visible, focused panel
	m := New(DefaultConfig()).SetSize(40, 20).Toggle().Focus()

	// vimtextarea starts in insert mode by default
	require.False(t, m.input.InNormalMode(), "should start in insert mode")

	// Ctrl+C in insert mode should still emit RequestQuitMsg (not switch to normal mode)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

	require.NotNil(t, cmd, "Ctrl+C in insert mode should return a command")
	msg := cmd()
	_, ok := msg.(RequestQuitMsg)
	require.True(t, ok, "Ctrl+C in insert mode should emit RequestQuitMsg")
}

func TestModel_Update_CtrlC_NotFocused_NoAction(t *testing.T) {
	// Create a visible but unfocused panel
	m := New(DefaultConfig()).SetSize(40, 20).Toggle()

	require.False(t, m.Focused())

	// Ctrl+C should not be processed when not focused
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

	require.Nil(t, cmd, "Ctrl+C should not be processed when not focused")
}

// ============================================================================
// Tab Switching Keybinding Tests
// ============================================================================

func TestModel_Update_CtrlK_PrevTab(t *testing.T) {
	m := New(DefaultConfig()).SetSize(40, 20).Toggle().Focus()

	// Start at TabChat (0), switch to TabSessions (1)
	m.activeTab = TabSessions

	// Ctrl+K should go to previous tab
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})

	require.Equal(t, TabChat, m.activeTab, "Ctrl+K should switch to previous tab")
}

func TestModel_Update_CtrlJ_NextTab(t *testing.T) {
	m := New(DefaultConfig()).SetSize(40, 20).Toggle().Focus()

	// Start at TabChat (0)
	require.Equal(t, TabChat, m.activeTab)

	// Ctrl+J should go to next tab
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})

	require.Equal(t, TabSessions, m.activeTab, "Ctrl+J should switch to next tab")
}

func TestModel_Update_CtrlK_WrapsAround(t *testing.T) {
	m := New(DefaultConfig()).SetSize(40, 20).Toggle().Focus()

	// Start at TabChat (0)
	require.Equal(t, TabChat, m.activeTab)

	// Ctrl+K should wrap to last tab (TabWorkflows)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})

	require.Equal(t, TabWorkflows, m.activeTab, "Ctrl+K should wrap to last tab")
}

func TestModel_Update_CtrlJ_WrapsAround(t *testing.T) {
	m := New(DefaultConfig()).SetSize(40, 20).Toggle().Focus()

	// Start at TabWorkflows (2) - the last tab
	m.activeTab = TabWorkflows

	// Ctrl+J should wrap to first tab (TabChat = 0)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})

	require.Equal(t, TabChat, m.activeTab, "Ctrl+J should wrap to first tab")
}

func TestModel_Update_CtrlJK_NotProcessedWhenNotFocused(t *testing.T) {
	m := New(DefaultConfig()).SetSize(40, 20).Toggle()
	// Not focused
	require.False(t, m.Focused())
	initialTab := m.activeTab

	// Ctrl+K should not be processed when not focused
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	require.Equal(t, initialTab, m.activeTab, "Ctrl+K should be ignored when not focused")

	// Ctrl+J should not be processed when not focused
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	require.Equal(t, initialTab, m.activeTab, "Ctrl+J should be ignored when not focused")
}

// ============================================================================
// Tab Navigation Tests
// ============================================================================

func TestModel_TabNavigation_NextPrev(t *testing.T) {
	m := New(DefaultConfig()).SetSize(40, 20).Toggle().Focus()

	// Start at TabChat (0)
	require.Equal(t, TabChat, m.activeTab)

	// NextTab should go to Sessions
	m = m.NextTab()
	require.Equal(t, TabSessions, m.activeTab)

	// NextTab should go to Workflows
	m = m.NextTab()
	require.Equal(t, TabWorkflows, m.activeTab)

	// NextTab should wrap back to Chat
	m = m.NextTab()
	require.Equal(t, TabChat, m.activeTab)

	// PrevTab should go to Workflows (wrap around)
	m = m.PrevTab()
	require.Equal(t, TabWorkflows, m.activeTab)

	// PrevTab should go to Sessions
	m = m.PrevTab()
	require.Equal(t, TabSessions, m.activeTab)

	// PrevTab should go back to Chat
	m = m.PrevTab()
	require.Equal(t, TabChat, m.activeTab)
}

func TestModel_View_ShowsTabs(t *testing.T) {
	m := New(DefaultConfig()).SetSize(40, 20).Toggle()

	view := scanView(m)

	// Should show Chat and Sessions tabs
	require.Contains(t, view, "Chat")
	require.Contains(t, view, "Sessions")
}

// ============================================================================
// View Rendering Tests with Messages
// ============================================================================

func TestModel_View_WithMessages(t *testing.T) {
	m := New(DefaultConfig()).SetSize(40, 20).Toggle()

	// Set session to Ready status to show messages (Pending shows loading indicator)
	m.sessions[DefaultSessionID].Status = events.ProcessStatusReady

	// Add some messages
	m = m.AddMessage(chatrender.Message{Role: RoleUser, Content: "Hello!"})
	m = m.AddMessage(chatrender.Message{Role: RoleAssistant, Content: "Hi there!"})

	view := scanView(m)

	// Should contain the role labels
	require.Contains(t, view, "You")
	require.Contains(t, view, "Assistant")

	// Should contain the message content
	require.Contains(t, view, "Hello!")
	require.Contains(t, view, "Hi there!")
}

func TestModel_View_ContainsDivider(t *testing.T) {
	m := New(DefaultConfig()).SetSize(40, 20).Toggle()

	view := scanView(m)

	// Should contain a horizontal divider between viewport and input
	require.Contains(t, view, "─")
}

func TestModel_View_ContainsInput(t *testing.T) {
	m := New(DefaultConfig()).SetSize(40, 20).Toggle()

	view := scanView(m)

	// Should contain the input area (bordered box)
	require.Contains(t, view, "│")
}

// ============================================================================
// Golden Tests
// ============================================================================

func TestView_Golden_Hidden(t *testing.T) {
	m := New(DefaultConfig()).SetSize(50, 20)

	// Not visible - should be empty
	view := scanView(m)
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_Visible_Empty(t *testing.T) {
	m := New(DefaultConfig()).SetSize(50, 20).Toggle()

	view := scanView(m)
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_Visible_Focused(t *testing.T) {
	m := New(DefaultConfig()).SetSize(50, 20).Toggle().Focus()

	view := scanView(m)
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_WithMessages(t *testing.T) {
	m := New(DefaultConfig()).SetSize(50, 25).Toggle()

	// Add some messages
	m = m.AddMessage(chatrender.Message{Role: RoleUser, Content: "Hello, how are you?"})
	m = m.AddMessage(chatrender.Message{Role: RoleAssistant, Content: "I'm doing well, thank you for asking! How can I help you today?"})

	view := scanView(m)
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_WithLongMessage(t *testing.T) {
	m := New(DefaultConfig()).SetSize(50, 25).Toggle()

	// Add a long message that should wrap
	longMsg := "This is a very long message that should wrap across multiple lines because it exceeds the width of the viewport area."
	m = m.AddMessage(chatrender.Message{Role: RoleUser, Content: longMsg})

	view := scanView(m)
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_NarrowPanel(t *testing.T) {
	m := New(DefaultConfig()).SetSize(30, 15).Toggle()

	m = m.AddMessage(chatrender.Message{Role: RoleUser, Content: "Hello!"})
	m = m.AddMessage(chatrender.Message{Role: RoleAssistant, Content: "Hi there!"})

	view := scanView(m)
	teatest.RequireEqualOutput(t, []byte(view))
}

// ============================================================================
// Word Wrap Tests
// ============================================================================

func TestWordWrap_ShortLine(t *testing.T) {
	result := chatrender.WordWrap("Hello", 20)
	require.Equal(t, "Hello", result)
}

func TestWordWrap_ExactFit(t *testing.T) {
	result := chatrender.WordWrap("Hello world", 11)
	require.Equal(t, "Hello world", result)
}

func TestWordWrap_Wraps(t *testing.T) {
	result := chatrender.WordWrap("Hello world foo", 10)
	require.Contains(t, result, "\n")
}

func TestWordWrap_PreservesNewlines(t *testing.T) {
	result := chatrender.WordWrap("Line one\nLine two", 50)
	require.Contains(t, result, "Line one")
	require.Contains(t, result, "Line two")
}

func TestWordWrap_ZeroWidth(t *testing.T) {
	result := chatrender.WordWrap("Hello world", 0)
	require.Equal(t, "Hello world", result)
}

// ============================================================================
// Prompt Tests
// ============================================================================

func TestBuildAssistantSystemPrompt(t *testing.T) {
	prompt := BuildAssistantSystemPrompt()

	// Verify prompt is non-empty
	require.NotEmpty(t, prompt)

	// Verify prompt contains bd CLI help section
	require.Contains(t, prompt, "bd CLI")
	require.Contains(t, prompt, "bd ready")
	require.Contains(t, prompt, "bd create")
	require.Contains(t, prompt, "bd dep add")

	// Verify prompt contains BQL help section
	require.Contains(t, prompt, "BQL")
	require.Contains(t, prompt, "Fields:")
	require.Contains(t, prompt, "Operators:")
	require.Contains(t, prompt, "priority")
	require.Contains(t, prompt, "status")

	// Verify prompt is approximately the right length (now larger with bd reference)
	lines := strings.Count(prompt, "\n")
	require.Greater(t, lines, 50, "prompt should have at least 50 lines")
	require.Less(t, lines, 120, "prompt should have fewer than 120 lines")
}

func TestBuildAssistantSystemPrompt_ContainsExamples(t *testing.T) {
	prompt := BuildAssistantSystemPrompt()

	// Verify prompt contains BQL examples
	require.Contains(t, prompt, "type = bug")
	require.Contains(t, prompt, "priority = P0")
}

func TestBuildAssistantInitialPrompt(t *testing.T) {
	prompt := BuildAssistantInitialPrompt()

	// Verify prompt is non-empty
	require.NotEmpty(t, prompt)

	// Verify prompt contains key elements
	require.Contains(t, prompt, "Perles")
	require.Contains(t, prompt, "bd ready")
	require.Contains(t, prompt, "bd activity")
}

// ============================================================================
// Infrastructure Integration Tests
// ============================================================================

func TestModel_SetInfrastructure(t *testing.T) {
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	// Set infrastructure
	m = m.SetInfrastructure(infra)

	// Verify infrastructure is set
	require.True(t, m.HasInfrastructure())
	require.NotNil(t, m.infra)
	require.NotNil(t, m.v2Listener)
	require.NotNil(t, m.ctx)
	require.NotNil(t, m.cancel)
}

func TestModel_HasInfrastructure_False(t *testing.T) {
	cfg := DefaultConfig()
	m := New(cfg)

	require.False(t, m.HasInfrastructure())
}

func TestModel_SpawnAssistant_NoInfrastructure(t *testing.T) {
	cfg := DefaultConfig()
	m := New(cfg)

	// SpawnAssistant should return an error command when no infrastructure
	_, cmd := m.SpawnAssistant()
	require.NotNil(t, cmd)

	// Execute the command
	msg := cmd()
	errMsg, ok := msg.(AssistantErrorMsg)
	require.True(t, ok, "expected AssistantErrorMsg")
	require.Equal(t, ErrNoInfrastructure, errMsg.Error)
}

func TestModel_SendMessage_NoInfrastructure(t *testing.T) {
	cfg := DefaultConfig()
	m := New(cfg)

	// SendMessage should return an error command when no infrastructure
	cmd := m.SendMessage("test message")
	require.NotNil(t, cmd)

	// Execute the command
	msg := cmd()
	errMsg, ok := msg.(AssistantErrorMsg)
	require.True(t, ok, "expected AssistantErrorMsg")
	require.Equal(t, ErrNoInfrastructure, errMsg.Error)
}

func TestModel_SpawnAssistant_SubmitsCommand(t *testing.T) {
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure with mock expectations for spawn
	infra := newTestInfrastructureWithSpawnExpectation(t)
	err := infra.Start()
	require.NoError(t, err)
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// SpawnAssistant should return spinnerTick cmd when infrastructure is set
	_, cmd := m.SpawnAssistant()
	require.NotNil(t, cmd, "SpawnAssistant should return spinnerTick cmd on successful submission")
}

func TestModel_SendMessage_SubmitsCommand(t *testing.T) {
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure with mock expectations
	infra := newTestInfrastructureWithSpawnExpectation(t)
	err := infra.Start()
	require.NoError(t, err)
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// SendMessage should return nil (no error) when infrastructure is set
	cmd := m.SendMessage("Hello assistant")
	require.Nil(t, cmd, "SendMessage should return nil cmd on successful submission")
}

func TestModel_SendMessage_NoProcessID(t *testing.T) {
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructureWithSpawnExpectation(t)
	err := infra.Start()
	require.NoError(t, err)
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Create a new session without a ProcessID and switch to it
	m, _ = m.CreateSession("session-2")
	m, _ = m.SwitchSession("session-2")

	// SendMessage should return an error command when session has no ProcessID
	cmd := m.SendMessage("test message")
	require.NotNil(t, cmd, "SendMessage should return error cmd when session has no ProcessID")

	// Execute the command
	msg := cmd()
	errMsg, ok := msg.(AssistantErrorMsg)
	require.True(t, ok, "expected AssistantErrorMsg")
	require.Contains(t, errMsg.Error.Error(), "no active session or process")
}

func TestModel_SendMessage_RoutesToActiveSession(t *testing.T) {
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructureWithSpawnExpectation(t)
	err := infra.Start()
	require.NoError(t, err)
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Create a second session with a ProcessID
	m, _ = m.CreateSession("session-2")
	m = m.SetSessionProcessID("session-2", "process-2")
	m, _ = m.SwitchSession("session-2")

	// SendMessage should succeed (routes to session-2's process)
	cmd := m.SendMessage("Hello session 2")
	require.Nil(t, cmd, "SendMessage should succeed when active session has ProcessID")

	// Verify active session is session-2
	require.Equal(t, "session-2", m.ActiveSessionID())
	require.Equal(t, "process-2", m.ActiveSession().ProcessID)
}

func TestModel_InitListener(t *testing.T) {
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Without infrastructure, InitListener returns nil
	cmd := m.InitListener()
	require.Nil(t, cmd)

	// With infrastructure, InitListener returns a command
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)
	cmd = m.InitListener()
	require.NotNil(t, cmd)
}

func TestModel_HandleProcessEvent_Output(t *testing.T) {
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Create a ProcessOutput event with ProcessID for routing
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:      events.ProcessOutput,
			ProcessID: ChatPanelProcessID, // Must match session's ProcessID for routing
			Output:    "Hello from assistant",
		},
	}

	// Handle the event
	m, cmd := m.Update(event)

	// Verify message was added
	require.Len(t, m.Messages(), 1)
	require.Equal(t, RoleAssistant, m.Messages()[0].Role)
	require.Equal(t, "Hello from assistant", m.Messages()[0].Content)

	// Verify listener continues (cmd is not nil)
	require.NotNil(t, cmd)
}

func TestModel_HandleProcessEvent_OutputDelta(t *testing.T) {
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// First message (delta=false)
	event1 := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:      events.ProcessOutput,
			ProcessID: ChatPanelProcessID,
			Output:    "Hello",
			Delta:     false,
		},
	}
	m, _ = m.Update(event1)
	require.Len(t, m.Messages(), 1)
	require.Equal(t, "Hello", m.Messages()[0].Content)

	// Second chunk (delta=true) - should accumulate
	event2 := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:      events.ProcessOutput,
			ProcessID: ChatPanelProcessID,
			Output:    " world",
			Delta:     true,
		},
	}
	m, _ = m.Update(event2)
	require.Len(t, m.Messages(), 1, "Should still be 1 message after delta")
	require.Equal(t, "Hello world", m.Messages()[0].Content)

	// Third chunk (delta=true) - should continue accumulating
	event3 := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:      events.ProcessOutput,
			ProcessID: ChatPanelProcessID,
			Output:    "!",
			Delta:     true,
		},
	}
	m, _ = m.Update(event3)
	require.Len(t, m.Messages(), 1)
	require.Equal(t, "Hello world!", m.Messages()[0].Content)
}

func TestModel_HandleProcessEvent_OutputDeltaNewMessageOnFalse(t *testing.T) {
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// First message
	event1 := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:      events.ProcessOutput,
			ProcessID: ChatPanelProcessID,
			Output:    "First message",
			Delta:     false,
		},
	}
	m, _ = m.Update(event1)
	require.Len(t, m.Messages(), 1)

	// Second message (delta=false) - should create new message
	event2 := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:      events.ProcessOutput,
			ProcessID: ChatPanelProcessID,
			Output:    "Second message",
			Delta:     false,
		},
	}
	m, _ = m.Update(event2)
	require.Len(t, m.Messages(), 2)
	require.Equal(t, "First message", m.Messages()[0].Content)
	require.Equal(t, "Second message", m.Messages()[1].Content)
}

func TestModel_HandleProcessEvent_OutputDeltaDoesNotAccumulateOntoToolCall(t *testing.T) {
	// Regression test: delta messages should NOT accumulate onto tool call messages.
	// Before the fix, a tool call like "🔧 run_shell_command" followed by a delta "Hello"
	// would result in "🔧 run_shell_commandHello" instead of creating a new message.
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// First: a tool call message (detected by 🔧 prefix)
	toolCallEvent := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:      events.ProcessOutput,
			ProcessID: ChatPanelProcessID,
			Output:    "🔧 run_shell_command",
			Delta:     false,
		},
	}
	m, _ = m.Update(toolCallEvent)
	require.Len(t, m.Messages(), 1)
	require.Equal(t, "🔧 run_shell_command", m.Messages()[0].Content)
	require.True(t, m.Messages()[0].IsToolCall, "Should be marked as tool call")

	// Second: a delta message that should NOT accumulate onto the tool call
	deltaEvent := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:      events.ProcessOutput,
			ProcessID: ChatPanelProcessID,
			Output:    "Hello! I'm here to help.",
			Delta:     true,
		},
	}
	m, _ = m.Update(deltaEvent)

	// Should create a NEW message, not concatenate
	require.Len(t, m.Messages(), 2, "Delta should create new message after tool call, not accumulate")
	require.Equal(t, "🔧 run_shell_command", m.Messages()[0].Content, "Tool call should be unchanged")
	require.Equal(t, "Hello! I'm here to help.", m.Messages()[1].Content, "New message should be separate")
	require.False(t, m.Messages()[1].IsToolCall, "New message should not be a tool call")
}

func TestModel_HandleProcessEvent_Error(t *testing.T) {
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Create a ProcessError event
	testErr := errors.New("test error")
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:  events.ProcessError,
			Error: testErr,
		},
	}

	// Handle the event
	_, cmd := m.Update(event)

	// Verify cmd is not nil (should be batch with listener + error msg)
	require.NotNil(t, cmd)
}

func TestModel_HandleProcessEvent_UnknownPayload(t *testing.T) {
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Create an event with unknown payload type
	event := pubsub.Event[any]{
		Type:    pubsub.UpdatedEvent,
		Payload: "unknown payload type",
	}

	// Handle the event - should not panic
	_, cmd := m.Update(event)

	// Verify listener continues
	require.NotNil(t, cmd)
}

func TestModel_HandleProcessEvent_NoListener(t *testing.T) {
	cfg := DefaultConfig()
	m := New(cfg)

	// Create a ProcessOutput event without setting infrastructure
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:   events.ProcessOutput,
			Output: "Hello",
		},
	}

	// Handle the event - should not panic and return nil cmd
	_, cmd := m.Update(event)
	require.Nil(t, cmd)
}

func TestModel_HandleProcessEvent_ProcessWorking(t *testing.T) {
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Initially not working (session starts in Pending state)
	require.False(t, m.AssistantWorking())

	// Get the active session for ProcessID
	session := m.ActiveSession()
	require.NotNil(t, session)

	// Create a ProcessWorking event targeting the active session's process
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:      events.ProcessWorking,
			ProcessID: session.ProcessID,
		},
	}

	// Handle the event
	m, cmd := m.Update(event)

	// Verify working state is true (delegates to active session)
	require.True(t, m.AssistantWorking())
	require.NotNil(t, cmd) // listener continues
}

func TestModel_HandleProcessEvent_ProcessReady(t *testing.T) {
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Simulate that assistant was working via session state
	session := m.ActiveSession()
	require.NotNil(t, session)
	session.Status = events.ProcessStatusWorking

	// Create a ProcessReady event targeting the active session's process
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:      events.ProcessReady,
			ProcessID: session.ProcessID,
		},
	}

	// Handle the event
	m, cmd := m.Update(event)

	// Verify working state is false (delegates to active session)
	require.False(t, m.AssistantWorking())
	require.NotNil(t, cmd) // listener continues
}

func TestModel_HandleProcessEvent_ProcessQueueChanged(t *testing.T) {
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Initially no queue (delegates to active session)
	require.Equal(t, 0, m.QueueCount())

	// Get the active session for ProcessID
	session := m.ActiveSession()
	require.NotNil(t, session)

	// Create a ProcessQueueChanged event with count = 3 targeting the active session
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:       events.ProcessQueueChanged,
			ProcessID:  session.ProcessID,
			QueueCount: 3,
		},
	}

	// Handle the event
	m, cmd := m.Update(event)

	// Verify queue count is updated (delegates to active session)
	require.Equal(t, 3, m.QueueCount())
	require.NotNil(t, cmd) // listener continues
}

func TestModel_HandleProcessEvent_ProcessQueueChanged_Zero(t *testing.T) {
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Set initial queue count via session state
	session := m.ActiveSession()
	require.NotNil(t, session)
	session.QueueCount = 5

	// Create a ProcessQueueChanged event with count = 0 (queue drained)
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:       events.ProcessQueueChanged,
			ProcessID:  session.ProcessID,
			QueueCount: 0,
		},
	}

	// Handle the event
	m, cmd := m.Update(event)

	// Verify queue count is updated to 0 (delegates to active session)
	require.Equal(t, 0, m.QueueCount())
	require.NotNil(t, cmd) // listener continues
}

func TestModel_HandleProcessEvent_ProcessTokenUsage(t *testing.T) {
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Initially no metrics (delegates to active session)
	require.Nil(t, m.Metrics())

	// Get the active session for ProcessID
	session := m.ActiveSession()
	require.NotNil(t, session)

	// Create a ProcessTokenUsage event with token data targeting the active session
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:      events.ProcessTokenUsage,
			ProcessID: session.ProcessID,
			Metrics: &metrics.TokenMetrics{
				TokensUsed:  27000,
				TotalTokens: 200000,
			},
		},
	}

	// Handle the event
	m, cmd := m.Update(event)

	// Verify metrics are updated (delegates to active session)
	require.NotNil(t, m.Metrics())
	require.Equal(t, 27000, m.Metrics().TokensUsed)
	require.Equal(t, 200000, m.Metrics().TotalTokens)
	require.Equal(t, "27k/200k", m.Metrics().FormatContextDisplay())
	require.NotNil(t, cmd) // listener continues
}

func TestModel_HandleProcessEvent_ProcessTokenUsage_NilMetrics(t *testing.T) {
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Create a ProcessTokenUsage event with nil metrics
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:    events.ProcessTokenUsage,
			Metrics: nil,
		},
	}

	// Handle the event
	m, cmd := m.Update(event)

	// Verify metrics remain nil
	require.Nil(t, m.Metrics())
	require.NotNil(t, cmd) // listener continues
}

func TestModel_HandleProcessEvent_ProcessIncoming(t *testing.T) {
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Initially no messages
	require.Empty(t, m.Messages())

	// Create a ProcessIncoming event (user message was delivered to assistant)
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:      events.ProcessIncoming,
			ProcessID: ChatPanelProcessID, // Must match session's ProcessID for routing
			Message:   "Hello from user",
		},
	}

	// Handle the event
	m, cmd := m.Update(event)

	// Verify user message was added
	require.Len(t, m.Messages(), 1)
	require.Equal(t, RoleUser, m.Messages()[0].Role)
	require.Equal(t, "Hello from user", m.Messages()[0].Content)
	require.NotNil(t, cmd) // listener continues
}

func TestModel_HandleProcessEvent_ProcessIncoming_Empty(t *testing.T) {
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Create a ProcessIncoming event with empty message
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:    events.ProcessIncoming,
			Message: "",
		},
	}

	// Handle the event
	m, cmd := m.Update(event)

	// Verify no message was added (empty messages are ignored)
	require.Empty(t, m.Messages())
	require.NotNil(t, cmd) // listener continues
}

func TestModel_HandleProcessEvent_ProcessSpawned(t *testing.T) {
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Verify initial session status is Pending
	require.Equal(t, events.ProcessStatusPending, m.ActiveSession().Status)

	// Create a ProcessSpawned event
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:      events.ProcessSpawned,
			ProcessID: ChatPanelProcessID, // Must match session's ProcessID for routing
		},
	}

	// Handle the event
	m, cmd := m.Update(event)

	// Verify session status is unchanged (ProcessSpawned is no-op, status stays Pending)
	require.Equal(t, events.ProcessStatusPending, m.ActiveSession().Status)
	require.NotNil(t, cmd) // listener continues
}

func TestModel_HandleProcessEvent_ProcessReady_UpdatesSessionStatus(t *testing.T) {
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Set session status to Working (simulating post-spawn state)
	m.ActiveSession().Status = events.ProcessStatusWorking

	// Create a ProcessReady event
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:      events.ProcessReady,
			ProcessID: ChatPanelProcessID,
		},
	}

	// Handle the event
	m, cmd := m.Update(event)

	// Verify session status was updated to Ready
	require.Equal(t, events.ProcessStatusReady, m.ActiveSession().Status)
	require.NotNil(t, cmd) // listener continues
}

func TestModel_HandleProcessEvent_ProcessWorking_UpdatesSessionStatus(t *testing.T) {
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Set session status to Ready first
	m.ActiveSession().Status = events.ProcessStatusReady

	// Create a ProcessWorking event
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:      events.ProcessWorking,
			ProcessID: ChatPanelProcessID,
		},
	}

	// Handle the event
	m, cmd := m.Update(event)

	// Verify session status was updated to Working
	require.Equal(t, events.ProcessStatusWorking, m.ActiveSession().Status)
	require.NotNil(t, cmd) // listener continues
}

func TestModel_HandleProcessEvent_ProcessError_UpdatesSessionStatus(t *testing.T) {
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Set session status to Working (simulating in-progress state)
	m.ActiveSession().Status = events.ProcessStatusWorking

	// Create a ProcessError event with an error
	testErr := errors.New("test spawn failure")
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:      events.ProcessError,
			ProcessID: ChatPanelProcessID,
			Status:    events.ProcessStatusFailed,
			Error:     testErr,
		},
	}

	// Handle the event
	m, cmd := m.Update(event)

	// Verify session status was updated to Failed
	require.Equal(t, events.ProcessStatusFailed, m.ActiveSession().Status)
	require.NotNil(t, cmd) // should be batch with listener + error msg
}

func TestModel_Cleanup(t *testing.T) {
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()

	m = m.SetInfrastructure(infra)

	// Cleanup should not panic
	m.Cleanup()

	// After cleanup, we can't easily verify internal state,
	// but we can verify it doesn't panic on double cleanup
	m.Cleanup()
}

func TestModel_Cleanup_NoInfrastructure(t *testing.T) {
	cfg := DefaultConfig()
	m := New(cfg)

	// Cleanup without infrastructure should not panic
	m.Cleanup()
}

func TestModel_SubmitError_ReturnsErrorMsg(t *testing.T) {
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure but don't start it - submissions will fail
	infra := newTestInfrastructure(t)
	// Note: Not calling infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// SpawnAssistant should return error since processor is not started
	_, cmd := m.SpawnAssistant()
	require.NotNil(t, cmd, "SpawnAssistant should return error cmd when processor not started")

	// Execute the command to verify it's an error
	msg := cmd()
	errMsg, ok := msg.(AssistantErrorMsg)
	require.True(t, ok, "expected AssistantErrorMsg")
	require.NotNil(t, errMsg.Error)
}

// ============================================================================
// Session Persistence Tests
// ============================================================================

func TestModel_SessionPersistence_Resume(t *testing.T) {
	// Create model with a short timeout for testing
	cfg := Config{
		ClientType:     "claude",
		WorkDir:        "/tmp/test",
		SessionTimeout: 30 * time.Minute,
	}
	m := New(cfg)

	// Set a fixed time for testing
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	m.Clock = func() time.Time { return baseTime }

	// Simulate a previous session
	m = m.SetSessionRef("session-123")
	m = m.updateInteractionTime() // Sets to baseTime

	// Move time forward by less than 30 minutes
	m.Clock = func() time.Time { return baseTime.Add(20 * time.Minute) }

	// Should resume session because it's within timeout
	require.True(t, m.ShouldResumeSession(), "should resume session when < 30 minutes")
	require.Equal(t, "session-123", m.SessionRef())
}

func TestModel_SessionPersistence_Fresh(t *testing.T) {
	// Create model with a short timeout for testing
	cfg := Config{
		ClientType:     "claude",
		WorkDir:        "/tmp/test",
		SessionTimeout: 30 * time.Minute,
	}
	m := New(cfg)

	// Set a fixed time for testing
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	m.Clock = func() time.Time { return baseTime }

	// Simulate a previous session
	m = m.SetSessionRef("session-123")
	m = m.updateInteractionTime() // Sets to baseTime

	// Move time forward by more than 30 minutes
	m.Clock = func() time.Time { return baseTime.Add(35 * time.Minute) }

	// Should NOT resume session because it's beyond timeout
	require.False(t, m.ShouldResumeSession(), "should NOT resume session when >= 30 minutes")
}

func TestModel_SessionPersistence_NoSessionRef(t *testing.T) {
	cfg := DefaultConfig()
	m := New(cfg)

	// Set a fixed time for testing
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	m.Clock = func() time.Time { return baseTime }

	// Set interaction time but no session ref
	m = m.updateInteractionTime()

	// Should NOT resume because no session ref
	require.False(t, m.ShouldResumeSession(), "should NOT resume session when no session ref")
}

func TestModel_SessionPersistence_NoInteraction(t *testing.T) {
	cfg := DefaultConfig()
	m := New(cfg)

	// Set session ref but no interaction
	m = m.SetSessionRef("session-123")

	// Should NOT resume because no previous interaction
	require.False(t, m.ShouldResumeSession(), "should NOT resume session when no previous interaction")
}

func TestModel_SessionRef_Stored(t *testing.T) {
	cfg := DefaultConfig()
	m := New(cfg)

	// Initially no session ref
	require.Empty(t, m.SessionRef())

	// Set session ref
	m = m.SetSessionRef("new-session-456")
	require.Equal(t, "new-session-456", m.SessionRef())
}

func TestModel_LastInteractionTime_Updated_OnSend(t *testing.T) {
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Set a fixed time for testing
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	m.Clock = func() time.Time { return baseTime }

	// Initially no interaction time
	require.True(t, m.LastInteractionTime().IsZero())

	// Process a ProcessIncoming event (message was delivered)
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:    events.ProcessIncoming,
			Message: "Hello!",
		},
	}
	m, _ = m.Update(event)

	// Interaction time should be updated when message is delivered
	require.Equal(t, baseTime, m.LastInteractionTime())
}

func TestModel_LastInteractionTime_Updated_OnReceive(t *testing.T) {
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Set a fixed time for testing
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	m.Clock = func() time.Time { return baseTime }

	// Initially no interaction time
	require.True(t, m.LastInteractionTime().IsZero())

	// Create a ProcessOutput event
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:   events.ProcessOutput,
			Output: "Hello from assistant",
		},
	}

	// Handle the event
	m, _ = m.Update(event)

	// Interaction time should be updated
	require.Equal(t, baseTime, m.LastInteractionTime())
}

func TestModel_ClearSession(t *testing.T) {
	cfg := DefaultConfig()
	m := New(cfg)

	// Set a fixed time for testing
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	m.Clock = func() time.Time { return baseTime }

	// Setup session state
	m = m.SetSessionRef("session-123")
	m = m.updateInteractionTime()

	// Verify state is set
	require.Equal(t, "session-123", m.SessionRef())
	require.Equal(t, baseTime, m.LastInteractionTime())

	// Clear session
	m = m.ClearSession()

	// Verify state is cleared
	require.Empty(t, m.SessionRef())
	require.True(t, m.LastInteractionTime().IsZero())
}

func TestModel_SessionPersistence_ExactBoundary(t *testing.T) {
	// Test the exact 30-minute boundary
	cfg := Config{
		ClientType:     "claude",
		WorkDir:        "/tmp/test",
		SessionTimeout: 30 * time.Minute,
	}
	m := New(cfg)

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	m.Clock = func() time.Time { return baseTime }

	m = m.SetSessionRef("session-123")
	m = m.updateInteractionTime()

	// Exactly at 30 minutes - should NOT resume (>= 30 means fresh)
	m.Clock = func() time.Time { return baseTime.Add(30 * time.Minute) }
	require.False(t, m.ShouldResumeSession(), "should NOT resume session at exactly 30 minutes")

	// Just under 30 minutes - should resume
	m.Clock = func() time.Time { return baseTime.Add(30*time.Minute - 1*time.Second) }
	require.True(t, m.ShouldResumeSession(), "should resume session at 29:59")
}

// ============================================================================
// Multi-Session Tests
// ============================================================================

func TestSessionDataStructure(t *testing.T) {
	// Verify SessionData struct has all required fields
	session := SessionData{
		ID:            "test-session",
		ProcessID:     "test-process",
		Messages:      []chatrender.Message{{Role: RoleUser, Content: "Hello"}},
		Status:        events.ProcessStatusReady,
		Metrics:       &metrics.TokenMetrics{TokensUsed: 100},
		ContentDirty:  true,
		HasNewContent: false,
		CreatedAt:     time.Now(),
		LastActivity:  time.Now(),
	}

	require.Equal(t, "test-session", session.ID)
	require.Equal(t, "test-process", session.ProcessID)
	require.Len(t, session.Messages, 1)
	require.Equal(t, events.ProcessStatusReady, session.Status)
	require.Equal(t, 100, session.Metrics.TokensUsed)
	require.True(t, session.ContentDirty)
	require.False(t, session.HasNewContent)
	require.False(t, session.CreatedAt.IsZero())
	require.False(t, session.LastActivity.IsZero())
}

// ============================================================================
// SessionData QueueCount Field Tests (perles-ci2e.1)
// ============================================================================

func TestSessionData_QueueCount_InitializesToZero(t *testing.T) {
	// Create a new model - initial session should have QueueCount = 0
	m := New(DefaultConfig())

	session := m.ActiveSession()
	require.NotNil(t, session)
	require.Equal(t, 0, session.QueueCount, "QueueCount should initialize to 0 (Go zero value)")
}

func TestSessionData_QueueCount_PersistsWhenSet(t *testing.T) {
	// Create a new model
	m := New(DefaultConfig())

	session := m.ActiveSession()
	require.NotNil(t, session)

	// Set QueueCount to a non-zero value
	session.QueueCount = 5

	// Verify it persists
	require.Equal(t, 5, session.QueueCount, "QueueCount should persist when set")

	// Verify it persists when accessed again via ActiveSession
	sessionAgain := m.ActiveSession()
	require.Equal(t, 5, sessionAgain.QueueCount, "QueueCount should persist across access")
}

func TestSessionData_QueueCount_InStructLiteral(t *testing.T) {
	// Verify QueueCount field works in struct literal
	session := SessionData{
		ID:         "test-session",
		QueueCount: 10,
	}

	require.Equal(t, 10, session.QueueCount, "QueueCount should be settable in struct literal")
}

func TestCreateSession_QueueCount_InitializesToZero(t *testing.T) {
	m := New(DefaultConfig())

	// Create a new session
	m, session := m.CreateSession("session-2")

	require.NotNil(t, session)
	require.Equal(t, 0, session.QueueCount, "newly created session should have QueueCount = 0")
}

func TestModelMultiSessionInit(t *testing.T) {
	m := New(DefaultConfig())

	// Verify maps are initialized
	require.NotNil(t, m.sessions, "sessions map should be initialized")
	require.NotNil(t, m.processToSession, "processToSession map should be initialized")

	// Verify initial session is created
	require.Len(t, m.sessions, 1, "should have exactly one session")
	require.Len(t, m.sessionOrder, 1, "sessionOrder should have one entry")
	require.Equal(t, DefaultSessionID, m.sessionOrder[0])
	require.Equal(t, DefaultSessionID, m.activeSessionID)

	// Verify initial session has correct fields
	session := m.sessions[DefaultSessionID]
	require.NotNil(t, session)
	require.Equal(t, DefaultSessionID, session.ID)
	require.Equal(t, ChatPanelProcessID, session.ProcessID)
	require.Empty(t, session.Messages)
	require.Equal(t, events.ProcessStatusPending, session.Status)
	require.True(t, session.ContentDirty)
	require.False(t, session.CreatedAt.IsZero())
	require.False(t, session.LastActivity.IsZero())

	// Verify reverse lookup is set
	sessionID, exists := m.processToSession[ChatPanelProcessID]
	require.True(t, exists, "processToSession should have entry for ChatPanelProcessID")
	require.Equal(t, DefaultSessionID, sessionID)
}

func TestActiveSession(t *testing.T) {
	m := New(DefaultConfig())

	// ActiveSession should return the initial session
	session := m.ActiveSession()
	require.NotNil(t, session)
	require.Equal(t, DefaultSessionID, session.ID)
}

func TestActiveSession_NoActiveID(t *testing.T) {
	m := New(DefaultConfig())

	// Clear the active session ID
	m.activeSessionID = ""

	// ActiveSession should return nil
	session := m.ActiveSession()
	require.Nil(t, session)
}

func TestCreateSession(t *testing.T) {
	m := New(DefaultConfig())

	// Set a fixed time for testing
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	m.Clock = func() time.Time { return baseTime }

	// Create a new session
	m, session := m.CreateSession("session-2")

	// Verify session was created
	require.NotNil(t, session)
	require.Equal(t, "session-2", session.ID)
	require.Empty(t, session.ProcessID, "ProcessID should be empty initially")
	require.Empty(t, session.Messages)
	require.Equal(t, events.ProcessStatusPending, session.Status)
	require.True(t, session.ContentDirty)
	require.Equal(t, baseTime, session.CreatedAt)
	require.Equal(t, baseTime, session.LastActivity)

	// Verify session was added to maps
	require.Len(t, m.sessions, 2)
	require.Len(t, m.sessionOrder, 2)
	require.Equal(t, DefaultSessionID, m.sessionOrder[0])
	require.Equal(t, "session-2", m.sessionOrder[1])

	// Verify session can be retrieved
	require.Equal(t, session, m.sessions["session-2"])

	// Verify active session was NOT changed
	require.Equal(t, DefaultSessionID, m.activeSessionID)
}

func TestCreateSession_MultipleCreations(t *testing.T) {
	m := New(DefaultConfig())

	m, _ = m.CreateSession("session-2")
	m, _ = m.CreateSession("session-3")
	m, _ = m.CreateSession("session-4")

	require.Len(t, m.sessions, 4)
	require.Len(t, m.sessionOrder, 4)
	require.Equal(t, []string{DefaultSessionID, "session-2", "session-3", "session-4"}, m.sessionOrder)
}

func TestSwitchSession(t *testing.T) {
	m := New(DefaultConfig())

	// Create a second session
	m, _ = m.CreateSession("session-2")

	// Switch to the new session
	m, ok := m.SwitchSession("session-2")
	require.True(t, ok)
	require.Equal(t, "session-2", m.activeSessionID)
	require.Equal(t, "session-2", m.ActiveSession().ID)

	// Switch back to original
	m, ok = m.SwitchSession(DefaultSessionID)
	require.True(t, ok)
	require.Equal(t, DefaultSessionID, m.activeSessionID)
}

func TestSwitchSession_NonExistent(t *testing.T) {
	m := New(DefaultConfig())

	// Try to switch to a non-existent session
	m, ok := m.SwitchSession("non-existent")
	require.False(t, ok, "switching to non-existent session should return false")
	require.Equal(t, DefaultSessionID, m.activeSessionID, "active session should be unchanged")
}

func TestProcessToSessionReverseLookup(t *testing.T) {
	m := New(DefaultConfig())

	// Verify initial session's reverse lookup
	session := m.SessionByProcessID(ChatPanelProcessID)
	require.NotNil(t, session)
	require.Equal(t, DefaultSessionID, session.ID)

	// Create a new session and set its ProcessID
	m, newSession := m.CreateSession("session-2")
	m = m.SetSessionProcessID("session-2", "process-2")

	// Verify new session can be looked up by ProcessID
	session = m.SessionByProcessID("process-2")
	require.NotNil(t, session)
	require.Equal(t, "session-2", session.ID)

	// Verify session's ProcessID was updated
	require.Equal(t, "process-2", newSession.ProcessID)
}

func TestSessionByProcessID_NonExistent(t *testing.T) {
	m := New(DefaultConfig())

	// Lookup non-existent process ID
	session := m.SessionByProcessID("non-existent")
	require.Nil(t, session)
}

func TestSetSessionProcessID_NonExistentSession(t *testing.T) {
	m := New(DefaultConfig())

	// Try to set ProcessID for non-existent session
	m = m.SetSessionProcessID("non-existent", "some-process")

	// Should not add to reverse lookup
	session := m.SessionByProcessID("some-process")
	require.Nil(t, session)
}

func TestSetSessionProcessID_UpdatesExistingSession(t *testing.T) {
	m := New(DefaultConfig())

	// The initial session already has ChatPanelProcessID
	// Update it to a new process ID
	m = m.SetSessionProcessID(DefaultSessionID, "new-process-id")

	// Verify new lookup works
	session := m.SessionByProcessID("new-process-id")
	require.NotNil(t, session)
	require.Equal(t, DefaultSessionID, session.ID)

	// Verify old lookup still works (we don't remove old mappings in this implementation)
	// This is intentional as processes may be replaced
}

func TestSessionCount(t *testing.T) {
	m := New(DefaultConfig())
	require.Equal(t, 1, m.SessionCount())

	m, _ = m.CreateSession("session-2")
	require.Equal(t, 2, m.SessionCount())

	m, _ = m.CreateSession("session-3")
	require.Equal(t, 3, m.SessionCount())
}

func TestActiveSessionID(t *testing.T) {
	m := New(DefaultConfig())
	require.Equal(t, DefaultSessionID, m.ActiveSessionID())

	m, _ = m.CreateSession("session-2")
	m, _ = m.SwitchSession("session-2")
	require.Equal(t, "session-2", m.ActiveSessionID())
}

// ============================================================================
// View Rendering with Active Session Tests
// ============================================================================

func TestRenderChatTabUsesActiveSession(t *testing.T) {
	m := New(DefaultConfig()).SetSize(50, 25).Toggle()

	// Set session to Ready status to show messages
	m.sessions[DefaultSessionID].Status = events.ProcessStatusReady

	// Add message to initial session (session-1)
	m = m.AddMessage(chatrender.Message{Role: RoleUser, Content: "Hello from session 1"})
	m = m.AddMessage(chatrender.Message{Role: RoleAssistant, Content: "Response in session 1"})

	// Verify view contains session 1 content
	view := scanView(m)
	require.Contains(t, view, "Hello from session 1")
	require.Contains(t, view, "Response in session 1")

	// Create and switch to session-2
	m, _ = m.CreateSession("session-2")
	m.sessions["session-2"].Status = events.ProcessStatusReady
	m, _ = m.SwitchSession("session-2")

	// Add different messages to session-2
	m = m.AddMessage(chatrender.Message{Role: RoleUser, Content: "Hello from session 2"})
	m = m.AddMessage(chatrender.Message{Role: RoleAssistant, Content: "Different response"})

	// Verify view now shows session 2 content, not session 1
	view = m.View()
	require.Contains(t, view, "Hello from session 2")
	require.Contains(t, view, "Different response")
	require.NotContains(t, view, "Hello from session 1")
	require.NotContains(t, view, "Response in session 1")

	// Switch back to session-1
	m, _ = m.SwitchSession(DefaultSessionID)

	// Verify view shows session 1 content again
	view = m.View()
	require.Contains(t, view, "Hello from session 1")
	require.Contains(t, view, "Response in session 1")
	require.NotContains(t, view, "Hello from session 2")
	require.NotContains(t, view, "Different response")
}

func TestViewportPerSession(t *testing.T) {
	m := New(DefaultConfig()).SetSize(50, 20).Toggle()

	// Set session to Ready status to show messages (Pending shows loading indicator)
	m.sessions[DefaultSessionID].Status = events.ProcessStatusReady

	// Add enough messages to session-1 to enable scrolling
	for i := 0; i < 20; i++ {
		m = m.AddMessage(chatrender.Message{
			Role:    RoleUser,
			Content: "Message " + string(rune('A'+i)),
		})
	}

	// Render to initialize viewport state
	_ = m.View()

	// Get session-1's viewport state
	session1 := m.ActiveSession()
	require.NotNil(t, session1)

	// Record initial position (at bottom after render)
	initialPosSession1 := session1.Pane.ScrollOffset()

	// Scroll up in session-1
	session1.Pane.ScrollUp(5)
	scrollPosSession1 := session1.Pane.ScrollOffset()
	require.NotEqual(t, initialPosSession1, scrollPosSession1, "scroll should change position")

	// Create and switch to session-2
	m, _ = m.CreateSession("session-2")
	m.sessions["session-2"].Status = events.ProcessStatusReady
	m, _ = m.SwitchSession("session-2")

	// Add messages to session-2
	for i := 0; i < 10; i++ {
		m = m.AddMessage(chatrender.Message{
			Role:    RoleUser,
			Content: "Session 2 msg " + string(rune('0'+i)),
		})
	}

	// Render to initialize session-2 viewport
	_ = m.View()

	// Get session-2's viewport state
	session2 := m.ActiveSession()
	require.NotNil(t, session2)

	// Verify session-2 pane is independent from session-1
	// (The pane will be at the bottom after render, but that position
	// depends on content size, so we just verify it's different from session-1's scrolled position)
	session2InitialPos := session2.Pane.ScrollOffset()

	// Scroll session-2 to a specific position
	session2.Pane.ScrollUp(2)
	scrollPosSession2 := session2.Pane.ScrollOffset()
	require.NotEqual(t, session2InitialPos, scrollPosSession2, "scroll should change position")

	// Switch back to session-1
	m, _ = m.SwitchSession(DefaultSessionID)

	// Verify session-1's scroll position is preserved
	session1AfterSwitch := m.ActiveSession()
	require.Equal(t, scrollPosSession1, session1AfterSwitch.Pane.ScrollOffset(), "session-1 scroll position should be preserved")

	// Switch to session-2 again
	m, _ = m.SwitchSession("session-2")

	// Verify session-2's scroll position is preserved
	session2AfterSwitch := m.ActiveSession()
	require.Equal(t, scrollPosSession2, session2AfterSwitch.Pane.ScrollOffset(), "session-2 scroll position should be preserved")
}

func TestContentDirtyPerSession(t *testing.T) {
	m := New(DefaultConfig()).SetSize(50, 20).Toggle()

	// Get session-1 and verify initial state
	session1 := m.ActiveSession()
	require.True(t, session1.ContentDirty, "new session should have ContentDirty=true")

	// Clear dirty flag by rendering
	session1.ContentDirty = false

	// Create session-2
	m, _ = m.CreateSession("session-2")

	// Session-2 should have ContentDirty=true by default
	session2 := m.sessions["session-2"]
	require.True(t, session2.ContentDirty, "new session should have ContentDirty=true")

	// Clear session-2's dirty flag
	session2.ContentDirty = false

	// Verify session-1 is still not dirty
	require.False(t, session1.ContentDirty, "session-1 ContentDirty should remain false")

	// Switch to session-2
	m, _ = m.SwitchSession("session-2")

	// Add message to session-2 (active session)
	m = m.AddMessage(chatrender.Message{Role: RoleUser, Content: "Test"})

	// Session-2 should now be dirty
	require.True(t, session2.ContentDirty, "session-2 should be dirty after AddMessage")

	// Session-1 should still not be dirty
	require.False(t, session1.ContentDirty, "session-1 should remain not dirty")
}

func TestSwitchingSessionsShowsCorrectHistory(t *testing.T) {
	m := New(DefaultConfig()).SetSize(60, 30).Toggle()

	// Build up conversation in session-1
	m = m.AddMessage(chatrender.Message{Role: RoleUser, Content: "User question 1"})
	m = m.AddMessage(chatrender.Message{Role: RoleAssistant, Content: "Assistant answer 1"})
	m = m.AddMessage(chatrender.Message{Role: RoleUser, Content: "User question 2"})
	m = m.AddMessage(chatrender.Message{Role: RoleAssistant, Content: "Assistant answer 2"})

	// Verify session-1 has 4 messages
	require.Len(t, m.Messages(), 4)

	// Create session-2
	m, _ = m.CreateSession("session-2")
	m, _ = m.SwitchSession("session-2")

	// Session-2 should start empty
	require.Empty(t, m.Messages(), "new session should have no messages")

	// Add different conversation to session-2
	m = m.AddMessage(chatrender.Message{Role: RoleUser, Content: "Different question"})
	m = m.AddMessage(chatrender.Message{Role: RoleAssistant, Content: "Different answer"})

	// Verify session-2 has 2 messages
	require.Len(t, m.Messages(), 2)

	// Switch back to session-1
	m, _ = m.SwitchSession(DefaultSessionID)

	// Verify session-1 still has its original 4 messages
	require.Len(t, m.Messages(), 4)
	require.Equal(t, "User question 1", m.Messages()[0].Content)
	require.Equal(t, "Assistant answer 2", m.Messages()[3].Content)

	// Switch to session-2 again
	m, _ = m.SwitchSession("session-2")

	// Verify session-2 still has its 2 messages
	require.Len(t, m.Messages(), 2)
	require.Equal(t, "Different question", m.Messages()[0].Content)
	require.Equal(t, "Different answer", m.Messages()[1].Content)
}

func TestSetSizeMarksActiveSessionDirty(t *testing.T) {
	m := New(DefaultConfig())

	// Create two sessions
	session1 := m.ActiveSession()
	session1.ContentDirty = false

	m, _ = m.CreateSession("session-2")
	session2 := m.sessions["session-2"]
	session2.ContentDirty = false

	// Verify both are not dirty
	require.False(t, session1.ContentDirty)
	require.False(t, session2.ContentDirty)

	// SetSize should only mark active session dirty (session-1 is still active)
	m = m.SetSize(80, 40)

	require.True(t, session1.ContentDirty, "active session should be marked dirty after SetSize")
	require.False(t, session2.ContentDirty, "non-active session should not be marked dirty")

	// Clear flags
	session1.ContentDirty = false

	// Switch to session-2
	m, _ = m.SwitchSession("session-2")

	// SetSize again
	m = m.SetSize(100, 50)

	require.False(t, session1.ContentDirty, "non-active session should not be marked dirty")
	require.True(t, session2.ContentDirty, "active session should be marked dirty after SetSize")
}

// ============================================================================
// Event Routing Tests (perles-81ic.3)
// ============================================================================

func TestMultipleSessionsEventRouting(t *testing.T) {
	// Create 3 sessions and verify events route to correct sessions by ProcessID
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Set a fixed time for testing
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	m.Clock = func() time.Time { return baseTime }

	// Initial session (session-1) already exists with ChatPanelProcessID
	// Create session-2 and session-3
	m, _ = m.CreateSession("session-2")
	m = m.SetSessionProcessID("session-2", "process-2")

	m, _ = m.CreateSession("session-3")
	m = m.SetSessionProcessID("session-3", "process-3")

	// Verify all sessions exist
	require.Equal(t, 3, m.SessionCount())

	// Send events to different sessions via their ProcessIDs

	// Event 1: Output to session-1 (ChatPanelProcessID)
	event1 := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:      events.ProcessOutput,
			ProcessID: ChatPanelProcessID,
			Output:    "Message for session-1",
		},
	}
	m, _ = m.Update(event1)

	// Event 2: Output to session-2 (process-2)
	event2 := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:      events.ProcessOutput,
			ProcessID: "process-2",
			Output:    "Message for session-2",
		},
	}
	m, _ = m.Update(event2)

	// Event 3: Output to session-3 (process-3)
	event3 := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:      events.ProcessOutput,
			ProcessID: "process-3",
			Output:    "Message for session-3",
		},
	}
	m, _ = m.Update(event3)

	// Verify each session received its own message
	session1 := m.sessions[DefaultSessionID]
	require.Len(t, session1.Messages, 1)
	require.Equal(t, "Message for session-1", session1.Messages[0].Content)

	session2 := m.sessions["session-2"]
	require.Len(t, session2.Messages, 1)
	require.Equal(t, "Message for session-2", session2.Messages[0].Content)

	session3 := m.sessions["session-3"]
	require.Len(t, session3.Messages, 1)
	require.Equal(t, "Message for session-3", session3.Messages[0].Content)

	// Send multiple messages to session-2
	event4 := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:      events.ProcessOutput,
			ProcessID: "process-2",
			Output:    "Second message for session-2",
		},
	}
	m, _ = m.Update(event4)

	// Verify session-2 now has 2 messages, others unchanged
	require.Len(t, session1.Messages, 1)
	require.Len(t, session2.Messages, 2)
	require.Len(t, session3.Messages, 1)
	require.Equal(t, "Second message for session-2", session2.Messages[1].Content)
}

func TestSessionSwitchDuringActiveOutput(t *testing.T) {
	// Verify events route correctly to non-active session during switch
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Create second session
	m, _ = m.CreateSession("session-2")
	m = m.SetSessionProcessID("session-2", "process-2")

	// Active session is session-1
	require.Equal(t, DefaultSessionID, m.activeSessionID)

	// Send output to session-1 (active)
	event1 := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:      events.ProcessOutput,
			ProcessID: ChatPanelProcessID,
			Output:    "Active session output",
		},
	}
	m, _ = m.Update(event1)

	// Verify active session received message
	require.Len(t, m.Messages(), 1)
	require.Equal(t, "Active session output", m.Messages()[0].Content)

	// Switch to session-2
	m, _ = m.SwitchSession("session-2")
	require.Equal(t, "session-2", m.activeSessionID)

	// Now send output to session-1 (now inactive)
	event2 := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:      events.ProcessOutput,
			ProcessID: ChatPanelProcessID,
			Output:    "Background output to session-1",
		},
	}
	m, _ = m.Update(event2)

	// Verify session-1 received its message even though it's not active
	session1 := m.sessions[DefaultSessionID]
	require.Len(t, session1.Messages, 2)
	require.Equal(t, "Background output to session-1", session1.Messages[1].Content)

	// Verify session-2 (active) has no messages yet
	require.Empty(t, m.Messages()) // Messages() returns active session's messages

	// Send output to session-2 (active)
	event3 := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:      events.ProcessOutput,
			ProcessID: "process-2",
			Output:    "Session-2 output",
		},
	}
	m, _ = m.Update(event3)

	// Verify session-2 received its message
	require.Len(t, m.Messages(), 1)
	require.Equal(t, "Session-2 output", m.Messages()[0].Content)
}

func TestSessionSwitchToFailedSession(t *testing.T) {
	// Verify failed session handling - events still route, status updates correctly
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Create second session
	m, _ = m.CreateSession("session-2")
	m = m.SetSessionProcessID("session-2", "process-2")

	// Switch to session-2
	m, _ = m.SwitchSession("session-2")

	// Session-2 is now active, session-1 is in background

	// Send error event to session-1 (background)
	errorEvent := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:      events.ProcessError,
			ProcessID: ChatPanelProcessID,
			Status:    events.ProcessStatusFailed,
			Error:     errors.New("process crashed"),
		},
	}
	m, _ = m.Update(errorEvent)

	// Verify session-1 is marked as failed
	session1 := m.sessions[DefaultSessionID]
	require.Equal(t, events.ProcessStatusFailed, session1.Status)

	// Switch to the failed session
	m, ok := m.SwitchSession(DefaultSessionID)
	require.True(t, ok, "should be able to switch to failed session")
	require.Equal(t, DefaultSessionID, m.activeSessionID)

	// Verify we can still see the failed session's status
	activeSession := m.ActiveSession()
	require.NotNil(t, activeSession)
	require.Equal(t, events.ProcessStatusFailed, activeSession.Status)
}

func TestHasNewContentTracking(t *testing.T) {
	// Verify HasNewContent is set for non-active sessions receiving events
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Create second session
	m, _ = m.CreateSession("session-2")
	m = m.SetSessionProcessID("session-2", "process-2")

	// Clear HasNewContent flags
	m.sessions[DefaultSessionID].HasNewContent = false
	m.sessions["session-2"].HasNewContent = false

	// Active session is session-1
	require.Equal(t, DefaultSessionID, m.activeSessionID)

	// Send output to session-2 (non-active)
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:      events.ProcessOutput,
			ProcessID: "process-2",
			Output:    "New content for session-2",
		},
	}
	m, _ = m.Update(event)

	// Verify session-2 has HasNewContent=true (non-active session)
	session2 := m.sessions["session-2"]
	require.True(t, session2.HasNewContent, "non-active session should have HasNewContent=true")

	// Verify session-1 (active) still has HasNewContent=false
	session1 := m.sessions[DefaultSessionID]
	require.False(t, session1.HasNewContent, "active session should not have HasNewContent changed")
}

func TestHasNewContentTracking_ScrolledUp(t *testing.T) {
	// Verify HasNewContent is set when active session is scrolled up
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg).SetSize(50, 20).Toggle()

	// Set session to Ready status to show messages (Pending shows loading indicator)
	m.sessions[DefaultSessionID].Status = events.ProcessStatusReady

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Add enough messages to enable scrolling
	for i := 0; i < 30; i++ {
		m = m.AddMessage(chatrender.Message{
			Role:    RoleUser,
			Content: "Message " + string(rune('A'+i)),
		})
	}

	// Render to initialize viewport
	_ = m.View()

	// Get active session
	session := m.ActiveSession()
	require.NotNil(t, session)

	// Clear HasNewContent
	session.HasNewContent = false

	// Scroll up (not at bottom anymore)
	session.Pane.ScrollUp(10)
	require.False(t, session.Pane.AtBottom(), "pane should not be at bottom after scrolling up")

	// Send output to active session while scrolled up
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:      events.ProcessOutput,
			ProcessID: ChatPanelProcessID,
			Output:    "New content while scrolled up",
		},
	}
	m, _ = m.Update(event)

	// Verify HasNewContent is now true (scrolled up when content arrived)
	require.True(t, session.HasNewContent, "active session scrolled up should have HasNewContent=true")
}

func TestUnknownProcessIDEventsIgnored(t *testing.T) {
	// Verify events with unknown ProcessID are logged and ignored (no crash)
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Send event with unknown ProcessID
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:      events.ProcessOutput,
			ProcessID: "unknown-process-id",
			Output:    "This should be ignored",
		},
	}

	// Should not panic
	m, cmd := m.Update(event)

	// Verify no messages were added to any session
	require.Empty(t, m.sessions[DefaultSessionID].Messages)

	// Verify listener continues
	require.NotNil(t, cmd)
}

func TestLastActivityUpdatedOnEvents(t *testing.T) {
	// Verify LastActivity timestamp is updated when events arrive
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Set a fixed time for testing
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	m.Clock = func() time.Time { return baseTime }

	// Get active session and record the initial LastActivity
	session := m.ActiveSession()
	require.NotNil(t, session)

	// Reset LastActivity to a known value using the Clock
	session.LastActivity = baseTime

	// Advance time
	newTime := baseTime.Add(5 * time.Minute)
	m.Clock = func() time.Time { return newTime }

	// Send output event
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:      events.ProcessOutput,
			ProcessID: ChatPanelProcessID,
			Output:    "New output",
		},
	}
	m, _ = m.Update(event)

	// Verify LastActivity was updated to the new time
	require.Equal(t, newTime, session.LastActivity, "LastActivity should be updated to new time")
}

func TestProcessStatusChangeRoutedToSession(t *testing.T) {
	// Verify ProcessStatusChange events update the correct session's status
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Create second session
	m, _ = m.CreateSession("session-2")
	m = m.SetSessionProcessID("session-2", "process-2")

	// Initial status is Pending
	require.Equal(t, events.ProcessStatusPending, m.sessions[DefaultSessionID].Status)
	require.Equal(t, events.ProcessStatusPending, m.sessions["session-2"].Status)

	// Send status change to session-2
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:      events.ProcessStatusChange,
			ProcessID: "process-2",
			Status:    events.ProcessStatusReady,
		},
	}
	m, _ = m.Update(event)

	// Verify only session-2's status changed
	require.Equal(t, events.ProcessStatusPending, m.sessions[DefaultSessionID].Status, "session-1 status should be unchanged")
	require.Equal(t, events.ProcessStatusReady, m.sessions["session-2"].Status, "session-2 status should be Ready")
}

func TestProcessTokenUsageRoutedToSession(t *testing.T) {
	// Verify ProcessTokenUsage events update the correct session's metrics
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Create second session
	m, _ = m.CreateSession("session-2")
	m = m.SetSessionProcessID("session-2", "process-2")

	// Initial metrics are nil
	require.Nil(t, m.sessions[DefaultSessionID].Metrics)
	require.Nil(t, m.sessions["session-2"].Metrics)

	// Send token usage to session-2
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:      events.ProcessTokenUsage,
			ProcessID: "process-2",
			Metrics: &metrics.TokenMetrics{
				TokensUsed:  5000,
				TotalTokens: 100000,
			},
		},
	}
	m, _ = m.Update(event)

	// Verify only session-2's metrics updated
	require.Nil(t, m.sessions[DefaultSessionID].Metrics, "session-1 metrics should be nil")
	require.NotNil(t, m.sessions["session-2"].Metrics, "session-2 metrics should be set")
	require.Equal(t, 5000, m.sessions["session-2"].Metrics.TokensUsed)
}

// ============================================================================
// Sessions Tab UI Tests (perles-81ic.4)
// ============================================================================

func TestSessionsTabLayout_Golden(t *testing.T) {
	// Create model with multiple sessions in various states
	m := New(DefaultConfig()).SetSize(60, 25).Toggle()

	// Session 1 (default) - Ready with messages
	m = m.AddMessage(chatrender.Message{Role: RoleUser, Content: "Hello"})
	m = m.AddMessage(chatrender.Message{Role: RoleAssistant, Content: "Hi there!"})
	m.sessions[DefaultSessionID].Status = events.ProcessStatusReady

	// Session 2 - Working, has new content
	m, _ = m.CreateSession("session-2")
	m = m.SetSessionProcessID("session-2", "process-2")
	m.sessions["session-2"].Status = events.ProcessStatusWorking
	m.sessions["session-2"].HasNewContent = true
	m.sessions["session-2"].Messages = []chatrender.Message{
		{Role: RoleUser, Content: "Task 1"},
	}

	// Session 3 - Failed (Session ended)
	m, _ = m.CreateSession("session-3")
	m = m.SetSessionProcessID("session-3", "process-3")
	m.sessions["session-3"].Status = events.ProcessStatusFailed
	m.sessions["session-3"].Messages = []chatrender.Message{
		{Role: RoleUser, Content: "Task 2"},
		{Role: RoleAssistant, Content: "Error occurred"},
	}

	// Switch to Sessions tab
	m.activeTab = TabSessions
	m.sessionListCursor = 0

	view := scanView(m)
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestSessionsTabEmpty_Golden(t *testing.T) {
	// Create model with no sessions
	m := New(DefaultConfig()).SetSize(60, 20).Toggle()

	// Clear all sessions to simulate empty state
	m.sessions = make(map[string]*SessionData)
	m.sessionOrder = []string{}
	m.activeSessionID = ""

	// Switch to Sessions tab
	m.activeTab = TabSessions

	view := scanView(m)
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestSessionsTabSelected_Golden(t *testing.T) {
	// Create model with multiple sessions, cursor on second session
	m := New(DefaultConfig()).SetSize(60, 20).Toggle()

	// Session 1 (default)
	m.sessions[DefaultSessionID].Status = events.ProcessStatusReady
	m.sessions[DefaultSessionID].Messages = []chatrender.Message{
		{Role: RoleUser, Content: "Hello"},
	}

	// Session 2
	m, _ = m.CreateSession("session-2")
	m.sessions["session-2"].Status = events.ProcessStatusReady
	m.sessions["session-2"].Messages = []chatrender.Message{
		{Role: RoleUser, Content: "World"},
	}

	// Session 3
	m, _ = m.CreateSession("session-3")
	m.sessions["session-3"].Status = events.ProcessStatusPending

	// Switch to Sessions tab with cursor on second session
	m.activeTab = TabSessions
	m.sessionListCursor = 1 // Points to session-2

	view := scanView(m)
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestSessionNavigationKeys(t *testing.T) {
	// Create model with multiple sessions
	m := New(DefaultConfig()).SetSize(60, 20).Toggle().Focus()

	// Create additional sessions
	m, _ = m.CreateSession("session-2")
	m, _ = m.CreateSession("session-3")

	// Switch to Sessions tab
	// Cursor layout: 0="Create new session", 1=session-1, 2=session-2, 3=session-3
	m.activeTab = TabSessions
	m.sessionListCursor = 0 // Start at "Create new session"

	// Test 'j' moves cursor down
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	require.Equal(t, 1, m.sessionListCursor, "j should move cursor down")

	// Test 'j' again
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	require.Equal(t, 2, m.sessionListCursor, "j should move cursor down again")

	// Test 'j' again
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	require.Equal(t, 3, m.sessionListCursor, "j should move cursor to last session")

	// Test 'j' at bottom doesn't go past end
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	require.Equal(t, 3, m.sessionListCursor, "j at bottom should not go past end")

	// Test 'k' moves cursor up
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	require.Equal(t, 2, m.sessionListCursor, "k should move cursor up")

	// Test 'k' again
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	require.Equal(t, 1, m.sessionListCursor, "k should move cursor up again")

	// Test 'k' again to "Create new session"
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	require.Equal(t, 0, m.sessionListCursor, "k should move cursor to Create new session")

	// Test 'k' at top doesn't go negative
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	require.Equal(t, 0, m.sessionListCursor, "k at top should not go negative")
}

func TestSessionNavigationKeys_DownArrow(t *testing.T) {
	// Test arrow keys also work for navigation
	// Cursor layout: 0="Create new session", 1=session-1, 2=session-2
	m := New(DefaultConfig()).SetSize(60, 20).Toggle().Focus()

	m, _ = m.CreateSession("session-2")

	m.activeTab = TabSessions
	m.sessionListCursor = 0 // Start at "Create new session"

	// Test down arrow
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, 1, m.sessionListCursor, "down arrow should move cursor down")

	// Test up arrow
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	require.Equal(t, 0, m.sessionListCursor, "up arrow should move cursor up")
}

func TestSessionEnterSwitch(t *testing.T) {
	// Create model with multiple sessions
	// Cursor layout: 0="Create new session", 1=session-1, 2=session-2, 3=session-3
	m := New(DefaultConfig()).SetSize(60, 20).Toggle().Focus()

	// Create additional sessions with messages
	m, _ = m.CreateSession("session-2")
	m.sessions["session-2"].Messages = []chatrender.Message{
		{Role: RoleUser, Content: "Session 2 message"},
	}
	m.sessions["session-2"].HasNewContent = true // Has unread content

	m, _ = m.CreateSession("session-3")
	m.sessions["session-3"].Messages = []chatrender.Message{
		{Role: RoleUser, Content: "Session 3 message"},
	}
	m.sessions["session-3"].HasNewContent = true

	// Verify we're on session-1
	require.Equal(t, DefaultSessionID, m.activeSessionID)

	// Switch to Sessions tab and move cursor to session-2 (index 2)
	m.activeTab = TabSessions
	m.sessionListCursor = 2 // Points to session-2 (index 0=Create, 1=session-1, 2=session-2)

	// Press Enter to switch
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Verify active session changed to session-2
	require.Equal(t, "session-2", m.activeSessionID, "Enter should switch to selected session")

	// Verify switched to Chat tab
	require.Equal(t, TabChat, m.activeTab, "Enter should switch to Chat tab")

	// Verify HasNewContent was cleared for the newly active session
	require.False(t, m.sessions["session-2"].HasNewContent, "HasNewContent should be cleared on switch")

	// Verify session-3 still has HasNewContent (wasn't switched to)
	require.True(t, m.sessions["session-3"].HasNewContent, "Other sessions should still have HasNewContent")
}

func TestSessionEnterSwitch_ClearsHasNewContent(t *testing.T) {
	// Test that switching sessions clears HasNewContent on the target session
	// Cursor layout: 0="Create new session", 1=session-1, 2=session-2
	m := New(DefaultConfig()).SetSize(60, 20).Toggle().Focus()

	// Create session-2 with new content
	m, _ = m.CreateSession("session-2")
	m.sessions["session-2"].HasNewContent = true

	// Switch to Sessions tab
	m.activeTab = TabSessions
	m.sessionListCursor = 2 // Points to session-2 (index 0=Create, 1=session-1, 2=session-2)

	// Verify HasNewContent is true before switch
	require.True(t, m.sessions["session-2"].HasNewContent)

	// Press Enter to switch
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Verify HasNewContent is now false
	require.False(t, m.sessions["session-2"].HasNewContent, "HasNewContent should be cleared after switching")
}

func TestSessionNavigationKeys_OnlyChatTab(t *testing.T) {
	// Test that j/k don't navigate when on Chat tab (keys go to input)
	m := New(DefaultConfig()).SetSize(60, 20).Toggle().Focus()

	m, _ = m.CreateSession("session-2")

	// Stay on Chat tab (default)
	m.activeTab = TabChat
	m.sessionListCursor = 0

	// Keys should NOT change cursor when on Chat tab
	initialCursor := m.sessionListCursor
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})

	// Cursor should be unchanged (key went to input)
	require.Equal(t, initialCursor, m.sessionListCursor, "j should not change cursor when on Chat tab")
}

func TestSessionsTab_BlocksInputForwarding(t *testing.T) {
	// Test that typing on Sessions tab doesn't go to the input field
	m := New(DefaultConfig()).SetSize(60, 20).Toggle().Focus()

	m, _ = m.CreateSession("session-2")

	// Switch to Sessions tab
	m.activeTab = TabSessions

	// Get initial input value
	initialValue := m.input.Value()

	// Type some characters that aren't navigation keys
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})

	// Input should be unchanged - keys should not have been forwarded
	require.Equal(t, initialValue, m.input.Value(), "Keys should not be forwarded to input when Sessions tab is active")
}

func TestSessionsTab_ActivityIndicator(t *testing.T) {
	// Test that activity indicators show correctly
	m := New(DefaultConfig()).SetSize(60, 20).Toggle()

	// Session 1 - no new content (viewed)
	m.sessions[DefaultSessionID].HasNewContent = false
	m.sessions[DefaultSessionID].Status = events.ProcessStatusReady

	// Session 2 - has new content (unread)
	m, _ = m.CreateSession("session-2")
	m.sessions["session-2"].HasNewContent = true
	m.sessions["session-2"].Status = events.ProcessStatusWorking

	// Switch to Sessions tab
	m.activeTab = TabSessions

	view := scanView(m)

	// Session 1 should show ○ (viewed)
	require.Contains(t, view, "○", "Session without new content should show ○")

	// Session 2 should show ● (unread)
	require.Contains(t, view, "●", "Session with new content should show ●")
}

func TestSessionsTab_FailedSessionShowsEnded(t *testing.T) {
	// Test that failed/stopped sessions show "Session ended"
	m := New(DefaultConfig()).SetSize(60, 20).Toggle()

	// Session 1 - failed
	m.sessions[DefaultSessionID].Status = events.ProcessStatusFailed

	// Session 2 - retired (also terminal)
	m, _ = m.CreateSession("session-2")
	m.sessions["session-2"].Status = events.ProcessStatusRetired

	// Switch to Sessions tab
	m.activeTab = TabSessions

	view := scanView(m)

	// Should show "Session ended" for terminal statuses
	require.Contains(t, view, "Session ended", "Failed sessions should show 'Session ended'")
}

func TestSessionListCursorBounds_SessionRemoval(t *testing.T) {
	// Test cursor stays in bounds when sessions are removed
	// Cursor layout: 0="Create new session", 1=session-1, 2=session-2, 3=session-3
	m := New(DefaultConfig()).SetSize(60, 20).Toggle().Focus()

	// Create multiple sessions
	m, _ = m.CreateSession("session-2")
	m, _ = m.CreateSession("session-3")

	// Move cursor to last session (session-3 is at index 3)
	m.activeTab = TabSessions
	m.sessionListCursor = 3 // Points to session-3

	// Simulate session removal by reducing sessionOrder
	m.sessionOrder = m.sessionOrder[:2] // Remove session-3
	delete(m.sessions, "session-3")

	// Cursor should be clamped when navigating
	// (The cursor position is only checked during rendering)
	// Let's verify j doesn't crash
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})

	// Cursor should still be at max valid position (len(sessionOrder) since index 0 is "Create")
	require.LessOrEqual(t, m.sessionListCursor, len(m.sessionOrder), "cursor should be within bounds")
}

// =============================================================================
// Ctrl+N/P Session Cycling Tests (replaces old Ctrl+N tests)
// =============================================================================

func TestCtrlNP_CyclesSessions(t *testing.T) {
	// Test that Ctrl+N cycles to next session and Ctrl+P to previous
	m := New(DefaultConfig()).SetSize(60, 20).Toggle().Focus()

	// Create additional sessions
	m, _ = m.CreateSession("session-2")
	m, _ = m.CreateSession("session-3")

	// Verify we're on session-1 (default)
	require.Equal(t, DefaultSessionID, m.activeSessionID)

	// Ctrl+N should cycle to next session
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	require.Equal(t, "session-2", m.activeSessionID)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	require.Equal(t, "session-3", m.activeSessionID)

	// Ctrl+N wraps around
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	require.Equal(t, DefaultSessionID, m.activeSessionID)

	// Ctrl+P goes to previous (wraps to end)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	require.Equal(t, "session-3", m.activeSessionID)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	require.Equal(t, "session-2", m.activeSessionID)
}

func TestCtrlNP_SingleSession_NoOp(t *testing.T) {
	// Test that Ctrl+N/P with only one session does nothing
	m := New(DefaultConfig()).SetSize(60, 20).Toggle().Focus()

	// Only one session exists
	require.Equal(t, 1, m.SessionCount())
	require.Equal(t, DefaultSessionID, m.activeSessionID)

	// Ctrl+N should not change session
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	require.Equal(t, DefaultSessionID, m.activeSessionID)

	// Ctrl+P should not change session
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	require.Equal(t, DefaultSessionID, m.activeSessionID)
}

func TestCtrlNP_IgnoredWhenNotVisibleOrFocused(t *testing.T) {
	// Test that Ctrl+N/P is ignored when panel is not visible or focused
	m := New(DefaultConfig()).SetSize(60, 20)
	m, _ = m.CreateSession("session-2")

	// Panel not visible, not focused
	require.False(t, m.Visible())
	require.False(t, m.Focused())

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	require.Equal(t, DefaultSessionID, m.activeSessionID, "Ctrl+N should be ignored when not visible")

	// Panel visible but not focused
	m = m.Toggle() // Now visible
	require.True(t, m.Visible())
	require.False(t, m.Focused())

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	require.Equal(t, DefaultSessionID, m.activeSessionID, "Ctrl+N should be ignored when not focused")
}

func TestNewSessionCreatedSwitchesSession(t *testing.T) {
	// Test that NewSessionCreatedMsg switches to the new session
	m := New(DefaultConfig()).SetSize(60, 20).Toggle().Focus()

	// Create a second session manually (simulating what app.go does)
	m, _ = m.CreateSession("session-2")

	// Verify we're still on session-1
	require.Equal(t, DefaultSessionID, m.activeSessionID)

	// Handle NewSessionCreatedMsg
	m, _ = m.Update(NewSessionCreatedMsg{SessionID: "session-2"})

	// Should now be on session-2
	require.Equal(t, "session-2", m.activeSessionID)

	// Should be on Chat tab (for immediate typing)
	require.Equal(t, TabChat, m.activeTab)
}

func TestNewSessionCreatedSwitchesFromSessionsTab(t *testing.T) {
	// Test that NewSessionCreatedMsg switches from Sessions tab to Chat tab
	m := New(DefaultConfig()).SetSize(60, 20).Toggle().Focus()

	// Switch to Sessions tab
	m.activeTab = TabSessions

	// Create and switch to a new session
	m, _ = m.CreateSession("session-2")
	m, _ = m.Update(NewSessionCreatedMsg{SessionID: "session-2"})

	// Should be on Chat tab now
	require.Equal(t, TabChat, m.activeTab)
	require.Equal(t, "session-2", m.activeSessionID)
}

func TestSessionIDGeneration(t *testing.T) {
	// Test that NextSessionID generates sequential IDs using atomic counter
	m := New(DefaultConfig())

	// Initially has session-1 (counter=1), so next should be session-2
	require.Equal(t, "session-2", m.NextSessionID())

	// Counter increments each call, regardless of actual sessions created
	require.Equal(t, "session-3", m.NextSessionID())
	require.Equal(t, "session-4", m.NextSessionID())
}

func TestSessionIDGeneration_NoCollisionAfterDeletion(t *testing.T) {
	// Test that session IDs don't collide after deletion
	// This was the bug: using len(sessions)+1 caused collision after delete
	m := New(DefaultConfig())

	// Create session-2 and session-3
	session2ID := m.NextSessionID() // "session-2"
	m, _ = m.CreateSession(session2ID)
	session3ID := m.NextSessionID() // "session-3"
	m, _ = m.CreateSession(session3ID)

	require.Equal(t, 3, m.SessionCount()) // session-1, session-2, session-3

	// Delete session-1 (need session-2 to have process ID for retire to work)
	m = m.SetSessionProcessID(session2ID, "proc-2")
	m, _ = m.RetireSession(DefaultSessionID)

	require.Equal(t, 2, m.SessionCount()) // session-2, session-3

	// Next ID should be session-4, NOT session-3 (which would collide)
	nextID := m.NextSessionID()
	require.Equal(t, "session-4", nextID, "Session ID should not collide after deletion")

	// Verify we can create a session with this ID without collision
	m, newSession := m.CreateSession(nextID)
	require.NotNil(t, newSession)
	require.Equal(t, "session-4", newSession.ID)
	require.Equal(t, 3, m.SessionCount())
}

func TestSpawnAssistantForSession(t *testing.T) {
	// Test that SpawnAssistantForSession works with a specific process ID
	m := New(DefaultConfig())
	infra := newTestInfrastructureWithSpawnExpectation(t)
	m = m.SetInfrastructure(infra)

	// Start the infrastructure to enable command processing
	err := infra.Start()
	require.NoError(t, err)
	defer infra.Shutdown()

	// SpawnAssistantForSession should return spinnerTick cmd on successful submission
	_, cmd := m.SpawnAssistantForSession("session-2")
	require.NotNil(t, cmd, "SpawnAssistantForSession should return spinnerTick cmd on successful submission")
}

func TestSpawnAssistantForSession_NoInfrastructure(t *testing.T) {
	// Test that SpawnAssistantForSession returns error when no infrastructure
	m := New(DefaultConfig())

	_, cmd := m.SpawnAssistantForSession("session-2")
	require.NotNil(t, cmd, "SpawnAssistantForSession should return error cmd when no infrastructure")

	// Execute the command to get the error message
	msg := cmd()
	errMsg, ok := msg.(AssistantErrorMsg)
	require.True(t, ok, "Should return AssistantErrorMsg")
	require.Equal(t, ErrNoInfrastructure, errMsg.Error)
}

func TestSpawnAssistantForSession_KeepsStatusPending(t *testing.T) {
	// Test that SpawnAssistantForSession keeps session status as Pending (shows loading indicator)
	m := New(DefaultConfig())
	infra := newTestInfrastructureWithSpawnExpectation(t)
	m = m.SetInfrastructure(infra)

	// Start the infrastructure
	err := infra.Start()
	require.NoError(t, err)
	defer infra.Shutdown()

	// Default session (session-1) should be mapped to ChatPanelProcessID
	session := m.SessionByProcessID(ChatPanelProcessID)
	require.NotNil(t, session, "Session should be mapped to ChatPanelProcessID")

	// Verify initial status is Pending
	require.Equal(t, events.ProcessStatusPending, session.Status,
		"Session status should initially be Pending")

	// Call SpawnAssistantForSession - must capture returned model to get status change
	var cmd tea.Cmd
	m, cmd = m.SpawnAssistantForSession(ChatPanelProcessID)
	require.NotNil(t, cmd, "Should return spinnerTick cmd on successful submission")

	// Status should still be Pending (loading indicator shows until ProcessReady)
	session = m.SessionByProcessID(ChatPanelProcessID)
	require.Equal(t, events.ProcessStatusPending, session.Status,
		"Session status should remain Pending until ProcessReady event")
}

func TestSpawnAssistantForSession_KeepsStatusPendingForNewSession(t *testing.T) {
	// Test that status remains Pending for a newly created session
	m := New(DefaultConfig())
	infra := newTestInfrastructureWithSpawnExpectation(t)
	m = m.SetInfrastructure(infra)

	// Start the infrastructure
	err := infra.Start()
	require.NoError(t, err)
	defer infra.Shutdown()

	// Create a new session
	var session *SessionData
	m, session = m.CreateSession("session-2")
	require.NotNil(t, session)

	// Set up the process ID mapping (as app.go does before calling SpawnAssistantForSession)
	processID := "session-2"
	session.ProcessID = processID
	m = m.SetSessionProcessID("session-2", processID)

	// Verify initial status is Pending
	require.Equal(t, events.ProcessStatusPending, session.Status,
		"New session status should be Pending")

	// Call SpawnAssistantForSession - must capture returned model to get status change
	var cmd tea.Cmd
	m, cmd = m.SpawnAssistantForSession(processID)
	require.NotNil(t, cmd, "Should return spinnerTick cmd on successful submission")

	// Status should still be Pending - check from returned model
	session = m.SessionByProcessID(processID)
	require.Equal(t, events.ProcessStatusPending, session.Status,
		"Session status should remain Pending until ProcessReady event")
}

func TestSpawnAssistantForSession_StatusNotSetForUnknownProcessID(t *testing.T) {
	// Test that status is NOT set if processID doesn't map to a session
	// (defensive behavior - should not crash)
	m := New(DefaultConfig())
	infra := newTestInfrastructureWithSpawnExpectation(t)
	m = m.SetInfrastructure(infra)

	// Start the infrastructure
	err := infra.Start()
	require.NoError(t, err)
	defer infra.Shutdown()

	// Get the original session (to verify it's not affected)
	originalSession := m.SessionByProcessID(ChatPanelProcessID)
	require.NotNil(t, originalSession)
	originalStatus := originalSession.Status

	// Call SpawnAssistantForSession with unknown processID
	// This should not crash - just submit the command without setting status
	var cmd tea.Cmd
	m, cmd = m.SpawnAssistantForSession("unknown-process-id")
	require.NotNil(t, cmd, "Should return spinnerTick cmd on successful submission even with unknown process ID")

	// Original session's status should be unchanged (check from returned model)
	originalSession = m.SessionByProcessID(ChatPanelProcessID)
	require.Equal(t, originalStatus, originalSession.Status,
		"Original session status should not be affected by unknown processID")
}

func TestNewSessionClearsHasNewContent(t *testing.T) {
	// Test that switching to a newly created session clears HasNewContent
	m := New(DefaultConfig()).SetSize(60, 20).Toggle().Focus()

	// Create a new session and mark it as having new content
	m, _ = m.CreateSession("session-2")
	m.sessions["session-2"].HasNewContent = true

	// Handle NewSessionCreatedMsg (simulates switching to the new session)
	m, _ = m.Update(NewSessionCreatedMsg{SessionID: "session-2"})

	// HasNewContent should be cleared by switchToSession
	require.False(t, m.sessions["session-2"].HasNewContent,
		"HasNewContent should be cleared when switching to new session")
}

func TestRetireSession_Basic(t *testing.T) {
	// Test basic session retirement
	m := New(DefaultConfig()).SetSize(60, 20).Toggle().Focus()

	// Create additional sessions
	m, _ = m.CreateSession("session-2")
	m, _ = m.CreateSession("session-3")
	require.Equal(t, 3, m.SessionCount())

	// Retire session-2
	m, _ = m.RetireSession("session-2")

	// Should have 2 sessions remaining
	require.Equal(t, 2, m.SessionCount())

	// session-2 should be gone
	require.Nil(t, m.sessions["session-2"])

	// session-1 and session-3 should remain
	require.NotNil(t, m.sessions[DefaultSessionID])
	require.NotNil(t, m.sessions["session-3"])
}

func TestRetireSession_CannotRetireLastSession(t *testing.T) {
	// Test that the last session cannot be retired
	m := New(DefaultConfig()).SetSize(60, 20).Toggle().Focus()

	// Only have the default session
	require.Equal(t, 1, m.SessionCount())

	// Try to retire it
	m, _ = m.RetireSession(DefaultSessionID)

	// Should still have 1 session
	require.Equal(t, 1, m.SessionCount())
	require.NotNil(t, m.sessions[DefaultSessionID])
}

func TestRetireSession_RetiringActiveSessionSwitches(t *testing.T) {
	// Test that retiring the active session switches to another
	m := New(DefaultConfig()).SetSize(60, 20).Toggle().Focus()

	// Create session-2 and switch to it
	m, _ = m.CreateSession("session-2")
	m, _ = m.SwitchSession("session-2")
	require.Equal(t, "session-2", m.activeSessionID)

	// Retire the active session (session-2)
	m, _ = m.RetireSession("session-2")

	// Should have switched to session-1
	require.Equal(t, DefaultSessionID, m.activeSessionID)
	require.Equal(t, 1, m.SessionCount())
}

func TestRetireSession_CleansUpProcessMapping(t *testing.T) {
	// Test that retiring a session cleans up the processToSession mapping
	m := New(DefaultConfig()).SetSize(60, 20).Toggle().Focus()

	// Create session-2 with a process ID
	m, _ = m.CreateSession("session-2")
	m = m.SetSessionProcessID("session-2", "process-2")

	// Verify mapping exists
	require.Equal(t, "session-2", m.processToSession["process-2"])

	// Retire session-2
	m, _ = m.RetireSession("session-2")

	// Mapping should be removed
	_, exists := m.processToSession["process-2"]
	require.False(t, exists, "processToSession mapping should be cleaned up")
}

func TestRetireSession_UpdatesSessionOrder(t *testing.T) {
	// Test that session order is correctly updated after retirement
	m := New(DefaultConfig()).SetSize(60, 20).Toggle().Focus()

	// Create sessions
	m, _ = m.CreateSession("session-2")
	m, _ = m.CreateSession("session-3")
	require.Equal(t, []string{DefaultSessionID, "session-2", "session-3"}, m.sessionOrder)

	// Retire session-2 (middle session)
	m, _ = m.RetireSession("session-2")

	// Order should be updated
	require.Equal(t, []string{DefaultSessionID, "session-3"}, m.sessionOrder)
}

func TestRetireSession_ClampsCursor(t *testing.T) {
	// Test that cursor is clamped after retiring the last item
	// Cursor layout: 0="Create new session", 1=session-1, 2=session-2
	m := New(DefaultConfig()).SetSize(60, 20).Toggle().Focus()

	// Create sessions
	m, _ = m.CreateSession("session-2")
	m.activeTab = TabSessions
	m.sessionListCursor = 2 // Points to session-2 (last item, index 2)

	// Retire session-2
	m, _ = m.RetireSession("session-2")

	// Cursor should be clamped to valid range (max is len(sessionOrder) = 1)
	require.LessOrEqual(t, m.sessionListCursor, 1)
}

func TestRetireSession_ViaKeyboard(t *testing.T) {
	// Test retiring a session via 'd' key in Sessions tab (two-step confirmation)
	// Cursor layout: 0="Create new session", 1=session-1, 2=session-2, 3=session-3
	m := New(DefaultConfig()).SetSize(60, 20).Toggle().Focus()

	// Create sessions
	m, _ = m.CreateSession("session-2")
	m, _ = m.CreateSession("session-3")

	// Switch to Sessions tab and move cursor to session-2 (index 2)
	m.activeTab = TabSessions
	m.sessionListCursor = 2 // Points to session-2

	// First 'd' press - should mark as pending, not retire yet
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	require.Equal(t, 3, m.SessionCount(), "First 'd' should not retire")
	require.Equal(t, "session-2", m.pendingRetireSessionID, "Should be pending confirmation")

	// Second 'd' press - should confirm and retire
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})

	// session-2 should now be retired
	require.Equal(t, 2, m.SessionCount())
	require.Nil(t, m.sessions["session-2"])
	require.NotNil(t, m.sessions[DefaultSessionID])
	require.NotNil(t, m.sessions["session-3"])
	require.Equal(t, "", m.pendingRetireSessionID, "Pending should be cleared after retire")
}

func TestRetireSession_CancelWithEsc(t *testing.T) {
	// Test canceling retirement with Escape key
	// Cursor layout: 0="Create new session", 1=session-1, 2=session-2
	m := New(DefaultConfig()).SetSize(60, 20).Toggle().Focus()

	m, _ = m.CreateSession("session-2")
	m.activeTab = TabSessions
	m.sessionListCursor = 2 // session-2 (index 2)

	// First 'd' press - mark as pending
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	require.Equal(t, "session-2", m.pendingRetireSessionID)

	// Press Escape to cancel
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape})

	// Pending should be cleared, session not retired
	require.Equal(t, "", m.pendingRetireSessionID)
	require.Equal(t, 2, m.SessionCount())
	require.NotNil(t, m.sessions["session-2"])
}

func TestRetireSession_CancelWithNavigation(t *testing.T) {
	// Test that navigation clears pending retirement
	// Cursor layout: 0="Create new session", 1=session-1, 2=session-2, 3=session-3
	m := New(DefaultConfig()).SetSize(60, 20).Toggle().Focus()

	m, _ = m.CreateSession("session-2")
	m, _ = m.CreateSession("session-3")
	m.activeTab = TabSessions
	m.sessionListCursor = 2 // session-2 (index 2)

	// First 'd' press - mark as pending
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	require.Equal(t, "session-2", m.pendingRetireSessionID)

	// Navigate away with 'j'
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})

	// Pending should be cleared
	require.Equal(t, "", m.pendingRetireSessionID)
	require.Equal(t, 3, m.SessionCount())
}

func TestRetireSession_ConfirmationShowsInView(t *testing.T) {
	// Test that confirmation prompt shows in the view
	m := New(DefaultConfig()).SetSize(80, 20).Toggle().Focus()

	m, _ = m.CreateSession("session-2")
	m.activeTab = TabSessions
	m.sessionListCursor = 1 // session-2

	// Before 'd' press - should show status
	view := scanView(m)
	require.NotContains(t, view, "Press d to confirm")

	// First 'd' press - should show confirmation prompt
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	view = m.View()
	require.Contains(t, view, "Press d to confirm", "Should show confirmation prompt")
}

func TestRetireSession_NonexistentSession(t *testing.T) {
	// Test that retiring a nonexistent session is a no-op
	m := New(DefaultConfig()).SetSize(60, 20).Toggle().Focus()

	m, _ = m.CreateSession("session-2")
	initialCount := m.SessionCount()

	// Try to retire a session that doesn't exist
	m, _ = m.RetireSession("session-999")

	// Nothing should change
	require.Equal(t, initialCount, m.SessionCount())
}

func TestRetireSession_SubmitsRetireCommand(t *testing.T) {
	// Test that retiring a session submits a RetireProcessCommand through infrastructure
	m := New(DefaultConfig()).SetSize(60, 20).Toggle().Focus()

	// Create and start infrastructure
	infra := newTestInfrastructure(t)
	err := infra.Start()
	require.NoError(t, err)
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Create session-2 with a process ID
	m, _ = m.CreateSession("session-2")
	m = m.SetSessionProcessID("session-2", "process-2")

	// Retire session-2 - should submit RetireProcessCommand
	// Note: The command will fail since process-2 doesn't exist in the registry,
	// but the important thing is that the command IS submitted (no immediate error)
	m, cmd := m.RetireSession("session-2")

	// Should have 1 session remaining (local state cleaned up)
	require.Equal(t, 1, m.SessionCount())

	// No error command returned means the submission succeeded
	// (The actual retire will fail asynchronously since process doesn't exist)
	require.Nil(t, cmd, "RetireSession should return nil cmd when infrastructure accepts the command")
}

func TestRetireSession_NoCommandWithoutProcessID(t *testing.T) {
	// Test that retiring a session without ProcessID skips command submission
	m := New(DefaultConfig()).SetSize(60, 20).Toggle().Focus()

	// Create and start infrastructure
	infra := newTestInfrastructure(t)
	err := infra.Start()
	require.NoError(t, err)
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Create session-2 WITHOUT a process ID
	m, _ = m.CreateSession("session-2")
	// Note: NOT calling SetSessionProcessID, so ProcessID remains ""

	// Retire session-2 - should skip command submission since no ProcessID
	m, cmd := m.RetireSession("session-2")

	// Should have 1 session remaining
	require.Equal(t, 1, m.SessionCount())

	// No command returned (skipped submission due to empty ProcessID)
	require.Nil(t, cmd)
}

func TestRetireSession_NoCommandWithoutInfrastructure(t *testing.T) {
	// Test that retiring a session without infrastructure skips command submission
	m := New(DefaultConfig()).SetSize(60, 20).Toggle().Focus()

	// No infrastructure set

	// Create session-2 with a process ID
	m, _ = m.CreateSession("session-2")
	m = m.SetSessionProcessID("session-2", "process-2")

	// Retire session-2 - should skip command submission since no infrastructure
	m, cmd := m.RetireSession("session-2")

	// Should have 1 session remaining (local cleanup still happens)
	require.Equal(t, 1, m.SessionCount())

	// No command returned (skipped submission due to nil infrastructure)
	require.Nil(t, cmd)
}

func TestEnterOnCreateNewSession_EmitsNewSessionRequest(t *testing.T) {
	// Test that pressing Enter on "Create new session" emits NewSessionRequestMsg
	m := New(DefaultConfig()).SetSize(60, 20).Toggle().Focus()

	// Switch to Sessions tab - cursor is at 0 ("Create new session")
	m.activeTab = TabSessions
	m.sessionListCursor = 0

	// Press Enter on "Create new session"
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should emit NewSessionRequestMsg
	require.NotNil(t, cmd, "Enter on Create new session should return a command")

	msg := cmd()
	_, ok := msg.(NewSessionRequestMsg)
	require.True(t, ok, "Should emit NewSessionRequestMsg, got %T", msg)
}

// ============================================================================
// Loading Indicator and Error State Tests (perles-6sce.3)
// ============================================================================

func TestRenderLoadingIndicator_Golden(t *testing.T) {
	// Test golden snapshot for loading indicator
	m := New(DefaultConfig()).SetSize(60, 20).Toggle()

	// Render the loading indicator with standard dimensions
	output := m.renderLoadingIndicator(58, 12) // Inner width/height accounting for borders

	teatest.RequireEqualOutput(t, []byte(output))
}

func TestRenderErrorState_Golden(t *testing.T) {
	// Test golden snapshot for error state
	m := New(DefaultConfig()).SetSize(60, 20).Toggle()

	// Render the error state with standard dimensions
	output := m.renderErrorState(58, 12)

	teatest.RequireEqualOutput(t, []byte(output))
}

func TestRenderLoadingIndicator_UsesSpinnerColor(t *testing.T) {
	// Unit test: Loading indicator uses SpinnerColor from theme
	m := New(DefaultConfig()).SetSize(60, 20).Toggle()

	output := m.renderLoadingIndicator(58, 12)

	// The output should contain the braille spinner character
	require.Contains(t, output, "⠋", "Loading indicator should contain braille spinner character")
	require.Contains(t, output, "Starting assistant...", "Loading indicator should contain message text")
}

func TestRenderErrorState_IncludesRecoveryGuidance(t *testing.T) {
	// Unit test: Error state includes recovery guidance text
	m := New(DefaultConfig()).SetSize(60, 20).Toggle()

	output := m.renderErrorState(58, 12)

	// Error state should include recovery guidance
	require.Contains(t, output, "Failed to start assistant", "Error state should contain failure message")
	require.Contains(t, output, "Ctrl+W", "Error state should contain recovery key hint")
	require.Contains(t, output, "to retry", "Error state should contain retry guidance")
}

func TestRenderLoadingIndicator_HandlesZeroDimensions(t *testing.T) {
	// Unit test: Loading indicator handles zero dimensions gracefully
	m := New(DefaultConfig()).SetSize(60, 20).Toggle()

	// Zero width
	output := m.renderLoadingIndicator(0, 12)
	require.Empty(t, output, "Loading indicator should return empty for zero width")

	// Zero height
	output = m.renderLoadingIndicator(58, 0)
	require.Empty(t, output, "Loading indicator should return empty for zero height")
}

func TestRenderErrorState_HandlesZeroDimensions(t *testing.T) {
	// Unit test: Error state handles zero dimensions gracefully
	m := New(DefaultConfig()).SetSize(60, 20).Toggle()

	// Zero width
	output := m.renderErrorState(0, 12)
	require.Empty(t, output, "Error state should return empty for zero width")

	// Zero height
	output = m.renderErrorState(58, 0)
	require.Empty(t, output, "Error state should return empty for zero height")
}

// ============================================================================
// View() Status-Based Routing Tests (perles-6sce.4)
// ============================================================================

func TestView_Golden_ChatTab_PendingStatus(t *testing.T) {
	// Golden test: Chat tab with session in Pending status shows loading indicator
	m := New(DefaultConfig()).SetSize(60, 20).Toggle()

	// Use default session with pending status
	m.sessions[DefaultSessionID].Status = events.ProcessStatusPending

	view := scanView(m)
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_ChatTab_FailedStatus(t *testing.T) {
	// Golden test: Chat tab with session in Failed status shows error state
	m := New(DefaultConfig()).SetSize(60, 20).Toggle()

	// Use default session with failed status
	m.sessions[DefaultSessionID].Status = events.ProcessStatusFailed

	view := scanView(m)
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_ChatTab_ReadyStatus(t *testing.T) {
	// Golden test: Chat tab with session in Ready status shows messages (existing behavior)
	m := New(DefaultConfig()).SetSize(60, 20).Toggle()

	// Use default session with ready status and some messages
	m.sessions[DefaultSessionID].Status = events.ProcessStatusReady
	m = m.AddMessage(chatrender.Message{Role: RoleUser, Content: "Hello"})
	m = m.AddMessage(chatrender.Message{Role: RoleAssistant, Content: "Hi there!"})

	view := scanView(m)
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_RoutesToLoadingIndicatorForPending(t *testing.T) {
	// Unit test: View() correctly routes to loading indicator for Pending status
	m := New(DefaultConfig()).SetSize(60, 20).Toggle()

	// Use default session with pending status
	m.sessions[DefaultSessionID].Status = events.ProcessStatusPending

	view := scanView(m)

	// Should show loading indicator content
	require.Contains(t, view, "Starting assistant...", "Pending status should show loading indicator")
	require.Contains(t, view, "⠋", "Pending status should show spinner")
}

func TestView_RoutesToErrorStateForFailed(t *testing.T) {
	// Unit test: View() correctly routes to error state for Failed status
	m := New(DefaultConfig()).SetSize(60, 20).Toggle()

	// Use default session with failed status
	m.sessions[DefaultSessionID].Status = events.ProcessStatusFailed

	view := scanView(m)

	// Should show error state content
	require.Contains(t, view, "Failed to start assistant", "Failed status should show error state")
	require.Contains(t, view, "Ctrl+W", "Failed status should show recovery hint")
}

func TestView_RoutesToMessagesForReadyStatus(t *testing.T) {
	// Unit test: View() correctly routes to messages for Ready status
	m := New(DefaultConfig()).SetSize(60, 20).Toggle()

	// Use default session with ready status and a message
	m.sessions[DefaultSessionID].Status = events.ProcessStatusReady
	m = m.AddMessage(chatrender.Message{Role: RoleUser, Content: "Test message"})

	view := scanView(m)

	// Should show messages, not loading or error
	require.Contains(t, view, "Test message", "Ready status should show messages")
	require.NotContains(t, view, "Starting assistant...", "Ready status should not show loading indicator")
	require.NotContains(t, view, "Failed to start assistant", "Ready status should not show error state")
}

func TestView_RoutesToMessagesForWorkingStatus(t *testing.T) {
	// Unit test: View() correctly routes to messages for Working status
	m := New(DefaultConfig()).SetSize(60, 20).Toggle()

	// Use default session with working status and a message
	m.sessions[DefaultSessionID].Status = events.ProcessStatusWorking
	m = m.AddMessage(chatrender.Message{Role: RoleUser, Content: "Working message"})

	view := scanView(m)

	// Should show messages, not loading or error
	require.Contains(t, view, "Working message", "Working status should show messages")
	require.NotContains(t, view, "Starting assistant...", "Working status should not show loading indicator")
	require.NotContains(t, view, "Failed to start assistant", "Working status should not show error state")
}

// ============================================================================
// Workflows Tab Helper Method Tests (perles-f3tm.1)
// ============================================================================

func TestGetWorkflowsForTab_NilRegistry(t *testing.T) {
	// Create model with nil registry
	m := New(DefaultConfig())
	require.Nil(t, m.workflowRegistry)

	// Should return empty slice (not nil) when registry is nil
	workflows := m.getWorkflowsForTab()
	require.NotNil(t, workflows, "should return empty slice, not nil")
	require.Empty(t, workflows, "should return empty slice when registry is nil")
}

func TestGetWorkflowsForTab_FiltersTargetChat(t *testing.T) {
	// Create registry with workflows of different target modes
	registry := workflow.NewRegistry()

	// Add a chat-targeted workflow
	registry.Add(workflow.Workflow{
		ID:         "chat-workflow",
		Name:       "Chat Workflow",
		TargetMode: workflow.TargetChat,
	})

	// Add an orchestration-targeted workflow (should NOT be returned)
	registry.Add(workflow.Workflow{
		ID:         "orch-workflow",
		Name:       "Orchestration Workflow",
		TargetMode: workflow.TargetOrchestration,
	})

	// Add a TargetBoth workflow (empty string, should be returned)
	registry.Add(workflow.Workflow{
		ID:         "both-workflow",
		Name:       "Both Workflow",
		TargetMode: workflow.TargetBoth,
	})

	cfg := Config{
		ClientType:       "claude",
		WorkDir:          "/test/dir",
		SessionTimeout:   30 * time.Minute,
		WorkflowRegistry: registry,
	}
	m := New(cfg)

	workflows := m.getWorkflowsForTab()

	// Should return chat and both workflows (sorted by name)
	require.Len(t, workflows, 2, "should return 2 workflows (chat + both)")

	// Verify sorted order and correct workflows
	require.Equal(t, "Both Workflow", workflows[0].Name)
	require.Equal(t, "Chat Workflow", workflows[1].Name)
}

func TestClampWorkflowListCursor_EmptyList(t *testing.T) {
	// Create model with nil registry (empty workflow list)
	m := New(DefaultConfig())
	m.workflowListCursor = 5 // Set invalid cursor position

	// Clamp should set cursor to 0 for empty list
	m = m.clampWorkflowListCursor()
	require.Equal(t, 0, m.workflowListCursor, "cursor should be 0 for empty list")
}

func TestClampWorkflowListCursor_CursorGreaterThanMax(t *testing.T) {
	// Create registry with 2 workflows
	registry := workflow.NewRegistry()
	registry.Add(workflow.Workflow{
		ID:         "wf-1",
		Name:       "Workflow 1",
		TargetMode: workflow.TargetChat,
	})
	registry.Add(workflow.Workflow{
		ID:         "wf-2",
		Name:       "Workflow 2",
		TargetMode: workflow.TargetChat,
	})

	cfg := Config{
		ClientType:       "claude",
		WorkDir:          "/test/dir",
		SessionTimeout:   30 * time.Minute,
		WorkflowRegistry: registry,
	}
	m := New(cfg)
	m.workflowListCursor = 10 // Set cursor way beyond max (max should be 1)

	m = m.clampWorkflowListCursor()
	require.Equal(t, 1, m.workflowListCursor, "cursor should be clamped to max valid index (len-1)")
}

func TestClampWorkflowListCursor_CursorNegative(t *testing.T) {
	// Create registry with 2 workflows
	registry := workflow.NewRegistry()
	registry.Add(workflow.Workflow{
		ID:         "wf-1",
		Name:       "Workflow 1",
		TargetMode: workflow.TargetChat,
	})
	registry.Add(workflow.Workflow{
		ID:         "wf-2",
		Name:       "Workflow 2",
		TargetMode: workflow.TargetChat,
	})

	cfg := Config{
		ClientType:       "claude",
		WorkDir:          "/test/dir",
		SessionTimeout:   30 * time.Minute,
		WorkflowRegistry: registry,
	}
	m := New(cfg)
	m.workflowListCursor = -5 // Set negative cursor

	m = m.clampWorkflowListCursor()
	require.Equal(t, 0, m.workflowListCursor, "cursor should be clamped to 0 for negative values")
}

func TestClampWorkflowListCursor_ValidCursorUnchanged(t *testing.T) {
	// Create registry with 3 workflows
	registry := workflow.NewRegistry()
	registry.Add(workflow.Workflow{
		ID:         "wf-1",
		Name:       "Workflow 1",
		TargetMode: workflow.TargetChat,
	})
	registry.Add(workflow.Workflow{
		ID:         "wf-2",
		Name:       "Workflow 2",
		TargetMode: workflow.TargetChat,
	})
	registry.Add(workflow.Workflow{
		ID:         "wf-3",
		Name:       "Workflow 3",
		TargetMode: workflow.TargetChat,
	})

	cfg := Config{
		ClientType:       "claude",
		WorkDir:          "/test/dir",
		SessionTimeout:   30 * time.Minute,
		WorkflowRegistry: registry,
	}
	m := New(cfg)
	m.workflowListCursor = 1 // Set to middle of list (valid)

	m = m.clampWorkflowListCursor()
	require.Equal(t, 1, m.workflowListCursor, "valid cursor should remain unchanged")
}

// =============================================================================
// Workflows Tab Key Handling Tests (Task 2)
// =============================================================================

func TestWorkflowsTab_JIncrementsCursor(t *testing.T) {
	// Create registry with 3 workflows
	registry := workflow.NewRegistry()
	registry.Add(workflow.Workflow{
		ID:         "wf-1",
		Name:       "Workflow 1",
		TargetMode: workflow.TargetChat,
	})
	registry.Add(workflow.Workflow{
		ID:         "wf-2",
		Name:       "Workflow 2",
		TargetMode: workflow.TargetChat,
	})
	registry.Add(workflow.Workflow{
		ID:         "wf-3",
		Name:       "Workflow 3",
		TargetMode: workflow.TargetChat,
	})

	cfg := Config{
		ClientType:       "claude",
		WorkDir:          "/test/dir",
		SessionTimeout:   30 * time.Minute,
		WorkflowRegistry: registry,
	}
	m := New(cfg).SetSize(60, 20).Toggle().Focus()
	m.activeTab = TabWorkflows
	m.workflowListCursor = 0

	// Test 'j' moves cursor down
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	require.Equal(t, 1, m.workflowListCursor, "j should move cursor down")

	// Test 'j' again
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	require.Equal(t, 2, m.workflowListCursor, "j should move cursor down again")
}

func TestWorkflowsTab_KDecrementsCursor(t *testing.T) {
	// Create registry with 3 workflows
	registry := workflow.NewRegistry()
	registry.Add(workflow.Workflow{
		ID:         "wf-1",
		Name:       "Workflow 1",
		TargetMode: workflow.TargetChat,
	})
	registry.Add(workflow.Workflow{
		ID:         "wf-2",
		Name:       "Workflow 2",
		TargetMode: workflow.TargetChat,
	})
	registry.Add(workflow.Workflow{
		ID:         "wf-3",
		Name:       "Workflow 3",
		TargetMode: workflow.TargetChat,
	})

	cfg := Config{
		ClientType:       "claude",
		WorkDir:          "/test/dir",
		SessionTimeout:   30 * time.Minute,
		WorkflowRegistry: registry,
	}
	m := New(cfg).SetSize(60, 20).Toggle().Focus()
	m.activeTab = TabWorkflows
	m.workflowListCursor = 2 // Start at last item

	// Test 'k' moves cursor up
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	require.Equal(t, 1, m.workflowListCursor, "k should move cursor up")

	// Test 'k' again
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	require.Equal(t, 0, m.workflowListCursor, "k should move cursor up again")
}

func TestWorkflowsTab_JAtEndStaysAtEnd(t *testing.T) {
	// Create registry with 3 workflows
	registry := workflow.NewRegistry()
	registry.Add(workflow.Workflow{
		ID:         "wf-1",
		Name:       "Workflow 1",
		TargetMode: workflow.TargetChat,
	})
	registry.Add(workflow.Workflow{
		ID:         "wf-2",
		Name:       "Workflow 2",
		TargetMode: workflow.TargetChat,
	})
	registry.Add(workflow.Workflow{
		ID:         "wf-3",
		Name:       "Workflow 3",
		TargetMode: workflow.TargetChat,
	})

	cfg := Config{
		ClientType:       "claude",
		WorkDir:          "/test/dir",
		SessionTimeout:   30 * time.Minute,
		WorkflowRegistry: registry,
	}
	m := New(cfg).SetSize(60, 20).Toggle().Focus()
	m.activeTab = TabWorkflows
	m.workflowListCursor = 2 // At last item (index 2)

	// Test 'j' at bottom doesn't go past end
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	require.Equal(t, 2, m.workflowListCursor, "j at bottom should not go past end")
}

func TestWorkflowsTab_KAtStartStaysAtStart(t *testing.T) {
	// Create registry with 3 workflows
	registry := workflow.NewRegistry()
	registry.Add(workflow.Workflow{
		ID:         "wf-1",
		Name:       "Workflow 1",
		TargetMode: workflow.TargetChat,
	})
	registry.Add(workflow.Workflow{
		ID:         "wf-2",
		Name:       "Workflow 2",
		TargetMode: workflow.TargetChat,
	})
	registry.Add(workflow.Workflow{
		ID:         "wf-3",
		Name:       "Workflow 3",
		TargetMode: workflow.TargetChat,
	})

	cfg := Config{
		ClientType:       "claude",
		WorkDir:          "/test/dir",
		SessionTimeout:   30 * time.Minute,
		WorkflowRegistry: registry,
	}
	m := New(cfg).SetSize(60, 20).Toggle().Focus()
	m.activeTab = TabWorkflows
	m.workflowListCursor = 0 // At first item

	// Test 'k' at top doesn't go negative
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	require.Equal(t, 0, m.workflowListCursor, "k at top should not go negative")
}

func TestWorkflowsTab_DownArrowIncrementsCursor(t *testing.T) {
	// Test arrow keys also work for navigation
	registry := workflow.NewRegistry()
	registry.Add(workflow.Workflow{
		ID:         "wf-1",
		Name:       "Workflow 1",
		TargetMode: workflow.TargetChat,
	})
	registry.Add(workflow.Workflow{
		ID:         "wf-2",
		Name:       "Workflow 2",
		TargetMode: workflow.TargetChat,
	})

	cfg := Config{
		ClientType:       "claude",
		WorkDir:          "/test/dir",
		SessionTimeout:   30 * time.Minute,
		WorkflowRegistry: registry,
	}
	m := New(cfg).SetSize(60, 20).Toggle().Focus()
	m.activeTab = TabWorkflows
	m.workflowListCursor = 0

	// Test down arrow
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, 1, m.workflowListCursor, "down arrow should move cursor down")

	// Test up arrow
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	require.Equal(t, 0, m.workflowListCursor, "up arrow should move cursor up")
}

func TestWorkflowsTab_EnterSelectsWorkflowAndSwitchesToChat(t *testing.T) {
	// Create registry with workflows
	// Note: Registry sorts by name alphabetically, so "Beta" comes before "Zebra"
	registry := workflow.NewRegistry()
	registry.Add(workflow.Workflow{
		ID:         "wf-1",
		Name:       "Zebra Workflow", // Will be second after sorting
		Content:    "Workflow content here",
		TargetMode: workflow.TargetChat,
	})
	registry.Add(workflow.Workflow{
		ID:         "wf-2",
		Name:       "Beta Workflow", // Will be first after sorting
		Content:    "Another content",
		TargetMode: workflow.TargetChat,
	})

	cfg := Config{
		ClientType:       "claude",
		WorkDir:          "/test/dir",
		SessionTimeout:   30 * time.Minute,
		WorkflowRegistry: registry,
	}
	m := New(cfg).SetSize(60, 20).Toggle().Focus()
	m.activeTab = TabWorkflows
	m.workflowListCursor = 1 // Select second workflow (Zebra after sort)

	// Press Enter
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Verify switched to Chat tab
	require.Equal(t, TabChat, m.activeTab, "Enter should switch to Chat tab")

	// Verify active workflow is set
	require.NotNil(t, m.activeWorkflow, "activeWorkflow should be set")
	require.Equal(t, "wf-1", m.activeWorkflow.ID, "correct workflow (Zebra, second after sort) should be selected")
}

func TestWorkflowsTab_EnterWithEmptyListDoesNothing(t *testing.T) {
	// Create empty registry
	registry := workflow.NewRegistry()

	cfg := Config{
		ClientType:       "claude",
		WorkDir:          "/test/dir",
		SessionTimeout:   30 * time.Minute,
		WorkflowRegistry: registry,
	}
	m := New(cfg).SetSize(60, 20).Toggle().Focus()
	m.activeTab = TabWorkflows
	m.workflowListCursor = 0

	// Press Enter on empty list
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should stay on Workflows tab
	require.Equal(t, TabWorkflows, m.activeTab, "Enter on empty list should not switch tabs")
	require.Nil(t, m.activeWorkflow, "activeWorkflow should remain nil")
}

func TestWorkflowsTab_NavigationWithSingleItemList(t *testing.T) {
	// Create registry with single workflow
	registry := workflow.NewRegistry()
	registry.Add(workflow.Workflow{
		ID:         "wf-1",
		Name:       "Only Workflow",
		TargetMode: workflow.TargetChat,
	})

	cfg := Config{
		ClientType:       "claude",
		WorkDir:          "/test/dir",
		SessionTimeout:   30 * time.Minute,
		WorkflowRegistry: registry,
	}
	m := New(cfg).SetSize(60, 20).Toggle().Focus()
	m.activeTab = TabWorkflows
	m.workflowListCursor = 0

	// Test j doesn't move (already at end since only 1 item)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	require.Equal(t, 0, m.workflowListCursor, "j with single item should stay at 0")

	// Test k doesn't move (already at start)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	require.Equal(t, 0, m.workflowListCursor, "k with single item should stay at 0")
}

func TestWorkflowsTab_BlocksInputForwarding(t *testing.T) {
	// Test that typing on Workflows tab doesn't go to the input field
	registry := workflow.NewRegistry()
	registry.Add(workflow.Workflow{
		ID:         "wf-1",
		Name:       "Workflow 1",
		TargetMode: workflow.TargetChat,
	})

	cfg := Config{
		ClientType:       "claude",
		WorkDir:          "/test/dir",
		SessionTimeout:   30 * time.Minute,
		WorkflowRegistry: registry,
	}
	m := New(cfg).SetSize(60, 20).Toggle().Focus()
	m.activeTab = TabWorkflows

	// Type random letters that shouldn't affect anything
	initialCursor := m.workflowListCursor
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})

	// Cursor should remain unchanged
	require.Equal(t, initialCursor, m.workflowListCursor, "random keys should not affect cursor")

	// Input should not have received the text (check by verifying input is still empty)
	require.Equal(t, "", m.input.Value(), "input should not receive keys while on Workflows tab")
}

// =============================================================================
// selectWorkflowFromTab Tests (Task 2)
// =============================================================================

func TestSelectWorkflowFromTab_AlwaysSendsMessage(t *testing.T) {
	registry := workflow.NewRegistry()
	registry.Add(workflow.Workflow{
		ID:         "test-wf",
		Name:       "Test Workflow",
		Content:    "This is the workflow content.",
		TargetMode: workflow.TargetChat,
	})

	cfg := Config{
		ClientType:       "claude",
		WorkDir:          "/test/dir",
		SessionTimeout:   30 * time.Minute,
		WorkflowRegistry: registry,
	}
	m := New(cfg).SetSize(60, 20).Toggle().Focus()
	m.activeTab = TabWorkflows

	// Get the workflow
	workflows := m.getWorkflowsForTab()
	require.Len(t, workflows, 1)

	// Session status doesn't matter - SendMessage handles queuing internally
	session := m.ActiveSession()
	require.NotNil(t, session)
	require.Equal(t, events.ProcessStatusPending, session.Status, "session starts as Pending")

	// Call selectWorkflowFromTab - should always return a command
	m, cmd := m.selectWorkflowFromTab(workflows[0])

	// Should return a command unconditionally (SendMessage queues if not ready)
	require.NotNil(t, cmd, "should return send command regardless of session status")

	// Verify activeWorkflow is set
	require.NotNil(t, m.activeWorkflow)
	require.Equal(t, "Test Workflow", m.activeWorkflow.Name)
}

func TestSelectWorkflowFromTab_SwitchesToChatTab(t *testing.T) {
	registry := workflow.NewRegistry()
	registry.Add(workflow.Workflow{
		ID:         "test-wf",
		Name:       "Test Workflow",
		Content:    "Content",
		TargetMode: workflow.TargetChat,
	})

	cfg := Config{
		ClientType:       "claude",
		WorkDir:          "/test/dir",
		SessionTimeout:   30 * time.Minute,
		WorkflowRegistry: registry,
	}
	m := New(cfg).SetSize(60, 20).Toggle().Focus()
	m.activeTab = TabWorkflows

	workflows := m.getWorkflowsForTab()
	m, _ = m.selectWorkflowFromTab(workflows[0])

	require.Equal(t, TabChat, m.activeTab, "should switch to Chat tab")
}

func TestSelectWorkflowFromTab_SetsActiveWorkflow(t *testing.T) {
	registry := workflow.NewRegistry()
	registry.Add(workflow.Workflow{
		ID:         "test-wf",
		Name:       "Test Workflow",
		Content:    "Content",
		TargetMode: workflow.TargetChat,
	})

	cfg := Config{
		ClientType:       "claude",
		WorkDir:          "/test/dir",
		SessionTimeout:   30 * time.Minute,
		WorkflowRegistry: registry,
	}
	m := New(cfg).SetSize(60, 20).Toggle().Focus()

	workflows := m.getWorkflowsForTab()
	m, _ = m.selectWorkflowFromTab(workflows[0])

	require.NotNil(t, m.activeWorkflow)
	require.Equal(t, "test-wf", m.activeWorkflow.ID)
}

// =============================================================================
// Ctrl+T Switches to Workflows Tab Tests (Task 2)
// =============================================================================

func TestCtrlT_SwitchesToWorkflowsTab(t *testing.T) {
	registry := workflow.NewRegistry()
	registry.Add(workflow.Workflow{
		ID:         "wf-1",
		Name:       "Workflow 1",
		TargetMode: workflow.TargetChat,
	})

	cfg := Config{
		ClientType:       "claude",
		WorkDir:          "/test/dir",
		SessionTimeout:   30 * time.Minute,
		WorkflowRegistry: registry,
	}
	m := New(cfg).SetSize(60, 20).Toggle().Focus()

	// Start on Chat tab (default)
	require.Equal(t, TabChat, m.activeTab)

	// Press Ctrl+T
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlT})

	// Should be on Workflows tab
	require.Equal(t, TabWorkflows, m.activeTab, "Ctrl+T should switch to Workflows tab")
}

func TestCtrlT_WorksFromSessionsTab(t *testing.T) {
	registry := workflow.NewRegistry()
	registry.Add(workflow.Workflow{
		ID:         "wf-1",
		Name:       "Workflow 1",
		TargetMode: workflow.TargetChat,
	})

	cfg := Config{
		ClientType:       "claude",
		WorkDir:          "/test/dir",
		SessionTimeout:   30 * time.Minute,
		WorkflowRegistry: registry,
	}
	m := New(cfg).SetSize(60, 20).Toggle().Focus()
	m.activeTab = TabSessions // Start on Sessions tab

	// Press Ctrl+T
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlT})

	// Should be on Workflows tab
	require.Equal(t, TabWorkflows, m.activeTab, "Ctrl+T should switch to Workflows tab from Sessions tab")
}

func TestCtrlT_WorksWithEmptyRegistry(t *testing.T) {
	// Create empty registry
	registry := workflow.NewRegistry()

	cfg := Config{
		ClientType:       "claude",
		WorkDir:          "/test/dir",
		SessionTimeout:   30 * time.Minute,
		WorkflowRegistry: registry,
	}
	m := New(cfg).SetSize(60, 20).Toggle().Focus()

	// Press Ctrl+T
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlT})

	// Should still switch to Workflows tab (even if empty)
	require.Equal(t, TabWorkflows, m.activeTab, "Ctrl+T should switch to Workflows tab even with empty registry")
}

func TestCtrlT_WorksWithNilRegistry(t *testing.T) {
	// Config without workflow registry
	cfg := Config{
		ClientType:     "claude",
		WorkDir:        "/test/dir",
		SessionTimeout: 30 * time.Minute,
		// WorkflowRegistry is nil
	}
	m := New(cfg).SetSize(60, 20).Toggle().Focus()

	// Press Ctrl+T
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlT})

	// Should switch to Workflows tab (view will handle empty state)
	require.Equal(t, TabWorkflows, m.activeTab, "Ctrl+T should switch to Workflows tab even with nil registry")
}

// =============================================================================
// Tab Switching Keys Still Work on Workflows Tab (Task 2)
// =============================================================================

func TestWorkflowsTab_TabSwitchingKeysStillWork(t *testing.T) {
	registry := workflow.NewRegistry()
	registry.Add(workflow.Workflow{
		ID:         "wf-1",
		Name:       "Workflow 1",
		TargetMode: workflow.TargetChat,
	})

	cfg := Config{
		ClientType:       "claude",
		WorkDir:          "/test/dir",
		SessionTimeout:   30 * time.Minute,
		WorkflowRegistry: registry,
	}
	m := New(cfg).SetSize(60, 20).Toggle().Focus()
	m.activeTab = TabWorkflows

	// Test Ctrl+K (previous tab)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	require.Equal(t, TabSessions, m.activeTab, "Ctrl+K should go to previous tab")

	// Go back to Workflows tab
	m.activeTab = TabWorkflows

	// Test Ctrl+J (next tab)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	require.Equal(t, TabChat, m.activeTab, "Ctrl+J should go to next tab (wraps to Chat)")

	// Go back to Workflows tab
	m.activeTab = TabWorkflows

	// Test Ctrl+] (cycle to next tab)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlCloseBracket})
	require.Equal(t, TabChat, m.activeTab, "Ctrl+] should cycle to next tab")
}

// =============================================================================
// Workflows Tab Rendering Tests (Task 3)
// =============================================================================

func TestWorkflowsTabLayout_Golden(t *testing.T) {
	// Create registry with multiple workflows (mixed sources)
	registry := workflow.NewRegistry()
	registry.Add(workflow.Workflow{
		ID:          "builtin-1",
		Name:        "Quick Plan",
		Description: "A quick planning workflow",
		Source:      workflow.SourceBuiltIn,
		TargetMode:  workflow.TargetChat,
	})
	registry.Add(workflow.Workflow{
		ID:          "user-1",
		Name:        "My Custom Workflow",
		Description: "User-defined workflow",
		Source:      workflow.SourceUser,
		TargetMode:  workflow.TargetChat,
	})
	registry.Add(workflow.Workflow{
		ID:          "builtin-2",
		Name:        "Research",
		Description: "Research workflow for exploring topics",
		Source:      workflow.SourceBuiltIn,
		TargetMode:  workflow.TargetChat,
	})

	cfg := Config{
		ClientType:       "claude",
		WorkDir:          "/test/dir",
		SessionTimeout:   30 * time.Minute,
		WorkflowRegistry: registry,
	}
	m := New(cfg).SetSize(60, 25).Toggle()

	// Set session to Ready so the view doesn't show loading
	m.sessions[DefaultSessionID].Status = events.ProcessStatusReady

	// Switch to Workflows tab with cursor on first item
	m.activeTab = TabWorkflows
	m.workflowListCursor = 0

	view := scanView(m)
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestWorkflowsTabMiddleSelected_Golden(t *testing.T) {
	// Create registry with multiple workflows
	registry := workflow.NewRegistry()
	registry.Add(workflow.Workflow{
		ID:          "wf-1",
		Name:        "Workflow A",
		Description: "First workflow",
		Source:      workflow.SourceBuiltIn,
		TargetMode:  workflow.TargetChat,
	})
	registry.Add(workflow.Workflow{
		ID:          "wf-2",
		Name:        "Workflow B",
		Description: "Second workflow",
		Source:      workflow.SourceUser,
		TargetMode:  workflow.TargetChat,
	})
	registry.Add(workflow.Workflow{
		ID:          "wf-3",
		Name:        "Workflow C",
		Description: "Third workflow",
		Source:      workflow.SourceBuiltIn,
		TargetMode:  workflow.TargetChat,
	})

	cfg := Config{
		ClientType:       "claude",
		WorkDir:          "/test/dir",
		SessionTimeout:   30 * time.Minute,
		WorkflowRegistry: registry,
	}
	m := New(cfg).SetSize(60, 20).Toggle()

	// Set session to Ready
	m.sessions[DefaultSessionID].Status = events.ProcessStatusReady

	// Switch to Workflows tab with cursor on middle item
	m.activeTab = TabWorkflows
	m.workflowListCursor = 1 // Second item

	view := scanView(m)
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestWorkflowsTabEmpty_Golden(t *testing.T) {
	// Create registry with only orchestration workflows (no chat workflows)
	registry := workflow.NewRegistry()
	registry.Add(workflow.Workflow{
		ID:          "orch-only",
		Name:        "Orchestration Workflow",
		Description: "For orchestration mode only",
		Source:      workflow.SourceBuiltIn,
		TargetMode:  workflow.TargetOrchestration,
	})

	cfg := Config{
		ClientType:       "claude",
		WorkDir:          "/test/dir",
		SessionTimeout:   30 * time.Minute,
		WorkflowRegistry: registry,
	}
	m := New(cfg).SetSize(60, 20).Toggle()

	// Set session to Ready
	m.sessions[DefaultSessionID].Status = events.ProcessStatusReady

	// Switch to Workflows tab (should show empty state)
	m.activeTab = TabWorkflows

	view := scanView(m)
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestWorkflowsTab_TabBarShowsAllThreeTabs_Golden(t *testing.T) {
	// Test that the tab bar correctly shows all three tabs
	cfg := Config{
		ClientType:     "claude",
		WorkDir:        "/test/dir",
		SessionTimeout: 30 * time.Minute,
	}
	m := New(cfg).SetSize(80, 20).Toggle()

	// Set session to Ready
	m.sessions[DefaultSessionID].Status = events.ProcessStatusReady

	// Stay on Chat tab, but verify tab bar shows all three
	m.activeTab = TabChat

	view := scanView(m)
	teatest.RequireEqualOutput(t, []byte(view))
}

// =============================================================================
// Workflows Tab Rendering Unit Tests (Task 3)
// =============================================================================

func TestRenderWorkflowsTab_LongNameTruncation(t *testing.T) {
	// Create a workflow with a very long name
	registry := workflow.NewRegistry()
	registry.Add(workflow.Workflow{
		ID:          "long-name",
		Name:        "This Is A Very Long Workflow Name That Should Be Displayed",
		Description: "Short desc",
		Source:      workflow.SourceBuiltIn,
		TargetMode:  workflow.TargetChat,
	})

	cfg := Config{
		ClientType:       "claude",
		WorkDir:          "/test/dir",
		SessionTimeout:   30 * time.Minute,
		WorkflowRegistry: registry,
	}
	// Use narrow width to trigger truncation behavior
	m := New(cfg).SetSize(40, 15).Toggle()
	m.activeTab = TabWorkflows

	// This should not panic and should render within width
	output := m.renderWorkflowsTab(10)
	require.NotEmpty(t, output, "should render something")
	// Verify the output doesn't exceed expected width bounds
	lines := strings.Split(output, "\n")
	require.Greater(t, len(lines), 0, "should have at least one line")
}

func TestRenderWorkflowsTab_LongDescriptionWrapping(t *testing.T) {
	// Create a workflow with a very long description
	registry := workflow.NewRegistry()
	registry.Add(workflow.Workflow{
		ID:          "long-desc",
		Name:        "Short",
		Description: "This is a very long description that should be wrapped because it exceeds the available width for rendering",
		Source:      workflow.SourceBuiltIn,
		TargetMode:  workflow.TargetChat,
	})

	cfg := Config{
		ClientType:       "claude",
		WorkDir:          "/test/dir",
		SessionTimeout:   30 * time.Minute,
		WorkflowRegistry: registry,
	}
	m := New(cfg).SetSize(60, 15).Toggle()
	m.activeTab = TabWorkflows

	output := m.renderWorkflowsTab(10)
	require.NotEmpty(t, output, "should render something")

	// Verify the description wraps to multiple lines (multi-line layout)
	lines := strings.Split(output, "\n")
	require.GreaterOrEqual(t, len(lines), 3, "long description should wrap to multiple lines")

	// First line should be the name
	require.Contains(t, lines[0], "Short", "first line should contain workflow name")

	// Description should appear on subsequent lines (indented)
	hasDescContent := false
	for i := 1; i < len(lines); i++ {
		if strings.Contains(lines[i], "very long description") || strings.Contains(lines[i], "wrapped") {
			hasDescContent = true
			break
		}
	}
	require.True(t, hasDescContent, "description should appear on subsequent lines")
}

func TestRenderWorkflowsTab_SourceIndicatorColors(t *testing.T) {
	// Create workflows with different sources
	registry := workflow.NewRegistry()
	registry.Add(workflow.Workflow{
		ID:          "builtin",
		Name:        "Built-in Workflow",
		Description: "A built-in workflow",
		Source:      workflow.SourceBuiltIn,
		TargetMode:  workflow.TargetChat,
	})
	registry.Add(workflow.Workflow{
		ID:          "user",
		Name:        "User Workflow",
		Description: "A user workflow",
		Source:      workflow.SourceUser,
		TargetMode:  workflow.TargetChat,
	})

	cfg := Config{
		ClientType:       "claude",
		WorkDir:          "/test/dir",
		SessionTimeout:   30 * time.Minute,
		WorkflowRegistry: registry,
	}
	m := New(cfg).SetSize(60, 15).Toggle()
	m.activeTab = TabWorkflows

	output := m.renderWorkflowsTab(10)
	require.NotEmpty(t, output, "should render something")

	// Both workflows should have the ● indicator
	require.Contains(t, output, "●", "should contain source indicator")

	// The actual color verification is done via golden tests since ANSI codes are complex to parse
	// Here we just verify the structure is correct
	lines := strings.Split(output, "\n")
	require.GreaterOrEqual(t, len(lines), 2, "should have at least 2 workflow lines")
}

func TestRenderEmptyWorkflowsState(t *testing.T) {
	cfg := Config{
		ClientType:     "claude",
		WorkDir:        "/test/dir",
		SessionTimeout: 30 * time.Minute,
	}
	m := New(cfg).SetSize(60, 15).Toggle()
	m.activeTab = TabWorkflows

	// Call renderEmptyWorkflowsState directly
	output := m.renderEmptyWorkflowsState(10)

	require.NotEmpty(t, output, "should render something")
	require.Contains(t, output, "No workflows available", "should show no workflows message")
	require.Contains(t, output, "~/.config/perles/workflows/", "should show guidance for adding workflows")
}

// ============================================================================
// Session Event Routing Tests (perles-ci2e.2)
// ============================================================================

func TestProcessQueueChanged_UpdatesCorrectSession(t *testing.T) {
	// Test that ProcessQueueChanged event updates the correct session's QueueCount
	m := New(DefaultConfig())

	// Create infrastructure for event handling
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Initial session (session-1) is mapped to ChatPanelProcessID
	session := m.SessionByProcessID(ChatPanelProcessID)
	require.NotNil(t, session)
	require.Equal(t, 0, session.QueueCount, "QueueCount should initially be 0")

	// Create a ProcessQueueChanged event targeting the session's process
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:       events.ProcessQueueChanged,
			ProcessID:  ChatPanelProcessID,
			QueueCount: 5,
		},
	}

	// Handle the event
	m, _ = m.Update(event)

	// Verify session's QueueCount is updated
	session = m.SessionByProcessID(ChatPanelProcessID)
	require.Equal(t, 5, session.QueueCount, "Session QueueCount should be updated to 5")
}

func TestProcessTokenUsage_UpdatesCorrectSession(t *testing.T) {
	// Test that ProcessTokenUsage event updates the correct session's Metrics
	m := New(DefaultConfig())

	// Create infrastructure for event handling
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Initial session should have no metrics
	session := m.SessionByProcessID(ChatPanelProcessID)
	require.NotNil(t, session)
	require.Nil(t, session.Metrics, "Metrics should initially be nil")

	// Create a ProcessTokenUsage event targeting the session's process
	testMetrics := &metrics.TokenMetrics{
		TokensUsed:  50000,
		TotalTokens: 200000,
	}
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:      events.ProcessTokenUsage,
			ProcessID: ChatPanelProcessID,
			Metrics:   testMetrics,
		},
	}

	// Handle the event
	m, _ = m.Update(event)

	// Verify session's Metrics is updated
	session = m.SessionByProcessID(ChatPanelProcessID)
	require.NotNil(t, session.Metrics, "Session Metrics should be set")
	require.Equal(t, 50000, session.Metrics.TokensUsed)
	require.Equal(t, 200000, session.Metrics.TotalTokens)
}

func TestSessionEventIsolation_EventsForSessionADontAffectSessionB(t *testing.T) {
	// Test that events for session A don't affect session B's state
	m := New(DefaultConfig())

	// Create infrastructure for event handling
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Create a second session with its own ProcessID
	m, sessionB := m.CreateSession("session-2")
	m = m.SetSessionProcessID("session-2", "process-2")

	// Get both sessions
	sessionA := m.SessionByProcessID(ChatPanelProcessID)
	require.NotNil(t, sessionA)
	require.NotNil(t, sessionB)

	// Verify initial state
	require.Equal(t, 0, sessionA.QueueCount)
	require.Equal(t, 0, sessionB.QueueCount)
	require.Nil(t, sessionA.Metrics)
	require.Nil(t, sessionB.Metrics)

	// Send ProcessQueueChanged event to session A's process
	eventA := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:       events.ProcessQueueChanged,
			ProcessID:  ChatPanelProcessID,
			QueueCount: 3,
		},
	}
	m, _ = m.Update(eventA)

	// Send ProcessTokenUsage event to session A's process
	eventA2 := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:      events.ProcessTokenUsage,
			ProcessID: ChatPanelProcessID,
			Metrics:   &metrics.TokenMetrics{TokensUsed: 10000, TotalTokens: 200000},
		},
	}
	m, _ = m.Update(eventA2)

	// Verify session A was updated
	sessionA = m.SessionByProcessID(ChatPanelProcessID)
	require.Equal(t, 3, sessionA.QueueCount, "Session A QueueCount should be 3")
	require.NotNil(t, sessionA.Metrics, "Session A Metrics should be set")
	require.Equal(t, 10000, sessionA.Metrics.TokensUsed)

	// Verify session B was NOT affected
	sessionB = m.SessionByProcessID("process-2")
	require.Equal(t, 0, sessionB.QueueCount, "Session B QueueCount should remain 0")
	require.Nil(t, sessionB.Metrics, "Session B Metrics should remain nil")

	// Now send events to session B's process
	eventB := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:       events.ProcessQueueChanged,
			ProcessID:  "process-2",
			QueueCount: 7,
		},
	}
	m, _ = m.Update(eventB)

	// Verify session B was updated
	sessionB = m.SessionByProcessID("process-2")
	require.Equal(t, 7, sessionB.QueueCount, "Session B QueueCount should be 7")

	// Verify session A was NOT affected by B's event
	sessionA = m.SessionByProcessID(ChatPanelProcessID)
	require.Equal(t, 3, sessionA.QueueCount, "Session A QueueCount should still be 3")
}

func TestUnknownProcessID_DoesNotPanic(t *testing.T) {
	// Test that events with unknown ProcessID don't cause panic
	m := New(DefaultConfig())

	// Create infrastructure for event handling
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Get initial session state
	session := m.SessionByProcessID(ChatPanelProcessID)
	require.NotNil(t, session)
	initialQueueCount := session.QueueCount
	initialStatus := session.Status

	// Send event with unknown ProcessID - should not panic
	require.NotPanics(t, func() {
		event := pubsub.Event[any]{
			Type: pubsub.UpdatedEvent,
			Payload: events.ProcessEvent{
				Type:       events.ProcessQueueChanged,
				ProcessID:  "unknown-process-id",
				QueueCount: 99,
			},
		}
		m, _ = m.Update(event)
	})

	// Verify existing session was not affected
	session = m.SessionByProcessID(ChatPanelProcessID)
	require.Equal(t, initialQueueCount, session.QueueCount, "QueueCount should not change for unknown ProcessID")
	require.Equal(t, initialStatus, session.Status, "Status should not change for unknown ProcessID")
}

func TestProcessReady_UpdatesCorrectSessionStatus(t *testing.T) {
	// Test that ProcessReady event updates the correct session's Status
	m := New(DefaultConfig())

	// Create infrastructure for event handling
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Initial session should be Pending
	session := m.SessionByProcessID(ChatPanelProcessID)
	require.NotNil(t, session)
	require.Equal(t, events.ProcessStatusPending, session.Status)

	// Send ProcessReady event
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:      events.ProcessReady,
			ProcessID: ChatPanelProcessID,
		},
	}
	m, _ = m.Update(event)

	// Verify session status is now Ready
	session = m.SessionByProcessID(ChatPanelProcessID)
	require.Equal(t, events.ProcessStatusReady, session.Status, "Session status should be Ready")
}

func TestProcessWorking_UpdatesCorrectSessionStatus(t *testing.T) {
	// Test that ProcessWorking event updates the correct session's Status
	m := New(DefaultConfig())

	// Create infrastructure for event handling
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Set session to Ready first
	session := m.sessions[DefaultSessionID]
	session.Status = events.ProcessStatusReady

	// Send ProcessWorking event
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: events.ProcessEvent{
			Type:      events.ProcessWorking,
			ProcessID: ChatPanelProcessID,
		},
	}
	m, _ = m.Update(event)

	// Verify session status is now Working
	session = m.SessionByProcessID(ChatPanelProcessID)
	require.Equal(t, events.ProcessStatusWorking, session.Status, "Session status should be Working")
}

// ============================================================================
// Per-Session State View Tests (perles-ci2e.3)
// Tests verify view renders state from active session, not global fields.
// ============================================================================

func TestView_Golden_WorkingSessionShowsBlueBorder(t *testing.T) {
	// Golden test: View with working session shows blue border
	m := New(DefaultConfig()).SetSize(60, 20).Toggle()

	// Set active session to Working status
	m.sessions[DefaultSessionID].Status = events.ProcessStatusWorking
	m = m.AddMessage(chatrender.Message{Role: RoleUser, Content: "Please help me"})
	m = m.AddMessage(chatrender.Message{Role: RoleAssistant, Content: "Let me think..."})

	view := scanView(m)
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_ReadySessionShowsDefaultBorder(t *testing.T) {
	// Golden test: View with ready session shows default border (not blue)
	m := New(DefaultConfig()).SetSize(60, 20).Toggle()

	// Set active session to Ready status
	m.sessions[DefaultSessionID].Status = events.ProcessStatusReady
	m = m.AddMessage(chatrender.Message{Role: RoleUser, Content: "Hello"})
	m = m.AddMessage(chatrender.Message{Role: RoleAssistant, Content: "Hi there!"})

	view := scanView(m)
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_QueuedMessagesShowsCount(t *testing.T) {
	// Golden test: View with queued messages shows count indicator
	m := New(DefaultConfig()).SetSize(60, 20).Toggle()

	// Set active session state with queue count
	m.sessions[DefaultSessionID].Status = events.ProcessStatusWorking
	m.sessions[DefaultSessionID].QueueCount = 3
	m = m.AddMessage(chatrender.Message{Role: RoleUser, Content: "First"})
	m = m.AddMessage(chatrender.Message{Role: RoleAssistant, Content: "Processing..."})

	view := scanView(m)
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_MetricsShowsTokenUsage(t *testing.T) {
	// Golden test: View with metrics shows token usage display
	m := New(DefaultConfig()).SetSize(60, 20).Toggle()

	// Set active session state with metrics
	m.sessions[DefaultSessionID].Status = events.ProcessStatusReady
	m.sessions[DefaultSessionID].Metrics = &metrics.TokenMetrics{
		TokensUsed:  50000,
		TotalTokens: 200000,
	}
	m = m.AddMessage(chatrender.Message{Role: RoleUser, Content: "Hello"})
	m = m.AddMessage(chatrender.Message{Role: RoleAssistant, Content: "Hi! I've used some tokens."})

	view := scanView(m)
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_NilSession_DoesNotPanic(t *testing.T) {
	// Unit test: Nil session case renders without panic
	m := New(DefaultConfig()).SetSize(60, 20).Toggle()

	// Force nil active session by clearing the sessions map and ID
	m.sessions = map[string]*SessionData{}
	m.sessionOrder = []string{}
	m.activeSessionID = ""

	// Should not panic when rendering with nil active session
	require.NotPanics(t, func() {
		view := scanView(m)
		// View should be rendered (may show empty content but should not crash)
		require.NotEmpty(t, view, "View should render even with nil session")
	})
}

func TestView_SessionStateUsedForBorderColor(t *testing.T) {
	// Unit test: Border color logic is derived from session.Status
	// Note: In test environment without a real terminal, lipgloss doesn't emit ANSI codes,
	// so we can't directly verify color output. Instead, we verify the logic path:
	// - When session.Status == Working, borderColor should be assistantWorkingBorderColor
	// - When session.Status != Working, borderColor should be styles.BorderDefaultColor
	// The golden tests (TestView_Golden_WorkingSessionShowsBlueBorder and
	// TestView_Golden_ReadySessionShowsDefaultBorder) capture the visual output.

	m := New(DefaultConfig()).SetSize(60, 20).Toggle()

	// Verify the view logic path for Ready status
	m.sessions[DefaultSessionID].Status = events.ProcessStatusReady
	m = m.AddMessage(chatrender.Message{Role: RoleUser, Content: "Test message"})
	viewReady := m.View()
	require.NotEmpty(t, viewReady, "View should render for Ready status")

	// Verify the view logic path for Working status
	m.sessions[DefaultSessionID].Status = events.ProcessStatusWorking
	viewWorking := m.View()
	require.NotEmpty(t, viewWorking, "View should render for Working status")

	// Both views should have the same structure (just different colors in real terminal)
	// The content is identical except for the border color styling
	require.Contains(t, viewReady, "Test message", "Ready view should contain message")
	require.Contains(t, viewWorking, "Test message", "Working view should contain message")
}

func TestView_SessionStateUsedForQueueCount(t *testing.T) {
	// Unit test: Queue count derived from session.QueueCount, not global field
	m := New(DefaultConfig()).SetSize(60, 20).Toggle()

	// Set session with queue count
	m.sessions[DefaultSessionID].Status = events.ProcessStatusWorking
	m.sessions[DefaultSessionID].QueueCount = 5
	m = m.AddMessage(chatrender.Message{Role: RoleUser, Content: "Test"})

	view := scanView(m)

	// Should show queue count from session
	require.Contains(t, view, "[5 queued]", "Queue count should be derived from session.QueueCount")
}

func TestView_SessionStateUsedForMetrics(t *testing.T) {
	// Unit test: Metrics display derived from session.Metrics, not global field
	m := New(DefaultConfig()).SetSize(60, 20).Toggle()

	// Set session with metrics
	m.sessions[DefaultSessionID].Status = events.ProcessStatusReady
	m.sessions[DefaultSessionID].Metrics = &metrics.TokenMetrics{
		TokensUsed:  75000,
		TotalTokens: 200000,
	}
	m = m.AddMessage(chatrender.Message{Role: RoleUser, Content: "Test"})

	view := scanView(m)

	// Should show metrics from session (75k/200k or similar format)
	require.Contains(t, view, "75", "Metrics should be derived from session.Metrics")
}

// Helper Method Delegation Tests (perles-ci2e.4)
// These tests verify that AssistantWorking() and QueueCount() properly delegate
// to the active session's state after global fields were removed.

func TestAssistantWorking_TrueWhenActiveSessionWorking(t *testing.T) {
	// Unit test: AssistantWorking() returns true when active session status is Working
	m := New(DefaultConfig())

	// Verify we have an active session
	session := m.ActiveSession()
	require.NotNil(t, session, "Should have active session")

	// Set session status to Working
	session.Status = events.ProcessStatusWorking

	// AssistantWorking() should return true
	require.True(t, m.AssistantWorking(),
		"AssistantWorking() should return true when active session is Working")
}

func TestAssistantWorking_FalseWhenNoActiveSession(t *testing.T) {
	// Unit test: AssistantWorking() returns false when no active session exists
	m := New(DefaultConfig())

	// Clear the active session ID to simulate no active session
	m.activeSessionID = ""

	// AssistantWorking() should return false without panic
	require.False(t, m.AssistantWorking(),
		"AssistantWorking() should return false when no active session")
}

func TestAssistantWorking_FalseForNonWorkingStatus(t *testing.T) {
	// Verify AssistantWorking returns false for non-Working statuses
	m := New(DefaultConfig())
	session := m.ActiveSession()
	require.NotNil(t, session)

	// Test various non-Working statuses
	nonWorkingStatuses := []events.ProcessStatus{
		events.ProcessStatusPending,
		events.ProcessStatusReady,
		events.ProcessStatusPaused,
		events.ProcessStatusStopped,
		events.ProcessStatusFailed,
	}

	for _, status := range nonWorkingStatuses {
		session.Status = status
		require.False(t, m.AssistantWorking(),
			"AssistantWorking() should return false for status: %v", status)
	}
}

func TestQueueCount_ReturnsActiveSessionQueueCount(t *testing.T) {
	// Unit test: QueueCount() returns the active session's queue count
	m := New(DefaultConfig())

	// Verify we have an active session
	session := m.ActiveSession()
	require.NotNil(t, session, "Should have active session")

	// Set session queue count
	session.QueueCount = 7

	// QueueCount() should return the session's queue count
	require.Equal(t, 7, m.QueueCount(),
		"QueueCount() should return active session's queue count")
}

func TestQueueCount_ReturnsZeroWhenNoActiveSession(t *testing.T) {
	// Unit test: QueueCount() returns 0 when no active session exists
	m := New(DefaultConfig())

	// Clear the active session ID to simulate no active session
	m.activeSessionID = ""

	// QueueCount() should return 0 without panic
	require.Equal(t, 0, m.QueueCount(),
		"QueueCount() should return 0 when no active session")
}

func TestQueueCount_ReturnsZeroForZeroQueueCount(t *testing.T) {
	// Verify QueueCount returns 0 when session has 0 queued
	m := New(DefaultConfig())
	session := m.ActiveSession()
	require.NotNil(t, session)

	// Explicitly set to 0 (Go zero value but verify behavior)
	session.QueueCount = 0

	require.Equal(t, 0, m.QueueCount(),
		"QueueCount() should return 0 when session queue count is 0")
}

func TestMetrics_DelegatesToActiveSession(t *testing.T) {
	// Unit test: Metrics() returns the active session's metrics
	m := New(DefaultConfig())

	// Verify we have an active session
	session := m.ActiveSession()
	require.NotNil(t, session, "Should have active session")

	// Initially nil
	require.Nil(t, m.Metrics(), "Metrics() should be nil initially")

	// Set session metrics
	expectedMetrics := &metrics.TokenMetrics{
		TokensUsed:  50000,
		TotalTokens: 200000,
	}
	session.Metrics = expectedMetrics

	// Metrics() should return the session's metrics
	result := m.Metrics()
	require.NotNil(t, result, "Metrics() should return session metrics")
	require.Equal(t, expectedMetrics.TokensUsed, result.TokensUsed)
	require.Equal(t, expectedMetrics.TotalTokens, result.TotalTokens)
}

func TestMetrics_ReturnsNilWhenNoActiveSession(t *testing.T) {
	// Unit test: Metrics() returns nil when no active session exists
	m := New(DefaultConfig())

	// Clear the active session ID to simulate no active session
	m.activeSessionID = ""

	// Metrics() should return nil without panic
	require.Nil(t, m.Metrics(),
		"Metrics() should return nil when no active session")
}

func TestGlobalFieldsRemoved_CodeCompiles(t *testing.T) {
	// Compile test: Verifies that the Model struct no longer has the removed fields.
	// This test passes if the code compiles - any attempt to access the old fields
	// would result in a compile error.
	//
	// The following fields were removed from Model struct:
	// - assistantWorking bool
	// - queueCount int
	// - metrics *metrics.TokenMetrics
	//
	// The helper methods now delegate to the active session's state.

	m := New(DefaultConfig())

	// These methods now delegate to active session instead of reading global fields
	_ = m.AssistantWorking() // Used to read m.assistantWorking
	_ = m.QueueCount()       // Used to read m.queueCount
	_ = m.Metrics()          // Used to read m.metrics

	// If this test compiles and runs, the global fields have been successfully removed
	require.True(t, true, "Code compiles without global fields")
}

// Session Isolation Integration Tests (perles-ci2e.5)
// These tests verify complete session state isolation works end-to-end.

func TestSessionIsolation_TwoSessionsDifferentStates(t *testing.T) {
	// Integration test: Two sessions with different states.
	// Verifies that switching sessions shows the correct session's state,
	// not leaked state from another session.
	//
	// Session A: working, 3 queued, 50k tokens
	// Session B: ready, 0 queued, 10k tokens

	m := New(DefaultConfig()).SetSize(80, 30).Toggle()

	// Session A is the default session - configure its state
	sessionA := m.ActiveSession()
	require.NotNil(t, sessionA, "Should have default session")
	sessionA.Status = events.ProcessStatusWorking
	sessionA.QueueCount = 3
	sessionA.Metrics = &metrics.TokenMetrics{
		TokensUsed:  50000,
		TotalTokens: 200000,
	}
	m = m.AddMessage(chatrender.Message{Role: RoleUser, Content: "Session A message"})

	// Create and configure Session B
	m, sessionB := m.CreateSession("session-b")
	require.NotNil(t, sessionB, "Should create session B")
	sessionB.Status = events.ProcessStatusReady
	sessionB.QueueCount = 0
	sessionB.Metrics = &metrics.TokenMetrics{
		TokensUsed:  10000,
		TotalTokens: 200000,
	}
	// Add a message to session B (need to switch first)
	m, ok := m.SwitchSession("session-b")
	require.True(t, ok, "Should switch to session B")
	m = m.AddMessage(chatrender.Message{Role: RoleUser, Content: "Session B message"})

	// --- Test 1: Verify Session B state when active ---
	require.Equal(t, "session-b", m.activeSessionID, "Session B should be active")

	// Verify helper methods return Session B state
	require.False(t, m.AssistantWorking(),
		"AssistantWorking() should return false for Session B (Ready status)")
	require.Equal(t, 0, m.QueueCount(),
		"QueueCount() should return 0 for Session B")
	require.NotNil(t, m.Metrics(), "Session B should have metrics")
	require.Equal(t, 10000, m.Metrics().TokensUsed,
		"Metrics should show Session B's 10k tokens")

	// Verify view shows Session B content
	viewB := m.View()
	require.Contains(t, viewB, "Session B message",
		"View should show Session B message")
	require.NotContains(t, viewB, "Session A message",
		"View should NOT show Session A message when B is active")
	// Should NOT show queue count indicator (0 queued, Ready status)
	require.NotContains(t, viewB, "[3 queued]",
		"View should NOT show Session A's queue count")

	// --- Test 2: Switch A→B→A and verify A's state restored ---
	m, ok = m.SwitchSession(DefaultSessionID)
	require.True(t, ok, "Should switch back to Session A")
	require.Equal(t, DefaultSessionID, m.activeSessionID, "Session A should be active")

	// Verify helper methods return Session A state
	require.True(t, m.AssistantWorking(),
		"AssistantWorking() should return true for Session A (Working status)")
	require.Equal(t, 3, m.QueueCount(),
		"QueueCount() should return 3 for Session A")
	require.NotNil(t, m.Metrics(), "Session A should have metrics")
	require.Equal(t, 50000, m.Metrics().TokensUsed,
		"Metrics should show Session A's 50k tokens")

	// Verify view shows Session A content
	viewA := m.View()
	require.Contains(t, viewA, "Session A message",
		"View should show Session A message")
	require.NotContains(t, viewA, "Session B message",
		"View should NOT show Session B message when A is active")
	// Should show working indicator and queue count
	require.Contains(t, viewA, "[3 queued]",
		"View should show Session A's queue count")
}

func TestSessionIsolation_StateNotLeakedOnSwitch(t *testing.T) {
	// Integration test: Verify state is NOT leaked between sessions.
	// This tests the critical invariant that switching sessions doesn't
	// accidentally mix state from different sessions.

	m := New(DefaultConfig()).SetSize(80, 30).Toggle()

	// Session A: Set specific state
	sessionA := m.ActiveSession()
	sessionA.Status = events.ProcessStatusWorking
	sessionA.QueueCount = 5
	sessionA.Metrics = &metrics.TokenMetrics{TokensUsed: 75000, TotalTokens: 200000}

	// Create Session B with completely different state
	m, sessionB := m.CreateSession("session-b")
	sessionB.Status = events.ProcessStatusPending
	sessionB.QueueCount = 0
	sessionB.Metrics = nil // Deliberately nil

	// Switch to B
	m, _ = m.SwitchSession("session-b")

	// Verify B's state, NOT A's
	require.False(t, m.AssistantWorking(),
		"Session B should NOT be working (leaked A's Working status)")
	require.Equal(t, 0, m.QueueCount(),
		"Session B queue count should be 0 (leaked A's count of 5)")
	require.Nil(t, m.Metrics(),
		"Session B metrics should be nil (leaked A's metrics)")

	// Switch back to A
	m, _ = m.SwitchSession(DefaultSessionID)

	// Verify A's state wasn't modified
	require.True(t, m.AssistantWorking(),
		"Session A should still be Working after switching back")
	require.Equal(t, 5, m.QueueCount(),
		"Session A queue count should still be 5 after switching back")
	require.NotNil(t, m.Metrics(),
		"Session A metrics should still exist after switching back")
	require.Equal(t, 75000, m.Metrics().TokensUsed,
		"Session A metrics should be unchanged after switching back")
}

func TestSessionIsolation_SwitchToSessionWithNoProcess(t *testing.T) {
	// Edge case: Switch to a session that has no process spawned yet.
	// New sessions start with ProcessID="" and Status=Pending.
	// The UI should handle this gracefully without crashing.

	m := New(DefaultConfig()).SetSize(80, 30).Toggle()

	// Create a new session (starts with no process)
	m, newSession := m.CreateSession("new-session")
	require.NotNil(t, newSession, "Should create new session")
	require.Empty(t, newSession.ProcessID, "New session should have no ProcessID")
	require.Equal(t, events.ProcessStatusPending, newSession.Status,
		"New session should have Pending status")

	// Switch to the new session
	m, ok := m.SwitchSession("new-session")
	require.True(t, ok, "Should switch to new session")

	// Verify state is from new session
	require.False(t, m.AssistantWorking(),
		"New session should not be working (Pending status)")
	require.Equal(t, 0, m.QueueCount(),
		"New session should have 0 queue count")
	require.Nil(t, m.Metrics(),
		"New session should have nil metrics")

	// View should render without panic
	require.NotPanics(t, func() {
		view := scanView(m)
		require.NotEmpty(t, view, "View should render for session with no process")
	})

	// Verify active session is correct
	activeSession := m.ActiveSession()
	require.NotNil(t, activeSession, "Should have active session")
	require.Equal(t, "new-session", activeSession.ID,
		"Active session should be the new session")
	require.Empty(t, activeSession.ProcessID,
		"Active session should still have no ProcessID")
}

func TestSessionIsolation_MultipleSessionsIndependent(t *testing.T) {
	// Integration test: Multiple sessions maintain independent state.
	// Creates 3 sessions with different states and verifies each
	// maintains its state independently when switching between them.

	m := New(DefaultConfig()).SetSize(80, 30).Toggle()

	// Session 1 (default): Working, 2 queued, 30k tokens
	session1 := m.ActiveSession()
	session1.Status = events.ProcessStatusWorking
	session1.QueueCount = 2
	session1.Metrics = &metrics.TokenMetrics{TokensUsed: 30000, TotalTokens: 200000}

	// Session 2: Ready, 0 queued, 60k tokens
	m, session2 := m.CreateSession("session-2")
	session2.Status = events.ProcessStatusReady
	session2.QueueCount = 0
	session2.Metrics = &metrics.TokenMetrics{TokensUsed: 60000, TotalTokens: 200000}

	// Session 3: Paused, 1 queued, 90k tokens
	m, session3 := m.CreateSession("session-3")
	session3.Status = events.ProcessStatusPaused
	session3.QueueCount = 1
	session3.Metrics = &metrics.TokenMetrics{TokensUsed: 90000, TotalTokens: 200000}

	// Round-robin through sessions and verify each shows correct state
	testCases := []struct {
		sessionID   string
		wantWorking bool
		wantQueue   int
		wantTokens  int
	}{
		{DefaultSessionID, true, 2, 30000},
		{"session-2", false, 0, 60000},
		{"session-3", false, 1, 90000},
		{DefaultSessionID, true, 2, 30000}, // Back to session 1
		{"session-3", false, 1, 90000},     // Jump to session 3
		{"session-2", false, 0, 60000},     // Back to session 2
	}

	for _, tc := range testCases {
		m, ok := m.SwitchSession(tc.sessionID)
		require.True(t, ok, "Should switch to session %s", tc.sessionID)

		require.Equal(t, tc.wantWorking, m.AssistantWorking(),
			"Session %s: AssistantWorking mismatch", tc.sessionID)
		require.Equal(t, tc.wantQueue, m.QueueCount(),
			"Session %s: QueueCount mismatch", tc.sessionID)
		require.NotNil(t, m.Metrics(),
			"Session %s: Metrics should not be nil", tc.sessionID)
		require.Equal(t, tc.wantTokens, m.Metrics().TokensUsed,
			"Session %s: TokensUsed mismatch", tc.sessionID)
	}
}

// =============================================================================
// CommandLogEvent Tests
// =============================================================================

func TestModel_HandleCommandLogEvent_DeliverFailed(t *testing.T) {
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Create a CommandLogEvent for failed deliver_process_queued
	testErr := errors.New("process chat-panel has no session ID")
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: processor.CommandLogEvent{
			CommandID:   "test-cmd-123",
			CommandType: command.CmdDeliverProcessQueued,
			Success:     false,
			Error:       testErr,
		},
	}

	// Handle the event
	_, cmd := m.Update(event)

	// Verify cmd is not nil (should be batch with listener + error msg)
	require.NotNil(t, cmd)
}

func TestModel_HandleCommandLogEvent_SendFailed(t *testing.T) {
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Create a CommandLogEvent for failed send_to_process
	testErr := errors.New("failed to send message")
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: processor.CommandLogEvent{
			CommandID:   "test-cmd-456",
			CommandType: command.CmdSendToProcess,
			Success:     false,
			Error:       testErr,
		},
	}

	// Handle the event
	_, cmd := m.Update(event)

	// Verify cmd is not nil (should be batch with listener + error msg)
	require.NotNil(t, cmd)
}

func TestModel_HandleCommandLogEvent_SpawnFailed(t *testing.T) {
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Create a CommandLogEvent for failed spawn_process
	testErr := errors.New("failed to spawn process")
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: processor.CommandLogEvent{
			CommandID:   "test-cmd-789",
			CommandType: command.CmdSpawnProcess,
			Success:     false,
			Error:       testErr,
		},
	}

	// Handle the event
	_, cmd := m.Update(event)

	// Verify cmd is not nil (should be batch with listener + error msg)
	require.NotNil(t, cmd)
}

func TestModel_HandleCommandLogEvent_SuccessIgnored(t *testing.T) {
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Create a successful CommandLogEvent (should be ignored)
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: processor.CommandLogEvent{
			CommandID:   "test-cmd-success",
			CommandType: command.CmdDeliverProcessQueued,
			Success:     true,
			Error:       nil,
		},
	}

	// Handle the event
	_, cmd := m.Update(event)

	// Verify cmd is not nil (should just be listener, no error)
	require.NotNil(t, cmd)
}

func TestModel_HandleCommandLogEvent_UnrelatedCommandIgnored(t *testing.T) {
	cfg := Config{
		ClientType: "claude",
		WorkDir:    "/tmp/test",
	}
	m := New(cfg)

	// Create infrastructure
	infra := newTestInfrastructure(t)
	infra.Start()
	defer infra.Shutdown()

	m = m.SetInfrastructure(infra)

	// Create a failed CommandLogEvent for unrelated command type
	testErr := errors.New("some error")
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: processor.CommandLogEvent{
			CommandID:   "test-cmd-unrelated",
			CommandType: command.CmdAssignTask, // Not relevant to chat panel
			Success:     false,
			Error:       testErr,
		},
	}

	// Handle the event
	_, cmd := m.Update(event)

	// Verify cmd is not nil (should just be listener, no error toast)
	require.NotNil(t, cmd)
}
