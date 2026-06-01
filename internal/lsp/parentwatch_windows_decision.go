//go:build !plan9

package lsp

import (
	"errors"
	"syscall"
)

// Windows liveness decision logic, kept free of the //go:build windows
// constraint so it is unit-testable on the (ubuntu-only) CI matrix and
// on developer macOS hosts. The thin syscall shim in
// parentwatch_windows.go calls these; nothing here touches
// golang.org/x/sys/windows. The one non-portable dependency is the
// syscall.Errno type, which exists on every GOOS except plan9 (plan9
// uses string errors) — hence the !plan9 tag. plan9 uses the
// processAlive fallback in parentwatch_other.go and never references
// these helpers.

// winWaitObject0 is WAIT_OBJECT_0: WaitForSingleObject returns this when
// the process handle is signaled, i.e. the process has exited.
const winWaitObject0 = 0x00000000

// winErrInvalidParameter is ERROR_INVALID_PARAMETER, the Win32 system
// error OpenProcess returns when no process has the given id. Declared
// here (rather than referencing windows.ERROR_INVALID_PARAMETER) so this
// file builds on every platform.
const winErrInvalidParameter = 87

// winAliveFromWait maps a WaitForSingleObject result to liveness. A
// signaled handle (WAIT_OBJECT_0) means the process exited; anything
// else (WAIT_TIMEOUT, or WAIT_FAILED reported via a non-zero event)
// means it is still running or the probe was inconclusive — in which
// case we err toward "alive" so a transient wait failure never reaps a
// healthy server. This replaces the GetExitCodeProcess(==259) check,
// which cannot distinguish a running process from one that genuinely
// exited with code STILL_ACTIVE (259).
func winAliveFromWait(event uint32) bool {
	return event != winWaitObject0
}

// winAliveFromOpenErr maps an OpenProcess failure to liveness.
// ERROR_INVALID_PARAMETER is Windows' "no process has this id" => dead.
// Every other failure is treated as alive — most importantly
// ERROR_ACCESS_DENIED (the process exists but we lack the rights to open
// it), which mirrors the unix EPERM-as-alive rule so a privilege
// mismatch with the editor host does not trigger a spurious self-exit.
// Erring toward "alive" on any unrecognized error keeps a transient or
// sandbox failure from reaping a healthy server.
func winAliveFromOpenErr(err error) bool {
	return !errors.Is(err, syscall.Errno(winErrInvalidParameter))
}
