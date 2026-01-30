package events

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProcessWorkflowComplete_ConstantValue(t *testing.T) {
	// Verify ProcessWorkflowComplete has the correct string value
	require.Equal(t, "workflow_complete", string(ProcessWorkflowComplete))
}

func TestProcessWorkflowComplete_UsableInProcessEvent(t *testing.T) {
	// Verify the event type can be used in ProcessEvent struct
	event := ProcessEvent{
		Type:      ProcessWorkflowComplete,
		ProcessID: "coordinator-1",
		Role:      RoleCoordinator,
	}

	require.Equal(t, ProcessWorkflowComplete, event.Type)
	require.Equal(t, "workflow_complete", string(event.Type))
}

func TestProcessAutoRefreshRequired_ConstantValue(t *testing.T) {
	// Verify ProcessAutoRefreshRequired has the correct string value
	require.Equal(t, "auto_refresh_required", string(ProcessAutoRefreshRequired))
}

func TestProcessAutoRefreshRequired_UsableInProcessEvent(t *testing.T) {
	// Verify the event type can be used in ProcessEvent struct
	event := ProcessEvent{
		Type:      ProcessAutoRefreshRequired,
		ProcessID: "coordinator-1",
		Role:      RoleCoordinator,
	}

	require.Equal(t, ProcessAutoRefreshRequired, event.Type)
	require.Equal(t, "auto_refresh_required", string(event.Type))
}

func TestProcessPhase_Values(t *testing.T) {
	// Verify all ProcessPhase constants have correct string values
	tests := []struct {
		phase    ProcessPhase
		expected string
	}{
		{ProcessPhaseIdle, "idle"},
		{ProcessPhaseImplementing, "implementing"},
		{ProcessPhaseAwaitingReview, "awaiting_review"},
		{ProcessPhaseReviewing, "reviewing"},
		{ProcessPhaseAddressingFeedback, "addressing_feedback"},
		{ProcessPhaseCommitting, "committing"},
	}

	for _, tt := range tests {
		t.Run(string(tt.phase), func(t *testing.T) {
			require.Equal(t, tt.expected, string(tt.phase))
		})
	}
}

func TestProcessPhase_AllPhasesAreDefined(t *testing.T) {
	// Verify we have exactly 6 phases as specified in the proposal
	phases := []ProcessPhase{
		ProcessPhaseIdle,
		ProcessPhaseImplementing,
		ProcessPhaseAwaitingReview,
		ProcessPhaseReviewing,
		ProcessPhaseAddressingFeedback,
		ProcessPhaseCommitting,
	}

	// Each phase should be distinct
	seen := make(map[ProcessPhase]bool)
	for _, phase := range phases {
		require.False(t, seen[phase], "Duplicate phase: %s", phase)
		seen[phase] = true
	}

	require.Len(t, phases, 6, "Expected exactly 6 workflow phases")
}

func TestProcessStatusRetiring_ConstantValue(t *testing.T) {
	// Verify ProcessStatusRetiring has the correct string value
	require.Equal(t, "retiring", string(ProcessStatusRetiring))
}

func TestProcessStatusRetiring_IsTerminal(t *testing.T) {
	// Verify IsTerminal() returns false for Retiring status
	// Retiring is an intermediate state, not a terminal state
	require.False(t, ProcessStatusRetiring.IsTerminal(), "Retiring should NOT be a terminal status")
}

func TestProcessStatus_IsTerminal(t *testing.T) {
	// Verify IsTerminal() returns correct values for all statuses
	tests := []struct {
		status     ProcessStatus
		isTerminal bool
	}{
		{ProcessStatusPending, false},
		{ProcessStatusStarting, false},
		{ProcessStatusReady, false},
		{ProcessStatusWorking, false},
		{ProcessStatusPaused, false},
		{ProcessStatusStopped, false},
		{ProcessStatusRetiring, false}, // Retiring is NOT terminal - coordinator is still active
		{ProcessStatusRetired, true},
		{ProcessStatusFailed, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			require.Equal(t, tt.isTerminal, tt.status.IsTerminal(),
				"IsTerminal() for %s should be %v", tt.status, tt.isTerminal)
		})
	}
}

func TestRoleObserver_Constant(t *testing.T) {
	// Verify RoleObserver constant exists with value "observer"
	require.Equal(t, "observer", string(RoleObserver))
}

func TestIsObserver_True(t *testing.T) {
	// Verify IsObserver() returns true when Role == RoleObserver
	event := ProcessEvent{
		Type:      ProcessOutput,
		ProcessID: "observer",
		Role:      RoleObserver,
	}

	require.True(t, event.IsObserver())
	require.False(t, event.IsCoordinator())
	require.False(t, event.IsWorker())
}

func TestIsObserver_False(t *testing.T) {
	// Verify IsObserver() returns false for Coordinator/Worker roles
	tests := []struct {
		name string
		role ProcessRole
	}{
		{"coordinator", RoleCoordinator},
		{"worker", RoleWorker},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := ProcessEvent{
				Type:      ProcessOutput,
				ProcessID: tt.name,
				Role:      tt.role,
			}
			require.False(t, event.IsObserver(), "IsObserver() should return false for %s role", tt.name)
		})
	}
}
