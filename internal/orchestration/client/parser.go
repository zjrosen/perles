package client

import (
	"encoding/json"
	"strings"
)

// EventParser defines the contract for provider-specific event parsing.
// All providers must implement this interface to ensure consistent behavior.
type EventParser interface {
	// ParseEvent converts provider-specific JSON to OutputEvent.
	// This is the main parsing entry point called for each stdout line.
	ParseEvent(data []byte) (OutputEvent, error)

	// ExtractSessionRef returns the session identifier from an event.
	// Called for every event (not just init) to support providers like OpenCode
	// that may emit session IDs in non-init events.
	ExtractSessionRef(event OutputEvent, rawLine []byte) string

	// IsContextExhausted checks if an event indicates context window exhaustion.
	// Centralizes detection logic that was previously duplicated/missing.
	IsContextExhausted(event OutputEvent) bool

	// ContextWindowSize returns the provider's context window size in tokens.
	// Replaces hardcoded values like 200000 (Claude/Amp) and 1000000 (Gemini).
	ContextWindowSize() int
}

// BaseParser provides common parsing utilities that all providers can embed.
// Providers embed this struct and override methods as needed.
type BaseParser struct {
	contextWindowSize int
}

// NewBaseParser creates a BaseParser with the specified context window size.
func NewBaseParser(contextWindow int) BaseParser {
	return BaseParser{contextWindowSize: contextWindow}
}

// ContextWindowSize returns the provider's context window size.
func (p *BaseParser) ContextWindowSize() int {
	return p.contextWindowSize
}

// IsContextExhausted implements shared context exhaustion detection.
// Consolidates detection logic from Claude and Amp, adding patterns for all providers.
func (p *BaseParser) IsContextExhausted(event OutputEvent) bool {
	if event.Error == nil {
		return false
	}
	// Check structured reason first (already classified)
	if event.Error.Reason == ErrReasonContextExceeded {
		return true
	}
	// Fall back to message pattern matching
	return isContextExhaustedMessage(event.GetErrorMessage())
}

// isContextExhaustedMessage checks if an error message indicates context window exhaustion.
// It performs case-insensitive matching against 6 known patterns.
func isContextExhaustedMessage(msg string) bool {
	if msg == "" {
		return false
	}
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "prompt is too long") ||
		strings.Contains(lower, "context window exceeded") ||
		strings.Contains(lower, "context exceeded") ||
		strings.Contains(lower, "context limit") ||
		strings.Contains(lower, "token limit") ||
		strings.Contains(lower, "maximum context length")
}

// ParsePolymorphicError handles the polymorphic error field from provider CLI outputs.
// It can be:
//   - A string: "error message" or "Connection refused"
//   - An object: {"code": "x", "message": "y"}
//   - Amp nested format: "413 {\"type\":\"error\",\"error\":{\"type\":\"invalid_request_error\",\"message\":\"Prompt is too long\"}}"
//
// Returns nil for null/empty input.
func ParsePolymorphicError(raw json.RawMessage) *ErrorInfo {
	if len(raw) == 0 {
		return nil
	}

	// Try parsing as object first
	var errInfo ErrorInfo
	if err := json.Unmarshal(raw, &errInfo); err == nil && (errInfo.Message != "" || errInfo.Code != "") {
		return &errInfo
	}

	// Try parsing as string (error message like "413 {...}")
	var errStr string
	if err := json.Unmarshal(raw, &errStr); err == nil && errStr != "" {
		return parseErrorString(errStr)
	}

	return nil
}

// parseErrorString extracts error information from an error string.
// Handles formats like: "413 {\"type\":\"error\",\"error\":{...}}"
func parseErrorString(errStr string) *ErrorInfo {
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
			return &ErrorInfo{
				Message: nested.Error.Message,
				Code:    nested.Error.Type,
			}
		}
	}

	// Fall back to using the entire string as the message
	return &ErrorInfo{
		Message: errStr,
	}
}
