//go:build !unix && !windows && !plan9

package build

import (
	"os"
	"os/exec"
)

// configureProcessGroup is a no-op on js/wasm and wasip1, which have
// neither POSIX process groups nor Windows Job Objects and cannot start
// a subprocess at all. plan9 has its own file, exec_plan9.go. The tag
// is the complement of the other exec files, not `js || wasip1`, so any
// other GOOS (zos, say) still compiles, killing only the leader.
func configureProcessGroup(*exec.Cmd) {}

// TimeoutKillAction names, for the timeout report, the kill a timed-out
// recipe gets on this platform.
const TimeoutKillAction = "killed recipe process"

// afterStart is a no-op on these targets. It returns nil so runRecipe
// installs no cleanup defer.
func afterStart(*exec.Cmd) func() { return nil }

// killGroup terminates only the recipe's leader process: there is no
// group kill on these targets. A nil Process (the command never
// started) is a no-op.
func killGroup(cmd *exec.Cmd) { forceKillLeader(cmd) }

// forceKillLeader kills the recipe's leader process with killLeader.
// runRecipe also uses it when the group kill left the leader running.
// A nil Process is a no-op.
func forceKillLeader(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = killLeader(cmd.Process)
}

// killLeader kills one process. It is a var so a js/wasm test, where no
// subprocess can start, can check that killGroup kills the leader.
var killLeader = (*os.Process).Kill
