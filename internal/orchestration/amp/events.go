package amp

import (
	"encoding/json"

	"perles/internal/orchestration/client"
)

// ampEvent represents the raw Amp stream-json event structure.
// This is used for parsing Amp-specific fields before converting to client.OutputEvent.
type ampEvent struct {
	// Common fields - matches client.OutputEvent
	Type      string `json:"type"`
	SubType   string `json:"subtype,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	WorkDir   string `json:"cwd,omitempty"`

	// Message content
	Message *ampMessage `json:"message,omitempty"`

	// Result event fields
	Result       string  `json:"result,omitempty"`
	DurationMs   int64   `json:"duration_ms,omitempty"`
	IsError      bool    `json:"is_error,omitempty"`
	NumTurns     int     `json:"num_turns,omitempty"`
	TotalCostUSD float64 `json:"total_cost_usd,omitempty"`

	// Error info
	Error *client.ErrorInfo `json:"error,omitempty"`
}

// ampMessage represents message content in Amp events.
// Amp includes usage info directly in the message, unlike Claude which puts it in result events.
type ampMessage struct {
	ID         string                `json:"id,omitempty"`
	Type       string                `json:"type,omitempty"`
	Role       string                `json:"role,omitempty"`
	Content    []client.ContentBlock `json:"content,omitempty"`
	Model      string                `json:"model,omitempty"`
	StopReason string                `json:"stop_reason,omitempty"`
	Usage      *ampUsage             `json:"usage,omitempty"`
}

// ampUsage represents token usage from Amp's stream-json output.
// Amp includes max_tokens and service_tier which we don't need.
type ampUsage struct {
	InputTokens              int `json:"input_tokens,omitempty"`
	OutputTokens             int `json:"output_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	MaxTokens                int `json:"max_tokens,omitempty"`
}

// parseEvent parses a JSON line from Amp's stream-json output into a client.OutputEvent.
func parseEvent(line []byte) (client.OutputEvent, error) {
	var raw ampEvent
	if err := json.Unmarshal(line, &raw); err != nil {
		return client.OutputEvent{}, err
	}

	event := client.OutputEvent{
		Type:          mapEventType(raw.Type),
		SubType:       raw.SubType,
		SessionID:     raw.SessionID,
		WorkDir:       raw.WorkDir,
		Result:        raw.Result,
		DurationMs:    raw.DurationMs,
		IsErrorResult: raw.IsError,
		TotalCostUSD:  raw.TotalCostUSD,
		Error:         raw.Error,
	}

	// Copy raw data for debugging
	event.Raw = make([]byte, len(line))
	copy(event.Raw, line)

	// Convert message content
	if raw.Message != nil {
		event.Message = &client.MessageContent{
			ID:      raw.Message.ID,
			Role:    raw.Message.Role,
			Content: raw.Message.Content,
			Model:   raw.Message.Model,
		}

		// Extract usage info from message (Amp puts it here, not in result events)
		if raw.Message.Usage != nil {
			event.Usage = &client.UsageInfo{
				InputTokens:              raw.Message.Usage.InputTokens,
				OutputTokens:             raw.Message.Usage.OutputTokens,
				CacheReadInputTokens:     raw.Message.Usage.CacheReadInputTokens,
				CacheCreationInputTokens: raw.Message.Usage.CacheCreationInputTokens,
			}
		}

		// Extract tool content from tool_use messages
		for _, block := range raw.Message.Content {
			if block.Type == "tool_use" {
				event.Tool = &client.ToolContent{
					ID:    block.ID,
					Name:  block.Name,
					Input: block.Input,
				}
				break
			}
		}
	}

	return event, nil
}

// mapEventType maps Amp event type strings to client.EventType.
// Amp uses the same type names as Claude Code for --stream-json compatibility.
func mapEventType(ampType string) client.EventType {
	switch ampType {
	case "system":
		return client.EventSystem
	case "assistant":
		return client.EventAssistant
	case "user":
		// Amp uses "user" for both user messages and tool results
		// The tool_result content type distinguishes them
		return client.EventToolResult
	case "result":
		return client.EventResult
	case "error":
		return client.EventError
	default:
		return client.EventType(ampType)
	}
}
