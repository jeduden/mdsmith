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

func TestKillLeaderUntil_NonUnix_NilProcess(t *testing.T) {
	// No grace period: a hook is killed at once, so nothing escalates.
	force := make(chan struct{})
	close(force)
	assert.False(t, killLeaderUntil(&exec.Cmd{}, force))
}
