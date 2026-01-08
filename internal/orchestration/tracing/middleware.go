// Package tracing provides distributed tracing infrastructure for the orchestration system.
package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/processor"
)

// TracingMiddlewareConfig configures the tracing middleware.
type TracingMiddlewareConfig struct {
	// Tracer is the OpenTelemetry tracer for creating spans.
	// If nil, the middleware returns a pass-through (no-op).
	Tracer trace.Tracer
}

// NewTracingMiddleware creates middleware that creates spans for command processing.
// It extracts or creates trace context, creates spans with command attributes,
// records errors, and propagates TraceID to follow-up commands.
//
// If Tracer is nil, the middleware returns a pass-through function that
// simply calls the next handler without any tracing overhead.
func NewTracingMiddleware(cfg TracingMiddlewareConfig) processor.Middleware {
	if cfg.Tracer == nil {
		// Return pass-through if tracing disabled
		return func(next processor.CommandHandler) processor.CommandHandler {
			return next
		}
	}

	return func(next processor.CommandHandler) processor.CommandHandler {
		return processor.HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
			// Restore span context from command if available (for follow-up commands)
			ctx = restoreSpanContext(ctx, cmd)

			// Create span for command processing
			spanName := fmt.Sprintf("%s%s", SpanPrefixCommand, cmd.Type())
			ctx, span := cfg.Tracer.Start(ctx, spanName,
				trace.WithSpanKind(trace.SpanKindInternal),
			)
			defer span.End()

			// Get trace ID from the span (unified source of truth)
			traceID := span.SpanContext().TraceID().String()

			// Set command attributes
			span.SetAttributes(
				attribute.String(AttrCommandID, cmd.ID()),
				attribute.String(AttrCommandType, string(cmd.Type())),
				attribute.Int(AttrCommandPriority, cmd.Priority()),
			)

			// Add source if available
			if hasSource, ok := cmd.(interface{ Source() command.CommandSource }); ok {
				span.SetAttributes(attribute.String(AttrCommandSource, string(hasSource.Source())))
			}

			// Execute handler
			result, err := next.Handle(ctx, cmd)

			// Record outcome
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			} else if result != nil && !result.Success {
				if result.Error != nil {
					span.RecordError(result.Error)
					span.SetStatus(codes.Error, result.Error.Error())
				} else {
					span.SetStatus(codes.Error, "command failed without error details")
				}
			} else {
				span.SetStatus(codes.Ok, "")
			}

			// Record follow-up commands as events and propagate trace context
			if result != nil && len(result.FollowUp) > 0 {
				// Get current span context to propagate to follow-ups
				currentSpanContext := span.SpanContext()

				for _, followUp := range result.FollowUp {
					span.AddEvent(EventFollowUpCreated,
						trace.WithAttributes(
							attribute.String(AttrCommandType, string(followUp.Type())),
							attribute.String(AttrCommandID, followUp.ID()),
						),
					)
					// Propagate trace ID to follow-up command
					if setter, ok := followUp.(interface{ SetTraceID(string) }); ok {
						setter.SetTraceID(traceID)
					}
					// Propagate span context to follow-up command for parent-child linking
					if setter, ok := followUp.(interface{ SetSpanContext(trace.SpanContext) }); ok {
						setter.SetSpanContext(currentSpanContext)
					}
				}
			}

			return result, err
		})
	}
}

// restoreSpanContext restores the OpenTelemetry span context from a command.
// If the command carries a valid span context (from a parent command), it creates
// a new context with that span context so that new spans become children.
func restoreSpanContext(ctx context.Context, cmd command.Command) context.Context {
	if hasSpanContext, ok := cmd.(interface{ SpanContext() trace.SpanContext }); ok {
		sc := hasSpanContext.SpanContext()
		if sc.IsValid() {
			// Create a remote span context to use as parent
			return trace.ContextWithRemoteSpanContext(ctx, sc)
		}
	}
	return ctx
}
