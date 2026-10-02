//go:build !unix && !windows && !plan9

package build

import (
	"errors"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigureProcessGroup_Other_NoOp(t *testing.T) {
	cmd := &exec.Cmd{}
	configureProcessGroup(cmd)
	assert.Nil(t, cmd.SysProcAttr)
}

func TestAfterStart_Other_ReturnsNil(t *testing.T) {
	assert.Nil(t, afterStart(&exec.Cmd{}))
}

func TestKillGroup_Other_NilProcess(t *testing.T) {
	assert.NotPanics(t, func() { killGroup(&exec.Cmd{}) })
}

func TestKillGroup_Other_KillsLeaderAndIgnoresError(t *testing.T) {
	// No subprocess can start under js/wasm, so killLeader is stubbed:
	// killGroup must hand it the recipe's leader and swallow its error.
	var got *os.Process
	old := killLeader
	killLeader = func(p *os.Process) error {
		got = p
		return errors.New("kill failed")
	}
	t.Cleanup(func() { killLeader = old })

	cmd := &exec.Cmd{Process: &os.Process{Pid: 42}}
	assert.NotPanics(t, func() { killGroup(cmd) })
	assert.Same(t, cmd.Process, got, "killGroup must kill the leader")
}

func TestForceKillLeader_Other_NilProcess(t *testing.T) {
	assert.NotPanics(t, func() { forceKillLeader(&exec.Cmd{}) })
}

func TestTimeoutKillAction_Other(t *testing.T) {
	assert.Equal(t, "killed recipe process", TimeoutKillAction)
}
