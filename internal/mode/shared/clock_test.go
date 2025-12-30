package shared

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFormatRelativeTime(t *testing.T) {
	// Reference time for all tests
	now := time.Date(2025, 12, 13, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		input    time.Time
		expected string
	}{
		// Edge cases: now and near-now
		{"now - exact", now, "now"},
		{"now - 30 seconds ago", now.Add(-30 * time.Second), "now"},
		{"now - 59 seconds ago", now.Add(-59 * time.Second), "now"},

		// Minutes boundary
		{"1m ago - boundary", now.Add(-1 * time.Minute), "1m ago"},
		{"5m ago", now.Add(-5 * time.Minute), "5m ago"},
		{"30m ago", now.Add(-30 * time.Minute), "30m ago"},
		{"59m ago - boundary", now.Add(-59 * time.Minute), "59m ago"},

		// Hours boundary
		{"1h ago - boundary", now.Add(-1 * time.Hour), "1h ago"},
		{"3h ago", now.Add(-3 * time.Hour), "3h ago"},
		{"12h ago", now.Add(-12 * time.Hour), "12h ago"},
		{"23h ago - boundary", now.Add(-23 * time.Hour), "23h ago"},

		// Days boundary
		{"1d ago - boundary", now.Add(-24 * time.Hour), "1d ago"},
		{"2d ago", now.Add(-48 * time.Hour), "2d ago"},
		{"5d ago", now.Add(-5 * 24 * time.Hour), "5d ago"},
		{"6d ago - boundary", now.Add(-6 * 24 * time.Hour), "6d ago"},

		// Weeks boundary
		{"1w ago - boundary", now.Add(-7 * 24 * time.Hour), "1w ago"},
		{"2w ago", now.Add(-14 * 24 * time.Hour), "2w ago"},
		{"3w ago - boundary", now.Add(-21 * 24 * time.Hour), "3w ago"},

		// Months boundary (4 weeks = 28 days transition, but months use 30-day periods)
		{"1mo ago - boundary", now.Add(-30 * 24 * time.Hour), "1mo ago"},
		{"2mo ago", now.Add(-60 * 24 * time.Hour), "2mo ago"},
		{"6mo ago", now.Add(-180 * 24 * time.Hour), "6mo ago"},
		{"11mo ago", now.Add(-330 * 24 * time.Hour), "11mo ago"},

		// Years boundary (365 days)
		{"1y ago - boundary", now.Add(-365 * 24 * time.Hour), "1y ago"},
		{"2y ago", now.Add(-730 * 24 * time.Hour), "2y ago"},
		{"5y ago", now.Add(-5 * 365 * 24 * time.Hour), "5y ago"},

		// Edge case: future timestamps
		{"future - 1h from now", now.Add(1 * time.Hour), "now"},
		{"future - 1d from now", now.Add(24 * time.Hour), "now"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatRelativeTimeFrom(tt.input, now)
			require.Equal(t, tt.expected, got, "FormatRelativeTimeFrom(%v, %v)", tt.input, now)
		})
	}
}
