//go:build plan9

package build

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

// configureProcessGroup starts the recipe in its own note group. rfork
// with RFNOTEG makes the child lead a new group, so a note posted to it
// reaches every process the recipe spawns afterwards.
func configureProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Rfork: syscall.RFNOTEG}
}

// afterStart is a no-op on plan9. It returns nil so runRecipe installs
// no cleanup defer.
func afterStart(*exec.Cmd) func() { return nil }

// notePgPath returns the control file whose "kill" write posts a kill
// note to the whole note group led by pid. It is a var so a test can
// point it at a path that fails.
var notePgPath = func(pid int) string {
	return "/proc/" + strconv.Itoa(pid) + "/notepg"
}

// killGroup kills the recipe's whole note group by writing "kill" to
// /proc/<pid>/notepg, so a recipe's background children die with it.
// plan9 has no graceful-then-forced pair, so there is no grace period.
// If the write fails, it falls back to killing only the leader. A nil
// Process (the command never started) is a no-op.
func killGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	if err := postKill(notePgPath(cmd.Process.Pid)); err != nil {
		_ = cmd.Process.Kill()
	}
}

// postKill writes "kill" to the notepg control file at path.
func postKill(path string) error {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	if _, err := f.WriteString("kill"); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
