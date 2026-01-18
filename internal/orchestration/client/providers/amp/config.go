package amp

import (
	"time"

	"github.com/zjrosen/perles/internal/orchestration/client"
)

// Config holds configuration for spawning an Amp process.
type Config struct {
	WorkDir         string
	Prompt          string
	ThreadID        string // For resume (Amp uses "threads" instead of "sessions")
	Model           string // "opus" or "sonnet" (default: opus)
	Mode            string // Agent mode: "free", "rush", "smart"
	SkipPermissions bool
	Timeout         time.Duration
	MCPConfig       string // JSON string for --mcp-config flag
	DisableIDE      bool   // Disable IDE integration
}

// configFromClient converts a client.Config to an amp.Config.
func configFromClient(cfg client.Config) Config {
	// Amp doesn't have a separate system prompt flag, so prefix the prompt
	prompt := cfg.Prompt
	if cfg.SystemPrompt != "" {
		prompt = cfg.SystemPrompt + "\n\n" + cfg.Prompt
	}

	return Config{
		WorkDir:         cfg.WorkDir,
		Prompt:          prompt,
		ThreadID:        cfg.SessionID, // Map session to thread
		Model:           cfg.AmpModel(),
		Mode:            cfg.GetExtensionString(ExtAmpMode),
		SkipPermissions: cfg.SkipPermissions,
		Timeout:         cfg.Timeout,
		MCPConfig:       cfg.MCPConfig,
		DisableIDE:      true, // Always disable IDE in headless mode
	}
}

// Extension keys for Amp-specific configuration.
const (
	// ExtAmpMode specifies the Amp agent mode (string: "free", "rush", "smart").
	ExtAmpMode = "amp.mode"
)
