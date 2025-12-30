package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestContextUsage(t *testing.T) {
	tests := []struct {
		name          string
		contextTokens int
		contextWindow int
		want          float64
	}{
		{
			name:          "zero window returns zero",
			contextTokens: 1000,
			contextWindow: 0,
			want:          0,
		},
		{
			name:          "zero tokens returns zero",
			contextTokens: 0,
			contextWindow: 200000,
			want:          0,
		},
		{
			name:          "50% usage",
			contextTokens: 100000,
			contextWindow: 200000,
			want:          50,
		},
		{
			name:          "85% usage (critical threshold)",
			contextTokens: 170000,
			contextWindow: 200000,
			want:          85,
		},
		{
			name:          "70% usage (warning threshold)",
			contextTokens: 140000,
			contextWindow: 200000,
			want:          70,
		},
		{
			name:          "27k/200k typical usage",
			contextTokens: 27000,
			contextWindow: 200000,
			want:          13.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := TokenMetrics{
				ContextTokens: tt.contextTokens,
				ContextWindow: tt.contextWindow,
			}
			got := m.ContextUsage()
			require.Equal(t, tt.want, got, "ContextUsage()")
		})
	}
}

func TestFormatContextDisplay(t *testing.T) {
	tests := []struct {
		name          string
		contextTokens int
		contextWindow int
		want          string
	}{
		{
			name:          "zero window returns dash",
			contextTokens: 1000,
			contextWindow: 0,
			want:          "-",
		},
		{
			name:          "typical 27k/200k",
			contextTokens: 27000,
			contextWindow: 200000,
			want:          "27k/200k",
		},
		{
			name:          "45k/200k",
			contextTokens: 45000,
			contextWindow: 200000,
			want:          "45k/200k",
		},
		{
			name:          "small numbers round down",
			contextTokens: 500,
			contextWindow: 200000,
			want:          "0k/200k",
		},
		{
			name:          "100k context window",
			contextTokens: 50000,
			contextWindow: 100000,
			want:          "50k/100k",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := TokenMetrics{
				ContextTokens: tt.contextTokens,
				ContextWindow: tt.contextWindow,
			}
			got := m.FormatContextDisplay()
			require.Equal(t, tt.want, got, "FormatContextDisplay()")
		})
	}
}

func TestFormatCostDisplay(t *testing.T) {
	tests := []struct {
		name         string
		totalCostUSD float64
		want         string
	}{
		{
			name:         "zero cost",
			totalCostUSD: 0,
			want:         "$0.0000",
		},
		{
			name:         "small cost",
			totalCostUSD: 0.0892,
			want:         "$0.0892",
		},
		{
			name:         "larger cost",
			totalCostUSD: 1.2345,
			want:         "$1.2345",
		},
		{
			name:         "rounds to 4 decimal places",
			totalCostUSD: 0.12345678,
			want:         "$0.1235",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := TokenMetrics{
				TotalCostUSD: tt.totalCostUSD,
			}
			got := m.FormatCostDisplay()
			require.Equal(t, tt.want, got, "FormatCostDisplay()")
		})
	}
}

func TestTokenMetrics_FullStruct(t *testing.T) {
	// Test a fully populated struct to ensure all fields work together
	now := time.Now()
	m := TokenMetrics{
		InputTokens:              5000,
		CacheReadInputTokens:     20000,
		CacheCreationInputTokens: 2000,
		OutputTokens:             1000,
		ContextTokens:            27000,
		ContextWindow:            200000,
		TurnCostUSD:              0.0150,
		TotalCostUSD:             0.0892,
		LastUpdatedAt:            now,
	}

	// Verify all calculations
	require.Equal(t, 13.5, m.ContextUsage(), "ContextUsage()")
	require.Equal(t, "27k/200k", m.FormatContextDisplay(), "FormatContextDisplay()")
	require.Equal(t, "$0.0892", m.FormatCostDisplay(), "FormatCostDisplay()")
	require.Equal(t, now, m.LastUpdatedAt, "LastUpdatedAt")
}
