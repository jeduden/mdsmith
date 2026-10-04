//go:build !plan9

package build

import (
	"os"
	"os/exec"
)

// killCmdLeader kills cmd's leader process with killLeader and ignores
// its error (the leader may already have exited). It is the shared
// forceLeader body on Unix and Windows, and the whole kill on targets
// with no group (exec_other.go). It is not built on plan9 (see
// killLeader). A nil Process (the command never started) is a no-op.
func killCmdLeader(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = killLeader(cmd.Process)
}

// killLeader kills one process: SIGKILL on Unix, TerminateProcess on
// Windows. This file builds everywhere but plan9: there
// (*os.Process).Kill posts a "kill" note the leader can catch, so
// exec_plan9.go kills through the ctl file (forceKillLeader) and must
// not reach this one. It is a var so a test can check which
// process a killer kills without killing one, as on js/wasm, where
// none can start.
var killLeader = (*os.Process).Kill
