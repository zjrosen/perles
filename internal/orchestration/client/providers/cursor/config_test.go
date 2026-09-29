package cursor

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
				WorkDir:  "/work/dir",
				BeadsDir: "/path/to/beads",
				Prompt:   "Hello",
				Timeout:  5 * time.Minute,
			},
			expected: Config{
				WorkDir:  "/work/dir",
				BeadsDir: "/path/to/beads",
				Prompt:   "Hello",
				Timeout:  5 * time.Minute,
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
			},
		},
		{
			name: "SystemPrompt only (no Prompt)",
			input: client.Config{
				SystemPrompt: "System instructions only",
			},
			expected: Config{
				Prompt: "System instructions only",
			},
		},
		{
			name: "Prompt only (no SystemPrompt)",
			input: client.Config{
				Prompt: "Just the prompt",
			},
			expected: Config{
				Prompt: "Just the prompt",
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
			},
		},
		{
			name: "SkipPermissions passed through",
			input: client.Config{
				SkipPermissions: true,
			},
			expected: Config{
				SkipPermissions: true,
			},
		},
		{
			name: "ExtCursorModel is extracted",
			input: client.Config{
				Extensions: map[string]any{
					client.ExtCursorModel: "composer-1",
				},
			},
			expected: Config{
				Model: "composer-1",
			},
		},
		{
			name:  "empty config handled gracefully",
			input: client.Config{},
			expected: Config{
				WorkDir:         "",
				Prompt:          "",
				Model:           "",
				SkipPermissions: false,
				Timeout:         0,
			},
		},
		{
			name: "MCPConfig passed through",
			input: client.Config{
				Prompt:    "test",
				MCPConfig: `{"mcpServers":{"perles-orchestrator":{"url":"http://localhost:9000/mcp"}}}`,
			},
			expected: Config{
				Prompt:    "test",
				MCPConfig: `{"mcpServers":{"perles-orchestrator":{"url":"http://localhost:9000/mcp"}}}`,
			},
		},
		{
			name: "all fields combined",
			input: client.Config{
				WorkDir:         "/project",
				BeadsDir:        "/beads",
				Prompt:          "Build the feature",
				SystemPrompt:    "You are a Go expert.",
				MCPConfig:       `{"mcpServers":{"perles-orchestrator":{"url":"http://localhost:9000/mcp"}}}`,
				Timeout:         10 * time.Minute,
				SkipPermissions: true,
				Extensions: map[string]any{
					client.ExtCursorModel: "composer-1",
				},
			},
			expected: Config{
				WorkDir:         "/project",
				BeadsDir:        "/beads",
				Prompt:          "You are a Go expert.\n\nBuild the feature",
				MCPConfig:       `{"mcpServers":{"perles-orchestrator":{"url":"http://localhost:9000/mcp"}}}`,
				Model:           "composer-1",
				SkipPermissions: true,
				Timeout:         10 * time.Minute,
			},
		},
		{
			name: "unsupported fields are silently ignored",
			input: client.Config{
				Prompt:       "test",
				AllowedTools: []string{"Bash"},
			},
			expected: Config{
				Prompt: "test",
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
