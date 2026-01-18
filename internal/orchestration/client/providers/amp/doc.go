// Package amp provides a Go interface to headless Amp sessions.
//
// Amp is an AI coding assistant CLI that supports Claude Code-compatible
// stream-json output format. This package implements the client.HeadlessClient
// interface to enable Amp as a provider in the orchestration system.
//
// # Usage
//
// Import this package to register the Amp client with the client registry:
//
//	import _ "github.com/zjrosen/perles/internal/orchestration/client/providers/amp"
//
// Then create a client using the registry:
//
//	client, err := client.NewClient(client.ClientAmp)
//
// # CLI Requirements
//
// The "amp" command must be available in PATH. Install from https://ampcode.com/
//
// # Headless Mode
//
// Amp runs in headless mode using execute mode with stream-json output:
//
//	echo "prompt" | amp -x --stream-json --dangerously-allow-all
//
// # Session Management
//
// Amp uses "threads" for session persistence. Thread IDs have the format T-<uuid>.
// Sessions can be resumed using:
//
//	echo "follow-up" | amp threads continue <thread-id> -x --stream-json
//
// # Event Compatibility
//
// Amp's --stream-json flag produces Claude Code-compatible JSON Lines output.
// Event types are mapped to client.OutputEvent automatically.
package amp
