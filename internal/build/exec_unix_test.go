//go:build unix

package build

import (
	"strconv"
	"syscall"
)

// parsePID parses a decimal PID string.
func parsePID(s string) (int, error) {
	return strconv.Atoi(s)
}

// processAlive reports whether a process with the given PID exists. It
// uses signal 0, which performs error checking without delivering a
// signal: ESRCH means the process is gone.
func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
