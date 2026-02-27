//go:build !windows

package main

import (
	"os"
	"syscall"
)

// isDaemonAlive checks whether a process with the given PID is running
// by sending signal 0 (no-op signal that checks process existence).
func isDaemonAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

// daemonSysProcAttr returns SysProcAttr for detaching the child from the
// parent's process group (Setpgid), so the daemon survives parent exit.
func daemonSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}
