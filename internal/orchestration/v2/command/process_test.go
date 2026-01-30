package command

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/orchestration/metrics"
	"github.com/zjrosen/perles/internal/orchestration/v2/prompt/roles"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
)

// ===========================================================================
// SpawnProcessCommand Tests
// ===========================================================================

func TestSpawnProcessCommand_Type(t *testing.T) {
	cmd := NewSpawnProcessCommand(SourceMCPTool, repository.RoleWorker)
	require.Equal(t, CmdSpawnProcess, cmd.Type())
}

func TestSpawnProcessCommand_Validate_RoleCoordinator(t *testing.T) {
	cmd := NewSpawnProcessCommand(SourceMCPTool, repository.RoleCoordinator)
	require.NoError(t, cmd.Validate())
}

func TestSpawnProcessCommand_Validate_RoleWorker(t *testing.T) {
	cmd := NewSpawnProcessCommand(SourceMCPTool, repository.RoleWorker)
	require.NoError(t, cmd.Validate())
}

func TestSpawnProcessCommand_Validate_RoleObserver(t *testing.T) {
	cmd := NewSpawnProcessCommand(SourceMCPTool, repository.RoleObserver)
	require.NoError(t, cmd.Validate())
}

func TestSpawnProcessCommand_Validate_InvalidRole(t *testing.T) {
	cmd := NewSpawnProcessCommand(SourceMCPTool, "invalid_role")
	err := cmd.Validate()
	require.Error(t, err)
	require.Equal(t, "role must be coordinator, worker, or observer, got: invalid_role", err.Error())
}

func TestSpawnProcessCommand_Validate_EmptyRole(t *testing.T) {
	cmd := NewSpawnProcessCommand(SourceMCPTool, "")
	err := cmd.Validate()
	require.Error(t, err)
	require.Equal(t, "role must be coordinator, worker, or observer, got: ", err.Error())
}

func TestSpawnProcessCommand_PreservesProcessID(t *testing.T) {
	cmd := NewSpawnProcessCommand(SourceMCPTool, repository.RoleWorker)
	cmd.ProcessID = "custom-worker-id"
	require.Equal(t, "custom-worker-id", cmd.ProcessID)
}

func TestSpawnProcessCommand_EmptyProcessID(t *testing.T) {
	cmd := NewSpawnProcessCommand(SourceMCPTool, repository.RoleWorker)
	require.Empty(t, cmd.ProcessID)
	// Should still validate successfully (auto-generated for workers if empty)
	require.NoError(t, cmd.Validate())
}

func TestSpawnProcessCommand_ImplementsCommand(t *testing.T) {
	var _ Command = &SpawnProcessCommand{}
}

func TestSpawnProcessCommand_WithAgentType(t *testing.T) {
	cmd := NewSpawnProcessCommand(SourceMCPTool, repository.RoleWorker, WithAgentType(roles.AgentTypeImplementer))
	require.Equal(t, roles.AgentTypeImplementer, cmd.AgentType)
}

func TestSpawnProcessCommand_DefaultAgentType(t *testing.T) {
	cmd := NewSpawnProcessCommand(SourceMCPTool, repository.RoleWorker)
	require.Equal(t, roles.AgentTypeGeneric, cmd.AgentType)
}

func TestSpawnProcessCommand_WithAgentType_AllTypes(t *testing.T) {
	testCases := []struct {
		name      string
		agentType roles.AgentType
	}{
		{"generic", roles.AgentTypeGeneric},
		{"implementer", roles.AgentTypeImplementer},
		{"reviewer", roles.AgentTypeReviewer},
		{"researcher", roles.AgentTypeResearcher},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := NewSpawnProcessCommand(SourceMCPTool, repository.RoleWorker, WithAgentType(tc.agentType))
			require.Equal(t, tc.agentType, cmd.AgentType)
			require.NoError(t, cmd.Validate())
		})
	}
}

func TestSpawnProcessCommand_WithAgentType_PreservesOtherFields(t *testing.T) {
	cmd := NewSpawnProcessCommand(SourceUser, repository.RoleCoordinator, WithAgentType(roles.AgentTypeReviewer))
	require.Equal(t, SourceUser, cmd.Source())
	require.Equal(t, repository.RoleCoordinator, cmd.Role)
	require.Equal(t, roles.AgentTypeReviewer, cmd.AgentType)
}

// ===========================================================================
// RetireProcessCommand Tests
// ===========================================================================

func TestRetireProcessCommand_Type(t *testing.T) {
	cmd := NewRetireProcessCommand(SourceMCPTool, "worker-1", "scaling down")
	require.Equal(t, CmdRetireProcess, cmd.Type())
}

func TestRetireProcessCommand_Validate_Valid(t *testing.T) {
	cmd := NewRetireProcessCommand(SourceMCPTool, "worker-1", "scaling down")
	require.NoError(t, cmd.Validate())
}

func TestRetireProcessCommand_Validate_EmptyProcessID(t *testing.T) {
	cmd := NewRetireProcessCommand(SourceMCPTool, "", "scaling down")
	err := cmd.Validate()
	require.Error(t, err)
	require.Equal(t, "process_id is required", err.Error())
}

func TestRetireProcessCommand_StoresReason(t *testing.T) {
	reason := "context window exceeded"
	cmd := NewRetireProcessCommand(SourceMCPTool, "worker-1", reason)
	require.Equal(t, reason, cmd.Reason)
}

func TestRetireProcessCommand_EmptyReason(t *testing.T) {
	cmd := NewRetireProcessCommand(SourceMCPTool, "worker-1", "")
	// Empty reason is valid
	require.NoError(t, cmd.Validate())
}

func TestRetireProcessCommand_ImplementsCommand(t *testing.T) {
	var _ Command = &RetireProcessCommand{}
}

// ===========================================================================
// ReplaceProcessCommand Tests
// ===========================================================================

func TestReplaceProcessCommand_Type(t *testing.T) {
	cmd := NewReplaceProcessCommand(SourceMCPTool, "coordinator", "context refresh")
	require.Equal(t, CmdReplaceProcess, cmd.Type())
}

func TestReplaceProcessCommand_Validate_Valid(t *testing.T) {
	cmd := NewReplaceProcessCommand(SourceMCPTool, "coordinator", "context refresh")
	require.NoError(t, cmd.Validate())
}

func TestReplaceProcessCommand_Validate_EmptyProcessID(t *testing.T) {
	cmd := NewReplaceProcessCommand(SourceMCPTool, "", "context refresh")
	err := cmd.Validate()
	require.Error(t, err)
	require.Equal(t, "process_id is required", err.Error())
}

func TestReplaceProcessCommand_StoresReason(t *testing.T) {
	reason := "context window full"
	cmd := NewReplaceProcessCommand(SourceMCPTool, "coordinator", reason)
	require.Equal(t, reason, cmd.Reason)
}

func TestReplaceProcessCommand_ImplementsCommand(t *testing.T) {
	var _ Command = &ReplaceProcessCommand{}
}

// ===========================================================================
// SendToProcessCommand Tests
// ===========================================================================

func TestSendToProcessCommand_Type(t *testing.T) {
	cmd := NewSendToProcessCommand(SourceUser, "coordinator", "Hello")
	require.Equal(t, CmdSendToProcess, cmd.Type())
}

func TestSendToProcessCommand_Validate_Valid(t *testing.T) {
	cmd := NewSendToProcessCommand(SourceUser, "worker-1", "Hello, worker!")
	require.NoError(t, cmd.Validate())
}

func TestSendToProcessCommand_Validate_EmptyProcessID(t *testing.T) {
	cmd := NewSendToProcessCommand(SourceUser, "", "Hello")
	err := cmd.Validate()
	require.Error(t, err)
	require.Equal(t, "process_id is required", err.Error())
}

func TestSendToProcessCommand_Validate_EmptyContent(t *testing.T) {
	cmd := NewSendToProcessCommand(SourceUser, "worker-1", "")
	err := cmd.Validate()
	require.Error(t, err)
	require.Equal(t, "content is required", err.Error())
}

func TestSendToProcessCommand_WorksWithCoordinator(t *testing.T) {
	cmd := NewSendToProcessCommand(SourceMCPTool, "coordinator", "Work on task")
	require.NoError(t, cmd.Validate())
	require.Equal(t, "coordinator", cmd.ProcessID)
}

func TestSendToProcessCommand_WorksWithWorker(t *testing.T) {
	cmd := NewSendToProcessCommand(SourceMCPTool, "worker-1", "Your task assignment")
	require.NoError(t, cmd.Validate())
	require.Equal(t, "worker-1", cmd.ProcessID)
}

func TestSendToProcessCommand_ImplementsCommand(t *testing.T) {
	var _ Command = &SendToProcessCommand{}
}

// ===========================================================================
// DeliverProcessQueuedCommand Tests
// ===========================================================================

func TestDeliverProcessQueuedCommand_Type(t *testing.T) {
	cmd := NewDeliverProcessQueuedCommand(SourceInternal, "worker-1")
	require.Equal(t, CmdDeliverProcessQueued, cmd.Type())
}

func TestDeliverProcessQueuedCommand_Validate_Valid(t *testing.T) {
	cmd := NewDeliverProcessQueuedCommand(SourceInternal, "worker-1")
	require.NoError(t, cmd.Validate())
}

func TestDeliverProcessQueuedCommand_Validate_EmptyProcessID(t *testing.T) {
	cmd := NewDeliverProcessQueuedCommand(SourceInternal, "")
	err := cmd.Validate()
	require.Error(t, err)
	require.Equal(t, "process_id is required", err.Error())
}

func TestDeliverProcessQueuedCommand_ImplementsCommand(t *testing.T) {
	var _ Command = &DeliverProcessQueuedCommand{}
}

// ===========================================================================
// ProcessTurnCompleteCommand Tests
// ===========================================================================

func TestProcessTurnCompleteCommand_Type(t *testing.T) {
	cmd := NewProcessTurnCompleteCommand("worker-1", true, nil, nil)
	require.Equal(t, CmdProcessTurnComplete, cmd.Type())
}

func TestProcessTurnCompleteCommand_Source_IsCallback(t *testing.T) {
	cmd := NewProcessTurnCompleteCommand("worker-1", true, nil, nil)
	require.Equal(t, SourceCallback, cmd.Source())
}

func TestProcessTurnCompleteCommand_Validate_Valid(t *testing.T) {
	cmd := NewProcessTurnCompleteCommand("worker-1", true, nil, nil)
	require.NoError(t, cmd.Validate())
}

func TestProcessTurnCompleteCommand_Validate_EmptyProcessID(t *testing.T) {
	cmd := NewProcessTurnCompleteCommand("", true, nil, nil)
	err := cmd.Validate()
	require.Error(t, err)
	require.Equal(t, "process_id is required", err.Error())
}

func TestProcessTurnCompleteCommand_StoresSucceeded_True(t *testing.T) {
	cmd := NewProcessTurnCompleteCommand("worker-1", true, nil, nil)
	require.True(t, cmd.Succeeded)
}

func TestProcessTurnCompleteCommand_StoresSucceeded_False(t *testing.T) {
	cmd := NewProcessTurnCompleteCommand("worker-1", false, nil, nil)
	require.False(t, cmd.Succeeded)
}

func TestProcessTurnCompleteCommand_StoresMetrics(t *testing.T) {
	m := &metrics.TokenMetrics{
		TokensUsed:   1000,
		OutputTokens: 500,
		TotalCostUSD: 0.05,
		TotalTokens:  200000,
	}
	cmd := NewProcessTurnCompleteCommand("worker-1", true, m, nil)
	require.NotNil(t, cmd.Metrics)
	require.Equal(t, 1000, cmd.Metrics.TokensUsed)
	require.Equal(t, 500, cmd.Metrics.OutputTokens)
}

func TestProcessTurnCompleteCommand_StoresError(t *testing.T) {
	testErr := errors.New("process failed")
	cmd := NewProcessTurnCompleteCommand("worker-1", false, nil, testErr)
	require.NotNil(t, cmd.Error)
	require.Equal(t, "process failed", cmd.Error.Error())
}

func TestProcessTurnCompleteCommand_HandlesNilMetrics(t *testing.T) {
	cmd := NewProcessTurnCompleteCommand("worker-1", true, nil, nil)
	require.Nil(t, cmd.Metrics)
	// Should validate successfully even with nil metrics
	require.NoError(t, cmd.Validate())
}

func TestProcessTurnCompleteCommand_ImplementsCommand(t *testing.T) {
	var _ Command = &ProcessTurnCompleteCommand{}
}

// ===========================================================================
// All Process Commands Implement Command Interface
// ===========================================================================

func TestAllProcessCommandsImplementCommandInterface(t *testing.T) {
	commands := []Command{
		&SpawnProcessCommand{},
		&RetireProcessCommand{},
		&ReplaceProcessCommand{},
		&SendToProcessCommand{},
		&DeliverProcessQueuedCommand{},
		&ProcessTurnCompleteCommand{},
	}

	require.Len(t, commands, 6)
}

// ===========================================================================
// Empty String Validation Tests for Process Commands
// ===========================================================================

func TestProcessCommand_EmptyStringValidation(t *testing.T) {
	testCases := []struct {
		name      string
		cmd       Command
		expectErr bool
	}{
		{"SpawnProcess empty Role", NewSpawnProcessCommand(SourceMCPTool, ""), true},
		{"SpawnProcess invalid Role", NewSpawnProcessCommand(SourceMCPTool, "invalid"), true},
		{"SpawnProcess valid RoleCoordinator", NewSpawnProcessCommand(SourceMCPTool, repository.RoleCoordinator), false},
		{"SpawnProcess valid RoleWorker", NewSpawnProcessCommand(SourceMCPTool, repository.RoleWorker), false},
		{"RetireProcess empty ProcessID", NewRetireProcessCommand(SourceMCPTool, "", "reason"), true},
		{"RetireProcess valid ProcessID", NewRetireProcessCommand(SourceMCPTool, "worker-1", "reason"), false},
		{"ReplaceProcess empty ProcessID", NewReplaceProcessCommand(SourceMCPTool, "", "reason"), true},
		{"ReplaceProcess valid ProcessID", NewReplaceProcessCommand(SourceMCPTool, "coordinator", "reason"), false},
		{"SendToProcess empty ProcessID", NewSendToProcessCommand(SourceUser, "", "content"), true},
		{"SendToProcess empty Content", NewSendToProcessCommand(SourceUser, "worker-1", ""), true},
		{"SendToProcess valid", NewSendToProcessCommand(SourceUser, "worker-1", "content"), false},
		{"DeliverProcessQueued empty ProcessID", NewDeliverProcessQueuedCommand(SourceInternal, ""), true},
		{"DeliverProcessQueued valid ProcessID", NewDeliverProcessQueuedCommand(SourceInternal, "worker-1"), false},
		{"ProcessTurnComplete empty ProcessID", NewProcessTurnCompleteCommand("", true, nil, nil), true},
		{"ProcessTurnComplete valid ProcessID", NewProcessTurnCompleteCommand("worker-1", true, nil, nil), false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cmd.Validate()
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// ===========================================================================
// CommandType Constants Tests
// ===========================================================================

func TestProcessCommandTypeConstants(t *testing.T) {
	tests := []struct {
		cmdType  CommandType
		expected string
	}{
		{CmdSpawnProcess, "spawn_process"},
		{CmdRetireProcess, "retire_process"},
		{CmdReplaceProcess, "replace_process"},
		{CmdSendToProcess, "send_to_process"},
		{CmdDeliverProcessQueued, "deliver_process_queued"},
		{CmdProcessTurnComplete, "process_turn_complete"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.cmdType.String())
		})
	}
}
