package tracing

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/config"
)

func TestConfigFromSettings_CopiesFields(t *testing.T) {
	cfg := ConfigFromSettings(config.TracingConfig{
		Enabled:      true,
		Exporter:     "otlp",
		FilePath:     "/tmp/custom.jsonl",
		OTLPEndpoint: "collector:4317",
		SampleRate:   0.25,
	})

	require.True(t, cfg.Enabled)
	require.Equal(t, "otlp", cfg.Exporter)
	require.Equal(t, "/tmp/custom.jsonl", cfg.FilePath)
	require.Equal(t, "collector:4317", cfg.OTLPEndpoint)
	require.Equal(t, 0.25, cfg.SampleRate)
	require.Equal(t, "perles-orchestrator", cfg.ServiceName)
}

func TestConfigFromSettings_DefaultsFilePathForFileExporter(t *testing.T) {
	cfg := ConfigFromSettings(config.TracingConfig{
		Enabled:  true,
		Exporter: "file",
	})

	require.Equal(t, config.DefaultTracesFilePath(), cfg.FilePath)
}

func TestConfigFromSettings_NoDefaultFilePathForOtherExporters(t *testing.T) {
	cfg := ConfigFromSettings(config.TracingConfig{
		Enabled:  true,
		Exporter: "stdout",
	})

	require.Empty(t, cfg.FilePath)
}

func TestNewProviderFromSettings_DisabledReturnsNil(t *testing.T) {
	provider, err := NewProviderFromSettings(config.TracingConfig{
		Enabled:  false,
		Exporter: "file",
	})

	require.NoError(t, err)
	require.Nil(t, provider, "disabled tracing should not create a provider")
}

func TestNewProviderFromSettings_EnabledFileExporter(t *testing.T) {
	tracePath := filepath.Join(t.TempDir(), "traces", "traces.jsonl")

	provider, err := NewProviderFromSettings(config.TracingConfig{
		Enabled:    true,
		Exporter:   "file",
		FilePath:   tracePath,
		SampleRate: 1.0,
	})
	require.NoError(t, err)
	require.NotNil(t, provider)
	require.True(t, provider.Enabled())

	_, span := provider.Tracer().Start(context.Background(), "test-span")
	span.End()

	require.NoError(t, provider.Shutdown(context.Background()))

	data, err := os.ReadFile(tracePath)
	require.NoError(t, err)
	require.Contains(t, string(data), "test-span", "span should be flushed to the trace file on shutdown")
}

func TestNewProviderFromSettings_UnsupportedExporterErrors(t *testing.T) {
	provider, err := NewProviderFromSettings(config.TracingConfig{
		Enabled:  true,
		Exporter: "bogus",
	})

	require.Error(t, err)
	require.Nil(t, provider)
}
