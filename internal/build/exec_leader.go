//go:build !plan9

package build

import (
	"os"
	"os/exec"
)

// leaderKill is embedded in each !plan9 groupKiller to hold the
// recipe's command: pgKiller and leaderTermKiller (Unix), jobKiller
// (Windows), and leaderKiller (exec_leader_only.go), whose kill is
// forceLeader. plan9's noteKiller kills through the ctl file instead.
type leaderKill struct{ cmd *exec.Cmd }

// forceLeader kills only the recipe's leader with killLeader: SIGKILL
// on Unix, TerminateProcess on Windows, neither of which the leader can
// catch. It is leaderKiller's whole kill: a hook's on Windows and the
// recipe's on targets with no group (exec_other.go). It ignores the
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
