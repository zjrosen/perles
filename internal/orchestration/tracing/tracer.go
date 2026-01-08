package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// Config configures the tracing subsystem.
type Config struct {
	// Enabled controls whether tracing is active.
	// When false, a no-op tracer is returned.
	Enabled bool `yaml:"enabled"`

	// Exporter selects the export backend.
	// Options: "none", "file", "stdout", "otlp"
	// Default: "file"
	Exporter string `yaml:"exporter"`

	// FilePath is the output file for "file" exporter.
	// Default: derived from config directory (e.g., ~/.config/perles/traces/traces.jsonl)
	FilePath string `yaml:"file_path"`

	// OTLPEndpoint is the OTLP collector endpoint for "otlp" exporter.
	// Default: "localhost:4317"
	OTLPEndpoint string `yaml:"otlp_endpoint"`

	// SampleRate controls the fraction of traces to sample.
	// 1.0 = all traces, 0.1 = 10% of traces
	// Default: 1.0 (sample all in development)
	SampleRate float64 `yaml:"sample_rate"`

	// ServiceName identifies this service in traces.
	// Default: "perles-orchestrator"
	ServiceName string `yaml:"service_name"`
}

// DefaultConfig returns sensible defaults for development.
func DefaultConfig() Config {
	return Config{
		Enabled:      false,
		Exporter:     "file",
		FilePath:     "",
		OTLPEndpoint: "localhost:4317",
		SampleRate:   1.0,
		ServiceName:  "perles-orchestrator",
	}
}

// Provider manages the OpenTelemetry tracer provider.
// It wraps the underlying TracerProvider and provides convenient methods
// for getting tracers and shutting down cleanly.
type Provider struct {
	provider *sdktrace.TracerProvider
	tracer   trace.Tracer
	enabled  bool
}

// NewProvider creates and configures the trace provider.
// If tracing is disabled in the config, a no-op provider is returned
// that has zero overhead.
func NewProvider(cfg Config) (*Provider, error) {
	if !cfg.Enabled {
		// Return no-op provider for zero overhead when disabled
		noopProvider := noop.NewTracerProvider()
		return &Provider{
			provider: nil,
			tracer:   noopProvider.Tracer("noop"),
			enabled:  false,
		}, nil
	}

	// Create exporter based on config
	var exporter sdktrace.SpanExporter
	var err error

	switch cfg.Exporter {
	case "file":
		if cfg.FilePath == "" {
			return nil, fmt.Errorf("file_path required for file exporter")
		}
		exporter, err = NewFileExporter(cfg.FilePath)
		if err != nil {
			return nil, fmt.Errorf("create file exporter: %w", err)
		}
	case "stdout":
		exporter, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return nil, fmt.Errorf("create stdout exporter: %w", err)
		}
	case "otlp":
		endpoint := cfg.OTLPEndpoint
		if endpoint == "" {
			endpoint = "localhost:4317"
		}
		exporter, err = otlptracegrpc.New(
			context.Background(),
			otlptracegrpc.WithEndpoint(endpoint),
			otlptracegrpc.WithInsecure(),
		)
		if err != nil {
			return nil, fmt.Errorf("create otlp exporter: %w", err)
		}
	case "none", "":
		// No exporter, but tracing enabled for internal correlation
		exporter = nil
	default:
		return nil, fmt.Errorf("unsupported exporter type: %s", cfg.Exporter)
	}

	// Ensure service name has a default
	serviceName := cfg.ServiceName
	if serviceName == "" {
		serviceName = "perles-orchestrator"
	}

	// Create resource with service info
	// We use resource.NewSchemaless to avoid schema version conflicts with resource.Default()
	res := resource.NewSchemaless(
		attribute.String("service.name", serviceName),
	)

	// Create sampler - use parent-based sampling with ratio
	sampleRate := cfg.SampleRate
	if sampleRate <= 0 {
		sampleRate = 1.0
	}
	sampler := sdktrace.ParentBased(
		sdktrace.TraceIDRatioBased(sampleRate),
	)

	// Build provider options
	opts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	}
	if exporter != nil {
		opts = append(opts, sdktrace.WithBatcher(exporter))
	}

	provider := sdktrace.NewTracerProvider(opts...)

	// Set as global provider
	otel.SetTracerProvider(provider)

	return &Provider{
		provider: provider,
		tracer:   provider.Tracer(serviceName),
		enabled:  true,
	}, nil
}

// Tracer returns the configured tracer for creating spans.
// The returned tracer is safe to use even if tracing is disabled
// (it will be a no-op tracer in that case).
func (p *Provider) Tracer() trace.Tracer {
	return p.tracer
}

// Enabled returns whether tracing is enabled.
func (p *Provider) Enabled() bool {
	return p.enabled
}

// Shutdown flushes pending spans and shuts down the provider.
// It should be called when the application is shutting down to ensure
// all spans are exported before exit.
func (p *Provider) Shutdown(ctx context.Context) error {
	if p.provider != nil {
		return p.provider.Shutdown(ctx)
	}
	return nil
}
