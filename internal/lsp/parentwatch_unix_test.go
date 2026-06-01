//go:build unix

package lsp

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessAliveSelf(t *testing.T) {
	t.Parallel()
	assert.True(t, processAlive(os.Getpid()), "the current process is alive")
}

func TestProcessAliveReapedChildIsDead(t *testing.T) {
	t.Parallel()
	cmd := exec.Command("sh", "-c", "exit 0")
	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid
	require.NoError(t, cmd.Wait()) // reap so the PID is fully released
	assert.False(t, processAlive(pid), "a reaped child is not alive")
}

func TestProcessAliveNonPositive(t *testing.T) {
	t.Parallel()
	assert.False(t, processAlive(0))
	assert.False(t, processAlive(-1))
}
