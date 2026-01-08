// Package tracing provides distributed tracing infrastructure for the orchestration system.
// It integrates with OpenTelemetry to provide span creation, context propagation,
// and trace export capabilities.
package tracing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

// contextKey is a private type for context keys to avoid collisions.
type contextKey string

// traceIDKey is the context key for storing trace IDs.
const traceIDKey contextKey = "trace_id"

// TraceIDFromContext extracts the trace ID from the context.
// Returns an empty string if no trace ID is present.
func TraceIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v := ctx.Value(traceIDKey); v != nil {
		if traceID, ok := v.(string); ok {
			return traceID
		}
	}
	return ""
}

// ContextWithTraceID returns a new context with the trace ID set.
// If traceID is empty, the original context is returned unchanged.
func ContextWithTraceID(ctx context.Context, traceID string) context.Context {
	if traceID == "" {
		return ctx
	}
	return context.WithValue(ctx, traceIDKey, traceID)
}

// GenerateTraceID creates a new random 32-character hex trace ID.
// This follows the W3C Trace Context format for trace-id (16 bytes = 32 hex chars).
func GenerateTraceID() string {
	bytes := make([]byte, 16)
	// crypto/rand.Read never returns an error on supported platforms
	_, _ = rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// GenerateSpanID creates a new random 16-character hex span ID.
// This follows the W3C Trace Context format for span-id (8 bytes = 16 hex chars).
func GenerateSpanID() string {
	bytes := make([]byte, 8)
	// crypto/rand.Read never returns an error on supported platforms
	_, _ = rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
