//go:build windows

package controlplane

import (
	"golang.org/x/sys/windows"
)

// isProcessAlive checks if a process with the given PID is still running.
// On Windows, we use OpenProcess to check if the process exists.
func isProcessAlive(pid int) bool {
	// PROCESS_QUERY_LIMITED_INFORMATION is the minimum access right needed
	// to check if a process exists.
	const PROCESS_QUERY_LIMITED_INFORMATION = 0x1000

	handle, err := windows.OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		// Process doesn't exist or we don't have access
		return false
	}
	defer windows.CloseHandle(handle)

	// Check if process has exited
	var exitCode uint32
	err = windows.GetExitCodeProcess(handle, &exitCode)
	if err != nil {
		return false
	}

	// STILL_ACTIVE (259) means the process is still running
	return exitCode == 259
}
