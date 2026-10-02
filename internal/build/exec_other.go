//go:build !unix && !windows

package build

import (
	"os"
	"os/exec"
)

// configureProcessGroup is a no-op on targets with neither POSIX process
// groups nor Windows Job Objects (js/wasm, wasip1, plan9). js/wasm and
// wasip1 cannot start a subprocess at all. plan9 has a group primitive,
// the note group (RFNOTEG, then "kill" written to /proc/<pid>/notepg),
// but this file does not use it yet; plan 2610020946 tracks that.
func configureProcessGroup(*exec.Cmd) {}

// afterStart is a no-op on these targets. It returns nil so runRecipe
// installs no cleanup defer.
func afterStart(*exec.Cmd) func() { return nil }

// killGroup terminates only the recipe's leader process: there is no
// group kill on js/wasm or wasip1, and plan9's is not wired up yet. A nil Process (the command never started)
// is a no-op.
func killGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = killLeader(cmd.Process)
}

// killLeader kills one process. It is a var so a js/wasm test, where no
// subprocess can start, can check that killGroup kills the leader.
var killLeader = (*os.Process).Kill
