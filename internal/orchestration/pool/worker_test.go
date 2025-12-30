package pool

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/orchestration/client"
	poolevents "github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/pubsub"
)

// workerTestProcess is a test helper that implements HeadlessProcess for worker tests.
type workerTestProcess struct {
	events    chan client.OutputEvent
	errors    chan error
	sessionID string
	workDir   string
	status    client.ProcessStatus
	done      chan struct{}
}

func newWorkerTestProcess() *workerTestProcess {
	return &workerTestProcess{
		events: make(chan client.OutputEvent, 100),
		errors: make(chan error, 10),
		status: client.StatusRunning,
		done:   make(chan struct{}),
	}
}

func (p *workerTestProcess) Events() <-chan client.OutputEvent { return p.events }
func (p *workerTestProcess) Errors() <-chan error              { return p.errors }
func (p *workerTestProcess) SessionRef() string                { return p.sessionID }
func (p *workerTestProcess) Status() client.ProcessStatus      { return p.status }
func (p *workerTestProcess) IsRunning() bool                   { return p.status == client.StatusRunning }
func (p *workerTestProcess) WorkDir() string                   { return p.workDir }
func (p *workerTestProcess) PID() int                          { return 0 }
func (p *workerTestProcess) Cancel() error {
	if p.status == client.StatusRunning {
		p.status = client.StatusCancelled
		close(p.events)
		close(p.errors)
		close(p.done)
	}
	return nil
}
func (p *workerTestProcess) Wait() error {
	<-p.done
	return nil
}

func (p *workerTestProcess) SendInitEvent(sessionID, workDir string) {
	p.sessionID = sessionID
	p.workDir = workDir
	p.events <- client.OutputEvent{
		Type:      client.EventSystem,
		SubType:   "init",
		SessionID: sessionID,
		WorkDir:   workDir,
	}
}

func (p *workerTestProcess) SendTextEvent(text string) {
	p.events <- client.OutputEvent{
		Type: client.EventAssistant,
		Message: &client.MessageContent{
			Role:    "assistant",
			Content: []client.ContentBlock{{Type: "text", Text: text}},
		},
	}
}

func (p *workerTestProcess) Complete() {
	if p.status == client.StatusRunning {
		p.status = client.StatusCompleted
		close(p.events)
		close(p.errors)
		close(p.done)
	}
}

func (p *workerTestProcess) Fail(err error) {
	if p.status == client.StatusRunning {
		p.status = client.StatusFailed
		close(p.events)
		close(p.errors)
		close(p.done)
	}
}

func TestWorkerStatus_String(t *testing.T) {
	tests := []struct {
		status   WorkerStatus
		expected string
	}{
		{WorkerReady, "ready"},
		{WorkerWorking, "working"},
		{WorkerRetired, "retired"},
		{WorkerStatus(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.status.String())
		})
	}
}

func TestWorkerStatus_IsDone(t *testing.T) {
	tests := []struct {
		status   WorkerStatus
		expected bool
	}{
		{WorkerReady, false},
		{WorkerWorking, false},
		{WorkerRetired, true},
	}

	for _, tt := range tests {
		t.Run(tt.status.String(), func(t *testing.T) {
			require.Equal(t, tt.expected, tt.status.IsDone())
		})
	}
}

func TestNewWorker(t *testing.T) {
	w := newWorker("worker-1", 50)

	require.Equal(t, "worker-1", w.ID)
	require.Equal(t, "", w.TaskID)                 // No task initially
	require.Equal(t, WorkerWorking, w.GetStatus()) // Starts Working (processing initial prompt)
	require.NotNil(t, w.Output)
	require.Equal(t, 50, w.Output.Capacity())
	require.False(t, w.StartedAt.IsZero()) // StartedAt is set on creation
}

func TestWorker_StatusMethods(t *testing.T) {
	w := newWorker("worker-1", 10)

	// Initial status is Working (processing initial prompt)
	require.Equal(t, WorkerWorking, w.GetStatus())

	// Complete task to transition to Ready
	w.CompleteTask()
	require.Equal(t, WorkerReady, w.GetStatus())

	// Assign task to transition to Working
	err := w.AssignTask("task-1")
	require.NoError(t, err)
	require.Equal(t, WorkerWorking, w.GetStatus())

	// Retire the worker
	w.Retire()
	require.Equal(t, WorkerRetired, w.GetStatus())
}

func TestWorker_SessionIDMethods(t *testing.T) {
	w := newWorker("worker-1", 10)

	require.Empty(t, w.GetSessionID())

	w.setSessionID("session-xyz")
	require.Equal(t, "session-xyz", w.GetSessionID())
}

func TestWorker_PhaseMethods(t *testing.T) {
	w := newWorker("worker-1", 10)

	// Worker starts with PhaseIdle
	require.Equal(t, poolevents.PhaseIdle, w.GetPhase())

	// Set to implementing
	w.SetPhase(poolevents.PhaseImplementing)
	require.Equal(t, poolevents.PhaseImplementing, w.GetPhase())

	// Set to awaiting review
	w.SetPhase(poolevents.PhaseAwaitingReview)
	require.Equal(t, poolevents.PhaseAwaitingReview, w.GetPhase())

	// Set to reviewing
	w.SetPhase(poolevents.PhaseReviewing)
	require.Equal(t, poolevents.PhaseReviewing, w.GetPhase())

	// Set to addressing feedback
	w.SetPhase(poolevents.PhaseAddressingFeedback)
	require.Equal(t, poolevents.PhaseAddressingFeedback, w.GetPhase())

	// Set to committing
	w.SetPhase(poolevents.PhaseCommitting)
	require.Equal(t, poolevents.PhaseCommitting, w.GetPhase())

	// Set back to idle
	w.SetPhase(poolevents.PhaseIdle)
	require.Equal(t, poolevents.PhaseIdle, w.GetPhase())
}

func TestWorker_Phase_ConcurrentAccess(t *testing.T) {
	w := newWorker("worker-1", 10)

	var wg sync.WaitGroup

	// Concurrent phase writes
	phases := []poolevents.WorkerPhase{
		poolevents.PhaseIdle,
		poolevents.PhaseImplementing,
		poolevents.PhaseAwaitingReview,
		poolevents.PhaseReviewing,
		poolevents.PhaseAddressingFeedback,
		poolevents.PhaseCommitting,
	}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				w.SetPhase(phases[j%len(phases)])
			}
		}(i)
	}

	// Concurrent phase reads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = w.GetPhase()
			}
		}()
	}

	wg.Wait()
	// Should not panic - verifies thread safety
}

func TestWorker_ErrorMethods(t *testing.T) {
	w := newWorker("worker-1", 10)

	require.Nil(t, w.GetLastError())

	testErr := errors.New("test error")
	w.setLastError(testErr)
	require.Equal(t, testErr, w.GetLastError())
}

func TestWorker_Duration(t *testing.T) {
	w := newWorker("worker-1", 10)

	// Worker has StartedAt set on creation, so duration > 0
	dur := w.Duration()
	require.GreaterOrEqual(t, dur, time.Duration(0))

	// After setting explicit start time
	w.StartedAt = time.Now().Add(-5 * time.Second)
	dur = w.Duration()
	require.GreaterOrEqual(t, dur, 5*time.Second)
	require.Less(t, dur, 6*time.Second)
}

func TestWorker_AssignTask(t *testing.T) {
	w := newWorker("worker-1", 10)
	// Worker starts in Working state (processing initial prompt)
	require.Equal(t, WorkerWorking, w.GetStatus())

	// Can't assign task while working
	err := w.AssignTask("task-abc")
	require.Error(t, err)

	// Simulate initial prompt completion - worker becomes Ready
	w.CompleteTask()
	require.Equal(t, WorkerReady, w.GetStatus())

	// Now can assign task
	err = w.AssignTask("task-abc")
	require.NoError(t, err)
	require.Equal(t, "task-abc", w.TaskID)
	require.Equal(t, WorkerWorking, w.GetStatus())

	// Can't assign while working
	err = w.AssignTask("task-xyz")
	require.Error(t, err)
}

func TestWorker_CompleteTask(t *testing.T) {
	w := newWorker("worker-1", 10)
	// Worker starts Working, complete initial prompt first
	w.CompleteTask()
	require.Equal(t, WorkerReady, w.GetStatus())

	// Assign a task
	w.AssignTask("task-abc")
	require.Equal(t, WorkerWorking, w.GetStatus())

	// Complete task - returns to Ready but preserves task ID
	w.CompleteTask()
	require.Equal(t, "task-abc", w.TaskID) // Task ID preserved for TUI display
	require.Equal(t, WorkerReady, w.GetStatus())
}

func TestWorker_SetTaskID(t *testing.T) {
	w := newWorker("worker-1", 10)

	// Initially no task
	require.Empty(t, w.GetTaskID())

	// SetTaskID sets the task ID
	w.SetTaskID("task-xyz")
	require.Equal(t, "task-xyz", w.GetTaskID())

	// Can change task ID
	w.SetTaskID("task-abc")
	require.Equal(t, "task-abc", w.GetTaskID())

	// Can clear task ID
	w.SetTaskID("")
	require.Empty(t, w.GetTaskID())
}

func TestWorker_TaskIDPreservedAcrossPhaseTransitions(t *testing.T) {
	w := newWorker("worker-1", 10)
	broker := pubsub.NewBroker[WorkerEvent]()
	defer broker.Close()

	ctx := context.Background()
	events := broker.Subscribe(ctx)

	// Create mock process
	proc := newWorkerTestProcess()

	// Set task ID before starting
	w.TaskID = "task-lifecycle"
	w.Phase = poolevents.PhaseImplementing

	// Start worker
	done := make(chan bool)
	go func() {
		w.start(ctx, proc, broker)
		done <- true
	}()

	// Wait for spawn event
	<-events

	// Complete process successfully
	proc.Complete()

	// Wait for worker to finish
	<-done

	// Task ID should be preserved after process completion
	require.Equal(t, WorkerReady, w.GetStatus())
	require.Equal(t, "task-lifecycle", w.TaskID, "Task ID should be preserved through phase transitions")
}

func TestWorker_Retire(t *testing.T) {
	w := newWorker("worker-1", 10)
	// Worker starts in Working state
	require.Equal(t, WorkerWorking, w.GetStatus())

	// Retire
	w.Retire()
	require.Equal(t, WorkerRetired, w.GetStatus())
}

func TestWorker_Cancel(t *testing.T) {
	w := newWorker("worker-1", 10)
	// Worker starts in Working state
	require.Equal(t, WorkerWorking, w.GetStatus())

	// Cancel without process - just retires
	err := w.Cancel()
	require.NoError(t, err)
	require.Equal(t, WorkerRetired, w.GetStatus())
}

func TestWorker_HandleOutputEvent(t *testing.T) {
	w := newWorker("worker-1", 10)
	broker := pubsub.NewBroker[WorkerEvent]()
	defer broker.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := broker.Subscribe(ctx)

	// Test init event extracts session ID
	initEvent := &client.OutputEvent{
		Type:      client.EventSystem,
		SubType:   "init",
		SessionID: "sess-123",
	}
	w.handleOutputEvent(initEvent, broker)
	require.Equal(t, "sess-123", w.GetSessionID())

	// Test assistant event stores text in buffer
	assistantEvent := &client.OutputEvent{
		Type: client.EventAssistant,
		Message: &client.MessageContent{
			Content: []client.ContentBlock{
				{Type: "text", Text: "Hello from Claude"},
			},
		},
	}
	w.handleOutputEvent(assistantEvent, broker)
	require.Equal(t, []string{"Hello from Claude"}, w.Output.Lines())

	// Test tool result stores output (truncated)
	toolEvent := &client.OutputEvent{
		Type: client.EventToolResult,
		Tool: &client.ToolContent{
			Name:   "Read",
			Output: "file contents",
		},
	}
	w.handleOutputEvent(toolEvent, broker)
	lines := w.Output.Lines()
	require.Len(t, lines, 2)
	require.Equal(t, "[Read] file contents", lines[1])

	// Verify events were forwarded - only assistant events publish WorkerOutput
	// Init events just set session ID, tool_result just writes to buffer
	eventCount := 0
	for {
		select {
		case <-events:
			eventCount++
		case <-time.After(100 * time.Millisecond):
			// Done receiving events
			require.Equal(t, 1, eventCount, "Expected 1 WorkerOutput event from assistant message")
			return
		}
	}
}

func TestWorker_HandleOutputEvent_LongToolOutput(t *testing.T) {
	w := newWorker("worker-1", 10)
	broker := pubsub.NewBroker[WorkerEvent]()
	defer broker.Close()

	// Create tool output > 500 chars
	longOutput := make([]byte, 600)
	for i := range longOutput {
		longOutput[i] = 'x'
	}

	toolEvent := &client.OutputEvent{
		Type: client.EventToolResult,
		Tool: &client.ToolContent{
			Name:   "Read",
			Output: string(longOutput),
		},
	}
	w.handleOutputEvent(toolEvent, broker)

	lines := w.Output.Lines()
	require.Len(t, lines, 1)
	// Should be truncated with "..."
	require.Contains(t, lines[0], "...")
	require.Less(t, len(lines[0]), 550) // "[Read] " + 500 + "..."
}

func TestWorker_HandleError(t *testing.T) {
	w := newWorker("worker-1", 10)
	broker := pubsub.NewBroker[WorkerEvent]()
	defer broker.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := broker.Subscribe(ctx)

	testErr := errors.New("test")
	w.handleError(testErr, broker)

	require.Equal(t, testErr, w.GetLastError())

	// Check event was sent
	select {
	case event := <-events:
		require.Equal(t, poolevents.WorkerError, event.Payload.Type)
		require.Equal(t, testErr, event.Payload.Error)
		require.Equal(t, "worker-1", event.Payload.WorkerID)
	case <-time.After(100 * time.Millisecond):
		require.Fail(t, "Expected error event to be sent")
	}
}

// Note: handleProcessComplete tests removed because they require a real claude.Process
// which we cannot easily mock due to the concrete type dependency.
// The behavior is integration tested when running with actual Claude processes.

// Note: TestSendEvent_NonBlocking removed because sendEvent was replaced by broker.Publish.
// The broker handles non-blocking publishing internally.

func TestWorker_ConcurrentStatusAccess(t *testing.T) {
	w := newWorker("worker-1", 10)

	var wg sync.WaitGroup

	// Concurrent status writes using direct field access (test is in same package)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				w.mu.Lock()
				if n%2 == 0 {
					w.Status = WorkerWorking
				} else {
					w.Status = WorkerReady
				}
				w.mu.Unlock()
			}
		}(i)
	}

	// Concurrent status reads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = w.GetStatus()
			}
		}()
	}

	wg.Wait()
	// Should not panic
}

// TestWorker_Start tests the start method with a mock process
func TestWorker_Start_ContextCancellation(t *testing.T) {
	w := newWorker("worker-1", 10)
	broker := pubsub.NewBroker[WorkerEvent]()
	defer broker.Close()

	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	// Subscribe to broker events
	events := broker.Subscribe(ctx)

	// Create a mock process
	proc := newWorkerTestProcess()

	// Start worker with the mock process
	done := make(chan bool)
	go func() {
		w.start(ctx, proc, broker)
		done <- true
	}()

	// Verify spawned event was sent
	select {
	case event := <-events:
		require.Equal(t, poolevents.WorkerSpawned, event.Payload.Type)
		require.Equal(t, WorkerWorking, event.Payload.Status)
	case <-time.After(time.Second):
		require.Fail(t, "Timeout waiting for spawned event")
	}

	// Cancel context
	cancel()

	// Worker should exit
	select {
	case <-done:
		// Expected
	case <-time.After(time.Second):
		require.Fail(t, "Worker did not exit after context cancellation")
	}
}

// TestWorker_Start_WithMockProcess tests full worker lifecycle with mock process
func TestWorker_Start_WithMockProcess(t *testing.T) {
	w := newWorker("worker-1", 10)
	broker := pubsub.NewBroker[WorkerEvent]()
	defer broker.Close()

	ctx := context.Background()
	events := broker.Subscribe(ctx)

	// Create mock process
	proc := newWorkerTestProcess()

	// Start worker in goroutine
	done := make(chan bool)
	go func() {
		w.start(ctx, proc, broker)
		done <- true
	}()

	// Verify spawned event
	select {
	case event := <-events:
		require.Equal(t, poolevents.WorkerSpawned, event.Payload.Type)
	case <-time.After(time.Second):
		require.Fail(t, "Timeout waiting for spawned event")
	}

	// Send init event
	proc.SendInitEvent("sess-123", "/work")

	// Send text output
	proc.SendTextEvent("Hello from mock AI")

	// Verify output event
	select {
	case event := <-events:
		require.Equal(t, poolevents.WorkerOutput, event.Payload.Type)
		require.Equal(t, "Hello from mock AI", event.Payload.Output)
	case <-time.After(time.Second):
		require.Fail(t, "Timeout waiting for output event")
	}

	// Complete the process
	proc.Complete()

	// Wait for worker to finish
	select {
	case <-done:
		// Expected
	case <-time.After(time.Second):
		require.Fail(t, "Worker did not exit after process completion")
	}

	// Verify worker returned to Ready status
	require.Equal(t, WorkerReady, w.GetStatus())
	require.Equal(t, "sess-123", w.GetSessionID())
}

// TestWorker_HandleProcessComplete_Success tests worker returns to Ready on success
func TestWorker_HandleProcessComplete_Success(t *testing.T) {
	w := newWorker("worker-1", 10)
	broker := pubsub.NewBroker[WorkerEvent]()
	defer broker.Close()

	ctx := context.Background()
	events := broker.Subscribe(ctx)

	// Create and complete a mock process
	proc := newWorkerTestProcess()

	// Start worker
	done := make(chan bool)
	go func() {
		w.start(ctx, proc, broker)
		done <- true
	}()

	// Wait for spawn event
	<-events

	// Complete process successfully
	proc.Complete()

	// Wait for worker to finish
	<-done

	// Worker should be Ready
	require.Equal(t, WorkerReady, w.GetStatus())
}

// TestWorker_HandleProcessComplete_Cancelled tests worker retires on cancellation
func TestWorker_HandleProcessComplete_Cancelled(t *testing.T) {
	w := newWorker("worker-1", 10)
	broker := pubsub.NewBroker[WorkerEvent]()
	defer broker.Close()

	ctx := context.Background()
	events := broker.Subscribe(ctx)

	// Create mock process
	proc := newWorkerTestProcess()

	// Start worker
	done := make(chan bool)
	go func() {
		w.start(ctx, proc, broker)
		done <- true
	}()

	// Wait for spawn event
	<-events

	// Cancel the process
	proc.Cancel()

	// Wait for worker to finish
	<-done

	// Worker should be Retired
	require.Equal(t, WorkerRetired, w.GetStatus())
}

// TestWorker_HandleProcessComplete_Failed tests worker retires on failure
func TestWorker_HandleProcessComplete_Failed(t *testing.T) {
	w := newWorker("worker-1", 10)
	broker := pubsub.NewBroker[WorkerEvent]()
	defer broker.Close()

	ctx := context.Background()
	events := broker.Subscribe(ctx)

	// Create mock process
	proc := newWorkerTestProcess()

	// Start worker
	done := make(chan bool)
	go func() {
		w.start(ctx, proc, broker)
		done <- true
	}()

	// Wait for spawn event
	<-events

	// Fail the process
	proc.Fail(errors.New("test failure"))

	// Wait for worker to finish
	<-done

	// Worker should be Retired
	require.Equal(t, WorkerRetired, w.GetStatus())
}
