package amp

import (
	"encoding/json"
	"strings"

	"github.com/zjrosen/perles/internal/orchestration/client"
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

	// Error info - can be string or object, use json.RawMessage for polymorphic parsing
	Error json.RawMessage `json:"error,omitempty"`
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
		Error:         parseErrorField(raw.Error),
	}
	// Note: Raw is set by BaseProcess.parseOutput() after calling this function

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
			tokensUsed := raw.Message.Usage.InputTokens + raw.Message.Usage.CacheReadInputTokens + raw.Message.Usage.CacheCreationInputTokens
			event.Usage = &client.UsageInfo{
				TokensUsed:   tokensUsed,
				TotalTokens:  200000, // Default context window for all current models
				OutputTokens: raw.Message.Usage.OutputTokens,
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

	// Detect context exhaustion patterns:
	// 1. Result event with is_error=true and result containing context-related messages
	// 2. Error event with message containing context-related messages
	// Common patterns: "Prompt is too long", "Context window exceeded", "context limit"
	if event.IsErrorResult || event.Type == client.EventError {
		errorMsg := event.Result
		if event.Error != nil && event.Error.Message != "" {
			errorMsg = event.Error.Message
		}
		if isContextExceededMessage(errorMsg) {
			if event.Error == nil {
				event.Error = &client.ErrorInfo{}
			}
			event.Error.Reason = client.ErrReasonContextExceeded
			if event.Error.Message == "" {
				event.Error.Message = errorMsg
			}
		}
	}

	return event, nil
}

// parseErrorField handles the polymorphic error field from Amp CLI.
// It can be either a string (error message) or an object (ErrorInfo).
// String format example: "413 {\"type\":\"error\",\"error\":{\"type\":\"invalid_request_error\",\"message\":\"Prompt is too long\"}}"
func parseErrorField(raw json.RawMessage) *client.ErrorInfo {
	if len(raw) == 0 {
		return nil
	}

	// Try parsing as object first
	var errInfo client.ErrorInfo
	if err := json.Unmarshal(raw, &errInfo); err == nil && (errInfo.Message != "" || errInfo.Code != "") {
		return &errInfo
	}

	// Try parsing as string (error message like "413 {...}")
	var errStr string
	if err := json.Unmarshal(raw, &errStr); err == nil && errStr != "" {
		// Extract the actual error message from the string
		// Common format: "413 {\"type\":\"error\",\"error\":{\"type\":\"invalid_request_error\",\"message\":\"Prompt is too long\"}}"
		return parseErrorString(errStr)
	}

	return nil
}

// parseErrorString extracts error information from an error string.
// Handles formats like: "413 {\"type\":\"error\",\"error\":{...}}"
func parseErrorString(errStr string) *client.ErrorInfo {
	// Try to find embedded JSON in the string
	if idx := strings.Index(errStr, "{"); idx >= 0 {
		jsonPart := errStr[idx:]
		// Try parsing as nested error object: {"type":"error","error":{...}}
		var nested struct {
			Type  string `json:"type"`
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(jsonPart), &nested); err == nil && nested.Error.Message != "" {
			return &client.ErrorInfo{
				Message: nested.Error.Message,
				Code:    nested.Error.Type,
			}
		}
	}

	// Fall back to using the entire string as the message
	return &client.ErrorInfo{
		Message: errStr,
	}
}

// isContextExceededMessage checks if an error message indicates context window exhaustion.
func isContextExceededMessage(msg string) bool {
	if msg == "" {
		return false
	}
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "prompt is too long") ||
		strings.Contains(lower, "context window exceeded") ||
		strings.Contains(lower, "context exceeded") ||
		strings.Contains(lower, "context limit") ||
		strings.Contains(lower, "token limit")
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
