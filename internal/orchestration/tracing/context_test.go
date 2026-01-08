package tracing

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTraceIDFromContext_EmptyContext(t *testing.T) {
	ctx := context.Background()
	traceID := TraceIDFromContext(ctx)
	require.Equal(t, "", traceID, "should return empty string for context without trace ID")
}

func TestTraceIDFromContext_NilContext(t *testing.T) {
	//nolint:staticcheck // testing nil context handling
	traceID := TraceIDFromContext(nil)
	require.Equal(t, "", traceID, "should return empty string for nil context")
}

func TestContextWithTraceID_Roundtrip(t *testing.T) {
	ctx := context.Background()
	expectedTraceID := "abc123def456789012345678901234ff"

	ctx = ContextWithTraceID(ctx, expectedTraceID)
	actualTraceID := TraceIDFromContext(ctx)

	require.Equal(t, expectedTraceID, actualTraceID, "trace ID should roundtrip correctly")
}

func TestContextWithTraceID_EmptyTraceID(t *testing.T) {
	ctx := context.Background()

	// Set a trace ID first
	ctx = ContextWithTraceID(ctx, "original-trace-id")

	// Try to set empty trace ID - should return original context
	ctx2 := ContextWithTraceID(ctx, "")
	require.Equal(t, "original-trace-id", TraceIDFromContext(ctx2),
		"empty trace ID should not overwrite existing value")
}

func TestContextWithTraceID_Overwrite(t *testing.T) {
	ctx := context.Background()

	ctx = ContextWithTraceID(ctx, "first-trace-id")
	ctx = ContextWithTraceID(ctx, "second-trace-id")

	require.Equal(t, "second-trace-id", TraceIDFromContext(ctx),
		"should be able to overwrite trace ID")
}

func TestGenerateTraceID_ValidFormat(t *testing.T) {
	traceID := GenerateTraceID()

	// Should be 32 characters (16 bytes hex encoded)
	require.Len(t, traceID, 32, "trace ID should be 32 characters")

	// Should be valid hex
	_, err := hex.DecodeString(traceID)
	require.NoError(t, err, "trace ID should be valid hex")
}

func TestGenerateTraceID_Unique(t *testing.T) {
	// Generate multiple trace IDs and verify they're unique
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		traceID := GenerateTraceID()
		require.False(t, seen[traceID], "trace IDs should be unique")
		seen[traceID] = true
	}
}

func TestGenerateSpanID_ValidFormat(t *testing.T) {
	spanID := GenerateSpanID()

	// Should be 16 characters (8 bytes hex encoded)
	require.Len(t, spanID, 16, "span ID should be 16 characters")

	// Should be valid hex
	_, err := hex.DecodeString(spanID)
	require.NoError(t, err, "span ID should be valid hex")
}

func TestGenerateSpanID_Unique(t *testing.T) {
	// Generate multiple span IDs and verify they're unique
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		spanID := GenerateSpanID()
		require.False(t, seen[spanID], "span IDs should be unique")
		seen[spanID] = true
	}
}
