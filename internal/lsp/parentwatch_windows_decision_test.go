//go:build !plan9

package lsp

import (
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

// These exercise the Windows liveness *decision* on every platform
// (including the Linux CI runner), since the golang.org/x/sys/windows
// syscall shim that feeds them only compiles under GOOS=windows and the
// CI matrix is ubuntu-only. Without this, the Windows path would ship
// with zero executed test coverage.

func TestWinAliveFromWait(t *testing.T) {
	t.Parallel()
	// WAIT_OBJECT_0 (0): the process handle is signaled => it has
	// exited => dead.
	assert.False(t, winAliveFromWait(winWaitObject0),
		"a signaled handle means the process exited")
	// WAIT_TIMEOUT (258): not signaled within the 0ms wait => still
	// running => alive.
	assert.True(t, winAliveFromWait(258),
		"WAIT_TIMEOUT means the process is still running")
	// WAIT_FAILED (0xFFFFFFFF): the wait itself failed; treat as alive
	// so a transient failure never reaps a healthy server.
	assert.True(t, winAliveFromWait(0xFFFFFFFF),
		"a failed wait must not be read as a dead process")
}

func TestWinAliveFromOpenErr(t *testing.T) {
	t.Parallel()
	// ERROR_INVALID_PARAMETER (87): no process has this PID => dead.
	assert.False(t, winAliveFromOpenErr(syscall.Errno(87)),
		"ERROR_INVALID_PARAMETER means no such process")
	// ERROR_ACCESS_DENIED (5): the process exists but we lack rights =>
	// alive. This mirrors the unix EPERM-as-alive rule and is the fix
	// for the spurious-shutdown asymmetry.
	assert.True(t, winAliveFromOpenErr(syscall.Errno(5)),
		"ERROR_ACCESS_DENIED means the process exists; treat as alive")
	// Any other failure (transient, sandbox, resource pressure): be
	// conservative and treat as alive rather than self-terminate a live
	// server.
	assert.True(t, winAliveFromOpenErr(syscall.Errno(1450)),
		"an unknown OpenProcess error must not reap a live server")
}
