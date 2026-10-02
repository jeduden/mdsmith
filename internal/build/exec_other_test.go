//go:build !unix && !windows

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

func TestAfterStart_Other_ReturnsNil(t *testing.T) {
	assert.Nil(t, afterStart(&exec.Cmd{}))
}

func TestKillGroup_Other_NilProcess(t *testing.T) {
	assert.NotPanics(t, func() { killGroup(&exec.Cmd{}) })
}

func TestKillGroup_Other_IgnoresKillError(t *testing.T) {
	// A zero os.Process makes Kill return "process not initialized";
	// killGroup must swallow it rather than panic. No subprocess can
	// start under js/wasm, so a live leader kill is not testable here.
	cmd := &exec.Cmd{Process: &os.Process{}}
	assert.NotPanics(t, func() { killGroup(cmd) })
}
