package orchestration

import (
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/pool"
)

func TestPhaseShortName(t *testing.T) {
	tests := []struct {
		name     string
		phase    events.WorkerPhase
		expected string
	}{
		{"idle returns empty", events.PhaseIdle, ""},
		{"implementing returns impl", events.PhaseImplementing, "impl"},
		{"awaiting review returns await", events.PhaseAwaitingReview, "await"},
		{"reviewing returns review", events.PhaseReviewing, "review"},
		{"addressing feedback returns feedback", events.PhaseAddressingFeedback, "feedback"},
		{"committing returns commit", events.PhaseCommitting, "commit"},
		{"unknown phase returns empty", events.WorkerPhase("unknown"), ""},
		{"empty phase returns empty", events.WorkerPhase(""), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := phaseShortName(tt.phase)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatWorkerTitle_WithTaskAndPhase(t *testing.T) {
	title := formatWorkerTitle("worker-1", pool.WorkerWorking, "perles-abc.1", events.PhaseImplementing)

	// Should contain worker ID in uppercase
	require.Contains(t, title, "WORKER-1")
	// Should contain task ID
	require.Contains(t, title, "perles-abc.1")
	// Should contain phase short name in parentheses
	require.Contains(t, title, "(impl)")
}

func TestFormatWorkerTitle_WithTaskNoPhase(t *testing.T) {
	// Task assigned but phase is idle
	title := formatWorkerTitle("worker-2", pool.WorkerWorking, "perles-xyz.5", events.PhaseIdle)

	require.Contains(t, title, "WORKER-2")
	require.Contains(t, title, "perles-xyz.5")
	// Should NOT contain parentheses when phase is idle
	require.NotContains(t, title, "(")
}

func TestFormatWorkerTitle_Idle(t *testing.T) {
	// Ready worker with no task
	title := formatWorkerTitle("worker-3", pool.WorkerReady, "", events.PhaseIdle)

	require.Contains(t, title, "WORKER-3")
	// Should NOT contain task ID or phase
	require.NotContains(t, title, "perles")
	require.NotContains(t, title, "(")
}

func TestFormatWorkerTitle_Retired(t *testing.T) {
	// Retired worker
	title := formatWorkerTitle("worker-4", pool.WorkerRetired, "", events.PhaseIdle)

	require.Contains(t, title, "WORKER-4")
	// Should NOT contain task info
	require.NotContains(t, title, "(")
}

func TestFormatWorkerTitle_AllPhases(t *testing.T) {
	// Test all phases produce expected short names
	phases := []struct {
		phase    events.WorkerPhase
		expected string
	}{
		{events.PhaseImplementing, "(impl)"},
		{events.PhaseAwaitingReview, "(await)"},
		{events.PhaseReviewing, "(review)"},
		{events.PhaseAddressingFeedback, "(feedback)"},
		{events.PhaseCommitting, "(commit)"},
	}

	for _, tt := range phases {
		t.Run(string(tt.phase), func(t *testing.T) {
			title := formatWorkerTitle("worker-1", pool.WorkerWorking, "task-123", tt.phase)
			require.Contains(t, title, tt.expected)
		})
	}
}

func TestFormatWorkerTitle_UnknownPhase(t *testing.T) {
	// Unknown phase should be handled gracefully (no parentheses)
	title := formatWorkerTitle("worker-1", pool.WorkerWorking, "task-123", events.WorkerPhase("unknown_phase"))

	require.Contains(t, title, "WORKER-1")
	require.Contains(t, title, "task-123")
	// Unknown phase should not produce parentheses
	require.NotContains(t, title, "(")
}

func TestRenderSingleWorkerPane_NilPoolDoesNotCrash(t *testing.T) {
	// Create model with nil pool
	m := New(Config{})
	m.pool = nil // Explicitly ensure pool is nil

	// Initialize worker pane state for the worker
	m.workerPane.workerStatus["worker-1"] = pool.WorkerWorking
	m.workerPane.viewports = make(map[string]viewport.Model)

	// Should not panic when pool is nil
	require.NotPanics(t, func() {
		_ = m.renderSingleWorkerPane("worker-1", 80, 20)
	})
}
