package controlplane

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// === WorkflowID Tests ===

func TestNewWorkflowID_GeneratesValidUUID(t *testing.T) {
	id := NewWorkflowID()

	require.True(t, id.IsValid(), "NewWorkflowID should generate a valid UUID")
	require.NotEmpty(t, id.String())
}

func TestWorkflowID_IsValid_AcceptsValidUUIDs(t *testing.T) {
	tests := []struct {
		name  string
		id    WorkflowID
		valid bool
	}{
		{"valid UUID v4", WorkflowID("550e8400-e29b-41d4-a716-446655440000"), true},
		{"another valid UUID", WorkflowID("6ba7b810-9dad-11d1-80b4-00c04fd430c8"), true},
		{"UUID without dashes", WorkflowID("550e8400e29b41d4a716446655440000"), true}, // UUID library accepts this format
		{"empty string", WorkflowID(""), false},
		{"invalid format", WorkflowID("not-a-uuid"), false},
		{"too short", WorkflowID("550e8400"), false},
		{"invalid characters", WorkflowID("zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.valid, tt.id.IsValid())
		})
	}
}

func TestWorkflowID_String(t *testing.T) {
	id := WorkflowID("test-id-123")
	require.Equal(t, "test-id-123", id.String())
}

// === WorkflowState Tests ===

func TestWorkflowState_String(t *testing.T) {
	tests := []struct {
		state    WorkflowState
		expected string
	}{
		{WorkflowPending, "pending"},
		{WorkflowRunning, "running"},
		{WorkflowPaused, "paused"},
		{WorkflowCompleted, "completed"},
		{WorkflowFailed, "failed"},
		{WorkflowStopped, "stopped"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.state.String())
		})
	}
}

func TestWorkflowState_IsValid(t *testing.T) {
	tests := []struct {
		state WorkflowState
		valid bool
	}{
		{WorkflowPending, true},
		{WorkflowRunning, true},
		{WorkflowPaused, true},
		{WorkflowCompleted, true},
		{WorkflowFailed, true},
		{WorkflowStopped, true},
		{WorkflowState("invalid"), false},
		{WorkflowState(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			require.Equal(t, tt.valid, tt.state.IsValid())
		})
	}
}

func TestWorkflowState_IsTerminal(t *testing.T) {
	tests := []struct {
		state    WorkflowState
		terminal bool
	}{
		{WorkflowPending, false},
		{WorkflowRunning, false},
		{WorkflowPaused, false},
		{WorkflowCompleted, true},
		{WorkflowFailed, true},
		{WorkflowStopped, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			require.Equal(t, tt.terminal, tt.state.IsTerminal(),
				"IsTerminal() should return %v for state %s", tt.terminal, tt.state)
		})
	}
}

func TestWorkflowState_CanTransitionTo_ValidTransitions(t *testing.T) {
	// Test all valid transitions from the state machine
	tests := []struct {
		from WorkflowState
		to   WorkflowState
	}{
		// From Pending
		{WorkflowPending, WorkflowRunning},
		{WorkflowPending, WorkflowStopped},
		// From Running
		{WorkflowRunning, WorkflowPaused},
		{WorkflowRunning, WorkflowCompleted},
		{WorkflowRunning, WorkflowFailed},
		{WorkflowRunning, WorkflowStopped},
		// From Paused
		{WorkflowPaused, WorkflowRunning},
		{WorkflowPaused, WorkflowStopped},
	}

	for _, tt := range tests {
		t.Run(tt.from.String()+"->"+tt.to.String(), func(t *testing.T) {
			require.True(t, tt.from.CanTransitionTo(tt.to),
				"transition from %s to %s should be valid", tt.from, tt.to)
		})
	}
}

func TestWorkflowState_CanTransitionTo_InvalidTransitions(t *testing.T) {
	// Test invalid transitions
	tests := []struct {
		from WorkflowState
		to   WorkflowState
	}{
		// Cannot transition from terminal states
		{WorkflowCompleted, WorkflowRunning},
		{WorkflowCompleted, WorkflowPending},
		{WorkflowFailed, WorkflowRunning},
		{WorkflowFailed, WorkflowPending},
		{WorkflowStopped, WorkflowRunning},
		{WorkflowStopped, WorkflowPending},
		// Cannot skip states
		{WorkflowPending, WorkflowPaused},
		{WorkflowPending, WorkflowCompleted},
		{WorkflowPending, WorkflowFailed},
		// Cannot go backwards
		{WorkflowRunning, WorkflowPending},
		{WorkflowPaused, WorkflowPending},
		// Invalid state
		{WorkflowState("invalid"), WorkflowRunning},
	}

	for _, tt := range tests {
		t.Run(tt.from.String()+"->"+tt.to.String(), func(t *testing.T) {
			require.False(t, tt.from.CanTransitionTo(tt.to),
				"transition from %s to %s should be invalid", tt.from, tt.to)
		})
	}
}

func TestWorkflowState_ValidTargets(t *testing.T) {
	tests := []struct {
		state    WorkflowState
		expected []WorkflowState
	}{
		{WorkflowPending, []WorkflowState{WorkflowRunning, WorkflowStopped}},
		{WorkflowRunning, []WorkflowState{WorkflowPaused, WorkflowCompleted, WorkflowFailed, WorkflowStopped}},
		{WorkflowPaused, []WorkflowState{WorkflowRunning, WorkflowStopped, WorkflowFailed}},
		{WorkflowCompleted, []WorkflowState{}},
		{WorkflowFailed, []WorkflowState{}},
		{WorkflowStopped, []WorkflowState{}},
		{WorkflowState("invalid"), nil},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			targets := tt.state.ValidTargets()
			if tt.expected == nil {
				require.Nil(t, targets)
			} else {
				require.ElementsMatch(t, tt.expected, targets)
			}
		})
	}
}

// === WorkflowSpec Tests ===

func TestWorkflowSpec_Validate_ValidSpec(t *testing.T) {
	spec := &WorkflowSpec{
		TemplateID:    "cook.md",
		InitialPrompt: "Implement feature X",
		Name:          "Feature X",
	}

	err := spec.Validate()
	require.NoError(t, err)
}

func TestWorkflowSpec_Validate_RequiredFields(t *testing.T) {
	tests := []struct {
		name      string
		spec      *WorkflowSpec
		errSubstr string
	}{
		{
			name: "missing template_id",
			spec: &WorkflowSpec{
				InitialPrompt: "Do something",
			},
			errSubstr: "template_id is required",
		},
		{
			name: "missing initial_prompt",
			spec: &WorkflowSpec{
				TemplateID: "cook.md",
			},
			errSubstr: "initial_prompt is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate()
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.errSubstr)
		})
	}
}

// === WorkflowInstance Tests ===

func TestNewWorkflowInstance_CreatesInstance(t *testing.T) {
	spec := &WorkflowSpec{
		TemplateID:    "cook.md",
		InitialPrompt: "Implement feature X",
		Name:          "Feature X",
		Labels:        map[string]string{"team": "platform"},
	}

	inst, err := NewWorkflowInstance(spec)

	require.NoError(t, err)
	require.NotNil(t, inst)
	require.True(t, inst.ID.IsValid())
	require.Equal(t, "cook.md", inst.TemplateID)
	require.Equal(t, "Feature X", inst.Name)
	require.Equal(t, WorkflowPending, inst.State)
	require.Equal(t, "platform", inst.Labels["team"])
	require.False(t, inst.CreatedAt.IsZero())
	require.Nil(t, inst.StartedAt)
}

func TestNewWorkflowInstance_DefaultsNameToTemplateID(t *testing.T) {
	spec := &WorkflowSpec{
		TemplateID:    "cook.md",
		InitialPrompt: "Do something",
		// Name intentionally left empty
	}

	inst, err := NewWorkflowInstance(spec)

	require.NoError(t, err)
	require.Equal(t, "cook.md", inst.Name)
}

func TestNewWorkflowInstance_CopiesLabels(t *testing.T) {
	labels := map[string]string{"team": "platform"}
	spec := &WorkflowSpec{
		TemplateID:    "cook.md",
		InitialPrompt: "Do something",
		Labels:        labels,
	}

	inst, err := NewWorkflowInstance(spec)
	require.NoError(t, err)

	// Modify original labels
	labels["team"] = "modified"
	labels["new"] = "added"

	// Instance labels should be unchanged
	require.Equal(t, "platform", inst.Labels["team"])
	require.NotContains(t, inst.Labels, "new")
}

func TestNewWorkflowInstance_ReturnsErrorForInvalidSpec(t *testing.T) {
	spec := &WorkflowSpec{
		// Missing required fields
	}

	inst, err := NewWorkflowInstance(spec)

	require.Error(t, err)
	require.Nil(t, inst)
	require.Contains(t, err.Error(), "invalid spec")
}

func TestWorkflowInstance_TransitionTo_ValidTransitions(t *testing.T) {
	spec := &WorkflowSpec{
		TemplateID:    "cook.md",
		InitialPrompt: "Do something",
	}
	inst, err := NewWorkflowInstance(spec)
	require.NoError(t, err)

	// Pending -> Running
	err = inst.TransitionTo(WorkflowRunning)
	require.NoError(t, err)
	require.Equal(t, WorkflowRunning, inst.State)
	require.NotNil(t, inst.StartedAt, "StartedAt should be set on first transition to Running")

	// Running -> Paused
	err = inst.TransitionTo(WorkflowPaused)
	require.NoError(t, err)
	require.Equal(t, WorkflowPaused, inst.State)

	// Paused -> Running
	err = inst.TransitionTo(WorkflowRunning)
	require.NoError(t, err)
	require.Equal(t, WorkflowRunning, inst.State)

	// Running -> Completed
	err = inst.TransitionTo(WorkflowCompleted)
	require.NoError(t, err)
	require.Equal(t, WorkflowCompleted, inst.State)
}

func TestWorkflowInstance_TransitionTo_InvalidTransitions(t *testing.T) {
	spec := &WorkflowSpec{
		TemplateID:    "cook.md",
		InitialPrompt: "Do something",
	}
	inst, err := NewWorkflowInstance(spec)
	require.NoError(t, err)

	// Pending -> Paused (invalid)
	err = inst.TransitionTo(WorkflowPaused)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid state transition")
	require.Equal(t, WorkflowPending, inst.State, "state should remain unchanged")
}

func TestWorkflowInstance_TransitionTo_FromTerminalState(t *testing.T) {
	spec := &WorkflowSpec{
		TemplateID:    "cook.md",
		InitialPrompt: "Do something",
	}
	inst, err := NewWorkflowInstance(spec)
	require.NoError(t, err)

	// Move to terminal state
	require.NoError(t, inst.TransitionTo(WorkflowRunning))
	require.NoError(t, inst.TransitionTo(WorkflowCompleted))

	// Try to transition from terminal state
	err = inst.TransitionTo(WorkflowRunning)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid state transition")
}

func TestWorkflowInstance_IsTerminal(t *testing.T) {
	spec := &WorkflowSpec{
		TemplateID:    "cook.md",
		InitialPrompt: "Do something",
	}
	inst, err := NewWorkflowInstance(spec)
	require.NoError(t, err)

	require.False(t, inst.IsTerminal(), "Pending should not be terminal")

	require.NoError(t, inst.TransitionTo(WorkflowRunning))
	require.False(t, inst.IsTerminal(), "Running should not be terminal")

	require.NoError(t, inst.TransitionTo(WorkflowCompleted))
	require.True(t, inst.IsTerminal(), "Completed should be terminal")
}

func TestWorkflowInstance_IsActive(t *testing.T) {
	spec := &WorkflowSpec{
		TemplateID:    "cook.md",
		InitialPrompt: "Do something",
	}
	inst, err := NewWorkflowInstance(spec)
	require.NoError(t, err)

	require.True(t, inst.IsActive(), "Pending should be active")

	require.NoError(t, inst.TransitionTo(WorkflowRunning))
	require.True(t, inst.IsActive(), "Running should be active")

	require.NoError(t, inst.TransitionTo(WorkflowCompleted))
	require.False(t, inst.IsActive(), "Completed should not be active")
}

func TestWorkflowInstance_IsRunning(t *testing.T) {
	spec := &WorkflowSpec{
		TemplateID:    "cook.md",
		InitialPrompt: "Do something",
	}
	inst, err := NewWorkflowInstance(spec)
	require.NoError(t, err)

	require.False(t, inst.IsRunning())
	require.NoError(t, inst.TransitionTo(WorkflowRunning))
	require.True(t, inst.IsRunning())
	require.NoError(t, inst.TransitionTo(WorkflowPaused))
	require.False(t, inst.IsRunning())
}

func TestWorkflowInstance_IsPaused(t *testing.T) {
	spec := &WorkflowSpec{
		TemplateID:    "cook.md",
		InitialPrompt: "Do something",
	}
	inst, err := NewWorkflowInstance(spec)
	require.NoError(t, err)

	require.False(t, inst.IsPaused())
	require.NoError(t, inst.TransitionTo(WorkflowRunning))
	require.False(t, inst.IsPaused())
	require.NoError(t, inst.TransitionTo(WorkflowPaused))
	require.True(t, inst.IsPaused())
}

func TestWorkflowInstance_RecordHeartbeat(t *testing.T) {
	spec := &WorkflowSpec{
		TemplateID:    "cook.md",
		InitialPrompt: "Do something",
	}
	inst, err := NewWorkflowInstance(spec)
	require.NoError(t, err)

	initialUpdated := inst.UpdatedAt
	require.True(t, inst.LastHeartbeatAt.IsZero())

	inst.RecordHeartbeat()

	require.False(t, inst.LastHeartbeatAt.IsZero())
	require.True(t, inst.UpdatedAt.After(initialUpdated) || inst.UpdatedAt.Equal(initialUpdated))
}

func TestWorkflowInstance_RecordProgress(t *testing.T) {
	spec := &WorkflowSpec{
		TemplateID:    "cook.md",
		InitialPrompt: "Do something",
	}
	inst, err := NewWorkflowInstance(spec)
	require.NoError(t, err)

	require.True(t, inst.LastProgressAt.IsZero())
	require.True(t, inst.LastHeartbeatAt.IsZero())

	inst.RecordProgress()

	require.False(t, inst.LastProgressAt.IsZero())
	require.False(t, inst.LastHeartbeatAt.IsZero(), "Progress should also update heartbeat")
	require.Equal(t, inst.LastProgressAt, inst.LastHeartbeatAt)
}

func TestWorkflowInstance_TokenTracking(t *testing.T) {
	spec := &WorkflowSpec{
		TemplateID:    "cook.md",
		InitialPrompt: "Do something",
	}
	inst, err := NewWorkflowInstance(spec)
	require.NoError(t, err)

	inst.AddTokens(5000)
	require.Equal(t, int64(5000), inst.TokensUsed)

	inst.AddTokens(5000)
	require.Equal(t, int64(10000), inst.TokensUsed)
}

func TestWorkflowInstance_TokenMetrics(t *testing.T) {
	spec := &WorkflowSpec{
		TemplateID:    "cook.md",
		InitialPrompt: "Do something",
	}
	inst, err := NewWorkflowInstance(spec)
	require.NoError(t, err)

	inst.AddTokens(5000)
	metrics := inst.TokenMetrics()

	require.NotNil(t, metrics)
	require.Equal(t, 5000, metrics.TotalTokens)
}

// === WorkflowSpec Worktree Tests ===

func TestWorkflowSpec_WorktreeFields(t *testing.T) {
	spec := &WorkflowSpec{
		TemplateID:         "cook.md",
		InitialPrompt:      "Implement feature X",
		Name:               "Feature X",
		WorktreeEnabled:    true,
		WorktreeBaseBranch: "main",
		WorktreeBranchName: "feature/my-branch",
	}

	// Verify fields are stored correctly
	require.True(t, spec.WorktreeEnabled)
	require.Equal(t, "main", spec.WorktreeBaseBranch)
	require.Equal(t, "feature/my-branch", spec.WorktreeBranchName)

	// Verify spec with worktree fields still validates
	err := spec.Validate()
	require.NoError(t, err)
}

func TestWorkflowSpec_WorktreeFieldsDefaultValues(t *testing.T) {
	spec := &WorkflowSpec{
		TemplateID:    "cook.md",
		InitialPrompt: "Implement feature X",
	}

	// Verify default values (all false/empty)
	require.False(t, spec.WorktreeEnabled)
	require.Empty(t, spec.WorktreeBaseBranch)
	require.Empty(t, spec.WorktreeBranchName)

	// Verify spec without worktree fields still validates
	err := spec.Validate()
	require.NoError(t, err)
}

// === NewWorkflowInstance Worktree Preservation Tests ===

func TestNewWorkflowInstance_PreservesWorktreeFields(t *testing.T) {
	spec := &WorkflowSpec{
		TemplateID:         "cook.md",
		InitialPrompt:      "Implement feature X",
		WorktreeEnabled:    true,
		WorktreeBaseBranch: "develop",
		WorktreeBranchName: "feature/custom-branch",
	}

	inst, err := NewWorkflowInstance(spec)

	require.NoError(t, err)
	require.NotNil(t, inst)

	// Verify worktree configuration is preserved
	require.True(t, inst.WorktreeEnabled)
	require.Equal(t, "develop", inst.WorktreeBaseBranch)
	require.Equal(t, "feature/custom-branch", inst.WorktreeBranchName)

	// Verify worktree state fields are empty (set by Supervisor.AllocateResources())
	require.Empty(t, inst.WorktreePath)
	require.Empty(t, inst.WorktreeBranch)
}

func TestNewWorkflowInstance_PreservesWorktreeDisabled(t *testing.T) {
	spec := &WorkflowSpec{
		TemplateID:      "cook.md",
		InitialPrompt:   "Implement feature X",
		WorktreeEnabled: false,
	}

	inst, err := NewWorkflowInstance(spec)

	require.NoError(t, err)
	require.NotNil(t, inst)

	// Verify worktree is disabled
	require.False(t, inst.WorktreeEnabled)
	require.Empty(t, inst.WorktreeBaseBranch)
	require.Empty(t, inst.WorktreeBranchName)
	require.Empty(t, inst.WorktreePath)
	require.Empty(t, inst.WorktreeBranch)
}

// === WorkflowSpec EpicID Tests ===

func TestWorkflowSpec_EpicIDField(t *testing.T) {
	spec := &WorkflowSpec{
		TemplateID:    "cook.md",
		InitialPrompt: "Implement feature X",
		Name:          "Feature X",
		EpicID:        "epic-123",
	}

	// Verify field is stored correctly
	require.Equal(t, "epic-123", spec.EpicID)

	// Verify spec with EpicID still validates
	err := spec.Validate()
	require.NoError(t, err)
}

func TestWorkflowSpec_EpicIDEmptyIsValid(t *testing.T) {
	spec := &WorkflowSpec{
		TemplateID:    "cook.md",
		InitialPrompt: "Implement feature X",
		// EpicID intentionally left empty for backwards compatibility
	}

	// Verify empty EpicID is valid
	require.Empty(t, spec.EpicID)

	// Verify spec without EpicID still validates
	err := spec.Validate()
	require.NoError(t, err)
}

// === NewWorkflowInstance EpicID Preservation Tests ===

func TestNewWorkflowInstance_PreservesEpicID(t *testing.T) {
	spec := &WorkflowSpec{
		TemplateID:    "cook.md",
		InitialPrompt: "Implement feature X",
		EpicID:        "perles-123",
	}

	inst, err := NewWorkflowInstance(spec)

	require.NoError(t, err)
	require.NotNil(t, inst)

	// Verify EpicID is copied from spec
	require.Equal(t, "perles-123", inst.EpicID)
}

func TestNewWorkflowInstance_EmptyEpicIDIsValid(t *testing.T) {
	spec := &WorkflowSpec{
		TemplateID:    "cook.md",
		InitialPrompt: "Implement feature X",
		// EpicID intentionally left empty for backwards compatibility
	}

	inst, err := NewWorkflowInstance(spec)

	require.NoError(t, err)
	require.NotNil(t, inst)

	// Verify empty EpicID is preserved (backwards compatibility)
	require.Empty(t, inst.EpicID)
}
