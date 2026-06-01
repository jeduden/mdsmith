//go:build unix

package lsp

import (
	"errors"
	"syscall"
)

// processAlive reports whether the process with the given PID is still
// running. Signal 0 runs the kernel's existence/permission check without
// delivering a signal: nil means the process exists, EPERM means it
// exists but we may not signal it (still alive), and ESRCH means it is
// gone.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
