package gemini

import (
	"encoding/json"

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/client"
)

const (
	// GeminiContextWindowSize is the context window size for Gemini models.
	// Gemini has a 1M token context window.
	GeminiContextWindowSize = 1000000
)

// Parser implements client.EventParser for Gemini CLI JSON events.
// It embeds BaseParser for shared utilities and overrides methods as needed.
type Parser struct {
	client.BaseParser
}

// NewParser creates a new Gemini EventParser with the 1M token context window.
func NewParser() *Parser {
	return &Parser{
		BaseParser: client.NewBaseParser(GeminiContextWindowSize),
	}
}

// ParseEvent converts Gemini CLI JSON to client.OutputEvent.
// This is the main parsing entry point called for each stdout line.
//
// Event mappings:
//   - init -> EventSystem (subtype: init) with SessionID extraction
//   - message (role: assistant) -> EventAssistant with text content
//   - message (role: user) -> ignored (returns empty event)
//   - tool_use -> EventToolUse with tool name and parameters
//   - tool_result -> EventToolResult with status and output
//   - result -> EventResult with usage mapping
//   - error -> EventError with message and code
//
// Unknown event types are logged as warnings but do not cause errors.
// The event type is passed through as-is for forward compatibility.
func (p *Parser) ParseEvent(data []byte) (client.OutputEvent, error) {
	var raw geminiEvent
	if err := json.Unmarshal(data, &raw); err != nil {
		return client.OutputEvent{}, err
	}

	event := client.OutputEvent{
		Type: mapEventType(raw.Type, raw.Role),
	}

	// Copy raw data for debugging
	event.Raw = make([]byte, len(data))
	copy(event.Raw, data)

	// Handle init -> EventSystem (init subtype)
	if raw.Type == "init" {
		event.SubType = "init"
		event.SessionID = raw.SessionID
		return event, nil
	}

	// Handle message events with role-based discrimination
	if raw.Type == "message" {
		if raw.Role == "assistant" {
			event.Delta = raw.Delta // Pass through streaming chunk indicator
			event.Message = &client.MessageContent{
				Role:  raw.Role,
				Model: raw.Model,
				Content: []client.ContentBlock{
					{
						Type: "text",
						Text: raw.Content,
					},
				},
			}
		}
		// User messages (role: "user") are ignored - return event with empty message
		return event, nil
	}

	// Handle tool_use -> EventToolUse
	// Gemini format: {"tool_name": "...", "tool_id": "...", "parameters": {...}}
	if raw.Type == "tool_use" && raw.ToolName != "" {
		event.Tool = &client.ToolContent{
			ID:    raw.ToolID,
			Name:  raw.ToolName,
			Input: raw.Parameters,
		}
		// Populate Message.Content for process handler compatibility
		event.Message = &client.MessageContent{
			Role: "assistant",
			Content: []client.ContentBlock{
				{
					Type:  "tool_use",
					ID:    raw.ToolID,
					Name:  raw.ToolName,
					Input: raw.Parameters,
				},
			},
		}
		return event, nil
	}

	// Handle tool_result -> EventToolResult
	// Gemini format: {"tool_id": "...", "status": "...", "output": "..."}
	if raw.Type == "tool_result" && raw.ToolID != "" {
		event.Tool = &client.ToolContent{
			ID:     raw.ToolID,
			Output: raw.Output,
		}
		// Mark as error if status indicates failure
		if raw.Status == "error" || raw.Status == "failed" {
			event.IsErrorResult = true
		}
		return event, nil
	}

	// Handle result -> EventResult with usage
	if raw.Type == "result" {
		if raw.Stats != nil {
			// TokensUsed = prompt + cached tokens
			tokensUsed := raw.Stats.TokensPrompt + raw.Stats.TokensCached
			event.Usage = &client.UsageInfo{
				TokensUsed:   tokensUsed,
				TotalTokens:  p.ContextWindowSize(),
				OutputTokens: raw.Stats.TokensCandidates,
			}
			event.DurationMs = raw.Stats.DurationMs
		}
		return event, nil
	}

	// Handle error -> EventError
	if raw.Type == "error" && raw.Error != nil {
		event.Error = &client.ErrorInfo{
			Message: raw.Error.Message,
			Code:    raw.Error.Code,
		}

		// Log error event for context exhaustion discovery (Phase 2.5)
		log.Debug(log.CatOrch, "gemini error event",
			"provider", "gemini",
			"code", raw.Error.Code,
			"message", raw.Error.Message,
			"raw", string(data),
		)

		// Check for context exhaustion patterns
		if p.BaseParser.IsContextExhausted(event) {
			event.Error.Reason = client.ErrReasonContextExceeded
		}

		return event, nil
	}

	return event, nil
}

// ExtractSessionRef returns the session identifier from an event.
// Gemini uses session_id from init events as its session reference.
func (p *Parser) ExtractSessionRef(event client.OutputEvent, _ []byte) string {
	if event.Type == client.EventSystem && event.SubType == "init" && event.SessionID != "" {
		return event.SessionID
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
