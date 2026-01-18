package claude

import (
	"encoding/json"
	"strings"

	"github.com/zjrosen/perles/internal/orchestration/client"
)

const (
	// ClaudeContextWindowSize is the context window size for Claude models (Opus 4.5).
	// TODO: This should be configurable per model.
	ClaudeContextWindowSize = 200000
)

// Parser implements client.EventParser for Claude CLI JSON events.
// It embeds BaseParser for shared utilities and overrides methods as needed.
type Parser struct {
	client.BaseParser
}

// NewParser creates a new Claude EventParser with the default context window size.
func NewParser() *Parser {
	return &Parser{
		BaseParser: client.NewBaseParser(ClaudeContextWindowSize),
	}
}

// ParseEvent converts Claude CLI JSON to client.OutputEvent.
// This is the main parsing entry point called for each stdout line.
func (p *Parser) ParseEvent(data []byte) (client.OutputEvent, error) {
	var raw rawEvent
	if err := json.Unmarshal(data, &raw); err != nil {
		return client.OutputEvent{}, err
	}

	event := client.OutputEvent{
		Type:          raw.Type,
		SubType:       raw.SubType,
		SessionID:     raw.SessionID,
		WorkDir:       raw.WorkDir,
		Tool:          raw.Tool,
		ModelUsage:    raw.ModelUsage,
		TotalCostUSD:  raw.TotalCostUSD,
		DurationMs:    raw.DurationMs,
		IsErrorResult: raw.IsErrorResult,
		Result:        raw.Result,
	}

	// Parse error field - Claude has specific polymorphic error handling
	// that classifies string error codes (like "invalid_request") into the Code field
	event.Error = parseErrorField(raw.Error)

	if raw.Message != nil {
		event.Message = &client.MessageContent{
			ID:    raw.Message.ID,
			Role:  raw.Message.Role,
			Model: raw.Message.Model,
		}
		for _, block := range raw.Message.Content {
			event.Message.Content = append(event.Message.Content, client.ContentBlock{
				Type:  block.Type,
				Text:  block.Text,
				ID:    block.ID,
				Name:  block.Name,
				Input: block.Input,
			})
		}

		// Detect context exhaustion pattern:
		// - error code is "invalid_request"
		// - message content contains "Prompt is too long"
		// - stop_reason is "stop_sequence" (unusual for normal completion)
		if event.Error != nil && event.Error.Code == "invalid_request" {
			messageText := event.Message.GetText()
			if strings.Contains(messageText, "Prompt is too long") || raw.Message.StopReason == "stop_sequence" {
				event.Error.Reason = client.ErrReasonContextExceeded
				if event.Error.Message == "" {
					event.Error.Message = messageText
				}
			}
		}
	}

	// TODO this could be wrong but currently the EventResult doesn't feel like its the correct token usage.
	// will need to revisit this to understand if this is a Claude bug or if we should be using the assistant event
	if raw.Type == client.EventAssistant && raw.Message != nil && raw.Message.Usage != nil {
		tokensUsed := raw.Message.Usage.InputTokens + raw.Message.Usage.CacheReadInputTokens + raw.Message.Usage.CacheCreationInputTokens
		event.Usage = &client.UsageInfo{
			TokensUsed:   tokensUsed,
			TotalTokens:  p.ContextWindowSize(),
			OutputTokens: raw.Message.Usage.OutputTokens,
		}
	}

	// Copy raw data for debugging
	event.Raw = make([]byte, len(data))
	copy(event.Raw, data)

	return event, nil
}

// ExtractSessionRef returns the session identifier from an event.
// Claude uses the OnInitEvent hook for session extraction, so this returns empty.
// The extractSession function in process.go handles Claude-specific session extraction.
func (p *Parser) ExtractSessionRef(_ client.OutputEvent, _ []byte) string {
	// Claude uses the OnInitEvent hook pattern via extractSession() in process.go
	// for session extraction. This method returns empty to indicate no extraction.
	return ""
}

// IsContextExhausted checks if an event indicates context window exhaustion.
// This extends BaseParser's detection with Claude-specific stop_reason check.
func (p *Parser) IsContextExhausted(event client.OutputEvent) bool {
	// First check BaseParser's detection (covers ErrReasonContextExceeded and message patterns)
	if p.BaseParser.IsContextExhausted(event) {
		return true
	}

	// Claude-specific: check for stop_reason == "stop_sequence" which can indicate context exhaustion
	// This is detected during parsing and sets ErrReasonContextExceeded, but we double-check here
	if event.Error != nil && event.Error.Reason == client.ErrReasonContextExceeded {
		return true
	}

	return false
}

// Verify Parser implements EventParser at compile time.
var _ client.EventParser = (*Parser)(nil)
