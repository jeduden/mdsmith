//go:build !unix && !plan9

package build

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
