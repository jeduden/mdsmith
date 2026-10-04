//go:build unix || windows || plan9

package build

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubKiller is a groupKiller whose kill, close, and forceLeader run
// the given funcs; a nil func does nothing.
type stubKiller struct{ killFn, closeFn, forceFn func() }

func (k stubKiller) forceLeader() {
	if k.forceFn != nil {
		k.forceFn()
	}
}

func (k stubKiller) kill() {
	if k.killFn != nil {
		k.killFn()
	}
}

func (k stubKiller) close() {
	if k.closeFn != nil {
		k.closeFn()
	}
}

func TestRunRecipe_ClosesKillerOnReturn(t *testing.T) {
	skipWithoutPOSIXTools(t, "sh")
	// runRecipe owns the killer afterStart returns and must close it
	// on return. A recipe that exits on its own is never killed.
	var ran, killed atomic.Bool
	old := afterStartFn
	afterStartFn = func(*exec.Cmd) groupKiller {
		return stubKiller{
			closeFn: func() { ran.Store(true) },
			killFn:  func() { killed.Store(true) },
			forceFn: func() { killed.Store(true) },
		}
	}
	t.Cleanup(func() { afterStartFn = old })

	stage := t.TempDir()
	script := writeScript(t, t.TempDir(), "noop.sh", `exit 0`)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _, err := runRecipe(ctx, runOpts{
		argv:    []string{script},
		dir:     stage,
		exec:    ExecConfig{},
		defExec: defaultExecConfig(),
	})
	require.NoError(t, err)
	assert.True(t, ran.Load(), "the killer must be closed on return")
	assert.False(t, killed.Load(), "a recipe that exited on its own must not be killed")
}

func TestRunRecipe_ClosesKillerOnTimeout(t *testing.T) {
	skipWithoutPOSIXTools(t, "sh")
	// The timeout return must close the killer too, after its kill: on
	// Windows close is what fires KILL_ON_JOB_CLOSE, and on plan9 it
	// frees the notepg file. The stub forwards to the real killer, so
	// the recipe's group still dies.
	var killed, closedAfterKill atomic.Bool
	old := afterStartFn
	afterStartFn = func(cmd *exec.Cmd) groupKiller {
		inner := afterStart(cmd)
		return stubKiller{
			killFn:  func() { killed.Store(true); inner.kill() },
			forceFn: inner.forceLeader,
			closeFn: func() { closedAfterKill.Store(killed.Load()); inner.close() },
		}
	}
	t.Cleanup(func() { afterStartFn = old })

	script := writeScript(t, t.TempDir(), "slow.sh", `sleep 120`)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, timedOut, err := runRecipe(ctx, runOpts{
		argv:    []string{script},
		dir:     t.TempDir(),
		defExec: defaultExecConfig(),
	})
	require.Error(t, err)
	require.True(t, timedOut)
	assert.True(t, closedAfterKill.Load(), "the killer must be closed after the timeout kill")
}

func TestRunRecipe_HermeticEnvVisibleToProcess(t *testing.T) {
	skipWithoutPOSIXTools(t, "sh")
	t.Setenv("HOME", "/home/tester")
	t.Setenv("SECRET_TOKEN", "leak-me")

	stage := t.TempDir()
	out := filepath.Join(stage, "env.txt")
	// `env` is a coreutils binary on PATH /usr/bin:/bin.
	script := writeScript(t, t.TempDir(), "dumpenv.sh", `env | sort > "$1"`)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _, recipeErr := runRecipe(ctx, runOpts{
		argv:    []string{script, out},
		dir:     stage,
		exec:    ExecConfig{},
		defExec: defaultExecConfig(),
	})
	require.NoError(t, recipeErr)

	data, err := os.ReadFile(out)
	require.NoError(t, err)
	body := string(data)
	assert.Contains(t, body, "PATH="+defaultExecPath)
	assert.Contains(t, body, "HOME=/home/tester")
	assert.NotContains(t, body, "SECRET_TOKEN")
}

func TestRunRecipe_CmdDirIsStaging(t *testing.T) {
	skipWithoutPOSIXTools(t, "sh")
	stage := t.TempDir()
	realStage, err := filepath.EvalSymlinks(stage)
	require.NoError(t, err)
	out := filepath.Join(stage, "pwd.txt")
	script := writeScript(t, t.TempDir(), "pwd.sh", `pwd > "$1"`)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _, err = runRecipe(ctx, runOpts{
		argv:    []string{script, out},
		dir:     stage,
		exec:    ExecConfig{},
		defExec: defaultExecConfig(),
	})
	require.NoError(t, err)
	data, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, realStage, strings.TrimSpace(string(data)))
}

func TestRunRecipe_TimeoutErrorMessageIsDeterministic(t *testing.T) {
	skipWithoutPOSIXTools(t, "sh")
	stage := t.TempDir()
	script := writeScript(t, t.TempDir(), "slow.sh", `sleep 120`)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, _, err := runRecipe(ctx, runOpts{
		argv:    []string{script},
		dir:     stage,
		exec:    ExecConfig{},
		defExec: defaultExecConfig(),
	})
	require.Error(t, err)
	// A deadline-bound context reports a timeout, never a bare "cancelled".
	assert.Contains(t, err.Error(), "timed out")
	assert.NotContains(t, err.Error(), "cancelled")
}

func TestRunRecipe_CancellationReported(t *testing.T) {
	skipWithoutPOSIXTools(t, "sh")
	stage := t.TempDir()
	script := writeScript(t, t.TempDir(), "slow.sh", `sleep 120`)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()
	_, _, err := runRecipe(ctx, runOpts{
		argv:    []string{script},
		dir:     stage,
		exec:    ExecConfig{},
		defExec: defaultExecConfig(),
	})
	require.Error(t, err)
	// A non-deadline cancellation reports "cancelled", not "timed out".
	assert.Contains(t, err.Error(), "cancelled")
}

// skipOnPlan9 skips a test that runs a `#!/bin/sh` script on plan9,
// which has only `rc`. These files carry `unix || windows || plan9`,
// the complement of exec_other.go's tag, so GOOS=plan9 go vet still
// type-checks them (plan 2610030243). The rc-based tests plan9 runs
// live in exec_plan9_test.go.
func skipOnPlan9(t testing.TB) {
	t.Helper()
	if runtime.GOOS == "plan9" {
		t.Skip("plan9 has no sh; see exec_plan9_test.go")
	}
}

// skipWithoutPOSIXTools skips a test whose recipe needs the named POSIX
// tools (an `sh` script, `cp`, ...): Windows lacks them and plan9 has
// only `rc`. The skip names tools on both, since a `cp` or `cat` test
// is skipped on plan9 for its POSIX recipe, not for a missing binary.
func skipWithoutPOSIXTools(t testing.TB, tools string) {
	t.Helper()
	switch runtime.GOOS {
	case "plan9":
		t.Skip(tools + " recipe assumes POSIX tools; plan9 has only rc, see exec_plan9_test.go")
	case "windows":
		t.Skip(tools + " is not available on Windows")
	}
}
