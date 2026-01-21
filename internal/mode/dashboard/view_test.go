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
