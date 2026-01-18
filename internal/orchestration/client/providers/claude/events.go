// Package claude provides a Go interface to headless Claude Code sessions.
package claude

import (
	"encoding/json"

	"github.com/zjrosen/perles/internal/orchestration/client"
)

// rawUsage holds raw token usage from Claude CLI JSON output.
type rawUsage struct {
	InputTokens              int `json:"input_tokens,omitempty"`
	OutputTokens             int `json:"output_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
}
type contentBlock struct {
	Type string `json:"type,omitempty"`
	Text string `json:"text,omitempty"`
	// Tool use fields (when Type == "tool_use")
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type messageContent struct {
	ID         string         `json:"id,omitempty"`
	Role       string         `json:"role,omitempty"`
	Content    []contentBlock `json:"content,omitempty"`
	Model      string         `json:"model,omitempty"`
	Usage      *rawUsage      `json:"usage,omitempty"`
	StopReason string         `json:"stop_reason,omitempty"`
}

// rawEvent is used for parsing raw Claude CLI JSON output.
// It mirrors client.OutputEvent but with rawUsage for the usage field.
// The Error field uses json.RawMessage because Claude CLI can send it as
// either a string (e.g., "invalid_request") or an object (e.g., {"message": "..."}).
type rawEvent struct {
	Type            client.EventType             `json:"type"`
	SubType         string                       `json:"subtype,omitempty"`
	SessionID       string                       `json:"session_id,omitempty"`
	WorkDir         string                       `json:"cwd,omitempty"`
	Message         *messageContent              `json:"message,omitempty"`
	Tool            *client.ToolContent          `json:"tool,omitempty"`
	Usage           *rawUsage                    `json:"usage,omitempty"`
	ModelUsage      map[string]client.ModelUsage `json:"modelUsage,omitempty"` //nolint:tagliatelle // Claude CLI uses camelCase
	Error           json.RawMessage              `json:"error,omitempty"`      // Can be string or ErrorInfo object
	TotalCostUSD    float64                      `json:"total_cost_usd,omitempty"`
	DurationMs      int64                        `json:"duration_ms,omitempty"`
	IsErrorResult   bool                         `json:"is_error,omitempty"`
	Result          string                       `json:"result,omitempty"`
	NumTurns        int                          `json:"num_turns,omitempty"`
	ParentToolUseId string                       `json:"parent_tool_use_id,omitempty"`
}

// parseErrorField handles the polymorphic error field from Claude CLI.
// It can be either a string (error code) or an object (ErrorInfo).
func parseErrorField(raw json.RawMessage) *client.ErrorInfo {
	if len(raw) == 0 {
		return nil
	}

	// Try parsing as object first
	var errInfo client.ErrorInfo
	if err := json.Unmarshal(raw, &errInfo); err == nil && errInfo.Message != "" {
		return &errInfo
	}

	// Try parsing as string (error code like "invalid_request")
	var errCode string
	if err := json.Unmarshal(raw, &errCode); err == nil && errCode != "" {
		reason := classifyErrorCode(errCode)
		return &client.ErrorInfo{
			Code:   errCode,
			Reason: reason,
		}
	}

	return nil
}

// classifyErrorCode maps known error codes to structured ErrorReasons.
func classifyErrorCode(code string) client.ErrorReason {
	switch code {
	case "invalid_request":
		return client.ErrReasonInvalidRequest
	case "rate_limit_exceeded", "rate_limited":
		return client.ErrReasonRateLimited
	default:
		return client.ErrReasonUnknown
	}
}
