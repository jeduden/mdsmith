//go:build unix

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

// parsePID parses a decimal PID string.
func parsePID(s string) (int, error) {
	return strconv.Atoi(s)
}

// processAlive reports whether a process with the given PID exists. It
// uses signal 0, which performs error checking without delivering a
// signal: ESRCH means the process is gone.
func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

func TestKillGroup_NilProcess(t *testing.T) {
	// A command that never started has a nil Process; killGroup must return
	// immediately rather than dereference it.
	killGroup(&exec.Cmd{})
}

func TestKillGroup_SIGKILLPath(t *testing.T) {
	// A recipe that ignores SIGTERM must still be force-killed: killGroup
	// waits gracePeriod for the polite signal to work, then sends SIGKILL.
	old := gracePeriod
	gracePeriod = 50 * time.Millisecond
	t.Cleanup(func() { gracePeriod = old })

	stage := t.TempDir()
	// trap '' TERM makes the process ignore SIGTERM; only SIGKILL ends it.
	script := writeScript(t, t.TempDir(), "ignore.sh", `trap '' TERM; sleep 60`)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _, err := runRecipe(ctx, runOpts{
		argv:    []string{script},
		dir:     stage,
		exec:    ExecConfig{},
		defExec: defaultExecConfig(),
	})
	require.Error(t, err)
	// With a 50ms grace period and SIGTERM ignored, SIGKILL ends it quickly.
	assert.Less(t, time.Since(start), 5*time.Second, "SIGKILL should be prompt")
}

func TestRunRecipe_TimeoutKillsProcessGroup(t *testing.T) {
	stage := t.TempDir()
	pidFile := filepath.Join(stage, "child.pid")
	// Parent spawns a long-lived child in the background, records its PID,
	// then sleeps. On timeout the whole group must die, including the child.
	body := `sleep 120 & echo $! > "` + pidFile + `"; sleep 120`
	script := writeScript(t, t.TempDir(), "spawn.sh", body)

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_, _, err := runRecipe(ctx, runOpts{
		argv:    []string{script},
		dir:     stage,
		exec:    ExecConfig{},
		defExec: defaultExecConfig(),
	})
	require.Error(t, err)
	assert.Less(t, time.Since(start), 10*time.Second, "kill should be prompt")

	// Give the kernel a moment to reap.
	deadline := time.Now().Add(6 * time.Second)
	var childPID int
	for time.Now().Before(deadline) {
		b, rerr := os.ReadFile(pidFile)
		if rerr == nil {
			if n, perr := parsePID(strings.TrimSpace(string(b))); perr == nil {
				childPID = n
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.NotZero(t, childPID, "child pid should have been recorded")

	// The child must no longer be alive: signal 0 probes existence.
	assert.Eventually(t, func() bool {
		return !processAlive(childPID)
	}, 6*time.Second, 100*time.Millisecond, "spawned child should not be orphaned")
}

// stubKillGroup swaps killGroupFn and shortens reapWait for one test.
func stubKillGroup(t *testing.T, fn func(*exec.Cmd)) {
	t.Helper()
	oldKill, oldReap := killGroupFn, reapWait
	killGroupFn, reapWait = fn, 100*time.Millisecond
	t.Cleanup(func() { killGroupFn, reapWait = oldKill, oldReap })
}

// killRecordedPID kills the process whose PID a recipe wrote to pidFile,
// so a test that leaves a survivor on purpose does not leak it.
func killRecordedPID(t *testing.T, pidFile string) {
	t.Helper()
	t.Cleanup(func() {
		b, err := os.ReadFile(pidFile)
		if err != nil {
			return
		}
		if pid, err := parsePID(strings.TrimSpace(string(b))); err == nil {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})
}

func TestRunRecipe_GroupKillThatMissesLeaderStillReturns(t *testing.T) {
	// Models Windows with no Job Object and a recipe that ignores
	// CTRL_BREAK: the group kill leaves the leader running. runRecipe
	// must kill the leader itself after reapWait, not wait forever.
	stubKillGroup(t, func(*exec.Cmd) {})
	script := writeScript(t, t.TempDir(), "slow.sh", `sleep 5`)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, timedOut, err := runRecipe(ctx, runOpts{
		argv:    []string{script},
		dir:     t.TempDir(),
		defExec: defaultExecConfig(),
	})
	require.Error(t, err)
	assert.True(t, timedOut)
	assert.Less(t, time.Since(start), 3*time.Second, "leader fallback kill should be prompt")
}

func TestRunRecipe_SurvivorHoldingPipeDoesNotBlock(t *testing.T) {
	// Models a target with no group kill (plan9): only the leader dies,
	// and a background child keeps the captured stdout pipe open, so
	// cmd.Wait would block until that child exits. runRecipe must stop
	// waiting after reapWait.
	stubKillGroup(t, func(cmd *exec.Cmd) { _ = cmd.Process.Kill() })
	stage := t.TempDir()
	pidFile := filepath.Join(stage, "child.pid")
	killRecordedPID(t, pidFile)
	script := writeScript(t, t.TempDir(), "daemon.sh",
		`sleep 5 & echo $! > "`+pidFile+`"; sleep 5`)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, timedOut, err := runRecipe(ctx, runOpts{
		argv:    []string{script},
		dir:     stage,
		defExec: defaultExecConfig(),
		stdout:  &strings.Builder{},
	})
	require.Error(t, err)
	assert.True(t, timedOut)
	assert.Less(t, time.Since(start), 3*time.Second, "a survivor's open pipe must not block")
}
