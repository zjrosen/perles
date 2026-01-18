package opencode

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/zjrosen/perles/internal/orchestration/client"
)

func TestConfigFromClient(t *testing.T) {
	tests := []struct {
		name     string
		input    client.Config
		expected Config
	}{
		{
			name: "basic fields pass through",
			input: client.Config{
				WorkDir:   "/work/dir",
				Prompt:    "Hello",
				Timeout:   5 * time.Minute,
				MCPConfig: `{"mcp":{}}`,
			},
			expected: Config{
				WorkDir:   "/work/dir",
				Prompt:    "Hello",
				Model:     "anthropic/claude-opus-4-5", // default model
				Timeout:   5 * time.Minute,
				MCPConfig: `{"mcp":{}}`,
			},
		},
		{
			name: "SystemPrompt prepended to Prompt",
			input: client.Config{
				SystemPrompt: "You are a helpful assistant.",
				Prompt:       "Do the task",
			},
			expected: Config{
				Prompt: "You are a helpful assistant.\n\nDo the task",
				Model:  "anthropic/claude-opus-4-5",
			},
		},
		{
			name: "SystemPrompt only (no Prompt)",
			input: client.Config{
				SystemPrompt: "System instructions only",
			},
			expected: Config{
				Prompt: "System instructions only\n\n",
				Model:  "anthropic/claude-opus-4-5",
			},
		},
		{
			name: "Prompt only (no SystemPrompt)",
			input: client.Config{
				Prompt: "Just the prompt",
			},
			expected: Config{
				Prompt: "Just the prompt",
				Model:  "anthropic/claude-opus-4-5",
			},
		},
		{
			name: "empty SystemPrompt does not prepend",
			input: client.Config{
				SystemPrompt: "",
				Prompt:       "Only prompt here",
			},
			expected: Config{
				Prompt: "Only prompt here",
				Model:  "anthropic/claude-opus-4-5",
			},
		},
		{
			name: "SkipPermissions passed through",
			input: client.Config{
				SkipPermissions: true,
			},
			expected: Config{
				SkipPermissions: true,
				Model:           "anthropic/claude-opus-4-5",
			},
		},
		{
			name: "SkipPermissions false passed through",
			input: client.Config{
				SkipPermissions: false,
			},
			expected: Config{
				SkipPermissions: false,
				Model:           "anthropic/claude-opus-4-5",
			},
		},
		{
			name: "ExtOpenCodeModel is extracted",
			input: client.Config{
				Extensions: map[string]any{
					client.ExtOpenCodeModel: "custom-model",
				},
			},
			expected: Config{
				Model: "custom-model",
			},
		},
		{
			name: "MCPConfig passed through",
			input: client.Config{
				MCPConfig: `{"mcp":{"test":{"type":"remote","url":"http://localhost"}}}`,
			},
			expected: Config{
				MCPConfig: `{"mcp":{"test":{"type":"remote","url":"http://localhost"}}}`,
				Model:     "anthropic/claude-opus-4-5",
			},
		},
		{
			name: "Timeout passed through",
			input: client.Config{
				Timeout: 10 * time.Minute,
			},
			expected: Config{
				Timeout: 10 * time.Minute,
				Model:   "anthropic/claude-opus-4-5",
			},
		},
		{
			name:  "empty config handled gracefully",
			input: client.Config{},
			expected: Config{
				WorkDir:         "",
				Prompt:          "",
				Model:           "anthropic/claude-opus-4-5",
				SkipPermissions: false,
				Timeout:         0,
				MCPConfig:       "",
			},
		},
		{
			name: "nil extensions handled gracefully",
			input: client.Config{
				WorkDir:    "/test",
				Extensions: nil,
			},
			expected: Config{
				WorkDir: "/test",
				Model:   "anthropic/claude-opus-4-5",
			},
		},
		{
			name: "all fields combined",
			input: client.Config{
				WorkDir:         "/project",
				Prompt:          "Build the feature",
				SystemPrompt:    "You are a Go expert.",
				Timeout:         10 * time.Minute,
				MCPConfig:       `{"test":true}`,
				SkipPermissions: true,
				Extensions: map[string]any{
					client.ExtOpenCodeModel: "opencode/fast-model",
				},
			},
			expected: Config{
				WorkDir:         "/project",
				Prompt:          "You are a Go expert.\n\nBuild the feature",
				Model:           "opencode/fast-model",
				SkipPermissions: true,
				Timeout:         10 * time.Minute,
				MCPConfig:       `{"test":true}`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := configFromClient(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConfigFromClient_SystemPromptPrepending(t *testing.T) {
	// Dedicated test for system prompt prepending behavior
	t.Run("follows Amp/Codex pattern for prepending", func(t *testing.T) {
		cfg := client.Config{
			SystemPrompt: "System message",
			Prompt:       "User message",
		}

		result := configFromClient(cfg)

		// Should be separated by double newline, matching Amp/Codex pattern
		assert.Equal(t, "System message\n\nUser message", result.Prompt)
	})
}

func TestConfigFromClient_ModelDefaulting(t *testing.T) {
	t.Run("defaults to anthropic/claude-opus-4-5 when not specified", func(t *testing.T) {
		cfg := client.Config{
			Prompt: "Test prompt",
		}

		result := configFromClient(cfg)

		assert.Equal(t, "anthropic/claude-opus-4-5", result.Model)
	})

	t.Run("uses specified model when set", func(t *testing.T) {
		cfg := client.Config{
			Prompt: "Test prompt",
			Extensions: map[string]any{
				client.ExtOpenCodeModel: "custom-model",
			},
		}

		result := configFromClient(cfg)

		assert.Equal(t, "custom-model", result.Model)
	})

	t.Run("empty string model defaults to anthropic/claude-opus-4-5", func(t *testing.T) {
		cfg := client.Config{
			Prompt: "Test prompt",
			Extensions: map[string]any{
				client.ExtOpenCodeModel: "",
			},
		}

		result := configFromClient(cfg)

		assert.Equal(t, "anthropic/claude-opus-4-5", result.Model)
	})
}
