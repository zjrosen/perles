package codex

import (
	"encoding/json"

	"github.com/zjrosen/perles/internal/orchestration/client"
)

// codexEvent represents the raw Codex CLI JSONL event structure.
// This is used for parsing Codex-specific fields before converting to client.OutputEvent.
//
// Codex CLI uses a different event format than Claude Code or Amp:
//   - thread.started: Thread initialization with thread_id
//   - turn.started: Beginning of a turn (user message to assistant response)
//   - turn.completed: End of turn with usage stats
//   - turn.failed: Turn failure with error details
//   - item.started/item.updated/item.completed: Thread item lifecycle
//   - error: Unrecoverable stream error
type codexEvent struct {
	// Type is the event type (e.g., "thread.started", "item.completed", "turn.completed")
	Type string `json:"type"`

	// ThreadID is the thread identifier (present in thread.started events)
	ThreadID string `json:"thread_id,omitempty"`

	// Item contains the item data for item.* events
	Item *codexItem `json:"item,omitempty"`

	// Usage contains token usage information (present in turn.completed events)
	Usage *codexUsage `json:"usage,omitempty"`

	// Error contains error details for turn.failed events.
	// Can be a string or object with message field.
	Error *codexError `json:"error,omitempty"`

	// Message contains error message for error events (top-level message field).
	// Format: {"type":"error","message":"..."}
	Message string `json:"message,omitempty"`
}

// codexItem represents an item in a Codex thread.
// Item types include:
//   - agent_message: Assistant text response
//   - reasoning: Summary of assistant's thinking (internal, not exposed)
//   - command_execution: Shell command execution
//   - file_change: File modification
//   - mcp_tool_call: MCP tool invocation
//   - web_search: Web search operation
//   - todo_list: Agent's running plan
type codexItem struct {
	// ID is the unique item identifier
	ID string `json:"id,omitempty"`

	// Type is the item type (e.g., "agent_message", "command_execution")
	Type string `json:"type,omitempty"`

	// Text is the message content (for agent_message items)
	Text string `json:"text,omitempty"`

	// Command is the shell command (for command_execution items)
	Command string `json:"command,omitempty"`

	// AggregatedOutput is the command output (for completed command_execution items)
	AggregatedOutput string `json:"aggregated_output,omitempty"`

	// ExitCode is the command exit code (for completed command_execution items)
	ExitCode *int `json:"exit_code,omitempty"`

	// Status is the item status (e.g., "in_progress", "completed")
	Status string `json:"status,omitempty"`

	// Server is the MCP server name (for mcp_tool_call items)
	Server string `json:"server,omitempty"`

	// Tool is the name of the MCP tool (for mcp_tool_call items)
	Tool string `json:"tool,omitempty"`

	// Arguments is the input to the MCP tool (for mcp_tool_call items)
	Arguments json.RawMessage `json:"arguments,omitempty"`

	// Result is the output from the MCP tool (for completed mcp_tool_call items)
	Result *codexToolResult `json:"result,omitempty"`
}

// codexToolResult represents the result of an MCP tool call.
type codexToolResult struct {
	Content []codexToolResultContent `json:"content,omitempty"`
}

// codexToolResultContent represents a content item in an MCP tool result.
type codexToolResultContent struct {
	Type string `json:"type,omitempty"`
	Text string `json:"text,omitempty"`
}

// codexUsage represents token usage from Codex's turn.completed events.
// Maps to client.UsageInfo with cached_input_tokens -> CacheReadInputTokens.
type codexUsage struct {
	InputTokens       int `json:"input_tokens,omitempty"`
	CachedInputTokens int `json:"cached_input_tokens,omitempty"`
	OutputTokens      int `json:"output_tokens,omitempty"`
}

// codexError represents the error field in turn.failed events.
// The error field can be either a string or an object with a message field.
type codexError struct {
	Message string `json:"message,omitempty"`
}

// UnmarshalJSON handles both string and object formats for the error field.
func (e *codexError) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as string first
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		e.Message = s
		return nil
	}

	// Try to unmarshal as object with message field
	var obj struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	e.Message = obj.Message
	return nil
}

// mapEventType maps Codex event type strings to client.EventType.
// Item events require the item to determine the specific event type.
func mapEventType(codexType string, item *codexItem) client.EventType {
	switch codexType {
	case "thread.started":
		return client.EventSystem
	case "turn.completed":
		return client.EventResult
	case "turn.started":
		// turn.started doesn't have a direct mapping, treat as system event
		return client.EventSystem
	case "turn.failed", "error":
		return client.EventError
	case "item.started":
		if item != nil && item.Type == "command_execution" {
			return client.EventToolUse
		}
		if item != nil && item.Type == "mcp_tool_call" {
			return client.EventToolUse
		}
		// Default for other item.started events
		return client.EventAssistant
	case "item.updated":
		// item.updated events are intermediate, treat based on item type
		if item != nil && item.Type == "agent_message" {
			return client.EventAssistant
		}
		return client.EventAssistant
	case "item.completed":
		if item != nil {
			switch item.Type {
			case "agent_message":
				return client.EventAssistant
			case "command_execution":
				return client.EventToolResult
			case "mcp_tool_call":
				return client.EventToolResult
			}
		}
		return client.EventAssistant
	default:
		// Unknown event type, return as-is
		return client.EventType(codexType)
	}
}

// mapItemEvent populates event fields based on item data.
func mapItemEvent(raw codexEvent, event client.OutputEvent) client.OutputEvent {
	item := raw.Item

	switch item.Type {
	case "agent_message":
		// Map agent_message to EventAssistant with text content
		event.Message = &client.MessageContent{
			ID:   item.ID,
			Role: "assistant",
			Content: []client.ContentBlock{
				{
					Type: "text",
					Text: item.Text,
				},
			},
		}

	case "command_execution":
		switch raw.Type {
		case "item.started":
			// item.started for command_execution -> EventToolUse
			// Populate both Tool (for compatibility) and Message.Content (for display)
			input := marshalToolInput(map[string]string{"command": item.Command})
			event.Tool = &client.ToolContent{
				ID:    item.ID,
				Name:  "Bash",
				Input: input,
			}
			event.Message = &client.MessageContent{
				ID:   item.ID,
				Role: "assistant",
				Content: []client.ContentBlock{
					{
						Type:  "tool_use",
						ID:    item.ID,
						Name:  "Bash",
						Input: input,
					},
				},
			}
		case "item.completed":
			// item.completed for command_execution -> EventToolResult
			event.Tool = &client.ToolContent{
				ID:     item.ID,
				Name:   "Bash",
				Output: item.AggregatedOutput,
			}
			// Include exit code in output if non-zero
			if item.ExitCode != nil && *item.ExitCode != 0 {
				event.IsErrorResult = true
			}
		}

	case "mcp_tool_call":
		switch raw.Type {
		case "item.started":
			// item.started for mcp_tool_call -> EventToolUse
			// Populate both Tool (for compatibility) and Message.Content (for display)
			event.Tool = &client.ToolContent{
				ID:    item.ID,
				Name:  item.Tool,
				Input: item.Arguments,
			}
			event.Message = &client.MessageContent{
				ID:   item.ID,
				Role: "assistant",
				Content: []client.ContentBlock{
					{
						Type:  "tool_use",
						ID:    item.ID,
						Name:  item.Tool,
						Input: item.Arguments,
					},
				},
			}
		case "item.completed":
			// item.completed for mcp_tool_call -> EventToolResult
			// Extract output text from result.content array
			var output string
			if item.Result != nil && len(item.Result.Content) > 0 {
				for _, c := range item.Result.Content {
					if c.Type == "text" && c.Text != "" {
						output = c.Text
						break
					}
				}
			}
			event.Tool = &client.ToolContent{
				ID:     item.ID,
				Name:   item.Tool,
				Output: output,
			}
		}

	case "reasoning":
		// TODO: Reasoning events are intentionally ignored.
		// They represent internal chain-of-thought summaries that should not
		// be exposed directly to users. If needed in the future, consider
		// adding a dedicated EventType for internal/debug events.

	case "file_change", "web_search", "todo_list":
		// These item types are informational and don't have direct mappings
		// to the client event types. They're preserved in Raw for debugging.
	}

	return event
}

// marshalToolInput converts a map to JSON for tool input.
func marshalToolInput(input map[string]string) json.RawMessage {
	data, err := json.Marshal(input)
	if err != nil {
		return nil
	}
	return data
}
