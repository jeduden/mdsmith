//go:build js || wasip1

package build

import (
	"os"
	"os/exec"
)

// configureProcessGroup is a no-op on js/wasm and wasip1, which have
// neither POSIX process groups nor Windows Job Objects and cannot start
// a subprocess at all. plan9 has its own file, exec_plan9.go.
func configureProcessGroup(*exec.Cmd) {}

// afterStart is a no-op on these targets. It returns nil so runRecipe
// installs no cleanup defer.
func afterStart(*exec.Cmd) func() { return nil }

// killGroup terminates only the recipe's leader process: there is no
// group kill on js/wasm or wasip1. A nil Process (the command never
// started) is a no-op.
func killGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = killLeader(cmd.Process)
}

// killLeader kills one process. It is a var so a js/wasm test, where no
// subprocess can start, can check that killGroup kills the leader.
var killLeader = (*os.Process).Kill
