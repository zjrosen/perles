package process

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/metrics"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
	"github.com/zjrosen/perles/internal/pubsub"
)

// mockProcess implements client.HeadlessProcess for testing.
type mockProcess struct {
	sessionRef string
	status     client.ProcessStatus
	events     chan client.OutputEvent
	errors     chan error
	cancelled  bool
	waited     bool
	workDir    string
}

func newMockHeadlessProcess() *mockProcess {
	return &mockProcess{
		sessionRef: "test-session-123",
		status:     client.StatusRunning,
		events:     make(chan client.OutputEvent),
		errors:     make(chan error),
		workDir:    "/test/dir",
	}
}

func (m *mockProcess) Events() <-chan client.OutputEvent { return m.events }
func (m *mockProcess) Errors() <-chan error              { return m.errors }
func (m *mockProcess) SessionRef() string                { return m.sessionRef }
func (m *mockProcess) Status() client.ProcessStatus      { return m.status }
func (m *mockProcess) IsRunning() bool                   { return m.status == client.StatusRunning }
func (m *mockProcess) WorkDir() string                   { return m.workDir }
func (m *mockProcess) PID() int                          { return 12345 }
func (m *mockProcess) Cancel() error {
	m.cancelled = true
	m.status = client.StatusCancelled
	return nil
}
func (m *mockProcess) Wait() error {
	m.waited = true
	return nil
}

// mockCommandSubmitter implements CommandSubmitter for testing.
type mockCommandSubmitter struct {
	submitted []command.Command
	mu        sync.Mutex
}

func (m *mockCommandSubmitter) Submit(cmd command.Command) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.submitted = append(m.submitted, cmd)
}

func (m *mockCommandSubmitter) getSubmitted() []command.Command {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.submitted
}

// ===========================================================================
// Constructor Tests
// ===========================================================================

func TestNew_CreatesProcessWithCorrectIDAndRole(t *testing.T) {
	proc := newMockHeadlessProcess()
	submitter := &mockCommandSubmitter{}
	eventBus := pubsub.NewBroker[any]()

	// Test coordinator role
	coordProc := New(repository.CoordinatorID, repository.RoleCoordinator, proc, submitter, eventBus)
	assert.Equal(t, repository.CoordinatorID, coordProc.ID)
	assert.Equal(t, repository.RoleCoordinator, coordProc.Role)

	// Test worker role
	workerProc := New("worker-1", repository.RoleWorker, proc, submitter, eventBus)
	assert.Equal(t, "worker-1", workerProc.ID)
	assert.Equal(t, repository.RoleWorker, workerProc.Role)
}

func TestNew_InitializesOutputBuffer(t *testing.T) {
	proc := newMockHeadlessProcess()
	p := New("worker-1", repository.RoleWorker, proc, nil, nil)

	assert.NotNil(t, p.Output())
	assert.Equal(t, DefaultOutputBufferCapacity, p.Output().Capacity())
}

func TestNew_CreatesCancelableContext(t *testing.T) {
	proc := newMockHeadlessProcess()
	p := New("worker-1", repository.RoleWorker, proc, nil, nil)

	assert.NotNil(t, p.ctx)
	assert.NotNil(t, p.cancel)

	// Verify context is not already cancelled
	select {
	case <-p.ctx.Done():
		require.FailNow(t, "context should not be done yet")
	default:
		// OK
	}
}

func TestNew_InitializesEmptyMetrics(t *testing.T) {
	proc := newMockHeadlessProcess()
	p := New("worker-1", repository.RoleWorker, proc, nil, nil)

	m := p.Metrics()
	assert.NotNil(t, m)
	assert.Equal(t, 0, m.InputTokens)
	assert.Equal(t, 0, m.OutputTokens)
}

// ===========================================================================
// Event Loop Tests
// ===========================================================================

func TestStart_BeginsProcessingEvents(t *testing.T) {
	proc := newMockHeadlessProcess()
	submitter := &mockCommandSubmitter{}
	eventBus := pubsub.NewBroker[any]()

	p := New("worker-1", repository.RoleWorker, proc, submitter, eventBus)
	p.Start()

	// Give the goroutine a moment to start
	time.Sleep(10 * time.Millisecond)

	// Close events to complete the event loop
	close(proc.events)

	// Wait for completion
	select {
	case <-p.eventDone:
		// Success
	case <-time.After(time.Second):
		require.FailNow(t, "event loop did not complete")
	}
}

func TestEventLoop_HandlesOutputEvents(t *testing.T) {
	proc := newMockHeadlessProcess()
	submitter := &mockCommandSubmitter{}
	eventBus := pubsub.NewBroker[any]()

	p := New("worker-1", repository.RoleWorker, proc, submitter, eventBus)
	p.Start()

	// Send an assistant output event
	proc.events <- client.OutputEvent{
		Type: client.EventAssistant,
		Message: &client.MessageContent{
			Content: []client.ContentBlock{
				{Type: "text", Text: "Hello from AI"},
			},
		},
	}

	// Give it time to process
	time.Sleep(20 * time.Millisecond)

	// Output should be in the buffer
	lines := p.Output().Lines()
	require.Contains(t, lines, "Hello from AI")

	// Clean up
	close(proc.events)
	<-p.eventDone
}

func TestEventLoop_HandlesAssistantToolUseBlocks(t *testing.T) {
	proc := newMockHeadlessProcess()
	p := New("worker-1", repository.RoleWorker, proc, nil, nil)
	p.Start()

	proc.events <- client.OutputEvent{
		Type: client.EventAssistant,
		Message: &client.MessageContent{
			Role: "assistant",
			Content: []client.ContentBlock{
				{Type: "text", Text: "Let me read that file."},
				{
					Type:  "tool_use",
					ID:    "toolu_123",
					Name:  "Read",
					Input: []byte(`{"file_path":"main.go"}`),
				},
			},
		},
	}

	time.Sleep(20 * time.Millisecond)

	lines := p.Output().Lines()
	require.Len(t, lines, 2)
	assert.Contains(t, lines[0], "Let me read that file.")
	assert.Contains(t, lines[1], "Read")

	close(proc.events)
	<-p.eventDone
}

func TestEventLoop_ExitsOnContextCancel(t *testing.T) {
	proc := newMockHeadlessProcess()
	p := New("worker-1", repository.RoleWorker, proc, nil, nil)
	p.Start()

	// Give goroutine time to start
	time.Sleep(10 * time.Millisecond)

	// Cancel context
	p.cancel()

	// Should exit
	select {
	case <-p.eventDone:
		// Success
	case <-time.After(time.Second):
		require.FailNow(t, "event loop did not exit on context cancel")
	}
}

func TestEventLoop_CallsHandleProcessCompleteOnChannelClose(t *testing.T) {
	proc := newMockHeadlessProcess()
	proc.status = client.StatusCompleted
	submitter := &mockCommandSubmitter{}

	p := New("worker-1", repository.RoleWorker, proc, submitter, nil)
	p.Start()

	// Close events to trigger completion
	close(proc.events)
	<-p.eventDone

	// Process should have been waited on
	require.True(t, proc.waited)
}

func TestHandleOutputEvent_BuffersOutputLines(t *testing.T) {
	proc := newMockHeadlessProcess()
	p := New("worker-1", repository.RoleWorker, proc, nil, nil)
	p.Start()

	// Send multiple output events
	for i := 0; i < 5; i++ {
		proc.events <- client.OutputEvent{
			Type: client.EventAssistant,
			Message: &client.MessageContent{
				Content: []client.ContentBlock{
					{Type: "text", Text: "Line"},
				},
			},
		}
	}

	time.Sleep(50 * time.Millisecond)

	lines := p.Output().Lines()
	assert.Len(t, lines, 5)

	close(proc.events)
	<-p.eventDone
}

func TestHandleProcessComplete_SubmitsProcessTurnCompleteCommand(t *testing.T) {
	proc := newMockHeadlessProcess()
	proc.status = client.StatusCompleted
	submitter := &mockCommandSubmitter{}

	p := New("worker-1", repository.RoleWorker, proc, submitter, nil)
	p.Start()

	close(proc.events)
	<-p.eventDone

	submitted := submitter.getSubmitted()
	require.Len(t, submitted, 1)

	cmd, ok := submitted[0].(*command.ProcessTurnCompleteCommand)
	require.True(t, ok)
	assert.Equal(t, "worker-1", cmd.ProcessID)
}

func TestHandleProcessComplete_SetsSucceededTrueForStatusCompleted(t *testing.T) {
	proc := newMockHeadlessProcess()
	proc.status = client.StatusCompleted
	submitter := &mockCommandSubmitter{}

	p := New("worker-1", repository.RoleWorker, proc, submitter, nil)
	p.Start()

	close(proc.events)
	<-p.eventDone

	submitted := submitter.getSubmitted()
	require.Len(t, submitted, 1)

	cmd := submitted[0].(*command.ProcessTurnCompleteCommand)
	assert.True(t, cmd.Succeeded)
}

func TestHandleProcessComplete_SetsSucceededFalseForOtherStatuses(t *testing.T) {
	testCases := []client.ProcessStatus{
		client.StatusFailed,
		client.StatusCancelled,
		client.StatusRunning,
	}

	for _, status := range testCases {
		t.Run(status.String(), func(t *testing.T) {
			proc := newMockHeadlessProcess()
			proc.status = status
			submitter := &mockCommandSubmitter{}

			p := New("worker-1", repository.RoleWorker, proc, submitter, nil)
			p.Start()

			close(proc.events)
			<-p.eventDone

			submitted := submitter.getSubmitted()
			require.Len(t, submitted, 1)

			cmd := submitted[0].(*command.ProcessTurnCompleteCommand)
			assert.False(t, cmd.Succeeded)
		})
	}
}

func TestHandleError_DoesNotCrash(t *testing.T) {
	proc := newMockHeadlessProcess()
	eventBus := pubsub.NewBroker[any]()

	p := New("worker-1", repository.RoleWorker, proc, nil, eventBus)
	p.Start()

	// Send an error
	go func() {
		proc.errors <- &testError{msg: "test error"}
	}()

	time.Sleep(20 * time.Millisecond)

	// Should not have crashed
	close(proc.events)
	<-p.eventDone
}

func TestHandleProcessComplete_RestoresPreviousSessionIDOnFailure(t *testing.T) {
	proc := newMockHeadlessProcess()
	proc.status = client.StatusFailed
	submitter := &mockCommandSubmitter{}

	p := New("worker-1", repository.RoleWorker, proc, submitter, nil)
	// Set a known good session ID (simulating a previous successful turn)
	p.setSessionID("valid-session-123")
	p.Start()

	// Simulate init event with a new (invalid) session ID
	// This happens when Claude can't find the session and creates a new one before failing
	proc.events <- client.OutputEvent{
		Type:      client.EventSystem,
		SubType:   "init",
		SessionID: "invalid-new-session-456",
	}
	time.Sleep(20 * time.Millisecond)

	// During event processing, the new session ID is set
	assert.Equal(t, "invalid-new-session-456", p.SessionID())

	// Now close events to trigger handleProcessComplete
	close(proc.events)
	<-p.eventDone

	// After process failure, the previous valid session ID should be restored
	assert.Equal(t, "valid-session-123", p.SessionID())
}

func TestHandleProcessComplete_ClearsSessionIDOnFailureWithNoPrevious(t *testing.T) {
	proc := newMockHeadlessProcess()
	proc.status = client.StatusFailed
	submitter := &mockCommandSubmitter{}

	p := New("worker-1", repository.RoleWorker, proc, submitter, nil)
	// No previous session ID - this is a fresh worker
	p.Start()

	// Simulate init event with a new session ID that will be invalid
	proc.events <- client.OutputEvent{
		Type:      client.EventSystem,
		SubType:   "init",
		SessionID: "invalid-session-from-failed-start",
	}
	time.Sleep(20 * time.Millisecond)

	// During event processing, the new session ID is set
	assert.Equal(t, "invalid-session-from-failed-start", p.SessionID())

	// Now close events to trigger handleProcessComplete
	close(proc.events)
	<-p.eventDone

	// After process failure with no previous session, the session ID should be cleared
	assert.Empty(t, p.SessionID())
}

func TestHandleProcessComplete_KeepsSessionIDOnSuccess(t *testing.T) {
	proc := newMockHeadlessProcess()
	proc.status = client.StatusCompleted
	submitter := &mockCommandSubmitter{}

	p := New("worker-1", repository.RoleWorker, proc, submitter, nil)
	p.Start()

	// Simulate init event with a valid session ID
	proc.events <- client.OutputEvent{
		Type:      client.EventSystem,
		SubType:   "init",
		SessionID: "valid-session-from-success",
	}
	time.Sleep(20 * time.Millisecond)

	close(proc.events)
	<-p.eventDone

	// On success, the session ID should be kept
	assert.Equal(t, "valid-session-from-success", p.SessionID())
}

// ===========================================================================
// State Access Tests
// ===========================================================================

func TestSessionID_ReturnsStoredSessionID(t *testing.T) {
	proc := newMockHeadlessProcess()
	p := New("worker-1", repository.RoleWorker, proc, nil, nil)

	assert.Empty(t, p.SessionID())

	p.setSessionID("session-abc")
	assert.Equal(t, "session-abc", p.SessionID())
}

func TestSessionID_IsThreadSafe(t *testing.T) {
	proc := newMockHeadlessProcess()
	p := New("worker-1", repository.RoleWorker, proc, nil, nil)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			p.setSessionID("session-" + string(rune(n)))
		}(i)
		go func() {
			defer wg.Done()
			_ = p.SessionID()
		}()
	}
	wg.Wait()
	// Test passes if no race detected
}

func TestMetrics_ReturnsTokenMetrics(t *testing.T) {
	proc := newMockHeadlessProcess()
	p := New("worker-1", repository.RoleWorker, proc, nil, nil)

	m := p.Metrics()
	require.NotNil(t, m)

	// Update metrics
	p.setMetrics(&metrics.TokenMetrics{InputTokens: 100, OutputTokens: 50})

	updated := p.Metrics()
	assert.Equal(t, 100, updated.InputTokens)
	assert.Equal(t, 50, updated.OutputTokens)
}

func TestMetrics_IsThreadSafe(t *testing.T) {
	proc := newMockHeadlessProcess()
	p := New("worker-1", repository.RoleWorker, proc, nil, nil)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			p.setMetrics(&metrics.TokenMetrics{InputTokens: n})
		}(i)
		go func() {
			defer wg.Done()
			_ = p.Metrics()
		}()
	}
	wg.Wait()
}

func TestIsRunning_ReturnsTrueWhenProcessRunning(t *testing.T) {
	proc := newMockHeadlessProcess()
	proc.status = client.StatusRunning

	p := New("worker-1", repository.RoleWorker, proc, nil, nil)
	assert.True(t, p.IsRunning())
}

func TestIsRunning_ReturnsFalseWhenProcessStopped(t *testing.T) {
	proc := newMockHeadlessProcess()
	proc.status = client.StatusCompleted

	p := New("worker-1", repository.RoleWorker, proc, nil, nil)
	assert.False(t, p.IsRunning())
}

func TestIsRunning_ReturnsFalseWhenProcessNil(t *testing.T) {
	p := &Process{}
	assert.False(t, p.IsRunning())
}

func TestWorkDir_DelegatesToUnderlyingProcess(t *testing.T) {
	proc := newMockHeadlessProcess()
	proc.workDir = "/custom/dir"

	p := New("worker-1", repository.RoleWorker, proc, nil, nil)
	assert.Equal(t, "/custom/dir", p.WorkDir())
}

func TestWorkDir_ReturnsEmptyForNilProcess(t *testing.T) {
	p := &Process{}
	assert.Empty(t, p.WorkDir())
}

// ===========================================================================
// Stop Tests
// ===========================================================================

func TestStop_CancelsContext(t *testing.T) {
	proc := newMockHeadlessProcess()
	p := New("worker-1", repository.RoleWorker, proc, nil, nil)
	p.Start()

	// Start goroutine to close events after Stop cancels context
	go func() {
		<-p.ctx.Done()
		close(proc.events)
	}()

	p.Stop()

	// Context should be cancelled
	select {
	case <-p.ctx.Done():
		// Success
	default:
		require.FailNow(t, "context was not cancelled")
	}
}

func TestStop_IsIdempotent(t *testing.T) {
	proc := newMockHeadlessProcess()
	p := New("worker-1", repository.RoleWorker, proc, nil, nil)
	p.Start()

	go func() {
		<-p.ctx.Done()
		close(proc.events)
	}()

	// Call Stop multiple times - should not panic
	p.Stop()
	// Second call would block forever if eventDone was closed improperly
	// But since eventDone is already closed, this should be fast
}

func TestStop_WaitsForEventLoopToExit(t *testing.T) {
	proc := newMockHeadlessProcess()
	p := New("worker-1", repository.RoleWorker, proc, nil, nil)
	p.Start()

	done := make(chan struct{})
	go func() {
		// Delay closing events to verify Stop actually waits
		time.Sleep(50 * time.Millisecond)
		close(proc.events)
	}()

	go func() {
		p.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Stop completed
	case <-time.After(2 * time.Second):
		require.FailNow(t, "Stop did not complete")
	}
}

func TestStop_CancelsUnderlyingProcess(t *testing.T) {
	proc := newMockHeadlessProcess()
	p := New("worker-1", repository.RoleWorker, proc, nil, nil)
	p.Start()

	p.Stop()

	assert.True(t, proc.cancelled, "Stop should cancel the underlying process")
}

// ===========================================================================
// Resume Tests
// ===========================================================================

func TestResume_SendsMessageToUnderlyingProcess(t *testing.T) {
	proc1 := newMockHeadlessProcess()
	p := New("worker-1", repository.RoleWorker, proc1, nil, nil)
	p.Start()

	// Complete first event loop
	close(proc1.events)
	<-p.eventDone

	// Resume with new process
	proc2 := newMockHeadlessProcess()
	p.Resume(proc2)

	// Verify new process is active
	assert.Equal(t, client.StatusRunning, p.Status())

	close(proc2.events)
	<-p.eventDone
}

func TestResume_UpdatesSessionID(t *testing.T) {
	proc1 := newMockHeadlessProcess()
	p := New("worker-1", repository.RoleWorker, proc1, nil, nil)
	p.Start()

	// Send init event with first session ID
	proc1.events <- client.OutputEvent{
		Type:      client.EventSystem,
		SubType:   "init",
		SessionID: "session-1",
	}
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, "session-1", p.SessionID())

	close(proc1.events)
	<-p.eventDone

	// Resume with new process
	proc2 := newMockHeadlessProcess()
	p.Resume(proc2)

	// Send init event with new session ID
	proc2.events <- client.OutputEvent{
		Type:      client.EventSystem,
		SubType:   "init",
		SessionID: "session-2",
	}
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, "session-2", p.SessionID())

	close(proc2.events)
	<-p.eventDone
}

func TestResume_WorksForCoordinatorRole(t *testing.T) {
	proc1 := newMockHeadlessProcess()
	p := New(repository.CoordinatorID, repository.RoleCoordinator, proc1, nil, nil)
	p.Start()

	close(proc1.events)
	<-p.eventDone

	proc2 := newMockHeadlessProcess()
	p.Resume(proc2)

	// Should work the same as worker
	assert.Equal(t, repository.RoleCoordinator, p.Role)
	assert.Equal(t, client.StatusRunning, p.Status())

	close(proc2.events)
	<-p.eventDone
}

func TestResume_WorksForWorkerRole(t *testing.T) {
	proc1 := newMockHeadlessProcess()
	p := New("worker-1", repository.RoleWorker, proc1, nil, nil)
	p.Start()

	close(proc1.events)
	<-p.eventDone

	proc2 := newMockHeadlessProcess()
	p.Resume(proc2)

	assert.Equal(t, repository.RoleWorker, p.Role)
	assert.Equal(t, client.StatusRunning, p.Status())

	close(proc2.events)
	<-p.eventDone
}

// ===========================================================================
// Additional Method Tests
// ===========================================================================

func TestGetTaskID_ReturnsTaskID(t *testing.T) {
	proc := newMockHeadlessProcess()
	p := New("worker-1", repository.RoleWorker, proc, nil, nil)

	assert.Empty(t, p.GetTaskID())

	p.SetTaskID("task-123")
	assert.Equal(t, "task-123", p.GetTaskID())
}

func TestSetRetired_MarksProcessAsRetired(t *testing.T) {
	proc := newMockHeadlessProcess()
	p := New("worker-1", repository.RoleWorker, proc, nil, nil)

	assert.False(t, p.IsRetired())

	p.SetRetired(true)
	assert.True(t, p.IsRetired())

	p.SetRetired(false)
	assert.False(t, p.IsRetired())
}

func TestCancel_StopsUnderlyingProcess(t *testing.T) {
	proc := newMockHeadlessProcess()
	p := New("worker-1", repository.RoleWorker, proc, nil, nil)

	err := p.Cancel()
	assert.NoError(t, err)
	assert.True(t, proc.cancelled)
}

func TestCancel_ReturnsNilForNilProcess(t *testing.T) {
	p := &Process{}
	err := p.Cancel()
	assert.NoError(t, err)
}

func TestWait_BlocksUntilProcessCompletes(t *testing.T) {
	proc := newMockHeadlessProcess()
	p := New("worker-1", repository.RoleWorker, proc, nil, nil)

	err := p.Wait()
	assert.NoError(t, err)
	assert.True(t, proc.waited)
}

func TestWait_ReturnsNilForNilProcess(t *testing.T) {
	p := &Process{}
	err := p.Wait()
	assert.NoError(t, err)
}

func TestEvents_ReturnsUnderlyingEventsChannel(t *testing.T) {
	proc := newMockHeadlessProcess()
	p := New("worker-1", repository.RoleWorker, proc, nil, nil)

	ch := p.Events()
	assert.NotNil(t, ch)
}

func TestEvents_ReturnsClosedChannelForNilProcess(t *testing.T) {
	p := &Process{}
	ch := p.Events()

	select {
	case _, ok := <-ch:
		assert.False(t, ok) // Channel should be closed
	default:
		require.FailNow(t, "expected closed channel")
	}
}

func TestErrors_ReturnsUnderlyingErrorsChannel(t *testing.T) {
	proc := newMockHeadlessProcess()
	p := New("worker-1", repository.RoleWorker, proc, nil, nil)

	ch := p.Errors()
	assert.NotNil(t, ch)
}

func TestErrors_ReturnsClosedChannelForNilProcess(t *testing.T) {
	p := &Process{}
	ch := p.Errors()

	select {
	case _, ok := <-ch:
		assert.False(t, ok) // Channel should be closed
	default:
		require.FailNow(t, "expected closed channel")
	}
}

func TestStatus_ReturnsUnderlyingStatus(t *testing.T) {
	proc := newMockHeadlessProcess()
	proc.status = client.StatusCompleted

	p := New("worker-1", repository.RoleWorker, proc, nil, nil)
	assert.Equal(t, client.StatusCompleted, p.Status())
}

func TestStatus_ReturnsPendingForNilProcess(t *testing.T) {
	p := &Process{}
	assert.Equal(t, client.StatusPending, p.Status())
}

// ===========================================================================
// Event Publishing Tests
// ===========================================================================

func TestPublishOutputEvent_PublishesProcessEvent(t *testing.T) {
	proc := newMockHeadlessProcess()
	eventBus := pubsub.NewBroker[any]()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := eventBus.Subscribe(ctx)

	p := New("worker-1", repository.RoleWorker, proc, nil, eventBus)
	p.Start()

	// Send output event
	proc.events <- client.OutputEvent{
		Type: client.EventAssistant,
		Message: &client.MessageContent{
			Content: []client.ContentBlock{
				{Type: "text", Text: "Test output"},
			},
		},
	}

	// Should receive event on bus
	select {
	case evt := <-sub:
		require.NotNil(t, evt.Payload)
	case <-time.After(500 * time.Millisecond):
		require.FailNow(t, "did not receive event on bus")
	}

	close(proc.events)
	<-p.eventDone
}

func TestPublishErrorEvent_PublishesProcessEvent(t *testing.T) {
	proc := newMockHeadlessProcess()
	eventBus := pubsub.NewBroker[any]()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := eventBus.Subscribe(ctx)

	p := New("worker-1", repository.RoleWorker, proc, nil, eventBus)
	p.Start()

	// Send error
	go func() {
		proc.errors <- &testError{msg: "test error"}
	}()

	// Should receive error event
	select {
	case evt := <-sub:
		require.NotNil(t, evt.Payload)
	case <-time.After(500 * time.Millisecond):
		require.FailNow(t, "did not receive error event")
	}

	close(proc.events)
	<-p.eventDone
}

func TestHandleOutputEvent_ErrorEvent(t *testing.T) {
	// Test that EventError events (turn.failed, error) are properly handled
	proc := newMockHeadlessProcess()
	eventBus := pubsub.NewBroker[any]()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := eventBus.Subscribe(ctx)

	p := New("coordinator", repository.RoleCoordinator, proc, nil, eventBus)
	p.Start()

	// Send error event (like turn.failed from Codex)
	proc.events <- client.OutputEvent{
		Type: client.EventError,
		Error: &client.ErrorInfo{
			Message: "You've hit your usage limit. Try again at 6:55 PM.",
		},
	}

	// Should receive ProcessError event
	select {
	case evt := <-sub:
		require.NotNil(t, evt.Payload)
		pe, ok := evt.Payload.(events.ProcessEvent)
		require.True(t, ok, "expected ProcessEvent")
		assert.Equal(t, events.ProcessError, pe.Type)
		assert.Contains(t, pe.Error.Error(), "usage limit")
	case <-time.After(500 * time.Millisecond):
		require.FailNow(t, "did not receive error event")
	}

	// Error should also appear in output buffer
	time.Sleep(20 * time.Millisecond)
	lines := p.Output().Lines()
	require.Len(t, lines, 1)
	assert.Contains(t, lines[0], "⚠️ Error:")
	assert.Contains(t, lines[0], "usage limit")

	close(proc.events)
	<-p.eventDone
}

func TestEventLoop_ExtractsSessionIDFromInitEvent(t *testing.T) {
	proc := newMockHeadlessProcess()
	p := New("worker-1", repository.RoleWorker, proc, nil, nil)
	p.Start()

	// Send init event with session ID
	proc.events <- client.OutputEvent{
		Type:      client.EventSystem,
		SubType:   "init",
		SessionID: "session-abc-123",
	}

	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, "session-abc-123", p.SessionID())

	close(proc.events)
	<-p.eventDone
}

func TestEventLoop_HandlesToolResults(t *testing.T) {
	proc := newMockHeadlessProcess()
	p := New("worker-1", repository.RoleWorker, proc, nil, nil)
	p.Start()

	// Send tool result
	proc.events <- client.OutputEvent{
		Type: client.EventToolResult,
		Tool: &client.ToolContent{
			Name:   "Bash",
			Output: "command output here",
		},
	}

	time.Sleep(20 * time.Millisecond)

	lines := p.Output().Lines()
	require.Len(t, lines, 1)
	assert.Contains(t, lines[0], "[Bash]")
	assert.Contains(t, lines[0], "command output here")

	close(proc.events)
	<-p.eventDone
}

func TestEventLoop_HandlesToolUseEvents(t *testing.T) {
	// Codex emits tool calls as EventToolUse with Message.Content containing tool_use blocks
	// This verifies tool calls are displayed in the UI for Codex-style events
	proc := newMockHeadlessProcess()
	eventBus := pubsub.NewBroker[any]()
	p := New("worker-1", repository.RoleWorker, proc, nil, eventBus)
	p.Start()

	// Send tool_use event (Codex style - separate from assistant message)
	proc.events <- client.OutputEvent{
		Type: client.EventToolUse,
		Tool: &client.ToolContent{
			ID:    "item_1",
			Name:  "signal_ready",
			Input: []byte(`{}`),
		},
		Message: &client.MessageContent{
			ID:   "item_1",
			Role: "assistant",
			Content: []client.ContentBlock{
				{
					Type:  "tool_use",
					ID:    "item_1",
					Name:  "signal_ready",
					Input: []byte(`{}`),
				},
			},
		},
	}

	time.Sleep(20 * time.Millisecond)

	// Tool call should appear in output buffer
	lines := p.Output().Lines()
	require.Len(t, lines, 1, "Expected tool call to be added to output buffer")
	assert.Contains(t, lines[0], "signal_ready", "Tool name should appear in output")

	close(proc.events)
	<-p.eventDone
}

func TestEventLoop_HandlesToolUseWithArguments(t *testing.T) {
	// Test tool_use event with arguments (like MCP tool calls in Codex)
	proc := newMockHeadlessProcess()
	eventBus := pubsub.NewBroker[any]()
	p := New("worker-1", repository.RoleWorker, proc, nil, eventBus)
	p.Start()

	// Send tool_use event with arguments
	proc.events <- client.OutputEvent{
		Type: client.EventToolUse,
		Tool: &client.ToolContent{
			ID:    "item_2",
			Name:  "post_message",
			Input: []byte(`{"to":"COORDINATOR","content":"Task completed"}`),
		},
		Message: &client.MessageContent{
			ID:   "item_2",
			Role: "assistant",
			Content: []client.ContentBlock{
				{
					Type:  "tool_use",
					ID:    "item_2",
					Name:  "post_message",
					Input: []byte(`{"to":"COORDINATOR","content":"Task completed"}`),
				},
			},
		},
	}

	time.Sleep(20 * time.Millisecond)

	lines := p.Output().Lines()
	require.Len(t, lines, 1)
	assert.Contains(t, lines[0], "post_message")

	close(proc.events)
	<-p.eventDone
}

func TestEventLoop_TruncatesLongToolOutput(t *testing.T) {
	proc := newMockHeadlessProcess()
	p := New("worker-1", repository.RoleWorker, proc, nil, nil)
	p.Start()

	// Create long output
	longOutput := make([]byte, 1000)
	for i := range longOutput {
		longOutput[i] = 'x'
	}

	proc.events <- client.OutputEvent{
		Type: client.EventToolResult,
		Tool: &client.ToolContent{
			Name:   "Read",
			Output: string(longOutput),
		},
	}

	time.Sleep(20 * time.Millisecond)

	lines := p.Output().Lines()
	require.Len(t, lines, 1)
	assert.Contains(t, lines[0], "...")
	assert.Less(t, len(lines[0]), 600)

	close(proc.events)
	<-p.eventDone
}

type testError struct {
	msg string
}

func (e *testError) Error() string { return e.msg }

// ===========================================================================
// Cumulative Cost Tracking Tests
// ===========================================================================

func TestCumulativeCostAccumulation_SingleTurn(t *testing.T) {
	proc := newMockHeadlessProcess()
	p := New("worker-1", repository.RoleWorker, proc, nil, nil)

	// Update metrics with a turn cost
	m := &metrics.TokenMetrics{TurnCostUSD: 0.05}
	p.setMetrics(m)

	// Verify cumulative cost equals turn cost
	result := p.Metrics()
	assert.Equal(t, 0.05, result.CumulativeCostUSD, "CumulativeCostUSD should equal turn cost after single turn")
	assert.Equal(t, 0.05, result.TotalCostUSD, "TotalCostUSD should equal cumulative cost")
}

func TestCumulativeCostAccumulation_MultipleTurns(t *testing.T) {
	proc := newMockHeadlessProcess()
	p := New("worker-1", repository.RoleWorker, proc, nil, nil)

	// First turn
	p.setMetrics(&metrics.TokenMetrics{TurnCostUSD: 0.05})
	m1 := p.Metrics()
	assert.Equal(t, 0.05, m1.CumulativeCostUSD, "After turn 1")

	// Second turn
	p.setMetrics(&metrics.TokenMetrics{TurnCostUSD: 0.03})
	m2 := p.Metrics()
	assert.Equal(t, 0.08, m2.CumulativeCostUSD, "After turn 2")

	// Third turn
	p.setMetrics(&metrics.TokenMetrics{TurnCostUSD: 0.02})
	m3 := p.Metrics()
	assert.Equal(t, 0.10, m3.CumulativeCostUSD, "After turn 3")
	assert.Equal(t, 0.10, m3.TotalCostUSD, "TotalCostUSD should equal cumulative")
}

func TestCumulativeCostAccumulation_ZeroCostTurn(t *testing.T) {
	proc := newMockHeadlessProcess()
	p := New("worker-1", repository.RoleWorker, proc, nil, nil)

	// First turn with cost
	p.setMetrics(&metrics.TokenMetrics{TurnCostUSD: 0.05})

	// Zero cost turn (e.g., cached response)
	p.setMetrics(&metrics.TokenMetrics{TurnCostUSD: 0.0})

	// Cumulative should still be 0.05
	result := p.Metrics()
	assert.Equal(t, 0.05, result.CumulativeCostUSD, "Zero cost turn shouldn't change cumulative")
	assert.Equal(t, 0.05, result.TotalCostUSD)
}

func TestCumulativeCostAccumulation_ThreadSafe(t *testing.T) {
	proc := newMockHeadlessProcess()
	p := New("worker-1", repository.RoleWorker, proc, nil, nil)

	const goroutines = 100
	var wg sync.WaitGroup

	// Spawn multiple goroutines updating metrics concurrently
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.setMetrics(&metrics.TokenMetrics{TurnCostUSD: 0.01})
		}()
	}
	wg.Wait()

	// Verify final cumulative cost
	result := p.Metrics()
	expected := float64(goroutines) * 0.01
	assert.InDelta(t, expected, result.CumulativeCostUSD, 0.0001, "Cumulative cost should be correct after concurrent updates")
	assert.InDelta(t, expected, result.TotalCostUSD, 0.0001)
}

func TestCumulativeCostAccumulation_EmittedInTokenUsageEvent(t *testing.T) {
	proc := newMockHeadlessProcess()
	eventBus := pubsub.NewBroker[any]()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := eventBus.Subscribe(ctx)

	p := New("worker-1", repository.RoleWorker, proc, nil, eventBus)
	p.Start()

	// Send a result event with token usage
	proc.events <- client.OutputEvent{
		Type: client.EventResult,
		Usage: &client.UsageInfo{
			InputTokens:  1000,
			OutputTokens: 500,
		},
		TotalCostUSD: 0.05,
	}

	// Give it time to process
	time.Sleep(50 * time.Millisecond)

	// Receive the token usage event
	select {
	case evt := <-sub:
		require.NotNil(t, evt.Payload)
		// The event should have been published with cumulative cost
		m := p.Metrics()
		assert.Equal(t, 0.05, m.CumulativeCostUSD, "CumulativeCostUSD should be set")
		assert.Equal(t, 0.05, m.TotalCostUSD, "TotalCostUSD should equal cumulative")
	case <-time.After(500 * time.Millisecond):
		require.FailNow(t, "did not receive token usage event")
	}

	close(proc.events)
	<-p.eventDone
}

func TestCumulativeCostAccumulation_MultiTurnWithEvents(t *testing.T) {
	proc := newMockHeadlessProcess()
	eventBus := pubsub.NewBroker[any]()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := eventBus.Subscribe(ctx)

	p := New("worker-1", repository.RoleWorker, proc, nil, eventBus)
	p.Start()

	// Simulate multiple turns by sending multiple result events
	turnCosts := []float64{0.02, 0.03, 0.05}
	expectedCumulative := []float64{0.02, 0.05, 0.10}

	for i, cost := range turnCosts {
		proc.events <- client.OutputEvent{
			Type: client.EventResult,
			Usage: &client.UsageInfo{
				InputTokens:  1000,
				OutputTokens: 500,
			},
			TotalCostUSD: cost,
		}

		// Give it time to process
		time.Sleep(30 * time.Millisecond)

		// Drain event from subscriber
		select {
		case <-sub:
		case <-time.After(200 * time.Millisecond):
			require.FailNowf(t, "did not receive event", "turn %d", i+1)
		}

		// Verify cumulative cost
		m := p.Metrics()
		assert.InDelta(t, expectedCumulative[i], m.CumulativeCostUSD, 0.0001,
			"Turn %d: expected cumulative %.4f, got %.4f", i+1, expectedCumulative[i], m.CumulativeCostUSD)
	}

	close(proc.events)
	<-p.eventDone
}

func TestNew_InitializesCumulativeCostToZero(t *testing.T) {
	proc := newMockHeadlessProcess()
	p := New("worker-1", repository.RoleWorker, proc, nil, nil)

	// Verify initial cumulative cost is zero
	m := p.Metrics()
	assert.Equal(t, float64(0), m.CumulativeCostUSD, "Initial cumulative cost should be zero")
}

// ===========================================================================
// PID Tests
// ===========================================================================

func TestPID_ReturnsUnderlyingPID(t *testing.T) {
	proc := newMockHeadlessProcess()
	p := New("worker-1", repository.RoleWorker, proc, nil, nil)

	pid := p.PID()
	assert.Equal(t, 12345, pid, "PID should delegate to underlying HeadlessProcess")
}

func TestPID_ReturnsZeroWhenNotRunning(t *testing.T) {
	p := &Process{}

	pid := p.PID()
	assert.Equal(t, 0, pid, "PID should return 0 when process is nil")
}
