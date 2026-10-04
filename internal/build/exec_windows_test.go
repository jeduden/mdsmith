//go:build windows

package build

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKill_Windows_NilProcess(t *testing.T) {
	assert.NotPanics(t, func() { afterStart(&exec.Cmd{}).kill(nil) })
}

func TestForceLeader_Windows_NilProcess(t *testing.T) {
	assert.NotPanics(t, afterStart(&exec.Cmd{}).forceLeader)
}

func TestClose_Windows_NoJobIsNoOp(t *testing.T) {
	// afterStart sets up no Job Object for a command that never
	// started, so close has no handle to close.
	assert.NotPanics(t, afterStart(&exec.Cmd{}).close)
}

func TestClose_Windows_ClosesJobOnce(t *testing.T) {
	// close must drop the handle it closed, so a second close cannot
	// close a handle value Windows has since handed to another object.
	job, err := createKillOnCloseJob()
	require.NoError(t, err)
	k := &jobKiller{leaderKill: leaderKill{&exec.Cmd{}}, job: job}
	k.close()
	assert.Zero(t, k.job)
	assert.NotPanics(t, k.close)
}

func TestTimeoutKillAction_Windows(t *testing.T) {
	assert.Equal(t, "sent CTRL_BREAK to process group and terminated its job object, if any", TimeoutKillAction)
}
