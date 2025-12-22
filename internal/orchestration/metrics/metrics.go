// Package metrics provides token usage and cost tracking for orchestration mode.
package metrics

import (
	"fmt"
	"time"
)

// TokenMetrics holds comprehensive token usage and cost data for a coordinator or worker.
type TokenMetrics struct {
	// Per-turn input metrics
	InputTokens              int `json:"input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`

	// Per-turn output metrics
	OutputTokens int `json:"output_tokens"`

	// Context tracking
	ContextTokens int `json:"context_tokens"` // Total tokens in context window
	ContextWindow int `json:"context_window"` // Maximum context size

	// Cost tracking
	TurnCostUSD  float64 `json:"turn_cost_usd"`
	TotalCostUSD float64 `json:"total_cost_usd"`

	// Metadata
	LastUpdatedAt time.Time `json:"last_updated_at"`
}

// ContextUsage returns the percentage of context window used (0-100).
func (m TokenMetrics) ContextUsage() float64 {
	if m.ContextWindow == 0 {
		return 0
	}
	return float64(m.ContextTokens) / float64(m.ContextWindow) * 100
}

// FormatContextDisplay returns a human-readable context usage string (e.g., "27k/200k").
func (m TokenMetrics) FormatContextDisplay() string {
	if m.ContextWindow == 0 {
		return "-"
	}
	return fmt.Sprintf("%dk/%dk", m.ContextTokens/1000, m.ContextWindow/1000)
}

// FormatCostDisplay returns a human-readable cost string (e.g., "$0.0892").
func (m TokenMetrics) FormatCostDisplay() string {
	return fmt.Sprintf("$%.4f", m.TotalCostUSD)
}
