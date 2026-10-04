//go:build windows

package build

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestForceLeader_Windows_NilProcess(t *testing.T) {
	assert.NotPanics(t, afterStart(&exec.Cmd{}).forceLeader)
}

func TestTimeoutKillAction_Windows(t *testing.T) {
	assert.Equal(t, "sent CTRL_BREAK to process group and terminated its job object, if any", TimeoutKillAction)
}
