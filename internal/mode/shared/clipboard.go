// Package shared provides common utilities shared between mode controllers.
package shared

import (
	"encoding/base64"
	"fmt"
	"os"

	"github.com/atotto/clipboard"
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
// Priority:
// 1. Local tmux session → use native clipboard tools directly
// 2. Remote SSH session → use OSC 52 escape sequences
// 3. GNU screen → use OSC 52 escape sequences
// 4. Bare local terminal → use native clipboard tools directly
func (SystemClipboard) Copy(text string) error {
	if isLocalTmux() {
		return copyViaNative(text)
	}

	if isRemoteSession() || isGNUScreen() {
		return copyViaOSC52(text)
	}

	return copyViaNative(text)
}

// isLocalTmux returns true if running in tmux without SSH.
func isLocalTmux() bool {
	return os.Getenv("TMUX") != "" && !isRemoteSession()
}

// isRemoteSession returns true if running over SSH.
func isRemoteSession() bool {
	return os.Getenv("SSH_TTY") != "" ||
		os.Getenv("SSH_CLIENT") != "" ||
		os.Getenv("SSH_CONNECTION") != ""
}

// isGNUScreen returns true if running in GNU screen.
func isGNUScreen() bool {
	return os.Getenv("STY") != ""
}

// buildOSC52Sequence builds an OSC 52 clipboard escape sequence for the given
// text. When inTmux is true, wraps the sequence in a DCS passthrough.
func buildOSC52Sequence(text string, inTmux bool) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(text))

	if inTmux {
		return fmt.Sprintf("\x1bPtmux;\x1b\x1b]52;c;%s\x07\x1b\\", encoded)
	}

	return fmt.Sprintf("\x1b]52;c;%s\x07", encoded)
}

// copyViaOSC52 copies text using OSC 52 escape sequences.
// This path uses /dev/tty and is Unix-only.
func copyViaOSC52(text string) (err error) {
	seq := buildOSC52Sequence(text, os.Getenv("TMUX") != "")

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

func copyViaNative(text string) error {
	if err := clipboard.WriteAll(text); err != nil {
		return fmt.Errorf("native clipboard copy: %w", err)
	}
	return nil
}
