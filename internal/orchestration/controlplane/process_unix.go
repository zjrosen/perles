//go:build !windows

package controlplane

import (
	"errors"
	"os"
	"syscall"
)

// isProcessAlive checks if a process with the given PID is still running.
// On Unix, we send signal 0 to check if the process exists.
func isProcessAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds. Send signal 0 to check if alive.
	err = process.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	// EPERM means process exists but we don't have permission to signal it
	// ESRCH means no such process
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno == syscall.EPERM
	}
	return false
}
