//go:build windows

package build

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestForceKillLeader_Windows_NilProcess(t *testing.T) {
	assert.NotPanics(t, func() { forceKillLeader(&exec.Cmd{}) })
}

func TestTimeoutKillAction_Windows(t *testing.T) {
	assert.Equal(t, "sent CTRL_BREAK to process group and terminated its job object, if any", TimeoutKillAction)
}
