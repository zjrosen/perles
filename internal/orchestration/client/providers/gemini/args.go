package gemini

// buildArgs constructs the command line arguments for Gemini CLI.
//
// Gemini CLI uses the following argument pattern:
//   - Flags first: ["-m", "<model>", "--yolo", "--output-format", "stream-json"]
//   - Prompt: positional for new sessions, "-p" flag when resuming (required by Gemini CLI)
//   - Model: ["-m", "<model>"]
//   - Session resume: ["--resume", "<session-id>"] (to continue existing session)
//   - Approval mode: ["--approval-mode", "<mode>"] (takes precedence over --yolo)
//   - Skip permissions: ["--yolo"] (when SkipPermissions)
func buildArgs(cfg Config) []string {
	var args []string

	// Model selection (-m flag)
	if cfg.Model != "" {
		args = append(args, "-m", cfg.Model)
	}

	// Session resume (--resume flag)
	if cfg.SessionID != "" {
		args = append(args, "--resume", cfg.SessionID)
	}

	args = append(args, "--yolo")

	// Output format (always stream-json for headless)
	args = append(args, "--output-format", "stream-json")

	// Prompt: When resuming, Gemini CLI requires -p flag instead of positional argument
	if cfg.SessionID != "" {
		args = append(args, "-p", cfg.Prompt)
	} else {
		// Prompt as positional argument for new sessions (must be last)
		args = append(args, cfg.Prompt)
	}

	return args
}
