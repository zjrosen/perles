package gemini

import (
	"time"

	"github.com/zjrosen/perles/internal/orchestration/client"
)

// Config holds configuration for spawning a Gemini process.
type Config struct {
	WorkDir         string
	BeadsDir        string // Path to beads database directory for BEADS_DIR env var
	Prompt          string // Includes prefixed system prompt
	Model           string // e.g., "gemini-3.1-pro-preview", "gemini-3.5-flash"
	SessionID       string // For --resume to continue existing session
	SkipPermissions bool   // Enables --yolo
	Timeout         time.Duration
	MCPConfig       string // JSON for settings.json
}

// configFromClient converts a client.Config to a gemini.Config.
func configFromClient(cfg client.Config) Config {
	// Gemini doesn't have a --append-system-prompt flag, so prefix the prompt
	prompt := cfg.Prompt
	if cfg.SystemPrompt != "" {
		prompt = cfg.SystemPrompt + "\n\n" + cfg.Prompt
	}

	return Config{
		WorkDir:         cfg.WorkDir,
		BeadsDir:        cfg.BeadsDir,
		Prompt:          prompt,
		Model:           cfg.GeminiModel(),
		SessionID:       cfg.SessionID,
		SkipPermissions: cfg.SkipPermissions,
		Timeout:         cfg.Timeout,
		MCPConfig:       cfg.MCPConfig,
	}
}
