// Package codex provides a Go interface to headless Codex CLI sessions.
//
// Codex is the OpenAI Codex CLI agent that supports non-interactive execution
// for AI-assisted coding tasks. This package implements the client.HeadlessClient
// interface to enable Codex as a provider in the orchestration system.
//
// # Usage
//
// Import this package to register the Codex client with the client registry:
//
//	import _ "github.com/zjrosen/perles/internal/orchestration/client/providers/codex"
//
// Then create a client using the registry:
//
//	client, err := client.NewClient(client.ClientCodex)
//
// # CLI Requirements
//
// The "codex" command must be available in PATH. Install from https://github.com/openai/codex
//
// # Headless Mode
//
// Codex runs in headless mode using the exec subcommand with JSON output:
//
//	codex exec --json -s danger-full-access "your prompt here"
//
// The exec subcommand is required for non-interactive operation. Key flags:
//   - --json: Outputs JSONL events for parsing
//   - -s, --sandbox: Sandbox mode (read-only, workspace-write, danger-full-access)
//   - -C: Working directory specification
//   - -m: Model selection
//
// # Session Resumption
//
// Codex uses thread IDs (UUIDs) for session management. Sessions can be resumed:
//
//	codex exec resume <thread-id> --json -s danger-full-access
//
// Or resume the most recent session:
//
//	codex exec resume --last --json
//
// # Event Format Differences
//
// Codex uses a different JSONL event format from Claude and Amp:
//
//	Event Type              | Description                      | Maps To
//	------------------------|----------------------------------|------------------
//	thread.started          | Session initialization           | EventSystem (init)
//	turn.started            | Turn begins (internal)           | (ignored)
//	item.completed          | Message/reasoning/command done   | EventAssistant/EventToolResult
//	item.started            | Command execution started        | EventToolUse
//	turn.completed          | Turn ends with usage stats       | EventResult
//
// Unlike Claude's flat event structure, Codex uses nested item objects with a
// type discriminator (item.type) to differentiate between:
//   - "reasoning": Internal model thinking (currently ignored)
//   - "agent_message": Assistant response text
//   - "command_execution": Tool/command execution
//
// # MCP Configuration
//
// Codex supports MCP server configuration via the -c flag with TOML-like syntax:
//
//	codex exec --json -c 'mcp_servers.server-name={url="http://localhost:8080/path"}' "prompt"
//
// This differs from Claude/Amp which use --mcp-config with JSON format.
// The TOML syntax allows inline server definitions without separate config files.
//
// Example MCP configuration for orchestration:
//
//	-c 'mcp_servers.perles-worker={url="http://localhost:9000/worker/worker-1"}'
//
// # Sandbox Mode Options
//
// Codex provides fine-grained permission control via sandbox modes:
//
//   - read-only: Can only read files, no writes
//   - workspace-write: Can write within workspace directory
//   - danger-full-access: Full system access (equivalent to SkipPermissions)
//
// When SkipPermissions is true, this maps to --dangerously-bypass-approvals-and-sandbox
// which grants maximum permissions.
//
// # Known Limitations
//
//   - No system prompt flag: Unlike Claude's --append-system-prompt, Codex has
//     no dedicated flag. System prompts must be prepended to the main prompt.
//   - Reasoning events ignored: Codex emits item.type="reasoning" events for
//     internal model thinking. These are intentionally not mapped to client
//     events as Claude/Amp don't emit equivalent events.
//   - AllowedTools/DisallowedTools not supported: Codex exec mode doesn't
//     support tool filtering.
//   - Token usage differs: Codex reports cached_input_tokens as a single field,
//     mapped to CacheReadInputTokens (CacheCreationInputTokens is always 0).
package codex
