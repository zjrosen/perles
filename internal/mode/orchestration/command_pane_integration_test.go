package orchestration

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/processor"
	"github.com/zjrosen/perles/internal/pubsub"
)

// ============================================================================
// Integration Test Helpers
// ============================================================================

// integrationTestCommand is a minimal command implementation for integration testing.
type integrationTestCommand struct {
	*command.BaseCommand
	value int
}

func newIntegrationTestCommand(value int) *integrationTestCommand {
	base := command.NewBaseCommand(command.CmdSpawnProcess, command.SourceInternal)
	return &integrationTestCommand{
		BaseCommand: &base,
		value:       value,
	}
}

func (c *integrationTestCommand) Validate() error {
	return nil
}

// integrationSuccessHandler returns a successful result.
func integrationSuccessHandler() processor.CommandHandler {
	return processor.HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
		return &command.CommandResult{Success: true, Data: "ok"}, nil
	})
}

// integrationErrorHandler returns an error.
func integrationErrorHandler(errMsg string) processor.CommandHandler {
	return processor.HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
		return nil, errors.New(errMsg)
	})
}

// integrationResultErrorHandler returns a result with an error (not a handler error).
func integrationResultErrorHandler(errMsg string) processor.CommandHandler {
	return processor.HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
		return &command.CommandResult{
			Success: false,
			Error:   errors.New(errMsg),
		}, nil
	})
}

// integrationEventBus implements processor.EventPublisher for testing.
// It wraps a pubsub.Broker to allow full integration testing.
type integrationEventBus struct {
	broker *pubsub.Broker[any]
}

func newIntegrationEventBus() *integrationEventBus {
	return &integrationEventBus{
		broker: pubsub.NewBroker[any](),
	}
}

func (b *integrationEventBus) Publish(eventType string, payload any) {
	b.broker.Publish(pubsub.EventType(eventType), payload)
}

func (b *integrationEventBus) Broker() *pubsub.Broker[any] {
	return b.broker
}

// ============================================================================
// End-to-End Integration Tests
// ============================================================================

// TestCommandLogFlow_EndToEnd_Success verifies the full flow:
// command processed → middleware emits event → UI receives → entry displayed
func TestCommandLogFlow_EndToEnd_Success(t *testing.T) {
	// Step 1: Create event bus for the full integration
	eventBus := newIntegrationEventBus()

	// Step 2: Create the command log middleware with the event bus
	middleware := processor.NewCommandLogMiddleware(processor.CommandLogMiddlewareConfig{
		EventBus: eventBus,
	})

	// Step 3: Create a handler and wrap with middleware
	handler := integrationSuccessHandler()
	wrapped := middleware(handler)

	// Step 4: Set up TUI Model with v2Listener connected to the event bus
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := New(Config{})
	m = m.SetSize(120, 40)
	m.ctx = ctx
	m.v2Listener = pubsub.NewContinuousListener(ctx, eventBus.Broker())

	// Verify command pane starts empty
	require.Empty(t, m.commandPane.entries, "command pane should start empty")

	// Subscribe to events before processing command
	sub := eventBus.Broker().Subscribe(ctx)

	// Step 5: Process a command through the middleware
	cmd := newIntegrationTestCommand(42)
	_, err := wrapped.Handle(context.Background(), cmd)
	require.NoError(t, err, "command should succeed")

	// Step 6: Manually receive the event from the subscription
	select {
	case event := <-sub:
		// Process the event through handleV2Event
		m, _ = m.handleV2Event(event)
	case <-time.After(100 * time.Millisecond):
		require.FailNow(t, "timed out waiting for event")
	}

	// Step 7: Verify entry was added to command pane
	require.Len(t, m.commandPane.entries, 1, "should have one entry after successful command")

	entry := m.commandPane.entries[0]
	require.Equal(t, cmd.ID(), entry.CommandID, "CommandID should match")
	require.Equal(t, cmd.Type(), entry.CommandType, "CommandType should match")
	require.Equal(t, command.SourceInternal, entry.Source, "Source should match")
	require.True(t, entry.Success, "Success should be true for successful command")
	require.Empty(t, entry.Error, "Error should be empty for successful command")
	require.Greater(t, entry.Duration, time.Duration(0), "Duration should be positive")
	require.False(t, entry.Timestamp.IsZero(), "Timestamp should be set")

	// Step 8: Verify contentDirty was set for rendering
	require.True(t, m.commandPane.contentDirty, "contentDirty should be set")

	// Step 9: Verify the entry renders correctly
	content := renderCommandContent(m.commandPane.entries, 200)
	require.Contains(t, content, "✓", "should contain success checkmark")
	require.Contains(t, content, "spawn_process", "should contain command type")
	require.Contains(t, content, "[internal]", "should contain source")
}

// TestCommandLogFlow_EndToEnd_Failure verifies the full flow for failed commands:
// command fails → middleware emits event with error → UI receives → entry displayed with red highlight
func TestCommandLogFlow_EndToEnd_Failure(t *testing.T) {
	// Step 1: Create event bus for the full integration
	eventBus := newIntegrationEventBus()

	// Step 2: Create the command log middleware with the event bus
	middleware := processor.NewCommandLogMiddleware(processor.CommandLogMiddlewareConfig{
		EventBus: eventBus,
	})

	// Step 3: Create a failing handler and wrap with middleware
	handler := integrationErrorHandler("handler failed")
	wrapped := middleware(handler)

	// Step 4: Set up TUI Model with v2Listener connected to the event bus
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := New(Config{})
	m = m.SetSize(120, 40)
	m.ctx = ctx
	m.v2Listener = pubsub.NewContinuousListener(ctx, eventBus.Broker())

	// Subscribe to events
	sub := eventBus.Broker().Subscribe(ctx)

	// Step 5: Process a command through the middleware (it will fail)
	cmd := newIntegrationTestCommand(1)
	_, err := wrapped.Handle(context.Background(), cmd)
	require.Error(t, err, "command should fail")

	// Step 6: Manually receive the event from the subscription
	select {
	case event := <-sub:
		// Process the event through handleV2Event
		m, _ = m.handleV2Event(event)
	case <-time.After(100 * time.Millisecond):
		require.FailNow(t, "timed out waiting for event")
	}

	// Step 7: Verify entry was added with failure status
	require.Len(t, m.commandPane.entries, 1, "should have one entry after failed command")

	entry := m.commandPane.entries[0]
	require.Equal(t, cmd.ID(), entry.CommandID, "CommandID should match")
	require.False(t, entry.Success, "Success should be false for failed command")
	require.Equal(t, "handler failed", entry.Error, "Error message should be set")

	// Step 8: Verify the entry renders with failure marker
	content := renderCommandContent(m.commandPane.entries, 200)
	require.Contains(t, content, "✗", "should contain failure marker")
	require.Contains(t, content, "handler failed", "should contain error message")
	require.NotContains(t, content, "✓", "should NOT contain success checkmark")
}

// TestCommandLogFlow_EndToEnd_ResultError verifies handling of result.Error (not handler error).
func TestCommandLogFlow_EndToEnd_ResultError(t *testing.T) {
	// Create event bus
	eventBus := newIntegrationEventBus()

	// Create middleware and handler that returns error in result
	middleware := processor.NewCommandLogMiddleware(processor.CommandLogMiddlewareConfig{
		EventBus: eventBus,
	})
	handler := integrationResultErrorHandler("result error")
	wrapped := middleware(handler)

	// Set up TUI Model
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := New(Config{})
	m = m.SetSize(120, 40)
	m.ctx = ctx
	m.v2Listener = pubsub.NewContinuousListener(ctx, eventBus.Broker())

	// Subscribe to events
	sub := eventBus.Broker().Subscribe(ctx)

	// Process command
	cmd := newIntegrationTestCommand(1)
	result, err := wrapped.Handle(context.Background(), cmd)
	require.NoError(t, err, "handler error should be nil")
	require.False(t, result.Success, "result.Success should be false")

	// Wait and receive event
	select {
	case event := <-sub:
		m, _ = m.handleV2Event(event)
	case <-time.After(100 * time.Millisecond):
		require.FailNow(t, "timed out waiting for event")
	}

	// Verify entry shows failure
	require.Len(t, m.commandPane.entries, 1)
	entry := m.commandPane.entries[0]
	require.False(t, entry.Success, "Success should be false for result error")
	require.Equal(t, "result error", entry.Error, "Error should contain result.Error message")
}

// TestCommandLogFlow_MultipleCommands verifies ordering is preserved across multiple commands.
func TestCommandLogFlow_MultipleCommands(t *testing.T) {
	// Create event bus
	eventBus := newIntegrationEventBus()

	// Create middleware
	middleware := processor.NewCommandLogMiddleware(processor.CommandLogMiddlewareConfig{
		EventBus: eventBus,
	})
	handler := integrationSuccessHandler()
	wrapped := middleware(handler)

	// Set up TUI Model
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := New(Config{})
	m = m.SetSize(120, 40)
	m.ctx = ctx
	m.v2Listener = pubsub.NewContinuousListener(ctx, eventBus.Broker())

	// Subscribe to events
	sub := eventBus.Broker().Subscribe(ctx)

	// Process multiple commands in order
	const numCommands = 10
	commandIDs := make([]string, numCommands)
	for i := 0; i < numCommands; i++ {
		cmd := newIntegrationTestCommand(i)
		commandIDs[i] = cmd.ID()
		_, err := wrapped.Handle(context.Background(), cmd)
		require.NoError(t, err)
	}

	// Receive and process all events
	for i := 0; i < numCommands; i++ {
		select {
		case event := <-sub:
			m, _ = m.handleV2Event(event)
		case <-time.After(100 * time.Millisecond):
			require.FailNow(t, "timed out waiting for event", "event index: %d", i)
		}
	}

	// Verify all entries present in order
	require.Len(t, m.commandPane.entries, numCommands, "should have all entries")
	for i, entry := range m.commandPane.entries {
		require.Equal(t, commandIDs[i], entry.CommandID,
			"entry %d should have correct CommandID", i)
	}

	// Verify rendered content shows all entries
	content := renderCommandContent(m.commandPane.entries, 200)
	lines := splitLines(content)
	require.Len(t, lines, numCommands, "should render all entries as separate lines")
}

// TestCommandLogFlow_BurstCommands verifies handling of rapid command bursts without race conditions.
// This test verifies that events are processed correctly when bursted via direct injection.
func TestCommandLogFlow_BurstCommands(t *testing.T) {
	// Set up TUI Model
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := New(Config{})
	m = m.SetSize(120, 40)
	m.ctx = ctx

	v2Broker := pubsub.NewBroker[any]()
	m.v2Listener = pubsub.NewContinuousListener(ctx, v2Broker)

	// Generate burst of events from multiple goroutines
	const numGoroutines = 5
	const commandsPerGoroutine = 20
	totalCommands := numGoroutines * commandsPerGoroutine

	// Generate all events first
	events := make([]pubsub.Event[any], 0, totalCommands)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for i := 0; i < commandsPerGoroutine; i++ {
				event := pubsub.Event[any]{
					Type: pubsub.UpdatedEvent,
					Payload: processor.CommandLogEvent{
						CommandID:   fmt.Sprintf("burst-%d-%d", goroutineID, i),
						CommandType: command.CmdSpawnProcess,
						Source:      command.SourceInternal,
						Success:     true,
						Duration:    10 * time.Millisecond,
						Timestamp:   time.Now(),
					},
				}
				mu.Lock()
				events = append(events, event)
				mu.Unlock()
			}
		}(g)
	}

	wg.Wait()

	// Process all events (handleV2Event is not thread-safe, so process sequentially)
	for _, event := range events {
		m, _ = m.handleV2Event(event)
	}

	// Verify we got all events
	require.Len(t, m.commandPane.entries, totalCommands,
		"should have all events processed")

	// Verify all entries are valid
	for _, entry := range m.commandPane.entries {
		require.NotEmpty(t, entry.CommandID, "CommandID should not be empty")
		require.True(t, entry.Success, "all commands should succeed")
		require.False(t, entry.Timestamp.IsZero(), "Timestamp should be set")
	}
}

// TestCommandLogFlow_MaxEntriesEviction verifies FIFO eviction works in the real event flow.
// This test uses direct event injection rather than async pubsub to avoid buffer issues.
func TestCommandLogFlow_MaxEntriesEviction(t *testing.T) {
	// Set up TUI Model
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := New(Config{})
	m = m.SetSize(120, 40)
	m.ctx = ctx

	v2Broker := pubsub.NewBroker[any]()
	m.v2Listener = pubsub.NewContinuousListener(ctx, v2Broker)

	// Process more commands than max entries allows by directly injecting events
	numCommands := maxCommandLogEntries + 50

	// Track command IDs
	commandIDs := make([]string, numCommands)
	for i := 0; i < numCommands; i++ {
		commandIDs[i] = fmt.Sprintf("cmd-%d", i)

		// Create event directly and process it
		event := pubsub.Event[any]{
			Type: pubsub.UpdatedEvent,
			Payload: processor.CommandLogEvent{
				CommandID:   commandIDs[i],
				CommandType: command.CmdSpawnProcess,
				Source:      command.SourceInternal,
				Success:     true,
				Duration:    10 * time.Millisecond,
				Timestamp:   time.Now(),
			},
		}
		m, _ = m.handleV2Event(event)
	}

	// Verify max entries limit is enforced
	require.Len(t, m.commandPane.entries, maxCommandLogEntries,
		"should have exactly max entries")

	// Verify FIFO: oldest entries should be evicted
	// The first entries should be from later commands (not cmd-0, cmd-1, etc.)
	firstEntry := m.commandPane.entries[0]
	lastEntry := m.commandPane.entries[len(m.commandPane.entries)-1]

	// First entry should be cmd-50 (0-49 evicted)
	require.Equal(t, commandIDs[50], firstEntry.CommandID,
		"first entry should be command 50 (commands 0-49 should be evicted)")

	// Last entry should be cmd-(numCommands-1)
	require.Equal(t, commandIDs[numCommands-1], lastEntry.CommandID,
		"last entry should be the most recent command")
}

// TestCommandLogFlow_AccumulatesWhenHidden verifies entries accumulate even when pane is hidden.
func TestCommandLogFlow_AccumulatesWhenHidden(t *testing.T) {
	// Create event bus
	eventBus := newIntegrationEventBus()

	// Create middleware
	middleware := processor.NewCommandLogMiddleware(processor.CommandLogMiddlewareConfig{
		EventBus: eventBus,
	})
	handler := integrationSuccessHandler()
	wrapped := middleware(handler)

	// Set up TUI Model - pane hidden by default
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := New(Config{})
	m = m.SetSize(120, 40)
	m.ctx = ctx
	m.v2Listener = pubsub.NewContinuousListener(ctx, eventBus.Broker())

	// Verify pane is hidden by default (no debug mode)
	require.False(t, m.showCommandPane, "pane should be hidden by default")

	// Subscribe to events
	sub := eventBus.Broker().Subscribe(ctx)

	// Process commands while pane is hidden
	const numCommands = 5
	for i := 0; i < numCommands; i++ {
		cmd := newIntegrationTestCommand(i)
		_, err := wrapped.Handle(context.Background(), cmd)
		require.NoError(t, err)
	}

	// Receive and process all events
	for i := 0; i < numCommands; i++ {
		select {
		case event := <-sub:
			m, _ = m.handleV2Event(event)
		case <-time.After(100 * time.Millisecond):
			require.FailNow(t, "timed out waiting for event", "event index: %d", i)
		}
	}

	// Verify entries accumulated even though pane is hidden
	require.Len(t, m.commandPane.entries, numCommands,
		"entries should accumulate even when pane is hidden")

	// Now show the pane - entries should be available
	m.showCommandPane = true
	require.Len(t, m.commandPane.entries, numCommands,
		"entries should be available when pane becomes visible")
}

// TestCommandLogFlow_MixedSuccessFailure verifies mixed success/failure commands render correctly.
func TestCommandLogFlow_MixedSuccessFailure(t *testing.T) {
	// Create event bus
	eventBus := newIntegrationEventBus()

	// Set up TUI Model
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := New(Config{})
	m = m.SetSize(120, 40)
	m.ctx = ctx
	m.v2Listener = pubsub.NewContinuousListener(ctx, eventBus.Broker())

	// Subscribe to events
	sub := eventBus.Broker().Subscribe(ctx)

	// Create success middleware
	successMiddleware := processor.NewCommandLogMiddleware(processor.CommandLogMiddlewareConfig{
		EventBus: eventBus,
	})
	successWrapped := successMiddleware(integrationSuccessHandler())

	// Create failure middleware
	failureMiddleware := processor.NewCommandLogMiddleware(processor.CommandLogMiddlewareConfig{
		EventBus: eventBus,
	})
	failureWrapped := failureMiddleware(integrationErrorHandler("test error"))

	// Alternate success and failure

	// Success
	cmd1 := newIntegrationTestCommand(1)
	_, _ = successWrapped.Handle(context.Background(), cmd1)

	// Failure
	cmd2 := newIntegrationTestCommand(2)
	_, _ = failureWrapped.Handle(context.Background(), cmd2)

	// Success
	cmd3 := newIntegrationTestCommand(3)
	_, _ = successWrapped.Handle(context.Background(), cmd3)

	// Receive all events
	for i := 0; i < 3; i++ {
		select {
		case event := <-sub:
			m, _ = m.handleV2Event(event)
		case <-time.After(100 * time.Millisecond):
			require.FailNow(t, "timed out waiting for event", "event index: %d", i)
		}
	}

	// Verify mixed results
	require.Len(t, m.commandPane.entries, 3)
	require.True(t, m.commandPane.entries[0].Success, "first should be success")
	require.False(t, m.commandPane.entries[1].Success, "second should be failure")
	require.True(t, m.commandPane.entries[2].Success, "third should be success")

	// Verify rendered content shows both markers
	content := renderCommandContent(m.commandPane.entries, 200)
	require.Contains(t, content, "✓", "should contain success checkmarks")
	require.Contains(t, content, "✗", "should contain failure markers")
}

// ============================================================================
// Helper Functions
// ============================================================================

// splitLines splits a string into lines, filtering empty lines.
func splitLines(s string) []string {
	var lines []string
	for _, line := range splitByNewline(s) {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// splitByNewline splits by newline character.
func splitByNewline(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// ============================================================================
// Tests for handleV2Event CommandLogEvent Case
// ============================================================================

// TestHandleV2Event_CommandLogEvent verifies the direct handling of CommandLogEvent.
func TestHandleV2Event_CommandLogEvent(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Create a v2 listener
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	v2Broker := pubsub.NewBroker[any]()
	m.v2Listener = pubsub.NewContinuousListener(ctx, v2Broker)

	// Create a CommandLogEvent
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: processor.CommandLogEvent{
			CommandID:   "test-cmd-123",
			CommandType: command.CmdSpawnProcess,
			Source:      command.SourceMCPTool,
			Success:     true,
			Error:       nil,
			Duration:    100 * time.Millisecond,
			Timestamp:   time.Now(),
		},
	}

	// Handle the event
	m, cmd := m.handleV2Event(event)

	// Verify entry was added
	require.Len(t, m.commandPane.entries, 1, "should have one entry")

	entry := m.commandPane.entries[0]
	require.Equal(t, "test-cmd-123", entry.CommandID)
	require.Equal(t, command.CmdSpawnProcess, entry.CommandType)
	require.Equal(t, command.SourceMCPTool, entry.Source)
	require.True(t, entry.Success)
	require.Empty(t, entry.Error)
	require.Equal(t, 100*time.Millisecond, entry.Duration)

	// Verify contentDirty is set
	require.True(t, m.commandPane.contentDirty)

	// Verify Listen() command is returned
	require.NotNil(t, cmd, "should return Listen() command")
}

// TestHandleV2Event_CommandLogEvent_WithError verifies error conversion to string.
func TestHandleV2Event_CommandLogEvent_WithError(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Create a v2 listener
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	v2Broker := pubsub.NewBroker[any]()
	m.v2Listener = pubsub.NewContinuousListener(ctx, v2Broker)

	// Create a CommandLogEvent with an error
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: processor.CommandLogEvent{
			CommandID:   "test-cmd-456",
			CommandType: command.CmdAssignTask,
			Source:      command.SourceUser,
			Success:     false,
			Error:       errors.New("worker not found"),
			Duration:    50 * time.Millisecond,
			Timestamp:   time.Now(),
		},
	}

	// Handle the event
	m, _ = m.handleV2Event(event)

	// Verify error was converted to string
	require.Len(t, m.commandPane.entries, 1)
	entry := m.commandPane.entries[0]
	require.False(t, entry.Success)
	require.Equal(t, "worker not found", entry.Error, "error should be converted to string")
}

// TestHandleV2Event_CommandLogEvent_MaxEntriesBoundary verifies FIFO at exact boundary.
func TestHandleV2Event_CommandLogEvent_MaxEntriesBoundary(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Create a v2 listener
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	v2Broker := pubsub.NewBroker[any]()
	m.v2Listener = pubsub.NewContinuousListener(ctx, v2Broker)

	// Pre-fill with exactly maxCommandLogEntries entries
	for i := 0; i < maxCommandLogEntries; i++ {
		m.commandPane.entries = append(m.commandPane.entries, CommandLogEntry{
			CommandID:   fmt.Sprintf("pre-fill-%d", i),
			CommandType: command.CmdSpawnProcess,
			Source:      command.SourceInternal,
			Success:     true,
			Duration:    10 * time.Millisecond,
			Timestamp:   time.Now(),
		})
	}

	require.Len(t, m.commandPane.entries, maxCommandLogEntries)

	// Add one more via event (should trigger FIFO eviction)
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: processor.CommandLogEvent{
			CommandID:   "new-entry-500",
			CommandType: command.CmdAssignTask,
			Source:      command.SourceUser,
			Success:     true,
			Duration:    20 * time.Millisecond,
			Timestamp:   time.Now(),
		},
	}

	m, _ = m.handleV2Event(event)

	// Verify FIFO eviction occurred
	require.Len(t, m.commandPane.entries, maxCommandLogEntries,
		"should still have exactly max entries")

	// First entry should now be "pre-fill-1" (pre-fill-0 was evicted)
	require.Equal(t, "pre-fill-1", m.commandPane.entries[0].CommandID,
		"oldest entry should be evicted")

	// Last entry should be the new one
	require.Equal(t, "new-entry-500", m.commandPane.entries[maxCommandLogEntries-1].CommandID,
		"new entry should be last")
}

// TestHandleV2Event_CommandLogEvent_HasNewContent verifies hasNewContent behavior.
func TestHandleV2Event_CommandLogEvent_HasNewContent(t *testing.T) {
	t.Run("hidden pane does not set hasNewContent", func(t *testing.T) {
		m := New(Config{})
		m = m.SetSize(120, 40)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		v2Broker := pubsub.NewBroker[any]()
		m.v2Listener = pubsub.NewContinuousListener(ctx, v2Broker)

		// Pane hidden by default
		require.False(t, m.showCommandPane)

		// Add event
		event := pubsub.Event[any]{
			Type: pubsub.UpdatedEvent,
			Payload: processor.CommandLogEvent{
				CommandID:   "test",
				CommandType: command.CmdSpawnProcess,
				Success:     true,
				Timestamp:   time.Now(),
			},
		}
		m, _ = m.handleV2Event(event)

		// hasNewContent should NOT be set when hidden
		require.False(t, m.commandPane.hasNewContent,
			"hasNewContent should not be set when pane is hidden")
	})

	t.Run("visible pane at bottom does not set hasNewContent", func(t *testing.T) {
		m := New(Config{})
		m = m.SetSize(120, 40)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		v2Broker := pubsub.NewBroker[any]()
		m.v2Listener = pubsub.NewContinuousListener(ctx, v2Broker)

		// Show the pane
		m.showCommandPane = true

		// Viewport is at bottom by default (no content yet)
		event := pubsub.Event[any]{
			Type: pubsub.UpdatedEvent,
			Payload: processor.CommandLogEvent{
				CommandID:   "test",
				CommandType: command.CmdSpawnProcess,
				Success:     true,
				Timestamp:   time.Now(),
			},
		}
		m, _ = m.handleV2Event(event)

		// hasNewContent should NOT be set when at bottom
		require.False(t, m.commandPane.hasNewContent,
			"hasNewContent should not be set when at bottom")
	})
}

// ============================================================================
// Trace ID Tests
// ============================================================================

// TestHandleV2Event_CommandLogEvent_TraceID verifies trace ID is extracted and stored.
func TestHandleV2Event_CommandLogEvent_TraceID(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Create a v2 listener
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	v2Broker := pubsub.NewBroker[any]()
	m.v2Listener = pubsub.NewContinuousListener(ctx, v2Broker)

	// Verify activeTraceID is initially empty
	require.Empty(t, m.activeTraceID, "activeTraceID should be empty initially")

	// Create a CommandLogEvent with a trace ID
	traceID := "abc123def456789012345678901234ff"
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: processor.CommandLogEvent{
			CommandID:   "test-cmd-123",
			CommandType: command.CmdSpawnProcess,
			Source:      command.SourceMCPTool,
			Success:     true,
			Error:       nil,
			Duration:    100 * time.Millisecond,
			Timestamp:   time.Now(),
			TraceID:     traceID,
		},
	}

	// Handle the event
	m, _ = m.handleV2Event(event)

	// Verify entry contains trace ID
	require.Len(t, m.commandPane.entries, 1, "should have one entry")
	entry := m.commandPane.entries[0]
	require.Equal(t, traceID, entry.TraceID, "entry should contain trace ID")

	// Verify activeTraceID was updated
	require.Equal(t, traceID, m.activeTraceID, "activeTraceID should be updated from event")
}

// TestHandleV2Event_CommandLogEvent_EmptyTraceID verifies empty trace ID doesn't overwrite existing.
func TestHandleV2Event_CommandLogEvent_EmptyTraceID(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Create a v2 listener
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	v2Broker := pubsub.NewBroker[any]()
	m.v2Listener = pubsub.NewContinuousListener(ctx, v2Broker)

	// Set an initial activeTraceID
	m.activeTraceID = "existing-trace-id"

	// Create a CommandLogEvent with empty trace ID
	event := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: processor.CommandLogEvent{
			CommandID:   "test-cmd-123",
			CommandType: command.CmdSpawnProcess,
			Success:     true,
			Timestamp:   time.Now(),
			TraceID:     "", // Empty
		},
	}

	// Handle the event
	m, _ = m.handleV2Event(event)

	// Verify activeTraceID was NOT overwritten
	require.Equal(t, "existing-trace-id", m.activeTraceID,
		"activeTraceID should not be overwritten by empty trace ID")
}

// TestHandleV2Event_CommandLogEvent_TraceIDUpdates verifies trace ID updates with new events.
func TestHandleV2Event_CommandLogEvent_TraceIDUpdates(t *testing.T) {
	m := New(Config{})
	m = m.SetSize(120, 40)

	// Create a v2 listener
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	v2Broker := pubsub.NewBroker[any]()
	m.v2Listener = pubsub.NewContinuousListener(ctx, v2Broker)

	// First event with trace ID
	event1 := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: processor.CommandLogEvent{
			CommandID:   "cmd-1",
			CommandType: command.CmdSpawnProcess,
			Success:     true,
			Timestamp:   time.Now(),
			TraceID:     "trace-id-one",
		},
	}
	m, _ = m.handleV2Event(event1)
	require.Equal(t, "trace-id-one", m.activeTraceID)

	// Second event with different trace ID should update
	event2 := pubsub.Event[any]{
		Type: pubsub.UpdatedEvent,
		Payload: processor.CommandLogEvent{
			CommandID:   "cmd-2",
			CommandType: command.CmdAssignTask,
			Success:     true,
			Timestamp:   time.Now(),
			TraceID:     "trace-id-two",
		},
	}
	m, _ = m.handleV2Event(event2)
	require.Equal(t, "trace-id-two", m.activeTraceID, "activeTraceID should be updated to new trace ID")

	// Verify both entries have their respective trace IDs
	require.Len(t, m.commandPane.entries, 2)
	require.Equal(t, "trace-id-one", m.commandPane.entries[0].TraceID)
	require.Equal(t, "trace-id-two", m.commandPane.entries[1].TraceID)
}
