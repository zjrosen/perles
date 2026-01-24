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

func TestExtensionKeys_OpenCodeConstants(t *testing.T) {
	// Verify all OpenCode extension key constants are defined
	require.Equal(t, "opencode.model", ExtOpenCodeModel)
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

func TestConfig_OpenCodeModel_Default(t *testing.T) {
	cfg := Config{}
	require.Equal(t, "anthropic/claude-opus-4-5", cfg.OpenCodeModel())
}

func TestConfig_OpenCodeModel_NilExtensions(t *testing.T) {
	cfg := Config{Extensions: nil}
	require.Equal(t, "anthropic/claude-opus-4-5", cfg.OpenCodeModel())
}

func TestConfig_OpenCodeModel_EmptyExtensions(t *testing.T) {
	cfg := Config{Extensions: map[string]any{}}
	require.Equal(t, "anthropic/claude-opus-4-5", cfg.OpenCodeModel())
}

func TestConfig_OpenCodeModel_EmptyString(t *testing.T) {
	cfg := Config{Extensions: map[string]any{
		ExtOpenCodeModel: "",
	}}
	require.Equal(t, "anthropic/claude-opus-4-5", cfg.OpenCodeModel())
}

func TestConfig_OpenCodeModel_CustomModel(t *testing.T) {
	cfg := Config{Extensions: map[string]any{
		ExtOpenCodeModel: "opencode/custom-model",
	}}
	require.Equal(t, "opencode/custom-model", cfg.OpenCodeModel())
}

func TestConfig_OpenCodeModel_WrongType(t *testing.T) {
	cfg := Config{Extensions: map[string]any{
		ExtOpenCodeModel: 123, // Not a string
	}}
	require.Equal(t, "anthropic/claude-opus-4-5", cfg.OpenCodeModel())
}

func TestConfig_OpenCodeModel_ViaSetExtension(t *testing.T) {
	cfg := Config{}
	cfg.SetExtension(ExtOpenCodeModel, "opencode/glm-4.8")
	require.Equal(t, "opencode/glm-4.8", cfg.OpenCodeModel())
}

// ============================================================================
// Claude Extension Constants
// ============================================================================

func TestExtensionKeys_ClaudeConstants(t *testing.T) {
	require.Equal(t, "claude.model", ExtClaudeModel)
	require.Equal(t, "claude.env", ExtClaudeEnv)
}

// ============================================================================
// ClaudeModel Additional Tests
// ============================================================================

func TestConfig_ClaudeModel_NilExtensions(t *testing.T) {
	cfg := Config{Extensions: nil}
	require.Equal(t, "opus", cfg.ClaudeModel())
}

func TestConfig_ClaudeModel_EmptyExtensions(t *testing.T) {
	cfg := Config{Extensions: map[string]any{}}
	require.Equal(t, "opus", cfg.ClaudeModel())
}

func TestConfig_ClaudeModel_EmptyString(t *testing.T) {
	cfg := Config{Extensions: map[string]any{
		ExtClaudeModel: "",
	}}
	require.Equal(t, "opus", cfg.ClaudeModel())
}

func TestConfig_ClaudeModel_WrongType(t *testing.T) {
	cfg := Config{Extensions: map[string]any{
		ExtClaudeModel: 123, // Not a string
	}}
	require.Equal(t, "opus", cfg.ClaudeModel())
}

func TestConfig_ClaudeModel_ViaSetExtension(t *testing.T) {
	cfg := Config{}
	cfg.SetExtension(ExtClaudeModel, "haiku")
	require.Equal(t, "haiku", cfg.ClaudeModel())
}

// ============================================================================
// ClaudeEnv Tests
// ============================================================================

func TestConfig_ClaudeEnv_NilExtensions(t *testing.T) {
	cfg := Config{Extensions: nil}
	require.Nil(t, cfg.ClaudeEnv())
}

func TestConfig_ClaudeEnv_EmptyExtensions(t *testing.T) {
	cfg := Config{Extensions: map[string]any{}}
	require.Nil(t, cfg.ClaudeEnv())
}

func TestConfig_ClaudeEnv_MapStringString(t *testing.T) {
	cfg := Config{Extensions: map[string]any{
		ExtClaudeEnv: map[string]string{
			"api_key": "secret123",
			"region":  "us-west-2",
		},
	}}
	env := cfg.ClaudeEnv()
	require.NotNil(t, env)
	require.Equal(t, "secret123", env["API_KEY"])
	require.Equal(t, "us-west-2", env["REGION"])
}

func TestConfig_ClaudeEnv_MapStringAny(t *testing.T) {
	// This simulates YAML unmarshaling behavior
	cfg := Config{Extensions: map[string]any{
		ExtClaudeEnv: map[string]any{
			"api_key": "secret456",
			"region":  "eu-west-1",
		},
	}}
	env := cfg.ClaudeEnv()
	require.NotNil(t, env)
	require.Equal(t, "secret456", env["API_KEY"])
	require.Equal(t, "eu-west-1", env["REGION"])
}

func TestConfig_ClaudeEnv_MapStringAny_NonStringValues(t *testing.T) {
	cfg := Config{Extensions: map[string]any{
		ExtClaudeEnv: map[string]any{
			"valid_key":   "valid_value",
			"invalid_key": 123, // Not a string - should be skipped
		},
	}}
	env := cfg.ClaudeEnv()
	require.NotNil(t, env)
	require.Equal(t, "valid_value", env["VALID_KEY"])
	require.NotContains(t, env, "INVALID_KEY")
}

func TestConfig_ClaudeEnv_WrongType(t *testing.T) {
	cfg := Config{Extensions: map[string]any{
		ExtClaudeEnv: "not a map",
	}}
	require.Nil(t, cfg.ClaudeEnv())
}

func TestConfig_ClaudeEnv_UppercasesKeys(t *testing.T) {
	// Viper lowercases YAML keys, but env vars are case-sensitive
	cfg := Config{Extensions: map[string]any{
		ExtClaudeEnv: map[string]string{
			"my_api_key": "value",
		},
	}}
	env := cfg.ClaudeEnv()
	require.NotNil(t, env)
	require.Equal(t, "value", env["MY_API_KEY"])
	require.NotContains(t, env, "my_api_key")
}

// ============================================================================
// Amp Extension Constants and Model Tests
// ============================================================================

func TestExtensionKeys_AmpConstants(t *testing.T) {
	require.Equal(t, "amp.model", ExtAmpModel)
}

func TestConfig_AmpModel_Default(t *testing.T) {
	cfg := Config{}
	require.Equal(t, "opus", cfg.AmpModel())
}

func TestConfig_AmpModel_NilExtensions(t *testing.T) {
	cfg := Config{Extensions: nil}
	require.Equal(t, "opus", cfg.AmpModel())
}

func TestConfig_AmpModel_EmptyExtensions(t *testing.T) {
	cfg := Config{Extensions: map[string]any{}}
	require.Equal(t, "opus", cfg.AmpModel())
}

func TestConfig_AmpModel_EmptyString(t *testing.T) {
	cfg := Config{Extensions: map[string]any{
		ExtAmpModel: "",
	}}
	require.Equal(t, "opus", cfg.AmpModel())
}

func TestConfig_AmpModel_CustomModel(t *testing.T) {
	cfg := Config{Extensions: map[string]any{
		ExtAmpModel: "sonnet",
	}}
	require.Equal(t, "sonnet", cfg.AmpModel())
}

func TestConfig_AmpModel_WrongType(t *testing.T) {
	cfg := Config{Extensions: map[string]any{
		ExtAmpModel: 123, // Not a string
	}}
	require.Equal(t, "opus", cfg.AmpModel())
}

func TestConfig_AmpModel_ViaSetExtension(t *testing.T) {
	cfg := Config{}
	cfg.SetExtension(ExtAmpModel, "haiku")
	require.Equal(t, "haiku", cfg.AmpModel())
}

// ============================================================================
// GetExtension Tests
// ============================================================================

func TestConfig_GetExtension_NilExtensions(t *testing.T) {
	cfg := Config{Extensions: nil}
	require.Nil(t, cfg.GetExtension("any.key"))
}

func TestConfig_GetExtension_KeyNotFound(t *testing.T) {
	cfg := Config{Extensions: map[string]any{
		"some.key": "value",
	}}
	require.Nil(t, cfg.GetExtension("other.key"))
}

func TestConfig_GetExtension_StringValue(t *testing.T) {
	cfg := Config{Extensions: map[string]any{
		"my.key": "my-value",
	}}
	require.Equal(t, "my-value", cfg.GetExtension("my.key"))
}

func TestConfig_GetExtension_IntValue(t *testing.T) {
	cfg := Config{Extensions: map[string]any{
		"my.int": 42,
	}}
	require.Equal(t, 42, cfg.GetExtension("my.int"))
}

func TestConfig_GetExtension_MapValue(t *testing.T) {
	expected := map[string]string{"a": "b"}
	cfg := Config{Extensions: map[string]any{
		"my.map": expected,
	}}
	require.Equal(t, expected, cfg.GetExtension("my.map"))
}

// ============================================================================
// GetExtensionString Tests
// ============================================================================

func TestConfig_GetExtensionString_NilExtensions(t *testing.T) {
	cfg := Config{Extensions: nil}
	require.Equal(t, "", cfg.GetExtensionString("any.key"))
}

func TestConfig_GetExtensionString_KeyNotFound(t *testing.T) {
	cfg := Config{Extensions: map[string]any{
		"some.key": "value",
	}}
	require.Equal(t, "", cfg.GetExtensionString("other.key"))
}

func TestConfig_GetExtensionString_StringValue(t *testing.T) {
	cfg := Config{Extensions: map[string]any{
		"my.key": "my-value",
	}}
	require.Equal(t, "my-value", cfg.GetExtensionString("my.key"))
}

func TestConfig_GetExtensionString_NonStringValue(t *testing.T) {
	cfg := Config{Extensions: map[string]any{
		"my.int": 42,
	}}
	require.Equal(t, "", cfg.GetExtensionString("my.int"))
}

func TestConfig_GetExtensionString_EmptyString(t *testing.T) {
	cfg := Config{Extensions: map[string]any{
		"my.key": "",
	}}
	require.Equal(t, "", cfg.GetExtensionString("my.key"))
}

// ============================================================================
// SetExtension Tests
// ============================================================================

func TestConfig_SetExtension_CreatesMap(t *testing.T) {
	cfg := Config{}
	require.Nil(t, cfg.Extensions)

	cfg.SetExtension("my.key", "my-value")

	require.NotNil(t, cfg.Extensions)
	require.Equal(t, "my-value", cfg.Extensions["my.key"])
}

func TestConfig_SetExtension_OverwritesExisting(t *testing.T) {
	cfg := Config{Extensions: map[string]any{
		"my.key": "old-value",
	}}

	cfg.SetExtension("my.key", "new-value")

	require.Equal(t, "new-value", cfg.Extensions["my.key"])
}

func TestConfig_SetExtension_PreservesOtherKeys(t *testing.T) {
	cfg := Config{Extensions: map[string]any{
		"existing.key": "existing-value",
	}}

	cfg.SetExtension("new.key", "new-value")

	require.Equal(t, "existing-value", cfg.Extensions["existing.key"])
	require.Equal(t, "new-value", cfg.Extensions["new.key"])
}
