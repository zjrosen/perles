package handler_test

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/orchestration/v2/handler"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
)

// ===========================================================================
// TurnCompletionTracker Unit Tests
// ===========================================================================

func TestNewTurnCompletionTracker(t *testing.T) {
	tracker := handler.NewTurnCompletionTracker()
	require.NotNil(t, tracker)
}

func TestNewTurnCompletionTrackerWithOptions(t *testing.T) {
	var logCalled bool
	logger := func(format string, args ...any) {
		logCalled = true
	}

	tracker := handler.NewTurnCompletionTrackerWithOptions(handler.WithLogger(logger))
	require.NotNil(t, tracker)

	// Verify logger is used
	tracker.OnMaxRetriesExceeded("worker-1", []string{"post_message"})
	assert.True(t, logCalled, "logger should be called")
}

// ===========================================================================
// RecordToolCall Tests
// ===========================================================================

func TestRecordToolCall_StoresToolName(t *testing.T) {
	tracker := handler.NewTurnCompletionTracker()

	tracker.RecordToolCall("worker-1", "post_message")

	// Verify by checking that CheckTurnCompletion returns empty (compliant)
	missing := tracker.CheckTurnCompletion("worker-1", repository.RoleWorker)
	assert.Empty(t, missing, "should be compliant after recording required tool")
}

func TestRecordToolCall_TracksMultipleTools(t *testing.T) {
	tracker := handler.NewTurnCompletionTracker()

	tracker.RecordToolCall("worker-1", "post_message")
	tracker.RecordToolCall("worker-1", "report_implementation_complete")

	// Should still be compliant
	missing := tracker.CheckTurnCompletion("worker-1", repository.RoleWorker)
	assert.Empty(t, missing)
}

func TestRecordToolCall_TracksMultipleProcesses(t *testing.T) {
	tracker := handler.NewTurnCompletionTracker()

	tracker.RecordToolCall("worker-1", "post_message")
	tracker.RecordToolCall("worker-2", "signal_ready")

	// Both should be compliant
	assert.Empty(t, tracker.CheckTurnCompletion("worker-1", repository.RoleWorker))
	assert.Empty(t, tracker.CheckTurnCompletion("worker-2", repository.RoleWorker))
}

// ===========================================================================
// ResetTurn Tests
// ===========================================================================

func TestResetTurn_ClearsToolCalls(t *testing.T) {
	tracker := handler.NewTurnCompletionTracker()

	// Record a tool call
	tracker.RecordToolCall("worker-1", "post_message")
	assert.Empty(t, tracker.CheckTurnCompletion("worker-1", repository.RoleWorker))

	// Reset turn
	tracker.ResetTurn("worker-1")

	// Should now be non-compliant (tool call cleared)
	missing := tracker.CheckTurnCompletion("worker-1", repository.RoleWorker)
	assert.NotEmpty(t, missing)
}

func TestResetTurn_ClearsRetryCount(t *testing.T) {
	tracker := handler.NewTurnCompletionTracker()

	// Increment retry count
	tracker.IncrementRetry("worker-1")
	tracker.IncrementRetry("worker-1")
	assert.False(t, tracker.ShouldRetry("worker-1"), "should not retry after 2 increments")

	// Reset turn
	tracker.ResetTurn("worker-1")

	// Retry count should be reset
	assert.True(t, tracker.ShouldRetry("worker-1"), "should retry after reset")
}

func TestResetTurn_ClearsNewlySpawnedFlag(t *testing.T) {
	tracker := handler.NewTurnCompletionTracker()

	// Mark as newly spawned
	tracker.MarkAsNewlySpawned("worker-1")
	assert.True(t, tracker.IsNewlySpawned("worker-1"))

	// Reset turn (simulates first turn completion)
	tracker.ResetTurn("worker-1")

	// Newly spawned flag should be cleared
	assert.False(t, tracker.IsNewlySpawned("worker-1"))
}

func TestResetTurn_DoesNotAffectOtherProcesses(t *testing.T) {
	tracker := handler.NewTurnCompletionTracker()

	// Setup state for two workers
	tracker.RecordToolCall("worker-1", "post_message")
	tracker.RecordToolCall("worker-2", "post_message")
	tracker.MarkAsNewlySpawned("worker-1")
	tracker.MarkAsNewlySpawned("worker-2")

	// Reset only worker-1
	tracker.ResetTurn("worker-1")

	// Worker-2 should be unaffected
	assert.Empty(t, tracker.CheckTurnCompletion("worker-2", repository.RoleWorker))
	assert.True(t, tracker.IsNewlySpawned("worker-2"))
}

// ===========================================================================
// MarkAsNewlySpawned Tests
// ===========================================================================

func TestMarkAsNewlySpawned_SetsSpawnFlag(t *testing.T) {
	tracker := handler.NewTurnCompletionTracker()

	// Initially not newly spawned
	assert.False(t, tracker.IsNewlySpawned("worker-1"))

	// Mark as newly spawned
	tracker.MarkAsNewlySpawned("worker-1")

	// Now should be newly spawned
	assert.True(t, tracker.IsNewlySpawned("worker-1"))
}

// ===========================================================================
// IsNewlySpawned Tests
// ===========================================================================

func TestIsNewlySpawned_ReturnsTrueForSpawnedProcesses(t *testing.T) {
	tracker := handler.NewTurnCompletionTracker()

	tracker.MarkAsNewlySpawned("worker-1")

	assert.True(t, tracker.IsNewlySpawned("worker-1"))
}

func TestIsNewlySpawned_ReturnsFalseForUnknownProcess(t *testing.T) {
	tracker := handler.NewTurnCompletionTracker()

	assert.False(t, tracker.IsNewlySpawned("unknown-worker"))
}

func TestIsNewlySpawned_ReturnsFalseAfterReset(t *testing.T) {
	tracker := handler.NewTurnCompletionTracker()

	tracker.MarkAsNewlySpawned("worker-1")
	tracker.ResetTurn("worker-1")

	assert.False(t, tracker.IsNewlySpawned("worker-1"))
}

// ===========================================================================
// CheckTurnCompletion Tests
// ===========================================================================

func TestCheckTurnCompletion_ReturnsMissingToolsForWorkers(t *testing.T) {
	tracker := handler.NewTurnCompletionTracker()

	missing := tracker.CheckTurnCompletion("worker-1", repository.RoleWorker)

	assert.NotEmpty(t, missing)
	assert.Contains(t, missing, "post_message")
	assert.Contains(t, missing, "report_implementation_complete")
	assert.Contains(t, missing, "report_review_verdict")
	assert.Contains(t, missing, "signal_ready")
}

func TestCheckTurnCompletion_ReturnsEmptySliceForCoordinators(t *testing.T) {
	tracker := handler.NewTurnCompletionTracker()

	// Even without any tool calls, coordinator should not have enforcement
	missing := tracker.CheckTurnCompletion("coordinator", repository.RoleCoordinator)

	assert.Empty(t, missing, "coordinators should never have missing tools")
}

func TestCheckTurnCompletion_ReturnsEmptyWhenAnyRequiredToolCalled(t *testing.T) {
	testCases := []struct {
		name     string
		toolName string
	}{
		{"post_message", "post_message"},
		{"report_implementation_complete", "report_implementation_complete"},
		{"report_review_verdict", "report_review_verdict"},
		{"signal_ready", "signal_ready"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tracker := handler.NewTurnCompletionTracker()

			tracker.RecordToolCall("worker-1", tc.toolName)

			missing := tracker.CheckTurnCompletion("worker-1", repository.RoleWorker)
			assert.Empty(t, missing, "%s should satisfy turn completion", tc.toolName)
		})
	}
}

func TestCheckTurnCompletion_IgnoresNonRequiredTools(t *testing.T) {
	tracker := handler.NewTurnCompletionTracker()

	// Record non-required tools
	tracker.RecordToolCall("worker-1", "check_messages")
	tracker.RecordToolCall("worker-1", "some_other_tool")

	// Should still have missing tools
	missing := tracker.CheckTurnCompletion("worker-1", repository.RoleWorker)
	assert.NotEmpty(t, missing, "non-required tools should not satisfy turn completion")
}

// ===========================================================================
// ShouldRetry Tests
// ===========================================================================

func TestShouldRetry_ReturnsTrueWhenRetryCountLessThanMax(t *testing.T) {
	tracker := handler.NewTurnCompletionTracker()

	// No retries yet
	assert.True(t, tracker.ShouldRetry("worker-1"))

	// After 1 retry
	tracker.IncrementRetry("worker-1")
	assert.True(t, tracker.ShouldRetry("worker-1"))
}

func TestShouldRetry_ReturnsFalseWhenRetryCountEqualsMax(t *testing.T) {
	tracker := handler.NewTurnCompletionTracker()

	// max is 2, so after 2 increments should return false
	tracker.IncrementRetry("worker-1")
	tracker.IncrementRetry("worker-1")

	assert.False(t, tracker.ShouldRetry("worker-1"))
}

func TestShouldRetry_ReturnsFalseWhenRetryCountExceedsMax(t *testing.T) {
	tracker := handler.NewTurnCompletionTracker()

	// Increment 3 times
	tracker.IncrementRetry("worker-1")
	tracker.IncrementRetry("worker-1")
	tracker.IncrementRetry("worker-1")

	assert.False(t, tracker.ShouldRetry("worker-1"))
}

// ===========================================================================
// IncrementRetry Tests
// ===========================================================================

func TestIncrementRetry_IncrementsCounter(t *testing.T) {
	tracker := handler.NewTurnCompletionTracker()

	// Initially should retry (count = 0)
	assert.True(t, tracker.ShouldRetry("worker-1"))

	// After increment (count = 1)
	tracker.IncrementRetry("worker-1")
	assert.True(t, tracker.ShouldRetry("worker-1"))

	// After second increment (count = 2)
	tracker.IncrementRetry("worker-1")
	assert.False(t, tracker.ShouldRetry("worker-1"))
}

func TestIncrementRetry_TracksSeparatelyPerProcess(t *testing.T) {
	tracker := handler.NewTurnCompletionTracker()

	// Increment worker-1 twice
	tracker.IncrementRetry("worker-1")
	tracker.IncrementRetry("worker-1")

	// worker-1 should not retry, but worker-2 should
	assert.False(t, tracker.ShouldRetry("worker-1"))
	assert.True(t, tracker.ShouldRetry("worker-2"))
}

// ===========================================================================
// GetReminderMessage Tests
// ===========================================================================

func TestGetReminderMessage_IncludesMissingToolNames(t *testing.T) {
	tracker := handler.NewTurnCompletionTracker()

	message := tracker.GetReminderMessage("worker-1", []string{"post_message", "report_implementation_complete"})

	assert.Contains(t, message, "post_message")
	assert.Contains(t, message, "report_implementation_complete")
	assert.Contains(t, message, "[SYSTEM REMINDER]")
	assert.Contains(t, message, "CRITICAL")
}

func TestGetReminderMessage_ContainsInstructions(t *testing.T) {
	tracker := handler.NewTurnCompletionTracker()

	message := tracker.GetReminderMessage("worker-1", handler.RequiredTools)

	// Should contain helpful instructions
	assert.Contains(t, message, "report_implementation_complete")
	assert.Contains(t, message, "post_message")
	assert.Contains(t, message, "signal_ready")
}

// ===========================================================================
// OnMaxRetriesExceeded Tests
// ===========================================================================

func TestOnMaxRetriesExceeded_CallsLoggerWhenConfigured(t *testing.T) {
	var logMessages []string
	logger := func(format string, args ...any) {
		logMessages = append(logMessages, format)
	}

	tracker := handler.NewTurnCompletionTrackerWithOptions(handler.WithLogger(logger))

	tracker.OnMaxRetriesExceeded("worker-1", []string{"post_message"})

	require.Len(t, logMessages, 1)
	assert.Contains(t, logMessages[0], "exceeded max enforcement retries")
}

func TestOnMaxRetriesExceeded_DoesNotPanicWithoutLogger(t *testing.T) {
	tracker := handler.NewTurnCompletionTracker()

	// Should not panic when logger is nil
	assert.NotPanics(t, func() {
		tracker.OnMaxRetriesExceeded("worker-1", []string{"post_message"})
	})
}

// ===========================================================================
// CleanupProcess Tests
// ===========================================================================

func TestCleanupProcess_RemovesAllStateForProcessID(t *testing.T) {
	tracker := handler.NewTurnCompletionTracker()

	// Setup state for worker-1
	tracker.RecordToolCall("worker-1", "post_message")
	tracker.MarkAsNewlySpawned("worker-1")
	tracker.IncrementRetry("worker-1")

	// Verify state exists
	assert.Empty(t, tracker.CheckTurnCompletion("worker-1", repository.RoleWorker))
	assert.True(t, tracker.IsNewlySpawned("worker-1"))

	// Cleanup
	tracker.CleanupProcess("worker-1")

	// Verify all state is removed
	assert.NotEmpty(t, tracker.CheckTurnCompletion("worker-1", repository.RoleWorker))
	assert.False(t, tracker.IsNewlySpawned("worker-1"))
	assert.True(t, tracker.ShouldRetry("worker-1"), "retry count should be reset")
}

func TestCleanupProcess_DoesNotAffectOtherProcesses(t *testing.T) {
	tracker := handler.NewTurnCompletionTracker()

	// Setup state for two workers
	tracker.RecordToolCall("worker-1", "post_message")
	tracker.RecordToolCall("worker-2", "post_message")
	tracker.MarkAsNewlySpawned("worker-1")
	tracker.MarkAsNewlySpawned("worker-2")
	tracker.IncrementRetry("worker-1")
	tracker.IncrementRetry("worker-2")

	// Cleanup only worker-1
	tracker.CleanupProcess("worker-1")

	// Worker-2 should be unaffected
	assert.Empty(t, tracker.CheckTurnCompletion("worker-2", repository.RoleWorker))
	assert.True(t, tracker.IsNewlySpawned("worker-2"))
}

func TestCleanupProcess_HandlesUnknownProcess(t *testing.T) {
	tracker := handler.NewTurnCompletionTracker()

	// Should not panic on unknown process
	assert.NotPanics(t, func() {
		tracker.CleanupProcess("unknown-worker")
	})
}

// ===========================================================================
// Thread Safety Tests
// ===========================================================================

func TestTurnCompletionTracker_ThreadSafety(t *testing.T) {
	tracker := handler.NewTurnCompletionTracker()

	const goroutines = 100
	const processCount = 10

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()

			processID := "worker-" + string(rune('0'+id%processCount))

			// Mix of operations
			tracker.RecordToolCall(processID, "post_message")
			tracker.CheckTurnCompletion(processID, repository.RoleWorker)
			tracker.MarkAsNewlySpawned(processID)
			tracker.IsNewlySpawned(processID)
			tracker.ShouldRetry(processID)
			tracker.IncrementRetry(processID)
			tracker.ResetTurn(processID)
			tracker.CleanupProcess(processID)
		}(i)
	}

	wg.Wait()
	// If we get here without panic/race, thread safety is verified
}

func TestTurnCompletionTracker_ConcurrentRecordAndCheck(t *testing.T) {
	tracker := handler.NewTurnCompletionTracker()

	var wg sync.WaitGroup
	const iterations = 1000

	// Start recorder goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			tracker.RecordToolCall("worker-1", "post_message")
		}
	}()

	// Start checker goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			tracker.CheckTurnCompletion("worker-1", repository.RoleWorker)
		}
	}()

	// Start resetter goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations/10; i++ {
			tracker.ResetTurn("worker-1")
		}
	}()

	wg.Wait()
}

// ===========================================================================
// RequiredTools Constant Tests
// ===========================================================================

func TestRequiredTools_ContainsExpectedTools(t *testing.T) {
	assert.Contains(t, handler.RequiredTools, "post_message")
	assert.Contains(t, handler.RequiredTools, "report_implementation_complete")
	assert.Contains(t, handler.RequiredTools, "report_review_verdict")
	assert.Contains(t, handler.RequiredTools, "signal_ready")
	assert.Len(t, handler.RequiredTools, 4)
}

// ===========================================================================
// Interface Compliance Test
// ===========================================================================

func TestTurnCompletionTracker_ImplementsEnforcerInterface(t *testing.T) {
	tracker := handler.NewTurnCompletionTracker()

	// Compile-time check via variable assignment
	var enforcer handler.TurnCompletionEnforcer = tracker
	assert.NotNil(t, enforcer)
}
