//go:build !plan9

package build

import (
	"errors"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

// stubKillLeader swaps killLeader for one test with a stub that records
// the process it is asked to kill and fails, so a caller that must
// ignore the error is checked too. It returns where the process is
// recorded: nil until a kill.
func stubKillLeader(t *testing.T) **os.Process {
	t.Helper()
	var got *os.Process
	old := killLeader
	killLeader = func(p *os.Process) error {
		got = p
		return errors.New("kill failed")
	}
	t.Cleanup(func() { killLeader = old })
	return &got
}

func TestKillCmdLeader_NilProcessIsNoOp(t *testing.T) {
	got := stubKillLeader(t)
	assert.NotPanics(t, func() { killCmdLeader(&exec.Cmd{}) })
	assert.Nil(t, *got, "a command that never started has no leader to kill")
}

func TestKillCmdLeader_KillsLeaderAndIgnoresError(t *testing.T) {
	got := stubKillLeader(t)
	cmd := &exec.Cmd{Process: &os.Process{Pid: 42}}
	assert.NotPanics(t, func() { killCmdLeader(cmd) })
	assert.Same(t, cmd.Process, *got)
}

func TestLeaderKill_ForceLeaderKillsLeader(t *testing.T) {
	got := stubKillLeader(t)
	cmd := &exec.Cmd{Process: &os.Process{Pid: 42}}
	assert.NotPanics(t, func() { leaderKill{cmd}.forceLeader() })
	assert.Same(t, cmd.Process, *got)
}
