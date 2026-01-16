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

// parseEvent parses a JSON line from OpenCode's --format json output into a client.OutputEvent.
func parseEvent(line []byte) (client.OutputEvent, error) {
	var raw opencodeEvent
	if err := json.Unmarshal(line, &raw); err != nil {
		return client.OutputEvent{}, err
	}

	event := client.OutputEvent{
		Type:      mapEventType(raw.Type),
		SessionID: raw.SessionID,
	}

	// Copy raw data for debugging
	event.Raw = make([]byte, len(line))
	copy(event.Raw, line)

	// Process part-based content
	if raw.Part != nil {
		// Handle text events
		if raw.Type == "text" && raw.Part.Text != "" {
			event.Message = &client.MessageContent{
				ID:   raw.Part.ID,
				Role: "assistant",
				Content: []client.ContentBlock{
					{Type: "text", Text: raw.Part.Text},
				},
			}
		}

		// Handle tool_use events
		if raw.Type == "tool_use" && raw.Part.Tool != "" {
			// Create tool content
			event.Tool = &client.ToolContent{
				ID:   raw.Part.CallID,
				Name: raw.Part.Tool,
			}
			if raw.Part.State != nil {
				event.Tool.Input = raw.Part.State.Input

				// Create a message with tool_use content block for consistency
				event.Message = &client.MessageContent{
					ID:   raw.Part.ID,
					Role: "assistant",
					Content: []client.ContentBlock{
						{
							Type:  "tool_use",
							ID:    raw.Part.CallID,
							Name:  raw.Part.Tool,
							Input: raw.Part.State.Input,
						},
					},
				}

				// If tool has output, it's a completed tool result
				if raw.Part.State.Output != "" {
					event.Result = raw.Part.State.Output
				}
			}
		}

		// Handle step_finish events
		if raw.Type == "step_finish" {
			// Set subtype based on reason
			if raw.Part.Reason != "" {
				event.SubType = raw.Part.Reason
			}

			// Extract token usage if available
			if raw.Part.Tokens != nil {
				tokens := raw.Part.Tokens
				cacheRead := 0
				if tokens.Cache != nil {
					cacheRead = tokens.Cache.Read
				}
				event.Usage = &client.UsageInfo{
					TokensUsed:   tokens.Input + cacheRead,
					TotalTokens:  200000, // Claude context window
					OutputTokens: tokens.Output,
				}
			}
		}
	}

	return event, nil
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
