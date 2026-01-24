package client

import (
	"strings"
	"time"
)

// Config holds provider-agnostic configuration for spawning a process.
type Config struct {
	// WorkDir is the working directory for the process.
	WorkDir string

	// BeadsDir is the path to the beads database directory.
	// When set, spawned processes receive BEADS_DIR environment variable.
	BeadsDir string

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
	// ExtClaudeEnv specifies custom environment variables for Claude (map[string]string).
	// Values support ${VAR} expansion from the current environment.
	ExtClaudeEnv = "claude.env"

	// ExtAmpModel specifies the Amp model selection (string).
	ExtAmpModel = "amp.model"

	// ExtCodexModel specifies the Codex model (string).
	ExtCodexModel = "codex.model"
	// ExtCodexSandbox specifies the sandbox mode (string: "read-only", "workspace-write", "danger-full-access").
	ExtCodexSandbox = "codex.sandbox"

	// ExtGeminiModel specifies the Gemini model (string: "gemini-3-pro-preview", "gemini-2.5-flash").
	ExtGeminiModel = "gemini.model"

	// ExtOpenCodeModel specifies the OpenCode model (string: "opencode/glm-4.7-free").
	ExtOpenCodeModel = "opencode.model"
)

// ClaudeModel returns the Claude model from Extensions, or "opus" as default.
func (c *Config) ClaudeModel() string {
	if c.Extensions == nil {
		return "opus"
	}
	if v, ok := c.Extensions[ExtClaudeModel].(string); ok && v != "" {
		return v
	}
	return "opus"
}

// ClaudeEnv returns custom environment variables for Claude from Extensions.
// Returns nil if not set.
// Note: Keys are uppercased because Viper lowercases YAML map keys,
// but env vars are case-sensitive on Linux.
func (c *Config) ClaudeEnv() map[string]string {
	if c.Extensions == nil {
		return nil
	}
	// Direct type assertion for map[string]string
	if env, ok := c.Extensions[ExtClaudeEnv].(map[string]string); ok {
		// Uppercase keys to fix Viper's lowercasing
		result := make(map[string]string, len(env))
		for k, v := range env {
			result[strings.ToUpper(k)] = v
		}
		return result
	}
	// Handle map[string]any from YAML unmarshaling
	if env, ok := c.Extensions[ExtClaudeEnv].(map[string]any); ok {
		result := make(map[string]string)
		for k, v := range env {
			if s, ok := v.(string); ok {
				// Uppercase keys to fix Viper's lowercasing
				result[strings.ToUpper(k)] = s
			}
		}
		return result
	}
	return nil
}

// CodexModel returns the Codex model from Extensions, or "gpt-5.2-codex" as default.
func (c *Config) CodexModel() string {
	if c.Extensions == nil {
		return "gpt-5.2-codex"
	}
	if v, ok := c.Extensions[ExtCodexModel].(string); ok && v != "" {
		return v
	}
	return "gpt-5.2-codex"
}

// GeminiModel returns the Gemini model from Extensions, or "gemini-3-pro-preview" as default.
func (c *Config) GeminiModel() string {
	if c.Extensions == nil {
		return "gemini-3-pro-preview"
	}
	if v, ok := c.Extensions[ExtGeminiModel].(string); ok && v != "" {
		return v
	}

	return "gemini-3-pro-preview"
}

// AmpModel returns the Amp model from Extensions, or "opus" as default.
func (c *Config) AmpModel() string {
	if c.Extensions == nil {
		return "opus"
	}
	if v, ok := c.Extensions[ExtAmpModel].(string); ok && v != "" {
		return v
	}

	return "opus"
}

// OpenCodeModel returns the OpenCode model from Extensions, or "anthropic/claude-opus-4-5" as default.
func (c *Config) OpenCodeModel() string {
	if c.Extensions == nil {
		return "anthropic/claude-opus-4-5"
	}
	if v, ok := c.Extensions[ExtOpenCodeModel].(string); ok && v != "" {
		return v
	}

	return "anthropic/claude-opus-4-5"
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
