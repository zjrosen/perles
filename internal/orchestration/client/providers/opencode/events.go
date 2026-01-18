package opencode

import (
	"encoding/json"

	"github.com/zjrosen/perles/internal/orchestration/client"
)

// opencodeEvent represents the raw OpenCode JSON output structure.
// OpenCode's --format json outputs JSONL with events of various types.
// Format observed from actual opencode run output:
//
//	{"type":"text","timestamp":...,"sessionID":"ses_...","part":{"type":"text","text":"..."}}
//	{"type":"tool_use","timestamp":...,"sessionID":"ses_...","part":{"type":"tool","tool":"bash","state":{...}}}
//	{"type":"step_start","timestamp":...,"sessionID":"ses_...","part":{"type":"step-start"}}
//	{"type":"step_finish","timestamp":...,"sessionID":"ses_...","part":{"type":"step-finish","tokens":{...}}}
type opencodeEvent struct {
	Type      string        `json:"type"`
	Timestamp int64         `json:"timestamp,omitempty"`
	SessionID string        `json:"sessionID,omitempty"` //nolint:tagliatelle // matches actual OpenCode API
	Part      *opencodePart `json:"part,omitempty"`
	Error     *opcodeError  `json:"error,omitempty"`
	Message   string        `json:"message,omitempty"` // Top-level message for some error formats
}

// opencodePart represents the "part" object in OpenCode events.
type opencodePart struct {
	ID        string `json:"id,omitempty"`
	SessionID string `json:"sessionID,omitempty"` //nolint:tagliatelle // matches actual OpenCode API
	MessageID string `json:"messageID,omitempty"` //nolint:tagliatelle // matches actual OpenCode API
	Type      string `json:"type,omitempty"`      // "text", "tool", "step-start", "step-finish"

	// Text event fields
	Text string `json:"text,omitempty"`

	// Tool event fields
	CallID string           `json:"callID,omitempty"` //nolint:tagliatelle // matches actual OpenCode API
	Tool   string           `json:"tool,omitempty"`
	State  *opcodeToolState `json:"state,omitempty"`

	// Step finish fields
	Reason   string        `json:"reason,omitempty"` // "tool-calls", "stop"
	Snapshot string        `json:"snapshot,omitempty"`
	Cost     float64       `json:"cost,omitempty"`
	Tokens   *opcodeTokens `json:"tokens,omitempty"`

	// Time fields
	Time *opcodeTime `json:"time,omitempty"`
}

// opcodeToolState represents tool execution state.
type opcodeToolState struct {
	Status   string          `json:"status,omitempty"` // "completed", "running", etc.
	Input    json.RawMessage `json:"input,omitempty"`
	Output   string          `json:"output,omitempty"`
	Title    string          `json:"title,omitempty"`
	Metadata *opcodeMetadata `json:"metadata,omitempty"`
	Time     *opcodeTime     `json:"time,omitempty"`
}

// opcodeMetadata represents tool metadata.
type opcodeMetadata struct {
	Output      string `json:"output,omitempty"`
	Exit        int    `json:"exit,omitempty"`
	Description string `json:"description,omitempty"`
	Truncated   bool   `json:"truncated,omitempty"`
}

// opcodeTokens represents token usage in step_finish events.
type opcodeTokens struct {
	Input     int          `json:"input,omitempty"`
	Output    int          `json:"output,omitempty"`
	Reasoning int          `json:"reasoning,omitempty"`
	Cache     *opcodeCache `json:"cache,omitempty"`
}

// opcodeCache represents cache token info.
type opcodeCache struct {
	Read  int `json:"read,omitempty"`
	Write int `json:"write,omitempty"`
}

// opcodeTime represents time info.
type opcodeTime struct {
	Start int64 `json:"start,omitempty"`
	End   int64 `json:"end,omitempty"`
}

// opcodeError represents error information in OpenCode events.
// OpenCode returns nested API errors with structure: {"name":"APIError","data":{"message":"...",...}}
type opcodeError struct {
	// Top-level error fields (for simpler error formats)
	Message string `json:"message,omitempty"`
	Code    string `json:"code,omitempty"`

	// Nested error structure (for API errors like context exceeded)
	Name string           `json:"name,omitempty"` // e.g., "APIError"
	Data *opcodeErrorData `json:"data,omitempty"`
}

// opcodeErrorData represents nested error data in API errors.
type opcodeErrorData struct {
	Message     string `json:"message,omitempty"`     // e.g., "prompt is too long: 200561 tokens > 200000 maximum"
	StatusCode  int    `json:"statusCode,omitempty"`  //nolint:tagliatelle // matches actual OpenCode API
	IsRetryable bool   `json:"isRetryable,omitempty"` //nolint:tagliatelle // matches actual OpenCode API
}

// mapEventType maps OpenCode event type strings to client.EventType.
func mapEventType(opencodeType string) client.EventType {
	switch opencodeType {
	case "system", "init":
		return client.EventSystem
	case "text":
		return client.EventAssistant
	case "tool_use":
		return client.EventToolUse
	case "step_start":
		return client.EventType("step_start")
	case "step_finish":
		return client.EventType("step_finish")
	case "result":
		return client.EventResult
	case "error":
		return client.EventError
	default:
		// Pass through unknown types as-is for forward compatibility
		return client.EventType(opencodeType)
	}
}
