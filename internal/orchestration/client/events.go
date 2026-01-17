package client

import (
	"encoding/json"
	"strings"
	"time"
)

// EventType identifies the kind of output event.
type EventType string

const (
	// EventInit is the initialization event with session info.
	EventInit EventType = "init"
	// EventSystem is a system-level event (init is a subtype).
	EventSystem EventType = "system"
	// EventAssistant is an assistant message event.
	EventAssistant EventType = "assistant"
	// EventToolUse is a tool invocation event.
	EventToolUse EventType = "tool_use"
	// EventToolResult is a tool result event.
	EventToolResult EventType = "tool_result"
	// EventResult is a completion/result event.
	EventResult EventType = "result"
	// EventError is an error event.
	EventError EventType = "error"
)

// OutputEvent represents a parsed event from the headless process output.
// This is a unified structure that all providers map their events to.
type OutputEvent struct {
	// Core event identification
	Type      EventType `json:"type"`
	SubType   string    `json:"subtype,omitempty"`
	Timestamp time.Time `json:"-"`

	// Delta indicates this is a streaming chunk that should be accumulated
	// with the previous message rather than displayed as a new message.
	Delta bool `json:"delta,omitempty"`

	// Session information (from init events)
	// SessionID is the session identifier from init events.
	// Note: The HeadlessProcess interface uses SessionRef() method for this value.
	SessionID string `json:"session_id,omitempty"`
	WorkDir   string `json:"cwd,omitempty"`

	// Message content (from assistant events)
	Message *MessageContent `json:"message,omitempty"`

	// Tool information (from tool_use and tool_result events)
	Tool *ToolContent `json:"tool,omitempty"`

	// Token usage (from result events)
	Usage      *UsageInfo            `json:"usage,omitempty"`
	ModelUsage map[string]ModelUsage `json:"modelUsage,omitempty"` //nolint:tagliatelle // Claude Code uses camelCase

	// Error information
	Error *ErrorInfo `json:"error,omitempty"`

	// Cost and duration (from result events)
	TotalCostUSD  float64 `json:"total_cost_usd,omitempty"`
	DurationMs    int64   `json:"duration_ms,omitempty"`
	IsErrorResult bool    `json:"is_error,omitempty"`
	Result        string  `json:"result,omitempty"`

	// Raw payload for debugging
	Raw json.RawMessage `json:"-"`
}

// IsInit returns true if this is a system init event.
func (e *OutputEvent) IsInit() bool {
	return e.Type == EventSystem && e.SubType == "init"
}

// IsAssistant returns true if this is an assistant message event.
func (e *OutputEvent) IsAssistant() bool {
	return e.Type == EventAssistant
}

// IsToolUse returns true if this is a tool_use event.
func (e *OutputEvent) IsToolUse() bool {
	return e.Type == EventToolUse
}

// IsToolResult returns true if this is a tool_result event.
func (e *OutputEvent) IsToolResult() bool {
	return e.Type == EventToolResult
}

// IsResult returns true if this is a result (completion) event.
func (e *OutputEvent) IsResult() bool {
	return e.Type == EventResult
}

// IsError returns true if this is an error event.
// This includes explicit error events and result events with is_error=true.
func (e *OutputEvent) IsError() bool {
	return e.Type == EventError || e.Error != nil || e.IsErrorResult
}

// GetErrorMessage returns the error message from this event.
// For explicit error events, returns Error.Message.
// For result errors (is_error=true), returns the Result field.
func (e *OutputEvent) GetErrorMessage() string {
	if e.Error != nil && e.Error.Message != "" {
		return e.Error.Message
	}
	if e.IsErrorResult && e.Result != "" {
		return e.Result
	}
	return "unknown error"
}

// GetContextTokens returns the current context window usage from this event.
// Returns 0 if no usage info is available.
func (e *OutputEvent) GetContextTokens() int {
	if e.Usage == nil {
		return 0
	}
	return e.Usage.TokensUsed
}

// MessageContent holds assistant message content.
type MessageContent struct {
	ID      string         `json:"id,omitempty"`
	Role    string         `json:"role,omitempty"`
	Content []ContentBlock `json:"content,omitempty"`
	Model   string         `json:"model,omitempty"`
}

// GetText returns the concatenated text content from all text blocks.
func (m *MessageContent) GetText() string {
	if m == nil {
		return ""
	}
	var sb strings.Builder
	for _, block := range m.Content {
		if block.Type == "text" {
			sb.WriteString(block.Text)
		}
	}
	return sb.String()
}

// GetToolUses returns all tool_use content blocks from the message.
func (m *MessageContent) GetToolUses() []ContentBlock {
	if m == nil {
		return nil
	}
	var tools []ContentBlock
	for _, block := range m.Content {
		if block.Type == "tool_use" {
			tools = append(tools, block)
		}
	}
	return tools
}

// HasToolUses returns true if the message contains any tool_use content blocks.
func (m *MessageContent) HasToolUses() bool {
	if m == nil {
		return false
	}
	for _, block := range m.Content {
		if block.Type == "tool_use" {
			return true
		}
	}
	return false
}

// ContentBlock represents a single content block in a message.
// Can be text, tool_use, or tool_result.
type ContentBlock struct {
	Type string `json:"type,omitempty"`
	Text string `json:"text,omitempty"`
	// Tool use fields (when Type == "tool_use")
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

// ToolContent holds tool use/result content.
type ToolContent struct {
	ID      string          `json:"id,omitempty"`
	Name    string          `json:"name,omitempty"`
	Input   json.RawMessage `json:"input,omitempty"`
	Content string          `json:"content,omitempty"`
	Output  string          `json:"output,omitempty"`
}

// GetOutput returns the tool output, preferring Output over Content.
// Different providers use different field names.
func (t *ToolContent) GetOutput() string {
	if t == nil {
		return ""
	}
	if t.Output != "" {
		return t.Output
	}
	return t.Content
}

// UsageInfo holds simplified token usage that all providers populate.
type UsageInfo struct {
	TokensUsed   int `json:"tokens_used,omitempty"`   // Context tokens consumed
	TotalTokens  int `json:"total_tokens,omitempty"`  // Context window size (default 200000)
	OutputTokens int `json:"output_tokens,omitempty"` // Tokens generated by model
}

// ModelUsage holds per-model usage details from result events.
//
//nolint:tagliatelle // Claude Code API uses camelCase, not snake_case
type ModelUsage struct {
	InputTokens              int     `json:"inputTokens,omitempty"`
	OutputTokens             int     `json:"outputTokens,omitempty"`
	CacheReadInputTokens     int     `json:"cacheReadInputTokens,omitempty"`
	CacheCreationInputTokens int     `json:"cacheCreationInputTokens,omitempty"`
	ContextWindow            int     `json:"contextWindow,omitempty"`
	CostUSD                  float64 `json:"costUSD,omitempty"`
}

// ErrorReason provides structured error classification for known error types.
type ErrorReason string

const (
	// ErrReasonUnknown is the default when error type cannot be determined.
	ErrReasonUnknown ErrorReason = ""
	// ErrReasonContextExceeded indicates the prompt/context window was exhausted.
	ErrReasonContextExceeded ErrorReason = "context_exceeded"
	// ErrReasonRateLimited indicates the API rate limit was hit.
	ErrReasonRateLimited ErrorReason = "rate_limited"
	// ErrReasonInvalidRequest indicates a malformed or invalid request.
	ErrReasonInvalidRequest ErrorReason = "invalid_request"
)

// ErrorInfo holds error details.
type ErrorInfo struct {
	Message string      `json:"message,omitempty"`
	Code    string      `json:"code,omitempty"`
	Reason  ErrorReason `json:"reason,omitempty"` // Structured error classification
}

// IsContextExceeded returns true if this error indicates context window exhaustion.
func (e *ErrorInfo) IsContextExceeded() bool {
	return e != nil && e.Reason == ErrReasonContextExceeded
}
