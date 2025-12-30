package styles

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFormatCommentIndicator(t *testing.T) {
	tests := []struct {
		name     string
		count    int
		expected string
	}{
		{"zero comments", 0, ""},
		{"negative count", -1, ""},
		{"one comment", 1, "1\U0001F4AC"},
		{"few comments", 3, "3\U0001F4AC"},
		{"many comments", 99, "99\U0001F4AC"},
		{"lots of comments", 999, "999\U0001F4AC"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatCommentIndicator(tt.count)
			require.Equal(t, tt.expected, got, "FormatCommentIndicator(%d)", tt.count)
		})
	}
}
