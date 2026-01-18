package gemini

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildArgs_MinimalConfig(t *testing.T) {
	cfg := Config{
		Prompt: "What is 2+2?",
	}

	args := buildArgs(cfg)

	// Should have: --yolo --output-format stream-json <prompt>
	assert.Equal(t, []string{"--yolo", "--output-format", "stream-json", "What is 2+2?"}, args)
}

func TestBuildArgs_ModelSelection(t *testing.T) {
	cfg := Config{
		Model:  "gemini-2.5-pro",
		Prompt: "Hello",
	}

	args := buildArgs(cfg)

	// Model comes first, then yolo, output format, then prompt as positional arg
	assert.Equal(t, []string{"-m", "gemini-2.5-pro", "--yolo", "--output-format", "stream-json", "Hello"}, args)
}

func TestBuildArgs_SessionResume(t *testing.T) {
	cfg := Config{
		Model:     "gemini-2.5-pro",
		SessionID: "d2682578-f919-48fc-a9b9-23581f692678",
		Prompt:    "Continue our conversation",
	}

	args := buildArgs(cfg)

	// When resuming, prompt must use -p flag (Gemini CLI requirement)
	expected := []string{
		"-m", "gemini-2.5-pro",
		"--resume", "d2682578-f919-48fc-a9b9-23581f692678",
		"--yolo",
		"--output-format", "stream-json",
		"-p", "Continue our conversation",
	}
	assert.Equal(t, expected, args)
}

func TestBuildArgs_SessionResumeWithYolo(t *testing.T) {
	cfg := Config{
		SessionID:       "sess-123",
		SkipPermissions: true,
		Prompt:          "Hello",
	}

	args := buildArgs(cfg)

	// When resuming, prompt must use -p flag (Gemini CLI requirement)
	expected := []string{
		"--resume", "sess-123",
		"--yolo",
		"--output-format", "stream-json",
		"-p", "Hello",
	}
	assert.Equal(t, expected, args)
}

func TestBuildArgs_SkipPermissions(t *testing.T) {
	cfg := Config{
		SkipPermissions: true,
		Prompt:          "Hello",
	}

	args := buildArgs(cfg)

	// --yolo is always included now
	assert.Equal(t, []string{"--yolo", "--output-format", "stream-json", "Hello"}, args)
}

func TestBuildArgs_FullConfigCombination(t *testing.T) {
	cfg := Config{
		Prompt:          "Implement a feature",
		Model:           "gemini-2.5-flash",
		SkipPermissions: true,
	}

	args := buildArgs(cfg)

	expected := []string{
		"-m", "gemini-2.5-flash",
		"--yolo",
		"--output-format", "stream-json",
		"Implement a feature",
	}
	assert.Equal(t, expected, args)
}

func TestBuildArgs_EmptyPrompt(t *testing.T) {
	cfg := Config{
		Prompt: "",
	}

	args := buildArgs(cfg)

	// Prompt is always included even if empty (as positional arg)
	assert.Equal(t, []string{"--yolo", "--output-format", "stream-json", ""}, args)
}

func TestBuildArgs_PromptWithSpecialCharacters(t *testing.T) {
	tests := []struct {
		name     string
		prompt   string
		expected []string
	}{
		{
			name:     "prompt with quotes",
			prompt:   `Say "hello world"`,
			expected: []string{"--yolo", "--output-format", "stream-json", `Say "hello world"`},
		},
		{
			name:     "prompt with newlines",
			prompt:   "Line 1\nLine 2\nLine 3",
			expected: []string{"--yolo", "--output-format", "stream-json", "Line 1\nLine 2\nLine 3"},
		},
		{
			name:     "prompt with backslashes",
			prompt:   `Path is C:\Users\test`,
			expected: []string{"--yolo", "--output-format", "stream-json", `Path is C:\Users\test`},
		},
		{
			name:     "prompt with single quotes",
			prompt:   "It's working",
			expected: []string{"--yolo", "--output-format", "stream-json", "It's working"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				Prompt: tt.prompt,
			}

			args := buildArgs(cfg)

			assert.Equal(t, tt.expected, args)
		})
	}
}

func TestBuildArgs_ArgumentOrdering(t *testing.T) {
	// Verify that arguments are in the correct order:
	// [-m model] [--yolo] --output-format stream-json <prompt>
	cfg := Config{
		Model:           "gemini-2.5-pro",
		SkipPermissions: true,
		Prompt:          "Final prompt",
	}

	args := buildArgs(cfg)

	// Find positions of each element
	findIndex := func(slice []string, val string) int {
		for i, v := range slice {
			if v == val {
				return i
			}
		}
		return -1
	}

	// Model flag should be first if present
	modelIdx := findIndex(args, "-m")
	assert.Equal(t, 0, modelIdx, "model flag should be first")

	// Yolo flag
	yoloIdx := findIndex(args, "--yolo")
	assert.Greater(t, yoloIdx, modelIdx, "yolo should come after model")

	// Output format flag should come after yolo
	outputIdx := findIndex(args, "--output-format")
	assert.Greater(t, outputIdx, yoloIdx, "output format should come after yolo")

	// Prompt should be last (positional argument)
	assert.Equal(t, "Final prompt", args[len(args)-1], "prompt should be the last argument (positional)")
}

func TestBuildArgs_ModelWithSkipPermissions(t *testing.T) {
	cfg := Config{
		Model:           "gemini-2.5-flash",
		SkipPermissions: true,
		Prompt:          "Test prompt",
	}

	args := buildArgs(cfg)

	expected := []string{
		"-m", "gemini-2.5-flash",
		"--yolo",
		"--output-format", "stream-json",
		"Test prompt",
	}
	assert.Equal(t, expected, args)
}

func TestBuildArgs_OnlyModel(t *testing.T) {
	cfg := Config{
		Model:  "gemini-2.5-pro",
		Prompt: "Hello",
	}

	args := buildArgs(cfg)

	// --yolo is always included now
	assert.Contains(t, args, "--yolo")
	assert.Equal(t, []string{"-m", "gemini-2.5-pro", "--yolo", "--output-format", "stream-json", "Hello"}, args)
}

func TestBuildArgs_OutputFormatAlwaysStreamJSON(t *testing.T) {
	// Verify --output-format stream-json is always present regardless of config
	configs := []Config{
		{Prompt: "test"},
		{Prompt: "test", Model: "gemini-2.5-pro"},
		{Prompt: "test", SkipPermissions: true},
		{Prompt: "", Model: "", SkipPermissions: false},
	}

	for i, cfg := range configs {
		args := buildArgs(cfg)

		// Find --output-format and verify it's followed by stream-json
		found := false
		for j := 0; j < len(args)-1; j++ {
			if args[j] == "--output-format" && args[j+1] == "stream-json" {
				found = true
				break
			}
		}
		assert.True(t, found, "config %d should have --output-format stream-json", i)
	}
}

func TestBuildArgs_PromptIsLastPositionalArg(t *testing.T) {
	// Verify prompt is always the last argument (positional) for non-resume sessions
	configs := []Config{
		{Prompt: "test"},
		{Prompt: ""},
		{Prompt: "multi\nline\nprompt"},
		{Prompt: "with model", Model: "gemini-2.5-pro"},
		{Prompt: "with yolo", SkipPermissions: true},
	}

	for i, cfg := range configs {
		args := buildArgs(cfg)

		// Prompt should be the last element
		assert.Equal(t, cfg.Prompt, args[len(args)-1], "config %d: prompt should be last positional arg", i)
	}
}
