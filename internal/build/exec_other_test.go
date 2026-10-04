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

func TestAfterStart_Other_CloseIsNoOp(t *testing.T) {
	assert.NotPanics(t, func() { afterStart(&exec.Cmd{}).close() })
}

func TestKill_Other_NilProcess(t *testing.T) {
	assert.NotPanics(t, func() { afterStart(&exec.Cmd{}).kill() })
}

func TestKill_Other_KillsLeaderAndIgnoresError(t *testing.T) {
	// No subprocess can start under js/wasm, so killLeader is stubbed:
	// kill must hand it the recipe's leader and swallow its error.
	var got *os.Process
	old := killLeader
	killLeader = func(p *os.Process) error {
		got = p
		return errors.New("kill failed")
	}
	t.Cleanup(func() { killLeader = old })

	cmd := &exec.Cmd{Process: &os.Process{Pid: 42}}
	assert.NotPanics(t, afterStart(cmd).kill)
	assert.Same(t, cmd.Process, got, "kill must kill the leader")
}

func TestForceKillLeader_Other_NilProcess(t *testing.T) {
	assert.NotPanics(t, func() { forceKillLeader(&exec.Cmd{}) })
}

func TestTimeoutKillAction_Other(t *testing.T) {
	assert.Equal(t, "killed recipe process", TimeoutKillAction)
}
