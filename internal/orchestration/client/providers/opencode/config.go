package opencode

import (
	"time"

	"github.com/zjrosen/perles/internal/orchestration/client"
)

// Config holds configuration for spawning an OpenCode process.
type Config struct {
	WorkDir         string
	BeadsDir        string // Path to beads database directory for BEADS_DIR env var
	Prompt          string // Includes prefixed system prompt
	Model           string // e.g., "anthropic/claude-opus-4-8"
	SessionID       string // For --session to continue existing session
	SkipPermissions bool   // Future: if OpenCode supports --yolo equivalent
	Timeout         time.Duration
	MCPConfig       string // JSON for opencode.jsonc
}

// configFromClient converts a client.Config to an opencode.Config.
func configFromClient(cfg client.Config) Config {
	// OpenCode doesn't have a --append-system-prompt flag, so prefix the prompt
	prompt := cfg.Prompt
	if cfg.SystemPrompt != "" {
		prompt = cfg.SystemPrompt + "\n\n" + cfg.Prompt
	}

	return Config{
		WorkDir:         cfg.WorkDir,
		BeadsDir:        cfg.BeadsDir,
		Prompt:          prompt,
		Model:           cfg.OpenCodeModel(),
		SessionID:       cfg.SessionID,
		SkipPermissions: cfg.SkipPermissions,
		Timeout:         cfg.Timeout,
		MCPConfig:       cfg.MCPConfig,
	}
}
