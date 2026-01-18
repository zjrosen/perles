package opencode

import (
	"encoding/json"

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/client"
)

const (
	// OpenCodeContextWindowSize is the context window size for OpenCode (uses Claude models).
	OpenCodeContextWindowSize = 200000
)

// Parser implements client.EventParser for OpenCode CLI JSON events.
// It embeds BaseParser for shared utilities and overrides methods as needed.
type Parser struct {
	client.BaseParser
}

// NewParser creates a new OpenCode EventParser with the default context window size.
func NewParser() *Parser {
	return &Parser{
		BaseParser: client.NewBaseParser(OpenCodeContextWindowSize),
	}
}

// ParseEvent converts OpenCode CLI JSON to client.OutputEvent.
// This is the main parsing entry point called for each stdout line.
// OpenCode uses camelCase JSON fields (e.g., sessionID not session_id).
func (p *Parser) ParseEvent(data []byte) (client.OutputEvent, error) {
	var raw opencodeEvent
	if err := json.Unmarshal(data, &raw); err != nil {
		return client.OutputEvent{}, err
	}

	event := client.OutputEvent{
		Type:      mapEventType(raw.Type),
		SessionID: raw.SessionID, // camelCase from JSON
	}

	// Copy raw data for debugging
	event.Raw = make([]byte, len(data))
	copy(event.Raw, data)

	// Handle error events
	if raw.Type == "error" {
		var message, code string
		if raw.Error != nil {
			// Check for nested API error structure first (e.g., {"name":"APIError","data":{"message":"..."}})
			if raw.Error.Data != nil && raw.Error.Data.Message != "" {
				message = raw.Error.Data.Message
				code = raw.Error.Name // Use error name as code (e.g., "APIError")
			} else {
				// Fall back to top-level error fields
				message = raw.Error.Message
				code = raw.Error.Code
			}
		} else if raw.Message != "" {
			message = raw.Message
		}
		event.Error = &client.ErrorInfo{
			Message: message,
			Code:    code,
		}

		// Log error event for context exhaustion discovery (Phase 2.5)
		// This helps capture real error patterns for implementing IsContextExhausted
		log.Debug(log.CatOrch, "opencode error event",
			"provider", "opencode",
			"code", code,
			"message", message,
			"raw", string(data),
		)

		// Check for context exhaustion patterns in error message
		if p.BaseParser.IsContextExhausted(event) {
			event.Error.Reason = client.ErrReasonContextExceeded
		}

		return event, nil
	}

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
				ID:   raw.Part.CallID, // camelCase from JSON
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
					TotalTokens:  p.ContextWindowSize(), // Use parser's context window instead of hardcoded
					OutputTokens: tokens.Output,
				}
			}
		}
	}

	return event, nil
}

// ExtractSessionRef returns the session identifier from an event.
// CRITICAL: OpenCode emits sessionID in ANY event type (not just init events).
// This is OpenCode's unique pattern - we must check sessionID on every event.
func (p *Parser) ExtractSessionRef(event client.OutputEvent, rawLine []byte) string {
	// OpenCode includes sessionID (camelCase) in most events
	// First check the parsed event
	if event.SessionID != "" {
		return event.SessionID
	}

	// Fall back to parsing raw line if needed (handles edge cases)
	if len(rawLine) > 0 {
		var raw struct {
			SessionID string `json:"sessionID"` //nolint:tagliatelle // matches actual OpenCode API
		}
		if err := json.Unmarshal(rawLine, &raw); err == nil && raw.SessionID != "" {
			return raw.SessionID
		}
	}

	return ""
}

// IsContextExhausted checks if an event indicates context window exhaustion.
// This delegates to BaseParser's detection which covers ErrReasonContextExceeded and message patterns.
func (p *Parser) IsContextExhausted(event client.OutputEvent) bool {
	return p.BaseParser.IsContextExhausted(event)
}

// Verify Parser implements EventParser at compile time.
var _ client.EventParser = (*Parser)(nil)
