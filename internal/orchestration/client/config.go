package client

import "time"

// Config holds provider-agnostic configuration for spawning a process.
type Config struct {
	// WorkDir is the working directory for the process.
	WorkDir string

	// Prompt is the initial prompt to send to the AI.
	Prompt string

	// SystemPrompt is text to append to the agent's system instructions.
	// How this is applied is provider-specific (e.g., --append-system-prompt for Claude).
	SystemPrompt string

	// MCPConfig is the MCP server configuration as a JSON string.
	// Used to configure MCP servers for the AI process.
	MCPConfig string

	// SessionID is the session identifier for resume operations.
	// How this is interpreted is provider-specific.
	SessionID string

	// Timeout is the maximum duration for the process.
	// Zero means no timeout.
	Timeout time.Duration

	// AllowedTools lists tools that are explicitly allowed.
	AllowedTools []string

	// DisallowedTools lists tools that are explicitly disallowed.
	DisallowedTools []string

	// SkipPermissions bypasses permission prompts.
	// Use with caution.
	SkipPermissions bool

	// Extensions holds provider-specific configuration.
	// Use the Ext* constants for standard keys.
	Extensions map[string]any
}

// Extension keys for provider-specific configuration.
const (
	// ExtClaudeModel specifies the Claude model (string: "sonnet", "opus", "haiku").
	ExtClaudeModel = "claude.model"

	// ExtAmpThreadID specifies the Amp thread reference (string).
	ExtAmpThreadID = "amp.thread_id"
	// ExtAmpModel specifies the Amp model selection (string).
	ExtAmpModel = "amp.model"
)

// ClaudeModel returns the Claude model from Extensions, or "sonnet" as default.
func (c *Config) ClaudeModel() string {
	if c.Extensions == nil {
		return "opus"
	}
	if v, ok := c.Extensions[ExtClaudeModel].(string); ok && v != "" {
		return v
	}
	return "opus"
}

// SetExtension sets a provider-specific extension value.
// Creates the Extensions map if nil.
func (c *Config) SetExtension(key string, value any) {
	if c.Extensions == nil {
		c.Extensions = make(map[string]any)
	}
	c.Extensions[key] = value
}

// GetExtension returns a provider-specific extension value.
// Returns nil if the key doesn't exist.
func (c *Config) GetExtension(key string) any {
	if c.Extensions == nil {
		return nil
	}
	return c.Extensions[key]
}

// GetExtensionString returns a provider-specific extension value as a string.
// Returns empty string if the key doesn't exist or isn't a string.
func (c *Config) GetExtensionString(key string) string {
	if c.Extensions == nil {
		return ""
	}
	if v, ok := c.Extensions[key].(string); ok {
		return v
	}
	return ""
}
