//go:build !windows

package api

import (
	"os"
	"syscall"
)

// processAlive reports whether pid refers to a live process.
//
// Signal 0 performs the permission and existence checks without delivering a
// signal, which is the portable way to probe liveness on Unix.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	// EPERM means the process exists but belongs to another user.
	return err == syscall.EPERM
}
