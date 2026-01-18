package amp

// buildArgs constructs the command line arguments for amp.
// For new sessions, the prompt is passed as the final positional argument.
// For resume, we use "threads continue <thread-id>".
func buildArgs(cfg Config, isResume bool) []string {
	var args []string

	// For resume, use "threads continue <thread-id>" subcommand
	if isResume && cfg.ThreadID != "" {
		args = append(args, "threads", "continue", cfg.ThreadID)
	}

	// Skip permission prompts
	if cfg.SkipPermissions {
		args = append(args, "--dangerously-allow-all")
	}

	// Disable notifications in headless mode
	args = append(args, "--no-notifications")

	// Disable IDE integration in headless mode
	if cfg.DisableIDE {
		args = append(args, "--no-ide")
	}

	// Model selection: Amp defaults to Opus, use --use-sonnet for Sonnet
	if cfg.Model == "sonnet" {
		args = append(args, "--use-sonnet")
	}

	// Agent mode
	if cfg.Mode != "" {
		args = append(args, "-m", cfg.Mode)
	}

	// MCP configuration
	if cfg.MCPConfig != "" {
		args = append(args, "--mcp-config", cfg.MCPConfig)
	}

	// Execute mode with stream-json output
	args = append(args, "--stream-json", "-x")

	// Prompt as final positional argument (if present)
	if cfg.Prompt != "" {
		args = append(args, cfg.Prompt)
	}

	return args
}
