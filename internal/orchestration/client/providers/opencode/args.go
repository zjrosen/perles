package opencode

// buildArgs constructs command line arguments for the OpenCode CLI.
//
// For new sessions:
//
//	opencode run --format json --model <model> -- "prompt"
//
// For resume sessions:
//
//	opencode run --format json --session <id> --model <model> -- "prompt"
//
// The "--" separator is used before the prompt to ensure it's not interpreted
// as a flag, following OpenCode CLI conventions.
func buildArgs(cfg Config, isResume bool) []string {
	// Base args: run subcommand with JSON output format
	args := []string{"run", "--format", "json"}

	// Session resume flag
	if isResume && cfg.SessionID != "" {
		args = append(args, "--session", cfg.SessionID)
	}

	// Model selection
	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}

	// Use "--" separator before prompt to ensure prompt is not parsed as flags.
	// This is especially important for prompts containing special characters.
	args = append(args, "--", cfg.Prompt)

	return args
}
