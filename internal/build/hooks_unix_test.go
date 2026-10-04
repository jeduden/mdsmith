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
	afterStartFn = func(*exec.Cmd) func() { calls++; return nil }
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
