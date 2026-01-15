package client

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtensionKeys_CodexConstants(t *testing.T) {
	// Verify all Codex extension key constants are defined
	require.Equal(t, "codex.model", ExtCodexModel)
	require.Equal(t, "codex.sandbox", ExtCodexSandbox)
}

func TestConfig_CodexModel_Default(t *testing.T) {
	cfg := Config{}
	require.Equal(t, "gpt-5.2-codex", cfg.CodexModel())
}

func TestConfig_CodexModel_NilExtensions(t *testing.T) {
	cfg := Config{Extensions: nil}
	require.Equal(t, "gpt-5.2-codex", cfg.CodexModel())
}

func TestConfig_CodexModel_EmptyExtensions(t *testing.T) {
	cfg := Config{Extensions: map[string]any{}}
	require.Equal(t, "gpt-5.2-codex", cfg.CodexModel())
}

func TestConfig_CodexModel_EmptyString(t *testing.T) {
	cfg := Config{Extensions: map[string]any{
		ExtCodexModel: "",
	}}
	require.Equal(t, "gpt-5.2-codex", cfg.CodexModel())
}

func TestConfig_CodexModel_CustomModel(t *testing.T) {
	cfg := Config{Extensions: map[string]any{
		ExtCodexModel: "gpt-4o",
	}}
	require.Equal(t, "gpt-4o", cfg.CodexModel())
}

func TestConfig_CodexModel_WrongType(t *testing.T) {
	cfg := Config{Extensions: map[string]any{
		ExtCodexModel: 123, // Not a string
	}}
	require.Equal(t, "gpt-5.2-codex", cfg.CodexModel())
}

func TestConfig_CodexModel_ViaSetExtension(t *testing.T) {
	cfg := Config{}
	cfg.SetExtension(ExtCodexModel, "o1")
	require.Equal(t, "o1", cfg.CodexModel())
}

func TestConfig_ClaudeModel_Default(t *testing.T) {
	cfg := Config{}
	require.Equal(t, "opus", cfg.ClaudeModel())
}

func TestConfig_ClaudeModel_CustomModel(t *testing.T) {
	cfg := Config{Extensions: map[string]any{
		ExtClaudeModel: "sonnet",
	}}
	require.Equal(t, "sonnet", cfg.ClaudeModel())
}

func TestExtensionKeys_GeminiConstants(t *testing.T) {
	// Verify all Gemini extension key constants are defined
	require.Equal(t, "gemini.model", ExtGeminiModel)
}

func TestConfig_GeminiModel_Default(t *testing.T) {
	cfg := Config{}
	require.Equal(t, "gemini-3-pro-preview", cfg.GeminiModel())
}

func TestConfig_GeminiModel_NilExtensions(t *testing.T) {
	cfg := Config{Extensions: nil}
	require.Equal(t, "gemini-3-pro-preview", cfg.GeminiModel())
}

func TestConfig_GeminiModel_EmptyExtensions(t *testing.T) {
	cfg := Config{Extensions: map[string]any{}}
	require.Equal(t, "gemini-3-pro-preview", cfg.GeminiModel())
}

func TestConfig_GeminiModel_EmptyString(t *testing.T) {
	cfg := Config{Extensions: map[string]any{
		ExtGeminiModel: "",
	}}
	require.Equal(t, "gemini-3-pro-preview", cfg.GeminiModel())
}

func TestConfig_GeminiModel_CustomModel(t *testing.T) {
	cfg := Config{Extensions: map[string]any{
		ExtGeminiModel: "gemini-2.5-flash",
	}}
	require.Equal(t, "gemini-2.5-flash", cfg.GeminiModel())
}

func TestConfig_GeminiModel_WrongType(t *testing.T) {
	cfg := Config{Extensions: map[string]any{
		ExtGeminiModel: 123, // Not a string
	}}
	require.Equal(t, "gemini-3-pro-preview", cfg.GeminiModel())
}

func TestConfig_GeminiModel_ViaSetExtension(t *testing.T) {
	cfg := Config{}
	cfg.SetExtension(ExtGeminiModel, "gemini-2.5-flash")
	require.Equal(t, "gemini-2.5-flash", cfg.GeminiModel())
}

// Tests for NewFromClientConfigs helper

func TestNewFromClientConfigs_Claude(t *testing.T) {
	configs := ClientConfigs{
		ClaudeModel: "opus",
	}

	extensions := NewFromClientConfigs(ClientClaude, configs)

	require.Equal(t, "opus", extensions[ExtClaudeModel])
	require.Len(t, extensions, 1)
}

func TestNewFromClientConfigs_Claude_EmptyModel(t *testing.T) {
	configs := ClientConfigs{
		ClaudeModel: "",
	}

	extensions := NewFromClientConfigs(ClientClaude, configs)

	// Empty model should not be added to extensions
	require.Empty(t, extensions)
}

func TestNewFromClientConfigs_Codex(t *testing.T) {
	configs := ClientConfigs{
		CodexModel: "gpt-5.2-codex",
	}

	extensions := NewFromClientConfigs(ClientCodex, configs)

	require.Equal(t, "gpt-5.2-codex", extensions[ExtCodexModel])
	require.Len(t, extensions, 1)
}

func TestNewFromClientConfigs_Codex_EmptyModel(t *testing.T) {
	configs := ClientConfigs{
		CodexModel: "",
	}

	extensions := NewFromClientConfigs(ClientCodex, configs)

	require.Empty(t, extensions)
}

func TestNewFromClientConfigs_Amp_ModelOnly(t *testing.T) {
	configs := ClientConfigs{
		AmpModel: "sonnet",
	}

	extensions := NewFromClientConfigs(ClientAmp, configs)

	require.Equal(t, "sonnet", extensions[ExtAmpModel])
	require.Len(t, extensions, 1)
}

func TestNewFromClientConfigs_Amp_ModeOnly(t *testing.T) {
	configs := ClientConfigs{
		AmpMode: "rush",
	}

	extensions := NewFromClientConfigs(ClientAmp, configs)

	require.Equal(t, "rush", extensions["amp.mode"])
	require.Len(t, extensions, 1)
}

func TestNewFromClientConfigs_Amp_ModelAndMode(t *testing.T) {
	configs := ClientConfigs{
		AmpModel: "opus",
		AmpMode:  "smart",
	}

	extensions := NewFromClientConfigs(ClientAmp, configs)

	require.Equal(t, "opus", extensions[ExtAmpModel])
	require.Equal(t, "smart", extensions["amp.mode"])
	require.Len(t, extensions, 2)
}

func TestNewFromClientConfigs_Amp_EmptyBoth(t *testing.T) {
	configs := ClientConfigs{
		AmpModel: "",
		AmpMode:  "",
	}

	extensions := NewFromClientConfigs(ClientAmp, configs)

	require.Empty(t, extensions)
}

func TestNewFromClientConfigs_Gemini(t *testing.T) {
	configs := ClientConfigs{
		GeminiModel: "gemini-2.5-flash",
	}

	extensions := NewFromClientConfigs(ClientGemini, configs)

	require.Equal(t, "gemini-2.5-flash", extensions[ExtGeminiModel])
	require.Len(t, extensions, 1)
}

func TestNewFromClientConfigs_Gemini_EmptyModel(t *testing.T) {
	configs := ClientConfigs{
		GeminiModel: "",
	}

	extensions := NewFromClientConfigs(ClientGemini, configs)

	require.Empty(t, extensions)
}

func TestNewFromClientConfigs_UnknownClientType(t *testing.T) {
	configs := ClientConfigs{
		ClaudeModel: "opus",
		CodexModel:  "gpt-5.2-codex",
		AmpModel:    "sonnet",
		GeminiModel: "gemini-3-pro-preview",
	}

	// Unknown client type should return empty map
	extensions := NewFromClientConfigs(ClientType("unknown"), configs)

	require.Empty(t, extensions)
}

func TestNewFromClientConfigs_IgnoresIrrelevantConfigs(t *testing.T) {
	// When creating Claude extensions, other client configs should be ignored
	configs := ClientConfigs{
		ClaudeModel: "opus",
		CodexModel:  "gpt-5.2-codex",
		AmpModel:    "sonnet",
		AmpMode:     "rush",
		GeminiModel: "gemini-2.5-flash",
	}

	extensions := NewFromClientConfigs(ClientClaude, configs)

	// Only Claude model should be in extensions
	require.Len(t, extensions, 1)
	require.Equal(t, "opus", extensions[ExtClaudeModel])
	require.NotContains(t, extensions, ExtCodexModel)
	require.NotContains(t, extensions, ExtAmpModel)
	require.NotContains(t, extensions, ExtGeminiModel)
}

func TestNewFromClientConfigs_EmptyConfigs(t *testing.T) {
	configs := ClientConfigs{}

	// All client types should return empty extensions with empty configs
	require.Empty(t, NewFromClientConfigs(ClientClaude, configs))
	require.Empty(t, NewFromClientConfigs(ClientCodex, configs))
	require.Empty(t, NewFromClientConfigs(ClientAmp, configs))
	require.Empty(t, NewFromClientConfigs(ClientGemini, configs))
}
