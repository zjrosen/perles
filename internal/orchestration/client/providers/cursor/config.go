package cursor

import (
	"time"

	"github.com/zjrosen/perles/internal/orchestration/client"
)

// Config holds configuration for spawning a Cursor process.
type Config struct {
	WorkDir         string
	BeadsDir        string // Path to beads database directory for BEADS_DIR env var
	Prompt          string // Includes prepended system prompt (Cursor has no --append-system-prompt)
	Model           string // e.g., "composer-1"
	SessionID       string // For --resume to continue existing session
	SkipPermissions bool   // Maps to --force flag
	Timeout         time.Duration
	MCPConfig       string // MCP config JSON; written to .cursor/mcp.json before spawn
}

// configFromClient converts a client.Config to a cursor.Config.
// Cursor CLI doesn't support --append-system-prompt, so the system prompt
// is prepended to the main prompt (same pattern as OpenCode).
func configFromClient(cfg client.Config) Config {
	prompt := cfg.Prompt
	if cfg.SystemPrompt != "" && cfg.Prompt != "" {
		prompt = cfg.SystemPrompt + "\n\n" + cfg.Prompt
	} else if cfg.SystemPrompt != "" {
		prompt = cfg.SystemPrompt
	}

	return Config{
		WorkDir:         cfg.WorkDir,
		BeadsDir:        cfg.BeadsDir,
		Prompt:          prompt,
		Model:           cfg.CursorModel(),
		SessionID:       cfg.SessionID,
		SkipPermissions: cfg.SkipPermissions,
		Timeout:         cfg.Timeout,
		MCPConfig:       cfg.MCPConfig,
	}
}
