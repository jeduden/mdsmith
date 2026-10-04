//go:build !plan9

package build

import (
	"os"
	"os/exec"
)

// leaderKill is embedded in each !plan9 groupKiller to supply its
// forceLeader: pgKiller (Unix), jobKiller (Windows), and leaderKiller
// (exec_other.go). plan9's noteKiller has its own, as kill already
// ends in an uncatchable leader kill there.
type leaderKill struct{ cmd *exec.Cmd }

// forceLeader kills only the recipe's leader with killLeader: SIGKILL
// on Unix, TerminateProcess on Windows, neither of which the leader can
// catch. It is the shared leader kill on Unix and Windows, and the
// whole kill on targets with no group (exec_other.go). It ignores the
// error, as the leader may already have exited. A nil Process (the
// command never started) is a no-op.
func (k leaderKill) forceLeader() {
	if k.cmd.Process == nil {
		return
	}
	_ = killLeader(k.cmd.Process)
}

// killLeader kills one process: SIGKILL on Unix, TerminateProcess on
// Windows. This file builds everywhere but plan9: there
// (*os.Process).Kill posts a "kill" note the leader can catch, so
// exec_plan9.go kills through the ctl file (forceKillLeader) and must
// not reach this one. It is a var so a test can check which
// process a killer kills without killing one, as on js/wasm, where
// none can start.
var killLeader = (*os.Process).Kill
