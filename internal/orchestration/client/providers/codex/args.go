package codex

// buildArgs constructs the command line arguments for Codex CLI.
//
// Codex uses `codex exec` for headless execution with the following argument pattern:
//
// For new sessions (exec):
//   - Base: ["exec", "--json"]
//   - Model: ["-m", "<model>"]
//   - Sandbox: ["-s", "<mode>"]
//   - Skip permissions: ["--dangerously-bypass-approvals-and-sandbox"]
//   - Working directory: ["-C", "<dir>"]
//   - MCP config: ["-c", "mcp_servers.NAME={url=\"...\"}"] (TOML syntax)
//   - Prompt: final positional argument
//
// For resume sessions (exec resume):
//   - Base: ["exec", "--json", "resume", "<session-id>"]
//   - MCP config: ["-c", "mcp_servers.NAME={url=\"...\"}"] (TOML syntax)
//   - Prompt: optional final argument
//
// Note: The resume subcommand does NOT support -s, -m, or -C flags.
func buildArgs(cfg Config, isResume bool) []string {
	// Base args: exec subcommand with JSON output
	args := []string{"exec", "--json"}

	if isResume && cfg.SessionID != "" {
		// Resume mode: limited options available
		// Format: codex exec --json resume <session-id> [-c config] [prompt]
		args = append(args, "resume", cfg.SessionID)

		// MCP configuration via -c flag (supported in resume)
		if cfg.MCPConfig != "" {
			args = append(args, "-c", cfg.MCPConfig)
		}

		// Prompt as optional final argument when resuming
		if cfg.Prompt != "" {
			args = append(args, cfg.Prompt)
		}
	} else {
		// New session mode: full options available
		// Model selection (-m flag)
		if cfg.Model != "" {
			args = append(args, "-m", cfg.Model)
		}

		// Sandbox mode: explicit mode takes precedence over skip permissions
		// Available modes: read-only, workspace-write, danger-full-access
		if cfg.SandboxMode != "" {
			args = append(args, "-s", cfg.SandboxMode)
		} else if cfg.SkipPermissions {
			// Fall back to --dangerously-bypass-approvals-and-sandbox
			args = append(args, "--dangerously-bypass-approvals-and-sandbox")
		}

		// Working directory (-C flag)
		// Note: We also set cmd.Dir in process.go for belt-and-suspenders reliability
		if cfg.WorkDir != "" {
			args = append(args, "-C", cfg.WorkDir)
		}

		// MCP configuration via -c flag with TOML syntax
		// Format: mcp_servers.perles-worker={url="http://localhost:PORT/worker/ID"}
		if cfg.MCPConfig != "" {
			args = append(args, "-c", cfg.MCPConfig)
		}

		// Prompt as final positional argument
		if cfg.Prompt != "" {
			args = append(args, cfg.Prompt)
		}
	}

	return args
}
