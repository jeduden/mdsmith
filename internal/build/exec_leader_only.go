//go:build !unix && !plan9

package build

import (
	"os"
	"os/exec"
)

// leaderKiller is a groupKiller that ends only the recipe's leader. It
// is afterStart's killer on targets with no group to kill
// (exec_other.go), and the killer for a hook (sharedGroup) on Windows
// and those targets. There is no grace period, so kill ignores the
// WithForceKill channel and reports false.
type leaderKiller struct{ leaderKill }

// sharedGroupKiller returns the killer for a hook, which stays in
// mdsmith's own group: an uncatchable kill of its leader alone, at
// once, with no Job Object.
func sharedGroupKiller(cmd *exec.Cmd) groupKiller { return leaderKiller{leaderKill{cmd}} }

// kill terminates only the recipe's leader with killLeader, which it
// cannot catch. It ignores the error, as the leader may already have
// exited. A nil Process (the command never started) is a no-op.
func (k leaderKiller) kill(<-chan struct{}) bool {
	if k.cmd.Process != nil {
		_ = killLeader(k.cmd.Process)
	}
	return false
}

// close has nothing to release.
func (leaderKiller) close() {}

// killLeader kills one process: TerminateProcess on Windows, and the
// platform's kill on targets with no group. This file builds on neither
// Unix, whose killers signal a group or SIGTERM a hook first, nor
// plan9, where (*os.Process).Kill posts a "kill" note the leader can
// catch, so exec_plan9.go kills through the ctl file instead. It is a
// var so a test can check which process a killer kills without killing
// one, as on js/wasm, where none can start.
var killLeader = (*os.Process).Kill
