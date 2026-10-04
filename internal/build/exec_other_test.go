//go:build !unix && !windows && !plan9

package build

import (
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
	assert.NotPanics(t, func() { afterStart(&exec.Cmd{}).kill(nil) })
}

func TestKill_Other_KillsLeaderAndIgnoresError(t *testing.T) {
	// No subprocess can start under js/wasm, so killLeader is stubbed:
	// kill must hand it the recipe's leader and swallow its error.
	got := stubKillLeader(t)
	cmd := &exec.Cmd{Process: &os.Process{Pid: 42}}
	assert.NotPanics(t, func() { afterStart(cmd).kill(nil) })
	assert.Same(t, cmd.Process, *got, "kill must kill the leader")
}

func TestForceLeader_Other_NilProcess(t *testing.T) {
	assert.NotPanics(t, afterStart(&exec.Cmd{}).forceLeader)
}

func TestForceLeader_Other_KillsLeader(t *testing.T) {
	got := stubKillLeader(t)
	cmd := &exec.Cmd{Process: &os.Process{Pid: 42}}
	assert.NotPanics(t, afterStart(cmd).forceLeader)
	assert.Same(t, cmd.Process, *got)
}

func TestTimeoutKillAction_Other(t *testing.T) {
	assert.Equal(t, "killed recipe process", TimeoutKillAction)
}
