// Package shared provides common utilities shared between mode controllers.
package shared

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// Clipboard defines the interface for clipboard operations.
// Use SystemClipboard for production and mocks.MockClipboard for testing.
type Clipboard interface {
	Copy(text string) error
}

// SystemClipboard implements Clipboard using the system clipboard.
// It auto-detects remote/tmux sessions and uses OSC 52 escape sequences
// when appropriate, falling back to native clipboard tools otherwise.
type SystemClipboard struct{}

// Copy copies text to the system clipboard.
// In remote sessions (SSH) or terminal multiplexers (tmux, screen),
// it uses OSC 52 escape sequences. Otherwise, it uses native tools
// like pbcopy (macOS) or xclip (Linux).
func (SystemClipboard) Copy(text string) error {
	if shouldUseOSC52() {
		return copyViaOSC52(text)
	}
	return copyViaNative(text)
}

// shouldUseOSC52 returns true if we should use OSC 52 escape sequences
// instead of native clipboard tools. This is the case when running in
// a remote session (SSH) or terminal multiplexer (tmux, screen).
func shouldUseOSC52() bool {
	// Check for SSH session
	if os.Getenv("SSH_TTY") != "" ||
		os.Getenv("SSH_CLIENT") != "" ||
		os.Getenv("SSH_CONNECTION") != "" {
		return true
	}
	// Check for tmux
	if os.Getenv("TMUX") != "" {
		return true
	}
	// Check for GNU screen
	if os.Getenv("STY") != "" {
		return true
	}
	return false
}

// copyViaOSC52 copies text using OSC 52 escape sequences.
// When inside tmux, it wraps the sequence in a DCS passthrough.
func copyViaOSC52(text string) (err error) {
	encoded := base64.StdEncoding.EncodeToString([]byte(text))

	var seq string
	if os.Getenv("TMUX") != "" {
		// tmux passthrough: wrap OSC 52 in DCS sequence
		// \x1bP starts DCS, tmux; identifies passthrough
		// Inner \x1b is doubled to \x1b\x1b
		// \x1b\\ ends DCS
		seq = fmt.Sprintf("\x1bPtmux;\x1b\x1b]52;c;%s\x07\x1b\\", encoded)
	} else {
		// Direct OSC 52
		seq = fmt.Sprintf("\x1b]52;c;%s\x07", encoded)
	}

	// Write to /dev/tty to bypass any stdout redirection
	// and work correctly with Bubble Tea's alt-screen mode
	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("failed to open /dev/tty: %w", err)
	}
	defer func() {
		if closeErr := tty.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	_, err = tty.WriteString(seq)
	return err
}

// copyViaNative copies text using native clipboard tools.
func copyViaNative(text string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		cmd = exec.Command("xclip", "-selection", "clipboard")
	default:
		cmd = exec.Command("xclip", "-selection", "clipboard")
	}

	pipe, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	if _, err := pipe.Write([]byte(text)); err != nil {
		return err
	}

	if err := pipe.Close(); err != nil {
		return err
	}

	return cmd.Wait()
}
