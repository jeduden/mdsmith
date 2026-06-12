//go:build unix

package build

import (
	"os/exec"
	"syscall"
	"time"
)

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
func killGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	pgid := cmd.Process.Pid // Setpgid made pgid == leader pid
	_ = signalGroup(pgid, syscall.SIGTERM)

	// Wait for the group to drain, polling with signal 0 (existence probe).
	deadline := time.Now().Add(gracePeriod)
	for time.Now().Before(deadline) {
		if signalGroup(pgid, 0) != nil {
			return // group is gone
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = signalGroup(pgid, syscall.SIGKILL)
}

// signalGroup sends sig to the process group pgid. It returns the syscall
// error (nil on success); callers use a sig of 0 to probe whether the
// group still exists.
func signalGroup(pgid int, sig syscall.Signal) error {
	return syscall.Kill(-pgid, sig)
}
