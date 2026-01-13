package chatpanel

import (
	"time"

	"github.com/zjrosen/perles/internal/orchestration/workflow"
)

// Config holds configuration for the chat panel.
type Config struct {
	// ClientType specifies which AI client to use (e.g., "claude", "amp", "codex").
	ClientType string

	// WorkDir is the working directory for the AI process.
	WorkDir string

	// SessionTimeout is the duration after which a session expires
	// and a fresh conversation is started. Default is 30 minutes.
	SessionTimeout time.Duration

	// WorkflowRegistry provides access to workflow templates for the chat panel.
	// When nil, the workflow picker feature is disabled.
	WorkflowRegistry *workflow.Registry

	// VimMode enables vim keybindings in the text input area.
	// When false, the input behaves as a standard textarea.
	VimMode bool
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		ClientType:     "claude",
		SessionTimeout: 30 * time.Minute,
	}
}
