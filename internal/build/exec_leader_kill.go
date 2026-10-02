//go:build unix || windows

package build

import "os/exec"

// forceKillLeader kills only the recipe's leader with a kill it cannot
// catch or refuse: (*os.Process).Kill sends SIGKILL on Unix and calls
// TerminateProcess on Windows. runRecipe uses it when the group kill
// left the leader running. A nil Process (the command never started)
// is a no-op. plan9 (exec_plan9.go) and the other targets
// (exec_other.go) have their own.
func forceKillLeader(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
