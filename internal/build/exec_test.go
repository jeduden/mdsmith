package build

import (
	"context"
	"errors"
	"os"
	"os/exec"
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
	ok, err := waitAtMost(done, time.Second, nil)
	assert.True(t, ok)
	assert.Same(t, want, err)

	ok, err = waitAtMost(done, time.Millisecond, nil)
	assert.False(t, ok, "an empty channel times out")
	assert.NoError(t, err)
}

func TestWaitAtMost_ForceCutsTheWaitShort(t *testing.T) {
	// A closed force (a second interrupt) ends a long wait after
	// forcedReapWait instead of the whole d.
	force := make(chan struct{})
	close(force)
	start := time.Now()
	ok, _ := waitAtMost(make(chan error), 5*time.Second, force)
	assert.False(t, ok)
	assert.GreaterOrEqual(t, time.Since(start), forcedReapWait)
	assert.Less(t, time.Since(start), 2*time.Second)

	// A wait already shorter than forcedReapWait keeps its own bound.
	start = time.Now()
	ok, _ = waitAtMost(make(chan error), 10*time.Millisecond, force)
	assert.False(t, ok)
	assert.Less(t, time.Since(start), forcedReapWait)

	// A value that arrives inside the cut wait is still received.
	done := make(chan error, 1)
	done <- nil
	ok, _ = waitAtMost(done, 5*time.Second, force)
	assert.True(t, ok)
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
	assert.ErrorIs(t, err, ErrNotStarted)
	assert.Contains(t, err.Error(), "recipe cancelled before start")
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
	assert.ErrorIs(t, err, ErrNotStarted, "the report must not claim a kill")
}

func TestNotStartedError(t *testing.T) {
	err := NotStartedError(context.Canceled)
	assert.EqualError(t, err, "recipe cancelled before start: context canceled")
	assert.ErrorIs(t, err, ErrNotStarted)
	assert.ErrorIs(t, err, context.Canceled)

	err = NotStartedError(context.DeadlineExceeded)
	assert.EqualError(t, err, "recipe timed out before start: context deadline exceeded")
	assert.ErrorIs(t, err, ErrNotStarted)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
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

func TestStartFailure(t *testing.T) {
	// A Start that failed for its own reason is a start failure, even
	// with ctx still live.
	boom := errors.New("boom")
	code, timedOut, err := startFailure(context.Background(), "hook", boom)
	assert.Equal(t, -1, code)
	assert.False(t, timedOut)
	require.ErrorIs(t, err, boom)
	assert.ErrorContains(t, err, "starting hook")
	assert.NotErrorIs(t, err, ErrNotStarted)

	// Start returned the done ctx's Err: the run never started.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	code, timedOut, err = startFailure(ctx, "recipe", ctx.Err())
	assert.Equal(t, -1, code)
	assert.False(t, timedOut, "a cancel is not a timeout")
	require.ErrorIs(t, err, ErrNotStarted)
	assert.ErrorIs(t, err, context.Canceled)

	dctx, dcancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer dcancel()
	<-dctx.Done()
	_, timedOut, err = startFailure(dctx, "recipe", dctx.Err())
	assert.True(t, timedOut)
	require.ErrorIs(t, err, ErrNotStarted)

	// A done ctx does not claim an unrelated start error.
	_, timedOut, err = startFailure(dctx, "recipe", boom)
	assert.False(t, timedOut)
	assert.NotErrorIs(t, err, ErrNotStarted)
}

// runKillLeaderOnForce runs killLeaderOnForce with fresh channels,
// lets step drive them, and reports whether the kill ran.
func runKillLeaderOnForce(step func(killed, force, exited chan struct{})) bool {
	killed, force, exited := make(chan struct{}), make(chan struct{}), make(chan struct{})
	ran := false
	done := make(chan struct{})
	go func() {
		defer close(done)
		killLeaderOnForce(func() { ran = true }, killed, force, exited)
	}()
	step(killed, force, exited)
	<-done
	return ran
}

func TestKillLeaderOnForce_ForceAfterKill(t *testing.T) {
	assert.True(t, runKillLeaderOnForce(func(killed, force, _ chan struct{}) {
		close(killed)
		close(force)
	}), "a second interrupt after the kill returned kills the leader")
}

func TestKillLeaderOnForce_ForceBeforeKill(t *testing.T) {
	assert.True(t, runKillLeaderOnForce(func(killed, force, _ chan struct{}) {
		close(force)
		close(killed)
	}), "a second interrupt during the kill kills the leader once it returns")
}

func TestKillLeaderOnForce_ExitBeforeKill(t *testing.T) {
	assert.False(t, runKillLeaderOnForce(func(_, force, exited chan struct{}) {
		close(exited)
		close(force)
	}), "a leader that exited with no kill run is not killed")
}

func TestKillLeaderOnForce_ExitWithNoForce(t *testing.T) {
	assert.False(t, runKillLeaderOnForce(func(killed, _, exited chan struct{}) {
		close(killed)
		close(exited)
	}), "a kill with no second interrupt leaves the leader to WaitDelay")
}

// recordKiller is a groupKiller that counts kills, sees the force
// channel it was given, and reports forced from each kill.
type recordKiller struct {
	kills  int
	force  <-chan struct{}
	forced bool
}

func (k *recordKiller) kill(force <-chan struct{}) bool {
	k.kills++
	k.force = force
	return k.forced
}

func (*recordKiller) close() {}

func TestKillerFor_RecipeUsesAfterStart(t *testing.T) {
	want := &recordKiller{}
	old := afterStartFn
	afterStartFn = func(*exec.Cmd) groupKiller { return want }
	t.Cleanup(func() { afterStartFn = old })
	assert.Same(t, want, killerFor(&exec.Cmd{}, false))
}

func TestKillerFor_HookSkipsAfterStart(t *testing.T) {
	old := afterStartFn
	afterStartFn = func(*exec.Cmd) groupKiller {
		t.Error("a hook must not reach afterStart")
		return &recordKiller{}
	}
	t.Cleanup(func() { afterStartFn = old })
	assert.NotNil(t, killerFor(&exec.Cmd{}, true))
}

func TestRecipeKill_CancelWaitsForArm(t *testing.T) {
	force := make(chan struct{})
	rk := newRecipeKill(force)
	k := &recordKiller{forced: true}
	done := make(chan error, 1)
	go func() { done <- rk.cancel() }()
	select {
	case <-rk.killed:
		t.Fatal("cancel ran the kill before arm installed the killer")
	case <-time.After(50 * time.Millisecond):
	}
	rk.arm(k)
	require.NoError(t, <-done)
	<-rk.killed
	assert.Equal(t, 1, k.kills)
	assert.Equal(t, (<-chan struct{})(force), k.force, "the kill gets the run's force channel")
	assert.True(t, rk.cancelled)
	assert.True(t, rk.forced, "forced is what the kill reported")
}

// noterKiller is a recordKiller that also records leaderExited.
type noterKiller struct {
	recordKiller
	noted bool
}

func (k *noterKiller) leaderExited() { k.noted = true }

func TestNoteLeaderExited_TellsANoter(t *testing.T) {
	k := &noterKiller{}
	noteLeaderExited(k)
	assert.True(t, k.noted)
}

func TestNoteLeaderExited_IgnoresOtherKillers(t *testing.T) {
	assert.NotPanics(t, func() { noteLeaderExited(&recordKiller{}) })
}
