// Package gemini provides a Go interface to headless Gemini CLI sessions.
//
// Gemini CLI is Google's agentic coding assistant that supports non-interactive
// execution for AI-assisted coding tasks. This package implements the
// client.HeadlessClient interface to enable Gemini as a provider in the
// orchestration system.
//
// # Usage
//
// Import this package to register the Gemini client with the client registry:
//
//	import _ "github.com/zjrosen/perles/internal/orchestration/client/providers/gemini"
//
// Then create a client using the registry:
//
//	client, err := client.NewClient(client.ClientGemini)
//
// # CLI Requirements
//
// The "gemini" command must be available in PATH. Install from:
// https://github.com/google-gemini/gemini-cli
//
// # Headless Mode
//
// Gemini runs in headless mode using the sandbox command with structured output:
//
//	gemini sandbox -m gemini-2.5-pro "your prompt here"
//
// Key flags:
//   - -m: Model selection (e.g., gemini-2.5-pro, gemini-2.5-flash)
//   - -y: Auto-approve mode (equivalent to SkipPermissions)
//   - --dir: Working directory specification
//
// # Event Format
//
// Gemini uses a different event structure from Claude/Amp/Codex. Events are
// parsed from the JSONL output stream and mapped to unified OutputEvent types:
//
//	Gemini Event      | Maps To
//	------------------|------------------
//	status            | EventSystem (init)
//	text_delta        | EventAssistant
//	tool_use          | EventToolUse
//	tool_result       | EventToolResult
//	usage             | (token tracking)
//	done              | EventResult
//
// # System Prompt
//
// Gemini doesn't have a dedicated --system-prompt flag. System prompts are
// prepended to the main prompt with a separator:
//
//	[System Instructions]
//	<system prompt content>
//
//	[User Request]
//	<user prompt>
//
// # MCP Configuration
//
// Gemini supports MCP server configuration via settings.json with merge-safe
// handling. The orchestration system generates appropriate configuration at
// spawn time.
//
// # Authentication
//
// Gemini CLI uses Google Cloud authentication. Ensure credentials are configured
// via `gcloud auth login` or service account credentials before use.
package gemini
