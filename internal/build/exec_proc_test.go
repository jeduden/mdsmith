//go:build unix || windows

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

func TestRunRecipe_NonNilJobCleanup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh is not available on Windows")
	}
	// On Unix afterStart returns nil; inject a non-nil cleanup so runRecipe
	// installs and runs the deferred-cleanup branch.
	var ran atomic.Bool
	old := afterStartFn
	afterStartFn = func(*exec.Cmd) func() { return func() { ran.Store(true) } }
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
	assert.True(t, ran.Load(), "deferred job cleanup must run")
}

func TestRunRecipe_HermeticEnvVisibleToProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh not available on Windows")
	}
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
	if runtime.GOOS == "windows" {
		t.Skip("sh not available on Windows")
	}
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
	if runtime.GOOS == "windows" {
		t.Skip("sh not available on Windows")
	}
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
	if runtime.GOOS == "windows" {
		t.Skip("sh not available on Windows")
	}
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
