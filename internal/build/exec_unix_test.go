//go:build unix

package build

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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
// The stub leaves survivors on purpose, so cleanup SIGKILLs the
// recipe's whole process group (Setpgid made pgid == leader pid):
// an orphan would otherwise keep the test binary's stderr open and
// stall `go test` until it exits.
func stubKillGroup(t *testing.T, fn func(*exec.Cmd)) {
	t.Helper()
	oldKill, oldReap := killGroupFn, reapWait
	pgid := 0
	killGroupFn = func(cmd *exec.Cmd) {
		pgid = cmd.Process.Pid
		fn(cmd)
	}
	reapWait = 100 * time.Millisecond
	t.Cleanup(func() {
		killGroupFn, reapWait = oldKill, oldReap
		if pgid > 0 {
			_ = signalGroup(pgid, syscall.SIGKILL)
		}
	})
}

// lockedBuffer is a strings.Builder safe to read while os/exec's copy
// goroutine may still write to it.
type lockedBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
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
	script := writeScript(t, t.TempDir(), "daemon.sh", `sleep 5 & sleep 5`)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, timedOut, err := runRecipe(ctx, runOpts{
		argv:    []string{script},
		dir:     t.TempDir(),
		defExec: defaultExecConfig(),
		stdout:  &lockedBuffer{},
	})
	require.Error(t, err)
	assert.True(t, timedOut)
	assert.Less(t, time.Since(start), 3*time.Second, "a survivor's open pipe must not block")
}

func TestRunRecipe_AbandonedSurvivorCannotWriteAfterReturn(t *testing.T) {
	// After runRecipe gives up on a survivor that holds the stdout pipe,
	// os/exec's copy goroutine is still running. A line the survivor
	// prints later must not reach the caller's writer: Build has already
	// closed its log and moved on by then.
	stubKillGroup(t, func(cmd *exec.Cmd) { _ = cmd.Process.Kill() })
	marker := filepath.Join(t.TempDir(), "printed")
	script := writeScript(t, t.TempDir(), "late.sh",
		`(sleep 1; echo late; : > "`+marker+`") & sleep 5`)

	out := &lockedBuffer{}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, timedOut, err := runRecipe(ctx, runOpts{
		argv:    []string{script},
		dir:     t.TempDir(),
		defExec: defaultExecConfig(),
		stdout:  out,
	})
	require.Error(t, err)
	require.True(t, timedOut)

	require.Eventually(t, func() bool {
		_, statErr := os.Stat(marker)
		return statErr == nil
	}, 5*time.Second, 20*time.Millisecond, "survivor should print after runRecipe returns")
	// Give the copy goroutine time to forward the line it read.
	time.Sleep(200 * time.Millisecond)
	assert.NotContains(t, out.String(), "late")
}
