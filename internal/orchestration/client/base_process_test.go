package client

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewBaseProcess_Defaults(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.Command("echo", "test")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp/workdir")

	require.NotNil(t, bp)
	require.Equal(t, StatusPending, bp.Status())
	require.Equal(t, "/tmp/workdir", bp.WorkDir())
	require.Equal(t, "base", bp.ProviderName())
	require.False(t, bp.CaptureStderr())
	require.NotNil(t, bp.Events())
	require.NotNil(t, bp.Errors())
}

func TestNewBaseProcess_WithProviderName(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.Command("echo", "test")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp",
		WithProviderName("claude"))

	require.Equal(t, "claude", bp.ProviderName())
}

func TestNewBaseProcess_WithStderrCapture(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.Command("echo", "test")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp",
		WithStderrCapture(true))

	require.True(t, bp.CaptureStderr())
}

func TestNewBaseProcess_WithParseEventFunc(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.Command("echo", "test")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	parseFunc := func(line []byte) (OutputEvent, error) {
		return OutputEvent{Type: EventAssistant}, nil
	}

	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp",
		WithParseEventFunc(parseFunc))

	require.NotNil(t, bp.ParseEventFn())

	// Verify the function works
	event, err := bp.ParseEventFn()([]byte("test"))
	require.NoError(t, err)
	require.Equal(t, EventAssistant, event.Type)
}

func TestNewBaseProcess_WithSessionExtractor(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.Command("echo", "test")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	extractFunc := func(event OutputEvent, rawLine []byte) string {
		return "extracted-session-123"
	}

	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp",
		WithSessionExtractor(extractFunc))

	require.NotNil(t, bp.ExtractSessionFn())

	// Verify the function works
	sessionRef := bp.ExtractSessionFn()(OutputEvent{}, []byte("test"))
	require.Equal(t, "extracted-session-123", sessionRef)
}

func TestNewBaseProcess_WithOnInitEvent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.Command("echo", "test")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	called := false
	initFunc := func(event OutputEvent, rawLine []byte) {
		called = true
	}

	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp",
		WithOnInitEvent(initFunc))

	require.NotNil(t, bp.OnInitEventFn())

	// Verify the function can be called
	bp.OnInitEventFn()(OutputEvent{}, []byte("test"))
	require.True(t, called)
}

func TestNewBaseProcess_MultipleOptions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.Command("echo", "test")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	parseFunc := func(line []byte) (OutputEvent, error) {
		return OutputEvent{Type: EventToolUse}, nil
	}

	extractFunc := func(event OutputEvent, rawLine []byte) string {
		return "session-abc"
	}

	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp",
		WithProviderName("amp"),
		WithStderrCapture(true),
		WithParseEventFunc(parseFunc),
		WithSessionExtractor(extractFunc))

	// Verify all options were applied
	require.Equal(t, "amp", bp.ProviderName())
	require.True(t, bp.CaptureStderr())
	require.NotNil(t, bp.ParseEventFn())
	require.NotNil(t, bp.ExtractSessionFn())

	// Verify functions work correctly
	event, err := bp.ParseEventFn()([]byte("test"))
	require.NoError(t, err)
	require.Equal(t, EventToolUse, event.Type)

	sessionRef := bp.ExtractSessionFn()(OutputEvent{}, []byte("test"))
	require.Equal(t, "session-abc", sessionRef)
}

func TestBaseProcess_Events(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.Command("echo", "test")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp")

	eventsChan := bp.Events()
	require.NotNil(t, eventsChan)

	// Verify we can send to the writable channel and receive from the read-only channel
	go func() {
		bp.EventsWritable() <- OutputEvent{Type: EventAssistant}
	}()

	event := <-eventsChan
	require.Equal(t, EventAssistant, event.Type)
}

func TestBaseProcess_Errors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.Command("echo", "test")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp")

	errorsChan := bp.Errors()
	require.NotNil(t, errorsChan)

	// Verify we can send to the writable channel and receive from the read-only channel
	go func() {
		bp.ErrorsWritable() <- io.EOF
	}()

	err := <-errorsChan
	require.Equal(t, io.EOF, err)
}

func TestBaseProcess_Status(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.Command("echo", "test")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp")

	// Initial status should be Pending
	require.Equal(t, StatusPending, bp.Status())

	// Set status to Running
	bp.SetStatus(StatusRunning)
	require.Equal(t, StatusRunning, bp.Status())

	// Set status to Completed
	bp.SetStatus(StatusCompleted)
	require.Equal(t, StatusCompleted, bp.Status())
}

func TestBaseProcess_IsRunning(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.Command("echo", "test")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp")

	// Initially not running (pending)
	require.False(t, bp.IsRunning())

	// Set to Running
	bp.SetStatus(StatusRunning)
	require.True(t, bp.IsRunning())

	// Set to Completed - no longer running
	bp.SetStatus(StatusCompleted)
	require.False(t, bp.IsRunning())

	// Set to Failed - not running
	bp.SetStatus(StatusFailed)
	require.False(t, bp.IsRunning())

	// Set to Cancelled - not running
	bp.SetStatus(StatusCancelled)
	require.False(t, bp.IsRunning())
}

func TestBaseProcess_WorkDir(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.Command("echo", "test")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/my/custom/dir")

	require.Equal(t, "/my/custom/dir", bp.WorkDir())
}

func TestBaseProcess_PID_NilCmd(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create BaseProcess with nil cmd
	bp := NewBaseProcess(ctx, cancel, nil, nil, nil, "/tmp")

	require.Equal(t, -1, bp.PID())
}

func TestBaseProcess_PID_NilProcess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Command not started yet, so Process will be nil
	cmd := exec.Command("echo", "test")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp")

	// Process is nil before Start()
	require.Equal(t, -1, bp.PID())
}

func TestBaseProcess_PID_WithProcess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start a real process so we get a PID
	cmd := exec.CommandContext(ctx, "sleep", "1")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	err := cmd.Start()
	require.NoError(t, err)
	defer func() {
		cancel()
		_ = cmd.Wait()
	}()

	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp")

	pid := bp.PID()
	require.Greater(t, pid, 0)
}

func TestBaseProcess_SessionRef(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.Command("echo", "test")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp")

	// Initially empty
	require.Empty(t, bp.SessionRef())

	// Set session ref
	bp.SetSessionRef("session-xyz-123")
	require.Equal(t, "session-xyz-123", bp.SessionRef())

	// Update session ref
	bp.SetSessionRef("new-session-456")
	require.Equal(t, "new-session-456", bp.SessionRef())
}

func TestBaseProcess_Stdin(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.Command("cat")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)

	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp")

	// Initially nil
	require.Nil(t, bp.Stdin())

	// Set stdin
	bp.SetStdin(stdin)
	require.NotNil(t, bp.Stdin())
}

func TestBaseProcess_StderrLines(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.Command("echo", "test")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp",
		WithStderrCapture(true))

	// Initially empty
	require.Empty(t, bp.StderrLines())

	// Append lines
	bp.AppendStderrLine("Error line 1")
	bp.AppendStderrLine("Error line 2")

	lines := bp.StderrLines()
	require.Len(t, lines, 2)
	require.Equal(t, "Error line 1", lines[0])
	require.Equal(t, "Error line 2", lines[1])

	// Verify copy semantics - modifying returned slice doesn't affect internal state
	lines[0] = "Modified"
	internalLines := bp.StderrLines()
	require.Equal(t, "Error line 1", internalLines[0])
}

func TestBaseProcess_SendError_Success(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.Command("echo", "test")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp")

	// Send an error
	testErr := io.EOF
	bp.SendError(testErr)

	// Read the error
	select {
	case err := <-bp.Errors():
		require.Equal(t, io.EOF, err)
	default:
		t.Fatal("Expected error to be sent")
	}
}

func TestBaseProcess_SendError_ChannelFull(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.Command("echo", "test")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp")

	// Fill up the error channel (capacity is 10)
	for i := 0; i < 10; i++ {
		bp.ErrorsWritable() <- io.EOF
	}

	// This should not block - error is dropped and logged
	bp.SendError(io.ErrUnexpectedEOF)

	// Channel should still have 10 errors
	count := 0
	for range bp.Errors() {
		count++
		if count >= 10 {
			break
		}
	}
	require.Equal(t, 10, count)
}

func TestBaseProcess_Context(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.Command("echo", "test")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp")

	require.Equal(t, ctx, bp.Context())
}

func TestBaseProcess_CancelFunc(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.Command("echo", "test")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp")

	// Verify cancel func works
	require.NotNil(t, bp.CancelFunc())

	// Call cancel and verify context is cancelled
	bp.CancelFunc()()
	require.Error(t, ctx.Err())
}

func TestBaseProcess_WaitGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.Command("echo", "test")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp")

	require.NotNil(t, bp.WaitGroup())

	// Verify WaitGroup works
	bp.WaitGroup().Add(1)
	done := make(chan struct{})
	go func() {
		bp.WaitGroup().Wait()
		close(done)
	}()

	// Should block
	select {
	case <-done:
		t.Fatal("WaitGroup should be blocking")
	default:
	}

	// Done - should unblock
	bp.WaitGroup().Done()
	<-done
}

func TestBaseProcess_Mutex(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.Command("echo", "test")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp")

	require.NotNil(t, bp.Mutex())

	// Verify mutex works
	bp.Mutex().Lock()
	locked := true
	bp.Mutex().Unlock()
	require.True(t, locked)
}

func TestBaseProcess_Cmd(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.Command("echo", "test")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp")

	require.Equal(t, cmd, bp.Cmd())
}

// mockWriteCloser implements io.WriteCloser for testing
type mockWriteCloser struct {
	*bytes.Buffer
	closed bool
}

func newMockWriteCloser() *mockWriteCloser {
	return &mockWriteCloser{Buffer: new(bytes.Buffer)}
}

func (m *mockWriteCloser) Close() error {
	m.closed = true
	return nil
}

func TestBaseProcess_SetStdin(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.Command("echo", "test")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp")

	// Initially nil
	require.Nil(t, bp.Stdin())

	// Set stdin
	mockStdin := newMockWriteCloser()
	bp.SetStdin(mockStdin)
	require.Equal(t, mockStdin, bp.Stdin())
}

// ============================================================================
// Lifecycle Method Tests
// ============================================================================

func TestBaseProcess_Cancel_SetsStatusBeforeCancelFunc(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancelCalled := false
	statusAtCancel := StatusPending

	// Wrap cancel to capture status at time of cancellation
	wrappedCancel := func() {
		cancelCalled = true
		cancel()
	}

	cmd := exec.Command("sleep", "10")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	bp := NewBaseProcess(ctx, wrappedCancel, cmd, stdout, stderr, "/tmp")
	bp.SetStatus(StatusRunning)

	// Override cancelFunc to capture status when called
	bp.cancelFunc = func() {
		statusAtCancel = bp.Status()
		wrappedCancel()
	}

	err := bp.Cancel()
	require.NoError(t, err)
	require.True(t, cancelCalled, "cancelFunc should have been called")
	require.Equal(t, StatusCancelled, statusAtCancel, "status should be Cancelled BEFORE cancelFunc is called")
	require.Equal(t, StatusCancelled, bp.Status())
}

func TestBaseProcess_Cancel_NoOpWhenTerminal(t *testing.T) {
	tests := []struct {
		name   string
		status ProcessStatus
	}{
		{"Completed", StatusCompleted},
		{"Failed", StatusFailed},
		{"Cancelled", StatusCancelled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			cancelCalled := false
			wrappedCancel := func() {
				cancelCalled = true
				cancel()
			}

			cmd := exec.Command("echo", "test")
			stdout, _ := cmd.StdoutPipe()
			stderr, _ := cmd.StderrPipe()

			bp := NewBaseProcess(ctx, wrappedCancel, cmd, stdout, stderr, "/tmp")
			bp.SetStatus(tt.status)

			err := bp.Cancel()
			require.NoError(t, err)
			require.False(t, cancelCalled, "cancelFunc should NOT be called when already terminal")
			require.Equal(t, tt.status, bp.Status(), "status should not change")
		})
	}
}

func TestBaseProcess_Cancel_ConcurrentCalls(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.Command("echo", "test")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp")
	bp.SetStatus(StatusRunning)

	// Call Cancel concurrently from multiple goroutines
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = bp.Cancel()
		}()
	}
	wg.Wait()

	// Status should be Cancelled
	require.Equal(t, StatusCancelled, bp.Status())
}

func TestBaseProcess_Wait_BlocksUntilComplete(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.Command("echo", "test")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp")

	// Add to WaitGroup
	bp.wg.Add(1)

	waitDone := make(chan struct{})
	go func() {
		_ = bp.Wait()
		close(waitDone)
	}()

	// Should not complete yet
	select {
	case <-waitDone:
		t.Fatal("Wait should block until goroutines complete")
	case <-time.After(50 * time.Millisecond):
		// Expected - still blocking
	}

	// Mark as done
	bp.wg.Done()

	// Now Wait should complete
	select {
	case <-waitDone:
		// Expected
	case <-time.After(time.Second):
		t.Fatal("Wait should complete after wg.Done()")
	}
}

func TestBaseProcess_StartGoroutines_LaunchesThree(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use a real process that outputs something
	cmd := exec.Command("echo", "test")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp",
		WithProviderName("test"),
		WithParseEventFunc(func(line []byte) (OutputEvent, error) {
			return OutputEvent{Type: EventAssistant}, nil
		}))

	// Start the command
	err := cmd.Start()
	require.NoError(t, err)

	// Start goroutines
	bp.StartGoroutines()

	// Wait for completion
	err = bp.Wait()
	require.NoError(t, err)

	// Goroutines should have run and closed channels
	// Events channel should be closed
	select {
	case _, ok := <-bp.Events():
		if ok {
			// Got an event, drain the rest
			for range bp.Events() {
			}
		}
		// Channel is now closed
	case <-time.After(time.Second):
		t.Fatal("Events channel should be closed")
	}
}

// mockReadCloser implements io.ReadCloser for testing
type mockReadCloser struct {
	*bytes.Reader
	closed bool
}

func newMockReadCloser(data string) *mockReadCloser {
	return &mockReadCloser{Reader: bytes.NewReader([]byte(data))}
}

func (m *mockReadCloser) Close() error {
	m.closed = true
	return nil
}

func TestBaseProcess_parseOutput_CallsParseEventFn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Mock stdout with test data
	jsonLine := `{"type":"assistant","message":"hello"}`
	stdout := newMockReadCloser(jsonLine + "\n")
	stderr := newMockReadCloser("")

	eventsParsed := 0
	parseFunc := func(line []byte) (OutputEvent, error) {
		eventsParsed++
		return OutputEvent{Type: EventAssistant}, nil
	}

	cmd := exec.Command("echo", "test")
	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp",
		WithProviderName("test"),
		WithParseEventFunc(parseFunc))

	// Run parseOutput directly
	bp.wg.Add(1)
	go bp.parseOutput()

	// Wait for events
	event := <-bp.Events()
	require.Equal(t, EventAssistant, event.Type)
	require.Equal(t, 1, eventsParsed)

	// Channel should be closed after parsing completes
	bp.wg.Wait()
}

func TestBaseProcess_parseOutput_CallsExtractSessionFnForEveryEvent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Multiple events - extractSessionFn should be called for each
	lines := `{"type":"assistant"}
{"type":"tool_use"}
{"type":"result"}`
	stdout := newMockReadCloser(lines + "\n")
	stderr := newMockReadCloser("")

	extractCallCount := 0
	extractFunc := func(event OutputEvent, rawLine []byte) string {
		extractCallCount++
		if extractCallCount == 2 {
			return "session-from-second-event"
		}
		return ""
	}

	parseFunc := func(line []byte) (OutputEvent, error) {
		return OutputEvent{Type: EventAssistant}, nil
	}

	cmd := exec.Command("echo", "test")
	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp",
		WithProviderName("test"),
		WithParseEventFunc(parseFunc),
		WithSessionExtractor(extractFunc))

	bp.wg.Add(1)
	go bp.parseOutput()

	// Drain events
	for range bp.Events() {
	}
	bp.wg.Wait()

	// extractSessionFn should be called for every event (3 events)
	require.Equal(t, 3, extractCallCount, "extractSessionFn should be called for EVERY event")
	require.Equal(t, "session-from-second-event", bp.SessionRef())
}

func TestBaseProcess_parseOutput_CallsOnInitEventFn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Init event
	stdout := newMockReadCloser(`{"type":"system","subtype":"init","session_id":"abc123"}` + "\n")
	stderr := newMockReadCloser("")

	initCalled := false
	initFunc := func(event OutputEvent, rawLine []byte) {
		initCalled = true
	}

	parseFunc := func(line []byte) (OutputEvent, error) {
		return OutputEvent{Type: EventSystem, SubType: "init"}, nil
	}

	cmd := exec.Command("echo", "test")
	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp",
		WithProviderName("test"),
		WithParseEventFunc(parseFunc),
		WithOnInitEvent(initFunc))

	bp.wg.Add(1)
	go bp.parseOutput()

	// Drain events
	for range bp.Events() {
	}
	bp.wg.Wait()

	require.True(t, initCalled, "onInitEventFn should be called for init event")
}

func TestBaseProcess_parseStderr_CapturesWhenEnabled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stderr := newMockReadCloser("error line 1\nerror line 2\n")
	stdout := newMockReadCloser("")

	cmd := exec.Command("echo", "test")
	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp",
		WithProviderName("test"),
		WithStderrCapture(true))

	bp.wg.Add(1)
	go bp.parseStderr()
	bp.wg.Wait()

	lines := bp.StderrLines()
	require.Len(t, lines, 2)
	require.Equal(t, "error line 1", lines[0])
	require.Equal(t, "error line 2", lines[1])
}

func TestBaseProcess_parseStderr_SkipsWhenDisabled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stderr := newMockReadCloser("error line 1\nerror line 2\n")
	stdout := newMockReadCloser("")

	cmd := exec.Command("echo", "test")
	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp",
		WithProviderName("test"),
		WithStderrCapture(false))

	bp.wg.Add(1)
	go bp.parseStderr()
	bp.wg.Wait()

	// Lines should NOT be captured when capture is disabled
	lines := bp.StderrLines()
	require.Empty(t, lines)
}

func TestBaseProcess_waitForCompletion_SetsStatusCompleted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.Command("echo", "test")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp",
		WithProviderName("test"))
	bp.SetStatus(StatusRunning)

	err := cmd.Start()
	require.NoError(t, err)

	bp.StartGoroutines()
	bp.Wait()

	require.Equal(t, StatusCompleted, bp.Status())
}

func TestBaseProcess_waitForCompletion_SetsStatusFailed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Command that will fail
	cmd := exec.Command("false")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp",
		WithProviderName("test"))
	bp.SetStatus(StatusRunning)

	err := cmd.Start()
	require.NoError(t, err)

	bp.StartGoroutines()
	bp.Wait()

	require.Equal(t, StatusFailed, bp.Status())

	// Should have sent an error
	select {
	case err := <-bp.Errors():
		require.Contains(t, err.Error(), "test process exited")
	default:
		t.Fatal("Expected error to be sent")
	}
}

func TestBaseProcess_waitForCompletion_IncludesStderrInError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Command that will fail
	cmd := exec.Command("sh", "-c", "echo 'stderr error' >&2; exit 1")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp",
		WithProviderName("test"),
		WithStderrCapture(true))
	bp.SetStatus(StatusRunning)

	err := cmd.Start()
	require.NoError(t, err)

	bp.StartGoroutines()
	bp.Wait()

	require.Equal(t, StatusFailed, bp.Status())

	// Error should include stderr
	select {
	case err := <-bp.Errors():
		require.Contains(t, err.Error(), "stderr error")
		require.Contains(t, err.Error(), "test process failed")
	default:
		t.Fatal("Expected error to be sent")
	}
}

func TestBaseProcess_waitForCompletion_DetectsTimeout(t *testing.T) {
	// Create a context that will time out quickly
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Command that will run longer than timeout
	cmd := exec.Command("sleep", "1")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp",
		WithProviderName("test"))
	bp.SetStatus(StatusRunning)

	err := cmd.Start()
	require.NoError(t, err)

	bp.StartGoroutines()
	bp.Wait()

	require.Equal(t, StatusFailed, bp.Status())

	// Should have sent ErrTimeout - use blocking read with timeout
	// The error channel is closed after waitForCompletion, so we can range over it
	select {
	case err := <-bp.Errors():
		require.Equal(t, ErrTimeout, err)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Expected ErrTimeout to be sent")
	}
}

func TestBaseProcess_waitForCompletion_PreservesCancelledStatus(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	cmd := exec.Command("sleep", "1")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	bp := NewBaseProcess(ctx, cancel, cmd, stdout, stderr, "/tmp",
		WithProviderName("test"))
	bp.SetStatus(StatusCancelled) // Pre-set to cancelled

	err := cmd.Start()
	require.NoError(t, err)

	// Cancel to stop the process
	cancel()

	bp.StartGoroutines()
	bp.Wait()

	// Status should remain Cancelled
	require.Equal(t, StatusCancelled, bp.Status())
}

func TestBaseProcess_parseOutput_ScannerError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a reader that returns an error
	errReader := &errorReader{err: io.ErrUnexpectedEOF}
	stderr := newMockReadCloser("")

	cmd := exec.Command("echo", "test")
	bp := NewBaseProcess(ctx, cancel, cmd, errReader, stderr, "/tmp",
		WithProviderName("test"),
		WithParseEventFunc(func(line []byte) (OutputEvent, error) {
			return OutputEvent{Type: EventAssistant}, nil
		}))

	bp.wg.Add(1)
	go bp.parseOutput()
	bp.wg.Wait()

	// Should have sent a scanner error
	select {
	case err := <-bp.Errors():
		require.Contains(t, err.Error(), "scanner error")
	default:
		t.Fatal("Expected scanner error to be sent")
	}
}

// errorReader is a reader that always returns an error
type errorReader struct {
	err error
}

func (e *errorReader) Read(p []byte) (int, error) {
	return 0, e.err
}

func (e *errorReader) Close() error {
	return nil
}

func TestErrTimeout(t *testing.T) {
	require.NotNil(t, ErrTimeout)
	require.Contains(t, ErrTimeout.Error(), "timed out")
}
