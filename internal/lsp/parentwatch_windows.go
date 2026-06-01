//go:build windows

package lsp

import "golang.org/x/sys/windows"

// processAlive reports whether the process with the given PID is still
// running. It opens a handle with SYNCHRONIZE | PROCESS_QUERY_LIMITED_-
// INFORMATION and waits on it with a zero timeout: a signaled handle
// (WAIT_OBJECT_0) means the process has exited. SYNCHRONIZE is required
// for the wait — PROCESS_QUERY_LIMITED_INFORMATION alone does not grant
// it.
//
// This deliberately avoids GetExitCodeProcess: its STILL_ACTIVE (259)
// sentinel cannot distinguish a running process from one that genuinely
// exited with code 259, which would pin processAlive at true forever and
// leak the orphan the watchdog exists to reap.
//
// The liveness decisions live in winAliveFromWait / winAliveFromOpenErr
// (parentwatch_windows_decision.go) so they are unit-tested on the
// ubuntu-only CI matrix, where this file is never compiled or run.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	const access = windows.SYNCHRONIZE | windows.PROCESS_QUERY_LIMITED_INFORMATION
	h, err := windows.OpenProcess(access, false, uint32(pid))
	if err != nil {
		return winAliveFromOpenErr(err)
	}
	defer windows.CloseHandle(h)
	event, err := windows.WaitForSingleObject(h, 0)
	if err != nil {
		// WAIT_FAILED: the wait itself errored. Be conservative and
		// treat the process as alive rather than reap a healthy server.
		return true
	}
	return winAliveFromWait(event)
}
