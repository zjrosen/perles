// Package opencode provides a Go interface to headless OpenCode CLI sessions.
//
// OpenCode is an agentic coding assistant that supports non-interactive
// execution for AI-assisted coding tasks. This package implements the
// client.HeadlessClient interface to enable OpenCode as a provider in the
// orchestration system.
//
// # Usage
//
// Import this package to register the OpenCode client with the client registry:
//
//	import _ "github.com/zjrosen/perles/internal/orchestration/client/providers/opencode"
//
// Then create a client using the registry:
//
//	client, err := client.NewClient(client.ClientOpenCode)
//
// # CLI Requirements
//
// The "opencode" command must be available in PATH. Install from:
// https://github.com/anomalyco/opencode
//
// # Headless Mode
//
// OpenCode runs in headless mode using the run command with JSON output:
//
//	opencode run --format json --model anthropic/claude-opus-4-5 -- "your prompt here"
//
// Key flags:
//   - --format json: Structured JSON output for parsing
//   - --model: Model selection (e.g., anthropic/claude-opus-4-5)
//   - --session: Resume existing session by ID
//   - --: Separator before prompt argument
//
// # System Prompt
//
// OpenCode doesn't have a dedicated --append-system-prompt flag. System prompts
// are prepended to the main prompt with a separator:
//
//	<system prompt content>
//
//	<user prompt>
//
// # MCP Configuration
//
// OpenCode supports MCP server configuration via the OPENCODE_CONFIG_CONTENT
// environment variable. This provides process isolation - each spawned process
// gets its own MCP config without file conflicts.
//
// The orchestration system passes MCP configuration at spawn time using the
// {"mcp": {...}} format via this env var, ensuring each coordinator/worker
// process only sees its designated MCP server.
package opencode
