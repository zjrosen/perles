package controlplane

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
)

// mockWorkflowProvider provides test workflows.
type mockWorkflowProvider struct {
	mu        sync.RWMutex
	workflows map[WorkflowID]*WorkflowInstance
}

func newMockWorkflowProvider() *mockWorkflowProvider {
	return &mockWorkflowProvider{
		workflows: make(map[WorkflowID]*WorkflowInstance),
	}
}

func (p *mockWorkflowProvider) Get(id WorkflowID) (*WorkflowInstance, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	inst, ok := p.workflows[id]
	return inst, ok
}

func (p *mockWorkflowProvider) Put(inst *WorkflowInstance) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.workflows[inst.ID] = inst
}

// mockCommandSubmitter captures submitted commands for verification.
type mockCommandSubmitter struct {
	mu       sync.Mutex
	commands []command.Command
	result   *command.CommandResult
	err      error
}

func newMockCommandSubmitter() *mockCommandSubmitter {
	return &mockCommandSubmitter{
		result: &command.CommandResult{Success: true},
	}
}

func (s *mockCommandSubmitter) SubmitAndWait(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.commands = append(s.commands, cmd)
	return s.result, s.err
}

func (s *mockCommandSubmitter) SetResult(result *command.CommandResult, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.result = result
	s.err = err
}

func (s *mockCommandSubmitter) GetCommands() []command.Command {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]command.Command{}, s.commands...)
}

// createTestWorkflow creates a test workflow instance.
func createTestWorkflow(id WorkflowID, state WorkflowState) *WorkflowInstance {
	spec := &WorkflowSpec{
		TemplateID:  "test",
		InitialGoal: "test goal",
	}
	inst, _ := NewWorkflowInstance(spec)
	inst.ID = id
	inst.State = state
	return inst
}

func TestRecoveryAction_String(t *testing.T) {
	tests := []struct {
		action   RecoveryAction
		expected string
	}{
		{RecoveryNudge, "nudge"},
		{RecoveryReplace, "replace"},
		{RecoveryPause, "pause"},
		{RecoveryFail, "fail"},
		{RecoveryAction(99), "unknown(99)"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.action.String())
		})
	}
}

func TestRecoveryAction_IsValid(t *testing.T) {
	tests := []struct {
		action   RecoveryAction
		expected bool
	}{
		{RecoveryNudge, true},
		{RecoveryReplace, true},
		{RecoveryPause, true},
		{RecoveryFail, true},
		{RecoveryAction(-1), false},
		{RecoveryAction(99), false},
	}

	for _, tt := range tests {
		t.Run(tt.action.String(), func(t *testing.T) {
			require.Equal(t, tt.expected, tt.action.IsValid())
		})
	}
}

func TestNewRecoveryExecutor_RequiresWorkflowProvider(t *testing.T) {
	_, err := NewRecoveryExecutor(RecoveryExecutorConfig{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "WorkflowProvider is required")
}

func TestRecoveryExecutor_ExecuteRecovery_InvalidAction(t *testing.T) {
	provider := newMockWorkflowProvider()
	executor, err := NewRecoveryExecutor(RecoveryExecutorConfig{
		WorkflowProvider: provider,
	})
	require.NoError(t, err)

	err = executor.ExecuteRecovery(context.Background(), "wf-1", RecoveryAction(99))
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid recovery action")
}

func TestRecoveryExecutor_ExecuteRecovery_WorkflowNotFound(t *testing.T) {
	provider := newMockWorkflowProvider()
	executor, err := NewRecoveryExecutor(RecoveryExecutorConfig{
		WorkflowProvider: provider,
	})
	require.NoError(t, err)

	err = executor.ExecuteRecovery(context.Background(), "nonexistent", RecoveryNudge)
	require.Error(t, err)
	require.Contains(t, err.Error(), "workflow not found")
}

func TestRecoveryExecutor_Nudge_SendsMessageToCoordinator(t *testing.T) {
	provider := newMockWorkflowProvider()
	mockSubmitter := newMockCommandSubmitter()

	inst := createTestWorkflow("wf-1", WorkflowRunning)
	provider.Put(inst)

	var receivedEvents []HealthEvent
	var eventMu sync.Mutex

	executor, err := NewRecoveryExecutor(RecoveryExecutorConfig{
		WorkflowProvider: provider,
		CommandSubmitterFactory: func(inst *WorkflowInstance) CommandSubmitter {
			return mockSubmitter
		},
		OnHealthEvent: func(event HealthEvent) {
			eventMu.Lock()
			receivedEvents = append(receivedEvents, event)
			eventMu.Unlock()
		},
	})
	require.NoError(t, err)

	err = executor.ExecuteRecovery(context.Background(), "wf-1", RecoveryNudge)
	require.NoError(t, err)

	// Verify send-to-process command was submitted
	commands := mockSubmitter.GetCommands()
	require.Len(t, commands, 1)
	sendCmd, ok := commands[0].(*command.SendToProcessCommand)
	require.True(t, ok)
	require.Equal(t, repository.CoordinatorID, sendCmd.ProcessID)
	require.Contains(t, sendCmd.Content, "Health check")

	// Wait for async events
	time.Sleep(50 * time.Millisecond)

	eventMu.Lock()
	require.Len(t, receivedEvents, 2) // Started + Success
	require.Equal(t, HealthRecoveryStarted, receivedEvents[0].Type)
	require.Equal(t, "nudge", receivedEvents[0].RecoveryAction)
	require.Equal(t, HealthRecoverySuccess, receivedEvents[1].Type)
	eventMu.Unlock()
}

func TestRecoveryExecutor_Nudge_FailsWhenNotRunning(t *testing.T) {
	provider := newMockWorkflowProvider()
	mockSubmitter := newMockCommandSubmitter()
	inst := createTestWorkflow("wf-1", WorkflowPaused)
	provider.Put(inst)

	executor, err := NewRecoveryExecutor(RecoveryExecutorConfig{
		WorkflowProvider: provider,
		CommandSubmitterFactory: func(inst *WorkflowInstance) CommandSubmitter {
			return mockSubmitter
		},
	})
	require.NoError(t, err)

	err = executor.ExecuteRecovery(context.Background(), "wf-1", RecoveryNudge)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot nudge workflow in state")
}

func TestRecoveryExecutor_Nudge_FailsWhenNoInfrastructure(t *testing.T) {
	provider := newMockWorkflowProvider()
	inst := createTestWorkflow("wf-1", WorkflowRunning)
	provider.Put(inst)

	executor, err := NewRecoveryExecutor(RecoveryExecutorConfig{
		WorkflowProvider: provider,
		CommandSubmitterFactory: func(inst *WorkflowInstance) CommandSubmitter {
			return nil // No infrastructure
		},
	})
	require.NoError(t, err)

	err = executor.ExecuteRecovery(context.Background(), "wf-1", RecoveryNudge)
	require.Error(t, err)
	require.Contains(t, err.Error(), "workflow infrastructure not available")
}

func TestRecoveryExecutor_Replace_CallsReplaceCoordinator(t *testing.T) {
	provider := newMockWorkflowProvider()
	mockSubmitter := newMockCommandSubmitter()

	inst := createTestWorkflow("wf-1", WorkflowRunning)
	provider.Put(inst)

	executor, err := NewRecoveryExecutor(RecoveryExecutorConfig{
		WorkflowProvider: provider,
		CommandSubmitterFactory: func(inst *WorkflowInstance) CommandSubmitter {
			return mockSubmitter
		},
	})
	require.NoError(t, err)

	err = executor.ExecuteRecovery(context.Background(), "wf-1", RecoveryReplace)
	require.NoError(t, err)

	// Verify replace-process command was submitted
	commands := mockSubmitter.GetCommands()
	require.Len(t, commands, 1)
	replaceCmd, ok := commands[0].(*command.ReplaceProcessCommand)
	require.True(t, ok)
	require.Equal(t, repository.CoordinatorID, replaceCmd.ProcessID)
	require.Contains(t, replaceCmd.Reason, "stuck workflow recovery")
}

func TestRecoveryExecutor_Pause_TransitionsToWorkflowPaused(t *testing.T) {
	provider := newMockWorkflowProvider()
	inst := createTestWorkflow("wf-1", WorkflowRunning)
	provider.Put(inst)

	executor, err := NewRecoveryExecutor(RecoveryExecutorConfig{
		WorkflowProvider: provider,
	})
	require.NoError(t, err)

	err = executor.ExecuteRecovery(context.Background(), "wf-1", RecoveryPause)
	require.NoError(t, err)

	// Verify state transitioned
	require.Equal(t, WorkflowPaused, inst.State)
}

func TestRecoveryExecutor_Pause_FailsWhenNotRunning(t *testing.T) {
	provider := newMockWorkflowProvider()
	inst := createTestWorkflow("wf-1", WorkflowPending)
	provider.Put(inst)

	executor, err := NewRecoveryExecutor(RecoveryExecutorConfig{
		WorkflowProvider: provider,
	})
	require.NoError(t, err)

	err = executor.ExecuteRecovery(context.Background(), "wf-1", RecoveryPause)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot pause workflow in state")
}

func TestRecoveryExecutor_Fail_TransitionsToWorkflowFailed(t *testing.T) {
	provider := newMockWorkflowProvider()
	inst := createTestWorkflow("wf-1", WorkflowRunning)
	provider.Put(inst)

	executor, err := NewRecoveryExecutor(RecoveryExecutorConfig{
		WorkflowProvider: provider,
	})
	require.NoError(t, err)

	err = executor.ExecuteRecovery(context.Background(), "wf-1", RecoveryFail)
	require.NoError(t, err)

	// Verify state transitioned
	require.Equal(t, WorkflowFailed, inst.State)
}

func TestRecoveryExecutor_Fail_FailsWhenAlreadyTerminal(t *testing.T) {
	provider := newMockWorkflowProvider()
	inst := createTestWorkflow("wf-1", WorkflowCompleted)
	provider.Put(inst)

	executor, err := NewRecoveryExecutor(RecoveryExecutorConfig{
		WorkflowProvider: provider,
	})
	require.NoError(t, err)

	err = executor.ExecuteRecovery(context.Background(), "wf-1", RecoveryFail)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot fail workflow already in terminal state")
}

func TestRecoveryExecutor_EmitsFailedEventOnError(t *testing.T) {
	provider := newMockWorkflowProvider()
	mockSubmitter := newMockCommandSubmitter()
	mockSubmitter.SetResult(&command.CommandResult{
		Success: false,
		Error:   errors.New("process not available"),
	}, nil)

	inst := createTestWorkflow("wf-1", WorkflowRunning)
	provider.Put(inst)

	var receivedEvents []HealthEvent
	var eventMu sync.Mutex
	eventsDone := make(chan struct{})

	executor, err := NewRecoveryExecutor(RecoveryExecutorConfig{
		WorkflowProvider: provider,
		CommandSubmitterFactory: func(inst *WorkflowInstance) CommandSubmitter {
			return mockSubmitter
		},
		OnHealthEvent: func(event HealthEvent) {
			eventMu.Lock()
			receivedEvents = append(receivedEvents, event)
			count := len(receivedEvents)
			eventMu.Unlock()
			if count >= 2 {
				select {
				case <-eventsDone: // Already closed
				default:
					close(eventsDone)
				}
			}
		},
	})
	require.NoError(t, err)

	err = executor.ExecuteRecovery(context.Background(), "wf-1", RecoveryNudge)
	require.Error(t, err)

	// Wait for both events with timeout
	select {
	case <-eventsDone:
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for events")
	}

	eventMu.Lock()
	require.Len(t, receivedEvents, 2) // Started + Failed
	// Events are emitted async so check both types are present
	eventTypes := make(map[HealthEventType]bool)
	for _, e := range receivedEvents {
		eventTypes[e.Type] = true
	}
	require.True(t, eventTypes[HealthRecoveryStarted], "should have started event")
	require.True(t, eventTypes[HealthRecoveryFailed], "should have failed event")

	// Also verify the failed event has the error details
	for _, e := range receivedEvents {
		if e.Type == HealthRecoveryFailed {
			require.Contains(t, e.Details, "process not available")
		}
	}
	eventMu.Unlock()
}

func TestDetermineRecoveryAction_NotStuck_ReturnsNegative(t *testing.T) {
	// Not stuck: LastProgressAt is recent (within ProgressTimeout)
	status := &HealthStatus{
		WorkflowID:     "wf-1",
		IsHealthy:      true,
		LastProgressAt: time.Now(), // Recent progress
	}
	policy := HealthPolicy{
		MaxRecoveries:   3,
		ProgressTimeout: 5 * time.Minute,
		EnableAutoNudge: true,
	}

	action := DetermineRecoveryAction(status, policy)
	require.Equal(t, RecoveryAction(-1), action)
}

func TestDetermineRecoveryAction_MaxRecoveriesExceeded_ReturnsFail(t *testing.T) {
	// Stuck and at max recoveries
	status := &HealthStatus{
		WorkflowID:     "wf-1",
		LastProgressAt: time.Now().Add(-10 * time.Minute), // stuck
		RecoveryCount:  3,                                 // At max
	}
	policy := HealthPolicy{
		MaxRecoveries:     3,
		ProgressTimeout:   5 * time.Minute,
		EnableAutoNudge:   true,
		EnableAutoReplace: true,
		EnableAutoPause:   true,
	}

	action := DetermineRecoveryAction(status, policy)
	require.Equal(t, RecoveryFail, action)
}

func TestDetermineRecoveryAction_FirstRecovery_ReturnsNudge(t *testing.T) {
	status := &HealthStatus{
		WorkflowID:     "wf-1",
		LastProgressAt: time.Now().Add(-10 * time.Minute), // stuck
		RecoveryCount:  0,
	}
	policy := HealthPolicy{
		MaxRecoveries:     5,
		MaxNudges:         2,
		ProgressTimeout:   5 * time.Minute,
		EnableAutoNudge:   true,
		EnableAutoReplace: true,
		EnableAutoPause:   true,
	}

	action := DetermineRecoveryAction(status, policy)
	require.Equal(t, RecoveryNudge, action)
}

func TestDetermineRecoveryAction_NudgeDisabled_ReturnsReplace_AtMaxNudges(t *testing.T) {
	// When nudge is disabled, we skip the nudge phase entirely.
	// Replace triggers when count == MaxNudges (even if no nudges were sent)
	status := &HealthStatus{
		WorkflowID:     "wf-1",
		LastProgressAt: time.Now().Add(-10 * time.Minute), // stuck
		RecoveryCount:  2,                                 // At MaxNudges threshold
	}
	policy := HealthPolicy{
		MaxRecoveries:     5,
		MaxNudges:         2,
		ProgressTimeout:   5 * time.Minute,
		EnableAutoNudge:   false, // Disabled
		EnableAutoReplace: true,
		EnableAutoPause:   true,
	}

	action := DetermineRecoveryAction(status, policy)
	require.Equal(t, RecoveryReplace, action)
}

func TestDetermineRecoveryAction_AfterMaxNudges_ReturnsReplace(t *testing.T) {
	// After MaxNudges (default 2) nudge attempts, escalate to Replace
	status := &HealthStatus{
		WorkflowID:     "wf-1",
		LastProgressAt: time.Now().Add(-10 * time.Minute), // stuck
		RecoveryCount:  2,                                 // After 2 nudges (MaxNudges reached)
	}
	policy := HealthPolicy{
		MaxRecoveries:     5,
		MaxNudges:         2,
		ProgressTimeout:   5 * time.Minute,
		EnableAutoNudge:   true,
		EnableAutoReplace: true,
		EnableAutoPause:   true,
	}

	action := DetermineRecoveryAction(status, policy)
	require.Equal(t, RecoveryReplace, action)
}

func TestDetermineRecoveryAction_AfterReplace_ReturnsPause(t *testing.T) {
	// After Replace (count == MaxNudges+1), escalate to Pause
	status := &HealthStatus{
		WorkflowID:     "wf-1",
		LastProgressAt: time.Now().Add(-10 * time.Minute), // stuck
		RecoveryCount:  3,                                 // After 2 nudges + 1 replace (MaxNudges + 1)
	}
	policy := HealthPolicy{
		MaxRecoveries:     5,
		MaxNudges:         2,
		ProgressTimeout:   5 * time.Minute,
		EnableAutoNudge:   true,
		EnableAutoReplace: true,
		EnableAutoPause:   true,
	}

	action := DetermineRecoveryAction(status, policy)
	require.Equal(t, RecoveryPause, action)
}

func TestDetermineRecoveryAction_AllAutoRecoveryDisabled_ReturnsNegative(t *testing.T) {
	// At MaxNudges threshold with all auto-recovery disabled
	status := &HealthStatus{
		WorkflowID:     "wf-1",
		LastProgressAt: time.Now().Add(-10 * time.Minute), // stuck
		RecoveryCount:  2,                                 // At MaxNudges threshold
	}
	policy := HealthPolicy{
		MaxRecoveries:     5,
		MaxNudges:         2,
		ProgressTimeout:   5 * time.Minute,
		EnableAutoNudge:   false,
		EnableAutoReplace: false,
		EnableAutoPause:   false,
	}

	action := DetermineRecoveryAction(status, policy)
	require.Equal(t, RecoveryAction(-1), action)
}

func TestHealthMonitor_RecoveryCountIncrementsAfterEachAttempt(t *testing.T) {
	clock := newMockClock(time.Now())
	provider := newMockWorkflowProvider()
	mockSubmitter := newMockCommandSubmitter()

	inst := createTestWorkflow("wf-1", WorkflowRunning)
	provider.Put(inst)

	recoveryExecutor, _ := NewRecoveryExecutor(RecoveryExecutorConfig{
		WorkflowProvider: provider,
		CommandSubmitterFactory: func(inst *WorkflowInstance) CommandSubmitter {
			return mockSubmitter
		},
	})

	policy := HealthPolicy{
		HeartbeatTimeout:  50 * time.Millisecond,
		ProgressTimeout:   100 * time.Millisecond,
		MaxRecoveries:     5,
		RecoveryBackoff:   10 * time.Millisecond,
		EnableAutoNudge:   true,
		EnableAutoReplace: true,
		EnableAutoPause:   true,
	}

	monitor := NewHealthMonitor(HealthMonitorConfig{
		Policy:           policy,
		CheckInterval:    20 * time.Millisecond,
		Clock:            clock,
		RecoveryExecutor: recoveryExecutor,
	})

	monitor.TrackWorkflow("wf-1")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := monitor.Start(ctx)
	require.NoError(t, err)

	// Advance past progress timeout to trigger stuck
	clock.Advance(150 * time.Millisecond)
	time.Sleep(30 * time.Millisecond) // Wait for check loop

	// First check - recovery count should be 1
	status, _ := monitor.GetStatus("wf-1")
	require.Equal(t, 1, status.RecoveryCount)

	// Advance past backoff and trigger another check
	clock.Advance(50 * time.Millisecond)
	time.Sleep(30 * time.Millisecond)

	// Second check - recovery count should be 2
	status, _ = monitor.GetStatus("wf-1")
	require.Equal(t, 2, status.RecoveryCount)

	monitor.Stop()
}

func TestHealthMonitor_RecoveryBackoffRespectedBetweenAttempts(t *testing.T) {
	clock := newMockClock(time.Now())
	provider := newMockWorkflowProvider()
	mockSubmitter := newMockCommandSubmitter()

	inst := createTestWorkflow("wf-1", WorkflowRunning)
	provider.Put(inst)

	recoveryExecutor, _ := NewRecoveryExecutor(RecoveryExecutorConfig{
		WorkflowProvider: provider,
		CommandSubmitterFactory: func(inst *WorkflowInstance) CommandSubmitter {
			return mockSubmitter
		},
	})

	policy := HealthPolicy{
		HeartbeatTimeout:  50 * time.Millisecond,
		ProgressTimeout:   100 * time.Millisecond,
		MaxRecoveries:     5,
		RecoveryBackoff:   500 * time.Millisecond, // Long backoff
		EnableAutoNudge:   true,
		EnableAutoReplace: true,
		EnableAutoPause:   true,
	}

	monitor := NewHealthMonitor(HealthMonitorConfig{
		Policy:           policy,
		CheckInterval:    20 * time.Millisecond,
		Clock:            clock,
		RecoveryExecutor: recoveryExecutor,
	})

	monitor.TrackWorkflow("wf-1")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := monitor.Start(ctx)
	require.NoError(t, err)

	// Advance past progress timeout to trigger stuck
	clock.Advance(150 * time.Millisecond)

	// Wait for first recovery (use Eventually for robustness on slow CI)
	require.Eventually(t, func() bool {
		status, _ := monitor.GetStatus("wf-1")
		return status.RecoveryCount == 1
	}, 500*time.Millisecond, 10*time.Millisecond, "expected recovery count to be 1")

	// Advance only 100ms (less than backoff)
	clock.Advance(100 * time.Millisecond)

	// Wait a bit and verify still at 1 (backoff not elapsed)
	time.Sleep(50 * time.Millisecond)
	status, _ := monitor.GetStatus("wf-1")
	require.Equal(t, 1, status.RecoveryCount, "recovery count should still be 1 during backoff")

	// Now advance past backoff
	clock.Advance(500 * time.Millisecond)

	// Wait for second recovery
	require.Eventually(t, func() bool {
		status, _ := monitor.GetStatus("wf-1")
		return status.RecoveryCount == 2
	}, 500*time.Millisecond, 10*time.Millisecond, "expected recovery count to be 2 after backoff")

	monitor.Stop()
}

func TestHealthMonitor_MaxRecoveriesLeadsToFailAction(t *testing.T) {
	clock := newMockClock(time.Now())
	provider := newMockWorkflowProvider()
	mockSubmitter := newMockCommandSubmitter()

	inst := createTestWorkflow("wf-1", WorkflowRunning)
	provider.Put(inst)

	recoveryExecutor, _ := NewRecoveryExecutor(RecoveryExecutorConfig{
		WorkflowProvider: provider,
		CommandSubmitterFactory: func(inst *WorkflowInstance) CommandSubmitter {
			return mockSubmitter
		},
	})

	// With MaxNudges=1, sequence is: nudge(0) -> replace(1) -> pause(2) -> fail(3)
	// MaxRecoveries=3 means fail triggers when count >= 3
	policy := HealthPolicy{
		HeartbeatTimeout:  50 * time.Millisecond,
		ProgressTimeout:   100 * time.Millisecond,
		MaxRecoveries:     3,
		MaxNudges:         1, // Only 1 nudge before escalation
		RecoveryBackoff:   10 * time.Millisecond,
		EnableAutoNudge:   true,
		EnableAutoReplace: true,
		EnableAutoPause:   true,
	}

	monitor := NewHealthMonitor(HealthMonitorConfig{
		Policy:           policy,
		CheckInterval:    20 * time.Millisecond,
		Clock:            clock,
		RecoveryExecutor: recoveryExecutor,
	})

	monitor.TrackWorkflow("wf-1")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := monitor.Start(ctx)
	require.NoError(t, err)

	// Run through all recovery attempts: nudge(0), replace(1), pause(2), fail(3)
	for i := 0; i < 5; i++ {
		clock.Advance(150 * time.Millisecond)
		time.Sleep(30 * time.Millisecond)
	}

	// After max recoveries, the workflow should be in Failed state
	require.Equal(t, WorkflowFailed, inst.State)

	monitor.Stop()
}

func TestHealthMonitor_HealthEventsEmittedForEachRecoveryAction(t *testing.T) {
	clock := newMockClock(time.Now())
	provider := newMockWorkflowProvider()
	mockSubmitter := newMockCommandSubmitter()

	inst := createTestWorkflow("wf-1", WorkflowRunning)
	provider.Put(inst)

	var receivedEvents []HealthEvent
	var eventMu sync.Mutex

	recoveryExecutor, _ := NewRecoveryExecutor(RecoveryExecutorConfig{
		WorkflowProvider: provider,
		CommandSubmitterFactory: func(inst *WorkflowInstance) CommandSubmitter {
			return mockSubmitter
		},
		OnHealthEvent: func(event HealthEvent) {
			eventMu.Lock()
			receivedEvents = append(receivedEvents, event)
			eventMu.Unlock()
		},
	})

	policy := HealthPolicy{
		HeartbeatTimeout:  50 * time.Millisecond,
		ProgressTimeout:   100 * time.Millisecond,
		MaxRecoveries:     5,
		MaxNudges:         2,
		RecoveryBackoff:   10 * time.Millisecond,
		EnableAutoNudge:   true,
		EnableAutoReplace: true,
		EnableAutoPause:   true,
	}

	monitor := NewHealthMonitor(HealthMonitorConfig{
		Policy:           policy,
		CheckInterval:    20 * time.Millisecond,
		Clock:            clock,
		RecoveryExecutor: recoveryExecutor,
	})

	monitor.TrackWorkflow("wf-1")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := monitor.Start(ctx)
	require.NoError(t, err)

	// Trigger stuck and first recovery
	clock.Advance(150 * time.Millisecond)
	time.Sleep(80 * time.Millisecond) // Increase wait time for async recovery

	eventMu.Lock()
	// Should have: Started + Success for the first recovery
	recoveryEvents := filterRecoveryEvents(receivedEvents)
	require.GreaterOrEqual(t, len(recoveryEvents), 2, "Expected at least 2 recovery events, got %d: %v", len(recoveryEvents), recoveryEvents)
	require.Equal(t, HealthRecoveryStarted, recoveryEvents[0].Type)
	require.Equal(t, HealthRecoverySuccess, recoveryEvents[1].Type)
	require.Equal(t, "nudge", recoveryEvents[0].RecoveryAction)
	eventMu.Unlock()

	monitor.Stop()
}

// Integration test: Full recovery sequence (nudge -> replace -> pause -> fail)
func TestHealthMonitor_FullRecoverySequence_Integration(t *testing.T) {
	clock := newMockClock(time.Now())
	provider := newMockWorkflowProvider()
	mockSubmitter := newMockCommandSubmitter()

	inst := createTestWorkflow("wf-1", WorkflowRunning)
	provider.Put(inst)

	var recoveryActions []RecoveryAction
	var mu sync.Mutex

	recoveryExecutor, _ := NewRecoveryExecutor(RecoveryExecutorConfig{
		WorkflowProvider: provider,
		CommandSubmitterFactory: func(inst *WorkflowInstance) CommandSubmitter {
			return mockSubmitter
		},
		OnHealthEvent: func(event HealthEvent) {
			if event.Type == HealthRecoveryStarted {
				mu.Lock()
				switch event.RecoveryAction {
				case "nudge":
					recoveryActions = append(recoveryActions, RecoveryNudge)
				case "replace":
					recoveryActions = append(recoveryActions, RecoveryReplace)
				case "pause":
					recoveryActions = append(recoveryActions, RecoveryPause)
				case "fail":
					recoveryActions = append(recoveryActions, RecoveryFail)
				}
				mu.Unlock()
			}
		},
	})

	// With MaxNudges=2, escalation sequence is:
	// nudge(0), nudge(1) -> replace(2) -> pause(3) -> fail(4) = 5 actions total
	// MaxRecoveries=4 means after count reaches 4, next action is fail (count >= MaxRecoveries)
	policy := HealthPolicy{
		HeartbeatTimeout:  50 * time.Millisecond,
		ProgressTimeout:   100 * time.Millisecond,
		MaxRecoveries:     4, // Fail triggers when count >= 4
		MaxNudges:         2,
		RecoveryBackoff:   10 * time.Millisecond,
		EnableAutoNudge:   true,
		EnableAutoReplace: true,
		EnableAutoPause:   true,
	}

	monitor := NewHealthMonitor(HealthMonitorConfig{
		Policy:           policy,
		CheckInterval:    20 * time.Millisecond,
		Clock:            clock,
		RecoveryExecutor: recoveryExecutor,
	})

	monitor.TrackWorkflow("wf-1")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := monitor.Start(ctx)
	require.NoError(t, err)

	// Run through all recoveries (need more iterations for 5 actions)
	for i := 0; i < 7; i++ {
		clock.Advance(150 * time.Millisecond)
		time.Sleep(40 * time.Millisecond)
	}

	monitor.Stop()

	// Verify the sequence: nudge -> nudge -> replace -> pause -> fail
	mu.Lock()
	require.Len(t, recoveryActions, 5, "Expected 5 recovery actions (nudge, nudge, replace, pause, fail)")
	require.Equal(t, RecoveryNudge, recoveryActions[0])
	require.Equal(t, RecoveryNudge, recoveryActions[1])
	require.Equal(t, RecoveryReplace, recoveryActions[2])
	require.Equal(t, RecoveryPause, recoveryActions[3])
	require.Equal(t, RecoveryFail, recoveryActions[4])
	mu.Unlock()

	// Workflow should be Failed
	require.Equal(t, WorkflowFailed, inst.State)
}

func filterRecoveryEvents(events []HealthEvent) []HealthEvent {
	var result []HealthEvent
	for _, e := range events {
		if e.Type == HealthRecoveryStarted || e.Type == HealthRecoverySuccess || e.Type == HealthRecoveryFailed {
			result = append(result, e)
		}
	}
	return result
}
