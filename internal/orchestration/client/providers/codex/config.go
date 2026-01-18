package codex

import (
	"time"

	"github.com/zjrosen/perles/internal/orchestration/client"
)

// Config holds configuration for spawning a Codex process.
type Config struct {
	WorkDir         string
	Prompt          string
	SessionID       string // For resume (Codex uses "sessions")
	Model           string // e.g., "gpt-5.2-codex", "o4-mini" (default: gpt-5.2-codex)
	SandboxMode     string // "read-only", "workspace-write", "danger-full-access"
	SkipPermissions bool
	Timeout         time.Duration
	MCPConfig       string // JSON string for -c flag TOML conversion
}

// configFromClient converts a client.Config to a codex.Config.
func configFromClient(cfg client.Config) Config {
	// Codex doesn't have a separate system prompt flag, so prefix the prompt
	prompt := cfg.Prompt
	if cfg.SystemPrompt != "" {
		prompt = cfg.SystemPrompt + "\n\n" + cfg.Prompt
	}

	// Determine sandbox mode:
	// 1. Explicit ExtCodexSandbox takes priority
	// 2. SkipPermissions=true maps to "danger-full-access"
	// 3. Default is empty (let Codex use its default)
	sandboxMode := cfg.GetExtensionString(client.ExtCodexSandbox)
	if sandboxMode == "" && cfg.SkipPermissions {
		sandboxMode = "danger-full-access"
	}

	return Config{
		WorkDir:         cfg.WorkDir,
		Prompt:          prompt,
		SessionID:       cfg.SessionID,
		Model:           cfg.CodexModel(),
		SandboxMode:     sandboxMode,
		SkipPermissions: cfg.SkipPermissions,
		Timeout:         cfg.Timeout,
		MCPConfig:       cfg.MCPConfig,
	}
}
