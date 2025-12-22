// Package claude provides a Go interface to headless Claude Code sessions.
//
// This package enables spawning, managing, and communicating with Claude Code
// processes running in headless mode (--print --output-format stream-json).
// It parses the JSONL output stream into typed Go events for easy consumption.
//
// # Basic Usage
//
// Spawn a new Claude process:
//
//	cfg := claude.Config{
//		WorkDir:         "/path/to/project",
//		Prompt:          "Analyze the code in main.go",
//		SkipPermissions: true,
//	}
//	proc, err := claude.Spawn(context.Background(), cfg)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Read events
//	for event := range proc.Events() {
//		switch {
//		case event.IsInit():
//			fmt.Println("Session:", event.SessionID)
//		case event.IsAssistant():
//			fmt.Println("Response:", event.Message.GetText())
//		case event.IsResult():
//			fmt.Printf("Cost: $%.4f\n", event.TotalCostUSD)
//		}
//	}
//
//	// Wait for completion
//	proc.Wait()
//
// # Resuming Sessions
//
// Continue an existing session with a follow-up message:
//
//	proc, err := claude.Resume(sessionID, "Now refactor the function", workDir)
//
// Or with full config control:
//
//	proc, err := claude.ResumeWithConfig(sessionID, claude.Config{
//		WorkDir:            workDir,
//		Prompt:             "Add error handling",
//		Model:              "opus",
//		AppendSystemPrompt: "Focus on edge cases",
//	})
//
// # Event Types
//
// The stream-json format produces these event types:
//
//   - system (subtype: init): Session initialization with SessionID and CWD
//   - assistant: Claude's response with Message containing text and tool_use blocks
//   - tool_result: Tool execution results with Tool containing output
//   - result (subtype: success): Completion event with cost and token usage
//   - error: Error information
//
// # Tool Usage
//
// Tool calls are embedded in assistant messages as content blocks, not separate events.
// Use MessageContent.GetToolUses() or HasToolUses() to check for tool calls:
//
//	if event.IsAssistant() && event.Message.HasToolUses() {
//		for _, tool := range event.Message.GetToolUses() {
//			fmt.Printf("Tool: %s\n", tool.Name)
//		}
//	}
//
// Tool results are separate events with type "tool_result":
//
//	if event.IsToolResult() {
//		fmt.Printf("Result from %s: %s\n", event.Tool.Name, event.Tool.GetOutput())
//	}
//
// # Token Tracking
//
// Context window usage can be calculated from result events:
//
//	if event.IsResult() {
//		ctx := event.GetContextTokens()  // InputTokens + CacheReadInputTokens + CacheCreationInputTokens
//		out := event.Usage.OutputTokens  // cumulative output tokens
//	}
//
// GetContextTokens() returns the total input tokens per Anthropic's formula:
// total_input_tokens = cache_read_input_tokens + cache_creation_input_tokens + input_tokens
//   - InputTokens: Tokens sent fresh to the model (not from cache)
//   - CacheReadInputTokens: Tokens read from prompt cache
//   - CacheCreationInputTokens: Tokens written to cache (also in context)
//
// OutputTokens should be accumulated across events for total session output.
//
// # Known Quirks
//
// These behaviors were discovered during development and testing:
//
//  1. Command line separator: The prompt MUST be preceded by "--" when using
//     --disallowed-tools, otherwise the prompt gets consumed by the flag.
//     This is handled automatically by buildArgs().
//
//  2. No stdin pipe: In --print mode, don't create a stdin pipe as it can cause
//     the process to wait for input unexpectedly.
//
//  3. Content is an array: Message content is []ContentBlock, not a string.
//     Use GetText() to extract concatenated text.
//
//  4. Tool uses are embedded: Tool calls appear as content blocks inside
//     assistant messages with type "tool_use", not as separate events.
//
//  5. Working directory: The process runs in Config.WorkDir. Ensure this is
//     the project root, not a subdirectory.
//
//  6. Context window calculation: GetContextTokens() returns InputTokens +
//     CacheReadInputTokens + CacheCreationInputTokens. OutputTokens should be accumulated.
//
// # Error Handling
//
// Errors are reported through the Errors() channel and process status:
//
//	select {
//	case event := <-proc.Events():
//		// handle event
//	case err := <-proc.Errors():
//		if errors.Is(err, claude.ErrTimeout) {
//			// handle timeout
//		}
//	}
//
// Check process status with Status() or IsRunning(). Cancel with Cancel().
package claude
