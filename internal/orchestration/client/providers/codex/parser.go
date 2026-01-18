package codex

import (
	"encoding/json"

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/client"
)

const (
	// CodexContextWindowSize is the context window size for Codex (200000 tokens).
	// TODO: This should be configurable per model.
	CodexContextWindowSize = 200000
)

// Parser implements client.EventParser for Codex CLI JSON events.
// It embeds BaseParser for shared utilities and overrides methods as needed.
type Parser struct {
	client.BaseParser
}

// NewParser creates a new Codex EventParser with the default context window size.
func NewParser() *Parser {
	return &Parser{
		BaseParser: client.NewBaseParser(CodexContextWindowSize),
	}
}

// ParseEvent converts Codex CLI JSON to client.OutputEvent.
// This is the main parsing entry point called for each stdout line.
//
// Event mappings:
//   - thread.started -> EventSystem (subtype: init) with SessionID extraction
//   - item.completed (agent_message) -> EventAssistant with text
//   - item.started (command_execution) -> EventToolUse with command
//   - item.completed (command_execution) -> EventToolResult with output
//   - turn.completed -> EventResult with usage mapping
//   - turn.failed -> EventError
//   - error -> EventError
//
// Note: reasoning events are intentionally ignored as they represent internal
// chain-of-thought summaries that should not be exposed to users.
func (p *Parser) ParseEvent(data []byte) (client.OutputEvent, error) {
	var raw codexEvent
	if err := json.Unmarshal(data, &raw); err != nil {
		return client.OutputEvent{}, err
	}

	event := client.OutputEvent{
		Type: mapEventType(raw.Type, raw.Item),
	}

	// Handle thread.started -> EventSystem (init subtype)
	if raw.Type == "thread.started" {
		event.SubType = "init"
		event.SessionID = raw.ThreadID
		return event, nil
	}

	// Handle turn.completed -> EventResult with usage
	if raw.Type == "turn.completed" {
		if raw.Usage != nil {
			// Codex: TokensUsed = input_tokens + cached_input_tokens
			tokensUsed := raw.Usage.InputTokens + raw.Usage.CachedInputTokens
			event.Usage = &client.UsageInfo{
				TokensUsed:   tokensUsed,
				TotalTokens:  p.ContextWindowSize(),
				OutputTokens: raw.Usage.OutputTokens,
			}
		}
		return event, nil
	}

	// Handle turn.failed and error events -> EventError
	if raw.Type == "turn.failed" || raw.Type == "error" {
		var message string
		// turn.failed uses "error":{"message":"..."}
		if raw.Error != nil && raw.Error.Message != "" {
			message = raw.Error.Message
		} else if raw.Message != "" {
			// error events use top-level "message":"..."
			message = raw.Message
		}
		event.Error = &client.ErrorInfo{
			Message: message,
		}

		// Log error event for context exhaustion discovery (Phase 2.5)
		log.Debug(log.CatOrch, "codex error event",
			"provider", "codex",
			"type", raw.Type,
			"message", message,
			"raw", string(data),
		)

		// Check for context exhaustion patterns
		if p.BaseParser.IsContextExhausted(event) {
			event.Error.Reason = client.ErrReasonContextExceeded
		}

		return event, nil
	}

	// Handle item events
	if raw.Item != nil {
		event = mapItemEvent(raw, event)
	}

	return event, nil
}

// ExtractSessionRef returns the session identifier from an event.
// Codex uses thread_id from thread.started events as its session reference.
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
