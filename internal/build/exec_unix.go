//go:build unix

package build

import (
	"os/exec"
	"syscall"
	"time"
)

// gracePeriod is how long mdsmith waits after SIGTERM before sending
// SIGKILL to the process group. Only Unix waits, so it lives here. It
// is a var, not a const, so a kill-path test can shorten it.
var gracePeriod = 5 * time.Second

// TimeoutKillAction names, for the timeout report, the kill a timed-out
// recipe gets on this platform.
const TimeoutKillAction = "sent SIGTERM to process group"

// configureProcessGroup puts the recipe in its own process group so a
// timeout can signal the whole group, not just the leader. Setpgid makes
// the child the leader of a new group whose pgid equals its pid.
func configureProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// afterStart is a no-op on Unix; the Job Object equivalent is Windows
// only. It returns nil so runRecipe installs no cleanup defer.
func afterStart(*exec.Cmd) func() { return nil }

// killGroup terminates the recipe's whole process group. It sends
// SIGTERM first, waits up to gracePeriod for the group to exit, then
// sends SIGKILL. Signaling the negative pgid reaches every process in
// the group, so a recipe's background children are killed too.
func killGroup(cmd *exec.Cmd) { killGroupUntil(cmd, nil) }

// killGroupUntil is killGroup with an escape hatch: once force is
// closed (a second CLI interrupt, see WithForceKill), it stops waiting
// out the grace period and sends SIGKILL at once. A nil force never
// fires, which is killGroup. It reports whether force cut the grace
// short while the group was still alive.
func killGroupUntil(cmd *exec.Cmd, force <-chan struct{}) bool {
	if cmd.Process == nil {
		return false
	}
	pgid := cmd.Process.Pid // Setpgid made pgid == leader pid
	return termThenKill(func(sig syscall.Signal) error { return signalGroup(pgid, sig) }, force)
}

// killLeaderUntil is killGroupUntil for a run in mdsmith's own process
// group (a hook, sharedGroup): signalling the group would hit mdsmith,
// so SIGTERM, the grace period, and the SIGKILL reach the leader alone.
// A hook that traps TERM runs its cleanup. The existence probe goes
// through cmd.Process, which fails once runRecipe's Wait reaped the
// leader, so a reused pid is never signalled.
func killLeaderUntil(cmd *exec.Cmd, force <-chan struct{}) bool {
	if cmd.Process == nil {
		return false
	}
	return termThenKill(func(sig syscall.Signal) error { return cmd.Process.Signal(sig) }, force)
}

// termThenKill sends SIGTERM through signal, polls with signal 0 (an
// existence probe) until the target is gone or gracePeriod runs out,
// then sends SIGKILL. A closed force skips what is left of the grace;
// it reports whether force did so while the target was still alive.
func termThenKill(signal func(syscall.Signal) error, force <-chan struct{}) bool {
	_ = signal(syscall.SIGTERM)

	// One ticker serves every poll instead of a new timer per iteration.
	poll := time.NewTicker(50 * time.Millisecond)
	defer poll.Stop()
	deadline := time.Now().Add(gracePeriod)
	forced := false
	for time.Now().Before(deadline) {
		if signal(0) != nil {
			return false // the target is gone
		}
		select {
		case <-force:
			forced = true
			deadline = time.Now() // escalated: skip the rest of the grace
		case <-poll.C:
		}
	}
	_ = signal(syscall.SIGKILL)
	return forced
}

// signalGroup sends sig to the process group pgid. It returns the syscall
// error (nil on success); callers use a sig of 0 to probe whether the
// group still exists.
func signalGroup(pgid int, sig syscall.Signal) error {
	return syscall.Kill(-pgid, sig)
}
