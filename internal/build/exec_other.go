//go:build !unix && !windows && !plan9

package build

import "os/exec"

// configureProcessGroup is a no-op on js/wasm and wasip1, which have
// neither POSIX process groups nor Windows Job Objects and cannot start
// a subprocess at all. plan9 has its own file, exec_plan9.go. The tag
// is the complement of the other exec files, not `js || wasip1`, so any
// other GOOS (zos, say) still compiles, killing only the leader.
func configureProcessGroup(*exec.Cmd) {}

// TimeoutKillAction names, for the timeout report, the kill a timed-out
// recipe gets on this platform.
const TimeoutKillAction = "killed recipe process"

// afterStart holds no state on these targets; it returns a killer for
// the leader alone (leaderKiller, exec_leader_only.go).
func afterStart(cmd *exec.Cmd) groupKiller { return leaderKiller{leaderKill{cmd}} }
