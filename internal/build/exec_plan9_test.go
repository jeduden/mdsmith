//go:build plan9

package build

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeRC writes an executable rc script; plan9 has no /bin/sh.
func writeRC(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte("#!/bin/rc\n"+body+"\n"), 0o755))
	return p
}

// procAlive reports whether /proc/<pid> still exists.
func procAlive(pid int) bool {
	_, err := os.Stat("/proc/" + strconv.Itoa(pid))
	return err == nil
}

func TestConfigureProcessGroup_Plan9_SetsRFNOTEG(t *testing.T) {
	cmd := &exec.Cmd{}
	configureProcessGroup(cmd)
	require.NotNil(t, cmd.SysProcAttr)
	assert.NotZero(t, cmd.SysProcAttr.Rfork&syscall.RFNOTEG)
}

func TestAfterStart_Plan9_ReturnsNil(t *testing.T) {
	assert.Nil(t, afterStart(&exec.Cmd{}))
}

func TestKillGroup_Plan9_NilProcess(t *testing.T) {
	assert.NotPanics(t, func() { killGroup(&exec.Cmd{}) })
}

func TestKillGroup_Plan9_FallsBackToLeaderKill(t *testing.T) {
	// When the notepg write fails, killGroup must still kill the leader.
	old := notePgPath
	notePgPath = func(int) string { return "/no/such/notepg" }
	t.Cleanup(func() { notePgPath = old })

	script := writeRC(t, t.TempDir(), "slow.rc", `sleep 120`)
	cmd := exec.Command(script)
	configureProcessGroup(cmd)
	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	killGroup(cmd)
	assert.Eventually(t, func() bool { return !procAlive(pid) },
		6*time.Second, 100*time.Millisecond, "leader should be killed")
}

func TestRunRecipe_Plan9_TimeoutKillsNoteGroup(t *testing.T) {
	stage := t.TempDir()
	pidFile := filepath.Join(stage, "child.pid")
	// The parent forks a long-lived child, records its pid, then sleeps.
	// On timeout the whole note group must die, including the child.
	body := `sleep 120 &
echo $apid > ` + pidFile + `
sleep 120`
	script := writeRC(t, t.TempDir(), "spawn.rc", body)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_, timedOut, err := runRecipe(ctx, runOpts{
		argv:    []string{script},
		dir:     stage,
		exec:    ExecConfig{},
		defExec: defaultExecConfig(),
	})
	require.Error(t, err)
	assert.True(t, timedOut)

	var childPID int
	require.Eventually(t, func() bool {
		b, rerr := os.ReadFile(pidFile)
		if rerr != nil {
			return false
		}
		n, perr := strconv.Atoi(strings.TrimSpace(string(b)))
		childPID = n
		return perr == nil
	}, 6*time.Second, 50*time.Millisecond, "child pid should be recorded")

	assert.Eventually(t, func() bool { return !procAlive(childPID) },
		6*time.Second, 100*time.Millisecond, "spawned child should not be orphaned")
}
