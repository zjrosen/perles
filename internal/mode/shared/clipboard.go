// Package shared provides common utilities shared between mode controllers.
package shared

import (
	"os/exec"
	"runtime"
)

// Clipboard defines the interface for clipboard operations.
type Clipboard interface {
	Copy(text string) error
}

// SystemClipboard implements Clipboard using the system clipboard.
type SystemClipboard struct{}

// MockClipboard is a no-op clipboard for testing.
type MockClipboard struct{}

// Copy is a no-op that always succeeds.
func (MockClipboard) Copy(string) error { return nil }

// Copy copies text to the system clipboard.
func (SystemClipboard) Copy(text string) error {
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
