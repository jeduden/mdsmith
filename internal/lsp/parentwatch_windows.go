//go:build windows

package lsp

import "golang.org/x/sys/windows"

// processAlive reports whether the process with the given PID is still
// running. On Windows we open a query handle and read the exit code: an
// exit code of STILL_ACTIVE (259) means the process has not exited.
// Failure to open the handle means the process is gone.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	const stillActive = 259
	return code == stillActive
}
