package codex

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/zjrosen/perles/internal/orchestration/client"

	"github.com/stretchr/testify/require"
)

// errTest is a sentinel error for testing
var errTest = errors.New("test error")

// =============================================================================
// Lifecycle Tests - Process struct behavior without actual subprocess spawning
// =============================================================================

// newTestProcess creates a Process struct for testing without spawning a real subprocess.
// This allows testing lifecycle methods, status transitions, and channel behavior.
func newTestProcess() *Process {
	ctx, cancel := context.WithCancel(context.Background())
	bp := client.NewBaseProcess(
		ctx,
		cancel,
		nil, // no cmd
		nil, // no stdout
		nil, // no stderr
		"/test/project",
		client.WithProviderName("codex"),
	)
	bp.SetSessionRef("019b6dea-903b-7bd3-aef5-202a16205a9a")
	bp.SetStatus(client.StatusRunning)
	return &Process{BaseProcess: bp}
}

func TestProcessLifecycle_StatusTransitions_PendingToRunning(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bp := client.NewBaseProcess(
		ctx,
		cancel,
		nil,
		nil,
		nil,
		"/test/project",
		client.WithProviderName("codex"),
	)
	// Status starts as Pending by default in NewBaseProcess
	p := &Process{BaseProcess: bp}

	require.Equal(t, client.StatusPending, p.Status())
	require.False(t, p.IsRunning())

	bp.SetStatus(client.StatusRunning)
	require.Equal(t, client.StatusRunning, p.Status())
	require.True(t, p.IsRunning())
}

func TestProcessLifecycle_StatusTransitions_RunningToCompleted(t *testing.T) {
	p := newTestProcess()

	require.Equal(t, client.StatusRunning, p.Status())
	require.True(t, p.IsRunning())

	p.BaseProcess.SetStatus(client.StatusCompleted)
	require.Equal(t, client.StatusCompleted, p.Status())
	require.False(t, p.IsRunning())
}

func TestProcessLifecycle_StatusTransitions_RunningToFailed(t *testing.T) {
	p := newTestProcess()

	require.Equal(t, client.StatusRunning, p.Status())
	require.True(t, p.IsRunning())

	p.BaseProcess.SetStatus(client.StatusFailed)
	require.Equal(t, client.StatusFailed, p.Status())
	require.False(t, p.IsRunning())
}

func TestProcessLifecycle_StatusTransitions_RunningToCancelled(t *testing.T) {
	p := newTestProcess()

	require.Equal(t, client.StatusRunning, p.Status())
	require.True(t, p.IsRunning())

	err := p.Cancel()
	require.NoError(t, err)
	require.Equal(t, client.StatusCancelled, p.Status())
	require.False(t, p.IsRunning())
}

func TestProcessLifecycle_Cancel_TerminatesAndSetsStatus(t *testing.T) {
	p := newTestProcess()

	// Verify initial state
	require.Equal(t, client.StatusRunning, p.Status())

	// Cancel should set status to Cancelled
	err := p.Cancel()
	require.NoError(t, err)
	require.Equal(t, client.StatusCancelled, p.Status())

	// Context should be cancelled
	select {
	case <-p.Context().Done():
		// Expected - context was cancelled
	default:
		require.Fail(t, "Context should be cancelled after Cancel()")
	}
}

func TestProcessLifecycle_Cancel_RacePrevention(t *testing.T) {
	// This test verifies that Cancel() sets status BEFORE calling cancelFunc,
	// preventing race conditions with goroutines that check status.
	// Run multiple iterations to catch potential race conditions.

	for i := 0; i < 100; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		bp := client.NewBaseProcess(
			ctx,
			cancel,
			nil,
			nil,
			nil,
			"/test/project",
			client.WithProviderName("codex"),
		)
		bp.SetStatus(client.StatusRunning)
		p := &Process{BaseProcess: bp}

		// Track status seen by a goroutine that races with Cancel
		var observedStatus client.ProcessStatus
		var wg sync.WaitGroup
		wg.Add(1)

		go func() {
			defer wg.Done()
			// Wait for context cancellation
			<-p.Context().Done()
			// Immediately check status - should already be StatusCancelled
			observedStatus = p.Status()
		}()

		// Small sleep to ensure goroutine is waiting
		time.Sleep(time.Microsecond)

		// Cancel the process
		p.Cancel()

		wg.Wait()

		// The goroutine should have seen StatusCancelled, not StatusRunning
		require.Equal(t, client.StatusCancelled, observedStatus,
			"Goroutine should see StatusCancelled after context cancel (iteration %d)", i)
	}
}

func TestProcessLifecycle_Cancel_DoesNotOverrideTerminalState(t *testing.T) {
	tests := []struct {
		name           string
		initialStatus  client.ProcessStatus
		expectedStatus client.ProcessStatus
	}{
		{
			name:           "does not override completed",
			initialStatus:  client.StatusCompleted,
			expectedStatus: client.StatusCompleted,
		},
		{
			name:           "does not override failed",
			initialStatus:  client.StatusFailed,
			expectedStatus: client.StatusFailed,
		},
		{
			name:           "does not override already cancelled",
			initialStatus:  client.StatusCancelled,
			expectedStatus: client.StatusCancelled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			bp := client.NewBaseProcess(
				ctx,
				cancel,
				nil,
				nil,
				nil,
				"/test/project",
				client.WithProviderName("codex"),
			)
			bp.SetStatus(tt.initialStatus)
			p := &Process{BaseProcess: bp}

			err := p.Cancel()
			require.NoError(t, err)
			require.Equal(t, tt.expectedStatus, p.Status())
		})
	}
}

func TestProcessLifecycle_SessionRef_ReturnsThreadID(t *testing.T) {
	p := newTestProcess()

	// SessionRef should return the thread_id (session ID for Codex)
	require.Equal(t, "019b6dea-903b-7bd3-aef5-202a16205a9a", p.SessionRef())
}

func TestProcessLifecycle_SessionRef_InitiallyEmpty(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bp := client.NewBaseProcess(
		ctx,
		cancel,
		nil,
		nil,
		nil,
		"/test/project",
		client.WithProviderName("codex"),
	)
	bp.SetStatus(client.StatusRunning)
	// Don't set session ref
	p := &Process{BaseProcess: bp}

	// SessionRef should be empty until thread.started event is processed
	require.Equal(t, "", p.SessionRef())
}

func TestProcessLifecycle_WorkDir(t *testing.T) {
	p := newTestProcess()
	require.Equal(t, "/test/project", p.WorkDir())
}

func TestProcessLifecycle_PID_NilProcess(t *testing.T) {
	p := newTestProcess()
	// cmd is nil, so PID should return -1 (BaseProcess returns -1 for nil)
	require.Equal(t, -1, p.PID())
}

func TestProcessLifecycle_Wait_BlocksUntilCompletion(t *testing.T) {
	p := newTestProcess()

	// Add a WaitGroup counter to simulate goroutines
	p.WaitGroup().Add(1)

	// Wait should block until wg is done
	done := make(chan bool)
	go func() {
		p.Wait()
		done <- true
	}()

	// Wait should be blocking
	select {
	case <-done:
		require.Fail(t, "Wait should be blocking")
	case <-time.After(10 * time.Millisecond):
		// Expected - still waiting
	}

	// Release the waitgroup
	p.WaitGroup().Done()

	// Wait should now complete
	select {
	case <-done:
		// Expected - Wait completed
	case <-time.After(time.Second):
		require.Fail(t, "Wait should have completed after wg.Done()")
	}
}

func TestProcessLifecycle_SendError_NonBlocking(t *testing.T) {
	p := newTestProcess()

	// Fill the channel (capacity is 10)
	for i := 0; i < 10; i++ {
		p.ErrorsWritable() <- errTest
	}

	// Channel is now full - SendError should not block
	done := make(chan bool)
	go func() {
		p.SendError(ErrTimeout) // This should not block
		done <- true
	}()

	select {
	case <-done:
		// Expected - sendError returned without blocking
	case <-time.After(100 * time.Millisecond):
		require.Fail(t, "sendError blocked on full channel - should have dropped error")
	}

	// Original errors should still be in channel
	require.Len(t, p.ErrorsWritable(), 10)
}

func TestProcessLifecycle_SendError_SuccessWhenSpaceAvailable(t *testing.T) {
	p := newTestProcess()

	// SendError should send to channel when space available
	p.SendError(ErrTimeout)

	select {
	case err := <-p.Errors():
		require.Equal(t, ErrTimeout, err)
	default:
		require.Fail(t, "Error should have been sent to channel")
	}
}

func TestProcessLifecycle_EventsChannelCapacity(t *testing.T) {
	p := newTestProcess()

	// Events channel should have capacity 100
	require.Equal(t, 100, cap(p.EventsWritable()))
}

func TestProcessLifecycle_ErrorsChannelCapacity(t *testing.T) {
	p := newTestProcess()

	// Errors channel should have capacity 10
	require.Equal(t, 10, cap(p.ErrorsWritable()))
}

func TestProcessLifecycle_EventsChannel(t *testing.T) {
	p := newTestProcess()

	// Events channel should be readable
	eventsCh := p.Events()
	require.NotNil(t, eventsCh)

	// Send an event
	go func() {
		p.EventsWritable() <- client.OutputEvent{Type: client.EventSystem, SubType: "init"}
	}()

	select {
	case event := <-eventsCh:
		require.Equal(t, client.EventSystem, event.Type)
		require.Equal(t, "init", event.SubType)
	case <-time.After(time.Second):
		require.Fail(t, "Timeout waiting for event")
	}
}

func TestProcessLifecycle_ErrorsChannel(t *testing.T) {
	p := newTestProcess()

	// Errors channel should be readable
	errorsCh := p.Errors()
	require.NotNil(t, errorsCh)

	// Send an error
	go func() {
		p.ErrorsWritable() <- errTest
	}()

	select {
	case err := <-errorsCh:
		require.Equal(t, errTest, err)
	case <-time.After(time.Second):
		require.Fail(t, "Timeout waiting for error")
	}
}

// =============================================================================
// Interface Compliance Tests
// =============================================================================

func TestProcess_ImplementsHeadlessProcess(t *testing.T) {
	// This test verifies at runtime that Process implements HeadlessProcess.
	// The compile-time check in process.go handles this, but this provides
	// additional runtime verification.
	var p client.HeadlessProcess = newTestProcess()
	require.NotNil(t, p)

	// Verify all interface methods are callable
	_ = p.Events()
	_ = p.Errors()
	_ = p.SessionRef()
	_ = p.Status()
	_ = p.IsRunning()
	_ = p.WorkDir()
	_ = p.PID()
}

// =============================================================================
// ErrTimeout Tests
// =============================================================================

func TestErrTimeout(t *testing.T) {
	require.NotNil(t, ErrTimeout)
	require.Contains(t, ErrTimeout.Error(), "timed out")
}

// =============================================================================
// extractSession Tests
// =============================================================================

func TestExtractSession(t *testing.T) {
	tests := []struct {
		name     string
		event    client.OutputEvent
		rawLine  []byte
		expected string
	}{
		{
			name: "extracts session from init event",
			event: client.OutputEvent{
				Type:      client.EventSystem,
				SubType:   "init",
				SessionID: "thread-123",
			},
			rawLine:  []byte(`{"type":"thread.started","thread_id":"thread-123"}`),
			expected: "thread-123",
		},
		{
			name: "returns empty for non-init event",
			event: client.OutputEvent{
				Type:      client.EventAssistant,
				SessionID: "thread-123",
			},
			rawLine:  []byte(`{"type":"item.completed"}`),
			expected: "",
		},
		{
			name: "returns empty for init event without session ID",
			event: client.OutputEvent{
				Type:    client.EventSystem,
				SubType: "init",
			},
			rawLine:  []byte(`{"type":"thread.started"}`),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractSession(tt.event, tt.rawLine)
			require.Equal(t, tt.expected, result)
		})
	}
}
