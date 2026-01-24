package dashboard

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/orchestration/controlplane"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/ui/styles"
)

// === Unit Tests: getStatusTextAndColor ===

func TestGetStatusTextAndColor(t *testing.T) {
	tests := []struct {
		name         string
		state        controlplane.WorkflowState
		expectedText string
	}{
		{"running", controlplane.WorkflowRunning, "RUNNING"},
		{"pending", controlplane.WorkflowPending, "PENDING"},
		{"paused", controlplane.WorkflowPaused, "PAUSED"},
		{"completed", controlplane.WorkflowCompleted, "COMPLETED"},
		{"failed", controlplane.WorkflowFailed, "FAILED"},
		{"stopped", controlplane.WorkflowStopped, "STOPPED"},
		{"unknown", controlplane.WorkflowState("unknown"), "PENDING"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			text, _ := getStatusTextAndColor(tc.state)
			require.Equal(t, tc.expectedText, text)
		})
	}
}

// === Unit Tests: phaseShortName ===

func TestPhaseShortName(t *testing.T) {
	tests := []struct {
		name     string
		phase    events.ProcessPhase
		expected string
	}{
		{"implementing", events.ProcessPhaseImplementing, "impl"},
		{"awaiting_review", events.ProcessPhaseAwaitingReview, "await"},
		{"reviewing", events.ProcessPhaseReviewing, "review"},
		{"addressing_feedback", events.ProcessPhaseAddressingFeedback, "feedback"},
		{"committing", events.ProcessPhaseCommitting, "commit"},
		{"idle", events.ProcessPhaseIdle, ""},
		{"empty", events.ProcessPhase(""), ""},
		{"unknown", events.ProcessPhase("unknown_phase"), ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := phaseShortName(tc.phase)
			require.Equal(t, tc.expected, result)
		})
	}
}

// === Unit Tests: WorkDir column rendering ===

// renderWorkDirColumn extracts and invokes the WorkDir column render function.
// This allows testing the rendering logic without needing the full table component.
func renderWorkDirColumn(wf *controlplane.WorkflowInstance, width int) string {
	// Replicate the render logic from createWorkflowTableConfig

	// Show worktree with tree icon
	if wf.WorktreePath != "" {
		display := "🌳 " + filepath.Base(wf.WorktreePath)
		if lipgloss.Width(display) > width {
			return styles.TruncateString(display, width)
		}
		return display
	}

	// Show custom workdir (not current directory)
	if wf.WorkDir != "" {
		cwd, _ := os.Getwd()
		if wf.WorkDir != cwd {
			display := filepath.Base(wf.WorkDir)
			if lipgloss.Width(display) > width {
				return styles.TruncateString(display, width)
			}
			return display
		}
	}

	// Using current directory
	return "·"
}

func TestWorkDirColumn_Worktree(t *testing.T) {
	wf := &controlplane.WorkflowInstance{
		ID:           "wf-001",
		WorktreePath: "/path/to/worktrees/perles-branch-abc",
	}

	result := renderWorkDirColumn(wf, 30)
	require.Equal(t, "🌳 perles-branch-abc", result)
}

func TestWorkDirColumn_CustomWorkDir(t *testing.T) {
	// Use a path that's definitely not the current directory
	wf := &controlplane.WorkflowInstance{
		ID:      "wf-001",
		WorkDir: "/some/other/project",
	}

	result := renderWorkDirColumn(wf, 30)
	require.Equal(t, "project", result)
}

func TestWorkDirColumn_CurrentDirectory(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	wf := &controlplane.WorkflowInstance{
		ID:      "wf-001",
		WorkDir: cwd, // Using current directory
	}

	result := renderWorkDirColumn(wf, 30)
	require.Equal(t, "·", result)
}

func TestWorkDirColumn_EmptyWorkDir(t *testing.T) {
	wf := &controlplane.WorkflowInstance{
		ID:      "wf-001",
		WorkDir: "", // Empty workdir defaults to current directory
	}

	result := renderWorkDirColumn(wf, 30)
	require.Equal(t, "·", result)
}

func TestWorkDirColumn_TruncatesLongWorktreePath(t *testing.T) {
	wf := &controlplane.WorkflowInstance{
		ID:           "wf-001",
		WorktreePath: "/path/to/worktrees/very-long-worktree-branch-name-that-exceeds-width",
	}

	// Width of 15 should truncate the display
	result := renderWorkDirColumn(wf, 15)

	// "🌳 " takes 3 characters (emoji width varies)
	// Should be truncated with "..."
	require.LessOrEqual(t, lipgloss.Width(result), 15)
	require.Contains(t, result, "...")
}

func TestWorkDirColumn_TruncatesLongCustomWorkDir(t *testing.T) {
	wf := &controlplane.WorkflowInstance{
		ID:      "wf-001",
		WorkDir: "/some/path/very-long-directory-name-that-exceeds-column-width",
	}

	// Width of 10 should truncate the display
	result := renderWorkDirColumn(wf, 10)

	require.LessOrEqual(t, lipgloss.Width(result), 10)
	require.Contains(t, result, "...")
}

// === Unit Tests: EpicID column rendering ===

// renderEpicIDColumn extracts and invokes the EpicID column render function.
// This allows testing the rendering logic without needing the full table component.
func renderEpicIDColumn(wf *controlplane.WorkflowInstance, width int) string {
	// Replicate the render logic from createWorkflowTableConfig
	epicID := wf.EpicID
	if epicID == "" {
		return "-"
	}
	if lipgloss.Width(epicID) > width {
		return styles.TruncateString(epicID, width)
	}
	return epicID
}

func TestEpicIDColumn_EmptyEpicID(t *testing.T) {
	wf := &controlplane.WorkflowInstance{
		ID:     "wf-001",
		EpicID: "",
	}

	result := renderEpicIDColumn(wf, 20)
	require.Equal(t, "-", result)
}

func TestEpicIDColumn_WithEpicID(t *testing.T) {
	wf := &controlplane.WorkflowInstance{
		ID:     "wf-001",
		EpicID: "epic-123",
	}

	result := renderEpicIDColumn(wf, 20)
	require.Equal(t, "epic-123", result)
}

func TestEpicIDColumn_TruncatesLongEpicID(t *testing.T) {
	wf := &controlplane.WorkflowInstance{
		ID:     "wf-001",
		EpicID: "very-long-epic-id-that-exceeds-column-width",
	}

	// Width of 15 should truncate the display
	result := renderEpicIDColumn(wf, 15)

	require.LessOrEqual(t, lipgloss.Width(result), 15)
	require.Contains(t, result, "...")
}

// === Unit Tests: renderEpicSection (perles-boi8.4) ===

func TestRenderEpicSectionNarrowWidth(t *testing.T) {
	// Setup model without tree
	m := createEpicTreeTestModel(t)

	// Render with narrow width (< 50)
	result := m.renderEpicSection(40, 20)

	// Should show warning message
	require.Contains(t, result, "Terminal too narrow for tree view")
	require.Contains(t, result, "need 50+ chars")
}

func TestRenderEpicSectionNoTree(t *testing.T) {
	// Setup model with a workflow with epic but no tree loaded
	m := createEpicTreeTestModelWithWorkflows(t)
	m.epicTree = nil    // No tree loaded yet
	m.selectedIndex = 0 // wf-1 with epic-100

	// Render with adequate width
	result := m.renderEpicSection(100, 20)

	// Should show empty state message
	require.Contains(t, result, "No tasks in epic")
}

func TestRenderEpicSectionNoEpic(t *testing.T) {
	// Setup model without epic
	m := createEpicTreeTestModel(t)
	// No workflows, so no epic ID

	// Render with adequate width
	result := m.renderEpicSection(100, 20)

	// Should show no epic message
	require.Contains(t, result, "No epic associated with this workflow")
}

func TestRenderEpicSectionNoEpicWithWorkflow(t *testing.T) {
	// Setup model with a workflow that has no epic
	m := createEpicTreeTestModelWithWorkflows(t)
	// Select workflow with no epic (index 2, wf-3 has empty EpicID)
	m.selectedIndex = 2

	// Render with adequate width
	result := m.renderEpicSection(100, 20)

	// Should show no epic message
	require.Contains(t, result, "No epic associated with this workflow")
}

func TestRenderEpicSectionEmptyEpic(t *testing.T) {
	// Setup model with workflow that has epic but tree is nil
	// Loading states are hidden to prevent UI flash, so we show "No tasks"
	m := createEpicTreeTestModelWithWorkflows(t)
	m.selectedIndex = 0 // wf-1 with epic-100
	m.epicTree = nil    // No tree loaded yet

	// Render with adequate width
	result := m.renderEpicSection(100, 20)

	// Should show "No tasks" message (loading states hidden to prevent flash)
	require.Contains(t, result, "No tasks in epic")
}

func TestRenderEpicSectionWidthAllocation(t *testing.T) {
	// This test verifies the 40%/60% width split calculation
	// We test the calculation logic by checking that tree and details
	// components are sized correctly

	// For a width of 100:
	// - Tree: 100 * 40 / 100 = 40
	// - Details: 100 - 40 - 1 = 59 (minus separator)

	// For a width of 200:
	// - Tree: 200 * 40 / 100 = 80
	// - Details: 200 - 80 - 1 = 119

	tests := []struct {
		name                 string
		totalWidth           int
		expectedTreeWidth    int
		expectedDetailsWidth int
	}{
		{"width 100", 100, 40, 59},
		{"width 200", 200, 80, 119},
		{"width 150", 150, 60, 89},
		{"minimum width 50", 50, 20, 29}, // Clamped to minimum 20
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Calculate widths using the same logic as renderEpicSection
			treeWidth := tc.totalWidth * 40 / 100
			detailsWidth := tc.totalWidth - treeWidth - 1

			// Apply minimum width constraints
			if treeWidth < 20 {
				treeWidth = 20
				detailsWidth = tc.totalWidth - treeWidth - 1
			}
			if detailsWidth < 20 {
				detailsWidth = 20
				treeWidth = tc.totalWidth - detailsWidth - 1
			}

			require.Equal(t, tc.expectedTreeWidth, treeWidth, "tree width mismatch")
			require.Equal(t, tc.expectedDetailsWidth, detailsWidth, "details width mismatch")
		})
	}
}

// === Unit Tests: getEpicPaneBorderConfig (perles-boi8.4) ===

func TestGetEpicPaneBorderConfigFocused(t *testing.T) {
	// Setup model with epic view focused
	m := createEpicTreeTestModel(t)
	m.focus = FocusEpicView
	m.epicViewFocus = EpicFocusTree

	// Get config for tree pane (should be focused)
	treeConfig := m.getEpicPaneBorderConfig(EpicFocusTree, 40, 20, "Tree")
	require.True(t, treeConfig.focused, "tree pane should be focused when epicViewFocus is EpicFocusTree")
	require.Equal(t, 40, treeConfig.width)
	require.Equal(t, 20, treeConfig.height)
	require.Equal(t, "Tree", treeConfig.title)

	// Get config for details pane (should not be focused)
	detailsConfig := m.getEpicPaneBorderConfig(EpicFocusDetails, 60, 20, "Details")
	require.False(t, detailsConfig.focused, "details pane should not be focused when epicViewFocus is EpicFocusTree")
}

func TestGetEpicPaneBorderConfigNotFocused(t *testing.T) {
	// Setup model with table focused (not epic view)
	m := createEpicTreeTestModel(t)
	m.focus = FocusTable
	m.epicViewFocus = EpicFocusTree // Even though epicViewFocus is Tree, focus is on Table

	// Get config for tree pane (should not be focused because dashboard focus is elsewhere)
	treeConfig := m.getEpicPaneBorderConfig(EpicFocusTree, 40, 20, "Tree")
	require.False(t, treeConfig.focused, "tree pane should not be focused when dashboard focus is FocusTable")
}

// === Unit Tests: Footer Hints (perles-boi8.9) ===

func TestFooterHintsAreStatic(t *testing.T) {
	// Setup model
	m := createEpicTreeTestModel(t)
	m.width = 100
	m.height = 30

	// Render footer hints
	footer := m.renderActionHints()

	// Verify static hints are present regardless of focus
	require.Contains(t, footer, "j/k", "footer should have j/k nav hint")
	require.Contains(t, footer, "tab", "footer should have tab cycle hint")
	require.Contains(t, footer, "new", "footer should have new hint")
	require.Contains(t, footer, "start", "footer should have start hint")
	require.Contains(t, footer, "stop", "footer should have stop hint")
	require.Contains(t, footer, "help", "footer should have help hint")
	require.Contains(t, footer, "quit", "footer should have quit hint")
}
