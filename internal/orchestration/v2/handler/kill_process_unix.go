//go:build !windows

package handler

import "syscall"

// killProcess forcefully terminates a process by PID using SIGKILL.
// This is the Unix implementation.
func killProcess(pid int) error {
	return syscall.Kill(pid, syscall.SIGKILL)
}
