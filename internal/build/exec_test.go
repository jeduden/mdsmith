package build

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestWaitAtMost(t *testing.T) {
	done := make(chan error, 1)
	want := errors.New("exit 1")
	done <- want
	ok, err := waitAtMost(done, time.Second)
	assert.True(t, ok)
	assert.Same(t, want, err)

	ok, err = waitAtMost(done, time.Millisecond)
	assert.False(t, ok, "an empty channel times out")
	assert.NoError(t, err)
}

// TestRunRecipe_CancelledBeforeStartSpawnsNothing checks that a context
// cancelled before runRecipe starts (a CLI interrupt that lands while
// the target is staged) refuses the recipe instead of forking it. The
// argv names no real program: a Start attempt would fail with
// "starting recipe", so the error proves Start never ran.
func TestRunRecipe_CancelledBeforeStartSpawnsNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	code, timedOut, err := runRecipe(ctx, runOpts{
		argv:    []string{filepath.Join(t.TempDir(), "no-such-recipe")},
		dir:     t.TempDir(),
		defExec: defaultExecConfig(),
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.NotContains(t, err.Error(), "starting recipe")
	assert.False(t, timedOut, "no kill path ran, so the run is not reported as killed")
	assert.Equal(t, -1, code)
}

// TestRunRecipe_ExpiredDeadlineBeforeStartSpawnsNothing checks that a
// deadline already past at entry also refuses the recipe, and still
// reports it as a timeout.
func TestRunRecipe_ExpiredDeadlineBeforeStartSpawnsNothing(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	_, timedOut, err := runRecipe(ctx, runOpts{
		argv:    []string{filepath.Join(t.TempDir(), "no-such-recipe")},
		dir:     t.TempDir(),
		defExec: defaultExecConfig(),
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.NotContains(t, err.Error(), "starting recipe")
	assert.Contains(t, err.Error(), "timed out")
	assert.True(t, timedOut)
}

func TestWithForceKill_RoundTrip(t *testing.T) {
	assert.Nil(t, forceKillFrom(context.Background()))
	force := make(chan struct{})
	ctx, cancel := context.WithTimeout(WithForceKill(context.Background(), force), time.Minute)
	defer cancel()
	// A child context (the per-target timeout) still carries the channel.
	assert.Equal(t, (<-chan struct{})(force), forceKillFrom(ctx))
}

func TestRunOpts_ProcessLabel(t *testing.T) {
	assert.Equal(t, "recipe", runOpts{}.processLabel())
	assert.Equal(t, "hook", runOpts{label: "hook"}.processLabel())
}

func TestRunRecipe_StartErrorNamesLabel(t *testing.T) {
	_, _, err := runRecipe(context.Background(), runOpts{
		argv:  []string{filepath.Join(t.TempDir(), "no-such-hook")},
		dir:   t.TempDir(),
		label: "hook",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "starting hook")
}
