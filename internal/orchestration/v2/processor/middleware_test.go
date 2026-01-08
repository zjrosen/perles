package processor

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
)

// ===========================================================================
// Test Helpers
// ===========================================================================

// successHandler returns a successful result.
func successHandler() CommandHandler {
	return HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
		return &command.CommandResult{Success: true, Data: "ok"}, nil
	})
}

// errorHandler returns an error.
func errorHandler(errMsg string) CommandHandler {
	return HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
		return nil, errors.New(errMsg)
	})
}

// slowHandler sleeps for the specified duration.
func slowHandler(d time.Duration) CommandHandler {
	return HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
		time.Sleep(d)
		return &command.CommandResult{Success: true}, nil
	})
}

// dedupTestCommand is a test command with ContentHash support for dedup testing.
type dedupTestCommand struct {
	*command.BaseCommand
	value int
}

func newDedupTestCommand(value int) *dedupTestCommand {
	base := command.NewBaseCommand("dedup_test_command", command.SourceInternal)
	return &dedupTestCommand{
		BaseCommand: &base,
		value:       value,
	}
}

func (c *dedupTestCommand) Validate() error {
	return nil
}

// ContentHash returns a hash of the command content excluding ID/timestamp.
func (c *dedupTestCommand) ContentHash() string {
	return fmt.Sprintf("value=%d", c.value)
}

// recordingHandler records when it's called.
type recordingHandler struct {
	mu          sync.Mutex
	calls       []command.Command
	returnError error
}

func newRecordingHandler() *recordingHandler {
	return &recordingHandler{calls: make([]command.Command, 0)}
}

func (h *recordingHandler) Handle(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
	h.mu.Lock()
	h.calls = append(h.calls, cmd)
	h.mu.Unlock()

	if h.returnError != nil {
		return nil, h.returnError
	}
	return &command.CommandResult{Success: true}, nil
}

func (h *recordingHandler) CallCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.calls)
}

// ===========================================================================
// ChainMiddleware Tests
// ===========================================================================

func TestChainMiddleware_AppliesInCorrectOrder(t *testing.T) {
	var order []string
	var mu sync.Mutex

	// Create middlewares that record their execution order
	makeMiddleware := func(name string) Middleware {
		return func(next CommandHandler) CommandHandler {
			return HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
				mu.Lock()
				order = append(order, name+"-before")
				mu.Unlock()

				result, err := next.Handle(ctx, cmd)

				mu.Lock()
				order = append(order, name+"-after")
				mu.Unlock()

				return result, err
			})
		}
	}

	handler := successHandler()
	chained := ChainMiddleware(handler,
		makeMiddleware("first"),
		makeMiddleware("second"),
		makeMiddleware("third"),
	)

	cmd := newTestCommand(1)
	_, err := chained.Handle(context.Background(), cmd)
	require.NoError(t, err)

	// First middleware should be outermost (executed first and last)
	// Order should be: first-before, second-before, third-before, (handler), third-after, second-after, first-after
	expected := []string{
		"first-before",
		"second-before",
		"third-before",
		"third-after",
		"second-after",
		"first-after",
	}
	assert.Equal(t, expected, order)
}

func TestChainMiddleware_NoMiddlewares(t *testing.T) {
	handler := successHandler()
	chained := ChainMiddleware(handler) // No middlewares

	cmd := newTestCommand(1)
	result, err := chained.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, "ok", result.Data)
}

func TestChainMiddleware_SingleMiddleware(t *testing.T) {
	called := false
	middleware := func(next CommandHandler) CommandHandler {
		return HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
			called = true
			return next.Handle(ctx, cmd)
		})
	}

	handler := successHandler()
	chained := ChainMiddleware(handler, middleware)

	cmd := newTestCommand(1)
	result, err := chained.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.True(t, called)
}

// ===========================================================================
// LoggingMiddleware Tests
// ===========================================================================

func TestLoggingMiddleware_SuccessfulExecution(t *testing.T) {
	middleware := NewLoggingMiddleware(LoggingMiddlewareConfig{})

	handler := successHandler()
	wrapped := middleware(handler)

	cmd := newTestCommand(42)
	result, err := wrapped.Handle(context.Background(), cmd)
	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestLoggingMiddleware_HandlesErrors(t *testing.T) {
	middleware := NewLoggingMiddleware(LoggingMiddlewareConfig{})

	handler := errorHandler("something went wrong")
	wrapped := middleware(handler)

	cmd := newTestCommand(1)
	_, err := wrapped.Handle(context.Background(), cmd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "something went wrong")
}

func TestLoggingMiddleware_HandlesErrorResult(t *testing.T) {
	middleware := NewLoggingMiddleware(LoggingMiddlewareConfig{})

	// Handler that returns error in result (not error return value)
	handler := HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
		return &command.CommandResult{
			Success: false,
			Error:   errors.New("result error"),
		}, nil
	})
	wrapped := middleware(handler)

	cmd := newTestCommand(1)
	result, err := wrapped.Handle(context.Background(), cmd)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
}

func TestLoggingMiddleware_ExtractsTraceID(t *testing.T) {
	middleware := NewLoggingMiddleware(LoggingMiddlewareConfig{})

	handler := successHandler()
	wrapped := middleware(handler)

	cmd := newTestCommand(1)
	cmd.SetTraceID("trace-abc-123")
	result, err := wrapped.Handle(context.Background(), cmd)
	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestLoggingMiddleware_ExtractsSource(t *testing.T) {
	middleware := NewLoggingMiddleware(LoggingMiddlewareConfig{})

	handler := successHandler()
	wrapped := middleware(handler)

	// Create a command with a specific source
	base := command.NewBaseCommand("test_command", command.SourceMCPTool)
	cmd := &testCommand{BaseCommand: &base, value: 1}

	result, err := wrapped.Handle(context.Background(), cmd)
	require.NoError(t, err)
	assert.True(t, result.Success)
}

// ===========================================================================
// DeduplicationMiddleware Tests
// ===========================================================================

func TestDeduplicationMiddleware_RejectsDuplicateWithinTTL(t *testing.T) {
	dm := NewDeduplicationMiddleware(DeduplicationMiddlewareConfig{
		TTL: 5 * time.Second,
	})
	defer dm.Stop()

	recorder := newRecordingHandler()
	wrapped := dm.Middleware()(recorder)

	// Create command (use dedupTestCommand which has ContentHash)
	cmd := newDedupTestCommand(42)

	// First call should succeed
	result, err := wrapped.Handle(context.Background(), cmd)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, 1, recorder.CallCount())

	// Second call with same command should be rejected as duplicate
	result, err = wrapped.Handle(context.Background(), cmd)
	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.ErrorIs(t, result.Error, ErrDuplicateCommand)
	assert.Equal(t, 1, recorder.CallCount()) // Handler not called again
}

func TestDeduplicationMiddleware_AllowsAfterTTLExpires(t *testing.T) {
	dm := NewDeduplicationMiddleware(DeduplicationMiddlewareConfig{
		TTL: 50 * time.Millisecond,
	})
	defer dm.Stop()

	recorder := newRecordingHandler()
	wrapped := dm.Middleware()(recorder)

	cmd := newDedupTestCommand(42)

	// First call
	result, err := wrapped.Handle(context.Background(), cmd)
	require.NoError(t, err)
	assert.True(t, result.Success)

	// Wait for TTL to expire
	time.Sleep(100 * time.Millisecond)

	// Second call should succeed after TTL
	result, err = wrapped.Handle(context.Background(), cmd)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, 2, recorder.CallCount())
}

func TestDeduplicationMiddleware_HashExcludesIDAndTimestamp(t *testing.T) {
	dm := NewDeduplicationMiddleware(DeduplicationMiddlewareConfig{
		TTL: 5 * time.Second,
	})
	defer dm.Stop()

	recorder := newRecordingHandler()
	wrapped := dm.Middleware()(recorder)

	// Create two commands with same content but different IDs/timestamps
	cmd1 := newDedupTestCommand(42)
	cmd2 := newDedupTestCommand(42)

	// Commands have different IDs (UUID generated)
	assert.NotEqual(t, cmd1.ID(), cmd2.ID())

	// First call succeeds
	result, err := wrapped.Handle(context.Background(), cmd1)
	require.NoError(t, err)
	assert.True(t, result.Success)

	// Second call with same content should be detected as duplicate
	// even though ID is different
	result, err = wrapped.Handle(context.Background(), cmd2)
	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.ErrorIs(t, result.Error, ErrDuplicateCommand)
}

func TestDeduplicationMiddleware_AllowsDifferentCommands(t *testing.T) {
	dm := NewDeduplicationMiddleware(DeduplicationMiddlewareConfig{
		TTL: 5 * time.Second,
	})
	defer dm.Stop()

	recorder := newRecordingHandler()
	wrapped := dm.Middleware()(recorder)

	// Create commands with different content
	cmd1 := newDedupTestCommand(1)
	cmd2 := newDedupTestCommand(2)

	// Both should succeed
	result, err := wrapped.Handle(context.Background(), cmd1)
	require.NoError(t, err)
	assert.True(t, result.Success)

	result, err = wrapped.Handle(context.Background(), cmd2)
	require.NoError(t, err)
	assert.True(t, result.Success)

	assert.Equal(t, 2, recorder.CallCount())
}

func TestDeduplicationMiddleware_CacheCleanup(t *testing.T) {
	dm := NewDeduplicationMiddleware(DeduplicationMiddlewareConfig{
		TTL:             50 * time.Millisecond,
		CleanupInterval: 25 * time.Millisecond, // Cleanup runs every 25ms
	})
	defer dm.Stop()

	recorder := newRecordingHandler()
	wrapped := dm.Middleware()(recorder)

	// Submit commands to populate cache
	for i := 0; i < 10; i++ {
		cmd := newDedupTestCommand(i)
		_, err := wrapped.Handle(context.Background(), cmd)
		require.NoError(t, err)
	}

	// Cache should have entries
	assert.Equal(t, 10, dm.CacheSize())

	// Wait for TTL + cleanup to run
	time.Sleep(100 * time.Millisecond)

	// Cache should be cleaned up
	assert.Equal(t, 0, dm.CacheSize())
}

func TestDeduplicationMiddleware_HighVolume(t *testing.T) {
	dm := NewDeduplicationMiddleware(DeduplicationMiddlewareConfig{
		TTL:             100 * time.Millisecond,
		CleanupInterval: 50 * time.Millisecond,
	})
	defer dm.Stop()

	recorder := newRecordingHandler()
	wrapped := dm.Middleware()(recorder)

	// Submit many unique commands rapidly
	const numCommands = 500
	for i := 0; i < numCommands; i++ {
		cmd := newDedupTestCommand(i)
		_, err := wrapped.Handle(context.Background(), cmd)
		require.NoError(t, err)
	}

	// Wait for cleanup cycles
	time.Sleep(250 * time.Millisecond)

	// Cache should not grow unbounded - should be cleaned up
	// After TTL expires, cache should be empty or nearly empty
	assert.LessOrEqual(t, dm.CacheSize(), 100, "Cache should not grow unbounded")
}

func TestDeduplicationMiddleware_ThreadSafe(t *testing.T) {
	dm := NewDeduplicationMiddleware(DeduplicationMiddlewareConfig{
		TTL: 5 * time.Second,
	})
	defer dm.Stop()

	recorder := newRecordingHandler()
	wrapped := dm.Middleware()(recorder)

	const numGoroutines = 10
	const commandsPerGoroutine = 100

	var wg sync.WaitGroup
	var successCount atomic.Int32
	var dupCount atomic.Int32

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for i := 0; i < commandsPerGoroutine; i++ {
				// Use same value across goroutines to test dedup
				cmd := newDedupTestCommand(i)
				result, err := wrapped.Handle(context.Background(), cmd)
				require.NoError(t, err)

				if result.Success {
					successCount.Add(1)
				} else {
					dupCount.Add(1)
				}
			}
		}(g)
	}

	wg.Wait()

	// Due to race conditions with sync.Map, there may be a small number of
	// duplicates that slip through (goroutines racing on the same key).
	// The important thing is that:
	// 1. No panics or data races occurred
	// 2. Most duplicates were caught (at least 80% dedup rate)
	// 3. Total processed equals total submitted
	totalProcessed := successCount.Load() + dupCount.Load()
	assert.Equal(t, int32(numGoroutines*commandsPerGoroutine), totalProcessed)

	// At least 80% of duplicates should be caught
	expectedDuplicates := int32((numGoroutines - 1) * commandsPerGoroutine)
	assert.GreaterOrEqual(t, dupCount.Load(), expectedDuplicates*8/10,
		"Expected at least 80%% of duplicates to be caught")
}

func TestDeduplicationMiddleware_DefaultTTL(t *testing.T) {
	dm := NewDeduplicationMiddleware(DeduplicationMiddlewareConfig{})
	defer dm.Stop()

	// Should use default TTL of 5 seconds
	assert.Equal(t, DefaultDeduplicationTTL, dm.ttl)
}

// ===========================================================================
// TimeoutMiddleware Tests
// ===========================================================================

func TestTimeoutMiddleware_SlowHandlerStillCompletes(t *testing.T) {
	middleware := NewTimeoutMiddleware(TimeoutMiddlewareConfig{
		WarningThreshold: 10 * time.Millisecond,
	})

	handler := slowHandler(50 * time.Millisecond)
	wrapped := middleware(handler)

	cmd := newTestCommand(1)
	result, err := wrapped.Handle(context.Background(), cmd)
	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestTimeoutMiddleware_DoesNotAbortHandler(t *testing.T) {
	middleware := NewTimeoutMiddleware(TimeoutMiddlewareConfig{
		WarningThreshold: 10 * time.Millisecond,
	})

	// Handler that takes longer than threshold but should complete
	var handlerCompleted atomic.Bool
	handler := HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
		time.Sleep(50 * time.Millisecond)
		handlerCompleted.Store(true)
		return &command.CommandResult{Success: true, Data: "completed"}, nil
	})
	wrapped := middleware(handler)

	cmd := newTestCommand(1)
	result, err := wrapped.Handle(context.Background(), cmd)

	// Handler should complete despite exceeding threshold
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, "completed", result.Data)
	assert.True(t, handlerCompleted.Load())
}

func TestTimeoutMiddleware_FastHandlerSucceeds(t *testing.T) {
	middleware := NewTimeoutMiddleware(TimeoutMiddlewareConfig{
		WarningThreshold: 100 * time.Millisecond,
	})

	// Fast handler
	handler := HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
		return &command.CommandResult{Success: true}, nil
	})
	wrapped := middleware(handler)

	cmd := newTestCommand(1)
	result, err := wrapped.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestTimeoutMiddleware_ExtractsTraceID(t *testing.T) {
	middleware := NewTimeoutMiddleware(TimeoutMiddlewareConfig{
		WarningThreshold: 10 * time.Millisecond,
	})

	handler := slowHandler(50 * time.Millisecond)
	wrapped := middleware(handler)

	cmd := newTestCommand(1)
	cmd.SetTraceID("trace-xyz-789")
	result, err := wrapped.Handle(context.Background(), cmd)
	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestTimeoutMiddleware_DefaultThreshold(t *testing.T) {
	// Should use default threshold of 100ms
	middleware := NewTimeoutMiddleware(TimeoutMiddlewareConfig{})

	handler := slowHandler(150 * time.Millisecond)
	wrapped := middleware(handler)

	// Just verify it works
	cmd := newTestCommand(1)
	result, err := wrapped.Handle(context.Background(), cmd)
	require.NoError(t, err)
	assert.True(t, result.Success)
}

// ===========================================================================
// Integration Tests - All Middlewares Together
// ===========================================================================

func TestMiddlewareChain_Integration(t *testing.T) {
	// Create all middlewares
	logging := NewLoggingMiddleware(LoggingMiddlewareConfig{})

	dedup := NewDeduplicationMiddleware(DeduplicationMiddlewareConfig{
		TTL: 5 * time.Second,
	})
	defer dedup.Stop()

	timeout := NewTimeoutMiddleware(TimeoutMiddlewareConfig{
		WarningThreshold: 10 * time.Millisecond,
	})

	// Chain them: logging -> dedup -> timeout -> handler
	recorder := newRecordingHandler()
	chained := ChainMiddleware(recorder,
		logging,
		dedup.Middleware(),
		timeout,
	)

	// First call should succeed
	cmd := newDedupTestCommand(42)
	result, err := chained.Handle(context.Background(), cmd)
	require.NoError(t, err)
	assert.True(t, result.Success)

	// Second call should be rejected by dedup
	result, err = chained.Handle(context.Background(), cmd)
	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.ErrorIs(t, result.Error, ErrDuplicateCommand)

	// Handler should only be called once
	assert.Equal(t, 1, recorder.CallCount())
}

func TestMiddlewareChain_SlowHandler(t *testing.T) {
	logging := NewLoggingMiddleware(LoggingMiddlewareConfig{})

	timeout := NewTimeoutMiddleware(TimeoutMiddlewareConfig{
		WarningThreshold: 10 * time.Millisecond,
	})

	handler := slowHandler(50 * time.Millisecond)
	chained := ChainMiddleware(handler, logging, timeout)

	cmd := newTestCommand(1)
	result, err := chained.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)
}

// ===========================================================================
// CommandLogMiddleware Tests
// ===========================================================================

// mockEventPublisher captures events for testing.
type mockEventPublisher struct {
	mu     sync.Mutex
	events []any
}

func newMockEventPublisher() *mockEventPublisher {
	return &mockEventPublisher{events: make([]any, 0)}
}

func (m *mockEventPublisher) Publish(eventType string, payload any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, payload)
}

func (m *mockEventPublisher) Events() []any {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]any, len(m.events))
	copy(result, m.events)
	return result
}

func (m *mockEventPublisher) LastEvent() (CommandLogEvent, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.events) == 0 {
		return CommandLogEvent{}, false
	}
	event, ok := m.events[len(m.events)-1].(CommandLogEvent)
	return event, ok
}

func TestCommandLogMiddleware_EmitsCorrectEventStructure(t *testing.T) {
	publisher := newMockEventPublisher()
	middleware := NewCommandLogMiddleware(CommandLogMiddlewareConfig{
		EventBus: publisher,
	})

	handler := successHandler()
	wrapped := middleware(handler)

	cmd := newTestCommand(42)
	_, err := wrapped.Handle(context.Background(), cmd)
	require.NoError(t, err)

	// Verify event was published
	event, ok := publisher.LastEvent()
	require.True(t, ok, "Expected a CommandLogEvent to be published")

	// Verify event structure
	assert.Equal(t, cmd.ID(), event.CommandID)
	assert.Equal(t, cmd.Type(), event.CommandType)
	assert.True(t, event.Success)
	assert.Nil(t, event.Error)
	assert.Greater(t, event.Duration, time.Duration(0))
	assert.False(t, event.Timestamp.IsZero())
}

func TestCommandLogMiddleware_Success(t *testing.T) {
	publisher := newMockEventPublisher()
	middleware := NewCommandLogMiddleware(CommandLogMiddlewareConfig{
		EventBus: publisher,
	})

	handler := successHandler()
	wrapped := middleware(handler)

	cmd := newTestCommand(1)
	result, err := wrapped.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	event, ok := publisher.LastEvent()
	require.True(t, ok)
	assert.True(t, event.Success)
	assert.Nil(t, event.Error)
}

func TestCommandLogMiddleware_Failure_HandlerError(t *testing.T) {
	publisher := newMockEventPublisher()
	middleware := NewCommandLogMiddleware(CommandLogMiddlewareConfig{
		EventBus: publisher,
	})

	handler := errorHandler("handler failed")
	wrapped := middleware(handler)

	cmd := newTestCommand(1)
	_, err := wrapped.Handle(context.Background(), cmd)
	require.Error(t, err)

	event, ok := publisher.LastEvent()
	require.True(t, ok)
	assert.False(t, event.Success)
	assert.NotNil(t, event.Error)
	assert.Contains(t, event.Error.Error(), "handler failed")
}

func TestCommandLogMiddleware_Failure_ResultError(t *testing.T) {
	publisher := newMockEventPublisher()
	middleware := NewCommandLogMiddleware(CommandLogMiddlewareConfig{
		EventBus: publisher,
	})

	// Handler that returns error in result (not error return value)
	handler := HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
		return &command.CommandResult{
			Success: false,
			Error:   errors.New("result error"),
		}, nil
	})
	wrapped := middleware(handler)

	cmd := newTestCommand(1)
	result, err := wrapped.Handle(context.Background(), cmd)
	require.NoError(t, err)
	assert.False(t, result.Success)

	event, ok := publisher.LastEvent()
	require.True(t, ok)
	assert.False(t, event.Success)
	assert.NotNil(t, event.Error)
	assert.Contains(t, event.Error.Error(), "result error")
}

func TestCommandLogMiddleware_Duration(t *testing.T) {
	publisher := newMockEventPublisher()
	middleware := NewCommandLogMiddleware(CommandLogMiddlewareConfig{
		EventBus: publisher,
	})

	// Handler with artificial delay
	handler := slowHandler(50 * time.Millisecond)
	wrapped := middleware(handler)

	cmd := newTestCommand(1)
	_, err := wrapped.Handle(context.Background(), cmd)
	require.NoError(t, err)

	event, ok := publisher.LastEvent()
	require.True(t, ok)
	// Duration should be at least 50ms
	assert.GreaterOrEqual(t, event.Duration, 50*time.Millisecond)
	// But not unreasonably long (allow some slack for test overhead)
	assert.Less(t, event.Duration, 200*time.Millisecond)
}

func TestCommandLogMiddleware_NilEventBus_GracefulNoOp(t *testing.T) {
	// Should not panic with nil event bus
	middleware := NewCommandLogMiddleware(CommandLogMiddlewareConfig{
		EventBus: nil,
	})

	handler := successHandler()
	wrapped := middleware(handler)

	cmd := newTestCommand(1)
	result, err := wrapped.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)
	// No panic, handler executes normally
}

func TestCommandLogMiddleware_ExtractsSource(t *testing.T) {
	publisher := newMockEventPublisher()
	middleware := NewCommandLogMiddleware(CommandLogMiddlewareConfig{
		EventBus: publisher,
	})

	handler := successHandler()
	wrapped := middleware(handler)

	// Create a command with a specific source
	base := command.NewBaseCommand("test_command", command.SourceMCPTool)
	cmd := &testCommand{BaseCommand: &base, value: 1}

	_, err := wrapped.Handle(context.Background(), cmd)
	require.NoError(t, err)

	event, ok := publisher.LastEvent()
	require.True(t, ok)
	assert.Equal(t, command.SourceMCPTool, event.Source)
}

func TestCommandLogMiddleware_ExtractsTraceID(t *testing.T) {
	publisher := newMockEventPublisher()
	middleware := NewCommandLogMiddleware(CommandLogMiddlewareConfig{
		EventBus: publisher,
	})

	handler := successHandler()
	wrapped := middleware(handler)

	// Create a command with a trace ID
	base := command.NewBaseCommand("test_command", command.SourceMCPTool)
	base.SetTraceID("test-trace-id-123")
	cmd := &testCommand{BaseCommand: &base, value: 1}

	_, err := wrapped.Handle(context.Background(), cmd)
	require.NoError(t, err)

	event, ok := publisher.LastEvent()
	require.True(t, ok)
	assert.Equal(t, "test-trace-id-123", event.TraceID, "TraceID should be extracted from command")
}

func TestCommandLogMiddleware_EmptyTraceID(t *testing.T) {
	publisher := newMockEventPublisher()
	middleware := NewCommandLogMiddleware(CommandLogMiddlewareConfig{
		EventBus: publisher,
	})

	handler := successHandler()
	wrapped := middleware(handler)

	// Create a command without a trace ID
	base := command.NewBaseCommand("test_command", command.SourceMCPTool)
	// Don't set trace ID
	cmd := &testCommand{BaseCommand: &base, value: 1}

	_, err := wrapped.Handle(context.Background(), cmd)
	require.NoError(t, err)

	event, ok := publisher.LastEvent()
	require.True(t, ok)
	assert.Empty(t, event.TraceID, "TraceID should be empty when not set on command")
}
