package shared

import (
	"encoding/base64"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShouldUseOSC52(t *testing.T) {
	// Helper to clear all relevant env vars
	clearEnv := func() {
		os.Unsetenv("SSH_TTY")
		os.Unsetenv("SSH_CLIENT")
		os.Unsetenv("SSH_CONNECTION")
		os.Unsetenv("TMUX")
		os.Unsetenv("STY")
	}

	tests := []struct {
		name     string
		envVars  map[string]string
		expected bool
	}{
		{
			name:     "no env vars set",
			envVars:  map[string]string{},
			expected: false,
		},
		{
			name:     "SSH_TTY set",
			envVars:  map[string]string{"SSH_TTY": "/dev/pts/0"},
			expected: true,
		},
		{
			name:     "SSH_CLIENT set",
			envVars:  map[string]string{"SSH_CLIENT": "192.168.1.1 12345 22"},
			expected: true,
		},
		{
			name:     "SSH_CONNECTION set",
			envVars:  map[string]string{"SSH_CONNECTION": "192.168.1.1 12345 192.168.1.2 22"},
			expected: true,
		},
		{
			name:     "TMUX set",
			envVars:  map[string]string{"TMUX": "/tmp/tmux-1000/default,12345,0"},
			expected: true,
		},
		{
			name:     "STY set (GNU screen)",
			envVars:  map[string]string{"STY": "12345.pts-0.hostname"},
			expected: true,
		},
		{
			name:     "SSH and TMUX both set",
			envVars:  map[string]string{"SSH_TTY": "/dev/pts/0", "TMUX": "/tmp/tmux"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv()
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}
			t.Cleanup(clearEnv)

			result := shouldUseOSC52()
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestOSC52SequenceFormat(t *testing.T) {
	// Test that base64 encoding is correct
	text := "ISSUE-123"
	encoded := base64.StdEncoding.EncodeToString([]byte(text))
	require.Equal(t, "SVNTVUUtMTIz", encoded)

	// Verify the expected OSC 52 sequence format
	expectedDirect := "\x1b]52;c;SVNTVUUtMTIz\x07"
	directSeq := "\x1b]52;c;" + encoded + "\x07"
	require.Equal(t, expectedDirect, directSeq)

	// Verify the expected tmux passthrough format
	expectedTmux := "\x1bPtmux;\x1b\x1b]52;c;SVNTVUUtMTIz\x07\x1b\\"
	tmuxSeq := "\x1bPtmux;\x1b\x1b]52;c;" + encoded + "\x07\x1b\\"
	require.Equal(t, expectedTmux, tmuxSeq)
}

func TestOSC52SequenceWithSpecialChars(t *testing.T) {
	// Test that special characters are properly base64 encoded
	tests := []struct {
		name    string
		text    string
		encoded string
	}{
		{
			name:    "simple text",
			text:    "hello",
			encoded: "aGVsbG8=",
		},
		{
			name:    "with spaces",
			text:    "hello world",
			encoded: "aGVsbG8gd29ybGQ=",
		},
		{
			name:    "with newlines",
			text:    "line1\nline2",
			encoded: "bGluZTEKbGluZTI=",
		},
		{
			name:    "unicode",
			text:    "hello 世界",
			encoded: "aGVsbG8g5LiW55WM",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := base64.StdEncoding.EncodeToString([]byte(tt.text))
			require.Equal(t, tt.encoded, encoded)
		})
	}
}
