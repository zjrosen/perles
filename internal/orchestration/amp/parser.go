package amp

import (
	"encoding/json"

	"github.com/zjrosen/perles/internal/orchestration/client"
)

const (
	// AmpContextWindowSize is the context window size for Amp (Claude models).
	AmpContextWindowSize = 200000
)

// Parser implements client.EventParser for Amp CLI JSON events.
// It embeds BaseParser for shared utilities and overrides methods as needed.
type Parser struct {
	client.BaseParser
}

// NewParser creates a new Amp EventParser with the default context window size.
func NewParser() *Parser {
	return &Parser{
		BaseParser: client.NewBaseParser(AmpContextWindowSize),
	}
}

// ParseEvent converts Amp CLI JSON to client.OutputEvent.
// This is the main parsing entry point called for each stdout line.
func (p *Parser) ParseEvent(data []byte) (client.OutputEvent, error) {
	var raw ampEvent
	if err := json.Unmarshal(data, &raw); err != nil {
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
		Error:         client.ParsePolymorphicError(raw.Error),
	}

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
				TotalTokens:  p.ContextWindowSize(),
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
	if event.IsErrorResult || event.Type == client.EventError {
		errorMsg := event.Result
		if event.Error != nil && event.Error.Message != "" {
			errorMsg = event.Error.Message
		}
		if p.BaseParser.IsContextExhausted(client.OutputEvent{Error: &client.ErrorInfo{Message: errorMsg}}) ||
			(event.Error != nil && event.Error.Reason == client.ErrReasonContextExceeded) {
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

// ExtractSessionRef returns the session identifier from an event.
// Amp uses threadID/session_id as its session reference from init events.
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
