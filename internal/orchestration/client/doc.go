// Package client provides interfaces for headless AI client abstraction.
//
// This package defines provider-agnostic interfaces for spawning and managing
// headless AI processes. It enables the orchestration layer to work with
// multiple AI providers (Claude, Amp, etc.) through a unified interface.
//
// Key interfaces:
//   - HeadlessClient: Factory for spawning headless processes
//   - HeadlessProcess: Unified process lifecycle management
//   - OutputEvent: Normalized event stream from processes
//   - Config: Provider-agnostic configuration
//
// Example usage:
//
//	// Create a client for a specific provider
//	client, err := client.NewClient(client.ClientClaude)
//	if err != nil {
//	    return err
//	}
//
//	// Spawn a new process
//	cfg := client.Config{
//	    WorkDir: "/path/to/work",
//	    Prompt:  "Hello, world!",
//	}
//	process, err := client.Spawn(ctx, cfg)
//	if err != nil {
//	    return err
//	}
//
//	// Consume events
//	for event := range process.Events() {
//	    if event.IsAssistant() {
//	        fmt.Println(event.Message.GetText())
//	    }
//	}
package client
