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

	// ExtAmpModel specifies the Amp model selection (string).
	ExtAmpModel = "amp.model"

	// ExtCodexModel specifies the Codex model (string).
	ExtCodexModel = "codex.model"
	// ExtCodexSandbox specifies the sandbox mode (string: "read-only", "workspace-write", "danger-full-access").
	ExtCodexSandbox = "codex.sandbox"

	// ExtGeminiModel specifies the Gemini model (string: "gemini-3-pro-preview", "gemini-2.5-flash").
	ExtGeminiModel = "gemini.model"
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

// ClientConfigs groups all client-specific configuration structs.
// This is used by NewFromClientConfigs to consolidate extensions building logic.
type ClientConfigs struct {
	ClaudeModel string // Claude model (sonnet, opus, haiku)
	CodexModel  string // Codex model (gpt-5.2-codex, o4-mini)
	AmpModel    string // Amp model (opus, sonnet)
	AmpMode     string // Amp mode (free, rush, smart)
	GeminiModel string // Gemini model (gemini-3-pro-preview, gemini-2.5-flash)
}

// NewFromClientConfigs builds a provider-specific Extensions map based on the client type.
//
// The function takes the client type and a ClientConfigs struct containing all possible
// client-specific settings, and returns an Extensions map with only the relevant settings
// for the specified client type.
//
// Example:
//
//	extensions := client.NewFromClientConfigs(client.ClientClaude, client.ClientConfigs{
//	    ClaudeModel: "opus",
//	})
//	// Result: map[string]any{"claude.model": "opus"}
func NewFromClientConfigs(clientType ClientType, configs ClientConfigs) map[string]any {
	extensions := make(map[string]any)

	switch clientType {
	case ClientClaude:
		if configs.ClaudeModel != "" {
			extensions[ExtClaudeModel] = configs.ClaudeModel
		}
	case ClientCodex:
		if configs.CodexModel != "" {
			extensions[ExtCodexModel] = configs.CodexModel
		}
	case ClientAmp:
		if configs.AmpModel != "" {
			extensions[ExtAmpModel] = configs.AmpModel
		}
		if configs.AmpMode != "" {
			// Note: Amp mode key is defined in amp package, but we use the literal here
			// to avoid import cycle. The value is "amp.mode".
			extensions["amp.mode"] = configs.AmpMode
		}
	case ClientGemini:
		if configs.GeminiModel != "" {
			extensions[ExtGeminiModel] = configs.GeminiModel
		}
	}

	return extensions
}
