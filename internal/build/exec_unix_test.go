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

// leaderGone returns a readiness check for deadlineWhen: it holds once
// pidFile holds the leader's pid and that pid is gone (the leader
// exited and runRecipe's Wait reaped it), so a test that pins the
// leader's own exit code never has its deadline fire first.
func leaderGone(pidFile string) func() bool {
	return func() bool {
		b, err := os.ReadFile(pidFile)
		if err != nil {
			return false
		}
		pid, err := parsePID(strings.TrimSpace(string(b)))
		return err == nil && !processAlive(pid)
	}
}

func TestKill_Unix_NilProcess(t *testing.T) {
	// A command that never started has a nil Process; kill must return
	// immediately rather than dereference it.
	assert.NotPanics(t, func() { afterStart(&exec.Cmd{}).kill(nil) })
}

func TestSharedGroupKiller_Unix_NilProcess(t *testing.T) {
	// A hook that never started has a nil Process: nothing to signal.
	assert.False(t, sharedGroupKiller(&exec.Cmd{}).kill(nil))
}

func TestSharedGroupKiller_Unix_CloseIsNoOp(t *testing.T) {
	assert.NotPanics(t, sharedGroupKiller(&exec.Cmd{}).close)
}

func TestKill_Unix_SIGKILLPath(t *testing.T) {
	// A recipe that ignores SIGTERM must still be force-killed: kill
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

// stubGroupKiller swaps afterStartFn for one that wraps the real Unix
// killer: kill runs fn instead, and close is the real close. It also
// shortens reapWait for one test.
// The stub leaves survivors on purpose, so cleanup SIGKILLs the
// recipe's whole process group (Setpgid made pgid == leader pid):
// an orphan would otherwise keep the test binary's stderr open and
// stall `go test` until it exits.
func stubGroupKiller(t *testing.T, fn func(*exec.Cmd)) {
	t.Helper()
	oldStart, oldReap := afterStartFn, reapWait
	pgid := 0
	afterStartFn = func(cmd *exec.Cmd) groupKiller {
		pgid = cmd.Process.Pid
		inner := afterStart(cmd)
		return stubKiller{
			killFn:  func() { fn(cmd) },
			closeFn: inner.close,
		}
	}
	reapWait = 100 * time.Millisecond
	t.Cleanup(func() {
		afterStartFn, reapWait = oldStart, oldReap
		if pgid > 0 {
			_ = signalGroup(pgid, syscall.SIGKILL)
		}
	})
}

func TestRunRecipe_GroupKillThatMissesLeaderStillReturns(t *testing.T) {
	// Models Windows with no Job Object and a recipe that ignores
	// CTRL_BREAK: the group kill leaves the leader running. os/exec's
	// WaitDelay (reapWait) must then kill the leader directly with a
	// kill it cannot catch, not wait forever.
	stubGroupKiller(t, func(*exec.Cmd) {})
	script := writeScript(t, t.TempDir(), "slow.sh", `sleep 5`)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	code, timedOut, err := runRecipe(ctx, runOpts{
		argv:    []string{script},
		dir:     t.TempDir(),
		defExec: defaultExecConfig(),
	})
	require.Error(t, err)
	assert.True(t, timedOut)
	assert.Less(t, time.Since(start), 3*time.Second, "leader fallback kill should be prompt")
	assert.Equal(t, -1, code, "the leader must die of the fallback's signal")
}

func TestClose_Unix_IsNoOp(t *testing.T) {
	// The process group holds no state, so close has nothing to release.
	assert.NotPanics(t, afterStart(&exec.Cmd{}).close)
}

func TestRunRecipe_SurvivorHoldingPipeDoesNotBlock(t *testing.T) {
	// Models a kill that reaches only the leader (plan9 when afterStart
	// read no noteid, so neither notepg nor the sweep can find the
	// group, or a child that left the note group):
	// a background child keeps the captured stdout pipe open, so
	// cmd.Wait would block until that child exits. runRecipe must stop
	// waiting after reapWait.
	stubGroupKiller(t, func(cmd *exec.Cmd) { _ = cmd.Process.Kill() })
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
	stubGroupKiller(t, func(cmd *exec.Cmd) { _ = cmd.Process.Kill() })
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
	stubGroupKiller(t, func(cmd *exec.Cmd) { _ = cmd.Process.Kill() })
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
	stubGroupKiller(t, func(cmd *exec.Cmd) { _ = cmd.Process.Kill() })
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
	leaderPID := filepath.Join(stage, "leader.pid")
	script := writeScript(t, t.TempDir(), "orphan.sh",
		`echo $$ > "`+leaderPID+`"; sleep 30 & echo $! > "`+pidFile+`"; echo started; exit 0`)

	out := &lockedBuffer{}
	ctx := deadlineWhen(t, leaderGone(leaderPID))
	start := time.Now()
	code, timedOut, err := runRecipe(ctx, runOpts{
		argv:    []string{script},
		dir:     stage,
		defExec: defaultExecConfig(),
		stdout:  out,
	})
	require.ErrorContains(t, err, "recipe timed out")
	assert.True(t, timedOut)
	assert.Equal(t, 0, code, "the leader's own exit status, not -1")
	assert.Less(t, time.Since(start), 3*time.Second)
	assert.Contains(t, out.String(), "started")

	b, rerr := os.ReadFile(pidFile)
	require.NoError(t, rerr)
	pid, perr := parsePID(strings.TrimSpace(string(b)))
	require.NoError(t, perr)
	assert.Eventually(t, func() bool { return !processAlive(pid) },
		5*time.Second, 50*time.Millisecond, "the group kill must reach the child")
}

func TestRunRecipe_TimedOutAfterLeaderExitKeepsExitCode(t *testing.T) {
	// The leader exits before the deadline; a child it backgrounded
	// holds the captured pipe past it. The run still times out, and
	// the code is the leader's own exit status: 0 included, which
	// carries no ExitError.
	old := gracePeriod
	gracePeriod = 50 * time.Millisecond
	t.Cleanup(func() { gracePeriod = old })
	for _, want := range []int{0, 2} {
		t.Run(strconv.Itoa(want), func(t *testing.T) {
			leaderPID := filepath.Join(t.TempDir(), "leader.pid")
			script := writeScript(t, t.TempDir(), "exit.sh",
				`echo $$ > "`+leaderPID+`"; sleep 30 & echo started; exit `+strconv.Itoa(want))
			code, timedOut, err := runRecipe(deadlineWhen(t, leaderGone(leaderPID)), runOpts{
				argv:    []string{script},
				dir:     t.TempDir(),
				defExec: defaultExecConfig(),
				stdout:  &lockedBuffer{},
			})
			require.ErrorContains(t, err, "recipe timed out")
			assert.True(t, timedOut)
			assert.Equal(t, want, code)
		})
	}
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

	// The script records its pid only once the trap is set: a cancel
	// that beat the trap would end it on SIGTERM, with no escalation.
	ready := filepath.Join(t.TempDir(), "ready.pid")
	script := writeScript(t, t.TempDir(), "ignore.sh", `trap '' TERM; echo $$ > "`+ready+`"; sleep 60`)
	force := make(chan struct{})
	ctx, cancel := context.WithCancel(WithForceKill(context.Background(), force))
	defer cancel()
	go func() {
		waitForPID(ready)
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

func TestKill_Unix_NilProcessWithForce(t *testing.T) {
	force := make(chan struct{})
	close(force)
	assert.False(t, afterStart(&exec.Cmd{}).kill(force))
}

func TestRunRecipe_ForceSkipsDrainWaitForSetsidDaemon(t *testing.T) {
	// A setsid daemon leaves the recipe's group but keeps the captured
	// stdout pipe. After a second interrupt the group is SIGKILLed;
	// runRecipe must then give the drain one short poll, not a whole
	// reapWait, before it abandons the pipe.
	if _, err := exec.LookPath("setsid"); err != nil {
		t.Skip("no setsid")
	}
	old := gracePeriod
	gracePeriod = 20 * time.Second
	t.Cleanup(func() { gracePeriod = old })
	require.Equal(t, 5*time.Second, reapWait, "the default reapWait")

	dir := t.TempDir()
	ready := filepath.Join(dir, "ready.pid")
	daemonPID := filepath.Join(dir, "daemon.pid")
	script := writeScript(t, t.TempDir(), "daemon.sh",
		`setsid sh -c 'echo $$ > "`+daemonPID+`"; exec sleep 30' &
trap '' TERM; echo $$ > "`+ready+`"; while :; do sleep 0.05; done`)
	t.Cleanup(func() {
		if pid := waitForPID(daemonPID); pid > 0 {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})

	force := make(chan struct{})
	ctx, cancel := context.WithCancel(WithForceKill(context.Background(), force))
	defer cancel()
	forcedAt := make(chan time.Time, 1)
	go func() {
		waitForPID(ready)
		waitForPID(daemonPID)
		cancel()
		time.Sleep(200 * time.Millisecond)
		forcedAt <- time.Now()
		close(force)
	}()
	_, timedOut, err := runRecipe(ctx, runOpts{
		argv:    []string{script},
		dir:     t.TempDir(),
		defExec: defaultExecConfig(),
		stdout:  &lockedBuffer{},
	})
	took := time.Since(<-forcedAt)
	require.ErrorIs(t, err, ErrForceKilled)
	assert.True(t, timedOut)
	assert.Less(t, took, time.Second, "a second interrupt must not wait out reapWait")
}

func TestRunRecipe_ForceShortensLeaderReap(t *testing.T) {
	// The group kill leaves the leader running (stubGroupKiller's kill
	// does nothing). With force already closed when the kill returns,
	// killLeaderOnForce kills the leader at once, not after reapWait.
	stubGroupKiller(t, func(*exec.Cmd) {})
	reapWait = 3 * time.Second
	script := writeScript(t, t.TempDir(), "slow.sh", `exec sleep 30`)

	force := make(chan struct{})
	close(force)
	ctx, cancel := context.WithTimeout(WithForceKill(context.Background(), force), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, timedOut, err := runRecipe(ctx, runOpts{
		argv:    []string{script},
		dir:     t.TempDir(),
		defExec: defaultExecConfig(),
	})
	require.Error(t, err)
	assert.True(t, timedOut)
	assert.Less(t, time.Since(start), 1500*time.Millisecond, "a closed force must not wait out reapWait")
}

func TestRunRecipe_LateForceKillsLeaderAtOnce(t *testing.T) {
	// The group kill leaves the leader running and returns at once (no
	// grace to cut short), so a second interrupt arrives only after it.
	// That late interrupt must still kill the leader directly, not
	// leave it to WaitDelay's whole reapWait.
	stubGroupKiller(t, func(*exec.Cmd) {})
	reapWait = 5 * time.Second
	script := writeScript(t, t.TempDir(), "slow.sh", `exec sleep 30`)

	force := make(chan struct{})
	ctx, cancel := context.WithTimeout(WithForceKill(context.Background(), force), 100*time.Millisecond)
	defer cancel()
	forcedAt := make(chan time.Time, 1)
	go func() {
		<-ctx.Done()
		time.Sleep(300 * time.Millisecond)
		forcedAt <- time.Now()
		close(force)
	}()
	_, timedOut, err := runRecipe(ctx, runOpts{
		argv:    []string{script},
		dir:     t.TempDir(),
		defExec: defaultExecConfig(),
	})
	took := time.Since(<-forcedAt)
	require.Error(t, err)
	assert.True(t, timedOut)
	assert.Less(t, took, 2*time.Second, "a late second interrupt must not wait out reapWait")
}

func TestRunRecipe_EmptiedGroupIsNotSignalledAtDeadline(t *testing.T) {
	// The leader backgrounds a setsid daemon, which leaves the recipe's
	// group but keeps the captured stdout pipe, and then exits. From then
	// on the group is empty, and its pgid (the leader's reaped pid) may
	// be reused by an unrelated group before the deadline. The deadline
	// kill must not signal it.
	if _, err := exec.LookPath("setsid"); err != nil {
		t.Skip("no setsid")
	}
	oldSignal, oldReap := signalGroup, reapWait
	var mu sync.Mutex
	var sent []syscall.Signal
	signalGroup = func(pgid int, sig syscall.Signal) error {
		mu.Lock()
		sent = append(sent, sig)
		mu.Unlock()
		return oldSignal(pgid, sig)
	}
	reapWait = 100 * time.Millisecond
	t.Cleanup(func() { signalGroup, reapWait = oldSignal, oldReap })

	dir := t.TempDir()
	leaderPID := filepath.Join(dir, "leader.pid")
	daemonPID := filepath.Join(dir, "daemon.pid")
	script := writeScript(t, t.TempDir(), "daemon.sh",
		`echo $$ > "`+leaderPID+`"
setsid sh -c 'echo $$ > "`+daemonPID+`"; exec sleep 30' &
while [ ! -s "`+daemonPID+`" ]; do sleep 0.05; done
echo started; exit 0`)
	t.Cleanup(func() {
		if pid := waitForPID(daemonPID); pid > 0 {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})

	code, timedOut, err := runRecipe(deadlineWhen(t, leaderGone(leaderPID)), runOpts{
		argv:    []string{script},
		dir:     t.TempDir(),
		defExec: defaultExecConfig(),
		stdout:  &lockedBuffer{},
	})
	require.ErrorContains(t, err, "recipe timed out")
	assert.True(t, timedOut)
	assert.Equal(t, 0, code)
	mu.Lock()
	defer mu.Unlock()
	assert.NotContains(t, sent, syscall.SIGTERM, "an emptied group must not be signalled")
	assert.NotContains(t, sent, syscall.SIGKILL, "an emptied group must not be signalled")
}

// stubSignalGroup swaps signalGroup for one test with one that returns
// err and records nothing.
func stubSignalGroup(t *testing.T, err error) {
	t.Helper()
	old := signalGroup
	signalGroup = func(int, syscall.Signal) error { return err }
	t.Cleanup(func() { signalGroup = old })
}

func TestPgKiller_LeaderExitedNilProcessIsNoOp(t *testing.T) {
	k := &pgKiller{leaderKill: leaderKill{&exec.Cmd{}}}
	k.leaderExited()
	assert.False(t, k.groupGone)
}

func TestPgKiller_LeaderExitedEmptyGroupStopsKill(t *testing.T) {
	stubSignalGroup(t, syscall.ESRCH)
	k := &pgKiller{leaderKill: leaderKill{&exec.Cmd{Process: &os.Process{Pid: 42}}}}
	k.leaderExited()
	assert.True(t, k.groupGone, "ESRCH: no member is left")
	called := false
	signalGroup = func(int, syscall.Signal) error { called = true; return nil }
	assert.False(t, k.kill(nil))
	assert.False(t, called, "kill must not signal an emptied group")
}

func TestPgKiller_LeaderExitedLiveGroupKeepsKill(t *testing.T) {
	for _, err := range []error{nil, syscall.EPERM} {
		stubSignalGroup(t, err)
		k := &pgKiller{leaderKill: leaderKill{&exec.Cmd{Process: &os.Process{Pid: 42}}}}
		k.leaderExited()
		assert.False(t, k.groupGone, "a group with members, or one we may not signal, still exists")
	}
}
