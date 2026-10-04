//go:build !unix && !plan9

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

func TestLeaderKiller_NilProcessIsNoOp(t *testing.T) {
	got := stubKillLeader(t)
	assert.False(t, leaderKiller{leaderKill{&exec.Cmd{}}}.kill(nil))
	assert.Nil(t, *got, "a command that never started has no leader to kill")
}

func TestLeaderKiller_KillsLeaderAndIgnoresError(t *testing.T) {
	got := stubKillLeader(t)
	cmd := &exec.Cmd{Process: &os.Process{Pid: 42}}
	assert.NotPanics(t, func() { leaderKiller{leaderKill{cmd}}.kill(nil) })
	assert.Same(t, cmd.Process, *got)
}

func TestLeaderKiller_ForceDoesNotEscalate(t *testing.T) {
	// No grace period to cut short: a closed force changes nothing and
	// kill reports false, even for a command that never started.
	force := make(chan struct{})
	close(force)
	assert.False(t, leaderKiller{leaderKill{&exec.Cmd{}}}.kill(force))
}

func TestSharedGroupKiller_KillsLeaderAtOnce(t *testing.T) {
	// A hook is killed at once through killLeader, so nothing escalates.
	got := stubKillLeader(t)
	cmd := &exec.Cmd{Process: &os.Process{Pid: 42}}
	k := sharedGroupKiller(cmd)
	assert.False(t, k.kill(nil))
	assert.Same(t, cmd.Process, *got, "kill must kill the hook's leader")
	assert.NotPanics(t, k.close)
}
