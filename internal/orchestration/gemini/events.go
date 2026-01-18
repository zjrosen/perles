package gemini

import (
	"encoding/json"
	stdlog "log"

	"github.com/zjrosen/perles/internal/orchestration/client"
)

// geminiEvent represents the raw Gemini CLI stream-json event structure.
// This is used for parsing Gemini-specific fields before converting to client.OutputEvent.
//
// Gemini CLI uses a different event format than Claude/Amp/Codex:
//   - init: Session initialization with session_id and model
//   - message: Assistant or user messages with role-based discrimination
//   - tool_use: Tool invocation with name and parameters
//   - tool_result: Tool execution result with status and output
//   - result: Session completion with usage statistics
//   - error: Error events with message and optional code
type geminiEvent struct {
	// Type is the event type (e.g., "init", "message", "tool_use", "result")
	Type string `json:"type"`

	// SessionID is the session identifier (present in init events)
	SessionID string `json:"session_id,omitempty"`

	// Model is the model name (present in init events)
	Model string `json:"model,omitempty"`

	// Timestamp is the event timestamp
	Timestamp string `json:"timestamp,omitempty"`

	// Role is the message role: "assistant" or "user" (for message events)
	Role string `json:"role,omitempty"`

	// Content is the message content (for message events)
	Content string `json:"content,omitempty"`

	// Delta indicates this is a streaming chunk (should be accumulated with previous message)
	Delta bool `json:"delta,omitempty"`

	// Tool fields (Gemini format uses top-level fields)
	ToolName   string          `json:"tool_name,omitempty"`
	ToolID     string          `json:"tool_id,omitempty"`
	Parameters json.RawMessage `json:"parameters,omitempty"`
	Status     string          `json:"status,omitempty"`
	Output     string          `json:"output,omitempty"`

	// Stats contains token usage information (present in result events)
	Stats *geminiStats `json:"stats,omitempty"`

	// Error contains error details (for error events)
	Error *geminiError `json:"error,omitempty"`
}

// geminiStats represents token usage from Gemini's result events.
type geminiStats struct {
	// TokensPrompt is the number of input/prompt tokens
	TokensPrompt int `json:"tokens_prompt,omitempty"`

	// TokensCandidates is the number of output/candidate tokens
	TokensCandidates int `json:"tokens_candidates,omitempty"`

	// TokensCached is the number of cached tokens
	TokensCached int `json:"tokens_cached,omitempty"`

	// DurationMs is the total execution duration in milliseconds
	DurationMs int64 `json:"duration_ms,omitempty"`
}

// geminiError represents error information in Gemini events.
type geminiError struct {
	// Message is the error message
	Message string `json:"message,omitempty"`

	// Code is the optional error code
	Code string `json:"code,omitempty"`
}

// mapEventType maps Gemini event type and role strings to client.EventType.
// Role is used to discriminate message events (assistant vs user).
func mapEventType(geminiType, role string) client.EventType {
	switch geminiType {
	case "init":
		return client.EventSystem
	case "message":
		if role == "assistant" {
			return client.EventAssistant
		}
		// User messages don't have a direct mapping, treat as tool result
		// (typically used for injected prompts or tool results)
		return client.EventToolResult
	case "tool_use":
		return client.EventToolUse
	case "tool_result":
		return client.EventToolResult
	case "result":
		return client.EventResult
	case "error":
		return client.EventError
	default:
		// Log warning for unknown event types
		stdlog.Printf("gemini: unknown event type: %q", geminiType)
		// Pass through as-is for forward compatibility
		return client.EventType(geminiType)
	}
}
