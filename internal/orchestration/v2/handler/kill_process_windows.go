//go:build windows

package handler

import "os"

// killProcess forcefully terminates a process by PID.
// This is the Windows implementation using os.Process.Kill()
// which calls TerminateProcess on Windows.
func killProcess(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}
