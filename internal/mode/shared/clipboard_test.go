package shared

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func clearClipboardEnv(t *testing.T) {
	t.Helper()
	t.Setenv("SSH_TTY", "")
	t.Setenv("SSH_CLIENT", "")
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("TMUX", "")
	t.Setenv("STY", "")
}

func TestClipboardDetection(t *testing.T) {
	t.Run("isRemoteSession", func(t *testing.T) {
		tests := []struct {
			name     string
			envVars  map[string]string
			expected bool
		}{
			{
				name:     "no env vars",
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
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				clearClipboardEnv(t)
				for k, v := range tt.envVars {
					t.Setenv(k, v)
				}

				require.Equal(t, tt.expected, isRemoteSession())
			})
		}
	})

	t.Run("isLocalTmux", func(t *testing.T) {
		tests := []struct {
			name     string
			envVars  map[string]string
			expected bool
		}{
			{
				name:     "no env vars",
				envVars:  map[string]string{},
				expected: false,
			},
			{
				name:     "TMUX only (local)",
				envVars:  map[string]string{"TMUX": "/tmp/tmux-1000/default,12345,0"},
				expected: true,
			},
			{
				name:     "TMUX with SSH (remote)",
				envVars:  map[string]string{"TMUX": "/tmp/tmux", "SSH_TTY": "/dev/pts/0"},
				expected: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				clearClipboardEnv(t)
				for k, v := range tt.envVars {
					t.Setenv(k, v)
				}

				require.Equal(t, tt.expected, isLocalTmux())
			})
		}
	})

	t.Run("isGNUScreen", func(t *testing.T) {
		tests := []struct {
			name     string
			envVars  map[string]string
			expected bool
		}{
			{
				name:     "no env vars",
				envVars:  map[string]string{},
				expected: false,
			},
			{
				name:     "STY set",
				envVars:  map[string]string{"STY": "12345.pts-0.hostname"},
				expected: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				clearClipboardEnv(t)
				for k, v := range tt.envVars {
					t.Setenv(k, v)
				}

				require.Equal(t, tt.expected, isGNUScreen())
			})
		}
	})
}

func TestBuildOSC52Sequence(t *testing.T) {
	t.Run("direct sequence", func(t *testing.T) {
		seq := buildOSC52Sequence("ISSUE-123", false)
		require.Equal(t, "\x1b]52;c;SVNTVUUtMTIz\x07", seq)
	})

	t.Run("tmux passthrough sequence", func(t *testing.T) {
		seq := buildOSC52Sequence("ISSUE-123", true)
		require.Equal(t, "\x1bPtmux;\x1b\x1b]52;c;SVNTVUUtMTIz\x07\x1b\\", seq)
	})

	t.Run("special characters", func(t *testing.T) {
		tests := []struct {
			name     string
			text     string
			expected string
		}{
			{
				name:     "with spaces",
				text:     "hello world",
				expected: "\x1b]52;c;aGVsbG8gd29ybGQ=\x07",
			},
			{
				name:     "with newlines",
				text:     "line1\nline2",
				expected: "\x1b]52;c;bGluZTEKbGluZTI=\x07",
			},
			{
				name:     "unicode",
				text:     "hello 世界",
				expected: "\x1b]52;c;aGVsbG8g5LiW55WM\x07",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				seq := buildOSC52Sequence(tt.text, false)
				require.Equal(t, tt.expected, seq)
			})
		}
	})
}
