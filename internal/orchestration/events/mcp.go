package events

import "time"

// MCPEventType identifies the kind of MCP event.
type MCPEventType string

const (
	// MCPToolCall is emitted when an MCP tool is invoked.
	MCPToolCall MCPEventType = "tool_call"
	// MCPToolResult is emitted when an MCP tool returns successfully.
	MCPToolResult MCPEventType = "tool_result"
	// MCPError is emitted when an MCP tool call fails.
	MCPError MCPEventType = "error"
)

// MCPEvent represents an event from the MCP server capturing tool call
// request/response data for session logging.
type MCPEvent struct {
	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`
	// Type identifies the kind of MCP event.
	Type MCPEventType `json:"type"`
	// Method is the JSON-RPC method (e.g., "tools/call").
	Method string `json:"method"`
	// ToolName is the name of the tool that was called.
	ToolName string `json:"tool_name"`
	// WorkerID identifies which worker made the call (empty for coordinator).
	WorkerID string `json:"worker_id,omitempty"`
	// RequestJSON contains the serialized request parameters.
	RequestJSON []byte `json:"request_json,omitempty"`
	// ResponseJSON contains the serialized response.
	ResponseJSON []byte `json:"response_json,omitempty"`
	// Error contains the error message if the call failed.
	Error string `json:"error,omitempty"`
	// Duration is the time taken to execute the tool call.
	Duration time.Duration `json:"duration"`
}
