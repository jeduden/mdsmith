//go:build unix

package build

import (
	"bytes"
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

// waitForPID polls path until it holds a pid, or returns 0 after 10 s.
func waitForPID(path string) int {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(path); err == nil {
			if n, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
				return n
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	return 0
}

// TestRunHook_RunsInMdsmithProcessGroup checks that a hook is not
// isolated like a recipe: it stays in mdsmith's own process group, so
// the terminal's Ctrl-C reaches it and the children it backgrounds (a
// dev server), and a hook that prompts on /dev/tty is not stopped by
// SIGTTIN as a background group would be.
func TestRunHook_RunsInMdsmithProcessGroup(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "hook.pid")
	script := writeScript(t, t.TempDir(), "self.sh", `echo $$ > "`+pidFile+`"; exec sleep 120`)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pgid := make(chan int, 1)
	go func() {
		defer cancel()
		pid := waitForPID(pidFile)
		g, err := syscall.Getpgid(pid)
		if err != nil {
			g = -1
		}
		pgid <- g
	}()

	result := runHook(ctx, []string{script}, dir)
	require.NotNil(t, result)
	assert.Equal(t, syscall.Getpgrp(), <-pgid, "a hook must share mdsmith's process group")
}

// TestRunHook_CancelKillsHookLeader checks that cancelling a hook's
// context (a CLI interrupt) ends the hook process itself. The kill is
// aimed at the leader alone: signalling the group would hit mdsmith.
func TestRunHook_CancelKillsHookLeader(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "hook.pid")
	script := writeScript(t, t.TempDir(), "self.sh", `echo $$ > "`+pidFile+`"; exec sleep 120`)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hookPID := make(chan int, 1)
	go func() {
		defer cancel()
		hookPID <- waitForPID(pidFile)
	}()

	start := time.Now()
	result := runHook(ctx, []string{script}, dir)
	pid := <-hookPID
	require.NotNil(t, result)
	assert.Contains(t, result.Err.Error(), "(interrupted)")
	require.NotZero(t, pid)
	assert.False(t, processAlive(pid), "the hook leader must be reaped")
	assert.Less(t, time.Since(start), 10*time.Second)
}

// TestRunHook_SuccessLeavesBackgroundChild checks the documented
// dev-server pattern: a before-hook that backgrounds a server and
// exits 0 leaves the server running for the recipes.
func TestRunHook_SuccessLeavesBackgroundChild(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "server.pid")
	script := writeScript(t, t.TempDir(), "start.sh",
		`sleep 120 >/dev/null 2>&1 & echo $! > "`+pidFile+`"`)

	require.Nil(t, runHook(context.Background(), []string{script}, dir))
	pid := waitForPID(pidFile)
	require.NotZero(t, pid)
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	assert.True(t, processAlive(pid), "a successful hook's background child must survive")
}

// TestRunHook_InstallsNoJobObject checks that a hook gets no Windows
// Job Object: its kill-on-close would end a dev server the hook
// started as soon as the hook exits.
func TestRunHook_InstallsNoJobObject(t *testing.T) {
	var calls int
	old := afterStartFn
	afterStartFn = func(cmd *exec.Cmd) groupKiller { calls++; return afterStart(cmd) }
	t.Cleanup(func() { afterStartFn = old })

	script := writeScript(t, t.TempDir(), "noop.sh", `exit 0`)
	require.Nil(t, runHook(context.Background(), []string{script}, t.TempDir()))
	assert.Zero(t, calls)
}

// TestRunHook_KeepsMdsmithEnvironment checks that moving hooks onto
// runRecipe did not give them the hermetic recipe environment: a hook
// still sees a variable from mdsmith's own environment.
func TestRunHook_KeepsMdsmithEnvironment(t *testing.T) {
	t.Setenv("MDSMITH_HOOK_ENV_PROBE", "yes")
	script := writeScript(t, t.TempDir(), "probe.sh", `[ "$MDSMITH_HOOK_ENV_PROBE" = yes ]`)
	assert.Nil(t, runHook(context.Background(), []string{script}, t.TempDir()))
}

// shortGrace sets gracePeriod to d for one test.
func shortGrace(t *testing.T, d time.Duration) {
	t.Helper()
	old := gracePeriod
	gracePeriod = d
	t.Cleanup(func() { gracePeriod = old })
}

// runHookCancelledWhenReady runs script as a hook and cancels its
// context once the script has written its pid to ready, then closes
// force after forceAfter (never when it is zero). It returns the
// hook's result, the hook pid, and how long runHook took after the
// cancel.
func runHookCancelledWhenReady(
	t *testing.T, script, ready string, forceAfter time.Duration,
) (*HookResult, int, time.Duration) {
	t.Helper()
	force := make(chan struct{})
	ctx, cancel := context.WithCancel(WithForceKill(context.Background(), force))
	defer cancel()
	pid := make(chan int, 1)
	cancelled := make(chan time.Time, 1)
	go func() {
		p := waitForPID(ready)
		pid <- p
		cancelled <- time.Now()
		cancel()
		if forceAfter > 0 {
			time.Sleep(forceAfter)
			close(force)
		}
	}()
	result := runHook(ctx, []string{script}, t.TempDir())
	end := time.Now()
	p := <-pid
	require.NotZero(t, p, "the hook never became ready")
	return result, p, end.Sub(<-cancelled)
}

// TestRunHook_CancelRunsTermTrapCleanup checks that a cancelled hook
// gets SIGTERM, not an immediate SIGKILL: a hook that traps TERM runs
// its cleanup, as it did before the build pass caught interrupts.
func TestRunHook_CancelRunsTermTrapCleanup(t *testing.T) {
	shortGrace(t, 5*time.Second)
	dir := t.TempDir()
	ready := filepath.Join(dir, "ready.pid")
	marker := filepath.Join(dir, "cleaned")
	script := writeScript(t, t.TempDir(), "trap.sh",
		`trap ': > "`+marker+`"; exit 0' TERM; echo $$ > "`+ready+`"; while :; do sleep 0.05; done`)

	result, pid, took := runHookCancelledWhenReady(t, script, ready, 0)
	require.NotNil(t, result)
	assert.Contains(t, result.Err.Error(), "(interrupted)")
	assert.FileExists(t, marker, "the hook's TERM trap must run its cleanup")
	assert.False(t, processAlive(pid), "the hook leader must be reaped")
	assert.Less(t, took, gracePeriod, "a hook that exits on SIGTERM ends the grace early")
}

// TestRunHook_CancelKillsTermIgnoringHookAfterGrace checks that a hook
// that ignores SIGTERM is SIGKILLed once the grace period runs out.
func TestRunHook_CancelKillsTermIgnoringHookAfterGrace(t *testing.T) {
	shortGrace(t, 300*time.Millisecond)
	dir := t.TempDir()
	ready := filepath.Join(dir, "ready.pid")
	script := writeScript(t, t.TempDir(), "ignore.sh",
		`trap '' TERM; echo $$ > "`+ready+`"; while :; do sleep 0.05; done`)

	result, pid, took := runHookCancelledWhenReady(t, script, ready, 0)
	require.NotNil(t, result)
	assert.False(t, processAlive(pid), "the hook leader must be killed after the grace")
	assert.GreaterOrEqual(t, took, gracePeriod, "the hook must get the whole grace period")
	assert.Less(t, took, 5*time.Second)
}

// TestRunHook_ForceSkipsHookGrace checks that a second interrupt (the
// force channel) cuts a hook's SIGTERM grace short with SIGKILL.
func TestRunHook_ForceSkipsHookGrace(t *testing.T) {
	shortGrace(t, 20*time.Second)
	dir := t.TempDir()
	ready := filepath.Join(dir, "ready.pid")
	script := writeScript(t, t.TempDir(), "ignore.sh",
		`trap '' TERM; echo $$ > "`+ready+`"; while :; do sleep 0.05; done`)

	result, pid, took := runHookCancelledWhenReady(t, script, ready, 200*time.Millisecond)
	require.NotNil(t, result)
	assert.False(t, processAlive(pid), "the hook leader must be killed")
	assert.Less(t, took, 5*time.Second, "the second interrupt must skip the grace")
}

// TestRunAfterHooks_InterruptDuringHookSkipsTheRest checks that an
// interrupt landing while one after-hook runs reports that hook as
// interrupted and starts and prints nothing for the hooks after it.
func TestRunAfterHooks_InterruptDuringHookSkipsTheRest(t *testing.T) {
	dir := t.TempDir()
	ready := filepath.Join(dir, "ready.pid")
	first := writeScript(t, t.TempDir(), "first.sh", `echo $$ > "`+ready+`"; exec sleep 60`)
	marker := filepath.Join(dir, "second-ran")
	second := writeScript(t, t.TempDir(), "second.sh", `: > "`+marker+`"`)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		waitForPID(ready)
		cancel()
	}()
	var w bytes.Buffer
	result := RunAfterHooks(ctx, []HookEntry{
		{Tokens: []string{first}, Name: "first"},
		{Tokens: []string{second}, Name: "second"},
	}, dir, &w)
	require.NotNil(t, result)
	assert.Contains(t, w.String(), "hook first: FAIL (exit 1): context canceled (interrupted)")
	assert.NotContains(t, w.String(), "hook second")
	assert.NoFileExists(t, marker)
}
