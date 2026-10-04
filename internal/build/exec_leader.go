//go:build !plan9

package build

import "os/exec"

// leaderKill is embedded in each !plan9 groupKiller to hold the
// recipe's command: pgKiller and leaderTermKiller (Unix), jobKiller
// (Windows), and leaderKiller (exec_leader_only.go). plan9's noteKiller
// kills through the ctl file instead.
type leaderKill struct{ cmd *exec.Cmd }
