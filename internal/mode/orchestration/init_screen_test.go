package orchestration

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/config"
	"github.com/zjrosen/perles/internal/git"
)

// newTestInitializer creates an Initializer in a specific state for testing.
func newTestInitializer(phase InitPhase, err error) *Initializer {
	failedAt := InitNotStarted
	if phase == InitFailed || phase == InitTimedOut {
		failedAt = InitCreatingWorkspace // Default failed phase
	}
	return &Initializer{
		phase:         phase,
		failedAtPhase: failedAt,
		err:           err,
	}
}

// newTestInitializerWithFailedPhase creates an Initializer with specific failed phase.
func newTestInitializerWithFailedPhase(phase InitPhase, failedAt InitPhase, err error) *Initializer {
	return &Initializer{
		phase:         phase,
		failedAtPhase: failedAt,
		err:           err,
	}
}

// newTestInitializerWithTimeouts creates an Initializer with specific failed phase and timeout config.
func newTestInitializerWithTimeouts(phase InitPhase, failedAt InitPhase, err error, timeouts config.TimeoutsConfig) *Initializer {
	return &Initializer{
		phase:         phase,
		failedAtPhase: failedAt,
		err:           err,
		cfg:           InitializerConfig{Timeouts: timeouts},
	}
}

// --- Unit tests for phase ordering ---

func TestInitPhase_PhaseOrdering(t *testing.T) {
	// Verify the numerical ordering of phases is correct
	require.Less(t, int(InitNotStarted), int(InitCreatingWorktree))
	require.Less(t, int(InitCreatingWorktree), int(InitCreatingWorkspace))
	require.Less(t, int(InitCreatingWorkspace), int(InitSpawningCoordinator))
	require.Less(t, int(InitSpawningCoordinator), int(InitAwaitingFirstMessage))
	require.Less(t, int(InitAwaitingFirstMessage), int(InitReady))
}

// --- Unit tests for timeout handling ---
// Note: InitTimeoutMsg is now just a no-op in the TUI layer.
// The actual timeout is handled by the Initializer internally.
// These tests verify the TUI ignores InitTimeoutMsg appropriately.

func TestInitPhase_Timeout(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.initializer = newTestInitializer(InitAwaitingFirstMessage, nil)

	// Send timeout message - should be a no-op in the TUI layer
	m, _ = m.Update(InitTimeoutMsg{})

	// Phase unchanged - Initializer handles actual timeout
	require.Equal(t, InitAwaitingFirstMessage, m.getInitPhase())
}

func TestInitPhase_TimeoutIgnoredWhenReady(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.initializer = newTestInitializer(InitReady, nil)

	// Timeout message should be ignored when already ready
	m, _ = m.Update(InitTimeoutMsg{})

	require.Equal(t, InitReady, m.getInitPhase())
}

func TestInitPhase_TimeoutIgnoredWhenFailed(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.initializer = newTestInitializer(InitFailed, errors.New("previous error"))

	// Timeout message should be ignored when already failed
	m, _ = m.Update(InitTimeoutMsg{})

	require.Equal(t, InitFailed, m.getInitPhase())
	require.NotNil(t, m.getInitError())
}

func TestInitPhase_TimeoutIgnoredWhenAlreadyTimedOut(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.initializer = newTestInitializer(InitTimedOut, nil)

	// Duplicate timeout message should be ignored
	m, _ = m.Update(InitTimeoutMsg{})

	require.Equal(t, InitTimedOut, m.getInitPhase())
}

// --- Unit tests for spinner tick ---

func TestSpinnerTick_AdvancesFrame(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.initializer = newTestInitializer(InitCreatingWorkspace, nil)
	m.spinnerFrame = 0

	// Send spinner tick
	m, cmd := m.Update(SpinnerTickMsg{})

	require.Equal(t, 1, m.spinnerFrame)
	require.NotNil(t, cmd, "should return another tick command during loading")
}

func TestSpinnerTick_WrapsAround(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.initializer = newTestInitializer(InitCreatingWorkspace, nil)
	m.spinnerFrame = len(spinnerFrames) - 1 // Last frame

	// Send spinner tick
	m, _ = m.Update(SpinnerTickMsg{})

	require.Equal(t, 0, m.spinnerFrame) // Should wrap to 0
}

func TestSpinnerTick_StopsWhenReady(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.initializer = newTestInitializer(InitReady, nil)

	// Spinner tick should not continue when ready
	_, cmd := m.Update(SpinnerTickMsg{})

	require.Nil(t, cmd, "should not return tick command when ready")
}

func TestSpinnerTick_StopsWhenFailed(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.initializer = newTestInitializer(InitFailed, nil)

	// Spinner tick should not continue when failed
	_, cmd := m.Update(SpinnerTickMsg{})

	require.Nil(t, cmd, "should not return tick command when failed")
}

func TestSpinnerTick_StopsWhenTimedOut(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.initializer = newTestInitializer(InitTimedOut, nil)

	// Spinner tick should not continue when timed out
	_, cmd := m.Update(SpinnerTickMsg{})

	require.Nil(t, cmd, "should not return tick command when timed out")
}

func TestSpinnerTick_StopsWhenNotStarted(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	// No initializer = InitNotStarted from getInitPhase()

	// Spinner tick should not continue when not started
	_, cmd := m.Update(SpinnerTickMsg{})

	require.Nil(t, cmd, "should not return tick command when not started")
}

func TestSpinnerTick_ContinuesDuringActivePhases(t *testing.T) {
	activePhases := []InitPhase{
		InitCreatingWorkspace,
		InitSpawningCoordinator,
		InitAwaitingFirstMessage,
	}

	for _, phase := range activePhases {
		t.Run(phaseLabels[phase], func(t *testing.T) {
			m := New(Config{})
			m = m.SetSize(120, 30)
			m.initializer = newTestInitializer(phase, nil)

			_, cmd := m.Update(SpinnerTickMsg{})

			require.NotNil(t, cmd, "should return tick command during active phase %v", phase)
		})
	}
}

// --- Unit tests for input blocking during loading ---

func TestInputBlocking_DuringLoading(t *testing.T) {
	activePhases := []InitPhase{
		InitCreatingWorkspace,
		InitSpawningCoordinator,
		InitAwaitingFirstMessage,
	}

	blockedKeys := []tea.KeyMsg{
		{Type: tea.KeyTab},
		{Type: tea.KeyEnter},
		{Type: tea.KeyCtrlP},
		{Type: tea.KeyCtrlF},
		{Type: tea.KeyRunes, Runes: []rune{'a'}},
	}

	for _, phase := range activePhases {
		for _, key := range blockedKeys {
			t.Run(phaseLabels[phase]+"_"+key.String(), func(t *testing.T) {
				m := New(Config{})
				m = m.SetSize(120, 30)
				m.initializer = newTestInitializer(phase, nil)

				// Try to send the key
				_, cmd := m.Update(key)

				// Command should be nil (blocked)
				require.Nil(t, cmd, "key %v should be blocked during phase %v", key, phase)
			})
		}
	}
}

func TestInputBlocking_EscAllowedDuringLoading(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.initializer = newTestInitializer(InitCreatingWorkspace, nil)

	// ESC should produce a QuitMsg command
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})

	require.NotNil(t, cmd, "ESC should not be blocked during loading")
	msg := cmd()
	_, ok := msg.(QuitMsg)
	require.True(t, ok, "ESC should produce QuitMsg")
}

func TestInputBlocking_CtrlCAllowedDuringLoading(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.initializer = newTestInitializer(InitCreatingWorkspace, nil)

	// Ctrl+C should produce a QuitMsg command
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

	require.NotNil(t, cmd, "Ctrl+C should not be blocked during loading")
	msg := cmd()
	_, ok := msg.(QuitMsg)
	require.True(t, ok, "Ctrl+C should produce QuitMsg")
}

func TestInputBlocking_NotBlockedWhenReady(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.initializer = newTestInitializer(InitReady, nil)

	// Input should work when ready - Tab should cycle targets
	initialTarget := m.messageTarget
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	require.NotEqual(t, initialTarget, m.messageTarget, "Tab should work when ready")
}

// --- Unit tests for retry functionality ---

func TestRetry_AfterFailed(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.initializer = newTestInitializer(InitFailed, errors.New("test error"))

	// Press R to retry - without initializer this triggers cleanup and StartCoordinatorMsg
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	// Should produce StartCoordinatorMsg command (initializer handles retry internally)
	require.NotNil(t, cmd)
}

func TestRetry_AfterTimedOut(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.initializer = newTestInitializer(InitTimedOut, nil)

	// Press R to retry - should trigger initializer retry
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}}) // Capital R

	// Spinner frame should be reset
	require.Equal(t, 0, m.spinnerFrame)

	// Should produce a command
	require.NotNil(t, cmd)
}

func TestRetry_EscExitsAfterFailed(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.initializer = newTestInitializer(InitFailed, nil)

	// Press ESC to exit
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})

	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(QuitMsg)
	require.True(t, ok, "ESC should quit after failure")
}

func TestRetry_EscExitsAfterTimedOut(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.initializer = newTestInitializer(InitTimedOut, nil)

	// Press ESC to exit
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})

	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(QuitMsg)
	require.True(t, ok, "ESC should quit after timeout")
}

func TestRetry_OtherKeysIgnoredAfterFailed(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.initializer = newTestInitializer(InitFailed, nil)

	// Try various keys that should be ignored
	keys := []tea.KeyMsg{
		{Type: tea.KeyTab},
		{Type: tea.KeyEnter},
		{Type: tea.KeyRunes, Runes: []rune{'a'}},
		{Type: tea.KeyRunes, Runes: []rune{'x'}},
	}

	for _, key := range keys {
		m, cmd := m.Update(key)
		require.Nil(t, cmd, "key %v should be ignored when failed", key)
		require.Equal(t, InitFailed, m.getInitPhase(), "phase should not change")
	}
}

// --- Unit tests for getPhaseIndicatorAndStyle ---

func TestGetPhaseIndicatorAndStyle_Completed(t *testing.T) {
	m := New(Config{})
	m.initializer = newTestInitializer(InitSpawningCoordinator, nil)

	// Phases before current should show completed
	indicator, _ := m.getPhaseIndicatorAndStyle(InitCreatingWorkspace, InitSpawningCoordinator)
	require.Contains(t, indicator, "✓")
}

func TestGetPhaseIndicatorAndStyle_InProgress(t *testing.T) {
	m := New(Config{})
	m.initializer = newTestInitializer(InitSpawningCoordinator, nil)
	m.spinnerFrame = 0

	// Current phase should show spinner
	indicator, _ := m.getPhaseIndicatorAndStyle(InitSpawningCoordinator, InitSpawningCoordinator)
	require.Contains(t, indicator, spinnerFrames[0])
}

func TestGetPhaseIndicatorAndStyle_Pending(t *testing.T) {
	m := New(Config{})
	m.initializer = newTestInitializer(InitCreatingWorkspace, nil)

	// Phases after current should show pending (space)
	indicator, _ := m.getPhaseIndicatorAndStyle(InitAwaitingFirstMessage, InitCreatingWorkspace)
	require.NotContains(t, indicator, "✓")
	require.NotContains(t, indicator, "✗")
}

func TestGetPhaseIndicatorAndStyle_Failed(t *testing.T) {
	m := New(Config{})
	m.initializer = newTestInitializer(InitFailed, errors.New("test error"))

	// Failed phase should show ✗ (default failed phase is CreatingWorkspace)
	indicator, _ := m.getPhaseIndicatorAndStyle(InitCreatingWorkspace, InitFailed)
	require.Contains(t, indicator, "✗")
}

func TestGetPhaseIndicatorAndStyle_TimedOut(t *testing.T) {
	m := New(Config{})
	m.initializer = newTestInitializerWithFailedPhase(InitTimedOut, InitAwaitingFirstMessage, nil)

	// Phases before AwaitingFirstMessage should be completed
	indicator, _ := m.getPhaseIndicatorAndStyle(InitSpawningCoordinator, InitTimedOut)
	require.Contains(t, indicator, "✓")

	// AwaitingFirstMessage should show failed (that's where we timed out)
	indicator, _ = m.getPhaseIndicatorAndStyle(InitAwaitingFirstMessage, InitTimedOut)
	require.Contains(t, indicator, "✗")
}

func TestGetPhaseIndicatorAndStyle_AllReady(t *testing.T) {
	m := New(Config{})
	m.initializer = newTestInitializer(InitReady, nil)

	// All phases should show completed when ready
	for _, phase := range phaseOrder {
		indicator, _ := m.getPhaseIndicatorAndStyle(phase, InitReady)
		require.Contains(t, indicator, "✓", "phase %v should show checkmark when ready", phase)
	}
}

// --- Golden tests for loading screen states ---

func TestView_Golden_LoadingCreatingWorkspace(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.initializer = newTestInitializer(InitCreatingWorkspace, nil)
	m.worktreeEnabled = true // Show worktree phase in loading screen
	m.spinnerFrame = 0

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_LoadingSpawningCoordinator(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.initializer = newTestInitializer(InitSpawningCoordinator, nil)
	m.worktreeEnabled = true // Show worktree phase in loading screen
	m.spinnerFrame = 2

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_LoadingAwaitingFirstMessage(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.initializer = newTestInitializer(InitAwaitingFirstMessage, nil)
	m.worktreeEnabled = true // Show worktree phase in loading screen
	m.spinnerFrame = 3

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_LoadingFailed(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.initializer = newTestInitializer(InitFailed, errors.New("listen tcp :8765: address already in use"))
	m.worktreeEnabled = true // Show worktree phase in loading screen

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_LoadingTimedOut(t *testing.T) {
	timeouts := config.TimeoutsConfig{
		WorktreeCreation: 30 * time.Second,
		WorkspaceSetup:   30 * time.Second,
		CoordinatorStart: 60 * time.Second,
	}
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.initializer = newTestInitializerWithTimeouts(InitTimedOut, InitAwaitingFirstMessage, nil, timeouts)
	m.worktreeEnabled = true // Show worktree phase in loading screen

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_LoadingNarrow(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(80, 24)
	m.initializer = newTestInitializer(InitSpawningCoordinator, nil)
	m.worktreeEnabled = true // Show worktree phase in loading screen
	m.spinnerFrame = 1

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_LoadingWide(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(160, 40)
	m.initializer = newTestInitializer(InitAwaitingFirstMessage, nil)
	m.worktreeEnabled = true // Show worktree phase in loading screen
	m.spinnerFrame = 5

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// --- Additional edge case tests ---

func TestRenderInitScreen_CentersContent(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(200, 50)
	m.initializer = newTestInitializer(InitCreatingWorkspace, nil)

	view := m.View()

	// The view should have leading whitespace for centering
	lines := make([]string, 0)
	for _, line := range []byte(view) {
		if line == '\n' {
			lines = append(lines, "")
		}
	}
	require.NotEmpty(t, view)
	require.Greater(t, len(view), 100) // Should be larger due to centering padding
}

func TestRenderInitScreen_ShowsCorrectTitle(t *testing.T) {
	tests := []struct {
		name          string
		phase         InitPhase
		err           error
		expectedTitle string
	}{
		{
			name:          "Loading shows Initializing",
			phase:         InitCreatingWorkspace,
			expectedTitle: "Initializing Orchestration",
		},
		{
			name:          "Failed shows Failed",
			phase:         InitFailed,
			err:           errors.New("test error"),
			expectedTitle: "Initialization Failed",
		},
		{
			name:          "TimedOut shows Timed Out",
			phase:         InitTimedOut,
			expectedTitle: "Initialization Timed Out",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(Config{})
			m = m.SetSize(120, 30)
			m.initializer = newTestInitializer(tt.phase, tt.err)

			view := m.View()
			require.Contains(t, view, tt.expectedTitle)
		})
	}
}

func TestRenderInitScreen_ShowsCorrectHints(t *testing.T) {
	tests := []struct {
		name          string
		phase         InitPhase
		err           error
		expectedHints []string
	}{
		{
			name:          "Loading shows ESC Cancel",
			phase:         InitCreatingWorkspace,
			expectedHints: []string{"[ESC] Cancel"},
		},
		{
			name:          "Failed shows Retry and Exit",
			phase:         InitFailed,
			err:           errors.New("test error"),
			expectedHints: []string{"[R] Retry", "[ESC] Exit"},
		},
		{
			name:          "TimedOut shows Retry and Exit",
			phase:         InitTimedOut,
			expectedHints: []string{"[R] Retry", "[ESC] Exit"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(Config{})
			m = m.SetSize(120, 30)
			m.initializer = newTestInitializer(tt.phase, tt.err)

			view := m.View()
			for _, hint := range tt.expectedHints {
				require.Contains(t, view, hint)
			}
		})
	}
}

// --- Worktree phase tests ---

// newTestInitializerWithWorktree creates an Initializer with worktree-specific state for testing.
func newTestInitializerWithWorktree(phase InitPhase, failedAt InitPhase, err error, worktreePath string) *Initializer {
	return &Initializer{
		phase:         phase,
		failedAtPhase: failedAt,
		err:           err,
		worktreePath:  worktreePath,
	}
}

func TestInitScreen_PhaseLabel_CreatingWorktree(t *testing.T) {
	// Verify the phaseLabels map includes InitCreatingWorktree
	label, exists := phaseLabels[InitCreatingWorktree]
	require.True(t, exists, "phaseLabels should include InitCreatingWorktree")
	require.Equal(t, "Creating Worktree", label)
}

func TestInitScreen_PhaseOrder_IncludesWorktree(t *testing.T) {
	// Verify InitCreatingWorktree is in phaseOrder and comes before InitCreatingWorkspace
	found := false
	worktreeIdx := -1
	workspaceIdx := -1
	for i, phase := range phaseOrder {
		if phase == InitCreatingWorktree {
			found = true
			worktreeIdx = i
		}
		if phase == InitCreatingWorkspace {
			workspaceIdx = i
		}
	}
	require.True(t, found, "phaseOrder should include InitCreatingWorktree")
	require.True(t, worktreeIdx < workspaceIdx, "InitCreatingWorktree should come before InitCreatingWorkspace")
}

func TestInitScreen_WorktreePath_DisplayedDuringCreation(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.worktreeEnabled = true // Must enable worktree to show the phase
	worktreePath := "/tmp/test-worktree-abc123"
	m.initializer = newTestInitializerWithWorktree(InitCreatingWorktree, InitNotStarted, nil, worktreePath)
	m.spinnerFrame = 0

	view := m.View()

	// Should show the worktree path during creation
	require.Contains(t, view, worktreePath)
	require.Contains(t, view, "Creating Worktree")
}

func TestInitScreen_WorktreePath_NotDisplayedWhenEmpty(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.worktreeEnabled = true // Must enable worktree to show the phase
	m.initializer = newTestInitializerWithWorktree(InitCreatingWorktree, InitNotStarted, nil, "")
	m.spinnerFrame = 0

	view := m.View()

	// Should show just the label without path when path is empty
	require.Contains(t, view, "Creating Worktree")
	// Should not have a colon after "Creating Worktree" (which indicates path display)
	require.NotContains(t, view, "Creating Worktree:")
}

func TestInitScreen_WorktreeError_ShowsHints(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.worktreeEnabled = true // Must enable worktree to show the phase
	m.initializer = newTestInitializerWithWorktree(InitFailed, InitCreatingWorktree, errors.New("worktree creation failed"), "")

	view := m.View()

	// Should show worktree-specific hints
	require.Contains(t, view, "[R] Retry")
	require.Contains(t, view, "[S] Skip")
	require.Contains(t, view, "[ESC] Exit")
	require.Contains(t, view, "use current dir")
}

func TestInitScreen_WorktreeError_BranchConflict_Message(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.worktreeEnabled = true // Must enable worktree to show the phase
	m.initializer = newTestInitializerWithWorktree(InitFailed, InitCreatingWorktree, errors.New("fatal: 'main' is already checked out at '/other/worktree'"), "")

	view := m.View()

	// Should show user-friendly branch conflict message
	require.Contains(t, view, "Branch is already checked out in another worktree")
}

func TestInitScreen_WorktreeError_PathExists_Message(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.worktreeEnabled = true // Must enable worktree to show the phase
	m.initializer = newTestInitializerWithWorktree(InitFailed, InitCreatingWorktree, errors.New("fatal: '/tmp/worktree' already exists"), "")

	view := m.View()

	// Should show user-friendly path exists message
	require.Contains(t, view, "Worktree path already exists")
}

func TestInitScreen_WorktreeError_NotGitRepo_Message(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.worktreeEnabled = true // Must enable worktree to show the phase
	m.initializer = newTestInitializerWithWorktree(InitFailed, InitCreatingWorktree, errors.New("fatal: not a git repository"), "")

	view := m.View()

	// Should show user-friendly not git repo message
	require.Contains(t, view, "Not a git repository")
	require.Contains(t, view, "Worktree feature unavailable")
}

func TestInitScreen_WorktreeError_GenericError_Message(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.worktreeEnabled = true // Must enable worktree to show the phase
	genericErr := errors.New("some unexpected error")
	m.initializer = newTestInitializerWithWorktree(InitFailed, InitCreatingWorktree, genericErr, "")

	view := m.View()

	// Should show generic worktree error message
	require.Contains(t, view, "Worktree creation failed")
	require.Contains(t, view, "some unexpected error")
}

func TestInitScreen_NonWorktreeError_DoesNotShowSkipHint(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.initializer = newTestInitializerWithFailedPhase(InitFailed, InitCreatingWorkspace, errors.New("workspace error"))

	view := m.View()

	// Should NOT show Skip hint for non-worktree errors
	require.NotContains(t, view, "[S] Skip")
	require.Contains(t, view, "[R] Retry")
	require.Contains(t, view, "[ESC] Exit")
}

func TestWorktreeErrorMessage_BranchConflict(t *testing.T) {
	err := errors.New("fatal: 'feature' is already checked out at '/path'")
	msg := worktreeErrorMessage(err)
	require.Equal(t, "Branch is already checked out in another worktree.", msg)
}

func TestWorktreeErrorMessage_PathExists(t *testing.T) {
	err := errors.New("fatal: '/tmp/worktree' already exists")
	msg := worktreeErrorMessage(err)
	require.Equal(t, "Worktree path already exists.", msg)
}

func TestWorktreeErrorMessage_NotGitRepo(t *testing.T) {
	err := errors.New("fatal: not a git repository (or any of the parent directories): .git")
	msg := worktreeErrorMessage(err)
	require.Equal(t, "Not a git repository. Worktree feature unavailable.", msg)
}

func TestWorktreeErrorMessage_Generic(t *testing.T) {
	err := errors.New("some unknown error")
	msg := worktreeErrorMessage(err)
	require.Equal(t, "Worktree creation failed: some unknown error", msg)
}

func TestWorktreeErrorMessage_NilError(t *testing.T) {
	msg := worktreeErrorMessage(nil)
	require.Empty(t, msg)
}

func TestPhaseToLinkIndex_IncludesWorktree(t *testing.T) {
	// Verify phaseToLinkIndex properly maps InitCreatingWorktree
	require.Equal(t, 0, phaseToLinkIndex(InitCreatingWorktree))
	require.Equal(t, 1, phaseToLinkIndex(InitCreatingWorkspace))
	require.Equal(t, 2, phaseToLinkIndex(InitSpawningCoordinator))
	require.Equal(t, 3, phaseToLinkIndex(InitAwaitingFirstMessage))
	require.Equal(t, 4, phaseToLinkIndex(InitReady))
}

func TestGetPhaseIndicatorAndStyle_Worktree_InProgress(t *testing.T) {
	m := New(Config{})
	m.initializer = newTestInitializerWithWorktree(InitCreatingWorktree, InitNotStarted, nil, "/tmp/test")
	m.spinnerFrame = 0

	// Worktree phase should show spinner when in progress
	indicator, _ := m.getPhaseIndicatorAndStyle(InitCreatingWorktree, InitCreatingWorktree)
	require.Contains(t, indicator, spinnerFrames[0])
}

func TestGetPhaseIndicatorAndStyle_Worktree_Failed(t *testing.T) {
	m := New(Config{})
	m.initializer = newTestInitializerWithWorktree(InitFailed, InitCreatingWorktree, errors.New("test error"), "")

	// Worktree phase should show ✗ when failed
	indicator, _ := m.getPhaseIndicatorAndStyle(InitCreatingWorktree, InitFailed)
	require.Contains(t, indicator, "✗")
}

func TestGetPhaseIndicatorAndStyle_Worktree_Completed(t *testing.T) {
	m := New(Config{})
	m.initializer = newTestInitializerWithWorktree(InitCreatingWorkspace, InitNotStarted, nil, "/tmp/test")

	// Worktree phase should show ✓ when completed (moved past it)
	indicator, _ := m.getPhaseIndicatorAndStyle(InitCreatingWorktree, InitCreatingWorkspace)
	require.Contains(t, indicator, "✓")
}

// ===========================================================================
// worktreeErrorMessage Tests for ErrInvalidBranchName (Task perles-s8xg.4)
// ===========================================================================

func TestWorktreeErrorMessage_InvalidBranchName_SentinelError(t *testing.T) {
	// Test: ErrInvalidBranchName sentinel error is detected via errors.Is
	err := git.ErrInvalidBranchName
	msg := worktreeErrorMessage(err)
	require.Equal(t, "Invalid branch name. Branch names cannot contain spaces, special characters (~^:?*[), or start with a dot.", msg)
}

func TestWorktreeErrorMessage_InvalidBranchName_WrappedSentinelError(t *testing.T) {
	// Test: Wrapped ErrInvalidBranchName is still detected via errors.Is
	err := fmt.Errorf("failed to create worktree: %w", git.ErrInvalidBranchName)
	msg := worktreeErrorMessage(err)
	require.Equal(t, "Invalid branch name. Branch names cannot contain spaces, special characters (~^:?*[), or start with a dot.", msg)
}

func TestWorktreeErrorMessage_InvalidBranchName_StringContains(t *testing.T) {
	// Test: Git error message containing "is not a valid branch name" is detected
	err := errors.New("fatal: 'my branch' is not a valid branch name")
	msg := worktreeErrorMessage(err)
	require.Equal(t, "Invalid branch name. Branch names cannot contain spaces, special characters (~^:?*[), or start with a dot.", msg)
}

// ===========================================================================
// timeoutErrorMessage Tests (Task perles-mo45.6)
// ===========================================================================

func TestTimeoutErrorMessage_AllPhases(t *testing.T) {
	// Test: Verify correct message for each phase
	tests := []struct {
		name     string
		phase    InitPhase
		duration time.Duration
		contains []string
	}{
		{
			name:     "Worktree phase timeout",
			phase:    InitCreatingWorktree,
			duration: 30 * time.Second,
			contains: []string{
				"Worktree creation timed out after 30s",
				"large repository size",
				"SSH key prompts",
				"NFS issues",
				"orchestration.timeouts.worktree_creation",
			},
		},
		{
			name:     "Workspace phase timeout",
			phase:    InitCreatingWorkspace,
			duration: 30 * time.Second,
			contains: []string{
				"Workspace setup timed out after 30s",
				"MCP server",
				"session initialization",
				"orchestration.timeouts.workspace_setup",
			},
		},
		{
			name:     "SpawningCoordinator phase timeout",
			phase:    InitSpawningCoordinator,
			duration: 60 * time.Second,
			contains: []string{
				"Coordinator did not respond within 1m0s",
				"AI service may be overloaded",
				"orchestration.timeouts.coordinator_start",
			},
		},
		{
			name:     "AwaitingFirstMessage phase timeout",
			phase:    InitAwaitingFirstMessage,
			duration: 60 * time.Second,
			contains: []string{
				"Coordinator did not respond within 1m0s",
				"AI service may be overloaded",
				"orchestration.timeouts.coordinator_start",
			},
		},
		{
			name:     "Default phase timeout",
			phase:    InitNotStarted,
			duration: 30 * time.Second,
			contains: []string{
				"Initialization timed out after 30s",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := timeoutErrorMessage(tt.phase, tt.duration)
			for _, expectedContent := range tt.contains {
				require.Contains(t, msg, expectedContent, "Expected message to contain %q", expectedContent)
			}
		})
	}
}

func TestTimeoutErrorMessage_CustomDuration(t *testing.T) {
	// Test: Verify duration is correctly formatted in message
	msg := timeoutErrorMessage(InitCreatingWorktree, 45*time.Second)
	require.Contains(t, msg, "45s")

	msg = timeoutErrorMessage(InitCreatingWorkspace, 15*time.Second)
	require.Contains(t, msg, "15s")

	msg = timeoutErrorMessage(InitAwaitingFirstMessage, 90*time.Second)
	require.Contains(t, msg, "1m30s")
}

func TestWorktreeErrorMessage_TimeoutError(t *testing.T) {
	// Test: context.DeadlineExceeded is detected as timeout
	err := context.DeadlineExceeded
	msg := worktreeErrorMessage(err)
	require.Contains(t, msg, "Worktree creation timed out")
	require.Contains(t, msg, "large repository size")
	require.Contains(t, msg, "SSH key prompts")
	require.Contains(t, msg, "NFS issues")
	require.Contains(t, msg, ".git/index.lock")
	require.Contains(t, msg, "orchestration.timeouts.worktree_creation")
}

func TestWorktreeErrorMessage_WrappedTimeoutError(t *testing.T) {
	// Test: Wrapped context.DeadlineExceeded is detected via errors.Is
	err := fmt.Errorf("git operation failed: %w", context.DeadlineExceeded)
	msg := worktreeErrorMessage(err)
	require.Contains(t, msg, "Worktree creation timed out")
}

func TestGetTimeoutDuration_WithInitializer(t *testing.T) {
	// Test: getTimeoutDuration returns correct duration for each phase
	timeouts := config.TimeoutsConfig{
		WorktreeCreation: 45 * time.Second,
		WorkspaceSetup:   15 * time.Second,
		CoordinatorStart: 90 * time.Second,
	}

	m := New(Config{})
	m.initializer = newTestInitializerWithTimeouts(InitTimedOut, InitCreatingWorktree, nil, timeouts)

	require.Equal(t, 45*time.Second, m.getTimeoutDuration(InitCreatingWorktree))
	require.Equal(t, 15*time.Second, m.getTimeoutDuration(InitCreatingWorkspace))
	require.Equal(t, 90*time.Second, m.getTimeoutDuration(InitSpawningCoordinator))
	require.Equal(t, 90*time.Second, m.getTimeoutDuration(InitAwaitingFirstMessage))
}

func TestGetTimeoutDuration_NilInitializer(t *testing.T) {
	// Test: getTimeoutDuration returns default 30s when initializer is nil
	m := New(Config{})
	// Initializer is nil by default

	require.Equal(t, 30*time.Second, m.getTimeoutDuration(InitCreatingWorktree))
	require.Equal(t, 30*time.Second, m.getTimeoutDuration(InitCreatingWorkspace))
}

// --- Golden tests for timeout error messages ---

func TestView_Golden_LoadingTimedOutWorktreePhase(t *testing.T) {
	timeouts := config.TimeoutsConfig{
		WorktreeCreation: 30 * time.Second,
		WorkspaceSetup:   30 * time.Second,
		CoordinatorStart: 60 * time.Second,
	}
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.initializer = newTestInitializerWithTimeouts(InitTimedOut, InitCreatingWorktree, nil, timeouts)
	m.worktreeEnabled = true

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_LoadingTimedOutWorkspacePhase(t *testing.T) {
	timeouts := config.TimeoutsConfig{
		WorktreeCreation: 30 * time.Second,
		WorkspaceSetup:   30 * time.Second,
		CoordinatorStart: 60 * time.Second,
	}
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.initializer = newTestInitializerWithTimeouts(InitTimedOut, InitCreatingWorkspace, nil, timeouts)
	m.worktreeEnabled = true

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_LoadingTimedOutCoordinatorPhase(t *testing.T) {
	timeouts := config.TimeoutsConfig{
		WorktreeCreation: 30 * time.Second,
		WorkspaceSetup:   30 * time.Second,
		CoordinatorStart: 60 * time.Second,
	}
	m := New(Config{})
	m = m.SetSize(120, 30)
	m.initializer = newTestInitializerWithTimeouts(InitTimedOut, InitAwaitingFirstMessage, nil, timeouts)
	m.worktreeEnabled = true

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}
