package tracing

import (
	"github.com/zjrosen/perles/internal/config"
)

// ConfigFromSettings converts the user-facing orchestration.tracing settings into a
// tracing Config. When the "file" exporter is selected without a file_path, the
// default trace file (config.DefaultTracesFilePath) is used. The service name is
// always the default ("perles-orchestrator").
func ConfigFromSettings(settings config.TracingConfig) Config {
	cfg := DefaultConfig()
	cfg.Enabled = settings.Enabled
	cfg.Exporter = settings.Exporter
	cfg.FilePath = settings.FilePath
	cfg.OTLPEndpoint = settings.OTLPEndpoint
	cfg.SampleRate = settings.SampleRate

	if cfg.FilePath == "" && cfg.Exporter == "file" {
		cfg.FilePath = config.DefaultTracesFilePath()
	}

	return cfg
}

// NewProviderFromSettings creates a Provider from the user-facing orchestration.tracing
// settings. It returns (nil, nil) when tracing is disabled so callers can skip wiring
// entirely: a nil tracer keeps the command processor middleware, handlers, and MCP
// servers on their zero-overhead pass-through paths.
//
// Callers own the returned provider and must call Shutdown on exit to flush spans.
func NewProviderFromSettings(settings config.TracingConfig) (*Provider, error) {
	if !settings.Enabled {
		return nil, nil
	}
	return NewProvider(ConfigFromSettings(settings))
}
