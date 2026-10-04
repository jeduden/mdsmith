//go:build !unix

package build

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKillGroupUntil_NonUnix_NilProcess(t *testing.T) {
	// No grace period to cut short: the adapter is killGroup, which
	// returns at once for a command that never started.
	force := make(chan struct{})
	close(force)
	assert.False(t, killGroupUntil(&exec.Cmd{}, force), "no grace, so nothing escalates")
}
