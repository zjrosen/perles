package tracing

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	require.False(t, cfg.Enabled, "tracing should be disabled by default")
	require.Equal(t, "file", cfg.Exporter, "default exporter should be file")
	require.Equal(t, "", cfg.FilePath, "file path should be empty by default")
	require.Equal(t, "localhost:4317", cfg.OTLPEndpoint, "default OTLP endpoint")
	require.Equal(t, 1.0, cfg.SampleRate, "default sample rate should be 1.0")
	require.Equal(t, "perles-orchestrator", cfg.ServiceName, "default service name")
}

func TestNewProvider_Disabled(t *testing.T) {
	cfg := Config{
		Enabled: false,
	}

	provider, err := NewProvider(cfg)
	require.NoError(t, err, "should not error when disabled")
	require.NotNil(t, provider, "should return provider even when disabled")
	require.False(t, provider.Enabled(), "provider should report as disabled")

	// Tracer should be no-op but not nil
	tracer := provider.Tracer()
	require.NotNil(t, tracer, "should return a tracer")

	// Creating spans should not panic
	ctx, span := tracer.Start(context.Background(), "test-span")
	require.NotNil(t, ctx)
	require.NotNil(t, span)
	span.End()

	// Shutdown should work
	err = provider.Shutdown(context.Background())
	require.NoError(t, err)
}

func TestNewProvider_Enabled_WithFileExporter(t *testing.T) {
	tmpDir := t.TempDir()
	tracePath := filepath.Join(tmpDir, "traces.jsonl")

	cfg := Config{
		Enabled:     true,
		Exporter:    "file",
		FilePath:    tracePath,
		SampleRate:  1.0,
		ServiceName: "test-service",
	}

	provider, err := NewProvider(cfg)
	require.NoError(t, err, "should create provider with file exporter")
	require.NotNil(t, provider)
	require.True(t, provider.Enabled(), "provider should report as enabled")

	// Create a span to verify tracer works
	tracer := provider.Tracer()
	require.NotNil(t, tracer)

	ctx, span := tracer.Start(context.Background(), "test-span")
	require.NotNil(t, ctx)
	require.NotNil(t, span)

	// Verify span context is valid
	sc := span.SpanContext()
	require.True(t, sc.IsValid(), "span context should be valid")
	require.True(t, sc.TraceID().IsValid(), "trace ID should be valid")
	require.True(t, sc.SpanID().IsValid(), "span ID should be valid")

	span.End()

	// Shutdown to flush spans
	err = provider.Shutdown(context.Background())
	require.NoError(t, err)

	// Verify trace file was created
	_, err = os.Stat(tracePath)
	require.NoError(t, err, "trace file should exist")
}

func TestNewProvider_Enabled_WithStdoutExporter(t *testing.T) {
	cfg := Config{
		Enabled:     true,
		Exporter:    "stdout",
		SampleRate:  1.0,
		ServiceName: "test-service",
	}

	provider, err := NewProvider(cfg)
	require.NoError(t, err, "should create provider with stdout exporter")
	require.NotNil(t, provider)
	require.True(t, provider.Enabled())

	// Create a span
	tracer := provider.Tracer()
	_, span := tracer.Start(context.Background(), "test-span")
	span.End()

	// Shutdown
	err = provider.Shutdown(context.Background())
	require.NoError(t, err)
}

func TestNewProvider_Enabled_WithNoExporter(t *testing.T) {
	cfg := Config{
		Enabled:     true,
		Exporter:    "none",
		SampleRate:  1.0,
		ServiceName: "test-service",
	}

	provider, err := NewProvider(cfg)
	require.NoError(t, err, "should create provider with no exporter")
	require.NotNil(t, provider)
	require.True(t, provider.Enabled())

	// Create a span - should work for internal correlation
	tracer := provider.Tracer()
	_, span := tracer.Start(context.Background(), "test-span")
	span.End()

	err = provider.Shutdown(context.Background())
	require.NoError(t, err)
}

func TestNewProvider_FileExporter_MissingPath(t *testing.T) {
	cfg := Config{
		Enabled:  true,
		Exporter: "file",
		FilePath: "", // Missing path
	}

	provider, err := NewProvider(cfg)
	require.Error(t, err, "should error when file path is missing")
	require.Nil(t, provider)
	require.Contains(t, err.Error(), "file_path required")
}

func TestNewProvider_UnsupportedExporter(t *testing.T) {
	cfg := Config{
		Enabled:  true,
		Exporter: "invalid-exporter",
	}

	provider, err := NewProvider(cfg)
	require.Error(t, err, "should error for unsupported exporter")
	require.Nil(t, provider)
	require.Contains(t, err.Error(), "unsupported exporter")
}

func TestNewProvider_DefaultSampleRate(t *testing.T) {
	tmpDir := t.TempDir()
	tracePath := filepath.Join(tmpDir, "traces.jsonl")

	cfg := Config{
		Enabled:    true,
		Exporter:   "file",
		FilePath:   tracePath,
		SampleRate: 0, // Invalid, should default to 1.0
	}

	provider, err := NewProvider(cfg)
	require.NoError(t, err, "should handle zero sample rate")
	require.NotNil(t, provider)

	err = provider.Shutdown(context.Background())
	require.NoError(t, err)
}

func TestNewProvider_DefaultServiceName(t *testing.T) {
	tmpDir := t.TempDir()
	tracePath := filepath.Join(tmpDir, "traces.jsonl")

	cfg := Config{
		Enabled:     true,
		Exporter:    "file",
		FilePath:    tracePath,
		ServiceName: "", // Should use default
	}

	provider, err := NewProvider(cfg)
	require.NoError(t, err, "should handle empty service name")
	require.NotNil(t, provider)

	err = provider.Shutdown(context.Background())
	require.NoError(t, err)
}

func TestProvider_TracerReturnsConsistentInstance(t *testing.T) {
	cfg := Config{
		Enabled: false,
	}

	provider, err := NewProvider(cfg)
	require.NoError(t, err)

	tracer1 := provider.Tracer()
	tracer2 := provider.Tracer()

	// Should return the same instance
	require.Equal(t, tracer1, tracer2, "Tracer() should return consistent instance")
}

func TestProvider_TracerCreatesValidSpans(t *testing.T) {
	tmpDir := t.TempDir()
	tracePath := filepath.Join(tmpDir, "traces.jsonl")

	cfg := Config{
		Enabled:  true,
		Exporter: "file",
		FilePath: tracePath,
	}

	provider, err := NewProvider(cfg)
	require.NoError(t, err)
	defer provider.Shutdown(context.Background())

	tracer := provider.Tracer()

	// Create parent span
	ctx, parentSpan := tracer.Start(context.Background(), "parent-span")
	require.True(t, parentSpan.SpanContext().IsValid())

	// Create child span - should inherit trace ID
	_, childSpan := tracer.Start(ctx, "child-span")
	require.True(t, childSpan.SpanContext().IsValid())
	require.Equal(t,
		parentSpan.SpanContext().TraceID(),
		childSpan.SpanContext().TraceID(),
		"child span should have same trace ID as parent")

	childSpan.End()
	parentSpan.End()
}

func TestProvider_SpanAttributes(t *testing.T) {
	tmpDir := t.TempDir()
	tracePath := filepath.Join(tmpDir, "traces.jsonl")

	cfg := Config{
		Enabled:  true,
		Exporter: "file",
		FilePath: tracePath,
	}

	provider, err := NewProvider(cfg)
	require.NoError(t, err)
	defer provider.Shutdown(context.Background())

	tracer := provider.Tracer()

	// Create span with attributes
	_, span := tracer.Start(context.Background(), "test-span",
		trace.WithSpanKind(trace.SpanKindInternal),
	)

	// Add attributes using the span attribute methods
	span.SetAttributes() // Can be called without error
	span.End()
}
