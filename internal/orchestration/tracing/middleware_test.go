package tracing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/processor"
)

// ===========================================================================
// Test Helpers
// ===========================================================================

// testCommand is a simple command for testing.
type testCommand struct {
	*command.BaseCommand
	value int
}

func newTestCommand(value int) *testCommand {
	base := command.NewBaseCommand("test_command", command.SourceInternal)
	return &testCommand{
		BaseCommand: &base,
		value:       value,
	}
}

func (c *testCommand) Validate() error {
	return nil
}

// successHandler returns a successful result.
func successHandler() processor.CommandHandler {
	return processor.HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
		return &command.CommandResult{Success: true, Data: "ok"}, nil
	})
}

// errorHandler returns an error.
func errorHandler(errMsg string) processor.CommandHandler {
	return processor.HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
		return nil, errors.New(errMsg)
	})
}

// failureResultHandler returns a failure result (not an error).
func failureResultHandler(errMsg string) processor.CommandHandler {
	return processor.HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
		return &command.CommandResult{
			Success: false,
			Error:   errors.New(errMsg),
		}, nil
	})
}

// failureResultHandlerNoError returns a failure result without an error.
func failureResultHandlerNoError() processor.CommandHandler {
	return processor.HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
		return &command.CommandResult{
			Success: false,
			Error:   nil,
		}, nil
	})
}

// followUpHandler returns a result with follow-up commands.
func followUpHandler(followUps ...command.Command) processor.CommandHandler {
	return processor.HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
		return &command.CommandResult{
			Success:  true,
			FollowUp: followUps,
		}, nil
	})
}

// setupTestTracer creates a test tracer with an in-memory exporter.
func setupTestTracer(t *testing.T) (trace.Tracer, *tracetest.InMemoryExporter) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
	tracer := provider.Tracer("test-tracer")
	return tracer, exporter
}

// getSpanByName finds a span by name from the exporter.
func getSpanByName(exporter *tracetest.InMemoryExporter, name string) (tracetest.SpanStub, bool) {
	for _, span := range exporter.GetSpans() {
		if span.Name == name {
			return span, true
		}
	}
	return tracetest.SpanStub{}, false
}

// getAttributeValue extracts an attribute value from a span.
func getAttributeValue(span tracetest.SpanStub, key string) (attribute.Value, bool) {
	for _, attr := range span.Attributes {
		if string(attr.Key) == key {
			return attr.Value, true
		}
	}
	return attribute.Value{}, false
}

// ===========================================================================
// TracingMiddleware Tests
// ===========================================================================

func TestNewTracingMiddleware_NilTracer_ReturnsPassThrough(t *testing.T) {
	middleware := NewTracingMiddleware(TracingMiddlewareConfig{
		Tracer: nil,
	})

	handler := successHandler()
	wrapped := middleware(handler)

	cmd := newTestCommand(42)
	result, err := wrapped.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, "ok", result.Data)
}

func TestTracingMiddleware_CreatesSpanWithCorrectName(t *testing.T) {
	tracer, exporter := setupTestTracer(t)
	middleware := NewTracingMiddleware(TracingMiddlewareConfig{
		Tracer: tracer,
	})

	handler := successHandler()
	wrapped := middleware(handler)

	cmd := newTestCommand(42)
	result, err := wrapped.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify span was created with correct name
	span, found := getSpanByName(exporter, "command.process.test_command")
	require.True(t, found, "Expected span with name 'command.process.test_command'")
	assert.Equal(t, "command.process.test_command", span.Name)
}

func TestTracingMiddleware_SetsCommandAttributes(t *testing.T) {
	tracer, exporter := setupTestTracer(t)
	middleware := NewTracingMiddleware(TracingMiddlewareConfig{
		Tracer: tracer,
	})

	handler := successHandler()
	wrapped := middleware(handler)

	cmd := newTestCommand(42)
	result, err := wrapped.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify span attributes
	span, found := getSpanByName(exporter, "command.process.test_command")
	require.True(t, found)

	// Check command.id
	cmdID, found := getAttributeValue(span, AttrCommandID)
	require.True(t, found, "Expected command.id attribute")
	assert.Equal(t, cmd.ID(), cmdID.AsString())

	// Check command.type
	cmdType, found := getAttributeValue(span, AttrCommandType)
	require.True(t, found, "Expected command.type attribute")
	assert.Equal(t, "test_command", cmdType.AsString())

	// Check command.priority
	cmdPriority, found := getAttributeValue(span, AttrCommandPriority)
	require.True(t, found, "Expected command.priority attribute")
	assert.Equal(t, int64(0), cmdPriority.AsInt64())

	// Check command.source
	cmdSource, found := getAttributeValue(span, AttrCommandSource)
	require.True(t, found, "Expected command.source attribute")
	assert.Equal(t, string(command.SourceInternal), cmdSource.AsString())
}

func TestTracingMiddleware_RecordsErrors(t *testing.T) {
	tracer, exporter := setupTestTracer(t)
	middleware := NewTracingMiddleware(TracingMiddlewareConfig{
		Tracer: tracer,
	})

	handler := errorHandler("something went wrong")
	wrapped := middleware(handler)

	cmd := newTestCommand(1)
	_, err := wrapped.Handle(context.Background(), cmd)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "something went wrong")

	// Verify span recorded error
	span, found := getSpanByName(exporter, "command.process.test_command")
	require.True(t, found)

	// Check span status
	assert.Equal(t, codes.Error, span.Status.Code)
	assert.Contains(t, span.Status.Description, "something went wrong")

	// Check that error was recorded as event
	assert.NotEmpty(t, span.Events, "Expected error event to be recorded")
	foundExceptionEvent := false
	for _, event := range span.Events {
		if event.Name == "exception" {
			foundExceptionEvent = true
			break
		}
	}
	assert.True(t, foundExceptionEvent, "Expected 'exception' event to be recorded")
}

func TestTracingMiddleware_RecordsFailureResult(t *testing.T) {
	tracer, exporter := setupTestTracer(t)
	middleware := NewTracingMiddleware(TracingMiddlewareConfig{
		Tracer: tracer,
	})

	handler := failureResultHandler("result error")
	wrapped := middleware(handler)

	cmd := newTestCommand(1)
	result, err := wrapped.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.False(t, result.Success)

	// Verify span recorded failure
	span, found := getSpanByName(exporter, "command.process.test_command")
	require.True(t, found)

	assert.Equal(t, codes.Error, span.Status.Code)
	assert.Contains(t, span.Status.Description, "result error")
}

func TestTracingMiddleware_RecordsFailureResultWithoutError(t *testing.T) {
	tracer, exporter := setupTestTracer(t)
	middleware := NewTracingMiddleware(TracingMiddlewareConfig{
		Tracer: tracer,
	})

	handler := failureResultHandlerNoError()
	wrapped := middleware(handler)

	cmd := newTestCommand(1)
	result, err := wrapped.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.False(t, result.Success)

	// Verify span recorded failure
	span, found := getSpanByName(exporter, "command.process.test_command")
	require.True(t, found)

	assert.Equal(t, codes.Error, span.Status.Code)
	assert.Equal(t, "command failed without error details", span.Status.Description)
}

func TestTracingMiddleware_SetsOkStatusOnSuccess(t *testing.T) {
	tracer, exporter := setupTestTracer(t)
	middleware := NewTracingMiddleware(TracingMiddlewareConfig{
		Tracer: tracer,
	})

	handler := successHandler()
	wrapped := middleware(handler)

	cmd := newTestCommand(1)
	result, err := wrapped.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	span, found := getSpanByName(exporter, "command.process.test_command")
	require.True(t, found)

	assert.Equal(t, codes.Ok, span.Status.Code)
}

func TestTracingMiddleware_PropagatesSpanContextToFollowUpCommands(t *testing.T) {
	tracer, exporter := setupTestTracer(t)
	middleware := NewTracingMiddleware(TracingMiddlewareConfig{
		Tracer: tracer,
	})

	// Create follow-up commands
	followUp1 := newTestCommand(100)
	followUp2 := newTestCommand(200)

	handler := followUpHandler(followUp1, followUp2)
	wrapped := middleware(handler)

	cmd := newTestCommand(1)

	result, err := wrapped.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Len(t, result.FollowUp, 2)

	// Verify span context was propagated to follow-up commands
	assert.True(t, followUp1.SpanContext().IsValid(), "Follow-up 1 should have valid span context")
	assert.True(t, followUp2.SpanContext().IsValid(), "Follow-up 2 should have valid span context")

	// Verify follow-up events were added to span
	span, found := getSpanByName(exporter, "command.process.test_command")
	require.True(t, found)

	// Verify trace IDs match (derived from span context)
	parentTraceID := span.SpanContext.TraceID().String()
	assert.NotEmpty(t, parentTraceID)
	assert.Equal(t, parentTraceID, followUp1.TraceID())
	assert.Equal(t, parentTraceID, followUp2.TraceID())

	// Reset found for the event check below
	span, found = getSpanByName(exporter, "command.process.test_command")
	require.True(t, found)

	followUpEvents := 0
	for _, event := range span.Events {
		if event.Name == EventFollowUpCreated {
			followUpEvents++
		}
	}
	assert.Equal(t, 2, followUpEvents, "Expected 2 follow-up events")
}

func TestTracingMiddleware_CreatesSpanWithTraceID(t *testing.T) {
	tracer, exporter := setupTestTracer(t)
	middleware := NewTracingMiddleware(TracingMiddlewareConfig{
		Tracer: tracer,
	})

	handler := successHandler()
	wrapped := middleware(handler)

	cmd := newTestCommand(1)

	result, err := wrapped.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Span should be created with valid trace ID
	span, found := getSpanByName(exporter, "command.process.test_command")
	require.True(t, found)
	assert.True(t, span.SpanContext.TraceID().IsValid(), "Span should have valid trace ID")
}

func TestTracingMiddleware_TraceIDDerivedFromSpan(t *testing.T) {
	tracer, exporter := setupTestTracer(t)
	middleware := NewTracingMiddleware(TracingMiddlewareConfig{
		Tracer: tracer,
	})

	handler := successHandler()
	wrapped := middleware(handler)

	cmd := newTestCommand(1)
	// No trace ID on command initially
	assert.Empty(t, cmd.TraceID())

	result, err := wrapped.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Span should be created
	span, found := getSpanByName(exporter, "command.process.test_command")
	require.True(t, found)

	// Trace ID is now derived from the span (32 hex chars)
	assert.True(t, span.SpanContext.TraceID().IsValid())
}

func TestTracingMiddleware_IntegrationWithFollowUps(t *testing.T) {
	tracer, exporter := setupTestTracer(t)
	middleware := NewTracingMiddleware(TracingMiddlewareConfig{
		Tracer: tracer,
	})

	// Create follow-up commands
	followUp1 := newTestCommand(100)
	followUp2 := newTestCommand(200)

	handler := followUpHandler(followUp1, followUp2)
	wrapped := middleware(handler)

	cmd := newTestCommand(1)
	result, err := wrapped.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Follow-ups should have span context propagated
	assert.True(t, followUp1.SpanContext().IsValid(), "Follow-up 1 should have valid span context")
	assert.True(t, followUp2.SpanContext().IsValid(), "Follow-up 2 should have valid span context")

	// All should share the same trace ID (derived from span context)
	assert.NotEmpty(t, followUp1.TraceID())
	assert.Equal(t, followUp1.TraceID(), followUp2.TraceID())

	// Verify span was created
	_, found := getSpanByName(exporter, "command.process.test_command")
	require.True(t, found)
}

// ===========================================================================
// Helper Function Tests
// ===========================================================================

// commandWithoutSpanContext is a command that doesn't implement SpanContext().
type commandWithoutSpanContext struct {
	id        string
	createdAt time.Time
}

func (c *commandWithoutSpanContext) ID() string                { return c.id }
func (c *commandWithoutSpanContext) Type() command.CommandType { return "no_span_context_command" }
func (c *commandWithoutSpanContext) Validate() error           { return nil }
func (c *commandWithoutSpanContext) Priority() int             { return 0 }
func (c *commandWithoutSpanContext) CreatedAt() time.Time      { return c.createdAt }

func TestRestoreSpanContext_WithValidSpanContext(t *testing.T) {
	cmd := newTestCommand(1)

	// Create a valid span context
	traceID, _ := trace.TraceIDFromHex("0102030405060708090a0b0c0d0e0f10")
	spanID, _ := trace.SpanIDFromHex("0102030405060708")
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	cmd.SetSpanContext(sc)

	ctx := context.Background()
	newCtx := restoreSpanContext(ctx, cmd)

	// The context should now have the span context
	restoredSC := trace.SpanContextFromContext(newCtx)
	assert.True(t, restoredSC.IsValid())
	assert.Equal(t, traceID, restoredSC.TraceID())
}

func TestRestoreSpanContext_WithoutSpanContext(t *testing.T) {
	cmd := &commandWithoutSpanContext{id: "test-id", createdAt: time.Now()}

	ctx := context.Background()
	newCtx := restoreSpanContext(ctx, cmd)

	// Context should be unchanged
	assert.Equal(t, ctx, newCtx)
}

func TestRestoreSpanContext_WithInvalidSpanContext(t *testing.T) {
	cmd := newTestCommand(1)
	// Default span context is invalid (zero value)

	ctx := context.Background()
	newCtx := restoreSpanContext(ctx, cmd)

	// Context should be unchanged since span context is invalid
	restoredSC := trace.SpanContextFromContext(newCtx)
	assert.False(t, restoredSC.IsValid())
}
