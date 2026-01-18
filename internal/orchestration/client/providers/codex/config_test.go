package codex

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
				SessionID: "session-123",
				Timeout:   5 * time.Minute,
				MCPConfig: `{"servers":{}}`,
			},
			expected: Config{
				WorkDir:   "/work/dir",
				Prompt:    "Hello",
				SessionID: "session-123",
				Model:     "gpt-5.2-codex", // default model
				Timeout:   5 * time.Minute,
				MCPConfig: `{"servers":{}}`,
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
				Model:  "gpt-5.2-codex",
			},
		},
		{
			name: "SystemPrompt only (no Prompt)",
			input: client.Config{
				SystemPrompt: "System instructions only",
			},
			expected: Config{
				Prompt: "System instructions only\n\n",
				Model:  "gpt-5.2-codex",
			},
		},
		{
			name: "Prompt only (no SystemPrompt)",
			input: client.Config{
				Prompt: "Just the prompt",
			},
			expected: Config{
				Prompt: "Just the prompt",
				Model:  "gpt-5.2-codex",
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
				Model:  "gpt-5.2-codex",
			},
		},
		{
			name: "SkipPermissions maps to danger-full-access",
			input: client.Config{
				SkipPermissions: true,
			},
			expected: Config{
				SkipPermissions: true,
				SandboxMode:     "danger-full-access",
				Model:           "gpt-5.2-codex",
			},
		},
		{
			name: "SkipPermissions false does not set SandboxMode",
			input: client.Config{
				SkipPermissions: false,
			},
			expected: Config{
				SkipPermissions: false,
				SandboxMode:     "",
				Model:           "gpt-5.2-codex",
			},
		},
		{
			name: "ExtCodexSandbox overrides SkipPermissions mapping",
			input: client.Config{
				SkipPermissions: true,
				Extensions: map[string]any{
					client.ExtCodexSandbox: "workspace-write",
				},
			},
			expected: Config{
				SkipPermissions: true,
				SandboxMode:     "workspace-write",
				Model:           "gpt-5.2-codex",
			},
		},
		{
			name: "ExtCodexModel is extracted",
			input: client.Config{
				Extensions: map[string]any{
					client.ExtCodexModel: "o4-mini",
				},
			},
			expected: Config{
				Model: "o4-mini",
			},
		},
		{
			name: "ExtCodexSandbox is extracted",
			input: client.Config{
				Extensions: map[string]any{
					client.ExtCodexSandbox: "read-only",
				},
			},
			expected: Config{
				SandboxMode: "read-only",
				Model:       "gpt-5.2-codex",
			},
		},
		{
			name: "MCPConfig passed through",
			input: client.Config{
				MCPConfig: `{"servers":{"test":{"command":"test-server"}}}`,
			},
			expected: Config{
				MCPConfig: `{"servers":{"test":{"command":"test-server"}}}`,
				Model:     "gpt-5.2-codex",
			},
		},
		{
			name:  "empty config handled gracefully",
			input: client.Config{},
			expected: Config{
				WorkDir:         "",
				Prompt:          "",
				SessionID:       "",
				Model:           "gpt-5.2-codex",
				SandboxMode:     "",
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
				Model:   "gpt-5.2-codex",
			},
		},
		{
			name: "all fields combined",
			input: client.Config{
				WorkDir:         "/project",
				Prompt:          "Build the feature",
				SystemPrompt:    "You are a Go expert.",
				SessionID:       "sess-456",
				Timeout:         10 * time.Minute,
				MCPConfig:       `{"test":true}`,
				SkipPermissions: false,
				Extensions: map[string]any{
					client.ExtCodexModel:   "o3",
					client.ExtCodexSandbox: "workspace-write",
				},
			},
			expected: Config{
				WorkDir:         "/project",
				Prompt:          "You are a Go expert.\n\nBuild the feature",
				SessionID:       "sess-456",
				Model:           "o3",
				SandboxMode:     "workspace-write",
				SkipPermissions: false,
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
	t.Run("follows Amp pattern for prepending", func(t *testing.T) {
		cfg := client.Config{
			SystemPrompt: "System message",
			Prompt:       "User message",
		}

		result := configFromClient(cfg)

		// Should be separated by double newline, matching Amp pattern
		assert.Equal(t, "System message\n\nUser message", result.Prompt)
	})
}

func TestConfigFromClient_SandboxModeLogic(t *testing.T) {
	// Dedicated tests for sandbox mode logic
	t.Run("explicit sandbox takes priority over SkipPermissions", func(t *testing.T) {
		cfg := client.Config{
			SkipPermissions: true,
			Extensions: map[string]any{
				client.ExtCodexSandbox: "read-only",
			},
		}

		result := configFromClient(cfg)

		// Explicit sandbox should win over SkipPermissions mapping
		assert.Equal(t, "read-only", result.SandboxMode)
	})

	t.Run("SkipPermissions only affects sandbox when no explicit sandbox", func(t *testing.T) {
		cfg := client.Config{
			SkipPermissions: true,
		}

		result := configFromClient(cfg)

		assert.Equal(t, "danger-full-access", result.SandboxMode)
	})
}
