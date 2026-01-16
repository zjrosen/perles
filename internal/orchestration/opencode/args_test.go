package opencode

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildArgs(t *testing.T) {
	tests := []struct {
		name     string
		cfg      Config
		isResume bool
		want     []string
	}{
		{
			name: "new session without model uses default args",
			cfg: Config{
				Prompt: "hello world",
			},
			isResume: false,
			want:     []string{"run", "--format", "json", "--", "hello world"},
		},
		{
			name: "new session with model includes --model flag",
			cfg: Config{
				Prompt: "hello world",
				Model:  "anthropic/claude-opus-4-5",
			},
			isResume: false,
			want:     []string{"run", "--format", "json", "--model", "anthropic/claude-opus-4-5", "--", "hello world"},
		},
		{
			name: "resume session with session ID includes --session flag",
			cfg: Config{
				Prompt:    "follow up",
				SessionID: "ses_abc123",
			},
			isResume: true,
			want:     []string{"run", "--format", "json", "--session", "ses_abc123", "--", "follow up"},
		},
		{
			name: "resume with both session ID and model",
			cfg: Config{
				Prompt:    "continue work",
				SessionID: "ses_xyz789",
				Model:     "opencode/custom-model",
			},
			isResume: true,
			want:     []string{"run", "--format", "json", "--session", "ses_xyz789", "--model", "opencode/custom-model", "--", "continue work"},
		},
		{
			name: "separator present before prompt",
			cfg: Config{
				Prompt: "test prompt",
				Model:  "model",
			},
			isResume: false,
			want:     []string{"run", "--format", "json", "--model", "model", "--", "test prompt"},
		},
		{
			name: "prompt with special characters is preserved",
			cfg: Config{
				Prompt: `Create a function that checks --flag "value" and handles $variables`,
			},
			isResume: false,
			want:     []string{"run", "--format", "json", "--", `Create a function that checks --flag "value" and handles $variables`},
		},
		{
			name: "resume flag set but no session ID omits --session",
			cfg: Config{
				Prompt: "prompt",
				Model:  "model",
			},
			isResume: true,
			want:     []string{"run", "--format", "json", "--model", "model", "--", "prompt"},
		},
		{
			name: "empty prompt is still included after separator",
			cfg: Config{
				Prompt: "",
				Model:  "model",
			},
			isResume: false,
			want:     []string{"run", "--format", "json", "--model", "model", "--", ""},
		},
		{
			name: "prompt with newlines preserved",
			cfg: Config{
				Prompt: "line1\nline2\nline3",
			},
			isResume: false,
			want:     []string{"run", "--format", "json", "--", "line1\nline2\nline3"},
		},
		{
			name: "session ID with special format",
			cfg: Config{
				Prompt:    "test",
				SessionID: "session_2026-01-15_abc123",
			},
			isResume: true,
			want:     []string{"run", "--format", "json", "--session", "session_2026-01-15_abc123", "--", "test"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildArgs(tt.cfg, tt.isResume)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestBuildArgs_SeparatorAlwaysPresent(t *testing.T) {
	// Verify the "--" separator is always present before the prompt
	testCases := []struct {
		name     string
		cfg      Config
		isResume bool
	}{
		{"new session empty", Config{Prompt: "p"}, false},
		{"new session with model", Config{Prompt: "p", Model: "m"}, false},
		{"resume with session", Config{Prompt: "p", SessionID: "s"}, true},
		{"resume with model", Config{Prompt: "p", Model: "m"}, true},
		{"full config", Config{Prompt: "p", Model: "m", SessionID: "s"}, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			args := buildArgs(tc.cfg, tc.isResume)

			// Find the "--" separator
			separatorIdx := -1
			for i, arg := range args {
				if arg == "--" {
					separatorIdx = i
					break
				}
			}

			require.NotEqual(t, -1, separatorIdx, "separator '--' must be present in args")
			require.Equal(t, len(args)-2, separatorIdx, "separator must be second to last")
			require.Equal(t, tc.cfg.Prompt, args[len(args)-1], "prompt must be last argument")
		})
	}
}

func TestBuildArgs_BaseArgsAlwaysPresent(t *testing.T) {
	// Verify "run", "--format", "json" is always at the beginning
	testCases := []Config{
		{Prompt: "test"},
		{Prompt: "test", Model: "m"},
		{Prompt: "test", SessionID: "s"},
		{Prompt: "test", Model: "m", SessionID: "s"},
	}

	for _, cfg := range testCases {
		for _, isResume := range []bool{true, false} {
			args := buildArgs(cfg, isResume)

			require.GreaterOrEqual(t, len(args), 4, "args must have at least base args + separator + prompt")
			require.Equal(t, "run", args[0], "first arg must be 'run'")
			require.Equal(t, "--format", args[1], "second arg must be '--format'")
			require.Equal(t, "json", args[2], "third arg must be 'json'")
		}
	}
}
