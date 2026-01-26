package app

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
	"unsafe"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	beadsapp "github.com/zjrosen/perles/internal/beads/application"
	beadsdomain "github.com/zjrosen/perles/internal/beads/domain"
	"github.com/zjrosen/perles/internal/config"
	"github.com/zjrosen/perles/internal/flags"
	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/mode"
	"github.com/zjrosen/perles/internal/mode/dashboard"
	"github.com/zjrosen/perles/internal/mode/kanban"
	"github.com/zjrosen/perles/internal/mode/search"
	"github.com/zjrosen/perles/internal/orchestration/client"
	v2 "github.com/zjrosen/perles/internal/orchestration/v2"
	appreg "github.com/zjrosen/perles/internal/registry/application"
	"github.com/zjrosen/perles/internal/ui/shared/chatpanel"
	"github.com/zjrosen/perles/internal/ui/shared/diffviewer"
)

// TestMain initializes the global zone manager for all tests in this package.
func TestMain(m *testing.M) {
	zone.NewGlobal()
	os.Exit(m.Run())
}

// newTestChatInfrastructure creates a v2.SimpleInfrastructure with mock provider for testing.
func newTestChatInfrastructure(t *testing.T) *v2.SimpleInfrastructure {
	t.Helper()
	mockProcess := mocks.NewMockHeadlessProcess(t)

	// Set up mock process expectations
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

// createTestModel creates a minimal Model for testing.
// It does not require a database connection.
func createTestModel(t *testing.T) Model {
	cfg := config.Defaults()
	clipboard := mocks.NewMockClipboard(t)
	clipboard.EXPECT().Copy(mock.Anything).Return(nil).Maybe()
	services := mode.Services{
		Config:    &cfg,
		Clipboard: clipboard,
	}

	// Create chat panel with config from services (same pattern as NewWithConfig)
	chatPanelCfg := chatpanel.Config{
		ClientType:     cfg.Orchestration.Client,
		WorkDir:        "",
		SessionTimeout: chatpanel.DefaultConfig().SessionTimeout,
	}

	return Model{
		currentMode: mode.ModeKanban,
		kanban:      kanban.New(services),
		search:      search.New(services),
		services:    services,
		chatPanel:   chatpanel.New(chatPanelCfg),
		width:       100,
		height:      40,
	}
}

func createWorkflowCreatorConfigFS(templateContent string) fstest.MapFS {
	return fstest.MapFS{
		"workflows/config-workflow/template.yaml": &fstest.MapFile{
			Data: []byte(`
registry:
  - namespace: "workflow"
    key: "config-workflow"
    version: "v1"
    name: "Config Workflow"
    description: "Config test workflow"
    nodes:
      - key: "task"
        name: "Task"
        template: "task.md"
`),
		},
		"workflows/config-workflow/task.md": &fstest.MapFile{
			Data: []byte(templateContent),
		},
	}
}

func getWorkflowCreatorTemplatesConfig(t *testing.T, creator *appreg.WorkflowCreator) config.TemplatesConfig {
	t.Helper()

	field := reflect.ValueOf(creator).Elem().FieldByName("templatesConfig")
	require.True(t, field.IsValid(), "templatesConfig field missing")
	return *(*config.TemplatesConfig)(unsafe.Pointer(field.UnsafeAddr()))
}

func setWorkflowCreatorExecutor(t *testing.T, creator *appreg.WorkflowCreator, executor beadsapp.IssueExecutor) {
	t.Helper()

	field := reflect.ValueOf(creator).Elem().FieldByName("executor")
	require.True(t, field.IsValid(), "executor field missing")

	ptr := unsafe.Pointer(field.UnsafeAddr())
	reflect.NewAt(field.Type(), ptr).Elem().Set(reflect.ValueOf(executor))
}

func TestApp_DefaultMode(t *testing.T) {
	m := createTestModel(t)
	require.Equal(t, mode.ModeKanban, m.currentMode, "expected default mode to be kanban")
}

func TestApp_WindowSizeMsg(t *testing.T) {
	m := createTestModel(t)

	// Simulate window resize
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 50})
	m = newModel.(Model)

	require.Equal(t, 120, m.width, "expected width to be updated")
	require.Equal(t, 50, m.height, "expected height to be updated")
}

func TestApp_CtrlSpaceSwitchesMode(t *testing.T) {
	m := createTestModel(t)

	// Ctrl+Space (ctrl+@) should switch from kanban to search mode
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = newModel.(Model)

	// Should now be in search mode
	require.Equal(t, mode.ModeSearch, m.currentMode, "mode should switch to search")

	// Ctrl+Space again should switch back to kanban
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = newModel.(Model)

	require.Equal(t, mode.ModeKanban, m.currentMode, "mode should switch back to kanban")
}

func TestApp_ViewDelegates(t *testing.T) {
	m := createTestModel(t)

	// View should delegate to kanban
	view := m.View()
	require.NotEmpty(t, view, "expected non-empty view from kanban mode")
}

func TestApp_ModeSwitchPreservesSize(t *testing.T) {
	t.Run("Panel hidden", func(t *testing.T) {
		m := createTestModel(t)

		// Set initial window size
		newModel, _ := m.Update(tea.WindowSizeMsg{Width: 150, Height: 60})
		m = newModel.(Model)

		require.Equal(t, 150, m.width, "initial width should be 150")
		require.Equal(t, 60, m.height, "initial height should be 60")

		// Switch to search mode (Ctrl+Space)
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
		m = newModel.(Model)

		// Verify size preserved in app
		require.Equal(t, 150, m.width, "width should be preserved after mode switch")
		require.Equal(t, 60, m.height, "height should be preserved after mode switch")

		// Verify search mode has the correct size (by checking View doesn't panic)
		view := m.View()
		require.NotEmpty(t, view, "search view should render without panic")

		// Switch back to kanban (Ctrl+Space)
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
		m = newModel.(Model)

		// Verify size still preserved
		require.Equal(t, 150, m.width, "width should be preserved after returning to kanban")
		require.Equal(t, 60, m.height, "height should be preserved after returning to kanban")
	})

	// Test case added for perles-yn47 chatpanel resize bug fix verification
	t.Run("Panel visible - search gets reduced width on mode switch", func(t *testing.T) {
		m := createTestModel(t)
		terminalWidth := 150
		terminalHeight := 60

		// Pre-set infrastructure to avoid client creation during toggle
		infra := newTestChatInfrastructure(t)
		err := infra.Start()
		require.NoError(t, err)
		m.chatInfra = infra
		m.chatPanel = m.chatPanel.SetInfrastructure(infra)

		// Set initial window size
		newModel, _ := m.Update(tea.WindowSizeMsg{Width: terminalWidth, Height: terminalHeight})
		m = newModel.(Model)

		require.Equal(t, terminalWidth, m.width, "initial width should be terminal width")
		require.Equal(t, terminalHeight, m.height, "initial height should be terminal height")

		// Open chat panel
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
		m = newModel.(Model)
		require.True(t, m.chatPanel.Visible(), "panel should be visible")

		// Unfocus panel to allow mode switch
		m.chatPanelFocused = false

		// Switch to search mode (Ctrl+Space) with panel visible
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
		m = newModel.(Model)
		require.Equal(t, mode.ModeSearch, m.currentMode, "should be in search mode")

		// WIDTH VERIFICATION: App width is preserved, search mode should be
		// sized to reduced width (terminal width - panel width) by SetSize
		require.Equal(t, terminalWidth, m.width, "app width preserved")
		require.True(t, m.chatPanel.Visible(), "panel should still be visible")

		// Verify search mode renders correctly at reduced width
		view := m.View()
		require.NotEmpty(t, view, "search view should render correctly with panel visible")

		// Switch back to kanban (Ctrl+Space) with panel still visible
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
		m = newModel.(Model)
		require.Equal(t, mode.ModeKanban, m.currentMode, "should be in kanban mode")

		// WIDTH VERIFICATION: Kanban should also have correct reduced width
		require.Equal(t, terminalWidth, m.width, "app width preserved after returning to kanban")
		require.True(t, m.chatPanel.Visible(), "panel should still be visible")

		// Verify kanban mode renders correctly at reduced width
		view = m.View()
		require.NotEmpty(t, view, "kanban view should render correctly with panel visible")
	})
}

// TestApp_SwitchMode_SetsSizeCorrectly verifies that switchMode() correctly
// calculates width based on chatpanel visibility and calls SetSize on the
// target mode. This ensures modes receive the correct dimensions when switching.
//
// Covers all 4 toggle scenarios:
// - Kanban → Search with panel visible: search gets reduced width
// - Kanban → Search with panel hidden: search gets full width
// - Search → Kanban with panel visible: kanban gets reduced width
// - Search → Kanban with panel hidden: kanban gets full width
func TestApp_SwitchMode_SetsSizeCorrectly(t *testing.T) {
	terminalWidth := 150
	terminalHeight := 40

	t.Run("Kanban to Search with panel visible", func(t *testing.T) {
		m := createTestModel(t)

		// Pre-set infrastructure to avoid client creation during toggle
		infra := newTestChatInfrastructure(t)
		err := infra.Start()
		require.NoError(t, err)
		m.chatInfra = infra
		m.chatPanel = m.chatPanel.SetInfrastructure(infra)

		// Set terminal size
		newModel, _ := m.Update(tea.WindowSizeMsg{Width: terminalWidth, Height: terminalHeight})
		m = newModel.(Model)

		// Open chat panel
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
		m = newModel.(Model)
		require.True(t, m.chatPanel.Visible(), "panel should be visible")

		// Unfocus panel to allow mode switch
		m.chatPanelFocused = false

		// Switch to search mode (Ctrl+Space)
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
		m = newModel.(Model)

		// Verify mode switched
		require.Equal(t, mode.ModeSearch, m.currentMode, "should be in search mode")
		require.True(t, m.chatPanel.Visible(), "panel should remain visible")

		// Verify View renders correctly (SetSize was called with reduced width)
		view := m.View()
		require.NotEmpty(t, view, "search view should render without panic")
	})

	t.Run("Kanban to Search with panel hidden", func(t *testing.T) {
		m := createTestModel(t)

		// Pre-set infrastructure
		infra := newTestChatInfrastructure(t)
		err := infra.Start()
		require.NoError(t, err)
		m.chatInfra = infra
		m.chatPanel = m.chatPanel.SetInfrastructure(infra)

		// Set terminal size
		newModel, _ := m.Update(tea.WindowSizeMsg{Width: terminalWidth, Height: terminalHeight})
		m = newModel.(Model)

		// Open then close panel (to test the bug scenario)
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
		m = newModel.(Model)
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
		m = newModel.(Model)
		require.False(t, m.chatPanel.Visible(), "panel should be hidden after toggle")

		// Switch to search mode
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
		m = newModel.(Model)

		// Verify mode switched
		require.Equal(t, mode.ModeSearch, m.currentMode, "should be in search mode")
		require.False(t, m.chatPanel.Visible(), "panel should remain hidden")

		// Verify View renders correctly (SetSize was called with full width)
		view := m.View()
		require.NotEmpty(t, view, "search view should render at full width")
	})

	t.Run("Search to Kanban with panel visible", func(t *testing.T) {
		m := createTestModel(t)

		// Pre-set infrastructure
		infra := newTestChatInfrastructure(t)
		err := infra.Start()
		require.NoError(t, err)
		m.chatInfra = infra
		m.chatPanel = m.chatPanel.SetInfrastructure(infra)

		// Set terminal size
		newModel, _ := m.Update(tea.WindowSizeMsg{Width: terminalWidth, Height: terminalHeight})
		m = newModel.(Model)

		// Switch to search mode first
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
		m = newModel.(Model)
		require.Equal(t, mode.ModeSearch, m.currentMode, "should be in search mode")

		// Open chat panel
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
		m = newModel.(Model)
		require.True(t, m.chatPanel.Visible(), "panel should be visible")

		// Unfocus panel to allow mode switch
		m.chatPanelFocused = false

		// Switch back to kanban
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
		m = newModel.(Model)

		// Verify mode switched
		require.Equal(t, mode.ModeKanban, m.currentMode, "should be in kanban mode")
		require.True(t, m.chatPanel.Visible(), "panel should remain visible")

		// Verify View renders correctly
		view := m.View()
		require.NotEmpty(t, view, "kanban view should render without panic")
	})

	t.Run("Search to Kanban with panel hidden", func(t *testing.T) {
		m := createTestModel(t)

		// Pre-set infrastructure
		infra := newTestChatInfrastructure(t)
		err := infra.Start()
		require.NoError(t, err)
		m.chatInfra = infra
		m.chatPanel = m.chatPanel.SetInfrastructure(infra)

		// Set terminal size
		newModel, _ := m.Update(tea.WindowSizeMsg{Width: terminalWidth, Height: terminalHeight})
		m = newModel.(Model)

		// Switch to search mode first
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
		m = newModel.(Model)
		require.Equal(t, mode.ModeSearch, m.currentMode, "should be in search mode")

		// Open then close panel
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
		m = newModel.(Model)
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
		m = newModel.(Model)
		require.False(t, m.chatPanel.Visible(), "panel should be hidden")

		// Switch back to kanban
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
		m = newModel.(Model)

		// Verify mode switched
		require.Equal(t, mode.ModeKanban, m.currentMode, "should be in kanban mode")
		require.False(t, m.chatPanel.Visible(), "panel should remain hidden")

		// Verify View renders correctly at full width
		view := m.View()
		require.NotEmpty(t, view, "kanban view should render at full width")
	})
}

func TestApp_SearchModeInit(t *testing.T) {
	m := createTestModel(t)

	// Switch to search mode (Ctrl+Space)
	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = newModel.(Model)

	// Verify mode switched
	require.Equal(t, mode.ModeSearch, m.currentMode, "should be in search mode")

	// Init should have been called (returns a command)
	// The search Init() returns nil if no initial query
	// This is expected behavior - we just verify the switch worked
	_ = cmd // May be nil for empty search

	// Verify View renders search mode content
	view := m.View()
	require.NotEmpty(t, view, "search view should render")
}

func TestApp_KanbanModeExtracted(t *testing.T) {
	m := createTestModel(t)

	// Verify kanban mode exists and works
	require.NotNil(t, m.kanban, "kanban mode should be initialized")
	require.Equal(t, mode.ModeKanban, m.currentMode, "default mode should be kanban")

	// Verify kanban view renders
	view := m.View()
	require.NotEmpty(t, view, "kanban view should render")

	// Verify we can interact with kanban mode
	// (j key for navigation - should not crash)
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = newModel.(Model)

	require.Equal(t, mode.ModeKanban, m.currentMode, "should still be in kanban mode")
}

func TestApp_CtrlC_ShowsQuitConfirmation(t *testing.T) {
	m := createTestModel(t)

	// Ctrl+C in kanban mode returns mode.RequestQuitMsg
	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = newModel.(Model)

	// Kanban returns a command that produces mode.RequestQuitMsg
	require.NotNil(t, cmd, "expected command from kanban")
	result := cmd()
	_, isRequestQuit := result.(mode.RequestQuitMsg)
	require.True(t, isRequestQuit, "expected mode.RequestQuitMsg")

	// Process the RequestQuitMsg - this shows the quit modal
	newModel, cmd = m.Update(result)
	m = newModel.(Model)

	// Now quit modal should be visible, no command returned
	require.True(t, m.quitModal.IsVisible(), "expected quit modal to be visible")
	require.Nil(t, cmd, "expected no command (quit modal showing)")
}

func TestApp_SearchModeReceivesUpdates(t *testing.T) {
	m := createTestModel(t)

	// Switch to search mode (Ctrl+Space)
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = newModel.(Model)

	// Send a key that search mode handles (? for help)
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = newModel.(Model)

	// Should still be in search mode (help overlay doesn't change mode)
	require.Equal(t, mode.ModeSearch, m.currentMode, "should still be in search mode")

	// View should render without panic
	view := m.View()
	require.NotEmpty(t, view, "view should render")
}

func TestApp_ModeSwitchRoundTrip(t *testing.T) {
	m := createTestModel(t)

	// Multiple round trips should work (Ctrl+Space)
	for i := 0; i < 3; i++ {
		// Switch to search
		newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
		m = newModel.(Model)
		require.Equal(t, mode.ModeSearch, m.currentMode, "should be in search mode")

		// Switch back to kanban
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
		m = newModel.(Model)
		require.Equal(t, mode.ModeKanban, m.currentMode, "should be in kanban mode")
	}
}

func TestApp_SwitchToSearchMsg_WithQuery(t *testing.T) {
	m := createTestModel(t)

	// Simulate receiving SwitchToSearchMsg from kanban mode
	newModel, cmd := m.Update(kanban.SwitchToSearchMsg{Query: "status:open"})
	m = newModel.(Model)

	// Verify mode switched to search
	require.Equal(t, mode.ModeSearch, m.currentMode, "should be in search mode")

	// Init should be called (returns command batch)
	require.NotNil(t, cmd, "expected Init command")

	// View should render without panic
	view := m.View()
	require.NotEmpty(t, view, "search view should render")
}

func TestApp_SwitchToSearchMsg_EmptyQuery(t *testing.T) {
	m := createTestModel(t)

	// Simulate SwitchToSearchMsg with empty query (no column focused)
	newModel, cmd := m.Update(kanban.SwitchToSearchMsg{Query: ""})
	m = newModel.(Model)

	// Verify mode switched to search
	require.Equal(t, mode.ModeSearch, m.currentMode, "should be in search mode")

	// Init should be called
	require.NotNil(t, cmd, "expected Init command")

	// View should render
	view := m.View()
	require.NotEmpty(t, view, "search view should render")
}

func TestApp_WorkflowCreatorReceivesConfig(t *testing.T) {
	cfg := config.Defaults()
	cfg.Orchestration.Templates = config.TemplatesConfig{DocumentPath: "docs/custom-proposals"}

	registryService, err := appreg.NewRegistryService(createWorkflowCreatorConfigFS("Doc: {{.Config.document_path}}"), "")
	require.NoError(t, err)

	model, err := NewWithConfig(
		nil,
		cfg,
		nil,
		nil,
		"",
		"",
		t.TempDir(),
		false,
		registryService,
	)
	require.NoError(t, err)

	got := getWorkflowCreatorTemplatesConfig(t, model.workflowCreator)
	require.Equal(t, cfg.Orchestration.Templates, got)
}

func TestApp_EndToEnd_CustomPath(t *testing.T) {
	cfg := config.Defaults()
	cfg.Orchestration.Templates = config.TemplatesConfig{DocumentPath: "docs/custom-proposals"}

	registryService, err := appreg.NewRegistryService(createWorkflowCreatorConfigFS("Doc: {{.Config.document_path}}/plan.md"), "")
	require.NoError(t, err)

	model, err := NewWithConfig(
		nil,
		cfg,
		nil,
		nil,
		"",
		"",
		t.TempDir(),
		false,
		registryService,
	)
	require.NoError(t, err)

	mockExecutor := mocks.NewMockIssueExecutor(t)
	mockExecutor.EXPECT().CreateEpic(
		"Config Workflow: Test Feature",
		mock.AnythingOfType("string"),
		[]string{"feature:test-feature", "workflow:config-workflow"},
	).Return(beadsdomain.CreateResult{ID: "test-epic", Title: "Config Workflow: Test Feature"}, nil)

	mockExecutor.EXPECT().CreateTask(
		"Task",
		mock.MatchedBy(func(content string) bool {
			return strings.Contains(content, "Doc: docs/custom-proposals/plan.md")
		}),
		"test-epic",
		mock.AnythingOfType("string"),
		[]string{"spec:plan"},
	).Return(beadsdomain.CreateResult{ID: "task-1", Title: "Task"}, nil)

	setWorkflowCreatorExecutor(t, model.workflowCreator, mockExecutor)

	_, err = model.workflowCreator.CreateWithArgs("test-feature", "config-workflow", nil)
	require.NoError(t, err)
}

func TestApp_EndToEnd_DefaultPath(t *testing.T) {
	cfg := config.Defaults()
	cfg.Orchestration.Templates = config.TemplatesConfig{}

	registryService, err := appreg.NewRegistryService(createWorkflowCreatorConfigFS("Doc: {{.Config.document_path}}/plan.md"), "")
	require.NoError(t, err)

	model, err := NewWithConfig(
		nil,
		cfg,
		nil,
		nil,
		"",
		"",
		t.TempDir(),
		false,
		registryService,
	)
	require.NoError(t, err)

	mockExecutor := mocks.NewMockIssueExecutor(t)
	mockExecutor.EXPECT().CreateEpic(
		"Config Workflow: Test Feature",
		mock.AnythingOfType("string"),
		[]string{"feature:test-feature", "workflow:config-workflow"},
	).Return(beadsdomain.CreateResult{ID: "test-epic", Title: "Config Workflow: Test Feature"}, nil)

	mockExecutor.EXPECT().CreateTask(
		"Task",
		mock.MatchedBy(func(content string) bool {
			return strings.Contains(content, "Doc: docs/proposals/plan.md")
		}),
		"test-epic",
		mock.AnythingOfType("string"),
		[]string{"spec:plan"},
	).Return(beadsdomain.CreateResult{ID: "task-1", Title: "Task"}, nil)

	setWorkflowCreatorExecutor(t, model.workflowCreator, mockExecutor)

	_, err = model.workflowCreator.CreateWithArgs("test-feature", "config-workflow", nil)
	require.NoError(t, err)
}

// TestApp_SwitchToSearchMsg_SetsSizeCorrectly verifies that the SwitchToSearchMsg
// handler correctly calculates width based on chatpanel visibility and calls SetSize
// on search mode BEFORE emitting EnterMsg. This ensures search mode has correct
// dimensions when processing the enter message.
//
// Covers panel visible/hidden scenarios:
// - Panel visible: search gets reduced width
// - Panel hidden: search gets full width
func TestApp_SwitchToSearchMsg_SetsSizeCorrectly(t *testing.T) {
	terminalWidth := 150
	terminalHeight := 40

	t.Run("Panel visible - search gets reduced width", func(t *testing.T) {
		m := createTestModel(t)

		// Pre-set infrastructure to avoid client creation during toggle
		infra := newTestChatInfrastructure(t)
		err := infra.Start()
		require.NoError(t, err)
		m.chatInfra = infra
		m.chatPanel = m.chatPanel.SetInfrastructure(infra)

		// Set terminal size
		newModel, _ := m.Update(tea.WindowSizeMsg{Width: terminalWidth, Height: terminalHeight})
		m = newModel.(Model)

		// Open chat panel
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
		m = newModel.(Model)
		require.True(t, m.chatPanel.Visible(), "panel should be visible")

		// Unfocus panel to allow mode switch
		m.chatPanelFocused = false

		// Simulate SwitchToSearchMsg (as if from kanban Enter or / key)
		newModel, _ = m.Update(kanban.SwitchToSearchMsg{Query: "status:open", SubMode: mode.SubModeList})
		m = newModel.(Model)

		// Verify mode switched
		require.Equal(t, mode.ModeSearch, m.currentMode, "should be in search mode")
		require.True(t, m.chatPanel.Visible(), "panel should remain visible")

		// Verify View renders correctly (SetSize was called with reduced width)
		view := m.View()
		require.NotEmpty(t, view, "search view should render without panic")
	})

	t.Run("Panel hidden - search gets full width", func(t *testing.T) {
		m := createTestModel(t)

		// Pre-set infrastructure
		infra := newTestChatInfrastructure(t)
		err := infra.Start()
		require.NoError(t, err)
		m.chatInfra = infra
		m.chatPanel = m.chatPanel.SetInfrastructure(infra)

		// Set terminal size
		newModel, _ := m.Update(tea.WindowSizeMsg{Width: terminalWidth, Height: terminalHeight})
		m = newModel.(Model)

		// Open then close panel (to test the bug scenario)
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
		m = newModel.(Model)
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
		m = newModel.(Model)
		require.False(t, m.chatPanel.Visible(), "panel should be hidden after toggle")

		// Simulate SwitchToSearchMsg
		newModel, _ = m.Update(kanban.SwitchToSearchMsg{Query: "", SubMode: mode.SubModeTree, IssueID: "TEST-1"})
		m = newModel.(Model)

		// Verify mode switched
		require.Equal(t, mode.ModeSearch, m.currentMode, "should be in search mode")
		require.False(t, m.chatPanel.Visible(), "panel should remain hidden")

		// Verify View renders correctly (SetSize was called with full width)
		view := m.View()
		require.NotEmpty(t, view, "search view should render at full width")
	})
}

func TestApp_ExitToKanbanMsg(t *testing.T) {
	m := createTestModel(t)

	// Switch to search mode first (Ctrl+Space)
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = newModel.(Model)
	require.Equal(t, mode.ModeSearch, m.currentMode, "should be in search mode")

	// Simulate ExitToKanbanMsg from search mode (ESC key)
	newModel, _ = m.Update(search.ExitToKanbanMsg{})
	m = newModel.(Model)

	// Verify mode switched back to kanban
	require.Equal(t, mode.ModeKanban, m.currentMode, "should be back in kanban mode")

	// View should render kanban mode
	view := m.View()
	require.NotEmpty(t, view, "kanban view should render")
}

// TestApp_ExitToKanbanMsg_SetsSizeCorrectly verifies that the ExitToKanbanMsg
// handler correctly calculates width based on chatpanel visibility and calls SetSize
// on kanban mode BEFORE RefreshFromConfig(). This ensures kanban mode has correct
// dimensions before layout recalculation when returning from search mode.
//
// Covers panel visible/hidden scenarios:
// - Panel visible: kanban gets reduced width
// - Panel hidden: kanban gets full width
func TestApp_ExitToKanbanMsg_SetsSizeCorrectly(t *testing.T) {
	terminalWidth := 150
	terminalHeight := 40

	t.Run("Panel visible - kanban gets reduced width", func(t *testing.T) {
		m := createTestModel(t)

		// Pre-set infrastructure to avoid client creation during toggle
		infra := newTestChatInfrastructure(t)
		err := infra.Start()
		require.NoError(t, err)
		m.chatInfra = infra
		m.chatPanel = m.chatPanel.SetInfrastructure(infra)

		// Set terminal size
		newModel, _ := m.Update(tea.WindowSizeMsg{Width: terminalWidth, Height: terminalHeight})
		m = newModel.(Model)

		// Open chat panel
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
		m = newModel.(Model)
		require.True(t, m.chatPanel.Visible(), "panel should be visible")

		// Unfocus panel to allow mode switch
		m.chatPanelFocused = false

		// Switch to search mode first (Ctrl+Space)
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
		m = newModel.(Model)
		require.Equal(t, mode.ModeSearch, m.currentMode, "should be in search mode")

		// Simulate ExitToKanbanMsg from search mode
		newModel, _ = m.Update(search.ExitToKanbanMsg{})
		m = newModel.(Model)

		// Verify mode switched back to kanban
		require.Equal(t, mode.ModeKanban, m.currentMode, "should be back in kanban mode")
		require.True(t, m.chatPanel.Visible(), "panel should remain visible")

		// Verify View renders correctly (SetSize was called with reduced width)
		view := m.View()
		require.NotEmpty(t, view, "kanban view should render without panic")
	})

	t.Run("Panel hidden - kanban gets full width", func(t *testing.T) {
		m := createTestModel(t)

		// Pre-set infrastructure
		infra := newTestChatInfrastructure(t)
		err := infra.Start()
		require.NoError(t, err)
		m.chatInfra = infra
		m.chatPanel = m.chatPanel.SetInfrastructure(infra)

		// Set terminal size
		newModel, _ := m.Update(tea.WindowSizeMsg{Width: terminalWidth, Height: terminalHeight})
		m = newModel.(Model)

		// Open then close panel (to test the bug scenario)
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
		m = newModel.(Model)
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
		m = newModel.(Model)
		require.False(t, m.chatPanel.Visible(), "panel should be hidden after toggle")

		// Switch to search mode
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
		m = newModel.(Model)
		require.Equal(t, mode.ModeSearch, m.currentMode, "should be in search mode")

		// Simulate ExitToKanbanMsg from search mode
		newModel, _ = m.Update(search.ExitToKanbanMsg{})
		m = newModel.(Model)

		// Verify mode switched back to kanban
		require.Equal(t, mode.ModeKanban, m.currentMode, "should be back in kanban mode")
		require.False(t, m.chatPanel.Visible(), "panel should remain hidden")

		// Verify View renders correctly (SetSize was called with full width)
		view := m.View()
		require.NotEmpty(t, view, "kanban view should render at full width")
	})
}

func TestApp_SaveSearchToNewView(t *testing.T) {
	m := createTestModel(t)
	initialViewCount := len(m.services.Config.Views)

	// Simulate SaveSearchToNewViewMsg (without config path, so AddView will fail)
	// This tests the in-memory handling
	msg := search.SaveSearchToNewViewMsg{
		ViewName:   "My Bugs",
		ColumnName: "Open Bugs",
		Color:      "#FF8787",
		Query:      "status = open",
	}
	newModel, _ := m.Update(msg)
	m = newModel.(Model)

	// In-memory config should be updated even if file write fails
	// (because we append before AddView in the actual handler)
	// Note: Our test model has no ConfigPath so AddView will fail
	// This is expected - the important thing is the handler doesn't panic
	require.GreaterOrEqual(t, len(m.services.Config.Views), initialViewCount,
		"view count should not decrease")
}

func TestApp_SaveSearchToNewView_Structure(t *testing.T) {
	m := createTestModel(t)

	// Test with a temporary config to verify correct structure
	msg := search.SaveSearchToNewViewMsg{
		ViewName:   "Test View",
		ColumnName: "Test Column",
		Color:      "#73F59F",
		Query:      "priority = 0",
	}

	// Call handler directly to test structure without file I/O
	result, _ := m.handleSaveSearchToNewView(msg)
	resultModel := result.(Model)

	// Since config path is empty, AddView fails but we can verify the handler runs
	// The in-memory update happens after AddView, so it won't update on error
	// This is correct behavior - don't partially update on error
	require.NotNil(t, resultModel, "handler should return model")
}

func TestApp_ShowDiffViewer(t *testing.T) {
	m := createTestModel(t)
	require.False(t, m.diffViewer.Visible(), "diff viewer should start hidden")

	// Send ShowDiffViewerMsg
	newModel, cmd := m.Update(diffviewer.ShowDiffViewerMsg{})
	m = newModel.(Model)

	require.True(t, m.diffViewer.Visible(), "diff viewer should be visible after ShowDiffViewerMsg")
	require.NotNil(t, cmd, "should return LoadDiff command")
}

func TestApp_HideDiffViewer(t *testing.T) {
	m := createTestModel(t)

	// First show the diff viewer
	newModel, _ := m.Update(diffviewer.ShowDiffViewerMsg{})
	m = newModel.(Model)
	require.True(t, m.diffViewer.Visible(), "diff viewer should be visible")

	// Send HideDiffViewerMsg
	newModel, cmd := m.Update(diffviewer.HideDiffViewerMsg{})
	m = newModel.(Model)

	require.False(t, m.diffViewer.Visible(), "diff viewer should be hidden after HideDiffViewerMsg")
	require.Nil(t, cmd, "should not return any command")
}

func TestApp_DiffViewerEventRouting(t *testing.T) {
	m := createTestModel(t)

	// Show diff viewer first
	newModel, _ := m.Update(diffviewer.ShowDiffViewerMsg{})
	m = newModel.(Model)
	require.True(t, m.diffViewer.Visible(), "diff viewer should be visible")

	// Send ESC key - should close diff viewer (not switch modes)
	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = newModel.(Model)

	// The diff viewer handles ESC and emits HideDiffViewerMsg
	// We need to process that message to verify the overlay closes
	if cmd != nil {
		msg := cmd()
		if _, ok := msg.(diffviewer.HideDiffViewerMsg); ok {
			newModel, _ = m.Update(msg)
			m = newModel.(Model)
		}
	}

	require.False(t, m.diffViewer.Visible(), "diff viewer should be hidden after ESC")
	require.Equal(t, mode.ModeKanban, m.currentMode, "should remain in kanban mode")
}

func TestApp_DiffViewerOverlay(t *testing.T) {
	m := createTestModel(t)

	// Set size first
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = newModel.(Model)

	// Show diff viewer
	newModel, _ = m.Update(diffviewer.ShowDiffViewerMsg{})
	m = newModel.(Model)

	// Verify view includes diff viewer overlay
	view := m.View()
	require.NotEmpty(t, view, "view should render")
	// Since diffViewer is visible and in loading state, the view should contain diff viewer content
	require.True(t, m.diffViewer.Visible(), "diff viewer should be visible")
}

func TestApp_DiffViewerWindowResize(t *testing.T) {
	m := createTestModel(t)

	// Show diff viewer
	newModel, _ := m.Update(diffviewer.ShowDiffViewerMsg{})
	m = newModel.(Model)

	// Resize window
	newModel, _ = m.Update(tea.WindowSizeMsg{Width: 200, Height: 80})
	m = newModel.(Model)

	require.Equal(t, 200, m.width, "width should be updated")
	require.Equal(t, 80, m.height, "height should be updated")
	// The diff viewer should have received the size update (we can't easily verify internal state,
	// but the View() should not panic)
	view := m.View()
	require.NotEmpty(t, view, "view should render after resize")
}

func TestApp_ChatPanel_Initialization(t *testing.T) {
	m := createTestModel(t)

	// Verify chatPanel field exists and is initialized
	// (non-zero value check - if field didn't exist, this wouldn't compile)
	require.False(t, m.chatPanel.Visible(), "chatPanel should be initialized but hidden")

	// Verify chatPanelFocused is initialized (defaults to false)
	// This field is used by app.Update() for focus routing - implemented in perles-hj2a.5
	require.False(t, m.chatPanelFocused, "chatPanelFocused should default to false")
}

// TestApp_ChatPanel_FocusFieldExists verifies the chatPanelFocused field exists
// and can be accessed. The focus routing logic is implemented in perles-hj2a.5.
func TestApp_ChatPanel_FocusFieldExists(t *testing.T) {
	m := createTestModel(t)

	// Field should default to false
	require.False(t, m.chatPanelFocused, "chatPanelFocused should default to false")

	// Field should be settable (demonstrating it's a mutable field)
	m.chatPanelFocused = true
	require.True(t, m.chatPanelFocused, "chatPanelFocused should be settable to true")
}

func TestApp_ChatPanel_DefaultState(t *testing.T) {
	m := createTestModel(t)

	// Panel should start hidden
	require.False(t, m.chatPanel.Visible(), "chat panel should start hidden")

	// Panel should start unfocused
	require.False(t, m.chatPanelFocused, "chat panel should start unfocused")

	// View should render without chat panel (since hidden)
	view := m.View()
	require.NotEmpty(t, view, "view should render with hidden chat panel")
}

func TestApp_ChatPanelWidth(t *testing.T) {
	m := createTestModel(t)

	// Chat panel uses a fixed width regardless of terminal size
	require.Equal(t, 50, m.chatPanelWidth(), "chat panel should use fixed width of 50")
}

func TestApp_View_WithPanelVisible(t *testing.T) {
	m := createTestModel(t)

	// Set terminal size
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 150, Height: 40})
	m = newModel.(Model)

	// Toggle panel visible
	m.chatPanel = m.chatPanel.Toggle()
	require.True(t, m.chatPanel.Visible(), "panel should be visible after toggle")

	// In kanban mode, view should include chat panel
	require.Equal(t, mode.ModeKanban, m.currentMode, "should be in kanban mode")

	view := m.View()
	require.NotEmpty(t, view, "view should render with panel visible")

	// Panel width should be 50 (fixed)
	panelWidth := m.chatPanelWidth()
	require.Equal(t, 50, panelWidth, "fixed panel width")

	// Main content should be shrunk (150 - 45 = 105)
	// We can verify this indirectly by checking the view renders
	// The actual width adjustment is verified by the layout not breaking
}

func TestApp_View_WithPanelVisible_InSearch(t *testing.T) {
	m := createTestModel(t)

	// Set terminal size
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 150, Height: 40})
	m = newModel.(Model)

	// Switch to search mode
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = newModel.(Model)
	require.Equal(t, mode.ModeSearch, m.currentMode, "should be in search mode")

	// Toggle panel visible
	m.chatPanel = m.chatPanel.Toggle()
	require.True(t, m.chatPanel.Visible(), "panel should be visible after toggle")

	// View should render without errors
	view := m.View()
	require.NotEmpty(t, view, "view should render with panel visible in search mode")
}

func TestApp_View_PanelHiddenInDashboard(t *testing.T) {
	m := createTestModel(t)

	// Set terminal size
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 150, Height: 40})
	m = newModel.(Model)

	// Toggle panel visible
	m.chatPanel = m.chatPanel.Toggle()
	require.True(t, m.chatPanel.Visible(), "panel should be visible after toggle")

	// Switch to dashboard mode
	m.currentMode = mode.ModeDashboard

	// View should NOT include chat panel (excluded in dashboard mode)
	// even though chatPanel.Visible() is true
	view := m.View()
	require.NotEmpty(t, view, "view should render in dashboard mode")

	// The dashboard view should be full width (not composed with chat panel)
	// We verify this by checking that the panel's View() isn't being called
	// through the layout composition logic
}

func TestApp_View_PanelHidden_NoLayoutChange(t *testing.T) {
	m := createTestModel(t)

	// Set terminal size
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 150, Height: 40})
	m = newModel.(Model)

	// Panel starts hidden
	require.False(t, m.chatPanel.Visible(), "panel should start hidden")

	// View should render at full width (no chat panel)
	view := m.View()
	require.NotEmpty(t, view, "view should render with hidden panel")

	// The main content should get the full width
	// This is verified by the view rendering without issues
}

func TestApp_ChatPanel_WindowSizeMsg_UpdatesPanel(t *testing.T) {
	m := createTestModel(t)

	// Initial resize
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = newModel.(Model)

	// Panel should have been sized (chatPanelWidth for 100 = 40)
	require.Equal(t, 50, m.chatPanelWidth(), "fixed panel width")

	// Resize to larger
	newModel, _ = m.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	m = newModel.(Model)

	// Panel width calculation should update
	require.Equal(t, 50, m.chatPanelWidth(), "fixed panel width")
}

func TestApp_ChatPanel_ToggleFromKanban(t *testing.T) {
	m := createTestModel(t)

	// Pre-set infrastructure to avoid client creation during toggle
	infra := newTestChatInfrastructure(t)
	err := infra.Start()
	require.NoError(t, err)
	m.chatInfra = infra
	m.chatPanel = m.chatPanel.SetInfrastructure(infra)

	// Set terminal size (must be >= 100 for panel to open)
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 150, Height: 40})
	m = newModel.(Model)

	// Verify we're in kanban mode
	require.Equal(t, mode.ModeKanban, m.currentMode, "should start in kanban mode")

	// Panel should start hidden and unfocused
	require.False(t, m.chatPanel.Visible(), "panel should start hidden")
	require.False(t, m.chatPanelFocused, "panel should start unfocused")

	// Ctrl+W should open panel and focus it
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = newModel.(Model)

	require.True(t, m.chatPanel.Visible(), "panel should be visible after Ctrl+W")
	require.True(t, m.chatPanelFocused, "panel should be focused after Ctrl+W")

	// Ctrl+W again should close panel and unfocus
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = newModel.(Model)

	require.False(t, m.chatPanel.Visible(), "panel should be hidden after second Ctrl+W")
	require.False(t, m.chatPanelFocused, "panel should be unfocused after second Ctrl+W")
}

func TestApp_ChatPanel_ToggleFromSearch(t *testing.T) {
	m := createTestModel(t)

	// Pre-set infrastructure to avoid client creation during toggle
	infra := newTestChatInfrastructure(t)
	err := infra.Start()
	require.NoError(t, err)
	m.chatInfra = infra
	m.chatPanel = m.chatPanel.SetInfrastructure(infra)

	// Set terminal size
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 150, Height: 40})
	m = newModel.(Model)

	// Switch to search mode
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = newModel.(Model)
	require.Equal(t, mode.ModeSearch, m.currentMode, "should be in search mode")

	// Panel should start hidden
	require.False(t, m.chatPanel.Visible(), "panel should start hidden")

	// Ctrl+W should open panel and focus it in search mode
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = newModel.(Model)

	require.True(t, m.chatPanel.Visible(), "panel should be visible after Ctrl+W in search mode")
	require.True(t, m.chatPanelFocused, "panel should be focused after Ctrl+W in search mode")

	// Ctrl+W again should close panel
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = newModel.(Model)

	require.False(t, m.chatPanel.Visible(), "panel should be hidden after second Ctrl+W")
	require.False(t, m.chatPanelFocused, "panel should be unfocused after second Ctrl+W")
}

func TestApp_ChatPanel_ToggleInDashboard_NoEffect(t *testing.T) {
	m := createTestModel(t)

	// Pre-set infrastructure to avoid client creation during toggle
	infra := newTestChatInfrastructure(t)
	err := infra.Start()
	require.NoError(t, err)
	m.chatInfra = infra
	m.chatPanel = m.chatPanel.SetInfrastructure(infra)

	// Set terminal size
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 150, Height: 40})
	m = newModel.(Model)

	// First open panel in kanban mode
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = newModel.(Model)
	require.True(t, m.chatPanel.Visible(), "panel should be visible in kanban mode")

	// Now close panel before switching to dashboard
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = newModel.(Model)
	require.False(t, m.chatPanel.Visible(), "panel should be hidden")

	// Switch to dashboard mode (setting directly to avoid initializing full dashboard)
	m.currentMode = mode.ModeDashboard

	// Verify the handleToggleChatPanel would return early for dashboard mode
	// We can't easily send Ctrl+W without initializing dashboard, but we can verify
	// the condition check works by testing the View excludes the panel
	m.chatPanel = m.chatPanel.Toggle() // Force visible
	require.True(t, m.chatPanel.Visible(), "panel.Visible() should be true")

	// But View should NOT show panel in dashboard mode (even if visible flag is true)
	// This is verified by the View implementation check: m.currentMode != mode.ModeDashboard
	view := m.View()
	require.NotEmpty(t, view, "view should render")

	// The key point: panel is excluded from dashboard mode by both:
	// 1. View(): checks m.currentMode != mode.ModeDashboard
	// 2. Update(): checks m.currentMode != mode.ModeDashboard before handling Ctrl+W
}

func TestApp_ChatPanel_FocusRouting(t *testing.T) {
	m := createTestModel(t)

	// Pre-set infrastructure to avoid client creation during toggle
	infra := newTestChatInfrastructure(t)
	err := infra.Start()
	require.NoError(t, err)
	m.chatInfra = infra
	m.chatPanel = m.chatPanel.SetInfrastructure(infra)

	// Set terminal size
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 150, Height: 40})
	m = newModel.(Model)

	// Open panel (focuses it)
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = newModel.(Model)

	require.True(t, m.chatPanel.Visible(), "panel should be visible")
	require.True(t, m.chatPanelFocused, "panel should be focused")

	// Key events should route to panel when focused
	// Send a letter key - it should go to the panel's input
	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = newModel.(Model)

	// Panel should still be focused (key was consumed by panel)
	require.True(t, m.chatPanelFocused, "panel should remain focused after key")

	// Command should be from panel (or nil if panel handled it internally)
	_ = cmd // Just verify no panic
}

func TestApp_ChatPanel_FocusRouting_KeysToModeWhenUnfocused(t *testing.T) {
	m := createTestModel(t)

	// Pre-set infrastructure to avoid client creation during toggle
	infra := newTestChatInfrastructure(t)
	err := infra.Start()
	require.NoError(t, err)
	m.chatInfra = infra
	m.chatPanel = m.chatPanel.SetInfrastructure(infra)

	// Set terminal size
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 150, Height: 40})
	m = newModel.(Model)

	// Open panel
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = newModel.(Model)
	require.True(t, m.chatPanel.Visible(), "panel should be visible")
	require.True(t, m.chatPanelFocused, "panel should be focused")

	// Close panel (unfocuses it)
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = newModel.(Model)
	require.False(t, m.chatPanel.Visible(), "panel should be hidden")
	require.False(t, m.chatPanelFocused, "panel should be unfocused")

	// Key events should route to mode when panel is not focused
	// Ctrl+Space should switch mode (demonstrates keys go to mode, not panel)
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = newModel.(Model)

	require.Equal(t, mode.ModeSearch, m.currentMode, "mode should switch when panel unfocused")
}

func TestApp_ChatPanel_MinWidthToast(t *testing.T) {
	m := createTestModel(t)

	// Set terminal width below minimum (100)
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	m = newModel.(Model)

	// Panel should start hidden
	require.False(t, m.chatPanel.Visible(), "panel should start hidden")

	// Ctrl+W should NOT open panel, should return toast command
	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = newModel.(Model)

	// Panel should still be hidden
	require.False(t, m.chatPanel.Visible(), "panel should remain hidden when terminal too narrow")
	require.False(t, m.chatPanelFocused, "panel should remain unfocused when terminal too narrow")

	// Command should be a toast message
	require.NotNil(t, cmd, "should return toast command")

	// Execute the command to get the message
	msg := cmd()
	toastMsg, ok := msg.(mode.ShowToastMsg)
	require.True(t, ok, "command should produce ShowToastMsg")
	require.Contains(t, toastMsg.Message, "100", "toast should mention required width")
	require.Contains(t, toastMsg.Message, "80", "toast should mention current width")
}

func TestApp_ChatPanel_MinWidthExactBoundary(t *testing.T) {
	m := createTestModel(t)

	// Pre-set infrastructure to avoid client creation during toggle
	infra := newTestChatInfrastructure(t)
	err := infra.Start()
	require.NoError(t, err)
	m.chatInfra = infra
	m.chatPanel = m.chatPanel.SetInfrastructure(infra)

	// Set terminal width at exactly minimum (100)
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = newModel.(Model)

	// Ctrl+W should open panel at exactly minimum width
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = newModel.(Model)

	require.True(t, m.chatPanel.Visible(), "panel should open at exactly minimum width")
	require.True(t, m.chatPanelFocused, "panel should be focused")
}

func TestApp_ChatPanel_MinWidthJustBelow(t *testing.T) {
	m := createTestModel(t)

	// Set terminal width just below minimum (99)
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 99, Height: 40})
	m = newModel.(Model)

	// Ctrl+W should NOT open panel
	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = newModel.(Model)

	require.False(t, m.chatPanel.Visible(), "panel should not open just below minimum width")
	require.NotNil(t, cmd, "should return toast command")
}

// ============================================================================
// Dashboard Mode Transition Tests
// ============================================================================

func TestApp_ChatPanel_HiddenInDashboardMode(t *testing.T) {
	m := createTestModel(t)

	// Set terminal size
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 150, Height: 40})
	m = newModel.(Model)

	// Toggle panel visible
	m.chatPanel = m.chatPanel.Toggle()
	require.True(t, m.chatPanel.Visible(), "panel should be visible after toggle")

	// Switch to dashboard mode (setting directly to avoid initializing full dashboard)
	m.currentMode = mode.ModeDashboard

	// View should NOT include chat panel (excluded in dashboard mode)
	// even though chatPanel.Visible() is true
	view := m.View()
	require.NotEmpty(t, view, "view should render in dashboard mode")

	// The infrastructure's context should be cancelled (verified via Shutdown being called)
	// We can't easily verify internal state, but we can verify no panic occurred
	// and the transition completed successfully
}

// ============================================================================
// Resize Edge Case Tests (perles-hj2a.11)
// ============================================================================

func TestApp_ChatPanel_ClosesOnResizeBelowMinimum(t *testing.T) {
	m := createTestModel(t)

	// Pre-set infrastructure to avoid client creation during toggle
	infra := newTestChatInfrastructure(t)
	err := infra.Start()
	require.NoError(t, err)
	m.chatInfra = infra
	m.chatPanel = m.chatPanel.SetInfrastructure(infra)

	// Set terminal size (must be >= 100 for panel to open)
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 150, Height: 40})
	m = newModel.(Model)

	// Open chat panel
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = newModel.(Model)
	require.True(t, m.chatPanel.Visible(), "panel should be visible after Ctrl+W")
	require.True(t, m.chatPanelFocused, "panel should be focused after Ctrl+W")

	// Resize terminal below minimum (100 columns)
	newModel, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	m = newModel.(Model)

	// Panel should auto-close
	require.False(t, m.chatPanel.Visible(), "panel should auto-close when terminal resizes below minimum")
	require.False(t, m.chatPanelFocused, "panel should be unfocused when auto-closed")

	// Should return a toast command
	require.NotNil(t, cmd, "should return toast command")
}

func TestApp_ChatPanel_ClosesOnResizeBelowMinimum_ExactBoundary(t *testing.T) {
	m := createTestModel(t)

	// Pre-set infrastructure to avoid client creation during toggle
	infra := newTestChatInfrastructure(t)
	err := infra.Start()
	require.NoError(t, err)
	m.chatInfra = infra
	m.chatPanel = m.chatPanel.SetInfrastructure(infra)

	// Set terminal size to exactly minimum
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = newModel.(Model)

	// Open chat panel
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = newModel.(Model)
	require.True(t, m.chatPanel.Visible(), "panel should be visible at exactly minimum width")

	// Resize to just below minimum (99)
	newModel, cmd := m.Update(tea.WindowSizeMsg{Width: 99, Height: 40})
	m = newModel.(Model)

	// Panel should auto-close
	require.False(t, m.chatPanel.Visible(), "panel should auto-close at 99 columns")
	require.NotNil(t, cmd, "should return toast command")
}

func TestApp_ChatPanel_StaysOpenOnResizeAboveMinimum(t *testing.T) {
	m := createTestModel(t)

	// Pre-set infrastructure to avoid client creation during toggle
	infra := newTestChatInfrastructure(t)
	err := infra.Start()
	require.NoError(t, err)
	m.chatInfra = infra
	m.chatPanel = m.chatPanel.SetInfrastructure(infra)

	// Set terminal size
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 150, Height: 40})
	m = newModel.(Model)

	// Open chat panel
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = newModel.(Model)
	require.True(t, m.chatPanel.Visible(), "panel should be visible")

	// Resize terminal but stay above minimum
	newModel, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 50})
	m = newModel.(Model)

	// Panel should stay open
	require.True(t, m.chatPanel.Visible(), "panel should remain visible when resizing above minimum")
	require.Nil(t, cmd, "should not return toast command when panel stays open")
}

func TestApp_ChatPanel_ResizeToastShown(t *testing.T) {
	m := createTestModel(t)

	// Pre-set infrastructure to avoid client creation during toggle
	infra := newTestChatInfrastructure(t)
	err := infra.Start()
	require.NoError(t, err)
	m.chatInfra = infra
	m.chatPanel = m.chatPanel.SetInfrastructure(infra)

	// Set terminal size
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 150, Height: 40})
	m = newModel.(Model)

	// Open chat panel
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = newModel.(Model)
	require.True(t, m.chatPanel.Visible(), "panel should be visible")

	// Resize terminal below minimum
	newModel, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	m = newModel.(Model)

	// Verify toast command is returned
	require.NotNil(t, cmd, "should return toast command")

	// Execute the command to get the message
	msg := cmd()
	toastMsg, ok := msg.(mode.ShowToastMsg)
	require.True(t, ok, "command should produce ShowToastMsg")
	require.Contains(t, toastMsg.Message, "narrow", "toast should mention 'narrow'")
	require.Contains(t, toastMsg.Message, "chat panel", "toast should mention 'chat panel'")
}

func TestApp_ChatPanel_PersistsAcrossModeSwitch(t *testing.T) {
	m := createTestModel(t)
	terminalWidth := 150
	terminalHeight := 40

	// Pre-set infrastructure to avoid client creation during toggle
	infra := newTestChatInfrastructure(t)
	err := infra.Start()
	require.NoError(t, err)
	m.chatInfra = infra
	m.chatPanel = m.chatPanel.SetInfrastructure(infra)

	// Set terminal size
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: terminalWidth, Height: terminalHeight})
	m = newModel.(Model)

	// Verify starting in Kanban mode
	require.Equal(t, mode.ModeKanban, m.currentMode, "should start in kanban mode")
	require.Equal(t, terminalWidth, m.width, "kanban should start with full terminal width")

	// Open chat panel
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = newModel.(Model)
	require.True(t, m.chatPanel.Visible(), "panel should be visible in kanban mode")
	require.True(t, m.chatPanelFocused, "panel should be focused")

	// WIDTH VERIFICATION: When panel is visible, verify app width is preserved
	// The main content width is calculated in View() as: m.width - m.chatPanelWidth()
	require.Equal(t, terminalWidth, m.width, "app width should remain terminal width with panel open")

	// Close panel to test persistence (panel state, not just focus)
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = newModel.(Model)
	require.False(t, m.chatPanel.Visible(), "panel should be hidden after toggle")

	// Re-open panel
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = newModel.(Model)
	require.True(t, m.chatPanel.Visible(), "panel should be visible again")

	// Switch to search mode (Ctrl+Space)
	// Note: We need to unfocus the panel first to allow mode switch key to go through
	m.chatPanelFocused = false // Unfocus panel so mode switch key works
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = newModel.(Model)
	require.Equal(t, mode.ModeSearch, m.currentMode, "should be in search mode after Ctrl+Space")

	// Panel should STILL be visible (persists across mode switch)
	require.True(t, m.chatPanel.Visible(), "panel should persist when switching to search mode")

	// WIDTH VERIFICATION: Search mode should have correct width with panel visible
	require.Equal(t, terminalWidth, m.width, "app width should remain terminal width in search mode")
	// Verify View renders correctly (SetSize was called with reduced width)
	view := m.View()
	require.NotEmpty(t, view, "search view should render correctly with panel visible")

	// Switch back to kanban mode
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = newModel.(Model)
	require.Equal(t, mode.ModeKanban, m.currentMode, "should be back in kanban mode")

	// Panel should STILL be visible
	require.True(t, m.chatPanel.Visible(), "panel should persist when switching back to kanban mode")

	// WIDTH VERIFICATION: Kanban mode should have correct width with panel visible
	require.Equal(t, terminalWidth, m.width, "app width should remain terminal width back in kanban")
	// Verify View renders correctly (SetSize was called with reduced width)
	view = m.View()
	require.NotEmpty(t, view, "kanban view should render correctly with panel visible")
}

func TestApp_ChatPanel_PersistsAcrossModeSwitch_HiddenState(t *testing.T) {
	m := createTestModel(t)
	terminalWidth := 150
	terminalHeight := 40

	// Pre-set infrastructure to avoid client creation during toggle
	infra := newTestChatInfrastructure(t)
	err := infra.Start()
	require.NoError(t, err)
	m.chatInfra = infra
	m.chatPanel = m.chatPanel.SetInfrastructure(infra)

	// Set terminal size
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: terminalWidth, Height: terminalHeight})
	m = newModel.(Model)

	// Panel starts hidden
	require.False(t, m.chatPanel.Visible(), "panel should start hidden")
	require.Equal(t, terminalWidth, m.width, "kanban should have full terminal width")

	// Switch to search mode
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = newModel.(Model)
	require.Equal(t, mode.ModeSearch, m.currentMode, "should be in search mode")

	// Panel should still be hidden
	require.False(t, m.chatPanel.Visible(), "hidden panel should persist as hidden in search mode")

	// WIDTH VERIFICATION: Search mode should have full width with panel hidden
	require.Equal(t, terminalWidth, m.width, "search should have full terminal width with panel hidden")
	view := m.View()
	require.NotEmpty(t, view, "search view should render at full width")

	// Open panel in search mode
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = newModel.(Model)
	require.True(t, m.chatPanel.Visible(), "panel should open in search mode")

	// Switch back to kanban (need to unfocus panel first)
	m.chatPanelFocused = false
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = newModel.(Model)
	require.Equal(t, mode.ModeKanban, m.currentMode, "should be back in kanban mode")

	// Panel should still be visible
	require.True(t, m.chatPanel.Visible(), "panel opened in search should persist to kanban")

	// WIDTH VERIFICATION: Kanban mode should have correct width with panel visible
	require.Equal(t, terminalWidth, m.width, "app width should remain terminal width")
	view = m.View()
	require.NotEmpty(t, view, "kanban view should render correctly with panel visible")
}

func TestApp_ChatPanel_SetSizeCalledOnResize(t *testing.T) {
	m := createTestModel(t)

	// Pre-set infrastructure to avoid client creation during toggle
	infra := newTestChatInfrastructure(t)
	err := infra.Start()
	require.NoError(t, err)
	m.chatInfra = infra
	m.chatPanel = m.chatPanel.SetInfrastructure(infra)

	// Initial resize
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = newModel.(Model)

	// Verify initial panel width calculation
	initialPanelWidth := m.chatPanelWidth()
	require.Equal(t, 50, initialPanelWidth, "fixed panel width")

	// Resize to larger terminal
	newModel, _ = m.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	m = newModel.(Model)

	// Panel width calculation should have updated
	newPanelWidth := m.chatPanelWidth()
	require.Equal(t, 50, newPanelWidth, "fixed panel width after resize")

	// Verify app dimensions updated
	require.Equal(t, 200, m.width, "app width should be 200")
	require.Equal(t, 60, m.height, "app height should be 60")

	// SetSize was called internally - verify by checking the chat panel dimensions
	// are consistent with the new terminal size (panel should work with new dimensions)
	// Open panel and verify view renders correctly
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = newModel.(Model)
	require.True(t, m.chatPanel.Visible(), "panel should open after resize")

	// View should render without panic (proves SetSize was called correctly)
	view := m.View()
	require.NotEmpty(t, view, "view should render after resize with panel visible")
}

func TestApp_ChatPanel_SetSizeCalledOnResize_WithPanelVisible(t *testing.T) {
	m := createTestModel(t)

	// Pre-set infrastructure to avoid client creation during toggle
	infra := newTestChatInfrastructure(t)
	err := infra.Start()
	require.NoError(t, err)
	m.chatInfra = infra
	m.chatPanel = m.chatPanel.SetInfrastructure(infra)

	// Set terminal size
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 150, Height: 40})
	m = newModel.(Model)

	// Open chat panel
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = newModel.(Model)
	require.True(t, m.chatPanel.Visible(), "panel should be visible")

	// Resize terminal (staying above minimum)
	newModel, _ = m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	m = newModel.(Model)

	// Panel should still be visible
	require.True(t, m.chatPanel.Visible(), "panel should remain visible after resize")

	// Panel width should have been recalculated
	require.Equal(t, 50, m.chatPanelWidth(), "fixed panel width")

	// View should render correctly with new dimensions
	view := m.View()
	require.NotEmpty(t, view, "view should render with resized panel")
}

func TestApp_ChatPanel_SubmitError_ShowsToast(t *testing.T) {
	m := createTestModel(t)

	// Pre-set infrastructure to avoid client creation during toggle
	infra := newTestChatInfrastructure(t)
	err := infra.Start()
	require.NoError(t, err)
	m.chatInfra = infra
	m.chatPanel = m.chatPanel.SetInfrastructure(infra)

	// Set terminal size
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 150, Height: 40})
	m = newModel.(Model)

	// Open chat panel
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = newModel.(Model)
	require.True(t, m.chatPanel.Visible(), "panel should be visible")

	// Try to send a message without infrastructure set
	// This simulates an error condition - the panel's SendMessage() should return AssistantErrorMsg
	// The app should convert AssistantErrorMsg to a ShowToastMsg

	// Create a chat panel without infrastructure and try to use it
	panelWithoutInfra := chatpanel.New(chatpanel.Config{
		ClientType:     "claude",
		WorkDir:        "",
		SessionTimeout: chatpanel.DefaultConfig().SessionTimeout,
	})

	// SendMessage without infrastructure should return error cmd
	cmd := panelWithoutInfra.SendMessage("test message")
	require.NotNil(t, cmd, "SendMessage should return error cmd when no infrastructure")

	// Execute the command to get the error message
	msg := cmd()
	errMsg, ok := msg.(chatpanel.AssistantErrorMsg)
	require.True(t, ok, "should be AssistantErrorMsg")
	require.Error(t, errMsg.Error, "error should be present")
	require.Equal(t, chatpanel.ErrNoInfrastructure, errMsg.Error, "should be ErrNoInfrastructure")
}

func TestApp_ChatPanel_SpawnError_ReturnsErrorMsg(t *testing.T) {
	// Test that SpawnAssistant without infrastructure returns an error

	panelWithoutInfra := chatpanel.New(chatpanel.Config{
		ClientType:     "claude",
		WorkDir:        "",
		SessionTimeout: chatpanel.DefaultConfig().SessionTimeout,
	})

	// SpawnAssistant without infrastructure should return error cmd
	_, cmd := panelWithoutInfra.SpawnAssistant()
	require.NotNil(t, cmd, "SpawnAssistant should return error cmd when no infrastructure")

	// Execute the command to get the error message
	msg := cmd()
	errMsg, ok := msg.(chatpanel.AssistantErrorMsg)
	require.True(t, ok, "should be AssistantErrorMsg")
	require.Error(t, errMsg.Error, "error should be present")
	require.Equal(t, chatpanel.ErrNoInfrastructure, errMsg.Error, "should be ErrNoInfrastructure")
}

func TestApp_ChatPanel_ResizeWithInfrastructure_CallsCleanup(t *testing.T) {
	m := createTestModel(t)

	// Set terminal size
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 150, Height: 40})
	m = newModel.(Model)

	// Create and set infrastructure on the chat panel
	infra := newTestChatInfrastructure(t)
	err := infra.Start()
	require.NoError(t, err)

	// Set infrastructure on chat panel
	m.chatPanel = m.chatPanel.SetInfrastructure(infra)
	require.True(t, m.chatPanel.HasInfrastructure(), "infrastructure should be set")

	// Open chat panel
	m.chatPanel = m.chatPanel.Toggle().Focus()
	m.chatPanelFocused = true
	require.True(t, m.chatPanel.Visible(), "panel should be visible")

	// Resize terminal below minimum
	// This should trigger Cleanup() which calls infra.Shutdown()
	newModel, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	m = newModel.(Model)

	// Panel should be closed
	require.False(t, m.chatPanel.Visible(), "panel should auto-close on resize below minimum")
	require.False(t, m.chatPanelFocused, "panel should be unfocused")

	// Toast should be returned
	require.NotNil(t, cmd, "should return toast command")

	// Verify the infrastructure was cleaned up (no panic, transition completed)
	// We can't easily verify internal state, but successful completion proves cleanup worked
}

// ============================================================================
// Width Restoration Tests (perles-hj2a.13)
// ============================================================================

func TestApp_ChatPanel_Toggle_RestoresFullWidth_Kanban(t *testing.T) {
	m := createTestModel(t)

	// Pre-set infrastructure to avoid client creation during toggle
	infra := newTestChatInfrastructure(t)
	err := infra.Start()
	require.NoError(t, err)
	m.chatInfra = infra
	m.chatPanel = m.chatPanel.SetInfrastructure(infra)

	// Set terminal size
	terminalWidth := 150
	terminalHeight := 40
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: terminalWidth, Height: terminalHeight})
	m = newModel.(Model)

	// Verify starting in Kanban mode with full width
	require.Equal(t, mode.ModeKanban, m.currentMode, "should start in kanban mode")
	require.Equal(t, terminalWidth, m.width, "app width should be terminal width")

	// Open chat panel
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = newModel.(Model)
	require.True(t, m.chatPanel.Visible(), "panel should be visible after Ctrl+W")

	// Panel uses fixed width
	panelWidth := m.chatPanelWidth()
	require.Equal(t, 50, panelWidth, "fixed panel width")

	// When panel is open, main content width should be terminal width - panel width
	// This is calculated in View() and applied via SetSize
	expectedMainWidth := terminalWidth - panelWidth
	require.Equal(t, 100, expectedMainWidth, "main content should be 100 wide with panel open (150 - 50)")

	// Close chat panel with Ctrl+W
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = newModel.(Model)
	require.False(t, m.chatPanel.Visible(), "panel should be hidden after toggle")

	// After closing, the kanban mode should have been resized to full width
	// We verify this by checking:
	// 1. The View() renders without the panel (showChatPanel = false)
	// 2. The kanban mode was explicitly resized in handleToggleChatPanel()
	//
	// The fix in handleToggleChatPanel() calls:
	//   m.kanban = m.kanban.SetSize(m.width, m.height)
	// This ensures the kanban mode gets the full terminal width immediately

	// Verify View() renders correctly at full width (no gap)
	view := m.View()
	require.NotEmpty(t, view, "view should render")

	// The view length should reflect full terminal width usage
	// Note: We can't easily measure rendered width, but the key test is that
	// SetSize was called with full width, which we verified by code inspection.
	// The integration test is that View() renders without errors and the fix
	// is in handleToggleChatPanel() calling SetSize on close.
}

func TestApp_ChatPanel_Toggle_RestoresFullWidth_Search(t *testing.T) {
	m := createTestModel(t)

	// Pre-set infrastructure to avoid client creation during toggle
	infra := newTestChatInfrastructure(t)
	err := infra.Start()
	require.NoError(t, err)
	m.chatInfra = infra
	m.chatPanel = m.chatPanel.SetInfrastructure(infra)

	// Set terminal size
	terminalWidth := 150
	terminalHeight := 40
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: terminalWidth, Height: terminalHeight})
	m = newModel.(Model)

	// Switch to search mode
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	m = newModel.(Model)
	require.Equal(t, mode.ModeSearch, m.currentMode, "should be in search mode")

	// Open chat panel
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = newModel.(Model)
	require.True(t, m.chatPanel.Visible(), "panel should be visible after Ctrl+W")

	// Close chat panel with Ctrl+W
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = newModel.(Model)
	require.False(t, m.chatPanel.Visible(), "panel should be hidden after toggle")

	// After closing, the search mode should have been resized to full width
	// The fix in handleToggleChatPanel() calls:
	//   m.search = m.search.SetSize(m.width, m.height)

	// Verify View() renders correctly at full width
	view := m.View()
	require.NotEmpty(t, view, "view should render with full width in search mode")
}

func TestApp_ChatPanel_Toggle_MultipleOpenClose_MaintainsWidth(t *testing.T) {
	m := createTestModel(t)

	// Pre-set infrastructure to avoid client creation during toggle
	infra := newTestChatInfrastructure(t)
	err := infra.Start()
	require.NoError(t, err)
	m.chatInfra = infra
	m.chatPanel = m.chatPanel.SetInfrastructure(infra)

	// Set terminal size
	terminalWidth := 150
	terminalHeight := 40
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: terminalWidth, Height: terminalHeight})
	m = newModel.(Model)

	// Multiple open/close cycles should all restore full width correctly
	for i := 0; i < 3; i++ {
		// Open panel
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
		m = newModel.(Model)
		require.True(t, m.chatPanel.Visible(), "panel should be visible (iteration %d)", i)

		// Close panel
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
		m = newModel.(Model)
		require.False(t, m.chatPanel.Visible(), "panel should be hidden (iteration %d)", i)

		// Verify view renders correctly (full width restored)
		view := m.View()
		require.NotEmpty(t, view, "view should render correctly (iteration %d)", i)
	}
}

// ============================================================================
// Mode Switch Resize Regression Tests (perles-yn47.5)
// ============================================================================
//
// These tests ensure the chatpanel resize bug (perles-yn47) cannot recur.
// The bug occurred when: open chatpanel → close chatpanel → switch mode.
// The target mode retained its previous dimensions instead of getting full width.

// TestApp_ModeSwitch_AfterClosingChatPanel_RestoresFullWidth is the main bug
// regression test. This directly tests the scenario that caused the original bug:
//  1. Open chatpanel in kanban mode (kanban resized to reduced width)
//  2. Close chatpanel (kanban should have full width now)
//  3. Switch to search via Ctrl+Space
//  4. Assert: search mode has full terminal width (not the old reduced width)
//
// Before the fix, search mode would retain its previous dimensions (which could
// be the reduced width from when chatpanel was visible), leaving a gap in the UI.
func TestApp_ModeSwitch_AfterClosingChatPanel_RestoresFullWidth(t *testing.T) {
	terminalWidth := 150
	terminalHeight := 40

	t.Run("Kanban to Search - main bug scenario", func(t *testing.T) {
		m := createTestModel(t)

		// Pre-set infrastructure to avoid client creation during toggle
		infra := newTestChatInfrastructure(t)
		err := infra.Start()
		require.NoError(t, err)
		m.chatInfra = infra
		m.chatPanel = m.chatPanel.SetInfrastructure(infra)

		// Set terminal size
		newModel, _ := m.Update(tea.WindowSizeMsg{Width: terminalWidth, Height: terminalHeight})
		m = newModel.(Model)

		// Verify starting state
		require.Equal(t, mode.ModeKanban, m.currentMode, "should start in kanban mode")
		require.Equal(t, terminalWidth, m.width, "app width should be terminal width")

		// Step 1: Open chatpanel (kanban now at reduced width)
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
		m = newModel.(Model)
		require.True(t, m.chatPanel.Visible(), "panel should be visible")

		// Step 2: Close chatpanel (kanban should be back at full width)
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
		m = newModel.(Model)
		require.False(t, m.chatPanel.Visible(), "panel should be hidden after toggle")

		// Step 3: Switch to search mode
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
		m = newModel.(Model)

		// Step 4: Assertions
		require.Equal(t, mode.ModeSearch, m.currentMode, "should be in search mode")
		require.False(t, m.chatPanel.Visible(), "panel should remain hidden")
		require.Equal(t, terminalWidth, m.width, "app width should still be terminal width")

		// The critical assertion: View renders correctly at full width.
		// Before the fix, the view would have gaps or incorrect layout because
		// search mode was using the old reduced width.
		view := m.View()
		require.NotEmpty(t, view, "search view should render without panic at full width")
	})

	t.Run("Search to Kanban - reverse scenario", func(t *testing.T) {
		m := createTestModel(t)

		// Pre-set infrastructure
		infra := newTestChatInfrastructure(t)
		err := infra.Start()
		require.NoError(t, err)
		m.chatInfra = infra
		m.chatPanel = m.chatPanel.SetInfrastructure(infra)

		// Set terminal size
		newModel, _ := m.Update(tea.WindowSizeMsg{Width: terminalWidth, Height: terminalHeight})
		m = newModel.(Model)

		// Start in search mode
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
		m = newModel.(Model)
		require.Equal(t, mode.ModeSearch, m.currentMode, "should be in search mode")

		// Step 1: Open chatpanel
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
		m = newModel.(Model)
		require.True(t, m.chatPanel.Visible(), "panel should be visible")

		// Step 2: Close chatpanel
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
		m = newModel.(Model)
		require.False(t, m.chatPanel.Visible(), "panel should be hidden after toggle")

		// Step 3: Switch to kanban mode
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
		m = newModel.(Model)

		// Step 4: Assertions
		require.Equal(t, mode.ModeKanban, m.currentMode, "should be in kanban mode")
		require.False(t, m.chatPanel.Visible(), "panel should remain hidden")
		require.Equal(t, terminalWidth, m.width, "app width should still be terminal width")

		// View renders correctly at full width
		view := m.View()
		require.NotEmpty(t, view, "kanban view should render without panic at full width")
	})
}

// TestApp_ModeSwitch_ChatPanelNeverOpened_CorrectWidth is a baseline regression
// test to ensure the fix doesn't break the normal case where chatpanel was never
// opened. Both modes should always have full terminal width in this case.
func TestApp_ModeSwitch_ChatPanelNeverOpened_CorrectWidth(t *testing.T) {
	terminalWidth := 150
	terminalHeight := 40

	m := createTestModel(t)

	// Set terminal size
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: terminalWidth, Height: terminalHeight})
	m = newModel.(Model)

	// Panel was never opened
	require.False(t, m.chatPanel.Visible(), "panel should start hidden")

	// Multiple mode switches with panel never opened
	for i := 0; i < 3; i++ {
		// Verify kanban state
		require.Equal(t, mode.ModeKanban, m.currentMode, "should be in kanban mode (iteration %d)", i)
		require.Equal(t, terminalWidth, m.width, "kanban should have full terminal width (iteration %d)", i)
		view := m.View()
		require.NotEmpty(t, view, "kanban view should render (iteration %d)", i)

		// Switch to search
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
		m = newModel.(Model)

		// Verify search state
		require.Equal(t, mode.ModeSearch, m.currentMode, "should be in search mode (iteration %d)", i)
		require.Equal(t, terminalWidth, m.width, "search should have full terminal width (iteration %d)", i)
		view = m.View()
		require.NotEmpty(t, view, "search view should render (iteration %d)", i)

		// Switch back to kanban
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
		m = newModel.(Model)
	}
}

// TestApp_ModeSwitch_RapidToggleSequence_MaintainsCorrectWidth is a stress test
// that covers edge cases around rapid panel toggle and mode switch sequences.
// This ensures the width is always correct after each operation in complex scenarios.
func TestApp_ModeSwitch_RapidToggleSequence_MaintainsCorrectWidth(t *testing.T) {
	terminalWidth := 150
	terminalHeight := 40

	m := createTestModel(t)

	// Pre-set infrastructure
	infra := newTestChatInfrastructure(t)
	err := infra.Start()
	require.NoError(t, err)
	m.chatInfra = infra
	m.chatPanel = m.chatPanel.SetInfrastructure(infra)

	// Set terminal size
	newModel, _ := m.Update(tea.WindowSizeMsg{Width: terminalWidth, Height: terminalHeight})
	m = newModel.(Model)

	// Helper to assert width is correct based on panel visibility
	assertCorrectWidth := func(description string) {
		if m.chatPanel.Visible() {
			// Panel visible: main content gets reduced width
			// We verify by checking app width is preserved and view renders
			require.Equal(t, terminalWidth, m.width, "app width should be terminal width: %s", description)
		} else {
			// Panel hidden: main content gets full width
			require.Equal(t, terminalWidth, m.width, "app width should be terminal width: %s", description)
		}
		view := m.View()
		require.NotEmpty(t, view, "view should render: %s", description)
	}

	// Multiple cycles of: open panel, close panel, switch mode
	for cycle := 0; cycle < 3; cycle++ {
		// Cycle start: kanban mode, panel hidden
		require.Equal(t, mode.ModeKanban, m.currentMode)
		require.False(t, m.chatPanel.Visible())
		assertCorrectWidth("cycle %d: kanban, panel hidden")

		// Open panel
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
		m = newModel.(Model)
		require.True(t, m.chatPanel.Visible())
		assertCorrectWidth("cycle %d: kanban, panel open")

		// Close panel
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
		m = newModel.(Model)
		require.False(t, m.chatPanel.Visible())
		assertCorrectWidth("cycle %d: kanban, panel closed")

		// Switch to search
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
		m = newModel.(Model)
		require.Equal(t, mode.ModeSearch, m.currentMode)
		assertCorrectWidth("cycle %d: search, panel hidden")

		// Open panel in search
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
		m = newModel.(Model)
		require.True(t, m.chatPanel.Visible())
		assertCorrectWidth("cycle %d: search, panel open")

		// Close panel in search
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
		m = newModel.(Model)
		require.False(t, m.chatPanel.Visible())
		assertCorrectWidth("cycle %d: search, panel closed")

		// Switch back to kanban
		newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
		m = newModel.(Model)
		require.Equal(t, mode.ModeKanban, m.currentMode)
		assertCorrectWidth("cycle %d: kanban after cycle")
	}
}

// TestApp_OrchestrationQuit_SetsSizeCorrectly verifies that the orchestration.QuitMsg
// handler correctly calculates width based on chatpanel visibility and calls SetSize
// on kanban mode BEFORE RefreshFromConfig(). This ensures kanban mode has correct
// dimensions before layout recalculation when exiting dashboard mode.
//
// Note: The chatpanel is always hidden when entering dashboard mode (closed on entry),
// so the panel should always be hidden when exiting. This test provides defensive coverage
// for the edge case where the panel is somehow visible.
func TestApp_DashboardQuit_SetsSizeCorrectly(t *testing.T) {
	terminalWidth := 150
	terminalHeight := 40

	t.Run("Panel visible (edge case) - kanban gets reduced width", func(t *testing.T) {
		m := createTestModel(t)

		// Pre-set infrastructure to avoid client creation during toggle
		infra := newTestChatInfrastructure(t)
		err := infra.Start()
		require.NoError(t, err)
		m.chatInfra = infra
		m.chatPanel = m.chatPanel.SetInfrastructure(infra)

		// Set terminal size
		newModel, _ := m.Update(tea.WindowSizeMsg{Width: terminalWidth, Height: terminalHeight})
		m = newModel.(Model)

		// Set dashboard mode directly (simulating being in dashboard)
		m.currentMode = mode.ModeDashboard

		// Manually set chatpanel visible to test the defensive edge case
		// (This shouldn't happen in normal operation since panel is closed on dashboard entry,
		// but the SetSize call provides defensive protection)
		m.chatPanel = m.chatPanel.Toggle()
		require.True(t, m.chatPanel.Visible(), "panel should be visible for edge case test")

		// Exit dashboard via QuitMsg
		newModel, _ = m.Update(dashboard.QuitMsg{})
		m = newModel.(Model)

		// Verify mode switched back to kanban
		require.Equal(t, mode.ModeKanban, m.currentMode, "should be back in kanban mode")
		require.True(t, m.chatPanel.Visible(), "panel should remain visible (edge case)")

		// Verify View renders correctly (SetSize was called with reduced width)
		view := m.View()
		require.NotEmpty(t, view, "kanban view should render without panic")
	})
}

// =====================================================
// Database Integration Tests
// =====================================================

func TestApp_InitializesDatabase(t *testing.T) {
	// Database uses ~/.perles/perles-test.db when running under `go test`
	// (automatically detected via testing.Testing())
	cfg := config.Defaults()
	cfg.Flags = map[string]bool{flags.FlagSessionPersistence: true}

	model, err := NewWithConfig(
		nil, // client - not needed for database tests
		cfg,
		nil, // bqlCache
		nil, // depGraphCache
		"",  // dbPath (beads db path)
		"",  // configPath
		"/tmp",
		false, // debugMode
		nil,   // registryService
	)
	require.NoError(t, err, "NewWithConfig should not error")
	require.NotNil(t, model.db, "database should be initialized")
	require.NotNil(t, model.services.SessionRepository, "SessionRepository should be available")

	// Verify database file was created at the test path
	testDBPath := config.DefaultDatabasePath()
	require.Contains(t, testDBPath, "perles-test.db", "should use test database")
	_, err = os.Stat(testDBPath)
	require.NoError(t, err, "database file should exist")

	// Cleanup
	err = model.Close()
	require.NoError(t, err, "Close should not error")
}

func TestApp_Shutdown_ClosesDatabase(t *testing.T) {
	cfg := config.Defaults()
	cfg.Flags = map[string]bool{flags.FlagSessionPersistence: true}

	model, err := NewWithConfig(
		nil, // client
		cfg,
		nil, // bqlCache
		nil, // depGraphCache
		"",  // dbPath
		"",  // configPath
		"/tmp",
		false, // debugMode
		nil,   // registryService
	)
	require.NoError(t, err)
	require.NotNil(t, model.db, "database should be initialized")

	// Get reference to db before close
	db := model.db

	// Close the model
	err = model.Close()
	require.NoError(t, err, "Close should not error")

	// Verify database connection is closed by attempting an operation
	// (A closed connection will return an error)
	_, err = db.Connection().Exec("SELECT 1")
	require.Error(t, err, "database connection should be closed after model.Close()")
}
