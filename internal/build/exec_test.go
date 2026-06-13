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

func TestRunRecipe_StartError(t *testing.T) {
	// A nonexistent program makes cmd.Start fail before Wait, hitting the
	// "starting recipe" error branch.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _, err := runRecipe(ctx, runOpts{
		argv:    []string{filepath.Join(t.TempDir(), "does-not-exist")},
		dir:     t.TempDir(),
		exec:    ExecConfig{},
		defExec: defaultExecConfig(),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "starting recipe")
}

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

func TestBuildEnv_DefaultsOnly(t *testing.T) {
	t.Setenv("HOME", "/home/tester")
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("SECRET_TOKEN", "leak-me")

	env := buildEnv(ExecConfig{}, defaultExecConfig())

	got := envMap(env)
	assert.Equal(t, defaultExecPath, got["PATH"])
	assert.Equal(t, "/home/tester", got["HOME"])
	assert.Equal(t, "en_US.UTF-8", got["LANG"])
	_, leaked := got["SECRET_TOKEN"]
	assert.False(t, leaked, "non-allowlisted var must not pass through")
}

func TestBuildEnv_CustomPathAndPassThrough(t *testing.T) {
	t.Setenv("HOME", "/home/tester")
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	t.Setenv("LANG", "C")

	cfg := ExecConfig{
		Path:           "/opt/bin:/bin",
		EnvPassThrough: []string{"SOURCE_DATE_EPOCH"},
	}
	env := buildEnv(cfg, defaultExecConfig())
	got := envMap(env)
	assert.Equal(t, "/opt/bin:/bin", got["PATH"])
	assert.Equal(t, "1700000000", got["SOURCE_DATE_EPOCH"])
	// EnvPassThrough replaces the default list; HOME/LANG are not re-listed.
	_, hasHome := got["HOME"]
	assert.False(t, hasHome, "custom pass-through replaces defaults, not appends")
	_, hasLang := got["LANG"]
	assert.False(t, hasLang)
}

func TestBuildEnv_UnsetPassThroughIsOmitted(t *testing.T) {
	_ = os.Unsetenv("LC_ALL")
	t.Setenv("HOME", "/h")
	env := buildEnv(ExecConfig{}, defaultExecConfig())
	got := envMap(env)
	_, ok := got["LC_ALL"]
	assert.False(t, ok, "an unset pass-through var produces no entry")
}

func TestBuildEnv_RejectsMalformedPassThroughName(t *testing.T) {
	t.Setenv("GOOD", "yes")
	// A name with "=" or a control char is skipped even if such a variable
	// somehow exists, so it cannot smuggle an entry into the environment.
	cfg := ExecConfig{EnvPassThrough: []string{"GOOD", "EVIL=injected", "BAD\nNAME"}}
	env := buildEnv(cfg, defaultExecConfig())
	got := envMap(env)
	assert.Equal(t, "yes", got["GOOD"])
	for k := range got {
		assert.NotContains(t, k, "=")
		assert.NotContains(t, k, "\n")
	}
}

// envMap parses a KEY=VALUE slice into a map.
func envMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, e := range env {
		k, v, _ := strings.Cut(e, "=")
		m[k] = v
	}
	return m
}

func TestBuildEnv_SkipsEmptyAndPathPassThroughNames(t *testing.T) {
	// An empty name and the literal "PATH" in the pass-through list must be
	// silently skipped: PATH is set explicitly and an empty name is meaningless.
	t.Setenv("PATH", "/injected")
	cfg := ExecConfig{
		Path:           "/custom/bin",
		EnvPassThrough: []string{"", "PATH"},
	}
	env := buildEnv(cfg, defaultExecConfig())
	got := envMap(env)
	// PATH must come from cfg.Path, not from the environment.
	assert.Equal(t, "/custom/bin", got["PATH"])
	// Only PATH should be in the map; empty name produces no entry.
	assert.Len(t, got, 1)
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

func TestRunRecipe_TimeoutKillsProcessGroup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process-group kill tested on Unix")
	}
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
