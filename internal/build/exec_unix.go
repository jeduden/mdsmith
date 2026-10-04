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
// fires, which is killGroup.
func killGroupUntil(cmd *exec.Cmd, force <-chan struct{}) {
	if cmd.Process == nil {
		return
	}
	pgid := cmd.Process.Pid // Setpgid made pgid == leader pid
	_ = signalGroup(pgid, syscall.SIGTERM)

	// Wait for the group to drain, polling with signal 0 (existence probe).
	// One ticker serves every poll instead of a new timer per iteration.
	poll := time.NewTicker(50 * time.Millisecond)
	defer poll.Stop()
	deadline := time.Now().Add(gracePeriod)
	for time.Now().Before(deadline) {
		if signalGroup(pgid, 0) != nil {
			return // group is gone
		}
		select {
		case <-force:
			deadline = time.Now() // escalated: skip the rest of the grace
		case <-poll.C:
		}
	}
	_ = signalGroup(pgid, syscall.SIGKILL)
}

// signalGroup sends sig to the process group pgid. It returns the syscall
// error (nil on success); callers use a sig of 0 to probe whether the
// group still exists.
func signalGroup(pgid int, sig syscall.Signal) error {
	return syscall.Kill(-pgid, sig)
}
