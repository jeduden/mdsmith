//go:build unix

package main_test

import "syscall"

// canProbeProcess reports that unixProcessAlive is a real probe here.
const canProbeProcess = true

// unixProcessAlive reports whether a process with the given PID exists,
// using signal 0 (existence probe). EPERM still implies the process is
// alive (we just may not be allowed to signal it).
func unixProcessAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
