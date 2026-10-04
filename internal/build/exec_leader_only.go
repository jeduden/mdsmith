//go:build !unix && !plan9

package build

import "os/exec"

// leaderKiller is a groupKiller that ends only the recipe's leader. It
// is afterStart's killer on targets with no group to kill
// (exec_other.go), and the killer for a hook (sharedGroup) on Windows
// and those targets. There is no grace period, so kill ignores the
// WithForceKill channel and reports false. The kill itself is the
// embedded leaderKill's forceLeader.
type leaderKiller struct{ leaderKill }

// sharedGroupKiller returns the killer for a hook, which stays in
// mdsmith's own group: an uncatchable kill of its leader alone, at
// once, with no Job Object.
func sharedGroupKiller(cmd *exec.Cmd) groupKiller { return leaderKiller{leaderKill{cmd}} }

// kill terminates only the recipe's leader process. A nil Process (the
// command never started) is a no-op.
func (k leaderKiller) kill(<-chan struct{}) bool {
	k.forceLeader()
	return false
}

// close has nothing to release.
func (leaderKiller) close() {}
