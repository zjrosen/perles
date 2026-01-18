package codex

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildArgs_MinimalConfig(t *testing.T) {
	cfg := Config{
		Prompt: "What is 2+2?",
	}

	args := buildArgs(cfg, false)

	assert.Equal(t, []string{"exec", "--json", "What is 2+2?"}, args)
}

func TestBuildArgs_ResumeWithSessionID(t *testing.T) {
	cfg := Config{
		SessionID: "019b6dea-903b-7bd3-aef5-202a16205a9a",
		Prompt:    "Follow up question",
	}

	args := buildArgs(cfg, true)

	// Resume: exec --json resume <session-id> [prompt]
	// Prompt IS supported for resume (per codex exec resume --help)
	assert.Equal(t, []string{"exec", "--json", "resume", "019b6dea-903b-7bd3-aef5-202a16205a9a", "Follow up question"}, args)
}

func TestBuildArgs_ResumeWithoutSessionID(t *testing.T) {
	cfg := Config{
		SessionID: "",
		Prompt:    "Some prompt",
	}

	args := buildArgs(cfg, true)

	// If resuming but no session ID, falls through to new session mode
	// (unlikely in practice, but buildArgs should handle it gracefully)
	assert.Equal(t, []string{"exec", "--json", "Some prompt"}, args)
}

func TestBuildArgs_ModelSelection(t *testing.T) {
	cfg := Config{
		Model:  "o4-mini",
		Prompt: "Hello",
	}

	args := buildArgs(cfg, false)

	assert.Equal(t, []string{"exec", "--json", "-m", "o4-mini", "Hello"}, args)
}

func TestBuildArgs_SandboxMode(t *testing.T) {
	tests := []struct {
		name        string
		sandboxMode string
		expected    []string
	}{
		{
			name:        "read-only",
			sandboxMode: "read-only",
			expected:    []string{"exec", "--json", "-s", "read-only", "Hello"},
		},
		{
			name:        "workspace-write",
			sandboxMode: "workspace-write",
			expected:    []string{"exec", "--json", "-s", "workspace-write", "Hello"},
		},
		{
			name:        "danger-full-access",
			sandboxMode: "danger-full-access",
			expected:    []string{"exec", "--json", "-s", "danger-full-access", "Hello"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				SandboxMode: tt.sandboxMode,
				Prompt:      "Hello",
			}

			args := buildArgs(cfg, false)

			assert.Equal(t, tt.expected, args)
		})
	}
}

func TestBuildArgs_SkipPermissions(t *testing.T) {
	cfg := Config{
		SkipPermissions: true,
		Prompt:          "Hello",
	}

	args := buildArgs(cfg, false)

	assert.Equal(t, []string{"exec", "--json", "--dangerously-bypass-approvals-and-sandbox", "Hello"}, args)
}

func TestBuildArgs_SandboxModeTakesPrecedenceOverSkipPermissions(t *testing.T) {
	cfg := Config{
		SandboxMode:     "workspace-write",
		SkipPermissions: true, // Should be ignored when SandboxMode is set
		Prompt:          "Hello",
	}

	args := buildArgs(cfg, false)

	// SandboxMode should take precedence
	assert.Equal(t, []string{"exec", "--json", "-s", "workspace-write", "Hello"}, args)
	// Should NOT contain --dangerously-bypass-approvals-and-sandbox
	assert.NotContains(t, args, "--dangerously-bypass-approvals-and-sandbox")
}

func TestBuildArgs_WorkingDirectory(t *testing.T) {
	cfg := Config{
		WorkDir: "/home/user/project",
		Prompt:  "Hello",
	}

	args := buildArgs(cfg, false)

	assert.Equal(t, []string{"exec", "--json", "-C", "/home/user/project", "Hello"}, args)
}

func TestBuildArgs_MCPConfig(t *testing.T) {
	// MCP config uses TOML syntax via -c flag
	mcpConfig := `mcp_servers.perles-worker={url="http://localhost:8765/worker/WORKER-1"}`
	cfg := Config{
		MCPConfig: mcpConfig,
		Prompt:    "Hello",
	}

	args := buildArgs(cfg, false)

	assert.Equal(t, []string{"exec", "--json", "-c", mcpConfig, "Hello"}, args)
}

func TestBuildArgs_FullConfigCombination(t *testing.T) {
	mcpConfig := `mcp_servers.perles-worker={url="http://localhost:8765/worker/WORKER-1"}`
	cfg := Config{
		WorkDir:     "/home/user/project",
		Prompt:      "Implement a feature",
		Model:       "o3",
		SandboxMode: "workspace-write",
		MCPConfig:   mcpConfig,
	}

	args := buildArgs(cfg, false)

	expected := []string{
		"exec", "--json",
		"-m", "o3",
		"-s", "workspace-write",
		"-C", "/home/user/project",
		"-c", mcpConfig,
		"Implement a feature",
	}
	assert.Equal(t, expected, args)
}

func TestBuildArgs_EmptyPrompt(t *testing.T) {
	cfg := Config{
		Prompt: "",
	}

	args := buildArgs(cfg, false)

	// Just base args, no prompt
	assert.Equal(t, []string{"exec", "--json"}, args)
}

func TestBuildArgs_ResumeWithPrompt(t *testing.T) {
	cfg := Config{
		SessionID: "session-123",
		Prompt:    "Follow up message",
	}

	args := buildArgs(cfg, true)

	// Prompt IS included in resume mode (per codex exec resume --help)
	assert.Equal(t, []string{"exec", "--json", "resume", "session-123", "Follow up message"}, args)
}

func TestBuildArgs_NotResumeWithSessionID(t *testing.T) {
	// Session ID is set but isResume is false - prompt should be used, sessionID ignored
	cfg := Config{
		SessionID: "session-123",
		Prompt:    "New prompt",
	}

	args := buildArgs(cfg, false)

	// Should NOT have resume subcommand
	assert.Equal(t, []string{"exec", "--json", "New prompt"}, args)
	assert.NotContains(t, args, "resume")
	assert.NotContains(t, args, "session-123")
}

func TestBuildArgs_PromptWithSpecialCharacters(t *testing.T) {
	// Test that prompts with special characters are passed correctly
	// (The actual escaping is handled by exec.Command, but we should test the args are built correctly)
	tests := []struct {
		name     string
		prompt   string
		expected []string
	}{
		{
			name:     "prompt with quotes",
			prompt:   `Say "hello world"`,
			expected: []string{"exec", "--json", `Say "hello world"`},
		},
		{
			name:     "prompt with newlines",
			prompt:   "Line 1\nLine 2\nLine 3",
			expected: []string{"exec", "--json", "Line 1\nLine 2\nLine 3"},
		},
		{
			name:     "prompt with backslashes",
			prompt:   `Path is C:\Users\test`,
			expected: []string{"exec", "--json", `Path is C:\Users\test`},
		},
		{
			name:     "prompt with single quotes",
			prompt:   "It's working",
			expected: []string{"exec", "--json", "It's working"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				Prompt: tt.prompt,
			}

			args := buildArgs(cfg, false)

			assert.Equal(t, tt.expected, args)
		})
	}
}

func TestBuildArgs_ArgumentOrdering(t *testing.T) {
	// Verify that arguments are in the correct order:
	// exec --json [resume <session>] [-m model] [-s mode] [-C dir] [-c config] [prompt]
	mcpConfig := `mcp_servers.test={url="http://localhost:1234"}`
	cfg := Config{
		Model:       "o4-mini",
		SandboxMode: "read-only",
		WorkDir:     "/test/dir",
		MCPConfig:   mcpConfig,
		Prompt:      "Final prompt",
	}

	args := buildArgs(cfg, false)

	// exec and --json must be first
	assert.Equal(t, "exec", args[0])
	assert.Equal(t, "--json", args[1])

	// Find positions of each element
	findIndex := func(slice []string, val string) int {
		for i, v := range slice {
			if v == val {
				return i
			}
		}
		return -1
	}

	// Model flag comes after base args
	modelIdx := findIndex(args, "-m")
	assert.Greater(t, modelIdx, 1, "model flag should come after base args")

	// Sandbox flag
	sandboxIdx := findIndex(args, "-s")
	assert.Greater(t, sandboxIdx, 1, "sandbox flag should come after base args")

	// Working dir flag
	wdIdx := findIndex(args, "-C")
	assert.Greater(t, wdIdx, 1, "working dir flag should come after base args")

	// MCP config flag
	mcpIdx := findIndex(args, "-c")
	assert.Greater(t, mcpIdx, 1, "MCP config flag should come after base args")

	// Prompt should be last
	assert.Equal(t, "Final prompt", args[len(args)-1], "prompt should be the last argument")
}
