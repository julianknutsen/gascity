//go:build windows

package main

import "syscall"

// isDaemonAlive is not supported on Windows.
func isDaemonAlive(_ int) bool {
	return false
}

// daemonSysProcAttr returns nil on Windows (no process group detachment).
func daemonSysProcAttr() *syscall.SysProcAttr {
	return nil
}
