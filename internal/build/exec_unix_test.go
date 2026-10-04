//go:build unix

package build

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
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
	killGroupFn = func(cmd *exec.Cmd, _ <-chan struct{}) bool {
		pgid = cmd.Process.Pid
		fn(cmd)
		return false
	}
	reapWait = 100 * time.Millisecond
	t.Cleanup(func() {
		killGroupFn, reapWait = oldKill, oldReap
		if pgid > 0 {
			_ = signalGroup(pgid, syscall.SIGKILL)
		}
	})
}

func TestRunRecipe_GroupKillThatMissesLeaderStillReturns(t *testing.T) {
	// Models Windows with no Job Object and a recipe that ignores
	// CTRL_BREAK: the group kill leaves the leader running. runRecipe
	// must kill the leader itself after reapWait, not wait forever.
	// That direct kill must go through forceKillLeader: on plan9
	// (*os.Process).Kill posts a note the leader can catch, so a direct
	// Process.Kill there would leave it running.
	stubKillGroup(t, func(*exec.Cmd) {})
	var forced atomic.Bool
	old := forceKillLeaderFn
	forceKillLeaderFn = func(cmd *exec.Cmd) {
		forced.Store(true)
		forceKillLeader(cmd)
	}
	t.Cleanup(func() { forceKillLeaderFn = old })
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
	assert.True(t, forced.Load(), "the reap fallback must use forceKillLeader")
}

func TestForceKillLeader_Unix_NilProcess(t *testing.T) {
	assert.NotPanics(t, func() { forceKillLeader(&exec.Cmd{}) })
}

func TestRunRecipe_SurvivorHoldingPipeDoesNotBlock(t *testing.T) {
	// Models a kill that reaches only the leader (plan9 when afterStart
	// read no noteid, so neither notepg nor the sweep can find the
	// group, or a child that left the note group):
	// a background child keeps the captured stdout pipe open, so
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
	// A line a survivor prints after runRecipe gave up on it must not
	// reach the caller's writer: Build has already closed its log and
	// moved on by then. SIGPIPE is ignored so the survivor outlives its
	// write to the closed pipe and can touch the marker.
	stubKillGroup(t, func(cmd *exec.Cmd) { _ = cmd.Process.Kill() })
	marker := filepath.Join(t.TempDir(), "printed")
	script := writeScript(t, t.TempDir(), "late.sh",
		`(trap '' PIPE; sleep 1; echo late; : > "`+marker+`") & sleep 5`)

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
	// Give a still-running copy goroutine time to forward the line.
	time.Sleep(200 * time.Millisecond)
	assert.NotContains(t, out.String(), "late")
}

func TestRunRecipe_AbandonedSurvivorPipeIsClosed(t *testing.T) {
	// Once runRecipe gives up on a survivor, it must close its read end
	// of the captured pipe rather than leave a copy goroutine and the
	// fd alive for as long as the survivor runs: the survivor's next
	// write then fails with EPIPE (SIGPIPE is ignored so it can tell).
	stubKillGroup(t, func(cmd *exec.Cmd) { _ = cmd.Process.Kill() })
	dir := t.TempDir()
	okMark, failMark := filepath.Join(dir, "ok"), filepath.Join(dir, "failed")
	script := writeScript(t, t.TempDir(), "epipe.sh",
		`(trap '' PIPE; sleep 1; if echo late; then : > "`+okMark+
			`"; else : > "`+failMark+`"; fi) & sleep 5`)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, timedOut, err := runRecipe(ctx, runOpts{
		argv:    []string{script},
		dir:     t.TempDir(),
		defExec: defaultExecConfig(),
		stdout:  &lockedBuffer{},
	})
	require.Error(t, err)
	require.True(t, timedOut)

	require.Eventually(t, func() bool {
		_, okErr := os.Stat(okMark)
		_, failErr := os.Stat(failMark)
		return okErr == nil || failErr == nil
	}, 5*time.Second, 20*time.Millisecond, "survivor should try to write")
	assert.FileExists(t, failMark, "the survivor's write must hit a closed pipe")
}

func TestRunRecipe_SurvivorCostsOneReapWait(t *testing.T) {
	// The leader dies at the group kill; only a background child holds
	// the pipe. Waiting for the leader and draining output share one
	// reapWait: a second wait after a pointless leader kill would add
	// a full reapWait to every such timeout.
	stubKillGroup(t, func(cmd *exec.Cmd) { _ = cmd.Process.Kill() })
	reapWait = time.Second
	script := writeScript(t, t.TempDir(), "daemon.sh", `sleep 5 & sleep 5`)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
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
	assert.Less(t, time.Since(start), 1800*time.Millisecond)
}

func TestExitCodeOf_ExitError(t *testing.T) {
	err := exec.Command("/bin/sh", "-c", "exit 7").Run()
	require.Error(t, err)
	assert.Equal(t, 7, exitCodeOf(err))
	assert.Equal(t, 7, exitCodeOf(fmt.Errorf("wrapped: %w", err)))
}

func TestRunRecipe_LeaderExitedChildHoldsPipeTimesOut(t *testing.T) {
	// The leader exits at once, but a background child it left in the
	// group keeps the captured stdout pipe open. The deadline still
	// applies to the drain: runRecipe must kill the group (taking the
	// child down), report a timeout, and return promptly.
	old := gracePeriod
	gracePeriod = 50 * time.Millisecond
	t.Cleanup(func() { gracePeriod = old })
	stage := t.TempDir()
	pidFile := filepath.Join(stage, "child.pid")
	script := writeScript(t, t.TempDir(), "orphan.sh",
		`sleep 30 & echo $! > "`+pidFile+`"; echo started; exit 0`)

	out := &lockedBuffer{}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	code, timedOut, err := runRecipe(ctx, runOpts{
		argv:    []string{script},
		dir:     stage,
		defExec: defaultExecConfig(),
		stdout:  out,
	})
	require.ErrorContains(t, err, "recipe timed out")
	assert.True(t, timedOut)
	assert.Equal(t, -1, code, "a leader that exited 0 carries no ExitError")
	assert.Less(t, time.Since(start), 3*time.Second)
	assert.Contains(t, out.String(), "started")

	b, rerr := os.ReadFile(pidFile)
	require.NoError(t, rerr)
	pid, perr := parsePID(strings.TrimSpace(string(b)))
	require.NoError(t, perr)
	assert.Eventually(t, func() bool { return !processAlive(pid) },
		5*time.Second, 50*time.Millisecond, "the group kill must reach the child")
}

func TestTimeoutKillAction_Unix(t *testing.T) {
	assert.Equal(t, "sent SIGTERM to process group", TimeoutKillAction)
}

func TestRunRecipe_ForceKillSkipsGrace(t *testing.T) {
	// A second interrupt closes the force channel: a recipe that ignores
	// SIGTERM must get SIGKILL then, not after the whole grace period.
	old := gracePeriod
	gracePeriod = 20 * time.Second
	t.Cleanup(func() { gracePeriod = old })

	script := writeScript(t, t.TempDir(), "ignore.sh", `trap '' TERM; sleep 60`)
	force := make(chan struct{})
	ctx, cancel := context.WithCancel(WithForceKill(context.Background(), force))
	defer cancel()
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
		time.Sleep(300 * time.Millisecond)
		close(force)
	}()
	start := time.Now()
	_, timedOut, err := runRecipe(ctx, runOpts{
		argv:    []string{script},
		dir:     t.TempDir(),
		defExec: defaultExecConfig(),
	})
	require.Error(t, err)
	assert.True(t, timedOut, "the kill path ran")
	assert.ErrorIs(t, err, context.Canceled)
	assert.Less(t, time.Since(start), 10*time.Second, "force must cut the SIGTERM grace short")
	assert.ErrorIs(t, err, ErrForceKilled, "the report must name the SIGKILL")
}

func TestRunRecipe_CancelWithinGraceIsNotForceKilled(t *testing.T) {
	// A recipe that exits on SIGTERM never reaches the escalation, even
	// when a second interrupt arrives later.
	script := writeScript(t, t.TempDir(), "plain.sh", `exec sleep 60`)
	force := make(chan struct{})
	ctx, cancel := context.WithCancel(WithForceKill(context.Background(), force))
	defer cancel()
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()
	_, _, err := runRecipe(ctx, runOpts{argv: []string{script}, dir: t.TempDir(), defExec: defaultExecConfig()})
	close(force)
	require.ErrorIs(t, err, context.Canceled)
	assert.NotErrorIs(t, err, ErrForceKilled)
}

func TestKillGroupUntil_NilProcess(t *testing.T) {
	force := make(chan struct{})
	close(force)
	assert.False(t, killGroupUntil(&exec.Cmd{}, force))
}
